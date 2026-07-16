# Patterns

**In short**

- Six named failure patterns from building and operating the factory this template came from. Every one is a real incident.
- Each produced a rule or a hook; the rules make more sense once you know the failure behind them.
- The common thread: a control that looked like it was working, and wasn't.

## Checker theater

**The failure.** A checker existed to verify that a plugin delegated its enforcement to a shared shell script. The plugin didn't delegate — it reimplemented the logic inline — but the checker passed anyway, because it grepped for the script's name and a comment mentioned the required call. The control looked enforced. It was prose.

**The fix.**

- Strip comments before checking. A checker satisfiable by a string in a comment checks documentation, not behavior.
- Require actual call sites, not mentions.
- Break/fix-test the checker itself: introduce the exact violation, watch it fail. A checker is code and gets the same verification discipline.

**Produced:** Verification Contract rule 6, and the break/fix fixture requirement for every hook.

## The unexecuted verification claim

**The failure.** Across a multi-round review history, every false "verified" or "fixed" claim shared one property: the check backing it had never been executed. Not one false claim came from a check that ran and was misread. The failure mode was never bad verification; it was verification that didn't happen, asserted as if it had.

**The fix.** The claim levels — WROTE (no evidence, may not say "verified"), RAN (check executed this session, output pasted), OBSERVED (watched it happen at the system level). Plus a commit lint that rejects any "verified"/"fixed"/"works" claim lacking a command-and-output citation.

**Produced:** the entire contract in [FACTORY_RULES.md](FACTORY_RULES.md), and `commit-message-lint.sh`.

## git add -A re-stages what git rm --cached removed

**The failure.** A file was removed from the index with `git rm --cached`, success output and all. A later `git add -A` in the same session silently re-staged it, and the file that was "verified removed" shipped in the commit.

**The fix.** Pair the removal with the `.gitignore` entry in the same commit, so the re-add is impossible rather than merely avoided. And confirm the removal with `git ls-files` immediately before commit — a command's success message says nothing about what later commands undid.

**Produced:** Verification Contract rule 5 (read back state after mutating it).

## Frontmatter beats central config

**The failure.** An agent's behavior was configured in two places: a central JSON config and the frontmatter of the agent's own markdown file. The frontmatter silently won. A permission change in the central config had no effect, and nothing reported the conflict.

**The fix.** When a value exists in two layers, change both in the same commit, or run an experiment to establish which layer wins. Better: generate one layer from the other with a CI drift check, so a divergence can't be committed. That's why this template generates the Claude Code and Codex adapters from the canonical opencode config.

**Produced:** the canonical-config-plus-generated-adapters architecture and the drift checks in CI.

## Headless permission semantics

**The failure.** A permission set to "ask" behaved three ways depending on context: interactively it prompted, in a headless primary session it auto-rejected, and in a headless child session it hung forever waiting for an answer no one could give. A hook that worked in manual testing deadlocked the factory in automation.

**The fix.** Test hooks in the execution context they'll actually run in. "Ask" is a family of behaviors selected by session type; the only way to know which one you have is to observe it there.

**Produced:** Verification Contract rule 2 — evidence from an interactive session is not evidence about headless behavior.

## The dead pipeline

**The failure.** A checker was built as `grep -q ... | grep -vq ...`. It can never fire: `grep -q` exits on first match without writing to stdout, so the second grep reads an empty stream and the compound evaluates the same way regardless of input. It passed on every commit, including commits containing the exact defect it existed to catch.

**The fix.** Prove checkers with break/fix before trusting them. The dead pipeline passed hundreds of times; one break/fix run would have exposed it in seconds.

**Produced:** Verification Contract rule 3, and the fixture requirement for every hook in this template.

---

Every pattern above is a control that looked like it was working — a passing checker, a success message, a config that read as authoritative. The template's answer is the same each time: don't infer that a control works, observe it working, and where possible make the observation itself computational so it can't quietly stop happening.
