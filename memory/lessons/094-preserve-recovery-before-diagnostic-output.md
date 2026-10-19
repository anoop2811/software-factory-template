# Preserve recovery before diagnostic output

An error-reporting write can itself fail. Decide which recovery inputs must survive
before attempting that write; otherwise an early return may let deferred cleanup
delete the inputs needed by the pending operation. Keep output failure classified
as operational failure without coupling recovery custody to a writable diagnostic
stream. The contract remains `docs/adr/0106-go-backup-retention.md:79`.

Provenance: observed 2026-10-19 while independently verifying PR #132's second
advisory review. An external compiler overlay ran the real `execute` error tail,
`reportRecovery` and acquired-input finalizers against real private cache directories
and a writer returning `syscall.EIO`; acquisition/planning/apply were controlled
collaborators, not an authenticated transaction. Before correction, both legacy
and Go-predecessor cases removed candidate recovery inputs despite pending evidence.
After the ordering correction in `internal/releasecmd/execute.go:106`, the same
race probe observed `2 Passed | 0 Failed`; both retained the necessary caches and
returned status 1. Command:
`go test -race -overlay=/private/tmp/factory-pr132-diagnostic-yjcpm5rt/overlay.json ./internal/releasecmd -count=1 -v -ginkgo.no-color`.
The external probe is supplementary component evidence, not shipped fault injection
or proof of a complete interrupted authenticated upgrade.
