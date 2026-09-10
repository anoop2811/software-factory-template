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

// This driver is a temporary evaluator, never a factory command or release asset.
// per docs/adr/0068-go-native-harness-execution.md:26
const nativeDriverSource = `package main
import("context";"encoding/json";"io";"os";"os/signal";"path/filepath";"strings";"syscall";"time";"errors";"os/exec";"strconv";"bufio";"bytes";"github.com/anoop2811/software-factory-template/internal/native";"github.com/anoop2811/software-factory-template/internal/usage")
type request struct{Action string;Request native.Request;Environment map[string]string;AllowanceMillis int;Cancel bool;CallbackError bool;CallbackDelayMillis int;SignalDelayMillis int;NilCallback bool;Input string}
func main(){
 if filepath.Base(os.Args[0])!="driver" {fake();return}
 var r request;if json.NewDecoder(os.Stdin).Decode(&r)!=nil{os.Exit(2)}
 ctx,stop:=signal.NotifyContext(context.Background(),syscall.SIGTERM,syscall.SIGINT,syscall.SIGHUP);defer stop()
 if r.SignalDelayMillis>0{go func(){time.Sleep(time.Duration(r.SignalDelayMillis)*time.Millisecond);syscall.Kill(os.Getpid(),syscall.SIGTERM)}()}
 if r.Cancel{var cancel context.CancelFunc;ctx,cancel=context.WithCancel(ctx);cancel()}
 out:=map[string]any{}
 if r.Action=="response"{answer,err:=usage.Response(ctx,r.Request.Harness,strings.NewReader(r.Input));out["answer"]=answer;if err!=nil{out["error"]=err.Error()};json.NewEncoder(os.Stdout).Encode(out);return}
 plan,err:=native.Prepare(r.Request,r.Environment);out["plan"]=plan
 if err==nil&&r.Action=="preflight"{err=native.Preflight(ctx,plan)}
 if err==nil&&r.Action=="execute"{
  callback:=func(_ context.Context,pid int)error{out["callback_pid"]=pid;if r.CallbackDelayMillis>0{time.Sleep(time.Duration(r.CallbackDelayMillis)*time.Millisecond)};if r.CallbackError{return errors.New("PRIVATE_CALLBACK_ERROR")};return nil}
  if r.NilCallback{callback=nil}
  result,runErr:=native.Execute(ctx,plan,time.Duration(r.AllowanceMillis)*time.Millisecond,callback);out["execution"]=result;err=runErr
 }
 if err!=nil{out["error"]=err.Error()};json.NewEncoder(os.Stdout).Encode(out)
}
func fake(){
 mode:=os.Getenv("NATIVE_MODE");executable,_:=os.Executable()
 if log:=os.Getenv("NATIVE_LOG");log!=""{f,_:=os.OpenFile(log,os.O_CREATE|os.O_WRONLY|os.O_APPEND,0600);if f!=nil{json.NewEncoder(f).Encode(map[string]any{"executable":executable,"pid":os.Getpid(),"argv":os.Args[1:],"role":os.Getenv("FACTORY_AGENT_ROLE"),"overlay":os.Getenv("OPENCODE_CONFIG_CONTENT")});f.Close()}}

 if len(os.Args)>1&&os.Args[1]=="app-server"{
  scanner:=bufio.NewScanner(os.Stdin);rows:=[]map[string]any{};for len(rows)<5&&scanner.Scan(){var row map[string]any;json.Unmarshal(scanner.Bytes(),&row);rows=append(rows,row)}
  if path:=os.Getenv("NATIVE_TRANSCRIPT");path!=""{raw,_:=json.Marshal(rows);os.WriteFile(path,raw,0600)}
  cwd,_:=os.Getwd();cwd,_=filepath.EvalSymlinks(cwd)
  hook:=map[string]any{"eventName":"preToolUse","handlerType":"command","command":"FACTORY_AGENT_ROLE=implementer \"$(git rev-parse --show-toplevel)/scripts/hooks/test-edit-denial.sh\"","matcher":"^(apply_patch|Edit|Write)$","enabled":true,"trustStatus":"trusted","async":false}
  trust:=os.Getenv("NATIVE_TRUST");if trust=="managed"{hook["trustStatus"]="managed"};if trust=="untrusted"{hook["trustStatus"]="untrusted"};if trust=="async"{hook["async"]=true};if trust=="wrong-command"{hook["command"]="PRIVATE_BAD_COMMAND"}
  features:=map[string]any{};if trust=="disabled"{features["hooks"]=false}
  var transcript bytes.Buffer
  json.NewEncoder(&transcript).Encode(map[string]any{"id":4,"result":map[string]any{"requirements":nil}})
  json.NewEncoder(&transcript).Encode(map[string]any{"id":3,"result":map[string]any{"data":[]any{map[string]any{"cwd":cwd,"errors":[]any{},"hooks":[]any{hook}}}}})
  json.NewEncoder(&transcript).Encode(map[string]any{"id":2,"result":map[string]any{"config":map[string]any{"features":features}}})
  if trust=="trailing-malformed"{transcript.WriteString("PRIVATE_MALFORMED_FRAME\n")}
  os.Stdout.Write(transcript.Bytes())
  if trust=="trailing-malformed"{return}
  time.Sleep(30*time.Second);return
 }
 if mode=="nonzero"{os.Exit(7)}
 if mode=="signal"{syscall.Kill(os.Getpid(),syscall.SIGKILL);return}
 if mode=="closed-pipes"{os.Stdout.Close();os.Stderr.Close();time.Sleep(30*time.Second);return}
 if mode=="stall"{time.Sleep(30*time.Second);return}
 if mode=="full-pipes"{go func(){io.WriteString(os.Stderr,strings.Repeat("PRIVATE_STDERR",1<<18))}();io.WriteString(os.Stdout,strings.Repeat("x",1<<20))}
 if mode=="stdout-limit"{count,_:=strconv.Atoi(os.Getenv("NATIVE_BYTES"));io.WriteString(os.Stdout,strings.Repeat("x",count));return}
 if mode=="descendant"{child:=exec.Command(os.Args[0]);child.Env=append(os.Environ(),"NATIVE_MODE=stall");child.Stdout=os.Stdout;child.Stderr=os.Stderr;child.Start();json.NewEncoder(os.Stdout).Encode(map[string]any{"child_pid":child.Process.Pid});return}
 if mode=="help-size"{flags:="--json --sandbox --cd --model --config --print --output-format --agent --permission-mode --format";size,_:=strconv.Atoi(os.Getenv("NATIVE_HELP_BYTES"));io.WriteString(os.Stdout,flags+strings.Repeat(" ",size-len(flags)));return}
 if mode=="help-missing"{io.WriteString(os.Stdout,"--json");return}
 if len(os.Args)>1&&(os.Args[1]=="--help"||(len(os.Args)>2&&os.Args[2]=="--help")){helpInput,_:=io.ReadAll(os.Stdin);if len(helpInput)!=0{os.Exit(9)};if help:=os.Getenv("NATIVE_HELP_TEXT");help!=""{io.WriteString(os.Stdout,help);return};io.WriteString(os.Stdout,"--json --sandbox --cd --model --config --print --output-format --agent --permission-mode --format");return}
 input:=[]byte{};if marker:=os.Getenv("NATIVE_STDIN_MARKER");marker!=""{buffer:=make([]byte,4096);for{n,err:=os.Stdin.Read(buffer);if n>0{f,_:=os.OpenFile(marker,os.O_CREATE|os.O_WRONLY|os.O_APPEND,0600);if f!=nil{f.Write(buffer[:n]);f.Close()};input=append(input,buffer[:n]...)};if err!=nil{break}}}else{input,_=io.ReadAll(os.Stdin)};cwd,_:=os.Getwd()
 json.NewEncoder(os.Stdout).Encode(map[string]any{"executable":executable,"argv":os.Args[1:],"stdin":string(input),"cwd":cwd,"role":os.Getenv("FACTORY_AGENT_ROLE"),"inherited":os.Getenv("NATIVE_INHERITED"),"overlay":os.Getenv("OPENCODE_CONFIG_CONTENT")})
}
`

var nativeDriver []byte

func nativeBuild() {
	GinkgoHelper()
	if nativeDriver != nil {
		return
	}
	root, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	dir, err := os.MkdirTemp("", "factory-native-driver-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, dir)
	moduleBytes, err := os.ReadFile(filepath.Join(root, "go.mod"))
	Expect(err).NotTo(HaveOccurred())
	goVersion := strings.Fields(strings.SplitN(string(moduleBytes), "\ngo ", 2)[1])[0]
	writeFixture(filepath.Join(dir, "go.mod"), []byte("module github.com/anoop2811/software-factory-template/acceptance/nativefixture\n\ngo "+goVersion+"\n\nrequire github.com/anoop2811/software-factory-template v0.0.0\nreplace github.com/anoop2811/software-factory-template => "+root+"\n"), 0600)
	writeFixture(filepath.Join(dir, "main.go"), []byte(nativeDriverSource), 0600)
	buildArgs := []string{"build", "-mod=mod", "-o", filepath.Join(dir, "driver"), "."}
	if os.Getenv("FACTORY_NATIVE_TEST_RACE") == "1" {
		buildArgs = append([]string{"build", "-race"}, buildArgs[1:]...)
	}
	cmd := exec.Command("go", buildArgs...) // #nosec G204 -- test-owned driver source/module and output path.
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOPROXY=off")
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "native component interface compilation: %s", output)
	nativeDriver, err = os.ReadFile(filepath.Join(dir, "driver"))
	Expect(err).NotTo(HaveOccurred())
}
func nativeFixture() (string, string) {
	GinkgoHelper()
	root, cwd := fixture()
	writeFixture(filepath.Join(root, "driver"), nativeDriver, 0755)
	for _, h := range []string{"codex", "claude", "opencode"} {
		writeFixture(filepath.Join(root, "bin", h), nativeDriver, 0755)
	}
	for _, role := range []string{"reviewer", "implementer", "spec-writer", "refactorer", "wiki-maintainer"} {
		writeFixture(filepath.Join(root, ".opencode/agent", role+".md"), []byte("---\nname: role\n---\nCanonical instructions\n"), 0600)
		writeFixture(filepath.Join(root, ".claude/agents", role+".md"), []byte("Native Claude instructions\n"), 0600)
	}
	writeFixture(filepath.Join(root, "opencode.json"), []byte(`{"agent":{"reviewer":{"permission":{"edit":"deny"}},"implementer":{"permission":{"edit":"allow"}},"spec-writer":{"permission":{}},"refactorer":{"permission":{}},"wiki-maintainer":{"permission":{}}}}`), 0600)
	writeFixture(filepath.Join(root, "scripts/hooks/test-edit-denial.sh"), []byte("#!/bin/sh\nexit 0\n"), 0755)
	return root, cwd
}
func nativeInvoke(root, cwd string, request map[string]any, extra ...string) map[string]any {
	GinkgoHelper()
	input, err := json.Marshal(request)
	Expect(err).NotTo(HaveOccurred())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(root, "driver")) // #nosec G204 G702 -- evaluator-owned executable, no shell interpretation.
	cmd.Dir = cwd
	cmd.Env = append([]string{"PATH=" + filepath.Join(root, "bin"), "GORACE=atexit_sleep_ms=0", "NATIVE_INHERITED=literal $(never execute)"}, extra...)
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred())
	Expect(err).NotTo(HaveOccurred(), stderr.String())
	var result map[string]any
	Expect(json.Unmarshal(stdout.Bytes(), &result)).To(Succeed(), stdout.String())
	return result
}
func nativeRequest(root, harness, action string) map[string]any {
	return map[string]any{"Action": action, "Request": map[string]any{"Harness": harness, "Role": "reviewer", "Model": "literal/model;$()", "Root": root, "Prompt": "PRIVATE_TASK\n"}, "AllowanceMillis": 2000}
}

var _ = Describe("G2 native harness component", func() {
	BeforeEach(nativeBuild)
	// per docs/adr/0068-go-native-harness-execution.md:60
	DescribeTable("prepares literal argv and stdin without launching a native process", func(harness string) {
		root, cwd := nativeFixture()
		result := nativeInvoke(root, cwd, nativeRequest(root, harness, "prepare"))
		Expect(result).NotTo(HaveKey("error"))
		plan := result["plan"].(map[string]any)
		var argv []any
		stdin := "PRIVATE_TASK\n"
		switch harness {
		case "codex":
			argv = []any{"codex", "exec", "--json", "--sandbox", "read-only", "--cd", root, "--model", "literal/model;$()", "-"}
			stdin = "Factory role: reviewer\n\nCanonical instructions\n\nTask:\n" + stdin
		case "claude":
			argv = []any{"claude", "-p", "--output-format", "json", "--agent", "reviewer", "--permission-mode", "plan", "--model", "literal/model;$()"}
		case "opencode":
			argv = []any{"opencode", "run", "--format", "json", "--agent", "reviewer", "--model", "literal/model;$()"}
		}
		Expect(plan["Argv"]).To(Equal(argv))
		Expect(plan["Stdin"]).To(Equal(stdin))
		baseline := nativeOracle(root, cwd, nativeRequest(root, harness, "prepare"))
		Expect(baseline["argv"]).To(Equal(argv))
		Expect(baseline["stdin"]).To(Equal(stdin))
		expectedEnv := baseline["env"].(map[string]any)
		actualEnv := plan["Environment"].(map[string]any)
		Expect(actualEnv["FACTORY_AGENT_ROLE"]).To(Equal(expectedEnv["FACTORY_AGENT_ROLE"]))
	}, Entry("codex", "codex"), Entry("claude", "claude"), Entry("opencode", "opencode"))
	// per docs/adr/0068-go-native-harness-execution.md:110
	DescribeTable("executes only the fake native binary with stdin cwd and role preserved", func(harness string) {
		root, cwd := nativeFixture()
		result := nativeInvoke(root, cwd, nativeRequest(root, harness, "execute"))
		Expect(result).NotTo(HaveKey("error"))
		run := result["execution"].(map[string]any)
		Expect(run["Outcome"]).To(Equal("completed"))
		Expect(run["ExitCode"]).To(Equal(float64(0)))
		Expect(run["ExitConfirmed"]).To(BeTrue())
		Expect(run["OwnershipUnconfirmed"]).To(BeFalse())
		Expect(result["callback_pid"]).To(Equal(run["ProcessPID"]))
		encoded, _ := json.Marshal(run["Stdout"])
		var raw []byte
		Expect(json.Unmarshal(encoded, &raw)).To(Succeed())
		var child map[string]any
		Expect(json.Unmarshal(raw, &child)).To(Succeed())
		Expect(child["cwd"]).To(Equal(root))
		Expect(child["role"]).To(Equal("reviewer"))
		Expect(child["inherited"]).To(Equal("literal $(never execute)"))
		Expect(child["stdin"]).To(Equal(result["plan"].(map[string]any)["Stdin"]))
	}, Entry("codex", "codex"), Entry("claude", "claude"), Entry("opencode", "opencode"))
})

func nativeExecution(result map[string]any) map[string]any {
	GinkgoHelper()
	Expect(result).To(HaveKey("execution"))
	return result["execution"].(map[string]any)
}
func nativeRaw(run map[string]any) []byte {
	GinkgoHelper()
	encoded, err := json.Marshal(run["Stdout"])
	Expect(err).NotTo(HaveOccurred())
	var raw []byte
	Expect(json.Unmarshal(encoded, &raw)).To(Succeed())
	return raw
}
func expectNativeReaped(run map[string]any) {
	GinkgoHelper()
	pid := int(run["ProcessPID"].(float64))
	Expect(pid).To(BeNumerically(">", 0))
	Expect(syscall.Kill(pid, 0)).To(Equal(syscall.ESRCH), "reaped leader must no longer exist")
}

var _ = Describe("G2 native harness boundaries", func() {
	BeforeEach(nativeBuild)
	// per docs/adr/0068-go-native-harness-execution.md:92
	DescribeTable("checks native help without running an agent", func(h string) {
		root, cwd := nativeFixture()
		log := filepath.Join(cwd, "calls.jsonl")
		result := nativeInvoke(root, cwd, nativeRequest(root, h, "preflight"), "NATIVE_LOG="+log)
		Expect(result).NotTo(HaveKey("error"))
		raw, err := os.ReadFile(log)
		Expect(err).NotTo(HaveOccurred())
		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		Expect(lines).To(HaveLen(1))
		var call map[string]any
		Expect(json.Unmarshal([]byte(lines[0]), &call)).To(Succeed())
		args := []any{"--help"}
		if h == "codex" {
			args = []any{"exec", "--help"}
		}
		if h == "opencode" {
			args = []any{"run", "--help"}
		}
		Expect(call["argv"]).To(Equal(args))
		Expect(call["role"]).To(Equal(""))
		Expect(call["overlay"]).To(Equal(""))
	}, Entry("codex", "codex"), Entry("claude", "claude"), Entry("opencode", "opencode"))
	// per docs/adr/0068-go-native-harness-execution.md:92
	DescribeTable("refuses failed help", func(mode string) {
		root, cwd := nativeFixture()
		result := nativeInvoke(root, cwd, nativeRequest(root, "claude", "preflight"), "NATIVE_MODE="+mode)
		Expect(result).To(HaveKey("error"))
	}, Entry("missing flags", "help-missing"), Entry("nonzero", "nonzero"))
	// per docs/adr/0068-go-native-harness-execution.md:110
	DescribeTable("supervises real native process exits", func(mode, outcome string, code float64) {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "opencode", "execute")
		request["AllowanceMillis"] = 150
		if outcome == "failed" {
			request["AllowanceMillis"] = 3000
		}
		result := nativeInvoke(root, cwd, request, "NATIVE_MODE="+mode)
		run := nativeExecution(result)
		Expect(run["Outcome"]).To(Equal(outcome))
		Expect(run["ExitCode"]).To(Equal(code))
		Expect(run["ExitConfirmed"]).To(BeTrue())
		expectNativeReaped(run)
		Expect(result).NotTo(HaveKey("error"))
	}, Entry("failed", "nonzero", "failed", float64(7)), Entry("signal", "signal", "failed", float64(-9)), Entry("closed pipes remain live", "closed-pipes", "timeout", float64(-9)), Entry("stalled child", "stall", "timeout", float64(-9)))
	// per docs/adr/0068-go-native-harness-execution.md:117
	DescribeTable("bounds stdout exactly", func(count int, want string) {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "opencode", "execute")
		request["AllowanceMillis"] = 4000
		result := nativeInvoke(root, cwd, request, "NATIVE_MODE=stdout-limit", "NATIVE_BYTES="+strconv.Itoa(count))
		run := nativeExecution(result)
		Expect(run["Outcome"]).To(Equal(want))
		Expect(nativeRaw(run)).To(HaveLen(count))
		expectNativeReaped(run)
	}, Entry("exact limit", 16*1024*1024, "completed"), Entry("one excess byte", 16*1024*1024+1, "output_limit"))
	// per docs/adr/0068-go-native-harness-execution.md:119
	It("reaps the native child when ownership publication fails", func() {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "opencode", "execute")
		request["CallbackError"] = true
		marker := filepath.Join(cwd, "prompt-received")
		result := nativeInvoke(root, cwd, request, "NATIVE_STDIN_MARKER="+marker)
		run := nativeExecution(result)
		Expect(result).To(HaveKey("error"))
		Expect(result["error"]).NotTo(ContainSubstring("PRIVATE"))
		Expect(run["Outcome"]).To(Equal("launch_error"))
		Expect(run["ExitConfirmed"]).To(BeTrue())
		Expect(nativeRaw(run)).To(BeEmpty())
		expectNativeReaped(run)
		received, readErr := os.ReadFile(marker)
		Expect(os.IsNotExist(readErr) || readErr == nil).To(BeTrue())
		Expect(received).To(BeEmpty(), "child must not receive prompt bytes before successful live ownership callback")
	})
	// per docs/adr/0068-go-native-harness-execution.md:114
	DescribeTable("refuses launch before any child exists", func(cancel bool, allowance int) {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "opencode", "execute")
		request["Cancel"] = cancel
		request["AllowanceMillis"] = allowance
		log := filepath.Join(cwd, "calls")
		result := nativeInvoke(root, cwd, request, "NATIVE_LOG="+log)
		Expect(result).To(HaveKey("error"))
		Expect(result).NotTo(HaveKey("callback_pid"))
		_, err := os.Stat(log)
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("canceled context", true, 1000), Entry("zero allowance", false, 0), Entry("negative allowance", false, -1))
	// per docs/adr/0068-go-native-harness-execution.md:71
	It("preserves unrelated OpenCode overlay data while overriding selected mode", func() {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "opencode", "prepare")
		request["Environment"] = map[string]string{"OPENCODE_CONFIG_CONTENT": `{"unknown":true,"agent":{"reviewer":{"mode":"subagent","disable":1,"extra":"retained"},"other":{"mode":"primary"}}}`}
		result := nativeInvoke(root, cwd, request)
		Expect(result).NotTo(HaveKey("error"))
		plan := result["plan"].(map[string]any)
		env := plan["Environment"].(map[string]any)
		Expect(env).To(HaveLen(2))
		Expect(env["FACTORY_AGENT_ROLE"]).To(Equal("reviewer"))
		Expect(env["OPENCODE_CONFIG_CONTENT"]).To(MatchJSON(`{"unknown":true,"agent":{"reviewer":{"mode":"all","disable":1,"extra":"retained"},"other":{"mode":"primary"}}}`))
	})
	// per docs/adr/0068-go-native-harness-execution.md:71
	DescribeTable("rejects invalid OpenCode overlays without subprocesses", func(overlay string) {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "opencode", "prepare")
		request["Environment"] = map[string]string{"OPENCODE_CONFIG_CONTENT": overlay}
		result := nativeInvoke(root, cwd, request)
		Expect(result).To(HaveKey("error"))
	}, Entry("array", "[]"), Entry("agent array", `{"agent":[]}`), Entry("role array", `{"agent":{"reviewer":[]}}`), Entry("disabled", `{"agent":{"reviewer":{"disable":true}}}`), Entry("nonfinite", `{"extra":NaN}`))
})

const nativeBaseline = "4cc771e894e11d5024032106aeb0ef788997bd29"

func nativeOracle(root, cwd string, request map[string]any) map[string]any {
	GinkgoHelper()
	source := exec.Command("git", "show", nativeBaseline+":scripts/lib/budget_adapters.py") // #nosec G204 -- immutable baseline source, fixed argv.
	original, err := source.Output()
	Expect(err).NotTo(HaveOccurred())
	path := filepath.Join(root, "immutable-native.py")
	writeFixture(path, original, 0600)
	python, err := exec.LookPath("python3")
	Expect(err).NotTo(HaveOccurred())
	raw, err := json.Marshal(request)
	Expect(err).NotTo(HaveOccurred())
	result := usageProcess(cwd, string(raw), "/usr/bin/env", "PATH="+os.Getenv("PATH"), "HOME="+os.Getenv("HOME"), python, "-I", "-B", "-c", `import json,runpy,sys
sys.set_int_max_str_digits(4300)
m=runpy.run_path(sys.argv[1]);q=json.load(sys.stdin);r=q["Request"]
if q["Action"]=="preflight-oracle":
 import os
 os.environ["PATH"]=q["Path"]
 os.environ["NATIVE_LOG"]=q.get("Log","")
 try: m["preflight"](r["Harness"],r["Role"],r["Root"]);print("{}")
 except ValueError as e: print(json.dumps({"error":str(e)}))
elif q["Action"]=="help-flags":
 import re
 print(json.dumps({"missing":[flag for flag in m["HELP"][r["Harness"]][1] if not re.search(re.escape(flag)+r"\b",q["Input"])]}))
elif q["Action"]=="prepare":
 a,p=m["build_command"](r["Harness"],r["Role"],r["Model"],r["Root"],r["Prompt"]); print(json.dumps({"argv":a,"stdin":p,"env":m["environment"](r["Harness"],r["Role"],q.get("Environment",{}))}))
else: print(json.dumps({"answer":m["response_text"](r["Harness"],[json.loads(line) for line in q["Input"].splitlines() if line.strip()])}))`, path)
	Expect(result.status).To(Equal(0), result.stderr)
	var out map[string]any
	Expect(json.Unmarshal([]byte(result.stdout), &out)).To(Succeed())
	return out
}

var _ = Describe("G2 native harness response selection", func() {
	BeforeEach(nativeBuild)
	// per docs/adr/0068-go-native-harness-execution.md:142
	DescribeTable("matches immutable selection and explicit answer text", func(h, input, want string) {
		root, cwd := nativeFixture()
		request := nativeRequest(root, h, "response")
		request["Input"] = input
		baseline := nativeOracle(root, cwd, request)
		Expect(baseline["answer"]).To(Equal(want))
		result := nativeInvoke(root, cwd, request)
		Expect(result).NotTo(HaveKey("error"))
		Expect(result["answer"]).To(Equal(want))
	},
		Entry("Codex last message", "codex", `{"type":"item.completed","item":{"type":"agent_message","text":"first"}}`+"\n"+`{"type":"item.completed","item":{"type":"agent_message","text":"last"}}`, "last"),
		Entry("Codex empty suppresses", "codex", `{"type":"item.completed","item":{"type":"agent_message","text":"first"}}`+"\n"+`{"type":"item.completed","item":{"type":"agent_message","text":""}}`, ""),
		Entry("Codex tool ignored", "codex", `{"type":"item.completed","item":{"type":"tool","text":"PRIVATE_TOOL"}}`, ""),
		Entry("Claude last success", "claude", `{"type":"result","subtype":"success","result":"first"}`+"\n"+`{"type":"result","subtype":"error","result":"PRIVATE_ERROR"}`, "first"),
		Entry("Claude numeric true stays admitted", "claude", `{"type":"result","subtype":"success","is_error":1,"result":"answer"}`, "answer"),
		Entry("Claude boolean true rejected", "claude", `{"type":"result","subtype":"success","is_error":true,"result":"PRIVATE_ERROR"}`, ""),
		Entry("Claude empty suppresses", "claude", `{"type":"result","subtype":"success","result":"first"}`+"\n"+`{"type":"result","subtype":"success","result":""}`, ""),
		Entry("OpenCode empty identities first wins", "opencode", `{"type":"text","part":{"sessionID":"","messageID":"","id":"","text":"first"}}`+"\n"+`{"type":"text","part":{"sessionID":"","messageID":"","id":"","text":"conflicting"}}`+"\n"+`{"type":"text","part":{"sessionID":"s","messageID":"m","id":"i","text":"second"}}`, "first\nsecond"),
		Entry("OpenCode invalid identities ignored", "opencode", `{"type":"text","part":{"sessionID":1,"messageID":"m","id":"i","text":"PRIVATE_INVALID"}}`, ""),
		Entry("OpenCode surrogate identities preserved", "opencode", `{"type":"text","part":{"sessionID":"\ud800","messageID":"m","id":"i","text":"first"}}`+"\n"+`{"type":"text","part":{"sessionID":"\ufffd","messageID":"m","id":"i","text":"second"}}`, "first\nsecond"),
	)
	// per docs/adr/0068-go-native-harness-execution.md:149
	It("refuses a selected non-scalar answer instead of replacing it", func() {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "codex", "response")
		request["Input"] = `{"type":"item.completed","item":{"type":"agent_message","text":"\ud800"}}`
		result := nativeInvoke(root, cwd, request)
		Expect(result).To(HaveKey("error"))
		Expect(result["answer"]).To(Equal(""))
	})
})

var _ = Describe("G2 native harness trust and ownership", func() {
	BeforeEach(nativeBuild)
	// per docs/adr/0068-go-native-harness-execution.md:95
	DescribeTable("inspects persistent Codex hook trust without waiting for EOF", func(trust string, allowed bool) {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "codex", "preflight")
		request["Request"].(map[string]any)["Role"] = "implementer"
		transcript := filepath.Join(cwd, "transcript.json")
		log := filepath.Join(cwd, "calls")
		result := nativeInvoke(root, cwd, request, "NATIVE_TRUST="+trust, "NATIVE_TRANSCRIPT="+transcript, "NATIVE_LOG="+log)
		if allowed {
			Expect(result).NotTo(HaveKey("error"))
		} else {
			Expect(result).To(HaveKey("error"))
			Expect(result["error"]).NotTo(ContainSubstring("PRIVATE"))
		}
		raw, err := os.ReadFile(transcript)
		Expect(err).NotTo(HaveOccurred())
		var rows []map[string]any
		Expect(json.Unmarshal(raw, &rows)).To(Succeed())
		Expect(rows).To(HaveLen(5))
		methods := []any{}
		for _, row := range rows {
			methods = append(methods, row["method"])
		}
		Expect(methods).To(Equal([]any{"initialize", "initialized", "config/read", "hooks/list", "configRequirements/read"}))
		Expect(rows[2]["params"].(map[string]any)["cwd"]).To(Equal(root))
		calls, err := os.ReadFile(log)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Split(strings.TrimSpace(string(calls)), "\n")).To(HaveLen(2))
	}, Entry("trusted", "trusted", true), Entry("managed", "managed", true), Entry("untrusted", "untrusted", false), Entry("asynchronous", "async", false), Entry("wrong command", "wrong-command", false), Entry("disabled feature", "disabled", false))
	// per docs/adr/0068-go-native-harness-execution.md:114
	It("rechecks allowance after a successful but late ownership callback before delivering stdin", func() {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "opencode", "execute")
		request["AllowanceMillis"] = 50
		request["CallbackDelayMillis"] = 150
		marker := filepath.Join(cwd, "prompt-received")
		result := nativeInvoke(root, cwd, request, "NATIVE_STDIN_MARKER="+marker)
		run := nativeExecution(result)
		Expect(run["Outcome"]).To(Equal("timeout"))
		Expect(nativeRaw(run)).To(BeEmpty())
		expectNativeReaped(run)
		received, readErr := os.ReadFile(marker)
		Expect(os.IsNotExist(readErr) || readErr == nil).To(BeTrue())
		Expect(received).To(BeEmpty(), "child must not receive prompt bytes before successful live ownership callback")
	})
	// per docs/adr/0068-go-native-harness-execution.md:111
	It("drains full stderr concurrently with full stdout and large stdin", func() {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "opencode", "execute")
		request["AllowanceMillis"] = 4000
		request["Request"].(map[string]any)["Prompt"] = strings.Repeat("prompt", 1<<17)
		result := nativeInvoke(root, cwd, request, "NATIVE_MODE=full-pipes")
		run := nativeExecution(result)
		Expect(run["Outcome"]).To(Equal("completed"))
		Expect(nativeRaw(run)).NotTo(ContainSubstring("PRIVATE_STDERR"))
		Expect(len(nativeRaw(run))).To(BeNumerically(">", 1<<20))
		expectNativeReaped(run)
	})
	// per docs/adr/0068-go-native-harness-execution.md:122
	It("kills a descendant holding output after its leader exits", func() {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "opencode", "execute")
		request["AllowanceMillis"] = 3000
		result := nativeInvoke(root, cwd, request, "NATIVE_MODE=descendant")
		run := nativeExecution(result)
		Expect(run["Outcome"]).To(Equal("completed"))
		expectNativeReaped(run)
		var child map[string]int
		Expect(json.Unmarshal(nativeRaw(run), &child)).To(Succeed())
		pid := child["child_pid"]
		Expect(pid).To(BeNumerically(">", 0))
		DeferCleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
		Eventually(func() error { return syscall.Kill(pid, 0) }, 3*time.Second).Should(Equal(syscall.ESRCH), "descendant must actually exit, not merely receive a signal request")
	})
})

var _ = Describe("G2 native harness local admission", func() {
	BeforeEach(nativeBuild)
	// per docs/adr/0068-go-native-harness-execution.md:60
	DescribeTable("preserves all canonical roles and their exact Codex hook arguments", func(role string) {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "codex", "prepare")
		request["Request"].(map[string]any)["Role"] = role
		result := nativeInvoke(root, cwd, request)
		baseline := nativeOracle(root, cwd, request)
		Expect(result).NotTo(HaveKey("error"))
		plan := result["plan"].(map[string]any)
		Expect(plan["Argv"]).To(Equal(baseline["argv"]))
		Expect(plan["Stdin"]).To(Equal(baseline["stdin"]))
	}, Entry("implementer", "implementer"), Entry("spec writer", "spec-writer"), Entry("refactorer", "refactorer"), Entry("wiki maintainer", "wiki-maintainer"))
	// per docs/adr/0068-go-native-harness-execution.md:62
	DescribeTable("refuses malformed requests before launching", func(field, value string) {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "opencode", "prepare")
		request["Request"].(map[string]any)[field] = value
		result := nativeInvoke(root, cwd, request)
		Expect(result).To(HaveKey("error"))
	}, Entry("unknown harness", "Harness", "other"), Entry("unknown role", "Role", "other"), Entry("prompt NUL", "Prompt", "private\x00text"), Entry("model NUL", "Model", "private\x00text"), Entry("oversized prompt", "Prompt", strings.Repeat("x", 16*1024*1024+1)))
	// per docs/adr/0068-go-native-harness-execution.md:80
	DescribeTable("preserves universal newlines and baseline frontmatter", func(contents string) {
		root, cwd := nativeFixture()
		writeFixture(filepath.Join(root, ".opencode/agent/reviewer.md"), []byte(contents), 0600)
		request := nativeRequest(root, "codex", "prepare")
		baseline := nativeOracle(root, cwd, request)
		result := nativeInvoke(root, cwd, request)
		Expect(result).NotTo(HaveKey("error"))
		Expect(result["plan"].(map[string]any)["Stdin"]).To(Equal(baseline["stdin"]))
	}, Entry("CRLF frontmatter", "---\r\nname: test\r\n---\r\nInstructions\r\n"), Entry("CR newlines", "---\rname: test\r---\rInstructions\r"), Entry("plain preserves whitespace", "  Instructions\n\n"), Entry("prefix closing delimiter", "---\nx\n---suffix\nInstructions"))
	// per docs/adr/0068-go-native-harness-execution.md:83
	DescribeTable("refuses missing or oversized role configuration", func(kind string) {
		root, cwd := nativeFixture()
		role := filepath.Join(root, ".opencode/agent/reviewer.md")
		switch kind {
		case "missing":
			Expect(os.Remove(role)).To(Succeed())
		case "empty":
			writeFixture(role, []byte(" \n"), 0600)
		case "frontmatter":
			writeFixture(role, []byte("---\nno closing delimiter"), 0600)
		case "oversized":
			writeFixture(role, []byte(strings.Repeat("x", 1024*1024+1)), 0600)
		case "invalid-utf8":
			writeFixture(role, []byte{0xff}, 0600)
		}
		result := nativeInvoke(root, cwd, nativeRequest(root, "codex", "prepare"))
		Expect(result).To(HaveKey("error"))
	}, Entry("missing", "missing"), Entry("empty", "empty"), Entry("frontmatter", "frontmatter"), Entry("oversized", "oversized"), Entry("invalid UTF8", "invalid-utf8"))
})

var _ = Describe("G2 native harness Claude role parity", func() {
	BeforeEach(nativeBuild)
	// per docs/adr/0068-go-native-harness-execution.md:68
	It("uses native acceptEdits for implementer with literal task stdin", func() {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "claude", "prepare")
		request["Request"].(map[string]any)["Role"] = "implementer"
		baseline := nativeOracle(root, cwd, request)
		result := nativeInvoke(root, cwd, request)
		Expect(result).NotTo(HaveKey("error"))
		plan := result["plan"].(map[string]any)
		Expect(plan["Argv"]).To(Equal(baseline["argv"]))
		Expect(plan["Argv"]).To(ContainElement("acceptEdits"))
		Expect(plan["Stdin"]).To(Equal("PRIVATE_TASK\n"))
	})
})

var _ = Describe("G2 native harness cancellation boundary", func() {
	BeforeEach(nativeBuild)
	// per docs/adr/0068-go-native-harness-execution.md:131
	It("translates driver SIGTERM to context cancellation and reaps its child", func() {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "opencode", "execute")
		request["SignalDelayMillis"] = 150
		request["AllowanceMillis"] = 5000
		result := nativeInvoke(root, cwd, request, "NATIVE_MODE=stall")
		run := nativeExecution(result)
		Expect(run["Outcome"]).To(Equal("interrupted"))
		Expect(run["ExitCode"]).To(Equal(float64(-9)))
		expectNativeReaped(run)
	})
	// per docs/adr/0068-go-native-harness-execution.md:53
	It("refuses an absent ownership callback before launch", func() {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "opencode", "execute")
		request["NilCallback"] = true
		log := filepath.Join(cwd, "calls")
		result := nativeInvoke(root, cwd, request, "NATIVE_LOG="+log)
		Expect(result).To(HaveKey("error"))
		Expect(result).NotTo(HaveKey("callback_pid"))
		_, err := os.Stat(log)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

var _ = Describe("G2 native harness Unicode help boundary", func() {
	BeforeEach(nativeBuild)
	// per docs/adr/0068-go-native-harness-execution.md:93
	It("does not accept a required flag followed by a Unicode word character", func() {
		root, cwd := nativeFixture()
		help := "--json --sandbox --cd --modelé --config"
		oracleRequest := nativeRequest(root, "codex", "help-flags")
		oracleRequest["Input"] = help
		baseline := nativeOracle(root, cwd, oracleRequest)
		Expect(baseline["missing"]).To(Equal([]any{"--model"}))
		result := nativeInvoke(root, cwd, nativeRequest(root, "codex", "preflight"), "NATIVE_HELP_TEXT="+help)
		Expect(result).To(HaveKey("error"))
	})
})

var _ = Describe("G2 native harness regression boundaries", func() {
	BeforeEach(nativeBuild)
	// per docs/adr/0068-go-native-harness-execution.md:92
	DescribeTable("preserves explicit cwd-local PATH executable resolution", func(path string) {
		root, _ := nativeFixture()
		writeFixture(filepath.Join(root, "opencode"), nativeDriver, 0755)
		oracleRequest := nativeRequest(root, "opencode", "preflight-oracle")
		oracleRequest["Path"] = path
		baseline := nativeOracle(root, root, oracleRequest)
		Expect(baseline).NotTo(HaveKey("error"))
		result := nativeInvoke(root, root, nativeRequest(root, "opencode", "preflight"), "PATH="+path)
		Expect(result).NotTo(HaveKey("error"))
	}, Entry("explicit dot", "."), Entry("empty PATH component", ":"))
	// per docs/adr/0068-go-native-harness-execution.md:99
	It("refuses malformed trailing trust data even when the probe exits immediately", func() {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "codex", "preflight")
		request["Request"].(map[string]any)["Role"] = "implementer"
		result := nativeInvoke(root, cwd, request, "NATIVE_TRUST=trailing-malformed")
		Expect(result).To(HaveKey("error"))
		Expect(result["error"]).NotTo(ContainSubstring("PRIVATE"))
	})
})

var _ = Describe("G2 native harness overlay integer boundary", func() {
	BeforeEach(nativeBuild)
	// per docs/adr/0068-go-native-harness-execution.md:83
	DescribeTable("preserves baseline JSON integer digit boundary", func(digits int, allowed bool) {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "opencode", "prepare")
		request["Environment"] = map[string]string{"OPENCODE_CONFIG_CONTENT": `{"unknown":` + strings.Repeat("1", digits) + `}`}
		result := nativeInvoke(root, cwd, request)
		if allowed {
			Expect(result).NotTo(HaveKey("error"))
			baseline := nativeOracle(root, cwd, request)
			Expect(baseline).To(HaveKey("env"))
		} else {
			Expect(result).To(HaveKey("error"))
		}
	}, Entry("4300 digits", 4300, true), Entry("4301 digits", 4301, false))
})

var _ = Describe("G2 native harness lexical root boundary", func() {
	BeforeEach(nativeBuild)
	// per docs/adr/0068-go-native-harness-execution.md:60
	It("reads policy from the actual symlink-parent cwd rather than a cleaned lexical neighbor", func() {
		root, cwd := nativeFixture()
		Expect(os.Mkdir(filepath.Join(root, "child"), 0700)).To(Succeed())
		Expect(os.Symlink(filepath.Join(root, "child"), filepath.Join(cwd, "link"))).To(Succeed())
		writeFixture(filepath.Join(cwd, ".opencode/agent/reviewer.md"), []byte("WRONG_NEIGHBOR_INSTRUCTIONS"), 0600)
		writeFixture(filepath.Join(cwd, "opencode.json"), []byte(`{"agent":{"reviewer":{"permission":{"edit":"allow"}}}}`), 0600)
		request := nativeRequest(root, "codex", "prepare")
		request["Request"].(map[string]any)["Root"] = cwd + "/link/.."
		baseline := nativeOracle(root, cwd, request)
		Expect(baseline["stdin"]).To(Equal("Factory role: reviewer\n\nCanonical instructions\n\nTask:\nPRIVATE_TASK\n"))
		result := nativeInvoke(root, cwd, request)
		Expect(result).NotTo(HaveKey("error"))
		plan := result["plan"].(map[string]any)
		Expect(plan["Stdin"]).To(Equal(baseline["stdin"]))
		Expect(plan["Argv"]).To(ContainElement("read-only"))
	})
})

var _ = Describe("G2 native harness relative PATH cwd boundary", func() {
	BeforeEach(nativeBuild)
	// per docs/adr/0068-go-native-harness-execution.md:92
	DescribeTable("selects the root executable after relative caller-path admission", func(action string) {
		root, cwd := nativeFixture()
		writeFixture(filepath.Join(root, "opencode"), nativeDriver, 0755)
		writeFixture(filepath.Join(cwd, "opencode"), nativeDriver, 0755)
		log := filepath.Join(cwd, "selected-executable.jsonl")
		if action == "preflight" {
			oracleRequest := nativeRequest(root, "opencode", "preflight-oracle")
			oracleRequest["Path"] = "."
			oracleRequest["Log"] = log
			baseline := nativeOracle(root, cwd, oracleRequest)
			Expect(baseline).NotTo(HaveKey("error"))
			raw, err := os.ReadFile(log)
			Expect(err).NotTo(HaveOccurred())
			var call map[string]any
			Expect(json.Unmarshal(raw, &call)).To(Succeed())
			expectNativeExecutable(call["executable"].(string), filepath.Join(root, "opencode"))
			Expect(os.Remove(log)).To(Succeed())
		}
		result := nativeInvoke(root, cwd, nativeRequest(root, "opencode", action), "PATH=.", "NATIVE_LOG="+log)
		Expect(result).NotTo(HaveKey("error"))
		raw, err := os.ReadFile(log)
		Expect(err).NotTo(HaveOccurred())
		var call map[string]any
		Expect(json.Unmarshal(raw, &call)).To(Succeed())
		expectNativeExecutable(call["executable"].(string), filepath.Join(root, "opencode"))
	}, Entry("preflight", "preflight"), Entry("execute", "execute"))
})

var _ = Describe("G2 native harness probe resource boundary", func() {
	BeforeEach(nativeBuild)
	// per docs/adr/0068-go-native-harness-execution.md:102
	DescribeTable("bounds otherwise valid help output exactly", func(size int, allowed bool) {
		root, cwd := nativeFixture()
		result := nativeInvoke(root, cwd, nativeRequest(root, "claude", "preflight"), "NATIVE_MODE=help-size", "NATIVE_HELP_BYTES="+strconv.Itoa(size))
		if allowed {
			Expect(result).NotTo(HaveKey("error"))
		} else {
			Expect(result).To(HaveKey("error"))
		}
	}, Entry("exact two MiB", 2*1024*1024, true), Entry("one byte excess", 2*1024*1024+1, false))
	// per docs/adr/0068-go-native-harness-execution.md:106
	It("cancels and reaps a hung help probe within the parent context", func() {
		root, cwd := nativeFixture()
		request := nativeRequest(root, "claude", "preflight")
		request["SignalDelayMillis"] = 1000
		log := filepath.Join(cwd, "probe-call")
		result := nativeInvoke(root, cwd, request, "NATIVE_MODE=stall", "NATIVE_LOG="+log)
		Expect(result).To(HaveKey("error"))
		raw, err := os.ReadFile(log)
		Expect(err).NotTo(HaveOccurred())
		var child map[string]any
		Expect(json.Unmarshal(raw, &child)).To(Succeed())
		Expect(syscall.Kill(int(child["pid"].(float64)), 0)).To(Equal(syscall.ESRCH))
	})
})

func expectNativeExecutable(actual, expected string) {
	GinkgoHelper()
	selected, err := os.Stat(actual)
	Expect(err).NotTo(HaveOccurred())
	wanted, err := os.Stat(expected)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.SameFile(selected, wanted)).To(BeTrue(), "native execution must select the expected file identity")
}
