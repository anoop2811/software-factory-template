package acceptance_test

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
)

// This independent suite executes only test-owned receipts. Its entry barrier
// requires every selected worker to execute an actual criterion, not merely start.
// per docs/adr/0102-parallel-go-qualification.md:78
const sourceGateWorkerSuite = `package workerproof_test
import (
 "fmt"
 "os"
 "path/filepath"
 "strconv"
 "testing"
 "time"
 . "github.com/onsi/ginkgo/v2"
 . "github.com/onsi/gomega"
)
func TestWorkerProof(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t,"Go factory command acceptance") }
func proof(name string) {
 dir:=os.Getenv("WORKER_PROOF_DIR")
 workers,err:=strconv.Atoi(os.Getenv("WORKER_PROOF_PROCS"));Expect(err).NotTo(HaveOccurred())
 Expect(os.WriteFile(filepath.Join(dir,fmt.Sprintf("worker-%d",GinkgoParallelProcess())),[]byte(strconv.Itoa(os.Getpid())),0600)).To(Succeed())
 Eventually(func() int { paths,_:=filepath.Glob(filepath.Join(dir,"worker-*"));return len(paths) },10*time.Second,10*time.Millisecond).Should(Equal(workers))
 receipt,err:=os.OpenFile(filepath.Join(dir,"criterion-"+name),os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 Expect(err).NotTo(HaveOccurred(),"criterion must execute exactly once")
 _,err=fmt.Fprint(receipt,os.Getpid());Expect(err).NotTo(HaveOccurred());Expect(receipt.Close()).To(Succeed())
 if os.Getenv("WORKER_PROOF_FAIL")==name { Fail("independent deliberate worker failure") }
}
var _=Describe("Parallel runner proof",func(){
 for i:=0;i<4;i++ { name:=fmt.Sprintf("base-%d",i);It(name,func(){proof(name)}) }
})
var _=Describe("Whole installation upgrade release candidate handoff",func(){
 for i:=0;i<4;i++ { name:=fmt.Sprintf("handoff-%d",i);It(name,func(){proof(name)}) }
})
`

// per docs/adr/0102-parallel-go-qualification.md:23
// per docs/adr/0102-parallel-go-qualification.md:44
var _ = Describe("Go source qualification worker execution", func() {
	// per docs/adr/0102-parallel-go-qualification.md:78
	// per docs/adr/0104-adaptive-qualification-workers.md:62
	It("executes one and two actual workers with exact-once reports and propagates a real worker failure", func() {
		// per docs/adr/0104-adaptive-qualification-workers.md:42
		// Exercise the compiled detector in children, without mutating this worker's runtime.
		detect := func(settings ...string) (string, string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, sourceGateBuildWorkerHelper()) // #nosec G204 -- cached independently built private helper.
			for _, entry := range os.Environ() {
				key, _, _ := strings.Cut(entry, "=")
				if !strings.HasPrefix(key, "GO_RUNTIME_TEST_") && key != "GOMAXPROCS" && key != "GODEBUG" {
					command.Env = append(command.Env, entry)
				}
			}
			command.Env = append(command.Env, "GO_RUNTIME_TEST_PROCS=auto", "GO_RUNTIME_TEST_MAX_PROCS=4")
			command.Env = append(command.Env, settings...)
			var stdout, stderr strings.Builder
			command.Stdout, command.Stderr = &stdout, &stderr
			err := command.Run()
			Expect(ctx.Err()).NotTo(HaveOccurred())
			Expect(err).NotTo(HaveOccurred(), "actual detector: %s", stderr.String())
			return stdout.String(), stderr.String()
		}
		stdout, stderr := detect("GOMAXPROCS=1")
		Expect(stdout).To(Equal("1 1\n"))
		Expect(stderr).To(ContainSubstring("cpus=1"))
		if runtime.GOOS == "linux" {
			for _, value := range []string{"0", "00", "+0", "-1", "invalid", "1,containermaxprocs=0"} {
				stdout, stderr = detect("GODEBUG=containermaxprocs=" + value)
				Expect(stdout).To(Equal("1 1\n"))
				Expect(stderr).To(ContainSubstring("cpus=1"))
			}
			_, enabled := detect("GODEBUG=containermaxprocs=1")
			_, finalEnabled := detect("GODEBUG=containermaxprocs=0,containermaxprocs=1")
			cpuField := func(diagnostic string) string {
				for _, field := range strings.Fields(diagnostic) {
					if strings.HasPrefix(field, "cpus=") {
						return field
					}
				}
				Fail("actual detector must report its CPU count")
				return ""
			}
			Expect(cpuField(finalEnabled)).To(Equal(cpuField(enabled)), "the final duplicate runtime setting takes precedence")
		}
		root := GinkgoT().TempDir()
		for _, path := range []string{"go.mod", "go.sum"} {
			body, err := os.ReadFile(filepath.Join("..", path))
			Expect(err).NotTo(HaveOccurred())
			writeFixture(filepath.Join(root, path), body, 0600)
		}
		writeFixture(filepath.Join(root, "acceptance/worker_test.go"), []byte(sourceGateWorkerSuite), 0600)
		for _, run := range []struct {
			group, requested, workers, failure string
		}{{"base", "1", "1", ""}, {"base", "3", "3", ""}, {"release-handoff", "3", "2", ""}, {"release-handoff", "2", "2", "handoff-0"}} {
			fixture := sourceGateMakeFixture()
			reportDirectory := filepath.Join(fixture.root, "reports")
			status, calls, _ := sourceGateRun(fixture, "make go-runtime-source-check", []string{"GO_RUNTIME_TEST_GROUP=" + run.group, "GO_RUNTIME_TEST_PROCS=" + run.requested, "GO_RUNTIME_REPORT_DIR=" + reportDirectory})
			Expect(status).To(Equal(0))
			tests := sourceGateTests(calls)
			Expect(tests).To(HaveLen(1))
			sourceGateAssertPartition(tests[0], run.group)
			Expect(tests[0].Args[0]).To(Equal("run"), "replay the real Make-selected pinned runner")
			jsonPath, junitPath := filepath.Join(reportDirectory, run.group+".json"), filepath.Join(reportDirectory, run.group+".xml")
			Expect(os.Remove(jsonPath)).To(Succeed())
			Expect(os.Remove(junitPath)).To(Succeed())
			receipts := GinkgoT().TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			command := exec.CommandContext(ctx, "go", tests[0].Args...) // #nosec G204 -- independently observed Make argv, replayed against an inert test-owned suite.
			command.Dir = root
			command.Env = append(os.Environ(), "WORKER_PROOF_DIR="+receipts, "WORKER_PROOF_PROCS="+run.workers, "WORKER_PROOF_FAIL="+run.failure)
			output, err := command.CombinedOutput()
			Expect(ctx.Err()).NotTo(HaveOccurred(), "actual runner exceeded its finite fixture bound: %s", output)
			cancel()
			if run.failure == "" {
				Expect(err).NotTo(HaveOccurred(), "actual %s/%s runner: %s", run.group, run.workers, output)
			} else {
				var exit *exec.ExitError
				Expect(errors.As(err, &exit)).To(BeTrue(), "a real failed worker must reach the selected runner: %s", output)
				Expect(exit.ExitCode()).NotTo(Equal(0))
			}
			workers, err := filepath.Glob(filepath.Join(receipts, "worker-*"))
			Expect(err).NotTo(HaveOccurred())
			expectedWorkers, err := strconv.Atoi(run.workers)
			Expect(err).NotTo(HaveOccurred())
			Expect(workers).To(HaveLen(expectedWorkers))
			pids := map[string]bool{}
			for _, path := range workers {
				pid, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(pid)).To(MatchRegexp("^[1-9][0-9]*$"))
				Expect(pids).NotTo(HaveKey(string(pid)), "two workers must be different executing processes")
				pids[string(pid)] = true
			}
			criteria, err := filepath.Glob(filepath.Join(receipts, "criterion-*"))
			Expect(err).NotTo(HaveOccurred())
			Expect(criteria).To(HaveLen(4), "one failed worker must not hide the other selected criteria")
			prefix, container := "base", "Parallel runner proof"
			if run.group == "release-handoff" {
				prefix, container = "handoff", "Whole installation upgrade release candidate handoff"
			}
			expected := []string{}
			for index := range 4 {
				name := fmt.Sprintf("%s-%d", prefix, index)
				pid, err := os.ReadFile(filepath.Join(receipts, "criterion-"+name))
				Expect(err).NotTo(HaveOccurred())
				Expect(pids).To(HaveKey(string(pid)))
				expected = append(expected, container+" "+name)
			}
			data, err := os.ReadFile(jsonPath)
			Expect(err).NotTo(HaveOccurred())
			var reports []types.Report
			Expect(json.Unmarshal(data, &reports)).To(Succeed())
			Expect(reports).To(HaveLen(1))
			Expect(reports[0].SuiteConfig.ParallelTotal).To(Equal(expectedWorkers))
			Expect(reports[0].SuiteSucceeded).To(Equal(run.failure == ""))
			identities, failed := []string{}, 0
			for _, spec := range reports[0].SpecReports {
				if spec.LeafNodeType != types.NodeTypeIt || spec.State.Is(types.SpecStateSkipped) {
					continue
				}
				identities = append(identities, spec.FullText())
				Expect(spec.RunTime).To(BeNumerically(">", 0))
				Expect(spec.StartTime.IsZero()).To(BeFalse())
				if spec.State.Is(types.SpecStateFailed) {
					failed++
				} else {
					Expect(spec.State.Is(types.SpecStatePassed)).To(BeTrue())
				}
			}
			sort.Strings(identities)
			sort.Strings(expected)
			Expect(identities).To(Equal(expected), "machine-readable identities must be exactly the independently declared four selected criteria")
			expectedFailures := 0
			if run.failure != "" {
				expectedFailures = 1
			}
			Expect(failed).To(Equal(expectedFailures))
			data, err = os.ReadFile(junitPath)
			Expect(err).NotTo(HaveOccurred())
			var junit struct {
				Suites []struct {
					Cases []struct {
						Name    string    `xml:"name,attr"`
						Time    string    `xml:"time,attr"`
						Skipped *struct{} `xml:"skipped"`
						Failure *struct{} `xml:"failure"`
					} `xml:"testcase"`
				} `xml:"testsuite"`
			}
			Expect(xml.Unmarshal(data, &junit)).To(Succeed())
			active, junitFailures := 0, 0
			for _, suite := range junit.Suites {
				for _, entry := range suite.Cases {
					if entry.Skipped == nil {
						active++
						Expect(entry.Name).To(ContainSubstring(container))
						Expect(entry.Time).NotTo(BeEmpty())
						if entry.Failure != nil {
							junitFailures++
						}
					}
				}
			}
			Expect(active).To(Equal(4))
			Expect(junitFailures).To(Equal(expectedFailures))
			_, _ = fmt.Fprintf(GinkgoWriter, "actual runner group=%s workers=%s PIDs=%v exact identities=%q deliberate_failure=%q output=%q\n", run.group, run.workers, pids, identities, run.failure, strings.TrimSpace(string(output)))
		}
	})
})
