# Successful helper exit is not group completion

A local help or Git helper can exit successfully after starting a same-group
descendant that closes its output streams. A successful leader status, finished
pipe capture and terminal controller record therefore do not establish owned
group completion. A transition marker must not disappear on those facts alone.

Observed 2026-10-04 UTC via independent actual legacy-budget and compiled/legacy
manual-loop regressions: status 0, completed accounting/checkpoint evidence and
zero activity markers coexisted with a live helper descendant after a PID/ready
handshake. The failing controls are acceptance/runtime_transition_test.go:115
and acceptance/runtime_transition_test.go:163; the original full logs are
`/private/tmp/factory-runtime-transition-help-descendant-red.log` and
`/private/tmp/factory-runtime-transition-snapshot-descendant-red.log`.

Reuse owned supervision for successful probes too, rather than adding another
leader-only wait. The controlling requirements and qualification remain in
[ADR 0093](../../docs/adr/0093-runtime-transition-guard.md) and
[runtime transition evidence](../../docs/migration/RUNTIME_TRANSITIONS.md).
This lesson grants no migration, recovery or pruning authority.

Observed again 2026-10-04 UTC via the real Go group-completion pair in
internal/native/group_completion_test.go:32: an acknowledged signal, reaped
leader and drained capture still left a ready same-group child alive. The
negative control failed before correction; both corrected controls passed with
race detection (`/private/tmp/factory-transition-final-pairs-race.log`). Require
explicit group absence within the existing cleanup deadline. The controlling
refinement is docs/adr/0093-runtime-transition-guard.md:195; temporary inspection
errors are not absence evidence.

Observed again 2026-10-07 UTC through public installation packaging Build probes:
a completed Git wrapper retained stdout/stderr for about three seconds past a
400 ms deadline, or left a closed-pipe descendant alive. The independent process
regressions at internal/packaging/installation_process_test.go:159 reproduced the
failure before the correction. Reusing the existing supervisor through a bounded
tool API returned in 401-404 ms and left the observed descendants absent. The
contract is docs/adr/0097-installation-source-image-bundles.md:187; retained probe
results are `/private/tmp/factory-installation-tail-requalification-aj0radfy/qualification.json`.
Bounded output collection must not parse or publish a snapshot before confirmed
completion and ownership; byte limits alone are insufficient.
