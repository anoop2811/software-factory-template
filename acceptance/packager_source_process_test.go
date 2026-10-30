package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/packaging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Real Git completes the selected operation before a real descendant retains
// its output descriptors. Counting forwards bytes without an in-memory archive.
// per docs/adr/0110-local-compiler-process-ownership.md:98
const packagerSourceProcessFixture = `package main
import("encoding/json";"io";"os";"os/exec";"path/filepath";"strconv";"syscall";"time")
type control struct{Real,Stage,Mode,Ready,Log string}
type counter struct{writer io.Writer;total int64}
func(c *counter)Write(p []byte)(int,error){n,e:=c.writer.Write(p);c.total+=int64(n);return n,e}
func main(){
 executable,e:=os.Executable();if e!=nil{os.Exit(10)}
 data,e:=os.ReadFile(filepath.Join(filepath.Dir(executable),"control.json"));if e!=nil{os.Exit(11)}
 var c control;if json.Unmarshal(data,&c)!=nil{os.Exit(12)}
 if len(os.Args)>1&&os.Args[1]=="--tail"{
  leader,_:=strconv.Atoi(os.Args[2]);group,_:=syscall.Getpgid(0);cwd,_:=os.Getwd()
  record,_:=json.Marshal(map[string]any{"PID":os.Getpid(),"Leader":leader,"Group":group,"Stage":os.Args[3],"Cwd":cwd,"Args":os.Args[4:]})
  temporary:=c.Ready+"."+strconv.Itoa(os.Getpid())
  if os.WriteFile(temporary,record,0600)!=nil||os.Rename(temporary,c.Ready)!=nil{os.Exit(13)}
  until:=time.Now().Add(30*time.Second)
  for time.Now().Before(until){if _,e:=os.Stat(c.Ready+".release");e==nil{return};time.Sleep(10*time.Millisecond)}
  return
 }
 stage:=""
 for i,arg:=range os.Args[1:]{switch arg{case "rev-parse":stage="probe";case "init":stage="init";case "archive":stage="archive";case "cat-file":if i+2<len(os.Args)&&os.Args[i+2]=="-t"{stage="type"}}}
 counted:=&counter{writer:os.Stdout}
 command:=exec.Command(c.Real,os.Args[1:]...);command.Stdin=os.Stdin;command.Stdout=counted;command.Stderr=os.Stderr
 if e:=command.Run();e!=nil{if exit,ok:=e.(*exec.ExitError);ok{os.Exit(exit.ExitCode())};os.Exit(14)}
 trace,_:=json.Marshal(map[string]any{"stage":stage,"stdout_bytes":counted.total,"real_completed":true})
 log,e:=os.OpenFile(c.Log,os.O_CREATE|os.O_WRONLY|os.O_APPEND,0600);if e!=nil{os.Exit(15)}
 if _,e=log.Write(append(trace,'\n'));e!=nil{os.Exit(16)};if log.Close()!=nil{os.Exit(17)}
 if c.Mode!="hold"||c.Stage!=stage{return}
 child:=exec.Command(executable,append([]string{"--tail",strconv.Itoa(os.Getpid()),stage},os.Args[1:]...)...)
 child.Stdout=os.Stdout;child.Stderr=os.Stderr
 if child.Run()!=nil{os.Exit(18)}
}
`

type packagerSourceControl struct{ Real, Stage, Mode, Ready, Log string }

func packagerSourceReadiness(ready, stage string) localCompilerReceipt {
	GinkgoHelper()
	var receipt localCompilerReceipt
	Eventually(func() bool {
		data, err := os.ReadFile(ready)
		return err == nil && json.Unmarshal(data, &receipt) == nil
	}, 15*time.Second).Should(BeTrue(), "real Git must complete before its descendant announces held capture")
	Expect(receipt.PID).To(BeNumerically(">", 1))
	Expect(receipt.Leader).To(BeNumerically(">", 1))
	Expect(receipt.Group).To(BeNumerically(">", 1))
	Expect(receipt.PID).NotTo(Equal(receipt.Leader))
	Expect(receipt.Stage).To(Equal(stage))
	Expect(receipt.Args).To(ContainElement(map[string]string{"probe": "rev-parse", "init": "init", "type": "cat-file", "archive": "archive"}[stage]))
	Expect(syscall.Kill(receipt.PID, 0)).To(Succeed())
	Expect(syscall.Kill(receipt.Leader, 0)).To(Succeed())
	AddReportEntry("packager-source.readiness", receipt, ReportEntryVisibilityNever)
	return receipt
}

var _ = Describe("V1 packager Git process ownership", Ordered, ContinueOnFailure, func() {
	var work, source, git, compiler, wrapper, revision, packager string
	var sequence int
	gitRun := func(args ...string) string {
		GinkgoHelper()
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, git, append([]string{"-c", "core.hooksPath=" + os.DevNull, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.com", "-c", "commit.gpgsign=false", "-C", source}, args...)...) // #nosec G204 -- resolved real Git and literal evaluator-owned setup operands.
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "GIT_") {
				command.Env = append(command.Env, entry)
			}
		}
		command.Env = append(command.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_NO_REPLACE_OBJECTS=1")
		data, err := command.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "%s", data)
		Expect(ctx.Err()).NotTo(HaveOccurred())
		return string(data)
	}
	BeforeAll(func() {
		var err error
		git, err = exec.LookPath("git")
		Expect(err).NotTo(HaveOccurred())
		git, err = filepath.EvalSymlinks(git)
		Expect(err).NotTo(HaveOccurred())
		git, err = filepath.Abs(git)
		Expect(err).NotTo(HaveOccurred())
		compiler, err = exec.LookPath("go")
		Expect(err).NotTo(HaveOccurred())
		compiler, err = filepath.EvalSymlinks(compiler)
		Expect(err).NotTo(HaveOccurred())
		compiler, err = filepath.Abs(compiler)
		Expect(err).NotTo(HaveOccurred())
		work, err = os.MkdirTemp("", "factory-packager-source-process-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, work)
		work, err = filepath.EvalSymlinks(work)
		Expect(err).NotTo(HaveOccurred())
		source = filepath.Join(work, "source with spaces")
		writeFixture(filepath.Join(source, "go.mod"), []byte("module example.com/packager-source-process\n\ngo 1.27.3\n"), 0600)
		writeFixture(filepath.Join(source, "cmd/factory/main.go"), []byte("package main\nfunc main(){}\n"), 0600)
		writeFixture(filepath.Join(source, "unrelated-large.bin"), make([]byte, 17<<20), 0600)
		gitRun("init", "--quiet", "--template=")
		gitRun("add", ".")
		gitRun("commit", "--quiet", "-m", "packager source process fixture")
		revision = strings.TrimSpace(gitRun("rev-parse", "HEAD"))
		writeFixture(filepath.Join(source, "untracked-user.txt"), []byte("preserve unrelated user work\n"), 0600)
		writeFixture(filepath.Join(work, "wrapper.go"), []byte(packagerSourceProcessFixture), 0600)
		wrapper = filepath.Join(work, "tools", "git")
		result := imageProcess(work, nil, compiler, "build", "-trimpath", "-buildvcs=false", "-o", wrapper, filepath.Join(work, "wrapper.go"))
		Expect(result.status).To(BeZero(), "%+v", result)
		project, err := filepath.Abs("..")
		Expect(err).NotTo(HaveOccurred())
		packager = filepath.Join(work, "factory-package")
		args := []string{"build", "-trimpath", "-buildvcs=false", "-o", packager, "./cmd/factory-package"}
		if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
			args = append(args[:1], append([]string{"-race"}, args[1:]...)...)
		}
		result = imageProcess(project, nil, compiler, args...)
		Expect(result.status).To(BeZero(), "%+v", result)
	})
	options := func(stage, mode string) (packaging.Options, string, string, string) {
		GinkgoHelper()
		sequence++
		area := filepath.Join(work, "case-"+strconv.Itoa(sequence))
		temporary := filepath.Join(area, "temporary")
		Expect(os.MkdirAll(temporary, 0700)).To(Succeed())
		GinkgoT().Setenv("TMPDIR", temporary)
		GinkgoT().Setenv("PATH", filepath.Dir(wrapper)+string(os.PathListSeparator)+os.Getenv("PATH"))
		ready, trace := filepath.Join(area, "ready.json"), filepath.Join(area, "git-operations.jsonl")
		body, err := json.Marshal(packagerSourceControl{git, stage, mode, ready, trace})
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(filepath.Dir(wrapper), "control.json"), body, 0600)
		return packaging.Options{Source: source, Revision: revision, Target: runtime.GOOS + "/" + runtime.GOARCH, Output: filepath.Join(area, "bundle"), Compiler: compiler}, ready, area, trace
	}
	assertCleanup := func(area string, receipt localCompilerReceipt, uncertain bool, diagnostic string) {
		GinkgoHelper()
		entries, err := os.ReadDir(filepath.Join(area, "temporary"))
		Expect(err).NotTo(HaveOccurred())
		if localCompilerAlive(receipt) {
			Expect(uncertain).To(BeTrue(), "a surviving Git descendant cannot count as confirmed cleanup")
		}
		if uncertain {
			Expect(entries).To(HaveLen(1))
			Expect(diagnostic).To(ContainSubstring(filepath.Join(area, "temporary", entries[0].Name())))
		} else {
			Expect(entries).To(BeEmpty())
		}
	}
	// per docs/adr/0110-local-compiler-process-ownership.md:92
	It("completes real Git and a V1 build with an unrelated archive larger than sixteen MiB", func() {
		option, _, area, trace := options("", "healthy")
		before := installationSnapshot(source)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		result, err := packaging.Build(ctx, option)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Archive).To(Equal(filepath.Join(option.Output, "factory-runtime.tar.gz")))
		data, err := os.ReadFile(result.Archive)
		Expect(err).NotTo(HaveOccurred())
		Expect(data).NotTo(BeEmpty())
		data, err = os.ReadFile(trace)
		Expect(err).NotTo(HaveOccurred())
		seen := map[string]bool{}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			var row struct {
				Stage     string `json:"stage"`
				Bytes     int64  `json:"stdout_bytes"`
				Completed bool   `json:"real_completed"`
			}
			Expect(json.Unmarshal([]byte(line), &row)).To(Succeed())
			Expect(row.Completed).To(BeTrue())
			seen[row.Stage] = true
			if row.Stage == "archive" {
				Expect(row.Bytes).To(BeNumerically(">", 16<<20))
				AddReportEntry("packager-source.archive-bytes", row.Bytes, ReportEntryVisibilityNever)
			}
		}
		Expect(seen).To(HaveKey("probe"))
		Expect(seen).To(HaveKey("init"))
		Expect(seen).To(HaveKey("type"))
		Expect(seen).To(HaveKey("archive"))
		Expect(installationSnapshot(source)).To(Equal(before))
		entries, err := os.ReadDir(filepath.Join(area, "temporary"))
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	})
	// per docs/adr/0110-local-compiler-process-ownership.md:98
	DescribeTable("cancels after a real Git stage and descendant readiness handshake", func(stage string) {
		option, ready, area, _ := options(stage, "hold")
		before := installationSnapshot(source)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		finished := make(chan localCompilerCompletion, 1)
		go func() { result, err := packaging.Build(ctx, option); finished <- localCompilerCompletion{result, err} }()
		receipt := packagerSourceReadiness(ready, stage)
		DeferCleanup(localCompilerCleanup, receipt)
		cancel()
		var completion localCompilerCompletion
		select {
		case completion = <-finished:
		case <-time.After(6 * time.Second):
			Expect(syscall.Kill(receipt.PID, 0)).To(Succeed())
			localCompilerCleanup(receipt)
			Eventually(finished, 6*time.Second).Should(Receive(&completion))
			Fail("canceled V1 Git operation exceeded bounded cleanup while its actual descendant retained capture")
		}
		Expect(errors.Is(completion.err, context.Canceled)).To(BeTrue(), "%v", completion.err)
		Expect(completion.result).To(Equal(packaging.Result{}))
		Expect(receipt.Group).To(Equal(receipt.Leader), "the actual Git leader and descendant must have their own process group")
		var ownership *native.OwnershipError
		uncertain := errors.As(completion.err, &ownership)
		if uncertain {
			Expect(ownership.ProcessPID).To(Equal(receipt.Leader))
		}
		assertCleanup(area, receipt, uncertain, completion.err.Error())
		_, err := os.Lstat(option.Output)
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(installationSnapshot(source)).To(Equal(before))
	}, Entry("object discovery", "probe"), Entry("private repository init", "init"), Entry("revision type", "type"), Entry("streamed archive", "archive"))
	// per docs/adr/0110-local-compiler-process-ownership.md:98
	DescribeTable("handles parent-only signals while an actual Git descendant retains output", func(stage string, signal syscall.Signal) {
		option, ready, area, _ := options(stage, "hold")
		before := installationSnapshot(source)
		args := []string{"--source", source, "--revision", revision, "--target", option.Target, "--output", option.Output, "--go", compiler}
		command := exec.Command(packager, args...) // #nosec G204 -- evaluator-owned compiled packager and literal committed-source operands.
		command.Dir = area
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		Expect(command.Start()).To(Succeed())
		finished := make(chan error, 1)
		go func() { finished <- command.Wait() }()
		DeferCleanup(func() { _ = command.Process.Kill() })
		receipt := packagerSourceReadiness(ready, stage)
		DeferCleanup(localCompilerCleanup, receipt)
		Expect(command.Process.Signal(signal)).To(Succeed())
		var returned error
		select {
		case returned = <-finished:
		case <-time.After(6 * time.Second):
			Expect(syscall.Kill(receipt.PID, 0)).To(Succeed())
			localCompilerCleanup(receipt)
			Eventually(finished, 6*time.Second).Should(Receive(&returned))
			Fail("parent-only signal left V1 packager waiting on actual Git descendant capture")
		}
		Expect(returned).To(HaveOccurred())
		Expect(command.ProcessState.ExitCode()).To(Equal(1), "%s", stderr.String())
		Expect(stdout.String()).To(BeEmpty())
		Expect(receipt.Group).To(Equal(receipt.Leader))
		uncertain := strings.Contains(stderr.String(), "ownership is unconfirmed")
		assertCleanup(area, receipt, uncertain, stderr.String())
		_, err := os.Lstat(option.Output)
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(installationSnapshot(source)).To(Equal(before))
	}, Entry("SIGINT during object discovery", "probe", syscall.SIGINT), Entry("SIGTERM during streamed archive", "archive", syscall.SIGTERM))
})
