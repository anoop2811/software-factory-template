# ADR 0144: bootstrap CI preparation through the authenticated installer

Status: implemented; independent RED/GREEN passed; hosted qualification pending.
Decision: 160. Continues R4 after merged PR #174.

## Bounded delivery

PR #174 merged as `b5d5d4ca6a25fa4f9bb860ec4e49584045c627f1` on
2026-11-08, delivering native disposable-checkout preparation and the bootstrap
watchdog correction. A new CI machine still needs a trusted executable before
it can use that operation. Extend the existing authenticated installer with one
explicit CI route; reuse the native preparation controller unchanged.

This slice does not generate workflows, replace existing workflow files, choose
a release or origin implicitly, change the stable default, or declare release
readiness. Those integration and rehearsal obligations remain separate R4 work.
Do not modify frozen recipes, application setup or the installed runtime layout.

## Public entrypoint and parsing

Run a separately trusted installer from the root of a fresh disposable checkout:

```sh
sh /path/to/reviewed/install.sh ci prepare \
  --go-candidate TAG --origin-target darwin/arm64 --ephemeral
```

Reserve a literal leading `ci` before the existing candidate, stable and legacy
installer routing. Every request on this route is owned by the CI parser;
unknown operations and malformed requests must never fall through to a source
cache, fetch-only mode, initializer or upgrader.

Match the native CI argument contract. Require one explicit exact tag, one origin
from the four supported Darwin/Linux amd64/arm64 targets, and `--ephemeral`.
Accept both separate and equals-form value options. Reject unknown or duplicate
options, extra positional operands, `--`, missing/empty values, and values attached
to Boolean flags. Normalize `-h` and `--help` for duplicate detection.
`ci --help` and `ci -h` are valid only as sole operation arguments. Within
`ci prepare`, help skips required-field presence but still validates every supplied
field. Help succeeds without tools, acquisition or project/cache changes;
invalid requests exit two with a CI-specific diagnostic before acquisition.

Match `releasecmd.exactRelease`: a lowercase `v`, all three canonical numeric
components, optional prerelease and build metadata, ASCII characters and the
existing 128-byte tag limit. Numeric core/prerelease identifiers cannot contain
leading zeroes; build identifiers may. Do not impose the ordinary stable route's
minimum-release restriction or disallow explicit prereleases. Do not convert
version components to machine integers. Confine any locale override to validation
so the selected owner's environment is preserved. This pre-execution validation
is necessary before a native parser exists; the selected native owner repeats it.

`FACTORY_REF`, `FACTORY_HOME` and `FACTORY_REPO` do not select a CI release,
cache or repository. Only the explicit arguments select the operation; official
release acquisition remains fixed. The operation never infers consent from `CI`.
Clear the inherited private `FACTORY_BRIDGE_PROTOCOL` at this public CI boundary
so it cannot divert the authenticated executable into sourceable bridge dispatch.
Do not change these variables' behavior for existing init/upgrade/fetch routes.

## Authentication and process ownership

After validation, call the existing `packaged_install` with candidate policy and
the original quoted argv. Keep the host selected by the running platform; the
origin argument is metadata for the native owner, never an executable selection.
Use the existing fixed-repository release metadata, exact source revision,
subject-name/digest attestation and trusted-root checks before the raw host
executable's first execution. Do not add a second acquisition implementation.

The shell's private release directory remains a borrowed input hint, not native
authority. The selected native owner imports it into fresh confined custody,
re-resolves the release/revision and reauthenticates the raw executable and image.
It authenticates a foreign origin separately without executing that runtime.
The native controller continues to own clean-checkout admission, exact asset
matching, leases, publication, final validation and pending-state refusal.
An older selected owner without this command fails with its own status; never
substitute a newer owner or fall back to legacy/source execution.

Preserve cwd, literal argument boundaries, stdin, output streams and native exit
status. Reuse the existing guarded watchdog/child lifecycle, cache cleanup and
unresolved-ownership refusal. Do not change timeouts, add process-name cleanup,
or bypass the caller-descriptor protection established by Decision 159.
No Go compiler, Python, model call or additional dependency is needed by the
installed path. Existing shell, Git and attestation-capable GitHub CLI prerequisites
remain explicit; project checks retain their own tool requirements.

The installer itself must come from a separately trusted, reviewed or pinned
source. Release attestation authenticates downloaded runtime assets; it cannot
establish trust in an arbitrary installer from an application's PR checkout.
Documentation must distinguish this executable bootstrap from the pending
workflow-generation/integration feature.

## Independent qualification

Use a separate spec-writer for outside-in Ginkgo/Gomega tests and observe public
RED before production implementation. Reuse the existing shell authentication
fixtures and actual packaged CI project fixture. Keep the native CI preparation
matrix and existing bootstrap regressions intact.

Qualify strict surface/refusal and no-acquisition help, accepted argv/stdio/status
forwarding, authentication failure before raw-owner execution, and a real
packaged-init/commit/clone/installer-prepare/installed-check journey. The latter
must keep Go/Python unavailable, prove exact public/private inventories and
preserved application/configuration/Git state, and run both a passing check and
an application-command failure. Poison inherited selectors/cache/bridge settings
without granting them authority; prove temporary acquisition cleanup.

Add one bounded composed cancellation control through the installer and selected
native CI owner, using existing owned-verifier handshake helpers and the shared
real image. Observe readiness before signalling, prove owned process closure and
unrelated-process preservation, and retain uncertain evidence rather than guessing
ownership. Do not duplicate the native cross-platform or publication-fault matrix.

Register the new cheap surface and shared-image project groups explicitly in the
existing release-handoff partition/scheduler. Discover the exact census; preserve
the original real scheduler negative control and resource limits. Run all builds,
tests and hooks in complete private checkouts, keeping the operator's ignored
binary and unrelated edits intact. Record independent review and final-head CI
separately from local RED/GREEN. No workflow or default-release readiness claim
follows from this bootstrap alone.

## Sources

Inspected the native parser and `exactRelease` in this repository, and the
[Semantic Versioning 2.0.0 rules and grammar](https://semver.org/spec/v2.0.0.html)
on 2026-11-08. No tool/library version pin or external API changes are required.

## Qualification trace refinement

The composed successful path verifies four subjects: the shell's raw host,
the native owner's raw host and installation archive, and the staging layer's
confined archive snapshot. Keep the exact observed trace and account for that
existing fourth verification; do not remove it or weaken the authentication
assertion to accommodate an incomplete test expectation.
