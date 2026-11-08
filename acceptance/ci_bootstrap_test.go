package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// per docs/adr/0144-ci-installer-bootstrap.md:28
// per docs/adr/0144-ci-installer-bootstrap.md:112
var _ = Describe("Whole installation upgrade release candidate handoff CI checkout preparation", SpecPriority(0), Label("handoff-ci-bootstrap-surface"), func() {
	// per docs/adr/0144-ci-installer-bootstrap.md:33
	// per docs/adr/0144-ci-installer-bootstrap.md:43
	It("owns installer CI help and strict native-compatible selectors before acquisition", func() {
		f := ordinaryBootstrapSentinel(handoffVersion, "")
		ciBootstrapPoison(f)
		f.environment["PATH"] = filepath.Join(f.root, "no-tools")
		Expect(os.Mkdir(f.environment["PATH"], 0700)).To(Succeed())
		driver := filepath.Join(f.root, "native-parser")
		writeFixture(driver, candidateBinary, 0700)
		nativeEnv := map[string]string{}
		for key, value := range f.environment {
			nativeEnv[key] = value
		}
		nativeEnv["FACTORY_BRIDGE_PROTOCOL"] = ""
		before := nativeInitArtifacts(f.cwd)
		type request struct {
			args   []string
			status int
		}
		requests := []request{{[]string{"ci", "--help"}, 0}, {[]string{"ci", "-h"}, 0}, {[]string{"ci", "prepare", "--help"}, 0}, {[]string{"ci"}, 2}, {[]string{"ci", "unknown"}, 2}, {[]string{"ci", "--help", "--help"}, 2}}
		for _, tag := range []string{"v0.0.0", "v1.2.3", "v1.2.3-rc.1+build.001", "v1.2.3+001", "v" + strings.Repeat("9", 80) + ".0.0", "v1.2.3+" + strings.Repeat("a", 121)} {
			requests = append(requests, request{[]string{"ci", "prepare", "--go-candidate=" + tag, "--help"}, 0})
		}
		for _, tag := range []string{"", "main", "latest", "V1.2.3", "v1.2", "v01.2.3", "v1.02.3", "v1.2.03", "v1.2.3-01", "v1.2.3-rc..1", "v1.2.3+", "v1.2.3+é", "v1.2.3+" + strings.Repeat("a", 122)} {
			requests = append(requests, request{[]string{"ci", "prepare", "--go-candidate=" + tag, "--help"}, 2})
		}
		for _, target := range []string{"darwin/amd64", "darwin/arm64", "linux/amd64", "linux/arm64"} {
			requests = append(requests, request{[]string{"ci", "prepare", "--origin-target", target, "-h"}, 0})
		}
		valid := []string{"ci", "prepare", "--go-candidate", handoffVersion, "--origin-target", runtime.GOOS + "/" + runtime.GOARCH, "--ephemeral"}
		for _, extra := range [][]string{{"--help", "-h"}, {"--help=x"}, {"--ephemeral=true"}, {"--ephemeral"}, {"--go-candidate", handoffVersion}, {"--origin-target=linux/amd64"}, {"--"}, {"unexpected operand"}, {"--source", "/unused"}, {"--ref", "v0.1.6"}, {"--local"}, {"--installation"}, {"--unknown"}} {
			requests = append(requests, request{append(append([]string{}, valid...), extra...), 2})
		}
		for _, args := range [][]string{{"ci", "prepare"}, {"ci", "prepare", "--go-candidate"}, {"ci", "prepare", "--origin-target"}, {"ci", "prepare", "--origin-target=", "--help"}, {"ci", "prepare", "--origin-target=windows/amd64", "--help"}, {"ci", "prepare", "--ephemeral=true", "--help"}, {"ci", "prepare", "--go-candidate", handoffVersion, "--ephemeral"}, {"ci", "prepare", "--origin-target=linux/amd64", "--ephemeral"}, valid[:len(valid)-1]} {
			requests = append(requests, request{args, 2})
		}
		for _, testcase := range requests {
			native := imageProcess(f.cwd, nativeEnv, driver, testcase.args...)
			Expect(native.status).To(Equal(testcase.status), "independent native parser: %q: %+v", testcase.args, native)
			for _, shell := range bootstrapWatchdogInterpreters() {
				out := imageProcess(f.cwd, f.environment, shell, append([]string{f.installer}, testcase.args...)...)
				Expect(out.status).To(Equal(testcase.status), "%s args %q: %+v", shell, testcase.args, out)
				if testcase.status == 0 {
					Expect(out.stdout).To(ContainSubstring("ci prepare"))
					Expect(out.stdout).To(ContainSubstring("--ephemeral"))
					Expect(out.stderr).To(BeEmpty())
				} else {
					Expect(strings.ToLower(out.stderr)).To(ContainSubstring("ci"))
				}
				ordinaryBootstrapNoTrace(f)
				Expect(nativeInitArtifacts(f.cwd)).To(Equal(before))
				ciBootstrapCacheUnchanged(f)
			}
		}
	})

	// per docs/adr/0144-ci-installer-bootstrap.md:52
	// per docs/adr/0144-ci-installer-bootstrap.md:61
	// per docs/adr/0144-ci-installer-bootstrap.md:77
	It("forwards authenticated installer CI argv streams environment and selected-owner status literally", func() {
		for _, shell := range bootstrapWatchdogInterpreters() {
			for _, equals := range []bool{false, true} {
				f := ordinaryBootstrapSentinel(handoffVersion, "")
				ciBootstrapPoison(f)
				f.environment["CI_BOOTSTRAP_LITERAL"] = "literal ' \" $(not-code)\nsecond line"
				f.environment["LC_ALL"] = "POSIX"
				raw := strings.Replace(string(f.raw), "exit 0\n", `pwd -P > "$HANDOFF_RECORD_DIR/native-cwd"
/bin/cat > "$HANDOFF_RECORD_DIR/native-stdin"
printf '%s' "$CI_BOOTSTRAP_LITERAL" > "$HANDOFF_RECORD_DIR/native-environment"
printf '%s' "${FACTORY_BRIDGE_PROTOCOL-}" > "$HANDOFF_RECORD_DIR/native-bridge"
printf '%s' "$LC_ALL" > "$HANDOFF_RECORD_DIR/native-locale"
printf 'selected owner output\n'
printf 'selected owner does not support CI\n' >&2
exit 37
`, 1)
				writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+handoffVersion+"-"+f.target), []byte(raw), 0600)
				args := ciPrepareArguments("linux/amd64")
				if equals {
					args = []string{"ci", "prepare", "--ephemeral", "--origin-target=linux/amd64", "--go-candidate=" + handoffVersion}
				}
				input := "stdin stays literal\nwith spaces and \"quotes\"\n"
				out := handoffNativeProcess(f.cwd, f.environment, input, shell, append([]string{f.installer}, args...)...)
				Expect(out.status).To(Equal(37), "%+v", out)
				Expect(out.stdout).To(Equal("selected owner output\n"))
				Expect(out.stderr).To(Equal("selected owner does not support CI\n"))
				Expect(handoffRecorded(f, "bootstrap-argv")).To(Equal(args))
				cwd, err := filepath.EvalSymlinks(f.cwd)
				Expect(err).NotTo(HaveOccurred())
				for name, expected := range map[string]string{"native-cwd": cwd + "\n", "native-stdin": input, "native-environment": f.environment["CI_BOOTSTRAP_LITERAL"], "native-bridge": "", "native-locale": "POSIX"} {
					Expect(os.ReadFile(filepath.Join(f.records, name))).To(Equal([]byte(expected)), name)
				}
				verify := handoffRecorded(f, "verify")
				Expect(handoffFlag(verify, "--repo")).To(Equal("anoop2811/software-factory-template"))
				Expect(handoffFlag(verify, "--source-ref")).To(Equal("refs/tags/" + handoffVersion))
				Expect(verify[2]).To(HaveSuffix("factory-bootstrap-" + handoffVersion + "-" + f.target))
				ciBootstrapCacheUnchanged(f)
				ordinaryBootstrapCache(f)
			}
		}
	})

	// per docs/adr/0144-ci-installer-bootstrap.md:59
	// per docs/adr/0144-ci-installer-bootstrap.md:74
	It("refuses installer CI acquisition and authentication failures before selected-owner execution", func() {
		for _, scenario := range []string{"download-fail", "roots-fail", "authentication-fail", "wrong-subject"} {
			f := ordinaryBootstrapSentinel(handoffVersion, scenario)
			ciBootstrapPoison(f)
			before := nativeInitArtifacts(f.cwd)
			out := imageProcess(f.cwd, f.environment, "/bin/sh", append([]string{f.installer}, ciPrepareArguments(runtime.GOOS+"/"+runtime.GOARCH)...)...)
			Expect(out.status).NotTo(BeZero(), "%s: %+v", scenario, out)
			Expect(out.status).NotTo(Equal(2), "a valid CI request must reach authentication: %+v", out)
			trace, err := os.ReadFile(filepath.Join(f.records, "trace"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(trace)).To(ContainSubstring("release-download\n"))
			Expect(string(trace)).NotTo(ContainSubstring("bootstrap\n"))
			for _, name := range []string{"candidate-executed", "early-chmod", "forbidden-tool"} {
				_, err := os.Lstat(filepath.Join(f.records, name))
				Expect(os.IsNotExist(err)).To(BeTrue(), name)
			}
			Expect(nativeInitArtifacts(f.cwd)).To(Equal(before))
			ciBootstrapCacheUnchanged(f)
			ordinaryBootstrapCache(f)
		}
	})
})

// per docs/adr/0144-ci-installer-bootstrap.md:98
// per docs/adr/0144-ci-installer-bootstrap.md:112
var _ = Describe("Whole installation upgrade release candidate handoff CI checkout preparation", Ordered, ContinueOnFailure, SpecPriority(2), Label("handoff-ci-bootstrap-project"), func() {
	var project ciPrepareProject
	BeforeAll(func() { project = ciPrepareProjectFixture() })

	// per docs/adr/0144-ci-installer-bootstrap.md:68
	// per docs/adr/0144-ci-installer-bootstrap.md:98
	It("prepares a real cloned project through the installer and runs installed checks without Go or Python", func() {
		f, _, root := project.checkout()
		ciBootstrapPoison(f)
		privatePublished := ciPreparePrivateWitness(root, project.private)
		beforePaths := ciPreparePublicPaths(root)
		head, index := imageGit(root, "rev-parse", "HEAD"), imageGit(root, "ls-files", "--stage", "-z")
		preserved := map[string][]byte{}
		for _, path := range []string{"application.txt", "factory.yaml", "Makefile", ".github/workflows/application.yml", "notes/operator-note", ".factory/operator-note", ".gitignore"} {
			data, err := os.ReadFile(filepath.Join(root, path))
			Expect(err).NotTo(HaveOccurred())
			preserved[path] = data
		}
		out := handoffNativeProcess(root, f.environment, "", "/bin/sh", append([]string{f.installer}, ciPrepareArguments(runtime.GOOS+"/"+runtime.GOARCH)...)...)
		AddReportEntry("ci-bootstrap.actual-result", map[string]any{"status": out.status, "stdout": out.stdout, "stderr": out.stderr}, ReportEntryVisibilityAlways)
		Expect(out.status).To(BeZero(), "%+v", out)
		privatePublished()
		defaultInitDescriptor(root, handoffVersion, project.image.revision, project.image.binary)
		// The inherited bridge poison tests the installer boundary above. Subsequent
		// public invocations use the normal caller environment of the existing fixture.
		f.environment["FACTORY_BRIDGE_PROTOCOL"] = ""
		proof := handoffNativeProcess(root, f.environment, "", filepath.Join(root, "factory"), "selftest")
		Expect(proof.status).To(BeZero(), "%+v", proof)
		Expect(proof.stdout).To(ContainSubstring("0 failed"))
		for _, status := range []string{"0", "23"} {
			f.environment["CI_PREPARE_CHECK_STATUS"] = status
			check := handoffNativeProcess(root, f.environment, "", filepath.Join(root, "factory"), "check")
			if status == "0" {
				Expect(check.status).To(BeZero(), "%+v", check)
			} else {
				Expect(check.status).NotTo(BeZero(), "%+v", check)
			}
		}
		Expect(os.ReadFile(f.environment["CI_PREPARE_CHECK_LOG"])).To(Equal([]byte("application check\napplication check\n")))
		for path, data := range preserved {
			Expect(os.ReadFile(filepath.Join(root, path))).To(Equal(data), path)
		}
		Expect(imageGit(root, "rev-parse", "HEAD")).To(Equal(head))
		Expect(imageGit(root, "ls-files", "--stage", "-z")).To(Equal(index))
		Expect(ciPreparePublicPaths(root)).To(Equal(beforePaths))
		trace, err := os.ReadFile(filepath.Join(f.records, "trace"))
		Expect(err).NotTo(HaveOccurred())
		AddReportEntry("ci-bootstrap.authentication-trace", string(trace), ReportEntryVisibilityAlways)
		// Shell raw authentication is followed by native raw/archive authentication
		// and the staging layer's independent confined archive snapshot verification.
		// per docs/adr/0144-ci-installer-bootstrap.md:128
		for name, count := range map[string]int{"release-download": 1, "api": 2, "release-view": 2, "trusted-root": 2, "verify": 4} {
			Expect(strings.Count(string(trace), name+"\n")).To(Equal(count), "shell authentication then independent native import: %s", name)
		}
		_, err = os.Lstat(filepath.Join(f.records, "forbidden-tool"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "bootstrap and installed checks must not use Go/Python or a source fallback")
		ciBootstrapCacheUnchanged(f)
		ordinaryBootstrapCache(f)
	})

	// per docs/adr/0144-ci-installer-bootstrap.md:106
	It("joins installer and native CI acquisition cancellation while preserving an unrelated process", func() {
		f, _, root := project.checkout()
		f.environment["FACTORY_RELEASE_DIR"] = ""
		help := imageProcess(root, f.environment, "/bin/sh", f.installer, "ci", "prepare", "--help")
		Expect(help.status).To(BeZero(), "the actual public installer must own the route before cancellation can be qualified: %+v", help)
		ready := filepath.Join(f.records, "native-ci-verifier-ready")
		path := filepath.Join(f.root, "host tools", "gh")
		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		old := "  verify)\n"
		Expect(strings.Count(string(data), old)).To(Equal(1))
		body := `  verify)
    if [ "${3##*/}" = archive ]; then
      /bin/sleep 120 &
      printf '%s %s\n' "$$" "$!" > "$CI_PREPARE_CANCEL_READY"
      wait
    fi
`
		writeFixture(path, []byte(strings.Replace(string(data), old, body, 1)), 0700)
		f.environment["CI_PREPARE_CANCEL_READY"] = ready
		unchanged := installationPreservationWitness(root)
		outsider := exec.Command("/bin/sleep", "30")
		outsider.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		Expect(outsider.Start()).To(Succeed())
		defer func() { _ = outsider.Process.Kill(); _ = outsider.Wait() }()
		ciPrepareCanceledProcess(f, "/bin/sh", root, ready, "acquisition", f.installer)
		Expect(outsider.Process.Signal(syscall.Signal(0))).To(Succeed(), "composed cancellation must preserve the unrelated process")
		unchanged()
		ordinaryBootstrapCache(f)
	})
})

// per docs/adr/0144-ci-installer-bootstrap.md:52
func ciBootstrapPoison(f handoffBootstrapFixture) {
	GinkgoHelper()
	f.environment["FACTORY_REF"] = "main"
	f.environment["FACTORY_REPO"] = "file:///must-not-select-this-repository"
	f.environment["FACTORY_HOME"] = filepath.Join(f.root, "unrelated source cache")
	f.environment["FACTORY_BRIDGE_PROTOCOL"] = "caller-must-not-select-private-bridge"
	f.environment["CI"] = "1"
	writeFixture(filepath.Join(f.environment["FACTORY_HOME"], "keep"), []byte("unrelated source cache\n"), 0600)
}

func ciBootstrapCacheUnchanged(f handoffBootstrapFixture) {
	GinkgoHelper()
	Expect(os.ReadFile(filepath.Join(f.environment["FACTORY_HOME"], "keep"))).To(Equal([]byte("unrelated source cache\n")))
	entries, err := os.ReadDir(f.environment["FACTORY_HOME"])
	Expect(err).NotTo(HaveOccurred())
	Expect(entries).To(HaveLen(1))
}
