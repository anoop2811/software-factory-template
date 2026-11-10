# ADR 0145: render an explicit installed-factory CI workflow

Status: implemented; five independent workflow criteria pass locally. Hosted CI pending.
Decision: 161. Continues R4 after merged PR #175.

## Bounded delivery

PR #175 merged as `9467b34d611bb62de7f8dfe8000404eb026f1dbe` on
2026-11-09. Its tree matches the reviewed head. The native preparation controller
and authenticated installer route are available; adopters still need correct job
wiring around them. Add a native command that renders one complete GitHub Actions
workflow to stdout for deliberate review and installation.

This slice does not overwrite workflow files or change initializer/upgrade
publication. Existing source-development CI cannot be made adopter-safe merely
by adding preparation: it includes unshipped development regressions. Pack jobs
also require preparation individually. Preserve frozen recipes and current
workflow bytes; automatic new-project integration and preservation-aware existing
workflow reconciliation remain later R4 work. Adding this workflow beside broken
old jobs is not a completed migration.

## Public operation

```sh
factory ci workflow --go-candidate TAG --origin-target OS/ARCH \
  --installer-revision FULL_COMMIT_SHA
```

Require all three selectors explicitly. Reuse the existing canonical exact-release,
supported-origin and lowercase forty-hex revision validators. Accept separate and
equals-form values. Reject duplicates, unknown flags, positional operands, `--`,
missing/empty/invalid values and values attached to help. Normalize `-h` for
duplicate detection. Help may omit required selectors but still validates supplied
values. Invalid requests return two before output or effects; help returns zero.

Render deterministically to stdout and report write/cancellation failure as
nonzero. Do not open project configuration, inspect ambient selector variables,
acquire release assets, execute a subprocess, write a file or call a model to
render. Shell redirection is the caller's operation, so documentation must render
to a temporary file rather than silently clobber an existing workflow.

Keep `ci prepare` parsing, authentication, custody and publication unchanged.
Extend native CI help and command documentation to distinguish the two operations.
The shell installer continues to bootstrap preparation only; it is not a workflow
renderer. No additional executable, dependency or public private-protocol flag is
needed.

## Generated workflow contract

Emit an ordinary pull-request, default-branch-push and manual workflow with one
Ubuntu 24.04 job and a bounded twenty-minute allowance. Filter non-default push
jobs using the repository default branch from GitHub context, not a hardcoded
branch name or shell interpolation. Group PRs by number; give every non-PR run
its own run-ID group. Cancel only superseded PR runs. Do not emit a privileged
`pull_request_target` or `workflow_run` trigger, paid model job, matrix, cache,
secret input or permissive failure handling.

Declare `contents: read`, pin checkout to the reviewed complete action SHA, fetch
full history for existing range gates, and disable credential persistence. Keep
GitHub expressions out of shell code by passing necessary values as environment
data. Literal validated tag, origin and installer revision belong in the
preparation step environment, not caller-selected script fragments.

Preparation uses a separately reviewed official installer at the explicit immutable
source revision. Download it over HTTPS from the fixed official repository into
a fresh private temporary file under the runner temporary directory, outside the
application checkout. Use fail-fast shell behavior and remove that temporary on
success or failure. A failed or incomplete download must never execute. The
checkout's `install.sh` and ambient factory source/repository selectors are not
bootstrap authorities.

Give only the preparation step the ordinary read-only GitHub job token as
`GH_TOKEN`. Invoke the downloaded installer with literal explicit release/origin
arguments and `--ephemeral`. Preparation authenticates runtime assets through the
existing boundary; pinning the installer is a separate trust decision. Rendering
does not prove that an arbitrary supplied revision contains a suitable installer,
or that the selected release already supports preparation. Do not infer either.

Place an explicit comment for application-specific tool/dependency setup after
preparation and before checks. Preparation requires a clean tracked checkout;
application setup must not precede it. The factory needs no Go compiler or Python
for shipped behavior, but application checks retain their own tools. Do not emit
a no-op step that claims to have provisioned application dependencies.

Run installed `./factory selftest`, `./factory check`, and structural evaluation
for OpenCode, Claude Code and Codex. Do not synchronize away failures, install
harness clients, supply model credentials, or substitute source-development
checks. Ordinary command failures must fail the job. Application setup and checks
must not inherit the preparation step's credential environment.

## Independent qualification

Use an independent spec writer and observe RED before implementation. Cover strict
parsing, no-effects help/refusal/rendering, deterministic complete YAML, safe
selectors, output failures, and unchanged preparation behavior. Inspect parsed
workflow semantics and run actionlint; string presence alone does not establish
an executable workflow contract.

Execute the actual emitted run blocks with the existing packaged-init, commit and
fresh-clone qualification machinery. Retain real generated Makefile/harness
behavior in at least one journey; the earlier preparation fixture's simplified
`check-drift` is not sufficient evidence for all-three-harness parity. Keep
application setup/check witnesses and prove a passing job and a deliberate
application failure. No Go/Python/model/harness client may be required by factory
behavior. Bind the trusted installer transport fixture to the exact emitted
official revision URL, poison any checkout-local installer, and prove download
refusal prevents execution and leaves no temporary residue.

Preserve project configuration, existing workflow bytes, Git HEAD/index and
unrelated application data. Register new acceptance groups in the exact current
partition/worker census without weakening scheduler controls or raising limits.
Use full private checkouts for builds, tests and hooks; preserve the operator's
ignored binary and unrelated changes. Record exact-head hosted source CI and
independent review separately from generated-job simulation. Hosted adopter
rehearsal, automatic workflow integration and stable release remain open.

## Authoritative sources

Fetched 2026-11-09: [checkout v7.0.1](https://github.com/actions/checkout/releases/tag/v7.0.1),
published 2026-07-20, resolves through the official tag API to commit
`3d3c42e5aac5ba805825da76410c181273ba90b1`. Reuse that existing repository pin.
[GitHub secure-use guidance](https://docs.github.com/en/actions/reference/security/secure-use)
supports immutable action revisions, minimum token permissions and avoiding
privileged untrusted checkout execution. No tool/library upgrade is part of this
slice.

## Event and range qualification refinement

GitHub concurrency can replace pending runs even when `cancel-in-progress` is
false. Use unique non-PR run-ID groups, not a shared branch group. The authoritative
[concurrency contract](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/control-workflow-concurrency)
was fetched on 2026-11-09 before implementation.

Dynamic default-branch event selection does not change the existing native range
gates, which use `origin/main` (`internal/checkcmd/command.go:83`, `:118`, `:130`).
Document that existing limitation for projects with another default branch;
event-filter qualification must not claim generalized native range coverage.

## Prepared-project fixture refinement

The first real generated-job run exposed a pre-existing initializer ordering
boundary: its next synchronization changes only the order of the Claude allow
array, following OpenCode's serialized key order. The diagnostic retained the
exact `.claude/settings.json` diff; membership and deny rules were unchanged.
The initializer already instructs the adopter to synchronize adapters after setup
(`internal/initcmd/command.go:385`). This renderer qualifies a prepared, committed
project: run that documented native synchronization before the fixture's initial
Git commit, then require a second synchronization to produce byte-identical
managed assets. Do not synchronize during checkout preparation or insert repair
into the emitted workflow. Keep first-initialization ordering reconciliation open
in R4 automatic integration; this fixture does not prove untouched-init CI parity.
