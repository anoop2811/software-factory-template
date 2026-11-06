# An allowing enforcement check can still launch native configuration

Run development enforcement checks from a disposable checkout with its complete
runtime closure, just like behavioral tests. An early role allowance does not
make every preceding step inert: inspect the entire call path first.

Observed 2026-11-06: invoking `scripts/hooks/test-edit-denial.sh` with the
spec-writer role from the primary checkout returned zero but rebuilt its ignored
`factory-go`. The hook reads patterns at
`scripts/hooks/test-edit-denial.sh:59` before the role allowance at line 77;
`runtime/shell/source.sh:15` sends that configuration read through the physical
source launcher. No product behavior change is claimed by this observation.

The original binary was restored from an exact matching backup and its recorded
0700 mode. Readback SHA-256 was
`260f17aa2077555240b650eb4b26bfc84a601ae9a20fb3877c166da510f71b02`.
Provenance: local `.factory/qualification/r3-helpers/operator-binary-incident.json`
records the command path, rebuilt/restored hashes and backup; the actual rebuild
mtime was 2026-11-05 17:55:22 America/Los_Angeles. The user was informed of both
the unintended rebuild and exact restoration.
