# Bash monitor flags do not prove an asynchronous child's job-control state

Apple Bash disables internal job control before running an asynchronous subshell
body, while exported `SHELLOPTS` can still contain `monitor`. Adding `set +m`
solely from that flag changes the compiler's inherited environment. Inspect
actual PID/PGID and preserve observed environment parity instead.

Provenance: Apple Bash source fetched 2026-10-29,
[execute_cmd.c](https://raw.githubusercontent.com/apple-oss-distributions/bash/main/bash-3.2/execute_cmd.c)
lines 1199–1203 and
[jobs.c](https://raw.githubusercontent.com/apple-oss-distributions/bash/main/bash-3.2/jobs.c)
lines 3765–3773; observed 2026-10-29 via explicit `/bin/bash` child-state and
exported-environment comparison. The canonical custody contract is
`docs/adr/0111-source-launcher-and-adopter-boundaries.md:32`.
