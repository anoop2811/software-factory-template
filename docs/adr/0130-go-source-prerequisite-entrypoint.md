# ADR 0130: native source prerequisite entrypoint

Status: accepted bounded R3 function; independent semantic RED precedes conversion

Date: 2026-11-02 UTC

## Scope

After PR #160 merged as `304a8f3c46e909048e2f81e079d710e22cf6d4fe`,
replace `scripts/prereq-check.sh` with an adapter to the existing Go prerequisite
owner. The existing `make prereq-check` target follows that path. Reuse the tool
contract in docs/adr/0099-native-factory-orchestration.md:90, including its
approved removal of the Python requirement. No new command, dependency, tool
version, timeout, model or public release default is introduced.

## Direct invocation compatibility

Use the established physical-owner source adapter. Refuse final-file symlinks
and exec the owning `factory prereq`, without PATH or environment runtime
selection. Preserve literal operands, original cwd, stdin, environment, output
streams and exit status, including owner paths ending in newlines. Missing and
failed owners fail without a legacy fallback. Forward operands exactly; the
native command already ignores them, including `--help`.

Configuration and probes belong to the caller. An explicit relative
`FACTORY_CONFIG` resolves against the original working directory. Without that
override, configuration comes from the caller's Git root with cwd fallback.
This deliberately differs from doctor's project-root-relative explicit override.
Tool lookup, including relative PATH entries, and version probes retain the
original cwd. The harness comes from `FACTORY_HARNESS` (default `opencode`),
and the product Go minimum comes from the selected configuration file.

Reuse native first-line version presentation, numeric Go comparison, harness
spelling and aggregated failures. Selected PATH tools are trusted executables;
this check is not a sandbox. Probe only the existing version operands. Ordinary
nonzero version exits retain their existing presence semantics. Python is no
longer a prerequisite; jq remains required for retained JSON validation gates.
The source launcher still needs a Go compiler to build the factory itself.
An installed runtime needs no factory compiler and skips the product Go check
when no product minimum is configured. Historical proof controllers have their
own dependencies until their separate conversion.

## Historical consumers

Add the original prerequisite script from immutable revision
`1754de23de95a7e3c734bedd86894295e823c988` to the full historical shell runner and
optional-pack wrapper materializers. Their frozen dispatcher and copied Makefile
must not accidentally enter the current native adapter. Neither current suite
invokes this target; this is fixture-coherence prevention, not a reproduced
failure. Preserve every historical assertion and current policy-gate subject.
Java, budget and loop fixtures already stub this boundary; retain those stubs.
Existing native orchestration and initialization archives are already frozen.
Historical bytes remain test data, never a production fallback.

## Qualification

An independent author must observe semantic RED on the old direct script before
replacement. Exercise one real private source launch, caller/default config
selection, selected harness, no-Python behavior and native status. Use the
compiled owner for further caller-relative configuration and failure checks.
Reuse existing numeric-version, missing-tool and installed-runtime acceptance.

Use exact child environments and private version-tool witnesses; do not invoke
live harnesses or network services. Keep real Go available for source compilation.
Record unexpected tool execution, since an ordinary nonzero version exit alone
does not prove a tool was unused. Cover literal transport, physical/directory
aliases, final-file symlink refusal, runtime decoys and missing/failed owners.
Discover the actual new census and retain every existing criterion name in its
proper partition. Run affected native and historical suites, quality gates and
the review diamond. All source build/generator dispatch runs in private copies,
with primary operator files and developer binary preserved. Hosted checks and
review disposition precede merge and the next R3 function boundary.

## Retained source evaluation prerequisite

The direct `scripts/golden-task-eval.sh` path and `make golden-eval` still use the
retained Python scorer until their own conversion. Native `factory eval golden`
and installed adapters already use Go. Removing Python from native prerequisite
checks must not let that retained path spend runner work before discovering its
own missing dependency.

After existing help, empty-task and runner validation, require `python3` presence
in the retained script before creating scratch files or invoking a runner.
Report its legacy-only requirement and the native alternative, then exit 1 when
absent. Preserve help and empty-task success, current validation precedence and
the Python-free native commands. This guard introduces no installation or model
authority and does not claim to convert the retained evaluator.

Independent outside-in acceptance must first observe the old script invoke an
owned runner before failing for missing Python. Then require the clear early
failure, no runner invocation and no published result, alongside unchanged help
and empty-task outcomes. Keep every existing criterion and discover the new census.
