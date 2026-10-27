package native

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const foregroundAdmissionArgument = "--factory-native-foreground-admission"

type initialForegroundRecord struct {
	PID        int `json:"pid"`
	Group      int `json:"group"`
	Foreground int `json:"foreground"`
	Owner      int `json:"owner"`
	InputFD    int `json:"input_fd"`
	Ctty       int `json:"ctty"`
}

// per docs/DECISION_LOG.md:5083
func init() {
	if len(os.Args) != 3 || os.Args[1] != foregroundAdmissionArgument {
		return
	}
	if err := foregroundAdmissionFixture(os.Args[2]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

// The real child's pipe record precedes its first stdin read. The parent start
// callback cannot return until that actual terminal snapshot has been received.
// per docs/DECISION_LOG.md:5083
func foregroundAdmissionFixture(mode string) error {
	if mode == "failed-start" {
		return foregroundFailedStartFixture()
	}
	if mode == "child" || mode == "snapshot-only" {
		foreground, err := unix.IoctlGetInt(int(os.Stdin.Fd()), unix.TIOCGPGRP)
		if err != nil {
			return err
		}
		acknowledgement := os.NewFile(3, "initial-foreground")
		defer acknowledgement.Close()
		record := initialForegroundRecord{PID: os.Getpid(), Group: syscall.Getpgrp(), Foreground: foreground, Owner: os.Getppid()}
		if err := json.NewEncoder(acknowledgement).Encode(record); err != nil {
			return err
		}
		if mode == "snapshot-only" {
			return nil
		}
		value, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "INITIAL_READ=%s\n", strings.TrimSuffix(value, "\n"))
		return err
	}
	if mode != "supervisor" && mode != "background" && mode != "cancel" {
		return fmt.Errorf("unknown foreground admission fixture mode %q", mode)
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer reader.Close()
	defer writer.Close()
	input := os.Stdin
	if mode == "supervisor" || mode == "cancel" {
		descriptor, err := unix.FcntlInt(os.Stdin.Fd(), unix.F_DUPFD_CLOEXEC, 3)
		if err != nil {
			return err
		}
		input = os.NewFile(uintptr(descriptor), "owned-duplicate-stdin")
		defer input.Close()
	}
	ops := defaultProcessOps()
	originalStart := ops.start
	var observationErr error
	ops.start = func(command *exec.Cmd) error {
		command.ExtraFiles = []*os.File{writer}
		if err := originalStart(command); err != nil {
			return err
		}
		_ = writer.Close()
		observationErr = reader.SetReadDeadline(time.Now().Add(3 * time.Second))
		if observationErr == nil {
			var record initialForegroundRecord
			observationErr = json.NewDecoder(reader).Decode(&record)
			if observationErr == nil {
				record.InputFD, record.Ctty = int(input.Fd()), command.SysProcAttr.Ctty
				data, marshalErr := json.Marshal(record)
				observationErr = marshalErr
				if observationErr == nil {
					_, observationErr = fmt.Fprintf(os.Stdout, "INITIAL_FOREGROUND=%s\n", data)
				}
			}
		}
		// Successful Start still returns nil if the evaluator's observation fails:
		// supervision remains the owner of the running process and its sole waiter.
		return nil
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	childMode := "child"
	if mode == "background" {
		childMode = "snapshot-only"
	}
	plan := Plan{Root: root, Argv: []string{executable, foregroundAdmissionArgument, childMode}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	onSpawn := func(context.Context, int) error { return nil }
	if mode == "cancel" {
		onSpawn = func(context.Context, int) error { cancel(); return nil }
	}
	result, runErr := supervise(ctx, plan, 5*time.Second, onSpawn, ops, runOptions{limit: outputLimit, inherited: &InheritedStreams{Stdin: input, Stdout: os.Stdout, Stderr: os.Stderr}})
	if observationErr != nil {
		return observationErr
	}
	if mode == "cancel" {
		after, err := unix.IoctlGetInt(int(input.Fd()), unix.TIOCGPGRP)
		if err != nil {
			return err
		}
		data, err := json.Marshal(map[string]any{"after": after, "parent_group": syscall.Getpgrp(), "outcome": result.Outcome, "no_ownership_error": runErr == nil && !result.OwnershipUnconfirmed, "exit_confirmed": result.ExitConfirmed, "exit_code": result.ExitCode, "actual_child_absent": errors.Is(syscall.Kill(result.ProcessPID, 0), syscall.ESRCH)})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "CANCELED_ADMISSION=%s\n", data)
		return err
	}
	if runErr != nil {
		return runErr
	}
	if !result.ExitConfirmed || result.ExitCode == nil || *result.ExitCode != 0 {
		return fmt.Errorf("initial foreground child did not complete normally")
	}
	if mode == "background" {
		_, err = fmt.Fprintln(os.Stdout, "BACKGROUND_COMPLETE")
		return err
	}
	return nil
}

// per docs/DECISION_LOG.md:5092
func foregroundFailedStartFixture() error {
	before, err := unix.IoctlGetInt(int(os.Stdin.Fd()), unix.TIOCGPGRP)
	if err != nil {
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	invalid := filepath.Join(root, "invalid-executable-image")
	if err := os.WriteFile(invalid, []byte("invalid executable image\n"), 0700); err != nil { // #nosec G306,G703 -- executable invalid image is required for real ENOEXEC; fixed leaf in the evaluator-owned temporary PTY root.
		return err
	}
	ops := defaultProcessOps()
	originalStart := ops.start
	starts, requested, failedExec := 0, false, false
	ops.start = func(command *exec.Cmd) error {
		starts++
		requested = command.SysProcAttr.Foreground
		err := originalStart(command)
		failedExec = errors.Is(err, syscall.ENOEXEC)
		return err
	}
	result, runErr := supervise(context.Background(), Plan{Root: root, Argv: []string{invalid}}, 5*time.Second, func(context.Context, int) error { return nil }, ops, runOptions{limit: outputLimit, inherited: &InheritedStreams{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}})
	after, err := unix.IoctlGetInt(int(os.Stdin.Fd()), unix.TIOCGPGRP)
	if err != nil {
		return err
	}
	data, err := json.Marshal(map[string]any{"before": before, "after": after, "parent_group": syscall.Getpgrp(), "start_calls": starts, "foreground_requested": requested, "actual_enoexec": failedExec, "launch_error": runErr != nil && result.Outcome == "launch_error"})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "FAILED_START=%s\n", data)
	return err
}
