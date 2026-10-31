# Inventory changes require consumer-fixture audits

When shrinking a selected inventory, enumerate literal asset paths in existing
consumer tests as well as inventory counts. Establish which source generation
each fixture actually installs. A path required by a historical image may be
unowned data in a current installation, while a removed planning input may no
longer belong in the installed private cache at all.

Keep the behavioral obligation and move its fixture to the current declared
asset. Separately preserve unknown data and historical-generation requirements.
Do not restore an unused cache asset solely to satisfy an outdated assertion.
The migration policy remains in
[ADR 0119](../../docs/adr/0119-minimal-qualified-initializer-inventory.md).

Provenance: observed 2026-10-31 UTC through GitHub Actions jobs
[124418800327](https://github.com/anoop2811/software-factory-template/actions/runs/41161089293/job/124418800327)
and [124418800421](https://github.com/anoop2811/software-factory-template/actions/runs/41161089293/job/124418800421)
at `c55943b`. They record a stale private-README precondition and a current
fixture treating omitted loop-cache data as required. The follow-up contract
was recorded before test edits in
`docs/adr/0119-minimal-qualified-initializer-inventory.md:157`.
