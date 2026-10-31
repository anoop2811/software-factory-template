# ADR 0124: authenticate ordinary Go installer requests before execution

Status: accepted bounded R2 bootstrap contract; independent public RED precedes production

Date: 2026-10-31 UTC

## Complete function and release boundary

After human merge of R1 as `21f2913`, extend `install.sh` so an ordinary
`init` or `upgrade` request selecting an exact stable Go release at or after
`v0.2.0` reaches the existing authenticated native initializer or upgrader.
Reuse the explicit candidate acquisition, attestation and cleanup boundary;
share its implementation without converting stable requests into candidate policy.

This first R2 PR changes the installer only. Source `factory`, direct init and
upgrade scripts, remaining direct command/helper adapters and their legacy bodies
remain separate functions. R2 stays partial until its later source/direct
entrypoint function is human-merged. Do not copy a developer compiler launcher
into an installed project. See docs/adr/0114-incremental-go-cutover-delivery.md:50.

Keep the public installer literal default `v0.1.6` until final stable activation
R7. Do not publish a release, claim `v0.2.0` exists, introduce a binary cache or
change release gates, supported platforms, roles, budgets, dependencies or limits.
The existing Go default `v0.2.0` is preparation, not publication authority.

Observed 2026-10-31 via `gh release view --repo
anoop2811/software-factory-template --json
tagName,isDraft,isPrerelease,publishedAt,url`: latest stable is `v0.1.6`, draft
false, prerelease false, published `2026-09-06T19:10:47Z`. Release-list readback
contains `v0.2.0-rc.1`, `.2` and `.3` prereleases, with no stable `v0.2.0`.
The official https://cli.github.com/manual/gh_release_view documentation, fetched
2026-10-31, lists the tagName, isDraft and isPrerelease JSON fields used here.
No tool version is changed by this decision.

## Consent and ownership of selectors

The effective selection is one explicit `--ref TAG` or `--ref=TAG`, then
nonempty `FACTORY_REF`, then the unchanged literal default `v0.1.6`. An explicit value
wins over the environment, including when it deliberately chooses a legacy ref.
A missing or empty explicit `--ref` value refuses status 2 before tools.
Only tokens before the first user `--` may select a ref or candidate, request
help or establish selector conflicts. Values after it remain literal native data.

For execution requests (`init` and `upgrade`):

- Exact canonical stable versions below `v0.2.0` retain the legacy installer.
- Non-version branch refs, including `main` and custom branches, retain that
  legacy path in this partial stage; this is not a Go release alias.
- Exact canonical stable versions at or after `v0.2.0` select authenticated Go.
- Version-looking values beginning `v` followed by a decimal digit that are
  shortened, malformed, noncanonical, leading-zero or prerelease labels refuse
  with status 2 before tools, acquisition or target writes, including with help.
  Do not let failed stable validation select the legacy path.
- Repeated or mixed explicit ref selectors refuse before any last-wins legacy
  parsing can conceal a Go request. Candidate/ref and source/local/installation
  selector conflicts retain explicit ownership and refuse before execution.

Stable means the existing native exact-release grammar: complete
`vMAJOR.MINOR.PATCH`, canonical numeric components, no prerelease, valid optional
build metadata retained verbatim, and no tag longer than the existing allowance.
Compare large numeric labels without shell integer overflow. The native owner
revalidates this policy; shell validation is not an authorization replacement.

Explicit `--go-candidate` remains a separate policy, including its existing
prerelease support and authentication/ownership guarantees. Do not change its
repository policy or silently downgrade a stable request into this route.

The new stable Go route accepts only the official repository URL
`https://github.com/anoop2811/software-factory-template` and its equivalent `.git`
spelling. A conflicting `FACTORY_REPO` refuses status 2 before tools rather than
ignoring the operator's fork selection. Existing legacy/custom-repository behavior
and explicit candidate policy remain unchanged.

## Bare fetch remains fetch only

An invocation without `init` or `upgrade` still performs the existing Git-only
fetch into `FACTORY_HOME`, including when a valid Go tag was selected. It neither
executes downloaded code nor modifies the caller's project. It does not acquire
or publish an executable binary cache. Existing ref availability checks and
unpublished-ref refusal remain; no silent fallback to main or another release.

For a fetched Go tag, next-step advice must re-enter this authenticated installer
with explicit `init --ref RESOLVED_TAG`. Do not suggest directly executing a
fetched initializer or sibling binary as though the Git-only fetch authenticated
its runnable artifacts. Legacy-ref advice and default legacy fetch/init remain
compatible in this intermediate function. See specs/001-go-runtime-conversion.md:250.

## Stable authentication and native handoff

Before chmod or the first downloaded executable call, resolve the exact requested
tag to its complete revision, obtain the matching release, and require exact
tag equality, isDraft false and isPrerelease false. Missing/unpublished, draft,
prerelease, mismatched, corrupt or unauthenticated artifacts refuse operationally
without changing the project, executing a sentinel or falling back to scripts.

Reuse the existing raw bootstrap plus complete installation image, published
attestation bundle, independently obtained trusted roots and exact official
repository/workflow/source revision/tag/subject digest verification. Retain all
current per-input bounds, verifier supervision, platform mappings, cleanup and
uncertain-ownership retention. No downloaded version probe precedes verification.
The authenticated Go owner independently validates the complete image and
cached identity under the same stable policy before mutation.

Pass exactly one normalized explicit `--ref RESOLVED_TAG` to the native child,
including an environment-selected tag, before any user terminator. Preserve the
other operands as literal argv, retaining spaces, quotes, newlines and flag-like
data. Do not pass only `--go-candidate`, drop the stable selector, re-split args,
use eval, execute a sibling/PATH factory, or build a compiler fallback.
Native argument/representation restrictions still apply to transported data.

Keep caller cwd, original inherited input/output/terminal behavior and native
exit status. Parse the complete piped installer source before handing input to
a child: source text must never become initializer answers. File-mode typed input
remains input. Keep the shell's existing signal and bounded ownership behavior,
including retained private inputs when cleanup cannot establish ownership.
The shipped native runtime does not require Go or Python; Git, the trusted GitHub
verifier, existing shell/platform tools and configured checks remain prerequisites.
See specs/001-go-runtime-conversion.md:237 and
docs/adr/0101-go-release-handoff.md:80.

## Native safety and backward compatibility

The existing native owners retain all consent and transaction semantics.
Initialization must use the authenticated selected initializer and its ordinary
installed launcher, never the fetched source initializer. A normal upgrade of a
real legacy installation previews read-only, requires its fresh complete digest,
quiescence and project inputs, and preserves project/custom state through apply
and existing recovery/rollback. Do not restore legacy fallback logic in Go.

No source or direct script is changed in this PR. Existing immutable historical
initializer/dispatcher evidence stays classified as historical; current tests
continue to exercise current public owners. Before later source/direct conversion,
audit all current-source script fixtures and freeze their actual pre-conversion
Git provenance rather than deleting or silently retargeting old assertions.

## Independent outside-in qualification

First establish a valid real compiled current native initializer fixture, then
observe the unchanged installer fail to enter that authenticated stable path.
Record the legacy-fetch marker or actual wrong boundary and project preservation;
a missing arbitrary test tool alone is not sufficient semantic RED.

Plan nine bounded current criteria: real selected native initialization and
installed proof without Go/Python; real legacy upgrade preview/fresh consent and
apply through the installer; bare-fetch and legacy-default consent; selector and
help/refusal controls; literal argv and child status; stable release/authentication
failure controls; piped/file input framing; verifier cancellation; and native
handoff cancellation with actual readiness/custody. Reuse existing fixtures and
parameterize input controls without multiplying an unnecessary cross product.
Preserve every existing explicit-candidate bootstrap criterion.

Derive actual registry identities from native discovery after writing tests;
`3186 + 9 ordinary + 1 timer-publication` is a proposal, not execution. Measure runtime
before choosing whether the existing lane has sufficient headroom; introduce a
new heavy partition only if evidence requires it. Preserve existing race checks,
all platform mappings, worker caps, empty/failure/union contracts, reports and
all command/test/job deadlines. No runtime, fixture or CI allowance increases.
Keep local qualification, hosted CI/model review and published release proof
separate. No public stable activation is earned by this PR alone.


## Reviewed refusal boundaries before parser repair

The terminator ownership rule does not require the retained Bash initializer to
implement native literal-operand grammar. For a legacy init/upgrade execution,
any user terminator is an unsupported form and refuses status 2 before Git,
GitHub acquisition or cached-source execution. Do not let a downstream legacy
ref parser reinterpret post-terminator data into a Go release selection. Native
Go and explicit candidate paths retain their supported literal terminator data.

A missing ref operand cannot consume a following option such as `--help` and
then enter legacy Git handling. Refuse that malformed selector before tools.
For Go-family or invalid version-looking selection, malformed literal verbs such
as `init `, ` init`, an empty token before `init`, or a newline-bearing verb are
not bare fetch. Refuse status 2 without trimming or re-splitting them into an
executable request. Genuine bare fetch has no verb or positional data.

Before changing these production boundaries, capture actual outcomes for the
legacy terminator and malformed Go-verb vectors against unchanged code, using
both mock Git records and a cached-source execution sentinel. Collect the
outcomes before assertions so an earlier malformed selector cannot hide the
other paths. Require no Git, no GH, no source initializer and unchanged project
on the corrected refusal; keep the existing nine criterion identities.

## Failure-refusal execution witnesses

A nonzero acquisition result alone cannot prove the absence of a fallback. For
every existing release/authentication refusal scenario, seed an executable
cached-source initializer with its independent invocation witness and preserve
its exact cache snapshot. Require no forbidden Git/compiler/Python/tool witness,
no cached-source invocation and unchanged cache, in addition to the existing
intended GH boundary, raw-execution, early-chmod, project and cleanup guards.

Challenge the same criterion using only a private copied installer that calls
the cached initializer after a wrong-subject denial and then returns nonzero.
The old guards must still hold and the new invocation guard must fail before
claiming this coverage. Restore the actual installer for the focused GREEN; no
production bypass, new criterion, deadline, dependency or candidate policy is
introduced.

## Watchdog cancellation during timer publication

The shared acquisition watchdog owns each timer from its creation through reaping.
A stop signal between starting a sleep and publishing its PID must not leave that
sleep alive. Defer cancellation during that publication window, then stop and
join the recorded timer before the watchdog exits. Reuse one supervised pause
for both the acquisition delay and escalation delay; retain every existing
duration, signal, verifier-group policy and authentication condition.

Independent public-bootstrap scheduling controls observed this ownership race in
both locally available sh and dash: the authenticated child and installer exited
successfully while the recorded timer survived. Both controls closed captured
output normally, so they do not establish the cause of the separate hosted
120-second candidate-test timeout. Do not equate those observations.

Add one bounded criterion that forces the pre-publication signal through a
private scheduling copy of the real installer, records the owned timer and proves
it is absent before test teardown. Preserve literal upgrade argv and unrelated
processes. Keep all existing criterion identities and derive the added registry
and lane counts from actual discovery. Observe its semantic RED before production
repair, then qualify it, the original candidate family and ordinary cancellation
controls without increasing any allowance or adding production test hooks.

The scheduling test must not signal a recorded group again after the body has
joined its caller and observed all owned processes absent. Failure teardown must
retain the fixture when forced termination leaves separate verifier-group
custody uncertain; disappearance of only the outer group does not prove it.
