// Package cli preserves the existing factory command boundary while individual
// implementations migrate. specs/001-go-runtime-conversion.md:91.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

const usage = `factory — software factory control surface

Usage: factory <command> [args]

  init        Set up the factory in the current repo (writes factory.yaml, installs hooks)
  doctor      Report the health of an installed factory: armed/inert/stale gates,
              hook + adapter integrity, and a break/fix proof that every gate fires
  upgrade     Pull framework updates (hooks, scripts, docs) over this repo; never
              touches your factory.yaml, content, or customized files
  check       Run the full pre-push gate suite
  selftest    Run the break/fix self-test (watch every gate fire)
  review-lane Enable or disable the advisory adversarial PR review (opt-in, costs tokens)
  metrics     What the factory is doing to this repo (--json, --html); local only
              (--html opens the page; --no-open just writes it)
  report      Cost report: gates enforced at 0 tokens, blocks caught, one labeled
              estimate (factory report --clear resets the block log)
  budget      Plan, bound, and report opt-in Codex/Claude/OpenCode invocations
  loop        Run manual checks or opt-in bounded implementation and repair
  migrate-config  Move a legacy factory.config into factory.yaml (--dry-run to preview)
  help        Show this message

Commands use auditable scripts. Inspect scripts/ for their implementation.
`

var scripts = map[string]string{
	"init":           "factory-init.sh",
	"doctor":         "factory-doctor.sh",
	"upgrade":        "factory-upgrade.sh",
	"check":          "pre-push-check.sh",
	"selftest":       "selftest/run.sh",
	"report":         "factory-report.sh",
	"budget":         "factory-budget.sh",
	"loop":           "factory-loop.sh",
	"metrics":        "factory-metrics.sh",
	"review-lane":    "factory-review-lane.sh",
	"migrate-config": "factory-migrate-config.sh",
}

// Run routes the legacy first command token without reinterpreting script flags.
// Cobra defaults must not change this contract. specs/001-go-runtime-conversion.md:293.
func Run(ctx context.Context, args []string) int {
	command := "help"
	if len(args) > 0 && args[0] != "" {
		command = args[0]
	}
	if command == "help" || command == "-h" || command == "--help" {
		// The shell dispatcher ignores all remaining arguments for these aliases.
		args = []string{"help"}
	} else if _, ok := scripts[command]; !ok {
		fmt.Fprintf(os.Stderr, "factory: unknown command '%s' (try: factory help)\n", command)
		return 2
	}

	root := &cobra.Command{
		Use:                "factory",
		SilenceErrors:      true,
		SilenceUsage:       true,
		DisableSuggestions: true,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		CompletionOptions:  cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	root.SetHelpCommand(&cobra.Command{
		Use:                "help",
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			_, err := io.WriteString(os.Stdout, usage)
			return err
		},
	})
	for name, script := range scripts {
		root.AddCommand(&cobra.Command{
			Use:                name,
			DisableFlagParsing: true,
			Args:               cobra.ArbitraryArgs,
			RunE: func(cmd *cobra.Command, forwarded []string) error {
				return dispatch(cmd.Context(), name, script, forwarded)
			},
		})
	}
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return 0
}

func dispatch(ctx context.Context, command, script string, args []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	invocation := os.Args[0]
	if !strings.ContainsRune(invocation, filepath.Separator) {
		path, err := exec.LookPath(invocation)
		// A relative PATH entry is valid here: the caller already selected and
		// started this executable, and the shell boundary permits such entries.
		if err != nil && !errors.Is(err, exec.ErrDot) {
			return fmt.Errorf("factory: locate dispatcher: %w", err)
		}
		invocation = path
	}
	directory, err := filepath.Abs(filepath.Dir(invocation))
	if err != nil {
		return fmt.Errorf("factory: locate dispatcher directory: %w", err)
	}
	path := filepath.Join(directory, "scripts", script)
	if err := syscall.Access(path, 1); err != nil {
		return fmt.Errorf("factory: '%s' is not available in this repo (%s is missing or not executable)", command, path)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Replace this process, just as the existing shell exec does, preserving
	// PID, inherited streams, working directory, environment and signal delivery.
	// #nosec G204 G702 -- A fixed command map selects the colocated trusted script; argv is passed directly without shell evaluation.
	if err := syscall.Exec(path, append([]string{path}, args...), os.Environ()); err == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// The legacy env-selected Bash supplies no-shebang interpretation and native
	// execution-failure statuses, which differ across supported platforms.
	// Keep the exec program constant: the script and all arguments are positional.
	fallback := append([]string{"env", "bash", "-c", `exec "$0" "$@"`, path}, args...)
	// #nosec G204 G702 -- The shell program is constant; trusted script path and literal argv are separate positional parameters.
	if err := syscall.Exec("/usr/bin/env", fallback, os.Environ()); err != nil {
		return fmt.Errorf("factory: execute '%s': %w", command, err)
	}
	return nil
}
