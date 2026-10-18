# Try an authenticated Go factory candidate

Candidate selection is explicit until the replacement release passes its remaining
release gates. Use a published `TAG` with all twelve attested release assets.
An older release without those assets is refused; no alternative release or script
implementation is substituted. The contract is [ADR 0101](../adr/0101-go-release-handoff.md).

The invoking machine needs a trusted GitHub CLI, Bash and the existing platform
tools. Installing or using the candidate does not require a Go compiler or Python.
Application language packs still declare their own build tools. Acquisition and
deterministic health checks do not call a model.

## Initialize a new project

From the project directory, select the published candidate:

```sh
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- init --go-candidate TAG --pack go
```

The initializer retains its prompts and explicit final consent. It installs Go
directly, proves the installed gates and records success after validation. Existing
factory metadata requires upgrade instead. A new project need not have Git metadata;
the initializer proves ignore rules without creating a repository for you.

## Upgrade v0.1.6 or a supported Bash installation

Start with a preview from the repository or one of its subdirectories:

```sh
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- upgrade --go-candidate TAG --dry-run --json
```

This explicitly acquires release assets into temporary private storage. It leaves
the project unchanged. It infers the supported predecessor and project inputs only
when the observed bytes identify them unambiguously. Unknown or customized required
files remain preserved blockers. A missing historical input can be supplied with
the repeatable literal argument `--project-input KEY=VALUE`.

Review the proposal, prevent other factory work for the maintenance period, then
repeat with its complete `proposal_digest`:

```sh
curl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- upgrade --go-candidate TAG --confirm DIGEST --quiescent
```

Repeat any project-input overrides used for the preview. Confirmation binds the
release identities, archive bytes and fresh project observations. A changed tag,
asset, input or project requires another preview; a previous download is not
permission to mutate. Replacements and retirements use the existing checked,
ignored `.factory/backups/MIGRATION_ID/` recovery set.

## Use the selected installation

Ordinary `./factory` commands and retained direct/sourceable paths use the checked
installed Go runtime. To select a later candidate, use the same public interface:

```sh
./factory upgrade --go-candidate TAG --dry-run --json
./factory upgrade --go-candidate TAG --confirm DIGEST --quiescent
```

Go-to-Go upgrades authenticate the predecessor too and require its supported
generation recipe. An installed Go factory initializes another project with
`./factory init TARGET --go-candidate TAG`; omitting the selector refuses before
prompts or writes rather than publishing the inert legacy scaffold.

Successful acquisition removes its temporary cache. Interrupted upgrades retain
needed authenticated inputs and print a concrete read-only recovery preview.
Partial initialization reports its target and pending evidence, without claiming
automatic rollback. Preserve that evidence and reconcile the reported state before
removing a pending barrier.

The bare installer and existing source/ref defaults remain unchanged. Automatic
later-release backup retention, actual release authentication on all four targets,
the mandatory adopter pilot and default cutover remain tracked in
[GO_RELEASE_READINESS.md](GO_RELEASE_READINESS.md).
