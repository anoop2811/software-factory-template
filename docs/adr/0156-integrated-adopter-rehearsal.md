# Integrated fresh and migrated adopter rehearsal

Status: merged in PR #183; local and hosted source qualification passed.
Decision: 172. Continues R4 after merged PR #182.

## Purpose and release boundary

PR #182 merged on 2026-11-15 UTC as
8772e919a66d625085a1f69320f7dd0afc2e59eb. Exercise the connected installed
experience now that ordinary CI and opt-in review delivery are implemented.
Keep the fixed release gates: R4 integrated rehearsal, R5 published RC4 native
qualification, R6 representative adopter pilot, and R7 stable activation.
The completed 32-path Go source inventory is separate from these release gates.

This rehearsal uses actual compiled and packaged current factory code, genuine
immutable v0.1.6 initialization, public commands, committed project files and
clean Git clones. Release transport and attestation responses are controlled
fixtures, not evidence of a published or cryptographically qualified release.
Provider responses are local deterministic fixtures; no paid model is called.
Existing detailed recovery, retention and refusal criteria remain required.
Do not describe this source rehearsal as the operator's current-candidate pilot.

## Connected journeys

1. Fresh initialization must deliver a project whose committed ordinary workflow
   prepares missing private runtime custody and runs its real factory checks and
   all three harness evaluations. Preserve the existing untouched-initializer
   journey and its application failure control. Explicitly enabled review must
   also execute the generated selector, pinned real installer, authenticated
   native client acquisition and public review command from a separate clean
   checkout. No project launcher is executed during review acquisition.
2. A genuine supported v0.1.6 project must preview and apply the current packaged
   selection through the normal installer upgrade boundary. Preserve application
   files, user configuration, native permissions, optional review choices and
   consumed history. Commit the resulting public assets without repair, clone
   them without private runtime/backup state, then execute the emitted ordinary
   CI and enabled review paths. An unchanged application failure must fail the
   actual check. A generated job that merely looks correct is insufficient.
3. Observe the documented opt-in full-check hook on a disposable topic push,
   including a failing application check that prevents the remote update. Do not
   overwrite an existing real user's hook or select a hook automatically.
4. Inspect the migration proposal and ignored backup against the resulting live
   tree: selected owned obsolete paths retire, required adapters remain and point
   to Go, saved originals are inert, unknown application data remains. Reuse the
   independently exercised public rollback/interruption/retention criteria;
   replay the relevant complete families on the candidate used for this rehearsal.

## Evidence and implementation discipline

Use the existing Ginkgo/Gomega fixtures, process supervision and private image
production. Keep test writing independent of production fixes. Share an image
within an ordered journey where safe; each project/checkout and transport keeps
its own custody and cleanup. Preserve existing command deadlines and process
ownership checks. Avoid adding jobs or parallelism to hide qualification cost.
Update exact discovery identities if new criteria are introduced, preserving the
complete disjoint union and the negative empty/failure controls.

Keep Go and Python poisoned in installed factory subprocess environments; retain
only explicit application tool fixtures when needed. Use the exact generated
workflow scripts, with only GitHub expressions translated into fixture values.
Transport adapters must validate requested immutable installer URL and release
identity, then deliver the corresponding actual bytes. Do not replace an
installer or native acquisition operation with a success stub in the integrated
journey. Review publication is captured locally; no external comment is posted
by an acceptance test. Record authentication calls and prove provider credentials
are absent during acquisition. Failure controls must reach their intended phase.

Record source/packaged revision, target, selected release, commands, statuses,
diagnostics and timings. Keep historical receipts unchanged and clearly label
fixtures. Run local relevant groups and exact-head hosted checks, then apply the
review-diamond workflow and respond to every hosted review finding. R4 can close
only on observed passing integration evidence; any remaining mismatch stays
explicit. No candidate tag, release default or version pin changes in this task.

## Compatibility finding to qualify

Source inspection identified an expected first-check migration failure: legacy
jq-generated Claude/MCP and OpenCode JSON uses insertion ordering, while native
sync marshals ordinary Go maps in sorted order. The preserved legacy Makefile
checks drift after synchronization. The integrated journey must retain these
original files through commit/clone so that any mismatch is observed honestly.
The second integrated run reproduced this on unmodified production source:
`Ran 2 of 3497 Specs in 317.457 seconds`, `1 Passed | 1 Failed`.
Fresh initialization, review, ordinary CI and both hook controls passed. The
v0.1.6 preview/apply, real review bootstrap/review and CI preparation passed;
the healthy migrated check returned 1 with `DRIFT: Claude config files do not
match sync output`. The captured diff showed equivalent Claude/MCP and OpenCode
JSON reordered by native synchronization. This establishes the runtime RED;
it does not yet establish a correction or complete rollback qualification.

Preserve existing JSON bytes only when strict comparison proves
that the generated document has the same effective content. Treat object field
order as incidental only where the harness format permits it; retain array
order, exact numeric values and ordering-sensitive OpenCode permission rules.
Do not use a blanket unordered-object comparison for OpenCode's unowned nested
configuration. Meaningful generated changes still publish and remain visible to
the drift check. Invalid canonical inputs still refuse. Malformed generated
destinations must not qualify as unchanged; keep their existing repair behavior.
Reuse confined destination inspection and publication rather than adding an
upgrade-specific synchronization or bypassing custody checks.

## Local regression evidence

Observed on 2026-11-15 with Go 1.27.4 on darwin/arm64, GOTOOLCHAIN=local,
GOENV=off and GOMAXPROCS=3. Long local runs used a temporary `caffeinate -i`
assertion; no runtime limit or machine power setting changed. The evaluator
used race instrumentation. Publisher authentication and provider transport
remained local fixtures as defined above.

The first rehearsal run stopped both journeys at an overly strict transport
fixture, which required the review download destination to exist already.
That result is not a product failure. Correcting the fixture to admit a new
regular output leaf, using the existing cooperative process supervisor and
keeping historical rollback tools separate allowed the real migration drift
failure above to be observed. The rollback-success scenario preserves unknown
operator history; existing state-compatibility controls retain the deliberate
refusal to roll modern budget/loop state back to a predecessor that cannot read it.

Before production changes, executed:

```sh
go test -race -v -count=1 -timeout=5m ./acceptance \
  -ginkgo.focus='Native orchestration sync compatible JSON' \
  -ginkgo.no-color -ginkgo.fail-on-empty \
  -ginkgo.json-report=../.factory/ci-mode-evidence/integrated-adopter/sync-red.json
```

Output: `Ran 4 of 3501 Specs in 4.298 seconds`,
`FAIL! -- 3 Passed | 1 Failed`. The failure was preservation of equivalent
Claude settings bytes; the meaningful array, scalar and malformed-output repair
controls already passed.

After the initial preservation correction, executed:

```sh
go test -race -v -count=1 -timeout=10m ./acceptance \
  -ginkgo.focus='Native orchestration sync' \
  -ginkgo.no-color -ginkgo.fail-on-empty \
  -ginkgo.json-report=../.factory/ci-mode-evidence/integrated-adopter/sync-green.json
```

Output: `Ran 41 of 3501 Specs in 45.910 seconds`,
`SUCCESS! -- 41 Passed | 0 Failed`. This includes model routing/inheritance,
ordered policy, invalid source, foreign-path preservation and publication checks.
Full connected qualification and the separately identified non-JSON cancellation
path correction remain pending at this checkpoint; this receipt is not a final
release-readiness claim.

The complete initialized-CI gate subsequently passed all eight criteria in
940.527 seconds, including both connected fresh/migrated journeys, emitted mock
evaluation, real topic-push controls and exact public rollback. Its build/vet,
lint and security stages also exited 0. The detailed command, source identity,
scope and results are recorded in
[the rehearsal report](../migration/INTEGRATED_ADOPTER_REHEARSAL.md).

Independent review found one cancellation-path regression in the initial patch:
non-JSON `CLAUDE.md` inspection can own a temporary leaf descriptor before the
caller registers cleanup. Returning a new context error in that window would
leak the descriptor. Preserve the old nil return for non-JSON destinations;
the subsequent `Digest` and custody checks still handle cancellation. This
one-line correction does not change the three JSON comparison paths or add a
new cancellation mechanism. The final focused run subsequently passed; hosted
qualification remains pending.

After that correction, executed:

```sh
go test -race -v -count=1 -timeout=25m ./acceptance \
  -ginkgo.focus='(Native orchestration sync|Go source qualification (scheduling|worker execution|workflow partition))' \
  -ginkgo.no-color -ginkgo.fail-on-empty \
  -ginkgo.json-report=../.factory/ci-mode-evidence/integrated-adopter/final-targeted.json
```

Output: `Ran 53 of 3501 Specs in 373.710 seconds`,
`SUCCESS! -- 53 Passed | 0 Failed`. Besides all native synchronization criteria,
this run covers exact native discovery across the complete disjoint 3,501-test
union, the worker execution controls and workflow partition checks.
