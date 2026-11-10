# Child admission needs a PID-publication guard

An asynchronous child can exist before its supervisor assigns `$!`. A signal
handler that treats an empty child variable as no owned work can exit and leave
that child alive. Preserve pending cancellation across admission and publication;
then use the existing bounded cleanup. See the canonical contract in
`docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:301` and its
implementation in `install.sh:372`.

Provenance: observed 2026-11-10 UTC through the real-installer Ginkgo
verifier-publication control in `acceptance/bootstrap_timer_stop_test.go:246`.
The original supervisor exited 143 while its verifier remained live with parent
1; the guarded version passed the existing process/group-absence assertions.
Controlled extraction also reproduced the original race on Bash 3 and Bash 5.
