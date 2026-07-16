#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/config.sh
. "$SCRIPT_DIR/lib/config.sh"

# scripts/prereq-check.sh
# Verifies that all tools required by the software factory are installed and
# meet the minimum version. Run before `setup.sh` and in CI.
#
# Tools checked:
#   - git (any modern version)
#   - go (minimum security patch)
#   - selected harness: opencode, claude, or codex (presence)
#   - golangci-lint (any version)
#   - jq (for sync-claude.sh and sync-codex.sh)
#   - python3 (for golden-task-eval.sh)
#
# Usage: ./scripts/prereq-check.sh
# Exit 0 = all good; Exit 1 = missing tool or version too old.

ERRORS=0

check() {
  local name="$1"
  local cmd="$2"
  local min_version="${3:-}"

  if ! command -v "$cmd" &>/dev/null; then
    echo "PREREQ FAIL: $name not found (command: $cmd)"
    ERRORS=$((ERRORS + 1))
    return
  fi

  local version_output
  version_output=$("$cmd" --version 2>&1 | head -1 || true)
  if [ -n "$min_version" ]; then
    local version
    version=$(echo "$version_output" | grep -oE '[0-9]+\.[0-9]+' | head -1 || echo "0.0")
    local min_major min_minor ver_major ver_minor
    min_major=$(echo "$min_version" | cut -d. -f1)
    min_minor=$(echo "$min_version" | cut -d. -f2)
    ver_major=$(echo "$version" | cut -d. -f1)
    ver_minor=$(echo "$version" | cut -d. -f2)

    if [ "$ver_major" -lt "$min_major" ] || \
       { [ "$ver_major" -eq "$min_major" ] && [ "$ver_minor" -lt "$min_minor" ]; }; then
      echo "PREREQ FAIL: $name version $version is below minimum $min_version"
      ERRORS=$((ERRORS + 1))
      return
    fi
  fi

  echo "prereq: OK $name ($version_output)"
}

check_go() {
  local min_version
  min_version="$(factory_config_get go_min_version)"
  if [ -z "$min_version" ]; then
    echo "  skip: no go_min_version in factory.yaml (no Go pack installed)"
    return 0
  fi
  if ! command -v go &>/dev/null; then
    echo "PREREQ FAIL: Go not found (command: go)"
    ERRORS=$((ERRORS + 1))
    return
  fi

  local version
  version=$(go version 2>/dev/null | grep -oE 'go[0-9]+\.[0-9]+\.[0-9]+' | head -1 | cut -c3- || echo "0.0.0")
  local min_major min_minor min_patch ver_major ver_minor ver_patch
  IFS=. read -r min_major min_minor min_patch <<< "$min_version"
  IFS=. read -r ver_major ver_minor ver_patch <<< "$version"

  if [ "$ver_major" -lt "$min_major" ] || \
     { [ "$ver_major" -eq "$min_major" ] && [ "$ver_minor" -lt "$min_minor" ]; } || \
     { [ "$ver_major" -eq "$min_major" ] && [ "$ver_minor" -eq "$min_minor" ] && [ "$ver_patch" -lt "$min_patch" ]; }; then
    echo "PREREQ FAIL: Go version $version is below minimum security patch $min_version"
    ERRORS=$((ERRORS + 1))
    return
  fi

  echo "prereq: OK Go ($(go version 2>/dev/null))"
}

check "git" "git"
check_go
FACTORY_HARNESS="${FACTORY_HARNESS:-opencode}"
case "$FACTORY_HARNESS" in
  opencode) check "opencode" "opencode" ;;
  claude) check "Claude Code" "claude" ;;
  codex) check "Codex" "codex" ;;
  *)
    echo "PREREQ FAIL: unsupported FACTORY_HARNESS=$FACTORY_HARNESS"
    ERRORS=$((ERRORS + 1))
    ;;
esac
check "golangci-lint" "golangci-lint"
check "jq" "jq"
check "python3" "python3" "3.8"

if [ "$ERRORS" -gt 0 ]; then
  echo ""
  echo "prereq-check: $ERRORS tool(s) missing or too old"
  echo "Install missing tools before running setup.sh."
  exit 1
fi

echo ""
echo "prereq-check: all tools present"
