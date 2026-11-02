# Config path precedence is command-specific

When replacing a direct script with an existing Go owner, audit that command's
configuration resolver instead of copying a neighboring command's fixture.
Prerequisite checks return an explicit relative config path unchanged; doctor
resolves it against the selected project. Give caller cwd, Git root and runtime
owner distinct fixture values so accidental rebasing is observable.

Provenance: inspected 2026-11-02 in `internal/prereqcmd/command.go:114` and
`internal/doctorcmd/command.go:95`. The bounded conversion contract remains
docs/adr/0130-go-source-prerequisite-entrypoint.md:25.
