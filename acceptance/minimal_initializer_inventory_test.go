package acceptance_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var minimalInitializerCache = []string{
	"packs/review-lane/review-pr.yml",
	"templates/metrics.html",
}

func minimalInitializerCacheFiles(root string) []string {
	GinkgoHelper()
	cache := filepath.Join(root, ".factory/assets/scaffold")
	var files []string
	err := filepath.Walk(cache, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		Expect(info.Mode().IsRegular()).To(BeTrue(), path)
		relative, err := filepath.Rel(cache, path)
		Expect(err).NotTo(HaveOccurred())
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	Expect(err).NotTo(HaveOccurred())
	return files
}

const minimalInventoryPredecessor = "9be9d0333d018c51dfccea18da966e6138e380ab"
const minimalInventoryGeneration = "777af3871acf1fb09a71c526834a68d46118b1f2"

// Read the actual immutable predecessor instead of using the new generator as
// an oracle. Only the evaluator builds this trusted setup executable.
// per docs/adr/0119-minimal-qualified-initializer-inventory.md:35
func minimalHistoricalImage(after handoffReleaseImage) handoffReleaseImage {
	return historicalInstallationImage(after, minimalInventoryPredecessor, handoffVersion)
}

// Only trusted immutable source is rebuilt for historical setup. Current
// consumers qualify the resulting image; serialized generation counts do not.
// per docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:25
func historicalInstallationImage(after handoffReleaseImage, revision, version string) handoffReleaseImage {
	GinkgoHelper()
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	source := filepath.Join(after.work, "immutable inventory predecessor-"+revision[:7])
	imageGit(repository, "clone", "--quiet", "--shared", "--no-checkout", repository, source)
	imageGit(source, "checkout", "--quiet", "--detach", revision)
	binaryPath := filepath.Join(after.work, "immutable-inventory-runtime-"+revision[:7])
	args := []string{"build", "-o", binaryPath, "./cmd/factory"}
	if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
		args = append([]string{"build", "-race"}, args[1:]...)
	}
	built := imageProcess(source, nil, "go", args...)
	Expect(built.status).To(Equal(0), "%+v", built)
	binary, err := os.ReadFile(binaryPath)
	Expect(err).NotTo(HaveOccurred())
	assets, payload := installationGitCatalog(repository, revision)
	baselines := []imageBaselineOracle{}
	for _, profile := range []string{"v0.1.6", "bash-baseline"} {
		ref := after.references[profile]
		baselines = append(baselines, imageBaselineOracle{profile, ref.revision, ref.assets})
	}
	manifest := imageManifestOracle{1, "installation_source_image", "committed_source_and_baseline_references", version, runtime.GOOS + "/" + runtime.GOARCH, revision, false, assets, baselines}
	archive := stageArchive(imageFixtureEntries(manifest, payload, binary))
	archivePath := filepath.Join(after.work, "immutable-inventory-image-"+revision[:7]+".tar.gz")
	writeFixture(archivePath, archive, 0600)
	return handoffReleaseImage{binary, archive, archivePath, revision, after.work, after.references}
}

func minimalInventoryOldProject(before, after handoffReleaseImage, prepared string, consumers ...handoffReleaseImage) (handoffBootstrapFixture, string, string, []string) {
	GinkgoHelper()
	consumer := after
	if len(consumers) == 1 {
		consumer = consumers[0]
	}
	f, driver := handoffNativeFixture(consumer)
	cloned := installationCloneLegacyFixture(consumer.binary, installationFixtureData{root: prepared})
	return f, driver, cloned.root, recipeOperands(before, after, handoffVersion)
}

func minimalInventoryJSON(path string) map[string]any {
	GinkgoHelper()
	data, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred(), path)
	var record map[string]any
	Expect(json.Unmarshal(data, &record)).To(Succeed(), path)
	return record
}

func minimalInventoryApply(root, driver string, f handoffBootstrapFixture, args []string, id string) map[string]any {
	GinkgoHelper()
	proposal := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...))
	Expect(proposal["blockers"]).To(BeEmpty())
	applied := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--migration-id", id, "--confirm", proposal["proposal_digest"].(string), "--quiescent")...)
	Expect(applied.status).To(Equal(0), "%+v", applied)
	return proposal
}

func minimalInventoryPrepare(patches ...func(string)) (consumer, image, before handoffReleaseImage, prepared string) {
	GinkgoHelper()

	consumer = handoffBuildImage(patches...)
	before = minimalHistoricalImage(consumer)
	image = historicalInstallationImage(consumer, minimalInventoryGeneration, handoffVersion)
	f, driver := handoffNativeFixture(before)
	prepared = filepath.Join(f.cwd, "genuine full inventory adopter")
	Expect(os.Mkdir(prepared, 0700)).To(Succeed())
	imageGit(prepared, "init", "--quiet")
	initialized := handoffNativeProcess(f.cwd, f.environment, recipeAnswers, driver, "init", prepared, "--go-candidate", handoffVersion, "--pack", "none")
	Expect(initialized.status).To(Equal(0), "%+v", initialized)
	Expect(minimalInitializerCacheFiles(prepared)).To(HaveLen(85))
	// Historical preservation criteria target the genuine original 58-row
	// source, while the independent current consumer owns their transaction.
	// per docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:135
	return consumer, image, before, prepared
}

var _ = Describe("Supported predecessor recipe compatibility minimal qualified initializer inventory", Ordered, ContinueOnFailure, func() {
	var consumer, image, before handoffReleaseImage
	var prepared string
	BeforeAll(func() { consumer, image, before, prepared = minimalInventoryPrepare() })

	// per docs/adr/0119-minimal-qualified-initializer-inventory.md:118
	It("publishes only consumed private cache data through the compiled public initializer", func() {
		f, driver := handoffNativeFixture(consumer)
		root := filepath.Join(f.cwd, "minimal qualified adopter")
		Expect(os.Mkdir(root, 0700)).To(Succeed())
		imageGit(root, "init", "--quiet")
		initialized := handoffNativeProcess(f.cwd, f.environment, recipeAnswers, driver, "init", root, "--go-candidate", handoffVersion, "--pack", "none")
		Expect(initialized.status).To(Equal(0), "%+v", initialized)
		paths := minimalInitializerCacheFiles(root)
		AddReportEntry("minimal-inventory.fresh-cache", paths, ReportEntryVisibilityNever)
		Expect(paths).To(ConsistOf(minimalInitializerCache), "new qualified initialization must persist only data consumed by installed commands")
		descriptor := minimalInventoryJSON(filepath.Join(root, ".factory/installation.current"))
		Expect(descriptor["assets"]).To(HaveLen(53), "55 active rows exclude the version marker and self-describing descriptor")
		cacheRows := []string{}
		for _, raw := range descriptor["assets"].([]any) {
			path := raw.(map[string]any)["path"].(string)
			if strings.HasPrefix(path, ".factory/assets/scaffold/") {
				cacheRows = append(cacheRows, strings.TrimPrefix(path, ".factory/assets/scaffold/"))
			}
		}
		Expect(cacheRows).To(ConsistOf(minimalInitializerCache))
		for _, path := range []string{"AGENTS.md", "factory.yaml", "scripts/hooks/test-edit-denial.sh", ".codex/config.toml", ".opencode/plugin/factory-hooks.ts"} {
			Expect(filepath.Join(root, filepath.FromSlash(path))).To(BeAnExistingFile(), "project scaffold stays complete: %s", path)
		}
		for _, path := range paths {
			info, err := os.Stat(filepath.Join(root, ".factory/assets/scaffold", filepath.FromSlash(path)))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
		}
		imageGit(root, "add", "factory.yaml")
		imageGit(root, "commit", "--quiet", "-m", "independent initialized project metadata")
		for _, args := range [][]string{{"selftest"}, {"metrics", "--html", "--no-open"}, {"review-lane", "enable", "MINIMAL_REVIEW_KEY"}} {
			out := handoffNativeProcess(root, f.environment, "", filepath.Join(root, "factory"), args...)
			Expect(out.status).To(Equal(0), "installed data consumer with poisoned compiler args=%q: %+v", args, out)
		}
		workflow, err := os.ReadFile(filepath.Join(root, ".github/workflows/adversarial-review.yml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(workflow)).To(ContainSubstring("MINIMAL_REVIEW_KEY"))
		_, err = os.Lstat(filepath.Join(f.records, "forbidden-tool"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "installed data consumers must not compile or call Python")
		AddReportEntry("minimal-inventory.selected-descriptor", map[string]any{"assets": len(descriptor["assets"].([]any)), "cache": cacheRows, "compiler": "poisoned"}, ReportEntryVisibilityNever)

	})
	// per docs/adr/0119-minimal-qualified-initializer-inventory.md:127
	It("reconstructs a closed predecessor across the generation boundary through preview apply and exact rollback", func() {
		f, driver, root, args := minimalInventoryOldProject(before, image, prepared, consumer)
		preserved := map[string][]byte{
			"docs/inventory-user-note.md":            []byte("user customization\n"),
			".factory/assets/scaffold/operator-note": []byte("unknown private witness\n"),
			".factory/loops.json":                    []byte("{\"schema\":1,\"runs\":[],\"note\":\"preserve history\"}\n"),
		}
		for path, data := range preserved {
			writeFixture(filepath.Join(root, filepath.FromSlash(path)), data, 0600)
		}
		snapshot := installationSnapshot(root)
		out := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...)
		Expect(out.status).To(Equal(0), "known immutable post-RC3 predecessor must cross the closed generation boundary: %+v", out)
		proposal := handoffProposal(out)
		Expect(installationSnapshot(root)).To(Equal(snapshot))
		applied := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--migration-id", "minimal-inventory-forward", "--confirm", proposal["proposal_digest"].(string), "--quiescent")...)
		Expect(applied.status).To(Equal(0), "%+v", applied)
		recordBytes, err := os.ReadFile(filepath.Join(root, ".factory/installation-transactions/minimal-inventory-forward.json"))
		Expect(err).NotTo(HaveOccurred())
		var record map[string]any
		Expect(json.Unmarshal(recordBytes, &record)).To(Succeed())
		Expect(record["entries"]).To(HaveLen(58), "the original minimal source generation must keep its 58-row journal shape")
		afterSnapshot := installationSnapshot(root)
		for path, value := range snapshot {
			if strings.HasPrefix(path, ".factory/assets/scaffold/") {
				Expect(afterSnapshot[path]).To(Equal(value), path)
			}
		}
		reverseArgs := append(append([]string{}, args...), "--rollback", "minimal-inventory-forward")
		reverse := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, reverseArgs...), "--dry-run", "--json")...))
		restored := handoffNativeProcess(root, f.environment, "", driver, append(reverseArgs, "--confirm", reverse["proposal_digest"].(string), "--quiescent")...)
		Expect(restored.status).To(Equal(0), "%+v", restored)
		restoredSnapshot := installationSnapshot(root)
		for path, value := range snapshot {
			if !strings.HasPrefix(path, ".git/") {
				Expect(restoredSnapshot[path]).To(Equal(value), path)
			}
		}
		for path, data := range preserved {
			Expect(os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))).To(Equal(data))
		}
		AddReportEntry("minimal-inventory.predecessor-roundtrip", map[string]any{"before_revision": before.revision, "after_revision": image.revision, "journal_rows": len(record["entries"].([]any))}, ReportEntryVisibilityNever)
	})

	// per docs/adr/0119-minimal-qualified-initializer-inventory.md:64
	It("keeps the original complete transaction inventory through newer-consumer rollback and rejects mixed records", func() {
		f, driver, root, _ := minimalInventoryOldProject(before, image, prepared, consumer)
		_, oldDriver := handoffNativeFixture(before)
		oldArgs := recipeOperands(before, before, handoffVersion)
		const id = "original-complete-inventory"
		minimalInventoryApply(root, oldDriver, f, oldArgs, id)
		journalPath := filepath.Join(root, ".factory/installation-transactions", id+".json")
		original, err := os.ReadFile(journalPath)
		Expect(err).NotTo(HaveOccurred())
		record := minimalInventoryJSON(journalPath)
		Expect(record["entries"]).To(HaveLen(141))
		manifestPath := filepath.Join(root, ".factory/backups", id, "installation.manifest")
		manifest := minimalInventoryJSON(manifestPath)
		Expect(manifest["entries"]).To(HaveLen(141))
		receiptPath := filepath.Join(root, ".factory/backups", id, "retention.json")
		receipt, err := os.ReadFile(receiptPath)
		Expect(err).NotTo(HaveOccurred())
		reverseArgs := append(append([]string{}, oldArgs...), "--rollback", id)
		healthy := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, reverseArgs...), "--dry-run", "--json")...)
		Expect(healthy.status).To(Equal(0), "new runtime must accept genuine original141-row evidence: %+v", healthy)
		for _, fault := range []string{"missing row", "mixed path", "duplicate path"} {
			var changed map[string]any
			Expect(json.Unmarshal(original, &changed)).To(Succeed())
			entries := changed["entries"].([]any)
			switch fault {
			case "missing row":
				changed["entries"] = entries[:len(entries)-1]
			case "mixed path":
				entries[0].(map[string]any)["path"] = ".factory/assets/scaffold/operator-mixed"
			case "duplicate path":
				entries[1].(map[string]any)["path"] = entries[0].(map[string]any)["path"]
			}
			data, err := json.Marshal(changed)
			Expect(err).NotTo(HaveOccurred())
			writeFixture(journalPath, data, 0600)
			snapshot := installationSnapshot(root)
			refused := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, reverseArgs...), "--dry-run", "--json")...)
			Expect(refused.status).To(Equal(2), "%s: %+v", fault, refused)
			Expect(installationSnapshot(root)).To(Equal(snapshot), fault)
			writeFixture(journalPath, original, 0600)
		}
		proposal := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, reverseArgs...), "--dry-run", "--json")...))
		restored := handoffNativeProcess(root, f.environment, "", driver, append(reverseArgs, "--confirm", proposal["proposal_digest"].(string), "--quiescent")...)
		Expect(restored.status).To(Equal(0), "%+v", restored)
		Expect(minimalInventoryJSON(journalPath)["entries"]).To(HaveLen(141))
		Expect(os.ReadFile(receiptPath)).To(Equal(receipt), "a newer runtime must not rewrite the old retention receipt")
		Expect(minimalInventoryJSON(manifestPath)["entries"]).To(HaveLen(141))
		AddReportEntry("minimal-inventory.old-record", map[string]any{"original_rows": 141, "remaining_rows": len(minimalInventoryJSON(journalPath)["entries"].([]any)), "mixed_refusals": 3}, ReportEntryVisibilityNever)
	})

	// per docs/adr/0119-minimal-qualified-initializer-inventory.md:64
	It("previews and resolves genuine old preparation evidence without converting its original inventory", func() {
		f, driver, root, _ := minimalInventoryOldProject(before, image, prepared, consumer)
		_, oldDriver := handoffNativeFixture(before)
		args := recipeOperands(before, before, handoffVersion)
		ignorePath := filepath.Join(root, ".gitignore")
		original, err := os.ReadFile(ignorePath)
		Expect(err).NotTo(HaveOccurred())
		exposed := "\n!/.factory/\n/.factory/bin/\n/.factory/assets/\n/.factory/installation.current\n/.factory/installation-transactions/\n/.factory/backups/\n!/.factory/bin/\n"
		writeFixture(ignorePath, append(append([]byte{}, original...), []byte(exposed)...), 0600)
		preview := handoffProposal(handoffNativeProcess(root, f.environment, "", oldDriver, append(append([]string{}, args...), "--dry-run", "--json")...))
		const id = "original-preparation-inventory"
		refused := handoffNativeProcess(root, f.environment, "", oldDriver, append(append([]string{}, args...), "--migration-id", id, "--confirm", preview["proposal_digest"].(string), "--quiescent")...)
		Expect(refused.status).To(Equal(2), "actual old publication must fail at the ignore boundary: %+v", refused)
		Expect(refused.stderr).To(ContainSubstring("not effectively ignored"))
		journalPath := filepath.Join(root, ".factory/installation-transactions", id+".json")
		oldRecord := minimalInventoryJSON(journalPath)
		Expect(oldRecord["entries"]).To(HaveLen(141))
		Expect(oldRecord["phase"]).To(Equal("prepared"))
		recoveryArgs := append(append([]string{}, args...), "--recover", id, "--direction", "reverse")
		snapshot := installationSnapshot(root)
		replay := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, recoveryArgs...), "--dry-run", "--json")...)
		Expect(replay.status).To(Equal(0), "%+v", replay)
		handoffProposal(replay)
		Expect(installationSnapshot(root)).To(Equal(snapshot))
		writeFixture(ignorePath, original, 0600)
		fresh := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, recoveryArgs...), "--dry-run", "--json")...))
		recovered := handoffNativeProcess(root, f.environment, "", driver, append(recoveryArgs, "--confirm", fresh["proposal_digest"].(string), "--quiescent")...)
		Expect(recovered.status).To(Equal(0), "%+v", recovered)
		if _, err := os.Lstat(journalPath); err == nil {
			Expect(minimalInventoryJSON(journalPath)["entries"]).To(HaveLen(141))
		} else {
			Expect(os.IsNotExist(err)).To(BeTrue(), "a resolved journal may be absent; other inspection errors must not bypass its generation check")
		}
		Expect(minimalInitializerCacheFiles(root)).To(HaveLen(85))
		AddReportEntry("minimal-inventory.old-preparation", map[string]any{"original_rows": 141, "preview_status": replay.status, "recovery_status": recovered.status}, ReportEntryVisibilityNever)
	})

})

// Independent ordered groups share fixture construction but own separate native
// images, roots and journals, so the existing two-worker lane can balance work.
// per docs/adr/0119-minimal-qualified-initializer-inventory.md:135
var _ = Describe("Supported predecessor recipe compatibility preserved historical inventory", Ordered, ContinueOnFailure, func() {
	var consumer, image, before handoffReleaseImage
	var prepared string
	BeforeAll(func() { consumer, image, before, prepared = minimalInventoryPrepare() })

	// per docs/adr/0119-minimal-qualified-initializer-inventory.md:80
	DescribeTable("preserves an unusable old cache and blocks reverse publication before changing the installation", func(missing bool) {
		f, driver, root, args := minimalInventoryOldProject(before, image, prepared, consumer)
		path := ".factory/assets/scaffold/scripts/lib/loop.py"
		absolute := filepath.Join(root, filepath.FromSlash(path))
		custom := []byte("operator-maintained inert history\n")
		if missing {
			Expect(os.Remove(absolute)).To(Succeed())
		} else {
			writeFixture(absolute, custom, 0600)
		}
		minimalInventoryApply(root, driver, f, args, "preserved-unusable-cache")
		if missing {
			_, err := os.Lstat(absolute)
			Expect(os.IsNotExist(err)).To(BeTrue())
		} else {
			Expect(os.ReadFile(absolute)).To(Equal(custom))
		}
		_, err := os.Lstat(filepath.Join(root, ".factory/backups/preserved-unusable-cache/assets", filepath.FromSlash(path)))
		Expect(os.IsNotExist(err)).To(BeTrue(), "unselected old cache is not adopted or backed up")
		snapshot := installationSnapshot(root)
		reverseArgs := append(append([]string{}, args...), "--rollback", "preserved-unusable-cache")
		reverse := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, reverseArgs...), "--dry-run", "--json")...)
		Expect(reverse.status).To(Equal(2), "%+v", reverse)
		var proposal map[string]any
		Expect(json.Unmarshal([]byte(reverse.stdout), &proposal)).To(Succeed())
		Expect(proposal["rollback_blockers"]).To(ContainElement(HaveKeyWithValue("path", path)))
		Expect(installationSnapshot(root)).To(Equal(snapshot))
		attempted := handoffNativeProcess(root, f.environment, "", driver, append(reverseArgs, "--confirm", proposal["proposal_digest"].(string), "--quiescent")...)
		Expect(attempted.status).To(Equal(2), "%+v", attempted)
		Expect(installationSnapshot(root)).To(Equal(snapshot))
	}, Entry("edited unused cache", false), Entry("missing unused cache", true))

	// per docs/adr/0119-minimal-qualified-initializer-inventory.md:35
	It("refuses every changed or missing closed predecessor member without executing supplied historical code", func() {
		f, driver, root, args := minimalInventoryOldProject(before, image, prepared, consumer)
		snapshot := installationSnapshot(root)
		marker := filepath.Join(f.cwd, "untrusted-before-executed")
		poison := []byte("#!/bin/sh\nprintf executed > '" + marker + "'\nexit 96\n")
		poisoned := handoffImageBinary(before, poison, handoffVersion)
		paths := append(append([]string{}, recipeHistoricalMembers...), recipeCurrentMembers...)
		receipts := []string{}
		for _, path := range paths {
			for _, missing := range []bool{false, true} {
				archive := recipeAsset(poisoned, path, []byte("package installation\n// unknown original recipe\n"), missing)
				source := filepath.Join(image.work, "unknown-inventory-predecessor.tar.gz")
				writeFixture(source, archive, 0600)
				out := handoffNativeProcess(root, f.environment, "", driver, append(recipeBeforePath(args, source), "--dry-run", "--json")...)
				Expect(out.status).To(Equal(2), "%s missing=%t: %+v", path, missing, out)
				Expect(out.stderr).To(ContainSubstring("recipe"))
				Expect(installationSnapshot(root)).To(Equal(snapshot))
				_, err := os.Lstat(marker)
				Expect(os.IsNotExist(err)).To(BeTrue())
				receipts = append(receipts, fmt.Sprintf("%s missing=%t status=%d", path, missing, out.status))
			}
		}
		Expect(receipts).To(HaveLen(20))
		AddReportEntry("minimal-inventory.closed-member-refusals", receipts, ReportEntryVisibilityNever)
	})

	// per docs/adr/0119-minimal-qualified-initializer-inventory.md:85
	It("retains the older recovery when a preserved historical cache cannot prove the alternate usable", func() {
		const oldest = "v9.7.0"
		f, driver := handoffNativeFixture(consumer)
		assets := f.environment["HANDOFF_ASSETS"]
		for _, version := range []string{oldest, handoffVersion, retentionReleaseC} {
			part := before
			if version == retentionReleaseC {
				part = image
			}
			binary := consumer.binary
			if version == oldest {
				binary = before.binary
			}
			archive := handoffImageBinary(part, binary, version)
			writeFixture(filepath.Join(assets, "factory-bootstrap-"+version+"-"+f.target), binary, 0600)
			writeFixture(filepath.Join(assets, "factory-installation-"+version+"-"+f.target+".tar.gz"), archive, 0600)
		}
		// The two original releases belong to the immutable old revision; only
		// the selected new release has the reduced source inventory.
		ghPath := filepath.Join(f.root, "host tools", "gh")
		gh, err := os.ReadFile(ghPath)
		Expect(err).NotTo(HaveOccurred())
		old := `  api) printf '%s\n' "$HANDOFF_REVISION" ;;`
		replacement := `  api) case "$2" in *v9.7.0|*v9.8.0-rc.1) printf '%s\n' "$HANDOFF_OLD_REVISION" ;; *) printf '%s\n' "$HANDOFF_REVISION" ;; esac ;;`
		rewritten := strings.Replace(string(gh), old, replacement, 1)
		Expect(rewritten).NotTo(Equal(string(gh)))
		writeFixture(ghPath, []byte(rewritten), 0700)
		f.environment["HANDOFF_OLD_REVISION"] = before.revision
		f.environment["HANDOFF_REVISION"] = image.revision
		root := filepath.Join(f.cwd, "historical alternate recovery adopter")
		Expect(os.Mkdir(root, 0700)).To(Succeed())
		imageGit(root, "init", "--quiet")
		initialized := handoffNativeProcess(f.cwd, f.environment, recipeAnswers, driver, "init", root, "--go-candidate", oldest, "--pack", "none")
		Expect(initialized.status).To(Equal(0), "%+v", initialized)
		Expect(minimalInitializerCacheFiles(root)).To(HaveLen(85))
		scenario := retentionScenario{f, root}
		scenario.advance(handoffVersion)
		Expect(os.ReadFile(filepath.Join(root, ".factory/bin/factory-runtime"))).To(Equal(consumer.binary), "current code owns retention preview, apply and explicit pruning")
		ids := retentionBackupIDs(root)
		Expect(ids).To(HaveLen(1))
		older := ids[0]
		oldBackup := installationSnapshot(root)
		path := ".factory/assets/scaffold/scripts/lib/loop.py"
		custom := []byte("unusable historical alternate must not authorize pruning\n")
		writeFixture(filepath.Join(root, filepath.FromSlash(path)), custom, 0600)
		preview := scenario.preview(retentionReleaseC)
		report := retentionPreviewReport(preview)
		Expect(retentionSet(report, older).Action).To(Equal("prune"), "first prove this is a genuinely eligible older recovery")
		advanced := scenario.apply(retentionReleaseC, preview)
		Expect(advanced.status).NotTo(Equal(0), "cleanup must report the unavailable alternate proof: %+v", advanced)
		applied := retentionApplyReport(advanced)
		Expect(applied.RemovedSets).To(BeZero())
		Expect(retentionSet(applied, older).Reason).To(ContainSubstring("historical cache"))
		Expect(minimalInventoryJSON(filepath.Join(root, ".factory/installation.current"))["version"]).To(Equal(retentionReleaseC), "the new installation is healthy; only cleanup is incomplete")
		Expect(os.ReadFile(filepath.Join(root, ".factory/bin/factory-runtime"))).To(Equal(consumer.binary), "the final pruning refusal must execute the current consumer")
		current := installationSnapshot(root)
		for path, value := range oldBackup {
			if strings.HasPrefix(path, ".factory/backups/"+older+"/") {
				Expect(current[path]).To(Equal(value), path)
			}
		}
		Expect(os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))).To(Equal(custom))
		snapshot := installationSnapshot(root)
		refused := retentionPruneResult(scenario, older)
		Expect(refused.status).To(Equal(2), "%+v", refused)
		Expect(installationSnapshot(root)).To(Equal(snapshot), "no usable alternate means no deletion authority")
		AddReportEntry("minimal-inventory.alternate-recovery", map[string]any{"older": older, "removed_sets": applied.RemovedSets, "reason": retentionSet(applied, older).Reason, "prune_status": refused.status}, ReportEntryVisibilityNever)
	})

})
