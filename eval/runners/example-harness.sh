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
TASK="$WORKDIR/task.md"
cd "$WORKDIR" || exit 1

# --- Fill in your harness's non-interactive invocation -----------------------
# Hand it the task (cat "$TASK") and have it produce code in "$WORKDIR" without
# prompting, running as the implementer role so the test-edit-denial boundary
# applies. Every harness has a headless mode — check its docs for the exact
# flags (Claude Code has a print/-p mode, Codex an exec mode, opencode a run
# mode). Use the model tier you want to evaluate; run several times (--runs N)
# to get a pass rate rather than a single stochastic sample.
echo "example-harness: not wired — fill in your harness invocation (see comments)" >&2
exit 1
# -----------------------------------------------------------------------------
