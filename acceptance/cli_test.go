package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// This matrix is an immutable historical script-dispatch oracle, not discovery from the
// candidate's registrations. per specs/001-go-runtime-conversion.md:295
// Budget/loop have native public qualification: per docs/adr/0078-go-public-budget-loop.md:43
// Check now has a native orchestration contract: per docs/adr/0099-native-factory-orchestration.md:25
// Current owners refuse fallback: per docs/adr/0108-default-go-cutover.md:207
var commandRoutes = []struct{ command, script string }{
	{"upgrade", "scripts/factory-upgrade.sh"},
	{"selftest", "scripts/selftest/run.sh"},
}

// Independent expected public command inventory, including the new native routes.
// per docs/adr/0108-default-go-cutover.md:207
// per docs/adr/0099-native-factory-orchestration.md:22
// per docs/adr/0100-batched-review.md:23
const nativeCLIUsage = `factory — software factory control surface

Usage: factory <command> [args]

  init        Set up the factory in the current repo (writes factory.yaml, installs hooks)
  doctor      Report the health of an installed factory: armed/inert/stale gates,
              hook + adapter integrity, and a break/fix proof that every gate fires
  upgrade     Preview an authenticated Go update; apply with fresh --confirm and --quiescent
              Preserve your factory.yaml, content, history and customized files
              Preview locally with --dry-run --source PATH (run at installation root)
              Plan recovery with --plan-restore --migration-id ID (read-only; always blocked)
  sync        Synchronize harness adapters (all, opencode, claude, codex)
  prereq      Check required tools and configured Go minimum
  check       Run the full pre-push gate suite
  eval        Run structural or golden evaluation, or a selected harness runner
  review      Review a diff with the configured adversarial provider
  selftest    Run the break/fix self-test (watch every gate fire)
  review-batch Review complete PR diffs in bounded, budgeted model batches
  review-lane Enable or disable the advisory adversarial PR review (opt-in, costs tokens)
  metrics     What the factory is doing to this repo (--json, --html); local only
              (--html opens the page; --no-open just writes it)
  report      Cost report: gates enforced at 0 tokens, blocks caught, one labeled
              estimate (factory report --clear resets the block log)
  budget      Plan, bound, and report opt-in Codex/Claude/OpenCode invocations
  loop        Run manual checks or opt-in bounded implementation and repair
  migrate-config  Move a legacy factory.config into factory.yaml (--dry-run to preview)
  help        Show this message

Commands use the Go runtime with declared policy-gate and harness boundaries.
Use init --help or upgrade --help for release selection and explicit local-source options.
`

type cliResult struct {
	stdout, stderr string
	status         int
}

var candidateBinary, legacyDispatcher []byte

var _ = BeforeSuite(func() {
	root, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	legacy := exec.Command("git", "show", baselineCommit+":factory") // #nosec G204 -- immutable baseline revision; no shell evaluation.
	legacy.Dir = root
	legacyDispatcher, err = legacy.Output()
	Expect(err).NotTo(HaveOccurred())
	buildDir, err := os.MkdirTemp("", "factory-acceptance-build-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, buildDir)
	captureSourceCompilerImage(filepath.Join(buildDir, "source-compiler-helper"))
	// per docs/adr/0104-adaptive-qualification-workers.md:18
	sourceGateWorkerHelperPath = filepath.Join(buildDir, "worker-count")
	// Build success is setup, not behavioral RED. per specs/001-go-runtime-conversion.md:289
	buildArgs := []string{"build", "-o", filepath.Join(buildDir, "factory"), "./cmd/factory"}
	if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
		buildArgs = append([]string{"build", "-race"}, buildArgs[1:]...)
	}
	build := exec.Command("go", buildArgs...) // #nosec G204 -- test-owned output path and fixed candidate package.
	build.Dir = root
	output, err := build.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "compiled CLI setup failed: %s", output)
	candidateBinary, err = os.ReadFile(filepath.Join(buildDir, "factory"))
	Expect(err).NotTo(HaveOccurred())
})

func writeFixture(path string, contents []byte, mode os.FileMode) {
	GinkgoHelper()
	Expect(os.MkdirAll(filepath.Dir(path), 0755)).To(Succeed()) // #nosec G703 -- parent of the same test-owned temporary path as the adjacent WriteFile; hostile fixture operands never select a destination from an external request.
	Expect(os.WriteFile(path, contents, mode)).To(Succeed())    // #nosec G703 -- fixture callers supply test-owned temporary paths, including deliberate path-traversal cases; no external request selects this destination.
}

// Bare directories keep component-only fixtures independent without copying CLI
// executables they never invoke. per docs/adr/0102-parallel-go-qualification.md:51
func fixtureDirectories() (string, string) {
	GinkgoHelper()
	temp, err := os.MkdirTemp("", "factory acceptance ")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, temp)
	// macOS /var can be a symlink; compare the child's physical cwd explicitly.
	temp, err = filepath.EvalSymlinks(temp)
	Expect(err).NotTo(HaveOccurred())
	root := filepath.Join(temp, "factory root with spaces")
	cwd := filepath.Join(temp, "separate adopter cwd")
	Expect(os.MkdirAll(root, 0755)).To(Succeed())
	Expect(os.MkdirAll(cwd, 0755)).To(Succeed())
	return root, cwd
}

func fixture() (string, string) {
	GinkgoHelper()
	root, cwd := fixtureDirectories()
	writeCLIExecutables(root)
	return root, cwd
}

func writeCLIExecutables(root string) {
	GinkgoHelper()
	writeFixture(filepath.Join(root, "factory"), candidateBinary, 0755)
	writeFixture(filepath.Join(root, "legacy-factory"), legacyDispatcher, 0755)
}

func invoke(root, cwd, executable, input string, args ...string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(root, executable), args...) // #nosec G204 -- isolated test-owned executable; literal argv are the acceptance boundary.
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "FACTORY_ACCEPTANCE_VALUE=literal $(not-code) value")
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred(), "CLI timed out")
	status := 0
	if err != nil {
		var exitError *exec.ExitError
		Expect(errors.As(err, &exitError)).To(BeTrue(), "launch failed: %v", err)
		status = exitError.ExitCode()
	}
	return cliResult{stdout.String(), stderr.String(), status}
}

func routeEntries() []TableEntry {
	entries := make([]TableEntry, 0, len(commandRoutes))
	for _, route := range commandRoutes {
		entries = append(entries, Entry(route.command, route.command, route.script))
	}
	return entries
}

// Only Bash's known fixture invocation prefix and optional source-line framing
// differ for the candidate. Preserve every error path, word, line and newline.
// Raw framing remains a recorded compatibility limitation, not exact parity.
// per specs/001-go-runtime-conversion.md:460
func normalizeBashDiagnostic(stderr, invocation string) (string, error) {
	lines := strings.SplitAfter(stderr, "\n")
	lineContext := regexp.MustCompile(`^line [0-9]+: `)
	for i, line := range lines {
		if line == "" {
			continue
		}
		prefix := invocation + ": "
		if !strings.HasPrefix(line, prefix) {
			return "", fmt.Errorf("unexpected Bash diagnostic invocation prefix: %q", line)
		}
		lines[i] = lineContext.ReplaceAllString(strings.TrimPrefix(line, prefix), "")
	}
	return strings.Join(lines, ""), nil
}

// The old process-boundary cases remain historical evidence. Their current CLI
// counterpart must enter its native owner, never execute the adjacent script.
// per docs/adr/0108-default-go-cutover.md:207
func nativeOwnerRefusal(result cliResult, command string) {
	GinkgoHelper()
	Expect(result.stdout).To(BeEmpty())
	if command == "upgrade" {
		Expect(result).To(Equal(cliResult{"", "factory candidate: invalid candidate arguments\n", 2}))
		return
	}
	Expect(result.status).To(Equal(1), "%+v", result)
	Expect(result.stderr).To(HavePrefix("factory selftest:"))
	Expect(result.stderr).NotTo(ContainSubstring("selftest/run.sh"))
	Expect(strings.Count(result.stderr, "\n")).To(Equal(1))
}

// These intentionally incomplete source fixtures cannot pass the native gate
// proof. Refusal must preserve the fixture and caller, even with a legacy body.
// per docs/adr/0108-default-go-cutover.md:207
func assertNativeNoLegacy(root, cwd, command, input string, args ...string) {
	GinkgoHelper()
	if command == "upgrade" && len(args) == 0 {
		args = []string{"--PRIVATE_NOT_AN_UPGRADE_OPTION"}
	}
	beforeRoot, beforeCaller := assessmentTree(root), assessmentTree(cwd)
	nativeOwnerRefusal(invoke(root, cwd, "factory", input, append([]string{command}, args...)...), command)
	Expect(assessmentTree(root)).To(Equal(beforeRoot))
	Expect(assessmentTree(cwd)).To(Equal(beforeCaller))
}

// per docs/adr/0108-default-go-cutover.md:31
const nativeStableSelectionRefusal = "factory release: select an exact stable Go release; use --go-candidate for a prerelease or explicit rollback for a predecessor\n"

// A marker-shaped source/ref value belongs to that option. It cannot select a
// backup, recovery or preview owner and cannot activate the historical script.
// per docs/adr/0108-default-go-cutover.md:78
// per docs/adr/0108-default-go-cutover.md:207
func assertUpgradeRefusalWithoutLegacy(binary, root string, args []string, diagnostic string) {
	GinkgoHelper()
	before := assessmentTree(root)
	out := upgradePreview(binary, root, args...)
	Expect(out).To(Equal(cliResult{"", diagnostic, 2}))
	Expect(assessmentTree(root)).To(Equal(before))
}

var _ = Describe("The developer-built Cobra command boundary", func() {
	// per specs/001-go-runtime-conversion.md:295
	// per docs/adr/0099-native-factory-orchestration.md:22
	DescribeTable("preserves help aliases without adding framework commands", func(args []string) {
		root, cwd := fixture()
		baseline := invoke(root, cwd, "legacy-factory", "", args...)
		for _, executable := range []string{"legacy-factory", "factory"} {
			result := invoke(root, cwd, executable, "", args...)
			expected := baseline
			if executable == "factory" {
				expected.stdout = nativeCLIUsage
			}
			Expect(result).To(Equal(expected), executable)
			Expect(result.status).To(Equal(0), executable)
			Expect(result.stderr).To(BeEmpty(), executable)
			Expect(result.stdout).To(ContainSubstring("Usage: factory <command> [args]"), executable)
			for _, route := range commandRoutes {
				Expect(result.stdout).To(ContainSubstring(route.command), executable)
			}
			Expect(result.stdout).To(ContainSubstring("budget"), executable)
			Expect(result.stdout).To(ContainSubstring("loop"), executable)
			Expect(strings.ToLower(result.stdout)).NotTo(ContainSubstring("completion"), executable)
		}
	},
		Entry("no arguments", []string{}),
		Entry("empty command is implicit help", []string{""}),
		Entry("help", []string{"help"}),
		Entry("short help", []string{"-h"}),
		Entry("long help", []string{"--help"}),
		Entry("help ignores extra arguments as before", []string{"help", "unknown", "--bogus"}),
		Entry("top-level help ignores extra arguments as before", []string{"--help", "budget"}),
	)

	// per specs/001-go-runtime-conversion.md:298
	DescribeTable("rejects unknown first tokens with only the existing diagnostic", func(token string) {
		root, cwd := fixture()
		expected := cliResult{"", fmt.Sprintf("factory: unknown command '%s' (try: factory help)\n", token), 2}
		Expect(invoke(root, cwd, "legacy-factory", "", token)).To(Equal(expected))
		Expect(invoke(root, cwd, "factory", "", token)).To(Equal(expected))
	},
		Entry("unknown command", "not-a-command"),
		Entry("near-match has no Cobra suggestion", "budge"),
		Entry("completion is not a public command", "completion"),
		Entry("unknown flag", "--bogus"),
		Entry("version is not an existing flag", "--version"),
		Entry("standalone delimiter is not a command", "--"),
	)

	// per specs/001-go-runtime-conversion.md:298
	It("does not skip an unknown first flag to discover a later command", func() {
		root, cwd := fixture()
		expected := cliResult{"", "factory: unknown command '--bogus' (try: factory help)\n", 2}
		Expect(invoke(root, cwd, "legacy-factory", "", "--bogus", "budget")).To(Equal(expected))
		Expect(invoke(root, cwd, "factory", "", "--bogus", "budget")).To(Equal(expected))
	})

	// per specs/001-go-runtime-conversion.md:95
	// per docs/adr/0108-default-go-cutover.md:207
	DescribeTable("retains historical script boundaries while the current native owner refuses fallback", func(command, script string) {
		root, cwd := fixture()
		recorder := "#!/bin/sh\nprintf '%s\\000' '" + script + "' \"$PWD\" \"$FACTORY_ACCEPTANCE_VALUE\" \"$#\"\nprintf '%s\\000' \"$@\"\ncat\nprintf 'child diagnostic\\n' >&2\nexit 37\n"
		writeFixture(filepath.Join(root, script), []byte(recorder), 0755)
		// A script in the caller's directory must never shadow the candidate's root.
		writeFixture(filepath.Join(cwd, script), []byte("#!/bin/sh\necho wrong-root >&2\nexit 99\n"), 0755)
		args := []string{"", "two words", "--help", "-h", "--flag=value with spaces", "--", "$(touch never-created)", "quote'\"", "line\nbreak"}
		input := "{\"input\":true}\nwith NUL:\x00tail\n"
		fields := []string{script, cwd, "literal $(not-code) value", fmt.Sprint(len(args))}
		fields = append(fields, args...)
		expected := cliResult{strings.Join(fields, "\x00") + "\x00" + input, "child diagnostic\n", 37}
		argv := append([]string{command}, args...)
		Expect(invoke(root, cwd, "legacy-factory", input, argv...)).To(Equal(expected))
		assertNativeNoLegacy(root, cwd, command, input, args...)
		_, err := os.Stat(filepath.Join(cwd, "never-created"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, routeEntries())

	// per specs/001-go-runtime-conversion.md:95
	// per docs/adr/0108-default-go-cutover.md:207
	DescribeTable("retains historical script availability checks without requiring those scripts for native ownership", func(command, script string) {
		root, cwd := fixture()
		path := filepath.Join(root, script)
		expected := cliResult{"", fmt.Sprintf("factory: '%s' is not available in this repo (%s is missing or not executable)\n", command, path), 2}
		for _, state := range []string{"missing", "non-executable"} {
			if state == "non-executable" {
				writeFixture(path, []byte("#!/bin/sh\necho must-not-run\n"), 0644)
			}
			Expect(invoke(root, cwd, "legacy-factory", "", command)).To(Equal(expected), state)
			assertNativeNoLegacy(root, cwd, command, "")
		}
	}, routeEntries())

	// per specs/001-go-runtime-conversion.md:297
	It("leaves successful piped JSON stdout free of CLI banners", func() {
		root, cwd := fixture()
		writeFixture(filepath.Join(root, "scripts/selftest/run.sh"), []byte("#!/bin/sh\ncat\n"), 0755)
		input := "{\"budget\":null,\"runs\":[]}\n"
		expected := cliResult{input, "", 0}
		Expect(invoke(root, cwd, "legacy-factory", input, "selftest", "--json")).To(Equal(expected))
		assertNativeNoLegacy(root, cwd, "selftest", input, "--json")
		// Native machine-readable output is owned by its real JSON operation,
		// rather than an adjacent shell script echoing the caller's stdin.
		// per docs/adr/0078-go-public-budget-loop.md:29
		out := publicCommand(root, cwd, "budget", []string{"report", "--json"}, nil)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stderr).To(BeEmpty())
		decoder := json.NewDecoder(strings.NewReader(out.stdout))
		var object map[string]any
		Expect(decoder.Decode(&object)).To(Succeed())
		Expect(object).To(HaveKey("runs"))
		var extra any
		Expect(decoder.Decode(&extra)).To(Equal(io.EOF))
	})

	// per specs/001-go-runtime-conversion.md:95
	// per docs/adr/0108-default-go-cutover.md:207
	It("retains historical Bash no-shebang behavior without enabling native script fallback", func() {
		root, cwd := fixture()
		writeFixture(filepath.Join(root, "scripts/selftest/run.sh"), []byte("printf 'legacy script fallback\\n'\nprintf '%s\\000' \"$@\"\nexit 29\n"), 0755)
		args := []string{"selftest", "$(touch never-created)", "two words", ""}
		expected := cliResult{"legacy script fallback\n$(touch never-created)\x00two words\x00\x00", "", 29}
		Expect(invoke(root, cwd, "legacy-factory", "", args...)).To(Equal(expected))
		assertNativeNoLegacy(root, cwd, "selftest", "", args[1:]...)
		_, err := os.Stat(filepath.Join(cwd, "never-created"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per specs/001-go-runtime-conversion.md:95
	// per docs/adr/0108-default-go-cutover.md:207
	DescribeTable("retains historical script execution failures while native proof refuses incomplete source", func(kind string) {
		root, cwd := fixture()
		path := filepath.Join(root, "scripts/selftest/run.sh")
		if kind == "directory" {
			Expect(os.MkdirAll(path, 0755)).To(Succeed())
		} else {
			writeFixture(path, []byte("#!/factory-acceptance-nonexistent-interpreter\n"), 0755)
		}
		baseline := invoke(root, cwd, "legacy-factory", "", "selftest")
		if kind == "directory" {
			Expect(baseline.status).To(Equal(126))
		} else {
			// Darwin's system Bash reports 1; other observed Bash versions report
			// 126 or 127. Retain the observed platform-specific historical result.
			Expect(baseline.status).To(BeElementOf(1, 126, 127))
		}
		Expect(baseline.stdout).To(BeEmpty())
		Expect(baseline.stderr).To(ContainSubstring("selftest/run.sh"))
		Expect(baseline.stderr).NotTo(ContainSubstring("Usage:"))
		diagnostic, err := normalizeBashDiagnostic(baseline.stderr, filepath.Join(root, "legacy-factory"))
		Expect(err).NotTo(HaveOccurred())
		Expect(diagnostic).To(ContainSubstring(path))
		assertNativeNoLegacy(root, cwd, "selftest", "")
	}, Entry("executable directory", "directory"), Entry("missing interpreter", "interpreter"))

	// per specs/001-go-runtime-conversion.md:95
	// per docs/adr/0108-default-go-cutover.md:207
	It("retains historical symlink lookup without executing either legacy script in the native owner", func() {
		root, cwd := fixture()
		aliasRoot := filepath.Join(filepath.Dir(root), "symlink invocation root")
		Expect(os.MkdirAll(aliasRoot, 0755)).To(Succeed())
		for _, executable := range []string{"legacy-factory", "factory"} {
			Expect(os.Symlink(filepath.Join(root, executable), filepath.Join(aliasRoot, executable))).To(Succeed())
		}
		writeFixture(filepath.Join(root, "scripts/selftest/run.sh"), []byte("#!/bin/sh\necho wrong-executable-target\nexit 99\n"), 0755)
		writeFixture(filepath.Join(aliasRoot, "scripts/selftest/run.sh"), []byte("#!/bin/sh\nprintf 'invocation root\\n'\n"), 0755)
		expected := cliResult{"invocation root\n", "", 0}
		Expect(invoke(aliasRoot, cwd, "legacy-factory", "", "selftest")).To(Equal(expected))
		assertNativeNoLegacy(aliasRoot, cwd, "selftest", "")
	})

	// per specs/001-go-runtime-conversion.md:95
	// per docs/adr/0108-default-go-cutover.md:207
	DescribeTable("retains historical PATH lookup while selecting the native owner after current PATH invocation", func(relative bool) {
		root, cwd := fixture()
		writeFixture(filepath.Join(root, "scripts/selftest/run.sh"), []byte("#!/bin/sh\nprintf 'PATH invocation\\n'\n"), 0755)
		lookupRoot := root
		if relative {
			var err error
			lookupRoot, err = filepath.Rel(cwd, root)
			Expect(err).NotTo(HaveOccurred())
		}
		for _, executable := range []string{"legacy-factory", "factory"} {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "/usr/bin/env", "PATH="+lookupRoot+":/bin:/usr/bin", executable, "selftest") // #nosec G204 -- isolated fixture PATH; direct argv intentionally exercises caller lookup.
			cmd.Dir = cwd
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			Expect(ctx.Err()).NotTo(HaveOccurred())
			if executable == "legacy-factory" {
				Expect(err).NotTo(HaveOccurred(), "%s: %s", executable, stderr.String())
				Expect(stdout.String()).To(Equal("PATH invocation\n"))
				Expect(stderr.String()).To(BeEmpty())
			} else {
				var exited *exec.ExitError
				Expect(errors.As(err, &exited)).To(BeTrue())
				nativeOwnerRefusal(cliResult{stdout.String(), stderr.String(), exited.ExitCode()}, "selftest")
			}
		}
	}, Entry("absolute PATH entry", false), Entry("relative PATH entry", true))

	// per specs/001-go-runtime-conversion.md:95
	// per docs/adr/0108-default-go-cutover.md:207
	It("retains historical script signal termination without running that script through the native owner", func() {
		root, cwd := fixture()
		writeFixture(filepath.Join(root, "scripts/selftest/run.sh"), []byte("#!/bin/sh\nkill -TERM \"$$\"\n"), 0755)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, filepath.Join(root, "legacy-factory"), "selftest") // #nosec G204 -- test-owned self-terminating fixture; no external process is signaled.
		cmd.Dir = cwd
		err := cmd.Run()
		Expect(ctx.Err()).NotTo(HaveOccurred())
		var exitError *exec.ExitError
		Expect(errors.As(err, &exitError)).To(BeTrue())
		status, ok := exitError.Sys().(syscall.WaitStatus)
		Expect(ok).To(BeTrue())
		Expect(status.Signaled()).To(BeTrue())
		Expect(status.Signal()).To(Equal(syscall.SIGTERM))
		assertNativeNoLegacy(root, cwd, "selftest", "")
	})

	// per specs/001-go-runtime-conversion.md:95
	// per docs/adr/0108-default-go-cutover.md:207
	It("retains historical script process ownership without native dispatch to that script", func() {
		root, cwd := fixture()
		writeFixture(filepath.Join(root, "scripts/selftest/run.sh"), []byte("#!/bin/sh\nprintf '%s' \"$$\"\n"), 0755)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, filepath.Join(root, "legacy-factory"), "selftest") // #nosec G204 -- isolated fixture; comparing PID requires a real process launch.
		cmd.Dir = cwd
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		Expect(cmd.Start()).To(Succeed())
		pid := cmd.Process.Pid
		Expect(cmd.Wait()).To(Succeed())
		Expect(stderr.String()).To(BeEmpty())
		Expect(stdout.String()).To(Equal(fmt.Sprint(pid)))
		assertNativeNoLegacy(root, cwd, "selftest", "")
	})
})
