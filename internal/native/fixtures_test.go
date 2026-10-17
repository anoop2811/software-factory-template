package native_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	"golang.org/x/sys/unix"
)

const nativeFixtureArgument = "--factory-native-test-fixture"
const nativeTerminalPrompt = "INHERITED_FIXTURE_PROMPT> "

// Re-execute the already compiled test binary rather than shipping helper code or
// requiring another interpreter. Internal and external specs use the same binary.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:634
func init() {
	if len(os.Args) < 3 || os.Args[1] != nativeFixtureArgument {
		return
	}
	if err := nativeFixture(os.Args[2], os.Args[3:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func nativeFixture(mode string, arguments []string) error {
	switch mode {
	case "inherited":
		environment := map[string]string{}
		for _, entry := range os.Environ() {
			key, value, _ := strings.Cut(entry, "=")
			environment[key] = value
		}
		root, err := os.Getwd()
		if err != nil {
			return err
		}
		result, err := native.ExecuteInherited(context.Background(), root, arguments, environment, native.InheritedStreams{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}, 0)
		if err != nil {
			return err
		}
		if result.ExitCode == nil {
			return errors.New("inherited fixture has no exit status")
		}
		status := *result.ExitCode
		if status < 0 {
			status = 128 - status
		}
		os.Exit(status)
	case "restop":
		continued := make(chan os.Signal, 1)
		signal.Notify(continued, syscall.SIGCONT)
		defer signal.Stop(continued)
		_, err := fmt.Fprintf(os.Stdout, "CHILD_PID=%d OWNER_PID=%d\n", os.Getpid(), os.Getppid())
		if err != nil {
			return err
		}
		// Reading is deliberately deferred until after two real continuations. The
		// marker is written before SIGSTOP, while post-stop work waits for SIGCONT:
		// Go may return from a self-SIGSTOP syscall before all threads have stopped.
		<-continued
		if _, err := os.Stdout.WriteString("FRESH_SELF_STOP\n"); err != nil {
			return err
		}
		if err := syscall.Kill(os.Getpid(), syscall.SIGSTOP); err != nil {
			return err
		}
		<-continued
		value, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "READ=%s\n", strings.TrimSuffix(value, "\n"))
		return err
	case "detached-leader":
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		child := exec.Command(executable, nativeFixtureArgument, "detached-child") // #nosec G204 -- the current compiled test binary, fixed test-only mode.
		child.Env = os.Environ()
		if err := child.Start(); err != nil {
			return err
		}
		ready := os.Getenv("NATIVE_GROUP_READY")
		deadline := time.Now().Add(5 * time.Second)
		for {
			data, readErr := os.ReadFile(ready) // #nosec G703 -- evaluator-created temporary readiness path passed only to this test-binary helper.
			if readErr == nil {
				var record struct {
					PID   int `json:"pid"`
					Group int `json:"group"`
				}
				if err := json.Unmarshal(data, &record); err != nil {
					return err
				}
				if record.PID != child.Process.Pid || record.Group != syscall.Getpgrp() {
					return errors.New("detached child did not establish same-group ownership")
				}
				_, err := fmt.Fprintln(os.Stdout, "fixture leader completed")
				return err
			}
			if !os.IsNotExist(readErr) {
				return readErr
			}
			if time.Now().After(deadline) {
				_ = child.Process.Kill()
				_ = child.Wait()
				return errors.New("child readiness was not established")
			}
			time.Sleep(10 * time.Millisecond)
		}
	case "detached-child":
		ready := os.Getenv("NATIVE_GROUP_READY")
		data, err := json.Marshal(map[string]int{"pid": os.Getpid(), "group": syscall.Getpgrp()})
		if err != nil {
			return err
		}
		if err := os.WriteFile(ready+".tmp", data, 0600); err != nil { // #nosec G703 -- evaluator-created temporary readiness path passed only to this test-binary helper.
			return err
		}
		if err := os.Rename(ready+".tmp", ready); err != nil { // #nosec G703 -- both names belong to the evaluator-created temporary readiness handshake.
			return err
		}
		time.Sleep(30 * time.Second)
		return nil
	}
	return fmt.Errorf("unknown native fixture mode %q", mode)
}

type terminalObservation struct {
	ChildForeground         bool   `json:"child_foreground"`
	StopReturnsPrompt       bool   `json:"stop_returns_prompt"`
	StopReturnsForeground   bool   `json:"stop_returns_foreground"`
	ResumeConsumesInput     bool   `json:"resume_consumes_input"`
	ExitReturnsPrompt       bool   `json:"exit_returns_prompt"`
	FreshStopMarker         bool   `json:"fresh_stop_marker"`
	SecondStopReturnsPrompt bool   `json:"second_stop_returns_prompt"`
	Transcript              string `json:"transcript"`
}

type nativeTerminal struct {
	master     *os.File
	shell      *exec.Cmd
	transcript []byte
	groups     map[int]bool
}

// Own a fresh real session and controlling terminal. No production terminal or
// stop-observation helpers participate in the evaluator's foreground evidence.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:634
func newNativeTerminal(ctx context.Context, root string) (*nativeTerminal, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	slaveName, err := nativePTYSlave(master)
	if err != nil {
		_ = master.Close()
		return nil, err
	}
	slave, err := os.OpenFile(slaveName, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		return nil, err
	}
	defer slave.Close()
	if err := unix.IoctlSetWinsize(int(master.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: 24, Col: 500}); err != nil {
		_ = master.Close()
		return nil, err
	}
	shell := exec.CommandContext(ctx, "/bin/bash", "--noprofile", "--norc", "-i")
	shell.Dir, shell.Stdin, shell.Stdout, shell.Stderr = root, slave, slave, slave
	shell.Env = append(os.Environ(), "PS1="+nativeTerminalPrompt, "BASH_SILENCE_DEPRECATION_WARNING=1", "GORACE=atexit_sleep_ms=0")
	shell.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := shell.Start(); err != nil {
		_ = master.Close()
		return nil, err
	}
	return &nativeTerminal{master: master, shell: shell, groups: map[int]bool{shell.Process.Pid: true}}, nil
}

func (terminal *nativeTerminal) foreground() int {
	group, err := unix.IoctlGetInt(int(terminal.master.Fd()), unix.TIOCGPGRP)
	if err != nil {
		return -1
	}
	return group
}

func (terminal *nativeTerminal) close() error {
	foreground := terminal.foreground()
	if foreground > 0 {
		terminal.groups[foreground] = true
	}
	for group := range terminal.groups {
		if group <= 1 || group == syscall.Getpgrp() {
			continue
		}
		if session, err := unix.Getsid(group); err == nil && session == terminal.shell.Process.Pid {
			_ = syscall.Kill(-group, syscall.SIGKILL)
		}
	}
	_ = terminal.shell.Wait()
	return terminal.master.Close()
}

// As in the previous fixture, each event retains its three-second bound. Poll
// waits for readable terminal bytes; elapsed time never establishes success.
func (terminal *nativeTerminal) until(ctx context.Context, predicate func(string) bool) (string, bool, error) {
	start := len(terminal.transcript)
	deadline := time.Now().Add(3 * time.Second)
	for {
		output := string(terminal.transcript[start:])
		if predicate(output) {
			return output, true, nil
		}
		if err := ctx.Err(); err != nil {
			return output, false, err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return output, false, nil
		}
		wait := min(50, int((remaining+time.Millisecond-1)/time.Millisecond))
		fd := terminal.master.Fd()
		if fd > math.MaxInt32 {
			return output, false, errors.New("PTY descriptor exceeds poll descriptor range")
		}
		descriptors := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		_, err := unix.Poll(descriptors, wait)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			return output, false, err
		}
		if descriptors[0].Revents == 0 {
			continue
		}
		var buffer [65536]byte
		count, err := terminal.master.Read(buffer[:])
		if errors.Is(err, syscall.EIO) {
			return output, false, nil
		}
		if err != nil {
			return output, false, err
		}
		if count == 0 {
			return output, false, nil
		}
		terminal.transcript = append(terminal.transcript, buffer[:count]...)
	}
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

var terminalChildPattern = regexp.MustCompile(`CHILD_PID=(\d+) OWNER_PID=(\d+)`)

func observeNativeTerminal(ctx context.Context, root, executable string, inherited, restop bool) (observed terminalObservation, err error) {
	terminal, err := newNativeTerminal(ctx, root)
	if err != nil {
		return observed, err
	}
	defer func() { observed.Transcript = string(terminal.transcript); err = errors.Join(err, terminal.close()) }()
	until := func(predicate func(string) bool) (string, bool, error) { return terminal.until(ctx, predicate) }
	if _, ready, err := until(func(output string) bool { return strings.Contains(output, nativeTerminalPrompt) }); err != nil || !ready {
		return observed, errors.Join(err, errors.New("interactive shell did not establish its prompt"))
	}
	program := `echo CHILD_PID=$$ OWNER_PID=$PPID; IFS= read -r value; printf 'READ=%s\n' "$value"`
	command := "/bin/bash -c " + shellQuote(program)
	if restop {
		command = shellQuote(executable) + " " + nativeFixtureArgument + " restop"
	}
	if inherited {
		command = shellQuote(executable) + " " + nativeFixtureArgument + " inherited " + command
	}
	if _, err := terminal.master.WriteString(command + "\n"); err != nil {
		return observed, err
	}
	output, ready, err := until(func(output string) bool { return terminalChildPattern.MatchString(output) })
	if err != nil {
		return observed, err
	}
	if !ready {
		return observed, fmt.Errorf("real child readiness not observed: %s", output)
	}
	matches := terminalChildPattern.FindStringSubmatch(output)
	child, _ := strconv.Atoi(matches[1])
	owner, _ := strconv.Atoi(matches[2])
	terminal.groups[child], terminal.groups[owner] = true, true
	observed.ChildForeground = terminal.foreground() == child
	if _, err := terminal.master.Write([]byte{0x1a}); err != nil {
		return observed, err
	}
	_, observed.StopReturnsPrompt, err = until(func(output string) bool {
		return strings.Contains(output, nativeTerminalPrompt) && terminal.foreground() == terminal.shell.Process.Pid
	})
	if err != nil {
		return observed, err
	}
	observed.StopReturnsForeground = terminal.foreground() == terminal.shell.Process.Pid
	if !observed.StopReturnsPrompt {
		return observed, nil
	}
	if _, err := terminal.master.WriteString("fg\n"); err != nil {
		return observed, err
	}
	if restop {
		output, observed.SecondStopReturnsPrompt, err = until(func(output string) bool {
			return strings.Contains(output, "\r\nFRESH_SELF_STOP\r\n") && strings.Contains(output, nativeTerminalPrompt) && terminal.foreground() == terminal.shell.Process.Pid
		})
		if err != nil {
			return observed, err
		}
		observed.FreshStopMarker = strings.Contains(output, "\r\nFRESH_SELF_STOP\r\n")
		if !observed.SecondStopReturnsPrompt {
			return observed, nil
		}
		if _, err := terminal.master.WriteString("fg\n"); err != nil {
			return observed, err
		}
	}
	_, resumed, err := until(func(string) bool { return terminal.foreground() == child })
	if err != nil || !resumed {
		return observed, err
	}
	if _, err := terminal.master.WriteString("literal resumed input\n"); err != nil {
		return observed, err
	}
	output, observed.ExitReturnsPrompt, err = until(func(output string) bool {
		return strings.Contains(output, "READ=literal resumed input\r\n") && strings.Contains(output, nativeTerminalPrompt) && terminal.foreground() == terminal.shell.Process.Pid
	})
	observed.ResumeConsumesInput = strings.Contains(output, "READ=literal resumed input\r\n")
	return observed, err
}
