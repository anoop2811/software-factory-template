# Establish the native fixture state before asserting safety

Observed 2026-10-02 during recovery qualification: native setuid/setgid bits were
stripped immediately after chmod, and invalid UTF-8 directory creation was
refused in an otherwise writable temporary parent. An ordinary file must not be
counted as an unsafe-mode fixture, and an unavailable filename shape must not be
counted as a successful containment test.

Read back the requested state before invoking the behavior. Report conditional
skips when the platform cannot represent it, keep ordinary control cases, and
retain portable metadata/raw-byte controls. Provenance: the independent compiled
fixture checks in
[recovery_inventory_test.go](../../acceptance/recovery_inventory_test.go), at
`acceptance/recovery_inventory_test.go:313` and
`acceptance/recovery_inventory_test.go:575`,
plus the 43-case internal recovery race run recorded in
[RECOVERY_CREATION.md](../../docs/migration/RECOVERY_CREATION.md).
