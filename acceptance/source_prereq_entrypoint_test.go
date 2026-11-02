package acceptance_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The prerequisite compatibility path has no factory- prefix.
// per docs/adr/0130-go-source-prerequisite-entrypoint.md:18
func sourcePrereqAdapter(root string) string {
	GinkgoHelper()
	body, err := os.ReadFile("../scripts/prereq-check.sh")
	Expect(err).NotTo(HaveOccurred())
	path := filepath.Join(root, "scripts/prereq-check.sh")
	writeFixture(path, body, 0755)
	return path
}

// Witness existing version fixtures; actual compilation and non-version Git
// commands delegate to qualified binaries. per docs/adr/0130-go-source-prerequisite-entrypoint.md:63
const sourcePrereqTool = `#!/bin/sh
name=${0##*/}
case "$name" in
  git)
    if [ "$#" -ne 1 ] || [ "$1" != --version ]; then exec "$SOURCE_PREREQ_REAL_GIT" "$@"; fi ;;
  go)
    case "${1:-}" in
      build)
        printf '%s\000' "$PWD" "$@" > "$SOURCE_PREREQ_RECORDS/go-build"
        exec "$SOURCE_PREREQ_REAL_GO" "$@" ;;
      version) name=go-version ;;
      *) printf called > "$SOURCE_PREREQ_RECORDS/unexpected-go"; exit 91 ;;
    esac ;;
esac
printf '%s\000' "$PWD" "$@" > "$SOURCE_PREREQ_RECORDS/$name"
if [ "$name" = go-version ]; then name=go; fi
exec "$SOURCE_PREREQ_VERSIONS/$name" "$@"
`

// Ordinary failing version exits do not establish that a tool was unused.
// per docs/adr/0130-go-source-prerequisite-entrypoint.md:65
func sourcePrereqNoProbe(records string, names ...string) {
	GinkgoHelper()
	for _, name := range names {
		_, err := os.Lstat(filepath.Join(records, name))
		Expect(os.IsNotExist(err)).To(BeTrue(), "unexpected prerequisite invocation: %s", name)
	}
}

// Probe operands and original cwd remain literal.
// per docs/adr/0130-go-source-prerequisite-entrypoint.md:29
func sourcePrereqProbe(records, name, cwd, argument string) {
	GinkgoHelper()
	raw, err := os.ReadFile(filepath.Join(records, name))
	Expect(err).NotTo(HaveOccurred())
	Expect(strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")).To(Equal([]string{cwd, argument}))
}

var _ = Describe("Go source runtime entrypoints prerequisite adapter", func() {
	// per docs/adr/0130-go-source-prerequisite-entrypoint.md:25
	// per docs/adr/0130-go-source-prerequisite-entrypoint.md:33
	// per docs/adr/0130-go-source-prerequisite-entrypoint.md:57
	It("uses native caller configuration and witnessed version tools without Python or live harnesses", func() {
		workspace, caller := fixtureDirectories()
		source := newSourceEntrypointFixture()
		f := &orchestrationFixture{root: source.root, repo: caller, bin: filepath.Join(workspace, "tools"), trace: filepath.Join(workspace, "version trace")}
		f.prereqTools()
		versions := filepath.Join(workspace, "version programs")
		Expect(os.Mkdir(versions, 0700)).To(Succeed())
		for _, name := range []string{"git", "go", "opencode", "claude", "codex", "golangci-lint", "jq", "python3", "curl"} {
			Expect(os.Rename(filepath.Join(f.bin, name), filepath.Join(versions, name))).To(Succeed())
			writeFixture(filepath.Join(f.bin, name), []byte(sourcePrereqTool), 0700)
		}
		Expect(os.Remove(filepath.Join(f.bin, "bash"))).To(Succeed())
		Expect(os.Symlink("/bin/bash", filepath.Join(f.bin, "bash"))).To(Succeed())
		realGo, err := exec.LookPath("go")
		Expect(err).NotTo(HaveOccurred())
		realGit, err := exec.LookPath("git")
		Expect(err).NotTo(HaveOccurred())
		environment := map[string]string{}
		for _, entry := range f.env {
			key, value, _ := strings.Cut(entry, "=")
			if key != "FACTORY_CONFIG" && key != "FACTORY_RUNTIME_BINARY" {
				environment[key] = value
			}
		}
		for key, value := range source.environment {
			if key != "PATH" {
				environment[key] = value
			}
		}
		environment["PATH"] = f.bin + string(os.PathListSeparator) + "/usr/bin:/bin"
		environment["BASH_ENV"], environment["ENV"] = "", ""
		environment["SOURCE_PREREQ_REAL_GO"], environment["SOURCE_PREREQ_REAL_GIT"] = realGo, realGit
		environment["SOURCE_PREREQ_VERSIONS"] = versions
		// An inherited Go minimum must not override the selected config file.
		environment["GO_MIN_VERSION"] = "999.0.0"
		program := sourcePrereqAdapter(source.root)
		sourceObservabilityPoison(source.root)
		environment["SOURCE_OBSERVABILITY_FALLBACK"] = filepath.Join(workspace, "LEGACY_PREREQ_CALLED")
		writeFixture(filepath.Join(source.root, "factory.yaml"), []byte("go_min_version: 999.0.0\n"), 0600)
		writeFixture(filepath.Join(caller, "factory.yaml"), []byte("factory_harness: codex\n"), 0600)
		orchestrationGit(caller, "init", "-q")
		nested := filepath.Join(caller, "nested original cwd")
		Expect(os.Mkdir(nested, 0700)).To(Succeed())
		writeFixture(filepath.Join(nested, "factory.yaml"), []byte("go_min_version: 999.0.0\n"), 0600)
		Expect(os.Symlink(f.bin, filepath.Join(nested, "relative-tools"))).To(Succeed())
		writeFixture(filepath.Join(source.root, "selected.yaml"), []byte("go_min_version: 999.0.0\n"), 0600)
		writeFixture(filepath.Join(caller, "selected.yaml"), []byte("go_min_version: 999.0.0\n"), 0600)
		writeFixture(filepath.Join(nested, "selected.yaml"), []byte("go_min_version: 1.27.2\nfactory_harness: codex\n"), 0600)
		for _, phase := range []string{"source-default", "cwd-explicit", "missing-harness"} {
			records := filepath.Join(workspace, "records", phase)
			Expect(os.MkdirAll(records, 0700)).To(Succeed())
			environment["SOURCE_PREREQ_RECORDS"] = records
			if phase != "source-default" {
				writeFixture(filepath.Join(source.root, "factory"), candidateBinary, 0755)
				environment["PATH"] = "relative-tools"
				environment["FACTORY_CONFIG"], environment["FACTORY_HARNESS"] = "selected.yaml", "claude"
				if phase == "missing-harness" {
					Expect(os.Remove(filepath.Join(f.bin, "claude"))).To(Succeed())
				}
			}
			beforeOwner, beforeCaller := sourceEntrypointSnapshot(source.root), installationSnapshot(caller)
			observed := handoffNativeExactProcess(nested, environment, "borrowed caller input\n", program, "--help", "", "literal $(not-code)\nline")
			AddReportEntry("source-prereq.native", map[string]any{"phase": phase, "owner": source.root, "caller": caller, "cwd": nested, "status": observed.status, "stdout": observed.stdout, "stderr": observed.stderr}, ReportEntryVisibilityNever)
			Expect(sourceEntrypointSnapshot(source.root)).To(Equal(beforeOwner))
			Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
			Expect(observed.stderr).To(BeEmpty())
			Expect(observed.stdout).NotTo(ContainSubstring("ignored second version line"))
			if phase == "missing-harness" {
				Expect(observed.status).To(Equal(1), "%+v", observed)
				Expect(observed.stdout).To(ContainSubstring("PREREQ FAIL: Claude Code not found"))
				Expect(observed.stdout).To(ContainSubstring("prereq-check: 1 tool(s) missing or too old"))
				sourcePrereqNoProbe(records, "claude")
			} else {
				Expect(observed.status).To(Equal(0), "%+v", observed)
				Expect(observed.stdout).To(ContainSubstring("prereq-check: all tools present"))
			}
			for _, name := range []string{"git", "golangci-lint", "jq"} {
				sourcePrereqProbe(records, name, nested, "--version")
			}
			sourcePrereqNoProbe(records, "python3", "curl", "codex", "unexpected-go")
			if phase == "source-default" {
				Expect(observed.stdout).To(ContainSubstring("skip: no go_min_version in factory.yaml"))
				Expect(observed.stdout).To(ContainSubstring("prereq: OK opencode"))
				sourcePrereqProbe(records, "opencode", nested, "--version")
				sourcePrereqNoProbe(records, "claude", "go-version")
				build, err := os.ReadFile(filepath.Join(records, "go-build"))
				Expect(err).NotTo(HaveOccurred(), "source compilation still requires actual Go")
				fields := strings.Split(strings.TrimSuffix(string(build), "\x00"), "\x00")
				Expect(len(fields)).To(BeNumerically(">=", 2))
				Expect(fields[0:2]).To(Equal([]string{source.root, "build"}))
			} else {
				Expect(observed.stdout).To(ContainSubstring("prereq: OK Go (go version go1.27.2 fixture/fixture)"))
				sourcePrereqProbe(records, "go-version", nested, "version")
				sourcePrereqNoProbe(records, "opencode", "go-build")
				if phase == "cwd-explicit" {
					Expect(observed.stdout).To(ContainSubstring("prereq: OK Claude Code"))
					sourcePrereqProbe(records, "claude", nested, "--version")
				}
			}
			_, err := os.Lstat(environment["SOURCE_OBSERVABILITY_FALLBACK"])
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	})

	// per docs/adr/0130-go-source-prerequisite-entrypoint.md:18
	// per docs/adr/0130-go-source-prerequisite-entrypoint.md:20
	It("preserves literal prerequisite transport through physical owners including newline paths", func() {
		operands := []string{"", "space and ' quote", "literal $(not-code)\nline", "--", "--json"}
		input := "original stdin\nliteral $(not-code)\x00tail\n"
		var violations []string
		const verb = "prereq"
		for _, alias := range []bool{false, true} {
			root, caller := fixtureDirectories()
			base := filepath.Dir(root)
			owner := filepath.Join(base, "physical owner ' $(not-code) \n")
			Expect(os.Rename(root, owner)).To(Succeed())
			physicalCaller := filepath.Join(base, "caller ' $(not-code) \n")
			Expect(os.Rename(caller, physicalCaller)).To(Succeed())
			program := sourcePrereqAdapter(owner)
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
				program = filepath.Join(foreign, "scripts", "prereq-check.sh")
			}
			beforeOwner, beforeCaller, beforeForeign := installationSnapshot(owner), installationSnapshot(physicalCaller), installationSnapshot(foreign)
			observed := handoffNativeExactProcess(physicalCaller, environment, input, program, operands...)
			Expect(installationSnapshot(owner)).To(Equal(beforeOwner))
			Expect(installationSnapshot(physicalCaller)).To(Equal(beforeCaller))
			Expect(installationSnapshot(foreign)).To(Equal(beforeForeign))
			raw, recordErr := os.ReadFile(record)
			stdin, inputErr := os.ReadFile(capturedInput)
			fields := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
			_, decoyErr := os.Lstat(decoy)
			AddReportEntry("source-prereq.literal-transport", map[string]any{"verb": verb, "directory_alias": alias, "status": observed.status, "argv": fields, "stdin": string(stdin), "stderr": observed.stderr, "decoy_executed": decoyErr == nil}, ReportEntryVisibilityNever)
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
		AddReportEntry("source-prereq.transport-violations", violations, ReportEntryVisibilityNever)
		Expect(violations).To(BeEmpty())
	})

	// per docs/adr/0130-go-source-prerequisite-entrypoint.md:18
	// per docs/adr/0130-go-source-prerequisite-entrypoint.md:22
	It("refuses file aliases and missing owners and propagates failed owners without fallback", func() {
		var violations []string
		const verb = "prereq"
		for _, mode := range []string{"missing-owner", "failed-owner", "file-alias"} {
			root, caller := fixtureDirectories()
			base := filepath.Dir(root)
			program := sourcePrereqAdapter(root)
			marker, ownerCalled := filepath.Join(base, "FALLBACK_CALLED"), filepath.Join(base, "OWNER_CALLED")
			sourceObservabilityPoison(root)
			if mode != "missing-owner" {
				writeFixture(filepath.Join(root, "factory"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_OBSERVABILITY_OWNER\"\nprintf 'native output\\n'\nprintf 'native error\\n' >&2\nexit 47\n"), 0755)
			}
			if mode == "file-alias" {
				alias := filepath.Join(caller, "prereq-check.sh")
				Expect(os.Symlink(program, alias)).To(Succeed())
				program = alias
			}
			environment := map[string]string{"PATH": "/usr/bin:/bin", "BASH_ENV": "", "ENV": "", "SOURCE_OBSERVABILITY_FALLBACK": marker, "SOURCE_OBSERVABILITY_OWNER": ownerCalled}
			beforeRoot, beforeCaller := installationSnapshot(root), installationSnapshot(caller)
			observed := handoffNativeExactProcess(caller, environment, "borrowed stdin\n", program)
			Expect(installationSnapshot(root)).To(Equal(beforeRoot))
			Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
			_, fallbackErr := os.Lstat(marker)
			_, ownerErr := os.Lstat(ownerCalled)
			AddReportEntry("source-prereq.refusal", map[string]any{"verb": verb, "mode": mode, "status": observed.status, "stdout": observed.stdout, "stderr": observed.stderr, "fallback_called": fallbackErr == nil, "owner_called": ownerErr == nil}, ReportEntryVisibilityNever)
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
		AddReportEntry("source-prereq.refusal-violations", violations, ReportEntryVisibilityNever)
		Expect(violations).To(BeEmpty())
	})
})
