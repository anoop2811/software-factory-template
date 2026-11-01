package acceptance_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Current adapters are always read from this checkout, never a frozen oracle.
// per docs/adr/0127-go-source-observability-entrypoints.md:42
func sourceObservabilityAdapter(root, verb string) string {
	GinkgoHelper()
	path := filepath.Join("scripts", "factory-"+verb+".sh")
	body, err := os.ReadFile(filepath.Join("..", path))
	Expect(err).NotTo(HaveOccurred())
	writeFixture(filepath.Join(root, path), body, 0755)
	return filepath.Join(root, path)
}

// A shell implementation cannot earn native credit by reading the same data.
// per docs/adr/0127-go-source-observability-entrypoints.md:23
func sourceObservabilityPoison(root string) {
	GinkgoHelper()
	for _, library := range []string{"config.sh", "color.sh"} {
		writeFixture(filepath.Join(root, "scripts", "lib", library), []byte("#!/bin/sh\nprintf called > \"$SOURCE_OBSERVABILITY_FALLBACK\"\nexit 94\n"), 0700)
	}
}

var _ = Describe("Go source runtime entrypoints observability adapters", func() {
	// per docs/adr/0127-go-source-observability-entrypoints.md:25
	// per docs/adr/0127-go-source-observability-entrypoints.md:42
	It("uses native report owner events and native metrics caller data without shell libraries or Python", func() {
		f := newSourceEntrypointFixture()
		report := sourceObservabilityAdapter(f.root, "report")
		sourceObservabilityPoison(f.root)
		marker := filepath.Join(filepath.Dir(f.root), "report fallback called")
		f.environment["SOURCE_OBSERVABILITY_FALLBACK"] = marker
		f.environment["FACTORY_CONFIG"] = filepath.Join(f.caller, "factory.yaml")
		writeFixture(filepath.Join(f.caller, "factory.yaml"), []byte("cost_profile: caller-config\n"), 0600)
		writeFixture(filepath.Join(f.root, ".factory/events.log"), []byte("today\towner-only\towner event one\ntoday\towner-only\towner event two\n"), 0600)
		writeFixture(filepath.Join(f.caller, ".factory/events.log"), []byte("today\tcaller-only\tcaller event\n"), 0600)
		beforeRoot, beforeCaller := sourceEntrypointSnapshot(f.root), installationSnapshot(f.caller)
		observed := handoffNativeProcess(f.caller, f.environment, "", report, "--ignored")
		AddReportEntry("source-observability.native-report", map[string]any{"status": observed.status, "stdout": observed.stdout, "stderr": observed.stderr, "source_launcher": filepath.Join(f.root, "factory")}, ReportEntryVisibilityNever)
		Expect(sourceEntrypointSnapshot(f.root)).To(Equal(beforeRoot))
		Expect(installationSnapshot(f.caller)).To(Equal(beforeCaller))
		Expect(observed.status).To(Equal(0), "%+v", observed)
		Expect(observed.stderr).To(BeEmpty())
		Expect(observed.stdout).To(ContainSubstring("Factory report\n"))
		Expect(observed.stdout).To(ContainSubstring("Gate blocks recorded:           2"))
		Expect(observed.stdout).To(ContainSubstring("owner event one"))
		Expect(observed.stdout).NotTo(ContainSubstring("caller event"))
		_, err := os.Lstat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue(), "the current source launcher must execute the native report, never the poisoned shell library")
		cache, err := os.Stat(filepath.Join(f.root, "factory-go"))
		Expect(err).NotTo(HaveOccurred())
		Expect(cache.Mode().IsRegular()).To(BeTrue(), "the actual private source launcher must have built its native owner")

		root, caller, environment := metricsFixture()
		metrics := sourceObservabilityAdapter(root, "metrics")
		sourceObservabilityPoison(root)
		marker = filepath.Join(filepath.Dir(root), "metrics fallback called")
		environment = append(environment, "SOURCE_OBSERVABILITY_FALLBACK="+marker, "BASH_ENV=", "ENV=")
		doctorGit(caller, environment, "init", "-q", "-b", "observability")
		writeFixture(filepath.Join(root, ".factory/events.log"), []byte("today\towner-only\tone\ntoday\towner-only\ttwo\n"), 0600)
		writeFixture(filepath.Join(caller, ".factory/events.log"), []byte(time.Now().UTC().Format("2006-01-02")+"\tcaller-only\tcaller event\n"), 0600)
		nested := filepath.Join(caller, "nested original cwd")
		Expect(os.Mkdir(nested, 0700)).To(Succeed())
		beforeRoot, beforeCaller = installationSnapshot(root), installationSnapshot(caller)
		observed = reportProcess(nested, environment, metrics, []string{"--json", "--days=7"})
		AddReportEntry("source-observability.native-metrics", map[string]any{"status": observed.status, "stdout": observed.stdout, "stderr": observed.stderr, "owner": filepath.Join(root, "factory"), "cwd": nested}, ReportEntryVisibilityNever)
		Expect(installationSnapshot(root)).To(Equal(beforeRoot))
		Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
		Expect(observed.status).To(Equal(0), "%+v", observed)
		Expect(observed.stderr).To(BeEmpty())
		data := metricsJSON(observed.stdout)
		Expect(data["schema"]).To(Equal("factory.metrics/v1"))
		Expect(data["window_days"]).To(Equal(float64(7)))
		enforcement, ok := data["enforcement"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(enforcement["blocks_total"]).To(Equal(float64(1)))
		Expect(enforcement["blocks_by_gate"]).To(Equal([]any{map[string]any{"gate": "caller-only", "blocks": float64(1)}}))
		for _, path := range []string{marker, filepath.Join(caller, "PYTHON_CALLED")} {
			_, err = os.Lstat(path)
			Expect(os.IsNotExist(err)).To(BeTrue(), "the direct adapter must reach the compiled Go metrics owner: %s", path)
		}
	})

	// per docs/adr/0127-go-source-observability-entrypoints.md:18
	// per docs/adr/0127-go-source-observability-entrypoints.md:20
	It("preserves literal transport through the physical report and metrics owners including trailing newlines", func() {
		operands := []string{"", "space and ' quote", "literal $(not-code)\nline", "--", "--json"}
		input := "original stdin\nliteral $(not-code)\x00tail\n"
		var violations []string
		for _, verb := range []string{"report", "metrics"} {
			for _, alias := range []bool{false, true} {
				root, caller := fixtureDirectories()
				base := filepath.Dir(root)
				owner := filepath.Join(base, "physical owner ' $(not-code) \n")
				Expect(os.Rename(root, owner)).To(Succeed())
				physicalCaller := filepath.Join(base, "caller ' $(not-code) \n")
				Expect(os.Rename(caller, physicalCaller)).To(Succeed())
				program := sourceObservabilityAdapter(owner, verb)
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
					program = filepath.Join(foreign, "scripts", "factory-"+verb+".sh")
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
				AddReportEntry("source-observability.literal-transport", map[string]any{"verb": verb, "directory_alias": alias, "status": observed.status, "argv": fields, "stdin": string(stdin), "stderr": observed.stderr, "decoy_executed": decoyErr == nil}, ReportEntryVisibilityNever)
				violations = append(violations, InterceptGomegaFailures(func() {
					Expect(observed).To(Equal(cliResult{"", "", 63}), "verb=%s directory_alias=%t", verb, alias)
					Expect(recordErr).NotTo(HaveOccurred())
					Expect(inputErr).NotTo(HaveOccurred())
					Expect(strings.HasSuffix(string(raw), "\x00")).To(BeTrue())
					Expect(fields).To(Equal(append([]string{physicalCaller, environment["SOURCE_ENTRY_LITERAL_ENV"], verb}, operands...)))
					Expect(string(stdin)).To(Equal(input))
					Expect(os.IsNotExist(decoyErr)).To(BeTrue())
				})...)
			}
		}
		AddReportEntry("source-observability.transport-violations", violations, ReportEntryVisibilityNever)
		Expect(violations).To(BeEmpty())
	})

	// per docs/adr/0127-go-source-observability-entrypoints.md:18
	// per docs/adr/0127-go-source-observability-entrypoints.md:23
	It("refuses file aliases and missing owners and propagates failed owners without fallback", func() {
		var violations []string
		for _, verb := range []string{"report", "metrics"} {
			for _, mode := range []string{"missing-owner", "failed-owner", "file-alias"} {
				root, caller := fixtureDirectories()
				base := filepath.Dir(root)
				program := sourceObservabilityAdapter(root, verb)
				marker, ownerCalled := filepath.Join(base, "FALLBACK_CALLED"), filepath.Join(base, "OWNER_CALLED")
				sourceObservabilityPoison(root)
				if mode != "missing-owner" {
					writeFixture(filepath.Join(root, "factory"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_OBSERVABILITY_OWNER\"\nprintf 'native output\\n'\nprintf 'native error\\n' >&2\nexit 47\n"), 0755)
				}
				if mode == "file-alias" {
					alias := filepath.Join(caller, "factory-"+verb+".sh")
					Expect(os.Symlink(program, alias)).To(Succeed())
					program = alias
				}
				environment := map[string]string{"PATH": "/usr/bin:/bin", "BASH_ENV": "", "ENV": "", "SOURCE_OBSERVABILITY_FALLBACK": marker, "SOURCE_OBSERVABILITY_OWNER": ownerCalled}
				beforeRoot, beforeCaller := installationSnapshot(root), installationSnapshot(caller)
				observed := handoffNativeProcess(caller, environment, "borrowed stdin\n", program)
				Expect(installationSnapshot(root)).To(Equal(beforeRoot))
				Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
				_, fallbackErr := os.Lstat(marker)
				_, ownerErr := os.Lstat(ownerCalled)
				AddReportEntry("source-observability.refusal", map[string]any{"verb": verb, "mode": mode, "status": observed.status, "stdout": observed.stdout, "stderr": observed.stderr, "fallback_called": fallbackErr == nil, "owner_called": ownerErr == nil}, ReportEntryVisibilityNever)
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
		AddReportEntry("source-observability.refusal-violations", violations, ReportEntryVisibilityNever)
		Expect(violations).To(BeEmpty())
	})
})
