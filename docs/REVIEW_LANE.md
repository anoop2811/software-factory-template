# The adversarial review lane (opt-in)

A model reviews the diff of every pull request and posts one advisory comment.

**In short**

- Off by default. It costs tokens on every PR and needs a secret only you can add,
  so it is never switched on for you.
- **Advisory, never a required check.** A model's opinion is not a computational
  control. The gates block; this one advises.
- `./factory review-lane enable` / `disable` / `status`.

## Turning it on

```sh
./factory review-lane enable        # installs the workflow, records the choice
```

### Provider credentials

**The lane cannot work until you add a repository secret.** Nothing in the
factory can do this for you.

> **GitHub → Settings → Secrets and variables → Actions → New repository secret**
> - **Name:** whatever `review_api_key_secret` says in `factory.yaml` — it
>   follows your provider (`OPENROUTER_API_KEY`, `ANTHROPIC_API_KEY`,
>   `OPENAI_API_KEY`)
> - **Value:** your API key for that provider

Until it exists the lane comments to say the secret is missing rather than
failing silently, and `./factory doctor` reports the lane as a warning on every
run — so this cannot be quietly forgotten. `factory init` and `factory upgrade`
also end with a highlighted **Action required** block naming the secret, printed
last so it does not scroll away.

Check any time with `./factory review-lane status`, or `./factory doctor`.

### Runner runtime

The generated workflow in current source acquires the exact installed release's
packaged review client using a separately pinned installer. Commit the generated
`factory` launcher and `.factory-version` together: the job reads their matching
release identity without running the launcher. It authenticates a client for the
runner's platform into private temporary storage; Go and Python are unnecessary.
The provider key is supplied only to review execution. Invalid identity or failed
acquisition stops the job. This requires a published release containing the new
bootstrap command; earlier candidates do not gain it retroactively. Integrated
release qualification is tracked in [release readiness](migration/GO_RELEASE_READINESS.md).

Default review keeps the single-request mode with your selected OpenRouter,
Anthropic or OpenAI provider. Explicit batching still requires a separately provisioned
compatible binary and `REVIEW_GO_CLIENT` set to its absolute path, as described
in [self-hosting](SELF_HOSTING.md#budgeted-batch-review). Canonical factory source
checkouts retain their declared Go toolchain and locked dependency preparation.
The workflow never builds an ordinary adopter application to obtain its client.

`disable` **removes** the workflow file rather than leaving it inert: a dormant
`pull_request_target` workflow sitting in a repository is an invitation to switch
on something nobody read.

## If you already have the factory installed

The Go upgrade preserves your review choice. An enabled, present workflow that
exactly matches its supported predecessor is refreshed in the same preview,
backup and recovery transaction as the installation. Its existing secret name,
provider, model and cost settings remain selected. Customized enabled workflows
are preserved as conflicts for you to reconcile. An absent workflow stays absent;
an off lane stays off. Use `./factory review-lane enable` when you choose to enable
review. See [upgrade consent and recovery](UPGRADING.md#ordinary-go-upgrades).

Older Bash upgraders may offer the lane interactively once. That historical prompt
is separate from the Go migration's preservation behavior.

## What it costs

Tokens on every pull request, at the **frontier tier** — the reviewer is the one
role the cost profile never routes to a cheap model
([COST_AND_TOKENS.md](COST_AND_TOKENS.md)), because a cheap skeptic is not a
skeptic. Set `review_model` in `factory.yaml` to pin a specific model; leave it
blank to use your provider's frontier tier. Diffs over 200 KB are skipped with a
comment saying so, rather than reviewed in truncated form.

## The privilege boundary

Posting a review comment needs write permission, which `pull_request` does not
grant on a fork — so the workflow uses `pull_request_target`, and that token is
exactly what makes this dangerous. Three constraints contain it:

1. **Same-repo pull requests only.** A fork PR never drives a job holding the
   token (`head.repo.full_name == github.repository`).
2. **The workflow and scripts come from the base commit**, never the PR head,
   with `persist-credentials: false`. A pull request cannot edit the thing that
   reviews it.
3. **The PR diff is data, never code.** It is fetched through the API and passed
   to a model. Nothing from the PR head is executed, sourced, or built.

## What the reviewer is told

The prompt asks the model to **refute, not assess** — a reviewer that summarises
is a reviewer that approves. It must cite `file:line`, may not praise or restate
what the change does, must label speculation as speculative, and is told that
**"No findings." is a valid and useful answer**. Inventing a finding to look
thorough is the failure mode the lane exists to avoid, so it is named in the
prompt itself.

It reviews for correctness, security, broken invariants, confabulated citations,
and over-engineering — in that order.

## What it is not

It is not a gate, and enabling it changes no gate. It sees a diff, not your
repository, and it says so when that limits it. Treat its output as a first pass
that costs a model instead of your attention — the deterministic checks are what
actually block a merge.
