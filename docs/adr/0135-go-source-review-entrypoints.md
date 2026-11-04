# ADR 0135: native source review execution and lane administration

Status: accepted design; implementation and qualification evidence are separate

Date: 2026-11-04 UTC

## Scope

Complete the grouped R3 review boundary: `scripts/adversarial-review.sh` calls
the existing `factory review` owner and `scripts/factory-review-lane.sh` calls
`factory review-lane`. Reuse the contracts in
docs/adr/0099-native-factory-orchestration.md:120 and
docs/adr/0089-go-native-review-lane.md:17. No new provider, model, dependency,
automatic paid call or release default is selected.

## Source invocation

Both paths use the established physical-owner Bash adapter. Resolve the invoked
source checkout independently of caller cwd and preserve literal operands,
stdin, environment, stdout, stderr and exit status through `exec`. Preserve
directory aliases and trailing-newline owner paths; reject final-file symlinks.
PATH or runtime overrides must not select another owner. Missing or failed
owners refuse explicitly without executing a saved implementation.

Source execution retains its existing compiler requirement. Installed adapters
already select the packaged binary and remain compiler-free. The source wrapper
must not reimplement configuration, HTTP transport, workflow publication or
model selection. Native fixed-endpoint review preserves bounded private requests,
provider-specific rendering and explicit-client failure without fallback.

Lane administration remains opt-in. Enable, status, pending, secret-name and
disable retain caller-project discovery, configured secret/model handling,
ownership checks and the native mutation contract. Enabling a lane neither
uploads secrets nor runs a model. Unsupported commands and unsafe workflow
ownership must refuse without silently altering unrelated user files.

## Consumers and historical custody

The repository review workflow already uses native `review-batch github`.
The shipped review-lane template also supports a direct-script compatibility
step. Qualify that actual step against the new review adapter, with private
GitHub/HTTP substitutes and no paid requests. Preserve its trusted-base and
credential boundaries and the configured-client opt-in; this conversion does
not change workflow timing or batching defaults.

Historical review request assertions still describe curl/jq and selected-client
mechanics. Retain them as explicit development-only evidence against revision
`f3a0b6520917e104883cc89b123d7244fa0c46a7`, including its matching config reader.
That revision has the same review, lane and config bodies as the pre-cutover
tree. Preserve existing separately pinned native differential oracles. General
and optional-pack historical controllers must receive coherent frozen lane
and review dependencies wherever they previously copied the current bodies.
Do not freeze the actual current policy gates or silently skip old assertions.

Record R01-R53 custody in the proof-retirement ledger after its complete
fixtures execute. Retaining these historical assertions does not assert that
every old transport mechanism is a native requirement. New source-boundary
coverage and existing native review acceptance establish the current runtime.
No historical fixture is an installed asset or production fallback.

## Qualification

An independent test author first observes the old source bodies fail native
owner-routing tests. Use real processes, literal transport witnesses, missing
owner refusals and at least one actual private source compilation. Exercise
native local-HTTP request/response and refusal behavior, lane lifecycle and
workflow ownership, and the shipped direct review step. Reuse existing native
criteria for the detailed transport, cancellation and publication matrices.

Keep unrelated operator files and the primary checkout's ignored `factory-go`
untouched. Run source acceptance with race, affected native/historical suites,
configured quality gates, independent review and hosted checks. Record actual
commands/results separately. The two rows close only after qualification,
review and merge; later grouped R3 work starts after this PR is merged.

## Cold source workflow preparation

The generated compatibility workflow must prepare a canonical source checkout
before calling its newly native adapter. With no explicit review client,
classify source only from the factory module identity and expected source
bootstrap inputs. Set up Go from that checkout's go.mod, acquire its locked
modules, reject go.mod/go.sum changes, and exercise the actual source launcher
with help before the credential-bearing review step. The later single-request
call remains the same public adapter; source launcher's offline-build admission
is unchanged. No model is called during preparation.

Explicit client selection bypasses this preparation. Ordinary adopter modules
must never be built or have their dependency files read as factory runtime
inputs. Installed projects still use their provisioned native runtime, without
a compiler. Existing trusted-base checkout and credential scopes remain intact;
this correction does not silently opt a single-request user into batch costs.
Test cold module acquisition through a local immutable module proxy and actual
source execution, not only a compiled recording owner. Missing preparation must
fail before the private provider is contacted.

Source qualification explicitly provisions locked modules with `go mod download`
before constructing the local proxy; a platform-specific test build alone is
not a complete module download. The fixture uses the qualified Go tool to stage
the locked graph through a file-only proxy, checks both lockfiles unchanged,
and then exercises the actual workflow with a separate empty consumer cache.
Neither fixture preparation nor behavioral acquisition may bypass the local
proxy through private-module settings or contact a public module service.
