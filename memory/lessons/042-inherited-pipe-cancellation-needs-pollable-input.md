# Inherited pipe cancellation needs pollable input

Closing an inherited os.Stdin from a context callback did not interrupt its
blocked read on the qualified macOS environment. The compiled command stayed
alive after SIGINT/SIGTERM until the fixture supplied EOF. Cancellation at the
context layer is insufficient when the descriptor is not registered for polling.

Provenance: observed 2026-09-24 via the real CLI regression at
acceptance/loop_checkpoint_test.go:438. A completed multi-megabyte pipe write
established that the child had started reading before the signal; no startup sleep
was used. The contract and correction live in
docs/adr/0074-go-loop-checkpoint-storage.md:134; evidence is in
docs/migration/LOOP_CHECKPOINTS.md. The official os.NewFile documentation fetched
2026-09-24 at https://pkg.go.dev/os#NewFile explains Unix pollability.

Duplicating the descriptor does not isolate file-status flags. Tests must also
observe original flags restored and the original descriptor usable on success,
cancellation and setup failure; internal/input/input_test.go:22 records that
separate obligation.
