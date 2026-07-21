#!/usr/bin/env bash
set -euo pipefail

# scripts/factory-upgrade.sh (Decision 16)
# Framework-only upgrade. Re-fetches the template and copies the byte-identical
# framework files — hooks, scripts, the factory dispatcher, factory-doctor,
# .githooks, and the framework docs — over this repo. Thanks to Decision 2 the
# hooks carry no placeholders, so upgrading them is a clean copy.
#
# It NEVER touches your factory.yaml, your content (wiki/ pages, memory/lessons,
# specs, docs/DECISION_LOG.md), or your identity/customizable files
# (opencode.json, the agent prompts, AGENTS.md, README, CODEOWNERS, Makefile).
# Those it only *reports*, so you can reconcile upstream changes yourself.
# Everything lands as an uncommitted diff for you to review.
#
# Usage: factory upgrade [--ref <tag>] [--source <dir>]
#   --ref <tag>     template ref to upgrade to (default: $FACTORY_REF or main)
#   --source <dir>  use an existing template checkout instead of fetching

FACTORY_REPO="${FACTORY_REPO:-https://github.com/anoop2811/software-factory-template}"
FACTORY_REF="${FACTORY_REF:-main}"

SOURCE=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --ref) FACTORY_REF="${2:-}"; shift 2 ;;
    --ref=*) FACTORY_REF="${1#*=}"; shift ;;
    --source) SOURCE="${2:-}"; shift 2 ;;
    --source=*) SOURCE="${1#*=}"; shift ;;
    *) echo "factory upgrade: unknown argument '$1'" >&2; exit 2 ;;
  esac
done

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [ ! -f factory.yaml ]; then
  echo "factory upgrade: no factory.yaml here — is this a factory repo? Run 'factory init' first." >&2
  exit 1
fi

# ── Get the template at the target ref ───────────────────────────────
CLEANUP=""
if [ -n "$SOURCE" ]; then
  TEMPLATE="$SOURCE"
  [ -d "$TEMPLATE" ] || { echo "factory upgrade: --source '$TEMPLATE' is not a directory" >&2; exit 1; }
  echo "factory upgrade: using local template at $TEMPLATE"
else
  TMP="$(mktemp -d)"
  CLEANUP="$TMP"
  echo "factory upgrade: fetching $FACTORY_REPO at ref '$FACTORY_REF'..."
  git clone --quiet --depth 1 --branch "$FACTORY_REF" "$FACTORY_REPO" "$TMP/template"
  TEMPLATE="$TMP/template"
fi

# ── Framework files: byte-identical, safe to overwrite ───────────────
FRAMEWORK="
factory
.githooks/pre-push
scripts/lib/config.sh
scripts/lib/roles.sh
scripts/lib/events.sh
scripts/selftest/run.sh
scripts/factory-doctor.sh
scripts/factory-upgrade.sh
scripts/factory-report.sh
scripts/pre-push-check.sh
scripts/prereq-check.sh
scripts/citation-lint.sh
scripts/golden-task-eval.sh
scripts/harness-structural-eval.sh
scripts/sync-opencode.sh
scripts/sync-claude.sh
scripts/sync-codex.sh
"

copied=0

copy_framework() {
  local rel="$1"
  local src="$TEMPLATE/$rel"
  [ -f "$src" ] || return 0
  # Add or refresh the framework file. It carries no install-time placeholders,
  # so a copy is byte-identical by design (Decision 2). Missing files are added,
  # not skipped: a repo installed before a framework file existed (e.g. a new
  # lib that shipped scripts now source) must receive it, or those scripts break.
  # Only the parent directory must already exist, which init guarantees.
  [ -d "$(dirname "$rel")" ] || return 0
  if [ -f "$rel" ] && cmp -s "$src" "$rel"; then
    return 0
  fi
  local verb="updated"
  [ -f "$rel" ] || verb="added  "
  # Atomic replace via rename: mv swaps the inode, so if we are updating a file
  # currently being read (this script upgrading itself), the running process
  # keeps reading the old inode and the new version lands for next time.
  cp "$src" "$rel.factory-tmp.$$"
  mv -f "$rel.factory-tmp.$$" "$rel"
  echo "  $verb: $rel"
  copied=$((copied + 1))
}

echo "Upgrading framework files..."
for f in $FRAMEWORK; do copy_framework "$f"; done

# All core hooks the template ships.
for src in "$TEMPLATE"/scripts/hooks/*.sh; do
  [ -f "$src" ] || continue
  copy_framework "scripts/hooks/$(basename "$src")"
done

# Installed pack hooks (dialect gates), for any pack this repo uses.
LP="$(sed -n 's/^language_packs:[[:space:]]*//p' factory.yaml | head -1 | tr -d '"')"
for lang in $LP; do
  for src in "$TEMPLATE"/packs/"$lang"/hooks/*.sh; do
    [ -f "$src" ] || continue
    copy_framework "scripts/hooks/$(basename "$src")"
  done
done

# Restore executable bits on scripts.
chmod +x factory scripts/*.sh scripts/hooks/*.sh .githooks/pre-push 2>/dev/null || true

# ── Record the version we upgraded to ────────────────────────────────
UPSTREAM_COMMIT="$(git -C "$TEMPLATE" rev-parse --short HEAD 2>/dev/null || echo unknown)"
printf 'ref=%s\ncommit=%s\n' "$FACTORY_REF" "$UPSTREAM_COMMIT" > .factory-version
echo "  recorded: .factory-version (ref=$FACTORY_REF commit=$UPSTREAM_COMMIT)"

# ── Report identity/customizable files (never overwritten) ───────────
echo ""
echo "Yours to reconcile (not touched — the template may have improved these):"
IDENTITY="opencode.json AGENTS.md README.md Makefile .github/CODEOWNERS .github/workflows/ci.yml .opencode/agent .opencode/plugin"
for rel in $IDENTITY; do
  [ -e "$rel" ] || continue
  if [ -e "$TEMPLATE/$rel" ] && ! diff -rq "$TEMPLATE/$rel" "$rel" >/dev/null 2>&1; then
    echo "  review: $rel"
  fi
done
echo "  (the adapters .claude/ and .codex/ regenerate from opencode.json via 'make sync-harnesses')"

# ── Prove the gates still fire, then hand back for review ────────────
echo ""
if [ -x scripts/factory-doctor.sh ]; then
  echo "Running factory doctor..."
  ./scripts/factory-doctor.sh || echo "factory upgrade: doctor reported problems — review above before committing"
fi

echo ""
echo "factory upgrade: $copied file(s) updated."
if [ -n "$CLEANUP" ]; then
  echo "  A fresh template checkout is at $TEMPLATE — diff your 'review' files against it, then: rm -rf $CLEANUP"
fi
echo "  Review the diff (git status), then commit. Nothing was committed for you."
