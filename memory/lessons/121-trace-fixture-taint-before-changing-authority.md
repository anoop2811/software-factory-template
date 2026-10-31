# Trace fixture taint before changing authority

Observed 2026-10-31 UTC through PR150 source job 124397882115: all base
behavioral cases passed before G703 reported the generic fixture writer.
Expanded diagnostic tracing connected an HTTP callback path to a same-signature
JSON helper. Actual callers used existing private roots; the helper's filename
was literal. A source trace is stronger evidence than guessing from the sink.

The bounded response and evidence limits are canonical in
`docs/adr/0121-local-upgrade-readiness-boundary.md:62` and
`docs/adr/0121-local-upgrade-readiness-boundary.md:72`. The confined writer kept
JSON bytes, permission semantics and all adversarial cases. The identical
diagnostic changed from one finding to zero; sync36 and doctor2 cases passed.
That diagnostic result does not replace native hosted qualification or establish
an exact reproduction of the pinned hosted analyzer.
