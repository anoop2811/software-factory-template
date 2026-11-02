# Keep legacy dependency guards in historical fixtures

Removing a runtime dependency changes the expected outcome of its old absence
test. The direct golden evaluator previously had to refuse before paid runner
work when Python was missing. After native routing, that same invocation should
succeed without Python. Keeping the old refusal assertion against current source
would test the retired implementation contract instead of the replacement.

Preserve the old guard against immutable historical bytes and independently
exercise the current entrypoint's successful result. The governing boundary is
[ADR 0132](../../docs/adr/0132-go-source-golden-evaluation.md).

Provenance: observed 2026-11-02 via the private R3 golden-evaluation acceptance
run: three new criteria failed against the old entrypoint, then the combined
native evaluation suite passed 42 criteria. The historical guard is pinned in
`acceptance/source_prereq_entrypoint_test.go:287`; current execution and independent
result checks are in `acceptance/source_golden_entrypoint_test.go:45`. Local run
receipts are `.factory/qualification/r3-golden/red-golden-full.json` and
`.factory/qualification/r3-golden/golden-and-native-eval.json`.
