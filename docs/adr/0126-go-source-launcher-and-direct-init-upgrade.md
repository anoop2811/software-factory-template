# ADR 0126: activate the Go source launcher and direct initialization entrypoints

Status: accepted bounded R2 source function; independent semantic RED precedes code

Date: 2026-11-01 UTC

## Function and predecessor

After human merge of PR #156 as
`1754de23de95a7e3c734bedd86894295e823c988`, activate the tracked developer
`factory` launcher and direct `scripts/factory-init.sh` and
`scripts/factory-upgrade.sh` adapters. Reuse the existing sixteen-command
Cobra dispatcher and qualified native initialization, upgrade and gate-proof
owners. No command may recurse through its own adapter or invoke a legacy
implementation as fallback. This completes ADR 0114 item 8's source function;
the remaining command/helper adapters and historical proof-runner cleanup are R3.

Keep the public installer default at v0.1.6, existing installation recipes,
generated compiler-free installed launchers, tool versions, dependencies,
worker/time/output allowances and review/budget settings. This PR publishes no
release and does not complete the packaged-platform, pilot or stable gates.

## Developer bootstrap and installed boundary

A source checkout compiles its own `./cmd/factory` using the provisioned Go
toolchain with read-only module selection, then replaces the launcher process
with that executable and literal original argv. Preserve caller cwd, standard
input, output, environment, inherited descriptors, umask and native exit status.
The source compiler uses the provisioned local toolchain/module cache, with
automatic toolchain/module acquisition and external workspace selection disabled.
It receives no application stdin. Missing Go, compilation
failure or uncertain compiler ownership refuses without a Bash/Python fallback.

Reuse the preserved source-launcher implementation at immutable reference
`07e323a76bbaeb34b2ef12321d958b1539fa528d` as reviewed design input, not as
qualification of current code. Its compiler group admission, bounded cancellation,
unused diagnostic-descriptor selection, exact startup diagnostic reconciliation
and pre-fork asynchronous-job custody require fresh independent execution.
Only the confirmed owned group may be signalled. Reaping the direct child and
establishing group absence precede scratch deletion or executable publication.
Unconfirmed ownership retains private build evidence and returns status 125.

Refuse a symlinked source launcher before selecting its directory or compiler;
an alias directory must not choose different source code. `factory-go` remains
the single ignored developer build output. A successful source invocation refreshes
that output atomically; a failed build preserves the previous output. Refuse
directory and other unsafe destinations, including when changed during a build.
The shell invokes that private build's existing Cobra bridge with
`helper publish-source` and no operands. The helper derives its own physical
executable/root, restricts the private build name, and publishes an independent
checked cache copy through the existing confined exact-leaf publication helper.
Then exec the invocation's private build with original argv. The developer cache
confers no installed identity, publisher trust or additional public command.
Unique private build, admission
and startup files are removed on confirmed completion, so ordinary source use
does not accumulate temporary executables. The agent's primary checkout contains
an operator-owned developer binary; qualification must use private fixtures and
must neither execute nor replace that operator-owned file during this task.

Installed projects retain the independently generated launcher pointing to
`.factory/bin/factory-runtime`, installed admission, leases and immutable runtime
identity. Native initialization must install that generated launcher, never the
developer bootstrap. Ordinary installed behavior requires neither Go nor Python.

## Thin direct entrypoints and selected branches

The two direct scripts derive their owner directory physically, preserve literal
root names and argv, and exec that owner's `factory` with `init` or `upgrade`.
Their semantic implementation is the existing Go command. Direct ordinary init
selects its authenticated stable default; developers explicitly select `--local`
when they intend committed source. Upgrade retains read-only preview, fresh
consent, quiescence, recovery and transaction ownership.

The shell installer still supports explicit legacy tags and non-version branch
refs. After Git fetch, recognize the exact current native initializer adapter
protocol before supplying `--local --source FETCHED_CHECKOUT` for a branch-selected
init. The user's selected checkout, not the unpublished v0.2.0 default, owns that
local operation. Preserve remaining literal operands and input. Legacy initializer
bytes retain their existing argv; stable Go refs keep ADR 0124 authentication.
Do not infer an adapter from a product go.mod, execute a version probe or use
ambient variables to grant publisher trust. Bare fetch remains Git-only.

The exact native upgrade-adapter protocol similarly selects
`--local --source FETCHED_CHECKOUT` and preserves every remaining literal operand.
Its native owner previews by default and retains fresh confirmation/quiescence;
the installer must not drop preview/confirmation arguments or auto-apply changes.
The old legacy-upgrader vector remains compatible for historical adapter bytes.

For an existing cache used by branch initialization, or any cache advertising a
native initializer, first require its recorded repository and requested local
ref's complete commit identity to match the effective selection and HEAD. Refuse
status 2 before cached code or project writes on a missing/mismatched identity,
with advice to refresh safely or select a fresh FACTORY_HOME. This check is
read-only; cache reuse does not claim to fetch the latest remote branch revision.
Ignore ambient Git redirection/configuration variables for these identity reads.
For branch-upgrade refresh, require the requested recorded origin and a clean
cache before fetching. Keep an actual requested branch reference consistent with
the successfully selected commit, so refresh followed by init can reuse the same
cache. Use transactional checkout of that named cache branch without forced
working-tree overwrites; tag selection stays detached and retains its exact ref.
Concurrent conflicting edits refuse while preserving the cache's prior branch.
Unrelated legacy-tag cache behavior remains the existing compatibility boundary.

For native upgrade adapters, discover the caller's root with isolated Git and
literal newline framing, then execute from the original caller cwd. Native Go
retains authority for target ancestry and path admission. An inherited GIT_DIR
or GIT_WORK_TREE cannot select another project, and command substitution must
not trim a newline-bearing root into a different existing sibling. Normalize the
selected cache physically before native transport, including a relative cache
selected from a project subdirectory. Preserve the legacy target-routing path.

Direct current-source copy-hook qualification must leave its explicit runtime
selector unset and exercise the automatic physical-owner protocol. Prove a real
declared input untracked refuses and the same tracked input passes, with exact
tree/index preservation. Native selftest's explicit runtime does not establish
that separate automatic-dispatch boundary.

Git interfaces checked against their official documentation on 2026-11-01:
[revision verification](https://git-scm.com/docs/git-rev-parse) documents
`^{commit}` and `--end-of-options`;
[checkout](https://git-scm.com/docs/git-checkout) preserves nonconflicting local
changes and refuses conflicting changes without force; and
[remote URL inspection](https://git-scm.com/docs/git-remote) documents `get-url`
and its URL-rewrite behavior. No tool version is changed.

## Copy inventory and historical evidence

ADR 0125's actual native inventory must remain active when the current initializer
has no Bash copy statements. Recognize only the physical hook owner's exact
native initializer protocol and invoke that owner's source launcher through the
existing private bridge. Explicit runtime selection retains precedence and strict
refusal; installed admission retains precedence over source detection. A foreign
subject cannot choose executable code. Unknown/empty initializer bodies still
refuse. A native validation failure never falls back to the legacy scanner.

Before conversion, inventory fixtures still borrowing current legacy bodies.
Freeze each genuine historical engine/dispatcher/initializer fixture at its
actual pre-conversion Git revision, preserving assertions and documenting its
provenance. Keep current public entrypoint assertions against current source.
Do not delete coverage, silently relabel current behavior as historical or add
production legacy copies. The direct historical shell suite can retain its
explicit historical fixtures until R3 replaces its final controller.

## Independent outside-in qualification

An independent spec author observes current tracked source dispatch fail the
new native-entrypoint expectation before production conversion. Keep compiler
setup success separate from semantic RED. Exercise literal argv/cwd/stdin and
exit status, successful publication with no private residue, compiler failure
preserving the previous binary, missing compiler refusal, actual parent-only
INT/TERM/HUP with held-open readiness pipes, inherited descriptors and uncertain
group custody. Preserve previous caller jobs and distinguish compiler output
from the exact known Bash startup diagnostic.

Observe the real direct scripts reach native owners rather than a sentinel legacy
body. Exercise current selected-source initialization and installed compiler-free
operation, current source gate proof, native upgrade preview/refusal, and actual
branch installer forwarding with a legacy control. A recorder alone qualifies
transport, not actual native installation. Keep all existing criterion identities
unless a separately documented historical fixture provenance changes its label.
Derive total and lane census from actual discovery and retain the completeness
ratchet, exhaustive partitions, race checks and configured deadlines.

Run the review-diamond recipe across correctness, security and test coverage,
then independently refute candidates before accepting findings. Hosted checks
and complete exact-head adversarial review remain separate evidence from local
qualification. Human protected-path review and merge precede the next R3 task.

## Genuine shallow-source installation boundary

The actual installer starts with a shallow Git clone. Its selected native local
packager also requires the existing two immutable predecessor catalogs from
`installationimage.References`; do not duplicate those pins in Bash or weaken
offline packaging when their objects are absent. Qualify the existing real-native
journey through the actual installer and a fresh file-transport source cache,
including its shallow Git state, actual initialization and installed gate proof.
Transport-only recorders and a full developer clone do not prove this boundary.

When explicit initialization/upgrade selects a native source adapter, obtain the
selected cache's complete history before local packaging if it is shallow. Keep
the selected commit and source/index unchanged. This is Git acquisition by the
already network-capable installer; the Go local packager stays offline. Bare
fetch and genuine legacy adapters retain their existing shallow behavior. A
repository lacking the required immutable objects still refuses; no alternate
publisher, release selection or partial installation fallback is introduced.

## Hosted review: invocation custody and setup transport

Concurrent source invocations must execute their own compiler output even when
another invocation publishes the shared developer cache before exec. Retain the
single-linked private BUILD file throughout that command. Publish a separate
bounded, digest-checked cache copy using the existing `filepublish` machinery;
hardlinking would violate the qualified filesystem reader's single-link rule.
The shell execs its exact private file with `--source-launcher --` followed by
the original public operands. Installed admission precedes that private protocol;
source protocol admission precedes source bridge/local-image dispatch. It grants
no installed identity, publisher trust or new public command.

Keep the private executable's pathname valid throughout command execution for
Darwin lookup and native source proof. Pin its confined root and qualified file;
on return, check the same file before removing only that invocation's leaf under
a noncancelled cleanup context. Never remove the shared cache or sweep matching
prefixes. Preserve a nonzero command status when cleanup also fails; turn success
into failure with an actionable retained path. Ignore the owned transient source
namespaces so strict committed-source admission continues to reject unrelated
changes. Interrupted/uncertain execution can retain ignored private evidence.

Atomic cache publication replaces the declared developer output leaf and does
not follow a destination symlink. Refuse unsafe destination types observed before
publication, including compiler-phase replacements. This is not a conditional
rename against arbitrary same-user mutations after inspection; no outside
referent may be modified or selected for execution through that shared cache.

Native-branch bare-fetch advice must re-enter installer init with the same
effective ref, physical cache and requested repository. Quote every selector as
literal shell data. Bare fetch stays Git-only; the subsequent initializer owns
history acquisition before the offline packager runs. A direct local-adapter hint
from a depth-1 cache cannot establish that complete setup path.

For non-version branch execution, defer terminator refusal until the fetched
adapter capability is known. Current native adapters preserve `--` and every
following token; the installer must not reinterpret later `--ref`/help/source
tokens. Genuine legacy branch adapters refuse before execution or target writes
after Git capability discovery. Stable legacy tags keep their pre-tool terminator
refusal, and authenticated Go release/candidate parsing retains its owner.

The Template CI source gate needs its declared module cache provisioned before
the offline developer bootstrap. Download only the existing go.mod/go.sum inputs
in CI setup; retain read-only module compilation, fixed versions and shipped
compiler independence. Qualify this against the actual failed hosted copy gate.
Retained selftest source predicates must exclude their own assertion text;
removing their real invocation markers must fail the predicate before repair.

Compiler-unavailability qualification must exclude a system Go compiler as well
as the test-owned compiler. Use a private utility path and prove child lookup
cannot find Go before exercising the launcher; require the launcher's explicit
missing-compiler diagnostic. Absence of the fake compiler's record alone does
not prove that no other compiler was attempted. Keep this test environment
change separate from the production tool-selection contract.

The borrowed-group witness in cancellation qualification must relinquish the
caller's output captures itself before reporting readiness. Preserve its release
descriptor and unrelated descriptors; close only positively identified capture
duplicates. Deliberately inherited stdout/stderr duplicates and an unrelated
pipe must exercise that isolation. Preserve the real `exec.Cmd.Wait` assertion
before witness release, the live borrowed-group witness and all existing timing
allowances. Observe launcher liveness separately because Go's Wait also waits
for capture-copy EOF. A controlled capture-holder reproduction establishes this
test ambiguity; it does not by itself establish the original hosted cause.

The witness helper must compile on every supported target. Use the existing
`golang.org/x/sys/unix` portable descriptor-duplication wrapper instead of an
architecture-specific syscall surface. Compile the acceptance package for
Linux/macOS on amd64/arm64 before publishing this portability refinement;
retain the locked dependency versions and the same descriptor assertions.

Native-bundle dependency provisioning keeps the existing Go cache, proxy and
checksum policy. Retry the same `go mod download` at most three times, with
five- and ten-second pauses between failed attempts. Stop on success and return
the final Go failure status on exhaustion. Keep the existing job timeout and
offline source-runtime compilation unchanged. Controlled execution must show
recovery after a transient failure and a nonzero result after three failures;
neither a sleep nor a diagnostic may turn acquisition failure into success.
