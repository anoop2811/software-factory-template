# ADR 0136: native source configuration and role boundaries

Status: accepted design; implementation and qualification evidence are separate

Decision: 151

## Scope

Complete R3 group 2: `scripts/factory-migrate-config.sh` and sourceable
`scripts/lib/config.sh`, `scripts/lib/roles.sh`, `scripts/lib/budget-config.sh`.
Their Go migration, configuration, role and helper implementations already exist.
Reuse those implementations and the canonical shell plan adapters; do not add
another parser, model-tier map or settings planner. Installed generation and
immutable predecessor recipes retain their existing contracts.

## Executable migration adapter

The direct migration path selects its physical source checkout's `factory`,
preserving literal arguments, cwd, environment, stdin, stdout, stderr and status.
Directory aliases and literal newline paths remain supported; a final-file alias
is refused. Missing, nonexecutable or failed owners do not select PATH, caller,
home, an environment-selected runtime or a historical shell fallback. Actual
source execution retains the qualified compiler requirement; installed launchers
continue to select their packaged runtime without a compiler.

The existing native migration owns dry-run/apply, caller Git-root selection,
YAML precedence, inert legacy parsing, preservation of the renamed backup and
refusal before mutation when that backup already exists. This cutover changes
the entrypoint, not those data or recovery contracts.

## Shared source library transport

Bash-capable callers load the existing `runtime/shell/readers.sh`,
`runtime/shell/config.sh` or `runtime/shell/helpers.sh` definitions as needed,
then load `runtime/shell/source.sh` to replace only their private transport.
The shared source transport selects the physical defining checkout's `factory`;
ambient `FACTORY_RUNTIME_BINARY` does not redirect it. Load the shared transport
by its absolute physical path so later caller `cd` does not change ownership.
Temporary loader state stays local; no persistent mutable owner variable is
used to select a different checkout. Final-file library aliases are refused.

Sourcing config and roles performs no native, Git or compiler invocation and
preserves caller cwd, shell options, IFS, positional arguments and bridge controls.
Builtin-only path resolution and reading the canonical definitions are allowed.
Configuration retains its existing source-time key constant. Budget settings
retain their intentional source-time export via `factory_budget_config` after
loading the canonical helper adapter. No shell configuration is evaluated.

Child-only controls carry the protocol, selected configuration and helper inputs;
they do not assign to caller-local or readonly control variables. Canonical
configuration and helper plans remain responsible for literal effects, complete
validation before application, attribute refusal and status propagation. Shell
word splitting and pathname expansion for local hooks remain in the canonical
shell adapter; Go retains normalization and configuration interpretation.

## POSIX role callers

POSIX shells cannot generally discover the path of a sourced file. When actual
Bash source-path support is unavailable, the inert role library defines only
literal forwarding functions and resolves its runtime at invocation: an explicit
absolute regular executable `FACTORY_RUNTIME_BINARY`, otherwise the caller Git
repository's `factory`. A supplied invalid or failed runtime refuses; it does
not fall through to another selection. Outside a repository, such callers must
supply the explicit runtime. This limitation is stated rather than pretending
POSIX can discover a foreign sourced checkout. macOS `/bin/sh` exposes Bash's
source-path support and follows the physical-owner branch. No role/tier decision
is duplicated in either shell path. Missing operands and nounset behavior retain
the sourceable function contract.

## Consumers and historical custody

Source proof and live policy fixtures that copy the new libraries must also
carry their canonical bridge closure and a real private native owner. In
particular, source selftest's private gate fixtures, current configuration
precedence tests, structural role checks and recovery discovery checks continue
to exercise current policy. Installed proof fixtures and immutable recipes must
not acquire a new dependency on source-only transport files.

Historical Bash controllers receive matching historical migration and reader
bytes from their declared immutable source. General, optional-pack, Java,
budget, loop and transition fixtures must distinguish those dependencies from
current gate subjects. Freezing the current configuration library globally or
silently replacing a live policy test with an old implementation is not allowed.
Keep existing assertions; preserve complete historical dependencies even when
a fixture requests only a subset of old executable entrypoints. Saved bytes are
development-only test inputs, never a missing-native-owner fallback.

The historical command tree already uses revision
`1754de23de95a7e3c734bedd86894295e823c988`; the review oracle uses
`f3a0b6520917e104883cc89b123d7244fa0c46a7`. Resolve each affected fixture against
its existing identity, compare the relevant bytes, and record its disposition.
Current source image and copy inventories must include required source assets
without changing historical installation-generation ownership.

## Qualification

An independent test author observes the old paths fail native source-boundary
criteria before production changes. Qualify literal transport, owner refusals,
inert config/role sourcing, POSIX selection, configuration precedence and literal
effects, budget exports/model selection, and an actual current policy gate.
At least one private checkout uses the real source launcher and compiler.
Exhaustive semantic matrices may use a private compiled owner while still
traversing the current source libraries; never compile the operator's primary
checkout as a side effect of a test fixture.

Reuse existing native migration/readers/export/hooks/helper criteria for detailed
semantics and malformed-plan cases. Add boundary coverage only where missing.
Run affected historical fixtures, source selftest, structural/recovery checks,
race acceptance and normal quality gates. Independent correctness, security and
test review precedes hosted review and merge. Progress stays at 16/32 merged
paths until this four-path group is qualified and merged; then it is 20/32.
R4-R7 release work remains separate.
