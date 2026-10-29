# Native developer proof runners

Decision 118 and [ADR 0108](../adr/0108-default-go-cutover.md#developer-proof-cutover)
move source development checks to the selected Go implementation. These runners
require the factory Go checkout and developer toolchain. They are not installed
runtime dependencies and make no paid model calls.

| Developer entrypoint | Native proof scope |
|---|---|
| `make budget-selftest` | Budget/native packages; public admission, usage, role/permission composition and controller outcomes |
| `make loop-selftest` | Loop package; public manual/bounded execution, checkpoints, ordering and process ownership |
| `make adversarial-review-selftest` | Direct provider JSON and streaming clients, large payloads, batched review and trusted workflow boundaries |
| `scripts/selftest/optional-packs.sh` | Native fourteen-gate source proof and explicitly historical initializer optional-input/synchronization criteria |
| `scripts/selftest/java-build-tool.sh` | Native initializer artifact/Maven criteria using frozen historical source inputs |

A fixed selector in `scripts/selftest/go-acceptance.sh` chooses these suites;
unknown selectors and incomplete source checkouts fail. `make check` and the
Linux/macOS Go base jobs already execute their packages and acceptance cases.
They do not repeat each focused developer runner. All source partitions, race
checks, quality stages and the required `selftest` check remain.

## Retirement traceability

The old development fixtures contained 32 budget cases, 40 loop cases and 53
review cases. Their private fake CLIs/check programs were fixtures, not additional
criteria. The outcome audit identified the following native proof owners before
retiring those Python runners and the curl-specific shell harness:

| Old outcome family | Native proof owners |
|---|---|
| Budget plan/grammar, disabled/preflight ordering, models/roles and private output | `budget_command_test.go`, `budget_controller_test.go`, `native_test.go`, `command_environment_test.go`, `usage_test.go` |
| Charged attempts, unknown usage/cost, stop/warn thresholds, reservations, locks and unsafe history | `budget_ledger_test.go`; `internal/budget/legacy_admission_coverage_test.go` adds 17 isolated policy criteria |
| Native child/probe deadlines, uncertain exit and compounded publication failure | `internal/native/ownership_test.go`, `internal/budget/runner_test.go`, native/controller acceptance cases |
| Manual/bounded repair, strict reviewer verdicts, budgets, safety, freshness and resume | `manual_loop_test.go`, `bounded_loop_test.go`, `loop_test.go`, `loop_checkpoint_test.go`; corresponding `internal/loop` criteria |
| Actual post-admission controller death, retained charged rows and blocked second controllers | `cutover_native_integration_test.go`; separate pre-admission death criteria remain in `runtime_transition_test.go` |
| Observed implementer/check/reviewer order and reviewer process failure | `cutover_native_integration_test.go`, using actual child/check receipts |
| Direct provider JSON shapes, caps, precedence, literal diff, errors, timeout and selected-client behavior | `orchestration_review_test.go`, `cutover_native_integration_test.go` |
| Streaming provider cancellation, transport recovery and incomplete responses | `review_test.go`; these do not substitute for direct provider-JSON evidence |
| Full payload fidelity without payload-sized argv and failed-client privacy | All six `adversarial_review_payload_test.go` cases now exercise the compiled Go command |
| Trusted workflow execution, exact revision/coverage, model batching and budgets | `review_batch_workflow_test.go`, `review_ci_test.go`, `review_batch_test.go`, `review_batch_routing_test.go` |
| Delivery and cleanup | Actual qualified installation/local-source/cache/recipe cases; historical initializer criteria are separately labeled |

Test filenames in this table are under `acceptance/` unless a package path is
shown. The new integration file adds 21 criteria; the six payload criteria remain
six. It covers gaps found by this audit rather than relying on a passing test of
a different client or a process killed before any charge.

## Historical boundaries

The 85 initializer component criteria remain intact. Their test-only driver calls
the current native initializer core with an explicitly frozen asset declaration;
this does not claim that current qualified init republishes the old scaffold.
Actual authenticated/local Go delivery is exercised independently.

Python-specific transition syscall and guard checks read thirteen exact files
from immutable commit `95dbecffafb5a04191317b7e16397e684bf5191c` into private test
directories. They still compare actual cooperating historical processes with the
current compiled Go implementation. No controller is restored to the active
source or shipped as a fallback. The full historical sources and retired proof
fixtures remain available as Git objects.

Curl metrics, private response-file storage and Python implementation hooks are
historical mechanisms. Applicable confidentiality, bounded execution, ownership
and output outcomes remain native requirements. Source-level proof is distinct
from published release authentication, minimum-OS qualification and the adopter
pilot's operator acceptance.
