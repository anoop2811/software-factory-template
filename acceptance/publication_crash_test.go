package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/transition"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

// This evaluator-only program calls the exported production API. It is never
// installed, dispatched by factory, or included in a shipping asset inventory.
// per docs/adr/0094-live-publication-restoration.md:21
// per docs/adr/0094-live-publication-restoration.md:120
const publicationCrashSource = `package main
import("context";"encoding/json";"fmt";"os";"time";"github.com/anoop2811/software-factory-template/internal/assessment")
func main(){var request assessment.PublicationRequest;if json.NewDecoder(os.Stdin).Decode(&request)!=nil{os.Exit(2)};operation,err:=assessment.BeginPublication(context.Background(),os.Args[1],request);if err==nil&&os.Args[3]=="applied"{err=operation.Apply(context.Background())};if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(2)};if err=os.WriteFile(os.Args[2],[]byte(fmt.Sprintf("%d:%s",os.Getpid(),os.Args[3])),0600);err!=nil{os.Exit(2)};for{time.Sleep(time.Second)}}
`

var publicationCrashBinary []byte

func publicationCrashBuild() []byte {
	GinkgoHelper()
	if publicationCrashBinary != nil {
		return publicationCrashBinary
	}
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	dir := GinkgoT().TempDir()
	module, err := os.ReadFile(filepath.Join(repository, "go.mod"))
	Expect(err).NotTo(HaveOccurred())
	parts := strings.SplitN(string(module), "\ngo ", 2)
	Expect(parts).To(HaveLen(2))
	version := strings.Fields(parts[1])[0]
	writeFixture(filepath.Join(dir, "go.mod"), []byte("module github.com/anoop2811/software-factory-template/acceptance/publicationfixture\n\ngo "+version+"\nrequire github.com/anoop2811/software-factory-template v0.0.0\nreplace github.com/anoop2811/software-factory-template => "+repository+"\n"), 0600)
	writeFixture(filepath.Join(dir, "main.go"), []byte(publicationCrashSource), 0600)
	args := []string{"build", "-mod=mod", "-o", filepath.Join(dir, "publication-crash"), "."}
	if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
		args = append([]string{"build", "-race"}, args[1:]...)
	}
	command := exec.Command("go", args...) // #nosec G204 -- evaluator-owned isolated module and fixed build arguments.
	command.Dir = dir
	command.Env = append(os.Environ(), "GOPROXY=off")
	output, err := command.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "%s", output)
	publicationCrashBinary, err = os.ReadFile(filepath.Join(dir, "publication-crash"))
	Expect(err).NotTo(HaveOccurred())
	return publicationCrashBinary
}

var _ = Describe("Live publication abrupt interruption", func() {
	// per docs/adr/0094-live-publication-restoration.md:111
	// per docs/adr/0094-live-publication-restoration.md:112
	// per docs/adr/0094-live-publication-restoration.md:120
	DescribeTable("loses the live capability on real process death while durable pending evidence blocks cooperating work", func(phase string) {
		binaryRoot, root, request := livePublicationFixture()
		original := durableBytes(root, request.Path)
		program := filepath.Join(binaryRoot, "publication-crash")
		writeFixture(program, publicationCrashBuild(), 0700)
		ready := filepath.Join(binaryRoot, "publication-ready")
		raw, err := json.Marshal(request)
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, program, root, ready, phase) // #nosec G204 -- evaluator-owned helper binary and fixture arguments.
		command.Stdin = bytes.NewReader(raw)
		command.Env = append(os.Environ(), "GORACE=atexit_sleep_ms=0")
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		var stderr bytes.Buffer
		command.Stderr = &stderr
		Expect(command.Start()).To(Succeed())
		waited := false
		DeferCleanup(func() {
			if !waited {
				_ = unix.Kill(-command.Process.Pid, unix.SIGKILL)
				_ = command.Wait()
			}
		})
		Eventually(func() string { data, _ := os.ReadFile(ready); return string(data) }, 10*time.Second, 10*time.Millisecond).Should(Equal(strconv.Itoa(command.Process.Pid)+":"+phase), "real helper must finish the selected operation before abrupt termination")
		pending, err := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(err).NotTo(HaveOccurred())
		transitionLockBlocked(root)
		expected := original
		if phase == "applied" {
			expected = request.Replacement
		}
		Expect(durableBytes(root, request.Path)).To(Equal(expected))
		Expect(command.Process.Kill()).To(Succeed())
		err = command.Wait()
		waited = true
		var exited *exec.ExitError
		Expect(errors.As(err, &exited)).To(BeTrue(), "%s", stderr.String())
		Expect(exited.ProcessState.Sys().(syscall.WaitStatus).Signal()).To(Equal(syscall.SIGKILL))
		Expect(stderr.String()).To(BeEmpty())
		publicationPendingPreserved(root, pending)
		lock, err := os.OpenFile(filepath.Join(root, ".factory/runtime-transition.lock"), os.O_RDWR, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB)).To(Succeed(), "real process death releases the OS flock")
		Expect(unix.Flock(int(lock.Fd()), unix.LOCK_UN)).To(Succeed())
		Expect(lock.Close()).To(Succeed())
		for _, acquire := range []func(context.Context, string) (*transition.Guard, error){transition.Shared, transition.Exclusive} {
			guard, acquireErr := acquire(context.Background(), root)
			if guard != nil {
				DeferCleanup(func() { _ = guard.Close(context.Background(), false) })
			}
			Expect(acquireErr).To(HaveOccurred())
			Expect(guard).To(BeNil())
		}
		_, _, scripts := transitionFixture()
		for _, legacy := range []bool{false, true} {
			for _, kind := range []string{"budget", "loop"} {
				out := transitionRun(binaryRoot, root, scripts, kind, legacy, transitionRunArgs(kind))
				_, err = os.Lstat(filepath.Join(binaryRoot, "child-started"))
				Expect(os.IsNotExist(err)).To(BeTrue(), "no help/check may execute after loss of the live handle: %+v", out)
				Expect(out.status).To(Equal(2), "%+v", out)
				Expect(out.stdout).To(BeEmpty())
			}
		}
		Expect(durableBytes(root, request.Path)).To(Equal(expected))
		publicationPendingPreserved(root, pending)
		livePublicationNoExecutionState(root)
	}, Entry("prepared but unapplied", "prepared"), Entry("actual replacement already applied", "applied"))
})
