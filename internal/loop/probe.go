package loop

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const probeLimit = 32 << 20

type probeOutput struct {
	mu       sync.Mutex
	data     bytes.Buffer
	total    int
	exceeded bool
	cancel   context.CancelFunc
}
type probeWriter struct {
	output *probeOutput
	retain bool
}

func (w probeWriter) Write(data []byte) (int, error) {
	p := w.output
	p.mu.Lock()
	defer p.mu.Unlock()
	count := min(len(data), probeLimit-p.total)
	p.total += count
	if w.retain {
		_, _ = p.data.Write(data[:count])
	}
	if count != len(data) {
		p.exceeded = true
		p.cancel()
		return count, io.ErrShortWrite
	}
	return count, nil
}

// Literal probes retain caller cwd, PATH and Git/locale environment. Cancellation
// kills the owned group and bounds pipe cleanup. docs/adr/0073-go-loop-fingerprint-foundation.md:96.
func probe(parent context.Context, environment map[string]string, allowance time.Duration, args []string, input []byte) ([]byte, int, error) {
	if err := parent.Err(); err != nil {
		return nil, 0, err
	}
	ctx, cancel := context.WithTimeout(parent, allowance)
	defer cancel()
	binary, err := probeBinary(environment, args[0])
	if err != nil {
		return nil, 0, loopError()
	}
	command := exec.CommandContext(ctx, binary, args[1:]...) // #nosec G204 -- fixed Git/grep argv only, never a shell or check command.
	command.Args[0] = args[0]
	command.Env = make([]string, 0, len(environment))
	for key, value := range environment {
		command.Env = append(command.Env, key+"="+value)
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	kill := func() error {
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	command.Cancel = kill
	command.WaitDelay = time.Second
	output := &probeOutput{cancel: cancel}
	command.Stdout = probeWriter{output: output, retain: true}
	command.Stderr = probeWriter{output: output}
	command.Stdin = bytes.NewReader(input)
	err = command.Run()
	output.mu.Lock()
	exceeded := output.exceeded
	data := append([]byte(nil), output.data.Bytes()...)
	output.mu.Unlock()
	if ctx.Err() != nil || exceeded || errors.Is(err, exec.ErrWaitDelay) {
		if command.Process != nil {
			_ = kill()
		}
		return nil, 0, loopError()
	}
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return nil, 0, loopError()
		}
		return data, exit.ExitCode(), nil
	}
	return data, 0, nil
}
func probeBinary(environment map[string]string, name string) (string, error) {
	path, exists := environment["PATH"]
	if !exists {
		path = "/bin:/usr/bin"
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for _, directory := range strings.Split(path, string(os.PathListSeparator)) {
		if directory == "" {
			directory = "."
		}
		candidate := directory + string(os.PathSeparator) + name
		if !filepath.IsAbs(candidate) {
			candidate = cwd + string(os.PathSeparator) + candidate
		}
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", loopError()
}
