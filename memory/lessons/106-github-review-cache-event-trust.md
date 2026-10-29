# GitHub review-cache access depends on the workflow trigger

Observed 2026-10-29 via Actions run 40819593500: the trusted review client received
HTTP 200 and content events, but its cache-save step was denied after cancellation.
GitHub reported `cache write denied: token has no writable scopes`.
Do not interpret a successful cache-action step as proof that its save succeeded.

GitHub's canonical cache policy gives default-branch cache writes to trusted
triggers including `workflow_dispatch`; `pull_request_target` defaults to read.
The existing review workflow's manual main-branch route is a supported recovery
path. Observe an actual save and restore before claiming hosted cache resumption.
Keep the trusted-main execution boundary and authenticate any loaded batch data.

Provenance: [observed run](https://github.com/anoop2811/software-factory-template/actions/runs/40819593500);
[GitHub dependency caching policy](https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching#cache-access-for-low-trust-workflow-triggers),
fetched 2026-10-29. Manual run 40821713165 then reported `Cache mode: write`
and `Cache saved with key`; cache API entry 9682743190 records a 12,955-byte
archive under `refs/heads/main`. Subsequent run 40824840127, attempt 3, observed
`129/129 files, 250/250 fragments, 14/14 batches completed (12 cached)`.
The automatic PR run 40821465804, attempt 3, then completed with all fourteen
batches cached. These reports establish authenticated reuse and complete
resumption; they do not establish the duration of an uncached review.
