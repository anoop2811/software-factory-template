#!/bin/bash
# Factory copy inventory protocol: native-v1
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

# Establish physical ownership before loading any sibling code. A file alias
# cannot choose its invocation directory's libraries or installed runtime.
# docs/adr/0125-native-initializer-copy-inventory-gate.md:196.
HOOK_PATH="${BASH_SOURCE[0]}"
case "$HOOK_PATH" in /*) ;; *) HOOK_PATH="$PWD/$HOOK_PATH" ;; esac
if [ -L "$HOOK_PATH" ]; then
  echo "COPY-MANIFEST FAIL: use the physical hook file instead of a symlinked entrypoint" >&2
  exit 2
fi
SCRIPT_DIR="$(cd -P -- "${HOOK_PATH%/*}" && pwd -P && printf '.')" || exit 2
SCRIPT_DIR="${SCRIPT_DIR%$'\n.'}"
# shellcheck source=../lib/events.sh
if [ -f "$SCRIPT_DIR/../lib/events.sh" ]; then . "$SCRIPT_DIR/../lib/events.sh"; else factory_log_event() { :; }; fi

# Keep literal root names, including trailing newlines, out of shell evaluation.
# docs/adr/0125-native-initializer-copy-inventory-gate.md:42.
ROOT="$(cd -P -- "${1:-$SCRIPT_DIR/../..}" && pwd -P && printf '.')" || exit 1
ROOT="${ROOT%$'\n.'}"
OWNER_ROOT="$(cd -P -- "$SCRIPT_DIR/../.." && pwd -P && printf '.')" || exit 1
OWNER_ROOT="${OWNER_ROOT%$'\n.'}"
cd -- "$ROOT" || exit 1

# Explicit selection is execution consent, never an implicit build or lookup.
# An installed entrypoint performs physical admission before the private bridge.
# docs/adr/0125-native-initializer-copy-inventory-gate.md:57.
if [ "${FACTORY_RUNTIME_BINARY+x}" = x ]; then
  case "$FACTORY_RUNTIME_BINARY" in
    /*)
      if [ -f "$FACTORY_RUNTIME_BINARY" ] && [ -x "$FACTORY_RUNTIME_BINARY" ]; then
        exec /usr/bin/env FACTORY_BRIDGE_PROTOCOL=1 FACTORY_COPY_GATE_EVENT=1 \
          "$FACTORY_RUNTIME_BINARY" helper copies "$ROOT"
      fi ;;
  esac
  echo "COPY-MANIFEST FAIL: FACTORY_RUNTIME_BINARY must name an executable absolute regular file" >&2
  factory_log_event "copy-manifest-check" "invalid explicit runtime"
  exit 2
fi
if [ -e "$OWNER_ROOT/.factory/installation.current" ] || [ -L "$OWNER_ROOT/.factory/installation.current" ]; then
  exec /usr/bin/env FACTORY_BRIDGE_PROTOCOL=1 FACTORY_COPY_GATE_EVENT=1 \
    "$OWNER_ROOT/factory" helper copies "$ROOT"
fi
if [ -e "$ROOT/.factory/installation.current" ] || [ -L "$ROOT/.factory/installation.current" ]; then
  echo "COPY-MANIFEST FAIL: a foreign installation subject requires an explicitly selected runtime" >&2
  factory_log_event "copy-manifest-check" "foreign installation has no selected runtime"
  exit 1
fi

INIT="scripts/factory-init.sh"
if [ ! -f "$INIT" ]; then
  echo "copy-manifest-check: no $INIT here — skipping"
  exit 0
fi

ERRORS=0
# Read from a here-string, not `done < <(...)`. Process substitution needs
# /dev/fd; where it is unavailable bash cannot open it, the loop body never runs,
# and this hook reports success having inspected nothing. grep exits 1 when it
# matches nothing, which cannot certify a native adapter's unknown inventory.
COPIES="$(grep -E 'cp "\$TEMPLATE_DIR/[^"]+"' "$INIT")" || {
  status=$?
  if [ "$status" -gt 1 ]; then
    echo "COPY-MANIFEST FAIL: cannot read $INIT to enumerate copies" >&2
    factory_log_event "copy-manifest-check" "install manifest unreadable"
    exit 1
  fi
}
if [ -z "$COPIES" ]; then
  echo "COPY-MANIFEST FAIL: $INIT has no recognized legacy copy inventory; select the native runtime" >&2
  factory_log_event "copy-manifest-check" "install inventory is unknown"
  exit 1
fi
while IFS= read -r line; do
  [ -n "$line" ] || continue
  # Guarded copies (|| true) tolerate a missing source; only unconditional
  # copies abort the installer, so only those must be tracked.
  case "$line" in *"|| true"*) continue ;; esac
  path="$(printf '%s\n' "$line" | sed -E 's/.*cp "\$TEMPLATE_DIR\/([^"]+)".*/\1/')"
  case "$path" in *'*'*) continue ;; esac   # globs expand at runtime
  # A runtime glob may follow the closing quote of its source-directory prefix.
  # docs/adr/0125-native-initializer-copy-inventory-gate.md:216.
  source_token='cp "$TEMPLATE_DIR/'"$path"'"'
  source_suffix="${line#*"$source_token"}"
  case "$source_suffix" in \**|\?*|\[*) continue ;; esac
  # Drain NUL-framed names; literal pathspec success alone also matches children.
  # No process substitution: /dev/fd is not available on every supported host.
  # docs/adr/0125-native-initializer-copy-inventory-gate.md:201.
  if ! git --literal-pathspecs ls-files -z --error-unmatch -- "$path" 2>/dev/null | (
    matched=0
    while IFS= read -r -d '' indexed; do
      if [ "$indexed" = "$path" ]; then matched=1; fi
    done
    [ "$matched" -eq 1 ]
  ); then
    echo "COPY-MANIFEST FAIL: factory-init copies '$path' but it is not tracked by git (absent in a clean clone)"
    ERRORS=$((ERRORS + 1))
  fi
done <<< "$COPIES"

if [ "$ERRORS" -gt 0 ]; then
  echo "copy-manifest-check: $ERRORS untracked file(s) in the install manifest"
  factory_log_event "copy-manifest-check" "$ERRORS untracked install-copy target(s)"
  exit 1
fi

echo "copy-manifest-check: every unconditional install-copy target is git-tracked"
