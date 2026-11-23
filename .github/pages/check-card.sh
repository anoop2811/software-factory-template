#!/usr/bin/env bash
# Run from the staged Pages root. See docs/DECISION_LOG.md:9175.
set -euo pipefail

meta() {
  local value
  # libxml's HTML4 parser warns on HTML5 elements elsewhere in the page.
  if ! value=$(xmllint --nonet --html --xpath "string(/html/head/meta[@property='$1' or @name='$1']/@content)" index.html 2>/dev/null); then
    echo "Cannot read $1 from index.html with xmllint" >&2
    return 1
  fi
  [[ -n "$value" ]] || { echo "Missing or empty $1 in index.html" >&2; return 1; }
  printf '%s' "$value"
}

expect() {
  if [[ "$2" != "$3" ]]; then
    printf '%s: expected "%s", got "%s"\n' "$1" "$3" "$2" >&2
    exit 1
  fi
}

expect_meta() {
  local value
  value=$(meta "$1") || return 1
  expect "$1" "$value" "$2"
}

command -v xmllint >/dev/null || { echo 'Install libxml2 tools to read the homepage metadata' >&2; exit 1; }
card=site-assets/share-card.png
domain=$(cat CNAME)
label='[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?'
if [[ ${#domain} -gt 253 || ! $domain =~ ^$label(\.$label)+$ ]]; then
  echo 'CNAME must contain one fully qualified hostname without a scheme or path' >&2
  exit 1
fi
# Crawlers read the absolute URLs from the source HTML. Both must match the Pages domain.
url="https://$domain/$card"
expect_meta og:image "$url"
expect_meta twitter:image "$url"
expect_meta twitter:card summary_large_image
expect_meta og:image:type image/png
alt=$(meta og:image:alt)
expect_meta twitter:image:alt "$alt"

decoded=$(identify +ping -regard-warnings -format '%m %wx%h' "$card")
expect 'Decoded image' "$decoded" 'PNG 1200x627'
width=$(meta og:image:width)
height=$(meta og:image:height)
expect 'Advertised dimensions' "PNG ${width}x${height}" "$decoded"
bytes=$(wc -c < "$card")
(( bytes < 5000000 )) || { echo 'Share card exceeds the 5 MB limit' >&2; exit 1; }
printf '%s: %s (%d bytes)\n' "$url" "$decoded" "$bytes"
