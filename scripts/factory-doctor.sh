#!/usr/bin/env bash
set -uo pipefail

# scripts/factory-doctor.sh (Decision 14)
# Health report for an installed software factory. Answers the question an
# adopter actually has: "are the gates live in my repo, or just installed?"
#
# It classifies every gate as armed / inert / stale from factory.yaml, checks
# that the hook scripts and generated adapters are intact, checks that the
# protected paths are covered by CODEOWNERS, and finally runs the break/fix
# self-test so you watch each gate fire. Honest by construction: a gate you
# have not armed is reported inert, not hidden.
#
# Exit 0 = healthy (inert gates are a choice, not a failure).
# Exit 1 = something is broken: a missing/unexecutable hook, adapter drift, or
#          a failing break/fix proof.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT" || exit 1

# shellcheck source=lib/config.sh
. "$SCRIPT_DIR/lib/config.sh"

FAIL=0
WARN=0

line() { printf '  %-9s %s\n' "$1" "$2"; }
armed() { line "[ARMED]" "$1"; }
inert() { line "[inert]" "$1"; }
stale() { line "[STALE]" "$1"; WARN=$((WARN + 1)); }
ok()    { line "[ ok ]" "$1"; }
warn()  { line "[warn]" "$1"; WARN=$((WARN + 1)); }
fail()  { line "[FAIL]" "$1"; FAIL=$((FAIL + 1)); }

CFG="$(factory_config_file)"
echo "factory doctor"
echo "  repo:   $ROOT"
echo "  config: $CFG"
if [ ! -f "$CFG" ]; then
  echo
  fail "factory.yaml not found — run 'factory init' first"
  echo
  echo "factory doctor: 1 problem"
  exit 1
fi

TFP="$(factory_config_get test_file_patterns)"
CP="$(factory_config_get citation_prefix)"
CC="$(factory_config_get check_command)"
PP="$(factory_config_get protected_paths)"
DL="$(factory_config_get decision_log)"
LP="$(factory_config_get language_packs)"
DR="$(factory_config_get docs_root)"
WR="$(factory_config_get wiki_root wiki)"

echo
echo "Gates"

# test-edit-denial (generator/evaluator separation)
if [ -n "$TFP" ]; then
  armed "test-edit-denial       implementer cannot edit: $TFP"
else
  inert "test-edit-denial       no test_file_patterns set — the implementer can edit tests"
fi

# citation-lint (opt-in)
if [ -n "$CP" ]; then
  if [ -n "$DR" ] && [ -d "$DR" ]; then
    armed "citation-lint          resolves ${CP}*.md citations against $DR/"
  else
    warn  "citation-lint          citation_prefix set but docs_root '$DR' is missing"
  fi
else
  inert "citation-lint          no citation_prefix set (opt-in)"
fi

# diff-aware-check
if [ -n "$CC" ]; then
  if [ -f memory/.parity-stale ]; then
    stale "diff-aware-check       an OBSERVED parity claim is stale (memory/.parity-stale)"
  else
    armed "diff-aware-check       re-verifies via: $CC"
  fi
else
  inert "diff-aware-check       no check_command set — nothing re-verified on change"
fi

# decision-log-gate
if [ -n "$DL" ]; then
  if [ -n "$PP" ]; then
    armed "decision-log-gate      governance surfaces + protected_paths ($PP) need a Decision"
  else
    armed "decision-log-gate      factory surfaces need a Decision (no protected_paths set)"
  fi
else
  warn  "decision-log-gate      no decision_log configured"
fi

# always-on gates
armed "commit-message-lint    verification-claim + conventional-commit lint"
armed "direct-main-push-block rejects pushes to main (local gate; pair with branch protection)"

if [ -f memory/PENDING-LESSONS.md ]; then
  stale "pending-lessons        memory/PENDING-LESSONS.md is unaddressed — push is blocked"
else
  armed "pending-lessons        clears once session lessons are written"
fi

if [ -d .opencode/plugin ]; then
  armed "shared-script-enforce  adapters must call scripts/hooks, not reimplement them"
else
  inert "shared-script-enforce  no .opencode/plugin present"
fi

# wiki-lint (the LLM-wiki pattern's lint operation)
if [ -d "$WR" ] && find "$WR" -type f -name '*.md' ! -name 'README.md' ! -name 'INDEX.md' 2>/dev/null | grep -q .; then
  armed "wiki-lint              every wiki/ content page must cite a source and resolve its links"
else
  inert "wiki-lint              no wiki content pages yet (wiki_root: $WR)"
fi

# pack dialect gate (only when a pack is installed)
if [ -n "$LP" ]; then
  for lang in $LP; do
    case "$lang" in
      go)         PH="scripts/hooks/ginkgo-only-check.sh"; DESC="Go tests use Ginkgo/Gomega" ;;
      java)       PH="scripts/hooks/junit5-only-check.sh"; DESC="Java tests use JUnit 5" ;;
      typescript) PH="scripts/hooks/vitest-only-check.sh"; DESC="TS tests use Vitest" ;;
      *)          PH=""; DESC="" ;;
    esac
    [ -z "$PH" ] && continue
    if [ -x "$PH" ]; then
      armed "pack:$lang dialect gate  $DESC"
    else
      fail "pack:$lang dialect gate  $PH missing (pack '$lang' selected but hook absent)"
    fi
  done
fi

echo
echo "Integrity"

# Hook scripts exist and are executable.
CORE_HOOKS="scripts/lib/config.sh scripts/selftest/run.sh scripts/hooks/test-edit-denial.sh \
scripts/hooks/commit-message-lint.sh scripts/hooks/decision-log-gate.sh \
scripts/hooks/diff-aware-check.sh scripts/hooks/hook-existence-check.sh \
scripts/hooks/shared-script-enforcement.sh scripts/hooks/direct-main-push-block.sh \
scripts/hooks/pending-lessons-push-block.sh scripts/citation-lint.sh"
MISSING=0
for h in $CORE_HOOKS; do
  if [ ! -f "$h" ]; then fail "$h is missing"; MISSING=$((MISSING + 1));
  elif [ ! -x "$h" ]; then fail "$h is not executable"; MISSING=$((MISSING + 1)); fi
done
[ "$MISSING" -eq 0 ] && ok "all core hook scripts present and executable"

# Adapter drift: the generated .claude/.codex must match the opencode canon.
if [ -x scripts/sync-claude.sh ] && [ -d .claude ]; then
  BEFORE="$(git status --porcelain .claude .codex .mcp.json 2>/dev/null)"
  ./scripts/sync-claude.sh >/dev/null 2>&1
  [ -x scripts/sync-codex.sh ] && ./scripts/sync-codex.sh >/dev/null 2>&1
  AFTER="$(git status --porcelain .claude .codex .mcp.json 2>/dev/null)"
  if [ "$BEFORE" = "$AFTER" ]; then
    ok "harness adapters match the opencode canon (no drift)"
  else
    warn "harness adapters drifted — run 'make sync-harnesses' and commit"
  fi
else
  line "[skip]" "adapter drift (sync scripts or .claude not present)"
fi

# CODEOWNERS covers protected paths.
if [ -n "$PP" ]; then
  CO=".github/CODEOWNERS"
  if [ ! -f "$CO" ]; then
    warn "protected_paths set but .github/CODEOWNERS is missing"
  elif grep -q '__PROTECTED_PATH__' "$CO"; then
    line "[skip]" "CODEOWNERS still holds the template placeholder (not a substituted adoption)"
  else
    UNCOVERED=""
    for p in $PP; do grep -q -- "$p" "$CO" || UNCOVERED="$UNCOVERED $p"; done
    if [ -z "$UNCOVERED" ]; then
      ok "CODEOWNERS references every protected path"
    else
      warn "protected path(s) not in CODEOWNERS:$UNCOVERED"
    fi
  fi
fi

echo
echo "Proof (break/fix self-test)"
if [ -x scripts/selftest/run.sh ]; then
  ST_OUT="$(scripts/selftest/run.sh 2>&1)"
  ST_STATUS=$?
  ST_TALLY="$(printf '%s\n' "$ST_OUT" | grep -E '^selftest:' || true)"
  if [ "$ST_STATUS" -eq 0 ]; then
    ok "${ST_TALLY:-every gate fired on its violation and passed clean}"
  else
    fail "${ST_TALLY:-break/fix self-test failed}"
    printf '%s\n' "$ST_OUT" | sed 's/^/    /'
  fi
else
  fail "scripts/selftest/run.sh is missing — cannot prove the gates fire"
fi

echo
if [ "$FAIL" -gt 0 ]; then
  echo "factory doctor: $FAIL problem(s), $WARN warning(s) — the factory is not fully sound"
  exit 1
fi
if [ "$WARN" -gt 0 ]; then
  echo "factory doctor: healthy, $WARN warning(s) to review (inert gates are a choice, not a fault)"
  exit 0
fi
echo "factory doctor: healthy — every armed gate is live and proven"
exit 0
