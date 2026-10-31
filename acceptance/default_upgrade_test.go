package acceptance_test

import (
	"bytes"
	"fmt"
	"io"
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
	"golang.org/x/sys/unix"
)

const defaultUpgradeTag = "v0.2.0"
const defaultUpgradePlanMarker = "INDEPENDENT_SELECTED_DEFAULT_UPGRADER_PLAN_20261031"
const defaultUpgradeApplyMarker = "INDEPENDENT_SELECTED_DEFAULT_UPGRADER_APPLY_20261031"

// A selected-only compiled planner and completion message distinguish ownership
// from a matching payload. Reuse the qualified image/transport fixture, not its
// production implementation as a behavioral oracle.
// per docs/adr/0122-default-packaged-go-upgrade.md:15
func prepareDefaultUpgrade() *defaultInitFixtures {
	return prepareDefaultInit(func(source string) []string {
		path := filepath.Join(source, "internal/installation/proposal.go")
		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		needle := `Checks: []string{"candidate-help", "installed-assets", "native-gate-proof", "state-compatibility"}`
		marked := strings.Replace(string(data), needle, `Checks: []string{"candidate-help", "installed-assets", "native-gate-proof", "state-compatibility", "`+defaultUpgradePlanMarker+`"}`, 1)
		Expect(marked).NotTo(Equal(string(data)))
		writeFixture(path, []byte(marked), 0644)
		path = filepath.Join(source, "internal/installation/transaction.go")
		data, err = os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		needle = "func (capability *Capability) Complete(ctx context.Context) error {"
		marked = strings.Replace(string(data), needle, needle+"\n _, _ = os.Stderr.WriteString("+fmt.Sprintf("%q", defaultUpgradeApplyMarker+"\n")+")", 1)
		Expect(marked).NotTo(Equal(string(data)))
		writeFixture(path, []byte(marked), 0644)
		return []string{"internal/installation/proposal.go", "internal/installation/transaction.go"}
	})
}

func defaultUpgradeArgs(extra ...string) []string {
	args := []string{"upgrade"}
	args = append(args, extra...)
	for _, input := range installationInputs {
		args = append(args, "--project-input", input)
	}
	return args
}

var _ = Describe("Go default packaged upgrade", Ordered, ContinueOnFailure, func() {
	var fixtures *defaultUpgradeJourneyFixtures
	BeforeAll(func() { fixtures = prepareDefaultUpgradeJourneys(true) })

	// per docs/adr/0122-default-packaged-go-upgrade.md:9
	// per docs/adr/0122-default-packaged-go-upgrade.md:15
	It("authenticates the declared stable default and previews a genuine legacy project through the selected planner", func() {
		project := fixtures.legacyProject()
		f, driver, root := project.f, project.driver, project.root
		snapshot := installationSnapshot(root)
		out := handoffNativeProcess(root, f.environment, "", driver, defaultUpgradeArgs("--dry-run", "--json")...)
		AddReportEntry("default-upgrade.public-preview", map[string]any{"status": out.status, "stdout": out.stdout, "stderr": out.stderr}, ReportEntryVisibilityNever)
		Expect(out.status).To(Equal(0), "ordinary dry-run without source must select the authenticated Go default: %+v", out)
		proposal := handoffProposal(out)
		Expect(proposal["profile"]).To(Equal("v0.1.6"))
		Expect(proposal["checks"]).To(ContainElement(defaultUpgradePlanMarker), "selected compiled planner must own the public result")
		Expect(handoffRecorded(f, "api")).To(ContainElement("repos/anoop2811/software-factory-template/commits/refs/tags/" + defaultUpgradeTag))
		Expect(handoffRecorded(f, "release-view")).To(ContainElement("tagName,isDraft,isPrerelease"))
		text := handoffNativeProcess(root, f.environment, "", driver, defaultUpgradeArgs()...)
		Expect(text.status).To(Equal(0), "%+v", text)
		Expect(text.stdout).To(ContainSubstring("Re-run the same Go executable from this repository with these arguments:\n"))
		Expect(text.stdout).To(ContainSubstring("'upgrade' '--ref' '" + defaultUpgradeTag + "' '--confirm' '" + proposal["proposal_digest"].(string) + "' '--quiescent'"))
		for _, input := range installationInputs {
			Expect(text.stdout).To(ContainSubstring("'--project-input' '" + input + "'"))
		}
		Expect(text.stdout).NotTo(ContainSubstring("--go-candidate"))
		Expect(text.stdout).NotTo(ContainSubstring("curl "))
		Expect(text.stdout).NotTo(ContainSubstring(f.root+string(filepath.Separator)), "normal cleanup must not invalidate its suggested executable")
		AddReportEntry("default-upgrade.public-advice", map[string]any{"stdout": text.stdout, "status": text.status}, ReportEntryVisibilityNever)
		Expect(installationSnapshot(root)).To(Equal(snapshot))
		defaultInitNoForbidden(f)
	})

	// per docs/adr/0122-default-packaged-go-upgrade.md:77
	It("requires fresh consent and restores exact legacy originals through inert backup and explicit rollback", func() {
		p := fixtures.legacyProject(defaultUpgradeNextTag)
		writeFixture(filepath.Join(p.root, "docs/operator-note.md"), []byte("preserve customized application history\n"), 0600)
		before := installationSnapshot(p.root)
		original, err := os.ReadFile(filepath.Join(p.root, "factory"))
		Expect(err).NotTo(HaveOccurred())
		head := imageProcess(p.root, nil, "git", "rev-parse", "--verify", "HEAD")
		index := imageProcess(p.root, nil, "git", "ls-files", "--stage", "-z")
		proposal := p.preview()
		for _, args := range [][]string{{"--confirm", proposal["proposal_digest"].(string)}, {"--confirm", strings.Repeat("0", 64), "--quiescent"}, {"--ref", defaultUpgradeNextTag, "--confirm", proposal["proposal_digest"].(string), "--quiescent"}} {
			out := p.run(args...)
			Expect(out.status).To(Equal(2), "fresh complete consent required: %+v", out)
			Expect(installationSnapshot(p.root)).To(Equal(before))
		}
		applied := p.apply(proposal)
		Expect(applied.status).To(Equal(0), "%+v", applied)
		Expect(applied.stderr).To(ContainSubstring(defaultUpgradeApplyMarker))
		defaultInitDescriptor(p.root, defaultUpgradeTag, fixtures.revision, fixtures.image(defaultUpgradeTag).binary)
		ids := retentionBackupIDs(p.root)
		Expect(ids).To(HaveLen(1))
		saved := filepath.Join(p.root, ".factory/backups", ids[0], "assets/factory")
		Expect(os.ReadFile(saved)).To(Equal(original))
		mode, err := os.Stat(saved)
		Expect(err).NotTo(HaveOccurred())
		Expect(mode.Mode().Perm() & 0111).To(BeZero())
		Expect(imageProcess(p.root, nil, "git", "check-ignore", "--no-index", "-q", "--", saved).status).To(Equal(0))
		Expect(handoffNativeProcess(p.root, p.f.environment, "", filepath.Join(p.root, "factory"), "selftest").status).To(Equal(0))
		defaultInitNoForbidden(p.f)
		rollbackArgs := fixtures.rollbackArgs(p, defaultUpgradeTag, ids[0])
		env := defaultUpgradeLegacyRecoveryEnvironment(p)
		reverse := handoffProposal(handoffNativeProcess(p.root, env, "", p.driver, append(append([]string{}, rollbackArgs...), "--dry-run", "--json")...))
		restored := handoffNativeProcess(p.root, env, "", p.driver, append(rollbackArgs, "--confirm", reverse["proposal_digest"].(string), "--quiescent")...)
		Expect(restored.status).To(Equal(0), "legacy recovery alone receives its provisioned historical tools: %+v", restored)
		after := installationSnapshot(p.root)
		for path, value := range before {
			if path != ".gitignore" && !strings.HasPrefix(path, ".git/") {
				Expect(after[path]).To(Equal(value), "restored original %s", path)
			}
		}
		Expect(imageProcess(p.root, nil, "git", "rev-parse", "--verify", "HEAD")).To(Equal(head))
		Expect(imageProcess(p.root, nil, "git", "ls-files", "--stage", "-z")).To(Equal(index))
		_, err = os.Lstat(filepath.Join(p.root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		AddReportEntry("default-upgrade.legacy-roundtrip", map[string]any{"migration": ids[0], "backup_executable": false, "rollback_status": restored.status, "legacy_tools": "provisioned only for historical rollback health"}, ReportEntryVisibilityNever)
	})

	// per docs/adr/0122-default-packaged-go-upgrade.md:52
	// per docs/adr/0122-default-packaged-go-upgrade.md:77
	DescribeTable("upgrades authenticated Go predecessors with literal inputs and exact rollback", func(beforeTag, afterTag string) {
		name := "Quoted 'project' $(touch injected)"
		p := fixtures.goProject(beforeTag, name)
		fixtures.provide(p.f, afterTag)
		history := []byte("{\"schema\":1,\"runs\":[],\"note\":\"retain modern history\"}\n")
		writeFixture(filepath.Join(p.root, ".factory/loops.json"), history, 0600)
		beforeDescriptor, err := os.ReadFile(filepath.Join(p.root, ".factory/installation.current"))
		Expect(err).NotTo(HaveOccurred())
		subdir := filepath.Join(p.root, "application/subdirectory")
		Expect(os.MkdirAll(subdir, 0700)).To(Succeed())
		snapshot := installationSnapshot(p.root)
		flags := p.args("--ref", afterTag, "--dry-run", "--json", "--")
		proposal := handoffProposal(handoffNativeProcess(subdir, p.f.environment, "", p.driver, flags...))
		Expect(proposal["profile"]).To(Equal("go-hybrid-v1"))
		Expect(proposal["checks"]).To(ContainElement(defaultUpgradePlanMarker))
		Expect(installationSnapshot(p.root)).To(Equal(snapshot))
		applied := p.apply(proposal, "--ref", afterTag)
		Expect(applied.status).To(Equal(0), "%+v", applied)
		Expect(applied.stderr).To(ContainSubstring(defaultUpgradeApplyMarker))
		defaultInitDescriptor(p.root, afterTag, fixtures.revision, fixtures.image(afterTag).binary)
		ids := retentionBackupIDs(p.root)
		Expect(ids).To(HaveLen(1))
		rollback := fixtures.rollbackArgs(p, afterTag, ids[0])
		reverse := handoffProposal(handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(append([]string{}, rollback...), "--dry-run", "--json")...))
		restored := handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(rollback, "--confirm", reverse["proposal_digest"].(string), "--quiescent")...)
		Expect(restored.status).To(Equal(0), "%+v", restored)
		Expect(os.ReadFile(filepath.Join(p.root, ".factory/bin/factory-runtime"))).To(Equal(p.before.binary))
		Expect(os.ReadFile(filepath.Join(p.root, ".factory/installation.current"))).To(Equal(beforeDescriptor))
		Expect(os.ReadFile(filepath.Join(p.root, ".factory/loops.json"))).To(Equal(history))
		for _, path := range []string{filepath.Join(p.root, "injected"), filepath.Join(subdir, "injected")} {
			_, err := os.Lstat(path)
			Expect(os.IsNotExist(err)).To(BeTrue(), "literal input must not execute %s", path)
		}
		defaultInitNoForbidden(p.f)
		AddReportEntry("default-upgrade.go-roundtrip", map[string]any{"before": beforeTag, "after": afterTag, "literal_project_name": name, "rollback_status": restored.status}, ReportEntryVisibilityNever)
	}, Entry("stable official predecessor", defaultUpgradeTag, defaultUpgradeNextTag), Entry("official prerelease predecessor", defaultUpgradeBeforeRC, defaultUpgradeTag))

	// per docs/adr/0122-default-packaged-go-upgrade.md:84
	DescribeTable("reconciles real acquisition children and bounds cancellation diagnostics", func(blockedStderr bool) {
		f, driver := fixtures.surface(defaultUpgradeTag)
		imageGit(f.cwd, "init", "--quiet")
		writeFixture(filepath.Join(f.cwd, "application.txt"), []byte("caller application data\n"), 0600)
		ready := filepath.Join(f.records, "actual-acquisition-git-ready")
		git, err := exec.LookPath("git")
		Expect(err).NotTo(HaveOccurred())
		git, err = filepath.EvalSymlinks(git)
		Expect(err).NotTo(HaveOccurred())
		f.environment["SOURCE_SELFTEST_READY"], f.environment["SOURCE_SELFTEST_REAL_GIT"] = ready, git
		held := strings.Replace(sourceSelftestHeldGit, "set -eu\n", "set -eu\nprintf '%s\\000' \"$@\" > \"$HANDOFF_RECORD_DIR/held-gh-arguments\"\n", 1)
		writeFixture(filepath.Join(f.root, "host tools/gh"), []byte(held), 0700)
		unrelated := exec.Command("/bin/sleep", "30")
		unrelated.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		Expect(unrelated.Start()).To(Succeed())
		DeferCleanup(func() { _ = unrelated.Process.Kill(); _ = unrelated.Wait() })
		sentinel := filepath.Join(f.root, "unrelated-sentinel")
		writeFixture(sentinel, []byte("preserve unrelated cache data\n"), 0600)
		projectBefore, sourceBefore := installationSnapshot(f.cwd), installationSnapshot(f.environment["FACTORY_HOME"])
		command := exec.Command(driver, defaultUpgradeArgs("--dry-run", "--json")...) // #nosec G204 -- real evaluator-owned public Go CLI; literal release-preview operands.
		command.Dir, command.Env = f.cwd, sourceSelftestEnvironment(f.environment)
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		var reader, writer *os.File
		var descriptor uintptr
		var flags, filled int
		if blockedStderr {
			reader, writer, err = os.Pipe()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = reader.Close(); _ = writer.Close() })
			descriptor = writer.Fd()
			filled = sourceSelftestFillPipe(writer)
			flags, err = unix.FcntlInt(descriptor, unix.F_GETFL, 0)
			Expect(err).NotTo(HaveOccurred())
			command.Stderr = writer
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
		Expect(handoffRecorded(f, "held-gh-arguments")[:2]).To(Equal([]string{"api", "repos/anoop2811/software-factory-template/commits/refs/tags/" + defaultUpgradeTag}))
		Expect(receipt.Cwd).To(HavePrefix(f.root+string(filepath.Separator)), "the held real Git belongs to the private acquisition, not the caller")
		parentGroup, err := syscall.Getpgid(command.Process.Pid)
		Expect(err).NotTo(HaveOccurred())
		Expect(receipt.Group).NotTo(Equal(parentGroup))
		deadline := time.Now().Add(6 * time.Second)
		Expect(command.Process.Signal(syscall.SIGTERM)).To(Succeed())
		Eventually(func() error { return syscall.Kill(-receipt.Group, 0) }, 2*time.Second).Should(MatchError(syscall.ESRCH))
		Expect(installationSnapshot(f.cwd)).To(Equal(projectBefore))
		Expect(installationSnapshot(f.environment["FACTORY_HOME"])).To(Equal(sourceBefore))
		Expect(syscall.Kill(unrelated.Process.Pid, 0)).To(Succeed())
		Expect(os.ReadFile(sentinel)).To(Equal([]byte("preserve unrelated cache data\n")))
		AddReportEntry("default-upgrade.acquisition-native-reconciled", map[string]any{"blocked_stderr": blockedStderr, "parent_pid": command.Process.Pid, "parent_alive": syscall.Kill(command.Process.Pid, 0) == nil, "owned_group": receipt.Group, "owned_group_absent": true, "filled_bytes": filled, "inherited_flags": flags, "project_and_foreign_source_preserved": true, "phase": "authenticating target before target inspection"}, ReportEntryVisibilityNever)
		remaining := time.Until(deadline)
		Expect(remaining).To(BeNumerically(">", 0))
		var returned error
		Eventually(finished, remaining).Should(Receive(&returned), "ordinary upgrade must not remain alive on a blocked cancellation diagnostic")
		completed = true
		Expect(returned).To(HaveOccurred())
		Expect(command.ProcessState.ExitCode()).To(Equal(1))
		Expect(stdout.String()).To(BeEmpty(), "failed acquisition cannot publish a proposal or completion")
		if blockedStderr {
			after, err := unix.FcntlInt(descriptor, unix.F_GETFL, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(after).To(Equal(flags))
			Expect(writer.Close()).To(Succeed())
			captured, err := io.ReadAll(reader)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(captured)).To(Equal(strings.Repeat("x", filled)))
			AddReportEntry("default-upgrade.acquisition-caller-reaped", map[string]any{"status": command.ProcessState.ExitCode(), "flags_before": flags, "flags_after": after, "filled_bytes": filled, "captured_bytes": len(captured)}, ReportEntryVisibilityNever)
		} else {
			Expect(stderr.String()).To(MatchRegexp(`^factory (candidate|upgrade):`))
			Expect(stderr.String()).To(ContainSubstring("trusted gh api failed"))
		}
		for _, pid := range []int{receipt.Leader, receipt.Git} {
			Eventually(func() error { return syscall.Kill(pid, 0) }, 2*time.Second).Should(MatchError(syscall.ESRCH))
		}
		Expect(installationSnapshot(f.cwd)).To(Equal(projectBefore))
		Expect(installationSnapshot(f.environment["FACTORY_HOME"])).To(Equal(sourceBefore))
		Expect(os.ReadFile(sentinel)).To(Equal([]byte("preserve unrelated cache data\n")))
		Expect(syscall.Kill(unrelated.Process.Pid, 0)).To(Succeed())
		defaultInitNoForbidden(f)
	}, Entry("ordinary stderr", false), Entry("full inherited stderr", true))
})

var _ = Describe("Go default packaged upgrade", Ordered, ContinueOnFailure, func() {
	var fixtures *defaultInitFixtures
	BeforeAll(func() { fixtures = prepareDefaultUpgrade() })
	// per docs/adr/0122-default-packaged-go-upgrade.md:84
	It("reconciles selected runtime and nested native children after caller-only cancellation", func() {
		f, driver := fixtures.surface(defaultUpgradeTag)
		imageGit(f.cwd, "init", "--quiet")
		writeFixture(filepath.Join(f.cwd, "application.txt"), []byte("caller project remains read-only\n"), 0600)
		ready := filepath.Join(f.records, "selected-acquisition-git-ready")
		git, err := exec.LookPath("git")
		Expect(err).NotTo(HaveOccurred())
		git, err = filepath.EvalSymlinks(git)
		Expect(err).NotTo(HaveOccurred())
		f.environment["SOURCE_SELFTEST_READY"], f.environment["SOURCE_SELFTEST_REAL_GIT"] = ready, git
		original := filepath.Join(f.root, "host tools/ordinary-gh")
		gh, err := os.ReadFile(filepath.Join(f.root, "host tools/gh"))
		Expect(err).NotTo(HaveOccurred())
		writeFixture(original, gh, 0700)
		f.environment["HANDOFF_ORDINARY_GH"] = original
		selectedRecord := filepath.Join(f.records, "selected-hop")
		observer := `if [ -z "${FACTORY_RELEASE_DIR-}" ]; then exec /bin/sh "$HANDOFF_ORDINARY_GH" "$@"; fi
selected_group="$(/bin/ps -p "$PPID" -o pgid= | tr -d '[:space:]')"
printf '%s\t%s\t%s\n' "$PPID" "$selected_group" "$FACTORY_RELEASE_DIR" > "$HANDOFF_RECORD_DIR/selected-hop.part"
mv "$HANDOFF_RECORD_DIR/selected-hop.part" "$HANDOFF_RECORD_DIR/selected-hop"
`
		held := strings.Replace(sourceSelftestHeldGit, "set -eu\n", "set -eu\n"+observer, 1)
		writeFixture(filepath.Join(f.root, "host tools/gh"), []byte(held), 0700)
		unrelated := exec.Command("/bin/sleep", "30")
		unrelated.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		Expect(unrelated.Start()).To(Succeed())
		DeferCleanup(func() { _ = unrelated.Process.Kill(); _ = unrelated.Wait() })
		sentinel := filepath.Join(f.root, "unrelated-sentinel")
		writeFixture(sentinel, []byte("unrelated data\n"), 0600)
		projectBefore, sourceBefore := installationSnapshot(f.cwd), installationSnapshot(f.environment["FACTORY_HOME"])
		command := exec.Command(driver, defaultUpgradeArgs("--dry-run", "--json")...) // #nosec G204 -- literal public upgrade through the evaluator-owned compiled Go CLI.
		command.Dir, command.Env = f.cwd, sourceSelftestEnvironment(f.environment)
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		Expect(command.Start()).To(Succeed())
		finished := make(chan error, 1)
		go func() { finished <- command.Wait() }()
		completed := false
		var receipt sourceSelftestReceipt
		selectedPID, selectedGroup := 0, 0
		DeferCleanup(func() {
			sourceSelftestCleanupGroup(receipt)
			if selectedPID > 1 {
				if group, err := syscall.Getpgid(selectedPID); err == nil && group == selectedGroup && selectedGroup != syscall.Getpgrp() {
					_ = syscall.Kill(-selectedGroup, syscall.SIGKILL)
				}
			}
			if !completed {
				_ = command.Process.Kill()
				select {
				case <-finished:
				case <-time.After(2 * time.Second):
				}
			}
			deadline := time.Now().Add(2 * time.Second)
			for _, group := range []int{receipt.Group, selectedGroup, command.Process.Pid} {
				if group <= 1 {
					continue
				}
				for syscall.Kill(-group, 0) == nil && time.Now().Before(deadline) {
					time.Sleep(10 * time.Millisecond)
				}
				if probe := syscall.Kill(-group, 0); probe != syscall.ESRCH {
					holdFixtureCleanup(f.root, fmt.Sprintf("owned group %d remains unconfirmed after guarded cleanup: %v", group, probe))
				}
			}
		})
		receipt = sourceSelftestReadiness(ready)
		body, err := os.ReadFile(selectedRecord)
		Expect(err).NotTo(HaveOccurred())
		fields := strings.Split(strings.TrimSpace(string(body)), "\t")
		Expect(fields).To(HaveLen(3))
		selectedPID, err = strconv.Atoi(fields[0])
		Expect(err).NotTo(HaveOccurred())
		selectedGroup, err = strconv.Atoi(fields[1])
		Expect(err).NotTo(HaveOccurred())
		Expect(selectedPID).To(Equal(selectedGroup))
		Expect(selectedPID).NotTo(Equal(command.Process.Pid))
		Expect(selectedGroup).NotTo(Equal(receipt.Group))
		Expect(fields[2]).To(HavePrefix(f.root + string(filepath.Separator) + "factory-release-"))
		Expect(syscall.Kill(selectedPID, 0)).To(Succeed())
		Expect(command.Process.Signal(syscall.SIGTERM)).To(Succeed())
		var returned error
		select {
		case returned = <-finished:
			completed = true
		case <-time.After(6 * time.Second):
		}
		// Record both independently observed boundaries before either assertion can
		// mask the other: a dead selected group does not prove its native group gone.
		outerProbe, innerProbe := syscall.Kill(-selectedGroup, 0), syscall.Kill(-receipt.Group, 0)
		status := -999
		if completed {
			status = command.ProcessState.ExitCode()
		}
		AddReportEntry("default-upgrade.selected-hop-cancellation", map[string]any{"caller_pid": command.Process.Pid, "caller_reaped": completed, "caller_status": status, "selected_pid": selectedPID, "selected_group": selectedGroup, "selected_group_probe": fmt.Sprint(outerProbe), "native_receipt": receipt, "native_group_probe": fmt.Sprint(innerProbe), "project_preserved": reflect.DeepEqual(installationSnapshot(f.cwd), projectBefore), "foreign_source_preserved": reflect.DeepEqual(installationSnapshot(f.environment["FACTORY_HOME"]), sourceBefore), "unrelated_alive": syscall.Kill(unrelated.Process.Pid, 0) == nil}, ReportEntryVisibilityNever)
		Expect(completed).To(BeTrue(), "caller cancellation must finish after owned native reconciliation")
		Expect(returned).To(HaveOccurred())
		Expect(status).To(Equal(1), "cancelled handoff must be operational failure, not a raw negative signal status")
		Expect(outerProbe).To(MatchError(syscall.ESRCH))
		Expect(innerProbe).To(MatchError(syscall.ESRCH), "nested native children remain owned across the selected-runtime handoff")
		Expect(stdout.String()).To(BeEmpty())
		Expect(stderr.String()).NotTo(ContainSubstring("Installation complete"))
		Expect(installationSnapshot(f.cwd)).To(Equal(projectBefore))
		Expect(installationSnapshot(f.environment["FACTORY_HOME"])).To(Equal(sourceBefore))
		Expect(os.ReadFile(sentinel)).To(Equal([]byte("unrelated data\n")))
		Expect(syscall.Kill(unrelated.Process.Pid, 0)).To(Succeed())
		defaultInitNoForbidden(f)
	})
})
