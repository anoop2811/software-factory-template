# Factory migration and release evidence

For an existing v0.1.6 installation, start with the [upgrade guide](../UPGRADING.md). It gives the pinned installer command, explains the proposal and shows how to keep or restore the previous installation. For a new project, use the [initialization guide](INIT.md).

| What do you need? | Where to look |
|---|---|
| See what passed on each platform and in the adopter project | [Stable v0.2.0 release evidence](GO_RELEASE_READINESS.md#stable-v020) |
| Understand CI in a new project or a disposable checkout | [CI preparation](CI_PREPARATION.md) |
| Decide which old files stay recoverable | [Backup retention](BACKUP_RETENTION.md) |
| Look up a day-to-day command | [Installed CLI reference](../CLI.md) |
| Evaluate a particular prerelease | [Candidate handoff](GO_CANDIDATE_HANDOFF.md) |

The implementation checkpoints, source tests and RC reports in this directory record what was available at the time each component was developed. Their private bridge commands and development fixtures are maintainer interfaces. The guides above describe the supported public workflow.
