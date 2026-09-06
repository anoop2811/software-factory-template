# Loop boundaries need real policy inputs

A controller can inherit a control's name while losing its semantics. Factory
language packs describe test paths as POSIX extended regular expressions; glob
matching silently misses real tests. Git's relevant-source list also omits ignored
native permission files. Safety snapshots must cover the actual policy inputs.

Recheck absolute deadlines after locks and before process creation. A callback
that normally records completion cannot carry ownership information if persistence
fails first; keep uncertainty independently and block overlapping work.

Freshness tests must use a resumable state. A test that accepts any refusal can
pass because a different terminal-state guard fires, even with freshness removed.

Provenance: observed 2026-09-06 during Decision 47 review and deterministic
fault/mutation injection in scripts/selftest/loop/acceptance.py: Go/Java patterns
failed glob classification; ignored Claude permissions escaped the snapshot;
a delayed admission lock launched after deadline; completion-write failure lost
loop uncertainty; disabling freshness validation initially left its tests passing.
The contract and fixes remain in docs/LOOPS.md and docs/DECISION_LOG.md:1736.
