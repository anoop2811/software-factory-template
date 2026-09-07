# Passing Go tests does not establish a passing Go quality gate

Run the quality tools pinned in the repository's CI, alongside behavioral
tests. Local `go test` and `go vet` passed while CI rejected unused fixture
parameters and reported G703 in the shared fixture writer. Constructing
malformed manifests from explicit fixture data removed the taint warning
without suppressing security checks.

Provenance: observed 2026-09-07 via `gh run view 34136327916 --log-failed`
for PR #81. Both operating-system jobs passed race-enabled acceptance tests
before golangci-lint reported four issues. The pinned linter invocation
`go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run --config packs/go/.golangci.yml ./...`
reported `0 issues.` after the fixture changes.
