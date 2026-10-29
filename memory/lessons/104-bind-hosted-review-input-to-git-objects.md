# Bind hosted review input to Git objects

Observed 2026-10-29 via `gh api repos/anoop2811/software-factory-template/pulls/141 -H 'Accept: application/vnd.github.diff'`: HTTP 406, "diff exceeded the maximum number of lines (20000)". Raising model output allowances cannot repair an upstream input refusal.

GitHub documents that PR and compare pages may calculate changes from different
merge bases. A successful per-file patch request alone does not establish its
old-side context. Keep complete coverage, exact revision bindings and input
limits; qualify patch context and the resulting Git blob identity before model
spend. Canon remains [ADR 0109](../../docs/adr/0109-complete-large-review-inputs.md).

Provenance: observed 2026-10-29 via the command above; official comparison semantics
fetched 2026-10-29 from https://docs.github.com/en/pull-requests/reference/pull-requests.
