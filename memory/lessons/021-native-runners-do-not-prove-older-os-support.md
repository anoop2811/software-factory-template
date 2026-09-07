# Native runners do not prove older OS support

An architecture passing on a hosted runner establishes evidence for that runner's
OS, not every earlier OS in the intended support range. Keep minimum-version
qualification explicit when hosted labels no longer cover that minimum.

Provenance: fetched 2026-09-07 from
https://docs.github.com/en/actions/reference/runners/github-hosted-runners;
standard Intel macOS labels start at macOS 15 while the approved runtime scope
includes macOS 14. The contract remains in
docs/adr/0060-runtime-source-bundles.md:64 and the evidence matrix lives in
docs/migration/ARTIFACTS.md.
