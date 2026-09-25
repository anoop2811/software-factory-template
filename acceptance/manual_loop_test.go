package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func manualLoopCLI(root, cwd, checkout, action, harness string, extra ...string) cliResult {
	environment := append([]string{"FACTORY_LOOP_ROOT=" + checkout, "FACTORY_LOOP_CHECK_COMMAND=true"}, extra...)
	return loopProcess(cwd, filepath.Join(root, "factory"), []string{"loop", "manual", action, harness, "s", "t"}, environment)
}
func manualLoopOracle(root, cwd, checkout, action, harness string, extra ...string) cliResult {
	environment := append([]string{"FACTORY_BUDGET_ROOT=" + checkout, "FACTORY_LOOP_CHECK_COMMAND=true"}, extra...)
	return loopProcess(cwd, "python3", []string{"-B", filepath.Join(root, "oracle/loop.py"), action, "--harness", harness, "--session", "s", "--task", "t", "--mode", "manual", "--json"}, environment)
}
func manualComparable(input string) any {
	GinkgoHelper()
	value := budgetDecode([]byte(input))
	object, ok := value.(map[string]any)
	Expect(ok).To(BeTrue())
	delete(object, "owner_pid")
	delete(object, "started_at")
	delete(object, "elapsed_seconds")
	if evidence, ok := object["evidence"].([]any); ok {
		for _, entry := range evidence {
			if item, ok := entry.(map[string]any); ok {
				delete(item, "duration_seconds")
			}
		}
	}
	return budgetJSONComparable(object)
}

var _ = Describe("G2 manual loop core", func() {
	// per docs/adr/0075-go-manual-loop-controller.md:30
	It("plans manual checks with both features disabled without writing state", func() {
		root, cwd, checkout := loopFixture()
		oracle := manualLoopOracle(root, cwd, checkout, "plan", "codex")
		Expect(oracle.status).To(BeZero(), "%+v", oracle)
		actual := manualLoopCLI(root, cwd, checkout, "plan", "codex")
		Expect(actual.status).To(BeZero(), "%+v", actual)
		Expect(actual.stderr).To(BeEmpty())
		Expect(manualComparable(actual.stdout)).To(Equal(manualComparable(oracle.stdout)))
		_, err := os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0075-go-manual-loop-controller.md:81
	DescribeTable("runs one deterministic check and preserves manual outcome semantics", func(harness, check string, status int) {
		root, cwd, checkout := loopFixture()
		extra := []string{"FACTORY_LOOP_CHECK_COMMAND=" + check}
		oracle := manualLoopOracle(root, cwd, checkout, "run", harness, extra...)
		Expect(oracle.status).To(Equal(status), "%+v", oracle)
		Expect(os.Rename(filepath.Join(checkout, ".factory"), filepath.Join(root, "oracle-checkpoint"))).To(Succeed())
		actual := manualLoopCLI(root, cwd, checkout, "run", harness, extra...)
		Expect(actual.status).To(Equal(status), "%+v", actual)
		Expect(actual.stdout).NotTo(BeEmpty())
		Expect(actual.stderr).To(BeEmpty())
		Expect(manualComparable(actual.stdout)).To(Equal(manualComparable(oracle.stdout)))
	}, Entry("passing Codex metadata", "codex", "printf PRIVATE_CHECK_OUTPUT; exit 0", 0), Entry("failing Claude metadata", "claude", "printf PRIVATE_CHECK_OUTPUT >&2; exit 7", 2))
})

func manualRecord(out cliResult) map[string]any {
	GinkgoHelper()
	Expect(out.stdout).NotTo(BeEmpty(), "%+v", out)
	record, ok := budgetDecode([]byte(out.stdout)).(map[string]any)
	Expect(ok).To(BeTrue())
	return record
}
func manualStored(checkout string) map[string]any {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join(checkout, ".factory/loops.json"))
	Expect(err).NotTo(HaveOccurred())
	return budgetDecode(data).(map[string]any)
}
func manualNumber(value any) float64 {
	GinkgoHelper()
	number, ok := value.(json.Number)
	Expect(ok).To(BeTrue())
	result, err := number.Float64()
	Expect(err).NotTo(HaveOccurred())
	return result
}

var _ = Describe("G2 manual loop execution", func() {
	// per docs/adr/0075-go-manual-loop-controller.md:65
	DescribeTable("uses reviewer role, checkout cwd and empty stdin without probing native clients", func(harness string) {
		root, cwd, checkout := loopFixture()
		bin := filepath.Join(root, "no-native")
		marker := filepath.Join(cwd, "native-called")
		for _, name := range []string{"codex", "claude", "opencode"} {
			writeFixture(filepath.Join(bin, name), []byte("#!/bin/sh\nprintf called > \"$MANUAL_NATIVE_MARKER\"\nexit 19\n"), 0700)
		}
		check := `[[ "$FACTORY_AGENT_ROLE" == reviewer ]] && [[ "$PWD" == "$MANUAL_EXPECT_ROOT" ]] || exit 17; if IFS= read -r line; then exit 18; fi; printf PRIVATE_CHECK_OUTPUT; printf PRIVATE_CHECK_ERROR >&2`
		actual := manualLoopCLI(root, cwd, checkout, "run", harness, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "MANUAL_EXPECT_ROOT="+checkout, "MANUAL_NATIVE_MARKER="+marker, "FACTORY_AGENT_ROLE=implementer", "FACTORY_LOOP_CHECK_COMMAND="+check)
		Expect(actual.status).To(BeZero(), "%+v", actual)
		Expect(actual.stderr).To(BeEmpty())
		record := manualRecord(actual)
		Expect(record["harness"]).To(Equal(harness))
		Expect(record["status"]).To(Equal("stopped"))
		Expect(record["outcome"]).To(Equal("manual_passed"))
		Expect(record["phase"]).To(Equal("finished"))
		Expect(record["uncertain"]).To(BeFalse())
		Expect(record["process_pid"]).To(BeNil())
		Expect(record["budget_runs"]).To(BeEmpty())
		Expect(record["attempts"]).To(Equal(json.Number("0")))
		Expect(record["no_progress"]).To(Equal(json.Number("0")))
		evidence := record["evidence"].([]any)
		Expect(evidence).To(HaveLen(1))
		item := evidence[0].(map[string]any)
		Expect(item["kind"]).To(Equal("check"))
		Expect(item["harness"]).To(Equal(harness))
		Expect(item["role"]).To(Equal("deterministic"))
		Expect(item["command"]).To(Equal(check))
		Expect(item["exit_code"]).To(Equal(json.Number("0")))
		Expect(item["snapshot"]).To(Equal(record["snapshot"]))
		Expect(manualNumber(item["duration_seconds"])).To(BeNumerically(">=", 0))
		Expect(record).NotTo(HaveKey("output"))
		Expect(item).NotTo(HaveKey("output"))
		_, err := os.Stat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue())
		_, err = os.Stat(filepath.Join(checkout, ".factory/budget.json"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("Codex metadata", "codex"), Entry("Claude metadata", "claude"), Entry("OpenCode metadata", "opencode"))
	// per docs/adr/0075-go-manual-loop-controller.md:75
	It("retains no emitted check output in terminal JSON or stored history", func() {
		root, cwd, checkout := loopFixture()
		check := `printf '%s%s' PRIVATE_ OUTPUT; printf '%s%s' SECRET_ STDERR >&2`
		actual := manualLoopCLI(root, cwd, checkout, "run", "opencode", "FACTORY_LOOP_CHECK_COMMAND="+check)
		Expect(actual.status).To(BeZero(), "%+v", actual)
		Expect(actual.stdout + actual.stderr).NotTo(ContainSubstring("PRIVATE_OUTPUT"))
		Expect(actual.stdout + actual.stderr).NotTo(ContainSubstring("SECRET_STDERR"))
		data, err := os.ReadFile(filepath.Join(checkout, ".factory/loops.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).NotTo(ContainSubstring("PRIVATE_OUTPUT"))
		Expect(string(data)).NotTo(ContainSubstring("SECRET_STDERR"))
	})
	// per docs/adr/0075-go-manual-loop-controller.md:43
	It("refuses duplicate run without launching a second check", func() {
		root, cwd, checkout := loopFixture()
		marker := filepath.Join(cwd, "checks")
		extra := []string{"FACTORY_LOOP_CHECK_COMMAND=printf x >> \"$MANUAL_MARKER\"", "MANUAL_MARKER=" + marker}
		Expect(manualLoopCLI(root, cwd, checkout, "run", "codex", extra...).status).To(BeZero())
		before, err := os.ReadFile(filepath.Join(checkout, ".factory/loops.json"))
		Expect(err).NotTo(HaveOccurred())
		loopRefused(manualLoopCLI(root, cwd, checkout, "run", "codex", extra...))
		after, err := os.ReadFile(filepath.Join(checkout, ".factory/loops.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		checks, err := os.ReadFile(marker)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(checks)).To(Equal("x"))
	})
	// per docs/adr/0075-go-manual-loop-controller.md:79
	DescribeTable("hands off after successful checks mutate their evidence basis", func(kind, check string) {
		root, cwd, checkout := loopFixture()
		actual := manualLoopCLI(root, cwd, checkout, "run", "claude", "FACTORY_LOOP_CHECK_COMMAND="+check)
		Expect(actual.status).To(Equal(2), "%+v", actual)
		record := manualRecord(actual)
		Expect(record["outcome"]).To(Equal("handoff"))
		Expect(record["uncertain"]).To(BeFalse())
		Expect(record["process_pid"]).To(BeNil())
		Expect(record["evidence"]).To(HaveLen(1))
		Expect(record["stop_reason"]).NotTo(BeEmpty())
		data, err := os.ReadFile(filepath.Join(checkout, kind))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("changed"))
	}, Entry("ordinary source", "source.txt", "printf changed > source.txt"), Entry("governing policy", "AGENTS.md", "printf changed > AGENTS.md"), Entry("effective configuration", "factory.yaml", "printf changed > factory.yaml"))
})

var _ = Describe("G2 manual loop resume", func() {
	// per docs/adr/0075-go-manual-loop-controller.md:43
	// per docs/adr/0075-go-manual-loop-controller.md:57
	It("matches repeated fresh Python resumes while preserving history and consumed allowance", func() {
		root, cwd, checkout := loopFixture()
		first := manualLoopCLI(root, cwd, checkout, "run", "opencode")
		Expect(first.status).To(BeZero())
		path := filepath.Join(checkout, ".factory/loops.json")
		history := manualStored(checkout)
		history["top_extension"] = map[string]any{"preserve": true}
		record := history["runs"].([]any)[0].(map[string]any)
		record["extension"] = map[string]any{"preserve": json.Number("9007199254740993")}
		record["attempts"] = json.Number("7")
		record["no_progress"] = json.Number("3")
		record["elapsed_seconds"] = json.Number("2.0")
		record["reserved_seconds"] = json.Number("17.5")
		record["started_at"] = "original-time"
		originalBaseline := record["baseline"]
		modified, err := json.Marshal(history)
		Expect(err).NotTo(HaveOccurred())
		writeFixture(path, modified, 0600)
		for attempt := range 2 {
			before, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			oracle := manualLoopOracle(root, cwd, checkout, "resume", "opencode")
			Expect(oracle.status).To(BeZero(), "%+v", oracle)
			writeFixture(path, before, 0600)
			actual := manualLoopCLI(root, cwd, checkout, "resume", "opencode")
			Expect(actual.status).To(BeZero(), "%+v", actual)
			Expect(manualComparable(actual.stdout)).To(Equal(manualComparable(oracle.stdout)))
			next := manualRecord(actual)
			Expect(next["started_at"]).To(Equal("original-time"))
			Expect(next["attempts"]).To(Equal(json.Number("7")))
			Expect(next["no_progress"]).To(Equal(json.Number("3")))
			Expect(manualNumber(next["elapsed_seconds"])).To(BeNumerically(">", 2))
			Expect(manualNumber(next["reserved_seconds"])).To(Equal(17.5))
			Expect(next["baseline"]).To(Equal(originalBaseline))
			Expect(next["evidence"]).To(HaveLen(attempt + 2))
			Expect(manualStored(checkout)["top_extension"]).To(Equal(history["top_extension"]))
		}
	})
	// per docs/adr/0075-go-manual-loop-controller.md:43
	DescribeTable("refuses stale resume without consuming another check", func(kind string) {
		root, cwd, checkout := loopFixture()
		marker := filepath.Join(cwd, "checks")
		extra := []string{"FACTORY_LOOP_CHECK_COMMAND=printf x >> \"$MANUAL_MARKER\"", "MANUAL_MARKER=" + marker}
		Expect(manualLoopCLI(root, cwd, checkout, "run", "codex", extra...).status).To(BeZero())
		harness := "codex"
		switch kind {
		case "source":
			writeFixture(filepath.Join(checkout, "source.txt"), []byte("changed"), 0600)
		case "safety":
			writeFixture(filepath.Join(checkout, "AGENTS.md"), []byte("changed"), 0600)
		case "policy":
			extra = append(extra, "FACTORY_LOOP_MAX_ATTEMPTS=3")
		case "harness":
			harness = "claude"
		case "prompt":
			history := manualStored(checkout)
			history["runs"].([]any)[0].(map[string]any)["prompt"] = "changed"
			data, err := json.Marshal(history)
			Expect(err).NotTo(HaveOccurred())
			writeFixture(filepath.Join(checkout, ".factory/loops.json"), data, 0600)
		}
		before, err := os.ReadFile(filepath.Join(checkout, ".factory/loops.json"))
		Expect(err).NotTo(HaveOccurred())
		loopRefused(manualLoopCLI(root, cwd, checkout, "resume", harness, extra...))
		after, err := os.ReadFile(filepath.Join(checkout, ".factory/loops.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		checks, err := os.ReadFile(marker)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(checks)).To(Equal("x"))
	}, Entry("changed source", "source"), Entry("changed safety", "safety"), Entry("changed policy", "policy"), Entry("different harness", "harness"), Entry("changed empty-prompt identity", "prompt"))
	// per docs/adr/0075-go-manual-loop-controller.md:43
	It("refuses absent resume identity without creating a checkpoint", func() {
		root, cwd, checkout := loopFixture()
		loopRefused(manualLoopCLI(root, cwd, checkout, "resume", "codex"))
		_, err := os.Stat(filepath.Join(checkout, ".factory/loops.json"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

var _ = Describe("G2 manual loop admission", func() {
	// per docs/adr/0075-go-manual-loop-controller.md:32
	DescribeTable("matches manual blocker ordering and never executes the configured check", func(kind string) {
		root, cwd, checkout := loopFixture()
		extra := []string{}
		if kind == "empty check" || kind == "combined" {
			extra = append(extra, "FACTORY_LOOP_CHECK_COMMAND= ")
		}
		if kind == "active loop" || kind == "uncertain loop" || kind == "combined" {
			record := checkpointWith("status", `"active"`)
			if kind == "uncertain loop" {
				record = checkpointWith("uncertain", "true")
			}
			writeFixture(filepath.Join(checkout, ".factory/loops.json"), []byte(checkpointHistory(record)), 0600)
		}
		if kind == "active budget" || kind == "combined" {
			row := budgetRow("active")
			row["status"] = "active"
			row["outcome"] = "running"
			row["complete"] = false
			row["tokens"] = nil
			row["estimated_usd"] = nil
			row["exit_code"] = nil
			row["ended_at"] = nil
			writeFixture(filepath.Join(checkout, ".factory/budget.json"), budgetHistory(row), 0600)
		}
		oracle := manualLoopOracle(root, cwd, checkout, "plan", "codex", extra...)
		Expect(oracle.status).To(Equal(2), "%+v", oracle)
		actual := manualLoopCLI(root, cwd, checkout, "plan", "codex", extra...)
		Expect(actual.status).To(Equal(2), "%+v", actual)
		Expect(manualComparable(actual.stdout)).To(Equal(manualComparable(oracle.stdout)))
		_, err := os.Stat(filepath.Join(checkout, ".factory/loops.lock"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("missing check", "empty check"), Entry("active loop", "active loop"), Entry("uncertain loop", "uncertain loop"), Entry("active shared budget", "active budget"), Entry("combined blockers ordered", "combined"))
	// per docs/adr/0075-go-manual-loop-controller.md:41
	DescribeTable("preserves corrupt accounting without launching or repairing", func(name string) {
		root, cwd, checkout := loopFixture()
		path := filepath.Join(checkout, ".factory", name)
		original := []byte("PRIVATE_CORRUPT")
		writeFixture(path, original, 0600)
		loopRefused(manualLoopCLI(root, cwd, checkout, "run", "codex"))
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(original))
	}, Entry("loop history", "loops.json"), Entry("budget history", "budget.json"))
	// per docs/adr/0075-go-manual-loop-controller.md:24
	DescribeTable("rejects invalid private operands before writing state", func(args []string) {
		root, cwd, checkout := loopFixture()
		actual := loopProcess(cwd, filepath.Join(root, "factory"), append([]string{"loop", "manual"}, args...), []string{"FACTORY_LOOP_ROOT=" + checkout})
		loopRefused(actual)
		_, err := os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("missing", []string{"run"}), Entry("invalid harness", []string{"run", "PRIVATE", "s", "t"}), Entry("invalid identity", []string{"run", "codex", "../PRIVATE", "t"}), Entry("extra", []string{"run", "codex", "s", "t", "PRIVATE"}), Entry("help", []string{"--help"}), Entry("unknown action", []string{"PRIVATE", "codex", "s", "t"}))
	// per docs/adr/0075-go-manual-loop-controller.md:59
	It("caps representable duration conversion for large finite limits", func() {
		root, cwd, checkout := loopFixture()
		actual := manualLoopCLI(root, cwd, checkout, "run", "codex", "FACTORY_LOOP_TIMEOUT_SECONDS=1e100", "FACTORY_LOOP_CHECK_TIMEOUT_SECONDS=1e100")
		Expect(actual.status).To(BeZero(), "%+v", actual)
		Expect(manualRecord(actual)["outcome"]).To(Equal("manual_passed"))
	})
	// per docs/adr/0075-go-manual-loop-controller.md:73
	It("does not launch a check after the total allowance is exhausted", func() {
		root, cwd, checkout := loopFixture()
		marker := filepath.Join(cwd, "should-not-launch")
		actual := manualLoopCLI(root, cwd, checkout, "run", "codex", "FACTORY_LOOP_TIMEOUT_SECONDS=1e-12", "FACTORY_LOOP_CHECK_COMMAND=printf x > \"$MANUAL_MARKER\"", "MANUAL_MARKER="+marker)
		Expect(actual.status).To(Equal(2))
		_, err := os.Stat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

var _ = Describe("G2 manual loop supervised failures", func() {
	// per docs/adr/0075-go-manual-loop-controller.md:73
	DescribeTable("bounds checks by the smaller timeout and confirms process exit", func(total, checkTimeout string) {
		root, cwd, checkout := loopFixture()
		marker := filepath.Join(cwd, "check-pid")
		out := manualLoopCLI(root, cwd, checkout, "run", "codex", "FACTORY_LOOP_TIMEOUT_SECONDS="+total, "FACTORY_LOOP_CHECK_TIMEOUT_SECONDS="+checkTimeout, "FACTORY_LOOP_CHECK_COMMAND=printf '%s' \"$$\" > \"$MANUAL_PID\"; exec /bin/sleep 30", "MANUAL_PID="+marker)
		Expect(out.status).To(Equal(2), "%+v", out)
		record := manualRecord(out)
		Expect(record["outcome"]).To(Equal("handoff"))
		Expect(record["uncertain"]).To(BeFalse())
		Expect(record["process_pid"]).To(BeNil())
		evidence := record["evidence"].([]any)
		Expect(evidence).To(HaveLen(1))
		Expect(evidence[0].(map[string]any)["outcome"]).To(Equal("timeout"))
		raw, err := os.ReadFile(marker)
		Expect(err).NotTo(HaveOccurred())
		pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		Expect(err).NotTo(HaveOccurred())
		Expect(syscall.Kill(pid, 0)).To(Equal(syscall.ESRCH))
	}, Entry("check timeout", "30", "0.2"), Entry("total remaining time", "2", "30"))
	// per docs/adr/0075-go-manual-loop-controller.md:74
	DescribeTable("bounds combined captured output without storing it", func(size string, success bool) {
		root, cwd, checkout := loopFixture()
		check := "/usr/bin/head -c " + size + " /dev/zero"
		if !success {
			check += "; exec /bin/sleep 30"
		}
		out := manualLoopCLI(root, cwd, checkout, "run", "claude", "FACTORY_LOOP_CHECK_COMMAND="+check)
		if success {
			Expect(out.status).To(BeZero(), "%+v", out)
		} else {
			Expect(out.status).To(Equal(2), "%+v", out)
		}
		record := manualRecord(out)
		Expect(record["process_pid"]).To(BeNil())
		Expect(record["uncertain"]).To(BeFalse())
		evidence := record["evidence"].([]any)
		Expect(evidence).To(HaveLen(1))
		if success {
			Expect(record["outcome"]).To(Equal("manual_passed"))
		} else {
			Expect(evidence[0].(map[string]any)["outcome"]).To(Equal("output_limit"))
			Expect(record["outcome"]).To(Equal("handoff"))
		}
		Expect(len(out.stdout)).To(BeNumerically("<", 16000))
	}, Entry("exact16MiB", "16777216", true), Entry("one byte excess", "16777217", false))
	// per docs/adr/0075-go-manual-loop-controller.md:70
	It("cleans a descendant holding stdout after its leader exits", func() {
		root, cwd, checkout := loopFixture()
		marker := filepath.Join(cwd, "descendant-pid")
		out := manualLoopCLI(root, cwd, checkout, "run", "opencode", "FACTORY_LOOP_CHECK_COMMAND=/bin/sleep 30 & printf '%s' \"$!\" > \"$MANUAL_PID\"; exit 0", "MANUAL_PID="+marker)
		Expect(out.status).To(BeZero(), "%+v", out)
		record := manualRecord(out)
		Expect(record["uncertain"]).To(BeFalse())
		Expect(record["process_pid"]).To(BeNil())
		raw, err := os.ReadFile(marker)
		Expect(err).NotTo(HaveOccurred())
		pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() bool {
			if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
				return true
			}
			status := loopProcess(cwd, "ps", []string{"-o", "stat=", "-p", strconv.Itoa(pid)}, nil)
			return strings.HasPrefix(strings.TrimSpace(status.stdout), "Z")
		}, time.Second).Should(BeTrue(), "descendant must no longer run; an exited zombie may await init reaping")
	})
})

var _ = Describe("G2 manual loop process signals", func() {
	// per docs/adr/0075-go-manual-loop-controller.md:90
	// per docs/adr/0075-go-manual-loop-controller.md:98
	DescribeTable("persists a certain interrupted handoff after cleaning its observed child", func(signal syscall.Signal) {
		root, cwd, checkout := loopFixture()
		marker := filepath.Join(cwd, "check-pid")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		command := exec.CommandContext(ctx, filepath.Join(root, "factory"), "loop", "manual", "run", "codex", "s", "t") // #nosec G204 G702 -- fixed compiled private command and test-owned fixture path.
		command.Dir = cwd
		for _, item := range os.Environ() {
			if !strings.HasPrefix(item, "FACTORY_") {
				command.Env = append(command.Env, item)
			}
		}
		command.Env = append(command.Env, "FACTORY_BRIDGE_PROTOCOL=1", "FACTORY_LOOP_ROOT="+checkout, "FACTORY_LOOP_CHECK_COMMAND=if read -r line; then exit 19; fi; printf '%s' \"$$\" > \"$MANUAL_PID\"; exec /bin/sleep 30", "MANUAL_PID="+marker, "GORACE=atexit_sleep_ms=0")
		var output, diagnostic bytes.Buffer
		command.Stdout = &output
		command.Stderr = &diagnostic
		Expect(command.Start()).To(Succeed())
		done := make(chan error, 1)
		go func() { defer close(done); done <- command.Wait() }()
		pid := 0
		DeferCleanup(func() {
			if pid > 0 {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			}
			cancel()
			Eventually(done, 3*time.Second).Should(BeClosed())
		})
		var raw []byte
		Eventually(func() error { var err error; raw, err = os.ReadFile(marker); return err }, 4*time.Second).Should(Succeed())
		var err error
		pid, err = strconv.Atoi(strings.TrimSpace(string(raw)))
		Expect(err).NotTo(HaveOccurred())
		Expect(syscall.Kill(pid, 0)).To(Succeed())
		Eventually(func() bool {
			data, err := os.ReadFile(filepath.Join(checkout, ".factory/loops.json"))
			if err != nil {
				return false
			}
			var history map[string]any
			if json.Unmarshal(data, &history) != nil {
				return false
			}
			runs, ok := history["runs"].([]any)
			if !ok || len(runs) != 1 {
				return false
			}
			row := runs[0].(map[string]any)
			return row["process_pid"] == float64(pid) && row["status"] == "active" && row["phase"] == "check"
		}, 2*time.Second).Should(BeTrue())
		Expect(command.Process.Signal(signal)).To(Succeed())
		var waitErr error
		Eventually(done, 7*time.Second).Should(Receive(&waitErr))
		Expect(ctx.Err()).NotTo(HaveOccurred())
		var exited *exec.ExitError
		Expect(errors.As(waitErr, &exited)).To(BeTrue())
		Expect(exited.ExitCode()).To(Equal(2))
		Expect(diagnostic.String()).To(BeEmpty())
		record := manualRecord(cliResult{output.String(), diagnostic.String(), 2})
		Expect(record["outcome"]).To(Equal("interrupted"))
		Expect(record["uncertain"]).To(BeFalse())
		Expect(record["process_pid"]).To(BeNil())
		Expect(record["status"]).To(Equal("stopped"))
		Expect(syscall.Kill(pid, 0)).To(Equal(syscall.ESRCH))
		pid = 0
		Expect(manualStored(checkout)["runs"].([]any)[0]).To(Equal(budgetDecode(output.Bytes())))
	}, Entry("SIGINT", syscall.SIGINT), Entry("SIGTERM", syscall.SIGTERM))
})

var _ = Describe("G2 manual loop validation order", func() {
	// per docs/adr/0075-go-manual-loop-controller.md:126
	DescribeTable("refuses corrupt checkpoint before probing configured ERE patterns", func(action string) {
		root, cwd, checkout := loopFixture()
		history := filepath.Join(checkout, ".factory/loops.json")
		writeFixture(history, []byte("PRIVATE_CORRUPT"), 0600)
		bin := filepath.Join(root, "grep-fixture")
		marker := filepath.Join(cwd, "grep-called")
		writeFixture(filepath.Join(bin, "grep"), []byte("#!/bin/sh\nprintf called > \"$MANUAL_GREP_MARKER\"\nexit 1\n"), 0700)
		extra := []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"), "FACTORY_LOOP_TEST_PATTERNS=x", "MANUAL_GREP_MARKER=" + marker}
		oracle := manualLoopOracle(root, cwd, checkout, action, "codex", extra...)
		Expect(oracle.status).To(Equal(2))
		_, err := os.Stat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue(), "immutable oracle must not probe malformed checkpoint")
		loopRefused(manualLoopCLI(root, cwd, checkout, action, "codex", extra...))
		_, err = os.Stat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue(), "configuration ERE probe must not precede checkpoint validation")
	}, Entry("plan", "plan"), Entry("run", "run"), Entry("resume", "resume"))
})
