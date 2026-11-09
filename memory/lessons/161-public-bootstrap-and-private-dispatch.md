# Public bootstrap must select the public dispatch path

An authenticated executable can still receive private dispatch settings from its
caller. The factory checks `FACTORY_BRIDGE_PROTOCOL` before entering the public
CLI. Forwarding the correct command arguments alone therefore does not establish
that the public operation will own the request.

The CI installer boundary clears that private setting before handing off to the
authenticated owner. This is scoped to the new public route; sourceable bridge
operations retain their own protocol. The authoritative boundary and qualification
requirements are in [ADR 0144](../../docs/adr/0144-ci-installer-bootstrap.md).

Provenance: `cmd/factory/main.go:93`, `install.sh:135`, and the poisoned-environment
public installer/real-project acceptance in `acceptance/ci_bootstrap_test.go`.
Observed 2026-11-08 via the seven-criterion private race run recorded in
`docs/DECISION_LOG.md` Decision 160; all seven criteria passed.
