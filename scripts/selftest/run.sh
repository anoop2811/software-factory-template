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

# Gate firings during the self-test log to a sandbox event log, not the real
# repo's .factory/events.log — so running the self-test never pollutes a
# developer's `factory report`.
export FACTORY_EVENT_LOG="$SANDBOX/events.log"

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

# Break/fix: the Java pack's junit5-only-check must reject a JUnit 4 import and
# accept a JUnit 5 (Jupiter) one.
JUNIT_HOOK="$TEMPLATE_ROOT/packs/java/hooks/junit5-only-check.sh"
if [ -x "$JUNIT_HOOK" ]; then
  JSAND="$SANDBOX/junit5"
  mkdir -p "$JSAND/src/test"
  printf 'import org.junit.Test;\npublic class FooTest {}\n' > "$JSAND/src/test/FooTest.java"
  check "junit5-only-check rejects JUnit 4 import" 1 \
    "$(run_status "$JUNIT_HOOK" "$JSAND")"
  printf 'import org.junit.jupiter.api.Test;\npublic class FooTest {}\n' > "$JSAND/src/test/FooTest.java"
  check "junit5-only-check accepts JUnit 5 (Jupiter)" 0 \
    "$(run_status "$JUNIT_HOOK" "$JSAND")"
fi

# Break/fix: the TypeScript pack's vitest-only-check must reject a non-Vitest
# test framework import and accept a Vitest one.
VITEST_HOOK="$TEMPLATE_ROOT/packs/typescript/hooks/vitest-only-check.sh"
if [ -x "$VITEST_HOOK" ]; then
  VSAND="$SANDBOX/vitest"
  mkdir -p "$VSAND/src"
  printf "import { describe } from 'jest';\n" > "$VSAND/src/app.test.ts"
  check "vitest-only-check rejects non-Vitest import" 1 \
    "$(run_status "$VITEST_HOOK" "$VSAND")"
  printf "import { describe } from 'vitest';\n" > "$VSAND/src/app.test.ts"
  check "vitest-only-check accepts Vitest" 0 \
    "$(run_status "$VITEST_HOOK" "$VSAND")"
fi

# Break/fix: wiki-lint requires provenance on every content page.
WIKI_HOOK="$HOOKS/wiki-lint.sh"
if [ -x "$WIKI_HOOK" ]; then
  WLWIKI="$SANDBOX/wl/wiki"
  mkdir -p "$WLWIKI"
  WLCFG="$SANDBOX/wl/factory.yaml"
  printf 'wiki_root: %s\n' "$WLWIKI" > "$WLCFG"
  printf '# Page\nA claim with no source.\n' > "$WLWIKI/page.md"
  check "wiki-lint rejects a page without provenance" 1 \
    "$(FACTORY_CONFIG="$WLCFG" run_status "$WIKI_HOOK")"
  printf '# Page\nA claim. Source: pkg/thing.go:3\n' > "$WLWIKI/page.md"
  check "wiki-lint accepts a cited page" 0 \
    "$(FACTORY_CONFIG="$WLCFG" run_status "$WIKI_HOOK")"

  # Reachability: an index present + an unlinked page is an orphan.
  OWIKI="$SANDBOX/wlo/wiki"
  mkdir -p "$OWIKI"
  OCFG="$SANDBOX/wlo/factory.yaml"
  printf 'wiki_root: %s\n' "$OWIKI" > "$OCFG"
  printf '# Index\n' > "$OWIKI/README.md"
  printf '# P\nCites pkg/x.go:3\n' > "$OWIKI/p.md"
  check "wiki-lint flags an orphan page" 1 \
    "$(FACTORY_CONFIG="$OCFG" run_status "$WIKI_HOOK")"
  printf '# Index\n[P](p.md)\n' > "$OWIKI/README.md"
  check "wiki-lint clears once the page is linked" 0 \
    "$(FACTORY_CONFIG="$OCFG" run_status "$WIKI_HOOK")"

  # Freshness (opt-in): a page older than its cited source is stale.
  SWDIR="$SANDBOX/wls"
  mkdir -p "$SWDIR/wiki" "$SWDIR/pkg"
  (
    cd "$SWDIR" || exit 1
    git init -q -b main
    git config user.email s@e.i
    git config user.name s
    printf 'wiki_root: wiki\nwiki_staleness: true\n' > factory.yaml
    printf 'x\n' > pkg/x.go
    printf '# P\nCites pkg/x.go:1\n' > wiki/p.md
    printf '# Index\n[P](p.md)\n' > wiki/README.md
    GIT_AUTHOR_DATE='2020-01-01T00:00:00' GIT_COMMITTER_DATE='2020-01-01T00:00:00' git add -A
    GIT_AUTHOR_DATE='2020-01-01T00:00:00' GIT_COMMITTER_DATE='2020-01-01T00:00:00' git commit -qm init
    printf 'x2\n' > pkg/x.go
    GIT_AUTHOR_DATE='2021-01-01T00:00:00' GIT_COMMITTER_DATE='2021-01-01T00:00:00' git add pkg/x.go
    GIT_AUTHOR_DATE='2021-01-01T00:00:00' GIT_COMMITTER_DATE='2021-01-01T00:00:00' git commit -qm src-later
  )
  check "wiki-lint (staleness) flags a page older than its source" 1 \
    "$(cd "$SWDIR" && FACTORY_CONFIG="$SWDIR/factory.yaml" run_status "$WIKI_HOOK")"
  (
    cd "$SWDIR" || exit 1
    printf '# P\nCites pkg/x.go:1 reviewed\n' > wiki/p.md
    GIT_AUTHOR_DATE='2022-01-01T00:00:00' GIT_COMMITTER_DATE='2022-01-01T00:00:00' git add wiki/p.md
    GIT_AUTHOR_DATE='2022-01-01T00:00:00' GIT_COMMITTER_DATE='2022-01-01T00:00:00' git commit -qm page-reviewed
  )
  check "wiki-lint (staleness) clears after the page is re-committed" 0 \
    "$(cd "$SWDIR" && FACTORY_CONFIG="$SWDIR/factory.yaml" run_status "$WIKI_HOOK")"
fi

# Break/fix: copy-manifest-check flags an install-copy target not tracked by git
# (the "works locally, missing in a clean clone" class the installer hit).
CM_HOOK="$HOOKS/copy-manifest-check.sh"
if [ -x "$CM_HOOK" ]; then
  CMDIR="$SANDBOX/cm"
  mkdir -p "$CMDIR/scripts" "$CMDIR/foo"
  (
    cd "$CMDIR" || exit 1
    git init -q -b main
    git config user.email c@e.i
    git config user.name c
    printf 'cp "$TEMPLATE_DIR/foo/bar.txt" "$TARGET_DIR/foo/"\n' > scripts/factory-init.sh
    printf 'x\n' > foo/bar.txt
    git add scripts/factory-init.sh && git commit -qm init
  )
  check "copy-manifest-check flags an untracked copy target" 1 \
    "$(run_status "$CM_HOOK" "$CMDIR")"
  ( cd "$CMDIR" && git add foo/bar.txt && git commit -qm track-bar )
  check "copy-manifest-check passes when the target is tracked" 0 \
    "$(run_status "$CM_HOOK" "$CMDIR")"
fi

# Break/fix: commit-message-lint matches claim words at word boundaries — the
# word "frameworks" must not read as a "works" claim, but a bare one still must.
CML_HOOK="$HOOKS/commit-message-lint.sh"
if [ -x "$CML_HOOK" ]; then
  check "commit-lint passes 'frameworks' (not a works claim)" 0 \
    "$(printf 'feat: framework awareness\n\n- frameworks ride on language packs\n' | run_status "$CML_HOOK")"
  check "commit-lint flags a bare 'works' claim" 1 \
    "$(printf 'fix: the retry logic works\n' | run_status "$CML_HOOK")"
fi

# Break/fix: sync routes each role to its tier's per-harness model from
# factory.config, and the standard/economy collapse is applied at sync time by
# resolve_tier — so editing factory.config (a model, or the profile) and running
# the sync re-routes every harness. With no factory.config, sync is a no-op:
# adapters inherit and opencode keeps its placeholders (committed repo stays clean).
SYNCROOT="$SANDBOX/syncroot"
mkdir -p "$SYNCROOT/scripts/lib" "$SYNCROOT/.opencode/agent"
cp "$TEMPLATE_ROOT/scripts/sync-opencode.sh" "$TEMPLATE_ROOT/scripts/sync-codex.sh" \
   "$TEMPLATE_ROOT/scripts/sync-claude.sh" "$SYNCROOT/scripts/"
cp "$TEMPLATE_ROOT/scripts/lib/roles.sh" "$SYNCROOT/scripts/lib/"
cat > "$SYNCROOT/opencode.json" <<'JSON'
{ "model": "__DEFAULT_MODEL__", "small_model": "__ECONOMY_MODEL__", "agent": {
  "reviewer": { "description": "r", "model": "__FRONTIER_MODEL__", "permission": { "edit": "deny" } },
  "implementer": { "description": "i", "model": "__DEFAULT_MODEL__" },
  "refactorer": { "description": "f", "model": "__ECONOMY_MODEL__" }
} }
JSON
for a in reviewer implementer refactorer; do
  printf -- '---\ndescription: x\nmodel: __DEFAULT_MODEL__\n---\nBody for %s\n' "$a" > "$SYNCROOT/.opencode/agent/$a.md"
done
sync_all() { ( cd "$SYNCROOT" && bash scripts/sync-opencode.sh && bash scripts/sync-codex.sh && bash scripts/sync-claude.sh ) >/dev/null 2>&1 || true; }
# No factory.config → sync is a no-op.
sync_all
check "codex inherits when no factory.config" "" \
  "$(grep -E '^model' "$SYNCROOT/.codex/agents/reviewer.toml" 2>/dev/null || true)"
check "opencode untouched when no factory.config" "__DEFAULT_MODEL__" \
  "$(jq -r '.model' "$SYNCROOT/opencode.json" 2>/dev/null || true)"
# economy profile → all three tiers distinct on every harness.
cat > "$SYNCROOT/factory.config" <<'CONF'
COST_PROFILE="economy"
OPENCODE_FRONTIER_MODEL="openrouter/z-ai/glm-5.2"
OPENCODE_DEFAULT_MODEL="openrouter/z-ai/glm-5.2"
OPENCODE_ECONOMY_MODEL="openrouter/qwen/qwen3-coder"
CODEX_FRONTIER_MODEL="gpt-5.6-sol"
CODEX_DEFAULT_MODEL="gpt-5.6-terra"
CODEX_ECONOMY_MODEL="gpt-5.6-luna"
CLAUDE_FRONTIER_MODEL="claude-opus-4-8"
CLAUDE_DEFAULT_MODEL="claude-sonnet-4-6"
CLAUDE_ECONOMY_MODEL="claude-haiku-4-5"
CONF
sync_all
check "codex frontier role -> sol" 'model = "gpt-5.6-sol"' \
  "$(grep -E '^model' "$SYNCROOT/.codex/agents/reviewer.toml" 2>/dev/null || true)"
check "codex economy role -> luna" 'model = "gpt-5.6-luna"' \
  "$(grep -E '^model' "$SYNCROOT/.codex/agents/refactorer.toml" 2>/dev/null || true)"
check "claude economy role -> haiku" "model: claude-haiku-4-5" \
  "$(grep -E '^model:' "$SYNCROOT/.claude/agents/refactorer.md" 2>/dev/null || true)"
check "sync-opencode wrote opencode.json economy model" "openrouter/qwen/qwen3-coder" \
  "$(jq -r '.agent.refactorer.model' "$SYNCROOT/opencode.json" 2>/dev/null || true)"
check "sync-opencode wrote small_model" "openrouter/qwen/qwen3-coder" \
  "$(jq -r '.small_model' "$SYNCROOT/opencode.json" 2>/dev/null || true)"
check "sync-opencode rewrote the role file" "model: openrouter/qwen/qwen3-coder" \
  "$(grep -E '^model:' "$SYNCROOT/.opencode/agent/refactorer.md" 2>/dev/null || true)"
# Flip the profile to standard → economy roles collapse to default everywhere.
sed -i.bak 's/COST_PROFILE="economy"/COST_PROFILE="standard"/' "$SYNCROOT/factory.config"
rm -f "$SYNCROOT/factory.config.bak"
sync_all
check "flip standard: codex economy role collapses to terra" 'model = "gpt-5.6-terra"' \
  "$(grep -E '^model' "$SYNCROOT/.codex/agents/refactorer.toml" 2>/dev/null || true)"
check "flip standard: claude economy role collapses to sonnet" "model: claude-sonnet-4-6" \
  "$(grep -E '^model:' "$SYNCROOT/.claude/agents/refactorer.md" 2>/dev/null || true)"
check "flip standard: opencode economy role collapses to glm" "openrouter/z-ai/glm-5.2" \
  "$(jq -r '.agent.refactorer.model' "$SYNCROOT/opencode.json" 2>/dev/null || true)"
# A blank opencode frontier/economy value falls back to the default tier rather
# than crashing under set -u (the distinctive default proves the sync ran).
cat > "$SYNCROOT/factory.config" <<'CONF'
COST_PROFILE="economy"
OPENCODE_DEFAULT_MODEL="fallback-model"
OPENCODE_FRONTIER_MODEL=""
OPENCODE_ECONOMY_MODEL=""
CONF
( cd "$SYNCROOT" && bash scripts/sync-opencode.sh ) >/dev/null 2>&1 || true
check "opencode blank tier falls back to default (no crash)" "fallback-model" \
  "$(jq -r '.agent.reviewer.model' "$SYNCROOT/opencode.json" 2>/dev/null || true)"
# Guard: a cross-provider slug or unresolved placeholder is not a valid native
# Codex/Claude model, so the sync scripts fall back to inherit rather than emit it.
cat > "$SYNCROOT/factory.config" <<'CONF'
CODEX_FRONTIER_MODEL="openrouter/z-ai/glm-5.2"
CLAUDE_FRONTIER_MODEL="__FRONTIER_MODEL__"
CONF
( cd "$SYNCROOT" && bash scripts/sync-codex.sh && bash scripts/sync-claude.sh ) >/dev/null 2>&1 || true
check "codex omits a cross-provider slug (inherit)" "" \
  "$(grep -E '^model' "$SYNCROOT/.codex/agents/reviewer.toml" 2>/dev/null || true)"
check "claude falls back to inherit on a placeholder" "model: inherit" \
  "$(grep -E '^model:' "$SYNCROOT/.claude/agents/reviewer.md" 2>/dev/null || true)"

# Break/fix: a gate firing records an event, and `factory report` reads it back
# (facts + one labeled estimate, never a "tokens saved" headline). --clear resets.
: > "$FACTORY_EVENT_LOG"
printf 'refs/heads/main a refs/heads/main b\n' | "$HOOKS/direct-main-push-block.sh" >/dev/null 2>&1 || true
check "a gate firing logs an event" "1" \
  "$(grep -c 'direct-main-push-block' "$FACTORY_EVENT_LOG" 2>/dev/null || echo 0)"
REPORT_OUT="$("$TEMPLATE_ROOT/scripts/factory-report.sh" 2>/dev/null || true)"
check "factory report shows the block" "1" \
  "$(printf '%s\n' "$REPORT_OUT" | grep -c 'direct-main-push-block' || echo 0)"
check "factory report refuses a tokens-saved headline" "0" \
  "$(printf '%s\n' "$REPORT_OUT" | grep -ic 'tokens saved:' || true)"
"$TEMPLATE_ROOT/scripts/factory-report.sh" --clear >/dev/null 2>&1 || true
check "factory report --clear resets the log" "0" \
  "$(grep -c . "$FACTORY_EVENT_LOG" 2>/dev/null || echo 0)"

echo ""
echo "selftest: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
