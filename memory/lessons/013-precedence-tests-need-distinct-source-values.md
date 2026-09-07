# Precedence tests need distinct source values

Give every competing configuration source a different value. A test that
compares caller and YAML settings can still hide a third source overwriting the
caller when that third source happens to contain the same value.

Observed 2026-09-06 while correcting EX-001: the legacy file used
`COST_PROFILE=standard`, YAML used `economy`, and the old caller assertion also
used `standard`. The broken reader therefore passed that assertion. Provenance:
immutable baseline `76952eaa` at `scripts/selftest/run.sh:609` and
`scripts/selftest/run.sh:616`.

A fresh Bash fixture using caller `caller-profile`, YAML `economy` and legacy
`standard` returned `standard` with the immutable reader and `caller-profile`
with the corrected reader. Both commands exited 0, so exit status alone would
not expose the error. The shipped shell assertion now distinguishes all three
sources; independent Ginkgo cases cover the supported keys, empty and readonly
values, and literal data handling. Decision 50 is the correction authority in
`docs/DECISION_LOG.md:1899`.
