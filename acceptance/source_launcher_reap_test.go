package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Go source launcher signal ownership", func() {
	// per docs/adr/0133-go-source-evaluation-runners.md:249
	It("reaps a real owned child that becomes terminal after the final polling sleep", func() {
		root, cwd := fixtureDirectories()
		launcher, err := os.ReadFile("../factory")
		Expect(err).NotTo(HaveOccurred())
		function := func(name string) string {
			anchor := []byte(name + "() {\n")
			Expect(bytes.Count(launcher, anchor)).To(Equal(1))
			start := bytes.Index(launcher, anchor)
			end := bytes.Index(launcher[start:], []byte("\n}\n"))
			Expect(end).To(BeNumerically(">", 0))
			return string(launcher[start : start+end+3])
		}
		var reaper strings.Builder
		sleeps := 0
		for _, line := range strings.SplitAfter(function("source_reap_child"), "\n") {
			reaper.WriteString(line)
			if strings.HasPrefix(strings.TrimSpace(line), "/bin/sleep ") {
				Expect(strings.Fields(line)).To(HaveLen(2))
				sleeps++
				reaper.WriteString(`      SOURCE_TEST_POLLS=$((SOURCE_TEST_POLLS + 1))
      if [ "$SOURCE_TEST_POLLS" -eq 5 ]; then
        builtin kill -0 "$SOURCE_CHILD" || exit 96
        printf '{"PID":%s}\n' "$SOURCE_CHILD" >&4
        IFS= read -r _source_test_continue <&5 || :
      fi
`)
			}
		}
		Expect(sleeps).To(Equal(1))
		script := "set -u\n" + function("source_child_active") + reaper.String() + `
SOURCE_CHILD=''
SOURCE_BUILDING=1
SOURCE_TEST_POLLS=0
trap 'if [ -n "$SOURCE_CHILD" ]; then builtin kill -TERM "$SOURCE_CHILD" 2>/dev/null || :; wait "$SOURCE_CHILD" 2>/dev/null || :; fi' EXIT
( IFS= read -r _source_test_release <&3 || : ) &
SOURCE_CHILD=$!
source_reap_child
status=$?
printf '{"status":%s,"child":"%s","building":%s,"polls":%s}\n' "$status" "$SOURCE_CHILD" "$SOURCE_BUILDING" "$SOURCE_TEST_POLLS"
exit "$status"
`
		program := filepath.Join(root, "actual reaper")
		writeFixture(program, []byte(script), 0700)
		childRelease, release, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		ready, readyWriter, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		pollContinue, resume, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		for _, file := range []*os.File{childRelease, release, ready, readyWriter, pollContinue, resume} {
			DeferCleanup(func() { _ = file.Close() })
		}
		var releaseOnce, resumeOnce sync.Once
		releaseChild := func() { releaseOnce.Do(func() { _ = release.Close() }) }
		resumePoll := func() { resumeOnce.Do(func() { _ = resume.Close() }) }
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, "/bin/bash", program) // #nosec G204 -- actual source functions copied into the evaluator-owned fixture.
		command.Dir = cwd
		command.Env = append(os.Environ(), "BASH_ENV=", "ENV=")
		command.ExtraFiles = []*os.File{childRelease, readyWriter, pollContinue}
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		finished, joined := make(chan error, 1), false
		Expect(command.Start()).To(Succeed())
		go func() { finished <- command.Wait() }()
		DeferCleanup(func() {
			releaseChild()
			resumePoll()
			if !joined {
				select {
				case <-finished:
					joined = true
				case <-time.After(3 * time.Second):
					_ = command.Process.Kill()
					select {
					case <-finished:
						joined = true
					case <-time.After(2 * time.Second):
						holdFixtureCleanup(filepath.Dir(root), "late reap fixture parent did not join")
					}
				}
			}
		})
		Expect(childRelease.Close()).To(Succeed())
		Expect(readyWriter.Close()).To(Succeed())
		Expect(pollContinue.Close()).To(Succeed())
		Expect(ready.SetReadDeadline(time.Now().Add(3 * time.Second))).To(Succeed())
		var child struct{ PID int }
		Expect(json.NewDecoder(ready).Decode(&child)).To(Succeed())
		Expect(child.PID).To(BeNumerically(">", 1))
		Expect(syscall.Kill(child.PID, 0)).To(Succeed(), "the child must remain live through the fifth completed sleep")
		releaseChild()
		Eventually(func() bool { return errors.Is(syscall.Kill(child.PID, 0), syscall.ESRCH) }, 3*time.Second).Should(BeTrue(), "the controlled child must become terminal before the final observation resumes")
		resumePoll()
		var returned error
		Eventually(finished, 3*time.Second).Should(Receive(&returned))
		joined = true
		Expect(returned).NotTo(HaveOccurred(), "the final terminal observation must reap the actual owned child: %s", stderr.String())
		var receipt struct {
			Status, Building, Polls int
			Child                   string
		}
		Expect(json.Unmarshal(stdout.Bytes(), &receipt)).To(Succeed())
		Expect(receipt.Status).To(BeZero())
		Expect(receipt.Child).To(BeEmpty())
		Expect(receipt.Building).To(BeZero())
		Expect(receipt.Polls).To(Equal(5))
		AddReportEntry("source-launcher.final-reap-observation", map[string]any{"child_pid": child.PID, "child_absent_before_resume": true, "receipt": receipt})
	})
})
