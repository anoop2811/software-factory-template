package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func discoveryFixture() string {
	GinkgoHelper()
	root, err := os.MkdirTemp("", "factory-discovery-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, root)
	root, err = filepath.EvalSymlinks(root)
	Expect(err).NotTo(HaveOccurred())
	files := map[string]string{"scripts/citation-lint.sh": "scripts/citation-lint.sh", "scripts/lib/config.sh": "scripts/lib/config.sh", "scripts/lib/events.sh": "scripts/lib/events.sh", "packs/java/hooks/junit5-only-check.sh": "scripts/hooks/junit5-only-check.sh", "packs/typescript/hooks/vitest-only-check.sh": "scripts/hooks/vitest-only-check.sh", "packs/go/hooks/ginkgo-only-check.sh": "scripts/hooks/ginkgo-only-check.sh"}
	for source, target := range files {
		data, readErr := os.ReadFile(filepath.Join("..", source))
		Expect(readErr).NotTo(HaveOccurred())
		writeFixture(filepath.Join(root, target), data, 0700)
	}
	writeFixture(filepath.Join(root, "factory.yaml"), []byte("docs_root: docs\ncitation_prefix: DISC_\n"), 0600)
	writeFixture(filepath.Join(root, "docs/DISC_SPEC.md"), []byte("current canonical line\n"), 0600)
	Expect(loopProcess(root, "git", []string{"init", "--quiet"}, nil).status).To(BeZero())
	return root
}
func discoveryGate(root, kind string) cliResult {
	GinkgoHelper()
	script := "scripts/citation-lint.sh"
	if kind != "citation" {
		script = "scripts/hooks/" + kind + "-only-check.sh"
	}
	return loopProcess(root, "bash", []string{filepath.Join(root, script)}, []string{"FACTORY_CONFIG=" + filepath.Join(root, "factory.yaml")})
}
func discoveryViolation(root, kind, path string) {
	GinkgoHelper()
	switch kind {
	case "citation":
		writeFixture(filepath.Join(root, path, "stale.sh"), []byte("# DISC_MISSING.md:999\n"), 0600)
	case "junit5":
		writeFixture(filepath.Join(root, path, "OldTest.java"), []byte("import org.junit.Test;\n"), 0600)
	case "vitest":
		writeFixture(filepath.Join(root, path, "old.test.ts"), []byte("import { test } from '@jest/globals';\n"), 0600)
	case "ginkgo":
		name := filepath.Join(path, "old_test.go")
		writeFixture(filepath.Join(root, name), []byte("package old\nimport \"testing\"\nfunc TestOld(t *testing.T) {}\n"), 0600)
		Expect(loopProcess(root, "git", []string{"add", "-f", "--", name}, nil).status).To(BeZero())
	}
}

var _ = Describe("Recovery discovery exclusion core", func() {
	// per docs/adr/0084-exclude-inert-recovery-from-discovery.md:24
	It("ignores archived stale citation sources in ignored recovery", func() {
		root := discoveryFixture()
		writeFixture(filepath.Join(root, ".gitignore"), []byte(".factory/backups/\n"), 0600)
		discoveryViolation(root, "citation", ".factory/backups/old")
		out := discoveryGate(root, "citation")
		Expect(out.status).To(BeZero(), "%+v", out)
	})
	// per docs/adr/0084-exclude-inert-recovery-from-discovery.md:42
	It("refuses an active citation whose only document is archived", func() {
		root := discoveryFixture()
		writeFixture(filepath.Join(root, "factory.yaml"), []byte("docs_root: .\ncitation_prefix: DISC_\n"), 0600)
		writeFixture(filepath.Join(root, "active.go"), []byte("// DISC_ONLY.md:1\n"), 0600)
		writeFixture(filepath.Join(root, ".factory/backups/old/DISC_ONLY.md"), []byte("archived canonical line\n"), 0600)
		out := discoveryGate(root, "citation")
		Expect(out.status).To(Equal(1), "%+v", out)
	})
	// per docs/adr/0084-exclude-inert-recovery-from-discovery.md:24
	DescribeTable("excludes archived dialect violations even when ignored or force-tracked", func(kind string) {
		root := discoveryFixture()
		writeFixture(filepath.Join(root, ".gitignore"), []byte(".factory/backups/\n"), 0600)
		discoveryViolation(root, kind, ".factory/backups/old")
		out := discoveryGate(root, kind)
		Expect(out.status).To(BeZero(), "%+v", out)
	}, Entry("Java JUnit4", "junit5"), Entry("TypeScript Jest", "vitest"), Entry("force tracked Go stdlib", "ginkgo"))
})

var _ = Describe("Recovery discovery active controls", func() {
	// per docs/adr/0084-exclude-inert-recovery-from-discovery.md:33
	DescribeTable("continues enforcing active and similar-looking directories alongside inert recovery", func(kind, path string) {
		root := discoveryFixture()
		discoveryViolation(root, kind, ".factory/backups/old")
		discoveryViolation(root, kind, path)
		out := discoveryGate(root, kind)
		Expect(out.status).To(Equal(1), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("FAIL"))
		Expect(out.stdout).NotTo(ContainSubstring(".factory/backups/old/"))
	},
		Entry("active citation", "citation", "active"), Entry("factory active citation", "citation", ".factory/active"), Entry("near-prefix citation", "citation", ".factory/backups-other"), Entry("nested citation", "citation", "app/.factory/backups"),
		Entry("active Java", "junit5", "active"), Entry("factory active Java", "junit5", ".factory/active"), Entry("near-prefix Java", "junit5", ".factory/backups-other"), Entry("nested Java", "junit5", "app/.factory/backups"),
		Entry("active TS", "vitest", "active"), Entry("factory active TS", "vitest", ".factory/active"), Entry("near-prefix TS", "vitest", ".factory/backups-other"), Entry("nested TS", "vitest", "app/.factory/backups"),
		Entry("active Go", "ginkgo", "active"), Entry("factory active Go", "ginkgo", ".factory/active"), Entry("near-prefix Go", "ginkgo", ".factory/backups-other"), Entry("nested Go", "ginkgo", "app/.factory/backups"))
	// per docs/adr/0084-exclude-inert-recovery-from-discovery.md:24
	DescribeTable("excludes unignored archived payloads without changing Git index membership", func(kind string) {
		root := discoveryFixture()
		discoveryViolation(root, kind, ".factory/backups/old")
		before := loopProcess(root, "git", []string{"ls-files", "--stage"}, nil)
		out := discoveryGate(root, kind)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(loopProcess(root, "git", []string{"ls-files", "--stage"}, nil)).To(Equal(before))
	}, Entry("citation", "citation"), Entry("Java", "junit5"), Entry("TS", "vitest"), Entry("Go", "ginkgo"))
	// per docs/adr/0084-exclude-inert-recovery-from-discovery.md:36
	DescribeTable("accepts valid active tests and citations while excluding old backup violations", func(kind string) {
		root := discoveryFixture()
		discoveryViolation(root, kind, ".factory/backups/old")
		switch kind {
		case "citation":
			writeFixture(filepath.Join(root, "active.go"), []byte("// DISC_SPEC.md:1\n"), 0600)
		case "junit5":
			writeFixture(filepath.Join(root, "CurrentTest.java"), []byte("import org.junit.jupiter.api.Test;\n"), 0600)
		case "vitest":
			writeFixture(filepath.Join(root, "current.test.ts"), []byte("import { test } from 'vitest';\n"), 0600)
		case "ginkgo":
			writeFixture(filepath.Join(root, "current_test.go"), []byte("package current\nimport \"testing\"\nfunc TestCurrent(t *testing.T) { RunSpecs(t, \"current\") }\n"), 0600)
			Expect(loopProcess(root, "git", []string{"add", "--", "current_test.go"}, nil).status).To(BeZero())
		}
		out := discoveryGate(root, kind)
		Expect(out.status).To(BeZero(), "%+v", out)
	}, Entry("citation", "citation"), Entry("Java", "junit5"), Entry("TS", "vitest"), Entry("Go bootstrap", "ginkgo"))
	// per docs/adr/0084-exclude-inert-recovery-from-discovery.md:53
	DescribeTable("prunes the reserved node before unreadable or blocking children", func(kind string) {
		root := discoveryFixture()
		backup := filepath.Join(root, ".factory/backups")
		Expect(os.MkdirAll(backup, 0700)).To(Succeed())
		Expect(syscall.Mkfifo(filepath.Join(backup, "blocking.sh"), 0600)).To(Succeed())
		Expect(os.Symlink("/PRIVATE_MISSING", filepath.Join(backup, "archived"))).To(Succeed())
		discoveryViolation(root, kind, ".factory/backups/unreadable")
		child := filepath.Join(backup, "unreadable")
		Expect(os.Chmod(child, 0000)).To(Succeed())
		DeferCleanup(os.Chmod, child, os.FileMode(0700))
		out := discoveryGate(root, kind)
		Expect(out.status).To(BeZero(), "%+v", out)
	}, Entry("citation", "citation"), Entry("Java", "junit5"), Entry("TS", "vitest"), Entry("Go", "ginkgo"))
})

func discoveryRelocate(root, kind string) string {
	GinkgoHelper()
	target := root + "-[literal*?]"
	if kind == "ancestor" {
		parent := root + "-parent"
		Expect(os.Mkdir(parent, 0700)).To(Succeed())
		DeferCleanup(os.RemoveAll, parent)
		target = filepath.Join(parent, "installation")
	}
	Expect(os.Rename(root, target)).To(Succeed())
	DeferCleanup(os.RemoveAll, target)
	return target
}

var _ = Describe("Recovery discovery citation target controls", func() {
	// per docs/adr/0084-exclude-inert-recovery-from-discovery.md:42
	DescribeTable("does not resolve active references from recovery through physical docs roots", func(kind string) {
		root := discoveryFixture()
		docsRoot := "."
		switch kind {
		case "absolute":
			docsRoot = root
		case "ancestor":
			root = discoveryRelocate(root, "ancestor")
			docsRoot = filepath.Dir(root)
		case "glob":
			root = discoveryRelocate(root, "glob")
			docsRoot = root
		case "alias":
			Expect(os.Symlink(root, filepath.Join(root, "doc-alias"))).To(Succeed())
			docsRoot = "doc-alias"
		}
		writeFixture(filepath.Join(root, "factory.yaml"), []byte("docs_root: "+docsRoot+"\ncitation_prefix: DISC_\n"), 0600)
		writeFixture(filepath.Join(root, "active.go"), []byte("// DISC_ONLY.md:1\n"), 0600)
		writeFixture(filepath.Join(root, ".factory/backups/old/DISC_ONLY.md"), []byte("archive\n"), 0600)
		out := discoveryGate(root, "citation")
		Expect(out.status).To(Equal(1), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("file not found"))
		writeFixture(filepath.Join(root, "docs/DISC_ONLY.md"), []byte("current\n"), 0600)
		out = discoveryGate(root, "citation")
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("OK DISC_ONLY.md:1"))
	}, Entry("relative dot", "dot"), Entry("absolute root", "absolute"), Entry("ancestor", "ancestor"), Entry("physical alias", "alias"), Entry("root glob characters", "glob"))
	// per docs/adr/0084-exclude-inert-recovery-from-discovery.md:46
	DescribeTable("refuses canonical document roots resolving inside recovery", func(kind string) {
		root := discoveryFixture()
		backup := filepath.Join(root, ".factory/backups/docs")
		Expect(os.MkdirAll(backup, 0700)).To(Succeed())
		writeFixture(filepath.Join(root, "active.go"), []byte("// DISC_ONLY.md:1\n"), 0600)
		writeFixture(filepath.Join(backup, "DISC_ONLY.md"), []byte("archive\n"), 0600)
		docsRoot := ".factory/backups/docs"
		if kind == "absolute" {
			docsRoot = backup
		}
		if kind == "symlink" {
			Expect(os.Symlink(backup, filepath.Join(root, "alias"))).To(Succeed())
			docsRoot = "alias"
		}
		writeFixture(filepath.Join(root, "factory.yaml"), []byte("docs_root: "+docsRoot+"\ncitation_prefix: DISC_\n"), 0600)
		out := discoveryGate(root, "citation")
		Expect(out.status).NotTo(BeZero(), "%+v", out)
		Expect(strings.ToLower(out.stdout + out.stderr)).To(ContainSubstring("recovery"))
		Expect(out.stdout).NotTo(ContainSubstring("OK DISC_ONLY"))
	}, Entry("relative subtree", "relative"), Entry("absolute subtree", "absolute"), Entry("symlink alias", "symlink"))
	// per docs/adr/0084-exclude-inert-recovery-from-discovery.md:33
	DescribeTable("allows current target documents in similar and nested directories", func(path string) {
		root := discoveryFixture()
		writeFixture(filepath.Join(root, "factory.yaml"), []byte("docs_root: .\ncitation_prefix: DISC_\n"), 0600)
		writeFixture(filepath.Join(root, "active.go"), []byte("// DISC_ONLY.md:1\n"), 0600)
		writeFixture(filepath.Join(root, path, "DISC_ONLY.md"), []byte("current\n"), 0600)
		out := discoveryGate(root, "citation")
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("OK DISC_ONLY.md:1"))
	}, Entry("active factory", ".factory/active"), Entry("near prefix", ".factory/backups-other"), Entry("nested factory", "application/.factory/backups"))
	// per docs/adr/0084-exclude-inert-recovery-from-discovery.md:51
	It("preserves citations supplied through PR_BODY source files", func() {
		root := discoveryFixture()
		body := filepath.Join(root, "description.txt")
		writeFixture(body, []byte("DISC_MISSING.md:999\n"), 0600)
		out := loopProcess(root, "bash", []string{filepath.Join(root, "scripts/citation-lint.sh")}, []string{"FACTORY_CONFIG=" + filepath.Join(root, "factory.yaml"), "PR_BODY=" + body})
		Expect(out.status).To(Equal(1), "%+v", out)
	})
	// per docs/adr/0084-exclude-inert-recovery-from-discovery.md:36
	It("preserves disabled citation checking even when docs_root names recovery", func() {
		root := discoveryFixture()
		writeFixture(filepath.Join(root, "factory.yaml"), []byte("docs_root: .factory/backups\ncitation_prefix: \n"), 0600)
		out := discoveryGate(root, "citation")
		Expect(out.status).To(BeZero())
		Expect(out.stdout).To(ContainSubstring("skipping"))
	})
})

var _ = Describe("Recovery discovery physical newline boundary", func() {
	// per docs/adr/0084-exclude-inert-recovery-from-discovery.md:89
	It("preserves the installation root newline when pruning ancestor document lookup", func() {
		root := discoveryFixture()
		parent := root + "-parent"
		Expect(os.Mkdir(parent, 0700)).To(Succeed())
		DeferCleanup(os.RemoveAll, parent)
		moved := filepath.Join(parent, "installation\n")
		Expect(os.Rename(root, moved)).To(Succeed())
		root = moved
		writeFixture(filepath.Join(root, "factory.yaml"), []byte("docs_root: "+parent+"\ncitation_prefix: DISC_\n"), 0600)
		writeFixture(filepath.Join(root, "active.go"), []byte("// DISC_ONLY.md:1\n"), 0600)
		writeFixture(filepath.Join(root, ".factory/backups/old/DISC_ONLY.md"), []byte("archived\n"), 0600)
		out := discoveryGate(root, "citation")
		Expect(out.status).To(Equal(1), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("file not found"))
	})
	// per docs/adr/0084-exclude-inert-recovery-from-discovery.md:89
	It("preserves a physical docs root newline reached through an ordinary configured alias", func() {
		root := discoveryFixture()
		physical := filepath.Join(root, "canonical\n")
		Expect(os.Mkdir(physical, 0700)).To(Succeed())
		Expect(os.Symlink(physical, filepath.Join(root, "docs-alias"))).To(Succeed())
		writeFixture(filepath.Join(root, "factory.yaml"), []byte("docs_root: docs-alias\ncitation_prefix: DISC_\n"), 0600)
		writeFixture(filepath.Join(root, "active.go"), []byte("// DISC_CURRENT.md:1\n"), 0600)
		writeFixture(filepath.Join(physical, "DISC_CURRENT.md"), []byte("current\n"), 0600)
		out := discoveryGate(root, "citation")
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("OK DISC_CURRENT.md:1"))
	})
})

var _ = Describe("Recovery discovery literal reserved node", func() {
	// per docs/adr/0084-exclude-inert-recovery-from-discovery.md:83
	It("does not redefine the excluded tree by following a reserved-node symlink", func() {
		root := discoveryFixture()
		active := filepath.Join(root, "canonical")
		Expect(os.Mkdir(active, 0700)).To(Succeed())
		Expect(os.Mkdir(filepath.Join(root, ".factory"), 0700)).To(Succeed())
		Expect(os.Symlink(active, filepath.Join(root, ".factory/backups"))).To(Succeed())
		writeFixture(filepath.Join(root, "factory.yaml"), []byte("docs_root: canonical\ncitation_prefix: DISC_\n"), 0600)
		writeFixture(filepath.Join(root, "active.go"), []byte("// DISC_CURRENT.md:1\n"), 0600)
		writeFixture(filepath.Join(active, "DISC_CURRENT.md"), []byte("current\n"), 0600)
		out := discoveryGate(root, "citation")
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("OK DISC_CURRENT.md:1"))
	})
	// per docs/adr/0084-exclude-inert-recovery-from-discovery.md:53
	It("accepts empty recovery without disabling any gate", func() {
		root := discoveryFixture()
		Expect(os.MkdirAll(filepath.Join(root, ".factory/backups"), 0700)).To(Succeed())
		for _, kind := range []string{"citation", "junit5", "vitest", "ginkgo"} {
			out := discoveryGate(root, kind)
			Expect(out.status).To(BeZero(), "%s: %+v", kind, out)
		}
	})
})
