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

Then add the repository secret it names — GitHub → Settings → Secrets and
variables → Actions. The name follows your provider (`OPENROUTER_API_KEY`,
`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`) and is recorded as `REVIEW_API_KEY_SECRET`
in `factory.config`. Without it the lane comments to say so rather than failing
silently.

`disable` **removes** the workflow file rather than leaving it inert: a dormant
`pull_request_target` workflow sitting in a repository is an invitation to switch
on something nobody read.

## What it costs

Tokens on every pull request, at the **frontier tier** — the reviewer is the one
role the cost profile never routes to a cheap model
([COST_AND_TOKENS.md](COST_AND_TOKENS.md)), because a cheap skeptic is not a
skeptic. Set `REVIEW_MODEL` in `factory.config` to pin a specific model; leave it
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
