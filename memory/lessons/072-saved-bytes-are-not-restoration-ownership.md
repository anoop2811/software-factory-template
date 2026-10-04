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
