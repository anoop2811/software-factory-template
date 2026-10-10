package acceptance_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The fixture declares invocation paths independently; it never reads the
// checker's arrays to decide what should exist.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:395
var installationHookInvocationPaths = []string{
	"scripts/selftest/run.sh",
	"scripts/hooks/test-edit-denial.sh",
	"scripts/hooks/shared-script-enforcement.sh",
	"scripts/hooks/commit-message-lint.sh",
	"scripts/hooks/loop-close-check.sh",
	"scripts/hooks/diff-aware-check.sh",
	"scripts/hooks/decision-log-gate.sh",
	"scripts/hooks/pending-lessons-push-block.sh",
	"scripts/hooks/direct-main-push-block.sh",
	"scripts/hooks/wiki-lint.sh",
	"scripts/hooks/workflow-lint.sh",
	"scripts/hooks/copy-manifest-check.sh",
	"scripts/hooks/gate-instrumentation-check.sh",
	"scripts/citation-lint.sh",
	"scripts/sync-claude.sh",
	"scripts/sync-codex.sh",
	"scripts/harness-structural-eval.sh",
	"scripts/golden-task-eval.sh",
	"scripts/prereq-check.sh",
	"scripts/pre-push-check.sh",
	".githooks/pre-push",
}

func installationHookFixture(local bool) string {
	GinkgoHelper()
	root, err := os.MkdirTemp("", "factory-installed-hook-modes-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, root)
	root, err = filepath.EvalSymlinks(root)
	Expect(err).NotTo(HaveOccurred())
	for _, path := range installationHookInvocationPaths {
		writeFixture(filepath.Join(root, filepath.FromSlash(path)), []byte("#!/bin/sh\nexit 0\n"), 0755)
	}
	// The gate and its sourceable collaborators are real production bytes. The
	// fixture executables are only presence/mode inputs and are never invoked.
	for _, source := range []struct {
		path string
		mode os.FileMode
	}{
		{"scripts/hooks/hook-existence-check.sh", 0755},
		{"scripts/lib/config.sh", 0755},
		{"scripts/lib/events.sh", 0644},
	} {
		body, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(source.path)))
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(root, filepath.FromSlash(source.path)), body, source.mode)
	}
	configuration := "local_hooks: \"\"\n"
	if local {
		configuration = "local_hooks: \"scripts/hooks/project-gate.sh --strict\"\n"
		writeFixture(filepath.Join(root, "scripts/hooks/project-gate.sh"), []byte("#!/bin/sh\nexit 0\n"), 0755)
	}
	writeFixture(filepath.Join(root, "factory.yaml"), []byte(configuration), 0644)
	imageGit(root, "init", "--quiet")
	imageGit(root, "add", "--force", "--all")
	return root
}

func installationHookGate(root string) cliResult {
	GinkgoHelper()
	return imageProcess(root, map[string]string{
		"FACTORY_CONFIG":    filepath.Join(root, "factory.yaml"),
		"FACTORY_EVENT_LOG": filepath.Join(root, "fixture-events.log"),
	}, "bash", filepath.Join(root, "scripts/hooks/hook-existence-check.sh"))
}

var _ = Describe("Installed hook-existence library and invocation modes", func() {
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:392
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:396
	It("accepts the unchanged readable tracked sourceable config library at mode 0644", func() {
		root := installationHookFixture(true)
		healthyLegacy := installationHookGate(root)
		Expect(healthyLegacy.status).To(Equal(0), "%+v", healthyLegacy)
		path := filepath.Join(root, "scripts/lib/config.sh")
		original, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Chmod(path, 0644)).To(Succeed())
		imageGit(root, "add", "--", "scripts/lib/config.sh")
		out := installationHookGate(root)
		Expect(out.status).To(Equal(0), "sourceable installed library requires read permission, not an execute bit: %+v", out)
		Expect(out.stderr).To(BeEmpty())
		info, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0644)))
		body, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(body).To(Equal(original))
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:394
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:395
	DescribeTable("preserves invocation safety beside a genuinely healthy tracked fixture", func(local bool, change string) {
		root := installationHookFixture(local)
		healthy := installationHookGate(root)
		Expect(healthy.status).To(Equal(0), "%+v", healthy)
		path := "scripts/hooks/test-edit-denial.sh"
		if local {
			path = "scripts/hooks/project-gate.sh"
		}
		absolute := filepath.Join(root, filepath.FromSlash(path))
		switch change {
		case "missing":
			Expect(os.Remove(absolute)).To(Succeed())
		case "non-executable":
			Expect(os.Chmod(absolute, 0644)).To(Succeed())
		case "untracked":
			imageGit(root, "rm", "--cached", "--", path)
		case "directory":
			Expect(os.Remove(absolute)).To(Succeed())
			Expect(os.Mkdir(absolute, 0755)).To(Succeed())
		case "unreadable":
			if os.Geteuid() == 0 {
				Skip("native owner permission denial requires a non-root evaluator")
			}
			Expect(os.Chmod(absolute, 0111)).To(Succeed())
			probe := imageProcess(root, nil, "bash", "-c", `test -r "$1"`, "native-read-permission", absolute)
			Expect(probe.status).To(Equal(1), "negative sibling must actually be unreadable to this evaluator")
		}
		out := installationHookGate(root)
		Expect(out.status).To(Equal(1), "unsafe invoked hook was accepted: %+v", out)
		Expect(out.stdout + out.stderr).To(ContainSubstring(path))
	},
		Entry("missing shipped hook", false, "missing"),
		Entry("non-executable shipped hook", false, "non-executable"),
		Entry("untracked shipped hook", false, "untracked"),
		Entry("non-regular shipped hook", false, "directory"),
		Entry("unreadable executable shipped hook", false, "unreadable"),
		Entry("missing configured local hook", true, "missing"),
		Entry("non-executable configured local hook", true, "non-executable"),
		Entry("untracked configured local hook", true, "untracked"),
		Entry("non-regular configured local hook", true, "directory"),
		Entry("unreadable executable configured local hook", true, "unreadable"),
	)

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:393
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:394
	It("requires sourceable config tracking even when its regular readable bytes are present", func() {
		root := installationHookFixture(false)
		healthy := installationHookGate(root)
		Expect(healthy.status).To(Equal(0), "%+v", healthy)
		imageGit(root, "rm", "--cached", "--", "scripts/lib/config.sh")
		out := installationHookGate(root)
		Expect(out.status).To(Equal(1), "%+v", out)
		Expect(out.stdout + out.stderr).To(ContainSubstring("scripts/lib/config.sh"))
	})
})
