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
available to adopters. factory.yaml records the selected GLM 5.3 Flash model
(`z-ai/glm-5.3-flash`) and `OPENROUTER_API_KEY` secret name. The shared review
runner defaults to OpenRouter; no native harness model setting is needed.

Add the key through GitHub Settings > Secrets and variables > Actions. Never
commit it. After the workflow is merged, same-repository PR open/update/reopen
events targeting the default branch fetch the diff as data and post an advisory
review using trusted base scripts. Fork PRs and other base branches are excluded. The model's opinion is not a required merge gate.

The lane permits one HTTP request per run, a 200000-byte diff, a 180-second
request timeout and a 4096-token OpenRouter output cap. Oversized diffs are
skipped explicitly; truncated responses and failures are reported as unperformed
or incomplete reviews. OpenRouter's routing and billing remain provider-owned;
these controls are not a strict dollar ceiling. Local fixtures never call a model.

OpenRouter reasoning effort is selectable with `review_reasoning_effort` in
factory.yaml or `REVIEW_REASONING_EFFORT` in the environment (caller wins).
No effective value means provider defaults; an explicitly empty caller value
also overrides configured effort. The gateway values are
`none`, `minimal`, `low`, `medium`, `high`, `xhigh`, and `max`; choose only values
supported by your model. GLM 5.3 Flash requires reasoning and supports `low`,
`high`, and `max`, defaulting to `max`. This repository selects `low` within the
same 4096-token allowance. Reasoning tokens share that allowance with the final
answer. Low effort trades reasoning depth for a better chance of completion;
it does not guarantee a complete review. Anthropic/OpenAI requests are unchanged.
See docs/adr/0054-explicit-review-reasoning-effort.md:17.

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
was added afterward; its live completion remains pending merge and a new event.

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
