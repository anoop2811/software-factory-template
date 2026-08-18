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
# shellcheck source=../lib/events.sh
if [ -f "$SCRIPT_DIR/../lib/events.sh" ]; then . "$SCRIPT_DIR/../lib/events.sh"; else factory_log_event() { :; }; fi

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT" || exit 1

WIKI="$(factory_config_get wiki_root wiki)"
STALE_CHECK="$(factory_config_get wiki_staleness false)"

if [ ! -d "$WIKI" ]; then
  echo "wiki-lint: no $WIKI/ directory — skipping"
  exit 0
fi

# Content pages, enumerated once and checked once. `find ... | grep -q .` used to
# stand in for this, and it could not tell "this wiki has no content pages yet"
# apart from "find could not read the directory" — an unreadable wiki reported
# "skipping" and the hook exited 0 having examined nothing. Sections 3 and 4 reuse
# this list rather than re-running the same find with the same blind spot.
if ! CONTENT_PAGES="$(find "$WIKI" -type f -name '*.md' ! -name 'README.md' ! -name 'INDEX.md' | sort)"; then
  echo "WIKI-LINT FAIL: cannot enumerate content pages under $WIKI" >&2
  exit 1
fi
if [ -z "$CONTENT_PAGES" ]; then
  echo "wiki-lint: no wiki content pages in $WIKI/ — skipping"
  exit 0
fi

# Loops read from here-strings rather than `done < <(...)`. Process substitution
# needs /dev/fd, and on a host without it bash cannot open the substitution: every
# loop below was skipped and the hook reported success having checked nothing.
# Here-strings are backed by a temporary file, so they work regardless — and the
# enumerations are captured first, so a failing find or grep is visible.
ERRORS=0

if ! PAGES="$(find "$WIKI" -type f -name '*.md' | sort)"; then
  echo "WIKI-LINT FAIL: cannot enumerate pages under $WIKI" >&2
  exit 1
fi

while IFS= read -r page; do
  [ -n "$page" ] || continue
  base="$(basename "$page")"

  # (1) Provenance — required on content pages, not on the index/readme.
  if [ "$base" != "README.md" ] && [ "$base" != "INDEX.md" ]; then
    prov=0
    # A file:line citation has to look like a path, not merely like a word with a
    # colon and a number: `Version:1` and `Note:2` satisfied the old pattern, so
    # ordinary prose counted as provenance. Requiring a dot-suffixed final segment
    # keeps `scripts/lib/config.sh:78` and `run.sh:12` while rejecting those.
    # The left boundary admits Markdown code delimiters. Writers wrap citations in
    # backticks — `scripts/lib/config.sh:78` — and requiring whitespace or '('
    # before the path rejected exactly the form the docs themselves use.
    grep -Eq '(^|[[:space:](`*_"'"'"'])[A-Za-z0-9_./-]*[A-Za-z0-9_-]+\.[A-Za-z0-9]+:L?[0-9]+' "$page" && prov=1
    if grep -Eiq 'https?://' "$page" && grep -Eq '[0-9]{4}-[0-9]{2}-[0-9]{2}' "$page"; then prov=1; fi
    # Word-bounded `observed`, so `unobserved 2024-01-01` — which asserts the
    # opposite — no longer passes as an observation.
    grep -Eiq '(^|[^[:alnum:]_])observed[[:space:]]+[0-9]{4}-[0-9]{2}-[0-9]{2}' "$page" && prov=1
    if [ "$prov" -eq 0 ]; then
      echo "WIKI-LINT FAIL: $page has no provenance — cite a source (file:line, a URL with a date, or 'observed YYYY-MM-DD')"
      ERRORS=$((ERRORS + 1))
    fi
  fi

  # (2) Live cross-references — every wiki-local link must resolve.
  dir="$(dirname "$page")"
  # A markdown destination ends at whitespace, so an optional "title" after it is
  # not part of the filename. Reference-style links are resolved through their
  # definition line rather than ignored.
  # Match from the opening bracket so an image can be told apart from a link: the
  # `!` that makes `![alt](logo.png)` an image sits before the bracket, and a
  # pattern anchored at `](` cannot see it. An image destination is an asset, not
  # a wiki page, and reporting `logo.png` as a missing page is noise.
  LINKS="$(
    grep -oE '!?\[[^]]*\]\([^)]+\)' "$page" \
      | grep -v '^!' \
      | sed -E 's/^\[[^]]*\]\(//; s/\)$//; s/[[:space:]].*$//; s/#.*$//'
    grep -oE '^\[[^]]+\]:[[:space:]]*[^[:space:]]+' "$page" | sed -E 's/^\[[^]]+\]:[[:space:]]*//; s/#.*$//'
  )"
  while IFS= read -r target; do
    [ -n "$target" ] || continue
    # Anything carrying a URI scheme addresses something outside the wiki —
    # mailto:, tel:, ftp:, data: as much as http:. Only a relative path can be a
    # wiki page, so a scheme means "not ours to resolve".
    case "$target" in
      *://*) continue ;;
      [A-Za-z]*:*)
        case "${target%%:*}" in
          *[!A-Za-z0-9+.-]*) : ;;   # not a scheme; fall through and resolve it
          *) continue ;;
        esac
        ;;
    esac
    # Resolve relative to the page; only a wiki-local target satisfies the link.
    if [ ! -f "$dir/$target" ]; then
      echo "WIKI-LINT FAIL: $page links to a missing page: $target"
      ERRORS=$((ERRORS + 1))
    fi
  done <<< "$LINKS"
  WIKILINKS="$(grep -oE '\[\[[^]]+\]\]' "$page" | sed -E 's/^\[\[//; s/\]\]$//')"
  while IFS= read -r name; do
    [ -n "$name" ] || continue
    if [ ! -f "$WIKI/$name.md" ]; then
      echo "WIKI-LINT FAIL: $page has a broken wikilink: [[$name]]"
      ERRORS=$((ERRORS + 1))
    fi
  done <<< "$WIKILINKS"
done <<< "$PAGES"

# (3) Reachability — when an index exists, every content page must be linked
# from some other wiki page. A page nothing points to is dead knowledge.
if [ -f "$WIKI/README.md" ] || [ -f "$WIKI/INDEX.md" ]; then
  CONTENT="$CONTENT_PAGES"
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
  done <<< "$CONTENT"
fi

# (4) Freshness (opt-in) — flag a page whose cited source changed after it.
if [ "$STALE_CHECK" = "true" ] && git rev-parse --show-toplevel >/dev/null 2>&1; then
  STALE_PAGES="$CONTENT_PAGES"
  while IFS= read -r page; do
    [ -n "$page" ] || continue
    page_t="$(git log -1 --format=%ct -- "$page" 2>/dev/null || true)"
    [ -n "$page_t" ] || continue
    SRCS="$(grep -oE '[A-Za-z0-9_./-]*[A-Za-z][A-Za-z0-9_./-]*:L?[0-9]+' "$page" | sed -E 's/:L?[0-9]+$//' | sort -u)"
    while IFS= read -r src; do
      [ -n "$src" ] || continue
      [ -f "$src" ] || continue
      src_t="$(git log -1 --format=%ct -- "$src" 2>/dev/null || true)"
      [ -n "$src_t" ] || continue
      if [ "$src_t" -gt "$page_t" ]; then
        echo "WIKI-LINT FAIL: $page is stale — its source $src changed after the page (re-review and re-commit the page)"
        ERRORS=$((ERRORS + 1))
      fi
    done <<< "$SRCS"
  done <<< "$STALE_PAGES"
fi

if [ "$ERRORS" -gt 0 ]; then
  echo "wiki-lint: $ERRORS problem(s) found"
  factory_log_event "wiki-lint" "$ERRORS uncited, orphaned, or stale wiki page(s)"
  exit 1
fi

echo "wiki-lint: every wiki content page is cited, reachable, and its cross-references resolve"
