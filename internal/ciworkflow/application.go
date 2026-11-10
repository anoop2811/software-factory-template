package ciworkflow

import (
	"fmt"
	"strconv"
	"strings"
)

// Application selects the union of application tools. Empty versions omit a pack.
// Maven's selected command remains in factory.yaml and the generated Makefile;
// Gradle requires the additional checks its ordinary check command does not run.
type Application struct {
	GoVersion, NodeVersion, JavaVersion string
	JavaBuildTool                       string
	MutationDirectory                   string
}

// Initialized composes one ordinary job around the unchanged installed checks.
// Versions and paths are YAML data, never interpolated into shell programs.
// docs/adr/0146-initialized-installed-ci.md:57.
func Initialized(release Release, application Application) string {
	var workflow strings.Builder
	workflow.WriteString(preparation(release))
	if application.GoVersion != "" {
		fmt.Fprintf(&workflow, goSetup, strconv.Quote(application.GoVersion))
	}
	if application.NodeVersion != "" {
		fmt.Fprintf(&workflow, nodeSetup, strconv.Quote(application.NodeVersion))
	}
	if application.JavaVersion != "" {
		fmt.Fprintf(&workflow, javaSetup, strconv.Quote(application.JavaVersion))
	}
	workflow.WriteString(actionlintInstall)
	if application.GoVersion != "" {
		workflow.WriteString(goToolsInstall)
	}
	workflow.WriteString(installedChecks)
	workflow.WriteString(actionlintCheck)
	if application.GoVersion != "" {
		workflow.WriteString(goSecurity)
	}
	if application.NodeVersion != "" || application.JavaVersion != "" {
		workflow.WriteString(osvScan)
	}
	gradle := application.JavaVersion != "" && application.JavaBuildTool == "gradle"
	if gradle {
		workflow.WriteString(gradleChecks)
	}
	if application.GoVersion != "" {
		directory := application.MutationDirectory
		if directory == "" {
			directory = "."
		}
		fmt.Fprintf(&workflow, goMutation, strconv.Quote(directory))
	}
	if application.NodeVersion != "" {
		workflow.WriteString(nodeMutation)
	}
	if gradle {
		workflow.WriteString(gradleMutation)
	}
	workflow.WriteString(mockGolden)
	return workflow.String()
}

// Immutable actions and official checksums: docs/adr/0146-initialized-installed-ci.md:142.
// Setup disables implicit caches and the actions' default job-token inputs.
const goSetup = `      - name: Set up application Go
        uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version: %s
          cache: false
          token: ''
`

const nodeSetup = `      - name: Set up application Node.js
        uses: actions/setup-node@949feb2413d6458794dcd2491c4babbbce0c15c1 # v7.1.0
        with:
          node-version: %s
          package-manager-cache: false
          token: ''
      - name: Install application Node dependencies
        shell: bash
        run: npm ci
`

const javaSetup = `      - name: Set up application JDK
        uses: actions/setup-java@77dd5b2ce225ea11469928ac317850245a74ae78 # v6.2.0
        with:
          distribution: temurin
          java-version: %s
          cache-jdk: false
          token: ''
`

const actionlintInstall = `      - name: Install pinned actionlint
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
`

const goToolsInstall = `      - name: Install pinned Go application tools
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
`

// Retain the existing narrow parser exception for the optional review workflow.
// docs/adr/0146-initialized-installed-ci.md:194.
const actionlintCheck = `      - name: Check project workflows
        shell: bash
        run: actionlint -ignore 'unexpected key "cache-mode" for "job" section'
`

const goSecurity = `      - name: Check Go application security
        shell: bash
        run: |
          set -euo pipefail
          gosec ./...
          govulncheck ./...
`

const osvScan = `      - name: Scan application dependency vulnerabilities
        uses: google/osv-scanner-action/osv-scanner-action@b1d267aea5211fa648d9d442fa64b90dbdcbdd36 # v2.7.0
        with:
          scan-args: |-
            scan
            source
            --recursive
            ./
`

const gradleChecks = `      - name: Check Gradle compilation and security
        shell: bash
        run: |
          set -euo pipefail
          ./gradlew --no-daemon compileJava compileTestJava
          ./gradlew --no-daemon spotbugsMain
`

// Only mutation analysis is advisory. Gremlins accepts a directory, not /....
// docs/adr/0146-initialized-installed-ci.md:70.
const goMutation = `      - name: Go mutation analysis
        continue-on-error: true
        env:
          FACTORY_MUTATION_DIRECTORY: %s
        shell: bash
        run: gremlins unleash "$FACTORY_MUTATION_DIRECTORY" --dry-run
`

const nodeMutation = `      - name: TypeScript mutation testing
        continue-on-error: true
        shell: bash
        run: npx -p @stryker-mutator/core@9.6.1 -p @stryker-mutator/vitest-runner@9.6.1 stryker run
`

const gradleMutation = `      - name: Gradle mutation testing
        continue-on-error: true
        shell: bash
        run: ./gradlew --no-daemon pitest
`

const mockGolden = `      - name: Mock golden evaluation
        if: ${{ github.event_name == 'pull_request' }}
        shell: bash
        run: ./factory eval golden --harness=mock
`
