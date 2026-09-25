# Probe status and CLI cancellation are separate contracts

A subprocess helper that returns an exit status can report -1 when the child
dies by signal. Testing only status > 1 accepted that result as a usable grep
observation. Explicitly admit the documented success/nonmatch statuses instead.

Context-aware process-group cleanup also needs a cancellation source at the
compiled command boundary. A background context cannot observe SIGINT/SIGTERM;
the parent can disappear while its isolated probe keeps running. Test an actual
started child, signal the compiled CLI, and observe child exit independently.

Provenance: observed 2026-09-23 (America/Los_Angeles) during the loop conversion through
the independently authored signaled-grep and process-signal acceptance cases in
acceptance/loop_test.go:429 and acceptance/loop_test.go:445. Both cases were seen
failing before correction; commands and outcomes are retained in
docs/migration/LOOP_FINGERPRINTS.md. The governing contract remains
docs/adr/0073-go-loop-fingerprint-foundation.md:96 and its signal clarification.
