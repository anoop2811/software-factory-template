# Command delimiters are not pathname bytes

Remove a command protocol's delimiter without trimming valid data. A repository
path can itself end in a newline; trimming all trailing newlines can select a
different existing directory instead of merely failing.

Observed 2026-11-06 via the independent source-check regression: a copied hook
selected the correct `project\n/factory`, but native Git-root parsing removed
both the output delimiter and pathname newline and executed a sibling gate.
The RED receipt recorded wrong_runtime_called=false and wrong_gate_called=true.
After single-delimiter removal, the source/native focus passed 26 criteria and
both wrong-target witnesses were false.

Provenance: `internal/checkcmd/command.go:218`,
`acceptance/source_check_entrypoint_test.go:395`, and
`docs/adr/0138-go-source-check-orchestration.md:129`.
Local receipts are `.factory/qualification/r3-check/spec-source/check-newline-red.json`
and `check-native-source-green.json` in the same directory. Decision 153 retains
the design and qualification boundary; this lesson does not replace it.
