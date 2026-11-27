# Measure canonical payload bytes at the CLI output

When a CLI publishes a canonical byte count, extract the raw payload from stdout
and compare it with the published fixture. Decoding both sides before comparison
discards key order and escape spelling. Include characters such as `<`, `&`
and U+2028 so an extra JSON serialization can change the length.

Observed 2026-11-27 via a compiled `factory context preview --json` in a fresh Git
worktree. The earlier binary emitted a 220-byte payload and reported 202. After
canonicalizing the response, both measurements were 202. The regression and
the public example are in [the acceptance tests](../../acceptance/context_preview_test.go)
and [Spec 003](../../specs/003-task-context-and-skills.md#7-data-model).
