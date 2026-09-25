# Qualify cancellation by input descriptor type

A cancellation-aware parser is not enough when its reader can block inside a
nonpollable device read. Pipe/socket cancellation evidence does not cover terminal
or block devices. Refuse unsupported descriptors before reading unless their owned
lifecycle has separate cancellation evidence, and preserve the caller's descriptor.

Provenance: observed 2026-09-25 UTC by running the compiled PR #101 candidate
at cfe74eafc820bcabad4334dbf0680c0467e25938 with PTY slave stdin. After partial
JSON and SIGTERM the process remained blocked; the reviewer reaped it with SIGKILL.
Canonical boundary: docs/adr/0074-go-loop-checkpoint-storage.md, section
"Nonpollable device input follow-up". Qualification is recorded in
docs/migration/LOOP_CHECKPOINTS.md.
