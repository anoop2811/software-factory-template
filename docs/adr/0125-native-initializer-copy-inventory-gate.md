# ADR 0125: prove the native initializer copy inventory before source activation

Status: accepted bounded R2 prerequisite; independent public RED precedes code

Date: 2026-11-01 UTC

## Complete function boundary

After the human merge of PR #155 as `14c0d319a3bab6c0976bee334ae70a573e4d2501`,
move the copy-manifest gate's native authority to the asset inventory actually
used by the Go initializer. This is one independently consumable prerequisite
to the source launcher and direct init/upgrade adapters in ADR 0114's item 8.
Do not activate those entrypoints in this PR, change the public v0.1.6 pin or
publish a release. R2 remains partial until its coherent source/direct function.

The current hook extracts Bash copy statements from `scripts/factory-init.sh`.
A legitimate empty grep currently passes. Replacing that implementation with an
adapter would therefore leave a passing check without an inspected inventory.
The native gate must remain meaningful before that source conversion occurs.

The current consumed initializer-cache data are metrics HTML and review-workflow
YAML. Cached initializer/prerequisite bodies are retirement rows, not current
Go initializer inputs. Preserve current and historical installation recipes,
role declarations, generators and the two consumed data assets.

## One inventory authority

Extract the Go initializer's fixed required and optional source paths, its copy
groups and conditional README input into a small neutral internal Go package.
Both initialization and validation consume those same definitions. Preserve
initialization's file selection, ordering after sorting, substitutions, modes,
language-pack handling and generated installation overlay. Do not duplicate a
second path list or parse Go source text in a shell hook.

The gate checks all fixed required source inputs, README as a possible new-target
input, present optional inputs and actual members of the declared base copy
groups. Required groups cannot be empty. Missing, unsafe or untracked required
inputs refuse; present optional or group inputs must also be safe and tracked.
Pack-specific selection remains the existing initializer's preflight concern;
do not claim this base inventory check validates every possible pack overlay.

Resolve paths beneath the explicitly selected subject root. Treat names as
literal data, including whitespace and shell metacharacters; do not follow
symlinks into outside referents or execute project configuration. Use the
existing bounded native process runner for Git, with literal pathspec handling,
finite output/time bounds and its owned-child cancellation. Never shell-evaluate
an emitted path. The native checker must not alter project bytes, Git index or
runtime. The hook retains only its existing gate-event instrumentation on refusal;
qualification names that expected log separately rather than hiding other writes.

## Existing runtime and private protocol

Expose the check through the existing factory executable's private Cobra bridge,
not another CLI binary or an automatic compiler/download path. Keep the public
sixteen-command admission and all existing bridge requests unchanged.

The hook uses an explicitly selected absolute executable regular file from the
existing `FACTORY_RUNTIME_BINARY` developer contract when that variable is set.
Empty, relative, missing and non-executable explicit selections refuse; no native
validation failure falls back to the legacy scanner or another runtime.

For a Go installation, invoke the physical hook owner's declared `factory` when
its selector is present, including unsafe types. A subject never selects code.
Physical installed admission precedes private bridge work. Broken installation
refuses; a foreign subject's selector enables neither execution nor fallback.

A validated installed runtime checking its own physical installation reports the
qualified installed inventory boundary truthfully: source copy inputs are not
resolved from the adopter's application tree. That result requires physical
runtime/installation authority, not a caller-supplied environment marker. An
explicit source subject elsewhere still receives the actual source check.
Preserve installed leases and keep ordinary users compiler/Python independent.

## Transitional legacy behavior

Without an explicit runtime or installation selector, retain the existing Bash
copy scanner for genuine legacy source initializers. Keep its literal Git
tracking and guarded-copy behavior. A missing initializer retains its documented
legacy-adopter skip. An existing initializer with no recognized copy inventory
must refuse clearly; an unknown or empty inventory cannot earn success.

Keep the source launcher, direct init/upgrade scripts and legacy component proof
fixtures unchanged in this slice. Their later conversion must explicitly address
current-copy fixtures and branch installer forwarding; it must not silently
replace a requested branch with the native release default.

## Non-vacuous native gate proof

Update the native source/installed gate-proof owner's copy-manifest pair to
exercise a real declared Go input. Seed its private fixture from the shared
inventory before the fixture's initial Git commit. Leave one existing required
input untracked for the violation; track that same input for the healthy control.
The gate must refuse the first and accept the second through the real compiled
private command and hook, with no compiler or Python invocation.

Do not prove only synthetic Bash copy text after the native gate is introduced.
Preserve all fourteen gate pairs, actual running-executable custody, isolated
configuration, bounded process cleanup, cancellation, and uncertain-ownership
retention. Share inventory metadata without creating the initcmd/installedproof
import cycle. Keep legacy hook proofs separately meaningful.

## Independent qualification

Under specs/001-go-runtime-conversion.md:287, an independent author first observes
the real hook or compiled command's missing native behavior against unchanged
main. A native-shaped initializer with an actual declared untracked Go input
must not obtain an empty-scanner success. A missing private command alone is not
the only evidence: retain the hook's concrete false-success result and its
unmodified project/index snapshots before repair.

Qualify native untracked/tracked controls, missing and unsafe inputs, required
group emptiness, present optional/group tracking, literal filenames, explicit
runtime refusal/no fallback, retained legacy scanner/skip, and installed physical
admission. Reuse existing native source/selftest and real local/default init
journeys to prove metadata extraction preserves generated compiler-free installs.
Do not claim the new public source launcher has been activated by those tests.

Derive the actual registry identities and disjoint lane census after adding the
bounded criteria. Preserve every prior criterion and meaningful assertion. Use
Ginkgo/Gomega, race instrumentation and the configured Go quality tools; measure
new family cost before choosing its existing lane. Preserve deadlines, workers,
model/budget settings, artifacts and all published predecessor recipes. Fresh
head CI, model review, findings replies and human merge precede the next slice.

## Private execution and proof custody

The literal private request is `helper copies ROOT` with
`FACTORY_BRIDGE_PROTOCOL=1`. The hook replaces itself with the selected runtime,
so parent-directed termination reaches the Go signal-aware owner rather than
leaving an unobserved foreground child. A hook-only event marker requests the
existing best-effort refusal log; direct private checking remains read-only.
That marker grants no installed authority or validation exemption.

Main passes an unexported context capability into the bridge only after physical
installed admission succeeds, reusing its retained lease. The source bridge
cannot manufacture that capability from arguments or environment. Compare the
subject's physical root with that admitted root before reporting installed scope.

Use the existing confined installation filesystem for non-following input and
parent inspection, with its regular-file, ownership and mode checks. Bound group
enumeration to 4096 entries and input size to the initializer's existing 16 MiB.
Execute one literal Git tracking request with a five-second allowance, clear
caller Git redirection/configuration variables, disable fsmonitor and optional
index writes, and keep native output/owned-process bounds. Neither configuration
evaluation nor diagnostic printing may execute source text.

Reserve status 125 for unconfirmed native child ownership. The outer proof maps
that status back to the typed ownership error before evaluating cancellation;
its scratch must remain available rather than being deleted while nested Git
ownership is uncertain. This status is never accepted as an ordinary violation.

The current hook carries an explicit native-inventory proof-protocol header.
Select the real Go-input pair only for that schema; preserved legacy hook bytes
retain their meaningful Bash-copy pair. This selects test data, never execution
authority. For the native pair, override any ambient runtime selector with the
already snapshotted physical proof driver before the first hook invocation.

## Review qualification before delivery

Literal Git pathspecs still select indexed descendants. Ask for NUL-delimited
names and require exact returned-name membership for every input, not merely a
successful Git exit. Qualify an indexed directory replaced in the worktree by a
safe regular input that is itself untracked, then its exactly tracked control.

Resolve installed runtime selection from the physical hook-owner directory,
independently of the checked subject. A source-owned hook cannot execute a
foreign subject's `factory` just because that subject plants a selector. An
admitted hook-owner installation can still inspect a foreign source inventory.
Qualify that refusal with a foreign executable sentinel before changing routing.

The current native copy hook is itself a managed Go controller whose Git child
uses a separate process group. Its outer proof must request cooperative shutdown
through the existing native supervisor rather than immediately killing that
controller. Expose the same bounded cooperative option for captured command
execution; select it only for this known native copy proof. Preserve existing
legacy hook execution and all cleanup deadlines. A held-inner-Git parent-only
cancellation regression must demonstrate actual group closure or retained exact
scratch on unacknowledged shutdown before any test teardown forces cleanup.

Preserve literal Unix backslashes in group-member filenames, as the initializer
already does. Keep canonical manifest-path validation unchanged: resolve the
known inventory parent through the confined tree, then open one validated literal
leaf through the same existing file-qualification implementation. The leaf API
rejects separators, NUL, empty names and dot/dot-dot; it does not add a second
filesystem validator. Independently qualify the tracked literal name before
changing this read-only helper boundary.

## Hosted review refinements

Re-enumerate the bounded groups and optional-input presence after Git completes.
Refuse if the sorted selected inventory differs from the inventory sent to Git;
checking only the original paths cannot detect a newly created copy input. The
regression owns and separately records its controlled actor mutation during the
Git handshake; it does not waive other project, index or runtime changes.

Reject a symlinked hook file before loading sibling libraries. Resolve its parent
directory physically before traversing to the owner root, so a symlinked scripts
directory cannot make a foreign installation select executable code. Qualify
both alias forms, including a hostile sibling-library sentinel.

Apply exact NUL-delimited filename membership to the retained legacy Git scanner
as well. Drain the pipeline without process substitution or temporary files;
preserve guarded-copy and no-initializer behavior. Its indexed-descendant case
must fail before this repair, just as the native case did.

Keep diagnostic causes useful and bounded: distinguish missing, unsafe,
permission-denied and canceled inputs using safe sentinel errors, preserving
their error identity without printing raw path-bearing filesystem errors.

The source selftest's existing native-outcome overlay is a real consumer of the
private captured-command helper. Adapt its invocation to the new runOptions
argument without changing any fault or success assertion. The old invocation's
compile failure was observed locally and in both hosted base lanes; this is
fixture compatibility, not evidence that its behavioral assertions passed.

Preserve the legacy scanner's documented runtime-glob exclusion, including a
glob appended outside the quoted source-directory prefix. Those prefixes are
not literal file inputs; exact filename matching must not reject the current
initializer's three group-copy statements. Recognize the suffix as literal data,
without evaluating it. The native gate still enumerates and validates each group.

## Narrow ownership protocol and refusal diagnostics

Only the known native copy-controller boundary interprets exit status 125 as
ownership uncertainty. An unexpected 125 from another proof command remains an
ordinary failed expectation; it must not retain scratch solely because of that
number. Real typed ownership errors still retain evidence from every command.
Place the shared status beside native ownership errors, removing the proof's
dependency on the checker package for an integer.

Apply the existing bounded cause handling to subject-root resolution failures as
well. Separate finite Git timeout, untracked-input and other tool-failure
diagnostics without exposing raw subprocess output or changing the five-second
allowance. Before dispatching an owner installation, require its declared wrapper
to be an executable regular file; otherwise emit a clear refusal and the existing
gate event. Independent regressions precede these refinements.

Cooperative execution remains selected by the known native-copy fixture context
and the program-owned hook invocation, with the existing cleanup bound and typed
retention. Confined Parent resolves only a path's parent; the hypothetical final
leaf is neither opened nor created. Clarifying that existing API does not add a
new filesystem protocol or weaken canonical path admission.
