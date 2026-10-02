# Recovery discovery exclusions

[ADR-0084](../adr/0084-exclude-inert-recovery-from-discovery.md) closes a
prerequisite for durable recovery creation. A local ignore rule alone does not
prevent recursive gates from reading obsolete code or docs.

The reserved `.factory/backups` node and its descendants are excluded from
citation source scanning and citation-target resolution, Java/TypeScript test
dialect scans, and Go tracked-test discovery. Active files elsewhere remain
subject to the same checks, including `.factory/backups-other` and a nested
application's ordinary `.factory/backups` directory. No file is removed from
Git's index. The separate [local writer](RECOVERY_CREATION.md) independently
refuses tracked backup paths before saving copies.

Citation docs_root cannot resolve into recovery storage: archived documentation
must not become the source of truth for active citations. Other document roots
retain their existing purpose; an ancestor scan prunes the reserved installation
recovery subtree while continuing to search active documentation.

## Audited boundaries and limits

- Go and Python loop source snapshots already exclude `.factory` contents.
- Go runtime source packaging selects `go.mod`, `go.sum`, `cmd/factory` and
  `internal`; runtime bundles have an explicit file allowlist.
- Adapter sync reads fixed active configuration/agent inputs, rather than
  recursively discovering instructions under recovery storage.
- Existing local-hook dispatch requires executable files; required recovery
  copies are inert files with mode 0600.
- Dedicated Go retrieval/context indexing is not implemented. Future discovery
  must explicitly exclude the recovery root before it can qualify for use.

This is not universal enforcement over arbitrary user-provided check commands,
external native scanners, or deliberate commands pointed at archived content.
Supported activation still needs explicit discovery/exclusion evidence. No
backup creator is enabled by this change; ignored storage, transaction ownership,
durability, restoration and bounded retention remain separate work.

## Qualification

Independent real-script Ginkgo/Gomega fixtures exercise exclusion together with
continued enforcement of active violations. Final RED/GREEN evidence and
current-head CI results belong in the PR. This prerequisite does not complete
the durable-recovery-creation milestone or change the conversion denominator.

Independent core RED before production edits:

```text
FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus="Recovery discovery exclusion core" -ginkgo.no-color -count=1 -v
FAIL! -- 0 Passed | 5 Failed | 0 Pending | 1746 Skipped
FAIL github.com/anoop2811/software-factory-template/acceptance 1.199s
```

Independent follow-up RED exposed two physical-root trailing-newline errors
before the sentinel correction. Final qualification ran the actual installed
scripts in all 50 scenarios:

```text
FACTORY_AGENT_ROLE=spec-writer go test -race ./acceptance -ginkgo.focus="Recovery discovery" -ginkgo.no-color -count=1 -v
Ran 50 of 1796 Specs in 3.140 seconds
SUCCESS! -- 50 Passed | 0 Failed | 0 Pending | 1746 Skipped
ok github.com/anoop2811/software-factory-template/acceptance 4.505s
```

The race detector covers the Go fixture runner; the production gates are Bash.
Bash syntax and ShellCheck passed for all four changed scripts. Independent
correctness and security reviews found no remaining actionable findings. Full
selftest and Linux/macOS CI evidence is recorded in the PR.
