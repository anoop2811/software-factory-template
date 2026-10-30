# ADR 0110: supervise compiler processes through the native executor

Status: accepted

Date: 2026-10-30 UTC

## Context

The existing developer packager captures compiler version, module inspection
and build output through direct `exec.CommandContext` calls. Killing the compiler
alone does not close capture pipes inherited by a descendant. Earlier candidate
rehearsal observed this failure through an unmerged public local-init prototype.
The bounded repair also applies independently to the existing packaging API and
standalone Cobra command. This slice qualifies that already available boundary;
public local initialization remains a later function in ADR 0114's delivery plan.

## Contract

Use the existing native supervised process-group executor for all three compiler
operations. Preserve resolved literal compiler identity, argv, committed source,
filtered Go environment, offline policy and working directory. Preserve stdout
separately for module JSON; version/build may merge stderr as before. Bound
version output at 64 KiB, module JSON at 1 MiB and build diagnostics at the native
16 MiB ceiling. Module stderr is separately capped at 64 KiB. Overflow is a failure,
never a usable truncated version/module response or successful build.

Use the caller's context and the native executor's existing bounded cleanup.
Accept results only after successful process exit and confirmed group ownership.
Retain context cancellation and typed ownership uncertainty through wrapping;
never publish a bundle on cancellation, output overflow or uncertain cleanup.
If compiler ownership remains uncertain, retain its private workspace with an
actionable path diagnostic instead of deleting bytes a descendant may still use.
Normal confirmed cleanup removes owned temporary workspaces. Preserve user source
and never delete an existing output directory.

The standalone Cobra `factory-package` command must use `signal.NotifyContext`
for INT/TERM and execute with that context, so compiler group isolation does not
create a new orphan boundary. Complete deferred cleanup before process exit.
Preserve success, malformed-request and operational-failure exit behavior.
Reuse existing native supervision and add no public flag or external dependency.

## Independent qualification

An independent Ginkgo/Gomega author first observes meaningful pre-fix failure
through the existing packaging API and standalone CLI after an actual compiler
and descendant readiness handshake. Cover all three compiler stages, captured
stdout/stderr and a healthy real build through the same wrapper. Own fixture
processes and cleanup; a sleep alone is not readiness evidence.

Require bounded context cancellation, parent-only INT/TERM handling, no bundle
publication, unchanged committed source and actual descendant absence or typed
ownership uncertainty. A surviving process cannot count as confirmed cleanup.
Exercise version/module/build output bounds independently. Preserve existing
output directories and require actionable retained-workspace diagnostics when
the native executor cannot confirm ownership. Use its established evaluator-only
fault control for otherwise unobservable inspection failures.

The ten focused criteria comprise a healthy real build, three stage cancellation
cases, four capture bounds and two standalone signal cases. The two prototype
public-local-init cases from preserved source `07e323a` belong with that later
feature; do not introduce unavailable routes or skipped tests in this slice.

Discover the real acceptance registry and its disjoint existing CI partitions
after adding criteria. Keep race checks, source quality stages and hosted limits.
Fresh Linux/macOS checks and exact-head model review are required before human
merge. These requirements do not qualify release publication or stable activation.

## Source and toolchain provenance

The reference implementation is preserved in PR #141 at
`07e323a76bbaeb34b2ef12321d958b1539fa528d`; its broader historical qualification
does not substitute for this focused main-based run. Decision 125 records the
new boundary after human merge of the recovery-guidance prerequisite #144.

The repository pins Go 1.27.3 in `go.mod`. The official release history fetched
2026-10-30 lists Go 1.27.3, released 2026-10-19:
https://go.dev/doc/devel/release#go1.27.0
This slice changes no toolchain or library pin.

## Signal-handling integration

Focused review reproduced a regression before opening this PR: installing the
standalone signal context also intercepts INT/TERM during default V1 Git source
extraction. Its old direct capture can wait on a Git descendant after the leader
is killed. The compiler-only tests do not qualify that earlier stage. The base
CLI exited on SIGTERM immediately; the candidate waited more than six seconds
until the evaluator released the descendant's capture descriptors.

Complete this boundary in the same change. Reuse existing native Git supervision
for object discovery, repository initialization and revision inspection. Supervise
the source archive into private workspace files before parsing it, retaining the
same literal Git operands, filtered environment and committed-byte extraction
policy. Do not impose the native in-memory capture ceiling on the archive: valid
repositories may contain large unrelated files that extraction already ignores.
Bound diagnostic reads and retain the private workspace on typed ownership
uncertainty. Confirm successful group completion before consuming archive bytes.

Independent readiness-based criteria must expose the Git-stage failure before
the repair, then exercise cancellation and parent-only signals across the fixed
responses and archive path, plus healthy real Git and large-archive controls.
Update the discovered acceptance census after adding these integration criteria.
The seven additional criteria are four API cancellation stages, standalone INT
at object discovery and TERM at archive capture, and a healthy real-Git build
with a committed unrelated file larger than 16 MiB.

## Focused local observations

Observed 2026-10-30 UTC with evaluator and standalone CLI race instrumentation:
the compiler family first reported `1 Passed | 9 Failed` in 27.981 seconds,
then `10 Passed | 0 Failed` in 3.033 seconds. Its healthy real compilation was
the original passing control. The Git integration family first reported
`1 Passed | 6 Failed` in 38.882 seconds, then `7 Passed | 0 Failed` in 3.542
seconds. Its healthy control streamed 17,838,080 real archive bytes.

The combined focus `Local source compiler process ownership|V1 packager Git
process ownership` reported `Ran 17 of 3096 Specs in 5.406 seconds` and
`SUCCESS! -- 17 Passed | 0 Failed | 0 Pending | 3079 Skipped`. All 22 observed
leader/descendant PIDs were absent after fixture cleanup. The actual source
scheduler reported `1 Passed | 0 Failed` in 19.813 seconds and established the
disjoint exhaustive registry: 2,954 base, 98 installation and 44 handoff entries.

An evaluator-only native inspection fault established typed ownership uncertainty
and one retained workspace with an actionable path. Negative controls removing
workspace retention or typed wrapping failed the corresponding assertions.
These overlays are evaluator inputs, never production switches.

Broader local commands ran:

```sh
go test -race -count=1 -timeout=4m ./internal/packaging -ginkgo.fail-on-empty
golangci-lint run --config packs/go/.golangci.yml ./...
go vet ./...
gosec ./internal/packaging ./cmd/factory-package
govulncheck ./...
```

Observed respectively: package exit zero, `0 issues.`, exit zero with no output,
`Issues : 0`, and `Your code is affected by 0 vulnerabilities.` These focused
local results do not establish hosted CI, model-review completion or release
readiness; the same exact-head hosted requirements still apply.
