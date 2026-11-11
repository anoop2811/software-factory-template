# Correlate local deadline failures with host suspension

A local timeout observed across host sleep is not a normal execution-time
measurement. Preserve both the test timestamps and the operating system's
sleep/wake events before attributing the failure to code or increasing a limit.
Keep that observation separate from failures on other runs or hosted machines.

Provenance: observed 2026-11-11 via `pmset -g log` and the source-entrypoint
Ginkgo report. The cold-source readiness case began at 12:19:10.900 -0700;
Idle Sleep began at 12:19:11 for 322 seconds, wake occurred at 12:24:33, and the
assertion ended at 12:24:35.984. Its reported test duration was 5.027 seconds.
This was not recorded as a passing full suite. The investigation and qualification
contract remain in `docs/adr/0149-compiler-provenance-and-security-patches.md:201`.
