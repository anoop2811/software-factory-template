# Authenticated standalone review-client bootstrap

Status: implemented; hosted qualification passed and PR #181 merged.
Decision: 169. Continues R4 after merged PR #180.

## Scope

Provide the one missing compiler-free acquisition boundary for a privileged
installed review workflow. Export a selected official host-native factory
executable into caller-selected disposable scratch. This operation does not
prepare an installation, inspect or change project files, execute checkout
scripts, enable reviews, change model/provider policy, or call a model.

Keep this PR bounded to bootstrap and its executable contract. The next dependent
R4 PR must pin this installer from its reviewed merged commit, wire the generated
workflow, and migrate exact predecessor-owned enabled review workflows through
the existing backup/recovery transaction. Off or absent lanes stay off; customized
workflows remain preserved conflicts. That integration and the fresh/migrated
adopter rehearsal remain within the existing R4 gate, not new release gates.
Template delivery alone does not qualify already-enabled users.

## Public boundary

Add the same operation to the native Cobra-owned CI surface and trusted installer:

```sh
factory ci review-client --go-candidate TAG --expected-revision SHA --output DIR
sh install.sh ci review-client --go-candidate TAG --expected-revision SHA --output DIR
```

DIR is an absolute, canonical, new physical directory outside the caller's
working-directory tree. Its parent must already exist. Spaces and shell-looking
characters are literal filename data. Reject empty paths, traversal, linked
ancestors or leaves, pre-existing destinations and unsafe parent custody.
Do not create missing parents. The operation creates a private 0700 directory
containing exactly one 0700 executable named factory-review-client.

Require one exact full canonical release tag under the existing CI release
grammar and one lowercase forty-character expected source commit. Reject
duplicates, unknown flags, extra operands, conflicting selectors and missing
values with status 2 before network activity or filesystem mutation. Support
split and equals forms. Help is local and does not acquire or publish anything;
supplied malformed operands remain invalid with help. No origin target is
required: review consumes data and does not reconstruct a cross-platform
installation. Existing ci prepare/workflow behavior stays compatible.

## Authentication and selected owner

Reuse the existing official acquisition and supervision implementation. The
installer resolves the selected tag and compares its commit to the explicit
expected revision before download, chmod or first execution. Existing official
repository, signer workflow, exact source ref, subject name/hash, trusted-root
and self-hosted-runner restrictions remain. Clear the inherited private bridge
protocol at the installer CI boundary.

The native operation independently compares the acquired source revision with
the explicit expected revision, including direct calls and selected-owner
handoffs. Retain authenticated raw-runtime/installation-image agreement.
Only the matching authenticated host owner may interpret and export its
selection. Preserve literal arguments, stdin/stdout/stderr, cancellation,
bounded descendant ownership and the selected owner's exit status. Never fall
back to source compilation, a project launcher, a saved script or another tag.

The provider key is unnecessary for bootstrap. Future workflow integration must
supply it only to review execution. GitHub authentication may be required for
acquisition; a token in a privileged review job is not made read-only merely by
placing it in a different step. No code or dependencies from a PR head may run.

## Output custody and failure behavior

Use existing confined filesystem and checked streaming publication primitives.
Retain directory identity through reservation, copying and final publication;
reject links, owner/mode changes and a late occupied leaf. Bound the raw input
to the existing 256 MiB limit, verify its authenticated digest while copying and
verify the prepared bytes before non-replacing publication. Do not expose an
executable partial copy under the final name. Recheck directory custody before
announcing success.

On success write one JSON object to stdout with schema_version 1, version,
revision, target, sha256 and path. The path names the exact published executable;
the other fields match authenticated acquisition. Emit no successful receipt
until publication and custody checks finish. The exported binary must run both
the existing single-request review and batch-review interfaces without installed
private assets, a compiler, Python or a source tree. Their existing configuration
discovery and provider/cost choices remain unchanged. The exporting owner closes
its publication custody and completes its acquisition cleanup before writing
the receipt. Existing enclosing installer/selected-owner supervisors retain
streaming stdout and may subsequently report cleanup uncertainty. Consumers
must require both a valid receipt and final status 0; receipt text never overrides
an enclosing nonzero status. Do not redesign stream handoff for this operation.

Failures return nonzero and preserve foreign files. No acquisition/publication
failure emits a successful owner receipt; enclosing cleanup failures retain the
status rule above. Clean only proven-owned staging and empty reserved directories.
Uncertain ownership must retain evidence with a concrete diagnostic instead of
broad removal. An output/close failure after publication may leave the complete
private client; report failure and never silently reuse that directory. Existing
temporary acquisition cleanup and uncertainty retention remain authoritative.
Do not add persistent caches or backups for disposable review clients.

## Independent acceptance before implementation

The spec-writer owns Ginkgo/Gomega criteria; the implementer does not edit tests.
Run focused RED before behavior, then GREEN with the actual compiled executable.
Reuse existing supervised fixture owners and image production. Do not run heavy
image builds concurrently or reduce existing product assertions/deadlines.

1. Compare native/installer parsing, including help, malformed revisions/releases,
   duplicate flags, equals forms, literal output operands and poison no-network
   witnesses. Preserve existing ordinary CI grammar.
2. Observe installer authentication and expected-revision rejection before first
   execution; prove literal handoff, inherited bridge clearing and native status.
3. Acquire real test-packaged host bytes through a controlled GitHub boundary,
   compare exported digest/mode/receipt, then run its real review interfaces
   against bounded fake transports. Keep Go/Python/project scripts poisoned,
   cwd/index/config/application files unchanged and no provider call in bootstrap.
4. Refuse authentication failure, moved tag/revision mismatch, wrong subject and
   corrupted bytes without a published client; no source/legacy fallback.
5. Exercise occupied output, linked ancestry/leaf, malformed path, late leaf
   collision and publication interruption with preserved outside canaries.
   Pair refusal assertions with successful actual publication.
6. Qualify actual cancellation/cleanup ownership and failed receipt output.
   Keep existing installer and native preparation acceptance green.

Targeted qualification, affected shared-package race checks, lint/security gates,
independent correctness/security/test review, exact-head Linux/macOS CI and model
review are required before merge. Record actual counts and timing; do not infer
published-release or adopter-pilot readiness from this source change.

## Trust references

Existing first-execution and selected-owner contracts:
[ADR 0144](0144-ci-installer-bootstrap.md) and
[ADR 0101](0101-go-release-handoff.md).
Existing installed preparation remains separate:
[ADR 0143](0143-ci-checkout-preparation.md).

GitHub guidance fetched 2026-11-13:
- https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#pull_request_target
- https://docs.github.com/en/actions/reference/security/secure-use

## Qualification record

Local qualification on 2026-11-13 used Go 1.27.4 on darwin/arm64, with
GOTOOLCHAIN=local, GOENV=off and GOMAXPROCS=3. The initial four surface criteria
failed before implementation because the CI operation did not exist. Independent
review then found a final-leaf custody gap: directory identity and the expected
name alone did not detect replacement or mutation of the executable. All four
new mutation controls failed before the correction. The retained leaf now uses
the existing installation filesystem identity and digest checks through closure.

Executed final affected acceptance:

```sh
go run -mod=readonly github.com/onsi/ginkgo/v2/ginkgo --race --procs=2 \
  --timeout=25m --keep-going --fail-on-empty --no-color \
  --focus='^Go factory command acceptance (Whole installation upgrade release candidate handoff CI checkout preparation|Authenticated ordinary Go installer bootstrap|Cooperative acceptance fixture lifecycle|Go source qualification worker execution|Go source qualification scheduling|Go source qualification workflow partition|The developer-built Cobra command boundary)( |$)' \
  --json-report=.factory/ci-mode-evidence/review-bootstrap/affected.json \
  ./acceptance -- -test.count=1 -test.timeout=25m
```

Observed output: `Ran 68 of 3478 Specs in 680.694 seconds` and
`SUCCESS! -- 68 Passed | 0 Failed | 0 Pending | 3410 Skipped`.
The JSON report identifies all nine new bootstrap criteria as passed, including
both actual standalone review interfaces, the cancellation witness and failed
receipt output. The shared image is built once for its five project criteria.
The existing release-ci partition grows from 19 to 28 criteria; no new job,
worker limit or deadline is introduced.

`make go-runtime-source-check GO_RUNTIME_TEST_GROUP=packages` passed formatting,
the Ginkgo-only gate, vet and all package race tests, then stopped on three
test-only lint findings. After those test corrections, these commands exited 0:

```sh
go test -race -count=1 ./internal/releasecmd -ginkgo.no-color -ginkgo.succinct
golangci-lint run --config packs/go/.golangci.yml ./...
gosec ./...
govulncheck ./...
shellcheck -s sh -S warning install.sh
```

Gosec reported `Issues : 0`; govulncheck reported no vulnerabilities affecting
the code. The release-command suite has twelve criteria, including the four
previously failing leaf mutation controls. Both `/bin/sh -n install.sh` and
`/bin/bash -n install.sh` also exited 0.

Factory checks also exited 0: `./scripts/selftest/run.sh` reported fourteen
shipped gates proved; `./scripts/selftest/development.sh` reported `218 passed,
0 failed, 0 skipped`; `./scripts/selftest/adversarial-review.sh` reported
`53 passed, 0 failed`. Citation lint took its configured empty-prefix skip;
copy-manifest, hook-existence, shared-script enforcement and gate-instrumentation
checks passed. Wiki lint took its no-content skip. These are source-development
checks, separate from the new executable acceptance above.

The independent review diamond consolidated one production finding and three
test-oracle findings. Final review found no remaining required correction after
retained-leaf checks, non-reaping canary observation, cooperative test ownership
and directory-inclusive project snapshots. Hosted qualification and model review
must still run on the submitted head; this record does not claim release or
adopter-pilot readiness.

PR #181 merged on 2026-11-14 as
3794e6cc8fa66eb02504e753b0ad11955f5cf85a. GitHub reports the final-head Go run
43539623922 and adversarial run 43539623000 successful on
2fc26e75fcc559f075e79b5462b47e8044f651af. The review job ran from 01:16:57 to
01:19:48 UTC (2m51s). Its six notes were answered in
[the review response](https://github.com/anoop2811/software-factory-template/pull/181#issuecomment-6869321495).
These are the preceding PR's results, not evidence for later integration.
[ADR 0154](0154-installed-review-workflow-migration.md) now governs the dependent
generated workflow and transactional ownership integration.

