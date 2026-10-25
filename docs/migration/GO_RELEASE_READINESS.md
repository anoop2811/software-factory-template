# Go replacement release readiness

This is the delivery checklist for the approved conversion specification, not a
second feature roadmap. Source implementation progress and release readiness are
different measurements. The last recorded fixed source-plan score is 60.7% at
merged PR #124; it is a historical score, not a new estimate for later work.

The supported initial upgrade inputs are v0.1.6 and immutable Bash baseline
76952eaa63aebd1ecd282f5ab51dd7c3627cb497. Unknown/customized installations are
preserved and reported. The spec remains authoritative for scope and exceptions.

## Completed source prerequisites

- Native source commands exist for init, doctor, report, metrics, review-lane,
  configuration migration, budgets and loops.
- Artifact verification, deterministic selection, local packaging and authenticated
  inert staging exist for the original runtime-only bundle.
- Confined adoption/preview, ignored recovery creation and read-only restoration
  planning exist for the bounded six-asset G2 scope.
- Transition exclusion, live restoration, durable publication and interrupted
  recovery source engines are qualified and merged.

The opt-in whole-installation consumer can select an installed Go runtime. The
default source launcher, bare bootstrap and source upgrade still select the script
installation until the release gates pass. Explicit candidate delivery is described
in [GO_CANDIDATE_HANDOFF.md](GO_CANDIDATE_HANDOFF.md).

## Current release candidate

[v0.2.0-rc.1](https://github.com/anoop2811/software-factory-template/releases/tag/v0.2.0-rc.1)
was published on 2026-10-24 to exercise real release authentication and migration.
All assets were built and attested, but the consumer's extensionless private
attestation copy caused all four native qualification jobs to fail before
successful migration. RC1 is marked unsuitable for migration; v0.1.6 remains
latest stable. See [the exact results](PUBLISHED_RELEASE_QUALIFICATION.md#first-published-candidate-result).
A corrected, reviewed new candidate must pass actual published qualification;
source-only checks do not close the gate below.

## Required release gates

- [x] Complete committed source-image bundle and fixed legacy reference catalogs,
      with shared validation and authenticated closed-world payload custody.
      ADR 0097 source implementation: 98 new criteria and 72 unchanged V1 criteria
      passed locally. PR #124 merged after all thirteen checks passed, including
      complete Linux/macOS source gates and four native V1/image target jobs.
- [x] Complete reviewed installation selection and retain/replace/retire map for
      each supported predecessor and the Go target. Source/test/development blobs
      in the image are not all installation assets and grant no ownership.
- [x] One whole-installation upgrade transaction retains exclusion and pending
      admission across all changed files, durable ignored backup, candidate checks,
      activation and the durable installed-version result.
- [x] Public upgrade, explicit rollback and interrupted recovery reuse that
      transaction and independently qualified images; later edits and unsupported
      predecessor state remain preserved conflicts.
- [x] Complete remaining factory-owned orchestration and adapter/dependency audit.
      Preserve required public/sourceable paths as thin adapters. Intentional
      validation gates, product build files and native host formats remain.
      [PR #126](https://github.com/anoop2811/software-factory-template/pull/126/checks)
      merged after all sixteen checks passed, including a
      [completed adversarial review of its final source head](https://github.com/anoop2811/software-factory-template/pull/126#issuecomment-6244742505).
- [ ] Init/install/bootstrap and normal upgrade select a verified packaged Go
      runtime without requiring Go or Python for shipped factory behavior.
- [ ] Retire only unchanged, explicitly owned/adopted obsolete implementations
      after compatible Go activation. Backups are inert and ignored; unknown,
      edited, held or unsafe entries are individually reported and preserved.
- [x] Later distinct successful forward releases prune eligible older recovery
      sets while keeping the immediately preceding usable installation. Same-release
      retries, rollback and local builds never advance retention.
      Source implementation and qualification are in [PR #132](https://github.com/anoop2811/software-factory-template/pull/132)
      and [BACKUP_RETENTION.md](BACKUP_RETENTION.md). This source gate does not claim
      human merge, default selection or release publication.
- [ ] Packaged acceptance passes natively on Linux amd64/arm64 and macOS amd64/arm64
      at the approved minimum operating systems, including actual release authenticity.
- [ ] Complete the mandatory manual adopter pilot and record its project,
      execution evidence and agreed exit criteria before default cutover.
- [ ] Publish the Go replacement release only after the above gates pass and the
      final dependency/reference audit finds no unclassified active remnants.

## Observable release rehearsal

An adopter starting from v0.1.6 can use the normal upgrade flow. The selected Go
binary and required template assets match a verified source revision. Existing
configuration, application files, custom hooks, native permissions and consumed
runtime history remain intact. Every retained public entrypoint reaches its one
declared implementation.

Interruptions at verification, backup, mutation, validation, activation and
cleanup boundaries either retain a complete compatible installation or stop
ordinary work with a concrete recovery action. Rollback does not execute saved
scripts, rewind accounting or overwrite later edits. Successful later-release
retention has visible file/byte totals and preserves all declared exceptions.

No model is called as an upgrade or health test. Existing paid/optional choices
and budgets remain explicit.

## Installed doctor correction

An isolated local adopter rehearsal exposed a sourceable-library permission
mismatch in native doctor. Decision 112 corrects it while retaining executable
requirements for invoked hooks. Independent criteria include an actual qualified
installation at its declared 0644 library mode; local permission and doctor suites
pass. See [the anonymized factory evidence](ADOPTER_REHEARSAL.md).

Require reviewed exact-head CI and new published qualification before selecting
another release candidate. The initial failed candidate, minimum-OS gap, mandatory
operator pilot and default cutover remain open requirements.

## Pending operator input

The pilot project and evidence period/exit criteria were requested from Anoop on
2026-10-06. Q5 of the conversion spec remains open until answered. This does not
block independent implementation of the source-image and installation consumer.

## Current installation-consumer qualification

PR #125 merged on 2026-10-17 after all fifteen reported checks passed. It adds
the opt-in go-hybrid-v1 whole upgrade, rollback and interrupted-recovery paths
specified in docs/adr/0098-whole-installation-upgrade-and-rollback.md:14. This
closes those three source prerequisites above; default bootstrap and replacement
release gates remain open. For PR #125, the advisory model review skipped the oversized diff
and Copilot had no quota, so neither is claimed as completed model review.

The completed orchestration implementation and its qualification are tracked in
[ORCHESTRATION.md](ORCHESTRATION.md). Do not count a
new interface, inventory or review correction as another source-plan package.
The fixed denominator remains thirty; no new percentage is inferred here.

## Published-release qualification lane

[Published-release qualification](PUBLISHED_RELEASE_QUALIFICATION.md) defines the
explicit attested candidate lane under [ADR 0107](../adr/0107-published-release-qualification.md).
Its implementation and ordinary hermetic contract checks do not close the native
published-release gate. Record actual published outcomes and exact platform/OS
evidence separately, including the outstanding macOS 14 Intel minimum-OS proof.
The mandatory adopter pilot and default cutover remain independent requirements.
