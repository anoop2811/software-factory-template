# JSON object order can carry application policy

Do not assume a map-based JSON round trip preserves behavior. OpenCode V1 builds
permission rules in object-entry order and uses the last matching rule. Sorting
those keys changes policy even when a decoded-map comparison reports equality.

When synchronization owns only model fields, retain raw unrelated subtrees and
change those fields deliberately. Test ordered permissions and compare the first
generated adapter state with the next synchronization; a second normalization
before taking the baseline can hide the original defect.

Provenance: observed 2026-11-10 via the independent
`Native orchestration sync ordered policy` acceptance RED against `6b8f08c`:
one failure in 3.259s showed reordered root, per-agent and non-Bash rule maps.
The contract and authoritative tagged OpenCode sources are recorded in
`docs/adr/0146-initialized-installed-ci.md:89` and
`docs/adr/0146-initialized-installed-ci.md:133`. This lesson points to that decision;
it does not define a separate permission policy.
