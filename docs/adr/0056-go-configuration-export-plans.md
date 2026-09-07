# ADR-0056: Go configuration plans with Bash variable application

Status: accepted for candidate implementation, 2026-09-07 UTC.

Continue Decision 51's Go conversion with factory_config_export and
factory_config_load_legacy. Go owns parsing, precedence and action selection;
a thin Bash adapter performs the parent-shell assignments that a subprocess
cannot perform. Existing installed libraries and public routing remain unchanged.

The same private FACTORY_BRIDGE_PROTOCOL=1 Cobra tree gains config export
[PRESERVED_KEY...] and config legacy FILE [PRESERVED_KEY...]. Preserved names
must belong to the fixed 17-key allowlist; caller values never enter this
protocol. Go must not infer caller presence from its own environment. Current
keys include REVIEW_REASONING_EFFORT, REVIEW_OPENROUTER_PROVIDER and
REVIEW_MAX_TOKENS in addition to the 14 historical settings. Unknown command
operands refuse with status 2.

Go emits a complete LF-delimited plan: FACTORY_CONFIG_PLAN_V1, zero or more
set<TAB>KEY<TAB>VALUE or export<TAB>KEY records, and END. SET means assign the
literal value with printf -v and export that variable; EXPORT changes only its
export bit. Legacy records retain their original order, including duplicates,
before YAML actions. A matching preserved key produces EXPORT instead of SET.
Missing/empty YAML values do not override legacy values; explicit empty legacy
assignments remain meaningful. Only fixed known keys can become actions.

The Bash candidate runtime/shell/config.sh defines the two sourceable helpers
and the static FACTORY_CONFIG_KEYS value, matching the existing source-time
constant assignment. It performs no runtime calls while being sourced. This
allowlist is retained adapter data, with acceptance checking equality to the Go
contract; parsing and precedence are not duplicated in shell. The adapter uses
only an explicitly selected absolute executable FACTORY_RUNTIME_BINARY, with
no PATH lookup, compilation, download, eval, generated shell code or temp files.

Full export snapshots which known caller variables are set, including local,
readonly and explicitly empty values. Standalone legacy loading still replaces
ordinary caller values unless its optional preserved_keys string contains the
known name with the existing space-delimited matching semantics. Preserve
matched caller values and the existing export-bit effect; unmatched locals
remain local. Runtime protocol/configuration transport must not assign to
readonly caller variables. Missing standalone legacy input diagnoses and exits
1; missing legacy input during full export is omitted.

Capture the complete runtime output and exit status before applying any action.
Validate the header, final END, every action and every known key before the
first parent-shell mutation. Failed, malformed or truncated plans refuse without
partial assignments. Read records without field splitting and peel exactly the
first two literal tabs for SET; VALUE is the untouched remainder. Empty values,
tabs, carriage returns, quotes, backslashes, equals signs and shell-looking text
remain data. Flat file values contain no LF; caller values, including embedded
newlines, are never serialized. Preserve native Bash assignment/export behavior
when applying an ordered valid plan, including readonly assignment diagnostics.

Independent outside-in Ginkgo/Gomega acceptance must observe compiled boundary
RED before implementation. Compare immutable merged Bash source at 2e3609b
with independent expected values, export bits and process results. Keep older
baseline evidence unchanged; new fixed settings are additions to its 14-key
contract. Exercise literal data, duplicate legacy assignments, missing files,
caller precedence, local/readonly/empty values, malformed protocol, runtime
failure, and one-line values containing leading/interior/trailing tabs. Bound
fixture subprocesses to ten seconds and make no native model calls.
Also compare the original 14 settings with both immutable historical baselines
after the existing, hash-checked EX-001 correction is applied only to isolated
fixtures. The 16-key oracle at 2e3609bfbb22698561167747caf175e3dbdb9e74 is
unchanged; REVIEW_MAX_TOKENS is the subsequent candidate-only addition.
Historical correction artifacts and Git objects remain unchanged.

This remains a candidate slice, not G1 completion or an adopter activation.
Use Bash scalar variables and the existing C/POSIX parsing scope. Arrays,
integer/nameref attributes, deliberately altered allowlists, nonregular legacy
inputs, NUL-bearing legacy inputs, mid-read failures and non-C locale differences require characterization
before public replacement. Packaging, recovery, cleanup and default cutover
remain governed by specs/001-go-runtime-conversion.md; no legacy files retire
in this slice.

Unsupported-attribute safety refinement, before implementation: an affected
integer, indexed/associative array, nameref, or other non-scalar variable must
cause explicit candidate refusal before any parent-shell action. Bash can
interpret arithmetic or redirect assignments for those attributes; leaving them
uncharacterized is not permission to execute configuration data. Inspect only
local declare -p attribute metadata, without sending caller values to Go. Allow
ordinary scalar readonly/export flags; validate every affected SET and EXPORT
target before applying the first record. This is a candidate admission boundary,
not an approved correction of public legacy behavior or permission to cut over.

The factory-only Go source gate must require the new config.sh candidate and
run Bash syntax validation; Template CI must include it in ShellCheck. Isolated
negative controls must reject a missing or syntactically invalid shim. These
source checks retain the adopter skip and introduce no adopter Go requirement.

Provenance:
- docs/DECISION_LOG.md:1952 — private candidate boundary and deferred export.
- scripts/lib/config.sh:87 — current fixed settings, including both additions.
- scripts/lib/config.sh:93 — legacy parsing, preserved keys and ordered effects.
- scripts/lib/config.sh:191 — full-export caller snapshot and YAML overlay.
- Observed 2026-09-07 via isolated Bash subprocesses: missing standalone legacy
  returns 1; readonly standalone assignment reports an error and continues in
  an ordinary shell; readonly full-export caller remains unchanged without error.

Legacy NUL input characterization: Bash 3.2 read truncates a record at its first
NUL; modern Bash read removes NUL bytes. The candidate currently removes NUL,
matching the modern behavior but not Bash 3.2. NUL-bearing legacy input is
excluded from supported flat-file parity until a cutover decision resolves this
difference; this behavior is not qualified for activation. Separate acceptance
characterizes the selected Bash and asserts the candidate's independent expected
normalization without treating their divergence as an approved correction.
Observed 2026-09-07 via /bin/bash and /opt/homebrew/bin/bash read subprocesses
using MODEL_PRO<NUL>VIDER=left<NUL>right records. Existing YAML reader NUL
semantics remain a separate, unchanged contract.
