# Cancellation must also bound error publication

A signal-aware controller can reconcile every owned child and still hang while
reporting its failure to a full inherited stderr pipe. Child cleanup and caller
completion are separate observations; draining the pipe during the test would
hide the latter defect.

Provenance: observed 2026-10-31 UTC through the compiled source CLI's independent
`SIGINT with blocked stderr` and `SIGTERM with blocked stderr` criteria. Both
real pipes held 65,536 bytes with idle readers; owned groups were absent while
the callers remained alive and their six-second exit checks failed. The RED run
reported 0 passed and 2 failed in 14.290 seconds.

The canonical diagnostic-publication and descriptor-custody contract is
`docs/adr/0120-native-source-gate-proof.md:131`.
