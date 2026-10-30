# Historical role defaults need an independent oracle

An omitted JSON field is not evidence of the historical runtime's default.
The abstract `canonical_source_assets` map omits `required`, while RC3's compiled
role table explicitly marks the cached `scripts/lib/loop.py` row required.
The active legacy script and its inert scaffold-cache copy have different roles.
Match the complete path and read the actual immutable generation recipe.

The RC3 acceptance fixture initially treated the omitted field as Go's false
zero value and expected an edited cache row to remain a successful optional
action. The consumer correctly returned a preserved ownership conflict instead.
The corrected assertion requires that exact path-specific conflict and an
unchanged adopter. The separate Git/AST oracle compares every original role
field, including `Required`, against the immutable compiled table.

Provenance: observed 2026-10-30 UTC during the RC3 compatibility race run
(`public-oracle-correction.json`: three passes, one evaluator assertion failure).
Immutable source is `95486ecdd3868a319a0e6362ee99cafd8f154200`;
`internal/installation/roles_v1.go:132` preserves its required cache row,
and `docs/migration/INSTALLATION_ROLE_MAP.json:927` records the abstract entry.
The preservation contract remains in
[ADR 0115](../../docs/adr/0115-frozen-rc3-recipe-compatibility.md#closed-reconstruction-contract).
