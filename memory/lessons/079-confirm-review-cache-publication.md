# Confirm cache publication before assuming a review can resume

Observed 2026-10-22 UTC by reading PR #133's retained workflow logs and cache
inventory. The PR-target review's cache-save step was green, but its log reported
`cache write denied: token has no writable scopes`; its retry reused zero batches.
The existing trusted-main manual route saved a cache, and the next manual run
completed all fourteen files and three batches with one authenticated cached
batch. A successful save-step status alone was not evidence of publication.

Inspect the actual save log, cache entry and final `batches_cached` count before
claiming reuse. The authenticated batch contract remains in
docs/adr/0100-batched-review.md:44. GitHub's trigger-specific cache access policy
is authoritative; do not infer cache write access from unrelated token scopes or
relax workflow permissions merely because a restore succeeds.

Provenance: [denied PR-target run](https://github.com/anoop2811/software-factory-template/actions/runs/39368565451/attempts/2),
[trusted cache producer](https://github.com/anoop2811/software-factory-template/actions/runs/39432048962),
[completed cached review](https://github.com/anoop2811/software-factory-template/actions/runs/39613630071),
and [GitHub cache access reference](https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching#cache-access-for-low-trust-workflow-triggers)
(fetched 2026-10-20 UTC).
