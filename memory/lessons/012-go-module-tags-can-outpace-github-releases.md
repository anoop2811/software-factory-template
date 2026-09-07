# Verify Go CLI pins against module tags

A GitHub latest-release page can lag the authoritative Go module version.
For a Go CLI pin, compare the module's latest version with the upstream tag,
then run the tool against every target the check is meant to cover.

Observed 2026-09-06: GitHub Releases identified govulncheck v1.1.4 while
`go list -m -json golang.org/x/vuln@latest` identified v1.7.0, timestamp
2026-08-13T18:01:04Z. The authoritative tag is
https://go.googlesource.com/vuln/+/refs/tags/v1.7.0
(commit 617f44b718537dccdea1915395650e0529e3b72e).

The older tool passed the local Darwin scan but panicked in its bundled
x/tools v0.29.0 SSA builder for Linux sources under Go 1.27.1. Provenance:
https://github.com/anoop2811/software-factory-template/actions/runs/34071286184
and local reproduction with `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 govulncheck
./...`. The v1.7.0 scan returned `No vulnerabilities found.` for Linux amd64
and Darwin arm64 without changing scan mode.

Decision 49 in `docs/DECISION_LOG.md:1800` records the correction; the active
pin belongs to `.github/workflows/go-runtime.yml`, not this lesson.
