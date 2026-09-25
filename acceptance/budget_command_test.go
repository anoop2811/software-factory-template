package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func budgetCLIProcess(root, cwd, program string, args []string, extra ...string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, args...) // #nosec G204 G702 -- test-owned executable and literal argv, no shell.
	cmd.Dir = cwd
	cmd.Env = append([]string{"FACTORY_BRIDGE_PROTOCOL=1", "FACTORY_BUDGET_ENABLED=true", "PATH=" + filepath.Join(root, "bin"), "LC_ALL=C", "PYTHONDONTWRITEBYTECODE=1", "GORACE=atexit_sleep_ms=0"}, extra...)
	if root != "" {
		cmd.Env = append(cmd.Env, "FACTORY_BUDGET_ROOT="+root)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred())
	status := 0
	if err != nil {
		var exit *exec.ExitError
		Expect(errors.As(err, &exit)).To(BeTrue())
		status = exit.ExitCode()
	}
	return cliResult{stdout.String(), stderr.String(), status}
}
func budgetCLI(root, cwd string, args []string, extra ...string) cliResult {
	budgetCommandCandidate(root)
	return budgetCLIProcess(root, cwd, filepath.Join(root, "factory"), append([]string{"budget", "controller"}, args...), extra...)
}
func budgetCLIOracle(root, cwd string, args []string, extra ...string) cliResult {
	GinkgoHelper()
	repo, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	oracle := filepath.Join(root, "immutable-oracle")
	for _, name := range []string{"budget.py", "budget_adapters.py"} {
		cmd := exec.Command("git", "show", "4cc771e894e11d5024032106aeb0ef788997bd29:scripts/lib/"+name) // #nosec G204 -- immutable revision and test-owned fixed source names.
		cmd.Dir = repo
		source, err := cmd.Output()
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(oracle, name), source, 0600)
	}
	python, err := exec.LookPath("python3")
	Expect(err).NotTo(HaveOccurred())
	resolved, err := exec.Command(python, "-B", "-c", "import sys; print(sys.executable)").Output() // #nosec G204 G702 -- resolve the local test oracle interpreter, with a fixed program.
	Expect(err).NotTo(HaveOccurred())
	python = strings.TrimSpace(string(resolved))
	return budgetCLIProcess(root, cwd, python, append([]string{"-B", filepath.Join(oracle, "budget.py")}, args...), extra...)
}

var _ = Describe("G2 budget command candidate", func() {
	// per docs/adr/0071-go-budget-command-candidate.md:58
	DescribeTable("matches immutable read-only command rendering", func(action string, jsonOutput bool) {
		root, cwd := fixture()
		if oldBinary := os.Getenv("FACTORY_BUDGET_COMMAND_BASELINE_BINARY"); oldBinary != "" {
			raw, err := os.ReadFile(oldBinary)
			Expect(err).NotTo(HaveOccurred())
			writeFixture(filepath.Join(root, "factory"), raw, 0755)
		}
		args := []string{action}
		if action == "plan" {
			args = append(args, "--session=s", "--task", "t", "--harness", "claude")
		}
		if jsonOutput {
			args = append(args, "--json")
		}
		baseline := budgetCLIOracle(root, cwd, args)
		Expect(baseline.status).To(BeZero())
		Expect(baseline.stderr).To(BeEmpty())
		Expect(baseline.stdout).NotTo(BeEmpty())
		actual := budgetCLI(root, cwd, args)
		Expect(actual.status).To(Equal(baseline.status))
		Expect(actual.stderr).To(BeEmpty())
		if jsonOutput {
			Expect(budgetJSONComparable(budgetDecode([]byte(actual.stdout)))).To(Equal(budgetJSONComparable(budgetDecode([]byte(baseline.stdout)))))
		} else {
			Expect(actual.stdout).To(Equal(baseline.stdout))
		}
		Expect(strings.HasSuffix(actual.stdout, "\n")).To(BeTrue())
		_, err := os.Stat(filepath.Join(root, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("text plan", "plan", false), Entry("JSON plan", "plan", true), Entry("text report", "report", false), Entry("JSON report", "report", true))
})

var _ = Describe("G2 budget command validation", func() {
	// per docs/adr/0072-go-budget-argument-compatibility.md:46
	DescribeTable("refuses invalid command input without state or native calls", func(args []string) {
		root, cwd := fixture()
		out := budgetArgumentParity(root, cwd, args)
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).NotTo(ContainSubstring("factory bridge:"))
		_, err := os.Stat(filepath.Join(root, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("missing action", []string{}), Entry("missing required", []string{"plan"}), Entry("empty session", []string{"plan", "--session=", "--task=t", "--harness=codex"}), Entry("invalid ID", []string{"plan", "--session=../x", "--task=t", "--harness=codex"}), Entry("invalid harness", []string{"plan", "--session=s", "--task=t", "--harness=other"}), Entry("invalid role", []string{"plan", "--session=s", "--task=t", "--harness=codex", "--role=other"}), Entry("unknown flag", []string{"report", "--other"}), Entry("report task scope", []string{"report", "--task=t"}), Entry("extra positional", []string{"report", "extra"}), Entry("explicit false help", []string{"report", "--help=false"}), Entry("explicit false JSON", []string{"report", "--json=false"}), Entry("explicit true JSON", []string{"report", "--json=true"}), Entry("completion", []string{"completion"}), Entry("hidden completion", []string{"report", "__complete", "--session"}), Entry("hidden completion no descriptions", []string{"report", "__completeNoDesc", "--session"}), Entry("run missing prompt", []string{"run", "--session=s", "--task=t", "--harness=claude"}))
	// per docs/adr/0071-go-budget-command-candidate.md:31
	It("uses the last scalar flag while preserving default role and exported model", func() {
		root, cwd := fixture()
		args := []string{"plan", "--session=first", "--task", "t", "--session", "last", "--harness=codex", "--json"}
		extra := []string{"FACTORY_BUDGET_MODEL=literal/model;$()"}
		expected := budgetCLIOracle(root, cwd, args, extra...)
		out := budgetCLI(root, cwd, args, extra...)
		Expect(out.status).To(BeZero())
		Expect(budgetJSONComparable(budgetDecode([]byte(out.stdout)))).To(Equal(budgetJSONComparable(budgetDecode([]byte(expected.stdout)))))
		plan := budgetDecode([]byte(out.stdout)).(map[string]any)
		Expect(plan["session"]).To(Equal("last"))
		Expect(plan["role"]).To(Equal("implementer"))
		Expect(plan["model"]).To(Equal("literal/model;$()"))
	})
	// per docs/adr/0071-go-budget-command-candidate.md:65
	DescribeTable("emits exactly one disabled plan before opening a missing prompt or probing", func(jsonOutput bool) {
		root, cwd := fixture()
		args := []string{"run", "--session=s", "--task=t", "--harness=claude", "--prompt-file=PRIVATE_MISSING"}
		if jsonOutput {
			args = append(args, "--json")
		}
		baseline := budgetCLIOracle(root, cwd, args, "FACTORY_BUDGET_ENABLED=false")
		out := budgetCLI(root, cwd, args, "FACTORY_BUDGET_ENABLED=false")
		Expect(out.status).To(Equal(2))
		Expect(out.stderr).To(BeEmpty())
		if jsonOutput {
			Expect(budgetJSONComparable(budgetDecode([]byte(out.stdout)))).To(Equal(budgetJSONComparable(budgetDecode([]byte(baseline.stdout)))))
			Expect(strings.Count(out.stdout, "\n")).To(Equal(1))
		} else {
			Expect(out.stdout).To(Equal(baseline.stdout))
		}
		Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_MISSING"))
		_, err := os.Stat(filepath.Join(root, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("text", false), Entry("JSON", true))
})
var _ = Describe("G2 budget command populated history", func() {
	// per docs/adr/0071-go-budget-command-candidate.md:44
	DescribeTable("reports immutable history despite invalid execution settings", func(jsonOutput bool, session string) {
		root, cwd := fixture()
		known := budgetRow("known")
		unknown := budgetRow("unknown")
		unknown["session"] = "other"
		unknown["estimated_usd"] = nil
		unknown["tokens"] = nil
		unknown["complete"] = false
		writeFixture(filepath.Join(root, ".factory/budget.json"), budgetHistory(known, unknown), 0600)
		args := []string{"report"}
		if session != "" {
			args = append(args, "--session="+session)
		}
		if jsonOutput {
			args = append(args, "--json")
		}
		expected := budgetCLIOracle(root, cwd, args, "FACTORY_BUDGET_TIMEOUT_SECONDS=INVALID", "FACTORY_BUDGET_ENABLED=INVALID")
		out := budgetCLI(root, cwd, args, "FACTORY_BUDGET_TIMEOUT_SECONDS=INVALID", "FACTORY_BUDGET_ENABLED=INVALID")
		Expect(expected.status).To(BeZero())
		Expect(out.status).To(BeZero())
		Expect(out.stderr).To(BeEmpty())
		if jsonOutput {
			Expect(budgetJSONComparable(budgetDecode([]byte(out.stdout)))).To(Equal(budgetJSONComparable(budgetDecode([]byte(expected.stdout)))))
		} else {
			Expect(out.stdout).To(Equal(expected.stdout))
		}
		_, err := os.Stat(filepath.Join(root, ".factory/budget.lock"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("text all", false, ""), Entry("JSON all", true, ""), Entry("text selected", false, "session"), Entry("JSON selected", true, "session"), Entry("text empty", false, "missing"), Entry("JSON empty", true, "missing"))
	// per docs/adr/0071-go-budget-command-candidate.md:59
	DescribeTable("renders huge counters exactly", func(jsonOutput bool) {
		root, cwd := fixture()
		args := []string{"plan", "--session=s", "--task=t", "--harness=codex"}
		if jsonOutput {
			args = append(args, "--json")
		}
		extra := []string{"FACTORY_BUDGET_MAX_ATTEMPTS=123456789012345678901234567890", "FACTORY_BUDGET_MAX_SESSION_RUNS=999999999999999999999999999999", "FACTORY_BUDGET_TIMEOUT_SECONDS=1e-6"}
		expected := budgetCLIOracle(root, cwd, args, extra...)
		out := budgetCLI(root, cwd, args, extra...)
		Expect(expected.status).To(BeZero())
		Expect(out.status).To(BeZero())
		if jsonOutput {
			Expect(budgetJSONComparable(budgetDecode([]byte(out.stdout)))).To(Equal(budgetJSONComparable(budgetDecode([]byte(expected.stdout)))))
		} else {
			Expect(out.stdout).To(Equal(expected.stdout))
		}
	}, Entry("text", false), Entry("JSON", true))
})

func budgetCLILive(root, cwd string, args []string, extra ...string) cliResult {
	GinkgoHelper()
	budgetCommandCandidate(root)
	outputPath := filepath.Join(cwd, "budget-output")
	file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
	Expect(err).NotTo(HaveOccurred())
	defer file.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(root, "factory"), append([]string{"budget", "controller"}, args...)...) // #nosec G204 G702 -- test-owned binary and literal acceptance operands, no shell.
	cmd.Dir = cwd
	cmd.Env = append([]string{"FACTORY_BRIDGE_PROTOCOL=1", "FACTORY_BUDGET_ENABLED=true", "PATH=" + filepath.Join(root, "bin"), "GORACE=atexit_sleep_ms=0", "CONTROLLER_FIXTURE_LOG=" + filepath.Join(cwd, "native-calls.jsonl"), "BUDGET_FIXTURE_OUTPUT=" + outputPath, "BUDGET_FIXTURE_ORDER=" + filepath.Join(cwd, "plan-seen"), "BUDGET_FIXTURE_NATIVE=" + filepath.Join(root, "native"), "FACTORY_BUDGET_ROOT=" + root}, extra...)
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = file, &stderr
	err = cmd.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred())
	status := 0
	if err != nil {
		var exit *exec.ExitError
		Expect(errors.As(err, &exit)).To(BeTrue())
		status = exit.ExitCode()
	}
	raw, err := os.ReadFile(outputPath)
	Expect(err).NotTo(HaveOccurred())
	return cliResult{string(raw), stderr.String(), status}
}
func budgetCLIFixture() (string, string) {
	GinkgoHelper()
	controllerBuild()
	root, cwd := controllerFixture()
	for _, h := range []string{"codex", "claude", "opencode"} {
		writeFixture(filepath.Join(root, "native", h), controllerDriver, 0755)
		wrapper := "#!/bin/sh\nfor arg do\n  if [ \"$arg\" = --help ]; then exec \"$BUDGET_FIXTURE_NATIVE/" + h + "\" \"$@\"; fi\ndone\nif [ -s \"$BUDGET_FIXTURE_OUTPUT\" ]; then printf 'plan-before-stdin\\n' > \"$BUDGET_FIXTURE_ORDER\"; else exit 91; fi\nexec \"$BUDGET_FIXTURE_NATIVE/" + h + "\" \"$@\"\n"
		writeFixture(filepath.Join(root, "bin", h), []byte(wrapper), 0755)
	}
	writeFixture(filepath.Join(cwd, "prompt.txt"), []byte("PRIVATE_PROMPT\n"), 0600)
	return root, cwd
}

var _ = Describe("G2 budget command native output", func() {
	// per docs/adr/0071-go-budget-command-candidate.md:86
	DescribeTable("writes a live plan before native stdin and keeps JSON private", func(h string, jsonOutput bool) {
		root, cwd := budgetCLIFixture()
		args := []string{"run", "--session=s", "--task=t", "--harness=" + h, "--role=reviewer", "--prompt-file=prompt.txt"}
		if jsonOutput {
			args = append(args, "--json")
		}
		out := budgetCLILive(root, cwd, args)
		Expect(out.status).To(BeZero())
		Expect(out.stderr).To(BeEmpty())
		order, err := os.ReadFile(filepath.Join(cwd, "plan-seen"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(order)).To(Equal("plan-before-stdin\n"))
		raw, err := os.ReadFile(filepath.Join(root, ".factory/budget.json"))
		Expect(err).NotTo(HaveOccurred())
		history := budgetDecode(raw).(map[string]any)
		row := history["runs"].([]any)[0]
		if jsonOutput {
			lines := strings.Split(strings.TrimSuffix(out.stdout, "\n"), "\n")
			Expect(lines).To(HaveLen(2))
			plan := budgetDecode([]byte(lines[0])).(map[string]any)
			Expect(plan).To(HaveKey("configuration"))
			Expect(budgetDecode([]byte(lines[1]))).To(Equal(row))
			Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_"))
			Expect(out.stdout).NotTo(ContainSubstring("Response"))
		} else {
			Expect(out.stdout).To(HavePrefix("Budget plan: "))
			Expect(out.stdout).To(ContainSubstring("\nRun "))
			Expect(out.stdout).To(HaveSuffix("PRIVATE_ANSWER\n"))
			Expect(strings.Index(out.stdout, "\nRun ")).To(BeNumerically("<", strings.Index(out.stdout, "PRIVATE_ANSWER")))
		}
		Expect(string(raw)).NotTo(ContainSubstring("PRIVATE_"))
	}, Entry("Codex JSON", "codex", true), Entry("Claude JSON", "claude", true), Entry("OpenCode JSON", "opencode", true), Entry("Codex text", "codex", false), Entry("Claude text", "claude", false), Entry("OpenCode text", "opencode", false))
	// per docs/adr/0071-go-budget-command-candidate.md:63
	It("does not select an invalid scalar answer in JSON mode", func() {
		root, cwd := budgetCLIFixture()
		out := budgetCLILive(root, cwd, []string{"run", "--session=s", "--task=t", "--harness=claude", "--role=reviewer", "--prompt-file=prompt.txt", "--json"}, "CONTROLLER_FIXTURE_MODE=invalid-answer")
		Expect(out.status).To(BeZero())
		Expect(out.stderr).To(BeEmpty())
		Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_"))
		Expect(strings.Count(out.stdout, "\n")).To(Equal(2))
	})
	// per docs/adr/0071-go-budget-command-candidate.md:68
	It("emits no plan after native preflight refusal", func() {
		root, cwd := budgetCLIFixture()
		out := budgetCLILive(root, cwd, []string{"run", "--session=s", "--task=t", "--harness=claude", "--role=reviewer", "--prompt-file=prompt.txt"}, "CONTROLLER_FIXTURE_MODE=bad-help")
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).To(HavePrefix("factory budget: "))
		_, err := os.Stat(filepath.Join(root, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0071-go-budget-command-candidate.md:38
	It("returns a failed native status with two JSON metadata events", func() {
		root, cwd := budgetCLIFixture()
		out := budgetCLILive(root, cwd, []string{"run", "--session=s", "--task=t", "--harness=claude", "--role=reviewer", "--prompt-file=prompt.txt", "--json"}, "CONTROLLER_FIXTURE_MODE=nonzero")
		Expect(out.status).To(Equal(1))
		Expect(out.stderr).To(BeEmpty())
		lines := strings.Split(strings.TrimSuffix(out.stdout, "\n"), "\n")
		Expect(lines).To(HaveLen(2))
		Expect(budgetDecode([]byte(lines[1])).(map[string]any)["outcome"]).To(Equal("failed"))
	})
})

// per docs/adr/0071-go-budget-command-candidate.md:54
func budgetJSONComparable(value any) any {
	switch v := value.(type) {
	case json.Number:
		number, ok := new(big.Rat).SetString(string(v))
		Expect(ok).To(BeTrue())
		return struct{ Number string }{number.RatString()}
	case map[string]any:
		out := map[string]any{}
		for key, item := range v {
			out[key] = budgetJSONComparable(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = budgetJSONComparable(item)
		}
		return out
	default:
		return value
	}
}

var budgetCommandRaceBinary []byte

func budgetCommandCandidate(root string) {
	GinkgoHelper()
	if os.Getenv("FACTORY_BUDGET_COMMAND_TEST_RACE") != "1" || os.Getenv("FACTORY_BUDGET_COMMAND_BASELINE_BINARY") != "" {
		return
	}
	if budgetCommandRaceBinary == nil {
		dir, err := os.MkdirTemp("", "budget-command-race-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, dir)
		repo, err := filepath.Abs("..")
		Expect(err).NotTo(HaveOccurred())
		cmd := exec.Command("go", "build", "-race", "-o", filepath.Join(dir, "factory"), "./cmd/factory") // #nosec G204 -- test-owned race binary output and fixed package.
		cmd.Dir = repo
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "instrumented command build: %s", output)
		budgetCommandRaceBinary, err = os.ReadFile(filepath.Join(dir, "factory"))
		Expect(err).NotTo(HaveOccurred())
	}
	writeFixture(filepath.Join(root, "factory"), budgetCommandRaceBinary, 0755)
}

var _ = Describe("G2 budget command environment and literal values", func() {
	// per docs/adr/0071-go-budget-command-candidate.md:40
	It("uses caller cwd when the root environment is unset", func() {
		root, cwd := fixture()
		budgetCommandCandidate(root)
		writeFixture(filepath.Join(cwd, ".factory/budget.json"), budgetHistory(budgetRow("caller-history")), 0600)
		out := budgetCLIProcess("", cwd, filepath.Join(root, "factory"), []string{"budget", "controller", "report", "--json"})
		Expect(out.status).To(BeZero())
		report := budgetDecode([]byte(out.stdout)).(map[string]any)
		Expect(report["runs"].([]any)).To(HaveLen(1))
		Expect(report["runs"].([]any)[0].(map[string]any)["id"]).To(Equal("caller-history"))
	})
	// per docs/adr/0071-go-budget-command-candidate.md:26
	It("keeps a completion-looking prompt filename literal", func() {
		root, cwd := budgetCLIFixture()
		writeFixture(filepath.Join(cwd, "__completeNoDesc"), []byte("PRIVATE_PROMPT\n"), 0600)
		out := budgetCLILive(root, cwd, []string{"run", "--session=s", "--task=t", "--harness=claude", "--role=reviewer", "--prompt-file=__completeNoDesc", "--json"})
		Expect(out.status).To(BeZero())
		Expect(out.stderr).To(BeEmpty())
		Expect(strings.Count(out.stdout, "\n")).To(Equal(2))
	})
})
