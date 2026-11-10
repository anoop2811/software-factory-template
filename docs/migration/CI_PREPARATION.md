# Runtime preparation for a disposable CI checkout

The Go factory keeps its executable and installation assets in ignored private
paths. A Git clone therefore needs runtime preparation before it can execute
the installed `./factory`. Public launchers also identify the runtime for the
machine on which the project was initialized; another platform needs a complete
matching selection, not just a copied executable.

The native `ci prepare` command and its installer entrypoint are being qualified
for the replacement release. They are unavailable in v0.1.6 and in earlier
candidates that lack the command. Current Go source also provides the explicit
workflow renderer below. Current authenticated initialization adds the ordinary
workflow described next; reconciliation of existing installations remains
[release-readiness work](GO_RELEASE_READINESS.md). The preparation and rendering
commands themselves do not install or rewrite workflow files.

## CI created by current initialization

Current authenticated `factory init` creates `.github/workflows/factory.yml`,
using the exact selected release, its authenticated source revision and the
initial installation platform. Earlier published candidates keep their original
initializer behavior; this is part of the upcoming replacement release.
Explicit candidate tags that are not full canonical semantic versions can still
initialize, but CI preparation does not support them. Initialization explains
that limitation and preserves existing workflows without creating the new job.

The workflow prepares the installed runtime, provisions the selected Go,
TypeScript and/or Java application tools, then runs the public factory checks
and all three harness structural evaluations. Java respects the Gradle/Maven
selection. Security checks remain required; existing mutation checks remain
advisory. A single PR-only mock golden evaluation exercises the scorer without
calling a model or reading model secrets. The job has a twenty-minute limit;
review that limit and application-specific setup before committing the scaffold.
Workflow linting retains the repository's narrow compatibility exception for
GitHub's job-level `cache-mode`; unrelated workflow errors still fail.

Initialization no longer adds the source repository's development workflow or
separate language-pack workflows. Existing application workflows, including
`ci.yml` and `<pack>-pack.yml`, remain unchanged. If `factory.yml` is already
occupied, initialization refuses before publication so the existing file can be
reviewed deliberately. It does not rename or remove older jobs on your behalf.

Explicit `--local` initialization cannot supply an authenticated downloadable
release. It preserves existing workflows, omits the new ordinary factory job,
and reports that hosted CI needs an authenticated release. The separate opt-in
review lane's installed bootstrap and preservation-aware upgrade of existing CI
are still pending; this ordinary job does not qualify those paths.

## Render a workflow for review

From an existing Go installation or separately provisioned native executable,
render to a new temporary file:

```sh
factory_workflow=$(mktemp)
./factory ci workflow \
  --go-candidate TAG \
  --origin-target darwin/arm64 \
  --installer-revision COMMIT > "$factory_workflow"
cat "$factory_workflow"
```

Replace `TAG` with the project's exact published release and select the original
installation platform. `COMMIT` must be the full lowercase forty-character SHA
of an independently reviewed official factory source commit whose installer
supports `ci prepare`. Rendering validates selector syntax; it neither downloads
nor authenticates those selections, and does not assume they are already
published. The runtime and installer selections have separate trust requirements.

After changing harness configuration, run `./factory sync all` and review and
commit its outputs before enabling CI. Current synchronization preserves ordered
OpenCode permissions, so an unchanged newly initialized project needs no extra
repair sync. CI checks drift and never repairs the committed configuration.

The output contains one Ubuntu 24.04 job with read-only repository permissions,
a SHA-pinned checkout action, full history and no persisted checkout credentials.
It obtains the pinned official installer outside the application checkout,
prepares authenticated runtime custody, and runs installed selftest, project
checks and structural evaluation for OpenCode, Claude Code and Codex. Only
preparation receives the ordinary read-only GitHub job token. No model secret,
harness client installation or paid model call is generated.

Review the application-setup insertion point after preparation and add the
project's language tools and dependencies there. For example, Go application
checks may need their selected Go/lint/security tools; TypeScript may need Node
and package installation; Java may need its JDK and build tool. The factory's own
runtime does not require Go or Python. Preparation must run before any setup
that changes tracked files.

The workflow runs on pull requests, manual dispatch and pushes to the repository's
default branch. Non-PR runs have unique concurrency groups; superseded PR runs
can be canceled. This event selection does not alter existing native range gates:
those currently use `origin/main`, so a different default branch does not gain
generalized range coverage from rendering alone. Review job timeout and project
requirements before committing the workflow.

Choose a new workflow destination deliberately after reviewing the output; the
renderer writes only stdout and has no overwrite option. Inspect existing jobs
before adoption. The older generic template calls development-only scripts, and
each pack job that invokes installed factory commands needs its own preparation.
Adding this workflow alongside obsolete jobs will not make those jobs pass.
Preservation-aware existing-workflow integration remains pending; retain project-specific
setup, checks and policy when reviewing changes to existing workflows.

Use `./factory ci workflow --help` for the selector contract. The shell installer
supports `ci prepare`, not workflow rendering. Older published candidates and
v0.1.6 do not contain this new command.

## Bootstrap without an existing native executable

Supply the installer from a separately trusted, reviewed or pinned source, then
run it from the root of the fresh disposable checkout:

```sh
sh /path/to/reviewed/install.sh ci prepare \
  --go-candidate TAG \
  --origin-target darwin/arm64 \
  --ephemeral
```

The installer authenticates the host release executable before running it. That
selected owner independently authenticates the complete release images and
prepares the checkout. An installer supplied by an application's pull request
is not independently trusted merely because it claims to verify release assets.

The command requires an explicit exact tag and origin. `FACTORY_REF`,
`FACTORY_HOME` and `FACTORY_REPO` do not select a different CI release, source
cache or repository; this operation uses official releases only. It does not
fall back to legacy scripts if the selected release cannot prepare CI. Use
`sh /path/to/reviewed/install.sh ci --help` for help without acquisition.

The installer needs the existing shell/Bash and GitHub CLI prerequisites;
native preparation also needs Git. The factory does not need Go or Python in
this job. Application checks still need their own language tools.

## Preparation with an existing native executable

In a fresh disposable job checkout, invoke a separately provisioned, trusted
native factory executable that supports this command:

```sh
/path/to/trusted/factory ci prepare \
  --go-candidate TAG \
  --origin-target darwin/arm64 \
  --ephemeral
```

Replace `TAG` with the project's exact release tag from `.factory-version` and
select the platform that produced the committed installation: `darwin/arm64`,
`darwin/amd64`, `linux/arm64`, or `linux/amd64`. The host platform is detected from
the running executable. `--go-candidate` accepts exact stable or prerelease tags;
it does not select the latest release or upgrade this project.

The command uses the existing official-release authentication path and requires
Git and the GitHub CLI, including its attestation verification support and access
to official release assets. Both platform images must belong to the same exact
release and source commit. A foreign-platform executable is never run. Installed
factory behavior needs no Go compiler or Python; configured application checks
still need the application's tools.

`--ephemeral` confirms that the checkout is disposable and no other process is
modifying it. This is for CI jobs, not a developer's active working copy. The
command requires clean tracked files, intact public factory assets, effective
private ignore rules, and absent private runtime custody. It refuses customized
factory-owned assets, linked/unsafe destinations, interrupted state, or an
already prepared checkout. It preserves application configuration and files,
workflow customizations, Git HEAD/index and ignore rules.

After successful preparation, use the ordinary project entrypoint:

```sh
./factory selftest
./factory check
```

Preparation authenticates and installs the runtime; it does not run project
setup, install language packages, synchronize harness configuration or call a
model. Configure those job steps deliberately.

## What changes and how failures are handled

The job receives the complete private host runtime and assets. Public factory
adapters may change to bind that runtime. Those changes are local to this job;
do not commit them or treat preparation as a project upgrade. Original tracked
bytes remain in the unchanged Git commit and index, so this explicitly disposable
operation creates no adjacent backup files. Ordinary upgrades retain their
durable ignored backups and recovery behavior.

If preparation fails after publication starts, the pending barrier keeps partial
state from being used. Preserve the job's diagnostics and recreate its checkout.
Do not delete the barrier or retry over partial custody. Use the normal upgrade,
rollback and recovery procedures for persistent installations.

The native implementation contract is [ADR 0143](../adr/0143-ci-checkout-preparation.md);
the installer boundary is [ADR 0144](../adr/0144-ci-installer-bootstrap.md), and
workflow rendering is [ADR 0145](../adr/0145-installed-ci-workflow-rendering.md).
Current initializer integration is [ADR 0146](../adr/0146-initialized-installed-ci.md).
