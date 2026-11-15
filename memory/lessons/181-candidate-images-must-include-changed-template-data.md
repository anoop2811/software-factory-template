# Candidate images must include changed template data

An artifact-level acceptance journey can combine the current executable with
stale template data if its private source snapshot overlays only Go production
files. A test comparing output with that same stale image can pass without
exercising the intended template change.

For installed review workflow qualification, include the changed review template
in the shared source snapshot before committing and packaging the fixture. Check
the generated workflow against the current reviewed template and immutable
installer pin as well as the authenticated image identity. Keep historical
predecessor data frozen; only the candidate overlay follows the current change.

Provenance: observed 2026-11-14 through source inspection of
`acceptance/installation_upgrade_test.go:261` and the explicit fresh-init output
assertion in `acceptance/default_init_test.go:298`. The integration contract is
`docs/adr/0154-installed-review-workflow-migration.md:104`. This records a local
qualification gap caught before submission, not a released user incident.
