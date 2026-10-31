// Package releasecmd connects explicit release selection to the native factory.
package releasecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

var tagPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)

// Claimed reserves malformed selectors before any legacy command dispatch.
// docs/adr/0101-go-release-handoff.md:40.
func Claimed(args []string) bool {
	for _, arg := range args {
		if arg == "--go-candidate" || strings.HasPrefix(arg, "--go-candidate=") {
			return true
		}
	}
	return false
}

// InitClaimed leaves terminated target operands to their initializer owner.
// Upgrade selector/admission behavior is unchanged by this init-only boundary.
// docs/adr/0116-explicit-go-local-initialization.md:26.
func InitClaimed(args []string) bool {
	for index, arg := range args {
		if arg == "--" {
			args = args[:index]
			break
		}
	}
	return Claimed(args)
}

type options struct {
	tag, confirmation string
	args              []string
	dryRun, json      bool
	quiescent         bool
	projectInputs     []string
	requireStable     bool
}

// Run keeps acquisition and migration under one explicit public command.
// docs/adr/0101-go-release-handoff.md:32.
func Run(parent context.Context, verb string, args []string, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) int {
	option, help, err := parse(verb, args)
	if err != nil {
		fmt.Fprintln(stderr, "factory candidate: invalid candidate arguments")
		return 2
	}
	if help {
		fmt.Fprint(stdout, Usage(verb))
		return 0
	}
	return runSelected(parent, verb, option, environment, stdin, stdout, stderr)
}

func runSelected(parent context.Context, verb string, option options, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return execute(ctx, verb, option, environment, stdin, stdout, stderr)
}

// Usage keeps installed and explicit-selector help on the same public contract.
// docs/adr/0101-go-release-handoff.md:543.
func Usage(verb string) string {
	if verb == "init" {
		return "Usage: factory init [TARGET] --go-candidate TAG [--pack PACKS]\n\nSelect one authenticated official Go release candidate.\n"
	}
	return "Usage: factory upgrade --go-candidate TAG [options]\n\nSelect one authenticated official Go release candidate.\nOptions: --dry-run --json --confirm DIGEST --quiescent --project-input KEY=VALUE\nJSON is preview-only; omit --json when confirming.\n"
}

func parse(verb string, args []string) (options, bool, error) {
	var result options
	seen := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		// User option termination survives authenticated init handoff.
		// docs/adr/0118-default-packaged-go-initialization.md:44.
		if verb == "init" && arg == "--" {
			result.args = append(result.args, args[i:]...)
			break
		}
		if arg == "--go-candidate" || strings.HasPrefix(arg, "--go-candidate=") {
			if seen {
				return result, false, errors.New("repeated candidate selector")
			}
			seen = true
			if arg == "--go-candidate" {
				i++
				if i == len(args) {
					return result, false, errors.New("missing candidate tag")
				}
				result.tag = args[i]
			} else {
				result.tag = strings.TrimPrefix(arg, "--go-candidate=")
			}
			continue
		}
		for _, conflict := range []string{"--source", "--ref", "--local", "--installation"} {
			if arg == conflict || strings.HasPrefix(arg, conflict+"=") {
				return result, false, errors.New("conflicting candidate source")
			}
		}
		result.args = append(result.args, arg)
	}
	if !seen || !validTag(result.tag) {
		return result, false, errors.New("invalid candidate tag")
	}
	if verb == "init" {
		for _, arg := range result.args {
			if arg == "--" {
				break
			}
			if arg == "--help" || arg == "-h" {
				return result, true, nil
			}
		}
		return result, false, nil
	}
	if verb != "upgrade" {
		return result, false, errors.New("unsupported candidate operation")
	}
	return parseUpgradeOptions(result, false)
}

// Local and authenticated upgrades share the same consent grammar.
// docs/adr/0117-explicit-go-local-upgrade.md:26.
func parseUpgradeOptions(result options, help bool) (options, bool, error) {
	command := &cobra.Command{Use: "upgrade", Args: cobra.NoArgs, SilenceErrors: true, SilenceUsage: true}
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	command.Flags().BoolVarP(&help, "help", "h", help, "Show upgrade help")
	command.Flags().BoolVar(&result.dryRun, "dry-run", false, "Acquire and inspect without project changes")
	command.Flags().BoolVar(&result.json, "json", false, "Report a candidate preview as JSON")
	command.Flags().BoolVar(&result.quiescent, "quiescent", false, "Declare maintained maintenance exclusion")
	command.Flags().StringVar(&result.confirmation, "confirm", "", "Confirm the current complete proposal digest")
	command.Flags().StringArrayVar(&result.projectInputs, "project-input", nil, "Explicit missing project input KEY=VALUE")
	command.Run = func(*cobra.Command, []string) {}
	// An explicitly empty argument vector must not fall back to os.Args.
	// docs/adr/0117-explicit-go-local-upgrade.md:26.
	command.SetArgs(append([]string{}, result.args...))
	if err := command.Execute(); err != nil {
		return result, false, err
	}
	if !help {
		confirmed := result.confirmation != ""
		previewMutation := result.dryRun && (confirmed || result.quiescent)
		missingExclusion := result.quiescent != confirmed
		confirmedJSON := result.json && confirmed
		invalidDigest := confirmed && (len(result.confirmation) != 64 || strings.Trim(result.confirmation, "0123456789abcdef") != "")
		if previewMutation || missingExclusion || confirmedJSON || invalidDigest {
			return result, false, errors.New("invalid upgrade consent")
		}
	}
	return result, help, nil
}

func validTag(tag string) bool {
	if !tagPattern.MatchString(tag) {
		return false
	}
	switch strings.ToLower(tag) {
	case "main", "head", "latest", "current":
		return false
	}
	return true
}
