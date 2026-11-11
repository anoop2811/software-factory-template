# Project workflows are not runtime custody

Observed 2026-11-10 during the existing-workflow migration design: adding a
customizable workflow to the installation's ordinary active role list would also
make its bytes an installed-command admission requirement. The same control
selection is used for cross-platform checkout preparation. A later workflow edit
could therefore block the CLI, or host preparation could replace origin-specific
workflow selectors.

Keep this distinction visible when extending migration: files that a transaction
can manage need not be files whose exact bytes every runtime command requires.
The decision and subsequent qualification belong to
[ADR 0147](../../docs/adr/0147-installed-ci-migration-preview.md); this lesson does
not define another ownership policy.

Provenance: `internal/installation/generate.go:104` constructs descriptor assets;
`internal/installedlayout/layout.go:175` validates them on admission;
`internal/initcmd/ci.go:111` obtains the origin/host control selections. These
locations were inspected at merged commit
`8f2c6ae27f47a1ddbffc76cc10bdceffd424f2c4` before the preview implementation.
