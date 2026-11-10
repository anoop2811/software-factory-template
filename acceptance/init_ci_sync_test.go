package acceptance_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Compare each unowned JSON subtree as ordered JSON, rather than decoding it
// into maps that would erase the permission policy's last-match precedence.
// per docs/adr/0146-initialized-installed-ci.md:97
func initCIUnownedJSON(source string) map[string]string {
	GinkgoHelper()
	var root map[string]json.RawMessage
	Expect(json.Unmarshal([]byte(source), &root)).To(Succeed())
	result := map[string]string{}
	retain := func(path string, value json.RawMessage) {
		var compact bytes.Buffer
		Expect(json.Compact(&compact, value)).To(Succeed())
		result[path] = compact.String()
	}
	for key, value := range root {
		switch key {
		case "model", "small_model":
		case "agent":
			if string(bytes.TrimSpace(value)) == "null" {
				retain(key, value)
				continue
			}
			var agents map[string]map[string]json.RawMessage
			Expect(json.Unmarshal(value, &agents)).To(Succeed())
			for name, fields := range agents {
				for field, content := range fields {
					if field != "model" {
						retain("agent/"+name+"/"+field, content)
					}
				}
			}
		default:
			retain(key, value)
		}
	}
	return result
}

// per docs/adr/0146-initialized-installed-ci.md:89
var _ = Describe("Native orchestration sync ordered policy", func() {
	// per docs/adr/0146-initialized-installed-ci.md:97
	// per docs/adr/0146-initialized-installed-ci.md:107
	It("preserves ordered permission subtrees and reaches the same stable state through all or sequential sync", func() {
		canonical := strings.Replace(syncCanonicalJSON, `"permission":{"edit":"deny","bash":{"go *":"allow","rm *":"deny","git status":"allow","*":"ask"}}`, `"permission":{"edit":"deny","bash":{"go *":"allow","rm *":"deny","git status":"allow","*":"ask"},"read":{"z-private/**":"deny","*":"allow"}}`, 1)
		canonical = strings.Replace(canonical, `"permission":{"edit":"ask"}`, `"permission":{"read":{"z-private/**":"deny","*":"allow"},"edit":"ask","bash":{"git *":"allow","*":"deny"}}`, 1)
		canonical = strings.Replace(canonical, `"retained":{"enabled":true,"number":7}`, `"retained":{"z-last":{"z":1,"a":2},"a-first":true}`, 1)
		for _, configured := range []bool{true, false} {
			root, cwd, environment := syncFixture()
			sequential, otherCWD, otherEnvironment := syncFixture()
			configuration := "opencode_default_model:\nclaude_default_model:\ncodex_default_model:\n"
			if configured {
				configuration = syncConfiguredModels()
			}
			for _, path := range []string{root, sequential} {
				writeFixture(filepath.Join(path, "opencode.json"), []byte(canonical), 0600)
			}
			for _, path := range []string{cwd, otherCWD} {
				writeFixture(filepath.Join(path, "factory.yaml"), []byte(configuration), 0600)
			}
			before := initCIUnownedJSON(canonical)
			out := syncRun(root, cwd, environment, "all")
			Expect(out.status).To(BeZero(), "%+v", out)
			Expect(initCIUnownedJSON(syncRead(root, "opencode.json"))).To(Equal(before), "configured=%t: synchronization must not reorder permission rules or unknown nested data", configured)
			models := syncSameModels("")
			if configured {
				models = map[string]string{"implementer": "source/default", "refactorer": "source/default", "reviewer": "source/frontier", "spec-writer": "source/frontier", "wiki-maintainer": "source/default", "zeta": "source/default"}
			}
			syncAssertModels(root, "opencode", models)
			for _, harness := range []string{"opencode", "claude", "codex"} {
				out = syncRun(sequential, otherCWD, otherEnvironment, harness)
				Expect(out.status).To(BeZero(), "%s: %+v", harness, out)
			}
			Expect(syncRead(root, "opencode.json")).To(Equal(syncRead(sequential, "opencode.json")))
			for _, directory := range []string{".opencode", ".claude", ".codex"} {
				Expect(nativeInitArtifacts(filepath.Join(root, directory))).To(Equal(nativeInitArtifacts(filepath.Join(sequential, directory))), directory)
			}
			stable := nativeInitArtifacts(root)
			out = syncRun(root, cwd, environment, "all")
			Expect(out.status).To(BeZero(), "%+v", out)
			Expect(nativeInitArtifacts(root)).To(Equal(stable), "the first all-harness synchronization must already be stable")
		}
		// per docs/adr/0146-initialized-installed-ci.md:99
		for _, agent := range []string{"", `,"agent":null`} {
			root, cwd, environment := syncFixture()
			input := `{"permission":{"bash":{"z *":"allow","*":"deny"}},"retained":{"z":1,"a":2}` + agent + `}`
			writeFixture(filepath.Join(root, "opencode.json"), []byte(input), 0600)
			writeFixture(filepath.Join(cwd, "factory.yaml"), []byte(syncConfiguredModels()), 0600)
			out := syncRun(root, cwd, environment, "opencode")
			Expect(out.status).To(BeZero(), "accepted absent/null agent: %+v", out)
			Expect(initCIUnownedJSON(syncRead(root, "opencode.json"))).To(Equal(initCIUnownedJSON(input)))
		}
	})
})
