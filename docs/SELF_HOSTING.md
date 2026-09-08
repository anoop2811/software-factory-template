# Using the factory to develop the factory

This repository is both the factory implementation and the source of adopter
scaffolds. Its own factory.yaml selects checks and enforcement values. Shared
hooks, role separation, native adapter generation, self-tests and Go acceptance
run against changes to the factory itself. ADR-0052 records the CI review setup
and local-hook activation in docs/adr/0052-self-hosted-adversarial-review.md:51.

## The development loop

Write the decision/spec, establish an independent failing test, implement, run
the factory checks, and submit a human-reviewed PR. The factory's own entrypoints
provide the evidence:

```sh
./factory doctor
make check
make eval
./scripts/pre-push-check.sh
```

`make check` includes core, budget, loop and fake-HTTP review fixtures, Go race
acceptance and quality tools, plus shared policy hooks. `make eval` checks wiring
for OpenCode, Claude and Codex without model calls. The full pre-push command
also checks generated-adapter drift, commit messages and decision references.

Enable the tracked local Git hook once per checkout installation:

```sh
git config --local core.hooksPath .githooks
```

This tracked hook blocks direct pushes to main; it does not run the whole
pre-push suite. The full command above and CI provide the broader checks.
Client hooks can be disabled, so they do not replace GitHub branch protection.

## Adversarial CI review

The source repository opts into the same `factory review-lane` capability
available to adopters. factory.yaml records GLM 5.3 Flash
(`z-ai/glm-5.3-flash`), DeepInfra routing, and the
`OPENROUTER_API_KEY` secret name. The shared review
runner defaults to OpenRouter; no native harness model setting is needed.

Add the key through GitHub Settings > Secrets and variables > Actions. Never
commit it. After the workflow is merged, same-repository PR open/update/reopen
events targeting the default branch fetch the diff as data and post an advisory
review using trusted base scripts. Fork PRs and other base branches are excluded. The model's opinion is not a required merge gate.

The lane permits one HTTP request per run, a 200000-byte diff, a 1200-second
total request deadline (with a separate 15-second connection limit) and a configured 32768-token OpenRouter output cap for this repository.
The shared runner defaults to 8192 when no cap is configured. Set
`review_max_tokens` in `factory.yaml` or `REVIEW_MAX_TOKENS` in the environment
to a decimal value from 1024 through 32768; the caller value wins. An explicitly
empty `REVIEW_MAX_TOKENS` overrides YAML and uses the shared 8192-token default.
Leave the environment variable unset to use the configured YAML cap. Oversized diffs are
skipped explicitly; truncated responses and failures are reported as unperformed
or incomplete reviews. OpenRouter's routing and billing remain provider-owned;
these controls are not a strict dollar ceiling. Local fixtures never call a model.

Set the GitHub Actions repository **variable** `REVIEW_TIMEOUT_SECONDS` to a
whole number from 1 through 1200 to override the deadline. It is a variable, not
a secret; an empty/unset value uses 1200. Locally, set the same environment
variable. The Actions job allows 25 minutes, leaving five minutes beyond the
maximum request deadline for setup and reporting. This transport setting is
intentionally not a factory.yaml key.
Generated review-lane workflows carry the same variable binding. Older installed
workflows need their normal upgrade before that binding is available.

The deadline controls waiting time, not the model's token allowance. Timeouts
now report safe HTTP/timing/byte evidence and never publish partial findings.
They do not prove the provider performed no work or incurred no cost. There is
no automatic retry or provider fallback. The PR #87 failure hit the old hard-coded
180-second curl deadline; its discarded partial response cannot establish why
the provider took longer. See [ADR-0065](adr/0065-adversarial-review-transport-deadline.md);
[Decision 65](DECISION_LOG.md#decision-65-2026-09-08-utc-allow-bounded-long-review-completion)
supersedes its original 480-second ceiling after the observed PR #92 timeout.

OpenRouter reasoning effort is selectable with `review_reasoning_effort` in
factory.yaml or `REVIEW_REASONING_EFFORT` in the environment (caller wins).
No effective value means provider defaults; an explicitly empty caller value
also overrides configured effort. The gateway values are
`none`, `minimal`, `low`, `medium`, `high`, `xhigh`, and `max`; choose only values
supported by your model. This repository selects `low`, the lowest supported
GLM 5.3 Flash API effort; thinking cannot be disabled for this model. Reasoning
tokens share this repository's 32768-token output allowance with the final review. Low effort
does not guarantee a complete or accurate review. See
[Decision 62](DECISION_LOG.md#decision-62-2026-09-08-utc-use-supported-glm-review-reasoning)
for the correction to Decision 61's reasoning setting,
[Decision 63](DECISION_LOG.md#decision-63-2026-09-08-utc-bound-the-glm-review-output-allowance)
for the repository cap increase after an observed truncated review, and the
[model's reasoning requirements](https://docs.z.ai/guides/capabilities/thinking).

OpenRouter hosting is selectable with `review_openrouter_provider` in factory.yaml
or `REVIEW_OPENROUTER_PROVIDER` in the environment. Caller values, including an
explicit empty value, override YAML and legacy factory.config. Empty means omit
the routing override. Use a lowercase provider slug such as `deepinfra`, or an
endpoint slug such as `deepinfra/fp8`. A nonempty selection sends one provider in
`order`, with `allow_fallbacks: false` and `require_parameters: true`. This keeps
requests on that provider and requires support for the supplied parameters.
If no eligible endpoint is available, the review fails visibly; it does not
fall back to another provider. The repository's `deepinfra` selection permits
DeepInfra endpoint variants. Anthropic/OpenAI requests and native harness role
tiers are unchanged. The provider-routing contract remains in
[ADR-0055](adr/0055-pin-deepseek-review-provider.md); its historical DeepSeek model
selection is superseded by [Decision 61](DECISION_LOG.md#decision-61-2026-09-08-utc-select-glm-53-flash-for-adversarial-review).

The workflow checks out the trusted base commit. A PR changing these settings
therefore runs with the previous configuration; the new selection takes effect
after merge and a subsequent eligible PR event. Fake-HTTP fixtures establish the
request shape, not live DeepInfra completion or review quality.

## Source templates and remaining boundaries

Run adoption/install/upgrade exercises in temporary adopter repositories. Running
factory-init over this source tree would substitute canonical placeholders with
one repository's identity and model selections. The existing self-tests exercise
those real adopter entrypoints in isolated fixtures.

Keep global native-harness settings unset unless deliberately changing how the
source scaffold is represented. In particular, setting model_provider while
leaving model tiers blank makes sync-opencode remove source model placeholders
(scripts/sync-opencode.sh:47). The CI review configuration avoids that signal.

The source .github/CODEOWNERS still contains adopter placeholders; it is not
proof of owner coverage for this repository. Full server-side human-review
protection needs its own concrete configuration and validation. Likewise,
structural adapter tests do not establish live native-agent enforcement, and
Go candidates do not establish packaged upgrade, recovery or cleanup acceptance.
Optional budgeted model runs and bounded loops remain opt-in.

## Evidence for this iteration

`./factory doctor` ran 217 self-tests: `217 passed, 0 failed, 0 skipped`.
It exposed the unset local hooks path, missing review secret and an adapter-drift
warning. After local hook activation, `hookspath_status` reported `armed` for
this worktree's tracked .githooks/pre-push. `make check-drift` exited 0 after the
review provider was kept separate from native template configuration.

The review request fixtures ran before implementation with
`adversarial-review: 12 passed, 2 failed` (missing cap and accepted truncated
response), then after implementation with `adversarial-review: 14 passed, 0 failed`.
Command: `./scripts/selftest/adversarial-review.sh`. Additional workflow/config
regressions now pass with `adversarial-review: 17 passed, 0 failed`. A copied-tree
negative control weakening only the generated workflow guard returned
`adversarial-review: 16 passed, 1 failed` and identified that generated file.
These are structural workflow assertions and fake-HTTP behavior, not live GitHub
permission or model evidence. At that point live OpenRouter CI was pending
secret setup, workflow merge and a subsequent PR event.

After PR #76 merged and the secret was added, updating PR #77 triggered
[the first live run](https://github.com/anoop2811/software-factory-template/actions/runs/34084339301)
on 2026-09-07 UTC. The authenticated response ended with `finish_reason=length`;
the lane posted an incomplete-review notice. A successful workflow status did
not establish a completed model review. Explicit reasoning-effort configuration
was added afterward. That historical run does not establish completion with
the currently selected model and provider.

The initial doctor drift warning was traced to an empty .claude/hooks directory:
sync-claude creates it, Git does not store it, and the doctor's directory snapshot
comparison sees it as a change in a fresh checkout. A later doctor run after
sync reported no adapter drift. The fresh-checkout false positive remains a
separate doctor issue; this iteration does not claim to fix it.

GitHub branch protection was read on 2026-09-06 via
`gh api repos/anoop2811/software-factory-template/branches/main/protection`:
strict `selftest` status checks and admin enforcement are enabled, while
`required_approving_review_count` is 0 and `require_code_owner_reviews` is false.
Those settings were inspected, not changed by this iteration.
