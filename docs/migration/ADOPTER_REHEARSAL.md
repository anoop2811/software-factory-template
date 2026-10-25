# Installed doctor permission rehearsal

Observed 2026-10-25; correction contract: Decision 112 in
[the decision log](../DECISION_LOG.md). Detailed adopter-specific records remain
local. This document describes only the factory permission defect and its source
qualification. It does not claim successful published replacement qualification.

## Installed failure

An isolated adopter rehearsal using local source bb8815a identified a genuine
integration mismatch. A supported freshly initialized v0.1.6 installation upgraded
successfully and retained its required state. Its installed public doctor then
reported its own sourceable configuration bridge as non-executable, despite all
fourteen installed gate proofs passing.

The declared configuration library is a readable regular 0644 file, per
[ADR 0098](../adr/0098-whole-installation-upgrade-and-rollback.md).
`internal/installation/roles.go:49` generates that mode. Native doctor had applied
the executable check for invoked hooks to this library too. Existing source-only
fixtures chmod the library 0700 and did not exercise the installed mode.

## Correction and independent evidence

The diagnostic now requires all core entries to be regular and readable. It
requires execute access for invoked hooks, while treating the named configuration
library as sourced. Installation modes and proof execution remain unchanged.

The independent compiled-public-command criteria include healthy 0644,
missing/unreadable/non-regular libraries, missing/non-executable/unreadable/
non-regular invoked hooks, and an actual qualified installation's public doctor.
They also check preservation of the installed library's bytes, mode and inode.

```text
# Before production correction
Ran 9 of 3008 Specs in 149.658 seconds
FAIL! -- 5 Passed | 4 Failed

# After correction, including actual installed public doctor
Ran 9 of 3008 Specs in 148.215 seconds
SUCCESS! -- 9 Passed | 0 Failed

# All native doctor acceptance criteria
go test -race -count=1 -timeout=6m -v ./acceptance -ginkgo.focus='Native Go doctor' -ginkgo.no-color -ginkgo.fail-on-empty
Ran 60 of 3008 Specs in 127.811 seconds
SUCCESS! -- 60 Passed | 0 Failed

# Focused permissions after the lint-required switch refactor
go test -race -count=1 -timeout=3m -v ./acceptance -ginkgo.focus='Native Go doctor sourceable library permissions' -ginkgo.no-color -ginkgo.fail-on-empty
Ran 8 of 3008 Specs in 8.767 seconds
SUCCESS! -- 8 Passed | 0 Failed

go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run --config packs/go/.golangci.yml ./internal/doctorcmd ./acceptance
0 issues.

go vet ./internal/doctorcmd ./acceptance
exit 0; no output

# Repository-pinned security scanner, targeted doctor package
gosec ./internal/doctorcmd
Files: 4; Issues: 0
```

Three independent review lenses found no actionable correctness, security or
coverage issue. The scheduler criterion was observed failing its old 2999-case
oracle, then passing native disjoint/exhaustive discovery with
`3008 = 2866 base + 98 installation + 44 release-handoff`.

These are local source results. Exact-head hosted CI, independent advisory review
and a new signed candidate are still required. RC1 remains failed and v0.1.6
remains latest stable. No release gate is closed merely by this correction.
