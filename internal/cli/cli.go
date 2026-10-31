// Package cli owns the factory command boundary and its native Cobra routes.
// specs/001-go-runtime-conversion.md:91.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/adversarialreviewcmd"
	"github.com/anoop2811/software-factory-template/internal/checkcmd"
	"github.com/anoop2811/software-factory-template/internal/configuredcmd"
	"github.com/anoop2811/software-factory-template/internal/doctorcmd"
	"github.com/anoop2811/software-factory-template/internal/evalcmd"
	"github.com/anoop2811/software-factory-template/internal/installationcmd"
	"github.com/anoop2811/software-factory-template/internal/installedproof"
	"github.com/anoop2811/software-factory-template/internal/metricscmd"
	"github.com/anoop2811/software-factory-template/internal/migrateconfigcmd"
	"github.com/anoop2811/software-factory-template/internal/output"
	"github.com/anoop2811/software-factory-template/internal/prereqcmd"
	"github.com/anoop2811/software-factory-template/internal/recoverycmd"
	"github.com/anoop2811/software-factory-template/internal/releasecmd"
	"github.com/anoop2811/software-factory-template/internal/reportcmd"
	"github.com/anoop2811/software-factory-template/internal/reviewbatch"
	"github.com/anoop2811/software-factory-template/internal/reviewci"
	"github.com/anoop2811/software-factory-template/internal/reviewlanecmd"
	"github.com/anoop2811/software-factory-template/internal/synccmd"
	"github.com/anoop2811/software-factory-template/internal/upgradecmd"
	"github.com/spf13/cobra"
)

const usage = `factory — software factory control surface

Usage: factory <command> [args]

  init        Set up the factory in the current repo (writes factory.yaml, installs hooks)
              Explicit local Go install: init [TARGET] --local [--source DIR]
  doctor      Report the health of an installed factory: armed/inert/stale gates,
              hook + adapter integrity, and a break/fix proof that every gate fires
  upgrade     Preview authenticated Go framework updates; apply with fresh consent
              Preserve project content and customized files
              Preview locally with --dry-run --source PATH (run at installation root)
              Plan recovery with --plan-restore --migration-id ID (read-only; always blocked)
  sync        Synchronize harness adapters (all, opencode, claude, codex)
  prereq      Check required tools and configured Go minimum
  check       Run the full pre-push gate suite
  eval        Run structural or golden evaluation, or a selected harness runner
  review      Review a diff with the configured adversarial provider
  selftest    Run the break/fix self-test (watch every gate fire)
  review-batch Review complete PR diffs in bounded, budgeted model batches
  review-lane Enable or disable the advisory adversarial PR review (opt-in, costs tokens)
  metrics     What the factory is doing to this repo (--json, --html); local only
              (--html opens the page; --no-open just writes it)
  report      Cost report: gates enforced at 0 tokens, blocks caught, one labeled
              estimate (factory report --clear resets the block log)
  budget      Plan, bound, and report opt-in Codex/Claude/OpenCode invocations
  loop        Run manual checks or opt-in bounded implementation and repair
  migrate-config  Move a legacy factory.config into factory.yaml (--dry-run to preview)
  help        Show this message

All commands use the Go runtime.
`

var commands = map[string]struct{}{
	"init":           {},
	"doctor":         {},
	"upgrade":        {},
	"check":          {},
	"selftest":       {},
	"report":         {},
	"budget":         {},
	"loop":           {},
	"metrics":        {},
	"review-lane":    {},
	"migrate-config": {},
	"sync":           {},
	"prereq":         {},
	"eval":           {},
	"review":         {},
	"review-batch":   {},
}

type installationKey struct{}
type installationContext struct{ root, assets, version string }

// RunInstalled receives the validated physical context; no environment value grants it.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:109.
func RunInstalled(ctx context.Context, args []string, root, assets, version string) int {
	return Run(context.WithValue(ctx, installationKey{}, installationContext{root: root, assets: assets, version: version}), args)
}
func assetDirectory(ctx context.Context) (string, error) {
	if selected, ok := ctx.Value(installationKey{}).(installationContext); ok {
		return selected.assets, nil
	}
	return dispatcherDirectory()
}

// Run preserves top-level command admission; native owners parse their arguments.
// Cobra defaults must not change this contract. specs/001-go-runtime-conversion.md:293.
func Run(ctx context.Context, args []string) int {
	command := "help"
	if len(args) > 0 && args[0] != "" {
		command = args[0]
	}
	if command == "help" || command == "-h" || command == "--help" {
		// The shell dispatcher ignores all remaining arguments for these aliases.
		args = []string{"help"}
	} else if _, ok := commands[command]; !ok {
		fmt.Fprintf(os.Stderr, "factory: unknown command '%s' (try: factory help)\n", command)
		return 2
	}
	// New orchestration owns cancellation until every child has been reconciled.
	// docs/adr/0099-native-factory-orchestration.md:81.
	switch command {
	case "sync", "prereq", "check", "eval", "review", "selftest":
		var stop context.CancelFunc
		ctx, stop = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stop()
	}

	status := 0
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
	for name := range commands {
		root.AddCommand(&cobra.Command{
			Use:                name,
			DisableFlagParsing: true,
			Args:               cobra.ArbitraryArgs,
			RunE: func(cmd *cobra.Command, forwarded []string) error {
				// Candidate ownership precedes both installed and source dispatch.
				// docs/adr/0101-go-release-handoff.md:40.
				if (name == "init" && releasecmd.InitClaimed(forwarded)) || (name == "upgrade" && releasecmd.Claimed(forwarded)) {
					status = releasecmd.Run(cmd.Context(), name, forwarded, configuredcmd.CaptureEnvironment(), os.Stdin, os.Stdout, os.Stderr)
					return nil
				}
				// Explicit local init runs the selected committed initializer.
				// docs/adr/0116-explicit-go-local-initialization.md:13.
				if name == "init" && releasecmd.LocalClaimed(forwarded) {
					var source func() (string, error)
					if _, installed := cmd.Context().Value(installationKey{}).(installationContext); !installed {
						source = dispatcherDirectory
					}
					status = releasecmd.RunLocalInit(cmd.Context(), forwarded, source, configuredcmd.CaptureEnvironment(), os.Stdin, os.Stdout, os.Stderr)
					return nil
				}
				// Whole-installation markers own malformed requests before older routes.
				// docs/adr/0098-whole-installation-upgrade-and-rollback.md:33.
				if name == "upgrade" && installationcmd.Claimed(forwarded) {
					status = installationcmd.Run(cmd.Context(), forwarded, configuredcmd.CaptureEnvironment(), os.Stdout, os.Stderr)
					return nil
				}
				// Local upgrade owns only its explicit selector, preserving assessment.
				// docs/adr/0117-explicit-go-local-upgrade.md:31.
				if name == "upgrade" && releasecmd.LocalUpgradeClaimed(forwarded) {
					var source func() (string, error)
					if _, installed := cmd.Context().Value(installationKey{}).(installationContext); !installed {
						source = dispatcherDirectory
					}
					status = releasecmd.RunLocalUpgrade(cmd.Context(), forwarded, source, configuredcmd.CaptureEnvironment(), os.Stdin, os.Stdout, os.Stderr)
					return nil
				}
				// Standalone controls reserve malformed requests before legacy dispatch,
				// after explicit selectors own their values. docs/adr/0106-go-backup-retention.md:133.
				if name == "upgrade" && releasecmd.BackupClaimed(forwarded) {
					status = releasecmd.RunBackup(cmd.Context(), forwarded, configuredcmd.CaptureEnvironment(), os.Stdin, os.Stdout, os.Stderr)
					return nil
				}
				// Trusted CI and local callers share one bounded Go review engine.
				// docs/adr/0100-batched-review.md:23.
				if name == "review-batch" {
					ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
					defer stop()
					environment := configuredcmd.CaptureEnvironment()
					if len(forwarded) > 0 && forwarded[0] == "github" {
						status = reviewci.Run(ctx, forwarded[1:], environment, os.Stdout, os.Stderr)
					} else {
						status = reviewbatch.Run(ctx, forwarded, environment, os.Stdout, os.Stderr)
					}
					return nil
				}
				// Native orchestration never executes its old script implementation.
				// docs/adr/0099-native-factory-orchestration.md:20.
				switch name {
				case "sync", "prereq", "check", "eval", "review":
					directory, err := assetDirectory(cmd.Context())
					if err != nil {
						return err
					}
					if selected, ok := cmd.Context().Value(installationKey{}).(installationContext); ok {
						directory = selected.root
					}
					environment := configuredcmd.CaptureEnvironment()
					switch name {
					case "sync":
						status = synccmd.Run(cmd.Context(), forwarded, directory, environment, os.Stdout, os.Stderr)
					case "prereq":
						status = prereqcmd.Run(cmd.Context(), forwarded, directory, environment, os.Stdout, os.Stderr)
					case "check":
						status = checkcmd.Run(cmd.Context(), forwarded, directory, environment, os.Stdin, os.Stdout, os.Stderr)
					case "eval":
						// Evaluation inputs and results belong to the caller's
						// project, including invocation through another checkout.
						// docs/adr/0099-native-factory-orchestration.md:99.
						directory, err = os.Getwd()
						if err != nil {
							return err
						}
						status = evalcmd.Run(cmd.Context(), forwarded, directory, environment, os.Stdout, os.Stderr)
					case "review":
						status = adversarialreviewcmd.Run(cmd.Context(), forwarded, directory, environment, os.Stdout, os.Stderr)
					}
					return nil
				}
				if name == "selftest" {
					var err error
					if selected, ok := cmd.Context().Value(installationKey{}).(installationContext); ok {
						err = installedproof.Run(cmd.Context(), selected.root, os.Stdout)
					} else {
						var directory string
						directory, err = dispatcherDirectory()
						if err == nil {
							// Reuse native gate pairs without the historical controller.
							// docs/adr/0120-native-source-gate-proof.md:9.
							err = installedproof.RunSource(cmd.Context(), directory, os.Stdout)
						}
					}
					if err != nil {
						// Cancellation also bounds borrowed diagnostic streams.
						// docs/adr/0120-native-source-gate-proof.md:134.
						diagnostic, cancel := context.WithTimeout(context.WithoutCancel(cmd.Context()), time.Second)
						defer cancel()
						_ = output.WriteEvent(diagnostic, os.Stderr, []byte(fmt.Sprintf("factory selftest: %v\n", err)))
						status = 1
					}
					return nil
				}
				if name == "migrate-config" {
					status = migrateconfigcmd.Run(cmd.Context(), forwarded, configuredcmd.CaptureEnvironment(), os.Stdout, os.Stderr)
					return nil
				}
				if name == "review-lane" {
					directory, err := assetDirectory(cmd.Context())
					if err != nil {
						return err
					}
					status = reviewlanecmd.Run(cmd.Context(), forwarded, directory, configuredcmd.CaptureEnvironment(), os.Stdout, os.Stderr)
					return nil
				}

				// Native metrics use only the invocation's HTML template asset.
				// docs/adr/0088-go-native-metrics.md:29.
				if name == "metrics" {
					directory, err := assetDirectory(cmd.Context())
					if err != nil {
						return err
					}
					status = metricscmd.Run(cmd.Context(), forwarded, directory, configuredcmd.CaptureEnvironment(), os.Stdout, os.Stderr)
					return nil
				}
				// Reporting selects assets beside the invoked dispatcher.
				// docs/adr/0087-go-native-report.md:22.
				if name == "report" {
					directory, err := assetDirectory(cmd.Context())
					if err != nil {
						return err
					}
					if selected, ok := cmd.Context().Value(installationKey{}).(installationContext); ok {
						directory = selected.root
					}
					status = reportcmd.Run(cmd.Context(), forwarded, directory, configuredcmd.CaptureEnvironment(), os.Stdout, os.Stderr)
					return nil
				}
				// Native diagnostics retain only the explicit shared tool boundaries.
				// docs/adr/0086-go-native-doctor.md:18.
				if name == "doctor" {
					status = doctorcmd.Run(cmd.Context(), configuredcmd.CaptureEnvironment(), os.Stdout, os.Stderr)
					return nil
				}
				// Ordinary Go init selects an authenticated packaged initializer.
				// docs/adr/0118-default-packaged-go-initialization.md:9.
				if name == "init" {
					version := ""
					if selected, installed := cmd.Context().Value(installationKey{}).(installationContext); installed {
						version = selected.version
					}
					status = releasecmd.RunDefaultInit(cmd.Context(), forwarded, version, configuredcmd.CaptureEnvironment(), os.Stdin, os.Stdout, os.Stderr)
					return nil
				}
				// Source-built budget/loop commands do not select colocated scripts.
				// docs/adr/0078-go-public-budget-loop.md:29.
				if name == "budget" || name == "loop" {
					code, err := configuredcmd.Run(cmd.Context(), name, forwarded, configuredcmd.CaptureEnvironment(), os.Stdout, os.Stderr)
					status = code
					if err != nil {
						return fmt.Errorf("factory: %w", err)
					}
					return nil
				}
				// Source-only assessments retain their prior installed restriction;
				// ordinary upgrades use the authenticated controller.
				// docs/adr/0122-default-packaged-go-upgrade.md:31.
				if name == "upgrade" {
					if legacyUpgradeClaimed(forwarded) {
						if _, installed := cmd.Context().Value(installationKey{}).(installationContext); installed {
							fmt.Fprintln(os.Stderr, "factory upgrade: installed upgrades require --installation (use upgrade --installation --help)")
							status = 2
						} else if recoverycmd.Claimed(forwarded) {
							status = recoverycmd.Run(cmd.Context(), forwarded, configuredcmd.CaptureEnvironment(), os.Stdout, os.Stderr)
						} else {
							status = upgradecmd.Run(cmd.Context(), forwarded, os.Stdout, os.Stderr)
						}
					} else {
						status = releasecmd.RunDefaultUpgrade(cmd.Context(), forwarded, configuredcmd.CaptureEnvironment(), os.Stdin, os.Stdout, os.Stderr)
					}
					return nil
				}
				return fmt.Errorf("factory: native command %q has no owner", name)
			},
		})
	}
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return status
}

func dispatcherDirectory() (string, error) {
	invocation := os.Args[0]
	if !strings.ContainsRune(invocation, filepath.Separator) {
		path, err := exec.LookPath(invocation)
		// A relative PATH entry is valid here: the caller already selected and
		// started this executable, and the shell boundary permits such entries.
		if err != nil && !errors.Is(err, exec.ErrDot) {
			return "", fmt.Errorf("factory: locate dispatcher: %w", err)
		}
		invocation = path
	}
	directory, err := filepath.Abs(filepath.Dir(invocation))
	if err != nil {
		return "", fmt.Errorf("factory: locate dispatcher directory: %w", err)
	}
	return directory, nil
}
