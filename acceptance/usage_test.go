package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const usageBaseline = "49636db85bfb58eeafa761fb99000a12420bcb43"
const codexAccounting = `{"type":"turn.completed","usage":{"input_tokens":11,"cached_input_tokens":2,"output_tokens":3}}`
const claudeAccounting = `{"type":"result","subtype":"success","usage":{"input_tokens":11,"output_tokens":3,"cache_creation_input_tokens":5,"cache_read_input_tokens":2},"total_cost_usd":0.25}`
const openAccounting = `{"type":"step_finish","part":{"sessionID":"s","messageID":"m","id":"i","reason":"stop","cost":0.25,"tokens":{"input":11,"output":3,"reasoning":7,"cache":{"read":2,"write":5}}}}`

var usageSources = map[string]string{"codex": "codex.turn.completed.usage; cost unavailable", "claude": "claude.result.usage (main agent); total_cost_usd (client estimate)", "opencode": "opencode.step_finish.part (deduplicated incremental client estimate)"}
var usageEvents = map[string]string{"codex": codexAccounting, "claude": claudeAccounting, "opencode": openAccounting}
var usageTokens = map[string]string{"codex": `{"input_tokens":11,"cached_input_tokens":2,"output_tokens":3}`, "claude": `{"input_tokens":11,"output_tokens":3,"cache_creation_input_tokens":5,"cache_read_input_tokens":2}`, "opencode": `{"input_tokens":11,"output_tokens":3,"reasoning_output_tokens":7,"cache_read_input_tokens":2,"cache_creation_input_tokens":5}`}

func usageProcess(cwd, input, program string, args ...string) cliResult {
	GinkgoHelper()
	return usageRun(cwd, strings.NewReader(input), program, args...)
}

func usageRun(cwd string, input io.Reader, program string, args ...string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, args...) // #nosec G204 G702 -- executable and argv are acceptance-owned; untrusted event data travels only on stdin.
	cmd.Dir = cwd
	cmd.Stdin = input
	cmd.Env = []string{"FACTORY_BRIDGE_PROTOCOL=1", "PATH=/absent-native-tools", "LC_ALL=C", "FACTORY_CONFIG=" + filepath.Join(cwd, "must-not-read-config"), "OPENROUTER_API_KEY=must-not-emit-credential"}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred())
	status := 0
	if err != nil {
		var exit *exec.ExitError
		Expect(errors.As(err, &exit)).To(BeTrue(), "process launch failed: %v", err)
		status = exit.ExitCode()
	}
	return cliResult{stdout.String(), stderr.String(), status}
}

func usageExpected(harness, tokens, cost string, complete, failed bool) string {
	GinkgoHelper()
	result := map[string]any{"tokens": json.RawMessage(tokens), "estimated_usd": json.RawMessage(cost), "complete": complete, "failed": failed, "source": usageSources[harness]}
	data, err := json.Marshal(result)
	Expect(err).NotTo(HaveOccurred())
	return string(data)
}

func usageMetadata(text string) map[string]any {
	GinkgoHelper()
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var result map[string]any
	Expect(decoder.Decode(&result)).To(Succeed())
	Expect(result).To(HaveLen(5))
	for _, key := range []string{"tokens", "estimated_usd", "complete", "failed", "source"} {
		Expect(result).To(HaveKey(key))
	}
	var trailing any
	Expect(decoder.Decode(&trailing)).To(Equal(io.EOF))
	if result["estimated_usd"] != nil {
		number, ok := result["estimated_usd"].(json.Number)
		Expect(ok).To(BeTrue())
		rational, ok := new(big.Rat).SetString(number.String())
		Expect(ok).To(BeTrue())
		result["estimated_usd"] = rational.RatString()
	}
	return result
}

func expectUsageParity(root, cwd, harness, input, expected string) {
	GinkgoHelper()
	original := exec.Command("git", "show", usageBaseline+":scripts/lib/budget_adapters.py") // #nosec G204 -- immutable acceptance baseline and fixed source path.
	contents, err := original.Output()
	Expect(err).NotTo(HaveOccurred())
	oracle := filepath.Join(root, "immutable-usage-oracle.py")
	writeFixture(oracle, contents, 0600)
	python, err := exec.LookPath("python3")
	Expect(err).NotTo(HaveOccurred())
	baseline := usageProcess(cwd, input, "/usr/bin/env", "PATH="+os.Getenv("PATH"), "HOME="+os.Getenv("HOME"), python, "-I", "-B", "-c", `import json,runpy,sys; module=runpy.run_path(sys.argv[1]); print(json.dumps(module["normalize"](sys.argv[2],json.load(sys.stdin)),allow_nan=False))`, oracle, harness)
	Expect(baseline.status).To(Equal(0), baseline.stderr)
	Expect(baseline.stderr).To(BeEmpty())
	Expect(usageMetadata(baseline.stdout)).To(Equal(usageMetadata(expected)), "immutable Python must match the explicit expectation")
	actual := usageProcess(cwd, input, filepath.Join(root, "factory"), "usage", "normalize", harness)
	Expect(actual.status).To(Equal(0), actual.stderr)
	Expect(actual.stderr).To(BeEmpty())
	Expect(actual.stdout).To(HaveSuffix("\n"))
	Expect(usageMetadata(actual.stdout)).To(Equal(usageMetadata(expected)))
	entries, err := os.ReadDir(cwd)
	Expect(err).NotTo(HaveOccurred())
	Expect(entries).To(BeEmpty(), "normalization must not create local state")
}

var _ = Describe("G2 native usage normalization", func() {
	// per docs/adr/0064-go-native-usage-accounting.md:40
	DescribeTable("normalizes explicit successful accounting for each harness", func(harness string) {
		root, cwd := fixture()
		cost := "0.25"
		if harness == "codex" {
			cost = "null"
		}
		expectUsageParity(root, cwd, harness, "["+usageEvents[harness]+"]", usageExpected(harness, usageTokens[harness], cost, true, false))
	}, Entry("Codex", "codex"), Entry("Claude", "claude"), Entry("OpenCode", "opencode"))

	// per docs/adr/0064-go-native-usage-accounting.md:40
	DescribeTable("retains unknown accounting for an empty stream", func(harness string) {
		root, cwd := fixture()
		expectUsageParity(root, cwd, harness, "[]", usageExpected(harness, "null", "null", false, false))
	}, Entry("Codex", "codex"), Entry("Claude", "claude"), Entry("OpenCode", "opencode"))
	// per docs/adr/0064-go-native-usage-accounting.md:40
	DescribeTable("keeps valid tokens but clears completeness and cost on malformed members", func(harness string) {
		root, cwd := fixture()
		tokens := usageTokens[harness]
		if harness == "opencode" {
			tokens = "null"
		}
		expectUsageParity(root, cwd, harness, "[null,3,false,[],\"private transcript\","+usageEvents[harness]+"]", usageExpected(harness, tokens, "null", false, false))
	}, Entry("Codex", "codex"), Entry("Claude", "claude"), Entry("OpenCode", "opencode"))

	// per docs/adr/0064-go-native-usage-accounting.md:48
	DescribeTable("uses the final result instead of summing cumulative native reports", func(harness string) {
		root, cwd := fixture()
		last := strings.Replace(usageEvents[harness], ":11", ":29", 1)
		tokens := strings.Replace(usageTokens[harness], ":11", ":29", 1)
		cost := "null"
		if harness == "claude" {
			cost = "0.25"
		}
		expectUsageParity(root, cwd, harness, "["+usageEvents[harness]+","+last+"]", usageExpected(harness, tokens, cost, true, false))
	}, Entry("Codex", "codex"), Entry("Claude", "claude"))

	// per docs/adr/0064-go-native-usage-accounting.md:48
	DescribeTable("keeps failure and accounting completeness distinct", func(harness, event, tokens, cost string, complete bool) {
		root, cwd := fixture()
		expectUsageParity(root, cwd, harness, event, usageExpected(harness, tokens, cost, complete, true))
	}, Entry("Codex failed turn", "codex", "["+codexAccounting+`,{"type":"turn.failed","error":"private-provider-error"}]`, usageTokens["codex"], "null", false),
		Entry("Codex error event", "codex", `[{"type":"error"},`+codexAccounting+`]`, usageTokens["codex"], "null", false),
		Entry("Claude reported failure can have complete accounting", "claude", "["+strings.Replace(claudeAccounting, `"success"`, `"error_max_turns"`, 1)+"]", usageTokens["claude"], "0.25", true),
		Entry("Claude is_error true", "claude", "["+strings.Replace(claudeAccounting, `"subtype"`, `"is_error":true,"subtype"`, 1)+"]", usageTokens["claude"], "0.25", true),
		Entry("Claude separate error", "claude", `[{"type":"error"},`+claudeAccounting+`]`, usageTokens["claude"], "0.25", true),
		Entry("Claude execution crash hides suspect cost", "claude", "["+strings.Replace(claudeAccounting, `"success"`, `"error_during_execution"`, 1)+"]", usageTokens["claude"], "null", false),
		Entry("OpenCode session failure", "opencode", "["+openAccounting+`,{"type":"session.error","message":"private-provider-error"}]`, "null", "null", false))

	// per docs/adr/0064-go-native-usage-accounting.md:43
	DescribeTable("refuses malformed required token numbers without promoting unknown usage to zero", func(number string) {
		root, cwd := fixture()
		for _, harness := range []string{"codex", "claude", "opencode"} {
			event := strings.Replace(usageEvents[harness], ":11", ":"+number, 1)
			cost := "null"
			if harness == "claude" {
				cost = "0.25"
			}
			expectUsageParity(root, cwd, harness, "["+event+"]", usageExpected(harness, "null", cost, false, false))
		}
	}, Entry("boolean", "true"), Entry("string", `"11"`), Entry("null", "null"), Entry("negative", "-1"), Entry("fraction", "1.5"), Entry("integral decimal encoding", "1.0"), Entry("integral exponent encoding", "1e0"), Entry("nonfinite exponent", "1e309"), Entry("integer finite conversion overflow", "1"+strings.Repeat("0", 309)), Entry("object", "{}"), Entry("array", "[]"))

	// per docs/adr/0064-go-native-usage-accounting.md:43
	DescribeTable("retains exact large integer tokens and signed zero semantics", func(number, expected string) {
		root, cwd := fixture()
		for _, harness := range []string{"codex", "claude", "opencode"} {
			event := strings.Replace(usageEvents[harness], ":11", ":"+number, 1)
			tokens := strings.Replace(usageTokens[harness], ":11", ":"+expected, 1)
			cost := "0.25"
			if harness == "codex" {
				cost = "null"
			}
			expectUsageParity(root, cwd, harness, "["+event+"]", usageExpected(harness, tokens, cost, true, false))
		}
	}, Entry("zero", "0", "0"), Entry("negative zero", "-0", "0"), Entry("beyond float precision", "9007199254740993", "9007199254740993"), Entry("large finite integer", "1"+strings.Repeat("0", 308), "1"+strings.Repeat("0", 308)))

	// per docs/adr/0064-go-native-usage-accounting.md:46
	DescribeTable("retains Claude finite cost values without accepting booleans or negative costs", func(number, expected string, complete bool) {
		root, cwd := fixture()
		event := strings.Replace(claudeAccounting, ":0.25", ":"+number, 1)
		expectUsageParity(root, cwd, "claude", "["+event+"]", usageExpected("claude", usageTokens["claude"], expected, complete, false))
	}, Entry("zero", "0", "0", true), Entry("negative zero", "-0.0", "0", true), Entry("integral float", "1e0", "1", true), Entry("positive underflow", "1e-999", "0", true), Entry("negative underflow", "-1e-999", "0", true), Entry("large exact integer", "9007199254740993", "9007199254740993", true), Entry("boolean", "true", "null", false), Entry("string", `"0.25"`, "null", false), Entry("negative", "-0.1", "null", false), Entry("overflowing float", "1e309", "null", false), Entry("overflowing integer", "1"+strings.Repeat("0", 309), "null", false), Entry("missing cost", "null", "null", false))

	// per docs/adr/0064-go-native-usage-accounting.md:48
	It("retains optional Codex reasoning tokens and rejects malformed optional counts", func() {
		root, cwd := fixture()
		valid := strings.Replace(codexAccounting, `"input_tokens":11`, `"reasoning_output_tokens":7,"input_tokens":11`, 1)
		expected := strings.Replace(usageTokens["codex"], `"input_tokens":11`, `"reasoning_output_tokens":7,"input_tokens":11`, 1)
		expectUsageParity(root, cwd, "codex", "["+valid+"]", usageExpected("codex", expected, "null", true, false))
		invalid := strings.Replace(valid, `"reasoning_output_tokens":7`, `"reasoning_output_tokens":1.0`, 1)
		expectUsageParity(root, cwd, "codex", "["+invalid+"]", usageExpected("codex", "null", "null", false, false))
	})

	// per docs/adr/0064-go-native-usage-accounting.md:56
	DescribeTable("deduplicates OpenCode step identities and preserves Python signature equality", func(events, tokens, cost string, complete bool) {
		root, cwd := fixture()
		expectUsageParity(root, cwd, "opencode", events, usageExpected("opencode", tokens, cost, complete, false))
	}, Entry("same event twice", "["+openAccounting+","+openAccounting+"]", usageTokens["opencode"], "0.25", true),
		Entry("unknown fields ignored", "["+openAccounting+","+strings.Replace(openAccounting, `"cost"`, `"unknown":"private transcript","cost"`, 1)+"]", usageTokens["opencode"], "0.25", true),
		Entry("two unique steps", "["+openAccounting+","+strings.Replace(openAccounting, `"id":"i"`, `"id":"j"`, 1)+"]", `{"input_tokens":22,"output_tokens":6,"reasoning_output_tokens":14,"cache_read_input_tokens":4,"cache_creation_input_tokens":10}`, "0.5", true),
		Entry("conflicting costs", "["+openAccounting+","+strings.Replace(openAccounting, ":0.25", ":0.5", 1)+"]", "null", "null", false),
		Entry("conflicting tokens", "["+openAccounting+","+strings.Replace(openAccounting, ":11", ":12", 1)+"]", "null", "null", false),
		Entry("valid cost then equal boolean duplicate", "["+strings.Replace(openAccounting, ":0.25", ":1", 1)+","+strings.Replace(openAccounting, ":0.25", ":true", 1)+"]", usageTokens["opencode"], "1", true),
		Entry("numeric equal duplicate", "["+strings.Replace(openAccounting, ":0.25", ":1", 1)+","+strings.Replace(openAccounting, ":0.25", ":1.0", 1)+"]", usageTokens["opencode"], "1", true),
		Entry("boolean first remains invalid", "["+strings.Replace(openAccounting, ":0.25", ":true", 1)+","+strings.Replace(openAccounting, ":0.25", ":1", 1)+"]", "null", "null", false),
		Entry("last unique reason controls completion", "["+openAccounting+","+strings.Replace(strings.Replace(openAccounting, `"id":"i"`, `"id":"j"`, 1), `"stop"`, `"tool-calls"`, 1)+","+openAccounting+"]", "null", "null", false),
		Entry("optional total validated and omitted", "["+strings.Replace(openAccounting, `"input":11`, `"total":999,"input":11`, 1)+"]", usageTokens["opencode"], "0.25", true),
		Entry("bad optional total invalidates", "["+strings.Replace(openAccounting, `"input":11`, `"total":1.0,"input":11`, 1)+"]", "null", "null", false),
		Entry("optional total conflict", "["+strings.Replace(openAccounting, `"input":11`, `"total":999,"input":11`, 1)+","+openAccounting+"]", "null", "null", false),
		Entry("zero cost retained", "["+strings.Replace(openAccounting, ":0.25", ":0", 1)+"]", usageTokens["opencode"], "0", true))

	// per docs/adr/0064-go-native-usage-accounting.md:56
	DescribeTable("requires every nonempty string identity component", func(key, value string) {
		root, cwd := fixture()
		old := map[string]string{"sessionID": "s", "messageID": "m", "id": "i"}[key]
		event := strings.Replace(openAccounting, `"`+key+`":"`+old+`"`, `"`+key+`":`+value, 1)
		expectUsageParity(root, cwd, "opencode", "["+event+"]", usageExpected("opencode", "null", "null", false, false))
	}, Entry("empty session", "sessionID", `""`), Entry("null message", "messageID", "null"), Entry("numeric id", "id", "1"), Entry("array session", "sessionID", "[]"))

	// per docs/adr/0064-go-native-usage-accounting.md:68
	It("retains aggregate integer tokens even when the sum exceeds float finiteness", func() {
		root, cwd := fixture()
		huge := "1" + strings.Repeat("0", 308)
		first := strings.Replace(openAccounting, ":11", ":"+huge, 1)
		second := strings.Replace(first, `"id":"i"`, `"id":"j"`, 1)
		tokens := `{"input_tokens":2` + strings.Repeat("0", 308) + `,"output_tokens":6,"reasoning_output_tokens":14,"cache_read_input_tokens":4,"cache_creation_input_tokens":10}`
		expectUsageParity(root, cwd, "opencode", "["+first+","+second+"]", usageExpected("opencode", tokens, "0.5", true, false))
	})

	// per docs/adr/0064-go-native-usage-accounting.md:68
	It("invalidates an overflowing OpenCode floating cost sum", func() {
		root, cwd := fixture()
		first := strings.Replace(openAccounting, ":0.25", ":1e308", 1)
		second := strings.Replace(first, `"id":"i"`, `"id":"j"`, 1)
		expectUsageParity(root, cwd, "opencode", "["+first+","+second+"]", usageExpected("opencode", "null", "null", false, false))
	})

	// per docs/adr/0064-go-native-usage-accounting.md:27
	It("retains the last duplicate JSON key before normalization", func() {
		root, cwd := fixture()
		event := strings.Replace(codexAccounting, `"input_tokens":11`, `"input_tokens":99,"input_tokens":11`, 1)
		expectUsageParity(root, cwd, "codex", "["+event+"]", usageExpected("codex", usageTokens["codex"], "null", true, false))
	})

	// per docs/adr/0064-go-native-usage-accounting.md:40
	DescribeTable("never emits transcript, credentials, provider errors or unknown fields", func(harness string) {
		root, cwd := fixture()
		secret := "private answer $(touch marker) must-not-emit-credential"
		event := strings.Replace(usageEvents[harness], "{", `{"private":"`+secret+`",`, 1)
		cost := "0.25"
		if harness == "codex" {
			cost = "null"
		}
		expectUsageParity(root, cwd, harness, `[{"type":"unknown","text":"`+secret+`"},`+event+"]", usageExpected(harness, usageTokens[harness], cost, true, false))
	}, Entry("Codex", "codex"), Entry("Claude", "claude"), Entry("OpenCode", "opencode"))

	// per docs/adr/0064-go-native-usage-accounting.md:20
	DescribeTable("refuses malformed or non-array input with a fixed private diagnostic", func(input string) {
		root, cwd := fixture()
		result := usageProcess(cwd, input, filepath.Join(root, "factory"), "usage", "normalize", "codex")
		Expect(result).To(Equal(cliResult{"", "factory bridge: invalid usage event input\n", 1}))
		entries, err := os.ReadDir(cwd)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	}, Entry("empty", ""), Entry("object", "{}"), Entry("null", "null"), Entry("scalar", "3"), Entry("NDJSON", "{}\n{}\n"), Entry("trailing array", "[] []"), Entry("private malformed body", `[{"secret":"must-not-emit-credential"`), Entry("NaN", "[NaN]"), Entry("Infinity", "[Infinity]"), Entry("invalid UTF8", "[\"\xff\"]"), Entry("beyond input limit", "[\""+strings.Repeat("x", 16*1024*1024)+"\"]"))

	// per docs/adr/0064-go-native-usage-accounting.md:19
	DescribeTable("refuses unsupported harnesses and wrong arity before interpreting input", func(args []string) {
		root, cwd := fixture()
		result := usageProcess(cwd, "private malformed input", filepath.Join(root, "factory"), args...)
		Expect(result.status).To(Equal(2))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(ContainSubstring("private malformed input"))
		Expect(result.stderr).To(HavePrefix("factory bridge: "))
	}, Entry("missing harness", []string{"usage", "normalize"}), Entry("unknown harness", []string{"usage", "normalize", "other"}), Entry("case mismatch", []string{"usage", "normalize", "Codex"}), Entry("flag is not harness", []string{"usage", "normalize", "--help"}), Entry("surplus", []string{"usage", "normalize", "codex", "extra"}))

	// per docs/adr/0064-go-native-usage-accounting.md:48
	It("does not fall back to an earlier Codex completion when the last usage is missing", func() {
		root, cwd := fixture()
		expectUsageParity(root, cwd, "codex", "["+codexAccounting+`,{"type":"turn.completed"}]`, usageExpected("codex", "null", "null", false, false))
	})

	// per docs/adr/0064-go-native-usage-accounting.md:48
	// per docs/adr/0064-go-native-usage-accounting.md:58
	DescribeTable("requires every harness token and cache counter", func(harness string) {
		root, cwd := fixture()
		fields := map[string][]string{"codex": {`"input_tokens":11`, `"cached_input_tokens":2`, `"output_tokens":3`}, "claude": {`"input_tokens":11`, `"output_tokens":3`, `"cache_creation_input_tokens":5`, `"cache_read_input_tokens":2`}, "opencode": {`"input":11`, `"output":3`, `"reasoning":7`, `"read":2`, `"write":5`}}[harness]
		for _, field := range fields {
			key, _, ok := strings.Cut(field, ":")
			Expect(ok).To(BeTrue())
			event := strings.Replace(usageEvents[harness], field, key+":null", 1)
			cost := "null"
			if harness == "claude" {
				cost = "0.25"
			}
			expectUsageParity(root, cwd, harness, "["+event+"]", usageExpected(harness, "null", cost, false, false))
		}
	}, Entry("Codex", "codex"), Entry("Claude", "claude"), Entry("OpenCode", "opencode"))

	// per docs/adr/0064-go-native-usage-accounting.md:56
	It("keeps all three identity strings separate during deduplication", func() {
		root, cwd := fixture()
		first := strings.Replace(openAccounting, `"sessionID":"s","messageID":"m","id":"i"`, `"sessionID":"a","messageID":"bc","id":"d"`, 1)
		second := strings.Replace(openAccounting, `"sessionID":"s","messageID":"m","id":"i"`, `"sessionID":"ab","messageID":"c","id":"d"`, 1)
		expectedCounters := `{"input_tokens":22,"output_tokens":6,"reasoning_output_tokens":14,"cache_read_input_tokens":4,"cache_creation_input_tokens":10}`
		expectUsageParity(root, cwd, "opencode", "["+first+","+second+"]", usageExpected("opencode", expectedCounters, "0.5", true, false))
	})

	// per docs/adr/0064-go-native-usage-accounting.md:19
	It("refuses an unknown harness without waiting for an open empty stdin pipe", func() {
		root, cwd := fixture()
		readEnd, writeEnd, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(readEnd.Close)
		DeferCleanup(writeEnd.Close)
		result := usageRun(cwd, readEnd, filepath.Join(root, "factory"), "usage", "normalize", "unknown")
		Expect(result.status).To(Equal(2))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).To(HavePrefix("factory bridge: "))
	})

	// per docs/adr/0064-go-native-usage-accounting.md:24
	It("reports an actual stdin read failure without emitting accounting", func() {
		root, cwd := fixture()
		directory, err := os.Open(cwd)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(directory.Close)
		result := usageRun(cwd, directory, filepath.Join(root, "factory"), "usage", "normalize", "codex")
		Expect(result).To(Equal(cliResult{"", "factory bridge: invalid usage event input\n", 1}))
	})

	// per docs/adr/0064-go-native-usage-accounting.md:25
	It("reports an actual output write failure with the fixed metadata diagnostic", func() {
		root, cwd := fixture()
		output := filepath.Join(cwd, "limited-output")
		script := `trap '' XFSZ; ulimit -f 0 || exit 77; exec "$1" usage normalize codex > "$2"`
		result := usageProcess(cwd, "[]", "/bin/bash", "-c", script, "limited-usage-output", filepath.Join(root, "factory"), output)
		if result.status == 77 {
			Skip("test platform does not support file-size limits")
		}
		Expect(result).To(Equal(cliResult{"", "factory bridge: cannot write usage metadata\n", 1}))
		contents, err := os.ReadFile(output)
		Expect(err).NotTo(HaveOccurred())
		Expect(contents).To(BeEmpty())
	})

})
