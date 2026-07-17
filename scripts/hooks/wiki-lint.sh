#!/bin/bash
set -uo pipefail

# scripts/hooks/wiki-lint.sh (Decision 15)
# The "lint" operation of the LLM-maintained wiki pattern. An agent can write a
# wiki fast; it cannot be trusted to keep every page cited and every
# cross-reference real. This is the deterministic gate that does — so the wiki
# compounds into knowledge you can rely on instead of unverified prose.
#
# Ingest (agent reads the immutable spec source, writes pages) and query
# (agent answers from the wiki) are the model's job. This gate is ours: it
# fails the build when a page is not honest.
#
# v1 enforces two invariants (see docs/CONCEPTS.md):
#   1. Provenance — every content page cites a source: a file:line reference,
#      a URL with a date, or `observed YYYY-MM-DD`.
#   2. Live cross-references — every wiki-local markdown link and [[wikilink]]
#      resolves to a file that exists.
# Orphan detection and source-drift/staleness are planned (Decision 15).
#
# Reads wiki_root from factory.yaml (default: wiki). Skips silently when there
# is no wiki to lint.
# Exit 0 = clean or skip, 1 = a missing citation or a broken link.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=../lib/config.sh
. "$SCRIPT_DIR/../lib/config.sh"

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT" || exit 1

WIKI="$(factory_config_get wiki_root wiki)"

if [ ! -d "$WIKI" ]; then
  echo "wiki-lint: no $WIKI/ directory — skipping"
  exit 0
fi

# Any markdown pages at all?
if ! find "$WIKI" -type f -name '*.md' | grep -q .; then
  echo "wiki-lint: no markdown pages in $WIKI/ — skipping"
  exit 0
fi

ERRORS=0

while IFS= read -r page; do
  [ -n "$page" ] || continue
  base="$(basename "$page")"

  # (1) Provenance — required on content pages, not on the index/readme.
  if [ "$base" != "README.md" ] && [ "$base" != "INDEX.md" ]; then
    prov=0
    grep -Eq '[A-Za-z0-9_./-]+\.[A-Za-z0-9]+:L?[0-9]+' "$page" && prov=1
    if grep -Eiq 'https?://' "$page" && grep -Eq '[0-9]{4}-[0-9]{2}-[0-9]{2}' "$page"; then prov=1; fi
    grep -Eiq 'observed[[:space:]]+[0-9]{4}-[0-9]{2}-[0-9]{2}' "$page" && prov=1
    if [ "$prov" -eq 0 ]; then
      echo "WIKI-LINT FAIL: $page has no provenance — cite a source (file:line, a URL with a date, or 'observed YYYY-MM-DD')"
      ERRORS=$((ERRORS + 1))
    fi
  fi

  # (2) Live cross-references — every wiki-local link must resolve.
  dir="$(dirname "$page")"
  for target in $(grep -oE '\]\([^) ]+\.md[^) ]*\)' "$page" | sed -E 's/^\]\(//; s/\)$//; s/#.*$//'); do
    case "$target" in http://*|https://*) continue ;; esac
    if [ ! -f "$dir/$target" ] && [ ! -f "$target" ]; then
      echo "WIKI-LINT FAIL: $page links to a missing page: $target"
      ERRORS=$((ERRORS + 1))
    fi
  done
  for name in $(grep -oE '\[\[[^]]+\]\]' "$page" | sed -E 's/^\[\[//; s/\]\]$//'); do
    if [ ! -f "$WIKI/$name.md" ]; then
      echo "WIKI-LINT FAIL: $page has a broken wikilink: [[$name]]"
      ERRORS=$((ERRORS + 1))
    fi
  done
done < <(find "$WIKI" -type f -name '*.md' | sort)

if [ "$ERRORS" -gt 0 ]; then
  echo "wiki-lint: $ERRORS problem(s) found"
  exit 1
fi

echo "wiki-lint: every wiki page is cited and its cross-references resolve"
