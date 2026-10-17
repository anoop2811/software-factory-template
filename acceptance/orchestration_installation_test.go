package acceptance_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These legacy source files must become inert initializer data or generated
// adapters. A poisoned committed target catches fallbacks without changing the
// qualified installed descriptor after activation.
// per docs/adr/0099-native-factory-orchestration.md:38
var orchestrationInstallationLegacyPaths = []string{
	"scripts/sync-opencode.sh", "scripts/sync-claude.sh", "scripts/sync-codex.sh",
	"scripts/pre-push-check.sh", "scripts/prereq-check.sh", "scripts/harness-structural-eval.sh",
	"scripts/golden-task-eval.sh", "scripts/adversarial-review.sh",
	"eval/runners/mock.sh", "eval/runners/example-harness.sh", "eval/runners/claude.sh", "eval/runners/codex.sh", "eval/runners/opencode.sh",
	"scripts/lib/config.sh", "scripts/lib/roles.sh", "scripts/lib/events.sh", "scripts/lib/color.sh",
	"scripts/lib/timing.sh", "scripts/lib/hookspath.sh", "scripts/lib/budget-config.sh",
}

func orchestrationInstallationPrepared() ([]byte, installationFixtureData) {
	GinkgoHelper()
	binary := installationBuildDriver()
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	work, err := os.MkdirTemp("", "factory-native-orchestration-installation-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, work)
	source := filepath.Join(work, "committed-poisoned-source")
	_, _, _, _ = installationTargetSnapshot(repository, source)
	for _, path := range orchestrationInstallationLegacyPaths {
		mode := os.FileMode(0755)
		if strings.HasPrefix(path, "scripts/lib/") {
			mode = 0644
		}
		writeFixture(filepath.Join(source, path), []byte("#!/bin/sh\nprintf legacy-orchestration-executed >&2\nprintf called > \"${ORCHESTRATION_LEGACY_MARKER:-/dev/null}\"\nexit 97\n"), mode)
	}
	imageGit(source, "add", "--force", "--all")
	imageGit(source, "commit", "--quiet", "-m", "test: commit inert orchestration poison")
	revision := strings.TrimSpace(imageGit(source, "rev-parse", "HEAD"))
	assets, payload := installationGitCatalog(source, revision)
	targetBinary := filepath.Join(work, "target-runtime")
	args := []string{"build", "-o", targetBinary, "./cmd/factory"}
	if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
		args = append([]string{"build", "-race"}, args[1:]...)
	}
	built := imageProcess(source, nil, "go", args...)
	Expect(built.status).To(BeZero(), "exact poisoned-source commit must compile: %+v", built)
	compiled, err := os.ReadFile(targetBinary)
	Expect(err).NotTo(HaveOccurred())
	var baselines []imageBaselineOracle
	var baseline installationReference
	for _, reference := range []installationReference{
		{profile: "v0.1.6", revision: "b71ecc32e07ecd87eb330ba8e497c86612f92acd"},
		{profile: "bash-baseline", revision: "76952eaa63aebd1ecd282f5ab51dd7c3627cb497"},
	} {
		reference.assets, reference.payload = installationGitCatalog(repository, reference.revision)
		baselines = append(baselines, imageBaselineOracle{reference.profile, reference.revision, reference.assets})
		if reference.profile == "bash-baseline" {
			baseline = reference
		}
	}
	manifest := imageManifestOracle{1, "installation_source_image", "committed_source_and_baseline_references", "v9.8.0", runtime.GOOS + "/" + runtime.GOARCH, revision, false, assets, baselines}
	archive := filepath.Join(work, "orchestration-source-image.tar.gz")
	writeFixture(archive, stageArchive(imageFixtureEntries(manifest, payload, compiled)), 0600)
	return binary, installationLegacyFixture(binary, baseline, archive, revision)
}

func orchestrationInstalledEnvironment(root string) map[string]string {
	GinkgoHelper()
	tools := filepath.Join(filepath.Dir(root), "orchestration-tools")
	marker := filepath.Join(filepath.Dir(root), "forbidden-orchestration-child")
	for _, name := range []string{"python", "python3", "curl", "wget"} {
		writeFixture(filepath.Join(tools, name), []byte("#!/bin/sh\nprintf called > \"$ORCHESTRATION_LEGACY_MARKER\"\nexit 97\n"), 0700)
	}
	writeFixture(filepath.Join(tools, "make"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$ORCHESTRATION_MAKE_ARGUMENTS\"\nexit 0\n"), 0700)
	writeFixture(filepath.Join(tools, "golangci-lint"), []byte("#!/bin/sh\nprintf 'golangci-lint fixture\\n'\n"), 0700)
	for _, name := range []string{"opencode", "claude", "codex"} {
		writeFixture(filepath.Join(tools, name), []byte("#!/bin/sh\ncase \"${1-}\" in --version|-v) printf 'fixture 1.0\\n'; exit 0 ;; esac\nprintf '%s\\000' \"$0\" \"$PWD\" \"$@\" >> \"$ORCHESTRATION_NATIVE_ARGUMENTS\"\nprintf 'local fake harness output\\n'\nexit 37\n"), 0700)
	}
	return map[string]string{
		"PATH": tools + string(os.PathListSeparator) + os.Getenv("PATH"), "FACTORY_BRIDGE_PROTOCOL": "",
		"FACTORY_CONFIG": filepath.Join(root, "factory.yaml"), "FACTORY_AGENT_ROLE": "spec-writer", "NO_COLOR": "1",
		"ORCHESTRATION_LEGACY_MARKER": marker, "ORCHESTRATION_MAKE_ARGUMENTS": filepath.Join(filepath.Dir(root), "make-arguments"),
		"ORCHESTRATION_NATIVE_ARGUMENTS": filepath.Join(filepath.Dir(root), "native-arguments"), "REVIEW_API_KEY": "local-empty-diff-key",
	}
}

var _ = Describe("Whole installation upgrade native orchestration adapters", Ordered, func() {
	var binary []byte
	var prepared installationFixtureData
	BeforeAll(func() { binary, prepared = orchestrationInstallationPrepared() })
	fixture := func(migration string) installationFixtureData {
		GinkgoHelper()
		installed := installationCloneLegacyFixture(binary, prepared)
		installationApply(installed, migration)
		return installed
	}

	// per docs/adr/0099-native-factory-orchestration.md:38
	// per docs/adr/0099-native-factory-orchestration.md:99
	// per docs/adr/0099-native-factory-orchestration.md:104
	// per docs/adr/0099-native-factory-orchestration.md:115
	It("runs actual installed orchestration paths while all legacy source implementations and Python remain poisoned", func() {
		installed := fixture("native-orchestration-routes")
		environment := orchestrationInstalledEnvironment(installed.root)
		writeFixture(filepath.Join(installed.root, "factory.yaml"), []byte("language_packs:\nreview_lane: off\nlocal_hooks:\ncost_profile: economy\n"), 0600)
		for _, harness := range []string{"opencode", "claude", "codex"} {
			out := imageProcess(installed.root, environment, filepath.Join(installed.root, "scripts/sync-"+harness+".sh"), "--ignored", "extra", "--help")
			Expect(out.status).To(BeZero(), "installed %s sync must discard old operands: %+v", harness, out)
			Expect(out.stdout).To(ContainSubstring("sync-" + harness + ":"))
			Expect(out.stderr).NotTo(ContainSubstring("legacy-orchestration-executed"))
		}
		Expect(syncJSON(syncRead(installed.root, ".claude/settings.json"))).To(HaveKey("permissions"))
		Expect(syncRead(installed.root, ".codex/agents/implementer.toml")).To(ContainSubstring("FACTORY_AGENT_ROLE=implementer"))
		for _, request := range []struct {
			path string
			args []string
		}{
			{"scripts/prereq-check.sh", nil}, {"scripts/pre-push-check.sh", nil},
			{"scripts/harness-structural-eval.sh", []string{"--harness=opencode"}},
		} {
			out := imageProcess(installed.root, environment, filepath.Join(installed.root, request.path), request.args...)
			Expect(out.status).To(BeZero(), "%s: %+v", request.path, out)
			Expect(out.stdout + out.stderr).NotTo(ContainSubstring("legacy-orchestration-executed"))
		}
		makeArguments, err := os.ReadFile(environment["ORCHESTRATION_MAKE_ARGUMENTS"])
		Expect(err).NotTo(HaveOccurred())
		Expect(string(makeArguments)).To(Equal("check\ncheck-drift\n"))
		orchestrationTask(installed.root, "native-installation-only")
		out := imageProcess(installed.root, environment, filepath.Join(installed.root, "scripts/golden-task-eval.sh"), "--runs=1")
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("native-installation-only: 1/1 passed"))
		Expect(orchestrationReadResults(installed.root, "current").Tasks).NotTo(BeEmpty())
		for _, harness := range []string{"mock", "claude", "codex", "opencode"} {
			work := filepath.Join(filepath.Dir(installed.root), "runner work "+harness)
			writeFixture(filepath.Join(work, "task.md"), []byte("Create answer.txt containing FIXED.\n"), 0600)
			out := imageProcess(installed.root, environment, filepath.Join(installed.root, "eval/runners/"+harness+".sh"), work)
			Expect(out).To(Equal(cliResult{"", "", 0}), harness)
			if harness == "mock" {
				answer, err := os.ReadFile(filepath.Join(work, "answer.txt"))
				Expect(err).NotTo(HaveOccurred())
				Expect(strings.TrimSpace(string(answer))).To(Equal("FIXED"))
			} else {
				log, err := os.ReadFile(filepath.Join(work, harness+"-runner.log"))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(log)).To(Equal("local fake harness output\n"))
			}
		}
		empty := filepath.Join(filepath.Dir(installed.root), "empty diff.patch")
		writeFixture(empty, nil, 0600)
		out = imageProcess(installed.root, environment, filepath.Join(installed.root, "scripts/adversarial-review.sh"), empty)
		Expect(out).To(Equal(cliResult{"_No reviewable change in this diff._\n", "", 0}))
		_, err = os.Stat(environment["ORCHESTRATION_LEGACY_MARKER"])
		Expect(os.IsNotExist(err)).To(BeTrue(), "no poisoned interpreter, network utility or source implementation may run")
	})

	// per docs/adr/0099-native-factory-orchestration.md:138
	// per docs/adr/0099-native-factory-orchestration.md:140
	// per docs/adr/0099-native-factory-orchestration.md:145
	// per docs/adr/0099-native-factory-orchestration.md:148
	It("sources generated helper bridges with their inert and intentional effects and literal caller-local values", func() {
		installed := fixture("native-orchestration-helpers")
		environment := orchestrationInstalledEnvironment(installed.root)
		writeFixture(filepath.Join(installed.root, "factory.yaml"), []byte("budget_enabled: true\nbudget_max_attempts: 7\ncost_profile: economy\ncodex_economy_model: yaml-model\n"), 0600)
		imageGit(installed.root, "config", "core.hooksPath", ".githooks")
		log := filepath.Join(filepath.Dir(installed.root), "caller event log")
		probe := `set -eu
FACTORY_CONFIG="$2"; FACTORY_BRIDGE_PROTOCOL=caller
readonly FACTORY_CONFIG FACTORY_BRIDGE_PROTOCOL
unset FACTORY_BUDGET_ENABLED
. "$1/scripts/lib/events.sh"
. "$1/scripts/lib/timing.sh"
. "$1/scripts/lib/hookspath.sh"
[ -z "${FACTORY_BUDGET_ENABLED+x}" ]
FACTORY_EVENT_LOG="$3"; FACTORY_EVENT_MAX_LINES=1
factory_log_event caller-gate 'literal $(touch HELPER_INJECTION)'
factory_duration 10 75; printf '\000'
hookspath_status "$1"; printf '\000'
C_RESET=before; C_BOLD=before; C_DIM=before; C_YELLOW=before; C_GREEN=before; C_RED=before
. "$1/scripts/lib/color.sh"
[ -z "$C_RESET$C_BOLD$C_DIM$C_YELLOW$C_GREEN$C_RED" ]
CODEX_ECONOMY_MODEL=caller-model
. "$1/scripts/lib/budget-config.sh"
printf '%s\000%s\000%s\000' "$FACTORY_BUDGET_ENABLED" "$FACTORY_BUDGET_MAX_ATTEMPTS" "$FACTORY_BRIDGE_PROTOCOL"
factory_budget_model codex refactorer; printf '\000'
/bin/sh -c 'printf "%s\000%s\000" "$FACTORY_BUDGET_ENABLED" "$CODEX_ECONOMY_MODEL"'
C_RESET='<R>'; C_BOLD='<B>'; C_YELLOW='<Y>'
action_box 'caller title' 'literal line'
`
		out := imageProcess(installed.root, environment, "/bin/bash", "-c", probe, "installed-helper-probe", installed.root, environment["FACTORY_CONFIG"], log)
		expected := "1m 5s\x00armed\t" + filepath.Join(installed.root, ".githooks/pre-push") + "\x00true\x007\x00caller\x00caller-model\x00true\x00caller-model\x00<Y><B>┌─ caller title <R>\n<Y>│<R> literal line\n<Y>└─<R>\n"
		Expect(out).To(Equal(cliResult{expected, "", 0}))
		event, err := os.ReadFile(log)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(event)).To(HaveSuffix("\tcaller-gate\tliteral $(touch HELPER_INJECTION)\n"))
		_, err = os.Stat(filepath.Join(installed.root, "HELPER_INJECTION"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		_, err = os.Stat(environment["ORCHESTRATION_LEGACY_MARKER"])
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})
