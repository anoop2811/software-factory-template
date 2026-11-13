package acceptance_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// The direct child reserves its PID and process-group identity until the sole
// wait begins. All signaling is permanently closed before that boundary.
// per docs/adr/0151-reliable-ci-qualification.md:193
type ciInterruptionOwner struct {
	command        *exec.Cmd
	terminationPID int
	joinAllowance  time.Duration
	mutex          sync.Mutex
	closed         bool
	done           chan struct{}
	result         ciInterruptionResult
}

type ciInterruptionResult struct {
	Joined      bool
	SignalError error
	WaitError   error
	State       *os.ProcessState
}

// The builder receives an uncancelled context: this owner, not CommandContext,
// sequences cancellation signals before reaping.
// per docs/adr/0151-reliable-ci-qualification.md:193
func startCIInterruptionOwner(ctx context.Context, build func(context.Context) *exec.Cmd) (*ciInterruptionOwner, error) {
	return startInterruptionOwner(ctx, build, true)
}

// A borrowed process group is never a cleanup target. Preserve the command's
// supplied process attributes and reserve only its direct child identity.
// per docs/adr/0151-reliable-ci-qualification.md:252
func startDirectInterruptionOwner(ctx context.Context, build func(context.Context) *exec.Cmd) (*ciInterruptionOwner, error) {
	return startInterruptionOwner(ctx, build, false)
}

func startInterruptionOwner(ctx context.Context, build func(context.Context) *exec.Cmd, isolatedGroup bool) (*ciInterruptionOwner, error) {
	command := build(context.Background())
	if isolatedGroup {
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		command.WaitDelay = time.Second
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	target, allowance := command.Process.Pid, 3*time.Second
	if isolatedGroup {
		target, allowance = -target, 5*time.Second
	}
	owner := &ciInterruptionOwner{command: command, terminationPID: target, joinAllowance: allowance, done: make(chan struct{})}
	go func() {
		select {
		case <-ctx.Done():
			owner.TerminateAndWait()
		case <-owner.done:
		}
	}()
	return owner, nil
}

func (owner *ciInterruptionOwner) PID() int { return owner.command.Process.Pid }

// Exited never consumes the direct child's wait status. Holding the phase lock
// prevents cancellation from starting a concurrent reap during this observation.
// per docs/adr/0151-reliable-ci-qualification.md:193
func (owner *ciInterruptionOwner) Exited() (bool, error) {
	owner.mutex.Lock()
	defer owner.mutex.Unlock()
	if owner.closed {
		return false, os.ErrProcessDone
	}
	return installationProcessExited(owner.PID())
}

func (owner *ciInterruptionOwner) Signal(signal syscall.Signal) error {
	owner.mutex.Lock()
	defer owner.mutex.Unlock()
	if owner.closed {
		return os.ErrProcessDone
	}
	return syscall.Kill(owner.PID(), signal)
}

var errCIInterruptionOwnerRunning = errors.New("interruption fixture child is still running")

// Refusing a live child does not relinquish custody. A terminal child can be
// joined without an unnecessary signal, after irrevocably closing that phase.
// per docs/adr/0151-reliable-ci-qualification.md:255
func (owner *ciInterruptionOwner) JoinExited() (ciInterruptionResult, error) {
	if err := owner.beginTerminalJoin(); err != nil {
		return ciInterruptionResult{}, err
	}
	<-owner.done
	return owner.result, nil
}

// One caller deadline covers nonreaping observation and the complete join.
// A live timeout leaves custody open; a terminal timeout cannot reopen signals.
// per docs/adr/0151-reliable-ci-qualification.md:274
func (owner *ciInterruptionOwner) AwaitExit(timeout time.Duration) (ciInterruptionResult, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	poll := time.NewTicker(time.Millisecond)
	defer poll.Stop()
	for {
		err := owner.beginTerminalJoin()
		if err == nil {
			select {
			case <-owner.done:
				return owner.result, nil
			case <-timer.C:
				return ciInterruptionResult{}, context.DeadlineExceeded
			}
		}
		if !errors.Is(err, errCIInterruptionOwnerRunning) {
			return ciInterruptionResult{}, err
		}
		select {
		case <-timer.C:
			return ciInterruptionResult{}, context.DeadlineExceeded
		case <-poll.C:
		}
	}
}

func (owner *ciInterruptionOwner) beginTerminalJoin() error {
	owner.mutex.Lock()
	defer owner.mutex.Unlock()
	if owner.closed {
		return nil
	}
	exited, err := installationProcessExited(owner.PID())
	if err != nil {
		return err
	}
	if !exited {
		return errCIInterruptionOwnerRunning
	}
	owner.closed = true
	owner.beginWaitLocked(nil)
	return nil
}

// Even a naturally exited child stays unreaped until the last possible cleanup
// signal completes. Every caller receives the same bounded terminal result.
// per docs/adr/0151-reliable-ci-qualification.md:196
func (owner *ciInterruptionOwner) TerminateAndWait() ciInterruptionResult {
	owner.mutex.Lock()
	if !owner.closed {
		owner.closed = true
		signalErr := syscall.Kill(owner.terminationPID, syscall.SIGKILL)
		if errors.Is(signalErr, syscall.ESRCH) {
			signalErr = nil
		}
		owner.beginWaitLocked(signalErr)
	}
	owner.mutex.Unlock()
	<-owner.done
	return owner.result
}

// Both terminal paths hold the phase mutex and close signaling before invoking
// this sole waiter. Publication of done makes the result immutable to readers.
// per docs/adr/0151-reliable-ci-qualification.md:255
func (owner *ciInterruptionOwner) beginWaitLocked(signalErr error) {
	go func() {
		joined := make(chan ciInterruptionResult, 1)
		go func() {
			waitErr := owner.command.Wait()
			joined <- ciInterruptionResult{Joined: true, SignalError: signalErr, WaitError: waitErr, State: owner.command.ProcessState}
		}()
		timer := time.NewTimer(owner.joinAllowance)
		defer timer.Stop()
		select {
		case owner.result = <-joined:
		case <-timer.C:
			owner.result = ciInterruptionResult{SignalError: signalErr, WaitError: context.DeadlineExceeded}
		}
		close(owner.done)
	}()
}
