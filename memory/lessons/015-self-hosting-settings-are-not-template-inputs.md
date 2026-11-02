# Self-hosting settings can change template inputs

The factory source repository is also its adopter scaffold. Before enabling a
capability for the source repo, trace whether a shared setting triggers adopter
substitution elsewhere. A CI-only review model should not turn native scaffold
placeholders into one repository's runtime configuration.

Observed 2026-09-06 via `make check-drift`: adding model_provider without native
model tiers made sync-opencode remove the canonical model placeholders. The
historical script uses presence of model_provider as a configured-adopter signal
in [the preserved implementation](https://github.com/anoop2811/software-factory-template/blob/1754de23de95a7e3c734bedd86894295e823c988/scripts/sync-opencode.sh#L47).
Removing that new setting and restoring only the
resulting generated edits retained the source scaffold. The review runner's
OpenRouter default is defined at scripts/adversarial-review.sh:44.

The adopted policy and remaining dogfood boundaries are recorded in
ADR-0052 (docs/adr/0052-self-hosted-adversarial-review.md:60) and
[SELF_HOSTING.md](../../docs/SELF_HOSTING.md); this lesson does not replace them.
