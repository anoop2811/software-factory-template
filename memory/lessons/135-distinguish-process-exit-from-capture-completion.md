# Distinguish process exit from capture completion

A pending `exec.Cmd.Wait` does not by itself prove its process remains alive.
A separate live process can retain a captured output writer. Test witnesses
must relinquish their captures while preserving release and unrelated
descriptors. Keep the actual Wait assertion and observe process liveness
independently.

Provenance: observed 2026-11-01 UTC through the controlled duplicate-capture
RED and witness-isolation GREEN. The launcher was absent, its compiler child
was absent, and the borrowed witness remained alive while Wait awaited capture
EOF. The original [macOS CI failure](https://github.com/anoop2811/software-factory-template/actions/runs/41334297601/job/124931145532)
lacked that liveness/descriptor evidence, so its cause was not established by
that receipt. The contract is
docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:241; exact commands
and observations remain local in
`.factory/qualification/source-entrypoints-20261101/qualification.json` under
`source_wait_fixture_diagnosis`.
