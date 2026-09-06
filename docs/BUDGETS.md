# Budgeting and observability

Status: P0 implementation and deterministic acceptance complete (Decision 46).
Native capability probes and unrun paid checks are distinguished below.

## Boundary

`factory budget` controls only invocations launched through its `run` command.
It does not meter unrelated interactive sessions, child model requests inside a
harness, provider-side retries, or external services. One factory attempt is one
CLI invocation, not one model turn. A CLI completion is not proof that the task
is correct. Existing gates and role separation still apply.

No budget command except explicit `run` may invoke a model. The feature is off
by default. `plan` and `report` run locally without a CLI or credentials.

## Quick start

Preview a task using your existing harness configuration:

```sh
./factory budget plan --harness codex --session maintenance --task issue-71
```

To launch it, set `budget_enabled: true` in factory.yaml, put your instructions
in a local file, and run:

```sh
./factory budget run --harness codex --session maintenance --task issue-71 --prompt-file task.md
./factory budget report --session maintenance
./factory budget report --session maintenance --json
```

Use `--harness claude` or `--harness opencode` for the other supported adapters.
The command uses your existing credentials and may consume paid usage. It does
not install a CLI, configure a provider, or bypass native permissions. For an
explicit second attempt, increase budget_max_attempts and repeat run; the old
attempt stays in the report. Distinct task IDs share the same session limits.

## Interface and configuration

The shared shell entry point reads flat factory.yaml through scripts/lib/config.sh
and passes values as data to a Python 3.8+ standard-library implementation.

Commands (flags use separate values or `--flag=value`):

- `factory budget plan --harness codex|claude|opencode --session ID --task ID
  [--role ROLE] [--json]`: show configuration, current reservations, remaining
  attempts/time, selected model, concurrency, expected model/tool services
  (unknown if inherited), cost-reporting capability and blockers. No writes.
  Exit 0 means the configured limits permit an attempt; native CLI/role preflight
  may still block execution. Exit 2 means blocked or invalid. A blocked JSON plan
  still contains the reasons, including the default disabled state.
- `factory budget run` takes the same identity/role arguments, plus mandatory
  `--prompt-file PATH`. Exactly one invocation; no automatic retry, repair,
  model escalation, or task verification. Run again explicitly for another
  attempt. Print the plan before starting and record the result afterwards.
- `factory budget report [--session ID] [--json]`: list metadata and totals;
  absent history reports no runs without creating files. Unknown cost is never
  converted to zero or included in a supposedly complete total.
- `--max-cost-usd` on plan/run requests a strict financial ceiling. Reject it
  before launch for all three harnesses: none of these adapters establishes an
  authoritative billing ceiling. This is distinct from the estimate threshold.

IDs match `[A-Za-z0-9][A-Za-z0-9_-]{0,63}`. Roles are the five canonical factory
roles. Default role is implementer. Model resolution uses the shared role/tier
resolver; a blank model inherits the harness default and is labelled inherited.

Configuration (all defaults; malformed, zero, negative or nonfinite limits fail
before invocation; there is no silent numeric fallback):

| Key | Default | Meaning |
|---|---|---|
| budget_enabled | false | Explicit permission for factory-managed invocations |
| budget_max_attempts | 1 | Maximum launches for a task within a named session |
| budget_max_session_runs | 5 | Maximum launches across all tasks in that session |
| budget_timeout_seconds | 300 | Wall-clock cap on one invocation |
| budget_session_seconds | 900 | Aggregate charged wall time across a session |
| budget_max_concurrent | 1 | Concurrent factory invocations in this checkout |
| budget_estimated_usd | empty | Optional session threshold using CLI cost estimates |
| budget_action | stop | `stop` or `warn` for the estimate threshold only |

Attempt, time, and concurrency limits always stop further launches, regardless
of budget_action. Reserve a full per-run time allowance (bounded by remaining
session time) atomically before spawning, so parallel launches cannot overspend
the session time allowance. Reconcile reserved time with elapsed time at finish.
Check concurrency against *all* active invocations in the checkout. A changed
configuration applies to the next admission; it does not relax an active run's
timeout. The factory does not schedule additional work or queue silently.

With an estimated USD threshold, `stop` refuses a harness lacking cost reporting
(Codex), refuses admission if previous session cost is unknown or if another
session invocation has not yet reported, and refuses once the known estimate
reaches the threshold. `warn` prints these conditions and permits admission
within the hard structural limits. Check again after completion. A single
invocation can cross this estimate threshold; it is NOT a financial cap.
No prices are hardcoded and no fabricated pre-run cost estimate is displayed.

## Execution and storage

Use argv arrays, never shell evaluation. Prompt text is supplied over stdin where
supported and never written into metadata. Native adapters preserve existing
project configuration and use the selected role/model without broad permission
bypasses. FACTORY_AGENT_ROLE is injected explicitly. Missing CLI, unsupported
flags, or role configuration prevent launch with an actionable error.

Persist metadata under gitignored `.factory/` with private permissions: schema,
run/session/task IDs, harness/role/requested model, start/end time, elapsed and
reserved seconds, attempt number, process outcome/exit code, reported token
fields, estimated USD or null, completeness/provenance and warnings. No prompts,
responses, tool results, secrets, or raw transcripts in the factory ledger.
Provider/harness history storage is outside this metadata promise.
The controller bounds stdout parsing at 16 MiB and stops a run that exceeds it;
stderr is drained without storage. Human-readable runs show the final answer
without retaining it; `--json` emits only the plan and run metadata as JSON lines.

Admission and completion are atomic and safe under concurrent commands. Refuse
symlinked storage, malformed history or unknown schema before launching; do not
reset corrupt state. Interrupted invocations consume an attempt and keep cost
unknown. Termination/timeouts kill the owned process group and cannot leave
children holding the invocation open. If a run record remains active after an
unclean supervisor death, fail closed and provide a documented recovery route;
never silently treat it as a free attempt. Local metadata is not a security
boundary against an agent or user with filesystem access.

### Recovering an unclean interruption

Normally signal/timeout cleanup finishes the run record automatically. If the
supervisor itself was forcibly killed, `plan` names the stale run and refuses
further launches. Preserve the ledger and confirm the recorded owner_pid and
process_pid process group have stopped before recovering the record. Do not
delete history to regain attempts.

With no factory budget commands running, change only that record in
`.factory/budget.json`: status to `completed`, outcome to `interrupted`, ended_at
to the current UTC timestamp, elapsed_seconds to its reserved_seconds,
estimated_usd to null, and complete to false. Preserve its identity, reservation,
and attempt. This charges the full reserved time and leaves spending unknown.
Run `factory budget report --json` to check the repaired history. If a process
is still active or the record cannot be understood, do not recover it as stopped.

## Harness contracts

The core budget logic is identical; adapters construct native CLI arguments and
normalize *metadata only*. Handle errors even when the process exits zero.
Missing, malformed, negative, nonfinite or incomplete usage remains unknown;
do not sum cumulative events twice. Each adapter records its usage source.

- Codex: `exec --json`, native sandbox derived from canonical edit permission
  (read-only for an edit-denied role, otherwise workspace-write); documented
  `turn.completed.usage` tokens, estimated USD unknown. Load the canonical role
  instructions for the root task; Codex has no equivalent root `--agent` flag.
  A root implementer must also receive its shared test-denial hook as an additive
  invocation configuration override. Probe the same hook through native
  app-server hooks/list before launch and require it to be enabled and trusted
  (or managed). An untrusted or modified hook blocks the run and prints the
  native trust/setup instruction. Never use a hook-trust bypass or erase
  existing user/project hooks. The bounded probe makes no model request.
- Claude Code: `-p --output-format json --agent ROLE`, native permission mode
  derived from canonical edit permission (plan for edit-denied roles,
  acceptEdits otherwise);
  final result usage and total_cost_usd are client-reported estimates. Error
  result subtypes are failures, not successful task completion.
- OpenCode: `run --format json --agent ROLE`; step_finish metadata is incremental
  and must be deduplicated by part identity. Cost is a client estimate, not a
  bill. Do not attach to a remote/shared session whose lifetime is not owned.
  OpenCode rejects a subagent-only role at the CLI root and falls back to its
  default agent. The adapter must use an invocation-local configuration overlay
  making the selected canonical role available at the root (`mode: all`), while
  preserving its permissions and any pre-existing configuration overlay. This
  does not rewrite generated files or select a different agent silently.

Sources fetched 2026-09-06:

- https://developers.openai.com/codex/noninteractive
- https://code.claude.com/docs/en/headless
- https://code.claude.com/docs/en/agent-sdk/cost-tracking
- https://code.claude.com/docs/en/sub-agents
- https://developers.openai.com/codex/hooks/
- https://developers.openai.com/codex/app-server/
- https://opencode.ai/docs/cli/
- https://raw.githubusercontent.com/anomalyco/opencode/dev/packages/opencode/src/cli/cmd/run.ts

## Acceptance and completion

P0 completion uses five equally weighted deliverables: (1) local plan and valid
configuration; (2) atomic enforced run limits; (3) private truthful ledger/report;
(4) all three adapters and their acceptance fixtures; (5) install/upgrade/CI and
user documentation. Existing foundations do not count toward completion of the
other roadmap additions. Live paid tests remain explicit opt-in and are reported
separately from deterministic fixtures; absence is never labelled a live pass.

Tests must prove no invocation when disabled, over budget, corrupt, missing
requirements or strict USD requested; exact argv/role/model for all harnesses;
normal/error/malformed/duplicate usage; partial unknown totals; stopped timeout
children; signal cleanup; concurrent admission and session-time reservations;
metadata privacy; and installation plus upgrade delivery. Paid calls are absent
from normal CI. Existing factory checks remain green.

## Compatibility evidence

| Harness | Adapter fixtures | Installed CLI capability check | Live paid execution |
|---|---|---|---|
| Codex | Acceptance results recorded below | 0.153.4: flags and reviewer preflight pass; implementer correctly refuses the untrusted role hook in this environment | Not run |
| Claude Code | Acceptance results recorded below | 2.1.220: flags and implementer preflight pass with ASDF_NODEJS_VERSION=20.11.0 | Not run |
| OpenCode | Acceptance results recorded below | 1.18.23: flags and implementer preflight pass | Not run |

These are installed versions observed with each CLI's `--version` on 2026-09-06,
not a claim that other versions support the same APIs. No CLI is installed or
upgraded by the budget feature. Codex implementers must review and trust the
exact hook named in the preflight instruction through native `/hooks`; the
factory never grants that trust automatically.

## Recorded acceptance evidence

Observed 2026-09-06:

- `bash scripts/selftest/budget.sh`: `budget: 28 passed, 0 failed` (exit 0).
- `bash scripts/selftest/run.sh`: `selftest: 217 passed, 0 failed, 0 skipped` (exit 0).
- `make eval`: `PASS harness=opencode`, `PASS harness=claude`, `PASS harness=codex`.
- Review regressions were observed red, then green: adopted CI referred to a
  missing budget fixture; Claude reviewer selected acceptEdits instead of plan.
  Isolated removal of required native CLI flags is now rejected by acceptance.

The suite uses fake native executables and real controller processes, including
timeouts, signals, parallel admission, and installer/upgrade copying. It proves
those mechanisms without consuming model usage. Paid end-to-end agent behavior
is NOT verified by these fixtures or by CLI help/capability probes.
