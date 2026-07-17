# Glossary

**In short**

- Plain definitions for the terms the other docs use without stopping to explain.
- If a word here surprises you, the doc that uses it will make more sense after.

---

**Harness** — the tool that drives a coding agent: reads your prompt, calls the
model, runs tools, edits files. This template supports three: opencode, Claude
Code, and Codex. The enforcement layer is plain shell, so it works with any of
them.

**Canon / canonical config** — the single source-of-truth configuration
(`opencode.json` plus `.opencode/`). The Claude Code and Codex configs are
*generated* from it by `sync-claude.sh` / `sync-codex.sh`, never hand-edited.
"Three harnesses, one canon" means you configure once and the adapters follow.

**Adapter** — a generated, harness-specific config derived from the canon
(`.claude/`, `.codex/`). A drift check fails CI if an adapter stops matching
what the canon would generate.

**factory.yaml** — the one flat config file each project owns. Hooks read it at
runtime, so the hook scripts stay byte-identical between this template and
every adopter. Keys: `protected_paths`, `test_file_patterns`, `check_command`,
`citation_prefix`, `docs_root`, and a few more.

**Agent role** — a named job with its own permissions: spec-writer, implementer,
refactorer, reviewer, wiki-maintainer. Roles are separated by what the hooks
let each one do, not by instructions asking them to behave.

**Generator / evaluator separation** — the thing that writes code is never the
thing that judges it. The spec-writer writes tests; the implementer makes them
pass but cannot edit them; the reviewer is a different model. Collapsing these
into one session is the failure the separation exists to prevent.

**The Verification Contract** — the rule that you may only claim what you have
observed: `WROTE` (wrote it), `RAN` (ran it, here is the command and output),
`OBSERVED` (watched it happen). Only `RAN` and `OBSERVED` may say "verified."
The commit lint enforces it. See [CONCEPTS.md](CONCEPTS.md).

**Break/fix (proof)** — the discipline of proving a check works by watching it
fail: introduce the violation, see the gate fire, revert, see it pass. A check
you have only ever seen pass proves nothing.

**Golden vector** — a fixed input with a hardcoded expected output, committed as
a constant. If a computation changes, the golden vector breaks the build even
when every other test still passes.

**The ratchet** — enforcement tightens automatically but loosens only by a
recorded human decision. Agents may propose relaxing a rule; they may never
apply the relaxation themselves.

**Pack** — an opinionated language bundle (`packs/<lang>/`) that arms
`test_file_patterns` and `check_command` for a language and ships its blessed
tools. The core works with no pack; a pack just wires the two language-specific
knobs.

**Protected path** — a directory that requires human review on every change, via
CODEOWNERS and branch protection. Set in `factory.yaml` `protected_paths`.

**Provenance** — the recorded source of a claim or lesson: `file:line`, a fetched
URL with a date, or `observed YYYY-MM-DD via <action>`. Facts without provenance
are treated as suspect.

**Second-brain / loop-close** — the habit of writing a short, cited lesson to
`memory/lessons/` at the end of a work session, so the project learns from each
failure instead of repeating it.
