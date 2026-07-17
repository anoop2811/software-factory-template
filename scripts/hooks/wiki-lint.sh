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
# It enforces (Decision 15, extended in Decision 17; see docs/CONCEPTS.md):
#   1. Provenance — every content page cites a source: a file:line reference,
#      a URL with a date, or `observed YYYY-MM-DD`.
#   2. Live cross-references — every wiki-local markdown link and [[wikilink]]
#      resolves to a file that exists.
#   3. Reachability — when an index (README/INDEX) is present, every content
#      page is linked from some other wiki page; nothing is orphaned.
#   4. Freshness (opt-in: wiki_staleness) — a page whose cited source file
#      changed after the page did is flagged stale, forcing a re-review.
#
# Reads wiki_root (default: wiki) and wiki_staleness (default: false) from
# factory.yaml. Skips (with a note) when there are no wiki content pages.
# Exit 0 = clean or skip, 1 = a missing citation, broken link, orphan, or stale page.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=../lib/config.sh
. "$SCRIPT_DIR/../lib/config.sh"

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT" || exit 1

WIKI="$(factory_config_get wiki_root wiki)"
STALE_CHECK="$(factory_config_get wiki_staleness false)"

if [ ! -d "$WIKI" ]; then
  echo "wiki-lint: no $WIKI/ directory — skipping"
  exit 0
fi

# The index/README are exempt from provenance, so a wiki with only those has
# nothing to lint — skip, matching factory doctor's "no content pages yet".
if ! find "$WIKI" -type f -name '*.md' ! -name 'README.md' ! -name 'INDEX.md' | grep -q .; then
  echo "wiki-lint: no wiki content pages in $WIKI/ — skipping"
  exit 0
fi

ERRORS=0

while IFS= read -r page; do
  [ -n "$page" ] || continue
  base="$(basename "$page")"

  # (1) Provenance — required on content pages, not on the index/readme.
  if [ "$base" != "README.md" ] && [ "$base" != "INDEX.md" ]; then
    prov=0
    grep -Eq '[A-Za-z0-9_./-]*[A-Za-z][A-Za-z0-9_./-]*:L?[0-9]+' "$page" && prov=1
    if grep -Eiq 'https?://' "$page" && grep -Eq '[0-9]{4}-[0-9]{2}-[0-9]{2}' "$page"; then prov=1; fi
    grep -Eiq 'observed[[:space:]]+[0-9]{4}-[0-9]{2}-[0-9]{2}' "$page" && prov=1
    if [ "$prov" -eq 0 ]; then
      echo "WIKI-LINT FAIL: $page has no provenance — cite a source (file:line, a URL with a date, or 'observed YYYY-MM-DD')"
      ERRORS=$((ERRORS + 1))
    fi
  fi

  # (2) Live cross-references — every wiki-local link must resolve.
  dir="$(dirname "$page")"
  while IFS= read -r target; do
    [ -n "$target" ] || continue
    case "$target" in http://*|https://*) continue ;; esac
    # Resolve relative to the page; only a wiki-local target satisfies the link.
    if [ ! -f "$dir/$target" ]; then
      echo "WIKI-LINT FAIL: $page links to a missing page: $target"
      ERRORS=$((ERRORS + 1))
    fi
  done < <(grep -oE '\]\([^)]+\.md[^)]*\)' "$page" | sed -E 's/^\]\(//; s/\)$//; s/#.*$//')
  while IFS= read -r name; do
    [ -n "$name" ] || continue
    if [ ! -f "$WIKI/$name.md" ]; then
      echo "WIKI-LINT FAIL: $page has a broken wikilink: [[$name]]"
      ERRORS=$((ERRORS + 1))
    fi
  done < <(grep -oE '\[\[[^]]+\]\]' "$page" | sed -E 's/^\[\[//; s/\]\]$//')
done < <(find "$WIKI" -type f -name '*.md' | sort)

# (3) Reachability — when an index exists, every content page must be linked
# from some other wiki page. A page nothing points to is dead knowledge.
if [ -f "$WIKI/README.md" ] || [ -f "$WIKI/INDEX.md" ]; then
  while IFS= read -r page; do
    [ -n "$page" ] || continue
    b="$(basename "$page")"
    name="${b%.md}"
    ref=0
    if grep -rlF --include='*.md' -- "$b" "$WIKI" 2>/dev/null | grep -qvF -- "$page"; then ref=1; fi
    if grep -rlF --include='*.md' -- "[[$name]]" "$WIKI" 2>/dev/null | grep -qvF -- "$page"; then ref=1; fi
    if [ "$ref" -eq 0 ]; then
      echo "WIKI-LINT FAIL: $page is an orphan — no other wiki page links to it (add a link from your index)"
      ERRORS=$((ERRORS + 1))
    fi
  done < <(find "$WIKI" -type f -name '*.md' ! -name 'README.md' ! -name 'INDEX.md' | sort)
fi

# (4) Freshness (opt-in) — flag a page whose cited source changed after it.
if [ "$STALE_CHECK" = "true" ] && git rev-parse --show-toplevel >/dev/null 2>&1; then
  while IFS= read -r page; do
    [ -n "$page" ] || continue
    page_t="$(git log -1 --format=%ct -- "$page" 2>/dev/null || true)"
    [ -n "$page_t" ] || continue
    while IFS= read -r src; do
      [ -n "$src" ] || continue
      [ -f "$src" ] || continue
      src_t="$(git log -1 --format=%ct -- "$src" 2>/dev/null || true)"
      [ -n "$src_t" ] || continue
      if [ "$src_t" -gt "$page_t" ]; then
        echo "WIKI-LINT FAIL: $page is stale — its source $src changed after the page (re-review and re-commit the page)"
        ERRORS=$((ERRORS + 1))
      fi
    done < <(grep -oE '[A-Za-z0-9_./-]*[A-Za-z][A-Za-z0-9_./-]*:L?[0-9]+' "$page" | sed -E 's/:L?[0-9]+$//' | sort -u)
  done < <(find "$WIKI" -type f -name '*.md' ! -name 'README.md' ! -name 'INDEX.md' | sort)
fi

if [ "$ERRORS" -gt 0 ]; then
  echo "wiki-lint: $ERRORS problem(s) found"
  exit 1
fi

echo "wiki-lint: every wiki content page is cited, reachable, and its cross-references resolve"
