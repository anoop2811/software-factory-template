package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

// This literal contract is independent of the proof implementation's inventory.
// per docs/adr/0120-native-source-gate-proof.md:29
var sourceSelftestGateNames = []string{
	"test-edit-denial", "commit-message-lint", "decision-log-gate", "diff-aware-check",
	"direct-main-push-block", "pending-lessons-push-block", "loop-close-check",
	"shared-script-enforcement", "hook-existence-check", "wiki-lint", "workflow-lint",
	"copy-manifest-check", "gate-instrumentation-check", "citation-lint",
}

type sourceSelftestFixture struct {
	root, caller, scratch, tools, forbidden string
	environment                             map[string]string
}

// Copy real source assets without discovering requirements from production code.
// per docs/adr/0120-native-source-gate-proof.md:78
func sourceSelftestCopyAssets(destination string) {
	GinkgoHelper()
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	for _, tree := range []string{"scripts", "eval/runners", ".githooks"} {
		origin := filepath.Join(repository, filepath.FromSlash(tree))
		Expect(filepath.WalkDir(origin, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(repository, path)
			if err != nil {
				return err
			}
			target := filepath.Join(destination, relative)
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return os.MkdirAll(target, info.Mode().Perm())
			}
			if info.Mode()&os.ModeSymlink != 0 {
				link, err := os.Readlink(path)
				if err != nil {
					return err
				}
				return os.Symlink(link, target) // #nosec G122 -- fixed evaluator checkout trees are quiescent; only link text is copied into a private fixture, without following the source link.
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("unexpected source fixture asset: %s", relative)
			}
			body, err := os.ReadFile(path) // #nosec G122 -- the input is a fixed, quiescent evaluator checkout tree; adversarial asset mutation begins only after fixture materialization.
			if err != nil {
				return err
			}
			return os.WriteFile(target, body, info.Mode().Perm()) // #nosec G703 -- fixed evaluator tree-relative paths are joined beneath a private destination; no external operand supplies a traversal component.
		})).To(Succeed())
	}
}

func newSourceSelftestFixture() sourceSelftestFixture {
	GinkgoHelper()
	root, caller := fixtureDirectories()
	work := filepath.Dir(root)
	f := sourceSelftestFixture{
		root: root, caller: caller,
		scratch:   filepath.Join(work, "private proof scratch"),
		tools:     filepath.Join(work, "qualified tools"),
		forbidden: filepath.Join(work, "FORBIDDEN_PROOF_EXECUTION"),
	}
	sourceSelftestCopyAssets(root)
	writeFixture(filepath.Join(root, "factory"), candidateBinary, 0755)
	Expect(os.MkdirAll(f.scratch, 0700)).To(Succeed())
	writeFixture(filepath.Join(f.scratch, "unrelated-sentinel"), []byte("caller-owned temporary data\n"), 0600)
	poison := []byte("#!/bin/bash\nprintf '%s\\n' \"$0\" >> \"$SOURCE_SELFTEST_FORBIDDEN\"\nexit 97\n")
	writeFixture(filepath.Join(root, "scripts/selftest/run.sh"), poison, 0755)
	writeFixture(filepath.Join(root, ".factory/bin/factory-runtime"), poison, 0700)
	writeFixture(filepath.Join(f.tools, "environment-selected-driver"), poison, 0700)
	for _, tool := range []string{"go", "python", "python3", "claude", "codex", "opencode", "curl", "wget", "gh", "npm", "npx"} {
		writeFixture(filepath.Join(f.tools, tool), poison, 0700)
	}
	realGit, err := exec.LookPath("git")
	Expect(err).NotTo(HaveOccurred())
	realGit, err = filepath.EvalSymlinks(realGit)
	Expect(err).NotTo(HaveOccurred())
	realGit, err = filepath.Abs(realGit)
	Expect(err).NotTo(HaveOccurred())
	writeFixture(filepath.Join(f.tools, "git"), []byte("#!/bin/bash\nexec \"$SOURCE_SELFTEST_REAL_GIT\" \"$@\"\n"), 0700)
	for _, directory := range []string{root, caller} {
		writeFixture(filepath.Join(directory, "factory.yaml"), []byte("test_file_patterns: '^never-a-proof-test$'\ncheck_command: false\nlocal_hooks: scripts/hooks/foreign-caller.sh\n"), 0600)
		writeFixture(filepath.Join(directory, ".factory/events.log"), []byte("old\tcaller-gate\tcaller reason\n"), 0600)
	}
	writeFixture(filepath.Join(caller, "scripts/hooks/test-edit-denial.sh"), poison, 0755)
	f.environment = map[string]string{
		"PATH":                      f.tools + string(os.PathListSeparator) + "/bin:/usr/bin",
		"TMPDIR":                    f.scratch,
		"SOURCE_SELFTEST_FORBIDDEN": f.forbidden,
		"SOURCE_SELFTEST_REAL_GIT":  realGit,
		"FACTORY_BRIDGE_PROTOCOL":   "",
		"FACTORY_CONFIG":            filepath.Join(caller, "factory.yaml"),
		"FACTORY_EVENT_LOG":         filepath.Join(caller, ".factory/events.log"),
		"FACTORY_RUNTIME_BINARY":    filepath.Join(f.tools, "environment-selected-driver"),
		"FACTORY_ASSET_ROOT":        caller,
		"FACTORY_AGENT_ROLE":        "implementer",
		"REPO_ROOT":                 caller,
		"GIT_CONFIG_NOSYSTEM":       "1",
		"GIT_CONFIG_GLOBAL":         os.DevNull,
		"BASH_ENV":                  "", "ENV": "", "LC_ALL": "C",
	}
	return f
}

func sourceSelftestEnvironment(overrides map[string]string) []string {
	result := []string{}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "FACTORY_") || strings.HasPrefix(key, "GIT_") {
			continue
		}
		if _, replaced := overrides[key]; !replaced {
			result = append(result, entry)
		}
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}

func (f sourceSelftestFixture) run(executable string, args ...string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, args...) // #nosec G204 -- evaluator-owned real CLI and literal public operands.
	command.Dir, command.Env = f.caller, sourceSelftestEnvironment(f.environment)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred(), "source selftest timed out: %s", stderr.String())
	status := 0
	if err != nil {
		var exit *exec.ExitError
		Expect(errors.As(err, &exit)).To(BeTrue(), "source CLI could not launch: %v", err)
		status = exit.ExitCode()
	}
	return cliResult{stdout.String(), stderr.String(), status}
}

func sourceSelftestExpectHealthy(result cliResult) {
	GinkgoHelper()
	Expect(result.status).To(BeZero(), "native source proof failed: %+v", result)
	Expect(result.stderr).To(BeEmpty())
	var expected strings.Builder
	for _, gate := range sourceSelftestGateNames {
		fmt.Fprintf(&expected, "proof: %s refused its violation and accepted its healthy control\n", gate)
	}
	expected.WriteString("selftest: 14 shipped gates proved, 0 failed\n")
	Expect(result.stdout).To(Equal(expected.String()), "status alone cannot establish any required gate proof")
}

func (f sourceSelftestFixture) expectClean() {
	GinkgoHelper()
	_, err := os.Lstat(f.forbidden)
	Expect(os.IsNotExist(err)).To(BeTrue(), "a compiler, Python, provider, downloader, legacy selftest or substituted driver ran")
	entries, err := os.ReadDir(f.scratch)
	Expect(err).NotTo(HaveOccurred())
	Expect(entries).To(HaveLen(1), "completed proof scratch must be removed without touching unrelated temporary data")
	Expect(entries[0].Name()).To(Equal("unrelated-sentinel"))
	body, err := os.ReadFile(filepath.Join(f.scratch, "unrelated-sentinel"))
	Expect(err).NotTo(HaveOccurred())
	Expect(string(body)).To(Equal("caller-owned temporary data\n"))
}

func sourceSelftestGatePath(root, gate string) string {
	if gate == "citation-lint" {
		return filepath.Join(root, "scripts/citation-lint.sh")
	}
	return filepath.Join(root, "scripts/hooks", gate+".sh")
}

// Only this native collaborator is replaced; the actual public CLI, asset
// custody, proof loop, output and cleanup still run. No process is invented.
// per docs/adr/0120-native-source-gate-proof.md:91
func sourceSelftestUncertainCLI(f sourceSelftestFixture, receipt string) string {
	fault := `{
 info, err := os.Lstat(root + "/..")
 if err != nil { return CommandResult{}, err }
 identity := info.Sys().(*syscall.Stat_t)
 receipt := fmt.Sprintf("%s\t%d\t%d\n", root, identity.Dev, identity.Ino)
 if err := os.WriteFile(os.Getenv("SOURCE_SELFTEST_OWNERSHIP_RECEIPT"), []byte(receipt), 0600); err != nil { return CommandResult{}, err }
 if os.Getenv("SOURCE_SELFTEST_UNCERTAIN_CANCEL") == "1" {
   if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil { return CommandResult{}, err }
   <-ctx.Done()
   if err := os.WriteFile(os.Getenv("SOURCE_SELFTEST_OWNERSHIP_RECEIPT")+".canceled", []byte("real context done"), 0600); err != nil { return CommandResult{}, err }
 }
 return CommandResult{}, &OwnershipError{ProcessPID: 8675309}
}`
	f.environment["SOURCE_SELFTEST_OWNERSHIP_RECEIPT"] = receipt
	return sourceSelftestNativeCLI(f, "uncertain-driver", []string{"fmt", "os", "syscall"}, fault)
}

// Overlays only the native invocation body; every untouched public controller
// and native supervision path is compiled from the current source.
// per docs/adr/0120-native-source-gate-proof.md:91
func sourceSelftestNativeCLI(f sourceSelftestFixture, name string, imports []string, fault string) string {
	GinkgoHelper()
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	original := filepath.Join(repository, "internal/native/command.go")
	body, err := os.ReadFile(original)
	Expect(err).NotTo(HaveOccurred())
	positions := token.NewFileSet()
	parsed, err := parser.ParseFile(positions, original, body, 0)
	Expect(err).NotTo(HaveOccurred())
	var invocation *ast.FuncDecl
	for _, declaration := range parsed.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == "ExecuteCommand" {
			invocation = function
		}
	}
	Expect(invocation).NotTo(BeNil(), "the fault must replace the native command invocation collaborator")
	start, end := positions.Position(invocation.Body.Pos()).Offset, positions.Position(invocation.Body.End()).Offset
	override := string(body[:start]) + fault + string(body[end:])
	packageEnd := positions.Position(parsed.Name.End()).Offset
	var declarations strings.Builder
	declarations.WriteString("\nimport (\n")
	for _, dependency := range imports {
		fmt.Fprintf(&declarations, "%q\n", dependency)
	}
	declarations.WriteString(")\n")
	override = override[:packageEnd] + declarations.String() + override[packageEnd:]
	return sourceSelftestOverlayCLI(f, name, map[string]string{original: override})
}

func sourceSelftestOverlayCLI(f sourceSelftestFixture, name string, sources map[string]string) string {
	GinkgoHelper()
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	area := filepath.Join(filepath.Dir(f.root), "native collaborator fault", name)
	replacements := map[string]string{}
	for original, contents := range sources {
		replacement := filepath.Join(area, filepath.Base(original))
		writeFixture(replacement, []byte(contents), 0600)
		replacements[original] = replacement
	}
	encoded, err := json.Marshal(map[string]any{"Replace": replacements})
	Expect(err).NotTo(HaveOccurred())
	overlay := filepath.Join(area, "overlay.json")
	writeFixture(overlay, encoded, 0600)
	compiler, err := exec.LookPath("go")
	Expect(err).NotTo(HaveOccurred())
	executable := filepath.Join(f.root, name)
	arguments := []string{"build", "-mod=readonly", "-buildvcs=false", "-overlay=" + overlay, "-o", executable, "./cmd/factory"}
	if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
		arguments = append(arguments[:1], append([]string{"-race"}, arguments[1:]...)...)
	}
	result := imageProcess(repository, nil, compiler, arguments...)
	Expect(result.status).To(BeZero(), "test-only collaborator setup failed: %+v", result)
	return executable
}

// Observe the actual publication call after its preceding context guards. The
// observer returns the identical *os.File so production output borrowing and
// polling remain real; only the scheduling boundary is controlled.
// per docs/adr/0120-native-source-gate-proof.md:123
func sourceSelftestPublicationCLI(f sourceSelftestFixture) string {
	GinkgoHelper()
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	original := filepath.Join(repository, "internal/installedproof/proof.go")
	body, err := os.ReadFile(original)
	Expect(err).NotTo(HaveOccurred())
	positions := token.NewFileSet()
	parsed, err := parser.ParseFile(positions, original, body, 0)
	Expect(err).NotTo(HaveOccurred())
	var target ast.Expr
	matches := 0
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		proof := false
		ast.Inspect(call, func(child ast.Node) bool {
			if literal, ok := child.(*ast.BasicLit); ok && literal.Kind == token.STRING {
				value, err := strconv.Unquote(literal.Value)
				proof = proof || err == nil && strings.HasPrefix(value, "proof: ")
			}
			return true
		})
		if proof {
			for _, argument := range call.Args {
				if name, ok := argument.(*ast.Ident); ok && name.Name == "out" {
					target = argument
					matches++
				}
			}
		}
		return true
	})
	Expect(matches).To(Equal(1), "observe only the real named-gate proof publication boundary")
	start, end := positions.Position(target.Pos()).Offset, positions.Position(target.End()).Offset
	override := string(body[:start]) + "sourceSelftestObservePublication(ctx, out)" + string(body[end:])
	override += `
func sourceSelftestObservePublication(ctx context.Context, writer io.Writer) io.Writer {
 receipt := fmt.Sprintf("%d\t%t\t%T\n", os.Getpid(), ctx.Err() == nil, writer)
 if err := os.WriteFile(os.Getenv("SOURCE_SELFTEST_OUTPUT_READY"), []byte(receipt), 0600); err != nil { panic(err) }
 <-ctx.Done()
 return writer
}
`
	return sourceSelftestOverlayCLI(f, "publication-driver", map[string]string{original: override})
}

func sourceSelftestFillPipe(writer *os.File) int {
	GinkgoHelper()
	descriptor := int(writer.Fd())
	Expect(syscall.SetNonblock(descriptor, true)).To(Succeed())
	filled := 0
	for _, payload := range [][]byte{[]byte(strings.Repeat("x", 4096)), []byte("x")} {
		for {
			count, err := syscall.Write(descriptor, payload)
			if count > 0 {
				filled += count
			}
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				break
			}
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(BeNumerically(">", 0))
		}
	}
	Expect(filled).To(BeNumerically(">", 0))
	Expect(syscall.SetNonblock(descriptor, false)).To(Succeed())
	return filled
}

// Complete the actual final healthy citation hook before selecting a native
// non-success outcome. Interrupted uses a real parent-only signal and waits for
// the actual CLI context; it does not manufacture a canceled context.
// per docs/adr/0120-native-source-gate-proof.md:115
const sourceSelftestFinalOutcome = `{
 result, err := executeCommand(ctx, root, argv, environment, allowance, false, nil)
 if err != nil || len(argv) == 0 || !strings.HasSuffix(argv[0], "/scripts/citation-lint.sh") || result.ExitCode == nil || *result.ExitCode != 0 { return result, err }
 original := result.Outcome
 outcome := os.Getenv("SOURCE_SELFTEST_NATIVE_OUTCOME")
 if outcome == "interrupted" {
   if ctx.Err() != nil { return result, errors.New("late-boundary fixture was already canceled") }
   if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil { return result, err }
   <-ctx.Done()
 }
 result.Outcome = outcome
 receipt, err := json.Marshal(map[string]any{"original_outcome":original,"returned_outcome":outcome,"exit_code":*result.ExitCode,"exit_confirmed":result.ExitConfirmed,"ownership_unconfirmed":result.OwnershipUnconfirmed,"context_done":ctx.Err()!=nil,"pid":os.Getpid(),"argv":argv,"root":root})
 if err != nil { return result, err }
 if err := os.WriteFile(os.Getenv("SOURCE_SELFTEST_NATIVE_RECEIPT"), receipt, 0600); err != nil { return result, err }
 return result, nil
}`

// Record the actual completed hook and context state before checking the
// public controller's outcome. The completed control uses the same overlay.
// per docs/adr/0120-native-source-gate-proof.md:115
func sourceSelftestOutcomeProbe(outcome string) cliResult {
	GinkgoHelper()
	f := newSourceSelftestFixture()
	receipt := filepath.Join(filepath.Dir(f.root), "final-native-outcome.json")
	f.environment["SOURCE_SELFTEST_NATIVE_OUTCOME"] = outcome
	f.environment["SOURCE_SELFTEST_NATIVE_RECEIPT"] = receipt
	executable := sourceSelftestNativeCLI(f, "native-outcome-driver", []string{"encoding/json", "os", "syscall"}, sourceSelftestFinalOutcome)
	sourceBefore, callerBefore := installationSnapshot(f.root), installationSnapshot(f.caller)
	result := f.run(executable, "selftest")
	body, err := os.ReadFile(receipt)
	Expect(err).NotTo(HaveOccurred(), "the actual final healthy hook must complete before the boundary fault")
	var observed map[string]any
	Expect(json.Unmarshal(body, &observed)).To(Succeed())
	Expect(observed["original_outcome"]).To(Equal("completed"))
	Expect(observed["returned_outcome"]).To(Equal(outcome))
	Expect(observed["exit_code"]).To(Equal(float64(0)))
	Expect(observed["exit_confirmed"]).To(BeTrue())
	Expect(observed["ownership_unconfirmed"]).To(BeFalse())
	Expect(observed["context_done"]).To(Equal(outcome == "interrupted"))
	Expect(observed["argv"]).To(Equal([]any{"./scripts/citation-lint.sh"}))
	AddReportEntry("source-selftest.final-native-boundary", observed, ReportEntryVisibilityNever)
	Expect(installationSnapshot(f.root)).To(Equal(sourceBefore))
	Expect(installationSnapshot(f.caller)).To(Equal(callerBefore))
	f.expectClean()
	return result
}

// Real Git retains the empty FIFO and both capture descriptors. The trap reaps
// it after the native owner signals the group, so readiness is not a sleep.
// per docs/adr/0120-native-source-gate-proof.md:88
const sourceSelftestHeldGit = `#!/bin/bash
set -eu
fifo="$SOURCE_SELFTEST_READY.fifo"
mkfifo "$fifo"
exec 9<>"$fifo"
"$SOURCE_SELFTEST_REAL_GIT" hash-object --stdin <&9 &
owned_git=$!
finish() {
  trap - INT TERM
  kill -TERM "$owned_git" 2>/dev/null || :
  wait "$owned_git" 2>/dev/null || :
  exec 9>&-
  rm -f "$fifo"
  exit "$1"
}
trap 'finish 130' INT
trap 'finish 143' TERM
group="$(/bin/ps -p "$$" -o pgid= | tr -d '[:space:]')"
printf '%s\t%s\t%s\t%s\n' "$$" "$owned_git" "$group" "$PWD" > "$SOURCE_SELFTEST_READY.part"
mv "$SOURCE_SELFTEST_READY.part" "$SOURCE_SELFTEST_READY"
wait "$owned_git"
finish 0
`

type sourceSelftestReceipt struct {
	Leader, Git, Group int
	Cwd                string
}

func sourceSelftestReadiness(path string) sourceSelftestReceipt {
	GinkgoHelper()
	var receipt sourceSelftestReceipt
	Eventually(func() bool {
		body, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		fields := strings.Split(strings.TrimSuffix(string(body), "\n"), "\t")
		if len(fields) != 4 {
			return false
		}
		values := make([]int, 3)
		for index := range values {
			values[index], err = strconv.Atoi(fields[index])
			if err != nil || values[index] <= 1 {
				return false
			}
		}
		receipt = sourceSelftestReceipt{values[0], values[1], values[2], fields[3]}
		if syscall.Kill(receipt.Leader, 0) != nil || syscall.Kill(receipt.Git, 0) != nil {
			return false
		}
		probe := exec.Command("/bin/ps", "-p", strconv.Itoa(receipt.Git), "-o", "command=") // #nosec G204 -- readiness inspects only the actual test-owned PID.
		observed, err := probe.Output()
		return err == nil && strings.Contains(string(observed), " hash-object --stdin")
	}, 10*time.Second, 20*time.Millisecond).Should(BeTrue(), "signal only after an actual real Git child has started and retained capture")
	for _, pid := range []int{receipt.Leader, receipt.Git} {
		group, err := syscall.Getpgid(pid)
		Expect(err).NotTo(HaveOccurred())
		Expect(group).To(Equal(receipt.Group))
	}
	Expect(receipt.Leader).To(Equal(receipt.Group), "the native child owns a separate process group")
	Expect(receipt.Git).NotTo(Equal(receipt.Leader))
	AddReportEntry("source-selftest.git-readiness", receipt, ReportEntryVisibilityNever)
	return receipt
}

func sourceSelftestCleanupGroup(receipt sourceSelftestReceipt) {
	if receipt.Group <= 1 || receipt.Group == syscall.Getpgrp() {
		return
	}
	for _, pid := range []int{receipt.Leader, receipt.Git} {
		if group, err := syscall.Getpgid(pid); err == nil && group == receipt.Group {
			_ = syscall.Kill(-receipt.Group, syscall.SIGKILL)
			return
		}
	}
}

var _ = Describe("Native source selftest gate proofs", func() {
	// per docs/adr/0120-native-source-gate-proof.md:10
	// per docs/adr/0120-native-source-gate-proof.md:55
	// per docs/adr/0120-native-source-gate-proof.md:78
	It("proves the fixed fourteen real gates without legacy execution or runtime construction", func() {
		f := newSourceSelftestFixture()
		sourceBefore, callerBefore := installationSnapshot(f.root), installationSnapshot(f.caller)
		started := time.Now()
		result := f.run(filepath.Join(f.root, "factory"), "selftest")
		AddReportEntry("source-selftest.healthy-duration", time.Since(started).String(), ReportEntryVisibilityNever)
		sourceSelftestExpectHealthy(result)
		Expect(installationSnapshot(f.root)).To(Equal(sourceBefore), "source assets/configuration/events must stay unchanged")
		Expect(installationSnapshot(f.caller)).To(Equal(callerBefore), "foreign caller assets/configuration/events must stay unchanged")
		f.expectClean()
	})

	// per docs/adr/0120-native-source-gate-proof.md:115
	DescribeTable("refuses a final healthy gate whose native result is not successful", func(outcome string) {
		result := sourceSelftestOutcomeProbe(outcome)
		Expect(result.status).To(Equal(1), "native %s must not become a successful source proof: %+v", outcome, result)
		Expect(result.stdout).NotTo(ContainSubstring("selftest: 14 shipped gates proved, 0 failed"))
	}, Entry("real parent SIGTERM after the final actual healthy hook completed", "interrupted"),
		Entry("native timeout while the caller context is still live", "timeout"),
		Entry("native output limit while the caller context is still live", "output_limit"))

	// per docs/adr/0120-native-source-gate-proof.md:117
	It("accepts a completed final healthy gate through the same live native-boundary overlay", func() {
		sourceSelftestExpectHealthy(sourceSelftestOutcomeProbe("completed"))
	})

	// per docs/adr/0120-native-source-gate-proof.md:123
	It("honors parent cancellation at proof publication when actual stdout is full", func() {
		f := newSourceSelftestFixture()
		ready := filepath.Join(filepath.Dir(f.root), "publication-ready")
		f.environment["SOURCE_SELFTEST_OUTPUT_READY"] = ready
		executable := sourceSelftestPublicationCLI(f)
		reader, writer, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = reader.Close(); _ = writer.Close() })
		filled := sourceSelftestFillPipe(writer)
		sourceBefore, callerBefore := installationSnapshot(f.root), installationSnapshot(f.caller)
		command := exec.Command(executable, "selftest") // #nosec G204 -- actual evaluator-owned public CLI, with only a scheduling observer in its publication boundary.
		command.Dir, command.Env = f.caller, sourceSelftestEnvironment(f.environment)
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		var stderr bytes.Buffer
		command.Stdout, command.Stderr = writer, &stderr
		Expect(command.Start()).To(Succeed())
		finished := make(chan error, 1)
		go func() { finished <- command.Wait() }()
		completed := false
		DeferCleanup(func() {
			if !completed {
				_ = command.Process.Kill()
				select {
				case <-finished:
				case <-time.After(2 * time.Second):
				}
			}
		})
		var fields []string
		Eventually(func() bool {
			body, err := os.ReadFile(ready)
			if err != nil {
				return false
			}
			fields = strings.Split(strings.TrimSuffix(string(body), "\n"), "\t")
			return len(fields) == 3
		}, 15*time.Second).Should(BeTrue(), "wait for the actual proof publication call after the real healthy hook completes")
		Expect(fields).To(Equal([]string{strconv.Itoa(command.Process.Pid), "true", "*os.File"}))
		AddReportEntry("source-selftest.full-stdout-boundary", map[string]any{"pid": command.Process.Pid, "filled_bytes": filled, "context_live_at_publication": true, "writer_type": fields[2]}, ReportEntryVisibilityNever)
		Expect(command.Process.Signal(syscall.SIGINT)).To(Succeed(), "signal only the real CLI parent at the observed publication boundary")
		var returned error
		Eventually(finished, 6*time.Second).Should(Receive(&returned), "a real full stdout pipe must not defeat native CLI cancellation")
		completed = true
		Expect(returned).To(HaveOccurred())
		Expect(command.ProcessState.ExitCode()).To(Equal(1), "%s", stderr.String())
		Expect(writer.Close()).To(Succeed())
		captured, err := io.ReadAll(reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(captured)).To(Equal(strings.Repeat("x", filled)), "canceled publication must not report a successful gate or aggregate")
		Expect(installationSnapshot(f.root)).To(Equal(sourceBefore))
		Expect(installationSnapshot(f.caller)).To(Equal(callerBefore))
		f.expectClean()
	})

	// per docs/adr/0120-native-source-gate-proof.md:84
	It("rejects both constant outcomes for every required gate and recovers after restoring all real gates", func() {
		f := newSourceSelftestFixture()
		sourceBefore, callerBefore := installationSnapshot(f.root), installationSnapshot(f.caller)
		started := time.Now()
		for _, gate := range sourceSelftestGateNames {
			path := sourceSelftestGatePath(f.root, gate)
			original, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			refusal := 1
			if gate == "test-edit-denial" {
				refusal = 2
			}
			for _, status := range []int{0, refusal} {
				receipt := filepath.Join(filepath.Dir(f.root), fmt.Sprintf("%s-outcome-%d", gate, status))
				f.environment["SOURCE_SELFTEST_GATE_RECEIPT"] = receipt
				writeFixture(path, []byte(fmt.Sprintf("#!/bin/bash\nprintf '%%s\\t%%s\\n' '%s' \"$PWD\" >> \"$SOURCE_SELFTEST_GATE_RECEIPT\"\nexit %d\n", gate, status)), 0755)
				result := f.run(filepath.Join(f.root, "factory"), "selftest")
				Expect(result.status).To(Equal(1), "constant outcome %d for %s must fail: %+v", status, gate, result)
				Expect(result.stderr).To(ContainSubstring(gate), "the real defective gate must cause the refusal")
				Expect(result.stdout).NotTo(ContainSubstring("proof: " + gate + " refused its violation"))
				Expect(result.stdout).NotTo(ContainSubstring("selftest: 14 shipped gates proved, 0 failed"))
				body, err := os.ReadFile(receipt)
				Expect(err).NotTo(HaveOccurred(), "the defective %s gate must actually execute", gate)
				invocations := strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
				expectedInvocations := 1
				if status != 0 {
					expectedInvocations = 2
				}
				Expect(invocations).To(HaveLen(expectedInvocations), "prove the real violation/healthy-control status boundary for %s", gate)
				for _, invocation := range invocations {
					fields := strings.Split(invocation, "\t")
					Expect(fields).To(HaveLen(2))
					Expect(fields[0]).To(Equal(gate))
					Expect(filepath.Base(fields[1])).To(Equal(gate))
					Expect(filepath.Dir(filepath.Dir(fields[1]))).To(Equal(f.scratch), "each real hook must execute inside its private proof scratch")
				}
				f.expectClean()
			}
			writeFixture(path, original, 0755)
		}
		sourceSelftestExpectHealthy(f.run(filepath.Join(f.root, "factory"), "selftest"))
		AddReportEntry("source-selftest.gate-fault-duration", time.Since(started).String(), ReportEntryVisibilityNever)
		Expect(installationSnapshot(f.root)).To(Equal(sourceBefore))
		Expect(installationSnapshot(f.caller)).To(Equal(callerBefore))
		f.expectClean()
	})

	// per docs/adr/0120-native-source-gate-proof.md:48
	// per docs/adr/0120-native-source-gate-proof.md:86
	DescribeTable("selects the invoked source assets from a foreign working directory", func(mode string) {
		f := newSourceSelftestFixture()
		selected := f.root
		var executable string
		arguments := []string{"selftest"}
		if mode == "symlink" {
			selected = filepath.Join(filepath.Dir(f.root), "logical source invocation")
			sourceSelftestCopyAssets(selected)
			poison, err := os.ReadFile(filepath.Join(f.root, "scripts/selftest/run.sh"))
			Expect(err).NotTo(HaveOccurred())
			writeFixture(filepath.Join(selected, "scripts/selftest/run.sh"), poison, 0755)
			writeFixture(filepath.Join(selected, ".factory/bin/factory-runtime"), poison, 0700)
			Expect(os.Symlink(filepath.Join(f.root, "factory"), filepath.Join(selected, "factory"))).To(Succeed())
			writeFixture(sourceSelftestGatePath(f.root, "test-edit-denial"), []byte("#!/bin/bash\nexit 0\n"), 0755)
			executable = filepath.Join(selected, "factory")
		} else {
			lookup := selected
			if mode == "relative PATH" {
				var err error
				lookup, err = filepath.Rel(f.caller, selected)
				Expect(err).NotTo(HaveOccurred())
			}
			f.environment["PATH"] = lookup + string(os.PathListSeparator) + f.environment["PATH"]
			executable, arguments = "/usr/bin/env", []string{"factory", "selftest"}
		}
		sourceBefore, selectedBefore, callerBefore := installationSnapshot(f.root), installationSnapshot(selected), installationSnapshot(f.caller)
		sourceSelftestExpectHealthy(f.run(executable, arguments...))
		Expect(installationSnapshot(f.root)).To(Equal(sourceBefore))
		Expect(installationSnapshot(selected)).To(Equal(selectedBefore))
		Expect(installationSnapshot(f.caller)).To(Equal(callerBefore))
		f.expectClean()
	}, Entry("symlink invocation keeps its logical source assets", "symlink"), Entry("absolute PATH invocation", "absolute PATH"), Entry("relative PATH invocation", "relative PATH"))

	// per docs/adr/0120-native-source-gate-proof.md:55
	It("uses the actual running executable beside hostile launcher and PATH driver lookalikes", func() {
		f := newSourceSelftestFixture()
		executable := filepath.Join(f.root, "factory-go")
		Expect(os.Rename(filepath.Join(f.root, "factory"), executable)).To(Succeed())
		poison, err := os.ReadFile(filepath.Join(f.root, "scripts/selftest/run.sh"))
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(f.root, "factory"), poison, 0755)
		writeFixture(filepath.Join(f.tools, "factory"), poison, 0755)
		sourceBefore, callerBefore := installationSnapshot(f.root), installationSnapshot(f.caller)
		sourceSelftestExpectHealthy(f.run(executable, "selftest"))
		Expect(installationSnapshot(f.root)).To(Equal(sourceBefore))
		Expect(installationSnapshot(f.caller)).To(Equal(callerBefore))
		f.expectClean()
	})

	// per docs/adr/0120-native-source-gate-proof.md:51
	DescribeTable("refuses missing or unsafe required source assets without a success summary", func(asset, state string) {
		f := newSourceSelftestFixture()
		path := filepath.Join(f.root, filepath.FromSlash(asset))
		original, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		switch state {
		case "nonexecutable":
			Expect(os.Chmod(path, 0600)).To(Succeed())
		case "writable":
			Expect(os.Chmod(path, 0666)).To(Succeed())
		default:
			Expect(os.Remove(path)).To(Succeed())
			switch state {
			case "symlink":
				outside := filepath.Join(filepath.Dir(f.root), "outside-required-asset")
				writeFixture(outside, original, 0755)
				Expect(os.Symlink(outside, path)).To(Succeed())
			case "directory":
				Expect(os.Mkdir(path, 0700)).To(Succeed())
			case "fifo":
				Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
			}
		}
		sourceBefore, callerBefore := installationSnapshot(f.root), installationSnapshot(f.caller)
		result := f.run(filepath.Join(f.root, "factory"), "selftest")
		Expect(result.status).To(Equal(1), "required %s asset %s must refuse: %+v", state, asset, result)
		Expect(result.stderr).NotTo(BeEmpty())
		Expect(result.stdout).NotTo(ContainSubstring("selftest: 14 shipped gates proved, 0 failed"))
		Expect(installationSnapshot(f.root)).To(Equal(sourceBefore))
		Expect(installationSnapshot(f.caller)).To(Equal(callerBefore))
		f.expectClean()
	}, Entry("missing required gate", "scripts/citation-lint.sh", "missing"), Entry("nonexecutable required gate", "scripts/citation-lint.sh", "nonexecutable"), Entry("linked required gate", "scripts/citation-lint.sh", "symlink"), Entry("directory required gate", "scripts/citation-lint.sh", "directory"), Entry("FIFO required gate", "scripts/citation-lint.sh", "fifo"), Entry("missing sourceable library", "scripts/lib/config.sh", "missing"), Entry("writable sourceable library", "scripts/lib/config.sh", "writable"))

	// per docs/adr/0120-native-source-gate-proof.md:55
	DescribeTable("refuses unsafe custody of its already-running source executable", func(state string) {
		f := newSourceSelftestFixture()
		executable := filepath.Join(f.root, "factory")
		if state == "writable" {
			Expect(os.Chmod(executable, 0777)).To(Succeed())
		} else {
			Expect(os.Link(executable, filepath.Join(filepath.Dir(f.root), "untrusted-driver-hardlink"))).To(Succeed())
		}
		sourceBefore, callerBefore := installationSnapshot(f.root), installationSnapshot(f.caller)
		result := f.run(executable, "selftest")
		Expect(result.status).To(Equal(1), "unsafe running executable must refuse: %+v", result)
		Expect(result.stderr).NotTo(BeEmpty())
		Expect(result.stdout).NotTo(ContainSubstring("selftest: 14 shipped gates proved, 0 failed"))
		Expect(installationSnapshot(f.root)).To(Equal(sourceBefore))
		Expect(installationSnapshot(f.caller)).To(Equal(callerBefore))
		f.expectClean()
	}, Entry("group/world-writable executable", "writable"), Entry("multiply-linked executable", "hardlink"))

	// per docs/adr/0120-native-source-gate-proof.md:69
	// per docs/adr/0120-native-source-gate-proof.md:88
	DescribeTable("handles a parent-only signal after its real owned Git child is ready", func(signal syscall.Signal, blockedStderr bool) {
		f := newSourceSelftestFixture()
		ready := filepath.Join(filepath.Dir(f.root), "actual-git-ready")
		f.environment["SOURCE_SELFTEST_READY"] = ready
		writeFixture(filepath.Join(f.tools, "git"), []byte(sourceSelftestHeldGit), 0700)
		unrelated := exec.Command("/bin/sleep", "30")
		unrelated.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		Expect(unrelated.Start()).To(Succeed())
		DeferCleanup(func() { _ = unrelated.Process.Kill(); _ = unrelated.Wait() })
		sourceBefore, callerBefore := installationSnapshot(f.root), installationSnapshot(f.caller)
		command := exec.Command(filepath.Join(f.root, "factory"), "selftest") // #nosec G204 -- actual evaluator-owned public CLI; only its parent receives the test signal.
		command.Dir, command.Env = f.caller, sourceSelftestEnvironment(f.environment)
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		var pipeReader, pipeWriter *os.File
		var descriptor uintptr
		var originalFlags, filled int
		var err error
		if blockedStderr {
			pipeReader, pipeWriter, err = os.Pipe()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = pipeReader.Close(); _ = pipeWriter.Close() })
			descriptor = pipeWriter.Fd()
			filled = sourceSelftestFillPipe(pipeWriter)
			// Compare the exact post-fill flags inherited by the child, using
			// the saved descriptor so a later Fd call cannot alter the observation.
			originalFlags, err = unix.FcntlInt(descriptor, unix.F_GETFL, 0)
			Expect(err).NotTo(HaveOccurred())
			command.Stderr = pipeWriter
		}
		Expect(command.Start()).To(Succeed())
		finished := make(chan error, 1)
		go func() { finished <- command.Wait() }()
		completed := false
		var receipt sourceSelftestReceipt
		DeferCleanup(func() {
			sourceSelftestCleanupGroup(receipt)
			if !completed {
				_ = command.Process.Kill()
				select {
				case <-finished:
				case <-time.After(2 * time.Second):
				}
			}
		})
		receipt = sourceSelftestReadiness(ready)
		parentGroup, err := syscall.Getpgid(command.Process.Pid)
		Expect(err).NotTo(HaveOccurred())
		Expect(receipt.Group).NotTo(Equal(parentGroup), "cancel the CLI, leaving native supervision responsible for its child group")
		relative, err := filepath.Rel(f.scratch, receipt.Cwd)
		Expect(err).NotTo(HaveOccurred())
		Expect(relative).NotTo(Equal(".."))
		Expect(relative).NotTo(HavePrefix(".." + string(filepath.Separator)))
		deadline := time.Now().Add(6 * time.Second)
		Expect(command.Process.Signal(signal)).To(Succeed())
		if blockedStderr {
			// Observe native reconciliation before testing caller exit, so a
			// full diagnostic pipe cannot masquerade as a still-running Git child.
			// per docs/adr/0120-native-source-gate-proof.md:138
			Eventually(func() error { return syscall.Kill(-receipt.Group, 0) }, 2*time.Second).Should(MatchError(syscall.ESRCH), "native children must reconcile before diagnostic publication")
			Expect(syscall.Kill(unrelated.Process.Pid, 0)).To(Succeed())
			Expect(installationSnapshot(f.root)).To(Equal(sourceBefore))
			Expect(installationSnapshot(f.caller)).To(Equal(callerBefore))
			Expect(os.ReadFile(filepath.Join(f.scratch, "unrelated-sentinel"))).To(Equal([]byte("caller-owned temporary data\n")))
			AddReportEntry("source-selftest.full-stderr-native-reconciled", map[string]any{"signal": signal.String(), "parent_pid": command.Process.Pid, "parent_alive": syscall.Kill(command.Process.Pid, 0) == nil, "owned_group": receipt.Group, "owned_group_absent": true, "filled_bytes": filled, "inherited_flags": originalFlags, "reader_idle": true, "source_and_caller_preserved": true, "unrelated_process_and_sentinel_preserved": true}, ReportEntryVisibilityNever)
		}
		allowance := 6 * time.Second
		if blockedStderr {
			allowance = time.Until(deadline)
			Expect(allowance).To(BeNumerically(">", 0))
		}
		var returned error
		Eventually(finished, allowance).Should(Receive(&returned), "parent-only cancellation must not wait for blocked Git input or diagnostic output")
		completed = true
		Expect(returned).To(HaveOccurred())
		Expect(command.ProcessState.ExitCode()).To(Equal(1), "cancellation is an operational failure: %s", stderr.String())
		Expect(stdout.String()).NotTo(ContainSubstring("selftest: 14 shipped gates proved, 0 failed"))
		if blockedStderr {
			restoredFlags, err := unix.FcntlInt(descriptor, unix.F_GETFL, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(restoredFlags).To(Equal(originalFlags), "bounded diagnostics must restore the inherited file description")
			Expect(pipeWriter.Close()).To(Succeed())
			captured, err := io.ReadAll(pipeReader)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(captured)).To(Equal(strings.Repeat("x", filled)), "the reader remains idle until the CLI is reaped")
			AddReportEntry("source-selftest.full-stderr-caller-reaped", map[string]any{"signal": signal.String(), "status": command.ProcessState.ExitCode(), "flags_before": originalFlags, "flags_after": restoredFlags, "filled_bytes": filled, "captured_bytes": len(captured)}, ReportEntryVisibilityNever)
		} else {
			Expect(stderr.String()).To(HavePrefix("factory selftest:"))
		}
		for _, pid := range []int{receipt.Leader, receipt.Git} {
			Eventually(func() error { return syscall.Kill(pid, 0) }, 2*time.Second).Should(MatchError(syscall.ESRCH), "owned PID %d survived", pid)
		}
		Eventually(func() error { return syscall.Kill(-receipt.Group, 0) }, 2*time.Second).Should(MatchError(syscall.ESRCH), "the owned process group must be gone")
		Expect(syscall.Kill(unrelated.Process.Pid, 0)).To(Succeed(), "unrelated process ownership must be preserved")
		Expect(installationSnapshot(f.root)).To(Equal(sourceBefore))
		Expect(installationSnapshot(f.caller)).To(Equal(callerBefore))
		f.expectClean()
	}, Entry("SIGINT", syscall.SIGINT, false), Entry("SIGTERM", syscall.SIGTERM, false), Entry("SIGINT with blocked stderr", syscall.SIGINT, true), Entry("SIGTERM with blocked stderr", syscall.SIGTERM, true))

	// per docs/adr/0120-native-source-gate-proof.md:71
	// per docs/adr/0120-native-source-gate-proof.md:91
	// per docs/adr/0120-native-source-gate-proof.md:115
	DescribeTable("retains the exact private scratch when the native collaborator reports uncertain ownership", func(canceled bool) {
		f := newSourceSelftestFixture()
		receipt := filepath.Join(filepath.Dir(f.root), "uncertain-native-identity")
		if canceled {
			f.environment["SOURCE_SELFTEST_UNCERTAIN_CANCEL"] = "1"
		}
		executable := sourceSelftestUncertainCLI(f, receipt)
		sourceBefore, callerBefore := installationSnapshot(f.root), installationSnapshot(f.caller)
		result := f.run(executable, "selftest")
		Expect(result.status).To(Equal(1), "uncertain ownership must refuse: %+v", result)
		Expect(result.stdout).NotTo(ContainSubstring("selftest: 14 shipped gates proved, 0 failed"))
		Expect(result.stderr).To(ContainSubstring("native process ownership is unconfirmed"))
		Expect(result.stderr).To(ContainSubstring("pid=8675309"))
		if canceled {
			marker, err := os.ReadFile(receipt + ".canceled")
			Expect(err).NotTo(HaveOccurred(), "the typed ownership error must survive an actual canceled CLI context")
			Expect(string(marker)).To(Equal("real context done"))
			AddReportEntry("source-selftest.uncertain-context-canceled", true, ReportEntryVisibilityNever)
		}
		body, err := os.ReadFile(receipt)
		Expect(err).NotTo(HaveOccurred(), "the deterministic collaborator must actually run")
		fields := strings.Split(strings.TrimSuffix(string(body), "\n"), "\t")
		Expect(fields).To(HaveLen(3))
		retained := filepath.Dir(fields[0])
		Expect(filepath.Dir(retained)).To(Equal(f.scratch))
		info, err := os.Lstat(retained)
		Expect(err).NotTo(HaveOccurred(), "uncertain ownership must preserve the scratch observed by the native collaborator")
		Expect(info.IsDir()).To(BeTrue())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0700)))
		identity, ok := info.Sys().(*syscall.Stat_t)
		Expect(ok).To(BeTrue())
		Expect(fmt.Sprint(identity.Dev)).To(Equal(fields[1]))
		Expect(fmt.Sprint(identity.Ino)).To(Equal(fields[2]))
		Expect(result.stderr).To(ContainSubstring(retained), "retained private evidence must be locatable")
		entries, err := os.ReadDir(f.scratch)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(2), "keep the original sentinel and only the uncertain proof workspace")
		_, err = os.Lstat(f.forbidden)
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(installationSnapshot(f.root)).To(Equal(sourceBefore))
		Expect(installationSnapshot(f.caller)).To(Equal(callerBefore))
		preserved, err := os.ReadFile(filepath.Join(f.scratch, "unrelated-sentinel"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(preserved)).To(Equal("caller-owned temporary data\n"))
	}, Entry("live CLI context", false), Entry("real parent cancellation with typed ownership uncertainty", true))
})
