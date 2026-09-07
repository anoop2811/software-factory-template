# Provider preference is not a provider pin

OpenRouter's provider order is a preference while fallbacks remain enabled.
Keeping a user-selected provider requires disabling fallbacks as well. Requiring
parameter support prevents routing to endpoints that silently ignore a supplied
control. These constraints can make a request unavailable; they do not establish
a dollar ceiling or prove the quality of a completed review.

Provenance: OpenRouter's official provider-selection documentation, fetched
2026-09-07 UTC:
https://openrouter.ai/docs/guides/routing/provider-selection

The factory's selected behavior is recorded in
docs/adr/0055-pin-deepseek-review-provider.md:20; this lesson points to that
decision rather than defining another configuration contract.
