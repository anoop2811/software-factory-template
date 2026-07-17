# AGENTS.md — project instructions for opencode, Claude Code, Codex, and other coding agents

## Project

This project uses the software factory scaffold (`opencode.json`, `.opencode/`, `.claude/`, `.codex/`, `scripts/`, `docs/`). Project-specific enforcement values (protected paths, test patterns, docs root, citation prefix) live in `factory.yaml` — hooks read it at runtime; never hardcode these in a hook.

## Long-term rules

### Always verify the latest version of any tool, library, or framework against its authoritative source — never rely on training data alone

When the task touches versions, release notes, current APIs, or "the latest" anything, **search the web first** (via `webfetch` on the official docs/release page). Training-data priors go stale. The rule:

- **Tool version:** fetch the official release page (e.g. `https://go.dev/doc/devel/release`, `https://github.com/<org>/<repo>/releases`) and cite the version + date.
- **API surface:** fetch the actual doc, not a recollection of it. If a citation is load-bearing, the citation-lint will catch a miss at CI time — but the goal is to never generate the miss in the first place.
- **Doc citations:** when citing a spec doc by file and line, resolve the line against the actual file in this repo before asserting it says something.

### Honesty about what you don't recognize

If a tool, library, or concept is named that you don't recognize, **say so explicitly** before guessing. Do not confabulate a plausible-sounding answer. The cost of "I don't recognize that — let me search" is minutes; the cost of a fabricated citation discovered in review is trust, and the cost of shipping one is paid by every user downstream.

### When you finish a task, do what was actually asked

If the user asks you to research and write findings to a doc, write the doc. Don't stop at the chat summary. Delivering a full synthesis in chat but never writing it to the file the user asked for is a real failure mode — don't repeat it.

## Working conventions

- **Spec source is the source of truth.** Every decision goes in the decision log (or an ADR) before code, not after.
- **No emojis in files** unless the user explicitly asks.
- **Language conventions come from the installed pack** (`packs/<language>/pack.yaml`): blessed test stack, linters, security scanners. Re-verify tool versions against their release pages before pinning — never pin from memory.

## Factory rules — enforced by CI hooks, not by this file

The factory has computational controls that enforce invariants at CI time. You do not need to memorize these rules — the hooks catch violations regardless. But you should know they exist, so you understand why CI fails when it does. The full rules and their enforcement mechanisms are documented in `docs/FACTORY_RULES.md`. Read that file when working on the factory itself (hooks, agents, scripts, CI), not when writing product code.

Key hooks you will encounter in CI failures:
- `scripts/citation-lint.sh` — resolves `<citation_prefix>*.md:NN` citations against `docs_root` (both configured in `factory.yaml`; empty prefix disables)
- `docs/examples/hooks/field-coverage-check.sh` — an example domain-invariant hook; copy and adapt it for structs whose fields must never silently escape a derived computation
- `scripts/hooks/shared-script-enforcement.sh` — verifies opencode, Claude Code, and Codex adapters call shared scripts, not inline logic
- `scripts/hooks/test-edit-denial.sh` — blocks the implementer role from editing `*_test.go`
- `scripts/hooks/ginkgo-only-check.sh` — permits `testing` only in `RunSpecs` bootstraps; all behavioral tests use Ginkgo/Gomega
- `scripts/hooks/commit-message-lint.sh` — conventional commits, max 6 bullets <=25 words each, no bare "verified" claim without command+output evidence (see `memory/lessons/001-verification-contract.md`)
- `scripts/hooks/direct-main-push-block.sh` — rejects direct pushes to `main`; push a feature branch and open an App-authored PR

### Write-time rules (not enforceable by hooks — must be in context)

- **Cite spec docs by `file:line`** in code comments and PR descriptions. The citation-lint checks that the file and line exist; it does not check that you cited the right line. That's your job.
- **The Verification Contract — you may only claim what you have observed.** Three levels: WROTE (no evidence), RAN (executed the check this session, pasted output), OBSERVED (saw it happen at the system level). Only RAN and OBSERVED may use "fixed"/"verified"/"works". Every "verified" line must cite the exact command and paste its real output. If you did not run it, write "written but NOT verified." "All N items fixed" is N independent claims — any item without evidence must be listed separately as NOT verified. Full contract in `docs/FACTORY_RULES.md`.
- **Protected paths (factory.yaml `protected_paths`) are permanently human-reviewed.** Every diff is reviewed line-by-line; no auto-merge, ever. CODEOWNERS enforces this at merge time; you enforce it by writing code that survives line-by-line review.
- **`FACTORY_AGENT_ROLE` must be set explicitly at the enforcement point.** The default is unset (allow). Only `implementer` is denied test-file edits. OpenCode derives the role from the session; Claude and Codex adapters must inject it from role-specific hook configuration.
- **Second-brain loop-close (Karpathy pattern).** At session end, write any lesson worth keeping to `memory/lessons/NNN-*.md` with provenance (the exact source: file:line, fetched URL + date, or "observed YYYY-MM-DD via <action>"). Lessons summarize and point; they never fork canon. A lesson without provenance is worse than no lesson — it becomes a stale fact asserted with confidence (the project's known scar tissue). The wiki (`wiki/`) is the agent-maintained query layer; your spec source (the `docs_root` in `factory.yaml`) and the ADRs stay the source of truth. A wiki page that restates them will drift into a second truth.
- **Loop-close nudge (two-layer trigger).** The opencode plugin writes `memory/.pending-lesson-reminder` on every `session.idle` event (after each agent turn). At the start of your next turn, if this file exists: reflect on whether the previous turn revealed a non-obvious fact (gotcha, version mismatch, API shape, bug fix that cost time). If so, write `memory/lessons/NNN-*.md` with provenance. Delete the flag file regardless. At process exit, the `dispose` hook calls `scripts/hooks/loop-close-check.sh` — if files changed but no lessons were written, it writes `memory/PENDING-LESSONS.md` as a reminder. These are nudges, not enforcement; the agent-discipline rule above is the backstop.
- **Agent-authored PRs declare their provenance** with two trailers in the PR body: `Agent-Runtime: <opencode|claude-code|codex>` and `Agent-Role: <spec-writer|implementer|refactorer|reviewer|wiki-maintainer>`. This costs one line and lets any audit or governance tooling attribute the work without guessing.
