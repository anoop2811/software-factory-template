package acceptance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/packaging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const defaultInitTag = "v0.2.0"
const defaultInitMarker = "INDEPENDENT_SELECTED_DEFAULT_INITIALIZER_20261030"

type defaultInitImage struct {
	result          packaging.Result
	binary, archive []byte
}
type defaultInitFixtures struct {
	invoker                 handoffReleaseImage
	source, revision, cache string
	images                  map[string]defaultInitImage
}

// A selected-only message witnesses the actual initializer owner, while the
// invoking public binary was compiled before this fixture commit.
// per docs/adr/0118-default-packaged-go-initialization.md:15
func prepareDefaultInit(patches ...func(string) []string) *defaultInitFixtures {
	GinkgoHelper()
	image := handoffBuildImage()
	Expect(string(image.binary)).NotTo(ContainSubstring(defaultInitMarker))
	source, err := filepath.EvalSymlinks(filepath.Join(image.work, "exact-source"))
	Expect(err).NotTo(HaveOccurred())
	imageGit(source, "config", "gc.auto", "0")
	imageGit(source, "config", "maintenance.auto", "false")
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	imageGit(source, "fetch", "--quiet", "--no-tags", repository, "b71ecc32e07ecd87eb330ba8e497c86612f92acd", "76952eaa63aebd1ecd282f5ab51dd7c3627cb497")
	path := filepath.Join(source, "internal/initcmd/command.go")
	data, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	changed := strings.Replace(string(data), `message := "factory-init:`, `message := "`+defaultInitMarker+`\nfactory-init:`, 1)
	Expect(changed).NotTo(Equal(string(data)))
	writeFixture(path, []byte(changed), 0644)
	changedFiles := []string{"internal/initcmd/command.go"}
	for _, patch := range patches {
		changedFiles = append(changedFiles, patch(source)...)
	}
	imageGit(source, append([]string{"add", "--"}, changedFiles...)...)
	imageGit(source, "commit", "--quiet", "-m", "independent selected default initializer witness")
	revision := strings.TrimSpace(imageGit(source, "rev-parse", "HEAD"))
	cache := filepath.Join(image.work, "qualified default images")
	Expect(os.Mkdir(cache, 0700)).To(Succeed())
	cache, err = filepath.EvalSymlinks(cache)
	Expect(err).NotTo(HaveOccurred())
	f := &defaultInitFixtures{image, source, revision, cache, map[string]defaultInitImage{}}
	f.image(defaultInitTag)
	AddReportEntry("default-init.selected-source", map[string]string{"source": source, "revision": revision, "invoker_revision": image.revision}, ReportEntryVisibilityNever)
	return f
}

func (f *defaultInitFixtures) image(version string) defaultInitImage {
	GinkgoHelper()
	if image, found := f.images[version]; found {
		return image
	}
	compiler, err := exec.LookPath("go")
	Expect(err).NotTo(HaveOccurred())
	compiler, err = filepath.EvalSymlinks(compiler)
	Expect(err).NotTo(HaveOccurred())
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := packaging.Build(ctx, packaging.Options{Source: f.source, Revision: f.revision, Version: version, Target: runtime.GOOS + "/" + runtime.GOARCH, Output: filepath.Join(f.cache, version), Compiler: compiler, Installation: true})
	Expect(err).NotTo(HaveOccurred())
	archive, err := os.ReadFile(result.Archive)
	Expect(err).NotTo(HaveOccurred())
	entries, _ := imageDecodeArchive(archive)
	var program []byte
	for _, entry := range entries {
		if strings.HasSuffix(entry.name, "/bin/factory-runtime") {
			program = entry.body
		}
	}
	Expect(program).NotTo(BeEmpty())
	Expect(string(program)).To(ContainSubstring(defaultInitMarker))
	image := defaultInitImage{result, program, archive}
	f.images[version] = image
	return image
}

func (f *defaultInitFixtures) surface(versions ...string) (handoffBootstrapFixture, string) {
	GinkgoHelper()
	surface, driver := handoffNativeFixture(f.invoker)
	surface.environment["HANDOFF_REVISION"] = f.revision
	delete(surface.environment, "FACTORY_REF")
	// Emulate the provider's requested projection: stable default selection
	// asks for isPrerelease as well, while explicit candidates retain two fields.
	gh := filepath.Join(surface.root, "host tools/gh")
	provider, err := os.ReadFile(gh)
	Expect(err).NotTo(HaveOccurred())
	row := `release-view) printf '%s\tfalse\n' "$3" ;;`
	complete := `release-view)
      [ "$HANDOFF_SCENARIO" != release-absent ] || exit 44
      prerelease=false
      case "$3" in *-*) prerelease=true ;; esac
      [ "$HANDOFF_SCENARIO" != marked-prerelease ] || prerelease=true
      case "$*" in *isPrerelease*) printf '%s\tfalse\t%s\n' "$3" "$prerelease" ;; *) printf '%s\tfalse\n' "$3" ;; esac ;;
`
	updated := strings.Replace(string(provider), row, complete, 1)
	Expect(updated).NotTo(Equal(string(provider)))
	writeFixture(gh, []byte(updated), 0700)

	for _, version := range versions {
		image := f.image(version)
		writeFixture(filepath.Join(surface.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+version+"-"+surface.target), image.binary, 0600)
		writeFixture(filepath.Join(surface.environment["HANDOFF_ASSETS"], "factory-installation-"+version+"-"+surface.target+".tar.gz"), image.archive, 0600)
	}
	writeFixture(filepath.Join(surface.environment["FACTORY_HOME"], "operator-witness"), []byte("foreign source cache; keep exactly\n"), 0600)
	return surface, driver
}

func defaultInitDescriptor(root, version, revision string, expected []byte) {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join(root, ".factory/installation.current"))
	Expect(err).NotTo(HaveOccurred())
	var descriptor map[string]any
	Expect(json.Unmarshal(data, &descriptor)).To(Succeed())
	Expect(descriptor["kind"]).To(Equal("go-hybrid-v1"))
	Expect(descriptor["version"]).To(Equal(version))
	Expect(descriptor["revision"]).To(Equal(revision))
	Expect(descriptor["target"]).To(Equal(runtime.GOOS + "/" + runtime.GOARCH))
	Expect(os.ReadFile(filepath.Join(root, ".factory/bin/factory-runtime"))).To(Equal(expected))
}

func defaultInitNoForbidden(surface handoffBootstrapFixture) {
	GinkgoHelper()
	_, err := os.Lstat(filepath.Join(surface.records, "forbidden-tool"))
	Expect(os.IsNotExist(err)).To(BeTrue(), "ordinary packaged initialization must not invoke compiler/Python/archive shell helpers")
	caches, err := filepath.Glob(filepath.Join(surface.root, "factory-release-*"))
	Expect(err).NotTo(HaveOccurred())
	Expect(caches).To(BeEmpty(), "confirmed initialization must clean its private acquisitions")
}

var _ = Describe("Go default packaged initialization", Ordered, ContinueOnFailure, func() {
	var fixtures *defaultInitFixtures
	BeforeAll(func() { fixtures = prepareDefaultInit() })
	// per docs/adr/0118-default-packaged-go-initialization.md:9
	It("runs the exact authenticated prepared default without ambient source assets or compiler fallback", func() {
		f, driver := fixtures.surface(defaultInitTag)
		target := filepath.Join(f.cwd, "new governed project")
		application := []byte("existing caller content\n")
		writeFixture(filepath.Join(f.cwd, "application.txt"), application, 0600)
		foreign := installationSnapshot(f.environment["FACTORY_HOME"])
		out := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", target, "--pack", "none")
		AddReportEntry("default-init.public-result", map[string]any{"status": out.status, "stdout": out.stdout, "stderr": out.stderr}, ReportEntryVisibilityNever)
		Expect(out.status).To(Equal(0), "ordinary Go initialization must select its prepared authenticated image: %+v", out)
		Expect(out.stdout).To(ContainSubstring(defaultInitMarker), "the selected compiled initializer, not the invoking build, owns setup")
		defaultInitDescriptor(target, defaultInitTag, fixtures.revision, fixtures.image(defaultInitTag).binary)
		Expect(handoffRecorded(f, "api")).To(ContainElement("repos/anoop2811/software-factory-template/commits/refs/tags/" + defaultInitTag))
		Expect(handoffRecorded(f, "release-view")).To(ContainElement("tagName,isDraft,isPrerelease"), "stable policy must survive the selected-runtime handoff")
		Expect(os.ReadFile(filepath.Join(f.cwd, "application.txt"))).To(Equal(application))
		Expect(installationSnapshot(f.environment["FACTORY_HOME"])).To(Equal(foreign))
		_, err := os.Lstat(filepath.Join(target, ".git"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		healthy := handoffNativeProcess(target, f.environment, "", filepath.Join(target, "factory"), "selftest")
		Expect(healthy.status).To(Equal(0), "%+v", healthy)
		defaultInitNoForbidden(f)
		fmt.Fprint(GinkgoWriter, "selected prepared Go initializer installed and qualified\n")
	})
	// per docs/adr/0118-default-packaged-go-initialization.md:25
	DescribeTable("keeps the admitted installed release when initializing another project", func(version string) {
		f, driver := fixtures.surface(version)
		origin := filepath.Join(f.cwd, "existing Go installation")
		first := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", origin, "--go-candidate", version, "--pack", "none")
		Expect(first.status).To(Equal(0), "%+v", first)
		before := installationSnapshot(origin)
		target := filepath.Join(f.cwd, "second project")
		actual := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), filepath.Join(origin, "factory"), "init", target, "--pack", "none")
		Expect(actual.status).To(Equal(0), "%+v", actual)
		Expect(actual.stdout).To(ContainSubstring(defaultInitMarker))
		defaultInitDescriptor(target, version, fixtures.revision, fixtures.image(version).binary)
		Expect(handoffRecorded(f, "api")).To(ContainElement("repos/anoop2811/software-factory-template/commits/refs/tags/" + version))
		Expect(installationSnapshot(origin)).To(Equal(before), "initializing another target must preserve the admitted source installation")
		defaultInitNoForbidden(f)
	}, Entry("stable selected release", defaultInitTag), Entry("selected minimum-family prerelease", "v0.2.0-rc.99"))

	// per docs/adr/0118-default-packaged-go-initialization.md:35
	It("refuses implicit local installed labels before GitHub and permits explicit official selection", func() {
		f, _ := fixtures.surface(defaultInitTag)
		local := fixtures.image("local-" + fixtures.revision)
		cache := filepath.Join(f.root, "borrowed local input")
		writeFixture(filepath.Join(cache, "package/factory-runtime.tar.gz"), local.archive, 0600)
		cache, err := filepath.EvalSymlinks(cache)
		Expect(err).NotTo(HaveOccurred())
		program := filepath.Join(f.root, "qualified local initializer")
		writeFixture(program, local.binary, 0700)
		origin := filepath.Join(f.cwd, "local Go installation")
		initialized := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), program, "--factory-local-init", "1", cache, fixtures.source, fixtures.revision, local.result.SHA256, "--", origin, "--pack", "none")
		Expect(initialized.status).To(Equal(0), "%+v", initialized)
		target := filepath.Join(f.cwd, "requested project")
		before := installationSnapshot(origin)
		refused := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), filepath.Join(origin, "factory"), "init", target, "--pack", "none")
		Expect(refused.status).To(Equal(2), "%+v", refused)
		Expect(refused.stderr).To(Or(ContainSubstring("--local"), ContainSubstring("official")))
		Expect(installationSnapshot(origin)).To(Equal(before))
		_, err = os.Lstat(target)
		Expect(os.IsNotExist(err)).To(BeTrue())
		help := handoffNativeProcess(f.cwd, f.environment, "", filepath.Join(origin, "factory"), "init", "--help")
		Expect(help.status).To(Equal(0), "%+v", help)
		localInitNoAcquisition(f)
		explicit := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), filepath.Join(origin, "factory"), "init", target, "--ref", defaultInitTag, "--pack", "none")
		Expect(explicit.status).To(Equal(0), "%+v", explicit)
		defaultInitDescriptor(target, defaultInitTag, fixtures.revision, fixtures.image(defaultInitTag).binary)
		Expect(installationSnapshot(origin)).To(Equal(before))
		defaultInitNoForbidden(f)
	})

	// per docs/adr/0118-default-packaged-go-initialization.md:23
	It("selects literal stable refs with explicit precedence over captured defaults", func() {
		for _, selection := range []struct {
			flags                 []string
			environment, expected string
		}{
			{[]string{"--ref", "v0.2.1"}, "main", "v0.2.1"},
			{[]string{"--ref=v0.2.1"}, defaultInitTag, "v0.2.1"},
			{nil, "v0.2.1", "v0.2.1"},
		} {
			f, driver := fixtures.surface(selection.expected)
			f.environment["FACTORY_REF"] = selection.environment
			target := filepath.Join(f.cwd, "ref selected project")
			args := append([]string{"init", target, "--pack", "none"}, selection.flags...)
			out := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("n"), driver, args...)
			Expect(out.status).To(Equal(1), "authenticated selection must reach explicit decline: %+v", out)
			Expect(out.stdout).To(ContainSubstring("Aborted"))
			Expect(handoffRecorded(f, "api")).To(ContainElement("repos/anoop2811/software-factory-template/commits/refs/tags/" + selection.expected))
			_, err := os.Lstat(target)
			Expect(os.IsNotExist(err)).To(BeTrue())
			defaultInitNoForbidden(f)
		}
	})

	// per docs/adr/0118-default-packaged-go-initialization.md:51
	It("owns malformed selectors before help or acquisition while help ignores ambient defaults", func() {
		f, driver := fixtures.surface(defaultInitTag)
		target := filepath.Join(f.cwd, "untouched project")
		writeFixture(filepath.Join(target, "README.md"), []byte("operator README\n"), 0640)
		before := installationSnapshot(target)
		invalid := [][]string{{"--ref"}, {"--ref="}, {"--ref", "--help"}, {"--ref", "main", "--help"}, {"--ref", "latest"}, {"--ref", "v0.1.6"}, {"--ref", "v0.2.0-rc.99"}, {"--ref", defaultInitTag, "--ref", defaultInitTag}, {"--ref=" + defaultInitTag, "--ref=" + defaultInitTag}, {"--ref", defaultInitTag, "--go-candidate", "v0.2.0-rc.99"}, {"--ref", defaultInitTag, "--local"}}
		for _, flags := range invalid {
			out := handoffNativeProcess(f.cwd, f.environment, "", driver, append([]string{"init", target}, flags...)...)
			Expect(out.status).To(Equal(2), "flags=%q: %+v", flags, out)
			Expect(installationSnapshot(target)).To(Equal(before))
			localInitNoAcquisition(f)
		}
		for _, value := range []string{"main", "v0.1.6", "v0.2.0-rc.99", "v0.2.0\nother"} {
			f.environment["FACTORY_REF"] = value
			noHelp := handoffNativeProcess(f.cwd, f.environment, "", driver, "init", target, "--pack", "none")
			Expect(noHelp.status).To(Equal(2), "ambient=%q: %+v", value, noHelp)
			for _, flag := range []string{"--help", "-h"} {
				help := handoffNativeProcess(f.cwd, f.environment, "", driver, "init", target, flag)
				Expect(help.status).To(Equal(0), "ambient=%q: %+v", value, help)
				Expect(help.stdout).To(ContainSubstring(defaultInitTag))
				Expect(help.stdout).To(ContainSubstring("--ref"))
			}
			Expect(installationSnapshot(target)).To(Equal(before))
			localInitNoAcquisition(f)
		}
	})

	// per docs/adr/0118-default-packaged-go-initialization.md:44
	DescribeTable("keeps terminated selector and initializer spellings as literal targets", func(literal string) {
		f, driver := fixtures.surface(defaultInitTag)
		out := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", "--pack", "none", "--", literal)
		Expect(out.status).To(Equal(0), "literal=%q: %+v", literal, out)
		Expect(out.stdout).To(ContainSubstring(defaultInitMarker))
		defaultInitDescriptor(filepath.Join(f.cwd, literal), defaultInitTag, fixtures.revision, fixtures.image(defaultInitTag).binary)
		defaultInitNoForbidden(f)
		if literal == "--go-candidate" {
			for _, target := range []string{"--go-candidate=v0.2.0-rc.99", "--ref", "--source", "--help"} {
				control, program := fixtures.surface(defaultInitTag)
				declined := handoffNativeProcess(control.cwd, control.environment, nativeInitAnswers("n"), program, "init", "--pack", "none", "--", target)
				Expect(declined.status).To(Equal(1), "terminated target=%q: %+v", target, declined)
				Expect(declined.stdout).To(ContainSubstring(filepath.Join(control.cwd, target)))
				Expect(declined.stdout).To(ContainSubstring("Aborted"))
				defaultInitNoForbidden(control)
			}
			control, program := fixtures.surface(defaultInitTag)
			trailing := handoffNativeProcess(control.cwd, control.environment, nativeInitAnswers("n"), program, "init", "--pack", "none", "--")
			Expect(trailing.status).To(Equal(1), "%+v", trailing)
			Expect(trailing.stdout).To(ContainSubstring(control.cwd))
			Expect(trailing.stdout).To(ContainSubstring("Aborted"))
			defaultInitNoForbidden(control)
		}
	}, Entry("candidate selector spelling", "--go-candidate"), Entry("known initializer option spelling", "--pack"))

	// per docs/adr/0118-default-packaged-go-initialization.md:12
	It("preserves targets and foreign caches on unavailable authentication and deliberate decline without source fallback", func() {
		for _, scenario := range []string{"release-absent", "marked-prerelease", "download-fail", "authentication-fail", "decline", "EOF"} {
			f, driver := fixtures.surface(defaultInitTag)

			f.environment["HANDOFF_SCENARIO"] = scenario
			target := filepath.Join(f.cwd, "existing project")
			writeFixture(filepath.Join(target, "README.md"), []byte("existing application must survive\n"), 0640)
			before, foreign := installationSnapshot(target), installationSnapshot(f.environment["FACTORY_HOME"])
			input := nativeInitAnswers("y")
			if scenario == "decline" {
				input = nativeInitAnswers("n")
			}
			if scenario == "EOF" {
				input = ""
			}
			out := handoffNativeProcess(f.cwd, f.environment, input, driver, "init", target, "--pack", "none")
			Expect(out.status).To(Equal(1), "scenario=%s: %+v", scenario, out)
			if scenario == "decline" {
				Expect(out.stdout).To(ContainSubstring("Aborted"))
			}
			if scenario == "release-absent" || scenario == "marked-prerelease" || scenario == "download-fail" || scenario == "authentication-fail" {
				Expect(out.stdout).NotTo(ContainSubstring("Proceed?"))
			}
			Expect(installationSnapshot(target)).To(Equal(before))
			Expect(installationSnapshot(f.environment["FACTORY_HOME"])).To(Equal(foreign))
			Expect(handoffRecorded(f, "api")).To(ContainElement("repos/anoop2811/software-factory-template/commits/refs/tags/" + defaultInitTag))
			defaultInitNoForbidden(f)
		}
	})

	// per docs/adr/0118-default-packaged-go-initialization.md:22
	It("keeps explicit candidate and local ownership ahead of ambient release defaults", func() {
		const version = "v0.2.0-rc.99"
		f, driver := fixtures.surface(version)
		f.environment["FACTORY_REF"] = "main"
		target := filepath.Join(f.cwd, "explicit candidate project")
		candidate := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", target, "--go-candidate", version, "--pack", "none")
		Expect(candidate.status).To(Equal(0), "%+v", candidate)
		defaultInitDescriptor(target, version, fixtures.revision, fixtures.image(version).binary)
		before := installationSnapshot(f.records)
		local := handoffNativeProcess(f.cwd, f.environment, "", driver, "init", filepath.Join(f.cwd, "local help target"), "--local", "--source", filepath.Join(f.cwd, "missing source"), "--help")
		Expect(local.status).To(Equal(0), "%+v", local)
		Expect(local.stdout).To(ContainSubstring("--local"))
		Expect(installationSnapshot(f.records)).To(Equal(before))
		defaultInitNoForbidden(f)
	})

})
