package acceptance_test

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const nativeCopyInventoryBaseline = "14c0d319a3bab6c0976bee334ae70a573e4d2501"

type nativeCopyInventoryFixture struct {
	root, caller, base, runtime, hook, event, forbidden string
	environment                                         map[string]string
}

// The immutable source is independent of the production metadata package.
// per docs/adr/0125-native-initializer-copy-inventory-gate.md:104
func newNativeCopyInventoryFixture() nativeCopyInventoryFixture {
	GinkgoHelper()
	root, caller := fixtureDirectories()
	base := filepath.Dir(root)
	literalRoot := filepath.Join(base, "inventory root ' $(touch NATIVE_COPY_ROOT_EXECUTED)")
	Expect(os.Rename(root, literalRoot)).To(Succeed())
	f := nativeCopyInventoryFixture{root: literalRoot, caller: caller, base: base, runtime: filepath.Join(base, "compiled runtime"), forbidden: filepath.Join(base, "forbidden-tool")}
	f.hook = filepath.Join(f.root, "scripts/hooks/copy-manifest-check.sh")
	f.event = filepath.Join(f.root, ".factory/events.log")
	archive := exec.Command("git", "archive", nativeCopyInventoryBaseline) // #nosec G204 -- immutable local revision, not a request-selected archive.
	archive.Dir = ".."
	data, err := archive.Output()
	Expect(err).NotTo(HaveOccurred())
	reader := tar.NewReader(bytes.NewReader(data))
	for {
		header, readErr := reader.Next()
		if errors.Is(readErr, io.EOF) {
			break
		}
		Expect(readErr).NotTo(HaveOccurred())
		if header.Typeflag == tar.TypeDir || header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		Expect(filepath.IsLocal(header.Name)).To(BeTrue())
		path := filepath.Join(f.root, header.Name) // #nosec G305 -- trusted immutable local Git archive; relative names are independently checked above.
		if header.Typeflag == tar.TypeSymlink {
			Expect(os.MkdirAll(filepath.Dir(path), 0700)).To(Succeed())
			Expect(os.Symlink(header.Linkname, path)).To(Succeed())
			continue
		}
		Expect(header.Typeflag).To(Equal(byte(tar.TypeReg)))
		body, bodyErr := io.ReadAll(reader)
		Expect(bodyErr).NotTo(HaveOccurred())
		writeFixture(path, body, os.FileMode(header.Mode&0777))
	}
	// Exercise the live gate against a source initializer that has become an adapter.
	data, err = os.ReadFile("../scripts/hooks/copy-manifest-check.sh")
	Expect(err).NotTo(HaveOccurred())
	writeFixture(f.hook, data, 0755)
	writeFixture(filepath.Join(f.root, "scripts/factory-init.sh"), []byte("#!/bin/sh\nexec \"$FACTORY_RUNTIME_BINARY\" init \"$@\"\n"), 0755)
	writeFixture(f.runtime, candidateBinary, 0700)
	writeFixture(f.event, []byte("old\tgate\tprior event\n"), 0600)
	writeFixture(filepath.Join(f.caller, "application.txt"), []byte("unrelated application content\n"), 0600)
	tools := filepath.Join(base, "qualified tools")
	qualifiedGit, err := exec.LookPath("git")
	Expect(err).NotTo(HaveOccurred())
	qualifiedGit, err = filepath.EvalSymlinks(qualifiedGit)
	Expect(err).NotTo(HaveOccurred())
	qualifiedGit, err = filepath.Abs(qualifiedGit)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.MkdirAll(tools, 0700)).To(Succeed())
	Expect(os.Symlink(qualifiedGit, filepath.Join(tools, "git"))).To(Succeed())
	for _, tool := range []string{"go", "python", "python3", "curl", "wget", "gh", "npm", "npx"} {
		writeFixture(filepath.Join(tools, tool), []byte("#!/bin/sh\nprintf '%s\\n' \"$0\" >> \"$NATIVE_COPY_FORBIDDEN\"\nexit 97\n"), 0700)
	}
	f.environment = map[string]string{
		"PATH": tools + string(os.PathListSeparator) + "/bin:/usr/bin", "HOME": base,
		"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull,
		"FACTORY_RUNTIME_BINARY": f.runtime, "FACTORY_BRIDGE_PROTOCOL": "",
		"FACTORY_EVENT_LOG": f.event, "FACTORY_CONFIG": filepath.Join(f.root, "factory.yaml"),
		"REPO_ROOT": f.caller, "NATIVE_COPY_FORBIDDEN": f.forbidden,
		"BASH_ENV": "", "ENV": "", "LC_ALL": "C", "GORACE": "atexit_sleep_ms=0",
	}
	imageGit(f.root, "init", "-q")
	imageGit(f.root, "config", "gc.auto", "0")
	imageGit(f.root, "config", "gc.autoDetach", "false")
	imageGit(f.root, "config", "maintenance.auto", "false")
	imageGit(f.root, "config", "core.fsmonitor", "false")
	imageGit(f.root, "add", "--force", "--all")
	imageGit(f.root, "commit", "-q", "-m", "chore: native copy inventory fixture")
	return f
}

func nativeCopyInventorySnapshot(root string) map[string]string {
	GinkgoHelper()
	state := installationSnapshot(root)
	delete(state, ".factory/events.log") // Only this named legacy gate-event log may change.
	return state
}

// Preserve bytes, modes, link text and the raw Git index around the real boundary.
// per docs/adr/0125-native-initializer-copy-inventory-gate.md:42
func (f nativeCopyInventoryFixture) observe(private bool) cliResult {
	GinkgoHelper()
	before, callerBefore := nativeCopyInventorySnapshot(f.root), installationSnapshot(f.caller)
	index, err := os.ReadFile(filepath.Join(f.root, ".git/index"))
	Expect(err).NotTo(HaveOccurred())
	runtimeBefore, err := os.ReadFile(f.runtime)
	Expect(err).NotTo(HaveOccurred())
	eventBefore, err := os.ReadFile(f.event)
	Expect(err).NotTo(HaveOccurred())
	environment := maps.Clone(f.environment)
	program, args := "/bin/bash", []string{f.hook, f.root}
	if private {
		environment["FACTORY_BRIDGE_PROTOCOL"] = "1"
		program, args = f.runtime, []string{"helper", "copies", f.root}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, program, args...) // #nosec G204 -- test-owned gate or compiled CLI, fixed protocol and literal subject root.
	command.Dir, command.Env = f.caller, sourceSelftestEnvironment(environment)
	command.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err = command.Run()
	if ctx.Err() != nil {
		holdFixtureCleanup(f.root, "copy inventory process exceeded its observation boundary")
	}
	Expect(ctx.Err()).NotTo(HaveOccurred(), "copy inventory process timed out: %s", stderr.String())
	status := 0
	if err != nil {
		var exit *exec.ExitError
		Expect(errors.As(err, &exit)).To(BeTrue(), "%v", err)
		status = exit.ExitCode()
	}
	after := nativeCopyInventorySnapshot(f.root)
	indexAfter, err := os.ReadFile(filepath.Join(f.root, ".git/index"))
	Expect(err).NotTo(HaveOccurred())
	runtimeAfter, err := os.ReadFile(f.runtime)
	Expect(err).NotTo(HaveOccurred())
	eventAfter, err := os.ReadFile(f.event)
	Expect(err).NotTo(HaveOccurred())
	subjectBefore, err := json.Marshal(before)
	Expect(err).NotTo(HaveOccurred())
	subjectAfter, err := json.Marshal(after)
	Expect(err).NotTo(HaveOccurred())
	AddReportEntry("native-copy-inventory.observation", map[string]any{"private": private, "status": status, "stdout": stdout.String(), "stderr": stderr.String(), "subject_preserved": reflect.DeepEqual(before, after), "index_preserved": bytes.Equal(index, indexAfter), "runtime_preserved": bytes.Equal(runtimeBefore, runtimeAfter), "named_event_log_changed": !bytes.Equal(eventBefore, eventAfter), "subject_before_sha256": fmt.Sprintf("%x", sha256.Sum256(subjectBefore)), "subject_after_sha256": fmt.Sprintf("%x", sha256.Sum256(subjectAfter)), "index_before_sha256": fmt.Sprintf("%x", sha256.Sum256(index)), "index_after_sha256": fmt.Sprintf("%x", sha256.Sum256(indexAfter))}, ReportEntryVisibilityNever)
	Expect(after).To(Equal(before), "only the explicitly named gate-event log may change")
	Expect(installationSnapshot(f.caller)).To(Equal(callerBefore))
	Expect(indexAfter).To(Equal(index))
	Expect(runtimeAfter).To(Equal(runtimeBefore))
	if private || status == 0 {
		Expect(eventAfter).To(Equal(eventBefore), "the native checker is read-only; only hook refusal logs an event")
	} else {
		Expect(string(eventAfter)).To(HavePrefix(string(eventBefore)))
		for _, line := range strings.Split(strings.TrimSpace(string(eventAfter[len(eventBefore):])), "\n") {
			if line != "" {
				Expect(strings.Split(line, "\t")).To(HaveLen(3))
				Expect(strings.Split(line, "\t")[1]).To(Equal("copy-manifest-check"))
			}
		}
	}
	_, forbiddenErr := os.Lstat(f.forbidden)
	Expect(os.IsNotExist(forbiddenErr)).To(BeTrue(), "inventory checking must not build, download or invoke Python")
	return cliResult{stdout.String(), stderr.String(), status}
}

// Hold the actual nested Git input rather than delaying or killing its controller.
// per docs/adr/0125-native-initializer-copy-inventory-gate.md:171
func nativeCopyInventoryCancelProof() {
	GinkgoHelper()
	f := newSourceSelftestFixture()
	ready := filepath.Join(filepath.Dir(f.root), "copy-inner-git-ready")
	f.environment["SOURCE_SELFTEST_READY"] = ready
	gate := "#!/bin/bash\nset -eu\nif [ \"${PWD##*/}\" = copy-manifest-check ]; then\n  for argument do\n    if [ \"$argument\" = ls-files ]; then\n" + strings.TrimPrefix(sourceSelftestHeldGit, "#!/bin/bash\n") + "\n    fi\n  done\nfi\nexec \"$SOURCE_SELFTEST_REAL_GIT\" \"$@\"\n"
	writeFixture(filepath.Join(f.tools, "git"), []byte(gate), 0700)
	outsider := exec.Command("/bin/sleep", "60")
	outsider.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	Expect(outsider.Start()).To(Succeed())
	DeferCleanup(func() { _ = outsider.Process.Kill(); _ = outsider.Wait() })
	sourceBefore, callerBefore := installationSnapshot(f.root), installationSnapshot(f.caller)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	DeferCleanup(cancel)
	command := exec.CommandContext(ctx, filepath.Join(f.root, "factory"), "selftest") // #nosec G204 -- evaluator-owned actual compiled CLI and fixed public command.
	command.Dir, command.Env = f.caller, sourceSelftestEnvironment(f.environment)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
	command.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	Expect(command.Start()).To(Succeed())
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	joined, settled := false, false
	var returned error
	var receipt sourceSelftestReceipt
	controllerPID, controllerGroup := 0, 0
	DeferCleanup(func() {
		if settled {
			return
		}
		if receipt.Group == 0 {
			// Recover only this test's published ownership if readiness validation failed.
			if data, err := os.ReadFile(ready); err == nil {
				_, _ = fmt.Sscanf(string(data), "%d\t%d\t%d", &receipt.Leader, &receipt.Git, &receipt.Group)
			}
		}
		for _, pid := range []int{receipt.Leader, receipt.Git} {
			if group, err := syscall.Getpgid(pid); pid > 1 && err == nil && group == receipt.Group && group != syscall.Getpgrp() {
				_ = syscall.Kill(-group, syscall.SIGTERM)
				break
			}
		}
		for attempt := 0; attempt < 100; attempt++ {
			if receipt.Group <= 1 || errors.Is(syscall.Kill(-receipt.Group, 0), syscall.ESRCH) {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		sourceSelftestCleanupGroup(receipt)
		if controllerPID > 1 && controllerGroup > 1 && controllerGroup != syscall.Getpgrp() {
			if group, err := syscall.Getpgid(controllerPID); err == nil && group == controllerGroup {
				_ = syscall.Kill(-group, syscall.SIGKILL)
			}
		}
		if !joined {
			_ = command.Process.Signal(syscall.SIGTERM)
			select {
			case returned = <-finished:
				joined = true
			case <-time.After(3 * time.Second):
				_ = command.Process.Kill()
				select {
				case returned = <-finished:
					joined = true
				case <-time.After(2 * time.Second):
				}
			}
		}
		closed := receipt.Group <= 1 || errors.Is(syscall.Kill(-receipt.Group, 0), syscall.ESRCH)
		if !joined || !closed {
			holdFixtureCleanup(f.scratch, "nested copy Git custody remains uncertain after test-owned cleanup")
		}
	})
	// The selected copy pair follows eleven complete gate pairs; stay within the
	// existing sixty-second source-selftest observation allowance.
	Eventually(func() bool { _, err := os.Stat(ready); return err == nil }, 30*time.Second, 20*time.Millisecond).Should(BeTrue())
	receipt = sourceSelftestReadiness(ready)
	parent := imageProcess(f.caller, nil, "/bin/ps", "-o", "ppid=", "-p", strconv.Itoa(receipt.Leader))
	Expect(parent.status).To(Equal(0))
	var err error
	controllerPID, err = strconv.Atoi(strings.TrimSpace(parent.stdout))
	Expect(err).NotTo(HaveOccurred())
	controller := imageProcess(f.caller, nil, "/bin/ps", "-o", "command=", "-p", strconv.Itoa(controllerPID))
	Expect(controller.status).To(Equal(0))
	Expect(controller.stdout).To(ContainSubstring(" helper copies "), "hold Git below the actual native copy controller, not an earlier proof Git command")
	controllerGroup, err = syscall.Getpgid(controllerPID)
	Expect(err).NotTo(HaveOccurred())
	mainGroup, err := syscall.Getpgid(command.Process.Pid)
	Expect(err).NotTo(HaveOccurred())
	Expect(controllerGroup).NotTo(Equal(mainGroup))
	Expect(receipt.Group).NotTo(Equal(controllerGroup))
	Expect(receipt.Group).NotTo(Equal(mainGroup))
	Expect(controllerPID).NotTo(Equal(command.Process.Pid))
	relative, err := filepath.Rel(f.scratch, receipt.Cwd)
	Expect(err).NotTo(HaveOccurred())
	Expect(relative).NotTo(Equal(".."))
	Expect(relative).NotTo(HavePrefix(".." + string(filepath.Separator)))
	input := filepath.Join(receipt.Cwd, ".opencode/package.json")
	inputBefore, err := os.ReadFile(input)
	Expect(err).NotTo(HaveOccurred())
	Expect(command.Process.Signal(syscall.SIGTERM)).To(Succeed(), "signal only the public CLI parent")
	Eventually(finished, 6*time.Second).Should(Receive(&returned))
	joined = true
	Expect(ctx.Err()).NotTo(HaveOccurred())
	Expect(returned).To(HaveOccurred())
	Expect(stdout.String()).NotTo(ContainSubstring("selftest: 14 shipped gates proved, 0 failed"))
	gitGone := errors.Is(syscall.Kill(-receipt.Group, 0), syscall.ESRCH)
	retained, retainErr := os.ReadFile(input)
	retainedExact := retainErr == nil && bytes.Equal(retained, inputBefore)
	AddReportEntry("native-copy-inventory.nested-cancellation", map[string]any{"parent_pid": command.Process.Pid, "controller_pid": controllerPID, "git_group": receipt.Group, "git_pid": receipt.Git, "parent_exit": command.ProcessState.ExitCode(), "nested_git_group_absent_before_teardown": gitGone, "proof_input_retained_exact_before_teardown": retainedExact, "proof_input": input, "stderr": stderr.String()}, ReportEntryVisibilityNever)
	Expect(gitGone || retainedExact).To(BeTrue(), "a canceled copy controller must close nested Git or retain its exact live proof inputs before teardown")
	if !gitGone {
		Expect(stderr.String()).To(ContainSubstring("preserve private proof evidence"), "live nested ownership must be reported as retained uncertainty")
	}
	Expect(syscall.Kill(outsider.Process.Pid, 0)).To(Succeed())
	Expect(installationSnapshot(f.root)).To(Equal(sourceBefore))
	Expect(installationSnapshot(f.caller)).To(Equal(callerBefore))
	Expect(os.ReadFile(filepath.Join(f.scratch, "unrelated-sentinel"))).To(Equal([]byte("caller-owned temporary data\n")))
	_, forbiddenErr := os.Lstat(f.forbidden)
	Expect(os.IsNotExist(forbiddenErr)).To(BeTrue())
	settled = gitGone && errors.Is(syscall.Kill(controllerPID, 0), syscall.ESRCH)
}

var _ = Describe("Native initializer copy inventory", func() {
	// per docs/adr/0125-native-initializer-copy-inventory-gate.md:104
	// per docs/adr/0125-native-initializer-copy-inventory-gate.md:35
	It("rejects a real untracked Go initializer input behind an adapter and accepts the same tracked input", func() {
		f := newNativeCopyInventoryFixture()
		const input = ".opencode/package.json"
		imageGit(f.root, "rm", "--cached", "--force", "--", input)
		data, err := os.ReadFile(filepath.Join(f.root, input))
		Expect(err).NotTo(HaveOccurred())
		Expect(data).NotTo(BeEmpty())
		untracked := imageProcess(f.root, nil, "git", "--literal-pathspecs", "ls-files", "--error-unmatch", "--", input)
		Expect(untracked.status).NotTo(Equal(0))
		result := f.observe(false)
		Expect(result.status).NotTo(Equal(0), "an empty Bash-copy scan must not certify an actual untracked Go input: %+v", result)
		imageGit(f.root, "--literal-pathspecs", "add", "--force", "--", input)
		Expect(f.observe(false).status).To(Equal(0))
		Expect(f.observe(true).status).To(Equal(0))
		// Literal Git pathspecs still match indexed descendants; require the file itself.
		imageGit(f.root, "rm", "--cached", "--force", "--", input)
		Expect(os.Remove(filepath.Join(f.root, input))).To(Succeed())
		child := input + "/indexed-child"
		writeFixture(filepath.Join(f.root, child), []byte("indexed descendant is not the initializer input\n"), 0600)
		imageGit(f.root, "--literal-pathspecs", "add", "--force", "--", child)
		Expect(os.RemoveAll(filepath.Join(f.root, input))).To(Succeed())
		writeFixture(filepath.Join(f.root, input), data, 0644)
		indexed := imageProcess(f.root, nil, "git", "--literal-pathspecs", "ls-files", "--error-unmatch", "-z", "--", input)
		Expect(indexed.status).To(Equal(0))
		Expect(indexed.stdout).To(Equal(child + "\x00"))
		AddReportEntry("native-copy-inventory.descendant-index", map[string]string{"required_regular_file": input, "actual_index_entry": child}, ReportEntryVisibilityNever)
		result = f.observe(false)
		Expect(result.status).NotTo(Equal(0), "tracking an indexed descendant must not certify its untracked regular-file parent: %+v", result)
		imageGit(f.root, "--literal-pathspecs", "add", "--force", "--", input)
		Expect(f.observe(true).status).To(Equal(0))
	})

	// per docs/adr/0125-native-initializer-copy-inventory-gate.md:35
	// per docs/adr/0125-native-initializer-copy-inventory-gate.md:42
	It("refuses missing required inputs README and unsafe file or parent types", func() {
		for _, scenario := range []string{"missing-required", "missing-readme", "directory", "symlink", "fifo", "parent-symlink"} {
			f := newNativeCopyInventoryFixture()
			Expect(f.observe(true).status).To(Equal(0), "healthy control before %s", scenario)
			path := filepath.Join(f.root, ".opencode/package.json")
			switch scenario {
			case "missing-readme":
				Expect(os.Remove(filepath.Join(f.root, "README.md"))).To(Succeed())
			case "parent-symlink":
				outside := filepath.Join(f.caller, "external opencode")
				Expect(os.Rename(filepath.Join(f.root, ".opencode"), outside)).To(Succeed())
				Expect(os.Symlink(outside, filepath.Join(f.root, ".opencode"))).To(Succeed())
			default:
				Expect(os.Remove(path)).To(Succeed())
				switch scenario {
				case "directory":
					Expect(os.Mkdir(path, 0700)).To(Succeed())
				case "symlink":
					outside := filepath.Join(f.caller, "outside input")
					writeFixture(outside, []byte("outside referent\n"), 0600)
					Expect(os.Symlink(outside, path)).To(Succeed())
				case "fifo":
					Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
				}
			}
			Expect(f.observe(true).status).NotTo(Equal(0), scenario)
		}
	})

	// per docs/adr/0125-native-initializer-copy-inventory-gate.md:35
	It("checks present optional inputs and matching group members while requiring nonempty groups", func() {
		f := newNativeCopyInventoryFixture()
		optional := filepath.Join(f.root, "wiki/README.md")
		body, err := os.ReadFile(optional)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Remove(optional)).To(Succeed())
		Expect(f.observe(true).status).To(Equal(0), "absent optional input remains optional")
		imageGit(f.root, "rm", "--cached", "--force", "--", "wiki/README.md")
		writeFixture(optional, body, 0644)
		Expect(f.observe(true).status).NotTo(Equal(0), "present optional input must be tracked")
		imageGit(f.root, "--literal-pathspecs", "add", "--force", "--", "wiki/README.md")
		Expect(f.observe(true).status).To(Equal(0))
		writeFixture(filepath.Join(f.root, "scripts/hooks/not-a-copy-input.txt"), []byte("not selected by the initializer\n"), 0600)
		Expect(f.observe(true).status).To(Equal(0), "nonmatching group files are outside the copy selection")
		member := "scripts/hooks/additional-copy-input.sh"
		writeFixture(filepath.Join(f.root, member), []byte("#!/bin/sh\nexit 0\n"), 0755)
		Expect(f.observe(true).status).NotTo(Equal(0))
		imageGit(f.root, "--literal-pathspecs", "add", "--force", "--", member)
		Expect(f.observe(true).status).To(Equal(0))
		for _, group := range [][2]string{{"scripts/hooks", ".sh"}, {".opencode/agent", ".md"}, {".codex/agents", ".toml"}} {
			empty := newNativeCopyInventoryFixture()
			entries, err := os.ReadDir(filepath.Join(empty.root, group[0]))
			Expect(err).NotTo(HaveOccurred())
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), group[1]) {
					Expect(os.RemoveAll(filepath.Join(empty.root, group[0], entry.Name()))).To(Succeed())
				}
			}
			Expect(empty.observe(true).status).NotTo(Equal(0), "required group %s must not be empty", group[0])
		}
	})

	// per docs/adr/0125-native-initializer-copy-inventory-gate.md:42
	It("keeps whitespace shell metacharacters and Git pathspec characters literal", func() {
		f := newNativeCopyInventoryFixture()
		writeFixture(filepath.Join(f.root, "factory.yaml"), []byte("$(touch NATIVE_COPY_CONFIG_EXECUTED)\n"), 0600)
		for _, name := range []string{"space and ' quote.sh", "line one\nline two.sh", "$(touch NATIVE_COPY_FILE_EXECUTED).sh", "-option-looking.sh", "back\\slash.sh"} {
			path := "scripts/hooks/" + name
			writeFixture(filepath.Join(f.root, path), []byte("#!/bin/sh\nexit 0\n"), 0755)
			imageGit(f.root, "--literal-pathspecs", "add", "--force", "--", path)
		}
		writeFixture(filepath.Join(f.root, "scripts/hooks/pathspec1.sh"), []byte("#!/bin/sh\nexit 0\n"), 0755)
		imageGit(f.root, "--literal-pathspecs", "add", "--force", "--", "scripts/hooks/pathspec1.sh")
		Expect(f.observe(true).status).To(Equal(0))
		literal := "scripts/hooks/pathspec[1].sh"
		writeFixture(filepath.Join(f.root, literal), []byte("#!/bin/sh\nexit 0\n"), 0755)
		Expect(f.observe(true).status).NotTo(Equal(0), "a tracked glob match is not tracking the literal input")
		imageGit(f.root, "--literal-pathspecs", "add", "--force", "--", literal)
		Expect(f.observe(false).status).To(Equal(0))
		for _, name := range []string{"NATIVE_COPY_CONFIG_EXECUTED", "NATIVE_COPY_FILE_EXECUTED", "NATIVE_COPY_ROOT_EXECUTED"} {
			for _, root := range []string{f.root, f.caller} {
				_, err := os.Lstat(filepath.Join(root, name))
				Expect(os.IsNotExist(err)).To(BeTrue(), name)
			}
		}
	})

	// per docs/adr/0125-native-initializer-copy-inventory-gate.md:57
	// per docs/adr/0125-native-initializer-copy-inventory-gate.md:62
	It("refuses invalid explicit runtimes and broken installed selectors without legacy fallback", func() {
		f := newNativeCopyInventoryFixture()
		writeFixture(filepath.Join(f.root, "scripts/factory-init.sh"), []byte("cp \"$TEMPLATE_DIR/AGENTS.md\" \"$OUTPUT\"\n"), 0755)
		inert := filepath.Join(f.base, "nonexecutable")
		writeFixture(inert, []byte("#!/bin/sh\nexit 0\n"), 0600)
		writeFixture(filepath.Join(f.caller, "relative-runtime"), candidateBinary, 0700)
		for _, selection := range []string{"", "./relative-runtime", filepath.Join(f.base, "missing"), inert, f.caller} {
			f.environment["FACTORY_RUNTIME_BINARY"] = selection
			Expect(f.observe(false).status).NotTo(Equal(0), "explicit selection %q must refuse", selection)
		}
		selected := filepath.Join(f.base, "explicit failing runtime")
		record := filepath.Join(f.base, "selected-argv")
		writeFixture(selected, []byte("#!/bin/sh\nprintf '%s\\000' \"$FACTORY_BRIDGE_PROTOCOL\" \"$@\" > \"$NATIVE_COPY_SELECTED_ARGV\"\nexit 63\n"), 0700)
		f.environment["FACTORY_RUNTIME_BINARY"], f.environment["NATIVE_COPY_SELECTED_ARGV"] = selected, record
		Expect(f.observe(false).status).NotTo(Equal(0))
		args, err := os.ReadFile(record)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Split(strings.TrimSuffix(string(args), "\x00"), "\x00")).To(Equal([]string{"1", "helper", "copies", f.root}))
		for _, kind := range []string{"file", "directory", "symlink"} {
			broken := newNativeCopyInventoryFixture()
			delete(broken.environment, "FACTORY_RUNTIME_BINARY")
			writeFixture(filepath.Join(broken.root, "scripts/factory-init.sh"), []byte("cp \"$TEMPLATE_DIR/AGENTS.md\" \"$OUTPUT\"\n"), 0755)
			writeFixture(filepath.Join(broken.root, ".factory/bin/factory-runtime"), candidateBinary, 0700)
			writeFixture(filepath.Join(broken.root, "factory"), []byte("#!/bin/bash\nroot=\"$(cd \"$(dirname \"${BASH_SOURCE[0]}\")\" && pwd -P)\"\nprintf called > \"$NATIVE_COPY_INSTALLED_CALLED\"\nexec \"$root/.factory/bin/factory-runtime\" \"$@\"\n"), 0755)
			called := filepath.Join(broken.base, "installed-called")
			broken.environment["NATIVE_COPY_INSTALLED_CALLED"] = called
			selector := filepath.Join(broken.root, ".factory/installation.current")
			switch kind {
			case "file":
				writeFixture(selector, []byte("not an installation descriptor\n"), 0600)
			case "directory":
				Expect(os.Mkdir(selector, 0700)).To(Succeed())
			case "symlink":
				Expect(os.Symlink(filepath.Join(broken.base, "missing selector"), selector)).To(Succeed())
			}
			Expect(broken.observe(false).status).NotTo(Equal(0), kind)
			Expect(os.ReadFile(called)).To(Equal([]byte("called")), "selector presence must reach physical installed admission")
		}
		owner, foreign := newNativeCopyInventoryFixture(), newNativeCopyInventoryFixture()
		foreign.hook = owner.hook
		delete(foreign.environment, "FACTORY_RUNTIME_BINARY")
		witness := filepath.Join(foreign.base, "UNTRUSTED_SUBJECT_FACTORY_EXECUTED")
		foreign.environment["NATIVE_COPY_SUBJECT_WITNESS"] = witness
		writeFixture(filepath.Join(foreign.root, ".factory/installation.current"), []byte("subject-supplied selector is not runtime consent\n"), 0600)
		writeFixture(filepath.Join(foreign.root, "factory"), []byte("#!/bin/bash\nprintf 'subject factory executed' > \"$NATIVE_COPY_SUBJECT_WITNESS\"\nexit 0\n"), 0755)
		result := foreign.observe(false)
		called, callErr := os.ReadFile(witness)
		AddReportEntry("native-copy-inventory.foreign-selector", map[string]any{"status": result.status, "subject_factory_executed": callErr == nil, "witness": string(called)}, ReportEntryVisibilityNever)
		Expect(os.IsNotExist(callErr)).To(BeTrue(), "a foreign subject selector must not authorize executing its factory")
		Expect(result.status).NotTo(Equal(0), "a source hook without a selected runtime cannot certify the foreign adapter's empty inventory")
		foreign.runtime = owner.runtime
		foreign.environment["FACTORY_RUNTIME_BINARY"] = owner.runtime
		Expect(foreign.observe(false).status).To(Equal(0), "an explicit trusted runtime can check the foreign source without executing its factory")
		imageGit(foreign.root, "rm", "--cached", "--force", "--", ".opencode/package.json")
		Expect(foreign.observe(false).status).NotTo(Equal(0))
		_, callErr = os.Lstat(witness)
		Expect(os.IsNotExist(callErr)).To(BeTrue())
	})

	// per docs/adr/0125-native-initializer-copy-inventory-gate.md:76
	It("retains genuine legacy guarded-copy and no-initializer behavior while refusing an empty inventory", func() {
		f := newNativeCopyInventoryFixture()
		delete(f.environment, "FACTORY_RUNTIME_BINARY")
		Expect(f.observe(false).status).NotTo(Equal(0), "an existing adapter without an inventory cannot become a successful legacy scan")
		writeFixture(filepath.Join(f.root, "scripts/factory-init.sh"), []byte("cp \"$TEMPLATE_DIR/AGENTS.md\" \"$OUTPUT\"\ncp \"$TEMPLATE_DIR/missing-optional\" \"$OUTPUT\" || true\n"), 0755)
		Expect(f.observe(false).status).To(Equal(0))
		imageGit(f.root, "rm", "--cached", "--force", "--", "AGENTS.md")
		Expect(f.observe(false).status).NotTo(Equal(0))
		Expect(os.Remove(filepath.Join(f.root, "scripts/factory-init.sh"))).To(Succeed())
		result := f.observe(false)
		Expect(result.status).To(Equal(0))
		Expect(result.stdout).To(ContainSubstring("skipping"))
	})

	// per docs/adr/0125-native-initializer-copy-inventory-gate.md:67
	It("requires physical installed admission for its own-root skip and still checks an explicit source subject", func() {
		installed := installedRuntimeCustodyFixture()
		before := installationSnapshot(installed.root)
		result := installedRuntimeInvoke(installed, "", []string{"FACTORY_BRIDGE_PROTOCOL=1"}, "helper", "copies", installed.root)
		Expect(result.status).To(Equal(0), "%+v", result)
		Expect(strings.ToLower(result.stdout)).To(ContainSubstring("installed"))
		Expect(strings.ToLower(result.stdout)).To(ContainSubstring("skip"))
		Expect(installationSnapshot(installed.root)).To(Equal(before))
		source := newNativeCopyInventoryFixture()
		imageGit(source.root, "rm", "--cached", "--force", "--", ".opencode/package.json")
		result = installedRuntimeInvoke(installed, "", []string{"FACTORY_BRIDGE_PROTOCOL=1"}, "helper", "copies", source.root)
		Expect(result.status).NotTo(Equal(0), "physical admission cannot skip an unrelated source inventory")
		imageGit(source.root, "--literal-pathspecs", "add", "--force", "--", ".opencode/package.json")
		Expect(installedRuntimeInvoke(installed, "", []string{"FACTORY_BRIDGE_PROTOCOL=1"}, "helper", "copies", source.root).status).To(Equal(0))
		imageGit(source.root, "rm", "--cached", "--force", "--", ".opencode/package.json")
		source.environment["FACTORY_INSTALLED_ROOT"] = source.root
		source.environment["FACTORY_ASSET_ROOT"] = installed.assets
		Expect(source.observe(true).status).NotTo(Equal(0), "caller environment markers do not grant installed authority")
		writeFixture(filepath.Join(installed.root, ".factory/bin/runtime.manifest"), []byte("tampered\n"), 0600)
		Expect(installedRuntimeInvoke(installed, "", []string{"FACTORY_BRIDGE_PROTOCOL=1"}, "helper", "copies", installed.root).status).NotTo(Equal(0))
	})

	// per docs/adr/0125-native-initializer-copy-inventory-gate.md:89
	// per docs/adr/0125-native-initializer-copy-inventory-gate.md:96
	It("proves a present real Go input untracked then tracked in the native fourteen-gate owner", func() {
		f := newSourceSelftestFixture()
		trace := filepath.Join(filepath.Dir(f.root), "native-copy-proof-observations")
		f.environment["NATIVE_COPY_PROOF_TRACE"] = trace
		path := filepath.Join(f.root, "scripts/hooks/copy-manifest-check.sh")
		body, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		probe := `
probe_root="${1:-$PWD}"
if [ ! -f "$probe_root/.opencode/package.json" ]; then
  printf 'missing\n' >> "$NATIVE_COPY_PROOF_TRACE"
elif ! git -C "$probe_root" cat-file -e HEAD:.opencode/package.json 2>/dev/null; then
  printf 'not-in-initial-commit\n' >> "$NATIVE_COPY_PROOF_TRACE"
elif git -C "$probe_root" --literal-pathspecs ls-files --error-unmatch -- .opencode/package.json >/dev/null 2>&1; then
  printf 'present-tracked\n' >> "$NATIVE_COPY_PROOF_TRACE"
else
  printf 'present-untracked\n' >> "$NATIVE_COPY_PROOF_TRACE"
fi
`
		first, rest, found := strings.Cut(string(body), "\n")
		Expect(found).To(BeTrue())
		writeFixture(path, []byte(first+"\n"+probe+rest), 0755)
		sourceBefore, callerBefore := installationSnapshot(f.root), installationSnapshot(f.caller)
		result := f.run(filepath.Join(f.root, "factory"), "selftest")
		sourceSelftestExpectHealthy(result)
		observed, err := os.ReadFile(trace)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Split(strings.TrimSpace(string(observed)), "\n")).To(Equal([]string{"present-untracked", "present-tracked"}))
		Expect(installationSnapshot(f.root)).To(Equal(sourceBefore))
		Expect(installationSnapshot(f.caller)).To(Equal(callerBefore))
		f.expectClean()
		nativeCopyInventoryCancelProof()
	})
})
