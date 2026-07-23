# Task: reference-answer

A trivial reference task that exercises the eval harness end to end without a
language toolchain. A real task would be a red acceptance spec (Ginkgo, pytest,
JUnit) the implementer must make pass; this one keeps the oracle in pure shell
so the harness can self-test anywhere.

## Requirement

Create a file `answer.txt` in the repository root whose contents are exactly the
word `FIXED` (a single line).

## Acceptance

`verify.sh` passes when `answer.txt` exists and contains `FIXED`.
