# Accounting parity crosses language numeric types

Provenance: observed 2026-09-07 by executing `normalize` from immutable
`49636db85bfb58eeafa761fb99000a12420bcb43:scripts/lib/budget_adapters.py`
with two OpenCode steps of `10**308` input tokens, and with equal-identity
costs `1` then `True`. Output showed exact `2*10**308` total and completeness,
and a deduplicated cost `1.0` with completeness respectively. The governing
candidate contract is `docs/adr/0064-go-native-usage-accounting.md`.

A port that decodes all JSON numbers into float64 can silently lose token
precision. A port that revalidates each duplicate before checking its signature
can change the baseline's accounting outcome because Python numeric equality
crosses boolean/integer/float types. Per-field admission, duplicate equality,
and aggregate arithmetic are separate behaviors that need independent fixtures.
The lesson does not prescribe new billing semantics; consult the ADR for the
explicit compatibility domain and the immutable baseline for historical behavior.
