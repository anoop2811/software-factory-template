// Package checkcmd orchestrates retained adopter validation boundaries.
package checkcmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/config"
	"github.com/anoop2811/software-factory-template/internal/native"
)

const discoveryLimit = 1 << 20

// Run retains the pre-push stage order, failure accounting and caller streams.
// docs/adr/0099-native-factory-orchestration.md:75.
func Run(ctx context.Context, _ []string, _ string, environment map[string]string, stdin, stdout, stderr *os.File) int {
	r := &runner{environment: environment, streams: native.InheritedStreams{Stdin: stdin, Stdout: stdout, Stderr: stderr}}
	if stdin == nil || stdout == nil || stderr == nil {
		return 1
	}
	if err := r.run(ctx); err != nil {
		_, _ = fmt.Fprintf(stderr, "factory check: %v\n", err)
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
	streams     native.InheritedStreams
	failures    int
	err         error
}

func (r *runner) print(format string, values ...any) {
	if r.err == nil {
		if _, err := fmt.Fprintf(r.streams.Stdout, format, values...); err != nil {
			r.err = fmt.Errorf("write check output: %w", err)
		}
	}
}

func (r *runner) run(ctx context.Context) error {
	var err error
	r.root, err = repositoryRoot(ctx, r.environment)
	if err != nil {
		return err
	}
	r.print("========================================\n  PRE-PUSH CHECK\n========================================\n\n")
	r.print("[1/8] direct-main-push-block (feature branches + PRs only)\n")
	if !r.execute(ctx, []string{"./scripts/hooks/direct-main-push-block.sh"}, false) {
		if r.err == nil {
			r.print("  FAIL\n")
			r.failures++
		}
		return r.err
	}
	r.print("  PASS\n\n")
	for _, stage := range []struct {
		heading string
		argv    []string
	}{
		{"[2/8] make check (lint, test, security, citations, hooks)", []string{"make", "check"}},
		{"[3/8] make check-drift (Claude + Codex adapter drift)", []string{"make", "check-drift"}},
	} {
		r.stage(ctx, stage.heading, stage.argv)
		if r.err != nil {
			return r.err
		}
	}
	r.print("[4/8] commit-message-lint (full unpushed range)\n")
	origin, err := r.git(ctx, "rev-parse", "origin/main")
	if err != nil {
		return err
	}
	if origin == "" {
		r.print("  SKIP (no origin/main found)\n")
	} else {
		commits, err := r.git(ctx, "rev-list", origin+"..HEAD")
		if err != nil {
			return err
		}
		shas := strings.Fields(commits)
		failed := false
		for _, sha := range shas {
			if !r.execute(ctx, []string{"./scripts/hooks/commit-message-lint.sh", sha}, true) {
				failed = true
			}
			if r.err != nil {
				return r.err
			}
		}
		switch {
		case failed:
			r.print("  FAIL (%d commits checked, at least one violation)\n", len(shas))
			r.failures++
		case len(shas) == 0:
			r.print("  SKIP (no unpushed commits)\n")
		default:
			r.print("  PASS (%d commits checked)\n", len(shas))
		}
	}
	r.print("\n")
	// Each range discovery is observed independently, as in the retained gate.
	// Failed Git stdout is never a revision: docs/adr/0099-native-factory-orchestration.md:178.
	r.print("[5/8] diff-aware-check (dispatch checks for changed files)\n")
	origin, err = r.git(ctx, "rev-parse", "origin/main")
	if err != nil {
		return err
	}
	if origin == "" {
		origin = "HEAD"
	}
	r.result(r.execute(ctx, []string{"./scripts/hooks/diff-aware-check.sh", origin, "HEAD"}, true))
	if r.err != nil {
		return r.err
	}
	r.print("[6/8] decision-log-gate (governance commits reference a Decision)\n")
	origin, err = r.git(ctx, "rev-parse", "origin/main")
	if err != nil {
		return err
	}
	if origin == "" {
		r.print("  SKIP (no origin/main found)\n\n")
	} else {
		r.result(r.execute(ctx, []string{"./scripts/hooks/decision-log-gate.sh", origin, "HEAD"}, true))
	}
	if r.err != nil {
		return r.err
	}
	r.stage(ctx, "[7/8] pending-lessons-push-block (no unaddressed loop-close reminders)", []string{"./scripts/hooks/pending-lessons-push-block.sh"})
	if r.err != nil {
		return r.err
	}
	r.print("[8/8] local hooks (repo-local gates registered in factory.yaml)\n")
	if err := r.localHooks(ctx); err != nil {
		return err
	}
	r.print("\n========================================\n")
	if r.failures > 0 {
		r.print("  PRE-PUSH: %d CHECK(S) FAILED — PUSH BLOCKED\n", r.failures)
	} else {
		r.print("  PRE-PUSH: ALL CHECKS PASSED — PUSH ALLOWED\n")
	}
	r.print("========================================\n")
	return errors.Join(ctx.Err(), r.err)
}

func (r *runner) stage(ctx context.Context, heading string, argv []string) {
	r.print("%s\n", heading)
	r.result(r.execute(ctx, argv, true))
}

func (r *runner) result(passed bool) {
	if r.err != nil {
		return
	}
	if passed {
		r.print("  PASS\n\n")
	} else {
		r.print("  FAIL\n\n")
		r.failures++
	}
}

func (r *runner) execute(ctx context.Context, argv []string, combined bool) bool {
	if r.err != nil {
		return false
	}
	streams := r.streams
	if combined {
		streams.Stderr = streams.Stdout
	}
	result, err := native.ExecuteInherited(ctx, r.root, argv, r.environment, streams, 0)
	if unsafe(ctx, result) {
		r.err = errors.Join(ctx.Err(), err, errors.New("validation did not complete safely"))
		return false
	}
	if err != nil {
		if _, outputErr := fmt.Fprintf(streams.Stderr, "factory check: %s: %v\n", argv[0], err); outputErr != nil {
			r.err = fmt.Errorf("write check diagnostic: %w", outputErr)
		}
	}
	return err == nil && succeeded(result)
}

func unsafe(ctx context.Context, result native.Execution) bool {
	return ctx.Err() != nil || result.OwnershipUnconfirmed || result.Outcome == "timeout" || result.Outcome == "interrupted" || result.Outcome == "output_limit" || (result.ProcessPID != 0 && !result.ExitConfirmed)
}

func succeeded(result native.Execution) bool {
	return result.Outcome == "completed" && result.ExitConfirmed && result.ExitCode != nil && *result.ExitCode == 0
}

func repositoryRoot(ctx context.Context, environment map[string]string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("read working directory: %w", err)
	}
	root := environment["REPO_ROOT"]
	if root == "" {
		result, err := native.ExecuteCommandBounded(ctx, cwd, []string{"git", "rev-parse", "--show-toplevel"}, environment, discoveryLimit, discoveryLimit)
		if unsafe(ctx, result.Execution) {
			return "", errors.Join(ctx.Err(), err, errors.New("repository discovery did not complete safely"))
		}
		if err == nil && succeeded(result.Execution) {
			root = strings.TrimRight(string(result.Stdout), "\n")
		}
		if root == "" {
			root = "."
		}
	}
	if !filepath.IsAbs(root) {
		root = filepath.Join(cwd, root)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("open repository root: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("repository root is not a directory")
	}
	return root, nil
}

func (r *runner) git(ctx context.Context, args ...string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	result, err := native.ExecuteCommandBounded(ctx, r.root, append([]string{"git"}, args...), r.environment, discoveryLimit, discoveryLimit)
	if unsafe(ctx, result.Execution) {
		return "", errors.Join(ctx.Err(), err, errors.New("git discovery did not complete safely"))
	}
	if !succeeded(result.Execution) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(result.Stdout), "\n"), nil
}

func (r *runner) localHooks(ctx context.Context) error {
	path := r.environment["FACTORY_CONFIG"]
	if path == "" {
		// The check selected REPO_ROOT before running any stage. Local hooks
		// must use that project's configuration, including nested projects.
		// docs/adr/0099-native-factory-orchestration.md:84.
		path = filepath.Join(r.root, "factory.yaml")
	} else if !filepath.IsAbs(path) {
		path = filepath.Join(r.root, path)
	}
	raw, err := config.Get(ctx, path, "local_hooks", "")
	if err != nil {
		return err
	}
	raw = strings.TrimRight(raw, "\n")
	if raw == "" {
		r.print("  SKIP (none registered)\n")
		return r.err
	}
	// This host-shell boundary only expands operands; Go owns normalization.
	// docs/adr/0099-native-factory-orchestration.md:84.
	const expand = `case "$1" in *,*) IFS=','; set -- $1 ;; *) set -- $1 ;; esac
if [ "$#" -gt 0 ]; then printf '%s\0' "$@"; fi`
	result, err := native.ExecuteCommandBounded(ctx, r.root, []string{"bash", "-c", expand, "factory-local-hooks", raw}, r.environment, discoveryLimit, discoveryLimit)
	if err != nil || !succeeded(result.Execution) {
		return errors.Join(ctx.Err(), err, errors.New("expand configured local hooks"))
	}
	tokens := strings.Split(strings.TrimSuffix(string(result.Stdout), "\x00"), "\x00")
	mode := "words"
	if strings.Contains(raw, ",") {
		mode = "commas"
	}
	entries, err := config.Hooks(ctx, mode, tokens)
	if err != nil {
		return err
	}
	if strings.Trim(entries, " \n") == "" {
		r.print("  SKIP (none registered)\n")
		return r.err
	}
	for _, entry := range strings.Split(entries, "\n") {
		argv := strings.FieldsFunc(entry, func(r rune) bool { return r == ' ' || r == '\t' })
		if len(argv) == 0 {
			continue
		}
		if err := r.localHook(ctx, argv); err != nil {
			return err
		}
	}
	return r.err
}

func (r *runner) localHook(ctx context.Context, argv []string) error {
	path := argv[0]
	fullPath := path
	if !filepath.IsAbs(path) {
		fullPath = filepath.Join(r.root, path)
	}
	if _, err := exec.LookPath(fullPath); err != nil {
		r.print("  FAIL %s is registered but not executable\n", path)
		r.failures++
		return r.err
	}
	output, err := os.CreateTemp("", "factory-local-hook-")
	if err != nil {
		return fmt.Errorf("capture local hook output: %w", err)
	}
	defer output.Close()
	defer os.Remove(output.Name())
	command := append([]string{"./" + path}, argv[1:]...)
	result, runErr := native.ExecuteInherited(ctx, r.root, command, r.environment, native.InheritedStreams{Stdin: r.streams.Stdin, Stdout: output, Stderr: output}, 0)
	if unsafe(ctx, result) {
		return errors.Join(ctx.Err(), runErr, errors.New("local hook did not complete safely"))
	}
	if _, err := output.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("read local hook output: %w", err)
	}
	reader := bufio.NewReader(output)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, readErr := reader.ReadString('\n')
		if line != "" {
			r.print("    %s\n", strings.TrimSuffix(line, "\n"))
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read local hook output: %w", readErr)
		}
	}
	if runErr == nil && succeeded(result) {
		r.print("  PASS %s\n", strings.Join(argv, " "))
	} else {
		r.print("  FAIL %s\n", strings.Join(argv, " "))
		r.failures++
	}
	return r.err
}
