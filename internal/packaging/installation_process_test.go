package packaging_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/packaging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Test-owned trusted Git wrapper: actual Git completes before a descendant is
// launched. Its handshake contains actual OS identity and observed Git output.
// A finite safety lifetime exceeds the production five-second cleanup allowance.
// per docs/adr/0097-installation-source-image-bundles.md:215
const installationGitWrapperFixture = `package main
import("bytes";"encoding/json";"os";"os/exec";"strconv";"syscall";"time")
func main(){
 if len(os.Args)>1 && os.Args[1]=="--owned-tail" {
  leader,_:=strconv.Atoi(os.Args[2]);group,_:=syscall.Getpgid(0)
  data,_:=json.Marshal(map[string]any{"pid":os.Getpid(),"leader":leader,"group":group,"git_output":os.Args[3]})
  ready:=os.Getenv("FACTORY_IMAGE_PROCESS_READY");tmp:=ready+".tmp"
  if os.WriteFile(tmp,data,0600)!=nil||os.Rename(tmp,ready)!=nil{os.Exit(8)}
  until:=time.Now().Add(20*time.Second)
  for time.Now().Before(until) {if _,err:=os.Stat(ready+".release");err==nil{return};time.Sleep(10*time.Millisecond)}
  return
 }
 command:=exec.Command(os.Getenv("FACTORY_IMAGE_REAL_GIT"),os.Args[1:]...)
 command.Stdin=os.Stdin;command.Stderr=os.Stderr
 var captured bytes.Buffer;trigger:=false
 for i,arg:=range os.Args {if arg=="cat-file"&&i+1<len(os.Args)&&os.Args[i+1]=="-t"{trigger=true}}
 if trigger {command.Stdout=&captured}else{command.Stdout=os.Stdout}
 if err:=command.Run();err!=nil{if e,ok:=err.(*exec.ExitError);ok{os.Exit(e.ExitCode())};os.Exit(7)}
 if !trigger{return}
 if _,err:=os.Stdout.Write(captured.Bytes());err!=nil{os.Exit(6)}
 mode:=os.Getenv("FACTORY_IMAGE_PROCESS_MODE");if mode=="none"{return}
 if captured.String()!="commit\n"{os.Exit(5)}
 child:=exec.Command(os.Args[0],"--owned-tail",strconv.Itoa(os.Getpid()),captured.String())
 child.Env=os.Environ()
 if mode=="stdout"{child.Stdout=os.Stdout};if mode=="stderr"{child.Stderr=os.Stderr}
 if err:=child.Start();err!=nil{os.Exit(4)}
 if err:=child.Process.Release();err!=nil{os.Exit(3)}
 ready:=os.Getenv("FACTORY_IMAGE_PROCESS_READY");until:=time.Now().Add(20*time.Second)
 for time.Now().Before(until){if _,err:=os.Stat(ready+".release");err==nil{return};time.Sleep(10*time.Millisecond)}
}
`

type installationTailObservation struct {
	PID       int    `json:"pid"`
	Leader    int    `json:"leader"`
	Group     int    `json:"group"`
	GitOutput string `json:"git_output"`
}

var _ = Describe("Installation Git process ownership", Ordered, ContinueOnFailure, func() {
	var work, source, git, wrapper, compiler, revision string
	var control packaging.Options
	write := func(name string, data []byte, mode os.FileMode) {
		GinkgoHelper()
		Expect(os.MkdirAll(filepath.Dir(name), 0700)).To(Succeed())
		Expect(os.WriteFile(name, data, mode)).To(Succeed())
	}
	gitRun := func(dir string, args ...string) string {
		GinkgoHelper()
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, git, append([]string{"--no-replace-objects", "-c", "core.hooksPath=" + os.DevNull, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.com", "-c", "commit.gpgsign=false", "-C", dir}, args...)...) // #nosec G204 -- resolved native Git and test-owned immutable fixture operands.
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "GIT_") {
				command.Env = append(command.Env, entry)
			}
		}
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
		goExecutable, err := exec.LookPath("go")
		Expect(err).NotTo(HaveOccurred())
		goRootQuery := exec.Command(goExecutable, "env", "GOROOT") // #nosec G204 -- local compiler discovery before injecting the test Git wrapper.
		goRoot, err := goRootQuery.Output()
		Expect(err).NotTo(HaveOccurred())
		compiler = filepath.Join(strings.TrimSpace(string(goRoot)), "bin", "go")
		work, err = os.MkdirTemp("", "installation-git-ownership-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, work)
		work, err = filepath.EvalSymlinks(work)
		Expect(err).NotTo(HaveOccurred())
		source = filepath.Join(work, "source")
		write(filepath.Join(source, "go.mod"), []byte("module example.com/ownership-fixture\n\ngo 1.27.1\n"), 0600)
		write(filepath.Join(source, "cmd/factory/main.go"), []byte("package main\nfunc main() {}\n"), 0600)
		gitRun(source, "init", "--quiet", "--template=")
		objects := strings.TrimSpace(gitRun("../..", "rev-parse", "--path-format=absolute", "--git-path", "objects"))
		write(filepath.Join(source, ".git/objects/info/alternates"), []byte(objects+"\n"), 0600)
		gitRun(source, "add", ".")
		gitRun(source, "commit", "--quiet", "-m", "owned fixture")
		revision = strings.TrimSpace(gitRun(source, "rev-parse", "HEAD"))
		write(filepath.Join(source, "user-untracked.txt"), []byte("preserve user work\n"), 0600)
		write(filepath.Join(work, "wrapper.go"), []byte(installationGitWrapperFixture), 0600)
		wrapper = filepath.Join(work, "tools", "git")
		Expect(os.MkdirAll(filepath.Dir(wrapper), 0700)).To(Succeed())
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, compiler, "build", "-trimpath", "-buildvcs=false", "-o", wrapper, filepath.Join(work, "wrapper.go")) // #nosec G204 -- qualified explicit Go compiler and evaluator-only native fixture.
		for _, entry := range os.Environ() {
			key, _, _ := strings.Cut(entry, "=")
			if key != "GOFLAGS" && key != "GOWORK" && key != "GOTOOLCHAIN" {
				command.Env = append(command.Env, entry)
			}
		}
		command.Env = append(command.Env, "GOFLAGS=", "GOWORK=off", "GOTOOLCHAIN=local")
		data, err := command.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "%s", data)
		Expect(ctx.Err()).NotTo(HaveOccurred())
		control = packaging.Options{Source: source, Revision: revision, Target: runtime.GOOS + "/" + runtime.GOARCH, Output: filepath.Join(work, "healthy"), Version: "v1.2.3", Compiler: compiler, Installation: true}
	})
	BeforeEach(func() {
		GinkgoT().Setenv("PATH", filepath.Dir(wrapper)+string(os.PathListSeparator)+os.Getenv("PATH"))
		GinkgoT().Setenv("FACTORY_IMAGE_REAL_GIT", git)
	})

	// This uses the exact same compiled wrapper and committed source without a
	// descendant, proving the control completes real Git, collection and build.
	// per docs/adr/0097-installation-source-image-bundles.md:216
	It("completes the same real Git wrapper without a descendant", func() {
		GinkgoT().Setenv("FACTORY_IMAGE_PROCESS_MODE", "none")
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		result, err := packaging.Build(ctx, control)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Revision).To(Equal(revision))
		Expect(result.Archive).To(Equal(filepath.Join(control.Output, "factory-runtime.tar.gz")))
		data, err := os.ReadFile(result.Archive)
		Expect(err).NotTo(HaveOccurred())
		Expect(data).NotTo(BeEmpty())
	})

	// per docs/adr/0097-installation-source-image-bundles.md:193
	// per docs/adr/0097-installation-source-image-bundles.md:202
	// per docs/adr/0097-installation-source-image-bundles.md:215
	DescribeTable("cancels only after actual Git and descendant handshake and preserves owned process evidence", func(mode string) {
		ready := filepath.Join(work, "tail-"+mode+".json")
		GinkgoT().Setenv("FACTORY_IMAGE_PROCESS_MODE", mode)
		GinkgoT().Setenv("FACTORY_IMAGE_PROCESS_READY", ready)
		options := control
		options.Output = filepath.Join(work, "cancelled-"+mode)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		type completed struct {
			result packaging.Result
			err    error
		}
		finished := make(chan completed, 1)
		go func() {
			result, err := packaging.Build(ctx, options)
			finished <- completed{result, err}
		}()
		var tail installationTailObservation
		Eventually(func() bool {
			data, err := os.ReadFile(ready)
			return err == nil && json.Unmarshal(data, &tail) == nil
		}, 10*time.Second).Should(BeTrue(), "the real native Git operation must complete before cancellation")
		Expect(tail.GitOutput).To(Equal("commit\n"))
		Expect(tail.PID).To(BeNumerically(">", 0))
		Expect(tail.Leader).To(BeNumerically(">", 0))
		Expect(tail.PID).NotTo(Equal(tail.Leader))
		Expect(syscall.Kill(tail.PID, 0)).To(Succeed(), "observe the real live controlled descendant")
		cleanupTail := func() {
			GinkgoHelper()
			group, err := syscall.Getpgid(tail.PID)
			if errors.Is(err, syscall.ESRCH) {
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(group).To(Equal(tail.Group), "refuse to signal a different process identity")
			err = syscall.Kill(tail.PID, syscall.SIGKILL)
			Expect(err == nil || errors.Is(err, syscall.ESRCH)).To(BeTrue())
		}
		DeferCleanup(cleanupTail)
		select {
		case premature := <-finished:
			Fail(fmt.Sprintf("Build finished before the observed cancellation: %v", premature.err))
		default:
		}
		cancelledAt := time.Now()
		cancel()
		var completion completed
		select {
		case completion = <-finished:
		case <-time.After(6 * time.Second):
			// The existing five-second cleanup plus scheduling allowance elapsed.
			// Release only our identified tail after observing the retained pipe.
			Expect(syscall.Kill(tail.PID, 0)).To(Succeed())
			cleanupTail()
			Eventually(finished, 6*time.Second).Should(Receive(&completion))
			Fail("Build exceeded its existing five-second ownership cleanup allowance while the actual Git descendant retained capture pipes")
		}
		Expect(time.Since(cancelledAt)).To(BeNumerically("<=", 6*time.Second))
		Expect(errors.Is(completion.err, context.Canceled)).To(BeTrue(), "preserve the original cancellation")
		Expect(completion.result).To(Equal(packaging.Result{}))
		_, err := os.Lstat(options.Output)
		Expect(os.IsNotExist(err)).To(BeTrue(), "cancelled or uncertain collection must not publish")
		alive := syscall.Kill(tail.PID, 0) == nil
		if alive {
			var ownership *native.OwnershipError
			Expect(errors.As(completion.err, &ownership)).To(BeTrue(), "a real surviving descendant requires typed retained ownership uncertainty")
			Expect(ownership.ProcessPID).To(Equal(tail.Leader))
		}
		contents, err := os.ReadFile(filepath.Join(source, "user-untracked.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(contents)).To(Equal("preserve user work\n"))
		Expect(strings.TrimSpace(gitRun(source, "rev-parse", "HEAD"))).To(Equal(revision))
	}, Entry("stdout held by descendant", "stdout"), Entry("stderr held by descendant", "stderr"), Entry("descendant with closed capture pipes", "closed"))
})
