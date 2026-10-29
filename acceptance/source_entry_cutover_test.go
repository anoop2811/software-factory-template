package acceptance_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// per docs/adr/0108-default-go-cutover.md:53
func sourceCutoverEnvironment() map[string]string {
	GinkgoHelper()
	compiler, err := exec.LookPath("go")
	Expect(err).NotTo(HaveOccurred())
	compiler, err = filepath.EvalSymlinks(compiler)
	Expect(err).NotTo(HaveOccurred())
	return map[string]string{
		"PATH":        filepath.Dir(compiler) + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GOTOOLCHAIN": "local", "GOPROXY": "off", "GOSUMDB": "off", "GOFLAGS": "", "FACTORY_BRIDGE_PROTOCOL": "",
	}
}

var _ = Describe("Go source entry cutover", func() {
	// per docs/adr/0108-default-go-cutover.md:53
	It("builds the native developer command surface and preserves its caller directory", func() {
		_, cwd := fixtureDirectories()
		repository, err := filepath.Abs("..")
		Expect(err).NotTo(HaveOccurred())
		program := filepath.Join(repository, "factory")
		environment := sourceCutoverEnvironment()
		help := handoffNativeProcess(cwd, environment, "", program, "help")
		Expect(help.status).To(Equal(0), "%+v", help)
		for _, command := range []string{"sync", "prereq", "eval", "review-batch"} {
			Expect(help.stdout).To(ContainSubstring("  " + command + " "))
		}
		literal := "literal $(not-code) source caller"
		writeFixture(filepath.Join(cwd, "factory.yaml"), []byte("project_name: source-wrapper\n"), 0600)
		writeFixture(filepath.Join(cwd, "factory.config"), []byte("WRAPPER_LITERAL='"+literal+"'\n"), 0600)
		before := installationSnapshot(cwd)
		migration := handoffNativeProcess(cwd, environment, "", program, "migrate-config", "--dry-run")
		Expect(migration.status).To(Equal(0), "%+v", migration)
		Expect(migration.stdout).To(ContainSubstring(literal))
		Expect(installationSnapshot(cwd)).To(Equal(before), "building in the source checkout cannot change the caller's dry-run semantics")
	})

	// per docs/adr/0108-default-go-cutover.md:86
	DescribeTable("keeps direct initializer and upgrader script paths on native public help", func(verb string) {
		_, cwd := fixtureDirectories()
		repository, err := filepath.Abs("..")
		Expect(err).NotTo(HaveOccurred())
		before := installationSnapshot(cwd)
		result := handoffNativeProcess(cwd, sourceCutoverEnvironment(), nativeInitAnswers("n"), filepath.Join(repository, "scripts", "factory-"+verb+".sh"), "--help")
		Expect(result.status).To(Equal(0), "%+v", result)
		Expect(result.stdout).To(ContainSubstring("Usage: factory " + verb))
		Expect(result.stderr).To(BeEmpty())
		Expect(installationSnapshot(cwd)).To(Equal(before))
	}, Entry("init", "init"), Entry("upgrade", "upgrade"))

	// per docs/adr/0108-default-go-cutover.md:86
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:207
	It("proves the fourteen source gates natively without invoking the legacy selftest controller", func() {
		root, cwd := fixture()
		repository, err := filepath.Abs("..")
		Expect(err).NotTo(HaveOccurred())
		for _, directory := range []string{"scripts", "eval/runners", ".githooks"} {
			base := filepath.Join(repository, directory)
			Expect(filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || (!strings.HasSuffix(path, ".sh") && directory != ".githooks") {
					return nil
				}
				relative, err := filepath.Rel(repository, path)
				if err != nil {
					return err
				}
				info, err := entry.Info()
				if err != nil {
					return err
				}
				body, err := os.ReadFile(path) // #nosec G703 -- fixed source directories in the trusted checkout; symlinks are excluded above.
				if err != nil {
					return err
				}
				writeFixture(filepath.Join(root, relative), body, info.Mode().Perm())
				return nil
			})).To(Succeed())
		}
		marker := filepath.Join(root, "legacy-selftest-executed")
		writeFixture(filepath.Join(root, "scripts/selftest/run.sh"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_SELFTEST_MARKER\"\nexit 91\n"), 0700)
		tools := filepath.Join(root, "proof tools")
		git, err := exec.LookPath("git")
		Expect(err).NotTo(HaveOccurred())
		git, err = filepath.EvalSymlinks(git)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.MkdirAll(tools, 0700)).To(Succeed())
		Expect(os.Symlink(git, filepath.Join(tools, "git"))).To(Succeed())
		for _, name := range []string{"go", "python", "python3"} {
			writeFixture(filepath.Join(tools, name), []byte("#!/bin/sh\nprintf called > \"$SOURCE_SELFTEST_MARKER\"\nexit 92\n"), 0700)
		}
		result := handoffNativeProcess(cwd, map[string]string{"PATH": tools + string(os.PathListSeparator) + "/usr/bin:/bin", "SOURCE_SELFTEST_MARKER": marker, "FACTORY_BRIDGE_PROTOCOL": ""}, "", filepath.Join(root, "factory"), "selftest")
		Expect(result.status).To(Equal(0), "%+v", result)
		Expect(result.stdout).To(ContainSubstring("selftest: 14 shipped gates proved, 0 failed"))
		Expect(strings.Count(result.stdout, "refused its violation and accepted its healthy control")).To(Equal(14))
		_, err = os.Lstat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue(), "native gate proof cannot execute the poisoned source controller or compiler/interpreter")
	})
})
