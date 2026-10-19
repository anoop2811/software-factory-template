# Resume deletion at the original durability boundary

A durable intent can explain an absent file after interruption. It does not prove
that the unlink reached stable storage. Before advancing the progress record,
retry must sync that entry's actual surviving parent directory, just as the normal
unlink path does. Syncing the directory containing the progress record is a
different operation. Keep file removals before their directories and remove empty
directories in postorder so the necessary parent still exists.

Provenance: observed 2026-10-19 during the independent correctness/tests review of
`internal/installation/retention_prune.go:268`. The reviewer found a resume branch
syncing the progress parent; a second reviewer confirmed the ordering issue before
it was corrected. The contract remains
`docs/adr/0106-go-backup-retention.md:89`.
The real-filesystem interruption criterion at
`internal/installation/retention_prune_test.go:114` interrupts after unlink and
before its parent sync, reopens the tree, and retries repeatedly without counting
the already absent leaf twice. The installation package race suite passed 28
criteria. This exercises interruption and retry, not physical power-loss
persistence or syscall tracing of fsync.
