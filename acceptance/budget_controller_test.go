package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
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

// per docs/adr/0070-go-budget-execution-controller.md:12
const controllerDriverSource = `package main
import("context";"encoding/json";"fmt";"io";"os";"path/filepath";"strings";"time";"github.com/anoop2811/software-factory-template/internal/budget")
type command struct{Root string;Request budget.Request;Environment map[string]string;Input budget.RunInput;TimeoutMillis,CancelMillis int}
func main(){if filepath.Base(os.Args[0])!="controller-driver"{fake();return};var c command;if json.NewDecoder(os.Stdin).Decode(&c)!=nil{os.Exit(2)};ctx,interrupt:=context.WithCancel(context.Background());defer interrupt();if c.CancelMillis>0{timer:=time.AfterFunc(time.Duration(c.CancelMillis)*time.Millisecond,interrupt);defer timer.Stop()};cancel:=func(){};if c.TimeoutMillis!=0{ctx,cancel=context.WithTimeout(ctx,time.Duration(c.TimeoutMillis)*time.Millisecond)};defer cancel();cfg,err:=budget.Configuration(c.Environment);out:=map[string]any{};if err==nil{result,e:=budget.NewRunner(c.Root).Run(ctx,c.Request,cfg,c.Input);out["result"]=result;out["response"]=result.Response;err=e};if err!=nil{out["error"]=err.Error()};json.NewEncoder(os.Stdout).Encode(out)}
func fake(){cwd,_:=os.Getwd();mode:=os.Getenv("CONTROLLER_FIXTURE_MODE");help:=false;for _,arg:=range os.Args[1:]{if arg=="--help"{help=true}};capture:=map[string]any{"argv":os.Args[1:],"cwd":cwd,"role":os.Getenv("FACTORY_AGENT_ROLE"),"help":help};if help{log(capture);if mode=="bad-help"{fmt.Print("unrecognized fixture");return};fmt.Print("--json --sandbox --cd --model --config --print --output-format --agent --permission-mode --format");return}
 first:=make([]byte,1);n,_:=os.Stdin.Read(first);published:=false;raw,_:=os.ReadFile(filepath.Join(cwd,".factory","budget.json"));var history map[string]any;if json.Unmarshal(raw,&history)==nil{if rows,ok:=history["runs"].([]any);ok{for _,value:=range rows{row:=value.(map[string]any);if row["process_pid"]==float64(os.Getpid())&&row["status"]=="active"{published=true}}}};rest,_:=io.ReadAll(os.Stdin);capture["stdin"]=string(append(first[:n],rest...));capture["pid_published_before_input"]=published;capture["pid"]=os.Getpid();log(capture)
 if mode=="stall"{time.Sleep(30*time.Second);return};if mode=="overflow"{io.WriteString(os.Stdout,strings.Repeat("x",16*1024*1024+1));time.Sleep(30*time.Second);return};if mode=="incomplete"{fmt.Println("{}");return}
 emit:=func(value string){if mode=="invalid-answer"{value=strings.ReplaceAll(value,"PRIVATE_ANSWER","\\ud800")};fmt.Println(value)}
 switch filepath.Base(os.Args[0]){
 case "codex":emit("{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"PRIVATE_ANSWER\"}}");emit("{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":3,\"cached_input_tokens\":1,\"output_tokens\":2}}")
 case "claude":emit("{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"PRIVATE_ANSWER\",\"usage\":{\"input_tokens\":3,\"output_tokens\":2,\"cache_creation_input_tokens\":0,\"cache_read_input_tokens\":1},\"total_cost_usd\":0.1}")
 case "opencode":emit("{\"type\":\"text\",\"part\":{\"sessionID\":\"s\",\"messageID\":\"m\",\"id\":\"text\",\"text\":\"PRIVATE_ANSWER\"}}");emit("{\"type\":\"step_finish\",\"part\":{\"sessionID\":\"s\",\"messageID\":\"m\",\"id\":\"step\",\"reason\":\"stop\",\"cost\":0.1,\"tokens\":{\"input\":3,\"output\":2,\"reasoning\":0,\"cache\":{\"read\":1,\"write\":0}}}}")
 };if mode=="nonzero"{os.Exit(7)}}
func log(row map[string]any){path:=os.Getenv("CONTROLLER_FIXTURE_LOG");if path==""{return};file,err:=os.OpenFile(path,os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600);if err==nil{json.NewEncoder(file).Encode(row);file.Close()}}
`

var controllerDriver []byte

func controllerBuild() {
	GinkgoHelper()
	nativeBuild()
	if controllerDriver != nil {
		return
	}
	root, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	dir, err := os.MkdirTemp("", "factory-controller-driver-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, dir)
	module, err := os.ReadFile(filepath.Join(root, "go.mod"))
	Expect(err).NotTo(HaveOccurred())
	parts := strings.SplitN(string(module), "\ngo ", 2)
	Expect(parts).To(HaveLen(2), "controller fixture requires a go directive in repository go.mod")
	fields := strings.Fields(parts[1])
	Expect(fields).NotTo(BeEmpty(), "controller fixture requires a Go version after the go directive")
	version := fields[0]
	writeFixture(filepath.Join(dir, "go.mod"), []byte("module github.com/anoop2811/software-factory-template/acceptance/controllerfixture\n\ngo "+version+"\nrequire github.com/anoop2811/software-factory-template v0.0.0\nreplace github.com/anoop2811/software-factory-template => "+root+"\n"), 0600)
	writeFixture(filepath.Join(dir, "main.go"), []byte(controllerDriverSource), 0600)
	args := []string{"build", "-mod=mod", "-o", filepath.Join(dir, "controller-driver"), "."}
	if os.Getenv("FACTORY_CONTROLLER_TEST_RACE") == "1" {
		args = append([]string{"build", "-race"}, args[1:]...)
	}
	cmd := exec.Command("go", args...) // #nosec G204 -- evaluator-owned module and output.
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOPROXY=off")
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "controller API compile: %s", output)
	controllerDriver, err = os.ReadFile(filepath.Join(dir, "controller-driver"))
	Expect(err).NotTo(HaveOccurred())
}
func controllerFixture() (string, string) {
	GinkgoHelper()
	root, cwd := nativeFixture()
	writeFixture(filepath.Join(root, "controller-driver"), controllerDriver, 0755)
	for _, h := range []string{"codex", "claude", "opencode"} {
		writeFixture(filepath.Join(root, "bin", h), controllerDriver, 0755)
	}
	return root, cwd
}
func controllerCommand(root, h string) map[string]any {
	return map[string]any{"Root": root, "Request": map[string]any{"Session": "s", "Task": "t", "Harness": h, "Role": "reviewer", "Model": "fixture/model;$()"}, "Environment": map[string]string{"FACTORY_BUDGET_ENABLED": "true"}, "Input": map[string]any{"Prompt": "PRIVATE_PROMPT\n", "WantResponse": true}}
}
func controllerInvoke(root, cwd string, c map[string]any, extra ...string) map[string]any {
	GinkgoHelper()
	raw, err := json.Marshal(c)
	Expect(err).NotTo(HaveOccurred())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(root, "controller-driver")) // #nosec G204 G702 -- test-built executable; inputs travel only through stdin.
	cmd.Dir = cwd
	cmd.Env = append([]string{"PATH=" + filepath.Join(root, "bin"), "GORACE=atexit_sleep_ms=0", "CONTROLLER_FIXTURE_LOG=" + filepath.Join(cwd, "native-calls.jsonl")}, extra...)
	cmd.Stdin = bytes.NewReader(raw)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred())
	Expect(err).NotTo(HaveOccurred(), stderr.String())
	Expect(stderr.String()).To(BeEmpty())
	return budgetDecode(stdout.Bytes()).(map[string]any)
}

var _ = Describe("G2 composed budget controller", func() {
	BeforeEach(controllerBuild)
	// per docs/adr/0070-go-budget-execution-controller.md:41
	It("returns a disabled plan before touching prompt CLI or storage", func() {
		root, cwd := controllerFixture()
		c := controllerCommand(root, "claude")
		c["Environment"].(map[string]string)["FACTORY_BUDGET_ENABLED"] = "false"
		c["Input"] = map[string]any{"PromptFile": filepath.Join(cwd, "missing")}
		out := controllerInvoke(root, cwd, c, "PATH=/no-native-programs")
		Expect(out).NotTo(HaveKey("error"))
		result := out["result"].(map[string]any)
		Expect(result["ExitCode"]).To(Equal(json.Number("2")))
		Expect(result["Record"]).To(BeNil())
		_, err := os.Stat(filepath.Join(root, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		_, err = os.Stat(filepath.Join(cwd, "native-calls.jsonl"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0070-go-budget-execution-controller.md:57
	It("does not reserve an attempt after local preflight refusal", func() {
		root, cwd := controllerFixture()
		out := controllerInvoke(root, cwd, controllerCommand(root, "claude"), "CONTROLLER_FIXTURE_MODE=bad-help")
		Expect(out).To(HaveKey("error"))
		Expect(out["result"].(map[string]any)["ExitCode"]).To(Equal(json.Number("2")))
		_, err := os.Stat(filepath.Join(root, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		raw, err := os.ReadFile(filepath.Join(cwd, "native-calls.jsonl"))
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Split(strings.TrimSpace(string(raw)), "\n")).To(HaveLen(1))
	})
	// per docs/adr/0070-go-budget-execution-controller.md:68
	DescribeTable("reserves and publishes PID before sending task bytes", func(h string) {
		root, cwd := controllerFixture()
		out := controllerInvoke(root, cwd, controllerCommand(root, h))
		Expect(out).NotTo(HaveKey("error"))
		result := out["result"].(map[string]any)
		Expect(result["ExitCode"]).To(Equal(json.Number("0")))
		Expect(out["response"]).To(Equal("PRIVATE_ANSWER"))
		Expect(result).NotTo(HaveKey("Response"))
		record := result["Record"].(map[string]any)
		Expect(record["status"]).To(Equal("completed"))
		Expect(record["complete"]).To(BeTrue())
		raw, err := os.ReadFile(filepath.Join(cwd, "native-calls.jsonl"))
		Expect(err).NotTo(HaveOccurred())
		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		Expect(lines).To(HaveLen(2))
		call := budgetDecode([]byte(lines[1])).(map[string]any)
		Expect(call["pid_published_before_input"]).To(BeTrue())
		Expect(call["role"]).To(Equal("reviewer"))
		expectedArgs := []any{"claude", "-p", "--output-format", "json", "--agent", "reviewer", "--permission-mode", "plan", "--model", "fixture/model;$()"}
		expectedStdin := "PRIVATE_PROMPT\n"
		if h == "codex" {
			expectedArgs = []any{"codex", "exec", "--json", "--sandbox", "read-only", "--cd", root, "--model", "fixture/model;$()", "-"}
			expectedStdin = "Factory role: reviewer\n\nCanonical instructions\n\nTask:\n" + expectedStdin
		}
		if h == "opencode" {
			expectedArgs = []any{"opencode", "run", "--format", "json", "--agent", "reviewer", "--model", "fixture/model;$()"}
		}
		Expect(call["argv"]).To(Equal(expectedArgs[1:]))
		Expect(call["stdin"]).To(Equal(expectedStdin))
		history, err := os.ReadFile(filepath.Join(root, ".factory/budget.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(history)).NotTo(ContainSubstring("PRIVATE_"))
	}, Entry("Codex", "codex"), Entry("Claude", "claude"), Entry("OpenCode", "opencode"))
})

func controllerCalls(cwd string) []map[string]any {
	GinkgoHelper()
	raw, err := os.ReadFile(filepath.Join(cwd, "native-calls.jsonl"))
	Expect(err).NotTo(HaveOccurred())
	rows := []map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		rows = append(rows, budgetDecode([]byte(line)).(map[string]any))
	}
	return rows
}

var _ = Describe("G2 composed budget controller boundaries", func() {
	BeforeEach(controllerBuild)
	// per docs/adr/0070-go-budget-execution-controller.md:93
	DescribeTable("keeps failed native outcomes and suppresses responses", func(mode, outcome string, code int) {
		root, cwd := controllerFixture()
		c := controllerCommand(root, "claude")
		if mode == "stall" {
			c["Environment"].(map[string]string)["FACTORY_BUDGET_TIMEOUT_SECONDS"] = "0.15"
		}
		out := controllerInvoke(root, cwd, c, "CONTROLLER_FIXTURE_MODE="+mode)
		Expect(out).NotTo(HaveKey("error"))
		result := out["result"].(map[string]any)
		Expect(result["ExitCode"]).To(Equal(json.Number(strconv.Itoa(code))))
		Expect(out["response"]).To(Equal(""))
		record := result["Record"].(map[string]any)
		Expect(record["outcome"]).To(Equal(outcome))
		Expect(record["status"]).To(Equal("completed"))
		// per docs/adr/0070-go-budget-execution-controller.md:207
		if mode == "overflow" {
			Expect(record["exit_code"]).To(Equal(json.Number("-9")), "overflow fixture must be terminated by the supervisor")
		}
		if mode != "nonzero" {
			Expect(record["tokens"]).To(BeNil())
			Expect(record["estimated_usd"]).To(BeNil())
			Expect(record["complete"]).To(BeFalse())
		}
	}, Entry("incomplete metadata", "incomplete", "failed", 1), Entry("nonzero exit", "nonzero", "failed", 1), Entry("timeout", "stall", "timeout", 124), Entry("stdout overflow", "overflow", "output_limit", 1))
	// per docs/adr/0070-go-budget-execution-controller.md:104
	DescribeTable("only selects an answer when explicitly requested", func(want bool) {
		root, cwd := controllerFixture()
		c := controllerCommand(root, "claude")
		c["Input"].(map[string]any)["WantResponse"] = want
		out := controllerInvoke(root, cwd, c, "CONTROLLER_FIXTURE_MODE=invalid-answer")
		result := out["result"].(map[string]any)
		Expect(out["response"]).To(Equal(""))
		if want {
			Expect(out).To(HaveKey("error"))
			Expect(result["ExitCode"]).To(Equal(json.Number("1")))
			Expect(result["Record"].(map[string]any)["outcome"]).To(Equal("launch_error"))
		} else {
			Expect(out).NotTo(HaveKey("error"))
			Expect(result["ExitCode"]).To(Equal(json.Number("0")))
		}
	}, Entry("requested invalid scalar", true), Entry("unrequested invalid scalar", false))
	// per docs/adr/0070-go-budget-execution-controller.md:76
	It("finalizes an interrupted native child after parent cancellation", func() {
		root, cwd := controllerFixture()
		c := controllerCommand(root, "claude")
		c["CancelMillis"] = 1000
		out := controllerInvoke(root, cwd, c, "CONTROLLER_FIXTURE_MODE=stall")
		Expect(out).NotTo(HaveKey("error"))
		result := out["result"].(map[string]any)
		Expect(result["ExitCode"]).To(Equal(json.Number("130")))
		record := result["Record"].(map[string]any)
		Expect(record["status"]).To(Equal("completed"))
		Expect(record["outcome"]).To(Equal("interrupted"))
		Expect(record["complete"]).To(BeFalse())
		Expect(out["response"]).To(Equal(""))
	})
	// per docs/adr/0070-go-budget-execution-controller.md:43
	DescribeTable("refuses invalid execution durations before probing or reserving", func(seconds string) {
		root, cwd := controllerFixture()
		c := controllerCommand(root, "claude")
		c["Environment"].(map[string]string)["FACTORY_BUDGET_TIMEOUT_SECONDS"] = seconds
		out := controllerInvoke(root, cwd, c)
		Expect(out).To(HaveKey("error"))
		Expect(out["result"].(map[string]any)["ExitCode"]).To(Equal(json.Number("2")))
		_, err := os.Stat(filepath.Join(root, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		_, err = os.Stat(filepath.Join(cwd, "native-calls.jsonl"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("duration overflow", "1e20"), Entry("duration rounds to zero", "1e-12"))
	// per docs/adr/0070-go-budget-execution-controller.md:49
	DescribeTable("distinguishes literal inline prompts from universal-newline files", func(inline bool) {
		root, cwd := controllerFixture()
		c := controllerCommand(root, "claude")
		text := "one\r\ntwo\rthree\n"
		if inline {
			c["Input"] = map[string]any{"Prompt": text, "PromptFile": "missing", "WantResponse": true}
		} else {
			writeFixture(filepath.Join(cwd, "prompt.txt"), []byte(text), 0600)
			c["Input"] = map[string]any{"PromptFile": "prompt.txt", "WantResponse": true}
		}
		out := controllerInvoke(root, cwd, c)
		Expect(out).NotTo(HaveKey("error"))
		calls := controllerCalls(cwd)
		want := text
		if !inline {
			want = "one\ntwo\nthree\n"
		}
		Expect(calls[1]["stdin"]).To(Equal(want))
	}, Entry("literal inline", true), Entry("caller cwd file", false))
	// per docs/adr/0070-go-budget-execution-controller.md:36
	It("lets a nonnil empty prompt override a missing prompt file", func() {
		root, cwd := controllerFixture()
		c := controllerCommand(root, "claude")
		c["Input"] = map[string]any{"Prompt": "", "PromptFile": "missing"}
		out := controllerInvoke(root, cwd, c)
		Expect(out).NotTo(HaveKey("error"))
		Expect(controllerCalls(cwd)[1]["stdin"]).To(Equal(""))
	})
	// per docs/adr/0070-go-budget-execution-controller.md:49
	DescribeTable("rejects unsafe prompt inputs before any local CLI", func(kind string) {
		root, cwd := controllerFixture()
		c := controllerCommand(root, "claude")
		path := filepath.Join(cwd, "prompt")
		switch kind {
		case "FIFO":
			Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
			c["Input"] = map[string]any{"PromptFile": path}
		case "invalid UTF8":
			writeFixture(path, []byte{0xff}, 0600)
			c["Input"] = map[string]any{"PromptFile": path}
		case "NUL":
			c["Input"] = map[string]any{"Prompt": "PRIVATE\x00TEXT"}
		case "oversized":
			c["Input"] = map[string]any{"Prompt": strings.Repeat("x", 16*1024*1024+1)}
		}
		out := controllerInvoke(root, cwd, c)
		Expect(out).To(HaveKey("error"))
		Expect(out["error"]).NotTo(ContainSubstring("PRIVATE"))
		Expect(out["error"]).NotTo(ContainSubstring(cwd))
		_, err := os.Stat(filepath.Join(root, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		_, err = os.Stat(filepath.Join(cwd, "native-calls.jsonl"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("special FIFO", "FIFO"), Entry("invalid UTF8", "invalid UTF8"), Entry("embedded NUL", "NUL"), Entry("oversized inline", "oversized"))
})

var _ = Describe("G2 composed budget controller lock boundaries", func() {
	BeforeEach(controllerBuild)
	// per docs/adr/0070-go-budget-execution-controller.md:61
	It("expires waiting for a Python-held admission lock after preflight", func() {
		root, cwd := controllerFixture()
		release, _ := budgetPythonHold(root)
		c := controllerCommand(root, "claude")
		c["TimeoutMillis"] = 2000
		out := controllerInvoke(root, cwd, c)
		release()
		Expect(out).To(HaveKey("error"))
		Expect(out["result"].(map[string]any)["Record"]).To(BeNil())
		calls := controllerCalls(cwd)
		Expect(calls).To(HaveLen(1))
		Expect(calls[0]["help"]).To(BeTrue())
		_, err := os.Stat(filepath.Join(root, ".factory/budget.json"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0070-go-budget-execution-controller.md:66
	It("admits only one competing controller for the final session slot", func() {
		root, cwd := controllerFixture()
		c := controllerCommand(root, "claude")
		c["Environment"].(map[string]string)["FACTORY_BUDGET_MAX_SESSION_RUNS"] = "1"
		raw, err := json.Marshal(c)
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		commands := make([]*exec.Cmd, 2)
		outputs := make([]bytes.Buffer, 2)
		for i := range commands {
			cmd := exec.CommandContext(ctx, filepath.Join(root, "controller-driver")) // #nosec G204 G702 -- evaluator-built fixture executable, no shell; request data is passed only through stdin.
			cmd.Dir = cwd
			cmd.Env = []string{"PATH=" + filepath.Join(root, "bin"), "GORACE=atexit_sleep_ms=0", "CONTROLLER_FIXTURE_LOG=" + filepath.Join(cwd, "native-calls.jsonl")}
			cmd.Stdin = bytes.NewReader(raw)
			cmd.Stdout = &outputs[i]
			commands[i] = cmd
			Expect(cmd.Start()).To(Succeed())
			DeferCleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		}
		codes := []string{}
		for _, cmd := range commands {
			Expect(cmd.Wait()).To(Succeed())
		}
		combined := []string{outputs[0].String(), outputs[1].String()}
		for i := range commands {
			out := budgetDecode(outputs[i].Bytes()).(map[string]any)
			Expect(out).NotTo(HaveKey("error"), "both controller results: %v", combined)
			codes = append(codes, string(out["result"].(map[string]any)["ExitCode"].(json.Number)))
		}
		Expect(codes).To(ConsistOf("0", "2"))
		runs := 0
		for _, call := range controllerCalls(cwd) {
			if call["help"] == false {
				runs++
			}
		}
		Expect(runs).To(Equal(1))
	})
})
