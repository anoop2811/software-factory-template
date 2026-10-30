# Cobra: explicitly empty arguments are not nil

Observed 2026-10-30 UTC via the compiled local-upgrade command with no tools on
PATH: `upgrade --local --help` returned exit 2 and `unknown flag: --local`.
Removing the public selectors had left a nil argument slice; Cobra consumed
the process's original arguments again. Passing an explicitly nonnil empty
slice returned exit 0 and the local-upgrade usage. Six public argument/help
controls then passed with `PATH=/nonexistent`.

The pinned Cobra v1.10.2 source, read from the module cache on that date,
assigns `SetArgs` at `command.go:281` and substitutes `os.Args[1:]` for a nil
slice at `command.go:1105`. The caller correction is
`internal/releasecmd/command.go:138`; the command contract is
`docs/adr/0117-explicit-go-local-upgrade.md:26`.

Exercise the actual process boundary when wrapping a parser: a library's own
test context can differ from the application's ambient argv. Preserve an empty
vector deliberately, and keep option termination and literal values intact
across every receiver; see
[the earlier forwarding lesson](113-option-termination-must-survive-every-parser.md).
