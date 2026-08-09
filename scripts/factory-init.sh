#!/bin/bash
set -euo pipefail

# scripts/setup.sh
# Installs the software factory into an existing project (or a new empty dir).
#
# This script:
#   1. Asks for project-specific values (name, GitHub owner, protected path, etc.)
#   2. Copies all template files into the target project
#   3. Substitutes identity/model values in harness configs (opencode
#      cannot read factory.yaml); enforcement values go to factory.yaml
#   4. Makes all hook scripts executable
#   5. Initializes memory/, wiki/, specs/ directories
#   6. Runs prereq-check.sh
#
# Usage:
#   ./setup.sh /path/to/target-project    # install into existing project
#   ./setup.sh /path/to/new-project       # create dir and install
#   ./setup.sh .                           # install into current dir

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TEMPLATE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Parse args: an optional target dir (first non-flag) and one or more packs.
# Packs may be comma-separated (--pack go,typescript) or repeated (--pack go
# --pack typescript) — real apps are polyglot (a Go backend, a TS frontend).
TARGET_ARG="."
PACKS=""
while [ $# -gt 0 ]; do
  case "$1" in
    --pack) PACKS="$PACKS ${2:-}"; if [ $# -ge 2 ]; then shift 2; else shift; fi ;;
    --pack=*) PACKS="$PACKS ${1#*=}"; shift ;;
    *) TARGET_ARG="$1"; shift ;;
  esac
done
PACKS="$(printf '%s' "$PACKS" | tr ',' ' ')"

TARGET_DIR="$TARGET_ARG"
# Resolve to an absolute path, creating the directory if it does not exist.
# (Grouping matters: the old one-liner ran pwd twice on an existing dir and
# embedded a newline in the path.)
mkdir -p "$TARGET_DIR"
TARGET_DIR="$(cd "$TARGET_DIR" && pwd)"

# Prompt helper. Reads from the controlling terminal (/dev/tty) when available,
# so prompts work even when this script is reached through a pipe
# (curl ... | sh -s -- init), where stdin carries the installer, not the user.
# Falls back to stdin when there is no tty (CI, tests).
ask() {
  local __prompt="$1" __var="$2" __reply=""
  if [ -r /dev/tty ] && [ -t 1 ]; then
    read -rp "$__prompt" __reply < /dev/tty
  else
    read -rp "$__prompt" __reply || true
  fi
  printf -v "$__var" '%s' "$__reply"
}

# ── Detect the stack (informational; packs are installed explicitly) ──
DETECTED=""
[ -f "$TARGET_DIR/go.mod" ] && DETECTED="$DETECTED go"
[ -f "$TARGET_DIR/package.json" ] && DETECTED="$DETECTED typescript"
{ [ -f "$TARGET_DIR/pom.xml" ] || [ -f "$TARGET_DIR/build.gradle" ] || [ -f "$TARGET_DIR/build.gradle.kts" ]; } && DETECTED="$DETECTED java"

# Frameworks ride on a language pack — detect them to point at the right pack.
FRAMEWORKS=""
if [ -f "$TARGET_DIR/package.json" ]; then
  grep -q '"react"' "$TARGET_DIR/package.json" 2>/dev/null && FRAMEWORKS="$FRAMEWORKS react"
  grep -q '"vue"' "$TARGET_DIR/package.json" 2>/dev/null && FRAMEWORKS="$FRAMEWORKS vue"
fi
for __gf in pom.xml build.gradle build.gradle.kts; do
  [ -f "$TARGET_DIR/$__gf" ] && grep -qi 'spring-boot' "$TARGET_DIR/$__gf" 2>/dev/null && { FRAMEWORKS="$FRAMEWORKS spring-boot"; break; }
done

echo "=== Software Factory Template Setup ==="
echo "Template dir: $TEMPLATE_DIR"
echo "Target dir:   $TARGET_DIR"
if [ -n "$DETECTED" ]; then echo "Detected stack(s):$DETECTED — install the matching packs/ after init"; fi
case " $FRAMEWORKS " in *" react "*) echo "  React detected      → --pack typescript (Biome's react rules auto-apply)" ;; esac
case " $FRAMEWORKS " in *" vue "*) echo "  Vue detected        → --pack typescript (Biome's vue rules auto-apply)" ;; esac
case " $FRAMEWORKS " in *" spring-boot "*) echo "  Spring Boot detected → --pack java (JUnit 5 + Testcontainers stack)" ;; esac
echo ""

# ── Collect project-specific values ──────────────────────────────────
ask "Project name (e.g., MyProject): " PROJECT_NAME
ask "Project slug — lowercase, for paths (e.g., myproject): " PROJECT_SLUG
ask "GitHub owner for CODEOWNERS (e.g., @yourname): " GITHUB_OWNER
ask "opencode username (e.g., ${PROJECT_SLUG}-founder): " OPENCODE_USERNAME
ask "Protected path — permanently human-reviewed dir (e.g., internal/billing): " PROTECTED_PATH
ask "Spec/docs source dir (or leave empty if none): " DOCS_ROOT
ask "Citation prefix for spec docs (e.g., MYPROJECT_ or leave empty): " CITATION_PREFIX
# Model provider. This only SEEDS default model tiers — every value lands in
# factory.yaml as a plain string you can change, and 'inherit' writes none at
# all. The factory does not assume you use any particular provider or have keys
# for one; opencode alone reaches 75+ providers via 'provider/model' strings.
if [ -r /dev/tty ] && [ -t 1 ]; then
  echo ""
  echo "Model provider — seeds the default model tiers (override any of them in factory.yaml):"
  echo "  inherit     keep whatever each harness already uses — nothing written, no assumptions"
  echo "  openrouter  one key, many models"
  echo "  anthropic   Claude models directly"
  echo "  openai      GPT models directly"
  echo "  other       e.g. ollama, bedrock, azure — you supply the model strings"
fi
ask "Provider? [inherit/openrouter/anthropic/openai/other]: " MODEL_PROVIDER
ask "Cost profile — 'standard' or 'economy' (economy adds a cheaper tier for low-stakes roles): " COST_PROFILE
# The review lane is opt-in and stays off unless asked for: it spends tokens on
# every pull request and needs a repository secret only the adopter can add.
# State both costs before the question, not after the answer.
if [ -r /dev/tty ] && [ -t 1 ]; then
  echo ""
  echo "Adversarial PR review (optional) — a model reviews the diff of every pull"
  echo "request and posts an advisory comment. Advisory only, never a required check."
  echo "  costs:  tokens on every PR, at the frontier tier (the reviewer is never cheap)"
  echo "  needs:  a repository secret you add in GitHub Settings"
  echo "  later:  ./factory review-lane enable   (or disable) at any time"
fi
ask "Enable the adversarial review lane? [y/N]: " REVIEW_LANE_ANSWER
# Normalize case so "Economy"/"ECONOMY" select the profile, and reject anything
# that is neither — a silent fall-through to standard would be a surprising
# override of what the user typed.
COST_PROFILE="$(printf '%s' "${COST_PROFILE:-standard}" | tr '[:upper:]' '[:lower:]')"
case "$COST_PROFILE" in
  economy) : ;;
  standard) : ;;
  *) echo "  (unrecognized cost profile '$COST_PROFILE' — using 'standard')"; COST_PROFILE="standard" ;;
esac
# Pick language pack(s) before asking versions, so we only ask for the versions
# the selected packs actually need.
if [ -z "${PACKS// /}" ] && [ -r /dev/tty ] && [ -t 1 ]; then
  echo ""
  echo "Language pack(s) — arm test patterns + check command (space-separated):"
  echo "  go          battle-tested"
  echo "  typescript  experimental"
  echo "  java        experimental"
  ask "Pack(s)? [go typescript java / none]: " PACKS
  PACKS="$(printf '%s' "$PACKS" | tr ',' ' ')"
fi

case " $PACKS " in *" go "*) ask "Go version for CI (e.g., 1.26): " GO_VERSION ;; esac
case " $PACKS " in *" java "*) ask "Java (JDK) version for CI (e.g., 25): " JAVA_VERSION ;; esac
case " $PACKS " in *" typescript "*) ask "Node.js version for CI (e.g., 24): " NODE_VERSION ;; esac

# Defaults
MODEL_PROVIDER="$(printf '%s' "${MODEL_PROVIDER:-inherit}" | tr '[:upper:]' '[:lower:]')"
# Every tier starts blank — blank is a real, meaningful value here ("inherit"),
# and under `set -u` an unseeded tier would otherwise abort the install.
DEFAULT_MODEL="${DEFAULT_MODEL:-}"; FRONTIER_MODEL="${FRONTIER_MODEL:-}"; ECONOMY_MODEL="${ECONOMY_MODEL:-}"
CLAUDE_FRONTIER_MODEL="${CLAUDE_FRONTIER_MODEL:-}"; CLAUDE_DEFAULT_MODEL="${CLAUDE_DEFAULT_MODEL:-}"; CLAUDE_ECONOMY_MODEL="${CLAUDE_ECONOMY_MODEL:-}"
CODEX_FRONTIER_MODEL="${CODEX_FRONTIER_MODEL:-}"; CODEX_DEFAULT_MODEL="${CODEX_DEFAULT_MODEL:-}"; CODEX_ECONOMY_MODEL="${CODEX_ECONOMY_MODEL:-}"
# opencode reaches any provider with a 'provider/model' string, so its tiers are
# seeded from the chosen provider. Claude Code and Codex only ever talk to
# Anthropic and OpenAI respectively, so their tiers use those native ids
# whenever we seed at all — they apply only if you actually run that harness.
case "$MODEL_PROVIDER" in
  openrouter)
    DEFAULT_MODEL="${DEFAULT_MODEL:-openrouter/z-ai/glm-5.2}"
    FRONTIER_MODEL="${FRONTIER_MODEL:-openrouter/z-ai/glm-5.2}"
    ECONOMY_MODEL="${ECONOMY_MODEL:-openrouter/qwen/qwen3-coder}"
    ;;
  anthropic)
    DEFAULT_MODEL="${DEFAULT_MODEL:-anthropic/claude-sonnet-4-6}"
    FRONTIER_MODEL="${FRONTIER_MODEL:-anthropic/claude-opus-4-8}"
    ECONOMY_MODEL="${ECONOMY_MODEL:-anthropic/claude-haiku-4-5}"
    ;;
  openai)
    DEFAULT_MODEL="${DEFAULT_MODEL:-openai/gpt-5.6-terra}"
    FRONTIER_MODEL="${FRONTIER_MODEL:-openai/gpt-5.6-sol}"
    ECONOMY_MODEL="${ECONOMY_MODEL:-openai/gpt-5.6-luna}"
    ;;
  inherit)
    : ;;   # zero assumption — every tier stays blank, each harness keeps its own
  *)
    echo "  (provider '$MODEL_PROVIDER': set opencode_*_model in factory.yaml — see docs/MODELS.md)"
    ;;
esac
if [ "$MODEL_PROVIDER" != "inherit" ]; then
  CLAUDE_FRONTIER_MODEL="${CLAUDE_FRONTIER_MODEL:-claude-opus-4-8}"
  CLAUDE_DEFAULT_MODEL="${CLAUDE_DEFAULT_MODEL:-claude-sonnet-4-6}"
  CODEX_FRONTIER_MODEL="${CODEX_FRONTIER_MODEL:-gpt-5.6-sol}"
  CODEX_DEFAULT_MODEL="${CODEX_DEFAULT_MODEL:-gpt-5.6-terra}"
fi
# Economy tier, per harness. Under 'economy' the low-stakes roles (refactorer,
# wiki-maintainer, opencode small_model) route to a cheaper model. Under
# 'standard' the economy tier collapses to that harness's default model, so
# behaviour is unchanged. spec-writer and reviewer stay frontier throughout.
# Economy-tier models are stored raw (uncollapsed) for every harness, regardless
# of profile. The standard/economy collapse is applied at sync time by
# resolve_tier reading COST_PROFILE — so flipping cost_profile in factory.yaml
# and re-running `make sync-harnesses` re-routes every harness, no re-init.
# Review lane: normalise the answer and pick a default secret name for the
# chosen provider. REVIEW_MODEL blank means "use the frontier tier at run time".
case "$(printf '%s' "${REVIEW_LANE_ANSWER:-n}" | tr '[:upper:]' '[:lower:]')" in
  y|yes) REVIEW_LANE="on" ;;
  *)     REVIEW_LANE="off" ;;
esac
case "$MODEL_PROVIDER" in
  anthropic) REVIEW_API_KEY_SECRET="${REVIEW_API_KEY_SECRET:-ANTHROPIC_API_KEY}" ;;
  openai)    REVIEW_API_KEY_SECRET="${REVIEW_API_KEY_SECRET:-OPENAI_API_KEY}" ;;
  *)         REVIEW_API_KEY_SECRET="${REVIEW_API_KEY_SECRET:-OPENROUTER_API_KEY}" ;;
esac
REVIEW_MODEL="${REVIEW_MODEL:-}"
if [ "$MODEL_PROVIDER" != "inherit" ]; then
  CLAUDE_ECONOMY_MODEL="${CLAUDE_ECONOMY_MODEL:-claude-haiku-4-5}"
  CODEX_ECONOMY_MODEL="${CODEX_ECONOMY_MODEL:-gpt-5.6-luna}"
fi
# Blank tiers are meaningful: sync writes nothing and each harness keeps its own
# model. Say so, or an 'economy' profile with no models looks broken.
if [ "$MODEL_PROVIDER" = "inherit" ] && [ "$COST_PROFILE" = "economy" ]; then
  echo "  (economy profile selected with provider 'inherit' — set the model tiers"
  echo "   in factory.yaml for it to route anything; see docs/MODELS.md)"
fi
GO_VERSION="${GO_VERSION:-1.26}"
JAVA_VERSION="${JAVA_VERSION:-25}"
NODE_VERSION="${NODE_VERSION:-24}"
CITATION_PREFIX="${CITATION_PREFIX:-SPEC_}"

echo ""
echo "=== Summary ==="
echo "  Project name:     $PROJECT_NAME"
echo "  Project slug:     $PROJECT_SLUG"
echo "  GitHub owner:     $GITHUB_OWNER"
echo "  Protected path:   $PROTECTED_PATH"
echo "  Docs source:      ${DOCS_ROOT:-none}"
echo "  Citation prefix:  $CITATION_PREFIX"
echo "  Model provider:   $MODEL_PROVIDER$([ "$MODEL_PROVIDER" = inherit ] && echo " (no models written — each harness keeps its own)")"
[ -n "$DEFAULT_MODEL" ] && echo "  Default model:    $DEFAULT_MODEL"
[ -n "$FRONTIER_MODEL" ] && echo "  Frontier model:   $FRONTIER_MODEL"
COST_SUMMARY="$COST_PROFILE"
[ "$COST_PROFILE" = economy ] && COST_SUMMARY="$COST_PROFILE (economy model: $ECONOMY_MODEL)"
echo "  Cost profile:     $COST_SUMMARY"
echo "  Review lane:      $REVIEW_LANE$([ "$REVIEW_LANE" = on ] && echo " (needs repo secret: $REVIEW_API_KEY_SECRET)")"
echo "  Language pack(s): $([ -n "${PACKS// /}" ] && printf '%s' "$PACKS" | sed 's/^ *//' || echo none)"
case " $PACKS " in *" go "*) echo "  Go version:       $GO_VERSION" ;; esac
case " $PACKS " in *" java "*) echo "  Java version:     $JAVA_VERSION" ;; esac
case " $PACKS " in *" typescript "*) echo "  Node version:     $NODE_VERSION" ;; esac
echo ""
ask "Proceed? (y/N): " CONFIRM
if [[ ! "$CONFIRM" =~ ^[Yy] ]]; then
  echo "Aborted."
  exit 1
fi

# ── Write factory.yaml (Decision 2: runtime config, not substitution) ──
echo ""
echo "Writing factory.yaml..."
cat > "$TARGET_DIR/factory.yaml" <<FACTORYEOF
# Software Factory configuration. Flat key: value only — one value per line.
# Lists are space-separated. Parsed by scripts/lib/config.sh. See Decision 2
# in the template's docs/DECISION_LOG.md.
project_name: $PROJECT_SLUG
decision_log: docs/DECISION_LOG.md
docs_root: ${DOCS_ROOT:-docs}
citation_prefix: "$CITATION_PREFIX"
protected_paths: "$PROTECTED_PATH"
test_file_patterns: ""
language_packs: ""
check_command: ""
# Your own hooks go here, not in hook-existence-check.sh (a framework file
# that upgrade overwrites). Space-separated paths.
local_hooks: ""
wiki_root: wiki
wiki_staleness: false
FACTORYEOF
echo "  wrote: factory.yaml (arm test_file_patterns/check_command via a language pack)"

# ── Backup existing files ─────────────────────────────────────────────
echo ""
echo "Backing up existing files..."

BACKUP_SUFFIX=".factory-backup.$(date +%Y%m%d%H%M%S)"
BACKED_UP=0

backup_file() {
  local file="$1"
  if [ -f "$file" ] && [ -s "$file" ]; then
    cp "$file" "${file}${BACKUP_SUFFIX}"
    echo "  backed up: $file -> $(basename "$file")${BACKUP_SUFFIX}"
    BACKED_UP=$((BACKED_UP + 1))
  fi
}

# Files that would be overwritten by cp (not merged)
BACKUP_FILES=(
  "$TARGET_DIR/opencode.json"
  "$TARGET_DIR/AGENTS.md"
  "$TARGET_DIR/Makefile"
  "$TARGET_DIR/.gitignore"
  "$TARGET_DIR/.github/CODEOWNERS"
  "$TARGET_DIR/.github/workflows/ci.yml"
  "$TARGET_DIR/docs/FACTORY_RULES.md"
  "$TARGET_DIR/README.md"
  "$TARGET_DIR/.opencode/plugin/factory-hooks.ts"
  "$TARGET_DIR/.opencode/package.json"
)

for FILE in "${BACKUP_FILES[@]}"; do
  backup_file "$FILE"
done

# Hook scripts: back up any that already exist (cp overwrites individual files)
for FILE in "$TARGET_DIR/scripts/hooks/"*.sh; do
  [ -f "$FILE" ] && backup_file "$FILE"
done

for FILE in "$TARGET_DIR/.opencode/agent/"*.md; do
  [ -f "$FILE" ] && backup_file "$FILE"
done

for FILE in "$TARGET_DIR/.codex/agents/"*.toml "$TARGET_DIR/.codex/config.toml"; do
  [ -f "$FILE" ] && backup_file "$FILE"
done

if [ "$BACKED_UP" -gt 0 ]; then
  echo "  ($BACKED_UP file(s) backed up with suffix ${BACKUP_SUFFIX})"
else
  echo "  (no existing files to back up)"
fi

# ── Copy template files ──────────────────────────────────────────────
echo ""
echo "Copying template files..."

# Directories to create
mkdir -p "$TARGET_DIR/.opencode/plugin"
mkdir -p "$TARGET_DIR/.opencode/agent"
mkdir -p "$TARGET_DIR/.codex/agents"
mkdir -p "$TARGET_DIR/scripts/hooks"
mkdir -p "$TARGET_DIR/.github/workflows"
mkdir -p "$TARGET_DIR/docs/adr"
mkdir -p "$TARGET_DIR/memory/lessons"
mkdir -p "$TARGET_DIR/wiki"
cp "$TEMPLATE_DIR/wiki/README.md" "$TARGET_DIR/wiki/" 2>/dev/null || true
mkdir -p "$TARGET_DIR/specs"
mkdir -p "$TARGET_DIR/eval/golden-tasks/reference-answer"
mkdir -p "$TARGET_DIR/eval/results"
mkdir -p "$TARGET_DIR/eval/runners"
mkdir -p "$TARGET_DIR/workflows"
mkdir -p "$TARGET_DIR/scripts/lib"
mkdir -p "$TARGET_DIR/scripts/selftest"
mkdir -p "$TARGET_DIR/.githooks"

# Copy files (using cp -r for directories, cp for files)
cp "$TEMPLATE_DIR/scripts/hooks/"*.sh "$TARGET_DIR/scripts/hooks/"
cp "$TEMPLATE_DIR/scripts/lib/config.sh" "$TARGET_DIR/scripts/lib/"
cp "$TEMPLATE_DIR/scripts/lib/roles.sh" "$TARGET_DIR/scripts/lib/"
cp "$TEMPLATE_DIR/scripts/lib/events.sh" "$TARGET_DIR/scripts/lib/"
cp "$TEMPLATE_DIR/scripts/lib/hookspath.sh" "$TARGET_DIR/scripts/lib/"
cp "$TEMPLATE_DIR/scripts/lib/color.sh" "$TARGET_DIR/scripts/lib/"
cp "$TEMPLATE_DIR/scripts/selftest/run.sh" "$TARGET_DIR/scripts/selftest/"
cp "$TEMPLATE_DIR/scripts/pre-push-check.sh" "$TARGET_DIR/scripts/"
cp "$TEMPLATE_DIR/scripts/factory-doctor.sh" "$TARGET_DIR/scripts/"
cp "$TEMPLATE_DIR/scripts/factory-upgrade.sh" "$TARGET_DIR/scripts/"
cp "$TEMPLATE_DIR/scripts/factory-report.sh" "$TARGET_DIR/scripts/"
cp "$TEMPLATE_DIR/scripts/factory-metrics.sh" "$TARGET_DIR/scripts/"
mkdir -p "$TARGET_DIR/templates"
cp "$TEMPLATE_DIR/templates/metrics.html" "$TARGET_DIR/templates/"
cp "$TEMPLATE_DIR/scripts/factory-review-lane.sh" "$TARGET_DIR/scripts/"
cp "$TEMPLATE_DIR/scripts/factory-migrate-config.sh" "$TARGET_DIR/scripts/"
cp "$TEMPLATE_DIR/scripts/adversarial-review.sh" "$TARGET_DIR/scripts/"
mkdir -p "$TARGET_DIR/packs/review-lane"
cp "$TEMPLATE_DIR/packs/review-lane/review-pr.yml" "$TARGET_DIR/packs/review-lane/"
cp "$TEMPLATE_DIR/.githooks/pre-push" "$TARGET_DIR/.githooks/"
cp "$TEMPLATE_DIR/scripts/prereq-check.sh" "$TARGET_DIR/scripts/"
cp "$TEMPLATE_DIR/scripts/golden-task-eval.sh" "$TARGET_DIR/scripts/" 2>/dev/null || true
cp "$TEMPLATE_DIR/eval/README.md" "$TARGET_DIR/eval/" 2>/dev/null || true
cp "$TEMPLATE_DIR/eval/runners/mock.sh" "$TEMPLATE_DIR/eval/runners/example-harness.sh" "$TARGET_DIR/eval/runners/" 2>/dev/null || true
cp "$TEMPLATE_DIR/eval/golden-tasks/reference-answer/task.md" "$TEMPLATE_DIR/eval/golden-tasks/reference-answer/verify.sh" "$TARGET_DIR/eval/golden-tasks/reference-answer/" 2>/dev/null || true
cp "$TEMPLATE_DIR/workflows/review-diamond.md" "$TEMPLATE_DIR/workflows/eval-fanout.md" "$TEMPLATE_DIR/workflows/README.md" "$TARGET_DIR/workflows/" 2>/dev/null || true
cp "$TEMPLATE_DIR/scripts/sync-opencode.sh" "$TARGET_DIR/scripts/" 2>/dev/null || true
cp "$TEMPLATE_DIR/scripts/sync-claude.sh" "$TARGET_DIR/scripts/" 2>/dev/null || true
cp "$TEMPLATE_DIR/scripts/sync-codex.sh" "$TARGET_DIR/scripts/" 2>/dev/null || true
cp "$TEMPLATE_DIR/scripts/harness-structural-eval.sh" "$TARGET_DIR/scripts/" 2>/dev/null || true
cp "$TEMPLATE_DIR/scripts/citation-lint.sh" "$TARGET_DIR/scripts/" 2>/dev/null || true
cp "$TEMPLATE_DIR/.opencode/plugin/factory-hooks.ts" "$TARGET_DIR/.opencode/plugin/"
cp "$TEMPLATE_DIR/.opencode/agent/"*.md "$TARGET_DIR/.opencode/agent/"
cp "$TEMPLATE_DIR/.opencode/package.json" "$TARGET_DIR/.opencode/"
cp "$TEMPLATE_DIR/.opencode/.gitignore" "$TARGET_DIR/.opencode/"
cp "$TEMPLATE_DIR/.codex/config.toml" "$TARGET_DIR/.codex/"
cp "$TEMPLATE_DIR/.codex/agents/"*.toml "$TARGET_DIR/.codex/agents/"
cp "$TEMPLATE_DIR/opencode.json" "$TARGET_DIR/"
cp "$TEMPLATE_DIR/AGENTS.md" "$TARGET_DIR/"
cp "$TEMPLATE_DIR/Makefile" "$TARGET_DIR/"
cp "$TEMPLATE_DIR/factory" "$TARGET_DIR/" && chmod +x "$TARGET_DIR/factory"
cp "$TEMPLATE_DIR/.gitignore" "$TARGET_DIR/"
cp "$TEMPLATE_DIR/.github/CODEOWNERS" "$TARGET_DIR/.github/"
cp "$TEMPLATE_DIR/.github/workflows/ci.yml" "$TARGET_DIR/.github/workflows/"
cp "$TEMPLATE_DIR/docs/FACTORY_RULES.md" "$TARGET_DIR/docs/"
cp "$TEMPLATE_DIR/memory/lessons/001-verification-contract.md" "$TARGET_DIR/memory/lessons/"
cp "$TEMPLATE_DIR/README.md" "$TARGET_DIR/"

# Copy specs template if it exists
cp "$TEMPLATE_DIR/specs/TEMPLATE.md" "$TARGET_DIR/specs/" 2>/dev/null || true

# ── Substitute placeholders ───────────────────────────────────────────
echo "Substituting placeholders..."

# The citation prefix is written once, to factory.yaml, from what you were asked
# (defaulted to SPEC_). There used to be a second one here, derived from the
# project slug and written to factory.config — the same setting, spelled twice,
# holding two different values, and read by nothing. Decision 41 is about exactly
# that; it is gone rather than carried across.

# Files to substitute
SUBSTITUTE_FILES=(
  "$TARGET_DIR/opencode.json"
  "$TARGET_DIR/AGENTS.md"
  "$TARGET_DIR/Makefile"
  "$TARGET_DIR/.github/CODEOWNERS"
  "$TARGET_DIR/.github/workflows/ci.yml"
  "$TARGET_DIR/.opencode/plugin/factory-hooks.ts"
  "$TARGET_DIR/.opencode/agent/spec-writer.md"
  "$TARGET_DIR/.opencode/agent/implementer.md"
  "$TARGET_DIR/.opencode/agent/refactorer.md"
  "$TARGET_DIR/.opencode/agent/wiki-maintainer.md"
  "$TARGET_DIR/.opencode/agent/reviewer.md"
  "$TARGET_DIR/scripts/hooks/test-edit-denial.sh"
  "$TARGET_DIR/scripts/hooks/loop-close-check.sh"
  "$TARGET_DIR/scripts/hooks/hook-existence-check.sh"
  "$TARGET_DIR/scripts/hooks/shared-script-enforcement.sh"
  "$TARGET_DIR/scripts/hooks/commit-message-lint.sh"
  "$TARGET_DIR/scripts/hooks/diff-aware-check.sh"
  "$TARGET_DIR/scripts/hooks/decision-log-gate.sh"
  "$TARGET_DIR/scripts/hooks/ginkgo-only-check.sh"
  "$TARGET_DIR/scripts/hooks/direct-main-push-block.sh"
  "$TARGET_DIR/scripts/citation-lint.sh"
  "$TARGET_DIR/scripts/sync-claude.sh"
  "$TARGET_DIR/scripts/sync-codex.sh"
  "$TARGET_DIR/scripts/harness-structural-eval.sh"
  "$TARGET_DIR/scripts/prereq-check.sh"
  "$TARGET_DIR/scripts/pre-push-check.sh"
  "$TARGET_DIR/.codex/config.toml"
  "$TARGET_DIR/.codex/agents/implementer.toml"
  "$TARGET_DIR/.codex/agents/refactorer.toml"
  "$TARGET_DIR/.codex/agents/reviewer.toml"
  "$TARGET_DIR/.codex/agents/spec-writer.toml"
  "$TARGET_DIR/.codex/agents/wiki-maintainer.toml"
  "$TARGET_DIR/docs/FACTORY_RULES.md"
  "$TARGET_DIR/memory/lessons/001-verification-contract.md"
  "$TARGET_DIR/README.md"
)

for FILE in "${SUBSTITUTE_FILES[@]}"; do
  if [ -f "$FILE" ]; then
    DOCS_ROOT_RESOLVED="${DOCS_ROOT:-docs}"
    sed -i.bak \
      -e "s|__PROJECT_NAME__|$PROJECT_NAME|g" \
      -e "s|__DOCS_ROOT__|$DOCS_ROOT_RESOLVED|g" \
      -e "s|__PROJECT_SLUG__|$PROJECT_SLUG|g" \
      -e "s|__GITHUB_OWNER__|$GITHUB_OWNER|g" \
      -e "s|__OPENCODE_USERNAME__|$OPENCODE_USERNAME|g" \
      -e "s|__PROTECTED_PATH__|${PROTECTED_PATH:-.}|g" \
      "$FILE"
    rm -f "$FILE.bak"
  fi
done

# ── Make scripts executable ───────────────────────────────────────────
echo "Making scripts executable..."
chmod +x "$TARGET_DIR/scripts/hooks/"*.sh
chmod +x "$TARGET_DIR/scripts/prereq-check.sh"
chmod +x "$TARGET_DIR/scripts/golden-task-eval.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/eval/runners/"*.sh "$TARGET_DIR/eval/golden-tasks/reference-answer/verify.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/sync-opencode.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/sync-claude.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/sync-codex.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/harness-structural-eval.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/citation-lint.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/pre-push-check.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/factory-doctor.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/factory-upgrade.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/factory-report.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/factory-metrics.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/factory-review-lane.sh" "$TARGET_DIR/scripts/adversarial-review.sh" "$TARGET_DIR/scripts/factory-migrate-config.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/selftest/run.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/.githooks/pre-push" 2>/dev/null || true

# ── Append the harness settings to factory.yaml ──────────────────────
# One configuration file (Decision 41). These settings used to live in a separate
# factory.config that every script *sourced*; they are now ordinary factory.yaml
# keys, parsed like the rest. Appended rather than written into the heredoc above
# so the two halves stay readable: what the gates read, then what the harnesses do.
cat >> "$TARGET_DIR/factory.yaml" <<EOF

# ── Identity ─────────────────────────────────────────────────────────
project_display_name: "$PROJECT_NAME"
github_owner: "$GITHUB_OWNER"
opencode_username: "$OPENCODE_USERNAME"

# ── Models ───────────────────────────────────────────────────────────
# Which provider seeded the tiers below. Blank tiers mean "inherit": sync writes
# nothing and each harness keeps its own model. See docs/MODELS.md.
cost_profile: "$COST_PROFILE"
model_provider: "$MODEL_PROVIDER"
opencode_frontier_model: "$FRONTIER_MODEL"
opencode_default_model: "$DEFAULT_MODEL"
opencode_economy_model: "$ECONOMY_MODEL"
claude_frontier_model: "$CLAUDE_FRONTIER_MODEL"
claude_default_model: "$CLAUDE_DEFAULT_MODEL"
claude_economy_model: "$CLAUDE_ECONOMY_MODEL"
codex_frontier_model: "$CODEX_FRONTIER_MODEL"
codex_default_model: "$CODEX_DEFAULT_MODEL"
codex_economy_model: "$CODEX_ECONOMY_MODEL"

# ── Advisory adversarial PR review ───────────────────────────────────
# Opt-in; costs tokens per PR and needs the repository secret named below.
# Toggle with: ./factory review-lane enable|disable
review_lane: "$REVIEW_LANE"
review_model: "$REVIEW_MODEL"
review_api_key_secret: "$REVIEW_API_KEY_SECRET"

# ── Toolchain versions ───────────────────────────────────────────────
go_version: "$GO_VERSION"
java_version: "$JAVA_VERSION"
node_version: "$NODE_VERSION"
EOF

# ── Install opencode plugin deps ─────────────────────────────────────
echo ""
echo "Installing opencode plugin dependencies..."
if [ -f "$TARGET_DIR/.opencode/package.json" ]; then
  (cd "$TARGET_DIR/.opencode" && npm install 2>/dev/null || echo "  npm install failed — run manually in .opencode/")
fi

# ── Done ─────────────────────────────────────────────────────────────
echo ""
echo "=== Setup complete ==="
echo ""
echo "Next steps:"
echo "  1. Run prereq-check:    ./scripts/prereq-check.sh"
echo "  2. Sync adapters:       make sync-harnesses"
echo "  3. Start opencode:      opencode"
echo "  4. Review AGENTS.md and edit the Project section for your project"
echo "  5. Add your protected code to $PROTECTED_PATH/"
echo "  6. Install pre-push:    cp scripts/pre-push-check.sh .git/hooks/pre-push"
echo "  7. Check health anytime: ./factory doctor"
echo ""
echo "factory.yaml saved — one config file, parsed and never executed."

# ── What still needs a human, printed last so it cannot scroll away ──
# A reminder buried above a doctor run is a reminder nobody acts on. This is
# recomputed from actual state every run, so it disappears once the work is done
# rather than nagging forever.
if [ -x "$TARGET_DIR"/scripts/factory-review-lane.sh ]; then
  # Run it inside the target repo: review-lane resolves its root with git
  # rev-parse from the CWD, which during init is wherever the installer was
  # invoked — not necessarily the repo being set up.
  PENDING_OUT="$( (cd "$TARGET_DIR" && ./scripts/factory-review-lane.sh pending) 2>/dev/null || true)"
  if [ -n "$PENDING_OUT" ]; then
    echo ""
    # shellcheck source=lib/color.sh
    [ -f "$TARGET_DIR"/scripts/lib/color.sh ] && . "$TARGET_DIR"/scripts/lib/color.sh
    printf '%s\n' "${C_YELLOW:-}${C_BOLD:-}┌─ Action required ${C_RESET:-}"
    printf '%s\n' "$PENDING_OUT" | while IFS= read -r _pl; do
      printf '%s\n' "${C_YELLOW:-}│${C_RESET:-} $_pl"
    done
    printf '%s\n' "${C_YELLOW:-}└─${C_RESET:-}"
  fi
fi


# ── Install a language pack (arms the gates for your language) ────────
# Packs live in the template's packs/<lang>/. Selecting one merges its
# test_file_patterns and check_command into factory.yaml (so the test-edit
# hook and the diff-aware check are armed) and copies whatever real files the
# pack ships. Only Go is battle-tested; TypeScript and Java are experimental
# scaffolds that arm the patterns but ship no stack configs yet (Decision 3).
set_factory_key() {
  # Rewrite one key's line without regex tools — pack values contain backslashes,
  # pipes, and $ (e.g. the Go pattern _test\.go([^[:alnum:]_]|$)), which sed
  # would misinterpret. Pure-bash case-glob + literal insertion preserves them.
  local key="$1" val="$2" file="$TARGET_DIR/factory.yaml" line out=""
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      "$key:"*) out="$out$key: \"$val\""$'\n' ;;
      *)        out="$out$line"$'\n' ;;
    esac
  done < "$file"
  printf '%s' "$out" > "$file"
}

pack_config() {  # read one key from a pack.yaml
  FACTORY_CONFIG="$1/pack.yaml" bash -c '. "'"$SCRIPT_DIR"'/lib/config.sh"; factory_config_get "'"$2"'"'
}

if [ -n "${PACKS// /}" ]; then
  ALL_PATTERNS=""
  ALL_CHECK=""
  INSTALLED=""
  # shellcheck disable=SC2086  # PACKS is a space-separated list — split on purpose.
  for PACK in $PACKS; do
    [ "$PACK" = "none" ] && continue
    case " $INSTALLED " in *" $PACK "*) continue ;; esac   # dedupe repeats
    PACK_DIR="$TEMPLATE_DIR/packs/$PACK"
    if [ ! -f "$PACK_DIR/pack.yaml" ]; then
      echo "factory-init: unknown pack '$PACK' (have: go typescript java). Skipping."
      continue
    fi
    echo ""
    echo "Installing '$PACK' pack..."
    P_MATURITY="$(pack_config "$PACK_DIR" maturity)"
    P_PATTERNS="$(pack_config "$PACK_DIR" test_file_patterns)"
    P_CHECK="$(pack_config "$PACK_DIR" check_command)"
    [ -n "$P_PATTERNS" ] && ALL_PATTERNS="$ALL_PATTERNS $P_PATTERNS"
    if [ -n "$P_CHECK" ]; then
      if [ -n "$ALL_CHECK" ]; then ALL_CHECK="$ALL_CHECK && $P_CHECK"; else ALL_CHECK="$P_CHECK"; fi
    fi
    INSTALLED="$INSTALLED $PACK"

    # Copy pack root files (Go's .golangci.yml, Java's quality.gradle, TS's
    # biome.json/stryker.config.json). dotglob so dotfiles match; a subshell
    # scopes the option so it does not leak into the rest of the installer.
    (
      shopt -s dotglob nullglob
      for pf in "$PACK_DIR"/*; do
        [ -f "$pf" ] || continue
        case "$(basename "$pf")" in
          pack.yaml|.DS_Store) continue ;;
        esac
        cp "$pf" "$TARGET_DIR/"
        echo "  copied: $(basename "$pf")"
      done
    )
    if [ -d "$PACK_DIR/hooks" ]; then
      cp "$PACK_DIR/hooks/"*.sh "$TARGET_DIR/scripts/hooks/" 2>/dev/null && \
        chmod +x "$TARGET_DIR/scripts/hooks/"*.sh && echo "  copied: pack hooks"
    fi
    if [ -f "$PACK_DIR/workflows/ci.yml" ]; then
      sed -e "s|__GO_VERSION__|$GO_VERSION|g" \
          -e "s|__JAVA_VERSION__|$JAVA_VERSION|g" \
          -e "s|__NODE_VERSION__|$NODE_VERSION|g" \
          -e "s|__PROTECTED_PATH__|${PROTECTED_PATH:-.}|g" \
        "$PACK_DIR/workflows/ci.yml" \
        > "$TARGET_DIR/.github/workflows/${PACK}-pack.yml"
      echo "  installed: .github/workflows/${PACK}-pack.yml"
    fi

    [ -n "$(pack_config "$PACK_DIR" go_min_version)" ] && \
      { printf 'go_min_version: "%s"\n' "$GO_VERSION" >> "$TARGET_DIR/factory.yaml"; echo "  set: go_min_version"; }
    [ -n "$(pack_config "$PACK_DIR" java_min_version)" ] && \
      { printf 'java_min_version: "%s"\n' "$JAVA_VERSION" >> "$TARGET_DIR/factory.yaml"; echo "  set: java_min_version"; }
    [ -n "$(pack_config "$PACK_DIR" node_min_version)" ] && \
      { printf 'node_min_version: "%s"\n' "$NODE_VERSION" >> "$TARGET_DIR/factory.yaml"; echo "  set: node_min_version"; }

    if [ "$P_MATURITY" != "battle-tested" ]; then
      echo "  NOTE: '$PACK' is $P_MATURITY — the full stack ships, but no real repository has adopted it yet."
    fi
  done

  if [ -n "${INSTALLED// /}" ]; then
    set_factory_key test_file_patterns "$(printf '%s' "$ALL_PATTERNS" | sed 's/^ *//; s/  */ /g; s/ *$//')"
    set_factory_key check_command "$ALL_CHECK"
    set_factory_key language_packs "$(printf '%s' "$INSTALLED" | sed 's/^ *//; s/  */ /g; s/ *$//')"
    echo ""
    echo "  armed factory.yaml for:$INSTALLED"
  fi
fi

# ── Apply per-tier models to every harness from factory.yaml ──────────
# opencode.json, .claude/agents, and .codex/agents all get their models here,
# so the repo is ready to use. Reconfiguring later is the same one command:
# edit factory.yaml, then `make sync-harnesses`.
echo ""
echo "Applying models to opencode, Claude, and Codex..."
(cd "$TARGET_DIR" && ./scripts/sync-opencode.sh && ./scripts/sync-claude.sh && ./scripts/sync-codex.sh) \
  || echo "  warning: harness sync did not complete — run 'make sync-harnesses' (jq required)"

# ── Install the review lane if it was asked for ──────────────────────
# Route through the same enable command the upgrade offer uses, so there is one
# install path rather than two that can drift. Recording REVIEW_LANE="on" in the
# config without installing the workflow would be a flag that governs nothing.
if [ "$REVIEW_LANE" = "on" ] && [ -x "$TARGET_DIR/scripts/factory-review-lane.sh" ]; then
  echo ""
  ( cd "$TARGET_DIR" && ./scripts/factory-review-lane.sh enable ) \
    || echo "  warning: could not enable the review lane — run './factory review-lane enable'"
fi

# ── Post-install attestation (Verification Contract rule 3) ───────────
# The installer does not say "done" — it proves the installed gates fire.
echo ""
echo "=== Post-install attestation: break/fix self-test of installed gates ==="
if (cd "$TARGET_DIR" && ./scripts/selftest/run.sh); then
  echo ""
  if [ -n "${INSTALLED:-}" ] && [ -n "${INSTALLED// /}" ]; then
    echo "factory-init: gates proven and armed for:$INSTALLED. Commit when ready."
  else
    echo "factory-init: gates proven. Install a language pack to arm the"
    echo "test-edit hook and check command: re-run with --pack go,typescript,java."
  fi
else
  echo ""
  echo "factory-init: INSTALL NOT VERIFIED — a gate failed its break/fix proof."
  echo "Do not rely on enforcement until this passes."
  exit 1
fi
