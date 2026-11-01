# Separate compiler custody from source-image volume

A compiler-readiness deadline can accidentally include expensive work that runs
before the compiler starts. Keep process-custody scenarios focused on a small
valid source, while separate complete-source journeys cover installation and
upgrade behavior. A suppressed signal must still fail the cancellation assertion;
faster setup alone does not prove a discriminating test.

Canon: docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:297 records the
observed failure and the bounded fixture contract. Retain real packaging and
reference validation rather than substituting fixture results.

Provenance: [macOS local-init job 124965883418](https://github.com/anoop2811/software-factory-template/actions/runs/41345887407/job/124965883418)
failed before sending SIGINT on 2026-11-01 UTC; its exact child state was not
captured. Observed that day through the private suppressed-signal overlay:
compiler READY preceded the expected six-second cancellation failure, and
post-teardown compiler PID/group probes returned ESRCH. The unmodified four
signal cases then passed with READY in approximately 5.2–5.4 seconds. Session
receipts: `local-cancellation-negative-receipt.json` and
`local-cancellation-four-green.json` under
`.factory/qualification/source-entrypoints-20261101/`.

Observation also has control-flow costs: adding shell commands can introduce
trap-dispatch opportunities and conceal an implicit wait. Keep source-launcher
observation at entered TERM/EXIT handlers and inspect process state externally;
the governing boundary is docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:328.
Provenance: source review on 2026-11-01 of
[Apple Bash 3.2 execution](https://raw.githubusercontent.com/apple-oss-distributions/bash/main/bash-3.2/execute_cmd.c),
especially the function-wait and pending-trap dispatch paths.
