package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The expected documents encode the canonical fixture's actual permissions and
// MCP conversion independently of the candidate's serializer. Their field order
// and whitespace deliberately differ from Go's generated JSON.
// per docs/adr/0156-integrated-adopter-rehearsal.md:91
const compatibleSyncClaudeJSON = `{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {"deny": ["Bash(rm *)", "Edit"], "allow": ["Bash(go *)", "Bash(git status)"]},
  "hooks": {"PreToolUse": [{"matcher": "Edit|Write", "hooks": [{"args": [], "command": "${CLAUDE_PROJECT_DIR}/scripts/hooks/test-edit-denial.sh", "type": "command"}]}]}
}
`

// per docs/adr/0156-integrated-adopter-rehearsal.md:91
const compatibleSyncMCPJSON = `{ "mcpServers": {
  "scalar": {"env": {}, "args": [], "command": "single-tool"},
  "fixture": {"env": {"KEY": "literal value"}, "args": ["two words", "$(not-code)"], "command": "literal tool"}
} }
`

// Model pins already match the selected configuration, while unowned policy
// order and adjacent integers beyond float64's exact range remain literal data.
// per docs/adr/0156-integrated-adopter-rehearsal.md:94
func compatibleSyncOpenCodeJSON() string {
	value := strings.NewReplacer("__DEFAULT_MODEL__", "source/default", "__ECONOMY_MODEL__", "source/default").Replace(syncCanonicalJSON)
	for _, role := range []string{"zeta", "wiki", "spec", "review", "refactor", "implement"} {
		value = strings.ReplaceAll(value, "old-"+role, "source/default")
	}
	return strings.Replace(value, `"retained":{"enabled":true,"number":7}`, `"retained":{"z":9007199254740993,"a":9007199254740992}`, 1)
}

var _ = Describe("Native orchestration sync compatible JSON", func() {
	// per docs/adr/0156-integrated-adopter-rehearsal.md:91
	// per docs/adr/0156-integrated-adopter-rehearsal.md:95
	It("retains equivalent JSON bytes while publishing changed model pins without losing ordered or numeric data", func() {
		root, cwd, environment := syncFixture()
		configuration := "cost_profile: standard\nopencode_default_model: source/default\nopencode_frontier_model: source/default\nopencode_economy_model: source/default\n"
		writeFixture(filepath.Join(cwd, "factory.yaml"), []byte(configuration), 0600)
		expected := map[string]string{"opencode.json": compatibleSyncOpenCodeJSON(), ".claude/settings.json": compatibleSyncClaudeJSON, ".mcp.json": compatibleSyncMCPJSON}
		for path, data := range expected {
			writeFixture(filepath.Join(root, path), []byte(data), 0640)
			Expect(os.Chmod(filepath.Join(root, path), 0640)).To(Succeed())
		}
		out := syncRun(root, cwd, environment, "all")
		Expect(out.status).To(BeZero(), "%+v", out)
		for path, data := range expected {
			Expect(syncRead(root, path)).To(Equal(data), "equivalent document %s must retain its original bytes", path)
			info, err := os.Lstat(filepath.Join(root, path))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0640)), path)
		}

		// A real configured model change still reaches publication. The existing
		// ordered-policy family separately checks all/sequential equivalence.
		// per docs/adr/0156-integrated-adopter-rehearsal.md:96
		before := initCIUnownedJSON(expected["opencode.json"])
		writeFixture(filepath.Join(cwd, "factory.yaml"), []byte(strings.ReplaceAll(configuration, "source/default", "source/next")), 0600)
		out = syncRun(root, cwd, environment, "opencode")
		Expect(out.status).To(BeZero(), "%+v", out)
		changed := syncRead(root, "opencode.json")
		Expect(changed).NotTo(Equal(expected["opencode.json"]))
		Expect(syncJSON(changed)).To(HaveKeyWithValue("model", "source/next"))
		Expect(syncJSON(changed)).To(HaveKeyWithValue("small_model", "source/next"))
		syncAssertModels(root, "opencode", syncSameModels("source/next"))
		Expect(initCIUnownedJSON(changed)).To(Equal(before))
		Expect(changed).To(ContainSubstring(`"z": 9007199254740993`))
		Expect(changed).To(ContainSubstring(`"a": 9007199254740992`))
	})

	// per docs/adr/0156-integrated-adopter-rehearsal.md:93
	// per docs/adr/0156-integrated-adopter-rehearsal.md:97
	DescribeTable("repairs meaningful generated JSON differences rather than preserving them as formatting", func(kind string) {
		root, cwd, environment := syncFixture()
		settings, mcp := compatibleSyncClaudeJSON, compatibleSyncMCPJSON
		switch kind {
		case "array":
			settings = strings.Replace(settings, `["Bash(go *)", "Bash(git status)"]`, `["Bash(git status)", "Bash(go *)"]`, 1)
		case "value":
			mcp = strings.Replace(mcp, `"command": "literal tool"`, `"command": "wrong tool"`, 1)
		case "malformed":
			settings, mcp = "{broken settings", "{broken MCP"
		}
		writeFixture(filepath.Join(root, ".claude/settings.json"), []byte(settings), 0600)
		writeFixture(filepath.Join(root, ".mcp.json"), []byte(mcp), 0600)
		canonical := syncRead(root, "opencode.json")
		out := syncRun(root, cwd, environment, "claude")
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(syncJSON(syncRead(root, ".claude/settings.json"))).To(Equal(syncJSON(compatibleSyncClaudeJSON)))
		Expect(syncJSON(syncRead(root, ".mcp.json"))).To(Equal(syncJSON(compatibleSyncMCPJSON)))
		Expect(syncRead(root, "opencode.json")).To(Equal(canonical))
	}, Entry("permission array order", "array"), Entry("MCP command value", "value"), Entry("malformed generated destinations", "malformed"))
})
