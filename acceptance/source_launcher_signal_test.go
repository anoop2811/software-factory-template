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
	"strings"
	"sync"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const sourceCompilerHandshake = "--factory-source-compiler-handshake"

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
	if len(os.Args) < 2 || os.Args[1] != sourceCompilerHandshake {
		return
	}
	ready, release := os.NewFile(3, "compiler-ready"), os.NewFile(4, "compiler-release")
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	released := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, release); close(released) }()
	if err := json.NewEncoder(ready).Encode(map[string]int{"pid": os.Getpid(), "group": syscall.Getpgrp()}); err != nil {
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
		if output == "" || os.WriteFile(output, []byte("#!/bin/sh\nexit 0\n"), 0700) != nil { // #nosec G306,G703 -- the source launcher supplied its own private executable output; this test-only compiler must produce executable bytes.
			os.Exit(95)
		}
		os.Exit(0)
	}
}

var _ = Describe("Go source launcher signal ownership", func() {
	// per docs/adr/0108-default-go-cutover.md:56
	DescribeTable("forwards a parent-only signal and reaps its compiler before discarding the private build", func(received syscall.Signal) {
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
		command.Env = append(os.Environ(), "PATH="+tools+string(os.PathListSeparator)+"/usr/bin:/bin", "SOURCE_COMPILER_HELPER="+executable, "GORACE=atexit_sleep_ms=0")
		command.ExtraFiles = []*os.File{readyWriter, releaseReader}
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
		var compiler struct{ PID, Group int }
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
		Expect(syscall.Kill(compiler.PID, 0)).To(Succeed())
		Expect(command.Process.Signal(received)).To(Succeed())
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
		AddReportEntry("source-launcher.parent-only-signal", map[string]any{"signal": int(received), "status": status, "exited_before_release": exitedBeforeRelease, "compiler_pid": compiler.PID, "compiler_absent": compilerAbsent, "source_unchanged": reflect.DeepEqual(unchanged, before)})
		Expect(exitedBeforeRelease).To(BeTrue(), "the launcher must handle the parent-only signal while its compiler's release pipe stays open; stderr=%s", strings.TrimSpace(stderr.String()))
		Expect(status).To(Equal(128+int(received)), "signal %d must keep its conventional status", received)
		Expect(compilerAbsent).To(BeTrue(), "the launcher must actually reap its owned compiler")
		Expect(unchanged).To(Equal(before), "cancellation must preserve an existing developer binary and remove every temporary build")
	}, Entry("TERM", syscall.SIGTERM), Entry("INT", syscall.SIGINT), Entry("HUP", syscall.SIGHUP))
})
