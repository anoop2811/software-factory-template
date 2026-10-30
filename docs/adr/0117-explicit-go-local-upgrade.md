# ADR 0117: upgrade from explicitly selected committed local Go source

Status: accepted implementation contract; independent RED precedes production changes

Date: 2026-10-30 UTC

## Complete function boundary

Deliver `factory upgrade --local [--source DIR]` as one independently reviewable
function after local initialization. The selected committed runtime owns its own
upgrade planner, generated assets, transaction and checks. Reuse the native
packager, local-image custody, installation inference, proposal/consent,
whole-installation backup/rollback and process supervision already in main.
Do not change ordinary initialization/upgrade defaults, source bootstrap,
installation inventories, historical recipes or the stable installer pin.

## Public inputs and consent

The source-built dispatcher may resolve its own checkout when `--source` is
omitted. Installed callers require an explicit source directory. Resolve a real
clean committed checkout, record its full HEAD and build its native installation
image with the provisioned pinned compiler. Dirty, missing, malformed or
conflicting selectors fail before acquisition or project mutation. Neither this
mode nor its output claims publisher authentication or downloads another image.

Use the existing upgrade flags: `--dry-run`, `--json`, `--project-input
KEY=VALUE`, `--confirm DIGEST` and `--quiescent`. JSON is preview-only. Apply needs
both fresh matching complete-proposal consent and declared maintenance
exclusion; dry run cannot carry mutation consent. Help validates selectors but
does not resolve source, invoke the compiler or inspect the target. Ordinary
`--installation` retains its existing explicit archive grammar and owner.

Legacy v0.1.6 and the supported Bash baseline use embedded reviewed references.
A Go predecessor additionally requires `--before-source DIR`, a clean checkout
whose full HEAD matches the installed descriptor. Build it with the descriptor's
exact version, revision and native target, keeping local trust explicit. The
whole-installation planner must independently match every required before role,
including the installed runtime digest; a matching descriptor or source commit
alone grants no ownership. A supplied before checkout for a legacy predecessor
is an error rather than silently ignored input.

Reconstruction is deliberately conditional on exact bytes. A different compiler
or recipe may prevent reconstruction even from the same commit. Preserve the
installation and explain the mismatch; an operator retaining the original
qualified archive can use the existing explicit `upgrade --installation`
before-image route instead. Never relabel a newly built `local-SHA` archive as a
different version after packaging, execute saved predecessor code, fetch a
`local-*` version from GitHub, or guess a historical revision.

## Selected-runtime and admission boundary

Add a separate private versioned local-upgrade transport. Keep the existing
local-init v1 grammar unchanged. The parent builds once, qualifies the exact
runtime and hands off using inherited streams and native owned process groups.
The child verifies source/HEAD, original archive, current executable digest and
private directory/file custody, then consumes a fresh owned snapshot. It never
deletes its borrowed parent input or recompiles the selected after-image.

Installed whole-upgrade selection enters exclusive transaction admission directly,
before retaining any shared installation lease. The private protocol remains
behind physical installed and sourceable-protocol admission; it cannot bypass a
pending selection or become an ambient environment trust hint.

## Shared transaction, trust and recovery

Reuse `InferCandidate`, `ProposeRetaining` and `ApplyRetaining` for local and
authenticated upgrades. Share preview/consent/application plumbing without
changing authenticated acquisition or its public diagnostics. Candidate binding
normalizes private cache paths but must retain each image's local trust mode.
Local transitions do not advance release retention or acquire older historical
images; keep existing recovery sets and use no retention network resolver.

Preview leaves the project unchanged. Apply rechecks custody and the complete
proposal, preserves customized/unknown entries, retains inert rollback backups
under existing ignored controls, and executes only the qualified new runtime's
checks. Normal completion cleans confirmed private acquisition workspaces.

On interruption or uncertain publication retain the selected child's qualified
after/before images and an executable outside the active installation. Emit a
literal replayable read-only recovery command using each image's correct local
or authenticated operands. Recovery uses the exact retained archives without a
compiler, old mutation consent or automatic rollback. Preserve primary errors,
bounded process cleanup, uncertain ownership and existing classified statuses.

## Independent evidence and delivery

An independent spec author first demonstrates failure through the public CLI
against unchanged main. Cover legacy and Go/local-Go preview/apply, exact
selected-source ownership, stale consent and missing exclusion, malformed/help
selectors, predecessor mismatch and required customized assets, installed direct
exclusive admission, rollback and real interrupted-worker recovery with the
compiler unavailable. Keep existing authenticated handoff criteria intact.

Place the heavy public family in a dedicated `local-upgrade` source CI partition
on Linux and macOS. Derive the census from actual Ginkgo discovery; prove six
disjoint partitions, complete union and failure propagation before wiring it.
Retain race, quality and bounded worker stages without increasing deadlines.

Use one PR for this complete function. Independent branches may progress in
parallel; dependency-ordered review, responses, checks and human merge remain
required. Subsequent defaults, inventory retirement, source selftest/adapters,
the final integrated migration rehearsal and release qualification stay separate.

The new signal-driven public proof uses the actual selected worker's SIGTERM.
An unhandleable hard kill cannot print new advice; preserve its existing journal,
backup and private inputs for the separately qualified raw recovery procedure.
