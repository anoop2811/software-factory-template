# ADR-0072: Go budget argument compatibility

Status: accepted for source implementation under specs/001-go-runtime-conversion.md.

## Scope and authority

Decision 71 completes the source budget command interface's remaining argument
contract. Keep the private `FACTORY_BRIDGE_PROTOCOL=1 factory budget controller`
boundary and the existing installed Bash/Python route. This changes no installer,
model routing, runtime activation, backup retention or script retirement.

The immutable oracle remains commit 4cc771e894e11d5024032106aeb0ef788997bd29,
scripts/lib/budget.py:593. The specification requires Cobra routing and observable
compatibility (specs/001-go-runtime-conversion.md:295), and explicitly permits
help prose differences (specs/001-go-runtime-conversion.md:457). This supersedes
ADR-0071's temporary refusal of help and abbreviated flags, not its execution,
metadata, cancellation, locking or output-failure contracts.

## Argument contract

Preserve exact plan/run/report command names, required options, per-command flag
scopes, defaults, duplicate scalar last-value behavior, separate and equals-form
values, and literal value boundaries. Accept unambiguous long-option prefixes
within the current command; exact names take precedence. Reject ambiguous or
unknown options and unknown commands without effects. Do not add command aliases
or completion commands. The sole short option is help (-h); preserve its valid
repeated grouping and reject attached non-help characters/values as the oracle
does. A command name cannot be abbreviated.

Preserve Python's distinction between a scalar value and an option-looking token:
a lone dash, ordinary text and recognized negative decimal literals may be values;
an option-looking token cannot silently become a missing scalar's value. An
equals-form scalar carries its value literally. The -- terminator must not turn
leftover operands into supported positionals or execute hidden Cobra commands.
Boolean --json is a presence switch; explicit values remain invalid.

Help at root and each leaf succeeds with code 0 on stdout and empty stderr,
without reading configuration/history/prompt files or probing/launching a CLI.
It remains discoverable with -h, --help and unique long prefixes. Preserve the
oracle's observable precedence where help competes with malformed input: already-encountered
missing values or invalid choices cannot be masked by help, while ambiguity
detected anywhere in the current parser scope defeats help. Deferred unknown
operands may be superseded by valid help. Compare
these combinations against the immutable oracle before changing production.

Ordinary argument errors return code 2, empty stdout and a useful stderr usage
plus error diagnostic. Semantic session/task identifier errors remain code 2 and
no effects. Configuration/runtime errors keep the existing factory diagnostic.
Never echo raw argument values in new diagnostics: prompt-file arguments and
unknown operands can contain private data. Identify the option/category instead.

## Implementation and reuse

Use the current Cobra/pflag dependencies, not a new CLI framework or a Python
runtime subprocess. A small compatibility normalizer may classify literal
operands and canonicalize accepted option names/values before Cobra parsing.
Derive flag names and arity from registered flags rather than maintaining two
independent schemas. Separate parsing/presentation from execution; keep budget
validation, admission, supervision and accounting in their existing components.

Keep the leaf-first parse and no-leftover-operand check before Cobra command
discovery. Help output must use checked writes, including short-write and flush
errors, and must not read execution state. A failed help write returns nonzero
with a bounded diagnostic and never launches or writes accounting.

## Enumerated normalization and independent evidence

Before implementation, the independent evaluator writes compiled-CLI
Ginkgo/Gomega cases and observes failures for the intended missing behavior.
Use the immutable Python oracle for the argument matrix; fake CLIs only.

For help and argument diagnostics ONLY, normalize human wording, wrapping,
program name/path, color and Python-version-specific option-section headings.
Still compare exit status, which stream is nonempty, usage/error presence,
command/option discoverability, successful parsed values and absence of effects.
Do not normalize status differences, missing advertised options, changed option
arity/scope, successful metadata fields, unexpected output or native launches.
For valid plan/report retain ADR-0071's exact text / typed JSON comparison.

The matrix covers all three commands, help spellings, valid/ambiguous/unknown
prefixes, duplicates/defaults, scoped flags, scalar and boolean equals forms,
missing/empty values, negative/option-looking values, short-option groups, --,
root versus leaf help/error precedence, ID/harness/role validation and hidden
completion operands. Fault-injection tests cover help output failures. Legacy
source tests asserting help/prefix refusal must be replaced by stronger parity
cases, not merely deleted. Existing controller/metadata tests remain passing.

Completion of this source interface does not close G2 activation or release
readiness: shell model selection, loop integration, mixed-runtime rollout and
transactional retirement remain separate inventoried obligations.

## Source references

Fetched 2026-09-23: Python argparse documents default unique-prefix matching,
help and option/value grammar: https://docs.python.org/3/library/argparse.html.
Pinned Cobra source confirms explicit flag parsing and command execution:
https://raw.githubusercontent.com/spf13/cobra/v1.10.2/command.go.
Pinned pflag source confirms registered flag metadata and flag parsing:
https://raw.githubusercontent.com/spf13/pflag/v1.0.9/flag.go. No versions change.

## Shared checked output

Extract the existing event-write primitive into a small internal/output package
so budget renderers and command help share short-write, flush and context error
handling. This is a mechanical extraction of the existing behavior, not a new
output framework or a change to accounting/presentation order. Preserve sanitized
command diagnostics and exercise both metadata and help writers independently.

Help must identify which options are required and the supported harness/role
choices, using shared registered/domain metadata. Human-prose normalization does
not permit misleading optionality or omission of accepted choices.
