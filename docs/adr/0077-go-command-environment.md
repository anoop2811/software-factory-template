# ADR-0077: Go budget and loop command environment composition

Status: accepted for private source qualification; installed activation pending.
Decision: 76. Date: 2026-09-26 UTC.

## Scope and measurement

Compose the existing configuration readers/export planner, role routing and Go
budget/loop commands to replace the orchestration in factory-budget.sh,
factory-loop.sh and lib/budget-config.sh. This is source integration, not installed
activation, a new model-routing policy or permission to retire files.

Keep the established 30-package denominator. The remaining G2 integration and
retirement package has three equal milestones: configuration/command composition;
public dispatch compatibility qualification; mixed-runtime activation/retirement
qualification. This slice can close only the first milestone after its evidence
passes. G4 still owns migration machinery, recovery and retention; shared evidence
does not complete those packages automatically.

The immutable wrapper oracle is commit
c8f8d34edbc14df5655fcbf0aab7eea46ced0295, scripts/factory-budget.sh,
scripts/factory-loop.sh, scripts/lib/budget-config.sh, scripts/lib/config.sh and
scripts/lib/roles.sh. Existing controller/oracle qualifications remain in force.

## Private compiled boundary

Under FACTORY_BRIDGE_PROTOCOL=1 add literal requests:

- factory budget configured ARGS
- factory loop configured ARGS

Prepare an effective child environment, then forward ARGS unchanged to the
existing budgetcmd.Run or loopcmd.Run. Do not add a second argument parser or
change the existing controller/command private routes. The preparation happens
before command argument parsing, including help, matching the shell wrapper.
No new public command, environment dump, installed routing or automatic fallback.
Unknown private request names remain refused before configuration access.

## Configuration and model contract

Preserve the wrapper's effective values, rather than assuming every environment
variable is a user override. Derive FACTORY_BUDGET_ROOT from git rev-parse
--show-toplevel, falling back to the caller working directory on failure, including
shell command-substitution newline trimming. Configuration path selection remains
the existing FACTORY_CONFIG/nonempty or Git-root factory.yaml rule, independently
of the budget root. Preserve relative configuration paths and nested checkouts.
Preserve failed Git stdout before each distinct fallback and trim trailing newlines
and shell-discarded NUL bytes at command-substitution boundaries. Resolve from the
supplied caller environment without mutating global environment. Configuration,
Git output and cwd are assumed stable throughout one preparation; snapshotting
them does not promise parity with files or Git output changing between reads.

Populate the eight shared budget settings from flat YAML with the exact existing
defaults. These derived FACTORY_BUDGET_* settings overwrite caller values just as
the shell wrapper does; do not silently introduce a new precedence policy.
Reuse config.ExportPlan and its fixed key allowlist for cost profile, model tiers
and review settings. For those exported keys, caller presence including an empty
value beats YAML, which beats parsed legacy factory.config. Treat values as inert
data; never source/eval configuration. Preserve established reader qualification
limits, including Bash-version-dependent legacy NUL handling.

Budget model selection retains the wrapper's literal scan of --harness/--role and
their equals forms, last occurrence wins, implementer is the role default. The
scan does not consume or reinterpret forwarded arguments; it does not learn
abbreviated options from Cobra. Thus a parser-accepted abbreviated harness may
still yield an empty selected model, as in the wrapper. Exact codex/claude/opencode
harness values select the role tier through existing roles.Tier/Resolve. Empty or
missing cost profile behaves as standard. Preserve explicit empty model overrides
and shell command-substitution trimming on the selected model. No model fallback
or reviewer downgrade is introduced. Always replace FACTORY_BUDGET_MODEL for the
budget command, including with the empty result.

Loop composition adds existing YAML-derived enabled, max_attempts,
timeout_seconds, check_timeout_seconds and no_progress_limit settings. Its check
command falls back to check_command only when loop_check_command resolves empty.
Preserve the eager read of check_command used by the nested shell substitution.
Read test_file_patterns, protected_paths and the resolved configuration path.
Populate all six harness/implementer-reviewer model combinations using the same
role/model resolver. Preserve unrelated incoming environment variables, and do
not mutate the caller map or global process environment. Do not expose inherited
credentials or the complete effective environment through command output.

## Reuse and failure boundaries

Use one small shared concrete environment composer for both callers. Reuse the
existing flat reader, export actions and role functions; no new YAML parser,
shell subprocess for configuration/model selection, Python subprocess, dependency
or toolchain change. Git remains the legacy root-discovery dependency.
The private candidate bounds that probe to five seconds and 1 MiB of combined
stdout/stderr, with process-group cancellation and a one-second pipe-drain wait.
These resource-limit failures refuse preparation rather than selecting a fallback;
ordinary missing-Git/nonzero exits retain the wrapper fallback. Preserve parent
cancellation identity. These are explicit private safety qualifications, not a
claim of unlimited shell-probe parity.
The private candidate requires PATH to be present (an explicit empty PATH remains
valid and searches cwd). An absent PATH is refused before Git/configuration work:
Bash synthesizes a build-dependent default that cannot be reconstructed portably
without invoking Bash. This unresolved compatibility exception blocks public
activation for that input; do not silently substitute a different search path.
Apply export actions to a cloned environment with caller-preserved keys derived
from presence, not nonempty values. Keep command-substitution normalization at
wrapper boundaries rather than altering the shared configuration reader.

Preparation failure returns status 2 and a sanitized diagnostic, with no command
execution, accounting/loop writes or native model probes. This is an explicit
private qualification for configuration I/O failures, not a claim that every
legacy sed/pipefail diagnostic/status is byte-identical. Successful effective
values, command statuses, JSON fields and native role/model choices remain exact;
only already-enumerated command prose/volatile values may be normalized.
Respect parent cancellation during cooperative operations. Do not claim deadline
support for arbitrary regular-file syscalls or change the existing controller
interruption/ownership qualifications. No paid calls during local qualification.

## Independent qualification

Independent outside-in Ginkgo/Gomega tests must fail before implementation.
Compare effective model choices, limits and root/config selection against the
immutable shell wrappers in isolated fixtures. A fake Python boundary may capture
an explicit allowlisted environment subset for wrapper characterization; it must
not become the candidate implementation or expose arbitrary caller variables.
Exercise each harness and role, standard/economy, explicit empty caller values,
YAML/legacy precedence, defaults, literal metacharacters, duplicate and abbreviated
options, equals forms, derived-setting overwrite, nested/non-Git cwd and explicit
config paths. Prove no configuration execution, no state/model effects from plan,
help/refusal and no global environment mutation.

Exercise the actual compiled configured routes, successful plans/reports, manual
checks and a bounded fake-client run so both implementer and reviewer model
selection cross the real command boundary. Re-run existing command/controller
regressions, race checks and quality gates. Installed activation, native live-client
enforcement, recovery copies and cleanup remain separate acceptance work.

## Source gate runtime allowance

The expanded acceptance suite reached Go's implicit ten-minute package timeout
on 2026-09-26 while continuing through existing cases, without an assertion
failure. Give the source gate an explicit GO_RUNTIME_TEST_TIMEOUT make variable,
defaulting to 15m and passed to go test -timeout. Keep every per-operation
deadline, assertion and CI job limit unchanged. This is a suite execution
allowance, not permission to bypass tests or lengthen production timeouts.
