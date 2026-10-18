package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Native process group completion proof", func() {
	// per docs/adr/0093-runtime-transition-guard.md:195
	// per docs/adr/0093-runtime-transition-guard.md:199
	// per docs/adr/0093-runtime-transition-guard.md:201
	// per specs/001-go-runtime-conversion.md:314
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:634
	// per docs/adr/0102-parallel-go-qualification.md:117
	// per docs/adr/0102-parallel-go-qualification.md:125
	DescribeTable("distinguishes group termination acknowledgement from the actual end of detached-pipe ownership", func(acknowledgeOnly bool) {
		root := GinkgoT().TempDir()
		executable, err := os.Executable()
		Expect(err).NotTo(HaveOccurred())
		ready := filepath.Join(root, "child.json")
		plan := Plan{Harness: "opencode", Role: "reviewer", Root: root, Argv: []string{executable, "--factory-native-test-fixture", "detached-leader"}, Environment: map[string]string{"NATIVE_GROUP_READY": ready, "GORACE": "atexit_sleep_ms=0"}}
		ops := defaultProcessOps()
		originalSignal, originalWait := ops.signalGroup, ops.wait
		waited := make(chan error, 1)
		ops.wait = func(command *exec.Cmd) error {
			err := originalWait(command)
			waited <- err
			return err
		}
		leader, descendant, group, signals := 0, 0, 0, 0
		realGroupSignaled := false
		DeferCleanup(func() {
			if leader > 0 && !realGroupSignaled {
				err := originalSignal(leader)
				Expect(err).NotTo(HaveOccurred())
			}
			if descendant > 0 {
				Eventually(func() error { return syscall.Kill(descendant, 0) }, 5*time.Second).Should(Equal(syscall.ESRCH), "the test's real owned descendant must be gone after its explicit final group cleanup")
			}
		})
		ops.signalGroup = func(pid int) error {
			signals++
			data, err := os.ReadFile(ready)
			Expect(err).NotTo(HaveOccurred(), "a real child must handshake before the leader returns")
			var record struct {
				PID   int `json:"pid"`
				Group int `json:"group"`
			}
			Expect(json.Unmarshal(data, &record)).To(Succeed())
			descendant, group = record.PID, record.Group
			Expect(descendant).NotTo(Equal(pid))
			Expect(group).To(Equal(pid), "the surviving process belongs to the actual native-owned group")
			Expect(syscall.Kill(descendant, 0)).To(Succeed())
			if acknowledgeOnly {
				return nil // Acknowledged delivery deliberately does not establish group disappearance.
			}
			err = originalSignal(pid)
			realGroupSignaled = err == nil
			return err
		}
		result, err := execute(context.Background(), plan, 5*time.Second, func(_ context.Context, pid int) error { leader = pid; return nil }, ops)
		Expect(signals).To(Equal(1))
		Expect(<-waited).NotTo(HaveOccurred(), "the real leader's Wait succeeded without a synthetic status")
		Expect(syscall.Kill(leader, 0)).To(Equal(syscall.ESRCH))
		Expect(result.ProcessPID).To(Equal(leader))
		Expect(result.ExitConfirmed).To(BeTrue())
		Expect(result.ExitCode).NotTo(BeNil())
		Expect(*result.ExitCode).To(BeZero())
		Expect(string(result.Stdout)).To(Equal("fixture leader completed\n"), "the detached child holds none of the supervisor's capture pipes")
		if acknowledgeOnly {
			Expect(syscall.Kill(-group, 0)).To(Succeed(), "group ownership actually remains after successful leader reap")
			var ownership *OwnershipError
			Expect(errors.As(err, &ownership)).To(BeTrue(), "signal acknowledgement plus a completed leader cannot prove the actual child group stopped")
			Expect(ownership.ProcessPID).To(Equal(leader))
			Expect(ownershipDiagnostic(ownership)).To(Equal(fmt.Sprintf("pid=%d phase=group signal_errno=none reaped=true exit_confirmed=true drained=true group_state=present group_errno=none", leader)), "real group survival must remain distinct from signal or pipe uncertainty")
			Expect(result.OwnershipUnconfirmed).To(BeTrue())
			Expect(result.Outcome).NotTo(Equal("completed"))
		} else {
			Expect(err).NotTo(HaveOccurred())
			Expect(result.OwnershipUnconfirmed).To(BeFalse())
			Expect(result.Outcome).To(Equal("completed"))
		}
	}, Entry("acknowledged signal leaves a real same-group descendant executing", true), Entry("actual group termination is a successful paired control", false))
})
