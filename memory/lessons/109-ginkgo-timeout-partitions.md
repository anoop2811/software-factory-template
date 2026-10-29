# Measure ordered families before changing a suite deadline

A suite timeout can interrupt a healthy case. The failed macOS report recorded
two active configuration cases below one second, while eight passing ordered
cutover scenarios consumed 550.911 seconds on one worker. Inspect actual family
timings and preserve the ordered state when separating qualification work.

Partition selectors also need Ginkgo's suite-description prefix. Independent
native discovery selected zero cases without it and eight with it; synthetic
selector fixtures must use that same full text. Keep a disjoint, exhaustive
identity check rather than trusting a plausible regular expression or count.

Provenance: observed 2026-10-29 via
[the macOS suite report](https://github.com/anoop2811/software-factory-template/actions/runs/40948478308/job/123710479452)
and native `-ginkgo.dry-run -ginkgo.fail-on-empty` selection controls.
The canonical partition contract is
`docs/adr/0113-local-cutover-qualification-partition.md:27`.
