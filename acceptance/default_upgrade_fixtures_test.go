package acceptance_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const defaultUpgradeNextTag = "v0.2.1"
const defaultUpgradeBeforeRC = "v0.2.0-rc.3"

type defaultUpgradeJourneyFixtures struct {
	*defaultInitFixtures
	legacy installationFixtureData
}
type defaultUpgradeProject struct {
	f                           handoffBootstrapFixture
	driver, root, beforeVersion string
	inputs                      []string
	before                      defaultInitImage
	legacy                      installationFixtureData
}

// Publisher versions are controlled fixture inputs; real historical recipe
// reconstruction remains independently covered by the predecessor family.
// per docs/adr/0122-default-packaged-go-upgrade.md:52
func prepareDefaultUpgradeJourneys(withLegacy bool) *defaultUpgradeJourneyFixtures {
	f := &defaultUpgradeJourneyFixtures{defaultInitFixtures: prepareDefaultUpgrade()}
	if withLegacy {
		f.legacy = installationLegacyFixture(f.invoker.binary, f.invoker.references["v0.1.6"], f.invoker.archivePath, f.invoker.revision)
		imageGit(f.legacy.root, "config", "gc.auto", "0")
		imageGit(f.legacy.root, "config", "maintenance.auto", "false")
	}
	return f
}
func defaultUpgradeInputArgs(inputs []string, extra ...string) []string {
	args := []string{"upgrade"}
	for _, input := range inputs {
		args = append(args, "--project-input", input)
	}
	return append(args, extra...)
}
func (f *defaultUpgradeJourneyFixtures) provide(surface handoffBootstrapFixture, versions ...string) {
	GinkgoHelper()
	for _, version := range versions {
		image := f.image(version)
		writeFixture(filepath.Join(surface.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+version+"-"+surface.target), image.binary, 0600)
		writeFixture(filepath.Join(surface.environment["HANDOFF_ASSETS"], "factory-installation-"+version+"-"+surface.target+".tar.gz"), image.archive, 0600)
	}
}

func (f *defaultUpgradeJourneyFixtures) legacyProject(versions ...string) defaultUpgradeProject {
	GinkgoHelper()
	Expect(f.legacy.root).NotTo(BeEmpty(), "prepare the immutable legacy project in BeforeAll, not an It cleanup")
	surface, driver := f.surface(append([]string{defaultUpgradeTag}, versions...)...)
	legacy := installationCloneLegacyFixture(f.invoker.binary, f.legacy)
	imageGit(legacy.root, "config", "gc.auto", "0")
	imageGit(legacy.root, "config", "maintenance.auto", "false")
	return defaultUpgradeProject{f: surface, driver: driver, root: legacy.root, inputs: append([]string{}, installationInputs...), legacy: legacy}
}
func (f *defaultUpgradeJourneyFixtures) goProject(version string, projectName ...string) defaultUpgradeProject {
	GinkgoHelper()
	surface, driver := f.surface(defaultUpgradeTag, version)
	root := filepath.Join(surface.cwd, "go governed project")
	Expect(os.Mkdir(root, 0700)).To(Succeed())
	imageGit(root, "init", "--quiet")
	imageGit(root, "config", "gc.auto", "0")
	imageGit(root, "config", "maintenance.auto", "false")
	answers := localUpgradeAnswers
	inputs := append([]string{}, installationInputs...)
	if len(projectName) == 1 {
		answers = strings.Replace(answers, "Installation Project\n", projectName[0]+"\n", 1)
		for i, input := range inputs {
			if strings.HasPrefix(input, "PROJECT_NAME=") {
				inputs[i] = "PROJECT_NAME=" + projectName[0]
			}
		}
	}
	initialized := handoffNativeProcess(surface.cwd, surface.environment, answers, driver, "init", root, "--go-candidate", version, "--pack", "none")
	Expect(initialized.status).To(Equal(0), "genuine Go predecessor setup: %+v", initialized)
	image := f.image(version)
	defaultInitDescriptor(root, version, f.revision, image.binary)
	return defaultUpgradeProject{f: surface, driver: driver, root: root, beforeVersion: version, inputs: inputs, before: image}
}
func (p defaultUpgradeProject) args(extra ...string) []string {
	return defaultUpgradeInputArgs(p.inputs, extra...)
}
func (p defaultUpgradeProject) run(extra ...string) cliResult {
	return handoffNativeProcess(p.root, p.f.environment, "", p.driver, p.args(extra...)...)
}
func (p defaultUpgradeProject) preview(extra ...string) map[string]any {
	return handoffProposal(p.run(append([]string{"--dry-run", "--json"}, extra...)...))
}
func (p defaultUpgradeProject) apply(proposal map[string]any, extra ...string) cliResult {
	flags := append([]string{}, extra...)
	return p.run(append(flags, "--confirm", proposal["proposal_digest"].(string), "--quiescent")...)
}

// Rollback keeps its archive/trust owner. Saved scripts are inert comparisons;
// only genuine legacy health receives provisioned historical tools.
// per docs/adr/0122-default-packaged-go-upgrade.md:77
func (f *defaultUpgradeJourneyFixtures) rollbackArgs(p defaultUpgradeProject, selectedTag, migration string) []string {
	GinkgoHelper()
	roots := filepath.Join(p.f.root, "operator-trusted-roots.jsonl")
	writeFixture(roots, []byte("independently supplied fixture roots\n"), 0600)
	profile := "v0.1.6"
	if p.beforeVersion != "" {
		profile = "go-hybrid-v1"
	}
	args := []string{"upgrade", "--installation", "--profile", profile, "--rollback", migration}
	type part struct {
		prefix  string
		image   defaultInitImage
		version string
	}
	parts := []part{{"", f.image(selectedTag), selectedTag}}
	if p.beforeVersion != "" {
		parts = append(parts, part{"before-", p.before, p.beforeVersion})
	}
	for _, item := range parts {
		args = append(args, "--"+item.prefix+"source", item.image.result.Archive, "--"+item.prefix+"version", item.version, "--"+item.prefix+"revision", f.revision, "--"+item.prefix+"target", runtime.GOOS+"/"+runtime.GOARCH, "--"+item.prefix+"attestation", filepath.Join(p.f.environment["HANDOFF_ASSETS"], "runtime-attestations.jsonl"), "--"+item.prefix+"trusted-root", roots)
	}
	for _, input := range p.inputs {
		args = append(args, "--project-input", input)
	}
	return args
}

func defaultUpgradeLegacyRecoveryEnvironment(p defaultUpgradeProject) map[string]string {
	GinkgoHelper()
	env := map[string]string{}
	for key, value := range p.f.environment {
		env[key] = value
	}
	tools := filepath.Join(p.f.root, "historical rollback tools")
	gh, err := os.ReadFile(filepath.Join(p.f.root, "host tools/gh"))
	Expect(err).NotTo(HaveOccurred())
	writeFixture(filepath.Join(tools, "gh"), gh, 0700)
	env["PATH"] = tools + string(os.PathListSeparator) + filepath.Join(filepath.Dir(p.legacy.source), "external tools") + string(os.PathListSeparator) + os.Getenv("PATH")
	return env
}
func defaultUpgradeNoAcquisition(f handoffBootstrapFixture) {
	GinkgoHelper()
	_, err := os.Lstat(filepath.Join(f.records, "trace"))
	Expect(os.IsNotExist(err)).To(BeTrue(), "strict/help/legacy owners must not acquire releases")
	defaultInitNoForbidden(f)
}
