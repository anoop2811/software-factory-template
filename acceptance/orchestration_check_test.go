package acceptance_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const checkOrchestrationBaseline = "4ecab79627d08d1e125391a5831eb995efd3fce4"

type orchestrationFixture struct {
	root, repo, bin, trace, input string
	env                           []string
	base, first, second           string
}

func orchestrationHistorical(path string) []byte {
	GinkgoHelper()
	command := exec.Command("git", "show", checkOrchestrationBaseline+":"+path) // #nosec G204 -- immutable local revision and test-owned path allowlist.
	command.Dir = ".."
	data, err := command.Output()
	Expect(err).NotTo(HaveOccurred(), path)
	return data
}

func orchestrationEnvironment(overrides ...string) []string {
	result := []string{}
	for _, value := range os.Environ() {
		key := strings.SplitN(value, "=", 2)[0]
		if strings.HasPrefix(key, "FACTORY_") || strings.HasPrefix(key, "GIT_") || strings.HasPrefix(key, "CHECK_") || key == "REPO_ROOT" || key == "PATH" || key == "BASH_ENV" || key == "ENV" {
			continue
		}
		result = append(result, value)
	}
	return append(result, append([]string{"LC_ALL=C", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull, "GORACE=atexit_sleep_ms=0"}, overrides...)...)
}

func orchestrationGit(repo string, args ...string) string {
	GinkgoHelper()
	command := exec.Command("git", args...) // #nosec G204 -- isolated local Git fixture; no shell or network operation.
	command.Dir = repo
	command.Env = orchestrationEnvironment("PATH=" + os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "git %v: %s", args, output)
	return strings.TrimSpace(string(output))
}

func newOrchestrationFixture() *orchestrationFixture {
	GinkgoHelper()
	root, repo := fixture()
	f := &orchestrationFixture{root: root, repo: repo, bin: filepath.Join(root, "fake-bin"), trace: filepath.Join(repo, "stage-trace"), input: filepath.Join(repo, "gate-input")}
	orchestrationGit(repo, "init", "-q")
	orchestrationGit(repo, "config", "user.email", "acceptance@example.invalid")
	orchestrationGit(repo, "config", "user.name", "Acceptance")
	orchestrationGit(repo, "config", "core.hooksPath", os.DevNull)
	for _, name := range []string{"base", "first", "second"} {
		writeFixture(filepath.Join(repo, "tracked"), []byte(name), 0600)
		orchestrationGit(repo, "add", "tracked")
		orchestrationGit(repo, "commit", "-qm", "test: "+name)
		sha := orchestrationGit(repo, "rev-parse", "HEAD")
		switch name {
		case "base":
			f.base = sha
		case "first":
			f.first = sha
		case "second":
			f.second = sha
		}
	}
	orchestrationGit(repo, "update-ref", "refs/remotes/origin/main", f.base)
	writeFixture(filepath.Join(root, "oracle/pre-push-check.sh"), orchestrationHistorical("scripts/pre-push-check.sh"), 0700)
	writeFixture(filepath.Join(root, "oracle/prereq-check.sh"), orchestrationHistorical("scripts/prereq-check.sh"), 0700)
	writeFixture(filepath.Join(root, "oracle/lib/config.sh"), orchestrationHistorical("scripts/lib/config.sh"), 0600)
	writeFixture(filepath.Join(repo, "scripts/lib/config.sh"), orchestrationHistorical("scripts/lib/config.sh"), 0600)
	reader, err := os.ReadFile(filepath.Join("..", "runtime/shell/readers.sh"))
	Expect(err).NotTo(HaveOccurred())
	writeFixture(filepath.Join(root, "runtime/shell/readers.sh"), reader, 0600)
	for _, script := range []string{"pre-push-check.sh", "prereq-check.sh"} {
		writeFixture(filepath.Join(root, "scripts", script), []byte("#!/bin/sh\nprintf 'LEGACY_ORCHESTRATION_CALLED\\n' >&2\nexit 93\n"), 0700)
	}
	for _, name := range []string{"direct-main-push-block", "commit-message-lint", "diff-aware-check", "decision-log-gate", "pending-lessons-push-block"} {
		f.gate(name, "")
	}
	writeFixture(filepath.Join(f.bin, "make"), []byte(`#!/bin/sh
printf 'make\t%s\n' "$1" >> "$CHECK_TRACE"
printf 'make %s stdout\n' "$1"
printf 'make %s stderr\n' "$1" >&2
case "$1" in
  check) exit "${CHECK_FAIL_MAKE_CHECK:-0}" ;;
  check-drift) exit "${CHECK_FAIL_MAKE_DRIFT:-0}" ;;
  *) exit 92 ;;
esac
`), 0700)
	writeFixture(filepath.Join(repo, "factory.yaml"), []byte("local_hooks: \"\"\n"), 0600)
	f.env = orchestrationEnvironment("PATH="+f.bin+string(os.PathListSeparator)+os.Getenv("PATH"), "CHECK_TRACE="+f.trace, "CHECK_STDIN="+f.input, "FACTORY_RUNTIME_BINARY="+filepath.Join(root, "factory"))
	return f
}

func (f *orchestrationFixture) gate(name, extra string) {
	GinkgoHelper()
	program := "#!/bin/sh\nprintf '%s' '" + name + "' >> \"$CHECK_TRACE\"\nfor argument do printf '\\t%s' \"$argument\" >> \"$CHECK_TRACE\"; done\nprintf '\\n' >> \"$CHECK_TRACE\"\n"
	if name == "direct-main-push-block" {
		program += "/bin/cat > \"$CHECK_STDIN\"\n"
	}
	program += extra
	writeFixture(filepath.Join(f.repo, "scripts/hooks", name+".sh"), []byte(program), 0700)
}

func orchestrationRead(path string) string {
	GinkgoHelper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	Expect(err).NotTo(HaveOccurred())
	return string(data)
}

func (f *orchestrationFixture) run(input string, environment []string, executable string, args ...string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	inputFile, err := os.CreateTemp(filepath.Dir(f.repo), "stdin-")
	Expect(err).NotTo(HaveOccurred())
	defer inputFile.Close()
	_, err = inputFile.WriteString(input)
	Expect(err).NotTo(HaveOccurred())
	_, err = inputFile.Seek(0, 0)
	Expect(err).NotTo(HaveOccurred())
	stdout, err := os.CreateTemp(filepath.Dir(f.repo), "stdout-")
	Expect(err).NotTo(HaveOccurred())
	defer stdout.Close()
	stderr, err := os.CreateTemp(filepath.Dir(f.repo), "stderr-")
	Expect(err).NotTo(HaveOccurred())
	defer stderr.Close()
	command := exec.CommandContext(ctx, executable, args...) // #nosec G204 -- compiled CLI or immutable local oracle with literal fixture argv.
	command.Dir = f.repo
	command.Env = append(append([]string{}, f.env...), environment...)
	command.Stdin, command.Stdout, command.Stderr = inputFile, stdout, stderr
	err = command.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred(), "orchestration timed out")
	status := 0
	if err != nil {
		var exit *exec.ExitError
		Expect(errors.As(err, &exit)).To(BeTrue(), "%v", err)
		status = exit.ExitCode()
	}
	return cliResult{orchestrationRead(stdout.Name()), orchestrationRead(stderr.Name()), status}
}

func (f *orchestrationFixture) check(input string, environment ...string) cliResult {
	return f.run(input, environment, filepath.Join(f.root, "factory"), "check")
}

func (f *orchestrationFixture) parity(input string, environment ...string) cliResult {
	GinkgoHelper()
	baseline := f.run(input, environment, "/bin/bash", filepath.Join(f.root, "oracle/pre-push-check.sh"))
	trace := orchestrationRead(f.trace)
	Expect(os.Remove(f.trace)).To(Succeed())
	// Use the existing thin reader for native config.Hooks integration; the oracle
	// above used the immutable parser, rather than the candidate as its own oracle.
	reader, err := os.ReadFile(filepath.Join(f.root, "runtime/shell/readers.sh"))
	Expect(err).NotTo(HaveOccurred())
	writeFixture(filepath.Join(f.repo, "scripts/lib/config.sh"), reader, 0600)
	actual := f.check(input, environment...)
	Expect(actual).To(Equal(baseline))
	Expect(orchestrationRead(f.trace)).To(Equal(trace))
	return actual
}

var _ = Describe("Native orchestration check", func() {
	// per docs/adr/0099-native-factory-orchestration.md:84
	// per docs/adr/0099-native-factory-orchestration.md:87
	It("runs local hooks from an explicitly selected nested repository root instead of the enclosing Git root", func() {
		f := newOrchestrationFixture()
		nested := filepath.Join(f.repo, "selected nested project")
		writeFixture(filepath.Join(nested, "factory.yaml"), []byte("local_hooks: reject.sh\n"), 0600)
		writeFixture(filepath.Join(nested, "scripts/lib/config.sh"), orchestrationHistorical("scripts/lib/config.sh"), 0600)
		writeFixture(filepath.Join(nested, "reject.sh"), []byte("#!/bin/sh\nprintf 'nested-local-hook\\n' >> \"$CHECK_TRACE\"\nexit 23\n"), 0700)
		writeFixture(filepath.Join(f.repo, "reject.sh"), []byte("#!/bin/sh\nprintf 'wrong-enclosing-hook\\n' >> \"$CHECK_TRACE\"\nexit 0\n"), 0700)
		for _, name := range []string{"direct-main-push-block", "commit-message-lint", "diff-aware-check", "decision-log-gate", "pending-lessons-push-block"} {
			writeFixture(filepath.Join(nested, "scripts/hooks", name+".sh"), []byte("#!/bin/sh\nexit 0\n"), 0700)
		}
		result := f.check("", "REPO_ROOT="+nested)
		Expect(result.status).To(Equal(1), "the selected nested hook denies the check: %+v", result)
		Expect(result.stdout).NotTo(ContainSubstring("PUSH ALLOWED"))
		Expect(orchestrationRead(f.trace)).To(ContainSubstring("nested-local-hook\n"))
		Expect(orchestrationRead(f.trace)).NotTo(ContainSubstring("wrong-enclosing-hook"))
	})

	// per docs/adr/0099-native-factory-orchestration.md:75
	It("runs the eight stages in baseline order without executing legacy orchestration", func() {
		f := newOrchestrationFixture()
		result := f.parity("")
		Expect(result.status).To(BeZero(), "%+v", result)
		Expect(orchestrationRead(f.trace)).To(Equal("direct-main-push-block\nmake\tcheck\nmake\tcheck-drift\ncommit-message-lint\t" + f.second + "\ncommit-message-lint\t" + f.first + "\ndiff-aware-check\t" + f.base + "\tHEAD\ndecision-log-gate\t" + f.base + "\tHEAD\npending-lessons-push-block\n"))
	})
	// per docs/adr/0099-native-factory-orchestration.md:78
	It("feeds actual binary stdin to the direct-main gate", func() {
		f := newOrchestrationFixture()
		input := "refs/heads/topic abcd refs/heads/topic 0000\nwith NUL:\x00tail\n"
		result := f.check(input)
		Expect(result.status).To(BeZero(), "%+v", result)
		Expect(orchestrationRead(f.input)).To(Equal(input))
	})
	// per docs/adr/0099-native-factory-orchestration.md:75
	It("denies an actual main ref update before every other stage", func() {
		f := newOrchestrationFixture()
		writeFixture(filepath.Join(f.repo, "scripts/hooks/direct-main-push-block.sh"), orchestrationHistorical("scripts/hooks/direct-main-push-block.sh"), 0700)
		result := f.check("refs/heads/topic abcd refs/heads/main 0000\n")
		Expect(result.status).To(Equal(1), "%+v", result)
		Expect(result.stdout).To(ContainSubstring("DENY direct push to main"))
		Expect(result.stdout).NotTo(ContainSubstring("[2/8]"))
		Expect(orchestrationRead(f.trace)).To(BeEmpty())
	})
	// per docs/adr/0099-native-factory-orchestration.md:77
	It("continues ordinary failures and counts a failed commit range only once", func() {
		f := newOrchestrationFixture()
		for _, name := range []string{"commit-message-lint", "diff-aware-check", "decision-log-gate", "pending-lessons-push-block"} {
			f.gate(name, "printf 'gate failed\\n' >&2\nexit 7\n")
		}
		writeFixture(filepath.Join(f.repo, "local-a"), []byte("#!/bin/sh\nprintf 'local failure\\n'\nexit 8\n"), 0700)
		writeFixture(filepath.Join(f.repo, "factory.yaml"), []byte("local_hooks: local-a missing-local\n"), 0600)
		result := f.parity("", "CHECK_FAIL_MAKE_CHECK=2", "CHECK_FAIL_MAKE_DRIFT=3")
		Expect(result.status).To(Equal(1))
		Expect(result.stdout).To(ContainSubstring("PRE-PUSH: 8 CHECK(S) FAILED"))
		Expect(result.stdout).To(ContainSubstring("FAIL (2 commits checked, at least one violation)"))
		Expect(orchestrationRead(f.trace)).To(ContainSubstring("commit-message-lint\t" + f.second + "\ncommit-message-lint\t" + f.first))
		Expect(result.stdout).To(ContainSubstring("[8/8]"))
	})
	// per docs/adr/0099-native-factory-orchestration.md:87
	It("keeps missing-origin skips while diff-aware receives HEAD HEAD", func() {
		f := newOrchestrationFixture()
		orchestrationGit(f.repo, "update-ref", "-d", "refs/remotes/origin/main")
		result := f.check("")
		Expect(result.status).To(BeZero())
		Expect(strings.Count(result.stdout, "SKIP (no origin/main found)")).To(Equal(2))
		trace := orchestrationRead(f.trace)
		Expect(trace).To(ContainSubstring("diff-aware-check\tHEAD\tHEAD\n"))
		Expect(trace).NotTo(ContainSubstring("commit-message-lint"))
		Expect(trace).NotTo(ContainSubstring("decision-log-gate"))
	})
	// per docs/adr/0099-native-factory-orchestration.md:78
	It("reports an empty unpushed range without inventing a commit invocation", func() {
		f := newOrchestrationFixture()
		orchestrationGit(f.repo, "update-ref", "refs/remotes/origin/main", f.second)
		result := f.parity("")
		Expect(result.status).To(BeZero())
		Expect(result.stdout).To(ContainSubstring("SKIP (no unpushed commits)"))
		Expect(orchestrationRead(f.trace)).NotTo(ContainSubstring("commit-message-lint"))
	})
	// per docs/adr/0099-native-factory-orchestration.md:84
	DescribeTable("normalizes local hooks once and executes literal argv", func(raw, expected string, matching bool) {
		f := newOrchestrationFixture()
		writeFixture(filepath.Join(f.repo, "factory.yaml"), []byte("local_hooks: \""+raw+"\"\n"), 0600)
		for _, name := range []string{"local-a.sh", "local-b.sh"} {
			writeFixture(filepath.Join(f.repo, name), []byte("#!/bin/sh\nprintf '%s' \"${0##*/}\" >> \"$CHECK_TRACE\"\nfor argument do printf '\\t%s' \"$argument\" >> \"$CHECK_TRACE\"; done\nprintf '\\n' >> \"$CHECK_TRACE\"\n"), 0700)
		}
		if matching {
			writeFixture(filepath.Join(f.repo, "--matched.txt"), nil, 0600)
		}
		result := f.parity("")
		Expect(result.status).To(BeZero(), "%+v", result)
		Expect(orchestrationRead(f.trace)).To(HaveSuffix(expected))
		_, err := os.Stat(filepath.Join(f.repo, "never-created"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	},
		Entry("whitespace flags", "local-a.sh --strict -n local-b.sh --help", "local-a.sh\t--strict\t-n\nlocal-b.sh\t--help\n", false),
		Entry("comma positional arguments", "local-a.sh two words, local-b.sh --strict", "local-a.sh\ttwo\twords\nlocal-b.sh\t--strict\n", false),
		Entry("first-stage word glob is retained", "local-*.sh --strict", "local-a.sh\nlocal-b.sh\t--strict\n", false),
		Entry("comma argument glob stays literal after normalization", "local-a.sh --*.txt,local-b.sh", "local-a.sh\t--*.txt\nlocal-b.sh\n", true),
		Entry("shell syntax stays data", "local-a.sh $(touch never-created) ; & `touch never-created`,local-b.sh", "local-a.sh\t$(touch\tnever-created)\t;\t&\t`touch\tnever-created`\nlocal-b.sh\n", false),
	)
	// per docs/adr/0099-native-factory-orchestration.md:87
	DescribeTable("fails closed for an unavailable required gate", func(gate string) {
		f := newOrchestrationFixture()
		Expect(os.Remove(filepath.Join(f.repo, "scripts/hooks", gate+".sh"))).To(Succeed())
		result := f.check("")
		Expect(result.status).To(Equal(1), "%+v", result)
		Expect(result.stdout + result.stderr).To(ContainSubstring(gate))
		Expect(result.stdout).NotTo(ContainSubstring("ALL CHECKS PASSED"))
		if gate == "direct-main-push-block" {
			Expect(orchestrationRead(f.trace)).To(BeEmpty())
		} else {
			Expect(result.stdout).To(ContainSubstring("[8/8]"))
		}
	}, Entry("input gate", "direct-main-push-block"), Entry("ordinary gate", "diff-aware-check"))
	// per docs/adr/0099-native-factory-orchestration.md:79
	DescribeTable("preserves caller role presence and literal environment at child boundaries", func(role string, present bool) {
		f := newOrchestrationFixture()
		f.gate("pending-lessons-push-block", "printf '%s\\000%s\\000%s' \"${FACTORY_AGENT_ROLE+x}\" \"${FACTORY_AGENT_ROLE-}\" \"$CHECK_LITERAL\" > role-environment\n")
		environment := []string{"CHECK_LITERAL=literal $(touch never-created) value"}
		want := "\x00\x00literal $(touch never-created) value"
		if present {
			environment = append(environment, "FACTORY_AGENT_ROLE="+role)
			want = "x\x00" + role + "\x00literal $(touch never-created) value"
		}
		result := f.check("", environment...)
		Expect(result.status).To(BeZero(), "%+v", result)
		Expect(orchestrationRead(filepath.Join(f.repo, "role-environment"))).To(Equal(want))
		_, err := os.Stat(filepath.Join(f.repo, "never-created"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("implementer stays implementer", "implementer", true), Entry("explicit empty stays present", "", true), Entry("unset stays unset", "", false))
	// per docs/adr/0099-native-factory-orchestration.md:81
	It("runs adopter gates and Make from the caller Git root when invoked in a subdirectory", func() {
		f := newOrchestrationFixture()
		repo := f.repo
		f.gate("pending-lessons-push-block", "printf '%s' \"$PWD\" > gate-working-directory\n")
		f.repo = filepath.Join(repo, "nested caller")
		Expect(os.MkdirAll(f.repo, 0700)).To(Succeed())
		result := f.check("")
		Expect(result.status).To(BeZero(), "%+v", result)
		Expect(orchestrationRead(filepath.Join(repo, "gate-working-directory"))).To(Equal(repo))
		Expect(orchestrationRead(f.trace)).To(ContainSubstring("make\tcheck\nmake\tcheck-drift\n"))
	})
	// per docs/adr/0099-native-factory-orchestration.md:80
	It("stops after fatal cancellation and reaps its active gate", func() {
		f := newOrchestrationFixture()
		f.gate("direct-main-push-block", "printf '%s' \"$$\" > active-gate\nwhile :; do /bin/sleep 1; done\n")
		stdout, err := os.CreateTemp(filepath.Dir(f.repo), "cancel-output-")
		Expect(err).NotTo(HaveOccurred())
		defer stdout.Close()
		command := exec.Command(filepath.Join(f.root, "factory"), "check") // #nosec G204 -- test-owned compiled binary and fixed command.
		command.Dir, command.Env = f.repo, f.env
		command.Stdin, command.Stdout, command.Stderr = nil, stdout, stdout
		Expect(command.Start()).To(Succeed())
		done := make(chan error, 1)
		go func() { done <- command.Wait() }()
		waited := false
		DeferCleanup(func() {
			var pid int
			if _, err := fmt.Sscan(orchestrationRead(filepath.Join(f.repo, "active-gate")), &pid); err == nil && pid > 0 {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
			_ = command.Process.Kill()
			if !waited {
				select {
				case <-done:
				case <-time.After(time.Second):
				}
			}
		})
		Eventually(func() string { return orchestrationRead(filepath.Join(f.repo, "active-gate")) }, 3*time.Second, 20*time.Millisecond).ShouldNot(BeEmpty())
		Expect(command.Process.Signal(syscall.SIGINT)).To(Succeed())
		Eventually(done, 4*time.Second).Should(Receive(HaveOccurred()))
		waited = true
		Expect(orchestrationRead(f.trace)).To(Equal("direct-main-push-block\n"))
		pidText := orchestrationRead(filepath.Join(f.repo, "active-gate"))
		var pid int
		_, err = fmt.Sscan(pidText, &pid)
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() error { return syscall.Kill(pid, 0) }, 3*time.Second).Should(MatchError(syscall.ESRCH))
	})
})

func (f *orchestrationFixture) prereqTools() {
	GinkgoHelper()
	for name, version := range map[string]string{"git": "git version 2.50.1", "go": "go version go1.27.2 fixture/fixture", "opencode": "opencode fixture", "claude": "Claude Code fixture", "codex": "codex fixture", "golangci-lint": "golangci-lint fixture", "jq": "jq fixture"} {
		program := "#!/bin/sh\nprintf '%s\\t%s\\n' '" + name + "' \"$*\" >> \"$CHECK_TRACE\"\nprintf '%s\\n' '" + version + "'\n"
		if name != "go" {
			program += "printf '%s\\n' 'ignored second version line'\n"
		}
		writeFixture(filepath.Join(f.bin, name), []byte(program), 0700)
	}
	// No inherited executable directory is available, so absent tools are real
	// absence rather than probes accidentally satisfied by the developer machine.
	f.env = orchestrationEnvironment("PATH="+f.bin, "CHECK_TRACE="+f.trace, "FACTORY_CONFIG="+filepath.Join(f.repo, "factory.yaml"), "FACTORY_RUNTIME_BINARY="+filepath.Join(f.root, "factory"))
	for _, name := range []string{"python3", "bash", "curl"} {
		writeFixture(filepath.Join(f.bin, name), []byte("#!/bin/sh\nprintf 'UNEXPECTED_INTERPRETER_OR_TRANSPORT\\n' >&2\nexit 91\n"), 0700)
	}
}

var _ = Describe("Native orchestration prereq", func() {
	// per docs/adr/0099-native-factory-orchestration.md:90
	DescribeTable("checks the selected harness spelling and first version line without Python", func(harness, display string) {
		f := newOrchestrationFixture()
		f.prereqTools()
		writeFixture(filepath.Join(f.repo, "factory.yaml"), []byte("go_min_version: 1.27.2\n"), 0600)
		result := f.run("", []string{"FACTORY_HARNESS=" + harness}, filepath.Join(f.root, "factory"), "prereq")
		Expect(result.status).To(BeZero(), "%+v", result)
		Expect(result.stderr).To(BeEmpty())
		Expect(result.stdout).To(ContainSubstring("prereq: OK " + display))
		Expect(result.stdout).To(ContainSubstring("prereq: OK Go (go version go1.27.2 fixture/fixture)"))
		Expect(result.stdout).To(ContainSubstring("prereq-check: all tools present"))
		Expect(result.stdout).NotTo(ContainSubstring("ignored second version line"))
		trace := orchestrationRead(f.trace)
		Expect(trace).To(ContainSubstring(harness + "\t--version\n"))
		Expect(trace).NotTo(ContainSubstring("python"))
		for _, other := range []string{"opencode", "claude", "codex"} {
			if other != harness {
				Expect(trace).NotTo(ContainSubstring(other + "\t"))
			}
		}
	}, Entry("OpenCode", "opencode", "opencode"), Entry("Claude", "claude", "Claude Code"), Entry("Codex", "codex", "Codex"))
	// per docs/adr/0099-native-factory-orchestration.md:90
	DescribeTable("compares the configured Go minimum numerically", func(version, minimum string, status int) {
		f := newOrchestrationFixture()
		f.prereqTools()
		writeFixture(filepath.Join(f.repo, "factory.yaml"), []byte("go_min_version: "+minimum+"\n"), 0600)
		writeFixture(filepath.Join(f.bin, "go"), []byte("#!/bin/sh\nprintf '%s\\n' 'go version go"+version+" fixture/fixture'\n"), 0700)
		result := f.run("", nil, filepath.Join(f.root, "factory"), "prereq")
		Expect(result.status).To(Equal(status), "%+v", result)
		if status == 0 {
			Expect(result.stdout).To(ContainSubstring("prereq: OK Go"))
		} else {
			Expect(result.stdout).To(ContainSubstring("Go version " + version + " is below minimum security patch " + minimum))
		}
	}, Entry("equal patch", "1.27.2", "1.27.2", 0), Entry("numeric patch above nine", "1.27.10", "1.27.2", 0), Entry("newer minor", "1.28.0", "1.27.9", 0), Entry("older patch", "1.27.1", "1.27.2", 1), Entry("older minor", "1.9.99", "1.27.2", 1))
	// per docs/adr/0099-native-factory-orchestration.md:90
	It("skips Go entirely when no product Go minimum is configured", func() {
		f := newOrchestrationFixture()
		f.prereqTools()
		Expect(os.Remove(filepath.Join(f.bin, "go"))).To(Succeed())
		Expect(os.Remove(filepath.Join(f.bin, "python3"))).To(Succeed())
		result := f.run("", nil, filepath.Join(f.root, "factory"), "prereq")
		Expect(result.status).To(BeZero(), "%+v", result)
		Expect(result.stdout).To(ContainSubstring("skip: no go_min_version in factory.yaml (no Go pack installed)"))
		Expect(result.stdout).NotTo(ContainSubstring("Python"))
		Expect(orchestrationRead(f.trace)).NotTo(ContainSubstring("go\t"))
	})
	// per docs/adr/0099-native-factory-orchestration.md:92
	It("aggregates missing Git, configured Go, selected harness, linter and jq", func() {
		f := newOrchestrationFixture()
		f.prereqTools()
		writeFixture(filepath.Join(f.repo, "factory.yaml"), []byte("go_min_version: 1.27.2\n"), 0600)
		for _, name := range []string{"git", "go", "codex", "golangci-lint", "jq"} {
			Expect(os.Remove(filepath.Join(f.bin, name))).To(Succeed())
		}
		result := f.run("", []string{"FACTORY_HARNESS=codex"}, filepath.Join(f.root, "factory"), "prereq")
		Expect(result.status).To(Equal(1), "%+v", result)
		for _, name := range []string{"git", "Go", "Codex", "golangci-lint", "jq"} {
			Expect(result.stdout).To(ContainSubstring("PREREQ FAIL: " + name + " not found"))
		}
		Expect(result.stdout).To(ContainSubstring("prereq-check: 5 tool(s) missing or too old"))
		Expect(result.stdout).NotTo(ContainSubstring("python"))
	})
	// per docs/adr/0099-native-factory-orchestration.md:91
	It("rejects an unsupported literal harness while checking the remaining prerequisites", func() {
		f := newOrchestrationFixture()
		f.prereqTools()
		result := f.run("", []string{"FACTORY_HARNESS=$(touch never-created)"}, filepath.Join(f.root, "factory"), "prereq")
		Expect(result.status).To(Equal(1), "%+v", result)
		Expect(result.stdout).To(ContainSubstring("unsupported FACTORY_HARNESS=$(touch never-created)"))
		Expect(result.stdout).To(ContainSubstring("prereq: OK jq"))
		Expect(result.stdout).To(ContainSubstring("prereq-check: 1 tool(s) missing or too old"))
		_, err := os.Stat(filepath.Join(f.repo, "never-created"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0099-native-factory-orchestration.md:93
	It("accepts absent Python while retaining jq as a required hook prerequisite", func() {
		f := newOrchestrationFixture()
		f.prereqTools()
		for _, name := range []string{"python3", "jq"} {
			Expect(os.Remove(filepath.Join(f.bin, name))).To(Succeed())
		}
		result := f.run("", nil, filepath.Join(f.root, "factory"), "prereq")
		Expect(result.status).To(Equal(1), "%+v", result)
		Expect(result.stdout).To(ContainSubstring("PREREQ FAIL: jq not found (command: jq)"))
		Expect(result.stdout).To(ContainSubstring("prereq-check: 1 tool(s) missing or too old"))
		Expect(result.stdout).NotTo(ContainSubstring("python"))
	})
})
