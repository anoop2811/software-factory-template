# Saved bytes are not restoration ownership

A checked saved original does not establish who replaced today's file. Keep
reverse authority separate from before-image integrity; an equal-byte new
inode must remain a conflict. The canonical boundaries are
docs/adr/0094-live-publication-restoration.md:34 and
docs/adr/0094-live-publication-restoration.md:78.

Provenance: observed 2026-10-04 UTC through independent actual production-API
Ginkgo qualification, including equal-byte inode replacement, later-edit
preservation and real prepared/applied SIGKILL controls. This is source-component
evidence, not released public rollback or installation acceptance.

```text
FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 go test -race -timeout=120s -v ./acceptance -ginkgo.focus='Live publication' -ginkgo.no-color
Ran 85 of 2467 Specs in 38.762 seconds
SUCCESS! -- 85 Passed | 0 Failed | 0 Pending | 2382 Skipped
ok github.com/anoop2811/software-factory-template/acceptance 41.092s
```

The command ran through `rtk proxy` with the task Go cache and qualified Python
3.12.14 PATH. Unselected specs account for the focus skips; no selected case skipped.

Retaining the actual file also has to survive a failed observation after rename.
An explicit retry must qualify that candidate before accepting its state; the
canonical requirement is docs/adr/0094-live-publication-restoration.md:214.
Provenance: observed 2026-10-04 UTC via four actual-rename, actual-stat-then-EIO
Ginkgo controls before correction, followed by this independent corrected run:

```text
FACTORY_AGENT_ROLE=spec-writer go test -race -timeout=120s -v ./internal/assessment -ginkgo.focus='Live publication retained candidate' -ginkgo.no-color
Ran 26 of 187 Specs in 1.335 seconds
SUCCESS! -- 26 Passed | 0 Failed | 0 Pending | 161 Skipped
ok github.com/anoop2811/software-factory-template/internal/assessment 2.692s
```

This command ran through `rtk proxy` with the task Go cache. The four original
regressions were independently RED; the remaining controls include conflicts
that already refused. This is injected source-test evidence, not an incident.

Retained candidate checks and later ordinary storage checks must share the same
observation-error classification. Canon:
docs/adr/0094-live-publication-restoration.md:259.
Provenance: observed 2026-10-04 UTC via the two actual named-stat-then-EIO
controls in internal/assessment/publication_test.go:808. The unchanged controls
failed before correction and passed afterward; preservation and exact resource
release assertions preceded the status assertion in each control.

The same classifier must also cover a named recheck after a retained descriptor
has been deliberately closed. Canon:
docs/adr/0094-live-publication-restoration.md:267.
Provenance: observed 2026-10-04 UTC via the actual sync/close/stat regression in
internal/assessment/publication_test.go:888, RED before the single-branch
correction and GREEN afterward. The explicit retry and exact resource-release
checks completed before the status assertion.
