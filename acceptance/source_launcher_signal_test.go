package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const sourceCompilerHandshake = "--factory-source-compiler-handshake"
const sourcePriorHandshake = "--factory-source-prior-job"
const sourceCompilerOutput = "compiler output: literal $(not-code)\n"
const sourcePublishedProgram = "#!/bin/sh\nprintf 'source factory result\\n'\n"

var sourceCompilerHelperImage string

// Capture the actual instrumented evaluator before nested Ginkgo builds can
// replace or unlink their workspace artifact. Linux's kernel-owned image path
// also remains readable if the artifact disappeared before suite setup.
// per docs/adr/0108-default-go-cutover.md:270
func captureSourceCompilerImage(destination string) {
	GinkgoHelper()
	image, err := os.Executable()
	Expect(err).NotTo(HaveOccurred())
	if runtime.GOOS == "linux" {
		image = "/proc/self/exe"
	}
	body, err := os.ReadFile(image)
	Expect(err).NotTo(HaveOccurred(), "capture the running evaluator, not a later workspace build")
	writeFixture(destination, body, 0700)
	Expect(os.ReadFile(destination)).To(Equal(body), "retain identical race instrumentation and helper behavior")
	sourceCompilerHelperImage = destination
	AddReportEntry("source-launcher.compiler-image", map[string]any{"source": image, "helper": destination, "sha256": fmt.Sprintf("%x", sha256.Sum256(body))})
}

// Re-execute the evaluator as a controlled compiler with real signal handling;
// no production flag, sleeping child or downloaded executable supplies evidence.
// per docs/adr/0108-default-go-cutover.md:56
func init() {
	if len(os.Args) >= 2 && os.Args[1] == sourcePriorHandshake {
		ready := os.Getenv("SOURCE_PRIOR_READY")
		body, err := json.Marshal(map[string]int{"PID": os.Getpid(), "Group": syscall.Getpgrp(), "Parent": os.Getppid()})
		if err != nil || os.WriteFile(ready+".pending", body, 0600) != nil || os.Rename(ready+".pending", ready) != nil { // #nosec G703 -- evaluator-owned prior-job readiness path; no production input or hook enables this mode.
			os.Exit(97)
		}
		_, _ = io.Copy(io.Discard, os.NewFile(4, "prior-job release"))
		os.Exit(0)
	}
	if len(os.Args) < 2 || os.Args[1] != sourceCompilerHandshake {
		return
	}
	ready, release := os.NewFile(3, "compiler-ready"), os.NewFile(4, "compiler-release")
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	released := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, release); close(released) }()
	inherited := map[string]string{}
	for descriptor := 5; descriptor <= 20; descriptor++ {
		file := os.NewFile(uintptr(descriptor), "inherited compiler descriptor")
		data, err := io.ReadAll(file)
		if err != nil {
			inherited[strconv.Itoa(descriptor)] = "descriptor error: " + err.Error()
		} else {
			inherited[strconv.Itoa(descriptor)] = string(data)
		}
	}
	if err := json.NewEncoder(ready).Encode(map[string]any{"pid": os.Getpid(), "group": syscall.Getpgrp(), "descriptors": inherited}); err != nil {
		os.Exit(94)
	}
	select {
	case received := <-interrupts:
		os.Exit(128 + int(received.(syscall.Signal)))
	case <-released:
		output := ""
		for index, arg := range os.Args[2:] {
			if arg == "-o" && index+3 < len(os.Args) {
				output = os.Args[index+3]
			}
		}
		if output == "" || os.WriteFile(output, []byte(sourcePublishedProgram), 0700) != nil || os.Chmod(output, 0700) != nil { // #nosec G306,G703 -- the source launcher supplied its owned executable output; chmod models the real compiler replacing the pre-existing private mktemp file.
			os.Exit(95)
		}
		_, _ = fmt.Fprint(os.Stdout, sourceCompilerOutput)
		if os.Getenv("SOURCE_COMPILER_DIAGNOSTIC") == "true" {
			_, _ = fmt.Fprintf(os.Stderr, "%s: child setpgid (%d to %d): Operation not permitted\ncompiler diagnostic: literal $(not-code)\n", os.Getenv("SOURCE_COMPILER_LAUNCHER"), os.Getpid(), os.Getpid())
		}
		os.Exit(0)
	}
}

var _ = Describe("Go source launcher signal ownership", func() {
	// per docs/adr/0108-default-go-cutover.md:56
	// per docs/adr/0111-source-launcher-and-adopter-boundaries.md:32
	// per docs/adr/0111-source-launcher-and-adopter-boundaries.md:64
	DescribeTable("admits the compiler group and preserves completion signals diagnostics and inherited descriptors", func(received syscall.Signal, diagnostic bool) {
		root, cwd := fixtureDirectories()
		launcher, err := os.ReadFile(filepath.Join("..", "factory"))
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(root, "factory"), launcher, 0700)
		writeFixture(filepath.Join(root, "factory-go"), []byte("#!/bin/sh\nexit 96\n"), 0700)
		tools := filepath.Join(root, "compiler tools")
		writeFixture(filepath.Join(tools, "go"), []byte("#!/bin/sh\nexec \"$SOURCE_COMPILER_HELPER\" "+sourceCompilerHandshake+" \"$@\"\n"), 0700)
		executable := sourceCompilerHelperImage
		Expect(executable).NotTo(BeEmpty(), "BeforeSuite must retain the actual instrumented helper")
		ready, readyWriter, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		releaseReader, release, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		for _, file := range []*os.File{ready, readyWriter, releaseReader} {
			DeferCleanup(func() {
				err := file.Close()
				Expect(err == nil || errors.Is(err, os.ErrClosed)).To(BeTrue(), "owned pipe cleanup: %v", err)
			})
		}
		var releaseOnce sync.Once
		releaseCompiler := func() { releaseOnce.Do(func() { _ = release.Close() }) }
		DeferCleanup(releaseCompiler)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, filepath.Join(root, "factory"), "help") // #nosec G204 -- acceptance-owned source launcher and fixed literal argv.
		command.Dir = cwd
		command.Env = append(os.Environ(), "PATH="+tools+string(os.PathListSeparator)+"/usr/bin:/bin", "SOURCE_COMPILER_HELPER="+executable, "GORACE=atexit_sleep_ms=0", "SOURCE_COMPILER_DIAGNOSTIC="+strconv.FormatBool(diagnostic), "SOURCE_COMPILER_LAUNCHER="+filepath.Join(root, "factory"))
		command.ExtraFiles = []*os.File{readyWriter, releaseReader}
		expectedDescriptors := map[string]string{}
		for descriptor := 5; descriptor <= 20; descriptor++ {
			key := strconv.Itoa(descriptor)
			value := "occupied descriptor " + key + ": literal $(not-code)\x00tail\n"
			path := filepath.Join(root, "descriptor "+key)
			writeFixture(path, []byte(value), 0600)
			file, err := os.Open(path)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { Expect(file.Close()).To(Succeed()) })
			command.ExtraFiles = append(command.ExtraFiles, file)
			expectedDescriptors[key] = value
		}
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		finished := make(chan error, 1)
		reaped := false
		DeferCleanup(func() {
			releaseCompiler()
			if command.Process == nil || reaped {
				return
			}
			_ = command.Process.Kill()
			select {
			case <-finished:
			case <-time.After(3 * time.Second):
				Fail("the owned source launcher was not reaped during bounded cleanup")
			}
		})
		before := installationSnapshot(root)
		Expect(command.Start()).To(Succeed())
		go func() { finished <- command.Wait() }()
		Expect(readyWriter.Close()).To(Succeed())
		Expect(releaseReader.Close()).To(Succeed())
		Expect(ready.SetReadDeadline(time.Now().Add(3 * time.Second))).To(Succeed())
		var compiler struct {
			PID, Group  int
			Descriptors map[string]string
		}
		if readyErr := json.NewDecoder(ready).Decode(&compiler); readyErr != nil {
			// Diagnose an actual launch failure before attempting a signal. Release
			// the owned helper, and read output only after its sole waiter returns.
			// per docs/adr/0108-default-go-cutover.md:181
			releaseCompiler()
			var launchErr error
			select {
			case launchErr = <-finished:
				reaped = true
			case <-time.After(3 * time.Second):
				Expect(command.Process.Kill()).To(Or(Succeed(), MatchError(os.ErrProcessDone)))
				select {
				case launchErr = <-finished:
					reaped = true
				case <-time.After(3 * time.Second):
					Fail("compiler readiness failed and the owned launcher was not reaped within bounded cleanup")
				}
			}
			status := 0
			var exited *exec.ExitError
			if errors.As(launchErr, &exited) {
				status = exited.ExitCode()
			} else {
				Expect(launchErr).NotTo(HaveOccurred())
			}
			AddReportEntry("source-launcher.readiness-failure", map[string]any{"error": readyErr.Error(), "launcher_reaped": reaped, "launcher_status": status, "stdout": stdout.String(), "stderr": stderr.String(), "helper": executable})
			Fail(fmt.Sprintf("the actual compiler must announce readiness before parent-only signaling: %v; launcher status=%d; stdout=%q; stderr=%q", readyErr, status, stdout.String(), stderr.String()))
		}
		Expect(compiler.PID).To(BeNumerically(">", 1))
		DeferCleanup(func() {
			releaseCompiler()
			Eventually(func() bool { return errors.Is(syscall.Kill(compiler.PID, 0), syscall.ESRCH) }, 3*time.Second).Should(BeTrue(), "the actual owned compiler must disappear before fixture removal")
			AddReportEntry("source-launcher.compiler-cleanup", map[string]any{"compiler_pid": compiler.PID, "compiler_absent": true})
		})
		Expect(syscall.Kill(compiler.PID, 0)).To(Succeed())
		actualGroup, err := syscall.Getpgid(compiler.PID)
		Expect(err).NotTo(HaveOccurred())
		launcherGroup, err := syscall.Getpgid(command.Process.Pid)
		Expect(err).NotTo(HaveOccurred())
		AddReportEntry("source-launcher.compiler-admission", map[string]any{"compiler_pid": compiler.PID, "reported_group": compiler.Group, "actual_group": actualGroup, "launcher_group": launcherGroup, "preserved_descriptors": reflect.DeepEqual(compiler.Descriptors, expectedDescriptors)})
		Expect(compiler.Group).To(Equal(compiler.PID), "the actual compiler must lead its own process group")
		Expect(actualGroup).To(Equal(compiler.PID))
		Expect(actualGroup).NotTo(Equal(launcherGroup))
		Expect(compiler.Descriptors).To(Equal(expectedDescriptors), "occupied FD5 through FD20 must retain their exact bytes while FD3/4 carry the held-open handshake")
		if received == 0 {
			releaseCompiler()
		} else {
			Expect(command.Process.Signal(received)).To(Succeed())
		}
		var waitErr error
		exitedBeforeRelease := false
		select {
		case waitErr = <-finished:
			reaped, exitedBeforeRelease = true, true
		case <-time.After(2 * time.Second):
			releaseCompiler()
			select {
			case waitErr = <-finished:
				reaped = true
			case <-time.After(3 * time.Second):
				Fail("the source launcher did not exit after its controlled compiler was released")
			}
		}
		status := 0
		var exit *exec.ExitError
		if errors.As(waitErr, &exit) {
			status = exit.ExitCode()
			if state, ok := exit.Sys().(syscall.WaitStatus); ok && state.Signaled() {
				status = 128 + int(state.Signal())
			}
		} else {
			Expect(waitErr).NotTo(HaveOccurred())
		}
		compilerAbsent := errors.Is(syscall.Kill(compiler.PID, 0), syscall.ESRCH)
		unchanged := installationSnapshot(root)
		entryName := "source-launcher.parent-only-signal"
		if received == 0 {
			entryName = "source-launcher.normal-completion"
		}
		AddReportEntry(entryName, map[string]any{"signal": int(received), "status": status, "exited_before_release": received != 0 && exitedBeforeRelease, "returned_promptly": exitedBeforeRelease, "compiler_pid": compiler.PID, "compiler_absent": compilerAbsent, "source_unchanged": reflect.DeepEqual(unchanged, before)})
		Expect(exitedBeforeRelease).To(BeTrue(), "the launcher must return within its existing bound; stderr=%s", strings.TrimSpace(stderr.String()))
		Expect(compilerAbsent).To(BeTrue(), "the launcher must actually reap its owned compiler")
		if received != 0 {
			Expect(status).To(Equal(128+int(received)), "signal %d must keep its conventional status", received)
			Expect(unchanged).To(Equal(before), "cancellation must preserve an existing developer binary and remove every temporary build")
			return
		}
		Expect(status).To(BeZero(), "normal compiler completion must execute its published result; stderr=%s", stderr.String())
		Expect(stdout.String()).To(Equal(sourceCompilerOutput + "source factory result\n"))
		expectedDiagnostic := ""
		if diagnostic {
			expectedDiagnostic = fmt.Sprintf("%s: child setpgid (%d to %d): Operation not permitted\ncompiler diagnostic: literal $(not-code)\n", filepath.Join(root, "factory"), compiler.PID, compiler.PID)
		}
		Expect(stderr.String()).To(Equal(expectedDiagnostic), "real compiler diagnostics must survive exactly, including text shaped like Bash's startup diagnostic")
		Expect(os.ReadFile(filepath.Join(root, "factory-go"))).To(Equal([]byte(sourcePublishedProgram)))
		info, err := os.Stat(filepath.Join(root, "factory-go"))
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0700)))
		delete(before, "factory-go")
		delete(unchanged, "factory-go")
		Expect(unchanged).To(Equal(before), "successful publication may replace only the developer binary and must remove private build/admission residue")
	}, Entry("TERM", syscall.SIGTERM, false), Entry("INT", syscall.SIGINT, false), Entry("HUP", syscall.SIGHUP, false), Entry("healthy normal completion with occupied descriptors", syscall.Signal(0), false), Entry("literal compiler diagnostics with occupied descriptors", syscall.Signal(0), true))

	// per docs/adr/0111-source-launcher-and-adopter-boundaries.md:33
	// per docs/adr/0111-source-launcher-and-adopter-boundaries.md:40
	It("refuses an absent compiler group without waiting on a live unadmitted child or killing its borrowed group", func() {
		root, cwd := fixtureDirectories()
		launcher, err := os.ReadFile(filepath.Join("..", "factory"))
		Expect(err).NotTo(HaveOccurred())
		monitor := []byte("\nset -m\n")
		Expect(bytes.Count(launcher, monitor)).To(Equal(1))
		launcher = bytes.Replace(launcher, monitor, []byte("\nset +m\n"), 1)
		admission := []byte("  compiler_group=\"$(/bin/ps -o pgid= -p \"$compiler_pid\")\" || return")
		Expect(bytes.Count(launcher, admission)).To(Equal(1))
		hold := []byte("  printf '{\"PID\":%s}\\n' \"$compiler_pid\" >&3\n  IFS= read -r _source_test_release <&4 || :\n")
		launcher = bytes.Replace(launcher, admission, append(hold, admission...), 1)
		writeFixture(filepath.Join(root, "factory"), launcher, 0700)
		writeFixture(filepath.Join(root, "factory-go"), []byte("prior developer binary\n"), 0700)
		witnessReady := filepath.Join(cwd, "borrowed-group-witness.json")
		startup := filepath.Join(cwd, "borrowed-group-startup.sh")
		writeFixture(startup, []byte("set +m\n\"$SOURCE_PRIOR_HELPER\" "+sourcePriorHandshake+" >/dev/null 2>/dev/null &\nwhile [ ! -s \"$SOURCE_PRIOR_READY\" ]; do /bin/sleep 0.01; done\n"), 0600)
		tools := filepath.Join(root, "tools")
		marker := filepath.Join(root, "UNADMITTED_COMPILER_EXECUTED")
		writeFixture(filepath.Join(tools, "go"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_UNADMITTED_MARKER\"\nexit 99\n"), 0700)
		ready, readyWriter, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		releaseReader, release, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		for _, file := range []*os.File{ready, readyWriter, releaseReader} {
			DeferCleanup(func() { err := file.Close(); Expect(err == nil || errors.Is(err, os.ErrClosed)).To(BeTrue()) })
		}
		var once sync.Once
		releaseChild := func() { once.Do(func() { Expect(release.Close()).To(Succeed()) }) }
		DeferCleanup(releaseChild)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, filepath.Join(root, "factory"), "help") // #nosec G204 -- owned copy with test-only admission handshake; production has no fault flag.
		command.Dir = cwd
		command.Env = append(os.Environ(), "PATH="+tools+string(os.PathListSeparator)+"/usr/bin:/bin", "SOURCE_UNADMITTED_MARKER="+marker, "BASH_ENV="+startup, "SOURCE_PRIOR_HELPER="+sourceCompilerHelperImage, "SOURCE_PRIOR_READY="+witnessReady, "GORACE=atexit_sleep_ms=0")
		command.ExtraFiles = []*os.File{readyWriter, releaseReader}
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		finished := make(chan error, 1)
		reaped := false
		DeferCleanup(func() {
			releaseChild()
			if command.Process != nil && !reaped {
				_ = command.Process.Kill()
				Eventually(finished, 3*time.Second).Should(Receive())
			}
		})
		before := installationSnapshot(root)
		Expect(command.Start()).To(Succeed())
		go func() { finished <- command.Wait() }()
		Expect(readyWriter.Close()).To(Succeed())
		Expect(releaseReader.Close()).To(Succeed())
		Expect(ready.SetReadDeadline(time.Now().Add(3 * time.Second))).To(Succeed())
		var child struct{ PID int }
		Expect(json.NewDecoder(ready).Decode(&child)).To(Succeed())
		Expect(child.PID).To(BeNumerically(">", 1))
		DeferCleanup(func() {
			releaseChild()
			Eventually(func() bool { return errors.Is(syscall.Kill(child.PID, 0), syscall.ESRCH) }, 3*time.Second).Should(BeTrue(), "the test-owned unadmitted child must disappear before fixture removal")
			AddReportEntry("source-launcher.unadmitted-child-cleanup", map[string]any{"child_pid": child.PID, "child_absent": true})
		})
		childGroup, err := syscall.Getpgid(child.PID)
		Expect(err).NotTo(HaveOccurred())
		launcherGroup, err := syscall.Getpgid(command.Process.Pid)
		Expect(err).NotTo(HaveOccurred())
		Expect(childGroup).To(Equal(launcherGroup))
		Expect(childGroup).NotTo(Equal(child.PID), "the controlled wrapper is alive outside its expected compiler group")
		body, err := os.ReadFile(witnessReady)
		Expect(err).NotTo(HaveOccurred())
		var witness struct{ PID, Group, Parent int }
		Expect(json.Unmarshal(body, &witness)).To(Succeed())
		Expect(witness.PID).To(BeNumerically(">", 1))
		Expect(witness.Parent).To(Equal(command.Process.Pid))
		Expect(witness.Group).To(Equal(launcherGroup))
		Expect(syscall.Kill(witness.PID, 0)).To(Succeed())
		DeferCleanup(func() {
			releaseChild()
			Eventually(func() bool { return errors.Is(syscall.Kill(witness.PID, 0), syscall.ESRCH) }, 3*time.Second).Should(BeTrue(), "release the test-owned borrowed-group witness separately")
			AddReportEntry("source-launcher.borrowed-group-witness-cleanup", map[string]any{"witness_pid": witness.PID, "witness_absent": true})
		})
		Expect(command.Process.Signal(syscall.SIGTERM)).To(Succeed())
		var returned error
		returnedBeforeRelease := false
		select {
		case returned = <-finished:
			reaped, returnedBeforeRelease = true, true
		case <-time.After(3 * time.Second):
		}
		liveBeforeRelease := syscall.Kill(child.PID, 0) == nil
		witnessAlive := syscall.Kill(witness.PID, 0) == nil
		held := installationSnapshot(root)
		if !returnedBeforeRelease {
			releaseChild()
			Eventually(finished, 3*time.Second).Should(Receive(&returned))
			reaped = true
		}
		Expect(returned).To(HaveOccurred())
		AddReportEntry("source-launcher.absent-group-refusal", map[string]any{"returned_before_release": returnedBeforeRelease, "child_live_before_release": liveBeforeRelease, "borrowed_group_witness_alive": witnessAlive, "child_pid": child.PID, "child_group": childGroup, "launcher_group": launcherGroup, "status": command.ProcessState.ExitCode(), "stderr": stderr.String()})
		Expect(returnedBeforeRelease).To(BeTrue(), "real caller Wait must return without releasing a live unadmitted child holding capture; stderr=%s", stderr.String())
		Expect(witnessAlive).To(BeTrue(), "uncertain compiler cleanup must never signal the borrowed caller group")
		Expect(command.ProcessState.ExitCode()).To(Equal(125))
		Expect(stdout.String()).To(BeEmpty())
		Expect(stderr.String()).To(ContainSubstring("ownership is unconfirmed"))
		retained := 0
		for path := range held {
			if strings.HasPrefix(path, ".factory-go.") || strings.HasPrefix(path, ".factory-startup.") || strings.HasPrefix(path, ".factory-admission.") {
				if strings.HasPrefix(path, ".factory-go.") {
					retained++
					Expect(stderr.String()).To(ContainSubstring(filepath.Join(root, path)))
				}
				delete(held, path)
			}
		}
		Expect(retained).To(Equal(1), "uncertain cleanup must retain the private build")
		Expect(held).To(Equal(before), "preserve the prior developer binary and all other source")
		publicNoPoison(marker)
	})

	// per docs/adr/0111-source-launcher-and-adopter-boundaries.md:34
	// per docs/adr/0111-source-launcher-and-adopter-boundaries.md:36
	It("preserves a prior caller background job when interrupted before its compiler fork", func() {
		root, cwd := fixtureDirectories()
		launcher, err := os.ReadFile(filepath.Join("..", "factory"))
		Expect(err).NotTo(HaveOccurred())
		prefork := []byte("SOURCE_BUILDING=1\n")
		Expect(bytes.Count(launcher, prefork)).To(Equal(1))
		hold := []byte("SOURCE_BUILDING=1\nprintf '{\"PID\":%s}\\n' \"$$\" >&3\nIFS= read -r _source_test_release <&4 || :\n")
		launcher = bytes.Replace(launcher, prefork, hold, 1)
		writeFixture(filepath.Join(root, "factory"), launcher, 0700)
		writeFixture(filepath.Join(root, "factory-go"), []byte("prior developer binary\n"), 0700)
		tools := filepath.Join(root, "tools")
		marker := filepath.Join(root, "COMPILER_EXECUTED_BEFORE_FORK")
		writeFixture(filepath.Join(tools, "go"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_UNADMITTED_MARKER\"\nexit 99\n"), 0700)
		priorReady := filepath.Join(cwd, "prior-ready.json")
		startup := filepath.Join(cwd, "startup.sh")
		writeFixture(startup, []byte("set -m\n\"$SOURCE_PRIOR_HELPER\" "+sourcePriorHandshake+" >/dev/null 2>/dev/null &\nwhile [ ! -s \"$SOURCE_PRIOR_READY\" ]; do /bin/sleep 0.01; done\nset +m\n"), 0600)
		ready, readyWriter, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		releaseReader, release, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		for _, file := range []*os.File{ready, readyWriter, releaseReader} {
			DeferCleanup(func() { err := file.Close(); Expect(err == nil || errors.Is(err, os.ErrClosed)).To(BeTrue()) })
		}
		var once sync.Once
		releasePrior := func() { once.Do(func() { Expect(release.Close()).To(Succeed()) }) }
		DeferCleanup(releasePrior)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, filepath.Join(root, "factory"), "help") // #nosec G204 -- owned launcher and BASH_ENV prior-job fixture; only the launcher parent receives the test signal.
		command.Dir = cwd
		command.Env = append(os.Environ(), "PATH="+tools+string(os.PathListSeparator)+"/usr/bin:/bin", "BASH_ENV="+startup, "SOURCE_PRIOR_HELPER="+sourceCompilerHelperImage, "SOURCE_PRIOR_READY="+priorReady, "SOURCE_UNADMITTED_MARKER="+marker, "GORACE=atexit_sleep_ms=0")
		command.ExtraFiles = []*os.File{readyWriter, releaseReader}
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		finished := make(chan error, 1)
		reaped := false
		DeferCleanup(func() {
			releasePrior()
			if command.Process != nil && !reaped {
				_ = command.Process.Kill()
				Eventually(finished, 3*time.Second).Should(Receive())
			}
		})
		before := installationSnapshot(root)
		Expect(command.Start()).To(Succeed())
		go func() { finished <- command.Wait() }()
		Expect(readyWriter.Close()).To(Succeed())
		Expect(releaseReader.Close()).To(Succeed())
		Expect(ready.SetReadDeadline(time.Now().Add(3 * time.Second))).To(Succeed())
		var parent struct{ PID int }
		Expect(json.NewDecoder(ready).Decode(&parent)).To(Succeed())
		Expect(parent.PID).To(Equal(command.Process.Pid), "signal only after SOURCE_BUILDING=1 and before any compiler fork")
		body, err := os.ReadFile(priorReady)
		Expect(err).NotTo(HaveOccurred())
		var prior struct{ PID, Group, Parent int }
		Expect(json.Unmarshal(body, &prior)).To(Succeed())
		Expect(prior.PID).To(BeNumerically(">", 1))
		DeferCleanup(func() {
			releasePrior()
			Eventually(func() bool { return errors.Is(syscall.Kill(prior.PID, 0), syscall.ESRCH) }, 3*time.Second).Should(BeTrue(), "release and observe absence of the evaluator-owned prior job")
			AddReportEntry("source-launcher.prior-job-cleanup", map[string]any{"prior_pid": prior.PID, "prior_absent": true})
		})
		Expect(prior.Parent).To(Equal(command.Process.Pid))
		Expect(prior.Group).To(Equal(prior.PID))
		Expect(syscall.Kill(prior.PID, 0)).To(Succeed())
		actualGroup, err := syscall.Getpgid(prior.PID)
		Expect(err).NotTo(HaveOccurred())
		Expect(actualGroup).To(Equal(prior.Group))
		Expect(command.Process.Signal(syscall.SIGINT)).To(Succeed())
		var returned error
		select {
		case returned = <-finished:
			reaped = true
		case <-time.After(3 * time.Second):
			releasePrior()
			Eventually(finished, 3*time.Second).Should(Receive(&returned))
			reaped = true
			Fail("parent-only INT must not wait for or cancel a prior caller job")
		}
		priorAlive := syscall.Kill(prior.PID, 0) == nil
		AddReportEntry("source-launcher.prior-job-preservation", map[string]any{"prior_pid": prior.PID, "prior_group": prior.Group, "prior_alive_after_parent_exit": priorAlive, "launcher_status": command.ProcessState.ExitCode()})
		Expect(returned).To(HaveOccurred())
		Expect(command.ProcessState.ExitCode()).To(Equal(130))
		Expect(priorAlive).To(BeTrue(), "SOURCE_BUILDING alone must never transfer ownership of an earlier $! job")
		Expect(stdout.String()).To(BeEmpty())
		Expect(stderr.String()).To(BeEmpty())
		Expect(installationSnapshot(root)).To(Equal(before))
		publicNoPoison(marker)
	})
})
