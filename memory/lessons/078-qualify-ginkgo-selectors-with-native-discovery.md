# Qualify Ginkgo selectors with actual discovery

Observed 2026-10-10 via the race-enabled native discovery regression in
acceptance/source_gate_selection_test.go:261. An anchored family selector passed
the recording fixture but selected zero real installation specs because Ginkgo
matches the suite description before the spec text. The base skip consequently
selected all 2,655 specs; an empty installation selection still returned success.

Keep the recording fixture's names in the actual suite namespace and qualify
focus/skip counts with the pinned runner's dry-run. Also retain a real empty
selection failure control. The shared selector and qualification contract live
in docs/adr/0098-whole-installation-upgrade-and-rollback.md:543. Dry-run counts
prove selection only; they do not prove execution of the selected behaviors.

Observed 2026-10-19 via the published-release selection dry-run: the broad
`Published release qualification` focus selected four ordinary workflow criteria
without any live scenario. A build tag and `fail-on-empty` were insufficient when
another container satisfied the focus. The live-only full namespace now has a
five-criterion discovery check and a Go overlay removing its registration must
fail with zero selected criteria. See
acceptance/published_release_workflow_test.go:387 and
.github/workflows/published-release-qualification.yml:141. These controls qualify
selection, not a published release's behavior or authenticity.
