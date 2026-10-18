# Duplicate borrowed stdin before starting a background shell child

Observed 2026-10-18 via the Linux Go source check in
[run 39095840523](https://github.com/anoop2811/software-factory-template/actions/runs/39095840523/job/117581980373)
and a local Dash reproduction: `printf 'typed\n' | /bin/dash -c 'cat <&0 & wait'`
produced no output. The control with `exec 9<&0; cat <&9 & wait` printed `typed`.
The same self-duplication had preserved input under the macOS host `sh`.

A self-duplication of fd 0 does not portably preserve background stdin. Duplicate
it in the foreground and close the auxiliary descriptor in child and parent.
The canonical contract is docs/adr/0101-go-release-handoff.md:508; its acceptance
criterion pairs literal file-mode answers with an installer read from a pipe,
under the host shell and Dash when available. Preserving answers and preventing
unread source from becoming answers are separate obligations.
