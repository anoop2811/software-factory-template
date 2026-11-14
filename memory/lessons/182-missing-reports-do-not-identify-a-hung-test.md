# Missing parallel reports do not identify a hung test

A Ginkgo suite deadline can interrupt ordinary progressing work, then a Go test
watchdog with the same limit can terminate JSON report generation. Empty reports
and the last printed test name do not establish which test caused the overrun.

Use an unchanged seed, selection, workers and limits with raw output and progress
reports to separate suite exhaustion from an individual stalled operation.
Compare retained partition timings before redistributing complete families;
preserve the full criterion union and measure the resulting hosted jobs.

Provenance: observed 2026-11-14 through the same-seed macOS reproduction recorded
in `docs/adr/0155-rebalance-source-qualification.md:70`. The deadline interrupted
an eight-second scheduler test and an 87-second installer setup; the subsequent
Go watchdog stack showed JSON report generation. This local result does not
establish identical active tests in the hosted failure.
