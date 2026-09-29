// Package doctorcmd reports local factory health without rewriting adopter files.
package doctorcmd

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
	"time"

	"github.com/anoop2811/software-factory-template/internal/config"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/output"
)

type report struct {
	ctx                context.Context
	out                io.Writer
	err                error
	failures, warnings int
	root               string
	environment        map[string]string
}

func (r *report) print(format string, args ...any) {
	if r.err == nil {
		r.err = output.WriteEvent(r.ctx, r.out, []byte(fmt.Sprintf(format, args...)))
	}
}
func (r *report) line(kind, message string) {
	r.print("  %-9s %s\n", kind, message)
	if kind == "[FAIL]" {
		r.failures++
	}
	if kind == "[warn]" || kind == "[STALE]" {
		r.warnings++
	}
}
func (r *report) path(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return r.root + "/" + path
}
func regular(path string) bool    { s, e := os.Stat(path); return e == nil && s.Mode().IsRegular() }
func directory(path string) bool  { s, e := os.Stat(path); return e == nil && s.IsDir() }
func executable(path string) bool { return regular(path) && syscall.Access(path, 1) == nil }

// Run retains ignored argv and the explicit subprocess boundaries.
// docs/adr/0086-go-native-doctor.md:18.
func Run(parent context.Context, environment map[string]string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	broken := make(chan os.Signal, 1)
	signal.Notify(broken, syscall.SIGPIPE)
	defer signal.Stop(broken)
	r := &report{ctx: ctx, out: stdout, environment: environment}
	err := r.run(ctx)
	if ctx.Err() != nil {
		err = fmt.Errorf("doctor canceled: %w", ctx.Err())
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "factory doctor: "+err.Error())
		return 1
	}
	if r.failures > 0 {
		return 1
	}
	return 0
}
func (r *report) run(ctx context.Context) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	r.root = cwd
	configRoot := "."
	result, err := native.ExecuteCommand(ctx, cwd, []string{"git", "rev-parse", "--show-toplevel"}, r.environment, 5*time.Second)
	if unsafeResult(result) || ctx.Err() != nil {
		return errors.New("git discovery did not complete safely")
	}
	if err == nil && succeeded(result) {
		r.root = strings.TrimRight(string(result.Stdout), "\n")
		configRoot = r.root
		if r.root == "" || strings.ContainsAny(r.root, "\x00\r\n") {
			return errors.New("invalid Git root")
		}
	}
	cfg := config.ResolvePath(r.environment["FACTORY_CONFIG"], configRoot)
	r.print("factory doctor\n  repo:   %s\n  config: %s\n", r.root, cfg)
	data, err := readFile(ctx, r.path(cfg))
	if errors.Is(err, os.ErrNotExist) {
		r.print("\n")
		r.line("[FAIL]", "factory.yaml not found — run 'factory init' first")
		r.print("\nfactory doctor: 1 problem\n")
		return r.err
	}
	if err != nil {
		return fmt.Errorf("cannot inspect configuration: %w", err)
	}
	values := map[string]string{}
	for _, key := range []string{"test_file_patterns", "citation_prefix", "check_command", "protected_paths", "decision_log", "language_packs", "docs_root", "wiki_root", "wiki_staleness"} {
		fallback := ""
		if key == "wiki_root" {
			fallback = "wiki"
		}
		values[key], err = config.GetBytes(ctx, data, key, fallback)
		if err != nil {
			return err
		}
	}
	r.gates(ctx, values)
	if r.err != nil {
		return r.err
	}
	if err := r.review(ctx, r.path(cfg), data); err != nil {
		return err
	}
	if regular(r.path("factory.config")) {
		r.line("[warn]", "factory.config is still present — two config files, one job (Decision 41)")
		r.line("", "  it still works; settings there are read when factory.yaml omits them")
		r.line("", "  move it: ./factory migrate-config   (--dry-run to preview)")
	}
	r.print("\nIntegrity\n")
	missing := 0
	for _, path := range coreHooks {
		if !regular(r.path(path)) {
			r.line("[FAIL]", path+" is missing")
			missing++
		} else if !executable(r.path(path)) {
			r.line("[FAIL]", path+" is not executable")
			missing++
		}
	}
	if missing == 0 {
		r.line("[ ok ]", "all core hook scripts present and executable")
	}
	if err := r.hooksPath(ctx); err != nil {
		return err
	}
	if err := r.adapters(ctx, r.path(cfg), data); err != nil {
		return err
	}
	r.codeowners(ctx, values["protected_paths"])
	if r.err != nil {
		return r.err
	}
	if err := r.proof(ctx); err != nil {
		return err
	}
	r.print("\n")
	switch {
	case r.failures > 0:
		r.print("factory doctor: %d problem(s), %d warning(s) — the factory is not fully sound\n", r.failures, r.warnings)
	case r.warnings > 0:
		r.print("factory doctor: healthy, %d warning(s) to review (inert gates are a choice, not a fault)\n", r.warnings)
	default:
		r.print("factory doctor: healthy — every armed gate is live and proven\n")
	}
	return r.err
}

var coreHooks = []string{"scripts/lib/config.sh", "scripts/selftest/run.sh", "scripts/hooks/test-edit-denial.sh", "scripts/hooks/commit-message-lint.sh", "scripts/hooks/decision-log-gate.sh", "scripts/hooks/diff-aware-check.sh", "scripts/hooks/hook-existence-check.sh", "scripts/hooks/shared-script-enforcement.sh", "scripts/hooks/direct-main-push-block.sh", "scripts/hooks/pending-lessons-push-block.sh", "scripts/citation-lint.sh"}

func unsafeResult(r native.CommandResult) bool {
	return r.OwnershipUnconfirmed || r.Outcome == "timeout" || r.Outcome == "interrupted" || r.Outcome == "output_limit"
}
func succeeded(r native.CommandResult) bool {
	return !unsafeResult(r) && r.ExitConfirmed && r.ExitCode != nil && *r.ExitCode == 0
}
