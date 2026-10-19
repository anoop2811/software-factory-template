package acceptance_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const retentionReleaseA = "v9.8.0"
const retentionReleaseC = "v9.8.2"

type retentionImages struct {
	image              handoffReleaseImage
	binaries, archives map[string][]byte
}

type retentionScenario struct {
	transport handoffBootstrapFixture
	root      string
}

type retentionSetOracle struct {
	MigrationID string `json:"migration_id"`
	Path        string `json:"path"`
	Action      string `json:"action"`
	Reason      string `json:"reason"`
	Files       *int64 `json:"files"`
	Bytes       *int64 `json:"bytes"`
	NextAction  string `json:"next_action"`
	Restorable  bool   `json:"restorable"`
}

type retentionReportOracle struct {
	SchemaVersion int                  `json:"schema_version"`
	Mode          string               `json:"mode"`
	Status        string               `json:"status"`
	Advanced      bool                 `json:"advanced"`
	Sets          []retentionSetOracle `json:"sets"`
	RemovedSets   int                  `json:"removed_sets"`
	RemovedFiles  int64                `json:"removed_files"`
	RemovedBytes  int64                `json:"removed_bytes"`
	RetainedSets  int                  `json:"retained_sets"`
	RetainedFiles int64                `json:"retained_files"`
	RetainedBytes int64                `json:"retained_bytes"`
	CountsKnown   bool                 `json:"counts_known"`
}

// Per-release executable bytes differ, while every source image has the same
// independently committed canonical recipe. No historical executable is a test oracle.
// per docs/adr/0106-go-backup-retention.md:38
func retentionPrepareImages() retentionImages {
	GinkgoHelper()
	image := handoffBuildImage()
	result := retentionImages{image, map[string][]byte{handoffVersion: image.binary}, map[string][]byte{handoffVersion: image.archive}}
	for _, version := range []string{retentionReleaseA, retentionReleaseC} {
		path := filepath.Join(image.work, "compiled-"+version)
		args := []string{"build", "-ldflags=-buildid=retention-" + version, "-o", path, "./cmd/factory"}
		if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
			args = append([]string{"build", "-race"}, args[1:]...)
		}
		built := imageProcess(filepath.Join(image.work, "exact-source"), nil, "go", args...)
		Expect(built.status).To(Equal(0), "%+v", built)
		binary, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(binary).NotTo(Equal(image.binary))
		result.binaries[version] = binary
		result.archives[version] = handoffImageBinary(image, binary, version)
	}
	Expect(result.binaries[retentionReleaseA]).NotTo(Equal(result.binaries[retentionReleaseC]))
	return result
}

// per docs/adr/0106-go-backup-retention.md:101
func (images retentionImages) freshA() retentionScenario {
	GinkgoHelper()
	f, driver := handoffNativeFixture(images.image)
	for _, version := range []string{retentionReleaseA, retentionReleaseC} {
		writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+version+"-"+f.target), images.binaries[version], 0600)
		writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-installation-"+version+"-"+f.target+".tar.gz"), images.archives[version], 0600)
	}
	for _, tool := range []string{"claude", "codex", "opencode", "ollama"} {
		writeFixture(filepath.Join(f.root, "host tools", tool), []byte("#!/bin/sh\nprintf called >> \"$HANDOFF_RECORD_DIR/forbidden-tool\"\nexit 97\n"), 0700)
	}
	root := filepath.Join(f.cwd, "retention project")
	Expect(os.Mkdir(root, 0700)).To(Succeed())
	git := imageProcess(root, nil, "git", "init", "--quiet", "--template=")
	Expect(git.status).To(Equal(0), "%+v", git)
	initialized := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", root, "--go-candidate", retentionReleaseA, "--pack", "none")
	Expect(initialized.status).To(Equal(0), "%+v", initialized)
	actual, err := os.ReadFile(filepath.Join(root, ".factory/bin/factory-runtime"))
	Expect(err).NotTo(HaveOccurred())
	Expect(actual).To(Equal(images.binaries[retentionReleaseA]))
	return retentionScenario{f, root}
}

// per docs/adr/0106-go-backup-retention.md:58
func (scenario retentionScenario) preview(version string) map[string]any {
	GinkgoHelper()
	before := installationSnapshot(scenario.root)
	proposal := handoffProposal(handoffNativeProcess(scenario.root, scenario.transport.environment, "", filepath.Join(scenario.root, "factory"), "upgrade", "--go-candidate", version, "--dry-run", "--json"))
	Expect(installationSnapshot(scenario.root)).To(Equal(before), "release/retention preview must not write into the adopter")
	return proposal
}

// per docs/adr/0106-go-backup-retention.md:18
func (scenario retentionScenario) advance(version string) cliResult {
	GinkgoHelper()
	proposal := scenario.preview(version)
	Expect(proposal["blockers"]).To(BeEmpty())
	result := scenario.apply(version, proposal)
	Expect(result.status).To(Equal(0), "%+v", result)
	return result
}

func (scenario retentionScenario) apply(version string, proposal map[string]any) cliResult {
	return handoffNativeProcess(scenario.root, scenario.transport.environment, "", filepath.Join(scenario.root, "factory"), "upgrade", "--go-candidate", version, "--confirm", proposal["proposal_digest"].(string), "--quiescent")
}

func retentionBackupIDs(root string) []string {
	GinkgoHelper()
	entries, err := os.ReadDir(filepath.Join(root, ".factory/backups"))
	if os.IsNotExist(err) {
		return []string{}
	}
	Expect(err).NotTo(HaveOccurred())
	result := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			result = append(result, entry.Name())
		}
	}
	return result
}

// Standalone controls report JSON without a source-selection or JSON flag.
// per docs/adr/0106-go-backup-retention.md:63
func retentionControl(scenario retentionScenario, flag, migration string) retentionReportOracle {
	GinkgoHelper()
	result := handoffNativeProcess(scenario.root, scenario.transport.environment, "", filepath.Join(scenario.root, "factory"), "upgrade", flag, migration)
	Expect(result.status).To(Equal(0), "%+v", result)
	var report retentionReportOracle
	decoder := json.NewDecoder(strings.NewReader(result.stdout))
	Expect(decoder.Decode(&report)).To(Succeed(), "%+v", result)
	var trailing any
	Expect(decoder.Decode(&trailing)).To(MatchError(io.EOF))
	Expect(report.SchemaVersion).To(Equal(1))
	Expect(report.Status).To(Equal("completed"))
	return report
}

// per docs/adr/0106-go-backup-retention.md:73
func retentionDecode(data []byte) retentionReportOracle {
	GinkgoHelper()
	var report retentionReportOracle
	decoder := json.NewDecoder(bytes.NewReader(data))
	Expect(decoder.Decode(&report)).To(Succeed())
	var trailing any
	Expect(decoder.Decode(&trailing)).To(MatchError(io.EOF))
	Expect(report.SchemaVersion).To(Equal(1))
	return report
}

func retentionPreviewReport(proposal map[string]any) retentionReportOracle {
	GinkgoHelper()
	Expect(proposal).To(HaveKey("retention"))
	data, err := json.Marshal(proposal["retention"])
	Expect(err).NotTo(HaveOccurred())
	report := retentionDecode(data)
	Expect(report.Mode).To(Equal("retention-preview"))
	Expect(report.Status).To(Equal("planned"))
	return report
}

func retentionApplyReport(result cliResult) retentionReportOracle {
	GinkgoHelper()
	reports := []string{}
	for _, line := range strings.Split(result.stderr, "\n") {
		if data, ok := strings.CutPrefix(line, "factory retention: "); ok {
			reports = append(reports, data)
		}
	}
	Expect(reports).To(HaveLen(1), "apply must visibly report retention outcomes: %+v", result)
	return retentionDecode([]byte(reports[0]))
}

func retentionSet(report retentionReportOracle, id string) retentionSetOracle {
	GinkgoHelper()
	found := []retentionSetOracle{}
	for _, row := range report.Sets {
		if row.MigrationID == id {
			found = append(found, row)
		}
	}
	Expect(found).To(HaveLen(1), "each named recovery set must have exactly one outcome")
	Expect(found[0].Path).To(Equal(".factory/backups/" + id))
	Expect(found[0].Reason).NotTo(BeEmpty())
	Expect(found[0].Restorable).To(BeFalse(), "a report never grants restoration authority")
	return found[0]
}

// Count actual test-owned regular files, including metadata, without trusting the report.
// per docs/adr/0106-go-backup-retention.md:73
func retentionCounts(root, id string) (int64, int64) {
	GinkgoHelper()
	var files, total int64
	Expect(filepath.WalkDir(filepath.Join(root, ".factory/backups", id), func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		Expect(info.Mode().IsRegular()).To(BeTrue(), "healthy count oracle never follows links")
		files++
		total += info.Size()
		return nil
	})).To(Succeed())
	return files, total
}

func (images retentionImages) atB() (retentionScenario, string) {
	GinkgoHelper()
	scenario := images.freshA()
	scenario.advance(handoffVersion)
	ids := retentionBackupIDs(scenario.root)
	Expect(ids).To(HaveLen(1))
	return scenario, ids[0]
}

func (images retentionImages) heldAtC() (retentionScenario, string, string) {
	GinkgoHelper()
	scenario, a := images.atB()
	retentionControl(scenario, "--keep-backup", a)
	scenario.advance(retentionReleaseC)
	ids := retentionBackupIDs(scenario.root)
	Expect(ids).To(HaveLen(2))
	Expect(ids).To(ContainElement(a))
	for _, id := range ids {
		if id != a {
			return scenario, a, id
		}
	}
	Fail("C must retain distinct recovery for B")
	return scenario, "", ""
}

func retentionPruneResult(scenario retentionScenario, id string) cliResult {
	return handoffNativeProcess(scenario.root, scenario.transport.environment, "", filepath.Join(scenario.root, "factory"), "upgrade", "--prune-backup", id)
}

func retentionRequireRefusal(result cliResult) {
	GinkgoHelper()
	Expect(result.status).To(Equal(2), "%+v", result)
	Expect(result.stderr).NotTo(BeEmpty(), "a refusal must explain the preserved recovery")
}

// The family retains the reviewed handoff selector, so every new public criterion
// belongs to exactly one source-qualification partition.
// per docs/adr/0106-go-backup-retention.md:109
var _ = Describe("Whole installation upgrade release candidate handoff backup retention", Ordered, ContinueOnFailure, Label("handoff-backup-retention"), func() {
	var images retentionImages
	BeforeAll(func() { images = retentionPrepareImages() })

	// per specs/001-go-runtime-conversion.md:225
	// per docs/adr/0106-go-backup-retention.md:63
	It("holds a complete predecessor backup through the installed public command without activation or acquisition", func() {
		scenario := images.freshA()
		scenario.advance(handoffVersion)
		ids := retentionBackupIDs(scenario.root)
		Expect(ids).To(HaveLen(1))
		beforeRuntime, err := os.ReadFile(filepath.Join(scenario.root, ".factory/bin/factory-runtime"))
		Expect(err).NotTo(HaveOccurred())
		beforeTrace, err := os.ReadFile(filepath.Join(scenario.transport.records, "trace"))
		Expect(err).NotTo(HaveOccurred())
		report := retentionControl(scenario, "--keep-backup", ids[0])
		Expect(report.Mode).To(Equal("keep-backup"))
		Expect(report.Advanced).To(BeFalse())
		Expect(report.Sets).To(ContainElement(And(HaveField("MigrationID", ids[0]), HaveField("Action", "held"))))
		retentionControl(scenario, "--keep-backup", ids[0])
		afterRuntime, err := os.ReadFile(filepath.Join(scenario.root, ".factory/bin/factory-runtime"))
		Expect(err).NotTo(HaveOccurred())
		Expect(afterRuntime).To(Equal(beforeRuntime))
		afterTrace, err := os.ReadFile(filepath.Join(scenario.transport.records, "trace"))
		Expect(err).NotTo(HaveOccurred())
		Expect(afterTrace).To(Equal(beforeTrace), "holding a complete set must not acquire releases or select a different runtime")
	})

	// per specs/001-go-runtime-conversion.md:218
	// per docs/adr/0106-go-backup-retention.md:18
	It("prunes A only after a successful ordinary authenticated A to B to C release chain", func() {
		scenario := images.freshA()
		Expect(retentionBackupIDs(scenario.root)).To(BeEmpty(), "fresh A has no predecessor to expire")
		scenario.advance(handoffVersion)
		ids := retentionBackupIDs(scenario.root)
		Expect(ids).To(HaveLen(1), "the first forward release must retain recovery for A")
		first := ids[0]
		savedA, err := os.ReadFile(filepath.Join(scenario.root, ".factory/backups", first, "assets/.factory/bin/factory-runtime"))
		Expect(err).NotTo(HaveOccurred())
		Expect(bytes.Equal(savedA, images.binaries[retentionReleaseA])).To(BeTrue(), "the retained set must contain the exact distinct predecessor bytes")
		files, size := retentionCounts(scenario.root, first)
		loopHistory := []byte("{\"schema\":1,\"runs\":[],\"note\":\"operator history must survive\"}\n")
		eventHistory := []byte("2026-10-19T00:00:00Z\toperator\tpreserved accounting history\n")
		writeFixture(filepath.Join(scenario.root, ".factory/loops.json"), loopHistory, 0600)
		writeFixture(filepath.Join(scenario.root, ".factory/events.log"), eventHistory, 0600)
		preview := scenario.preview(retentionReleaseC)
		planned := retentionPreviewReport(preview)
		Expect(planned.Advanced).To(BeTrue())
		row := retentionSet(planned, first)
		Expect(row.Action).To(Equal("prune"))
		Expect(row.Files).NotTo(BeNil())
		Expect(row.Bytes).NotTo(BeNil())
		Expect(*row.Files).To(Equal(files))
		Expect(*row.Bytes).To(Equal(size))
		result := scenario.apply(retentionReleaseC, preview)
		Expect(result.status).To(Equal(0), "%+v", result)
		applied := retentionApplyReport(result)
		Expect(applied.Advanced).To(BeTrue())
		Expect(applied.RemovedSets).To(Equal(1))
		Expect(applied.RemovedFiles).To(Equal(files))
		Expect(applied.RemovedBytes).To(Equal(size))
		later := retentionBackupIDs(scenario.root)
		Expect(later).To(HaveLen(1), "successful C must expire the eligible older A set, not accumulate both")
		Expect(later).NotTo(ContainElement(first))
		savedB, err := os.ReadFile(filepath.Join(scenario.root, ".factory/backups", later[0], "assets/.factory/bin/factory-runtime"))
		Expect(err).NotTo(HaveOccurred())
		Expect(bytes.Equal(savedB, images.binaries[handoffVersion])).To(BeTrue(), "B recovery must independently contain the exact distinct B runtime after A is gone")
		retainedFiles, retainedBytes := retentionCounts(scenario.root, later[0])
		Expect(applied.RetainedSets).To(Equal(1))
		Expect(applied.RetainedFiles).To(Equal(retainedFiles))
		Expect(applied.RetainedBytes).To(Equal(retainedBytes))
		Expect(applied.CountsKnown).To(BeTrue())
		activeRuntime, err := os.ReadFile(filepath.Join(scenario.root, ".factory/bin/factory-runtime"))
		Expect(err).NotTo(HaveOccurred())
		Expect(activeRuntime).To(Equal(images.binaries[retentionReleaseC]))
		Expect(os.ReadFile(filepath.Join(scenario.root, ".factory/loops.json"))).To(Equal(loopHistory))
		events, readErr := os.ReadFile(filepath.Join(scenario.root, ".factory/events.log"))
		Expect(readErr).NotTo(HaveOccurred())
		Expect(bytes.HasPrefix(events, eventHistory)).To(BeTrue(), "retention must not erase accounting history")
		// Prove retained B is usable after A's bytes and journal have actually gone.
		// per docs/adr/0106-go-backup-retention.md:20
		rollback := []string{"upgrade", "--installation", "--profile", "go-hybrid-v1", "--rollback", later[0]}
		roots := filepath.Join(scenario.transport.root, "rollback-trusted-roots.jsonl")
		writeFixture(roots, []byte("independently obtained fixture roots\n"), 0600)
		for _, source := range []struct{ prefix, version string }{{"", retentionReleaseC}, {"before-", handoffVersion}} {
			rollback = append(rollback, "--"+source.prefix+"source", filepath.Join(scenario.transport.environment["HANDOFF_ASSETS"], "factory-installation-"+source.version+"-"+scenario.transport.target+".tar.gz"), "--"+source.prefix+"version", source.version,
				"--"+source.prefix+"revision", images.image.revision, "--"+source.prefix+"target", runtime.GOOS+"/"+runtime.GOARCH, "--"+source.prefix+"attestation", filepath.Join(scenario.transport.environment["HANDOFF_ASSETS"], "runtime-attestations.jsonl"), "--"+source.prefix+"trusted-root", roots)
		}
		receipt, readErr := os.ReadFile(filepath.Join(scenario.root, ".factory/backups", later[0], "retention.json"))
		Expect(readErr).NotTo(HaveOccurred())
		var hints struct {
			Inputs map[string]string `json:"inputs"`
		}
		Expect(json.Unmarshal(receipt, &hints)).To(Succeed())
		Expect(hints.Inputs).NotTo(BeEmpty())
		keys := []string{}
		for key := range hints.Inputs {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			rollback = append(rollback, "--project-input", key+"="+hints.Inputs[key])
		}
		command := filepath.Join(scenario.root, "factory")
		reverse := handoffProposal(handoffNativeProcess(scenario.root, scenario.transport.environment, "", command, append(append([]string{}, rollback...), "--dry-run", "--json")...))
		restored := handoffNativeProcess(scenario.root, scenario.transport.environment, "", command, append(rollback, "--confirm", reverse["proposal_digest"].(string), "--quiescent")...)
		Expect(restored.status).To(Equal(0), "%+v", restored)
		Expect(retentionBackupIDs(scenario.root)).To(Equal(later), "rollback must not expire any other recovery")
		restoredBytes, readErr := os.ReadFile(filepath.Join(scenario.root, ".factory/bin/factory-runtime"))
		Expect(readErr).NotTo(HaveOccurred())
		Expect(bytes.Equal(restoredBytes, images.binaries[handoffVersion])).To(BeTrue())
		ordinary := handoffNativeProcess(scenario.root, scenario.transport.environment, "", command, "selftest")
		Expect(ordinary.status).To(Equal(0), "%+v", ordinary)
		Expect(os.ReadFile(filepath.Join(scenario.root, ".factory/loops.json"))).To(Equal(loopHistory))
		_, err = os.Lstat(filepath.Join(scenario.transport.records, "forbidden-tool"))
		Expect(os.IsNotExist(err)).To(BeTrue(), fmt.Sprint(err))
	})

	// per docs/adr/0106-go-backup-retention.md:63
	// per specs/001-go-runtime-conversion.md:225
	It("preserves a held A through C and explicitly prunes it only while exact B recovery remains usable", func() {
		scenario, a, b := images.heldAtC()
		beforeB := installationSnapshot(filepath.Join(scenario.root, ".factory/backups", b))
		files, size := retentionCounts(scenario.root, a)
		report := retentionControl(scenario, "--prune-backup", a)
		Expect(report.Mode).To(Equal("prune-backup"))
		Expect(report.RemovedSets).To(Equal(1))
		Expect(report.RemovedFiles).To(Equal(files))
		Expect(report.RemovedBytes).To(Equal(size))
		Expect(retentionSet(report, a).Action).To(Equal("removed"))
		Expect(retentionSet(report, b).Action).To(Equal("retain"))
		Expect(retentionBackupIDs(scenario.root)).To(Equal([]string{b}))
		Expect(installationSnapshot(filepath.Join(scenario.root, ".factory/backups", b))).To(Equal(beforeB))
		again := retentionControl(scenario, "--prune-backup", a)
		Expect(again.RemovedSets).To(Equal(0))
		Expect(again.RemovedFiles).To(Equal(int64(0)))
		Expect(again.RemovedBytes).To(Equal(int64(0)))
		Expect(retentionSet(again, a).Action).To(Equal("already-removed"))
		Expect(retentionBackupIDs(scenario.root)).To(Equal([]string{b}))
	})

	// per docs/adr/0106-go-backup-retention.md:48
	It("refuses to prune the sole complete recovery set even when it is explicitly held", func() {
		scenario, a := images.atB()
		retentionControl(scenario, "--keep-backup", a)
		before := installationSnapshot(scenario.root)
		result := retentionPruneResult(scenario, a)
		retentionRequireRefusal(result)
		Expect(installationSnapshot(scenario.root)).To(Equal(before))
		Expect(retentionBackupIDs(scenario.root)).To(Equal([]string{a}))
	})

	// per docs/adr/0106-go-backup-retention.md:58
	DescribeTable("refuses stale release confirmation after retention observations change", func(change string) {
		scenario, a := images.atB()
		proposal := scenario.preview(retentionReleaseC)
		if change == "hold" {
			retentionControl(scenario, "--keep-backup", a)
		} else {
			path := filepath.Join(scenario.root, ".factory/backups", a, "assets/.factory/bin/factory-runtime")
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
			Expect(err).NotTo(HaveOccurred())
			_, err = file.Write([]byte("operator edit"))
			Expect(err).NotTo(HaveOccurred())
			Expect(file.Close()).To(Succeed())
		}
		before := installationSnapshot(scenario.root)
		result := scenario.apply(retentionReleaseC, proposal)
		retentionRequireRefusal(result)
		Expect(installationSnapshot(scenario.root)).To(Equal(before), "stale consent cannot activate C or delete any recovery")
	}, Entry("a new explicit hold", "hold"), Entry("an edited saved before-image", "saved-image"))

	// per docs/adr/0106-go-backup-retention.md:22
	It("keeps prior recovery on a same-release rerun without advancing retention", func() {
		scenario, a := images.atB()
		proposal := scenario.preview(handoffVersion)
		report := retentionPreviewReport(proposal)
		Expect(report.Advanced).To(BeFalse())
		Expect(retentionSet(report, a).Action).NotTo(Equal("prune"))
		result := scenario.apply(handoffVersion, proposal)
		Expect(result.status).To(Equal(0), "%+v", result)
		applied := retentionApplyReport(result)
		Expect(applied.Advanced).To(BeFalse())
		Expect(applied.RemovedSets).To(Equal(0))
		Expect(retentionBackupIDs(scenario.root)).To(ContainElement(a))
	})

	// per docs/adr/0106-go-backup-retention.md:18
	It("does not expire A when C fails its actual installed native proof", func() {
		scenario, a := images.atB()
		before := installationSnapshot(filepath.Join(scenario.root, ".factory/backups", a))
		broken := handoffRewriteAsset(images.archives[retentionReleaseC], "scripts/hooks/test-edit-denial.sh", func([]byte) []byte { return []byte("#!/bin/sh\nexit 0\n") })
		writeFixture(filepath.Join(scenario.transport.environment["HANDOFF_ASSETS"], "factory-installation-"+retentionReleaseC+"-"+scenario.transport.target+".tar.gz"), broken, 0600)
		proposal := scenario.preview(retentionReleaseC)
		result := scenario.apply(retentionReleaseC, proposal)
		Expect(result.status).NotTo(Equal(0), "%+v", result)
		Expect(result.stderr).To(ContainSubstring("proof"))
		Expect(retentionBackupIDs(scenario.root)).To(ContainElement(a))
		Expect(installationSnapshot(filepath.Join(scenario.root, ".factory/backups", a))).To(Equal(before))
	})
})

// per docs/adr/0106-go-backup-retention.md:101
var _ = Describe("Whole installation upgrade release candidate handoff backup retention safeguards", Ordered, ContinueOnFailure, Label("handoff-retention-safeguards"), func() {
	var images retentionImages
	BeforeAll(func() { images = retentionPrepareImages() })

	// per docs/adr/0106-go-backup-retention.md:73
	DescribeTable("preserves unsafe older sets with a visible reason instead of recursively deleting them", func(change string) {
		scenario, a, b := images.heldAtC()
		set := filepath.Join(scenario.root, ".factory/backups", a)
		outside := filepath.Join(scenario.transport.root, "outside-preserved")
		writeFixture(outside, []byte("outside reference"), 0600)
		switch change {
		case "extra":
			writeFixture(filepath.Join(set, "operator-note"), []byte("keep this"), 0600)
		case "symlink":
			Expect(os.Symlink(outside, filepath.Join(set, "outside-link"))).To(Succeed())
		case "hardlink":
			Expect(os.Link(outside, filepath.Join(set, "outside-hardlink"))).To(Succeed())
		case "edited":
			file, err := os.OpenFile(filepath.Join(set, "assets/.factory/bin/factory-runtime"), os.O_WRONLY|os.O_APPEND, 0600)
			Expect(err).NotTo(HaveOccurred())
			_, err = file.Write([]byte("edited recovery"))
			Expect(err).NotTo(HaveOccurred())
			Expect(file.Close()).To(Succeed())
		case "unknown-record":
			writeFixture(filepath.Join(set, "retention.json"), []byte("{\"schema_version\":999}\n"), 0600)
		case "unsafe-directory":
			Expect(os.Chmod(set, 0777)).To(Succeed()) // #nosec G302 -- deliberately unsafe test-owned backup directory.
		case "unignored":
			path := filepath.Join(scenario.root, ".gitignore")
			body, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			writeFixture(path, append(body, []byte("\n!/.factory/\n!/.factory/backups/\n!/.factory/backups/**\n")...), 0644)
		case "tracked":
			git := imageProcess(scenario.root, nil, "git", "-c", "core.hooksPath="+os.DevNull, "add", "--force", "--", ".factory/backups/"+a+"/installation.manifest")
			Expect(git.status).To(Equal(0), "%+v", git)
		}
		before := installationSnapshot(scenario.root)
		result := retentionPruneResult(scenario, a)
		retentionRequireRefusal(result)
		Expect(result.stderr).To(MatchRegexp(`(?i)(preserv|unsafe|changed|unknown|unrecognized|ignored|tracked|complete|conflict)`))
		Expect(installationSnapshot(scenario.root)).To(Equal(before), "a refused older set and its usable alternate must remain unchanged")
		Expect(retentionBackupIDs(scenario.root)).To(ConsistOf(a, b))
		Expect(os.ReadFile(outside)).To(Equal([]byte("outside reference")))
	}, Entry("operator file", "extra"), Entry("external symlink", "symlink"), Entry("foreign hardlink", "hardlink"), Entry("edited before-image", "edited"), Entry("unknown record schema", "unknown-record"), Entry("unsafe directory permissions", "unsafe-directory"), Entry("no longer ignored", "unignored"), Entry("tracked recovery path", "tracked"))

	// per docs/adr/0106-go-backup-retention.md:48
	DescribeTable("refuses coherent legacy-shaped alternate records that omit freshly required B roles", func(attack string) {
		scenario, a, b := images.heldAtC()
		base := filepath.Join(scenario.root, ".factory/backups", b)
		marker, err := os.ReadFile(filepath.Join(base, "assets/.factory-version"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(marker)).To(ContainSubstring(handoffVersion))
		Expect(os.Remove(filepath.Join(base, "retention.json"))).To(Succeed())
		if attack == "retained-roles" {
			Expect(os.RemoveAll(filepath.Join(base, "assets"))).To(Succeed())
			writeFixture(filepath.Join(base, "assets/.factory-version"), marker, 0600)
		} else {
			Expect(os.Remove(filepath.Join(base, "assets/.factory/bin/factory-runtime"))).To(Succeed())
			parent := filepath.Join(base, "assets/.factory/bin")
			entries, readErr := os.ReadDir(parent)
			Expect(readErr).NotTo(HaveOccurred())
			if len(entries) == 0 {
				Expect(os.Remove(parent)).To(Succeed())
			}
		}
		expectedFiles := []string{"installation.manifest"}
		for _, path := range []string{filepath.Join(base, "installation.manifest"), filepath.Join(scenario.root, ".factory/installation-transactions", b+".json")} {
			data, readErr := os.ReadFile(path)
			Expect(readErr).NotTo(HaveOccurred())
			var document map[string]any
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.UseNumber()
			Expect(decoder.Decode(&document)).To(Succeed())
			Expect(decoder.Decode(new(any))).To(Equal(io.EOF))
			delete(document, "retention_receipt")
			rows, ok := document["entries"].([]any)
			Expect(ok).To(BeTrue())
			for _, value := range rows {
				row := value.(map[string]any)
				name := row["path"].(string)
				if attack == "retained-roles" && name != ".factory-version" {
					row["action"] = "retain"
				}
				if attack == "missing-runtime" && name == ".factory/bin/factory-runtime" {
					row["action"], row["before"] = "create", nil
				}
				if strings.HasSuffix(path, "installation.manifest") && (row["action"] == "replace" || row["action"] == "retire") {
					expectedFiles = append(expectedFiles, "assets/"+name)
				}
			}
			replacement, marshalErr := json.Marshal(document)
			Expect(marshalErr).NotTo(HaveOccurred())
			writeFixture(path, append(replacement, '\n'), 0600)
		}
		// Prove this is the supported old receiptless, closed file shape; a stale
		// sidecar or unexplained extra asset must not mask fresh-image qualification.
		_, err = os.Lstat(filepath.Join(base, "retention.json"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		actualFiles := []string{}
		Expect(filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			relative, relErr := filepath.Rel(base, path)
			if relErr != nil {
				return relErr
			}
			actualFiles = append(actualFiles, filepath.ToSlash(relative))
			return nil
		})).To(Succeed())
		Expect(actualFiles).To(ConsistOf(expectedFiles))
		before := installationSnapshot(scenario.root)
		result := retentionPruneResult(scenario, a)
		retentionRequireRefusal(result)
		Expect(result.stderr).To(MatchRegexp(`(?i)(usable|alternate|predecessor|complete|qualified|recovery)`))
		Expect(installationSnapshot(scenario.root)).To(Equal(before))
		Expect(retentionBackupIDs(scenario.root)).To(ConsistOf(a, b))
	}, Entry("forged retain labels with only the real B marker saved", "retained-roles"), Entry("one invented create row with B runtime omitted", "missing-runtime"))

})

// Syntax/control ownership is cheap and independent of authenticated fixture setup.
// per docs/adr/0106-go-backup-retention.md:68
var _ = Describe("Whole installation upgrade backup retention controls", func() {
	// per docs/adr/0106-go-backup-retention.md:68
	DescribeTable("refuses malformed and conflicting standalone operands before legacy dispatch or project writes", func(args []string) {
		root, cwd := fixture()
		writeFixture(filepath.Join(root, "scripts/factory-upgrade.sh"), []byte("#!/bin/sh\nprintf dispatched > legacy-dispatched\nexit 73\n"), 0755)
		before := installationSnapshot(cwd)
		result := invoke(root, cwd, "factory", "", append([]string{"upgrade"}, args...)...)
		Expect(result.status).To(Equal(2), "%+v", result)
		Expect(result.stderr).NotTo(BeEmpty())
		Expect(installationSnapshot(cwd)).To(Equal(before))
		_, err := os.Lstat(filepath.Join(cwd, "legacy-dispatched"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "these operands belong to the native control parser")
	},
		Entry("missing hold identifier", []string{"--keep-backup"}),
		Entry("missing prune identifier", []string{"--prune-backup"}),
		Entry("empty hold identifier", []string{"--keep-backup", ""}),
		Entry("empty prune identifier", []string{"--prune-backup="}),
		Entry("traversing hold identifier", []string{"--keep-backup", "../older"}),
		Entry("absolute prune identifier", []string{"--prune-backup", "/older"}),
		Entry("repeated hold", []string{"--keep-backup", "older", "--keep-backup", "other"}),
		Entry("repeated prune", []string{"--prune-backup", "older", "--prune-backup", "other"}),
		Entry("both standalone modes", []string{"--keep-backup", "older", "--prune-backup", "other"}),
		Entry("preview cannot combine", []string{"--keep-backup", "older", "--dry-run"}),
		Entry("JSON flag cannot combine", []string{"--prune-backup", "older", "--json"}),
		Entry("confirmation cannot combine", []string{"--prune-backup", "older", "--confirm", strings.Repeat("a", 64)}),
		Entry("candidate cannot combine", []string{"--go-candidate", handoffVersion, "--keep-backup", "older"}),
		Entry("installation cannot combine", []string{"--installation", "--prune-backup", "older"}),
		Entry("source cannot combine", []string{"--keep-backup", "older", "--source", "input.tar.gz"}),
		Entry("rollback cannot combine", []string{"--prune-backup", "older", "--rollback", "other"}),
		Entry("recovery cannot combine", []string{"--keep-backup", "older", "--recover", "other"}),
		Entry("quiescent cannot combine", []string{"--prune-backup", "older", "--quiescent"}),
		Entry("extra positional operand", []string{"--keep-backup", "older", "unexpected"}),
	)
})
