# Input limits do not bound repeated work

Bounded input can still produce quadratic work when every setting scans and
copies the entire file. Index selected keys, accumulate edits, then render once;
preserve intermediate size limits even when a later edit would shrink the result.
The governing contract is docs/adr/0090-go-native-config-migration.md:123.

Provenance: observed 2026-09-30 America/Los_Angeles while addressing PR #117's
Copilot comment `discussion_r4151713766`. Two 8 MiB/4096-setting fixtures exceeded
their five-second post-admission deadlines before the indexed editor:
`rtk proxy env FACTORY_AGENT_ROLE=spec-writer FACTORY_CLI_TEST_RACE=1 go test -race -v ./acceptance -run TestAcceptance -ginkgo.focus='Native Go config migration bounded planning performance' -ginkgo.no-color -ginkgo.succinct -count=1`
reported `0 Passed | 2 Failed` (`13.206s`). After correction, normal compiled CLI
cases completed after admission in `220.631958ms` and `211.999625ms`, and the
compiled-CLI/outer-race native selection passed 77 cases (`37.035s`). These are
fixture observations, not promises about arbitrary storage or hardware.
