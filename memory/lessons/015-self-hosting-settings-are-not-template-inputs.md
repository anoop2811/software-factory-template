# Self-hosting settings can change template inputs

The factory source repository is also its adopter scaffold. Before enabling a
capability for the source repo, trace whether a shared setting triggers adopter
substitution elsewhere. A CI-only review model should not turn native scaffold
placeholders into one repository's runtime configuration.

Observed 2026-09-06 via `make check-drift`: adding model_provider without native
model tiers made sync-opencode remove the canonical model placeholders. The
script uses presence of model_provider as a configured-adopter signal at
scripts/sync-opencode.sh:47. Removing that new setting and restoring only the
resulting generated edits retained the source scaffold. The review runner's
OpenRouter default is defined at scripts/adversarial-review.sh:44.

The adopted policy and remaining dogfood boundaries are recorded in
ADR-0052 (docs/adr/0052-self-hosted-adversarial-review.md:60) and
[SELF_HOSTING.md](../../docs/SELF_HOSTING.md); this lesson does not replace them.
