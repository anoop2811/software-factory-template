package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
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

// Materialize current source bytes without invoking the operator's checkout.
// per docs/adr/0139-go-source-budget-entrypoint.md:78
func sourceBudgetAdapter(root string) string {
	GinkgoHelper()
	body, err := os.ReadFile("../scripts/factory-budget.sh")
	Expect(err).NotTo(HaveOccurred())
	path := filepath.Join(root, "scripts/factory-budget.sh")
	writeFixture(path, body, 0755)
	return path
}

// per docs/adr/0139-go-source-budget-entrypoint.md:18
const sourceBudgetRecorder = `#!/bin/sh
printf '%s\000' "$PWD" "$FACTORY_CONFIG" "$FACTORY_BUDGET_MODEL" "${FACTORY_AGENT_ROLE+x}" "${FACTORY_AGENT_ROLE-}" "$@" > "$BUDGET_ARGV"
/bin/cat > "$BUDGET_INPUT"
printf 'selected stdout\n'
printf 'selected stderr\n' >&2
exit 63
`

// Reuse the qualified Go harness fake, with no Python executable in child PATH.
// The caller project and physical source owner are different directories.
// per docs/adr/0139-go-source-budget-entrypoint.md:34
// per docs/adr/0139-go-source-budget-entrypoint.md:74
func sourceBudgetNativeFixture() (project, owner, program string, environment map[string]string) {
	GinkgoHelper()
	controllerBuild()
	project, owner = controllerFixture()
	writeFixture(filepath.Join(owner, "factory"), candidateBinary, 0700)
	program = sourceBudgetAdapter(owner)
	git, err := exec.LookPath("git")
	Expect(err).NotTo(HaveOccurred())
	git, err = filepath.EvalSymlinks(git)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.Symlink(git, filepath.Join(project, "bin/git"))).To(Succeed())
	imageGit(project, "init", "-q")
	writeFixture(filepath.Join(project, "factory.yaml"), []byte("budget_enabled: false\n"), 0600)
	writeFixture(filepath.Join(project, "selected.yaml"), []byte(configuredModels()+"budget_enabled: true\nbudget_max_attempts: 1\nbudget_max_session_runs: 8\n"), 0600)
	writeFixture(filepath.Join(project, "prompt file\n.txt"), []byte("PRIVATE_BUDGET_PROMPT\n"), 0600)
	environment = map[string]string{
		"PATH": filepath.Join(project, "bin"), "BASH_ENV": "", "ENV": "", "GORACE": "atexit_sleep_ms=0",
		"FACTORY_CONFIG": filepath.Join(project, "selected.yaml"), "FACTORY_BUDGET_MODEL": "WRONG_INHERITED_MODEL", "REPO_ROOT": owner,
		"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull,
		"CONTROLLER_FIXTURE_LOG": filepath.Join(project, "native-calls.jsonl"),
	}
	lookup := handoffNativeExactProcess(project, environment, "", "/bin/bash", "-c", "command -v python python3")
	Expect(lookup).To(Equal(cliResult{"", "", 1}), "configured execution must have neither Python spelling available")
	return
}

// per docs/adr/0139-go-source-budget-entrypoint.md:39
func sourceBudgetRunArgs(project, harness string) []string {
	return []string{"run", "--session", "source-session", "--task", "source-task", "--harness=" + harness, "--role", "reviewer", "--prompt-file", filepath.Join(project, "prompt file\n.txt"), "--json"}
}

// per docs/adr/0139-go-source-budget-entrypoint.md:35
func sourceBudgetRecord(project string) map[string]any {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join(project, ".factory/budget.json"))
	Expect(err).NotTo(HaveOccurred())
	Expect(string(data)).NotTo(ContainSubstring("PRIVATE_"))
	rows := budgetDecode(data).(map[string]any)["runs"].([]any)
	Expect(rows).To(HaveLen(1))
	return rows[0].(map[string]any)
}

var _ = Describe("Go source runtime entrypoints budget adapter", func() {
	// per docs/adr/0139-go-source-budget-entrypoint.md:16
	// per docs/adr/0139-go-source-budget-entrypoint.md:27
	It("uses physical source identity and transports literal arguments streams cwd and environment", func() {
		shells := []string{"/bin/bash"}
		if bash, err := exec.LookPath("bash"); err == nil && bash != shells[0] {
			shells = append(shells, bash)
		}
		for _, shell := range shells {
			for _, mode := range []string{"direct", "explicit", "directory-alias", "absolute-path", "relative-path"} {
				root, caller := fixtureDirectories()
				owner := filepath.Join(filepath.Dir(root), "budget owner ' $(literal)\n")
				Expect(os.Rename(root, owner)).To(Succeed())
				program := sourceBudgetAdapter(owner)
				base := filepath.Dir(owner)
				tools, marker := filepath.Join(base, "decoys"), filepath.Join(base, "WRONG_OWNER")
				poison := []byte("#!/bin/sh\nprintf called > \"$BUDGET_DECOY\"\nexit 97\n")
				for _, path := range []string{filepath.Join(caller, "factory"), filepath.Join(tools, "factory"), filepath.Join(tools, "factory-budget.sh"), filepath.Join(tools, "git"), filepath.Join(tools, "python3"), filepath.Join(owner, "scripts/lib/config.sh"), filepath.Join(owner, "scripts/lib/roles.sh"), filepath.Join(owner, "scripts/lib/budget-config.sh"), filepath.Join(owner, "scripts/lib/budget.py")} {
					writeFixture(path, poison, 0700)
				}
				writeFixture(filepath.Join(owner, "factory"), []byte(sourceBudgetRecorder), 0700)
				record, input := filepath.Join(base, "argv"), filepath.Join(base, "stdin")
				environment := map[string]string{"PATH": tools + ":/usr/bin:/bin", "BASH_ENV": "", "ENV": "", "REPO_ROOT": caller, "FACTORY_HOME": caller, "FACTORY_RUNTIME_BINARY": filepath.Join(caller, "factory"), "FACTORY_CONFIG": "literal $(touch BUDGET_INJECTION)\nconfig", "FACTORY_BUDGET_MODEL": "caller model\n", "FACTORY_AGENT_ROLE": "spec-writer", "BUDGET_ARGV": record, "BUDGET_INPUT": input, "BUDGET_DECOY": marker}
				if mode == "directory-alias" {
					alias := filepath.Join(base, "parent alias")
					Expect(os.Symlink(filepath.Join(owner, "scripts"), alias)).To(Succeed())
					program = filepath.Join(alias, "factory-budget.sh")
				}
				args := []string{"run", "--harness=claude", "--role", "reviewer", "", "--role=literal $(touch BUDGET_INJECTION)", "--", "line\noperand"}
				executable, operands := shell, append([]string{program}, args...)
				if mode == "direct" {
					executable, operands = program, args
				}
				if strings.HasSuffix(mode, "-path") {
					Expect(os.Chmod(program, 0644)).To(Succeed())
					lookup := filepath.Dir(program)
					if mode == "relative-path" {
						var err error
						lookup, err = filepath.Rel(caller, lookup)
						Expect(err).NotTo(HaveOccurred())
					}
					environment["PATH"] = lookup + ":" + environment["PATH"]
					operands = append([]string{"factory-budget.sh"}, args...)
				}
				beforeOwner, beforeCaller := installationSnapshot(owner), installationSnapshot(caller)
				stdin := "borrowed budget input\x00tail\n"
				out := handoffNativeExactProcess(caller, environment, stdin, executable, operands...)
				Expect(out).To(Equal(cliResult{"selected stdout\n", "selected stderr\n", 63}), "%s/%s", shell, mode)
				expected := append([]string{caller, environment["FACTORY_CONFIG"], environment["FACTORY_BUDGET_MODEL"], "x", "spec-writer", "budget"}, args...)
				Expect(os.ReadFile(record)).To(Equal([]byte(strings.Join(expected, "\x00") + "\x00")))
				Expect(os.ReadFile(input)).To(Equal([]byte(stdin)))
				Expect(installationSnapshot(owner)).To(Equal(beforeOwner))
				Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
				_, err := os.Lstat(marker)
				Expect(os.IsNotExist(err)).To(BeTrue(), "readers, Python and alternate owners must not participate in handoff")
			}
		}
	})

	// per docs/adr/0139-go-source-budget-entrypoint.md:21
	// per docs/adr/0139-go-source-budget-entrypoint.md:23
	It("refuses invalid source metadata file symlinks and unavailable owners without fallback", func() {
		shells := []string{"/bin/bash"}
		if bash, err := exec.LookPath("bash"); err == nil && bash != shells[0] {
			shells = append(shells, bash)
		}
		for _, shell := range shells {
			for _, mode := range []string{"scalar-empty", "scalar-path", "scalar-declaration", "file-alias", "missing", "nonexecutable", "failed"} {
				root, caller := fixtureDirectories()
				program := sourceBudgetAdapter(root)
				marker, tools := filepath.Join(filepath.Dir(root), "FALLBACK"), filepath.Join(filepath.Dir(root), "tools")
				poison := []byte("#!/bin/sh\nprintf called > \"$BUDGET_DECOY\"\nexit 97\n")
				for _, path := range []string{filepath.Join(tools, "factory"), filepath.Join(tools, "python3"), filepath.Join(caller, "factory"), filepath.Join(root, "scripts/lib/config.sh"), filepath.Join(root, "scripts/lib/budget.py")} {
					writeFixture(path, poison, 0700)
				}
				owner := filepath.Join(root, "factory")
				if mode != "missing" {
					writeFixture(owner, poison, 0700)
				}
				if mode == "nonexecutable" {
					Expect(os.Chmod(owner, 0600)).To(Succeed())
				}
				if mode == "failed" {
					writeFixture(owner, []byte("#!/bin/sh\nprintf 'owner stdout\\n'\nprintf 'owner failed\\n' >&2\nexit 47\n"), 0700)
				}
				if mode == "file-alias" {
					alias := filepath.Join(caller, "factory-budget.sh")
					Expect(os.Symlink(program, alias)).To(Succeed())
					program = alias
				}
				environment := map[string]string{"PATH": tools + ":/usr/bin:/bin", "BASH_ENV": "", "ENV": "", "BUDGET_DECOY": marker, "REPO_ROOT": caller, "FACTORY_RUNTIME_BINARY": filepath.Join(caller, "factory")}
				if strings.HasPrefix(mode, "scalar-") {
					environment["BASH_SOURCE"] = map[string]string{"scalar-empty": "", "scalar-path": "forged/path", "scalar-declaration": "declare -a BASH_SOURCE=([0]=forged)"}[mode]
				}
				out := handoffNativeExactProcess(caller, environment, "caller input\n", shell, program)
				if mode == "failed" {
					Expect(out).To(Equal(cliResult{"owner stdout\n", "owner failed\n", 47}))
				} else {
					Expect(out.status).To(Equal(2), "%s/%s: %+v", shell, mode, out)
					Expect(out.stdout).To(BeEmpty())
					Expect(out.stderr).NotTo(BeEmpty())
					if strings.HasPrefix(mode, "scalar-") {
						Expect(out.stderr).To(ContainSubstring("native Bash source metadata"))
					}
				}
				_, err := os.Lstat(marker)
				Expect(os.IsNotExist(err)).To(BeTrue())
			}
		}
	})

	// per docs/adr/0139-go-source-budget-entrypoint.md:23
	It("retains compiler refusal and never executes stale output or a Python fallback", func() {
		for _, missing := range []bool{false, true} {
			root, caller := fixtureDirectories()
			program := sourceBudgetAdapter(root)
			launcher, err := os.ReadFile("../factory")
			Expect(err).NotTo(HaveOccurred())
			writeFixture(filepath.Join(root, "factory"), launcher, 0755)
			tools, marker, record := filepath.Join(filepath.Dir(root), "tools"), filepath.Join(filepath.Dir(root), "FALLBACK"), filepath.Join(filepath.Dir(root), "compiler")
			poison := []byte("#!/bin/sh\nprintf called > \"$BUDGET_DECOY\"\nexit 97\n")
			for _, path := range []string{filepath.Join(root, "factory-go"), filepath.Join(tools, "factory"), filepath.Join(tools, "python3"), filepath.Join(root, "scripts/lib/config.sh"), filepath.Join(root, "scripts/lib/budget.py")} {
				writeFixture(path, poison, 0700)
			}
			writeFixture(filepath.Join(tools, "go"), []byte("#!/bin/sh\nprintf called > \"$BUDGET_COMPILER\"\nprintf 'controlled compiler failure\\n' >&2\nexit 58\n"), 0700)
			Expect(os.Symlink("/bin/bash", filepath.Join(tools, "bash"))).To(Succeed())
			environment := map[string]string{"PATH": tools + ":/usr/bin:/bin", "BASH_ENV": "", "ENV": "", "BUDGET_COMPILER": record, "BUDGET_DECOY": marker}
			if missing {
				Expect(os.Remove(filepath.Join(tools, "go"))).To(Succeed())
				environment["PATH"] = tools
				Expect(handoffNativeExactProcess(caller, environment, "", "/bin/bash", "-c", "command -v go")).To(Equal(cliResult{"", "", 1}))
			}
			before := installationSnapshot(root)
			out := handoffNativeExactProcess(caller, environment, "", program, "report", "--json")
			Expect(installationSnapshot(root)).To(Equal(before))
			if missing {
				Expect(out.status).To(Equal(1), "%+v", out)
				Expect(out.stderr).To(ContainSubstring("qualified Go toolchain"))
			} else {
				Expect(out.status).To(Equal(58), "%+v", out)
				Expect(out.stderr).To(ContainSubstring("controlled compiler failure"))
				Expect(os.ReadFile(record)).To(Equal([]byte("called")))
			}
			_, err = os.Lstat(marker)
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	})

	// per docs/adr/0139-go-source-budget-entrypoint.md:33
	// per docs/adr/0139-go-source-budget-entrypoint.md:74
	It("compiles its actual source owner and keeps configured disabled and read-only calls free of state", func() {
		project, _, _, environment := sourceBudgetNativeFixture()
		source := newSourceEntrypointFixture()
		program := sourceBudgetAdapter(source.root)
		buildEnvironment := maps.Clone(environment)
		maps.Copy(buildEnvironment, source.environment)
		maps.Copy(buildEnvironment, sourceReviewGoCaches())
		marker, tools := filepath.Join(filepath.Dir(source.root), "PYTHON_CALLED"), filepath.Join(filepath.Dir(source.root), "poison tools")
		writeFixture(filepath.Join(tools, "python3"), []byte("#!/bin/sh\nprintf called > \"$BUDGET_DECOY\"\nexit 97\n"), 0700)
		buildEnvironment["PATH"] = tools + ":" + buildEnvironment["PATH"]
		buildEnvironment["BUDGET_DECOY"] = marker
		before := sourceEntrypointSnapshot(source.root)
		out := handoffNativeExactProcess(project, buildEnvironment, "", program, "report", "--json")
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stderr).To(BeEmpty())
		compiled, err := os.ReadFile(filepath.Join(source.root, "factory-go"))
		Expect(err).NotTo(HaveOccurred(), "actual source invocation must compile its native owner")
		Expect(compiled).NotTo(BeEmpty())
		Expect(sourceEntrypointSnapshot(source.root)).To(Equal(before))
		_, err = os.Lstat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue())
		writeFixture(filepath.Join(source.root, "factory"), compiled, 0700)
		for _, mode := range []string{"disabled-plan", "disabled-run", "enabled-plan", "report", "invalid-preparation"} {
			if mode == "invalid-preparation" && os.Geteuid() == 0 {
				continue // Read refusal requires the same unprivileged caller as the native oracle.
			}
			config := "budget_enabled: false\n" + configuredModels()
			args, status := configuredPlanArgs("claude", "reviewer"), 2
			switch mode {
			case "disabled-run":
				args = []string{"run", "--harness=claude", "--session=s", "--task=t", "--prompt-file=missing", "--json"}
			case "enabled-plan":
				config, status = "budget_enabled: true\n"+configuredModels(), 0
			case "report":
				args, status = []string{"report", "--json"}, 0
			case "invalid-preparation":
				args = []string{"--help"}
			}
			writeFixture(environment["FACTORY_CONFIG"], []byte(config), 0600)
			beforeProject := installationSnapshot(project)
			if mode == "invalid-preparation" {
				Expect(os.Chmod(environment["FACTORY_CONFIG"], 0000)).To(Succeed())
				DeferCleanup(func() { Expect(os.Chmod(environment["FACTORY_CONFIG"], 0600)).To(Succeed()) })
			}
			out = handoffNativeExactProcess(project, environment, "", program, args...)
			Expect(os.Chmod(environment["FACTORY_CONFIG"], 0600)).To(Succeed())
			Expect(out.status).To(Equal(status), "%s: %+v", mode, out)
			if mode == "invalid-preparation" {
				Expect(out.stdout).To(BeEmpty())
				Expect(out.stderr).To(ContainSubstring("cannot prepare command environment"))
			} else {
				Expect(out.stderr).To(BeEmpty())
				Expect(budgetDecode([]byte(out.stdout))).NotTo(BeNil())
			}
			Expect(installationSnapshot(project)).To(Equal(beforeProject), "%s must not create state, probe a harness or read a missing prompt", mode)
			_, err = os.Lstat(filepath.Join(project, ".factory"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	})

	// per docs/adr/0139-go-source-budget-entrypoint.md:34
	// per docs/adr/0139-go-source-budget-entrypoint.md:75
	It("runs all three native harnesses without Python and preserves role model and private accounting", func() {
		for _, harness := range []string{"codex", "claude", "opencode"} {
			project, owner, program, environment := sourceBudgetNativeFixture()
			cwd := filepath.Join(project, "nested caller")
			Expect(os.Mkdir(cwd, 0700)).To(Succeed())
			model := harness + "/literal;$(touch BUDGET_INJECTION)"
			environment[strings.ToUpper(harness)+"_FRONTIER_MODEL"] = model
			environment["FACTORY_AGENT_ROLE"] = "implementer"
			for _, item := range []struct{ role, profile, model string }{{"implementer", "standard", harness + "/default"}, {"refactorer", "economy", harness + "/economy"}} {
				environment["COST_PROFILE"] = item.profile
				plan := handoffNativeExactProcess(cwd, environment, "", program, configuredPlanArgs(harness, item.role)...)
				Expect(plan.status).To(BeZero(), "%+v", plan)
				Expect(budgetDecode([]byte(plan.stdout)).(map[string]any)["model"]).To(Equal(item.model))
			}
			environment["COST_PROFILE"] = "standard"
			beforeOwner := installationSnapshot(owner)
			out := handoffNativeExactProcess(cwd, environment, "unused caller stdin\n", program, sourceBudgetRunArgs(project, harness)...)
			Expect(out.status).To(BeZero(), "%s: %+v", harness, out)
			Expect(out.stderr).To(BeEmpty())
			Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_"))
			calls := controllerCalls(project)
			Expect(calls).To(HaveLen(2), "one local help probe and one admitted execution")
			Expect(calls[0]["help"]).To(BeTrue())
			call := calls[1]
			Expect(call["pid_published_before_input"]).To(BeTrue())
			Expect(call["cwd"]).To(Equal(project))
			Expect(call["role"]).To(Equal("reviewer"))
			args := []any{"-p", "--output-format", "json", "--agent", "reviewer", "--permission-mode", "plan", "--model", model}
			input := "PRIVATE_BUDGET_PROMPT\n"
			switch harness {
			case "codex":
				args = []any{"exec", "--json", "--sandbox", "read-only", "--cd", project, "--model", model, "-"}
				input = "Factory role: reviewer\n\nCanonical instructions\n\nTask:\n" + input
			case "opencode":
				args = []any{"run", "--format", "json", "--agent", "reviewer", "--model", model}
			}
			Expect(call["argv"]).To(Equal(args))
			Expect(call["stdin"]).To(Equal(input))
			record := sourceBudgetRecord(project)
			Expect(record).To(HaveKeyWithValue("status", "completed"))
			Expect(record).To(HaveKeyWithValue("complete", true))
			Expect(record).To(HaveKeyWithValue("model", model))
			Expect(record).To(HaveKeyWithValue("exit_code", json.Number("0")))
			Expect(installationSnapshot(owner)).To(Equal(beforeOwner))
			_, err := os.Lstat(filepath.Join(project, "BUDGET_INJECTION"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	})

	// per docs/adr/0139-go-source-budget-entrypoint.md:39
	It("charges a failed native attempt and refuses a second admission without retry", func() {
		project, _, program, environment := sourceBudgetNativeFixture()
		environment["CONTROLLER_FIXTURE_MODE"] = "nonzero"
		args := sourceBudgetRunArgs(project, "claude")
		out := handoffNativeExactProcess(project, environment, "", program, args...)
		Expect(out.status).To(Equal(1), "%+v", out)
		Expect(out.stderr).To(BeEmpty())
		Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_"))
		Expect(controllerCalls(project)).To(HaveLen(2))
		record := sourceBudgetRecord(project)
		Expect(record).To(HaveKeyWithValue("status", "completed"))
		Expect(record).To(HaveKeyWithValue("outcome", "failed"))
		Expect(record).To(HaveKeyWithValue("exit_code", json.Number("7")))
		Expect(record).To(HaveKeyWithValue("complete", true))
		Expect(record["tokens"]).NotTo(BeNil())
		Expect(record["estimated_usd"]).NotTo(BeNil())
		after := handoffNativeExactProcess(project, environment, "", program, args...)
		Expect(after.status).To(Equal(2), "%+v", after)
		Expect(controllerCalls(project)).To(HaveLen(2), "exhausted admission must not probe or execute again")
		Expect(sourceBudgetRecord(project)).To(Equal(record))
	})

	// per docs/adr/0139-go-source-budget-entrypoint.md:40
	// per docs/adr/0139-go-source-budget-entrypoint.md:99
	It("cancels acknowledged native work on INT TERM and HUP with durable accounting and owned cleanup", func() {
		for _, item := range []struct {
			native bool
			signal syscall.Signal
		}{{true, syscall.SIGHUP}, {false, syscall.SIGINT}, {false, syscall.SIGTERM}, {false, syscall.SIGHUP}} {
			project, owner, program, environment := sourceBudgetNativeFixture()
			environment["CONTROLLER_FIXTURE_MODE"] = "stall"
			args := sourceBudgetRunArgs(project, "claude")
			if item.native {
				program, args = filepath.Join(owner, "factory"), append([]string{"budget"}, args...)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			command := exec.CommandContext(ctx, program, args...) // #nosec G204 G702 -- current adapter and actual native candidate in a private fixture.
			command.Dir = project
			for key, value := range environment {
				command.Env = append(command.Env, key+"="+value)
			}
			command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			Expect(command.Start()).To(Succeed())
			done := make(chan error, 1)
			go func() { defer close(done); done <- command.Wait() }()
			child, childGroup := 0, 0
			callerGroup := syscall.Getpgrp()
			DeferCleanup(func() {
				if child > 0 && childGroup > 0 {
					_, err := sourceRunnerStopObservedGroup(child, childGroup)
					if err != nil {
						holdFixtureCleanup(filepath.Dir(project), err.Error())
					}
					Expect(err).NotTo(HaveOccurred())
				}
				cancel()
				Eventually(done, 3*time.Second).Should(BeClosed())
			})
			Eventually(func() bool {
				data, err := os.ReadFile(environment["CONTROLLER_FIXTURE_LOG"])
				return err == nil && strings.HasSuffix(string(data), "\n") && strings.Contains(string(data), `"pid_published_before_input":true`)
			}, 5*time.Second).Should(BeTrue(), "fake harness acknowledges stdin after durable PID publication")
			calls := controllerCalls(project)
			Expect(calls).To(HaveLen(2))
			var err error
			child, err = strconv.Atoi(string(calls[1]["pid"].(json.Number)))
			Expect(err).NotTo(HaveOccurred())
			childGroup, err = syscall.Getpgid(child)
			Expect(err).NotTo(HaveOccurred())
			Expect(childGroup).To(Equal(child))
			Expect(childGroup).NotTo(Equal(callerGroup))
			Expect(command.Process.Signal(item.signal)).To(Succeed())
			var waitErr error
			Eventually(done, 8*time.Second).Should(Receive(&waitErr))
			Expect(ctx.Err()).NotTo(HaveOccurred())
			var exited *exec.ExitError
			Expect(errors.As(waitErr, &exited)).To(BeTrue())
			record := sourceBudgetRecord(project)
			AddReportEntry("source-budget.cancellation", map[string]any{"native": item.native, "signal": item.signal.String(), "status": exited.ExitCode(), "child_pid": child, "child_group": childGroup, "child_alive": syscall.Kill(child, 0) == nil, "ledger": record}, ReportEntryVisibilityAlways)
			Expect(exited.ExitCode()).To(Equal(130), "native=%t signal=%s stdout=%s stderr=%s", item.native, item.signal, stdout.String(), stderr.String())
			Expect(stderr.String()).To(BeEmpty())
			Expect(stdout.String()).NotTo(ContainSubstring("PRIVATE_"))
			Expect(record).To(HaveKeyWithValue("status", "completed"))
			Expect(record).To(HaveKeyWithValue("outcome", "interrupted"))
			Expect(record).To(HaveKeyWithValue("complete", false))
			Expect(controllerCalls(project)).To(HaveLen(2))
			Eventually(func() bool { return errors.Is(syscall.Kill(-childGroup, 0), syscall.ESRCH) }, time.Second).Should(BeTrue())
			Expect(syscall.Getpgrp()).To(Equal(callerGroup))
			Expect(syscall.Kill(-callerGroup, 0)).To(Succeed())
		}
	})
})
