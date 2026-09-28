# Ignore rules do not exclude active gate inputs

Observed 2026-09-27 via five independent real-script regression tests: ignored
recovery scripts triggered citation lint, archived docs satisfied current
citations, archived Java/TypeScript tests triggered dialect checks, and a
force-tracked Go recovery test remained a gate input. Ignore rules alone did not
separate inert reference material from current source.

Enforce exclusions where each consumer discovers its inputs, including both
citation sources and citation targets. Pair exclusion tests with active-file
controls so an overly broad filter cannot make a gate pass vacuously. Preserve
the distinction between excluded recovery content and unknown/unsafe storage;
exclusion never grants permission to create, restore or delete a recovery set.

Provenance: `FACTORY_AGENT_ROLE=spec-writer go test ./acceptance
-ginkgo.focus="Recovery discovery exclusion core" -ginkgo.no-color -count=1 -v`
returned `0 Passed | 5 Failed`, package `1.199s`, before production changes.
[ADR-0084](../../docs/adr/0084-exclude-inert-recovery-from-discovery.md) owns the
scope; [qualification](../../docs/migration/RECOVERY_DISCOVERY.md) records the
boundary and limits. Do not infer control over arbitrary external scanners.
