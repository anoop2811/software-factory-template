# ADR 0121: measure local upgrade interruption at the publisher boundary

Status: accepted bounded test correction; controlled RED precedes changes

Date: 2026-10-31 UTC

## Observed CI failure and limits

PR #150 source job 124380501158 at `3e65d4b` failed the existing forward
durable-interruption criterion's sixty-second readiness predicate. The complete
family reported twelve passes and one failure in 1174.200 suite seconds. The
reverse criterion passed. The artifact contains no retained worker phase or
stdout/stderr snapshot for the failed wait, so it cannot establish which
acquisition or publication step the hosted process had reached.

Source inspection shows that the readiness timer starts before local image
compilation, exact predecessor reconstruction and transaction preflight. It
therefore measures more than the intended durable publication boundary. A
controlled slow-compiler reproduction must demonstrate that distinction before
changing the canonical criterion. Do not label an unobserved hosted stage as
the proven cause.

## Correction contract

Keep the existing ninety-second overall command context and sixty-second
publication allowance. Add an atomic test-owned publisher-admission handshake
at the actual selected worker's publication entry, after image acquisition and
preflight. Wait for that admission under the existing overall context, then
start the publication-specific predicate. The existing command deadline still
bounds the complete journey; no runtime compiler, process, CI, model or suite
limit increases.

Retain both original durable-row witnesses, the real owned PID/group check,
actual interruption, exact retained image identities, compiler poisoning during
recovery, fresh consent and completed recovery assertions. Preserve both forward
and reverse criteria and the thirteen-case family. Use existing test-owned
selected-source instrumentation; add no shipping protocol, environment bypass
or dependency.

Record stage and liveness evidence when admission/readiness fails. Reconcile
the real child group before inspecting output or deleting private fixtures.
Avoid a blind boolean timeout that loses the process's actual outcome. Prove
the corrected criterion still rejects a missing durable publication witness;
successful delayed acquisition alone must never satisfy it.

Observe the recorded process group directly after every caller-wait path; a
missing leader alone is not group absence. Only a group probe returning ESRCH
proves absence. Keep identity checks before destructive fallback and treat
other errors conservatively. If reconciliation cannot be established, retain
the owned source, target and cache directories through a test-only explicit
cleanup hold. Both temporary-directory and selected-image cleanup registrations
must honor it; normal fixture cleanup still removes the directory. This is not
a shipping flag or retention policy.

Run focused RED/GREEN controls and the complete affected family. Repeat hosted
checks and review for the new head. Keep PR #150 in draft while qualification is
incomplete; independently developed inventory and source-proof functions do
not authorize merging this failed check.

## Linux fixture writer qualification

At head `e917e83`, the Linux base behavioral suite passed 2939 criteria and
configured lint reported G703 at the generic `writeFixture` directory creation.
Fresh local Linux-target scans did not reproduce it. An evaluator-only expanded
caller trace identified the chain from an HTTP test callback's `r.URL.Path`
through a same-signature call-graph edge to `syncWriteJSON`, then the generic
writer. Actual callers are three sync scenarios and a doctor staging helper,
all using existing private roots and literal `opencode.json`; no observed production path
traversal is asserted. The emulated native Linux attempt stalled in package
loading and supplies no scanner verdict.

Refine only the sync test helper to marshal the same JSON bytes, open its
already allocated fixture root with `os.OpenRoot`, and create/truncate the
literal relative `opencode.json` through that confined root. Preserve mode 0600,
all configuration values and adversarial traversal-agent scenarios. Eliminate
the unnecessary generic directory-creation path; add no suppression, dependency,
scanner exclusion, production behavior or CI limit change.

Retain the hosted RED and diagnostic trace. Require the configured analyzer and
sync behavioral family after the helper change, with native hosted Linux lint
remaining the final platform proof. Do not relabel diagnostic-only expanded
analysis as an exact pinned-native reproduction.
