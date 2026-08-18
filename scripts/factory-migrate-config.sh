#!/usr/bin/env bash
set -euo pipefail

# scripts/factory-migrate-config.sh (Decision 41)
# Moves factory.config into factory.yaml, so the factory has one configuration
# file that is parsed rather than executed.
#
# Usage: factory migrate-config [--dry-run]
#
# What it does:
#   - copies every setting from factory.config into factory.yaml, as flat keys
#   - skips any key factory.yaml already defines (yours wins; nothing is clobbered)
#   - renames factory.config to factory.config.migrated
#   - leaves everything as an uncommitted diff for you to review
#
# What it never does: delete anything, or commit. If it stops halfway, the old
# file is still there and still read — the reader falls back to it for any key
# the YAML does not define, so a partial migration is not a broken repo.

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT" || exit 1
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/config.sh
. "$SCRIPT_DIR/lib/config.sh"

DRY_RUN=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=1; shift ;;
    *) echo "factory migrate-config: unknown argument '$1'" >&2; exit 2 ;;
  esac
done

LEGACY="$ROOT/factory.config"
YAML="$ROOT/factory.yaml"

if [ ! -f "$YAML" ]; then
  echo "factory migrate-config: no factory.yaml here — run 'factory init' first." >&2
  exit 1
fi
if [ ! -f "$LEGACY" ]; then
  echo "factory migrate-config: no factory.config — nothing to migrate."
  exit 0
fi

# Checked BEFORE anything is written. The rename at the end is what makes a bad
# migration one `git mv` from being undone, and an unconditional mv destroyed the
# backup that promise depends on. Refusing late was almost as bad: factory.yaml
# had already been rewritten and config_migrated recorded, so the repo was left
# half-migrated by a command that reported failure.
# -L as well as -e: a dangling symlink fails -e, and `mv` would then replace the
# link — destroying the pointer to a backup that may still exist elsewhere.
if [ -e "$LEGACY.migrated" ] || [ -L "$LEGACY.migrated" ]; then
  echo "" >&2
  echo "factory migrate-config: $LEGACY.migrated already exists." >&2
  echo "  Refusing to overwrite it — it is the backup from an earlier migration," >&2
  echo "  and the recovery path documented below depends on it surviving." >&2
  echo "  Move or remove that file, then run this again." >&2
  exit 1
fi
echo "Migrating factory.config into factory.yaml..."

moved=0
skipped=0
# Read the legacy file as data. It is shell-shaped, but parsing it beats sourcing
# it: this runs in a repo whose config may have come from a branch or a patch,
# and the entire point of the move is to stop executing that file.
while IFS= read -r line || [ -n "$line" ]; do
  case "$line" in
    ''|'#'*) continue ;;
  esac
  case "$line" in
    *=*) : ;;
    *) continue ;;
  esac

  key="${line%%=*}"
  value="${line#*=}"
  # Only well-formed shell identifiers; anything else is not a setting.
  case "$key" in
    ''|*[!A-Za-z0-9_]*) continue ;;
  esac
  # Unwrap the value, in the order a shell would read it:
  #   VALUE="x" # note   ->  x
  #   VALUE=x # note     ->  x
  #   VALUE="a # b"      ->  a # b   (a # inside quotes is part of the value)
  # A trailing comment is only stripped from the unquoted remainder, so a hash
  # that belongs to the value survives.
  case "$value" in
    \"*)
      # Quoted: take up to the closing quote, discard whatever follows.
      value="${value#\"}"
      value="${value%%\"*}"
      ;;
    \'*)
      value="${value#\'}"
      value="${value%%\'*}"
      ;;
    *)
      # Unquoted: a ' #' sequence starts a comment.
      value="$(printf '%s' "$value" | sed 's/[[:space:]]#.*$//; s/[[:space:]]*$//')"
      ;;
  esac

  yaml_key="$(printf '%s' "$key" | tr '[:upper:]' '[:lower:]')"

  # Present-but-blank is not "already configured". The runtime treats a blank
  # model key as unset and falls back to the legacy file — so skipping the legacy
  # value here, and then renaming that file, silently dropped the setting
  # altogether. A key only counts as yours if it has a value.
  if factory_config_has "$yaml_key" && [ -n "$(factory_config_get "$yaml_key")" ]; then
    printf '  keeping yours: %s (already set in factory.yaml)\n' "$yaml_key"
    skipped=$((skipped + 1))
    continue
  fi

  if [ -n "$DRY_RUN" ]; then
    printf '  would add: %s: "%s"\n' "$yaml_key" "$value"
  else
    factory_config_set "$yaml_key" "$value"
    printf '  moved: %s -> %s\n' "$key" "$yaml_key"
  fi
  moved=$((moved + 1))
done < "$LEGACY"

if [ -n "$DRY_RUN" ]; then
  echo ""
  echo "factory migrate-config: dry run — $moved key(s) would move, $skipped already set."
  echo "  Nothing was changed. Re-run without --dry-run to apply."
  exit 0
fi

# Record that this happened, so the upgrade stops offering it.
factory_config_set config_migrated "yes"

# Renamed, not deleted. The rename is what stops the fallback from reading it,
# and keeping the file means a migration that got something wrong is one `git mv`
# from being undone.
mv "$LEGACY" "$LEGACY.migrated"

echo ""
echo "factory migrate-config: $moved key(s) moved, $skipped kept as you had them."
echo "  factory.config -> factory.config.migrated (renamed, not deleted)"
echo "  Review the diff (git status), then commit. Nothing was committed for you."
echo "  If anything looks wrong: git checkout factory.yaml && git mv factory.config.migrated factory.config"
