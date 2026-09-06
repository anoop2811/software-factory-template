# Exercise the entrypoints adopters actually use

A script can return the expected error code for the wrong reason. The original
Java gate returned exit 2 under sh because its parser failed; the shell guard
regression tests also require an actionable diagnostic and no parser error.
Likewise, checking that a Maven XML file is well formed cannot establish that
its compiler and analysis plugins load or reject their intended violations.

Provenance: observed 2026-09-06 during issues #66-#68.
`sh packs/java/hooks/junit5-only-check.sh` initially printed a syntax error at
the process-substitution loop. The new selftest shell matrix checks sh,
bash --posix, and dash; `bash scripts/selftest/run.sh` reported
`selftest: 217 passed, 0 failed, 0 skipped`.
`bash scripts/selftest/maven-quality.sh`, with Maven 3.9.16 and JDK 26.0.1,
reported `maven-quality: 5 passed, 0 failed (verify, format, Error Prone, SpotBugs, PIT)`
after detector-specific failures, restoration passes, and a killed mutation.
See Decisions 43-44 and docs/adr/0045-maven-adoption.md for the requirements.
