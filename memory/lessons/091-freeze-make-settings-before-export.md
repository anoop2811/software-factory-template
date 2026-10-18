# Freeze Make settings before exporting them

Exporting a recursively expanded Make variable does not turn it into inert
configuration. A value containing `$(shell ...)` can execute while Make prepares
the environment, before the receiving Go program validates it. Preserve its raw
value with an immediate `override VAR := $(value VAR)` assignment before export;
then let the typed validator reject malformed input.

Provenance: observed 2026-10-18 through real `make go-runtime-source-check`
probes with an owned temporary witness. Before the change the invalid request
returned exit 2 and created the witness; afterward both worker settings returned
exit 2 without creating it. See Makefile:34 and
[ADR 0104](../../docs/adr/0104-adaptive-qualification-workers.md) for the canonical
policy and independent regression checks.
