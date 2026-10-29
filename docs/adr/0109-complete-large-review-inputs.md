# ADR 0109: retain complete hosted review input beyond GitHub's diff limit

Status: implemented and locally qualified; hosted review and merge pending

Date: 2026-10-29 UTC

## Context

PR #141 at 67c27b201a3818d70ca90ea3ac292b1ecd826c4d has 118 changed files.
The normal GitHub diff request returned HTTP 406 and the explicit diagnostic
"the diff exceeded the maximum number of lines (20000)". No provider request
was made. Raising response tokens cannot repair an upstream acquisition refusal.
The paginated file API returned all 118 patches with matching independent
additions/deletions in an observed read-only probe; that observation does not
qualify a general fallback implementation.

The existing [batched review contract](0100-batched-review.md)
requires complete changed-file/hunk coverage, exact revision binding, explicit
budgets and trusted-base execution. A skipped or incomplete review never counts
as complete. Keep those requirements.

## Acquisition contract

Try the normal raw diff first. Only its exact HTTP 406 may select the following
bounded fallback. Authentication, permission, transport, oversized 200 and other
errors retain their existing refusal; do not reinterpret them as permission to
use weaker input.

Reuse the complete paginated changed-file manifest. Use fixed same-repository
GitHub REST endpoints with exact hexadecimal identities, never response-supplied
URLs or refs from patch text. Resolve the base/head comparison's merge-base
commit and exact commit-tree identities, then require complete nontruncated
recursive trees. GitHub documents a recursive-tree 100,000-entry/7MB ceiling and
its explicit truncated flag: [tree API](https://docs.github.com/en/rest/git/trees#get-a-tree).
A truncated or missing tree is not evidence that a path is absent.

PR and compare pages can calculate changed files from different merge bases;
see [GitHub comparison semantics](https://docs.github.com/en/pull-requests/reference/pull-requests).
Do not infer patch context from a current base tree or a compare result alone.
For every changed regular text blob, obtain the selected old blob by its exact
Git object ID, validate its encoding/size and Git blob identity, apply every hunk
strictly against its context/removal bytes, and require the resulting Git blob
ID to equal the head tree's actual object ID. Added content starts from the empty
blob; removed content must leave the empty blob. Mismatch refuses with no model
spend. This proof also detects a cached/incompatible patch base.

Construct canonical quoted Git headers from verified lifecycle, path, blob and
mode evidence. Preserve executable-mode changes and exact rename/copy metadata.
A zero-content mode change or pure rename/copy needs equal proven blob IDs.
Added/removed empty files need explicit empty-blob evidence. Missing patches
with unexplained changed content, binary data, unsupported types/type changes,
malformed paths, duplicate identities or truncated/contradictory patches refuse;
never fabricate an empty diff or silently omit such a file.

Reuse the existing strict batch parser before the first model call: unchanged
manifest status, per-file additions/deletions, hunk bounds, path/lifecycle/mode
and aggregate input checks still apply. A narrow strict patch-application helper
may reuse that parser's existing hunk representation; do not fork a permissive
second diff grammar. Preserve context, CRLF and no-final-newline bytes, with
cancellation and bounded output. Git SHA1 here is an existing object-format
identity check, not a replacement for repository/ref/TLS authentication.

Bound each response and aggregate decoded source bytes, changed-file count and
assembled input using existing 8 MiB input limits. Tree traversal is bounded and
must reject truncation. Provider request/token/byte allowances, deadlines and
cached-completion validation remain unchanged. Recheck exact current PR identities
after preparation and before publication. No PR source checkout, generator,
hook, shell program or response-provided URL is executed to obtain a diff.

## Qualification and trusted deployment

Independent outside-in Ginkgo/Gomega criteria must observe meaningful pre-fix
failure. Exercise an actual >20,000-line 406 response, all content reaching bounded
batches, lifecycle/modes/quoted paths, strict context and resulting-blob binding,
missing/truncated content/trees, non-406 refusal, revision drift and publication
safety. Existing normal-diff and hostile-input criteria remain unchanged.

The hosted workflow intentionally builds its client from trusted main before
exposing credentials. Land this bounded repair in a small separate main-based PR,
review it through the existing normal-diff path, then rebase and rerun PR #141.
Never execute PR-head code with the write token or provider key to work around
this deployment boundary. Any required larger review allowance stays explicit;
complete acquisition does not itself mean a model review completed.

Sources above were fetched 2026-10-29 UTC. The observed 406 and full 118-file API
probe are retained in local qualification evidence; no provider billing is
inferred from unused budget allowances.

## Lifecycle reconciliation

Complete tree evidence and the manifest must explain the same non-directory
changes. A rename does not explain an unchanged original path retained in head;
a copy alone does not explain deletion of its source. An explicit additional
manifest row may explain a recreated, modified or removed source path. Reject
unexplained tree deltas or contradictory lifecycle claims before model spend,
including a PR/compare-base mismatch that leaves extra changes outside the
manifest. Directory tree-ID changes alone are not separate file changes.
Track old-path consumption separately from new-path coverage: reading a copy's
source does not account for overwriting an already present destination. Explicit
additional lifecycle edges may account for that old destination; its mere
appearance as a new filename is insufficient.

## Local qualification

The independently authored hosted-review suite now includes 49 new criteria and
21 existing criteria. It covers exact ordered delivery of all 20,001 changed
lines, strict context and blob identities, lifecycle reconciliation and refusal
before provider calls. Six lifecycle omissions produced false completion on the
pre-correction source; two explicitly explained lifecycle controls remain valid.
An independently injected duplicated middle line fails the exact-sequence check.

RAN with Go 1.27.3 and both CLI and outer race instrumentation:

```sh
FACTORY_CLI_TEST_RACE=1 go test -race -count=1 -v ./acceptance \
  -ginkgo.no-color -ginkgo.focus='Hosted budgeted adversarial review' \
  -ginkgo.fail-on-empty
```

Observed output: `Ran 70 of 3057 Specs in 43.281 seconds` and
`70 Passed | 0 Failed | 0 Pending | 2987 Skipped`.
Skipped criteria are outside the selected focus, not unreported passes.
The scheduler census independently observes 2,915 base, 98 installation and 44
handoff criteria, with its selected partition assertion passing. Source reviews
and verified negative controls close the two retained lifecycle/coverage findings.
These local results do not claim a hosted model review or new-head CI pass.
