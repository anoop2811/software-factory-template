package native

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Darwin natural exit reconciliation", func() {
	// per docs/adr/0103-darwin-natural-exit-reconciliation.md:31
	// per docs/adr/0103-darwin-natural-exit-reconciliation.md:40
	// per docs/adr/0103-darwin-natural-exit-reconciliation.md:53
	DescribeTable("requires actual prior reap and terminal absence while preserving the leader status", func(code int) {
		plan := Plan{Harness: "opencode", Role: "reviewer", Root: GinkgoT().TempDir(), Argv: []string{"/bin/sh", "-c", fmt.Sprintf("printf PRIVATE_RESULT; exit %d", code)}, Environment: map[string]string{}}
		ops := defaultProcessOps()
		waited := make(chan struct{})
		originalWait := ops.wait
		ops.wait = func(command *exec.Cmd) error { err := originalWait(command); close(waited); return err }
		signals := 0
		ops.signalGroup = func(pid int) error {
			signals++
			Expect(waited).To(BeClosed(), "the real leader must already be reaped before the EPERM report")
			Expect(syscall.Kill(pid, 0)).To(Equal(syscall.ESRCH))
			Expect(syscall.Kill(-pid, 0)).To(Equal(syscall.ESRCH), "the independent real terminal group observation must actually be absent")
			return fmt.Errorf("PRIVATE_SIGNAL_FAILURE: %w", syscall.EPERM)
		}
		result, err := execute(context.Background(), plan, time.Second, func(context.Context, int) error { return nil }, ops)
		Expect(signals).To(Equal(1), "reconciliation must not skip or retry the termination attempt")
		Expect(result.ExitConfirmed).To(BeTrue())
		Expect(result.ExitCode).NotTo(BeNil())
		Expect(*result.ExitCode).To(Equal(code))
		Expect(string(result.Stdout)).To(Equal("PRIVATE_RESULT"))
		if runtime.GOOS == "darwin" {
			Expect(err).NotTo(HaveOccurred(), "a real closed group and confirmed natural exit permit this exact Darwin reconciliation")
			Expect(result.OwnershipUnconfirmed).To(BeFalse())
			outcome := "completed"
			if code != 0 {
				outcome = "failed"
			}
			Expect(result.Outcome).To(Equal(outcome))
		} else {
			var ownership *OwnershipError
			Expect(errors.As(err, &ownership)).To(BeTrue(), "other platforms retain the original signal error refusal")
			Expect(result.OwnershipUnconfirmed).To(BeTrue())
		}
	}, Entry("zero exit remains successful only with full Darwin terminal proof", 0), Entry("nonzero exit is retained by the same terminal proof", 7))
})

var _ = Describe("Darwin reconciliation refusal", func() {
	// per docs/adr/0103-darwin-natural-exit-reconciliation.md:34
	// per docs/adr/0103-darwin-natural-exit-reconciliation.md:41
	// per docs/adr/0103-darwin-natural-exit-reconciliation.md:57
	DescribeTable("retains typed EPERM uncertainty outside the exact natural terminal proof", func(boundary string) {
		root := GinkgoT().TempDir()
		plan := Plan{Harness: "opencode", Role: "reviewer", Root: root, Argv: []string{"/bin/sh", "-c", "printf PRIVATE_RESULT; exit 0"}, Environment: map[string]string{}}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if boundary == "timeout" {
			ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
		}
		ops := defaultProcessOps()
		options := runOptions{limit: 1024}
		onSpawn := func(context.Context, int) error { return nil }
		preReap := false
		switch boundary {
		case "publication":
			preReap = true
			plan.Argv = []string{"/bin/sh", "-c", "read line"}
			onSpawn = func(context.Context, int) error { return errors.New("PRIVATE_PUBLICATION_FAILURE") }
		case "cancel":
			preReap = true
			plan.Argv = []string{"/bin/sh", "-c", "read line"}
			onSpawn = func(context.Context, int) error { cancel(); return nil }
		case "timeout":
			preReap = true
			plan.Argv = []string{"/bin/sh", "-c", "read line"}
			onSpawn = func(context.Context, int) error { <-ctx.Done(); return nil }
		case "signalled":
			plan.Argv = []string{"/bin/sh", "-c", "read line"}
			onSpawn = func(_ context.Context, pid int) error { return syscall.Kill(pid, syscall.SIGTERM) }
		case "overflow":
			preReap = true
			plan.Argv = []string{"/bin/sh", "-c", "printf '%02048d' 0; while :; do :; done"}
			options.limit = 16
		case "expired-cleanup":
			ops.waitLimit = -time.Nanosecond
		}
		release, finished := make(chan struct{}), make(chan struct{})
		var releaseOnce sync.Once
		defer releaseOnce.Do(func() { close(release) })
		if boundary == "unfinished-capture" {
			ops.waitLimit = 20 * time.Millisecond
			options.consume = func(context.Context, []byte) (bool, error) { defer close(finished); <-release; return false, nil }
		}
		waited := make(chan struct{})
		originalWait := ops.wait
		ops.wait = func(command *exec.Cmd) error {
			err := originalWait(command)
			close(waited)
			if boundary == "unknown-exit" {
				return errors.New("PRIVATE_WAIT_FAILURE " + root)
			}
			return err
		}
		originalSignal := ops.signalGroup
		signals := 0
		ops.signalGroup = func(pid int) error {
			signals++
			if preReap {
				// The real live child is terminated before the synthetic report is returned.
				Expect(originalSignal(pid)).To(Succeed())
			} else {
				Expect(waited).To(BeClosed(), "this boundary must hold the real prior-reap prerequisite")
				Expect(syscall.Kill(pid, 0)).To(Equal(syscall.ESRCH))
			}
			if boundary == "late-cancel" {
				cancel()
			}
			return fmt.Errorf("PRIVATE_SIGNAL_FAILURE %s: %w", root, syscall.EPERM)
		}
		result, err := supervise(ctx, plan, time.Second, onSpawn, ops, options)
		releaseOnce.Do(func() { close(release) })
		if boundary == "unfinished-capture" {
			Eventually(finished, time.Second).Should(BeClosed())
		}
		Expect(signals).To(Equal(1))
		Expect(syscall.Kill(result.ProcessPID, 0)).To(Equal(syscall.ESRCH), "the fault fixture's real leader must still be reaped")
		var ownership *OwnershipError
		Expect(errors.As(err, &ownership)).To(BeTrue(), "EPERM cannot authorize success at boundary %s", boundary)
		Expect(ownership.ProcessPID).To(Equal(result.ProcessPID))
		Expect(result.OwnershipUnconfirmed).To(BeTrue())
		Expect(result.Outcome).NotTo(Equal("completed"))
		diagnostic := ownershipDiagnostic(ownership)
		Expect(diagnostic).To(ContainSubstring("signal_errno=" + fmt.Sprint(int(syscall.EPERM))))
		Expect(diagnostic).To(ContainSubstring(fmt.Sprintf("reaped_before_signal=%t", !preReap)))
		Expect(diagnostic).To(ContainSubstring("group_state=not_observed"), "ineligible cleanup must not invent an ESRCH observation")
		Expect(diagnostic).NotTo(ContainSubstring("PRIVATE"))
		Expect(diagnostic).NotTo(ContainSubstring(root))
		switch boundary {
		case "unknown-exit":
			Expect(result.ExitConfirmed).To(BeFalse())
			Expect(result.ExitCode).To(BeNil())
		case "signalled":
			Expect(result.ExitConfirmed).To(BeTrue())
			Expect(result.ExitCode).NotTo(BeNil())
			Expect(*result.ExitCode).To(Equal(-int(syscall.SIGTERM)))
		case "unfinished-capture":
			Expect(diagnostic).To(ContainSubstring("drained=false"))
		case "cancel":
			Expect(result.Outcome).To(Equal("interrupted"))
		case "timeout":
			Expect(result.Outcome).To(Equal("timeout"))
		case "overflow":
			Expect(result.Outcome).To(Equal("output_limit"))
		}
	}, Entry("failed publication signals before reap", "publication"), Entry("cancellation before reap", "cancel"), Entry("timeout before reap", "timeout"), Entry("cancellation after prior reap", "late-cancel"), Entry("externally signalled leader", "signalled"), Entry("unknown actual exit report", "unknown-exit"), Entry("unfinished capture after reap", "unfinished-capture"), Entry("expired shared cleanup allowance", "expired-cleanup"), Entry("output overflow before reap", "overflow"))
})
