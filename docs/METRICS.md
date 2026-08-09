# Metrics

What the factory is doing to your repository. **Local only** — computed on your
machine, from your repository, for you. Nothing is transmitted, and nothing here
changes the standing promise: the factory you install never phones home.

```sh
factory metrics                # terminal report
factory metrics --json         # machine-readable, for your own collector
factory metrics --html         # writes .factory/metrics.html — no server
factory metrics --days 90      # window (default 30)
```

Most of it is derived from **git history**, so it is meaningful the day you
install the factory rather than after months of collecting events.

## Two rules

**Every metric names the decision it informs.** A number nobody acts on is noise
wearing a lab coat, and a dashboard of them is worse than none.

**The uncomfortable numbers are first-class.** Inert gates, repeat blocks, stale
baselines and reverts are shown as prominently as the wins. A report that only
shows wins is marketing, not measurement — the same reason `factory report`
refuses to print a "tokens saved" headline.

## What it measures

| Section | Question it answers |
|---|---|
| **Enforcement** | Is the factory real, or installed theatre? Gates installed, how many are armed vs inert, what they blocked, and whether one gate keeps firing. |
| **Loop health** | Is work converging or circling? Commits, merges, reverts, and how many files were changed more than once. |
| **Verification discipline** | Are claims cited, or asserted? Commits claiming "verified"/"fixed"/"works" that carry command-and-output evidence. |
| **Agents** | Getting better, or worse? Eval pass rate per task against baseline, and how many baselines have gone stale. |

Verification is counted **per commit**, not per line. A commit whose body claims
"fixed" on three separate bullets is one commit that made a claim; counting the
lines would inflate both numbers against a report that says "commits".

Repeat blocks deserve a note: one gate firing three or more times within an hour
is reported as **friction**, not as a win. It means either the gate is catching a
real habit or the gate is wrong, and you cannot tell which from the number alone
— but you should look.

### A zero block count means nothing on its own

"Gates installed: 14" beside "blocks caught: 0" reads as fourteen quiet gates. It
is only true if all fourteen can *report* a block, and a gate that exits non-zero
without calling `factory_log_event` cannot. An adopter repo shipped exactly that
shape: fourteen gates, eight of them structurally unable to record a block,
including the dialect gate most likely to fire.

So the report counts them. `gates_reporting` is how many gates are wired to the
event log, and `gates_mute` is how many can block without recording it. When
`gates_mute` is above zero the report says so next to the block count, not in a
footnote, because that number is what tells you whether the zero is calm or
silence.

Two things keep this from rotting:

- `scripts/hooks/gate-instrumentation-check.sh` fails CI when a gate with a
  non-zero exit path does not call `factory_log_event` — or calls it without
  sourcing `scripts/lib/events.sh`, which is the subtler mute, since the call is
  present but no event is ever written. It covers `packs/*/hooks/` too, because
  pack gates install into `scripts/hooks/` and block like any other.
- A gate whose non-zero exit genuinely stops no work — a session-end nudge, say
  — opts out by carrying `# factory: no-block-event` in a comment, next to the
  reason. Both the check and the report read that marker, so the exemption lives
  with the gate instead of in a central list that drifts. Counting nudges as
  blocks would inflate the enforcement numbers, which is the flattery this report
  exists to avoid.

If you write your own gate, this applies to it: source the lib, call
`factory_log_event <gate> <reason>` on the blocking path, and CI will tell you if
you forget.

## What it deliberately does not measure

- **Token spend per role.** Your harness owns that; the factory does not meter
  tokens and will not scrape a number it cannot stand behind.
- **Code quality.** Not honestly measurable without judgment, so it is not
  claimed.

## Sending it somewhere else

The factory ships **no exporters**. Everyone's collector is different and most
adopters have none, so integrations would be dependencies most people carry for
nothing.

Instead `--json` emits a versioned, documented document (`factory.metrics/v1`).
Anyone with a collector already knows what to do with that:

```sh
# Prometheus textfile collector
factory metrics --json | jq -r '
  "factory_gates_armed \(.enforcement.gates_armed)",
  "factory_blocks \(.enforcement.blocks_in_window)",
  "factory_reverts \(.loop.reverts)"' > /var/lib/node_exporter/factory.prom

# anything else
factory metrics --json | your-shipper
```

## Storage

Gate blocks are appended to `.factory/events.log` — one short line each, and
blocks are rare. The log is capped (`FACTORY_EVENT_MAX_LINES`, default 5000) and
trimmed to the most recent half when it grows past that. No database, no rotation
scheme. Because older raw events are dropped, the report says blocks **retained**
rather than claiming an all-time total it can no longer stand behind.

`.factory/` is gitignored. Events record gate names, timestamps and counts —
never file contents, diffs, or commit messages.

## The HTML page

`--html` opens the page in your browser (`--no-open` just writes it) — a self-contained `.factory/metrics.html`: no server, no daemon,
no port, no external requests. Open the file.

It is generated from `templates/metrics.html`, an ordinary web page whose data is
injected at a marker. Edit it like any other page — there is no build step, and
it ships as a framework file so `factory upgrade` refreshes it.
