# A check can dispatch mutating builds

Before running a gate in a working checkout, trace the commands it dispatches.
`scripts/hooks/diff-aware-check.sh:104` invokes `make check-drift`, whose sync
prerequisite runs the source launcher and publishes its developer build to
`factory-go`. A check can therefore succeed while replacing an ignored local
artifact. A previously recorded hash detects that replacement but cannot restore
bytes that were never saved.

Run this gate in an isolated committed source copy when preserving the working
checkout's generated files or developer binary. Include indirect dispatch in the
same execution boundary as direct launchers.

Provenance: observed 2026-11-01 UTC while running
`bash scripts/hooks/diff-aware-check.sh origin/main HEAD` in the primary checkout.
The output reported two dispatched sync/drift checks; a subsequent SHA256 check
showed the ignored developer binary had changed. Tracked files and the existing
package/tool-version edits were unchanged. This was a qualification execution
mistake, not a failing synchronization implementation.
