package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// per docs/adr/0148-transactional-installed-ci-migration.md:35
func ciTransactionSetting(root, key, value string) {
	GinkgoHelper()
	path := filepath.Join(root, "factory.yaml")
	body, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	lines := strings.Split(string(body), "\n")
	changed := 0
	for index, line := range lines {
		if strings.HasPrefix(line, key+":") {
			lines[index] = key + ": " + strconv.Quote(value)
			changed++
		}
	}
	Expect(changed).To(Equal(1), "change one real existing setting")
	writeFixture(path, []byte(strings.Join(lines, "\n")), 0644)
}

// per docs/adr/0148-transactional-installed-ci-migration.md:103
func ciTransactionInterrupt(p defaultUpgradeProject, selected defaultInitImage, proposal map[string]any) string {
	GinkgoHelper()
	program := filepath.Join(p.f.root, "selected-transaction-owner")
	writeFixture(program, selected.binary, 0700)
	id := "candidate-" + proposal["proposal_digest"].(string)[:24]
	ciTransactionInterruptOwner(p, program, p.args("--ref", defaultUpgradeNextTag, "--confirm", proposal["proposal_digest"].(string), "--quiescent"), id, "forward", nil)
	return id
}

// per docs/adr/0148-transactional-installed-ci-migration.md:103
func ciTransactionInterruptOwner(p defaultUpgradeProject, program string, args []string, id, direction string, expectedWorkflow []byte) {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := handoffCommand(ctx, p.root, p.f.environment, "", program, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	Expect(cmd.Start()).To(Succeed())
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	waited := false
	DeferCleanup(func() {
		if !waited {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-finished
		}
	})
	path := filepath.Join(p.root, ".factory/installation-transactions", id+".json")
	witness := map[string]any{}
	observe := func(stopped bool) bool {
		select {
		case err := <-finished:
			waited = true
			StopTrying(fmt.Sprintf("owner exited before workflow publication witness: %v; stdout=%q stderr=%q", err, stdout.String(), stderr.String())).Now()
		default:
		}
		body, err := os.ReadFile(path)
		var record map[string]any
		decodeErr := json.Unmarshal(body, &record)
		witness = map[string]any{"owner_pid": cmd.Process.Pid, "requested_direction": direction, "journal_error": fmt.Sprint(err), "decode_error": fmt.Sprint(decodeErr), "phase": record["phase"], "direction": record["direction"]}
		// The owner can enter post-publication verification before SIGSTOP arrives.
		// A committed record is never an interruption witness.
		unfinished := record["phase"] == "applying" || stopped && (record["phase"] == "checking" || record["phase"] == "metadata_prepared")
		if err != nil || decodeErr != nil || !unfinished || record["direction"] != direction {
			return false
		}
		rows, ok := record["entries"].([]any)
		if !ok {
			return false
		}
		phase := "applied"
		if direction == "reverse" {
			phase = "reversed"
		}
		for _, value := range rows {
			row := value.(map[string]any)
			if row["path"] == ".github/workflows/factory.yml" && row["phase"] == phase {
				witness["workflow_phase"] = row["phase"]
				if expectedWorkflow != nil {
					current, err := os.ReadFile(filepath.Join(p.root, ".github/workflows/factory.yml"))
					witness["workflow_error"] = fmt.Sprint(err)
					witness["original_workflow_restored"] = bytes.Equal(current, expectedWorkflow)
					if err != nil || !bytes.Equal(current, expectedWorkflow) {
						return false
					}
				}
				_, err := os.Lstat(filepath.Join(p.root, ".factory/runtime-publication.pending"))
				witness["pending_error"] = fmt.Sprint(err)
				return err == nil
			}
		}
		return false
	}
	Eventually(func() bool { return observe(false) }, 80*time.Second, time.Millisecond).Should(BeTrue(), "witness the actual workflow publication under pending intent")
	beforeStop := witness
	Expect(syscall.Kill(cmd.Process.Pid, syscall.SIGSTOP)).To(Succeed())
	Eventually(func() bool {
		stopped, err := installationProcessStopped(cmd.Process.Pid)
		Expect(err).NotTo(HaveOccurred())
		return stopped
	}, 5*time.Second, time.Millisecond).Should(BeTrue())
	stillPublished := observe(true)
	AddReportEntry("CI publication before stopping", beforeStop)
	AddReportEntry("CI publication after stopping", witness)
	Expect(stillPublished).To(BeTrue(), "stopped owner must still retain the published workflow and pending transaction in the required direction")
	stopped, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	Expect(syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)).To(Succeed())
	Expect(<-finished).To(HaveOccurred())
	waited = true
	status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	Expect(ok).To(BeTrue())
	Expect(status.Signaled()).To(BeTrue())
	Expect(status.Signal()).To(Equal(syscall.SIGKILL), "actual transaction owner was stopped and killed")
	Expect(os.ReadFile(path)).To(Equal(stopped), "reaped owner cannot advance durable intent")
}

// per docs/adr/0148-transactional-installed-ci-migration.md:29
func ciTransactionRecoveryArguments(proposal map[string]any) []string {
	GinkgoHelper()
	advice := []string{}
	for index, value := range proposal["recovery_arguments"].([]any) {
		if index == 0 || value == "--dry-run" {
			continue
		}
		advice = append(advice, value.(string))
	}
	Expect(advice).To(ContainElement("--installation"))
	return advice
}

type ciTransactionRetainedInput struct {
	identity os.FileInfo
	snapshot map[string]string
}

// per docs/adr/0101-go-release-handoff.md:579
// per docs/adr/0148-transactional-installed-ci-migration.md:100
func ciTransactionRetainedGoInputs(p defaultUpgradeProject, diagnostic string) map[string]ciTransactionRetainedInput {
	GinkgoHelper()
	paths, err := filepath.Glob(filepath.Join(p.f.root, "factory-release-*"))
	Expect(err).NotTo(HaveOccurred())
	Expect(paths).To(HaveLen(2), "interrupted Go transactions retain target and predecessor inputs")
	retained := map[string]ciTransactionRetainedInput{}
	for _, path := range paths {
		info, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.IsDir()).To(BeTrue())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0700)))
		if diagnostic != "" {
			Expect(diagnostic).To(ContainSubstring(path), "retained inputs need concrete recovery advice")
		}
		retained[path] = ciTransactionRetainedInput{identity: info, snapshot: installationSnapshot(path)}
	}
	return retained
}

// per docs/adr/0101-go-release-handoff.md:579
func ciTransactionAssertRetainedGoInputs(p defaultUpgradeProject, retained map[string]ciTransactionRetainedInput) {
	GinkgoHelper()
	_, forbidden := os.Lstat(filepath.Join(p.f.records, "forbidden-tool"))
	Expect(os.IsNotExist(forbidden)).To(BeTrue(), "archive recovery must not invoke compiler/Python/archive shell helpers")
	paths, err := filepath.Glob(filepath.Join(p.f.root, "factory-release-*"))
	Expect(err).NotTo(HaveOccurred())
	Expect(paths).To(HaveLen(len(retained)), "archive recovery must not invent acquisition work or delete retained external input owners")
	for _, path := range paths {
		original, found := retained[path]
		Expect(found).To(BeTrue(), path)
		current, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(original.identity, current)).To(BeTrue(), path)
		Expect(installationSnapshot(path)).To(Equal(original.snapshot), "independently retained inputs remain byte-for-byte usable: %s", path)
	}
}

var _ = Describe("Whole installation upgrade release candidate handoff initialized CI migration transaction", Ordered, ContinueOnFailure, SpecPriority(2), Label("handoff-init-ci-project"), func() {
	var fixtures *defaultUpgradeJourneyFixtures
	BeforeAll(func() { fixtures = prepareDefaultUpgradeJourneys(true) })

	// per docs/adr/0148-transactional-installed-ci-migration.md:31
	// per docs/adr/0148-transactional-installed-ci-migration.md:90
	// per docs/adr/0148-transactional-installed-ci-migration.md:103
	It("migrates exact legacy workflows with ignored inert backups and complete public rollback", func() {
		p := fixtures.legacyProject()
		oldPath := ".github/workflows/ci.yml"
		newPath := ".github/workflows/factory.yml"
		oldBytes, err := os.ReadFile(filepath.Join(p.root, oldPath))
		Expect(err).NotTo(HaveOccurred())
		unrelated := []byte("name: Application-owned job\non: workflow_dispatch\n")
		writeFixture(filepath.Join(p.root, ".github/workflows/application.yml"), unrelated, 0644)
		proposal := p.preview()
		report, ok := proposal["ci_workflow"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(report).To(HaveKeyWithValue("apply_supported", true))
		Expect(proposal["actions"]).To(ContainElement(HaveKeyWithValue("path", oldPath)))
		Expect(proposal["actions"]).To(ContainElement(HaveKeyWithValue("path", newPath)))
		rows := ciMigrationRows(report)
		Expect(rows[oldPath]["disposition"]).To(Equal("retire"))
		Expect(rows[newPath]["disposition"]).To(Equal("create"))
		applied := p.apply(proposal)
		Expect(applied.status).To(BeZero(), "%+v", applied)
		_, err = os.Lstat(filepath.Join(p.root, oldPath))
		Expect(os.IsNotExist(err)).To(BeTrue(), "the recognized copied source CI must retire")
		created, err := os.ReadFile(filepath.Join(p.root, newPath))
		Expect(err).NotTo(HaveOccurred())
		Expect(fmt.Sprintf("%x", sha256.Sum256(created))).To(Equal(rows[newPath]["after_sha256"]))
		Expect(string(created)).To(ContainSubstring(defaultUpgradeTag))
		Expect(string(created)).To(ContainSubstring(fixtures.revision))
		Expect(os.ReadFile(filepath.Join(p.root, ".github/workflows/application.yml"))).To(Equal(unrelated))
		ids := retentionBackupIDs(p.root)
		Expect(ids).To(HaveLen(1))
		saved := filepath.Join(p.root, ".factory/backups", ids[0], "assets", oldPath)
		Expect(os.ReadFile(saved)).To(Equal(oldBytes))
		info, err := os.Stat(saved)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
		Expect(imageProcess(p.root, nil, "git", "check-ignore", "--no-index", "-q", "--", saved).status).To(BeZero())
		rollback := fixtures.rollbackArgs(p, defaultUpgradeTag, ids[0])
		env := defaultUpgradeLegacyRecoveryEnvironment(p)
		reverse := handoffProposal(handoffNativeProcess(p.root, env, "", p.driver, append(append([]string{}, rollback...), "--dry-run", "--json")...))
		restored := handoffNativeProcess(p.root, env, "", p.driver, append(rollback, "--confirm", reverse["proposal_digest"].(string), "--quiescent")...)
		Expect(restored.status).To(BeZero(), "%+v", restored)
		Expect(os.ReadFile(filepath.Join(p.root, oldPath))).To(Equal(oldBytes))
		_, err = os.Lstat(filepath.Join(p.root, newPath))
		Expect(os.IsNotExist(err)).To(BeTrue(), "rollback removes only its newly created generated workflow")
		Expect(os.ReadFile(filepath.Join(p.root, ".github/workflows/application.yml"))).To(Equal(unrelated))
		defaultInitNoForbidden(p.f)
	})

	// per docs/adr/0148-transactional-installed-ci-migration.md:35
	DescribeTable("rejects stale consent without changing active files", func(change string) {
		p := fixtures.legacyProject()
		if change == "language-version" {
			ciTransactionSetting(p.root, "language_packs", "go")
			configPath := filepath.Join(p.root, "factory.yaml")
			body, err := os.ReadFile(configPath)
			Expect(err).NotTo(HaveOccurred())
			if strings.Contains("\n"+string(body), "\ngo_version:") {
				ciTransactionSetting(p.root, "go_version", "1.26")
			} else {
				writeFixture(configPath, append(body, []byte("\ngo_version: \"1.26\"\n")...), 0644)
			}
		}
		proposal := p.preview()
		path := filepath.Join(p.root, ".github/workflows/ci.yml")
		switch change {
		case "configuration":
			ciTransactionSetting(p.root, "protected_paths", "application")
		case "language-version":
			ciTransactionSetting(p.root, "go_version", "1.27")
			fresh := p.preview()
			Expect(fresh["proposal_digest"]).NotTo(Equal(proposal["proposal_digest"]), "a valid selected-language version change must alter combined consent")
			oldRows, newRows := ciMigrationRows(ciMigrationReport(proposal)), ciMigrationRows(ciMigrationReport(fresh))
			Expect(newRows[".github/workflows/factory.yml"]["after_sha256"]).NotTo(Equal(oldRows[".github/workflows/factory.yml"]["after_sha256"]), "the changed valid setting must affect proposed CI bytes")
		default:
			body, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			writeFixture(path, append(body, []byte("\n# operator changed CI after preview\n")...), 0644)
		}
		before := installationSnapshot(p.root)
		refused := p.apply(proposal)
		Expect(refused.status).To(Equal(2), "%+v", refused)
		Expect(installationSnapshot(p.root)).To(Equal(before))
		Expect(retentionBackupIDs(p.root)).To(BeEmpty())
		defaultInitNoForbidden(p.f)
	}, Entry("workflow bytes", "workflow"), Entry("relevant configuration", "configuration"), Entry("selected language version", "language-version"))

	// per docs/adr/0148-transactional-installed-ci-migration.md:55
	DescribeTable("preserves conflicting known workflow occupants before any combined mutation", func(kind string) {
		p := fixtures.legacyProject()
		path := filepath.Join(p.root, ".github/workflows/ci.yml")
		switch kind {
		case "custom":
			writeFixture(path, []byte("name: Operator-owned CI\non: workflow_dispatch\n"), 0644)
		case "mode":
			Expect(os.Chmod(path, 0755)).To(Succeed())
		case "link":
			Expect(os.Remove(path)).To(Succeed())
			Expect(os.Symlink("/must/not/follow/operator-ci", path)).To(Succeed())
		}
		before := installationSnapshot(p.root)
		proposal := ciMigrationProposal(p.run("--dry-run", "--json"), 2)
		Expect(proposal["blockers"]).To(ContainElement(HaveKeyWithValue("path", ".github/workflows/ci.yml")))
		refused := p.apply(proposal)
		Expect(refused.status).To(Equal(2), "%+v", refused)
		Expect(installationSnapshot(p.root)).To(Equal(before))
		Expect(retentionBackupIDs(p.root)).To(BeEmpty())
		defaultInitNoForbidden(p.f)
	}, Entry("customized bytes", "custom"), Entry("changed mode", "mode"), Entry("dangling link", "link"))

	// per docs/adr/0148-transactional-installed-ci-migration.md:81
	// per docs/adr/0148-transactional-installed-ci-migration.md:103
	It("refreshes the actual merged predecessor recipe and restores its generated workflow after settings change", func() {
		const revision = "24712604cbe98dc2df087b73910141defb68f745"
		before := historicalInstallationImage(fixtures.invoker, revision, defaultUpgradeTag)
		oldSurface, oldDriver := handoffNativeFixture(before)
		writeFixture(filepath.Join(oldSurface.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+defaultUpgradeTag+"-"+oldSurface.target), before.binary, 0600)
		writeFixture(filepath.Join(oldSurface.environment["HANDOFF_ASSETS"], "factory-installation-"+defaultUpgradeTag+"-"+oldSurface.target+".tar.gz"), before.archive, 0600)
		root := filepath.Join(oldSurface.cwd, "immutable predecessor project")
		Expect(os.Mkdir(root, 0700)).To(Succeed())
		imageGit(root, "init", "--quiet")
		initialized := handoffNativeProcess(oldSurface.cwd, oldSurface.environment, localUpgradeAnswers, oldDriver, "init", root, "--go-candidate", defaultUpgradeTag, "--pack", "none")
		Expect(initialized.status).To(BeZero(), "%+v", initialized)
		oldWorkflow, err := os.ReadFile(filepath.Join(root, ".github/workflows/factory.yml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(oldWorkflow)).To(ContainSubstring(revision))
		surface, driver := fixtures.surface(defaultUpgradeNextTag)
		providerPath := filepath.Join(surface.root, "host tools/gh")
		provider, err := os.ReadFile(providerPath)
		Expect(err).NotTo(HaveOccurred())
		oldAPI := `  api) printf '%s\n' "$HANDOFF_REVISION" ;;`
		newAPI := `  api) case "$2" in *v0.2.0) printf '%s\n' "$HANDOFF_OLD_REVISION" ;; *) printf '%s\n' "$HANDOFF_REVISION" ;; esac ;;`
		Expect(strings.Count(string(provider), oldAPI)).To(Equal(1))
		writeFixture(providerPath, []byte(strings.Replace(string(provider), oldAPI, newAPI, 1)), 0700)
		surface.environment["HANDOFF_OLD_REVISION"] = revision
		writeFixture(filepath.Join(surface.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+defaultUpgradeTag+"-"+surface.target), before.binary, 0600)
		writeFixture(filepath.Join(surface.environment["HANDOFF_ASSETS"], "factory-installation-"+defaultUpgradeTag+"-"+surface.target+".tar.gz"), before.archive, 0600)
		oldImage := defaultInitImage{binary: before.binary, archive: before.archive}
		oldImage.result.Archive = before.archivePath
		p := defaultUpgradeProject{f: surface, driver: driver, root: root, beforeVersion: defaultUpgradeTag, before: oldImage, inputs: append([]string{}, installationInputs...)}
		proposal := p.preview("--ref", defaultUpgradeNextTag)
		rows := ciMigrationRows(ciMigrationReport(proposal))
		Expect(rows[".github/workflows/factory.yml"]["disposition"]).To(Equal("replace"))
		applied := p.apply(proposal, "--ref", defaultUpgradeNextTag)
		Expect(applied.status).To(BeZero(), "%+v", applied)
		newWorkflow, err := os.ReadFile(filepath.Join(root, ".github/workflows/factory.yml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(newWorkflow).NotTo(Equal(oldWorkflow))
		Expect(string(newWorkflow)).To(ContainSubstring(fixtures.revision))
		ids := retentionBackupIDs(root)
		Expect(ids).To(HaveLen(1))
		Expect(os.ReadFile(filepath.Join(root, ".factory/backups", ids[0], "assets/.github/workflows/factory.yml"))).To(Equal(oldWorkflow))
		ciTransactionSetting(root, "protected_paths", "changed-after-upgrade")
		rollback := fixtures.rollbackArgs(p, defaultUpgradeNextTag, ids[0])
		for index, value := range rollback {
			if value == "--before-revision" {
				rollback[index+1] = revision
			}
		}
		reverse := handoffProposal(handoffNativeProcess(root, surface.environment, "", driver, append(append([]string{}, rollback...), "--dry-run", "--json")...))
		restored := handoffNativeProcess(root, surface.environment, "", driver, append(rollback, "--confirm", reverse["proposal_digest"].(string), "--quiescent")...)
		Expect(restored.status).To(BeZero(), "%+v", restored)
		Expect(os.ReadFile(filepath.Join(root, ".github/workflows/factory.yml"))).To(Equal(oldWorkflow))
		Expect(os.ReadFile(filepath.Join(root, ".factory/bin/factory-runtime"))).To(Equal(before.binary))
		defaultInitNoForbidden(surface)
	})

	// per docs/adr/0148-transactional-installed-ci-migration.md:113
	It("prunes eligible workflow backups on a later release and retains an independently usable immediate predecessor", func() {
		const third = "v0.2.2"
		p := fixtures.goProject(defaultUpgradeTag)
		fixtures.provide(p.f, defaultUpgradeNextTag, third)
		oldWorkflow, err := os.ReadFile(filepath.Join(p.root, ".github/workflows/factory.yml"))
		Expect(err).NotTo(HaveOccurred())
		first := p.preview("--ref", defaultUpgradeNextTag)
		Expect(p.apply(first, "--ref", defaultUpgradeNextTag).status).To(BeZero())
		ids := retentionBackupIDs(p.root)
		Expect(ids).To(HaveLen(1))
		oldID := ids[0]
		Expect(os.ReadFile(filepath.Join(p.root, ".factory/backups", oldID, "assets/.github/workflows/factory.yml"))).To(Equal(oldWorkflow))
		files, size := retentionCounts(p.root, oldID)
		same := retentionPreviewReport(p.preview("--ref", defaultUpgradeNextTag))
		Expect(same.Advanced).To(BeFalse())
		Expect(retentionBackupIDs(p.root)).To(Equal(ids))
		middleWorkflow, err := os.ReadFile(filepath.Join(p.root, ".github/workflows/factory.yml"))
		Expect(err).NotTo(HaveOccurred())
		// This setting changes historical reconstruction inputs without altering
		// the no-pack target workflow. The saved context must qualify old backups.
		ciTransactionSetting(p.root, "protected_paths", "changed-after-upgrade")
		next := p.preview("--ref", third)
		planned := retentionPreviewReport(next)
		Expect(planned.Advanced).To(BeTrue())
		row := retentionSet(planned, oldID)
		Expect(row.Action).To(Equal("prune"))
		Expect(row.Files).NotTo(BeNil())
		Expect(row.Bytes).NotTo(BeNil())
		Expect(*row.Files).To(Equal(files))
		Expect(*row.Bytes).To(Equal(size))
		result := p.apply(next, "--ref", third)
		Expect(result.status).To(BeZero(), "%+v", result)
		applied := retentionApplyReport(result)
		Expect(applied.RemovedSets).To(Equal(1))
		Expect(applied.RemovedFiles).To(Equal(files))
		Expect(applied.RemovedBytes).To(Equal(size))
		remaining := retentionBackupIDs(p.root)
		Expect(remaining).To(HaveLen(1))
		Expect(remaining).NotTo(ContainElement(oldID))
		Expect(os.ReadFile(filepath.Join(p.root, ".factory/backups", remaining[0], "assets/.github/workflows/factory.yml"))).To(Equal(middleWorkflow))
		p.beforeVersion, p.before = defaultUpgradeNextTag, fixtures.image(defaultUpgradeNextTag)
		rollback := fixtures.rollbackArgs(p, third, remaining[0])
		reverse := handoffProposal(handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(append([]string{}, rollback...), "--dry-run", "--json")...))
		restored := handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(rollback, "--confirm", reverse["proposal_digest"].(string), "--quiescent")...)
		Expect(restored.status).To(BeZero(), "%+v", restored)
		Expect(os.ReadFile(filepath.Join(p.root, ".github/workflows/factory.yml"))).To(Equal(middleWorkflow))
		Expect(retentionBackupIDs(p.root)).To(Equal(remaining))
		defaultInitNoForbidden(p.f)
	})

	// per docs/adr/0148-transactional-installed-ci-migration.md:103
	// per docs/adr/0101-go-release-handoff.md:579
	It("abandons genuine preparation-only CI intent through public archive recovery without changing workflows", func() {
		p := fixtures.goProject(defaultUpgradeTag)
		fixtures.provide(p.f, defaultUpgradeNextTag)
		originalInstallation := installationSnapshot(p.root)
		path := filepath.Join(p.root, ".gitignore")
		original, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		exposed := "\n!/.factory/\n/.factory/bin/\n/.factory/assets/\n/.factory/installation.current\n/.factory/installation-transactions/\n!/.factory/backups/\n"
		writeFixture(path, append(append([]byte{}, original...), []byte(exposed)...), 0644)
		proposal := p.preview("--ref", defaultUpgradeNextTag)
		Expect(ciMigrationReport(proposal)["apply_supported"]).To(BeTrue())
		before := installationSnapshot(p.root)
		refused := p.apply(proposal, "--ref", defaultUpgradeNextTag)
		Expect(refused.status).To(Equal(2), "%+v", refused)
		Expect(refused.stderr).To(ContainSubstring("not effectively ignored"))
		retained := ciTransactionRetainedGoInputs(p, refused.stderr)
		installationAssertExistingUnchanged(p.root, before)
		id := "candidate-" + proposal["proposal_digest"].(string)[:24]
		record := minimalInventoryJSON(filepath.Join(p.root, ".factory/installation-transactions", id+".json"))
		Expect(record).To(HaveKey("ci_workflow"))
		Expect(record["phase"]).To(Equal("prepared"))
		for _, raw := range record["entries"].([]any) {
			Expect(raw.(map[string]any)["phase"]).NotTo(Equal("applied"))
		}
		writeFixture(path, original, 0644)
		args := fixtures.rollbackArgs(p, defaultUpgradeNextTag, id)
		for index, arg := range args {
			if arg == "--rollback" {
				args[index] = "--recover"
			}
		}
		args = append(args, "--direction", "reverse")
		recovery := handoffProposal(handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(append([]string{}, args...), "--dry-run", "--json")...))
		complete := handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(args, "--confirm", recovery["proposal_digest"].(string), "--quiescent")...)
		Expect(complete.status).To(BeZero(), "%+v", complete)
		for _, name := range []string{".factory/installation-transactions/" + id + ".json", ".factory/runtime-publication.pending"} {
			_, err := os.Lstat(filepath.Join(p.root, name))
			Expect(os.IsNotExist(err)).To(BeTrue(), "abandoned preparation must release its durable intent: %s", name)
		}
		installationAssertExistingUnchanged(p.root, originalInstallation)
		ordinary := handoffNativeProcess(p.root, p.f.environment, "", filepath.Join(p.root, "factory"), "metrics", "--json")
		Expect(ordinary.status).To(BeZero(), "%+v", ordinary)
		ciTransactionAssertRetainedGoInputs(p, retained)
	})

	// per docs/adr/0148-transactional-installed-ci-migration.md:113
	DescribeTable("preserves held or edited workflow-bearing backup sets across a later release", func(kind string) {
		const third = "v0.2.2"
		p := fixtures.goProject(defaultUpgradeTag)
		fixtures.provide(p.f, defaultUpgradeNextTag, third)
		first := p.preview("--ref", defaultUpgradeNextTag)
		Expect(p.apply(first, "--ref", defaultUpgradeNextTag).status).To(BeZero())
		ids := retentionBackupIDs(p.root)
		Expect(ids).To(HaveLen(1))
		id := ids[0]
		path := filepath.Join(p.root, ".factory/backups", id, "assets/.github/workflows/factory.yml")
		original, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		if kind == "held" {
			held := handoffNativeProcess(p.root, p.f.environment, "", filepath.Join(p.root, "factory"), "upgrade", "--keep-backup", id)
			Expect(held.status).To(BeZero(), "%+v", held)
			var receipt retentionReportOracle
			Expect(json.Unmarshal([]byte(held.stdout), &receipt)).To(Succeed())
			Expect(receipt.Mode).To(Equal("keep-backup"))
			Expect(receipt.Advanced).To(BeFalse())
			Expect(retentionSet(receipt, id).Action).To(Equal("held"))
		} else {
			original = append(original, []byte("\n# operator saved-file edit\n")...)
			writeFixture(path, original, 0600)
		}
		proposal := p.preview("--ref", third)
		row := retentionSet(retentionPreviewReport(proposal), id)
		Expect(row.Action).NotTo(Equal("prune"))
		if kind == "held" {
			Expect(row.Action).To(Equal("retain"))
			Expect(row.Reason).To(Equal("held for reference"))
		}
		result := p.apply(proposal, "--ref", third)
		Expect(result.status).To(BeZero(), "%+v", result)
		Expect(retentionApplyReport(result).RemovedSets).To(BeZero())
		Expect(retentionBackupIDs(p.root)).To(HaveLen(2))
		Expect(retentionBackupIDs(p.root)).To(ContainElement(id))
		Expect(os.ReadFile(path)).To(Equal(original))
		defaultInitNoForbidden(p.f)
	}, Entry("explicit operator hold", "held"), Entry("edited saved workflow", "edited"))

	// per docs/adr/0148-transactional-installed-ci-migration.md:103
	DescribeTable("recovers actual interrupted workflow publication through emitted archive recovery arguments", func(direction string) {
		p := fixtures.goProject(defaultUpgradeTag)
		fixtures.provide(p.f, defaultUpgradeNextTag)
		old, err := os.ReadFile(filepath.Join(p.root, ".github/workflows/factory.yml"))
		Expect(err).NotTo(HaveOccurred())
		proposal := p.preview("--ref", defaultUpgradeNextTag)
		Expect(ciMigrationReport(proposal)["apply_supported"]).To(BeTrue())
		id := ciTransactionInterrupt(p, fixtures.image(defaultUpgradeNextTag), proposal)
		retained := ciTransactionRetainedGoInputs(p, "")
		published, err := os.ReadFile(filepath.Join(p.root, ".github/workflows/factory.yml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(published).NotTo(Equal(old), "forward crash witness must contain genuinely published target CI")
		args := fixtures.rollbackArgs(p, defaultUpgradeNextTag, id)
		for index, arg := range args {
			if arg == "--rollback" {
				args[index] = "--recover"
			}
		}
		args = append(args, "--direction", direction)
		recovery := handoffProposal(handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(append([]string{}, args...), "--dry-run", "--json")...))
		advice := ciTransactionRecoveryArguments(recovery)
		if direction == "reverse" {
			ciTransactionInterruptOwner(p, p.driver, append(append([]string{}, advice...), "--confirm", recovery["proposal_digest"].(string), "--quiescent"), id, "reverse", old)
			fresh := handoffProposal(handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(append([]string{}, args...), "--dry-run", "--json")...))
			Expect(fresh["proposal_digest"]).NotTo(Equal(recovery["proposal_digest"]), "actual reverse publication requires fresh recovery consent")
			recovery, advice = fresh, ciTransactionRecoveryArguments(fresh)
		}
		result := handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(advice, "--confirm", recovery["proposal_digest"].(string), "--quiescent")...)
		Expect(result.status).To(BeZero(), "%+v", result)
		current, err := os.ReadFile(filepath.Join(p.root, ".github/workflows/factory.yml"))
		Expect(err).NotTo(HaveOccurred())
		if direction == "reverse" {
			Expect(current).To(Equal(old))
		} else {
			Expect(current).NotTo(Equal(old))
			Expect(string(current)).To(ContainSubstring(defaultUpgradeNextTag))
		}
		_, err = os.Lstat(filepath.Join(p.root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		installationAssertCompletedReplay(p.root, func() cliResult {
			return handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(append([]string{}, args...), "--dry-run", "--json")...)
		}, func(digest string) cliResult {
			return handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(append([]string{}, args...), "--confirm", digest, "--quiescent")...)
		})
		// User CI edits are not runtime custody violations but must block reverse
		// overwriting of the now-customized workflow.
		if direction == "forward" {
			writeFixture(filepath.Join(p.root, ".github/workflows/factory.yml"), append(current, []byte("\n# operator's later CI edit\n")...), 0644)
			ordinary := handoffNativeProcess(p.root, p.f.environment, "", filepath.Join(p.root, "factory"), "metrics", "--json")
			Expect(ordinary.status).To(BeZero(), "%+v", ordinary)
			before := installationSnapshot(p.root)
			rollback := fixtures.rollbackArgs(p, defaultUpgradeNextTag, id)
			refused := handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(rollback, "--dry-run", "--json")...)
			Expect(refused.status).To(Equal(2), "%+v", refused)
			Expect(installationSnapshot(p.root)).To(Equal(before))
		}
		ciTransactionAssertRetainedGoInputs(p, retained)
	}, Entry("forward completion", "forward"), Entry("reverse restoration", "reverse"))

	// per docs/adr/0148-transactional-installed-ci-migration.md:65
	// per docs/adr/0148-transactional-installed-ci-migration.md:107
	It("refuses partial or tampered workflow evidence without changing the active installation", func() {
		p := fixtures.goProject(defaultUpgradeTag)
		fixtures.provide(p.f, defaultUpgradeNextTag)
		proposal := p.preview("--ref", defaultUpgradeNextTag)
		Expect(p.apply(proposal, "--ref", defaultUpgradeNextTag).status).To(BeZero())
		ids := retentionBackupIDs(p.root)
		Expect(ids).To(HaveLen(1))
		journalPath := filepath.Join(p.root, ".factory/installation-transactions", ids[0]+".json")
		manifestPath := filepath.Join(p.root, ".factory/backups", ids[0], "installation.manifest")
		savedPath := filepath.Join(p.root, ".factory/backups", ids[0], "assets/.github/workflows/factory.yml")
		originals := map[string][]byte{}
		for _, path := range []string{journalPath, manifestPath, savedPath} {
			body, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			originals[path] = body
		}
		rollback := fixtures.rollbackArgs(p, defaultUpgradeNextTag, ids[0])
		for _, scenario := range []string{"missing-context", "missing-row", "forged-action", "unknown-schema", "unknown-input", "changed-backup"} {
			healthy := handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(append([]string{}, rollback...), "--dry-run", "--json")...)
			Expect(healthy.status).To(BeZero(), "healthy control before %s: %+v", scenario, healthy)
			for _, path := range []string{journalPath, manifestPath} {
				var record map[string]any
				Expect(json.Unmarshal(originals[path], &record)).To(Succeed())
				Expect(record).To(HaveKey("ci_workflow"))
				switch scenario {
				case "missing-context":
					delete(record, "ci_workflow")
				case "unknown-schema":
					record["ci_workflow"].(map[string]any)["schema_version"] = 99
				case "unknown-input":
					record["ci_workflow"].(map[string]any)["current"].(map[string]any)["UNRECOGNIZED_SETTING"] = "must not be accepted"
				case "missing-row":
					rows := []any{}
					for _, value := range record["entries"].([]any) {
						if value.(map[string]any)["path"] != ".github/workflows/java-pack.yml" {
							rows = append(rows, value)
						}
					}
					Expect(rows).To(HaveLen(len(record["entries"].([]any)) - 1))
					record["entries"] = rows
				case "forged-action":
					for _, value := range record["entries"].([]any) {
						row := value.(map[string]any)
						if row["path"] == ".github/workflows/factory.yml" {
							row["action"] = "retain"
						}
					}
				}
				body, err := json.Marshal(record)
				Expect(err).NotTo(HaveOccurred())
				writeFixture(path, body, 0600)
			}
			if scenario == "changed-backup" {
				writeFixture(savedPath, []byte("not the original workflow\n"), 0600)
			}
			before := installationSnapshot(p.root)
			refused := handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(append([]string{}, rollback...), "--dry-run", "--json")...)
			Expect(refused.status).NotTo(BeZero(), "%s: %+v", scenario, refused)
			Expect(installationSnapshot(p.root)).To(Equal(before), scenario)
			for path, body := range originals {
				writeFixture(path, body, 0600)
			}
			healthy = handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(append([]string{}, rollback...), "--dry-run", "--json")...)
			Expect(healthy.status).To(BeZero(), "healthy control after %s: %+v", scenario, healthy)
		}
	})
})
