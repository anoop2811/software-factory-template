# Bind source execution to its private image

Atomic cache publication does not bind a later exec to that published image.
Keep invocation ownership through dispatch; an independent cache copy lets the
executable retain the single-link and pathname custody needed by native proof.
Clean only the pinned invocation file after return.

Provenance: observed 2026-11-01 UTC in the independent A/B interleaving RED.
Both processes returned zero, but A printed B's distinct native help after B
replaced the shared cache. The same two criteria subsequently passed, followed
by all twelve source criteria. See acceptance/source_invocation_custody_test.go:54
and docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:189. Exact RED,
GREEN and preservation receipts are local under
`.factory/qualification/source-entrypoints-20261101/source-hosted-review-*`.
