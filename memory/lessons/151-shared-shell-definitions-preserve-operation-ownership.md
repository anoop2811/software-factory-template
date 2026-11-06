# Shared shell definitions must preserve operation ownership

A canonical shell bridge can define more public functions than its immediate
consumer needs. Loading it may replace functions sourced earlier, even when
every individual library passes its own checks. Test both source orders with
distinct owner receipts, not output alone: two runtimes can return the same text.

Observed 2026-11-06 via the R3 helper review's private Dash recorder: duration
selected the caller Git runtime before sourcing color, then selected the color
source root afterward. The independent source-order regression failed against
that implementation. Limiting color's source-root fallback to palette/action-box
requests restored the non-color owner in both source orders; the final helper
run reported `7 Passed | 0 Failed`.

Provenance: `scripts/lib/color.sh:78`,
`acceptance/source_helper_entrypoints_test.go:206`, and
`docs/adr/0137-go-source-shared-helpers.md:36`.
Local execution receipts: `.factory/qualification/r3-helpers/helpers-source-order-red.log`
and `.factory/qualification/r3-helpers/helpers-final-green.log`.
Decision 152 records the design and correction; this lesson does not replace it.
