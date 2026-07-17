# Hooks reference

**In short**

- Every deterministic control is a shell script in `scripts/hooks/`. Adapters and CI call the same scripts; nothing is reimplemented inline.
- Every hook is break/fix proven: CI introduces the exact defect, watches the hook fail, reverts, watches it pass.
- Hooks read `factory.yaml` via `scripts/lib/config.sh` — hook files are byte-identical between template and adopters.
- The edit path fails open if a script is missing; `hook-existence-check.sh` in CI catches that before merge.

On the fail-open: if a hook script is missing or unexecutable, edits proceed with a log line, because failing closed would halt all work — including the spec-writer writing tests — on a missing file. The CI existence check is the compensating control.

---

## citation-lint.sh

Resolves every `<citation_prefix>*.md:NN` reference in code and docs against your spec source, so a citation can't point at a file or line that doesn't exist.

**Fires when:** CI and `make check`, every commit. Skips silently when `citation_prefix` is empty (the check is opt-in).

**Exit codes:** 0 pass or skip, 1 a citation doesn't resolve.

**Configuration:** reads `citation_prefix` and `docs_root` from `factory.yaml`.

## commit-message-lint.sh

Rejects malformed commit messages and any "verified"/"fixed"/"works" claim lacking a command-and-output citation.

**Fires when:** CI lints every commit in a PR's BASE..HEAD range; locally via `make lint-commits` (HEAD). Takes a SHA argument or a message on stdin. Merge and revert commits are exempt from the subject check.

**Exit codes:** 0 pass, 1 fail.

```
$ git commit -m "fix: verified the retry logic works"
COMMIT-LINT FAIL: line claims verification but lacks command + output citation:
  fix: verified the retry logic works
  Every 'verified'/'fixed'/'works' claim must cite the exact command and paste its output.
  Or write 'written but NOT verified' if you did not execute the check.
```

Also checked: conventional-commit subject (`feat` `fix` `chore` `docs` `refactor` `test` `ci` `build` `perf`), no trailing period, body max 6 bullets of max 25 words each.

## decision-log-gate.sh

Requires commits touching governance-sensitive paths to reference a Decision number.

**Fires when:** CI on pull requests (base..head), the pre-push gate, and against the working tree when run with no arguments. Governance paths are the factory's own surfaces (`opencode.json`, the adapters, `scripts/`, `.github/workflows/`, `Makefile`, `specs/`, `factory.yaml`) plus every prefix in `protected_paths`. Commits older than `decision_gate_cutoff` are exempt.

**Exit codes:** 0 pass, 1 fail.

**Configuration:** reads `protected_paths` and `decision_gate_cutoff` from `factory.yaml`.

```
DECISION-LOG-GATE FAIL: commit 3fa9c12 touches governance-sensitive paths
  but does not reference a Decision number or ADR in the commit message.
  Changed governance paths:
    - scripts/hooks/new-gate.sh
  Add 'Decision N', 'Decision: N', or 'ADR-NNNN' to the commit message...
```

The fix is to record the decision and reference it, not to reword the commit to dodge the pattern.

## diff-aware-check.sh

Maps changed files to the re-verification they invalidate, and runs those checks.

**Fires when:** CI on every PR, the pre-push gate, and `make diff-aware`. Diffs working tree vs HEAD, or a base..head range.

**Exit codes:** 0 all dispatched checks passed (or none needed), 1 any failed.

**Configuration:** reads `protected_paths` and `check_command` from `factory.yaml`.

```
$ ./scripts/hooks/diff-aware-check.sh
diff-aware-check: factory-hooks.ts changed — parity re-eval REQUIRED
  WARNING: live parity (OBSERVED) re-verification required — cannot run in CI
  The previous OBSERVED pass is now stale.
```

It also writes staleness flags (`memory/.parity-stale`) that `pending-lessons-push-block.sh` refuses to push past. A stale OBSERVED claim is treated as no claim.

## direct-main-push-block.sh

Rejects any push that updates `refs/heads/main`.

**Fires when:** the tracked pre-push hook runs, before the expensive verification suite. Reads git's standard pre-push ref-update stream.

**Exit codes:** 0 no main update, 1 push denied.

```
$ git push origin main
direct-main-push-block: DENY direct push to main
Push a feature branch and open a pull request instead.
```

This is local enforcement, not server-side protection. If your hosting plan supports branch protection or rulesets, enable them as the authoritative layer and keep this as the fast local gate.

## ginkgo-only-check.sh (Go pack)

Enforces one Go testing dialect: the standard `testing` package appears only in each package's single `RunSpecs` bootstrap; behavioral tests use Ginkgo v2 and Gomega.

**Fires when:** CI and `make check`, every commit (Go pack installed).

**Exit codes:** 0 pass, 1 fail.

```
GINKGO-ONLY FAIL: pkg/parser/parser_test.go calls testing.T outside Ginkgo/Gomega
ginkgo-only-check: 1 violation(s) found
```

## field-coverage-check.sh (example, not an active gate)

The worked example of a domain-invariant hook: a struct's coverage function must include every field except deliberate exclusions, so a new field can't silently escape a derived computation.

**Fires when:** nothing by default. Lives in `docs/examples/hooks/`; copy into `scripts/hooks/` and adapt it once your project has a struct that warrants it. Skips silently if the configured record file doesn't exist.

**Exit codes:** 0 pass or skip, 1 fail.

```
FIELD-COVERAGE FAIL: RetryCount is in <Struct> but not in <CoverageFunc>
field-coverage-check: 1 field(s) missing from <CoverageFunc>
```

The shipped example uses invented types; the value is the pattern. When your spec says "X always holds", write a script that reads the code and fails when X doesn't hold, and ship it with a break/fix fixture.

## hook-existence-check.sh

Verifies every hook script referenced by the plugin and CI exists and is executable — the safety net for the deliberate fail-open edit path.

**Fires when:** CI and `make check`, every commit.

**Exit codes:** 0 all present and executable, 1 fail.

```
HOOK-EXISTENCE FAIL: scripts/hooks/test-edit-denial.sh is not executable
hook-existence-check: 1 script(s) missing or not executable
The plugin fails open when a script is missing — this check catches that before merge.
```

## loop-close-check.sh

A nudge, not enforcement: at session end, if files changed without a lesson written to `memory/lessons/`, it writes `memory/PENDING-LESSONS.md` as a reminder.

**Fires when:** the opencode plugin's `dispose` hook at agent process exit — best-effort and non-blocking, since opencode has no session-close event it could block on.

**Exit codes:** 0 nothing pending, 1 reminder written. The caller ignores the exit code; the escalation happens one layer up.

```
loop-close-check: changes detected but no lessons written
  Reminder written to memory/PENDING-LESSONS.md
```

## pending-lessons-push-block.sh

Blocks push while a session-end reminder or a staleness flag is unaddressed. The nudge is ignorable; this isn't.

**Fires when:** the pre-push gate and CI. Checks for `memory/PENDING-LESSONS.md` and `memory/.parity-stale`.

**Exit codes:** 0 clean, 1 blocked.

```
PENDING-LESSONS BLOCK: memory/PENDING-LESSONS.md exists
  Files changed during a previous session without a corresponding lesson.
  Either:
    1. Write the lesson to memory/lessons/NNN-*.md with provenance, then delete the reminder
    2. Or delete the reminder if no lesson is warranted (and accept the nudge was ignored)
```

The fix is to do the deferred work — write the lesson, or re-run the parity verification — and remove the flag as part of doing it.

## shared-script-enforcement.sh

Catches enforcement logic reimplemented inline in a harness adapter instead of delegated to `scripts/hooks/*.sh`.

**Fires when:** CI and `make check`, every commit. Greps the plugin surfaces (comments stripped) for known enforcement patterns and for missing `execFile`/`spawn` call sites to the shared scripts.

**Exit codes:** 0 pass, 1 fail (also skips with 0 if no plugin dir exists).

```
shared-script-enforcement: checking .opencode/plugin/factory-hooks.ts
SHARED-SCRIPT FAIL: .opencode/plugin/factory-hooks.ts contains inline enforcement pattern: _test\.go
  This logic belongs in scripts/hooks/, not in the plugin.
```

This hook exists because the failure happened: a plugin reimplemented test-edit denial inline while a checker was satisfied by a comment (see [PATTERNS.md](PATTERNS.md)).

## test-edit-denial.sh

Denies test-file edits to the implementer role — generator/evaluator separation at the write path.

**Fires when:** as a pre-tool-use hook in the agent's write path, not in CI. Accepts Claude Code/Codex JSON on stdin or an opencode file-path argument; all adapters call this same script. Only acts if `FACTORY_AGENT_ROLE` is exactly `implementer` and the path matches a pattern in `test_file_patterns`.

**Exit codes:** 0 allow, 2 deny.

**Configuration:** reads `test_file_patterns` from `factory.yaml`.

```
$ FACTORY_AGENT_ROLE=implementer ./scripts/hooks/test-edit-denial.sh pkg/parser/parser_test.go
DENIED: implementer role cannot edit test files (pattern: _test\.go([^[:alnum:]_]|$)). Generator/evaluator separation.
$ echo $?
2
```

Two deliberate defaults: an unset or non-implementer role allows (a missing env var must not block the spec-writer, whose job is writing tests), and empty `test_file_patterns` disables the check entirely.

## wiki-lint.sh

The "lint" operation of the LLM-maintained wiki pattern (raw sources → agent-written wiki → lint). An agent can write a wiki fast but won't reliably keep every page cited and every cross-reference real; this gate does. Ingest and query are the model's job — lint is the deterministic control that keeps the result honest.

**Fires when:** CI and `make check`, every commit; also surfaced by `factory doctor`. Skips (with a note) when there are no wiki content pages — a wiki with only its index/README has nothing to lint.

**Exit codes:** 0 pass or skip, 1 a page lacks provenance or a link doesn't resolve.

**Configuration:** reads `wiki_root` from `factory.yaml` (default `wiki`).

v1 enforces two invariants on every content page (the index/README are exempt):
- **Provenance** — each page cites a source: a `file:line` reference, a URL with a date, or `observed YYYY-MM-DD`.
- **Live cross-references** — every wiki-local markdown link and `[[wikilink]]` resolves to a file that exists.

```
WIKI-LINT FAIL: wiki/store.md has no provenance — cite a source (file:line, a URL with a date, or 'observed YYYY-MM-DD')
WIKI-LINT FAIL: wiki/api.md links to a missing page: store.md
wiki-lint: 2 problem(s) found
```

Orphan detection and source-drift/staleness are planned (Decision 15). This is what makes `wiki/README.md`'s "lint-gated at merge" a fact rather than a convention.
