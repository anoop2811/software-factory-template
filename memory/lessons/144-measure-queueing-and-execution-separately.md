# Measure queueing and execution separately

Duplicated work is not necessarily the largest elapsed-time cost. In successful
CI run 41370385630, the macOS base job ran for 1,225 seconds but started 810 seconds
after workflow creation. Its per-worker BeforeSuite setup took about four seconds;
the release-handoff setup took about two. Optimizing those small setup costs alone
cannot explain or remove the long workflow delay.

Provenance: observed 2026-11-02 by reading GitHub job timestamps and Ginkgo JSON
artifacts for https://github.com/anoop2811/software-factory-template/actions/runs/41370385630.
The separate successful run 41385426253 was already faster before the performance
change. Retain both baselines and report setup/cache differences when measuring.
The scheduling contract remains in docs/adr/0131-bounded-ci-feedback.md:33;
criterion-preservation requirements remain in docs/adr/0131-bounded-ci-feedback.md:89.

Hosted follow-up: run 41392210314 took 32m23s and failed, despite the local
installation comparison improving from 22m37s to 10m15s. The macOS base suite
grew from 1,014 to 1,383 seconds on the same reported three CPUs / 7 GiB; its
followup then hit the job limit. Two consumer fixture preparations exceeded
their archive budget, and a metrics cancellation proof observed an empty PID
receipt. Measure the critical job and inspect failure phases before treating a
local speedup or fewer jobs as a pipeline improvement.

Provenance: observed 2026-11-02 via run 41392210314 job logs and JSON artifacts,
compared with run 41385426253's base artifact. Repair contract:
docs/adr/0131-bounded-ci-feedback.md:118.

Separate leader exit from captured-output EOF when measuring process timeouts.
`Cmd.Wait` with buffered captures can wait for a descendant's inherited pipe
after the leader exits. Independent controls reproduced this with the old test
helper, then passed with explicit stream observations and cooperative cancellation
(observed 2026-11-02; acceptance/handoff_process_fixture_test.go:77 and
acceptance/handoff_process_fixture_test.go:220). This is evidence about the test
helper; it does not establish the cause of an earlier untimestamped runtime hang.
