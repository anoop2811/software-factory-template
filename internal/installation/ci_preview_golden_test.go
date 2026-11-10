package installation

// Captured by executing ciworkflow.Initialized from the unchanged production
// sources at immutable 8f2c6ae27f47a1ddbffc76cc10bdceffd424f2c4 in a fresh full
// --no-local private clone. The only added source was an untracked Ginkgo capture
// test; it called the exported historical renderer without overrides.
//
// Capture command (Go 1.27.3, GOTOOLCHAIN=local, GORACE=atexit_sleep_ms=0):
// go test -race -count=1 -timeout=2m ./internal/ciworkflow -run TestHistoricalWorkflowCapture -ginkgo.fail-on-empty -ginkgo.no-color
// Captured 2026-11-10; SHA256: a1d7092024b0e226146f9441aba14316090aba50132f171c907b39bcaf11ecb8
// Release: v0.3.0, linux/amd64, the immutable revision above, official repository.
// Application: Go 1.26, Node 24, Java 25, Gradle, mutation directory ./internal.
// These are literal captured bytes, not a projection assembled by this oracle.
// per docs/adr/0147-installed-ci-migration-preview.md:209
const ciPreviewAllGradleHistoricalGolden = `name: Installed factory checks

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
          FACTORY_RELEASE_TAG: 'v0.3.0'
          FACTORY_ORIGIN_TARGET: 'linux/amd64'
          FACTORY_INSTALLER_REVISION: '8f2c6ae27f47a1ddbffc76cc10bdceffd424f2c4'
        shell: bash
        run: |
          set -euo pipefail
          umask 077
          factory_installer="$(mktemp "${RUNNER_TEMP}/factory-installer.XXXXXXXX")"
          trap 'rm -f -- "$factory_installer"' EXIT
          curl --fail --silent --show-error --location \
            --proto '=https' --proto-redir '=https' --tlsv1.2 \
            "https://raw.githubusercontent.com/anoop2811/software-factory-template/${FACTORY_INSTALLER_REVISION}/install.sh" \
            --output "$factory_installer"
          test -s "$factory_installer"
          sh "$factory_installer" ci prepare \
            --go-candidate "$FACTORY_RELEASE_TAG" \
            --origin-target "$FACTORY_ORIGIN_TARGET" \
            --ephemeral
      - name: Set up application Go
        uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version: "1.26"
          cache: false
          token: ''
      - name: Set up application Node.js
        uses: actions/setup-node@949feb2413d6458794dcd2491c4babbbce0c15c1 # v7.1.0
        with:
          node-version: "24"
          package-manager-cache: false
          token: ''
      - name: Install application Node dependencies
        shell: bash
        run: npm ci
      - name: Set up application JDK
        uses: actions/setup-java@77dd5b2ce225ea11469928ac317850245a74ae78 # v6.2.0
        with:
          distribution: temurin
          java-version: "25"
          cache-jdk: false
          token: ''
      - name: Install pinned actionlint
        shell: bash
        run: |
          set -euo pipefail
          umask 077
          factory_tools="$(mktemp -d "${RUNNER_TEMP}/factory-actionlint.XXXXXXXX")"
          curl --fail --silent --show-error --location \
            --proto '=https' --proto-redir '=https' --tlsv1.2 \
            https://github.com/rhysd/actionlint/releases/download/v1.7.12/actionlint_1.7.12_linux_amd64.tar.gz \
            --output "$factory_tools/actionlint.tar.gz"
          printf '%s  %s\n' '8aca8db96f1b94770f1b0d72b6dddcb1ebb8123cb3712530b08cc387b349a3d8' "$factory_tools/actionlint.tar.gz" | sha256sum --check --strict
          tar -xzf "$factory_tools/actionlint.tar.gz" -C "$factory_tools" actionlint
          rm -- "$factory_tools/actionlint.tar.gz"
          printf '%s\n' "$factory_tools" >> "$GITHUB_PATH"
      - name: Install pinned Go application tools
        shell: bash
        run: |
          set -euo pipefail
          umask 077
          factory_tools="$(mktemp -d "${RUNNER_TEMP}/factory-go-tools.XXXXXXXX")"
          curl --fail --silent --show-error --location \
            --proto '=https' --proto-redir '=https' --tlsv1.2 \
            https://github.com/golangci/golangci-lint/releases/download/v2.15.0/golangci-lint-2.15.0-linux-amd64.tar.gz \
            --output "$factory_tools/golangci-lint.tar.gz"
          printf '%s  %s\n' '9b7fe6a6bf6334b98decf8d86b11415ea7889afdabdd1959fdce23af2a9ebcea' "$factory_tools/golangci-lint.tar.gz" | sha256sum --check --strict
          tar -xzf "$factory_tools/golangci-lint.tar.gz" -C "$factory_tools" --strip-components=1 golangci-lint-2.15.0-linux-amd64/golangci-lint
          rm -- "$factory_tools/golangci-lint.tar.gz"
          GOBIN="$factory_tools" GOTOOLCHAIN=auto go install github.com/securego/gosec/v2/cmd/gosec@v2.30.0
          GOBIN="$factory_tools" GOTOOLCHAIN=auto go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
          GOBIN="$factory_tools" GOTOOLCHAIN=auto go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0
          printf '%s\n' "$factory_tools" >> "$GITHUB_PATH"
      - name: Check installed factory and application
        shell: bash
        run: |
          set -euo pipefail
          ./factory selftest
          ./factory check
          ./factory eval structural --harness=opencode
          ./factory eval structural --harness=claude
          ./factory eval structural --harness=codex
      - name: Check project workflows
        shell: bash
        run: actionlint -ignore 'unexpected key "cache-mode" for "job" section'
      - name: Check Go application security
        shell: bash
        run: |
          set -euo pipefail
          gosec ./...
          govulncheck ./...
      - name: Scan application dependency vulnerabilities
        uses: google/osv-scanner-action/osv-scanner-action@b1d267aea5211fa648d9d442fa64b90dbdcbdd36 # v2.7.0
        with:
          scan-args: |-
            scan
            source
            --recursive
            ./
      - name: Check Gradle compilation and security
        shell: bash
        run: |
          set -euo pipefail
          ./gradlew --no-daemon compileJava compileTestJava
          ./gradlew --no-daemon spotbugsMain
      - name: Go mutation analysis
        continue-on-error: true
        env:
          FACTORY_MUTATION_DIRECTORY: "./internal"
        shell: bash
        run: gremlins unleash "$FACTORY_MUTATION_DIRECTORY" --dry-run
      - name: TypeScript mutation testing
        continue-on-error: true
        shell: bash
        run: npx -p @stryker-mutator/core@9.6.1 -p @stryker-mutator/vitest-runner@9.6.1 stryker run
      - name: Gradle mutation testing
        continue-on-error: true
        shell: bash
        run: ./gradlew --no-daemon pitest
      - name: Mock golden evaluation
        if: ${{ github.event_name == 'pull_request' }}
        shell: bash
        run: ./factory eval golden --harness=mock
`
