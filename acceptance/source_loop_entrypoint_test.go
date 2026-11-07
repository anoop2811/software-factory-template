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

// Keep this explicit list independent of the production initializer inventory.
// per docs/adr/0140-go-source-loop-and-python-retirement.md:33
var sourceRetiredPython = []string{"budget.py", "budget_adapters.py", "loop.py", "runtime_transition.py"}

// Reuse the Go harness and admitted-PID receipt, with a real committed project.
// per docs/adr/0140-go-source-loop-and-python-retirement.md:24
func sourceLoopNativeFixture() (project, owner, program string, environment map[string]string) {
	GinkgoHelper()
	project, owner, _, environment = sourceBudgetNativeFixture()
	Expect(os.Rename(filepath.Join(project, "prompt file\n.txt"), filepath.Join(project, "prompt.txt"))).To(Succeed())
	program = sourceControllerAdapter(owner, "loop")
	writeFixture(filepath.Join(project, "source.txt"), []byte("initial implementation\n"), 0600)
	writeFixture(filepath.Join(project, ".gitignore"), []byte("bin/\ndriver\ncontroller-driver\nnative-calls.jsonl\n.factory/\n"), 0600)
	writeFixture(filepath.Join(project, "scripts/hooks/test-edit-denial.sh"), []byte("#!/bin/sh\nexit 0\n"), 0700)
	writeFixture(filepath.Join(project, "bin/native-probe"), nativeDriver, 0700)
	Expect(os.Symlink("/bin/bash", filepath.Join(project, "bin/bash"))).To(Succeed())
	environment["CONTROLLER_FIXTURE_PROBE"] = filepath.Join(project, "bin/native-probe")
	environment["LOOP_CHECK_LOG"] = filepath.Join(owner, "checks")
	writeFixture(environment["FACTORY_CONFIG"], []byte(configuredModels()+"budget_enabled: true\nbudget_max_attempts: 6\nbudget_max_session_runs: 6\nloop_enabled: true\nloop_max_attempts: 2\nloop_check_command: printf checked >> \"$LOOP_CHECK_LOG\"\n"), 0600)
	imageGit(project, "add", "source.txt", ".gitignore", "factory.yaml", "selected.yaml", "scripts", ".opencode", ".claude", "opencode.json")
	imageGit(project, "-c", "user.name=Loop fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "loop fixture")
	return
}

// per docs/adr/0140-go-source-loop-and-python-retirement.md:61
func sourceLoopArgs(project, action, harness, mode string) []string {
	args := []string{action, "--session", "source-session", "--task", "source-task", "--json"}
	if action != "status" {
		args = append(args, "--harness", harness, "--mode", mode)
		if mode == "bounded" && (action == "run" || action == "resume") {
			args = append(args, "--prompt-file", filepath.Join(project, "prompt.txt"))
		}
	}
	return args
}

// per docs/adr/0140-go-source-loop-and-python-retirement.md:26
func sourceLoopExecutions(project string) []map[string]any {
	GinkgoHelper()
	var calls []map[string]any
	for _, call := range controllerCalls(project) {
		if call["help"] == false {
			calls = append(calls, call)
		}
	}
	return calls
}

var _ = Describe("Go source runtime entrypoints loop adapter", func() {
	// per docs/adr/0140-go-source-loop-and-python-retirement.md:16
	It("uses physical source identity and transports literal arguments streams cwd and environment", func() {
		sourceControllerTransport("loop")
	})
	// per docs/adr/0140-go-source-loop-and-python-retirement.md:20
	It("refuses invalid source metadata file symlinks and unavailable owners without fallback", func() {
		sourceControllerRefusal("loop")
	})
	// per docs/adr/0140-go-source-loop-and-python-retirement.md:22
	It("retains compiler refusal and never executes stale output or a Python fallback", func() {
		sourceControllerCompilerRefusal("loop")
	})
	// per docs/adr/0140-go-source-loop-and-python-retirement.md:33
	// per docs/adr/0140-go-source-loop-and-python-retirement.md:66
	It("retires current Python modules while accepting complete native initializer inputs", func() {
		absent := map[string]bool{}
		for _, name := range sourceRetiredPython {
			_, err := os.Lstat(filepath.Join("../scripts/lib", name))
			absent[name] = os.IsNotExist(err)
		}
		fixture := newNativeCopyInventoryFixture()
		for _, name := range sourceRetiredPython {
			Expect(os.Remove(filepath.Join(fixture.root, "scripts/lib", name))).To(Succeed())
		}
		result := fixture.observe(true)
		AddReportEntry("source-loop.retirement", map[string]any{"modules_absent": absent, "copy_status": result.status, "copy_diagnostic": result.stderr}, ReportEntryVisibilityAlways)
		Expect(result.status).To(BeZero(), "current copy boundary must accept a complete source with no retired modules: %+v", result)
		for name, missing := range absent {
			Expect(missing).To(BeTrue(), "current runtime module must be absent: %s", name)
		}
		Expect(os.Remove(filepath.Join(fixture.root, "scripts/lib/config.sh"))).To(Succeed())
		result = fixture.observe(true)
		Expect(result.status).NotTo(BeZero(), "retirement cannot disable required current-input validation")
	})
	// per docs/adr/0140-go-source-loop-and-python-retirement.md:22
	// per docs/adr/0140-go-source-loop-and-python-retirement.md:60
	It("compiles actual source and preserves read-only manual checkpoint and resume behavior without Python", func() {
		project, owner, _, environment := sourceLoopNativeFixture()
		source := newSourceEntrypointFixture()
		program := sourceControllerAdapter(source.root, "loop")
		buildEnvironment := maps.Clone(environment)
		maps.Copy(buildEnvironment, source.environment)
		maps.Copy(buildEnvironment, sourceReviewGoCaches())
		poison := filepath.Join(owner, "poison")
		writeFixture(filepath.Join(poison, "python3"), []byte("#!/bin/sh\nprintf python > \"$LOOP_CHECK_LOG\"\nexit 97\n"), 0700)
		buildEnvironment["PATH"] = poison + ":" + buildEnvironment["PATH"]
		beforeSource := sourceEntrypointSnapshot(source.root)
		out := handoffNativeExactProcess(project, buildEnvironment, "", program, sourceLoopArgs(project, "plan", "claude", "manual")...)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stderr).To(BeEmpty())
		compiled, err := os.ReadFile(filepath.Join(source.root, "factory-go"))
		Expect(err).NotTo(HaveOccurred(), "the source adapter must compile its actual physical owner")
		Expect(compiled).NotTo(BeEmpty())
		Expect(sourceEntrypointSnapshot(source.root)).To(Equal(beforeSource))
		writeFixture(filepath.Join(source.root, "factory"), compiled, 0700)
		writeFixture(environment["FACTORY_CONFIG"], []byte(configuredModels()+"budget_enabled: false\nloop_enabled: false\nloop_check_command: printf checked >> \"$LOOP_CHECK_LOG\"\n"), 0600)
		before := installationSnapshot(project)
		for _, item := range []struct {
			action, mode string
			status       int
		}{{"plan", "manual", 0}, {"plan", "bounded", 2}, {"run", "bounded", 2}, {"status", "manual", 2}} {
			out = handoffNativeExactProcess(project, environment, "", program, sourceLoopArgs(project, item.action, "claude", item.mode)...)
			Expect(out.status).To(Equal(item.status), "%s/%s: %+v", item.action, item.mode, out)
			if item.action == "status" {
				Expect(out.stderr).To(ContainSubstring("checkpoint not found"))
			} else {
				Expect(out.stderr).To(BeEmpty())
			}
			Expect(installationSnapshot(project)).To(Equal(before))
		}
		_, err = os.Lstat(environment["LOOP_CHECK_LOG"])
		Expect(os.IsNotExist(err)).To(BeTrue(), "read-only and blocked calls cannot execute checks or Python")
		out = handoffNativeExactProcess(project, environment, "", program, sourceLoopArgs(project, "run", "claude", "manual")...)
		Expect(out.status).To(BeZero(), "%+v", out)
		first := manualRecord(out)
		Expect(first).To(HaveKeyWithValue("outcome", "manual_passed"))
		Expect(first["evidence"]).To(HaveLen(1))
		Expect(os.ReadFile(environment["LOOP_CHECK_LOG"])).To(Equal([]byte("checked")))
		out = handoffNativeExactProcess(project, environment, "", program, sourceLoopArgs(project, "resume", "claude", "manual")...)
		Expect(out.status).To(BeZero(), "%+v", out)
		resumed := manualRecord(out)
		Expect(resumed["baseline"]).To(Equal(first["baseline"]))
		Expect(resumed["evidence"]).To(HaveLen(2))
		Expect(os.ReadFile(environment["LOOP_CHECK_LOG"])).To(Equal([]byte("checkedchecked")))
		writeFixture(filepath.Join(project, "source.txt"), []byte("independent source change\n"), 0600)
		out = handoffNativeExactProcess(project, environment, "", program, sourceLoopArgs(project, "resume", "claude", "manual")...)
		Expect(out.status).To(Equal(2), "%+v", out)
		Expect(os.ReadFile(environment["LOOP_CHECK_LOG"])).To(Equal([]byte("checkedchecked")), "stale identity must not run another check")
		_, err = os.Lstat(filepath.Join(project, ".factory/budget.json"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "manual mode must not admit a paid call")
		_, err = os.Lstat(environment["CONTROLLER_FIXTURE_LOG"])
		Expect(os.IsNotExist(err)).To(BeTrue(), "manual mode cannot probe or execute a harness")
	})
	// per docs/adr/0140-go-source-loop-and-python-retirement.md:24
	// per docs/adr/0140-go-source-loop-and-python-retirement.md:61
	It("runs bounded three-harness work without Python and preserves accounting and failure without retry", func() {
		for _, harness := range []string{"codex", "claude", "opencode"} {
			for _, fail := range []bool{false, true} {
				project, _, program, environment := sourceLoopNativeFixture()
				environment["CONTROLLER_FIXTURE_MODE"] = "loop-approve"
				if fail {
					environment["CONTROLLER_FIXTURE_MODE"] = "nonzero"
				}
				args := sourceLoopArgs(project, "run", harness, "bounded")
				out := handoffNativeExactProcess(project, environment, "", program, args...)
				status, calls := 0, 2
				if fail {
					status, calls = 2, 1
				}
				Expect(out.status).To(Equal(status), "%s failure=%t: %+v", harness, fail, out)
				Expect(out.stderr).To(BeEmpty())
				Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_"))
				record := manualRecord(out)
				Expect(record["budget_runs"]).To(HaveLen(calls))
				if fail {
					Expect(record).To(HaveKeyWithValue("status", "stopped"))
				} else {
					Expect(record).To(HaveKeyWithValue("status", "completed"))
					Expect(record).To(HaveKeyWithValue("outcome", "approved"))
				}
				observed := sourceLoopExecutions(project)
				Expect(observed).To(HaveLen(calls))
				Expect(observed[0]).To(HaveKeyWithValue("role", "implementer"))
				Expect(observed[0]["argv"]).To(ContainElement(harness + "/default"))
				if !fail {
					Expect(observed[1]).To(HaveKeyWithValue("role", "reviewer"))
					Expect(observed[1]["argv"]).To(ContainElement(harness + "/frontier"))
				}
				for _, call := range observed {
					Expect(call).To(HaveKeyWithValue("cwd", project))
					Expect(call).To(HaveKeyWithValue("pid_published_before_input", true))
				}
				ledger, err := os.ReadFile(filepath.Join(project, ".factory/budget.json"))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(ledger)).NotTo(ContainSubstring("PRIVATE_"))
				rows := budgetDecode(ledger).(map[string]any)["runs"].([]any)
				Expect(rows).To(HaveLen(calls))
				Expect(rows[0]).To(HaveKeyWithValue("status", "completed"))
				if fail {
					Expect(rows[0]).To(HaveKeyWithValue("exit_code", json.Number("7")))
				}
				checkpoint, err := os.ReadFile(filepath.Join(project, ".factory/loops.json"))
				Expect(err).NotTo(HaveOccurred())
				out = handoffNativeExactProcess(project, environment, "", program, args...)
				Expect(out.status).To(Equal(2), "a repeated run cannot automatically retry a paid call: %+v", out)
				Expect(os.ReadFile(filepath.Join(project, ".factory/loops.json"))).To(Equal(checkpoint))
				Expect(os.ReadFile(filepath.Join(project, ".factory/budget.json"))).To(Equal(ledger))
				Expect(sourceLoopExecutions(project)).To(Equal(observed))
			}
		}
	})
	// per docs/adr/0140-go-source-loop-and-python-retirement.md:27
	// per docs/adr/0140-go-source-loop-and-python-retirement.md:63
	It("cancels acknowledged source work on INT TERM and HUP with durable checkpoints and owned cleanup", func() {
		for _, signal := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP} {
			project, _, program, environment := sourceLoopNativeFixture()
			environment["CONTROLLER_FIXTURE_MODE"] = "stall"
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			command := exec.CommandContext(ctx, program, sourceLoopArgs(project, "run", "claude", "bounded")...) // #nosec G204 G702 -- actual source adapter in a private native fixture.
			command.Dir, command.Env = project, sourceSelftestEnvironment(environment)
			command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			Expect(command.Start()).To(Succeed())
			done := make(chan error, 1)
			go func() { defer close(done); done <- command.Wait() }()
			child, group := 0, 0
			callerGroup := syscall.Getpgrp()
			func() {
				defer func() { Expect(sourceBudgetCleanup(command, done, cancel, project, child, group)).To(Succeed()) }()
				Eventually(func() bool {
					data, err := os.ReadFile(environment["CONTROLLER_FIXTURE_LOG"])
					return err == nil && strings.HasSuffix(string(data), "\n") && strings.Contains(string(data), `"pid_published_before_input":true`)
				}, 5*time.Second).Should(BeTrue())
				calls := sourceLoopExecutions(project)
				Expect(calls).To(HaveLen(1))
				var err error
				child, err = strconv.Atoi(string(calls[0]["pid"].(json.Number)))
				Expect(err).NotTo(HaveOccurred())
				group, err = syscall.Getpgid(child)
				Expect(err).NotTo(HaveOccurred())
				Expect(group).To(Equal(child))
				Expect(group).NotTo(Equal(callerGroup))
				Expect(command.Process.Signal(signal)).To(Succeed())
				var waitErr error
				Eventually(done, 8*time.Second).Should(Receive(&waitErr))
				Expect(ctx.Err()).NotTo(HaveOccurred())
				var exited *exec.ExitError
				Expect(errors.As(waitErr, &exited)).To(BeTrue())
				Expect(exited.ExitCode()).To(Equal(2), "%s stdout=%s stderr=%s", signal, stdout.String(), stderr.String())
				Expect(stderr.String()).To(BeEmpty())
				Expect(sourceBudgetRecord(project)).To(HaveKeyWithValue("outcome", "interrupted"))
				checkpoint := manualStored(project)["runs"].([]any)
				Expect(checkpoint).To(HaveLen(1))
				Expect(checkpoint[0]).To(HaveKeyWithValue("status", "stopped"))
				Expect(checkpoint[0]).To(HaveKeyWithValue("outcome", "interrupted"))
				Expect(sourceLoopExecutions(project)).To(HaveLen(1))
				Eventually(func() bool {
					return errors.Is(syscall.Kill(child, 0), syscall.ESRCH) && errors.Is(syscall.Kill(-group, 0), syscall.ESRCH)
				}, time.Second).Should(BeTrue())
				Expect(syscall.Getpgrp()).To(Equal(callerGroup))
				Expect(syscall.Kill(-callerGroup, 0)).To(Succeed())
			}()
		}
	})
})
