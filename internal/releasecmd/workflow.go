package releasecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/output"
)

const ciWorkflowUsage = "Usage: factory ci workflow --go-candidate TAG --origin-target OS/ARCH --installer-revision FULL_COMMIT_SHA\n\nRender a workflow to stdout for review; no project files are changed.\n"

type ciWorkflowOptions struct {
	tag, origin, installerRevision string
}

// The public renderer consumes explicit data and the existing output boundary.
// docs/adr/0145-installed-ci-workflow-rendering.md:29.
func runCIWorkflow(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	option, help, err := parseCIWorkflow(args)
	if err != nil {
		_ = writeDiagnostic(stderr, "factory ci workflow: %s; require --go-candidate, --origin-target and --installer-revision\n", err)
		return 2
	}
	value := ciWorkflowUsage
	if !help {
		value = fmt.Sprintf(ciWorkflowTemplate, option.tag, option.origin, option.installerRevision, officialRepository)
	}
	if output.WriteEvent(ctx, stdout, []byte(value)) != nil {
		_ = writeDiagnostic(stderr, "factory ci workflow: output failed\n")
		return 1
	}
	return 0
}

func parseCIWorkflow(args []string) (ciWorkflowOptions, bool, error) {
	var option ciWorkflowOptions
	seen := map[string]bool{}
	help := false
	for index := 0; index < len(args); index++ {
		name, value, supplied := strings.Cut(args[index], "=")
		if name == "-h" {
			name = "--help"
		}
		if seen[name] {
			return option, false, errors.New("duplicate workflow argument")
		}
		seen[name] = true
		switch name {
		case "--help":
			if supplied {
				return option, false, errors.New("unexpected help value")
			}
			help = true
		case "--go-candidate", "--origin-target", "--installer-revision":
			if !supplied {
				index++
				if index == len(args) {
					return option, false, errors.New("missing workflow argument")
				}
				value = args[index]
			}
			switch name {
			case "--go-candidate":
				if !exactRelease(value) {
					return option, false, errors.New("invalid workflow release")
				}
				option.tag = value
			case "--origin-target":
				if !supportedTarget(value) {
					return option, false, errors.New("invalid workflow origin target")
				}
				option.origin = value
			case "--installer-revision":
				if !revisionPattern.MatchString(value) {
					return option, false, errors.New("invalid installer revision")
				}
				option.installerRevision = value
			}
		default:
			return option, false, errors.New("unknown workflow argument")
		}
	}
	if !help && (option.tag == "" || option.origin == "" || option.installerRevision == "") {
		return option, false, errors.New("missing workflow selection")
	}
	return option, help, nil
}

// Each non-PR event has its own concurrency group, preserving publication runs.
// Selectors are environment data; only preparation receives the read-only token.
// docs/adr/0145-installed-ci-workflow-rendering.md:50.
const ciWorkflowTemplate = `name: Installed factory checks

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
      # Add application-specific tool and dependency setup steps here.
      # Preparation requires a clean tracked checkout, so setup follows it.
      - name: Check installed factory and application
        shell: bash
        run: |
          set -euo pipefail
          ./factory selftest
          ./factory check
          ./factory eval structural --harness=opencode
          ./factory eval structural --harness=claude
          ./factory eval structural --harness=codex
`
