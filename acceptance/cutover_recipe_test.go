package acceptance_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const cutoverRC3Revision = "95486ecdd3868a319a0e6362ee99cafd8f154200"
const cutoverRC3Version = "v0.2.0-rc.3"
const cutoverRecipeAnswers = "Recipe Project\nrecipe-project\n@example\nrecipe-user\ninternal\ndocs\n\ninherit\nstandard\nn\ny\n"

type cutoverRecipeImages struct {
	before, after handoffReleaseImage
	paths         []string
}

// Rebuild actual immutable RC3 source locally. This fixture executes its trusted
// compiled initializer to create a real predecessor; the NEW consumer must infer
// ownership from reviewed data, never execute source from the supplied archive.
// per docs/adr/0108-default-go-cutover.md:107
func cutoverPrepareRecipes() cutoverRecipeImages {
	GinkgoHelper()
	after := handoffBuildImage()
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	source := filepath.Join(after.work, "immutable-rc3")
	imageGit(repository, "clone", "--quiet", "--shared", "--no-checkout", repository, source)
	imageGit(source, "checkout", "--quiet", "--detach", cutoverRC3Revision)
	Expect(strings.TrimSpace(imageGit(source, "rev-parse", "HEAD"))).To(Equal(cutoverRC3Revision))
	binaryPath := filepath.Join(after.work, "locally-rebuilt-rc3-runtime")
	build := imageProcess(source, nil, "go", "build", "-o", binaryPath, "./cmd/factory")
	Expect(build.status).To(Equal(0), "%+v", build)
	binary, err := os.ReadFile(binaryPath)
	Expect(err).NotTo(HaveOccurred())
	assets, payload := installationGitCatalog(repository, cutoverRC3Revision)
	var baselines []imageBaselineOracle
	for _, profile := range []string{"v0.1.6", "bash-baseline"} {
		ref := after.references[profile]
		baselines = append(baselines, imageBaselineOracle{profile, ref.revision, ref.assets})
	}
	manifest := imageManifestOracle{1, "installation_source_image", "committed_source_and_baseline_references", cutoverRC3Version, runtime.GOOS + "/" + runtime.GOARCH, cutoverRC3Revision, false, assets, baselines}
	archive := stageArchive(imageFixtureEntries(manifest, payload, binary))
	archivePath := filepath.Join(after.work, "locally-rebuilt-rc3-image.tar.gz")
	writeFixture(archivePath, archive, 0600)
	before := handoffReleaseImage{binary, archive, archivePath, cutoverRC3Revision, after.work, after.references}
	var roles installationRoleMapOracle
	Expect(json.Unmarshal(payload["docs/migration/INSTALLATION_ROLE_MAP.json"], &roles)).To(Succeed())
	var paths []string
	for _, row := range roles.CanonicalSourceAssets {
		paths = append(paths, row.Path)
	}
	Expect(paths).To(HaveLen(85), "immutable RC3 independently defines the prior cache census")
	return cutoverRecipeImages{before, after, paths}
}

func (images cutoverRecipeImages) predecessor() (handoffBootstrapFixture, string, string, []string) {
	GinkgoHelper()
	f, oldDriver := cutoverNativeFixture(images.before)
	assets := f.environment["HANDOFF_ASSETS"]
	writeFixture(filepath.Join(assets, "factory-bootstrap-"+cutoverRC3Version+"-"+f.target), images.before.binary, 0600)
	writeFixture(filepath.Join(assets, "factory-installation-"+cutoverRC3Version+"-"+f.target+".tar.gz"), images.before.archive, 0600)
	root := filepath.Join(f.cwd, "genuine RC3 project")
	Expect(os.Mkdir(root, 0700)).To(Succeed())
	imageGit(root, "init", "--quiet")
	initialized := handoffNativeProcess(f.cwd, f.environment, cutoverRecipeAnswers, oldDriver, "init", root, "--go-candidate", cutoverRC3Version, "--pack", "none")
	Expect(initialized.status).To(Equal(0), "%+v", initialized)
	Expect(cutoverCachePaths(root)).To(HaveLen(85))
	transport, driver := cutoverNativeFixture(images.after)
	return transport, driver, root, cutoverRecipeOperands(images.before, images.after, cutoverRC3Version)
}

func cutoverRecipeOperands(before, after handoffReleaseImage, beforeVersion string) []string {
	args := []string{"upgrade", "--installation", "--profile", "go-hybrid-v1"}
	for _, part := range []struct {
		prefix  string
		image   handoffReleaseImage
		version string
	}{{"", after, handoffVersion}, {"before-", before, beforeVersion}} {
		args = append(args, "--"+part.prefix+"source", part.image.archivePath, "--"+part.prefix+"version", part.version, "--"+part.prefix+"revision", part.image.revision, "--"+part.prefix+"target", runtime.GOOS+"/"+runtime.GOARCH, "--"+part.prefix+"local")
	}
	for _, value := range []string{"PROJECT_NAME=Recipe Project", "PROJECT_SLUG=recipe-project", "GITHUB_OWNER=@example", "OPENCODE_USERNAME=recipe-user", "PROTECTED_PATH=internal", "DOCS_ROOT=docs", "CITATION_PREFIX="} {
		args = append(args, "--project-input", value)
	}
	return args
}

var _ = Describe("Whole installation upgrade known RC3 cache recipe", Ordered, ContinueOnFailure, func() {
	var images cutoverRecipeImages
	BeforeAll(func() { images = cutoverPrepareRecipes() })
	// per docs/adr/0108-default-go-cutover.md:114
	// per docs/adr/0108-default-go-cutover.md:101
	It("reconstructs the exact old selection, retires all 83 obsolete cache rows, and rolls back without losing custom state", func() {
		f, driver, root, args := images.predecessor()
		cached := map[string][]byte{}
		for _, path := range images.paths {
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
			Expect(err).NotTo(HaveOccurred())
			cached[path] = data
		}
		preserved := map[string][]byte{"docs/recipe-user-note.md": []byte("user customization\n"), ".factory/loops.json": []byte("{\"schema\":1,\"runs\":[],\"note\":\"preserve history\"}\n"), ".factory/events.log": []byte("2026-10-28T00:00:00Z\toperator\tpreserve history\n")}
		for path, data := range preserved {
			writeFixture(filepath.Join(root, filepath.FromSlash(path)), data, 0600)
		}
		before := installationSnapshot(root)
		proposal := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...))
		Expect(installationSnapshot(root)).To(Equal(before))
		actions := map[string]map[string]any{}
		for _, raw := range proposal["actions"].([]any) {
			row := raw.(map[string]any)
			actions[row["path"].(string)] = row
		}
		retired := 0
		for _, path := range images.paths {
			Expect(actions).To(HaveKey(path))
			if strings.HasSuffix(path, "/templates/metrics.html") || strings.HasSuffix(path, "/packs/review-lane/review-pr.yml") {
				continue
			}
			Expect(actions[path]["action"]).To(Equal("retire"), path)
			Expect(actions[path]["required"]).To(BeFalse(), path)
			retired++
		}
		Expect(retired).To(Equal(83))
		applied := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--migration-id", "rc3-cache-cleanup", "--confirm", proposal["proposal_digest"].(string), "--quiescent")...)
		Expect(applied.status).To(Equal(0), "%+v", applied)
		Expect(cutoverCachePaths(root)).To(ConsistOf("templates/metrics.html", "packs/review-lane/review-pr.yml"))
		for path, original := range cached {
			if actions[path]["action"] != "retire" {
				continue
			}
			saved := filepath.Join(root, ".factory/backups/rc3-cache-cleanup/assets", filepath.FromSlash(path))
			Expect(os.ReadFile(saved)).To(Equal(original), path)
			info, err := os.Stat(saved)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)), path)
		}
		backup := filepath.Join(root, ".factory/backups/rc3-cache-cleanup/assets/.factory/assets/scaffold/scripts/lib/loop.py")
		data, err := os.ReadFile(backup)
		Expect(err).NotTo(HaveOccurred())
		Expect(data).NotTo(BeEmpty())
		info, err := os.Stat(backup)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
		ignored := imageProcess(root, nil, "git", "check-ignore", "--no-index", "-q", "--", backup)
		Expect(ignored.status).To(Equal(0), "%+v", ignored)
		for path, data := range preserved {
			Expect(os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))).To(Equal(data))
		}
		reverseArgs := append(append([]string{}, args...), "--rollback", "rc3-cache-cleanup")
		reverse := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, reverseArgs...), "--dry-run", "--json")...))
		restored := handoffNativeProcess(root, f.environment, "", driver, append(reverseArgs, "--confirm", reverse["proposal_digest"].(string), "--quiescent")...)
		Expect(restored.status).To(Equal(0), "%+v", restored)
		Expect(cutoverCachePaths(root)).To(HaveLen(85))
		actual, err := os.ReadFile(filepath.Join(root, ".factory/bin/factory-runtime"))
		Expect(err).NotTo(HaveOccurred())
		Expect(bytes.Equal(actual, images.before.binary)).To(BeTrue())
		for path, data := range preserved {
			Expect(os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))).To(Equal(data))
		}
	})
	// per docs/adr/0108-default-go-cutover.md:108
	It("refuses an unrecognized prior fingerprint without executing it or changing the adopter", func() {
		f, driver, root, args := images.predecessor()
		before := installationSnapshot(root)
		for _, asset := range []string{"internal/installation/generate.go", "internal/installation/roles.go", "internal/installation/orchestration.go", "internal/installation/reference.go", "internal/installation/model.go", "internal/installation/observe.go", "internal/installation/selection.go", "internal/installedlayout/layout.go"} {
			changed := handoffRewriteAsset(images.before.archive, asset, func(data []byte) []byte { return append(data, []byte("\n// unknown recipe must remain inert\n")...) })
			path := filepath.Join(images.after.work, "unknown-prior-recipe.tar.gz")
			writeFixture(path, changed, 0600)
			for index, arg := range args {
				if arg == "--before-source" {
					args[index+1] = path
				}
			}
			out := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...)
			Expect(out.status).To(Equal(2), "%s: %+v", asset, out)
			Expect(out.stderr).To(ContainSubstring("recipe"))
			Expect(installationSnapshot(root)).To(Equal(before), asset)
		}
	})
	// per docs/adr/0108-default-go-cutover.md:101
	It("preserves an edited obsolete cache asset instead of inferring ownership from its filename", func() {
		f, driver, root, args := images.predecessor()
		path := ".factory/assets/scaffold/scripts/lib/loop.py"
		custom := []byte("operator maintained reference; never execute\n")
		writeFixture(filepath.Join(root, filepath.FromSlash(path)), custom, 0600)
		before := installationSnapshot(root)
		out := handoffNativeProcess(root, f.environment, "", driver, append(args, "--dry-run", "--json")...)
		Expect(out.status).To(Or(Equal(0), Equal(2)), "%+v", out)
		if out.status == 0 {
			proposal := handoffProposal(out)
			found := false
			for _, raw := range proposal["actions"].([]any) {
				row := raw.(map[string]any)
				if row["path"] == path {
					found = true
					Expect(row["action"]).To(Equal("retain"))
				}
			}
			Expect(found).To(BeTrue())
		} else {
			Expect(out.stderr).NotTo(BeEmpty())
		}
		Expect(installationSnapshot(root)).To(Equal(before))
		Expect(os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))).To(Equal(custom))
	})
	// per docs/adr/0108-default-go-cutover.md:119
	It("includes the new selector and prior-role table in current-recipe compatibility checks", func() {
		f, driver := cutoverNativeFixture(images.after)
		root := filepath.Join(f.cwd, "current recipe project")
		Expect(os.Mkdir(root, 0700)).To(Succeed())
		imageGit(root, "init", "--quiet")
		out := handoffNativeProcess(f.cwd, f.environment, cutoverRecipeAnswers, driver, "init", root, "--go-candidate", handoffVersion, "--pack", "none")
		Expect(out.status).To(Equal(0), "%+v", out)
		args := cutoverRecipeOperands(images.after, images.after, handoffVersion)
		before := installationSnapshot(root)
		for _, asset := range []string{"internal/installation/recipe.go", "internal/installation/roles_v1.go"} {
			changed := handoffRewriteAsset(images.after.archive, asset, func(data []byte) []byte { return append(data, []byte("\n// incompatible current recipe\n")...) })
			path := filepath.Join(images.after.work, "changed-current-recipe.tar.gz")
			writeFixture(path, changed, 0600)
			for index, arg := range args {
				if arg == "--before-source" {
					args[index+1] = path
				}
			}
			out = handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...)
			Expect(out.status).To(Equal(2), "%s: %+v", asset, out)
			Expect(out.stderr).To(ContainSubstring("recipe"))
			Expect(installationSnapshot(root)).To(Equal(before), asset)
		}
	})
})
