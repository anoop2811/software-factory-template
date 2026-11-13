package acceptance_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// per docs/adr/0148-transactional-installed-ci-migration.md:39
func ciMigrationReport(proposal map[string]any) map[string]any {
	GinkgoHelper()
	Expect(proposal).To(HaveKey("ci_workflow"), "authenticated upgrade preview must expose its integrated CI proposal")
	report, ok := proposal["ci_workflow"].(map[string]any)
	Expect(ok).To(BeTrue())
	Expect(report).To(HaveKeyWithValue("schema_version", float64(1)))
	Expect(report).To(HaveKeyWithValue("mode", "ci-workflow-preview"))
	Expect(report).To(HaveKeyWithValue("apply_supported", true))
	Expect(report["reason"]).To(BeAssignableToTypeOf(""))
	Expect(report["reason"]).NotTo(BeEmpty())
	reason := strings.ToLower(report["reason"].(string))
	Expect(reason).To(ContainSubstring("confirmation"))
	Expect(reason).To(ContainSubstring("installation"))
	Expect(reason).To(ContainSubstring("workflow"))
	Expect(report).To(HaveKey("conflicts"))
	Expect(report["changes"]).To(HaveLen(5))
	return report
}

func ciMigrationRows(report map[string]any) map[string]map[string]any {
	GinkgoHelper()
	rows := map[string]map[string]any{}
	for _, value := range report["changes"].([]any) {
		row := value.(map[string]any)
		path := row["path"].(string)
		Expect(rows).NotTo(HaveKey(path))
		Expect(row["reason"]).NotTo(BeEmpty())
		Expect(row["disposition"]).To(BeElementOf("create", "replace", "retire", "preserve", "absent"))
		rows[path] = row
	}
	Expect(rows).To(HaveLen(5))
	return rows
}

// per docs/adr/0148-transactional-installed-ci-migration.md:55
func ciMigrationProposal(out cliResult, status int) map[string]any {
	GinkgoHelper()
	Expect(out.status).To(Equal(status), "%+v", out)
	var proposal map[string]any
	Expect(json.Unmarshal([]byte(out.stdout), &proposal)).To(Succeed())
	Expect(proposal["proposal_digest"]).To(MatchRegexp("^[a-f0-9]{64}$"))
	return proposal
}

// per docs/adr/0147-installed-ci-migration-preview.md:106
func ciMigrationGit(root string) []string {
	GinkgoHelper()
	return []string{imageGit(root, "rev-parse", "HEAD"), imageGit(root, "ls-files", "--stage", "-z"), imageGit(root, "status", "--porcelain=v1", "--untracked-files=all")}
}

var _ = Describe("Whole installation upgrade release candidate handoff initialized CI migration preview", Ordered, ContinueOnFailure, SpecPriority(2), Label("handoff-init-ci-project"), func() {
	var fixtures *defaultUpgradeJourneyFixtures
	BeforeAll(func() { fixtures = prepareDefaultUpgradeJourneys(true) })

	// per docs/adr/0147-installed-ci-migration-preview.md:23
	// per docs/adr/0147-installed-ci-migration-preview.md:106
	It("adds a separate authenticated JSON CI proposal without mutation", func() {
		p := fixtures.legacyProject()
		imageGit(p.root, "-c", "core.hooksPath="+os.DevNull, "commit", "--quiet", "--allow-empty", "-m", "test: establish preview Git baseline")
		gitBefore := ciMigrationGit(p.root)
		before := installationSnapshot(p.root)
		out := p.run("--dry-run", "--json")
		AddReportEntry("ci-migration.public-json", map[string]any{"status": out.status, "stdout": out.stdout, "stderr": out.stderr}, ReportEntryVisibilityAlways)
		Expect(out.status).To(BeZero(), "%+v", out)
		proposal := ciMigrationProposal(out, 0)
		report := ciMigrationReport(proposal)
		rows := ciMigrationRows(report)
		Expect(rows[".github/workflows/factory.yml"]["disposition"]).To(Equal("create"))
		for _, name := range []string{"go-pack.yml", "typescript-pack.yml", "java-pack.yml"} {
			Expect(rows[".github/workflows/"+name]["disposition"]).To(Equal("absent"))
		}
		Expect(rows[".github/workflows/ci.yml"]["disposition"]).To(Equal("retire"))
		old, err := os.ReadFile(filepath.Join(p.root, ".github/workflows/ci.yml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(rows[".github/workflows/ci.yml"]["before_sha256"]).To(Equal(fmt.Sprintf("%x", sha256.Sum256(old))))
		Expect(rows[".github/workflows/factory.yml"]["after_sha256"]).To(MatchRegexp("^[a-f0-9]{64}$"))
		Expect(report["conflicts"]).To(BeEmpty())
		Expect(installationSnapshot(p.root)).To(Equal(before))
		Expect(ciMigrationGit(p.root)).To(Equal(gitBefore))
		defaultInitNoForbidden(p.f)
	})

	// per docs/adr/0148-transactional-installed-ci-migration.md:31
	// per docs/adr/0148-transactional-installed-ci-migration.md:55
	It("binds preserved CI conflicts into consent and renders bounded confirmation advice", func() {
		p := fixtures.legacyProject()
		imageGit(p.root, "-c", "core.hooksPath="+os.DevNull, "commit", "--quiet", "--allow-empty", "-m", "test: establish preview Git baseline")
		baseline := p.preview()
		writeFixture(filepath.Join(p.root, ".github/workflows/ci.yml"), []byte("name: Adopter customization\non: workflow_dispatch\n"), 0644)
		writeFixture(filepath.Join(p.root, ".github/workflows/factory.yml"), []byte("name: Existing factory customization\n"), 0644)
		// An unselected dangling link remains outside the finite workflow selection.
		Expect(os.Symlink("/unread/application/workflow", filepath.Join(p.root, ".github/workflows/application.yml"))).To(Succeed())
		gitBefore := ciMigrationGit(p.root)
		before := installationSnapshot(p.root)
		out := p.run("--go-candidate", defaultUpgradeTag, "--dry-run", "--json")
		Expect(out.status).To(Equal(2), "CI conflicts block automatic combined migration: %+v", out)
		proposal := ciMigrationProposal(out, 2)
		Expect(proposal["proposal_digest"]).NotTo(Equal(baseline["proposal_digest"]))
		Expect(proposal["blockers"]).NotTo(BeEmpty())
		report := ciMigrationReport(proposal)
		rows := ciMigrationRows(report)
		Expect(rows[".github/workflows/ci.yml"]["disposition"]).To(Equal("preserve"))
		Expect(rows[".github/workflows/factory.yml"]["disposition"]).To(Equal("preserve"))
		Expect(report["conflicts"]).To(HaveLen(2))
		text := p.run("--dry-run")
		Expect(text.status).To(Equal(out.status), "%+v", text)
		Expect(text.stderr).To(BeEmpty())
		lower := strings.ToLower(text.stdout)
		Expect(lower).To(ContainSubstring("ci workflow"))
		Expect(lower).To(ContainSubstring("confirmation"))
		Expect(lower).To(ContainSubstring("installation"))
		Expect(lower).To(ContainSubstring("workflow"))
		Expect(lower).NotTo(ContainSubstring("apply unsupported"))
		Expect(lower).NotTo(ContainSubstring("read-only"))
		Expect(text.stdout).To(ContainSubstring("'--confirm' '" + proposal["proposal_digest"].(string) + "'"))
		Expect(lower).NotTo(ContainSubstring("ready for installed ci"))
		Expect(lower).NotTo(ContainSubstring("ready for release"))
		Expect(installationSnapshot(p.root)).To(Equal(before))
		Expect(ciMigrationGit(p.root)).To(Equal(gitBefore))
		defaultInitNoForbidden(p.f)
	})
})
