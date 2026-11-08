# Watchdog shutdown needs more than a successful signal

Observed 2026-11-08 during Decision 159's private Linux Dash probe: TERM returned
success while the actual watchdog's timer remained alive until its natural
expiry. The waiting shell also retained saved capture descriptors. The public
command can therefore appear complete while its caller still waits for output.

A stronger numeric signal was not an adequate correction: independent review
observed a late trap request it after the timer had already been reaped. Keep
stop authority tied to an owned resource and prove that the watcher joined.
The replacement policy belongs to
[ADR 0124](../../docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md),
including cancellation, failure retention and unchanged deadlines.

The original hosted timeout did not capture its process or descriptor state.
The reproduced mechanism is evidence of a defect, not proof of that incident's
exact trigger. Preserve that distinction when recording the correction.

Provenance: observed 2026-11-08 via the extracted `candidate_watch` Dash probe
and late-trap ownership review; recorded in `docs/DECISION_LOG.md` Decision 159
and `docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:229`.
