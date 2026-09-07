# Custom linter rules can disable defaults

When refining one linter rule, inspect the effective rule set. A narrow-looking
configuration can replace the defaults rather than extend them.

Observed 2026-09-06 via `GL_DEBUG=revive golangci-lint run --config
packs/go/.golangci.yml ./...` using golangci-lint v2.13.2: adding only a
`dot-imports` rule for the blessed Ginkgo/Gomega packages reported
`Enabled by config rules (1): dot-imports.` The tool listed 23 defaults before
applying that configuration. The explicit `enable-default-rules: true` setting
preserves those defaults alongside the package allowlist.

Provenance: the effective shared policy is `packs/go/.golangci.yml:15`;
Decision 49 records the integration and review correction in
`docs/DECISION_LOG.md:1800`. This lesson points to that policy rather than
maintaining another linter configuration.
