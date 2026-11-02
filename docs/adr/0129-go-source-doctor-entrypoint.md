# ADR 0129: native source doctor entrypoint

Status: accepted bounded R3 function; independent semantic RED precedes conversion

Date: 2026-11-02 UTC

## Scope

After PR #159 merged as `8f4b80a51b03b15e253ff6fba94db9dd6da22b60`,
replace the active body of `scripts/factory-doctor.sh` with an adapter to the
existing Go doctor. This also activates the existing `make doctor` route. Reuse
docs/adr/0086-go-native-doctor.md:18 and the native orchestration contract
from docs/adr/0099-native-factory-orchestration.md:31. Other direct commands,
sourceable helpers and historical proof controllers remain separate R3 work.
No dependency, version, model, timeout or public release default changes.

## Direct invocation compatibility

Resolve the physical script owner with the established source adapter pattern.
Reject final-file symlink invocation and exec that owner's `factory doctor`.
Preserve literal operands, cwd, stdin, environment, stdout/stderr and exit status,
including physical paths ending in newlines. Do not select the runtime through
PATH or environment overrides. Missing or failing owners fail without fallback.

Forward operands exactly as the existing installed doctor adapter does. The
native doctor owner already ignores them, including `--help`; it does not pass
them to a strict selector parser. Prove both literal forwarding and ignored
native behavior. This reuses internal/installation/generate.go:51 and
internal/cli/cli.go:302 rather than changing installed adapter generation.

The runtime executable belongs to the script owner; diagnostic subjects belong
to the caller project. Preserve Git-root discovery with cwd fallback and relative
FACTORY_CONFIG resolution against the selected project. Distinct owner/caller
configurations must not be confused. Gate inputs, review status and the retained
proof come from that selected project, not the source dispatcher directory.

Native synchronization and review-lane owners remain reused in Go. Adapter drift
inspection uses private scratch data rather than replacing live generated files.
The caller's `scripts/selftest/run.sh` remains an explicitly supervised proof
boundary with empty child input and the existing deadlines/capture limits. A
failed proof remains a diagnostic failure. Custom selftests are trusted code;
this conversion does not promise to sandbox or prevent their own side effects.
No model request, dependency installation or migration authority is introduced.

## Historical consumers

Keep coherent historical engines in the five existing materializers: the full
shell runner, optional-pack wrapper, Java installer matrix, budget delivery and
loop delivery. Add the original doctor body from immutable revision
`1754de23de95a7e3c734bedd86894295e823c988` beside their frozen dispatcher/init/upgrade
inputs. A historical dispatcher paired with a new doctor adapter would recurse
if doctor were invoked. The current audited fixtures either do not invoke doctor
or suppress it as nested upgrade work; this is fixture-coherence prevention,
not a claim of a reproduced production failure.

Preserve current policy-gate subjects and every historical assertion. Existing
native doctor oracles and initializer/installation archives already retain
immutable inputs and need no blanket rewrite. Saved historical scripts remain
test data, never a production fallback.

## Qualification

An independent test author must observe the current direct script fail native
delegation criteria before replacement. Exercise one real private source launcher,
caller-root configuration/proof selection, ignored operands, health/failure
statuses and live adapter preservation. Poison legacy shell dependencies so a
historical implementation cannot earn native delegation credit.

Reuse exact sanitized child environments through the existing bounded runner.
Cover literal transport, physical/directory aliases, decoy runtimes, final-file
symlink refusal and missing/failed-owner behavior. Reuse existing native doctor
parity, scratch safety, progress and cancellation criteria; do not duplicate the
diagnostic engine. Actual discovery must retain all existing criterion names and
place new criteria in the current source-entrypoints partition.

Run the new criteria, affected existing native diagnostics and historical
consumer suites, configured quality checks and the review diamond. Run commands
that dispatch source builds, generators or caller proofs only in private copies.
Preserve operator files and the primary developer binary, including an inert
exact backup before qualification. Hosted checks and review disposition precede
merge and the next R3 function boundary.

The health-only native fixture explicitly disables its review lane. An inherited
`REVIEW_LANE=on` must not override that fixture choice or authorize a GitHub secret
query. Use a controlled client witness when proving this environment boundary.
