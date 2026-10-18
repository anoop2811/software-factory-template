package native

import (
	"context"
	"errors"
)

// CommandOptions selects bounded literal I/O without changing process ownership.
// docs/adr/0099-native-factory-orchestration.md:191.
type CommandOptions struct {
	Input                    string
	StdoutLimit, StderrLimit int
	MergeStderr              bool
}

// ExecuteCommandBounded shares process ownership with existing tool execution,
// using only the caller's deadline and independently bounded output channels.
// docs/adr/0097-installation-source-image-bundles.md:196.
func ExecuteCommandBounded(ctx context.Context, root string, argv []string, environment map[string]string, stdoutLimit, stderrLimit int) (CommandResult, error) {
	return ExecuteCommandWithOptions(ctx, root, argv, environment, CommandOptions{StdoutLimit: stdoutLimit, StderrLimit: stderrLimit})
}

// ExecuteCommandWithOptions retains caller-only deadlines and the shared engine.
// docs/adr/0099-native-factory-orchestration.md:191.
func ExecuteCommandWithOptions(ctx context.Context, root string, argv []string, environment map[string]string, options CommandOptions) (CommandResult, error) {
	stdoutLimit, stderrLimit := options.StdoutLimit, options.StderrLimit
	if stdoutLimit <= 0 || stdoutLimit > outputLimit || stderrLimit <= 0 || stderrLimit > outputLimit {
		return CommandResult{}, errors.New("invalid bounded tool output limits")
	}
	plan, variables, err := prepareCommand(root, argv, environment)
	if err != nil {
		return CommandResult{}, err
	}
	if len(options.Input) > outputLimit {
		return CommandResult{}, errors.New("tool input limit exceeded")
	}
	plan.Stdin = options.Input
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	stderr := &commandCapture{cancel: cancel, limit: stderrLimit}
	execution, err := supervise(child, plan, 0,
		func(context.Context, int) error { return nil }, defaultProcessOps(),
		runOptions{limit: stdoutLimit, environment: variables, errorOutput: stderr, parentContextOnly: true, mergeStderr: options.MergeStderr})
	data, overflow := stderr.snapshot()
	if overflow {
		execution.Outcome = "output_limit"
	}
	if execution.Outcome == "output_limit" {
		err = errors.Join(err, errors.New("tool output limit exceeded"))
	}
	// The shared capture keeps one internal overflow sentinel; this additive
	// API exposes only the caller's declared bytes after recording overflow.
	// docs/adr/0097-installation-source-image-bundles.md:223.
	if len(execution.Stdout) > stdoutLimit {
		execution.Stdout = execution.Stdout[:stdoutLimit]
	}
	if execution.Outcome != "completed" && err == nil {
		err = errors.New("bounded tool command did not complete")
	}
	return CommandResult{Execution: execution, Stderr: data}, errors.Join(ctx.Err(), err)
}
