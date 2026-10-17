package evalcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/fileinput"
	"github.com/anoop2811/software-factory-template/internal/native"
)

func runner(ctx context.Context, args []string, _ string, environment map[string]string, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "usage: factory eval run HARNESS WORKDIR")
		return 2
	}
	harness, work := args[0], args[1]
	switch harness {
	case "mock", "claude", "codex", "opencode":
	default:
		fmt.Fprintf(stderr, "factory eval run: runner %s is not wired\n", harness)
		return 2
	}
	result, err := runHarness(ctx, harness, work, environment)
	if err != nil || result.OwnershipUnconfirmed || !result.ExitConfirmed || (result.Outcome != "completed" && result.Outcome != "failed") {
		fmt.Fprintf(stderr, "%s runner: could not complete the run\n", harness)
		return 1
	}
	return 0
}

// Keep the source runners' final literal prompt and combined log stream.
// docs/adr/0099-native-factory-orchestration.md:115.
func runHarness(ctx context.Context, harness, work string, environment map[string]string) (native.CommandResult, error) {
	if harness == "mock" {
		return runMock(ctx, work, environment)
	}
	if work == "" {
		return native.CommandResult{}, errors.New("missing runner workdir")
	}
	prompt, err := fileinput.Read(ctx, work+"/task.md", fileLimit)
	if err != nil {
		return native.CommandResult{}, err
	}
	argv := []string{harness}
	switch harness {
	case "claude":
		argv = append(argv, "-p", "--permission-mode", "acceptEdits")
	case "codex":
		argv = append(argv, "exec", "--sandbox", "workspace-write", "--skip-git-repo-check")
	case "opencode":
		argv = append(argv, "run", "--agent", "implementer")
	default:
		return native.CommandResult{}, errors.New("unknown runner")
	}
	if model := environment["FACTORY_EVAL_MODEL"]; model != "" {
		argv = append(argv, "--model", model)
	}
	argv = append(argv, strings.TrimRight(strings.ReplaceAll(string(prompt), "\x00", ""), "\n"))
	result, err := native.ExecuteCommandWithOptions(ctx, work, argv, environment, native.CommandOptions{StdoutLimit: fileLimit, StderrLimit: fileLimit, MergeStderr: true})
	if ctx.Err() == nil && result.Outcome == "failed" && result.ExitConfirmed && !result.OwnershipUnconfirmed && result.ExitCode != nil && *result.ExitCode != 0 {
		err = nil // The source runner returns success after the CLI's ordinary exit.
	}
	if writeErr := publish(ctx, work, harness+"-runner.log", result.Stdout); writeErr != nil {
		err = errors.Join(err, writeErr)
	}
	return result, err
}

func runMock(ctx context.Context, work string, environment map[string]string) (native.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return native.CommandResult{}, err
	}
	info, err := os.Stat(work)
	if err != nil || !info.IsDir() {
		return native.CommandResult{}, errors.New("missing mock workdir")
	}
	switch environment["FACTORY_MOCK_MODE"] {
	case "", "pass", "cheat":
		err = os.WriteFile(work+"/answer.txt", []byte("FIXED\n"), 0600)
		if err == nil && environment["FACTORY_MOCK_MODE"] == "cheat" {
			err = os.WriteFile(work+"/verify.sh", nil, 0600)
		}
	}
	code := 0
	return native.CommandResult{Execution: native.Execution{Outcome: "completed", ExitCode: &code, ExitConfirmed: true}}, errors.Join(err, ctx.Err())
}
