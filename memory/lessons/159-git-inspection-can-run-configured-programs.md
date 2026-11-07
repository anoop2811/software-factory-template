# Read-only Git inspection can run configured programs

Observed 2026-11-07 during Decision 158's independent review: a private Git
`ls-files` probe executed the configured fsmonitor even when its selected path
produced no output. A separate disposable submodule probe executed a child-local
clean filter during the superproject's worktree diff. A different-size dirty
file initially avoided conversion; a same-length dirty control exposed it.

Neutralizing helpers in one repository does not neutralize configuration in a
submodule inspected recursively. Keep each content comparison nonrecursive and
apply the same inspection policy to separately validated child worktrees.
The contract and bounds live in
[ADR 0143](../../docs/adr/0143-ci-checkout-preparation.md); this lesson does not
replace that policy or claim final acceptance completion.

Also observed in the private qualification checkout: `git check-ignore -z`
requires stdin, and global `--literal-pathspecs` causes that command to reject
otherwise valid literal names. The corrected literal NUL-input probe exited 0.
Check the actual combined command, not just each option in isolation.

Provenance: Decision 158 review probes, `internal/initcmd/ci.go:202` and
`internal/initcmd/ci.go:373`, and the official
[check-ignore](https://git-scm.com/docs/git-check-ignore) and
[diff](https://git-scm.com/docs/git-diff) manuals fetched 2026-11-07.
