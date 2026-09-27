package assessment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

func recoveryFaultFixture() (string, string, []string) {
	GinkgoHelper()
	root, err := os.MkdirTemp("", "factory-recovery-fault-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, root)
	root, err = filepath.EvalSymlinks(root)
	Expect(err).NotTo(HaveOccurred())
	set := filepath.Join(root, ".factory/backups/set-1")
	paths := []string{"scripts/factory-budget.sh", "scripts/factory-loop.sh"}
	var assets []map[string]any
	for _, path := range paths {
		data, err := exec.Command("git", "show", "c8f8d34edbc14df5655fcbf0aab7eea46ced0295:"+path).Output() // #nosec G204 -- fixed immutable revision and two constant catalog paths, no shell.
		Expect(err).NotTo(HaveOccurred())
		sum := sha256.Sum256(data)
		assets = append(assets, map[string]any{"path": path, "sha256": hex.EncodeToString(sum[:]), "bytes": len(data), "mode": "0755"})
		leaf := filepath.Join(set, "files", path)
		Expect(os.MkdirAll(filepath.Dir(leaf), 0700)).To(Succeed())
		Expect(os.WriteFile(leaf, data, 0600)).To(Succeed())
	}
	manifest := map[string]any{"schema_version": 1, "migration_id": "set-1", "source_revision": "c8f8d34edbc14df5655fcbf0aab7eea46ced0295", "target_revision": strings.Repeat("a", 40), "scope": "g2-budget-loop-six", "held": true, "assets": assets}
	data, err := json.Marshal(manifest)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.WriteFile(filepath.Join(set, "manifest.json"), data, 0600)).To(Succeed())
	return root, set, paths
}
func recoveryFaultObject(value any) map[string]any {
	GinkgoHelper()
	data, err := json.Marshal(value)
	Expect(err).NotTo(HaveOccurred())
	var object map[string]any
	Expect(json.Unmarshal(data, &object)).To(Succeed())
	return object
}
func recoveryClosed(files []*os.File) {
	GinkgoHelper()
	Expect(files).NotTo(BeEmpty())
	for _, file := range files {
		_, err := file.Stat()
		Expect(errors.Is(err, os.ErrClosed)).To(BeTrue(), "descriptor left open: %s", file.Name())
	}
}

var _ = Describe("Recovery inventory controlled observations", func() {
	// per docs/adr/0083-go-recovery-set-inspection.md:101
	DescribeTable("rejects identity and listing changes across the complete set observation", func(change string) {
		root, set, paths := recoveryFaultFixture()
		var opened []*os.File
		names := make(map[*os.File]string)
		changed := false
		controls := ops{open: func(parent *os.File, name string, flags int) (*os.File, error) {
			file, err := openAt(parent, name, flags)
			if file != nil {
				opened = append(opened, file)
				names[file] = name
			}
			return file, err
		}, read: func(file *os.File, buffer []byte) (int, error) {
			n, err := file.Read(buffer)
			if err != nil {
				return n, err
			}
			trigger := names[file] == "manifest.json"
			if change == "earlier leaf" || change == "earlier leaf mode" {
				trigger = names[file] == filepath.Base(paths[1])
			}
			if trigger && !changed {
				changed = true
				switch change {
				case "installation mode":
					Expect(os.Chmod(root, 0755)).To(Succeed())
				case "root":
					target := filepath.Join(root, ".factory/backups")
					Expect(os.Rename(target, target+".held")).To(Succeed())
					Expect(os.Mkdir(target, 0700)).To(Succeed())
				case "set":
					Expect(os.Rename(set, set+".held")).To(Succeed())
					Expect(os.Mkdir(set, 0700)).To(Succeed())
				case "leaf":
					path := filepath.Join(set, "manifest.json")
					data, readErr := os.ReadFile(path)
					Expect(readErr).NotTo(HaveOccurred())
					Expect(os.Rename(path, path+".held")).To(Succeed())
					Expect(os.WriteFile(path, data, 0600)).To(Succeed())
				case "listing":
					Expect(os.WriteFile(filepath.Join(set, "new-extra"), nil, 0600)).To(Succeed())
				case "directory mode":
					Expect(os.Chmod(filepath.Join(set, "files/scripts"), 0755)).To(Succeed())
				case "earlier leaf":
					path := filepath.Join(set, "files", paths[0])
					data, readErr := os.ReadFile(path)
					Expect(readErr).NotTo(HaveOccurred())
					Expect(os.Rename(path, path+".held")).To(Succeed())
					Expect(os.WriteFile(path, data, 0600)).To(Succeed())
				case "earlier leaf mode":
					Expect(os.Chmod(filepath.Join(set, "files", paths[0]), 0700)).To(Succeed())
				}
			}
			return n, nil
		}}
		result, err := inspectRecovery(context.Background(), root, controls)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())
		Expect(result.Status()).To(Equal(2))
		object := recoveryFaultObject(result)
		Expect(object["file_count"]).To(Equal(float64(0)))
		Expect(object["bytes"]).To(Equal(float64(0)))
		if change == "root" || change == "set" || change == "installation mode" {
			Expect(object["root_status"]).To(Equal("unsafe"))
			Expect(object["complete"]).To(BeFalse())
			Expect(object["sets"]).To(BeEmpty())
		} else {
			Expect(object["sets"]).To(HaveLen(1))
			Expect(object["sets"].([]any)[0].(map[string]any)["classification"]).To(Equal("unsafe"))
		}
		recoveryClosed(opened)
	}, Entry("installation root mode mutation", "installation mode"), Entry("backup root replacement", "root"), Entry("set replacement", "set"), Entry("manifest replacement", "leaf"), Entry("set listing mutation", "listing"), Entry("directory mode mutation", "directory mode"), Entry("earlier leaf replaced during later read", "earlier leaf"), Entry("earlier leaf mode changed during later read", "earlier leaf mode"))
	// per docs/adr/0083-go-recovery-set-inspection.md:99
	DescribeTable("propagates cancellation without a complete empty inventory and closes handles", func(stage string) {
		root, _, _ := recoveryFaultFixture()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var opened []*os.File
		names := make(map[*os.File]string)
		triggered := false
		controls := ops{open: func(parent *os.File, name string, flags int) (*os.File, error) {
			file, err := openAt(parent, name, flags)
			if file != nil {
				opened = append(opened, file)
				names[file] = name
			}
			if (stage == "directory" && name == "backups") || (stage == "factory" && name == ".factory") || (stage == "set" && name == "set-1") || (stage == "nested" && name == "files") {
				triggered = true
				cancel()
			}
			return file, err
		}, read: func(file *os.File, buffer []byte) (int, error) {
			n, err := file.Read(buffer)
			if (stage == "manifest" && names[file] == "manifest.json") || (stage == "copy" && names[file] == "factory-budget.sh") {
				triggered = true
				cancel()
			}
			return n, err
		}}
		result, err := inspectRecovery(ctx, root, controls)
		Expect(triggered).To(BeTrue())
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		object := recoveryFaultObject(result)
		Expect(object["restorable"]).To(BeFalse())
		Expect(object["prune_authorized"]).To(BeFalse())
		Expect(object["complete"]).NotTo(BeTrue())
		recoveryClosed(opened)
	}, Entry("before directory enumeration", "directory"), Entry("factory opened", "factory"), Entry("set opened", "set"), Entry("nested opened", "nested"), Entry("manifest read", "manifest"), Entry("copy read", "copy"))
	// per docs/adr/0083-go-recovery-set-inspection.md:90
	DescribeTable("reports operating system errors without false absence or raw error disclosure", func(stage string) {
		root, _, _ := recoveryFaultFixture()
		var opened []*os.File
		names := make(map[*os.File]string)
		triggered := false
		controls := ops{open: func(parent *os.File, name string, flags int) (*os.File, error) {
			if stage == "root open" && name == "backups" {
				triggered = true
				return nil, syscall.EIO
			}
			file, err := openAt(parent, name, flags)
			if file != nil {
				opened = append(opened, file)
				names[file] = name
			}
			return file, err
		}, read: func(file *os.File, buffer []byte) (int, error) {
			if (stage == "manifest read" && names[file] == "manifest.json") || (stage == "copy read" && names[file] == "factory-budget.sh") {
				triggered = true
				return 0, errors.New("PRIVATE_RECOVERY_IO_ERROR")
			}
			return file.Read(buffer)
		}}
		result, err := inspectRecovery(context.Background(), root, controls)
		Expect(err).NotTo(HaveOccurred())
		Expect(triggered).To(BeTrue())
		Expect(result.Status()).To(Equal(1))
		object := recoveryFaultObject(result)
		if stage == "root open" {
			Expect(object["root_status"]).To(Equal("assessment_error"))
			Expect(object["complete"]).To(BeFalse())
		} else {
			Expect(object["sets"]).To(HaveLen(1))
			Expect(object["sets"].([]any)[0].(map[string]any)["classification"]).To(Equal("assessment_error"))
		}
		Expect(object["file_count"]).To(Equal(float64(0)))
		Expect(object["bytes"]).To(Equal(float64(0)))
		data, marshalErr := json.Marshal(result)
		Expect(marshalErr).NotTo(HaveOccurred())
		Expect(string(data)).NotTo(ContainSubstring("PRIVATE_"))
		recoveryClosed(opened)
	}, Entry("backup root open", "root open"), Entry("manifest I/O", "manifest read"), Entry("saved original I/O", "copy read"))
})

var _ = Describe("Recovery inventory byte-preserving row", func() {
	// per docs/adr/0083-go-recovery-set-inspection.md:151
	It("serializes distinct raw names without filesystem-dependent Unicode replacement", func() {
		names := []string{string([]byte{0xff}), string([]byte{0xfe}), "%FF", ".x", "α"}
		expected := []string{"%FF", "%FE", "%25FF", "%2Ex", "%CE%B1"}
		for index, name := range names {
			object := recoveryFaultObject(recoveryRow(name, "unrecognized", nil))
			Expect(object["path"]).To(Equal(".factory/backups/" + expected[index]))
		}
	})
})

var _ = Describe("Recovery inventory extra metadata revalidation", func() {
	// per docs/adr/0083-go-recovery-set-inspection.md:101
	It("invalidates a safe unknown entry changed after its initial metadata observation", func() {
		root, set, _ := recoveryFaultFixture()
		extra := filepath.Join(set, "extra")
		Expect(os.WriteFile(extra, nil, 0600)).To(Succeed())
		changed := false
		controls := ops{named: func(parent *os.File, name string) (unix.Stat_t, error) {
			stat, err := named(parent, name)
			if name == "extra" && err == nil && !changed {
				changed = true
				Expect(os.Chmod(extra, 0666)).To(Succeed())
			}
			return stat, err
		}}
		result, err := inspectRecovery(context.Background(), root, controls)
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())
		object := recoveryFaultObject(result)
		Expect(object["root_status"]).To(Equal("inspected"))
		Expect(object["sets"]).To(HaveLen(1))
		row := object["sets"].([]any)[0].(map[string]any)
		Expect(row["classification"]).To(Equal("unsafe"))
		Expect(row["reason"]).To(Equal("unsafe_recovery_path"))
		Expect(object["file_count"]).To(Equal(float64(0)))
		Expect(object["bytes"]).To(Equal(float64(0)))
	})
})

var _ = Describe("Recovery inventory bounded payload reads", func() {
	// per docs/adr/0083-go-recovery-set-inspection.md:57
	It("refuses an oversized manifest before reading payload bytes", func() {
		root, set, _ := recoveryFaultFixture()
		Expect(os.WriteFile(filepath.Join(set, "manifest.json"), []byte(strings.Repeat(" ", (16<<10)+1)), 0600)).To(Succeed())
		reads := 0
		result, err := inspectRecovery(context.Background(), root, ops{read: func(file *os.File, buffer []byte) (int, error) { reads++; return file.Read(buffer) }})
		Expect(err).NotTo(HaveOccurred())
		Expect(reads).To(BeZero())
		Expect(result.Sets).To(HaveLen(1))
		Expect(result.Sets[0].Classification).To(Equal("limit_exceeded"))
	})
	// per docs/adr/0083-go-recovery-set-inspection.md:60
	It("does not open or read unknown extra payloads while preserving their set", func() {
		root, set, _ := recoveryFaultFixture()
		Expect(os.WriteFile(filepath.Join(set, "PRIVATE_EXTRA"), []byte("PRIVATE_UNKNOWN_PAYLOAD"), 0600)).To(Succeed())
		var names []string
		result, err := inspectRecovery(context.Background(), root, ops{open: func(parent *os.File, name string, flags int) (*os.File, error) {
			names = append(names, name)
			return openAt(parent, name, flags)
		}})
		Expect(err).NotTo(HaveOccurred())
		Expect(names).NotTo(ContainElement("PRIVATE_EXTRA"))
		Expect(result.Sets).To(HaveLen(1))
		Expect(result.Sets[0].Classification).To(Equal("incomplete"))
	})
})
