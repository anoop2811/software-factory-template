# Transport timeouts are not token exhaustion

Provenance: observed 2026-09-07 via GitHub Actions run `34170004289` and its
PR #87 comment: curl exited 28 at 180001 ms after receiving 660 bytes.
`scripts/adversarial-review.sh` at `49636db85bfb58eeafa761fb99000a12420bcb43`
hard-coded `--max-time 180` and discarded failed-transfer response data.
https://github.com/anoop2811/software-factory-template/actions/runs/34170004289

A request reaching a total transfer deadline does not establish token exhaustion,
a missing key, or a specific provider-side fault. Partial bytes do not establish
a completed model review. Preserve safe status/timing evidence and distinguish
transport failure from a completed response's finish reason before changing a
model's output allowance. The correction and its limits are in
`docs/adr/0065-adversarial-review-transport-deadline.md`; this lesson does not
claim a longer timeout makes the provider reliable.
