#!/usr/bin/env bash
# Run from the staged Pages root. See docs/DECISION_LOG.md:9175.
set -euo pipefail

meta() {
  xmllint --nonet --html --xpath "string(/html/head/meta[@property='$1' or @name='$1']/@content)" index.html 2>/dev/null
}

expect() {
  if [[ "$2" != "$3" ]]; then
    printf '%s: expected "%s", got "%s"\n' "$1" "$3" "$2" >&2
    exit 1
  fi
}

card=site-assets/share-card.png
url="https://$(cat CNAME)/$card"
expect 'Open Graph image' "$(meta og:image)" "$url"
expect 'X image' "$(meta twitter:image)" "$url"
expect 'X card' "$(meta twitter:card)" summary_large_image
expect 'Open Graph media type' "$(meta og:image:type)" image/png
alt=$(meta og:image:alt)
[[ -n "$alt" ]] || { echo 'Missing image description' >&2; exit 1; }
expect 'X image description' "$(meta twitter:image:alt)" "$alt"

decoded=$(identify +ping -regard-warnings -format '%m %wx%h' "$card")
expect 'Decoded image' "$decoded" 'PNG 1200x627'
expect 'Advertised dimensions' "PNG $(meta og:image:width)x$(meta og:image:height)" "$decoded"
bytes=$(wc -c < "$card")
(( bytes < 5000000 )) || { echo 'Share card exceeds the 5 MB limit' >&2; exit 1; }
printf '%s: %s (%d bytes)\n' "$url" "$decoded" "$bytes"
