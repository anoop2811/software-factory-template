# Health checks must distinguish sourceable libraries from invoked hooks

An installed sourceable shell bridge can correctly be readable 0644 while an
invoked hook must remain executable. Applying one execute-bit requirement to both
makes a valid installation report unhealthy. Chmodding a source fixture executable
can mask the integration mismatch; qualify the actual installed modes and keep
negative controls for unreadable libraries and non-executable invoked hooks.

The canonical permission contract is docs/adr/0098-whole-installation-upgrade-and-rollback.md:392.
The observed source mismatch is internal/doctorcmd/command.go:135 and
internal/installation/roles.go:49. The original observation predates the correction; Decision 112 records the
independent regression and source correction.

Provenance: observed 2026-10-25 via `rtk proxy ./factory doctor` in the isolated
local-bb8815a adopter fixture, independently repeated by a reviewer: exit 1 with
`scripts/lib/config.sh is not executable`, while all 14 installed gate proofs passed.
The setup, comparison and retained evidence are in docs/migration/ADOPTER_REHEARSAL.md.
