package native

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestNativeOwnership(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Native ownership collaborator acceptance")
}

// per docs/adr/0102-parallel-go-qualification.md:117
func ownershipDiagnostic(ownership *OwnershipError) string {
	GinkgoHelper()
	Expect(ownership.Error()).To(Equal("native process ownership is unconfirmed"))
	diagnostic, ok := any(ownership).(interface{ SafeDiagnostic() string })
	Expect(ok).To(BeTrue(), "typed ownership uncertainty must expose the safe diagnostic accessor")
	return diagnostic.SafeDiagnostic()
}

var _ = Describe("Native ownership uncertainty", func() {
	// per docs/adr/0068-go-native-harness-execution.md:44
	// per docs/adr/0102-parallel-go-qualification.md:117
	// per docs/adr/0102-parallel-go-qualification.md:125
	// per docs/adr/0103-darwin-natural-exit-reconciliation.md:42
	DescribeTable("preserves typed uncertainty after deterministic cleanup-report failure", func(boundary string) {
		root, err := os.MkdirTemp("", "factory-native-ownership-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, root)
		plan := Plan{Harness: "opencode", Role: "reviewer", Root: root, Argv: []string{"/bin/sh", "-c", "printf PRIVATE_CHILD_OUTPUT; exit 0", "PRIVATE_COMMAND_OPERAND"}, Stdin: "PRIVATE_STDIN", Environment: map[string]string{"PRIVATE_ENV": "PRIVATE_ENVIRONMENT_VALUE"}}
		ops := defaultProcessOps()
		if boundary != "wait" {
			original := ops.signalGroup
			ops.signalGroup = func(pid int) error {
				if err := original(pid); err != nil {
					return err
				}
				if boundary == "signal_unknown" {
					return errors.New("PRIVATE_SIGNAL_FAILURE " + root)
				}
				return fmt.Errorf("PRIVATE_SIGNAL_FAILURE %s: %w", root, syscall.EACCES)
			}
		} else {
			original := ops.wait
			ops.wait = func(command *exec.Cmd) error {
				_ = original(command)
				return errors.New("PRIVATE_WAIT_FAILURE " + root)
			}
		}
		// Failure is injected only after real OS cleanup, so evaluator faults cannot
		// leave a live orphan. No global hook or production environment override.
		pid := 0
		result, err := execute(context.Background(), plan, time.Second, func(_ context.Context, id int) error { pid = id; return nil }, ops)
		Expect(pid).To(BeNumerically(">", 0))
		Expect(syscall.Kill(pid, 0)).To(Equal(syscall.ESRCH), "independently establish the real child was reaped")
		var ownership *OwnershipError
		Expect(errors.As(err, &ownership)).To(BeTrue())
		Expect(ownership.ProcessPID).To(Equal(pid))
		Expect(result.ProcessPID).To(Equal(pid))
		Expect(result.OwnershipUnconfirmed).To(BeTrue())
		Expect(result.Outcome).NotTo(Equal("completed"))
		if boundary == "wait" {
			Expect(result.ExitCode).To(BeNil())
			Expect(result.ExitConfirmed).To(BeFalse())
		} else {
			Expect(result.ExitConfirmed).To(BeTrue())
		}
		Expect(string(result.Stdout)).To(Equal("PRIVATE_CHILD_OUTPUT"))
		phase, errno := "signal", fmt.Sprint(int(syscall.EACCES))
		switch boundary {
		case "wait":
			phase, errno = "reap", "none"
		case "signal_unknown":
			errno = "unknown"
		}
		Expect(ownershipDiagnostic(ownership)).To(Equal(fmt.Sprintf("pid=%d phase=%s signal_errno=%s reaped=true exit_confirmed=%t drained=true group_state=not_observed group_errno=none reaped_before_signal=true", pid, phase, errno, result.ExitConfirmed)), "diagnostics must contain only the actual failed cleanup phase and allowlisted facts")
		Expect(ownershipDiagnostic(&OwnershipError{ProcessPID: pid})).To(Equal(fmt.Sprintf("pid=%d reason=unavailable", pid)), "externally constructed errors have no supervisor observation to invent")
	}, Entry("group signaling report", "signal"), Entry("leader reap report", "wait"), Entry("unknown group signaling report remains private", "signal_unknown"))
})

var _ = Describe("Native final capture error", func() {
	// per docs/adr/0068-go-native-harness-execution.md:99
	It("retains a parser failure delivered after real leader reap", func() {
		root, err := os.MkdirTemp("", "factory-native-capture-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, root)
		plan := Plan{Harness: "codex", Role: "implementer", Root: root, Argv: []string{"/bin/sh", "-c", "printf response"}, Environment: map[string]string{}}
		ops := defaultProcessOps()
		original := ops.signalGroup
		signalComplete := make(chan struct{})
		ops.signalGroup = func(pid int) error { err := original(pid); close(signalComplete); return err }
		options := runOptions{limit: 1024, consume: func(_ context.Context, _ []byte) (bool, error) {
			<-signalComplete
			return false, errors.New("PRIVATE_PARSE_ERROR")
		}}
		pid := 0
		result, err := supervise(context.Background(), plan, time.Second, func(_ context.Context, id int) error { pid = id; return nil }, ops, options)
		Expect(syscall.Kill(pid, 0)).To(Equal(syscall.ESRCH))
		Expect(result.ExitConfirmed).To(BeTrue())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).NotTo(ContainSubstring("PRIVATE"))
		Expect(result.Outcome).NotTo(Equal("completed"))
	})
})

var _ = Describe("Native incomplete capture ownership", func() {
	// per docs/adr/0068-go-native-harness-execution.md:44
	// per docs/adr/0102-parallel-go-qualification.md:117
	// per docs/adr/0102-parallel-go-qualification.md:125
	It("refuses success while a capability consumer remains unfinished after cleanup expiry", func() {
		root, err := os.MkdirTemp("", "factory-native-incomplete-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, root)
		plan := Plan{Harness: "codex", Role: "implementer", Root: root, Argv: []string{"/bin/sh", "-c", "printf response"}, Environment: map[string]string{}}
		release, finished := make(chan struct{}), make(chan struct{})
		var once sync.Once
		defer once.Do(func() { close(release) })
		ops := defaultProcessOps()
		ops.waitLimit = 20 * time.Millisecond
		options := runOptions{limit: 1024, consume: func(_ context.Context, _ []byte) (bool, error) { defer close(finished); <-release; return true, nil }}
		result, err := supervise(context.Background(), plan, time.Second, func(_ context.Context, _ int) error { return nil }, ops, options)
		once.Do(func() { close(release) })
		Eventually(finished, time.Second).Should(BeClosed())
		Expect(syscall.Kill(result.ProcessPID, 0)).To(Equal(syscall.ESRCH))
		Expect(result.ExitConfirmed).To(BeTrue())
		var ownership *OwnershipError
		Expect(errors.As(err, &ownership)).To(BeTrue())
		Expect(ownership.ProcessPID).To(Equal(result.ProcessPID))
		Expect(result.OwnershipUnconfirmed).To(BeTrue())
		Expect(result.Outcome).NotTo(Equal("completed"))
		Expect(ownershipDiagnostic(ownership)).To(Equal(fmt.Sprintf("pid=%d phase=pipe_drain signal_errno=none reaped=true exit_confirmed=true drained=false group_state=not_observed group_errno=none reaped_before_signal=true", result.ProcessPID)))
	})
})
