# Publish complete fixture receipts before announcing readiness

Observed 2026-10-22 UTC in PR #133: a macOS transition criterion read an empty
PID because the fake help process created its ready path before writing the PID.
File existence proved creation, not completion. A temporary evaluator overlay
paused the actual writer after creation and reproduced the empty receipt.

Keep the reader's process, lock and durable-evidence assertions intact. Publish
the completed receipt through a closed private sibling and same-directory atomic
rename, as recorded in docs/DECISION_LOG.md:4560. This observation concerns a test
fixture; it does not establish a production controller failure.

Provenance: [failed macOS source run](https://github.com/anoop2811/software-factory-template/actions/runs/39725376503),
`acceptance/runtime_transition_test.go` (`transitionNativeSource` and
`transitionAwaitPID`), and the deterministic old-writer overlay executed on
2026-10-22 UTC (`ready path exposed incomplete PID bytes ""`).
