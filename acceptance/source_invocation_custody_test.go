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
	"path/filepath"
	"sync"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const sourceInvocationBMarker = "INDEPENDENT_SOURCE_INVOCATION_B"

// Compile a distinct real native runtime without changing the primary checkout.
// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:189
func sourceInvocationImageB(work string) string {
	GinkgoHelper()
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	original := filepath.Join(repository, "internal/cli/cli.go")
	body, err := os.ReadFile(original)
	Expect(err).NotTo(HaveOccurred())
	before := "All commands use the Go runtime."
	Expect(bytes.Count(body, []byte(before))).To(Equal(1))
	changed := bytes.Replace(body, []byte(before), []byte(before+" "+sourceInvocationBMarker), 1)
	replacement := filepath.Join(work, "distinct native help.go")
	writeFixture(replacement, changed, 0600)
	overlay, err := json.Marshal(map[string]any{"Replace": map[string]string{original: replacement}})
	Expect(err).NotTo(HaveOccurred())
	overlayPath := filepath.Join(work, "native B overlay.json")
	writeFixture(overlayPath, overlay, 0600)
	output := filepath.Join(work, "actual compiled B runtime")
	args := []string{"build", "-mod=readonly", "-buildvcs=false", "-overlay", overlayPath, "-o", output, "./cmd/factory"}
	if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
		args = append([]string{"build", "-race"}, args[1:]...)
	}
	built := imageProcess(repository, map[string]string{"GOTOOLCHAIN": "local", "GOPROXY": "off", "GOSUMDB": "off", "GOWORK": "off", "GIT_OPTIONAL_LOCKS": "0"}, "go", args...)
	Expect(built.status).To(Equal(0), "real distinct runtime compilation is fixture setup: %+v", built)
	return output
}

var _ = Describe("Go source runtime entrypoints", func() {
	// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:189
	It("executes its own compiled invocation when another source call replaces the developer cache before exec", func() {
		root, caller := fixtureDirectories()
		base := filepath.Dir(root)
		programA := filepath.Join(base, "actual compiled A runtime")
		writeFixture(programA, candidateBinary, 0700)
		programB := sourceInvocationImageB(base)
		binaryB, err := os.ReadFile(programB)
		Expect(err).NotTo(HaveOccurred())
		Expect(binaryB).NotTo(Equal(candidateBinary))
		environment := map[string]string{"GORACE": "atexit_sleep_ms=0", "FACTORY_BRIDGE_PROTOCOL": "", "BASH_ENV": "", "ENV": ""}
		controlA := handoffNativeProcess(caller, environment, "", programA, "help")
		controlB := handoffNativeProcess(caller, environment, "", programB, "help")
		Expect(controlA.status).To(Equal(0))
		Expect(controlA.stdout).To(Equal(nativeCLIUsage))
		Expect(controlB.status).To(Equal(0))
		Expect(controlB.stdout).To(ContainSubstring(sourceInvocationBMarker))
		launcher, err := os.ReadFile("../factory")
		Expect(err).NotTo(HaveOccurred())
		anchor := []byte("umask \"$SOURCE_UMASK\"\n")
		Expect(bytes.Count(launcher, anchor)).To(Equal(1), "the private scheduler must pause after publication and before execution")
		gate := []byte("if [ \"${SOURCE_INVOCATION_PHASE:-}\" = A ]; then\n  printf 'published A\\n' >&3\n  IFS= read -r _source_test_release <&4 || :\nfi\n")
		launcher = bytes.Replace(launcher, anchor, append(gate, anchor...), 1)
		writeFixture(filepath.Join(root, "factory"), launcher, 0755)
		tools := filepath.Join(base, "compiler tools")
		writeFixture(filepath.Join(tools, "go"), []byte("#!/bin/sh\nset -eu\n[ \"$#\" -eq 5 ] && [ \"$1\" = build ] && [ \"$2\" = -mod=readonly ] && [ \"$3\" = -o ] && [ \"$5\" = ./cmd/factory ] || exit 97\n/bin/cp \"$SOURCE_INVOCATION_IMAGE\" \"$4\"\n/bin/chmod 700 \"$4\"\n"), 0700)
		writeFixture(filepath.Join(root, "factory-go"), []byte("prior developer bytes\n"), 0700)
		environment["PATH"] = tools + string(os.PathListSeparator) + "/usr/bin:/bin"
		environment["SOURCE_INVOCATION_IMAGE"] = programA
		environment["SOURCE_INVOCATION_PHASE"] = "A"
		ready, readyWriter, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		releaseReader, release, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		for _, file := range []*os.File{ready, readyWriter, releaseReader} {
			DeferCleanup(func() { _ = file.Close() })
		}
		var releaseOnce sync.Once
		releaseA := func() { releaseOnce.Do(func() { _ = release.Close() }) }
		DeferCleanup(releaseA)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		DeferCleanup(cancel)
		command := handoffCommand(ctx, caller, environment, "", filepath.Join(root, "factory"), "help")
		command.ExtraFiles = []*os.File{readyWriter, releaseReader}
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
		command.WaitDelay = time.Second
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		finished := make(chan error, 1)
		joined := false
		DeferCleanup(func() {
			releaseA()
			if joined || command.Process == nil {
				return
			}
			_ = command.Process.Signal(syscall.SIGTERM)
			select {
			case <-finished:
				joined = true
			case <-time.After(3 * time.Second):
				_ = command.Process.Kill()
				select {
				case <-finished:
					joined = true
				case <-time.After(3 * time.Second):
				}
				holdFixtureCleanup(root, "source invocation did not complete its owned process cleanup")
			}
		})
		before, callerBefore := sourceEntrypointSnapshot(root), installationSnapshot(caller)
		Expect(command.Start()).To(Succeed())
		go func() { finished <- command.Wait() }()
		Expect(readyWriter.Close()).To(Succeed())
		Expect(releaseReader.Close()).To(Succeed())
		Expect(ready.SetReadDeadline(time.Now().Add(10 * time.Second))).To(Succeed())
		data := make([]byte, len("published A\n"))
		n, err := io.ReadFull(ready, data)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data[:n])).To(Equal("published A\n"))
		Expect(os.ReadFile(filepath.Join(root, "factory-go"))).To(Equal(candidateBinary), "A must have actually published before B starts")
		other := map[string]string{}
		for key, value := range environment {
			other[key] = value
		}
		other["SOURCE_INVOCATION_PHASE"], other["SOURCE_INVOCATION_IMAGE"] = "B", programB
		resultB := handoffNativeProcess(caller, other, "", filepath.Join(root, "factory"), "help")
		Expect(resultB.status).To(Equal(0), "%+v", resultB)
		Expect(resultB.stdout).To(Equal(controlB.stdout))
		Expect(os.ReadFile(filepath.Join(root, "factory-go"))).To(Equal(binaryB), "B must replace the cache while A is still paused")
		releaseA()
		var returned error
		select {
		case returned = <-finished:
			joined = true
		case <-time.After(5 * time.Second):
			Fail("A did not return after its private publication barrier was released")
		}
		status := 0
		if returned != nil {
			var exited *exec.ExitError
			Expect(errors.As(returned, &exited)).To(BeTrue())
			status = exited.ExitCode()
		}
		AddReportEntry("source-entrypoint.concurrent-invocations", map[string]any{"A_status": status, "A_stdout": stdout.String(), "A_stderr": stderr.String(), "B_status": resultB.status, "B_stdout": resultB.stdout, "A_image_sha256": fmt.Sprintf("%x", sha256.Sum256(candidateBinary)), "B_image_sha256": fmt.Sprintf("%x", sha256.Sum256(binaryB)), "A_joined": joined, "cache_is_B_before_A_release": true}, ReportEntryVisibilityNever)
		Expect(sourceEntrypointSnapshot(root)).To(Equal(before), "both invocation leases and publication scratch must close after their own command")
		Expect(installationSnapshot(caller)).To(Equal(callerBefore))
		Expect(os.ReadFile(filepath.Join(root, "factory-go"))).To(Equal(binaryB), "A cleanup must never remove B's shared developer cache")
		Expect(status).To(Equal(0))
		Expect(stdout.String()).To(Equal(controlA.stdout), "A must execute its own compiled image even though the published cache is now B")
	})
})
