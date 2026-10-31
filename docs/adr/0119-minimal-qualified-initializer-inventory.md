# ADR 0119: publish only consumed qualified initializer data

Status: accepted implementation contract; independent RED precedes production

Date: 2026-10-30 UTC

## Complete function boundary

After human merge of #149 as `9be9d0333d018c51dfccea18da966e6138e380ab`,
reduce the persistent private scaffold selection of a new qualified Go
installation from 85 cached assets to these two consumed data files:

- `.factory/assets/scaffold/templates/metrics.html`
- `.factory/assets/scaffold/packs/review-lane/review-pr.yml`

Installed metrics HTML reads the first through its selected asset directory;
review-lane enable reads the second. Keep the complete private installation
package as the initializer's planning input. Project-facing scaffold files,
language pack selection, native adapters, configuration and gate proofs retain
their current behavior. Two cached files does not mean two installed files.

The current reviewed role map has 141 rows, including 85 inert cache rows and
three existing retirement rows. Removing the 83 unused cache rows produces 58
reviewed rows, 55 active selected assets and 53 descriptor asset entries. Keep
the descriptor schema and `go-hybrid-v1` kind; its selected generation is
qualified from source and complete image bytes, never inferred from a count.

Do not delete, rewrite, chmod, adopt or back up those 83 existing cache entries
merely because this inventory no longer selects them. An upgrade may leave
them physically present, including edited or unknown files. Their qualified
retirement is a separate function in ADR 0114. Do not change ordinary command
defaults, source bootstrap, source selftest, shared controller entrypoints,
toolchain or model pins, release activation or retention expiry policy here.

## Freeze the predecessor before changing selection

The accepted RC3 vector in ADR 0115 is not a description of the current
post-RC3 generator. Before changing production, record a second closed
fingerprint from immutable `9be9d0333d018c51dfccea18da966e6138e380ab` over:

```text
internal/installation/generate.go
internal/installation/roles.go
internal/installation/orchestration.go
internal/installation/reference.go
internal/installation/model.go
internal/installation/observe.go
internal/installation/selection.go
internal/installedlayout/layout.go
internal/installation/recipe.go
internal/installation/roles_v1.go
```

An independent literal-data oracle must compare every ordered role field from
that commit with `rolesV1`. Read-only inspection found the same 141 rows; use
the existing frozen table only after that equality is independently proven.
Keep the RC3 fingerprint unchanged. The two closed historical generations may
share inert role data and current reviewed generators; never execute imported
historical source or duplicate a generator implementation.

Use one image-specific role selector for current and supported historical
selection, descriptor construction and whole-installation planning. A matched
closed historical image uses the frozen 141-row table. The new current image
uses the reduced 58-row table. Preserve the existing exact-current-closure
comparison; any other predecessor fingerprint still refuses without mutation.

## Journals, recovery and existing data

A transaction belongs to the generation of its original source image. New
58-row proposals create 58-row journals and receipts. Existing 141-row records
remain 141-row records when inspected or recovered by a newer consumer.
Never rewrite old records into a new inventory or accept their row count as
ownership proof.

Parsing may recognize either closed path set, requiring exact membership,
uniqueness and complete entries. Recovery, preparation recovery, retention
qualification and pruning must then bind that set to freshly inspected images
and the appropriate generation. Keep unknown, mixed or incomplete inventories
blocked or preserved. A recognized journal is descriptive evidence, not
publication or deletion authority.

Reconstruct both historical descriptors with their original role filter and
exact bytes and modes. Keep existing cache entries untouched during forward
upgrade and rollback. If restoring a historical descriptor would require an
old required cache entry whose current bytes, type or mode no longer match the
independently qualified predecessor, report a preserved blocker before reverse
publication. Do not overwrite that entry from an archive, execute saved code,
or activate an unusable old descriptor and discover the mismatch afterward.
The same completeness requirement applies when retention proves a usable
alternate recovery before pruning: an unavailable old required cache cannot
authorize deletion of another recovery set.

The existing backup, pending barrier, fresh-consent, exclusive-admission and
inert rollback mechanisms remain the transaction owner. This change must not
advance expiry or turn local images into authenticated retention authority.

## Production ownership and reuse

The bounded implementation owns the current role data and role map, the
fingerprint/role selector, and its existing selection/descriptor/planning
consumers. Expected consumers are `selection.go`, `generate.go`, `proposal.go`,
`recovery.go`, `preparation_recovery.go`, `storage.go`, `transaction.go`,
`legacy_health.go`, `retention_scan.go`, `retention_qualify.go` and
`retention_prune.go` under `internal/installation/`.

Keep the selector and closed record-shape helpers with the existing recipe
collaborator. Reuse the shared generator and descriptor pipeline. Update
`docs/migration/INSTALLATION_ROLE_MAP.json` alongside current literal roles;
remove unused current cache declarations without declaring them retired.
No new library or public API is required. The initializer's full private plan
already consumes `installation.Selection`; do not fork it or filter its source
archive to achieve a smaller persistent selection.

## Independent qualification and CI

An independent author first observes meaningful failure through the compiled
public initializer: new qualified installations currently contain 85 cached
files instead of two. Separately force a generation boundary for a genuine
immutable post-RC3 predecessor while leaving that predecessor untouched; the
old RC3-only matcher must fail before the new closed recipe is implemented.

Require exact two-file private cache and descriptor membership after real
qualified initialization, with the full project scaffold still produced.
Run installed selftest, metrics HTML and review-lane setup with a poisoned
compiler to demonstrate that neither a scaffold cache nor rebuild is needed.

Cover genuine RC3 and post-RC3 preview, apply and exact rollback, preserving
all existing cache files and unrelated state. Replay an existing 141-row
transaction through the new consumer, including preparation or interrupted
recovery where supported; assert original entry sets and receipts survive.
Require unknown fingerprints and mixed/incomplete records to refuse. An edited
unused historical cache entry must remain untouched and block an otherwise
unusable historical rollback before publication.

Reuse the existing `historical-recipe` Linux/macOS CI lane for these bounded
ordered scenarios if its observed runtime remains within the existing 25-minute
test and 30-minute job allowances. Extend only the exact family selector if the
new feature family needs its own name; do not add a job merely for organization.
Derive the seven-partition census from real Ginkgo identities and preserve full
union/failure propagation, race checks, worker bounds and reports. Increase no
deadline, retry, worker, model or budget limit in this function.

Each function keeps its own PR. Independent branches may proceed in parallel;
dependency-ordered review, findings responses, checks and human merge remain
required before integration or release claims.

## Provenance

Read-only inspection on 2026-10-30 UTC of immutable main `9be9d033` found 85
current inert cache rows and identical 141-row literal arrays in `roles.go` and
`roles_v1.go`. The existing consumers are
`internal/metricscmd/html.go:19` and
`internal/reviewlanecmd/mutation.go:83`. The delivery boundary follows
`docs/adr/0114-incremental-go-cutover-delivery.md:60`; closed reconstruction
extends `docs/adr/0115-frozen-rc3-recipe-compatibility.md:23`.
