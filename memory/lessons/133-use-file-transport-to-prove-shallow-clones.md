# Use file transport to prove shallow clones

A local-path clone can ignore `--depth`; it cannot prove a shallow installer
works. Use file transport and record the actual shallow state when qualification
depends on missing historical objects. Keep transport recorders separate from
real packaging and initialization evidence.

Provenance: observed 2026-11-01 UTC through the actual installer and a fresh
`file://` source cache. Native packaging failed with `fatal: not a tree object`
while shallow was true; the source and target remained unchanged. After native
history acquisition, that same real journey initialized successfully with
shallow false. See acceptance/source_entrypoints_test.go:538 and
docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:169. Exact RED/GREEN
receipts remain local under `.factory/qualification/source-entrypoints-20261101/`.
