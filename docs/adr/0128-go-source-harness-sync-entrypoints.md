# ADR 0128: native source harness synchronization entrypoints

Status: accepted bounded R3 function; independent semantic RED precedes conversion

Date: 2026-11-01 UTC

## Scope

After PR #158 merged as `32f956d625f2ad016d7f5d26c2051ebc6ea9be48`,
replace the active bodies of `scripts/sync-opencode.sh`,
`scripts/sync-claude.sh` and `scripts/sync-codex.sh` with adapters to the existing
Go synchronization owner. Reuse ADR 0099's rendering, configuration, publication
and installed-adapter contracts. This closes the direct source sync boundary;
other command/helper bodies and historical proof controllers remain separate R3
work. No dependency, version, model, timeout or public release default changes.

## Direct invocation compatibility

Each script resolves its physical owner with the established source adapter
pattern, rejects a file-symlink invocation and execs that owner's `factory` with
exactly `sync` and its fixed harness. Preserve caller cwd, stdin, environment,
stdout, stderr and exit status. Preserve literal physical paths, including
trailing newlines, and do not select code through PATH or environment overrides.
Missing or failing owners must fail without a legacy fallback.

These historical direct scripts ignored every command-line operand. Continue
discarding their operands, including `--help`, rather than forwarding them to the
new strict selector parser. The public `factory sync` parser remains unchanged.
This distinction is already part of docs/adr/0099-native-factory-orchestration.md:27.

Output belongs to the source/installed owner, while configuration retains the
existing caller Git-root and explicit FACTORY_CONFIG selection. Reuse the Go
owner's literal model handling, role boundaries, render-before-publish behavior,
safe destination checks and per-file publication. Direct scripts invoke no model.
Installed recipes retain their existing generated compiler-free adapters.

Inspect deterministic output in a private source copy before refreshing any
tracked generated harness artifacts. Only the existing Go owner's required
rendering changes belong in such a refresh; canonical policies and model choices
remain the inputs. Qualification must never run the primary source launcher,
execute or replace the operator-owned factory-go, or overwrite operator edits.

## Historical consumers and coverage

Audit every fixture that materializes the old dispatcher or initializer, not
only direct sync tests. Historical shell synchronization assertions must retain
the actual three sync bodies from immutable revision
`1754de23de95a7e3c734bedd86894295e823c988`. Carry them into private fixtures before
source Git custody is dropped. Preserve current policy-gate subjects, all
historical assertions, and independent frozen/native Go oracles. Saved scripts
are test inputs and never a production fallback.

The full historical runner, optional-pack runner and historical initializer
fixtures must not pair a legacy dispatcher with a current native adapter. Their
exact consumer audit determines the required materializer updates. No test may
claim current native synchronization merely by exercising a historical body.

An independent test author must observe the current scripts fail new native
delegation criteria before conversion. Cover actual native rendering for all
three harnesses, owner versus caller configuration selection, ignored legacy
operands, literal transport, directory aliases, symlink refusal and failed-owner
propagation. Use private fixtures and preserve caller files. Reuse existing
native sync conformance/publication tests rather than duplicating those engines.
Actual native discovery must retain every existing criterion identity and place
new source criteria in the existing source-entrypoints partition.

## Qualification

Run focused current-source acceptance, existing native sync and affected health
or initialization criteria, the complete historical shell suite and affected
historical consumer suites. Exercise generation/drift in a clean private copy.
Use the review diamond and require hosted checks and review disposition before
merging this bounded PR or beginning another R3 function boundary.

The initial private current-Go rendering audit identified only object-key order
changes in `.claude/settings.json` and `.mcp.json`; parsed JSON values are equal.
Refresh those two generated outputs from that owner and prove repeated direct
sync leaves them stable. The concrete historical closure is the full shell
runner's sync sandbox plus the optional-pack, Java, budget and loop initializer
materializers. Existing Go initializer/doctor/installation oracles already archive
their original sync bodies and require no blanket fixture rewrite.

Current generated-file drift belongs to `make check-drift` (Makefile:194), also
dispatched by scripts/pre-push-check.sh:49 and the changed-adapter rule in
scripts/hooks/diff-aware-check.sh:104. Run it in a private committed source copy;
it executes all three current direct adapters before comparing generated files.
The historical shell block has always rendered into its own sandbox. Current
native no-settings byte preservation is covered independently by
acceptance/orchestration_sync_test.go:289, while the new direct-entrypoint
criteria prove owner/caller separation and native rendering for each harness.

The native rendering criterion must execute with its exact sanitized fixture
environment. A process helper that merges absent keys back from the parent can
reintroduce host model settings and invalidate YAML-selection evidence. Preserve
the existing bounded process watchdog and capture/status behavior while passing
the selected environment exactly; do not replace omitted model keys with empty
overrides or mutate the parent environment of parallel criteria. Observe this
fixture defect with exported host model settings before correcting the runner.
