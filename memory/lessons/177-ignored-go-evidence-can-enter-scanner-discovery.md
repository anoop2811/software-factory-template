# Ignored Go evidence can enter scanner discovery

Git ignore rules do not define every scanner's source inventory. Store Go overlay
replacement payloads with a non-Go extension, or outside the scanned repository,
and keep the explicit overlay mapping. Preserve the payloads as evidence without
making unrelated packages appear together in a scanner's discovered directory.

Provenance: observed 2026-11-13 UTC while running `gosec ./...` for PR #180.
Four private mutation payloads under the ignored
`.factory/ci-mode-evidence/fixture-controls-negative` directory had `.go`
extensions and mixed `acceptance_test` and `native` package declarations. Gosec
reported an import error and exited 1 despite its `Issues : 0` summary. Renaming
the payloads preserves the negative-control evidence; the command exit status,
not the issue count alone, determines whether qualification passed.
