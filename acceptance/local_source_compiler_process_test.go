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

// The same native compiler wrapper delegates healthy operations to the resolved
// real compiler and atomically publishes an actual descendant readiness receipt.
// per docs/adr/0110-local-compiler-process-ownership.md:44
const localCompilerProcessFixture = `package main
import("encoding/json";"fmt";"os";"os/exec";"path/filepath";"strconv";"strings";"syscall";"time")
type control struct{Real,Stage,Mode,Ready,Stream string}
func main(){
 executable,err:=os.Executable();if err!=nil{os.Exit(10)}
 data,err:=os.ReadFile(filepath.Join(filepath.Dir(executable),"control.json"));if err!=nil{os.Exit(11)}
 var c control;if json.Unmarshal(data,&c)!=nil{os.Exit(12)}
 if len(os.Args)>1&&os.Args[1]=="--tail"{
  leader,_:=strconv.Atoi(os.Args[2]);group,_:=syscall.Getpgid(0);cwd,_:=os.Getwd()
  receipt,_:=json.Marshal(map[string]any{"pid":os.Getpid(),"leader":leader,"group":group,"stage":c.Stage,"cwd":cwd,"args":os.Args[3:]})
  temporary:=c.Ready+"."+strconv.Itoa(os.Getpid())
  if os.WriteFile(temporary,receipt,0600)!=nil||os.Rename(temporary,c.Ready)!=nil{os.Exit(13)}
  until:=time.Now().Add(30*time.Second)
  for time.Now().Before(until){if _,err:=os.Stat(c.Ready+".release");err==nil{return};time.Sleep(10*time.Millisecond)}
  return
 }
 stage:="";if len(os.Args)>1{stage=os.Args[1]}
 if stage==c.Stage&&c.Mode=="hold"{
  child:=exec.Command(executable,append([]string{"--tail",strconv.Itoa(os.Getpid())},os.Args[1:]...)...)
  if c.Stream=="stdout"||c.Stream=="both"{child.Stdout=os.Stdout};if c.Stream=="stderr"||c.Stream=="both"{child.Stderr=os.Stderr}
  if child.Run()!=nil{os.Exit(14)};return
 }
 if stage==c.Stage&&c.Mode=="overflow"{
  switch stage{
  case "version":fmt.Print("go version go1.27.3 fixture/fixture\n"+strings.Repeat(" ",64<<10));return
  case "mod":if c.Stream=="stderr"{fmt.Print("{}\n");fmt.Fprint(os.Stderr,strings.Repeat("x",(64<<10)+1))}else{fmt.Print("{}\n"+strings.Repeat(" ",1<<20))};return
  case "build":fmt.Fprint(os.Stderr,strings.Repeat("x",(16<<20)+1))
  }
 }
 if stage=="mod"&&c.Mode=="healthy"{fmt.Fprintln(os.Stderr,"fixture module warning, separate from JSON")}
 command:=exec.Command(c.Real,os.Args[1:]...);command.Stdin=os.Stdin;command.Stdout=os.Stdout;command.Stderr=os.Stderr
 if err:=command.Run();err!=nil{if exit,ok:=err.(*exec.ExitError);ok{os.Exit(exit.ExitCode())};os.Exit(15)}
}
`

type localCompilerControl struct{ Real, Stage, Mode, Ready, Stream string }
type localCompilerReceipt struct {
	PID, Leader, Group int
	Stage, Cwd         string
	Args               []string
}
type localCompilerCompletion struct {
	result packaging.Result
	err    error
}

func localCompilerReadiness(ready, stage string, bound time.Duration) localCompilerReceipt {
	GinkgoHelper()
	var receipt localCompilerReceipt
	Eventually(func() bool {
		data, err := os.ReadFile(ready)
		return err == nil && json.Unmarshal(data, &receipt) == nil
	}, bound).Should(BeTrue(), "cancel only after the compiler descendant has retained actual capture descriptors")
	Expect(receipt.PID).To(BeNumerically(">", 1))
	Expect(receipt.Leader).To(BeNumerically(">", 1))
	Expect(receipt.Group).To(BeNumerically(">", 1))
	Expect(receipt.PID).NotTo(Equal(receipt.Leader))
	Expect(receipt.Stage).To(Equal(stage))
	Expect(receipt.Args[0]).To(Equal(stage))
	Expect(syscall.Kill(receipt.PID, 0)).To(Succeed())
	Expect(syscall.Kill(receipt.Leader, 0)).To(Succeed())
	AddReportEntry("compiler.readiness", receipt, ReportEntryVisibilityNever)
	return receipt
}

func localCompilerCleanup(receipt localCompilerReceipt) {
	GinkgoHelper()
	for _, pid := range []int{receipt.PID, receipt.Leader} {
		group, err := syscall.Getpgid(pid)
		if errors.Is(err, syscall.ESRCH) {
			continue
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(group).To(Equal(receipt.Group), "never signal an identity outside the observed fixture group")
		err = syscall.Kill(pid, syscall.SIGKILL)
		Expect(err == nil || errors.Is(err, syscall.ESRCH)).To(BeTrue())
	}
}

func localCompilerAlive(receipt localCompilerReceipt) bool {
	return syscall.Kill(receipt.PID, 0) == nil || syscall.Kill(receipt.Leader, 0) == nil
}

var _ = Describe("Local source compiler process ownership", Ordered, ContinueOnFailure, func() {
	var work, source, compiler, wrapper, revision, packager string
	var sequence int
	git := func(args ...string) string {
		GinkgoHelper()
		result := imageProcess(source, nil, "git", append([]string{"-c", "core.hooksPath=" + os.DevNull}, args...)...)
		Expect(result.status).To(BeZero(), "%+v", result)
		return result.stdout
	}
	BeforeAll(func() {
		var err error
		work, err = os.MkdirTemp("", "factory-local-compiler-ownership-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, work)
		work, err = filepath.EvalSymlinks(work)
		Expect(err).NotTo(HaveOccurred())
		compiler, err = exec.LookPath("go")
		Expect(err).NotTo(HaveOccurred())
		compiler, err = filepath.EvalSymlinks(compiler)
		Expect(err).NotTo(HaveOccurred())
		compiler, err = filepath.Abs(compiler)
		Expect(err).NotTo(HaveOccurred())
		source = filepath.Join(work, "source")
		writeFixture(filepath.Join(source, "go.mod"), []byte("module example.com/compiler-ownership\n\ngo 1.27.3\n"), 0600)
		writeFixture(filepath.Join(source, "cmd/factory/main.go"), []byte("package main\nfunc main(){}\n"), 0600)
		git("init", "--quiet", "--template=")
		git("add", ".")
		git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.com", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "compiler ownership fixture")
		revision = strings.TrimSpace(git("rev-parse", "HEAD"))
		objects := imageProcess("..", nil, "git", "rev-parse", "--path-format=absolute", "--git-path", "objects")
		Expect(objects.status).To(BeZero(), "%+v", objects)
		writeFixture(filepath.Join(source, ".git/objects/info/alternates"), []byte(strings.TrimSpace(objects.stdout)+"\n"), 0600)
		wrapper = filepath.Join(work, "tools", "go")
		writeFixture(filepath.Join(work, "wrapper.go"), []byte(localCompilerProcessFixture), 0600)
		result := imageProcess(work, nil, compiler, "build", "-trimpath", "-buildvcs=false", "-o", wrapper, filepath.Join(work, "wrapper.go"))
		Expect(result.status).To(BeZero(), "%+v", result)
		project, err := filepath.Abs("..")
		Expect(err).NotTo(HaveOccurred())
		packager = filepath.Join(work, "factory-package")
		arguments := []string{"build", "-trimpath", "-buildvcs=false", "-o", packager, "./cmd/factory-package"}
		if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
			arguments = append(arguments[:1], append([]string{"-race"}, arguments[1:]...)...)
		}
		result = imageProcess(project, nil, compiler, arguments...)
		Expect(result.status).To(BeZero(), "%+v", result)
	})
	options := func(stage, mode, stream string) (packaging.Options, string, string) {
		GinkgoHelper()
		sequence++
		area := filepath.Join(work, "case-"+strconv.Itoa(sequence))
		Expect(os.MkdirAll(filepath.Join(area, "temporary"), 0700)).To(Succeed())
		GinkgoT().Setenv("TMPDIR", filepath.Join(area, "temporary"))
		ready := filepath.Join(area, "ready.json")
		data, err := json.Marshal(localCompilerControl{compiler, stage, mode, ready, stream})
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(filepath.Dir(wrapper), "control.json"), data, 0600)
		return packaging.Options{Source: source, Revision: revision, Target: runtime.GOOS + "/" + runtime.GOARCH, Output: filepath.Join(area, "bundle"), Compiler: wrapper}, ready, area
	}
	// per docs/adr/0110-local-compiler-process-ownership.md:33
	// per docs/adr/0110-local-compiler-process-ownership.md:47
	It("completes real compilation through the same wrapper and preserves an existing bundle", func() {
		option, _, area := options("", "healthy", "")
		before := installationSnapshot(source)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := packaging.Build(ctx, option)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Archive).To(Equal(filepath.Join(option.Output, "factory-runtime.tar.gz")))
		archive, err := os.ReadFile(result.Archive)
		Expect(err).NotTo(HaveOccurred())
		Expect(archive).NotTo(BeEmpty())
		published := installationSnapshot(option.Output)
		_, err = packaging.Build(ctx, option)
		Expect(err).To(HaveOccurred())
		Expect(installationSnapshot(option.Output)).To(Equal(published))
		Expect(installationSnapshot(source)).To(Equal(before))
		entries, err := os.ReadDir(filepath.Join(area, "temporary"))
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	})
	// per docs/adr/0110-local-compiler-process-ownership.md:19
	// per docs/adr/0110-local-compiler-process-ownership.md:27
	// per docs/adr/0110-local-compiler-process-ownership.md:50
	DescribeTable("cancels the actual compiler stage without waiting for inherited capture", func(stage, stream string) {
		option, ready, area := options(stage, "hold", stream)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		finished := make(chan localCompilerCompletion, 1)
		go func() {
			result, err := packaging.Build(ctx, option)
			finished <- localCompilerCompletion{result, err}
		}()
		receipt := localCompilerReadiness(ready, stage, 15*time.Second)
		DeferCleanup(localCompilerCleanup, receipt)
		cancel()
		var completion localCompilerCompletion
		select {
		case completion = <-finished:
		case <-time.After(6 * time.Second):
			Expect(syscall.Kill(receipt.PID, 0)).To(Succeed())
			localCompilerCleanup(receipt)
			Eventually(finished, 6*time.Second).Should(Receive(&completion))
			Fail("cancelled compiler exceeded the existing five-second cleanup allowance while its actual descendant retained capture")
		}
		Expect(errors.Is(completion.err, context.Canceled)).To(BeTrue(), "%v", completion.err)
		Expect(completion.result).To(Equal(packaging.Result{}))
		_, err := os.Lstat(option.Output)
		Expect(os.IsNotExist(err)).To(BeTrue())
		var ownership *native.OwnershipError
		uncertain := errors.As(completion.err, &ownership)
		if localCompilerAlive(receipt) {
			Expect(uncertain).To(BeTrue(), "a surviving descendant cannot be reported as confirmed cleanup")
		}
		entries, err := os.ReadDir(filepath.Join(area, "temporary"))
		Expect(err).NotTo(HaveOccurred())
		if uncertain {
			Expect(ownership.ProcessPID).To(Equal(receipt.Leader))
			Expect(entries).To(HaveLen(1), "uncertain ownership retains the private compiler workspace")
			Expect(completion.err.Error()).To(ContainSubstring(filepath.Join(area, "temporary", entries[0].Name())))
		} else {
			Expect(entries).To(BeEmpty(), "confirmed cleanup removes only the private workspace")
		}
		Expect(strings.TrimSpace(git("rev-parse", "HEAD"))).To(Equal(revision))
		Expect(git("status", "--porcelain", "--untracked-files=all")).To(BeEmpty())
	}, Entry("version stdout", "version", "stdout"), Entry("module inspection stderr", "mod", "stderr"), Entry("build both captures", "build", "both"))
	// per docs/adr/0110-local-compiler-process-ownership.md:23
	DescribeTable("refuses successful compiler operations whose capture exceeds its bound", func(stage, stream string) {
		option, _, _ := options(stage, "overflow", stream)
		before := installationSnapshot(source)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := packaging.Build(ctx, option)
		Expect(err).To(HaveOccurred(), "overflowing %s must not become a usable truncated response", stage)
		Expect(err.Error()).To(ContainSubstring("limit"))
		Expect(result).To(Equal(packaging.Result{}))
		_, err = os.Lstat(option.Output)
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(installationSnapshot(source)).To(Equal(before))
	}, Entry("version above 64 KiB", "version", "stdout"), Entry("module JSON above 1 MiB", "mod", "stdout"), Entry("module stderr above independent 64 KiB", "mod", "stderr"), Entry("build diagnostics above 16 MiB", "build", "stderr"))
	// per docs/adr/0110-local-compiler-process-ownership.md:36
	// per docs/adr/0110-local-compiler-process-ownership.md:44
	// per docs/adr/0110-local-compiler-process-ownership.md:50
	DescribeTable("handles parent-only signals while an actual compiler descendant holds capture", func(stage string, signal syscall.Signal) {
		option, ready, area := options(stage, "hold", "both")
		arguments := []string{"--source", source, "--revision", revision, "--target", option.Target, "--output", option.Output, "--go", wrapper}
		before := installationSnapshot(source)
		command := exec.Command(packager, arguments...) // #nosec G204 -- evaluator-owned native executables and literal source/target operands.
		command.Dir = area
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "PATH=") {
				command.Env = append(command.Env, entry)
			}
		}
		command.Env = append(command.Env, "PATH="+filepath.Dir(wrapper)+string(os.PathListSeparator)+os.Getenv("PATH"))
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		Expect(command.Start()).To(Succeed())
		finished := make(chan error, 1)
		go func() { finished <- command.Wait() }()
		DeferCleanup(func() { _ = command.Process.Kill() })
		receipt := localCompilerReadiness(ready, stage, 90*time.Second)
		DeferCleanup(localCompilerCleanup, receipt)
		Expect(command.Process.Signal(signal)).To(Succeed(), "signal only the real CLI parent, never its process group")
		var returned error
		select {
		case returned = <-finished:
		case <-time.After(6 * time.Second):
			Expect(syscall.Kill(receipt.PID, 0)).To(Succeed())
			localCompilerCleanup(receipt)
			_ = command.Process.Kill()
			Eventually(finished, 6*time.Second).Should(Receive(&returned))
			Fail("parent-only signal left the CLI blocked on its actual compiler descendant's capture")
		}
		Expect(returned).To(HaveOccurred())
		Expect(command.ProcessState.ExitCode()).To(Equal(1), "handled cancellation retains ordinary operational-failure exit behavior: %s", stderr.String())
		Expect(stdout.String()).To(BeEmpty(), "cancelled builds cannot publish a result")
		uncertain := strings.Contains(stderr.String(), "ownership is unconfirmed")
		if localCompilerAlive(receipt) || uncertain {
			Expect(stderr.String()).To(ContainSubstring("ownership is unconfirmed"))
			entries, err := os.ReadDir(filepath.Join(area, "temporary"))
			Expect(err).NotTo(HaveOccurred())
			var retained []string
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "factory-runtime-package-") {
					retained = append(retained, filepath.Join(area, "temporary", entry.Name()))
				}
			}
			Expect(retained).To(HaveLen(1))
			Expect(stderr.String()).To(ContainSubstring(retained[0]))
		} else {
			entries, err := os.ReadDir(filepath.Join(area, "temporary"))
			Expect(err).NotTo(HaveOccurred())
			for _, entry := range entries {
				Expect(entry.Name()).NotTo(HavePrefix("factory-runtime-package-"), "confirmed compiler cleanup removes its workspace")
			}
		}
		_, err := os.Lstat(option.Output)
		Expect(os.IsNotExist(err)).To(BeTrue(), "signals cannot publish a bundle")
		Expect(installationSnapshot(source)).To(Equal(before))
	}, Entry("standalone packager SIGINT during module inspection", "mod", syscall.SIGINT), Entry("standalone packager SIGTERM during build", "build", syscall.SIGTERM))
})
