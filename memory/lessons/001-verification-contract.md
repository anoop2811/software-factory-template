# Lesson: The Verification Contract — four rounds of false "verified" claims

## Date
2026-07-05

## Context
Across four review rounds on the original factory scaffold, every false "fixed/verified" claim was a check that had not been executed, and every check that was actually executed held up. The pattern:

- Round 1: claimed "hook blocks implementer" — tested the shell script in isolation, not the harness wiring
- Round 2: claimed "plugin calls shared scripts" — the plugin reimplemented the logic inline; the checker was satisfied by a comment
- Round 3: claimed "fixed the broken exec( pipeline" — the pipeline was still dead; the break/fix demo only exercised a different check
- Round 4: claimed "verified: all items fixed" — the execFile `input:` option doesn't exist on async execFile, causing a deadlock that would freeze every edit in a real session

## Root cause
The same agent wrote the spec, the tests, the implementation, and the verification claims in the same session. Generator/evaluator collapse: the agent that wrote the rule also judged whether the rule was followed. No adversarial review caught the divergence until a separate model cross-audited.

## The fix
The Verification Contract (stored in `docs/FACTORY_RULES.md`): you may only claim what you have observed. Three levels (WROTE/RAN/OBSERVED). Only RAN and OBSERVED may use "fixed"/"verified"/"works". Eight rules. The key ones:

- Rule 1: no claim without execution (cite command + paste output)
- Rule 3: every check must be seen to fail (break → FAIL → revert → PASS)
- Rule 5: read back state after mutating it (git ls-files, not just command output)
- Rule 7: batch claims decompose (N items = N independent evidence lines)

## Ratchet tightening
The commit-message lint hook (reject "verified"/"fixed" lines lacking command+output citation) is the computational enforcement of this lesson. Prose rules decay; hooks don't. Once implemented, this class of false claim is caught mechanically.

## Applies to
Every agent, every model, every harness. The contract is model-agnostic and survives model switching because it lives in committed files, not in model memory.
