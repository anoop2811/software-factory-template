package acceptance_test

import (
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"
)

const sourceStructuralBaseline = "9b7ef1ff4f2990309e94af61a19839e94e56cebf"

// The current public source script is the subject of this boundary test.
// per docs/adr/0134-go-source-structural-evaluation.md:9
func sourceStructuralAdapter(root string) string {
	GinkgoHelper()
	body, err := os.ReadFile("../scripts/harness-structural-eval.sh")
	Expect(err).NotTo(HaveOccurred())
	path := filepath.Join(root, "scripts/harness-structural-eval.sh")
	writeFixture(path, body, 0755)
	return path
}

// Both documented source invocation forms enter the same owning runtime.
// per docs/adr/0134-go-source-structural-evaluation.md:17
func sourceStructuralInvoke(cwd string, environment map[string]string, input, program string, bash bool, args ...string) cliResult {
	GinkgoHelper()
	if bash {
		return handoffNativeExactProcess(cwd, environment, input, "/bin/bash", append([]string{program}, args...)...)
	}
	return handoffNativeExactProcess(cwd, environment, input, program, args...)
}

// Observe the current real shared gate without replacing its denial decision.
// per docs/adr/0134-go-source-structural-evaluation.md:24
func sourceStructuralProject(root, base string) map[string]string {
	GinkgoHelper()
	for _, path := range []string{".opencode/plugin/factory-hooks.ts", ".claude/agents/implementer.md", ".codex/agents/implementer.toml", "scripts/lib/config.sh", "scripts/lib/events.sh", "scripts/hooks/test-edit-denial.sh"} {
		body, err := os.ReadFile(filepath.Join("..", path))
		Expect(err).NotTo(HaveOccurred())
		if path == "scripts/hooks/test-edit-denial.sh" {
			path = "scripts/hooks/observed-test-edit-denial.sh"
		}
		writeFixture(filepath.Join(root, path), body, 0700)
	}
	writeFixture(filepath.Join(root, "scripts/hooks/test-edit-denial.sh"), []byte("#!/bin/sh\n\"$SOURCE_STRUCTURAL_GATE\" \"$@\"\nstatus=$?\nprintf '%s\\000%s\\000%s\\000' \"$FACTORY_AGENT_ROLE\" \"$status\" \"$FACTORY_CONFIG\" >> \"$SOURCE_STRUCTURAL_PROBES\"\nexit \"$status\"\n"), 0700)
	writeFixture(filepath.Join(root, "factory.yaml"), []byte("test_file_patterns: ''\n"), 0600)
	jq, err := exec.LookPath("jq")
	Expect(err).NotTo(HaveOccurred())
	jq, err = filepath.EvalSymlinks(jq)
	Expect(err).NotTo(HaveOccurred())
	return map[string]string{
		"PATH": filepath.Dir(jq) + ":/usr/bin:/bin", "HOME": root, "TMPDIR": base, "BASH_ENV": "", "ENV": "", "GORACE": "atexit_sleep_ms=0",
		"FACTORY_CONFIG": filepath.Join(root, "factory.yaml"), "FACTORY_EVENT_LOG": filepath.Join(base, "gate-events"),
		"SOURCE_STRUCTURAL_GATE": filepath.Join(root, "scripts/hooks/observed-test-edit-denial.sh"), "SOURCE_STRUCTURAL_PROBES": filepath.Join(base, "gate-probes"),
		"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull,
	}
}

// The owner witness delegates to the actual compiled evaluator.
// per docs/adr/0134-go-source-structural-evaluation.md:39
func sourceStructuralNativeFixture() (string, map[string]string) {
	GinkgoHelper()
	root, _ := fixtureDirectories()
	base := filepath.Dir(root)
	environment := sourceStructuralProject(root, base)
	sourceStructuralAdapter(root)
	environment["SOURCE_STRUCTURAL_NATIVE"] = filepath.Join(base, "native evaluator")
	environment["SOURCE_STRUCTURAL_OWNER"] = filepath.Join(base, "owner calls")
	writeFixture(environment["SOURCE_STRUCTURAL_NATIVE"], candidateBinary, 0700)
	writeFixture(filepath.Join(root, "factory"), []byte("#!/bin/sh\nprintf '%s\\000' \"$@\" >> \"$SOURCE_STRUCTURAL_OWNER\"\nexec \"$SOURCE_STRUCTURAL_NATIVE\" \"$@\"\n"), 0700)
	return root, environment
}

// Role results and temporary-config custody are independent of evaluator text.
// per docs/adr/0134-go-source-structural-evaluation.md:25
func sourceStructuralProbes(environment map[string]string, calls int) {
	GinkgoHelper()
	raw, err := os.ReadFile(environment["SOURCE_STRUCTURAL_PROBES"])
	Expect(err).NotTo(HaveOccurred())
	fields := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
	Expect(fields).To(HaveLen(calls * 6))
	entries, err := os.ReadDir(environment["TMPDIR"])
	Expect(err).NotTo(HaveOccurred())
	present := map[string]bool{}
	for _, entry := range entries {
		present[entry.Name()] = true
	}
	for offset := 0; offset < len(fields); offset += 6 {
		Expect(fields[offset : offset+2]).To(Equal([]string{"implementer", "2"}))
		Expect(fields[offset+3 : offset+5]).To(Equal([]string{"spec-writer", "0"}))
		for _, config := range []string{fields[offset+2], fields[offset+5]} {
			Expect(config).NotTo(Equal(environment["FACTORY_CONFIG"]))
			Expect(filepath.Dir(config)).To(Equal(environment["TMPDIR"]))
			Expect(present).NotTo(HaveKey(filepath.Base(config)), "the evaluation must remove its private probe configuration, including any symlink entry")
		}
	}
	Expect(os.ReadFile(environment["FACTORY_CONFIG"])).To(Equal([]byte("test_file_patterns: ''\n")))
}

func sourceStructuralSuccess(harness string) string {
	return "harness-structural-eval: PASS harness=" + harness + "\nharness-structural-eval: live in-harness parity remains a separate OBSERVED gate\n"
}

var _ = Describe("Go source runtime entrypoints structural evaluation adapter", func() {
	// per docs/adr/0134-go-source-structural-evaluation.md:17
	// per docs/adr/0134-go-source-structural-evaluation.md:20
	DescribeTable("preserves literal channels and the physical owner through directory aliases", func(bash bool) {
		for _, alias := range []bool{false, true} {
			root, caller := fixtureDirectories()
			base := filepath.Dir(root)
			owner := filepath.Join(base, "physical owner ' $(not-code) \n")
			Expect(os.Rename(root, owner)).To(Succeed())
			program := sourceStructuralAdapter(owner)
			record, capturedInput, decoy := filepath.Join(base, "argv"), filepath.Join(base, "input"), filepath.Join(base, "DECOY_CALLED")
			writeFixture(filepath.Join(owner, "factory"), []byte("#!/bin/sh\nprintf '%s\\000' \"$PWD\" \"$SOURCE_ENTRY_LITERAL_ENV\" \"$@\" > \"$SOURCE_ENTRY_ARGV\"\n/bin/cat > \"$SOURCE_ENTRY_INPUT\"\nprintf 'owner stdout\\n'\nprintf 'owner stderr\\n' >&2\nexit 63\n"), 0700)
			foreign, tools := filepath.Join(base, "foreign"), filepath.Join(base, "tools")
			for _, path := range []string{filepath.Join(foreign, "factory"), filepath.Join(caller, "factory"), filepath.Join(tools, "factory")} {
				writeFixture(path, []byte("#!/bin/sh\nprintf called > \"$SOURCE_ENTRY_DECOY\"\nexit 64\n"), 0700)
			}
			if alias {
				Expect(os.Symlink(filepath.Join(owner, "scripts"), filepath.Join(foreign, "scripts"))).To(Succeed())
				program = filepath.Join(foreign, "scripts/harness-structural-eval.sh")
			}
			environment := map[string]string{
				"PATH": tools + ":/usr/bin:/bin", "BASH_ENV": "", "ENV": "",
				"SOURCE_ENTRY_ARGV": record, "SOURCE_ENTRY_INPUT": capturedInput, "SOURCE_ENTRY_DECOY": decoy,
				"SOURCE_ENTRY_LITERAL_ENV": "literal $(not-code)\nvalue\n", "FACTORY_RUNTIME_BINARY": filepath.Join(foreign, "factory"), "FACTORY_HOME": foreign,
			}
			operands := []string{"", "space and ' quote", "literal $(not-code)\nline", "--", "--harness=codex"}
			input := "borrowed stdin\x00literal ' $(not-code)\n"
			beforeOwner, beforeCaller := installationSnapshot(owner), installationSnapshot(caller)
			observed := sourceStructuralInvoke(caller, environment, input, program, bash, operands...)
			Expect(observed).To(Equal(cliResult{"owner stdout\n", "owner stderr\n", 63}), "bash=%t directory_alias=%t", bash, alias)
			raw, err := os.ReadFile(record)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.HasSuffix(string(raw), "\x00")).To(BeTrue())
			Expect(strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")).To(Equal(append([]string{caller, environment["SOURCE_ENTRY_LITERAL_ENV"], "eval", "structural"}, operands...)))
			Expect(os.ReadFile(capturedInput)).To(Equal([]byte(input)))
			_, err = os.Lstat(decoy)
			Expect(os.IsNotExist(err)).To(BeTrue())
			Expect(installationSnapshot(owner)).To(Equal(beforeOwner))
			Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
		}
	}, Entry("direct executable", false), Entry("explicit Bash", true))

	// per docs/adr/0134-go-source-structural-evaluation.md:20
	It("refuses file aliases and missing owners and preserves failed-owner channels without fallback", func() {
		for _, mode := range []string{"missing-owner", "failed-owner", "file-alias"} {
			for _, bash := range []bool{false, true} {
				owner, caller := fixtureDirectories()
				base := filepath.Dir(owner)
				environment := sourceStructuralProject(caller, base)
				program := sourceStructuralAdapter(owner)
				called := filepath.Join(base, "owner-called")
				environment["SOURCE_STRUCTURAL_CALLED"] = called
				if mode != "missing-owner" {
					writeFixture(filepath.Join(owner, "factory"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_STRUCTURAL_CALLED\"\nprintf 'owner stdout\\n'\nprintf 'owner stderr\\n' >&2\nexit 47\n"), 0700)
				}
				if mode == "file-alias" {
					alias := filepath.Join(caller, "aliased structural evaluator")
					Expect(os.Symlink(program, alias)).To(Succeed())
					program = alias
				}
				observed := sourceStructuralInvoke(caller, environment, "", program, bash, "--harness=opencode")
				if mode == "failed-owner" {
					Expect(observed).To(Equal(cliResult{"owner stdout\n", "owner stderr\n", 47}))
					Expect(os.ReadFile(called)).To(Equal([]byte("called")))
				} else {
					Expect(observed.status).NotTo(BeZero(), "%s bash=%t: %+v", mode, bash, observed)
					Expect(observed.stdout).To(BeEmpty())
					Expect(observed.stderr).NotTo(BeEmpty())
					if mode == "file-alias" {
						Expect(observed.status).To(Equal(2))
						Expect(observed.stderr).To(ContainSubstring("file symlink"))
					}
					_, err := os.Lstat(called)
					Expect(os.IsNotExist(err)).To(BeTrue())
				}
				_, err := os.Lstat(environment["SOURCE_STRUCTURAL_PROBES"])
				Expect(os.IsNotExist(err)).To(BeTrue(), "refused owner selection must not fall through to the legacy role probes")
			}
		}
	})

	// per docs/adr/0134-go-source-structural-evaluation.md:24
	// per docs/adr/0134-go-source-structural-evaluation.md:53
	It("compiles its private source owner and evaluates all three caller-owned real denial gates", func() {
		source := newSourceEntrypointFixture()
		base := filepath.Dir(source.root)
		project := sourceStructuralProject(source.caller, base)
		projectTools := filepath.Join(base, "project tools")
		decoy := filepath.Join(base, "project compiler called")
		writeFixture(filepath.Join(projectTools, "go"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_STRUCTURAL_DECOY_GO\"\nprintf 'unexpected project Go compiler\\n' >&2\nexit 83\n"), 0700)
		project["PATH"] = projectTools + string(os.PathListSeparator) + project["PATH"]
		project["SOURCE_STRUCTURAL_DECOY_GO"] = decoy
		environment := maps.Clone(source.environment)
		for name, value := range project {
			environment[name] = value
		}
		environment["PATH"] = source.environment["PATH"] + string(os.PathListSeparator) + project["PATH"]
		compiler, err := exec.LookPath("go")
		Expect(err).NotTo(HaveOccurred())
		compiler, err = filepath.EvalSymlinks(compiler)
		Expect(err).NotTo(HaveOccurred())
		cacheQuery := exec.Command(compiler, "env", "-json", "GOCACHE", "GOMODCACHE") // #nosec G204 -- evaluator-selected compiler, fixed read-only environment query before project tools enter PATH.
		cacheQuery.Env = append(os.Environ(), "GOTOOLCHAIN=local")
		cacheBytes, err := cacheQuery.Output()
		Expect(err).NotTo(HaveOccurred())
		var caches map[string]string
		Expect(json.Unmarshal(cacheBytes, &caches)).To(Succeed())
		for _, name := range []string{"GOCACHE", "GOMODCACHE"} {
			Expect(filepath.IsAbs(caches[name])).To(BeTrue(), "%s must be an explicit evaluator cache", name)
			environment[name] = caches[name]
		}
		environment["GOENV"] = "off"
		program := sourceStructuralAdapter(source.root)
		before := sourceEntrypointSnapshot(source.root)
		for index, harness := range []string{"opencode", "claude", "codex"} {
			observed := sourceStructuralInvoke(source.caller, environment, "borrowed stdin\n", program, index%2 != 0, "--harness="+harness)
			Expect(observed).To(Equal(cliResult{sourceStructuralSuccess(harness), "", 0}))
			if index == 0 {
				after := sourceEntrypointSnapshot(source.root)
				for path, snapshot := range before {
					Expect(after[path]).To(Equal(snapshot), "source path %s must remain unchanged", path)
				}
				Expect(after).To(HaveLen(len(before)))
				compiled, err := os.ReadFile(filepath.Join(source.root, "factory-go"))
				Expect(err).NotTo(HaveOccurred(), "the first source invocation must actually compile its owning runtime")
				Expect(compiled).NotTo(BeEmpty())
				writeFixture(filepath.Join(source.root, "factory"), compiled, 0700)
			}
		}
		sourceStructuralProbes(environment, 3)
		for _, bash := range []bool{false, true} {
			for _, args := range [][]string{nil, {"--harness=bogus"}} {
				observed := sourceStructuralInvoke(source.caller, environment, "", program, bash, args...)
				Expect(observed).To(Equal(cliResult{"", "usage: harness-structural-eval.sh --harness=opencode|claude|codex\n", 2}))
			}
		}
		sourceStructuralProbes(environment, 3)
		_, err = os.Lstat(decoy)
		Expect(os.IsNotExist(err)).To(BeTrue(), "project tooling must not shadow the evaluator-selected source compiler")
	})

	// per docs/adr/0134-go-source-structural-evaluation.md:39
	It("runs the actual make eval recipe through the native owner", func() {
		root, environment := sourceStructuralNativeFixture()
		makefile, err := os.ReadFile("../Makefile")
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(root, "Makefile"), makefile, 0600)
		observed := handoffNativeExactProcess(root, environment, "", "/usr/bin/make", "eval")
		Expect(observed.status).To(BeZero(), "%+v", observed)
		Expect(observed.stderr).To(BeEmpty())
		for _, harness := range []string{"opencode", "claude", "codex"} {
			Expect(observed.stdout).To(ContainSubstring(sourceStructuralSuccess(harness)))
		}
		Expect(os.ReadFile(environment["SOURCE_STRUCTURAL_OWNER"])).To(Equal([]byte("eval\x00structural\x00--harness=opencode\x00eval\x00structural\x00--harness=claude\x00eval\x00structural\x00--harness=codex\x00")))
		sourceStructuralProbes(environment, 3)
	})

	// per docs/adr/0134-go-source-structural-evaluation.md:39
	It("executes the actual structural steps from every language-pack workflow with a native runtime", func() {
		root, environment := sourceStructuralNativeFixture()
		type defaults struct {
			Run map[string]any `yaml:"run"`
		}
		type step struct {
			sourceGateWorkflowStep `yaml:",inline"`
			Shell                  *string `yaml:"shell"`
		}
		for _, path := range []string{"packs/go/workflows/ci.yml", "packs/typescript/workflows/ci.yml", "packs/java/workflows/ci.yml", "packs/java/maven/workflows/ci.yml"} {
			body, err := os.ReadFile(filepath.Join("..", path))
			Expect(err).NotTo(HaveOccurred())
			var workflow struct {
				Defaults defaults `yaml:"defaults"`
				Jobs     map[string]struct {
					Defaults defaults `yaml:"defaults"`
					Steps    []step   `yaml:"steps"`
				} `yaml:"jobs"`
			}
			Expect(yaml.Unmarshal(body, &workflow)).To(Succeed())
			Expect(workflow.Defaults.Run).NotTo(HaveKey("shell"))
			var selected []sourceGateWorkflowStep
			for _, job := range workflow.Jobs {
				for _, step := range job.Steps {
					if strings.Contains(step.Run, "harness-structural-eval.sh") {
						Expect(job.Defaults.Run).NotTo(HaveKey("shell"))
						Expect(step.Shell).To(BeNil())
						selected = append(selected, step.sourceGateWorkflowStep)
					}
				}
			}
			Expect(selected).To(HaveLen(1), "%s must retain its structural consumer", path)
			Expect(selected[0].If).To(BeEmpty())
			Expect(selected[0].Env).To(BeEmpty())
			script := filepath.Join(filepath.Dir(root), "pack structural step")
			writeFixture(script, []byte(selected[0].Run), 0600)
			observed := handoffNativeExactProcess(root, environment, "", "/bin/bash", "-e", script)
			Expect(observed).To(Equal(cliResult{sourceStructuralSuccess("opencode") + sourceStructuralSuccess("claude") + sourceStructuralSuccess("codex"), "", 0}), path)
			Expect(os.ReadFile(environment["SOURCE_STRUCTURAL_OWNER"])).To(Equal([]byte("eval\x00structural\x00--harness=opencode\x00eval\x00structural\x00--harness=claude\x00eval\x00structural\x00--harness=codex\x00")))
			sourceStructuralProbes(environment, 3)
			Expect(os.Remove(environment["SOURCE_STRUCTURAL_OWNER"])).To(Succeed())
			Expect(os.Remove(environment["SOURCE_STRUCTURAL_PROBES"])).To(Succeed())
		}
	})

	// per docs/adr/0134-go-source-structural-evaluation.md:44
	It("prepares Go and locked modules before selftest and structural source use", func() {
		command := exec.Command("git", "show", sourceStructuralBaseline+":.github/workflows/ci.yml") // #nosec G204 -- fixed independently identified merged workflow.
		command.Dir = ".."
		baselineBytes, err := command.Output()
		Expect(err).NotTo(HaveOccurred())
		candidateBytes, err := os.ReadFile("../.github/workflows/ci.yml")
		Expect(err).NotTo(HaveOccurred())
		var baseline, candidate map[string]any
		Expect(yaml.Unmarshal(baselineBytes, &baseline)).To(Succeed())
		Expect(yaml.Unmarshal(candidateBytes, &candidate)).To(Succeed())
		normalize := func(workflow map[string]any, checkOrder bool) (map[string]any, map[string]any) {
			job := workflow["jobs"].(map[string]any)["selftest"].(map[string]any)
			var setup, acquisition map[string]any
			setupIndex, acquisitionIndex, structuralIndex, firstSourceIndex := -1, -1, -1, -1
			for index, value := range job["steps"].([]any) {
				step := value.(map[string]any)
				uses, _ := step["uses"].(string)
				run, _ := step["run"].(string)
				if firstSourceIndex < 0 && (strings.Contains(run, "./scripts/selftest/") || run == "make eval") {
					firstSourceIndex = index
				}
				switch {
				case strings.HasPrefix(uses, "actions/setup-go@"):
					Expect(setup).To(BeNil())
					setup, setupIndex = step, index
				case step["name"] == "Provision locked source runtime modules":
					Expect(acquisition).To(BeNil())
					acquisition, acquisitionIndex = step, index
				default:
					if step["run"] == "make eval" {
						Expect(structuralIndex).To(Equal(-1))
						structuralIndex = index
					}
				}
			}
			Expect(setup).NotTo(BeNil())
			Expect(acquisition).NotTo(BeNil())
			Expect(structuralIndex).To(BeNumerically(">=", 0))
			Expect(firstSourceIndex).To(BeNumerically(">=", 0))
			if checkOrder {
				Expect(setupIndex).To(BeNumerically("<", acquisitionIndex))
				Expect(acquisitionIndex).To(BeNumerically("<", firstSourceIndex), "the first selftest or structural source invocation needs its compiler and acquired locked modules")
			}
			delete(setup, "name") // Its label may describe its broader prerequisite role.
			return setup, acquisition
		}
		oldSetup, oldAcquisition := normalize(baseline, false)
		newSetup, newAcquisition := normalize(candidate, true)
		Expect(newSetup).To(Equal(oldSetup), "preserve the existing setup action and its configuration")
		Expect(newAcquisition).To(Equal(oldAcquisition), "preserve acquisition guards, locked download and go.mod/go.sum checks")
	})
})
