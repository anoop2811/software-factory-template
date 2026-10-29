package initcmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// This entrypoint exists only in the cached acceptance test executable. Normal
// package tests retain the real production declarations. The public CLI never
// contains this file, this guard or the historical asset declarations below.
// per docs/adr/0108-default-go-cutover.md:192
func init() {
	if len(os.Args) < 2 || os.Args[1] != "init" || filepath.Base(os.Args[0]) != "factory-init-component" {
		return
	}
	requiredFiles = legacyComponentRequired
	optionalFiles = legacyComponentOptional
	environment := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		environment[key] = value
	}
	executable, err := os.Executable()
	if err != nil {
		os.Exit(2)
	}
	os.Exit(Run(context.Background(), os.Args[2:], filepath.Dir(executable), environment, os.Stdin, os.Stdout, os.Stderr))
}

// Historical declarative fixture data, not current delivery authority. Frozen
// from immutable 95486ecdd3868a319a0e6362ee99cafd8f154200:internal/initcmd/plan.go.
// The source programs themselves remain pinned Git inputs in private test trees.
// per docs/adr/0108-default-go-cutover.md:195
var legacyComponentRequired = []string{
	"scripts/lib/config.sh", "scripts/lib/roles.sh", "scripts/lib/events.sh", "scripts/lib/hookspath.sh", "scripts/lib/color.sh", "scripts/lib/timing.sh", "scripts/lib/budget.py", "scripts/lib/budget_adapters.py", "scripts/lib/runtime_transition.py", "scripts/lib/budget-config.sh", "scripts/lib/loop.py",
	"scripts/selftest/run.sh", "scripts/pre-push-check.sh", "scripts/factory-doctor.sh", "scripts/factory-upgrade.sh", "scripts/factory-report.sh", "scripts/factory-budget.sh", "scripts/factory-loop.sh", "scripts/factory-metrics.sh", "templates/metrics.html", "scripts/factory-review-lane.sh", "scripts/factory-migrate-config.sh", "scripts/adversarial-review.sh", "packs/review-lane/review-pr.yml", ".githooks/pre-push", "scripts/prereq-check.sh",
	".opencode/plugin/factory-hooks.ts", ".opencode/package.json", ".opencode/.gitignore", ".codex/config.toml", "opencode.json", "AGENTS.md", "Makefile", "factory", ".gitignore", ".github/CODEOWNERS", ".github/workflows/ci.yml", "docs/FACTORY_RULES.md", "docs/BUDGETS.md", "docs/LOOPS.md", "memory/lessons/001-verification-contract.md",
}

var legacyComponentOptional = []string{
	"wiki/README.md", "scripts/golden-task-eval.sh", "eval/README.md", "eval/runners/mock.sh", "eval/runners/example-harness.sh", "eval/runners/claude.sh", "eval/runners/codex.sh", "eval/runners/opencode.sh", "eval/golden-tasks/reference-answer/task.md", "eval/golden-tasks/reference-answer/verify.sh",
	"workflows/review-diamond.md", "workflows/eval-fanout.md", "workflows/README.md", "scripts/sync-opencode.sh", "scripts/sync-claude.sh", "scripts/sync-codex.sh", "scripts/harness-structural-eval.sh", "scripts/citation-lint.sh", "specs/TEMPLATE.md",
}
