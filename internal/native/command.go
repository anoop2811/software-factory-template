package native

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

// CommandResult retains the two output channels of an explicitly selected tool.
type CommandResult struct {
	Execution
	Stderr []byte
}

// ExecuteCommand supervises literal initializer tools without harness-role overlays.
// docs/adr/0085-go-native-init.md:55.
func ExecuteCommand(ctx context.Context, root string, argv []string, environment map[string]string, allowance time.Duration) (CommandResult, error) {
	return executeCommand(ctx, root, argv, environment, allowance, false)
}

// ExecuteCommandCombined preserves the proof script's shared output stream.
// docs/adr/0086-go-native-doctor.md:72.
func ExecuteCommandCombined(ctx context.Context, root string, argv []string, environment map[string]string, allowance time.Duration) (CommandResult, error) {
	return executeCommand(ctx, root, argv, environment, allowance, true)
}

func executeCommand(ctx context.Context, root string, argv []string, environment map[string]string, allowance time.Duration, combined bool) (CommandResult, error) {
	if root == "" || len(argv) == 0 || argv[0] == "" {
		return CommandResult{}, errors.New("invalid tool command")
	}
	for _, value := range append([]string{root}, argv...) {
		if strings.ContainsRune(value, 0) {
			return CommandResult{}, errors.New("invalid tool command")
		}
	}
	variables := make([]string, 0, len(environment))
	for key, value := range environment {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, 0) {
			return CommandResult{}, errors.New("invalid tool environment")
		}
		variables = append(variables, key+"="+value)
	}
	binary := argv[0]
	if !strings.ContainsRune(binary, '/') {
		path, exists := environment["PATH"]
		if !exists {
			path = "/bin:/usr/bin"
		}
		var err error
		binary, err = findExecutableOnPath(binary, root, path)
		if err != nil {
			return CommandResult{}, errors.New("cannot resolve tool")
		}
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	stderr := &commandCapture{cancel: cancel}
	arguments := append([]string{binary}, argv[1:]...)
	execution, err := supervise(child, Plan{Root: root, Argv: arguments}, allowance,
		func(context.Context, int) error { return nil }, defaultProcessOps(),
		runOptions{limit: outputLimit, environment: variables, errorOutput: stderr, mergeStderr: combined})
	data, overflow := stderr.snapshot()
	if overflow {
		err = errors.Join(err, errors.New("tool output limit exceeded"))
		execution.Outcome = "output_limit"
	}
	return CommandResult{Execution: execution, Stderr: data}, err
}

type commandCapture struct {
	mutex    sync.Mutex
	data     []byte
	overflow bool
	cancel   context.CancelFunc
}

func (c *commandCapture) Write(data []byte) (int, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	count := min(len(data), outputLimit-len(c.data))
	c.data = append(c.data, data[:count]...)
	if count != len(data) {
		c.overflow = true
		c.cancel()
		return count, errors.New("tool output limit exceeded")
	}
	return count, nil
}
func (c *commandCapture) snapshot() ([]byte, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return append([]byte(nil), c.data...), c.overflow
}
