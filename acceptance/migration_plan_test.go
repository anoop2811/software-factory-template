package acceptance_test

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type planObservation struct {
	Classification string         `json:"classification"`
	Observed       *assessedBytes `json:"observed"`
}
type plannedAsset struct {
	Path      string          `json:"path"`
	Role      string          `json:"role"`
	Action    string          `json:"action"`
	Reason    string          `json:"reason"`
	Reference assessedBytes   `json:"reference"`
	Installed planObservation `json:"installed"`
	Source    planObservation `json:"source"`
}
type plannedMigration struct {
	SchemaVersion        int            `json:"schema_version"`
	ReferenceRevision    string         `json:"reference_revision"`
	Scope                string         `json:"scope"`
	PriorOrigin          string         `json:"prior_origin"`
	SourceAuthentication string         `json:"source_authentication"`
	OwnershipAuthorized  bool           `json:"ownership_authorized"`
	ActivationReady      bool           `json:"activation_ready"`
	RollbackReady        bool           `json:"rollback_ready"`
	Applicable           bool           `json:"applicable"`
	Blockers             []string       `json:"blockers"`
	Assets               []plannedAsset `json:"assets"`
	Counts               map[string]int `json:"counts"`
}

func migrationPlanCommand(binary, cwd string, args ...string) cliResult {
	return loopProcess(cwd, filepath.Join(binary, "factory"), append([]string{"migration", "plan"}, args...), nil)
}
func migrationPlanDecode(out cliResult, roots ...string) plannedMigration {
	GinkgoHelper()
	Expect(out.stderr).To(BeEmpty())
	Expect(out.stdout).To(HaveSuffix("\n"))
	for _, root := range roots {
		Expect(out.stdout).NotTo(ContainSubstring(root))
	}
	decoder := json.NewDecoder(strings.NewReader(out.stdout))
	decoder.DisallowUnknownFields()
	var plan plannedMigration
	Expect(decoder.Decode(&plan)).To(Succeed())
	var extra any
	Expect(decoder.Decode(&extra)).To(Equal(io.EOF))
	object := budgetDecode([]byte(out.stdout)).(map[string]any)
	Expect(object).To(HaveLen(12))
	for _, key := range []string{"ownership_authorized", "activation_ready", "rollback_ready", "applicable"} {
		Expect(object).To(HaveKeyWithValue(key, false))
	}
	Expect(plan.SchemaVersion).To(Equal(1))
	Expect(plan.ReferenceRevision).To(Equal(assessmentRevision))
	Expect(plan.Scope).To(Equal("g2-budget-loop-six"))
	Expect(plan.PriorOrigin).To(Equal("unproven"))
	Expect(plan.SourceAuthentication).To(Equal("unverified_local"))
	Expect(plan.Blockers).To(Equal([]string{"prior_origin_unproven", "target_authentication_unproven", "runtime_qualification_pending", "transition_quiescence_unproven", "verified_recovery_pending"}))
	Expect(plan.Assets).To(HaveLen(6))
	Expect(plan.Counts).To(HaveLen(8))
	for _, key := range []string{"retain", "replace_candidate", "add_candidate", "retire_candidate", "preserve_customized", "absent", "conflict", "assessment_error"} {
		Expect(plan.Counts).To(HaveKey(key))
	}
	for i, row := range plan.Assets {
		Expect(row.Path).To(Equal(assessmentPaths[i]))
		role := "compatibility_adapter"
		if i >= 3 {
			role = "legacy_implementation"
		}
		Expect(row.Role).To(Equal(role))
	}
	for _, raw := range object["assets"].([]any) {
		row := raw.(map[string]any)
		Expect(row).To(HaveLen(7))
		for _, key := range []string{"installed", "source"} {
			value := row[key].(map[string]any)
			Expect(value).To(HaveLen(2))
			Expect(value).To(HaveKey("observed"))
		}
	}
	return plan
}

var _ = Describe("G4 migration action planning core", func() {
	// per docs/adr/0080-go-migration-action-planning.md:91
	It("retains six identical references but remains explicitly blocked", func() {
		binary, installed, references := assessmentFixture()
		_, source, _ := assessmentFixture()
		out := migrationPlanCommand(binary, installed, installed, source)
		Expect(out.status).To(Equal(2))
		plan := migrationPlanDecode(out, installed, source)
		Expect(plan.Counts["retain"]).To(Equal(6))
		for i, row := range plan.Assets {
			Expect(row.Action).To(Equal("retain"))
			Expect(row.Reason).To(Equal("unchanged_target"))
			Expect(row.Reference).To(Equal(references[i]))
			Expect(row.Installed.Observed).To(Equal(&references[i]))
			Expect(row.Source.Observed).To(Equal(&references[i]))
		}
		Expect(migrationPlanCommand(binary, installed, installed, source)).To(Equal(out))
	})
	// per docs/adr/0080-go-migration-action-planning.md:75
	It("proposes retirement only for the three explicitly omitted Python paths", func() {
		binary, installed, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		for _, path := range assessmentPaths[3:] {
			Expect(os.Remove(filepath.Join(source, path))).To(Succeed())
		}
		out := migrationPlanCommand(binary, installed, installed, source)
		Expect(out.status).To(Equal(2))
		plan := migrationPlanDecode(out, installed, source)
		Expect(plan.Counts["retain"]).To(Equal(3))
		Expect(plan.Counts["retire_candidate"]).To(Equal(3))
		for _, row := range plan.Assets[3:] {
			Expect(row.Action).To(Equal("retire_candidate"))
			Expect(row.Reason).To(Equal("explicit_legacy_retirement"))
			Expect(row.Source.Classification).To(Equal("missing"))
			Expect(row.Source.Observed).To(BeNil())
		}
	})
	// per docs/adr/0080-go-migration-action-planning.md:80
	It("proposes replacement for different usable target bytes without claiming qualification", func() {
		binary, installed, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		writeFixture(filepath.Join(source, assessmentPaths[0]), []byte("PRIVATE_TARGET_CONTENT\n"), 0755)
		out := migrationPlanCommand(binary, installed, installed, source)
		Expect(out.status).To(Equal(2))
		plan := migrationPlanDecode(out, installed, source)
		Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_TARGET_CONTENT"))
		Expect(plan.Counts["replace_candidate"]).To(Equal(1))
		Expect(plan.Assets[0].Action).To(Equal("replace_candidate"))
		Expect(plan.Assets[0].Reason).To(Equal("target_differs"))
		Expect(plan.Assets[0].Source.Classification).To(Equal("customized"))
	})
})

var _ = Describe("G4 migration action planning precedence", func() {
	// per docs/adr/0080-go-migration-action-planning.md:64
	DescribeTable("follows explicit action precedence without inferring ownership", func(scenario, action, reason string) {
		binary, installed, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		index := 0
		if strings.Contains(scenario, "Python") {
			index = 3
		}
		installedPath := filepath.Join(installed, assessmentPaths[index])
		sourcePath := filepath.Join(source, assessmentPaths[index])
		switch scenario {
		case "missing compatibility":
			Expect(os.Remove(sourcePath)).To(Succeed())
		case "both missing compatibility", "both missing Python":
			Expect(os.Remove(sourcePath)).To(Succeed())
			Expect(os.Remove(installedPath)).To(Succeed())
		case "missing installed":
			Expect(os.Remove(installedPath)).To(Succeed())
		case "equal customization":
			writeFixture(installedPath, []byte("PRIVATE_IDENTICAL_CUSTOMIZATION"), 0755)
			writeFixture(sourcePath, []byte("PRIVATE_IDENTICAL_CUSTOMIZATION"), 0755)
		case "customized installed missing target":
			writeFixture(installedPath, []byte("custom"), 0755)
			Expect(os.Remove(sourcePath)).To(Succeed())
		case "customized installed unsafe target":
			writeFixture(installedPath, []byte("custom"), 0755)
			Expect(os.Remove(sourcePath)).To(Succeed())
			Expect(os.Symlink(installedPath, sourcePath)).To(Succeed())
		case "customized installed invalid target mode":
			writeFixture(installedPath, []byte("custom"), 0755)
			Expect(os.Chmod(sourcePath, 0600)).To(Succeed())
		case "changed present Python":
			writeFixture(sourcePath, []byte("PRIVATE_CHANGED_PYTHON"), 0644)
		case "installed special mode":
			Expect(os.Chmod(installedPath, 0755|os.ModeSetuid)).To(Succeed())
		case "installed mode customization":
			Expect(os.Chmod(installedPath, 0600)).To(Succeed())
		case "missing installed invalid source mode":
			Expect(os.Remove(installedPath)).To(Succeed())
			Expect(os.Chmod(sourcePath, 0600)).To(Succeed())
		}
		out := migrationPlanCommand(binary, installed, installed, source)
		Expect(out.status).To(Equal(2))
		plan := migrationPlanDecode(out, installed, source)
		row := plan.Assets[index]
		Expect(row.Action).To(Equal(action))
		Expect(row.Reason).To(Equal(reason))
		Expect(plan.Counts[action]).To(Equal(1))
		Expect(plan.Counts["retain"]).To(Equal(5))
		Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_"))
	}, Entry("missing compatibility", "missing compatibility", "conflict", "required_compatibility_path_missing"),
		Entry("both missing compatibility", "both missing compatibility", "conflict", "required_compatibility_path_missing"), Entry("both absent Python", "both missing Python", "absent", "absent_in_both"), Entry("addition", "missing installed", "add_candidate", "missing_installed_path"),
		Entry("equal target customization", "equal customization", "preserve_customized", "installed_customization"), Entry("customization before target absence", "customized installed missing target", "preserve_customized", "installed_customization"), Entry("unsafe before customization", "customized installed unsafe target", "conflict", "unsafe_path_or_source_mode"), Entry("invalid mode before customization", "customized installed invalid target mode", "conflict", "unsafe_path_or_source_mode"),
		Entry("Python retained when present", "changed present Python", "replace_candidate", "target_differs"), Entry("installed special mode preserved", "installed special mode", "preserve_customized", "installed_customization"), Entry("installed mode preserved", "installed mode customization", "preserve_customized", "installed_customization"), Entry("source mode before addition", "missing installed invalid source mode", "conflict", "unsafe_path_or_source_mode"))
	// per docs/adr/0080-go-migration-action-planning.md:111
	It("reports mixed actions in catalog order with exact zero-inclusive counts", func() {
		binary, installed, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		writeFixture(filepath.Join(source, assessmentPaths[0]), []byte("target"), 0755)
		Expect(os.Remove(filepath.Join(installed, assessmentPaths[1]))).To(Succeed())
		Expect(os.Remove(filepath.Join(source, assessmentPaths[2]))).To(Succeed())
		Expect(os.Remove(filepath.Join(source, assessmentPaths[3]))).To(Succeed())
		writeFixture(filepath.Join(installed, assessmentPaths[4]), []byte("custom"), 0644)
		Expect(os.Remove(filepath.Join(installed, assessmentPaths[5]))).To(Succeed())
		Expect(os.Remove(filepath.Join(source, assessmentPaths[5]))).To(Succeed())
		out := migrationPlanCommand(binary, installed, installed, source)
		Expect(out.status).To(Equal(2))
		plan := migrationPlanDecode(out, installed, source)
		Expect(plan.Counts).To(Equal(map[string]int{"retain": 0, "replace_candidate": 1, "add_candidate": 1, "retire_candidate": 1, "preserve_customized": 1, "absent": 1, "conflict": 1, "assessment_error": 0}))
		for i, action := range []string{"replace_candidate", "add_candidate", "conflict", "retire_candidate", "preserve_customized", "absent"} {
			Expect(plan.Assets[i].Action).To(Equal(action))
		}
	})
})
var _ = Describe("G4 migration action planning source safety", func() {
	// per docs/adr/0080-go-migration-action-planning.md:44
	DescribeTable("keeps all source permission and special bits in eligibility decisions", func(mode os.FileMode) {
		binary, installed, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		Expect(os.Chmod(filepath.Join(source, assessmentPaths[0]), mode)).To(Succeed())
		out := migrationPlanCommand(binary, installed, installed, source)
		Expect(out.status).To(Equal(2))
		plan := migrationPlanDecode(out, installed, source)
		Expect(plan.Assets[0].Action).To(Equal("conflict"))
		Expect(plan.Assets[0].Reason).To(Equal("unsafe_path_or_source_mode"))
		Expect(plan.Assets[0].Source.Classification).To(Equal("customized"))
		if mode.Perm() == 0755 {
			Expect(plan.Assets[0].Source.Observed.Mode).To(Equal(plan.Assets[0].Reference.Mode))
		}
	}, Entry("ordinary different mode", os.FileMode(0644)), Entry("setuid", os.FileMode(0755)|os.ModeSetuid), Entry("setgid", os.FileMode(0755)|os.ModeSetgid), Entry("sticky", os.FileMode(0755)|os.ModeSticky))
	// per docs/adr/0080-go-migration-action-planning.md:36
	DescribeTable("conflicts on unsafe observations from either tree", func(side, kind string) {
		binary, installed, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		tree := source
		if side == "installed" {
			tree = installed
		}
		path := filepath.Join(tree, assessmentPaths[0])
		switch kind {
		case "symlink":
			Expect(os.Remove(path)).To(Succeed())
			Expect(os.Symlink("/dev/null", path)).To(Succeed())
		case "hardlink":
			Expect(os.Link(path, filepath.Join(binary, "extra-link"))).To(Succeed())
		case "FIFO":
			Expect(os.Remove(path)).To(Succeed())
			Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
		case "directory":
			Expect(os.Remove(path)).To(Succeed())
			Expect(os.Mkdir(path, 0700)).To(Succeed())
		case "oversized":
			writeFixture(path, []byte(strings.Repeat("x", (1<<20)+1)), 0755)
		}
		out := migrationPlanCommand(binary, installed, installed, source)
		Expect(out.status).To(Equal(2))
		plan := migrationPlanDecode(out, installed, source)
		Expect(plan.Assets[0].Action).To(Equal("conflict"))
		Expect(plan.Assets[0].Reason).To(Equal("unsafe_path_or_source_mode"))
		observed := plan.Assets[0].Source
		if side == "installed" {
			observed = plan.Assets[0].Installed
		}
		Expect(observed.Classification).To(Equal("unsafe"))
		Expect(observed.Observed).To(BeNil())
	}, Entry("source symlink", "source", "symlink"), Entry("source hardlink", "source", "hardlink"), Entry("source FIFO", "source", "FIFO"), Entry("source directory", "source", "directory"), Entry("source oversized", "source", "oversized"), Entry("installed symlink", "installed", "symlink"), Entry("installed hardlink", "installed", "hardlink"), Entry("installed FIFO", "installed", "FIFO"), Entry("installed directory", "installed", "directory"), Entry("installed oversized", "installed", "oversized"))
	// per docs/adr/0080-go-migration-action-planning.md:66
	DescribeTable("keeps assessment errors ahead of conflicts or preserved customizations", func(side string) {
		if os.Geteuid() == 0 {
			Skip("requires unprivileged read checks")
		}
		binary, installed, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		tree := source
		other := installed
		if side == "installed" {
			tree, other = installed, source
		}
		unreadable := filepath.Join(tree, assessmentPaths[0])
		Expect(os.Chmod(unreadable, 0000)).To(Succeed())
		DeferCleanup(func() { Expect(os.Chmod(unreadable, 0600)).To(Succeed()) })
		writeFixture(filepath.Join(other, assessmentPaths[0]), []byte("custom"), 0755)
		out := migrationPlanCommand(binary, installed, installed, source)
		Expect(out.status).To(Equal(1))
		plan := migrationPlanDecode(out, installed, source)
		Expect(plan.Assets[0].Action).To(Equal("assessment_error"))
		Expect(plan.Assets[0].Reason).To(Equal("cannot_assess_path"))
		Expect(plan.Counts["assessment_error"]).To(Equal(1))
	}, Entry("source", "source"), Entry("installed", "installed"))
})
var _ = Describe("G4 migration action planning authority and roots", func() {
	// per docs/adr/0080-go-migration-action-planning.md:97
	It("leaves both trees unchanged and ignores all caller-provided ownership claims", func() {
		binary, installed, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		marker := filepath.Join(binary, "CALLED")
		bin := filepath.Join(binary, "bin")
		for _, name := range []string{"git", "bash", "python3", "codex", "claude", "opencode"} {
			writeFixture(filepath.Join(bin, name), []byte("#!/bin/sh\nprintf called > '"+marker+"'\nexit 99\n"), 0700)
		}
		for _, tree := range []string{installed, source} {
			for _, path := range []string{".factory-version", ".factory/ownership.json", "docs/migration/baseline-assets.json", "target-intent.json"} {
				writeFixture(filepath.Join(tree, path), []byte(`{"trusted":true,"ownership_authorized":true,"retire":["user-policy"]}`), 0600)
			}
			writeFixture(filepath.Join(tree, "user-policy"), []byte("PRIVATE_POLICY"), 0600)
			Expect(syscall.Mkfifo(filepath.Join(tree, "extra-fifo"), 0600)).To(Succeed())
		}
		beforeInstalled, beforeSource := assessmentTree(installed), assessmentTree(source)
		out := loopProcess(installed, filepath.Join(binary, "factory"), []string{"migration", "plan", installed, source}, []string{"PATH=" + bin, "FACTORY_OWNERSHIP_AUTHORIZED=true", "FACTORY_MIGRATION_TRUSTED=true", "PRIVATE_TOKEN=never-print"})
		Expect(out.status).To(Equal(2))
		plan := migrationPlanDecode(out, installed, source)
		Expect(plan.Counts["retain"]).To(Equal(6))
		Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_"))
		Expect(out.stdout).NotTo(ContainSubstring("never-print"))
		publicNoPoison(marker)
		Expect(assessmentTree(installed)).To(Equal(beforeInstalled))
		Expect(assessmentTree(source)).To(Equal(beforeSource))
	})
	// per docs/adr/0080-go-migration-action-planning.md:40
	It("allows identical roots while leaving every trust prerequisite unresolved", func() {
		binary, root, _ := assessmentFixture()
		out := migrationPlanCommand(binary, root, root, root)
		Expect(out.status).To(Equal(2))
		plan := migrationPlanDecode(out, root)
		Expect(plan.Counts["retain"]).To(Equal(6))
	})
	// per docs/adr/0080-go-migration-action-planning.md:29
	It("accepts a literal dash-prefixed source directory", func() {
		binary, installed, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		target := filepath.Join(filepath.Dir(source), "--help")
		Expect(os.Rename(source, target)).To(Succeed())
		out := migrationPlanCommand(binary, filepath.Dir(target), installed, "--help")
		Expect(out.status).To(Equal(2))
		plan := migrationPlanDecode(out, installed, target)
		Expect(plan.Counts["retain"]).To(Equal(6))
	})
	// per docs/adr/0080-go-migration-action-planning.md:118
	DescribeTable("rejects invalid or unavailable roots without a plan", func(side, kind string, status int) {
		binary, installed, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		operand := source
		if side == "installed" {
			operand = installed
		}
		switch kind {
		case "missing":
			operand = filepath.Join(operand, "missing")
		case "parent":
			operand += "/../" + filepath.Base(operand)
		case "symlink":
			alias := filepath.Join(binary, "alias")
			Expect(os.Symlink(operand, alias)).To(Succeed())
			operand = alias
		case "file":
			operand = filepath.Join(operand, assessmentPaths[0])
		}
		if side == "installed" {
			installed = operand
		} else {
			source = operand
		}
		out := migrationPlanCommand(binary, binary, installed, source)
		Expect(out.status).To(Equal(status))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).NotTo(BeEmpty())
		Expect(out.stderr).NotTo(ContainSubstring(binary))
	}, Entry("missing source", "source", "missing", 1), Entry("missing installed", "installed", "missing", 1), Entry("parent source", "source", "parent", 2), Entry("parent installed", "installed", "parent", 2), Entry("symlink source", "source", "symlink", 2), Entry("symlink installed", "installed", "symlink", 2), Entry("file source", "source", "file", 2), Entry("file installed", "installed", "file", 2))
	// per docs/adr/0080-go-migration-action-planning.md:29
	DescribeTable("requires exactly two literal roots", func(args []string) {
		binary, cwd := fixture()
		out := migrationPlanCommand(binary, cwd, args...)
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).NotTo(BeEmpty())
	}, Entry("none", []string{}), Entry("one", []string{"."}), Entry("three", []string{".", ".", "extra"}))
})
