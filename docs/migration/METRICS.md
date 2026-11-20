# Native Go local metrics

Decision 87 and [ADR-0088](../adr/0088-go-native-metrics.md) define source-native
`metrics`. The Cobra command computes local enforcement, Git history,
verification claims and eval summaries without the legacy metrics script or
Python. It retains text, versioned JSON and self-contained HTML reports.

The caller's Git root selects project data, with cwd as the ordinary fallback.
The invocation directory supplies the HTML template. JSON and text modes do
not write project files or invoke models. HTML publishes a complete page
atomically, rejects unsafe destinations and treats browser launching as a
best-effort presentation action. Browser arguments never pass through a shell.

Input, directory, aggregate eval, task-row and generated-output bounds prevent
large repositories or repeated metadata from consuming unbounded resources.
The native implementation explicitly stabilizes tie sorting, uses checked
decimal day values, rejects NUL event data and counts newline-bearing hook
names once. These intentional corrections are detailed in the ADR.

Shared pipe/socket output now uses the same cancellable borrowed-descriptor
mechanism as input. A blocked stdout pipe can be interrupted without closing
the caller's original descriptor or leaving a writer goroutine behind.

## Qualification

Independent native-route tests first reported `0 Passed | 3 Failed`: the
executable still required the missing legacy script. Ordinary output is compared
with frozen commit `fb68fb43602abcc0838e5089a1da9604c022530c`.
Further failing regressions exposed cross-line exit matching, directory-symlink
inspection, byte-width text alignment, duplicate template markers, rendering
amplification, excessive task rows, blocked stdout cancellation and embedded
HTML tokenizer sequences. The corrected publication-safety selection reported
`ok 11.538s`.

The descriptor ownership tests retain exact flag comparisons after an ordinary
initial write: macOS sets a kernel bookkeeping bit on the first write, including
writes unrelated to this adapter. `go test -race ./internal/output ./internal/input
./internal/streamfile` passed; output reported `ok 1.749s` and input was cached.
The final `FACTORY_CLI_TEST_RACE=1 go test -race ./acceptance -run TestAcceptance
-ginkgo.focus="Native Go metrics|developer-built Cobra" -count=1` selection
passed all 93 cases in `69.514s`. Configured lint reported `0 issues`, vet passed,
gosec reported `Issues : 0` and govulncheck reported `No vulnerabilities found.`
The shell selftest reported `217 passed, 0 failed, 0 skipped`.
The broader internal race run exposed an existing loop-fixture cancellation
ordering race; after registering cancellation with Ginkgo cleanup,
`go test -race ./internal/...` passed (loop package `ok 10.924s`).
Exact-head platform CI remains a separate merge gate.

## Installed use in v0.2.0

The packaged Go CLI owns `./factory metrics`.
A v0.1.6 installation keeps using its script until you apply the supported
[upgrade](../UPGRADING.md). See the [command reference](../CLI.md) for usage.
