package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Native orchestration review cutover inherited outcomes", func() {
	// per docs/adr/0099-native-factory-orchestration.md:126
	DescribeTable("refuses invalid provider JSON without emitting private content or retrying", func(body string) {
		root, cwd, path, environment := orchestrationReviewFixture("+PRIVATE_DIFF")
		service := orchestrationReviewHTTP(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) { _, _ = io.WriteString(w, body) })
		result := orchestrationReview(root, cwd, orchestrationReviewEnvironment(environment, service), path)
		Expect(result.status).To(Equal(1))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(ContainSubstring("PRIVATE"))
		Expect(service.observed()).To(HaveLen(1))
	}, Entry("malformed JSON", `{"PRIVATE_BROKEN":`), Entry("provider error", `{"error":{"message":"PRIVATE_PROVIDER_ERROR"}}`), Entry("missing content", `{}`), Entry("empty content", `{"choices":[{"message":{"content":""}}]}`))
	// per docs/adr/0099-native-factory-orchestration.md:120
	DescribeTable("keeps native request defaults and the historical OpenAI length behavior", func(provider, extra string, tokens bool) {
		root, cwd, path, environment := orchestrationReviewFixture("+PRIVATE_DIFF")
		service := orchestrationReviewHTTP(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
			finish := "stop"
			if provider == "openai" {
				finish = "length"
			}
			_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"`+finish+`","message":{"content":"No findings."}}]}`)
		})
		environment = orchestrationReviewEnvironment(environment, service, "MODEL_PROVIDER="+provider)
		if extra != "" {
			environment = append(environment, extra)
		}
		Expect(orchestrationReview(root, cwd, environment, path)).To(Equal(cliResult{"No findings.\n", "", 0}))
		requests := service.observed()
		Expect(requests).To(HaveLen(1))
		var body map[string]any
		Expect(json.Unmarshal(requests[0].body, &body)).To(Succeed())
		if tokens {
			Expect(body["max_tokens"]).To(Equal(float64(8192)))
		} else {
			Expect(body).NotTo(HaveKey("max_tokens"))
		}
		Expect(body).NotTo(HaveKey("reasoning"))
		Expect(body).NotTo(HaveKey("provider"))
	}, Entry("unconfigured token cap", "openrouter", "", true), Entry("explicit empty cap", "openrouter", "REVIEW_MAX_TOKENS=", true), Entry("explicit empty client uses native HTTP", "openrouter", "REVIEW_GO_CLIENT=", true), Entry("OpenAI historical length", "openai", "", false))
	// per docs/adr/0099-native-factory-orchestration.md:123
	It("refuses invalid selected clients without executing data or contacting a provider", func() {
		root, cwd, path, environment := orchestrationReviewFixture("+PRIVATE_DIFF")
		service := orchestrationReviewHTTP(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) { _, _ = io.WriteString(w, `{}`) })
		writeFixture(filepath.Join(cwd, "not executable"), []byte("PRIVATE_NOT_EXECUTABLE"), 0600)
		for _, client := range []string{"relative-client", filepath.Join(cwd, "absent"), cwd, filepath.Join(cwd, "not executable"), filepath.Join(cwd, "$(touch injected)")} {
			result := orchestrationReview(root, cwd, orchestrationReviewEnvironment(environment, service, "REVIEW_GO_CLIENT="+client), path)
			Expect(result.status).To(Equal(1))
			Expect(result.stdout).To(BeEmpty())
			Expect(result.stderr).To(ContainSubstring("REVIEW_GO_CLIENT"))
		}
		Expect(service.observed()).To(BeEmpty())
		_, err := os.Stat(filepath.Join(cwd, "injected"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0099-native-factory-orchestration.md:123
	DescribeTable("keeps non-OpenRouter providers on native HTTP despite a selected client", func(provider string) {
		root, cwd, path, environment := orchestrationReviewFixture("+PRIVATE_DIFF")
		marker := filepath.Join(cwd, "selected-was-run")
		client := filepath.Join(cwd, "selected client")
		writeFixture(client, []byte("#!/bin/sh\nprintf x > \"$SELECTED_MARKER\"\nexit 91\n"), 0700)
		service := orchestrationReviewHTTP(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
			if provider == "anthropic" {
				_, _ = io.WriteString(w, `{"content":[{"text":"No findings."}]}`)
			} else {
				_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"No findings."}}]}`)
			}
		})
		result := orchestrationReview(root, cwd, orchestrationReviewEnvironment(environment, service, "MODEL_PROVIDER="+provider, "REVIEW_GO_CLIENT="+client, "SELECTED_MARKER="+marker), path)
		Expect(result).To(Equal(cliResult{"No findings.\n", "", 0}))
		Expect(service.observed()).To(HaveLen(1))
		_, err := os.Stat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("Anthropic", "anthropic"), Entry("OpenAI", "openai"))
	// per docs/adr/0099-native-factory-orchestration.md:126
	DescribeTable("times out an actual stalled JSON response without publishing its partial answer", func(provider string) {
		root, cwd, path, environment := orchestrationReviewFixture("+PRIVATE_DIFF")
		started, closed := make(chan struct{}), make(chan struct{})
		service := orchestrationReviewHTTP(cwd, func(w http.ResponseWriter, r *http.Request, _ int) {
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"PRIVATE_PARTIAL"}}]}`)
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
			close(closed)
		})
		result := orchestrationReview(root, cwd, orchestrationReviewEnvironment(environment, service, "MODEL_PROVIDER="+provider, "REVIEW_TIMEOUT_SECONDS=1"), path)
		Expect(started).To(BeClosed(), "fixture must reach the real response body")
		Expect(result.status).To(Equal(1))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).To(ContainSubstring("timed out"))
		Expect(result.stderr).NotTo(ContainSubstring("PRIVATE"))
		Expect(service.observed()).To(HaveLen(1))
		Eventually(closed, 2*time.Second).Should(BeClosed())
	}, Entry("OpenRouter", "openrouter"), Entry("OpenAI", "openai"), Entry("Anthropic", "anthropic"))
	// per docs/adr/0099-native-factory-orchestration.md:126
	It("cancels an observed direct HTTP response and leaves no published partial review", func() {
		root, cwd, path, environment := orchestrationReviewFixture("+PRIVATE_DIFF")
		started, closed := make(chan struct{}), make(chan struct{})
		service := orchestrationReviewHTTP(cwd, func(w http.ResponseWriter, r *http.Request, _ int) {
			_, _ = io.WriteString(w, "PRIVATE_PARTIAL")
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
			close(closed)
		})
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, filepath.Join(root, "factory"), "review", path) // #nosec G204 G702 -- compiled candidate, literal diff path and fixed TLS fixture only.
		cmd.Dir, cmd.Env = cwd, orchestrationReviewEnvironment(environment, service)
		var output, diagnostic bytes.Buffer
		cmd.Stdout, cmd.Stderr = &output, &diagnostic
		Expect(cmd.Start()).To(Succeed())
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		Eventually(started, 3*time.Second).Should(BeClosed())
		Expect(cmd.Process.Signal(syscall.SIGTERM)).To(Succeed())
		var err error
		Eventually(done, 3*time.Second).Should(Receive(&err))
		Expect(err).To(HaveOccurred())
		Expect(ctx.Err()).NotTo(HaveOccurred())
		Expect(output.String()).To(BeEmpty())
		Expect(diagnostic.String()).NotTo(ContainSubstring("PRIVATE"))
		Expect(service.observed()).To(HaveLen(1))
		Eventually(closed, 2*time.Second).Should(BeClosed())
	})
})

// per docs/LOOPS.md:96
func cutoverControllerFixture() (string, string, string, []string) {
	GinkgoHelper()
	root, cwd, checkout, environment := boundedFixture()
	config := filepath.Join(checkout, "factory.yaml")
	writeFixture(config, []byte("budget_enabled: true\nbudget_max_attempts: 12\nbudget_max_session_runs: 12\nbudget_max_concurrent: 2\nbudget_timeout_seconds: 30\nbudget_session_seconds: 90\nloop_enabled: true\nloop_max_attempts: 3\nloop_timeout_seconds: 30\nloop_check_timeout_seconds: 3\nloop_check_command: true\n"), 0600)
	return root, cwd, checkout, append(environment, "FACTORY_CONFIG="+config, "GORACE=atexit_sleep_ms=0")
}

func cutoverControllerArgs(kind, cwd, session string) []string {
	args := []string{kind, "run", "--harness=claude", "--session=" + session, "--task=t", "--prompt-file=" + filepath.Join(cwd, "prompt.txt"), "--json"}
	if kind == "loop" {
		args = append(args, "--mode=bounded")
	}
	return args
}

var _ = Describe("G2 budget and loop paid controller interruption", func() {
	// per docs/BUDGETS.md:129
	// per docs/LOOPS.md:68
	DescribeTable("preserves a charged live reservation after SIGKILL and refuses a second named controller", func(kind string) {
		root, cwd, checkout, environment := cutoverControllerFixture()
		environment = append(environment, "BOUNDED_SCENARIO=stall")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		cmd := exec.CommandContext(ctx, filepath.Join(root, "factory"), cutoverControllerArgs(kind, cwd, "original")...) // #nosec G204 G702 -- public compiled candidate, isolated Git checkout and fake model executables only.
		cmd.Dir, cmd.Env = checkout, environment
		var output, diagnostic bytes.Buffer
		cmd.Stdout, cmd.Stderr = &output, &diagnostic
		Expect(cmd.Start()).To(Succeed())
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		pid := 0
		DeferCleanup(func() {
			if pid > 0 {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
				Eventually(func() bool { return transitionAlive(checkout, pid) }, 3*time.Second).Should(BeFalse())
			}
			cancel()
		})
		Eventually(func() bool {
			raw, err := os.ReadFile(filepath.Join(cwd, "calls.jsonl"))
			if err != nil {
				return false
			}
			var call map[string]any
			if json.Unmarshal(bytes.TrimSpace(raw), &call) != nil {
				return false
			}
			value, ok := call["pid"].(float64)
			if ok {
				pid = int(value)
			}
			return pid > 0
		}, 5*time.Second).Should(BeTrue(), "paid fake must consume stdin after durable PID publication")
		budgetPath := filepath.Join(checkout, ".factory/budget.json")
		before, err := os.ReadFile(budgetPath)
		Expect(err).NotTo(HaveOccurred())
		var history map[string]any
		Expect(json.Unmarshal(before, &history)).To(Succeed())
		rows := history["runs"].([]any)
		Expect(rows).To(HaveLen(1))
		row := rows[0].(map[string]any)
		Expect(row["status"]).To(Equal("active"))
		Expect(row["owner_pid"]).To(Equal(float64(cmd.Process.Pid)))
		Expect(row["process_pid"]).To(Equal(float64(pid)))
		Expect(row["attempt"]).To(Equal(float64(1)))
		Expect(row["reserved_seconds"]).To(BeNumerically(">", 0))
		var checkpoint []byte
		if kind == "loop" {
			checkpoint, err = os.ReadFile(filepath.Join(checkout, ".factory/loops.json"))
			Expect(err).NotTo(HaveOccurred())
			var stored map[string]any
			Expect(json.Unmarshal(checkpoint, &stored)).To(Succeed())
			loopRows := stored["runs"].([]any)
			Expect(loopRows).To(HaveLen(1))
			Expect(loopRows[0].(map[string]any)["status"]).To(Equal("active"))
		}
		Expect(cmd.Process.Kill()).To(Succeed())
		var killed error
		Eventually(done, 3*time.Second).Should(Receive(&killed))
		var exit *exec.ExitError
		Expect(errors.As(killed, &exit)).To(BeTrue())
		Expect(exit.ProcessState.Sys().(syscall.WaitStatus).Signal()).To(Equal(syscall.SIGKILL))
		Expect(syscall.Kill(pid, 0)).To(Succeed(), "child must remain alive after its supervisor is killed")
		Expect(transitionAlive(checkout, pid)).To(BeTrue(), "the held paid child must still execute, not merely retain a zombie PID")
		calls, err := os.ReadFile(filepath.Join(cwd, "calls.jsonl"))
		Expect(err).NotTo(HaveOccurred())
		secondEnvironment := append(append([]string{}, environment...), "BOUNDED_SCENARIO=approve")
		result := orchestrationCommand(root, checkout, secondEnvironment, filepath.Join(root, "factory"), cutoverControllerArgs(kind, cwd, "second")...)
		Expect(result.status).To(Equal(2), "%+v", result)
		if kind == "budget" {
			Expect(result.stdout).To(ContainSubstring("stale active run"))
		} else {
			Expect(result.stdout).To(ContainSubstring("active budget invocation"))
		}
		after, err := os.ReadFile(budgetPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		laterCalls, err := os.ReadFile(filepath.Join(cwd, "calls.jsonl"))
		Expect(err).NotTo(HaveOccurred())
		Expect(laterCalls).To(Equal(calls))
		if kind == "loop" {
			after, err = os.ReadFile(filepath.Join(checkout, ".factory/loops.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(after).To(Equal(checkpoint))
		}
	}, Entry("budget", "budget"), Entry("loop", "loop"))
})

var _ = Describe("G2 bounded loop observed check ordering", func() {
	// per docs/LOOPS.md:42
	// per docs/LOOPS.md:59
	DescribeTable("executes the check before its independent reviewer and stops on reviewer process failure", func(harness string, reviewFails bool) {
		root, cwd, checkout, environment := cutoverControllerFixture()
		receipt := filepath.Join(cwd, "checked receipt")
		environment = append(environment, "CUTOVER_CHECK_RECEIPT="+receipt)
		if reviewFails {
			environment = append(environment, "CUTOVER_REVIEW_FAIL=1")
		}
		program := filepath.Join(root, "bin", harness)
		source, err := os.ReadFile(program)
		Expect(err).NotTo(HaveOccurred())
		anchor := "scenario=os.environ.get('BOUNDED_SCENARIO','approve')\n"
		Expect(string(source)).To(ContainSubstring(anchor))
		source = bytes.Replace(source, []byte(anchor), []byte(anchor+"if role=='reviewer' and not Path(os.environ['CUTOVER_CHECK_RECEIPT']).is_file(): sys.exit(91)\n"), 1)
		source = append(source, []byte("\nif role=='reviewer' and os.environ.get('CUTOVER_REVIEW_FAIL')=='1': sys.exit(7)\n")...)
		writeFixture(program, source, 0700)
		config := filepath.Join(checkout, "factory.yaml")
		settings, err := os.ReadFile(config)
		Expect(err).NotTo(HaveOccurred())
		check := "printf checked > \"$CUTOVER_CHECK_RECEIPT\"; printf '%s\\n' '{\"role\":\"check\"}' >> \"$BOUNDED_LOG\""
		settings = bytes.Replace(settings, []byte("loop_check_command: true"), []byte("loop_check_command: "+check), 1)
		writeFixture(config, settings, 0600)
		args := cutoverControllerArgs("loop", cwd, "ordered")
		args[2] = "--harness=" + harness
		result := orchestrationCommand(root, checkout, environment, filepath.Join(root, "factory"), args...)
		row := manualRecord(result)
		if reviewFails {
			Expect(result.status).To(Equal(2))
			Expect(row["outcome"]).To(Equal("handoff"))
		} else {
			Expect(result.status).To(BeZero(), "%+v", result)
			Expect(row["outcome"]).To(Equal("approved"))
		}
		calls := boundedCalls(cwd)
		roles := make([]string, 0, len(calls))
		for _, call := range calls {
			roles = append(roles, call["role"].(string))
		}
		Expect(roles).To(Equal([]string{"implementer", "check", "reviewer"}))
		Expect(row["budget_runs"]).To(HaveLen(2), "failed reviewers still consume their own attempt")
		content, err := os.ReadFile(filepath.Join(checkout, "source.txt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(Equal("implementation 1\n"), "handoff must preserve useful implementation changes")
	}, Entry("Codex ordered approval", "codex", false), Entry("Claude ordered approval", "claude", false), Entry("OpenCode ordered approval", "opencode", false), Entry("reviewer process failure after passed check", "claude", true))
})
