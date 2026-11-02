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
