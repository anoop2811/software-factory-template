## What this changes

<!-- One or two sentences. What does this PR do, and why? -->

## Decision

<!-- If this changes what the template enforces, a config key, a pack, or what an
adopter receives, link the Decision it implements (e.g. "Decision 11"). Decisions
go in docs/DECISION_LOG.md before the code. If this is a pure doc/typo/example
change that touches no governance path, write "none — non-governance change". -->

## Checklist

- [ ] Ran `make check` locally, or the body says "written but NOT verified"
- [ ] Commit messages follow the Verification Contract — any "verified"/"fixed"/"works" claim cites the command and its output
- [ ] Adds or changes a hook? A break/fix fixture proves it fails on the defect and passes without, and the hook is registered in `scripts/hooks/hook-existence-check.sh`
- [ ] Changed `opencode.json` or an agent role file? Ran `make sync-harnesses` and committed the regenerated `.claude/` and `.codex/` files
- [ ] Governance-path change references a Decision in the commit message

## Verification

<!-- Paste the commands you ran and their output. This is the evidence, not a
promise. "written but NOT verified" is an acceptable and honest answer. -->
