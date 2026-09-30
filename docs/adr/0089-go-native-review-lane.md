# ADR-0089: Native Go advisory review-lane management

Status: accepted; native source implementation; installed cutover separate.

Decision: 88

## Context

Metrics is merged in PR #115. The next requested source conversion replaces
`scripts/factory-review-lane.sh`; frozen ordinary behavior comes from commit
`315e4f90250260ca2933ade74063cdbd532cc5ad`. This manages local configuration and
workflow files. It does not change the hosted review client, model selection,
provider limits or review policy.

## Command contract

Native `review-lane` supports `status`, `enable [SECRET]`, `disable`, `pending`
and `secret-name`. No operand or an empty first operand means status. Preserve
ignored trailing operands and unknown-command status 2. Read-only commands do
not create project files. Preserve ordinary piped output and terminal emphasis
from the existing color helper without sourcing it.

Discover the caller's Git root with the shared bounded process supervisor;
ordinary unavailable Git falls back to cwd. Explicit relative config overrides
resolve from that root, matching the old script's change of directory. The
invoked factory directory selects the workflow template. Use bounded snapshots
and the shared flat configuration export plan, including caller environment
preservation and legacy sibling fallback. Missing optional config is allowed
for read-only commands. Keep provider-specific fallback secret/model strings
from the frozen implementation; this conversion does not assert they are the
latest model versions.

Status reports configured lane/model/secret and whether a regular workflow is
present, including a read-only symlink to a regular file as in the old status.
Secret-name prints the effective name. Pending is silent while the lane is off.
While on, invoke literal `gh secret list` at most once, with no secret values
requested or logged. Ordinary absence/failure means unknown; an absent matching
first field means missing. Match names case-insensitively, as GitHub does.
Missing/unknown produces the existing action instructions; a matching name is
silent. Process timeout, capture overflow, cancellation or uncertain cleanup
must fail rather than be described as an ordinary missing secret.

For enable, use a nonempty second operand, then configured secret, then provider
default. Prompt for a model only when the effective model is empty, stdout is a
terminal and a controlling terminal can be opened. Empty answer/EOF keeps the
frontier default. Reuse cancellable terminal input, bound the answer at 64 KiB
and never consume piped stdin in lieu of the controlling terminal. Retain the
cost/advisory disclosure; do not call a model or create/upload secrets.

## Validation and mutation contract

Before enable mutations, require the existing writable configuration and readable
template. A secret name must contain only ASCII letters, digits or underscores,
must not start with a digit and must not start with GITHUB_ (case-insensitive).
Reject invalid names with nonzero status before writing. This deliberately
replaces unsafe sed substitution and invalid workflow expressions. Preserve
valid supplied spelling. Reject NUL, CR, LF or double quotes in the model value
before writing it through the existing flat config setter. All input files and
generated workflow output are bounded at 16 MiB.

GitHub naming source, fetched 2026-09-30:
https://docs.github.com/en/actions/reference/security/secrets

Ownership requires the exact first line:
`# Managed by: factory review-lane. Remove with: ./factory review-lane disable`
A substring elsewhere is insufficient. Mutation refuses symlinks, hardlinks,
special files and unsafe parent directories; an unmanaged file is preserved.
Pin the project and workflow parents and reject observed replacement. Operate
in a trusted, quiescent directory; do not claim concurrent-writer CAS safety.
Reuse proven bounded publication machinery where its contract matches.

Enable stages the complete workflow beside its destination with exclusive
creation. Prefix the ownership line, substitute the secret placeholder as
literal data and require that the template contains the placeholder. Finish
and close staging before config changes. Record secret and model first, then
review_lane on, and only then atomically publish the workflow with mode 0644.
If publication fails after turning the lane on, try to record off and report
whether rollback succeeded. Failed staging/config writes must never publish a
new privileged workflow. Partial inert config settings may remain, as in the
existing command; this is not a two-file atomic transaction. Recheck ownership
before replacement, clean only identified owned staging files and retain the
previous workflow until publication succeeds. Output failure after publication
returns nonzero without undoing the complete published artifact.

Disable removes only the exact managed ordinary file, rechecking its identity;
an absent workflow is allowed. Record off after removal. For an unmanaged file,
preserve it, attempt to record off, explain manual removal and return nonzero.
Reject unsafe directories/special files without following them. Do not create
backups of generated workflows or accumulate persistent temporary artifacts.

## Reuse and integration

Expose a native service taking explicit project and template roots so init and
doctor can call it without process-global chdir or shell scripts. Init retains
its advisory warning behavior around enable and pending; doctor retains its
armed/inert/missing-secret diagnostics. Replace their converted review-lane
script calls and qualify them without a usable legacy lane script. Extract
terminal or publication helpers only where contracts match, preserving their
existing acceptance tests. No new dependencies are required.

## Qualification and rollout

Independent Ginkgo/Gomega tests must observe core RED before production. Compare
ordinary status, enable, disable, pending and secret-name against the frozen
script using isolated repositories, explicit fake gh and controlled config.
Qualify invalid secret/model input, missing config/template, exact ownership,
symlink/hardlink/special paths, atomic publication failures, cancellation,
checked output, prompt behavior and init/doctor integration. Use the established
review diamond and complete exact-head Linux/macOS CI before merge.

Keep the legacy script for existing installations. Native source completion
does not activate the installed Go runtime, retire scripts or complete recovery
creation/restore/retention. Merge this command before starting config migration.

Rollback after lane-on uses a cancellation-independent context with a five-second
deadline, including cancellation detected before publication. Preserve an error
if rollback fails; do not claim lane-off or successful publication without evidence.
Git discovery and secret listing each have a ten-second command deadline plus
the shared bounded child-cleanup allowance.

Disable must also preflight configuration writability before removing a managed
workflow, so a missing/read-only config cannot cause a partial disable. Explicit
configuration paths retain the shared setter's lexical parent-path behavior
(including intentionally selected symlink parents); workflow mutation paths
have the stronger no-symlink parent policy above. Configuration publication
still requires the setter's trusted, quiescent parent-directory contract.

Init already collects its review-model choice. Its native lane call must retain
the old captured-child presentation boundary: no second prompt or terminal color
from the lane service. Preserve the actual output descriptor's cancellation
behavior rather than hiding it behind an arbitrary writer wrapper.
