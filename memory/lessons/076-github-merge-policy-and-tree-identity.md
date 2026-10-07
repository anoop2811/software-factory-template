# GitHub merge policy and tree identity

Repository-wide merge options do not establish what a protected branch permits.
For this merge, the repository advertised merge commits, but main required linear
history. A rebase merge preserved the commits and satisfied that branch policy.
Re-read the branch policy before future merges rather than assuming this setting
is permanent. Use the authenticated account's default merge identity when an
explicit merge-author email is rejected.

Canon: docs/FACTORY_RULES.md describes server-side branch protection and the
Verification Contract. This lesson records one observed delivery boundary rather
than defining a new repository policy.

Provenance: observed 2026-10-06 UTC via the following GitHub CLI actions:
- gh api repos/anoop2811/software-factory-template returned allow_merge_commit=true.
- gh api repos/anoop2811/software-factory-template/branches/main/protection returned required_linear_history.enabled=true.
- gh pr merge 123 --merge was rejected with "Merge commits are not allowed on this repository."
- gh pr merge 123 --rebase --match-head-commit 93106b3409a9eb752cd0f81a5a274d605d1800ab returned exit 0.
- gh pr view 123 returned MERGED, mergedBy=anoop2811 and mergeCommit=6b46e90661fa8a49a337a449dec8f1748b2fcc2e.

After fetching main, git diff --exit-code HEAD main returned exit 0 with no output.
Both commit trees were f29829e43564fc11109ae375fa7fd8d687b5050e despite the changed
commit IDs. Local main and origin/main matched the observed merge commit.
The source tree comparison qualifies unchanged checked content; it does not
replace the separate hosted CI result.
