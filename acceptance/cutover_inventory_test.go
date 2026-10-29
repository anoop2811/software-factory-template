package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Deliberately use a thin initializer containing no shell cp declarations. The
// manifest gate must still inspect the real native initializer's required data.
// per docs/adr/0108-default-go-cutover.md:95
func cutoverInventoryFixture() string {
	GinkgoHelper()
	_, work := fixture()
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	assets, payload := installationGitCatalog(repository, "HEAD")
	source := filepath.Join(work, "native source")
	installationMaterializeReference(source, installationReference{assets: assets, payload: payload})
	imageGit(source, "init", "--quiet")
	imageGit(source, "config", "gc.auto", "0")
	imageGit(source, "config", "maintenance.auto", "false")
	imageGit(source, "config", "core.fsmonitor", "false")
	imageGit(source, "add", "--force", "--all")
	imageGit(source, "commit", "--quiet", "-m", "independent native inventory source")
	writeFixture(filepath.Join(source, "factory"), candidateBinary, 0755)
	writeFixture(filepath.Join(source, "scripts/factory-init.sh"), []byte("#!/bin/bash\nexec \"$(dirname \"$0\")/../factory\" init \"$@\"\n"), 0755)
	hook, err := os.ReadFile(filepath.Join(repository, "scripts/hooks/copy-manifest-check.sh"))
	Expect(err).NotTo(HaveOccurred())
	writeFixture(filepath.Join(source, "scripts/hooks/copy-manifest-check.sh"), hook, 0755)
	return source
}

var _ = Describe("Default Go initializer inventory proof", func() {
	// per docs/adr/0108-default-go-cutover.md:95
	It("reports a nonempty tracked canonical native inventory through the private compiled bridge", func() {
		source := cutoverInventoryFixture()
		result := imageProcess(source, map[string]string{"FACTORY_BRIDGE_PROTOCOL": "1"}, filepath.Join(source, "factory"), "inventory", "init", source)
		Expect(result.status).To(Equal(0), "%+v", result)
		Expect(result.stdout).To(MatchRegexp(`^copy-manifest-check: [1-9][0-9]* canonical initializer assets are present and git-tracked\n$`))
		Expect(result.stderr).To(BeEmpty())
	})
	// per docs/adr/0108-default-go-cutover.md:98
	DescribeTable("refuses damaged canonical data even when the source initializer has no shell copy declarations", func(kind string) {
		for _, asset := range []string{"templates/metrics.html", "README.md"} {
			source := cutoverInventoryFixture()
			path := filepath.Join(source, filepath.FromSlash(asset))
			switch kind {
			case "missing":
				Expect(os.Remove(path)).To(Succeed())
			case "untracked":
				imageGit(source, "rm", "--cached", "--quiet", asset)
			case "symlink":
				Expect(os.Remove(path)).To(Succeed())
				outside := filepath.Join(filepath.Dir(source), "outside metrics")
				writeFixture(outside, []byte("private outside bytes\n"), 0600)
				Expect(os.Symlink(outside, path)).To(Succeed())
			}
			result := imageProcess(source, map[string]string{"FACTORY_BRIDGE_PROTOCOL": ""}, "bash", filepath.Join(source, "scripts/hooks/copy-manifest-check.sh"), source)
			Expect(result.status).To(Equal(1), "%+v", result)
			Expect(result.stdout + result.stderr).To(And(ContainSubstring("COPY-MANIFEST FAIL:"), ContainSubstring(asset)))
			Expect(strings.TrimSpace(result.stderr)).NotTo(ContainSubstring("private outside bytes"))
		}
	}, Entry("missing required source", "missing"), Entry("untracked required source", "untracked"), Entry("symlink required source", "symlink"))
})
