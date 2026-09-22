# ADR-0068: Go native harness execution component

Status: accepted for implementation under specs/001-go-runtime-conversion.md.

## Scope and baseline

Decision 67 continues G2 after native accounting and captured-stream parsing.
Immutable behavior source is commit `4cc771e894e11d5024032106aeb0ef788997bd29`,
`scripts/lib/budget_adapters.py` and `scripts/lib/budget.py`. Port invocation
preparation, local preflight, process supervision and assistant response selection
into Go, retaining the existing shared accounting/parser behavior. This is an
internal execution component, not a replacement budget or loop controller.

The approved twelve-section feature specification remains the source of truth:
[Go runtime conversion](../../specs/001-go-runtime-conversion.md). This ADR defines
one implementation slice and its qualification boundaries, not another feature.

## Admission boundary and component interfaces

Do not add a public or private Cobra launch command. The legacy execute function
contains no budget admission; its caller performs initial plan checks, preflight,
locked re-admission/reservation and PID publication. Keep that ordering available
to the future Go controller rather than creating an unbudgeted native-run path.
Existing public budget/loop commands and their Python implementations stay active.

Introduce internal/native with these boundaries (test-only compiled drivers may
call them; application callers must supply prior budget admission):

- Request: Harness, Role, Model, Root, Prompt strings.
- Plan: Harness, Role, Root strings; Argv []string; Stdin string; Environment
  map[string]string containing only explicit role/overlay overrides.
- Prepare(request Request, environment map[string]string) (Plan, error): local
  role/configuration validation and literal argv/stdin construction; no subprocess.
- Preflight(context.Context, Plan) error: local CLI help and, for Codex
  implementer, native hook-trust inspection. No agent/model invocation.
- Execute(context.Context, Plan, time.Duration, func(context.Context, int) error)
  (Execution, error): already-admitted execution, with a mandatory ownership
  callback immediately after spawn and before writing prompt bytes.
- Execution: Outcome string, ExitCode *int, ProcessPID int, ElapsedSeconds float64,
  Stdout []byte, ExitConfirmed bool, OwnershipUnconfirmed bool. Stdout is transient
  private data, never printed or persisted by this component.
- usage.Response(context.Context, harness string, io.Reader) (string, error):
  assistant-text selection using the existing bounded Python-compatible parser.
- OwnershipError carries ProcessPID with a fixed diagnostic when probe or
  execution cleanup cannot establish ownership resolution. Callers can inspect
  its type without parsing private process output. Deterministic failure tests
  may inject private per-invocation signal/reap collaborators; no environment
  fault switches or mutable global test hooks belong in production.

Prepare and preflight are separate from Execute so locked admission can occur
between preflight and spawn. Execute validates its plan and positive allowance
before any launch, but does not read factory budget settings or claim reservation
by itself. A nil ownership callback is invalid. Its caller must honor context
cancellation while publishing ownership; the executor cannot make an arbitrary
caller callback terminate. No test driver is an installed/discoverable factory
entry point, release asset or fallback implementation.

## Invocation and role parity

Preserve all three native command shapes, selected model, stdin-only prompts,
root cwd, inherited environment and FACTORY_AGENT_ROLE override. No shell is
used to interpret argv, prompts or environment values. Reject NULs, unsupported
harnesses/roles and malformed local inputs before process launch.

- Codex: exec --json --sandbox read-only/workspace-write --cd ROOT, optional
  exact implementer hook configuration, optional --model MODEL, then literal '-'.
  Prefix stdin with Factory role, canonical instructions, and Task as baseline.
- Claude: -p --output-format json --agent ROLE --permission-mode plan/acceptEdits,
  optional model. Read canonical role and native Claude role configuration;
  preserve literal task stdin.
- OpenCode: run --format json --agent ROLE, optional model; preserve task stdin.
  Preserve unknown overlay fields and roles while setting only selected role
  mode to all. Reject non-object overlay/agent/role and literal disable:true.
  Equivalent JSON serialization is sufficient for this environment overlay;
  no byte-formatting contract is added for insignificant JSON whitespace.

Canonical edit permission exactly 'deny' chooses read-only/plan. Other values
retain baseline write modes. The implementer Codex test-edit-denial hook must
exist and be executable; its command and matcher remain exact baseline values.
Role text preserves universal newline handling and the baseline frontmatter
rule (only initial --- followed by LF, then first LF--- split and stripping).

Qualification domain: scalar UTF-8 role, prompt and configuration strings,
regular readable role/config files of at most 1 MiB each, stdin prompt at most
16 MiB including the Codex prefix, and JSON values representable by baseline
serialization. Refuse oversized/non-scalar/nonfinite configuration inputs
before execution rather than silently replacing them. This bounded domain is
not a claim that every permissive malformed Python input has been migrated.

## Local capability checks

Resolve native executable using PATH. Preserve baseline required help flags and
word-boundary matching, help stdin EOF, root cwd, merged stdout/stderr and
nonzero/missing flag refusal. Validate environment/role files before probing.
Codex implementer preflight must perform the existing five-line app-server
initialize/initialized/config-read/hooks-list/configRequirements-read exchange.
Preserve out-of-order response IDs, exact canonical root, explicit hook feature
policy refusals and exact enabled/trusted-or-managed synchronous command/matcher
checks. Parse bounded JSON responses as data, never provider code. Unknown or
malformed trust information cannot authorize model execution.

Every probe has a ten-second maximum inside the parent context, a 2 MiB output
cap and a dedicated process group. Close/reap probes on success and every error.
Do not wait for app-server EOF before validating the required responses; a
healthy server is persistent. Hook probe failure must not hide uncertain process
ownership. Parent cancellation prevents subsequent probes/execution.

## Process supervision and result contract

- Use a fresh process group/session on supported Linux/macOS targets. Preserve
  environment except explicit overrides. Write stdin and drain both output
  streams concurrently to prevent pipe deadlocks. Discard stderr; never expose
  command/prompt/raw child error text through supervisor diagnostics.
- Enforce the earlier of parent context deadline and positive execution
  allowance, including prompt delivery. Expired/cancelled context refuses spawn.
  A parent closing output pipes while still running remains supervised.
- Stdout capture is bounded at 16 MiB; stop when exceeded and retain at most
  limit+1 bytes to establish overflow. Stderr is drained without retention.
- Retain PID before invoking the ownership callback. A callback error triggers
  group termination and reap; preserve typed uncertain ownership if cleanup
  fails. No prompt bytes may be sent before the callback succeeds.
- Kill the process group on timeout, cancellation, output overflow, callback
  failure and ordinary leader exit, including descendants holding pipes open.
  Allow up to five seconds for leader reap. Preserve unknown exit status and
  PID when exit/termination cannot be established; never report uncertainty as
  successful completion. No retry or second native execution occurs.
- Outcomes: completed, failed, timeout, interrupted, output_limit, launch_error.
  ExitCode uses Python-compatible negative signal numbers. ExitConfirmed means
  the leader was reaped; successful group signaling is not proof that a process
  deliberately escaping its group was contained. Elapsed includes cleanup.
- SIGINT/SIGTERM/SIGHUP handling belongs to the calling process via context;
  the library must not replace global signal handlers. Test drivers translate
  these signals into context cancellation as a future controller will.

An execution result is not a completed budget record. The future controller
must invalidate tokens/cost/completeness on timeout/interruption/output-limit,
retain unresolved reservations and publish answer text only after completed,
complete accounting, exactly as the existing controller does.

## Response selection

Reuse the raw parser's whole-document-first, splitlines, numeric and surrogate
identity behavior. Codex selects the last string agent_message text; Claude
selects the last string successful result with is_error not literal true. An
empty final string suppresses older answers. OpenCode selects text records in
order with first identity winning; all identity components must be strings,
including empty strings, and joined answers use LF. Ignore tool, reasoning and
error payloads. Return no automatic framing/newline and never insert answers
into Metadata. Refuse non-scalar selected answer text rather than emit invalid
UTF-8. Answer extraction does not itself authorize terminal publication.

## Explicit safety tightenings and deferred integration

Baseline ordinary CLI help has unbounded output and no dedicated group; the Go
probe bounds both, including descendant cleanup. Baseline execute may launch
with zero allowance when no absolute deadline is supplied; Go refuses nonpositive
allowances before spawn. These are deliberate safety tightenings within the
candidate, recorded before code; do not claim identical behavior for those cases.
Fixed diagnostics avoid baseline OS-error text that may contain private data.

Locked budget admission, history/fingerprint interoperability, loop state and
installation remain separate conversion work. No Python file is retired now.
Actual activation must include ownership-safe gitignored recovery copies,
rollback and predecessor cleanup, plus the required manual adopter pilot.

## Outside-in acceptance and delivery

Independent Ginkgo/Gomega specs precede implementation. Compile a test-only
driver against the production component and fake native executables; do not
invoke installed native agents or use credentials/network/model calls. Compare
command preparation and answer selection to the immutable Python baseline with
explicit expected values. Exercise all three harnesses, roles, stdin/env/cwd,
preflight refusals and Codex trust transcripts.

Use real fake-process cases for full pipes, closed pipes with live parent,
descendants holding pipes, nonzero/signal exits, expired allowance/context,
cancellation, output bounds and ownership callback failure. Verify child exit
independently, not merely that SIGKILL was requested. Add deterministic internal
collaborator probes if real OS failures cannot reproducibly exercise uncertainty.

Run focused acceptance, full Go race/lint/security checks and independent
correctness/security review. Existing public-command tests must prove no new
launch route; packaging remains unchanged. Document source-only versus installed
qualification honestly. This work adds no dependency or native CLI version pin;
existing toolchain and module lock remain in force.


## API provenance

Go API references fetched 2026-09-10 UTC: [os/exec](https://pkg.go.dev/os/exec),
[os.ProcessState](https://pkg.go.dev/os#ProcessState) and
[syscall.SysProcAttr](https://pkg.go.dev/syscall#SysProcAttr). In particular,
CommandContext's default cancellation targets the process; it is not a process
group supervisor. Pipe closure and child Wait must be coordinated, and signal
exit reporting needs WaitStatus rather than treating all signalled exits as -1.
The native argument/protocol contract is the immutable factory baseline named
above, not a claim to support arbitrary future CLI releases.

## Cleanup completion clarification

Cleanup also bounds pipe drainage. If stdout/stderr/stdin workers have not
finished by that deadline, close their descriptors and return OwnershipError
even when leader ExitConfirmed is true; incomplete captured/protocol data
cannot authorize success. Worker state snapshots are synchronized, and no
completed-output claim follows expired drainage. Preserve a parser error
reported during drainage even if leader exit was observed first.
