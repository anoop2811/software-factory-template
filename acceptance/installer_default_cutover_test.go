package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Only release-policy data changes in this private installer fixture. Support
// the recorded old pin spelling while observing RED before the named pin exists.
// per docs/adr/0108-default-go-cutover.md:35
func installerPreparedGoPin(f handoffBootstrapFixture) {
	GinkgoHelper()
	body, err := os.ReadFile(f.installer)
	Expect(err).NotTo(HaveOccurred())
	old, next := "INSTALLER_STABLE_VERSION='v0.1.6'", "INSTALLER_STABLE_VERSION='v0.2.0'"
	if !strings.Contains(string(body), old) {
		old, next = `FACTORY_REF="${FACTORY_REF:-v0.1.6}"`, `FACTORY_REF="${FACTORY_REF:-v0.2.0}"`
	}
	Expect(strings.Count(string(body), old)).To(Equal(1), "replace exactly the declared stable pin, never executable policy")
	writeFixture(f.installer, []byte(strings.Replace(string(body), old, next, 1)), 0600)
}

// per docs/adr/0108-default-go-cutover.md:36
func installerCutoverSurface(scenario string) handoffBootstrapFixture {
	GinkgoHelper()
	f := handoffBootstrap(map[string]string{"linux": "Linux", "darwin": "Darwin"}[runtime.GOOS], map[string]string{"amd64": "x86_64", "arm64": "arm64"}[runtime.GOARCH], runtime.GOOS+"-"+runtime.GOARCH, scenario)
	defaultReleasePolicyMetadata(f)
	assets := f.environment["HANDOFF_ASSETS"]
	writeFixture(filepath.Join(assets, "factory-bootstrap-v0.2.0-"+f.target), f.raw, 0600)
	writeFixture(filepath.Join(assets, "factory-installation-v0.2.0-"+f.target+".tar.gz"), []byte("inert installation input for the authenticated bootstrap consumer"), 0600)
	f.environment["FACTORY_REF"] = ""
	f.environment["HANDOFF_METADATA_PRERELEASE"] = "false"
	return f
}

// per docs/adr/0108-default-go-cutover.md:29
func installerCutoverOperands(f handoffBootstrapFixture, verb string) []string {
	if verb == "init" {
		return []string{"init", filepath.Join(f.cwd, "literal $(not-code) installer target"), "--pack", "none"}
	}
	return []string{"upgrade", "--dry-run", "--json", "--project-input", "PROJECT_NAME=literal $(not-code) installer value"}
}

// per docs/adr/0108-default-go-cutover.md:31
func installerCutoverAccepted(f handoffBootstrapFixture, args, operands []string) {
	GinkgoHelper()
	before := installationSnapshot(f.cwd)
	result := imageProcess(f.cwd, f.environment, "/bin/sh", append([]string{f.installer}, args...)...)
	Expect(result.status).To(Equal(0), "%+v", result)
	receipt := handoffRecorded(f, "bootstrap-argv")
	var tag string
	var actual []string
	for index := 0; index < len(receipt); index++ {
		if receipt[index] == "--ref" && index+1 < len(receipt) {
			Expect(tag).To(BeEmpty(), "the stable receipt must have one release selector")
			tag = receipt[index+1]
			index++
			continue
		}
		if strings.HasPrefix(receipt[index], "--ref=") {
			Expect(tag).To(BeEmpty(), "the stable receipt must have one release selector")
			tag = strings.TrimPrefix(receipt[index], "--ref=")
			continue
		}
		actual = append(actual, receipt[index])
	}
	Expect(tag).To(Equal("v0.2.0"))
	Expect(actual).To(Equal(operands), "metadata/ref selection cannot re-split or evaluate user operands")
	verification := handoffRecorded(f, "verify")
	Expect(handoffFlag(verification, "--repo")).To(Equal("anoop2811/software-factory-template"))
	Expect(handoffFlag(verification, "--source-ref")).To(Equal("refs/tags/v0.2.0"))
	Expect(verification).To(ContainElement("--deny-self-hosted-runners"))
	trace, err := os.ReadFile(filepath.Join(f.records, "trace"))
	Expect(err).NotTo(HaveOccurred())
	Expect(string(trace)).To(ContainSubstring("verify\nbootstrap\n"), "authentication must precede first execution")
	for _, marker := range []string{"forbidden-tool", "early-chmod"} {
		_, err := os.Lstat(filepath.Join(f.records, marker))
		Expect(os.IsNotExist(err)).To(BeTrue(), marker)
	}
	Expect(installationSnapshot(f.cwd)).To(Equal(before), "the inert consumer cannot mutate the project during a selection proof")
}

var _ = Describe("Go installer default cutover", func() {
	// per docs/adr/0108-default-go-cutover.md:29
	DescribeTable("routes exact Go refs through authenticated acquisition with literal operands", func(verb string) {
		for _, placement := range []string{"before", "after", "equals"} {
			f := installerCutoverSurface("")
			operands := installerCutoverOperands(f, verb)
			args := append([]string(nil), operands...)
			switch placement {
			case "before":
				args = append([]string{"--ref", "v0.2.0"}, args...)
			case "after":
				args = append(args, "--ref", "v0.2.0")
			case "equals":
				args = append(args, "--ref=v0.2.0")
			}
			installerCutoverAccepted(f, args, operands)
		}
	}, Entry("init", "init"), Entry("upgrade", "upgrade"))

	// per docs/adr/0108-default-go-cutover.md:35
	DescribeTable("prepares the declared Go default without activating the published stable installer", func(verb string) {
		f := installerCutoverSurface("")
		installerPreparedGoPin(f)
		operands := installerCutoverOperands(f, verb)
		installerCutoverAccepted(f, operands, operands)
	}, Entry("init", "init"), Entry("upgrade", "upgrade"))

	// per docs/adr/0108-default-go-cutover.md:31
	It("refuses a selected Go release failure without invoking legacy fallback", func() {
		for _, scenario := range []string{"download-fail", "missing-image", "roots-fail", "authentication-fail"} {
			f := installerCutoverSurface(scenario)
			installerPreparedGoPin(f)
			before := installationSnapshot(f.cwd)
			result := imageProcess(f.cwd, f.environment, "/bin/sh", append([]string{f.installer}, installerCutoverOperands(f, "init")...)...)
			Expect(result.status).NotTo(Equal(0), "%s: %+v", scenario, result)
			Expect(handoffRecorded(f, "api")).To(ContainElement("repos/anoop2811/software-factory-template/commits/refs/tags/v0.2.0"))
			for _, path := range []string{f.marker, filepath.Join(f.records, "forbidden-tool")} {
				_, err := os.Lstat(path)
				Expect(os.IsNotExist(err)).To(BeTrue(), "%s cannot execute a candidate, legacy path, compiler or interpreter", scenario)
			}
			Expect(installationSnapshot(f.cwd)).To(Equal(before))
		}
	})

	// per docs/adr/0108-default-go-cutover.md:35
	// per docs/adr/0108-default-go-cutover.md:43
	DescribeTable("keeps bare fetch inert and prints the selected authenticated installer next step", func(version string, explicitRef, preparedPin bool) {
		f := installerCutoverSurface("")
		if preparedPin {
			installerPreparedGoPin(f)
		}
		repository, err := filepath.Abs("..")
		Expect(err).NotTo(HaveOccurred())
		assets, payload := installationGitCatalog(repository, "b71ecc32e07ecd87eb330ba8e497c86612f92acd")
		origin := filepath.Join(f.root, "frozen local origin")
		installationMaterializeReference(origin, installationReference{assets: assets, payload: payload})
		downloadedExecution := filepath.Join(f.records, "downloaded-source-executed")
		if preparedPin {
			// Inspection of a future source checkout grants no execution authority.
			// per docs/adr/0108-default-go-cutover.md:43
			writeFixture(filepath.Join(origin, "scripts/lib/timing.sh"), []byte("printf called > \"$BARE_DOWNLOADED_SOURCE_EXECUTION\"\nfactory_duration() { printf '0s'; }\n"), 0644)
		}
		imageGit(origin, "init", "--quiet")
		imageGit(origin, "add", "--force", "--all")
		imageGit(origin, "commit", "--quiet", "-m", "immutable v0.1.6 source fixture")
		imageGit(origin, "tag", version)
		git, err := exec.LookPath("git")
		Expect(err).NotTo(HaveOccurred())
		git, err = filepath.EvalSymlinks(git)
		Expect(err).NotTo(HaveOccurred())
		tool := filepath.Join(f.root, "host tools", "git")
		Expect(os.Remove(tool)).To(Succeed())
		Expect(os.Symlink(git, tool)).To(Succeed())
		f.environment["FACTORY_REPO"] = origin
		f.environment["BARE_DOWNLOADED_SOURCE_EXECUTION"] = downloadedExecution
		imageGit(f.cwd, "init", "--quiet")
		writeFixture(filepath.Join(f.cwd, "project.txt"), []byte("adopter-owned source\n"), 0644)
		imageGit(f.cwd, "add", "--all")
		imageGit(f.cwd, "commit", "--quiet", "-m", "adopter history before bare fetch")
		head := imageGit(f.cwd, "rev-parse", "HEAD")
		status := imageGit(f.cwd, "status", "--porcelain=v1")
		before := installationSnapshot(f.cwd)
		args := []string{f.installer}
		if explicitRef {
			args = append(args, "--ref", version)
		}
		result := imageProcess(f.cwd, f.environment, "/bin/sh", args...)
		Expect(result.status).To(Equal(0), "%+v", result)
		Expect(installationSnapshot(f.cwd)).To(Equal(before))
		Expect(imageGit(f.cwd, "rev-parse", "HEAD")).To(Equal(head))
		Expect(imageGit(f.cwd, "status", "--porcelain=v1")).To(Equal(status))
		Expect(os.ReadFile(filepath.Join(f.environment["FACTORY_HOME"], "scripts/factory-init.sh"))).To(Equal(payload["scripts/factory-init.sh"]))
		Expect(result.stdout).To(ContainSubstring("sh -s -- init"))
		Expect(strings.TrimSpace(imageGit(f.environment["FACTORY_HOME"], "describe", "--exact-match", "--tags", "HEAD"))).To(Equal(version))
		if preparedPin {
			Expect(result.stdout).To(ContainSubstring("https://softwareaifactory.sh/install.sh | sh -s -- init --ref v0.2.0"), "the inspection checkout is not a compiler-free installed initializer")
			Expect(result.stdout).NotTo(ContainSubstring(filepath.Join(f.environment["FACTORY_HOME"], "scripts/factory-init.sh")))
		}
		for _, marker := range []string{f.marker, downloadedExecution, filepath.Join(f.records, "api"), filepath.Join(f.records, "release-view"), filepath.Join(f.records, "release-download"), filepath.Join(f.records, "trusted-root"), filepath.Join(f.records, "verify"), filepath.Join(f.records, "forbidden-tool")} {
			_, err := os.Lstat(marker)
			Expect(os.IsNotExist(err)).To(BeTrue(), "bare fetch cannot grant initializer or tool execution")
		}
	}, Entry("active frozen stable pin", "v0.1.6", false, false), Entry("explicit frozen ref", "v0.1.6", true, false), Entry("prepared Go stable pin", "v0.2.0", false, true))
})
