# ADR 0143: prepare an authenticated runtime in a disposable CI checkout

Status: accepted for implementation; qualification pending.
Decision: 158. Continues R4 after merged PR #173.

## Context and bounded scope

Git carries the public factory files and `.factory-version`, but intentionally
omits private runtime custody. Moreover, installed launchers bind the originating
platform's runtime digest. Copying a Linux binary into a macOS-created checkout
cannot satisfy installed admission. A fresh checkout needs a complete host
selection derived from authenticated images, not a relaxed descriptor check.

Add a native `factory ci prepare` operation. This PR delivers the preparation
boundary and its outside-in qualification. Installer bootstrap integration,
adopter workflow generation and explicit migration of existing workflow wiring
are later R4 tasks. Do not copy source-development CI into adopter projects or
claim that this command alone completes fresh-checkout workflow integration.

## Public contract

Invoke a separately provisioned, trusted native factory executable from the
checkout root:

```sh
factory ci prepare --go-candidate TAG --origin-target darwin/arm64 --ephemeral
```

Require an exact release tag, an explicit origin target from the four supported
Darwin/Linux amd64/arm64 targets, and `--ephemeral`. The host target is the
executing native platform. `--go-candidate` also permits exact stable tags; it
never means latest, a branch, an arbitrary repository, or an unauthenticated
local build. Reject missing, duplicate, conflicting and unknown arguments.
Help performs no acquisition or project mutation. This first boundary does not
guess the origin target, mutate CI YAML, install language tools or run setup.

`--ephemeral` declares that this is a disposable checkout, with no concurrent
editor/build process, whose worktree may be reprojected for this job. `CI=1`
alone is never consent. Do not use this command for a developer's working copy,
upgrade, rollback, or repair of an interrupted installation. On failure after
publication begins, preserve evidence and discard/recreate the job checkout.

## Authentication and ownership

Treat the tracked release marker and exact generated launcher as untrusted
hints. Their tag, source revision and origin runtime hash must agree with the
explicit selector and independently authenticated origin release image. Obtain
both host and origin raw/runtime archives through the existing official release
acquisition and attestation boundary, including subject name/digest, tag, source
revision, signer workflow and trusted roots. Same-target preparation reuses one
acquisition. Cross-target acquisition never executes the foreign binary.

Both images must identify the same exact tag and source revision. Run the
matching authenticated host owner through the existing cooperative handoff when
the caller differs, so a current controller cannot reinterpret a future image's
recipe. A selected release that does not implement this operation fails before
checkout mutation; this does not retrofit the command into historical releases.
Host handoff caches keep their existing reauthentication policy. Origin images
may be fetched independently; this contract does not promise offline preparation.

Project metadata may supply the existing supported substitution inputs, retaining
duplicate-key and input validation. It is not ownership evidence. The legacy
single-protected-path inference restriction remains in ordinary upgrade inference;
CI preserves valid multi-path configuration and recognizes only exact projected
asset bytes. Reuse `installation.Selection` to project origin and host images.
Before publication, compare every public selected asset with the authenticated
origin selection, including bytes and executable mode; require each to be
tracked and clean. Refuse missing, customized, ambiguous, linked or unsafe
factory-owned assets. Preserve application files, project configuration,
unknown files, workflow customizations, Git HEAD/index and ignore rules.

## Publication and recovery

Require a real checkout at its root, with a clean tracked index and worktree.
Ignore untracked application files except when they occupy selected destinations;
never remove them. Require effective private ignore rules using the existing
isolated Git proof. Private installation roots (`bin`, `assets`, `backups` and
`installation-transactions` under `.factory`), `installation.current` and pending
publication state must be absent, including empty roots. Refuse existing custody with
a diagnostic to use a fresh disposable checkout; repeated preparation is not a
repair protocol. Do not reinterpret a predecessor as a legacy installation.

In addition to the shared representative ignore checks, prove every private
destination in the selected host image is untracked and effectively ignored.
Selective exceptions can expose real assets while leaving representative probes
ignored. Derive this closed list from the authenticated selection; do not add
hardcoded sample paths or change frozen image recipes to approximate it.

Acquire the existing exclusive root lease, pin the physical root, and repeat the
release/asset/Git/ignore/custody checks before publication. Reuse initializer
confined observation and atomic publication primitives, runtime copying, and the
standard pending barrier. Publish the complete host selection, descriptor and
public adapters, then validate the installed layout before clearing the barrier.
No relaxed `--installed` admission, alternate digest alias, PATH shim or saved
script execution is allowed. Preserve cancellation and child-process ownership.

CI uses the shared installed-layout validators and confined pending-file custody
directly for finalization, with its hardened exhaustive ignore proof. Do not edit
the frozen initializer selection recipe merely to inject this Git policy into
its activation wrapper. Preserve normal initializer behavior and historical
recipe reconstruction. Disable repository-configured executable Git helpers
during inspection, and apply explicit submodule handling to both index and
worktree cleanliness checks.

Do not let a superproject worktree diff invoke submodule-local filters. Inspect
each worktree without recursive submodule content checks, then inspect initialized
gitlinks separately with the same hardened Git reader. Require confined physical
child roots, matching recorded gitlink commits, clean child indexes/worktrees
and the same flag checks. Empty uninitialized submodule directories may remain
uninitialized; unsafe or nonempty ambiguous children are preserved refusals.
Bound traversal to 128 gitlinks and 16 nesting levels. Exceeding the bound fails
before publication. This preserves ordinary submodule projects without running
their configured filter, fsmonitor or diff programs during preparation.

Only clean, tracked, authenticated public originals can be replaced without
initializer-style adjacent backup files in this explicitly disposable mode.
Their original bytes remain in the unchanged Git commit and index. This is a
scoped publication policy of the existing publisher; ordinary initializer and
migration backup behavior is unchanged. Do not produce unignored backup cruft,
delete unknown private data, or automatically roll back a partial publication.
Failure retains the pending barrier and partial owned state. A fresh checkout
is the recovery path; ordinary migration's durable backup/recovery is separate.

## Independent acceptance and evidence

The spec-writer owns Ginkgo/Gomega tests and the implementer owns production
code. Start from actual packaged initialization, commit the public project, and
clone it without private custody. Through the real compiled public command and
controlled authenticated-release fixture, prepare the checkout and run its
installed selftest/configured check, including a failing application command.
Keep Go and Python unavailable during installed execution. Prove unchanged
application/configuration files and Git HEAD/index, effective ignores, no
adjacent backups, and complete installed validation.

Exercise cross-platform reprojection with an inert, authenticated origin image;
never execute a foreign runtime. Refusal criteria cover absent consent, malformed
selectors, mismatched release/source/runtime bindings, failed authentication,
customized public assets, dirty tracked state, unsafe paths, existing custody
and cancellation. Before-publication refusal preserves the checkout. Reuse
existing acquisition/initializer custody tests where they already qualify shared
primitives; avoid duplicating the entire migration matrix or weakening deadlines.

Exact test-owned committed source images may add a receipt proving which selected
owner ran, or a narrow scheduling gate after real pending-barrier publication.
Observe that handshake before signalling the actual command. These qualify
handoff and interruption deterministically without production test controls;
label their evidence as instrumented. Keep the ordinary successful installed
journey on an uninstrumented image, and retain real publication/admission checks.

All runtime/build/hook execution occurs in complete private checkouts. Preserve
the operator's ignored binary and unrelated edits. Record observed RED/GREEN,
independent review and exact-head hosted qualification separately. This decision
closes only the CI preparation primitive, not the remaining R4 rehearsal or
R5-R7 candidate/platform, pilot and stable-publication gates.

## Authoritative Git interfaces

Read on 2026-11-07: [check-ignore](https://git-scm.com/docs/git-check-ignore)
documents NUL-separated stdin/output and index-independent ignore inspection;
[diff](https://git-scm.com/docs/git-diff) documents explicit submodule handling
and external-helper controls; [ls-files](https://git-scm.com/docs/git-ls-files)
documents stage records and assume-unchanged/skip-worktree flags. Actual private
Git probes additionally reproduced the unsupported `-z` without stdin spelling,
the staged-submodule override and execution of configured helpers. No Git or Go
version pin is changed by this decision.
