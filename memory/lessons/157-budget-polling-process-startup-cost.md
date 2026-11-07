# Budget polling process startup cost

A shell loop's requested sleeps are not its elapsed cleanup time. Command
substitutions and external sleeps add process startup and scheduling cost, which
can consume an enclosing supervisor's grace before the inner owner escalates.

Reduce repeated process launches while preserving terminal-child evidence and
the final observation at the allowance boundary. Qualify the actual caller,
child, borrowed-group witness and capture pipes under controlled scheduling
delay. A passing focused rerun does not explain an earlier hosted failure.

Provenance: observed 2026-11-06 by running the independent scheduling regressions
against preserved PR #171 head `0f67caa`: delayed reap exceeded the three-second
caller window; delayed cold compilation returned pipe-drain uncertainty with
the compiler still present. Both failures retained their original assertions
and performed owned fixture cleanup. The design and evidence limits are in
docs/adr/0133-go-source-evaluation-runners.md:230 and docs/DECISION_LOG.md:7618.
