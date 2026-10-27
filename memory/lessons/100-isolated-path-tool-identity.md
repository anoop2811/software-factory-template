# Isolation must name the tool it actually qualifies

A restricted test PATH can silently select a different executable from the
runner's qualified toolchain. Keep required real tools explicit while retaining
poisons for unavailable dependencies. A real-command timeout identifies a failed
boundary; a path mismatch alone does not prove why that command stalled.

Canonical qualification decision: docs/DECISION_LOG.md:4917. The native handoff
fixture inherits its restricted PATH from acceptance/release_bootstrap_test.go:99.

Provenance: observed 2026-10-26 UTC in merged-source run 40275252277. Checkout
uses `/opt/homebrew/bin/git`; after removal of its Git poison, the native fixture
selects `/usr/bin/git`. Its healthy ignore-proof Git initialization exceeded the
unchanged five-second bound. The underlying host-delay cause remains unproven.
[Retained source-run evidence](https://github.com/anoop2811/software-factory-template/actions/runs/40275252277).
