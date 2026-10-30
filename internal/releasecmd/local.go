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

// LocalClaimed reserves malformed init selectors before ordinary initialization.
// docs/adr/0116-explicit-go-local-initialization.md:23.
func LocalClaimed(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--local" || strings.HasPrefix(arg, "--local=") || arg == "--source" || strings.HasPrefix(arg, "--source=") {
			return true
		}
	}
	return false
}

type localOptions struct {
	source string
	args   []string
	help   bool
}

func parseLocalInit(args []string, source string) (localOptions, error) {
	result := localOptions{source: source}
	localSeen, sourceSeen := false, false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			result.args = append(result.args, args[i:]...)
			i = len(args)
		case arg == "--local":
			if localSeen {
				return result, errors.New("repeated local selector")
			}
			localSeen = true
		case strings.HasPrefix(arg, "--local="):
			return result, errors.New("--local does not take a value")
		case arg == "--source" || strings.HasPrefix(arg, "--source="):
			if sourceSeen {
				return result, errors.New("repeated source directory")
			}
			sourceSeen = true
			if arg == "--source" {
				i++
				if i == len(args) || strings.HasPrefix(args[i], "--") {
					return result, errors.New("--source requires a directory")
				}
				result.source = args[i]
			} else {
				result.source = strings.TrimPrefix(arg, "--source=")
			}
		case arg == "--help" || arg == "-h":
			result.help = true
		default:
			for _, conflict := range []string{"--go-candidate", "--ref", "--installation"} {
				if arg == conflict || strings.HasPrefix(arg, conflict+"=") {
					return result, errors.New("conflicting local source selector")
				}
			}
			result.args = append(result.args, arg)
		}
	}
	if !localSeen || sourceSeen && result.source == "" || strings.ContainsAny(result.source, "\x00\r\n") {
		return result, errors.New("local init requires --local and a valid source directory")
	}
	if !result.help && result.source == "" {
		return result, errors.New("installed local init requires --source DIR")
	}
	return result, nil
}

// RunLocalInit packages clean source, then delegates to that exact initializer.
// docs/adr/0116-explicit-go-local-initialization.md:47.
func RunLocalInit(parent context.Context, args []string, source string, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) (status int) {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	option, err := parseLocalInit(args, source)
	if err != nil {
		fmt.Fprintln(stderr, "factory local-source:", err)
		return 2
	}
	if option.help {
		fmt.Fprintln(stdout, "Usage: factory init [TARGET] --local [--source DIR] [initializer options]\n\nBuild clean committed source using the provisioned Go toolchain; no publisher authentication is claimed.")
		return 0
	}
	physical, revision, err := inspectLocalSource(ctx, option.source, environment)
	if err != nil {
		fmt.Fprintln(stderr, "factory local-source:", err)
		return 1
	}
	image, err := acquireLocal(ctx, physical, revision, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "factory local-source:", err)
		return 1
	}
	defer func() { status = image.finish(stderr, status) }()
	digest, err := digestFile(ctx, image.raw, 256<<20)
	if err != nil || digest != image.digest {
		fmt.Fprintln(stderr, "factory local-source: selected executable changed")
		return 1
	}
	if err := os.Chmod(image.raw, 0700); err != nil { // #nosec G302 -- Owner-only selected executable matched the locally qualified package digest.
		fmt.Fprintln(stderr, "factory local-source: cannot make selected executable runnable:", err)
		return 1
	}
	forwarded := []string{image.raw, localInitProtocol, "1", image.directory, physical, revision, image.image.ArchiveSHA256, "--"}
	forwarded = append(forwarded, option.args...)
	return inheritedHandoff(ctx, image, forwarded, localEnvironment(environment), stdin, stdout, stderr, "factory local-source")
}

func inspectLocalSource(ctx context.Context, source string, environment map[string]string) (string, string, error) {
	physical, err := filepath.EvalSymlinks(source)
	if err == nil {
		physical, err = filepath.Abs(physical)
	}
	if err != nil {
		return "", "", errors.New("cannot resolve the explicit source directory")
	}
	env := localEnvironment(environment)
	for key := range env {
		if strings.HasPrefix(key, "GIT_") {
			delete(env, key)
		}
	}
	env["GIT_CONFIG_NOSYSTEM"], env["GIT_CONFIG_GLOBAL"] = "1", os.DevNull
	git := func(args ...string) (string, error) {
		child, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		command := []string{"git", "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false"}
		value, err := native.ExecuteCommandBounded(child, physical, append(command, args...), env, 1<<20, 64<<10)
		if err != nil || value.ExitCode == nil || *value.ExitCode != 0 || !value.ExitConfirmed || value.OwnershipUnconfirmed {
			return "", errors.Join(err, errors.New("cannot inspect committed source Git identity"))
		}
		return string(value.Stdout), nil
	}
	dirty, err := git("status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return "", "", err
	}
	if dirty != "" {
		return "", "", errors.New("source has dirty or uncommitted changes; select a clean committed checkout")
	}
	revision, err := git("rev-parse", "--verify", "HEAD^{commit}")
	revision = strings.TrimSpace(revision)
	if err != nil || !revisionPattern.MatchString(revision) {
		return "", "", errors.Join(err, errors.New("source requires one full committed HEAD identity"))
	}
	return physical, revision, nil
}

func acquireLocal(ctx context.Context, source, revision string, diagnostics io.Writer) (result *acquired, returned error) {
	a, err := newAcquired()
	if err != nil {
		return nil, err
	}
	defer func() {
		if result == nil {
			returned = a.failed(returned)
		}
	}()
	target, version := runtime.GOOS+"/"+runtime.GOARCH, "local-"+revision
	if _, err := fmt.Fprintf(diagnostics, "factory local-source: source=%s revision=%s version=%s target=%s (committed source; no publisher authentication)\n", source, revision, version, target); err != nil {
		return nil, err
	}
	built, err := packaging.Build(ctx, packaging.Options{Source: source, Revision: revision, Target: target, Output: filepath.Join(a.directory, "package"), Version: version, Installation: true})
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

func localEnvironment(environment map[string]string) map[string]string {
	result := withoutCache(environment)
	delete(result, "FACTORY_BRIDGE_PROTOCOL")
	return result
}
