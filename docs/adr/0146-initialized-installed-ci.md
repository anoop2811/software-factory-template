# ADR 0146: initialize ordinary CI for the installed Go factory

Status: implemented; independent integration, ordering and scheduler criteria pass locally. Hosted qualification is tracked in the PR.
Decision: 162. Continues R4 after merged PR #176.

## Boundary

PR #176 merged as `6b8f08c820c347a9c9d12c4afd00b3f7e6aca547` on
2026-11-10 UTC. Its tree matches reviewed head
`3b818205d840e55ba3e1168c2094c80b4b39dc97`. The explicit workflow renderer is
available, but initialization still copies development CI whose source tests are
not shipped, and pack jobs lack installed-runtime preparation. Complete ordinary
new-project CI integration in this slice, including the current Go, TypeScript,
Java Gradle and Java Maven choices and supported pack combinations.

Existing-project workflow reconciliation, privileged opt-in review-lane bootstrap,
combined hosted adopter rehearsal and release qualification remain separate R4
obligations. Do not claim that ordinary workflow creation completes those tasks.
Do not change frozen installation recipes, inventory, custody roles, historical
initializer component behavior, or the explicit `ci workflow` command contract.

## Initialization and preservation

For current qualified initialization, omit newly copied source-development
`.github/workflows/ci.yml` and `<pack>-pack.yml` assets from the plan. Preserve
existing application workflows at those paths and elsewhere byte-for-byte. Apply
the omission before target observation so an unrelated preserved workflow is not
read or followed as an initializer asset. Do not delete existing old-looking jobs.

Authenticated release initialization creates `.github/workflows/factory.yml`.
The release owner supplies the exact authenticated tag, source revision and origin
platform explicitly. An inspection record, local version label or environment
variable alone cannot authorize a downloadable official release. Share the pure
renderer/preparation implementation with the explicit command without introducing
a dependency cycle or a second bootstrap implementation.

Plan the generated workflow before confirmation and publication. An occupied
destination, including a regular file, directory or symbolic link, is a refusal;
unsafe parent directories also refuse. Do not overwrite, back up or partially
initialize on this preflight refusal. Retain the existing identity checks,
publication barrier, confinement and race checks for the planned file.

Explicit local-source initialization has no official downloadable release. It
omits the newly copied development and pack workflows and emits no active
ordinary factory workflow. Explain that hosted CI needs an authenticated release.
Preserve application workflows and the existing local-runtime contract. Do not
invent a remote tag, silently select latest, or fetch network metadata to fill it.

## Generated ordinary job

Reuse ADR 0145's triggers, unique non-PR concurrency, read-only permissions,
immutable checkout, full history, credential isolation and authenticated installer
preparation. Keep one Ubuntu 24.04 job and a twenty-minute bound. No paid model,
privileged event, duplicated per-pack factory job, cache policy, or new CLI flag
is introduced. The existing generic renderer remains unchanged for its callers.

After clean-checkout preparation, provision the union of selected application
tools once. Use selected application language versions as safely encoded data,
not shell source. Install checksum-pinned actionlint once without a Go compiler.
Only Go application packs require Go setup. TypeScript uses its selected Node
version and `npm ci`; Java uses its selected Temurin JDK. Honor the selected Java
build tool and existing Maven wrapper choice. Do not modify application dependency
pins, language defaults, Makefile composition or configured check commands here.

Run the unchanged installed `./factory selftest`, `./factory check`, and structural
evaluation for OpenCode, Claude Code and Codex. Retain configured application
checks and failures; never synchronize or suppress a failed gate in CI. Run
actionlint against project workflows. Preserve selected-pack security coverage
explicitly where Makefile composition cannot guarantee it: Go gosec/govulncheck,
TypeScript/Java OSV scanning, and Gradle compile and SpotBugs checks. Keep the
existing Go dry-run mutation analysis and TypeScript/Gradle mutation checks
advisory. No other command failure is advisory. For Gremlins pass a directory,
not a Go package pattern with an extra `/...` suffix.

Run a single PR-only mock golden evaluation with no model credentials, regardless
of which packs are selected. Merely having a secret must not enable paid calls.
The separate opt-in review lane retains its current explicit policy and is not
claimed as qualified installed CI by this decision.

Use immutable full action revisions and official release checksums for downloaded
tools. New dependencies must have permissive licenses; existing golangci-lint is
an inherited GPL tool, not a newly introduced dependency. Keep that existing
lint capability rather than introducing another linter. Record the exact verified
catalog and official sources before implementing it. Do not download executable
scripts from a mutable main branch. Application setup does not inherit the
preparation step's `GH_TOKEN`; setup actions should not receive an unnecessary
job-token input or enable implicit caches.

## First-sync semantics

OpenCode V1 permission order is semantic: rules are built in entry order and the
last match wins. The old native renderer recursively sorts map keys, while Claude
uses the original Bash rule order. That changes OpenCode policy and makes the
next synchronization alter Claude arrays. ADR 0099's incidental-order statement
does not apply to ordered permission rules.

Preserve raw nested OpenCode JSON subtrees and change only owned root model fields
and each agent's model field. Preserve ordering at both levels of permission maps,
including non-Bash and per-agent permissions, unknown fields, and accepted absent
or null agent values. Keep typed validation and adapter rendering. Do not change
the canonical template's permission policy, sort Claude permission arrays, or
hide the issue by synchronizing twice. Initial generated adapters must already
be stable before any post-init repair.

## Independent qualification

Observe independent Ginkgo/Gomega RED against merged main before production
changes. Assert immediate post-init byte/mode stability, ordered permissions,
configured and inherited model behavior, and equivalence of one all-harness sync
to sequential harness sync. Preserve existing standalone Claude expectations.

Exercise actual packaged release initialization and local initialization. Inspect
parsed generated workflow semantics for all pack selections and Java variants,
including setup deduplication, application failures, security/mutation coverage,
credential scope, mock-only evaluation and exact authenticated provenance. Test
custom workflow preservation, occupied/unsafe destinations and no partial writes.

Commit an untouched initialized project, clone its public files, read the workflow
from that clone and execute its actual preparation and installed-check blocks.
Retain the real generated Makefile and all three harness checks. Bind transport
to the emitted exact revision, poison checkout-local bootstrap and compiler/
Python/model clients for factory-only execution, and include application success
and failure controls. Fixture witnesses for external pack tools do not prove
hosted application-tool downloads; record that limit and retain the later hosted
rehearsal. Run actionlint on all generated variants.

Register actual acceptance discovery counts without weakening scheduler controls,
timeouts or historical assertions. Build, test and run hooks only in complete
private checkouts; preserve the operator's ignored binary and unrelated files.
Independent review, exact-head hosted CI and release qualification are distinct
evidence. No stable-release readiness claim follows from this slice alone.

## Authoritative ordering evidence

Fetched 2026-11-10 UTC: OpenCode's tagged
[V1 permission implementation](https://raw.githubusercontent.com/anomalyco/opencode/v1.2.15/packages/opencode/src/permission/next.ts)
constructs rules with `Object.entries` and selects with `findLast`; its tagged
[V1 permission documentation](https://raw.githubusercontent.com/anomalyco/opencode/v1.2.15/packages/web/src/content/docs/permissions.mdx)
states that the last matching rule wins. The current website describes V2 arrays;
this change preserves the repository's existing V1 schema.

## Verified tool catalog

Official release/tag APIs and tagged action definitions fetched 2026-11-10 UTC
provide these pins for the new ordinary workflow. Application dependency versions
in existing pack metadata are not changed.

| Tool/action | Version and publication | Immutable identity |
| --- | --- | --- |
| [checkout](https://github.com/actions/checkout/releases/tag/v7.0.1) | v7.0.1, 2026-07-20 | `3d3c42e5aac5ba805825da76410c181273ba90b1` |
| [setup-go](https://github.com/actions/setup-go/releases/tag/v7.0.0) | v7.0.0, 2026-07-16 | `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e` |
| [setup-node](https://github.com/actions/setup-node/releases/tag/v7.1.0) | v7.1.0, 2026-10-08 | `949feb2413d6458794dcd2491c4babbbce0c15c1` |
| [setup-java](https://github.com/actions/setup-java/releases/tag/v6.2.0) | v6.2.0, 2026-11-04 | `77dd5b2ce225ea11469928ac317850245a74ae78` |
| [OSV direct action](https://github.com/google/osv-scanner-action/releases/tag/v2.7.0) | v2.7.0, 2026-10-12 | `b1d267aea5211fa648d9d442fa64b90dbdcbdd36` |
| [actionlint](https://github.com/rhysd/actionlint/releases/tag/v1.7.12) | v1.7.12, 2026-03-30 | Linux amd64 archive SHA256 `8aca8db96f1b94770f1b0d72b6dddcb1ebb8123cb3712530b08cc387b349a3d8` |
| [golangci-lint](https://github.com/golangci/golangci-lint/releases/tag/v2.15.0) | v2.15.0, 2026-10-16 | Linux amd64 archive SHA256 `9b7fe6a6bf6334b98decf8d86b11415ea7889afdabdd1959fdce23af2a9ebcea` |
| [gosec](https://github.com/securego/gosec/releases/tag/v2.30.0) | v2.30.0, 2026-10-23 | Explicit Go module version |
| [govulncheck](https://go.googlesource.com/vuln/+/refs/tags/v1.9.0) | v1.9.0, module timestamp 2026-10-11 | Explicit Go module version |
| [gremlins](https://github.com/go-gremlins/gremlins/releases/tag/v0.6.0) | v0.6.0, 2025-12-06 | Explicit Go module version |

Setup actions are MIT; OSV and gosec/gremlins are Apache-2.0; govulncheck is
BSD-3-Clause; actionlint is MIT. The inherited golangci-lint remains GPL-3.0.
Install its official binary rather than an unsupported `go install` build.
Ginkgo's standalone CLI is not needed by the existing Go pack's `go test` command;
do not add an unnecessary CLI/module version mismatch to application checks.

The tagged setup action definitions support explicit empty `token` inputs. Disable
Go's `cache`, Node's `package-manager-cache`, and Java's `cache-jdk`; omit Node and
Java package-manager `cache` inputs rather than passing the invalid value `false`.
The gosec v2.30.0 tagged CI explicitly exercises Go 1.27.3; gosec and govulncheck
declare Go 1.26 minimums. Tool installation may use Go's ordinary toolchain
selection without changing the application's selected setup version. Complete
runtime qualification of these downloads is still separate evidence.

The pinned OSV direct action uses `scan-args` and performs no SARIF upload; it
does not require security-events write permission. Retain its existing no-lockfile
success policy. Its metadata pins a versioned Docker image, not an image digest;
do not claim complete container immutability. The Gremlins command accepts a
directory and appends the package recursion itself, as shown by its tagged
[coverage implementation](https://raw.githubusercontent.com/go-gremlins/gremlins/v0.6.0/internal/coverage/coverage.go).

## Review refinements before qualification

Explicit candidate initialization historically admits authenticated literal tags
such as `candidate-r4` or `v1.2`, while `ci prepare` requires a full canonical
semantic version. Preserve that initializer contract. If the authenticated tag
is not eligible for CI preparation, complete initialization without creating the
ordinary workflow and clearly explain that a canonical release tag is needed.
Do not relabel an authenticated candidate as a local build, infer a replacement
tag or relax preparation's exact-release validation. Share the existing eligibility
decision rather than inventing a second, inconsistent tag grammar. Occupied
`factory.yml` is only a publication collision when a workflow is actually planned.

The installed optional review workflow already contains the valid job-level
`cache-mode` key. The qualified actionlint parser still rejects that key. Reuse
the repository's exact diagnostic exclusion for this one parser limitation,
without ignoring other syntax, expression or shell errors. Add independent
eligibility and lint compatibility controls; a deliberate unrelated workflow
error must still fail. This does not enable a cache or qualify review bootstrap.
GitHub's authoritative
[cache-mode documentation](https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching#controlling-cache-access-with-cache-mode)
was fetched on 2026-11-10 UTC; the local v1.7.12 parser reproduced the exact
unsupported-key failure against the existing review template.

## Real application-check oracle refinement

The first composed run completed the emitted healthy check, all three structural
evaluations and mock golden evaluation, then retained the deliberate application
failure. Its final fixture count incorrectly expected two Vitest calls overall.
The unchanged TypeScript Makefile runs Vitest as its `test` prerequisite and the
unchanged configured check command runs it again. Thus the healthy request makes
two calls, while the deliberately failing request stops at its first call.
Assert exact per-request deltas of two and one, with the existing failure and
structural-output checks retained. Do not change or bypass application checks to
fit the mistaken fixture count. This is an oracle correction, not an application
optimization.

The locally chosen eight-minute aggregate qualification command subsequently
expired during the partition proof, after the handoff scenarios and ordered-sync
case completed. No child deadline expired. Qualify the actual integration group
and metadata proofs separately with explicit aggregate allowances based on their
observed work. Product, child, hosted CI and watchdog deadlines remain unchanged.

## Local qualification receipt

With the qualified Go 1.27.3 environment and CLI/controller/native race settings,
the independent private checkout ran:

```sh
go test -race ./acceptance -run TestAcceptance -count=1 -timeout=15m \
  '-ginkgo.focus=Whole installation upgrade release candidate handoff initialized CI' \
  -ginkgo.no-color -ginkgo.fail-on-empty
```

Observed package output: `ok github.com/anoop2811/software-factory-template/acceptance 476.166s`.
The retained JSON reports four passed, zero failed in 474.836s. This includes 13
actual initialized workflow variants, preservation/refusal cases, authenticated
ineligible-tag and local-source omission, and untouched init/commit/clone execution
of the real installed checks and mock evaluation with application failure control.
The separate ordering/partition/worker run passed three criteria in 97.320s,
including the original removed-priority negative control. Discovery is 3348 total.
Actionlint v1.7.12 accepted all 13 emitted YAML files; the narrow review-template
compatibility exclusion still rejects an unrelated invalid workflow property.
Configured Go lint reported `0 issues.` and `go vet ./...` exited zero.

Independent security, correctness and test review has no surviving findings.
Application tool-download execution, hosted adopter rehearsal and release readiness
remain distinct evidence; these local witnesses do not claim those outcomes.
