# Durable local recovery creation

[ADR-0091](../adr/0091-durable-local-recovery-creation.md) defines the source-built
Go backup writer. It copies an explicitly confirmed selection from the six-path
legacy budget/loop reference catalog into `.factory/backups/MIGRATION_ID/`.
Installed activation, complete historical-factory coverage, controlled restoration,
script retirement and later-release pruning remain pending. The installed shell
dispatcher does not invoke this operation.

The [read-only restoration plan](RECOVERY_RESTORATION_PLAN.md) compares one
checked set with its current installed paths; it does not grant restoration
authority.

## Create a checked local set

Build the candidate beside the template assets, then invoke it from the physical
installation root. Obtain the adoption proposal with the existing read-only
preview, selecting each unchanged known path to preserve:

```sh
go build -o factory-go ./cmd/factory
/path/to/factory-go upgrade --dry-run --source /physical/local/target \
  --adopt-path scripts/factory-budget.sh --json
```

The preview's status 2 is expected: applying migration remains blocked. Inspect
the proposal and use its `adoption_proposal.proposal_digest` as explicit consent.
The proposal is bound to this root, selection and current file identities. An
environment variable, copied manifest or matching filename cannot replace it.

```sh
/path/to/factory-go upgrade --create-backup --migration-id migration-001 \
  --target-revision TARGET_SOURCE_COMMIT \
  --adopt-path scripts/factory-budget.sh \
  --confirm-adoption REVIEWED_PROPOSAL_DIGEST --json
```

Replace both placeholders with the reviewed values. The target revision must be
40 lowercase hexadecimal characters distinct from the compiled reference. It is
operator-supplied metadata; creation does not authenticate or execute that target.
Repeat `--adopt-path` for the exact proposed selection. The other options must
occur once; creation cannot be combined with preview, source, ref or inspection
options. It performs no paid calls.

The six known paths are `scripts/factory-budget.sh`, `scripts/factory-loop.sh`,
`scripts/lib/budget-config.sh`, `scripts/lib/budget.py`,
`scripts/lib/budget_adapters.py` and `scripts/lib/loop.py`. A selected absent or
customized file blocks creation; the writer does not infer ownership or expand
its scope to the entire old factory. Older releases may not contain this catalog.

## What it writes and preserves

Before saving payloads, the writer checks the Git index and effective ignoring.
It establishes the exact `/.factory/backups/` rule in local `.git/info/exclude`,
preserving project ignore files and existing local patterns. A broader local or
project ignore rule does not replace this durable local fallback. Already tracked
recovery paths block creation; nothing is untracked. Higher-precedence rules that
expose recovery entries block the operation rather than being rewritten.
The added line is staged as an inert comment, synced and checked, then activated
with a single-byte change. Interrupted staging cannot turn a partial pattern into
an exclusion of unrelated files. Incomplete comments may remain as reported local
evidence; they do not authorize copying or count as an effective ignore rule.
Every successful creation or reuse syncs and reads back the validated local
exclude file and syncs `.git/info`, including retries after activation or parent
sync failures. A visible rule is not evidence that a previous sync completed.

This initial implementation requires a normal non-bare Git repository rooted at
the invocation directory, with physical `.git` and `.git/info` directories.
Linked worktrees, gitfile submodules and external Git directories refuse safely.
These installation forms must gain independent qualification before cutover.
Directory and file safety checks reject links, unsafe ownership or permissions,
multi-link files and special file types. Use a trusted quiescent installation;
the writer does not claim a global atomic snapshot against concurrent user edits.

Each saved copy under `files/<original relative path>` has mode 0600; recovery
directories have mode 0700. Originals retain their bytes and active modes. The
strict manifest records original paths, digests, byte counts, modes and source
and declared target revisions. Files and directories are synced and read back;
`manifest.json` is written last, after final consent and Git checks. A valid
manifest and inert payloads must also pass the existing integrity inspector.
Runtime history, configuration, user code and Git index remain untouched.

Status 0 reports `created` or `already_present`. Exact, unheld, intact same-ID
sets can be rechecked and reused without another copy or duplicate ignore rule.
Unknown, held, edited, unsafe, differently targeted/selected or incomplete sets
are preserved and refuse. The command does not choose new retry IDs or remove
partial sets automatically. A failure after writes may leave local evidence;
inspect that evidence before retrying. A sync failure after manifest publication
is still an error, not a successful migration.

The output keeps `coverage=partial`, `target_authentication=operator_metadata`,
`restorable=false`, `activation_ready=false` and `prune_authorized=false`.
`created` establishes this operation's checked backup creation, not a completed
upgrade or authority to restore/delete imported local data. No backup is active
fallback code and creation never advances release retention.

## Inspect and next steps

Use the existing preview's `--inspect-backups` option to inspect local evidence;
see [recovery inspection](RECOVERY_INSPECTION.md). Factory-owned discovery
exclusions are described in [recovery discovery](RECOVERY_DISCOVERY.md).
Arbitrary user commands and external scanners still require their own exclusion
qualification. Controlled restoration, transactional activation, full inventory
coverage and retention remain separate requirements in the approved conversion
specification.

## Local qualification

The following focused runs record local creation qualification. PR #118's later
review corrections have their own independent RED evidence and platform checks,
recorded separately below.

RAN 2026-10-02:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -v ./acceptance -ginkgo.focus='Durable local recovery creation' -ginkgo.no-color -ginkgo.succinct
Go factory command acceptance - 79/2239 specs
SUCCESS! 57.254761417s
ok github.com/anoop2811/software-factory-template/acceptance 58.817s
```

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -v ./internal/assessment ./internal/recoverycmd -ginkgo.focus='Durable recovery creation|Recovery creation public' -ginkgo.no-color -ginkgo.succinct
Installation assessment collaborators - 43/98 specs
SUCCESS! 6.5387385s
ok github.com/anoop2811/software-factory-template/internal/assessment 8.363s
Recovery creation public - 13/13 specs
SUCCESS! 2.22590725s
ok github.com/anoop2811/software-factory-template/internal/recoverycmd 3.695s
```

Those selected creation cases have no skips. Three existing inspection fixtures
separately report conditional skips because this sandbox strips native setuid and
setgid bits before inspection and rejects invalid UTF-8 directory names. Portable
special-mode and raw-name display controls still execute; the skipped native
shapes require supporting-platform qualification. Broader source checks and
hosted Linux/macOS qualification are tracked separately and are not implied by
these focused results.

The broader source gate was RAN on the same date with the existing CI timeout
override and qualified tool paths:

```text
rtk proxy env FACTORY_AGENT_ROLE=reviewer GOCACHE=/private/tmp/factory-durable-recovery-go-cache GOLANGCI_LINT_CACHE=/private/tmp/factory-durable-recovery-lint-cache bash -c 'export PATH=/private/tmp/factory-quality-tools:/var/folders/83/yj7qqyt551xbbpvm54tqkcbw0000gn/T/factory-pr113-python-_gp80gpr/venv/bin:$PATH; make go-runtime-source-check GO_RUNTIME_TEST_TIMEOUT=20m > /private/tmp/factory-durable-recovery-source-check.log 2>&1; check_status=$?; tail -22 /private/tmp/factory-durable-recovery-source-check.log; exit "$check_status"'
ginkgo-only-check: all Go behavioral tests use Ginkgo/Gomega
go vet ./...
go test -race -count=1 -timeout=20m ./...
panic: test timed out after 20m0s
FAIL github.com/anoop2811/software-factory-template/acceptance 1200.503s
make: *** [go-runtime-source-check] Error 1
```

This was not a passing full gate. Before the timeout, the log recorded 63
loopback-bind denials in HTTP fixtures, five process-inspection denials, two
invalid-UTF-8 filename creation denials and eight native special-mode fixture
assertions. This sandbox separately demonstrated that it strips the requested
native setuid/setgid bits. The timeout stack was in the existing native harness
role-configuration test, not a creation test. No completed acceptance-suite
totals are claimed. All internal packages with tests reported `ok`; hosted
supporting-platform qualification remains required. Existing HTTP, process and
mode assertions were not disabled to manufacture a passing source gate.

After that gate stopped, the remaining quality operations were run independently:

```text
rtk proxy env FACTORY_AGENT_ROLE=reviewer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go build -o /private/tmp/factory-durable-recovery-delivery.WnyqbP/factory ./cmd/factory
exit: 0
rtk proxy env FACTORY_AGENT_ROLE=reviewer GOCACHE=/private/tmp/factory-durable-recovery-go-cache GOLANGCI_LINT_CACHE=/private/tmp/factory-durable-recovery-lint-cache /private/tmp/factory-quality-tools/golangci-lint run --config packs/go/.golangci.yml ./...
0 issues.
rtk proxy env FACTORY_AGENT_ROLE=reviewer GOCACHE=/private/tmp/factory-durable-recovery-go-cache /private/tmp/factory-quality-tools/gosec ./...
Files : 129
Lines : 20476
Nosec : 17
Issues : 0
rtk proxy env FACTORY_AGENT_ROLE=reviewer GOCACHE=/private/tmp/factory-durable-recovery-go-cache /private/tmp/factory-quality-tools/govulncheck ./...
govulncheck: fetching vulnerabilities: Get "https://vuln.go.dev/index/modules.json.gz": dial tcp: lookup vuln.go.dev: no such host
```

That earlier fresh vulnerability scan was incomplete. A restricted module-cache write also
produced a Go stat-cache warning; the observed build still exited 0. Neither a
cached scan nor focused acceptance results substitute for the pending checks.

## PR #118 review qualification

The initial hosted macOS source gate passed for head
`1cf266f1029966a00a55dc821f7d03f9c36319ad` in workflow run `37049997627`:
its acceptance package reported `ok` at 962.155s, pack lint reported `0 issues.`,
gosec reported `Issues : 0`, and govulncheck reported `No vulnerabilities found.`
The same run's Linux source gate failed with eight acceptance, 40 assessment and
seven command failures, including `unsafe local Git exclude file` before the
intended recovery/output behavior. These are recorded results, not a claim that
the initial head passed both platforms.

The positive fixture helpers now explicitly chmod and read back their intended
Git control directory and file modes. `os.WriteFile` with a creation mode does
not change a pre-existing Git-created file's mode. The Linux log does not identify
the precise rejected bits; local controlled-umask reproduction separately
established that inherited group writing can invalidate the fixture. Production
ownership, link and writable-by-others checks remain strict.

RAN before production correction, 2026-10-02:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -v ./internal/assessment -ginkgo.focus='re-establishes unchanged local exclusion durability' -ginkgo.no-color -ginkgo.succinct
Ran 9 of 107 Specs
0 Passed | 9 Failed
```

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -v ./acceptance -ginkgo.focus='preserves detached legacy option values|establishes the canonical local exclusion|explicit digest conflict|exclude group writing survives overwrite' -ginkgo.no-color -ginkgo.succinct
7 selected: 2 Passed | 5 Failed
```

The second run reproduced three missing canonical-local-rule cases and two
detached legacy operand dispatch cases. Its explicit-confirmation/environment
conflict and group-writable existing-template refusal were already passing
coverage additions. The fixture qualification separately passed under inherited
umask `0002`: two existing assessment cases and all 13 command cases.

After network access was restored, the fresh vulnerability operation was RAN:

```text
rtk proxy env FACTORY_AGENT_ROLE=reviewer GOCACHE=/private/tmp/factory-durable-recovery-go-cache /private/tmp/factory-quality-tools/govulncheck ./...
No vulnerabilities found.
```

The corrected source was RAN with child-CLI and outer race instrumentation:

```text
rtk proxy env FACTORY_AGENT_ROLE=reviewer FACTORY_CLI_TEST_RACE=1 GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -v ./acceptance -ginkgo.focus='Durable local recovery creation' -ginkgo.no-color -ginkgo.succinct > /private/tmp/factory-pr118-recovery-acceptance.log 2>&1
Go factory command acceptance - 85/2245 specs
SUCCESS! 59.689387417s
--- PASS: TestAcceptance (59.83s)
ok github.com/anoop2811/software-factory-template/acceptance 61.772s
```

```text
rtk proxy env FACTORY_AGENT_ROLE=reviewer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -v ./internal/assessment ./internal/recoverycmd -ginkgo.focus='Durable recovery creation|Recovery creation public' -ginkgo.no-color -ginkgo.succinct > /private/tmp/factory-pr118-recovery-internal.log 2>&1
Installation assessment collaborators - 52/107 specs
SUCCESS! 9.164341625s
ok github.com/anoop2811/software-factory-template/internal/assessment 10.496s
Durable recovery command boundaries - 13/13 specs
SUCCESS! 3.36753275s
ok github.com/anoop2811/software-factory-template/internal/recoverycmd 4.964s
```

All 150 selected cases passed. Independent final correctness/test-quality and
security/durability reviews reported no surviving findings. Full pack lint
reported `0 issues.`; build and vet exited 0; gosec reported 129 files, 20482
lines, 17 suppressions and `Issues : 0`; the fresh vulnerability scan again
reported `No vulnerabilities found.` on the corrected source. Hosted Linux/macOS
qualification remains separate and required before merge. The full installed
migration lifecycle remains pending.
