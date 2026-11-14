package releasecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/initcmd"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/output"
)

const ciUsage = "Usage: factory ci prepare --go-candidate TAG --origin-target OS/ARCH --ephemeral\n       factory ci workflow --go-candidate TAG --origin-target OS/ARCH --installer-revision FULL_COMMIT_SHA\n       factory ci review-client --go-candidate TAG --expected-revision SHA --output DIR\n\nPrepare authenticated runtime custody in a clean disposable checkout, render an installed-factory workflow, or export a standalone review client.\nUse a fresh checkout after any incomplete preparation; this is not repair.\n"

type ciOptions struct {
	tag, origin string
	ephemeral   bool
}

// RunCI owns explicit CI preparation and workflow rendering.
// docs/adr/0143-ci-checkout-preparation.md:20.
func RunCI(parent context.Context, args []string, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "review-client" {
		return runReviewClient(parent, args[1:], environment, stdin, stdout, stderr)
	}
	// Rendering has no project, environment-selection or acquisition boundary.
	// docs/adr/0145-installed-ci-workflow-rendering.md:36.
	if len(args) > 0 && args[0] == "workflow" {
		return runCIWorkflow(parent, args[1:], stdout, stderr)
	}
	option, help, err := parseCI(args)
	if err != nil {
		_ = writeDiagnostic(stderr, "factory ci: %s; require an exact release, origin target and --ephemeral\n", err)
		return 2
	}
	if help {
		if output.WriteEvent(parent, stdout, []byte(ciUsage)) != nil {
			return 1
		}
		return 0
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return prepareCI(ctx, option, environment, stdin, stdout, stderr)
}

func parseCI(args []string) (ciOptions, bool, error) {
	var option ciOptions
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return option, true, nil
	}
	if len(args) == 0 || args[0] != "prepare" {
		return option, false, errors.New("unknown CI operation")
	}
	seen := map[string]bool{}
	help := false
	for index := 1; index < len(args); index++ {
		name, value, supplied := strings.Cut(args[index], "=")
		if name == "-h" {
			name = "--help"
		}
		if seen[name] {
			return option, false, errors.New("duplicate CI argument")
		}
		seen[name] = true
		switch name {
		case "--help", "--ephemeral":
			if supplied {
				return option, false, errors.New("unexpected CI flag value")
			}
			if name == "--help" {
				help = true
			} else {
				option.ephemeral = true
			}
		case "--go-candidate", "--origin-target":
			if !supplied {
				index++
				if index == len(args) {
					return option, false, errors.New("missing CI argument")
				}
				value = args[index]
			}
			if name == "--go-candidate" {
				if !exactRelease(value) {
					return option, false, errors.New("invalid CI release")
				}
				option.tag = value
			} else {
				if !supportedTarget(value) {
					return option, false, errors.New("invalid CI origin target")
				}
				option.origin = value
			}
		default:
			return option, false, errors.New("unknown CI argument")
		}
	}
	if !help && (option.tag == "" || option.origin == "" || !option.ephemeral) {
		return option, false, errors.New("missing CI consent or selection")
	}
	return option, help, nil
}

func prepareCI(ctx context.Context, option ciOptions, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) (status int) {
	host, err := acquireWithPolicy(ctx, option.tag, environment, false)
	if err != nil {
		_ = writeDiagnostic(stderr, "factory ci: host acquisition failed: %s\n", err)
		return 1
	}
	defer func() { status = host.finish(stderr, status) }()
	digest, err := currentDigest(ctx)
	if ctx.Err() != nil {
		_ = writeDiagnostic(stderr, "factory ci: preparation canceled\n")
		return 1
	}
	// Only the matching authenticated host owner may interpret its selection.
	// docs/adr/0143-ci-checkout-preparation.md:53.
	if err != nil || digest != host.digest {
		args := []string{"ci", "prepare", "--go-candidate", option.tag, "--origin-target", option.origin, "--ephemeral"}
		return authenticatedHandoff(ctx, host, args, environment, stdin, stdout, stderr)
	}
	origin := host
	if option.origin != host.options.Target {
		origin, err = acquireForTarget(ctx, option.tag, option.origin, withoutCache(environment), false)
		if err != nil {
			_ = writeDiagnostic(stderr, "factory ci: origin acquisition failed: %s\n", err)
			return 1
		}
		defer func() { status = origin.finish(stderr, status) }()
	}
	if origin.options.Version != host.options.Version || origin.options.Revision != host.options.Revision {
		_ = writeDiagnostic(stderr, "factory ci: origin and host release identities differ\n")
		return 1
	}
	root, err := os.Getwd()
	if err == nil {
		err = initcmd.PrepareCI(ctx, root, origin.image, host.image, filepath.Join(host.staged, "bin/factory-runtime"), environment)
	}
	if err != nil {
		var ownership *native.OwnershipError
		if errors.As(err, &ownership) {
			host.retain, origin.retain = true, true
		}
		_ = writeDiagnostic(stderr, "factory ci: %s\n", err)
		return 1
	}
	if output.WriteEvent(ctx, stdout, []byte(fmt.Sprintf("factory ci: prepared %s at %s for %s\n", host.options.Version, host.options.Revision, host.options.Target))) != nil {
		return 1
	}
	return 0
}
