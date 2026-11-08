# ADR 0142: installed-safe full-check Git hook setup

Status: merged in PR #173; final-head hosted checks passed.
Decision: 157. Starts R4 after merged PR #172 completes R3.

## Context

The initializer recommends copying `scripts/pre-push-check.sh` into Git's hooks
directory. Its installed generated form resolves its owner relative to `scripts/`;
the copied file instead selects `.git/factory`. Source copied-hook support does
not repair the frozen installed generator. R4 must close this setup gap before
stable activation while preserving existing installations and user hooks.

The tracked `.githooks/pre-push` remains a direct-main guard. Selecting that
directory is not evidence that the full eight-stage native check runs. Native
initialization does not itself install an active full-check Git hook.

## Installed-safe opt-in setup

Document a user-owned, executable `pre-push` hook with this body:

```sh
#!/bin/sh
exec ./factory check "$@"
```

Use Git's existing pre-push contract: in a non-bare repository Git enters the
worktree root and supplies remote operands plus the ref-update stream on stdin.
The hook preserves those operands, stdin and the native command's exit status.
It uses the worktree's installed launcher, never a PATH factory or saved script.
The installed runtime requires no Go compiler or Python for factory behavior;
an application's configured checks may require its own language tools.

This is an explicit manual setup procedure, not a new automatic hook installer.
From the worktree root, inspect all `core.hooksPath` values and their origins,
then obtain the effective path with `git rev-parse --git-path hooks/pre-push`.
Do not assume `.git` is a directory: linked worktrees share Git-managed paths.
Review the destination before creating an absent hook and making it executable.
Existing files, symlinks, modes and hook-path settings are preserved. A shared or
external hooks directory requires its owner's integration decision, not a silent
project-local write. Do not automatically chain arbitrary existing hooks: order
and preservation/replay of the ref-update stdin stream matter.

Replace initializer copying advice with a link to the qualified setup procedure.
Document its scope and the separate tracked direct-main-only option. Correct
claims that init automatically enables local commit/push enforcement. Pack CI
continues invoking its configured checks independently of local Git hooks.

## Doctor reporting and compatibility

Keep the shared `githooks.Status` protocol and its existing classifications.
Scope doctor's human-readable result to selection of the tracked `.githooks`
path. A default or custom hook may be active; do not label every such selection
as missing enforcement or advise replacing its `core.hooksPath`. Show the
reported effective path without claiming to have validated arbitrary hook
contents. Selecting the tracked executable proves path selection, not that a
customized body runs full checks. Doctor remains read-only.

Leave installed generators, role inventories and frozen recipe inputs unchanged.
No new public command or dependency is needed. Historical reconstruction,
upgrade and rollback continue to use their exact predecessor recipes. The new
hook is user-owned Git setup rather than an installation-owned generated asset.

## Independent qualification

Capture the old advertised copy's failure through an actual installed candidate
and isolated local Git push. Before changing guidance/reporting, independently
write outside-in Ginkgo/Gomega criteria for the corrected procedure and advice.
Execute the documented hook body through Git, not a substitute Git executable.
Prove a healthy topic push, configured-check failure blocking the remote update,
and real direct-main refusal before expensive checks. Preserve remote operands
and the ref-update stream. Include default and repository-local custom hook paths
and a linked worktree, without duplicating the full native check matrix.

Check that init/doctor preserve existing hooks and hook-path configuration, and
that doctor's messages do not instruct replacement of custom/default hooks.
Reuse the actual installed-candidate fixtures and existing frozen-recipe and
hook-preservation criteria. Keep compiled tools and hook execution in complete
private checkouts, preserving the operator's ignored binary and unrelated edits.
Record RED/GREEN, independent review and exact-head hosted results separately.

This closes the installed-hook setup obligation only. Fresh-project/adopter CI
delivery, final migration/backup/rollback/recovery rehearsal, candidate platform
qualification, pilot and stable publication remain the separate R4-R7 gates.

## Authoritative contract

Read on 2026-11-07: [Git hooks](https://git-scm.com/docs/githooks) documents hook
cwd, pre-push arguments/input and failure propagation;
[Git rev-parse](https://git-scm.com/docs/git-rev-parse) documents Git path
resolution. No Git version pin is changed by this decision.

## Qualification boundary refinement

The first broad run exposed seven frozen doctor report comparisons that still
expect the intentionally replaced hook warning. Preserve the historical oracle
and every unrelated report byte and exit status. Translate only that exact old
warning block to the explicitly approved current diagnostic; do not remove the
hook section or normalize arbitrary differences. The independent hook-state
criteria separately exercise the new advice and preservation behavior.

A linked worktree needs its own complete local installation. Git checks out
tracked files, not ignored runtime custody, and does not preserve private file
modes. Qualify the shared hook path by creating a real linked worktree from a
legacy project and applying the installation transaction in that worktree.
Do not manufacture a partial installed layout from force-tracked private files.
Poison the primary worktree's launcher afterward to distinguish the selected
runtime. This qualifies hook invocation in a prepared worktree, not automatic
provisioning of every worktree by Git.

## Local evidence

The independent advice run reported 1 passed and 5 failed before implementation.
The separate actual-installed copy diagnostic observed Git status 1, a missing
`.git/factory`, and no remote update. The first broader run reported 58 passed
and 8 failed: seven intentional warning changes needed the bounded historical
comparison above; the linked fixture lacked complete private installation state.
Those failed receipts remain separate from later qualification.

After correction, the private race-enabled acceptance invocation selected
`Whole installation upgrade full-check hook setup`, the ordinary-assets init
criterion, and doctor's frozen-report/hook-state criteria. Its JSON report records
15 passed criteria and zero failures in 155.332 seconds. The command printed
`ok github.com/anoop2811/software-factory-template/acceptance 156.774s`.
This includes real Git pushes through actual installed runtimes, default/custom
hook paths, failed application checks, direct-main refusal and a separately
installed linked worktree. A separate partition criterion reported one pass in
51.209 seconds: 3323 total, 101 installation, 115 source and 2997 base criteria;
other partition counts are unchanged. The artifacts are under
`.factory/qualification/r4-installed-check/`.

Independent correctness/security and test reviews, followed by refutation of the
fixture corrections, retained no actionable findings. These are local results;
the enclosing installation gate and exact-head hosted results are recorded
separately before this PR can be considered ready for merge. R4-R7 remain open.

The final-head review identified a fixture-isolation omission: the remote ref
probe used `imageProcess` without the push fixture's Git configuration overrides.
That helper removes inherited `GIT_*` variables but otherwise still permits
system/global configuration lookup. Pass the existing isolated environment to
the remote probe too, so a host policy cannot change the verification result.
This is a test-fixture correction, not a change to user Git configuration or
factory runtime behavior.

## Retained recovery control refinement

The enclosing hosted installation suite exposed an old recovery criterion that
assumed `.githooks/pre-push` still matched the immutable Bash baseline. This
decision corrects that file's comments, so replacement is legitimate. Use the
required literal-copy asset `templates/metrics.html` for the retained-original
control: its bytes and mode match both the pinned baseline and current target.
Assert equal before/after content, size and mode in the original proposal, then
`retain` for that asset in the fresh forward-recovery proposal. The initial
proposal still calls a recognized copy `replace`; recovery decides retention
from the current image and its physical custody.
Keep the untouched historical input and every physical identity, mode, timestamp,
byte, completed-replay and foreign-inode refusal assertion. Do not seed current
target bytes into the historical hook or relax recovery custody requirements.
Qualify both native interruption directions from a complete checkout at the
current commit; an older committed image with source overlays misses this asset
change and cannot establish the relevant result.

The corrected forward and reverse native SIGKILL criteria passed under race in
a complete private checkout at `8966a59` with the test overlay: two passed,
zero failed in 212.378 seconds; `go test` printed `ok` in 213.617 seconds.
Independent review confirmed the retained custody and refusal assertions.
This focused result does not replace final-head hosted installation qualification.

## Merge evidence

PR #173 merged at `2026-11-07T23:09:10Z` as
`d05d99d67d496caaf5f3c2310eefd1b503315cdf`. Final head `0c813fb` completed all
32 [hosted checks](https://github.com/anoop2811/software-factory-template/pull/173/checks).
Adversarial review completed in 85 seconds with
[responses to all findings](https://github.com/anoop2811/software-factory-template/pull/173#issuecomment-6712558607);
Copilot had insufficient quota. One macOS local-upgrade helper timed
out on its first attempt after reporting completed installation. Its unchanged
failed-job rerun passed all 11 criteria; no persistent fix or root cause is
claimed. This closes the hook-setup obligation, not the remaining R4-R7 gates.
