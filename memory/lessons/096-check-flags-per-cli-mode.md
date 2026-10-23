# Check each CLI mode before documenting its flags

A Cobra router does not imply uniform help, version or dry-run behavior. This
factory deliberately disables top-level flag parsing for compatibility, and its
native leaf commands retain separate argument contracts. Document a flag only
after inspecting that leaf parser and exercising a non-mutating invocation.

For review batching, `--plan` belongs to the local/offline mode. The GitHub mode
has a separate parser and publishes model review results to the PR; it is not an
offline preview. A safe documentation check must use complete local diff/manifest
inputs and a new report directory with no provider key.

Provenance: internal/cli/cli.go:135 (compatible router),
internal/reviewbatch/settings.go:56 (offline plan flag),
internal/reviewci/command.go:67 (separate GitHub flags).
Observed 2026-10-23 via a compiled `cmd/factory` offline plan: one file, one planned
batch, `status: planned`, `requests_used: 0`, exit 0. The documentation audit also
ran seven safe help invocations and budget/loop plans for all three harnesses.
This is CLI-contract evidence, not published-release migration qualification.
