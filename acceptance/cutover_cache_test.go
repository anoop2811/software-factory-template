package acceptance_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// per docs/adr/0108-default-go-cutover.md:139
func cutoverNativeFixture(image handoffReleaseImage) (handoffBootstrapFixture, string) {
	GinkgoHelper()
	f, driver := handoffNativeFixture(image)
	DeferCleanup(func() {
		_, err := os.Lstat(filepath.Join(f.records, "forbidden-tool"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "installed execution must not invoke compilers or Python, including during recipe reconstruction")
	})
	return f, driver
}

func cutoverCachePaths(root string) []string {
	GinkgoHelper()
	cache := filepath.Join(root, ".factory/assets/scaffold")
	var paths []string
	Expect(filepath.WalkDir(cache, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		Expect(info.Mode().IsRegular()).To(BeTrue())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
		relative, err := filepath.Rel(cache, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relative))
		return nil
	})).To(Succeed())
	return paths
}

// Remove only declared source assets, retaining the real compiled runtime and
// both immutable Bash reference catalogs. This is an isolated negative fixture,
// not a claim that the altered archive is a published source revision.
// per docs/adr/0108-default-go-cutover.md:95
func cutoverWithoutPython(image handoffReleaseImage) handoffReleaseImage {
	GinkgoHelper()
	removed := map[string]bool{"scripts/lib/budget.py": true, "scripts/lib/budget_adapters.py": true, "scripts/lib/loop.py": true, "scripts/lib/runtime_transition.py": true}
	entries, _ := imageDecodeArchive(image.archive)
	var manifest imageManifestOracle
	Expect(json.Unmarshal(entries[3].body, &manifest)).To(Succeed())
	var assets []imageAssetOracle
	var retained = append([]stageEntry(nil), entries[:4]...)
	for index, asset := range manifest.Assets {
		if removed[asset.Path] {
			continue
		}
		assets = append(assets, asset)
		retained = append(retained, entries[4+index])
	}
	manifest.Assets = assets
	retained[3].body = imageMarshal(manifest)
	image.archive = stageArchive(retained)
	image.archivePath = filepath.Join(image.work, "without-obsolete-python.tar.gz")
	writeFixture(image.archivePath, image.archive, 0600)
	return image
}

var _ = Describe("Default Go initializer cache retirement", Ordered, ContinueOnFailure, func() {
	var image handoffReleaseImage
	BeforeAll(func() { image = handoffBuildImage() })
	// per docs/adr/0108-default-go-cutover.md:121
	It("keeps only the two required cache data assets and runs their native consumers", func() {
		f, driver := cutoverNativeFixture(image)
		target := filepath.Join(f.cwd, "small Go governed project")
		Expect(os.Mkdir(target, 0700)).To(Succeed())
		imageGit(target, "init", "--quiet")
		result := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", target, "--go-candidate", handoffVersion, "--pack", "none")
		Expect(result.status).To(Equal(0), "%+v", result)
		Expect(cutoverCachePaths(target)).To(ConsistOf("templates/metrics.html", "packs/review-lane/review-pr.yml"))
		for _, args := range [][]string{{"metrics", "--html", "--no-open"}, {"review-lane", "enable"}} {
			out := handoffNativeProcess(target, f.environment, "", filepath.Join(target, "factory"), args...)
			Expect(out.status).To(Equal(0), "%q: %+v", args, out)
		}
		workflow, err := os.ReadFile(filepath.Join(target, ".github/workflows/adversarial-review.yml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(workflow)).To(ContainSubstring("Adversarial review"))
	})
	// per docs/adr/0108-default-go-cutover.md:95
	It("initializes directly from a qualified image without obsolete Python implementation assets", func() {
		f, driver := cutoverNativeFixture(cutoverWithoutPython(image))
		target := filepath.Join(f.cwd, "without legacy implementations")
		out := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", target, "--go-candidate", handoffVersion, "--pack", "none")
		Expect(out.status).To(Equal(0), "%+v", out)
		Expect(cutoverCachePaths(target)).To(ConsistOf("templates/metrics.html", "packs/review-lane/review-pr.yml"))
		for _, path := range []string{"scripts/lib/budget.py", "scripts/lib/budget_adapters.py", "scripts/lib/loop.py", "scripts/lib/runtime_transition.py"} {
			_, err := os.Lstat(filepath.Join(target, filepath.FromSlash(path)))
			Expect(os.IsNotExist(err)).To(BeTrue(), strings.TrimSpace(path))
		}
	})
})
