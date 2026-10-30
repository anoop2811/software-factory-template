# ADR 0114: deliver the Go cutover through complete function boundaries

Status: accepted; first recovery-guidance slice locally qualified, hosted review pending

Date: 2026-10-29 UTC

## Delivery decision

The operator requested splitting PR #141 into separately consumable changes.
Its preserved source `07e323a76bbaeb34b2ef12321d958b1539fa528d` spans 134 files,
7,991 added lines and 12,008 deleted lines. Retain that committed work as the
reference implementation; extract each replacement from current main without
making unrelated unfinished changes a prerequisite.

Each PR owns one complete user or maintainer function: its production path,
independent outside-in tests, relevant compatibility behavior and documentation.
Tests, safety checks and cleanup required by that function belong in the same
PR. Complete review, findings responses and checks before human merge, then
start the next slice. Keep the source branch visible in the primary checkout.

Smaller reviews reduce context and request count. They do not establish that
the observed HTTP 429 responses are resolved; provider behavior remains a
separate observation. Do not retry the monolithic paid review while splitting.

## Dependency order

1. Replayable structured whole-installation recovery instructions.
2. Bounded compiler execution through the existing bundle packager API and CLI.
3. Exact historical RC3 recipe reconstruction before changing generators or
   installation inventories; unknown recipes continue to refuse.
4. Explicit local initialization and local upgrade as separate functions,
   reusing qualified staging and the existing whole-installation transaction.
5. Ordinary packaged-Go initialization defaults, preserving selected-version
   reuse, explicit selector precedence and authenticated acquisition.
6. Ordinary packaged-Go upgrade defaults, including downgrade refusal and
   exclusive admission before opening a retained shared installation lease.
7. Minimal qualified initializer inventory, followed by declared cache cleanup
   with historical reconstruction, ownership checks, inert backups and rollback.
8. Source bootstrap activation with the direct init/upgrade adapters and a
   non-vacuous inventory hook. Native source selftest routing must be ready
   before changing its entrypoint.
9. Remaining command and helper entrypoints by function after native dispatch
   exists. Retire shared Python controllers only after their final current
   consumer and proof runner migrate; keep historical proofs explicit.
10. Final audit, one replacement candidate, published native qualification and
    operator acceptance before stable activation.

These are dependency boundaries, not a promise to combine every line of a
numbered item into one PR. Keep separate independently consumable functions
separate. Move a heavy ordered qualification family into its own checked CI
partition when that family is introduced; preserve exhaustive identity coverage.

## Intermediate-state invariants

Never introduce an adapter that calls the same command through a dispatcher
which still calls that adapter. Source init must not copy a developer-only
compiler-dependent launcher into an ordinary installation. Keep direct init and
upgrade entrypoints compatible when the source bootstrap changes.

Do not retire initializer cache inputs while the qualified plan still requires
them. Retire only unchanged declared assets through existing backup/recovery
rules. Preserve the two consumed assets and every edited, unknown or unsafe
entry. Publish no intermediate recipe RCs merely because a slice merged: each
additional published recipe creates a predecessor compatibility obligation.

The stable installer stays pinned to v0.1.6. Existing release gates, supported
operating systems, the approved pilot and final operator acceptance remain
unchanged. A smaller PR or a merge does not itself close a release gate.

## First slice: structured recovery arguments

Repair the executable `RecoveryArguments` vector emitted for whole-installation
recovery. The descriptive `RecoveryCommand` string remains descriptive; do not
claim it is a complete shell replay command.

Reuse one argument builder for preparation and applying/pending transition
proposals. Preserve source identity, local-versus-authenticated trust operands,
before-image operands for Go predecessors and deterministically sorted literal
project inputs. Emit an explicit read-only dry run; never carry old mutation
consent, quiescent declarations, credentials or private staging output paths.
The normative recovery contract is
`docs/adr/0098-whole-installation-upgrade-and-rollback.md:340`.

An independent author first replays the actual emitted vectors from the existing
forward/reverse interrupted-installation scenarios against unchanged main and
observes failure. After the production repair, require successful literal argv
replay, an unchanged project, the same proposal digest and fresh consent for any
later mutation. Add focused local/authenticated and Go-predecessor argument
coverage. Name preparation-only evidence separately from applying/pending
recovery; shared code alone is not proof of both public paths.

The first slice changes no default command routing, compiler supervision,
installation roles, retirement rules, public release pin or published assets.

## First-slice local evidence

Observed 2026-10-30 UTC with evaluator and CLI race instrumentation: the two
existing real-interruption scenarios first failed while replaying the incomplete
vectors (`0 Passed | 2 Failed` in 165.429 seconds), then passed after the shared
builder repair (`2 Passed | 0 Failed` in 254.507 seconds). The selected cases are
`Whole installation upgrade semantic consumer retains one pending installation
after native interruption and recovers with fresh consent`, forward and reverse
after actual SIGKILL. Replay preserves the project and the same transition
proposal digest before a separate fresh-consent mutation.

All four `Installation recovery argument custody` cases passed. A private Go
overlay restoring the incomplete vector failed all four; canonical restoration
passed all four again. The existing acceptance registry remains 3,079 entries.
Public preparation-only structured replay remains unexecuted; the shared
builder coverage does not claim that additional journey.

Broader local checks ran:

```sh
go test -race -count=1 -timeout=5m ./internal/installation -ginkgo.no-color -ginkgo.fail-on-empty
golangci-lint run --config packs/go/.golangci.yml ./...
go vet ./...
gosec -quiet ./internal/installation/...
```

Observed respectively: `ok .../internal/installation 2.668s`, `0 issues.`,
exit zero with no output, and exit zero with no output. Independent correctness,
security and coverage reviews recorded no actionable finding. These local
results do not establish hosted CI or model-review completion.
