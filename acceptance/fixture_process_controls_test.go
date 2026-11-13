package acceptance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// An independent child owns a real nested group. Its READY file is published
// only after signal handling and the nested child are ready, avoiding sleeps as
// an admission oracle. per docs/adr/0151-reliable-ci-qualification.md:139
const cooperativeControlSource = `package main
import (
 "encoding/json"
 "fmt"
 "io"
 "os"
 "os/exec"
 "os/signal"
 "strconv"
 "syscall"
 "time"
)
func must(err error){if err!=nil{panic(err)}}
func main(){
 if len(os.Args)>1 && os.Args[1]=="leaf" {
  signal.Ignore(syscall.SIGTERM)
  must(os.WriteFile(os.Getenv("LEAF_READY"),[]byte(strconv.Itoa(os.Getpid())),0600))
  for {time.Sleep(time.Hour)}
 }
 mode:=os.Getenv("CONTROL_MODE")
 if mode=="normal" || mode=="nonzero" {
  input,err:=io.ReadAll(os.Stdin);must(err)
  cwd,err:=os.Getwd();must(err)
  must(json.NewEncoder(os.Stdout).Encode(map[string]any{"argv":os.Args[1:],"input":string(input),"cwd":cwd,"marker":os.Getenv("CONTROL_MARKER")}))
  fmt.Fprintln(os.Stderr,"independent stderr")
  if mode=="nonzero" {os.Exit(23)}
  return
 }
 signals:=make(chan os.Signal,1)
 if mode=="forced" {signal.Ignore(syscall.SIGTERM)} else {signal.Notify(signals,syscall.SIGTERM)}
 if mode=="forced" || mode=="inherited" {
  must(os.WriteFile(os.Getenv("CONTROL_READY"),[]byte(fmt.Sprintf("%d 0",os.Getpid())),0600))
  for {
   if mode=="inherited" {if _,err:=os.Stat(os.Getenv("CONTROL_RELEASE"));err==nil{return}}
   time.Sleep(10*time.Millisecond)
  }
 }
 child:=exec.Command(os.Args[0],"leaf")
 child.Env=os.Environ()
 child.Stdin,child.Stdout,child.Stderr=os.Stdin,os.Stdout,os.Stderr
 child.SysProcAttr=&syscall.SysProcAttr{Setpgid:true}
 must(child.Start())
 reaped:=false
 defer func(){if !reaped{_=syscall.Kill(-child.Process.Pid,syscall.SIGKILL);_=child.Wait()}}()
 deadline:=time.Now().Add(10*time.Second)
 for {if _,err:=os.Stat(os.Getenv("LEAF_READY"));err==nil{break};if time.Now().After(deadline){panic("leaf readiness timeout")};time.Sleep(10*time.Millisecond)}
 must(os.WriteFile(os.Getenv("CONTROL_READY"),[]byte(fmt.Sprintf("%d %d",os.Getpid(),child.Process.Pid)),0600))
 <-signals
 must(syscall.Kill(-child.Process.Pid,syscall.SIGKILL))
 _=child.Wait()
 reaped=true
 must(os.WriteFile(os.Getenv("CONTROL_CLEANED"),[]byte("nested child reaped\n"),0600))
}
`

func cooperativeControlReport(name string) map[string]any {
	GinkgoHelper()
	for _, entry := range CurrentSpecReport().ReportEntries {
		if entry.Name == name {
			value, ok := entry.GetRawValue().(map[string]any)
			Expect(ok).To(BeTrue())
			return value
		}
	}
	Fail("missing lifecycle report: " + name)
	return nil
}

// Controls reclaim their own intentionally retained inputs only after observing
// the real retention decision and terminating any deliberately orphaned child.
// per docs/adr/0151-reliable-ci-qualification.md:141
func cooperativeControlRoots() (string, string) {
	GinkgoHelper()
	roots := []string{}
	for range 2 {
		root, err := os.MkdirTemp("", "factory-fixture-custody-control-")
		Expect(err).NotTo(HaveOccurred())
		root, err = filepath.EvalSymlinks(root)
		Expect(err).NotTo(HaveOccurred())
		roots = append(roots, root)
		writeFixture(filepath.Join(root, "sentinel"), []byte("exact borrowed bytes\n"), 0600)
	}
	DeferCleanup(func() {
		for _, root := range roots {
			delete(fixtureCleanupHolds, root)
			Expect(os.RemoveAll(root)).To(Succeed())
		}
	})
	return roots[0], roots[1]
}

var _ = Describe("Cooperative acceptance fixture lifecycle", Ordered, func() {
	var program string
	BeforeAll(func() {
		base, err := os.MkdirTemp("", "factory-fixture-process-controls-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupFixtureDirectory, base)
		program = filepath.Join(base, "independent-child")
		source := filepath.Join(base, "main.go")
		writeFixture(source, []byte(cooperativeControlSource), 0600)
		environment := map[string]string{}
		for _, entry := range os.Environ() {
			key, value, _ := strings.Cut(entry, "=")
			environment[key] = value
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		built, err := native.ExecuteCommand(ctx, base, []string{"go", "build", "-o", program, source}, environment, 90*time.Second)
		if err != nil || built.OwnershipUnconfirmed || !built.ExitConfirmed {
			holdFixtureCleanup(base, "independent control compiler ownership unconfirmed")
		}
		Expect(err).NotTo(HaveOccurred(), "%s", built.Stderr)
		Expect(ctx.Err()).NotTo(HaveOccurred())
		Expect(built.Outcome).To(Equal("completed"), "%s", built.Stderr)
		Expect(built.ExitCode).NotTo(BeNil())
		Expect(*built.ExitCode).To(BeZero())
	})

	command := func(root, mode string) func(context.Context) *exec.Cmd {
		return func(ctx context.Context) *exec.Cmd {
			cmd := exec.CommandContext(ctx, program, "literal space", "--", "$(not-a-command)") // #nosec G204 G702 -- compiled independent fixture, literal arguments only.
			cmd.Dir = root
			cmd.Env = []string{"CONTROL_MODE=" + mode, "CONTROL_MARKER=exact fixture environment", "CONTROL_READY=" + filepath.Join(root, "ready"), "CONTROL_RELEASE=" + filepath.Join(root, "release"), "LEAF_READY=" + filepath.Join(root, "leaf-ready"), "CONTROL_CLEANED=" + filepath.Join(root, "cleaned")}
			return cmd
		}
	}
	assertRetained := func(roots ...string) {
		GinkgoHelper()
		for _, root := range roots {
			Expect(cleanupFixtureDirectory(root)).To(Succeed())
			Expect(os.ReadFile(filepath.Join(root, "sentinel"))).To(Equal([]byte("exact borrowed bytes\n")))
		}
	}

	// per docs/adr/0151-reliable-ci-qualification.md:128
	It("preserves literal streams environment and nonzero status while deleting only successful fixture roots", func() {
		for _, mode := range []string{"normal", "nonzero"} {
			owned, borrowed := cooperativeControlRoots()
			file, err := os.CreateTemp("/tmp", "factory-standalone-custody-")
			Expect(err).NotTo(HaveOccurred())
			_, err = file.WriteString("exact standalone input\n")
			Expect(err).NotTo(HaveOccurred())
			Expect(file.Close()).To(Succeed())
			standalone, err := filepath.EvalSymlinks(file.Name())
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				delete(fixtureCleanupHolds, standalone)
				Expect(os.RemoveAll(standalone)).To(Succeed())
			})
			name := "fixture-control." + mode
			out := cooperativeFixtureProcess(owned, name, "literal input\n", []string{owned, borrowed, standalone}, command(owned, mode))
			want := 0
			if mode == "nonzero" {
				want = 23
			}
			Expect(out.status).To(Equal(want))
			Expect(out.stderr).To(Equal("independent stderr\n"))
			var observed map[string]any
			Expect(json.Unmarshal([]byte(out.stdout), &observed)).To(Succeed())
			Expect(observed).To(Equal(map[string]any{"argv": []any{"literal space", "--", "$(not-a-command)"}, "input": "literal input\n", "cwd": owned, "marker": "exact fixture environment"}))
			report := cooperativeControlReport(name)
			Expect(report["exit_confirmed"]).To(BeTrue())
			Expect(report["ownership_unconfirmed"]).To(BeFalse())
			if mode == "nonzero" {
				assertRetained(owned, borrowed)
				Expect(cleanupFixtureDirectory(standalone)).To(Succeed())
				Expect(os.ReadFile(standalone)).To(Equal([]byte("exact standalone input\n")))
				Expect(fixtureCleanupHolds).NotTo(HaveKey(filepath.Dir(standalone)), "custody must not retain the shared temporary parent")
				continue
			}
			for _, root := range []string{owned, borrowed, standalone} {
				Expect(cleanupFixtureDirectory(root)).To(Succeed())
				_, err := os.Stat(root)
				Expect(os.IsNotExist(err)).To(BeTrue(), "successful fixture was not removed")
			}
		}
	})

	// per docs/adr/0151-reliable-ci-qualification.md:139
	It("settles a directly reaped inherited-stream group member after its leader exits", func() {
		owned, borrowed := cooperativeControlRoots()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		type groupObservation struct {
			leader, member int
			err            error
			waitErr        error
		}
		finished := make(chan groupObservation, 1)
		members := make(chan *exec.Cmd, 1)
		reaped := false
		DeferCleanup(func() {
			cancel()
			select {
			case member := <-members:
				_ = member.Process.Kill()
			default:
			}
			if !reaped {
				Eventually(finished, 12*time.Second).Should(Receive())
			}
		})
		go func() {
			observed := groupObservation{}
			defer func() {
				finished <- observed
				if observed.err != nil {
					cancel()
				}
			}()
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				data, err := os.ReadFile(filepath.Join(owned, "ready"))
				if err == nil {
					var unused int
					if _, err = fmt.Sscanf(string(data), "%d %d", &observed.leader, &unused); err == nil && observed.leader > 1 {
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
			}
			if observed.leader <= 1 {
				observed.err = fmt.Errorf("leader readiness missing")
				return
			}
			stdout, err := os.OpenFile(filepath.Join(owned, "fixture-control.inherited-stdout"), os.O_WRONLY|os.O_APPEND, 0600)
			if err != nil {
				observed.err = err
				return
			}
			defer stdout.Close()
			stderr, err := os.OpenFile(filepath.Join(owned, "fixture-control.inherited-stderr"), os.O_WRONLY|os.O_APPEND, 0600)
			if err != nil {
				observed.err = err
				return
			}
			defer stderr.Close()
			member := exec.Command("/bin/sleep", "30")
			member.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: observed.leader}
			member.Stdout, member.Stderr = stdout, stderr
			if err = member.Start(); err != nil {
				observed.err = err
				return
			}
			observed.member = member.Process.Pid
			members <- member
			defer func() { _ = member.Process.Kill() }()
			group, err := syscall.Getpgid(observed.member)
			if err != nil || group != observed.leader {
				if err != nil {
					observed.err = fmt.Errorf("inspect member group: %w", err)
				} else {
					observed.err = fmt.Errorf("member group %d differs from admitted group %d", group, observed.leader)
				}
				_ = member.Process.Kill()
				_ = member.Wait()
				return
			}
			if err = os.WriteFile(filepath.Join(owned, "release"), []byte("member holds streams\n"), 0600); err != nil {
				observed.err = err
				_ = member.Process.Kill()
			}
			observed.waitErr = member.Wait()
		}()
		started := time.Now()
		out := cooperativeFixtureProcessContext(ctx, owned, "fixture-control.inherited", "", []string{owned, borrowed}, command(owned, "inherited"))
		Expect(out.status).To(BeZero())
		Expect(time.Since(started)).To(BeNumerically("<", 10*time.Second))
		var observed groupObservation
		Eventually(finished, time.Second).Should(Receive(&observed))
		reaped = true
		Expect(observed.err).NotTo(HaveOccurred())
		Expect(observed.waitErr).To(BeAssignableToTypeOf(&exec.ExitError{}))
		for _, pid := range []int{observed.leader, observed.member} {
			Expect(syscall.Kill(pid, 0)).To(Equal(syscall.ESRCH), "owned leader or descendant remains")
		}
		Expect(cleanupFixtureDirectory(owned)).To(Succeed())
		_, err := os.Stat(owned)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// Both paths cancel the real helper only after independently observing READY.
	// No assertion runs in the controller goroutine.
	// per docs/adr/0151-reliable-ci-qualification.md:139
	for _, mode := range []string{"cooperative", "forced"} {
		It("preserves custody and unrelated work after "+mode+" cancellation", func() {
			owned, borrowed := cooperativeControlRoots()
			unrelated := exec.Command("/bin/sleep", "30")
			unrelated.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			Expect(unrelated.Start()).To(Succeed())
			DeferCleanup(func() { _ = unrelated.Process.Kill(); _ = unrelated.Wait() })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			type admission struct {
				leader, child int
				err           error
			}
			ready := make(chan admission, 1)
			go func() {
				deadline := time.Now().Add(10 * time.Second)
				for time.Now().Before(deadline) {
					data, err := os.ReadFile(filepath.Join(owned, "ready"))
					if err == nil {
						var observed admission
						_, observed.err = fmt.Sscanf(string(data), "%d %d", &observed.leader, &observed.child)
						if observed.err == nil && observed.leader > 1 && (mode == "forced" || observed.child > 1) {
							ready <- observed
							cancel()
							return
						}
					}
					time.Sleep(10 * time.Millisecond)
				}
				ready <- admission{err: fmt.Errorf("fixture did not publish admitted child readiness")}
				cancel()
			}()
			name := "fixture-control." + mode
			started := time.Now()
			intercepted := InterceptGomegaFailure(func() {
				cooperativeFixtureProcessContext(ctx, owned, name, "", []string{owned, borrowed}, command(owned, mode))
			})
			var observed admission
			Eventually(ready, time.Second).Should(Receive(&observed))
			if observed.child > 1 {
				DeferCleanup(func() { _ = syscall.Kill(-observed.child, syscall.SIGKILL) })
			}
			Expect(observed.err).NotTo(HaveOccurred())
			Expect(intercepted).To(HaveOccurred(), "the helper must report its canceled watchdog")
			Expect(time.Since(started)).To(BeNumerically("<", 10*time.Second))
			report := cooperativeControlReport(name)
			Expect(report["context_error"]).To(Equal(context.Canceled.Error()))
			Expect(report["exit_confirmed"]).To(BeTrue())
			Expect(report["ownership_unconfirmed"]).To(Equal(mode == "forced"))
			Expect(syscall.Kill(observed.leader, 0)).To(Equal(syscall.ESRCH))
			Expect(syscall.Kill(unrelated.Process.Pid, 0)).To(Succeed())
			if mode == "cooperative" {
				Expect(os.ReadFile(filepath.Join(owned, "cleaned"))).To(Equal([]byte("nested child reaped\n")))
				Expect(syscall.Kill(observed.child, 0)).To(Equal(syscall.ESRCH))
			} else {
				Expect(observed.child).To(BeZero(), "forced uncertainty needs no orphan fixture")
				Expect(report["run_error"]).NotTo(Equal("<nil>"))
			}
			assertRetained(owned, borrowed)
		})
	}
})
