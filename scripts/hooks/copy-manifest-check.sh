#!/bin/bash
set -uo pipefail

# scripts/hooks/copy-manifest-check.sh (Decision 18)
# Every file factory-init copies unconditionally must be tracked by git, so a
# clean clone — what an adopter actually installs from — contains it. A file
# present in the working tree but gitignored or untracked passes every local
# test and then aborts the installer's `cp` in a fresh clone. This is
# Decision 6's git-tracking rule, extended from the hooks to the whole install
# manifest, because the same class bit twice.
#
# Scans the repo given as $1, or this repo. Skips when there is no factory-init
# (an adopter repo doesn't ship the installer).
# Exit 0 = every unconditional copy target is tracked (or skip), 1 = one is not.

ROOT="${1:-$(cd "$(dirname "$0")/../.." && pwd)}"
cd "$ROOT" || exit 1

INIT="scripts/factory-init.sh"
if [ ! -f "$INIT" ]; then
  echo "copy-manifest-check: no $INIT here — skipping"
  exit 0
fi

ERRORS=0
while IFS= read -r line; do
  # Guarded copies (|| true) tolerate a missing source; only unconditional
  # copies abort the installer, so only those must be tracked.
  case "$line" in *"|| true"*) continue ;; esac
  path="$(printf '%s\n' "$line" | sed -E 's/.*cp "\$TEMPLATE_DIR\/([^"]+)".*/\1/')"
  case "$path" in *'*'*) continue ;; esac   # globs expand at runtime
  if ! git ls-files --error-unmatch "$path" >/dev/null 2>&1; then
    echo "COPY-MANIFEST FAIL: factory-init copies '$path' but it is not tracked by git (absent in a clean clone)"
    ERRORS=$((ERRORS + 1))
  fi
done < <(grep -E 'cp "\$TEMPLATE_DIR/[^"]+"' "$INIT")

if [ "$ERRORS" -gt 0 ]; then
  echo "copy-manifest-check: $ERRORS untracked file(s) in the install manifest"
  exit 1
fi

echo "copy-manifest-check: every unconditional install-copy target is git-tracked"
