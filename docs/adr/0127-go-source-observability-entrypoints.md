# ADR 0127: native source report and metrics entrypoints

Status: accepted bounded R3 function; independent semantic RED precedes conversion

Date: 2026-11-01 UTC

## Scope

After PR #157 merged as `90240e42b17342c7899ffb2df78fd2771ac15e8b`,
replace the active bodies of `scripts/factory-report.sh` and
`scripts/factory-metrics.sh` with direct adapters to their physical owner's
`factory report` and `factory metrics`. Reuse the existing Go owners and the
source launcher qualified in ADR 0126. This is the observability entrypoint
portion of R3, not completion of all helper or proof-runner retirement.

## Compatibility and authority

The adapters use the same physical-root resolution and file-symlink refusal as
the source init/upgrade adapters. Preserve literal arguments, caller cwd,
environment, stdin, stdout, stderr and exit status. A path containing whitespace,
shell metacharacters or trailing newlines remains data. Do not select a runtime
through PATH, the caller's repository or an environment override. A missing or
failed owner must fail; there is no shell/Python fallback.

The Go owners continue to interpret arguments. Report retains its template-root
event default and first-argument clear semantics; metrics retains its caller
project, schema, text/HTML output and browser consent rules. Existing installed
recipes generate their own compiler-free adapters and do not change. Direct
source use has the same provisioned-Go requirement as the source launcher.
No dependency, tool version, public default, timeout or model setting changes.

## Historical evidence and current coverage

The broad historical shell selftest still tests old implementation details,
including embedded Python quoting. Materialize report and metrics there from
the same immutable pre-conversion revision already used for its initializer:
`1754de23de95a7e3c734bedd86894295e823c988`. Explicitly route those historical
fixtures to that private copy, preserving their assertions. Missing historical
objects fail qualification; no old implementation is retained in active source
or used as a runtime fallback. Current policy gates keep their current owners.

Existing frozen report/metrics Go parity suites remain independent historical
oracles. Add current-source acceptance of both direct adapters, with a failing
pre-conversion semantic control. Prove actual native output and selected data
roots as well as literal transport, failed-owner propagation and symlink refusal.
Use private fixtures; never execute or replace the operator-owned `factory-go`.
Keep historical checks visibly historical so their success cannot claim current
entrypoint behavior. Final proof-runner cleanup remains a later R3 boundary.

## Qualification

Run the new entrypoint criteria, existing native report/metrics criteria and the
complete historical shell selftest in an isolated source copy. Preserve the
operator's uncommitted files. Independent review follows the repository's review
diamond, and hosted checks must pass before this bounded PR can be merged.

The optional-pack selftest drops source Git custody after materializing the old
dispatcher and initializer. It must carry the same two historical observability
scripts at that boundary: otherwise the old initializer marker prevents the
runner's own historical overlay, leaving new adapters paired with an old
dispatcher or copied into an ownerless metrics fixture. Preserve its complete
no-pack and missing-review-template scenarios and all their assertions.
