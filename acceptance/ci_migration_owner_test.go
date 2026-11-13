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
	command *exec.Cmd
	mutex   sync.Mutex
	closed  bool
	done    chan struct{}
	result  ciInterruptionResult
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
	command := build(context.Background())
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.WaitDelay = time.Second
	if err := command.Start(); err != nil {
		return nil, err
	}
	owner := &ciInterruptionOwner{command: command, done: make(chan struct{})}
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

// Even a naturally exited child stays unreaped until the last possible group
// signal completes. Every caller receives the same bounded terminal result.
// per docs/adr/0151-reliable-ci-qualification.md:196
func (owner *ciInterruptionOwner) TerminateAndWait() ciInterruptionResult {
	owner.mutex.Lock()
	if !owner.closed {
		owner.closed = true
		signalErr := syscall.Kill(-owner.PID(), syscall.SIGKILL)
		if errors.Is(signalErr, syscall.ESRCH) {
			signalErr = nil
		}
		go func() {
			joined := make(chan ciInterruptionResult, 1)
			go func() {
				waitErr := owner.command.Wait()
				joined <- ciInterruptionResult{Joined: true, SignalError: signalErr, WaitError: waitErr, State: owner.command.ProcessState}
			}()
			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			select {
			case owner.result = <-joined:
			case <-timer.C:
				owner.result = ciInterruptionResult{SignalError: signalErr, WaitError: context.DeadlineExceeded}
			}
			close(owner.done)
		}()
	}
	owner.mutex.Unlock()
	<-owner.done
	return owner.result
}
