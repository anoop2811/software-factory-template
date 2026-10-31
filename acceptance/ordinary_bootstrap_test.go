package acceptance_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func ordinaryBootstrapStableProvider(f handoffBootstrapFixture) {
	GinkgoHelper()
	path := filepath.Join(f.root, "host tools/gh")
	data, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	original := `release-view) printf '%s\tfalse\n' "$3" ;;`
	replacement := `release-view)
  [ "$HANDOFF_SCENARIO" != release-absent ] || exit 44
  tag="$3"; draft=false; prerelease=false
  case "$HANDOFF_SCENARIO" in wrong-tag) tag=v0.3.0 ;; draft) draft=true ;; marked-prerelease) prerelease=true ;; esac
  case "$*" in *isPrerelease*) printf '%s\t%s\t%s\n' "$tag" "$draft" "$prerelease" ;; *) printf '%s\t%s\n' "$tag" "$draft" ;; esac ;;
`
	changed := strings.Replace(string(data), original, replacement, 1)
	Expect(changed).NotTo(Equal(string(data)))
	writeFixture(path, []byte(changed), 0700)
}
func ordinaryBootstrapSentinel(tag, scenario string) handoffBootstrapFixture {
	GinkgoHelper()
	f := handoffBootstrap(map[string]string{"linux": "Linux", "darwin": "Darwin"}[runtime.GOOS], map[string]string{"amd64": "x86_64", "arm64": "arm64"}[runtime.GOARCH], runtime.GOOS+"-"+runtime.GOARCH, scenario)
	for _, part := range []struct{ prefix, suffix string }{{"factory-bootstrap-", ""}, {"factory-installation-", ".tar.gz"}} {
		Expect(os.Rename(filepath.Join(f.environment["HANDOFF_ASSETS"], part.prefix+handoffVersion+"-"+f.target+part.suffix), filepath.Join(f.environment["HANDOFF_ASSETS"], part.prefix+tag+"-"+f.target+part.suffix))).To(Succeed())
	}
	f.environment["HANDOFF_VERSION"] = tag
	f.environment["FACTORY_REF"] = ""
	f.environment["FACTORY_REPO"] = "https://github.com/anoop2811/software-factory-template"
	ordinaryBootstrapStableProvider(f)
	return f
}
func ordinaryBootstrapOperands(args []string) (string, []string) {
	GinkgoHelper()
	remaining := []string{}
	tag := ""
	count := 0
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			remaining = append(remaining, args[i:]...)
			break
		}
		if args[i] == "--ref" {
			Expect(i + 1).To(BeNumerically("<", len(args)))
			i++
			tag = args[i]
			count++
			continue
		}
		if strings.HasPrefix(args[i], "--ref=") {
			tag = strings.TrimPrefix(args[i], "--ref=")
			count++
			continue
		}
		remaining = append(remaining, args[i])
	}
	Expect(count).To(Equal(1), "stable policy must reach the native child exactly once")
	return tag, remaining
}
func ordinaryBootstrapNoTrace(f handoffBootstrapFixture) {
	GinkgoHelper()
	for _, name := range []string{"trace", "candidate-executed", "forbidden-tool"} {
		_, err := os.Lstat(filepath.Join(f.records, name))
		Expect(os.IsNotExist(err)).To(BeTrue(), "no acquisition/execution for invalid input: %s", name)
	}
}
func ordinaryBootstrapCache(f handoffBootstrapFixture) {
	GinkgoHelper()
	for _, pattern := range []string{"factory-release.*", "factory-release-*"} {
		entries, err := filepath.Glob(filepath.Join(f.root, pattern))
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty(), "confirmed work must dispose only its private acquisition")
	}
}
func ordinaryBootstrapLegacyGit(f handoffBootstrapFixture) {
	GinkgoHelper()
	source := `#!/bin/sh
set -eu
printf '%s\000' "$@" >> "$HANDOFF_RECORD_DIR/legacy-git-arguments"
case "$1" in
 ls-remote) exit 0 ;;
 clone)
  for destination do :; done
  /bin/mkdir -p "$destination/.git" "$destination/scripts"
  /bin/cat > "$destination/scripts/factory-init.sh" <<'LEGACY'
#!/bin/sh
printf '%s\000' "$@" > "$HANDOFF_RECORD_DIR/legacy-init-arguments"
exit 0
LEGACY
  /bin/chmod 700 "$destination/scripts/factory-init.sh" ;;
 *) exit 96 ;;
esac
`
	writeFixture(filepath.Join(f.root, "host tools/git"), []byte(source), 0700)
}

var _ = Describe("Authenticated ordinary Go installer bootstrap", Ordered, ContinueOnFailure, func() {
	var fixtures *defaultUpgradeJourneyFixtures
	BeforeAll(func() { fixtures = prepareDefaultUpgradeJourneys(true) })
	// per docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:136
	It("runs the authenticated selected initializer without executing a cached source initializer", func() {
		f, driver := fixtures.surface(defaultInitTag)
		legacy := filepath.Join(f.environment["FACTORY_HOME"], "scripts/factory-init.sh")
		writeFixture(legacy, []byte("#!/bin/sh\nprintf executed > \"$HANDOFF_RECORD_DIR/legacy-source-executed\"\nexit 97\n"), 0700)
		foreign := installationSnapshot(f.environment["FACTORY_HOME"])
		control := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", filepath.Join(f.cwd, "direct authenticated control"), "--ref", defaultInitTag, "--pack", "none")
		Expect(control.status).To(Equal(0), "real complete native fixture must work independently: %+v", control)
		Expect(control.stdout).To(ContainSubstring(defaultInitMarker))
		target := filepath.Join(f.cwd, "ordinary bootstrap target with spaces")
		application := []byte("existing caller content remains\n")
		writeFixture(filepath.Join(f.cwd, "application.txt"), application, 0600)
		before := installationSnapshot(f.cwd)
		result := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), "/bin/sh", f.installer, "init", target, "--ref", defaultInitTag, "--pack", "none")
		_, legacyErr := os.Lstat(filepath.Join(f.records, "legacy-source-executed"))
		AddReportEntry("ordinary-bootstrap.real-init", map[string]any{"status": result.status, "stdout": result.stdout, "stderr": result.stderr, "legacy_executed": legacyErr == nil, "direct_control": control.status}, ReportEntryVisibilityNever)
		Expect(result.status).To(Equal(0), "ordinary stable installer must choose authenticated native code: %+v", result)
		Expect(result.stdout).To(ContainSubstring(defaultInitMarker))
		Expect(os.IsNotExist(legacyErr)).To(BeTrue())
		defaultInitDescriptor(target, defaultInitTag, fixtures.revision, fixtures.image(defaultInitTag).binary)
		Expect(os.ReadFile(filepath.Join(f.cwd, "application.txt"))).To(Equal(application))
		after := installationSnapshot(f.cwd)
		for path, value := range before {
			Expect(after[path]).To(Equal(value), path)
		}
		Expect(installationSnapshot(f.environment["FACTORY_HOME"])).To(Equal(foreign))
		Expect(handoffNativeProcess(target, f.environment, "", filepath.Join(target, "factory"), "selftest").status).To(Equal(0))
		defaultInitNoForbidden(f)
		ordinaryBootstrapCache(f)
	})
	// per docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:121
	It("previews and applies a genuine legacy project with fresh complete consent through the stable installer", func() {
		p := fixtures.legacyProject()
		f := p.f
		writeFixture(filepath.Join(p.root, "docs/bootstrap-user-note"), []byte("operator history\n"), 0600)
		before := installationSnapshot(p.root)
		args := defaultUpgradeInputArgs(p.inputs, "--ref", defaultUpgradeTag)
		run := func(extra ...string) cliResult {
			return handoffNativeProcess(p.root, f.environment, "", "/bin/sh", append([]string{f.installer}, append(append([]string{}, args...), extra...)...)...)
		}
		preview := run("--dry-run", "--json")
		proposal := handoffProposal(preview)
		Expect(proposal["profile"]).To(Equal("v0.1.6"))
		Expect(proposal["checks"]).To(ContainElement(defaultUpgradePlanMarker))
		Expect(installationSnapshot(p.root)).To(Equal(before))
		refused := run("--confirm", proposal["proposal_digest"].(string))
		Expect(refused.status).To(Equal(2))
		Expect(installationSnapshot(p.root)).To(Equal(before))
		applied := run("--confirm", proposal["proposal_digest"].(string), "--quiescent")
		Expect(applied.status).To(Equal(0), "native stable bootstrap confirmation: %+v", applied)
		Expect(applied.stderr).To(ContainSubstring(defaultUpgradeApplyMarker))
		defaultInitDescriptor(p.root, defaultUpgradeTag, fixtures.revision, fixtures.image(defaultUpgradeTag).binary)
		ids := retentionBackupIDs(p.root)
		Expect(ids).To(HaveLen(1))
		// The original bytes are independently available in the genuine setup.
		original, err := os.ReadFile(filepath.Join(fixtures.legacy.root, "factory"))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.ReadFile(filepath.Join(p.root, ".factory/backups", ids[0], "assets/factory"))).To(Equal(original))
		Expect(os.ReadFile(filepath.Join(p.root, "docs/bootstrap-user-note"))).To(Equal([]byte("operator history\n")))
		Expect(handoffNativeProcess(p.root, f.environment, "", filepath.Join(p.root, "factory"), "selftest").status).To(Equal(0))
		defaultInitNoForbidden(f)
		ordinaryBootstrapCache(f)
	})
	It("reconciles native acquisition after authenticated handoff on parent-only cancellation", func() { ordinaryBootstrapCancel(fixtures, true) })
})

var _ = Describe("Authenticated ordinary Go installer bootstrap", Ordered, ContinueOnFailure, func() {
	// per docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:74
	It("keeps bare fetch inert and the published legacy default unchanged", func() {
		for _, ref := range []string{"", "main", defaultInitTag} {
			f := ordinaryBootstrapSentinel(defaultInitTag, "")
			ordinaryBootstrapLegacyGit(f)
			args := []string{f.installer}
			if ref != "" {
				args = append(args, "--ref", ref)
			}
			before := installationSnapshot(f.cwd)
			fetched := imageProcess(f.cwd, f.environment, "/bin/sh", args...)
			Expect(fetched.status).To(Equal(0), "%q: %+v", ref, fetched)
			Expect(installationSnapshot(f.cwd)).To(Equal(before))
			for _, name := range []string{"legacy-init-arguments", "candidate-executed", "trace"} {
				_, err := os.Lstat(filepath.Join(f.records, name))
				Expect(os.IsNotExist(err)).To(BeTrue(), name)
			}
			git := handoffRecorded(f, "legacy-git-arguments")
			want := ref
			if want == "" {
				want = "v0.1.6"
			}
			Expect(git).To(ContainElement(want))
			if ref == defaultInitTag {
				Expect(fetched.stdout).To(ContainSubstring("init --ref " + defaultInitTag))
				Expect(fetched.stdout).NotTo(ContainSubstring("scripts/factory-init.sh"))
			}
		}
		f := ordinaryBootstrapSentinel(defaultInitTag, "")
		ordinaryBootstrapLegacyGit(f)
		legacy := imageProcess(f.cwd, f.environment, "/bin/sh", f.installer, "init", "--pack", "none")
		Expect(legacy.status).To(Equal(0), "%+v", legacy)
		Expect(handoffRecorded(f, "legacy-git-arguments")).To(ContainElement("v0.1.6"))
		Expect(handoffRecorded(f, "legacy-init-arguments")).To(ContainElement(f.cwd))
		_, err := os.Lstat(filepath.Join(f.records, "trace"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:35
	It("reserves strict Go selectors and conflicts before tools without changing legacy precedence", func() {
		type observation struct {
			Request                           []string `json:"request"`
			Status                            int      `json:"status"`
			Stderr                            string   `json:"stderr"`
			Touched                           []string `json:"touched"`
			ProjectUnchanged, SourceUnchanged bool
		}
		observed := []observation{}
		probe := func(args []string, repo string) {
			f := ordinaryBootstrapSentinel(defaultInitTag, "")
			ordinaryBootstrapLegacyGit(f)
			writeFixture(filepath.Join(f.environment["FACTORY_HOME"], "scripts/factory-init.sh"), []byte("#!/bin/sh\nprintf executed > \"$HANDOFF_RECORD_DIR/legacy-source-executed\"\nexit 97\n"), 0700)
			if repo != "" {
				f.environment["FACTORY_REPO"] = repo
			}
			before, source := installationSnapshot(f.cwd), installationSnapshot(f.environment["FACTORY_HOME"])
			result := imageProcess(f.cwd, f.environment, "/bin/sh", append([]string{f.installer}, args...)...)
			touched := []string{}
			for _, name := range []string{"trace", "candidate-executed", "forbidden-tool", "legacy-git-arguments", "legacy-init-arguments", "legacy-source-executed"} {
				_, err := os.Lstat(filepath.Join(f.records, name))
				if err == nil {
					touched = append(touched, name)
				} else {
					Expect(os.IsNotExist(err)).To(BeTrue(), name)
				}
			}
			observed = append(observed, observation{args, result.status, result.stderr, touched, reflect.DeepEqual(before, installationSnapshot(f.cwd)), reflect.DeepEqual(source, installationSnapshot(f.environment["FACTORY_HOME"]))})
		}
		cases := [][]string{{"init", "--ref"}, {"init", "--ref="}, {"init", "--ref", "v0.2"}, {"init", "--ref", "v0.02.0"}, {"init", "--ref", "v1.0.0-rc.1"}, {"init", "--ref", "v0.2.0+bad..build"}, {"init", "--ref", defaultInitTag, "--ref", "main"}, {"init", "--ref", "main", "--ref", defaultInitTag}, {"init", "--ref", defaultInitTag, "--go-candidate", handoffVersion}, {"init", "--ref", defaultInitTag, "--local"}, {"upgrade", "--ref", defaultInitTag, "--source", "."}, {"upgrade", "--ref", defaultInitTag, "--installation"}}
		for _, args := range cases {
			probe(args, "")
			probe(append(append([]string{}, args...), "--help"), "")
		}
		for _, repo := range []string{"https://example.invalid/fork", "file:///untrusted/source"} {
			probe([]string{"init", "--ref", defaultInitTag, "--help"}, repo)
		}
		// A downstream legacy parser cannot reinterpret a terminal ref or a
		// malformed literal verb into permission to execute cached source.
		// per docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:164
		for _, prefix := range [][]string{{"init"}, {"init", "--ref", "v0.1.6"}, {"init "}, {"", "init"}} {
			probe(append(append([]string{}, prefix...), "--", "--ref", defaultInitTag), "")
		}
		for _, ref := range []string{defaultInitTag, "v0.2"} {
			for _, verb := range [][]string{{"init "}, {" init"}, {"", "init"}, {"init\n"}} {
				probe(append(append([]string{}, verb...), "--ref", ref), "")
			}
		}
		AddReportEntry("ordinary-bootstrap.selector-boundaries", observed, ReportEntryVisibilityNever)
		for _, row := range observed {
			Expect(row.Status).To(Equal(2), "%+v", row)
			Expect(row.Touched).To(BeEmpty(), "%+v", row)
			Expect(row.ProjectUnchanged).To(BeTrue(), "%+v", row)
			Expect(row.SourceUnchanged).To(BeTrue(), "%+v", row)
		}
		f := ordinaryBootstrapSentinel(defaultInitTag, "")
		f.environment["FACTORY_REF"] = "main"
		help := imageProcess(f.cwd, f.environment, "/bin/sh", f.installer, "init", "--ref", defaultInitTag, "--help")
		Expect(help.status).To(Equal(0))
		ordinaryBootstrapNoTrace(f)
	})
	// per docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:102
	It("preserves exact stable selection literal argv terminators and native child exit status", func() {
		for _, ambient := range []bool{false, true} {
			tag := defaultInitTag + "+build.1"
			f := ordinaryBootstrapSentinel(tag, "")
			raw := strings.Replace(string(f.raw), "exit 0\n", "exit 37\n", 1)
			writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+tag+"-"+f.target), []byte(raw), 0600)
			args := []string{"init", "folder with spaces", "--pack", "none", "--", "--go-candidate", "quote ' and \" and $(not-code)", "line one\nline two"}
			if ambient {
				f.environment["FACTORY_REF"] = tag
			} else {
				f.environment["FACTORY_REF"] = "main"
				args = append(args[:1], append([]string{"--ref=" + tag}, args[1:]...)...)
			}
			result := imageProcess(f.cwd, f.environment, "/bin/sh", append([]string{f.installer}, args...)...)
			Expect(result.status).To(Equal(37), "%+v", result)
			selected, remaining := ordinaryBootstrapOperands(handoffRecorded(f, "bootstrap-argv"))
			Expect(selected).To(Equal(tag))
			Expect(remaining).To(Equal([]string{"init", "folder with spaces", "--pack", "none", "--", "--go-candidate", "quote ' and \" and $(not-code)", "line one\nline two"}))
			Expect(handoffRecorded(f, "release-view")).To(ContainElement("tagName,isDraft,isPrerelease"))
			ordinaryBootstrapCache(f)
		}
	})
	// per docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:88
	It("refuses unpublished unstable mismatched or unauthenticated assets before downloaded execution", func() {
		for _, scenario := range []string{"release-absent", "wrong-tag", "draft", "marked-prerelease", "download-fail", "missing-image", "roots-fail", "authentication-fail", "wrong-subject", "malformed-verification"} {
			f := ordinaryBootstrapSentinel(defaultInitTag, scenario)
			// per docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:187
			writeFixture(filepath.Join(f.environment["FACTORY_HOME"], "scripts/factory-init.sh"), []byte("#!/bin/sh\nprintf executed > \"$HANDOFF_RECORD_DIR/legacy-source-executed\"\nexit 97\n"), 0700)
			cached := installationSnapshot(f.environment["FACTORY_HOME"])
			before := installationSnapshot(f.cwd)
			result := imageProcess(f.cwd, f.environment, "/bin/sh", f.installer, "init", "--ref", defaultInitTag)
			Expect(result.status).NotTo(Equal(0), "%s: %+v", scenario, result)
			_, err := os.Lstat(f.marker)
			Expect(os.IsNotExist(err)).To(BeTrue(), scenario)
			Expect(installationSnapshot(f.cwd)).To(Equal(before))
			trace, err := os.ReadFile(filepath.Join(f.records, "trace"))
			Expect(err).NotTo(HaveOccurred(), "must reach intended authenticated boundary %s", scenario)
			if scenario == "release-absent" || scenario == "wrong-tag" || scenario == "draft" || scenario == "marked-prerelease" {
				Expect(string(trace)).To(ContainSubstring("release-view\n"))
				Expect(string(trace)).NotTo(ContainSubstring("release-download\n"))
			}
			_, err = os.Lstat(filepath.Join(f.records, "early-chmod"))
			Expect(os.IsNotExist(err)).To(BeTrue())
			ordinaryBootstrapCache(f)
			_, sourceErr := os.Lstat(filepath.Join(f.records, "legacy-source-executed"))
			_, toolErr := os.Lstat(filepath.Join(f.records, "forbidden-tool"))
			cachedAfter := installationSnapshot(f.environment["FACTORY_HOME"])
			AddReportEntry("ordinary-bootstrap.failure-refusal", map[string]any{"scenario": scenario, "status": result.status, "old_guards_completed": true, "cached_source_executed": sourceErr == nil, "forbidden_tool_executed": toolErr == nil, "cached_source_unchanged": reflect.DeepEqual(cachedAfter, cached)}, ReportEntryVisibilityNever)
			Expect(os.IsNotExist(sourceErr)).To(BeTrue(), "no cached-source fallback after %s", scenario)
			Expect(os.IsNotExist(toolErr)).To(BeTrue(), "no forbidden-tool fallback after %s", scenario)
			Expect(cachedAfter).To(Equal(cached), "preserve cached source after %s", scenario)
		}
	})
	// per docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:108
	It("separates piped installer source from answers and preserves file-mode typed stdin", func() {
		for _, piped := range []bool{false, true} {
			f := ordinaryBootstrapSentinel(defaultInitTag, "")
			record := filepath.Join(f.records, "actual-input")
			raw := "#!/bin/sh\n/bin/cat > \"$HANDOFF_RECORD_DIR/actual-input\"\nexit 0\n"
			writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+defaultInitTag+"-"+f.target), []byte(raw), 0600)
			program := "/bin/sh"
			args := []string{f.installer, "init", "--ref", defaultInitTag}
			input := "literal answer data\n"
			if piped {
				body, err := os.ReadFile(f.installer)
				Expect(err).NotTo(HaveOccurred())
				input = string(body)
				args = []string{"-s", "--", "init", "--ref", defaultInitTag}
			}
			result := handoffNativeProcess(f.cwd, f.environment, input, program, args...)
			Expect(result.status).To(Equal(0), "%+v", result)
			actual, err := os.ReadFile(record)
			Expect(err).NotTo(HaveOccurred())
			if piped {
				Expect(actual).To(BeEmpty(), "source text is not an initializer answer")
			} else {
				Expect(string(actual)).To(Equal(input))
			}
			ordinaryBootstrapCache(f)
		}
	})
	// per docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:112
	It("reconciles the live verifier group on parent-only cancellation", func() { ordinaryBootstrapCancel(nil, false) })
})

func ordinaryBootstrapCancel(fixtures *defaultUpgradeJourneyFixtures, native bool) {
	GinkgoHelper()
	var f handoffBootstrapFixture
	if native {
		f, _ = fixtures.surface(defaultInitTag)
	} else {
		f = ordinaryBootstrapSentinel(defaultInitTag, "")
		writeFixture(filepath.Join(f.environment["FACTORY_HOME"], "operator-witness"), []byte("foreign source cache; keep exactly\n"), 0600)
	}
	ready := filepath.Join(f.records, "held-bootstrap-git")
	git, err := exec.LookPath("git")
	Expect(err).NotTo(HaveOccurred())
	git, err = filepath.EvalSymlinks(git)
	Expect(err).NotTo(HaveOccurred())
	f.environment["SOURCE_SELFTEST_READY"], f.environment["SOURCE_SELFTEST_REAL_GIT"] = ready, git
	held := filepath.Join(f.root, "host tools/held-gh")
	writeFixture(held, []byte(sourceSelftestHeldGit), 0700)
	gh := filepath.Join(f.root, "host tools/gh")
	original := filepath.Join(f.root, "host tools/original-gh")
	Expect(os.Rename(gh, original)).To(Succeed())
	wrapper := "#!/bin/sh\nexec \"$BOOTSTRAP_HELD_GH\" \"$@\"\n"
	if native {
		wrapper = "#!/bin/sh\nif [ -z \"${FACTORY_RELEASE_DIR-}\" ]; then exec \"$BOOTSTRAP_ORIGINAL_GH\" \"$@\"; fi\nexec \"$BOOTSTRAP_HELD_GH\" \"$@\"\n"
	}
	writeFixture(gh, []byte(wrapper), 0700)
	f.environment["BOOTSTRAP_HELD_GH"], f.environment["BOOTSTRAP_ORIGINAL_GH"] = held, original
	outsider := exec.Command("/bin/sleep", "30")
	outsider.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	Expect(outsider.Start()).To(Succeed())
	DeferCleanup(func() { _ = outsider.Process.Kill(); _ = outsider.Wait() })
	before, foreign := installationSnapshot(f.cwd), installationSnapshot(f.environment["FACTORY_HOME"])
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := handoffCommand(ctx, f.cwd, f.environment, "", "/bin/sh", f.installer, "init", "--ref", defaultInitTag, "--pack", "none")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	Expect(command.Start()).To(Succeed())
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	completed := false
	var receipt sourceSelftestReceipt
	DeferCleanup(func() {
		sourceSelftestCleanupGroup(receipt)
		if !completed {
			_ = command.Process.Signal(syscall.SIGTERM)
			select {
			case <-finished:
				completed = true
			case <-time.After(8 * time.Second):
				group, e := syscall.Getpgid(command.Process.Pid)
				if e == nil && group == command.Process.Pid {
					_ = syscall.Kill(-group, syscall.SIGKILL)
				}
				select {
				case <-finished:
					completed = true
				case <-time.After(time.Second):
				}
			}
		}
		confirmed := completed && receipt.Group > 1
		for _, group := range []int{receipt.Group, command.Process.Pid} {
			if group <= 1 || syscall.Kill(-group, 0) != syscall.ESRCH {
				confirmed = false
			}
		}
		if !confirmed {
			holdFixtureCleanup(f.root, "bootstrap caller or owned group custody is unconfirmed")
		}
		Expect(confirmed).To(BeTrue(), "all owned groups must be absent after Wait before fixture disposal")
	})
	receipt = sourceSelftestReadiness(ready)
	Expect(command.Process.Signal(syscall.SIGTERM)).To(Succeed())
	var returned error
	select {
	case returned = <-finished:
		completed = true
	case <-time.After(8 * time.Second):
		Fail("bootstrap cancellation exceeded existing cleanup allowance")
	}
	Expect(returned).To(HaveOccurred())
	Expect(command.ProcessState.ExitCode()).To(Equal(143), stderr.String())
	Eventually(func() error { return syscall.Kill(-receipt.Group, 0) }, time.Second).Should(MatchError(syscall.ESRCH))
	Eventually(func() error { return syscall.Kill(-command.Process.Pid, 0) }, time.Second).Should(MatchError(syscall.ESRCH), "shell child and watchdogs must also settle")
	Expect(installationSnapshot(f.cwd)).To(Equal(before))
	Expect(installationSnapshot(f.environment["FACTORY_HOME"])).To(Equal(foreign))
	Expect(syscall.Kill(outsider.Process.Pid, 0)).To(Succeed())
	Expect(stdout.String()).NotTo(ContainSubstring("Installation complete"))
	ordinaryBootstrapCache(f)
	AddReportEntry("ordinary-bootstrap.cancellation", map[string]any{"native_handoff": native, "caller": command.Process.Pid, "status": command.ProcessState.ExitCode(), "receipt": receipt, "group_absent": true, "outsider_alive": true}, ReportEntryVisibilityNever)
}
