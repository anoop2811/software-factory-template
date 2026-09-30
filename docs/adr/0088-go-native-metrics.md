# ADR-0088: Native Go local metrics

Status: accepted; source implementation qualified locally; platform CI pending.

Decision: 87

## Context

Native report is merged in PR #114. Convert metrics next, including its embedded
Python computation and rendering. The frozen reference is commit
`fb68fb43602abcc0838e5089a1da9604c022530c`. This completes the second half of the
existing reports/metrics source package, without installed activation credit.

## Command and data contract

Source-built `factory-go metrics` must not require the old metrics script,
Python or sourced config/color libraries. Keep text, JSON and HTML modes.
The final `--json` or `--html` wins; repeated days options use the last value.
Accept `--days VALUE` and `--days=VALUE`. Empty or nondigit values default to 30;
zero is valid. Preserve a missing trailing `--days` operand as status 1 and
unknown options as status 2. A days operand that looks like another option is
still the operand. `--no-open` requires the final format to be HTML, otherwise
status 2. Digit-only days use decimal arithmetic; retain supplied digits in
text but emit a JSON number. Reject values above 2147483647 with status 2,
an explicit bound avoiding platform-dependent date overflow.

Discover the caller's Git root, falling back to cwd for an ordinary Git failure.
Resolve relative config and event overrides from that selected root, matching
the shell's `cd`. Only the HTML template comes from the invocation directory.
Reuse the shared flat config grammar, bounded file inputs and process supervisor.
Read-only modes do not write files, run models, download tools or transmit data.

Preserve `factory.metrics/v1`, its object/array shapes, numeric/null/boolean types,
not-measured disclosures and empty arrays. JSON object order and whitespace are
not contractual. Preserve ordinary text, including mock-only, missing scaffold,
missing tasks, absent baseline and stale-baseline advice. Color only on terminal
stdout with empty/unset `NO_COLOR`, matching the existing palette.

Count immediate `.sh` hook entries, including nonexecutables, directories and
symlinks, once per entry even for newline-bearing names. Match shell glob scope
for reporting/mute inspection: hidden names do not participate. Reporting uses
the `factory_log_event` substring; mute gates have an anchored nonzero literal
exit and neither that substring nor the `factory: no-block-event` marker.
Four config keys always contribute armed/inert counts in the native runtime:
test patterns, citation prefix, protected paths and check command. Absence of
the old optional config library no longer suppresses those measurements.

Count nonempty event records, including unterminated tails. Whitespace records
are not empty. Window selection compares the timestamp lexically against the
UTC date cutoff, inclusively. Metrics uses tab-separated fields with empty
fields preserved, unlike report's shell-read IFS handling. Group by second
field, and count repeat buckets with at least three events sharing that gate
and the first thirteen timestamp characters. Sort by descending count then
descending gate bytes, explicitly stabilizing locale-sensitive tie ordering.
Reject NUL event input. Ordinary missing event logs mean zero, not an error.

Use literal, read-only Git commands for commit/merge/revert/author counts,
touched/reworked paths and per-commit verification claims. Preserve the frozen
Unicode word-boundary meaning for verified/fixed/works, case-insensitively;
evidence matching remains case-sensitive. Reuse existing JSON value helpers
where their contracts match. Ordinary unavailable Git/history means zero;
timeouts, capture overflow, cancellation or uncertain child cleanup are errors.

Read eval baseline files in sorted filename order. Count matching baseline
files before parsing, retain null scores and distinguish mock from real results.
Compare nonempty fingerprints for staleness. Preserve valid task order and last
duplicate current-task selection. Malformed JSON/structures are skipped per
file rather than collapsing unrelated valid results. Scaffold/task-directory
discovery retains the existing nonhidden glob scope.
Nonfinite numeric extensions such as NaN and Infinity are malformed JSON here;
never emit them in the versioned JSON output. Preserve ordinary Python-style
finite score display, including a recorded floating score such as `1.0`.

Bound each file and Git output stream at 16 MiB, each directory enumeration at
4096 entries, aggregate eval input at 64 MiB and aggregate eval task rows at
4096. Bound generated JSON and HTML at 16 MiB before full allocation: repeated
per-task metadata must not amplify a bounded input into an unbounded rendering.
Reuse streaming JSON encoding with a capped sink where appropriate.
Reject unsafe special files,
overflow and unreadable required inputs with nonzero diagnostics. Missing
optional inputs and malformed eval files retain the explicit fallback above.
Use cancellation checks and checked output; never claim a complete successful
report after a resource-limit or output failure. Git calls use a 10-second
deadline each, with the existing bounded child cleanup policy.

Compiled regression qualification observed a blocked stdout pipe ignoring
SIGTERM after the first JSON byte was read. Make shared pipe/socket output
cancellable while preserving the caller's descriptor and restoring original
settable descriptor flags. Reuse the already qualified borrowed-descriptor cancellation
mechanism where practical; do not leave a blocked writer goroutine behind.
Keep ordinary files, terminal detection, checked short writes and flush behavior
compatible. Arbitrary custom writers retain their own blocking contract.

## HTML publication and browser boundary

Inject JSON into `/*__FACTORY_METRICS_JSON__*/null` in the existing template;
repository strings must not terminate a script element or execute shell code.
Escape every JSON less-than character as `\u003c`, including comment/script
openers that could change HTML tokenizer state; include expansion in size limits.
Keep the page self-contained. Require exactly one placeholder before writing. Generate
the complete page first, then publish atomically to `.factory/metrics.html`.
Pin the destination directory, refuse a symlinked `.factory`, refuse symlink,
hardlink or special-file destinations, and preserve a prior page on failure.
That preservation applies before the atomic rename. After publication, an output
error or cancellation may return nonzero with the complete new page present;
do not roll it back or leave a partial page to disguise a failed status message.
New pages are private (0600); preserve permissions of an existing regular page.
Clean owned temporary files; create no recovery backups for a generated report.

Browser launch is the one explicit presentation side effect: only HTML, terminal
stdout, empty/unset CI and no `--no-open`. Try the entire `BROWSER` value as an
executable first, then literal ASCII whitespace argument splitting, then
`open`, `xdg-open`, `wslview`. Do not evaluate shell syntax or expand globs.
Retain best-effort detached launch with stdin/stdout/stderr connected to null;
do not wait for a browser session to end. This user-requested presentation child
is intentionally outside bounded computation supervision. Launch failure cannot
invalidate an already published page. Other command modes never launch it.

## Qualification and rollout

Observe independent native-route RED before implementation. Compare normal
JSON semantically and text against the frozen script with controlled history
and events; test argument precedence, window boundaries, eval advice and types,
HTML injection/publication safety, TTY/CI/no-open browser gating, limits and
cancellation. Run configured quality checks and exact-head platform CI, then
review and merge before starting review-lane.

Keep the existing script for installed legacy users. This command does not
activate the Go runtime, remove old scripts or change recovery retention.

The broader race run exposed an existing checkpoint-fixture cleanup ordering
failure: a Go deferred context cancellation could precede Ginkgo cleanup and
close stdin first. Register cancellation as Ginkgo cleanup before the child
cleanup, so the child stream is closed and joined in a deterministic order.
