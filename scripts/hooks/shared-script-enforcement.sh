#!/bin/bash
set -euo pipefail

# scripts/hooks/shared-script-enforcement.sh
# Computational check: verifies that opencode, Claude Code, and Codex adapter
# surfaces call scripts/hooks/*.sh rather than reimplementing the rule.
#
# This is the hook that would have caught critical #5 (plugin contradicted
# the shared-script rule by reimplementing test-edit denial inline).
#
# Enforcement logic lives in shared shell scripts; all harnesses are thin wrappers.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../lib/events.sh
if [ -f "$SCRIPT_DIR/../lib/events.sh" ]; then . "$SCRIPT_DIR/../lib/events.sh"; else factory_log_event() { :; }; fi

PLUGIN_DIR=".opencode/plugin"

ERRORS=0

# A missing plugin directory means there is no opencode plugin to check — it does
# not mean there is nothing to check. This used to `exit 0` here, skipping the
# Claude and Codex adapter checks further down, so a repo without .opencode/
# passed with malformed adapters. Only the plugin loop is skipped now.
CHECK_PLUGIN=1
if [ ! -d "$PLUGIN_DIR" ]; then
  echo "shared-script-enforcement: no $PLUGIN_DIR — skipping the plugin checks"
  CHECK_PLUGIN=0
fi

# Known enforcement scripts that must be called from the plugin, not reimplemented.
ENFORCEMENT_SCRIPTS="test-edit-denial.sh"

# Known inline patterns that indicate reimplemented enforcement logic.
INLINE_PATTERNS="_test\.go"

# strip_ts_comments <file> — the file's code with comments removed, line for line.
#
# Line-oriented sed cannot do this. A single expression per line either deleted
# code that shared a line with a comment (`execFile(x) /* note` lost the call, and
# the hook then reported the call missing), or treated a `/*` inside a `//`
# comment as opening a block and swallowed the real code that followed. A string
# containing `//` — any URL — was truncated the same way.
#
# So track the three states that matter: inside a block comment, inside a string,
# and code. Comments become empty space; everything else is preserved verbatim.
strip_ts_comments() {
  awk '
  BEGIN { inblk = 0 }
  {
    line = $0; out = ""
    while (length(line) > 0) {
      if (inblk) {
        p = index(line, "*/")
        if (p == 0) { line = ""; break }
        line = substr(line, p + 2); inblk = 0; continue
      }
      # Earliest of: string opener, block open, line comment.
      lo = index(line, "//"); bo = index(line, "/*")
      qd = index(line, "\""); qs = index(line, "\047"); qb = index(line, "`")
      first = 0; kind = ""
      if (lo > 0)                     { first = lo; kind = "line" }
      if (bo > 0 && (first == 0 || bo < first)) { first = bo; kind = "block" }
      if (qd > 0 && (first == 0 || qd < first)) { first = qd; kind = "str"; q = "\"" }
      if (qs > 0 && (first == 0 || qs < first)) { first = qs; kind = "str"; q = "\047" }
      if (qb > 0 && (first == 0 || qb < first)) { first = qb; kind = "str"; q = "`" }
      if (first == 0) { out = out line; line = ""; break }
      out = out substr(line, 1, first - 1)
      if (kind == "line") { line = ""; break }
      if (kind == "block") { line = substr(line, first + 2); inblk = 1; continue }
      # A string: copy it through, honouring backslash escapes, so neither `//`
      # nor `/*` inside it is mistaken for a comment.
      out = out q
      rest = substr(line, first + 1)
      i = 1
      while (i <= length(rest)) {
        c = substr(rest, i, 1)
        if (c == "\\") { out = out substr(rest, i, 2); i += 2; continue }
        out = out c
        i += 1
        if (c == q) break
      }
      line = substr(rest, i)
    }
    print out
  }' "$1"
}

for PLUGIN_FILE in "$PLUGIN_DIR"/*.ts; do
  [ "$CHECK_PLUGIN" = "1" ] || break
  [ -f "$PLUGIN_FILE" ] || continue

  echo "shared-script-enforcement: checking $PLUGIN_FILE"

  CODE_ONLY="$(strip_ts_comments "$PLUGIN_FILE")"

  for SCRIPT in $ENFORCEMENT_SCRIPTS; do
    if ! echo "$CODE_ONLY" | grep -qE "(execFile|spawn).*${SCRIPT}|${SCRIPT}.*execFile|${SCRIPT}.*spawn" 2>/dev/null; then
      if ! echo "$CODE_ONLY" | grep -qE "execFile.*script|script.*execFile" 2>/dev/null; then
        echo "SHARED-SCRIPT FAIL: $PLUGIN_FILE does not call $SCRIPT via execFile/spawn"
        echo "  Enforcement logic must call scripts/hooks/$SCRIPT."
        ERRORS=$((ERRORS + 1))
      fi
    fi
  done

  for PATTERN in $INLINE_PATTERNS; do
    if echo "$CODE_ONLY" | grep -qE "$PATTERN" 2>/dev/null; then
      if echo "$CODE_ONLY" | grep -qE "(if|&&|\?\.).*${PATTERN}|test\.*${PATTERN}" 2>/dev/null; then
        echo "SHARED-SCRIPT FAIL: $PLUGIN_FILE contains inline enforcement pattern: $PATTERN"
        echo "  This logic belongs in scripts/hooks/, not in the plugin."
        ERRORS=$((ERRORS + 1))
      fi
    fi
  done

  STRIPPED=$(echo "$CODE_ONLY" | sed 's/execFile(//g')
  if echo "$STRIPPED" | grep -qE '\bexec\('; then
    echo "SHARED-SCRIPT WARN: $PLUGIN_FILE uses exec() — use execFile() to prevent command injection"
    ERRORS=$((ERRORS + 1))
  fi
done

# Generated adapters must delegate to the same script and make the implementer
# role explicit at the hook boundary. These files are produced by
# sync-claude.sh / sync-codex.sh — if they are absent (a fresh clone before
# `make sync-harnesses`), skip rather than fail, the way hook-existence-check
# treats a missing script. The drift check runs sync first, so real
# divergence is still caught there.
if [ -f .claude/settings.json ]; then
  if ! grep -q 'scripts/hooks/test-edit-denial.sh' .claude/settings.json; then
    echo "SHARED-SCRIPT FAIL: .claude/settings.json does not call test-edit-denial.sh"
    ERRORS=$((ERRORS + 1))
  fi
  if ! grep -q 'FACTORY_AGENT_ROLE=implementer' .claude/agents/implementer.md 2>/dev/null; then
    echo "SHARED-SCRIPT FAIL: Claude implementer hook does not set FACTORY_AGENT_ROLE"
    ERRORS=$((ERRORS + 1))
  fi
else
  echo "shared-script-enforcement: no .claude adapter yet — run 'make sync-harnesses' (skipping)"
fi

if [ -f .codex/agents/implementer.toml ]; then
  if ! grep -q 'scripts/hooks/test-edit-denial.sh' .codex/agents/implementer.toml; then
    echo "SHARED-SCRIPT FAIL: .codex/agents/implementer.toml does not call test-edit-denial.sh"
    ERRORS=$((ERRORS + 1))
  fi
  if ! grep -q 'FACTORY_AGENT_ROLE=implementer' .codex/agents/implementer.toml; then
    echo "SHARED-SCRIPT FAIL: Codex implementer hook does not set FACTORY_AGENT_ROLE"
    ERRORS=$((ERRORS + 1))
  fi
else
  echo "shared-script-enforcement: no .codex adapter yet — run 'make sync-harnesses' (skipping)"
fi

if [ "$ERRORS" -gt 0 ]; then
  echo "shared-script-enforcement: $ERRORS violation(s) found"
  factory_log_event "shared-script-enforcement" "$ERRORS adapter(s) reimplementing shared logic"
  exit 1
fi

echo "shared-script-enforcement: all harness adapters use shared scripts correctly"
