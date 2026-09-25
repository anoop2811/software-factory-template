# Atomic read snapshots can change inodes

An atomic writer may replace history after a read-only consumer inspects the
pathname but before it opens or finishes reading the file. A blanket inode
identity rejection can misclassify cooperating publication as unsafe storage.
Distinguish proven snapshot replacement from malformed or unsafe files, bound
any read-only retry, and never apply that retry to admission or publication.

Provenance: `docs/adr/0070-go-budget-execution-controller.md:144` defines the
cooperating-replacement boundary. Observed 2026-09-22 UTC through the independent
Ginkgo cases in `internal/budget/runner_test.go` for replacement before open,
before descriptor inspection and during parsing; the prior strict reader failed
those cases. The ADR remains canonical for limits and file-safety qualification.
