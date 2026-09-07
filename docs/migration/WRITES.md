# Go configuration-write candidate

This G1 slice adds `factory_config_set` under
[ADR-0063](../adr/0063-go-configuration-writes.md). It changes the explicit source
candidate; installed factory scripts keep their current routing until the
migration's compatibility, delivery and recovery requirements pass.

## Local use

Build the Go candidate in the factory source checkout, then source the Bash
configuration adapter:

```sh
go build -o factory-go ./cmd/factory
FACTORY_RUNTIME_BINARY="$PWD/factory-go"
. runtime/shell/config.sh
factory_config_set review_lane off
```

This is a write operation on the file selected by `FACTORY_CONFIG` or the existing
Git-root/default path rules. It requires an existing, caller-owned regular file
with one hard link and ordinary permissions. Use only a quiescent file in a trusted, quiescent parent directory without
ACLs or extended attributes; those metadata and concurrency cases are not yet
qualified for installed cutover. Missing configuration is not created.

The private Cobra request is `config set KEY VALUE` under
`FACTORY_BRIDGE_PROTOCOL=1`. Keys use the reader's literal identifier contract;
values remain literal data, including flag-looking strings. The shell helper
ignores surplus arguments and retains its required-operand behavior under
nounset. There is no runtime search, download, compilation or legacy fallback
when the explicitly selected runtime is unavailable. Sourcing the adapter alone
does not call it.

## File behavior

Go replaces every matching physical key line or appends the setting when absent.
Unrelated bytes and ordinary permissions are preserved. This deliberately retains
the legacy format, including its quirks:

- Matching lines lose their old trailing comment and CR; unrelated CRLF stays.
- Replacement folds value LF characters into spaces; append keeps them literal.
- Appending to an unterminated file does not first add a newline.
- Embedded quotes are written literally, not escaped as general YAML.

Consequently, arbitrary quote/newline values are not guaranteed to round-trip
through the existing flat reader. This conversion does not silently repair the
format or change its parsing rules.

The writer prepares an exclusive temporary file beside the configuration,
synchronizes it and rechecks the original before publication. Before-publication
failures leave the original unchanged and clean the call's temporary file.
Existing `.factory-bak` siblings are preserved. Input and output are each bounded
to 16 MiB. Unsafe links and unsupported file modes are refused.

This is not the installation upgrade transaction: it neither retires legacy
scripts nor advances backup retention. Mixed-runtime writer exclusion, extended
metadata parity, safe activation, ignored local recovery copies and predecessor
cleanup remain required migration work.

## Acceptance

Independent Ginkgo/Gomega scenarios exercise the compiled CLI and sourced Bash
helper against explicit outputs and both immutable reader baselines. They cover
literal byte rewriting, mode preservation, source/caller behavior and no-mutation
refusals. Recorded RED/GREEN and quality results follow below.

## Recorded development evidence (2026-09-07)

Before implementation, independent acceptance against the compiled CLI failed
at the missing helper and private request:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus='G1 candidate configuration writes' -ginkgo.no-color -ginkgo.succinct -count=1
```

```text
Ran 54 of 560 Specs in 5.842 seconds
FAIL! -- 8 Passed | 46 Failed | 0 Pending | 506 Skipped
FAIL github.com/anoop2811/software-factory-template/acceptance 6.243s
```

The eight passing malformed-request cases exercised the already-existing bridge
refusal, not a completed writer. Baseline byte expectations were checked before
the corresponding candidate invocation. Later supplemental diagnostic/metadata
cases are distinguished from this initial RED record.

The final setter suite contains 64 cases. Review regressions for lexical `/..`,
trailing path LF and a replaced temporary occupant each failed before their
corrections. The occupant test is a deterministic in-process collaborator test,
not a claim about an adversarial concurrent directory. The same focused command
on the final source returned:

```text
ok  github.com/anoop2811/software-factory-template/acceptance 14.002s
```

Two additional packaged-conformance cases reject an immutable pre-writer binary
from `b6eab79cb1360e10baf6326c6fbbf2c6eee7f846`. The observed local fixture command:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_BUNDLE_BINARY=/var/folders/83/yj7qqyt551xbbpvm54tqkcbw0000gn/T/factory-write-baseline-0xwa0zdb/factory go test ./acceptance -ginkgo.focus='Packaged runtime conformance.*(writes literal bytes|refuses writes through a symlink)' -ginkgo.no-color -ginkgo.succinct -count=1
```

```text
Ran 2 of 572 Specs in 0.637 seconds
FAIL! -- 0 Passed | 2 Failed | 0 Pending | 570 Skipped
FAIL github.com/anoop2811/software-factory-template/acceptance 0.994s
```

With the current locally built binary, all five packaged checks passed:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_BUNDLE_BINARY=/var/folders/83/yj7qqyt551xbbpvm54tqkcbw0000gn/T/factory-write-current-512_849s/factory go test -v ./acceptance -ginkgo.focus='Packaged runtime conformance' -ginkgo.no-color -ginkgo.succinct -count=1
```

```text
PASS
ok  github.com/anoop2811/software-factory-template/acceptance 0.967s
```

These paths record ephemeral local executable fixtures, not signed releases.
The native bundle CI jobs supply the actual staged artifacts to this same gate.

An initial full-suite run also caught an obsolete reader fixture that called
`config set` an unknown operation. Its token now names `unknown-operation`;
its status, stdout and no-mutation assertions remain unchanged.

On the final source/test snapshot, the complete source gate exited 0:

```sh
rtk proxy env PATH="/private/tmp/factory-quality-tools:$PATH" make go-runtime-source-check
```

```text
ok  github.com/anoop2811/software-factory-template/acceptance 174.965s
0 issues.
Issues : 0
No vulnerabilities found.
```

`rtk proxy env FACTORY_AGENT_ROLE=reviewer shellcheck -s bash -S warning runtime/shell/config.sh`
exited 0 with no output. The configured diff-aware gate also exited 0:

```sh
rtk proxy env FACTORY_AGENT_ROLE=reviewer ./scripts/hooks/diff-aware-check.sh
```

```text
selftest: 217 passed, 0 failed, 0 skipped
diff-aware-check: all 1 dispatched check(s) passed
```

Independent correctness/tests and security reviews were rechecked after their
findings were addressed. Live paid-model execution, installed activation and
upgrade backup/cleanup acceptance are not claimed by this evidence.
