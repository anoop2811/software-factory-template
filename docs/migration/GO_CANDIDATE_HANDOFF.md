# Try an authenticated Go factory candidate

To install or upgrade to stable v0.2.0, follow the [stable guide](../UPGRADING.md).
Use this guide to try a specific published prerelease with all twelve attested assets.
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
generation recipe. In v0.2.0 an authenticated installation reuses its selected
release for `./factory init TARGET`. Pass `--go-candidate TAG` to try a specific
prerelease. An explicitly local installation must select a release or local source.

The current source consumer retains the closed RC3 generation recipe from
`95486ecdd3868a319a0e6362ee99cafd8f154200`. It reconstructs the original
141-role layout from a complete eight-file fingerprint and frozen native role
data, so later target inventories need not be mistaken for the prior layout.
Current-recipe matching also checks the selector and historical-role files.
Unknown or altered unsupported recipes remain preserved conflicts; image
authentication, project inputs and fresh mutation consent are still required.
The older recipe selected 85 cached files. A v0.2.0 upgrade can retire its 83
unused cache entries after establishing ownership. See [cache retirement](../adr/0123-owned-obsolete-initializer-cache-retirement.md).

Successful acquisition removes its temporary cache. Interrupted upgrades retain
needed authenticated inputs and print a concrete read-only recovery preview.
Partial initialization reports its target and pending evidence, without claiming
automatic rollback. Preserve that evidence and reconcile the reported state before
removing a pending barrier.

Whole-installation JSON reports provide `recovery_arguments` as literal argv for
a fresh read-only preview from the installation root. The vector retains the
selected image identities, local or authenticated trust operands, Go predecessor
image and project-input overrides. `recovery_command` is a short description;
use the structured vector for replay. Any later mutation still requires the new
proposal's confirmation and explicit quiescent declaration.

Candidate preview also reports [backup retention](BACKUP_RETENTION.md), including
held and preserved exceptions. A successful later forward release keeps its usable
predecessor recovery and prunes only qualified older sets. Standalone controls are
`./factory upgrade --keep-backup MIGRATION_ID` and
`./factory upgrade --prune-backup MIGRATION_ID`.

The [stable release record](GO_RELEASE_READINESS.md#stable-v020) documents the
four-platform qualification and the separate v0.1.6 adopter pilot. A bare installer
invocation fetches the template. Give it an explicit verb to initialize or upgrade.

Release maintainers can use the
[published-release qualification lane](PUBLISHED_RELEASE_QUALIFICATION.md) to
collect actual authentication, initialization and migration evidence for an exact
published tag. Its reports distinguish the observed runner OS from the approved
minimum-OS requirement.
