package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func transitionInstallationSource() (string, string, []string) {
	GinkgoHelper()
	source, target, environment := nativeInitFixture()
	repo, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	for _, name := range []string{"factory-budget.sh", "factory-loop.sh", "factory-upgrade.sh", "factory-init.sh"} {
		data, err := os.ReadFile(filepath.Join(repo, "scripts", name))
		Expect(err).NotTo(HaveOccurred())
		if name == "factory-init.sh" {
			name = "factory-init-legacy.sh"
		}
		writeFixture(filepath.Join(source, "scripts", name), data, 0700)
	}
	entries, err := os.ReadDir(filepath.Join(repo, "scripts/lib"))
	Expect(err).NotTo(HaveOccurred())
	for _, entry := range entries {
		if entry.Type().IsRegular() && (strings.HasSuffix(entry.Name(), ".py") || strings.HasSuffix(entry.Name(), ".sh")) {
			data, err := os.ReadFile(filepath.Join(repo, "scripts/lib", entry.Name()))
			Expect(err).NotTo(HaveOccurred())
			writeFixture(filepath.Join(source, "scripts/lib", entry.Name()), data, 0600)
		}
	}
	return source, target, environment
}

var _ = Describe("Runtime transition source delivery", func() {
	// per docs/adr/0093-runtime-transition-guard.md:146
	// per docs/adr/0093-runtime-transition-guard.md:162
	DescribeTable("installs the actual Python shim and its controllers as an executable legacy exclusion boundary", func(route string) {
		source, target, environment := transitionInstallationSource()
		var out cliResult
		if route == "legacy-init" {
			out = nativeInitLegacy(source, target, nativeInitAnswers("y"), environment, []string{"--pack", "none"})
		} else {
			out = nativeInitRun(source, target, nativeInitAnswers("y"), environment, []string{"--pack", "none"})
		}
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stderr).NotTo(ContainSubstring("LEGACY_INIT_CALLED"))
		configPath := filepath.Join(target, "factory.yaml")
		config, err := os.ReadFile(configPath)
		Expect(err).NotTo(HaveOccurred())
		if route == "legacy-upgrade" {
			Expect(os.Remove(filepath.Join(target, "scripts/lib/runtime_transition.py"))).To(Succeed())
			out = loopProcess(target, "bash", []string{filepath.Join(source, "scripts/factory-upgrade.sh"), "--source", source, "--ref", "transition-fixture"}, append(environment, "FACTORY_UPGRADE_ACTIVE=1"))
			Expect(out.status).To(BeZero(), "%+v", out)
			after, err := os.ReadFile(configPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(after).To(Equal(config), "source upgrade preserves adopter policy")
		}
		for _, name := range []string{"runtime_transition.py", "budget.py", "budget_adapters.py", "loop.py"} {
			expected, err := os.ReadFile(filepath.Join(source, "scripts/lib", name))
			Expect(err).NotTo(HaveOccurred())
			installed, err := os.ReadFile(filepath.Join(target, "scripts/lib", name))
			Expect(err).NotTo(HaveOccurred())
			Expect(installed).To(Equal(expected))
		}
		Expect(string(config)).To(ContainSubstring("budget_enabled: false"))
		writeFixture(configPath, []byte(strings.Replace(string(config), "budget_enabled: false", "budget_enabled: true", 1)), 0600)
		writeFixture(filepath.Join(target, "prompt.txt"), []byte("fixture prompt"), 0600)
		sentinel := filepath.Join(filepath.Dir(source), "installed-probe")
		bin := filepath.Join(filepath.Dir(source), "bin")
		writeFixture(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nprintf forbidden > \"$TRANSITION_CHILD_MARKER\"\nexit 0\n"), 0700)
		transitionExclusive(target)
		out = loopProcess(target, "bash", append([]string{filepath.Join(target, "scripts/factory-budget.sh")}, transitionRunArgs("budget")...), []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"), "TRANSITION_CHILD_MARKER=" + sentinel})
		Expect(out.status).To(Equal(2), "%+v", out)
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).To(ContainSubstring("runtime transition"))
		Expect(out.stderr).NotTo(ContainSubstring("No module named"))
		_, err = os.Lstat(sentinel)
		Expect(os.IsNotExist(err)).To(BeTrue(), "the installed script must import and enforce the shim before help")
		for _, name := range []string{"budget.json", "loops.json", "runtime-activity"} {
			_, err := os.Lstat(filepath.Join(target, ".factory", name))
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	}, Entry("compiled Go source initialization", "go-init"), Entry("actual legacy source initialization", "legacy-init"), Entry("actual legacy source upgrade delivers a missing dependency", "legacy-upgrade"))
})
