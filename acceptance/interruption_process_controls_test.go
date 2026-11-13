package acceptance_test

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
		// Keep the borrowed-group control outside the evaluator group, including
		// under deliberate group-target negative controls.
		// docs/adr/0151-reliable-ci-qualification.md:244.
		canary.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		canary.Stdin, canary.Stdout, canary.Stderr = canaryRead, null, null
		Expect(canary.Start()).To(Succeed())
		DeferCleanup(func() {
			_ = syscall.Kill(-canary.Process.Pid, syscall.SIGKILL)
			_ = canary.Wait()
			Expect(canaryRead.Close()).To(Succeed())
			Expect(canaryWrite.Close()).To(Succeed())
		})
		// docs/adr/0151-reliable-ci-qualification.md:244 -- use a real held pipe
		// to reject release-before-resume without signaling an unowned process.
		for _, mode := range []string{"live", "stopped", "resume-error"} {
			reader, writer, err := os.Pipe()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				Expect(reader.Close()).To(Succeed())
				closeErr := writer.Close()
				Expect(closeErr == nil || errors.Is(closeErr, os.ErrClosed)).To(BeTrue())
			})
			resumeCalls := 0
			resumeFailure := errors.New("controlled resume refusal")
			var resume func() error
			if mode != "live" {
				resume = func() error {
					resumeCalls++
					if _, err := writer.Stat(); err != nil {
						return err
					}
					if mode == "resume-error" {
						return resumeFailure
					}
					return nil
				}
			}
			release := sourceHeldChildRelease(writer, resume)
			first := release()
			if mode == "resume-error" {
				Expect(errors.Is(first, resumeFailure)).To(BeTrue())
			} else {
				Expect(first).NotTo(HaveOccurred(), "resume must run while the release pipe remains held")
			}
			_, err = writer.Stat()
			Expect(errors.Is(err, os.ErrClosed)).To(BeTrue(), "release must close even after an unconfirmed resume")
			var token [1]byte
			count, readErr := reader.Read(token[:])
			Expect(count).To(BeZero())
			Expect(readErr).To(Equal(io.EOF))
			second := release()
			if first == nil {
				Expect(second).NotTo(HaveOccurred(), "repeated successful release must remain successful")
			} else {
				Expect(second).To(Equal(first), "repeated release must retain its original error without signaling again")
			}
			if mode == "live" {
				Expect(resumeCalls).To(BeZero())
			} else {
				Expect(resumeCalls).To(Equal(1))
			}
		}
		for _, mode := range []string{"exited", "stopped", "canceled", "direct-exited", "direct-stopped", "direct-canceled"} {
			direct := strings.HasPrefix(mode, "direct-")
			behavior := strings.TrimPrefix(mode, "direct-")
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			input, writer, err := os.Pipe()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { Expect(input.Close()).To(Succeed()); Expect(writer.Close()).To(Succeed()) })
			ready := filepath.Join(root, mode+"-ready")
			script := "printf ready > \"$1\"; read token"
			if behavior == "exited" {
				script += "; exit 23"
			}
			startOwner := startCIInterruptionOwner
			if direct {
				startOwner = startDirectInterruptionOwner
			}
			owner, err := startOwner(ctx, func(childContext context.Context) *exec.Cmd {
				command := exec.CommandContext(childContext, "/bin/sh", "-c", script, "owned-fixture", ready) // #nosec G204 -- fixed evaluator-owned scripts and a private readiness path; no user command text.
				command.Stdin, command.Stdout, command.Stderr = input, null, null
				if direct {
					command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: canary.Process.Pid}
				}
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
			if direct {
				Expect(group).To(Equal(canary.Process.Pid), "direct custody must preserve the deliberately borrowed group")
			} else {
				Expect(group).To(Equal(owner.PID()))
			}
			Expect(group).NotTo(Equal(syscall.Getpgrp()))
			_, joinErr := owner.JoinExited()
			Expect(joinErr).To(MatchError(errCIInterruptionOwnerRunning), "a live join refusal must preserve custody")
			Expect(owner.Signal(0)).To(Succeed(), "live join refusal must not close signaling")
			_, awaitErr := owner.AwaitExit(5 * time.Millisecond)
			Expect(awaitErr).To(MatchError(context.DeadlineExceeded), "bounded live observation must leave the child owned")
			Expect(owner.Signal(0)).To(Succeed(), "observation timeout must not consume signal authority")
			Expect(owner.Exited()).To(BeFalse())
			switch behavior {
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
			var result ciInterruptionResult
			if direct && behavior == "exited" {
				result, err = owner.JoinExited()
				Expect(err).NotTo(HaveOccurred())
			} else {
				result = owner.TerminateAndWait()
			}
			Expect(result.Joined).To(BeTrue(), mode)
			if !direct && behavior == "exited" && runtime.GOOS == "darwin" {
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
			if behavior == "exited" {
				Expect(result.State.ExitCode()).To(Equal(23))
			} else {
				Expect(status.Signaled()).To(BeTrue())
				Expect(status.Signal()).To(Equal(syscall.SIGKILL))
			}
			Expect(owner.TerminateAndWait()).To(Equal(result), "repeated cleanup must reuse the terminal result")
			joinedResult, joinErr := owner.JoinExited()
			Expect(joinErr).NotTo(HaveOccurred())
			Expect(joinedResult).To(Equal(result), "terminal joins must reuse the same published result")
			Expect(owner.Signal(0)).To(MatchError(os.ErrProcessDone), "closed custody must refuse even a non-mutating probe")
			_, err = owner.Exited()
			Expect(err).To(MatchError(os.ErrProcessDone))
			Expect(syscall.Kill(owner.PID(), 0)).To(Equal(syscall.ESRCH))
			Expect(syscall.Kill(-owner.PID(), 0)).To(Equal(syscall.ESRCH))
			Expect(canary.Process.Signal(syscall.Signal(0))).To(Succeed(), "the unrelated process must survive every cleanup")
			Expect(installationProcessExited(canary.Process.Pid)).To(BeFalse(), "an unreaped zombie is not a surviving borrowed-group canary")
		}
	})
})
