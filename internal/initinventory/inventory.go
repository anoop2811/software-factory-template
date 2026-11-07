// Package initinventory declares the base source inputs shared by initialization
// and its copy gate. docs/adr/0125-native-initializer-copy-inventory-gate.md:28.
package initinventory

// Readme is copied only when the target has no README.
const Readme = "README.md"

// Group selects immediate children by suffix, as the initializer has always done.
type Group struct {
	Directory string
	Suffix    string
}

// Retired Python controllers are not current source inputs. docs/adr/0140-go-source-loop-and-python-retirement.md:33.
var required = []string{
	"scripts/lib/config.sh", "scripts/lib/roles.sh", "scripts/lib/events.sh", "scripts/lib/hookspath.sh", "scripts/lib/color.sh", "scripts/lib/timing.sh", "scripts/lib/budget-config.sh",
	"scripts/selftest/run.sh", "scripts/pre-push-check.sh", "scripts/factory-doctor.sh", "scripts/factory-upgrade.sh", "scripts/factory-report.sh", "scripts/factory-budget.sh", "scripts/factory-loop.sh", "scripts/factory-metrics.sh", "templates/metrics.html", "scripts/factory-review-lane.sh", "scripts/factory-migrate-config.sh", "scripts/adversarial-review.sh", "packs/review-lane/review-pr.yml", ".githooks/pre-push", "scripts/prereq-check.sh",
	".opencode/plugin/factory-hooks.ts", ".opencode/package.json", ".opencode/.gitignore", ".codex/config.toml", "opencode.json", "AGENTS.md", "Makefile", "factory", ".gitignore", ".github/CODEOWNERS", ".github/workflows/ci.yml", "docs/FACTORY_RULES.md", "docs/BUDGETS.md", "docs/LOOPS.md", "memory/lessons/001-verification-contract.md",
}

var optional = []string{
	"wiki/README.md", "scripts/golden-task-eval.sh", "eval/README.md", "eval/runners/mock.sh", "eval/runners/example-harness.sh", "eval/runners/claude.sh", "eval/runners/codex.sh", "eval/runners/opencode.sh", "eval/golden-tasks/reference-answer/task.md", "eval/golden-tasks/reference-answer/verify.sh",
	"workflows/review-diamond.md", "workflows/eval-fanout.md", "workflows/README.md", "scripts/sync-opencode.sh", "scripts/sync-claude.sh", "scripts/sync-codex.sh", "scripts/harness-structural-eval.sh", "scripts/citation-lint.sh", "specs/TEMPLATE.md",
}

var groups = []Group{{"scripts/hooks", ".sh"}, {".opencode/agent", ".md"}, {".codex/agents", ".toml"}}

// Required returns a copy of the unconditional base input paths.
func Required() []string { return append([]string(nil), required...) }

// Optional returns a copy of the base paths copied when present.
func Optional() []string { return append([]string(nil), optional...) }

// Groups returns a copy of the required nonempty base copy groups.
func Groups() []Group { return append([]Group(nil), groups...) }
