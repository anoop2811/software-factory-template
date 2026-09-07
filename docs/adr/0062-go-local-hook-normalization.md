# ADR-0062: Go local-hook entry normalization

Status: accepted for a developer candidate; installed cutover remains gated.

## Context

The G1 sourceable-library contract in specs/001-go-runtime-conversion.md:306
includes `factory_local_hooks`. Its consumers expect one entry per line, with
attached flags retained. The baseline in `scripts/lib/config.sh` also uses the
calling shell's field splitting and pathname expansion. Replacing those shell
primitives with a Go whitespace split would change configured hook behavior.

## Decision

Under Decision 57, add private protocol `config hooks MODE [TOKEN...]`, where
MODE is exactly `words` or `commas`. Cobra passes tokens literally, including
flag-looking tokens, without interpreting them as options. Missing/unknown modes
return status 2, no stdout and a diagnostic without a usage banner.

Go owns entry grouping and normalization. In words mode a nonempty token not
starting with `-` starts an entry; subsequent flag tokens attach with one space.
Leading flag tokens are ignored. An empty token terminates the current entry
and resets flag attachment, without itself producing an empty output line;
non-whitespace IFS delimiters can create these empty tokens. In commas mode each operand
is one field: collapse ASCII space, tab, LF, CR, vertical tab and form feed to
one space, trim the edges and omit empty fields. Emit each entry with one LF.
Never run a hook, inspect its existence, evaluate its text or perform expansion
inside Go. Unicode whitespace remains literal under the C/POSIX contract.

Add `factory_local_hooks` to the existing candidate `runtime/shell/readers.sh`.
Read the configured value through `factory_config_get local_hooks`; select comma
mode when it contains a comma. Preserve baseline shell expansion by passing the
unquoted value through the caller shell's field splitting and pathname expansion
once. Words use the caller's IFS; commas temporarily use only comma as IFS.
Pass the resulting positional arguments literally to Go. Shell expansion is a
host compatibility primitive, not a second entry parser. Never use eval or
construct shell source from configuration. Ignore surplus function arguments as
before. Sourcing defines functions only and must not invoke the runtime.

Run the helper in a subshell to preserve caller variables, IFS, options, current
directory and positional arguments. Use the existing explicit absolute runtime
selection and child-only environment bridge. A failed selected-runtime invocation
must propagate its status with no legacy fallback; this private candidate
admission behavior does not authorize replacing the installed helper's historical
masked-failure behavior. Baseline read-error/locale/NUL framing and unusual shell
state remain subject to the existing reader characterization boundaries. This
slice qualifies ordinary C/POSIX Bash/sh contexts plus independently exercised
IFS and glob options, not every shell extension or caller-defined function.

## Inert scratch transport refinement

Independent review reproduced code execution when a caller had declared a new
scratch variable as an integer: assigning literal `array[$(touch marker)]` from
configuration caused arithmetic evaluation. A subshell protects parent state,
but it does not prevent that execution. Avoid named scratch assignments entirely.
Carry captured reader data and a separately delimited numeric child status in
quoted positional parameters; validate the child outcome before expanding data.
Preserve previous IFS set/unset state and value in positional slots around comma
splitting. Reader text never becomes an arithmetic assignment or a variable name.

The adapter must keep literal output and leave inherited integer, array and
nameref variables untouched, including under Bash localvar_inherit where available.
Independent regression must first demonstrate the marker-file side effect, then
require literal registration output and absence of the side effect. This corrects
the new candidate adapter, not the baseline or its installed routing.

## Acceptance and delivery

An independent spec-writer records failing Ginkgo/Gomega subprocess acceptance
before implementation. Compare explicit results with both immutable reader
baselines and the candidate. Exercise comma/word parsing, attached flags,
empty/duplicate settings, literal metacharacters, custom IFS, shell glob options,
source-time inactivity, caller-state preservation and failed runtime selection.
Direct protocol tests reject malformed requests and prove literal token handling.
The existing source gate covers the modified shim and all Go acceptance.

No dependency changes, new public command, installed adapter changes, writes,
activation, backup or deletion occur. Configuration writes, unresolved reader
parity, release qualification and the transactional recovery/cleanup lifecycle
remain required before G1 cutover. This slice does not complete G0 or G1.
