package acceptance_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Interruption fixture child custody", func() {
	// docs/adr/0151-reliable-ci-qualification.md:193 -- direct child identity remains reserved until all signals end.
	It("retains unreaped identities through exit stop and cancellation and closes signaling before joining", func() {
		root, _ := fixtureDirectories()
		null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(null.Close()).To(Succeed()) })
		canaryRead, canaryWrite, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		canary := exec.Command("/bin/sh", "-c", "read token")
		canary.Stdin, canary.Stdout, canary.Stderr = canaryRead, null, null
		Expect(canary.Start()).To(Succeed())
		DeferCleanup(func() {
			_ = canary.Process.Kill()
			_ = canary.Wait()
			Expect(canaryRead.Close()).To(Succeed())
			Expect(canaryWrite.Close()).To(Succeed())
		})
		for _, mode := range []string{"exited", "stopped", "canceled"} {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			input, writer, err := os.Pipe()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { Expect(input.Close()).To(Succeed()); Expect(writer.Close()).To(Succeed()) })
			ready := filepath.Join(root, mode+"-ready")
			script := "printf ready > \"$1\"; read token"
			if mode == "exited" {
				script += "; exit 23"
			}
			owner, err := startCIInterruptionOwner(ctx, func(childContext context.Context) *exec.Cmd {
				command := exec.CommandContext(childContext, "/bin/sh", "-c", script, "owned-fixture", ready) // #nosec G204 -- fixed evaluator-owned scripts and a private readiness path; no user command text.
				command.Stdin, command.Stdout, command.Stderr = input, null, null
				return command
			})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				result := owner.TerminateAndWait()
				if !result.Joined {
					holdFixtureCleanup(root, "interruption control could not join its direct child")
				}
				Expect(result.Joined).To(BeTrue())
			})
			Eventually(func() bool { _, err := os.Stat(ready); return err == nil }, 3*time.Second).Should(BeTrue())
			group, err := syscall.Getpgid(owner.PID())
			Expect(err).NotTo(HaveOccurred())
			Expect(group).To(Equal(owner.PID()))
			Expect(group).NotTo(Equal(syscall.Getpgrp()))
			switch mode {
			case "exited":
				_, err := writer.WriteString("release natural exit\n")
				Expect(err).NotTo(HaveOccurred())
				Eventually(owner.Exited, 3*time.Second).Should(BeTrue())
				Consistently(owner.Exited, 25*time.Millisecond, time.Millisecond).Should(BeTrue(), "terminal observation must not reap the identity")
				Expect(owner.command.ProcessState).To(BeNil(), "no concurrent Cmd.Wait may publish a reaped state")
			case "stopped":
				Expect(owner.Signal(syscall.SIGSTOP)).To(Succeed())
				Eventually(func() (bool, error) { return installationProcessStopped(owner.PID()) }, 3*time.Second).Should(BeTrue())
				Expect(owner.Exited()).To(BeFalse())
			default:
				cancel()
				Eventually(func() bool { _, err := owner.Exited(); return errors.Is(err, os.ErrProcessDone) }, 3*time.Second).Should(BeTrue())
			}
			result := owner.TerminateAndWait()
			Expect(result.Joined).To(BeTrue(), mode)
			if mode == "exited" && runtime.GOOS == "darwin" {
				// Darwin can reject a group signal after natural exit. Preserve that
				// error; the transaction caller retains inputs instead of hiding it.
				Expect(result.SignalError).To(Or(BeNil(), Equal(syscall.EPERM)))
			} else {
				Expect(result.SignalError).NotTo(HaveOccurred(), mode)
			}
			Expect(result.WaitError).To(HaveOccurred(), mode)
			Expect(result.State).NotTo(BeNil())
			status, ok := result.State.Sys().(syscall.WaitStatus)
			Expect(ok).To(BeTrue())
			if mode == "exited" {
				Expect(result.State.ExitCode()).To(Equal(23))
			} else {
				Expect(status.Signaled()).To(BeTrue())
				Expect(status.Signal()).To(Equal(syscall.SIGKILL))
			}
			Expect(owner.TerminateAndWait()).To(Equal(result), "repeated cleanup must reuse the terminal result")
			Expect(owner.Signal(0)).To(MatchError(os.ErrProcessDone), "closed custody must refuse even a non-mutating probe")
			_, err = owner.Exited()
			Expect(err).To(MatchError(os.ErrProcessDone))
			Expect(syscall.Kill(owner.PID(), 0)).To(Equal(syscall.ESRCH))
			Expect(syscall.Kill(-owner.PID(), 0)).To(Equal(syscall.ESRCH))
			Expect(canary.Process.Signal(syscall.Signal(0))).To(Succeed(), "the unrelated process must survive every cleanup")
		}
	})
})
