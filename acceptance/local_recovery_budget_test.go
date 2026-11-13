package acceptance_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/anoop2811/software-factory-template/internal/qualification"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type recoveryBudgetActor struct {
	root   string
	owner  *ciInterruptionOwner
	writer *os.File
}

// A real held child publishes only evaluator-requested receipt files. It has no
// compiler or product shortcuts, and owns no sleeping descendant.
// per docs/adr/0152-phase-bounded-local-recovery-qualification.md:55
func newRecoveryBudgetActor() *recoveryBudgetActor {
	GinkgoHelper()
	root, _ := fixtureDirectories()
	input, writer, err := os.Pipe()
	Expect(err).NotTo(HaveOccurred())
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { Expect(null.Close()).To(Succeed()) })
	DeferCleanup(func() { err := input.Close(); Expect(err == nil || errors.Is(err, os.ErrClosed)).To(BeTrue()) })
	const script = `set -eu
printf 'held\n' > "$1/child.pending"
/bin/mv "$1/child.pending" "$1/child"
while IFS= read -r command; do
  case "$command" in
    admission|ready|timing)
      printf '%s\n' "$command" > "$1/$command.pending"
      /bin/mv "$1/$command.pending" "$1/$command"
      ;;
    fail) exit 23 ;;
    *) exit 99 ;;
  esac
done
`
	owner, err := startDirectInterruptionOwner(context.Background(), func(ctx context.Context) *exec.Cmd {
		command := exec.CommandContext(ctx, "/bin/sh", "-c", script, "recovery-budget-actor", root) // #nosec G204 -- fixed evaluator script and a private root passed as literal argv.
		command.Stdin, command.Stdout, command.Stderr = input, null, null
		return command
	})
	Expect(err).NotTo(HaveOccurred())
	actor := &recoveryBudgetActor{root: root, owner: owner, writer: writer}
	DeferCleanup(actor.close)
	Expect(input.Close()).To(Succeed())
	Eventually(func() ([]byte, error) { return os.ReadFile(filepath.Join(root, "child")) }, 3*time.Second).Should(Equal([]byte("held\n")))
	return actor
}

func (actor *recoveryBudgetActor) close() {
	GinkgoHelper()
	result := actor.owner.TerminateAndWait()
	if !result.Joined || result.SignalError != nil {
		holdFixtureCleanup(actor.root, "recovery budget actor custody could not be confirmed")
	}
	closeErr := actor.writer.Close()
	Expect(closeErr == nil || errors.Is(closeErr, os.ErrClosed)).To(BeTrue())
	Expect(result.Joined).To(BeTrue())
	Expect(result.SignalError).NotTo(HaveOccurred())
}

func (actor *recoveryBudgetActor) send(command string) error {
	_, err := actor.writer.WriteString(command + "\n")
	return err
}

func (actor *recoveryBudgetActor) observe(name string) func() (bool, error) {
	return func() (bool, error) {
		body, err := os.ReadFile(filepath.Join(actor.root, name))
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if string(body) != name+"\n" {
			return false, fmt.Errorf("invalid actual %s receipt: %q", name, body)
		}
		return true, nil
	}
}

func (actor *recoveryBudgetActor) exited() error {
	exited, err := actor.owner.Exited()
	if err != nil {
		return err
	}
	if !exited {
		return nil
	}
	result, err := actor.owner.JoinExited()
	if err != nil {
		return err
	}
	if !result.Joined {
		return errors.New("recovery budget actor exit could not be joined")
	}
	return fmt.Errorf("actual recovery actor exited: %w", result.WaitError)
}

// Timer goroutines only deliver literal pipe messages; cleanup cancels and joins
// every sender before destroying the actor's private input.
// per docs/adr/0152-phase-bounded-local-recovery-qualification.md:55
func recoveryBudgetPublishAfter(actor *recoveryBudgetActor, delay time.Duration, command string) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			done <- ctx.Err()
		case <-timer.C:
			done <- actor.send(command)
		}
	}()
	DeferCleanup(func() {
		cancel()
		err := <-done
		Expect(err == nil || errors.Is(err, context.Canceled)).To(BeTrue())
	})
}

var _ = Describe("Local recovery qualification budgets", func() {
	// per docs/adr/0152-phase-bounded-local-recovery-qualification.md:31
	// per docs/adr/0152-phase-bounded-local-recovery-qualification.md:45
	// per docs/adr/0152-phase-bounded-local-recovery-qualification.md:55
	It("keeps real admission publication and child-exit observations within one non-resetting command budget", func() {
		defaults := qualification.LocalRecoveryLimits()
		limits := qualification.RecoveryLimits{Preview: defaults.Preview / 30, Admission: defaults.Admission / 30, Publication: defaults.Publication / 30, Command: defaults.Command / 30}
		actor := newRecoveryBudgetActor()
		outer, cancel := limits.Context(context.Background())
		DeferCleanup(cancel)
		started := time.Now()
		recoveryBudgetPublishAfter(actor, 3200*time.Millisecond, "admission")
		Expect(limits.AwaitAdmission(outer, actor.observe("admission"), actor.exited)).To(Succeed(), "actual admission after the old scaled90s cap must still qualify")
		admissionElapsed := time.Since(started)
		Expect(admissionElapsed).To(BeNumerically(">", 3*time.Second))
		Expect(actor.send("ready")).To(Succeed())
		Expect(limits.AwaitPublication(outer, actor.observe("ready"), actor.exited)).To(Succeed())
		Expect(outer.Err()).NotTo(HaveOccurred())
		AddReportEntry("local-recovery.phase-budget-control", map[string]any{"admission_seconds": admissionElapsed.Seconds(), "total_seconds": time.Since(started).Seconds(), "scaled_command_seconds": limits.Command.Seconds(), "actor_pid": actor.owner.PID()})
		actor.close()
		cancel()
		Expect(defaults).To(Equal(qualification.RecoveryLimits{Preview: 120 * time.Second, Admission: 120 * time.Second, Publication: 60 * time.Second, Command: 180 * time.Second}))

		short := qualification.RecoveryLimits{Preview: 100 * time.Millisecond, Admission: 100 * time.Millisecond, Publication: 100 * time.Millisecond, Command: 2 * time.Second}
		preview, cancelPreview := short.PreviewContext(context.Background())
		<-preview.Done()
		Expect(preview.Err()).To(MatchError(context.DeadlineExceeded))
		cancelPreview()
		for _, mode := range []string{"absent admission", "timing only", "absent publication"} {
			actor = newRecoveryBudgetActor()
			if mode == "timing only" {
				Expect(actor.send("timing")).To(Succeed())
				Eventually(actor.observe("timing"), time.Second).Should(BeTrue())
			}
			if mode == "absent publication" {
				Expect(actor.send("admission")).To(Succeed())
				Eventually(actor.observe("admission"), time.Second).Should(BeTrue())
			}
			outer, cancel = short.Context(context.Background())
			DeferCleanup(cancel)
			phase := "admission"
			wait := short.AwaitAdmission
			observe := actor.observe("admission")
			if mode == "absent publication" {
				Expect(short.AwaitAdmission(outer, actor.observe("admission"), actor.exited)).To(Succeed())
				phase, wait, observe = "publication", short.AwaitPublication, actor.observe("ready")
			}
			started = time.Now()
			err := wait(outer, observe, actor.exited)
			Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue(), "%s: %v", mode, err)
			Expect(err.Error()).To(ContainSubstring(phase))
			Expect(time.Since(started)).To(BeNumerically(">=", 50*time.Millisecond))
			Expect(outer.Err()).NotTo(HaveOccurred(), "the phase bound must win before the command bound")
			actor.close()
			cancel()
		}

		// Receipt files arriving after cancellation must not bypass the same
		// outer context, even when a separately restarted phase would accept them.
		// per docs/adr/0152-phase-bounded-local-recovery-qualification.md:34
		for _, mode := range []string{"parent cancellation", "parent deadline", "outer deadline"} {
			actor = newRecoveryBudgetActor()
			// Establish the real prerequisite before arming the deliberately
			// short publication deadline; only the tested phase consumes it.
			// per docs/adr/0152-phase-bounded-local-recovery-qualification.md:33
			if mode != "parent cancellation" {
				Expect(actor.send("admission")).To(Succeed())
				Eventually(actor.observe("admission"), time.Second).Should(BeTrue())
			}
			bounded := qualification.RecoveryLimits{Preview: time.Second, Admission: time.Second, Publication: time.Second, Command: 2 * time.Second}
			parent, cancelParent := context.WithCancel(context.Background())
			if mode == "parent deadline" {
				cancelParent()
				parent, cancelParent = context.WithTimeout(context.Background(), 100*time.Millisecond)
			}
			DeferCleanup(cancelParent)
			if mode == "outer deadline" {
				bounded.Command = 150 * time.Millisecond
			}
			outer, cancel = bounded.Context(parent)
			DeferCleanup(cancel)
			parentPreview, stopPreview := bounded.PreviewContext(parent)
			DeferCleanup(stopPreview)
			phase, command := "publication", "ready"
			wait, observe := bounded.AwaitPublication, actor.observe("ready")
			wanted := context.DeadlineExceeded
			if mode == "parent cancellation" {
				phase, command, wait, observe = "admission", "admission", bounded.AwaitAdmission, actor.observe("admission")
				wanted = context.Canceled
				timer := time.AfterFunc(100*time.Millisecond, cancelParent)
				DeferCleanup(func() { timer.Stop() })
			} else {
				Expect(bounded.AwaitAdmission(outer, actor.observe("admission"), actor.exited)).To(Succeed())
			}
			published := make(chan error, 1)
			go func(child *recoveryBudgetActor, commandContext context.Context, message string) {
				<-commandContext.Done()
				published <- child.send(message)
			}(actor, outer, command)
			stopPublication := cancel
			DeferCleanup(func() { stopPublication(); Expect(<-published).To(Succeed()) })
			err := wait(outer, observe, actor.exited)
			Expect(errors.Is(err, wanted)).To(BeTrue(), "%s: %v", mode, err)
			Expect(err.Error()).To(ContainSubstring(phase))
			Eventually(observe, time.Second).Should(BeTrue(), "the late receipt must actually exist after the refused context")
			Expect(outer.Err()).To(MatchError(wanted))
			if mode != "outer deadline" {
				Expect(parentPreview.Err()).To(MatchError(wanted), "preview must also inherit earlier parent cancellation/deadline")
			}
			actor.close()
			cancel()
			cancelParent()
		}

		for _, phase := range []string{"admission", "publication"} {
			actor = newRecoveryBudgetActor()
			if phase == "publication" {
				Expect(actor.send("admission")).To(Succeed())
				Eventually(actor.observe("admission"), time.Second).Should(BeTrue())
			}
			bounded := qualification.RecoveryLimits{Admission: 5 * time.Second, Publication: 5 * time.Second, Command: 10 * time.Second}
			outer, cancel = bounded.Context(context.Background())
			DeferCleanup(cancel)
			wait, observe := bounded.AwaitAdmission, actor.observe("admission")
			if phase == "publication" {
				Expect(bounded.AwaitAdmission(outer, actor.observe("admission"), actor.exited)).To(Succeed())
				wait, observe = bounded.AwaitPublication, actor.observe("ready")
			}
			Expect(actor.send("fail")).To(Succeed())
			Eventually(actor.owner.Exited, time.Second).Should(BeTrue(), "establish the actual terminal child before observing its exit cause")
			err := wait(outer, observe, actor.exited)
			var exit *exec.ExitError
			Expect(errors.As(err, &exit)).To(BeTrue(), "real child-exit cause must survive: %v", err)
			Expect(exit.ExitCode()).To(Equal(23))
			Expect(err.Error()).To(ContainSubstring(phase))
			Expect(outer.Err()).NotTo(HaveOccurred())
			actor.close()
			cancel()
		}
	})
})
