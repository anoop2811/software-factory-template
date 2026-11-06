# ADR 0138: native source check and pre-push orchestration

Status: accepted implementation boundary; qualification and merge pending.
Decision: 153. Implements the next one-path R3 group after PR #168.

## Context

The native Cobra `check` command already owns the eight-stage controller in
`internal/checkcmd`. The direct source `scripts/pre-push-check.sh` still duplicates
that orchestration. The approved conversion keeps validation hooks and adopter
build tools as explicit boundaries; it replaces their factory controller.

## Direct source owner

Replace the source command with a thin executable adapter to its physical
checkout's `factory check`. Resolve the script directory physically without
changing caller cwd. Preserve spaces, quotes, directory aliases and newline
paths. Refuse a final-file symlink; a missing or failing owner must never select
another installation, PATH factory, saved script or historical implementation.
Do not invoke a hook, configuration reader or compiler just to locate the owner.
The established source launcher owns any required development compilation.
Use validated native Bash source-array identity, never inherited scalar metadata.

Forward literal arguments and inherited streams/environment to the native
command. Git supplies a remote name and location to pre-push; the existing Go
check owner ignores these operands, as the old controller did. It must not parse
shell syntax or reinterpret remote operands as configuration or check flags.
No public CLI or Go dependency is added.

## Copied source Git hook

The source command documents copying it as `.git/hooks/pre-push`. Preserve that
usage explicitly. An adapter invoked with basename `pre-push` is the copied hook
form: select its caller project's factory using nonempty `REPO_ROOT`, otherwise
successful caller Git-root discovery, otherwise the caller directory, keeping
each admitted runtime bound to the native-selected project. Resolve relative paths from
the original cwd; preserve the original cwd for the eventual native invocation.
This is a distinct invocation form, not a fallback when a direct physical owner
is missing. A selected root without an executable factory refuses. Do not fall
back to the source tree that originally supplied the copied file.

Prove the copied form through a real isolated Git push as well as direct
invocation with literal remote operands. Keep `.githooks/pre-push` unchanged:
its tracked contract remains direct-main denial only.

## Native contract and custody

Reuse the existing Go owner without rewriting its stage logic: direct-main gate
first and fail-fast with actual stdin; make check and drift; all unpushed commit
lint; diff-aware; decision-log; pending lessons; then local hooks. Preserve
ordinary failure aggregation, missing/empty origin ranges, caller role presence,
explicit project/config selection and literal local-hook normalization. Fatal
cancellation or uncertain ownership stops later stages without another owner.

Existing native/oracle check criteria remain at their immutable baseline. The
four historical development materializers must also pin pre-push to their
existing 1754de2 entrypoint revision, preventing an old factory dispatcher from
recursing through a new source adapter. Retain current policy-gate subjects and
all historical assertions. Native-init and transition archives already retain
the old pre-push body and need no replacement. No installed recipe bytes change.

## Independent qualification

Write outside-in Ginkgo/Gomega criteria before implementation. Observe RED
against the old source controller, then GREEN against the new adapter. Cover
physical and copied ownership, literal argv/environment/stdin/status, missing
owner and compiler refusal without fallback, actual native stage behavior and
failure accounting, real direct-main denial, local-hook operands, and owned
cancellation. Reuse existing fixtures and native check tests where they already
prove the behavior; new source tests must witness the new entrypoint itself.

Run an actual source compilation only in a disposable complete checkout. Other
focused checks use privately compiled runtimes. Keep the operator's primary
`factory-go` and unrelated files unchanged. Run enforcement hooks from disposable
checkouts too: an allowing hook can still invoke native configuration first.
Record the actual partition census, focused native/source checks, affected
historical suites, full source gate and exact-head hosted review/CI separately.
Do not increase deadlines or weaken assertions merely to make a run pass.

## Separate installed-copy release obligation

Inspection found a pre-existing gap in the generated installed adapter: copying
it to `.git/hooks/pre-push` resolves `.git/factory` because the generator assumes
its installed `scripts/` location. Current initializer text still recommends
that copy. This group must not silently rewrite frozen generation recipes to
repair it. R4 must supply and qualify an installed-safe full-check hook setup,
preserve earlier-generation reconstruction/upgrade behavior, and correct the
initializer advice before stable 0.2.0. Existing in-place installed adapters
remain unchanged; source copied-hook evidence does not qualify that separate
installed boundary.

## Progress

PR #168 merged on 2026-11-06 UTC; R3 now has 24/32 paths merged (75%). This
single-path group reaches 25/32 (78.125%) only after qualification and merge.
Budget, loop/Python and public selftest remain three subsequent groups; R4-R7
release gates remain separate from source cleanup.

## Copied-root normalization clarification

The native check joins relative REPO_ROOT to its cwd lexically, but leaves an
absolute REPO_ROOT for filesystem resolution (`internal/checkcmd/command.go:224`).
For copied-hook runtime selection, use logical shell directory traversal for
relative roots and physical traversal for absolute roots, then read the physical
directory. Preserve the raw caller variable for Go and its child gates. A
relative `alias/..` must not execute the symlink target's factory while the Go
command checks the caller's project. Absolute selectors follow the existing Go
owner's filesystem semantics rather than the old shell controller's logical cd.

Retain conservative shell-path admission: relative `absent/..`, `dangling/..` or
`file/..` may be rejected before handoff even though Go's lexical join would
eliminate those components. The old source shell also refused them. This avoids
duplicating a path-cleaning algorithm in the adapter. A rejected selector must
not fall back to another owner. Independent tests must cover accepted relative
and absolute symlink/parent cases and refusal before any runtime is invoked.

## Bash script identity clarification

For `bash pre-push-check.sh`, Bash may find a readable, nonexecutable file on
PATH while leaving `$0` as a bare filename. Executable-only PATH lookup can
instead select a later, different file. Reuse the indexed-array declaration
guard already qualified for the source libraries, then take `BASH_SOURCE[0]`
as the script identity before selecting direct or copied mode. An inherited
scalar, including an empty value or declaration-shaped text, must refuse with
status two and a diagnostic before an owner runs. Do not repair caller metadata
or duplicate Bash's script lookup in shell code. Cover readable PATH-selected
scripts, relative PATH entries and malformed metadata in the existing criteria.

## Native Git-root output delimiter correction

Independent review reproduced a copied adapter selecting `project\n/factory`
while the native checker selected sibling `project`: the adapter preserves the
Git root's path bytes, but `repositoryRoot` removes every trailing newline.
Remove only Git's single output newline in the native root reader. This is a
bounded correction to the same command's project-selection boundary; it does
not change stage orchestration, installed generator recipes or wrapper owners.

An independent native/source regression must use an actual repository ending
in a newline and a distinguishable sibling without that newline. The copied
hook must execute and validate only the newline repository. Observe failure
before the Go correction and success afterward, retaining raw caller environment
and no fallback. Re-run the existing native check criteria with the source group.
