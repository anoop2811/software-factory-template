# Historical success can hide skipped rendering

A retained initializer's successful exit does not establish that its generated
harness configuration was rendered. The historical initializer copies the three
sync scripts, executes them as an `&&` chain, then converts failure into a
warning. Pairing that initializer with a new native adapter and an old dispatcher
can therefore skip generation without failing the surrounding installer test.

When replacing a direct entrypoint, trace both explicit test invocations and
transitive historical consumers. Preserve a matching immutable input set for
each historical oracle; use separate current-source assertions to prove native
delegation. The current boundary is docs/adr/0128-go-source-harness-sync-entrypoints.md:45.

Provenance: inspected immutable revision `1754de23de95a7e3c734bedd86894295e823c988`
with `git show` on 2026-11-01 UTC. Its
[initializer copies](https://github.com/anoop2811/software-factory-template/blob/1754de23de95a7e3c734bedd86894295e823c988/scripts/factory-init.sh#L449)
and
[warning-only synchronization failure](https://github.com/anoop2811/software-factory-template/blob/1754de23de95a7e3c734bedd86894295e823c988/scripts/factory-init.sh#L835)
establish this consumer relationship. This is source-audit evidence, not a claim
of a released adopter incident.
