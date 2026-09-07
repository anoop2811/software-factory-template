# Reasoning shares the review output budget

The first live GLM 5.3 Flash CI review hit `finish_reason=length` with a
4096-token output allowance. The workflow succeeded in posting an honest
incomplete-review notice; it did not produce a completed model review.

Observe the posted outcome as well as the Actions conclusion. Reasoning tokens
count toward billed output, and GLM defaults to max reasoning. A smaller effort
setting is a configurable tradeoff, not proof that the next review will finish.
The configuration decision remains in
docs/adr/0054-explicit-review-reasoning-effort.md:17; retained limits and the
repository choice are in docs/adr/0054-explicit-review-reasoning-effort.md:25.

Provenance: observed 2026-09-07 UTC via GitHub run 34084339301 and
https://github.com/anoop2811/software-factory-template/pull/77#issuecomment-5565176443;
https://openrouter.ai/docs/guides/best-practices/reasoning-tokens and
https://openrouter.ai/api/v1/models fetched 2026-09-07 UTC.
