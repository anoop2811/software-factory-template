# Audit fixture consumers after initializer changes

Replacing a copied installation fixture with a real initializer can leave eager
setup for the old fixture in its shared owner. Audit both the new consumer and
the other consumers before attributing a suite timeout to the product.

Observed 2026-11-12 via GitHub Actions run `43259987561`, macOS handoff job
`131328354262`: the suite deadline interrupted Go recovery inside the Bash
initializer, although Go recovery no longer consumed either Bash template.
The legacy/init group likewise did not consume its prepared Go predecessor.
[ADR 0148](../../docs/adr/0148-transactional-installed-ci-migration.md) records
the boundary-specific setup and partition decision. Preserve actual initializer
behavior and assertions when removing unused preparation.

The six other cancelled macOS jobs in that run had no executed steps. Their
check-run annotations reported that hosted runners were not acquired after
multiple attempts, with ARM64 capacity constraints. Inspect annotations and
executed steps before treating every cancelled check as a code failure.
