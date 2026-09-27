# Reserve preview intent before argument validation

When adding a read-only route beside an existing mutating command, recognize the
preview marker before looking up or executing the old implementation. Validation
failure must terminate that selected route; it must never mean that the request
was not a preview and can be delegated to the mutating command.

Make ambiguous operand spellings explicit in the contract and test them against
a script that would leave a visible sentinel if executed. A read-only success
case alone does not prove that malformed preview input stays read-only. Also
state clearly when partial preview coverage does not describe legacy apply.

Provenance: docs/adr/0082-go-public-upgrade-preview.md:23 reserves preview intent
before legacy dispatch and documents the literal-path spelling; the same ADR's
public report contract requires the distinction between Go preview and legacy
apply. Independent compiled RED was observed on 2026-09-27 via the public preview
core suite, including the poison-script dispatch case.
