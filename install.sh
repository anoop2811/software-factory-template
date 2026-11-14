#!/bin/sh
set -eu

# Public stable activation remains a separate release step.
# docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:21.
FACTORY_DEFAULT_REF=v0.1.6

# Candidate selection is reserved before the legacy parser (which re-splits its
# arguments). Preserve literal argv and authenticate before the first execution.
# docs/adr/0101-go-release-handoff.md:40, docs/adr/0101-go-release-handoff.md:80.
candidate_install() {
  umask 077
  case "${1-}" in init|upgrade) ;; *) printf '%s\n' 'install: candidate requires init or upgrade' >&2; exit 2 ;; esac
  candidate_tag='' candidate_pending=0 candidate_seen=0 candidate_help=0 candidate_terminated=0
  for candidate_arg do
    if [ "$candidate_pending" -eq 1 ]; then
      candidate_tag=$candidate_arg; candidate_pending=0; continue
    fi
    [ "$candidate_terminated" -eq 0 ] || continue
    case "$candidate_arg" in
      --) candidate_terminated=1 ;;
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
  [ "$candidate_pending" -eq 0 ] && [ "${#candidate_tag}" -le 128 ] || { printf '%s\n' 'install: invalid candidate tag' >&2; exit 2; }
  if [ "$candidate_help" -eq 1 ]; then
    printf '%s\n' 'Usage: install.sh init|upgrade [args] --go-candidate TAG'
    exit 0
  fi
  packaged_install candidate "$candidate_tag" "$@"
}

# Canonical semver is textual; numeric identifiers never enter shell arithmetic.
# Validation's C locale does not change the selected owner's environment.
# docs/adr/0144-ci-installer-bootstrap.md:43.
ci_exact_release() (
  LC_ALL=C
  export LC_ALL
  [ "${#1}" -le 128 ] || return 1
  case "$1" in v[0-9]*) ;; *) return 1 ;; esac
  ci_version=${1#v}
  case "$ci_version" in
    *+*)
      ci_build=${ci_version#*+}
      ci_version=${ci_version%%+*}
      case "$ci_build" in ''|*[!a-zA-Z0-9.-]*|.*|*.|*..*) return 1 ;; esac ;;
  esac
  case "$ci_version" in
    *-*)
      ci_pre=${ci_version#*-}
      ci_version=${ci_version%%-*}
      case "$ci_pre" in ''|*[!a-zA-Z0-9.-]*|.*|*.|*..*) return 1 ;; esac
      while :; do
        ci_identifier=${ci_pre%%.*}
        case "$ci_identifier" in *[!0-9]*) ;; 0?*) return 1 ;; esac
        case "$ci_pre" in *.*) ci_pre=${ci_pre#*.} ;; *) break ;; esac
      done ;;
  esac
  case "$ci_version" in ''|*[!0-9.]*|.*|*.|*..*) return 1 ;; esac
  ci_major=${ci_version%%.*}
  ci_tail=${ci_version#*.}
  [ "$ci_tail" != "$ci_version" ] || return 1
  ci_minor=${ci_tail%%.*}
  ci_patch=${ci_tail#*.}
  [ "$ci_patch" != "$ci_tail" ] || return 1
  case "$ci_patch" in *.*) return 1 ;; esac
  for ci_number in "$ci_major" "$ci_minor" "$ci_patch"; do
    case "$ci_number" in ''|0?*) return 1 ;; esac
  done
)

# Parse in a separate function so the handoff keeps the original quoted argv.
# docs/adr/0144-ci-installer-bootstrap.md:33.
ci_parse() {
  ci_tag='' ci_origin='' ci_ephemeral=0 ci_help=0 ci_seen=' '
  ci_operation='' ci_expected_revision='' ci_output=''
  shift
  if [ "$#" -eq 1 ]; then
    case "$1" in --help|-h) ci_help=1; return 0 ;; esac
  fi
  case "${1-}" in prepare|review-client) ci_operation=$1 ;; *) return 1 ;; esac
  shift
  while [ "$#" -gt 0 ]; do
    ci_name=${1%%=*}
    ci_value=${1#*=}
    case "$1" in *=*) ci_supplied=1 ;; *) ci_supplied=0 ;; esac
    case "$ci_name" in -h) ci_name=--help ;; esac
    case "$ci_seen" in *" $ci_name "*) return 1 ;; esac
    ci_seen="$ci_seen$ci_name "
    case "$ci_name" in
      --help|--ephemeral)
        [ "$ci_supplied" -eq 0 ] || return 1
        [ "$ci_name" != --ephemeral ] || [ "$ci_operation" = prepare ] || return 1
        case "$ci_name" in --help) ci_help=1 ;; *) ci_ephemeral=1 ;; esac ;;
      --go-candidate|--origin-target|--expected-revision|--output)
        if [ "$ci_supplied" -eq 0 ]; then
          shift
          [ "$#" -gt 0 ] || return 1
          ci_value=$1
        fi
        case "$ci_name" in
          --go-candidate)
            ci_exact_release "$ci_value" || return 1
            ci_tag=$ci_value ;;
          --origin-target)
            [ "$ci_operation" = prepare ] || return 1
            case "$ci_value" in linux/amd64|linux/arm64|darwin/amd64|darwin/arm64) ;; *) return 1 ;; esac
            ci_origin=$ci_value ;;
          --expected-revision)
            [ "$ci_operation" = review-client ] || return 1
            case "$ci_value" in ''|*[!0123456789abcdef]*) return 1 ;; esac
            [ "${#ci_value}" -eq 40 ] || return 1
            ci_expected_revision=$ci_value ;;
          --output)
            [ "$ci_operation" = review-client ] || return 1
            case "$ci_value" in /*) ;; *) return 1 ;; esac
            case "$ci_value" in /|*/|*//*|*/./*|*/../*|*/.|*/..) return 1 ;; esac
            ci_output=$ci_value ;;
        esac ;;
      *) return 1 ;;
    esac
    shift
  done
  [ "$ci_help" -eq 1 ] && return 0
  [ -n "$ci_tag" ] || return 1
  if [ "$ci_operation" = review-client ]; then
    [ -n "$ci_expected_revision" ] && [ -n "$ci_output" ]
  else
    [ -n "$ci_origin" ] && [ "$ci_ephemeral" -eq 1 ]
  fi
}
ci_install() {
  if ! ci_parse "$@"; then
    printf '%s\n' 'install ci: invalid arguments; prepare requires an exact release, origin target and --ephemeral; review-client requires an exact release, expected revision and output' >&2
    exit 2
  fi
  if [ "$ci_help" -eq 1 ]; then
    printf '%s\n' 'Usage: install.sh ci prepare --go-candidate TAG --origin-target OS/ARCH --ephemeral'
    printf '%s\n' '       install.sh ci review-client --go-candidate TAG --expected-revision SHA --output DIR'
    exit 0
  fi
  # Public CI cannot inherit the private sourceable bridge dispatch protocol.
  # docs/adr/0144-ci-installer-bootstrap.md:55.
  unset FACTORY_BRIDGE_PROTOCOL
  if [ "$ci_operation" = review-client ]; then
    packaged_install review-client "$ci_tag" "$ci_expected_revision" "$@"
  else
    packaged_install candidate "$ci_tag" "$@"
  fi
}

# Both selectors cross the same first-execution trust and ownership boundary.
# Stable requests keep their stricter publisher policy across the Go handoff.
# docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:88.
packaged_install() {
  candidate_policy=$1 candidate_tag=$2
  shift 2
  # Only the explicit operation passes this guard; inherited CI variables cannot
  # change ordinary installation. docs/adr/0153-authenticated-review-client-bootstrap.md:49.
  candidate_expected_revision=''
  if [ "$candidate_policy" = review-client ]; then
    candidate_expected_revision=$1
    shift
  fi
  umask 077
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
  candidate_retain=0 candidate_guard=0 candidate_cancel_status=0
  candidate_signal_epoch=0 candidate_control_open=0 candidate_watch_serial=0
  candidate_cancel() {
    candidate_cancel_status=$1
    candidate_signal_epoch=$((candidate_signal_epoch + 1))
    if [ "$candidate_guard" -eq 0 ]; then
      candidate_cleanup
      exit "$candidate_cancel_status"
    fi
  }
  candidate_resume_cancel() {
    if [ "$candidate_cancel_status" -ne 0 ]; then
      # Bash 3 unwinds a function's redirections before running its EXIT trap.
      candidate_cleanup
      exit "$candidate_cancel_status"
    fi
  }
  candidate_join() {
    candidate_previous_wait=
    while :; do
      candidate_wait_epoch=$candidate_signal_epoch
      candidate_join_status=0
      wait "$1" 2>/dev/null || candidate_join_status=$?
      if [ "$candidate_join_status" -eq 127 ] && [ -n "$candidate_previous_wait" ]; then
        candidate_join_status=$candidate_previous_wait
        break
      fi
      case "$candidate_join_status" in
        129|130|143)
          [ "$candidate_wait_epoch" -ne "$candidate_signal_epoch" ] || break
          candidate_previous_wait=$candidate_join_status ;;
        *) break ;;
      esac
    done
    return "$candidate_join_status"
  }
  # A fresh channel carries stop authority without a reusable numeric PID.
  # docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:241.
  candidate_prepare_watch() {
    candidate_watch_serial=$((candidate_watch_serial + 1))
    candidate_control=$candidate_dir/watch.$candidate_watch_serial
    mkfifo "$candidate_control" || return 125
    [ -p "$candidate_control" ] && [ ! -L "$candidate_control" ] || return 125
  }
  candidate_watch() {
    (candidate_parent=$$
      trap 'kill -TERM "$candidate_parent" 2>/dev/null || :' 0
      unset BASH_ENV ENV
      exec /bin/bash --noprofile --norc -c '
      case "$5" in ""|*[!0-9]*) exit 125 ;; esac
      [[ "$5" -gt 1 ]] || exit 125
      parent=$5
      failed() {
        failure_status=$?
        trap - EXIT
        if [[ "$failure_status" != 0 ]]; then
          kill -TERM "$parent" 2>/dev/null || :
        fi
        exit "$failure_status"
      }
      trap failed EXIT
      trap "exit 125" INT TERM HUP
      case "$1:$3" in 120:TERM|5:KILL) ;; *) exit 125 ;; esac
      case "$2" in ""|*[!0-9]*) exit 125 ;; esac
      [[ "$2" -gt 1 ]] || exit 125
      [[ -p /dev/fd/9 ]] || exit 125
      pause() {
        status=0
        if ! { IFS= read -r -t "$1" -u 9 stop || status=$?; } 2> "$4"; then
          exit 125
        fi
        if [[ "$status" == 0 ]]; then
          [[ "$stop" == stop ]] || exit 125
          exit 0
        fi
        [[ -p /dev/fd/9 && ! -s "$4" ]] || exit 125
        case "$status" in 1|142) ;; *) exit 125 ;; esac
      }
      pause "$@"
      kill -"$3" "$2" 2>/dev/null || :
      if [[ "$3" == TERM ]]; then
        pause 5 "$2" KILL "$4"
        kill -KILL "$2" 2>/dev/null || :
      fi
    ' factory-watch "$1" "$2" "$3" "$candidate_control.error" "$$") </dev/null >/dev/null 2>&1 &
    candidate_watchdog=$!
  }
  candidate_stop_watch() {
    candidate_saved_guard=$candidate_guard
    candidate_guard=1
    candidate_stop_status=0
    if [ "$candidate_control_open" -ne 0 ]; then
      printf '%s\n' stop >&9 || candidate_stop_status=125
      exec 9>&-
      candidate_control_open=0
    fi
    if [ -n "$candidate_watchdog" ]; then
      candidate_join "$candidate_watchdog" || candidate_stop_status=125
      candidate_watchdog=
    fi
    [ "$candidate_stop_status" -eq 0 ] || candidate_retain=1
    candidate_guard=$candidate_saved_guard
    [ "$candidate_guard" -ne 0 ] || candidate_resume_cancel
    return "$candidate_stop_status"
  }
  candidate_cleanup_child() {
    [ -p /dev/fd/9 ] || return 125
    candidate_control_open=1
    candidate_watch 5 "$candidate_child" KILL
    candidate_cleanup_epoch=$candidate_signal_epoch
    candidate_cleanup_status=0
    wait "$candidate_child" 2>/dev/null || candidate_cleanup_status=$?
    case "$candidate_cleanup_status" in
      129|130|143)
        if [ "$candidate_cleanup_epoch" -ne "$candidate_signal_epoch" ]; then
          # A failed reaper wakes this guarded join; never wait unboundedly.
          candidate_reaper_status=0
          candidate_join "$candidate_watchdog" || candidate_reaper_status=$?
          candidate_watchdog=
          if [ "$candidate_reaper_status" -ne 0 ]; then
            candidate_retain=1
            candidate_stop_watch || :
            return 125
          fi
          candidate_join "$candidate_child" || candidate_cleanup_status=$?
        fi ;;
    esac
    [ "$candidate_cleanup_status" -ne 125 ] || candidate_retain=1
    candidate_child=
    candidate_stop_watch
  }
  candidate_cleanup() {
    trap - 0
    candidate_guard=1
    candidate_stop_watch || candidate_retain=1
    if [ -n "$candidate_child" ]; then
      kill "$candidate_child" 2>/dev/null || :
      if ! candidate_prepare_watch || ! candidate_cleanup_child 9<> "$candidate_control"; then
        candidate_retain=1
        candidate_stop_watch || :
        printf 'install: unresolved child ownership; preserve private cache %s\n' "$candidate_dir" >&2
        exit 125
      fi
    fi
    if [ "$candidate_retain" -eq 0 ]; then
      rm -rf -- "$candidate_dir"
    else
      printf 'install: unresolved verifier ownership; preserve private cache %s\n' "$candidate_dir" >&2
      exit 125
    fi
  }
  trap candidate_cleanup 0
  trap 'candidate_cancel 130' INT
  trap 'candidate_cancel 143' TERM HUP
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
    candidate_guard=1
    if ! candidate_prepare_watch; then
      candidate_retain=1
      candidate_guard=0
      candidate_resume_cancel
      printf '%s\n' 'install: watchdog channel setup failed' >&2
      return 125
    fi
    candidate_channel_entered=0
    candidate_gh_status=0
    candidate_gh_control "$@" 9<> "$candidate_control" || candidate_gh_status=$?
    candidate_guard=0
    if [ "$candidate_channel_entered" -eq 0 ]; then
      candidate_retain=1
      printf '%s\n' 'install: watchdog channel open failed' >&2
      return 125
    fi
    candidate_resume_cancel
    return "$candidate_gh_status"
  }
  candidate_gh_control() {
    candidate_channel_entered=1
    [ -p /dev/fd/9 ] || { candidate_retain=1; return 125; }
    candidate_control_open=1
    (unset BASH_ENV ENV; ulimit -f 524288; exec /bin/bash --noprofile --norc -c '
      set -m
      child= admission_guard=1 cancel_status=0
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
      # Retain cancellation until the admitted verifier has an owned PID.
      # docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:301.
      cancel() {
        cancel_status=$1
        if [ "$admission_guard" -eq 0 ]; then
          stop_group || exit 125
          exit "$cancel_status"
        fi
      }
      trap "cancel 143" TERM HUP
      trap "cancel 130" INT
      "$@" &
      child=$!
      admission_guard=0
      [ "$cancel_status" -eq 0 ] || cancel "$cancel_status"
      status=0
      wait "$child" 2>/dev/null || status=$?
      stop_group || exit 125
      exit "$status"
    ' factory-bootstrap "$candidate_verifier" "$@") 9>&- > "$candidate_dir/command.stdout" 2> "$candidate_dir/command.stderr" &
    candidate_child=$!
    candidate_watch 120 "$candidate_child" TERM
    candidate_guard=0
    candidate_resume_cancel
    candidate_status=0
    wait "$candidate_child" || candidate_status=$?
    [ "$candidate_status" -ne 125 ] || candidate_retain=1
    candidate_child=
    if ! candidate_stop_watch; then
      candidate_retain=1
      printf '%s\n' 'install: watchdog control did not close' >&2
      return 125
    fi
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
  if [ "$candidate_policy" = review-client ] && [ "$candidate_revision" != "$candidate_expected_revision" ]; then
    printf '%s\n' 'install ci review-client: selected source revision does not match expected revision' >&2
    exit 1
  fi
  candidate_fields=tagName,isDraft
  candidate_query='[.tagName,.isDraft]|@tsv'
  candidate_expected=$(printf '%s\tfalse' "$candidate_tag")
  if [ "$candidate_policy" = stable ]; then
    candidate_fields=tagName,isDraft,isPrerelease
    candidate_query='[.tagName,.isDraft,.isPrerelease]|@tsv'
    candidate_expected=$(printf '%s\tfalse\tfalse' "$candidate_tag")
  fi
  candidate_gh release view "$candidate_tag" --repo "$candidate_repo" --json "$candidate_fields" --jq "$candidate_query"
  candidate_release=$(cat "$candidate_dir/command.stdout")
  [ "$candidate_release" = "$candidate_expected" ] || { printf 'install: %s release is unavailable\n' "$candidate_policy" >&2; exit 1; }
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
FACTORY_REF="${FACTORY_REF:-$FACTORY_DEFAULT_REF}"
FACTORY_HOME="${FACTORY_HOME:-$HOME/.software-factory-template}"

# --ref <ref> (or --ref=<ref>): install from the given branch or tag instead of
# the pinned default — e.g. `init --ref main` for the latest. Pulled out of the
# args here so it works before or after the verb, and passes nothing extra to
# factory-init. Precedence: --ref beats the FACTORY_REF env var beats the default.
REF_OVERRIDE=""
# Rotate retained original operands to the tail without evaluating or splitting.
# docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:74.
installer_remaining=$#
installer_literal=0
while [ "$installer_remaining" -gt 0 ]; do
  [ "$1" != -- ] || installer_literal=1
  case "$installer_literal:$1" in
    0:--ref)
      if [ "$installer_remaining" -lt 2 ]; then
        printf '%s\n' "install: --ref needs a value (e.g. --ref main)" >&2
        exit 2
      fi
      REF_OVERRIDE="$2"; shift 2
      installer_remaining=$((installer_remaining - 2)) ;;
    0:--ref=*) REF_OVERRIDE="${1#--ref=}"; shift
      installer_remaining=$((installer_remaining - 1)) ;;
    *) installer_argument=$1; shift
      set -- "$@" "$installer_argument"
      installer_remaining=$((installer_remaining - 1)) ;;
  esac
done
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
# Bootstrap timing must not execute fetched source before native handoff.
# docs/adr/0137-go-source-shared-helpers.md:67.
elapsed_since() { # <start> -> "4s" | "1m 12s"
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

installer_source_adapter() {
  [ -f "$FACTORY_HOME/scripts/factory-init.sh" ] &&
    [ "$(sed -n '2p' "$FACTORY_HOME/scripts/factory-init.sh")" = '# Factory native initializer adapter: source-v1' ]
}

installer_upgrade_adapter() {
  [ -f "$FACTORY_HOME/scripts/factory-upgrade.sh" ] &&
    [ "$(sed -n '2p' "$FACTORY_HOME/scripts/factory-upgrade.sh")" = '# Factory native upgrade adapter: source-v1' ]
}

installer_branch_ref() {
  case "$FACTORY_REF" in v[0-9]*) return 1 ;; *) return 0 ;; esac
}

installer_legacy_operands_refusal() {
  printf '%s\n' 'install: the selected legacy adapter does not support -- operands; select an explicit Go release' >&2
  return 2
}

installer_upgrade_operands() {
  [ "$installer_literal" -eq 0 ] || installer_upgrade_adapter || installer_legacy_operands_refusal
}

installer_quote() {
  printf "'"
  printf '%s' "$1" | sed "s/'/'\\\\''/g"
  printf "'"
}

# Metadata reads are isolated; refresh retains explicit transport settings.
# Both ignore caller Git redirection. Variable names are data, never shell code.
# docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:265.
installer_git_at() (
  installer_git_root=$1
  installer_git_mode=$2
  shift 2
  for installer_git_name in $(/usr/bin/env | sed -n 's/^\(GIT_[A-Za-z0-9_]*\)=.*/\1/p'); do
    if [ "$installer_git_mode" = refresh ]; then
      case "$installer_git_name" in
        GIT_ASKPASS|GIT_TERMINAL_PROMPT|GIT_SSH|GIT_SSH_COMMAND|GIT_SSH_VARIANT|\
        GIT_SSL_CAINFO|GIT_SSL_CAPATH|GIT_SSL_CERT|GIT_SSL_KEY|\
        GIT_SSL_CERT_PASSWORD_PROTECTED|GIT_SSL_CERT_TYPE|GIT_SSL_KEY_TYPE|\
        GIT_SSL_VERSION|GIT_SSL_CIPHER_LIST|GIT_SSL_NO_VERIFY|\
        GIT_HTTP_PROXY_AUTHMETHOD|GIT_PROXY_SSL_CAINFO|GIT_PROXY_SSL_CERT|\
        GIT_PROXY_SSL_KEY|GIT_PROXY_SSL_CERT_PASSWORD_PROTECTED|GIT_PROXY_COMMAND)
          continue ;;
      esac
    fi
    unset "$installer_git_name"
  done
  case "$installer_git_mode" in
    identity) export LC_ALL=C GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null ;;
    refresh) : ;;
    *) exit 2 ;;
  esac
  exec git -C "$installer_git_root" --no-optional-locks -c core.fsmonitor=false -c core.untrackedCache=false -c core.hooksPath=/dev/null "$@"
)

installer_cache_git() {
  installer_git_at "$FACTORY_HOME" "$@"
}

installer_native_home() {
  FACTORY_HOME="$(CDPATH='' cd -P -- "$FACTORY_HOME" && pwd -P && printf '.')" || return 2
  FACTORY_HOME=${FACTORY_HOME%??}
}

installer_native_history() {
  # docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:169.
  installer_cache_origin || return 2
  installer_shallow=$(installer_cache_git identity rev-parse --is-shallow-repository) || return 1
  case "$installer_shallow" in
    false) return 0 ;;
    true)
      say 'install: obtaining complete native source history for predecessor qualification...'
      installer_cache_git refresh fetch --quiet --unshallow origin ;;
    *) printf '%s\n' 'install: cannot establish native source history completeness' >&2; return 1 ;;
  esac
}

installer_select_fetched_cache() {
  # docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:96.
  installer_branch_status=0
  installer_cache_git refresh ls-remote --exit-code --heads origin "$FACTORY_REF" >/dev/null || installer_branch_status=$?
  case "$installer_branch_status" in
    0)
      installer_cache_branch=${FACTORY_REF#refs/heads/}
      installer_cache_git refresh checkout --quiet -B "$installer_cache_branch" FETCH_HEAD ;;
    2) installer_cache_git refresh checkout --quiet --detach FETCH_HEAD ;;
    *) return "$installer_branch_status" ;;
  esac
}

installer_cache_refusal() {
  printf '%s\n' 'install: cached source does not match the requested repository/ref.' \
    'Preserve this cache; select a fresh FACTORY_HOME or refresh it safely before retrying.' >&2
  return 2
}

installer_cache_origin() {
  installer_origin=$(installer_cache_git identity remote get-url origin 2>/dev/null) || { installer_cache_refusal; return 2; }
  [ "$installer_origin" != "$FACTORY_REPO" ] || return 0
  case "$FACTORY_REPO:$installer_origin" in
    https://github.com/anoop2811/software-factory-template:https://github.com/anoop2811/software-factory-template.git|\
    https://github.com/anoop2811/software-factory-template.git:https://github.com/anoop2811/software-factory-template) return 0 ;;
  esac
  installer_cache_refusal
}

installer_cache_selection() {
  installer_cache_origin || return 2
  installer_expected=$(installer_cache_git identity rev-parse --verify --end-of-options "$FACTORY_REF^{commit}" 2>/dev/null) || { installer_cache_refusal; return 2; }
  installer_actual=$(installer_cache_git identity rev-parse --verify 'HEAD^{commit}' 2>/dev/null) || { installer_cache_refusal; return 2; }
  case "$installer_actual" in ''|*[!0-9a-f]*) installer_cache_refusal; return 2 ;; esac
  [ "${#installer_actual}" -eq 40 ] && [ "$installer_expected" = "$installer_actual" ] || { installer_cache_refusal; return 2; }
}

# upgrade: refresh the machine-wide template cache, then apply the framework
# update to the repo you're in (a reviewable diff — nothing is committed). Like
# 'init', it acts on the current directory. It upgrades THIS repo, not every
# repo on the machine — each repo owns its committed, governance-gated framework
# files, so you upgrade them where you are.
if [ "$DO_UPGRADE" -eq 1 ]; then
  if [ -d "$FACTORY_HOME/.git" ]; then
    installer_safe_refresh=0
    if installer_branch_ref || installer_source_adapter || installer_upgrade_adapter; then
      installer_safe_refresh=1
      installer_cache_origin || exit 2
      installer_dirty=$(installer_cache_git identity status --porcelain --untracked-files=all) || exit 2
      if [ -n "$installer_dirty" ]; then
        printf '%s\n' 'install: cached source has local changes; preserve them and select a clean FACTORY_HOME before refreshing.' >&2
        exit 2
      fi
    fi
    say "install: refreshing the template at $FACTORY_HOME to '$FACTORY_REF'..."
    _t_fetch="$(date +%s 2>/dev/null || printf '0')"
    if [ "$installer_safe_refresh" -eq 1 ]; then
      installer_cache_git refresh fetch --quiet --depth 1 origin "$FACTORY_REF"
      installer_select_fetched_cache
    else
      git -C "$FACTORY_HOME" fetch --quiet --depth 1 origin "$FACTORY_REF"
      git -C "$FACTORY_HOME" reset --quiet --hard FETCH_HEAD
    fi
    installer_upgrade_operands || exit 2
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
  installer_upgrade_operands || exit 2
  # Apply to the current repo. Resolve its root via git so running from a
  # subdirectory upgrades the whole repo, not just where you happen to stand.
  if installer_upgrade_adapter; then
    # Keep physical cache identity and original caller cwd separate.
    # docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:104.
    installer_native_home || exit 2
    if installer_root_frame="$(installer_git_at "$TARGET_DIR" identity rev-parse --show-toplevel 2>/dev/null && printf '.')"; then
      REPO_ROOT=${installer_root_frame%??}
    else
      REPO_ROOT=''
    fi
  else
    REPO_ROOT="$( (cd "$TARGET_DIR" && git rev-parse --show-toplevel) 2>/dev/null || true)"
  fi
  if [ -n "$REPO_ROOT" ] && [ -f "$REPO_ROOT/factory.yaml" ]; then
    say "install: applying framework updates to $REPO_ROOT ..."
    say ""
    # The upgrade reports the total, so give it the real start.
    FACTORY_UPGRADE_STARTED="$INSTALL_T0"
    export FACTORY_UPGRADE_STARTED
    if installer_upgrade_adapter; then
      installer_native_history || exit $?
      exec "$FACTORY_HOME/scripts/factory-upgrade.sh" --local --source "$FACTORY_HOME" "$@"
    fi
    cd "$REPO_ROOT"
    exec "$FACTORY_HOME/scripts/factory-upgrade.sh" --source "$FACTORY_HOME"
  fi
  say "install: template cache updated. This isn't a factory repo, so nothing was"
  say "  upgraded — run 'upgrade' from inside one, or 'init' to set one up."
  exit 0
fi

if [ -e "$FACTORY_HOME" ]; then
  if [ "$DO_INIT" -eq 1 ] && { installer_branch_ref || installer_source_adapter; }; then
    installer_cache_selection || exit 2
  fi
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
  installer_ref_kind "$FACTORY_REF"
  say ""
  say "Next, from the repository you want to govern:"
  say ""
  if [ "$installer_kind" = go ]; then
    # Git-only fetch did not authenticate runnable release artifacts.
    # docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:80.
    say "    curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- init --ref $FACTORY_REF"
    say ""
    say "Go initialization authenticates official release artifacts before execution."
    exit 0
  fi
  if installer_source_adapter; then
    installer_native_home || exit 2
    printf '    curl -fsSL https://softwareaifactory.sh/install.sh | FACTORY_HOME='
    installer_quote "$FACTORY_HOME"
    printf ' FACTORY_REPO='
    installer_quote "$FACTORY_REPO"
    printf ' sh -s -- init --ref '
    installer_quote "$FACTORY_REF"
    printf '\n'
    say ""
    say "Installer init obtains native source history before building the selected committed checkout."
    exit 0
  else
    say "    cd your-project && \"$FACTORY_HOME/scripts/factory-init.sh\""
  fi
  say ""
  say "Or re-run this installer with 'init' from inside that repository:"
  say "    curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- init"
  exit 0
fi

[ "$installer_literal" -eq 0 ] || installer_source_adapter || { installer_legacy_operands_refusal; exit 2; }

say ""
say "install: running factory-init against the current directory:"
say "    $TARGET_DIR"
say ""
# factory-init reads its prompts from /dev/tty, so it works through the pipe.
# Any remaining args (e.g. --pack go) pass straight through.
if installer_source_adapter; then
  # An explicitly fetched branch is the selected committed developer source.
  # Do not silently substitute the native command's prepared stable default.
  # docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:74.
  installer_native_home || exit 2
  installer_native_history || exit $?
  exec "$FACTORY_HOME/scripts/factory-init.sh" "$TARGET_DIR" --local --source "$FACTORY_HOME" "$@"
fi
exec "$FACTORY_HOME/scripts/factory-init.sh" "$TARGET_DIR" "$@"
}

# Classify without shell arithmetic on version components. Native Go repeats
# this validation; execution routing must never turn an invalid Go ref into Git.
# docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:43.
installer_ref_kind() {
  installer_kind=legacy
  case "$1" in v[0-9]*) ;; *) return 0 ;; esac
  installer_kind=invalid
  [ "${#1}" -le 128 ] || return 0
  ref_version=${1#v}
  case "$ref_version" in
    *+*)
      ref_build=${ref_version#*+}
      ref_version=${ref_version%%+*}
      case "$ref_build" in ''|*[!a-zA-Z0-9.-]*|.*|*.|*..*) return 0 ;; esac ;;
  esac
  case "$ref_version" in ''|*[!0-9.]*|.*|*.|*..*) return 0 ;; esac
  ref_major=${ref_version%%.*}
  ref_tail=${ref_version#*.}
  [ "$ref_tail" != "$ref_version" ] || return 0
  ref_minor=${ref_tail%%.*}
  ref_patch=${ref_tail#*.}
  [ "$ref_patch" != "$ref_tail" ] || return 0
  case "$ref_patch" in *.*) return 0 ;; esac
  for ref_number in "$ref_major" "$ref_minor" "$ref_patch"; do
    case "$ref_number" in ''|0?*) return 0 ;; esac
  done
  case "$ref_major:$ref_minor" in 0:0|0:1) installer_kind=legacy ;; *) installer_kind=go ;; esac
}

# Remove only actual pre-terminator selector tokens, rotating the original argv
# instead of serializing it. Quoting and empty operands survive unchanged.
# docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:104.
stable_install() {
  stable_remaining=$# stable_terminated=0
  while [ "$stable_remaining" -gt 0 ]; do
    stable_value=$1
    shift
    stable_remaining=$((stable_remaining - 1))
    if [ "$stable_terminated" -eq 0 ]; then
      case "$stable_value" in
        --ref) shift; stable_remaining=$((stable_remaining - 1)); continue ;;
        --ref=*) continue ;;
        --) stable_terminated=1 ;;
      esac
    fi
    set -- "$@" "$stable_value"
  done
  case "${1-}" in init|upgrade) ;; *) printf '%s\n' 'install: Go bootstrap requires init or upgrade' >&2; exit 2 ;; esac
  stable_verb=$1
  shift
  packaged_install stable "$installer_ref" "$stable_verb" --ref "$installer_ref" "$@"
}

# Parse the complete source before a candidate can inherit stdin. In a curl|sh
# invocation unread script text must never become initializer answers.
# docs/adr/0101-go-release-handoff.md:455.
installer_main() {
  # This complete public route never enters legacy or implicit-ref routing.
  # docs/adr/0144-ci-installer-bootstrap.md:28.
  if [ "${1-}" = ci ]; then ci_install "$@"; return; fi
  installer_ref=${FACTORY_REF:-$FACTORY_DEFAULT_REF}
  installer_ref_count=0 installer_pending='' installer_terminated=0
  installer_candidate=0 installer_help=0 installer_conflict=0 installer_first=0 installer_verb=''
  for installer_arg do
    if [ -n "$installer_pending" ]; then
      if [ "$installer_pending" = ref ]; then installer_ref=$installer_arg; fi
      installer_pending=''
      continue
    fi
    [ "$installer_terminated" -eq 0 ] || continue
    case "$installer_arg" in
      --) installer_terminated=1 ;;
      --ref) installer_ref_count=$((installer_ref_count + 1)); installer_pending=ref ;;
      --ref=*) installer_ref_count=$((installer_ref_count + 1)); installer_ref=${installer_arg#--ref=} ;;
      --go-candidate) installer_candidate=1; installer_pending=candidate ;;
      --go-candidate=*) installer_candidate=1 ;;
      --help|-h) installer_help=1 ;;
      --source|--source=*|--local|--local=*|--installation|--installation=*) installer_conflict=1 ;;
      *)
        if [ "$installer_first" -eq 0 ]; then installer_verb=$installer_arg; installer_first=1; fi ;;
    esac
  done
  if [ "$installer_candidate" -eq 1 ]; then candidate_install "$@"; return; fi
  if [ "$installer_pending" = ref ] || [ "$installer_ref_count" -gt 1 ] || [ -z "$installer_ref" ]; then
    printf '%s\n' 'install: select one nonempty --ref value' >&2
    exit 2
  fi
  case "$installer_ref" in
    -*) printf '%s\n' 'install: --ref requires a value, not an option' >&2; exit 2 ;;
  esac
  installer_ref_kind "$installer_ref"
  # Legacy word splitting must never repair a malformed Go verb into execution.
  # docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:173.
  case "$installer_kind:$installer_first:$installer_verb" in
    go:1:init|go:1:upgrade|invalid:1:init|invalid:1:upgrade) ;;
    go:1:*|invalid:1:*) printf '%s\n' 'install: Go bootstrap requires literal init or upgrade' >&2; exit 2 ;;
  esac
  case "$installer_verb:$installer_kind" in
    init:invalid|upgrade:invalid)
      printf '%s\n' 'install: --ref requires an exact stable release tag; use --go-candidate for prereleases' >&2
      exit 2 ;;
    init:go|upgrade:go)
      [ "$installer_conflict" -eq 0 ] || { printf '%s\n' 'install: conflicting Go release source' >&2; exit 2; }
      case "${FACTORY_REPO-}" in
        ''|https://github.com/anoop2811/software-factory-template|https://github.com/anoop2811/software-factory-template.git) ;;
        *) printf '%s\n' 'install: Go releases require the official repository' >&2; exit 2 ;;
      esac
      if [ "$installer_help" -eq 1 ]; then
        printf '%s\n' 'Usage: install.sh init|upgrade [args] --ref TAG' 'Select an authenticated stable Go release at or after v0.2.0.'
        exit 0
      fi
      stable_install "$@"
      return ;;
  esac
  # Frozen legacy controllers cannot preserve end-of-options operands safely.
  # Authenticated paths already returned with their literal argv intact.
  # docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:164.
  if [ "$installer_terminated" -ne 0 ]; then
    case "$installer_ref" in
      v[0-9]*) printf '%s\n' 'install: legacy releases do not support -- operands; select an explicit Go release' >&2; exit 2 ;;
    esac
  fi
  legacy_install "$@"
}
installer_main "$@"
