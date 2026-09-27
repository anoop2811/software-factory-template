# Qualify output errors at the process boundary

An injected io.Writer failure can return a checked error while a real CLI exits
before that code runs. Go treats broken writes to stdout/stderr specially: without
SIGPIPE notification, the process can terminate on the signal instead of returning
the documented command status. Test an actual compiled process with the pipe's
read end closed before launch; no timing sleep is needed.

A correction for one command should own its scope and restore notification on
return. Do not turn SIGPIPE into cancellation or globally change unrelated
commands. Keep writer fault tests too: they cover short writes and flush failures
that the process probe does not replace.

Provenance: observed 2026-09-27 via the compiled public upgrade-preview probe
(returncode -13) and independent closed-pipe RED, then four passing compiled race
cases. docs/adr/0082-go-public-upgrade-preview.md:135 records the correction;
https://pkg.go.dev/os/signal#hdr-SIGPIPE was fetched 2026-09-27 to check the Go
runtime contract. This lesson summarizes the evidence, not a new signal policy.
