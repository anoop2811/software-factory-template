package native_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Native cooperative inherited handoff ownership", func() {
	// per docs/adr/0122-default-packaged-go-upgrade.md:155
	// per docs/adr/0122-default-packaged-go-upgrade.md:163
	DescribeTable("retains typed uncertainty when selected cooperation cannot complete", func(externalKill bool) {
		root := GinkgoT().TempDir()
		ready := filepath.Join(root, "real-selected-ready")
		stdin, input, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(stdin.Close)
		DeferCleanup(input.Close)
		out, err := os.CreateTemp(root, "stdout-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(out.Close)
		diagnostic, err := os.CreateTemp(root, "stderr-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(diagnostic.Close)
		sentinel := filepath.Join(root, "parent-selected-inputs")
		Expect(os.WriteFile(sentinel, []byte("retain exact selected inputs\n"), 0600)).To(Succeed())
		unrelated := exec.Command("/bin/sleep", "30")
		unrelated.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		Expect(unrelated.Start()).To(Succeed())
		DeferCleanup(func() { _ = unrelated.Process.Kill(); _ = unrelated.Wait() })
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		type returned struct {
			result native.Execution
			err    error
		}
		finished := make(chan returned, 1)
		go func() {
			result, err := native.ExecuteInheritedCooperative(ctx, root, []string{"/bin/bash", "-c", `trap '' TERM; printf '%s\n' "$$" > "$READY"; IFS= read -r value`}, map[string]string{"READY": ready}, native.InheritedStreams{Stdin: stdin, Stdout: out, Stderr: diagnostic}, time.Hour)
			finished <- returned{result, err}
		}()
		pid := 0
		DeferCleanup(func() {
			cancel()
			if pid > 1 {
				if group, err := syscall.Getpgid(pid); err == nil && group == pid && group != syscall.Getpgrp() {
					_ = syscall.Kill(-group, syscall.SIGKILL)
				}
			}
		})
		Eventually(func() bool {
			data, err := os.ReadFile(ready)
			if err != nil {
				return false
			}
			pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil || pid <= 1 {
				return false
			}
			group, err := syscall.Getpgid(pid)
			return err == nil && group == pid && syscall.Kill(pid, 0) == nil
		}, 5*time.Second).Should(BeTrue(), "real selected leader must install TERM ignore before the parent acts")
		started := time.Now()
		if externalKill {
			Expect(syscall.Kill(-pid, syscall.SIGKILL)).To(Succeed())
		} else {
			cancel()
		}
		var observed returned
		Eventually(finished, 6*time.Second).Should(Receive(&observed), "cooperation/escalation/reap must fit the original single cleanup budget")
		Expect(syscall.Kill(-pid, 0)).To(Equal(syscall.ESRCH), "actual direct group is absent even though nested ownership cannot be inferred")
		Expect(syscall.Kill(pid, 0)).To(Equal(syscall.ESRCH), "selected leader was actually reaped")
		retained, retainError := os.ReadFile(sentinel)
		AddReportEntry("native.cooperative-abnormal-observation", map[string]any{"external_kill": externalKill, "pid": pid, "elapsed": time.Since(started).String(), "result": observed.result, "error": fmt.Sprint(observed.err), "direct_group_absent": true, "sentinel_preserved": retainError == nil && string(retained) == "retain exact selected inputs\n"}, ReportEntryVisibilityNever)
		var uncertainty *native.OwnershipError
		Expect(errors.As(observed.err, &uncertainty)).To(BeTrue(), "forced or abnormal selected exit cannot confirm cooperation completed")
		Expect(uncertainty.ProcessPID).To(Equal(pid))
		Expect(observed.result.OwnershipUnconfirmed).To(BeTrue(), "the caller must retain exact inputs rather than treating direct-group absence as transitive proof")
		Expect(observed.result.Outcome).NotTo(Equal("completed"))
		Expect(os.ReadFile(sentinel)).To(Equal([]byte("retain exact selected inputs\n")))
		Expect(syscall.Kill(unrelated.Process.Pid, 0)).To(Succeed())
	}, Entry("ignored TERM requires forced escalation", false), Entry("external SIGKILL prevents cooperative completion", true))
})
