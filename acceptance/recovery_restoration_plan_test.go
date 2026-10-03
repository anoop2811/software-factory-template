package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

var restorationBlockers = []any{"migration_ownership_unproven", "transition_quiescence_unproven", "runtime_compatibility_unproven", "activation_checks_pending"}
var restorationActions = []string{"retain_reference", "restore_missing_candidate", "preserve_customized", "conflict", "assessment_error"}

func restorationFixture(selection []string) (string, string, []assessedBytes) {
	GinkgoHelper()
	binary, root, references := durableRecoveryFixture()
	args := durableRecoveryArgs(binary, root, durableRecoveryID, strings.Repeat("a", 40), selection)
	out := durableRecoveryRun(binary, root, args)
	Expect(out.status).To(BeZero(), "%+v", out)
	Expect(out.stderr).To(BeEmpty())
	return binary, root, references
}

func restorationRun(binary, root, id string, json bool, environment ...string) cliResult {
	args := []string{"--plan-restore", "--migration-id", id}
	if json {
		args = append(args, "--json")
	}
	return publicCommand(binary, root, "upgrade", args, environment)
}

func restorationObject(out cliResult, root string, valid bool) map[string]any {
	GinkgoHelper()
	Expect(out.status).To(Equal(2), "%+v", out)
	Expect(out.stderr).To(BeEmpty())
	Expect(out.stdout).To(HaveSuffix("\n"))
	Expect(out.stdout).NotTo(ContainSubstring(root))
	Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_"))
	object := budgetDecode([]byte(out.stdout)).(map[string]any)
	Expect(object).To(HaveLen(17))
	for key, value := range map[string]any{"schema_version": json.Number("1"), "mode": "restore_plan", "coverage": "partial", "scope": "g2-budget-loop-six", "migration_id": durableRecoveryID, "restorable": false, "rollback_ready": false, "activation_ready": false, "applicable": false, "prune_authorized": false} {
		Expect(object).To(HaveKeyWithValue(key, value))
	}
	source, target, authentication := "", "", "unavailable"
	if valid {
		source, target, authentication = assessmentRevision, strings.Repeat("a", 40), "operator_metadata"
	}
	Expect(object).To(HaveKeyWithValue("source_revision", source))
	Expect(object).To(HaveKeyWithValue("target_revision", target))
	Expect(object).To(HaveKeyWithValue("target_authentication", authentication))
	recovery := object["recovery"].(map[string]any)
	Expect(recovery).To(HaveLen(9))
	Expect(recovery).To(HaveKeyWithValue("path", ".factory/backups/"+durableRecoveryID))
	Expect(recovery).To(HaveKeyWithValue("restorable", false))
	Expect(recovery).To(HaveKeyWithValue("prune_authorized", false))
	counts := object["counts"].(map[string]any)
	Expect(counts).To(HaveLen(5))
	for _, action := range restorationActions {
		Expect(counts).To(HaveKey(action))
	}
	blockers := object["blockers"].([]any)
	Expect(len(blockers)).To(BeNumerically(">=", 4))
	Expect(blockers[:4]).To(Equal(restorationBlockers))
	previous := ""
	for _, value := range object["assets"].([]any) {
		asset := value.(map[string]any)
		Expect(asset).To(HaveLen(5))
		path := asset["path"].(string)
		Expect(path > previous).To(BeTrue())
		previous = path
		Expect(asset["reason"]).To(BeAssignableToTypeOf(""))
		Expect(asset["reason"]).NotTo(BeEmpty())
		Expect(asset["reference"]).To(HaveLen(3))
		installed := asset["installed"].(map[string]any)
		Expect(installed).To(HaveLen(2))
		Expect(installed).To(HaveKey("classification"))
		Expect(installed).To(HaveKey("observed"))
	}
	return object
}

func restorationTextObject(out cliResult) map[string]any {
	GinkgoHelper()
	Expect(out.status).To(Equal(2))
	Expect(out.stderr).To(BeEmpty())
	Expect(out.stdout).To(HaveSuffix("\n"))
	lines := strings.Split(strings.TrimSuffix(out.stdout, "\n"), "\n")
	Expect(lines).To(HaveLen(17))
	result := map[string]any{}
	for _, line := range lines {
		key, value, found := strings.Cut(line, "=")
		Expect(found).To(BeTrue())
		Expect(result).NotTo(HaveKey(key))
		switch key {
		case "schema_version", "recovery", "assets", "counts", "blockers", "restorable", "rollback_ready", "activation_ready", "applicable", "prune_authorized":
			result[key] = budgetDecode([]byte(value))
		default:
			result[key] = value
		}
	}
	return result
}

var _ = Describe("Read-only recovery restoration planning core", func() {
	// per docs/adr/0092-go-recovery-restoration-planning.md:52
	// per docs/adr/0092-go-recovery-restoration-planning.md:61
	// per docs/adr/0092-go-recovery-restoration-planning.md:74
	// per docs/adr/0092-go-recovery-restoration-planning.md:100
	DescribeTable("reports only selected references without granting restoration authority or mutating storage", func(full bool) {
		selection := assessmentPaths[:1]
		if full {
			selection = assessmentPaths
		}
		binary, root, references := restorationFixture(selection)
		before := assessmentTree(root)
		out := restorationRun(binary, root, durableRecoveryID, true)
		object := restorationObject(out, root, true)
		Expect(object["blockers"]).To(Equal(restorationBlockers))
		Expect(object["assets"]).To(HaveLen(len(selection)))
		counts := object["counts"].(map[string]any)
		for _, action := range restorationActions {
			count := 0
			if action == "retain_reference" {
				count = len(selection)
			}
			Expect(counts[action]).To(Equal(json.Number(strconv.Itoa(count))))
		}
		for index, value := range object["assets"].([]any) {
			asset := value.(map[string]any)
			Expect(asset).To(HaveKeyWithValue("path", selection[index]))
			Expect(asset).To(HaveKeyWithValue("action", "retain_reference"))
			Expect(asset["reference"]).To(Equal(map[string]any{"sha256": references[index].SHA256, "mode": references[index].Mode, "bytes": json.Number(strconv.Itoa(references[index].Bytes))}))
			Expect(asset["installed"]).To(HaveKeyWithValue("classification", "matching_reference"))
		}
		Expect(assessmentTree(root)).To(Equal(before))
		Expect(restorationRun(binary, root, durableRecoveryID, true)).To(Equal(out))
	}, Entry("single-reference recovery scope", false), Entry("full six-reference recovery scope", true))

	// per docs/adr/0092-go-recovery-restoration-planning.md:59
	// per docs/adr/0092-go-recovery-restoration-planning.md:85
	It("separates matching missing ordinary customizations and unsafe installed paths", func() {
		binary, root, _ := restorationFixture(assessmentPaths)
		Expect(os.Remove(filepath.Join(root, assessmentPaths[1]))).To(Succeed())
		writeFixture(filepath.Join(root, assessmentPaths[2]), []byte("PRIVATE_LATER_CUSTOMIZATION"), 0755)
		Expect(os.Chmod(filepath.Join(root, assessmentPaths[3]), 0700)).To(Succeed())
		Expect(os.Remove(filepath.Join(root, assessmentPaths[4]))).To(Succeed())
		Expect(os.Symlink("/PRIVATE_EXTERNAL", filepath.Join(root, assessmentPaths[4]))).To(Succeed())
		before := assessmentTree(root)
		object := restorationObject(restorationRun(binary, root, durableRecoveryID, true), root, true)
		for index, expected := range []string{"retain_reference", "restore_missing_candidate", "preserve_customized", "preserve_customized", "conflict", "retain_reference"} {
			Expect(object["assets"].([]any)[index]).To(HaveKeyWithValue("action", expected))
		}
		Expect(object["counts"]).To(Equal(map[string]any{"retain_reference": json.Number("2"), "restore_missing_candidate": json.Number("1"), "preserve_customized": json.Number("2"), "conflict": json.Number("1"), "assessment_error": json.Number("0")}))
		Expect(object["blockers"]).To(Equal(append(append([]any(nil), restorationBlockers...), "installed_conflicts")))
		Expect(assessmentTree(root)).To(Equal(before))
	})

	// per docs/adr/0092-go-recovery-restoration-planning.md:74
	It("reports the identical seventeen fields in deterministic text and JSON", func() {
		binary, root, _ := restorationFixture(assessmentPaths[:1])
		jsonReport := restorationObject(restorationRun(binary, root, durableRecoveryID, true), root, true)
		textReport := restorationRun(binary, root, durableRecoveryID, false)
		Expect(restorationTextObject(textReport)).To(Equal(jsonReport))
		Expect(restorationRun(binary, root, durableRecoveryID, false)).To(Equal(textReport))
	})
})

var _ = Describe("Read-only recovery restoration planning admission", func() {
	// per docs/adr/0092-go-recovery-restoration-planning.md:32
	// per docs/adr/0092-go-recovery-restoration-planning.md:93
	DescribeTable("reserves malformed planning requests without execution or persistent changes", func(kind string) {
		binary, root, _ := durableRecoveryFixture()
		args := []string{"--plan-restore", "--migration-id", durableRecoveryID, "--json"}
		switch kind {
		case "attached marker":
			args[0] = "--plan-restore=PRIVATE_VALUE"
		case "empty attached marker":
			args[0] = "--plan-restore="
		case "duplicate marker":
			args = append(args, "--plan-restore")
		case "missing ID":
			args = []string{"--plan-restore", "--json"}
		case "unsafe ID":
			args[2] = "../PRIVATE_ID\nSECOND"
		case "long ID":
			args[2] = strings.Repeat("a", 65)
		case "empty attached ID":
			args = []string{"--plan-restore", "--migration-id=", "--json"}
		case "duplicate ID":
			args = append(args, "--migration-id=PRIVATE_SECOND")
		case "duplicate JSON":
			args = append(args, "--json")
		case "attached JSON":
			args[len(args)-1] = "--json=true"
		default:
			args = append(args, kind)
		}
		marker := previewPoison(binary)
		before := assessmentTree(root)
		out := publicCommand(binary, root, "upgrade", args, nil)
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).To(HavePrefix("factory upgrade:"))
		Expect(strings.Count(out.stderr, "\n")).To(Equal(1))
		Expect(out.stderr).NotTo(ContainSubstring(root))
		Expect(out.stderr).NotTo(ContainSubstring("PRIVATE_"))
		Expect(assessmentTree(root)).To(Equal(before))
		publicNoPoison(marker)
	}, Entry("valued marker", "attached marker"), Entry("empty valued marker", "empty attached marker"), Entry("duplicate marker", "duplicate marker"), Entry("missing ID", "missing ID"), Entry("unsafe ID", "unsafe ID"), Entry("overlong ID", "long ID"), Entry("empty attached ID", "empty attached ID"), Entry("duplicate ID", "duplicate ID"), Entry("duplicate JSON", "duplicate JSON"), Entry("valued JSON", "attached JSON"), Entry("creation mode", "--create-backup"), Entry("source option", "--source=PRIVATE_SOURCE"), Entry("ref option", "--ref=PRIVATE_REF"), Entry("preview", "--dry-run"), Entry("inspection", "--inspect-backups"), Entry("rollback", "--rollback"), Entry("help", "--help"), Entry("consent", "--confirm-adoption=PRIVATE_CONSENT"), Entry("unknown flag", "--PRIVATE_UNKNOWN"), Entry("operand", "PRIVATE_OPERAND"))

	// per docs/adr/0092-go-recovery-restoration-planning.md:34
	It("accepts attached nonempty scalar ID with the same blocked plan", func() {
		binary, root, _ := restorationFixture(assessmentPaths[:1])
		detached := restorationRun(binary, root, durableRecoveryID, true)
		attached := publicCommand(binary, root, "upgrade", []string{"--plan-restore", "--migration-id=" + durableRecoveryID, "--json"}, nil)
		restorationObject(attached, root, true)
		Expect(attached).To(Equal(detached))
	})

	// per docs/adr/0092-go-recovery-restoration-planning.md:36
	// per docs/adr/0092-go-recovery-restoration-planning.md:104
	DescribeTable("preserves detached legacy value ownership even when it resembles a native marker", func(option, value string) {
		binary, root, _ := durableRecoveryFixture()
		writeFixture(filepath.Join(binary, "scripts/factory-upgrade.sh"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\nexit 37\n"), 0700)
		args := []string{option, value}
		out := publicCommand(binary, root, "upgrade", args, nil)
		Expect(out).To(Equal(cliResult{strings.Join(args, "\n") + "\n", "", 37}))
	}, Entry("source planning-like value", "--source", "--plan-restore"), Entry("ref planning-like value", "--ref", "--plan-restore"), Entry("source creation-like value", "--source", "--create-backup"), Entry("ref creation-like value", "--ref", "--create-backup"))

	// per docs/adr/0092-go-recovery-restoration-planning.md:38
	DescribeTable("claims an independent marker after a marker-valued legacy operand", func(value, independent string) {
		binary, root, _ := durableRecoveryFixture()
		marker := previewPoison(binary)
		before := assessmentTree(root)
		out := publicCommand(binary, root, "upgrade", []string{"--source", value, independent, "--migration-id", durableRecoveryID, "--json"}, nil)
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).To(HavePrefix("factory upgrade:"))
		Expect(assessmentTree(root)).To(Equal(before))
		publicNoPoison(marker)
	}, Entry("independent planning marker", "--create-backup", "--plan-restore"), Entry("independent creation marker", "--plan-restore", "--create-backup"))

	// per docs/adr/0092-go-recovery-restoration-planning.md:38
	It("keeps private bridge precedence over public restoration planning", func() {
		binary, root, _ := restorationFixture(assessmentPaths[:1])
		before := assessmentTree(root)
		out := loopProcess(root, filepath.Join(binary, "factory"), []string{"upgrade", "--plan-restore", "--migration-id", durableRecoveryID, "--json"}, nil)
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).To(HavePrefix("factory bridge:"))
		Expect(assessmentTree(root)).To(Equal(before))
	})
})

var _ = Describe("Read-only recovery restoration planning recovery evidence", func() {
	// per docs/adr/0092-go-recovery-restoration-planning.md:52
	It("reports held intact selection while retaining the held blocker and all authority limits", func() {
		binary, root, _ := restorationFixture(assessmentPaths[:1])
		set := filepath.Join(root, ".factory/backups", durableRecoveryID)
		manifest := recoveryManifest(set)
		manifest["held"] = true
		recoveryWriteManifest(set, manifest)
		before := assessmentTree(root)
		object := restorationObject(restorationRun(binary, root, durableRecoveryID, true), root, true)
		Expect(object["assets"]).To(HaveLen(1))
		Expect(object["recovery"]).To(HaveKeyWithValue("held", true))
		Expect(object["blockers"]).To(Equal(append(append([]any(nil), restorationBlockers...), "recovery_held")))
		Expect(assessmentTree(root)).To(Equal(before))
	})

	// per docs/adr/0092-go-recovery-restoration-planning.md:54
	// per docs/adr/0092-go-recovery-restoration-planning.md:82
	DescribeTable("reports invalid requested recovery evidence without asset candidates or imported metadata", func(change, classification string) {
		binary, root, _ := restorationFixture(assessmentPaths[:1])
		set := filepath.Join(root, ".factory/backups", durableRecoveryID)
		manifestPath := filepath.Join(set, "manifest.json")
		copyPath := filepath.Join(set, "files", assessmentPaths[0])
		switch change {
		case "absent set":
			Expect(os.RemoveAll(set)).To(Succeed())
		case "absent backups":
			Expect(os.RemoveAll(filepath.Join(root, ".factory/backups"))).To(Succeed())
		case "missing manifest":
			Expect(os.Remove(manifestPath)).To(Succeed())
		case "missing copy":
			Expect(os.Remove(copyPath)).To(Succeed())
		case "changed copy":
			writeFixture(copyPath, []byte("PRIVATE_SAVED_DIFFERENCE"), 0600)
		case "arbitrary manifest":
			writeFixture(manifestPath, []byte(`{"PRIVATE_IMPORTED":"PRIVATE_CONTENT"}`), 0600)
		case "manifest oversized":
			writeFixture(manifestPath, []byte(strings.Repeat(" ", (16<<10)+1)), 0600)
		case "unsafe copy":
			Expect(os.Chmod(copyPath, 0644)).To(Succeed())
		case "copy symlink":
			Expect(os.Remove(copyPath)).To(Succeed())
			Expect(os.Symlink("/PRIVATE_OUTSIDE", copyPath)).To(Succeed())
		case "set symlink":
			Expect(os.Rename(set, set+".held")).To(Succeed())
			Expect(os.Symlink(set+".held", set)).To(Succeed())
		case "unsafe root":
			Expect(os.Chmod(filepath.Join(root, ".factory/backups"), 0755)).To(Succeed())
		case "unknown leaf":
			writeFixture(filepath.Join(set, "PRIVATE_EXTRA"), []byte("PRIVATE_UNKNOWN_PAYLOAD"), 0600)
		}
		before := assessmentTree(root)
		object := restorationObject(restorationRun(binary, root, durableRecoveryID, true), root, false)
		Expect(object["assets"]).To(BeEmpty())
		Expect(object["recovery"]).To(HaveKeyWithValue("classification", classification))
		Expect(object["blockers"]).To(Equal(append(append([]any(nil), restorationBlockers...), "recovery_not_integrity_checked")))
		for _, action := range restorationActions {
			Expect(object["counts"]).To(HaveKeyWithValue(action, json.Number("0")))
		}
		Expect(assessmentTree(root)).To(Equal(before))
	}, Entry("missing requested set", "absent set", "missing"), Entry("missing backups root", "absent backups", "missing"), Entry("missing manifest", "missing manifest", "unrecognized"), Entry("missing saved leaf", "missing copy", "incomplete"), Entry("changed saved leaf", "changed copy", "incomplete"), Entry("unknown manifest", "arbitrary manifest", "unrecognized"), Entry("oversized manifest", "manifest oversized", "limit_exceeded"), Entry("public saved mode", "unsafe copy", "unsafe"), Entry("saved link", "copy symlink", "unsafe"), Entry("set link", "set symlink", "unsafe"), Entry("unsafe recovery root", "unsafe root", "unsafe"), Entry("unknown extra leaf", "unknown leaf", "incomplete"))
})

var _ = Describe("Read-only recovery restoration planning installed paths", func() {
	// per docs/adr/0092-go-recovery-restoration-planning.md:61
	// per docs/adr/0092-go-recovery-restoration-planning.md:64
	DescribeTable("preserves ordinary customization and conflicts with unsafe installed paths", func(change, action string) {
		binary, root, _ := restorationFixture(assessmentPaths[:1])
		path := filepath.Join(root, assessmentPaths[0])
		switch change {
		case "content":
			writeFixture(path, []byte("PRIVATE_CUSTOM_ACTIVE"), 0755)
		case "mode":
			Expect(os.Chmod(path, 0700)).To(Succeed())
		case "group-writable file":
			Expect(os.Chmod(path, 0664)).To(Succeed())
		case "missing":
			Expect(os.Remove(path)).To(Succeed())
		case "missing ancestor":
			Expect(os.RemoveAll(filepath.Join(root, "scripts"))).To(Succeed())
		case "symlink":
			Expect(os.Rename(path, path+".held")).To(Succeed())
			Expect(os.Symlink(path+".held", path)).To(Succeed())
		case "hardlink":
			Expect(os.Link(path, path+".linked")).To(Succeed())
		case "FIFO":
			Expect(os.Remove(path)).To(Succeed())
			Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
		case "directory":
			Expect(os.Remove(path)).To(Succeed())
			Expect(os.Mkdir(path, 0700)).To(Succeed())
		case "oversized":
			writeFixture(path, []byte(strings.Repeat("x", (1<<20)+1)), 0755)
		case "special bits":
			Expect(os.Chmod(path, 0755|os.ModeSticky)).To(Succeed())
			info, err := os.Lstat(path)
			Expect(err).NotTo(HaveOccurred())
			if info.Mode()&os.ModeSticky == 0 {
				Skip("filesystem does not retain special mode fixture")
			}
		case "writable ancestry":
			Expect(os.Chmod(filepath.Join(root, "scripts"), 0770)).To(Succeed())
		case "ancestor link":
			path = filepath.Join(root, "scripts")
			Expect(os.Rename(path, path+".held")).To(Succeed())
			Expect(os.Symlink(path+".held", path)).To(Succeed())
		}
		before := assessmentTree(root)
		object := restorationObject(restorationRun(binary, root, durableRecoveryID, true), root, true)
		Expect(object["assets"]).To(HaveLen(1))
		Expect(object["assets"].([]any)[0]).To(HaveKeyWithValue("action", action))
		Expect(object["counts"]).To(HaveKeyWithValue(action, json.Number("1")))
		blockers := append([]any(nil), restorationBlockers...)
		if action == "conflict" || action == "preserve_customized" {
			blockers = append(blockers, "installed_conflicts")
		}
		Expect(object["blockers"]).To(Equal(blockers))
		Expect(assessmentTree(root)).To(Equal(before))
	}, Entry("later content", "content", "preserve_customized"), Entry("later ordinary mode", "mode", "preserve_customized"), Entry("ordinary group-writable file", "group-writable file", "preserve_customized"), Entry("missing path", "missing", "restore_missing_candidate"), Entry("missing safe ancestor", "missing ancestor", "restore_missing_candidate"), Entry("symlink", "symlink", "conflict"), Entry("hardlink", "hardlink", "conflict"), Entry("FIFO", "FIFO", "conflict"), Entry("directory", "directory", "conflict"), Entry("oversized regular file", "oversized", "conflict"), Entry("native special bits", "special bits", "conflict"), Entry("writable ancestry", "writable ancestry", "conflict"), Entry("linked ancestry", "ancestor link", "conflict"))
})

var _ = Describe("Read-only recovery restoration planning mutation boundary", func() {
	// per docs/adr/0092-go-recovery-restoration-planning.md:39
	It("uses only the physical current installation despite a different root in the environment", func() {
		binary, root, _ := restorationFixture(assessmentPaths[:1])
		current := GinkgoT().TempDir()
		current, err := filepath.EvalSymlinks(current)
		Expect(err).NotTo(HaveOccurred())
		before, currentBefore := assessmentTree(root), assessmentTree(current)
		object := restorationObject(restorationRun(binary, current, durableRecoveryID, true, "FACTORY_ROOT="+root, "FACTORY_INSTALLATION_ROOT="+root), current, false)
		Expect(object["assets"]).To(BeEmpty())
		Expect(object["recovery"]).To(HaveKeyWithValue("classification", "missing"))
		Expect(assessmentTree(root)).To(Equal(before))
		Expect(assessmentTree(current)).To(Equal(currentBefore))
	})

	// per docs/adr/0092-go-recovery-restoration-planning.md:100
	It("plans without acquiring the existing recovery creation lock", func() {
		binary, root, _ := restorationFixture(assessmentPaths[:1])
		lock, err := os.OpenFile(filepath.Join(root, ".git/info/factory-recovery.lock"), os.O_RDWR, 0)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(lock.Close)
		Expect(unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB)).To(Succeed())
		before := assessmentTree(root)
		object := restorationObject(restorationRun(binary, root, durableRecoveryID, true), root, true)
		Expect(object["assets"]).To(HaveLen(1))
		Expect(assessmentTree(root)).To(Equal(before))
	})

	// per docs/adr/0092-go-recovery-restoration-planning.md:57
	// per docs/adr/0092-go-recovery-restoration-planning.md:100
	DescribeTable("ignores unrelated execution configuration and preserves every observed state", func(change string) {
		binary, root, _ := restorationFixture(assessmentPaths[:1])
		var environment []string
		switch change {
		case "missing Git":
			Expect(os.RemoveAll(filepath.Join(root, ".git"))).To(Succeed())
			environment = []string{"PATH=" + binary}
		case "invalid configuration":
			writeFixture(filepath.Join(root, "factory.yaml"), []byte("PRIVATE_INVALID: [\n"), 0600)
		case "runtime FIFO":
			Expect(os.Remove(filepath.Join(root, ".factory/events.log"))).To(Succeed())
			Expect(syscall.Mkfifo(filepath.Join(root, ".factory/events.log"), 0600)).To(Succeed())
		case "unrelated sets":
			for index := range 70 {
				Expect(os.Mkdir(filepath.Join(root, ".factory/backups", "unrelated-"+strconv.Itoa(index)), 0700)).To(Succeed())
			}
			Expect(syscall.Mkfifo(filepath.Join(root, ".factory/backups/PRIVATE_UNRELATED_FIFO"), 0600)).To(Succeed())
		case "unselected unsafe":
			Expect(os.Remove(filepath.Join(root, assessmentPaths[1]))).To(Succeed())
			Expect(syscall.Mkfifo(filepath.Join(root, assessmentPaths[1]), 0600)).To(Succeed())
		case "poison commands":
			for _, name := range []string{"git", "bash", "curl", "codex", "opencode", "python3"} {
				writeFixture(filepath.Join(binary, name), []byte("#!/bin/sh\nprintf invoked > \"$PRIVATE_PLAN_PROCESS_MARKER\"\nexit 71\n"), 0700)
			}
			environment = []string{"PATH=" + binary, "PRIVATE_PLAN_PROCESS_MARKER=" + filepath.Join(binary, "PRIVATE_PROCESS_CALLED")}
		}
		marker := previewPoison(binary)
		before := assessmentTree(root)
		object := restorationObject(restorationRun(binary, root, durableRecoveryID, true, environment...), root, true)
		Expect(object["assets"]).To(HaveLen(1))
		Expect(object["blockers"]).To(Equal(restorationBlockers))
		Expect(assessmentTree(root)).To(Equal(before))
		publicNoPoison(marker)
		_, err := os.Lstat(filepath.Join(binary, "PRIVATE_PROCESS_CALLED"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("missing Git", "missing Git"), Entry("invalid unrelated YAML", "invalid configuration"), Entry("history is a FIFO", "runtime FIFO"), Entry("more than inventory limit in unrelated sets", "unrelated sets"), Entry("unselected unsafe path", "unselected unsafe"), Entry("all external tools poisoned", "poison commands"))
})
