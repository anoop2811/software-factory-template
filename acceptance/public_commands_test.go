package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func publicCommand(root, cwd, kind string, args, environment []string) cliResult {
	// env removes the private protocol that loopProcess supplies for older fixtures.
	return loopProcess(cwd, "/usr/bin/env", append([]string{"-u", "FACTORY_BRIDGE_PROTOCOL", filepath.Join(root, "factory"), kind}, args...), configuredSafeEnvironment(root, environment))
}

var _ = Describe("G2 public native command core", func() {
	// per docs/adr/0078-go-public-budget-loop.md:29
	DescribeTable("runs without colocated scripts or a private protocol", func(kind string, args []string, status int) {
		root, checkout, scripts := configuredFixture()
		writeFixture(filepath.Join(checkout, "factory.yaml"), []byte("check_command: true\n"), 0600)
		oracle := loopProcess(checkout, "bash", append([]string{filepath.Join(scripts, "factory-"+kind+".sh")}, args...), configuredSafeEnvironment(root, nil))
		Expect(oracle.status).To(Equal(status), "%+v", oracle)
		out := publicCommand(root, checkout, kind, args, nil)
		Expect(out.status).To(Equal(status), "%+v", out)
		Expect(out.stderr).To(BeEmpty())
		Expect(manualComparable(out.stdout)).To(Equal(manualComparable(oracle.stdout)))
		_, err := os.Stat(filepath.Join(root, "scripts"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		_, err = os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("disabled budget plan", "budget", configuredPlanArgs("codex", "implementer"), 2), Entry("empty report", "budget", []string{"report", "--json"}, 0), Entry("manual loop plan", "loop", []string{"plan", "--harness", "claude", "--session", "s", "--task", "t", "--json"}, 0))
})

func publicPoison(root string) string {
	GinkgoHelper()
	marker := filepath.Join(root, "FALLBACK_EXECUTED")
	for _, kind := range []string{"budget", "loop"} {
		writeFixture(filepath.Join(root, "scripts", "factory-"+kind+".sh"), []byte("#!/bin/sh\nprintf called > '"+marker+"'\nprintf POISON_FALLBACK >&2\nexit 99\n"), 0700)
	}
	return marker
}
func publicNoPoison(marker string) {
	GinkgoHelper()
	_, err := os.Stat(marker)
	Expect(os.IsNotExist(err)).To(BeTrue())
}

var _ = Describe("G2 public native command forwarding", func() {
	// per docs/adr/0078-go-public-budget-loop.md:29
	DescribeTable("preserves leaf status and diagnostics without falling back to present scripts", func(kind string, args []string, status int) {
		root, checkout, _ := configuredFixture()
		marker := publicPoison(root)
		writeFixture(filepath.Join(checkout, "factory.yaml"), []byte("check_command: true\n"), 0600)
		expected := configuredCommand(root, checkout, kind, args, nil)
		actual := publicCommand(root, checkout, kind, args, nil)
		Expect(actual.status).To(Equal(status), "%+v", actual)
		Expect(actual).To(Equal(expected))
		publicNoPoison(marker)
	}, Entry("budget help", "budget", []string{"--help"}, 0), Entry("loop help", "loop", []string{"--help"}, 0),
		Entry("budget missing operands", "budget", []string{"plan"}, 2), Entry("loop missing operands", "loop", []string{"run"}, 2),
		Entry("budget unknown option", "budget", []string{"report", "--PRIVATE_UNKNOWN"}, 2), Entry("loop unknown option", "loop", []string{"status", "--PRIVATE_UNKNOWN"}, 2),
		Entry("literal equals and abbreviations", "budget", []string{"plan", "--harn=codex", "--session=s", "--task=t", "--role=reviewer", "--json"}, 2),
		Entry("enabled independent status", "budget", []string{"report", "--json"}, 0))
	// per docs/adr/0078-go-public-budget-loop.md:63
	DescribeTable("prints preparation failures once with the public boundary prefix", func(kind string) {
		root, checkout, _ := configuredFixture()
		marker := publicPoison(root)
		config := filepath.Join(root, "PRIVATE_UNREADABLE")
		writeFixture(config, []byte("budget_enabled: true\n"), 0600)
		Expect(os.Chmod(config, 0000)).To(Succeed())
		DeferCleanup(func() { Expect(os.Chmod(config, 0600)).To(Succeed()) })
		if os.Geteuid() == 0 {
			Skip("requires unprivileged read refusal")
		}
		out := publicCommand(root, checkout, kind, []string{"--help"}, []string{"FACTORY_CONFIG=" + config})
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).To(Equal("factory: cannot prepare command environment\n"))
		publicNoPoison(marker)
		_, err := os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("budget", "budget"), Entry("loop", "loop"))
	// per docs/adr/0078-go-public-budget-loop.md:25
	It("refuses an unknown public token without preparing configuration", func() {
		root, checkout, _ := configuredFixture()
		bin := filepath.Join(root, "bin")
		marker := filepath.Join(root, "GIT_CALLED")
		writeFixture(filepath.Join(bin, "git"), []byte("#!/bin/sh\nprintf called > '"+marker+"'\nexit 99\n"), 0700)
		out := publicCommand(root, checkout, "unknown-native", nil, []string{"PATH=" + bin})
		Expect(out).To(Equal(cliResult{"", "factory: unknown command 'unknown-native' (try: factory help)\n", 2}))
		publicNoPoison(marker)
	})
	// per docs/adr/0078-go-public-budget-loop.md:26
	It("keeps private protocol precedence over a public-looking budget operand", func() {
		root, checkout, _ := configuredFixture()
		marker := publicPoison(root)
		out := loopProcess(checkout, filepath.Join(root, "factory"), []string{"budget", "report", "--json"}, nil)
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).To(ContainSubstring("factory bridge:"))
		publicNoPoison(marker)
	})
	// per docs/adr/0078-go-public-budget-loop.md:83
	DescribeTable("runs public manual checks and reads their status without native clients", func(harness string) {
		root, checkout, _ := configuredFixture()
		marker := publicPoison(root)
		writeFixture(filepath.Join(checkout, "factory.yaml"), []byte("check_command: true\n"), 0600)
		args := []string{"run", "--harness", harness, "--session", "s", "--task", "t", "--json"}
		out := publicCommand(root, checkout, "loop", args, nil)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(manualRecord(out)["outcome"]).To(Equal("manual_passed"))
		statusArgs := []string{"status", "--session", "s", "--task", "t", "--json"}
		Expect(publicCommand(root, checkout, "loop", statusArgs, nil)).To(Equal(configuredCommand(root, checkout, "loop", statusArgs, nil)))
		publicNoPoison(marker)
	}, Entry("Codex", "codex"), Entry("Claude", "claude"), Entry("OpenCode", "opencode"))
	// per docs/adr/0078-go-public-budget-loop.md:83
	DescribeTable("executes public bounded checks with distinct model tiers and no script fallback", func(harness string) {
		root, cwd, checkout, env := boundedFixture()
		marker := publicPoison(root)
		writeFixture(filepath.Join(checkout, "factory.yaml"), []byte(configuredModels()+"budget_enabled: true\nbudget_max_attempts: 6\nbudget_max_session_runs: 6\nloop_enabled: true\ncheck_command: true\n"), 0600)
		out := publicCommand(root, checkout, "loop", boundedArgs("run", harness, cwd), env)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(manualRecord(out)["outcome"]).To(Equal("approved"))
		calls := boundedCalls(cwd)
		Expect(calls).To(HaveLen(2))
		Expect(calls[0]["argv"]).To(ContainElement(harness + "/default"))
		Expect(calls[1]["argv"]).To(ContainElement(harness + "/frontier"))
		publicNoPoison(marker)
	}, Entry("Codex", "codex"), Entry("Claude", "claude"), Entry("OpenCode", "opencode"))
})

var _ = Describe("G2 public native command failures", func() {
	// per docs/adr/0078-go-public-budget-loop.md:34
	DescribeTable("propagates admitted native failure without another invocation or script", func(kind string, status int) {
		root, cwd, checkout, env := boundedFixture()
		marker := publicPoison(root)
		writeFixture(filepath.Join(checkout, "factory.yaml"), []byte(configuredModels()+"budget_enabled: true\nbudget_max_attempts: 6\nbudget_max_session_runs: 6\nloop_enabled: true\ncheck_command: true\n"), 0600)
		env = append(env, "BOUNDED_SCENARIO=native_failure")
		if status == 1 {
			path := filepath.Join(root, "bin/claude")
			script, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			writeFixture(path, []byte(strings.Replace(string(script), "sys.exit(7)", "sys.exit(1)", 1)), 0700)
		}
		args := boundedArgs("run", "claude", cwd)
		if kind == "budget" {
			args = []string{"run", "--harness", "claude", "--session", "s", "--task", "t", "--prompt-file", filepath.Join(cwd, "prompt.txt"), "--json"}
		}
		out := publicCommand(root, checkout, kind, args, env)
		adapterStatus := 2
		if kind == "budget" {
			adapterStatus = 1
		}
		Expect(out.status).To(Equal(adapterStatus), "%+v", out)
		ledger, err := os.ReadFile(filepath.Join(checkout, ".factory/budget.json"))
		Expect(err).NotTo(HaveOccurred())
		row := budgetDecode(ledger).(map[string]any)["runs"].([]any)[0].(map[string]any)
		nativeStatus := 7
		if status == 1 {
			nativeStatus = 1
		}
		Expect(row["exit_code"]).To(Equal(json.Number(strconv.Itoa(nativeStatus))))
		Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_IMPLEMENTER_RESPONSE"))
		Expect(boundedCalls(cwd)).To(HaveLen(1))
		publicNoPoison(marker)
	}, Entry("budget retains native exit seven", "budget", 7), Entry("budget native exit one", "budget", 1), Entry("loop handoff status", "loop", 2))
	// per docs/adr/0078-go-public-budget-loop.md:90
	DescribeTable("cancels its observed native child and persists metadata without retry or fallback", func(kind string, signal syscall.Signal) {
		root, cwd, checkout, env := boundedFixture()
		marker := publicPoison(root)
		writeFixture(filepath.Join(checkout, "factory.yaml"), []byte(configuredModels()+"budget_enabled: true\nbudget_max_attempts: 6\nbudget_max_session_runs: 6\nloop_enabled: true\ncheck_command: true\n"), 0600)
		args := boundedArgs("run", "claude", cwd)
		if kind == "budget" {
			args = []string{"run", "--harness", "claude", "--session", "s", "--task", "t", "--prompt-file", filepath.Join(cwd, "prompt.txt"), "--json"}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		command := exec.CommandContext(ctx, filepath.Join(root, "factory"), append([]string{kind}, args...)...) // #nosec G204 G702 -- test-built executable, fixed public route, test-owned literal arguments.
		command.Dir = checkout
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "FACTORY_") {
				command.Env = append(command.Env, entry)
			}
		}
		command.Env = append(command.Env, env...)
		command.Env = append(command.Env, "BOUNDED_SCENARIO=stall", "GORACE=atexit_sleep_ms=0")
		var out, diagnostic bytes.Buffer
		command.Stdout = &out
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
		// Fake client writes its call only after stdin EOF, which follows durable PID publication.
		Eventually(func() bool {
			data, err := os.ReadFile(filepath.Join(cwd, "calls.jsonl"))
			return err == nil && strings.HasSuffix(string(data), "\n")
		}, 5*time.Second).Should(BeTrue())
		calls := boundedCalls(cwd)
		Expect(calls).To(HaveLen(1))
		pid = int(calls[0]["pid"].(float64))
		Expect(syscall.Kill(pid, 0)).To(Succeed())
		Expect(command.Process.Signal(signal)).To(Succeed())
		var waitErr error
		Eventually(done, 8*time.Second).Should(Receive(&waitErr))
		Expect(ctx.Err()).NotTo(HaveOccurred())
		var exited *exec.ExitError
		Expect(errors.As(waitErr, &exited)).To(BeTrue())
		expectedStatus := 2
		if kind == "budget" {
			expectedStatus = 130
		}
		Expect(exited.ExitCode()).To(Equal(expectedStatus), "stderr:%s", diagnostic.String())
		Expect(diagnostic.String()).To(BeEmpty())
		Expect(out.String()).NotTo(ContainSubstring("PRIVATE_TASK"))
		Expect(boundedCalls(cwd)).To(HaveLen(1))
		publicNoPoison(marker)
		ledger, err := os.ReadFile(filepath.Join(checkout, ".factory/budget.json"))
		Expect(err).NotTo(HaveOccurred())
		records := budgetDecode(ledger).(map[string]any)["runs"].([]any)
		Expect(records).To(HaveLen(1))
		row := records[0].(map[string]any)
		Expect(row["outcome"]).To(Equal("interrupted"))
		Expect(row["status"]).To(Equal("completed"))
		Eventually(func() bool {
			state := loopProcess(cwd, "/bin/ps", []string{"-o", "stat=", "-p", strconv.Itoa(pid)}, nil)
			return state.status != 0 || strings.HasPrefix(strings.TrimSpace(state.stdout), "Z")
		}, time.Second).Should(BeTrue())
	}, Entry("budget SIGINT", "budget", syscall.SIGINT), Entry("budget SIGTERM", "budget", syscall.SIGTERM), Entry("loop SIGINT", "loop", syscall.SIGINT), Entry("loop SIGTERM", "loop", syscall.SIGTERM))
})
