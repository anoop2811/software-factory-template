# Managed handoffs need cooperative shutdown

A selected Go executable can own native work in another process group. Reaping
the selected group alone does not prove that nested work disappeared. Preserve
that distinction when reporting ownership and deciding which inputs to clean.

Provenance: observed 2026-10-31 UTC by signaling only the original compiled Go
caller after actual authenticated selected-runtime Git readiness. The RED caller
exited 247, its selected group was absent, and the separate Git group remained
alive. The matching GREEN caller exited 1 with both recorded groups absent.
Independent real ignored-TERM and external-SIGKILL controls first rejected false
confirmation, then passed with typed ownership uncertainty.

The canonical cooperative-budget, normal-exit and retained-input contract is
`docs/adr/0122-default-packaged-go-upgrade.md:146`. The parent observes its direct
group; normal selected-runtime completion delegates nested cleanup to that
qualified controller rather than inventing a transitive process-tree proof.

Observed again on 2026-11-03 UTC during R3 runner review: replacing an executable
shell runner with a Go adapter introduced another separately grouped harness.
A private fake CLI completed after the evaluator reported a one-second timeout;
the frozen shell control did not. Independent timeout and parent-cancellation
regressions then observed the live CLI PID at evaluator return before cleanup.
The runner boundary and explicit shutdown correction are recorded in
`docs/adr/0133-go-source-evaluation-runners.md:70`. Audit the supervising caller
when converting an entrypoint, not only the new adapter's local behavior.
The same review then observed a nested controller returning ordinary status 1
for uncertain pipe ownership while its parent scored a pass. Normal exit alone
does not carry that nested result; the per-attempt failure-receipt contract is
`docs/adr/0133-go-source-evaluation-runners.md:109`, including pre-exec source
bootstrap failures. Keep ordinary custom-runner exit semantics separate from
explicitly reported factory execution failure.

Two further PR #164 probes on 2026-11-03 UTC showed why the wrapper still matters:
80 ms of extra scheduling cost per shell poll exhausted the outer grace before
compiler cleanup, and a valid uncertainty receipt beyond a 16 MiB capture limit
was lost while a detached child still used work that the evaluator deleted.
The first probe returned an ownership error; the second scored zero, so neither
was evidence of a false passing score. The canonical remedies and limits are in
docs/adr/0133-go-source-evaluation-runners.md:166 and
docs/adr/0133-go-source-evaluation-runners.md:182. Exact process observations are
retained under `.factory/qualification/r3-runners/cold-timing-wmj0_ygv/probe.json`
and `.factory/qualification/r3-runners/receipt-overflow-3m08gj7n/result.json`.
