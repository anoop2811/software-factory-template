# ADR 0099: complete native factory orchestration

- Status: implementation contract; independent RED precedes implementation
- Date: 2026-10-17 UTC
- Decision: 98

## Scope and admission

The user authorized completion of the remaining orchestration workstream after
merged PR #125: harness synchronization, check/prerequisite wiring, structural
and golden evaluation, review request orchestration, and shared helpers. Refine
specs/001-go-runtime-conversion.md:533 and the retained boundaries of ADR 0098.
This does not activate default bootstrap, publish a release or waive the pilot.

Keep Cobra and outside-in Ginkgo/Gomega with separate test and implementation
authors. Introduce no module, interpreter or Cgo dependency. Reuse config,
roles, commandenv, native supervision, file publication and the existing review
stream client. No live paid model is a qualification dependency.

## Public and installed routes

The developer-built Go CLI adds `factory sync [all|opencode|claude|codex]`,
`factory prereq`, `factory eval structural ...`, `factory eval golden ...`,
`factory eval run HARNESS WORKDIR`, and `factory review DIFF_FILE`.
`factory check` becomes native orchestration of its existing eight stages.
New sync defaults to all in OpenCode/Claude/Codex order. Its help is inert and
invalid selectors return 2. Existing direct sync scripts ignored all operands;
their generated installed adapters select their fixed harness and discard old
operands. Other legacy routes preserve their observed parsing and statuses.

Canonical package entrypoints are `synccmd.Run`, `checkcmd.Run`,
`prereqcmd.Run`, `evalcmd.Run`, and `adversarialreviewcmd.Run`. Each takes
context, argument slice, selected root, captured environment and stdout/stderr;
check additionally receives the real stdin file and inherited output files.
Public commands remain under the single factory binary. The existing private
bridge adds `helper` operations; it does not expose another user CLI.

Source Bash implementations remain baseline/source-mode compatibility evidence
until default cutover. The installation role map must replace active sync,
pre-push, prereq, eval/review and runner implementations with thin Go adapters.
Retain stable public paths. Original bytes are covered by the existing inert,
ignored recovery transaction. Inert initializer assets are not active fallbacks.
Installed init/doctor must call Go implementations rather than execute legacy
sync in newly created targets or private scratch. Preserve advisory/mandatory
stage distinctions and cancellation/ownership refusal.

## Synchronization

Output roots come from the selected source/installation, while configuration
retains caller Git-root/FACTORY_CONFIG selection and environment presence over
nonempty YAML over parsed legacy settings. Reuse `config.Effective(ctx,path,env)`
and `EffectiveBytes(ctx,yaml,legacy,env)` plus existing role-tier resolution.
These helpers return a fresh effective environment without mutating input.

Preserve generated Claude settings/MCP/agents and Codex TOML/hook semantics,
model-tier fallbacks, explicit implementer role, role body boundaries, sorted
agent traversal and OpenCode's explicit activation/inherit behavior. Retain
CLAUDE.md -> AGENTS.md linkage and preserve nonempty displaced regular content
in an exclusively named local backup. Existing compatible link is retained;
an unrelated link is a conflict. JSON whitespace/member ordering is incidental;
types, values, instruction bodies and native permission semantics are not.

Deliberate corrections: reject traversal agent identifiers, unsafe parents or
output links, invalid formats and unsupported data before publishing output.
Render/validate the complete selection first, then use exclusive staged atomic
per-file publication. Never claim a multi-file transaction. Recheck destination
identity before replacement; preserve unrelated edits. Treat model strings as
literal data, not sed replacement programs; edit model metadata only in role
frontmatter. Do not overwrite old backups on timestamp collisions. All-harness
sync is sequential; retained Make host wiring must not race adapter readers
against OpenCode canonical edits.

## Check and prerequisite wiring

Preserve the eight-stage pre-push order: direct-main input gate (fail-fast),
make check, check-drift, all unpushed commit lint, diff-aware, decision-log,
pending lessons, and configured local hooks. Continue other stages after ordinary
failure, count failures consistently, and preserve Git-range skips. Feed actual
stdin to the direct-main gate. Retain caller role/environment: this command must
not silently force reviewer role. Fatal cancellation or uncertain process
ownership stops further work. Gate scripts, adopter Make/build files and an
explicit configured shell check remain declared validation boundaries.

Local-hook normalization reuses config.Hooks and retains the legacy first-stage
host-shell word splitting/globbing through a thin sourceable reader adapter;
execute each normalized literal argv without a second glob or eval. Never turn
a missing required gate into success. Missing origin keeps documented skips.
The tracked .githooks/pre-push remains the direct-main-only Git hook contract.

Prereq preserves configured Go minimum (only for selected Go product packs),
Git/harness/linter/jq presence, selected harness spelling, first-line version
presentation, numeric version comparison and aggregated failures. Remove the
Python prerequisite: its old stated reason was golden evaluation, now native.
jq remains a prerequisite for explicitly retained JSON validation hooks, not
for native sync/evaluation/review. No new tool version is pinned here.

## Evaluation and review

`eval structural` preserves the three adapter checks, real test-edit hook probe
payloads, explicit implementer denial/spec-writer allowance, isolated temporary
config and successful disclaimer. Preserve its --harness= parsing and ignored
extra operands. No model is invoked.

`eval golden` preserves strict spaced/equal options, default mock runner, sorted
immediate tasks, result/baseline schema and two-decimal scores, literal runner
workdir/prompt, POSIX cksum fingerprints, baseline staleness/removal/regression
rules and exit statuses. Run each task in a private copied workdir; check the
untampered original oracle, dispose descendants and publish complete JSON
atomically. A timeout, interruption, uncertain cleanup or modified oracle can
never count as a pass, even if a partial answer is valid. The old scorer ignored
runner timeout while scoring; this explicit correction closes that discrepancy.
Ordinary nonzero runner exit alone keeps legacy oracle-based scoring. New or
invalid invocations must never silently select a paid harness or mock fallback.

Native runners preserve the exact existing Claude/Codex/OpenCode argv and logs,
optional FACTORY_EVAL_MODEL and literal final task prompt. Mock pass/fail/cheat
remain deterministic. Example runner remains unwired. No additional model call,
budget bypass, automatic repair or fallback is introduced.

Native review preserves configuration precedence, prompt bytes, provider/model
selection, API-key validation, empty-diff behavior, request shapes, token/reasoning
and provider-pin constraints, timeout/retry limits and sanitized diagnostics.
Use the existing Go OpenRouter stream client whenever explicitly selected by
REVIEW_GO_CLIENT and preserve no-fallback behavior. Other existing JSON response
paths use native bounded net/http and JSON parsing; no curl/jq runtime needed.
Keep fixed endpoints and private request custody; never print keys/prompts/raw
provider error bodies. The client selection cannot enable review-lane by itself.

## Canonical shared helpers

Implement Go event append/retention, epoch/duration formatting, terminal palette
and action-box rendering, hooksPath classification and budget settings/model
plans. Reuse existing config/roles/commandenv; extract duplicated native helpers
instead of creating another parser or classifier. `helper` bridge operations
are event, now, duration, palette, action-box, hookspath, budget-plan and
budget-model; their exact framed data is independently tested before adoption.

Event/timing failures remain best effort. Preserve event path/cap, retained TSV
semantics, NO_COLOR/TTY policy, configured-vs-executable hooksPath distinction,
source-time effects and caller-local palette overrides. Sourceable adapters do
only host-shell effects and literal protocol delegation: no eval, no parent
assignment to bridge controls, no invocation while sourcing inert readers.
Validate complete bounded plans before caller-variable mutation, including
readonly/attribute guards. Color's source-time palette and budget settings'
source-time exports are intentional effects. Pass caller TTY explicitly across
command substitution. Existing command cancellation/descriptor rules remain.

Install generated bridges for events/color/timing/hookspath/budget-config as well
as config/roles. Add every new bridge asset to source-image, syntax/lint and
installed-proof inventories. The completed workstream has an explicit audit of
each remaining active shell boundary and no unclassified orchestration fallback.

## Qualification and measurement

Independent acceptance tests precede implementations. Exercise compiled public
commands, adapter paths, actual child exit/timeout, fake HTTP providers and
literal inputs; use immutable 4ecab79627d08d1e125391a5831eb995efd3fce4 as the
reference for unchanged behavior. Pair corrections with negative controls.
No source command or test implementation is its own behavioral oracle.
Run review-diamond correctness/security/coverage fanout, verification and
deterministic synthesis. Complete relevant source quality and Linux/macOS CI.
Do not change the fixed 30-package denominator or invent completion percentages.
Keep the twelve-feature progress table in chat. Default bootstrap, retention,
four-target release qualification and the adopter pilot remain separate work.

Authoritative API documentation fetched 2026-10-17 before implementation:
https://pkg.go.dev/os/exec
https://pkg.go.dev/net/http

### Frontmatter clarification

Role frontmatter ends at its first closing delimiter. Preserve subsequent body
horizontal rules rather than deleting every exact delimiter line as the old awk
did. Missing/malformed frontmatter is invalid and must refuse before writes.

### Failed Git discovery clarification

Only successful origin/main discovery supplies a revision. Discard stdout from
failed Git discovery, which can contain the unresolved operand itself; retain
the documented missing-origin skips and HEAD fallback instead of treating that
diagnostic operand as a commit identity.

### Helper plan bound

Palette and budget plans are bounded to 1 MiB including framing. A sourceable
adapter rejects an oversized or malformed complete plan before changing any
caller variable; the producer applies the same output bound.

### Shared literal command input

Extend the existing bounded native command supervisor with a plain options
structure for bounded literal stdin and combined/separate output selection.
Reuse the same process-group ownership, cancellation, pipe and capture engine;
do not create a second subprocess implementation for eval probes or review
clients. Existing bounded callers retain their behavior through delegation.

### Nonterminal inherited descriptors

The real check cancellation scenario must reach its gate with stdin attached to
`/dev/null`. On Darwin, its terminal foreground query returns ENODEV rather than
ENOTTY. Both mean no terminal ownership transfer is available; retain ordinary
inherited I/O and cancellation rather than refusing the gate before it starts.
Other terminal-query failures remain fatal.

### Selected runner identity

A named harness selects the actual runner file, including adopter customizations.
Only an exact digest match to the qualified, frozen built-in runner bytes may
use the corresponding native implementation. Other selected executable bytes
remain an explicit custom-runner boundary and must execute without substituting
a paid CLI. Installed thin adapters may delegate to `factory eval run`; they
are the actual selected runner. Fingerprints identify the selected bytes.

### Reachable normalization vulnerability

The native captured-environment proxy resolver makes x/text normalization
reachable. Qualification reported GO-2026-5970 in the existing v0.38.0 module.
Raise x/text to the first fixed compatible v0.39.0, rather than broadening this
conversion into an unrelated dependency refresh. The official vulnerability
record identifies versions before v0.39.0 as affected (published 2026-07-14).
Authoritative sources checked 2026-10-17: https://pkg.go.dev/vuln/GO-2026-5970
and https://github.com/golang/text/tags (v0.39.0 dated 2026-06-30). The official
Go module proxy latest query returned v0.43.0, dated 2026-10-09; the selected
v0.39.0 is deliberately the bounded security update, not a latest-version claim.

### Frozen initializer qualification allowance

The installation acceptance fixture executes the immutable initializer and its
entire installed selftest. Give that fixture a bounded three-minute allowance:
the macOS run at commit 3bcf530 reached its former one-minute deadline while
the legacy selftest was still reporting successful checks. Keep the real
initializer, process-group cancellation, successful exit and `gates proven`
assertions. This changes only the test fixture's allowance; adopter operation
timeouts and production execution limits remain unchanged.
Observed 2026-10-17:
https://github.com/anoop2811/software-factory-template/actions/runs/39050121070/job/117446817823
