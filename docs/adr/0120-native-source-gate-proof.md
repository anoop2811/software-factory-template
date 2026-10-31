# ADR 0120: run source gate proofs through the native engine

Status: accepted for implementation; independent public RED required before code

Date: 2026-10-30 UTC

## Bounded function

Deliver the source gate-proof controller as a separate PR. The source-built
Go CLI's `factory selftest` must exercise the fourteen required core gate
violation/healthy-control pairs through the same native engine used by an
installed runtime. It must not execute `scripts/selftest/run.sh`, compile a
driver, download a runtime, invoke Python or call a paid provider.

This is the source gate-proof controller, not a claim of behavioral parity with
the entire historical shell regression suite. Retain and classify its broader
configuration, pack, sync, upgrade, evaluation, review-lane, metrics and event
regressions until independently replaced in the remaining helper/proof cutover.
The coverage rule remains
`specs/001-go-runtime-conversion.md:593`; the existing source/installed distinction
is recorded in
`docs/adr/0098-whole-installation-upgrade-and-rollback.md:81`.

The source shell launcher, direct selftest script and public installer remain
unchanged in this function. Source bootstrap activation is a later dependency.
This PR publishes no release and changes no stable installer pin, dependency,
supported platform, paid-review limit or release gate.

The required semantic inventory is:

1. `test-edit-denial`
2. `commit-message-lint`
3. `decision-log-gate`
4. `diff-aware-check`
5. `direct-main-push-block`
6. `pending-lessons-push-block`
7. `loop-close-check`
8. `shared-script-enforcement`
9. `hook-existence-check`
10. `wiki-lint`
11. `workflow-lint`
12. `copy-manifest-check`
13. `gate-instrumentation-check`
14. `citation-lint`

## Authority and reuse

Add a source entry to `internal/installedproof`; retain one shared proof loop,
asset reader, pair implementation and output contract. Resolve source assets
beside the invoked executable through the existing dispatcher resolver, never
from the caller's working directory or an environment override. Read confined
regular files through `installationfs` and keep existing size and identity
checks. A missing or unsafe required asset refuses before a success summary.

For source invocation, snapshot the already-running executable obtained from
`os.Executable` and physical resolution. Open it through a separately confined
reader for its actual directory, then use the existing digest-checked private
file publication. Do not trust a colocated `factory` launcher, a source-tree
`.factory/bin/factory-runtime`, PATH replacement or environment-selected driver.

Keep installed admission and installed driver lookup unchanged: the validated
installation root must open `.factory/bin/factory-runtime` relative to its own
confined reader. Do not weaken that path to an independently opened parent
directory. Installed and source entrypoints share the engine only after their
different authority boundaries have selected the driver.

## Cancellation and cleanup

Include `selftest` in the CLI's SIGINT/SIGTERM context ownership. Supervise proof
children through the existing native process engine, reconcile the complete
owned group, and retain typed bounded ownership diagnostics. Cancellation must
produce failure with no success summary. Remove only the identity-checked
private proof scratch after confirmed child completion; preserve it if process
ownership is uncertain. Never remove unrelated temporary files or source data.

## Independent outside-in evidence

An independent spec author first runs the real source-built CLI against real
source assets and a poisoned legacy selftest. Require a semantic failure before
production changes, then prove the fourteen named gate pairs with compiler,
Python, download and provider executables poisoned. Keep an independent literal
gate oracle rather than importing the production inventory.

For each required gate, replace its fixture with always-allow and always-refuse
controls and require failure; restore the real file, then complete a healthy
proof after all controls. Also cover missing/unsafe assets, foreign working
directory, PATH and symlink invocation, hostile driver lookalikes, unchanged
caller/source data and cleanup. Cancellation must wait for a real owned Git
child's readiness and record its process group before signaling only the CLI.
Assert bounded failure, no surviving owned process, no successful proof summary
and preservation of an unrelated sentinel. Exercise uncertainty retention with
an explicit deterministic collaborator fault rather than timing assumptions.

Existing generic mapped-script boundary tests continue to prove literal argv,
stdin/stdout/status, no-shebang fallback, executable failure, signals and PID
semantics through the still-mapped ordinary source upgrade route. The new suite
owns source selftest routing and custody; remove no behavioral criterion and
introduce no shipping test bypass. Reclassify the remaining ordinary upgrade
boundary when its own default conversion lands.

Measure the new family's cost before adding another heavy CI partition. Keep
the existing exhaustive disjoint acceptance census, quality stages, deadlines
and installed proof qualification. Hosted results must identify the reviewed
source; a local pass alone does not establish release readiness.

## Review refinement: completion is more than an exit code

The independent public late-cancellation regression observed the real final
healthy gate complete with confirmed exit zero, then canceled the CLI's actual
context at the native result boundary. Returning the confirmed zero with an
interrupted outcome still let the current controller print its success summary
and exit zero. An independent reviewer accepted this as a violation of the
cancellation contract above before production repair.

Validate both execution outcome and context after a native call, retaining typed
native ownership errors before checking cancellation. Expected violation exits
1/2 legitimately have the normal `failed` outcome; accept normal completed or
failed execution only when the expected status and confirmed ownership also
match. Interruption, timeout and output-limit outcomes cannot prove a gate even
with the expected exit code. Share this validation with the direct-main stdin
proof instead of leaving a second unchecked result path.

Publish per-gate results and the final summary through the existing
context-aware output helper. A borrowed stdout pipe must remain cancellable,
with its original descriptor flags restored. Keep the real-child signal cases
and independently exercise the late result and blocked-output boundaries; do
not replace them with a single early-cancellation happy path.

## Review refinement: bounded cancellation diagnostics

The controller owns signals until it returns, including error publication. A
full inherited stderr pipe must not keep it alive after proof children have
been reconciled. Publish the existing diagnostic prefix and error through the
shared context-aware output helper, using a one-second timeout derived from
`context.WithoutCancel`. Preserve operational status 1 even when that bounded
diagnostic cannot be written.

Independent SIGINT and SIGTERM criteria must fill a real inherited stderr pipe
before invocation and leave its reader idle. Wait for the real owned Git child
to report readiness, signal only the CLI, and observe owned-group disappearance
and bounded exit. Drain stderr only after the caller has been reaped. Require
the borrowed writer's original descriptor flags, unrelated process and sentinel,
source and caller data, and the absence of a successful proof summary.
