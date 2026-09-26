package acceptance_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const assessmentRevision = "c8f8d34edbc14df5655fcbf0aab7eea46ced0295"

var assessmentPaths = []string{"scripts/factory-budget.sh", "scripts/factory-loop.sh", "scripts/lib/budget-config.sh", "scripts/lib/budget.py", "scripts/lib/budget_adapters.py", "scripts/lib/loop.py"}

type assessedBytes struct {
	SHA256 string `json:"sha256"`
	Mode   string `json:"mode"`
	Bytes  int    `json:"bytes"`
}
type assessedRow struct {
	Path           string         `json:"path"`
	Classification string         `json:"classification"`
	Reference      assessedBytes  `json:"reference"`
	Observed       *assessedBytes `json:"observed"`
	Reason         string         `json:"reason"`
}
type assessedReport struct {
	SchemaVersion       int            `json:"schema_version"`
	ReferenceRevision   string         `json:"reference_revision"`
	Scope               string         `json:"scope"`
	OwnershipAuthorized bool           `json:"ownership_authorized"`
	ActivationReady     bool           `json:"activation_ready"`
	Assets              []assessedRow  `json:"assets"`
	Counts              map[string]int `json:"counts"`
}

func assessmentFixture() (string, string, []assessedBytes) {
	GinkgoHelper()
	root, project := fixture()
	references := make([]assessedBytes, 0, len(assessmentPaths))
	for _, path := range assessmentPaths {
		data, err := exec.Command("git", "show", assessmentRevision+":"+path).Output() // #nosec G204 -- immutable Git revision and six fixed reference paths.
		Expect(err).NotTo(HaveOccurred())
		tree, err := exec.Command("git", "ls-tree", "--full-tree", assessmentRevision, "--", path).Output() // #nosec G204 -- immutable Git mode oracle with fixed path allowlist.
		Expect(err).NotTo(HaveOccurred())
		fields := strings.Fields(string(tree))
		Expect(fields).To(HaveLen(4))
		bits, err := strconv.ParseUint(fields[0], 8, 32)
		Expect(err).NotTo(HaveOccurred())
		mode := os.FileMode(bits & 0777)
		writeFixture(filepath.Join(project, path), data, mode)
		Expect(os.Chmod(filepath.Join(project, path), mode)).To(Succeed())
		digest := sha256.Sum256(data)
		references = append(references, assessedBytes{hex.EncodeToString(digest[:]), "0" + strconv.FormatUint(bits&0777, 8), len(data)})
	}
	return root, project, references
}
func assessmentCommand(root, cwd string, args ...string) cliResult {
	return loopProcess(cwd, filepath.Join(root, "factory"), append([]string{"migration", "assess"}, args...), nil)
}
func assessmentDecode(out cliResult, project string) assessedReport {
	GinkgoHelper()
	Expect(out.stderr).To(BeEmpty())
	Expect(out.stdout).To(HaveSuffix("\n"))
	Expect(out.stdout).NotTo(ContainSubstring(project))
	decoder := json.NewDecoder(strings.NewReader(out.stdout))
	decoder.DisallowUnknownFields()
	var report assessedReport
	Expect(decoder.Decode(&report)).To(Succeed())
	var trailing any
	Expect(decoder.Decode(&trailing)).To(Equal(io.EOF))
	object := budgetDecode([]byte(out.stdout)).(map[string]any)
	Expect(object).To(HaveLen(7))
	Expect(object).To(HaveKeyWithValue("ownership_authorized", false))
	Expect(object).To(HaveKeyWithValue("activation_ready", false))
	for _, raw := range object["assets"].([]any) {
		row := raw.(map[string]any)
		Expect(row).To(HaveLen(5))
		Expect(row).To(HaveKey("observed"))
	}
	Expect(report.SchemaVersion).To(Equal(1))
	Expect(report.ReferenceRevision).To(Equal(assessmentRevision))
	Expect(report.Scope).To(Equal("g2-budget-loop-six"))
	Expect(report.OwnershipAuthorized).To(BeFalse())
	Expect(report.ActivationReady).To(BeFalse())
	Expect(report.Assets).To(HaveLen(6))
	Expect(report.Counts).To(HaveLen(5))
	for _, key := range []string{"matching_reference", "customized", "missing", "unsafe", "assessment_error"} {
		Expect(report.Counts).To(HaveKey(key))
	}
	for i, row := range report.Assets {
		Expect(row.Path).To(Equal(assessmentPaths[i]))
	}
	return report
}

var _ = Describe("G4 installation assessment core", func() {
	// per docs/adr/0079-go-installation-reference-assessment.md:47
	It("reports all immutable blobs and Git modes without authorizing ownership", func() {
		root, project, references := assessmentFixture()
		out := assessmentCommand(root, project, project)
		Expect(out.status).To(BeZero(), "%+v", out)
		report := assessmentDecode(out, project)
		Expect(report.Counts).To(Equal(map[string]int{"matching_reference": 6, "customized": 0, "missing": 0, "unsafe": 0, "assessment_error": 0}))
		for i, row := range report.Assets {
			Expect(row.Reference).To(Equal(references[i]))
			Expect(row.Observed).To(Equal(&references[i]))
			Expect(row.Classification).To(Equal("matching_reference"))
			Expect(row.Reason).To(Equal("content_type_mode_match"))
		}
		Expect(assessmentCommand(root, project, project)).To(Equal(out))
	})
	// per docs/adr/0079-go-installation-reference-assessment.md:63
	It("classifies all absent leaves without creating installation state", func() {
		root, project := fixture()
		out := assessmentCommand(root, project, project)
		Expect(out.status).To(Equal(2), "%+v", out)
		report := assessmentDecode(out, project)
		Expect(report.Counts["missing"]).To(Equal(6))
		for _, row := range report.Assets {
			Expect(row.Reason).To(Equal("path_absent"))
			Expect(row.Observed).To(BeNil())
		}
		entries, err := os.ReadDir(project)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	})
	// per docs/adr/0079-go-installation-reference-assessment.md:60
	It("reports modified content separately from the embedded reference", func() {
		root, project, references := assessmentFixture()
		path := filepath.Join(project, assessmentPaths[0])
		writeFixture(path, []byte("PRIVATE_CUSTOM_CONTENT\n"), 0700)
		out := assessmentCommand(root, project, project)
		Expect(out.status).To(Equal(2))
		report := assessmentDecode(out, project)
		Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_CUSTOM_CONTENT"))
		Expect(report.Assets[0].Reference).To(Equal(references[0]))
		Expect(report.Assets[0].Classification).To(Equal("customized"))
		Expect(report.Assets[0].Reason).To(Equal("content_or_mode_differs"))
		Expect(report.Assets[0].Observed.Bytes).To(Equal(len("PRIVATE_CUSTOM_CONTENT\n")))
		Expect(report.Counts["matching_reference"]).To(Equal(5))
	})
})

var _ = Describe("G4 installation assessment classification", func() {
	// per docs/adr/0079-go-installation-reference-assessment.md:60
	DescribeTable("separates bounded content and mode customization from unsafe filesystem objects", func(kind, classification string, count int) {
		root, project, _ := assessmentFixture()
		path := filepath.Join(project, assessmentPaths[0])
		outside := filepath.Join(root, "outside")
		switch kind {
		case "permissions":
			Expect(os.Chmod(path, 0600)).To(Succeed())
		case "special mode", "setgid", "sticky":
			bits := map[string]os.FileMode{"special mode": os.ModeSetuid, "setgid": os.ModeSetgid, "sticky": os.ModeSticky}[kind]
			Expect(os.Chmod(path, 0755|bits)).To(Succeed())
		case "exact limit":
			writeFixture(path, []byte(strings.Repeat("x", 1<<20)), 0600)
		case "oversized":
			writeFixture(path, []byte(strings.Repeat("x", (1<<20)+1)), 0600)
		case "missing leaf":
			Expect(os.Remove(path)).To(Succeed())
		case "missing ancestor":
			Expect(os.RemoveAll(filepath.Join(project, "scripts/lib"))).To(Succeed())
		case "leaf symlink":
			Expect(os.Rename(path, outside)).To(Succeed())
			Expect(os.Symlink(outside, path)).To(Succeed())
		case "dangling symlink":
			Expect(os.Remove(path)).To(Succeed())
			Expect(os.Symlink(outside, path)).To(Succeed())
		case "hardlink":
			Expect(os.Link(path, outside)).To(Succeed())
		case "directory":
			Expect(os.Remove(path)).To(Succeed())
			Expect(os.Mkdir(path, 0700)).To(Succeed())
		case "FIFO":
			Expect(os.Remove(path)).To(Succeed())
			Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
		case "ancestor symlink":
			Expect(os.Rename(filepath.Join(project, "scripts"), outside)).To(Succeed())
			Expect(os.Symlink(outside, filepath.Join(project, "scripts"))).To(Succeed())
		case "ancestor file":
			Expect(os.RemoveAll(filepath.Join(project, "scripts"))).To(Succeed())
			writeFixture(filepath.Join(project, "scripts"), []byte("PRIVATE_ANCESTOR"), 0600)
		}
		out := assessmentCommand(root, project, project)
		Expect(out.status).To(Equal(2), "%+v", out)
		report := assessmentDecode(out, project)
		Expect(report.Counts[classification]).To(Equal(count))
		Expect(report.Counts["matching_reference"]).To(Equal(6 - count))
		reason := map[string]string{"customized": "content_or_mode_differs", "missing": "path_absent", "unsafe": "unsafe_path_or_file"}[classification]
		for _, row := range report.Assets {
			if row.Classification == classification {
				Expect(row.Reason).To(Equal(reason))
				if classification == "customized" {
					Expect(row.Observed).NotTo(BeNil())
					if kind == "special mode" || kind == "setgid" || kind == "sticky" {
						Expect(row.Observed.Mode).To(Equal(row.Reference.Mode))
					}
				} else {
					Expect(row.Observed).To(BeNil())
				}
			}
		}
	}, Entry("permissions", "permissions", "customized", 1), Entry("special bits", "special mode", "customized", 1), Entry("setgid bit", "setgid", "customized", 1), Entry("sticky bit", "sticky", "customized", 1), Entry("exactly1MiB", "exact limit", "customized", 1), Entry("over1MiB", "oversized", "unsafe", 1),
		Entry("missing leaf", "missing leaf", "missing", 1), Entry("missing ancestor", "missing ancestor", "missing", 4), Entry("leaf symlink", "leaf symlink", "unsafe", 1), Entry("dangling symlink", "dangling symlink", "unsafe", 1), Entry("multiple links", "hardlink", "unsafe", 1), Entry("directory leaf", "directory", "unsafe", 1), Entry("FIFO leaf", "FIFO", "unsafe", 1), Entry("ancestor symlink", "ancestor symlink", "unsafe", 6), Entry("non-directory ancestor", "ancestor file", "unsafe", 6))
	// per docs/adr/0079-go-installation-reference-assessment.md:68
	It("gives read errors precedence over missing and customized observations", func() {
		if os.Geteuid() == 0 {
			Skip("requires unprivileged filesystem read checks")
		}
		root, project, _ := assessmentFixture()
		unreadable := filepath.Join(project, assessmentPaths[0])
		Expect(os.Chmod(unreadable, 0000)).To(Succeed())
		DeferCleanup(func() { Expect(os.Chmod(unreadable, 0600)).To(Succeed()) })
		Expect(os.Remove(filepath.Join(project, assessmentPaths[1]))).To(Succeed())
		writeFixture(filepath.Join(project, assessmentPaths[2]), []byte("PRIVATE_CUSTOM"), 0600)
		out := assessmentCommand(root, project, project)
		Expect(out.status).To(Equal(1))
		report := assessmentDecode(out, project)
		Expect(report.Counts).To(Equal(map[string]int{"matching_reference": 3, "customized": 1, "missing": 1, "unsafe": 0, "assessment_error": 1}))
		Expect(report.Assets[0].Classification).To(Equal("assessment_error"))
		Expect(report.Assets[0].Observed).To(BeNil())
		Expect(report.Assets[0].Reason).To(Equal("cannot_read_asset"))
	})
})

var _ = Describe("G4 installation assessment root and request", func() {
	// per docs/adr/0079-go-installation-reference-assessment.md:81
	DescribeTable("accepts physical relative dot and trailing-slash roots", func(form string) {
		root, project, _ := assessmentFixture()
		operand := project
		cwd := project
		switch form {
		case "dot":
			operand = "."
		case "relative":
			cwd = filepath.Dir(project)
			operand = filepath.Base(project)
		case "trailing":
			operand = project + "/"
		case "embedded dot":
			operand = project + "/./"
		case "dash name":
			moved := filepath.Join(filepath.Dir(project), "--help")
			Expect(os.Rename(project, moved)).To(Succeed())
			project = moved
			cwd = filepath.Dir(project)
			operand = "--help"
		}
		out := assessmentCommand(root, cwd, operand)
		Expect(out.status).To(BeZero(), "%+v", out)
		assessmentDecode(out, project)
	}, Entry("dot", "dot"), Entry("relative", "relative"), Entry("trailing slash", "trailing"), Entry("ordinary dot component", "embedded dot"), Entry("literal flag-looking directory", "dash name"))
	// per docs/adr/0079-go-installation-reference-assessment.md:74
	DescribeTable("refuses invalid and unsafe roots without JSON or project-path disclosure", func(kind string, status int) {
		root, project, _ := assessmentFixture()
		operand := project
		switch kind {
		case "empty":
			operand = ""
		case "parent":
			operand = project + "/../" + filepath.Base(project)
		case "missing":
			operand = filepath.Join(project, "PRIVATE_MISSING")
		case "file":
			operand = filepath.Join(project, assessmentPaths[0])
		case "root symlink":
			operand = filepath.Join(root, "PRIVATE_SYMLINK")
			Expect(os.Symlink(project, operand)).To(Succeed())
		case "ancestor symlink":
			alias := filepath.Join(root, "PRIVATE_ALIAS")
			Expect(os.Symlink(filepath.Dir(project), alias)).To(Succeed())
			operand = filepath.Join(alias, filepath.Base(project))
		case "unreadable":
			if os.Geteuid() == 0 {
				Skip("requires unprivileged directory traversal")
			}
			Expect(os.Chmod(project, 0000)).To(Succeed())
			DeferCleanup(func() { Expect(os.Chmod(project, 0700)).To(Succeed()) })
		}
		out := assessmentCommand(root, root, operand)
		Expect(out.status).To(Equal(status), "%+v", out)
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).NotTo(BeEmpty())
		Expect(out.stderr).NotTo(ContainSubstring("PRIVATE_"))
		Expect(out.stderr).NotTo(ContainSubstring(project))
	}, Entry("empty", "empty", 2), Entry("parent traversal", "parent", 2), Entry("missing", "missing", 1), Entry("regular root", "file", 2), Entry("root symlink", "root symlink", 2), Entry("ancestor symlink", "ancestor symlink", 2), Entry("unreadable", "unreadable", 1))
	// per docs/adr/0079-go-installation-reference-assessment.md:22
	DescribeTable("accepts exactly one literal operand and no Cobra help or completion flags", func(args []string) {
		root, project := fixture()
		out := assessmentCommand(root, project, args...)
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).NotTo(BeEmpty())
		entries, err := os.ReadDir(project)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	}, Entry("missing", []string{}), Entry("extra", []string{".", "extra"}), Entry("completion", []string{"__complete", "x"}))
})

type assessedFileState struct {
	Mode     os.FileMode
	Data     string
	Modified int64
}

func assessmentTree(root string) map[string]assessedFileState {
	GinkgoHelper()
	result := map[string]assessedFileState{}
	Expect(filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		state := assessedFileState{Mode: info.Mode(), Modified: info.ModTime().UnixNano()}
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path) // #nosec G122 -- this snapshot traverses only a stable test-owned fixture before/after assessment, with no concurrent mutation.
			if err != nil {
				return err
			}
			state.Data = hex.EncodeToString(data)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			state.Data = target
		}
		result[relative] = state
		return nil
	})).To(Succeed())
	return result
}

var _ = Describe("G4 installation assessment read-only authority", func() {
	// per docs/adr/0079-go-installation-reference-assessment.md:41
	It("ignores forged ownership and extra executable files while leaving the whole fixture unchanged", func() {
		root, project, _ := assessmentFixture()
		marker := filepath.Join(root, "PROBE_CALLED")
		bin := filepath.Join(root, "poison")
		for _, name := range []string{"git", "python3", "bash", "codex", "claude", "opencode"} {
			writeFixture(filepath.Join(bin, name), []byte("#!/bin/sh\nprintf called > '"+marker+"'\nexit 99\n"), 0700)
		}
		for _, path := range []string{".factory-version", ".factory/ownership.json", "docs/migration/baseline-assets.json"} {
			writeFixture(filepath.Join(project, path), []byte(`{"ownership_authorized":true,"activation_ready":true}`), 0600)
		}
		writeFixture(filepath.Join(project, "unknown/execute-me"), []byte("#!/bin/sh\nprintf called > '"+marker+"'\n"), 0700)
		Expect(syscall.Mkfifo(filepath.Join(project, "unknown/ignored-fifo"), 0600)).To(Succeed())
		before := assessmentTree(project)
		out := loopProcess(project, filepath.Join(root, "factory"), []string{"migration", "assess", project}, []string{"PATH=" + bin, "PRIVATE_CREDENTIAL=never-print-this"})
		Expect(out.status).To(BeZero(), "%+v", out)
		assessmentDecode(out, project)
		Expect(out.stdout + out.stderr).NotTo(ContainSubstring("never-print-this"))
		Expect(assessmentTree(project)).To(Equal(before))
		publicNoPoison(marker)
	})
})
