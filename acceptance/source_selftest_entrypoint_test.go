package acceptance_test

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Go source runtime entrypoints public selftest adapter", func() {
	// per docs/adr/0141-go-source-public-selftest.md:17
	It("uses physical source identity and transports literal arguments streams cwd and environment", func() {
		sourceControllerTransport("selftest")
	})

	// per docs/adr/0141-go-source-public-selftest.md:20
	It("refuses forged source metadata file symlinks and unavailable owners without fallback", func() {
		sourceControllerRefusal("selftest")
	})

	// per docs/adr/0141-go-source-public-selftest.md:22
	It("retains real compiler refusal without executing stale output or development regressions", func() {
		sourceControllerCompilerRefusal("selftest")
	})

	// per docs/adr/0141-go-source-public-selftest.md:61
	It("compiles its actual source owner and proves fourteen real gates without development or Python execution", func() {
		f := newSourceEntrypointFixture()
		sourceSelftestCopyAssets(f.root)
		program := sourceControllerAdapter(f.root, "selftest")
		base := filepath.Dir(f.root)
		tools, forbidden := filepath.Join(base, "poison tools"), filepath.Join(base, "FORBIDDEN")
		poison := []byte("#!/bin/sh\nprintf '%s\\n' \"$0\" >> \"$SOURCE_SELFTEST_FORBIDDEN\"\nexit 97\n")
		for _, tool := range []string{"python", "python3", "claude", "codex", "opencode", "curl", "wget", "gh", "npm", "npx", "factory"} {
			writeFixture(filepath.Join(tools, tool), poison, 0700)
		}
		writeFixture(filepath.Join(f.root, "scripts/selftest/development.sh"), poison, 0700)
		writeFixture(filepath.Join(f.root, ".factory/bin/factory-runtime"), poison, 0700)
		writeFixture(filepath.Join(f.caller, "factory"), poison, 0700)
		environment := maps.Clone(f.environment)
		maps.Copy(environment, sourceReviewGoCaches())
		environment["PATH"] = tools + string(os.PathListSeparator) + environment["PATH"]
		environment["SOURCE_SELFTEST_FORBIDDEN"] = forbidden
		environment["FACTORY_RUNTIME_BINARY"] = filepath.Join(f.caller, "factory")
		environment["REPO_ROOT"] = f.caller
		environment["FACTORY_CONFIG"] = filepath.Join(f.caller, "foreign.yaml")
		environment["FACTORY_EVENT_LOG"] = filepath.Join(f.caller, "foreign.events")
		writeFixture(environment["FACTORY_CONFIG"], []byte("check_command: false\n"), 0600)
		writeFixture(environment["FACTORY_EVENT_LOG"], []byte("caller events\n"), 0600)
		beforeSource, beforeCaller := sourceEntrypointSnapshot(f.root), installationSnapshot(f.caller)
		result := handoffNativeProcess(f.caller, environment, "", program)
		sourceSelftestExpectHealthy(result)
		compiled, err := os.ReadFile(filepath.Join(f.root, "factory-go"))
		Expect(err).NotTo(HaveOccurred(), "direct source invocation must compile the actual Go owner")
		Expect(compiled).NotTo(BeEmpty())
		Expect(sourceEntrypointSnapshot(f.root)).To(Equal(beforeSource))
		Expect(installationSnapshot(f.caller)).To(Equal(beforeCaller))
		_, err = os.Lstat(forbidden)
		Expect(os.IsNotExist(err)).To(BeTrue(), "public proof cannot execute development, Python, providers or alternate owners")
	})

	// per docs/adr/0141-go-source-public-selftest.md:62
	It("rejects both constant outcomes for every required gate and recovers through the public adapter", func() {
		f := newSourceSelftestFixture()
		program := sourceControllerAdapter(f.root, "selftest")
		writeFixture(filepath.Join(f.root, "scripts/selftest/development.sh"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_SELFTEST_FORBIDDEN\"\nexit 97\n"), 0700)
		sourceSelftestGateFaults(f, program)
	})

	// per docs/adr/0141-go-source-public-selftest.md:47
	It("keeps Make selftest public and runs development coverage through the actual configured check command", func() {
		root, _ := fixtureDirectories()
		for _, path := range []string{"Makefile", "factory.yaml"} {
			body, err := os.ReadFile(filepath.Join("..", path))
			Expect(err).NotTo(HaveOccurred())
			writeFixture(filepath.Join(root, path), body, 0600)
		}
		sourceHelperLibraryAssets(root)
		writeFixture(filepath.Join(root, "factory"), candidateBinary, 0700)
		calls := filepath.Join(filepath.Dir(root), "make calls")
		for _, name := range []string{"run", "development"} {
			writeFixture(filepath.Join(root, "scripts/selftest", name+".sh"), []byte("#!/bin/sh\nprintf '%s\\n' '"+name+"' >> \"$SOURCE_SELFTEST_MAKE_CALLS\"\nexit 0\n"), 0700)
		}
		for _, hook := range []string{"citation-lint", "shared-script-enforcement", "hook-existence-check", "copy-manifest-check", "gate-instrumentation-check", "wiki-lint", "workflow-lint"} {
			writeFixture(sourceSelftestGatePath(root, hook), []byte("#!/bin/sh\nexit 0\n"), 0700)
		}
		makeCommand, err := exec.LookPath("make")
		Expect(err).NotTo(HaveOccurred())
		environment := map[string]string{"SOURCE_SELFTEST_MAKE_CALLS": calls, "BASH_ENV": "", "ENV": "", "FACTORY_RUNTIME_BINARY": "", "FACTORY_BRIDGE_PROTOCOL": ""}
		public := handoffNativeExactProcess(root, environment, "", makeCommand, "--no-print-directory", "selftest")
		Expect(public.status).To(BeZero(), "%+v", public)
		Expect(os.ReadFile(calls)).To(Equal([]byte("run\n")))
		Expect(os.Remove(calls)).To(Succeed())
		checked := handoffNativeExactProcess(root, environment, "", makeCommand, "--no-print-directory", "check")
		Expect(checked.status).To(BeZero(), "%+v", checked)
		Expect(os.ReadFile(calls)).To(Equal([]byte("run\ndevelopment\n")), "the unchanged Make recipe must retain the separate development suite through factory.yaml")
		Expect(checked.stdout).To(ContainSubstring("check: running the configured product checks"))
		Expect(strings.Count(checked.stdout, "./scripts/selftest/development.sh")).To(Equal(1))
		Expect(os.Remove(calls)).To(Succeed())
		writeFixture(filepath.Join(root, "scripts/selftest/development.sh"), []byte("#!/bin/sh\nprintf 'development\\n' >> \"$SOURCE_SELFTEST_MAKE_CALLS\"\nexit 23\n"), 0700)
		failed := handoffNativeExactProcess(root, environment, "", makeCommand, "--no-print-directory", "check")
		Expect(failed.status).NotTo(BeZero(), "a failing development regression must fail the actual configured check: %+v", failed)
		Expect(os.ReadFile(calls)).To(Equal([]byte("run\ndevelopment\n")))
	})
})
