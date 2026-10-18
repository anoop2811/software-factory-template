package acceptance_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Independent wire allowlists, rather than the implementation's declarations.
// per docs/adr/0099-native-factory-orchestration.md:131
var orchestrationPaletteKeys = []string{"C_RESET", "C_BOLD", "C_DIM", "C_YELLOW", "C_GREEN", "C_RED"}

var orchestrationBudgetKeys = []string{
	"FACTORY_BUDGET_ROOT", "FACTORY_BUDGET_ENABLED", "FACTORY_BUDGET_MAX_ATTEMPTS",
	"FACTORY_BUDGET_MAX_SESSION_RUNS", "FACTORY_BUDGET_TIMEOUT_SECONDS", "FACTORY_BUDGET_SESSION_SECONDS",
	"FACTORY_BUDGET_MAX_CONCURRENT", "FACTORY_BUDGET_ESTIMATED_USD", "FACTORY_BUDGET_ACTION",
}

var orchestrationConfigKeys = []string{
	"COST_PROFILE", "MODEL_PROVIDER", "OPENCODE_FRONTIER_MODEL", "OPENCODE_DEFAULT_MODEL", "OPENCODE_ECONOMY_MODEL",
	"CLAUDE_FRONTIER_MODEL", "CLAUDE_DEFAULT_MODEL", "CLAUDE_ECONOMY_MODEL",
	"CODEX_FRONTIER_MODEL", "CODEX_DEFAULT_MODEL", "CODEX_ECONOMY_MODEL",
	"REVIEW_LANE", "REVIEW_MODEL", "REVIEW_API_KEY_SECRET", "REVIEW_REASONING_EFFORT", "REVIEW_OPENROUTER_PROVIDER", "REVIEW_MAX_TOKENS",
}

// Exercise the real executable with isolated helper/configuration controls.
// per docs/adr/0099-native-factory-orchestration.md:155
func orchestrationHelperProcess(cwd string, environment map[string]string, executable string, args ...string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...) // #nosec G204 -- evaluator-owned executable and literal argv, including deliberate hostile data.
	cmd.Dir = cwd
	for _, entry := range os.Environ() {
		key := strings.SplitN(entry, "=", 2)[0]
		if strings.HasPrefix(key, "FACTORY_") || member(key, orchestrationConfigKeys...) || member(key, orchestrationPaletteKeys...) || member(key, "NO_COLOR", "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_CONFIG", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM", "GIT_CONFIG_NOSYSTEM", "BASH_ENV", "ENV", "LC_ALL") {
			continue
		}
		if _, overridden := environment[key]; !overridden {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "LC_ALL=C", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	for key, value := range environment {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred(), "helper process timed out")
	status := 0
	if err != nil {
		var exit *exec.ExitError
		Expect(errors.As(err, &exit)).To(BeTrue(), "helper process failed to launch: %v", err)
		status = exit.ExitCode()
	}
	return cliResult{stdout.String(), stderr.String(), status}
}

func orchestrationHelperBridge(root, cwd string, environment map[string]string, args ...string) cliResult {
	GinkgoHelper()
	private := map[string]string{"FACTORY_BRIDGE_PROTOCOL": "1"}
	for key, value := range environment {
		private[key] = value
	}
	return orchestrationHelperProcess(cwd, private, filepath.Join(root, "factory"), append([]string{"helper"}, args...)...)
}

func orchestrationHelperSource() string {
	GinkgoHelper()
	path, err := filepath.Abs(filepath.Join("..", "runtime", "shell", "helpers.sh"))
	Expect(err).NotTo(HaveOccurred())
	return path
}

func orchestrationHelperShell(root, cwd, shell, probe string, environment map[string]string, args ...string) cliResult {
	GinkgoHelper()
	controls := map[string]string{"FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}
	for key, value := range environment {
		controls[key] = value
	}
	argv := []string{"-c", probe, "helper-probe", orchestrationHelperSource()}
	return orchestrationHelperProcess(cwd, controls, shell, append(argv, args...)...)
}

func orchestrationHelperPlan(payload, header string, keys []string) map[string]string {
	GinkgoHelper()
	Expect(payload).To(HaveSuffix("\n"), "plans end with a complete END record")
	rows := strings.Split(strings.TrimSuffix(payload, "\n"), "\n")
	Expect(rows).To(HaveLen(len(keys) + 2))
	Expect(rows[0]).To(Equal(header))
	Expect(rows[len(rows)-1]).To(Equal("END"))
	values := map[string]string{}
	for i, key := range keys {
		fields := strings.SplitN(rows[i+1], "\t", 3)
		Expect(fields).To(HaveLen(3))
		Expect(fields[0]).To(Equal("set"))
		Expect(fields[1]).To(Equal(key))
		values[key] = fields[2]
	}
	return values
}

func orchestrationHelperGit(cwd string, args ...string) {
	GinkgoHelper()
	result := orchestrationHelperProcess(cwd, nil, "git", args...)
	Expect(result).To(Equal(cliResult{"", "", 0}), "git %v", args)
}

var _ = Describe("Native orchestration helpers", func() {
	// per docs/adr/0099-native-factory-orchestration.md:35
	// per docs/adr/0099-native-factory-orchestration.md:134
	It("keeps helper operations private and rejects malformed requests before effects", func() {
		root, cwd := fixture()
		for _, request := range [][]string{
			{}, {"unknown"}, {"now", "extra"}, {"duration"}, {"duration", "1", "2", "3"},
			{"palette"}, {"palette", "yes"}, {"palette", "01"}, {"palette", "0", "extra"},
			{"event", "gate", "reason", "extra"}, {"hookspath"}, {"hookspath", cwd, "extra"},
			{"action-box", "reset", "bold", "yellow"}, {"budget-plan", "PATH"},
			{"budget-plan", "FACTORY_BUDGET_ENABLED"}, {"budget-model", "codex", "reviewer", "standard", "frontier", "default"},
		} {
			result := orchestrationHelperBridge(root, cwd, nil, request...)
			Expect(result.status).To(Equal(2), "%q: %+v", request, result)
			Expect(result.stdout).To(BeEmpty())
			Expect(result.stderr).To(HavePrefix("factory bridge: "))
			Expect(result.stderr).NotTo(ContainSubstring("Usage:"))
		}
		public := orchestrationHelperProcess(cwd, nil, filepath.Join(root, "factory"), "helper", "now")
		Expect(public).To(Equal(cliResult{"", "factory: unknown command 'helper' (try: factory help)\n", 2}))
		entries, err := os.ReadDir(cwd)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	})

	// per docs/adr/0099-native-factory-orchestration.md:138
	It("appends a UTC TSV event at the caller Git root with default gate and reason", func() {
		root, cwd := fixture()
		orchestrationHelperGit(cwd, "init", "-q")
		nested := filepath.Join(cwd, "deep", "caller")
		Expect(os.MkdirAll(nested, 0700)).To(Succeed())
		before := time.Now().UTC().Add(-time.Second)
		Expect(orchestrationHelperBridge(root, nested, nil, "event")).To(Equal(cliResult{"", "", 0}))
		data, err := os.ReadFile(filepath.Join(cwd, ".factory", "events.log"))
		Expect(err).NotTo(HaveOccurred())
		fields := strings.Split(string(data), "\t")
		Expect(fields).To(HaveLen(3))
		timestamp, err := time.Parse("2006-01-02T15:04:05Z", fields[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(timestamp).To(BeTemporally(">=", before))
		Expect(timestamp).To(BeTemporally("<=", time.Now().UTC().Add(time.Second)))
		Expect(fields[1:]).To(Equal([]string{"gate", "blocked\n"}))
	})

	// per docs/adr/0099-native-factory-orchestration.md:138
	It("honors a literal relative event override outside Git and leaves existing rows intact", func() {
		root, cwd := fixture()
		path := filepath.Join(cwd, "event logs", "chosen.tsv")
		writeFixture(path, []byte("old\tgate\treason\n"), 0600)
		result := orchestrationHelperBridge(root, cwd, map[string]string{"FACTORY_EVENT_LOG": "event logs/chosen.tsv"}, "event", "literal $(touch EVENT_EXECUTED)", "quote'\" and backslash\\")
		Expect(result).To(Equal(cliResult{"", "", 0}))
		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(HavePrefix("old\tgate\treason\n"))
		Expect(string(data)).To(HaveSuffix("\tliteral $(touch EVENT_EXECUTED)\tquote'\" and backslash\\\n"))
		_, err = os.Stat(filepath.Join(cwd, "EVENT_EXECUTED"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0099-native-factory-orchestration.md:138
	It("trims only above the cap and retains its newest half with at least one row", func() {
		root, cwd := fixture()
		for _, cap := range []string{"4", "1"} {
			path := filepath.Join(cwd, "events-"+cap)
			n, err := strconv.Atoi(cap)
			Expect(err).NotTo(HaveOccurred())
			var seed strings.Builder
			for i := 0; i < n-1; i++ {
				fmt.Fprintf(&seed, "old\tgate\t%d\n", i)
			}
			writeFixture(path, []byte(seed.String()), 0600)
			env := map[string]string{"FACTORY_EVENT_LOG": path, "FACTORY_EVENT_MAX_LINES": cap}
			Expect(orchestrationHelperBridge(root, cwd, env, "event", "gate", "at-cap")).To(Equal(cliResult{"", "", 0}))
			data, readErr := os.ReadFile(path)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(strings.Count(string(data), "\n")).To(Equal(n))
			Expect(orchestrationHelperBridge(root, cwd, env, "event", "gate", "newest")).To(Equal(cliResult{"", "", 0}))
			data, readErr = os.ReadFile(path)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(strings.Count(string(data), "\n")).To(Equal(max(n/2, 1)))
			Expect(string(data)).To(HaveSuffix("\tgate\tnewest\n"))
			if n > 1 {
				Expect(string(data)).To(ContainSubstring("\tgate\tat-cap\n"))
			}
		}
	})

	// per docs/adr/0099-native-factory-orchestration.md:248
	It("retains the newest prior row and appended event after a log exceeds 64 MiB", func() {
		root, cwd := fixture()
		path := filepath.Join(cwd, "large-events.log")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		Expect(err).NotTo(HaveOccurred())
		payload := strings.Repeat("x", 22<<20)
		for row := 0; row < 3; row++ {
			_, err = file.WriteString("old\tgate\t")
			Expect(err).NotTo(HaveOccurred())
			_, err = file.WriteString(payload)
			Expect(err).NotTo(HaveOccurred())
			_, err = file.WriteString("\n")
			Expect(err).NotTo(HaveOccurred())
		}
		_, err = file.WriteString("prior\tgate\tretained-newest\n")
		Expect(err).NotTo(HaveOccurred())
		Expect(file.Close()).To(Succeed())
		info, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Size()).To(BeNumerically(">", 64<<20))
		environment := map[string]string{"FACTORY_EVENT_LOG": path, "FACTORY_EVENT_MAX_LINES": "4"}
		Expect(orchestrationHelperBridge(root, cwd, environment, "event", "gate", "newest")).To(Equal(cliResult{"", "", 0}))
		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Count(string(data), "\n")).To(Equal(2))
		Expect(string(data)).To(HavePrefix("prior\tgate\tretained-newest\n"))
		Expect(string(data)).To(HaveSuffix("\tgate\tnewest\n"))
	})

	// per docs/adr/0099-native-factory-orchestration.md:138
	It("falls back to 5000 for invalid caps and never turns logging failure into a gate failure", func() {
		root, cwd := fixture()
		for _, cap := range []string{"0", "-1", "not-a-number"} {
			path := filepath.Join(cwd, "events-"+cap)
			writeFixture(path, []byte(strings.Repeat("old\tgate\told\n", 5000)), 0600)
			Expect(orchestrationHelperBridge(root, cwd, map[string]string{"FACTORY_EVENT_LOG": path, "FACTORY_EVENT_MAX_LINES": cap}, "event")).To(Equal(cliResult{"", "", 0}))
			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.Count(string(data), "\n")).To(Equal(2500))
		}
		writeFixture(filepath.Join(cwd, "regular-parent"), []byte("unchanged"), 0600)
		for _, path := range []string{filepath.Join(cwd, "regular-parent", "events.log"), cwd} {
			Expect(orchestrationHelperBridge(root, cwd, map[string]string{"FACTORY_EVENT_LOG": path}, "event")).To(Equal(cliResult{"", "", 0}))
		}
		Expect(orchestrationHelperBridge(root, cwd, nil, "event")).To(Equal(cliResult{"", "", 0}))
		_, err := os.Stat(filepath.Join(cwd, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "no implicit log is created outside Git")
	})

	// per docs/adr/0099-native-factory-orchestration.md:131
	It("returns current epoch seconds without delegating timing to an external date utility", func() {
		root, cwd := fixture()
		before := time.Now().Unix()
		result := orchestrationHelperBridge(root, cwd, map[string]string{"PATH": ""}, "now")
		Expect(result.status).To(BeZero())
		Expect(result.stderr).To(BeEmpty())
		epoch, err := strconv.ParseInt(result.stdout, 10, 64)
		Expect(err).NotTo(HaveOccurred())
		Expect(epoch).To(BeNumerically(">=", before))
		Expect(epoch).To(BeNumerically("<=", time.Now().Unix()))
	})

	// per docs/adr/0099-native-factory-orchestration.md:138
	DescribeTable("formats durations and degrades unsupported numbers to unknown", func(start, end, expected string) {
		root, cwd := fixture()
		Expect(orchestrationHelperBridge(root, cwd, nil, "duration", start, end)).To(Equal(cliResult{expected, "", 0}))
	}, Entry("seconds", "100", "159", "59s"), Entry("minutes", "100", "172", "1m 12s"),
		Entry("hours omit seconds", "100", "3887", "1h 3m"), Entry("backwards clock", "172", "100", "0s"),
		Entry("nonnumeric start", "yesterday", "100", "unknown"), Entry("out of range", "0", "18446744073709551616", "unknown"))

	// per docs/adr/0099-native-factory-orchestration.md:139
	// per docs/adr/0099-native-factory-orchestration.md:145
	DescribeTable("frames the six palette values using explicit caller TTY and NO_COLOR", func(tty, noColor string, colored bool) {
		root, cwd := fixture()
		result := orchestrationHelperBridge(root, cwd, map[string]string{"NO_COLOR": noColor}, "palette", tty)
		Expect(result.status).To(BeZero())
		Expect(result.stderr).To(BeEmpty())
		values := orchestrationHelperPlan(result.stdout, "FACTORY_HELPER_PALETTE_V1", orchestrationPaletteKeys)
		codes := []string{"\x1b[0m", "\x1b[1m", "\x1b[2m", "\x1b[33m", "\x1b[32m", "\x1b[31m"}
		for i, key := range orchestrationPaletteKeys {
			expected := ""
			if colored {
				expected = codes[i]
			}
			Expect(values[key]).To(Equal(expected))
		}
	}, Entry("captured stdout still obeys caller terminal", "1", "", true), Entry("pipe disables colors", "0", "", false),
		Entry("nonempty NO_COLOR disables colors", "1", "1", false))

	// per docs/adr/0099-native-factory-orchestration.md:140
	It("renders action boxes with literal caller palette, title and lines", func() {
		root, cwd := fixture()
		reset, bold, yellow := "<reset>", "<bold>", "$(touch BOX_EXECUTED)"
		title := "quote'\" \\ title"
		result := orchestrationHelperBridge(root, cwd, nil, "action-box", reset, bold, yellow, title, "", "two words\nnext line")
		expected := yellow + bold + "┌─ " + title + " " + reset + "\n" + yellow + "│" + reset + " \n" + yellow + "│" + reset + " two words\nnext line\n" + yellow + "└─" + reset + "\n"
		Expect(result).To(Equal(cliResult{expected, "", 0}))
		_, err := os.Stat(filepath.Join(cwd, "BOX_EXECUTED"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0099-native-factory-orchestration.md:139
	DescribeTable("classifies the hook Git resolves separately from its executable bit", func(state string) {
		root, cwd := fixture()
		orchestrationHelperGit(cwd, "init", "-q")
		resolved := filepath.Join(cwd, ".git", "hooks", "pre-push")
		if state != "absent" {
			selected := ".githooks"
			if state == "hijacked" {
				selected = "elsewhere"
			}
			orchestrationHelperGit(cwd, "config", "core.hooksPath", selected)
			resolved = filepath.Join(cwd, selected, "pre-push")
			mode := os.FileMode(0600)
			if state == "armed" {
				mode = 0700
			}
			writeFixture(resolved, []byte("#!/bin/sh\nexit 0\n"), mode)
		}
		Expect(orchestrationHelperBridge(root, cwd, nil, "hookspath", cwd)).To(Equal(cliResult{state + "\t" + resolved, "", 0}))
	}, Entry("absent", "absent"), Entry("configured but inert", "inert"), Entry("armed", "armed"), Entry("hijacked", "hijacked"))

	// per docs/adr/0099-native-factory-orchestration.md:132
	// per docs/adr/0099-native-factory-orchestration.md:143
	It("frames only nine default budget assignments and never serializes unrelated environment secrets", func() {
		root, cwd := fixture()
		result := orchestrationHelperBridge(root, cwd, map[string]string{"UNRELATED_API_KEY": "must-never-appear", "FACTORY_BUDGET_ENABLED": "caller-is-overwritten"}, "budget-plan")
		Expect(result.status).To(BeZero())
		Expect(result.stderr).To(BeEmpty())
		values := orchestrationHelperPlan(result.stdout, "FACTORY_BUDGET_PLAN_V1", orchestrationBudgetKeys)
		Expect(values).To(Equal(map[string]string{
			"FACTORY_BUDGET_ROOT": cwd, "FACTORY_BUDGET_ENABLED": "false", "FACTORY_BUDGET_MAX_ATTEMPTS": "1",
			"FACTORY_BUDGET_MAX_SESSION_RUNS": "5", "FACTORY_BUDGET_TIMEOUT_SECONDS": "300", "FACTORY_BUDGET_SESSION_SECONDS": "900",
			"FACTORY_BUDGET_MAX_CONCURRENT": "1", "FACTORY_BUDGET_ESTIMATED_USD": "", "FACTORY_BUDGET_ACTION": "stop",
		}))
		Expect(result.stdout + result.stderr).NotTo(ContainSubstring("must-never-appear"))
	})

	// per docs/adr/0099-native-factory-orchestration.md:133
	It("uses shared configuration precedence and preserves explicitly named caller configuration values as exports", func() {
		root, cwd := fixture()
		path := filepath.Join(cwd, "selected settings.yaml")
		writeFixture(path, []byte("budget_max_attempts: 7\ncost_profile: economy\nmodel_provider: yaml/provider\ncodex_economy_model: literal $(touch BUDGET_EXECUTED)\n"), 0600)
		writeFixture(filepath.Join(cwd, "factory.config"), []byte("MODEL_PROVIDER=legacy/provider\nCODEX_DEFAULT_MODEL=legacy/default\nUNRELATED_API_KEY=private-value\n"), 0600)
		result := orchestrationHelperBridge(root, cwd, map[string]string{"FACTORY_CONFIG": path}, "budget-plan", "MODEL_PROVIDER")
		Expect(result.status).To(BeZero())
		Expect(result.stderr).To(BeEmpty())
		Expect(result.stdout).To(HavePrefix("FACTORY_BUDGET_PLAN_V1\n"))
		Expect(result.stdout).To(HaveSuffix("END\n"))
		Expect(result.stdout).To(ContainSubstring("set\tFACTORY_BUDGET_MAX_ATTEMPTS\t7\n"))
		Expect(result.stdout).To(ContainSubstring("set\tCOST_PROFILE\teconomy\n"))
		Expect(result.stdout).To(ContainSubstring("set\tCODEX_DEFAULT_MODEL\tlegacy/default\n"))
		Expect(result.stdout).To(ContainSubstring("set\tCODEX_ECONOMY_MODEL\tliteral $(touch BUDGET_EXECUTED)\n"))
		Expect(result.stdout).To(ContainSubstring("export\tMODEL_PROVIDER\n"))
		Expect(result.stdout).NotTo(ContainSubstring("set\tMODEL_PROVIDER\t"))
		Expect(result.stdout).NotTo(ContainSubstring("private-value"))
		_, err := os.Stat(filepath.Join(cwd, "BUDGET_EXECUTED"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0099-native-factory-orchestration.md:133
	DescribeTable("selects budget models with frontier protection and literal model bytes", func(harness, role, profile, expected string) {
		root, cwd := fixture()
		Expect(orchestrationHelperBridge(root, cwd, nil, "budget-model", harness, role, profile, "frontier $(touch MODEL_EXECUTED)", "default/model", "economy/model")).To(Equal(cliResult{expected, "", 0}))
		_, err := os.Stat(filepath.Join(cwd, "MODEL_EXECUTED"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("economy reviewer remains frontier", "codex", "reviewer", "economy", "frontier $(touch MODEL_EXECUTED)"),
		Entry("standard refactorer collapses economy", "claude", "refactorer", "standard", "default/model"),
		Entry("economy refactorer", "opencode", "refactorer", "economy", "economy/model"))

	// per docs/adr/0099-native-factory-orchestration.md:140
	// per docs/adr/0099-native-factory-orchestration.md:142
	It("sources inert helper definitions in POSIX sh without child launches or caller state changes", func() {
		root, cwd := fixture()
		marker := filepath.Join(cwd, "unexpected-launch")
		fake := filepath.Join(root, "must-not-run")
		writeFixture(fake, []byte("#!/bin/sh\n: > \"$FACTORY_ACCEPTANCE_HELPER_MARKER\"\nprintf unexpected\n"), 0755)
		probe := `set -eu; before="$(env | sort)"; options="$-"; directory="$PWD"; . "$1"; [ "$before" = "$(env | sort)" ] && [ "$options" = "$-" ] && [ "$directory" = "$PWD" ]`
		result := orchestrationHelperShell(root, cwd, "/bin/sh", probe, map[string]string{"FACTORY_RUNTIME_BINARY": fake, "FACTORY_ACCEPTANCE_HELPER_MARKER": marker})
		Expect(result).To(Equal(cliResult{"", "", 0}))
		_, err := os.Stat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0099-native-factory-orchestration.md:140
	It("passes unexported caller palette overrides literally to action-box rendering", func() {
		root, cwd := fixture()
		probe := `set -eu; . "$1"; C_RESET='<R>'; C_BOLD='<B>'; C_YELLOW='<Y>'; action_box 'caller title' 'literal $(touch PALETTE_EXECUTED)'; printf '%s\000%s\000%s' "$C_RESET" "$C_BOLD" "$C_YELLOW"`
		result := orchestrationHelperShell(root, cwd, "/bin/sh", probe, nil)
		Expect(result).To(Equal(cliResult{"<Y><B>┌─ caller title <R>\n<Y>│<R> literal $(touch PALETTE_EXECUTED)\n<Y>└─<R>\n<R>\x00<B>\x00<Y>", "", 0}))
		_, err := os.Stat(filepath.Join(cwd, "PALETTE_EXECUTED"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0099-native-factory-orchestration.md:141
	// per docs/adr/0099-native-factory-orchestration.md:144
	It("keeps readonly bridge controls in the parent while applying intentional budget exports", func() {
		root, cwd := fixture()
		path := filepath.Join(cwd, "caller configuration.yaml")
		writeFixture(path, []byte("budget_enabled: true\nmodel_provider: yaml/provider\n"), 0600)
		probe := `set -eu; . "$1"; FACTORY_CONFIG="$2"; FACTORY_BRIDGE_PROTOCOL=caller; readonly FACTORY_CONFIG FACTORY_BRIDGE_PROTOCOL FACTORY_RUNTIME_BINARY; factory_duration 10 15; factory_budget_config; printf '\000%s\000%s\000%s\000' "$FACTORY_CONFIG" "$FACTORY_BRIDGE_PROTOCOL" "$MODEL_PROVIDER"; /bin/sh -c 'printf "%s\000%s" "$FACTORY_BUDGET_ENABLED" "$MODEL_PROVIDER"'`
		result := orchestrationHelperShell(root, cwd, "bash", probe, nil, path)
		Expect(result).To(Equal(cliResult{"5s\x00" + path + "\x00caller\x00yaml/provider\x00true\x00yaml/provider", "", 0}))
	})

	// per docs/adr/0099-native-factory-orchestration.md:143
	It("validates every bounded budget record before changing any caller variable", func() {
		root, cwd := fixture()
		fake := filepath.Join(root, "plan-runtime")
		writeFixture(fake, []byte("#!/bin/sh\n/bin/cat \"$FACTORY_ACCEPTANCE_HELPER_PLAN_FILE\"\n"), 0755)
		planFile := filepath.Join(cwd, "plan-records")
		validPrefix := "FACTORY_BUDGET_PLAN_V1\nset\tFACTORY_BUDGET_ENABLED\tchanged\n"
		for _, payload := range []string{
			"WRONG\nset\tFACTORY_BUDGET_ENABLED\tchanged\nEND\n",
			validPrefix + "set\tPATH\tattacker\nEND\n",
			validPrefix + "export\tUNRELATED_API_KEY\nEND\n",
			validPrefix + "unknown\tMODEL_PROVIDER\tvalue\nEND\n",
			validPrefix, validPrefix + "END", validPrefix + "END\ntrailing\n",
			validPrefix + "set\tMODEL_PROVIDER\t" + strings.Repeat("x", 2<<20) + "\nEND\n",
		} {
			writeFixture(planFile, []byte(payload), 0600)
			probe := `. "$1" || exit "$?"; FACTORY_BUDGET_ENABLED=original; MODEL_PROVIDER=original; factory_budget_config; status=$?; printf '%s\000%s\000%s' "$status" "$FACTORY_BUDGET_ENABLED" "$MODEL_PROVIDER"`
			result := orchestrationHelperShell(root, cwd, "bash", probe, map[string]string{"FACTORY_RUNTIME_BINARY": fake, "FACTORY_ACCEPTANCE_HELPER_PLAN_FILE": planFile})
			Expect(result.status).To(BeZero())
			Expect(result.stdout).To(Equal("2\x00original\x00original"))
			Expect(result.stderr).To(HavePrefix("factory bridge: "))
		}
	})

	// per docs/adr/0099-native-factory-orchestration.md:143
	DescribeTable("refuses budget target attributes before an earlier valid assignment can take effect", func(declaration string) {
		root, cwd := fixture()
		shell, err := exec.LookPath("bash")
		Expect(err).NotTo(HaveOccurred())
		if strings.HasPrefix(declaration, "declare -n ") {
			if supported := orchestrationHelperProcess(cwd, nil, shell, "-c", `declare -n fixture_probe`); supported.status != 0 {
				Skip("the selected Bash does not support declare -n: " + shell)
			}
		}
		fake := filepath.Join(root, "plan-runtime")
		writeFixture(fake, []byte("#!/bin/sh\nprintf '%s' \"$FACTORY_ACCEPTANCE_HELPER_PLAN\"\n"), 0755)
		payload := "FACTORY_BUDGET_PLAN_V1\nset\tMODEL_PROVIDER\tchanged\nset\tFACTORY_BUDGET_MAX_ATTEMPTS\t9\nEND\n"
		probe := `. "$1" || exit "$?"; MODEL_PROVIDER=original; target=1; ` + declaration + `; factory_budget_config; status=$?; printf '%s\000%s\000%s' "$status" "$MODEL_PROVIDER" "$target"`
		result := orchestrationHelperShell(root, cwd, shell, probe, map[string]string{"FACTORY_RUNTIME_BINARY": fake, "FACTORY_ACCEPTANCE_HELPER_PLAN": payload})
		Expect(result.status).To(BeZero())
		Expect(result.stdout).To(Equal("2\x00original\x001"))
		Expect(result.stderr).To(HavePrefix("factory bridge: "))
	}, Entry("readonly", "readonly FACTORY_BUDGET_MAX_ATTEMPTS=1"), Entry("integer", "declare -i FACTORY_BUDGET_MAX_ATTEMPTS=1"),
		Entry("nameref", "declare -n FACTORY_BUDGET_MAX_ATTEMPTS=target"))
})
