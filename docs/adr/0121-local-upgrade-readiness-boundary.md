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

## Pre-admission timing diagnosis

Source run 41165946878 at `97c5cf1` passed 25 checks and failed the macOS
reverse interruption criterion at its ninety-second publisher-admission wait.
Artifact 13030165136 records no admission or durable witness and an untimed
selected-worker APPLY marker. That marker is at `Complete` entry before
context checks; it does not identify which acquisition or preflight stage
consumed the allowance, or prove entry before the deadline expired.

The ninety-second context belongs to this test, rather than the public local
CLI. Real packaging and preflight share that context in this criterion.
Overlap with another compilation-heavy worker is an observed scheduling fact,
not proof of the failure's cause. Local four-node experiments also encountered
watchdogs and workstation sleep; their results do not identify the hosted cause.

Before choosing a repair, use a diagnostic-only evaluator overlay to record
atomic timestamp receipts at selected-worker entry and preflight start/end,
plus the native check outcomes and existing journal/check state after Wait.
Bind receipts to the owned root, PID/group and exact source images. Keep the
ninety-second overall context, sixty-second durable-witness wait, publication
admission location, real durable rows, cancellation and compiler-free recovery
assertions unchanged. Diagnostic receipts never satisfy admission or READY.
Retain the original failed artifact and distinguish observed stage timings
from any subsequent resource or scheduling hypothesis.

## Resource-isolated local recovery qualification

The reviewed diagnostic overlay passed all three selected criteria on unchanged
`97c5cf1` with two workers in 475.336 seconds. Forward admission/durable READY
occurred at 57.436/57.527 seconds; reverse at 60.971/61.049 seconds. Native help
and gate proofs completed with confirmed ownership. This did not reproduce the
hosted failure or establish its unique cause.

Give the two durable selected-worker interruption/recovery criteria their own
`local-recovery` source CI partition. Append that group to the existing matrix
on Linux and macOS. Keep the other eleven local-upgrade criteria in their
existing group. Select the new group by the exact anchored DescribeTable family
with its existing word/end boundary, and skip precisely that family from
`local-upgrade`. Both groups retain the existing two-worker resource policy,
race mode, all source quality stages, report retention, twenty-five-minute test
allowance and thirty-minute job limit. The test's ninety-second overall context
and sixty-second durable-witness wait remain unchanged.

This is a resource-isolation guarantee: unrelated compilation-heavy criteria
cannot be co-selected on the recovery runner. It is not a claim that contention
caused the original failure or a guarantee of completion on every runner.
Making the controls serial inside their old job risks exceeding its existing
allowance: job 124433428007/artifact 13030165136 reports 1257.512s overall plus
161.498s/200.895s controls, estimating 1619.905s against 1500s, not a measured prediction.

Independent tests first reject the old seven-group execution. Require actual
eight-way disjoint/exhaustive discovery of the same 3141 identities, local-upgrade
eleven plus local-recovery two, default/all and explicit-group failure propagation,
empty-selection refusal, report paths and both operating-system mappings. Keep
the full affected public controls and the original failed artifact. Fresh isolated
local and exact-head hosted qualification must pass before merge. Do not alter
runtime code, compiler environment policy, retries, budgets, dependency pins,
criteria or deadlines to implement this partition.
