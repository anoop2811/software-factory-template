# Follow review claims through finalization

A controller can temporarily populate a field that the ledger later clears or
rejects. Verify the complete call chain before describing a review finding as
persisted accounting corruption. Defensive guards can still make the controller
contract clearer without implying that the alleged production failure existed.

Provenance: PR #97 advisory comment at
https://github.com/anoop2811/software-factory-template/pull/97#issuecomment-5771595819,
checked 2026-09-22 UTC against internal/budget/lifecycle.go:199 and :232, and
internal/native/execute.go:197. The no-PID collaborator regression observed
parsing before the controller correction; it did not demonstrate persisted usage.
