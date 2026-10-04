# Retained directories do not restart enumeration

Keeping a validated directory descriptor preserves identity, not its initial
enumeration position. A second directory read on an exhausted descriptor can
mistake existing entries for an empty namespace. Capacity checks need a complete
checked count or a separately qualified enumeration. The canonical contract is
docs/adr/0095-durable-live-publication.md:219.

Provenance: observed 2026-10-04 UTC through independent actual-record Ginkgo
controls in the R2.1 correctness and security reviews. A 63-record baseline
admitted record 64; a checked 64-record baseline incorrectly admitted record 65.
Both paired race runs produced one pass and one failure. The persistent
spec-writer reproduction then ran:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -timeout=120s -v ./internal/assessment -ginkgo.focus='Durable publication independent record quota|Durable publication inspection I/O precedence' -ginkgo.no-color
Ran 5 of 199 Specs in 2.103 seconds
FAIL! -- 3 Passed | 2 Failed | 0 Pending | 194 Skipped
FAIL github.com/anoop2811/software-factory-template/internal/assessment 2.658s
```

That combined run includes the separate inspection I/O-precedence regression.
This is source-test evidence, not a released customer incident.
