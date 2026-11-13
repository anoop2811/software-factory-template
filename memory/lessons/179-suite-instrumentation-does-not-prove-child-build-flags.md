# Inspect child build flags independently of the evaluator

A race-enabled acceptance suite does not establish that a CLI it compiles uses
race instrumentation. Check the actual child build arguments and environment
or read the child's Go build metadata before attributing timing to that mode.

Provenance: observed 2026-11-13 while reviewing macOS job `131973875574`.
acceptance/installation_upgrade_test.go:283 enables child race instrumentation
only for an explicit selector; the workflow did not set it. The diagnostic
boundary is recorded in docs/adr/0151-reliable-ci-qualification.md:283.
