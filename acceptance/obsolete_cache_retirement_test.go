package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The unchanged historical role map is evaluator data, not the new role selector.
// per docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:66
func obsoleteCacheOracle() []string {
	GinkgoHelper()
	command := exec.Command("git", "--no-replace-objects", "show", minimalInventoryPredecessor+":docs/migration/INSTALLATION_ROLE_MAP.json") // #nosec G204 -- fixed immutable trusted Git object and role-map path, read only.
	command.Dir = ".."
	data, err := command.Output()
	Expect(err).NotTo(HaveOccurred())
	var mapping installationRoleMapOracle
	Expect(json.Unmarshal(data, &mapping)).To(Succeed())
	Expect(mapping.CanonicalSourceAssets).To(HaveLen(85))
	result := []string{}
	for _, row := range mapping.CanonicalSourceAssets {
		if row.Path == ".factory/assets/scaffold/templates/metrics.html" || row.Path == ".factory/assets/scaffold/packs/review-lane/review-pr.yml" {
			continue
		}
		result = append(result, row.Path)
	}
	Expect(result).To(HaveLen(83))
	return result
}

func obsoleteRetirementPrepare() (image, minimal, before handoffReleaseImage, prepared, control string) {
	GinkgoHelper()
	image, minimal, before, prepared = minimalInventoryPrepare(func(source string) {
		control = filepath.Join(filepath.Dir(source), "obsolete-retirement-control.json")
		path := filepath.Join(source, "internal/installation/transaction.go")
		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		needle := "owner.journalImage, err = saveJournal(ctx, owner.tree, owner.record, owner.journalImage)\n\treturn err"
		patched := strings.Replace(string(data), needle, "owner.journalImage, err = saveJournal(ctx, owner.tree, owner.record, owner.journalImage)\n\tif err == nil { err = owner.independentObsoleteBoundary(ctx) }\n\treturn err", 1)
		Expect(patched).NotTo(Equal(string(data)))
		writeFixture(path, []byte(patched), 0644)
		hook := strings.ReplaceAll(obsoleteBoundarySource, "INDEPENDENT_CONTROL", fmt.Sprintf("%q", control))
		writeFixture(filepath.Join(source, "internal/installation/independent_obsolete_fixture.go"), []byte(hook), 0644)
	})
	return
}

// Historical code establishes a genuine original; only the current consumer
// qualifies and mutates the project in each retirement journey.
// per docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:66
func obsoleteHistoricalProject(image handoffReleaseImage, version string) string {
	GinkgoHelper()
	f, driver := handoffNativeFixture(image)
	assets := f.environment["HANDOFF_ASSETS"]
	writeFixture(filepath.Join(assets, "factory-bootstrap-"+version+"-"+f.target), image.binary, 0600)
	writeFixture(filepath.Join(assets, "factory-installation-"+version+"-"+f.target+".tar.gz"), image.archive, 0600)
	root := filepath.Join(f.cwd, "genuine historical retirement predecessor")
	Expect(os.Mkdir(root, 0700)).To(Succeed())
	imageGit(root, "init", "--quiet")
	initialized := handoffNativeProcess(f.cwd, f.environment, recipeAnswers, driver, "init", root, "--go-candidate", version, "--pack", "none")
	Expect(initialized.status).To(Equal(0), "%+v", initialized)
	return root
}

var _ = Describe("Qualified obsolete initializer cache retirement", Ordered, ContinueOnFailure, func() {
	var image, minimal, before handoffReleaseImage
	var prepared string
	BeforeAll(func() { image, minimal, before, prepared, _ = obsoleteRetirementPrepare() })
	// per docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:73
	// per docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:81
	// per docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:98
	It("previews qualified retirement and restores exact obsolete originals through inert backup and rollback", func() {
		f, driver, root, args := minimalInventoryOldProject(before, image, prepared)
		obsolete := obsoleteCacheOracle()
		writeFixture(filepath.Join(root, "docs/retirement-operator-note.md"), []byte("preserve application and user history\n"), 0600)
		snapshot := installationSnapshot(root)
		descriptor, err := os.ReadFile(filepath.Join(root, ".factory/installation.current"))
		Expect(err).NotTo(HaveOccurred())
		proposal := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...))
		Expect(installationSnapshot(root)).To(Equal(snapshot), "preview must not change active cache, ignore rules or recovery")
		retired := []string{}
		for _, raw := range proposal["actions"].([]any) {
			row := raw.(map[string]any)
			if row["action"] == "retire" && strings.HasPrefix(row["path"].(string), ".factory/assets/scaffold/") {
				retired = append(retired, row["path"].(string))
			}
		}
		AddReportEntry("obsolete-cache.preview", map[string]any{"declared_obsolete": len(obsolete), "retire_count": len(retired), "paths": retired, "before_cache_count": len(minimalInitializerCacheFiles(root)), "before_revision": before.revision}, ReportEntryVisibilityNever)
		Expect(retired).To(ConsistOf(obsolete), "only unchanged cache selected by the independently qualified old image should retire")
		const id = "qualified-obsolete-cache"
		apply := append(append([]string{}, args...), "--migration-id", id, "--confirm", proposal["proposal_digest"].(string), "--quiescent")
		applied := handoffNativeProcess(root, f.environment, "", driver, apply...)
		Expect(applied.status).To(Equal(0), "%+v", applied)
		Expect(minimalInitializerCacheFiles(root)).To(ConsistOf(minimalInitializerCache))
		for _, path := range obsolete {
			_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(path)))
			Expect(os.IsNotExist(err)).To(BeTrue(), path)
			saved := filepath.Join(root, ".factory/backups", id, "assets", filepath.FromSlash(path))
			original, err := os.ReadFile(filepath.Join(prepared, filepath.FromSlash(path)))
			Expect(err).NotTo(HaveOccurred())
			Expect(os.ReadFile(saved)).To(Equal(original), path)
			mode, err := os.Stat(saved)
			Expect(err).NotTo(HaveOccurred())
			Expect(mode.Mode().Perm()).To(Equal(os.FileMode(0600)), "saved cache remains inert: %s", path)
			Expect(imageProcess(root, nil, "git", "check-ignore", "--no-index", "-q", "--", saved).status).To(Equal(0), path)
		}
		Expect(minimalInventoryJSON(filepath.Join(root, ".factory/installation-transactions", id+".json"))["entries"]).To(HaveLen(141))
		reverse := append(append([]string{}, args...), "--rollback", id)
		// Both unsigned records agree on the forged nil original; neither can
		// replace the freshly qualified before/saved-original proof.
		// per docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:158
		journalPath := filepath.Join(root, ".factory/installation-transactions", id+".json")
		manifestPath := filepath.Join(root, ".factory/backups", id, "installation.manifest")
		originalJournal, err := os.ReadFile(journalPath)
		Expect(err).NotTo(HaveOccurred())
		originalManifest, err := os.ReadFile(manifestPath)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			writeFixture(journalPath, originalJournal, 0600)
			writeFixture(manifestPath, originalManifest, 0600)
		})
		for _, path := range []string{journalPath, manifestPath} {
			record := minimalInventoryJSON(path)
			found := false
			for _, raw := range record["entries"].([]any) {
				entry := raw.(map[string]any)
				if entry["path"] == obsolete[0] {
					Expect(entry["action"]).To(Equal("retire"))
					Expect(entry["before"]).NotTo(BeNil())
					entry["before"] = nil
					found = true
				}
			}
			Expect(found).To(BeTrue())
			writeFixture(path, imageMarshal(record), 0600)
		}
		tampered := installationSnapshot(root)
		refused := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, reverse...), "--dry-run", "--json")...)
		AddReportEntry("obsolete-cache.nil-before-reverse", map[string]any{"status": refused.status, "stdout": refused.stdout, "stderr": refused.stderr, "path": obsolete[0]}, ReportEntryVisibilityNever)
		Expect(installationSnapshot(root)).To(Equal(tampered))
		Expect(refused.status).To(Equal(2), "coherent nil-before metadata must not qualify an old required cache: %+v", refused)
		var denied map[string]any
		Expect(json.Unmarshal([]byte(refused.stdout), &denied)).To(Succeed())
		Expect(fmt.Sprint(denied["rollback_blockers"])).To(ContainSubstring(obsolete[0]))
		writeFixture(journalPath, originalJournal, 0600)
		writeFixture(manifestPath, originalManifest, 0600)
		rollback := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, reverse...), "--dry-run", "--json")...))
		restored := handoffNativeProcess(root, f.environment, "", driver, append(reverse, "--confirm", rollback["proposal_digest"].(string), "--quiescent")...)
		Expect(restored.status).To(Equal(0), "%+v", restored)
		Expect(os.ReadFile(filepath.Join(root, ".factory/installation.current"))).To(Equal(descriptor))
		after := installationSnapshot(root)
		for path, value := range snapshot {
			if path != ".gitignore" && !strings.HasPrefix(path, ".git/") {
				Expect(after[path]).To(Equal(value), path)
			}
		}
		Expect(minimalInitializerCacheFiles(root)).To(HaveLen(85))
		AddReportEntry("obsolete-cache.roundtrip", map[string]any{"retired": len(obsolete), "restored_cache": 85, "migration": id, "rollback_status": restored.status}, ReportEntryVisibilityNever)
	})

	// per docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:41
	It("retires the earliest supported historical recipe and restores its exact original cache", func() {
		rc3 := historicalInstallationImage(image, recipeRC3Revision, recipeRC3Version)
		original := obsoleteHistoricalProject(rc3, recipeRC3Version)
		Expect(minimalInitializerCacheFiles(original)).To(HaveLen(85))
		f, driver := handoffNativeFixture(image)
		cloned := installationCloneLegacyFixture(image.binary, installationFixtureData{root: original})
		root := cloned.root
		args := recipeOperands(rc3, image, recipeRC3Version)
		snapshot := installationSnapshot(root)
		descriptor, err := os.ReadFile(filepath.Join(root, ".factory/installation.current"))
		Expect(err).NotTo(HaveOccurred())
		proposal := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...))
		Expect(installationSnapshot(root)).To(Equal(snapshot))
		retire := []string{}
		for _, raw := range proposal["actions"].([]any) {
			row := raw.(map[string]any)
			if row["action"] == "retire" && strings.HasPrefix(row["path"].(string), ".factory/assets/scaffold/") {
				retire = append(retire, row["path"].(string))
			}
		}
		Expect(retire).To(ConsistOf(obsoleteCacheOracle()))
		const id = "earliest-supported-cache-retirement"
		applied := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--migration-id", id, "--confirm", proposal["proposal_digest"].(string), "--quiescent")...)
		Expect(applied.status).To(Equal(0), "%+v", applied)
		Expect(minimalInitializerCacheFiles(root)).To(ConsistOf(minimalInitializerCache))
		for _, path := range retire {
			saved := filepath.Join(root, ".factory/backups", id, "assets", filepath.FromSlash(path))
			data, err := os.ReadFile(filepath.Join(original, filepath.FromSlash(path)))
			Expect(err).NotTo(HaveOccurred())
			Expect(os.ReadFile(saved)).To(Equal(data), path)
			info, err := os.Stat(saved)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
		}
		reverse := append(append([]string{}, args...), "--rollback", id)
		preview := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, reverse...), "--dry-run", "--json")...))
		restored := handoffNativeProcess(root, f.environment, "", driver, append(reverse, "--confirm", preview["proposal_digest"].(string), "--quiescent")...)
		Expect(restored.status).To(Equal(0), "%+v", restored)
		Expect(os.ReadFile(filepath.Join(root, ".factory/installation.current"))).To(Equal(descriptor))
		final := installationSnapshot(root)
		for path, value := range snapshot {
			if path != ".gitignore" && !strings.HasPrefix(path, ".git/") {
				Expect(final[path]).To(Equal(value), path)
			}
		}
		Expect(minimalInitializerCacheFiles(root)).To(HaveLen(85))
		AddReportEntry("obsolete-cache.earliest-recipe-roundtrip", map[string]any{"revision": rc3.revision, "retired": len(retire), "restored": 85}, ReportEntryVisibilityNever)
	})

	// A minimal predecessor never selected these paths; equal bytes are not
	// retrospective ownership, and its original 58-row record remains valid.
	// per docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:66
	It("preserves unowned equal-looking leftovers and replays the original minimal transaction generation", func() {
		original := obsoleteHistoricalProject(minimal, handoffVersion)
		Expect(minimalInitializerCacheFiles(original)).To(ConsistOf(minimalInitializerCache))
		f, driver := handoffNativeFixture(image)
		cloned := installationCloneLegacyFixture(image.binary, installationFixtureData{root: original})
		root := cloned.root
		originalCache, err := os.OpenRoot(prepared)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(originalCache.Close()).To(Succeed()) })
		project, err := os.OpenRoot(root)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(project.Close()).To(Succeed()) })
		for _, path := range obsoleteCacheOracle() {
			data, err := originalCache.ReadFile(filepath.FromSlash(path))
			Expect(err).NotTo(HaveOccurred())
			Expect(project.MkdirAll(filepath.Dir(filepath.FromSlash(path)), 0700)).To(Succeed())
			Expect(project.WriteFile(filepath.FromSlash(path), data, 0600)).To(Succeed())
		}
		unknown := filepath.Join(root, ".factory/assets/scaffold/operator-reference")
		writeFixture(unknown, []byte("unknown local history is never cleanup authority\n"), 0600)
		oldArgs := recipeOperands(minimal, minimal, handoffVersion)
		_, oldDriver := handoffNativeFixture(minimal)
		const oldID = "original-minimal-generation"
		minimalInventoryApply(root, oldDriver, f, oldArgs, oldID)
		journal := filepath.Join(root, ".factory/installation-transactions", oldID+".json")
		manifest := filepath.Join(root, ".factory/backups", oldID, "installation.manifest")
		receipt := filepath.Join(root, ".factory/backups", oldID, "retention.json")
		Expect(minimalInventoryJSON(journal)["entries"]).To(HaveLen(58))
		Expect(minimalInventoryJSON(manifest)["entries"]).To(HaveLen(58))
		journalBytes, err := os.ReadFile(journal)
		Expect(err).NotTo(HaveOccurred())
		manifestBytes, err := os.ReadFile(manifest)
		Expect(err).NotTo(HaveOccurred())
		receiptBytes, err := os.ReadFile(receipt)
		Expect(err).NotTo(HaveOccurred())
		// Qualify the older transaction before later publication legitimately
		// changes its captured leaf identities; never refresh historical evidence.
		// per docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:91
		oldReverse := append(append([]string{}, oldArgs...), "--rollback", oldID)
		record := minimalInventoryJSON(journal)
		entries := record["entries"].([]any)
		record["entries"] = entries[:len(entries)-1]
		writeFixture(journal, imageMarshal(record), 0600)
		tampered := installationSnapshot(root)
		refused := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, oldReverse...), "--dry-run", "--json")...)
		Expect(refused.status).To(Equal(2), "%+v", refused)
		Expect(installationSnapshot(root)).To(Equal(tampered))
		writeFixture(journal, journalBytes, 0600)
		oldProposal := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, oldReverse...), "--dry-run", "--json")...))
		completed := handoffNativeProcess(root, f.environment, "", driver, append(oldReverse, "--confirm", oldProposal["proposal_digest"].(string), "--quiescent")...)
		Expect(completed.status).To(Equal(0), "%+v", completed)
		Expect(minimalInventoryJSON(journal)["entries"]).To(HaveLen(58))
		Expect(os.ReadFile(manifest)).To(Equal(manifestBytes))
		Expect(os.ReadFile(receipt)).To(Equal(receiptBytes))
		journalBytes, err = os.ReadFile(journal)
		Expect(err).NotTo(HaveOccurred())

		snapshot := installationSnapshot(root)
		args := recipeOperands(minimal, image, handoffVersion)
		proposal := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...))
		Expect(installationSnapshot(root)).To(Equal(snapshot))
		for _, raw := range proposal["actions"].([]any) {
			row := raw.(map[string]any)
			if strings.HasPrefix(row["path"].(string), ".factory/assets/scaffold/") && row["action"] == "retire" {
				Fail("a 58-row predecessor cannot own an omitted cache path: " + row["path"].(string))
			}
		}
		const id = "unowned-minimal-leftovers"
		applied := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--migration-id", id, "--confirm", proposal["proposal_digest"].(string), "--quiescent")...)
		Expect(applied.status).To(Equal(0), "%+v", applied)
		current := installationSnapshot(root)
		for path, value := range snapshot {
			if strings.HasPrefix(path, ".factory/assets/scaffold/") {
				Expect(current[path]).To(Equal(value), path)
			}
		}
		for _, path := range obsoleteCacheOracle() {
			_, err := os.Lstat(filepath.Join(root, ".factory/backups", id, "assets", filepath.FromSlash(path)))
			Expect(os.IsNotExist(err)).To(BeTrue(), "unowned path cannot be backed up: %s", path)
		}
		reverse := append(append([]string{}, args...), "--rollback", id)
		rollback := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, reverse...), "--dry-run", "--json")...))
		restored := handoffNativeProcess(root, f.environment, "", driver, append(reverse, "--confirm", rollback["proposal_digest"].(string), "--quiescent")...)
		Expect(restored.status).To(Equal(0), "%+v", restored)
		Expect(os.ReadFile(journal)).To(Equal(journalBytes))
		Expect(os.ReadFile(manifest)).To(Equal(manifestBytes))
		Expect(os.ReadFile(receipt)).To(Equal(receiptBytes))

		Expect(os.ReadFile(unknown)).To(Equal([]byte("unknown local history is never cleanup authority\n")))
		AddReportEntry("obsolete-cache.unowned-minimal", map[string]any{"old_rows": 58, "leftovers": 83, "retirement_rows": 141, "rollback_status": completed.status}, ReportEntryVisibilityNever)
	})

})

var _ = Describe("Qualified obsolete initializer cache retirement", Ordered, ContinueOnFailure, func() {
	var image, before handoffReleaseImage
	var prepared, control string
	BeforeAll(func() { image, _, before, prepared, control = obsoleteRetirementPrepare() })
	// per docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:148
	It("preserves unsafe retained cache through completed replay terminal cleanup and preparation abandonment", func() {
		type observation struct {
			Mode              string
			Preview, Complete int
			Stdout, Stderr    string
		}
		observed := []observation{}
		for _, mode := range []string{"replay", "committed", "preparation"} {
			f, driver, root, args := minimalInventoryOldProject(before, image, prepared)
			outside := filepath.Join(f.cwd, "outside cache referents")
			symbolic := filepath.Join(outside, "symbolic-data")
			hard := filepath.Join(outside, "hard-data")
			writeFixture(symbolic, []byte("outside symbolic referent must remain\n"), 0600)
			writeFixture(hard, []byte("outside hardlink referent must remain\n"), 0600)
			symbolicPath := filepath.Join(root, ".factory/assets/scaffold/.codex/agents/implementer.toml")
			hardPath := filepath.Join(root, ".factory/assets/scaffold/.codex/agents/refactorer.toml")
			Expect(os.Remove(symbolicPath)).To(Succeed())
			Expect(os.Symlink(symbolic, symbolicPath)).To(Succeed())
			Expect(os.Remove(hardPath)).To(Succeed())
			Expect(os.Link(hard, hardPath)).To(Succeed())
			editedPath := filepath.Join(root, ".factory/assets/scaffold/.codex/agents/reviewer.toml")
			writeFixture(editedPath, []byte("operator-owned obsolete reference\n"), 0600)
			modePath := filepath.Join(root, ".factory/assets/scaffold/.codex/agents/spec-writer.toml")
			Expect(os.Chmod(modePath, 0700)).To(Succeed())
			docsPath := filepath.Join(root, ".factory/assets/scaffold/docs")
			outDocs := filepath.Join(outside, "original docs")
			Expect(os.Rename(docsPath, outDocs)).To(Succeed())
			Expect(os.Symlink(outDocs, docsPath)).To(Succeed())
			unknownPath := ".factory/assets/scaffold/operator-reference"
			writeFixture(filepath.Join(root, unknownPath), []byte("unknown reference must survive\n"), 0600)
			preserved := installationSnapshot(root)
			retainedPaths := []string{
				".factory/assets/scaffold/.codex/agents/implementer.toml",
				".factory/assets/scaffold/.codex/agents/refactorer.toml",
				".factory/assets/scaffold/.codex/agents/reviewer.toml",
				".factory/assets/scaffold/.codex/agents/spec-writer.toml",
				".factory/assets/scaffold/docs", unknownPath,
			}
			outsideBefore := installationSnapshot(outside)
			originalHard, err := os.Stat(hard)
			Expect(err).NotTo(HaveOccurred())
			if mode == "committed" {
				writeFixture(control, imageMarshal(map[string]string{"Root": root, "Mode": "committed", "Ready": filepath.Join(f.records, "actual-committed-boundary")}), 0600)
			} else {
				if _, err := os.Lstat(control); err == nil {
					Expect(os.Remove(control)).To(Succeed())
				} else {
					Expect(os.IsNotExist(err)).To(BeTrue())
				}
			}
			ignore := filepath.Join(root, ".gitignore")
			originalIgnore, err := os.ReadFile(ignore)
			Expect(err).NotTo(HaveOccurred())
			if mode == "preparation" {
				exposed := "\n!/.factory/\n/.factory/bin/\n/.factory/assets/\n/.factory/installation.current\n/.factory/installation-transactions/\n/.factory/backups/\n!/.factory/bin/\n"
				writeFixture(ignore, append(append([]byte{}, originalIgnore...), []byte(exposed)...), 0600)
			}
			proposal := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...))
			id := "retained-unsafe-" + mode
			apply := append(append([]string{}, args...), "--migration-id", id, "--confirm", proposal["proposal_digest"].(string), "--quiescent")
			applied := handoffNativeProcess(root, f.environment, "", driver, apply...)
			journalPath := filepath.Join(root, ".factory/installation-transactions", id+".json")
			current := installationSnapshot(root)
			for _, path := range retainedPaths {
				Expect(current[path]).To(Equal(preserved[path]), "preserve customized, unsafe and unknown cache: %s", path)
			}
			record := minimalInventoryJSON(journalPath)
			switch mode {
			case "replay":
				Expect(applied.status).To(Equal(0), "%+v", applied)
				Expect(record["phase"]).To(Equal("committed"))
			case "committed":
				Expect(applied.status).To(Equal(1), "the evaluator faults only after the actual committed save: %+v", applied)
				Expect(record["phase"]).To(Equal("committed"))
				Expect(filepath.Join(root, ".factory/runtime-publication.pending")).To(BeAnExistingFile())
			case "preparation":
				Expect(applied.status).To(Equal(2), "actual private-ignore refusal: %+v", applied)
				Expect(applied.stderr).To(ContainSubstring("not effectively ignored"))
				Expect(record["phase"]).To(Equal("prepared"))
				writeFixture(ignore, originalIgnore, 0600)
			}
			snapshot := installationSnapshot(root)
			if mode == "replay" {
				replay := handoffNativeProcess(root, f.environment, "", driver, apply...)
				observed = append(observed, observation{mode, replay.status, replay.status, replay.stdout, replay.stderr})
				Expect(installationSnapshot(root)).To(Equal(snapshot), "completed replay cannot mutate retained unsafe data")
				reverse := append(append([]string{}, args...), "--rollback", id, "--dry-run", "--json")
				blocked := handoffNativeProcess(root, f.environment, "", driver, reverse...)
				Expect(blocked.status).To(Equal(2), "unsafe required historical cache must still prevent reverse activation: %+v", blocked)
				Expect(installationSnapshot(root)).To(Equal(snapshot))
			} else {
				direction := "forward"
				if mode == "preparation" {
					direction = "reverse"
				}
				recovery := append(append([]string{}, args...), "--recover", id, "--direction", direction)
				preview := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, recovery...), "--dry-run", "--json")...)
				Expect(installationSnapshot(root)).To(Equal(snapshot), "recovery preview is read-only")
				completed := cliResult{status: -999}
				if preview.status == 0 {
					current := handoffProposal(preview)
					completed = handoffNativeProcess(root, f.environment, "", driver, append(recovery, "--confirm", current["proposal_digest"].(string), "--quiescent")...)
				}
				observed = append(observed, observation{mode, preview.status, completed.status, preview.stdout + completed.stdout, preview.stderr + completed.stderr})
				if completed.status == 0 {
					after := installationSnapshot(root)
					for path, value := range snapshot {
						if path != ".factory/runtime-publication.pending" && path != ".factory/installation-transactions/"+id+".json" {
							Expect(after[path]).To(Equal(value), path)
						}
					}
					if mode == "committed" {
						_, err := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
						Expect(os.IsNotExist(err)).To(BeTrue())
					}
				}
			}
			Expect(installationSnapshot(outside)).To(Equal(outsideBefore))
			target, err := os.Readlink(symbolicPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(target).To(Equal(symbolic))
			retainedHard, err := os.Stat(hardPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.SameFile(originalHard, retainedHard)).To(BeTrue())
		}
		AddReportEntry("obsolete-cache.retained-unsafe-recovery", observed, ReportEntryVisibilityNever)
		for _, out := range observed {
			Expect(out.Preview).To(Equal(0), "%s preview/replay preserves irrelevant unsafe cache: %+v", out.Mode, out)
			Expect(out.Complete).To(Equal(0), "%s completion preserves irrelevant unsafe cache: %+v", out.Mode, out)
		}
	})

	// Retirement obeys the existing consent and inert-backup prerequisites;
	// neither an old digest nor failed private preparation permits an unlink.
	// per docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:81
	It("refuses stale retirement consent and preserves every original when inert backup preparation cannot proceed", func() {
		f, driver, root, args := minimalInventoryOldProject(before, image, prepared)
		proposal := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...))
		path := filepath.Join(root, ".factory/assets/scaffold/.codex/agents/implementer.toml")
		original, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		writeFixture(path, []byte("new operator data invalidates retirement consent\n"), 0600)
		changed := installationSnapshot(root)
		stale := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--migration-id", "stale-retirement", "--confirm", proposal["proposal_digest"].(string), "--quiescent")...)
		Expect(stale.status).To(Equal(2), "%+v", stale)
		installationAssertExistingUnchanged(root, changed)
		// First exclusive admission may create exactly the permanent guard
		// objects, which are not retirement or recovery publication.
		// per docs/adr/0093-runtime-transition-guard.md:37
		admitted := installationSnapshot(root)
		added := []string{}
		for path := range admitted {
			if _, existed := changed[path]; !existed {
				added = append(added, path)
			}
		}
		Expect(added).To(ConsistOf(".factory/runtime-transition.lock", ".factory/runtime-activity"))
		lock, err := os.Lstat(filepath.Join(root, ".factory/runtime-transition.lock"))
		Expect(err).NotTo(HaveOccurred())
		Expect(lock.Mode().IsRegular()).To(BeTrue())
		Expect(lock.Mode().Perm()).To(Equal(os.FileMode(0600)))
		Expect(lock.Size()).To(BeZero())
		activity, err := os.Lstat(filepath.Join(root, ".factory/runtime-activity"))
		Expect(err).NotTo(HaveOccurred())
		Expect(activity.IsDir()).To(BeTrue())
		Expect(activity.Mode().Perm()).To(Equal(os.FileMode(0700)))
		Expect(os.ReadDir(filepath.Join(root, ".factory/runtime-activity"))).To(BeEmpty())
		for _, name := range []string{".factory/installation-transactions/stale-retirement.json", ".factory/backups/stale-retirement", ".factory/runtime-publication.pending"} {
			_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(name)))
			Expect(os.IsNotExist(err)).To(BeTrue(), "stale consent cannot publish recovery evidence: %s", name)
		}
		again := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--migration-id", "stale-retirement", "--confirm", proposal["proposal_digest"].(string), "--quiescent")...)
		Expect(again.status).To(Equal(2), "%+v", again)
		Expect(installationSnapshot(root)).To(Equal(admitted), "repeat stale refusal preserves the complete tree and admission state")
		for name, info := range map[string]os.FileInfo{".factory/runtime-transition.lock": lock, ".factory/runtime-activity": activity} {
			current, err := os.Lstat(filepath.Join(root, name))
			Expect(err).NotTo(HaveOccurred())
			Expect(os.SameFile(info, current)).To(BeTrue(), "permanent admission identity must not be replaced: %s", name)
		}
		writeFixture(path, original, 0600)
		ignore := filepath.Join(root, ".gitignore")
		originalIgnore, err := os.ReadFile(ignore)
		Expect(err).NotTo(HaveOccurred())
		exposed := "\n!/.factory/\n/.factory/bin/\n/.factory/assets/\n/.factory/installation.current\n/.factory/installation-transactions/\n!/.factory/backups/\n"
		writeFixture(ignore, append(append([]byte{}, originalIgnore...), []byte(exposed)...), 0600)
		beforeFailure := installationSnapshot(root)
		fresh := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...))
		const id = "unavailable-inert-backup"
		refused := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--migration-id", id, "--confirm", fresh["proposal_digest"].(string), "--quiescent")...)
		Expect(refused.status).To(Equal(2), "actual ignored-backup prerequisite must refuse: %+v", refused)
		Expect(refused.stderr).To(ContainSubstring("not effectively ignored"))
		current := installationSnapshot(root)
		for path, value := range beforeFailure {
			Expect(current[path]).To(Equal(value), "backup preparation cannot overwrite any existing asset: %s", path)
		}
		Expect(minimalInitializerCacheFiles(root)).To(HaveLen(85))
		journal := minimalInventoryJSON(filepath.Join(root, ".factory/installation-transactions", id+".json"))
		Expect(journal["phase"]).To(Equal("prepared"))
		for _, raw := range journal["entries"].([]any) {
			Expect(raw.(map[string]any)["phase"]).NotTo(Equal("applied"))
		}
		writeFixture(ignore, originalIgnore, 0600)
		recovery := append(append([]string{}, args...), "--recover", id, "--direction", "reverse")
		planned := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, recovery...), "--dry-run", "--json")...))
		completed := handoffNativeProcess(root, f.environment, "", driver, append(recovery, "--confirm", planned["proposal_digest"].(string), "--quiescent")...)
		Expect(completed.status).To(Equal(0), "%+v", completed)
		Expect(minimalInitializerCacheFiles(root)).To(HaveLen(85))
		AddReportEntry("obsolete-cache.pre-deletion-prerequisites", map[string]any{"stale_status": stale.status, "backup_status": refused.status, "journal_phase": journal["phase"], "abandon_status": completed.status, "original_cache": 85}, ReportEntryVisibilityNever)
	})

	// Actual save completion, a live owned PID and an applied retirement row
	// establish interruption; elapsed sleep never substitutes for that witness.
	// per docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:98
	DescribeTable("recovers durably interrupted obsolete retirement with exact originals and no compiler", func(direction string) {
		f, driver, root, args := minimalInventoryOldProject(before, image, prepared)
		writeFixture(filepath.Join(root, "docs/interrupted-retirement-note"), []byte("preserve application history\n"), 0600)
		original := installationSnapshot(root)
		descriptor, err := os.ReadFile(filepath.Join(root, ".factory/installation.current"))
		Expect(err).NotTo(HaveOccurred())
		localInitPoisonCompiler(f)
		proposal := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...))
		ready := filepath.Join(f.records, "actual-retirement-ready.json")
		writeFixture(control, imageMarshal(map[string]string{"Root": root, "Mode": "applying", "Ready": ready}), 0600)
		DeferCleanup(func() { Expect(os.Remove(control)).To(Succeed()) })
		id := "interrupted-retirement-" + direction
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		command := handoffCommand(ctx, root, f.environment, "", driver, append(append([]string{}, args...), "--migration-id", id, "--confirm", proposal["proposal_digest"].(string), "--quiescent")...)
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		Expect(command.Start()).To(Succeed())
		finished := make(chan struct{})
		var returned error
		go func() { returned = command.Wait(); close(finished) }()
		type workerReceipt struct {
			PID, Group, Retired    int
			Root, Migration, Phase string
		}
		var receipt workerReceipt
		groupAbsent := func() (bool, error) {
			err := syscall.Kill(-command.Process.Pid, 0)
			if errors.Is(err, syscall.ESRCH) {
				return true, nil
			}
			return false, err
		}
		defer func() {
			waited := false
			select {
			case <-finished:
				waited = true
			default:
				_ = command.Process.Signal(syscall.SIGTERM)
				select {
				case <-finished:
					waited = true
				case <-time.After(10 * time.Second):
					group, err := syscall.Getpgid(command.Process.Pid)
					if err == nil && group == command.Process.Pid {
						_ = syscall.Kill(-group, syscall.SIGKILL)
					}
					select {
					case <-finished:
						waited = true
					case <-time.After(10 * time.Second):
					}
				}
			}
			absent, probe := groupAbsent()
			if !waited || !absent || probe != nil {
				reason := fmt.Sprintf("obsolete retirement process custody uncertain: pid=%d waited=%t absent=%t probe=%v", command.Process.Pid, waited, absent, probe)
				for _, path := range []string{root, f.root, image.work} {
					holdFixtureCleanup(path, reason)
				}
			}
			observed := map[string]any{"receipt": receipt, "caller_pid": command.Process.Pid, "direction": direction, "waited": waited, "group_absent": absent, "group_probe": fmt.Sprint(probe)}
			if waited {
				observed["exit"] = command.ProcessState.ExitCode()
				observed["wait_status"] = command.ProcessState.String()
				observed["stderr"] = stderr.String()
			}
			AddReportEntry("obsolete-cache.actual-interruption", observed, ReportEntryVisibilityNever)
			Expect(waited).To(BeTrue(), "reap the owned caller before fixture disposal")
			Expect(probe).NotTo(HaveOccurred())
			Expect(absent).To(BeTrue(), "whole recorded group must settle before fixture disposal")
		}()
		Eventually(func() bool {
			select {
			case <-finished:
				StopTrying(fmt.Sprintf("caller exited before actual retirement receipt: %v stderr=%q", returned, stderr.String())).Now()
			default:
			}
			data, err := os.ReadFile(ready)
			if os.IsNotExist(err) {
				return false
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(json.Unmarshal(data, &receipt)).To(Succeed())
			return true
		}, 60*time.Second).WithContext(ctx).Should(BeTrue())
		Expect(receipt.PID).To(Equal(command.Process.Pid))
		Expect(receipt.Group).To(Equal(receipt.PID))
		Expect(receipt.Root).To(Equal(root))
		Expect(receipt.Migration).To(Equal(id))
		Expect(receipt.Phase).To(Equal("applying"))
		Expect(receipt.Retired).To(BeNumerically(">", 0))
		Expect(syscall.Kill(receipt.PID, 0)).To(Succeed(), "readiness belongs to a live actual publisher")
		journal := minimalInventoryJSON(filepath.Join(root, ".factory/installation-transactions", id+".json"))
		Expect(journal["phase"]).To(Equal("applying"))
		retired := 0
		for _, raw := range journal["entries"].([]any) {
			row := raw.(map[string]any)
			if row["action"] == "retire" && row["phase"] == "applied" && strings.HasPrefix(row["path"].(string), ".factory/assets/scaffold/") {
				retired++
				Expect(row["before"]).NotTo(BeNil())
				saved := filepath.Join(root, ".factory/backups", id, "assets", filepath.FromSlash(row["path"].(string)))
				info, err := os.Stat(saved)
				Expect(err).NotTo(HaveOccurred())
				Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
			}
		}
		Expect(retired).To(Equal(receipt.Retired), "witness must match the actual persisted applied retirement rows")
		Expect(filepath.Join(root, ".factory/runtime-publication.pending")).To(BeAnExistingFile())
		Expect(command.Process.Signal(syscall.SIGTERM)).To(Succeed())
		select {
		case <-finished:
		case <-time.After(10 * time.Second):
			Fail("actual retirement SIGTERM did not return within the existing cleanup allowance")
		}
		Expect(returned).To(HaveOccurred())
		// The raw whole-installation owner retains ordinary OS signal behavior.
		// per docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:112
		var terminated *exec.ExitError
		Expect(errors.As(returned, &terminated)).To(BeTrue())
		waitStatus, ok := terminated.Sys().(syscall.WaitStatus)
		Expect(ok).To(BeTrue())
		Expect(waitStatus.Signaled()).To(BeTrue())
		Expect(waitStatus.Signal()).To(Equal(syscall.SIGTERM))
		absent, probe := groupAbsent()
		Expect(probe).NotTo(HaveOccurred())
		Expect(absent).To(BeTrue())
		stopped := installationSnapshot(root)
		recovery := append(append([]string{}, args...), "--recover", id, "--direction", direction)
		planned := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, recovery...), "--dry-run", "--json")...))
		Expect(installationSnapshot(root)).To(Equal(stopped))
		completed := handoffNativeProcess(root, f.environment, "", driver, append(recovery, "--confirm", planned["proposal_digest"].(string), "--quiescent")...)
		Expect(completed.status).To(Equal(0), "%+v", completed)
		if direction == "forward" {
			Expect(minimalInitializerCacheFiles(root)).To(ConsistOf(minimalInitializerCache))
		} else {
			Expect(minimalInitializerCacheFiles(root)).To(HaveLen(85))
			Expect(os.ReadFile(filepath.Join(root, ".factory/installation.current"))).To(Equal(descriptor))
			current := installationSnapshot(root)
			for path, value := range original {
				if path != ".gitignore" && !strings.HasPrefix(path, ".git/") {
					Expect(current[path]).To(Equal(value), path)
				}
			}
		}
		_, err = os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(os.ReadFile(filepath.Join(root, "docs/interrupted-retirement-note"))).To(Equal([]byte("preserve application history\n")))
		localInitNoAcquisition(f)
	}, Entry("forward", "forward"), Entry("reverse", "reverse"))

})

// Evaluator-only source is compiled into a private, committed image. No shipping
// flag or mutable production callback is introduced. Actual saves establish the
// pending/preparation state; records are never fabricated for these controls.
const obsoleteBoundarySource = `package installation
import("context";"encoding/json";"errors";"os";"strings";"syscall";"time")
func(owner *transaction)independentObsoleteBoundary(ctx context.Context)error{
 data,err:=os.ReadFile(INDEPENDENT_CONTROL);if os.IsNotExist(err){return nil};if err!=nil{return err}
 var c struct{Root,Mode,Ready string};if err=json.Unmarshal(data,&c);err!=nil{return err}
 if owner.root!=c.Root||owner.request.Recover!=""||owner.request.Rollback!=""{return nil}
 retired:=0;for _,row:=range owner.record.Entries{if row.Action=="retire"&&row.Phase=="applied"&&strings.HasPrefix(row.Path,".factory/assets/scaffold/"){retired++}}
 if c.Mode=="committed"&&owner.record.Phase!="committed"{return nil}
 if c.Mode=="applying"&&(owner.record.Phase!="applying"||retired==0){return nil}
 if c.Mode!="committed"&&c.Mode!="applying"{return nil}
 group,err:=syscall.Getpgid(0);if err!=nil{return err}
 receipt:=map[string]any{"pid":os.Getpid(),"group":group,"root":owner.root,"migration":owner.record.MigrationID,"phase":owner.record.Phase,"retired":retired,"at":time.Now().UTC().Format(time.RFC3339Nano)}
 data,err=json.Marshal(receipt);if err!=nil{return err};if err=os.WriteFile(c.Ready+".tmp",data,0600);err!=nil{return err};if err=os.Rename(c.Ready+".tmp",c.Ready);err!=nil{return err}
 if c.Mode=="committed"{return errors.New("independent failure after actual committed save")}
 <-ctx.Done();return ctx.Err()
}
`
