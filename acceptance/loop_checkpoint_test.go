package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const checkpointRecord = `{"session":"s","task":"t","harness":"codex","mode":"manual","status":"stopped","attempts":1,"no_progress":0,"elapsed_seconds":2.0,"reserved_seconds":10.0,"policy":"policy","prompt":"prompt","started_at":"time","phase":"finished","outcome":"manual_passed","stop_reason":"done","next_action":"inspect","snapshot":{"head":"h","source":"s","safety":"f"},"baseline":{"head":"h","source":"s","safety":"f"},"owner_pid":123,"process_pid":null,"uncertain":false,"evidence":[],"budget_runs":[],"extension":{"preserve":true}}`

func checkpointHistory(record string) string {
	return `{"schema":1,"runs":[` + record + `],"extension":"keep"}`
}
func checkpointCLI(root, cwd, checkout string, args ...string) cliResult {
	return loopProcess(cwd, filepath.Join(root, "factory"), append([]string{"loop", "checkpoint"}, args...), []string{"FACTORY_LOOP_ROOT=" + checkout, "FACTORY_LOOP_ENABLED=INVALID", "FACTORY_BUDGET_ENABLED=INVALID"})
}
func checkpointOracle(root, cwd, checkout, operation string) cliResult {
	program := `import json,pathlib,sys
sys.path.insert(0,sys.argv[1])
import loop
store=loop.Store(pathlib.Path(sys.argv[2]))
history=store.read()
if sys.argv[3]=='status': history=next(row for row in history['runs'] if row['session']=='s' and row['task']=='t')
print(json.dumps(history,sort_keys=True,ensure_ascii=True,allow_nan=False))`
	return loopProcess(cwd, "python3", []string{"-B", "-c", program, filepath.Join(root, "oracle"), checkout, operation}, nil)
}

var _ = Describe("G2 loop checkpoint core", func() {
	// per docs/adr/0074-go-loop-checkpoint-storage.md:21
	It("reads absent history without creating storage or parsing configuration", func() {
		root, cwd, checkout := loopFixture()
		oracle := checkpointOracle(root, cwd, checkout, "read")
		Expect(oracle.status).To(BeZero())
		actual := checkpointCLI(root, cwd, checkout, "read")
		Expect(actual.status).To(BeZero(), "%+v", actual)
		Expect(actual.stderr).To(BeEmpty())
		Expect(budgetDecode([]byte(actual.stdout))).To(Equal(budgetDecode([]byte(oracle.stdout))))
		Expect(budgetDecode([]byte(actual.stdout))).To(Equal(budgetDecode([]byte(`{"schema":1,"runs":[]}`))))
		_, err := os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0074-go-loop-checkpoint-storage.md:113
	It("selects a validated record and preserves unknown data without writes", func() {
		root, cwd, checkout := loopFixture()
		path := filepath.Join(checkout, ".factory/loops.json")
		original := []byte(checkpointHistory(checkpointRecord))
		writeFixture(path, original, 0600)
		oracle := checkpointOracle(root, cwd, checkout, "status")
		Expect(oracle.status).To(BeZero())
		actual := checkpointCLI(root, cwd, checkout, "status", "s", "t")
		Expect(actual.status).To(BeZero(), "%+v", actual)
		Expect(actual.stderr).To(BeEmpty())
		Expect(budgetDecode([]byte(actual.stdout))).To(Equal(budgetDecode([]byte(oracle.stdout))))
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(original))
		_, err = os.Stat(filepath.Join(checkout, ".factory/loops.lock"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0074-go-loop-checkpoint-storage.md:114
	It("durably roundtrips the existing validated history", func() {
		root, cwd, checkout := loopFixture()
		path := filepath.Join(checkout, ".factory/loops.json")
		writeFixture(path, []byte(checkpointHistory(checkpointRecord)), 0600)
		oracle := checkpointOracle(root, cwd, checkout, "read")
		Expect(oracle.status).To(BeZero())
		actual := checkpointCLI(root, cwd, checkout, "roundtrip")
		Expect(actual.status).To(BeZero(), "%+v", actual)
		Expect(actual.stderr).To(BeEmpty())
		Expect(budgetDecode([]byte(actual.stdout))).To(Equal(budgetDecode([]byte(oracle.stdout))))
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(budgetDecode(after)).To(Equal(budgetDecode([]byte(oracle.stdout))))
		lock, err := os.Stat(filepath.Join(checkout, ".factory/loops.lock"))
		Expect(err).NotTo(HaveOccurred())
		Expect(lock.Mode().Perm()).To(Equal(os.FileMode(0600)))
	})
})

func checkpointWith(field, raw string) string {
	GinkgoHelper()
	var fields map[string]json.RawMessage
	Expect(json.Unmarshal([]byte(checkpointRecord), &fields)).To(Succeed())
	if raw == "MISSING" {
		delete(fields, field)
	} else {
		fields[field] = json.RawMessage(raw)
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var parts []string
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%q:%s", key, fields[key]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}
func checkpointInput(root, cwd, checkout, input string, args ...string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(root, "factory"), append([]string{"loop", "checkpoint"}, args...)...) // #nosec G204 G702 -- fixed private command on the compiled fixture; request data is stdin only.
	command.Dir = cwd
	command.Stdin = strings.NewReader(input)
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "FACTORY_") {
			command.Env = append(command.Env, item)
		}
	}
	command.Env = append(command.Env, "FACTORY_BRIDGE_PROTOCOL=1", "FACTORY_LOOP_ROOT="+checkout, "FACTORY_LOOP_ENABLED=INVALID", "FACTORY_BUDGET_ENABLED=INVALID", "GORACE=atexit_sleep_ms=0")
	var output, diagnostic bytes.Buffer
	command.Stdout = &output
	command.Stderr = &diagnostic
	err := command.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred())
	status := 0
	if err != nil {
		var exited *exec.ExitError
		Expect(errors.As(err, &exited)).To(BeTrue(), "%v", err)
		status = exited.ExitCode()
	}
	return cliResult{output.String(), diagnostic.String(), status}
}
func checkpointCanonical(input string) string {
	GinkgoHelper()
	command := exec.Command("python3", "-B", "-c", `import json,sys;print(json.dumps(json.load(sys.stdin),sort_keys=True,ensure_ascii=True,allow_nan=False))`)
	command.Stdin = strings.NewReader(input)
	out, err := command.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "canonical oracle: %s", out)
	return string(out)
}
func checkpointFixtureHistory(history string) (string, string, string, string) {
	GinkgoHelper()
	root, cwd, checkout := loopFixture()
	path := filepath.Join(checkout, ".factory/loops.json")
	writeFixture(path, []byte(history), 0600)
	return root, cwd, checkout, path
}

const checkpointResumeRequest = `{"harness":"codex","mode":"manual","snapshot":{"head":"h","source":"s","safety":"f"},"policy":"policy","prompt":"prompt","timeout_seconds":10.0}`

var _ = Describe("G2 loop checkpoint identity", func() {
	// per docs/adr/0074-go-loop-checkpoint-storage.md:24
	DescribeTable("preserves valid baseline record identity through read and publication", func(field, raw string) {
		history := checkpointHistory(checkpointWith(field, raw))
		root, cwd, checkout, path := checkpointFixtureHistory(history)
		oracle := checkpointOracle(root, cwd, checkout, "read")
		Expect(oracle.status).To(BeZero(), "%+v", oracle)
		read := checkpointCLI(root, cwd, checkout, "read")
		Expect(read.status).To(BeZero(), "%+v", read)
		Expect(checkpointCanonical(read.stdout)).To(Equal(checkpointCanonical(oracle.stdout)))
		round := checkpointCLI(root, cwd, checkout, "roundtrip")
		Expect(round.status).To(BeZero(), "%+v", round)
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(checkpointCanonical(string(after))).To(Equal(checkpointCanonical(oracle.stdout)))
	}, Entry("Claude record", "harness", `"claude"`), Entry("OpenCode record", "harness", `"opencode"`), Entry("active record remains inspectable", "status", `"active"`), Entry("counter exact integer", "attempts", "9007199254740993"), Entry("large owner without float limit", "owner_pid", "1"+strings.Repeat("0", 400)), Entry("absent process PID", "process_pid", "MISSING"), Entry("large process PID", "process_pid", "1"+strings.Repeat("0", 400)), Entry("arbitrary evidence", "evidence", `[null,true,2.0,{"x":"\ud800"}]`), Entry("budget references are not invented schema", "budget_runs", `[false,[],{"unknown":123}]`), Entry("unknown huge numeric identity", "extension", `{"large":`+"1"+strings.Repeat("0", 400)+`,"float":1.0,"surrogate":"\udcff","replacement":"�"}`), Entry("surrogate known string", "stop_reason", `"\ud800"`), Entry("snapshot strings are not hash syntax", "snapshot", `{"head":"","source":"a","safety":"\udfff"}`), Entry("negative zero float time", "elapsed_seconds", "-0.0"))
	// per docs/adr/0074-go-loop-checkpoint-storage.md:27
	It("keeps last duplicate keys and array order", func() {
		history := `{"schema":99,"schema":1,"runs":[` + checkpointRecord + `],"unknown":[3,1,2],"value":"old","value":"last"}`
		root, cwd, checkout, path := checkpointFixtureHistory(history)
		oracle := checkpointOracle(root, cwd, checkout, "read")
		Expect(oracle.status).To(BeZero())
		actual := checkpointCLI(root, cwd, checkout, "roundtrip")
		Expect(actual.status).To(BeZero(), "%+v", actual)
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(checkpointCanonical(string(after))).To(Equal(checkpointCanonical(oracle.stdout)))
	})
	// per docs/adr/0074-go-loop-checkpoint-storage.md:23
	It("validates every record before selecting the requested status", func() {
		bad := checkpointWith("task", `"other"`)
		bad = strings.Replace(bad, `"uncertain":false`, `"uncertain":0`, 1)
		root, cwd, checkout, path := checkpointFixtureHistory(`{"schema":1,"runs":[` + checkpointRecord + `,` + bad + `]}`)
		before, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		loopRefused(checkpointCLI(root, cwd, checkout, "status", "s", "t"))
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
	})
	// per docs/adr/0074-go-loop-checkpoint-storage.md:22
	It("reads permissive existing modes without chmod or lock creation", func() {
		root, cwd, checkout, path := checkpointFixtureHistory(checkpointHistory(checkpointRecord))
		Expect(os.Chmod(filepath.Dir(path), 0755)).To(Succeed())
		Expect(os.Chmod(path, 0644)).To(Succeed())
		out := checkpointCLI(root, cwd, checkout, "read")
		Expect(out.status).To(BeZero())
		for name, mode := range map[string]os.FileMode{filepath.Dir(path): 0755, path: 0644} {
			info, err := os.Stat(name)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(mode))
		}
		_, err := os.Stat(filepath.Join(checkout, ".factory/loops.lock"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

var _ = Describe("G2 loop checkpoint invalid schema", func() {
	// per docs/adr/0074-go-loop-checkpoint-storage.md:29
	DescribeTable("refuses malformed records without rewriting bytes", func(field, raw string) {
		history := checkpointHistory(checkpointWith(field, raw))
		root, cwd, checkout, path := checkpointFixtureHistory(history)
		Expect(checkpointOracle(root, cwd, checkout, "read").status).NotTo(BeZero())
		loopRefused(checkpointCLI(root, cwd, checkout, "roundtrip"))
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(after)).To(Equal(history))
	},
		Entry("invalid identity", "session", `"../PRIVATE"`), Entry("unknown harness", "harness", `"PRIVATE"`), Entry("unknown mode", "mode", `"other"`), Entry("unknown status", "status", `"other"`), Entry("missing string", "phase", "MISSING"), Entry("nonstrings", "policy", "1"), Entry("boolean counter", "attempts", "true"), Entry("float counter", "no_progress", "0.0"), Entry("negative counter", "attempts", "-1"), Entry("counter overflows finite conversion", "attempts", "1"+strings.Repeat("0", 309)), Entry("boolean elapsed", "elapsed_seconds", "false"), Entry("negative time", "reserved_seconds", "-1"), Entry("missing snapshot key", "snapshot", `{"head":"h","source":"s"}`), Entry("extra snapshot key", "baseline", `{"head":"h","source":"s","safety":"f","extra":"x"}`), Entry("zero owner", "owner_pid", "0"), Entry("float owner", "owner_pid", "1.0"), Entry("bool process", "process_pid", "true"), Entry("nonboolean uncertainty", "uncertain", "0"), Entry("nonarray evidence", "evidence", "{}"), Entry("nonarray references", "budget_runs", "null"))
	// per docs/adr/0074-go-loop-checkpoint-storage.md:21
	DescribeTable("refuses malformed complete histories", func(history string) {
		root, cwd, checkout, path := checkpointFixtureHistory(history)
		Expect(checkpointOracle(root, cwd, checkout, "read").status).NotTo(BeZero())
		loopRefused(checkpointCLI(root, cwd, checkout, "read"))
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(after)).To(Equal(history))
	}, Entry("wrong schema", `{"schema":2,"runs":[]}`), Entry("boolean schema", `{"schema":true,"runs":[]}`), Entry("missing runs", `{"schema":1}`), Entry("duplicate identity", `{"schema":1,"runs":[`+checkpointRecord+`,`+checkpointRecord+`]}`), Entry("broken JSON", `{"schema":`))
	// per docs/adr/0074-go-loop-checkpoint-storage.md:40
	DescribeTable("refuses private JSON qualification violations before rewrite", func(history string) {
		root, cwd, checkout, path := checkpointFixtureHistory(history)
		loopRefused(checkpointCLI(root, cwd, checkout, "roundtrip"))
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(after)).To(Equal(history))
	}, Entry("nonfinite unknown", `{"schema":1,"runs":[],"unknown":NaN}`), Entry("integer beyond4300digits", `{"schema":1,"runs":[],"unknown":`+strings.Repeat("1", 4301)+`}`), Entry("infinite exponent unknown", `{"schema":1,"runs":[],"unknown":1e999}`), Entry("invalid UTF8", `{"schema":1,"runs":[],"unknown":"`+string([]byte{255})+`"}`), Entry("depth beyond512", `{"schema":1,"runs":[],"unknown":`+strings.Repeat("[", 513)+"0"+strings.Repeat("]", 513)+"}"))
})

var _ = Describe("G2 loop checkpoint resume", func() {
	// per docs/adr/0074-go-loop-checkpoint-storage.md:96
	DescribeTable("assesses eligible manual records without changing state", func(record, input string, remaining float64) {
		root, cwd, checkout, path := checkpointFixtureHistory(checkpointHistory(record))
		before, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		out := checkpointInput(root, cwd, checkout, input, "resume-check", "s", "t")
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stderr).To(BeEmpty())
		value := budgetDecode([]byte(out.stdout)).(map[string]any)
		Expect(value).To(HaveLen(3))
		Expect(value["eligible"]).To(BeTrue())
		number, ok := value["remaining_seconds"].(json.Number)
		Expect(ok).To(BeTrue())
		seconds, err := number.Float64()
		Expect(err).NotTo(HaveOccurred())
		Expect(seconds).To(Equal(remaining))
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		Expect(checkpointCanonical(string(mustCheckpointJSON(value["record"])))).To(Equal(checkpointCanonical(record)))
		_, err = os.Stat(filepath.Join(checkout, ".factory/budget.json"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		_, err = os.Stat(filepath.Join(checkout, ".factory/loops.lock"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("manual passed", checkpointRecord, checkpointResumeRequest, 8.0), Entry("manual failed", checkpointWith("outcome", `"manual_failed"`), checkpointResumeRequest, 8.0), Entry("completed manual is eligible", checkpointWith("status", `"completed"`), checkpointResumeRequest, 8.0), Entry("exact integer comparison before rounded subtraction", checkpointWith("elapsed_seconds", "9007199254740995"), strings.Replace(checkpointResumeRequest, "10.0", "9007199254740996.0", 1), 0.0))
	// per docs/adr/0074-go-loop-checkpoint-storage.md:96
	DescribeTable("refuses every stale or unsafe eligibility predicate", func(record, input string) {
		root, cwd, checkout, path := checkpointFixtureHistory(checkpointHistory(record))
		before, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		loopRefused(checkpointInput(root, cwd, checkout, input, "resume-check", "s", "t"))
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
	}, Entry("active loop", checkpointWith("status", `"active"`), checkpointResumeRequest), Entry("uncertainty", checkpointWith("uncertain", "true"), checkpointResumeRequest), Entry("harness changed", checkpointRecord, strings.Replace(checkpointResumeRequest, "codex", "claude", 1)), Entry("mode changed", checkpointRecord, strings.Replace(checkpointResumeRequest, "manual", "bounded", 1)), Entry("snapshot changed", checkpointRecord, strings.Replace(checkpointResumeRequest, `"source":"s"`, `"source":"changed"`, 1)), Entry("policy changed", checkpointRecord, strings.Replace(checkpointResumeRequest, `"policy":"policy"`, `"policy":"new"`, 1)), Entry("prompt changed", checkpointRecord, strings.Replace(checkpointResumeRequest, `"prompt":"prompt"`, `"prompt":"new"`, 1)), Entry("terminal bounded", checkpointWith("mode", `"bounded"`), strings.Replace(checkpointResumeRequest, "manual", "bounded", 1)), Entry("terminal other outcome", checkpointWith("outcome", `"manual_timeout"`), checkpointResumeRequest), Entry("elapsed equal limit", checkpointWith("elapsed_seconds", "10"), checkpointResumeRequest), Entry("elapsed beyond limit", checkpointWith("elapsed_seconds", "11"), checkpointResumeRequest), Entry("nonpositive timeout", checkpointRecord, strings.Replace(checkpointResumeRequest, "10.0", "0", 1)), Entry("missing requestfield", checkpointRecord, `{}`), Entry("unknown request field", checkpointRecord, strings.TrimSuffix(checkpointResumeRequest, "}")+`,"PRIVATE_EXTRA":true}`), Entry("boolean timeout", checkpointRecord, strings.Replace(checkpointResumeRequest, "10.0", "true", 1)))
})

func mustCheckpointJSON(value any) []byte {
	GinkgoHelper()
	data, err := json.Marshal(value)
	Expect(err).NotTo(HaveOccurred())
	return data
}

var _ = Describe("G2 loop checkpoint path safety", func() {
	// per docs/adr/0074-go-loop-checkpoint-storage.md:58
	DescribeTable("refuses unsafe storage without touching its target", func(kind string) {
		root, cwd, checkout := loopFixture()
		directory := filepath.Join(checkout, ".factory")
		Expect(os.Mkdir(directory, 0700)).To(Succeed())
		target := filepath.Join(cwd, "PRIVATE_TARGET")
		writeFixture(target, []byte("PRIVATE_PRESERVE"), 0600)
		name := filepath.Join(directory, "loops.json")
		switch kind {
		case "symlink":
			Expect(os.Symlink(target, name)).To(Succeed())
		case "hardlink":
			Expect(os.Link(target, name)).To(Succeed())
		case "fifo":
			Expect(syscall.Mkfifo(name, 0600)).To(Succeed())
		case "directory":
			Expect(os.Mkdir(name, 0700)).To(Succeed())
		case "lock symlink":
			writeFixture(name, []byte(checkpointHistory(checkpointRecord)), 0600)
			Expect(os.Symlink(target, filepath.Join(directory, "loops.lock"))).To(Succeed())
		}
		loopRefused(checkpointCLI(root, cwd, checkout, "roundtrip"))
		after, err := os.ReadFile(target)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(after)).To(Equal("PRIVATE_PRESERVE"))
	}, Entry("history symlink", "symlink"), Entry("history hardlink", "hardlink"), Entry("history FIFO", "fifo"), Entry("history directory", "directory"), Entry("lock symlink", "lock symlink"))
	// per docs/adr/0074-go-loop-checkpoint-storage.md:124
	DescribeTable("refuses private protocol misuse before storage", func(args []string) {
		root, cwd, checkout := loopFixture()
		loopRefused(checkpointCLI(root, cwd, checkout, args...))
		_, err := os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("unknown", []string{"PRIVATE_COMMAND"}), Entry("extra", []string{"read", "PRIVATE_EXTRA"}), Entry("help", []string{"read", "--help"}), Entry("completion", []string{"__complete"}), Entry("invalid ID", []string{"status", "../PRIVATE", "t"}), Entry("missing identity", []string{"status", "s"}))
})

var _ = Describe("G2 loop checkpoint capacity", func() {
	// per docs/adr/0074-go-loop-checkpoint-storage.md:42
	DescribeTable("bounds actual history input at32MiB", func(excess int) {
		base := `{"schema":1,"runs":[]}`
		history := base + strings.Repeat(" ", 32*1024*1024-len(base)+excess)
		root, cwd, checkout, path := checkpointFixtureHistory(history)
		out := checkpointCLI(root, cwd, checkout, "read")
		if excess == 0 {
			Expect(out.status).To(BeZero(), "%+v", out)
			Expect(checkpointCanonical(out.stdout)).To(Equal(checkpointCanonical(base)))
		} else {
			loopRefused(out)
		}
		info, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Size()).To(Equal(int64(len(history))))
	}, Entry("exact", 0), Entry("one byte over", 1))
	// per docs/adr/0074-go-loop-checkpoint-storage.md:81
	It("rejects oversized canonical output without replacing history or leaving tempfiles", func() {
		prefix := `{"schema":1,"runs":[],"x":"`
		suffix := `"}`
		history := prefix + strings.Repeat("x", 32*1024*1024-len(prefix)-len(suffix)) + suffix
		root, cwd, checkout, path := checkpointFixtureHistory(history)
		loopRefused(checkpointCLI(root, cwd, checkout, "roundtrip"))
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(after)).To(Equal(history))
		entries, err := os.ReadDir(filepath.Dir(path))
		Expect(err).NotTo(HaveOccurred())
		for _, entry := range entries {
			Expect(entry.Name()).To(BeElementOf("loops.json", "loops.lock"))
		}
	})
})

var _ = Describe("G2 loop checkpoint global resume blockers", func() {
	// per docs/adr/0074-go-loop-checkpoint-storage.md:96
	DescribeTable("inspects the whole checkout without modifying either history", func(kind string) {
		history := checkpointHistory(checkpointRecord)
		if kind == "other active" || kind == "other uncertain" {
			other := checkpointWith("task", `"other"`)
			if kind == "other active" {
				other = strings.Replace(other, `"status":"stopped"`, `"status":"active"`, 1)
			} else {
				other = strings.Replace(other, `"uncertain":false`, `"uncertain":true`, 1)
			}
			history = `{"schema":1,"runs":[` + checkpointRecord + `,` + other + `]}`
		}
		if kind == "missing selected" {
			history = `{"schema":1,"runs":[]}`
		}
		root, cwd, checkout, path := checkpointFixtureHistory(history)
		budgetPath := filepath.Join(checkout, ".factory/budget.json")
		budgetBytes := []byte{}
		if kind == "active budget" {
			row := budgetRow("other-run")
			row["status"] = "active"
			row["outcome"] = "running"
			row["complete"] = false
			row["tokens"] = nil
			row["estimated_usd"] = nil
			row["exit_code"] = nil
			row["ended_at"] = nil
			budgetBytes = budgetHistory(row)
			writeFixture(budgetPath, budgetBytes, 0600)
		}
		if kind == "corrupt budget" {
			budgetBytes = []byte("PRIVATE_BROKEN_HISTORY")
			writeFixture(budgetPath, budgetBytes, 0600)
		}
		loopRefused(checkpointInput(root, cwd, checkout, checkpointResumeRequest, "resume-check", "s", "t"))
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(after)).To(Equal(history))
		if len(budgetBytes) > 0 {
			after, err = os.ReadFile(budgetPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(after).To(Equal(budgetBytes))
		}
		for _, name := range []string{"budget.lock", "loops.lock"} {
			_, err = os.Stat(filepath.Join(checkout, ".factory", name))
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	}, Entry("another active checkpoint", "other active"), Entry("another uncertain checkpoint", "other uncertain"), Entry("unrelated active budget run", "active budget"), Entry("missing selected identity", "missing selected"), Entry("corrupt shared budget", "corrupt budget"))
	// per docs/adr/0074-go-loop-checkpoint-storage.md:123
	DescribeTable("resolves default and relative roots from caller cwd", func(relative bool) {
		root, cwd, checkout, _ := checkpointFixtureHistory(checkpointHistory(checkpointRecord))
		var out cliResult
		if relative {
			out = checkpointCLI(root, cwd, "checkout", "read")
		} else {
			out = loopProcess(checkout, filepath.Join(root, "factory"), []string{"loop", "checkpoint", "read"}, nil)
		}
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(checkpointCanonical(out.stdout)).To(Equal(checkpointCanonical(checkpointHistory(checkpointRecord))))
	}, Entry("caller cwd", false), Entry("relative root", true))
	// per docs/adr/0074-go-loop-checkpoint-storage.md:81
	It("publishes exactly32MiB of canonical output including its final newline", func() {
		base := `{"schema":1,"runs":[],"x":""}`
		padding := 32*1024*1024 - len(checkpointCanonical(base))
		history := strings.Replace(base, `"x":""`, `"x":"`+strings.Repeat("x", padding)+`"`, 1)
		root, cwd, checkout, path := checkpointFixtureHistory(history)
		actual := checkpointCLI(root, cwd, checkout, "roundtrip")
		Expect(actual.status).To(BeZero(), "%s", actual.stderr)
		info, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Size()).To(Equal(int64(32 * 1024 * 1024)))
		Expect(len(actual.stdout)).To(BeNumerically("<=", 32*1024*1024))
		persisted, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(checkpointCanonical(actual.stdout)).To(Equal(string(persisted)))
	})
})

var _ = Describe("G2 loop checkpoint blocked input cancellation", func() {
	// per docs/adr/0074-go-loop-checkpoint-storage.md:129
	DescribeTable("interrupts its owned stdin after consuming an incomplete request", func(signal syscall.Signal) {
		root, cwd, checkout, path := checkpointFixtureHistory(checkpointHistory(checkpointRecord))
		before, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		input, writer, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(input.Close()).To(Succeed()) })
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		command := exec.CommandContext(ctx, filepath.Join(root, "factory"), "loop", "checkpoint", "resume-check", "s", "t") // #nosec G204 G702 -- fixed compiled fixture protocol, incomplete data uses an owned pipe.
		command.Dir = cwd
		command.Stdin = input
		command.Env = append(os.Environ(), "FACTORY_BRIDGE_PROTOCOL=1", "FACTORY_LOOP_ROOT="+checkout, "GORACE=atexit_sleep_ms=0")
		var output, diagnostic bytes.Buffer
		command.Stdout = &output
		command.Stderr = &diagnostic
		Expect(command.Start()).To(Succeed())
		done := make(chan error, 1)
		written := make(chan error, 1)
		go func() { defer close(done); done <- command.Wait() }()
		DeferCleanup(func() {
			Expect(writer.Close()).To(Succeed())
			cancel()
			Eventually(done, 3*time.Second).Should(BeClosed())
			Eventually(written, 3*time.Second).Should(BeClosed())
		})
		go func() { _, err := writer.Write(bytes.Repeat([]byte(" "), 4*1024*1024)); written <- err; close(written) }()
		var writeErr error
		Eventually(written, 3*time.Second).Should(Receive(&writeErr))
		Expect(writeErr).NotTo(HaveOccurred())
		Expect(command.Process.Signal(signal)).To(Succeed())
		var waitErr error
		Eventually(done, 3*time.Second).Should(Receive(&waitErr))
		var exited *exec.ExitError
		Expect(errors.As(waitErr, &exited)).To(BeTrue())
		loopRefused(cliResult{output.String(), diagnostic.String(), exited.ExitCode()})
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		_, err = os.Stat(filepath.Join(checkout, ".factory/loops.lock"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("SIGTERM", syscall.SIGTERM), Entry("SIGINT", syscall.SIGINT))
})
