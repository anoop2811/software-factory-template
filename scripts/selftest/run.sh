#!/bin/bash
set -euo pipefail

# scripts/selftest/run.sh
# Break/fix self-tests for the factory's own gates. Every hook is proven by
# watching it FIRE on the violation it exists to catch, then PASS on the
# clean case. A check that has only ever been seen passing is unverified
# (docs/FACTORY_RULES.md, Verification Contract rule 3).
#
# Run from anywhere: ./scripts/selftest/run.sh
# Exit 0 = every case held; exit 1 = at least one gate failed its proof.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMPLATE_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
HOOKS="$TEMPLATE_ROOT/scripts/hooks"
SANDBOX="$(mktemp -d)"
trap 'rm -rf "$SANDBOX"' EXIT

PASS=0
FAIL=0

check() {
  local name="$1" expected="$2" actual="$3"
  if [ "$expected" = "$actual" ]; then
    PASS=$((PASS + 1))
    echo "  ok: $name"
  else
    FAIL=$((FAIL + 1))
    echo "  FAIL: $name (expected exit $expected, got $actual)"
  fi
}

run_status() {
  set +e
  "$@" >/dev/null 2>&1
  local status=$?
  set -e
  echo "$status"
}

echo "[1/5] config parser"
CFG="$SANDBOX/parser.yaml"
printf 'plain: value\nquoted: "two words"\ncommented: kept # not this\nlist: a b c\n' > "$CFG"
# shellcheck source=../lib/config.sh
. "$TEMPLATE_ROOT/scripts/lib/config.sh"
export FACTORY_CONFIG="$CFG"
check "plain value" "value" "$(factory_config_get plain)"
check "quoted value" "two words" "$(factory_config_get quoted)"
check "trailing comment stripped" "kept" "$(factory_config_get commented)"
check "space-separated list" "a b c" "$(factory_config_get list)"
check "missing key default" "fallback" "$(factory_config_get absent fallback)"
unset FACTORY_CONFIG

echo "[2/5] test-edit-denial"
CFG="$SANDBOX/denial.yaml"
printf 'test_file_patterns: "_test\\.go([^[:alnum:]_]|$) \\.spec\\.ts$"\n' > "$CFG"
export FACTORY_CONFIG="$CFG"
# BREAK: implementer editing a matching test file must be denied (exit 2).
check "deny implementer on _test.go" 2 \
  "$(FACTORY_AGENT_ROLE=implementer run_status "$HOOKS/test-edit-denial.sh" "pkg/parser_test.go")"
check "deny implementer on .spec.ts (second pattern)" 2 \
  "$(FACTORY_AGENT_ROLE=implementer run_status "$HOOKS/test-edit-denial.sh" "src/app.spec.ts")"
# FIX: every non-violating combination must be allowed (exit 0).
check "allow implementer on non-test file" 0 \
  "$(FACTORY_AGENT_ROLE=implementer run_status "$HOOKS/test-edit-denial.sh" "pkg/store.go")"
check "allow spec-writer on test file" 0 \
  "$(FACTORY_AGENT_ROLE=spec-writer run_status "$HOOKS/test-edit-denial.sh" "pkg/parser_test.go")"
check "allow unset role on test file" 0 \
  "$(run_status "$HOOKS/test-edit-denial.sh" "pkg/parser_test.go")"
printf 'test_file_patterns: ""\n' > "$CFG"
check "allow when no patterns configured" 0 \
  "$(FACTORY_AGENT_ROLE=implementer run_status "$HOOKS/test-edit-denial.sh" "pkg/parser_test.go")"
unset FACTORY_CONFIG

echo "[3/5] citation-lint"
CITE_DIR="$SANDBOX/cite"
mkdir -p "$CITE_DIR/docs"
printf 'line one\nline two\nline three\n' > "$CITE_DIR/docs/TESTPROJ_SPEC.md"
printf 'project: cite\ndocs_root: docs\ncitation_prefix: TESTPROJ_\n' > "$CITE_DIR/factory.yaml"
export FACTORY_CONFIG="$CITE_DIR/factory.yaml"
printf 'See TESTPROJ_SPEC.md:2 for details.\n' > "$CITE_DIR/note.md"
check "valid citation resolves" 0 \
  "$(cd "$CITE_DIR" && run_status "$TEMPLATE_ROOT/scripts/citation-lint.sh")"
# BREAK: a citation past end-of-file must fail.
printf 'See TESTPROJ_SPEC.md:99 for details.\n' > "$CITE_DIR/note.md"
check "out-of-range citation fails" 1 \
  "$(cd "$CITE_DIR" && run_status "$TEMPLATE_ROOT/scripts/citation-lint.sh")"
# Prefix unset disables the check even with a bad citation present.
printf 'project: cite\ndocs_root: docs\ncitation_prefix: ""\n' > "$CITE_DIR/factory.yaml"
check "empty prefix skips" 0 \
  "$(cd "$CITE_DIR" && run_status "$TEMPLATE_ROOT/scripts/citation-lint.sh")"
unset FACTORY_CONFIG

echo "[4/5] decision-log-gate"
GATE_DIR="$SANDBOX/gate"
mkdir -p "$GATE_DIR"
(
  cd "$GATE_DIR"
  git init -q -b main
  git config user.email selftest@example.invalid
  git config user.name selftest
  mkdir -p docs core
  printf 'project: gate\ndecision_log: docs/DECISION_LOG.md\nprotected_paths: "core"\n' > factory.yaml
  printf '# Decision Log\n\n## Decision 1: exists\n' > docs/DECISION_LOG.md
  git add -A && git commit -qm "chore: fixture base"
  printf 'x\n' > core/thing.txt
  git add -A && git commit -qm "feat: touch protected path without a reference"
)
BASE_SHA="$(git -C "$GATE_DIR" rev-parse HEAD~1)"
export FACTORY_CONFIG="$GATE_DIR/factory.yaml"
# BREAK: protected-path commit without a Decision reference must fail.
check "protected path without Decision ref fails" 1 \
  "$(cd "$GATE_DIR" && run_status "$TEMPLATE_ROOT/scripts/hooks/decision-log-gate.sh" "$BASE_SHA" HEAD)"
# FIX: amend the message to reference Decision 1 — must pass.
git -C "$GATE_DIR" commit -q --amend -m "feat: touch protected path

Implements Decision 1."
check "protected path with Decision ref passes" 0 \
  "$(cd "$GATE_DIR" && run_status "$TEMPLATE_ROOT/scripts/hooks/decision-log-gate.sh" "$BASE_SHA" HEAD)"
# BREAK: referencing a Decision that is not in the log must fail.
git -C "$GATE_DIR" commit -q --amend -m "feat: touch protected path

Implements Decision 99."
check "reference to absent Decision fails" 1 \
  "$(cd "$GATE_DIR" && run_status "$TEMPLATE_ROOT/scripts/hooks/decision-log-gate.sh" "$BASE_SHA" HEAD)"
unset FACTORY_CONFIG

echo "[5/5] pack patterns arm the test-edit hook"
# Regression guard: a pack's test_file_patterns must actually deny a matching
# test file (they were once double-escaped, matching nothing).
for PACK_YAML in "$TEMPLATE_ROOT"/packs/*/pack.yaml; do
  PACK_NAME="$(basename "$(dirname "$PACK_YAML")")"
  PPAT="$(FACTORY_CONFIG="$PACK_YAML" bash -c '. "'"$TEMPLATE_ROOT"'/scripts/lib/config.sh"; factory_config_get test_file_patterns')"
  PCFG="$SANDBOX/pack-$PACK_NAME.yaml"
  printf 'test_file_patterns: "%s"\n' "$PPAT" > "$PCFG"
  case "$PACK_NAME" in
    go)         PSAMPLE="pkg/foo_test.go" ;;
    typescript) PSAMPLE="src/app.test.ts" ;;
    java)       PSAMPLE="src/test/FooTest.java" ;;
    *)          PSAMPLE="" ;;
  esac
  [ -z "$PSAMPLE" ] && continue
  check "pack '$PACK_NAME' pattern denies $PSAMPLE" 2 \
    "$(FACTORY_AGENT_ROLE=implementer FACTORY_CONFIG="$PCFG" run_status "$HOOKS/test-edit-denial.sh" "$PSAMPLE")"
done

echo ""
echo "selftest: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
