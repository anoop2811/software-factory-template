// Package packaging builds inert developer bundles from committed runtime source.
package packaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/artifact"
	"github.com/anoop2811/software-factory-template/internal/installationimage"
	"github.com/anoop2811/software-factory-template/internal/native"
)

// ErrInvalidRequest distinguishes malformed operands from operational failures.
var ErrInvalidRequest = errors.New("invalid packaging request")

var (
	revisionPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)
	versionPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)
)

// Options identifies one source, toolchain and output; no fallback is permitted.
type Options struct {
	Source, Revision, Target, Output, Version, Compiler string
	Installation                                        bool
}

// Result is emitted only after every bundle file has been published.
type Result struct {
	Version  string `json:"version"`
	Revision string `json:"revision"`
	Target   string `json:"target"`
	Archive  string `json:"archive"`
	SHA256   string `json:"sha256"`
}

// Build follows docs/adr/0060-runtime-source-bundles.md:13. It never executes
// the built candidate and removes only its private temporary workspace.
func Build(ctx context.Context, options Options) (result Result, err error) {
	if err := options.validate(); err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	output, err := resolveOutput(options.Output)
	if err != nil {
		return Result{}, fmt.Errorf("resolve output: %w", err)
	}
	if _, err = os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			err = os.ErrExist
		}
		return Result{}, fmt.Errorf("output must not exist: %w", err)
	}
	workspace, err := os.MkdirTemp("", "factory-runtime-package-")
	if err != nil {
		return Result{}, err
	}
	defer func() {
		var ownership *native.OwnershipError
		if errors.As(err, &ownership) {
			result = Result{}
			err = fmt.Errorf("preserve unresolved packaging workspace %s: %w", workspace, err)
			return
		}
		cleanupErr := os.RemoveAll(workspace)
		if options.Installation {
			err = errors.Join(err, cleanupErr)
			if err != nil {
				result = Result{}
			}
		}
	}()
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		closeErr := root.Close()
		if options.Installation {
			err = errors.Join(err, closeErr)
		}
	}()
	if err = root.Mkdir("source", 0700); err != nil {
		return Result{}, err
	}
	var image installationimage.Manifest
	if options.Installation {
		if _, err = isolatedRepository(ctx, options, root); err != nil {
			return Result{}, err
		}
		if image, err = collectInstallation(ctx, root, options); err != nil {
			return Result{}, err
		}
		if err = prepareInstallationSource(ctx, root, options, image); err != nil {
			return Result{}, err
		}
	} else if err = extractSource(ctx, options, root); err != nil {
		return Result{}, err
	}
	if err = compile(ctx, options, workspace); err != nil {
		return Result{}, err
	}
	digest, names, err := createBundle(ctx, root, options, image)
	if err != nil {
		return Result{}, err
	}
	if err = publish(ctx, root, output, names); err != nil {
		return Result{}, fmt.Errorf("publish bundle: %w", err)
	}
	return Result{options.Version, options.Revision, options.Target, filepath.Join(output, archiveName), digest}, nil
}

// Preserve interior symlink/.. semantics until the actual parent is resolved.
// Only suffixes are trimmed so Lstat can still reject a final output symlink.
func resolveOutput(output string) (string, error) {
	separator := string(filepath.Separator)
	for {
		output = strings.TrimRight(output, separator)
		if output == "" {
			output = separator
			break
		}
		if !strings.HasSuffix(output, separator+".") {
			break
		}
		output = strings.TrimSuffix(output, separator+".")
	}
	if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			err = os.ErrExist
		}
		return "", fmt.Errorf("output must not exist: %w", err)
	}
	index := strings.LastIndex(output, separator)
	parent, name := ".", output
	if index >= 0 {
		parent, name = output[:index], output[index+1:]
		if parent == "" {
			parent = separator
		}
	}
	parent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", err
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, name), nil
}

func (options *Options) validate() error {
	if options.Source == "" || options.Output == "" || !revisionPattern.MatchString(options.Revision) {
		return fmt.Errorf("%w: source, full lowercase commit SHA and output are required", ErrInvalidRequest)
	}
	switch options.Target {
	case "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64":
	default:
		return fmt.Errorf("%w: unsupported target %q", ErrInvalidRequest, options.Target)
	}
	if options.Version == "" {
		options.Version = "local-" + options.Revision
	}
	if !versionPattern.MatchString(options.Version) {
		return fmt.Errorf("%w: version must be a literal path component", ErrInvalidRequest)
	}
	switch strings.ToLower(options.Version) {
	case "latest", "current", "main", "head":
		return fmt.Errorf("%w: moving version aliases are not permitted", ErrInvalidRequest)
	}
	if strings.ContainsRune(options.Source+options.Output+options.Compiler, '\x00') {
		return fmt.Errorf("%w: paths must not contain NUL", ErrInvalidRequest)
	}
	return nil
}

func compile(ctx context.Context, options Options, workspace string) error {
	compiler := options.Compiler
	if compiler == "" {
		compiler = "go"
	}
	compiler, err := exec.LookPath(compiler)
	if err != nil {
		return fmt.Errorf("locate compiler: %w", err)
	}
	compiler, err = filepath.EvalSymlinks(compiler)
	if err != nil {
		return fmt.Errorf("resolve compiler symlinks: %w", err)
	}
	compiler, err = filepath.Abs(compiler)
	if err != nil {
		return fmt.Errorf("resolve compiler: %w", err)
	}
	env := compilerEnvironment(options.Target)
	version, err := compilerCommand(ctx, ".", []string{compiler, "version"}, env, 64<<10, true)
	output := version.Stdout
	if err != nil {
		return fmt.Errorf("compiler version: %w: %s", err, output)
	}
	fields := strings.Fields(string(output))
	if len(fields) != 4 || fields[0] != "go" || fields[1] != "version" || fields[2] != artifact.SourceGoVersion {
		return fmt.Errorf("compiler must be %s; received %q", artifact.SourceGoVersion, strings.TrimSpace(string(output)))
	}
	if err = checkModuleReplacements(ctx, compiler, filepath.Join(workspace, "source"), env); err != nil {
		return err
	}
	build, err := compilerCommand(ctx, filepath.Join(workspace, "source"), []string{compiler, "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-o", filepath.Join(workspace, "factory-runtime"), "./cmd/factory"}, env, 16<<20, true)
	if err != nil {
		return fmt.Errorf("compile runtime: %w: %s", err, build.Stdout)
	}
	return nil
}

func checkModuleReplacements(ctx context.Context, compiler, source string, env []string) error {
	// -json is read-only; versionless replacements name filesystem modules that
	// could inject uncommitted bytes. docs/adr/0060-runtime-source-bundles.md:29.
	command, err := compilerCommand(ctx, source, []string{compiler, "mod", "edit", "-json"}, env, 1<<20, false)
	if err != nil {
		return fmt.Errorf("inspect source module: %w: %s", err, command.Stderr)
	}
	var module struct {
		Replace []struct {
			New struct{ Path, Version string }
		}
	}
	if err = json.Unmarshal(command.Stdout, &module); err != nil {
		return fmt.Errorf("decode source module: %w", err)
	}
	for _, replacement := range module.Replace {
		if replacement.New.Version == "" {
			return fmt.Errorf("filesystem module replacement %q is forbidden in committed-source bundles", replacement.New.Path)
		}
	}
	return nil
}

// Compiler capture shares the native group's bounded cleanup and caller context.
// docs/adr/0110-local-compiler-process-ownership.md:19.
func compilerCommand(ctx context.Context, directory string, args, env []string, limit int, combined bool) (native.CommandResult, error) {
	result, err := native.ExecuteCommandWithOptions(ctx, directory, args, environmentMap(env), native.CommandOptions{StdoutLimit: limit, StderrLimit: 64 << 10, MergeStderr: combined})
	if result.ExitCode != nil && *result.ExitCode > 0 {
		err = errors.Join(fmt.Errorf("exit status %d", *result.ExitCode), err)
	}
	if err == nil && (result.Outcome != "completed" || !result.ExitConfirmed || result.OwnershipUnconfirmed || result.ExitCode == nil || *result.ExitCode != 0) {
		err = errors.New("compiler command did not establish completed process ownership")
	}
	return result, errors.Join(ctx.Err(), err)
}

func environmentMap(env []string) map[string]string {
	environment := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		environment[key] = value
	}
	return environment
}

func compilerEnvironment(target string) []string {
	env := make([]string, 0, len(os.Environ())+12)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, "GO") && !strings.HasPrefix(key, "CGO") {
			env = append(env, entry)
		}
	}
	platform := strings.Split(target, "/")
	return append(env, "GOENV=off", "GOTOOLCHAIN=local", "GOWORK=off", "GOFLAGS=", "GOEXPERIMENT=", "GOPROXY=off", "GOSUMDB=off", "CGO_ENABLED=0", "GOOS="+platform[0], "GOARCH="+platform[1], "GOAMD64=v1", "GOARM64=v8.0")
}
