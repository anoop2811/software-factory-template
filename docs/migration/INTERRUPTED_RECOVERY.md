# Interrupted publication recovery

Status: Go source implementation; no installed recovery consumer.
[ADR 0096](../adr/0096-interrupted-publication-recovery.md) is authoritative.

A crashed process loses its live publication handle. The retained journal and
pending marker keep ordinary cooperating execution blocked. Reading those files
does not restore authority. The source component uses a fresh proposal and
explicit consent to grant a new capability over currently checked evidence.

A trusted caller supplies the known replacement bytes, an independently qualified
after-reference, selected operation and direction, and an explicit affirmation
of unbridged-process quiescence. Proposal checks are read-only. Grant recomputes
them under the existing permanent exclusive lock and preserves the pending marker.
Actual G2 state must be readable without unresolved work; this is bounded schema
compatibility, not proof of complete installation or target-runtime compatibility.

Recovery can finish an already published after-image, restore its saved original,
or abort a prepared operation that still has its original. Edited, ambiguous,
held, unsafe or incompatible evidence remains preserved. Fresh consent binds the
actual current file, journal, marker, saved input and state; it does not authenticate
historical local records. No force cleanup or age/PID-based inference is supplied.

Checked completion makes the selected image and terminal record durable before
removing the exact pending marker. Retry keeps its direction. Another crash needs
another proposal and grant. Close only releases resources, preserving incomplete
evidence. Backups, holds, history, unrelated records and orphan siblings stay inert.

This is one-file source recovery. Public rollback, full installer coverage,
activation, legacy retirement, retention and seamless v0.1.6 migration still need
their implementation and release qualification. The installed dispatcher is unchanged.
