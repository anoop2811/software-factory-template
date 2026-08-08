# Lesson: prove a new fixture fails before trusting that it passes

## Date
2026-08-08

## Context
While fixing review findings on `factory metrics` (PR #45), five new break/fix
fixtures were added and the suite reported them green on the first run. Green on
the first run is the signal to be suspicious of, not satisfied by: a fixture that
has never been observed failing has not been shown to test anything.

Running the fixtures' own inputs against the pre-fix code found that six checks
passed while measuring nothing:

- `scripts/selftest/run.sh` exports `FACTORY_EVENT_LOG` globally near the top of
  the file. The metrics fixtures wrote hostile gate names to
  `$MROOT/.factory/events.log` while every metrics run read the exported path.
  The reads and the writes addressed different files.
- The pre-existing fixture seeded its event with the fixed date `2026-01-01`.
  Gate names are reported over a rolling 30-day window, so the seeded event had
  aged out of every windowed metric. A hardcoded timestamp in a windowed metric
  is a fixture with an expiry date.

Both bugs share a shape: the checks asserted *absence* ("hostile markup does not
appear in the page"), and absence is exactly what an empty measurement produces.
An assertion that something is missing passes for free when nothing was measured.

The same exercise corrected the review itself. The finding said an unquoted
heredoc let `$(...)` in a gate name execute as shell; running it showed no
execution, because shell expansion is not recursive — `$(metrics_json)` expands
once and its output is not re-expanded. The real defect was adjacent and equally
worth fixing: the JSON was interpolated into a Python triple-quoted string, so a
gate name containing three quote characters ended the string and crashed page
generation with a `SyntaxError`. Data was becoming code, via Python rather than
via the shell.

## Rules

1. A new fixture is not finished until it has been observed failing. Run it
   against the pre-fix code, or break the fix on purpose, and keep the output.
2. Prefer asserting presence over absence. Before asserting that hostile input
   was neutralized, assert that the hostile input reached the thing under test.
3. In a shared suite, check what the harness exports. A globally exported path
   silently redirects a fixture that sets up its own.
4. Never hardcode a timestamp into a fixture for a windowed metric. Compute it
   with `date -u` so the fixture cannot age into vacuous success.
5. Verify a review finding before fixing it. A misdiagnosed cause fixed by
   copying the reviewer's suggested remedy leaves the real defect in place.

## Provenance
Observed 2026-08-08 while addressing review findings on PR #45
(`anoop2811/software-factory-template`). Pre-fix behaviour reproduced directly
against `git show HEAD:scripts/factory-metrics.sh` in a scratch repository:
`claim_commits: 3` where one commit made the claim; `tail -n 0` emptying the log
at `FACTORY_EVENT_MAX_LINES=1`; `grep -c '</script><b>' .factory/metrics.html`
returning 1; and `SyntaxError: unterminated string literal` on a gate name
containing three quote characters. Suite went from "95 passed" (six vacuous) to
"103 passed, 0 failed" after the fixtures were wired to their own log. See
[[001-verification-contract]] — the same failure mode one level up: there, claims
without evidence; here, evidence that measured nothing.
