#!/bin/bash
set -euo pipefail

# scripts/hooks/workflow-lint.sh
# Lints canonical workflow recipes (workflows/*.md) for graph hygiene — the
# harness-agnostic substrate of "graph engineering". A recipe is a plain-text
# graph: each `## <node>` block declares `- role:` and `- kind:`. The checks:
#   - every node's role is a real factory role or `code` (deterministic plumbing)
#   - a plumbing node (dedupe/merge/flatten/...) is an edge, not an agent
#   - an `edge` node is `role: code` (coordination is code, not a conversation)
#   - a `fanout` node declares what it fans `over:`
#   - every recipe has at least one `verify` node (findings are checked first)
# It fires only if workflows/ exists — opt-in by construction. See docs/WORKFLOWS.md.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../lib/events.sh
. "$SCRIPT_DIR/../lib/events.sh"

WF_DIR="${1:-workflows}"
if [ ! -d "$WF_DIR" ]; then
  echo "workflow-lint: no $WF_DIR/ — nothing to lint"
  exit 0
fi

FILES=$(find "$WF_DIR" -maxdepth 1 -name '*.md' -not -name 'README.md' 2>/dev/null | sort || true)
if [ -z "$FILES" ]; then
  echo "workflow-lint: no recipes in $WF_DIR/"
  exit 0
fi

ERRORS=0
for f in $FILES; do
  out=$(awk '
    function validate() {
      if (node == "") return
      if (role !~ /^(spec-writer|implementer|reviewer|refactorer|wiki-maintainer|code)$/)
        print "  node \"" node "\": unknown role \"" role "\""
      if (kind !~ /^(agent|fanout|verify|edge)$/)
        print "  node \"" node "\": unknown kind \"" kind "\""
      if (kind == "edge" && role != "code")
        print "  node \"" node "\": an edge must be role: code (coordination is code, not an agent)"
      if (node ~ /(dedupe|merge|flatten|combine|filter|collect|aggregate)/ && role != "code")
        print "  node \"" node "\": looks like plumbing — make it role: code, not an agent"
      if (kind == "fanout" && !has_over)
        print "  node \"" node "\": a fanout must declare what it runs over (over:)"
      if (kind == "verify") seen_verify = 1
    }
    /^##[^#]/ { validate(); node=$2; for (i=3; i<=NF; i++) node=node " " $i; role=""; kind=""; has_over=0; next }
    /^-[[:space:]]*role:/  { role=$3 }
    /^-[[:space:]]*kind:/  { kind=$3 }
    /^-[[:space:]]*over:/  { has_over=1 }
    END {
      validate()
      if (!seen_verify)
        print "  no verify node — findings must be checked before they reach output"
    }
  ' "$f")
  if [ -n "$out" ]; then
    echo "workflow-lint: $f"
    echo "$out"
    ERRORS=$((ERRORS + 1))
  fi
done

if [ "$ERRORS" -gt 0 ]; then
  echo "workflow-lint: $ERRORS recipe(s) failed graph-hygiene checks"
  factory_log_event "workflow-lint" "workflow recipe failed graph hygiene"
  exit 1
fi
echo "workflow-lint: OK ($(printf '%s\n' "$FILES" | wc -l | tr -d ' ') recipe(s))"
