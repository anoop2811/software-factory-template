# Runtime preparation for a disposable CI checkout

The Go factory keeps its executable and installation assets in ignored private
paths. A Git clone therefore needs runtime preparation before it can execute
the installed `./factory`. Public launchers also identify the runtime for the
machine on which the project was initialized; another platform needs a complete
matching selection, not just a copied executable.

The native `ci prepare` command is being qualified for the replacement release.
It is unavailable in v0.1.6 and in earlier candidates that lack the command.
Automatic installer bootstrap and generated workflow integration remain separate
[release-readiness work](GO_RELEASE_READINESS.md). This page documents the native
preparation boundary, not a complete copy-and-paste workflow.

## Explicit preparation

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

The implementation and qualification contract is [ADR 0143](../adr/0143-ci-checkout-preparation.md).
