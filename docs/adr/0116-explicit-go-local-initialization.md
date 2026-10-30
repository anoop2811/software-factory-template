# ADR 0116: initialize from explicit committed local Go source

Status: accepted; focused local qualification complete, hosted review pending

Date: 2026-10-30 UTC

## Scope and public contract

After human merge of #146, implement the local-initialization boundary in
ADR 0114. The public interface is:

```text
factory init [TARGET] --local [--source DIR] [existing initializer options]
```

The explicit local selector authorizes a developer build. Resolve one physical
source directory, reject dirty tracked or untracked inputs, and package its full
committed HEAD with the existing native installation-image packager. Default the
source only to the invoked developer checkout; installed callers require an
explicit source. Use the native host target and `local-FULL_REVISION` identity.
Never present a local build as an authenticated official release.

Retain explicit candidate selector ownership before local routing. Repeated or
valued `--local`, repeated/empty/missing source operands and conflicting release
or installation selectors fail before compilation or target writes. Preserve
literal source/target arguments, help and option termination. Normalize a terminated
relative target with `./` in the private child so the selected initializer cannot
read it as an option; keep its existing non-terminated options unchanged.

Reuse Cobra routing, existing package/stage validation, inherited streams,
native process supervision and the qualified initializer. Local acquisition
does not query GitHub, authenticate a publisher, download a fallback toolchain
or call a model. Go modules must be provisioned for the existing offline build.
The initializer's declared external dependency/setup boundaries remain intact.

Ordinary source init, authenticated candidate init, upgrade, defaults, active
installation roles and the 85 initializer-cache rows remain outside this change.
Do not activate the source bootstrap or publish another candidate here.

## The selected build owns initialization

The preserved implementation in `07e323a` calls the invoker's qualified
initializer for local images. That can mix the invoker's compiled planning,
generation and activation code with another source revision's runtime. The
closed RC3 role recipe does not establish full initializer compatibility.

Require initialization in the exact built executable, using the existing
native inherited-stream handoff pattern. Recheck its identity/digest before
execution and bind the child to its package archive digest, local revision,
host target and physical source. An older selected executable without the
private protocol must refuse before target mutation; never fall back to its
ordinary source initializer or silently use the invoker's generators.

Use a small versioned private local-image entrypoint in the existing factory
executable. Keep it separate from sourceable-library `FACTORY_BRIDGE_PROTOCOL`
and authenticated `FACTORY_RELEASE_DIR` handling. Public `--local` must always
perform the clean-source build; ambient cache hints cannot substitute an image.
The parent constructs literal private handoff arguments. The child validates
private directory/file custody, exact archive SHA, source/revision consistency
and actual self-binary SHA, then privately stages/inspects the local image and
calls its own `initcmd.RunQualified` once, without another build or handoff.
These are explicit-local consistency controls, not publisher authentication.

Preserve installed admission before every private protocol. An installed
runtime cannot use the private entrypoint to bypass custody or pending barriers.
Strip internal handoff/protocol hints before initializer subprocesses. Help or
malformed private input cannot acquire, compile, initialize or remove another
directory. The protocol is an internal transport, not another user CLI.

## Ownership, failure and cleanup

Share the existing acquired-workspace constructor and failure cleanup with
official acquisition without changing its authentication policy. Retain typed
ownership uncertainty and actionable workspace diagnostics. Confirm child
process-group completion before removing parent-owned package/staging data;
retain it on uncertain ownership. The child borrows this workspace and never
recursively deletes a caller-selected cache root.

Before initialization begins, cancellation or build/validation failures preserve
source and target. Once qualified publication begins, retain the initializer's
pending barrier, adjacent backups and incomplete-installation diagnostic. Do
not imply automatic initializer rollback. Successful attempts clean only their
own transient inputs. Existing user data and persistent factory state survive.

## Independent qualification

An independent Ginkgo/Gomega author first observes meaningful failure through
the compiled public command, then covers explicit and invoked-checkout source
selection, clean/dirty state, literal paths, strict selectors/help, compiler
failure and real parent-only INT/TERM after compiler-descendant readiness.

Commit a distinguishable initializer marker only in the selected source fixture;
the invoker predates it. Require that marker during initialization, then inspect
the local revision/native runtime and run installed selftest with the compiler
poisoned. This tests the selected initializer, not merely the installed payload.
Require private-handoff custody/identity failures to preserve the target and
unrelated cache witnesses, with no rebuild loop or release acquisition.

Give `Go explicit local source initialization` its own `local-init` Linux/macOS
CI partition. Reuse bounded handoff workers, race checks and report plumbing.
Keep historical-recipe separate, preserve all prior criteria, and derive the
five-way disjoint exhaustive census from actual discovery. Default/all attempts
every group and propagates every failure without extending deadlines.

Fresh exact-head hosted checks, independent findings verification and public
model-review responses precede human merge. This source prerequisite does not
complete default Go activation, legacy cleanup or release qualification.

## Provenance

Design review on 2026-10-30 UTC read the preserved local shortcut, current
`internal/releasecmd/execute.go:29` digest handoff and
`internal/initcmd/selection.go:55` compiled selection call. FR-019 in
`specs/001-go-runtime-conversion.md:324` binds selected runtime identity;
`docs/adr/0101-go-release-handoff.md:148` requires a release-matched working
installation. The existing services remain the implementation collaborators.

The selected build owns its remaining initializer options. Malformed options may
be refused after compilation but cannot mutate the target. An empty terminated
tail retains the default target. This local transport normalization does not
change ordinary source or authenticated candidate argument parsing.

User refinement on 2026-10-30: test suites and test filenames describe features,
not release numbers. Name the existing historical family `Supported predecessor
recipe compatibility` and its data oracle `Historical installation recipe
integrity`. Keep immutable release identifiers in fixture data and provenance;
this is a naming change, with no removal of compatibility coverage.

## Independent local qualification

The independent spec author ran the real compiled CLI with Go 1.27.3, the
qualified Python 3.12.14 evaluator, `FACTORY_AGENT_ROLE=spec-writer`,
`FACTORY_CLI_TEST_RACE=1`, `GOTOOLCHAIN=local`, `GOMAXPROCS=2` and
`GORACE=atexit_sleep_ms=0`. The focused command was:

```sh
go test -race -count=1 -v ./acceptance \
  -ginkgo.focus='^Go factory command acceptance Go explicit local source initialization( |$)' \
  -ginkgo.fail-on-empty -ginkgo.no-color \
  -ginkgo.json-report=../.factory/qualification/local-init-slice-20261030/public-green.json
```

Observed output: `Ran 10 of 3110 Specs in 210.551 seconds` and
`SUCCESS! -- 10 Passed | 0 Failed | 0 Pending | 3100 Skipped`.
The initial missing-function run was one pass/eight failures. Independent
literal-target regressions then failed twice for candidate selector ownership
and once for a known initializer option interpreted after `--`; the final ten
include their corrections and three private exact-target decline controls.

The renamed predecessor family passed all four cases in 197.504 seconds. The
unchanged immutable-data oracle passed its one criterion. All eleven scheduling
criteria passed in 45.766 seconds, using actual five-way identity discovery:
2,954 base + 98 installation + 44 release handoff + 4 historical recipe +
10 local init = 3,110 disjoint, exhaustive criteria. Discovery is not an assertion
that all 3,110 criteria ran locally.

A separate private source overlay replaced selected-runtime execution with the
former invoker-side qualified initializer. Its explicit-source criterion failed
on the selected-only initializer marker; restoring the canonical code passed
that same criterion. Primary production was never changed for this control.
These local results do not qualify a published release, default activation,
local upgrade, cache retirement or the supported-platform release matrix.

Review clarification: resolve an omitted developer source lazily, only after
valid selector help and explicit source selection are handled. Surface a real
invocation-directory lookup failure; an installed caller's absent resolver still
requires explicit source. Include bounded local Git failure diagnostics while
preserving typed process-ownership errors. Keep the existing generic Cobra help
surface, adding the local selector to its command summary. A flag-looking relative
source remains representable with `--source=--name` or `--source ./--name`;
ambiguous split selectors are not reinterpreted as source values.
