package acceptance_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func recoverySet(root, name string, references []assessedBytes, held bool) string {
	GinkgoHelper()
	set := filepath.Join(root, ".factory/backups", name)
	assets := make([]map[string]any, 0, len(references))
	for i, reference := range references {
		path := assessmentPaths[i]
		assets = append(assets, map[string]any{"path": path, "sha256": reference.SHA256, "bytes": reference.Bytes, "mode": reference.Mode})
		data, err := os.ReadFile(filepath.Join(root, path))
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(set, "files", path), data, 0600)
	}
	manifest := map[string]any{"schema_version": 1, "migration_id": name, "source_revision": assessmentRevision, "target_revision": strings.Repeat("a", 40), "scope": "g2-budget-loop-six", "held": held, "assets": assets}
	data, err := json.Marshal(manifest)
	Expect(err).NotTo(HaveOccurred())
	writeFixture(filepath.Join(set, "manifest.json"), data, 0600)
	Expect(filepath.WalkDir(filepath.Join(root, ".factory"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return os.Chmod(path, 0700) // #nosec G122 -- fixture-owned tree contains only freshly created directories and regular files; no concurrent writers or symlinks during setup.
		}
		return nil
	})).To(Succeed())
	return set
}
func recoveryCommand(binary, root string, args ...string) cliResult {
	return upgradePreview(binary, root, append([]string{"--dry-run", "--source=.", "--inspect-backups", "--json"}, args...)...)
}
func recoveryDecode(out cliResult, root string) map[string]any {
	GinkgoHelper()
	Expect(out.status).To(Equal(2), "%+v", out)
	Expect(out.stderr).To(BeEmpty())
	Expect(out.stdout).To(HaveSuffix("\n"))
	Expect(out.stdout).NotTo(ContainSubstring(root))
	object := budgetDecode([]byte(out.stdout)).(map[string]any)
	Expect(object).To(HaveLen(9))
	Expect(object["recovery_assessment"]).To(Equal("inspected"))
	plan := object["plan"].(map[string]any)
	for _, key := range []string{"rollback_ready", "applicable", "ownership_authorized"} {
		Expect(plan).To(HaveKeyWithValue(key, false))
	}
	inventory := object["recovery_inventory"].(map[string]any)
	Expect(inventory).To(HaveLen(9))
	Expect(inventory["schema_version"]).To(Equal(json.Number("1")))
	for _, key := range []string{"restorable", "prune_authorized"} {
		Expect(inventory).To(HaveKeyWithValue(key, false))
	}
	Expect(inventory).To(HaveKey("sets"))
	Expect(inventory["sets"]).NotTo(BeNil())
	for _, entry := range inventory["sets"].([]any) {
		row := entry.(map[string]any)
		Expect(row).To(HaveLen(9))
		for _, key := range []string{"restorable", "prune_authorized"} {
			Expect(row).To(HaveKeyWithValue(key, false))
		}
		Expect(row).To(HaveKey("held"))
	}
	return inventory
}

var _ = Describe("G4 recovery inventory core", func() {
	// per docs/adr/0083-go-recovery-set-inspection.md:69
	It("reports absent recovery without creating directories", func() {
		binary, root, _ := assessmentFixture()
		before := assessmentTree(root)
		inventory := recoveryDecode(recoveryCommand(binary, root), root)
		Expect(inventory["root_status"]).To(Equal("absent"))
		Expect(inventory["complete"]).To(BeTrue())
		Expect(inventory["sets"]).To(BeEmpty())
		for _, key := range []string{"set_count", "file_count", "bytes"} {
			Expect(inventory[key]).To(Equal(json.Number("0")))
		}
		Expect(assessmentTree(root)).To(Equal(before))
	})
	// per docs/adr/0083-go-recovery-set-inspection.md:79
	It("checks inert originals but grants neither restore nor prune authority", func() {
		binary, root, references := assessmentFixture()
		recoverySet(root, "migration-1", references[:1], false)
		before := assessmentTree(root)
		inventory := recoveryDecode(recoveryCommand(binary, root), root)
		Expect(inventory["root_status"]).To(Equal("inspected"))
		Expect(inventory["complete"]).To(BeTrue())
		row := inventory["sets"].([]any)[0].(map[string]any)
		Expect(row["classification"]).To(Equal("integrity_checked"))
		Expect(row["reason"]).To(Equal("transaction_authority_not_established"))
		Expect(row["next_action"]).To(Equal("review_transaction_evidence"))
		Expect(row["held"]).To(BeFalse())
		Expect(row["file_count"]).To(Equal(json.Number("1")))
		Expect(inventory["file_count"]).To(Equal(json.Number("1")))
		Expect(assessmentTree(root)).To(Equal(before))
	})
	// per docs/adr/0083-go-recovery-set-inspection.md:88
	It("preserves an incomplete set without counting its missing originals", func() {
		binary, root, references := assessmentFixture()
		set := recoverySet(root, "migration-2", references[:1], true)
		Expect(os.Remove(filepath.Join(set, "files", assessmentPaths[0]))).To(Succeed())
		before := assessmentTree(root)
		inventory := recoveryDecode(recoveryCommand(binary, root), root)
		row := inventory["sets"].([]any)[0].(map[string]any)
		Expect(row["classification"]).To(Equal("incomplete"))
		Expect(row["held"]).To(BeTrue())
		Expect(row["next_action"]).To(Equal("inspect_preserved_set"))
		Expect(inventory["file_count"]).To(Equal(json.Number("0")))
		Expect(inventory["bytes"]).To(Equal(json.Number("0")))
		Expect(assessmentTree(root)).To(Equal(before))
	})
})

func recoveryManifest(set string) map[string]any {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join(set, "manifest.json"))
	Expect(err).NotTo(HaveOccurred())
	return budgetDecode(data).(map[string]any)
}
func recoveryWriteManifest(set string, manifest map[string]any) {
	GinkgoHelper()
	data, err := json.Marshal(manifest)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.WriteFile(filepath.Join(set, "manifest.json"), data, 0600)).To(Succeed())
}
func recoveryOnlyRow(inventory map[string]any, classification string) map[string]any {
	GinkgoHelper()
	Expect(inventory["root_status"]).To(Equal("inspected"))
	Expect(inventory["complete"]).To(BeTrue())
	Expect(inventory["sets"]).To(HaveLen(1))
	Expect(inventory["set_count"]).To(Equal(json.Number("1")))
	row := inventory["sets"].([]any)[0].(map[string]any)
	Expect(row["classification"]).To(Equal(classification))
	reasons := map[string]string{"unrecognized": "unrecognized_recovery_record", "incomplete": "recovery_content_incomplete", "unsafe": "unsafe_recovery_path", "assessment_error": "cannot_inspect_recovery", "limit_exceeded": "recovery_limit_exceeded", "integrity_checked": "transaction_authority_not_established"}
	actions := map[string]string{"unrecognized": "inspect_preserved_set", "incomplete": "inspect_preserved_set", "unsafe": "resolve_unsafe_path", "assessment_error": "retry_inspection", "limit_exceeded": "reduce_inventory_scope", "integrity_checked": "review_transaction_evidence"}
	Expect(row["reason"]).To(Equal(reasons[classification]))
	Expect(row["next_action"]).To(Equal(actions[classification]))
	if classification != "integrity_checked" {
		for _, key := range []string{"file_count", "bytes"} {
			Expect(row[key]).To(Equal(json.Number("0")))
			Expect(inventory[key]).To(Equal(json.Number("0")))
		}
	}
	return row
}

var _ = Describe("G4 recovery inventory manifest admission", func() {
	// per docs/adr/0083-go-recovery-set-inspection.md:32
	DescribeTable("preserves malformed records without trusting held or catalog claims", func(change string) {
		binary, root, references := assessmentFixture()
		set := recoverySet(root, "set-1", references[:1], true)
		manifest := recoveryManifest(set)
		asset := manifest["assets"].([]any)[0].(map[string]any)
		raw := ""
		switch change {
		case "missing":
			delete(manifest, "scope")
		case "unknown":
			manifest["PRIVATE_UNKNOWN"] = true
		case "null":
			manifest["held"] = nil
		case "held string":
			manifest["held"] = "true"
		case "float version":
			manifest["schema_version"] = json.Number("1.0")
		case "bool version":
			manifest["schema_version"] = true
		case "wrong version":
			manifest["schema_version"] = 2
		case "different id":
			manifest["migration_id"] = "other"
		case "wrong source":
			manifest["source_revision"] = strings.Repeat("b", 40)
		case "same revision":
			manifest["target_revision"] = assessmentRevision
		case "uppercase revision":
			manifest["target_revision"] = strings.Repeat("A", 40)
		case "short revision":
			manifest["target_revision"] = "abcd"
		case "wrong scope":
			manifest["scope"] = "full-installation"
		case "empty assets":
			manifest["assets"] = []any{}
		case "null assets":
			manifest["assets"] = nil
		case "duplicate assets":
			manifest["assets"] = []any{asset, asset}
		case "seven assets":
			manifest["assets"] = []any{asset, asset, asset, asset, asset, asset, asset}
		case "asset unknown":
			asset["PRIVATE_EXTRA"] = true
		case "asset missing":
			delete(asset, "bytes")
		case "asset null":
			asset["mode"] = nil
		case "wrong path":
			asset["path"] = "../../PRIVATE_OUTSIDE"
		case "alias path":
			asset["path"] = "./" + assessmentPaths[0]
		case "wrong hash":
			asset["sha256"] = strings.Repeat("0", 64)
		case "wrong size":
			asset["bytes"] = json.Number("0")
		case "float size":
			asset["bytes"] = json.Number(fmt.Sprintf("%d.0", references[0].Bytes))
		case "wrong mode":
			asset["mode"] = "0600"
		case "unsorted":
			recoverySet(root, "set-1", references[:2], true)
			manifest = recoveryManifest(set)
			assets := manifest["assets"].([]any)
			assets[0], assets[1] = assets[1], assets[0]
		case "duplicate top":
			raw = `{"held":false,`
		case "duplicate nested":
			raw = "nested"
		case "trailing":
			raw = "trailing"
		case "invalid utf8":
			raw = string([]byte{0xff})
		case "syntax":
			raw = "{"
		}
		recoveryWriteManifest(set, manifest)
		path := filepath.Join(set, "manifest.json")
		if raw != "" {
			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			switch change {
			case "duplicate top":
				raw += string(data[1:])
			case "duplicate nested":
				raw = strings.Replace(string(data), `"path":`, `"path":"PRIVATE_DUPLICATE","path":`, 1)
			case "trailing":
				raw = string(data) + "{}"
			}
			Expect(os.WriteFile(path, []byte(raw), 0600)).To(Succeed())
		}
		before := assessmentTree(root)
		row := recoveryOnlyRow(recoveryDecode(recoveryCommand(binary, root), root), "unrecognized")
		Expect(row["held"]).To(BeNil())
		Expect(assessmentTree(root)).To(Equal(before))
	},
		Entry("missing field", "missing"), Entry("unknown field", "unknown"), Entry("null field", "null"), Entry("wrong held type", "held string"),
		Entry("float version", "float version"), Entry("boolean version", "bool version"), Entry("wrong version", "wrong version"), Entry("id mismatch", "different id"),
		Entry("wrong source revision", "wrong source"), Entry("identical target revision", "same revision"), Entry("uppercase revision", "uppercase revision"), Entry("short revision", "short revision"), Entry("wrong scope", "wrong scope"),
		Entry("empty assets", "empty assets"), Entry("null assets", "null assets"), Entry("duplicate assets", "duplicate assets"), Entry("too many assets", "seven assets"),
		Entry("nested unknown field", "asset unknown"), Entry("nested missing field", "asset missing"), Entry("nested null field", "asset null"), Entry("path traversal", "wrong path"), Entry("path alias", "alias path"),
		Entry("wrong hash", "wrong hash"), Entry("wrong size", "wrong size"), Entry("float size", "float size"), Entry("wrong mode", "wrong mode"), Entry("unsorted assets", "unsorted"),
		Entry("duplicate top key", "duplicate top"), Entry("duplicate nested key", "duplicate nested"), Entry("trailing JSON", "trailing"), Entry("invalid UTF8", "invalid utf8"), Entry("syntax", "syntax"))
})
var _ = Describe("G4 recovery inventory inert filesystem", func() {
	// per docs/adr/0083-go-recovery-set-inspection.md:51
	DescribeTable("preserves unsafe or incomplete sets without reading unknown payloads", func(change, classification string) {
		binary, root, references := assessmentFixture()
		set := recoverySet(root, "set-1", references[:1], true)
		leaf := filepath.Join(set, "files", assessmentPaths[0])
		manifest := filepath.Join(set, "manifest.json")
		switch change {
		case "missing manifest":
			Expect(os.Remove(manifest)).To(Succeed())
		case "missing copy":
			Expect(os.Remove(leaf)).To(Succeed())
		case "custom copy":
			Expect(os.WriteFile(leaf, []byte("PRIVATE_SAVED_CUSTOM"), 0600)).To(Succeed())
		case "extra file":
			writeFixture(filepath.Join(set, "PRIVATE_EXTRA"), []byte("PRIVATE_EXTRA_CONTENT"), 0600)
		case "extra dir":
			Expect(os.Mkdir(filepath.Join(set, "extra"), 0700)).To(Succeed())
		case "extra subtree":
			Expect(os.Mkdir(filepath.Join(set, "extra"), 0700)).To(Succeed())
			Expect(syscall.Mkfifo(filepath.Join(set, "extra", "PRIVATE_NESTED_FIFO"), 0600)).To(Succeed())
		case "extra fifo":
			Expect(syscall.Mkfifo(filepath.Join(set, "PRIVATE_FIFO"), 0600)).To(Succeed())
		case "extra symlink":
			Expect(os.Symlink("/PRIVATE_OUTSIDE", filepath.Join(set, "PRIVATE_LINK"))).To(Succeed())
		case "manifest symlink":
			Expect(os.Remove(manifest)).To(Succeed())
			Expect(os.Symlink("/PRIVATE_OUTSIDE", manifest)).To(Succeed())
		case "manifest fifo":
			Expect(os.Remove(manifest)).To(Succeed())
			Expect(syscall.Mkfifo(manifest, 0600)).To(Succeed())
		case "manifest hardlink":
			Expect(os.Link(manifest, filepath.Join(root, "PRIVATE_MANIFEST_LINK"))).To(Succeed())
		case "copy hardlink":
			Expect(os.Link(leaf, filepath.Join(root, "PRIVATE_COPY_LINK"))).To(Succeed())
		case "copy symlink":
			Expect(os.Remove(leaf)).To(Succeed())
			Expect(os.Symlink(filepath.Join(root, assessmentPaths[0]), leaf)).To(Succeed())
		case "copy fifo":
			Expect(os.Remove(leaf)).To(Succeed())
			Expect(syscall.Mkfifo(leaf, 0600)).To(Succeed())
		case "copy executable":
			Expect(os.Chmod(leaf, 0700)).To(Succeed())
		case "copy group readable":
			Expect(os.Chmod(leaf, 0640)).To(Succeed())
		case "copy setuid":
			Expect(os.Chmod(leaf, 0600|os.ModeSetuid)).To(Succeed())
		case "copy setgid":
			Expect(os.Chmod(leaf, 0600|os.ModeSetgid)).To(Succeed())
		case "copy sticky":
			Expect(os.Chmod(leaf, 0600|os.ModeSticky)).To(Succeed())
		case "manifest mode":
			Expect(os.Chmod(manifest, 0644)).To(Succeed())
		case "set mode":
			Expect(os.Chmod(set, 0755)).To(Succeed())
		case "ancestor mode":
			Expect(os.Chmod(filepath.Join(set, "files/scripts"), 0755)).To(Succeed())
		case "ancestor symlink":
			old := filepath.Join(set, "files/scripts")
			Expect(os.Rename(old, old+".held")).To(Succeed())
			Expect(os.Symlink(old+".held", old)).To(Succeed())
		}
		before := assessmentTree(root)
		out := recoveryCommand(binary, root)
		recoveryOnlyRow(recoveryDecode(out, root), classification)
		Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_SAVED_CUSTOM"))
		Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_EXTRA_CONTENT"))
		Expect(assessmentTree(root)).To(Equal(before))
	}, Entry("missing manifest", "missing manifest", "unrecognized"), Entry("missing copy", "missing copy", "incomplete"), Entry("custom copy", "custom copy", "incomplete"), Entry("extra file", "extra file", "incomplete"), Entry("empty extra directory", "extra dir", "incomplete"), Entry("unknown subtree not read", "extra subtree", "incomplete"), Entry("extra fifo", "extra fifo", "unsafe"), Entry("extra symlink", "extra symlink", "unsafe"), Entry("manifest symlink", "manifest symlink", "unsafe"), Entry("manifest fifo", "manifest fifo", "unsafe"), Entry("manifest hardlink", "manifest hardlink", "unsafe"), Entry("copy hardlink", "copy hardlink", "unsafe"), Entry("copy symlink", "copy symlink", "unsafe"), Entry("copy fifo", "copy fifo", "unsafe"), Entry("copy executable", "copy executable", "unsafe"), Entry("copy group readable", "copy group readable", "unsafe"), Entry("copy setuid", "copy setuid", "unsafe"), Entry("copy setgid", "copy setgid", "unsafe"), Entry("copy sticky", "copy sticky", "unsafe"), Entry("manifest mode", "manifest mode", "unsafe"), Entry("set mode", "set mode", "unsafe"), Entry("nested directory mode", "ancestor mode", "unsafe"), Entry("nested directory symlink", "ancestor symlink", "unsafe"))
	// per docs/adr/0083-go-recovery-set-inspection.md:69
	DescribeTable("distinguishes absent and unsafe roots without traversal", func(change, status string) {
		binary, root, _ := assessmentFixture()
		factory := filepath.Join(root, ".factory")
		backups := filepath.Join(factory, "backups")
		if change != "factory symlink" {
			Expect(os.Mkdir(factory, 0700)).To(Succeed())
		}
		switch change {
		case "factory only":
		case "factory symlink":
			Expect(os.Symlink("/PRIVATE_OUTSIDE", factory)).To(Succeed())
		case "factory write":
			Expect(os.Chmod(factory, 0770)).To(Succeed())
		case "backup symlink":
			Expect(os.Symlink("/PRIVATE_OUTSIDE", backups)).To(Succeed())
		case "backup mode":
			Expect(os.Mkdir(backups, 0755)).To(Succeed())
		case "backup fifo":
			Expect(syscall.Mkfifo(backups, 0600)).To(Succeed())
		}
		before := assessmentTree(root)
		inventory := recoveryDecode(recoveryCommand(binary, root), root)
		Expect(inventory["root_status"]).To(Equal(status))
		Expect(inventory["complete"]).To(Equal(status == "absent"))
		Expect(inventory["sets"]).To(BeEmpty())
		Expect(assessmentTree(root)).To(Equal(before))
	}, Entry("missing backups", "factory only", "absent"), Entry("factory symlink", "factory symlink", "unsafe"), Entry("factory writable by group", "factory write", "unsafe"), Entry("backup symlink", "backup symlink", "unsafe"), Entry("backup mode", "backup mode", "unsafe"), Entry("backup fifo", "backup fifo", "unsafe"))
})

var _ = Describe("G4 recovery inventory composition and limits", func() {
	// per docs/adr/0083-go-recovery-set-inspection.md:19
	It("leaves default eight-field JSON and text byte-identical without inspecting unsafe backups", func() {
		binary, root, _ := assessmentFixture()
		jsonBefore := upgradePreview(binary, root, "--dry-run", "--source=.", "--json")
		textBefore := upgradePreview(binary, root, "--dry-run", "--source=.")
		Expect(budgetDecode([]byte(jsonBefore.stdout))).To(HaveLen(8))
		Expect(os.Mkdir(filepath.Join(root, ".factory"), 0700)).To(Succeed())
		Expect(syscall.Mkfifo(filepath.Join(root, ".factory/backups"), 0600)).To(Succeed())
		Expect(upgradePreview(binary, root, "--dry-run", "--source=.", "--json")).To(Equal(jsonBefore))
		Expect(upgradePreview(binary, root, "--dry-run", "--source=.")).To(Equal(textBefore))
	})
	// per docs/adr/0083-go-recovery-set-inspection.md:17
	DescribeTable("rejects duplicate or valued inspection flags before filesystem assessment", func(flag string) {
		binary, root, _ := assessmentFixture()
		marker := previewPoison(binary)
		args := []string{"--dry-run", "--source=/PRIVATE_MISSING", "--inspect-backups"}
		if flag == "duplicate" {
			args = append(args, "--inspect-backups")
		} else {
			args[2] = "--inspect-backups=" + flag
		}
		out := upgradePreview(binary, root, args...)
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).To(Equal("factory upgrade: invalid preview arguments\n"))
		publicNoPoison(marker)
	}, Entry("duplicate", "duplicate"), Entry("true value", "true"), Entry("false value", "false"), Entry("empty value", ""))
	// per docs/adr/0083-go-recovery-set-inspection.md:19
	It("delegates inspection-looking operands literally without dry-run", func() {
		binary, root, _ := assessmentFixture()
		writeFixture(filepath.Join(binary, "scripts/factory-upgrade.sh"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\nexit 43\n"), 0700)
		Expect(upgradePreview(binary, root, "--inspect-backups", "--source=.")).To(Equal(cliResult{"--inspect-backups\n--source=.\n", "", 43}))
	})
	// per docs/adr/0083-go-recovery-set-inspection.md:75
	It("sorts held and ordinary sets and sums only all-six validated saved originals", func() {
		binary, root, references := assessmentFixture()
		recoverySet(root, "z-last", references, true)
		recoverySet(root, "a-first", references[:1], false)
		bad := filepath.Join(root, ".factory/backups/m-unknown")
		Expect(os.Mkdir(bad, 0700)).To(Succeed())
		before := assessmentTree(root)
		out := recoveryCommand(binary, root)
		inventory := recoveryDecode(out, root)
		rows := inventory["sets"].([]any)
		Expect(rows).To(HaveLen(3))
		Expect(inventory["set_count"]).To(Equal(json.Number("3")))
		Expect(inventory["file_count"]).To(Equal(json.Number("7")))
		bytes := references[0].Bytes
		for _, reference := range references {
			bytes += reference.Bytes
		}
		Expect(inventory["bytes"]).To(Equal(json.Number(fmt.Sprint(bytes))))
		for i, name := range []string{"a-first", "m-unknown", "z-last"} {
			Expect(rows[i].(map[string]any)["path"]).To(Equal(".factory/backups/" + name))
		}
		Expect(rows[2].(map[string]any)["held"]).To(BeTrue())
		Expect(rows[2].(map[string]any)["classification"]).To(Equal("integrity_checked"))
		Expect(rows[1].(map[string]any)["held"]).To(BeNil())
		Expect(recoveryCommand(binary, root)).To(Equal(out))
		Expect(assessmentTree(root)).To(Equal(before))
	})
	// per docs/adr/0083-go-recovery-set-inspection.md:47
	It("rejects matching forged copy metadata against the immutable catalog", func() {
		binary, root, references := assessmentFixture()
		set := recoverySet(root, "set-1", references[:1], false)
		data := []byte("PRIVATE_FORGED_ORIGINAL")
		Expect(os.WriteFile(filepath.Join(set, "files", assessmentPaths[0]), data, 0600)).To(Succeed())
		manifest := recoveryManifest(set)
		asset := manifest["assets"].([]any)[0].(map[string]any)
		asset["bytes"] = len(data)
		sum := sha256.Sum256(data)
		asset["sha256"] = hex.EncodeToString(sum[:])
		recoveryWriteManifest(set, manifest)
		recoveryOnlyRow(recoveryDecode(recoveryCommand(binary, root), root), "unrecognized")
	})
	// per docs/adr/0083-go-recovery-set-inspection.md:23
	It("quotes non-ASCII and terminal-control names in text without hiding preserved rows", func() {
		binary, root, _ := assessmentFixture()
		backups := filepath.Join(root, ".factory/backups")
		Expect(os.MkdirAll(backups, 0700)).To(Succeed())
		names := []string{"\x1bPRIVATE_CONTROL\n", "α-name", ".unknown"}
		for _, name := range names {
			Expect(os.Mkdir(filepath.Join(backups, name), 0700)).To(Succeed())
		}
		inventory := recoveryDecode(recoveryCommand(binary, root), root)
		Expect(inventory["sets"]).To(HaveLen(3))
		text := upgradePreview(binary, root, "--dry-run", "--source=.", "--inspect-backups")
		Expect(text.status).To(Equal(2))
		Expect(text.stderr).To(BeEmpty())
		Expect(text.stdout).NotTo(ContainSubstring("\x1b"))
		Expect(text.stdout).NotTo(ContainSubstring("PRIVATE_CONTROL\n"))
		Expect(text.stdout).NotTo(ContainSubstring(root))
		Expect(text.stdout).To(ContainSubstring("unrecognized_recovery_record"))
		Expect(text.stdout).To(ContainSubstring("inspect_preserved_set"))
		Expect(text.stdout).To(ContainSubstring("%CE%B1-name"))
		Expect(upgradePreview(binary, root, "--dry-run", "--source=.", "--inspect-backups")).To(Equal(text))
	})
	// per docs/adr/0083-go-recovery-set-inspection.md:96
	DescribeTable("bounds root enumeration without claiming partial arbitrary rows", func(count int) {
		binary, root, _ := assessmentFixture()
		backups := filepath.Join(root, ".factory/backups")
		Expect(os.MkdirAll(backups, 0700)).To(Succeed())
		for i := 0; i < count; i++ {
			Expect(os.Mkdir(filepath.Join(backups, fmt.Sprintf("set-%02d", i)), 0700)).To(Succeed())
		}
		inventory := recoveryDecode(recoveryCommand(binary, root), root)
		if count == 64 {
			Expect(inventory["root_status"]).To(Equal("inspected"))
			Expect(inventory["complete"]).To(BeTrue())
			Expect(inventory["sets"]).To(HaveLen(64))
		} else {
			Expect(inventory["root_status"]).To(Equal("limit_exceeded"))
			Expect(inventory["complete"]).To(BeFalse())
			Expect(inventory["sets"]).To(BeEmpty())
			Expect(inventory["set_count"]).To(Equal(json.Number("0")))
		}
	}, Entry("exact64", 64), Entry("excess65", 65))
	// per docs/adr/0083-go-recovery-set-inspection.md:96
	DescribeTable("bounds complete known-tree enumeration per set", func(extras int, classification string) {
		binary, root, references := assessmentFixture()
		set := recoverySet(root, "set-1", references[:1], false)
		// Four known entries: manifest, files, scripts, and the declared leaf.
		for i := 0; i < extras; i++ {
			writeFixture(filepath.Join(set, fmt.Sprintf("extra-%02d", i)), nil, 0600)
		}
		recoveryOnlyRow(recoveryDecode(recoveryCommand(binary, root), root), classification)
	}, Entry("exact32entries", 28, "incomplete"), Entry("33entries", 29, "limit_exceeded"))
	// per docs/adr/0083-go-recovery-set-inspection.md:57
	DescribeTable("enforces the manifest byte limit including trailing whitespace", func(size int, classification string) {
		binary, root, references := assessmentFixture()
		set := recoverySet(root, "set-1", references[:1], false)
		path := filepath.Join(set, "manifest.json")
		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		data = append(data, bytes.Repeat([]byte(" "), size-len(data))...)
		Expect(os.WriteFile(path, data, 0600)).To(Succeed())
		recoveryOnlyRow(recoveryDecode(recoveryCommand(binary, root), root), classification)
	}, Entry("exact16KiB", 16<<10, "integrity_checked"), Entry("over16KiB", (16<<10)+1, "limit_exceeded"))
	// per docs/adr/0083-go-recovery-set-inspection.md:52
	DescribeTable("refuses foreign-owned roots and saved leaves when host permits chown", func(target string) {
		if os.Geteuid() != 0 {
			Skip("requires permission to change fixture ownership to a different UID")
		}
		binary, root, references := assessmentFixture()
		set := recoverySet(root, "set-1", references[:1], false)
		path := filepath.Join(set, "files", assessmentPaths[0])
		if target == "root" {
			path = filepath.Join(root, ".factory/backups")
		}
		Expect(os.Chown(path, 65534, -1)).To(Succeed())
		DeferCleanup(os.Chown, path, 0, -1)
		inventory := recoveryDecode(recoveryCommand(binary, root), root)
		if target == "root" {
			Expect(inventory["root_status"]).To(Equal("unsafe"))
			Expect(inventory["complete"]).To(BeFalse())
		} else {
			recoveryOnlyRow(inventory, "unsafe")
		}
	}, Entry("root owner", "root"), Entry("leaf owner", "leaf"))
})

var _ = Describe("G4 recovery inventory held text", func() {
	// per docs/adr/0083-go-recovery-set-inspection.md:148
	DescribeTable("exposes the held marker without treating it as authority", func(marker string) {
		binary, root, references := assessmentFixture()
		set := recoverySet(root, "set-1", references[:1], marker == "true")
		if marker == "unknown" {
			Expect(os.Remove(filepath.Join(set, "manifest.json"))).To(Succeed())
		}
		out := upgradePreview(binary, root, "--dry-run", "--source=.", "--inspect-backups")
		Expect(out.status).To(Equal(2))
		Expect(out.stderr).To(BeEmpty())
		Expect(out.stdout).To(ContainSubstring("held: " + marker))
	}, Entry("held", "true"), Entry("unheld", "false"), Entry("unknown", "unknown"))
})

var _ = Describe("G4 recovery inventory raw-name identity", func() {
	// per docs/adr/0083-go-recovery-set-inspection.md:151
	It("escapes literal percent and dots without colliding with display escapes", func() {
		binary, root, _ := assessmentFixture()
		backups := filepath.Join(root, ".factory/backups")
		Expect(os.MkdirAll(backups, 0700)).To(Succeed())
		for _, name := range []string{"%FF", ".unknown", "a b"} {
			Expect(os.Mkdir(filepath.Join(backups, name), 0700)).To(Succeed())
		}
		inventory := recoveryDecode(recoveryCommand(binary, root), root)
		rows := inventory["sets"].([]any)
		Expect(rows).To(HaveLen(3))
		for i, name := range []string{"%25FF", "%2Eunknown", "a%20b"} {
			Expect(rows[i].(map[string]any)["path"]).To(Equal(".factory/backups/" + name))
		}
	})
	// per docs/adr/0083-go-recovery-set-inspection.md:151
	It("preserves distinct invalid UTF8 directory bytes sorted before display encoding", func() {
		binary, root, _ := assessmentFixture()
		backups := filepath.Join(root, ".factory/backups")
		Expect(os.MkdirAll(backups, 0700)).To(Succeed())
		for _, name := range []string{string([]byte{0xff}), string([]byte{0xfe}), "%FF"} {
			err := os.Mkdir(filepath.Join(backups, name), 0700)
			if errors.Is(err, syscall.EILSEQ) || errors.Is(err, syscall.EINVAL) {
				Skip("raw-name identity requires a filesystem accepting these bytes")
			}
			Expect(err).NotTo(HaveOccurred())
		}
		rows := recoveryDecode(recoveryCommand(binary, root), root)["sets"].([]any)
		Expect(rows).To(HaveLen(3))
		for i, name := range []string{"%25FF", "%FE", "%FF"} {
			Expect(rows[i].(map[string]any)["path"]).To(Equal(".factory/backups/" + name))
		}
	})
})
