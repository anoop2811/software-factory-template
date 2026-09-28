# Review payloads are not command arguments

A diff can fit the configured review budget while exceeding the operating
system's command-argument limit. Moving HTTP streaming into Go does not remove
an earlier shell/jq argument boundary. Inspect the complete request path,
including JSON construction and alternative transports.

Provenance: observed 2026-09-28 in PR #112's hosted review:
https://github.com/anoop2811/software-factory-template/pull/112#issuecomment-5874148689
reported jq "Argument list too long" before any provider request. The independent
regression in acceptance/adversarial_review_payload_test.go:18 reproduced the
large-argument boundary locally before production changes. Decision 84's
follow-up in docs/DECISION_LOG.md is the canonical transport contract.

The same qualification exposed a second boundary: the test originally resolved
Homebrew Bash 5 while the production shebang selects /bin/bash (Bash 3 on macOS).
Observed 2026-09-28 via GitHub run 36482756956 and a local /bin/bash reproduction:
the old whitespace-removal substitution exceeded the 15-second test deadline;
a presence check avoids rewriting the diff. Test the production interpreter,
not whichever compatible-looking executable appears first on the author's PATH.
