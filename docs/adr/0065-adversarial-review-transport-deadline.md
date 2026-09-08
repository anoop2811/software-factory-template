# ADR-0065: Bounded review transport deadlines and diagnostics

Date: 2026-09-07
Status: implementation correction

## Observed failure

PR #87 review run `34170004289` fetched a 54,709-byte diff, below its 200,000-byte
limit. The request began at 23:25:51 UTC and exited at 23:28:52 UTC. Its comment
reported curl exit 28 at 180001 milliseconds with 660 response bytes. The
workflow's job allows ten minutes, but both HTTP branches hard-code
`--max-time 180`. The script discards curl failure output and then reports
that the model returned no review. Provider queueing, generation latency and
the contents of those partial bytes cannot be established from existing logs.

Sources read 2026-09-07:
- https://github.com/anoop2811/software-factory-template/actions/runs/34170004289
- https://curl.se/docs/manpage.html#--max-time (total transfer deadline)
- https://openrouter.ai/docs/api/reference/errors-and-debugging

## Decision

Expose `REVIEW_TIMEOUT_SECONDS` as an environment-only transport setting,
including a same-named GitHub Actions repository variable in the active workflow
and generated review-lane template. Default to 480 seconds; accept decimal
integers 1 through 480, with leading zero normalization before arithmetic.
Empty/unset uses the default. Reject invalid values before any HTTP request.
Use a separate 15-second connect timeout. The 480-second ceiling leaves room
inside the ten-minute job for setup and posting an honest result.

Keep model, hosting route, reasoning effort, diff cap and completion cap unchanged.
No retry, paid fallback or streaming migration is introduced. A longer wait does
not increase the requested token cap; it permits the original request to finish.
This mitigates the confirmed client deadline, not a guarantee of provider health.

Classify failed requests before attempting review extraction: curl timeout,
other transport failure and HTTP non-success must be distinguishable from empty
or invalid provider response. Never accept a partial review after curl failure,
including a partial body that happens to contain valid JSON. Capture safe numeric
transport evidence (curl status, HTTP status, total time, time to first byte and
received byte count) on failures without echoing credentials, request headers,
request body, raw partial response or arbitrary curl stderr. Preserve existing
provider error handling where a completed HTTP 2xx response supplies a valid error.
For HTTP non-2xx responses, emit only the numeric transport diagnostics above;
do not extract or display the response body, even when it contains valid JSON.
Temporary response storage must be private and removed on exit. Existing success
stdout remains review markdown with no diagnostics added to success stderr.

## Acceptance and rollout

Use the real review script with fake HTTP fixtures before implementation: delayed
completion beyond the old limit, explicit/empty/default timeout, invalid and
oversized values, all provider branches, connect limit, one-call-only behavior,
partial timeout and completed JSON on failed transfer, transport error, HTTP error,
no input/secret leakage, cleanup and successful review extraction.
Keep the active and generated workflow variable wiring aligned and test it.
Record independent RED/GREEN evidence. The `pull_request_target` job intentionally
executes trusted base code; a PR proposing this correction cannot claim a live
provider test of the changed script until the correction reaches the base.

## Development evidence

Before implementation, the independent fake-HTTP boundary tests ran with:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer bash scripts/selftest/adversarial-review.sh
```

```text
adversarial-review: 36 passed, 9 failed
```

The new 240-second fixture failed under the old 180-second option, without
sleeping or calling a paid provider. Adding the private-storage assertion before
implementation produced `36 passed, 10 failed`. After the correction and a
supplemental malformed-metrics case, the frozen suite ran with:

```sh
rtk proxy env FACTORY_AGENT_ROLE=reviewer bash scripts/selftest/adversarial-review.sh
```

```text
adversarial-review: 47 passed, 0 failed
```

An independent security review also reported 47 passed, 0 failed and no concrete
findings. A local HTTP server experiment with real curl (test wrapper redirects
only the fixed endpoint to loopback; no live provider/key) returned:

```text
deadline=1 status=1 stdout=''
adversarial-review: request timed out.
adversarial-review: curl_status=28 http_status=200 total_seconds=1.003176 first_byte_seconds=0.001107 received_bytes=1
deadline=480 status=0 stdout='No findings.\n' stderr=''
```

The server sent one whitespace byte immediately and completed its JSON after
two seconds. Both response files were absent after exit. This is evidence about
curl timeout/classification/cleanup, not DeepInfra generation time.

Both commands exited 0 with no output:

```sh
rtk proxy shellcheck -x -P scripts scripts/adversarial-review.sh
rtk proxy shellcheck -S warning scripts/selftest/adversarial-review.sh
```

The test script retains pre-existing informational SC2016 notices for intentional
literal shell expressions; production ShellCheck has no such exclusion. The
active workflow exactly matched the template after normal secret-name expansion
and the managed header were applied.

Repository verification:

```sh
rtk proxy env FACTORY_AGENT_ROLE=reviewer ./scripts/hooks/diff-aware-check.sh
```

```text
selftest: 217 passed, 0 failed, 0 skipped
diff-aware-check: all 1 dispatched check(s) passed
```
