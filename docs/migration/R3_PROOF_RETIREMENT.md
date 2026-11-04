# R3 proof retirement map

Status: source inventory and retirement plan; no suite retirement is approved by this file.

This map supports [R3 cutover](R3_CUTOVER.md). Its authority is the conversion
contract: specifications in `specs/001-go-runtime-conversion.md:207` require a
complete asset disposition, `specs/001-go-runtime-conversion.md:326` excludes
Go/Python requirements from shipped commands and self-checks, and
`specs/001-go-runtime-conversion.md:591` requires behavioral replacement before
historical assertions disappear. The explicit historical-fixture exception is
`specs/001-go-runtime-conversion.md:543`.

The inventoried controller bodies are from source revision
`f3a0b6520917e104883cc89b123d7244fa0c46a7` (merged PR #161), inspected on
2026-11-02 UTC. Stable B/L/R/F row IDs identify the old obligation even after
its source path becomes an adapter. When that happens, resolve the old source
citation against this immutable revision, not the adapter's shorter file.
Native criterion citations identify the current declarations inspected for this
map; they are lookup points, not a claim that those tests ran or that every
subordinate assertion matches.

## Evidence and completion rules

The source inventory contains **32 budget, 40 loop and 53 review registrations**.
The review count includes the ten timeout/transport cases preceding the selected
Go-client cases. Counts are registrations, not expanded runtime test totals.
`run.sh` is grouped below because loops and conditional pack availability change
its expanded count; a lexical count must not be presented as executed coverage.

The original map was created through source reading and deterministic text
extraction, without running native or historical behavioral suites. Later
qualification is recorded separately below, with its decision and evidence.
Budget, loop and general-family dispositions remain **OPEN**. Review retention
under Decision 150 has local execution evidence; review and merge are pending.
No percentage is inferred from matching names, table length or the size of the
existing native suite.

- **C (candidate):** a named native criterion addresses the stated behavior;
  compare the complete old assertions and run the relevant criterion before
  retiring the historical check.
- **P (partial):** related native criteria exist, but the last column identifies
  an additional boundary, combination or parameter matrix not established by
  this mapping. Locate existing exact coverage first; add independent Ginkgo
  acceptance only if it is actually missing.
- **H (historical mechanism):** an old implementation detail is not a required
  Go mechanism. Preserve the user-visible invariant with an explicit reviewed
  disposition and replacement evidence; deletion alone is not the proof.

An OPEN C/P/H mapping is an unresolved permission to retire that historical test,
not evidence of a production defect, an additional release gate, or a requirement
to rewrite all historical tests in Go. Each row can receive either disposition:

- **NATIVE-REPLACED:** record the exact candidate revision, assertion comparison,
  RED/negative-control evidence where new coverage was needed, passing command/output
  and review disposition before deleting the old assertion.
- **RETAINED-HISTORICAL:** record its explicit compatibility purpose, immutable
  source identity, complete fixture dependencies, development-only invocation and
  exclusion from installed runtime/self-check discovery or fallback. This closes
  its R3 custody obligation without claiming native replacement, deleting the
  oracle or requiring a new Go test for the same historical mechanism.

The approved historical-fixture exception in
`specs/001-go-runtime-conversion.md:543` remains available for every row. Neither
disposition permits duplicate production implementations or Python in shipped
commands/self-checks. The compiler, packaged runtime and source wrapper are
separate boundaries. A collaborator test does not alone establish the same
public-command follow-up sequence. A streaming review test does not qualify the
fixed-HTTP provider decoder or its deadline.

## Native criterion lookup

These are existing named Ginkgo declarations, not new tests. Labels below are
stable map references; parameterized entries mentioned later must also be selected.

| Ref | Named native criterion | Source |
|---|---|---|
| B-read | matches immutable read-only command rendering | `acceptance/budget_command_test.go:67` |
| B-invalid | refuses invalid command input without state or native calls | `acceptance/budget_command_test.go:101` |
| B-disabled | emits exactly one disabled plan before opening a missing prompt or probing | `acceptance/budget_command_test.go:125` |
| B-preflight | does not reserve an attempt after local preflight refusal | `acceptance/budget_controller_test.go:134` |
| B-live | reserves and publishes PID before sending task bytes | `acceptance/budget_controller_test.go:145` |
| B-model | selects budget models with frontier protection and literal model bytes | `acceptance/orchestration_helpers_test.go:369` |
| B-uncertain | retains timeout uncertainty and typed ownership evidence | `internal/budget/runner_test.go:34` |
| B-publication | preserves publication identity without retrying or releasing an answer | `internal/budget/runner_test.go:67` |
| B-pid-error | retains the original PID publication error when finalization also fails | `internal/budget/runner_test.go:273` |
| B-lock-time | recomputes reservation duration after waiting for the lock | `internal/budget/runner_test.go:189` |
| B-plan | matches ordered blockers and session versus checkout accounting | `acceptance/budget_ledger_test.go:300` |
| B-race | admits exactly one of two processes competing for the last slot | `acceptance/budget_ledger_test.go:647` |
| B-schema | matches schema distinctions without repairing history | `acceptance/budget_ledger_test.go:254` |
| B-storage | refuses unsafe storage entries without following or blocking | `acceptance/budget_ledger_test.go:507` |
| B-config | matches configuration validation with explicit admission expectations | `acceptance/budget_ledger_test.go:238` |
| B-output | writes a live plan before native stdin and keeps JSON private | `acceptance/budget_command_test.go:239` |
| B-signal | finalizes an interrupted native child after parent cancellation | `acceptance/budget_controller_test.go:236` |
| N-argv | prepares literal argv and stdin without launching a native process | `acceptance/native_test.go:146` |
| N-trust | inspects persistent Codex hook trust without waiting for EOF | `acceptance/native_test.go:398` |
| N-overlay | preserves unrelated OpenCode overlay data while overriding selected mode | `acceptance/native_test.go:304` |
| N-ownership | preserves typed uncertainty after deterministic cleanup-report failure | `internal/native/ownership_test.go:37` |
| N-parser | retains a parser failure delivered after real leader reap | `internal/native/ownership_test.go:94` |
| N-consumer | refuses success while a capability consumer remains unfinished after cleanup expiry | `internal/native/ownership_test.go:121` |
| N-role | refuses missing or oversized role configuration | `acceptance/native_test.go:506` |
| N-descendant | kills a descendant holding output after its leader exits | `acceptance/native_test.go:456` |
| U-dedup | deduplicates OpenCode step identities and preserves Python signature equality | `acceptance/usage_test.go:204` |
| U-errors | keeps failure and accounting completeness distinct | `acceptance/usage_test.go:148` |
| U-numbers | refuses malformed required token numbers without promoting unknown usage to zero | `acceptance/usage_test.go:160` |
| U-private | never emits transcript, credentials, provider errors or unknown fields | `acceptance/usage_test.go:255` |
| P-failure | propagates admitted native failure without another invocation or script | `acceptance/public_commands_test.go:142` |
| P-signal | cancels its observed native child and persists metadata without retry or fallback | `acceptance/public_commands_test.go:176` |
| P-death | retains evidence after abrupt controller death even if its flock is released and ledger is empty | `acceptance/runtime_transition_test.go:387` |
| I-activate | activates the complete target and runs the installed native proof | `acceptance/installation_upgrade_test.go:1033` |
| I-init | initializes ordinary assets without invoking the poisoned legacy implementation | `acceptance/native_init_test.go:119` |
| I-optional | tolerates missing optional evaluation and workflow assets | `acceptance/native_init_test.go:541` |
| I-local | upgrades a genuine legacy installation with fresh consent and restores exact originals through rollback | `acceptance/local_source_upgrade_test.go:277` |
| I-packs | matches immutable generated artifacts for packs and model profiles | `acceptance/native_init_test.go:290` |
| L-plan | plans manual checks with both features disabled without writing state | `acceptance/manual_loop_test.go:48` |
| L-manual | runs one deterministic check and preserves manual outcome semantics | `acceptance/manual_loop_test.go:60` |
| L-refusal | reconciles invocation failures without inventing or losing ownership | `internal/loop/bounded_test.go:191` |
| L-harness | approves after separate budgeted implementation and review | `acceptance/bounded_loop_test.go:133` |
| L-iterate | matches immutable iteration and paid-call counts | `acceptance/bounded_loop_test.go:167` |
| L-grammar | preserves argparse status and output channel before any state effects | `acceptance/bounded_loop_test.go:208` |
| L-unsafe | hands off without further paid calls on unsafe transitions | `acceptance/bounded_loop_test.go:242` |
| L-review | refuses malformed review without launching repair | `acceptance/bounded_loop_test.go:255` |
| L-strict-review | admits only exact independent review verdicts | `internal/loop/bounded_test.go:20` |
| L-source | matches content and safety changes independently | `acceptance/loop_test.go:159` |
| L-publication | retains paid accounting when the following loop save fails and never relaunches | `internal/loop/bounded_test.go:154` |
| L-terminal | never resumes a terminal bounded loop | `acceptance/bounded_loop_test.go:266` |
| L-fresh | matches repeated fresh Python resumes while preserving history and consumed allowance | `acceptance/manual_loop_test.go:185` |
| L-stale | refuses stale resume without consuming another check | `acceptance/manual_loop_test.go:224` |
| L-allowance | does not launch a check after the total allowance is exhausted | `acceptance/manual_loop_test.go:326` |
| L-unmerged | refuses a genuinely unmerged index | `acceptance/loop_test.go:315` |
| L-private | retains no emitted check output in terminal JSON or stored history | `acceptance/manual_loop_test.go:137` |
| L-corrupt | preserves corrupt accounting without launching or repairing | `acceptance/manual_loop_test.go:300` |
| L-storage | refuses unsafe storage without touching its target | `acceptance/loop_checkpoint_test.go:291` |
| L-timeout | bounds checks by the smaller timeout and confirms process exit | `acceptance/manual_loop_test.go:338` |
| L-native-timeout | hands off a timed out native invocation without another paid attempt | `acceptance/bounded_loop_test.go:308` |
| L-lock | keeps the persistent loop lock throughout execution | `internal/loop/manual_test.go:133` |
| L-signal | persists a certain interrupted handoff after cleaning its observed child | `acceptance/manual_loop_test.go:408` |
| L-active | inspects the whole checkout without modifying either history | `acceptance/loop_checkpoint_test.go:362` |
| R-args | preserves argument, key and empty-diff ordering without legacy tools | `acceptance/orchestration_review_test.go:183` |
| R-body | sends faithful fixed-endpoint JSON shapes without curl or jq | `acceptance/orchestration_review_test.go:206` |
| R-precedence | retains environment presence over YAML and YAML over parsed legacy settings | `acceptance/orchestration_review_test.go:261` |
| R-controls | rejects inadmissible review controls before HTTP without leaking private input | `acceptance/orchestration_review_test.go:281` |
| R-client | uses an explicitly selected Go stream client once and never falls back on its failure | `acceptance/orchestration_review_test.go:293` |
| R-failure | withholds provider bodies and partial output on HTTP failure | `acceptance/orchestration_review_test.go:312` |
| R-truncated | rejects an OpenRouter length-limited answer as incomplete | `acceptance/orchestration_review_test.go:326` |
| R-stream | sends one faithful fixed-endpoint streaming request and publishes completed findings | `acceptance/review_test.go:141` |
| R-stream-time | enforces one deadline and closes a stalled response | `acceptance/review_test.go:280` |
| R-connect | bounds a stalled proxy CONNECT to fifteen seconds despite a sixty-second overall deadline | `acceptance/review_test.go:457` |
| R-stream-fail | fails closed and never retries a response stream | `acceptance/review_test.go:205` |
| R-workflow | keeps executable checkout trusted and scopes credentials to review steps | `acceptance/review_batch_workflow_test.go:124` |
| R-workflow-time | bounds hosted execution and grants cache write access only to the trusted review job | `acceptance/review_batch_workflow_test.go:70` |
| R-trust | refuses unsupported or mismatched pull requests without model calls | `acceptance/review_ci_test.go:332` |
| G-core | proves the fixed fourteen real gates without legacy execution or runtime construction | `acceptance/source_selftest_test.go:501` |
| G-controls | rejects both constant outcomes for every required gate and recovers after restoring all real gates | `acceptance/source_selftest_test.go:583` |
| G-copy | rejects a real untracked Go initializer input behind an adapter and accepts the same tracked input | `acceptance/native_copy_inventory_test.go:411` |
| C-read | reads the first matching flat value using the existing literal grammar | `acceptance/readers_test.go:124` |
| C-export | matches all 17 current keys against immutable Bash and independent expected state | `acceptance/export_test.go:132` |
| C-migrate | migrates blank and missing keys while preserving nonempty YAML and the legacy inode | `acceptance/native_config_migration_test.go:48` |
| S-models | routes each harness by role tier and collapses economy only for the standard profile | `acceptance/orchestration_sync_test.go:318` |
| S-inherit | uses native harness inheritance for blank, cross-provider or unresolved model tiers | `acceptance/orchestration_sync_test.go:346` |
| H-event | appends a UTC TSV event at the caller Git root with default gate and reason | `acceptance/orchestration_helpers_test.go:146` |
| H-retention | trims only above the cap and retains its newest half with at least one row | `acceptance/orchestration_helpers_test.go:180` |
| H-duration | formats durations and degrades unsupported numbers to unknown | `acceptance/orchestration_helpers_test.go:271` |
| H-hooks | classifies the hook Git resolves separately from its executable bit | `acceptance/orchestration_helpers_test.go:310` |
| M-report | reads the template event log and labels its estimate without a legacy script | `acceptance/native_report_test.go:79` |
| M-clear | clears the selected event log when the first argument requests it and ignores extras | `acceptance/native_report_test.go:89` |
| M-json | emits the metrics JSON schema without the legacy script or Python | `acceptance/native_metrics_test.go:63` |
| M-history | matches controlled Git history and Unicode verification claims from a nested cwd | `acceptance/native_metrics_test.go:101` |
| M-html | keeps event data inert in the generated script element | `acceptance/native_metrics_safety_test.go:305` |
| M-open | launches only on the permitted output surface with literal arguments | `acceptance/native_metrics_safety_test.go:51` |
| V-enable | enables a managed workflow and records configuration without the legacy script | `acceptance/native_review_lane_test.go:70` |
| V-disable | disables its managed workflow and records off without the legacy script | `acceptance/native_review_lane_test.go:83` |
| V-secret | matches the secret first field case-insensitively with exactly one lookup | `acceptance/native_review_lane_test.go:158` |
| E-grammar | retains strict golden parsing against the immutable shell oracle | `acceptance/orchestration_eval_test.go:113` |
| E-empty | measures nothing and publishes no result when no tasks exist | `acceptance/orchestration_eval_test.go:132` |
| E-baseline | saves complete schema with independent POSIX checksums and identical baseline bytes | `acceptance/orchestration_eval_test.go:158` |
| E-stale | refuses a stale baseline rather than claiming no regression | `acceptance/orchestration_eval_test.go:250` |
| E-timeout | fails a timed-out valid answer and disposes the observed child process group | `acceptance/orchestration_eval_test.go:225` |

## Budget controller obligations

Historical source: [scripts/selftest/budget/acceptance.py](https://github.com/anoop2811/software-factory-template/blob/f3a0b6520917e104883cc89b123d7244fa0c46a7/scripts/selftest/budget/acceptance.py). Row line numbers refer to that immutable source.

| ID | Historical registration | Native refs | Map | Comparison still required before retirement |
|---|---|---|---|---|
| B01 | unconfirmed harness exit remains timed out and blocks admission with its reservation (`:217`) | B-uncertain, B-plan | P | Compare the retained PID, reserved seconds and subsequent rejected admission together; the collaborator check alone does not prove the full CLI sequence. |
| B02 | cleanup reaping timeout preserves the original error and restores execution state (`:247`) | N-ownership, N-parser | P | Compare original callback error plus bounded cleanup; Python signal-handler/selector mechanics are historical, not Go acceptance requirements. |
| B03 | spawn-ledger failure retains an unreaped process identity and blocks admission (`:268`) | B-publication, B-pid-error, B-uncertain | P | Require the combined PID-publication failure plus unconfirmed child and later admission refusal, not separate successful cleanup cases. |
| B04 | Codex hook probe cleanup is bounded and refuses launch when exit is unconfirmed (`:312`) | N-ownership, N-consumer, N-trust | P | Tie the unconfirmed real Codex probe to refusal before a model launch; generic supervisor coverage is only part of this assertion. |
| B05 | plan and empty report are local and read-only (`:337`) | B-read | C | Read-only plan/report and no state creation. |
| B06 | disabled feature and strict USD never launch (`:347`) | B-disabled, B-config | P | Disabled public command is mapped; explicitly qualify strict max-cost refusal before any paid call. |
| B07 | invalid limits, identifiers, and roles fail before launch (`:358`) | B-invalid, B-config | C | Command identity/role and configuration limits have separate named matrices. |
| B08 | missing CLI, capabilities, roles, and prompt fail before launch (`:374`) | B-preflight, N-role | P | Compare missing executable, capabilities, role and prompt combinations at the public budget boundary. |
| B09 | all three adapters preserve argv, role, model and report real fixture usage (`:395`) | B-live, N-argv | C | All three harnesses, literal argv, role, model, prompt and native accounting. |
| B10 | role-tier model resolution honors the economy profile without lowering reviewers (`:434`) | B-model | C | Economy routing retains frontier review and literal model bytes. |
| B11 | Codex enforces native hook trust and preserves a read-only reviewer sandbox (`:445`) | N-trust, N-argv | C | Actual persistent hook trust plus the exact read-only Codex reviewer arguments. |
| B12 | Claude reviewer retains the canonical plan permission mode (`:460`) | N-argv | C | Claude reviewer explicitly expects permission-mode plan. |
| B13 | OpenCode duplicate step events do not double-count usage (`:468`) | U-dedup | C | Duplicate OpenCode step identities and accounting. |
| B14 | OpenCode role overlay preserves existing project and tool permissions (`:476`) | N-overlay | C | Unrelated project/tool overlay permissions remain intact. |
| B15 | error events are failures even when the harness exits zero (`:493`) | U-errors, P-failure | C | Failure events remain failures independently of accounting completeness and process exit. |
| B16 | malformed, incomplete and invalid numbers never become known costs (`:500`) | U-numbers, U-errors | C | Malformed required numeric usage and incomplete metadata stay unknown. |
| B17 | attempt and cross-task session-run limits are hard even in warn mode (`:514`) | B-plan | P | Compare separate task-attempt and cross-task session limits in warn mode through public run; plan parity alone is insufficient. |
| B18 | failed invocations consume attempts and are never retried automatically (`:524`) | P-failure | C | Failed admitted execution remains charged and is not retried. |
| B19 | simultaneous admissions reserve the last session launch atomically (`:532`) | B-race | C | Real competing processes, exactly one last-slot admission. |
| B20 | estimated USD stop and warn remain distinct from a strict financial ceiling (`:545`) | B-plan, B-config | P | Require explicit stop/warn threshold sequence and call counts; existing uncertain-row fixture does not cover all these branches. |
| B21 | unknown previous cost blocks estimate-stop admission (`:558`) | B-plan | P | Add or locate the completed unknown-cost row followed by estimate-stop refusal; active uncertainty is a different cause. |
| B22 | private metadata excludes prompts, responses, errors and unrelated secrets (`:566`) | U-private, B-live, B-storage | P | Compare all persisted files, unrelated secrets and private modes; a metadata rendering check alone is narrower. |
| B23 | human run output shows the final answer while JSON and ledger stay metadata-only (`:582`) | B-output | C | Human answer is shown while JSON/history remain metadata-only. |
| B24 | corrupt or unknown-schema history is preserved and blocks launches (`:598`) | B-schema | C | Corrupt and unsupported schema distinctions preserve existing bytes. |
| B25 | symlinked storage is refused without writing through it (`:611`) | B-storage | C | Symlinked storage and ledger leaves refuse without following. |
| B26 | timeout terminates descendants, charges an attempt and records unknown cost (`:627`) | L-native-timeout, N-descendant | P | Locate or add budget-specific timeout, charged attempt, unknown cost and exhausted follow-up admission; loop timeout is related evidence only. |
| B27 | termination signal cleans owned process group and records interruption (`:645`) | B-signal, P-signal | C | Actual child cancellation and persisted interruption without retry. |
| B28 | checkout concurrency limit covers different named sessions (`:661`) | B-plan | P | Compare a live first session and a different second session; a static planning row is not the complete concurrency scenario. |
| B29 | active reservations prevent parallel overspending of session wall time (`:674`) | B-lock-time, B-plan | P | Require live reservation consuming the second task wall-time allowance; lock-delay recomputation is related but different. |
| B30 | active unknown-cost invocation blocks concurrent estimate-stop admission (`:689`) | B-plan | P | Require a live unfinished ordinary invocation under estimate-stop, not only a statically uncertain row. |
| B31 | unclean supervisor death remains a charged fail-closed reservation (`:702`) | P-death, B-plan | P | Transition evidence after abrupt death is covered; compare charged budget reservation and later public refusal as one scenario. |
| B32 | installer and upgrade deliver working local budget commands without enabling paid runs (`:724`) | I-init, I-activate | P | Native installation/activation exist; compare post-install budget plan and disabled paid execution for the supported predecessor. |

## Loop controller obligations

Historical source: [scripts/selftest/loop/acceptance.py](https://github.com/anoop2811/software-factory-template/blob/f3a0b6520917e104883cc89b123d7244fa0c46a7/scripts/selftest/loop/acceptance.py). Row line numbers refer to that immutable source.

| ID | Historical registration | Native refs | Map | Comparison still required before retirement |
|---|---|---|---|---|
| L01 | default manual plan is local, read-only and never inherits enabled budgets (`:257`) | L-plan | P | Compare enabled-budget input with default manual loop: both-features-disabled is a narrower fixture. |
| L02 | manual checks distinguish passing evidence from implemented and reviewed completion (`:264`) | L-manual | C | Manual passing checks remain checked evidence, not implemented-and-reviewed approval. |
| L03 | disabled bounded execution refuses before checks or model calls (`:284`) | L-plan, L-grammar | P | Locate the explicit disabled bounded execution branch with zero check/model calls. |
| L04 | missing native CLI is a deterministic handoff and does not block a later manual controller (`:292`) | L-refusal, B-preflight | P | Certain refusal is mapped; require later separate manual run succeeds and no budget admission occurred. |
| L05 | unsupported native flags stop without uncertainty and permit later manual checks (`:305`) | L-refusal, B-preflight | P | Unsupported help is mapped; qualify later manual admission, not only uncertain=false. |
| L06 | unconfirmed Codex hook probe before admission retains PID and blocks manual overlap (`:341`) | L-refusal, N-ownership | P | Combine typed pre-admission probe ownership with PID retention and later manual overlap refusal. |
| L07 | malformed Codex probe response cannot mask cleanup uncertainty or lose owned PID (`:346`) | L-refusal, N-parser | P | Malformed probe plus failed cleanup must retain ownership; separate parser and controller tests do not alone prove composition. |
| L08 | probe signal permission and final wait errors retain ownership with bounded cleanup (`:351`) | N-ownership, L-refusal | P | Exercise signal-permission and final-wait reports through the loop, then prove no manual overlap. |
| L09 | unreadable accounting during a preflight exception remains uncertain and blocks manual work (`:363`) | L-refusal | P | The unreadable-accounting table entry is present; compare subsequent manual refusal as well. |
| L10 | malformed modes, identities, limits and missing commands fail without side effects (`:384`) | L-grammar, L-corrupt | C | Grammar and validation must remain effect-free; compare the exact legacy configuration cases during qualification. |
| L11 | all three harnesses implement then check then independently review with shared accounting (`:400`) | L-harness | C | All harnesses implement, check and independently review with shared accounting. |
| L12 | failed checks feed bounded diagnostics to a repair before review (`:416`) | L-iterate | C | Use the failed-check-repair entry, including diagnostic forwarding and call order. |
| L13 | review findings drive a separately checked repair and a second reviewer (`:425`) | L-iterate | C | Use the review-repair entry and second independently charged reviewer. |
| L14 | one budget attempt permits implementation but refuses the reviewer without raising limits (`:435`) | L-iterate | C | Use review-consumes-remaining-shared-attempt; never increase the user limit. |
| L15 | implementation failures never trigger automatic paid retries (`:445`) | L-unsafe, P-failure | C | Use native-nonzero-is-not-repair-authorization and public failure propagation. |
| L16 | malformed and failed reviewer results stop without accepting a passing check alone (`:452`) | L-review | C | Malformed reviewer outputs cannot approve or launch a repair. |
| L17 | strict reviewer contract rejects duplicate keys, extra fields and contradictory findings (`:460`) | L-strict-review, L-review | C | Compare duplicate keys, extra fields and contradictory verdict/findings, not only JSON parsing. |
| L18 | unchanged failed source stops repeated repair before exhausting the generous budget (`:473`) | L-iterate | C | Use no-source-progress independently of the generous budget. |
| L19 | repeated identical check and source state stops independently of no-progress allowance (`:483`) | L-iterate | C | Use repeated-failure independently of no-progress allowance. |
| L20 | loop attempt limit counts the first implementation and preserves failed changes (`:491`) | L-iterate | C | Use attempt-exhaustion and retain the modified source. |
| L21 | test, governing config and protected-path mutations require human handoff (`:499`) | L-unsafe, L-source | C | Protected/source policy mutation produces handoff before further paid work. |
| L22 | ignored native harness permission files remain governing policy inputs (`:513`) | L-source | C | The ignored-native-file entry remains a governing policy change. |
| L23 | Go and Java pack POSIX test patterns protect their actual test filenames (`:519`) | L-source | P | POSIX ERE test classification exists; compare the actual Go and Java pack patterns and filenames. |
| L24 | budget admission deadline is checked after waiting for ledger lock before native spawn (`:531`) | B-lock-time | C | Recompute remaining execution allowance after admission lock wait. |
| L25 | unconfirmed model exit plus failed completion write blocks subsequent manual loops (`:571`) | B-uncertain, L-publication, L-refusal | P | Require the compound unconfirmed child plus failed completion publication, then subsequent manual refusal. |
| L26 | source changes during a check or reviewer invalidate the passing evidence (`:626`) | L-unsafe | C | Use check-basis mutation plus review-changes-checked-source; successful checks cannot certify changed inputs. |
| L27 | fresh terminal resume does not relaunch or reset accounting; changed source refuses (`:635`) | L-terminal, L-stale | C | Terminal bounded resume never relaunches; stale manual resume cannot consume another check. |
| L28 | prompt, policy, file modes and deletions invalidate checkpoint freshness (`:651`) | L-source, L-stale | C | Prompt/policy identity and mode/deletion freshness remain meaningful; compare the listed mutations. |
| L29 | resumable manual checkpoints reject changed tracked, untracked, mode, deleted and policy inputs (`:668`) | L-source, L-stale | C | Tracked/untracked/mode/deleted/policy changes have distinct source and resume criteria. |
| L30 | manual checkpoint resume cannot silently enable bounded model calls (`:687`) | L-fresh, L-terminal | P | Compare a saved manual checkpoint with a requested bounded mode; terminal bounded refusal is not this mode-escalation case. |
| L31 | manual resume retains elapsed allowance and cannot reset an exhausted checkpoint (`:695`) | L-fresh, L-allowance | C | Repeated manual resumes preserve consumption and exhausted allowance refuses. |
| L32 | unmerged index entries prevent any check, model or checkpoint write (`:714`) | L-unmerged | P | Actual unmerged index snapshot refuses; compare public check/model/checkpoint zero-effects boundary. |
| L33 | private checkpoints omit prompt, agent answers, raw logs and secrets (`:743`) | L-private, L-harness | P | Manual capture privacy and bounded composition exist; compare prompt/model/error/unrelated-secret exclusion in every stored checkpoint file. |
| L34 | corrupt checkpoint state is preserved and refuses further work (`:755`) | L-corrupt | C | Corrupt history is preserved and no check runs. |
| L35 | symlinked storage and checkpoint files never write outside private storage (`:767`) | L-storage | C | Unsafe storage is refused without touching the target. |
| L36 | deterministic check timeout terminates descendants and records a stopped handoff (`:785`) | L-timeout, N-descendant | C | Real timeout kills owned descendants and retains stopped outcome. |
| L37 | loop deadline also bounds a budgeted model invocation and kills descendants (`:799`) | L-native-timeout | C | Loop deadline limits paid invocation without another attempt. |
| L38 | checkout lock rejects concurrent sessions and signal cleans owned children (`:814`) | L-lock, L-signal | C | Persistent lock ownership and actual signal cleanup have separate criteria. |
| L39 | unclean controller death keeps a stale active checkpoint and blocks new work (`:829`) | P-death, L-active | P | Abrupt-death transition barriers and active records exist; compare stale loop checkpoint plus a later refused public command. |
| L40 | installer and upgrade deliver loop assets while preserving manual disabled defaults (`:851`) | I-init, I-activate | P | Compare post-install manual/disabled defaults after both init and supported legacy upgrade. |

## Review controller obligations

Historical source: [scripts/selftest/adversarial-review.sh](https://github.com/anoop2811/software-factory-template/blob/f3a0b6520917e104883cc89b123d7244fa0c46a7/scripts/selftest/adversarial-review.sh). Row line numbers refer to that immutable source.

| ID | Historical registration | Native refs | Map | Comparison still required before retirement |
|---|---|---|---|---|
| R01 | delayed completion gets bounded time without a larger token cap (`:700`) | R-stream-time, R-body | P | These cover streaming timeout and fixed request shape separately; add/locate delayed-success fixed-HTTP completion with unchanged token cap. |
| R02 | timeout overrides normalize and retain a separate connect bound (`:701`) | R-connect, R-controls | P | Streaming CONNECT is bounded; compare default fixed-HTTP timeout normalization and independent connect deadline. |
| R03 | invalid timeouts refuse before HTTP (`:702`) | R-controls | C | Invalid controls stop before HTTP without private input in diagnostics. |
| R04 | timeout diagnostics preserve numeric evidence without leaking partial data (`:703`) | R-stream-time, R-failure | P | Require fixed-HTTP safe numeric timeout evidence and no partial findings; streaming tests are not a substitute. |
| R05 | transport failures are distinct from absent review content (`:704`) | R-failure, R-stream-fail | P | Compare transport failure versus syntactically valid empty model content on fixed HTTP. |
| R06 | HTTP errors cannot turn valid-looking partial content into findings (`:705`) | R-failure | C | Non-success HTTP response never becomes findings, even if its body looks usable. |
| R07 | all providers share the same bounded timeout contract (`:706`) | R-body, R-stream-time | P | All provider request shapes exist; qualify the bounded deadline on each fixed-HTTP provider. |
| R08 | malformed transport metrics cannot leak raw diagnostic data (`:707`) | R-failure | H | Curl write-out strings no longer exist in Go; retain the privacy requirement and qualify Go transport diagnostics instead. |
| R09 | private response storage is cleaned without touching existing siblings (`:708`) | R-failure | H | Private curl response-file mechanics are historical. Explicitly prove the Go replacement leaves no task-owned response artifacts or sibling mutations. |
| R10 | active and generated workflows expose the Actions timeout variable (`:709`) | R-workflow-time | P | Hosted duration is bounded; compare both active and generated Actions timeout-variable plumbing. |
| R11 | selected OpenRouter Go client receives literal JSON and bounded environment (`:710`) | R-client | P | Current criterion proves literal request and bounded env on failure; compare successful client output as well. |
| R12 | selected Go client failure never falls back to curl (`:711`) | R-client | C | One selected Go client failure, no fixed-HTTP fallback. |
| R13 | invalid Go client paths refuse without execution or HTTP (`:712`) | R-client, R-controls | P | Locate or add invalid client path refusals with no process or HTTP call; invalid provider settings are different. |
| R14 | Go client selection leaves Anthropic and OpenAI on legacy HTTP (`:713`) | R-body | P | Require non-OpenRouter providers to ignore explicit selected Go client, not merely their normal request shapes. |
| R15 | explicit empty Go client keeps legacy OpenRouter HTTP (`:714`) | R-body, R-precedence | P | Require explicit empty client to retain fixed HTTP; empty reasoning/provider fields are different controls. |
| R16 | trusted active workflow builds Go while generated adopters remain opt-in (`:715`) | R-workflow | P | Trust and scoped secrets are covered; compare active built client versus generated adopter opt-in. |
| R17 | configured OpenRouter model and markdown (`:716`) | R-body | C | Configured fixed-endpoint model and complete Markdown are preserved. |
| R18 | repository config uses default OpenRouter without native provider selection (`:717`) | R-body, R-precedence | P | Compare absent model_provider using repository configuration/default OpenRouter, not only explicit provider rows. |
| R19 | structural: privileged job requires same repository AND default base branch (`:718`) | R-workflow, R-trust | C | Both trusted workflow checkout and exact supported PR identity checks matter. |
| R20 | OpenRouter default 8192-token response cap (`:719`) | R-body | P | Current body fixture explicitly sets 32768; qualify the absent-control default of 8192. |
| R21 | OpenRouter YAML output cap is configurable (`:720`) | R-precedence | C | YAML max_tokens is read when the caller does not supply it. |
| R22 | caller output cap overrides YAML (`:721`) | R-body, R-precedence | P | Explicit caller body value exists; compare it against conflicting YAML in the same case. |
| R23 | empty caller output cap uses the safe default (`:722`) | R-precedence | P | Empty caller output cap must select the safe default; empty reasoning/provider examples do not prove this numeric control. |
| R24 | invalid output caps refuse before HTTP (`:723`) | R-controls | C | Out-of-range output caps are refused before HTTP. |
| R25 | OpenRouter truncated output is incomplete, not review evidence (`:724`) | R-truncated | C | Length-limited OpenRouter content is incomplete, never approval. |
| R26 | diff content reaches JSON literally without execution (`:725`) | R-body | C | Diff metacharacters enter JSON literally with an independent no-execution witness. |
| R27 | missing API key refuses before HTTP (`:726`) | R-args | C | Missing key refuses before HTTP, including empty-diff ordering. |
| R28 | HTTP failure makes one bounded request without retries (`:727`) | R-failure | C | Fixed-HTTP failure is one request with withheld body; selected stream-client retry policy is separate. |
| R29 | provider error refuses with diagnostic (`:728`) | R-failure, R-stream-fail | P | Compare HTTP-200 provider error payload in the fixed-HTTP decoder; HTTP failure and SSE error have different paths. |
| R30 | empty diff performs no HTTP request (`:729`) | R-args | C | Keyed empty diff performs no request. |
| R31 | Anthropic request retains existing 4096-token cap (`:730`) | R-body | C | Anthropic body retains 4096 cap and its existing shape. |
| R32 | OpenAI request body remains unchanged (`:731`) | R-body | C | OpenAI request shape remains independent of OpenRouter controls. |
| R33 | OpenAI length response retains its existing behavior (`:732`) | R-truncated | P | OpenRouter truncation refusal does not establish legacy OpenAI length behavior; qualify that provider separately. |
| R34 | caller model overrides configured model (`:733`) | R-body, R-precedence | P | Explicit model transport exists; compare caller override over conflicting YAML. |
| R35 | malformed response cannot become approval (`:734`) | R-stream-fail | P | Malformed SSE is covered; fixed-HTTP malformed JSON must be mapped independently. |
| R36 | missing diff refuses before HTTP (`:735`) | R-args | C | Missing operand/file refuses before requests. |
| R37 | complete no-findings answer remains valid (`:736`) | R-body | C | Complete No findings. response is valid. |
| R38 | configured reasoning effort reaches OpenRouter within existing cap (`:737`) | R-body | C | Configured reasoning is forwarded with the selected cap. |
| R39 | unconfigured reasoning preserves provider defaults (`:738`) | R-precedence | P | Explicit empty reasoning is covered; compare absent reasoning without any configured value. |
| R40 | caller reasoning effort overrides configuration (`:739`) | R-body, R-precedence | P | Compare caller reasoning against conflicting YAML; separate field examples do not prove this exact precedence. |
| R41 | explicit empty caller effort omits configured reasoning (`:740`) | R-precedence | C | Present empty caller reasoning omits the configured field. |
| R42 | legacy reasoning fallback remains below YAML precedence (`:741`) | R-precedence | P | Current fixture uses legacy model/tokens; explicitly map legacy reasoning below YAML. |
| R43 | documented gateway reasoning vocabulary is forwarded literally (`:742`) | R-body, R-controls | P | Low effort and invalid effort examples exist; qualify the full accepted vocabulary from the old table. |
| R44 | invalid configured reasoning refuses before HTTP (`:743`) | R-controls | C | Malformed reasoning refuses before HTTP. |
| R45 | invalid caller reasoning is inert data and refuses before HTTP (`:744`) | R-controls | P | Invalid private value is covered; compare shell metacharacters and absence of side effects. |
| R46 | configured OpenRouter provider excludes fallbacks and requires parameters (`:745`) | R-body | C | Provider order, allow_fallbacks=false and require_parameters=true are asserted. |
| R47 | unconfigured OpenRouter routing preserves provider defaults (`:746`) | R-precedence | P | Explicit empty provider is covered; compare unconfigured provider omission. |
| R48 | caller OpenRouter endpoint overrides configured provider (`:747`) | R-body, R-precedence | P | Compare caller endpoint against conflicting YAML provider in one case. |
| R49 | explicit empty caller route omits provider restriction (`:748`) | R-precedence | C | Present empty caller route omits the configured provider restriction. |
| R50 | legacy OpenRouter route fallback remains below YAML precedence (`:749`) | R-precedence | P | Explicitly map legacy provider fallback below YAML; model/tokens legacy rows are related only. |
| R51 | base and endpoint route slugs are forwarded literally (`:750`) | R-body, R-controls | P | One endpoint slug is covered; qualify the accepted base/endpoint vocabulary. |
| R52 | invalid configured provider slugs refuse before HTTP (`:751`) | R-controls | C | Invalid provider slug refuses before HTTP. |
| R53 | invalid caller route is inert data and refuses before HTTP (`:752`) | R-controls | P | Compare shell-looking invalid route, zero requests and no execution witness. |

### Review custody under Decision 150

R01-R15 and R17-R53 are selected for **RETAINED-HISTORICAL**, subject to final
review and merge of the source-review boundary. Their purpose is to preserve the former
curl/jq request, timeout, payload and selected-client regression evidence.
`scripts/selftest/adversarial-review.sh` now materializes the provider engine
and matching config reader from immutable
`f3a0b6520917e104883cc89b123d7244fa0c46a7`. Its curl/client substitutes and
responses remain test-owned. Its workflow trust, deadline and explicit-client
assertions continue to inspect the current workflow files. The command is
invoked only by development/template CI; it is not installed or discovered as
the native public selftest, and no public command can fall back to it.

In a private qualification checkout, `FACTORY_AGENT_ROLE=spec-writer
bash scripts/selftest/adversarial-review.sh` reported
`adversarial-review: 53 passed, 0 failed`. This run used local Python 3.12.0
on PATH for historical fixture tools. It qualifies retention of those old
assertions, not native equivalence for each transport mechanism. Existing
native fixed-HTTP/streaming acceptance and new direct-source qualification
remain separate evidence. The payload-size oracle also receives an explicit
immutable source identity. The final combined source/payload/workflow acceptance
command recorded in Decision 150 passed all 23 selected criteria, including all
six retained payload cases.

R16 retains its active/generated explicit-client and credential wiring checks.
Its blanket assertion that no setup-go step may appear in the generated
workflow is selected for **NATIVE-REPLACED**: the user invariant is that an
adopter's application is not compiled to obtain the factory runtime. A canonical
factory source checkout does require preparation. Independent source/adopter/
installed/explicit-client selection and cold module-cache qualification replace
that text predicate under docs/adr/0135-go-source-review-entrypoints.md:78.
Before updating the predicate, the changed template produced
`adversarial-review: 52 passed, 1 failed`, with only R16 failing. After replacement,
the same historical command reported `adversarial-review: 53 passed, 0 failed`.
The new source preparation criterion and actual fresh-cache workflow criterion
passed in the 23-case focused run and the 71-case full source run. Independent
review retained no findings. Hosted checks and merge remain pending; no
registration is silently removed.

## General selftest families

Historical source: [scripts/selftest/run.sh at the inventoried revision](https://github.com/anoop2811/software-factory-template/blob/f3a0b6520917e104883cc89b123d7244fa0c46a7/scripts/selftest/run.sh).
These stable family rows include conditional and loop-expanded checks. A family
is not one test, and its mapped core gate pair is not evidence for every extra
assertion in that family.

| ID | Historical family and starting line | Native refs or existing boundary | Map and remaining obligation |
|---|---|---|---|
| F01 | Flat config values and inert legacy parsing, line 76 | C-read, C-export | C; include first-key, comments, defaults and planted-command witness. |
| F02 | Test-edit role/pattern/payload controls, line 104 | G-core, G-controls | P; compare the full role, malformed payload and empty-pattern matrix with the native proof's smaller gate pair. |
| F03 | Verification-contract commit-message cases, line 140 | G-core | P; preserve header/evidence, hedge scope and word-boundary cases, not merely one bad/good commit. |
| F04 | Citation validity/range/disabled prefix, line 173 | G-core | P; compare all three cases, including disabled prefix. |
| F05 | Decision range and referenced decision existence, line 192 | G-core | P; missing range and nonexistent Decision reference must remain separate negative cases. |
| F06 | Every installed pack pattern arms test denial, line 231 | I-packs; actual retained pack hooks | P; generated pack parity does not prove the real hook denies each language's test filename. |
| F07 | Java and TypeScript dialect violation/healthy pairs, line 254 | Retained pack hooks | OPEN; the fourteen core gates exclude these pack gates. Keep or port their actual violation/healthy cases. |
| F08 | Unsupported shell diagnostics and direct/Bash dialect execution for three packs, line 286 | Retained pack hooks | OPEN; preserve actionable diagnostic, no parser error, supported direct invocation and healthy control. |
| F09 | Wiki provenance and reachability, line 357 | G-core | P; compare every historical wiki mutation, not only presence of wiki-lint in the gate inventory. |
| F10 | Tracked installation copy inputs, line 414 | G-copy | C; current native inventory proof uses real tracked/untracked inputs and does not depend on vanished Bash copy text. |
| F11 | Gate instrumentation and optional event-library behavior, line 438 | G-core, H-event | P; compare helper loading, mute gates, advisory gates and pack paths separately. |
| F12 | Commit claim word boundaries, line 480 | G-core | P; retain substring versus whole-word behavior beyond the generic commit pair. |
| F13 | Three-harness model/profile/default/inherit rendering, line 490 | S-models, S-inherit | C; match absent-config, blank fallback and cross-provider cases. |
| F14 | Real gate event through report and clear, line 587 | H-event, M-report, M-clear | P; component criteria exist; preserve the composed real gate-to-report assertion. |
| F15 | Upgrade adds missing library, line 602 | I-local, I-activate | H; modern ownership-qualified additions replace the old broad copier. Compare complete selected assets, not old copy-loop text. |
| F16 | Single config authority, literal values, precedence and migration, line 617 | C-export, C-migrate | C; preserve actual resulting bytes and inert input. |
| F17 | Upgrade handoff, nesting, ref and copy count, line 665 | I-local; source handoff suites | H; old self-replacing Bash recursion is historical. Record the native selected-worker replacement and retain exact selector/status evidence. |
| F18 | Shared duration presentation, line 723 | H-duration | C; seconds/minutes/hours and unsupported values. |
| F19 | Stale self-exec handshake, line 736 | Source handoff/custody suites | H; identify the current public lifecycle criterion before retiring the old shell marker assertion. |
| F20 | Metrics eval advice, no-task output and mock-versus-real labeling, line 758 | E-empty, M-json | P; no-task evaluator behavior is mapped, but exact mock/agent labeling and actionable advice still need comparison. |
| F21 | Embedded metrics Python quote scanner and its negative control, line 812 | M-json, M-html | H; scanner delimiters disappear with Python. Preserve successful rendering and literal hostile data; do not keep a scanner that examines no implementation. |
| F22 | Eval option spelling, selected harness and missing values, line 830 | E-grammar | C; direct source adapter qualification is separate from the compiled evaluator. |
| F23 | Unpublished installer pin diagnostic, line 865 | `acceptance/ordinary_bootstrap_test.go:304` | P; named criterion "refuses unpublished unstable mismatched or unauthenticated assets before downloaded execution" exists; compare actionable diagnostic and fetch/init distinctions. |
| F24 | CI credentials are configured rather than assumed, line 883 | R-workflow, R-trust | P; compare the exact generated workflow and token declarations. |
| F25 | Upgrade refreshes pack dialect gates and their instrumentation, line 910 | I-activate, I-packs | P; compare a genuinely obsolete owned installed gate, its replacement and a real failure event. |
| F26 | Golden real scoring, stale baseline and hung-runner bound, line 953 | E-baseline, E-stale, E-timeout | C; retain no tampering, changed oracle and actual observed child cleanup assertions. |
| F27 | Workflow graph hygiene, line 1009 | G-core | P; compare malformed role/plumbing graphs against the pair's particular violation. |
| F28 | Actual Git hooksPath and executable bit, line 1020 | H-hooks | C; configured/inert/armed/hijacked distinctions are named native entries. |
| F29 | Gate enforcement without optional events and configured local hooks, line 1036 | G-core; `acceptance/hooks_test.go:159` | P; named "reports executable hook paths without running them or external parsing tools" does not alone prove absent-event-library enforcement. |
| F30 | Opt-in review lane, live pending state and shipped offer, line 1083 | V-enable, V-disable, V-secret, I-activate | P; compare configured secret, status query and default-off preservation; legacy source-copy assertions require inventory replacement. |
| F31 | Upgrade recursion guard and color-library copy text, line 1129 | I-activate; source handoff suites | H; explicit asset inventory and native lifecycle must replace these implementation-string predicates. |
| F32 | Metrics schema, hostile event names, HTML, output surfaces and commit claims, line 1171 | M-json, M-history, M-html, M-open | C; compare every hostile name and command/output surface; keep real event input in the assertion. |
| F33 | Event cap and tiny-cap retention, line 1268 | H-retention | C; the native criterion explicitly retains at least one newest row. |
| F34 | Landing page placeholder hygiene, line 1281 | `index.html` source check | OPEN; this is repository website validation, not a runtime self-check. Move its real validation to an explicit development check before removing it from historical run.sh. |

## Fixture custody and deliberate retained boundaries

The public native selftest and development regressions have different contracts.
`internal/installedproof/proof.go:20` declares fourteen core gates; its exact
inventory and constant-pass/constant-fail controls are G-core and G-controls.
It does not implement the full set of F families, optional pack dialect checks,
or the budget/loop/review controllers above. Simply replacing `make selftest`
with the fourteen-gate command would not preserve this development coverage.

The following historical fixture dependencies must be isolated before their
active source bodies can disappear:

| Consumer | Current custody | Required disposition |
|---|---|---|
| General historical controller | `scripts/selftest/run.sh:25` materializes selected entrypoints from `1754de23de95a7e3c734bedd86894295e823c988`; the other consumed scripts/libraries still come from the current tree. | Maintain an explicit complete historical dependency allowlist while keeping actual policy-gate subjects current. Add each converted dependency before its old body disappears. |
| Optional-pack wrapper | `scripts/selftest/optional-packs.sh:61` materializes historical entrypoints before dropping source Git custody. | Preserve no-packs/no-review-template diagnostics and later-assertion witnesses. The copied suite must not enter a current source launcher accidentally. |
| Historical golden evaluator | General and optional-pack fixtures materialize `scripts/golden-task-eval.sh` from `1754de23de95a7e3c734bedd86894295e823c988`. `scripts/selftest/run.sh` supplies its own inert selection runners and deterministic pass/fail/cheat mock; it does not copy the shipped runner bodies into those evaluations. Native differential fixtures explicitly freeze the mock and real runner bodies at `4ecab79627d08d1e125391a5831eb995efd3fce4`. | Runner custody remains test-owned/frozen during ADR 0133's source cutover. The config library, policy and configuration inputs remain separate dependencies: isolate the library before its later conversion. Preserve deliberate model/fingerprint inputs; do not imply that the entire fixture is an era-matched snapshot. |
| Budget and loop controllers | `scripts/selftest/budget/acceptance.py:28` and `scripts/selftest/loop/acceptance.py:24` freeze selected entrypoints but copy current runtime libraries. | Freeze the old wrappers and all four Python module inputs as test-only oracles, or retire each assertion after its row closes. Never import removed current production files as a hidden fallback. |
| Native eval/check parity | `acceptance/orchestration_eval_test.go:23` and `acceptance/orchestration_check_test.go:20` already load `4ecab79627d08d1e125391a5831eb995efd3fce4` by explicit Git paths. | Preserve these independent immutable oracles. Production adapter replacement must not alter their meaning. |
| Transition/native-init differential tests | `acceptance/runtime_transition_delivery_test.go:39`, `acceptance/interrupted_recovery_test.go:186`, `acceptance/native_init_test.go:65`, `acceptance/runtime_transition_python_test.go:12`. | Replace current-tree Python module reads/import roots with an identified historical fixture source before production deletion; preserve mixed-runtime and recovery fault coverage. |
| Historical installation recipes | `internal/installation/roles_v1.go:126` and its adjacent historical module declarations. | Retain exact predecessor reconstruction declarations. They describe historical installed bytes and confer no permission to execute old controllers in a current installation. |
| Policy gates and product build tools | Hook and pack sources. | Retain their chosen implementation languages as the approved boundary. Go test orchestration must execute the real relevant gates; it must not convert a missing test subject into a skip. |

Historical-only Bash/Python evidence is allowed by the specification when it has
an explicit purpose and cannot enter installed runtime discovery or fallback.
This exception does not excuse production controllers remaining active forever,
or an installed self-check requiring Python. It also does not authorize deleting
an old assertion because a large unrelated native suite is green.

## Smallest safe remaining proof plan

| Work item | Completion evidence | Status |
|---|---|---|
| P1: Close direct-command boundaries | Source adapters reach their existing native owners; literal arguments, caller roots, status, signals and missing-owner refusal have independent real-entrypoint coverage. Update only their affected historical fixture loaders. | OPEN; see R3 cutover asset rows. |
| P2: Close sourceable-library boundaries | Inert source-time loading, exported/unexported and readonly caller state, literal values, no unsafe eval, and actual native owner selection through the retained function names. | OPEN. Existing helper/reader suites are candidate evidence, not proof of the new source wrappers. |
| P3: Complete the B/L compound fault and admission comparisons | For tests selected for retirement, resolve P rows using exact existing coverage or the smallest missing scenario. Otherwise assign RETAINED-HISTORICAL with explicit custody; keep ownership, accounting and no-retry obligations. | OPEN. |
| P4: Complete fixed-HTTP review control/transport comparisons | For tests selected for retirement, resolve R rows without crediting SSE coverage to fixed HTTP. Otherwise retain the immutable historical oracle. Preserve provider differences, no implicit fallback and private diagnostics. | OPEN. |
| P5: Preserve the broader development selftest contract | Assign native replacement or explicit historical retention to F rows. Retain real pack/dialect and website checks; replace current implementation-text checks with behavior/inventory predicates. Keep optional-source absence visible. | OPEN. |
| P6: Isolate historical test inputs and switch public selftest | Give historical-only fixtures explicit source identities and a development-only runner. Public/source/installed selftest reaches the native fourteen-gate proof with no Python/compiler requirement in the installed form. | OPEN. Do not drop historical coverage to make this switch pass. |
| P7: Remove obsolete active controllers and close the reference inventory | All current references are classified; installation/source manifests and fixture materializers are coherent; no retired runtime module is executed by current commands. Historical recipe references remain explicit. | OPEN. |
| P8: Run bounded affected suites and review the actual cutover | Record exact candidate hashes, commands, outputs, discovered criterion identities and review dispositions. Run native platform/packaged release qualification under the separate release gates. | OPEN. This mapping is not execution evidence. |

Start with existing named native criteria, not a wholesale rewrite of all 125
controller registrations. New tests are justified by the unresolved behavior in
a row. Split production changes by the command/helper boundary in R3_CUTOVER;
keep each such PR's historical fixture repair with that same boundary. Final
proof-controller deletion waits until its last row has an explicit disposition.


The concrete R3 exit sequence is: (1) convert the remaining current command and
sourceable-helper entrypoints, with their new-boundary evidence; (2) give every
historical regression dependency either NATIVE-REPLACED or RETAINED-HISTORICAL
custody; (3) route public selftest to Go while preserving the chosen development
regression runner; (4) remove the four obsolete active Python modules and close
current reference/manifests; (5) run the affected checks and complete review.
The 125 registrations are an accounting aid for step 2, not 125 new implementation
tasks. R4 integrated rehearsal and R5-R7 published qualification/pilot/stable
release remain the existing separate release plan.
