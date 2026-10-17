# Native factory orchestration

The Go runtime owns harness synchronization, prerequisite discovery, the eight
pre-push stages, structural and golden evaluation, review requests and shared
helpers. The contract is [ADR 0099](../adr/0099-native-factory-orchestration.md).
This completes the orchestration source workstream; default packaged bootstrap
and replacement-release qualification remain separate gates.

| Supported invocation | Canonical Go implementation | Retained boundary |
|---|---|---|
| `factory sync [all\|opencode\|claude\|codex]` | `internal/synccmd` | Generated harness Markdown, JSON and TOML |
| `factory prereq` | `internal/prereqcmd` | Tool discovery and the configured product Go minimum |
| `factory check` | `internal/checkcmd` | Adopter Make targets, policy gates and local hooks |
| `factory eval structural --harness=NAME` | `internal/evalcmd` | Real shared test-edit gate and native adapter wiring |
| `factory eval golden [options]` | `internal/evalcmd` | Selected runner, task oracle and optional live harness |
| `factory eval run HARNESS WORKDIR` | `internal/evalcmd` | Explicitly selected Codex, Claude or OpenCode executable |
| `factory review DIFF_FILE` | `internal/adversarialreviewcmd` | Configured fixed provider endpoint or explicit streaming Go client |
| Sourceable events, timing, palette, hooksPath and budget configuration | `internal/helpers`, `internal/eventlog`, `internal/githooks`, `internal/presentation`, `internal/config` | Literal bounded caller-shell plans and native host hook formats |

The reviewed `go-hybrid-v1` installation map generates adapters at the old sync,
check, prereq, eval, review and runner script paths. They reach the installed
`factory` command. Sourceable helpers use the private Go bridge; they do not
execute shell text returned by the runtime. Existing config and role bridges
remain shared. Native init and doctor call synchronization directly in Go.

Synchronization validates the entire selected rendering before publication,
rejects unsafe paths and existing output links, writes staged files, and creates
exclusive `CLAUDE.md` backups. Publication is atomic per file, not a transaction
across every generated file. Models remain literal data. Only the initial role
frontmatter block is metadata; later body rules remain intact.

Golden evaluation uses the caller's project, preserves the result/baseline
schema and POSIX checksum identity, and scores ordinary runner exits through
the unchanged task oracle. A timeout, cancellation, uncertain cleanup or edited
oracle cannot pass. Mock runs stay deterministic and local. Real harnesses and
paid review requests remain explicit; qualification uses fake providers.

Review keeps the configured model, token limits, reasoning and pinned provider
semantics. Native HTTP requests have fixed endpoints, bounded responses,
disabled redirects and sanitized errors. The explicit streaming client remains
available without a fallback to another provider or model.

Intentional retained shell boundaries are policy gates, sourceable assignment
adapters, Bash operand expansion for local-hook compatibility, generated
invocation adapters, user Make/build commands and harness-native hook formats.
The source checkout still carries the old default bootstrap/upgrade and frozen
compatibility implementations until the separately qualified default cutover.
Their presence does not establish release readiness or a runtime fallback in
an opted-in Go installation. No Go or Python interpreter is required for the
converted installed orchestration; retained JSON validation gates still require
`jq`, and configured product packs can require their own toolchains.

Outside-in acceptance covers the compiled commands, frozen baseline artifacts,
invalid selectors, literal model/config values, safe publication, real process
cancellation, task oracle integrity, fake provider requests and sourceable plan
validation. Installed integration and native hosted CI provide separate evidence;
see the qualification entry in the decision log and the release readiness file.
