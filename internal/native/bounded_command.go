package native

import (
	"context"
	"errors"
)

// ExecuteCommandBounded shares process ownership with existing tool execution,
// using only the caller's deadline and independently bounded output channels.
// docs/adr/0097-installation-source-image-bundles.md:196.
func ExecuteCommandBounded(ctx context.Context, root string, argv []string, environment map[string]string, stdoutLimit, stderrLimit int) (CommandResult, error) {
	if stdoutLimit <= 0 || stdoutLimit > outputLimit || stderrLimit <= 0 || stderrLimit > outputLimit {
		return CommandResult{}, errors.New("invalid bounded tool output limits")
	}
	plan, variables, err := prepareCommand(root, argv, environment)
	if err != nil {
		return CommandResult{}, err
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	stderr := &commandCapture{cancel: cancel, limit: stderrLimit}
	execution, err := supervise(child, plan, 0,
		func(context.Context, int) error { return nil }, defaultProcessOps(),
		runOptions{limit: stdoutLimit, environment: variables, errorOutput: stderr, parentContextOnly: true})
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
