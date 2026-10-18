# ADR 0101: authenticated release handoff into the installed Go factory

- Status: user-authorized implementation contract; independent RED precedes behavior
- Date: 2026-10-18 UTC
- Decision: 100

## Purpose and scope

After merged PR #126, deliver the normal installation and upgrade handoff to the
packaged Go factory. A user explicitly selects one official candidate release,
then uses the ordinary installed `factory` command and existing public adapters.
They do not build a binary or supply individual archive, architecture, source SHA
and attestation operands. Factory behavior requires no Go or Python executable.

This refines specs/001-go-runtime-conversion.md:91,
specs/001-go-runtime-conversion.md:154, specs/001-go-runtime-conversion.md:176,
specs/001-go-runtime-conversion.md:187, specs/001-go-runtime-conversion.md:235,
specs/001-go-runtime-conversion.md:242 and
specs/001-go-runtime-conversion.md:250. Reuse ADR 0097's complete installation
image, ADR 0098's installation transaction and ADR 0099's native orchestration.
Do not introduce another installation engine or a regular-user packaging CLI.

The candidate selector is explicit while the mandatory adopter pilot and final
release qualification remain incomplete. Existing defaults and source/ref flows
retain their contracts. This delivery does not declare the Go replacement release
ready or make Go the global bootstrap default. A successful candidate installation
must nevertheless select its installed Go runtime for subsequent ordinary commands;
placing an unused binary beside the Bash dispatcher does not complete this slice.

## Public selection and consent

Add the literal selector `--go-candidate TAG` to these existing entry points:

- `install.sh init --go-candidate TAG`, retaining supported initializer arguments.
- `install.sh upgrade --go-candidate TAG`, targeting the caller's repository.
- Compiled Go `factory init [TARGET] --go-candidate TAG`.
- Compiled Go `factory upgrade --go-candidate TAG`, including an explicit candidate
  preview with `--dry-run` and optional `--json`.

Accept the corresponding attached `--go-candidate=TAG` form. Reserve this marker
before legacy dispatch, including malformed, empty and repeated forms. Parse
literal arguments; never re-split user values or interpolate them into shell code.
Reject conflicting source/ref/local selectors rather than choosing one silently.
A bare installer remains fetch-only and never executes a fetched binary or changes
the project. A candidate selector without an explicit supported operation is not
consent to installation. Existing invocations without the selector retain their
current defaults, behavior and error paths.

TAG identifies one explicit official release and resolves to one full committed
source revision. Reject moving aliases such as main, head, current and latest,
unsupported host platforms, absent releases and incomplete asset sets. Do not
substitute another tag, architecture, repository, local build, PATH binary or Bash
implementation. A literal tag is still revalidated against its resolved revision;
a moved tag must not silently change the candidate approved for application.

Candidate acquisition is an explicit network operation and can create a private
acquisition cache. Its preview reports that distinction and does not modify the
project, installation state or recovery sets, launch checks or call a model.
The existing local `upgrade --dry-run --source PATH` remains entirely read-only,
with no download or cache write, under specs/001-go-runtime-conversion.md:154.
Machine-readable stdout remains JSON only; progress and diagnostics use stderr.

Preserve reviewable proposal consent and the maintained-quiescence requirement
from ADR 0098. The public handoff removes low-level transport operands, not the
operator's decision to adopt the listed assets. Apply must bind a fresh complete
proposal and explicit confirmation; configuration, environment, a saved report
or a free lock cannot supply that consent. Keep the detailed `--installation`
interface available for explicit offline images, recovery and advanced operation.

## Release assets and the first trust boundary

Retain the qualified runtime-only V1 assets. Publish the complete installation
image for each supported target as
`factory-installation-TAG-TARGET.tar.gz`, where TARGET is the fixed host asset
spelling linux-amd64, linux-arm64, darwin-amd64 or darwin-arm64. Publish a raw
bootstrap executable named `factory-bootstrap-TAG-TARGET` for the same committed
source revision and platform. These are release assets, not another user-facing
command. Build and qualify them with the existing native matrix and package code.

The raw executable solves the first-install trust dependency: the downloaded
candidate cannot authenticate itself before it is safe to execute. The trusted
bootstrap shell therefore uses the independently installed GitHub CLI verifier to
authenticate the raw bytes before chmod or execution. It never extracts an archive
before the authenticated Go program can apply the existing strict image rules.
Do not replace closed-world staging with a shell tar extractor or install a Go
compiler, Python interpreter or new signing client.

The release workflow attests both installation archives and raw bootstrap assets,
using the same qualified repository, source revision, tag and hosted signing
workflow policy as the current attested release. Preserve runtime-only consumers
and their asset names. Signing/upload privileges remain unavailable to pull
requests; source qualification in a PR is not official release authentication.

Acquisition fixes the official repository to
`anoop2811/software-factory-template` and the signer workflow to its
`.github/workflows/runtime-release.yml`. Bind the full committed source digest,
`refs/tags/TAG`, expected host-specific asset name and exact downloaded digest.
Deny self-hosted signer runners as in the existing staging policy. Checking only a
checksum, an archive name supplied by a server, or a matching release title is
insufficient.

In particular, a correctly signed different-architecture asset renamed locally
must fail. Inspect verified attestation subjects and require the expected
host-specific subject name and digest, in addition to the repository/workflow/tag
and source policy. Do not accept an unverified metadata subject or infer target
identity merely because authentication of some other subject succeeded.

Obtain trusted roots independently through the trusted verifier, not from the
candidate archive or a release-provided trust-root file. Snapshot roots and the
attestation bundle in the private acquisition directory. Use the same independently
established root policy for the raw executable and the installation archive.
Missing verifier, failed root acquisition, unavailable release, bad subject,
malformed output or failed authentication stops before candidate execution and
leaves the active installation unchanged. Do not run a version probe to decide
whether unauthenticated bytes are safe.

## Acquisition custody and bounded execution

Use an exclusively created owner-only acquisition directory and private files.
Reject unsafe cache roots or links and never reuse an imported path as authenticated
authority. Treat release metadata, downloaded bytes, verifier output and all
subprocess diagnostics as bounded input. Retain cancellation, finite timeouts,
checked stream/child closure and primary error causes; no unbounded retry or
fallback loop belongs to release acquisition.

The trusted bootstrap executable is the only downloaded file made executable.
Installation image controls and source payload stay inert. Reuse the strict
staging archive grammar, count/byte bounds, identity validation and confined
filesystem publication. The raw executable's attestation does not independently
authenticate another archive: qualify the exact complete installation archive
before its candidate data or executable assets are used.

A prior verified cache entry, staging result or successful preview does not grant
future mutation authority. Recheck custody and selected identities when preparing
the actual installation transaction. A replaced downloaded file, changed target
path or moved release identity must fail safely. Failed acquisition may retain
explicitly identified private diagnostic material, but must not advertise it as
an active or successfully installed release.

Use the existing native process supervisor and output bounds for Go subprocesses;
do not add a parallel process-management abstraction. GitHub CLI and existing
bootstrap shell/platform tools remain explicit prerequisites. Ordinary installed
commands must not download, update or compile in response to a missing or corrupt
runtime, and must never dispatch from acquisition or backup directories.

## Fresh installation and subsequent upgrade

Fresh candidate init must install the reviewed Go role selection, required data,
native adapters and an authenticated release-matched runtime, rather than copying
the entire committed source image into the adopter. Preserve supported prompts,
initializer inputs, language packs and non-Go application choices. Keep user
content, policy settings and generated Codex, Claude Code and OpenCode contracts.

Before success, the destination must have the checked installed descriptor,
runtime identity and required effective ignore rules. Its ordinary `./factory`
launcher, direct command paths and sourceable bridges must reach the selected Go
implementation. Required installed deterministic health/proof checks must run
without a compiler, interpreter, native agent client or model credential. A failed
check must not publish a successful installed version. Do not claim automatic
rollback for an initializer failure unless that restoration is actually supported
and independently qualified.

For legacy upgrade, infer a profile and deterministic initializer inputs only when
the observed installation uniquely matches the approved v0.1.6 or immutable Bash
baseline contracts. Inference identifies the proposal; it does not prove historical
ownership or authorize adoption. Present exact changes and preserved conflicts,
then reuse the current installation engine's fresh confirmation, exclusion,
backup-before-mutation, pending barrier, candidate checks and activation metadata.
Unknown, ambiguous, unsafe or customized required assets stay preserved conflicts.

Subsequent candidate Go-to-Go upgrades must use the same installation transaction
and checked predecessor identity. Installed metadata is descriptive, not authority
to delete arbitrary matching paths. Requalify the predecessor image, immutable
release identity and actual installed asset evidence required by the engine;
preserve later edits and state. Neither fresh init nor upgrade can commit, push or
merge the adopter's repository, reset budgets/history, refresh stale checkpoints or
launch a model as an installation test.

Existing rollback and interrupted recovery retain their independently qualified
before/after images, explicit direction/consent and owned-work requirements. A
higher-level selector must not bypass them or execute saved scripts. Preserve
concrete recovery instructions and refuse ordinary work through a partial
installation. Any needed profile or initializer refinement belongs in an append
below before the corresponding production behavior changes.

## Cleanup boundary and remaining release gates

The handoff uses existing checked backups for owned/adopted replacements and
retirements under private ignored `.factory/backups/MIGRATION_ID/`. It does not
leave superseded implementation files active or overwrite customizations to make
a candidate work. Retained public shell paths are the declared thin adapters and
intentional host validation boundaries, with one canonical implementation.

Automatic later-release retention, four-platform authenticated release rehearsal,
the mandatory real adopter pilot with agreed exit criteria, final dependency and
reference audit, and global default cutover remain separately evidenced release
gates. This slice must not silently claim retention completion, a released artifact,
a platform pass or a successful pilot. Preserve the fixed source-plan denominator;
report implemented, qualified, merged and released states separately.

## Independent acceptance and review

Use Cobra and independent outside-in Ginkgo/Gomega TDD under
specs/001-go-runtime-conversion.md:291 and
specs/001-go-runtime-conversion.md:298. The evaluator owns behavioral tests;
the implementer owns production code. First observe a failing public candidate
request, then authentication/activation failures through real compiled boundaries,
then GREEN and focused regressions. No Python fixture or new module is required.

Acceptance must pair healthy controls with each refusal and cover:

- Bare bootstrap and existing source/ref behavior remain compatible.
- No Go/Python executable is available to the invoking factory process.
- Each host target selects its exact asset; unsupported hosts refuse.
- Invalid/moved release identities, failed verification, renamed signed subjects,
  malformed metadata, replaced cache entries and missing assets execute no candidate.
- Authenticated bytes precede the first candidate invocation; the raw bootstrap
  cannot supply its own trust or silently run from PATH.
- Fresh init selects a real installed Go runtime; ordinary help, doctor, checks
  and existing direct/sourceable adapters use the installed selection.
- v0.1.6, Bash-baseline and Go-to-Go upgrades reuse real proposal/application
  behavior, preserving unknown/customized assets, runtime state and all three
  harness contracts.
- Confirmation changes, maintenance exclusion, failed proof, interrupted apply and
  subsequent recovery do not turn a partial installation into success.
- Candidate preview changes only its explicit acquisition cache, whereas the
  existing local preview makes no filesystem or network changes.
- Download, verifier and candidate subprocess failures retain bounded cleanup and
  truthful output/status; no model call or automatic Git operation occurs.

Mocked release transport/verifier fixtures establish ordering, custody and refusal
behavior only. Real publisher authentication requires a real attested release and
is recorded separately. Run the review diamond's independent correctness,
security and coverage lenses, verify findings before deterministic deduplication,
and obtain a completed exact-head model review before claiming that evidence.


## Fresh-init overlay and activation refinement

Reuse the native initializer's existing argument parsing, collection, pack planning
and confined publication. Pass a typed, qualified Go-selection overlay into that
plan before any destination write. The overlay comes from freshly authenticated
and inspected candidate bytes; a public path, environment switch or saved staging
receipt cannot construct equivalent authority.

Do not initialize a Bash installation and then run an upgrade over it. Before
publication, replace planned legacy implementations with the reviewed Go roles and
omit retired implementations outright. Keep ordinary initializer preservation,
prompts, literal inputs, pack selection and generated harness contracts. Reject
this fresh-install mode when an existing factory installation or private Go
installation metadata is present: it must not become an unjournaled overwrite or
an alternate route around the whole-installation upgrade transaction.

The selected runtime is one separately declared binary role, with the existing
256 MiB binary ceiling and bounded streaming copy/hash/publication. The ordinary
initializer's 16 MiB per-file limit stays unchanged; no general filename or caller
option grants the larger allowance. Check the exact runtime digest, byte count,
mode and fixed destination, and preserve confinement, no-follow custody and checked
closure. Controls, canonical assets and other initializer files retain their
ordinary bounds and inert modes.

Establish effective private-runtime ignore rules before writing private assets.
Keep runtime identity, required assets, native adapters and the descriptor coherent
with the selected release. Run mandatory installed-layout and native deterministic
proof validation before final activation metadata advertises a usable installation.
A controlled validation capability may inspect an incomplete owner-held candidate;
ordinary commands cannot bypass pending admission to make the checks pass.

An initialization failure is reported as incomplete with a concrete next action.
It must not publish a successful activation marker, silently fall back to Bash or
claim complete init atomicity/automatic rollback. Existing noncandidate initializer
behavior remains unchanged. Independently cover prewrite rejection, binary bounds,
retired-path absence, final Go dispatch and failed activation proof.

## Qualified predecessor recipes and legacy inference refinement

Add Go predecessor recognition to the existing baseline abstraction, so forward
upgrade and explicit restoration share the same checked original-image evidence.
For `go-hybrid-v1`, acquire and freshly authenticate the exact predecessor V2 image
as well as the target image. Bind installed descriptor, runtime bytes and manifest,
source controls, version marker and observed selected assets to that predecessor's
literal version, full source revision, target and archive identity. The descriptor
or `.factory-version` marker alone never establishes management authority.

The first supported Go recipe is deliberately closed and versioned. Require the
authenticated predecessor's committed assets for
`internal/installation/generate.go`, `internal/installation/roles.go` and
`internal/installation/orchestration.go` to equal the qualified target recipe.
This equality check must also close over output-affecting collaborators, including
project substitution and descriptor schema/mode interpretation; merely comparing
those three files while using a changed helper is not a closed recipe proof.
An implementation may fingerprint the additional source/schema files or preserve
an independently qualified explicit recipe version. Missing, different or unknown
recipe semantics refuse until that version has its own support and acceptance.

Regenerate expected predecessor selection from the freshly qualified predecessor
image and proven supported recipe, using the bound project inputs and predecessor
runtime identity. Compare actual paths, complete types/modes, bytes and descriptor
membership against that selection. Never execute predecessor code, use current
generation unconditionally, or trust a locally edited manifest as deletion
permission. The same recipe rule applies to generating rollback targets; supporting
a new candidate recipe does not retroactively establish an older recipe's output.

For legacy profiles, infer initializer inputs only from literal existing
configuration and independently matching approved baseline rows. Preserve existing
flat-configuration semantics. Require one unambiguous profile and compatible input
set, then let the ordinary complete proposal classify all actual assets. A version
marker may be recognized only in conjunction with the full profile evidence; a
marker name or claimed commit is insufficient. Unknown substitutions, ambiguous
profiles, unsafe files and customized required entries remain preserved conflicts.

Keep the simple high-level apply operands `--confirm DIGEST --quiescent`; the
handoff supplies qualified transport identities internally. Confirmation remains
an operator-supplied current proposal digest, never an implicit yes, environment
value or automatic adoption receipt. Interactive presentation can be refined
separately without weakening this noninteractive contract.

## Candidate proposal identity across private acquisition directories

A preview and its subsequent apply independently acquire release assets into fresh
private directories. Those random transport paths are not the release identity and
must not make an otherwise unchanged confirmation unusable after cache cleanup.
Add an internal candidate-request mode with a domain-separated proposal binding
based on the literal version, full source revision, target and freshly observed
complete archive SHA-256, for both source and qualified Go predecessor images.
Bind the official authentication policy and supported predecessor recipe as well.
Do not expose this mode as a public trusted boolean or infer it from an arbitrary
source directory; only the candidate handoff constructs the qualified request.

Normalize only transient transport locations. Preserve the complete observed root,
profile, project inputs, asset actions, types/modes/digests, checked absences,
parent identities, runtime state, rollback/recovery direction and other existing
consent material. A byte change, moved tag, changed installed path, changed input
or different predecessor must invalidate confirmation. Existing low-level
installation requests retain their current exact-path proposal binding.

`Begin` still freshly authenticates exact source and predecessor archive snapshots
under the installation exclusion and recomputes the complete proposal before
accepting confirmation. An observed archive digest is not authentication; preview
or imported metadata cannot suppress that check. New cache locations containing
the same authenticated release may preserve the digest, while replaced bytes or
failed verification stop before candidate execution or mutation.

Independent criteria must observe preview in one private directory, removal of
that acquisition directory and successful confirmed apply after fresh acquisition
of the same authenticated release. Pair it with changed-archive, changed-predecessor,
changed-project and failed-authentication controls. Preserve the existing strict
local preview's no-write/no-download contract and exact-path binding.


## Fresh destinations outside an existing Git worktree

Preserve the initializer's ability to create a fresh target outside Git. Candidate
init must not run `git init`, create Git metadata in the adopter, or misreport an
ordinary nonrepository as an unsafe existing installation merely because
`git ls-files` cannot run there. Missing Git, invalid redirected Git environment or
an operational discovery failure is not automatically proof of a nonrepository.

For a target inside a Git worktree, check the actual effective ignore decisions
and tracked status for every declared private installation root, with literal paths
and the existing confined repository selection. Reject tracked private paths or
rules that expose the runtime, canonical assets, installed controls, transactions
or backups. Do not alter the Git index or erase user ignore rules.

For a fresh target outside Git, validate the exact planned root `.gitignore` in an
exclusively created private temporary Git fixture using real `git check-ignore`
and representative sentinel paths for each private root. This fixture is the only
repository initialized by the check. Keep verifier Git configuration/environment
isolated from user redirection and bound its child execution. The planned private
layout has no preexisting `.factory` tree under the fresh-mode refusal above;
if relevant nested ignore sources are ever allowed, reproduce their effective
hierarchy in the fixture or refuse rather than testing a simpler set of rules.

Prove the planned rules before publishing private installation assets, then publish
and read back the preserved/augmented `.gitignore` before those assets. At final
activation, revalidate the actual installed ignore bytes and effective decisions,
using the actual worktree when present and the isolated fixture otherwise. An
ignore proof after secret/private bytes were already written is insufficient.
No fixture result claims that the target already has a Git index or constrains
future operator edits; it establishes the installed rules' behavior if the project
is subsequently put under Git. Clean up only the owned fixture and retain primary
errors and bounded process cleanup on failure.

Independent acceptance includes successful fresh init outside Git with no Go or
Python executable, unchanged absence of target Git metadata, and a healthy
subsequent ordinary Go command. Pair actual-worktree tracked/unignored refusals
and changed-since-plan ignore rules with the healthy cases. The outside-Git fixture
is proof of ignore-rule behavior, not permission to bypass fresh-mode custody or
mandatory installed activation checks.


## Bootstrap verifier process ownership refinement

A trusted GitHub CLI can still create descendants. Observing or reaping only its
direct PID does not establish that its owned work has exited, including after the
leader returns or cancellation interrupts verification. The bootstrap must stop
and account for its owned verifier group before deleting the acquisition cache or
claiming complete cancellation. Authentication success does not excuse uncertain
process ownership.

Retain the POSIX `sh` installer entry point and unchanged bare/legacy behavior.
The explicit candidate path may use the existing `/bin/bash` prerequisite shared
by supported Linux/macOS public adapters. Use a fixed literal supervisory snippet
with Bash job control to create a separately owned process group for each verifier
invocation. Pass the trusted executable and all operands as literal positional
arguments. Do not use eval, interpolate user values into shell code, introduce a
Go/Python compiler/interpreter dependency, or assume an unavailable external
`setsid` command. Suppress shell-startup environment hooks that could execute user
code before the fixed supervisor runs.

On cancellation, timeout, verifier failure and normal leader exit, traps account
for any remaining owned descendants. Apply bounded group termination, finite
escalation and direct-child reaping; retain the original failure and any uncertain
closure cause. A missing direct leader or closed output pipe alone is insufficient.
Signal only the established owned group, never the installer's caller group.
Do not introduce retries or permit descendants to keep running against a deleted
private cache. If owned group closure cannot be established, return an error and
retain the private acquisition directory with a concrete diagnostic instead of
advertising clean completion or removing potential live inputs.

Native installation-archive authentication must use the existing bounded native
process supervisor rather than its own direct-PID transport. Preserve its owned
group, stream limits, cancellation/escalation and typed uncertainty semantics.
When workspace cleanup would race unconfirmed owned work, retain that private
workspace and the primary failure. No candidate or model is launched to resolve
such uncertainty.

Independent regressions use real verifier subprocesses that fork descendants,
including a leader that exits while descendants retain streams or run after pipe
closure. Observe the prior ownership defect, then prove bounded cancellation and
group exit or truthful retained uncertainty. Pair with an ordinary successful
verifier, preserve literal argument tests, and check that no outside group is
signaled and no retained live-input directory is deleted.

## Installed initializer selection

The independently reviewed installed context must not initialize another project
from the inert legacy scaffold without a candidate selector. That would publish
a Bash selection and invoke its historical interpreter-dependent selftest.
Installed `factory init TARGET` therefore refuses before prompts or project writes
and explains `factory init TARGET --go-candidate TAG`. Its help remains usable.
The existing source/legacy initializer without a selector keeps its contract.
This bounded handoff does not introduce an implicit offline initializer or make
ordinary commands acquire a release to repair a missing or corrupt runtime.

## Reviewed transport refinements

The public acquisition owner applies the expected attestation subject name and
observed digest policy independently to both the raw bootstrap and the installation
archive before staging. Generic explicit/offline staging retains its existing
contract. Closed-world target validation already rejects a renamed wrong-target
image; the additional query enforces the literal subject policy for a same-target
image signed under an unexpected name too.

When the installer is itself read from a pipe, the candidate must not receive
unread installer source as initializer answers, including with redirected stdout
and no terminal. Define the legacy installer body before dispatch and place the
candidate selection and legacy invocation at the end of the script, so the shell
has parsed the complete source before starting a candidate. Preserve explicitly
piped answers when the installer is invoked from a file and retain the native
initializer's existing terminal handling. Do not guess from the script name or
discard a legitimate answer stream. Pair a piped-source nonterminal regression
with the existing file-invocation input control.
