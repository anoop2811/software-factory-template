package releasecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/installation"
)

const localUpgradeProtocol = "--factory-local-upgrade"

// LocalUpgradeClaimed reserves explicit local selection. A bare --source still
// belongs to the existing assessment route. docs/adr/0117-explicit-go-local-upgrade.md:31.
func LocalUpgradeClaimed(args []string) bool {
	for _, arg := range args {
		if arg == "--local" || strings.HasPrefix(arg, "--local=") {
			return true
		}
	}
	return false
}

type localUpgradeOptions struct {
	local        localOptions
	upgrade      options
	beforeSource string
}

func parseLocalUpgrade(args []string) (result localUpgradeOptions, returned error) {
	var remaining []string
	seen := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			remaining = append(remaining, args[i:]...)
			break
		}
		if arg != "--before-source" && !strings.HasPrefix(arg, "--before-source=") {
			remaining = append(remaining, arg)
			continue
		}
		if seen {
			return result, errors.New("repeated predecessor source directory")
		}
		seen = true
		if arg == "--before-source" {
			i++
			if i == len(args) || strings.HasPrefix(args[i], "--") {
				return result, errors.New("--before-source requires a directory")
			}
			result.beforeSource = args[i]
		} else {
			result.beforeSource = strings.TrimPrefix(arg, "--before-source=")
		}
	}
	if seen && result.beforeSource == "" || strings.ContainsAny(result.beforeSource, "\x00\r\n") {
		return result, errors.New("invalid predecessor source directory")
	}
	result.local, returned = parseLocalSource(remaining, "upgrade")
	if returned != nil {
		return result, returned
	}
	result.upgrade, result.local.help, returned = parseUpgradeOptions(options{args: result.local.args}, result.local.help)
	return result, returned
}

const localUpgradeUsage = "Usage: factory upgrade --local [--source DIR] [--before-source DIR] [options]\n\nBuild clean committed source using the provisioned Go toolchain; no publisher authentication is claimed.\nGo predecessors require a matching clean --before-source checkout and exact installed runtime/asset bytes.\nOptions: --dry-run --json --confirm DIGEST --quiescent --project-input KEY=VALUE\nJSON is preview-only; omit --json when confirming.\n"

// RunLocalUpgrade builds once and delegates the complete operation to that image.
// docs/adr/0117-explicit-go-local-upgrade.md:9.
func RunLocalUpgrade(parent context.Context, args []string, source func() (string, error), environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	option, err := parseLocalUpgrade(args)
	if err != nil {
		fmt.Fprintln(stderr, "factory local-source:", err)
		return 2
	}
	if option.local.help {
		fmt.Fprint(stdout, localUpgradeUsage)
		return 0
	}
	if option.beforeSource != "" {
		// Keep the selector ahead of a valid user option terminator.
		option.local.args = append([]string{"--before-source=" + option.beforeSource}, option.local.args...)
	}
	return runLocalSource(ctx, "upgrade", localUpgradeProtocol, option.local, source, environment, stdin, stdout, stderr)
}

// LocalUpgradeImageClaimed keeps this transport distinct from local-init v1.
// docs/adr/0117-explicit-go-local-upgrade.md:52.
func LocalUpgradeImageClaimed(args []string) bool {
	return len(args) > 0 && args[0] == localUpgradeProtocol
}

// RunLocalUpgradeImage consumes one qualified snapshot, then uses its own planner.
// Physical installed and bridge admission must precede this private receiver.
// docs/adr/0117-explicit-go-local-upgrade.md:59.
func RunLocalUpgradeImage(parent context.Context, args []string, environment map[string]string, stdout, stderr io.Writer) (status int) {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if !validLocalImage(args, localUpgradeProtocol) {
		fmt.Fprintln(stderr, "factory local-source: invalid private local upgrade protocol")
		return 2
	}
	selectors := []string{"--local", "--source", args[3]}
	option, err := parseLocalUpgrade(append(selectors, args[7:]...))
	if err != nil || option.local.help {
		localFailure(stderr, "invalid private local upgrade arguments", err)
		return 2
	}
	image, err := snapshotLocalImage(ctx, args, environment, "upgrader")
	if err != nil {
		fmt.Fprintln(stderr, "factory local-source:", err)
		return 1
	}
	defer func() { status = image.finish(stderr, status) }()
	return executeLocalUpgrade(ctx, option, image, localEnvironment(environment), stdout, stderr)
}

func executeLocalUpgrade(ctx context.Context, option localUpgradeOptions, image *acquired, environment map[string]string, stdout, stderr io.Writer) (status int) {
	root, err := projectRoot(ctx, environment)
	if err != nil {
		localFailure(stderr, "target root unavailable", err)
		return 1
	}
	request, err := installation.InferCandidate(ctx, root, image.options, option.upgrade.projectInputs)
	if err != nil {
		fmt.Fprintln(stderr, "factory local-source:", err)
		return installation.ErrorStatus(err)
	}
	var previous *acquired
	if request.Profile == "go-hybrid-v1" {
		if option.beforeSource == "" {
			fmt.Fprintln(stderr, "factory local-source: Go predecessor requires --before-source DIR; retain its original qualified archive if exact reconstruction is unavailable")
			return 2
		}
		physical, revision, err := inspectLocalSource(ctx, option.beforeSource, environment)
		if err != nil {
			localFailure(stderr, "cannot inspect predecessor source", err)
			return 1
		}
		if revision != request.Before.Revision || request.Before.Target != request.Source.Target || !validTag(request.Before.Version) {
			fmt.Fprintln(stderr, "factory local-source: predecessor source identity does not match the installed version, revision and native target")
			return 2
		}
		previous, err = acquireLocalVersion(ctx, physical, revision, request.Before.Version, stderr)
		if err != nil {
			localFailure(stderr, "predecessor acquisition failed", err)
			return 1
		}
		defer func() { status = previous.finish(stderr, status) }()
		request.Before = previous.options
	} else if option.beforeSource != "" {
		fmt.Fprintln(stderr, "factory local-source: --before-source is only valid for a Go predecessor")
		return 2
	}
	operation := upgradeOperation{root: root, request: request, candidate: image, previous: previous, prefix: "factory local-source"}
	operation.preview = func(proposal installation.RetentionProposal) string {
		return fmt.Sprintf("local %s commit=%s target=%s (committed source; project unchanged; no publisher authentication)\nprofile=%s proposal_digest=%s\nactions=%d blockers=%d\nApply by repeating the same Go invocation with:\n--confirm '%s' --quiescent\n", request.Source.Version, request.Source.Revision, request.Source.Target, proposal.Profile, proposal.ProposalDigest, len(proposal.Actions), len(proposal.Blockers), proposal.ProposalDigest)
	}
	if previous != nil {
		operation.blockerHint = "Go predecessor bytes must match exactly. Preserve customized entries; if reconstruction differs, retain the original qualified archive and use upgrade --installation --help for explicit before-image operands."
	}
	return operation.run(ctx, option.upgrade, stdout, stderr)
}
