# ADR-0054: explicitly budget reviewer reasoning effort

Status: accepted for implementation, 2026-09-07 UTC.

The first live OpenRouter review of PR #77 reached the existing 4096-token
completion limit and was correctly reported as incomplete. Evidence:
https://github.com/anoop2811/software-factory-template/actions/runs/34084339301
https://github.com/anoop2811/software-factory-template/pull/77#issuecomment-5565176443
A green advisory workflow does not imply a completed model review.

Official model metadata and the Z.ai model card show GLM 5.3 Flash defaults to
max reasoning and requires reasoning. The failed run did not retain token-usage
details, so reasoning exhaustion is a plausible explanation, not a measured
attribution of its 4096 tokens. Do not disable mandatory reasoning or hide a
truncated answer as a completed review.

Add optional review_reasoning_effort / REVIEW_REASONING_EFFORT for OpenRouter.
The environment takes precedence through the existing config export. Empty means
omit the reasoning object and retain provider defaults for existing adopters.
Accept the documented gateway effort values none, minimal, low, medium, high,
xhigh and max; reject any other nonempty value before HTTP. A model may support
only a subset, and its provider may reject an unsupported value. Do not apply
this OpenRouter setting to Anthropic or OpenAI requests.

At the time this decision was written, the repository selected low while
retaining max_tokens 4096. ADR-0057 supersedes that output-cap value: the
current default is configurable and 8192. The diff bound, 180-second timeout,
single request, truncation refusal and trusted-base workflow guards remain.
Reasoning effort still trades reasoning depth against the configured allowance;
completion and review quality are not guaranteed.

Independent fake-HTTP tests must first fail for configured effort, environment
precedence and invalid-setting refusal, then pass after implementation. Preserve
legacy unconfigured requests, other providers and incomplete-response behavior.
Document selectable values and the GLM subset in the self-hosting guide. Record
the first live outcome honestly; a later successful review still requires this
fix on the trusted default branch and a new PR event. Hold additional large-PR
requests until the small PR returns a completed review.

Authoritative sources fetched 2026-09-07 UTC:
- https://openrouter.ai/docs/guides/best-practices/reasoning-tokens — reasoning
  counts as billed output; gateway effort options and model capability discovery.
- https://openrouter.ai/api/v1/models — z-ai/glm-5.3-flash reports mandatory true,
  default_effort max, supported_efforts max/high/low.
- https://huggingface.co/zai-org/GLM-5.3-Flash — model card Note documents low,
  high and max, with max the default.
