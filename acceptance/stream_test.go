package acceptance_test

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const streamBaseline = "a0216779e4f94057f3e6752ce0ef178499721af4"

func expectStreamParity(root, cwd, harness, input, expected string) {
	GinkgoHelper()
	for _, name := range []string{"budget.py", "budget_adapters.py"} {
		command := exec.Command("git", "show", streamBaseline+":scripts/lib/"+name) // #nosec G204 -- immutable test baseline and two fixed source paths.
		contents, err := command.Output()
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(root, name), contents, 0600)
	}
	python, err := exec.LookPath("python3")
	Expect(err).NotTo(HaveOccurred())
	// Only the immutable parse_events AST is executed from budget.py. Importing
	// the controller itself would introduce unrelated command/lifecycle behavior.
	oracle := `import ast,json,runpy,sys
sys.set_int_max_str_digits(4300)
source=ast.parse(open(sys.argv[1],encoding="utf-8").read())
functions=[n for n in source.body if isinstance(n,ast.FunctionDef) and n.name=="parse_events"]
assert len(functions)==1
scope={"json":json}
exec(compile(ast.Module(body=functions,type_ignores=[]),sys.argv[1],"exec"),scope)
normalizer=runpy.run_path(sys.argv[2])["normalize"]
print(json.dumps(normalizer(sys.argv[3],scope["parse_events"](sys.stdin.buffer.read())),allow_nan=False))`
	baseline := usageProcess(cwd, input, "/usr/bin/env", "PATH="+os.Getenv("PATH"), "HOME="+os.Getenv("HOME"), python, "-I", "-B", "-c", oracle, filepath.Join(root, "budget.py"), filepath.Join(root, "budget_adapters.py"), harness)
	Expect(baseline.status).To(Equal(0), baseline.stderr)
	Expect(baseline.stderr).To(BeEmpty())
	Expect(usageMetadata(baseline.stdout)).To(Equal(usageMetadata(expected)), "immutable Python stream parsing must match explicit metadata")
	result := usageProcess(cwd, input, filepath.Join(root, "factory"), "usage", "parse", harness)
	Expect(result.status).To(Equal(0), result.stderr)
	Expect(result.stderr).To(BeEmpty())
	Expect(result.stdout).To(HaveSuffix("\n"))
	Expect(usageMetadata(result.stdout)).To(Equal(usageMetadata(expected)))
	entries, err := os.ReadDir(cwd)
	Expect(err).NotTo(HaveOccurred())
	Expect(entries).To(BeEmpty())
}

type streamChunks struct {
	text  string
	width int
}

func (r *streamChunks) Read(p []byte) (int, error) {
	if r.text == "" {
		return 0, io.EOF
	}
	size := r.width
	if size > len(p) {
		size = len(p)
	}
	if size > len(r.text) {
		size = len(r.text)
	}
	copy(p, r.text[:size])
	r.text = r.text[size:]
	return size, nil
}

var _ = Describe("G2 native event stream accounting", func() {
	// per docs/adr/0066-go-native-event-streams.md:34
	DescribeTable("accepts a whole native object for each harness", func(harness string) {
		root, cwd := fixture()
		cost := "0.25"
		if harness == "codex" {
			cost = "null"
		}
		expectStreamParity(root, cwd, harness, usageEvents[harness], usageExpected(harness, usageTokens[harness], cost, true, false))
	}, Entry("Codex", "codex"), Entry("Claude", "claude"), Entry("OpenCode", "opencode"))

	// per docs/adr/0066-go-native-event-streams.md:35
	DescribeTable("does not reinterpret a whole JSON array as native event records", func(harness string) {
		root, cwd := fixture()
		expectStreamParity(root, cwd, harness, "["+usageEvents[harness]+"]", usageExpected(harness, "null", "null", false, false))
	}, Entry("Codex", "codex"), Entry("Claude", "claude"), Entry("OpenCode", "opencode"))
	// per docs/adr/0066-go-native-event-streams.md:36
	DescribeTable("accepts native records separated by every Python splitlines boundary", func(separator string) {
		root, cwd := fixture()
		input := `{"type":"ignored"}` + separator + codexAccounting
		expectStreamParity(root, cwd, "codex", input, usageExpected("codex", usageTokens["codex"], "null", true, false))
	}, Entry("LF", "\n"), Entry("CR", "\r"), Entry("CRLF", "\r\n"), Entry("vertical tab", "\v"), Entry("form feed", "\f"), Entry("file separator", "\x1c"), Entry("group separator", "\x1d"), Entry("record separator", "\x1e"), Entry("NEL", "\u0085"), Entry("line separator", "\u2028"), Entry("paragraph separator", "\u2029"))

	// per docs/adr/0066-go-native-event-streams.md:39
	DescribeTable("returns unknown accounting for empty or whole nonobject input", func(input string) {
		root, cwd := fixture()
		expectStreamParity(root, cwd, "codex", input, usageExpected("codex", "null", "null", false, false))
	}, Entry("empty", ""), Entry("JSON whitespace", " \t\r\n"), Entry("Unicode whitespace", "\u2003\u00a0"), Entry("null", "null"), Entry("boolean", "true"), Entry("number", "42"), Entry("string", `"private transcript"`), Entry("empty array", "[]"), Entry("NaN scalar", "NaN"), Entry("malformed", "{private transcript"))

	// per docs/adr/0066-go-native-event-streams.md:37
	DescribeTable("distinguishes Python strip whitespace from accepted JSON whitespace", func(input string, complete bool) {
		root, cwd := fixture()
		expected := "null"
		if complete {
			expected = usageTokens["codex"]
		}
		expectStreamParity(root, cwd, "codex", input, usageExpected("codex", expected, "null", complete, false))
	}, Entry("Unicode-only line is skipped", "\u2003\u00a0\n"+codexAccounting, true),
		Entry("unit separator whitespace-only line is skipped", "\x1f\n"+codexAccounting, true),
		Entry("unit separator does not split records", `{"type":"ignored"}`+"\x1f"+codexAccounting, false),
		Entry("Unicode whitespace around object is not JSON whitespace", "\u00a0"+codexAccounting+"\u00a0", false),
		Entry("ASCII whitespace around object is allowed", " \t"+codexAccounting+"\r\n", true))

	// per docs/adr/0066-go-native-event-streams.md:34
	It("parses pretty whole objects before attempting line fallback", func() {
		root, cwd := fixture()
		pretty := strings.ReplaceAll(codexAccounting, ",", ",\n ")
		expectStreamParity(root, cwd, "codex", pretty, usageExpected("codex", usageTokens["codex"], "null", true, false))
	})

	// per docs/adr/0066-go-native-event-streams.md:34
	It("keeps a Unicode line separator inside a whole JSON string but splits after whole decoding fails", func() {
		root, cwd := fixture()
		input := strings.Replace(codexAccounting, "{", `{"private":"left`+"\u2028"+`right",`, 1)
		expectStreamParity(root, cwd, "codex", input, usageExpected("codex", usageTokens["codex"], "null", true, false))
		expectStreamParity(root, cwd, "codex", input+"\n{}", usageExpected("codex", "null", "null", false, false))
	})

	// per docs/adr/0066-go-native-event-streams.md:38
	DescribeTable("retains selected usage but marks partial trailing records incomplete", func(harness string) {
		root, cwd := fixture()
		expected := usageTokens[harness]
		if harness == "opencode" {
			expected = "null"
		}
		expectStreamParity(root, cwd, harness, usageEvents[harness]+"\n"+`{"private":"unfinished`, usageExpected(harness, expected, "null", false, false))
	}, Entry("Codex", "codex"), Entry("Claude", "claude"), Entry("OpenCode", "opencode"))

	// per docs/adr/0066-go-native-event-streams.md:32
	DescribeTable("discards every decoded event when any raw byte is invalid UTF8", func(invalid string) {
		root, cwd := fixture()
		expectStreamParity(root, cwd, "codex", codexAccounting+"\n"+invalid, usageExpected("codex", "null", "null", false, false))
	}, Entry("invalid byte", "\xff"), Entry("overlong encoding", "\xc0\xaf"), Entry("encoded surrogate", "\xed\xa0\x80"), Entry("truncated codepoint", "\xf0\x9f"))

	// per docs/adr/0066-go-native-event-streams.md:43
	DescribeTable("keeps permissive constants in unknown fields from erasing valid usage", func(constant string) {
		root, cwd := fixture()
		for _, harness := range []string{"codex", "claude", "opencode"} {
			input := strings.Replace(usageEvents[harness], "{", `{"private":`+constant+",", 1)
			cost := "0.25"
			if harness == "codex" {
				cost = "null"
			}
			expectStreamParity(root, cwd, harness, input, usageExpected(harness, usageTokens[harness], cost, true, false))
		}
	}, Entry("NaN", "NaN"), Entry("positive infinity", "Infinity"), Entry("negative infinity", "-Infinity"))

	// per docs/adr/0066-go-native-event-streams.md:43
	It("retains a failure event that contains a permissive numeric constant", func() {
		root, cwd := fixture()
		expectStreamParity(root, cwd, "codex", `{"type":"error","private":NaN}`, usageExpected("codex", "null", "null", false, true))
	})

	// per docs/adr/0066-go-native-event-streams.md:45
	DescribeTable("preserves integer, float and nonfinite distinctions in accounting fields", func(number, tokens string) {
		root, cwd := fixture()
		input := strings.Replace(codexAccounting, ":11", ":"+number, 1)
		expectStreamParity(root, cwd, "codex", input, usageExpected("codex", tokens, "null", tokens != "null", false))
	}, Entry("exact large integer", "9007199254740993", strings.Replace(usageTokens["codex"], ":11", ":9007199254740993", 1)),
		Entry("integral float", "11.0", "null"), Entry("integral exponent", "11e0", "null"), Entry("NaN token", "NaN", "null"), Entry("infinite token", "Infinity", "null"))

	// per docs/adr/0066-go-native-event-streams.md:45
	DescribeTable("preserves cost float overflow and underflow without dropping valid tokens", func(number, cost string, complete bool) {
		root, cwd := fixture()
		input := strings.Replace(claudeAccounting, ":0.25", ":"+number, 1)
		expectStreamParity(root, cwd, "claude", input, usageExpected("claude", usageTokens["claude"], cost, complete, false))
	}, Entry("underflow", "1e-999", "0", true), Entry("negative underflow", "-1e-999", "0", true), Entry("overflow", "1e999", "null", false), Entry("NaN cost", "NaN", "null", false), Entry("Infinity cost", "Infinity", "null", false))

	// per docs/adr/0066-go-native-event-streams.md:46
	DescribeTable("keeps Python Unicode identity equality without merging distinct surrogates", func(firstID, secondID string, deduplicated bool) {
		root, cwd := fixture()
		first := strings.Replace(openAccounting, `"id":"i"`, `"id":`+firstID, 1)
		second := strings.Replace(openAccounting, `"id":"i"`, `"id":`+secondID, 1)
		expectedCounters := `{"input_tokens":22,"output_tokens":6,"reasoning_output_tokens":14,"cache_read_input_tokens":4,"cache_creation_input_tokens":10}`
		cost := "0.5"
		if deduplicated {
			expectedCounters = usageTokens["opencode"]
			cost = "0.25"
		}
		expectStreamParity(root, cwd, "opencode", first+"\n"+second, usageExpected("opencode", expectedCounters, cost, true, false))
	}, Entry("surrogate pair equals literal codepoint", `"\ud83d\ude00"`, `"😀"`, true),
		Entry("distinct unpaired high surrogates", `"\ud800"`, `"\ud801"`, false),
		Entry("unpaired surrogate differs from replacement character", `"\ud800"`, `"�"`, false),
		Entry("high and low surrogates remain distinct", `"\ud800"`, `"\udc00"`, false),
		Entry("same unpaired surrogate deduplicates", `"\ud800"`, `"\ud800"`, true))

	// per docs/adr/0066-go-native-event-streams.md:43
	It("retains Python NaN singleton equality inside duplicate step signatures", func() {
		root, cwd := fixture()
		first := strings.Replace(openAccounting, `"reason":"stop"`, `"reason":NaN`, 1)
		last := strings.Replace(openAccounting, `"id":"i"`, `"id":"j"`, 1)
		expectedCounters := `{"input_tokens":22,"output_tokens":6,"reasoning_output_tokens":14,"cache_read_input_tokens":4,"cache_creation_input_tokens":10}`
		expectStreamParity(root, cwd, "opencode", first+"\n"+first+"\n"+last, usageExpected("opencode", expectedCounters, "0.5", true, false))
	})

	// per docs/adr/0066-go-native-event-streams.md:40
	It("keeps the last duplicate key in a parsed native object", func() {
		root, cwd := fixture()
		input := strings.Replace(codexAccounting, `"input_tokens":11`, `"input_tokens":99,"input_tokens":11`, 1)
		expectStreamParity(root, cwd, "codex", input, usageExpected("codex", usageTokens["codex"], "null", true, false))
	})

	// per docs/adr/0066-go-native-event-streams.md:15
	DescribeTable("is independent of fragmented stdin writes including Unicode and escape boundaries", func(width int) {
		root, cwd := fixture()
		input := strings.Replace(openAccounting, `"id":"i"`, `"id":"\ud83d\ude00日本語"`, 1)
		result := usageRun(cwd, &streamChunks{text: input, width: width}, filepath.Join(root, "factory"), "usage", "parse", "opencode")
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(result.stderr).To(BeEmpty())
		Expect(usageMetadata(result.stdout)).To(Equal(usageMetadata(usageExpected("opencode", usageTokens["opencode"], "0.25", true, false))))
	}, Entry("one byte", 1), Entry("three bytes", 3), Entry("seventeen bytes", 17))

	// per docs/adr/0066-go-native-event-streams.md:51
	It("admits JSON nesting exactly at the private depth boundary", func() {
		root, cwd := fixture()
		input := strings.TrimSuffix(codexAccounting, "}") + `,"private":` + strings.Repeat("[", 511) + "0" + strings.Repeat("]", 511) + "}"
		expectStreamParity(root, cwd, "codex", input, usageExpected("codex", usageTokens["codex"], "null", true, false))
	})

	// per docs/adr/0066-go-native-event-streams.md:20
	// per docs/adr/0066-go-native-event-streams.md:51
	DescribeTable("refuses excessive raw input or depth without emitting partial metadata", func(input string) {
		root, cwd := fixture()
		result := usageProcess(cwd, input, filepath.Join(root, "factory"), "usage", "parse", "codex")
		Expect(result).To(Equal(cliResult{"", "factory bridge: invalid usage stream input\n", 1}))
	}, Entry("more than16MiB", strings.Repeat(" ", 16*1024*1024+1)),
		Entry("depth513", strings.TrimSuffix(codexAccounting, "}")+`,"private":`+strings.Repeat("[", 512)+"0"+strings.Repeat("]", 512)+"}"))

	// per docs/adr/0066-go-native-event-streams.md:19
	DescribeTable("refuses unsupported command operands", func(args []string) {
		root, cwd := fixture()
		result := usageProcess(cwd, "private malformed data", filepath.Join(root, "factory"), args...)
		Expect(result.status).To(Equal(2))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).To(HavePrefix("factory bridge: "))
		Expect(result.stderr).NotTo(ContainSubstring("private malformed data"))
	}, Entry("missing harness", []string{"usage", "parse"}), Entry("unknown harness", []string{"usage", "parse", "other"}), Entry("wrong case", []string{"usage", "parse", "Codex"}), Entry("surplus", []string{"usage", "parse", "codex", "extra"}), Entry("help as harness", []string{"usage", "parse", "--help"}))

	// per docs/adr/0066-go-native-event-streams.md:19
	It("refuses invalid arity before reading an open stdin pipe", func() {
		root, cwd := fixture()
		readEnd, writeEnd, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(readEnd.Close)
		DeferCleanup(writeEnd.Close)
		result := usageRun(cwd, readEnd, filepath.Join(root, "factory"), "usage", "parse")
		Expect(result.status).To(Equal(2))
		Expect(result.stdout).To(BeEmpty())
	})

	// per docs/adr/0066-go-native-event-streams.md:21
	It("reports a real stdin read failure using only the fixed input diagnostic", func() {
		root, cwd := fixture()
		directory, err := os.Open(cwd)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(directory.Close)
		Expect(usageRun(cwd, directory, filepath.Join(root, "factory"), "usage", "parse", "codex")).To(Equal(cliResult{"", "factory bridge: invalid usage stream input\n", 1}))
	})

	// per docs/adr/0066-go-native-event-streams.md:23
	It("reports a real output write failure with no partial metadata", func() {
		root, cwd := fixture()
		output := filepath.Join(cwd, "limited-stream-output")
		script := `trap '' XFSZ; ulimit -f 0 || exit 77; exec "$1" usage parse codex > "$2"`
		result := usageProcess(cwd, codexAccounting, "/bin/bash", "-c", script, "limited-stream", filepath.Join(root, "factory"), output)
		if result.status == 77 {
			Skip("test platform does not support file-size limits")
		}
		Expect(result).To(Equal(cliResult{"", "factory bridge: cannot write usage metadata\n", 1}))
		data, err := os.ReadFile(output)
		Expect(err).NotTo(HaveOccurred())
		Expect(data).To(BeEmpty())
	})

	// per docs/adr/0066-go-native-event-streams.md:54
	DescribeTable("matches the qualified Python integer digit limit for ignored fields", func(digits int, complete bool) {
		root, cwd := fixture()
		input := strings.Replace(codexAccounting, "{", `{"private":1`+strings.Repeat("0", digits-1)+",", 1)
		tokens := "null"
		if complete {
			tokens = usageTokens["codex"]
		}
		expectStreamParity(root, cwd, "codex", input, usageExpected("codex", tokens, "null", complete, false))
	}, Entry("4300digits", 4300, true), Entry("4301digits", 4301, false))

})
