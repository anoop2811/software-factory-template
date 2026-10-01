# ADR-0091: Create durable local recovery sets before migration

Status: Accepted for source implementation; installed cutover remains pending.
Date: 2026-10-01
Decision: 90

## Purpose and boundary

Implement the second existing backup/rollback milestone after native command
PR #117. The requirements are specs/001-go-runtime-conversion.md:214,
specs/001-go-runtime-conversion.md:323 and
specs/001-go-runtime-conversion.md:337. Reuse the six reviewed legacy references,
explicit adoption proposal, confined observation and recovery integrity reader.
No new dependency or version pin. This is partial source implementation, not a
whole-factory backup, successful installation transaction or release cutover.

Backup creation preserves active originals. It never executes copies, changes
runtime routing, rewrites the Git index, resets runtime history, downloads assets
or invokes a model. Restoration, destructive retirement and later-release
retention require separate qualified implementations. A valid recovery manifest
still grants no restoration, activation or deletion authority.

## Public contract

The source-built Go CLI reserves bare --create-backup and its attached spellings
before legacy dispatch. The new operation accepts exactly:

```sh
factory upgrade --create-backup --migration-id ID --target-revision REVISION \
  --adopt-path PATH --confirm-adoption DIGEST [--json]
```

--adopt-path repeats for distinct reviewed paths. Every other option occurs once;
bare switches reject attached values. ID obeys the recovery reader's 1..64 ASCII
identifier grammar. REVISION is 40 lowercase hexadecimal characters distinct
from the compiled reference revision. It is operator-supplied descriptive target
metadata, not an authenticated source identity. Require a nonempty valid explicit
confirmation; environment variables and pre-existing manifests cannot supply
consent. Reject source, dry-run, inspect-backups, ref, help and unknown operands
in creation mode. Existing previews, private protocol precedence and ordinary
legacy upgrade dispatch remain unchanged.

Resolve the installation only from the physical current directory. Validate the
complete selection and recomputed adoption digest before making persistent
changes. Copy only selected unchanged reference assets; customized, missing,
unsafe or identity-changed selections refuse. An unselected customization stays
untouched. The selection and root identity remain pinned and are rechecked after
copying and immediately before publishing the completion manifest.

Success returns status 0 with bounded deterministic text or JSON containing
schema_version=1, mode=create_backup, coverage=partial, scope=g2-budget-loop-six,
migration_id, path, result (created or already_present), file_count, bytes,
source_revision, target_revision, target_authentication=operator_metadata,
restorable=false, activation_ready=false and prune_authorized=false. Report only
fixed relative paths and known metadata; never print source contents, absolute
roots, Git output or unsanitized error operands. Refusal uses status 2; operational,
sync, cancellation and output errors use status 1 with one safe diagnostic.
Failure after writes must say that local recovery state may remain and needs
inspection, rather than imply that nothing changed or migration succeeded.

## Effective local Git exclusion

Require a normal non-bare Git installation rooted at the current directory with
a physical .git directory and physical info directory inside it. Linked worktrees,
submodules with gitfiles, external Git directories and missing Git are safely
refused in this initial writer; support must be qualified before final cutover.
Do not honor inherited GIT_DIR, GIT_WORK_TREE, GIT_INDEX_FILE or injected Git
configuration/environment overrides. Use literal argv and bounded, cancellable
Git commands, with a finite per-command deadline and bounded captured output.

Before copying any backup payload, use git ls-files to refuse any tracked path
under .factory/backups, including a tracked recovery-root file. Never untrack it.
Preserve .git/info/exclude contents and all project .gitignore files. If needed,
append only a newline-delimited /.factory/backups/ rule to the validated ordinary,
single-link, current-owner local exclude file. Bound its input to 64 KiB and
reject unsafe types, special modes, writable-by-others files and ancestors.
Serialize cooperating exclusion updates without replacing their lock inode.
Sync and read back the change. A retry must not append duplicate rules.

Use Git's actual ignore evaluation, not a regex approximation, to confirm that
the root, selected set, manifest and every saved leaf are ignored. Recheck tracked
paths and effective ignoring before completion publication. Higher-precedence
project rules that expose any recovery path block creation; do not overwrite
those rules. A failed operation may leave the narrow local exclusion installed,
but must not pretend that its backup was completed. Existing effective ignoring
does not exempt tracked-path or unsafe-control-directory checks.

Authoritative Git behavior consulted 2026-10-01:
https://git-scm.com/docs/git-check-ignore,
https://git-scm.com/docs/git-ls-files and
https://git-scm.com/docs/gitignore. Ignore rules do not protect tracked files;
local exclude rules have lower precedence than project ignore rules.

## Confined durable publication

Require a trusted quiescent installation: current-owner real directories, no
special bits or group/other writing, no symlink traversal. Retain descriptors for
installation, Git control, recovery ancestors and selected source leaves; recheck
their named identities and relevant metadata at mutation/publication boundaries.
This does not claim a filesystem-wide snapshot against hostile same-UID writers.
Reject links, multi-link regular files, FIFOs, devices and unsupported modes;
ordinary regular-file I/O is not promised interruptible.

Create .factory safely if absent, and private 0700 backups/set/nested directories.
Reserve ID exclusively before payload writes; no operation replaces an occupied
set, resets permissions of an existing path or recursively removes a partial set.
Limit inventory to the reader's 64-entry bound before reservation. Concurrent
creators cannot overwrite each other's reservation. Require private 0600,
single-link saved regular files, never original executable modes. Copies retain
the relative files/<catalog path> layout. Record original mode, bytes, digest and
reference source/declared target revisions in the strict existing v1 manifest;
its catalog constraints establish original type as regular.

Write through exclusive no-follow descriptor-relative opens. Check source
identity, type, mode, size and digest against consent while copying. Sync each
saved file, verify its readback, and sync created directories bottom-up. Publish
manifest.json exclusively only after payload validation and the final consent,
containment, tracked-path and ignore checks. Sync the manifest, set and ancestors;
read back through the existing integrity inspector before returning success.
No completion manifest is written for failures before that boundary. A sync or
readback failure after manifest publication remains an error with preserved
evidence; a parseable manifest alone never certifies transaction durability.

An occupied set is reusable only when its strict manifest exactly matches the
requested ID, target revision and sorted selected references, the complete tree
passes the existing inspector and effective ignore/tracked-path checks pass.
Re-sync and validate it before reporting already_present. Unknown, held, changed,
unsafe, incomplete or differently selected/targeted sets remain untouched and
refuse. Reuse does not establish its historical origin or advance retention.
Interrupted sets remain visible to inspection; same-ID retries do not overwrite
them, and arbitrary automatic new IDs must not accumulate retry cruft.

## Qualification and progress

Independent outside-in Ginkgo/Gomega RED must precede implementation. Observe
compiled creation, Git status/ignore state, exact bytes, inert complete modes,
strict reader acceptance, unchanged active/runtime/user assets, repeat behavior,
single-path and full-catalog selection, confirmation/root changes, malformed
arguments, tracked paths, negation precedence, absent/unsafe Git/recovery roots,
hard/symbolic links, FIFOs, occupied/held/incomplete sets and privacy. Independent
internal fault tests must cover file/directory sync failures, partial writes,
cancellation, changed originals/ancestors, concurrent reservation and descriptor
closure. At least one real failure must prove that no completion is published.
Run race, quality, source gates and Linux/macOS CI; no paid provider is needed.

The existing 30-package source denominator remains fixed. Qualifying and merging
this second of three backup/rollback milestones earns 1/3 of one package:
(17 + 2/3 + 1/3)/30 = 60.0%. Before qualification/merge, earned progress stays
58.9%. Complete installed coverage, controlled restore, transactional activation,
legacy retirement, retention and adopter/platform qualification remain pending.
