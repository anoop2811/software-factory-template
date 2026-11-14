# Installed review workflow bootstrap and transactional migration

Status: implemented; local behavior and package quality qualified; hosted qualification pending.
Decision: 170. Continues R4 after merged PR #181.

## Scope and compatibility

Connect the authenticated standalone review-client operation to the generated
adopter review workflow, and deliver that workflow to exact factory-owned enabled
installations through the existing whole-installation transaction. The reviewed
installer is pinned to merged commit
3794e6cc8fa66eb02504e753b0ad11955f5cf85a. No release activation occurs here.
Fresh and migrated integrated rehearsal remains within R4; R5 published RC4
qualification, R6 representative pilot and R7 stable activation remain the other
fixed release gates. Source conversion remains a separate completed inventory.

Preserve current provider/model/secret/cost settings and the explicit
REVIEW_GO_CLIENT batch opt-in. An unset batch client continues single-request
review with OpenRouter, Anthropic or OpenAI. Do not silently enable batching or
the review lane. Canonical factory source and supported historical script layouts
retain their existing routes. Installed Go projects use the standalone client;
they do not need a compiler, private runtime assets or a committed executable.

## Trusted generated workflow

Keep trusted-base checkout, same-repository/default-base admission, exact base
and head binding before publication, existing cache boundaries and the twenty
minute job ceiling. Never check out or execute PR-head code. Credentials for
the provider are scoped to review execution, not acquisition. The job's GitHub
token retains its declared job permissions; a separate preparation step does
not make that token read-only.

Read installed identity as bounded inert literal data from .factory-version and
the generated factory launcher header. Require an exact canonical release tag,
lowercase forty-character source revision, and agreement between both records.
Do not source either file or execute the project launcher to discover identity.
A declared installed layout with malformed, incomplete, linked or disagreeing
identity fails; it must never fall through to scripts or source compilation.

Download only the immutable pinned official installer and invoke its public
ci review-client operation with both tag and expected revision and a new private
output directory outside the checkout. Require final status zero and exactly one
valid JSON receipt before publishing the executable path as a step output.
Validate schema, selected identity, supported runner target, exact expected path,
regular non-linked executable, private mode and matching digest. The launcher's
digest belongs to the origin installation and must not be compared with the
runner-host client digest. Preserve bounded network/command failure behavior.
Never reuse a pre-existing output or bootstrap failure as a reason to fall back.

Invoke the acquired client's public review command for the single-request path,
with FACTORY_BRIDGE_PROTOCOL and REVIEW_GO_CLIENT cleared. Keep the explicit
external batch client's existing invocation and budgets separate. Preserve fixed
public failure diagnostics, private provider stderr, comment/ref validation and
report artifacts. Bootstrap success alone must not imply a successful review.

## Ownership and one transaction

Recognize .github/workflows/adversarial-review.yml only by exact bytes and mode
from an authenticated predecessor template and its qualified historical renderer.
The managed header alone is insufficient. Frozen Bash profiles and supported Go
predecessors retain their own rendering semantics; review workflow mode is 0644,
including the Go family whose ordinary copied CI workflow used 0600. Reject
unrecognized renderer recipes, missing/duplicate/mismatched manifest assets and
unsupported or ambiguous selected configuration rather than guessing ownership.

An enabled, present, exactly owned workflow is replaced by the authenticated
target template rendering using its existing secret name and settings. An absent
workflow remains absent, including when config says on. An off lane remains off
and is not automatically created or rewritten. A customized or unsafe workflow
that would otherwise be migrated is preserved as a blocking conflict. Config
changes after consent invalidate the transaction. No provider key is stored in
plans or journals; only the validated secret name and bounded selected state may
participate in the replay context. Never call review-lane enable from migration.

Use the existing installation preview, consent, backup, publication, rollback
and durable recovery owners. Extend the CI component with a versioned finite
review row/context; preserve schema-1 five-row journal replay exactly. Do not
append the review path unconditionally to the legacy global row set. Recovery
must freshly reproduce exact predecessor and target workflow bytes from retained
authenticated images and inputs, and reject missing/duplicate/forged rows or
changed retained evidence. Ordinary runtime admission remains distinct from
transaction-only workflow ownership. No separate recovery mechanism or backup
directory is introduced.

Reuse the small target rendering operation where practical. Frozen historical
recognition remains independent of today's rendering implementation. Avoid
changing initializer sources merely to pass release identity into the workflow;
the generated workflow reads the committed installed identity at execution time.

## Independent acceptance before implementation

Separate test/spec ownership from production implementation. Use Ginkgo/Gomega,
focused RED before implementation and the existing supervised fixture owners.
Keep image production shared, heavy suites sequential, existing product deadlines
and the full criterion union intact. Do not add CI jobs merely to host this work.

1. Execute the actual generated workflow selection/bootstrap/review scripts with
   recording transports and poisoned project launchers/compilers. Prove literal
   identity handling, immutable installer pin, final-status/receipt validation,
   private scratch, no provider key during acquisition and no unsafe fallback.
2. Exercise installed success and malformed identity, nonzero bootstrap with a
   plausible receipt, invalid receipt/hash/path/target and linked/missing inputs.
   Preserve explicit batch, canonical source and historical compatibility routes.
3. Prove fresh opted-in initialization and later enable deliver the new template
   while ordinary off initialization stays off. Keep all supported single-request
   provider settings, explicit batch opt-in and private error redaction intact.
4. Qualify real historical Bash and supported Go enabled workflow projections,
   including default/custom valid secret names, exact modes and current settings.
   Refuse header-only ownership, changed bytes/mode, ambiguous config and recipe
   or manifest tampering. Preserve off/absent and unrelated application files.
5. Exercise public upgrade preview/apply and backup/rollback/recovery with the
   review row, exact original bytes restored, late configuration/workflow changes
   refused, and one combined transaction. Retain legacy schema-1 recovery controls.
6. Run affected package race/lint/security checks, factory and workflow gates,
   independent correctness/security/tests review, exact-head Linux/macOS CI and
   the bounded model review. Record actual commands/results and review closure;
   do not claim published-release or pilot readiness from source qualification.

## Qualification fixture boundaries

The first complete local group ran all 25 criteria: 22 passed and three new
fixture cases failed. Two recovery cases stopped after the review row while a
later ordinary workflow retirement was between unlink and its durable observed
record. Existing recovery correctly refused that ambiguous absence. Successful
review recovery criteria must witness the review's applied bytes and the
transaction's checking/metadata-prepared phase, then reobserve the stopped owner
before terminating it. Retain the production refusal for ambiguous deletion.

The genuine merged predecessor 3794e6cc declares Go 1.27.4, while the existing
historical fixture builder deliberately requires 1.27.3 for earlier generations.
Reuse the immutable-image builder with an explicit qualified compiler callback
for this predecessor; preserve the old default and its exact compiler assertions.
Record actual executable build metadata in the packaged fixture provenance.
Do not rewrite predecessor source or permit automatic toolchain downloads.

Share the existing ordered migration group's candidate images and legacy
initializer owner with the new review criteria. Preserve the full criterion
names, independent projects, leaf source locations and existing deadlines. The
initial combined run took 1309.410 seconds, including 838.690 seconds in existing
criteria; avoiding duplicate candidate production gives qualification more room
without changing product assertions or adding jobs.

## Local qualification

Qualification uses Go 1.27.4 on darwin/arm64 with GOTOOLCHAIN=local,
GOENV=off and GOMAXPROCS=3. Before implementation, executing the generated
selector returned only mode=legacy instead of selecting an installed client;
the public v0.1.6 upgrade preview exposed five ordinary rows rather than the
required sixth review row. A separate schema control also showed schema 1
silently accepting the newly supplied review context. These were behavioral
failures, separate from the corrected first fixture's unborn Git HEAD.

Executed the affected workflow/source/scheduling selection under race with two
workers:

```sh
go run -mod=readonly github.com/onsi/ginkgo/v2/ginkgo --race --procs=2 \
  --timeout=25m --keep-going --fail-on-empty --no-color \
  --focus='^Go factory command acceptance (Installed review workflow execution|Budgeted review workflow security and publication|Go source runtime entrypoints review adapters|Go source qualification worker execution|Go source qualification scheduling|Go source qualification workflow partition)( |$)' \
  --json-report=.factory/ci-mode-evidence/installed-review/workflow-source.json \
  ./acceptance -- -test.count=1 -test.timeout=25m
```

Observed output: `Ran 36 of 3495 Specs in 232.642 seconds` and
`SUCCESS! -- 36 Passed | 0 Failed | 0 Pending | 3459 Skipped`.
This includes the generated installed bootstrap, all three single-request
providers, failed receipts and custody checks, source/legacy/batch compatibility,
private-error handling, and exact scheduling identities. The earlier focused
seven-case run caught an assertion expecting unquoted configuration output;
the corrected assertion compares parsed values independently.

Actionlint v1.7.12, using the repository's existing narrow cache-mode exception,
exited 0 for the generated template, including its ShellCheck pass. Independent
security and correctness review found no required production correction. Test
review identified the scheduler's old file-origin assertion; the new precise
prefix retains exact origin checking for the ten new criteria.

Executed the complete migration group, including its fifteen existing criteria
and ten new installed-review criteria:

```sh
make go-runtime-source-check GO_RUNTIME_TEST_GROUP=ci-migration \
  GO_RUNTIME_REPORT_DIR=.factory/ci-mode-evidence/installed-review/final-migration
```

Observed output: `Ran 25 of 3495 Specs in 1333.762 seconds` and
`SUCCESS! -- 25 Passed | 0 Failed | 0 Pending | 3470 Skipped`.
This covers legacy and genuine Go predecessor upgrades, retained off/absent
choices, customized conflicts, recovery in both directions and backup retention.
The subsequent lint stage returned one append-assignment style finding and three
false positives for repository secret names. The test writer changed only the
append assignment; the implementer renamed the local secret-name variable.
The subsequent complete package quality gate ran on those corrections:

```sh
make go-runtime-source-check GO_RUNTIME_TEST_GROUP=packages
```

It exited 0 after formatting, the Ginkgo-only gate, vet, non-acceptance race tests,
CLI compilation, golangci-lint (`0 issues.`), gosec (`Issues : 0`) and govulncheck
(`Your code is affected by 0 vulnerabilities.`). Independent review of the two
lint-only corrections found no assertion or behavior change.

The complete fresh default-initialization family and current exact scheduling
criterion ran under race with two workers:

```sh
go run -mod=readonly github.com/onsi/ginkgo/v2/ginkgo --race --procs=2 \
  --timeout=25m --keep-going --fail-on-empty --no-color \
  --focus='^Go factory command acceptance (Go default packaged initialization|Go source qualification scheduling executes disjoint feature partitions while retaining every source quality stage)( |$)' \
  --json-report=.factory/ci-mode-evidence/installed-review/fresh-init-scheduling.json \
  ./acceptance -- -test.count=1 -test.timeout=25m
```

Observed output: `Ran 11 of 3495 Specs in 315.643 seconds` and
`SUCCESS! -- 11 Passed | 0 Failed | 0 Pending | 3484 Skipped`.
The emitted enabled workflow equals the current candidate template with the
selected OpenAI secret and immutable installer pin; ordinary off initialization
emits no review workflow. Hosted qualification and the separate integrated R4
rehearsal remain pending. These local fixtures do not qualify a published release.

Factory regression commands also exited 0: `./scripts/selftest/run.sh` reported
`14 shipped gates proved, 0 failed`; `./scripts/selftest/development.sh` reported
`218 passed, 0 failed, 0 skipped`; `./scripts/selftest/adversarial-review.sh`
reported `53 passed, 0 failed`; `make eval` reported structural PASS for OpenCode,
Claude and Codex. This is structural adapter evidence, not live model parity.
The shared-script, hook-existence, copy-manifest, gate-instrumentation, wiki and
workflow checks exited 0. Citation lint reported its configured empty-prefix
skip; `git diff --check` exited 0.

## References

Existing contracts: [ADR 0153](0153-authenticated-review-client-bootstrap.md),
[ADR 0148](0148-transactional-installed-ci-migration.md) and
[ADR 0089](0089-go-native-review-lane.md).

GitHub guidance fetched 2026-11-14:
- https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#pull_request_target
- https://docs.github.com/en/actions/reference/security/secure-use
