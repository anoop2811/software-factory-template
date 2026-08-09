#!/usr/bin/env bash
set -euo pipefail

# scripts/factory-upgrade.sh (Decision 16)
# Framework-only upgrade. Re-fetches the template and copies the byte-identical
# framework files — hooks, scripts, the factory dispatcher, factory-doctor,
# .githooks, and the framework docs — over this repo. Thanks to Decision 2 the
# hooks carry no placeholders, so upgrading them is a clean copy.
#
# It NEVER touches your factory.yaml, your content (wiki/ pages, memory/lessons,
# specs, docs/DECISION_LOG.md), or your identity/customizable files
# (opencode.json, the agent prompts, AGENTS.md, README, CODEOWNERS, Makefile).
# Those it only *reports*, so you can reconcile upstream changes yourself.
# Everything lands as an uncommitted diff for you to review.
#
# Usage: factory upgrade [--ref <tag>] [--source <dir>]
#   --ref <tag>     template ref to upgrade to (default: $FACTORY_REF or main)
#   --source <dir>  use an existing template checkout instead of fetching

FACTORY_REPO="${FACTORY_REPO:-https://github.com/anoop2811/software-factory-template}"
FACTORY_REF="${FACTORY_REF:-main}"

SOURCE=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --ref) FACTORY_REF="${2:-}"; shift 2 ;;
    --ref=*) FACTORY_REF="${1#*=}"; shift ;;
    --source) SOURCE="${2:-}"; shift 2 ;;
    --source=*) SOURCE="${1#*=}"; shift ;;
    *) echo "factory upgrade: unknown argument '$1'" >&2; exit 2 ;;
  esac
done

# Re-entrancy guard. Upgrade runs factory doctor, doctor proves itself with the
# self-test, and the self-test exercises upgrade — so an unguarded run recurses
# without bound and forks until the machine gives up. Remember whether we are
# already inside an upgrade, then mark that we are.
UPGRADE_NESTED="${FACTORY_UPGRADE_ACTIVE:-}"
export FACTORY_UPGRADE_ACTIVE=1

# Whether the hand-off below already happened. It bounds the hand-off and
# NOTHING else — in particular it does not, and must not, influence nestedness.
#
# It once did, and that was a fork bomb. The parent passed its own nestedness
# across as a second variable, and environment variables are inherited by every
# descendant: the doctor proof spawns a self-test that runs upgrade fixtures with
# an explicit FACTORY_UPGRADE_ACTIVE=1, those fixtures read the stale handshake,
# concluded they were top-level, ran the doctor, and recursed —
# doctor -> self-test -> upgrade -> doctor, forking until the machine gave up.
# It ran on an adopter's machine.
#
# So nestedness has exactly one source, above, and the hand-off restores the
# environment to what the caller had before exec'ing, rather than describing it.
# One fact, one variable, derived the same way in every invocation.
HANDED_OFF="${FACTORY_UPGRADE_REEXEC:-}"
unset FACTORY_UPGRADE_REEXEC

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [ ! -f factory.yaml ]; then
  echo "factory upgrade: no factory.yaml here — is this a factory repo? Run 'factory init' first." >&2
  exit 1
fi

# Sourced from the repo's own copy, and only after factory.yaml is known to
# exist. It is sourced again after the framework copy below, because upgrade
# exists partly to repair a repo whose lib is missing or stale: at this point the
# functions may be absent or old, and by then they are present and current.
# shellcheck source=lib/config.sh
[ -f scripts/lib/config.sh ] && . scripts/lib/config.sh
# shellcheck source=lib/timing.sh
[ -f scripts/lib/timing.sh ] && . scripts/lib/timing.sh

# When this run began. Inherited across the hand-off, so the total reported at
# the end is the wall clock from the adopter's command — not from whichever half
# of the run happens to print it. A timer that restarts mid-run reports a number
# nobody asked for.
if command -v factory_now >/dev/null 2>&1; then
  T_START="${FACTORY_UPGRADE_STARTED:-$(factory_now)}"
  export FACTORY_UPGRADE_STARTED="$T_START"
else
  T_START=""
fi
took() { # took <stage-start> -> " in 4s", or nothing if timing is unavailable
  [ -n "$T_START" ] && command -v factory_duration >/dev/null 2>&1 || return 0
  printf ' in %s' "$(factory_duration "$1")"
}

# ── Get the template at the target ref ───────────────────────────────
CLEANUP=""
if [ -n "$SOURCE" ]; then
  TEMPLATE="$SOURCE"
  [ -d "$TEMPLATE" ] || { echo "factory upgrade: --source '$TEMPLATE' is not a directory" >&2; exit 1; }
  echo "factory upgrade: using local template at $TEMPLATE"
else
  TMP="$(mktemp -d)"
  CLEANUP="$TMP"
  echo "factory upgrade: fetching $FACTORY_REPO at ref '$FACTORY_REF'..."
  T_FETCH="${T_START:-}"
  [ -n "$T_START" ] && T_FETCH="$(factory_now)"
  git clone --quiet --depth 1 --branch "$FACTORY_REF" "$FACTORY_REPO" "$TMP/template"
  TEMPLATE="$TMP/template"
  [ -n "$T_START" ] && echo "  fetched$(took "$T_FETCH")"
fi

# ── Framework files: byte-identical, safe to overwrite ───────────────
FRAMEWORK="
factory
.githooks/pre-push
scripts/lib/config.sh
scripts/lib/roles.sh
scripts/lib/events.sh
scripts/lib/hookspath.sh
scripts/lib/color.sh
scripts/lib/timing.sh
scripts/selftest/run.sh
scripts/factory-doctor.sh
scripts/factory-upgrade.sh
scripts/factory-report.sh
scripts/factory-metrics.sh
templates/metrics.html
scripts/factory-review-lane.sh
scripts/factory-migrate-config.sh
scripts/adversarial-review.sh
packs/review-lane/review-pr.yml
scripts/pre-push-check.sh
scripts/prereq-check.sh
scripts/citation-lint.sh
scripts/golden-task-eval.sh
eval/README.md
eval/runners/mock.sh
eval/runners/example-harness.sh
eval/golden-tasks/reference-answer/task.md
eval/golden-tasks/reference-answer/verify.sh
scripts/harness-structural-eval.sh
scripts/sync-opencode.sh
scripts/sync-claude.sh
scripts/sync-codex.sh
"

# Carried across the hand-off. The first pass does the copying and the second
# pass reports, so a counter that started at zero here would tell the adopter
# "0 file(s) updated" immediately after listing two dozen updates.
copied="${FACTORY_UPGRADE_COPIED:-0}"
case "$copied" in ''|*[!0-9]*) copied=0 ;; esac
# What THIS pass copied, as distinct from the running total. The summary wants
# the total; the stage line wants the work just done, or it claims the second
# pass did what the first one did.
copied_here=0
# Remembered before the copy, so we can tell afterwards whether this very script
# was one of the files replaced.
SELF_BEFORE=""
[ -f scripts/factory-upgrade.sh ] && SELF_BEFORE="$(cksum < scripts/factory-upgrade.sh 2>/dev/null || true)"

# copy_framework <dest-relative-path> [source-relative-path]
#
# The source defaults to the same path inside the template, which holds for
# everything the template keeps where the adopter keeps it. Pack dialect gates
# are the exception: they live in packs/<lang>/hooks/ upstream and install into
# scripts/hooks/, so they must pass their real source. Without that this
# function looked for $TEMPLATE/scripts/hooks/<gate>.sh, found nothing, and
# returned success — pack gates were silently never upgraded.
copy_framework() {
  local rel="$1"
  local src="$TEMPLATE/${2:-$rel}"
  [ -f "$src" ] || return 0
  # Add or refresh the framework file. It carries no install-time placeholders,
  # so a copy is byte-identical by design (Decision 2). Missing files are added,
  # not skipped: a repo installed before a framework file existed (e.g. a new
  # lib that shipped scripts now source) must receive it, or those scripts break.
  # Only the parent directory must already exist, which init guarantees.
  mkdir -p "$(dirname "$rel")" 2>/dev/null || return 0
  if [ -f "$rel" ] && cmp -s "$src" "$rel"; then
    return 0
  fi
  local verb="updated"
  [ -f "$rel" ] || verb="added  "
  # Atomic replace via rename: mv swaps the inode, so if we are updating a file
  # currently being read (this script upgrading itself), the running process
  # keeps reading the old inode and the new version lands for next time.
  cp "$src" "$rel.factory-tmp.$$"
  mv -f "$rel.factory-tmp.$$" "$rel"
  echo "  $verb: $rel"
  copied=$((copied + 1))
  copied_here=$((copied_here + 1))
}

echo "Upgrading framework files..."
T_COPY=""
[ -n "$T_START" ] && T_COPY="$(factory_now)"
for f in $FRAMEWORK; do copy_framework "$f"; done

# All core hooks the template ships.
for src in "$TEMPLATE"/scripts/hooks/*.sh; do
  [ -f "$src" ] || continue
  copy_framework "scripts/hooks/$(basename "$src")"
done

# Installed pack hooks (dialect gates), for any pack this repo uses.
LP="$(sed -n 's/^language_packs:[[:space:]]*//p' factory.yaml | head -1 | tr -d '"')"
for lang in $LP; do
  for src in "$TEMPLATE"/packs/"$lang"/hooks/*.sh; do
    [ -f "$src" ] || continue
    copy_framework "scripts/hooks/$(basename "$src")" "packs/$lang/hooks/$(basename "$src")"
  done
done

# Restore executable bits on scripts.
chmod +x factory scripts/*.sh scripts/hooks/*.sh .githooks/pre-push 2>/dev/null || true
# The eval scaffold too: a runner or an oracle that arrives without its execute
# bit is a task that cannot be scored, which reads as a failing agent rather
# than a broken install.
chmod +x eval/runners/*.sh eval/golden-tasks/*/verify.sh 2>/dev/null || true
# Silent when it copied nothing — the second pass of a hand-off always does,
# and "0 file(s) copied" under an empty list is noise, not information.
[ -n "$T_COPY" ] && [ "$copied_here" -gt 0 ] && echo "  $copied_here file(s) copied$(took "$T_COPY")"

# Re-source the config lib now that the copy has run. A repo whose lib was
# missing or stale is one upgrade is meant to repair, and the questions below
# call into it — so they must use the version that just landed, not the one that
# may not have existed when this script started.
# shellcheck source=lib/config.sh
[ -f scripts/lib/config.sh ] && . scripts/lib/config.sh

# ── Hand off to the newer copy of this script, once ──────────────────
# The script running the copy is the one the repo already had. Everything below
# this point — which capabilities to offer, which migrations to propose — is
# logic that ships WITH a release, so an adopter upgrading across versions would
# be asked the old release's questions and never hear about the new ones. Two
# consecutive upgrades cured it, which is a fine explanation and a poor default:
# adopters skip versions, and the second run is exactly the one nobody does.
#
# So: if the copy replaced this script, exec the new one and let it ask. Bounded
# to a single hand-off by $HANDED_OFF — a shell variable, not an exported one,
# so the bound is scoped to this process and cannot be inherited by anything
# this run spawns. The re-run's copy pass is a no-op, since every file now
# matches the template.
SELF_AFTER=""
[ -f scripts/factory-upgrade.sh ] && SELF_AFTER="$(cksum < scripts/factory-upgrade.sh 2>/dev/null || true)"
if [ -z "$HANDED_OFF" ] && [ -n "$SELF_AFTER" ] && [ "$SELF_BEFORE" != "$SELF_AFTER" ]; then
  echo ""
  echo "factory upgrade: the upgrade script itself was updated — continuing with the new one."
  export FACTORY_UPGRADE_REEXEC=1
  # Put FACTORY_UPGRADE_ACTIVE back the way the caller had it, so the child
  # derives its nestedness exactly as any other invocation does — from the one
  # variable that means it. Describing nestedness in a second variable is what
  # made this recurse; restoring the environment cannot go stale.
  # UPGRADE_NESTED holds exactly what the caller had, captured before this run
  # set the flag — so restoring it means assigning that value back, not a
  # stand-in for it. Only emptiness is tested today, which is precisely why a
  # hard-coded 1 would drift from the comment without ever failing a test.
  if [ -z "$UPGRADE_NESTED" ]; then
    unset FACTORY_UPGRADE_ACTIVE
  else
    export FACTORY_UPGRADE_ACTIVE="$UPGRADE_NESTED"
  fi
  # Hand over the checkout we already have, so the second pass neither clones
  # again nor risks resolving the ref differently — and hand over the ref itself,
  # which is a label rather than a lookup once --source is set. Without it the
  # child falls back to the default and writes `ref=main` into .factory-version
  # for an adopter who asked for a tag: the tree would be right and the record
  # would be wrong, which is worse than either.
  [ -n "$CLEANUP" ] && export FACTORY_UPGRADE_CLEANUP="$CLEANUP"
  export FACTORY_UPGRADE_COPIED="$copied"
  exec bash scripts/factory-upgrade.sh --ref "$FACTORY_REF" --source "$TEMPLATE"
fi

# Inherited from the process that handed over: the temp checkout is still ours to
# report, even though we did not fetch it.
[ -n "${FACTORY_UPGRADE_CLEANUP:-}" ] && CLEANUP="$FACTORY_UPGRADE_CLEANUP"

# ── Record the version we upgraded to ────────────────────────────────
UPSTREAM_COMMIT="$(git -C "$TEMPLATE" rev-parse --short HEAD 2>/dev/null || echo unknown)"
printf 'ref=%s\ncommit=%s\n' "$FACTORY_REF" "$UPSTREAM_COMMIT" > .factory-version
echo "  recorded: .factory-version (ref=$FACTORY_REF commit=$UPSTREAM_COMMIT)"

# ── Report identity/customizable files (never overwritten) ───────────
echo ""
echo "Yours to reconcile (not touched — the template may have improved these):"
IDENTITY="opencode.json AGENTS.md README.md Makefile .github/CODEOWNERS .github/workflows/ci.yml .opencode/agent .opencode/plugin"
for rel in $IDENTITY; do
  [ -e "$rel" ] || continue
  if [ -e "$TEMPLATE/$rel" ] && ! diff -rq "$TEMPLATE/$rel" "$rel" >/dev/null 2>&1; then
    echo "  review: $rel"
  fi
done
echo "  (the adapters .claude/ and .codex/ regenerate from opencode.json via 'make sync-harnesses')"

# ── Opt-in capabilities this repository has never been offered ───────
# A capability nobody hears about is a capability nobody uses; a capability that
# asks again after you declined is nagware. So: ask once, and let the answer
# live in factory.yaml. The key's PRESENCE is the record — "off" is a decision
# that was made, not an absence — so a repo that has answered is never asked
# again, either way.
#
# When there is no terminal (curl … | sh in CI, say) nothing is recorded: it
# prints how to enable and leaves the question open for an interactive run,
# rather than banking an answer the adopter never gave.
capability_offer() {
  _cap_key="$1" _cap_title="$2" _cap_detail="$3" _cap_enable="$4"
  _cap_yaml_key="$(printf '%s' "$_cap_key" | tr '[:upper:]' '[:lower:]')"
  # The lib is required to record an answer; without it, asking would be asking
  # a question whose reply cannot be kept, and the adopter would be asked again
  # every single upgrade.
  command -v factory_config_has >/dev/null 2>&1 || return 0
  # Answered before — in either direction, and in either file. A repo that
  # answered before Decision 41 recorded it in factory.config; re-asking because
  # the storage moved would be exactly the nagging this avoids.
  factory_config_has "$_cap_yaml_key" && return 0
  [ -f factory.config ] && grep -q "^${_cap_key}=" factory.config && return 0

  echo ""
  echo "New, and off: $_cap_title"
  printf '  %s\n' "$_cap_detail"

  if [ ! -r /dev/tty ] || [ ! -t 1 ]; then
    printf '  enable with: %s   (asked again next time you upgrade interactively)\n' "$_cap_enable"
    return 0
  fi

  printf '  Enable it now? [y/N]: '
  # A failed read is not an answer. Pressing Enter is a decline and is recorded;
  # an EOF or a broken terminal must leave the question open rather than bank a
  # "no" the adopter never gave.
  if ! read -r _cap_answer < /dev/tty; then
    printf '\n  (no answer received — leaving the question open for next time)\n'
    return 0
  fi
  case "$(printf '%s' "$_cap_answer" | tr '[:upper:]' '[:lower:]')" in
    y|yes)
      # The enable command records the key itself.
      sh -c "$_cap_enable" || echo "  (enable failed — run '$_cap_enable' yourself)"
      ;;
    *)
      factory_config_set "$_cap_yaml_key" "off"
      printf '  Left off, and recorded — this will not ask again. Enable later with: %s\n' "$_cap_enable"
      ;;
  esac
}

# ── One-time migration: two config files become one (Decision 41) ────
# Offered, never forced. The old file keeps working either way — it is read as a
# fallback for any key the YAML does not define — so declining costs nothing and
# the question is asked once.
migrate_config_offer() {
  [ -f factory.config ] || return 0
  command -v factory_config_set >/dev/null 2>&1 || return 0
  # Already answered, in either direction.
  factory_config_has config_migrated && return 0

  echo ""
  echo "Two config files, one job: factory.config can move into factory.yaml."
  echo "  Configuration is parsed rather than executed, and one fact stops having"
  echo "  two spellings. Your factory.config keeps working if you decline."

  if [ ! -r /dev/tty ] || [ ! -t 1 ]; then
    printf '  migrate later with: ./factory migrate-config   (asked again next interactive upgrade)\n'
    return 0
  fi

  printf '  Migrate now? [y/N]: '
  if ! read -r _mig_answer < /dev/tty; then
    printf '\n  (no answer received — leaving the question open for next time)\n'
    return 0
  fi
  case "$(printf '%s' "$_mig_answer" | tr '[:upper:]' '[:lower:]')" in
    y|yes)
      if [ -x scripts/factory-migrate-config.sh ]; then
        ./scripts/factory-migrate-config.sh || echo "  (migration failed — factory.config is untouched and still read)"
      else
        echo "  (scripts/factory-migrate-config.sh not found — skipping)"
      fi
      ;;
    *)
      factory_config_set config_migrated "declined"
      printf '  Left as is, and recorded — this will not ask again.\n'
      ;;
  esac
}
migrate_config_offer

if [ -f packs/review-lane/review-pr.yml ]; then
  capability_offer REVIEW_LANE "adversarial PR review" \
    "A model reviews each PR diff and posts an advisory comment — never a required check. Costs tokens on every PR, and needs a repository secret you add." \
    "./factory review-lane enable"
fi

# ── Prove the gates still fire, then hand back for review ────────────
# After the questions, not before: an adopter answers while the upgrade is
# fresh rather than waiting through a full break/fix proof first.
echo ""
if [ -n "$UPGRADE_NESTED" ]; then
  echo "(nested upgrade — skipping the doctor proof to avoid recursion)"
elif [ -x scripts/factory-doctor.sh ]; then
  echo "Running factory doctor..."
  T_DOCTOR=""
  [ -n "$T_START" ] && T_DOCTOR="$(factory_now)"
  ./scripts/factory-doctor.sh || echo "factory upgrade: doctor reported problems — review above before committing"
fi

echo ""
# The total is wall clock from the adopter's command, carried across the hand-off
# — the number they would have measured themselves, had they thought to.
echo "factory upgrade: $copied file(s) updated$(took "${T_START:-}")."
if [ -n "${T_DOCTOR:-}" ]; then
  echo "  of which the break/fix proof took $(factory_duration "$T_DOCTOR")"
fi
if [ -n "$CLEANUP" ]; then
  echo "  A fresh template checkout is at $TEMPLATE — diff your 'review' files against it, then: rm -rf $CLEANUP"
fi
echo "  Review the diff (git status), then commit. Nothing was committed for you."

# ── What still needs a human, printed last so it cannot scroll away ──
# A reminder buried above a doctor run is a reminder nobody acts on. This is
# recomputed from actual state every run, so it disappears once the work is done
# rather than nagging forever.
if [ -x scripts/factory-review-lane.sh ]; then
  PENDING_OUT="$(./scripts/factory-review-lane.sh pending 2>/dev/null || true)"
  if [ -n "$PENDING_OUT" ]; then
    echo ""
    # shellcheck source=lib/color.sh
    [ -f scripts/lib/color.sh ] && . scripts/lib/color.sh
    printf '%s\n' "${C_YELLOW:-}${C_BOLD:-}┌─ Action required ${C_RESET:-}"
    printf '%s\n' "$PENDING_OUT" | while IFS= read -r _pl; do
      printf '%s\n' "${C_YELLOW:-}│${C_RESET:-} $_pl"
    done
    printf '%s\n' "${C_YELLOW:-}└─${C_RESET:-}"
  fi
fi

