# Separate fixture timeouts from consumer failures

A timeout's last stack frame does not identify the expensive part of a suite.
PR #163's macOS handoff workers accumulated about 1443 and 1412 seconds of
criterion work before the suite deadline; the final apply call alone did not
establish a process hang. The default-upgrade failure occurred in image setup,
before its consumer assertion. Inspect setup phases and per-worker reports before
changing runtime behavior, test assertions or scheduling.

Provenance: observed 2026-11-03 via the logs and JSON artifacts of
https://github.com/anoop2811/software-factory-template/actions/runs/41473183811.
The bounded fixture repair contract is in
docs/adr/0132-go-source-golden-evaluation.md:74; production packaging and its
dedicated qualification remain separate from consumer-fixture preparation.
