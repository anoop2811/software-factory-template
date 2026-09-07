package acceptance_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

// This matrix is an independent compatibility oracle, not discovery from the
// candidate's registrations. per specs/001-go-runtime-conversion.md:295
var commandRoutes = []struct{ command, script string }{
	{"init", "scripts/factory-init.sh"},
	{"doctor", "scripts/factory-doctor.sh"},
	{"upgrade", "scripts/factory-upgrade.sh"},
	{"check", "scripts/pre-push-check.sh"},
	{"selftest", "scripts/selftest/run.sh"},
	{"report", "scripts/factory-report.sh"},
	{"budget", "scripts/factory-budget.sh"},
	{"loop", "scripts/factory-loop.sh"},
	{"metrics", "scripts/factory-metrics.sh"},
	{"review-lane", "scripts/factory-review-lane.sh"},
	{"migrate-config", "scripts/factory-migrate-config.sh"},
}

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
	// Build success is setup, not behavioral RED. per specs/001-go-runtime-conversion.md:289
	build := exec.Command("go", "build", "-o", filepath.Join(buildDir, "factory"), "./cmd/factory") // #nosec G204 -- test-owned output path and fixed candidate package.
	build.Dir = root
	output, err := build.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "compiled CLI setup failed: %s", output)
	candidateBinary, err = os.ReadFile(filepath.Join(buildDir, "factory"))
	Expect(err).NotTo(HaveOccurred())
})

func writeFixture(path string, contents []byte, mode os.FileMode) {
	GinkgoHelper()
	Expect(os.MkdirAll(filepath.Dir(path), 0755)).To(Succeed())
	Expect(os.WriteFile(path, contents, mode)).To(Succeed())
}

func fixture() (string, string) {
	GinkgoHelper()
	temp, err := os.MkdirTemp("", "factory acceptance ")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, temp)
	// macOS /var can be a symlink; compare the child's physical cwd explicitly.
	temp, err = filepath.EvalSymlinks(temp)
	Expect(err).NotTo(HaveOccurred())
	root := filepath.Join(temp, "factory root with spaces")
	cwd := filepath.Join(temp, "separate adopter cwd")
	Expect(os.MkdirAll(cwd, 0755)).To(Succeed())
	writeFixture(filepath.Join(root, "factory"), candidateBinary, 0755)
	writeFixture(filepath.Join(root, "legacy-factory"), legacyDispatcher, 0755)
	return root, cwd
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

var _ = Describe("The developer-built Cobra command boundary", func() {
	// per specs/001-go-runtime-conversion.md:295
	DescribeTable("preserves help aliases without adding framework commands", func(args []string) {
		root, cwd := fixture()
		baseline := invoke(root, cwd, "legacy-factory", "", args...)
		for _, executable := range []string{"legacy-factory", "factory"} {
			result := invoke(root, cwd, executable, "", args...)
			Expect(result).To(Equal(baseline), executable)
			Expect(result.status).To(Equal(0), executable)
			Expect(result.stderr).To(BeEmpty(), executable)
			Expect(result.stdout).To(ContainSubstring("Usage: factory <command> [args]"), executable)
			for _, route := range commandRoutes {
				Expect(result.stdout).To(ContainSubstring(route.command), executable)
			}
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
	DescribeTable("dispatches each public command preserving the complete subprocess boundary", func(command, script string) {
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
		Expect(invoke(root, cwd, "factory", input, argv...)).To(Equal(expected))
		_, err := os.Stat(filepath.Join(cwd, "never-created"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, routeEntries())

	// per specs/001-go-runtime-conversion.md:95
	DescribeTable("refuses missing and non-executable canonical scripts", func(command, script string) {
		root, cwd := fixture()
		path := filepath.Join(root, script)
		expected := cliResult{"", fmt.Sprintf("factory: '%s' is not available in this repo (%s is missing or not executable)\n", command, path), 2}
		for _, state := range []string{"missing", "non-executable"} {
			if state == "non-executable" {
				writeFixture(path, []byte("#!/bin/sh\necho must-not-run\n"), 0644)
			}
			Expect(invoke(root, cwd, "legacy-factory", "", command)).To(Equal(expected), state)
			Expect(invoke(root, cwd, "factory", "", command)).To(Equal(expected), state)
		}
	}, routeEntries())

	// per specs/001-go-runtime-conversion.md:297
	It("leaves successful piped JSON stdout free of CLI banners", func() {
		root, cwd := fixture()
		writeFixture(filepath.Join(root, "scripts/factory-report.sh"), []byte("#!/bin/sh\ncat\n"), 0755)
		input := "{\"budget\":null,\"runs\":[]}\n"
		expected := cliResult{input, "", 0}
		Expect(invoke(root, cwd, "legacy-factory", input, "report", "--json")).To(Equal(expected))
		Expect(invoke(root, cwd, "factory", input, "report", "--json")).To(Equal(expected))
	})

	// per specs/001-go-runtime-conversion.md:95
	It("retains Bash exec fallback for an executable script without a shebang", func() {
		root, cwd := fixture()
		writeFixture(filepath.Join(root, "scripts/factory-report.sh"), []byte("printf 'legacy script fallback\\n'\nprintf '%s\\000' \"$@\"\nexit 29\n"), 0755)
		args := []string{"report", "$(touch never-created)", "two words", ""}
		expected := cliResult{"legacy script fallback\n$(touch never-created)\x00two words\x00\x00", "", 29}
		Expect(invoke(root, cwd, "legacy-factory", "", args...)).To(Equal(expected))
		Expect(invoke(root, cwd, "factory", "", args...)).To(Equal(expected))
		_, err := os.Stat(filepath.Join(cwd, "never-created"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per specs/001-go-runtime-conversion.md:95
	DescribeTable("retains OS execution-failure status with a useful script diagnostic", func(kind string) {
		root, cwd := fixture()
		path := filepath.Join(root, "scripts/factory-report.sh")
		if kind == "directory" {
			Expect(os.MkdirAll(path, 0755)).To(Succeed())
		} else {
			writeFixture(path, []byte("#!/factory-acceptance-nonexistent-interpreter\n"), 0755)
		}
		baseline := invoke(root, cwd, "legacy-factory", "", "report")
		if kind == "directory" {
			Expect(baseline.status).To(Equal(126))
		} else {
			// Darwin's system Bash reports 1; newer Bash reports 127 for the
			// same missing interpreter. Require the actual local baseline status.
			Expect(baseline.status).To(BeElementOf(1, 127))
		}
		for _, executable := range []string{"legacy-factory", "factory"} {
			result := invoke(root, cwd, executable, "", "report")
			Expect(result.status).To(Equal(baseline.status), executable)
			Expect(result.stdout).To(BeEmpty(), executable)
			Expect(result.stderr).To(ContainSubstring("factory-report.sh"), executable)
			Expect(result.stderr).NotTo(ContainSubstring("Usage:"), executable)
			baselineDiagnostic, err := normalizeBashDiagnostic(baseline.stderr, filepath.Join(root, "legacy-factory"))
			Expect(err).NotTo(HaveOccurred())
			invocation := filepath.Join(root, "legacy-factory")
			if executable == "factory" {
				invocation = path
			}
			diagnostic, err := normalizeBashDiagnostic(result.stderr, invocation)
			Expect(err).NotTo(HaveOccurred())
			Expect(diagnostic).To(Equal(baselineDiagnostic), executable)
		}
	}, Entry("executable directory", "directory"), Entry("missing interpreter", "interpreter"))

	// per specs/001-go-runtime-conversion.md:95
	It("uses the invocation directory when the dispatcher is a symlink", func() {
		root, cwd := fixture()
		aliasRoot := filepath.Join(filepath.Dir(root), "symlink invocation root")
		Expect(os.MkdirAll(aliasRoot, 0755)).To(Succeed())
		for _, executable := range []string{"legacy-factory", "factory"} {
			Expect(os.Symlink(filepath.Join(root, executable), filepath.Join(aliasRoot, executable))).To(Succeed())
		}
		writeFixture(filepath.Join(root, "scripts/factory-report.sh"), []byte("#!/bin/sh\necho wrong-executable-target\nexit 99\n"), 0755)
		writeFixture(filepath.Join(aliasRoot, "scripts/factory-report.sh"), []byte("#!/bin/sh\nprintf 'invocation root\\n'\n"), 0755)
		expected := cliResult{"invocation root\n", "", 0}
		Expect(invoke(aliasRoot, cwd, "legacy-factory", "", "report")).To(Equal(expected))
		Expect(invoke(aliasRoot, cwd, "factory", "", "report")).To(Equal(expected))
	})

	// per specs/001-go-runtime-conversion.md:95
	DescribeTable("locates co-located scripts after PATH invocation", func(relative bool) {
		root, cwd := fixture()
		writeFixture(filepath.Join(root, "scripts/factory-report.sh"), []byte("#!/bin/sh\nprintf 'PATH invocation\\n'\n"), 0755)
		lookupRoot := root
		if relative {
			var err error
			lookupRoot, err = filepath.Rel(cwd, root)
			Expect(err).NotTo(HaveOccurred())
		}
		for _, executable := range []string{"legacy-factory", "factory"} {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "/usr/bin/env", "PATH="+lookupRoot+":/bin:/usr/bin", executable, "report") // #nosec G204 -- isolated fixture PATH; direct argv intentionally exercises caller lookup.
			cmd.Dir = cwd
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			Expect(cmd.Run()).To(Succeed(), "%s: %s", executable, stderr.String())
			Expect(stdout.String()).To(Equal("PATH invocation\n"))
			Expect(stderr.String()).To(BeEmpty())
		}
	}, Entry("absolute PATH entry", false), Entry("relative PATH entry", true))

	// per specs/001-go-runtime-conversion.md:95
	It("preserves signal termination instead of converting it into a normal error exit", func() {
		root, cwd := fixture()
		writeFixture(filepath.Join(root, "scripts/factory-report.sh"), []byte("#!/bin/sh\nkill -TERM \"$$\"\n"), 0755)
		for _, executable := range []string{"legacy-factory", "factory"} {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, filepath.Join(root, executable), "report") // #nosec G204 -- test-owned self-terminating fixture; no external process is signaled.
			cmd.Dir = cwd
			err := cmd.Run()
			Expect(ctx.Err()).NotTo(HaveOccurred())
			var exitError *exec.ExitError
			Expect(errors.As(err, &exitError)).To(BeTrue())
			status, ok := exitError.Sys().(syscall.WaitStatus)
			Expect(ok).To(BeTrue())
			Expect(status.Signaled()).To(BeTrue())
			Expect(status.Signal()).To(Equal(syscall.SIGTERM))
		}
	})

	// per specs/001-go-runtime-conversion.md:95
	It("replaces the dispatcher process with its script, preserving caller process ownership", func() {
		root, cwd := fixture()
		writeFixture(filepath.Join(root, "scripts/pre-push-check.sh"), []byte("#!/bin/sh\nprintf '%s' \"$$\"\n"), 0755)
		for _, executable := range []string{"legacy-factory", "factory"} {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, filepath.Join(root, executable), "check") // #nosec G204 -- isolated fixture; comparing PID requires a real process launch.
			cmd.Dir = cwd
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			Expect(cmd.Start()).To(Succeed())
			pid := cmd.Process.Pid
			Expect(cmd.Wait()).To(Succeed())
			Expect(stderr.String()).To(BeEmpty())
			Expect(stdout.String()).To(Equal(fmt.Sprint(pid)))
		}
	})
})
