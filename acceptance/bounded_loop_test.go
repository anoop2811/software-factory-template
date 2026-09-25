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

func boundedCommand(root, cwd, checkout string, oracle bool, args []string, extra ...string) cliResult {
	environment := append([]string{"FACTORY_BUDGET_ROOT=" + checkout, "FACTORY_LOOP_CHECK_COMMAND=true"}, extra...)
	if oracle {
		return loopProcess(cwd, "python3", append([]string{"-B", filepath.Join(root, "oracle/loop.py")}, args...), environment)
	}
	return loopProcess(cwd, filepath.Join(root, "factory"), append([]string{"loop", "command"}, args...), environment)
}

var _ = Describe("G2 bounded loop command core", func() {
	// per docs/adr/0076-go-bounded-loop-controller.md:19
	DescribeTable("matches immutable planning without creating checkpoint infrastructure", func(mode string, status int) {
		root, cwd, checkout := loopFixture()
		args := []string{"plan", "--harness", "codex", "--session", "s", "--task", "t", "--mode", mode, "--json"}
		oracle := boundedCommand(root, cwd, checkout, true, args)
		Expect(oracle.status).To(Equal(status), "%+v", oracle)
		actual := boundedCommand(root, cwd, checkout, false, args)
		Expect(actual.status).To(Equal(status), "%+v", actual)
		Expect(actual.stderr).To(BeEmpty())
		Expect(manualComparable(actual.stdout)).To(Equal(manualComparable(oracle.stdout)))
		_, err := os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("manual default permission", "manual", 0), Entry("bounded disabled blockers", "bounded", 2))
	// per docs/adr/0076-go-bounded-loop-controller.md:25
	It("reads status without interpreting unrelated invalid execution configuration", func() {
		root, cwd, checkout := loopFixture()
		writeFixture(filepath.Join(checkout, ".factory/loops.json"), []byte(checkpointHistory(checkpointRecord)), 0600)
		args := []string{"status", "--session", "s", "--task", "t", "--json"}
		extra := []string{"FACTORY_LOOP_TIMEOUT_SECONDS=PRIVATE_INVALID"}
		oracle := boundedCommand(root, cwd, checkout, true, args, extra...)
		Expect(oracle.status).To(BeZero(), "%+v", oracle)
		actual := boundedCommand(root, cwd, checkout, false, args, extra...)
		Expect(actual.status).To(BeZero(), "%+v", actual)
		Expect(actual.stderr).To(BeEmpty())
		Expect(budgetDecode([]byte(actual.stdout))).To(Equal(budgetDecode([]byte(oracle.stdout))))
	})
})

// per docs/adr/0076-go-bounded-loop-controller.md:122
const boundedFake = `import sys,os,json,time
from pathlib import Path
if len(sys.argv)>1 and sys.argv[1]=='app-server':
 os.execv(os.environ['BOUNDED_PROBE'],[os.environ['BOUNDED_PROBE']]+sys.argv[1:])
if '--help' in sys.argv:
 print('--json --sandbox --cd --model --config --print --output-format --agent --permission-mode --format');sys.exit()
role=os.environ['FACTORY_AGENT_ROLE']; prompt=sys.stdin.read(); path=Path(os.environ['BOUNDED_LOG'])
previous=[json.loads(x) for x in path.read_text().splitlines()] if path.exists() else []
count=sum(x['role']==role for x in previous)+1
with path.open('a') as f: f.write(json.dumps({'role':role,'prompt':prompt,'argv':sys.argv[1:],'overlay':os.environ.get('OPENCODE_CONFIG_CONTENT'),'pid':os.getpid()})+'\n')
scenario=os.environ.get('BOUNDED_SCENARIO','approve')
if scenario=='stall': time.sleep(30)
if role=='implementer':
 if scenario not in ('no_progress','repeated_failure'): Path('source.txt').write_text('implementation '+str(count)+'\n')
 if scenario=='safety': Path('AGENTS.md').write_text('PRIVATE_MUTATION')
 if scenario=='prompt': Path(os.environ['BOUNDED_PROMPT']).write_text('PRIVATE_CHANGED_PROMPT')
 answer='PRIVATE_IMPLEMENTER_RESPONSE'
else:
 if scenario=='review_mutation': Path('source.txt').write_text('review changed source')
 answer=os.environ.get('BOUNDED_VERDICT','{"verdict":"approve","findings":[]}')
 if scenario=='review_repair' and count==1: answer='{"verdict":"repair","findings":["PRIVATE_FINDING"]}'
 if scenario=='attempt_limit': answer='{"verdict":"repair","findings":["PRIVATE_FINDING"]}'
harness=Path(sys.argv[0]).name
if harness=='codex':
 print(json.dumps({'type':'item.completed','item':{'type':'agent_message','text':answer}}));print(json.dumps({'type':'turn.completed','usage':{'input_tokens':3,'cached_input_tokens':1,'output_tokens':2}}))
elif harness=='claude':
 print(json.dumps({'type':'result','subtype':'success','result':answer,'usage':{'input_tokens':3,'output_tokens':2,'cache_creation_input_tokens':0,'cache_read_input_tokens':1},'total_cost_usd':0.1}))
else:
 print(json.dumps({'type':'text','part':{'sessionID':'s','messageID':'m','id':'text','text':answer}}));print(json.dumps({'type':'step_finish','part':{'sessionID':'s','messageID':'m','id':'step','reason':'stop','cost':0.1,'tokens':{'input':3,'output':2,'reasoning':0,'cache':{'read':1,'write':0}}}}))
if scenario=='native_failure':sys.exit(7)
`

func boundedFixture() (string, string, string, []string) {
	GinkgoHelper()
	root, cwd, checkout := loopFixture()
	nativeBuild()
	probe := filepath.Join(root, "native-probe")
	writeFixture(probe, nativeDriver, 0755)
	python := loopProcess(cwd, "python3", []string{"-B", "-c", "import sys; print(sys.executable)"}, nil)
	Expect(python.status).To(BeZero())
	for _, h := range []string{"codex", "claude", "opencode"} {
		writeFixture(filepath.Join(root, "bin", h), []byte("#!"+strings.TrimSpace(python.stdout)+"\n"+boundedFake), 0755)
	}
	for _, role := range []string{"implementer", "reviewer"} {
		writeFixture(filepath.Join(checkout, ".opencode/agent", role+".md"), []byte("---\nname: role\n---\nCanonical role instructions\n"), 0600)
		writeFixture(filepath.Join(checkout, ".claude/agents", role+".md"), []byte("Native instructions\n"), 0600)
	}
	writeFixture(filepath.Join(checkout, "opencode.json"), []byte(`{"agent":{"reviewer":{"permission":{"edit":"deny"}},"implementer":{"permission":{"edit":"allow"}}}}`), 0600)
	writeFixture(filepath.Join(checkout, "scripts/hooks/test-edit-denial.sh"), []byte("#!/bin/sh\nexit 0\n"), 0755)
	prompt := filepath.Join(cwd, "prompt.txt")
	writeFixture(prompt, []byte("PRIVATE_TASK\r\nsecond\rthird\n"), 0600)
	environment := []string{"PATH=" + filepath.Join(root, "bin") + string(os.PathListSeparator) + os.Getenv("PATH"), "BOUNDED_PROBE=" + probe, "BOUNDED_LOG=" + filepath.Join(cwd, "calls.jsonl"), "BOUNDED_PROMPT=" + prompt, "FACTORY_LOOP_ENABLED=true", "FACTORY_BUDGET_ENABLED=true", "FACTORY_BUDGET_MAX_ATTEMPTS=12", "FACTORY_BUDGET_MAX_SESSION_RUNS=12", "FACTORY_LOOP_MAX_ATTEMPTS=3", "FACTORY_LOOP_NO_PROGRESS_LIMIT=2"}
	return root, cwd, checkout, environment
}
func boundedArgs(action, harness, cwd string) []string {
	return []string{action, "--harness", harness, "--session", "s", "--task", "t", "--mode", "bounded", "--prompt-file", filepath.Join(cwd, "prompt.txt"), "--json"}
}
func boundedCalls(cwd string) []map[string]any {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join(cwd, "calls.jsonl"))
	if os.IsNotExist(err) {
		return nil
	}
	Expect(err).NotTo(HaveOccurred())
	result := []map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var row map[string]any
		Expect(json.Unmarshal([]byte(line), &row)).To(Succeed())
		result = append(result, row)
	}
	return result
}

var _ = Describe("G2 bounded loop execution", func() {
	// per docs/adr/0076-go-bounded-loop-controller.md:101
	DescribeTable("approves after separate budgeted implementation and review", func(harness string) {
		root, cwd, checkout, environment := boundedFixture()
		prefix := "FACTORY_LOOP_" + strings.ToUpper(harness)
		environment = append(environment, prefix+"_IMPLEMENTER_MODEL=fixture/implementation", prefix+"_REVIEWER_MODEL=fixture/review")
		out := boundedCommand(root, cwd, checkout, false, boundedArgs("run", harness, cwd), environment...)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stderr).To(BeEmpty())
		row := manualRecord(out)
		Expect(row["status"]).To(Equal("completed"))
		Expect(row["outcome"]).To(Equal("approved"))
		Expect(row["attempts"]).To(Equal(json.Number("1")))
		Expect(row["budget_runs"]).To(HaveLen(2))
		Expect(row["uncertain"]).To(BeFalse())
		calls := boundedCalls(cwd)
		Expect(calls).To(HaveLen(2))
		Expect(calls[0]["role"]).To(Equal("implementer"))
		Expect(calls[1]["role"]).To(Equal("reviewer"))
		Expect(calls[0]["prompt"]).To(ContainSubstring("PRIVATE_TASK\nsecond\nthird\n"))
		Expect(calls[0]["argv"]).To(ContainElement("fixture/implementation"))
		Expect(calls[1]["argv"]).To(ContainElement("fixture/review"))
		for _, name := range []string{"loops.json", "budget.json"} {
			raw, err := os.ReadFile(filepath.Join(checkout, ".factory", name))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(raw)).NotTo(ContainSubstring("PRIVATE_TASK"))
			Expect(string(raw)).NotTo(ContainSubstring("PRIVATE_IMPLEMENTER_RESPONSE"))
		}
		ledger, err := os.ReadFile(filepath.Join(checkout, ".factory/budget.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(budgetDecode(ledger).(map[string]any)["runs"]).To(HaveLen(2))
	}, Entry("Codex", "codex"), Entry("Claude", "claude"), Entry("OpenCode", "opencode"))
})

var _ = Describe("G2 bounded loop iteration", func() {
	// per docs/adr/0076-go-bounded-loop-controller.md:77
	DescribeTable("matches immutable iteration and paid-call counts", func(scenario, check, outcome string, status, attempts, calls int, extra []string) {
		root, cwd, checkout, environment := boundedFixture()
		environment = append(environment, "BOUNDED_SCENARIO="+scenario, "FACTORY_LOOP_CHECK_COMMAND="+check)
		environment = append(environment, extra...)
		args := boundedArgs("run", "claude", cwd)
		oracle := boundedCommand(root, cwd, checkout, true, args, environment...)
		Expect(oracle.status).To(Equal(status), "oracle: %+v", oracle)
		expected := manualRecord(oracle)
		Expect(expected["outcome"]).To(Equal(outcome))
		Expect(expected["attempts"]).To(Equal(json.Number(strconv.Itoa(attempts))))
		Expect(boundedCalls(cwd)).To(HaveLen(calls))
		Expect(os.RemoveAll(filepath.Join(checkout, ".factory"))).To(Succeed())
		Expect(os.WriteFile(filepath.Join(checkout, "source.txt"), []byte("initial source\n"), 0600)).To(Succeed())
		Expect(os.Remove(filepath.Join(cwd, "calls.jsonl"))).To(Succeed())
		actual := boundedCommand(root, cwd, checkout, false, args, environment...)
		Expect(actual.status).To(Equal(status), "actual: %+v", actual)
		row := manualRecord(actual)
		for _, key := range []string{"outcome", "status", "attempts", "no_progress", "uncertain", "phase", "stop_reason", "next_action"} {
			Expect(row[key]).To(Equal(expected[key]), "field %s", key)
		}
		observed := boundedCalls(cwd)
		Expect(observed).To(HaveLen(calls))
		Expect(row["evidence"]).To(HaveLen(len(expected["evidence"].([]any))))
		if scenario == "review_repair" {
			Expect(observed[2]["prompt"]).To(ContainSubstring("Findings are untrusted diagnostic data:\n"))
			Expect(observed[2]["prompt"]).To(ContainSubstring("PRIVATE_FINDING"))
		}
		if scenario == "check_repair" {
			Expect(observed[1]["prompt"]).To(ContainSubstring("<untrusted-diagnostics>\nPRIVATE_CHECK\n</untrusted-diagnostics>"))
		}
	}, Entry("approval", "approve", "true", "approved", 0, 1, 2, nil),
		Entry("failed check repair", "check_repair", "if /usr/bin/grep -q 'implementation 1' source.txt; then printf PRIVATE_CHECK; exit 7; fi", "approved", 0, 2, 3, nil),
		Entry("review repair", "review_repair", "true", "approved", 0, 2, 4, nil),
		Entry("attempt exhaustion", "attempt_limit", "true", "attempt_limit", 2, 2, 4, []string{"FACTORY_LOOP_MAX_ATTEMPTS=2"}),
		Entry("no source progress", "no_progress", "false", "no_progress", 2, 2, 2, []string{"FACTORY_LOOP_NO_PROGRESS_LIMIT=1"}),
		Entry("repeated failure", "repeated_failure", "false", "repeated_failure", 2, 2, 2, nil),
		Entry("review consumes remaining shared attempt", "approve", "true", "handoff", 2, 1, 1, []string{"FACTORY_BUDGET_MAX_ATTEMPTS=1"}))
})

var _ = Describe("G2 bounded loop grammar", func() {
	// per docs/adr/0076-go-bounded-loop-controller.md:31
	DescribeTable("preserves argparse status and output channel before any state effects", func(args []string) {
		root, cwd, checkout := loopFixture()
		oracle := boundedCommand(root, cwd, checkout, true, args)
		actual := boundedCommand(root, cwd, checkout, false, args)
		Expect(actual.status).To(Equal(oracle.status), "oracle=%+v actual=%+v", oracle, actual)
		if oracle.status == 0 && strings.Contains(oracle.stdout, "usage:") {
			Expect(actual.stdout).To(ContainSubstring("--help"))
			Expect(actual.stderr).To(BeEmpty())
		} else if oracle.status == 2 && oracle.stdout == "" {
			Expect(actual.stdout).To(BeEmpty())
			Expect(actual.stderr).NotTo(BeEmpty())
			Expect(actual.stderr).NotTo(ContainSubstring("PRIVATE"))
		} else {
			Expect(manualComparable(actual.stdout)).To(Equal(manualComparable(oracle.stdout)))
			Expect(actual.stderr).To(BeEmpty())
		}
		_, err := os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("root help", []string{"--help"}), Entry("leaf help", []string{"run", "-h"}), Entry("help before ordinary unknown", []string{"plan", "-h", "--PRIVATE_UNKNOWN"}), Entry("invalid help value", []string{"status", "--help=false"}), Entry("loop root attached help letter", []string{"-h=h"}), Entry("loop root attached repeated help", []string{"-h=hh"}), Entry("loop leaf attached help letter", []string{"plan", "-h=h"}), Entry("loop leaf attached repeated help", []string{"plan", "-h=hh"}), Entry("loop root empty help value", []string{"-h="}), Entry("loop leaf empty help value", []string{"plan", "-h="}), Entry("missing action", []string{}), Entry("missing required", []string{"run"}), Entry("unknown private option", []string{"status", "--PRIVATE_UNKNOWN"}), Entry("missing scalar", []string{"run", "--session", "--PRIVATE_NEXT"}), Entry("invalid choice", []string{"plan", "--session=s", "--task=t", "--harness=PRIVATE_INVALID"}), Entry("hidden completion", []string{"status", "__complete", "--session"}), Entry("short help cluster", []string{"plan", "-hhh"}), Entry("unique prefixes", []string{"plan", "--sess=s", "--ta=t", "--har=claude", "--j"}), Entry("duplicate last wins", []string{"plan", "--session=first", "--session=s", "--task=t", "--harness=claude", "--json"}), Entry("negative session", []string{"plan", "--session=-2", "--task=t", "--harness=claude", "--json"}), Entry("attached JSON bool", []string{"plan", "--session=s", "--task=t", "--harness=claude", "--json=true"}))
	// per docs/adr/0076-go-bounded-loop-controller.md:40
	It("ignores manual prompt-file and matches human run output", func() {
		root, cwd, checkout := loopFixture()
		args := []string{"run", "--session=s", "--task=t", "--harness=claude", "--prompt-file=PRIVATE_MISSING"}
		oracle := boundedCommand(root, cwd, checkout, true, args)
		Expect(oracle.status).To(BeZero(), "%+v", oracle)
		Expect(os.RemoveAll(filepath.Join(checkout, ".factory"))).To(Succeed())
		actual := boundedCommand(root, cwd, checkout, false, args)
		Expect(actual.status).To(BeZero(), "%+v", actual)
		Expect(actual.stdout).To(Equal(oracle.stdout))
		Expect(actual.stderr).To(BeEmpty())
	})
})
var _ = Describe("G2 bounded loop safety", func() {
	// per docs/adr/0076-go-bounded-loop-controller.md:59
	DescribeTable("hands off without further paid calls on unsafe transitions", func(scenario string, expectedCalls int) {
		root, cwd, checkout, environment := boundedFixture()
		environment = append(environment, "BOUNDED_SCENARIO="+scenario)
		out := boundedCommand(root, cwd, checkout, false, boundedArgs("run", "claude", cwd), environment...)
		Expect(out.status).To(Equal(2), "%+v", out)
		row := manualRecord(out)
		Expect(row["outcome"]).To(Equal("handoff"))
		Expect(row["uncertain"]).To(BeFalse())
		Expect(boundedCalls(cwd)).To(HaveLen(expectedCalls))
		Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_IMPLEMENTER_RESPONSE"))
		Expect(out.stderr).NotTo(ContainSubstring("PRIVATE"))
	}, Entry("protected input mutation", "safety", 1), Entry("task prompt mutation", "prompt", 1), Entry("review changes checked source", "review_mutation", 2), Entry("native nonzero is not repair authorization", "native_failure", 1))
	// per docs/adr/0076-go-bounded-loop-controller.md:97
	DescribeTable("refuses malformed review without launching repair", func(verdict string) {
		root, cwd, checkout, environment := boundedFixture()
		environment = append(environment, "BOUNDED_VERDICT="+verdict)
		out := boundedCommand(root, cwd, checkout, false, boundedArgs("run", "claude", cwd), environment...)
		Expect(out.status).To(Equal(2), "%+v", out)
		row := manualRecord(out)
		Expect(row["outcome"]).To(Equal("handoff"))
		Expect(boundedCalls(cwd)).To(HaveLen(2))
		Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_FINDING"))
	}, Entry("duplicate key", `{"verdict":"repair","verdict":"approve","findings":[]}`), Entry("fenced JSON", "```json\n{\"verdict\":\"approve\",\"findings\":[]}\n```"), Entry("approve with finding", `{"verdict":"approve","findings":["PRIVATE_FINDING"]}`), Entry("empty repair", `{"verdict":"repair","findings":[]}`))
	// per docs/adr/0076-go-bounded-loop-controller.md:64
	It("never resumes a terminal bounded loop", func() {
		root, cwd, checkout, environment := boundedFixture()
		out := boundedCommand(root, cwd, checkout, false, boundedArgs("run", "claude", cwd), environment...)
		Expect(out.status).To(BeZero(), "%+v", out)
		before, err := os.ReadFile(filepath.Join(checkout, ".factory/loops.json"))
		Expect(err).NotTo(HaveOccurred())
		out = boundedCommand(root, cwd, checkout, false, boundedArgs("resume", "claude", cwd), environment...)
		Expect(out.status).To(Equal(2))
		Expect(boundedCalls(cwd)).To(HaveLen(2))
		after, err := os.ReadFile(filepath.Join(checkout, ".factory/loops.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
	})
})

var _ = Describe("G2 bounded loop prompt and deadline", func() {
	// per docs/adr/0076-go-bounded-loop-controller.md:56
	DescribeTable("refuses unsafe or oversized prompts before native calls", func(kind string) {
		root, cwd, checkout, environment := boundedFixture()
		prompt := filepath.Join(cwd, "prompt.txt")
		switch kind {
		case "missing":
			Expect(os.Remove(prompt)).To(Succeed())
		case "directory":
			Expect(os.Remove(prompt)).To(Succeed())
			Expect(os.Mkdir(prompt, 0700)).To(Succeed())
		case "fifo":
			Expect(os.Remove(prompt)).To(Succeed())
			Expect(syscall.Mkfifo(prompt, 0600)).To(Succeed())
		case "oversized":
			Expect(os.WriteFile(prompt, []byte(strings.Repeat("x", 1024*1024+1)), 0600)).To(Succeed())
		case "invalid UTF8":
			Expect(os.WriteFile(prompt, []byte{0xff}, 0600)).To(Succeed())
		}
		out := boundedCommand(root, cwd, checkout, false, boundedArgs("run", "claude", cwd), environment...)
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
		Expect(boundedCalls(cwd)).To(BeEmpty())
		_, err := os.Stat(filepath.Join(checkout, ".factory/loops.json"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("missing", "missing"), Entry("directory", "directory"), Entry("FIFO nonblocking", "fifo"), Entry("more than normalized MiB", "oversized"), Entry("invalid UTF8", "invalid UTF8"))
	// per docs/adr/0076-go-bounded-loop-controller.md:74
	It("hands off a timed out native invocation without another paid attempt", func() {
		root, cwd, checkout, environment := boundedFixture()
		environment = append(environment, "BOUNDED_SCENARIO=stall", "FACTORY_BUDGET_TIMEOUT_SECONDS=0.3")
		out := boundedCommand(root, cwd, checkout, false, boundedArgs("run", "claude", cwd), environment...)
		Expect(out.status).To(Equal(2), "%+v", out)
		row := manualRecord(out)
		Expect(row["outcome"]).To(Equal("handoff"))
		Expect(row["budget_runs"]).To(HaveLen(1))
		Expect(row["uncertain"]).To(BeFalse())
		Expect(boundedCalls(cwd)).To(HaveLen(1))
	})
})

var _ = Describe("G2 bounded loop signal", func() {
	// per docs/adr/0076-go-bounded-loop-controller.md:116
	It("interrupts the observed paid child and publishes a certain terminal record without retry", func() {
		root, cwd, checkout, environment := boundedFixture()
		environment = append(environment, "BOUNDED_SCENARIO=stall", "FACTORY_BRIDGE_PROTOCOL=1", "FACTORY_BUDGET_ROOT="+checkout, "FACTORY_LOOP_CHECK_COMMAND=true", "GORACE=atexit_sleep_ms=0")
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		command := exec.CommandContext(ctx, filepath.Join(root, "factory"), append([]string{"loop", "command"}, boundedArgs("run", "claude", cwd)...)...) // #nosec G204 G702 -- test-owned compiled candidate, fixed protocol and literal fixture operands.
		command.Dir = cwd
		for _, item := range os.Environ() {
			if !strings.HasPrefix(item, "FACTORY_") {
				command.Env = append(command.Env, item)
			}
		}
		command.Env = append(command.Env, environment...)
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
			select {
			case <-done:
			case <-time.After(6 * time.Second):
				Fail("candidate did not exit during test cleanup")
			}
		})
		Eventually(func() bool {
			data, err := os.ReadFile(filepath.Join(cwd, "calls.jsonl"))
			if err != nil {
				return false
			}
			var row map[string]any
			if json.Unmarshal(bytes.TrimSpace(data), &row) != nil {
				return false
			}
			pid = int(row["pid"].(float64))
			return pid > 0
		}, 6*time.Second).Should(BeTrue())
		Expect(command.Process.Signal(syscall.SIGTERM)).To(Succeed())
		var waitErr error
		Eventually(done, 8*time.Second).Should(Receive(&waitErr))
		var exited *exec.ExitError
		Expect(errors.As(waitErr, &exited)).To(BeTrue())
		Expect(exited.ExitCode()).To(Equal(2))
		Expect(diagnostic.String()).To(BeEmpty())
		row := manualRecord(cliResult{output.String(), diagnostic.String(), 2})
		Expect(row["outcome"]).To(Equal("interrupted"))
		Expect(row["uncertain"]).To(BeFalse())
		Expect(row["budget_runs"]).To(HaveLen(1))
		Expect(boundedCalls(cwd)).To(HaveLen(1))
		Expect(syscall.Kill(pid, 0)).To(Equal(syscall.ESRCH))
		pid = 0
	})
})

var _ = Describe("G2 bounded loop repair framing", func() {
	// per docs/adr/0076-go-bounded-loop-controller.md:89
	It("clips raw diagnostic bytes before replacement decoding and never persists the output", func() {
		root, cwd, checkout, environment := boundedFixture()
		check := `if /usr/bin/grep -q 'implementation 1' source.txt; then python3 -c 'import sys; sys.stdout.buffer.write(b"x"*16383+"€PRIVATE_TAIL".encode()); sys.exit(7)'; fi`
		environment = append(environment, "FACTORY_LOOP_CHECK_COMMAND="+check)
		out := boundedCommand(root, cwd, checkout, false, boundedArgs("run", "claude", cwd), environment...)
		Expect(out.status).To(BeZero(), "%+v", out)
		calls := boundedCalls(cwd)
		Expect(calls).To(HaveLen(3))
		prompt := calls[1]["prompt"].(string)
		Expect(prompt).To(ContainSubstring("<untrusted-diagnostics>\n" + strings.Repeat("x", 16383) + "�\n</untrusted-diagnostics>"))
		Expect(prompt).NotTo(ContainSubstring("PRIVATE_TAIL"))
		raw, err := os.ReadFile(filepath.Join(checkout, ".factory/loops.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).NotTo(ContainSubstring(strings.Repeat("x", 100)))
	})
})

var _ = Describe("G2 bounded loop command ordering", func() {
	// per docs/adr/0076-go-bounded-loop-controller.md:39
	It("validates source even when bounded planning is disabled", func() {
		root, cwd, checkout := loopFixture()
		Expect(os.RemoveAll(filepath.Join(checkout, ".git"))).To(Succeed())
		args := []string{"plan", "--session=s", "--task=t", "--harness=claude", "--mode=bounded", "--json"}
		oracle := boundedCommand(root, cwd, checkout, true, args)
		Expect(oracle.status).To(Equal(2))
		Expect(oracle.stdout).To(BeEmpty())
		actual := boundedCommand(root, cwd, checkout, false, args)
		Expect(actual.status).To(Equal(2))
		Expect(actual.stdout).To(BeEmpty())
		_, err := os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0076-go-bounded-loop-controller.md:25
	It("returns a blocked plan before requiring or opening a bounded prompt", func() {
		root, cwd, checkout := loopFixture()
		args := []string{"run", "--session=s", "--task=t", "--harness=claude", "--mode=bounded", "--json"}
		oracle := boundedCommand(root, cwd, checkout, true, args)
		actual := boundedCommand(root, cwd, checkout, false, args)
		Expect(actual.status).To(Equal(2))
		Expect(actual.stderr).To(BeEmpty())
		Expect(manualComparable(actual.stdout)).To(Equal(manualComparable(oracle.stdout)))
		_, err := os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0076-go-bounded-loop-controller.md:23
	It("infers the existing manual harness on command resume", func() {
		root, cwd, checkout := loopFixture()
		out := manualLoopCLI(root, cwd, checkout, "run", "opencode")
		Expect(out.status).To(BeZero())
		path := filepath.Join(checkout, ".factory/loops.json")
		before, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		args := []string{"resume", "--session=s", "--task=t", "--json"}
		oracle := boundedCommand(root, cwd, checkout, true, args)
		Expect(oracle.status).To(BeZero(), "%+v", oracle)
		Expect(os.WriteFile(path, before, 0600)).To(Succeed())
		actual := boundedCommand(root, cwd, checkout, false, args)
		Expect(actual.status).To(BeZero(), "%+v", actual)
		Expect(manualComparable(actual.stdout)).To(Equal(manualComparable(oracle.stdout)))
	})
})

var _ = Describe("G2 bounded loop prompt admission ordering", func() {
	// per docs/adr/0076-go-bounded-loop-controller.md:55
	It("refuses a missing admitted prompt before creating loop lock infrastructure", func() {
		root, cwd, checkout, environment := boundedFixture()
		Expect(os.Remove(filepath.Join(cwd, "prompt.txt"))).To(Succeed())
		args := boundedArgs("run", "claude", cwd)
		oracle := boundedCommand(root, cwd, checkout, true, args, environment...)
		Expect(oracle.status).To(Equal(2))
		Expect(oracle.stdout).To(BeEmpty())
		_, err := os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "immutable oracle must not create lock infrastructure")
		actual := boundedCommand(root, cwd, checkout, false, args, environment...)
		Expect(actual.status).To(Equal(2))
		Expect(actual.stdout).To(BeEmpty())
		_, err = os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "candidate must refuse before taking the persistent lock")
	})
})

var _ = Describe("G2 bounded loop qualified help precedence", func() {
	// per docs/adr/0076-go-bounded-loop-controller.md:150
	DescribeTable("keeps fixed Go refusals across known argparse precedence changes", func(args []string) {
		root, cwd, checkout := loopFixture()
		extra := []string{"FACTORY_LOOP_TIMEOUT_SECONDS=INVALID"}
		actual := boundedCommand(root, cwd, checkout, false, args, extra...)
		Expect(actual.status).To(Equal(2))
		Expect(actual.stdout).To(BeEmpty())
		Expect(strings.ToLower(actual.stderr)).To(ContainSubstring("usage:"))
		Expect(strings.ToLower(actual.stderr)).To(ContainSubstring("error:"))
		_, err := os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		program := `import argparse,sys
parser=argparse.ArgumentParser()
commands=parser.add_subparsers(dest='command',required=True)
plan=commands.add_parser('plan')
plan.add_argument('--harness',required=True)
parser.parse_args(sys.argv[1:])`
		minimal := loopProcess(cwd, "python3", append([]string{"-B", "-c", program}, args...), nil)
		profile := []any{minimal.status, minimal.stdout != "", minimal.stderr != ""}
		Expect(profile).To(Or(Equal([]any{2, false, true}), Equal([]any{0, true, false})))
		oracle := boundedCommand(root, cwd, checkout, true, args, extra...)
		Expect(oracle.status).To(Equal(minimal.status))
		Expect(oracle.stdout != "").To(Equal(minimal.stdout != ""))
		Expect(oracle.stderr != "").To(Equal(minimal.stderr != ""))
		_, err = os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("ambiguity after help", []string{"plan", "-h", "--h"}), Entry("root terminator before command help", []string{"--", "plan", "-h"}))
})
