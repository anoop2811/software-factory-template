# ADR 0152: bound local recovery qualification by observed phases

Status: accepted internal qualification correction; final-head evidence pending.
Decision: 168.

## Evidence and boundary

Hosted macOS job `131973875574` at checkpoint `cb79827` exhausted the two
local-recovery calls before publisher admission. They already run sequentially
under one ordered owner. Reverse reached candidate materialization; forward's
precise preparation step was not captured. Neither reached a durable witness.

The diagnostic-only local run retained the original public invoker and passed
both cases in 393.718 suite seconds. Actual invoker build metadata contained no
race flag. Preview took 51.072 and 52.738 seconds; selected Begin to publisher
admission took 15.978 and 18.227 seconds. Materialization took 0.611 and 0.601
seconds, each staging operation about 1.1 seconds, and admission to durable READY
took 0.090 and 0.089 seconds. The original command-to-admission intervals are
approximately 65.47 and 73.95 seconds, reconstructed from the saved context
remainder and report time, with a small report-publication offset. Hosted
preview-adjacent intervals were about 86–90 seconds. These are observations,
not proof of host contention, a product deadlock or a measured optimization.

The fixed ninety-second test context budgets two genuine source acquisitions,
predecessor reconstruction and selected-worker preflight together with the
interruption witness. It is an evaluator watchdog, not a public CLI latency
contract. Its margin is inadequate for the observed workload variation.

## Phase policy

For only the two local-recovery criteria, allow 120 seconds for preview and
120 seconds for publisher admission. Retain the sixty-second durable-publication
allowance, beginning only after the existing actual admission receipt is
validated. One 180-second command context bounds acquisition and publication
together; never replace or reset it at admission. An earlier parent deadline
or cancellation always wins. Existing bounded process cleanup may follow that
command cancellation; 180 seconds is not a claim about the whole test duration.

This explicitly supersedes ADR 0121's ninety-second test context for these two
criteria. It does not change runtime compiler, process, gate or review limits,
other fixture deadlines, the 25-minute suite allowance or the 30-minute CI job
limit. The chosen 120/180-second margins require hosted qualification; they are
not asserted to be statistically optimal or universally sufficient.

Use a small repository-qualification wait boundary with context-scoped phases,
actual receipt observations and prompt child-exit errors. Diagnostic timing
files never confer admission or readiness. Keep the original invoker, real
compilation, authentication and freshness checks, source identities, held child
PID/group, durable journal rows, actual interruption, compiler poisoning and
forward/reverse replay. No shipping command, flag, dependency or trust shortcut
is added.

## Independent controls and qualification

An independent real-child control must demonstrate late valid admission beyond
the former scaled overall allowance, then durable readiness within its separate
publication allowance. It must also reject absent admission, timing-only files,
admitted-but-absent durable readiness, an earlier outer cancellation/deadline,
and an early nonzero child exit without waiting for an unrelated timeout.
Restore the old combined cap or remove a phase/outer bound in private negative
controls and observe failures. Retain exact criteria and update the census for
the added independent criterion.

Run both genuine recovery cases locally and on Linux/macOS, retain phase timings,
and require the complete final-head CI/review result before human merge. A local
pass or diagnostic improvement alone does not close the hosted failure.
