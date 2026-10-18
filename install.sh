#!/bin/sh
set -eu

# Candidate selection is reserved before the legacy parser (which re-splits its
# arguments). Preserve literal argv and authenticate before the first execution.
# docs/adr/0101-go-release-handoff.md:40, docs/adr/0101-go-release-handoff.md:80.
candidate_install() {
  umask 077
  case "${1-}" in init|upgrade) ;; *) printf '%s\n' 'install: candidate requires init or upgrade' >&2; exit 2 ;; esac
  candidate_tag='' candidate_pending=0 candidate_seen=0 candidate_help=0
  for candidate_arg do
    if [ "$candidate_pending" -eq 1 ]; then
      candidate_tag=$candidate_arg; candidate_pending=0; continue
    fi
    case "$candidate_arg" in
      --go-candidate|--go-candidate=*)
        [ "$candidate_seen" -eq 0 ] || { printf '%s\n' 'install: repeated candidate selector' >&2; exit 2; }
        candidate_seen=1
        case "$candidate_arg" in
          --go-candidate) candidate_pending=1 ;;
          *) candidate_tag=${candidate_arg#--go-candidate=} ;;
        esac ;;
      --source|--source=*|--ref|--ref=*|--local|--local=*|--installation|--installation=*)
        printf '%s\n' 'install: conflicting candidate source' >&2; exit 2 ;;
      --help|-h) candidate_help=1 ;;
    esac
  done
  case "$candidate_tag" in
    ''|*[!a-zA-Z0-9._+-]*|[!a-zA-Z0-9]*|[mM][aA][iI][nN]|[hH][eE][aA][dD]|[lL][aA][tT][eE][sS][tT]|[cC][uU][rR][rR][eE][nN][tT])
      printf '%s\n' 'install: invalid candidate tag' >&2; exit 2 ;;
  esac
  [ "$candidate_pending" -eq 0 ] && [ "${#candidate_tag}" -le 128 ] || exit 2
  if [ "$candidate_help" -eq 1 ]; then
    printf '%s\n' 'Usage: install.sh init|upgrade [args] --go-candidate TAG'
    exit 0
  fi
  command -v gh >/dev/null 2>&1 || { printf '%s\n' 'install: trusted GitHub CLI (gh) is required' >&2; exit 1; }
  candidate_verifier=$(command -v gh)
  [ -f "$candidate_verifier" ] && [ -x "$candidate_verifier" ] && [ -x /bin/bash ] || { printf '%s\n' 'install: trusted gh executable and /bin/bash are required' >&2; exit 1; }
  case "$(uname -s)/$(uname -m)" in
    Linux/x86_64) candidate_target=linux-amd64 ;;
    Linux/aarch64|Linux/arm64) candidate_target=linux-arm64 ;;
    Darwin/x86_64) candidate_target=darwin-amd64 ;;
    Darwin/arm64) candidate_target=darwin-arm64 ;;
    *) printf '%s\n' 'install: unsupported candidate platform' >&2; exit 1 ;;
  esac
  candidate_dir=$(mktemp -d "${TMPDIR:-/tmp}/factory-release.XXXXXXXX")
  candidate_child='' candidate_watchdog=''
  candidate_retain=0
  candidate_watch() {
    candidate_sleep=
    candidate_stop_sleep() {
      if [ -n "$candidate_sleep" ]; then
        kill "$candidate_sleep" 2>/dev/null || :
        wait "$candidate_sleep" 2>/dev/null || :
        candidate_sleep=
      fi
    }
    trap candidate_stop_sleep 0
    trap 'candidate_stop_sleep; exit 0' TERM HUP INT
    sleep "$1" &
    candidate_sleep=$!
    wait "$candidate_sleep" || exit 0
    candidate_sleep=
    kill -"$3" "$2" 2>/dev/null || :
    if [ "$3" = TERM ]; then
      sleep 5 &
      candidate_sleep=$!
      wait "$candidate_sleep" || exit 0
      candidate_sleep=
      kill -KILL "$2" 2>/dev/null || :
    fi
  }
  candidate_cleanup() {
    if [ -n "$candidate_child" ]; then
      kill "$candidate_child" 2>/dev/null || :
      candidate_watch 5 "$candidate_child" KILL </dev/null >/dev/null 2>&1 &
      candidate_reaper=$!
      candidate_cleanup_status=0
      wait "$candidate_child" 2>/dev/null || candidate_cleanup_status=$?
      [ "$candidate_cleanup_status" -ne 125 ] || candidate_retain=1
      kill "$candidate_reaper" 2>/dev/null || :
      wait "$candidate_reaper" 2>/dev/null || :
    fi
    [ -z "$candidate_watchdog" ] || { kill "$candidate_watchdog" 2>/dev/null || :; wait "$candidate_watchdog" 2>/dev/null || :; }
    if [ "$candidate_retain" -eq 0 ]; then
      rm -rf -- "$candidate_dir"
    else
      printf 'install: unresolved verifier ownership; preserve private cache %s\n' "$candidate_dir" >&2
    fi
  }
  trap candidate_cleanup 0
  trap 'exit 130' INT
  trap 'exit 143' TERM HUP
  # Finite acquisition commands, bounded files, and no downloaded executable
  # probes. The authenticated Go owner supplies strict per-input bounds again.
  candidate_gh() {
    candidate_output_limit=65536
    case "$1 ${2-}" in
      api\ *) candidate_output_limit=256 ;;
      'release view') candidate_output_limit=512 ;;
      'attestation verify') candidate_output_limit=256 ;;
      'attestation trusted-root') candidate_output_limit=16777216 ;;
    esac
    # Fixed code only; every user operand remains an argv element. Bash is the
    # factory's existing adapter prerequisite and supplies portable job groups.
    # docs/adr/0101-go-release-handoff.md:401.
    (unset BASH_ENV ENV; ulimit -f 524288; exec /bin/bash --noprofile --norc -c '
      set -m
      child=
      stop_group() {
        trap "" INT TERM HUP
        [ -n "$child" ] || return 0
        kill -TERM -- "-$child" 2>/dev/null || :
        for ((attempt=0; attempt<25; attempt++)); do
          kill -0 -- "-$child" 2>/dev/null || { wait "$child" 2>/dev/null || :; child=; return 0; }
          sleep 0.02
        done
        kill -KILL -- "-$child" 2>/dev/null || :
        wait "$child" 2>/dev/null || :
        for ((attempt=0; attempt<50; attempt++)); do
          kill -0 -- "-$child" 2>/dev/null || { child=; return 0; }
          sleep 0.02
        done
        printf "%s\n" "install: verifier process group did not close" >&2
        return 125
      }
      trap "stop_group || exit 125; exit 143" TERM HUP
      trap "stop_group || exit 125; exit 130" INT
      "$@" &
      child=$!
      status=0
      wait "$child" 2>/dev/null || status=$?
      stop_group || exit 125
      exit "$status"
    ' factory-bootstrap "$candidate_verifier" "$@") > "$candidate_dir/command.stdout" 2> "$candidate_dir/command.stderr" &
    candidate_child=$!
    candidate_watch 120 "$candidate_child" TERM </dev/null >/dev/null 2>&1 &
    candidate_watchdog=$!
    candidate_status=0
    wait "$candidate_child" || candidate_status=$?
    [ "$candidate_status" -ne 125 ] || candidate_retain=1
    candidate_child=
    kill "$candidate_watchdog" 2>/dev/null || :
    wait "$candidate_watchdog" 2>/dev/null || :
    candidate_watchdog=
    head -c 65536 "$candidate_dir/command.stderr" >&2
    [ "$(wc -c < "$candidate_dir/command.stdout")" -le "$candidate_output_limit" ] || { printf '%s\n' 'install: oversized candidate metadata' >&2; return 1; }
    return "$candidate_status"
  }
  GH_HOST=github.com GH_PROMPT_DISABLED=1
  export GH_HOST GH_PROMPT_DISABLED
  candidate_repo=anoop2811/software-factory-template
  candidate_gh api "repos/$candidate_repo/commits/refs/tags/$candidate_tag" --jq .sha
  candidate_revision=$(cat "$candidate_dir/command.stdout")
  case "$candidate_revision" in ''|*[!0-9a-f]*) printf '%s\n' 'install: invalid candidate revision' >&2; exit 1 ;; esac
  [ "${#candidate_revision}" -eq 40 ] || exit 1
  candidate_gh release view "$candidate_tag" --repo "$candidate_repo" --json tagName,isDraft --jq '[.tagName,.isDraft]|@tsv'
  candidate_release=$(cat "$candidate_dir/command.stdout")
  [ "$candidate_release" = "$(printf '%s\tfalse' "$candidate_tag")" ] || { printf '%s\n' 'install: candidate release is unavailable' >&2; exit 1; }
  candidate_raw=factory-bootstrap-$candidate_tag-$candidate_target
  candidate_image=factory-installation-$candidate_tag-$candidate_target.tar.gz
  candidate_gh release download "$candidate_tag" --repo "$candidate_repo" --pattern "$candidate_raw" --pattern "$candidate_image" --pattern runtime-attestations.jsonl --dir "$candidate_dir"
  for candidate_file in "$candidate_raw" "$candidate_image" runtime-attestations.jsonl; do
    [ -f "$candidate_dir/$candidate_file" ] && [ ! -L "$candidate_dir/$candidate_file" ] || { printf '%s\n' 'install: incomplete candidate release' >&2; exit 1; }
    candidate_limit=16777216
    case "$candidate_file" in "$candidate_raw") candidate_limit=268435456 ;; "$candidate_image") candidate_limit=134217728 ;; esac
    [ "$(wc -c < "$candidate_dir/$candidate_file")" -le "$candidate_limit" ] || { printf '%s\n' 'install: oversized candidate asset' >&2; exit 1; }
  done
  candidate_gh attestation trusted-root
  cp "$candidate_dir/command.stdout" "$candidate_dir/trusted-root.jsonl"
  if command -v sha256sum >/dev/null 2>&1; then
    candidate_checksum=$(sha256sum "$candidate_dir/$candidate_raw")
  else
    candidate_checksum=$(shasum -a 256 "$candidate_dir/$candidate_raw")
  fi
  candidate_digest=${candidate_checksum%% *}
  case "$candidate_digest" in ''|*[!0-9a-f]*) exit 1 ;; esac
  [ "${#candidate_digest}" -eq 64 ] || exit 1
  candidate_gh attestation verify "$candidate_dir/$candidate_raw" --repo "$candidate_repo" --signer-workflow "$candidate_repo/.github/workflows/runtime-release.yml" --source-digest "$candidate_revision" --source-ref "refs/tags/$candidate_tag" --bundle "$candidate_dir/runtime-attestations.jsonl" --custom-trusted-root "$candidate_dir/trusted-root.jsonl" --deny-self-hosted-runners --format json --jq "any(.[]; any(.verificationResult.statement.subject[]; .name == \"$candidate_raw\" and .digest.sha256 == \"$candidate_digest\"))"
  candidate_verified=$(cat "$candidate_dir/command.stdout")
  [ "$candidate_verified" = true ] || { printf '%s\n' 'install: candidate authentication failed' >&2; exit 1; }
  printf '%s\n' "$candidate_revision" > "$candidate_dir/revision"
  chmod 700 "$candidate_dir/$candidate_raw"
  FACTORY_RELEASE_DIR=$candidate_dir
  export FACTORY_RELEASE_DIR
  # Keep this parent alive to remove its private handoff after the native child.
  # Dash resets asynchronous stdin before a self-duplication; save it first.
  # docs/adr/0101-go-release-handoff.md:508.
  exec 9<&0
  "$candidate_dir/$candidate_raw" "$@" <&9 9<&- &
  candidate_child=$!
  exec 9<&-
  candidate_status=0
  wait "$candidate_child" || candidate_status=$?
  candidate_child=
  exit "$candidate_status"
}
legacy_install() {
# softwareaifactory.sh installer.
#
#   curl -fsSL https://softwareaifactory.sh/install.sh | sh
#       Fetch only. Clones the template into $FACTORY_HOME and stops.
#
#   curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- init
#       Fetch, then run factory-init against the CURRENT directory. Use this
#       from inside the repository you want to govern. The 'init' word is your
#       explicit consent to modify the current directory — the bare command
#       never touches your project.
#
#   curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- upgrade
#       Refresh the machine-wide template cache, then upgrade the repo you're in
#       (a reviewable diff — nothing committed), just like 'init' acts on the
#       current directory. Upgrades THIS repo, not every repo on the machine:
#       each repo owns its committed, governance-gated framework files. Locally,
#       './factory upgrade' does the same thing without curl.
#
#   curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- init --ref main
#       Same as 'init' (or 'upgrade'), but install from the given branch or tag
#       instead of the pinned release — '--ref main' tracks the latest. Works
#       with the bare fetch too. Equivalent to the FACTORY_REF env var, but
#       pipe-safe: the flag can't be mis-attached to curl instead of sh.
#
# What the fetch does, in full:
#   1. Clones the template (shallow, at a pinned ref) into $FACTORY_HOME
#      (default: ~/.software-factory-template)
#   2. Prints the factory-init command (or runs it, with 'init')
#
# What it never does: use sudo, phone home, or execute anything it downloaded
# other than this template's own factory-init when you ask for it.
#
# Prefer to inspect first? So would we — that is rather the point of the
# template. Download this file, read it, then run it:
#   curl -fsSLO https://softwareaifactory.sh/install.sh && less install.sh && sh install.sh
#
# Pinning: the default ref is the pinned tag below (reproducible installs). The
# `--ref <ref>` flag or the FACTORY_REF env var overrides it — e.g. main for the
# latest. From the first tagged release onward the default is that tag, never a
# moving branch.

FACTORY_REPO="${FACTORY_REPO:-https://github.com/anoop2811/software-factory-template}"
FACTORY_REF="${FACTORY_REF:-v0.1.6}"
FACTORY_HOME="${FACTORY_HOME:-$HOME/.software-factory-template}"

# --ref <ref> (or --ref=<ref>): install from the given branch or tag instead of
# the pinned default — e.g. `init --ref main` for the latest. Pulled out of the
# args here so it works before or after the verb, and passes nothing extra to
# factory-init. Precedence: --ref beats the FACTORY_REF env var beats the default.
REF_OVERRIDE=""
_args=""
while [ $# -gt 0 ]; do
  case "$1" in
    --ref)
      if [ $# -lt 2 ]; then
        printf '%s\n' "install: --ref needs a value (e.g. --ref main)" >&2
        exit 2
      fi
      REF_OVERRIDE="$2"; shift 2 ;;
    --ref=*) REF_OVERRIDE="${1#--ref=}"; shift ;;
    *) _args="$_args $1"; shift ;;
  esac
done
# shellcheck disable=SC2086  # deliberate re-split; passthrough args carry no spaces
set -- $_args
[ -n "$REF_OVERRIDE" ] && FACTORY_REF="$REF_OVERRIDE"

DO_INIT=0
DO_UPGRADE=0
case "${1:-}" in
  init) DO_INIT=1; shift ;;   # remaining args (e.g. --pack go) pass to factory-init
  upgrade) DO_UPGRADE=1; shift ;;   # refresh the machine-wide template cache
  "") : ;;
  *)
    printf '%s\n' "install: unknown argument '$1' (did you mean 'init' or 'upgrade'?)" >&2
    exit 2
    ;;
esac

# The directory to govern is where the user invoked the command — captured
# before anything else, since factory-init modifies it.
TARGET_DIR="$PWD"

say() { printf '%s\n' "$*"; }

# A pinned ref that does not exist yet is the one failure this installer can
# predict. Publishing a release is two steps — the pin lands on the default
# branch, the tag is pushed — and between them install.sh names a tag no clone
# can find. git's own message ("Remote branch not found in upstream origin") does
# not say that, and does not say what to do about it.
#
# Deliberately NOT a silent fallback to main: an adopter who asked for a pinned
# install and quietly received an unpinned one has lost the property they came
# for. Say what happened, and let them choose.
require_ref() {
  if git ls-remote --exit-code --heads --tags "$FACTORY_REPO" "$FACTORY_REF" >/dev/null 2>&1; then
    return 0
  fi
  say "install: ref '$FACTORY_REF' was not found in $FACTORY_REPO." >&2
  say "  If a release was just published, its tag may be moments behind — retry shortly." >&2
  say "  Otherwise pick a ref that exists:" >&2
  say "    curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- --ref main" >&2
  exit 1
}

# When this invocation began. Handed to factory-upgrade so the total it prints is
# wall clock from the adopter's command, not from the point the upgrade takes
# over — the fetch happens here, and a total that omitted it would be a smaller
# number than the one the adopter actually waited through.
INSTALL_T0="$(date +%s 2>/dev/null || printf '0')"
# Durations come from scripts/lib/timing.sh once the template is on disk, so
# install, upgrade and doctor phrase them identically — including the hour form
# this fallback lacks. The fallback exists because the very first fetch happens
# before there is any template to source from, and a missing lib must never be
# why an install fails.
elapsed_since() { # <start> -> "4s" | "1m 12s" | "1h 4m"
  if [ -f "$FACTORY_HOME/scripts/lib/timing.sh" ]; then
    # shellcheck source=scripts/lib/timing.sh
    . "$FACTORY_HOME/scripts/lib/timing.sh"
    factory_duration "$1"
    return 0
  fi
  _es_end="$(date +%s 2>/dev/null || printf '0')"
  case "$1$_es_end" in *[!0-9]*) printf 'unknown'; return 0 ;; esac
  _es="$((_es_end - $1))"
  [ "$_es" -lt 0 ] && _es=0
  if [ "$_es" -lt 60 ]; then printf '%ss' "$_es"
  else printf '%dm %ds' "$((_es / 60))" "$((_es % 60))"; fi
}

if ! command -v git >/dev/null 2>&1; then
  say "install: git is required and was not found." >&2
  exit 1
fi

# upgrade: refresh the machine-wide template cache, then apply the framework
# update to the repo you're in (a reviewable diff — nothing is committed). Like
# 'init', it acts on the current directory. It upgrades THIS repo, not every
# repo on the machine — each repo owns its committed, governance-gated framework
# files, so you upgrade them where you are.
if [ "$DO_UPGRADE" -eq 1 ]; then
  if [ -d "$FACTORY_HOME/.git" ]; then
    say "install: refreshing the template at $FACTORY_HOME to '$FACTORY_REF'..."
    _t_fetch="$(date +%s 2>/dev/null || printf '0')"
    git -C "$FACTORY_HOME" fetch --quiet --depth 1 origin "$FACTORY_REF"
    git -C "$FACTORY_HOME" reset --quiet --hard FETCH_HEAD
    say "  refreshed in $(elapsed_since "$_t_fetch")"
  elif [ -e "$FACTORY_HOME" ]; then
    say "install: $FACTORY_HOME exists but is not a template checkout." >&2
    say "  Remove it and re-run, or set FACTORY_HOME to a different path." >&2
    exit 1
  else
    say "install: cloning the template into $FACTORY_HOME..."
    require_ref
    git clone --quiet --depth 1 --branch "$FACTORY_REF" "$FACTORY_REPO" "$FACTORY_HOME"
  fi
  # Apply to the current repo. Resolve its root via git so running from a
  # subdirectory upgrades the whole repo, not just where you happen to stand.
  REPO_ROOT="$( (cd "$TARGET_DIR" && git rev-parse --show-toplevel) 2>/dev/null || true)"
  if [ -n "$REPO_ROOT" ] && [ -f "$REPO_ROOT/factory.yaml" ]; then
    say "install: applying framework updates to $REPO_ROOT ..."
    say ""
    cd "$REPO_ROOT"
    # The upgrade reports the total, so give it the real start.
    FACTORY_UPGRADE_STARTED="$INSTALL_T0"
    export FACTORY_UPGRADE_STARTED
    exec "$FACTORY_HOME/scripts/factory-upgrade.sh" --source "$FACTORY_HOME"
  fi
  say "install: template cache updated. This isn't a factory repo, so nothing was"
  say "  upgraded — run 'upgrade' from inside one, or 'init' to set one up."
  exit 0
fi

if [ -e "$FACTORY_HOME" ]; then
  say "install: $FACTORY_HOME already exists (reusing it)."
  say "  To update it to the latest template:"
  say "    curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- upgrade"
else
  say "install: cloning $FACTORY_REPO at ref '$FACTORY_REF' into $FACTORY_HOME"
  require_ref
  git clone --quiet --depth 1 --branch "$FACTORY_REF" "$FACTORY_REPO" "$FACTORY_HOME"
  say "install: done. Nothing outside $FACTORY_HOME was touched."
fi

if [ "$DO_INIT" -eq 0 ]; then
  say ""
  say "Next, from the repository you want to govern:"
  say ""
  say "    cd your-project && \"$FACTORY_HOME/scripts/factory-init.sh\""
  say ""
  say "Or re-run this installer with 'init' from inside that repository:"
  say "    curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- init"
  exit 0
fi

say ""
say "install: running factory-init against the current directory:"
say "    $TARGET_DIR"
say ""
# factory-init reads its prompts from /dev/tty, so it works through the pipe.
# Any remaining args (e.g. --pack go) pass straight through.
exec "$FACTORY_HOME/scripts/factory-init.sh" "$TARGET_DIR" "$@"
}

# Parse the complete source before a candidate can inherit stdin. In a curl|sh
# invocation unread script text must never become initializer answers.
# docs/adr/0101-go-release-handoff.md:455.
installer_main() {
  for installer_arg do
    case "$installer_arg" in
      --go-candidate|--go-candidate=*) candidate_install "$@"; exit $? ;;
    esac
  done
  legacy_install "$@"
}
installer_main "$@"
