# ADR 0108: prepare default Go entrypoints and retire duplicate source implementations

Status: implementation contract under Decision 118; release activation remains gated

Date: 2026-10-28

## Context

The installed Go factory already supplies native commands and a checked whole-
installation migration. Its source launcher, ordinary source initializer and bare
installer still select Bash implementations. The selected initializer also keeps
an inert source scaffold containing obsolete controllers. Replacing a launcher
without changing that initializer would publish a compiler-dependent or incomplete
installation. Deleting the scaffold without recipe qualification could break an
earlier Go installation. Complete these connected changes before one RC4.

The approved specification remains authoritative: stable paths and literal
arguments (specs/001-go-runtime-conversion.md:93), fetch-only installer consent
(specs/001-go-runtime-conversion.md:251), native Cobra routing
(specs/001-go-runtime-conversion.md:334), revised platform scope and pilot criteria
(specs/001-go-runtime-conversion.md:481). No policy gate or user-authored program
is converted merely because it uses a shell.

## Release selection and activation

Prepare one literal immutable Go release pin, `v0.2.0`. Ordinary source init and
ordinary upgrade select that declared Go target through the existing release
acquisition engine. An installed initializer may select its validated installed
release version when creating another project. `--ref TAG` selects an exact Go
release through the same authenticated path; `--go-candidate TAG` retains its
existing explicit prerelease contract. Missing, malformed, conflicting or
unavailable selections refuse. Do not infer another release from asset presence,
resolve moving aliases, silently downgrade, or fall back to scripts after failure.

Keep the bare installer's active stable pin at `v0.1.6` while a qualified Go
stable release does not exist. Prepare and independently exercise the Go-pin
branch now. Activating the public stable pin is an explicit final release step,
after published qualification and operator acceptance; prepared code does not
claim that activation occurred. Existing explicit installer source/ref selection
of the frozen Bash release retains its historical implementation. It is not an
authentication fallback from a selected Go release.

Bare fetch continues to fetch only: no initializer execution, project mutation,
implicit consent or Git history changes. Its next-step instructions must route
normal users to the authenticated packaged initializer without requiring a Go
compiler. Every Go initializer and upgrade keeps the existing declared trusted
GitHub CLI/Bash/platform prerequisites; shipped factory execution needs neither
Go nor Python. Declared stable default selection rejects prereleases, while an
explicit candidate selection continues to permit them.

## Source development and explicit local operations

The repository's source `factory` becomes a small developer launcher for the Go
CLI. Building committed or edited factory source is a developer operation with
the documented Go toolchain; it is not the installed end-user launcher. Preserve
caller working directory, literal argv, inherited streams and exit/signal behavior.
Never treat an arbitrary colocated executable as an authenticated installation.

Explicit local source interfaces are:

```text
factory init [TARGET] --local [--source DIR] [existing initializer options]
factory upgrade --local --source DIR [--dry-run --json]
factory upgrade --local --source DIR --confirm DIGEST --quiescent
```

Local init defaults its source to the invoked developer checkout; local upgrade
requires its source explicitly. The new local-source route conflicts with release
selectors. An explicit `--installation` owns its existing low-level parser first;
its legitimate archive `--local` option remains supported and does not select
the new source-directory route. Package the committed source HEAD with the existing Go packager,
stage and inspect with explicit local trust, then use qualified native init or
the existing whole-installation proposal/transaction. Report `local-source`, the
full revision and local package identity. A dirty worktree is not silently
represented as a committed package. No network or paid model is needed for a
provisioned local package build.

Preserve existing explicit candidate, whole-installation, backup and recovery
selector ownership before generic defaults. Preserve the existing read-only
`upgrade --dry-run --source DIR` boundary without `--local`. Do not guess the
provenance of an earlier local Go image; its rich `--installation` and explicit
before-image interface remains available. Mutation still requires a fresh complete
proposal digest and maintained quiescence. Installed whole upgrades obtain the
exclusive route before shared installed admission, avoiding lock upgrades.

## Source and installed cleanup

Replace current full source controllers at supported script paths with thin Go
invocation/sourceable adapters and remove obsolete Python runtime implementations
after their native routes are established. Do not relocate a second complete
legacy runtime into a permanent compatibility directory. Immutable historical
reference catalogs and explicitly classified development/legacy proof fixtures
remain data or developer utilities; they do not supply ordinary runtime fallback.

Ordinary source and installed `selftest` use the canonical native gate proof.
Its retained script/Make entrypoint delegates without recursing into itself.
Historical full Bash proofs stay with the immutable legacy fixtures. Any retained
Python development proof must have an actual classified historical input; do not
leave source checks importing removed runtime modules or keep duplicate shipped
controllers only to satisfy an obsolete current-source oracle.

The canonical initializer inventory must contain only needed data and boundary
assets. Qualified init must not first require or copy the old Bash/Python scaffold
and then overwrite it. Retain native policy gates, user programs, harness host
formats and product toolchain boundaries. Update the copy/inventory proof so an
empty shell-copy scan cannot vacuously claim complete initializer coverage.

The private source-inventory interface is
`FACTORY_BRIDGE_PROTOCOL=1 factory inventory init ROOT`. It shares the native
initializer's canonical inventory, checks actual regular files and tracked source
membership, and reports a positive asset count on success. Missing, nonregular or
untracked required assets fail with the affected path; malformed requests return
usage status 2. The source copy-manifest hook delegates to this proof rather than
parsing Go source or accepting an empty Bash-copy scan. Existing installed custody
and legacy-only inventory boundaries remain separately classified.

Current supported predecessors keep strict byte/mode ownership and history
checks. Recognized obsolete implementations enter private Git-ignored inert
recovery before retirement; unknown, edited, held or unsafe entries remain
reported and preserved. A filename is not adoption authority. Keep the existing
immediately preceding recovery and release-transition retention rules.

Evolve the deterministic selection recipe explicitly if cleanup changes it.
Qualify the known earlier RC3 recipe and the new recipe independently; preserve
unknown-recipe refusals. Do not weaken the reference guard or execute code from
an old source archive to infer its selection. Use bounded reviewed recipe data
and exact qualification identities for reconstruction. Preserve the immutable
v0.1.6 and Bash-baseline recognition catalogs.

Use a bounded compiled prior-role table and an exact eight-file recipe fingerprint
from immutable RC3 source to reconstruct that known predecessor. Keep the declared
role path census stable: obsolete inert cache rows become optional retirements,
not arbitrary omitted paths. Current selection omits their bytes; an earlier
recognized selection is reconstructed with its prior roles and unchanged reviewed
generator semantics. The new recipe comparison includes its new recipe-selector
and prior-role-table source dependencies; the eight-file set identifies only the
frozen historical recipe. Unknown fingerprints remain refusals. Installed consumers
need only `templates/metrics.html` and `packs/review-lane/review-pr.yml` from the
old 85-row scaffold cache; the other 83 rows become optional retirements. The
initializer acquires a complete qualified release image or
explicitly packages local source, rather than republishing the old cached scaffold.

## Qualification and pilot

The approved future minimum evidence families are Ubuntu 24.04 on both targets,
macOS 15.x on Intel and macOS 14.x on Apple Silicon. A newer supported system
does not by itself prove execution at the minimum. Keep historical reports and
signed assets unchanged.

Independently authored outside-in Ginkgo/Gomega criteria must fail meaningfully
before production implementation. Exercise actual launcher/bootstrap flow,
literal/refused selectors, private authentication, both supported legacy
predecessors, known Go recipe transitions, preserved customizations/state,
interruption/recovery, compatible rollback, exact retired/retained inventories,
ignored inert backups and forbidden compiler/interpreter controls. Development
build tools and freshly reconstructed historical rollback health are separately
classified prerequisites. No paid model is required by these checks.

Run a reversible representative development cycle in an isolated supported-
project copy with the complete local package. Record identities, commands,
outcomes, source preservation and cleanup; keep private adopter data local.
No additional calendar waiting period is required. Local success is not publisher
authentication or operator acceptance. After the reviewed change is merged,
publish one RC4, run its actual four-target attested qualification and obtain
final operator acceptance before activating the stable Go default. No release
gate closes merely because its preparatory code or test was written.

## Preserved release environment precedence

For ordinary release init/upgrade, an explicit `--ref TAG` wins over the caller's
nonempty `FACTORY_REF`, which wins over the declared default. Without a caller
selection, installed init may reuse its validated installed version. All selected
release operands remain exact and satisfy the stable/candidate policy above.
Explicit candidate, local-source, whole-installation, backup and recovery routes
keep their own parsing and are not stolen by an ambient `FACTORY_REF`. This
preserves the historical release environment boundary rather than adding a new
mutable release channel or implicit fallback.

## Developer build cancellation

The source launcher owns its compiler until exit is confirmed. Parent-only INT,
TERM and HUP must interrupt a blocked build, forward cancellation to its owned
builder group, and finish bounded cleanup before exit. Remove the owned temporary
build result and preserve the previous developer executable on cancellation.
Return conventional signal statuses: INT 130, TERM 143 and HUP 129. A foreground
child that delays Bash trap handling does not satisfy this boundary. Use a real
owned compiler handshake in independent signal regressions; elapsed time alone
does not establish child exit or reap.

## Historical initializer component evidence

Keep the existing 85 initializer prompt/data/publication/TTY/ownership cases as
explicit historical-source component qualification. Their exact immutable Bash
oracle remains unchanged. A cached, guarded test-only executable may call the
current native initializer core with the frozen historical asset declaration,
preserving its unsafe-path, mutation and child-process assertions. The driver
lives only in `*_test.go`, preserves requested race instrumentation, and changes
no ordinary production inventory or public routing. It is not a shipping selector,
environment bypass, or evidence that ordinary Go init emits a Bash scaffold.

Current authenticated/local Go delivery is qualified separately through the real
public CLI, compiled images, defaults, migration/cache/recipe, retention and source
inventory cases. Do not delete or skip the old component cases or restore removed
runtime modules to the active source to satisfy them. Read historical fixture
programs only from their explicitly pinned immutable test inputs.

Remove the unreachable old script-dispatch fallback after every registered public
operation returns through its native owner. Keep sixteen operations plus help,
and fail closed if a registered operation lacks an owner. Intentional gate/product
program execution remains in the native orchestration packages, separate from
an alternative factory runtime.

## Executable local and stable advice

Local text previews name the already invoked Go executable by its absolute path,
retain the physical `--local --source DIR` operands, complete proposal digest,
quiescence and project inputs, and identify local-source trust. Do not assume a
global `factory` exists on PATH or invoke the still-legacy target's `./factory`.
Printed recovery advice likewise retains the private staged Go executable and
uses `--local` for local images, without empty publisher-verification operands.
Stable text advice preserves `--ref TAG`; an explicitly selected candidate keeps
`--go-candidate TAG`. Advice must not silently change selection intent between
preview, raw handoff and confirmation. Independent tests execute the printed
inspection/recovery commands as read-only operations before trusting the guidance.

## Developer proof cutover

Developer test runners must follow the selected runtime. Replace obsolete
budget/loop Python runner fixtures and the curl-specific review harness with
focused Ginkgo/Gomega commands. Keep the existing developer script/Make names;
a shared source-only helper selects fixed suite families, refuses unknown
selectors and fails if the factory module or required test sources are absent.
These developer commands require Go; they are not installed factory commands.

Record the old budget32, loop40 and review53 outcome coverage before retiring
their obsolete fixtures. Preserve applicable admission, ownership, ordering,
privacy and transport outcomes in native tests. Add direct evidence for gaps;
streaming-client tests do not qualify the separate provider-JSON client, and a
pre-admission process death does not prove a charged reservation survives death.
Curl argv/temporary-file mechanisms and Python syscall injection remain historical
implementation evidence, not mandatory mechanisms of the native controller.
Historical Python guard tests continue to read exact pinned Git blobs in private
test directories; no runtime module is restored to the active source.

The aggregate Linux/macOS Go base qualification already runs these native
packages and acceptance families. Remove redundant legacy budget/loop workflow
jobs and obsolete extra fixture steps from template CI rather than executing the
whole native suite repeatedly. Retain every Go partition, race check and quality
stage. Install the existing qualified Go toolchain before template source gates;
this is source-development setup, not an adopter dependency. Optional-pack and
Java developer proof entrypoints likewise select the existing native gate and
initializer artifact criteria, with their historical component scope explicit.
Do not present historical assets as qualification of a newly packaged release.

Adapter path resolution preserves physical directory bytes, including trailing
newlines where the existing installed-path contract permits them. Shell command
substitution must not silently select a sibling root by trimming those bytes.
Keep explicit release/source operand representation refusals unchanged.

When this correction changes generated adapter bytes, preserve the exact compiled
historical root-prelude serialization for the already qualified RC3 recipe. Only
the complete eight-hash historical identity may select that bounded serialization;
unknown recipes still refuse. Current generation uses the corrected prelude and
the current dependency guard. Re-run actual RC3 reconstruction, cache retirement
and rollback criteria after the generator change. Never execute a historical
archive's generator or infer compatibility from its version label alone.

## Evaluator executable custody

The source-launcher signal fixture re-executes the instrumented acceptance binary
as its controlled compiler. Capture an identical executable in the suite's owned
temporary directory before nested Ginkgo compilation/cleanup can unlink the
workspace pathname. On Linux, read the running kernel image through
`/proc/self/exe`; on other supported systems capture the initial executable path.
Preserve the exact instrumentation and helper program, executable mode and
suite-owned cleanup. Record image identity and retain all real handshake,
signal-status, reap and prior-result preservation assertions. This is test-only
custody, not a retry, skipped signal check or production behavior change.
