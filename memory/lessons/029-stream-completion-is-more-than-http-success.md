# Stream completion needs protocol evidence

OpenRouter can commit HTTP 200 before producing a token, then emit an error in
an SSE event. Heartbeats establish a live connection, not generation progress.
Its Chat Completions accounting frame repeats the terminal finish_reason, so
rejecting every repeated terminal reason would reject valid responses.

Use the completion, bounded retry and private-output contract in
[ADR-0067](../../docs/adr/0067-streaming-adversarial-review-client.md); do not infer
review success from the HTTP code or connection bytes. Retry-After is guidance
for explicit rate limiting, not permission to replay an ambiguous generation.

Provenance: official OpenRouter
[streaming documentation](https://openrouter.ai/docs/api_reference/streaming)
and [error documentation](https://openrouter.ai/docs/api_reference/errors-and-debugging),
fetched 2026-09-09 UTC. This lesson records documented protocol behavior, not
an observed live billing or reliability guarantee.
