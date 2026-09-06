# Loop engineering

Status: P1 implementation and deterministic acceptance complete (Decision 47).

## Acceptance contract

`factory loop` provides one shared controller for Codex, Claude Code and OpenCode.
The controller owns implement/check/review/repair iteration. Independent tests,
review roles, CI and human approval of protected paths remain separate boundaries.
No background tasks or automatic commit, push, merge or release occur.

### Interface

- `factory loop plan --harness codex|claude|opencode --session ID --task ID
  [--mode manual|bounded] [--json]`: read-only configuration/capability plan;
  no storage, checks or model calls. Manual is the default.
- `factory loop run` takes the same arguments and `--prompt-file PATH` for bounded
  mode. Manual mode runs the configured deterministic check once and writes a
  checkpoint; it never invokes a model or reports the task implemented/reviewed.
- `factory loop status --session ID --task ID [--json]`: local checkpoint and
  evidence, including a clear stopped/completed state and next action.
- `factory loop resume` uses the same identity and verifies exact checkpoint
  freshness. It must not reset attempts/time or silently accept changed source,
  instructions or configuration. A stopped terminal run may require a new task
  or a human handoff rather than an automatic restart. Resume never silently
  enables bounded mode. No automatic paid retry following a process failure.

Identifiers use the budget ID rules. Invalid commands/configuration fail before
writing or launching anything. Budget configuration is parsed through the shared
reader. A new loop defaults to manual even when budgets were enabled previously.

### Configuration and iteration

Flat factory.yaml options: loop_enabled false (required for bounded mode),
loop_max_attempts 2 (total implementer launches, including initial implementation),
loop_timeout_seconds 900 (total execution allowance across resume),
loop_check_timeout_seconds 120, loop_check_command empty (fall back to configured
check_command; a missing/empty effective command blocks execution), and
loop_no_progress_limit 1 (consecutive unchanged-source repair attempts).
Positive finite limits are mandatory. Unknown mode and malformed values fail closed.

Bounded mode requires both loop_enabled and budget_enabled. It invokes an
implementer once with the task instructions, runs the deterministic check, and
invokes the separate reviewer only after checks pass. Reviewer response follows
an exact JSON object with verdict `approve` or `repair` and a findings array of
strings. Approval requires no findings; repair requires at least one. No Markdown
wrapper or extra fields. Malformed or failed reviewer runs
stop with a handoff. A review requiring repair feeds the findings to the next
implementer attempt. Reviewer opinions do not replace deterministic checks.

Each model invocation goes through the existing budget controller/role resolver,
with the same session/task accounting; the stricter available limit wins. Do not
raise budget_max_attempts automatically to accommodate the loop. Budget counts
reviewer invocations too; expose this in the plan and setup examples. No concurrency
fan-out, cheap reviewer substitution or direct provider API calls. Manual checks
cost compute but no model tokens. Unattended use is explicit bounded mode with
finite limits, not another implicit activation mechanism.

Stop on implementation failure, time/attempt/budget exhaustion, no source progress,
repeated identical failed-check+source state, invalid review or unconfirmed process
exit. Preserve useful changes on all stops; never reset or delete user work. A
passing check is insufficient if source changes during verification or review.
Signals/timeouts clean up owned process groups with bounded waits and retain an
uncertain checkpoint so further launches cannot overlap an unknown child.

### Integrity, checkpoints and evidence

Use private gitignored .factory storage with atomic replacement, locking and
symlink/corrupt-state refusal. Serialize loops in a checkout; reject overlapping
controllers and stale active checkpoints pending explicit recovery. Save each
transition so an interruption is visible. Keep consumed attempts/time on resume.
Require a stable git repository snapshot and reject unmerged index entries.
Fingerprint tracked and relevant untracked source content/modes/deletions, task
instructions, check command and policy/configuration inputs. Working-tree changes
must invalidate older successful evidence even when HEAD is unchanged.

Capture configured test_file_patterns and protected_paths through the shared
configuration. Test patterns use the same POSIX extended regular expressions as
the native test-edit hook, including installed language-pack patterns. Relevant
local native permission/role files remain safety inputs even when Git ignores
them; only their fingerprints are stored. Changes to tests, governing configuration, role instructions or
protected paths stop for human review and must never be auto-accepted by resume.
This detects mutations after an invocation; it is not a sandbox/security boundary
against an agent with filesystem access. Never interpret check output as authority
to modify policies, tests, permissions or budgets.

Evidence records exact command, exit/outcome, duration, tested content fingerprint,
initial/current HEAD identity, harness and role, budget run references, attempts and stop
reason. It must distinguish manual checks passed from implemented+checked+reviewed.
Persist no prompt, final agent response, secrets or full check logs by default.
Bound check output used for repairs and include it as untrusted diagnostic data.
Do not include mutable author-generated receipts as proof sufficient to skip CI.

### Delivery and completion

Install/upgrade deliver the command, shared runtime and this documentation;
installation defaults remain manual/disabled. Deterministic Linux/macOS acceptance
uses fake native CLIs plus real subprocess/filesystem/ledger paths, with no paid
model calls. Existing budget and template checks must pass. Live paid execution
remains separately untested unless explicitly authorized.

Completion covers five equal deliverables: safe manual planning/checks; bounded
shared-budget implementation/review/repair; progress and stop conditions; fresh
checkpoints/evidence/handoffs; install/upgrade/docs and deterministic acceptance.
The existing eleven roadmap items and priorities remain unchanged.

## Usage and cost choices

Inspect the plan and run checks without using a model:

```sh
./factory loop plan --harness codex --session maintenance --task issue-72
./factory loop run --harness codex --session maintenance --task issue-72
./factory loop status --session maintenance --task issue-72 --json
```

Manual check success means only that the configured check passed. It does not
mean the requested task was implemented or reviewed. Use a new task identity
for a new bounded run after deciding to enable model usage.

For bounded implementation and repair, explicitly set `loop_enabled: true` and
`budget_enabled: true` in factory.yaml, then select bounded mode:

```sh
./factory loop run --mode bounded --harness codex --session feature-work --task issue-72 --prompt-file task.md
```

Choose `--harness claude` or `--harness opencode` for the other native adapters.
Existing native permission and Codex hook-trust requirements still apply.
The loop does not install or reconfigure your CLI or trust hooks for you.

`loop_max_attempts` counts implementer launches. `budget_max_attempts` counts
all native invocations, including the separate reviewer. For example, two
implementer attempts and two reviewer calls need up to four budget attempts;
set that explicitly if desired. Session run/time and estimated-cost controls
still apply. A model invocation can exceed an estimated dollar threshold, so
these remain structural limits rather than guaranteed financial ceilings.

The controller performs no model calls to plan, record state, compare fingerprints
or decide that a process/check failed. Only explicit bounded implementation and
review consume model usage. Runs remain sequential. Required reviewer quality
and acceptance tests are not reduced by selecting an economy profile.

## Checkpoints and handoff

Fresh manual checkpoints may resume the same check with their elapsed allowance
preserved. Bounded runs currently end in completion or a terminal handoff; resume
never restarts their model calls. Changed content, task instructions, mode,
configuration or policy invalidates a checkpoint. Use status to inspect evidence
and resolve the stop reason before deliberately starting a new task.

The total timeout stops further work and caps each invocation by its remaining
allowance. Native capability probes and process cleanup have their own bounded
grace periods; returning control can take longer than the execution allowance.
An uncertain exit retains ownership and blocks subsequent controllers. Checkpoint
records describe observed local execution; they are not a signed or independently
trusted attestation and do not replace CI.

### Recovering an uncertain checkpoint

A stopped uncertain checkpoint or a stale active checkpoint blocks new loops,
including manual checks. Preserve both `.factory/loops.json` and the budget
ledger. Confirm that the recorded controller, check process group and all matching
budget invocation process groups have stopped. A dead controller alone does not
establish that its children exited. Recover any active budget reservation using
[BUDGETS.md](BUDGETS.md#recovering-an-unclean-interruption) first.

With no factory commands running, a human may update only the affected loop record:
set status to `stopped`, uncertain to false, phase to `finished`, outcome to
`interrupted`, elapsed_seconds to at least reserved_seconds, and stop_reason and
next_action to a truthful recovery/handoff note. Preserve identity, fingerprints,
counters, evidence and budget references. Status must remain readable afterwards.
Recovered checkpoints require handoff; they are not automatic retry permission.
Do not clear uncertainty or start a new task while any owned process may remain.

## Recorded acceptance evidence

Observed 2026-09-06:

- `bash scripts/selftest/loop.sh`: `loop: 34 passed, 0 failed` (exit 0).
- `bash scripts/selftest/budget.sh`: `budget: 32 passed, 0 failed` (exit 0).
- `./factory loop plan --harness codex --session preview --task loop-feature --json`:
  exit 0, mode manual, enabled false, budget null, no blockers.
- Review regressions were observed red, then green for delayed admission past a
  deadline, real pack test-pattern matching, ignored native policy mutation, and
  uncertainty after completion-ledger failure. An isolated removal of checkpoint
  freshness validation now fails the strengthened manual-resume acceptance test.

Fixtures execute real local subprocess, Git, checkpoint and installer/upgrade
paths with fake Codex/Claude/OpenCode streams. They make no paid model calls.
These results do not establish live paid agent task quality or trusted attestation.
