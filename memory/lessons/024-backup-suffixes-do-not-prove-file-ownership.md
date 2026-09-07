# Backup suffixes do not prove file ownership

Provenance: `scripts/lib/config.sh:263` at immutable commit
`b6eab79cb1360e10baf6326c6fbbf2c6eee7f846`; independent setter characterization
observed 2026-09-07 against both reader baselines. See
`docs/adr/0063-go-configuration-writes.md` for the candidate decision.

The legacy `sed -i.factory-bak` replacement overwrites a pre-existing backup
sibling and then removes it. A familiar suffix is not evidence that the current
operation created or owns a file.

The candidate writer uses an exclusive temporary path and preserves pre-existing
siblings. The installation cleanup contract applies the same principle more
broadly: prove ownership and unchanged content before removing an old asset.
Consult the migration spec and ADR for the authoritative requirements.
