package acceptance_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The executable subject is always the current source adapter.
// per docs/adr/0128-go-source-harness-sync-entrypoints.md:58
func sourceSyncAdapter(root, harness string) string {
	GinkgoHelper()
	path := filepath.Join("scripts", "sync-"+harness+".sh")
	body, err := os.ReadFile(filepath.Join("..", path))
	Expect(err).NotTo(HaveOccurred())
	writeFixture(filepath.Join(root, path), body, 0755)
	return filepath.Join(root, path)
}

// Current native delegation must not derive credit from the old shell renderer.
// per docs/adr/0128-go-source-harness-sync-entrypoints.md:24
func sourceSyncPoison(root string) {
	GinkgoHelper()
	for _, name := range []string{"config.sh", "roles.sh"} {
		writeFixture(filepath.Join(root, "scripts/lib", name), []byte("#!/bin/sh\nprintf called > \"$SOURCE_SYNC_FALLBACK\"\nexit 94\n"), 0700)
	}
}

// Only the selected harness's existing publication surface may change.
// per docs/adr/0128-go-source-harness-sync-entrypoints.md:31
func sourceSyncInputs(root, harness string) map[string]string {
	GinkgoHelper()
	files := sourceEntrypointSnapshot(root)
	for path := range files {
		if (harness == "opencode" && (path == "opencode.json" || strings.HasPrefix(path, ".opencode/agent/"))) ||
			(harness == "claude" && (path == ".claude" || strings.HasPrefix(path, ".claude/") || path == ".mcp.json" || path == "CLAUDE.md" || path == "CLAUDE.md.replaced-by-symlink")) ||
			(harness == "codex" && (path == ".codex" || strings.HasPrefix(path, ".codex/"))) {
			delete(files, path)
		}
	}
	return files
}

// Synthetic tiers distinguish selected configuration from the output owner.
// per docs/adr/0128-go-source-harness-sync-entrypoints.md:31
func sourceSyncModels(model string) []byte {
	var lines []string
	for _, harness := range []string{"opencode", "claude", "codex"} {
		for _, tier := range []string{"default", "frontier", "economy"} {
			lines = append(lines, harness+"_"+tier+"_model: "+model)
		}
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

var _ = Describe("Go source runtime entrypoints harness synchronization adapters", func() {
	// per docs/adr/0128-go-source-harness-sync-entrypoints.md:26
	// per docs/adr/0128-go-source-harness-sync-entrypoints.md:31
	// per docs/adr/0128-go-source-harness-sync-entrypoints.md:58
	// per docs/adr/0128-go-source-harness-sync-entrypoints.md:92
	It("renders every harness natively in the owner using caller configuration while ignoring direct operands", func() {
		for _, harness := range []string{"opencode", "claude", "codex"} {
			root, caller, inherited := syncFixture()
			environment := map[string]string{}
			for _, entry := range inherited {
				key, value, _ := strings.Cut(entry, "=")
				environment[key] = value
			}
			if harness == "opencode" {
				// Reuse the qualified full source closure for exactly one real compiler invocation.
				// per docs/adr/0128-go-source-harness-sync-entrypoints.md:40
				source := newSourceEntrypointFixture()
				paths := []string{"opencode.json", "AGENTS.md"}
				for _, role := range syncRoleNames {
					paths = append(paths, filepath.Join(".opencode/agent", role+".md"))
				}
				for _, path := range paths {
					data, err := os.ReadFile(filepath.Join(root, path))
					Expect(err).NotTo(HaveOccurred())
					writeFixture(filepath.Join(source.root, path), data, 0600)
				}
				root = source.root
				for key, value := range source.environment {
					environment[key] = value
				}
			}
			environment["BASH_ENV"], environment["ENV"] = "", ""
			marker := filepath.Join(filepath.Dir(root), "legacy sync called")
			environment["SOURCE_SYNC_FALLBACK"] = marker
			program := sourceSyncAdapter(root, harness)
			sourceSyncPoison(root)
			writeFixture(filepath.Join(root, "factory.yaml"), sourceSyncModels("wrong-owner-model"), 0600)
			writeFixture(filepath.Join(caller, "factory.yaml"), sourceSyncModels("caller-model"), 0600)
			doctorGit(caller, inherited, "init", "-q")
			nested := filepath.Join(caller, "nested original cwd")
			Expect(os.Mkdir(nested, 0700)).To(Succeed())
			expected := "caller-model"
			if harness == "codex" {
				selected := filepath.Join(caller, "explicit config.yaml")
				writeFixture(selected, sourceSyncModels("explicit-model"), 0600)
				environment["FACTORY_CONFIG"] = selected
				expected = "explicit-model"
			}
			beforeInputs, beforeCaller := sourceSyncInputs(root, harness), installationSnapshot(caller)
			observed := handoffNativeExactProcess(nested, environment, "borrowed application input\n", program, "--help", "unknown selector", "", "--", "$(not-code)")
			AddReportEntry("source-sync.native-render", map[string]any{"harness": harness, "status": observed.status, "stdout": observed.stdout, "stderr": observed.stderr, "source_launcher": harness == "opencode", "expected_model": expected}, ReportEntryVisibilityNever)
			Expect(sourceSyncInputs(root, harness)).To(Equal(beforeInputs))
			Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
			Expect(observed.status).To(Equal(0), "harness=%s: %+v", harness, observed)
			Expect(observed.stderr).To(BeEmpty())
			Expect(observed.stdout).To(ContainSubstring("sync-" + harness + ":"))
			syncAssertModels(root, harness, syncSameModels(expected))
			_, err := os.Lstat(marker)
			Expect(os.IsNotExist(err)).To(BeTrue(), "the current adapter must invoke the native renderer")
			if harness == "opencode" {
				cache, err := os.Stat(filepath.Join(root, "factory-go"))
				Expect(err).NotTo(HaveOccurred())
				Expect(cache.Mode().IsRegular()).To(BeTrue())
			}
		}
	})

	// per docs/adr/0128-go-source-harness-sync-entrypoints.md:19
	// per docs/adr/0128-go-source-harness-sync-entrypoints.md:26
	It("preserves fixed sync and harness transport through physical owners while ignoring all direct operands", func() {
		operands := []string{"--help", "", "space and ' quote", "literal $(not-code)\nline", "--", "--json"}
		input := "original stdin\nliteral $(not-code)\x00tail\n"
		var violations []string
		for _, verb := range []string{"opencode", "claude", "codex"} {
			for _, alias := range []bool{false, true} {
				root, caller := fixtureDirectories()
				base := filepath.Dir(root)
				owner := filepath.Join(base, "physical owner ' $(not-code) \n")
				Expect(os.Rename(root, owner)).To(Succeed())
				physicalCaller := filepath.Join(base, "caller ' $(not-code) \n")
				Expect(os.Rename(caller, physicalCaller)).To(Succeed())
				program := sourceSyncAdapter(owner, verb)
				writeFixture(filepath.Join(owner, "factory"), []byte(sourceEntrypointRecorder), 0755)
				record, capturedInput := filepath.Join(base, "argv"), filepath.Join(base, "input")
				decoy := filepath.Join(base, "DECOY_EXECUTED")
				foreign := filepath.Join(base, "foreign logical owner")
				tools := filepath.Join(base, "caller tools")
				decoyBody := []byte("#!/bin/sh\nprintf called > \"$SOURCE_ENTRY_DECOY\"\nexit 64\n")
				for _, path := range []string{filepath.Join(foreign, "factory"), filepath.Join(physicalCaller, "factory"), filepath.Join(tools, "factory")} {
					writeFixture(path, decoyBody, 0755)
				}
				environment := map[string]string{
					"PATH": tools + string(os.PathListSeparator) + "/usr/bin:/bin", "BASH_ENV": "", "ENV": "",
					"SOURCE_ENTRY_ARGV": record, "SOURCE_ENTRY_INPUT": capturedInput, "SOURCE_ENTRY_DECOY": decoy,
					"SOURCE_ENTRY_LITERAL_ENV": "literal environment $(not-code)\nvalue\n",
					"FACTORY_RUNTIME_BINARY":   filepath.Join(foreign, "factory"), "FACTORY_HOME": foreign,
				}
				if alias {
					Expect(os.Symlink(filepath.Join(owner, "scripts"), filepath.Join(foreign, "scripts"))).To(Succeed())
					program = filepath.Join(foreign, "scripts", "sync-"+verb+".sh")
				}
				beforeOwner, beforeCaller, beforeForeign := installationSnapshot(owner), installationSnapshot(physicalCaller), installationSnapshot(foreign)
				observed := handoffNativeProcess(physicalCaller, environment, input, program, operands...)
				Expect(installationSnapshot(owner)).To(Equal(beforeOwner))
				Expect(installationSnapshot(physicalCaller)).To(Equal(beforeCaller))
				Expect(installationSnapshot(foreign)).To(Equal(beforeForeign))
				raw, recordErr := os.ReadFile(record)
				stdin, inputErr := os.ReadFile(capturedInput)
				fields := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
				_, decoyErr := os.Lstat(decoy)
				AddReportEntry("source-sync.literal-transport", map[string]any{"verb": verb, "directory_alias": alias, "status": observed.status, "argv": fields, "stdin": string(stdin), "stderr": observed.stderr, "decoy_executed": decoyErr == nil}, ReportEntryVisibilityNever)
				violations = append(violations, InterceptGomegaFailures(func() {
					Expect(observed).To(Equal(cliResult{"", "", 63}), "verb=%s directory_alias=%t", verb, alias)
					Expect(recordErr).NotTo(HaveOccurred())
					Expect(inputErr).NotTo(HaveOccurred())
					Expect(strings.HasSuffix(string(raw), "\x00")).To(BeTrue())
					Expect(fields).To(Equal([]string{physicalCaller, environment["SOURCE_ENTRY_LITERAL_ENV"], "sync", verb}))
					Expect(string(stdin)).To(Equal(input))
					Expect(os.IsNotExist(decoyErr)).To(BeTrue())
				})...)
			}
		}
		AddReportEntry("source-sync.transport-violations", violations, ReportEntryVisibilityNever)
		Expect(violations).To(BeEmpty())
	})

	// per docs/adr/0128-go-source-harness-sync-entrypoints.md:19
	// per docs/adr/0128-go-source-harness-sync-entrypoints.md:24
	It("refuses file aliases and missing owners and propagates failed owners without fallback", func() {
		var violations []string
		for _, verb := range []string{"opencode", "claude", "codex"} {
			for _, mode := range []string{"missing-owner", "failed-owner", "file-alias"} {
				root, caller := fixtureDirectories()
				base := filepath.Dir(root)
				program := sourceSyncAdapter(root, verb)
				marker, ownerCalled := filepath.Join(base, "FALLBACK_CALLED"), filepath.Join(base, "OWNER_CALLED")
				sourceSyncPoison(root)
				if mode != "missing-owner" {
					writeFixture(filepath.Join(root, "factory"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_SYNC_OWNER\"\nprintf 'native output\\n'\nprintf 'native error\\n' >&2\nexit 47\n"), 0755)
				}
				if mode == "file-alias" {
					alias := filepath.Join(caller, "sync-"+verb+".sh")
					Expect(os.Symlink(program, alias)).To(Succeed())
					program = alias
				}
				environment := map[string]string{"PATH": "/usr/bin:/bin", "BASH_ENV": "", "ENV": "", "SOURCE_SYNC_FALLBACK": marker, "SOURCE_SYNC_OWNER": ownerCalled}
				beforeRoot, beforeCaller := installationSnapshot(root), installationSnapshot(caller)
				observed := handoffNativeProcess(caller, environment, "borrowed stdin\n", program)
				Expect(installationSnapshot(root)).To(Equal(beforeRoot))
				Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
				_, fallbackErr := os.Lstat(marker)
				_, ownerErr := os.Lstat(ownerCalled)
				AddReportEntry("source-sync.refusal", map[string]any{"verb": verb, "mode": mode, "status": observed.status, "stdout": observed.stdout, "stderr": observed.stderr, "fallback_called": fallbackErr == nil, "owner_called": ownerErr == nil}, ReportEntryVisibilityNever)
				violations = append(violations, InterceptGomegaFailures(func() {
					Expect(os.IsNotExist(fallbackErr)).To(BeTrue(), "%s/%s must not run a shell fallback", verb, mode)
					switch mode {
					case "failed-owner":
						Expect(observed).To(Equal(cliResult{"native output\n", "native error\n", 47}))
						Expect(ownerErr).NotTo(HaveOccurred())
					case "file-alias":
						Expect(observed.status).To(Equal(2))
						Expect(observed.stdout).To(BeEmpty())
						Expect(observed.stderr).To(ContainSubstring("file symlink"))
						Expect(os.IsNotExist(ownerErr)).To(BeTrue(), "refuse the alias before any owner runs")
					case "missing-owner":
						Expect(observed.status).NotTo(Equal(0))
						Expect(observed.stdout).To(BeEmpty())
						Expect(observed.stderr).NotTo(BeEmpty())
						Expect(os.IsNotExist(ownerErr)).To(BeTrue())
					default:
						Fail(fmt.Sprintf("unknown private case %q", mode))
					}
				})...)
			}
		}
		AddReportEntry("source-sync.refusal-violations", violations, ReportEntryVisibilityNever)
		Expect(violations).To(BeEmpty())
	})
})
