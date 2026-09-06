# Repair-loop ownership and verification trust

Paul Stack's post distinguishes who controls repair from where checks execute.
For the factory, this suggests refining existing Loop engineering and Harness
engineering roadmap items with bounded repair and structured verification evidence.
This is a research recommendation, not an implementation decision or completed feature.

A receipt should identify the tested snapshot and relevant policy/tool inputs.
Matching hashes can establish identity and freshness; an author-controlled receipt
cannot alone establish that execution honored those inputs. Preserve independent
checks at the merge boundary. Begin cost optimization with duplicate local checks.

Provenance (fetched 2026-09-06):
- https://stack72.dev/the-feedback-loop-is-moving-out-of-ci/ (published 2026-09-03)
- https://slsa.dev/spec/v1.2/threats-overview
- https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue

Repository anchors at a080fa5: scripts/pre-push-check.sh:41 and
scripts/pre-push-check.sh:87 invoke the broad and diff-aware gates;
scripts/hooks/diff-aware-check.sh:149 can dispatch the configured protected-path
check again. scripts/golden-task-eval.sh:175 already fingerprints some eval inputs.
GitHub merge-queue availability depends on repository ownership/plan; do not
assume the personal factory repository supports it. Roadmap canon remains
in docs/FEATURE_ROADMAP.md; this lesson does not change priorities or percentages.
