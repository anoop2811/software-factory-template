package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Deliberately independent of the implementation's allowlist.
// per specs/001-go-runtime-conversion.md:102
var configurationKeys = []string{
	"COST_PROFILE", "MODEL_PROVIDER",
	"OPENCODE_FRONTIER_MODEL", "OPENCODE_DEFAULT_MODEL", "OPENCODE_ECONOMY_MODEL",
	"CLAUDE_FRONTIER_MODEL", "CLAUDE_DEFAULT_MODEL", "CLAUDE_ECONOMY_MODEL",
	"CODEX_FRONTIER_MODEL", "CODEX_DEFAULT_MODEL", "CODEX_ECONOMY_MODEL",
	"REVIEW_LANE", "REVIEW_MODEL", "REVIEW_API_KEY_SECRET",
}

type configurationSnapshot struct {
	set, exported bool
	value, child  string
}

func configText(value string) *string { return &value }

// Fresh Bash processes exercise the actual sourceable adapter. Positional
// arguments and environment carry fixture data; no config is evaluated as code.
const configurationProbe = `set -euo pipefail
. "$1"
configuration_emit() {
  local observed_key="$1"
  printf '%s\000%s\000' "${!observed_key+x}" "${!observed_key-}"
  bash -c 'key="$1"; printf '\''%s\000%s\000'\'' "${!key+x}" "${!key-}"' _ "$observed_key"
}
if [ "$4" = local ] || [ "$4" = readonly ]; then
  configuration_scope() {
    local "$2=$3"
    if [ "$4" = readonly ]; then readonly "$2"; fi
    factory_config_export
    configuration_emit "$2"
  }
  configuration_scope "$@"
elif [ "$4" = legacy-reader ]; then
  factory_config_load_legacy "$(dirname "$FACTORY_CONFIG")/factory.config"
  configuration_emit "$2"
else
  factory_config_export
  configuration_emit "$2"
fi
`

func configurationFixture(yaml, legacy *string) string {
	GinkgoHelper()
	root, err := os.MkdirTemp("", "factory config acceptance ")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, root)
	if yaml != nil {
		writeFixture(filepath.Join(root, "factory.yaml"), []byte(*yaml), 0600)
	}
	if legacy != nil {
		writeFixture(filepath.Join(root, "factory.config"), []byte(*legacy), 0600)
	}
	return root
}

func observeConfiguration(root, source, key string, caller *string, scope string) configurationSnapshot {
	GinkgoHelper()
	if source == "" {
		var err error
		source, err = filepath.Abs(filepath.Join("..", "scripts", "lib", "config.sh"))
		Expect(err).NotTo(HaveOccurred())
	}
	environment := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if member(name, configurationKeys...) || member(name, "FACTORY_CONFIG", "UNEXPECTED_FACTORY_SETTING", "BASH_ENV") {
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(environment, "FACTORY_CONFIG="+filepath.Join(root, "factory.yaml"))
	value := ""
	if caller != nil {
		value = *caller
		if scope != "local" && scope != "readonly" {
			environment = append(environment, key+"="+value)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", configurationProbe, "configuration-probe", source, key, value, scope) // #nosec G204 -- fixed probe; fixture values are literal argv/environment, not shell source.
	cmd.Dir, cmd.Env = root, environment
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	Expect(cmd.Run()).To(Succeed(), "config boundary failed: %s", stderr.String())
	Expect(ctx.Err()).NotTo(HaveOccurred())
	Expect(stderr.String()).To(BeEmpty())
	fields := strings.Split(stdout.String(), "\x00")
	Expect(fields).To(HaveLen(5), "unexpected probe output: %q", stdout.String())
	Expect(fields[0]).To(BeElementOf("", "x"))
	Expect(fields[2]).To(BeElementOf("", "x"))
	Expect(fields[4]).To(BeEmpty())
	return configurationSnapshot{fields[0] == "x", fields[2] == "x", fields[1], fields[3]}
}

func historicalConfiguration(root, revision string) string {
	GinkgoHelper()
	git := exec.Command("git", "show", revision+":scripts/lib/config.sh") // #nosec G204 -- immutable, test-declared revision; no shell interpolation.
	contents, err := git.Output()
	Expect(err).NotTo(HaveOccurred(), "historical baseline must be fetched in CI")
	path := filepath.Join(root, "historical-config.sh")
	writeFixture(path, contents, 0600)
	return path
}

func callerPrecedenceEntries() []TableEntry {
	var entries []TableEntry
	for _, key := range configurationKeys {
		for _, placement := range []string{"both files", "no YAML", "nonmatching YAML key"} {
			for _, empty := range []bool{false, true} {
				entries = append(entries, Entry(fmt.Sprintf("%s / %s / empty=%t", key, placement, empty), key, placement, empty))
			}
		}
	}
	return entries
}

func configurationKeyEntries() []TableEntry {
	var entries []TableEntry
	for _, key := range configurationKeys {
		entries = append(entries, Entry(key, key))
	}
	return entries
}

var _ = Describe("EX001 sourceable configuration precedence", func() {
	// per specs/001-go-runtime-conversion.md:102
	// per docs/DECISION_LOG.md:1903
	DescribeTable("preserves explicit caller values, including empty, ahead of both files", func(key, placement string, empty bool) {
		yaml := configText(strings.ToLower(key) + ": yaml value\n")
		switch placement {
		case "no YAML":
			yaml = nil
		case "nonmatching YAML key":
			yaml = configText("project_name: fixture\n")
		}
		root := configurationFixture(yaml, configText(key+"=legacy value\n"))
		caller := "caller " + key
		if empty {
			caller = ""
		}
		Expect(observeConfiguration(root, "", key, &caller, "environment")).To(Equal(configurationSnapshot{true, true, caller, caller}))
	}, callerPrecedenceEntries())

	// per specs/001-go-runtime-conversion.md:102
	DescribeTable("uses YAML, then legacy, only when the caller key is unset", func(key string) {
		for _, fixture := range []struct {
			yaml, legacy *string
			want         string
		}{
			{configText(strings.ToLower(key) + ": yaml value\n"), configText(key + "=legacy value\n"), "yaml value"},
			{configText(strings.ToLower(key) + ": yaml value\n"), nil, "yaml value"},
			{nil, configText(key + "=legacy value\n"), "legacy value"},
			{configText("project_name: fixture\n"), configText(key + "=legacy value\n"), "legacy value"},
			{configText(strings.ToLower(key) + ": \"\"\n"), configText(key + "=legacy value\n"), "legacy value"},
		} {
			root := configurationFixture(fixture.yaml, fixture.legacy)
			Expect(observeConfiguration(root, "", key, nil, "environment")).To(Equal(configurationSnapshot{true, true, fixture.want, fixture.want}))
		}
	}, configurationKeyEntries())

	// per specs/001-go-runtime-conversion.md:102
	It("keeps an absent key unset and an explicitly empty legacy assignment set", func() {
		root := configurationFixture(nil, nil)
		Expect(observeConfiguration(root, "", "MODEL_PROVIDER", nil, "environment")).To(Equal(configurationSnapshot{}))
		root = configurationFixture(nil, configText("MODEL_PROVIDER=\n"))
		Expect(observeConfiguration(root, "", "MODEL_PROVIDER", nil, "environment")).To(Equal(configurationSnapshot{true, true, "", ""}))
	})

	// per specs/001-go-runtime-conversion.md:102
	DescribeTable("preserves caller local values and the existing export side effect", func(placement string, exported bool) {
		var yaml, legacy *string
		switch placement {
		case "YAML":
			yaml = configText("model_provider: yaml\n")
		case "legacy":
			legacy = configText("MODEL_PROVIDER=legacy\n")
		}
		for _, caller := range []string{"local caller", ""} {
			root := configurationFixture(yaml, legacy)
			child := ""
			if exported {
				child = caller
			}
			Expect(observeConfiguration(root, "", "MODEL_PROVIDER", &caller, "local")).To(Equal(configurationSnapshot{true, exported, caller, child}))
		}
	}, Entry("matching YAML promotes local caller to environment", "YAML", true),
		Entry("matching legacy promotes local caller to environment", "legacy", true),
		Entry("no matching file does not export a local caller", "none", false))

	// per specs/001-go-runtime-conversion.md:102
	// per docs/DECISION_LOG.md:1903
	DescribeTable("preserves readonly caller values without attempting a legacy assignment", func(withYAML, empty bool) {
		var yaml *string
		if withYAML {
			yaml = configText("model_provider: YAML\n")
		}
		root := configurationFixture(yaml, configText("MODEL_PROVIDER=legacy\n"))
		caller := "readonly caller"
		if empty {
			caller = ""
		}
		Expect(observeConfiguration(root, "", "MODEL_PROVIDER", &caller, "readonly")).To(Equal(configurationSnapshot{true, true, caller, caller}))
	}, Entry("both files / nonempty", true, false), Entry("both files / empty", true, true),
		Entry("legacy only / nonempty", false, false), Entry("legacy only / empty", false, true))

	// per specs/001-go-runtime-conversion.md:103
	DescribeTable("never evaluates metacharacters from the winning source", func(winner string) {
		root := configurationFixture(nil, nil)
		marker := filepath.Join(root, "must-not-exist")
		literal := "$(touch '" + marker + "') `touch '" + marker + "'` ; $HOME # literal"
		var caller *string
		switch winner {
		case "caller":
			caller = &literal
			writeFixture(filepath.Join(root, "factory.yaml"), []byte("model_provider: yaml\n"), 0600)
			writeFixture(filepath.Join(root, "factory.config"), []byte("MODEL_PROVIDER=legacy\n"), 0600)
		case "YAML":
			writeFixture(filepath.Join(root, "factory.yaml"), []byte("model_provider: \""+literal+"\" # outside comment\n"), 0600)
			writeFixture(filepath.Join(root, "factory.config"), []byte("MODEL_PROVIDER=legacy\n"), 0600)
		default:
			writeFixture(filepath.Join(root, "factory.config"), []byte("MODEL_PROVIDER=\""+literal+"\" # outside comment\n"), 0600)
		}
		Expect(observeConfiguration(root, "", "MODEL_PROVIDER", caller, "environment")).To(Equal(configurationSnapshot{true, true, literal, literal}))
		_, err := os.Stat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("caller", "caller"), Entry("YAML", "YAML"), Entry("legacy", "legacy"))

	// per specs/001-go-runtime-conversion.md:103
	It("refuses to export unknown keys from either file", func() {
		root := configurationFixture(configText("unexpected_factory_setting: YAML\n"), configText("UNEXPECTED_FACTORY_SETTING=legacy\n"))
		Expect(observeConfiguration(root, "", "UNEXPECTED_FACTORY_SETTING", nil, "environment")).To(Equal(configurationSnapshot{}))
	})

	// per specs/001-go-runtime-conversion.md:102
	It("retains the last recognized legacy assignment and the standalone legacy reader contract", func() {
		root := configurationFixture(nil, configText("MODEL_PROVIDER=first\nexport model_provider='last # literal' # comment\n"))
		expected := configurationSnapshot{true, true, "last # literal", "last # literal"}
		Expect(observeConfiguration(root, "", "MODEL_PROVIDER", nil, "environment")).To(Equal(expected))
		Expect(observeConfiguration(root, "", "MODEL_PROVIDER", configText("caller"), "legacy-reader")).To(Equal(expected))
	})
})

var _ = Describe("EX001 immutable historical discrepancy evidence", func() {
	// Historical bytes retain their observed defect; desired correction is tested
	// separately above. per specs/001-go-runtime-conversion.md:484
	DescribeTable("records the legacy overwrite without pretending an old release was corrected", func(revision string) {
		for _, scope := range []string{"environment", "local"} {
			for _, caller := range []string{"caller", ""} {
				root := configurationFixture(configText("model_provider: YAML\n"), configText("MODEL_PROVIDER=legacy\n"))
				source := historicalConfiguration(root, revision)
				Expect(observeConfiguration(root, source, "MODEL_PROVIDER", &caller, scope)).To(Equal(configurationSnapshot{true, true, "legacy", "legacy"}))
			}
		}
	}, Entry("merged Bash baseline", baselineCommit), Entry("v0.1.6", "b71ecc32e07ecd87eb330ba8e497c86612f92acd"))
})

type configurationCorrection struct {
	SchemaVersion   int    `json:"schema_version"`
	ID              string `json:"id"`
	PatchFile       string `json:"patch_file"`
	PatchSHA256     string `json:"patch_sha256"`
	Path            string `json:"path"`
	OriginalSHA256  string `json:"original_sha256"`
	CorrectedSHA256 string `json:"corrected_sha256"`
	GitMode         string `json:"git_mode"`
	Baselines       []struct {
		Label, Commit string
	} `json:"baselines"`
}

func configurationDigest(contents []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(contents))
}

// Test-only construction of an explicitly corrected parity fixture. Neither
// historical Git objects nor installed source files are modified.
// per specs/001-go-runtime-conversion.md:489
func applyConfigurationCorrection(root string, original, patch []byte, mode string, correction configurationCorrection) (string, error) {
	if correction.SchemaVersion != 1 || correction.ID != "EX-001" || correction.PatchFile != "EX-001.patch" || correction.Path != "scripts/lib/config.sh" {
		return "", fmt.Errorf("unexpected correction identity or path")
	}
	if len(correction.Baselines) != 2 {
		return "", fmt.Errorf("unexpected correction baselines")
	}
	wantBaselines := map[string]string{"v0.1.6": "b71ecc32e07ecd87eb330ba8e497c86612f92acd", "merged-bash": baselineCommit}
	for _, baseline := range correction.Baselines {
		want, known := wantBaselines[baseline.Label]
		if !known || want != baseline.Commit {
			return "", fmt.Errorf("unexpected correction baseline")
		}
		delete(wantBaselines, baseline.Label)
	}
	if correction.OriginalSHA256 != "38fbc5580c8f676d9fc28e56b17b928d11a00fa05727f248fed09b07f21b811c" || configurationDigest(original) != correction.OriginalSHA256 {
		return "", fmt.Errorf("original source hash mismatch")
	}
	if mode != "100755" || correction.GitMode != mode {
		return "", fmt.Errorf("original Git mode mismatch")
	}
	if configurationDigest(patch) != correction.PatchSHA256 {
		return "", fmt.Errorf("correction patch hash mismatch")
	}
	apply := func(args ...string) (string, error) {
		cmd := exec.Command("git", append([]string{"apply"}, args...)...) // #nosec G204 -- fixed git operation; patch bytes and paths belong to this isolated acceptance fixture.
		cmd.Dir, cmd.Stdin = root, bytes.NewReader(patch)
		for _, entry := range os.Environ() {
			name := strings.SplitN(entry, "=", 2)[0]
			if !member(name, "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE") {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		output, err := cmd.CombinedOutput()
		return string(output), err
	}
	stat, err := apply("--numstat", "-")
	if err != nil {
		return "", fmt.Errorf("cannot inspect correction patch: %w: %s", err, stat)
	}
	fields := strings.Split(strings.TrimSuffix(stat, "\n"), "\t")
	if len(fields) != 3 || fields[2] != correction.Path {
		return "", fmt.Errorf("correction patch changes an unexpected path")
	}
	path := filepath.Join(root, "scripts", "lib", "config.sh")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, original, 0755); err != nil { // #nosec G306 G703 -- test-owned MkdirTemp root plus fixed relative path; immutable fixture retains checked executable mode.
		return "", err
	}
	if output, err := apply("--check", "-"); err != nil {
		return "", fmt.Errorf("correction does not apply: %w: %s", err, output)
	}
	if output, err := apply("-"); err != nil {
		return "", fmt.Errorf("cannot apply correction: %w: %s", err, output)
	}
	corrected, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if configurationDigest(corrected) != correction.CorrectedSHA256 {
		return "", fmt.Errorf("corrected source hash mismatch")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Mode().Perm() != 0755 {
		return "", fmt.Errorf("corrected source mode changed")
	}
	return path, nil
}

var _ = Describe("EX001 explicit corrected historical parity fixtures", func() {
	var correction configurationCorrection
	var patch []byte

	BeforeEach(func() {
		directory := filepath.Join("..", "docs", "migration", "corrections")
		metadata, err := os.ReadFile(filepath.Join(directory, "EX-001.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(json.Unmarshal(metadata, &correction)).To(Succeed())
		patch, err = os.ReadFile(filepath.Join(directory, "EX-001.patch"))
		Expect(err).NotTo(HaveOccurred())
	})

	// per specs/001-go-runtime-conversion.md:489
	DescribeTable("applies only the identified correction before asserting the desired caller precedence", func(revision string) {
		root := configurationFixture(nil, nil)
		originalPath := historicalConfiguration(root, revision)
		original, err := os.ReadFile(originalPath)
		Expect(err).NotTo(HaveOccurred())
		modeQuery := exec.Command("git", "ls-tree", "--full-tree", revision, "--", "scripts/lib/config.sh") // #nosec G204 -- immutable test-declared revision and fixed path.
		modeOutput, err := modeQuery.Output()
		Expect(err).NotTo(HaveOccurred())
		modeFields := strings.Fields(string(modeOutput))
		Expect(modeFields).To(HaveLen(4))
		mode := modeFields[0]
		_, err = strconv.ParseUint(mode, 8, 32)
		Expect(err).NotTo(HaveOccurred())
		source, err := applyConfigurationCorrection(root, original, patch, mode, correction)
		Expect(err).NotTo(HaveOccurred())
		for _, key := range configurationKeys {
			writeFixture(filepath.Join(root, "factory.yaml"), []byte(strings.ToLower(key)+": YAML\n"), 0600)
			writeFixture(filepath.Join(root, "factory.config"), []byte(key+"=legacy\n"), 0600)
			for _, value := range []string{"caller", ""} {
				Expect(observeConfiguration(root, source, key, &value, "environment")).To(Equal(configurationSnapshot{true, true, value, value}))
			}
			Expect(observeConfiguration(root, source, key, nil, "environment")).To(Equal(configurationSnapshot{true, true, "YAML", "YAML"}))
			Expect(os.Remove(filepath.Join(root, "factory.yaml"))).To(Succeed())
			Expect(observeConfiguration(root, source, key, nil, "environment")).To(Equal(configurationSnapshot{true, true, "legacy", "legacy"}))
		}
		writeFixture(filepath.Join(root, "factory.config"), []byte("MODEL_PROVIDER=legacy\n"), 0600)
		for _, scope := range []string{"local", "readonly"} {
			Expect(observeConfiguration(root, source, "MODEL_PROVIDER", configText(""), scope)).To(Equal(configurationSnapshot{true, true, "", ""}))
		}
		// The actual historical fixture remains unchanged and retains its defect.
		contents, err := os.ReadFile(originalPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(contents).To(Equal(original))
	}, Entry("v0.1.6 plus EX-001", "b71ecc32e07ecd87eb330ba8e497c86612f92acd"), Entry("merged Bash plus EX-001", baselineCommit))

	// per specs/001-go-runtime-conversion.md:269
	DescribeTable("rejects damaged correction inputs instead of manufacturing parity", func(damage string) {
		root := configurationFixture(nil, nil)
		originalPath := historicalConfiguration(root, baselineCommit)
		original, err := os.ReadFile(originalPath)
		Expect(err).NotTo(HaveOccurred())
		var expected string
		switch damage {
		case "source":
			original = append(original, []byte("\n# wrong historical input\n")...)
			expected = "original source hash mismatch"
		case "patch":
			patch = append(patch, []byte("\n# altered patch\n")...)
			expected = "correction patch hash mismatch"
		case "metadata":
			correction.CorrectedSHA256 = strings.Repeat("0", 64)
			expected = "corrected source hash mismatch"
		case "mode":
			correction.GitMode = "100644"
			expected = "original Git mode mismatch"
		}
		_, err = applyConfigurationCorrection(root, original, patch, "100755", correction)
		Expect(err).To(MatchError(ContainSubstring(expected)))
	}, Entry("wrong historical source", "source"), Entry("corrupted patch", "patch"), Entry("metadata mismatch", "metadata"), Entry("mode mismatch", "mode"))
})
