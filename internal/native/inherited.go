package native

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type InheritedStreams struct{ Stdin, Stdout, Stderr *os.File }

// ExecuteInherited retains the existing ownership/cleanup engine and caller streams.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:157.
func ExecuteInherited(ctx context.Context, root string, argv []string, environment map[string]string, streams InheritedStreams, allowance time.Duration) (Execution, error) {
	if streams.Stdin == nil || streams.Stdout == nil || streams.Stderr == nil {
		return Execution{}, errors.New("inherited streams unavailable")
	}
	plan, variables, err := prepareCommand(root, argv, environment)
	if err != nil {
		return Execution{}, err
	}
	return supervise(ctx, plan, allowance, func(context.Context, int) error { return nil }, defaultProcessOps(), runOptions{limit: outputLimit, environment: variables, inherited: &streams, parentContextOnly: allowance == 0})
}

type foreground struct {
	file  *os.File
	group int
}

// Capture only terminal ownership held by this supervisor. Initial foreground
// admission is completed by the child before exec. docs/DECISION_LOG.md:5092.
func captureForeground(file *os.File) (*foreground, error) {
	group, err := unix.IoctlGetInt(int(file.Fd()), unix.TIOCGPGRP)
	// Darwin /dev/null reports ENODEV; neither case owns a terminal foreground.
	// docs/adr/0099-native-factory-orchestration.md:200.
	if errors.Is(err, unix.ENOTTY) || errors.Is(err, unix.ENODEV) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if group != syscall.Getpgrp() {
		return nil, nil
	}
	return &foreground{file, group}, nil
}

func takeForeground(file *os.File, pid int) (*foreground, error) {
	terminal, err := captureForeground(file)
	if err != nil || terminal == nil {
		return terminal, err
	}
	if err := terminal.transfer(pid); err != nil {
		return nil, err
	}
	return terminal, nil
}

func (terminal *foreground) transfer(group int) error {
	signal.Ignore(syscall.SIGTTOU)
	err := unix.IoctlSetPointerInt(int(terminal.file.Fd()), unix.TIOCSPGRP, group)
	signal.Reset(syscall.SIGTTOU)
	return err
}

func (terminal *foreground) restore() error { return terminal.transfer(terminal.group) }
func forwardSignals(pid int, terminalOwned bool) func() {
	signals := make(chan os.Signal, 8)
	done := make(chan struct{})
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTSTP)
	if !terminalOwned {
		signal.Notify(signals, syscall.SIGCONT)
	}
	go func() {
		for {
			select {
			case <-done:
				return
			case received := <-signals:
				if value, ok := received.(syscall.Signal); ok {
					_ = syscall.Kill(-pid, value)
				}
			}
		}
	}()
	return func() { signal.Stop(signals); close(done) }
}

// Observe stopped owned work without consuming Cmd.Wait's exit/reap evidence.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:282.
func watchJobControl(ctx context.Context, pid int, terminal *foreground, leaderDone <-chan struct{}, failed func(error)) func() {
	stop := make(chan struct{})
	finished := make(chan struct{})
	continued := make(chan os.Signal, 1)
	signal.Notify(continued, syscall.SIGCONT)
	go func() {
		defer close(finished)
		defer signal.Stop(continued)
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-leaderDone:
				return
			case <-stop:
				return
			case <-ticker.C:
				stopped, err := processStopped(ctx, pid)
				select {
				case <-leaderDone:
					return
				default:
				}
				if err != nil {
					failed(err)
					return
				}
				if !stopped {
					continue
				}
				if err := terminal.restore(); err != nil {
					failed(err)
					return
				}
				// Discard a notification preceding this stop cycle.
				select {
				case <-continued:
				default:
				}
				// The caller's shell sees this supervisor stop and regains its terminal.
				if err := syscall.Kill(os.Getpid(), syscall.SIGSTOP); err != nil {
					failed(err)
					return
				}
				// SIGSTOP delivery can follow the syscall return on a multithreaded
				// process. Only a native continuation notification permits handoff.
				// docs/adr/0098-whole-installation-upgrade-and-rollback.md:615.
				select {
				case <-continued:
				case <-ctx.Done():
					return
				case <-leaderDone:
					return
				case <-stop:
					return
				}
				if ctx.Err() != nil {
					return
				}
				// fg resumes the supervisor first. Transfer before resuming real stdin work.
				_, err = takeForeground(terminal.file, pid)
				if err != nil {
					failed(err)
					return
				}
				if err := syscall.Kill(-pid, syscall.SIGCONT); err != nil && !errors.Is(err, syscall.ESRCH) {
					failed(err)
					return
				}
			}
		}
	}()
	return func() { close(stop); <-finished }
}
