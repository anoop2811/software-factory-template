# ADR-0055: select DeepSeek review through DeepInfra

Status: accepted for implementation, 2026-09-07 UTC; model selection superseded by [Decision 61](../DECISION_LOG.md#decision-61-2026-09-08-utc-select-glm-53-flash-for-adversarial-review). Provider-routing contract retained.

The user explicitly selected DeepSeek V4 Flash 0731 through DeepInfra for this
repository's advisory review. Set review_model to
deepseek/deepseek-v4-flash-0731, review_reasoning_effort to none, and
review_openrouter_provider to deepinfra. The existing reasoning.effort none
request disables thinking rather than merely hiding reasoning output. Keep
model_provider absent: this selection must not rewrite native harness models.
Native role tiers remain unchanged; this choice applies only to the advisory
HTTP review lane and does not assert evaluated review quality.

Add optional review_openrouter_provider / REVIEW_OPENROUTER_PROVIDER through
the existing fixed-key configuration export and legacy loader. Explicit caller
environment, including an empty value, takes precedence over YAML, then legacy
factory.config. Add the key to both existing export key lists. Empty or absent
selection omits the provider object and preserves routing for existing adopters.

For a nonempty OpenRouter route, send exactly the constrained provider object
{"order":["ROUTE"],"allow_fallbacks":false,"require_parameters":true}.
The single order entry and disabled fallbacks keep the requested provider;
require_parameters prevents routing to an endpoint that cannot honor supplied
parameters. The repository selects the base slug deepinfra, not an immutable
endpoint. A base provider can expose several endpoints, including deepinfra/fp8.

Accept only route slugs matching ^[a-z0-9]+([-/][a-z0-9]+)*$ and reject other
nonempty values before HTTP. Route strings remain literal JSON data, never shell
code. This constrained grammar supports base providers and endpoint names.
Do not validate or apply this OpenRouter-only setting to native OpenAI or
Anthropic requests; preserve their existing request and response contracts.

Retain the one-request policy, bounded timeout (now defined by ADR-0065), incomplete-response refusal,
diff-size bound, and trusted-base workflow controls. ADR-0057 supersedes the
initial 4096-token output limit with the validated, configurable cap. Provider
unavailability must not silently select a different
provider or trigger another paid invocation. No strict dollar ceiling, speed,
review quality, or successful live completion is claimed by changing settings.

Independent fake-HTTP acceptance must first fail for configured routing, caller
override, and invalid-route refusal. Cover absent and explicit-empty omission,
legacy/YAML precedence, literal malicious input, accepted base/endpoint slugs,
provider isolation, and the actual repository model/reasoning/route combination.
The test suite exercises the real shared review script with captured JSON and
fake curl; source/configuration checks do not prove a completed live review.

Authoritative sources checked 2026-09-07 UTC:
- https://openrouter.ai/docs/guides/best-practices/reasoning-tokens — effort none
  disables reasoning; excluding reasoning output is a different operation.
- https://openrouter.ai/docs/guides/routing/provider-selection — provider order,
  allow_fallbacks false, and require_parameters true control endpoint selection.
- https://openrouter.ai/api/v1/models/deepseek/deepseek-v4-flash-0731/endpoints —
  DeepInfra endpoint tag deepinfra/fp8 lists reasoning and reasoning_effort among
  supported parameters; this is metadata, not observed review completion.
