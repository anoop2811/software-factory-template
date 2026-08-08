#!/bin/bash
set -euo pipefail

# scripts/factory-metrics.sh
# What the factory is actually doing to this repository.
#
# Local only. Nothing is transmitted, and nothing here changes that: the factory
# you install never phones home. These numbers are computed on your machine, from
# your repository, for you — export them yourself if you want them elsewhere.
#
# Usage:
#   factory metrics                 # terminal report
#   factory metrics --json          # machine-readable, for your own collector
#   factory metrics --html          # writes .factory/metrics.html (no server)
#   factory metrics --days 90       # window (default 30)
#
# Two rules govern what appears here:
#   1. Every metric names the decision it informs. A number nobody acts on is
#      noise wearing a lab coat.
#   2. The uncomfortable ones are shown first-class — gates you route around,
#      gates that are inert, baselines that are stale. A report that only shows
#      wins is marketing, not measurement.
#
# Most of this is derived from git history, so it is meaningful the day you
# install the factory rather than after months of collecting events.

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT" || exit 1
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/config.sh
[ -f "$SCRIPT_DIR/lib/config.sh" ] && . "$SCRIPT_DIR/lib/config.sh"
# shellcheck source=lib/color.sh
[ -f "$SCRIPT_DIR/lib/color.sh" ] && . "$SCRIPT_DIR/lib/color.sh"

FORMAT="text"
DAYS=30
while [ "$#" -gt 0 ]; do
  case "$1" in
    --json) FORMAT="json"; shift ;;
    --html) FORMAT="html"; shift ;;
    --days) DAYS="${2:-30}"; shift 2 ;;
    --days=*) DAYS="${1#*=}"; shift ;;
    *) echo "factory metrics: unknown argument '$1'" >&2; exit 2 ;;
  esac
done
case "$DAYS" in ''|*[!0-9]*) DAYS=30 ;; esac

SINCE="$DAYS days ago"
EVENT_LOG="${FACTORY_EVENT_LOG:-$ROOT/.factory/events.log}"

# ── Enforcement: are the gates real, and are they earning their keep? ──
GATES_INSTALLED="$( { find scripts/hooks -maxdepth 1 -name '*.sh' 2>/dev/null || true; } | wc -l | tr -d ' ')"

# How many of those gates can report a block at all. "gates installed 14" next
# to "blocks caught 0" reads as fourteen quiet gates; if only six are wired to
# the event log, the honest reading is unavailable and the report flatters
# itself. gate-instrumentation-check keeps the shipped gates at parity, but an
# adopter's own gate may be mute, so the number is measured, never assumed.
GATES_REPORTING="$( { grep -lE 'factory_log_event' scripts/hooks/*.sh 2>/dev/null || true; } | wc -l | tr -d ' ')"
# A gate carrying the `factory: no-block-event` marker has declared that its
# non-zero exit stops no work, so it is not mute — it is not a block at all.
# Same marker gate-instrumentation-check reads, so the two cannot disagree.
GATES_MUTE="$( for _h in scripts/hooks/*.sh; do
    [ -f "$_h" ] || continue
    grep -q 'factory: no-block-event' "$_h" 2>/dev/null && continue
    grep -qE '^[[:space:]]*exit[[:space:]]+[1-9]' "$_h" 2>/dev/null || continue
    grep -q 'factory_log_event' "$_h" 2>/dev/null || basename "$_h"
  done | wc -l | tr -d ' ')"

# Armed vs inert is a property of configuration, not of the files existing.
# Computed here directly rather than shelling to doctor, which runs the full
# break/fix proof and would make a metrics read cost 17 seconds.
armed_count=0; inert_count=0
count_gate() { if [ -n "$1" ]; then armed_count=$((armed_count+1)); else inert_count=$((inert_count+1)); fi; }
if command -v factory_config_get >/dev/null 2>&1; then
  count_gate "$(factory_config_get test_file_patterns || true)"
  count_gate "$(factory_config_get citation_prefix || true)"
  count_gate "$(factory_config_get protected_paths || true)"
  count_gate "$(factory_config_get check_command || true)"
fi

# Blocks: what the gates stopped. Repeat blocks of the same gate in a short
# window are the friction signal — a gate you keep hitting is either catching a
# real habit or is wrong, and both are worth knowing.
BLOCKS_TOTAL=0; BLOCKS_WINDOW=0; BLOCKS_BY_GATE=""; REPEAT_BLOCKS=0
num() { case "${1:-}" in ""|*[!0-9]*) printf 0 ;; *) printf %s "$1" ;; esac; }
if [ -f "$EVENT_LOG" ]; then
  BLOCKS_TOTAL="$(grep -c . "$EVENT_LOG" 2>/dev/null || true)"
  CUTOFF="$(date -u -v-"${DAYS}"d +%Y-%m-%d 2>/dev/null || date -u -d "$DAYS days ago" +%Y-%m-%d 2>/dev/null || echo 0000-00-00)"
  BLOCKS_WINDOW="$( { awk -F'\t' -v c="$CUTOFF" '$1 >= c' "$EVENT_LOG" 2>/dev/null || true; } | grep -c . || true)"
  BLOCKS_BY_GATE="$(awk -F'\t' -v c="$CUTOFF" '$1 >= c {n[$2]++} END {for (g in n) printf "%s\t%d\n", g, n[g]}' "$EVENT_LOG" 2>/dev/null | sort -k2 -rn || true)"
  # Same gate, 3+ times within one hour: a friction signal, not a verdict.
  REPEAT_BLOCKS="$(awk -F'\t' -v c="$CUTOFF" '$1 >= c {h=substr($1,1,13); n[h"\t"$2]++} END {r=0; for (k in n) if (n[k] >= 3) r++; print r}' "$EVENT_LOG" 2>/dev/null || echo 0)"
fi

# ── Loop health, from git. Works retroactively — no instrumentation needed. ──
COMMITS="$(git rev-list --count --since="$SINCE" HEAD 2>/dev/null || echo 0)"
MERGES="$(git rev-list --count --merges --since="$SINCE" HEAD 2>/dev/null || echo 0)"
REVERTS="$( { git log --since="$SINCE" --format='%s' 2>/dev/null || true; } | grep -ci '^revert' || true)"
AUTHORS="$( { git log --since="$SINCE" --format='%an' 2>/dev/null || true; } | sort -u | grep -c . || true)"
# Churn: files changed in more than one commit in the window. High churn on the
# same files is rework — the loop circling rather than converging.
CHURN_FILES="$( { git log --since="$SINCE" --name-only --format='' 2>/dev/null || true; } | { grep . || true; } | sort | uniq -c | awk '$1 > 1' | wc -l | tr -d ' ')"
TOUCHED_FILES="$( { git log --since="$SINCE" --name-only --format='' 2>/dev/null || true; } | { grep . || true; } | sort -u | wc -l | tr -d ' ')"

# ── Verification discipline: are claims cited, or asserted? ──
# The contract's whole point. Counted over real history, not self-reported.
#
# Per commit, not per line. A commit whose body makes the claim on three bullets
# is one commit that made a claim, and counting the lines would inflate both
# numbers against a report that says "commits".
VERIFICATION="$( { git log --since="$SINCE" --format='%B%x00' 2>/dev/null || true; } | python3 -c '
import re, sys
raw = sys.stdin.buffer.read().decode("utf-8", "replace")
claim = re.compile(r"(?:^|[^\w])(?:verified|fixed|works)(?:[^\w]|$)", re.I)
# Evidence: a quoted command, an arrow to output, an exit status, or an explicit
# admission that something was NOT verified — the honest form counts as cited.
cite = re.compile(r"`[^`]+`|→|exit:|NOT verified|unverified")
claims = cited = 0
for msg in raw.split("\x00"):
    if not msg.strip() or not claim.search(msg):
        continue
    claims += 1
    if cite.search(msg):
        cited += 1
print(claims, cited)
' 2>/dev/null || echo "0 0")"
CLAIM_COMMITS="${VERIFICATION%% *}"
CITED_COMMITS="${VERIFICATION##* }"

# ── Agent quality, from eval baselines ──
EVAL_JSON="$(python3 - "$ROOT" <<'PY' 2>/dev/null || echo '{"tasks":[],"stale":0,"harnesses":0}'
import json, glob, os, sys
root = sys.argv[1]
tasks, stale, harnesses = [], 0, 0
for f in sorted(glob.glob(os.path.join(root, "eval/results/*-baseline.json"))):
    harnesses += 1
    try:
        d = json.load(open(f))
    except Exception:
        continue
    cur = f.replace("-baseline.json", "-current.json")
    curd = {}
    if os.path.exists(cur):
        try:
            c = json.load(open(cur))
            curd = {t["task"]: t for t in c.get("tasks", [])}
            if d.get("inputs_fingerprint") and c.get("inputs_fingerprint") \
               and d["inputs_fingerprint"] != c["inputs_fingerprint"]:
                stale += 1
        except Exception:
            pass
    for t in d.get("tasks", []):
        c = curd.get(t["task"], {})
        tasks.append({
            "harness": d.get("harness", "?"), "task": t["task"],
            "baseline": t.get("score"), "current": c.get("score"),
        })
print(json.dumps({"tasks": tasks, "stale": stale, "harnesses": harnesses}))
PY
)"

# Empty is zero: a repo with no history or no events must still report.
BLOCKS_TOTAL="$(num "${BLOCKS_TOTAL}")"
BLOCKS_WINDOW="$(num "${BLOCKS_WINDOW}")"
REPEAT_BLOCKS="$(num "${REPEAT_BLOCKS}")"
COMMITS="$(num "${COMMITS}")"
MERGES="$(num "${MERGES}")"
REVERTS="$(num "${REVERTS}")"
AUTHORS="$(num "${AUTHORS}")"
CHURN_FILES="$(num "${CHURN_FILES}")"
TOUCHED_FILES="$(num "${TOUCHED_FILES}")"
CLAIM_COMMITS="$(num "${CLAIM_COMMITS}")"
CITED_COMMITS="$(num "${CITED_COMMITS}")"
GATES_INSTALLED="$(num "${GATES_INSTALLED}")"
GATES_REPORTING="$(num "${GATES_REPORTING}")"
GATES_MUTE="$(num "${GATES_MUTE}")"

# ── Emit ─────────────────────────────────────────────────────────────
metrics_json() {
  python3 - "$DAYS" "$GATES_INSTALLED" "$armed_count" "$inert_count" \
    "$BLOCKS_TOTAL" "$BLOCKS_WINDOW" "$REPEAT_BLOCKS" "$COMMITS" "$MERGES" \
    "$REVERTS" "$AUTHORS" "$CHURN_FILES" "$TOUCHED_FILES" "$CLAIM_COMMITS" \
    "$CITED_COMMITS" "$EVAL_JSON" "$BLOCKS_BY_GATE" "$GATES_REPORTING" \
    "$GATES_MUTE" <<'PY'
import json, sys
(days, gates, armed, inert, bt, bw, rb, commits, merges, reverts, authors,
 churn, touched, claims, cited, evaljson, bygate, reporting, mute) = sys.argv[1:20]
by = []
for line in bygate.splitlines():
    if "\t" in line:
        g, n = line.split("\t", 1)
        by.append({"gate": g, "blocks": int(n)})
print(json.dumps({
  "schema": "factory.metrics/v1",
  "window_days": int(days),
  "enforcement": {
    "gates_installed": int(gates), "gates_armed": int(armed),
    "gates_inert": int(inert),
    # How many gates can report a block, and how many can block without
    # reporting. A zero block count means nothing until you know these.
    "gates_reporting": int(reporting), "gates_mute": int(mute),
    "blocks_total": int(bt),
    "blocks_in_window": int(bw), "repeat_block_hours": int(rb),
    "blocks_by_gate": by,
  },
  "loop": {
    "commits": int(commits), "merges": int(merges), "reverts": int(reverts),
    "authors": int(authors), "files_touched": int(touched),
    "files_reworked": int(churn),
  },
  "verification": {"claim_commits": int(claims), "cited_commits": int(cited)},
  "agents": json.loads(evaljson),
  "not_measured": [
    "token spend per role — your harness owns that; the factory does not meter it",
    "code quality — not honestly measurable without judgment, so it is not claimed",
  ],
}, indent=2))
PY
}

pct() { [ "$2" -eq 0 ] && { printf 'n/a'; return; }; awk "BEGIN{printf \"%d%%\", ($1/$2)*100}"; }

case "$FORMAT" in
  json) metrics_json ;;

  html)
    TPL="$SCRIPT_DIR/../templates/metrics.html"
    OUT="$ROOT/.factory/metrics.html"
    [ -f "$TPL" ] || { echo "factory metrics: template not found at $TPL" >&2; exit 1; }
    mkdir -p "$ROOT/.factory"
    # The page is a normal HTML file; only the data is injected. Edit the
    # template like any web page — no server, no build step, no binary.
    #
    # The JSON travels through a file, and the heredoc below is quoted. An
    # unquoted heredoc would let the shell expand anything the JSON happened to
    # contain — and the JSON carries repo-derived strings such as gate names
    # from .factory/events.log, which is an ordinary writable file. Data must
    # not become code on the way to the page.
    METRICS_TMP="$(mktemp "${TMPDIR:-/tmp}/factory-metrics.XXXXXX")"
    trap 'rm -f "$METRICS_TMP"' EXIT INT TERM
    metrics_json > "$METRICS_TMP"
    python3 - "$TPL" "$OUT" "$METRICS_TMP" <<'PY'
import sys
tpl, out, src = sys.argv[1], sys.argv[2], sys.argv[3]
with open(src, encoding="utf-8") as fh:
    data = fh.read()
# The data lands inside a <script> element, where the HTML parser ends the
# script at the first literal "</" sequence regardless of JSON syntax. The
# escape is valid inside a JSON string and invisible to JSON.parse.
data = data.replace("</", "<\\/")
with open(tpl, encoding="utf-8") as fh:
    html = fh.read().replace("/*__FACTORY_METRICS_JSON__*/null", data)
with open(out, "w", encoding="utf-8") as fh:
    fh.write(html)
PY
    echo "factory metrics: wrote $OUT"
    echo "  open it directly — it is a self-contained file, nothing is served."
    ;;

  *)
    echo "${C_BOLD:-}Factory metrics${C_RESET:-} — last $DAYS days, computed locally"
    echo ""
    echo "${C_BOLD:-}Enforcement${C_RESET:-}  ${C_DIM:-}is the factory real, or installed theatre?${C_RESET:-}"
    printf '  %-34s %s\n' "gates installed" "$GATES_INSTALLED"
    if [ $((armed_count + inert_count)) -gt 0 ]; then
      printf '  %-34s %s armed, %s%s inert%s\n' "config-armed gates" "$armed_count" \
        "${C_YELLOW:-}" "$inert_count" "${C_RESET:-}"
      [ "$inert_count" -gt 0 ] && printf '  %-34s %s\n' "" "${C_DIM:-}inert is a choice — 'factory doctor' names which${C_RESET:-}"
    fi
    printf '  %-34s %s in window (%s retained)\n' "blocks caught" "$BLOCKS_WINDOW" "$BLOCKS_TOTAL"
    # Said next to the block count, not in a footnote: a reader who sees zero
    # blocks must find out here whether every gate could have reported one.
    if [ "$GATES_MUTE" -gt 0 ]; then
      printf '  %-34s %s%s of %s can block without recording it%s\n' "" \
        "${C_YELLOW:-}" "$GATES_MUTE" "$GATES_INSTALLED" "${C_RESET:-}"
      printf '  %-34s %s\n' "" "${C_DIM:-}so a low block count may be silence, not calm${C_RESET:-}"
    fi
    if [ -n "$BLOCKS_BY_GATE" ]; then
      printf '%s\n' "$BLOCKS_BY_GATE" | while IFS="$(printf '\t')" read -r g n; do
        [ -n "$g" ] && printf '    %-32s %s\n' "$g" "$n"
      done
    fi
    if [ "$REPEAT_BLOCKS" -gt 0 ]; then
      printf '  %-34s %s%s hour(s)%s %s\n' "same gate 3+ times in an hour" \
        "${C_YELLOW:-}" "$REPEAT_BLOCKS" "${C_RESET:-}" \
        "${C_DIM:-}— friction: a real habit, or a wrong gate${C_RESET:-}"
    fi
    echo ""
    echo "${C_BOLD:-}Loop health${C_RESET:-}  ${C_DIM:-}is work converging, or circling?${C_RESET:-}"
    printf '  %-34s %s across %s author(s)\n' "commits" "$COMMITS" "$AUTHORS"
    printf '  %-34s %s\n' "merges" "$MERGES"
    printf '  %-34s %s %s\n' "reverts" "$REVERTS" "${C_DIM:-}— work that had to be undone${C_RESET:-}"
    printf '  %-34s %s of %s files %s\n' "reworked" "$CHURN_FILES" "$TOUCHED_FILES" \
      "${C_DIM:-}— changed in more than one commit${C_RESET:-}"
    echo ""
    echo "${C_BOLD:-}Verification discipline${C_RESET:-}  ${C_DIM:-}are claims cited, or asserted?${C_RESET:-}"
    printf '  %-34s %s of %s (%s)\n' "claims carrying evidence" "$CITED_COMMITS" \
      "$CLAIM_COMMITS" "$(pct "$CITED_COMMITS" "$CLAIM_COMMITS")"
    echo ""
    echo "${C_BOLD:-}Agents${C_RESET:-}  ${C_DIM:-}getting better, or worse?${C_RESET:-}"
    printf '%s' "$EVAL_JSON" | python3 -c "
import json,sys
d=json.load(sys.stdin)
if not d['tasks']:
    print('  no eval baselines yet — run: factory selftest, then golden-task-eval --save-baseline')
else:
    for t in d['tasks']:
        cur = t['current'] if t['current'] is not None else '-'
        print('    %-30s baseline %s  current %s' % (t['task'][:30], t['baseline'], cur))
    if d['stale']:
        print('  %d baseline(s) STALE — measured against different inputs' % d['stale'])
" 2>/dev/null || echo "  (eval results unreadable)"
    echo ""
    echo "${C_DIM:-}Not measured here: token spend per role (your harness owns that), and code"
    echo "quality (not honestly measurable without judgment, so it is not claimed)."
    echo "Nothing above left this machine. --json to export, --html for a page.${C_RESET:-}"
    ;;
esac
