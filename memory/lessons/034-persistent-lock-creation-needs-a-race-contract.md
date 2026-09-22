# Persistent lock creation needs a race contract

Do not assume concurrent O_CREATE opens of an absent persistent lock are
portable merely because opening an existing file succeeds. A local Darwin
probe returned ENOENT on the first simultaneous attempt. Exclusive creation,
with a single non-creating open only after ErrExist, avoids that observed race
without unlinking the shared inode or retrying a budget admission.

Provenance: observed 2026-09-22 UTC via `rtk proxy go run
/private/tmp/probe-lock-open.go` (two simultaneous os.Root.OpenFile calls) and
`rtk proxy go run /private/tmp/probe-lock-exclusive.go` (10,000 iterations
completed). `docs/adr/0070-go-budget-execution-controller.md:161` records the
accepted acquisition contract and qualification limits. This is not a claim
about the precise kernel/toolchain cause or every supported filesystem.
