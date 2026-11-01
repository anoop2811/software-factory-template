package acceptance_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const sourceEntrypointBaseline = "1754de23de95a7e3c734bedd86894295e823c988"

type sourceEntrypointFixture struct {
	root, caller string
	environment  map[string]string
}

// Source bytes are materialized privately; the operator's developer output is never used.
// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:56
func newSourceEntrypointFixture() sourceEntrypointFixture {
	GinkgoHelper()
	root, caller := fixtureDirectories()
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	imageGit(repository, "clone", "--quiet", "--shared", "--no-checkout", repository, root)
	imageGit(root, "checkout", "--quiet", "--detach", sourceEntrypointBaseline)
	imageGit(root, "config", "gc.auto", "0")
	imageGit(root, "config", "gc.autoDetach", "false")
	imageGit(root, "config", "maintenance.auto", "false")
	imageGit(root, "config", "core.fsmonitor", "false")
	for _, path := range []string{"factory", "scripts/factory-init.sh", "scripts/factory-upgrade.sh", "scripts/hooks/copy-manifest-check.sh", ".gitignore"} {
		data, err := os.ReadFile(filepath.Join(repository, path))
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(root, path), data, 0755)
	}
	for _, directory := range []string{"cmd", "internal"} {
		origin := filepath.Join(repository, directory)
		Expect(filepath.WalkDir(origin, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			relative, err := filepath.Rel(repository, path)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path) // #nosec G122 -- fixed, quiescent cmd/internal trees in the trusted checkout; adversarial mutation begins only in separately materialized private fixtures.
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			writeFixture(filepath.Join(root, relative), data, info.Mode().Perm())
			return nil
		})).To(Succeed())
	}
	compiler, err := exec.LookPath("go")
	Expect(err).NotTo(HaveOccurred())
	compiler, err = filepath.EvalSymlinks(compiler)
	Expect(err).NotTo(HaveOccurred())
	git, err := exec.LookPath("git")
	Expect(err).NotTo(HaveOccurred())
	git, err = filepath.EvalSymlinks(git)
	Expect(err).NotTo(HaveOccurred())
	return sourceEntrypointFixture{root, caller, map[string]string{
		"PATH":        filepath.Dir(compiler) + string(os.PathListSeparator) + filepath.Dir(git) + string(os.PathListSeparator) + "/usr/bin:/bin:/usr/sbin:/sbin",
		"GOTOOLCHAIN": "local", "GOPROXY": "off", "GOSUMDB": "off", "GOWORK": "off", "GOFLAGS": "",
		"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull, "GIT_OPTIONAL_LOCKS": "0",
		"FACTORY_BRIDGE_PROTOCOL": "", "GORACE": "atexit_sleep_ms=0", "BASH_ENV": "", "ENV": "",
	}}
}

// Automatic protocol selection must still prove a real current Go input.
// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:112
func sourceEntrypointAutomaticCopy(f sourceEntrypointFixture) {
	GinkgoHelper()
	const input = ".opencode/package.json"
	environment := maps.Clone(f.environment)
	environment["FACTORY_EVENT_LOG"] = filepath.Join(filepath.Dir(f.root), "automatic copy gate events")
	_, explicit := environment["FACTORY_RUNTIME_BINARY"]
	Expect(explicit).To(BeFalse(), "the current owner protocol, not an explicit runtime, must select this check")
	for _, tracked := range []bool{false, true} {
		if tracked {
			imageGit(f.root, "--literal-pathspecs", "add", "--force", "--", input)
		} else {
			imageGit(f.root, "rm", "--cached", "--force", "--", input)
		}
		data, err := os.ReadFile(filepath.Join(f.root, input))
		Expect(err).NotTo(HaveOccurred())
		Expect(data).NotTo(BeEmpty())
		before, callerBefore := sourceEntrypointSnapshot(f.root), installationSnapshot(f.caller)
		index, err := os.ReadFile(filepath.Join(f.root, ".git/index"))
		Expect(err).NotTo(HaveOccurred())
		observed := handoffNativeProcess(f.caller, environment, "", "/bin/bash", filepath.Join(f.root, "scripts/hooks/copy-manifest-check.sh"), f.root)
		after := sourceEntrypointSnapshot(f.root)
		indexAfter, err := os.ReadFile(filepath.Join(f.root, ".git/index"))
		Expect(err).NotTo(HaveOccurred())
		AddReportEntry("source-entrypoint.automatic-copy", map[string]any{"tracked": tracked, "status": observed.status, "stdout": observed.stdout, "stderr": observed.stderr, "source_preserved": maps.Equal(before, after), "index_before_sha256": fmt.Sprintf("%x", sha256.Sum256(index)), "index_after_sha256": fmt.Sprintf("%x", sha256.Sum256(indexAfter))}, ReportEntryVisibilityNever)
		Expect(after).To(Equal(before), "only ignored developer output and the separately named event log may change")
		Expect(indexAfter).To(Equal(index))
		Expect(installationSnapshot(f.caller)).To(Equal(callerBefore))
		if tracked {
			Expect(observed.status).To(Equal(0), "%+v", observed)
			Expect(observed.stdout).To(ContainSubstring("every native initializer copy input"))
		} else {
			Expect(observed.status).To(Equal(1), "%+v", observed)
			Expect(observed.stderr).To(ContainSubstring("git-tracked"))
		}
	}
}

// Preserve genuine legacy engines at their actual last pre-adapter Git object.
// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:136
func sourceLegacyEntrypointBytes(path string) []byte {
	GinkgoHelper()
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	result := imageProcess(repository, map[string]string{"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull}, "git", "show", sourceEntrypointBaseline+":"+path)
	Expect(result.status).To(Equal(0), "immutable legacy input %s: %+v", path, result)
	data := []byte(result.stdout)
	AddReportEntry("source-entrypoint.historical-input", map[string]string{"revision": sourceEntrypointBaseline, "path": path, "sha256": fmt.Sprintf("%x", sha256.Sum256(data))}, ReportEntryVisibilityNever)
	return data
}

// The named atomic developer output is the only permitted persistent source delta.
// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:45
func sourceEntrypointSnapshot(root string) map[string]string {
	GinkgoHelper()
	snapshot := installationSnapshot(root)
	delete(snapshot, "factory-go")
	return snapshot
}

// Test-owned compiler actions isolate output publication from application execution.
// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:29
// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:43
func sourceEntrypointCompilerRefusals() {
	GinkgoHelper()
	launcher, err := os.ReadFile("../factory")
	Expect(err).NotTo(HaveOccurred())
	for _, mode := range []string{"failed-build", "missing-go", "directory-start", "directory-race", "symlink-race", "file-alias"} {
		root, caller := fixtureDirectories()
		base := filepath.Dir(root)
		tools := filepath.Join(base, "compiler tools")
		record, prior := filepath.Join(base, "compiler argv"), filepath.Join(base, "PRIOR_BINARY_EXECUTED")
		outside := filepath.Join(base, "outside output")
		writeFixture(filepath.Join(outside, "sentinel"), []byte("outside user data\n"), 0600)
		image := filepath.Join(base, "actual native image")
		writeFixture(image, candidateBinary, 0700)
		writeFixture(filepath.Join(root, "factory"), launcher, 0755)
		writeFixture(filepath.Join(root, "factory-go"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_ENTRY_PRIOR\"\nexit 96\n"), 0700)
		writeFixture(filepath.Join(tools, "go"), []byte(sourceEntrypointFailingCompiler), 0700)
		environment := map[string]string{"PATH": tools + string(os.PathListSeparator) + "/usr/bin:/bin", "SOURCE_ENTRY_MODE": mode, "SOURCE_ENTRY_ROOT": root, "SOURCE_ENTRY_IMAGE": image, "SOURCE_ENTRY_BUILD_RECORD": record, "SOURCE_ENTRY_OUTSIDE": outside, "SOURCE_ENTRY_PRIOR": prior, "GORACE": "atexit_sleep_ms=0"}
		program := filepath.Join(root, "factory")
		if mode == "missing-go" {
			// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:234
			Expect(os.Remove(filepath.Join(tools, "go"))).To(Succeed())
			Expect(os.Symlink("/bin/bash", filepath.Join(tools, "bash"))).To(Succeed())
			environment["PATH"] = tools
			environment["BASH_ENV"], environment["ENV"] = "", ""
			lookup := handoffNativeProcess(caller, environment, "", filepath.Join(tools, "bash"), "-c", "command -v go")
			AddReportEntry("source-entrypoint.missing-compiler-lookup", map[string]any{"PATH": environment["PATH"], "status": lookup.status, "stdout": lookup.stdout, "stderr": lookup.stderr}, ReportEntryVisibilityNever)
			Expect(lookup.status).To(Equal(1), "the child utility path must actually exclude every Go compiler")
			Expect(lookup.stdout).To(BeEmpty())
			Expect(lookup.stderr).To(BeEmpty())
		}
		if mode == "directory-start" {
			Expect(os.Remove(filepath.Join(root, "factory-go"))).To(Succeed())
			writeFixture(filepath.Join(root, "factory-go", "sentinel"), []byte("developer-owned directory\n"), 0600)
		}
		if mode == "file-alias" {
			alias := filepath.Join(caller, "foreign alias", "factory")
			Expect(os.MkdirAll(filepath.Dir(alias), 0700)).To(Succeed())
			Expect(os.Symlink(program, alias)).To(Succeed())
			program = alias
		}
		before, callerBefore, outsideBefore := installationSnapshot(root), installationSnapshot(caller), installationSnapshot(outside)
		result := handoffNativeProcess(caller, environment, "application input must remain borrowed\n", program, "help")
		after := installationSnapshot(root)
		if mode == "directory-race" || mode == "symlink-race" {
			delete(before, "factory-go")
			delete(after, "factory-go")
			if mode == "directory-race" {
				entries, err := os.ReadDir(filepath.Join(root, "factory-go"))
				Expect(err).NotTo(HaveOccurred())
				Expect(entries).To(HaveLen(1), "a directory changed during compilation must never become a container for BUILD")
				Expect(entries[0].Name()).To(Equal("sentinel"))
				Expect(os.ReadFile(filepath.Join(root, "factory-go", "sentinel"))).To(Equal([]byte("controlled directory race\n")))
				delete(after, "factory-go/sentinel")
			} else {
				target, err := os.Readlink(filepath.Join(root, "factory-go"))
				Expect(err).NotTo(HaveOccurred())
				Expect(target).To(Equal(outside), "publication must preserve the refused symlink and outside referent")
			}
		}
		AddReportEntry("source-entrypoint.compiler-refusal", map[string]any{"mode": mode, "status": result.status, "stdout": result.stdout, "stderr": result.stderr, "source_matches_expected_output_actor": fmt.Sprint(before) == fmt.Sprint(after)}, ReportEntryVisibilityNever)
		Expect(after).To(Equal(before), "only the separately checked output-race actor may alter this source: %s", mode)
		Expect(installationSnapshot(caller)).To(Equal(callerBefore))
		Expect(installationSnapshot(outside)).To(Equal(outsideBefore))
		_, err = os.Lstat(prior)
		Expect(os.IsNotExist(err)).To(BeTrue(), "failed bootstrap must never execute the cached prior developer binary")
		Expect(result.status).NotTo(Equal(0), mode)
		if mode == "failed-build" {
			Expect(result.status).To(Equal(58))
			Expect(result.stderr).To(ContainSubstring("controlled compiler failure"))
		}
		if mode == "missing-go" {
			Expect(result.status).To(Equal(1))
			Expect(result.stdout).To(BeEmpty())
			Expect(result.stderr).To(Equal("factory: source development requires the qualified Go toolchain; use the authenticated installer for a compiler-free installation\n"))
		}
		if mode == "directory-start" || mode == "missing-go" || mode == "file-alias" {
			_, err = os.Lstat(record)
			Expect(os.IsNotExist(err)).To(BeTrue(), "refuse before the compiler: %s", mode)
		} else {
			raw, err := os.ReadFile(record)
			Expect(err).NotTo(HaveOccurred(), "the controlled compiler must actually run")
			fields := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
			Expect(fields).To(HaveLen(6))
			Expect(fields[0]).To(Equal(root), "compilation belongs to the source, not caller cwd")
			Expect(fields[1:4]).To(Equal([]string{"build", "-mod=readonly", "-o"}))
			Expect(filepath.Dir(fields[4])).To(Equal(root))
			Expect(fields[5]).To(Equal("./cmd/factory"))
		}
	}
}

const sourceEntrypointFailingCompiler = `#!/bin/sh
set -eu
printf '%s\000' "$PWD" "$@" > "$SOURCE_ENTRY_BUILD_RECORD"
[ "$#" -eq 5 ] && [ "$1" = build ] && [ "$2" = -mod=readonly ] && [ "$3" = -o ] && [ "$5" = ./cmd/factory ] || exit 97
if IFS= read -r application_input; then
 printf 'compiler consumed application stdin\n' >&2
 exit 97
fi
if [ "$SOURCE_ENTRY_MODE" = failed-build ]; then printf 'controlled compiler failure\n' >&2; exit 58; fi
case "$SOURCE_ENTRY_MODE" in
 directory-race)
  /bin/rm -f "$SOURCE_ENTRY_ROOT/factory-go"
  /bin/mkdir "$SOURCE_ENTRY_ROOT/factory-go"
  printf 'controlled directory race\n' > "$SOURCE_ENTRY_ROOT/factory-go/sentinel" ;;
 symlink-race)
  /bin/rm -f "$SOURCE_ENTRY_ROOT/factory-go"
  /bin/ln -s "$SOURCE_ENTRY_OUTSIDE" "$SOURCE_ENTRY_ROOT/factory-go" ;;
esac
/bin/cp "$SOURCE_ENTRY_IMAGE" "$4"
/bin/chmod 700 "$4"
`

const sourceEntrypointRecorder = `#!/bin/sh
printf '%s\000' "$PWD" "$SOURCE_ENTRY_LITERAL_ENV" "$@" > "$SOURCE_ENTRY_ARGV"
/bin/cat > "$SOURCE_ENTRY_INPUT"
exit 63
`

var _ = Describe("Go source runtime entrypoints", func() {
	// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:25
	// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:146
	It("enters the existing native command surface from a private source checkout without changing its caller", func() {
		f := newSourceEntrypointFixture()
		control := filepath.Join(filepath.Dir(f.root), "independent native control")
		writeFixture(control, candidateBinary, 0700)
		native := handoffNativeProcess(f.caller, f.environment, "", control, "help")
		Expect(native.status).To(Equal(0), "real compiled native control: %+v", native)
		Expect(native.stdout).To(Equal(nativeCLIUsage))
		before, callerBefore := sourceEntrypointSnapshot(f.root), installationSnapshot(f.caller)
		result := handoffNativeProcess(f.caller, f.environment, "", filepath.Join(f.root, "factory"), "help")
		AddReportEntry("source-entrypoint.native-surface", map[string]any{"status": result.status, "stdout": result.stdout, "stderr": result.stderr, "compiled_control_status": native.status, "baseline": sourceEntrypointBaseline}, ReportEntryVisibilityNever)
		Expect(sourceEntrypointSnapshot(f.root)).To(Equal(before), "compilation must leave no private build/admission/startup files")
		Expect(installationSnapshot(f.caller)).To(Equal(callerBefore))
		Expect(result.status).To(Equal(0), "%+v", result)
		Expect(result.stdout).To(Equal(native.stdout), "the source entrypoint must enter the already-shipping Cobra command surface")
		Expect(result.stderr).To(BeEmpty())
		output, err := os.ReadFile(filepath.Join(f.root, "factory-go"))
		Expect(err).NotTo(HaveOccurred())
		Expect(output).NotTo(BeEmpty())
		Expect(strings.HasPrefix(string(output), "#!")).To(BeFalse(), "developer output must be the real compiled runtime")
		AddReportEntry("source-entrypoint.developer-output", fmt.Sprintf("%x", sha256.Sum256(output)), ReportEntryVisibilityNever)
		alias := filepath.Join(filepath.Dir(f.root), "logical source alias")
		Expect(os.Mkdir(alias, 0700)).To(Succeed())
		Expect(os.Symlink(filepath.Join(f.root, "scripts"), filepath.Join(alias, "scripts"))).To(Succeed())
		aliasBefore := installationSnapshot(alias)
		// Preserve the literal /../ component: filepath.Join would collapse the security-relevant operand.
		aliased := handoffNativeProcess(f.caller, f.environment, "", alias+"/scripts/../factory", "help")
		AddReportEntry("source-entrypoint.literal-parent-alias", map[string]any{"status": aliased.status, "stdout": aliased.stdout, "stderr": aliased.stderr}, ReportEntryVisibilityNever)
		Expect(aliased.status).To(Equal(0), "%+v", aliased)
		Expect(aliased.stdout).To(Equal(native.stdout))
		Expect(installationSnapshot(alias)).To(Equal(aliasBefore), "a logical alias must not become a build/code owner")
		Expect(sourceEntrypointSnapshot(f.root)).To(Equal(before))
		Expect(installationSnapshot(f.caller)).To(Equal(callerBefore))
		sourceEntrypointAutomaticCopy(f)
		sourceEntrypointCompilerRefusals()
	})

	// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:67
	It("forwards each direct entrypoint through its physical owner with literal arguments input and status", func() {
		results := map[string]cliResult{}
		arguments := map[string][]string{}
		inputs := map[string]string{}
		decoys := map[string]bool{}
		operands := []string{"space and ' quote", "literal $(not-code)\nline", "--", "--java-build-tool=invalid"}
		for _, testcase := range []string{"physical/init", "physical/upgrade", "directory-alias/init", "directory-alias/upgrade"} {
			variant, verb, _ := strings.Cut(testcase, "/")
			root, caller := fixtureDirectories()
			owner := filepath.Join(filepath.Dir(root), "physical owner ' $(not-code)\n.")
			Expect(os.Rename(root, owner)).To(Succeed())
			data, err := os.ReadFile(filepath.Join("..", "scripts", "factory-"+verb+".sh"))
			Expect(err).NotTo(HaveOccurred())
			writeFixture(filepath.Join(owner, "scripts", "factory-"+verb+".sh"), data, 0755)
			writeFixture(filepath.Join(owner, "factory"), []byte(sourceEntrypointRecorder), 0755)
			record, input := filepath.Join(filepath.Dir(root), "argv"), filepath.Join(filepath.Dir(root), "input")
			environment := map[string]string{"PATH": "/usr/bin:/bin", "SOURCE_ENTRY_ARGV": record, "SOURCE_ENTRY_INPUT": input, "SOURCE_ENTRY_LITERAL_ENV": "literal environment $(not-code)\nvalue"}
			program := filepath.Join(owner, "scripts", "factory-"+verb+".sh")
			foreign := filepath.Join(filepath.Dir(root), "foreign logical owner")
			decoy := filepath.Join(filepath.Dir(root), "DECOY_FACTORY_EXECUTED")
			writeFixture(filepath.Join(foreign, "factory"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_ENTRY_DECOY\"\nexit 64\n"), 0755)
			environment["SOURCE_ENTRY_DECOY"] = decoy
			if variant == "directory-alias" {
				Expect(os.Symlink(filepath.Join(owner, "scripts"), filepath.Join(foreign, "scripts"))).To(Succeed())
				program = filepath.Join(foreign, "scripts", "factory-"+verb+".sh")
			}
			foreignBefore := installationSnapshot(foreign)
			before, callerBefore := installationSnapshot(owner), installationSnapshot(caller)
			observed := handoffNativeProcess(caller, environment, "original application stdin\n", program, operands...)
			Expect(installationSnapshot(owner)).To(Equal(before))
			Expect(installationSnapshot(caller)).To(Equal(callerBefore))
			Expect(installationSnapshot(foreign)).To(Equal(foreignBefore))
			_, decoyErr := os.Lstat(decoy)
			decoys[testcase] = decoyErr == nil
			raw, _ := os.ReadFile(record)
			stdin, _ := os.ReadFile(input)
			fields := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
			results[testcase], arguments[testcase], inputs[testcase] = observed, fields, string(stdin)
			AddReportEntry("source-entrypoint.direct-transport", map[string]any{"variant": variant, "decoy_executed": decoys[testcase], "verb": verb, "status": observed.status, "stderr": observed.stderr, "argv": fields, "stdin": string(stdin)}, ReportEntryVisibilityNever)
			expected := append([]string{caller, environment["SOURCE_ENTRY_LITERAL_ENV"], verb}, operands...)
			if observed.status == 63 {
				Expect(fields).To(Equal(expected))
			}
		}
		failures := InterceptGomegaFailures(func() {
			for _, testcase := range []string{"physical/init", "physical/upgrade", "directory-alias/init", "directory-alias/upgrade"} {
				Expect(results[testcase].status).To(Equal(63), "the actual direct adapter must exec its physical owner: %s: %+v", testcase, results[testcase])
				Expect(decoys[testcase]).To(BeFalse(), testcase)
				Expect(inputs[testcase]).To(Equal("original application stdin\n"), testcase)
				Expect(arguments[testcase]).To(HaveLen(3+len(operands)), testcase)
			}
		})
		AddReportEntry("source-entrypoint.direct-violations", failures, ReportEntryVisibilityNever)
		Expect(failures).To(BeEmpty())
	})

	// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:74
	// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:83
	// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:89
	// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:104
	// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:214
	// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:220
	It("keeps branch-selected initializer operands local while preserving legacy controls and fetch-only consent", func() {
		sourceInstallerGitAuth()
		observations := map[string]map[string]any{}
		for _, kind := range []string{"native", "legacy-product-module", "fetch-only", "native-upgrade", "matching-existing-cache", "mismatched-existing-cache", "mismatched-origin", "foreign-git-upgrade", "relative-subdir-upgrade", "newline-caller-upgrade", "refresh-then-init", "native-terminator", "legacy-tag-terminator", "legacy-branch-terminator", "native-fetch-advice"} {
			source, caller := fixtureDirectories()
			base := filepath.Dir(source)
			if kind == "native-fetch-advice" {
				literal := filepath.Join(base, "advice origin ' $(not-code)")
				Expect(os.Rename(source, literal)).To(Succeed())
				source = literal
			}
			upgradeCase := kind == "native-upgrade" || kind == "foreign-git-upgrade" || kind == "relative-subdir-upgrade" || kind == "newline-caller-upgrade" || kind == "refresh-then-init"
			foreignProject := ""
			if kind == "newline-caller-upgrade" {
				foreignProject = caller
				Expect(os.Rename(caller, caller+"\n")).To(Succeed())
				caller += "\n"
			}
			tools, cache := filepath.Join(base, "tools"), filepath.Join(base, "selected template cache")
			if kind == "native-fetch-advice" {
				cache = filepath.Join(base, "advice cache ' $(not-code)")
			}
			record, input, forbidden := filepath.Join(base, "argv"), filepath.Join(base, "input"), filepath.Join(base, "forbidden-tool")
			writeFixture(filepath.Join(source, "factory"), []byte(sourceEntrypointRecorder), 0755)
			data, err := os.ReadFile("../scripts/factory-init.sh")
			Expect(err).NotTo(HaveOccurred())
			if kind == "legacy-product-module" || kind == "legacy-branch-terminator" {
				data = []byte(sourceEntrypointRecorder)
			}
			writeFixture(filepath.Join(source, "scripts/factory-init.sh"), data, 0755)
			upgrade, err := os.ReadFile("../scripts/factory-upgrade.sh")
			Expect(err).NotTo(HaveOccurred())
			lines := strings.SplitN(string(upgrade), "\n", 3)
			Expect(lines).To(HaveLen(3))
			// The native body is current; the fixture declares the reviewed upgrade protocol before installer repair.
			// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:83
			upgrade = []byte(lines[0] + "\n# Factory native upgrade adapter: source-v1\n" + lines[2])
			writeFixture(filepath.Join(source, "scripts/factory-upgrade.sh"), upgrade, 0755)
			writeFixture(filepath.Join(source, "go.mod"), []byte("module example.com/unrelated-product\n\ngo 1.27.3\n"), 0644)
			imageGit(source, "init", "-q")
			imageGit(source, "config", "gc.auto", "0")
			imageGit(source, "config", "gc.autoDetach", "false")
			imageGit(source, "config", "maintenance.auto", "false")
			imageGit(source, "add", "--all")
			imageGit(source, "-c", "user.name=Entrypoint fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "selected branch source")
			branch := "feature/selected-native-source"
			if kind == "native-fetch-advice" {
				branch = "feature/advice'$(not-code)"
			}
			imageGit(source, "branch", branch)
			selectedRevision := strings.TrimSpace(imageGit(source, "rev-parse", branch+"^{commit}"))
			var cacheBefore map[string]string
			if strings.Contains(kind, "existing-cache") || kind == "mismatched-origin" || kind == "foreign-git-upgrade" || kind == "relative-subdir-upgrade" || kind == "refresh-then-init" {
				imageGit(base, "clone", "--quiet", "--branch", branch, source, cache)
				imageGit(cache, "config", "gc.auto", "0")
				imageGit(cache, "config", "maintenance.auto", "false")
				if kind == "mismatched-existing-cache" {
					imageGit(cache, "checkout", "--quiet", "-b", "feature/wrong-cache")
					writeFixture(filepath.Join(cache, "wrong-branch-witness"), []byte("cached code is not the selected ref\n"), 0600)
					imageGit(cache, "add", "wrong-branch-witness")
					imageGit(cache, "-c", "user.name=Cache fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "wrong cached branch")
					Expect(strings.TrimSpace(imageGit(cache, "rev-parse", "HEAD"))).NotTo(Equal(selectedRevision))
					Expect(strings.TrimSpace(imageGit(cache, "rev-parse", branch+"^{commit}"))).To(Equal(selectedRevision), "the requested local ref is present but differs from HEAD")
				}
				if kind == "mismatched-origin" {
					imageGit(cache, "remote", "set-url", "origin", filepath.Join(base, "unselected repository"))
				}
				if kind == "refresh-then-init" {
					imageGit(source, "checkout", "--quiet", branch)
					writeFixture(filepath.Join(source, "refreshed-source-witness"), []byte("new branch revision\n"), 0600)
					imageGit(source, "add", "refreshed-source-witness")
					imageGit(source, "-c", "user.name=Refresh fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "advance selected branch")
					selectedRevision = strings.TrimSpace(imageGit(source, "rev-parse", "HEAD"))
				} else if kind != "foreign-git-upgrade" && kind != "relative-subdir-upgrade" {
					cacheBefore = installationSnapshot(cache)
				}
			}
			if upgradeCase {
				writeFixture(filepath.Join(caller, "factory.yaml"), []byte("project_name: branch-upgrade\n"), 0600)
				imageGit(caller, "init", "-q")
				imageGit(caller, "config", "gc.auto", "0")
				imageGit(caller, "config", "maintenance.auto", "false")
				imageGit(caller, "add", "--all")
				imageGit(caller, "-c", "user.name=Upgrade fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "caller project")
			}
			if kind == "foreign-git-upgrade" {
				foreignProject = filepath.Join(base, "foreign Git selected project")
			}
			if foreignProject != "" {
				writeFixture(filepath.Join(foreignProject, "factory.yaml"), []byte("project_name: foreign-project\n"), 0600)
				imageGit(foreignProject, "init", "-q")
				imageGit(foreignProject, "add", "--all")
				imageGit(foreignProject, "-c", "user.name=Foreign fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "unselected foreign project")
			}
			runCwd := caller
			if kind == "relative-subdir-upgrade" {
				runCwd = filepath.Join(caller, "nested invocation")
				Expect(os.Mkdir(runCwd, 0700)).To(Succeed())
			}
			installer, err := os.ReadFile("../install.sh")
			Expect(err).NotTo(HaveOccurred())
			installerPath := filepath.Join(base, "installer.sh")
			writeFixture(installerPath, installer, 0755)
			git, err := exec.LookPath("git")
			Expect(err).NotTo(HaveOccurred())
			git, err = filepath.EvalSymlinks(git)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.MkdirAll(tools, 0700)).To(Succeed())
			Expect(os.Symlink(git, filepath.Join(tools, "git"))).To(Succeed())
			gitTrace := filepath.Join(base, "installer Git trace")
			if kind == "legacy-tag-terminator" || kind == "legacy-branch-terminator" {
				Expect(os.Remove(filepath.Join(tools, "git"))).To(Succeed())
				writeFixture(filepath.Join(tools, "git"), []byte("#!/bin/sh\nprintf '%s\000' \"$@\" >> \"$SOURCE_ENTRY_GIT_TRACE\"\nexec \"$SOURCE_ENTRY_REAL_GIT\" \"$@\"\n"), 0700)
			}
			for _, name := range []string{"go", "python", "python3", "gh", "curl", "wget", "claude", "codex", "opencode"} {
				writeFixture(filepath.Join(tools, name), []byte("#!/bin/sh\nprintf '%s\\n' \"$0\" >> \"$SOURCE_ENTRY_FORBIDDEN\"\nexit 97\n"), 0700)
			}
			environment := map[string]string{"PATH": tools + string(os.PathListSeparator) + "/usr/bin:/bin", "FACTORY_HOME": cache, "FACTORY_REPO": source, "FACTORY_REF": "wrong-environment-branch", "SOURCE_ENTRY_ARGV": record, "SOURCE_ENTRY_INPUT": input, "SOURCE_ENTRY_LITERAL_ENV": "branch literal", "SOURCE_ENTRY_FORBIDDEN": forbidden, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull}
			if kind == "foreign-git-upgrade" {
				environment["GIT_DIR"] = filepath.Join(foreignProject, ".git")
				environment["GIT_WORK_TREE"] = foreignProject
			}
			if kind == "relative-subdir-upgrade" {
				environment["FACTORY_HOME"] = "../../selected template cache"
			}
			environment["SOURCE_ENTRY_GIT_TRACE"], environment["SOURCE_ENTRY_REAL_GIT"] = gitTrace, git
			var foreignBefore map[string]string
			if foreignProject != "" {
				foreignBefore = installationSnapshot(foreignProject)
			}
			operands := []string{"--pack", "none", "--label", "space ' $(not-code)\nvalue", "--java-build-tool=invalid"}
			args := append([]string{installerPath, "init", "--ref", branch}, operands...)
			if kind == "fetch-only" || kind == "native-fetch-advice" {
				args = []string{installerPath, "--ref=" + branch}
			}
			if upgradeCase {
				operands = []string{"--dry-run", "--json", "--project-input", "literal input with spaces"}
				args = append([]string{installerPath, "upgrade", "--ref", branch}, operands...)
			}
			if strings.HasSuffix(kind, "terminator") {
				ref := branch
				if kind == "legacy-tag-terminator" {
					ref = "v0.1.6"
				}
				operands = []string{"--pack", "none", "--", "--ref", "post-terminator-is-data", "--go-candidate", "v9.8.0-rc.1", "--source", "literal source operand"}
				args = append([]string{installerPath, "init", "--ref", ref}, operands...)
			}
			before, callerBefore := installationSnapshot(source), installationSnapshot(caller)
			result := handoffNativeProcess(runCwd, environment, "typed installer input\n", "/bin/sh", args...)
			Expect(installationSnapshot(source)).To(Equal(before))
			Expect(installationSnapshot(caller)).To(Equal(callerBefore))
			if foreignBefore != nil {
				Expect(installationSnapshot(foreignProject)).To(Equal(foreignBefore), "caller discovery must preserve the unselected project")
			}
			if cacheBefore != nil {
				Expect(installationSnapshot(cache)).To(Equal(cacheBefore), "existing branch cache checks/refusals must be read-only: %s", kind)
			}
			_, err = os.Lstat(forbidden)
			Expect(os.IsNotExist(err)).To(BeTrue(), "branch transport must not authenticate unpublished releases, compile, call models or download")
			raw, recordErr := os.ReadFile(record)
			stdin, _ := os.ReadFile(input)
			fields := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
			expected := append([]string{caller, "branch literal", "init", caller, "--local", "--source", cache}, operands...)
			if kind == "legacy-product-module" {
				expected = append([]string{caller, "branch literal", caller}, operands...)
			}
			if upgradeCase {
				expected = append([]string{runCwd, "branch literal", "upgrade", "--local", "--source", cache}, operands...)
			}
			_, cacheErr := os.Lstat(filepath.Join(cache, ".git"))
			_, traceErr := os.Lstat(gitTrace)
			observations[kind] = map[string]any{"kind": kind, "cache_acquired": cacheErr == nil, "Git_called": traceErr == nil, "status": result.status, "stderr": result.stderr, "argv": fields, "expected_argv": expected, "stdin": string(stdin), "record_absent": os.IsNotExist(recordErr)}
			AddReportEntry("source-entrypoint.branch-transport", observations[kind], ReportEntryVisibilityNever)
			if kind == "native-fetch-advice" {
				commands := []string{}
				for _, line := range strings.Split(result.stdout, "\n") {
					line = strings.TrimSpace(line)
					if strings.Contains(line, "https://softwareaifactory.sh/install.sh") && strings.Contains(line, "init") {
						commands = append(commands, line)
					}
				}
				Expect(commands).To(HaveLen(1), "native advice must provide one runnable installer re-entry")
				curlTrace := filepath.Join(base, "advice curl called")
				writeFixture(filepath.Join(tools, "curl"), []byte("#!/bin/sh\nset -eu\nprintf called > \"$SOURCE_ENTRY_CURL_TRACE\"\nexec /bin/cat \"$SOURCE_ENTRY_INSTALLER\"\n"), 0700)
				replay := maps.Clone(environment)
				replay["FACTORY_HOME"] = filepath.Join(base, "unselected advice cache")
				replay["FACTORY_REPO"] = filepath.Join(base, "unselected advice repository")
				replay["FACTORY_REF"] = "unselected-advice-ref"
				replay["SOURCE_ENTRY_CURL_TRACE"], replay["SOURCE_ENTRY_INSTALLER"] = curlTrace, installerPath
				cacheState := installationSnapshot(cache)
				reran := handoffNativeProcess(caller, replay, "typed installer input\n", "/bin/sh", "-c", commands[0])
				Expect(installationSnapshot(cache)).To(Equal(cacheState), "replayed advice selects the same complete local transport fixture")
				Expect(installationSnapshot(source)).To(Equal(before))
				Expect(installationSnapshot(caller)).To(Equal(callerBefore))
				raw, recordErr := os.ReadFile(record)
				stdin, _ := os.ReadFile(input)
				_, curlErr := os.Lstat(curlTrace)
				fields := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
				expected := []string{caller, "branch literal", "init", caller, "--local", "--source", cache}
				observations["replayed-native-advice"] = map[string]any{"kind": "replayed-native-advice", "status": reran.status, "stderr": reran.stderr, "advice": commands[0], "argv": fields, "expected_argv": expected, "stdin": string(stdin), "record_absent": os.IsNotExist(recordErr), "reentered_installer": curlErr == nil}
				AddReportEntry("source-entrypoint.branch-transport", observations["replayed-native-advice"], ReportEntryVisibilityNever)
			}
			if kind == "refresh-then-init" {
				actualRevision := strings.TrimSpace(imageGit(cache, "rev-parse", "HEAD"))
				Expect(actualRevision).To(Equal(selectedRevision), "the upgrade refresh really selected the new remote commit")
				Expect(os.Remove(record)).To(Succeed())
				Expect(os.Remove(input)).To(Succeed())
				cacheState := installationSnapshot(cache)
				initOperands := []string{"--pack", "none"}
				second := handoffNativeProcess(caller, environment, "typed installer input\n", "/bin/sh", append([]string{installerPath, "init", "--ref", branch}, initOperands...)...)
				Expect(installationSnapshot(cache)).To(Equal(cacheState), "follow-up init cache selection is read-only")
				Expect(installationSnapshot(caller)).To(Equal(callerBefore))
				Expect(installationSnapshot(source)).To(Equal(before))
				raw, recordErr := os.ReadFile(record)
				stdin, _ := os.ReadFile(input)
				fields := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
				expected := append([]string{caller, "branch literal", "init", caller, "--local", "--source", cache}, initOperands...)
				observations["refreshed-cache-init"] = map[string]any{"kind": "refreshed-cache-init", "status": second.status, "stderr": second.stderr, "argv": fields, "expected_argv": expected, "stdin": string(stdin), "record_absent": os.IsNotExist(recordErr), "selected_revision": selectedRevision, "actual_refreshed_head": actualRevision}
				AddReportEntry("source-entrypoint.branch-transport", observations["refreshed-cache-init"], ReportEntryVisibilityNever)
			}
		}
		failures := InterceptGomegaFailures(func() {
			for _, kind := range []string{"native", "legacy-product-module", "native-upgrade", "matching-existing-cache", "foreign-git-upgrade", "relative-subdir-upgrade", "newline-caller-upgrade", "refresh-then-init", "refreshed-cache-init", "native-terminator"} {
				Expect(observations[kind]["status"]).To(Equal(63), kind)
				Expect(observations[kind]["argv"]).To(Equal(observations[kind]["expected_argv"]), kind)
				Expect(observations[kind]["stdin"]).To(Equal("typed installer input\n"), kind)
			}
			for _, kind := range []string{"mismatched-existing-cache", "mismatched-origin"} {
				Expect(observations[kind]["status"]).To(Equal(2), "a mismatched existing cache must refuse before code: %s", kind)
				Expect(observations[kind]["record_absent"]).To(BeTrue(), kind)
			}
			Expect(observations["native-fetch-advice"]["status"]).To(Equal(0))
			Expect(observations["native-fetch-advice"]["record_absent"]).To(BeTrue())
			Expect(observations["replayed-native-advice"]["reentered_installer"]).To(BeTrue())
			Expect(observations["replayed-native-advice"]["status"]).To(Equal(63))
			Expect(observations["replayed-native-advice"]["argv"]).To(Equal(observations["replayed-native-advice"]["expected_argv"]))
			Expect(observations["replayed-native-advice"]["stdin"]).To(BeEmpty(), "piped installer source must never become initializer answers")
			for _, kind := range []string{"legacy-tag-terminator", "legacy-branch-terminator"} {
				Expect(observations[kind]["status"]).To(Equal(2), kind)
				Expect(observations[kind]["record_absent"]).To(BeTrue(), kind)
			}
			Expect(observations["legacy-tag-terminator"]["Git_called"]).To(BeFalse(), "known legacy tags refuse terminators before acquisition")
			Expect(observations["legacy-branch-terminator"]["cache_acquired"]).To(BeTrue(), "branch capability must be discovered before refusing its genuine legacy adapter")
			Expect(observations["fetch-only"]["status"]).To(Equal(0))
			Expect(observations["fetch-only"]["record_absent"]).To(BeTrue(), "bare Git fetch must never execute the selected source")
		})
		AddReportEntry("source-entrypoint.branch-violations", failures, ReportEntryVisibilityNever)
		Expect(failures).To(BeEmpty(), "all branch selections, forwarded operands and cache ownership controls must hold")
	})
	// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:60
	// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:155
	// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:171
	It("initializes through the real local adapter then proves source installed and upgrade boundaries natively", func() {
		f := newSourceEntrypointFixture()
		imageGit(f.root, "add", "--all")
		imageGit(f.root, "-c", "user.name=Source fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-q", "-m", "actual selected source entrypoints")
		revision := strings.TrimSpace(imageGit(f.root, "rev-parse", "HEAD"))
		Expect(imageGit(f.root, "status", "--porcelain", "--untracked-files=all")).To(BeEmpty())
		base := filepath.Dir(f.root)
		tools, forbidden := filepath.Join(base, "native journey tools"), filepath.Join(base, "FORBIDDEN_NETWORK_OR_LEGACY")
		writeFixture(filepath.Join(tools, "npm"), []byte("#!/bin/sh\nexit 0\n"), 0700)
		for _, tool := range []string{"gh", "python", "python3", "curl", "wget", "claude", "codex", "opencode"} {
			writeFixture(filepath.Join(tools, tool), []byte("#!/bin/sh\nprintf '%s\\n' \"$0\" >> \"$SOURCE_ENTRY_FORBIDDEN\"\nexit 97\n"), 0700)
		}
		f.environment["PATH"] = tools + string(os.PathListSeparator) + f.environment["PATH"] + string(os.PathListSeparator) + os.Getenv("PATH")
		f.environment["SOURCE_ENTRY_FORBIDDEN"] = forbidden
		branch := "feature/real-native-installer"
		imageGit(f.root, "branch", branch)
		cache := filepath.Join(base, "fetched native branch")
		f.environment["FACTORY_HOME"] = cache
		f.environment["FACTORY_REPO"] = (&url.URL{Scheme: "file", Path: f.root}).String()
		installer, err := os.ReadFile("../install.sh")
		Expect(err).NotTo(HaveOccurred())
		installerPath := filepath.Join(base, "actual installer.sh")
		writeFixture(installerPath, installer, 0755)
		target := filepath.Join(f.caller, "new project with spaces")
		writeFixture(filepath.Join(target, "README.md"), []byte("application-owned README\n"), 0600)
		sourceBefore := sourceEntrypointSnapshot(f.root)
		targetBefore := installationSnapshot(target)
		initialized := handoffNativeProcess(target, f.environment, nativeInitAnswers("y"), "/bin/sh", installerPath, "init", "--ref", branch, "--pack", "none")
		shallow := strings.TrimSpace(imageGit(cache, "rev-parse", "--is-shallow-repository"))
		AddReportEntry("source-entrypoint.real-native-init", map[string]any{"status": initialized.status, "stdout": initialized.stdout, "stderr": initialized.stderr, "revision": revision, "cache_shallow_after_handoff": shallow, "target_preserved_on_failure": initialized.status == 0 || maps.Equal(targetBefore, installationSnapshot(target)), "source_preserved": maps.Equal(sourceBefore, sourceEntrypointSnapshot(f.root)), "selected_file_uri": f.environment["FACTORY_REPO"]}, ReportEntryVisibilityNever)
		if initialized.status != 0 {
			Expect(installationSnapshot(target)).To(Equal(targetBefore), "failed branch acquisition/build cannot mutate the project")
			Expect(sourceEntrypointSnapshot(f.root)).To(Equal(sourceBefore))
		}
		Expect(initialized.status).To(Equal(0), "%+v", initialized)
		Expect(shallow).To(Equal("false"), "native-selected source needs its complete pinned predecessor history")
		localInitDescriptor(target, revision)
		Expect(os.ReadFile(filepath.Join(target, "README.md"))).To(Equal([]byte("application-owned README\n")))
		installedLauncher, err := os.ReadFile(filepath.Join(target, "factory"))
		Expect(err).NotTo(HaveOccurred())
		sourceLauncher, err := os.ReadFile(filepath.Join(f.root, "factory"))
		Expect(err).NotTo(HaveOccurred())
		Expect(installedLauncher).NotTo(Equal(sourceLauncher), "ordinary users must receive the generated compiler-free launcher")
		Expect(string(installedLauncher)).To(ContainSubstring(".factory/bin/factory-runtime"))
		sourceProof := handoffNativeProcess(f.caller, f.environment, "", filepath.Join(f.root, "factory"), "selftest")
		sourceSelftestExpectHealthy(sourceProof)
		before := installationSnapshot(target)
		preview := handoffNativeProcess(target, f.environment, "", filepath.Join(f.root, "scripts/factory-upgrade.sh"), "--dry-run", "--source", f.root, "--json")
		Expect(preview.status).To(Equal(2), "the existing blocked native assessment is a read-only preview: %+v", preview)
		var plan map[string]any
		Expect(json.Unmarshal([]byte(preview.stdout), &plan)).To(Succeed())
		Expect(plan["mode"]).To(Equal("dry_run"))
		Expect(plan["coverage"]).To(Equal("partial"))
		Expect(installationSnapshot(target)).To(Equal(before), "the direct upgrade adapter cannot grant mutation consent")
		writeFixture(filepath.Join(tools, "go"), []byte("#!/bin/sh\nprintf go >> \"$SOURCE_ENTRY_FORBIDDEN\"\nexit 97\n"), 0700)
		ordinary := handoffNativeProcess(target, f.environment, "", filepath.Join(target, "factory"), "selftest")
		sourceSelftestExpectHealthy(ordinary)
		Expect(sourceEntrypointSnapshot(f.root)).To(Equal(sourceBefore), "only ignored developer output may persist in source")
		_, err = os.Lstat(forbidden)
		Expect(os.IsNotExist(err)).To(BeTrue(), "local entrypoints must not authenticate an unpublished release or execute legacy/Python/provider tools")
	})
})
