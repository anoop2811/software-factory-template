package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const orchestrationEvalBaseline = "4ecab79627d08d1e125391a5831eb995efd3fce4"

func orchestrationFrozen(path string) []byte {
	GinkgoHelper()
	command := exec.Command("git", "show", orchestrationEvalBaseline+":"+path) // #nosec G204 -- immutable repository revision and test-owned path.
	command.Dir = ".."
	data, err := command.Output()
	Expect(err).NotTo(HaveOccurred(), "%s", path)
	return data
}

func orchestrationCommand(_, cwd string, environment []string, executable string, args ...string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, args...) // #nosec G204 -- compiled CLI or immutable oracle, literal test-owned argv.
	command.Dir, command.Env = cwd, environment
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred(), "watchdog expired: %s", stderr.String())
	status := 0
	if err != nil {
		var exited *exec.ExitError
		Expect(errors.As(err, &exited)).To(BeTrue(), "%v", err)
		status = exited.ExitCode()
	}
	return cliResult{stdout.String(), stderr.String(), status}
}

func orchestrationEvalFixture() (string, []string) {
	GinkgoHelper()
	root, _ := fixture()
	for _, path := range []string{"scripts/golden-task-eval.sh", "scripts/harness-structural-eval.sh", "scripts/lib/config.sh", "eval/runners/mock.sh"} {
		writeFixture(filepath.Join(root, "frozen", path), orchestrationFrozen(path), 0700)
	}
	writeFixture(filepath.Join(root, "eval/runners/mock.sh"), orchestrationFrozen("eval/runners/mock.sh"), 0700)
	writeFixture(filepath.Join(root, "scripts/golden-task-eval.sh"), []byte("#!/bin/sh\nprintf legacy-fallback >&2\nexit 98\n"), 0700)
	writeFixture(filepath.Join(root, "AGENTS.md"), []byte("Private eval instructions.\n"), 0600)
	writeFixture(filepath.Join(root, "factory.yaml"), []byte("review_lane: off\n"), 0600)
	return root, []string{"PATH=/bin:/usr/bin:/opt/homebrew/bin", "HOME=" + root, "FACTORY_CONFIG=" + filepath.Join(root, "factory.yaml"), "GORACE=atexit_sleep_ms=0"}
}

func orchestrationEval(root string, environment []string, args ...string) cliResult {
	return orchestrationCommand(root, root, environment, filepath.Join(root, "factory"), append([]string{"eval"}, args...)...)
}

func orchestrationTask(root, name string) {
	GinkgoHelper()
	writeFixture(filepath.Join(root, "eval/golden-tasks", name, "task.md"), []byte("Create answer.txt containing FIXED.\n"), 0600)
	writeFixture(filepath.Join(root, "eval/golden-tasks", name, "verify.sh"), []byte("#!/bin/sh\n[ \"$(/bin/cat answer.txt 2>/dev/null)\" = FIXED ]\n"), 0700)
}

type orchestrationTaskResult struct {
	Task, Fingerprint string
	Passes, Runs      int
	Score             float64
}
type orchestrationResults struct {
	Harness, Runner   string
	Runs              int
	InputsFingerprint string `json:"inputs_fingerprint"`
	Tasks             []orchestrationTaskResult
}

func orchestrationReadResults(root, suffix string) orchestrationResults {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join(root, "eval/results/mock-"+suffix+".json"))
	Expect(err).NotTo(HaveOccurred())
	var result orchestrationResults
	Expect(json.Unmarshal(data, &result)).To(Succeed())
	return result
}

func orchestrationChecksum(data []byte) string {
	GinkgoHelper()
	command := exec.Command("/usr/bin/cksum")
	command.Stdin = bytes.NewReader(data)
	raw, err := command.Output()
	Expect(err).NotTo(HaveOccurred())
	parts := strings.Fields(string(raw))
	Expect(parts).To(HaveLen(2))
	return strings.Join(parts, "-")
}

var _ = Describe("Native orchestration eval", func() {
	// per docs/adr/0099-native-factory-orchestration.md:104
	// per docs/adr/0099-native-factory-orchestration.md:157
	DescribeTable("retains strict golden parsing against the immutable shell oracle", func(args []string, diagnostic string) {
		root, environment := orchestrationEvalFixture()
		baseline := orchestrationCommand(root, root, environment, "/bin/bash", append([]string{filepath.Join(root, "frozen/scripts/golden-task-eval.sh")}, args...)...)
		Expect(baseline.status).To(Equal(2))
		Expect(baseline.stderr).To(ContainSubstring(diagnostic))
		result := orchestrationEval(root, environment, append([]string{"golden"}, args...)...)
		Expect(result.status).To(Equal(baseline.status), result.stderr)
		Expect(result.stderr).To(Equal(baseline.stderr))
		Expect(result.stdout).To(Equal(baseline.stdout))
	},
		Entry("unknown option", []string{"--runns=2"}, "unknown argument '--runns=2'"),
		Entry("positional operand", []string{"surprise"}, "unknown argument 'surprise'"),
		Entry("missing spaced runner", []string{"--runner"}, "--runner needs a value"),
		Entry("empty spaced runs", []string{"--runs", ""}, "--runs needs a value"),
		Entry("missing timeout", []string{"--timeout"}, "--timeout needs a value"),
		Entry("unknown harness cannot fall back", []string{"--harness=unshipped"}, "Not falling back to the mock"),
	)

	// per docs/adr/0099-native-factory-orchestration.md:104
	It("measures nothing and publishes no result when no tasks exist", func() {
		root, environment := orchestrationEvalFixture()
		result := orchestrationEval(root, environment, "golden")
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(result.stdout).To(ContainSubstring("no tasks in eval/golden-tasks/"))
		_, err := os.Stat(filepath.Join(root, "eval/results/mock-current.json"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0099-native-factory-orchestration.md:104
	// per docs/adr/0099-native-factory-orchestration.md:116
	DescribeTable("scores the deterministic mock without invoking a model", func(mode string, passes int) {
		root, environment := orchestrationEvalFixture()
		orchestrationTask(root, "reference-answer")
		result := orchestrationEval(root, append(environment, "FACTORY_MOCK_MODE="+mode), "golden", "--runs", "2", "--timeout=2")
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(result.stdout).To(ContainSubstring(fmt.Sprintf("%d/2 passed (score %.2f)", passes, float64(passes)/2)))
		current := orchestrationReadResults(root, "current")
		Expect(current.Harness).To(Equal("mock"))
		Expect(current.Runner).To(Equal("eval/runners/mock.sh"))
		Expect(current.Runs).To(Equal(2))
		Expect(current.Tasks).To(HaveLen(1))
		Expect(current.Tasks[0].Passes).To(Equal(passes))
	}, Entry("default pass", "pass", 2), Entry("fail", "fail", 0), Entry("oracle tampering", "cheat", 0))

	// per docs/adr/0099-native-factory-orchestration.md:106
	It("saves complete schema with independent POSIX checksums and identical baseline bytes", func() {
		root, environment := orchestrationEvalFixture()
		orchestrationTask(root, "reference-answer")
		result := orchestrationEval(root, environment, "golden", "--save-baseline")
		Expect(result.status).To(Equal(0), result.stderr)
		current := orchestrationReadResults(root, "current")
		input := append(orchestrationFrozen("eval/runners/mock.sh"), []byte("Private eval instructions.\n")...)
		Expect(current.InputsFingerprint).To(Equal(orchestrationChecksum(input)))
		task, err := os.ReadFile(filepath.Join(root, "eval/golden-tasks/reference-answer/task.md"))
		Expect(err).NotTo(HaveOccurred())
		oracle, err := os.ReadFile(filepath.Join(root, "eval/golden-tasks/reference-answer/verify.sh"))
		Expect(err).NotTo(HaveOccurred())
		Expect(current.Tasks[0].Fingerprint).To(Equal(orchestrationChecksum(append(task, oracle...))))
		baseline, err := os.ReadFile(filepath.Join(root, "eval/results/mock-baseline.json"))
		Expect(err).NotTo(HaveOccurred())
		published, err := os.ReadFile(filepath.Join(root, "eval/results/mock-current.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(baseline).To(Equal(published))
		Expect(result.stdout).To(ContainSubstring("it proves the scorer, not your agents"))
	})

	// per docs/adr/0099-native-factory-orchestration.md:105
	// per docs/adr/0099-native-factory-orchestration.md:107
	It("runs sorted immediate tasks in private copies and keeps a literal runner path", func() {
		root, environment := orchestrationEvalFixture()
		for _, task := range []string{"z-last", "a-first"} {
			orchestrationTask(root, task)
		}
		orchestrationTask(filepath.Join(root, "eval/golden-tasks/nested"), "ignored-grandchild")
		runner := filepath.Join(root, "runner $(touch must-not-exist).sh")
		writeFixture(runner, []byte("#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$RUNNER_WORKDIRS\"\n[ ! -e \"$1/copied-marker\" ] || exit 99\nprintf marker > \"$1/copied-marker\"\nprintf 'FIXED\\n' > \"$1/answer.txt\"\n"), 0700)
		log := filepath.Join(root, "workdirs")
		result := orchestrationEval(root, append(environment, "RUNNER_WORKDIRS="+log), "golden", "--runner", runner, "--runs=2")
		Expect(result.status).To(Equal(0), result.stderr)
		current := orchestrationReadResults(root, "current")
		Expect(current.Tasks).To(HaveLen(2))
		Expect(current.Tasks[0].Task).To(Equal("a-first"))
		Expect(current.Tasks[1].Task).To(Equal("z-last"))
		for _, task := range current.Tasks {
			Expect(task.Passes).To(Equal(2))
		}
		raw, err := os.ReadFile(log)
		Expect(err).NotTo(HaveOccurred())
		workdirs := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
		Expect(workdirs).To(HaveLen(4))
		for _, work := range workdirs {
			_, err := os.Stat(work)
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
		_, err = os.Stat(filepath.Join(root, "must-not-exist"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		_, err = os.Stat(filepath.Join(root, "eval/golden-tasks/a-first/answer.txt"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0099-native-factory-orchestration.md:112
	It("scores a valid original oracle despite an ordinary nonzero runner exit", func() {
		root, environment := orchestrationEvalFixture()
		orchestrationTask(root, "reference-answer")
		runner := filepath.Join(root, "nonzero-runner")
		writeFixture(runner, []byte("#!/bin/sh\nprintf 'FIXED\\n' > \"$1/answer.txt\"\nexit 37\n"), 0700)
		result := orchestrationEval(root, environment, "golden", "--runner="+runner)
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(orchestrationReadResults(root, "current").Tasks[0].Passes).To(Equal(1))
	})

	// per docs/adr/0099-native-factory-orchestration.md:109
	It("fails a timed-out valid answer and disposes the observed child process group", func() {
		root, environment := orchestrationEvalFixture()
		orchestrationTask(root, "reference-answer")
		runner := filepath.Join(root, "hanging-runner")
		pidfile := filepath.Join(root, "runner-child.pid")
		DeferCleanup(func() {
			if raw, err := os.ReadFile(pidfile); err == nil {
				if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil {
					_ = syscall.Kill(pid, syscall.SIGKILL)
				}
			}
		})
		writeFixture(runner, []byte("#!/bin/sh\nprintf 'FIXED\\n' > \"$1/answer.txt\"\n/bin/sleep 60 &\nprintf '%s\\n' \"$!\" > \"$CHILD_PID_FILE\"\nwait\n"), 0700)
		result := orchestrationEval(root, append(environment, "CHILD_PID_FILE="+pidfile), "golden", "--runner="+runner, "--timeout=1")
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(result.stdout).To(ContainSubstring("hit the 1s cap"))
		Expect(orchestrationReadResults(root, "current").Tasks[0].Passes).To(BeZero())
		raw, err := os.ReadFile(pidfile)
		Expect(err).NotTo(HaveOccurred())
		pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() bool { return transitionAlive(root, pid) }, 3*time.Second, 25*time.Millisecond).Should(BeFalse())
	})

	// per docs/adr/0099-native-factory-orchestration.md:106
	DescribeTable("refuses a stale baseline rather than claiming no regression", func(change string) {
		root, environment := orchestrationEvalFixture()
		orchestrationTask(root, "reference-answer")
		orchestrationTask(root, "retained")
		Expect(orchestrationEval(root, environment, "golden", "--save-baseline").status).To(BeZero())
		switch change {
		case "oracle":
			writeFixture(filepath.Join(root, "eval/golden-tasks/reference-answer/verify.sh"), []byte("#!/bin/sh\n# changed oracle\nexit 0\n"), 0700)
		case "instructions":
			writeFixture(filepath.Join(root, "AGENTS.md"), []byte("Changed instructions.\n"), 0600)
		case "removed":
			Expect(os.RemoveAll(filepath.Join(root, "eval/golden-tasks/reference-answer"))).To(Succeed())
		}
		result := orchestrationEval(root, environment, "golden")
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(ContainSubstring("BASELINE STALE"))
		Expect(result.stdout).NotTo(ContainSubstring("golden-task-eval: no regression from baseline"))
	}, Entry("oracle changed", "oracle"), Entry("shared instructions changed", "instructions"), Entry("task removed", "removed"))

	// per docs/adr/0099-native-factory-orchestration.md:106
	It("reports an actual score regression against unchanged inputs", func() {
		root, environment := orchestrationEvalFixture()
		orchestrationTask(root, "reference-answer")
		Expect(orchestrationEval(root, environment, "golden", "--save-baseline").status).To(BeZero())
		result := orchestrationEval(root, append(environment, "FACTORY_MOCK_MODE=fail"), "golden")
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(ContainSubstring("REGRESSION DETECTED"))
		Expect(result.stdout).To(ContainSubstring("1.00 -> 0.00"))
	})

	// per docs/adr/0099-native-factory-orchestration.md:105
	// per docs/adr/0099-native-factory-orchestration.md:106
	DescribeTable("rejects baseline scores that cannot support a regression comparison", func(nullScore bool) {
		root, environment := orchestrationEvalFixture()
		orchestrationTask(root, "reference-answer")
		Expect(orchestrationEval(root, environment, "golden", "--save-baseline").status).To(BeZero())
		path := filepath.Join(root, "eval/results/mock-baseline.json")
		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		var baseline map[string]any
		Expect(json.Unmarshal(data, &baseline)).To(Succeed())
		task := baseline["tasks"].([]any)[0].(map[string]any)
		if nullScore {
			task["score"] = nil
		} else {
			delete(task, "score")
		}
		data, err = json.Marshal(baseline)
		Expect(err).NotTo(HaveOccurred())
		writeFixture(path, data, 0600)
		result := orchestrationEval(root, environment, "golden")
		Expect(result.status).To(Equal(1), "an absent score must not silently become zero: %+v", result)
		Expect(result.stdout).NotTo(ContainSubstring("no regression from baseline"))
	}, Entry("missing score", false), Entry("null score", true))

	// per docs/adr/0099-native-factory-orchestration.md:115
	// per docs/adr/0099-native-factory-orchestration.md:157
	DescribeTable("preserves native runner argv, optional model, literal final prompt and log", func(harness, model string) {
		root, environment := orchestrationEvalFixture()
		work := filepath.Join(root, "task directory with spaces")
		prompt := "Implement literal $(touch injected) 'quoted' \"text\"\nsecond line\n\n"
		writeFixture(filepath.Join(work, "task.md"), []byte(prompt), 0600)
		arguments := filepath.Join(root, "arguments")
		writeFixture(filepath.Join(root, "bin", harness), []byte("#!/bin/sh\nprintf '%s\\000' \"$PWD\" \"$@\" > \"$ARGUMENT_LOG\"\nprintf 'harness stdout\\n'\nprintf 'harness stderr\\n' >&2\nexit 37\n"), 0700)
		environment = append(environment, "PATH="+filepath.Join(root, "bin")+":/bin:/usr/bin", "ARGUMENT_LOG="+arguments, "FACTORY_EVAL_MODEL="+model)
		var expected []string
		switch harness {
		case "claude":
			expected = []string{"-p", "--permission-mode", "acceptEdits"}
		case "codex":
			expected = []string{"exec", "--sandbox", "workspace-write", "--skip-git-repo-check"}
		case "opencode":
			expected = []string{"run", "--agent", "implementer"}
		}
		if model != "" {
			expected = append(expected, "--model", model)
		}
		expected = append([]string{work}, append(expected, strings.TrimRight(prompt, "\n"))...)
		for _, legacy := range []bool{true, false} {
			var result cliResult
			if legacy {
				path := filepath.Join(root, "frozen-runner.sh")
				writeFixture(path, orchestrationFrozen("eval/runners/"+harness+".sh"), 0700)
				result = orchestrationCommand(root, root, environment, "/bin/sh", path, work)
			} else {
				result = orchestrationEval(root, environment, "run", harness, work)
			}
			Expect(result).To(Equal(cliResult{"", "", 0}), "legacy=%t", legacy)
			raw, err := os.ReadFile(arguments)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")).To(Equal(expected))
			log, err := os.ReadFile(filepath.Join(work, harness+"-runner.log"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(log)).To(Equal("harness stdout\nharness stderr\n"))
		}
		_, err := os.Stat(filepath.Join(work, "injected"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("Claude default", "claude", ""), Entry("Claude literal model", "claude", "fixture/model $(literal)"), Entry("Codex default", "codex", ""), Entry("Codex literal model", "codex", "fixture/model $(literal)"), Entry("OpenCode default", "opencode", ""), Entry("OpenCode literal model", "opencode", "fixture/model $(literal)"))

	// per docs/adr/0099-native-factory-orchestration.md:113
	// per docs/adr/0099-native-factory-orchestration.md:117
	DescribeTable("refuses unwired or unknown runners without a model fallback", func(harness string) {
		root, environment := orchestrationEvalFixture()
		result := orchestrationEval(root, environment, "run", harness, root)
		Expect(result.status).To(Equal(2), result.stderr)
		Expect(result.stderr).To(ContainSubstring(harness))
		Expect(result.stderr).NotTo(ContainSubstring("unknown command 'eval'"))
	}, Entry("example is inert", "example-harness"), Entry("unknown name", "not-a-harness"))

	// per docs/adr/0099-native-factory-orchestration.md:105
	// per docs/adr/0099-native-factory-orchestration.md:113
	It("executes a customized named runner literally without selecting its paid harness CLI", func() {
		root, environment := orchestrationEvalFixture()
		orchestrationTask(root, "local-safety")
		runner := []byte("#!/bin/sh\nprintf '%s\\000' \"$PWD\" \"$1\" > \"$LOCAL_RUNNER_ARGUMENTS\"\nprintf FIXED > \"$1/answer.txt\"\n")
		writeFixture(filepath.Join(root, "eval/runners/claude.sh"), runner, 0700)
		marker := filepath.Join(root, "paid-harness-selected")
		arguments := filepath.Join(root, "local-runner-arguments")
		writeFixture(filepath.Join(root, "bin/claude"), []byte("#!/bin/sh\nprintf forbidden > \"$PAID_HARNESS_MARKER\"\nexit 89\n"), 0700)
		environment = append(environment, "PATH="+filepath.Join(root, "bin")+":/bin:/usr/bin", "PAID_HARNESS_MARKER="+marker, "LOCAL_RUNNER_ARGUMENTS="+arguments)
		result := orchestrationEval(root, environment, "golden", "--harness=claude", "--runs=1")
		_, err := os.Stat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue(), "a local-only runner named claude must never cause a direct paid CLI invocation: %+v", result)
		Expect(result.status).To(BeZero(), "%+v", result)
		Expect(result.stdout).To(ContainSubstring("1/1 passed (score 1.00)"))
		data, err := os.ReadFile(arguments)
		Expect(err).NotTo(HaveOccurred(), "the selected script must actually execute")
		fields := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
		Expect(fields).To(HaveLen(2))
		Expect(fields[0]).To(Equal(root), "custom runners retain the caller repository cwd")
		Expect(fields[1]).NotTo(Equal(root))
		Expect(fields[1]).NotTo(Equal(filepath.Join(root, "eval/golden-tasks/local-safety")), "the literal task operand identifies a private task copy")
		data, err = os.ReadFile(filepath.Join(root, "eval/results/claude-current.json"))
		Expect(err).NotTo(HaveOccurred())
		var current orchestrationResults
		Expect(json.Unmarshal(data, &current)).To(Succeed())
		Expect(current.Harness).To(Equal("claude"))
		Expect(current.Runner).To(Equal("eval/runners/claude.sh"))
		Expect(current.InputsFingerprint).To(Equal(orchestrationChecksum(append(runner, []byte("Private eval instructions.\n")...))))
	})

	// per docs/adr/0099-native-factory-orchestration.md:105
	// per docs/adr/0099-native-factory-orchestration.md:107
	It("keeps a custom runner in the caller repository while passing a private task directory literally", func() {
		root, environment := orchestrationEvalFixture()
		orchestrationTask(root, "relative-runner-input")
		writeFixture(filepath.Join(root, "repo-marker"), []byte("REPO_ONLY\n"), 0600)
		runner := filepath.Join(root, "local-only-runner.sh")
		writeFixture(runner, []byte("#!/bin/sh\n[ \"$(/bin/cat repo-marker 2>/dev/null)\" = REPO_ONLY ] || exit 41\nprintf '%s\\000%s' \"$PWD\" \"$1\" > \"$CUSTOM_CWD_LOG\"\nprintf FIXED > \"$1/answer.txt\"\n"), 0700)
		arguments := filepath.Join(root, "custom-cwd-log")
		result := orchestrationEval(root, append(environment, "CUSTOM_CWD_LOG="+arguments), "golden", "--runner", runner)
		Expect(result.status).To(BeZero(), "%+v", result)
		Expect(result.stdout).To(ContainSubstring("relative-runner-input: 1/1 passed (score 1.00)"))
		data, err := os.ReadFile(arguments)
		Expect(err).NotTo(HaveOccurred())
		fields := strings.Split(string(data), "\x00")
		Expect(fields).To(HaveLen(2))
		Expect(fields[0]).To(Equal(root))
		Expect(fields[1]).NotTo(Equal(filepath.Join(root, "eval/golden-tasks/relative-runner-input")))
		_, err = os.Stat(filepath.Join(root, "eval/golden-tasks/relative-runner-input/answer.txt"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "the original task remains untouched")
	})

	// per docs/adr/0099-native-factory-orchestration.md:244
	DescribeTable("distinguishes missing structural adapters from ordinary missing markers", func(harness, path string) {
		root, environment := orchestrationEvalFixture()
		legacy := orchestrationCommand(root, root, environment, "/bin/bash", filepath.Join(root, "frozen/scripts/harness-structural-eval.sh"), "--harness="+harness)
		Expect(legacy.status).To(Equal(2), "%+v", legacy)
		Expect(legacy.stderr).To(ContainSubstring(path))
		missing := orchestrationEval(root, environment, "structural", "--harness="+harness)
		Expect(missing.status).To(Equal(legacy.status), "%+v", missing)
		Expect(missing.stderr).To(ContainSubstring(path))
		writeFixture(filepath.Join(root, path), []byte("PRIVATE_ADAPTER_CONTENT_WITHOUT_MARKERS\n"), 0600)
		mismatch := orchestrationEval(root, environment, "structural", "--harness="+harness)
		Expect(mismatch.status).To(Equal(1), "%+v", mismatch)
		Expect(mismatch.stderr).NotTo(ContainSubstring("PRIVATE_ADAPTER_CONTENT"))
	}, Entry("OpenCode", "opencode", ".opencode/plugin/factory-hooks.ts"),
		Entry("Claude", "claude", ".claude/agents/implementer.md"),
		Entry("Codex", "codex", ".codex/agents/implementer.toml"))

	// per docs/adr/0099-native-factory-orchestration.md:99
	// per docs/adr/0099-native-factory-orchestration.md:104
	DescribeTable("evaluates caller project inputs when its binary is located in another checkout", func(operation string) {
		root, environment := orchestrationEvalFixture()
		caller := filepath.Join(filepath.Dir(root), "caller evaluation project")
		Expect(os.MkdirAll(caller, 0700)).To(Succeed())
		writeFixture(filepath.Join(caller, "factory.yaml"), []byte("test_file_patterns: ''\n"), 0600)
		writeFixture(filepath.Join(caller, "AGENTS.md"), []byte("Caller project evaluation instructions.\n"), 0600)
		environment = append(environment, "FACTORY_CONFIG="+filepath.Join(caller, "factory.yaml"))
		args := []string{"eval", operation}
		if operation == "golden" {
			orchestrationTask(caller, "caller-only")
			writeFixture(filepath.Join(caller, "eval/runners/mock.sh"), orchestrationFrozen("eval/runners/mock.sh"), 0700)
		} else {
			writeFixture(filepath.Join(caller, ".opencode/plugin/factory-hooks.ts"), []byte("// test-edit-denial.sh agentNameToRole\n"), 0600)
			writeFixture(filepath.Join(caller, "scripts/hooks/test-edit-denial.sh"), orchestrationFrozen("scripts/hooks/test-edit-denial.sh"), 0700)
			writeFixture(filepath.Join(caller, "scripts/lib/config.sh"), orchestrationFrozen("scripts/lib/config.sh"), 0600)
			args = append(args, "--harness=opencode")
		}
		before := nativeInitArtifacts(root)
		result := orchestrationCommand(root, caller, environment, filepath.Join(root, "factory"), args...)
		Expect(result.status).To(BeZero(), "%+v", result)
		if operation == "golden" {
			Expect(result.stdout).To(ContainSubstring("caller-only: 1/1 passed"))
			current := orchestrationReadResults(caller, "current")
			Expect(current.Tasks).To(HaveLen(1))
			Expect(current.Tasks[0].Task).To(Equal("caller-only"))
		} else {
			Expect(result.stdout).To(ContainSubstring("PASS harness=opencode"))
		}
		Expect(nativeInitArtifacts(root)).To(Equal(before), "the executable checkout is not the evaluation project")
	}, Entry("golden tasks and results", "golden"), Entry("structural adapter and real hook", "structural"))

	// per docs/adr/0099-native-factory-orchestration.md:99
	DescribeTable("probes the real denial hook with all three adapter payloads and isolated config", func(harness string) {
		root, environment := orchestrationEvalFixture()
		realHook := filepath.Join(root, "frozen/scripts/hooks/test-edit-denial.sh")
		writeFixture(realHook, orchestrationFrozen("scripts/hooks/test-edit-denial.sh"), 0700)
		records := filepath.Join(root, "probe-records")
		Expect(os.MkdirAll(records, 0700)).To(Succeed())
		writeFixture(filepath.Join(root, "scripts/hooks/test-edit-denial.sh"), []byte("#!/bin/sh\nprintf '%s\\000' \"$FACTORY_AGENT_ROLE\" \"$#\" \"$@\" > \"$EVAL_RECORD_DIR/$FACTORY_AGENT_ROLE-argv\"\nprintf '%s' \"$FACTORY_CONFIG\" > \"$EVAL_RECORD_DIR/config-path\"\n/bin/cat \"$FACTORY_CONFIG\" > \"$EVAL_RECORD_DIR/config-bytes\"\nif [ \"$#\" = 0 ]; then\n /bin/cat > \"$EVAL_RECORD_DIR/$FACTORY_AGENT_ROLE-stdin\"\n exec \"$EVAL_REAL_HOOK\" < \"$EVAL_RECORD_DIR/$FACTORY_AGENT_ROLE-stdin\"\nfi\nexec \"$EVAL_REAL_HOOK\" \"$@\"\n"), 0700)
		environment = append(environment, "EVAL_RECORD_DIR="+records, "EVAL_REAL_HOOK="+realHook)
		writeFixture(filepath.Join(root, ".opencode/plugin/factory-hooks.ts"), []byte("// test-edit-denial.sh agentNameToRole\n"), 0600)
		writeFixture(filepath.Join(root, ".claude/agents/implementer.md"), []byte("FACTORY_AGENT_ROLE=implementer scripts/hooks/test-edit-denial.sh\n"), 0600)
		writeFixture(filepath.Join(root, ".codex/agents/implementer.toml"), []byte("FACTORY_AGENT_ROLE=implementer scripts/hooks/test-edit-denial.sh\n"), 0600)
		// Host configuration deliberately disarms denial; the evaluator must isolate its own.
		before := []byte("test_file_patterns: ''\n")
		writeFixture(filepath.Join(root, "factory.yaml"), before, 0600)
		assertProbes := func() {
			for _, role := range []string{"implementer", "spec-writer"} {
				raw, err := os.ReadFile(filepath.Join(records, role+"-argv"))
				Expect(err).NotTo(HaveOccurred())
				if harness == "opencode" {
					Expect(string(raw)).To(Equal(role + "\x001\x00pkg/probe_test.go\x00"))
				} else {
					Expect(string(raw)).To(Equal(role + "\x000\x00"))
					payload, err := os.ReadFile(filepath.Join(records, role+"-stdin"))
					Expect(err).NotTo(HaveOccurred())
					var parsed map[string]any
					Expect(json.Unmarshal(payload, &parsed)).To(Succeed())
					if harness == "claude" {
						Expect(parsed).To(Equal(map[string]any{"tool_name": "Write", "tool_input": map[string]any{"file_path": "pkg/probe_test.go"}}))
					} else {
						Expect(parsed).To(Equal(map[string]any{"tool_name": "apply_patch", "tool_input": map[string]any{"command": "*** Begin Patch\n*** Update File: pkg/probe_test.go\n@@\n-old\n+new\n*** End Patch"}}))
					}
				}
			}
			configPath, err := os.ReadFile(filepath.Join(records, "config-path"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(configPath)).NotTo(Equal(filepath.Join(root, "factory.yaml")))
			configBytes, err := os.ReadFile(filepath.Join(records, "config-bytes"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(configBytes)).To(ContainSubstring("test_file_patterns:"))
			Expect(configBytes).NotTo(Equal(before))
		}
		legacy := orchestrationCommand(root, root, environment, "/bin/bash", filepath.Join(root, "frozen/scripts/harness-structural-eval.sh"), "ignored", "--harness="+harness, "--unknown-ignored")
		Expect(legacy.status).To(BeZero(), legacy.stderr)
		assertProbes()
		Expect(os.RemoveAll(records)).To(Succeed())
		Expect(os.MkdirAll(records, 0700)).To(Succeed())
		result := orchestrationEval(root, environment, "structural", "ignored", "--harness="+harness, "--unknown-ignored")
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(result.stdout).To(ContainSubstring("PASS harness=" + harness))
		Expect(result.stdout).To(ContainSubstring("live in-harness parity remains a separate OBSERVED gate"))
		assertProbes()
		after, err := os.ReadFile(filepath.Join(root, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		// A hook that permits implementer edits must make the structural evaluator fail.
		writeFixture(filepath.Join(root, "scripts/hooks/test-edit-denial.sh"), []byte("#!/bin/sh\nexit 0\n"), 0700)
		broken := orchestrationEval(root, environment, "structural", "--harness="+harness)
		Expect(broken.status).To(Equal(1), broken.stderr)
		Expect(broken.stderr).To(ContainSubstring("implementer edit returned 0, expected 2"))
		writeFixture(filepath.Join(root, "scripts/hooks/test-edit-denial.sh"), []byte("#!/bin/sh\nexit 2\n"), 0700)
		broken = orchestrationEval(root, environment, "structural", "--harness="+harness)
		Expect(broken.status).To(Equal(1), broken.stderr)
		Expect(broken.stderr).To(ContainSubstring("non-implementer edit returned 2, expected 0"))
	}, Entry("OpenCode argv", "opencode"), Entry("Claude Write JSON", "claude"), Entry("Codex patch JSON", "codex"))
})
