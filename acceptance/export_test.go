package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const exportBaseline = "2e3609bfbb22698561167747caf175e3dbdb9e74"

var exportKeys = append(append([]string{}, configurationKeys...), "REVIEW_REASONING_EFFORT", "REVIEW_OPENROUTER_PROVIDER", "REVIEW_MAX_TOKENS")

const exportProbe = `set -euo pipefail
for probe_key in $8; do unset "$probe_key"; done
. "$1"
export_probe_apply() {
  if [ "$5" = legacy ]; then factory_config_load_legacy "$6" "$7"; else factory_config_export; fi
}
export_probe_emit() {
  local probe_key="$1"
  printf '%s\000%s\000' "${!probe_key+x}" "${!probe_key-}"
  bash -c 'probe_key="$1"; printf '\''%s\000%s\000'\'' "${!probe_key+x}" "${!probe_key-}"' _ "$probe_key"
}
if [ "$4" = local ] || [ "$4" = readonly ]; then
  export_probe_scope() {
    local "$2=$3"
    if [ "$4" = readonly ]; then readonly "$2"; fi
    export_probe_apply "$@"
    export_probe_emit "$2"
  }
  export_probe_scope "$@"
else
  if [ "$4" = environment ]; then export "$2=$3"; fi
  export_probe_apply "$@"
  export_probe_emit "$2"
fi
`

func exportShimPath() string {
	GinkgoHelper()
	path, err := filepath.Abs(filepath.Join("..", "runtime", "shell", "config.sh"))
	Expect(err).NotTo(HaveOccurred())
	return path
}

func exportSnapshot(root, cwd, source, key, caller, scope, operation, preserved string) configurationSnapshot {
	GinkgoHelper()
	result := readerProcess(cwd, map[string]string{"FACTORY_CONFIG": filepath.Join(cwd, "factory.yaml"), "FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")},
		"bash", "-c", exportProbe, "export-probe", source, key, caller, scope, operation, filepath.Join(cwd, "factory.config"), preserved, strings.Join(exportKeys, " "))
	Expect(result.status).To(Equal(0), result.stderr)
	Expect(result.stderr).To(BeEmpty())
	fields := strings.Split(result.stdout, "\x00")
	Expect(fields).To(HaveLen(5), "unexpected parent/child state: %q", result.stdout)
	Expect(fields[0]).To(BeElementOf("", "x"))
	Expect(fields[2]).To(BeElementOf("", "x"))
	Expect(fields[4]).To(BeEmpty())
	return configurationSnapshot{fields[0] == "x", fields[2] == "x", fields[1], fields[3]}
}

func correctedExportSource(root, revision string) string {
	GinkgoHelper()
	original, err := os.ReadFile(historicalConfiguration(root, revision))
	Expect(err).NotTo(HaveOccurred())
	directory := filepath.Join("..", "docs", "migration", "corrections")
	metadata, err := os.ReadFile(filepath.Join(directory, "EX-001.json"))
	Expect(err).NotTo(HaveOccurred())
	var correction configurationCorrection
	Expect(json.Unmarshal(metadata, &correction)).To(Succeed())
	patch, err := os.ReadFile(filepath.Join(directory, "EX-001.patch"))
	Expect(err).NotTo(HaveOccurred())
	path, err := applyConfigurationCorrection(root, original, patch, "100755", correction)
	Expect(err).NotTo(HaveOccurred())
	return path
}

var _ = Describe("G1 configuration export plans", func() {
	// per docs/adr/0056-go-configuration-export-plans.md:17
	It("preserves ordered legacy assignments before the YAML overlay", func() {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		writeFixture(path, []byte("model_provider: YAML\nreview_model: \"\"\nreview_reasoning_effort: none\nreview_openrouter_provider: deepinfra\n"), 0600)
		writeFixture(filepath.Join(cwd, "factory.config"), []byte("MODEL_PROVIDER=first\nMODEL_PROVIDER=last\nREVIEW_MODEL='legacy # value'\n"), 0600)
		expected := "FACTORY_CONFIG_PLAN_V1\nset\tMODEL_PROVIDER\tfirst\nset\tMODEL_PROVIDER\tlast\nset\tREVIEW_MODEL\tlegacy # value\nset\tMODEL_PROVIDER\tYAML\nset\tREVIEW_REASONING_EFFORT\tnone\nset\tREVIEW_OPENROUTER_PROVIDER\tdeepinfra\nEND\n"
		Expect(bridgeReader(root, cwd, map[string]string{"FACTORY_CONFIG": path, "MODEL_PROVIDER": "child environment must not imply preservation"}, "config", "export")).To(Equal(cliResult{expected, "", 0}))
	})

	// per docs/adr/0056-go-configuration-export-plans.md:20
	It("uses explicit caller names to export without serializing caller values", func() {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		writeFixture(path, []byte("model_provider: YAML\nreview_reasoning_effort: none\n"), 0600)
		writeFixture(filepath.Join(cwd, "factory.config"), []byte("MODEL_PROVIDER=legacy\nREVIEW_REASONING_EFFORT=high\n"), 0600)
		expected := "FACTORY_CONFIG_PLAN_V1\nexport\tMODEL_PROVIDER\nexport\tREVIEW_REASONING_EFFORT\nexport\tMODEL_PROVIDER\nexport\tREVIEW_REASONING_EFFORT\nEND\n"
		Expect(bridgeReader(root, cwd, map[string]string{"FACTORY_CONFIG": path}, "config", "export", "MODEL_PROVIDER", "REVIEW_REASONING_EFFORT")).To(Equal(cliResult{expected, "", 0}))
	})

	// per docs/adr/0056-go-configuration-export-plans.md:44
	It("keeps empty and tab-bearing legacy values as literal plan data", func() {
		root, cwd := fixture()
		path := filepath.Join(cwd, "legacy settings")
		writeFixture(path, []byte("REVIEW_MODEL='\tleft\t\tright\t'\nMODEL_PROVIDER=\nUNEXPECTED_KEY=ignored\n"), 0600)
		expected := "FACTORY_CONFIG_PLAN_V1\nset\tREVIEW_MODEL\t\tleft\t\tright\t\nset\tMODEL_PROVIDER\t\nEND\n"
		Expect(bridgeReader(root, cwd, nil, "config", "legacy", path)).To(Equal(cliResult{expected, "", 0}))
	})

	It("distinguishes missing standalone legacy input from absent export inputs", func() {
		root, cwd := fixture()
		result := bridgeReader(root, cwd, nil, "config", "legacy", filepath.Join(cwd, "missing"))
		Expect(result.status).To(Equal(1))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
		Expect(bridgeReader(root, cwd, map[string]string{"FACTORY_CONFIG": filepath.Join(cwd, "missing.yaml")}, "config", "export")).To(Equal(cliResult{"FACTORY_CONFIG_PLAN_V1\nEND\n", "", 0}))
	})

	DescribeTable("refuses malformed private operands", func(args []string) {
		root, cwd := fixture()
		result := bridgeReader(root, cwd, nil, args...)
		Expect(result.status).To(Equal(2))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
	}, Entry("unknown preserved export key", []string{"config", "export", "PATH"}),
		Entry("missing legacy file operand", []string{"config", "legacy"}),
		Entry("unknown preserved legacy key", []string{"config", "legacy", "missing", "UNEXPECTED_SETTING"}))
})

var _ = Describe("G1 sourceable configuration export", func() {
	It("matches all 17 current keys against immutable Bash and independent expected state", func() {
		root, cwd := fixture()
		baseline := historicalConfiguration(root, exportBaseline)
		for _, key := range exportKeys {
			writeFixture(filepath.Join(cwd, "factory.yaml"), []byte(strings.ToLower(key)+": YAML\n"), 0600)
			writeFixture(filepath.Join(cwd, "factory.config"), []byte(key+"=legacy\n"), 0600)
			sources := []string{exportShimPath()}
			// REVIEW_MAX_TOKENS was added after the immutable baseline used by
			// this parity fixture; the candidate must carry it forward while
			// historical keys remain byte-for-byte comparable.
			if key != "REVIEW_MAX_TOKENS" {
				sources = append([]string{baseline}, sources...)
			}
			for _, source := range sources {
				Expect(exportSnapshot(root, cwd, source, key, "", "unset", "export", "")).To(Equal(configurationSnapshot{true, true, "YAML", "YAML"}), key)
				Expect(exportSnapshot(root, cwd, source, key, "", "environment", "export", "")).To(Equal(configurationSnapshot{true, true, "", ""}), key)
			}
			Expect(os.Remove(filepath.Join(cwd, "factory.yaml"))).To(Succeed())
			sources = []string{exportShimPath()}
			if key != "REVIEW_MAX_TOKENS" {
				sources = append([]string{baseline}, sources...)
			}
			for _, source := range sources {
				Expect(exportSnapshot(root, cwd, source, key, "", "unset", "export", "")).To(Equal(configurationSnapshot{true, true, "legacy", "legacy"}), key)
			}
		}
	})

	DescribeTable("retains the 14-key corrected historical contract", func(revision string) {
		root, cwd := fixture()
		source := correctedExportSource(root, revision)
		for _, key := range configurationKeys {
			writeFixture(filepath.Join(cwd, "factory.yaml"), []byte(strings.ToLower(key)+": YAML\n"), 0600)
			writeFixture(filepath.Join(cwd, "factory.config"), []byte(key+"=legacy\n"), 0600)
			for _, library := range []string{source, exportShimPath()} {
				Expect(exportSnapshot(root, cwd, library, key, "caller", "environment", "export", "")).To(Equal(configurationSnapshot{true, true, "caller", "caller"}), key)
			}
		}
	}, Entry("v0.1.6 plus approved EX-001", releaseReaderBaseline), Entry("merged Bash plus approved EX-001", baselineCommit))

	DescribeTable("preserves local and readonly callers and their conditional export bits", func(scope, placement, value string, exported bool) {
		root, cwd := fixture()
		switch placement {
		case "YAML":
			writeFixture(filepath.Join(cwd, "factory.yaml"), []byte("model_provider: YAML\n"), 0600)
		case "legacy":
			writeFixture(filepath.Join(cwd, "factory.config"), []byte("MODEL_PROVIDER=legacy\n"), 0600)
		}
		child := ""
		if exported {
			child = value
		}
		expected := configurationSnapshot{true, exported, value, child}
		for _, library := range []string{historicalConfiguration(root, exportBaseline), exportShimPath()} {
			Expect(exportSnapshot(root, cwd, library, "MODEL_PROVIDER", value, scope, "export", "")).To(Equal(expected))
		}
	}, Entry("matching YAML exports a local", "local", "YAML", "caller\nwith newline", true),
		Entry("matching legacy exports an empty local", "local", "legacy", "", true),
		Entry("unmatched local stays local", "local", "none", "local", false),
		Entry("matching YAML preserves readonly", "readonly", "YAML", "readonly", true),
		Entry("matching legacy preserves empty readonly", "readonly", "legacy", "", true))

	DescribeTable("transports file values without tab splitting or shell evaluation", func(value string) {
		root, cwd := fixture()
		writeFixture(filepath.Join(cwd, "factory.config"), []byte("REVIEW_MODEL='"+value+"'\n"), 0600)
		for _, library := range []string{historicalConfiguration(root, exportBaseline), exportShimPath()} {
			Expect(exportSnapshot(root, cwd, library, "REVIEW_MODEL", "", "unset", "legacy", "")).To(Equal(configurationSnapshot{true, true, value, value}))
		}
		_, err := os.Stat(filepath.Join(cwd, "marker"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("empty", ""), Entry("leading tab", "\tvalue"), Entry("interior repeated tabs", "left\t\tright"),
		Entry("trailing tab", "value\t"), Entry("carriage return", "value\r"),
		Entry("literal shell text", "$(touch marker) `touch marker` = \\\"quoted\\\""))

	It("preserves standalone legacy override and optional preserved-name semantics", func() {
		root, cwd := fixture()
		writeFixture(filepath.Join(cwd, "factory.config"), []byte("MODEL_PROVIDER=first\nMODEL_PROVIDER=last\n"), 0600)
		for _, library := range []string{historicalConfiguration(root, exportBaseline), exportShimPath()} {
			Expect(exportSnapshot(root, cwd, library, "MODEL_PROVIDER", "caller", "environment", "legacy", "")).To(Equal(configurationSnapshot{true, true, "last", "last"}))
			Expect(exportSnapshot(root, cwd, library, "MODEL_PROVIDER", "caller", "local", "legacy", "ignored MODEL_PROVIDER ignored")).To(Equal(configurationSnapshot{true, true, "caller", "caller"}))
		}
	})

	It("retains ordinary readonly scalar legacy diagnostics while continuing later records", func() {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.config")
		writeFixture(path, []byte("MODEL_PROVIDER=attempt\nREVIEW_MODEL=later\n"), 0600)
		probe := `. "$1"; MODEL_PROVIDER=caller; readonly MODEL_PROVIDER; unset REVIEW_MODEL; factory_config_load_legacy "$2"; status=$?; printf '%s\n%s\n%s\n' "$status" "$MODEL_PROVIDER" "$REVIEW_MODEL"; bash -c 'printf %s "$MODEL_PROVIDER"'`
		for _, library := range []string{historicalConfiguration(root, exportBaseline), exportShimPath()} {
			result := readerProcess(cwd, map[string]string{"FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}, "bash", "-c", probe, "readonly-legacy", library, path)
			Expect(result.status).To(Equal(0))
			Expect(result.stdout).To(Equal("0\ncaller\nlater\ncaller"))
			Expect(result.stderr).To(ContainSubstring("readonly variable"))
		}
	})

	It("characterizes legacy NUL divergence in the selected Bash without claiming parity", func() {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.config")
		writeFixture(path, []byte("MODEL_PRO\x00VIDER=left\x00right\n"), 0600)
		read := readerProcess(cwd, nil, "bash", "-c", `IFS= read -r line <"$1"; printf %s "$line"`, "nul-read", path)
		Expect(read.status).To(Equal(0))
		Expect(read.stderr).To(BeEmpty())
		var expected configurationSnapshot
		switch read.stdout {
		case "MODEL_PRO": // Bash 3.2 truncates at the first NUL.
			expected = configurationSnapshot{}
		case "MODEL_PROVIDER=leftright": // Modern Bash removes NUL bytes.
			expected = configurationSnapshot{true, true, "leftright", "leftright"}
		default:
			Fail("uncharacterized Bash NUL handling: " + strconv.Quote(read.stdout))
		}
		Expect(exportSnapshot(root, cwd, historicalConfiguration(root, exportBaseline), "MODEL_PROVIDER", "", "unset", "legacy", "")).To(Equal(expected))
	})

	It("characterizes candidate legacy NUL normalization outside supported parity", func() {
		root, cwd := fixture()
		writeFixture(filepath.Join(cwd, "factory.config"), []byte("MODEL_PRO\x00VIDER=left\x00right\n"), 0600)
		Expect(exportSnapshot(root, cwd, exportShimPath(), "MODEL_PROVIDER", "", "unset", "legacy", "")).To(Equal(configurationSnapshot{true, true, "leftright", "leftright"}))
	})

	It("preserves lexical legacy paths through a symlink followed by dot-dot", func() {
		root, cwd := fixture()
		Expect(os.MkdirAll(filepath.Join(cwd, "real", "child"), 0755)).To(Succeed())
		Expect(os.Symlink(filepath.Join(cwd, "real", "child"), filepath.Join(cwd, "link"))).To(Succeed())
		writeFixture(filepath.Join(cwd, "real", "factory.yaml"), []byte("project_name: fixture\n"), 0600)
		writeFixture(filepath.Join(cwd, "real", "factory.config"), []byte("MODEL_PROVIDER=lexical\n"), 0600)
		writeFixture(filepath.Join(cwd, "factory.config"), []byte("MODEL_PROVIDER=wrong-cleaned-path\n"), 0600)
		for _, library := range []string{historicalConfiguration(root, exportBaseline), exportShimPath()} {
			result := readerProcess(cwd, map[string]string{"FACTORY_CONFIG": cwd + "/link/../factory.yaml", "FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}, "bash", "-c", `. "$1"; unset MODEL_PROVIDER; factory_config_export; printf '%s' "$MODEL_PROVIDER"`, "lexical-path", library)
			Expect(result).To(Equal(cliResult{"lexical", "", 0}))
		}
	})

	DescribeTable("refuses an unavailable explicit runtime without assigning caller state", func(runtime string) {
		root, cwd := fixture()
		if runtime == "missing" {
			runtime = filepath.Join(root, "missing-runtime")
		}
		probe := `. "$1"; unset MODEL_PROVIDER; if factory_config_export; then status=0; else status=$?; fi; printf '%s\n%s' "$status" "${MODEL_PROVIDER-unset}"`
		result := readerProcess(cwd, map[string]string{"FACTORY_RUNTIME_BINARY": runtime}, "bash", "-c", probe, "runtime-refusal", exportShimPath())
		Expect(result.status).To(Equal(0))
		Expect(result.stdout).To(Equal("2\nunset"))
		Expect(result.stderr).NotTo(BeEmpty())
	}, Entry("unset selection", ""), Entry("relative selection", "relative-runtime"), Entry("missing executable", "missing"))

	DescribeTable("refuses complete invalid plans before any assignment", func(plan string, status int) {
		root, cwd := fixture()
		writeFixture(filepath.Join(root, "fake-runtime"), []byte("#!/bin/bash\nprintf '%s' \"$FACTORY_TEST_PLAN\"\nexit \"$FACTORY_TEST_STATUS\"\n"), 0755)
		probe := `. "$1"; unset MODEL_PROVIDER; if factory_config_export; then result=0; else result=$?; fi; printf '%s\n%s' "$result" "${MODEL_PROVIDER-unset}"`
		result := readerProcess(cwd, map[string]string{"FACTORY_RUNTIME_BINARY": filepath.Join(root, "fake-runtime"), "FACTORY_TEST_PLAN": plan, "FACTORY_TEST_STATUS": strconv.Itoa(status)}, "bash", "-c", probe, "invalid-plan", exportShimPath())
		Expect(result.status).To(Equal(0))
		Expect(result.stdout).NotTo(HavePrefix("0\n"))
		Expect(result.stdout).To(HaveSuffix("\nunset"))
	}, Entry("truncated after valid assignment", "FACTORY_CONFIG_PLAN_V1\nset\tMODEL_PROVIDER\tshould-not-apply\n", 0),
		Entry("unknown key after valid assignment", "FACTORY_CONFIG_PLAN_V1\nset\tMODEL_PROVIDER\tshould-not-apply\nset\tPATH\tbad\nEND\n", 0),
		Entry("unknown operation", "FACTORY_CONFIG_PLAN_V1\nrun\tMODEL_PROVIDER\tbad\nEND\n", 0),
		Entry("extra data after terminator", "FACTORY_CONFIG_PLAN_V1\nEND\nextra\n", 0),
		Entry("runtime fails after complete output", "FACTORY_CONFIG_PLAN_V1\nset\tMODEL_PROVIDER\tshould-not-apply\nEND\n", 9))

	It("defines the allowlist without invoking the runtime at source time", func() {
		root, cwd := fixture()
		writeFixture(filepath.Join(root, "fake-runtime"), []byte("#!/bin/bash\ntouch marker\nexit 1\n"), 0755)
		result := readerProcess(cwd, map[string]string{"FACTORY_RUNTIME_BINARY": filepath.Join(root, "fake-runtime")}, "bash", "-c", `. "$1"; printf '%s' "$FACTORY_CONFIG_KEYS"`, "source-only", exportShimPath())
		Expect(result.status).To(Equal(0))
		Expect(result.stderr).To(BeEmpty())
		Expect(strings.Fields(result.stdout)).To(Equal(exportKeys))
		_, err := os.Stat(filepath.Join(cwd, "marker"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	It("refuses arithmetic-bearing integer targets before executing data or applying earlier records", func() {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.config")
		writeFixture(path, []byte("MODEL_PROVIDER=must-not-apply\nREVIEW_MODEL='injection_array[$(touch marker)]'\n"), 0600)
		probe := `. "$1"; unset MODEL_PROVIDER REVIEW_MODEL; declare -i REVIEW_MODEL; declare -a injection_array; injection_array[0]=0; if factory_config_load_legacy "$2"; then status=0; else status=$?; fi; printf '%s\n%s' "$status" "${MODEL_PROVIDER-unset}"`
		result := readerProcess(cwd, map[string]string{"FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}, "bash", "-c", probe, "integer-target", exportShimPath(), path)
		_, err := os.Stat(filepath.Join(cwd, "marker"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "configuration arithmetic executed; result: %#v", result)
		Expect(result.status).To(Equal(0))
		Expect(result.stdout).NotTo(HavePrefix("0\n"))
		Expect(result.stdout).To(HaveSuffix("\nunset"))
	})

	DescribeTable("refuses unsupported variable attributes before any SET or EXPORT", func(attribute, operation string) {
		root, cwd := fixture()
		if supported := readerProcess(cwd, nil, "bash", "-c", `declare "$1" fixture_probe`, "attribute-probe", attribute); supported.status != 0 {
			Skip("the current Bash does not support " + attribute)
		}
		writeFixture(filepath.Join(cwd, "factory.config"), []byte("MODEL_PROVIDER=must-not-apply\nREVIEW_MODEL=value\n"), 0600)
		probe := `. "$1"; unset MODEL_PROVIDER REVIEW_MODEL; referenced_value=before; declare "$2" REVIEW_MODEL; if [ "$2" = -n ]; then REVIEW_MODEL=referenced_value; fi; if [ "$3" = export ]; then REVIEW_MODEL=7; fi; if [ "$3" = legacy ]; then if factory_config_load_legacy "$FACTORY_CONFIG_LEGACY"; then status=0; else status=$?; fi; else if factory_config_export; then status=0; else status=$?; fi; fi; printf '%s\n%s' "$status" "${MODEL_PROVIDER-unset}"`
		result := readerProcess(cwd, map[string]string{"FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory"), "FACTORY_CONFIG": filepath.Join(cwd, "factory.yaml"), "FACTORY_CONFIG_LEGACY": filepath.Join(cwd, "factory.config")}, "bash", "-c", probe, "attribute-target", exportShimPath(), attribute, operation)
		Expect(result.status).To(Equal(0))
		Expect(result.stdout).NotTo(HavePrefix("0\n"))
		Expect(result.stdout).To(HaveSuffix("\nunset"))
	}, Entry("indexed array SET", "-a", "legacy"), Entry("associative array SET", "-A", "legacy"),
		Entry("nameref SET", "-n", "legacy"), Entry("integer EXPORT", "-i", "export"), Entry("indexed array EXPORT", "-a", "export"))
})
