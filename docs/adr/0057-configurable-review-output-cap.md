# ADR-0057: configurable adversarial-review output cap

Status: accepted for implementation, 2026-09-06 UTC.

The advisory OpenRouter review lane uses an output cap because the provider
request must be bounded. The fixed 4096-token cap produced an incomplete
review on a real pull request (`finish_reason: length`). Increase the default
OpenRouter cap to 8192 tokens and expose it as `review_max_tokens` in
`factory.yaml`, with `REVIEW_MAX_TOKENS` taking precedence for one CI run.

The value is an integer from 1024 through 32768. Reject malformed, zero,
negative, fractional, or out-of-range values before making an HTTP request.
Keep the cap provider-specific: Anthropic remains at its existing 4096-token
request contract and OpenAI remains unchanged. Keep the one-request policy,
diff-size bound, timeout, provider pinning, and incomplete-response refusal.

This is a ceiling, not a spending promise. Actual output and reasoning tokens
remain provider-billed. The default is deliberately large enough for the
configured DeepSeek reviewer to finish a bounded diff while remaining below
the provider's larger context limits. A later cost evaluation may lower the
default or use a smaller per-project value.

The configuration key is added to the fixed export allowlist and generated
factory configuration so GitHub Actions reads the trusted base commit's
setting. Environment precedence and explicit empty-value semantics match the
existing review settings.

Evidence: the incomplete review comment on PR #80 reported
`finish_reason=length`; OpenRouter documents that `max_tokens` is the maximum
completion allowance and that reasoning tokens count toward output usage:
https://openrouter.ai/docs/guides/best-practices/reasoning-tokens (checked
2026-09-06 UTC).
