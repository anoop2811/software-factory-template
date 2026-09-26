package acceptance_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/commandenv"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const commandEnvironmentBaseline = "c8f8d34edbc14df5655fcbf0aab7eea46ced0295"

func configuredFixture() (string, string, string) {
	GinkgoHelper()
	root, _, checkout := loopFixture()
	scripts := filepath.Join(root, "wrapper/scripts")
	for _, name := range []string{"codex", "claude", "opencode"} {
		writeFixture(filepath.Join(root, "no-native", name), []byte("#!/bin/sh\nprintf unexpected-native-probe >&2\nexit 97\n"), 0700)
	}
	for _, name := range []string{"factory-budget.sh", "factory-loop.sh", "lib/budget-config.sh", "lib/config.sh", "lib/roles.sh", "lib/budget.py", "lib/loop.py", "lib/budget_adapters.py"} {
		command := exec.Command("git", "show", commandEnvironmentBaseline+":scripts/"+name) // #nosec G204 -- immutable revision and explicit fixture source allowlist.
		data, err := command.Output()
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(scripts, name), data, 0600)
	}
	return root, checkout, scripts
}

func configuredSafeEnvironment(root string, env []string) []string {
	path := os.Getenv("PATH")
	for _, entry := range env {
		if strings.HasPrefix(entry, "PATH=") {
			path = strings.TrimPrefix(entry, "PATH=")
		}
	}
	result := append([]string(nil), env...)
	return append(result, "PATH="+filepath.Join(root, "no-native")+string(os.PathListSeparator)+path)
}

func configuredCommand(root, cwd, kind string, args, env []string) cliResult {
	return loopProcess(cwd, filepath.Join(root, "factory"), append([]string{kind, "configured"}, args...), configuredSafeEnvironment(root, env))
}

var _ = Describe("G2 configured command core", func() {
	// per docs/adr/0077-go-command-environment.md:27
	DescribeTable("matches immutable wrapper command results without state writes", func(kind string, args []string, status int) {
		root, checkout, scripts := configuredFixture()
		writeFixture(filepath.Join(checkout, "factory.yaml"), []byte("check_command: true\n"), 0600)
		oracle := loopProcess(checkout, "bash", append([]string{filepath.Join(scripts, "factory-"+kind+".sh")}, args...), configuredSafeEnvironment(root, nil))
		Expect(oracle.status).To(Equal(status), "oracle: %+v", oracle)
		actual := configuredCommand(root, checkout, kind, args, nil)
		Expect(actual.status).To(Equal(status), "candidate: %+v", actual)
		Expect(actual.stderr).To(BeEmpty())
		Expect(budgetJSONComparable(budgetDecode([]byte(actual.stdout)))).To(Equal(budgetJSONComparable(budgetDecode([]byte(oracle.stdout)))))
		_, err := os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("disabled budget plan", "budget", []string{"plan", "--harness", "codex", "--session", "s", "--task", "t", "--json"}, 2),
		Entry("empty budget report", "budget", []string{"report", "--json"}, 0),
		Entry("manual loop plan", "loop", []string{"plan", "--harness", "claude", "--session", "s", "--task", "t", "--json"}, 0))
})

func configuredParity(root, cwd, scripts, kind string, args, env []string) map[string]any {
	GinkgoHelper()
	oracle := loopProcess(cwd, "bash", append([]string{filepath.Join(scripts, "factory-"+kind+".sh")}, args...), configuredSafeEnvironment(root, env))
	actual := configuredCommand(root, cwd, kind, args, env)
	Expect(actual.status).To(Equal(oracle.status), "oracle:%+v candidate:%+v", oracle, actual)
	Expect(actual.stderr).To(BeEmpty(), "%+v", actual)
	Expect(manualComparable(actual.stdout)).To(Equal(manualComparable(oracle.stdout)))
	return budgetDecode([]byte(actual.stdout)).(map[string]any)
}
func configuredPlanArgs(harness, role string) []string {
	return []string{"plan", "--harness", harness, "--role", role, "--session", "s", "--task", "t", "--json"}
}
func configuredModels() string {
	var out strings.Builder
	for _, h := range []string{"codex", "claude", "opencode"} {
		for _, tier := range []string{"frontier", "default", "economy"} {
			fmt.Fprintf(&out, "%s_%s_model: %s/%s\n", h, tier, h, tier)
		}
	}
	return out.String()
}

var _ = Describe("G2 configured command model composition", func() {
	for _, harness := range []string{"codex", "claude", "opencode"} {
		for _, role := range []string{"spec-writer", "implementer", "refactorer", "reviewer", "wiki-maintainer"} {
			for _, profile := range []string{"standard", "economy"} {
				// per docs/adr/0077-go-command-environment.md:63
				It("resolves "+harness+" "+role+" under "+profile, func() {
					root, checkout, scripts := configuredFixture()
					writeFixture(filepath.Join(checkout, "factory.yaml"), []byte(configuredModels()+"cost_profile: "+profile+"\n"), 0600)
					result := configuredParity(root, checkout, scripts, "budget", configuredPlanArgs(harness, role), []string{"FACTORY_BUDGET_MODEL=WRONG_INHERITED"})
					tier := "default"
					if role == "reviewer" || role == "spec-writer" {
						tier = "frontier"
					}
					if profile == "economy" && (role == "refactorer" || role == "wiki-maintainer") {
						tier = "economy"
					}
					Expect(result["model"]).To(Equal(harness + "/" + tier))
				})
			}
		}
	}
	// per docs/adr/0077-go-command-environment.md:57
	DescribeTable("preserves caller presence and parsed legacy precedence", func(yaml, legacy string, env []string, expected string) {
		root, checkout, scripts := configuredFixture()
		writeFixture(filepath.Join(checkout, "factory.yaml"), []byte(yaml), 0600)
		writeFixture(filepath.Join(checkout, "factory.config"), []byte(legacy), 0600)
		result := configuredParity(root, checkout, scripts, "budget", configuredPlanArgs("codex", "implementer"), env)
		Expect(result["model"]).To(Equal(expected))
	}, Entry("YAML beats legacy", "codex_default_model: yaml/model\n", "CODEX_DEFAULT_MODEL=legacy/model\n", nil, "yaml/model"),
		Entry("legacy fallback", "", "CODEX_DEFAULT_MODEL=legacy/model\n", nil, "legacy/model"),
		Entry("caller beats YAML", "codex_default_model: yaml/model\n", "CODEX_DEFAULT_MODEL=legacy/model\n", []string{"CODEX_DEFAULT_MODEL=caller/model"}, "caller/model"),
		Entry("empty caller stays empty", "codex_default_model: yaml/model\n", "CODEX_DEFAULT_MODEL=legacy/model\n", []string{"CODEX_DEFAULT_MODEL="}, ""),
		Entry("model trailing newlines trimmed", "", "", []string{"CODEX_DEFAULT_MODEL=caller/model\n\n"}, "caller/model"),
		Entry("empty profile behaves standard", "cost_profile: economy\ncodex_default_model: d\n", "", []string{"COST_PROFILE="}, "d"))
	// per docs/adr/0077-go-command-environment.md:63
	DescribeTable("scans literal operands without changing parser forwarding", func(operands []string, expected string) {
		root, checkout, scripts := configuredFixture()
		writeFixture(filepath.Join(checkout, "factory.yaml"), []byte(configuredModels()), 0600)
		args := append([]string{"plan", "--session", "s", "--task", "t", "--json"}, operands...)
		result := configuredParity(root, checkout, scripts, "budget", args, nil)
		Expect(result["model"]).To(Equal(expected))
	}, Entry("equals forms", []string{"--harness=claude", "--role=reviewer"}, "claude/frontier"),
		Entry("last harness and role win", []string{"--harness", "codex", "--harness=opencode", "--role", "reviewer", "--role=implementer"}, "opencode/default"),
		Entry("default role", []string{"--harness", "codex"}, "codex/default"),
		Entry("abbreviated harness not scanned", []string{"--harn", "codex", "--role", "reviewer"}, ""),
		Entry("abbreviated role not scanned", []string{"--harness", "codex", "--rol", "reviewer"}, "codex/default"))
})

var _ = Describe("G2 configured command settings", func() {
	// per docs/adr/0077-go-command-environment.md:53
	It("overwrites all eight incoming budget settings with YAML and defaults", func() {
		root, checkout, scripts := configuredFixture()
		writeFixture(filepath.Join(checkout, "factory.yaml"), []byte("budget_enabled: true\nbudget_max_attempts: 7\nbudget_max_session_runs: 9\nbudget_timeout_seconds: 11\nbudget_session_seconds: 25\nbudget_max_concurrent: 3\nbudget_estimated_usd: 0.75\nbudget_action: warn\n"), 0600)
		var env []string
		for _, key := range []string{"ENABLED", "MAX_ATTEMPTS", "MAX_SESSION_RUNS", "TIMEOUT_SECONDS", "SESSION_SECONDS", "MAX_CONCURRENT", "ESTIMATED_USD", "ACTION"} {
			env = append(env, "FACTORY_BUDGET_"+key+"=PRIVATE_INVALID")
		}
		result := configuredParity(root, checkout, scripts, "budget", configuredPlanArgs("codex", "implementer"), env)
		configuration := result["configuration"].(map[string]any)
		Expect(configuration["enabled"]).To(BeTrue())
		Expect(configuration["action"]).To(Equal("warn"))
		Expect(fmt.Sprint(configuration["max_attempts"])).To(Equal("7"))
		Expect(fmt.Sprint(configuration["estimated_usd"])).To(Equal("0.75"))
		writeFixture(filepath.Join(checkout, "factory.yaml"), nil, 0600)
		defaults := configuredParity(root, checkout, scripts, "budget", configuredPlanArgs("codex", "implementer"), env)
		Expect(defaults["configuration"].(map[string]any)["enabled"]).To(BeFalse())
	})
	// per docs/adr/0077-go-command-environment.md:75
	DescribeTable("composes loop settings and check fallback", func(loopCheck string, expected string) {
		root, checkout, scripts := configuredFixture()
		yaml := "check_command: true\nloop_check_command: " + loopCheck + "\nloop_enabled: true\nloop_max_attempts: 4\nloop_timeout_seconds: 27\nloop_check_timeout_seconds: 8\nloop_no_progress_limit: 3\ntest_file_patterns: _test\nprotected_paths: AGENTS.md\n"
		writeFixture(filepath.Join(checkout, "factory.yaml"), []byte(yaml), 0600)
		result := configuredParity(root, checkout, scripts, "loop", []string{"plan", "--harness", "codex", "--session", "s", "--task", "t", "--json"}, []string{"FACTORY_LOOP_ENABLED=INVALID", "FACTORY_LOOP_CHECK_COMMAND=INVALID", "FACTORY_LOOP_MAX_ATTEMPTS=INVALID"})
		configuration := result["configuration"].(map[string]any)
		Expect(configuration["check_command"]).To(Equal(expected))
		Expect(configuration["enabled"]).To(BeTrue())
		Expect(fmt.Sprint(configuration["max_attempts"])).To(Equal("4"))
	}, Entry("fallback when empty", "", "true"), Entry("explicit command", "printf literal", "printf literal"))
	// per docs/adr/0077-go-command-environment.md:43
	DescribeTable("selects relative and nested configuration independently of root", func(kind string) {
		root, checkout, scripts := configuredFixture()
		cwd := checkout
		var env []string
		selected := filepath.Join(checkout, "factory.yaml")
		switch kind {
		case "nested":
			cwd = filepath.Join(checkout, "nested")
			Expect(os.Mkdir(cwd, 0700)).To(Succeed())
		case "relative":
			cwd = filepath.Join(checkout, "nested")
			Expect(os.Mkdir(cwd, 0700)).To(Succeed())
			selected = filepath.Join(cwd, "custom.yaml")
			env = append(env, "FACTORY_CONFIG=custom.yaml")
		case "absolute":
			selected = filepath.Join(root, "chosen.yaml")
			env = append(env, "FACTORY_CONFIG="+selected)
		case "non-git":
			cwd = filepath.Join(root, "outside")
			Expect(os.Mkdir(cwd, 0700)).To(Succeed())
			selected = filepath.Join(cwd, "factory.yaml")
		}
		writeFixture(selected, []byte("codex_default_model: selected/model\n"), 0600)
		result := configuredParity(root, cwd, scripts, "budget", configuredPlanArgs("codex", "implementer"), append(env, "FACTORY_BUDGET_ROOT=/wrong/inherited"))
		Expect(result["model"]).To(Equal("selected/model"))
	}, Entry("nested", "nested"), Entry("relative", "relative"), Entry("absolute", "absolute"), Entry("non-git", "non-git"))
})

var _ = Describe("G2 configured command effects", func() {
	// per docs/adr/0077-go-command-environment.md:57
	It("keeps legacy and YAML metacharacters inert and inherited secrets private", func() {
		root, checkout, scripts := configuredFixture()
		marker := filepath.Join(root, "EXECUTED")
		literal := "$(touch " + marker + "); `touch " + marker + "`"
		writeFixture(filepath.Join(checkout, "factory.yaml"), []byte("codex_default_model: "+literal+"\n"), 0600)
		writeFixture(filepath.Join(checkout, "factory.config"), []byte("touch "+marker+"\nCODEX_DEFAULT_MODEL='"+literal+"'\n"), 0600)
		result := configuredParity(root, checkout, scripts, "budget", configuredPlanArgs("codex", "implementer"), []string{"UNRELATED_CREDENTIAL=PRIVATE_CREDENTIAL"})
		Expect(result["model"]).To(Equal(literal))
		Expect(fmt.Sprint(result)).NotTo(ContainSubstring("PRIVATE_CREDENTIAL"))
		_, err := os.Stat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0077-go-command-environment.md:104
	DescribeTable("refuses preparation errors even before help with no state effects", func(kind string) {
		root, checkout, _ := configuredFixture()
		path := filepath.Join(root, "PRIVATE_INVALID_DIRECTORY")
		writeFixture(path, []byte("budget_enabled: true\n"), 0600)
		Expect(os.Chmod(path, 0000)).To(Succeed())
		DeferCleanup(func() { Expect(os.Chmod(path, 0600)).To(Succeed()) })
		if os.Geteuid() == 0 {
			Skip("requires an unprivileged caller for read refusal")
		}
		out := configuredCommand(root, checkout, kind, []string{"--help"}, []string{"FACTORY_CONFIG=" + path})
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).NotTo(BeEmpty())
		Expect(out.stderr).NotTo(ContainSubstring(path))
		_, err := os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("budget", "budget"), Entry("loop", "loop"))
	// per docs/adr/0077-go-command-environment.md:32
	DescribeTable("passes help and invalid arguments through after safe preparation", func(kind string, args []string, status int) {
		root, checkout, _ := configuredFixture()
		out := configuredCommand(root, checkout, kind, args, []string{"UNRELATED_CREDENTIAL=PRIVATE_CREDENTIAL"})
		Expect(out.status).To(Equal(status))
		Expect(out.stdout + out.stderr).NotTo(ContainSubstring("PRIVATE_CREDENTIAL"))
		if status == 0 {
			Expect(out.stdout).To(ContainSubstring("Usage:"))
			Expect(out.stderr).To(BeEmpty())
		} else {
			Expect(out.stdout).To(BeEmpty())
		}
		_, err := os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("budget help", "budget", []string{"--help"}, 0), Entry("loop help", "loop", []string{"--help"}, 0), Entry("budget bad command", "budget", []string{"PRIVATE_BAD_COMMAND"}, 2), Entry("loop bad command", "loop", []string{"PRIVATE_BAD_COMMAND"}, 2))
	// per docs/adr/0077-go-command-environment.md:127
	DescribeTable("runs a configured manual check for each metadata harness", func(harness string) {
		root, checkout, _ := configuredFixture()
		writeFixture(filepath.Join(checkout, "factory.yaml"), []byte("check_command: test \"$FACTORY_AGENT_ROLE\" = reviewer && printf \"$CONFIGURED_CHECK_PAYLOAD\"\n"), 0600)
		out := configuredCommand(root, checkout, "loop", []string{"run", "--harness", harness, "--session", "s", "--task", "t", "--json"}, []string{"CONFIGURED_CHECK_PAYLOAD=PRIVATE_CHECK"})
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stderr).To(BeEmpty())
		Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_CHECK"))
		Expect(manualRecord(out)["outcome"]).To(Equal("manual_passed"))
	}, Entry("Codex", "codex"), Entry("Claude", "claude"), Entry("OpenCode", "opencode"))
	// per docs/adr/0077-go-command-environment.md:127
	DescribeTable("carries configured implementer and reviewer models through real fake-client invocations", func(harness string) {
		root, cwd, checkout, environment := boundedFixture()
		yaml := configuredModels() + "budget_enabled: true\nbudget_max_attempts: 6\nbudget_max_session_runs: 6\nloop_enabled: true\nloop_max_attempts: 2\ncheck_command: true\n"
		writeFixture(filepath.Join(checkout, "factory.yaml"), []byte(yaml), 0600)
		out := configuredCommand(root, checkout, "loop", boundedArgs("run", harness, cwd), environment)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stderr).To(BeEmpty())
		Expect(manualRecord(out)["outcome"]).To(Equal("approved"))
		calls := boundedCalls(cwd)
		Expect(calls).To(HaveLen(2))
		Expect(calls[0]["role"]).To(Equal("implementer"))
		Expect(calls[1]["role"]).To(Equal("reviewer"))
		Expect(calls[0]["argv"]).To(ContainElement(harness + "/default"))
		Expect(calls[1]["argv"]).To(ContainElement(harness + "/frontier"))
		for _, name := range []string{"loops.json", "budget.json"} {
			contents, err := os.ReadFile(filepath.Join(checkout, ".factory", name))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(contents)).NotTo(ContainSubstring("PRIVATE_TASK"))
		}
	}, Entry("Codex", "codex"), Entry("Claude", "claude"), Entry("OpenCode", "opencode"))
})

var configuredCaptureKeys = []string{
	"FACTORY_BUDGET_ROOT", "FACTORY_BUDGET_ENABLED", "FACTORY_BUDGET_MAX_ATTEMPTS", "FACTORY_BUDGET_MAX_SESSION_RUNS", "FACTORY_BUDGET_TIMEOUT_SECONDS", "FACTORY_BUDGET_SESSION_SECONDS", "FACTORY_BUDGET_MAX_CONCURRENT", "FACTORY_BUDGET_ESTIMATED_USD", "FACTORY_BUDGET_ACTION", "FACTORY_BUDGET_MODEL",
	"COST_PROFILE", "CODEX_DEFAULT_MODEL", "CODEX_FRONTIER_MODEL", "CODEX_ECONOMY_MODEL", "CLAUDE_DEFAULT_MODEL", "CLAUDE_FRONTIER_MODEL", "OPENCODE_DEFAULT_MODEL", "OPENCODE_FRONTIER_MODEL",
	"FACTORY_LOOP_ENABLED", "FACTORY_LOOP_MAX_ATTEMPTS", "FACTORY_LOOP_TIMEOUT_SECONDS", "FACTORY_LOOP_CHECK_TIMEOUT_SECONDS", "FACTORY_LOOP_NO_PROGRESS_LIMIT", "FACTORY_LOOP_CHECK_COMMAND", "FACTORY_LOOP_TEST_PATTERNS", "FACTORY_LOOP_PROTECTED_PATHS", "FACTORY_LOOP_CONFIG_PATH",
	"FACTORY_LOOP_CODEX_IMPLEMENTER_MODEL", "FACTORY_LOOP_CODEX_REVIEWER_MODEL", "FACTORY_LOOP_CLAUDE_IMPLEMENTER_MODEL", "FACTORY_LOOP_CLAUDE_REVIEWER_MODEL", "FACTORY_LOOP_OPENCODE_IMPLEMENTER_MODEL", "FACTORY_LOOP_OPENCODE_REVIEWER_MODEL",
}

func configuredCompose(ctx context.Context, kind string, args []string, env map[string]string) (map[string]string, error) {
	if kind == "loop" {
		return commandenv.Loop(ctx, env)
	}
	return commandenv.Budget(ctx, args, env)
}

var _ = Describe("G2 configured command composer isolation", func() {
	// per docs/adr/0077-go-command-environment.md:81
	DescribeTable("clones supplied values and matches only allowlisted immutable wrapper exports", func(kind string) {
		root, checkout, scripts := configuredFixture()
		python := loopProcess(checkout, "python3", []string{"-B", "-c", "import sys; print(sys.executable)"}, nil)
		Expect(python.status).To(BeZero())
		keys, err := json.Marshal(configuredCaptureKeys)
		Expect(err).NotTo(HaveOccurred())
		bin := filepath.Join(root, "capture")
		writeFixture(filepath.Join(bin, "python3"), []byte("#!"+strings.TrimSpace(python.stdout)+"\nimport os,json\nprint(json.dumps({k:os.environ.get(k,'') for k in "+string(keys)+"}))\n"), 0700)
		writeFixture(filepath.Join(bin, "git"), []byte("#!/bin/sh\nprintf '%s\\n' \"$CONFIGURED_TEST_ROOT\"\n"), 0700)
		configPath := filepath.Join(checkout, "factory.yaml")
		writeFixture(configPath, []byte(configuredModels()+"check_command: true\nreview_model: fixture/reviewer\n"), 0600)
		env := map[string]string{"PATH": bin + string(os.PathListSeparator) + os.Getenv("PATH"), "FACTORY_CONFIG": configPath, "CONFIGURED_TEST_ROOT": checkout, "UNRELATED_CREDENTIAL": "PRIVATE_PRESERVED", "CODEX_DEFAULT_MODEL": ""}
		snapshot := maps.Clone(env)
		global := os.Environ()
		args := configuredPlanArgs("codex", "implementer")
		actual, err := configuredCompose(context.Background(), kind, args, env)
		Expect(err).NotTo(HaveOccurred())
		Expect(env).To(Equal(snapshot))
		Expect(os.Environ()).To(Equal(global))
		Expect(actual["UNRELATED_CREDENTIAL"]).To(Equal("PRIVATE_PRESERVED"))
		actual["UNRELATED_CREDENTIAL"] = "changed"
		Expect(env["UNRELATED_CREDENTIAL"]).To(Equal("PRIVATE_PRESERVED"))
		var extra []string
		for key, value := range env {
			extra = append(extra, key+"="+value)
		}
		oracle := loopProcess(checkout, "bash", append([]string{filepath.Join(scripts, "factory-"+kind+".sh")}, args...), extra)
		Expect(oracle.status).To(BeZero(), "%+v", oracle)
		expected := budgetDecode([]byte(oracle.stdout)).(map[string]any)
		for _, key := range configuredCaptureKeys {
			Expect(actual[key]).To(Equal(expected[key]), "key %s", key)
		}
		Expect(oracle.stdout).NotTo(ContainSubstring("PRIVATE_PRESERVED"))
	}, Entry("budget", "budget"), Entry("loop", "loop"))
	// per docs/adr/0077-go-command-environment.md:47
	DescribeTable("preserves failed Git stdout before distinct root and config fallbacks", func(output string, status int) {
		root, _, _ := configuredFixture()
		bin := filepath.Join(root, "git-fixture")
		writeFixture(filepath.Join(bin, "git"), []byte(fmt.Sprintf("#!/bin/sh\nprintf '%%b' '%s'\nexit %d\n", strings.ReplaceAll(output, "\x00", "\\000"), status)), 0700)
		cwd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		env := map[string]string{"PATH": bin, "FACTORY_CONFIG": ""}
		actual, err := commandenv.Loop(context.Background(), env)
		Expect(err).NotTo(HaveOccurred())
		normalized := strings.ReplaceAll(output, "\x00", "")
		expectedRoot := normalized
		expectedConfig := normalized
		if status != 0 {
			expectedRoot += cwd + "\n"
			expectedConfig += ".\n"
		}
		expectedRoot = strings.TrimRight(expectedRoot, "\n")
		expectedConfig = strings.TrimRight(expectedConfig, "\n") + "/factory.yaml"
		Expect(actual["FACTORY_BUDGET_ROOT"]).To(Equal(expectedRoot))
		Expect(actual["FACTORY_LOOP_CONFIG_PATH"]).To(Equal(expectedConfig))
	}, Entry("failure with partial stdout", "partial\n", 1), Entry("success trailing newlines", "/missing/root\n\n", 0), Entry("discarded NUL bytes", "part\x00ial\n", 1))
	// per docs/adr/0077-go-command-environment.md:89
	DescribeTable("bounds Git probes and refuses without a partial environment", func(body string, parentTimeout time.Duration, identity bool) {
		root := GinkgoT().TempDir()
		bin := filepath.Join(root, "bin")
		writeFixture(filepath.Join(bin, "git"), []byte("#!/bin/sh\n"+body), 0700)
		env := map[string]string{"PATH": bin, "FACTORY_CONFIG": filepath.Join(root, "missing.yaml")}
		snapshot := maps.Clone(env)
		ctx, cancel := context.WithTimeout(context.Background(), parentTimeout)
		defer cancel()
		started := time.Now()
		result, err := commandenv.Loop(ctx, env)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(env).To(Equal(snapshot))
		Expect(time.Since(started)).To(BeNumerically("<", 7*time.Second))
		if identity {
			Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
		}
	}, Entry("parent deadline", "exec /bin/sleep 30\n", 100*time.Millisecond, true), Entry("private deadline", "exec /bin/sleep 30\n", 9*time.Second, false), Entry("combined output overflow", "/usr/bin/head -c 1100000 /dev/zero >&2\n", 9*time.Second, false))
})

var _ = Describe("G2 configured command probe ownership", func() {
	// per docs/adr/0077-go-command-environment.md:89
	It("refuses a nonzero Git leader whose descendant retains the output pipe", func() {
		root := GinkgoT().TempDir()
		bin := filepath.Join(root, "bin")
		marker := filepath.Join(root, "child")
		writeFixture(filepath.Join(bin, "git"), []byte("#!/bin/sh\n/bin/sleep 30 &\nchild=$!\nprintf '%s' \"$child\" > \"$CONFIGURED_CHILD_MARKER\"\nexit 1\n"), 0700)
		childPID := func() int {
			data, err := os.ReadFile(marker)
			if err != nil {
				return 0
			}
			pid, _ := strconv.Atoi(string(data))
			return pid
		}
		DeferCleanup(func() {
			if pid := childPID(); pid > 0 {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		})
		result, err := commandenv.Loop(context.Background(), map[string]string{"PATH": bin, "CONFIGURED_CHILD_MARKER": marker, "FACTORY_CONFIG": filepath.Join(root, "missing.yaml")})
		Expect(childPID()).To(BeNumerically(">", 0), "fake leader published descendant identity before exiting")
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Eventually(func() bool {
			out := loopProcess(root, "/bin/ps", []string{"-o", "stat=", "-p", strconv.Itoa(childPID())}, nil)
			return out.status != 0 || strings.HasPrefix(strings.TrimSpace(out.stdout), "Z")
		}, 2*time.Second).Should(BeTrue(), "probe descendant must be gone or exited")
	})
})

var _ = Describe("G2 configured command absent PATH qualification", func() {
	// per docs/adr/0077-go-command-environment.md:95
	It("refuses an absent captured PATH without using the global executable search", func() {
		env := map[string]string{"FACTORY_CONFIG": filepath.Join(GinkgoT().TempDir(), "missing.yaml")}
		before := maps.Clone(env)
		result, err := commandenv.Budget(context.Background(), configuredPlanArgs("codex", "implementer"), env)
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(env).To(Equal(before))
	})
})

var _ = Describe("G2 configured command protocol boundaries", func() {
	// per docs/adr/0077-go-command-environment.md:37
	It("refuses unknown private routes before Git discovery", func() {
		root, checkout, _ := configuredFixture()
		bin := filepath.Join(root, "bin")
		marker := filepath.Join(root, "git-called")
		writeFixture(filepath.Join(bin, "git"), []byte("#!/bin/sh\nprintf called > \"$CONFIGURED_GIT_MARKER\"\nexit 1\n"), 0700)
		out := loopProcess(checkout, filepath.Join(root, "factory"), []string{"loop", "unknown-configured-route"}, []string{"PATH=" + bin, "CONFIGURED_GIT_MARKER=" + marker})
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
		_, err := os.Stat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0077-go-command-environment.md:95
	It("admits an explicitly empty PATH with ordinary missing-Git fallback", func() {
		cwd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		result, err := commandenv.Budget(context.Background(), nil, map[string]string{"PATH": "", "FACTORY_CONFIG": filepath.Join(GinkgoT().TempDir(), "missing.yaml")})
		Expect(err).NotTo(HaveOccurred())
		Expect(result["FACTORY_BUDGET_ROOT"]).To(Equal(cwd))
		Expect(result["FACTORY_BUDGET_MODEL"]).To(BeEmpty())
	})
})
