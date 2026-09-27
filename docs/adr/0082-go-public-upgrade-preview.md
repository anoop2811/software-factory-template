# ADR-0082: Public read-only Go upgrade preview

Status: accepted for source qualification; installed application pending.
Decision: 81. Date: 2026-09-27 UTC.

## Scope and progress

Implement the additive preview in specs/001-go-runtime-conversion.md:156 in the
source-built Go binary. Compose the six-file planner and explicit adoption from
ADRs 0079-0081. This is partial factory coverage: six legacy budget/loop paths,
not a complete factory installation audit. Every report must make that scope
visible. Authentication, runtime qualification, quiescence and recovery still
block application. No successful applicable plan exists in this slice.

Close the remaining one-third of the existing G4 ownership/preview package only
after independent qualification and merge: (13 + 2/3 + 1)/30 = 48.8889%, reported
48.9%, from 47.8%. Do not add packages or award activation, recovery, backup,
retention, retirement or full G4 stage credit. The tracked shell dispatcher,
installer, direct scripts and installed runtime selection remain unchanged.

## Admission and argument contract

For the exact public upgrade command, any operand exactly --dry-run or beginning
--dry-run= reserves the Go preview route before legacy script discovery. Once
reserved, parsing, assessment, cancellation and output failures never execute or
fall back to the shell. This deliberately refuses the ambiguous legacy spelling
--source --dry-run; a literal directory with that name remains expressible as
--source=--dry-run (which alone does not reserve preview) or ./--dry-run.
All invocations without a preview marker retain their existing literal exec
boundary, including unknown options. Other commands and private protocol routing
are unchanged. Do not broadly change root Cobra flag handling.

Accept exactly one bare --dry-run and exactly one nonempty --source PATH or
--source=PATH. Optional --json is a single bare flag. Optional --adopt-path PATH
or --adopt-path=PATH can occur once per selected exact catalog path (one to six).
Optional --confirm-adoption DIGEST or --confirm-adoption=DIGEST occurs once,
requires an adoption selection, and obeys ADR-0081's exact lowercase digest.
Selections without confirmation produce a proposal; confirmation never derives
from environment or stored JSON. Reject unknown options/operands, --ref, --,
help flags, duplicate singleton flags, empty values and attached boolean values.
Separate string values beginning with - are refused; use attached values or ./
for literal dash-leading paths. Catalog selection rejects duplicates/aliases.
No adoption options means an ordinary blocked plan, with no proposal.

Use Cobra's existing public command routing and a small shared command adapter;
keep its strict argument semantics out of the generic dispatch layer. Add no
dependencies or toolchain pins. Capture SIGINT/SIGTERM only for the Go preview.

## Root, observations and authority

The installation is the current directory, pinned as . through the existing
confined observer. Relative SOURCE resolves from that same directory. Do not
reconstruct it from PWD, executable location, Git, factory.yaml or environment.
Callers must run from the installation root. Subdirectory invocation deliberately
assesses that subdirectory; unlike legacy apply it never discovers a parent Git
root. Preview performs no subprocesses, network, configuration evaluation,
recursive discovery, checks, model calls, state writes, receipts or backups.

Reuse assessment.Plan when no adoption is requested. For selected paths, share
ADR-0081's pinned-root adoption planning implementation between the new preview
and existing private plan-adopted: build the proposal, use full selected identities
from action planning, reobserve selections before return, and recheck both roots.
An unconfirmed proposal remains unauthorized with the original ownership blocker.
A confirmed selection uses exactly ADR-0081's partial/full ownership semantics.
Expose its matching proposal in the preview so users can inspect and explicitly
confirm it. Never compose independently opened root scans into apparently coherent
authorization. Preserve existing private JSON and error contracts unchanged.

Before any later apply, consent and observations must be revalidated under
migration exclusion with all remaining prerequisites satisfied. Preview does not
create a durable authority token. It does not inspect or prune recovery folders;
report that recovery/retention is not assessed rather than inventing zero counts.

## Public report and status

Default output is deterministic human-readable text. Always identify read-only
operation, partial six-path coverage relative to the current directory, all six
actions with reasons, all eight action counts, ownership basis and authorized
path count, rollback readiness, all remaining blockers and unappliable status.
When selected, include proposal digest, selected paths and whether confirmation
was supplied. For an unconfirmed proposal, explain rerunning the same selection
with --confirm-adoption DIGEST; do not print executable shell commands or raw
source paths. State backup/retention not assessed and no backup changes made.
Also state that ordinary legacy upgrade behavior is outside this preview; removing
--dry-run would select that existing script path, not apply this Go plan.

--json emits exactly one newline-terminated object, no banner or stderr for a
complete report. Top-level fields: schema_version=1, mode=dry_run,
coverage=partial, plan (unchanged PlanResult shape), adoption_proposal
(ADR-0081 Proposal or null), ownership_basis (unproven, unconfirmed or
explicit_operator_adoption), authorized_paths (canonical array, empty unless
confirmed), recovery_assessment=not_assessed. plan.scope names the six-path scope;
plan.prior_origin always remains unproven. No absolute roots or file contents.
An unconfirmed proposal alone never clears the ownership blocker.

Complete blocked reports return 2, or 1 for per-file assessment I/O errors.
Invalid input, unsafe roots, invalid selected adoption assets and mismatched
confirmation return 2 with sanitized stderr and no report. Ordinary/unselected
unsafe asset rows retain existing planner conflict-report semantics. Missing/unreadable roots, cancellation and unexpected I/O
or output/flush failures return 1 with sanitized stderr, without claiming atomic
pipe output. No status 0 is defined while prerequisites remain blocked. Root
Cobra must preserve the adapter's status and print no duplicate diagnostic.
Preview diagnostics use the single prefix `factory upgrade:`. Known sanitized
assessment refusals name their reason; unknown failures never disclose raw errors.
During assessment, cancellation reports `factory upgrade: preview canceled`; a
context deadline reports `factory upgrade: preview deadline exceeded`, both with
status 1. Output/flush failures retain `factory upgrade: cannot write preview`.

Keep top-level help aliases and all existing descriptions, inserting beneath
upgrade's existing description exactly:
`              Preview locally with --dry-run --source PATH (run at installation root)`
Replace only its footer with:
`Budget, loop and read-only upgrade preview use the Go runtime. Other commands use auditable scripts.`
Installed shell help remains unchanged.

## Independent qualification

Independent compiled Ginkgo/Gomega RED must precede production changes. Cover
public no-script preview, text/JSON, all action labels and counts, partial coverage,
full/partial/unconfirmed adoption, stale/copied confirmation, preserved customized
files, exact statuses, no writes and no subprocess activity. Poison colocated
upgrade, Git/config/model executables and environment settings. Exercise malformed
preview admission including markers in operand positions, option-looking literals,
source order/forms, repeated/empty flags and private protocol precedence. Keep
all nine legacy routes and old private assessment/planning/adoption regressions;
update only documented public help differences and preview-specific expectations.

Controlled tests must cover selected files and either root changing during
unconfirmed as well as confirmed planning, cancellation, no authority after I/O
failure and output/flush failure. Share existing filesystem observations rather
than weakening their tests or inventing a second walker. Require compiled child
race coverage, full source gates, Linux/macOS CI and independent review. These
checks do not establish complete installed migration or cleanup behavior.

## Qualification follow-up: closed output pipes

On 2026-09-27, a real compiled preview with its stdout pipe reader already closed
exited through SIGPIPE (Python returncode -13), with no diagnostic. Independent
compiled RED reproduced the mismatch with the required output-failure status 1.
The [official Go signal documentation](https://pkg.go.dev/os/signal#hdr-SIGPIPE),
fetched 2026-09-27, confirms that fd 1/2 writes can terminate the process before
returning EPIPE unless SIGPIPE notification is registered.

During this preview command only, register a separate buffered SIGPIPE channel
before argument diagnostics and defer signal.Stop across all return paths. Do not
include SIGPIPE in the cancellation context, globally Ignore/Reset it, launch a
drain goroutine, or change shared output and other command routes. This makes
broken stdout reach the existing checked write/status-1 path and prevents a broken
diagnostic pipe from replacing an already selected failure status. The command
still does not claim atomic output or interruptible arbitrary regular-file I/O.
Add deterministic compiled closed-reader coverage and retain all previous tests.
