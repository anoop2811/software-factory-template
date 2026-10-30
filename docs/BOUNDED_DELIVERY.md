# Bounded delivery

Plan one complete, observable function per pull request. A reviewer should be
able to follow the requirement, implementation and evidence without first
untangling unrelated changes. Use this practice with Codex, Claude Code or
OpenCode; it does not depend on a particular model or harness.

## Choose a complete boundary

Describe what a user or maintainer can do after the PR merges. Include the
production path, independent tests, compatibility behavior, required cleanup and
documentation for that function. Keep unrelated functions in separate PRs.

For example, initialization and upgrading an existing installation can be two
PRs. Each needs its own end-to-end behavior and failure checks. Splitting a new
command from the tests or recovery it needs would leave an incomplete boundary.

Prefer a coherent change over an arbitrary line count. A mechanical generated
file may be large; a short change spanning unrelated responsibilities may still
need splitting. The aim is a review whose complete context a person or model can
inspect, with evidence tied to that change.

## Work in parallel, merge in order

1. Identify the functions and their dependencies before implementation. Record
   each PR's scope, prerequisites and acceptance criteria.
2. Give independent work separate branches or worktrees and explicit file
   ownership. Share changes to common APIs before dependent work relies on them.
3. For behavioral changes, have an independent author write meaningful failing
   tests, implement the behavior, then run its checks and review workflow. Use
   relevant existing checks for documentation or other low-impact changes.
4. Address and respond to review findings, then merge after that PR's checks and
   required human reviews pass. Rebase dependent branches on the merged result
   and rerun their relevant checks.
5. Run the integrated acceptance journey after its prerequisites merge. Parallel
   implementation does not replace the final migration or release qualification.

A preview-only reporting change may proceed alongside a separate command. A
bootstrap that calls that command depends on its native dispatch being ready;
retiring the old implementation depends on its final consumer moving first.
Make those edges explicit rather than bundling the whole migration into one PR.

## What the factory checks

Complete-function PR scope is a planning and review practice. There is no
universal, automatically enforced PR line limit. The configured gates still
check each change, and [workflow recipes](WORKFLOWS.md) keep independent review,
finding verification and deterministic coordination explicit.

[Batched review](CLI.md#batched-review-and-ci) is a separate computational
control: it plans complete diff coverage within configured per-request limits
and review budgets. Splitting a PR does not replace those limits or guarantee a
provider response. [Budget controls](BUDGETS.md) govern factory-managed model
launches; they do not measure whether a PR has one coherent purpose.

The factory applies this practice to its own Go conversion. The project decision
and dependency constraints are recorded in
[ADR 0114](adr/0114-incremental-go-cutover-delivery.md), especially
`docs/adr/0114-incremental-go-cutover-delivery.md:15`.
