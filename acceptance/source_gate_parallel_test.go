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
const sourceGateWorkerBootstrap = `package workerproof_test
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
`

const sourceGateWorkerSuite = sourceGateWorkerBootstrap + `
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

// Four small whole groups exercise scheduling without splitting Ordered fixtures.
// The heavy groups rendezvous; one stays occupied until medium completes, so
// selecting light before medium is an observable policy failure, not a timing claim.
// per docs/adr/0107-published-release-qualification.md:222
const sourceGatePrioritySuite = sourceGateWorkerBootstrap + `
func exists(name string) bool { _,err:=os.Stat(filepath.Join(os.Getenv("WORKER_PROOF_DIR"),name));return err==nil }
func receipt(name string) {
 file,err:=os.OpenFile(filepath.Join(os.Getenv("WORKER_PROOF_DIR"),name),os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 Expect(err).NotTo(HaveOccurred(),"each ordered node must execute exactly once")
 _,err=fmt.Fprint(file,os.Getpid());Expect(err).NotTo(HaveOccurred());Expect(file.Close()).To(Succeed())
}
func priority(name string) int { n,err:=strconv.Atoi(os.Getenv("PRIORITY_"+name));if err!=nil {panic(err)};return n }
func group(name string) bool {
 return Describe("Whole installation upgrade release candidate handoff "+name,Ordered,ContinueOnFailure,SpecPriority(priority(name)),func(){
  BeforeAll(func(){
   receipt("started-"+name)
   switch name {
   case "HEAVY_A","HEAVY_B":
    Eventually(func() bool{return exists("started-HEAVY_A")&&exists("started-HEAVY_B")},3*time.Second,10*time.Millisecond).Should(BeTrue(),"priority oracle: both heavy groups must be dispatched first")
   case "MEDIUM":
    Expect(exists("started-HEAVY_A")&&exists("started-HEAVY_B")).To(BeTrue(),"priority oracle: medium cannot precede either heavy group")
   case "LIGHT":
    Expect(exists("finished-MEDIUM")).To(BeTrue(),"priority oracle: light cannot precede medium completion")
   }
  })
  It("first",func(){receipt("criterion-"+name+"-first")})
  It("second",func(){
   Expect(exists("criterion-"+name+"-first")).To(BeTrue(),"Ordered first must precede second")
   if name=="HEAVY_B" {Eventually(func()bool{return exists("finished-MEDIUM")},3*time.Second,10*time.Millisecond).Should(BeTrue(),"priority oracle: the freed worker must dispatch medium before light")}
   receipt("criterion-"+name+"-second")
  })
  AfterAll(func(){if exists("criterion-"+name+"-second"){receipt("finished-"+name)}})
 })
}
var _=group("HEAVY_A")
var _=group("HEAVY_B")
var _=group("MEDIUM")
var _=group("LIGHT")
`

// Ginkgo's default JSON omits priority and Ordered fields; serialize those
// primitives explicitly from its actual per-spec callback, including in dry-run.
// per docs/adr/0107-published-release-qualification.md:222
const sourceGatePriorityObserver = `package acceptance_test
import (
 "encoding/json"
 "os"
 . "github.com/onsi/ginkgo/v2"
 "github.com/onsi/ginkgo/v2/types"
 . "github.com/onsi/gomega"
)
var _=ReportAfterEach(func(spec SpecReport){
 path:=os.Getenv("PRIORITY_METADATA_PATH")
 if path==""||spec.LeafNodeType!=types.NodeTypeIt||spec.State.Is(types.SpecStateSkipped){return}
 file,err:=os.OpenFile(path,os.O_WRONLY|os.O_CREATE|os.O_APPEND,0600);Expect(err).NotTo(HaveOccurred())
 err=json.NewEncoder(file).Encode(struct{Text string;Labels []string;Priority int;Ordered bool}{spec.FullText(),spec.Labels(),spec.SpecPriority,spec.IsInOrderedContainer})
 Expect(err).NotTo(HaveOccurred());Expect(file.Close()).To(Succeed())
})
`

type sourceGatePriorityEntry struct {
	Text     string
	Labels   []string
	Priority int
	Ordered  bool
}

// per docs/adr/0107-published-release-qualification.md:207
// per docs/adr/0107-published-release-qualification.md:222
func sourceGateHandoffPriorityProof() {
	GinkgoHelper()
	fixture := sourceGateMakeFixture()
	reportsDir := filepath.Join(fixture.root, "priority reports")
	status, calls, _ := sourceGateRun(fixture, "make go-runtime-source-check", []string{"GO_RUNTIME_TEST_GROUP=release-handoff", "GO_RUNTIME_TEST_PROCS=2", "GO_RUNTIME_REPORT_DIR=" + reportsDir})
	Expect(status).To(Equal(0))
	tests := sourceGateTests(calls)
	Expect(tests).To(HaveLen(1))
	sourceGateAssertPartition(tests[0], "release-handoff")
	jsonPath := filepath.Join(reportsDir, "release-handoff.json")
	readReport := func() types.Report {
		data, err := os.ReadFile(jsonPath)
		Expect(err).NotTo(HaveOccurred())
		var reports []types.Report
		Expect(json.Unmarshal(data, &reports)).To(Succeed())
		Expect(reports).To(HaveLen(1))
		return reports[0]
	}
	replay := func(directory string, extraArgs, environment []string) ([]byte, error) {
		args := append([]string{}, tests[0].Args...)
		// Ginkgo metadata-only dry-runs require serial mode. Real receipt
		// executions below retain the observed two-worker Make operand.
		for _, extra := range extraArgs {
			if extra == "--dry-run" {
				for index, arg := range args {
					if strings.HasPrefix(arg, "--procs=") {
						args[index] = "--procs=1"
					}
				}
			}
		}
		for index, arg := range args {
			if arg == "./acceptance" {
				args = append(args[:index], append(extraArgs, args[index:]...)...)
				break
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		command := exec.CommandContext(ctx, "go", args...) // #nosec G204 -- observed Make runner replayed with fixed fixture/dry-run operands, without a shell.
		command.Dir = directory
		command.Env = append(os.Environ(), environment...)
		output, err := command.CombinedOutput()
		Expect(ctx.Err()).NotTo(HaveOccurred(), "finite native priority proof: %s", output)
		return output, err
	}
	observerPath := filepath.Join(fixture.root, "observer_test.go")
	writeFixture(observerPath, []byte(sourceGatePriorityObserver), 0600)
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	overlay, err := json.Marshal(map[string]any{"Replace": map[string]string{filepath.Join(repository, "acceptance/source_gate_priority_observer_test.go"): observerPath}})
	Expect(err).NotTo(HaveOccurred())
	overlayPath := filepath.Join(fixture.root, "observer-overlay.json")
	writeFixture(overlayPath, overlay, 0600)
	metadataPath := filepath.Join(fixture.root, "metadata.jsonl")
	output, err := replay("..", []string{"--dry-run"}, []string{"GOFLAGS=-overlay=" + overlayPath, "PRIORITY_METADATA_PATH=" + metadataPath})
	Expect(err).NotTo(HaveOccurred(), "real handoff priority metadata: %s", output)
	readMetadata := func() []sourceGatePriorityEntry {
		data, err := os.ReadFile(metadataPath)
		Expect(err).NotTo(HaveOccurred())
		entries := []sourceGatePriorityEntry{}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			var entry sourceGatePriorityEntry
			Expect(json.Unmarshal([]byte(line), &entry)).To(Succeed())
			entries = append(entries, entry)
		}
		return entries
	}
	type bucket struct {
		label, name     string
		count, priority int
	}
	buckets := []bucket{{"handoff-legacy-and-init", "HEAVY_A", 21, 2}, {"handoff-retention-safeguards", "HEAVY_B", 10, 2}, {"handoff-backup-retention", "MEDIUM", 8, 1}, {"handoff-go-recovery", "LIGHT", 5, 0}}
	priorities, counts := map[string]int{}, map[string]int{}
	identities := map[string]bool{}
	for _, spec := range readMetadata() {
		Expect(spec.Ordered).To(BeTrue(), "handoff fixture lifetime must stay Ordered: %s", spec.Text)
		Expect(identities).NotTo(HaveKey(spec.Text))
		identities[spec.Text] = true
		matched := 0
		for _, b := range buckets {
			for _, label := range spec.Labels {
				if label == b.label {
					matched++
					counts[b.name]++
					if value, ok := priorities[b.name]; ok {
						Expect(spec.Priority).To(Equal(value), "one whole group must have one priority")
					}
					priorities[b.name] = spec.Priority
				}
			}
		}
		Expect(matched).To(Equal(1), "each selected criterion must belong to exactly one known whole group")
	}
	Expect(identities).To(HaveLen(44))
	for _, b := range buckets {
		Expect(counts[b.name]).To(Equal(b.count))
	}
	_, _ = fmt.Fprintf(GinkgoWriter, "actual handoff ordered metadata: counts=%v priorities=%v exact44=%v\n", counts, priorities, identities)
	root := GinkgoT().TempDir()
	for _, path := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(filepath.Join("..", path))
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(root, path), data, 0600)
	}
	writeFixture(filepath.Join(root, "acceptance/priority_test.go"), []byte(sourceGatePrioritySuite), 0600)
	writeFixture(filepath.Join(root, "acceptance/observer_test.go"), []byte(strings.Replace(sourceGatePriorityObserver, "package acceptance_test", "package workerproof_test", 1)), 0600)
	for _, removed := range []bool{false, true} {
		receipts := GinkgoT().TempDir()
		Expect(os.Remove(metadataPath)).To(Succeed())
		environment := []string{"WORKER_PROOF_DIR=" + receipts, "PRIORITY_METADATA_PATH=" + metadataPath}
		for _, b := range buckets {
			value := priorities[b.name]
			if removed {
				value = 0
			}
			environment = append(environment, fmt.Sprintf("PRIORITY_%s=%d", b.name, value))
		}
		output, err := replay(root, []string{"--seed=1"}, environment)
		report := readReport()
		if removed {
			var exit *exec.ExitError
			Expect(errors.As(err, &exit)).To(BeTrue(), "removing priorities must make the real scheduling oracle fail: %s", output)
			Expect(report.SuiteSucceeded).To(BeFalse())
			Expect(string(output)).To(ContainSubstring("priority oracle:"))
			_, _ = fmt.Fprintf(GinkgoWriter, "removed-priority negative control: exit=%d output=%q\n", exit.ExitCode(), output)
			continue
		}
		Expect(err).NotTo(HaveOccurred(), "actual whole-group priority scheduling: %s", output)
		Expect(report.SuiteSucceeded).To(BeTrue())
		Expect(report.SuiteConfig.ParallelTotal).To(Equal(2))
		pids := map[string]string{}
		for _, b := range buckets {
			data, err := os.ReadFile(filepath.Join(receipts, "started-"+b.name))
			Expect(err).NotTo(HaveOccurred())
			pids[b.name] = string(data)
			for _, step := range []string{"first", "second"} {
				data, err := os.ReadFile(filepath.Join(receipts, "criterion-"+b.name+"-"+step))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(data)).To(Equal(pids[b.name]), "Ordered nodes must execute in their group-owned process")
			}
			Expect(priorities[b.name]).To(Equal(b.priority), "actual handoff metadata must select the reviewed coarse bucket")
		}
		Expect(pids["HEAVY_A"]).NotTo(Equal(pids["HEAVY_B"]), "both heavy groups must really execute on different workers")
		selected := 0
		for _, spec := range report.SpecReports {
			if spec.LeafNodeType == types.NodeTypeIt && !spec.State.Is(types.SpecStateSkipped) {
				selected++
				Expect(spec.State.Is(types.SpecStatePassed)).To(BeTrue())

			}
		}
		Expect(selected).To(Equal(8))
		metadata := readMetadata()
		Expect(metadata).To(HaveLen(8))
		for _, spec := range metadata {
			Expect(spec.Ordered).To(BeTrue())
		}
		paths, err := filepath.Glob(filepath.Join(receipts, "criterion-*"))
		Expect(err).NotTo(HaveOccurred())
		Expect(paths).To(HaveLen(8))
		_, _ = fmt.Fprintf(GinkgoWriter, "real two-worker coarse priority proof: PIDs=%v selected=%d output=%q\n", pids, selected, output)
	}
}

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
		// per docs/adr/0107-published-release-qualification.md:222
		sourceGateHandoffPriorityProof()
	})
})
