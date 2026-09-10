# ADR-0067: bounded streaming adversarial review client

Status: implemented with local acceptance; live qualification pending.

## Problem and evidence

PR #92 reached a 480-second deadline after HTTP 200 and partial bytes; subsequent
reviews returned initial HTTP 429. PR #93 increases the deadline but neither
observes completion incrementally nor handles explicit rate limiting. A client
cannot guarantee provider capacity. We need bounded recovery and trustworthy
failure evidence, rather than repeatedly increasing token limits.

## Decision

Add `FACTORY_BRIDGE_PROTOCOL=1 factory-go review openrouter` to the existing Cobra
private protocol. It reads one prepared Chat Completions JSON object from stdin,
reads REVIEW_API_KEY from the environment, adds `stream: true`, and sends it to
the fixed HTTPS OpenRouter Chat Completions endpoint. Keep model, messages,
reasoning, provider restriction and max_tokens unchanged. Never follow redirects
or execute tools. Reject malformed/oversized requests before network activity.
The prepared request admits only model, messages, max_tokens, reasoning and
provider fields. Require a nonblank string model, nonempty string-content
messages and an integer max_tokens in 1024..32768. Reject extra request fields
(such as tools, multiple completions or plugins) rather than enabling another
execution or spending mode. Optional reasoning is exactly an effort string
using the shell's supported vocabulary. Optional provider is exactly one valid
lowercase hosting slug in order, allow_fallbacks false and require_parameters
true, matching the shell adapter. Omitting provider preserves existing gateway
default routing; this client adds no fallback policy. The transport is Go
standard library code; no additional SDK or dependencies.

The shared shell script remains the public entry point for all harnesses. An
explicit absolute executable path in REVIEW_GO_CLIENT delegates only OpenRouter
to the private command; empty/unset retains the legacy curl client. A selected
client failure must never fall back and issue another request. Anthropic/OpenAI
retain their existing path. No automatic build or download occurs in this script.

This repository's trusted-base workflow builds the existing pinned Go module
before the secret-bearing step, selects that binary, and selects one retry.
The generated adopter workflow exposes the optional client path and retries as
Actions variables but does not require a Go module/toolchain or select a binary.
Adopters must provision a trusted compatible binary to opt in; installation and
legacy retirement remain governed by the separate Go migration specification.

## Completion and resource contract

- One overall REVIEW_TIMEOUT_SECONDS deadline, default 1200, range 1..1200,
  covers every attempt, backoff and stream. Keep a 15-second connection bound.
  Cancellation closes the active body; SIGINT/SIGTERM must cancel the client.
- Bound the request to 1 MiB, response body to 8 MiB and each SSE event/line
  to 1 MiB. Bounds are byte limits, including ignored reasoning/heartbeat data.
- Parse SSE framing across arbitrary read boundaries, CRLF/LF/CR delimiters,
  comments, blank events and multiline data. Ignore one initial UTF-8 BOM,
  counting its wire bytes toward the bound. Ignore comments as heartbeats.
  Accept content-free usage objects and repeated matching terminal reasons;
  non-null usage values of other JSON types are malformed.
- Buffer findings privately in memory. Publish only nonblank text after a
  `stop` terminal reason, `[DONE]`, and clean transport completion. Reject
  malformed events, errors (even under HTTP 200), length/filter/tool completion,
  missing completion, contradictory endings, and data after completion. No
  partial stdout on a failure; reasoning is never published.
- Diagnostics are fixed classifications and validated numeric metrics only:
  HTTP status, attempts, elapsed duration, response bytes, content/reasoning
  event counts and heartbeat counts. Emit a safe progress line at most every
  30 seconds while reading and a final summary, including on failure. Never
  log API keys, prompts, raw provider errors, response text or reasoning text.

## Rate limiting and cost

REVIEW_HTTP_RETRIES is environment-only: default zero, valid 0 or 1. The
repository workflow defaults it to 1 and exposes the Actions variable; a value
of 0 disables retries. Only an explicit initial HTTP 429 or 503 is eligible.
Honor valid Retry-After delta-seconds or HTTP-date; absent header uses a short
bounded jittered delay of 2..5 seconds. Malformed, negative or greater-than-60s
Retry-After delta-seconds values refuse retry instead of retrying early. A valid
HTTP-date already in the past requires no further wait; a future HTTP-date more
than 60 seconds away refuses retry. If the wait cannot
fit the remaining deadline, stop. Close the first body before waiting. Never
retry HTTP 200 errors, partial streams, transport failures, length exhaustion,
authorization/payment failures or any other status. No hidden transport retry.

There is no automatic provider or model fallback. Current GLM 5.3 Flash, low
reasoning, DeepInfra restriction and 32768-token repository cap remain intact.
One retry can incur additional prompt charges; the allowance bounds attempts,
not dollars. Provider `max_price` is a unit-price filter, not a total spend cap.
A broader routing policy would require explicit provider/price choices and its
own acceptance contract.

## Alternatives and authoritative research

Sources fetched 2026-09-09 UTC:

- [OpenRouter streaming](https://openrouter.ai/docs/api_reference/streaming):
  supports SSE comments and multiline framing; HTTP 200 may precede a later
  error; Chat Completions repeats finish_reason in its accounting frame.
  Streaming cancellation is documented for DeepInfra; local cancellation does
  not establish a measured billing result.
- [WHATWG SSE framing](https://html.spec.whatwg.org/multipage/server-sent-events.html#interpreting-an-event-stream):
  UTF-8 decoding removes one initial BOM before interpreting fields.
- [OpenRouter errors](https://openrouter.ai/docs/api_reference/errors-and-debugging):
  Retry-After applies to rate limiting/unavailability, and error metadata can
  contain sensitive input. Empty content does not prove no prompt charge.
- [Provider routing](https://openrouter.ai/docs/guides/routing/provider-selection):
  provider restrictions and unit-price caps support controlled routing, but
  performance preferences are not completion guarantees.
- [Official Go SDK](https://github.com/OpenRouterTeam/go-sdk): offers streaming
  and retries, but is beta and warns of breaking changes in minor releases.
  For this single endpoint, a bounded standard-library adapter avoids adding
  that broader dependency; the tradeoff is owning SSE framing and its tests.

Workflow build pins were rechecked on 2026-09-09 UTC: Go 1.27.1 was released
2026-09-01 ([Go release history](https://go.dev/doc/devel/release#go1.27.1));
setup-go v7.0.0 was released 2026-07-16 ([official release](https://github.com/actions/setup-go/releases/tag/v7.0.0)),
and its tag API resolves to `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e`.
The build reuses the existing Go module and locked dependencies.

Changing SDKs alone cannot remedy rate limits. Harness CLIs add installation,
credentials and tool-execution concerns without solving the provider failures.
Unbounded retries and silent model fallback violate predictable spending.

## Acceptance and delivery

Independent Ginkgo/Gomega outside-in tests precede implementation. Exercise
the compiled private command through a local HTTPS fixture/proxy, including
request fidelity, split framing, repeated accounting terminal, terminal failure,
no partial publication, bounded bytes/deadline, cancellation, secret redaction,
redirect refusal and exact request counts for eligible/ineligible retries.
Exercise the shell adapter and workflow wiring separately; existing legacy
self-tests must remain green. Run Go quality/race/security checks and independent
correctness/security review. A fake service proves client behavior, not live
OpenRouter reliability. Trusted-base production qualification follows merge.


## Local delivery evidence (2026-09-09 UTC)

The initial compiled private-command acceptance failed with unsupported request:
2 cases, 0 passed and 2 failed. Independent adversarial probes subsequently
caught raw HTTP trailer leakage, malformed usage-frame admission and initial
SSE BOM content loss; all five regression cases failed before their corrections.

RAN `go test ./acceptance -ginkgo.focus="Bounded streaming adversarial review"
-ginkgo.no-color -ginkgo.succinct -count=1`: the 61-case focused group returned
`ok github.com/anoop2811/software-factory-template/acceptance 29.705s`. Two additional local timing cases ran separately with
`go test -race ./acceptance -ginkgo.focus="Bounded streaming adversarial review timing"
-ginkgo.no-color -ginkgo.succinct -count=1`: `ok github.com/anoop2811/software-factory-template/acceptance 47.699s`. They
observed the combined connection cutoff and safe progress reporting, for 63
review cases in the final source.

RAN `make go-runtime-source-check`: exit 0; full race acceptance returned
`ok github.com/anoop2811/software-factory-template/acceptance 303.339s`, lint `0 issues.`, gosec `Issues : 0`, and
govulncheck `No vulnerabilities found.` The two timing cases were added after
that full acceptance invocation and passed their separate race run above.
RAN `bash scripts/selftest/adversarial-review.sh`: `53 passed, 0 failed`.
RAN `bash scripts/hooks/diff-aware-check.sh`: `217 passed, 0 failed, 0 skipped`;
all dispatched checks passed. Independent correctness and security review
reported no remaining findings after corrections.

These runs used local fixtures; they do not establish a live OpenRouter
completion, provider billing result, released adopter installation or recovery
qualification. The privileged workflow will select this implementation only
from a trusted base commit after merge.
