# ADR 0097: bind installation source images into authenticated bundles

- Status: accepted implementation refinement; installation consumer and default cutover remain pending
- Date: 2026-10-06 UTC
- Decision: 96

## Release objective and dependency order

The user authorized completing the Go replacement and publishing a release after
legacy runtime retirement. The governing requirements remain
specs/001-go-runtime-conversion.md:163, specs/001-go-runtime-conversion.md:168,
specs/001-go-runtime-conversion.md:194 and specs/001-go-runtime-conversion.md:324.

Pre-code correctness/security review established a required dependency: current
three-file runtime bundles cannot supply independently known installation assets,
and separate one-file publications cannot form one installation transaction.
Public completed-installation rollback must follow a complete installation
consumer; do not weaken the interrupted recovery matrix or import journal fields
as ownership, trusted after-images or compatibility authority.

This delivery provides the complete committed-source image and the two approved
immutable reference catalogs inside the same authenticated archive as the binary.
It is a prerequisite to that consumer, not installation activation or R3 closure.
The following deliveries must implement one installation-owned pending barrier,
fresh explicit adoption, backup-before-mutation, complete compatibility/health
checks, public rollback/recovery, legacy retirement, retention and release/pilot
qualification. Preserve the fixed 30-package denominator and all feature rows.
Go progress remains 60.7% until an existing measured milestone actually closes.

## Additive format selection and compatibility

Add Installation bool to packaging.Options and staging.Options, default false.
Add the Cobra boolean --installation option to factory-package and factory-stage.
Use Cobra's existing boolean grammar and error handling. Producer and stager JSON
success summaries retain their existing exact fields; the new wire format is
described by its archive contents, not extra success authority fields.

Without this option, the existing three-file V1 archive, source-build metadata,
limits, output, modes, ordering tolerance and trust behavior remain unchanged.
Installation mode requires the new format; a valid V1 archive must not silently
satisfy it. V1 mode rejects the expanded image archive. No automatic format,
target, release, local-authentication or version fallback is introduced.

Keep runtime.manifest in its existing FACTORY_RUNTIME_ARTIFACT_V1 format; it
continues binding the binary. Installation source.json retains the exact five
identity keys and changes only kind to installation-source-build. All identities
must equal the requested committed revision, literal version and explicit target.

## Closed archive and wire contract

Use the existing VERSION/TARGET/ archive slot. Installation archives have:
1. bin/factory-runtime
2. runtime.manifest
3. source.json
4. installation.manifest
5. installation/assets/PATH for every current committed-source blob, in bytewise
   path order.

The four control entries occur first in the order above; payload entries then
match the manifest's sorted asset order exactly. No undeclared, duplicate,
reordered, missing or extra member is accepted. All archive members are regular
files; no tar links, devices, FIFOs or hidden directory entries. Retain strict PAX,
complete tar end markers, gzip checksum, single-stream and zero-padding checks.

installation.manifest is strict UTF-8 JSON with exactly these nine keys:
schema_version=1, mode=installation_source_image,
scope=committed_source_and_baseline_references, version, target, source_revision,
activation_ready=false, assets and baselines. Reject duplicate/unknown fields at
every object level, wrong types, trailing data and unsupported values.
Reject invalid UTF-8 and unpaired JSON surrogate escapes rather than silently
replacing them during decoding.

assets is the complete sorted current Git blob catalog. Each entry has exactly
path, mode, sha256 and bytes. Modes are Git logical 100644, 100755 or 120000;
digests are lowercase SHA-256; bytes is a bounded nonnegative integer. No empty,
duplicate, unsafe, invalid UTF-8 or noncanonical path is permitted. Require at
least one current asset and a prefix-free file map: a and a/b cannot coexist.
Reject such collisions before publication. Reject NUL,
backslash, CR, LF and TAB, absolute paths, dot/parent/empty components and names
longer than 1024 UTF-8 bytes. Reject the exact root .factory path and everything
below .factory/; unrelated names such as .factory-not-private are allowed.
Reject unsupported Git tree entry types rather than silently omitting them.

A logical symlink is stored only as its raw Git link-target bytes in an ordinary
inert payload file. Its digest and size describe those bytes, not the referent.
Never follow or create source links while collecting, packaging or staging.
Executable logical source assets also remain ordinary data in staging.

baselines contains exactly two entries in this order, with exactly label,
revision and assets:
- v0.1.6: b71ecc32e07ecd87eb330ba8e497c86612f92acd, 137 blobs.
- bash-baseline: 76952eaa63aebd1ecd282f5ab51dd7c3627cb497, 160 blobs.

These are complete immutable Git blob reference catalogs, not installation
ownership or deletion manifests. Their entries use the same asset schema, with
no additional payload members. The candidate source image also includes source,
test and development files as inert evidence; a later installer must use an
explicit reviewed installation selection and disposition map rather than copying
this entire source image into an adopter or inferring ownership from paths.

Pin the baseline catalog digests independently of imported data. Canonical input
is FACTORY_INSTALLATION_CATALOG_V1 followed by LF, then each sorted path, mode,
sha256 and base-10 byte count, each followed by NUL. The observed SHA-256 values:
- v0.1.6: 595f75dddcda285ba3b43a0936970ef96af3b9eae66f4beb9fd3a37df5d7e16b
- bash-baseline: 8514055a8ee6f79a8fd891a6107e46bd9d0f1a2096e349825db83aafc7b91bbe

Provenance: observed 2026-10-06 UTC using git ls-tree -r -z at the two immutable
commits and git cat-file blob for each object, with replacement refs disabled.
Root read the resulting counts/digests before this implementation contract.
Independent evaluator must calculate its own oracle, not load producer helpers.

## Bounds, custody and producer reuse

Limit an image manifest to 512 KiB, the current catalog to 4096 blobs, each source
blob to 1 MiB, and total current payload bytes to 64 MiB. Baseline counts/digests
are exact. Keep the existing 256 MiB runtime binary and 128 MiB compressed input
bounds. Installation decompression is bounded at 321 MiB plus the existing
one-byte overflow sentinel; V1 retains its 257 MiB boundary. This aggregate wire
bound also includes tar headers, PAX, padding and control entries. It is an
additional independent limit, not a promise that every combination of individual
maxima fits. Producer and stager must enforce the same aggregate ceiling before
publishing, including the exact-boundary/one-byte-overflow controls.

Reuse the isolated committed-object repository, no replacement refs, forced
export-ignore/export-subst suppression, pinned offline compiler and CGO-disabled
target build. The complete source collector must read only requested committed
objects and both fixed baseline commits; working-tree edits, untracked files,
local attributes, replace refs and caller Go environment must not change the
image. Missing baseline objects refuse; do not download or infer another baseline.
Bound new Git child stdout and diagnostics while reading, before whole-output
allocation. Use size preflight and capped streaming for catalogs and blobs;
count/byte limits after an unbounded Output call are insufficient. Cancellation,
oversized actual trees/blobs and read failures must trigger checked child cleanup
and no success publication. Preserve the existing V1 collector's contract.

Share one installation-image schema/validation implementation between packaging
and staging. Reuse the current bundle publication, archive writer, scoped roots,
hashing and trust/snapshot machinery where appropriate; preserve checked closure.
No new dependency, tool version or compiler pin is introduced.

Only the fixed top-level runtime binary is executable: its producer/archive mode
is 0755 and its private staged mode is 0700. Every image control/payload file uses
0644 in the archive/producer and 0600 in staging, even a payload named
factory-runtime or having logical Git mode 100755. Never dispatch from image data.

## Staging, trust and publication

Authenticate the entire private archive snapshot before extraction in official
mode using the existing independent trusted-root, repository, workflow, source
revision and source-ref policy. No network fetch or candidate execution is added.
Explicit local mode remains local-source and never claims publisher authenticity.

Validate control identities, the complete manifest, pinned baseline catalogs and
every declared current payload digest/length before publishing a result. The
archive must supply exactly that map. Build and staging output reservations remain
exclusive and private; invalid content/trust produces no final output. Publication
failure may leave a partial exclusive output and must not delete foreign files
or claim success. Preserve cancellation and cleanup-error precedence.

A staged path or prior Result is not future installation authority. A later
consumer must requalify custody, authenticity and all images under its own
installation exclusion before any mutation or candidate execution. No adoption,
backup, installed-version update, activation, public rollback, pruning, model
call, background agent or default-runtime change belongs to this slice.

## Independent outside-in qualification

The spec-writer owns only new Ginkgo/Gomega files. First observe the missing flag
surface separately, then interface-only options/flags with unsupported behavior,
then actual runtime RED before allowing the implementation. No existing tests or
assertions may be edited by the implementer.

Use real committed fixture repositories, native producer/stager binaries and
independent Git blob oracles. Qualify complete catalogs, raw symlinks, empty and
executable files, filename role collisions, reproducibility, baseline tampering,
closed-world/member ordering, strict JSON, every bound, source identity and
trust-before-extraction. Pair malformed staged fixtures with a valid identical
fixture to prevent unsupported behavior from passing every refusal case.

Requalify frozen V1 packaging/staging, full source race/quality gates and four
native target bundles. Apply review-diamond: independent correctness, security
and test lenses, reproduce/refute findings before deterministic dedupe, then
reviewer synthesis. Real release authentication and the manual adopter pilot
remain later release gates; mocked verifier transport is not live attestation
qualification.
