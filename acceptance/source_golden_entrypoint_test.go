package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Current script bytes are the subject, never a frozen oracle.
// per docs/adr/0132-go-source-golden-evaluation.md:56
func sourceGoldenAdapter(root string) string {
	GinkgoHelper()
	body, err := os.ReadFile("../scripts/golden-task-eval.sh")
	Expect(err).NotTo(HaveOccurred())
	path := filepath.Join(root, "scripts/golden-task-eval.sh")
	writeFixture(path, body, 0755)
	return path
}

// The actual child PATH excludes Python while retaining the historical script's
// ordinary utilities, so RED must reach its missing-scorer boundary.
// per docs/adr/0132-go-source-golden-evaluation.md:56
func sourceGoldenTools(base string) string {
	GinkgoHelper()
	tools := filepath.Join(base, "golden-tools")
	Expect(os.MkdirAll(tools, 0700)).To(Succeed())
	for _, name := range []string{"bash", "sh", "mkdir", "find", "sort", "dirname", "basename", "mktemp", "rm", "cat", "grep", "cksum", "awk", "seq", "cp", "sleep", "sed", "tr", "cut", "make", "git", "go"} {
		path, err := exec.LookPath(name)
		Expect(err).NotTo(HaveOccurred())
		path, err = filepath.EvalSymlinks(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Symlink(path, filepath.Join(tools, name))).To(Succeed())
	}
	return tools
}

var _ = Describe("Go source runtime entrypoints golden evaluation adapter", func() {
	// per docs/adr/0132-go-source-golden-evaluation.md:29
	// per docs/adr/0132-go-source-golden-evaluation.md:38
	// per docs/adr/0132-go-source-golden-evaluation.md:56
	It("scores a caller-owned task without Python through the direct source path", func() {
		source := newSourceEntrypointFixture()
		program := sourceGoldenAdapter(source.root)
		tools := sourceGoldenTools(filepath.Dir(source.root))
		source.environment["PATH"] = tools
		source.environment["HOME"] = os.Getenv("HOME")
		source.environment["FACTORY_CONFIG"] = filepath.Join(source.caller, "factory.yaml")
		writeFixture(filepath.Join(source.caller, "factory.yaml"), []byte("mock_frontier_model: caller-model\n"), 0600)
		writeFixture(filepath.Join(source.caller, "AGENTS.md"), []byte("Caller-owned evaluation policy.\n"), 0600)
		orchestrationTask(source.caller, "reference-answer")
		witness := filepath.Join(filepath.Dir(source.root), "runner-called")
		source.environment["SOURCE_GOLDEN_RUNNER_WITNESS"] = witness
		runner := []byte("#!/bin/sh\nprintf '%s\\n' \"$FACTORY_EVAL_MODEL\" >> \"$SOURCE_GOLDEN_RUNNER_WITNESS\"\nprintf 'FIXED\\n' > \"$1/answer.txt\"\nif [ \"${SOURCE_GOLDEN_CHEAT:-}\" = 1 ]; then printf '#!/bin/sh\\nexit 0\\n' > \"$1/verify.sh\"; fi\nexit 19\n")
		writeFixture(filepath.Join(source.caller, "eval/runners/mock.sh"), runner, 0700)
		lookup := handoffNativeExactProcess(source.caller, source.environment, "", "/bin/bash", "-c", "command -v python3")
		Expect(lookup).To(Equal(cliResult{"", "", 1}))
		before := sourceEntrypointSnapshot(source.root)
		observed := handoffNativeExactProcess(source.caller, source.environment, "borrowed stdin\n", program, "--runs", "2", "--save-baseline")
		AddReportEntry("source-golden.no-python", map[string]any{"status": observed.status, "stdout": observed.stdout, "stderr": observed.stderr, "PATH": tools, "source": source.root, "caller": source.caller}, ReportEntryVisibilityNever)
		Expect(observed.status).To(Equal(0), "%+v", observed)
		Expect(observed.stderr).To(BeEmpty())
		Expect(observed.stdout).To(ContainSubstring("2/2 passed (score 1.00)"))
		Expect(os.ReadFile(witness)).To(Equal([]byte("caller-model\ncaller-model\n")))
		result := orchestrationReadResults(source.caller, "current")
		Expect(result.Harness).To(Equal("mock"))
		Expect(result.Runner).To(Equal("eval/runners/mock.sh"))
		Expect(result.Runs).To(Equal(2))
		Expect(result.Tasks).To(HaveLen(1))
		Expect(result.Tasks[0].Task).To(Equal("reference-answer"))
		Expect(result.Tasks[0].Passes).To(Equal(2))
		Expect(result.Tasks[0].Score).To(Equal(1.0))
		inputs := append(append([]byte{}, runner...), []byte("Caller-owned evaluation policy.\nmock_frontier_model: caller-model\n")...)
		Expect(result.InputsFingerprint).To(Equal(orchestrationChecksum(inputs)))
		task, err := os.ReadFile(filepath.Join(source.caller, "eval/golden-tasks/reference-answer/task.md"))
		Expect(err).NotTo(HaveOccurred())
		oraclePath := filepath.Join(source.caller, "eval/golden-tasks/reference-answer/verify.sh")
		oracle, err := os.ReadFile(oraclePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Tasks[0].Fingerprint).To(Equal(orchestrationChecksum(append(task, oracle...))))
		current, err := os.ReadFile(filepath.Join(source.caller, "eval/results/mock-current.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.ReadFile(filepath.Join(source.caller, "eval/results/mock-baseline.json"))).To(Equal(current))
		Expect(sourceEntrypointSnapshot(source.root)).To(Equal(before), "the caller's evaluation must not mutate its source owner")
		info, err := os.Stat(filepath.Join(source.root, "factory-go"))
		Expect(err).NotTo(HaveOccurred(), "the private source launcher must actually compile")
		Expect(info.Mode().IsRegular()).To(BeTrue())

		// The actual Make recipe invokes the same public path in its own project.
		// per docs/adr/0132-go-source-golden-evaluation.md:9
		makefile, err := os.ReadFile("../Makefile")
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(source.caller, "Makefile"), makefile, 0600)
		sourceGoldenAdapter(source.caller)
		writeFixture(filepath.Join(source.caller, "factory"), candidateBinary, 0700)
		for _, phase := range []string{"compare", "oracle-tampering", "stale-policy"} {
			source.environment["SOURCE_GOLDEN_CHEAT"] = ""
			if phase == "oracle-tampering" {
				source.environment["SOURCE_GOLDEN_CHEAT"] = "1"
			}
			if phase == "stale-policy" {
				writeFixture(filepath.Join(source.caller, "AGENTS.md"), []byte("Changed caller policy.\n"), 0600)
			}
			observed = handoffNativeExactProcess(source.caller, source.environment, "", filepath.Join(tools, "make"), "golden-eval")
			AddReportEntry("source-golden.make", map[string]any{"phase": phase, "status": observed.status, "stdout": observed.stdout, "stderr": observed.stderr}, ReportEntryVisibilityNever)
			if phase == "compare" {
				Expect(observed.status).To(Equal(0), "%+v", observed)
				Expect(observed.stdout).To(ContainSubstring("no regression from baseline"))
			} else {
				Expect(observed.status).NotTo(Equal(0), "%+v", observed)
				if phase == "oracle-tampering" {
					Expect(observed.stdout).To(ContainSubstring("REGRESSION DETECTED"))
					Expect(orchestrationReadResults(source.caller, "current").Tasks[0].Passes).To(Equal(0))
				} else {
					Expect(observed.stdout).To(ContainSubstring("BASELINE STALE"))
				}
			}
			Expect(os.ReadFile(oraclePath)).To(Equal(oracle))
			Expect(os.ReadFile(filepath.Join(source.caller, "eval/results/mock-baseline.json"))).To(Equal(current))
			Expect(sourceEntrypointSnapshot(source.root)).To(Equal(before))
		}
	})

	// per docs/adr/0132-go-source-golden-evaluation.md:21
	// per docs/adr/0132-go-source-golden-evaluation.md:24
	It("preserves literal evaluation transport and physical ownership through directory aliases", func() {
		operands := []string{"", "space and ' quote", "literal $(not-code)\nline", "--", "--runs=2"}
		input := "original stdin\nliteral $(not-code)\x00tail\n"
		for _, alias := range []bool{false, true} {
			root, caller := fixtureDirectories()
			base := filepath.Dir(root)
			owner := filepath.Join(base, "physical owner ' $(not-code) \n")
			Expect(os.Rename(root, owner)).To(Succeed())
			program := sourceGoldenAdapter(owner)
			writeFixture(filepath.Join(owner, "factory"), []byte(sourceEntrypointRecorder), 0755)
			record, capturedInput, marker := filepath.Join(base, "argv"), filepath.Join(base, "stdin"), filepath.Join(base, "DECOY_CALLED")
			foreign, tools := filepath.Join(base, "foreign owner"), filepath.Join(base, "tools")
			for _, decoy := range []string{filepath.Join(foreign, "factory"), filepath.Join(caller, "factory"), filepath.Join(tools, "factory")} {
				writeFixture(decoy, []byte("#!/bin/sh\nprintf called > \"$SOURCE_ENTRY_DECOY\"\nexit 64\n"), 0755)
			}
			environment := map[string]string{
				"PATH": tools + ":/usr/bin:/bin", "BASH_ENV": "", "ENV": "",
				"SOURCE_ENTRY_ARGV": record, "SOURCE_ENTRY_INPUT": capturedInput, "SOURCE_ENTRY_DECOY": marker,
				"SOURCE_ENTRY_LITERAL_ENV": "literal environment $(not-code)\nvalue\n",
				"FACTORY_RUNTIME_BINARY":   filepath.Join(foreign, "factory"), "FACTORY_HOME": foreign,
			}
			if alias {
				Expect(os.Symlink(filepath.Join(owner, "scripts"), filepath.Join(foreign, "scripts"))).To(Succeed())
				program = filepath.Join(foreign, "scripts/golden-task-eval.sh")
			}
			beforeOwner, beforeCaller := installationSnapshot(owner), installationSnapshot(caller)
			observed := handoffNativeExactProcess(caller, environment, input, program, operands...)
			Expect(observed).To(Equal(cliResult{"", "", 63}), "directory alias=%t", alias)
			raw, err := os.ReadFile(record)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.HasSuffix(string(raw), "\x00")).To(BeTrue())
			Expect(strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")).To(Equal(append([]string{caller, environment["SOURCE_ENTRY_LITERAL_ENV"], "eval", "golden"}, operands...)))
			Expect(os.ReadFile(capturedInput)).To(Equal([]byte(input)))
			_, err = os.Lstat(marker)
			Expect(os.IsNotExist(err)).To(BeTrue())
			Expect(installationSnapshot(owner)).To(Equal(beforeOwner))
			Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
		}
	})

	// per docs/adr/0132-go-source-golden-evaluation.md:21
	// per docs/adr/0132-go-source-golden-evaluation.md:26
	It("refuses file aliases and absent owners and preserves failed-owner channels", func() {
		for _, mode := range []string{"missing-owner", "failed-owner", "file-alias"} {
			owner, caller := fixtureDirectories()
			base := filepath.Dir(owner)
			program := sourceGoldenAdapter(owner)
			marker, ownerCalled := filepath.Join(base, "FALLBACK_CALLED"), filepath.Join(base, "OWNER_CALLED")
			sourceObservabilityPoison(owner)
			if mode != "missing-owner" {
				writeFixture(filepath.Join(owner, "factory"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_OBSERVABILITY_OWNER\"\nprintf 'native output\\n'\nprintf 'native error\\n' >&2\nexit 47\n"), 0755)
			}
			if mode == "file-alias" {
				alias := filepath.Join(caller, "golden-task-eval.sh")
				Expect(os.Symlink(program, alias)).To(Succeed())
				program = alias
			}
			environment := map[string]string{"PATH": "/usr/bin:/bin", "BASH_ENV": "", "ENV": "", "SOURCE_OBSERVABILITY_FALLBACK": marker, "SOURCE_OBSERVABILITY_OWNER": ownerCalled}
			beforeOwner, beforeCaller := installationSnapshot(owner), installationSnapshot(caller)
			observed := handoffNativeExactProcess(caller, environment, "borrowed stdin\n", program)
			_, err := os.Lstat(marker)
			Expect(os.IsNotExist(err)).To(BeTrue(), "%s must not run a fallback", mode)
			_, ownerErr := os.Lstat(ownerCalled)
			switch mode {
			case "failed-owner":
				Expect(observed).To(Equal(cliResult{"native output\n", "native error\n", 47}))
				Expect(ownerErr).NotTo(HaveOccurred())
			case "file-alias":
				Expect(observed.status).To(Equal(2))
				Expect(observed.stdout).To(BeEmpty())
				Expect(observed.stderr).To(ContainSubstring("file symlink"))
				Expect(os.IsNotExist(ownerErr)).To(BeTrue())
			case "missing-owner":
				Expect(observed.status).NotTo(Equal(0))
				Expect(observed.stdout).To(BeEmpty())
				Expect(observed.stderr).NotTo(BeEmpty())
				Expect(os.IsNotExist(ownerErr)).To(BeTrue())
			}
			Expect(installationSnapshot(owner)).To(Equal(beforeOwner))
			Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
		}
	})
})
