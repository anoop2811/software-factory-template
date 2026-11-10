// Package ciworkflow renders installed-factory workflows from explicit inputs.
package ciworkflow

import "fmt"

// Release identifies the installer and runtime selected by the calling owner.
// This value is rendering data, not proof of release authentication.
type Release struct {
	Tag, OriginTarget, SourceRevision, Repository string
}

// Generic retains the public renderer's exact three-step workflow.
// docs/adr/0145-installed-ci-workflow-rendering.md:29.
func Generic(release Release) string {
	return preparation(release) + applicationSetupComment + installedChecks
}

func preparation(release Release) string {
	return fmt.Sprintf(preparationTemplate, release.Tag, release.OriginTarget, release.SourceRevision, release.Repository)
}

// Selectors are environment data; only preparation receives the read-only token.
// docs/adr/0145-installed-ci-workflow-rendering.md:50.
const preparationTemplate = `name: Installed factory checks

on:
  pull_request:
  push:
  workflow_dispatch:

permissions:
  contents: read

concurrency:
  group: factory-${{ github.workflow }}-${{ github.event_name }}-${{ github.event_name == 'pull_request' && github.event.pull_request.number || github.run_id }}
  cancel-in-progress: ${{ github.event_name == 'pull_request' }}

jobs:
  factory:
    if: ${{ github.event_name != 'push' || github.ref == format('refs/heads/{0}', github.event.repository.default_branch) }}
    runs-on: ubuntu-24.04
    timeout-minutes: 20
    steps:
      - name: Check out the project
        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
          persist-credentials: false
      - name: Prepare installed factory
        env:
          GH_TOKEN: ${{ github.token }}
          FACTORY_RELEASE_TAG: '%s'
          FACTORY_ORIGIN_TARGET: '%s'
          FACTORY_INSTALLER_REVISION: '%s'
        shell: bash
        run: |
          set -euo pipefail
          umask 077
          factory_installer="$(mktemp "${RUNNER_TEMP}/factory-installer.XXXXXXXX")"
          trap 'rm -f -- "$factory_installer"' EXIT
          curl --fail --silent --show-error --location \
            --proto '=https' --proto-redir '=https' --tlsv1.2 \
            "https://raw.githubusercontent.com/%s/${FACTORY_INSTALLER_REVISION}/install.sh" \
            --output "$factory_installer"
          test -s "$factory_installer"
          sh "$factory_installer" ci prepare \
            --go-candidate "$FACTORY_RELEASE_TAG" \
            --origin-target "$FACTORY_ORIGIN_TARGET" \
            --ephemeral
`

const applicationSetupComment = `      # Add application-specific tool and dependency setup steps here.
      # Preparation requires a clean tracked checkout, so setup follows it.
`

const installedChecks = `      - name: Check installed factory and application
        shell: bash
        run: |
          set -euo pipefail
          ./factory selftest
          ./factory check
          ./factory eval structural --harness=opencode
          ./factory eval structural --harness=claude
          ./factory eval structural --harness=codex
`
