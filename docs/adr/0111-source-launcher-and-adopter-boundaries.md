# ADR 0111: qualify source-launcher admission and installed-adopter detection

Status: implemented with local qualification; exact-head hosted qualification pending

Date: 2026-10-29 UTC

## Observations

PR #141 at `c2f12114ce7aadada7d90a165a5e5140a5aa8735` failed one macOS
configuration criterion because its source launcher printed Bash's child
`setpgid` EPERM diagnostic, although the command succeeded. An independent
private reproduction observed the diagnostic twice in 2,500 launches. Both
affected compiler processes reported their own PID as their kernel process
group ID. This establishes a redundant Bash setup failure with an established
group in those observations; it does not establish the exact kernel race.

Apple's Bash source calls `setpgid` in both parent and child, and prints the
child diagnostic before executing the child body. A child-body redirection
therefore cannot capture that startup diagnostic. Source fetched 2026-10-29:
https://raw.githubusercontent.com/apple-oss-distributions/bash/main/bash-3.2/jobs.c
(the child call and diagnostic are at lines 1641–1642; the parent call is at
line 1705). These observations do not justify suppressing arbitrary stderr.

Independent review also reproduced a separate classification defect. The
copy-manifest hook selects canonical factory-source inventory for any `go.mod`.
A healthy installed Go adopter consequently fails with missing canonical source
assets. Removing only that owned fixture's ordinary product `go.mod` restores
the adopter result. Application language is not evidence of factory source.

## Source-launcher contract

Keep the existing Bash developer bootstrap and installed compiler-free runtime.
Before executing a compiler, positively confirm that the actual wrapper PID is
its kernel process-group ID. Preserve literal arguments, caller environment,
compiler stdout/stderr, inherited descriptors, conventional signal statuses,
bounded group cleanup and an existing developer binary on failure.

A missing process group does not prove its child has exited. Never enter an
unbounded wait on a still-live or uncertain child during failed admission.
Refuse with retained custody when safe cleanup cannot be established, and never
signal the caller's borrowed group. A pre-fork signal must not adopt an older
background job from the caller's environment as the compiler; distinguish the
newly spawned job before using Bash's asynchronous PID as authority.

Isolate only Bash startup diagnostics in private custody. Reconcile only the
exact observed child-setup EPERM record for the positively identified owned
compiler, after successful admission and confirmed cleanup. Unknown diagnostics,
missing or mismatched admission, descriptor exhaustion and uncertain ownership
must refuse publication with actionable diagnostics. Never strip a matching
message from the compiler's own stderr. Restrict monitor mode to the owned
launch where practical; that restriction alone does not repair the reproduced
compiler-startup fault.

Do not introduce an interpreter, external process-control dependency, public
command or production fault-injection flag. Any temporary stderr descriptor
must be positively unused before borrowing; preserve existing FD3/4 handshakes
and every other occupied descriptor. Keep private admission and diagnostic
files out of user output and remove them only under confirmed ownership.

## Inventory contract

Recognize canonical factory source using its actual source identity and
structural evidence, rather than an arbitrary product module declaration.
Preserve refusal for damaged canonical source, including missing required
assets and untracked or symlinked source. Installed and historical adopters
retain their declared inventory behavior. A failed native inventory is never
permission to fall back to scripts or report a successful skip.

## Independent qualification

Independent Ginkgo/Gomega authors strengthen the real compiler readiness
criteria with kernel PGID equality, and add healthy normal completion with
empty stderr, correct executable publication, actual compiler absence and no
private build residue. Preserve all three parent-only signal criteria and their
held-open FD3/4 handshake. Qualify preservation of occupied descriptors and
literal compiler diagnostics. Retain the natural old-source diagnostic
reproduction separately from deliberately broken-group negative controls.

For inventory, exercise a genuine healthy installed Go adopter and a healthy
canonical source control. Observe the adopter criterion fail on the previous
predicate, then pass after correction. Retain all damaged-source refusal
criteria. Fix the confirmed rehearsal-document spacing without changing its
recorded counts or broadening its qualification claims.

Review correctness, process/descriptor trust and coverage independently, then
verify findings against full source. Exact-head hosted checks and complete
advisory model coverage remain required before merge. This contract does not
qualify RC4 or stable publication.

## Local qualification

The independent source criteria first observed both custody faults fail:
`0 Passed | 2 Failed` in 5.325 seconds. The final source selection passed
`7 Passed | 0 Failed` in 4.935 seconds with evaluator and CLI race instrumentation.
It retained all three normal signal statuses, occupied FD3–20, quiet normal
completion and literal compiler stderr. Failed group admission returned 125
before the held child was released, preserved its private workspace and existing
binary, and left the borrowed-group witness alive. The pre-fork signal returned
130 while preserving the older background job. Test-owned processes were reaped.

The wider local regression command was:

```sh
FACTORY_CLI_TEST_RACE=1 GOMAXPROCS=2 GORACE=atexit_sleep_ms=0 \
  go test -race -count=1 -timeout=6m -v ./acceptance -ginkgo.no-color \
  -ginkgo.focus='EX001|Go source launcher signal ownership|Default Go initializer inventory proof' \
  -ginkgo.fail-on-empty -ginkgo.json-report=.factory-final-source-boundaries.json
```

Observed: `Ran 135 of 3173 Specs in 214.672 seconds`,
`135 Passed | 0 Failed | 0 Pending | 3038 Skipped`.
The genuine installed-Go-adopter criterion failed before the predicate repair;
the final nine inventory criteria passed. A copied-hook control that converted
native refusal into success failed all four damaged-module criteria.

These are local Darwin observations. The actual discovery census is
`3173 = 3026 base + 103 installation + 44 handoff`; it is a registry count, not
a claim that every criterion ran in this focused command. Hosted Linux/macOS
checks, complete exact-head review and published qualification remain separate.
