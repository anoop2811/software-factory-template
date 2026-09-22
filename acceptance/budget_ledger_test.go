package acceptance_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/big"
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

// The evaluator is compiled only into a temporary directory, never installed.
// per docs/adr/0069-go-budget-ledger-admission.md:12
const budgetDriverSource = `package main
import("context";"encoding/json";"os";"strings";"time";"github.com/anoop2811/software-factory-template/internal/budget")
type command struct{Action,Root,ID string;Environment map[string]string;Request budget.Request;History json.RawMessage;PID,TimeoutMS int;Completion budget.Completion}
func main(){decoder:=json.NewDecoder(os.Stdin);encoder:=json.NewEncoder(os.Stdout);lastID:="";for{var c command;if decoder.Decode(&c)!=nil{return};ctx:=context.Background();cancel:=func(){};if c.TimeoutMS!=0{ctx,cancel=context.WithTimeout(ctx,time.Duration(c.TimeoutMS)*time.Millisecond)}
 out:=map[string]any{};ledger:=budget.NewLedger(c.Root);cfg,err:=budget.Configuration(c.Environment)
 if c.Action=="read"{h,e:=ledger.Read(ctx);out["history"]=h;err=e
 }else if c.Action=="parse"{h,e:=budget.ParseHistory(ctx,strings.NewReader(string(c.History)));out["history"]=h;err=e
 }else if c.Action=="report"||c.Action=="report-cancel"{parseCtx:=ctx;if c.Action=="report-cancel"{parseCtx=context.Background()};h,e:=budget.ParseHistory(parseCtx,strings.NewReader(string(c.History)));err=e;if e==nil{r,e:=budget.Report(ctx,h,c.Request.Session);out["report"]=r;err=e}
 }else if c.Action=="publish"{id:=c.ID;if id==""{id=lastID};pid:=c.PID;if pid==0{pid=os.Getpid()};r,e:=ledger.PublishPID(ctx,id,pid);out["record"]=r;err=e
 }else if c.Action=="finalize"{id:=c.ID;if id==""{id=lastID};r,e:=ledger.Finalize(ctx,id,c.Completion,cfg);out["record"]=r;err=e
 }else if err==nil{switch c.Action{
 case "config":out["config"]=cfg
 case "admit":a,e:=ledger.Admit(ctx,c.Request,cfg);out["plan"]=a.Plan;out["record"]=a.Record;err=e;if a.Record!=nil{raw,_:=json.Marshal(a.Record);var row map[string]any;json.Unmarshal(raw,&row);lastID,_=row["id"].(string)}
 case "plan","report":h,e:=budget.ParseHistory(ctx,strings.NewReader(string(c.History)));err=e;if e==nil{if c.Action=="plan"{p,e:=budget.MakePlan(ctx,c.Request,cfg,h);out["plan"]=p;err=e}else{r,e:=budget.Report(ctx,h,c.Request.Session);out["report"]=r;err=e}}
 }}
 if err!=nil{out["error"]=err.Error()};cancel();encoder.Encode(out)
}}
`

var budgetDriver []byte

func budgetBuild() {
	GinkgoHelper()
	if budgetDriver != nil {
		return
	}
	root, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	dir, err := os.MkdirTemp("", "factory-budget-driver-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, dir)
	module, err := os.ReadFile(filepath.Join(root, "go.mod"))
	Expect(err).NotTo(HaveOccurred())
	version := strings.Fields(strings.SplitN(string(module), "\ngo ", 2)[1])[0]
	writeFixture(filepath.Join(dir, "go.mod"), []byte("module github.com/anoop2811/software-factory-template/acceptance/budgetfixture\n\ngo "+version+"\nrequire github.com/anoop2811/software-factory-template v0.0.0\nreplace github.com/anoop2811/software-factory-template => "+root+"\n"), 0600)
	writeFixture(filepath.Join(dir, "main.go"), []byte(budgetDriverSource), 0600)
	args := []string{"build", "-mod=mod", "-o", filepath.Join(dir, "driver"), "."}
	if os.Getenv("FACTORY_BUDGET_TEST_RACE") == "1" {
		args = append([]string{"build", "-race"}, args[1:]...)
	}
	cmd := exec.Command("go", args...) // #nosec G204 -- fixed Go builder and evaluator-owned module/output arguments.
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOPROXY=off")
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "budget API compile: %s", output)
	budgetDriver, err = os.ReadFile(filepath.Join(dir, "driver"))
	Expect(err).NotTo(HaveOccurred())
}
func budgetFixture() (string, string) {
	GinkgoHelper()
	root, cwd := fixture()
	writeFixture(filepath.Join(root, "budget-driver"), budgetDriver, 0755)
	return root, cwd
}
func budgetCommand(root, action string) map[string]any {
	return map[string]any{"Action": action, "Root": root, "Environment": map[string]string{"FACTORY_BUDGET_ENABLED": "true"}, "Request": map[string]any{"Session": "session", "Task": "task", "Harness": "claude", "Role": "reviewer", "Model": ""}, "History": json.RawMessage(`{"schema":1,"runs":[]}`)}
}
func budgetInvoke(root, cwd string, commands ...map[string]any) []map[string]any {
	GinkgoHelper()
	var input bytes.Buffer
	for _, c := range commands {
		Expect(json.NewEncoder(&input).Encode(c)).To(Succeed())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(root, "budget-driver")) // #nosec G204 G702 -- test-built isolated executable; request data only on stdin.
	cmd.Dir = cwd
	cmd.Env = []string{"PATH=/absent-native-tools", "GORACE=atexit_sleep_ms=0", "PYTHONDONTWRITEBYTECODE=1"}
	cmd.Stdin = &input
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred())
	Expect(err).NotTo(HaveOccurred(), stderr.String())
	Expect(stderr.String()).To(BeEmpty())
	decoder := json.NewDecoder(&stdout)
	decoder.UseNumber()
	results := []map[string]any{}
	for range commands {
		var out map[string]any
		Expect(decoder.Decode(&out)).To(Succeed())
		results = append(results, out)
	}
	return results
}

var _ = Describe("G2 interoperable budget ledger", func() {
	BeforeEach(budgetBuild)
	// per docs/adr/0069-go-budget-ledger-admission.md:36
	It("preserves the baseline configuration defaults", func() {
		root, cwd := budgetFixture()
		c := budgetCommand(root, "config")
		c["Environment"] = map[string]string{}
		out := budgetInvoke(root, cwd, c)[0]
		Expect(out).NotTo(HaveKey("error"))
		cfg := out["config"].(map[string]any)
		Expect(cfg["enabled"]).To(BeFalse())
		Expect(cfg["max_attempts"]).To(Equal(json.Number("1")))
		Expect(cfg["action"]).To(Equal("stop"))
		Expect(cfg["estimated_usd"]).To(BeNil())
	})
	// per docs/adr/0069-go-budget-ledger-admission.md:46
	It("reads missing history without creating storage", func() {
		root, cwd := budgetFixture()
		out := budgetInvoke(root, cwd, budgetCommand(root, "read"))[0]
		Expect(out).NotTo(HaveKey("error"))
		Expect(out["history"]).To(Equal(map[string]any{"schema": json.Number("1"), "runs": []any{}}))
		_, err := os.Stat(filepath.Join(root, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0069-go-budget-ledger-admission.md:79
	It("durably admits exactly one attempt and blocks a repeated task", func() {
		root, cwd := budgetFixture()
		out := budgetInvoke(root, cwd, budgetCommand(root, "admit"), budgetCommand(root, "admit"))
		Expect(out[0]).NotTo(HaveKey("error"))
		Expect(out[0]["record"]).NotTo(BeNil())
		Expect(out[1]).NotTo(HaveKey("error"))
		Expect(out[1]["record"]).To(BeNil())
		raw, err := os.ReadFile(filepath.Join(root, ".factory/budget.json"))
		Expect(err).NotTo(HaveOccurred())
		var h map[string]any
		Expect(json.Unmarshal(raw, &h)).To(Succeed())
		Expect(h["runs"]).To(HaveLen(1))
	})
})

func budgetDecode(raw []byte) any {
	GinkgoHelper()
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var out any
	Expect(d.Decode(&out)).To(Succeed())
	return out
}
func budgetComparable(value any) any {
	switch v := value.(type) {
	case json.Number:
		r, ok := new(big.Rat).SetString(v.String())
		Expect(ok).To(BeTrue())
		return r.RatString()
	case map[string]any:
		out := map[string]any{}
		for k, x := range v {
			out[k] = budgetComparable(x)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = budgetComparable(x)
		}
		return out
	default:
		return value
	}
}
func budgetHistory(rows ...map[string]any) json.RawMessage {
	GinkgoHelper()
	data, err := json.Marshal(map[string]any{"schema": 1, "runs": rows})
	Expect(err).NotTo(HaveOccurred())
	return data
}
func budgetRow(id string) map[string]any {
	return map[string]any{"id": id, "session": "session", "task": "task", "harness": "claude", "role": "reviewer", "status": "completed", "outcome": "completed", "elapsed_seconds": json.Number("1.0"), "reserved_seconds": json.Number("2.0"), "attempt": 1, "owner_pid": os.Getpid(), "complete": true, "warnings": []any{}, "model": "", "started_at": "2026-01-01T00:00:00Z", "ended_at": "2026-01-01T00:00:01Z", "source": "client estimate", "tokens": map[string]any{"input_tokens": 2}, "estimated_usd": json.Number("0.1"), "process_pid": nil, "exit_code": 0}
}
func budgetOracle(root, cwd string, c map[string]any) map[string]any {
	GinkgoHelper()
	original := exec.Command("git", "show", "4cc771e894e11d5024032106aeb0ef788997bd29"+":scripts/lib/budget.py") // #nosec G204 -- immutable source and fixed path.
	source, err := original.Output()
	Expect(err).NotTo(HaveOccurred())
	path := filepath.Join(root, "immutable-budget.py")
	writeFixture(path, source, 0600)
	python, err := exec.LookPath("python3")
	Expect(err).NotTo(HaveOccurred())
	input, err := json.Marshal(c)
	Expect(err).NotTo(HaveOccurred())
	result := usageProcess(cwd, string(input), "/usr/bin/env", "PYTHONDONTWRITEBYTECODE=1", "PATH="+os.Getenv("PATH"), "HOME="+os.Getenv("HOME"), python, "-I", "-B", "-c", `import ast,json,os,sys,types
sys.set_int_max_str_digits(4300)
tree=ast.parse(open(sys.argv[1],encoding="utf-8").read())
tree.body=[n for n in tree.body if not(isinstance(n,ast.Import) and any(a.name=="budget_adapters" for a in n.names))]
m={"__name__":"budget_oracle"}
exec(compile(tree,sys.argv[1],"exec"),m)
q=json.load(sys.stdin)
r=q.get("Request",{})
os.environ.update(q.get("Environment",{}))
os.environ["FACTORY_BUDGET_MODEL"]=r.get("Model","")
try:
    action=q["Action"]
    if action=="config":
        out={"config":m["configuration"]()}
    elif action=="parse":
        out={"history":m["validate_history"](q["History"])}
    elif action=="report":
        out={"report":m["report"](m["validate_history"](q["History"]),r.get("Session") or None)}
    elif action=="python-write":
        ledger=m["Ledger"](m["Path"](q["Root"]))
        with ledger.locked():
            ledger.write(q["History"])
        out={"history":ledger.read()}
    elif action=="read":
        out={"history":m["Ledger"](m["Path"](q["Root"])).read()}
    else:
        args=types.SimpleNamespace(session=r["Session"],task=r["Task"],harness=r["Harness"],role=r["Role"],max_cost_usd=r.get("MaxCostUSD"))
        out={"plan":m["make_plan"](args,m["configuration"](),m["validate_history"](q["History"]))}
    print(json.dumps(out,allow_nan=False))
except (m["BudgetError"],ValueError,TypeError,KeyError) as e:
    print(json.dumps({"error":str(e)}))`, path)
	Expect(result.status).To(Equal(0), result.stderr)
	return budgetDecode([]byte(result.stdout)).(map[string]any)
}

var _ = Describe("G2 budget configuration and schema parity", func() {
	BeforeEach(budgetBuild)
	// per docs/adr/0069-go-budget-ledger-admission.md:36
	DescribeTable("matches configuration validation with explicit admission expectations", func(key, value string, allowed bool) {
		root, cwd := budgetFixture()
		c := budgetCommand(root, "config")
		c["Environment"].(map[string]string)[key] = value
		baseline := budgetOracle(root, cwd, c)
		out := budgetInvoke(root, cwd, c)[0]
		if allowed {
			Expect(baseline).NotTo(HaveKey("error"))
			Expect(out).NotTo(HaveKey("error"))
			Expect(budgetComparable(out["config"])).To(Equal(budgetComparable(baseline["config"])))
		} else {
			Expect(baseline).To(HaveKey("error"))
			Expect(out).To(HaveKey("error"))
		}
	}, Entry("leading-zero integer", "FACTORY_BUDGET_MAX_ATTEMPTS", "01", true), Entry("integer whitespace", "FACTORY_BUDGET_MAX_ATTEMPTS", " 1 ", false), Entry("integer exponent", "FACTORY_BUDGET_MAX_ATTEMPTS", "1e2", false), Entry("invalid underscore before exponent", "FACTORY_BUDGET_TIMEOUT_SECONDS", "1_e2", false), Entry("float underscore", "FACTORY_BUDGET_TIMEOUT_SECONDS", "1_000.5", true), Entry("float exponent", "FACTORY_BUDGET_SESSION_SECONDS", " 1e3 ", true), Entry("nonfinite", "FACTORY_BUDGET_TIMEOUT_SECONDS", "NaN", false), Entry("zero", "FACTORY_BUDGET_MAX_CONCURRENT", "0", false), Entry("bad enabled", "FACTORY_BUDGET_ENABLED", "TRUE", false), Entry("bad action", "FACTORY_BUDGET_ACTION", "ignore", false))
	// per docs/adr/0069-go-budget-ledger-admission.md:22
	DescribeTable("matches schema distinctions without repairing history", func(change string, allowed bool) {
		root, cwd := budgetFixture()
		row := budgetRow("r")
		switch change {
		case "bool attempt":
			row["attempt"] = true
		case "float attempt":
			row["attempt"] = json.Number("1.0")
		case "negative token":
			row["tokens"] = map[string]any{"x": -1}
		case "optional absence":
			delete(row, "tokens")
			delete(row, "estimated_usd")
			delete(row, "process_pid")
		case "permissive running":
			row["status"] = "active"
			row["outcome"] = "running"
			delete(row, "ended_at")
			row["exit_code"] = 9
		case "invalid uncertainty":
			row["status"] = "active"
			row["outcome"] = "timeout"
			delete(row, "ended_at")
		case "large exact token":
			row["tokens"] = map[string]any{"input_tokens": json.Number("9007199254740993")}
		case "unknown fields":
			row["unknown"] = map[string]any{"literal": "$(never execute)", "integer": json.Number("9007199254740993")}
		}
		c := budgetCommand(root, "parse")
		c["History"] = budgetHistory(row)
		baseline := budgetOracle(root, cwd, c)
		out := budgetInvoke(root, cwd, c)[0]
		if allowed {
			Expect(baseline).NotTo(HaveKey("error"))
			Expect(out).NotTo(HaveKey("error"))
			Expect(out["history"]).To(Equal(baseline["history"]))
		} else {
			Expect(baseline).To(HaveKey("error"))
			Expect(out).To(HaveKey("error"))
		}
	}, Entry("boolean attempt", "bool attempt", false), Entry("float attempt", "float attempt", false), Entry("negative token", "negative token", false), Entry("optional absent", "optional absence", true), Entry("permissive running", "permissive running", true), Entry("uncertainty requires unknown metadata", "invalid uncertainty", false), Entry("exact large token", "large exact token", true), Entry("opaque extensions", "unknown fields", true))
})

var _ = Describe("G2 budget planning and reporting", func() {
	BeforeEach(budgetBuild)
	// per docs/adr/0069-go-budget-ledger-admission.md:51
	DescribeTable("matches ordered blockers and session versus checkout accounting", func(harness, action string) {
		root, cwd := budgetFixture()
		completed := budgetRow("done")
		uncertain := budgetRow("uncertain")
		uncertain["session"] = "other"
		uncertain["status"] = "active"
		uncertain["outcome"] = "timeout"
		uncertain["ended_at"] = nil
		uncertain["process_pid"] = os.Getpid()
		uncertain["exit_code"] = nil
		uncertain["tokens"] = nil
		uncertain["estimated_usd"] = nil
		uncertain["complete"] = false
		c := budgetCommand(root, "plan")
		c["History"] = budgetHistory(completed, uncertain)
		c["Request"].(map[string]any)["Harness"] = harness
		c["Environment"].(map[string]string)["FACTORY_BUDGET_ACTION"] = action
		c["Environment"].(map[string]string)["FACTORY_BUDGET_ESTIMATED_USD"] = "0.1"
		baseline := budgetOracle(root, cwd, c)
		out := budgetInvoke(root, cwd, c)[0]
		Expect(out).NotTo(HaveKey("error"))
		Expect(budgetComparable(out["plan"])).To(Equal(budgetComparable(baseline["plan"])))
		plan := out["plan"].(map[string]any)
		Expect(plan["remaining_attempts"]).To(Equal(json.Number("0")))
		Expect(plan["active_runs"]).To(Equal(json.Number("1")))
		Expect(plan["blockers"].([]any)[0]).To(ContainSubstring("unconfirmed process exit"))
	}, Entry("Codex stop", "codex", "stop"), Entry("Claude warn", "claude", "warn"), Entry("OpenCode stop", "opencode", "stop"))
	// per docs/adr/0069-go-budget-ledger-admission.md:26
	DescribeTable("matches report filtering and compensated float sums", func(session string) {
		root, cwd := budgetFixture()
		rows := []map[string]any{}
		for i := 0; i < 10; i++ {
			r := budgetRow("r" + strconv.Itoa(i))
			r["elapsed_seconds"] = json.Number("0.1")
			rows = append(rows, r)
		}
		other := budgetRow("other")
		other["session"] = "other"
		other["estimated_usd"] = nil
		rows = append(rows, other)
		c := budgetCommand(root, "report")
		c["History"] = budgetHistory(rows...)
		c["Request"].(map[string]any)["Session"] = session
		c["Environment"] = map[string]string{"FACTORY_BUDGET_ENABLED": "invalid-unrelated"}
		baseline := budgetOracle(root, cwd, c)
		out := budgetInvoke(root, cwd, c)[0]
		Expect(out).NotTo(HaveKey("error"))
		Expect(budgetComparable(out["report"])).To(Equal(budgetComparable(baseline["report"])))
	}, Entry("all sessions", ""), Entry("one session", "session"))
})

func budgetCompletion(outcome string) map[string]any {
	return map[string]any{"Outcome": outcome, "ExitCode": 0, "ElapsedSeconds": json.Number("1.0"), "ExitConfirmed": true, "OwnershipUnconfirmed": false, "Tokens": map[string]any{"input_tokens": 1}, "EstimatedUSD": json.Number("0.1"), "Complete": true, "Source": "fixture accounting"}
}

var _ = Describe("G2 budget reservation lifecycle", func() {
	BeforeEach(budgetBuild)
	// per docs/adr/0069-go-budget-ledger-admission.md:95
	It("publishes ownership then completes the original reservation", func() {
		root, cwd := budgetFixture()
		publish := budgetCommand(root, "publish")
		publish["PID"] = 1234
		finish := budgetCommand(root, "finalize")
		finish["Completion"] = budgetCompletion("completed")
		results := budgetInvoke(root, cwd, budgetCommand(root, "admit"), publish, finish)
		for _, result := range results {
			Expect(result).NotTo(HaveKey("error"))
		}
		initial := results[0]["record"].(map[string]any)
		record := results[2]["record"].(map[string]any)
		Expect(record["id"]).To(Equal(initial["id"]))
		Expect(record["reserved_seconds"]).To(Equal(initial["reserved_seconds"]))
		Expect(record["status"]).To(Equal("completed"))
		Expect(record["process_pid"]).To(Equal(json.Number("1234")))
		Expect(record["complete"]).To(BeTrue())
		baseline := budgetOracle(root, cwd, budgetCommand(root, "read"))
		Expect(baseline).NotTo(HaveKey("error"))
	})
	// per docs/adr/0069-go-budget-ledger-admission.md:108
	DescribeTable("retains schema-valid active uncertainty for every terminal outcome", func(outcome, want string) {
		root, cwd := budgetFixture()
		publish := budgetCommand(root, "publish")
		publish["PID"] = 1234
		finish := budgetCommand(root, "finalize")
		completion := budgetCompletion(outcome)
		completion["OwnershipUnconfirmed"] = true
		finish["Completion"] = completion
		results := budgetInvoke(root, cwd, budgetCommand(root, "admit"), publish, finish)
		Expect(results[2]).NotTo(HaveKey("error"))
		record := results[2]["record"].(map[string]any)
		Expect(record["status"]).To(Equal("active"))
		Expect(record["outcome"]).To(Equal(want))
		Expect(record["exit_code"]).To(BeNil())
		Expect(record["tokens"]).To(BeNil())
		Expect(record["estimated_usd"]).To(BeNil())
		Expect(record["complete"]).To(BeFalse())
		Expect(record["ended_at"]).To(BeNil())
		Expect(record["process_pid"]).To(Equal(json.Number("1234")))
		Expect(budgetOracle(root, cwd, budgetCommand(root, "read"))).NotTo(HaveKey("error"))
	}, Entry("timeout", "timeout", "timeout"), Entry("failed", "failed", "launch_error"), Entry("interrupted", "interrupted", "launch_error"), Entry("output limit", "output_limit", "launch_error"), Entry("completed leader", "completed", "launch_error"), Entry("launch error", "launch_error", "launch_error"))
	// per docs/adr/0069-go-budget-ledger-admission.md:103
	DescribeTable("refuses inconsistent completion without rewriting history", func(kind string) {
		root, cwd := budgetFixture()
		publish := budgetCommand(root, "publish")
		publish["PID"] = 1234
		finish := budgetCommand(root, "finalize")
		completion := budgetCompletion("completed")
		switch kind {
		case "conflicting pid":
			completion["ProcessPID"] = 2345
		case "unknown outcome":
			completion["Outcome"] = "unknown"
		case "confirmed without code":
			completion["ExitCode"] = nil
		}
		finish["Completion"] = completion
		results := budgetInvoke(root, cwd, budgetCommand(root, "admit"), publish, finish, budgetCommand(root, "read"))
		Expect(results[2]).To(HaveKey("error"))
		rows := results[3]["history"].(map[string]any)["runs"].([]any)
		Expect(rows[0]).To(Equal(results[1]["record"]))
	}, Entry("PID conflict", "conflicting pid"), Entry("unknown outcome", "unknown outcome"), Entry("confirmed without code", "confirmed without code"))
	// per docs/adr/0069-go-budget-ledger-admission.md:108
	It("fills missing process identity from retained ownership while preserving uncertainty", func() {
		root, cwd := budgetFixture()
		finish := budgetCommand(root, "finalize")
		completion := budgetCompletion("failed")
		completion["ProcessPID"] = 1234
		completion["OwnershipUnconfirmed"] = true
		finish["Completion"] = completion
		out := budgetInvoke(root, cwd, budgetCommand(root, "admit"), finish)
		Expect(out[1]).NotTo(HaveKey("error"))
		record := out[1]["record"].(map[string]any)
		Expect(record["process_pid"]).To(Equal(json.Number("1234")))
		Expect(record["status"]).To(Equal("active"))
	})
	// per docs/adr/0069-go-budget-ledger-admission.md:99
	It("refuses foreign reservation publication and missing records", func() {
		root, cwd := budgetFixture()
		admitted := budgetInvoke(root, cwd, budgetCommand(root, "admit"))[0]
		Expect(admitted).NotTo(HaveKey("error"))
		id := admitted["record"].(map[string]any)["id"].(string)
		publish := budgetCommand(root, "publish")
		publish["ID"] = id
		publish["PID"] = 1234
		missing := budgetCommand(root, "publish")
		missing["ID"] = "missing"
		missing["PID"] = 1234
		out := budgetInvoke(root, cwd, publish, missing)
		Expect(out[0]).To(HaveKey("error"))
		Expect(out[1]).To(HaveKey("error"))
	})
})

type budgetSession struct {
	cmd    *exec.Cmd
	input  io.WriteCloser
	output *json.Decoder
	stderr bytes.Buffer
	cancel context.CancelFunc
	closed bool
}

func budgetStart(root, cwd string) *budgetSession {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	cmd := exec.CommandContext(ctx, filepath.Join(root, "budget-driver")) // #nosec G204 G702 -- evaluator-owned executable.
	cmd.Dir = cwd
	cmd.Env = []string{"PATH=/absent-native-tools", "GORACE=atexit_sleep_ms=0", "PYTHONDONTWRITEBYTECODE=1"}
	session := &budgetSession{cmd: cmd, cancel: cancel}
	input, err := cmd.StdinPipe()
	Expect(err).NotTo(HaveOccurred())
	output, err := cmd.StdoutPipe()
	Expect(err).NotTo(HaveOccurred())
	session.input = input
	session.output = json.NewDecoder(output)
	session.output.UseNumber()
	cmd.Stderr = &session.stderr
	Expect(cmd.Start()).To(Succeed())
	DeferCleanup(session.close)
	return session
}
func (s *budgetSession) close() {
	GinkgoHelper()
	if s.closed {
		return
	}
	s.closed = true
	_ = s.input.Close()
	err := s.cmd.Wait()
	s.cancel()
	Expect(err).NotTo(HaveOccurred(), s.stderr.String())
}
func (s *budgetSession) send(c map[string]any) {
	GinkgoHelper()
	Expect(json.NewEncoder(s.input).Encode(c)).To(Succeed())
}
func (s *budgetSession) receive() map[string]any {
	GinkgoHelper()
	var out map[string]any
	Expect(s.output.Decode(&out)).To(Succeed())
	return out
}
func (s *budgetSession) call(c map[string]any) map[string]any { s.send(c); return s.receive() }

var _ = Describe("G2 budget storage boundaries", func() {
	BeforeEach(budgetBuild)
	// per docs/adr/0069-go-budget-ledger-admission.md:72
	DescribeTable("refuses unsafe storage entries without following or blocking", func(kind string) {
		root, cwd := budgetFixture()
		storage := filepath.Join(root, ".factory")
		Expect(os.Mkdir(storage, 0700)).To(Succeed())
		history := filepath.Join(storage, "budget.json")
		lock := filepath.Join(storage, "budget.lock")
		outside := filepath.Join(cwd, "outside")
		writeFixture(outside, []byte(`{"schema":1,"runs":[]}`), 0600)
		switch kind {
		case "directory symlink":
			Expect(os.Remove(storage)).To(Succeed())
			Expect(os.Symlink(cwd, storage)).To(Succeed())
		case "history symlink":
			Expect(os.Symlink(outside, history)).To(Succeed())
		case "lock symlink":
			Expect(os.Symlink(outside, lock)).To(Succeed())
		case "history hardlink":
			Expect(os.Link(outside, history)).To(Succeed())
		case "lock hardlink":
			Expect(os.Link(outside, lock)).To(Succeed())
		case "history FIFO":
			Expect(syscall.Mkfifo(history, 0600)).To(Succeed())
		}
		out := budgetInvoke(root, cwd, budgetCommand(root, "read"))[0]
		Expect(out).To(HaveKey("error"))
		raw, err := os.ReadFile(outside)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).To(Equal(`{"schema":1,"runs":[]}`))
	}, Entry("directory symlink", "directory symlink"), Entry("history symlink", "history symlink"), Entry("lock symlink", "lock symlink"), Entry("history hardlink", "history hardlink"), Entry("lock hardlink", "lock hardlink"), Entry("history FIFO", "history FIFO"))
	// per docs/adr/0069-go-budget-ledger-admission.md:76
	It("preserves read-only modes and publishes private modes on mutation", func() {
		root, cwd := budgetFixture()
		storage := filepath.Join(root, ".factory")
		writeFixture(filepath.Join(storage, "budget.json"), []byte(`{"schema":1,"runs":[]}`), 0644)
		Expect(os.Chmod(storage, 0755)).To(Succeed())
		out := budgetInvoke(root, cwd, budgetCommand(root, "read"))[0]
		Expect(out).NotTo(HaveKey("error"))
		info, err := os.Stat(storage)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0755)))
		out = budgetInvoke(root, cwd, budgetCommand(root, "admit"))[0]
		Expect(out).NotTo(HaveKey("error"))
		for _, name := range []string{"budget.json", "budget.lock"} {
			info, err = os.Stat(filepath.Join(storage, name))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
		}
		info, err = os.Stat(storage)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0700)))
	})
	// per docs/adr/0069-go-budget-ledger-admission.md:30
	DescribeTable("bounds actual history input bytes", func(size int, allowed bool) {
		root, cwd := budgetFixture()
		prefix := `{"schema":1,"runs":[]}`
		writeFixture(filepath.Join(root, ".factory/budget.json"), []byte(prefix+strings.Repeat(" ", size-len(prefix))), 0600)
		out := budgetInvoke(root, cwd, budgetCommand(root, "read"))[0]
		if allowed {
			Expect(out).NotTo(HaveKey("error"))
		} else {
			Expect(out).To(HaveKey("error"))
		}
	}, Entry("32MiB", 32*1024*1024, true), Entry("32MiB plus one", 32*1024*1024+1, false))
	// per docs/adr/0069-go-budget-ledger-admission.md:75
	It("preserves lexical symlink-parent root traversal", func() {
		root, cwd := budgetFixture()
		Expect(os.Mkdir(filepath.Join(root, "child"), 0700)).To(Succeed())
		Expect(os.Symlink(filepath.Join(root, "child"), filepath.Join(cwd, "link"))).To(Succeed())
		request := budgetCommand(cwd+"/link/..", "admit")
		out := budgetInvoke(root, cwd, request)[0]
		Expect(out).NotTo(HaveKey("error"))
		_, err := os.Stat(filepath.Join(root, ".factory/budget.json"))
		Expect(err).NotTo(HaveOccurred())
		_, err = os.Stat(filepath.Join(cwd, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

func budgetPythonHold(root string) (func(), os.FileInfo) {
	GinkgoHelper()
	python, err := exec.LookPath("python3")
	Expect(err).NotTo(HaveOccurred())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	command := exec.CommandContext(ctx, python, "-I", "-B", "-c", `import fcntl,json,os,sys
p=os.path.join(sys.argv[1],".factory");os.makedirs(p,mode=0o700,exist_ok=True)
f=open(os.path.join(p,"budget.lock"),"a+");fcntl.flock(f,fcntl.LOCK_EX)
print(json.dumps({"locked":True}),flush=True);sys.stdin.readline()`, root) // #nosec G204 G702 -- local Python fixture, literal program, test-owned root argument.
	command.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	stdin, err := command.StdinPipe()
	Expect(err).NotTo(HaveOccurred())
	stdout, err := command.StdoutPipe()
	Expect(err).NotTo(HaveOccurred())
	var stderr bytes.Buffer
	command.Stderr = &stderr
	Expect(command.Start()).To(Succeed())
	line, err := bufio.NewReader(stdout).ReadString('\n')
	Expect(err).NotTo(HaveOccurred())
	Expect(line).To(ContainSubstring("true"))
	info, err := os.Stat(filepath.Join(root, ".factory/budget.lock"))
	Expect(err).NotTo(HaveOccurred())
	closed := false
	release := func() {
		if closed {
			return
		}
		closed = true
		_, _ = io.WriteString(stdin, "release\n")
		_ = stdin.Close()
		err := command.Wait()
		cancel()
		Expect(err).NotTo(HaveOccurred(), stderr.String())
	}
	DeferCleanup(release)
	return release, info
}

var _ = Describe("G2 budget cooperating process locks", func() {
	BeforeEach(budgetBuild)
	// per docs/adr/0069-go-budget-ledger-admission.md:60
	It("honors Python flock and cancels waiting without replacing the lock inode", func() {
		root, cwd := budgetFixture()
		release, before := budgetPythonHold(root)
		session := budgetStart(root, cwd)
		request := budgetCommand(root, "admit")
		request["TimeoutMS"] = 150
		began := time.Now()
		out := session.call(request)
		Expect(out).To(HaveKey("error"))
		Expect(time.Since(began)).To(BeNumerically(">=", 100*time.Millisecond))
		_, err := os.Stat(filepath.Join(root, ".factory/budget.json"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		release()
		out = session.call(budgetCommand(root, "admit"))
		Expect(out).NotTo(HaveKey("error"))
		Expect(out["record"]).NotTo(BeNil())
		after, err := os.Stat(filepath.Join(root, ".factory/budget.lock"))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
	})
	// per docs/adr/0069-go-budget-ledger-admission.md:79
	It("admits exactly one of two processes competing for the last slot", func() {
		root, cwd := budgetFixture()
		release, _ := budgetPythonHold(root)
		first, second := budgetStart(root, cwd), budgetStart(root, cwd)
		request := budgetCommand(root, "admit")
		first.send(request)
		second.send(request)
		release()
		a, b := first.receive(), second.receive()
		Expect(a).NotTo(HaveKey("error"))
		Expect(b).NotTo(HaveKey("error"))
		admitted := 0
		for _, out := range []map[string]any{a, b} {
			if out["record"] != nil {
				admitted++
			}
		}
		Expect(admitted).To(Equal(1))
		read := first.call(budgetCommand(root, "read"))
		Expect(read["history"].(map[string]any)["runs"]).To(HaveLen(1))
	})
	// per docs/adr/0069-go-budget-ledger-admission.md:103
	It("patches the fresh row without discarding extensions added after admission", func() {
		root, cwd := budgetFixture()
		session := budgetStart(root, cwd)
		admitted := session.call(budgetCommand(root, "admit"))
		Expect(admitted).NotTo(HaveKey("error"))
		path := filepath.Join(root, ".factory/budget.json")
		raw, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		history := budgetDecode(raw).(map[string]any)
		history["top_extension"] = "preserved"
		row := history["runs"].([]any)[0].(map[string]any)
		row["future_field"] = json.Number("9007199254740993")
		updated, err := json.Marshal(history)
		Expect(err).NotTo(HaveOccurred())
		writeFixture(path, updated, 0600)
		finish := budgetCommand(root, "finalize")
		completion := budgetCompletion("completed")
		completion["ProcessPID"] = 1234
		finish["Completion"] = completion
		out := session.call(finish)
		Expect(out).NotTo(HaveKey("error"))
		read := session.call(budgetCommand(root, "read"))
		fresh := read["history"].(map[string]any)
		Expect(fresh["top_extension"]).To(Equal("preserved"))
		Expect(fresh["runs"].([]any)[0].(map[string]any)["future_field"]).To(Equal(json.Number("9007199254740993")))
	})
})

var _ = Describe("G2 budget planning regressions", func() {
	BeforeEach(budgetBuild)
	// per docs/adr/0069-go-budget-ledger-admission.md:54
	It("does not aggregate unrequested cost while planning without a USD threshold", func() {
		root, cwd := budgetFixture()
		a, b := budgetRow("a"), budgetRow("b")
		a["estimated_usd"] = json.Number("1e308")
		b["estimated_usd"] = json.Number("1e308")
		c := budgetCommand(root, "plan")
		c["History"] = budgetHistory(a, b)
		c["Environment"].(map[string]string)["FACTORY_BUDGET_MAX_ATTEMPTS"] = "3"
		baseline := budgetOracle(root, cwd, c)
		Expect(baseline).NotTo(HaveKey("error"))
		Expect(baseline["plan"].(map[string]any)["blockers"]).To(BeEmpty())
		out := budgetInvoke(root, cwd, c)[0]
		Expect(out).NotTo(HaveKey("error"))
		Expect(budgetComparable(out["plan"])).To(Equal(budgetComparable(baseline["plan"])))
	})
	// per docs/adr/0069-go-budget-ledger-admission.md:25
	DescribeTable("requires explicit null unknown metadata on uncertain active rows", func(missing string) {
		root, cwd := budgetFixture()
		r := budgetRow("uncertain")
		r["status"] = "active"
		r["outcome"] = "timeout"
		r["ended_at"] = nil
		r["process_pid"] = 1234
		r["exit_code"] = nil
		r["complete"] = false
		r["tokens"] = nil
		r["estimated_usd"] = nil
		delete(r, missing)
		c := budgetCommand(root, "parse")
		c["History"] = budgetHistory(r)
		Expect(budgetOracle(root, cwd, c)).To(HaveKey("error"))
		Expect(budgetInvoke(root, cwd, c)[0]).To(HaveKey("error"))
	}, Entry("missing tokens", "tokens"), Entry("missing estimated cost", "estimated_usd"))
})

var _ = Describe("G2 budget canceled report", func() {
	BeforeEach(budgetBuild)
	// per docs/adr/0069-go-budget-ledger-admission.md:159
	It("refuses an already canceled empty-history report", func() {
		root, cwd := budgetFixture()
		c := budgetCommand(root, "report-cancel")
		c["TimeoutMS"] = -1
		out := budgetInvoke(root, cwd, c)[0]
		Expect(out).To(HaveKey("error"))
	})
})

var _ = Describe("G2 budget lifecycle consistency", func() {
	BeforeEach(budgetBuild)
	// per docs/adr/0069-go-budget-ledger-admission.md:108
	It("rejects uncertain completion without any process identity", func() {
		root, cwd := budgetFixture()
		finish := budgetCommand(root, "finalize")
		completion := budgetCompletion("failed")
		completion["OwnershipUnconfirmed"] = true
		finish["Completion"] = completion
		out := budgetInvoke(root, cwd, budgetCommand(root, "admit"), finish, budgetCommand(root, "read"))
		Expect(out[1]).To(HaveKey("error"))
		Expect(out[2]["history"].(map[string]any)["runs"].([]any)[0]).To(Equal(out[0]["record"]))
	})
	// per docs/adr/0069-go-budget-ledger-admission.md:113
	It("completes a confirmed pre-spawn launch failure with unknown accounting", func() {
		root, cwd := budgetFixture()
		finish := budgetCommand(root, "finalize")
		completion := budgetCompletion("launch_error")
		completion["ExitConfirmed"] = false
		completion["ExitCode"] = nil
		finish["Completion"] = completion
		out := budgetInvoke(root, cwd, budgetCommand(root, "admit"), finish)
		Expect(out[1]).NotTo(HaveKey("error"))
		row := out[1]["record"].(map[string]any)
		Expect(row["status"]).To(Equal("completed"))
		Expect(row["tokens"]).To(BeNil())
		Expect(row["estimated_usd"]).To(BeNil())
		Expect(row["complete"]).To(BeFalse())
	})
	// per docs/adr/0069-go-budget-ledger-admission.md:104
	It("refuses finalizing the same completed reservation twice", func() {
		root, cwd := budgetFixture()
		finish := budgetCommand(root, "finalize")
		completion := budgetCompletion("completed")
		completion["ProcessPID"] = 1234
		finish["Completion"] = completion
		out := budgetInvoke(root, cwd, budgetCommand(root, "admit"), finish, finish)
		Expect(out[1]).NotTo(HaveKey("error"))
		Expect(out[2]).To(HaveKey("error"))
	})
	// per docs/adr/0069-go-budget-ledger-admission.md:136
	It("reads an actual Python Ledger.write and preserves its extension fields", func() {
		root, cwd := budgetFixture()
		row := budgetRow("python")
		row["extension"] = json.Number("9007199254740993")
		command := budgetCommand(root, "python-write")
		command["History"] = budgetHistory(row)
		original := budgetOracle(root, cwd, command)
		Expect(original).NotTo(HaveKey("error"))
		out := budgetInvoke(root, cwd, budgetCommand(root, "read"))[0]
		Expect(out).NotTo(HaveKey("error"))
		Expect(out["history"]).To(Equal(original["history"]))
	})
})

var _ = Describe("G2 budget no-process completion evidence", func() {
	BeforeEach(budgetBuild)
	// per docs/adr/0069-go-budget-ledger-admission.md:203
	DescribeTable("rejects exit evidence on a pre-spawn failure without process identity", func(confirmed bool) {
		root, cwd := budgetFixture()
		finish := budgetCommand(root, "finalize")
		completion := budgetCompletion("launch_error")
		completion["ExitConfirmed"] = confirmed
		finish["Completion"] = completion
		out := budgetInvoke(root, cwd, budgetCommand(root, "admit"), finish, budgetCommand(root, "read"))
		Expect(out[1]).To(HaveKey("error"))
		Expect(out[2]["history"].(map[string]any)["runs"].([]any)[0]).To(Equal(out[0]["record"]))
	}, Entry("confirmed exit without process", true), Entry("unconfirmed code without process", false))
})
