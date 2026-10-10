# ADR 0098: one installation transaction for upgrade and rollback

- Status: implementation contract; independent RED precedes behavior
- Date: 2026-10-09 UTC
- Decision: 97

## Delivery and scope

Build the complete installation consumer after merged PR #124, using the image
contract in ADR 0097. Refine specs/001-go-runtime-conversion.md:154,
specs/001-go-runtime-conversion.md:163, specs/001-go-runtime-conversion.md:168,
specs/001-go-runtime-conversion.md:183 and specs/001-go-runtime-conversion.md:214.

The delivery includes reviewed installation selection, generated compatibility
adapters, qualified installed asset lookup, installed deterministic health checks,
one durable batch transaction, interrupted recovery and fresh completed rollback.
It must not simulate a batch by looping separately admitted single-file APIs.

The opt-in target is go-hybrid-v1: existing native commands use the installed Go
binary; explicitly unconverted check/sync/eval and host-required gates remain
declared shell boundaries. The old implementation-specific selftest cannot be
used as the installed proof after replacing its dependencies. Supply an installed
Go proof boundary before activating this target. Existing source-mode fixtures
and legacy command contracts remain unchanged.

This is not default release cutover. Trusted first-install/bootstrap handoff,
remaining orchestration conversion, retention, actual release authentication,
minimum-platform qualification and the mandatory adopter pilot remain gates.
Regular users continue to use factory. Developer package/stage CLIs stay separate.

## Additive public command

Reserve the explicit --installation marker in factory upgrade before old preview,
recovery and script fallback, including attached, false, malformed or repeated
forms. Existing requests without the marker retain their current contracts.

Whole preview uses --installation --dry-run --source ARCHIVE --profile PROFILE
--version VERSION --revision FULL_SHA --target GOOS/GOARCH, with optional --json.
PROFILE is exactly v0.1.6 or bash-baseline for initial legacy adoption. Source is
the complete installation archive, not a Git checkout, saved staging Result or
untrusted installation directory. Identity/trust operands have the same literal
grammar and four-target restrictions as staging. Explicit --local labels local
source; otherwise --attestation, --trusted-root and optional --gh are required.

Forward apply adds --migration-id ID --confirm DIGEST --quiescent. DIGEST is the
current complete proposal confirmation, not a saved receipt or environment value.
The operator establishes and maintains prevention of unbridged work throughout
the maintenance window. A free flock, empty history or absent PID is insufficient.

Completed rollback uses --installation --rollback ID with freshly supplied
qualified before/after images, identities and trust inputs, a new proposal and
confirmation, and explicit quiescence. Interrupted recovery uses --installation
--recover ID --direction forward|reverse with the same fresh qualification.
These routes never execute backup scripts or silently fall back to Bash.

Help remains available. Malformed operands and preserved conflicts return 2;
real I/O, cancellation, authentication, process or checked closure failures return
1. Joined errors retain errors.Is/errors.As causes and operational precedence.

## Reviewed installation role map

The complete Git catalogs are reference evidence, not an install-all manifest.
Keep a deterministic compiled role map binding source path, installed path,
transformation, required/optional status, full mode and disposition/reason.
Changes to this map require review; no path-extension ownership inference.

Generate factory and direct adapters for native init, doctor, report, metrics,
review-lane, migrate-config, budget and loop. The upgrade adapter must reach the
whole consumer without recursive old dispatch. Sourceable config.sh, roles.sh
and budget-config.sh retain their supported functions through the Go bridge.
Retain budget-config.sh's public path even if current internal callers disappear.

Retire recognized unchanged budget.py, budget_adapters.py and loop.py only after
adoption, checked backups, adapter/reference closure and installed proof. A later
runtime_transition.py has no historical v0.1.6/76952 blob: absence is harmless,
an unrecognized existing file is preserved rather than deleted by filename.

Retain explicit check/pre-push, sync, eval, adversarial transport, gates and
events/hookspath/color/timing boundaries until their own conversion. Required
native data includes templates/metrics.html and packs/review-lane/review-pr.yml.
Installed selftest/run.sh is a thin adapter to the native installed proof; the
repository's historical selftest stays source/CI evidence, not installed fallback.

Preserve configuration, identity/prompt/plugin/package settings, generated host
adapters, application Makefile/CODEOWNERS/CI, README, specs, ADRs, memory/wiki,
credentials and history. Existing custom files cannot be regenerated wholesale.
Missing required data may be added from explicit selections; unsafe or customized
required framework paths stop activation. Optional preserved conflicts are named.

Recognize copied baseline files through independent immutable blob catalogs and
reviewed installed-mode normalization. Recognize transformed files only through
the actual baseline initializer's deterministic substitutions and explicitly
bound project inputs. Source hashes do not recognize arbitrary generated files.
Unknown transformations or profiles preserve/conflict. Language pack remappings
and product files are retained; this consumer never selects the Go product pack.

The v0.1.6 profile has no budget/loop implementation, while 76952 does. Qualify
real installed fixtures separately; source counts 137/160 are not install counts.

## Installed layout and custody

Use fixed private .factory/bin/factory-runtime, runtime.manifest and source.json,
plus a strict .factory/installation.current descriptor. These are explicit
installation roles, not arbitrary exceptions to source-image path validation.
Only the declared binary is executable. Manifest/source controls are private data.
Runtime data and canonical init assets have a declared private asset namespace;
they never come from backups, caller PATH or an environment-only locator.

The generated public launcher binds the expected version, revision and runtime
digest. Verify physical layout, descriptor/control identities, binary and data
before dispatch; corrupted or missing selections refuse without download,
compilation, update or fallback. Native installed asset lookup distinguishes
application cwd, installation root and canonical assets. Source-mode lookup keeps
its existing behavior. Public adapter content includes release identity so actual
runtime changes invalidate ordinary source fingerprints without rewriting history.

The caller uses an independently trusted migration driver. A new target binary
cannot act as its own publisher verifier. Preview may inspect bounded image data
and current files without filesystem writes, external checks, models or downloads;
it reports trust not yet established rather than claiming publisher authentication.

Under retained exclusion, freshly snapshot/authenticate the entire archive and
requalify identities, catalog digests, selected payloads and generated target
bytes. Reuse staging, installationimage and artifact code; a previous Result or
local descriptor does not grant later target or mutation authority. Hold confined
roots/leaf identities and revalidate before each use. Local mode stays local.

## Whole proposal and consent

The proposal reports every create/replace/retire/retain action, preserved conflict,
mandatory check and rollback blocker. Its confirmation binds physical root,
baseline/profile and project inputs, source/trust snapshots, target and all
selected before types/modes/bytes/identities, checked absences and current state.
Recompute the complete proposal under exclusion before consent can be used.
No imported receipt, PID, config key or trusted boolean grants origin or adoption.

All user state is outside the ordinary installation action map. Never replace
budget/loop/history/control/backups through an asset row. Installed controls and
the narrowly added backup/runtime ignore rules have separate declared operations.
Validate effective ignore/untracked status before writing private binary or backup
data, preserve unrelated ignore semantics and never change the user's Git index.

## Exclusion and ordinary admission

One guard covers all preparation, backup, mutation, checks, metadata and closure.
Use the permanent interoperable transition flock and preserve its inode. Hold one
durable runtime-publication.pending marker throughout partial active changes.
Ordinary work must not observe a partially installed target or start between rows.

Pure read-only Go asset/state readers need a non-writing shared lease on the same
physical installation root; the installer needs the corresponding exclusive root
lease before its existing transition guard. Qualify root-directory flock behavior
on native platforms before relying on it. Define one lock order and pin/revalidate
root identity. Mutation/child work also retains existing durable activity evidence.
Budget/loop reuse their current ownership rather than weakening their guards.

Installed retained script dispatch requires owned inherited-stream supervision;
syscall.Exec alone would discard deferred lease cleanup. Preserve literal argv,
cwd, environment, stdout/stderr/stdin, interactive terminal and signal/status
semantics, while retaining typed uncertainty until owned group exit is established.
Keep source-mode exec behavior unchanged. No environment or public flag bypasses
pending admission. Unbridged older/direct callers still require explicit quiescence.

## Backup, publication and durable phases

Add one opaque installation capability with Propose, Begin, Complete and Close.
Purpose/direction is fixed at grant; aliases share private state. Completed
rollback obtains a fresh grant, not a reopened completed single-file handle.

Use a separate bounded whole-plan journal, preserving old R1/R2 formats and quotas.
Journal phases are prepared, applying, checking, metadata_prepared and committed,
with per-row prepared/applied evidence and reverse counterparts. Records describe
observations and progress; they never reconstruct historical ownership by trust.

Finish all changed/removed before-images under one private ignored migration set
before any destructive action. Preserve exact path/type/full mode/source identity
and checked content. Files are inert 0600, directories 0700. Backup failure stops
mutation. Bound regular assets at 1 MiB, their total at 64 MiB, selected rows at
512 and binary at the existing 256 MiB. Whole journals/manifests are at most 2 MiB;
one operation owns at most one 320 MiB before-image set and one target image.
Use bounded streaming for binary staging/backup/hash. Reuse filepublish rather
than allocating binary-sized []byte or duplicating checked publication semantics.

Additions bind checked absence and use no-replace publication. Replacements
revalidate original parent/leaf before staged rename. Removals durably record
intent before checked unlink and confirm exact absence; absence alone is never
proof of historical ownership. Record actual prepared/published identities and
reconcile errors after mutation. Preserve late foreign creations, edits, chmods,
links and unsafe parents. No recursive deletion of unclassified directories.
Only explicitly recognized CLAUDE.md-to-AGENTS.md link data may be retained;
other link actions require their own checked policy rather than following referents.

Write all terminal images/check results durably, then installed-version metadata
last, then the terminal record, then remove the exact owned pending entry and
sync its parent. Failed cleanup must retain typed primary uncertainty. Close
releases resources without implicit rollback or clearing unresolved evidence.

## Candidate health and semantic compatibility

Materialize a complete private candidate installation only after target trust.
Run actual compiled target help, installed asset/adapter checks and native
deterministic proof against an isolated candidate root. This root has no parent
installation pending marker or exclusive lock. Post-apply checks run through a
private owner-bound in-process validation capability against the retained root,
not an ordinary command or forgeable environment bypass.

Installed proof exercises actual shared gates with positive and negative controls
in isolated fixtures; no Go compiler, Python interpreter, model call or native
agent client is required. It cannot report success from shell status 0 alone or
omit shipped required gates. Source-only implementation tests remain separate.
Use existing owned supervision, finite explicit check allowances and durable PID
publication. Write checking before launch and save the actual PID onSpawn. A
crash between spawn and PID publication is unknown work, not safe automatic retry.

Activation and rollback check state semantics for the specific runtime profile.
For 76952, qualify unchanged known budget/checkpoint formats through independent
legacy reader fixtures. v0.1.6 lacks those controllers: refuse rollback when
modern state/configuration requires unavailable controls. Preserve all history and
later edits byte-for-byte. Readability alone is not downgrade safety.

## Fresh recovery and rollback

On restart keep pending and report a concrete recovery command. Requalify known
before/after images, actual current paths/absences, backups, complete journal
consistency, state compatibility and explicit consent under recovery exclusion.
Do not import a serialized live capability or derive ownership from equal bytes,
an absent path, old inode tuple, terminal journal or missing child PID.

Reverse only currently unchanged selected changes. Changed user entries are
preserved conflicts; new directories are removed only when owned and empty.
Restore previous metadata only after complete previous installation checks.
Rollback does not advance retention, reset counters, rebase fingerprints, execute
saved code or auto-restart model work. Repeated identical successful operations
create no new backup or mutation; unresolved operations keep retryable evidence.

## Qualification and measurement

Separate spec-writer owns new Ginkgo/Gomega files. Observe missing-marker surface
RED, interface-only unsupported runtime RED, then semantic RED against actual
legacy installations before behavior. Pair bad inputs with healthy siblings.
Test real candidate execution, multiple row types beneath one pending marker,
native process interruption and closure/error precedence. Requalify old preview,
R1/R2, native execution, guards and affected command contracts.

Run review-diamond with independent correctness/security/test verification before
deterministic dedupe and synthesis. Full source quality and native target CI must
pass. No new dependency or tool pin is selected. No protected auto-merge.

Keep the fixed 30-package denominator and all twelve conversation feature rows.
Current merged progress is 60.7%. Credit only existing milestones actually closed
by qualified and merged behavior; a draft, interface, inventory or review fix earns
no completion. Update the feature table in chat, not this PR. Pilot Q5 remains
pending operator input; no release/default completeness follows from this contract.

The exact first target selection is docs/migration/INSTALLATION_ROLE_MAP.json.
Its enumerated runtime/control rows are separate from its canonical inert source
assets and preserved user paths. Compile production selection from reviewed data;
the evaluator reads canon independently, never an implementation helper as oracle.
No unlisted hook, script, language-pack payload or source file may be installed.
Canonical init source data is not ordinary active code or an execution fallback.
Missing required canonical data refuses activation; language-pack initialization
uses its separately qualified source contract, not inferred product-pack selection.

Whole preview JSON uses actions, checks, conflicts, blockers and rollback_blockers
arrays, authentication string, applicable boolean and proposal_digest string.
Healthy read-only inspection returns 0 with a nonempty proposal digest and
complete action/preservation information. applicable/checks_ready/success and
rollback_ready stay false while consent, quiescence, fresh trust and activation
checks remain pending. Explicit local preview reports local-source; official
preview reports not-verified. Preserved mandatory conflicts return 2 with their
proposal; operational inspection failures return 1 without authority claims.

## Qualified implementation refinements

Select whole-installation routes before ordinary shared admission; never upgrade
a retained SH lease to EX. The fixed nonblocking lock order is ordinary root SH,
transition SH when activity is needed, then controller stores; installation root
EX, transition EX/recovery EX, then recovery stores. Physical installed-context
validation and ordinary admission precede both CLI and sourceable bridge dispatch.
An environment-selected bridge protocol is not an admission escape.

The inherited-stream mode shares the existing process ownership/cleanup engine
but directly binds caller file streams, keeps the caller session and owns a child
process group. For terminal invocation transfer and restore foreground ownership,
propagate stop/continue and termination behavior, and preserve signal/status
semantics. Qualify real PTY and non-TTY controls before relying on it. Preserve
existing public command ceilings; only cleanup remains separately bounded.

Initial legacy before-image evidence comes from the two immutable baseline
catalogs and reviewed embedded raw reference blobs, not a nonexistent historical
Go archive. Independently regenerate/check those raw blobs against their pinned
Git identities and canonical catalog digests. Apply the exact reviewed baseline
initializer transformations with current explicit inputs, then match the actual
current/saved before-image. Embedded reference bytes are inert recognition data,
never ordinary runtime fallback. Do not execute saved recovery scripts.

Legacy reverse health is profile-aware: validate the complete restored baseline
selection and control absences, qualify state semantics and execute only freshly
qualified reference/active legacy entrypoints in an isolated candidate or through
private owner-bound validation. It does not require a historical Go binary.
Independent legacy reader fixtures establish the shipped driver's compatibility
predicate; absence of required legacy runtime tools is an explicit blocker.

For forward upgrade --source and its identity/trust flags name the new after
archive. For rollback/recovery they name the known after archive whose assets must
match current transaction observations. Legacy --profile identifies the pinned
before reference. Later go-hybrid-v1 predecessors additionally use --before-source,
--before-version, --before-revision, --before-target and corresponding
--before-local or --before-attestation/--before-trusted-root/--before-gh inputs;
these never grant authority from a local descriptor. Reject conflicting aliases.

Explicit --project-input KEY=VALUE is repeatable with unique allowlisted keys:
PROJECT_NAME, PROJECT_SLUG, GITHUB_OWNER, OPENCODE_USERNAME, PROTECTED_PATH,
DOCS_ROOT and CITATION_PREFIX. Bind their exact literal bounded UTF-8 values to
the proposal. Unknown/duplicate keys refuse; absent inputs needed by a selected
baseline transformation preserve/conflict rather than inventing defaults.
Proposal JSON names its confirmation proposal_digest; row fields include path,
source_path, action, reason, required, before and after. All success/check and
rollback-readiness fields remain false until actual qualified completion.

## Whole journal observation protocol

Use .factory/installation-transactions/ID.json for the separately bounded whole
journal. The namespace is private 0700 and each record 0600, confined, regular,
single-link and strict JSON. It is not an ordinary asset role. Initial fields are
schema_version=1, kind=installation-transaction, migration_id, purpose, direction,
phase, profile, target, plan_digest, entries, checks and outcome. Each entry names
path, action, phase, before, expected_after and observed_after. Check records name
the prescribed check, phase, actual process PID when known, outcome and ownership
uncertainty. Refuse unknown/duplicate/wrong-type fields and unsupported phases.
Records remain descriptive evidence, never independent restoration authority.

Write the atomic checked whole backup manifest at
.factory/backups/ID/installation.manifest; regular saved before-images live under
.factory/backups/ID/assets/PATH with inert modes and separate original metadata.
Reject preoccupied old-format or foreign sets rather than overwriting/converting
them. Whole namespace/set caps are sixteen bounded operations; old R1/R2 count
and storage rules remain unchanged. Same-ID reuse requires full fresh equality.

Interrupted diagnostics and JSON reports expose recovery_command and
recovery_arguments for the exact ID and direction. Arguments start with factory
upgrade --installation --recover ID --direction DIRECTION --dry-run plus the
freshly selected source/profile/identity operands. Report required trust and
confirmation operands without serializing credentials or inventing consent.
The command produces a new proposal; it does not automatically finish work.

## Constructor and operand absence refinements

A healthy root need not already have .factory. Qualify its absence directly with
the native named no-follow observation under the root lease, preserving the actual
ENOENT for this branch. Record and revalidate absence at grant and later Check;
if state exists, retain its identity and recheck pending absence. Do not change
the old normal Guard's public error classification merely to recover a lost errno.
Read-only grant/closure must still create no file or directory.

Distinguish an absent scalar flag from a supplied empty operand using Cobra's
Changed information. Present rollback/recover IDs, migration IDs, direction and
confirmation must satisfy their grammar, even when empty. Select mutual-exclusive
modes by flag presence; malformed operands cannot become a forward request.
These checks precede source inspection or any state change.

Count actual flag occurrences according to declared Cobra flag types. A string
operand beginning with -- remains data; split/attached representations must be
equivalent. Reject repeated scalar options without counting their operands as
options. Keep repeated unique project-input entries supported.

Whole journal checks use name, phase, pid, outcome and ownership_unconfirmed;
pid is null before launch and a positive actual PID after onSpawn. Unknown work
remains explicit even when no PID was durably recorded. Names are prescribed
factory checks, not caller-selected arbitrary commands.

## Installed descriptor and parent confinement

The strict installed descriptor has exactly schema_version=1, kind=go-hybrid-v1,
version, revision, target, runtime_sha256 and assets. Asset entries bind path, full
numeric mode, sha256 and bytes. Exclude installation.current and .factory-version
from their own digest list; both are separately checked controlled metadata.
Include runtime.manifest/source.json and every selected active/canonical data row.
Derive the physical installation from the executable's fixed bin location and
validate the literal launcher identity, never an environment/PATH locator.

Share one small confined-parent implementation between installation observation,
publication and installed layout. Open every parent component no-follow beneath
retained descriptors, verify owner/type/mode and named/opened identities, and
retain/revalidate the chain before each leaf or absence operation. Lstat followed
by root-relative OpenFile with leaf O_NOFOLLOW is insufficient: a replaced internal
symlink parent can redirect the observation. Keep checked closure and native error
causes; actual I/O/cancellation cannot be relabeled as a preserved conflict.

## Installed gate modes and proof reuse

The retained hook-existence gate must accept the installed sourceable config.sh
library at its declared mode 0644. Require that library to be a readable regular
file and tracked when checking a Git checkout; require invoked hooks, including
configured local hooks, to be readable regular executable files and tracked.
The native proof must use the installed library mode without changing it to
simulate a legacy layout. Independently qualify the non-executable library and
missing/non-executable hook controls before changing the retained gate.

The transaction may retain a private in-memory proof of the exact candidate
descriptor, runtime and every selected asset after executing all gate controls.
Post-apply validation must independently reread the active descriptor and every
selected asset's bytes, mode and identity under the same owner-held exclusion,
and check actual project state compatibility. Matching the complete proven
selection avoids repeating identical isolated gate fixtures; a mismatch refuses
completion. A persisted journal, environment value or caller-supplied receipt
cannot substitute for this private proof or the active-root validation.

## Preparation failure and final cleanup recovery

Before-images and pending need not exist during isolated candidate checks. On a
known candidate failure with confirmed owned child/group exit and no active asset
change, explicitly discard only the exact newly owned preparation journal after
checked identity/parent revalidation and durable unlink. Preserve existing files
and the Git index. Report uncertainty if that cleanup cannot be established; Close
must not silently undo active work. A fresh same-ID request may then be proposed.

Interrupted preparation is distinct from partial application. Fresh recovery must
qualify the source, complete pinned baseline projection, journal consistency,
current unchanged before selections and state, with explicit consent/quiescence.
It cannot require a backup that preparation had not yet created. Unknown child
ownership remains explicit; no automatic restart or authority from journal bytes.
Manual reverse recovery may abandon only qualified, unchanged preparation.

The terminal journal is written before pending cleanup. A committed record with
the exact owned pending marker still present is a recoverable cleanup phase.
Fresh direction-consistent recovery must requalify the entire terminal selection,
current state, trust and consent before checked pending unlink and parent sync.
Do not replay asset changes, execute backups, create another before-image set or
treat committed journal bytes alone as a cleanup grant. Missing/foreign/changed
pending entries and changed active assets remain explicitly classified.

## Conditional initializer source data

Include README.md as explicitly selected inert canonical source data at
.factory/assets/scaffold/README.md, mode 0600. Native no-pack initialization needs
this source when the new application's README is absent. Preserve an existing
application README; this adds no active application-file replacement or product
language-pack selection. The reviewed map now has 51 runtime/control roles and
85 canonical source assets. Independently qualify empty and existing-README
initialization through the installed public command before crediting this closure.

## Whole-operation evaluator allowance

A real complete v0.1.6 application on the qualified native host took 14.806
seconds: candidate checks completed at 9.522 seconds, all rows at 13.922 and
pending cleanup at 14.758. The evaluator's generic ten-second subprocess ceiling
therefore expires during valid preparation. Use a finite sixty-second allowance
for explicit whole-installation mutation requests in the acceptance harness.
Keep read-only/malformed surface checks at their existing ceiling, all behavioral
assertions unchanged, native interruption witnesses explicit and production check
and cleanup allowances unchanged. This is a measured harness correction, not a
public timeout increase or permission to omit durable checks/publications.

## Installed upgrade aliases

An installed generated upgrade adapter cannot fall back to itself. For the
installed runtime only, bare upgrade, upgrade --help and upgrade -h show the
native whole-installation help without launching an adapter. Unmarked unsupported
mutation operands return 2 with guidance to use --installation. Existing explicit
whole routes remain unchanged; unmarked source-mode legacy preview/recovery and
dispatch contracts remain unchanged. Qualify public and direct-adapter aliases
with an independently bounded recursion witness before correcting dispatch.

## Pending observation and preparation reconciliation

The new unreleased whole journal also carries pending, null before creation and
then the full actual observed pending-file image/identity after owned publication.
Fresh terminal cleanup requires continuity with that observation as a necessary
precondition alongside freshly authenticated source, complete terminal selection,
state and consent under exclusion. The tuple or journal alone is not authority.
Preserve a replaced, changed or foreign pending entry rather than unlinking it.
This field does not alter released R1/R2 journals or their readers.

Interrupted preparation without pending uses an existing-state-only preparation
exclusion: retain the permanent transition/root locks, validate the real absence,
fresh complete baseline projection, no applied rows, source and state. Explicit
reverse recovery may abandon only that unchanged preparation evidence after
reconciled owned checks; it never invents a backup or auto-restarts unknown work.

## Unresolved ownership and control identity qualification

Introduce only a private per-operation native-command collaborator for regression
qualification, with empty operations calling the existing observed native runner.
No public flag, environment bypass or shipping test export selects it. An
independent evaluator must launch the real fixture-owned child and then report
exit-observation uncertainty; qualify preservation and admission before correction.

Close must preserve candidate scratch while owned child/group exit is unresolved.
Keep durable admission evidence for unresolved preparation before releasing
exclusion; do not present journal-only uncertainty as ordinary-work readiness.
If publication of that evidence fails, retain the primary uncertainty and report
that maintenance must remain in force. Do not execute or automatically delete the
retained candidate during recovery. Existing resolved preparation cleanup remains
unchanged.

The active installation.current descriptor has a separate retained publication
or preservation observation because it cannot hash itself in its asset list.
Post-apply proof must check that native identity as well as its complete bytes.
Independently qualify a genuine same-byte replacement beside its unchanged healthy
control; preserve the foreign entry and pending evidence on refusal.

## Prepared publication continuity

Each changed file with an after image records prepared_after before its active
rename: the actual checked staged file's bytes, full mode and native identity,
bound to the freshly qualified logical target. Keep this observation separate
from expected_after and observed_after. It is null before staging and never
grants origin or mutation authority by itself. Reuse the checked filepublish
stage and publish only after its whole-journal intent is durable.

Fresh interrupted recovery may reconcile publication before observed_after was
saved only when the current file matches the known prepared object and fresh
target. Compare stable native object identity, owner, mode, link count, size and
modification time; rename may change status-change time, so that field cannot
stand alone as a pre-rename/post-rename equality test. A saved observed_after still
requires full identity equality. Foreign equal-byte inodes, altered current
content/mode and unresolved unqualified intent remain preserved conflicts.
Do not weaken the existing multiple-row SIGKILL or foreign-replacement criteria.

## Source qualification scheduling

The current unpartitioned acceptance run exhausted its 25-minute package allowance.
The selected 76 installation/runtime/hook criteria alone take about 21 minutes.
Separate source qualification into base and installation acceptance groups while
retaining every criterion, race checks and the existing 25-minute group ceiling.
Base runs all Go packages except the three named installation acceptance families;
installation runs those families in acceptance. Default local qualification runs
both groups, failing if either fails. Unknown group choices refuse before tests.
Keep build, dialect, vet, lint and security checks. Preserve existing OS-matrix
Go-runtime check names in CI for the base group and add parallel installation
checks on the same OS matrix; do not rename existing required checks or omit any
platform. This is test scheduling, not a runtime or model deadline increase.

GO_RUNTIME_TEST_GROUP accepts exactly all, base or installation and defaults to
all. The installation-family expression is anchored to Whole installation upgrade,
Installed runtime root separation, or Installed hook-existence library and
invocation modes. Base uses that expression with Ginkgo skip; installation uses
the identical expression with Ginkgo focus. All attempts both partitions even
when one fails, then returns failure if either failed. A valid selection cannot
silently omit the other partition from the default local source qualification.

Ginkgo matches the suite description followed by the spec text. The actual
acceptance suite prefix is Go factory command acceptance. The shared anchored
expression must include that prefix before the three families; verify selection
with the actual Ginkgo dry-run as well as the recording fixture. Installation
must select all 76 current family criteria and base must select their complement,
with no overlap or zero-selection success. Keep focus/skip expressions identical.
The installation invocation also uses Ginkgo fail-on-empty, supported by the
currently pinned Ginkgo flag definition, so an empty selection fails the check.
