# Late validation must retain error kind

A boolean metadata validator loses the distinction between a changed object and
an observation that failed. In an inspector, that can turn an operational error
into a conflict or erase an earlier read failure when rows are discarded. Keep
classification through final validation and reset; the canonical contracts are
docs/adr/0095-durable-live-publication.md:265 and
docs/adr/0095-durable-live-publication.md:273.

Provenance: observed 2026-10-04 UTC through independently authored PR #122
regressions at test commit 7e275f0. Complete real record reads and final native
metadata observations preceded one-shot reported EIO. Both inspectors returned
status 2 instead of 1; recovery inspection also lost read EIO after actual mode
invalidation. Preservation, descriptor closure and inert-authority checks passed
before the classification assertions failed.

The author ran the following with the qualified interpreter PATH:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer GOCACHE=/private/tmp/factory-durable-recovery-go-cache FACTORY_CLI_TEST_RACE=1 go test -race ./internal/assessment -run '^TestAssessment$' -ginkgo.focus 'Durable publication review' -ginkgo.no-color -count=1
Ran 22 of 235 Specs in 7.372 seconds
FAIL! -- 12 Passed | 10 Failed | 0 Pending | 213 Skipped
FAIL github.com/anoop2811/software-factory-template/internal/assessment 7.934s
```

The reported EIO was injected after real operations; this is source-test evidence,
not naturally occurring kernel EIO or a released customer incident. Passing
metadata-only controls are separate qualification evidence, not RED claims.
