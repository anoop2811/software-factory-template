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

# Parse args: an optional target dir (first non-flag) and --pack <lang>.
TARGET_ARG="."
PACK=""
while [ $# -gt 0 ]; do
  case "$1" in
    --pack) PACK="${2:-}"; shift 2 ;;
    --pack=*) PACK="${1#*=}"; shift ;;
    *) TARGET_ARG="$1"; shift ;;
  esac
done

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

echo "=== Software Factory Template Setup ==="
echo "Template dir: $TEMPLATE_DIR"
echo "Target dir:   $TARGET_DIR"
if [ -n "$DETECTED" ]; then echo "Detected stack(s):$DETECTED — install the matching packs/ after init"; fi
echo ""

# ── Collect project-specific values ──────────────────────────────────
ask "Project name (e.g., MyProject): " PROJECT_NAME
ask "Project slug — lowercase, for paths (e.g., myproject): " PROJECT_SLUG
ask "GitHub owner for CODEOWNERS (e.g., @yourname): " GITHUB_OWNER
ask "opencode username (e.g., ${PROJECT_SLUG}-founder): " OPENCODE_USERNAME
ask "Protected path — permanently human-reviewed dir (e.g., internal/billing): " PROTECTED_PATH
ask "Spec/docs source dir (or leave empty if none): " DOCS_ROOT
ask "Citation prefix for spec docs (e.g., MYPROJECT_ or leave empty): " CITATION_PREFIX
ask "Default model (e.g., openrouter/z-ai/glm-5.2): " DEFAULT_MODEL
ask "Frontier model (e.g., openrouter/anthropic/claude-sonnet-4.6): " FRONTIER_MODEL
ask "Go version for CI (e.g., 1.26): " GO_VERSION
ask "Java (JDK) version for CI (e.g., 25): " JAVA_VERSION
ask "Node.js version for CI (e.g., 24): " NODE_VERSION

# Defaults
DEFAULT_MODEL="${DEFAULT_MODEL:-openrouter/z-ai/glm-5.2}"
FRONTIER_MODEL="${FRONTIER_MODEL:-openrouter/anthropic/claude-sonnet-4.6}"
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
echo "  Default model:    $DEFAULT_MODEL"
echo "  Frontier model:   $FRONTIER_MODEL"
echo "  Go version:       $GO_VERSION"
echo "  Java version:     $JAVA_VERSION"
echo "  Node version:     $NODE_VERSION"
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
wiki_root: wiki
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
mkdir -p "$TARGET_DIR/eval/golden-tasks"
mkdir -p "$TARGET_DIR/eval/results"
mkdir -p "$TARGET_DIR/scripts/lib"
mkdir -p "$TARGET_DIR/scripts/selftest"
mkdir -p "$TARGET_DIR/.githooks"

# Copy files (using cp -r for directories, cp for files)
cp "$TEMPLATE_DIR/scripts/hooks/"*.sh "$TARGET_DIR/scripts/hooks/"
cp "$TEMPLATE_DIR/scripts/lib/config.sh" "$TARGET_DIR/scripts/lib/"
cp "$TEMPLATE_DIR/scripts/selftest/run.sh" "$TARGET_DIR/scripts/selftest/"
cp "$TEMPLATE_DIR/scripts/pre-push-check.sh" "$TARGET_DIR/scripts/"
cp "$TEMPLATE_DIR/scripts/factory-doctor.sh" "$TARGET_DIR/scripts/"
cp "$TEMPLATE_DIR/scripts/factory-upgrade.sh" "$TARGET_DIR/scripts/"
cp "$TEMPLATE_DIR/.githooks/pre-push" "$TARGET_DIR/.githooks/"
cp "$TEMPLATE_DIR/scripts/prereq-check.sh" "$TARGET_DIR/scripts/"
cp "$TEMPLATE_DIR/scripts/golden-task-eval.sh" "$TARGET_DIR/scripts/" 2>/dev/null || true
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

# Build the citation prefix uppercase (e.g., MYPROJECT_)
CITATION_PREFIX_UPPER=$(echo "$PROJECT_SLUG" | tr '[:lower:]' '[:upper:]')_

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
      -e "s|__DEFAULT_MODEL__|$DEFAULT_MODEL|g" \
      -e "s|__FRONTIER_MODEL__|$FRONTIER_MODEL|g" \
      "$FILE"
    rm -f "$FILE.bak"
  fi
done

# ── Make scripts executable ───────────────────────────────────────────
echo "Making scripts executable..."
chmod +x "$TARGET_DIR/scripts/hooks/"*.sh
chmod +x "$TARGET_DIR/scripts/prereq-check.sh"
chmod +x "$TARGET_DIR/scripts/golden-task-eval.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/sync-claude.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/sync-codex.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/harness-structural-eval.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/citation-lint.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/pre-push-check.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/factory-doctor.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/factory-upgrade.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/scripts/selftest/run.sh" 2>/dev/null || true
chmod +x "$TARGET_DIR/.githooks/pre-push" 2>/dev/null || true

# ── Create factory.config ────────────────────────────────────────────
cat > "$TARGET_DIR/factory.config" <<EOF
# factory.config — project-specific values for the software factory
# Generated by setup.sh. Edit and re-run setup.sh to update.
PROJECT_NAME="$PROJECT_NAME"
PROJECT_SLUG="$PROJECT_SLUG"
GITHUB_OWNER="$GITHUB_OWNER"
OPENCODE_USERNAME="$OPENCODE_USERNAME"
PROTECTED_PATH="$PROTECTED_PATH"
DOCS_ROOT="$DOCS_ROOT"
CITATION_PREFIX="$CITATION_PREFIX_UPPER"
DEFAULT_MODEL="$DEFAULT_MODEL"
FRONTIER_MODEL="$FRONTIER_MODEL"
GO_VERSION="$GO_VERSION"
JAVA_VERSION="$JAVA_VERSION"
NODE_VERSION="$NODE_VERSION"
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
echo "factory.config saved — re-run setup.sh to update placeholders."

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

if [ -z "$PACK" ] && [ -r /dev/tty ] && [ -t 1 ]; then
  echo ""
  echo "Language pack (arms test patterns + check command):"
  echo "  go          battle-tested"
  echo "  typescript  experimental"
  echo "  java        experimental"
  ask "Install a pack? [go/typescript/java/none]: " PACK
fi

if [ -n "$PACK" ] && [ "$PACK" != "none" ]; then
  PACK_DIR="$TEMPLATE_DIR/packs/$PACK"
  if [ ! -f "$PACK_DIR/pack.yaml" ]; then
    echo "factory-init: unknown pack '$PACK' (have: go typescript java). Skipping."
  else
    echo ""
    echo "Installing '$PACK' pack..."
    P_MATURITY="$(FACTORY_CONFIG="$PACK_DIR/pack.yaml" bash -c '. "'"$SCRIPT_DIR"'/lib/config.sh"; factory_config_get maturity')"
    P_PATTERNS="$(FACTORY_CONFIG="$PACK_DIR/pack.yaml" bash -c '. "'"$SCRIPT_DIR"'/lib/config.sh"; factory_config_get test_file_patterns')"
    P_CHECK="$(FACTORY_CONFIG="$PACK_DIR/pack.yaml" bash -c '. "'"$SCRIPT_DIR"'/lib/config.sh"; factory_config_get check_command')"
    set_factory_key test_file_patterns "$P_PATTERNS"
    set_factory_key check_command "$P_CHECK"
    set_factory_key language_packs "$PACK"
    echo "  armed factory.yaml: test_file_patterns, check_command (maturity: $P_MATURITY)"

    # Copy pack root files that land at the repository root (Go's .golangci.yml,
    # Java's quality.gradle). pack.yaml is metadata and is not shipped. dotglob
    # is required so dotfiles like .golangci.yml are matched; a subshell scopes
    # it so the option does not leak into the rest of the installer.
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

    P_MIN="$(FACTORY_CONFIG="$PACK_DIR/pack.yaml" bash -c '. "'"$SCRIPT_DIR"'/lib/config.sh"; factory_config_get go_min_version')"
    if [ -n "$P_MIN" ]; then
      printf 'go_min_version: "%s"\n' "$GO_VERSION" >> "$TARGET_DIR/factory.yaml"
      echo "  set: go_min_version"
    fi
    P_JMIN="$(FACTORY_CONFIG="$PACK_DIR/pack.yaml" bash -c '. "'"$SCRIPT_DIR"'/lib/config.sh"; factory_config_get java_min_version')"
    if [ -n "$P_JMIN" ]; then
      printf 'java_min_version: "%s"\n' "$JAVA_VERSION" >> "$TARGET_DIR/factory.yaml"
      echo "  set: java_min_version"
    fi
    P_NMIN="$(FACTORY_CONFIG="$PACK_DIR/pack.yaml" bash -c '. "'"$SCRIPT_DIR"'/lib/config.sh"; factory_config_get node_min_version')"
    if [ -n "$P_NMIN" ]; then
      printf 'node_min_version: "%s"\n' "$NODE_VERSION" >> "$TARGET_DIR/factory.yaml"
      echo "  set: node_min_version"
    fi

    if [ "$P_MATURITY" != "battle-tested" ]; then
      if [ -f "$PACK_DIR/workflows/ci.yml" ]; then
        echo "  NOTE: '$PACK' is $P_MATURITY — the full stack and CI ship, but no"
        echo "        real repository has adopted it yet. Report back if you do."
      else
        echo "  NOTE: '$PACK' is $P_MATURITY — test patterns and check command are"
        echo "        armed, but no linter/CI stack configs ship for this pack yet."
      fi
    fi
  fi
fi

# ── Post-install attestation (Verification Contract rule 3) ───────────
# The installer does not say "done" — it proves the installed gates fire.
echo ""
echo "=== Post-install attestation: break/fix self-test of installed gates ==="
if (cd "$TARGET_DIR" && ./scripts/selftest/run.sh); then
  echo ""
  if [ -n "$PACK" ] && [ "$PACK" != "none" ]; then
    echo "factory-init: gates proven and armed for the '$PACK' pack. Commit when ready."
  else
    echo "factory-init: gates proven. Install a language pack to arm the"
    echo "test-edit hook and check command: re-run with --pack go|typescript|java."
  fi
else
  echo ""
  echo "factory-init: INSTALL NOT VERIFIED — a gate failed its break/fix proof."
  echo "Do not rely on enforcement until this passes."
  exit 1
fi
