# Test interpreter dependencies

The Go runtime and the source test suite have different prerequisites. Removing
an interpreter from a fixture does not establish that every compatibility test
can run without it. ADR 0098 defines the native fixture conversion contract at
docs/adr/0098-whole-installation-upgrade-and-rollback.md:634.

## Native process fixtures

The terminal and detached-process fixtures in `internal/native` now use Go and
the existing `golang.org/x/sys` module. They re-execute the compiled test binary,
so fixture execution needs neither an interpreter nor another helper build.
This introduces no module or Cgo dependency. Qualification must preserve
the real direct/inherited shell controls, repeated stop, terminal ownership,
literal input and surviving-descendant observations with Python calls refused.

Before conversion, the complete native race suite with `python` and `python3`
refused reported 15 passed and three failed. Those failures were the terminal
criterion and the two detached-process controls, establishing that the
interpreter-refusal check detects the dependency being removed.

After conversion, the same complete race suite passed all 18 criteria on macOS
and Linux with zero attempted Python calls. An isolated Linux compiler overlay
that replaced child SIGCONT with SIGWINCH made the terminal criterion fail;
the tracked production file remained unchanged. Decision 97 records commands,
actual outputs and hosted qualification separately.

## Remaining replaceable fixture programs

Source inspection on 2026-10-17 identified these separate fixture programs outside
`internal/native`. They remain pending; the inventory is not a claim of removal.
Shared fixtures should be converted once and reused by their callers.

| Test file | Fixture responsibility |
|---|---|
| `acceptance/native_init_test.go` | Terminal answers, redirected output and cleanup. |
| `acceptance/native_doctor_test.go` | Terminal/pipe progress observation and proof release. |
| `acceptance/native_metrics_safety_test.go` | Terminal output and browser eligibility. |
| `acceptance/native_review_lane_safety_test.go` | Terminal answers, EOF, cancellation and oversized input. |
| `acceptance/source_gate_selection_test.go` | Fake compiler recording and selection/failure simulation. |
| `acceptance/adversarial_review_payload_test.go` | Fake transport request and permission capture. |
| `acceptance/bounded_loop_test.go` | Shared harness fixture and diagnostic byte generation. |
| `acceptance/runtime_transition_test.go` | Shared help/harness descendants and Git snapshot wrapper. |
| `acceptance/command_environment_test.go` | Environment capture at an immutable shell-wrapper boundary. |

## Compatibility evidence that still needs Python

Keep the executable Python reference where a test establishes supported legacy
behavior, rather than merely arranging a fixture:

- Immutable budget, usage, stream, native execution and loop reference programs.
- Current legacy transition and pending-state interoperability APIs.
- Python argparse and JSON characterization, including surrogate handling,
  integer/float identity and canonical byte size.
- Actual Python/Go flock interoperability in the budget and loop tests.

Replacing these references with Go implementations would remove independent
cross-language evidence. Their retirement belongs to the corresponding legacy
support decision. Qualified Python provisioning therefore remains in the full
source CI gate. Tests that intentionally hide or poison Python, and references
to retired migration assets, do not themselves require an interpreter.

## Default-cutover preparation

Decision 118 retires the four active factory Python runtime modules and the
obsolete budget/loop developer runners. The installed native factory and its
fourteen gate proofs do not require Python. The payload criteria in the dated
fixture inventory above now exercise Go; [DEVELOPER_PROOFS.md](DEVELOPER_PROOFS.md)
records the replacement owners and historical boundaries.

Source CI still requires Python for retained compatibility references and the
remaining evaluator fixtures. The template-only Maven quality job also invokes
`scripts/selftest/maven-quality.sh`, whose Python standard-library driver builds
an isolated Java project and checks the actual quality plugins. It is a developer
proof runner, not an installed factory command. Its Maven/JDK/Python prerequisites
remain separate from the compiler/interpreter-free installed runtime contract.
