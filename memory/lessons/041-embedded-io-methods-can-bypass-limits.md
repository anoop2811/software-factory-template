# Embedded I/O methods can bypass limits

Overriding Write on a struct that embeds bytes.Buffer does not constrain every
write path: the embedded WriteString method is promoted too. A buffered encoder
can detect io.StringWriter and use that method without invoking the bounded
Write implementation. Prefer a named buffer field and expose only the intended
bounded interface, or deliberately constrain every fast-path method.

Provenance: observed 2026-09-24 via the independently authored Ginkgo regression
at internal/loop/checkpoint_test.go:266. A 4300-digit integer sent through the real
canonical encoder exceeded the prefilled buffer's remaining allowance before the
correction. The final publication-size check still refused the checkpoint; the
defect was the earlier encoding bound. RED/GREEN commands and outcomes are recorded
in docs/migration/LOOP_CHECKPOINTS.md. The canonical encoder is shared; the limit
belongs to its caller under docs/adr/0074-go-loop-checkpoint-storage.md.
