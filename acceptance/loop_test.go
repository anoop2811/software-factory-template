package acceptance_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

const loopBaseline = "76952eaa63aebd1ecd282f5ab51dd7c3627cb497"

func loopProcess(cwd, program string, args, extra []string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, args...) // #nosec G204 G702 -- fixed acceptance programs and test-owned argv, never shell evaluation.
	cmd.Dir = cwd
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "FACTORY_") && !strings.HasPrefix(item, "OPENCODE_CONFIG_CONTENT=") {
			cmd.Env = append(cmd.Env, item)
		}
	}
	cmd.Env = append(cmd.Env, "PYTHONDONTWRITEBYTECODE=1", "FACTORY_BRIDGE_PROTOCOL=1", "GORACE=atexit_sleep_ms=0")
	cmd.Env = append(cmd.Env, extra...)
	var out, diagnostic bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &diagnostic
	err := cmd.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred())
	status := 0
	if err != nil {
		var exited *exec.ExitError
		Expect(errors.As(err, &exited)).To(BeTrue(), "%v", err)
		status = exited.ExitCode()
	}
	return cliResult{out.String(), diagnostic.String(), status}
}

func loopFixture() (string, string, string) {
	GinkgoHelper()
	root, cwd := fixture()
	checkout := filepath.Join(cwd, "checkout")
	Expect(os.MkdirAll(checkout, 0755)).To(Succeed())
	gitArgs := [][]string{{"init", "-q"}, {"config", "user.name", "Fixture"}, {"config", "user.email", "fixture@example.invalid"}, {"config", "commit.gpgsign", "false"}}
	for _, args := range gitArgs {
		out := loopProcess(checkout, "git", args, nil)
		Expect(out.status).To(BeZero(), "%+v", out)
	}
	writeFixture(filepath.Join(checkout, "source.txt"), []byte("initial source\n"), 0644)
	for _, args := range [][]string{{"add", "source.txt"}, {"commit", "-qm", "fixture"}} {
		Expect(loopProcess(checkout, "git", args, nil).status).To(BeZero())
	}
	oracle := filepath.Join(root, "oracle")
	for _, name := range []string{"loop.py", "budget.py", "budget_adapters.py"} {
		cmd := exec.Command("git", "show", loopBaseline+":scripts/lib/"+name) // #nosec G204 -- immutable revision and fixed fixture source names.
		data, err := cmd.Output()
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(oracle, name), data, 0600)
	}
	return root, cwd, checkout
}

func loopOracle(root, cwd, checkout string, extra ...string) cliResult {
	GinkgoHelper()
	program := `import json,os,pathlib,sys
sys.path.insert(0,sys.argv[1])
import loop,budget
config=loop.configuration()
result={"configuration":config,"snapshot":loop.stable_snapshot(pathlib.Path(os.environ["FACTORY_LOOP_ROOT"]),config),"policy":loop.policy(config,budget.configuration())}
print(json.dumps(result,sort_keys=True,allow_nan=False))`
	env := append([]string{"FACTORY_LOOP_ROOT=" + checkout}, extra...)
	return loopProcess(cwd, "python3", []string{"-B", "-c", program, filepath.Join(root, "oracle")}, env)
}

func loopCandidate(root, cwd, checkout string, extra ...string) cliResult {
	return loopProcess(cwd, filepath.Join(root, "factory"), []string{"loop", "fingerprint"}, append([]string{"FACTORY_LOOP_ROOT=" + checkout}, extra...))
}

var _ = Describe("G2 loop fingerprint core", func() {
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:13
	DescribeTable("matches immutable configuration and exact identities without state writes", func(extra []string) {
		root, cwd, checkout := loopFixture()
		oracle := loopOracle(root, cwd, checkout, extra...)
		Expect(oracle.status).To(BeZero(), "%+v", oracle)
		actual := loopCandidate(root, cwd, checkout, extra...)
		Expect(actual.status).To(BeZero(), "%+v", actual)
		Expect(actual.stderr).To(BeEmpty())
		Expect(budgetJSONComparable(budgetDecode([]byte(actual.stdout)))).To(Equal(budgetJSONComparable(budgetDecode([]byte(oracle.stdout)))))
		_, err := os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("defaults", []string{}), Entry("numeric identity", []string{"FACTORY_LOOP_MAX_ATTEMPTS=9007199254740993", "FACTORY_LOOP_TIMEOUT_SECONDS=1e-5"}), Entry("policy inputs", []string{"FACTORY_LOOP_CHECK_COMMAND=echo PRIVATE_CHECK", "FACTORY_LOOP_CODEX_IMPLEMENTER_MODEL=PRIVATE_MODEL", "OPENCODE_CONFIG_CONTENT=PRIVATE_OVERLAY"}))
})

func loopParity(root, cwd, checkout string, extra ...string) map[string]any {
	GinkgoHelper()
	oracle := loopOracle(root, cwd, checkout, extra...)
	Expect(oracle.status).To(BeZero(), "oracle: %+v", oracle)
	actual := loopCandidate(root, cwd, checkout, extra...)
	Expect(actual.status).To(BeZero(), "candidate: %+v", actual)
	Expect(actual.stderr).To(BeEmpty())
	want := budgetDecode([]byte(oracle.stdout))
	got := budgetDecode([]byte(actual.stdout))
	Expect(budgetJSONComparable(got)).To(Equal(budgetJSONComparable(want)))
	value, ok := got.(map[string]any)
	Expect(ok).To(BeTrue())
	Expect(value).To(HaveLen(3))
	return value
}
func loopRefused(out cliResult) {
	GinkgoHelper()
	Expect(out.status).To(Equal(2))
	Expect(out.stdout).To(BeEmpty())
	Expect(out.stderr).NotTo(BeEmpty())
	Expect(strings.Count(out.stderr, "\n")).To(Equal(1))
	Expect(out.stderr).NotTo(ContainSubstring("PRIVATE"))
}

var _ = Describe("G2 loop fingerprint policy", func() {
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:49
	DescribeTable("includes each configured policy field in the exact digest", func(setting string) {
		root, cwd, checkout := loopFixture()
		before := loopParity(root, cwd, checkout)
		after := loopParity(root, cwd, checkout, setting)
		Expect(after["policy"]).NotTo(Equal(before["policy"]))
	},
		Entry("loop enabled", "FACTORY_LOOP_ENABLED=true"), Entry("check command", "FACTORY_LOOP_CHECK_COMMAND=printf PRIVATE_COMMAND"),
		Entry("test patterns", "FACTORY_LOOP_TEST_PATTERNS=_test[.]go$"), Entry("protected paths", "FACTORY_LOOP_PROTECTED_PATHS=secure/*"),
		Entry("attempts", "FACTORY_LOOP_MAX_ATTEMPTS=3"), Entry("timeout", "FACTORY_LOOP_TIMEOUT_SECONDS=901"), Entry("check timeout", "FACTORY_LOOP_CHECK_TIMEOUT_SECONDS=121"), Entry("no progress", "FACTORY_LOOP_NO_PROGRESS_LIMIT=2"),
		Entry("budget enabled", "FACTORY_BUDGET_ENABLED=true"), Entry("budget action", "FACTORY_BUDGET_ACTION=warn"), Entry("budget attempts", "FACTORY_BUDGET_MAX_ATTEMPTS=2"), Entry("budget sessions", "FACTORY_BUDGET_MAX_SESSION_RUNS=6"), Entry("budget concurrency", "FACTORY_BUDGET_MAX_CONCURRENT=2"), Entry("budget timeout", "FACTORY_BUDGET_TIMEOUT_SECONDS=301"), Entry("budget session seconds", "FACTORY_BUDGET_SESSION_SECONDS=901"), Entry("budget estimate", "FACTORY_BUDGET_ESTIMATED_USD=0.1"),
		Entry("arbitrary role model", "FACTORY_LOOP_UNKNOWN_REVIEW_MODEL=PRIVATE_MODEL"), Entry("literal config path", "FACTORY_LOOP_CONFIG_PATH=absent.yaml"), Entry("native overlay", "OPENCODE_CONFIG_CONTENT=PRIVATE_OVERLAY"))
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:51
	It("excludes unrelated environment and environment ordering", func() {
		root, cwd, checkout := loopFixture()
		first := loopParity(root, cwd, checkout, "FACTORY_LOOP_CODEX_IMPLEMENTER_MODEL=x", "OPENCODE_CONFIG_CONTENT=y")
		second := loopParity(root, cwd, checkout, "OPENCODE_CONFIG_CONTENT=y", "PRIVATE_UNRELATED=z", "FACTORY_LOOP_OTHER=x", "FACTORY_LOOP_CODEX_IMPLEMENTER_MODEL=x")
		Expect(second).To(Equal(first))
	})
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:55
	DescribeTable("preserves Unicode and raw environment identity", func(extra []string) { root, cwd, checkout := loopFixture(); loopParity(root, cwd, checkout, extra...) },
		Entry("Python control whitespace", []string{"FACTORY_LOOP_PROTECTED_PATHS=a\x1cb\x1dc\x1ed\x1fe\u0085f\u00a0g"}),
		Entry("raw and Unicode model key ordering", []string{"FACTORY_LOOP_\xff_MODEL=one", "FACTORY_LOOP_😀_MODEL=two", "FACTORY_LOOP_�_MODEL=three"}),
		Entry("Unicode and binary model values", []string{"FACTORY_LOOP_CUSTOM_MODEL=PRIVATE_😀_\xff_\xed\xa0\x80", "OPENCODE_CONFIG_CONTENT=<>&\u2028"}),
		Entry("float spelling boundaries", []string{"FACTORY_LOOP_TIMEOUT_SECONDS=1e16", "FACTORY_LOOP_CHECK_TIMEOUT_SECONDS=1e-4", "FACTORY_BUDGET_ESTIMATED_USD=1e-7"}),
		Entry("Python numeric whitespace and digits", []string{"FACTORY_LOOP_TIMEOUT_SECONDS=\u00a0١_٢.٥\u00a0"}))
})

var _ = Describe("G2 loop fingerprint source", func() {
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:64
	DescribeTable("matches content and safety changes independently", func(kind string, sourceChanged, safetyChanged bool) {
		root, cwd, checkout := loopFixture()
		writeFixture(filepath.Join(checkout, ".gitignore"), []byte("ignored*\n.codex/\n"), 0644)
		writeFixture(filepath.Join(checkout, "case_test.go"), []byte("test fixture"), 0644)
		before := loopParity(root, cwd, checkout, "FACTORY_LOOP_TEST_PATTERNS=_test[.]go$")
		switch kind {
		case "edit":
			writeFixture(filepath.Join(checkout, "source.txt"), []byte("PRIVATE_NEW_SOURCE"), 0644)
		case "untracked":
			writeFixture(filepath.Join(checkout, "new.txt"), []byte("new"), 0644)
		case "mode":
			Expect(os.Chmod(filepath.Join(checkout, "source.txt"), 0751)).To(Succeed())
		case "special mode":
			Expect(os.Chmod(filepath.Join(checkout, "source.txt"), 0751|os.ModeSetuid|os.ModeSetgid|os.ModeSticky)).To(Succeed())
		case "delete":
			Expect(os.Remove(filepath.Join(checkout, "source.txt"))).To(Succeed())
		case "symlink":
			Expect(os.Symlink("PRIVATE_MISSING_TARGET", filepath.Join(checkout, "link"))).To(Succeed())
		case "test":
			writeFixture(filepath.Join(checkout, "case_test.go"), []byte("changed"), 0644)
		case "governing":
			writeFixture(filepath.Join(checkout, "AGENTS.md"), []byte("PRIVATE_POLICY"), 0644)
		case "ignored":
			writeFixture(filepath.Join(checkout, "ignored.txt"), []byte("PRIVATE_IGNORED"), 0644)
		case "factory":
			writeFixture(filepath.Join(checkout, ".factory/budget.json"), []byte("PRIVATE_INVALID_LEDGER"), 0644)
		case "native":
			writeFixture(filepath.Join(checkout, ".codex/config.toml"), []byte("PRIVATE_NATIVE_POLICY"), 0644)
		case "empty tree":
			Expect(os.MkdirAll(filepath.Join(checkout, ".codex/hooks"), 0755)).To(Succeed())
		case "stage":
			Expect(loopProcess(checkout, "git", []string{"add", "source.txt"}, nil).status).To(BeZero())
		case "binary name":
			loopRawNameFixture(filepath.Join(checkout, "name_\xff"), []byte{0, 255, 1})
		case "Unicode name":
			writeFixture(filepath.Join(checkout, "😀_é_é"), []byte("unicode"), 0644)
		}
		after := loopParity(root, cwd, checkout, "FACTORY_LOOP_TEST_PATTERNS=_test[.]go$")
		first := before["snapshot"].(map[string]any)
		last := after["snapshot"].(map[string]any)
		Expect(last["source"] != first["source"]).To(Equal(sourceChanged))
		Expect(last["safety"] != first["safety"]).To(Equal(safetyChanged))
		Expect(last["head"]).To(Equal(first["head"]))
	}, Entry("tracked edit", "edit", true, false), Entry("untracked", "untracked", true, false), Entry("mode", "mode", true, false), Entry("full stat mode bits", "special mode", true, false), Entry("deleted tracked", "delete", true, false), Entry("symlink target text", "symlink", true, false), Entry("POSIX ERE test", "test", true, true), Entry("fixed governing", "governing", true, true), Entry("ignored ordinary", "ignored", false, false), Entry("factory ignored by scanner", "factory", false, false), Entry("ignored native file", "native", false, true), Entry("missing versus empty native tree", "empty tree", false, true), Entry("index-only stage", "stage", false, false), Entry("nonUTF8 filename", "binary name", true, false), Entry("Unicode filenames", "Unicode name", true, false))
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:38
	DescribeTable("uses Python protected glob semantics", func(pattern, name string, protected bool) {
		root, cwd, checkout := loopFixture()
		writeFixture(filepath.Join(checkout, name), []byte("initial"), 0644)
		before := loopParity(root, cwd, checkout, "FACTORY_LOOP_PROTECTED_PATHS="+pattern)
		writeFixture(filepath.Join(checkout, name), []byte("changed"), 0644)
		after := loopParity(root, cwd, checkout, "FACTORY_LOOP_PROTECTED_PATHS="+pattern)
		Expect(after["snapshot"].(map[string]any)["safety"] != before["snapshot"].(map[string]any)["safety"]).To(Equal(protected))
	}, Entry("star crosses slash", "secure*", "secure/a/b", true), Entry("negated bracket", "[!a]*", "beta", true), Entry("literal bracket", "[[]*", "[file", true), Entry("literal backslash", "a\\*", "a\\name", true), Entry("invalid descending range", "[z-a]*", "z-file", false), Entry("invalid range with retained literal", "[a--!]", "a", true), Entry("invalid range retains endpoint", "[a--z]", "z", true), Entry("invalid range removes start", "[a--z]", "a", false), Entry("invalid range retains hyphen", "[b-a-c]", "-", true), Entry("invalid range retains trailing letter", "[b-a-c]", "c", true), Entry("invalid numeric range retains digit", "[9--0]", "0", true), Entry("case sensitive", "UPPER*", "upper-file", false), Entry("directory descendant", "safe/", "safe/deep/file", true))
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:78
	DescribeTable("preserves explicit effective config resolution", func(kind string) {
		root, cwd, checkout := loopFixture()
		external := filepath.Join(cwd, "external.yaml")
		writeFixture(external, []byte("PRIVATE_CONFIG"), 0600)
		path := external
		if kind == "relative" {
			path = "external.yaml"
		}
		if kind == "symlink" {
			path = filepath.Join(cwd, "config-link")
			Expect(os.Symlink("external.yaml", path)).To(Succeed())
		}
		loopParity(root, cwd, checkout, "FACTORY_LOOP_CONFIG_PATH="+path)
	}, Entry("outside checkout", "external"), Entry("relative caller cwd", "relative"), Entry("symlink text", "symlink"))
})

var _ = Describe("G2 loop fingerprint refusals", func() {
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:32
	DescribeTable("refuses invalid config without partial output or state", func(setting string) {
		root, cwd, checkout := loopFixture()
		oracle := loopOracle(root, cwd, checkout, setting)
		Expect(oracle.status).NotTo(BeZero())
		loopRefused(loopCandidate(root, cwd, checkout, setting))
		_, err := os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("case-sensitive boolean", "FACTORY_LOOP_ENABLED=True"), Entry("zero attempts", "FACTORY_LOOP_MAX_ATTEMPTS=0"), Entry("decimal integer", "FACTORY_LOOP_MAX_ATTEMPTS=1.0"), Entry("nonfinite", "FACTORY_LOOP_TIMEOUT_SECONDS=inf"), Entry("underflow", "FACTORY_LOOP_TIMEOUT_SECONDS=1e-999"), Entry("invalid underscore", "FACTORY_LOOP_TIMEOUT_SECONDS=1_e2"), Entry("invalid ERE", "FACTORY_LOOP_TEST_PATTERNS=[PRIVATE"), Entry("bad shared budget", "FACTORY_BUDGET_ACTION=PRIVATE"))
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:65
	DescribeTable("refuses unsafe snapshot input", func(kind string) {
		root, cwd, checkout := loopFixture()
		switch kind {
		case "no HEAD":
			Expect(loopProcess(checkout, "git", []string{"checkout", "--orphan", "unborn"}, nil).status).To(BeZero())
		case "newline":
			writeFixture(filepath.Join(checkout, "PRIVATE\nname"), []byte("x"), 0644)
		case "CR":
			writeFixture(filepath.Join(checkout, "PRIVATE\rname"), []byte("x"), 0644)
		case "FIFO":
			Expect(os.Remove(filepath.Join(checkout, "source.txt"))).To(Succeed())
			Expect(syscall.Mkfifo(filepath.Join(checkout, "source.txt"), 0600)).To(Succeed())
		case "native symlink":
			Expect(os.Symlink("source.txt", filepath.Join(checkout, "opencode.json"))).To(Succeed())
		case "native ancestor":
			Expect(os.Symlink(".", filepath.Join(checkout, ".codex"))).To(Succeed())
		case "native directory":
			Expect(os.Mkdir(filepath.Join(checkout, "opencode.json"), 0755)).To(Succeed())
		case "gitlink":
			head := strings.TrimSpace(loopProcess(checkout, "git", []string{"rev-parse", "HEAD"}, nil).stdout)
			Expect(loopProcess(checkout, "git", []string{"update-index", "--add", "--cacheinfo", "160000," + head + ",submodule"}, nil).status).To(BeZero())
		}
		if kind != "FIFO" {
			Expect(loopOracle(root, cwd, checkout).status).NotTo(BeZero())
		}
		loopRefused(loopCandidate(root, cwd, checkout))
	}, Entry("unborn HEAD", "no HEAD"), Entry("newline path", "newline"), Entry("CR path", "CR"), Entry("FIFO", "FIFO"), Entry("native symlink", "native symlink"), Entry("native ancestor link", "native ancestor"), Entry("native nonfile", "native directory"), Entry("gitlink", "gitlink"))
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:75
	DescribeTable("enforces native policy byte boundaries", func(size int, valid bool) {
		root, cwd, checkout := loopFixture()
		writeFixture(filepath.Join(checkout, "opencode.json"), bytes.Repeat([]byte("x"), size), 0644)
		if valid {
			loopParity(root, cwd, checkout)
		} else {
			Expect(loopOracle(root, cwd, checkout).status).NotTo(BeZero())
			loopRefused(loopCandidate(root, cwd, checkout))
		}
	}, Entry("exact1MiB", 1024*1024, true), Entry("above1MiB", 1024*1024+1, false))
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:75
	It("refuses more than512 native policy entries", func() {
		root, cwd, checkout := loopFixture()
		for i := range 513 {
			writeFixture(filepath.Join(checkout, ".codex/hooks", fmt.Sprint(i)), []byte{}, 0644)
		}
		Expect(loopOracle(root, cwd, checkout).status).NotTo(BeZero())
		loopRefused(loopCandidate(root, cwd, checkout))
	})
})

var _ = Describe("G2 loop fingerprint identities", func() {
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:55
	It("keeps invalid UTF8 names distinct from a literal replacement character", func() {
		root, cwd, checkout := loopFixture()
		loopRawNameFixture(filepath.Join(checkout, "name_\xff"), []byte("first"))
		writeFixture(filepath.Join(checkout, "name_\uFFFD"), []byte("second"), 0644)
		loopParity(root, cwd, checkout)
	})
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:64
	It("honors inherited alternate Git index without modifying the actual index", func() {
		root, cwd, checkout := loopFixture()
		alternate := filepath.Join(cwd, "alternate-index")
		writeFixture(filepath.Join(checkout, ".gitignore"), []byte("source.txt\n"), 0644)
		before := loopParity(root, cwd, checkout)
		index, err := os.ReadFile(filepath.Join(checkout, ".git/index"))
		Expect(err).NotTo(HaveOccurred())
		writeFixture(alternate, index, 0600)
		out := loopProcess(checkout, "git", []string{"update-index", "--force-remove", "source.txt"}, []string{"GIT_INDEX_FILE=" + alternate})
		Expect(out.status).To(BeZero())
		changed := loopParity(root, cwd, checkout, "GIT_INDEX_FILE="+alternate)
		Expect(changed["snapshot"].(map[string]any)["source"]).NotTo(Equal(before["snapshot"].(map[string]any)["source"]))
		after, err := os.ReadFile(filepath.Join(checkout, ".git/index"))
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(index))
	})
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:65
	It("refuses a genuinely unmerged index", func() {
		root, cwd, checkout := loopFixture()
		Expect(loopProcess(checkout, "git", []string{"checkout", "-qb", "other"}, nil).status).To(BeZero())
		writeFixture(filepath.Join(checkout, "source.txt"), []byte("other\n"), 0644)
		Expect(loopProcess(checkout, "git", []string{"commit", "-qam", "other"}, nil).status).To(BeZero())
		Expect(loopProcess(checkout, "git", []string{"checkout", "-q", "-"}, nil).status).To(BeZero())
		writeFixture(filepath.Join(checkout, "source.txt"), []byte("main\n"), 0644)
		Expect(loopProcess(checkout, "git", []string{"commit", "-qam", "main"}, nil).status).To(BeZero())
		Expect(loopProcess(checkout, "git", []string{"merge", "--no-edit", "other"}, nil).status).NotTo(BeZero())
		Expect(loopOracle(root, cwd, checkout).status).NotTo(BeZero())
		loopRefused(loopCandidate(root, cwd, checkout))
	})
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:70
	It("observes a new HEAD even with identical worktree content", func() {
		root, cwd, checkout := loopFixture()
		before := loopParity(root, cwd, checkout)
		Expect(loopProcess(checkout, "git", []string{"commit", "--allow-empty", "-qm", "next"}, nil).status).To(BeZero())
		after := loopParity(root, cwd, checkout)
		a := before["snapshot"].(map[string]any)
		b := after["snapshot"].(map[string]any)
		Expect(a["head"]).NotTo(Equal(b["head"]))
		Expect(a["source"]).To(Equal(b["source"]))
		Expect(a["safety"]).To(Equal(b["safety"]))
	})
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:77
	It("allows regular hardlinked native policy files", func() {
		root, cwd, checkout := loopFixture()
		Expect(os.Link(filepath.Join(checkout, "source.txt"), filepath.Join(checkout, "opencode.json"))).To(Succeed())
		loopParity(root, cwd, checkout)
	})
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:15
	It("defaults root to the actual caller cwd", func() {
		root, _, checkout := loopFixture()
		oracle := loopOracle(root, checkout, checkout)
		Expect(oracle.status).To(BeZero())
		actual := loopProcess(checkout, filepath.Join(root, "factory"), []string{"loop", "fingerprint"}, nil)
		Expect(actual.status).To(BeZero())
		Expect(budgetJSONComparable(budgetDecode([]byte(actual.stdout)))).To(Equal(budgetJSONComparable(budgetDecode([]byte(oracle.stdout)))))
	})
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:14
	It("refuses extra operands before inspecting the checkout", func() {
		root, cwd, _ := loopFixture()
		out := loopProcess(cwd, filepath.Join(root, "factory"), []string{"loop", "fingerprint", "PRIVATE_EXTRA"}, []string{"FACTORY_LOOP_ROOT=/PRIVATE_ABSENT"})
		loopRefused(out)
	})
})

var _ = Describe("G2 loop fingerprint private source limits", func() {
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:90
	DescribeTable("enforces the64MiB regular source bound", func(excess int64) {
		root, cwd, checkout := loopFixture()
		path := filepath.Join(checkout, "large.bin")
		writeFixture(path, nil, 0600)
		Expect(os.Truncate(path, 64*1024*1024+excess)).To(Succeed())
		if excess == 0 {
			loopParity(root, cwd, checkout)
		} else {
			loopRefused(loopCandidate(root, cwd, checkout))
		}
	}, Entry("exact", int64(0)), Entry("one byte over", int64(1)))
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:91
	It("counts actual content for each path toward512MiB even for hardlinks", func() {
		root, cwd, checkout := loopFixture()
		first := filepath.Join(checkout, "bulk0")
		writeFixture(first, nil, 0600)
		Expect(os.Truncate(first, 64*1024*1024)).To(Succeed())
		for i := 1; i < 9; i++ {
			Expect(os.Link(first, filepath.Join(checkout, fmt.Sprintf("bulk%d", i)))).To(Succeed())
		}
		loopRefused(loopCandidate(root, cwd, checkout))
	})
})

var _ = Describe("G2 loop fingerprint privacy", func() {
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:19
	It("does not execute the configured check or expose policy inputs", func() {
		root, cwd, checkout := loopFixture()
		marker := filepath.Join(cwd, "must-not-exist")
		writeFixture(filepath.Join(checkout, "opencode.json"), []byte("PRIVATE_FILE_CONTENT"), 0600)
		extra := []string{"FACTORY_LOOP_CHECK_COMMAND=touch '" + marker + "'", "FACTORY_LOOP_CODEX_IMPLEMENTER_MODEL=PRIVATE_MODEL", "OPENCODE_CONFIG_CONTENT=PRIVATE_OVERLAY"}
		actual := loopCandidate(root, cwd, checkout, extra...)
		Expect(actual.status).To(BeZero())
		Expect(actual.stdout + actual.stderr).NotTo(ContainSubstring("PRIVATE_"))
		_, err := os.Stat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue())
		_, err = os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:96
	It("refuses missing Git without source output", func() {
		root, cwd, checkout := loopFixture()
		loopRefused(loopCandidate(root, cwd, checkout, "PATH=/PRIVATE_ABSENT"))
	})
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:79
	It("retains lexical symlink-parent root resolution", func() {
		root, cwd, checkout := loopFixture()
		target := filepath.Join(checkout, "directory")
		Expect(os.Mkdir(target, 0755)).To(Succeed())
		alias := filepath.Join(cwd, "alias")
		Expect(os.Symlink(target, alias)).To(Succeed())
		lexical := alias + "/.."
		loopParity(root, cwd, lexical)
	})
})

func loopRawNameFixture(path string, data []byte) {
	GinkgoHelper()
	err := os.WriteFile(path, data, 0600)
	if errors.Is(err, syscall.EILSEQ) {
		Skip("test filesystem refuses non-UTF8 filenames; raw-name identity requires a filesystem accepting these bytes")
	}
	Expect(err).NotTo(HaveOccurred())
}

var _ = Describe("G2 loop fingerprint signaled grep", func() {
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:35
	DescribeTable("refuses terminated validation and source classification probes", func(classification bool) {
		root, cwd, checkout := loopFixture()
		bin := filepath.Join(root, "probe-bin")
		script := "#!/bin/sh\nkill -TERM $$\n"
		if classification {
			script = "#!/bin/sh\nif [ \"$2\" = '-q' ]; then exit 1; fi\nkill -TERM $$\n"
		}
		writeFixture(filepath.Join(bin, "grep"), []byte(script), 0755)
		extra := []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"), "FACTORY_LOOP_TEST_PATTERNS=x"}
		Expect(loopOracle(root, cwd, checkout, extra...).status).NotTo(BeZero())
		loopRefused(loopCandidate(root, cwd, checkout, extra...))
	}, Entry("validation", false), Entry("classification after valid pattern", true))
})

var _ = Describe("G2 loop fingerprint process signals", func() {
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:141
	DescribeTable("cleans up an observed owned probe before reporting interruption", func(signal syscall.Signal) {
		root, cwd, checkout := loopFixture()
		bin := filepath.Join(root, "signal-bin")
		marker := filepath.Join(cwd, "probe-pid")
		writeFixture(filepath.Join(bin, "git"), []byte("#!/bin/sh\nprintf '%s\\n' \"$$\" > \"$LOOP_SIGNAL_PID\"\nexec /bin/sleep 30\n"), 0700)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		command := exec.CommandContext(ctx, filepath.Join(root, "factory"), "loop", "fingerprint") // #nosec G204 G702 -- compiled test-owned candidate with fixed command, no shell.
		command.Dir = cwd
		for _, item := range os.Environ() {
			if !strings.HasPrefix(item, "FACTORY_") && !strings.HasPrefix(item, "OPENCODE_CONFIG_CONTENT=") {
				command.Env = append(command.Env, item)
			}
		}
		command.Env = append(command.Env, "FACTORY_BRIDGE_PROTOCOL=1", "FACTORY_LOOP_ROOT="+checkout, "PATH="+bin, "LOOP_SIGNAL_PID="+marker, "GORACE=atexit_sleep_ms=0")
		var output, diagnostic bytes.Buffer
		command.Stdout = &output
		command.Stderr = &diagnostic
		Expect(command.Start()).To(Succeed())
		done := make(chan error, 1)
		go func() { defer close(done); done <- command.Wait() }()
		probePID := 0
		DeferCleanup(func() {
			if probePID > 0 {
				_ = syscall.Kill(-probePID, syscall.SIGKILL)
			}
			cancel()
			Eventually(done, 3*time.Second).Should(BeClosed())
		})
		var raw []byte
		Eventually(func() error { var err error; raw, err = os.ReadFile(marker); return err }, 3*time.Second).Should(Succeed())
		var err error
		probePID, err = strconv.Atoi(strings.TrimSpace(string(raw)))
		Expect(err).NotTo(HaveOccurred())
		Expect(probePID).To(BeNumerically(">", 0))
		Expect(syscall.Kill(probePID, 0)).To(Succeed())
		Expect(command.Process.Signal(signal)).To(Succeed())
		var waitErr error
		Eventually(done, 3*time.Second).Should(Receive(&waitErr))
		Expect(ctx.Err()).NotTo(HaveOccurred())
		var exited *exec.ExitError
		Expect(errors.As(waitErr, &exited)).To(BeTrue())
		status := exited.ExitCode()
		running := syscall.Kill(probePID, 0) == nil
		Expect(status).To(Equal(2), "probe still exists=%v; stdout=%q stderr=%q", running, output.String(), diagnostic.String())
		loopRefused(cliResult{output.String(), diagnostic.String(), status})
		Expect(syscall.Kill(probePID, 0)).To(Equal(syscall.ESRCH))
	}, Entry("SIGTERM", syscall.SIGTERM), Entry("SIGINT", syscall.SIGINT))
})
