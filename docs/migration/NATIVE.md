# Go native harness execution candidate

[ADR-0068](../adr/0068-go-native-harness-execution.md) defines the next G2
component: invocation preparation, local capability checks, process supervision
and assistant response selection for Codex, Claude Code and OpenCode.

## Integration boundary

The candidate lives in `internal/native`; response selection shares the bounded
parser in `internal/usage`. There is no new CLI launch command. The existing
Python budget and loop controllers remain the installed execution path.

A future Go controller must check the budget, preflight locally, acquire the
shared lock, recheck admission and reserve spending before calling Execute.
Its ownership callback must durably publish the spawned PID before prompt
delivery. Only completed execution with complete accounting can publish answer
text. Unknown process ownership must retain the reservation for recovery.

Prepare builds literal argv, stdin and explicit environment overrides. Preflight
checks help output and Codex implementer hook trust without a model request.
Execute supervises a dedicated process group with bounded capture, context and
allowance deadlines, and separate leader-exit and ownership information.
Response selection returns private text to its caller; it does not authorize
display or write transcripts to history.

The qualified input domain and explicit safety tightenings are in the ADR.
In particular, nonpositive execution allowance refuses launch, and help probes
have bounded output and descendant cleanup. Those tightenings are intentional
differences from the immutable Python baseline.

## Qualification and remaining work

Development uses independently compiled test drivers and fake native programs.
These drivers are test fixtures, excluded from release and installed command
discovery. Local process tests do not establish compatibility with an installed
native CLI version or a paid provider invocation.

Budget admission, mixed-runtime locks and fingerprints, durable history,
loop recovery and installed activation remain separate work. No legacy file
is retired by this component. Activation must include ownership-safe local
gitignored recovery copies, rollback, predecessor cleanup and the required
manual adopter pilot.

## Development evidence

The independent test driver first compiled against minimal API stubs, then
failed on six behavior assertions before substantive implementation. The focused
command was:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer go test ./acceptance -ginkgo.focus='G2 native harness component' -ginkgo.no-color -ginkgo.succinct -count=1
```

The captured behavioral RED result was:

```text
Ran 6 of 839 Specs in 1.946 seconds
FAIL! -- 0 Passed | 6 Failed | 0 Pending | 833 Skipped
```

The earlier missing-package compilation failure is not the behavioral RED
claim. Supplemental regression tests independently exposed Unicode help-word
boundaries, relative PATH selection, symlink/parent traversal, the baseline
integer digit limit, discarded late parser errors and incomplete drainage.
The corrected implementation preserves the qualified baseline and refuses
uncertain cleanup.

The final 80 outside-in cases passed with the test driver and fake children
themselves race-instrumented, in addition to the test process:

```sh
rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_NATIVE_TEST_RACE=1 go test -race ./acceptance -ginkgo.focus='G2 native harness' -ginkgo.no-color -ginkgo.succinct -count=1
```

```text
ok  	github.com/anoop2811/software-factory-template/acceptance	66.633s
```

Four internal collaborator cases exercise uncertain signal/reap reports, a
parser error after leader exit, and an expired drain deadline. The final root
verification command was:

```sh
rtk proxy env FACTORY_AGENT_ROLE=reviewer go test -race ./internal/native -count=1 -v
```

```text
Ran 4 of 4 Specs in 0.049 seconds
SUCCESS! -- 4 Passed | 0 Failed | 0 Pending | 0 Skipped
ok  	github.com/anoop2811/software-factory-template/internal/native	1.414s
```

The collaborator faults use real children with independent exit checks and
test-owned cleanup. No production environment switches or global fault hooks
are introduced. Independent review found no remaining actionable issue after
the late-error and drainage corrections.

The full source gate exited 0:

```sh
rtk proxy env FACTORY_AGENT_ROLE=reviewer PATH="/private/tmp/factory-quality-tools:$PATH" make go-runtime-source-check
```

```text
ok  	github.com/anoop2811/software-factory-template/acceptance	385.988s
ok  	github.com/anoop2811/software-factory-template/internal/native	1.777s
0 issues.
Issues : 0
No vulnerabilities found.
```

The Linux-target lint and diff-aware gates also exited 0:

```sh
rtk proxy env GOOS=linux GOARCH=amd64 /private/tmp/factory-quality-tools/golangci-lint run --config packs/go/.golangci.yml ./...
rtk proxy env FACTORY_AGENT_ROLE=reviewer ./scripts/hooks/diff-aware-check.sh
```

```text
0 issues.
selftest: 217 passed, 0 failed, 0 skipped
diff-aware-check: all 1 dispatched check(s) passed
```

These results qualify the source component and fake-process behavior. Installed
activation, real native CLI versions, release artifacts, mixed-runtime controller
integration and cleanup of predecessor installations remain unqualified here.
