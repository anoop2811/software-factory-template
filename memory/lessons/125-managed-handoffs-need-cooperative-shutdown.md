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
