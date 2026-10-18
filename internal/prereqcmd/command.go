// Package prereqcmd checks the retained factory tool prerequisites.
package prereqcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/config"
	"github.com/anoop2811/software-factory-template/internal/native"
)

const versionOutputLimit = 1 << 20

// Run checks the selected product tools without an interpreter dependency.
// docs/adr/0099-native-factory-orchestration.md:90.
func Run(ctx context.Context, _ []string, _ string, environment map[string]string, stdout, stderr io.Writer) int {
	if stdout == nil || stderr == nil {
		return 1
	}
	r := &runner{environment: environment, stdout: stdout}
	if err := r.run(ctx); err != nil {
		_, _ = fmt.Fprintf(stderr, "factory prereq: %v\n", err)
		return 1
	}
	if r.failures != 0 {
		return 1
	}
	return 0
}

type runner struct {
	root        string
	environment map[string]string
	stdout      io.Writer
	failures    int
	err         error
}

func (r *runner) print(format string, values ...any) {
	if r.err == nil {
		if _, err := fmt.Fprintf(r.stdout, format, values...); err != nil {
			r.err = fmt.Errorf("write prerequisite output: %w", err)
		}
	}
}

func (r *runner) run(ctx context.Context) error {
	var err error
	r.root, err = os.Getwd()
	if err != nil {
		return fmt.Errorf("read working directory: %w", err)
	}
	path, err := r.configPath(ctx)
	if err != nil {
		return err
	}
	minimum, err := config.Get(ctx, path, "go_min_version", "")
	if err != nil {
		return err
	}
	r.check(ctx, "git", "git")
	if r.err != nil {
		return r.err
	}
	if minimum == "" {
		r.print("  skip: no go_min_version in factory.yaml (no Go pack installed)\n")
	} else {
		r.checkGo(ctx, minimum)
	}
	if r.err != nil {
		return r.err
	}
	harness := r.environment["FACTORY_HARNESS"]
	if harness == "" {
		harness = "opencode"
	}
	switch harness {
	case "opencode":
		r.check(ctx, "opencode", harness)
	case "claude":
		r.check(ctx, "Claude Code", harness)
	case "codex":
		r.check(ctx, "Codex", harness)
	default:
		r.print("PREREQ FAIL: unsupported FACTORY_HARNESS=%s\n", harness)
		r.failures++
	}
	for _, name := range []string{"golangci-lint", "jq"} {
		if r.err != nil {
			return r.err
		}
		r.check(ctx, name, name)
	}
	// Golden evaluation is native; jq still serves retained JSON gates.
	// docs/adr/0099-native-factory-orchestration.md:93.
	r.print("\n")
	if r.failures > 0 {
		r.print("prereq-check: %d tool(s) missing or too old\nInstall missing tools before running setup.sh.\n", r.failures)
	} else {
		r.print("prereq-check: all tools present\n")
	}
	return errors.Join(ctx.Err(), r.err)
}

func (r *runner) configPath(ctx context.Context) (string, error) {
	if explicit := r.environment["FACTORY_CONFIG"]; explicit != "" {
		return explicit, nil
	}
	result, err := native.ExecuteCommandBounded(ctx, r.root, []string{"git", "rev-parse", "--show-toplevel"}, r.environment, versionOutputLimit, versionOutputLimit)
	if unsafe(ctx, result.Execution) {
		return "", errors.Join(ctx.Err(), err, errors.New("configuration discovery did not complete safely"))
	}
	root := "."
	if err == nil && result.Outcome == "completed" && result.ExitCode != nil && *result.ExitCode == 0 {
		root = strings.TrimRight(string(result.Stdout), "\n")
	}
	return config.ResolvePath("", root), nil
}

func (r *runner) check(ctx context.Context, display, command string) {
	version, found := r.version(ctx, command, "--version")
	if r.err != nil {
		return
	}
	if !found {
		r.print("PREREQ FAIL: %s not found (command: %s)\n", display, command)
		r.failures++
		return
	}
	r.print("prereq: OK %s (%s)\n", display, version)
}

func (r *runner) version(ctx context.Context, command, operand string) (string, bool) {
	if err := ctx.Err(); err != nil {
		r.err = err
		return "", false
	}
	binary := ""
	searchPath, present := r.environment["PATH"]
	if !present {
		searchPath = "/bin:/usr/bin"
	}
	for _, directory := range strings.Split(searchPath, string(os.PathListSeparator)) {
		if directory == "" {
			directory = "."
		}
		candidate := filepath.Join(directory, command)
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(r.root, candidate)
		}
		if path, err := exec.LookPath(candidate); err == nil {
			binary = path
			break
		}
	}
	if binary == "" {
		return "", false
	}
	result, err := native.ExecuteCommandCombined(ctx, r.root, []string{binary, operand}, r.environment, 30*time.Second)
	if unsafe(ctx, result.Execution) {
		r.err = errors.Join(ctx.Err(), err, errors.New("prerequisite probe did not complete safely"))
		return "", false
	}
	line, _, _ := strings.Cut(string(result.Stdout), "\n")
	return line, true
}

var goVersionPattern = regexp.MustCompile(`go([0-9]+\.[0-9]+\.[0-9]+)`)

func (r *runner) checkGo(ctx context.Context, minimum string) {
	output, found := r.version(ctx, "go", "version")
	if r.err != nil {
		return
	}
	if !found {
		r.print("PREREQ FAIL: Go not found (command: go)\n")
		r.failures++
		return
	}
	version := "0.0.0"
	if match := goVersionPattern.FindStringSubmatch(output); len(match) == 2 {
		version = match[1]
	}
	current, currentErr := versionParts(version)
	required, requiredErr := versionParts(minimum)
	if currentErr != nil || requiredErr != nil {
		r.print("PREREQ FAIL: Go version %s cannot be compared with minimum security patch %s\n", version, minimum)
		r.failures++
		return
	}
	for index := range current {
		if current[index] < required[index] {
			r.print("PREREQ FAIL: Go version %s is below minimum security patch %s\n", version, minimum)
			r.failures++
			return
		}
		if current[index] > required[index] {
			break
		}
	}
	r.print("prereq: OK Go (%s)\n", output)
}

func versionParts(value string) ([3]uint64, error) {
	var result [3]uint64
	parts := strings.Split(value, ".")
	if len(parts) != len(result) {
		return result, errors.New("version must have three numeric components")
	}
	for index, part := range parts {
		parsed, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return result, fmt.Errorf("parse version: %w", err)
		}
		result[index] = parsed
	}
	return result, nil
}

func unsafe(ctx context.Context, result native.Execution) bool {
	return ctx.Err() != nil || result.OwnershipUnconfirmed || result.Outcome == "timeout" || result.Outcome == "interrupted" || result.Outcome == "output_limit" || (result.ProcessPID != 0 && !result.ExitConfirmed)
}
