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

func upgradePreview(binary, root string, args ...string) cliResult {
	return publicCommand(binary, root, "upgrade", args, nil)
}
func previewObject(out cliResult) map[string]any {
	GinkgoHelper()
	Expect(out.status).To(Equal(2), "%+v", out)
	Expect(out.stderr).To(BeEmpty())
	Expect(out.stdout).To(HaveSuffix("\n"))
	decoder := json.NewDecoder(strings.NewReader(out.stdout))
	decoder.UseNumber()
	var object map[string]any
	Expect(decoder.Decode(&object)).To(Succeed())
	var extra any
	Expect(decoder.Decode(&extra)).To(Equal(io.EOF))
	Expect(object).To(HaveLen(8))
	Expect(object).To(HaveKeyWithValue("schema_version", json.Number("1")))
	Expect(object).To(HaveKeyWithValue("mode", "dry_run"))
	Expect(object).To(HaveKeyWithValue("coverage", "partial"))
	Expect(object).To(HaveKeyWithValue("recovery_assessment", "not_assessed"))
	plan := object["plan"].(map[string]any)
	Expect(plan).To(HaveKeyWithValue("scope", "g2-budget-loop-six"))
	Expect(plan).To(HaveKeyWithValue("prior_origin", "unproven"))
	for _, key := range []string{"activation_ready", "rollback_ready", "applicable"} {
		Expect(plan).To(HaveKeyWithValue(key, false))
	}
	Expect(plan["assets"]).To(HaveLen(6))
	Expect(plan["counts"]).To(HaveLen(8))
	return object
}

var _ = Describe("G4 public upgrade preview core", func() {
	// per docs/adr/0082-go-public-upgrade-preview.md:87
	It("emits a blocked partial JSON plan without colocated scripts", func() {
		binary, root, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		before := assessmentTree(root)
		out := upgradePreview(binary, root, "--dry-run", "--source", source, "--json")
		object := previewObject(out)
		Expect(object).To(HaveKey("adoption_proposal"))
		Expect(object["adoption_proposal"]).To(BeNil())
		Expect(object).To(HaveKeyWithValue("ownership_basis", "unproven"))
		Expect(object["authorized_paths"]).To(Equal([]any{}))
		expected := migrationPlanCommand(binary, root, root, source)
		Expect(object["plan"]).To(Equal(budgetDecode([]byte(expected.stdout))))
		Expect(out.stdout).NotTo(ContainSubstring(root))
		Expect(out.stdout).NotTo(ContainSubstring(source))
		Expect(assessmentTree(root)).To(Equal(before))
	})
	// per docs/adr/0082-go-public-upgrade-preview.md:23
	It("reserves preview before a colocated mutating upgrade script", func() {
		binary, root, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		marker := filepath.Join(binary, "MUTATING_UPGRADE_CALLED")
		writeFixture(filepath.Join(binary, "scripts/factory-upgrade.sh"), []byte("#!/bin/sh\nprintf called > '"+marker+"'\nexit 99\n"), 0700)
		previewObject(upgradePreview(binary, root, "--source="+source, "--json", "--dry-run"))
		publicNoPoison(marker)
	})
	// per docs/adr/0082-go-public-upgrade-preview.md:59
	DescribeTable("keeps selected adoption unconfirmed or explicitly confirmed while blocked", func(confirmed bool) {
		binary, root, references := assessmentFixture()
		_, source, _ := assessmentFixture()
		proposal := adoptionFixtureProposal(root, references)
		args := []string{"--dry-run", "--source", source, "--json"}
		for _, path := range assessmentPaths {
			args = append(args, "--adopt-path", path)
		}
		if confirmed {
			args = append(args, "--confirm-adoption", proposal.ProposalDigest)
		}
		before := assessmentTree(root)
		object := previewObject(upgradePreview(binary, root, args...))
		selected := object["adoption_proposal"].(map[string]any)
		Expect(selected).To(HaveKeyWithValue("proposal_digest", proposal.ProposalDigest))
		Expect(selected).To(HaveKeyWithValue("ownership_authorized", false))
		plan := object["plan"].(map[string]any)
		Expect(plan).To(HaveKeyWithValue("ownership_authorized", confirmed))
		if confirmed {
			Expect(object).To(HaveKeyWithValue("ownership_basis", "explicit_operator_adoption"))
			Expect(object["authorized_paths"]).To(Equal(toAnyStrings(assessmentPaths)))
		} else {
			Expect(object).To(HaveKeyWithValue("ownership_basis", "unconfirmed"))
			Expect(object["authorized_paths"]).To(Equal([]any{}))
			Expect(plan["blockers"]).To(ContainElement("prior_origin_unproven"))
		}
		Expect(assessmentTree(root)).To(Equal(before))
	}, Entry("unconfirmed", false), Entry("confirmed", true))
})

func previewPoison(binary string) string {
	GinkgoHelper()
	marker := filepath.Join(binary, "PREVIEW_EXECUTED_SUBPROCESS")
	script := []byte("#!/bin/sh\nprintf called > '" + marker + "'\nprintf PRIVATE_POISON >&2\nexit 97\n")
	writeFixture(filepath.Join(binary, "scripts/factory-upgrade.sh"), script, 0700)
	for _, executable := range []string{"git", "python3", "codex", "claude", "opencode", "bash", "sh"} {
		writeFixture(filepath.Join(binary, "poison", executable), script, 0700)
	}
	return marker
}
func previewSelectedArgs(source string, selection []string, digest string) []string {
	args := []string{"--dry-run", "--source=" + source, "--json"}
	for _, path := range selection {
		args = append(args, "--adopt-path="+path)
	}
	if digest != "" {
		args = append(args, "--confirm-adoption="+digest)
	}
	return args
}

var _ = Describe("G4 public upgrade preview admission", func() {
	// per docs/adr/0082-go-public-upgrade-preview.md:32
	DescribeTable("refuses malformed reserved previews before any legacy execution or writes", func(tail []string) {
		binary, root, _ := assessmentFixture()
		marker := previewPoison(binary)
		before := assessmentTree(root)
		out := upgradePreview(binary, root, tail...)
		Expect(out.status).To(Equal(2), "%+v", out)
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).To(HavePrefix("factory upgrade:"))
		Expect(strings.Count(out.stderr, "\n")).To(Equal(1))
		Expect(out.stderr).NotTo(ContainSubstring("PRIVATE_"))
		Expect(assessmentTree(root)).To(Equal(before))
		publicNoPoison(marker)
	},
		Entry("missing source", []string{"--dry-run"}),
		Entry("empty source", []string{"--dry-run", "--source="}),
		Entry("source lacking operand", []string{"--dry-run", "--source"}),
		Entry("attached true dry run", []string{"--dry-run=true", "--source=."}),
		Entry("attached false dry run", []string{"--dry-run=false", "--source=."}),
		Entry("empty dry run", []string{"--dry-run=", "--source=."}),
		Entry("duplicate dry run", []string{"--dry-run", "--dry-run", "--source=."}),
		Entry("duplicate source", []string{"--dry-run", "--source=.", "--source=."}),
		Entry("duplicate json", []string{"--dry-run", "--source=.", "--json", "--json"}),
		Entry("valued json", []string{"--dry-run", "--source=.", "--json=true"}),
		Entry("empty json", []string{"--dry-run", "--source=.", "--json="}),
		Entry("unknown secret flag", []string{"--dry-run", "--source=.", "--PRIVATE_UNKNOWN"}),
		Entry("unknown operand", []string{"--dry-run", "--source=.", "PRIVATE_OPERAND"}),
		Entry("ref incompatible", []string{"--dry-run", "--source=.", "--ref=PRIVATE_REF"}),
		Entry("separator", []string{"--dry-run", "--source=.", "--"}),
		Entry("help", []string{"--dry-run", "--source=.", "--help"}),
		Entry("short help", []string{"--dry-run", "--source=.", "-h"}),
		Entry("marker used as source operand", []string{"--source", "--dry-run"}),
		Entry("marker used as adoption operand", []string{"--source=.", "--adopt-path", "--dry-run"}),
		Entry("option looking separate source", []string{"--dry-run", "--source", "-PRIVATE_PATH"}),
		Entry("confirmation lacks selection", []string{"--dry-run", "--source=.", "--confirm-adoption=" + strings.Repeat("a", 64)}),
		Entry("empty confirmation", []string{"--dry-run", "--source=.", "--adopt-path=scripts/factory-budget.sh", "--confirm-adoption="}),
		Entry("empty selection", []string{"--dry-run", "--source=.", "--adopt-path="}),
		Entry("duplicate selection", []string{"--dry-run", "--source=.", "--adopt-path=scripts/factory-budget.sh", "--adopt-path=scripts/factory-budget.sh"}),
		Entry("selection alias", []string{"--dry-run", "--source=.", "--adopt-path=./scripts/factory-budget.sh"}),
		Entry("selection unknown", []string{"--dry-run", "--source=.", "--adopt-path=PRIVATE_UNKNOWN"}),
		Entry("duplicate confirmation", []string{"--dry-run", "--source=.", "--adopt-path=scripts/factory-budget.sh", "--confirm-adoption=" + strings.Repeat("a", 64), "--confirm-adoption=" + strings.Repeat("a", 64)}))
	// per docs/adr/0082-go-public-upgrade-preview.md:23
	DescribeTable("keeps literal legacy dispatch when no standalone preview marker appears", func(args []string) {
		binary, root, _ := assessmentFixture()
		writeFixture(filepath.Join(binary, "scripts/factory-upgrade.sh"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\nexit 37\n"), 0700)
		out := upgradePreview(binary, root, args...)
		Expect(out).To(Equal(cliResult{strings.Join(args, "\n") + "\n", "", 37}))
	}, Entry("unknown apply option", []string{"--PRIVATE_APPLY"}),
		Entry("attached marker is source literal", []string{"--source=--dry-run"}),
		Entry("prefixed literal marker", []string{"./--dry-run"}),
		Entry("similar option", []string{"--dry-running"}))
	// per docs/adr/0082-go-public-upgrade-preview.md:30
	It("keeps private protocol precedence", func() {
		binary, root, _ := assessmentFixture()
		marker := previewPoison(binary)
		out := loopProcess(root, filepath.Join(binary, "factory"), []string{"upgrade", "--dry-run", "--source=.", "--json"}, nil)
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).To(HavePrefix("factory bridge:"))
		publicNoPoison(marker)
	})
})
var _ = Describe("G4 public upgrade preview observations", func() {
	// per docs/adr/0082-go-public-upgrade-preview.md:51
	DescribeTable("resolves only current directory and literal source without discovery", func(sourceName string) {
		binary, root, _ := assessmentFixture()
		source := filepath.Join(root, sourceName)
		Expect(os.Mkdir(source, 0700)).To(Succeed())
		marker := previewPoison(binary)
		writeFixture(filepath.Join(root, "factory.yaml"), []byte("PRIVATE_INVALID: [\n"), 0600)
		writeFixture(filepath.Join(root, ".factory/recovery/PRIVATE_BACKUP"), []byte("PRIVATE_BACKUP_CONTENT"), 0600)
		before := assessmentTree(root)
		out := publicCommand(binary, root, "upgrade", []string{"--dry-run", "--source=" + sourceName, "--json"}, []string{"PATH=" + filepath.Join(binary, "poison"), "PWD=/PRIVATE_WRONG_ROOT", "FACTORY_ROOT=/PRIVATE_WRONG_ROOT", "FACTORY_CONFIG=/PRIVATE_MISSING", "FACTORY_ADOPTION_DIGEST=" + strings.Repeat("a", 64)})
		object := previewObject(out)
		Expect(object["ownership_basis"]).To(Equal("unproven"))
		Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_"))
		Expect(assessmentTree(root)).To(Equal(before))
		publicNoPoison(marker)
	}, Entry("relative source", "target"), Entry("attached flag-looking source", "--dry-run"))
	// per docs/adr/0082-go-public-upgrade-preview.md:54
	It("assesses the invoked subdirectory rather than finding a parent installation", func() {
		binary, root, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		child := filepath.Join(root, "subdirectory")
		Expect(os.Mkdir(child, 0700)).To(Succeed())
		object := previewObject(upgradePreview(binary, child, "--dry-run", "--source", source, "--json"))
		plan := object["plan"].(map[string]any)
		Expect(plan["counts"].(map[string]any)["add_candidate"]).To(Equal(json.Number("6")))
	})
	// per docs/adr/0082-go-public-upgrade-preview.md:96
	DescribeTable("distinguishes root refusal and missing root without a report", func(kind string, status int) {
		binary, root, _ := assessmentFixture()
		source := filepath.Join(root, "PRIVATE_TARGET")
		if kind == "symlink" {
			Expect(os.Symlink(root, source)).To(Succeed())
		}
		if kind == "file" {
			writeFixture(source, []byte("PRIVATE_CONTENT"), 0600)
		}
		out := upgradePreview(binary, root, "--dry-run", "--source", source, "--json")
		Expect(out.status).To(Equal(status))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).NotTo(ContainSubstring("PRIVATE_"))
		Expect(strings.Count(out.stderr, "\n")).To(Equal(1))
	}, Entry("missing", "missing", 1), Entry("symlink", "symlink", 2), Entry("regular file", "file", 2))
	// per docs/adr/0082-go-public-upgrade-preview.md:96
	It("reports an unselected unsafe leaf as a blocked conflict", func() {
		binary, root, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		path := filepath.Join(root, assessmentPaths[0])
		Expect(os.Remove(path)).To(Succeed())
		Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
		object := previewObject(upgradePreview(binary, root, "--dry-run", "--source", source, "--json"))
		rows := object["plan"].(map[string]any)["assets"].([]any)
		Expect(rows[0].(map[string]any)["action"]).To(Equal("conflict"))
	})
	// per docs/adr/0082-go-public-upgrade-preview.md:64
	It("confirms only the selected partial scope and preserves customized unselected files", func() {
		binary, root, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		writeFixture(filepath.Join(root, assessmentPaths[1]), []byte("PRIVATE_CUSTOM_CONTENT"), 0600)
		selection := []string{assessmentPaths[0]}
		proposal := adoptionDecode(adoptionCommand(binary, root, "propose-adoption", root, selection[0]), root)
		before := assessmentTree(root)
		object := previewObject(upgradePreview(binary, root, previewSelectedArgs(source, selection, proposal.ProposalDigest)...))
		Expect(object["ownership_basis"]).To(Equal("explicit_operator_adoption"))
		Expect(object["authorized_paths"]).To(Equal(toAnyStrings(selection)))
		plan := object["plan"].(map[string]any)
		Expect(plan["ownership_authorized"]).To(BeFalse())
		Expect(plan["blockers"]).To(ContainElement("ownership_scope_incomplete"))
		Expect(plan["assets"].([]any)[1].(map[string]any)["action"]).To(Equal("preserve_customized"))
		Expect(assessmentTree(root)).To(Equal(before))
	})
	// per docs/adr/0082-go-public-upgrade-preview.md:97
	DescribeTable("refuses stale copied or invalid confirmation and selected unsafe assets", func(change string) {
		binary, root, references := assessmentFixture()
		_, source, _ := assessmentFixture()
		proposal := adoptionFixtureProposal(root, references)
		digest := proposal.ProposalDigest
		path := filepath.Join(root, assessmentPaths[0])
		switch change {
		case "copied":
			_, root, _ = assessmentFixture()
		case "replaced":
			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Rename(path, path+".held")).To(Succeed())
			writeFixture(path, data, 0755)
		case "mode":
			Expect(os.Chmod(path, 0700)).To(Succeed())
		case "content":
			writeFixture(path, []byte("PRIVATE_CUSTOM_CONTENT"), 0600)
		case "uppercase":
			digest = strings.ToUpper(digest)
		case "malformed":
			digest = "PRIVATE_DIGEST"
		case "mismatch":
			digest = strings.Repeat("0", 64)
		case "fifo":
			Expect(os.Remove(path)).To(Succeed())
			Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
		}
		before := assessmentTree(root)
		out := upgradePreview(binary, root, previewSelectedArgs(source, assessmentPaths, digest)...)
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).NotTo(ContainSubstring("PRIVATE_"))
		Expect(strings.Count(out.stderr, "\n")).To(Equal(1))
		Expect(assessmentTree(root)).To(Equal(before))
	}, Entry("copied root", "copied"), Entry("same bytes new identity", "replaced"), Entry("mode changed", "mode"), Entry("content changed", "content"), Entry("uppercase", "uppercase"), Entry("malformed", "malformed"), Entry("mismatch", "mismatch"), Entry("selected FIFO", "fifo"))
	// per docs/adr/0082-go-public-upgrade-preview.md:76
	It("renders deterministic text exposing every action count blocker and adoption instruction", func() {
		binary, root, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		args := []string{"--dry-run", "--source", source, "--adopt-path", assessmentPaths[0]}
		out := upgradePreview(binary, root, args...)
		Expect(out.status).To(Equal(2))
		Expect(out.stderr).To(BeEmpty())
		for _, token := range []string{"read-only", "partial", "6", "retain", "replace_candidate", "add_candidate", "retire_candidate", "preserve_customized", "absent", "conflict", "assessment_error", "unconfirmed", "--confirm-adoption", "prior_origin_unproven", "target_authentication_unproven", "runtime_qualification_pending", "transition_quiescence_unproven", "verified_recovery_pending"} {
			Expect(strings.ToLower(out.stdout)).To(ContainSubstring(token))
		}
		for _, path := range assessmentPaths {
			Expect(out.stdout).To(ContainSubstring(path))
		}
		Expect(strings.ToLower(out.stdout)).To(ContainSubstring("not assessed"))
		Expect(strings.ToLower(out.stdout)).To(ContainSubstring("legacy"))
		Expect(out.stdout).To(ContainSubstring("--dry-run"))
		Expect(out.stdout).NotTo(ContainSubstring(root))
		Expect(out.stdout).NotTo(ContainSubstring(source))
		Expect(upgradePreview(binary, root, args...)).To(Equal(out))
	})
})

var _ = Describe("G4 public upgrade preview safe reasons", func() {
	// per docs/adr/0082-go-public-upgrade-preview.md:96
	DescribeTable("explains sanitized assessment refusal without disclosing operands", func(kind string, reason string) {
		binary, root, references := assessmentFixture()
		_, source, _ := assessmentFixture()
		digest := adoptionFixtureProposal(root, references).ProposalDigest
		args := previewSelectedArgs(source, assessmentPaths, digest)
		switch kind {
		case "confirmation":
			args = previewSelectedArgs(source, assessmentPaths, strings.Repeat("0", 64))
		case "asset":
			writeFixture(filepath.Join(root, assessmentPaths[0]), []byte("PRIVATE_CONTENT"), 0600)
		case "root":
			link := filepath.Join(root, "PRIVATE_LINK")
			Expect(os.Symlink(source, link)).To(Succeed())
			args = []string{"--dry-run", "--source", link, "--json"}
		}
		out := upgradePreview(binary, root, args...)
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).To(Equal("factory upgrade: " + reason + "\n"))
	}, Entry("confirmation mismatch", "confirmation", "adoption confirmation does not match current observations"), Entry("selected asset changed", "asset", "adoption requires unchanged reference assets"), Entry("unsafe root", "root", "unsafe assessment root"))
})

var _ = Describe("G4 public upgrade preview action projection", func() {
	// per docs/adr/0082-go-public-upgrade-preview.md:76
	DescribeTable("projects each planner action and all counts into JSON and text without writes", func(action string) {
		binary, root, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		index := 0
		if action == "retire_candidate" || action == "absent" {
			index = 3
		}
		installedPath, targetPath := filepath.Join(root, assessmentPaths[index]), filepath.Join(source, assessmentPaths[index])
		switch action {
		case "replace_candidate":
			writeFixture(targetPath, []byte("PRIVATE_NEW_TARGET"), 0600)
		case "add_candidate":
			Expect(os.Remove(installedPath)).To(Succeed())
		case "retire_candidate":
			Expect(os.Remove(targetPath)).To(Succeed())
		case "absent":
			Expect(os.Remove(installedPath)).To(Succeed())
			Expect(os.Remove(targetPath)).To(Succeed())
		case "preserve_customized":
			writeFixture(installedPath, []byte("PRIVATE_LOCAL_CONTENT"), 0600)
		case "conflict":
			Expect(os.Remove(targetPath)).To(Succeed())
		case "assessment_error":
			if os.Geteuid() == 0 {
				Skip("requires host permission denial")
			}
			Expect(os.Chmod(installedPath, 0000)).To(Succeed())
			DeferCleanup(os.Chmod, installedPath, os.FileMode(0755))
		}
		expected := migrationPlanCommand(binary, root, root, source)
		wantStatus := 2
		if action == "assessment_error" {
			wantStatus = 1
		}
		Expect(expected.status).To(Equal(wantStatus))
		var before any
		if action != "assessment_error" {
			before = assessmentTree(root)
		}
		actual := upgradePreview(binary, root, "--dry-run", "--source", source, "--json")
		Expect(actual.status).To(Equal(wantStatus))
		Expect(actual.stderr).To(BeEmpty())
		object := budgetDecode([]byte(actual.stdout)).(map[string]any)
		plan := budgetDecode([]byte(expected.stdout)).(map[string]any)
		Expect(object["plan"]).To(Equal(plan))
		Expect(plan["assets"].([]any)[index].(map[string]any)["action"]).To(Equal(action))
		text := upgradePreview(binary, root, "--dry-run", "--source", source)
		Expect(text.status).To(Equal(wantStatus))
		Expect(text.stderr).To(BeEmpty())
		for _, entry := range plan["assets"].([]any) {
			row := entry.(map[string]any)
			Expect(text.stdout).To(ContainSubstring(row["path"].(string)))
			Expect(text.stdout).To(ContainSubstring(row["reason"].(string)))
		}
		for label, count := range plan["counts"].(map[string]any) {
			Expect(text.stdout).To(ContainSubstring(label + ": " + count.(json.Number).String()))
		}
		Expect(text.stdout).NotTo(ContainSubstring("PRIVATE_"))
		Expect(text.stdout).To(ContainSubstring("Rollback ready: false"))
		Expect(text.stdout).To(ContainSubstring("Applicable: false"))
		if before != nil {
			Expect(assessmentTree(root)).To(Equal(before))
		}
	}, Entry("retain", "retain"), Entry("replace", "replace_candidate"), Entry("add", "add_candidate"), Entry("retire", "retire_candidate"), Entry("customized", "preserve_customized"), Entry("absent", "absent"), Entry("conflict", "conflict"), Entry("I/O", "assessment_error"))
})
