# Upgrade v0.1.6 to the Go factory

## Availability and the command you keep

[v0.2.0](https://github.com/anoop2811/software-factory-template/releases/tag/v0.2.0) is the stable Go release. The public installer selects it by default. Pin the exact version for preview and confirmation so both steps authenticate the same release.

You continue to use `./factory` in your project. Thin executable and sourceable
adapters retain supported old paths for hooks and automation. Regular users do
not need the separate maintainer staging/packaging tools. See [CLI.md](CLI.md).

## Earlier experimental Go installations

Use the freshly downloaded installer in steps 1 and 2 for both preview and
confirmation. An older experimental Go executable can reject a newer bundle
with a `go_version` source-metadata error before it can hand off to the new
runtime. Its immutable reader cannot be repaired by changing the new release.

The installer authenticates the selected release's raw bootstrap first, then
that new owner reads its bundle and the supported predecessor. Current source
keeps compiler provenance separate from requested artifact identity while
retaining attestation, digest and exact inventory checks. This route requires no
Go compiler or Python in the adopter project. Keep the exact same published tag
and project inputs for preview and confirmation; never edit `source.json` or
version markers to bypass a refusal.

The v0.1.6 Bash migration already uses this authenticated installer route.
An unknown/customized predecessor can still be a preserved blocker. These
instructions do not make an unpublished release available or turn earlier
experimental Go readers into direct-upgrade-compatible programs. See
[the compiler compatibility contract](adr/0149-compiler-provenance-and-security-patches.md).

## Before upgrading

- The Go replacement supports Ubuntu 24.04+ and macOS 15+, on amd64/arm64.
  macOS 14 is outside the supported scope under Decision 128 (2026-10-30).
  Historical candidate results do not qualify a newer binary.
- The initial supported predecessors are v0.1.6 and the immutable Bash baseline
  `76952eaa63aebd1ecd282f5ab51dd7c3627cb497`. A marker claiming a version is not
  enough: observed files must identify a supported installation. Unknown or
  customized required files can block automatic migration and are preserved.
- Start from a reviewable working tree. Save/commit your own project work so its
  diff is distinguishable from the upgrade. The upgrader does not commit for you.
- Release acquisition needs an authenticated GitHub CLI (`gh auth status`), Bash and the existing platform
  tools, plus network access to official GitHub releases and verification roots.
  The installed Go runtime needs no Go compiler or Python. Git, retained JSON gates
  such as those using `jq`, coding harnesses and your product's language tools
  retain their separate requirements.
- Review both the proposed file actions and existing backup/retention exceptions.
  For application source, `factory.yaml`, custom hooks, role/model choices, native
  permissions and budget/loop history, preservation/conflict reporting is the
  contract; an upgrade is not permission to reset or overwrite them.

## 1. Preview the exact published Go release

From inside your existing project (a project subdirectory is also supported):

```sh
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- upgrade --ref v0.2.0 --dry-run --json
```

For a published prerelease, replace `--ref v0.2.0` with `--go-candidate TAG` in
both preview and confirmation, using its exact tag. The command obtains the matching native binary and installation
image, checks their attestation against the official release workflow, tag and
source revision, and computes a proposal. The preview uses temporary private
acquisition storage and leaves the project unchanged. It calls no model.

Read `proposal_digest`, all blockers, retained/replaced/retired paths, recovery
requirements and the `retention` report. A preview is not permission to apply.
If a historical initializer input is missing, the diagnostic can request a
repeatable literal `--project-input KEY=VALUE`; use the same inputs on confirmation.
Do not change version markers, force ownership or delete custom files to clear a
blocker. See [candidate handoff](migration/GO_CANDIDATE_HANDOFF.md).

Read the `ci_workflow` section too. For an eligible authenticated upgrade,
`apply_supported: true` means the listed workflow changes participate in the
same confirmation, backup and recovery transaction as the runtime. The upgrader
recognizes only exact supported factory copies at five ordinary workflow paths.
When the review lane is enabled, it also considers
`.github/workflows/adversarial-review.yml`: an exact predecessor-owned workflow
is refreshed with its existing secret selection, while an absent workflow stays
absent. An off lane is untouched. Other application workflows are outside its
selection. Customized or unsafe occupants at selected paths are preserved
blockers. Review and reconcile
those conflicts deliberately; adding a new job alone does not repair old jobs.
Local or unsupported release previews explain why automatic authenticated CI
migration is unavailable. See the
[transactional CI migration contract](adr/0148-transactional-installed-ci-migration.md)
and [review workflow migration](adr/0154-installed-review-workflow-migration.md).

## 2. Confirm during a quiet maintenance window

Stop factory jobs, background harness work and other automation that can touch
this installation. Keep them stopped for the transaction. Older processes do not
join the new runtime's locks merely because files on disk change; `--quiescent`
is your explicit affirmation of this maintenance condition.

Replace `PROPOSAL_DIGEST` below with the full digest from your reviewed preview:

```sh
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- upgrade --ref v0.2.0 --confirm PROPOSAL_DIGEST --quiescent
```

Repeat any `--project-input` overrides from the preview. Omit `--json` on apply;
it is preview-only. The engine independently reacquires and rechecks identities,
project observations and backup state. Changes since preview require a new
preview and confirmation. Validation and durable publication precede a successful
activation result. There is no silent fallback to another release or script runtime.

When the proposal includes CI migration, unchanged old factory jobs are retired
and `.github/workflows/factory.yml` is created or refreshed for the selected
release. Their original bytes join the same ignored recovery set. Changing a
selected workflow or relevant configuration after preview requires fresh consent.
Review and commit the resulting workflow diff yourself; the factory does not push
it to your repository or enable a paid review lane.

## 3. Check the selected installation

After successful activation, from the project root:

```sh
./factory help
./factory doctor
./factory selftest
./factory budget report --json
./factory metrics --json
```

Read each result and inspect your Git diff. `doctor` and `selftest` run deterministic
gate proofs; they do not call a model. Budget reporting preserves unknown usage
rather than converting it to zero. Run your normal product tests/checks separately;
they retain their own prerequisites and side effects. Existing paid/optional
settings remain explicit: migration does not enable budgeted runs, bounded model
loops or a paid review lane for you.

If a command fails, preserve the diagnostics and migration evidence. An exit that
reports activation succeeded but cleanup failed requires cleanup reconciliation;
it is not permission to reinstall or discard recovery files.

## Ordinary Go upgrades

The shell installer accepts the same exact stable Go selection:
`install.sh init --ref TAG` or `install.sh upgrade --ref TAG`. Explicit `--ref`
wins over `FACTORY_REF`. It authenticates stable publisher metadata and the
native bootstrap before execution; the Go owner then validates the complete
installation image. Go releases use the official repository; a conflicting
`FACTORY_REPO` refuses. See the [bootstrap contract](adr/0124-ordinary-authenticated-go-installer-bootstrap.md).

The public default is v0.2.0. Older stable tags keep
their legacy route. A non-version branch containing the native init/upgrade
adapters explicitly selects that committed local source and requires the Go
toolchain; native upgrades preview before fresh confirmation. A reused cache
must match the requested repository/ref; a mismatch refuses with guidance to
refresh safely or select a fresh `FACTORY_HOME`. Legacy execution rejects `--`
operands that its initializer cannot preserve. A bare fetch only caches source;
Go-tag advice re-enters authenticated initialization, and native-branch advice
explicitly selects local source. It never executes the fetched initializer.
The tracked developer launcher and direct init/upgrade scripts now enter Go.

A Go release containing ordinary upgrade selection can preview its declared
stable release with `./factory upgrade --dry-run --json`. Current source selects v0.2.0.
Selection is one explicit `--ref TAG`, then `FACTORY_REF`, then the declared
default. Upgrade does not implicitly reuse the installed version.

Pin the same exact stable selection for preview and confirmation:

```sh
./factory upgrade --ref v0.2.0 --dry-run --json
./factory upgrade --ref v0.2.0 --confirm PROPOSAL_DIGEST --quiescent
```

Ordinary refs reject moving aliases such as `main`, old Bash tags and
prereleases. Use explicit `--go-candidate TAG` for a published candidate. An
authenticated Go prerelease predecessor can advance to stable; a stable
downgrade refuses and directs you to explicit rollback. Equal-release retries
remain eligible without advancing backup retention.

Local or unsupported Go version labels cannot identify an official before-image.
Preserve the original source and use `--local --source DIR --before-source DIR`
or the documented whole-installation archive route. Unavailable or unauthenticated
official images preserve the project and fail without a script fallback.
The existing `--dry-run --source DIR` assessment remains a separate read-only
source inspection; it does not select an authenticated release or authorize apply.

## What happens to the old scripts and backups?

The reviewed installation map classifies each supported predecessor asset.
Unchanged, proven-owned obsolete implementations are retired or replaced only as
part of the compatible Go transaction. Supported public script paths become thin
adapters; policy gates and native harness formats can intentionally remain shell,
JSON, TOML or TypeScript. Their existence is not a fallback to the old controllers.
The source repository retains compatibility implementations and fixtures until
its separate source/default cutover; migration cleanup concerns the selected
adopter installation, not every script in the repository.

The current Go implementation also declares the 83 unused private scaffold-cache
files from older full-cache Go installations retired. The metrics HTML template
and review-lane workflow template remain installed. A cache file retires only
when its bytes, type and mode match a freshly qualified predecessor that actually
selected it. A minimal-generation predecessor never owned those omitted paths;
equal-looking leftovers there remain preserved. See the
[cache retirement contract](adr/0123-owned-obsolete-initializer-cache-retirement.md).

Original replacement/retirement bytes are saved as inert reference/recovery data in
private Git-ignored `.factory/backups/MIGRATION_ID/`. Backup scripts are not executed
as a rollback mechanism. Application files, customizations and runtime/accounting
history are not cleanup targets. Edited, unknown, held, unsafe or unqualified
entries are preserved and individually reported, even when that prevents cleanup.

After A → B, recovery for A remains. A later distinct successful authenticated
B → C upgrade can prune qualified older recovery for A while retaining usable
recovery for B. Same-release retries, rollback, local builds and elapsed time do
not advance expiry. To hold a checked set for reference:

```sh
./factory upgrade --keep-backup MIGRATION_ID
```

To request cleanup of an older set:

```sh
./factory upgrade --prune-backup MIGRATION_ID
```

These standalone controls recheck eligibility; prune refuses the current/sole
immediately preceding recovery or unavailable authentication. Do not combine them
with apply, release-selection or recovery flags. See [backup retention](migration/BACKUP_RETENTION.md)
for file/byte reports, preserved exceptions and retry behavior.

## Interrupted upgrades and rollback

An interrupted transaction can leave a pending barrier that deliberately blocks
ordinary factory work. Keep its backup, journal, private authenticated input cache
and the exact recovery command printed by the diagnostic. Do not remove the
pending marker, expire locks by age/PID or execute the saved scripts.

Whole-installation rollback and forward/reverse interrupted recovery use the
`upgrade --installation` interface with independently qualified source images and
a fresh proposal/digest. The public candidate flow prints the concrete read-only
recovery preview needed for its actual state. Follow that evidence-specific
command before confirming any recovery; `--go-candidate` alone is not a generic
rollback switch. Later edits and unrecognized state remain conflicts.
See [the whole-installation contract](adr/0098-whole-installation-upgrade-and-rollback.md)
and [candidate handoff](migration/GO_CANDIDATE_HANDOFF.md).

Rolling back to v0.1.6 also checks that the old factory can run. Have its
original prerequisites, including Python 3, available before confirming.
With the Decision 176 change, if a prerequisite check exits with a failure,
use the same trusted installation inputs to preview `--recover ID --direction
forward` with `--dry-run --json`. Confirm that proposal to return to the Go
installation. Restore the prerequisite, then preview and confirm a fresh
`--rollback ID`. If the diagnostic reports unresolved process ownership, follow
its reconciliation instructions first.

Published RC4 predates this correction. If RC4 has already refused both rollback
and recovery after a missing prerequisite, preserve the journal and backup for
operator inspection. The [adopter pilot](migration/RC4_ADOPTER_PILOT.md)
records the diagnostics for that case.

Legacy configuration conversion is separate: `./factory migrate-config --dry-run`
previews `factory.config` conversion, and successful apply retains the adjacent
`factory.config.migrated` file. It does not switch runtimes or become part of the
runtime backup expiry policy. See [configuration migration](migration/CONFIG_MIGRATION.md).

## Explicit developer upgrade from committed local source

Use the Go CLI for this opt-in path. From inside the adopter, select a clean
committed factory checkout:

```sh
/path/to/factory-go upgrade --local --source /path/to/clean-factory --dry-run --json
```

The source-built caller can omit `--source` when it lives beside its own source
assets; installed Go callers require it explicitly. The selected source needs
the provisioned pinned Go compiler and its module dependencies. This builds a
native `local-FULL_REVISION` image and runs that exact build's own upgrader.
No official release image is downloaded and no publisher authentication is
claimed. Normal installed commands still need no compiler.

Read the proposal and blockers, then repeat the same Go invocation with
`--confirm PROPOSAL_DIGEST --quiescent`, removing `--dry-run --json`. This uses
the existing preservation, transaction and inert-backup controls. Source changes,
target changes or stale consent require a new preview. Local transitions never
advance release-backup expiry.

For an existing Go installation, also supply `--before-source
/path/to/exact-before-checkout` in both preview and apply. That checkout must be
clean and match the installed full revision. Its rebuilt image must reproduce
the original version, native target, runtime bytes and required asset bytes.
Matching metadata alone is insufficient. Compiler or recipe differences can
prevent exact reconstruction; the installation is preserved. If you retained
the original qualified archive, use the explicit before-image operands shown by
`upgrade --installation --help` instead. Legacy predecessors do not take
`--before-source`.

If an interrupted transaction prints a recovery command, preserve its named
private image directories and use that exact read-only preview. It uses retained
archives and an executable outside the active selection, so recovery does not
require recompiling. A hard kill cannot print new diagnostics; preserve the
journal, backups and private inputs for the existing whole-installation recovery
procedure. Never remove pending controls to force ordinary commands through.
See [the local upgrade contract](adr/0117-explicit-go-local-upgrade.md).

## Keeping a project on the Bash release

Projects that need the historical script update can explicitly select v0.1.6:

```sh
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- upgrade --ref v0.1.6
```

The historical updater refreshes known framework paths and can replace edits to those files. Save your changes first, review the resulting diff and run `./factory doctor`. To migrate to Go, use the preview and confirmation above.
