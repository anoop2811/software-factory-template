# Offline fixtures must disable optional service activation

Removing model settings does not make a diagnostic fixture offline. An exported
`REVIEW_LANE=on` overrides YAML `review_lane: off`; replacing a fixture's tool PATH
can then expose the operator's real GitHub client.

Give health-only fixtures an explicit disabled lane and a private client witness.
Keep an inherited-on challenge in the criterion so ordinary CI proves the boundary.
The contract is docs/adr/0129-go-source-doctor-entrypoint.md:83; production caller
configuration precedence remains unchanged.

Provenance: observed 2026-11-02 UTC during the three source-doctor acceptance
criteria with `REVIEW_LANE=on` and a recording fake `gh`: 2 passed, 1 failed, and
the fake recorded `secret list`. No real GitHub query was executed. Local evidence:
`.factory/qualification/r3-doctor/source-doctor-review-env-red.json`.
