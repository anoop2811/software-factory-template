# ADR 0107: qualify published attested releases through the public installer

Date: 2026-10-19 UTC
Status: user-authorized implementation contract; independent RED precedes wiring
Decision: 106

## Scope and completion boundary

After merged PR #132, add the release-candidate qualification lane required by
specs/001-go-runtime-conversion.md:235 (AC 5.1),
specs/001-go-runtime-conversion.md:242 (AC 5.2),
specs/001-go-runtime-conversion.md:250 (AC 5.3),
specs/001-go-runtime-conversion.md:273 (AC 6.2) and
specs/001-go-runtime-conversion.md:280 (AC 6.3). Test real published assets and
GitHub attestations through the supported public installer. Local staging or a
fake verifier is not evidence that an official release authenticates or migrates.

Reuse the existing four-target build/attestation/upload pipeline, public
`--go-candidate TAG` selector, fixed-repository acquisition, installation
transaction, pinned legacy catalogs and acceptance helpers. Add no public command,
installation engine, runtime service, paid model invocation, dependency version or
compiler/interpreter prerequisite for shipped Go behavior. The lane does not
publish a release, choose a moving latest tag, change bootstrap defaults, retire
unreviewed assets or claim the mandatory adopter pilot complete.

## Trusted workflow and explicit selection

Use a reusable qualification workflow and a manual entry point in the official
repository. Invoke qualification after the release workflow has successfully
uploaded all attested assets; publication alone can precede upload and must not
race acquisition. The same lane supports explicit requalification of an existing
non-draft release, including a prerelease. Manual runs execute trusted default-
branch harness code, not an arbitrary dispatch ref or candidate-provided tests.
Record the exact trusted harness commit separately from the candidate revision.

Require one literal release tag and one full expected source commit. Validate
bounded operands as data, resolve the official published release and tag, and
require their repository, tag and resolved source commit to agree with those
inputs before any candidate execution. Reject drafts, aliases, missing or moved
tags, absent assets and unsupported targets. Keep the fixed official repository
and signer workflow policy; neither release metadata nor caller inputs can
substitute a repository, script, workflow or arbitrary executable path.

Qualification needs only repository contents read permission. Signing, OIDC,
attestation-write and release-upload permissions remain confined to the existing
attestation job. Do not pass release-write credentials, persisted checkout
credentials or signing capabilities to downloaded code. Never execute a pull
request head in a release-privileged job. Reusable workflow invocation cannot
increase the caller's token permissions.

Keep finite job/process deadlines, fail-fast disabled across the native matrix,
and deterministic failure propagation. Upload diagnostic evidence on failure
where available, but an absent report, skipped scenario, unavailable verifier or
failed platform is not a successful qualification. No automatic retry, extra
model call or fallback to local artifacts can manufacture a pass.

## Authentication before execution

Use the independently installed real GitHub CLI to fetch trusted roots and verify
the exact downloaded host-specific raw bootstrap, runtime-only archive and complete
installation archive. Bind the official repository, existing runtime-release
signer workflow, `refs/tags/TAG`, full source digest, expected subject name and
actual SHA-256 digest; deny self-hosted signing runners. Preserve the existing
closed-world staging checks. A successful checksum or unverified subject is not
an authentication result. Keep the downloaded archives inert until trusted
verification succeeds; never ask downloaded code to establish its own trust.

Read the public installer from the authenticated candidate's exact Git source
commit in the fixed official repository, not from main, the current website or a
locally rebuilt candidate. Exercise that installer and let its existing real
acquisition path authenticate the selected bootstrap and installation image again.
This additional test-side verification records independent evidence; it must not
replace or bypass the public installer's verification. Do not alter signed
archives, forge version metadata, stub GitHub CLI or use a fake signing service
in the live release lane. Hermetic refusal tests remain separately labeled.

## Published release scenarios

Use an opt-in Ginkgo/Gomega suite selected by the `publishedrelease` build tag.
The ordinary source suite does not register these live scenarios, so its fixed
criterion census and complete source gates remain meaningful. An explicitly
selected live suite requires its tag, expected revision and evidence destination;
missing inputs fail rather than producing an environment-dependent green skip.
The trusted test harness may use its existing Go toolchain; child Go factory
processes run with Go, Python and native model clients unavailable on PATH.

Qualify these independent public-boundary scenarios on each native target:

1. Fresh initialization installs the authenticated Go selection. Ordinary help,
   doctor and the required deterministic installed checks use that selection;
   generated Codex, Claude Code and OpenCode adapters retain their contracts.
2. Genuinely initialize the two supported Bash predecessors from their pinned
   sources: v0.1.6 at `b71ecc32e07ecd87eb330ba8e497c86612f92acd`, and the immutable
   Bash baseline at `76952eaa63aebd1ecd282f5ab51dd7c3627cb497`. Do not synthesize a
   version marker or matching file subset and call it a historical installation.
   Drive public candidate preview and explicit confirmed upgrade for both.
3. Preview leaves the project unchanged. Apply backs up only recognized/adopted
   changes into private ignored inert recovery, removes the classified obsolete
   implementation, selects the release-matched Go runtime and preserves application
   files, customization, native policy and consumed budget/event/checkpoint history.
   Exercise preserved customization and a required-asset conflict; do not relax
   ownership to make a migration pass. No models run and Git history/index are not
   silently rewritten.
4. Explicit rollback uses the real retained recovery and freshly qualified source
   data, preserves subsequent user data/history and checks the restored predecessor.
   Restored Bash health may require its established baseline tools; that is distinct
   from the Go installation's compiler/interpreter-free runtime contract. Do not
   execute inert saved backup scripts as a verification shortcut.
5. Witness a real durable pending/journal boundary in a running upgrade, stop and
   terminate that owned process, then use the supported public recovery path.
   Ordinary work remains blocked while partial; explicit recovery reaches a usable
   installation and a repeated recovery/application is safe. Use bounded observation
   and retain the witnessed journal evidence; a blind timed kill or modified signed
   image does not establish interrupted recovery.

Reuse existing fixture/state/oracle helpers where their assumptions remain valid.
The live lane adds actual transport/authentication and public migration evidence;
it does not replace the existing detailed hermetic corruption, containment,
customization, process-ownership and phase-by-phase interruption negative controls.

## Native evidence and minimum operating systems

Run natively on Linux amd64/arm64 and macOS amd64/arm64. Keep the existing four-
target builder unchanged. Each qualification artifact contains Ginkgo JSON/JUnit
results plus structured release evidence: schema version, harness commit, official
repository, release ID/tag, full source revision, target, actual OS version and
architecture, exact asset subjects/digests, authentication outcome, scenario
outcomes and workflow/run identity. Record genuine observations, not inferred
success or the runner label alone. Separate live published evidence from hermetic
source/package checks. Preserve useful failure diagnostics without secrets.

specs/001-go-runtime-conversion.md:481 approves Ubuntu 24.04+ and macOS 14+ on both
architectures. A macOS 15 Intel hosted pass does not prove macOS 14 Intel support.
Record that minimum-OS gap explicitly and retain the release gate until matching
native evidence is supplied. GitHub's scheduled macOS 14 retirement likewise
cannot silently raise the approved floor or authorize paid/self-hosted runner
provisioning. The lane may record newer-OS passes honestly while minimum-OS
qualification remains open.

## Independent acceptance and rollout

The independent test author owns behavioral tests; the implementer owns workflow
wiring. Observe a meaningful RED for missing qualification/trust-flow behavior
before production edits, then GREEN and targeted mutation/refusal controls under
specs/001-go-runtime-conversion.md:287. Validate executable flow as well as the
workflow graph; matching strings or comments alone cannot establish trust order,
permission isolation, scenario execution or report publication. Run the review-
diamond verifier before findings count and retain protected-path human review.

The implementation PR may establish a reusable lane and hermetic contract proofs.
Only a completed actual published-release run can close its live target evidence;
only the approved minimum-OS matrix can close the full platform gate. Keep lane
implementation, published RC qualification, human merge, adopter pilot, default
cutover/audit and stable publication as separate observable states. No completion
percentage is inferred merely from adding a workflow.

## Revised platform approval (2026-10-28)

Decision 118 and Q2 now approve Ubuntu 24.04+ on amd64/arm64, macOS 15+ on Intel
and macOS 14+ on Apple Silicon. The earlier minimum-OS discussion above records
the policy under which RC1-RC3 ran. Their reports remain unchanged. Future live
qualification uses the revised architecture-specific floors and still requires
actual execution at each minimum family; a policy revision does not qualify a
new binary or establish the mandatory pilot's operator acceptance.

## Authoritative workflow references

Fetched 2026-10-19 UTC before implementation:

- [GitHub workflow events](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows):
  `release.published` covers stable releases and prereleases; manual workflow
  availability requires the file on the default branch. Qualification follows
  successful asset upload rather than independently racing the release event.
- [GitHub reusable workflow reference](https://docs.github.com/en/actions/reference/workflows-and-actions/reusing-workflow-configurations):
  reusable workflow token permissions cannot exceed the caller's permissions.
- [GitHub CLI attestation verification](https://cli.github.com/manual/gh_attestation_verify):
  source digest/ref, signer workflow, bundle, trusted-root and hosted-runner policy
  operands support the existing exact-release authentication boundary.
- [GitHub macOS 14 retirement notice](https://github.blog/changelog/2026-10-01-github-actions-macos-14-runner-image-retirement/):
  the hosted image retires on 2026-11-02, with prior brownouts. Runner availability
  is not evidence changing the approved minimum operating-system contract.


## Review refinement: refuse incomplete releases before native scheduling

On 2026-10-20 UTC, PR #133 review prompted an early presence check in the trusted
guard: require all twelve expected native bootstrap/runtime/installation subjects
and the shared attestation bundle before publishing outputs or scheduling native
jobs. File presence is not authentication; each native evaluator and the public
installer still independently authenticate their exact downloaded subjects.
Keep independent runs uncancelled by the workflow's default scheduling rather
than a unique-per-run concurrency group that adds no exclusion. Preserve current
finite command deadlines until actual execution supplies evidence for changing
them. Source contract counts remain dated observations, not live release proof.


The next PR #133 review identified a narrow refusal-oracle distinction: the
interrupted installation's ordinary command must return the existing policy
refusal status 2 and its admission diagnostic. A signal exit (-1) is not evidence
of that refusal. Strengthen only the live assertion; do not normalize subprocess
statuses or change the runtime's established policy/error behavior.

## Source-CI review refinement: schedule whole handoff groups by coarse cost

On 2026-10-20 UTC, [PR #133 source-CI run 39364383333](https://github.com/anoop2811/software-factory-template/actions/runs/39364383333/job/118437939519)
reported `Suite Timeout Elapsed` although all 44 selected handoff `It` criteria
passed. The retained Ginkgo report records four indivisible `Ordered` groups:
legacy/init (21 criteria, 674.509 seconds), retention safeguards (10, 785.893
seconds), retention (8, 647.181 seconds), and Go recovery (5, 465.827 seconds).
Both longest groups ran on the same worker. These are measurements from that
failed run, not estimates of the corrected run. The primary worker's 346.889-second
suite-cleanup report includes waiting for its peer; it does not establish that
deleting fixtures took that long.

Use the already pinned Ginkgo v2.32.1 `SpecPriority` decorator for three coarse
scheduling buckets: legacy/init and retention safeguards use priority 2, retention
uses priority 1, and Go recovery retains default priority 0. Start the two expensive
groups first and offer retention before Go recovery as workers become available.
Keep random ordering within an equal-priority bucket; do not prescribe a total
order or a particular worker assignment. `--randomize-all` is not a substitute:
an `Ordered` container remains a single execution group.

Preserve all 44 full criterion identities and assertions, the four `Ordered`
groups, their `ContinueOnFailure` behavior, group-owned `BeforeAll` fixtures and
cleanup lifetimes, independently built predecessor binaries, and genuine
subprocess/interruption/recovery proofs. Preserve adaptive worker selection, the
existing two-worker handoff ceiling, race instrumentation, the 25-minute suite
and 30-minute job limits, and every source-quality gate. Do not alter Makefile
dispatch, runtime/release protocols, dependencies or published-release selection.

Before changing the three decorators, qualify an independent scheduling oracle:
extract the real selected criteria's priority metadata, retain the exact 44-name
inventory, and exercise four small `Ordered` groups with those priorities under
the actual two-worker Ginkgo scheduler. An independent negative control with
priorities removed must fail that oracle. Synthetic scheduling and dry-run
metadata do not establish a performance improvement. Only a subsequent actual
hosted handoff run can qualify the scheduling correction; no live published-release
or minimum-OS evidence is inferred.

Authoritative semantics fetched 2026-10-20 UTC:
[Ginkgo prioritization](https://onsi.github.io/ginkgo/#prioritizing-specs),
[pinned ordering implementation](https://github.com/onsi/ginkgo/blob/v2.32.1/internal/ordering.go)
and [suite cleanup](https://github.com/onsi/ginkgo/blob/v2.32.1/internal/suite.go).
Higher priorities are scheduled first even without `--randomize-all`; ordered
groups are preserved and equal-priority groups remain shuffled. Suite cleanup
waits for nonprimary processes before the primary cleanup body.
