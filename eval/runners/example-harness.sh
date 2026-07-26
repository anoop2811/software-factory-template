#!/bin/sh
# eval/runners/example-harness.sh
# A TEMPLATE for a real runner — copy it, fill in the one line that invokes your
# harness, and pass it with `--runner=eval/runners/your-harness.sh`. As shipped it
# exits 1 on purpose, so you don't mistake it for a working runner.
#
# Runner contract:
#   $1 = workdir. It contains task.md (the spec). Write your implementation into
#        this directory. Exit status is ignored — verify.sh scores the result.
WORKDIR="$1"
# shellcheck disable=SC2034  # TASK is for the invocation you fill in below
TASK="$WORKDIR/task.md"
cd "$WORKDIR" || exit 1

# --- Fill in your harness's non-interactive invocation -----------------------
# Hand it the task (cat "$TASK") and have it produce code in "$WORKDIR" without
# prompting, running as the implementer role so the test-edit-denial boundary
# applies. Every harness has a headless mode — check its docs for the exact
# flags (Claude Code has a print/-p mode, Codex an exec mode, opencode a run
# mode). Use the model tier you want to evaluate; run several times (--runs N)
# to get a pass rate rather than a single stochastic sample.
#
# HEADLESS PERMISSIONS — the trap that will bite you first.
# Non-interactive runs do not have a human to approve anything, and "ask"
# permissions do not fail cleanly:
#   - the primary session auto-REJECTS them, so edits and bash commands that
#     need approval are silently blocked and the task fails for the wrong
#     reason (it looks like the model was incapable);
#   - a subagent can HANG instead — its permission request goes onto a queue
#     nothing will service, and it waits forever.
# So grant the operations the task needs as explicit "allow" in your harness
# config before evaluating anything, or you are measuring your permission
# settings rather than the agent. The eval caps every run (--timeout, default
# 300s) so a hang scores 0 instead of wedging the suite — but a run that hits
# the cap is a signal to check permissions first, not a verdict on the model.
echo "example-harness: not wired — fill in your harness invocation (see comments)" >&2
exit 1
# -----------------------------------------------------------------------------
