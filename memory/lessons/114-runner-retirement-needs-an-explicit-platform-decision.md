# Runner retirement needs an explicit platform decision

A hosted job can be cancelled before any step because its image is in a
provider brownout. Inspect check-run annotations before treating that outcome
as a product failure or retrying the same unavailable image indefinitely.

Moving the CI host does not establish compatibility at an older product floor.
The user explicitly revised this project's supported macOS minimum; the
current policy is [Q2](../../specs/001-go-runtime-conversion.md#10-open-questions)
and Decision 128. Preserve historical measurements under their original
criteria, then qualify the next binary on the newly approved minimum.

Provenance: observed 2026-10-30 UTC in Actions job `124215632371`, whose
annotation reported the scheduled macOS 14 brownout before any build step.
[The official notice](https://github.com/actions/runner-images/issues/13518),
fetched that day, schedules the interruption and 2026-11-02 retirement.
`specs/001-go-runtime-conversion.md:481` records the approved replacement floor.
