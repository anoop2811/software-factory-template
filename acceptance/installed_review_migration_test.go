package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const installedReviewPath = ".github/workflows/adversarial-review.yml"
const installedReviewHeader = "# Managed by: factory review-lane. Remove with: ./factory review-lane disable\n"

// per docs/adr/0154-installed-review-workflow-migration.md:66
func installedReviewAsset(image defaultInitImage, path string) []byte {
	GinkgoHelper()
	entries, _ := imageDecodeArchive(image.archive)
	for _, entry := range entries {
		if strings.HasSuffix(entry.name, "/installation/assets/"+path) {
			return entry.body
		}
	}
	Fail("missing independently inspected image asset: " + path)
	return nil
}

// Historical templates are inert fixture evidence, never executed to infer ownership.
// per docs/adr/0154-installed-review-workflow-migration.md:59
func installedReviewRender(template []byte, secret string) []byte {
	GinkgoHelper()
	Expect(string(template)).To(ContainSubstring("__REVIEW_API_KEY_SECRET__"))
	return append([]byte(installedReviewHeader), bytes.ReplaceAll(template, []byte("__REVIEW_API_KEY_SECRET__"), []byte(secret))...)
}

// per docs/adr/0154-installed-review-workflow-migration.md:66
func installedReviewLegacy(f *defaultUpgradeJourneyFixtures, secret string) (defaultUpgradeProject, []byte) {
	GinkgoHelper()
	p := f.legacyProject(defaultUpgradeNextTag)
	ciTransactionSetting(p.root, "review_lane", "on")
	ciTransactionSetting(p.root, "review_api_key_secret", secret)
	template, err := os.ReadFile(filepath.Join(p.legacy.source, "packs/review-lane/review-pr.yml"))
	Expect(err).NotTo(HaveOccurred())
	original := installedReviewRender(template, secret)
	writeFixture(filepath.Join(p.root, installedReviewPath), original, 0644)
	return p, original
}

// per docs/adr/0154-installed-review-workflow-migration.md:75
func installedReviewRows(proposal map[string]any) map[string]map[string]any {
	GinkgoHelper()
	report, ok := proposal["ci_workflow"].(map[string]any)
	Expect(ok).To(BeTrue(), "review workflow participates in the existing CI component")
	rows := map[string]map[string]any{}
	for _, value := range report["changes"].([]any) {
		row := value.(map[string]any)
		path := row["path"].(string)
		Expect(rows).NotTo(HaveKey(path))
		rows[path] = row
	}
	Expect(rows).To(HaveLen(6), "enabled review extends, rather than substitutes for, the five ordinary workflow rows")
	Expect(rows).To(HaveKey(installedReviewPath))
	return rows
}

// per docs/adr/0154-installed-review-workflow-migration.md:79
func installedReviewRead(root string) []byte {
	GinkgoHelper()
	path := filepath.Join(root, installedReviewPath)
	info, err := os.Lstat(path)
	Expect(err).NotTo(HaveOccurred())
	Expect(info.Mode().IsRegular()).To(BeTrue())
	Expect(info.Mode().Perm()).To(Equal(os.FileMode(0644)))
	body, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	return body
}

// This merged predecessor already requires 1.27.4. Older image callers retain
// the separate, unchanged exact-1.27.3 historical compiler qualification.
// per docs/adr/0154-installed-review-workflow-migration.md:129
func buildInstalledReviewPredecessor(source, destination string, args ...string) {
	GinkgoHelper()
	compiler, err := exec.LookPath("go")
	Expect(err).NotTo(HaveOccurred())
	compiler, err = filepath.EvalSymlinks(compiler)
	Expect(err).NotTo(HaveOccurred())
	Expect(filepath.IsAbs(compiler)).To(BeTrue())
	environment := map[string]string{"GOTOOLCHAIN": "local", "GOROOT": "", "GOENV": "off", "GOWORK": "off", "GOFLAGS": "", "GOEXPERIMENT": "", "GOOS": runtime.GOOS, "GOARCH": runtime.GOARCH}
	version := imageProcess(source, environment, compiler, "version")
	Expect(version.status).To(BeZero(), "%+v", version)
	Expect(strings.Fields(version.stdout)).To(Equal([]string{"go", "version", "go1.27.4", runtime.GOOS + "/" + runtime.GOARCH}))
	built := imageProcess(source, environment, compiler, args...)
	Expect(built.status).To(BeZero(), "immutable reviewed predecessor build: %+v", built)
	info, err := buildinfo.ReadFile(destination)
	Expect(err).NotTo(HaveOccurred())
	Expect(info.GoVersion).To(Equal("go1.27.4"))
}

// Initialize with the actual reviewed predecessor binary, then enable through
// that predecessor's public command; no current renderer establishes old bytes.
// per docs/adr/0154-installed-review-workflow-migration.md:59
func installedReviewGo(f *defaultUpgradeJourneyFixtures) (defaultUpgradeProject, string) {
	GinkgoHelper()
	const revision = "3794e6cc8fa66eb02504e753b0ad11955f5cf85a"
	before := immutableInstallationImage(f.invoker, revision, defaultUpgradeTag, buildInstalledReviewPredecessor)
	old, oldDriver := handoffNativeFixture(before)
	writeFixture(filepath.Join(old.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+defaultUpgradeTag+"-"+old.target), before.binary, 0600)
	writeFixture(filepath.Join(old.environment["HANDOFF_ASSETS"], "factory-installation-"+defaultUpgradeTag+"-"+old.target+".tar.gz"), before.archive, 0600)
	root := filepath.Join(old.cwd, "reviewed enabled Go predecessor")
	Expect(os.Mkdir(root, 0700)).To(Succeed())
	imageGit(root, "init", "--quiet")
	initialized := handoffNativeProcess(old.cwd, old.environment, localUpgradeAnswers, oldDriver, "init", root, "--go-candidate", defaultUpgradeTag, "--pack", "none")
	Expect(initialized.status).To(BeZero(), "%+v", initialized)
	enabled := handoffNativeProcess(root, old.environment, "", filepath.Join(root, "factory"), "review-lane", "enable", "GO_TEAM_REVIEW")
	Expect(enabled.status).To(BeZero(), "%+v", enabled)
	original := installedReviewRender(installedReviewAsset(defaultInitImage{archive: before.archive}, "packs/review-lane/review-pr.yml"), "GO_TEAM_REVIEW")
	Expect(installedReviewRead(root)).To(Equal(original))
	surface, driver := f.surface(defaultUpgradeNextTag)
	provider := filepath.Join(surface.root, "host tools/gh")
	data, err := os.ReadFile(provider)
	Expect(err).NotTo(HaveOccurred())
	const oldAPI = `  api) printf '%s\n' "$HANDOFF_REVISION" ;;`
	const newAPI = `  api) case "$2" in *v0.2.0) printf '%s\n' "$HANDOFF_OLD_REVISION" ;; *) printf '%s\n' "$HANDOFF_REVISION" ;; esac ;;`
	Expect(strings.Count(string(data), oldAPI)).To(Equal(1))
	writeFixture(provider, []byte(strings.Replace(string(data), oldAPI, newAPI, 1)), 0700)
	surface.environment["HANDOFF_OLD_REVISION"] = revision
	writeFixture(filepath.Join(surface.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+defaultUpgradeTag+"-"+surface.target), before.binary, 0600)
	writeFixture(filepath.Join(surface.environment["HANDOFF_ASSETS"], "factory-installation-"+defaultUpgradeTag+"-"+surface.target+".tar.gz"), before.archive, 0600)
	image := defaultInitImage{binary: before.binary, archive: before.archive}
	image.result.Archive = before.archivePath
	return defaultUpgradeProject{f: surface, driver: driver, root: root, beforeVersion: defaultUpgradeTag, before: image, inputs: append([]string{}, installationInputs...)}, revision
}

// The review row itself, not an earlier factory.yml row, is the crash witness.
// Reuse the sole-waiter process owner so no signal follows PID relinquishment.
// per docs/adr/0154-installed-review-workflow-migration.md:79
// per docs/adr/0151-reliable-ci-qualification.md:193
func installedReviewInterrupt(p defaultUpgradeProject, binary []byte, args []string, id string, expected []byte) {
	GinkgoHelper()
	program := filepath.Join(p.f.root, "review-transaction-owner")
	writeFixture(program, binary, 0700)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	capture, err := os.CreateTemp(p.f.root, "review-interruption-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(capture.Close)
	owner, err := startCIInterruptionOwner(ctx, func(ctx context.Context) *exec.Cmd {
		cmd := handoffCommand(ctx, p.root, p.f.environment, "", program, args...)
		cmd.Stdout, cmd.Stderr = capture, capture
		return cmd
	})
	Expect(err).NotTo(HaveOccurred())
	finish := func() ciInterruptionResult {
		result := owner.TerminateAndWait()
		if !result.Joined || result.SignalError != nil {
			for _, root := range []string{p.root, p.f.root} {
				holdFixtureCleanup(root, "review transaction interruption owner did not confirm termination")
			}
		}
		return result
	}
	DeferCleanup(func() {
		result := finish()
		Expect(result.SignalError).NotTo(HaveOccurred())
		Expect(result.Joined).To(BeTrue())
	})
	journal := filepath.Join(p.root, ".factory/installation-transactions", id+".json")
	observe := func() bool {
		exited, err := owner.Exited()
		if exited || err != nil {
			result := finish()
			body, _ := os.ReadFile(capture.Name())
			StopTrying(fmt.Sprintf("review owner exited before publication: %+v; %s", result, body)).Now()
		}
		body, err := os.ReadFile(journal)
		var record map[string]any
		if err != nil || json.Unmarshal(body, &record) != nil || record["direction"] != "forward" {
			return false
		}
		// Do not mistake a later retirement's unlink-before-record window for
		// durable ownership. All rows must have completed publication first.
		// per docs/adr/0154-installed-review-workflow-migration.md:125
		if record["phase"] != "checking" && record["phase"] != "metadata_prepared" {
			return false
		}
		for _, value := range record["entries"].([]any) {
			row := value.(map[string]any)
			if row["path"] == installedReviewPath && row["phase"] == "applied" {
				current, err := os.ReadFile(filepath.Join(p.root, installedReviewPath))
				_, pending := os.Lstat(filepath.Join(p.root, ".factory/runtime-publication.pending"))
				return err == nil && pending == nil && bytes.Equal(current, expected)
			}
		}
		return false
	}
	Eventually(observe, 80*time.Second, time.Millisecond).Should(BeTrue())
	Expect(owner.Signal(syscall.SIGSTOP)).To(Succeed())
	Eventually(func() bool {
		stopped, err := installationProcessStopped(owner.PID())
		Expect(err).NotTo(HaveOccurred())
		return stopped
	}, 5*time.Second, time.Millisecond).Should(BeTrue())
	Expect(observe()).To(BeTrue(), "the actual stopped owner must still hold review publication and pending intent")
	stopped, err := os.ReadFile(journal)
	Expect(err).NotTo(HaveOccurred())
	result := finish()
	Expect(result.SignalError).NotTo(HaveOccurred())
	Expect(result.Joined).To(BeTrue())
	Expect(result.WaitError).To(HaveOccurred())
	Expect(result.State).NotTo(BeNil())
	status, ok := result.State.Sys().(syscall.WaitStatus)
	Expect(ok).To(BeTrue())
	Expect(status.Signaled()).To(BeTrue())
	Expect(status.Signal()).To(Equal(syscall.SIGKILL))
	Expect(os.ReadFile(journal)).To(Equal(stopped), "reaped owner cannot advance review intent")
}

// Register under the original Ordered owner to reuse its qualified source,
// candidate images and legacy initializer without sharing mutable projects.
// per docs/adr/0154-installed-review-workflow-migration.md:136
func installedReviewMigrationSpecs(fixturesAccessor func() *defaultUpgradeJourneyFixtures) {
	Describe("installed review", func() {
		var fixtures *defaultUpgradeJourneyFixtures
		BeforeEach(func() {
			fixtures = fixturesAccessor()
			Expect(fixtures).NotTo(BeNil())
		})

		// per docs/adr/0154-installed-review-workflow-migration.md:59
		// per docs/adr/0154-installed-review-workflow-migration.md:75
		It("previews exact enabled Bash review as one additional authenticated transaction row", func() {
			p, original := installedReviewLegacy(fixtures, "MY_PRIVATE_REVIEW_KEY")
			imageGit(p.root, "-c", "core.hooksPath="+os.DevNull, "commit", "--quiet", "--allow-empty", "-m", "test: establish review migration preview baseline")
			before, gitBefore := installationSnapshot(p.root), ciMigrationGit(p.root)
			proposal := p.preview()
			row := installedReviewRows(proposal)[installedReviewPath]
			Expect(row["disposition"]).To(Equal("replace"))
			Expect(row["before_sha256"]).To(Equal(fmt.Sprintf("%x", sha256.Sum256(original))))
			target := installedReviewRender(installedReviewAsset(fixtures.image(defaultUpgradeTag), "packs/review-lane/review-pr.yml"), "MY_PRIVATE_REVIEW_KEY")
			Expect(row["after_sha256"]).To(Equal(fmt.Sprintf("%x", sha256.Sum256(target))))
			Expect(installationSnapshot(p.root)).To(Equal(before))
			Expect(ciMigrationGit(p.root)).To(Equal(gitBefore))
			defaultInitNoForbidden(p.f)
		})

		// per docs/adr/0154-installed-review-workflow-migration.md:17
		// per docs/adr/0154-installed-review-workflow-migration.md:75
		It("applies review with ordinary CI in one backup and restores the exact enabled Bash original", func() {
			p, original := installedReviewLegacy(fixtures, "PRIVATE_TEAM_REVIEW")
			config, err := os.ReadFile(filepath.Join(p.root, "factory.yaml"))
			Expect(err).NotTo(HaveOccurred())
			unrelated := []byte("name: Application-owned workflow\non: workflow_dispatch\n")
			writeFixture(filepath.Join(p.root, ".github/workflows/application.yml"), unrelated, 0644)
			p.f.environment["REVIEW_API_KEY"] = "NEVER_RECORD_PROVIDER_CREDENTIAL"
			proposal := p.preview()
			row := installedReviewRows(proposal)[installedReviewPath]
			applied := p.apply(proposal)
			Expect(applied.status).To(BeZero(), "%+v", applied)
			expected := installedReviewRender(installedReviewAsset(fixtures.image(defaultUpgradeTag), "packs/review-lane/review-pr.yml"), "PRIVATE_TEAM_REVIEW")
			Expect(installedReviewRead(p.root)).To(Equal(expected))
			Expect(fmt.Sprintf("%x", sha256.Sum256(expected))).To(Equal(row["after_sha256"]))
			Expect(os.ReadFile(filepath.Join(p.root, "factory.yaml"))).To(Equal(config))
			Expect(os.ReadFile(filepath.Join(p.root, ".github/workflows/application.yml"))).To(Equal(unrelated))
			ids := retentionBackupIDs(p.root)
			Expect(ids).To(HaveLen(1), "runtime, ordinary CI and review use one transaction")
			backup := filepath.Join(p.root, ".factory/backups", ids[0])
			Expect(os.ReadFile(filepath.Join(backup, "assets", installedReviewPath))).To(Equal(original))
			info, err := os.Lstat(filepath.Join(backup, "assets", installedReviewPath))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()&0111).To(BeZero(), "saved workflows remain inert")
			Expect(imageProcess(p.root, nil, "git", "check-ignore", "--no-index", "-q", "--", backup).status).To(BeZero())
			for _, path := range []string{filepath.Join(backup, "installation.manifest"), filepath.Join(p.root, ".factory/installation-transactions", ids[0]+".json")} {
				body, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(body)).NotTo(ContainSubstring(p.f.environment["REVIEW_API_KEY"]))
				var record map[string]any
				Expect(json.Unmarshal(body, &record)).To(Succeed())
				ci := record["ci_workflow"].(map[string]any)
				Expect(ci["schema_version"]).To(Equal(float64(2)))
				Expect(ci["review"]).To(Equal(map[string]any{"secret": "PRIVATE_TEAM_REVIEW", "present": true}))
			}
			args := fixtures.rollbackArgs(p, defaultUpgradeTag, ids[0])
			env := defaultUpgradeLegacyRecoveryEnvironment(p)
			reverse := handoffProposal(handoffNativeProcess(p.root, env, "", p.driver, append(append([]string{}, args...), "--dry-run", "--json")...))
			restored := handoffNativeProcess(p.root, env, "", p.driver, append(args, "--confirm", reverse["proposal_digest"].(string), "--quiescent")...)
			Expect(restored.status).To(BeZero(), "%+v", restored)
			Expect(installedReviewRead(p.root)).To(Equal(original))
			Expect(os.ReadFile(filepath.Join(p.root, "factory.yaml"))).To(Equal(config))
			defaultInitNoForbidden(p.f)
		})

		// per docs/adr/0154-installed-review-workflow-migration.md:68
		It("preserves absent and explicitly off review lanes through ordinary upgrade", func() {
			for _, variant := range []string{"enabled-absent", "off-absent", "off-custom"} {
				By(variant)
				p := fixtures.legacyProject()
				if variant == "enabled-absent" {
					ciTransactionSetting(p.root, "review_lane", "on")
				}
				var custom []byte
				if variant == "off-custom" {
					custom = []byte("name: Operator-owned dormant review\n")
					writeFixture(filepath.Join(p.root, installedReviewPath), custom, 0600)
				}
				config, err := os.ReadFile(filepath.Join(p.root, "factory.yaml"))
				Expect(err).NotTo(HaveOccurred())
				proposal := p.preview()
				if variant == "enabled-absent" {
					Expect(installedReviewRows(proposal)[installedReviewPath]["disposition"]).To(Equal("absent"))
				} else {
					Expect(proposal["ci_workflow"].(map[string]any)["changes"]).To(HaveLen(5))
				}
				Expect(p.apply(proposal).status).To(BeZero())
				Expect(os.ReadFile(filepath.Join(p.root, "factory.yaml"))).To(Equal(config))
				if custom != nil {
					Expect(os.ReadFile(filepath.Join(p.root, installedReviewPath))).To(Equal(custom))
					info, err := os.Lstat(filepath.Join(p.root, installedReviewPath))
					Expect(err).NotTo(HaveOccurred())
					Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
				} else {
					_, err := os.Lstat(filepath.Join(p.root, installedReviewPath))
					Expect(os.IsNotExist(err)).To(BeTrue())
				}
			}
		})

		// per docs/adr/0154-installed-review-workflow-migration.md:60
		// per docs/adr/0154-installed-review-workflow-migration.md:70
		It("blocks customized unsafe and ambiguously configured enabled review without mutation", func() {
			p, original := installedReviewLegacy(fixtures, "TEAM_REVIEW")
			configPath := filepath.Join(p.root, "factory.yaml")
			config, err := os.ReadFile(configPath)
			Expect(err).NotTo(HaveOccurred())
			path := filepath.Join(p.root, installedReviewPath)
			for _, variant := range []string{"custom", "header-only", "mode", "link", "duplicate-setting", "invalid-secret", "unsupported-provider"} {
				By(variant)
				writeFixture(configPath, config, 0644)
				Expect(os.Remove(path)).To(Succeed())
				writeFixture(path, original, 0644)
				switch variant {
				case "custom":
					writeFixture(path, append(original, []byte("\n# operator change\n")...), 0644)
				case "header-only":
					writeFixture(path, []byte(installedReviewHeader+"name: Unrelated custom job\n"), 0644)
				case "mode":
					Expect(os.Chmod(path, 0600)).To(Succeed())
				case "link":
					Expect(os.Remove(path)).To(Succeed())
					Expect(os.Symlink("/must/not/follow/review-workflow", path)).To(Succeed())
				case "duplicate-setting":
					config, err := os.ReadFile(filepath.Join(p.root, "factory.yaml"))
					Expect(err).NotTo(HaveOccurred())
					writeFixture(filepath.Join(p.root, "factory.yaml"), append(config, []byte("\nreview_lane: off\n")...), 0644)
				case "invalid-secret":
					ciTransactionSetting(p.root, "review_api_key_secret", "${{ secrets.INJECTED }}")
				case "unsupported-provider":
					ciTransactionSetting(p.root, "model_provider", "unknown-provider")
				}
				before := installationSnapshot(p.root)
				proposal := ciMigrationProposal(p.run("--dry-run", "--json"), 2)
				Expect(proposal["blockers"]).NotTo(BeEmpty())
				Expect(p.apply(proposal).status).To(Equal(2))
				Expect(installationSnapshot(p.root)).To(Equal(before))
				Expect(retentionBackupIDs(p.root)).To(BeEmpty())
			}
		})

		// per docs/adr/0154-installed-review-workflow-migration.md:70
		It("requires fresh consent after review bytes or relevant settings change", func() {
			p, original := installedReviewLegacy(fixtures, "TEAM_REVIEW")
			configPath := filepath.Join(p.root, "factory.yaml")
			config, err := os.ReadFile(configPath)
			Expect(err).NotTo(HaveOccurred())
			for _, changed := range []string{"workflow", "secret", "lane", "late-creation"} {
				By(changed)
				writeFixture(configPath, config, 0644)
				writeFixture(filepath.Join(p.root, installedReviewPath), original, 0644)
				if changed == "late-creation" {
					Expect(os.Remove(filepath.Join(p.root, installedReviewPath))).To(Succeed())
				}
				proposal := p.preview()
				switch changed {
				case "workflow":
					writeFixture(filepath.Join(p.root, installedReviewPath), append(original, []byte("\n# edited after consent\n")...), 0644)
				case "secret":
					ciTransactionSetting(p.root, "review_api_key_secret", "OTHER_TEAM_REVIEW")
				case "lane":
					ciTransactionSetting(p.root, "review_lane", "off")
				case "late-creation":
					writeFixture(filepath.Join(p.root, installedReviewPath), original, 0644)
				}
				before := installationSnapshot(p.root)
				refused := p.apply(proposal)
				Expect(refused.status).To(Equal(2), "%+v", refused)
				Expect(installationSnapshot(p.root)).To(Equal(before))
				Expect(retentionBackupIDs(p.root)).To(BeEmpty())
			}
		})

		// per docs/adr/0154-installed-review-workflow-migration.md:79
		DescribeTable("replays interrupted published review through the existing archive recovery owner", func(direction string) {
			p, original := installedReviewLegacy(fixtures, "RECOVERY_REVIEW_KEY")
			proposal := p.preview()
			installedReviewRows(proposal)
			id := "candidate-" + proposal["proposal_digest"].(string)[:24]
			target := installedReviewRender(installedReviewAsset(fixtures.image(defaultUpgradeTag), "packs/review-lane/review-pr.yml"), "RECOVERY_REVIEW_KEY")
			Expect(target).NotTo(Equal(original), "crash must witness genuinely new review bytes")
			installedReviewInterrupt(p, fixtures.image(defaultUpgradeTag).binary, p.args("--ref", defaultUpgradeTag, "--confirm", proposal["proposal_digest"].(string), "--quiescent"), id, target)
			args := fixtures.rollbackArgs(p, defaultUpgradeTag, id)
			for index, arg := range args {
				if arg == "--rollback" {
					args[index] = "--recover"
				}
			}
			args = append(args, "--direction", direction)
			env := defaultUpgradeLegacyRecoveryEnvironment(p)
			preview := func() cliResult {
				return handoffNativeProcess(p.root, env, "", p.driver, append(append([]string{}, args...), "--dry-run", "--json")...)
			}
			recovery := handoffProposal(preview())
			advice := ciTransactionRecoveryArguments(recovery)
			readonly := installationSnapshot(p.root)
			fresh := handoffProposal(handoffNativeProcess(p.root, env, "", p.driver, append(append([]string{}, advice...), "--dry-run", "--json")...))
			Expect(fresh["proposal_digest"]).To(Equal(recovery["proposal_digest"]))
			Expect(installationSnapshot(p.root)).To(Equal(readonly))
			result := handoffNativeProcess(p.root, env, "", p.driver, append(advice, "--confirm", recovery["proposal_digest"].(string), "--quiescent")...)
			Expect(result.status).To(BeZero(), "%+v", result)
			if direction == "forward" {
				Expect(installedReviewRead(p.root)).To(Equal(target))
			} else {
				Expect(installedReviewRead(p.root)).To(Equal(original))
			}
			_, err := os.Lstat(filepath.Join(p.root, ".factory/runtime-publication.pending"))
			Expect(os.IsNotExist(err)).To(BeTrue())
			installationAssertCompletedReplay(p.root, preview, func(digest string) cliResult {
				return handoffNativeProcess(p.root, env, "", p.driver, append(append([]string{}, args...), "--confirm", digest, "--quiescent")...)
			})
		}, Entry("forward completion", "forward"), Entry("reverse exact restoration", "reverse"))

		// per docs/adr/0154-installed-review-workflow-migration.md:79
		It("rejects missing forged or changed saved review evidence and leaves active files unchanged", func() {
			p, _ := installedReviewLegacy(fixtures, "TEAM_REVIEW")
			proposal := p.preview()
			Expect(p.apply(proposal).status).To(BeZero())
			ids := retentionBackupIDs(p.root)
			Expect(ids).To(HaveLen(1))
			paths := []string{
				filepath.Join(p.root, ".factory/installation-transactions", ids[0]+".json"),
				filepath.Join(p.root, ".factory/backups", ids[0], "installation.manifest"),
				filepath.Join(p.root, ".factory/backups", ids[0], "assets", installedReviewPath),
			}
			originals := map[string][]byte{}
			for _, path := range paths {
				body, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())
				originals[path] = body
			}
			args := fixtures.rollbackArgs(p, defaultUpgradeTag, ids[0])
			env := defaultUpgradeLegacyRecoveryEnvironment(p)
			healthy := handoffNativeProcess(p.root, env, "", p.driver, append(append([]string{}, args...), "--dry-run", "--json")...)
			Expect(healthy.status).To(BeZero(), "healthy evidence control: %+v", healthy)
			for _, variant := range []string{"missing-context", "missing-row", "duplicate-row", "forged-action", "changed-secret", "changed-backup"} {
				By(variant)
				for _, path := range paths[:2] {
					var record map[string]any
					Expect(json.Unmarshal(originals[path], &record)).To(Succeed())
					ci := record["ci_workflow"].(map[string]any)
					switch variant {
					case "missing-context":
						delete(ci, "review")
					case "changed-secret":
						ci["review"].(map[string]any)["secret"] = "OTHER_KEY"
					case "missing-row", "duplicate-row", "forged-action":
						rows := []any{}
						for _, value := range record["entries"].([]any) {
							row := value.(map[string]any)
							if row["path"] == installedReviewPath {
								if variant == "missing-row" {
									continue
								}
								if variant == "forged-action" {
									row["action"] = "retain"
								} else {
									rows = append(rows, row)
								}
							}
							rows = append(rows, row)
						}
						record["entries"] = rows
					}
					body, err := json.Marshal(record)
					Expect(err).NotTo(HaveOccurred())
					writeFixture(path, body, 0600)
				}
				if variant == "changed-backup" {
					writeFixture(paths[2], []byte("untrusted saved review\n"), 0600)
				}
				before := installationSnapshot(p.root)
				refused := handoffNativeProcess(p.root, env, "", p.driver, append(append([]string{}, args...), "--dry-run", "--json")...)
				Expect(refused.status).NotTo(BeZero(), "%s: %+v", variant, refused)
				Expect(installationSnapshot(p.root)).To(Equal(before))
				for path, body := range originals {
					writeFixture(path, body, 0600)
				}
			}
			healthy = handoffNativeProcess(p.root, env, "", p.driver, append(append([]string{}, args...), "--dry-run", "--json")...)
			Expect(healthy.status).To(BeZero(), "restored evidence control: %+v", healthy)
		})

		// per docs/adr/0154-installed-review-workflow-migration.md:59
		// per docs/adr/0154-installed-review-workflow-migration.md:75
		It("refreshes a genuinely initialized Go predecessor and restores its exact 0644 review independently of ordinary CI mode", func() {
			p, revision := installedReviewGo(fixtures)
			original := installedReviewRead(p.root)
			config, err := os.ReadFile(filepath.Join(p.root, "factory.yaml"))
			Expect(err).NotTo(HaveOccurred())
			proposal := p.preview("--ref", defaultUpgradeNextTag)
			row := installedReviewRows(proposal)[installedReviewPath]
			Expect(row["disposition"]).To(Equal("replace"))
			Expect(p.apply(proposal, "--ref", defaultUpgradeNextTag).status).To(BeZero())
			expected := installedReviewRender(installedReviewAsset(fixtures.image(defaultUpgradeNextTag), "packs/review-lane/review-pr.yml"), "GO_TEAM_REVIEW")
			Expect(installedReviewRead(p.root)).To(Equal(expected))
			Expect(expected).NotTo(Equal(original))
			Expect(os.ReadFile(filepath.Join(p.root, "factory.yaml"))).To(Equal(config))
			ids := retentionBackupIDs(p.root)
			Expect(ids).To(HaveLen(1))
			Expect(os.ReadFile(filepath.Join(p.root, ".factory/backups", ids[0], "assets", installedReviewPath))).To(Equal(original))
			args := fixtures.rollbackArgs(p, defaultUpgradeNextTag, ids[0])
			for index, arg := range args {
				if arg == "--before-revision" {
					args[index+1] = revision
				}
			}
			reverse := handoffProposal(handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(append([]string{}, args...), "--dry-run", "--json")...))
			restored := handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(args, "--confirm", reverse["proposal_digest"].(string), "--quiescent")...)
			Expect(restored.status).To(BeZero(), "%+v", restored)
			Expect(installedReviewRead(p.root)).To(Equal(original))
			Expect(os.ReadFile(filepath.Join(p.root, ".factory/bin/factory-runtime"))).To(Equal(p.before.binary))
			defaultInitNoForbidden(p.f)
		})

		// per docs/adr/0154-installed-review-workflow-migration.md:75
		It("retires only eligible older review backups while keeping immediate predecessor restoration usable", func() {
			p, original := installedReviewLegacy(fixtures, "RETENTION_REVIEW")
			first := p.preview()
			Expect(p.apply(first).status).To(BeZero())
			ids := retentionBackupIDs(p.root)
			Expect(ids).To(HaveLen(1))
			oldID := ids[0]
			Expect(os.ReadFile(filepath.Join(p.root, ".factory/backups", oldID, "assets", installedReviewPath))).To(Equal(original))
			middle := installedReviewRead(p.root)
			next := p.preview("--ref", defaultUpgradeNextTag)
			planned := retentionPreviewReport(next)
			Expect(retentionSet(planned, oldID).Action).To(Equal("prune"))
			applied := p.apply(next, "--ref", defaultUpgradeNextTag)
			Expect(applied.status).To(BeZero(), "%+v", applied)
			Expect(retentionApplyReport(applied).RemovedSets).To(Equal(1))
			remaining := retentionBackupIDs(p.root)
			Expect(remaining).To(HaveLen(1))
			Expect(remaining).NotTo(ContainElement(oldID))
			Expect(os.ReadFile(filepath.Join(p.root, ".factory/backups", remaining[0], "assets", installedReviewPath))).To(Equal(middle))
			p.beforeVersion, p.before = defaultUpgradeTag, fixtures.image(defaultUpgradeTag)
			args := fixtures.rollbackArgs(p, defaultUpgradeNextTag, remaining[0])
			reverse := handoffProposal(handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(append([]string{}, args...), "--dry-run", "--json")...))
			restored := handoffNativeProcess(p.root, p.f.environment, "", p.driver, append(args, "--confirm", reverse["proposal_digest"].(string), "--quiescent")...)
			Expect(restored.status).To(BeZero(), "%+v", restored)
			Expect(installedReviewRead(p.root)).To(Equal(middle))
			defaultInitNoForbidden(p.f)
		})
	})
}
