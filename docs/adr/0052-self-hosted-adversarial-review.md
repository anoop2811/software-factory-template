# ADR-0052: enable the repository's OpenRouter adversarial review

Status: accepted for implementation, 2026-09-06.

The user explicitly requested adversarial review in CI for this repository,
using OpenRouter and GLM 5.3 Flash. Enable the existing opt-in lane described
in docs/DECISION_LOG.md:947. Preserve its advisory status and human merge review.
This is repository configuration, not a change to the default opt-in policy
for factory adopters or the native Codex/Claude/OpenCode model selections.

Record review_model: z-ai/glm-5.3-flash; retain the OpenRouter provider default,
review_api_key_secret: OPENROUTER_API_KEY, and review_lane: on in factory.yaml.
Generate .github/workflows/adversarial-review.yml through factory review-lane.
Only the secret name belongs in Git. The Actions secret must be supplied through
GitHub; a missing secret is reported as an unperformed review, never approval.

Retain the trusted-base checkout, same-repository PR guard, diff-as-data input,
200000-byte maximum diff, 180-second HTTP timeout, one request without automatic
retries, and cancellation of superseded workflow runs. Add an explicit 4096-token
OpenRouter response limit; Anthropic already has that limit. Leave the unrelated
OpenAI request contract unchanged. An OpenRouter response with finish_reason length must
report an incomplete review and fail the review runner instead of presenting a
truncated answer as completed review evidence. No strict dollar ceiling is claimed.

Update the shared review-lane checkout pin to the verified v7.0.1 commit so that
the generated workflow and future lane generation use the same immutable source.
The privileged job must keep persist-credentials: false and never check out or
execute PR-head content. Keep the existing provider HTTP script canonical.

Independent tests must fail at the real script boundary before the request-cap
and incomplete-response changes. Use a fake curl that captures request JSON and
returns controlled responses; no model calls or credentials are needed locally.
Exercise configured model/provider selection, the output cap, literal diff data,
one request per review, missing-secret refusal, successful findings, and explicit
truncation refusal. Confirm generated workflow/configuration and its privilege
boundary, and include the deterministic fixtures in Template CI and make check.

The pull_request_target workflow must land on the base/default branch before
it can review PRs. Opening this configuration PR does not activate its own new
workflow; after merge and secret setup, a new PR event can exercise the live lane.
The separate Go reader PR remains independent and is not merged automatically.

Sources read 2026-09-06:
- https://openrouter.ai/z-ai/glm-5.3-flash — model identifier; released 2026-08-26.
- https://openrouter.ai/docs/api_reference/parameters — max_tokens upper limit.
- https://github.com/actions/checkout/releases/tag/v7.0.1 — released 2026-07-20.
- https://api.github.com/repos/actions/checkout/git/ref/tags/v7.0.1 — commit
  3d3c42e5aac5ba805825da76410c181273ba90b1, resolved before pinning.
- https://openrouter.ai/docs/api_reference/overview — normalized response finish_reason.

Self-hosting refinement, before local configuration changes: the user's follow-up
asks to dogfood the factory in this repository. Use its real doctor, check,
pre-push, review-lane and harness structural-eval entrypoints. Activate the
existing tracked .githooks via local core.hooksPath, which was unset. Do not run
factory-init over this source tree: adopter substitution would overwrite the
scaffold's canonical placeholder files. Record remaining doctor warnings as
actual gaps; an installed-looking hook or placeholder CODEOWNERS is not proof
of enforcement. Keep optional paid execution and automatic loops opt-in.

Provider-scope refinement from the dogfood run: model_provider is a native
harness-setting signal to sync-opencode (scripts/sync-opencode.sh:47). Setting
it in the source repository causes sync to remove its model placeholders when
native tiers are unset. Keep that key absent here and use the existing review
runner's OpenRouter default (scripts/adversarial-review.sh:44), with an explicit
review model and secret name. This keeps review independent from native harness configuration
and preserves the template inputs. Restore only the generated edits from
this diagnostic run; do not commit adopter substitutions into canonical source.

Privileged-base refinement before the review fix: the same-repository head guard
is necessary but not sufficient to establish a trusted base. Require the PR base
branch to equal github.event.repository.default_branch as well, before starting
the secret-bearing job. Mirror this conjunction in the shared lane template and
generated instance. PRs targeting other branches are excluded; no unprotected
feature-branch checkout may become the source of a script holding the review key.
An independent structural guard regression must fail before the template change.
