# Go replacement release readiness

This is the delivery checklist for the approved conversion specification, not a
second feature roadmap. Source implementation progress and release readiness are
different measurements. At merged PR #124, the fixed source plan is 60.7%.

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

These components do not establish a complete installed Go upgrade. The shipped
factory launcher and ordinary upgrade still use the script installation.

## Required release gates

- [x] Complete committed source-image bundle and fixed legacy reference catalogs,
      with shared validation and authenticated closed-world payload custody.
      ADR 0097 source implementation: 98 new criteria and 72 unchanged V1 criteria
      passed locally. PR #124 merged after all thirteen checks passed, including
      complete Linux/macOS source gates and four native V1/image target jobs.
- [ ] Complete reviewed installation selection and retain/replace/retire map for
      each supported predecessor and the Go target. Source/test/development blobs
      in the image are not all installation assets and grant no ownership.
- [ ] One whole-installation upgrade transaction retains exclusion and pending
      admission across all changed files, durable ignored backup, candidate checks,
      activation and the durable installed-version result.
- [ ] Public upgrade, explicit rollback and interrupted recovery reuse that
      transaction and independently qualified images; later edits and unsupported
      predecessor state remain preserved conflicts.
- [ ] Complete remaining factory-owned orchestration and adapter/dependency audit.
      Preserve required public/sourceable paths as thin adapters. Intentional
      validation gates, product build files and native host formats remain.
- [ ] Init/install/bootstrap and normal upgrade select a verified packaged Go
      runtime without requiring Go or Python for shipped factory behavior.
- [ ] Retire only unchanged, explicitly owned/adopted obsolete implementations
      after compatible Go activation. Backups are inert and ignored; unknown,
      edited, held or unsafe entries are individually reported and preserved.
- [ ] Later distinct successful forward releases prune eligible older recovery
      sets while keeping the immediately preceding usable installation. Same-release
      retries, rollback and local builds never advance retention.
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

## Pending operator input

The pilot project and evidence period/exit criteria were requested from Anoop on
2026-10-06. Q5 of the conversion spec remains open until answered. This does not
block independent implementation of the source-image and installation consumer.

## Current installation-consumer qualification

The unmerged installation-consumer branch adds the opt-in go-hybrid-v1 whole
upgrade, rollback and interrupted-recovery paths specified in
docs/adr/0098-whole-installation-upgrade-and-rollback.md:14. It does not change
default bootstrap or establish a replacement release.

Source qualification now separates complementary base and installation groups,
with all as the local default. Actual pinned Ginkgo discovery selects 76
installation criteria and 2,579 base criteria; an empty installation selection
fails. Eight scheduling regressions passed under race detection. One complete
current-source installation run also passed all 76 selected criteria with race
detection (package 1257.089s). The broader local source run encountered
sandbox-denied process inspection, loopback listeners
and filesystem fixtures; vulnerability data was unreachable. Native hosted CI and
the remaining release gates still require evidence. Decision 97 records the
commands, observed results and qualification limits.

Keep the release checkboxes and merged source percentage above unchanged until
the corresponding gates have completed; this branch's implementation is not a
merged-progress increment.
