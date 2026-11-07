package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The executable input is the public guide, not a second maintained hook body.
// per docs/adr/0142-installed-full-check-hook-setup.md:20
func installedCheckDocumentedHook() []byte {
	GinkgoHelper()
	guide, err := os.ReadFile("../docs/HOOKS.md")
	Expect(err).NotTo(HaveOccurred())
	Expect(string(guide)).To(ContainSubstring("## Full pre-push checks"))
	_, tail, found := strings.Cut(string(guide), "<!-- factory-full-check-hook -->")
	Expect(found).To(BeTrue(), "the guide must identify its executable full-check hook body")
	_, tail, found = strings.Cut(tail, "```sh\n")
	Expect(found).To(BeTrue())
	body, _, found := strings.Cut(tail, "```")
	Expect(found).To(BeTrue())
	Expect(body).To(Equal("#!/bin/sh\nexec ./factory check \"$@\"\n"))
	return []byte(body)
}

// Git resolves the destination; fixture creation follows the documented absent-path procedure.
// per docs/adr/0142-installed-full-check-hook-setup.md:34
func installedCheckHookPath(root string) string {
	GinkgoHelper()
	path := orchestrationGit(root, "rev-parse", "--git-path", "hooks/pre-push")
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	_, err := os.Lstat(path)
	Expect(os.IsNotExist(err)).To(BeTrue(), "the manual setup fixture must not overwrite an existing hook: %s", path)
	return path
}

// Keep the actual selected launcher, runtime and policy gates; only application Make work is local and bounded.
// per docs/adr/0142-installed-full-check-hook-setup.md:66
func installedCheckPushFixture(installed installationFixtureData) (string, map[string]string) {
	GinkgoHelper()
	root := installed.root
	env := orchestrationInstalledEnvironment(root)
	env["GIT_CONFIG_NOSYSTEM"], env["GIT_CONFIG_GLOBAL"] = "1", os.DevNull
	env["REPO_ROOT"] = ""
	tools := strings.Split(env["PATH"], string(os.PathListSeparator))[0]
	writeFixture(filepath.Join(tools, "go"), []byte("#!/bin/sh\nprintf called > \"$ORCHESTRATION_LEGACY_MARKER\"\nexit 97\n"), 0700)
	writeFixture(filepath.Join(tools, "make"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$ORCHESTRATION_MAKE_ARGUMENTS\"\nif [ \"$1\" = check ]; then exit \"${INSTALLED_CHECK_FAIL:-0}\"; fi\n"), 0700)
	writeFixture(filepath.Join(root, "factory.yaml"), []byte("language_packs:\nreview_lane: off\nlocal_hooks:\n"), 0600)
	orchestrationGit(root, "config", "user.email", "acceptance@example.invalid")
	orchestrationGit(root, "config", "user.name", "Acceptance")
	orchestrationGit(root, "config", "core.hooksPath", os.DevNull)
	for _, path := range []string{"factory", "factory.yaml", "scripts", "runtime", ".factory-version"} {
		if _, err := os.Lstat(filepath.Join(root, path)); err == nil {
			orchestrationGit(root, "add", "--force", "--", path)
		}
	}
	orchestrationGit(root, "commit", "-qm", "test: installed full-check fixture")
	orchestrationGit(root, "update-ref", "refs/remotes/origin/main", "HEAD")
	orchestrationGit(root, "config", "--unset", "core.hooksPath")
	remote := filepath.Join(filepath.Dir(root), "bare remote ' $(literal)")
	Expect(os.Mkdir(remote, 0700)).To(Succeed())
	orchestrationGit(remote, "init", "--bare", "-q")
	orchestrationGit(root, "remote", "add", "fixture", remote)
	return remote, env
}

// Native check deliberately ignores operands; this independent transport witness checks the documented shell's exact bytes.
// per docs/adr/0142-installed-full-check-hook-setup.md:27
var _ = Describe("Whole installation upgrade full-check hook setup", Ordered, ContinueOnFailure, func() {
	var binary []byte
	var prepared installationFixtureData
	BeforeAll(func() {
		installedCheckDocumentedHook()
		binary, prepared = orchestrationInstallationPrepared()
	})
	fixture := func() installationFixtureData {
		GinkgoHelper()
		installed := installationCloneLegacyFixture(binary, prepared)
		installationApply(installed, "documented-full-check-hook")
		return installed
	}

	// per docs/adr/0142-installed-full-check-hook-setup.md:27
	It("preserves literal remote operands, stdin and status from the documented hook body", func() {
		body := installedCheckDocumentedHook()
		root, err := os.MkdirTemp("", "factory-installed-hook-transport-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, root)
		root, err = filepath.EvalSymlinks(root)
		Expect(err).NotTo(HaveOccurred())
		hook := filepath.Join(root, "pre-push")
		writeFixture(hook, body, 0700)
		writeFixture(filepath.Join(root, "factory"), []byte("#!/bin/sh\nprintf '%s\\000' \"$PWD\" \"$@\" > \"$HOOK_ARGUMENTS\"\n/bin/cat > \"$HOOK_INPUT\"\nprintf hook-stdout\nprintf hook-stderr >&2\nexit 37\n"), 0700)
		env := map[string]string{"HOOK_ARGUMENTS": filepath.Join(root, "args"), "HOOK_INPUT": filepath.Join(root, "input")}
		input := "refs/heads/topic abc refs/heads/topic def\nNUL:\x00tail\n"
		args := []string{"remote ' $(literal)", "../bare remote\n*"}
		out := handoffNativeExactProcess(root, env, input, hook, args...)
		Expect(out).To(Equal(cliResult{"hook-stdout", "hook-stderr", 37}))
		Expect(os.ReadFile(env["HOOK_ARGUMENTS"])).To(Equal([]byte(strings.Join(append([]string{root, "check"}, append(args, "")...), "\x00"))))
		Expect(os.ReadFile(env["HOOK_INPUT"])).To(Equal([]byte(input)))
	})

	// per docs/adr/0142-installed-full-check-hook-setup.md:66
	It("runs installed full checks for real default and custom-path pushes and blocks failing updates", func() {
		body := installedCheckDocumentedHook()
		installed := fixture()
		remote, env := installedCheckPushFixture(installed)
		for _, custom := range []bool{false, true} {
			label := "default"
			if custom {
				label = "custom"
				orchestrationGit(installed.root, "config", "core.hooksPath", "user hooks")
			}
			hook := installedCheckHookPath(installed.root)
			writeFixture(hook, body, 0750)
			hookBefore, err := os.Lstat(hook)
			Expect(err).NotTo(HaveOccurred())
			configBefore, err := os.ReadFile(filepath.Join(installed.root, ".git/config"))
			Expect(err).NotTo(HaveOccurred())
			for _, mode := range []string{"main", "failure", "healthy"} {
				Expect(os.WriteFile(env["ORCHESTRATION_MAKE_ARGUMENTS"], nil, 0600)).To(Succeed())
				env["INSTALLED_CHECK_FAIL"] = "0"
				branch := label + "-" + mode
				if mode == "main" {
					branch = "main"
				}
				if mode == "failure" {
					env["INSTALLED_CHECK_FAIL"] = "23"
				}
				out := handoffNativeExactProcess(installed.root, env, "", "git", "push", "fixture", "HEAD:refs/heads/"+branch)
				AddReportEntry("installed-check."+label+"."+mode, out, ReportEntryVisibilityNever)
				trace, err := os.ReadFile(env["ORCHESTRATION_MAKE_ARGUMENTS"])
				Expect(err).NotTo(HaveOccurred())
				ref := imageProcess(remote, nil, "git", "rev-parse", "--verify", "refs/heads/"+branch)
				if mode == "healthy" {
					Expect(out.status).To(BeZero(), "%+v", out)
					Expect(ref.status).To(BeZero())
					Expect(strings.TrimSpace(ref.stdout)).To(Equal(orchestrationGit(installed.root, "rev-parse", "HEAD")))
				} else {
					Expect(out.status).NotTo(BeZero(), "%+v", out)
					Expect(ref.status).NotTo(BeZero(), "rejected push must not update the remote")
				}
				if mode == "main" {
					Expect(out.stdout + out.stderr).To(ContainSubstring("direct push to main"))
					Expect(trace).To(BeEmpty(), "direct-main policy must stop before application checks")
				} else {
					Expect(string(trace)).To(Equal("check\ncheck-drift\n"))
				}
			}
			after, err := os.Lstat(hook)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.SameFile(hookBefore, after)).To(BeTrue())
			Expect(after.Mode()).To(Equal(hookBefore.Mode()))
			Expect(os.ReadFile(hook)).To(Equal(body))
			Expect(os.ReadFile(filepath.Join(installed.root, ".git/config"))).To(Equal(configBefore))
		}
		_, err := os.Lstat(env["ORCHESTRATION_LEGACY_MARKER"])
		Expect(os.IsNotExist(err)).To(BeTrue(), "installed checks must not invoke compiler, Python or poisoned source")
	})

	// per docs/adr/0142-installed-full-check-hook-setup.md:34
	// per docs/adr/0142-installed-full-check-hook-setup.md:73
	// per docs/adr/0142-installed-full-check-hook-setup.md:102
	It("uses Git's effective shared hook path and the linked worktree's installed runtime", func() {
		body := installedCheckDocumentedHook()
		installed := installationCloneLegacyFixture(binary, prepared)
		primary := installed.root
		imageGit(primary, "add", "--force", "--all")
		imageGit(primary, "commit", "--quiet", "-m", "test: complete legacy worktree input")
		linked := filepath.Join(filepath.Dir(primary), "linked worktree")
		orchestrationGit(primary, "worktree", "add", "--detach", linked, "HEAD")
		installed.root = linked
		installed.before = installationSnapshot(linked)
		installationApply(installed, "linked-worktree-full-check-hook")
		remote, env := installedCheckPushFixture(installed)
		info, err := os.Lstat(filepath.Join(linked, ".git"))
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().IsRegular()).To(BeTrue(), "linked worktree must exercise a .git file")
		hook := installedCheckHookPath(linked)
		Expect(filepath.Dir(hook)).NotTo(Equal(filepath.Join(linked, ".git/hooks")))
		writeFixture(hook, body, 0750)
		env["FACTORY_CONFIG"] = filepath.Join(linked, "factory.yaml")
		writeFixture(filepath.Join(primary, "factory"), []byte("#!/bin/sh\nprintf wrong-owner > \"$ORCHESTRATION_LEGACY_MARKER\"\nexit 96\n"), 0700)
		out := handoffNativeExactProcess(linked, env, "", "git", "push", "fixture", "HEAD:refs/heads/linked-topic")
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(orchestrationGit(remote, "rev-parse", "refs/heads/linked-topic")).To(Equal(orchestrationGit(linked, "rev-parse", "HEAD")))
		Expect(os.ReadFile(env["ORCHESTRATION_MAKE_ARGUMENTS"])).To(Equal([]byte("check\ncheck-drift\n")))
		_, err = os.Lstat(env["ORCHESTRATION_LEGACY_MARKER"])
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})
