# Command-wide signals need stage-wide cancellation

A signal context installed at a CLI entrypoint intercepts termination during
preparation as well as the operation being repaired. Qualifying compiler
cancellation did not qualify the packager's earlier Git source reads. A Git
descendant could retain a captured descriptor after its leader was killed,
leaving the newly signal-aware parent waiting instead of exiting.

Review the whole signal-covered call chain. Use readiness receipts from actual
owned processes, then exercise cancellation at each subprocess boundary. Keep
large archive compatibility separate from bounded in-memory command responses.
The contract and repair belong in [ADR 0110](../../docs/adr/0110-local-compiler-process-ownership.md#signal-handling-integration).

Provenance: observed 2026-10-30 UTC through two independent compiled V1
`factory-package` probes against base `78648c6` and the compiler-only candidate.
After parent-only SIGTERM, the base exited in 0.001 seconds; the candidate
remained blocked for 6.505 and 6.504 seconds until each evaluator released its
own Git descendant's capture. Healthy controls succeeded, no bundle was
published after cancellation, and owned fixture PIDs were absent after cleanup.
These observations establish the regression, not release readiness.
