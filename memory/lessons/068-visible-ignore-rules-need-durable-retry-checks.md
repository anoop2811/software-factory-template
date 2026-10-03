# A visible ignore rule does not establish crash durability

Observed 2026-10-02 through PR #118's Copilot review and nine independent failing
retry/reuse tests: a complete ignore rule can remain visible after its file or
parent sync fails. A retry that trusts `git check-ignore` alone can then save
durable recovery payloads without re-establishing durable local exclusion.

Require the canonical local rule and repeat its file sync/readback and parent
sync before creation or reuse. Preserve interrupted evidence and refuse another
failed sync. The contract is
`docs/adr/0091-durable-local-recovery-creation.md:185`; the recorded RED and GREEN
commands are in [RECOVERY_CREATION.md](../../docs/migration/RECOVERY_CREATION.md).
The original finding is
[Copilot's retry review](https://github.com/anoop2811/software-factory-template/pull/118#discussion_r4168835784).
