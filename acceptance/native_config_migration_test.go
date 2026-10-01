package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func configMigrationFixture() (string, string, []string) {
	GinkgoHelper()
	root, cwd, environment := reportFixture()
	writeFixture(filepath.Join(cwd, "factory.yaml"), []byte("cost_profile: mine\nreview_model:\n"), 0600)
	writeFixture(filepath.Join(cwd, "factory.config"), []byte("COST_PROFILE=legacy\nREVIEW_MODEL=chosen\nEXTRA=value\n"), 0600)
	return root, cwd, environment
}

func configMigrationRun(root, cwd string, environment []string, args ...string) cliResult {
	return reportProcess(cwd, environment, filepath.Join(root, "factory"), append([]string{"migrate-config"}, args...))
}

func configMigrationLegacy(root, cwd string, environment []string, args ...string) cliResult {
	GinkgoHelper()
	for _, path := range []string{"scripts/factory-migrate-config.sh", "scripts/lib/config.sh"} {
		command := exec.Command("git", "show", "c10739fa95ff2123946a43388153ae931b593804:"+path) // #nosec G204 -- frozen local revision and fixed fixture assets.
		command.Dir = ".."
		data, err := command.Output()
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(root, path), data, 0700)
	}
	return reportProcess(cwd, environment, "/bin/bash", append([]string{filepath.Join(root, "scripts/factory-migrate-config.sh")}, args...))
}

var _ = Describe("Native Go config migration core", func() {
	// per docs/adr/0090-go-native-config-migration.md:47
	It("previews additions without the legacy migration script and preserves files", func() {
		root, cwd, environment := configMigrationFixture()
		before := nativeInitArtifacts(cwd)
		out := configMigrationRun(root, cwd, environment, "--dry-run")
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("would add: review_model: \"chosen\""))
		Expect(out.stdout).To(ContainSubstring("keeping yours: cost_profile"))
		Expect(nativeInitArtifacts(cwd)).To(Equal(before))
	})
	// per docs/adr/0090-go-native-config-migration.md:42
	It("migrates blank and missing keys while preserving nonempty YAML and the legacy inode", func() {
		root, cwd, environment := configMigrationFixture()
		legacy := filepath.Join(cwd, "factory.config")
		before, err := os.Stat(legacy)
		Expect(err).NotTo(HaveOccurred())
		out := configMigrationRun(root, cwd, environment)
		Expect(out.status).To(BeZero(), "%+v", out)
		data, err := os.ReadFile(filepath.Join(cwd, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("cost_profile: mine"))
		Expect(string(data)).To(ContainSubstring("review_model: \"chosen\""))
		Expect(string(data)).To(ContainSubstring("config_migrated: \"yes\""))
		after, err := os.Stat(legacy + ".migrated")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		_, err = os.Stat(legacy)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0090-go-native-config-migration.md:23
	It("succeeds without changes when there is no legacy configuration", func() {
		root, cwd, environment := configMigrationFixture()
		Expect(os.Remove(filepath.Join(cwd, "factory.config"))).To(Succeed())
		before := nativeInitArtifacts(cwd)
		out := configMigrationRun(root, cwd, environment)
		Expect(out).To(Equal(cliResult{"factory migrate-config: no factory.config — nothing to migrate.\n", "", 0}))
		Expect(nativeInitArtifacts(cwd)).To(Equal(before))
	})
})

var _ = Describe("Native Go config migration frozen parity", func() {
	// per docs/adr/0090-go-native-config-migration.md:62
	DescribeTable("matches the full frozen recovery-refusal diagnostic in the same project", func(kind string) {
		root, cwd, environment := configMigrationFixture()
		path := filepath.Join(cwd, "factory.config.migrated")
		switch kind {
		case "file":
			writeFixture(path, []byte("RECOVERY KEEP"), 0600)
		case "directory":
			Expect(os.Mkdir(path, 0700)).To(Succeed())
			writeFixture(filepath.Join(path, "keep"), []byte("RECOVERY KEEP"), 0600)
		case "dangling symlink":
			Expect(os.Symlink("missing-recovery", path)).To(Succeed())
		}
		before := nativeInitArtifacts(cwd)
		expected := configMigrationLegacy(root, cwd, environment)
		Expect(expected.status).To(Equal(1), "%+v", expected)
		Expect(nativeInitArtifacts(cwd)).To(Equal(before))
		Expect(os.Remove(filepath.Join(root, "scripts/factory-migrate-config.sh"))).To(Succeed())
		actual := configMigrationRun(root, cwd, environment)
		Expect(actual).To(Equal(expected), "compare every diagnostic byte, including the leading newline and project path")
		Expect(nativeInitArtifacts(cwd)).To(Equal(before))
	}, Entry("file", "file"), Entry("directory", "directory"), Entry("dangling symlink", "dangling symlink"))

	// per docs/adr/0090-go-native-config-migration.md:26
	DescribeTable("accepts same-inode overrides from a nested Git cwd and publishes root YAML", func(kind string) {
		root, cwd, environment := configMigrationFixture()
		doctorGit(cwd, environment, "init", "-q")
		override := "./factory.yaml"
		switch kind {
		case "absolute":
			override = filepath.Join(cwd, "factory.yaml")
		case "alias":
			Expect(os.Symlink("factory.yaml", filepath.Join(cwd, "selected.yaml"))).To(Succeed())
			override = "selected.yaml"
		case "lexical":
			Expect(os.MkdirAll(filepath.Join(cwd, "real/child"), 0700)).To(Succeed())
			Expect(os.Symlink(filepath.Join(cwd, "real/child"), filepath.Join(cwd, "link"))).To(Succeed())
			override = "link/../../factory.yaml"
		}
		environment = append(environment, "FACTORY_CONFIG="+override)
		nested := filepath.Join(cwd, "nested")
		Expect(os.Mkdir(nested, 0700)).To(Succeed())
		out := configMigrationRun(root, nested, environment)
		Expect(out.status).To(BeZero(), "%+v", out)
		data, err := os.ReadFile(filepath.Join(cwd, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("review_model: \"chosen\""))
		Expect(string(data)).To(ContainSubstring("config_migrated: \"yes\""))
		_, err = os.Stat(filepath.Join(cwd, "factory.config.migrated"))
		Expect(err).NotTo(HaveOccurred())
		if kind == "alias" {
			info, err := os.Lstat(filepath.Join(cwd, "selected.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode() & os.ModeSymlink).NotTo(BeZero())
		}
		entries, err := os.ReadDir(nested)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	}, Entry("relative path", "relative"), Entry("absolute path", "absolute"), Entry("symlink alias selects same inode", "alias"), Entry("lexical parent components", "lexical"))

	// per docs/adr/0090-go-native-config-migration.md:18
	DescribeTable("rejects unsupported operands with the frozen diagnostic", func(arg string) {
		root, cwd, environment := configMigrationFixture()
		before := nativeInitArtifacts(cwd)
		expected := configMigrationLegacy(root, cwd, environment, arg)
		Expect(expected.status).To(Equal(2))
		Expect(os.Remove(filepath.Join(root, "scripts/factory-migrate-config.sh"))).To(Succeed())
		out := configMigrationRun(root, cwd, environment, arg)
		Expect(out).To(Equal(expected))
		Expect(nativeInitArtifacts(cwd)).To(Equal(before))
	}, Entry("help is not an option", "--help"), Entry("positional operand", "apply"), Entry("empty operand", ""), Entry("attached dry-run value", "--dry-run=yes"))

	// per docs/adr/0090-go-native-config-migration.md:88
	DescribeTable("matches ordinary migration output and file bytes", func(kind string, dry bool) {
		root, cwd, environment := configMigrationFixture()
		oracleRoot, oracleCWD, oracleEnv := configMigrationFixture()
		yaml := "cost_profile: mine\nreview_model:\n"
		legacy := "COST_PROFILE=legacy\nREVIEW_MODEL=chosen\nEXTRA=value\n"
		switch kind {
		case "grammar":
			legacy = "# comment\nnot a setting\nexport NOPE=no\nBAD KEY=no\n=emptykey\n9KEY=digit\nSINGLE='a # b' ignored\nDOUBLE=\"quoted # value\" ignored\nSPACE=  leading value   # comment\nHASH=inside#hash\nLITERAL=$(touch MIGRATION_INJECTION)\nBACKTICK=`touch MIGRATION_INJECTION`\n"
		case "duplicates":
			yaml = "same:\nsame: second\nother: mine\nother: second\n"
			legacy = "SAME=first\nSAME=second\nOTHER=legacy\nEMPTY=\nEMPTY=later\n"
		case "unterminated":
			yaml = "cost_profile: mine"
			legacy = "EXTRA=tail"
		case "empty":
			legacy = ""
		case "unmatched single quote":
			legacy = "SINGLE='value   # retained comment   \n"
		case "unmatched double quote":
			legacy = "DOUBLE=\"value   # retained comment   \n"
		case "YAML migration marker":
			yaml += "config_migrated: mine\n"
			legacy += "CONFIG_MIGRATED=legacy\n"
		case "legacy migration marker":
			legacy += "CONFIG_MIGRATED=legacy\n"
		case "unquoted blank tail then append":
			yaml = "blank:"
			legacy = "OTHER=appended\nBLANK=later\n"
		case "quoted blank tail then append":
			yaml = "blank: \"\""
			legacy = "OTHER=appended\nBLANK=later\n"
		case "tail prefix creates later key":
			yaml = "n"
			legacy = "EW=joined\nNEW=later\n"
		case "tail rewrite before append":
			yaml = "blank:"
			legacy = "BLANK=filled\nOTHER=appended\n"
		case "completion replaces merged tail":
			yaml = "config_migrated: old"
			legacy = "B=appended\n"
		case "empty legacy marker tail":
			yaml = "config_migrated:"
			legacy = ""
		}
		for _, dir := range []string{cwd, oracleCWD} {
			writeFixture(filepath.Join(dir, "factory.yaml"), []byte(yaml), 0600)
			writeFixture(filepath.Join(dir, "factory.config"), []byte(legacy), 0600)
		}
		var args []string
		if dry {
			args = []string{"--dry-run", "--dry-run"}
		}
		expected := configMigrationLegacy(oracleRoot, oracleCWD, oracleEnv, args...)
		Expect(expected.status).To(BeZero(), "%+v", expected)
		actual := configMigrationRun(root, cwd, environment, args...)
		Expect(actual).To(Equal(expected))
		Expect(nativeInitArtifacts(cwd)).To(Equal(nativeInitArtifacts(oracleCWD)))
		_, err := os.Stat(filepath.Join(cwd, "MIGRATION_INJECTION"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		if !dry {
			before := nativeInitArtifacts(cwd)
			again := configMigrationRun(root, cwd, environment)
			Expect(again.status).To(BeZero())
			Expect(again.stdout).To(ContainSubstring("nothing to migrate"))
			Expect(nativeInitArtifacts(cwd)).To(Equal(before))
		}
	}, Entry("normal apply", "normal", false), Entry("normal dry-run repeated flag", "normal", true), Entry("grammar apply", "grammar", false), Entry("grammar dry-run", "grammar", true), Entry("evolving duplicate apply", "duplicates", false), Entry("unchanged duplicate dry-run", "duplicates", true), Entry("unterminated apply", "unterminated", false), Entry("unterminated dry-run", "unterminated", true), Entry("empty legacy applies marker", "empty", false), Entry("unmatched single quote apply", "unmatched single quote", false), Entry("unmatched single quote preview", "unmatched single quote", true), Entry("unmatched double quote apply", "unmatched double quote", false), Entry("unmatched double quote preview", "unmatched double quote", true), Entry("preset YAML migration marker apply", "YAML migration marker", false), Entry("preset YAML migration marker preview", "YAML migration marker", true), Entry("preset legacy migration marker apply", "legacy migration marker", false), Entry("preset legacy migration marker preview", "legacy migration marker", true), Entry("unquoted blank tail append apply", "unquoted blank tail then append", false), Entry("unquoted blank tail append preview", "unquoted blank tail then append", true), Entry("quoted blank tail append apply", "quoted blank tail then append", false), Entry("quoted blank tail append preview", "quoted blank tail then append", true), Entry("tail prefix creates key apply", "tail prefix creates later key", false), Entry("tail prefix creates key preview", "tail prefix creates later key", true), Entry("tail rewrite before append apply", "tail rewrite before append", false), Entry("tail rewrite before append preview", "tail rewrite before append", true), Entry("completion replaces merged tail apply", "completion replaces merged tail", false), Entry("completion replaces merged tail preview", "completion replaces merged tail", true), Entry("empty legacy marker tail apply", "empty legacy marker tail", false), Entry("empty legacy marker tail preview", "empty legacy marker tail", true))
})
