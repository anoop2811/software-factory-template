package releasecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/packaging"
	"github.com/anoop2811/software-factory-template/internal/staging"
)

// LocalClaimed reserves malformed local controls after richer existing owners.
// docs/adr/0108-default-go-cutover.md:67.
func LocalClaimed(args []string) bool {
	for _, arg := range args {
		if arg == "--local" || strings.HasPrefix(arg, "--local=") {
			return true
		}
	}
	return false
}

// RunLocal is an explicit developer build, never a release-verification fallback.
// docs/adr/0108-default-go-cutover.md:62.
func RunLocal(parent context.Context, verb string, args []string, sourceRoot string, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	option, source, help, err := parseLocal(verb, args, sourceRoot)
	if err != nil {
		fmt.Fprintln(stderr, "factory local-source:", err)
		return 2
	}
	if help {
		fmt.Fprintln(stdout, "Usage: factory init [TARGET] --local [--source DIR] [initializer options]\n       factory upgrade --local --source DIR [--dry-run --json | --confirm DIGEST --quiescent]\n\nBuild the clean committed source HEAD using the provisioned developer Go toolchain; no publisher authentication is claimed.")
		return 0
	}
	candidate, err := acquireLocal(ctx, source, environment, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "factory local-source:", err)
		return 1
	}
	option.tag = candidate.options.Version
	return executeSelection(ctx, verb, option, candidate, environment, stdin, stdout, stderr)
}

func parseLocal(verb string, args []string, sourceRoot string) (options, string, bool, error) {
	forwarded := make([]string, 0, len(args)+2)
	source, localSeen, sourceSeen := sourceRoot, false, false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--local":
			if localSeen {
				return options{}, "", false, errors.New("repeated local selector")
			}
			localSeen = true
		case arg == "--source" || strings.HasPrefix(arg, "--source="):
			if sourceSeen {
				return options{}, "", false, errors.New("repeated source directory")
			}
			sourceSeen = true
			if arg == "--source" {
				i++
				if i == len(args) {
					return options{}, "", false, errors.New("--source requires a directory")
				}
				source = args[i]
			} else {
				source = strings.TrimPrefix(arg, "--source=")
			}
		case strings.HasPrefix(arg, "--local="):
			return options{}, "", false, errors.New("--local does not take a value")
		default:
			forwarded = append(forwarded, arg)
		}
	}
	if !localSeen || source == "" || strings.ContainsAny(source, "\x00\r\n") || verb == "upgrade" && !sourceSeen {
		return options{}, "", false, errors.New("local upgrade requires an explicit --source DIR; local init requires an invoked developer checkout or --source DIR")
	}
	forwarded = append(forwarded, "--go-candidate", DefaultVersion)
	option, help, err := parse(verb, forwarded)
	return option, source, help, err
}

func acquireLocal(ctx context.Context, source string, environment map[string]string, diagnostics io.Writer) (result *acquired, returned error) {
	physical, err := filepath.EvalSymlinks(source)
	if err == nil {
		physical, err = filepath.Abs(physical)
	}
	if err != nil {
		return nil, errors.New("cannot resolve the explicit source directory")
	}
	env := make(map[string]string, len(environment)+2)
	for key, value := range environment {
		if !strings.HasPrefix(key, "GIT_") {
			env[key] = value
		}
	}
	env["GIT_CONFIG_NOSYSTEM"], env["GIT_CONFIG_GLOBAL"] = "1", os.DevNull
	git := func(args ...string) (string, error) {
		child, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		value, err := native.ExecuteCommandBounded(child, physical, append([]string{"git"}, args...), env, 1<<20, 64<<10)
		if err != nil || value.ExitCode == nil || *value.ExitCode != 0 || !value.ExitConfirmed || value.OwnershipUnconfirmed {
			return "", errors.Join(err, errors.New("cannot inspect committed source Git identity"))
		}
		return string(value.Stdout), nil
	}
	dirty, err := git("status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	if dirty != "" {
		return nil, errors.New("source has dirty or uncommitted changes; commit them or select a clean committed source checkout")
	}
	revision, err := git("rev-parse", "--verify", "HEAD^{commit}")
	revision = strings.TrimSpace(revision)
	if err != nil || !revisionPattern.MatchString(revision) {
		return nil, errors.Join(err, errors.New("source requires one full committed HEAD identity"))
	}
	a, err := newAcquired()
	if err != nil {
		return nil, err
	}
	a.source = physical
	defer func() {
		if result == nil {
			returned = errors.Join(returned, a.close())
		}
	}()
	target, version := runtime.GOOS+"/"+runtime.GOARCH, "local-"+revision
	if _, err := fmt.Fprintf(diagnostics, "factory local-source: source=%s revision=%s version=%s target=%s (committed source; no publisher authentication)\n", physical, revision, version, target); err != nil {
		return nil, err
	}
	built, err := packaging.Build(ctx, packaging.Options{Source: physical, Revision: revision, Target: target, Output: filepath.Join(a.directory, "package"), Version: version, Installation: true})
	if err != nil {
		return nil, err
	}
	a.options = staging.Options{Archive: built.Archive, Version: version, Revision: revision, Target: target, Output: filepath.Join(a.directory, "stage"), Local: true, Installation: true}
	staged, err := staging.Stage(ctx, a.options)
	if err != nil {
		return nil, err
	}
	a.image, err = staging.Inspect(ctx, a.options)
	if err != nil {
		return nil, err
	}
	if a.image.ArchiveSHA256 != staged.SHA256 || staged.SHA256 != built.SHA256 {
		return nil, errors.New("local package changed during qualification")
	}
	a.staged = filepath.Join(staged.Root, version, target)
	a.raw, a.digest = filepath.Join(a.staged, "bin/factory-runtime"), a.image.Runtime.SHA256
	return a, nil
}
