# Separate Git identity isolation from authenticated transport

Metadata reads and an authorized remote refresh have different environment
requirements. Clearing all Git overrides protects identity discovery but also
removes the user's explicit authentication transport during refresh. Keep that
distinction at the command boundary; preserve inherited transport values without
reconstructing them, including exported empty values.

Canon: docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:265 records the
bounded transport contract; specs/001-go-runtime-conversion.md:307 requires
environment compatibility. Do not maintain another allowlist here.

Observed 2026-11-01 UTC through the actual installer upgrade regression: the
existing branch criterion failed on all three refresh calls for both literal and
exported-empty transport values, then passed after the scoped repair. Identity,
redirection, caller-environment and handoff controls remained required. The
session receipts are `source-auth-refresh-{red,green}.json` under
`.factory/qualification/source-entrypoints-20261101/`; PR #157 tracks the repair.
