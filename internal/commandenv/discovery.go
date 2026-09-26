package commandenv

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

	"github.com/anoop2811/software-factory-template/internal/config"
)

const discoveryLimit = 1 << 20

type discoveryOutput struct {
	mu       sync.Mutex
	stdout   bytes.Buffer
	size     int
	exceeded bool
	cancel   context.CancelFunc
}
type discoveryWriter struct {
	output *discoveryOutput
	retain bool
}

func (w discoveryWriter) Write(data []byte) (int, error) {
	o := w.output
	o.mu.Lock()
	defer o.mu.Unlock()
	n := min(len(data), discoveryLimit-o.size)
	o.size += n
	if w.retain {
		_, _ = o.stdout.Write(data[:n])
	}
	if n != len(data) {
		o.exceeded = true
		o.cancel()
		return n, io.ErrShortWrite
	}
	return n, nil
}

// Discovery preserves the two different shell fallback expressions from one
// stable Git observation. docs/adr/0077-go-command-environment.md:42.
func discover(ctx context.Context, environment map[string]string) (string, string, error) {
	cwd, err := workingDirectory(environment)
	if err != nil {
		return "", "", preparationError(ctx)
	}
	output, success, err := gitRoot(ctx, environment, cwd)
	if err != nil {
		return "", "", err
	}
	root, configRoot := string(output), string(output)
	if !success {
		root += cwd + "\n"
		configRoot += ".\n"
	}
	path := config.ResolvePath(environment["FACTORY_CONFIG"], substitution(configRoot))
	return substitution(root), substitution(path), nil
}

// A supplied logical PWD is meaningful only when it identifies the actual cwd.
func workingDirectory(environment map[string]string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", err
	}
	if logical := environment["PWD"]; filepath.IsAbs(logical) {
		current, err := os.Stat(cwd)
		selected, selectedErr := os.Stat(logical)
		if err == nil && selectedErr == nil && os.SameFile(current, selected) {
			return logical, nil
		}
	}
	return cwd, nil
}

// Bound both output streams and terminate residual owned process groups.
// docs/adr/0077-go-command-environment.md:89.
func gitRoot(parent context.Context, environment map[string]string, cwd string) ([]byte, bool, error) {
	if err := parent.Err(); err != nil {
		return nil, false, err
	}
	binary, err := gitExecutable(environment, cwd)
	if err != nil {
		return nil, false, nil
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "rev-parse", "--show-toplevel") // #nosec G204 -- fixed Git arguments with caller-selected PATH, matching the wrapper dependency.
	command.Args[0] = "git"
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
	output := &discoveryOutput{cancel: cancel}
	command.Stdout = discoveryWriter{output: output, retain: true}
	command.Stderr = discoveryWriter{output: output}
	err = command.Run()
	// WaitDelay may be hidden by a nonzero leader exit. Refuse any residual
	// owned group rather than treating its open pipes as an ordinary fallback.
	// A successful signal does not claim that descendant processes were reaped.
	residual := false
	if command.Process != nil {
		residual = !errors.Is(kill(), os.ErrProcessDone)
	}
	output.mu.Lock()
	exceeded := output.exceeded
	data := append([]byte(nil), output.stdout.Bytes()...)
	output.mu.Unlock()
	if ctx.Err() != nil || exceeded || residual || errors.Is(err, exec.ErrWaitDelay) {
		return nil, false, preparationError(parent)
	}
	if err != nil {
		return data, false, nil
	}
	return data, true, nil
}

func gitExecutable(environment map[string]string, cwd string) (string, error) {
	path := environment["PATH"]
	for _, directory := range strings.Split(path, string(os.PathListSeparator)) {
		if directory == "" {
			directory = "."
		}
		candidate := directory + string(os.PathSeparator) + "git"
		if !filepath.IsAbs(candidate) {
			candidate = cwd + string(os.PathSeparator) + candidate
		}
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", environmentError()
}
