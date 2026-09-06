# Compare trees before resolving conflicts after a stacked PR is squash-merged

A stacked branch can conflict with main even when main contains exactly the
changes from its earlier ancestor. Compare the trees before choosing conflict
sides: an identical tree establishes that there is no independent upstream
content to combine. Preserve the later branch corrections and merge main into
the branch to reconnect the histories without a force push.

Provenance: observed 2026-09-06 while resolving PR #65.
`git rev-parse 427676a^{tree} origin/main^{tree}` returned
`d4bd793b64549ae4cdbc7b6a3405686974319e9f` twice;
`git diff 427676a origin/main` produced no output.
The integration decision is recorded in docs/DECISION_LOG.md under Decision 42.
