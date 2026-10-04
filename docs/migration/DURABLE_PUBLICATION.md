# Durable live publication completion

Status: source contract; qualification is recorded in the pull request. The
installed factory does not invoke this component.
[ADR 0095](../adr/0095-durable-live-publication.md) defines the authoritative
contract and remaining recovery scope.

## Live completion

`BeginDurablePublication` adds operation records to the existing opaque
[live publication lifecycle](LIVE_RESTORATION.md). `Apply` publishes one checked
replacement. `Finish` retains that actual owned replacement and completes its
durability checks; `Restore` returns to the checked original or aborts an
unapplied operation. Both terminal directions must durably publish their record
before removing their exact owned pending entry.

`Finish` cannot complete an unapplied operation or a legacy `BeginPublication`
handle. Choosing a valid terminal direction binds subsequent retries to that
direction. A failed forward completion cannot become an implicit restore.
Ordinary value copies share the same private lifecycle and mutex. `Close` only
releases resources: unresolved operations retain pending evidence.

The durable constructor first establishes the pending barrier, then uses the
existing bounded local Git supervisor to qualify exclusion. It requires the
existing exact `/.factory/backups/` rule in `.git/info/exclude`, reads back and
syncs that unchanged file and its parent, and confirms effective ignoring and
absence from the index. It does not edit ignore files. Constructor failure after
pending creation retains the barrier, including when query ownership is uncertain.
The remaining lifecycle and inspection perform no subprocess work.

## Inert records

Records occupy `.factory/backups/.publications/MIGRATION_ID.json`, beside rather
than inside immutable saved sets. The directory is private, and records contain
bounded metadata rather than saved content, credentials or runtime history.
Every occupied slot blocks a new transaction; explicit live retries can reuse
only their actual owned record. A copied or edited JSON record grants no authority.

`InspectPublications` reports bounded observations without creating storage or
reconstructing a handle. Its `complete` field describes inspection completeness.
All restoration, rollback, activation, application and pruning fields remain
false. A terminal record alongside any pending entry still reports a blocker;
that record does not prove that barrier removal or an installation completed.
Legacy empty pending entries remain opaque blockers.

The reserved metadata namespace has its own 64-record bound. It must be strictly
validated and does not consume one of the 64 saved-set slots. Unsafe or unknown
entries remain preserved and reported. The existing backup discovery and release
packaging exclusions also cover this namespace.

## Remaining migration work

Abrupt process death loses the live ownership capability. Reading records after
restart does not recover it or authorize deleting pending evidence. Qualified
interrupted recovery is the separate R2.2 deliverable, followed by public rollback
integration. No force-cleanup command is supplied by this source contract.

Associated-record cleanup, safe slot reuse and later-release pruning still need
their qualified lifecycle. Durable completion of one file does not establish
complete installation coverage, runtime compatibility, process quiescence,
activation, script retirement or release readiness. The installed shell
dispatcher remains unchanged, and seamless migration from v0.1.6 remains pending
the full installer and release qualification gates.
