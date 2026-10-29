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
	// per docs/adr/0111-source-launcher-and-adopter-boundaries.md:72
	It("reports a nonempty tracked canonical native inventory through the private compiled bridge", func() {
		source := cutoverInventoryFixture()
		result := imageProcess(source, map[string]string{"FACTORY_BRIDGE_PROTOCOL": "1"}, filepath.Join(source, "factory"), "inventory", "init", source)
		Expect(result.status).To(Equal(0), "%+v", result)
		Expect(result.stdout).To(MatchRegexp(`^copy-manifest-check: [1-9][0-9]* canonical initializer assets are present and git-tracked\n$`))
		Expect(result.stderr).To(BeEmpty())
		hook := imageProcess(source, map[string]string{"FACTORY_BRIDGE_PROTOCOL": ""}, "bash", filepath.Join(source, "scripts/hooks/copy-manifest-check.sh"), source)
		Expect(hook.status).To(BeZero(), "%+v", hook)
		Expect(hook.stdout).To(Equal(result.stdout))
		Expect(hook.stderr).To(BeEmpty())
	})
	// per docs/adr/0111-source-launcher-and-adopter-boundaries.md:55
	// per docs/adr/0111-source-launcher-and-adopter-boundaries.md:72
	It("retains adopter inventory behavior for a genuinely healthy installed Go application", func() {
		image := handoffBuildImage()
		f, driver := cutoverNativeFixture(image)
		target := filepath.Join(f.cwd, "ordinary Go product")
		writeFixture(filepath.Join(target, "go.mod"), []byte("module example.com/ordinary-product\n\ngo 1.27.3\n"), 0644)
		writeFixture(filepath.Join(target, "cmd/api/main.go"), []byte("package main\nfunc main() {}\n"), 0644)
		imageGit(target, "init", "--quiet")
		imageGit(target, "add", "--all")
		imageGit(target, "commit", "--quiet", "-m", "ordinary product before factory adoption")
		installed := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", target, "--go-candidate", handoffVersion, "--pack", "none")
		Expect(installed.status).To(BeZero(), "%+v", installed)
		healthy := handoffNativeProcess(target, f.environment, "", filepath.Join(target, "factory"), "help")
		Expect(healthy.status).To(BeZero(), "%+v", healthy)
		Expect(healthy.stderr).To(BeEmpty())
		Expect(healthy.stdout).To(ContainSubstring("Usage:"))
		repository, err := filepath.Abs("..")
		Expect(err).NotTo(HaveOccurred())
		f.environment["FACTORY_RUNTIME_BINARY"] = filepath.Join(target, ".factory/bin/factory-runtime")
		result := handoffNativeProcess(target, f.environment, "", "bash", filepath.Join(repository, "scripts/hooks/copy-manifest-check.sh"), target)
		Expect(result.status).To(BeZero(), "a product go.mod cannot select factory source inventory: %+v", result)
		Expect(result.stdout).To(Equal("copy-manifest-check: every unconditional install-copy target is git-tracked\n"))
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
	// per docs/adr/0111-source-launcher-and-adopter-boundaries.md:55
	// per docs/adr/0111-source-launcher-and-adopter-boundaries.md:59
	DescribeTable("does not bypass a damaged canonical inventory when its module declaration is damaged", func(kind string) {
		source := cutoverInventoryFixture()
		module := filepath.Join(source, "go.mod")
		switch kind {
		case "missing":
			Expect(os.Remove(module)).To(Succeed())
		case "untracked":
			imageGit(source, "rm", "--cached", "--quiet", "go.mod")
		case "symlink":
			Expect(os.Remove(module)).To(Succeed())
			outside := filepath.Join(filepath.Dir(source), "outside module")
			writeFixture(outside, []byte("module example.com/not-the-factory\n"), 0600)
			Expect(os.Symlink(outside, module)).To(Succeed())
		case "changed identity":
			writeFixture(module, []byte("module example.com/not-the-factory\n"), 0600)
		}
		Expect(os.Remove(filepath.Join(source, "templates/metrics.html"))).To(Succeed())
		result := imageProcess(source, map[string]string{"FACTORY_BRIDGE_PROTOCOL": ""}, "bash", filepath.Join(source, "scripts/hooks/copy-manifest-check.sh"), source)
		Expect(result.status).To(Equal(1), "%+v", result)
		Expect(result.stdout + result.stderr).To(And(ContainSubstring("COPY-MANIFEST FAIL:"), ContainSubstring("templates/metrics.html")))
		Expect(result.stdout).NotTo(ContainSubstring("every unconditional install-copy target"), "native inventory refusal cannot select a successful legacy scan")
	}, Entry("missing module", "missing"), Entry("untracked module", "untracked"), Entry("symlink module", "symlink"), Entry("product module identity in factory source", "changed identity"))
})
