package acceptance_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const durableRecoveryID = "reviewed-migration-1"

func durableGitEnvironment() []string {
	var environment []string
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if !strings.HasPrefix(key, "GIT_") && key != "FACTORY_BRIDGE_PROTOCOL" {
			environment = append(environment, value)
		}
	}
	return append(environment, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GORACE=atexit_sleep_ms=0")
}

func durableGit(root string, args ...string) string {
	GinkgoHelper()
	command := exec.Command("git", args...) // #nosec G204 -- literal Git operations on a fixture-owned repository.
	command.Dir, command.Env = root, durableGitEnvironment()
	data, err := command.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "%s", data)
	return string(data)
}

func durableRecoveryFixture() (string, string, []assessedBytes) {
	GinkgoHelper()
	binary, root, references := assessmentFixture()
	durableGit(root, "init", "-q")
	durableGit(root, "add", "--", "scripts")
	writeFixture(filepath.Join(root, ".git/info/exclude"), []byte("# existing local exclusions\nkeep-local\n"), 0600)
	// Git creates these paths before WriteFile; qualify their actual modes so
	// the happy path does not inherit writable template/umask control metadata.
	// per docs/adr/0091-durable-local-recovery-creation.md:76
	// per docs/adr/0091-durable-local-recovery-creation.md:200
	for _, relative := range []string{".git", ".git/info", ".git/info/exclude", ".git/config", ".git/HEAD", ".git/index"} {
		mode := os.FileMode(0600)
		if relative == ".git" || relative == ".git/info" {
			mode = 0700
		}
		path := filepath.Join(root, relative)
		Expect(os.Chmod(path, mode)).To(Succeed())
		info, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(mode), "positive fixture must retain trusted Git metadata")
	}
	writeFixture(filepath.Join(root, ".gitignore"), []byte("# project rules preserved\n"), 0600)
	writeFixture(filepath.Join(root, "factory.yaml"), []byte("PRIVATE_CONFIGURATION: keep\n"), 0600)
	writeFixture(filepath.Join(root, ".factory/events.log"), []byte("PRIVATE_RUNTIME_HISTORY\n"), 0600)
	return binary, root, references
}

func durableRecoveryArgs(binary, root, id, target string, selection []string) []string {
	GinkgoHelper()
	proposal := adoptionDecode(adoptionCommand(binary, root, "propose-adoption", append([]string{root}, selection...)...), root)
	args := []string{"--create-backup", "--migration-id", id, "--target-revision", target}
	for _, path := range selection {
		args = append(args, "--adopt-path", path)
	}
	return append(args, "--confirm-adoption", proposal.ProposalDigest, "--json")
}

func durableRecoveryRun(binary, root string, args []string, environment ...string) cliResult {
	return publicCommand(binary, root, "upgrade", args, environment)
}

func durableRecoveryObject(out cliResult, root, id, target, result string, count, bytes int) map[string]any {
	GinkgoHelper()
	Expect(out.status).To(BeZero(), "%+v", out)
	Expect(out.stderr).To(BeEmpty())
	Expect(out.stdout).To(HaveSuffix("\n"))
	Expect(out.stdout).NotTo(ContainSubstring(root))
	Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_"))
	object := budgetDecode([]byte(out.stdout)).(map[string]any)
	Expect(object).To(HaveLen(15))
	for key, value := range map[string]any{
		"schema_version": json.Number("1"), "mode": "create_backup", "coverage": "partial", "scope": "g2-budget-loop-six",
		"migration_id": id, "path": ".factory/backups/" + id, "result": result,
		"file_count": json.Number(strconv.Itoa(count)), "bytes": json.Number(strconv.Itoa(bytes)),
		"source_revision": assessmentRevision, "target_revision": target, "target_authentication": "operator_metadata",
		"restorable": false, "activation_ready": false, "prune_authorized": false,
	} {
		Expect(object).To(HaveKeyWithValue(key, value))
	}
	return object
}

func durableBytes(root, path string) []byte {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join(root, path))
	Expect(err).NotTo(HaveOccurred())
	return data
}

var _ = Describe("Durable local recovery creation core", func() {
	// per docs/adr/0091-durable-local-recovery-creation.md:50
	It("returns all bounded metadata fields in deterministic text", func() {
		binary, root, references := durableRecoveryFixture()
		target := strings.Repeat("a", 40)
		args := durableRecoveryArgs(binary, root, durableRecoveryID, target, assessmentPaths[:1])
		args = args[:len(args)-1]
		out := durableRecoveryRun(binary, root, args)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stderr).To(BeEmpty())
		expected := strings.Join([]string{"schema_version=1", "mode=create_backup", "coverage=partial", "scope=g2-budget-loop-six", "migration_id=" + durableRecoveryID, "path=.factory/backups/" + durableRecoveryID, "result=created", "file_count=1", "bytes=" + strconv.Itoa(references[0].Bytes), "source_revision=" + assessmentRevision, "target_revision=" + target, "target_authentication=operator_metadata", "restorable=false", "activation_ready=false", "prune_authorized=false", ""}, "\n")
		Expect(out.stdout).To(Equal(expected))
		repeated := durableRecoveryRun(binary, root, args)
		Expect(repeated.status).To(BeZero())
		Expect(repeated.stderr).To(BeEmpty())
		Expect(repeated.stdout).To(Equal(strings.Replace(expected, "result=created\n", "result=already_present\n", 1)))
	})

	// per docs/adr/0091-durable-local-recovery-creation.md:104
	// per docs/adr/0091-durable-local-recovery-creation.md:50
	DescribeTable("creates an inert ignored recovery set while preserving active files and Git index", func(full bool) {
		binary, root, references := durableRecoveryFixture()
		selection := assessmentPaths[:1]
		if full {
			selection = assessmentPaths
		}
		target := strings.Repeat("a", 40)
		args := durableRecoveryArgs(binary, root, durableRecoveryID, target, selection)
		before := map[string][]byte{}
		for _, path := range append(append([]string(nil), assessmentPaths...), "factory.yaml", ".factory/events.log", ".git/index", ".gitignore") {
			before[path] = durableBytes(root, path)
		}
		exclude := durableBytes(root, ".git/info/exclude")
		bytes := 0
		for i := range selection {
			bytes += references[i].Bytes
		}
		out := durableRecoveryRun(binary, root, args)
		durableRecoveryObject(out, root, durableRecoveryID, target, "created", len(selection), bytes)
		for path, data := range before {
			Expect(durableBytes(root, path)).To(Equal(data), "%s active bytes changed", path)
		}
		afterExclude := durableBytes(root, ".git/info/exclude")
		Expect(string(afterExclude)).To(HavePrefix(string(exclude)))
		Expect(strings.Count(string(afterExclude), "/.factory/backups/")).To(Equal(1))
		set := filepath.Join(root, ".factory/backups", durableRecoveryID)
		Expect(filepath.WalkDir(filepath.Join(root, ".factory/backups"), func(_ string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if entry.IsDir() {
				Expect(info.Mode().Perm()).To(Equal(os.FileMode(0700)), "%s", entry.Name())
			} else {
				Expect(info.Mode().IsRegular()).To(BeTrue())
				Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)), "%s", entry.Name())
			}
			Expect(info.Mode() & (os.ModeSetuid | os.ModeSetgid | os.ModeSticky)).To(BeZero())
			return nil
		})).To(Succeed())
		for _, path := range selection {
			Expect(durableBytes(set, "files/"+path)).To(Equal(before[path]))
			durableGit(root, "check-ignore", "-q", "--", ".factory/backups/"+durableRecoveryID+"/files/"+path)
		}
		durableGit(root, "check-ignore", "-q", "--", ".factory/backups/")
		durableGit(root, "check-ignore", "-q", "--", ".factory/backups/"+durableRecoveryID+"/")
		durableGit(root, "check-ignore", "-q", "--", ".factory/backups/"+durableRecoveryID+"/manifest.json")
		Expect(durableGit(root, "ls-files", "--", ".factory/backups")).To(BeEmpty())
		Expect(durableGit(root, "status", "--porcelain", "--untracked-files=all")).NotTo(ContainSubstring(".factory/backups"))
		inventory := recoveryDecode(recoveryCommand(binary, root), root)
		Expect(inventory["complete"]).To(BeTrue())
		Expect(inventory["sets"].([]any)).To(HaveLen(1))
		row := inventory["sets"].([]any)[0].(map[string]any)
		Expect(row["classification"]).To(Equal("integrity_checked"))
		Expect(row["file_count"]).To(Equal(json.Number(strconv.Itoa(len(selection)))))
		manifest := recoveryManifest(set)
		Expect(manifest).To(HaveLen(7))
		Expect(manifest).To(HaveKeyWithValue("migration_id", durableRecoveryID))
		Expect(manifest).To(HaveKeyWithValue("source_revision", assessmentRevision))
		Expect(manifest).To(HaveKeyWithValue("target_revision", target))
		Expect(manifest).To(HaveKeyWithValue("held", false))
		assets := manifest["assets"].([]any)
		Expect(assets).To(HaveLen(len(selection)))
		for index, value := range assets {
			asset := value.(map[string]any)
			Expect(asset).To(HaveLen(4))
			Expect(asset).To(HaveKeyWithValue("path", selection[index]))
			Expect(asset).To(HaveKeyWithValue("mode", references[index].Mode))
			Expect(asset).To(HaveKeyWithValue("bytes", json.Number(strconv.Itoa(references[index].Bytes))))
			Expect(asset).To(HaveKeyWithValue("sha256", references[index].SHA256))
		}
	}, Entry("one selected reference", false), Entry("all six selected references", true))

	// per docs/adr/0091-durable-local-recovery-creation.md:124
	It("reuses the same exact completed selection without duplicate exclusions or recovery sets", func() {
		binary, root, references := durableRecoveryFixture()
		target := strings.Repeat("a", 40)
		args := durableRecoveryArgs(binary, root, durableRecoveryID, target, assessmentPaths[:1])
		durableRecoveryObject(durableRecoveryRun(binary, root, args), root, durableRecoveryID, target, "created", 1, references[0].Bytes)
		before := assessmentTree(filepath.Join(root, ".factory/backups"))
		exclude := durableBytes(root, ".git/info/exclude")
		durableRecoveryObject(durableRecoveryRun(binary, root, args), root, durableRecoveryID, target, "already_present", 1, references[0].Bytes)
		Expect(assessmentTree(filepath.Join(root, ".factory/backups"))).To(Equal(before))
		Expect(durableBytes(root, ".git/info/exclude")).To(Equal(exclude))
	})
})
