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

# Break/fix: a gate that can block must be able to say so. Found in the field —
# a repo reported 14 gates installed and 0 blocks while 8 of those gates had no
# way to record one, so the report read as calm when it was merely deaf.
GI_HOOK="$HOOKS/gate-instrumentation-check.sh"
if [ -x "$GI_HOOK" ]; then
  GIDIR="$SANDBOX/gi"
  mkdir -p "$GIDIR/scripts/hooks" "$GIDIR/scripts/lib" "$GIDIR/packs/x/hooks"
  cp "$TEMPLATE_ROOT/scripts/lib/events.sh" "$GIDIR/scripts/lib/"
  printf '#!/bin/bash\necho blocked\nexit 1\n' > "$GIDIR/scripts/hooks/mute-gate.sh"
  check "a blocking gate that logs nothing is caught" 1 \
    "$(run_status "$GI_HOOK" "$GIDIR")"
  # Calling the function without sourcing the lib is the subtler mute: the call
  # is present, so a grep for it passes, but no event is ever written.
  printf '#!/bin/bash\nfactory_log_event g r\nexit 1\n' > "$GIDIR/scripts/hooks/mute-gate.sh"
  check "a gate calling the logger without sourcing it is caught" 1 \
    "$(run_status "$GI_HOOK" "$GIDIR")"
  printf '#!/bin/bash\n. "$(dirname "$0")/../lib/events.sh"\nfactory_log_event g r\nexit 1\n' \
    > "$GIDIR/scripts/hooks/mute-gate.sh"
  check "an instrumented blocking gate passes" 0 \
    "$(run_status "$GI_HOOK" "$GIDIR")"
  # Advisory scripts never block, so they have nothing to report.
  printf '#!/bin/bash\necho advisory\nexit 0\n' > "$GIDIR/scripts/hooks/mute-gate.sh"
  check "an advisory script is not required to log" 0 \
    "$(run_status "$GI_HOOK" "$GIDIR")"
  # A gate whose non-zero exit stops no work opts out where it lives, so the
  # reason travels with the code rather than sitting in a list that drifts.
  printf '#!/bin/bash\n# factory: no-block-event — nudge only\necho nudge\nexit 1\n' \
    > "$GIDIR/scripts/hooks/mute-gate.sh"
  check "a declared non-blocking exit is exempt" 0 \
    "$(run_status "$GI_HOOK" "$GIDIR")"
  # Pack gates install into scripts/hooks/ and block like any other.
  printf '#!/bin/bash\necho blocked\nexit 1\n' > "$GIDIR/packs/x/hooks/pack-gate.sh"
  check "a mute pack gate is caught too" 1 \
    "$(run_status "$GI_HOOK" "$GIDIR")"
  rm -f "$GIDIR/packs/x/hooks/pack-gate.sh"

  # Every gate this template ships is instrumented, checked against the real
  # tree rather than a fixture — that is the invariant adopters inherit.
  check "every shipped blocking gate is instrumented" 0 \
    "$(run_status "$GI_HOOK" "$TEMPLATE_ROOT")"
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
# Provider "inherit": blank tiers mean remove every model pin, so opencode falls
# back to its own config. Leaving an unresolved placeholder would be read as a
# model name, so stripping — not skipping — is the correct behaviour.
printf 'COST_PROFILE="standard"\nOPENCODE_DEFAULT_MODEL=""\n' > "$SYNCROOT/factory.config"
( cd "$SYNCROOT" && bash scripts/sync-opencode.sh ) >/dev/null 2>&1 || true
check "opencode inherit strips the top-level model pin" "null" \
  "$(jq -r '.model // "null"' "$SYNCROOT/opencode.json" 2>/dev/null || true)"
check "opencode inherit strips per-agent model pins" "null" \
  "$(jq -r '.agent.reviewer.model // "null"' "$SYNCROOT/opencode.json" 2>/dev/null || true)"
check "opencode inherit leaves no placeholder behind" "0" \
  "$(grep -c '__.*MODEL__' "$SYNCROOT/opencode.json" 2>/dev/null || true)"
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

# Break/fix: factory upgrade ADDS a new framework lib, not just refreshes existing
# files. A repo installed before a lib existed must receive it, or the shipped
# scripts that source it break (the roles.sh/events.sh upgrade regression).
UPGROOT="$SANDBOX/upgroot"
mkdir -p "$UPGROOT/scripts/lib"
( cd "$UPGROOT" && git init -q )
printf 'project_name: t\nlanguage_packs: ""\n' > "$UPGROOT/factory.yaml"
cp "$TEMPLATE_ROOT/scripts/lib/config.sh" "$UPGROOT/scripts/lib/"   # roles.sh absent
# Marked as nested so the upgrade skips its doctor proof. Without this the
# cycle is doctor -> selftest -> upgrade -> doctor, which never terminates:
# the guard inside upgrade only helps when the OUTER process is an upgrade.
( cd "$UPGROOT" && FACTORY_UPGRADE_ACTIVE=1 bash "$TEMPLATE_ROOT/scripts/factory-upgrade.sh" --source "$TEMPLATE_ROOT" ) >/dev/null 2>&1 || true
check "factory upgrade adds a missing framework lib" "yes" \
  "$([ -f "$UPGROOT/scripts/lib/roles.sh" ] && echo yes || echo no)"

# Break/fix: pack dialect gates are upgradeable. They are the one thing the
# template stores somewhere other than where the adopter keeps it — upstream in
# packs/<lang>/hooks/, installed to scripts/hooks/ — so the copy needs an
# explicit source. It did not have one, the file check found nothing at the
# assumed path and returned success, and pack gates were silently never
# upgraded: an adopter kept whatever gate they installed with, forever.
PKGROOT="$SANDBOX/packupg"
mkdir -p "$PKGROOT/scripts/hooks" "$PKGROOT/scripts/lib"
( cd "$PKGROOT" && git init -q )
printf 'project_name: t\nlanguage_packs: "typescript"\n' > "$PKGROOT/factory.yaml"
cp "$TEMPLATE_ROOT/scripts/lib/config.sh" "$PKGROOT/scripts/lib/"
# A stale copy of the gate, standing in for one installed by an older release.
printf '#!/bin/bash\n# stale pack gate\nexit 0\n' > "$PKGROOT/scripts/hooks/vitest-only-check.sh"
( cd "$PKGROOT" && FACTORY_UPGRADE_ACTIVE=1 bash "$TEMPLATE_ROOT/scripts/factory-upgrade.sh" --source "$TEMPLATE_ROOT" ) >/dev/null 2>&1 || true
if grep -q 'stale pack gate' "$PKGROOT/scripts/hooks/vitest-only-check.sh" 2>/dev/null; then
  PKG_STALE=yes
else
  PKG_STALE=no
fi
check "factory upgrade refreshes a stale pack gate" "no" "$PKG_STALE"
# And the refreshed gate is the real one, instrumentation included — proving the
# content arrived rather than the file merely being touched.
#
# grep -q inside an if, not a counting pipeline: this suite runs under
# `set -euo pipefail`, where a grep that legitimately matches nothing exits 1
# and takes the whole run down — precisely in the case this is here to detect.
if grep -q 'factory_log_event' "$PKGROOT/scripts/hooks/vitest-only-check.sh" 2>/dev/null; then
  PKG_INSTRUMENTED=yes
else
  PKG_INSTRUMENTED=no
fi
check "the upgraded pack gate can report a block" "yes" "$PKG_INSTRUMENTED"

# Break/fix: the golden-task eval scores a real run — the reference task passes
# when solved, fails when unsolved, catches a runner that tampers the oracle, and
# flags a regression from a saved baseline. The mock runner keeps it credential-free.
GEROOT="$SANDBOX/geval"
mkdir -p "$GEROOT/eval/golden-tasks/reference-answer" "$GEROOT/eval/runners" "$GEROOT/scripts"
cp "$TEMPLATE_ROOT/scripts/golden-task-eval.sh" "$GEROOT/scripts/"
# Self-contained task + mock runner, so the fixture depends only on the framework
# script (not on the repo's shipped example files, which an upgrade may not add).
printf 'Create answer.txt containing FIXED.\n' > "$GEROOT/eval/golden-tasks/reference-answer/task.md"
printf '#!/bin/sh\ngrep -qx FIXED answer.txt 2>/dev/null\n' > "$GEROOT/eval/golden-tasks/reference-answer/verify.sh"
cat > "$GEROOT/eval/runners/mock.sh" <<'MOCK'
#!/bin/sh
cd "$1" || exit 0
case "${FACTORY_MOCK_MODE:-pass}" in
  pass) printf 'FIXED\n' > answer.txt ;;
  fail) : ;;
  cheat) printf 'FIXED\n' > answer.txt; : > verify.sh ;;
esac
MOCK
chmod +x "$GEROOT/eval/runners/mock.sh"
geval_score() { ( cd "$GEROOT" && FACTORY_MOCK_MODE="$1" ./scripts/golden-task-eval.sh 2>/dev/null | grep reference-answer | grep -oE 'score [0-9.]+' | awk '{print $2}' ); }
geval_exit() { ( cd "$GEROOT" && FACTORY_MOCK_MODE="$1" ./scripts/golden-task-eval.sh >/dev/null 2>&1; echo $? ); }
check "eval scores a solved task as pass" "1.00" "$(geval_score pass)"
check "eval scores an unsolved task as fail" "0.00" "$(geval_score fail)"
check "eval catches a runner tampering the oracle" "0.00" "$(geval_score cheat)"
( cd "$GEROOT" && FACTORY_MOCK_MODE=pass ./scripts/golden-task-eval.sh --save-baseline >/dev/null 2>&1 )
check "eval flags a regression from baseline" "1" "$(geval_exit fail)"
check "eval passes when no regression" "0" "$(geval_exit pass)"
# Staleness: a saved score only means something against the inputs it was measured
# on. Change the oracle and the eval must say STALE — not "no regression", which
# would claim something it cannot know (the score itself is unchanged here).
printf '#!/bin/sh\n# oracle changed\ngrep -qx FIXED answer.txt 2>/dev/null\n' > "$GEROOT/eval/golden-tasks/reference-answer/verify.sh"
check "eval flags a stale baseline when the oracle changes" "1" "$(geval_exit pass)"
check "eval names why the baseline went stale" "1" \
  "$( ( cd "$GEROOT" && FACTORY_MOCK_MODE=pass ./scripts/golden-task-eval.sh 2>&1 || true ) | grep -c 'oracle changed' || true )"
check "eval does not claim no-regression when stale" "0" \
  "$( ( cd "$GEROOT" && FACTORY_MOCK_MODE=pass ./scripts/golden-task-eval.sh 2>&1 || true ) | grep -c 'no regression from baseline' || true )"
# A headless agent can hang rather than fail (a subagent waiting on an "ask"
# permission nothing services). The eval must cap the run, not wedge on it.
printf '#!/bin/sh\nsleep 30\n' > "$GEROOT/eval/runners/hang.sh"
chmod +x "$GEROOT/eval/runners/hang.sh"
check "eval caps a hung runner instead of wedging" "1" \
  "$( ( cd "$GEROOT" && ./scripts/golden-task-eval.sh --runner=eval/runners/hang.sh --timeout=2 2>&1 || true ) | grep -c 'hit the 2s cap' || true )"

# Break/fix: workflow-lint enforces graph hygiene on recipes — a clean recipe
# passes; a plumbing node (merge) run as an agent fails (coordination is code).
WLROOT="$SANDBOX/wflint/workflows"
mkdir -p "$WLROOT"
printf '# W\n## review\n- role: reviewer\n- kind: fanout\n- over: files\n## verify\n- role: reviewer\n- kind: verify\n' > "$WLROOT/good.md"
check "workflow-lint passes a clean recipe" 0 \
  "$(run_status "$HOOKS/workflow-lint.sh" "$WLROOT")"
printf '# W\n## merge\n- role: reviewer\n- kind: agent\n## verify\n- role: reviewer\n- kind: verify\n' > "$WLROOT/bad.md"
check "workflow-lint flags a plumbing node run as an agent" 1 \
  "$(run_status "$HOOKS/workflow-lint.sh" "$WLROOT")"

# Break/fix: an installed .githooks/pre-push is not proof git will run it. A
# core.hooksPath pointing elsewhere (commonly inherited from global config)
# silently makes the push gate inert — factory doctor must detect that.
# shellcheck source=../lib/hookspath.sh
. "$TEMPLATE_ROOT/scripts/lib/hookspath.sh"
HPROOT="$SANDBOX/hookspath"
mkdir -p "$HPROOT/.githooks"
# Hermetic: a developer's global core.hooksPath leaks into every fresh repo (that
# is the hazard being tested), so ignore global/system config to make the three
# states deterministic on any machine.
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
( cd "$HPROOT" && git init -q )
printf '#!/bin/sh\nexit 0\n' > "$HPROOT/.githooks/pre-push"
chmod +x "$HPROOT/.githooks/pre-push"
check "hookspath: absent when core.hooksPath is unset" "absent" \
  "$(hookspath_status "$HPROOT" | cut -f1)"
# Break/fix: enforcement must not depend on bookkeeping. An older repo whose
# hooks were refreshed before lib/events.sh shipped has no events.sh — the gate
# must still deny and allow correctly rather than error on the missing source.
NOEV="$SANDBOX/no-events/scripts"
mkdir -p "$NOEV/hooks" "$NOEV/lib"
cp "$HOOKS/direct-main-push-block.sh" "$NOEV/hooks/"
cp "$TEMPLATE_ROOT/scripts/lib/config.sh" "$NOEV/lib/"   # deliberately no events.sh
# Break/fix: a repo-local hook is registered in factory.yaml, not by editing
# hook-existence-check.sh — that is a framework file upgrade overwrites, so a
# hand-added entry would vanish. Registered-but-missing must fail; present passes.
LHROOT="$SANDBOX/localhooks"
mkdir -p "$LHROOT/scripts/hooks" "$LHROOT/scripts/lib"
( cd "$LHROOT" && git init -q )
cp "$HOOKS/hook-existence-check.sh" "$LHROOT/scripts/hooks/"
cp "$TEMPLATE_ROOT/scripts/lib/config.sh" "$LHROOT/scripts/lib/"
printf 'project_name: t\nlocal_hooks: "scripts/hooks/my-gate.sh"\n' > "$LHROOT/factory.yaml"
check "local_hooks flags a registered hook that is missing" "1" \
  "$( ( cd "$LHROOT" && ./scripts/hooks/hook-existence-check.sh 2>&1 || true ) | grep -c 'my-gate.sh.*missing\|missing.*my-gate.sh' || true )"
printf '#!/bin/sh\nexit 0\n' > "$LHROOT/scripts/hooks/my-gate.sh"
chmod +x "$LHROOT/scripts/hooks/my-gate.sh"
( cd "$LHROOT" && git add -A >/dev/null 2>&1 )
check "local_hooks passes once the hook exists" "1" \
  "$( ( cd "$LHROOT" && ./scripts/hooks/hook-existence-check.sh 2>&1 || true ) | grep -c 'OK scripts/hooks/my-gate.sh' || true )"
printf 'project_name: t\nlocal_hooks: ""\n' > "$LHROOT/factory.yaml"
check "an unregistered local hook is not checked" "0" \
  "$( ( cd "$LHROOT" && ./scripts/hooks/hook-existence-check.sh 2>&1 || true ) | grep -c 'my-gate' || true )"

check "hook denies with events.sh missing" 1 \
  "$(printf 'refs/heads/main a refs/heads/main b\n' | run_status "$NOEV/hooks/direct-main-push-block.sh")"
check "hook allows with events.sh missing" 0 \
  "$(printf 'refs/heads/feat a refs/heads/feat b\n' | run_status "$NOEV/hooks/direct-main-push-block.sh")"
( cd "$HPROOT" && git config core.hooksPath .githooks )
check "hookspath: armed when it points at .githooks" "armed" \
  "$(hookspath_status "$HPROOT" | cut -f1)"
( cd "$HPROOT" && git config core.hooksPath "$SANDBOX/elsewhere-hooks" )
check "hookspath: hijacked when it points elsewhere" "hijacked" \
  "$(hookspath_status "$HPROOT" | cut -f1)"
unset GIT_CONFIG_GLOBAL GIT_CONFIG_SYSTEM

# Break/fix: the review lane is opt-in. Enabling installs the workflow and wires
# the provider's secret name into it; disabling REMOVES the file rather than
# leaving a dormant pull_request_target workflow in the repository.
RLROOT="$SANDBOX/reviewlane"
mkdir -p "$RLROOT/scripts" "$RLROOT/packs/review-lane"
( cd "$RLROOT" && git init -q )
cp "$TEMPLATE_ROOT/scripts/factory-review-lane.sh" "$RLROOT/scripts/"
cp "$TEMPLATE_ROOT/packs/review-lane/review-pr.yml" "$RLROOT/packs/review-lane/"
printf 'MODEL_PROVIDER="anthropic"\nREVIEW_LANE="off"\n' > "$RLROOT/factory.config"
check "review lane is off until asked for" "0" \
  "$([ -f "$RLROOT/.github/workflows/adversarial-review.yml" ] && echo 1 || echo 0)"
( cd "$RLROOT" && ./scripts/factory-review-lane.sh enable ) >/dev/null 2>&1 || true
check "enabling installs the workflow" "1" \
  "$([ -f "$RLROOT/.github/workflows/adversarial-review.yml" ] && echo 1 || echo 0)"
check "the provider's secret name is wired in" "0" \
  "$(grep -c '__REVIEW_API_KEY_SECRET__' "$RLROOT/.github/workflows/adversarial-review.yml" || true)"
check "a fork PR cannot drive the privileged job" "1" \
  "$(grep -c 'head.repo.full_name == github.repository' "$RLROOT/.github/workflows/adversarial-review.yml" || true)"
( cd "$RLROOT" && ./scripts/factory-review-lane.sh disable ) >/dev/null 2>&1 || true
check "disabling removes the workflow, not just the flag" "0" \
  "$([ -f "$RLROOT/.github/workflows/adversarial-review.yml" ] && echo 1 || echo 0)"

# Break/fix: an opt-in capability is offered once. The config key's PRESENCE is
# the record — "off" is a decision that was made — so a repo that answered is
# never asked again, and a repo that never has is still told.
OFFROOT="$SANDBOX/capoffer"
mkdir -p "$OFFROOT/packs/review-lane" "$OFFROOT/scripts"
( cd "$OFFROOT" && git init -q )
cp "$TEMPLATE_ROOT/packs/review-lane/review-pr.yml" "$OFFROOT/packs/review-lane/"
printf 'project_name: t\n' > "$OFFROOT/factory.yaml"
cap_offer_output() {
  printf 'PROJECT_NAME="t"\n%s' "$1" > "$OFFROOT/factory.config"
  ( cd "$OFFROOT" && FACTORY_UPGRADE_ACTIVE=1 bash "$TEMPLATE_ROOT/scripts/factory-upgrade.sh" --source "$TEMPLATE_ROOT" 2>&1 || true ) | grep -c 'New, and off' || true
}
check "a repo never offered the capability is told" "1" "$(cap_offer_output '')"
# The offer is guarded on the workflow template existing, so upgrade must SHIP
# it — otherwise the capability is announced to nobody, which is how it shipped
# broken the first time.
check "upgrade ships the workflow template the offer depends on" "1" \
  "$(grep -c '^packs/review-lane/review-pr.yml$' "$TEMPLATE_ROOT/scripts/factory-upgrade.sh" || true)"
# Upgrade runs doctor, doctor runs the self-test, and the self-test runs upgrade.
# Without a re-entrancy guard that recurses until the machine gives up.
check "upgrade guards against recursing into itself" "1" \
  "$(grep -c 'UPGRADE_NESTED' "$TEMPLATE_ROOT/scripts/factory-upgrade.sh" >/dev/null && echo 1 || echo 0)"

# The cycle doctor -> selftest -> upgrade -> doctor also has to be cut, and the
# guard inside upgrade cannot do it: it only helps when the OUTER process is an
# upgrade. The self-test must mark the upgrades it spawns as nested.
check "selftest marks the upgrades it spawns as nested" "1" \
  "$(grep -c 'FACTORY_UPGRADE_ACTIVE=1 bash "$TEMPLATE_ROOT/scripts/factory-upgrade.sh"' "$TEMPLATE_ROOT/scripts/selftest/run.sh" >/dev/null && echo 1 || echo 0)"
# The colour helper is sourced by init and upgrade, so upgrade must ship it —
# under set -u a missing lib aborted the run the first time this shipped.
check "upgrade ships the colour lib it sources" "1" \
  "$(grep -c '^scripts/lib/color.sh$' "$TEMPLATE_ROOT/scripts/factory-upgrade.sh" || true)"
# Outstanding work is reported from live state: named when the lane is on and the
# secret is not confirmed, silent when the lane is off.
PENDROOT="$SANDBOX/pending"
mkdir -p "$PENDROOT/scripts/lib" "$PENDROOT/packs/review-lane"
( cd "$PENDROOT" && git init -q )
cp "$TEMPLATE_ROOT/scripts/factory-review-lane.sh" "$PENDROOT/scripts/"
cp "$TEMPLATE_ROOT/scripts/lib/color.sh" "$PENDROOT/scripts/lib/"
cp "$TEMPLATE_ROOT/packs/review-lane/review-pr.yml" "$PENDROOT/packs/review-lane/"
printf 'project_name: t\n' > "$PENDROOT/factory.yaml"
printf 'MODEL_PROVIDER="openrouter"\nREVIEW_LANE="on"\nREVIEW_API_KEY_SECRET="OPENROUTER_API_KEY"\n' > "$PENDROOT/factory.config"
check "pending names the secret when the lane is on" "1" \
  "$( ( cd "$PENDROOT" && ./scripts/factory-review-lane.sh pending 2>/dev/null || true ) | grep -c 'OPENROUTER_API_KEY' || true )"
printf 'MODEL_PROVIDER="openrouter"\nREVIEW_LANE="off"\n' > "$PENDROOT/factory.config"
check "pending is silent when the lane is off" "0" \
  "$( ( cd "$PENDROOT" && ./scripts/factory-review-lane.sh pending 2>/dev/null || true ) | grep -c . || true )"
check "a repo that declined is not asked again" "0" "$(cap_offer_output 'REVIEW_LANE="off"
')"
check "a repo that enabled it is not asked again" "0" "$(cap_offer_output 'REVIEW_LANE="on"
')"

# Break/fix: metrics are local-only and honest. The JSON must carry a schema and
# an explicit not-measured list, the HTML must have its data injected (not left
# as a placeholder), and nothing may claim to have been transmitted anywhere.
MROOT="$SANDBOX/metrics"
mkdir -p "$MROOT/scripts/lib" "$MROOT/templates" "$MROOT/.factory"
( cd "$MROOT" && git init -q && git config user.email m@e && git config user.name m
  printf 'x\n' > f.txt && git add -A && git commit -qm "feat: seed" )
cp "$TEMPLATE_ROOT/scripts/factory-metrics.sh" "$MROOT/scripts/"
cp "$TEMPLATE_ROOT/scripts/lib/config.sh" "$TEMPLATE_ROOT/scripts/lib/color.sh" "$TEMPLATE_ROOT/scripts/lib/events.sh" "$MROOT/scripts/lib/"
cp "$TEMPLATE_ROOT/templates/metrics.html" "$MROOT/templates/"
printf 'project_name: m\n' > "$MROOT/factory.yaml"
MLOG="$MROOT/.factory/events.log"
printf '2026-01-01T00:00:00Z\tcommit-message-lint\tviolation\n' > "$MLOG"
# This suite exports FACTORY_EVENT_LOG globally, so every metrics run below must
# point at this fixture's own log. Without it the reads and the writes address
# different files and the checks pass by measuring nothing.
metrics_run() { ( cd "$MROOT" && FACTORY_EVENT_LOG="$MLOG" ./scripts/factory-metrics.sh "$@" ); }
check "metrics emit a versioned schema" "1" \
  "$( metrics_run --json 2>/dev/null | grep -c 'factory.metrics/v1' || true )"
check "metrics state what they do NOT measure" "1" \
  "$( metrics_run --json 2>/dev/null | grep -c 'not_measured' || true )"
metrics_run --html >/dev/null 2>&1 || true
check "metrics html has its data injected" "0" \
  "$(grep -c '__FACTORY_METRICS_JSON__' "$MROOT/.factory/metrics.html" 2>/dev/null || true)"
check "metrics html is self-contained (no external requests)" "0" \
  "$(grep -cE 'src="https?://|href="https?://[^"]*\.(css|js)' "$MROOT/.factory/metrics.html" 2>/dev/null || true)"
# The event log is an ordinary writable file, so its contents are untrusted
# input. A gate name that looks like shell must not run while the page is being
# built, and one that looks like markup must not close the script element.
# Dated now, not with a fixed timestamp: gate names are reported for a rolling
# window, so a hardcoded date ages out and the fixture would pass by measuring
# nothing.
MARK="$MROOT/EXECUTED"
NOW="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
printf '%s\t$(touch %s)`touch %s`\tviolation\n' "$NOW" "$MARK" "$MARK" >> "$MLOG"
printf '%s\t</script><b>x</b>\tviolation\n' "$NOW" >> "$MLOG"
printf "%s\tgate-with-'''-quotes-and-\\\\n\tviolation\n" "$NOW" >> "$MLOG"
# Prove the hostile names actually reached the report, or the checks below would
# be measuring an empty window.
check "the hostile gate names are in the window" "3" \
  "$( metrics_run --json 2>/dev/null |
      grep -cE '"gate": "(\$\(touch|</script>|gate-with)' || true )"
rm -f "$MROOT/.factory/metrics.html"
HTML_RC=0
metrics_run --html >/dev/null 2>&1 || HTML_RC=$?
# Generation must survive the odd name, not just avoid executing it. A crash here
# would leave a stale page behind and every check below would read the old file.
check "an odd gate name does not break page generation" "0" "$HTML_RC"
check "a gate name cannot execute as shell" "0" \
  "$([ -e "$MARK" ] && echo 1 || echo 0)"
check "a gate name cannot close the script element" "0" \
  "$(grep -c '</script><b>' "$MROOT/.factory/metrics.html" 2>/dev/null || true)"
# Quotes and backslashes in a gate name must survive as data. JSON escapes them;
# a shell-interpolated template would corrupt or crash on them.
ODD_JSON="$( metrics_run --json 2>/dev/null || true )"
check "an odd gate name still yields parseable JSON" "1" \
  "$(printf '%s' "$ODD_JSON" | python3 -m json.tool >/dev/null 2>&1 && echo 1 || echo 0)"
check "an odd gate name survives as data, not code" "1" \
  "$(printf '%s' "$ODD_JSON" | grep -c "gate-with" || true)"

# Claims are counted per commit. A commit whose body says "fixed" on three
# bullets made one claim, and counting lines would inflate the number against a
# report that says "commits".
( cd "$MROOT" && printf 'y\n' > g.txt && git add -A &&
  git commit -qm "test: multi-line claim

- fixed one thing per \`make a\`
- fixed another per \`make b\`
- works now per \`make c\`" ) >/dev/null 2>&1
check "claims are counted per commit, not per line" "1" \
  "$( metrics_run --json 2>/dev/null |
      sed -n 's/.*"claim_commits": \([0-9]*\).*/\1/p' | head -1 )"

# The event log stays bounded rather than growing without limit.
: > "$MROOT/.factory/events.log"
( cd "$MROOT" && . scripts/lib/events.sh
  i=0; while [ $i -lt 60 ]; do FACTORY_EVENT_MAX_LINES=40 FACTORY_EVENT_LOG="$MROOT/.factory/events.log" factory_log_event g r; i=$((i+1)); done )
check "the event log is trimmed, not unbounded" "1" \
  "$([ "$(grep -c . "$MROOT/.factory/events.log")" -le 40 ] && echo 1 || echo 0)"
# A tiny cap must keep a little, not silently empty the log.
: > "$MROOT/.factory/events.log"
( cd "$MROOT" && . scripts/lib/events.sh
  i=0; while [ $i -lt 4 ]; do FACTORY_EVENT_MAX_LINES=1 FACTORY_EVENT_LOG="$MROOT/.factory/events.log" factory_log_event g r; i=$((i+1)); done )
check "a tiny cap keeps at least one event" "1" \
  "$(grep -c . "$MROOT/.factory/events.log" 2>/dev/null || true)"

# The landing page ships from this repo, so an unreplaced placeholder would go
# live as a broken analytics tag. A note would be forgotten; this is a gate.
# (Adopter repos have no index.html — the check simply does not apply there.)
if [ -f "$TEMPLATE_ROOT/index.html" ]; then
  check "landing page has no unreplaced placeholders" "0" \
    "$(grep -c '__[A-Z_]\+__' "$TEMPLATE_ROOT/index.html" || true)"
fi

echo ""
echo "selftest: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
