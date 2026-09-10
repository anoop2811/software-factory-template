package native

import (
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

// OwnershipError preserves the PID when termination or reap is unconfirmed.
// docs/adr/0068-go-native-harness-execution.md:44.
type OwnershipError struct{ ProcessPID int }

func (e *OwnershipError) Error() string { return "native process ownership is unconfirmed" }

type processOps struct {
	signalGroup func(int) error
	wait        func(*exec.Cmd) error
	waitLimit   time.Duration
}

func defaultProcessOps() processOps {
	return processOps{
		signalGroup: func(pid int) error {
			err := syscall.Kill(-pid, syscall.SIGKILL)
			if errors.Is(err, syscall.ESRCH) {
				return nil
			}
			return err
		},
		wait: func(command *exec.Cmd) error { return command.Wait() }, waitLimit: 5 * time.Second,
	}
}

type runOptions struct {
	limit       int
	mergeStderr bool
	consume     func(context.Context, []byte) (bool, error)
}

// Execute supervises an already-admitted process; publication precedes stdin.
// docs/adr/0068-go-native-harness-execution.md:119.
func Execute(ctx context.Context, plan Plan, allowance time.Duration, onSpawn func(context.Context, int) error) (Execution, error) {
	return execute(ctx, plan, allowance, onSpawn, defaultProcessOps())
}
func execute(ctx context.Context, plan Plan, allowance time.Duration, onSpawn func(context.Context, int) error, ops processOps) (Execution, error) {
	if err := validatePlan(plan); err != nil {
		return Execution{Outcome: "launch_error"}, err
	}
	return supervise(ctx, plan, allowance, onSpawn, ops, runOptions{limit: outputLimit})
}

func validatePlan(plan Plan) error {
	if !validHarness(plan.Harness) || !validRole(plan.Role) || plan.Root == "" || len(plan.Argv) == 0 || plan.Argv[0] == "" || len(plan.Stdin) > outputLimit {
		return errors.New("invalid native execution plan")
	}
	for _, value := range append([]string{plan.Root, plan.Stdin}, plan.Argv...) {
		if !scalar(value) || len(value) > outputLimit {
			return errors.New("invalid native execution plan")
		}
	}
	for key, value := range plan.Environment {
		if key == "" || strings.Contains(key, "=") || !scalar(key) || !scalar(value) || len(value) > fileLimit {
			return errors.New("invalid native execution environment")
		}
	}
	return nil
}

type processPipes struct{ inputR, inputW, outputR, outputW, errorR, errorW *os.File }

func openPipes() (processPipes, error) {
	var p processPipes
	var err error
	p.inputR, p.inputW, err = os.Pipe()
	if err != nil {
		return p, err
	}
	p.outputR, p.outputW, err = os.Pipe()
	if err != nil {
		p.close()
		return p, err
	}
	p.errorR, p.errorW, err = os.Pipe()
	if err != nil {
		p.close()
		return p, err
	}
	return p, nil
}
func (p processPipes) close() {
	for _, file := range []*os.File{p.inputR, p.inputW, p.outputR, p.outputW, p.errorR, p.errorW} {
		if file != nil {
			_ = file.Close()
		}
	}
}
func (p processPipes) closeChild() { _ = p.inputR.Close(); _ = p.outputW.Close(); _ = p.errorW.Close() }

type capture struct {
	ctx     context.Context
	options runOptions
	mutex   sync.Mutex
	data    []byte
	stop    chan captureStop
	final   captureStop
	stopped bool
}
type captureStop struct {
	outcome string
	err     error
}

func (c *capture) Write(data []byte) (int, error) {
	c.mutex.Lock()
	remaining := c.options.limit + 1 - len(c.data)
	count := min(len(data), remaining)
	c.data = append(c.data, data[:count]...)
	overflow := len(c.data) > c.options.limit
	c.mutex.Unlock()
	if overflow {
		c.notify(captureStop{outcome: "output_limit"})
		return count, errors.New("native output limit")
	}
	if c.options.consume != nil {
		complete, err := c.options.consume(c.ctx, data)
		if err != nil || complete {
			c.notify(captureStop{err: err})
			return len(data), io.EOF
		}
	}
	return len(data), nil
}
func (c *capture) notify(stop captureStop) {
	c.mutex.Lock()
	if !c.stopped {
		c.final = stop
		c.stopped = true
	}
	c.mutex.Unlock()
	select {
	case c.stop <- stop:
	default:
	}
}
func (c *capture) status() captureStop { c.mutex.Lock(); defer c.mutex.Unlock(); return c.final }
func (c *capture) snapshot() []byte {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return append([]byte(nil), c.data...)
}

// Explicit file descriptors keep Cmd.Wait independent of pipe EOF. A descendant
// holding a pipe cannot conceal leader exit: docs/adr/0068-go-native-harness-execution.md:122.
func supervise(parent context.Context, plan Plan, allowance time.Duration, onSpawn func(context.Context, int) error, ops processOps, options runOptions) (result Execution, returned error) {
	start := time.Now()
	defer func() { result.ElapsedSeconds = time.Since(start).Seconds() }()
	result.Outcome = "launch_error"
	if allowance <= 0 || onSpawn == nil {
		return result, errors.New("invalid native execution admission")
	}
	if err := parent.Err(); err != nil {
		result.Outcome = contextOutcome(err)
		return result, errors.New("native execution context ended before launch")
	}
	ctx, cancel := context.WithTimeout(parent, allowance)
	defer cancel()
	pipes, err := openPipes()
	if err != nil {
		return result, errors.New("cannot create native process pipes")
	}
	defer pipes.close()
	binary, err := resolveExecutable(plan.Argv[0], plan.Root)
	if err != nil {
		return result, errors.New("cannot resolve native harness")
	}
	command := exec.Command(binary, plan.Argv[1:]...) // #nosec G204 -- internal admitted argv is passed literally, without a shell.
	command.Dir = plan.Root
	command.Env = mergedEnvironment(plan.Environment)
	command.Stdin, command.Stdout, command.Stderr = pipes.inputR, pipes.outputW, pipes.errorW
	if options.mergeStderr {
		command.Stderr = pipes.outputW
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if ctx.Err() != nil {
		result.Outcome = contextOutcome(ctx.Err())
		return result, errors.New("native execution context ended before launch")
	}
	if command.Start() != nil {
		return result, errors.New("cannot launch native harness")
	}
	result.ProcessPID = command.Process.Pid
	pipes.closeChild()
	waited := make(chan error, 1)
	go func() { waited <- ops.wait(command) }()
	captured := &capture{ctx: ctx, options: options, stop: make(chan captureStop, 1)}
	outputDone := make(chan struct{})
	go func() { defer close(outputDone); _, _ = io.Copy(captured, pipes.outputR) }()
	errorDone := make(chan struct{})
	go func() { defer close(errorDone); _, _ = io.Copy(io.Discard, pipes.errorR) }()
	inputDone := make(chan struct{})
	var waitErr error
	reaped := false
	callbackErr := onSpawn(ctx, result.ProcessPID)
	switch {
	case callbackErr != nil:
		returned = errors.New("native ownership publication failed")
		close(inputDone)
	case ctx.Err() != nil:
		result.Outcome = contextOutcome(ctx.Err())
		close(inputDone)
	default:
		go func() {
			defer close(inputDone)
			_, _ = io.Copy(pipes.inputW, strings.NewReader(plan.Stdin))
			_ = pipes.inputW.Close()
		}()
		result.Outcome = "completed"
		select {
		case waitErr = <-waited:
			reaped = true
		case <-ctx.Done():
			result.Outcome = contextOutcome(ctx.Err())
		case stop := <-captured.stop:
			if stop.outcome != "" {
				result.Outcome = stop.outcome
			}
			if stop.err != nil {
				returned = errors.New("invalid native capability response")
				result.Outcome = "failed"
			}
		}
	}
	// Termination is required even after ordinary leader exit, before pipe drainage.
	// docs/adr/0068-go-native-harness-execution.md:122.
	signalErr := ops.signalGroup(result.ProcessPID)
	_ = pipes.inputW.Close()
	cleanupCtx, finishCleanup := context.WithTimeout(context.Background(), ops.waitLimit)
	defer finishCleanup()
	if !reaped {
		select {
		case waitErr = <-waited:
			reaped = true
		case <-cleanupCtx.Done():
		}
	}
	if reaped {
		var exitError *exec.ExitError
		if waitErr == nil || errors.As(waitErr, &exitError) {
			if command.ProcessState != nil {
				code := command.ProcessState.ExitCode()
				if status, ok := command.ProcessState.Sys().(syscall.WaitStatus); ok && status.Signaled() {
					code = -int(status.Signal())
				}
				result.ExitCode = &code
				result.ExitConfirmed = true
			}
		}
	}
	// Incomplete workers cannot authorize success even after the leader is reaped.
	// docs/adr/0068-go-native-harness-execution.md:201.
	drained := true
	for _, done := range []chan struct{}{outputDone, errorDone, inputDone} {
		select {
		case <-done:
		case <-cleanupCtx.Done():
			select {
			case <-done:
			default:
				drained = false
			}
			pipes.close()
		}
	}
	result.Stdout = captured.snapshot()
	finalCapture := captured.status()
	if finalCapture.err != nil {
		returned = errors.New("invalid native capability response")
		if result.Outcome == "completed" {
			result.Outcome = "failed"
		}
	}
	if len(result.Stdout) > options.limit {
		result.Outcome = "output_limit"
	}
	if signalErr != nil || !result.ExitConfirmed || !drained {
		result.OwnershipUnconfirmed = true
		if result.Outcome == "completed" {
			result.Outcome = "failed"
		}
		return result, &OwnershipError{ProcessPID: result.ProcessPID}
	}
	if result.Outcome == "completed" && result.ExitCode != nil && *result.ExitCode != 0 {
		result.Outcome = "failed"
	}
	return result, returned
}
func contextOutcome(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "interrupted"
}
func mergedEnvironment(overrides map[string]string) []string {
	environment := os.Environ()
	result := make([]string, 0, len(environment)+len(overrides))
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		if _, overridden := overrides[key]; !overridden {
			result = append(result, entry)
		}
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}

// Interpret relative PATH entries after the child's cwd, as native Popen does.
// Preflight supplies its separately selected literal path to the same boundary.
// docs/adr/0068-go-native-harness-execution.md:92.
func resolveExecutable(name, root string) (string, error) {
	if !strings.ContainsRune(name, os.PathSeparator) {
		var err error
		name, err = findExecutable(name, root)
		if err != nil {
			return "", err
		}
	}
	return executionPath(root, name)
}
func executionPath(root, path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	if !filepath.IsAbs(root) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		root = rootPath(cwd, root)
	}
	return rootPath(root, path), nil
}
func findExecutable(name, root string) (string, error) {
	searchPath, exists := os.LookupEnv("PATH")
	if !exists {
		searchPath = "/bin:/usr/bin"
	}
	for _, directory := range strings.Split(searchPath, string(os.PathListSeparator)) {
		if directory == "" {
			directory = "."
		}
		candidate := rootPath(directory, name)
		resolved, err := executionPath(root, candidate)
		if err != nil {
			return "", err
		}
		if _, err := exec.LookPath(resolved); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("native executable not found")
}
