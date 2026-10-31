# Source assets and the proof driver have different authority roots

An invoked source dispatcher can select its asset directory without granting
authority to a colocated executable. Native source selftest snapshots the actual
running executable; an installed selftest instead keeps the complete runtime
path confined beneath its validated installation root. Opening only that path's
parent would change the installed boundary.

Provenance: `docs/adr/0120-native-source-gate-proof.md:48`,
`docs/adr/0120-native-source-gate-proof.md:55`, and
`docs/adr/0120-native-source-gate-proof.md:61`. Observed 2026-10-31 UTC through
the compiled public CLI's twenty-four source-proof criteria, including hostile
driver lookalikes, PATH/symlink invocation, real child cancellation and typed
uncertainty retention. The combined source/CLI/installed/census run passed
52 criteria in 390.434 suite seconds. This observation does not qualify a published
release or the broader historical shell suite.
