# Runtime conversion includes lock and hash compatibility

Equivalent JSON values do not necessarily produce identical checkpoint hashes.
A new language must preserve the legacy encoding or explicitly migrate evidence.
Likewise, equivalent lock filenames are insufficient if lock primitives or inodes
differ. The conversion contract is specs/001-go-runtime-conversion.md; keep its
compatibility inventory as the source of the migration requirements.

Upgrades also need a transition boundary covering probes before budget admission;
an empty ledger alone cannot prove that legacy processes are quiescent. The
existing copy-manifest check discovers assets from shell copy statements, so a
Go replacement needs a non-empty explicit inventory and negative asset fixtures.

Provenance: baseline 76952eaa63aebd1ecd282f5ab51dd7c3627cb497, read 2026-09-06:
scripts/lib/loop.py:24 (digest encoding), scripts/lib/loop.py:271 and
scripts/lib/budget.py:194 (locking), scripts/lib/budget.py:484 (pre-admission
probe), scripts/hooks/copy-manifest-check.sh:36 (shell-derived asset discovery).
