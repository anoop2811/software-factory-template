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

Observed 2026-10-20 via [PR #133 source-CI run 39364383333](https://github.com/anoop2811/software-factory-template/actions/runs/39364383333/job/118437939519):
all 44 handoff criteria passed, but the suite still failed its deadline. The
retained JSON showed four indivisible ordered groups, with both longest groups
on one worker. The primary worker's long cleanup included Ginkgo's peer barrier,
not measured fixture deletion. Inspect per-group timelines and the framework's
barrier before attributing elapsed time to cleanup or adding workers. Ordinary
randomization cannot split ordered groups. The bounded scheduling contract is
in docs/adr/0107-published-release-qualification.md:194; a predicted critical path
is not a measured speedup.

Observed 2026-10-20 while preparing that scheduler regression: Ginkgo v2.32.1's
`SpecReport` exposes priority and ordered-container fields, but its custom
`MarshalJSON` omits both. Deserializing a standard report therefore yielded zero
and false even for the four ordered groups. Check the serializer as well as the
public type before treating an absent report field as observed metadata.
Provenance: [pinned SpecReport serializer](https://github.com/onsi/ginkgo/blob/v2.32.1/types/types.go),
read from the qualified module source; the retained failed-run JSON likewise
lacks those keys. This observation changes the evaluator, not the canonical
scheduling contract in docs/adr/0107-published-release-qualification.md:222.
