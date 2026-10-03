package assessment

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

func recoveryPlanningFixture() (string, string, []string) {
	GinkgoHelper()
	root, set, paths := recoveryFaultFixture()
	data, err := os.ReadFile(filepath.Join(set, "manifest.json"))
	Expect(err).NotTo(HaveOccurred())
	var manifest map[string]any
	Expect(json.Unmarshal(data, &manifest)).To(Succeed())
	manifest["held"] = false
	data, err = json.Marshal(manifest)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.WriteFile(filepath.Join(set, "manifest.json"), data, 0600)).To(Succeed())
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(set, "files", path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, path), data, 0600)).To(Succeed()) // #nosec G703 -- fixture supplies only its two fixed catalog paths inside a test-owned root.
		Expect(os.Chmod(filepath.Join(root, path), 0755)).To(Succeed())
	}
	return root, set, paths
}

type recoveryPlanningHandles struct {
	files     []*os.File
	labels    map[*os.File]string
	locations map[string]string
}

func recoveryPlanningControls(root, set string, paths []string) (*recoveryPlanningHandles, ops) {
	GinkgoHelper()
	handles := &recoveryPlanningHandles{labels: map[*os.File]string{}, locations: map[string]string{root: "installation", filepath.Join(root, "scripts"): "installed/scripts", filepath.Join(root, ".factory"): "factory", filepath.Join(root, ".factory/backups"): "backups", set: "set", filepath.Join(set, "files"): "saved/files", filepath.Join(set, "files/scripts"): "saved/scripts", filepath.Join(set, "manifest.json"): "manifest"}}
	for _, path := range paths {
		handles.locations[filepath.Join(root, path)] = "installed/" + path
		handles.locations[filepath.Join(set, "files", path)] = "saved/" + path
	}
	controls := ops{open: func(parent *os.File, name string, flags int) (*os.File, error) {
		Expect(flags&(unix.O_WRONLY|unix.O_RDWR|unix.O_CREAT|unix.O_TRUNC|unix.O_APPEND)).To(BeZero(), "planning must open only read descriptors")
		Expect(flags & unix.O_NOFOLLOW).NotTo(BeZero())
		handles.capture(parent)
		file, err := openAt(parent, name, flags)
		if file != nil {
			handles.capture(file)
		}
		return file, err
	}}
	return handles, controls
}

func (h *recoveryPlanningHandles) capture(file *os.File) {
	if _, exists := h.labels[file]; exists {
		return
	}
	h.files = append(h.files, file)
	h.labels[file] = "unknown"
	info, err := file.Stat()
	if err != nil {
		return
	}
	for path, label := range h.locations {
		other, statErr := os.Lstat(path)
		if statErr == nil && os.SameFile(info, other) {
			h.labels[file] = label
			return
		}
	}
}

func recoveryPlanningError(result RecoveryPlan, err error, status int) {
	GinkgoHelper()
	if err != nil {
		Expect(ErrorStatus(err)).To(Equal(status))
		Expect(err.Error()).NotTo(ContainSubstring("PRIVATE_"))
	} else {
		Expect(result.Status()).To(Equal(status))
	}
	Expect(result.Restorable).To(BeFalse())
	Expect(result.RollbackReady).To(BeFalse())
	Expect(result.ActivationReady).To(BeFalse())
	Expect(result.Applicable).To(BeFalse())
	Expect(result.PruneAuthorized).To(BeFalse())
}

var _ = Describe("Read-only restoration planning operation failures", func() {
	// per docs/adr/0092-go-recovery-restoration-planning.md:56
	// per docs/adr/0092-go-recovery-restoration-planning.md:65
	// per docs/adr/0092-go-recovery-restoration-planning.md:93
	DescribeTable("reports real bounded I/O failures without leaking raw errors and closes all handles", func(stage string) {
		root, set, paths := recoveryPlanningFixture()
		handles, controls := recoveryPlanningControls(root, set, paths)
		actualOpen := controls.open
		triggered := false
		controls.open = func(parent *os.File, name string, flags int) (*os.File, error) {
			handles.capture(parent)
			if (stage == "manifest open" && name == "manifest.json") || (stage == "installed open" && handles.labels[parent] == "installed/scripts" && name == filepath.Base(paths[0])) {
				triggered = true
				return nil, errors.New("PRIVATE_PLANNING_OPEN_FAILURE")
			}
			return actualOpen(parent, name, flags)
		}
		controls.read = func(file *os.File, buffer []byte) (int, error) {
			Expect(len(buffer)).To(BeNumerically("<=", 32<<10))
			n, err := file.Read(buffer)
			label := handles.labels[file]
			if !triggered && ((stage == "manifest read" && label == "manifest") || (stage == "saved copy read" && label == "saved/"+paths[0]) || (stage == "installed read" && label == "installed/"+paths[0])) {
				triggered = true
				return n, errors.New("PRIVATE_PLANNING_READ_FAILURE")
			}
			return n, err
		}
		controls.named = func(parent *os.File, name string) (unix.Stat_t, error) {
			handles.capture(parent)
			if stage == "installed metadata" && handles.labels[parent] == "installed/scripts" && name == filepath.Base(paths[0]) {
				triggered = true
				return unix.Stat_t{}, syscall.EIO
			}
			return named(parent, name)
		}
		result, err := planRecovery(context.Background(), root, "set-1", controls)
		Expect(triggered).To(BeTrue(), "the requested fault operation was never exercised")
		recoveryPlanningError(result, err, 1)
		if strings.HasPrefix(stage, "manifest") || stage == "saved copy read" {
			Expect(result.Assets).To(BeEmpty())
		}
		encoded, encodeErr := json.Marshal(result)
		Expect(encodeErr).NotTo(HaveOccurred())
		Expect(string(encoded)).NotTo(ContainSubstring("PRIVATE_"))
		recoveryClosed(handles.files)
	}, Entry("manifest open", "manifest open"), Entry("manifest read after bytes", "manifest read"), Entry("saved copy read after bytes", "saved copy read"), Entry("installed file open", "installed open"), Entry("installed read after bytes", "installed read"), Entry("installed metadata", "installed metadata"))

	// per docs/adr/0092-go-recovery-restoration-planning.md:93
	DescribeTable("cancels at actual source or destination operations without returning a complete plan", func(stage string) {
		root, set, paths := recoveryPlanningFixture()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		handles, controls := recoveryPlanningControls(root, set, paths)
		actualOpen := controls.open
		triggered := false
		controls.open = func(parent *os.File, name string, flags int) (*os.File, error) {
			file, err := actualOpen(parent, name, flags)
			if (stage == "set open" && name == "set-1") || (stage == "installed open" && handles.labels[file] == "installed/"+paths[0]) {
				triggered = true
				cancel()
			}
			return file, err
		}
		controls.read = func(file *os.File, buffer []byte) (int, error) {
			n, err := file.Read(buffer)
			label := handles.labels[file]
			if (stage == "manifest read" && label == "manifest") || (stage == "saved read" && label == "saved/"+paths[0]) || (stage == "installed read" && label == "installed/"+paths[0]) {
				triggered = true
				cancel()
			}
			return n, err
		}
		result, err := planRecovery(ctx, root, "set-1", controls)
		Expect(triggered).To(BeTrue())
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		recoveryPlanningError(result, err, 1)
		Expect(result.Assets).To(BeEmpty())
		recoveryClosed(handles.files)
	}, Entry("recovery set open", "set open"), Entry("manifest read", "manifest read"), Entry("saved file read", "saved read"), Entry("installed file open", "installed open"), Entry("installed file read", "installed read"))

	// per docs/adr/0092-go-recovery-restoration-planning.md:94
	DescribeTable("treats descriptor close failure as operational failure and discards a completed report", func(rootClose bool) {
		root, set, paths := recoveryPlanningFixture()
		handles, controls := recoveryPlanningControls(root, set, paths)
		triggered := false
		controls.close = func(file *os.File) error {
			err := file.Close()
			label := handles.labels[file]
			if (rootClose && label == "installation") || (!rootClose && label == "manifest") {
				triggered = true
				return errors.Join(err, errors.New("PRIVATE_PLANNING_CLOSE_FAILURE"))
			}
			return err
		}
		result, err := planRecovery(context.Background(), root, "set-1", controls)
		Expect(triggered).To(BeTrue())
		Expect(err).To(HaveOccurred())
		recoveryPlanningError(result, err, 1)
		Expect(result.Assets).To(BeEmpty())
		recoveryClosed(handles.files)
	}, Entry("manifest descriptor", false), Entry("installation descriptor", true))
})

var _ = Describe("Read-only restoration planning observation lifetime", func() {
	// per docs/adr/0092-go-recovery-restoration-planning.md:48
	// per docs/adr/0092-go-recovery-restoration-planning.md:50
	// per docs/adr/0092-go-recovery-restoration-planning.md:133
	DescribeTable("invalidates earlier source or installed observations changed during a later installed read", func(change string) {
		root, set, paths := recoveryPlanningFixture()
		if change == "missing installed path" {
			Expect(os.Remove(filepath.Join(root, paths[0]))).To(Succeed())
		}
		handles, controls := recoveryPlanningControls(root, set, paths)
		triggered := false
		controls.read = func(file *os.File, buffer []byte) (int, error) {
			n, err := file.Read(buffer)
			if !triggered && handles.labels[file] == "installed/"+paths[1] {
				triggered = true
				switch change {
				case "saved copy", "saved manifest", "installed inode":
					path := filepath.Join(set, "files", paths[0])
					if change == "saved manifest" {
						path = filepath.Join(set, "manifest.json")
					}
					if change == "installed inode" {
						path = filepath.Join(root, paths[0])
					}
					data, readErr := os.ReadFile(path)
					Expect(readErr).NotTo(HaveOccurred())
					Expect(os.Rename(path, path+".held")).To(Succeed())
					Expect(os.WriteFile(path, data, 0600)).To(Succeed())
					if change == "installed inode" {
						Expect(os.Chmod(path, 0755)).To(Succeed())
					}
				case "installed bytes":
					Expect(os.WriteFile(filepath.Join(root, paths[0]), []byte("PRIVATE_LATER_CHANGE"), 0600)).To(Succeed())
				case "installed mode":
					Expect(os.Chmod(filepath.Join(root, paths[0]), 0700)).To(Succeed())
				case "missing installed path":
					Expect(os.WriteFile(filepath.Join(root, paths[0]), []byte("PRIVATE_NEW_OCCUPANT"), 0600)).To(Succeed())
				case "saved directory", "installed directory", "set directory":
					path := filepath.Join(set, "files/scripts")
					if change == "installed directory" {
						path = filepath.Join(root, "scripts")
					}
					if change == "set directory" {
						path = set
					}
					Expect(os.Rename(path, path+".held")).To(Succeed())
					Expect(os.Mkdir(path, 0700)).To(Succeed())
				case "installation":
					Expect(os.Rename(root, root+".held")).To(Succeed())
					DeferCleanup(os.RemoveAll, root+".held")
					Expect(os.Mkdir(root, 0700)).To(Succeed())
				case "installation mode":
					Expect(os.Chmod(root, 0770)).To(Succeed())
				}
			}
			return n, err
		}
		result, err := planRecovery(context.Background(), root, "set-1", controls)
		Expect(triggered).To(BeTrue())
		Expect(err).To(HaveOccurred(), "changed observations must discard the report instead of blaming recovery storage")
		recoveryPlanningError(result, err, 2)
		Expect(result).To(Equal(RecoveryPlan{}), "the entire transient plan must be discarded")
		Expect(err.Error()).NotTo(ContainSubstring(root))
		recoveryClosed(handles.files)
	}, Entry("saved bytes identical replacement", "saved copy"), Entry("saved manifest identical replacement", "saved manifest"), Entry("saved ancestor replacement", "saved directory"), Entry("set ancestor replacement", "set directory"), Entry("installed identical replacement", "installed inode"), Entry("installed content", "installed bytes"), Entry("installed complete mode", "installed mode"), Entry("installed absent leaf acquired occupant", "missing installed path"), Entry("installed ancestor replacement", "installed directory"), Entry("installation root replacement", "installation"), Entry("installation root writing mode", "installation mode"))

	// per docs/adr/0092-go-recovery-restoration-planning.md:49
	// per docs/adr/0092-go-recovery-restoration-planning.md:134
	It("revalidates a missing safe installed ancestor before publishing missing candidates", func() {
		root, set, paths := recoveryPlanningFixture()
		Expect(os.RemoveAll(filepath.Join(root, "scripts"))).To(Succeed())
		handles, controls := recoveryPlanningControls(root, set, paths)
		observations, triggered := 0, false
		controls.named = func(parent *os.File, name string) (unix.Stat_t, error) {
			handles.capture(parent)
			if handles.labels[parent] == "installation" && name == "scripts" {
				observations++
				if observations == 2 {
					triggered = true
					Expect(os.Mkdir(filepath.Join(root, "scripts"), 0700)).To(Succeed())
					Expect(os.WriteFile(filepath.Join(root, paths[0]), []byte("PRIVATE_NEW_TREE"), 0600)).To(Succeed())
				}
			}
			return named(parent, name)
		}
		result, err := planRecovery(context.Background(), root, "set-1", controls)
		Expect(triggered).To(BeTrue())
		Expect(err).To(HaveOccurred())
		recoveryPlanningError(result, err, 2)
		Expect(result).To(Equal(RecoveryPlan{}))
		recoveryClosed(handles.files)
	})

	// per docs/adr/0092-go-recovery-restoration-planning.md:133
	// per docs/adr/0092-go-recovery-restoration-planning.md:135
	It("preserves an earlier assessment error when later observations change and discards the entire plan", func() {
		root, set, paths := recoveryPlanningFixture()
		handles, controls := recoveryPlanningControls(root, set, paths)
		readFailed, changed := false, false
		controls.read = func(file *os.File, buffer []byte) (int, error) {
			n, err := file.Read(buffer)
			if !readFailed && handles.labels[file] == "installed/"+paths[0] {
				readFailed = true
				return n, errors.New("PRIVATE_EARLIER_READ_FAILURE")
			}
			if !changed && handles.labels[file] == "installed/"+paths[1] {
				changed = true
				Expect(os.Chmod(filepath.Join(root, paths[0]), 0700)).To(Succeed())
			}
			return n, err
		}
		result, err := planRecovery(context.Background(), root, "set-1", controls)
		Expect(readFailed).To(BeTrue())
		Expect(changed).To(BeTrue())
		Expect(err).To(HaveOccurred())
		recoveryPlanningError(result, err, 1)
		Expect(result).To(Equal(RecoveryPlan{}))
		Expect(err.Error()).To(Equal("recovery planning observations changed"))
		recoveryClosed(handles.files)
	})

	// per docs/adr/0092-go-recovery-restoration-planning.md:133
	// per docs/adr/0092-go-recovery-restoration-planning.md:135
	DescribeTable("preserves an earlier assessment error when a later installation root change invalidates the plan", func(replaceRoot bool) {
		root, set, paths := recoveryPlanningFixture()
		handles, controls := recoveryPlanningControls(root, set, paths)
		readFailed, changed := false, false
		controls.read = func(file *os.File, buffer []byte) (int, error) {
			n, err := file.Read(buffer)
			if !readFailed && handles.labels[file] == "installed/"+paths[0] {
				readFailed = true
				return n, errors.New("PRIVATE_EARLIER_ROOT_READ_FAILURE")
			}
			if !changed && handles.labels[file] == "installed/"+paths[1] {
				Expect(readFailed).To(BeTrue(), "the read failure must already be observed before mutating the root")
				changed = true
				before, statErr := os.Lstat(root)
				Expect(statErr).NotTo(HaveOccurred())
				if replaceRoot {
					Expect(os.Rename(root, root+".held")).To(Succeed())
					DeferCleanup(os.RemoveAll, root+".held")
					Expect(os.Mkdir(root, 0700)).To(Succeed())
					after, statErr := os.Lstat(root)
					Expect(statErr).NotTo(HaveOccurred())
					Expect(os.SameFile(before, after)).To(BeFalse(), "control must replace the root inode")
				} else {
					Expect(os.Chmod(root, 0770)).To(Succeed())
					after, statErr := os.Lstat(root)
					Expect(statErr).NotTo(HaveOccurred())
					Expect(after.Mode().Perm()).To(Equal(os.FileMode(0770)), "control must change root metadata")
					Expect(os.SameFile(before, after)).To(BeTrue())
				}
			}
			return n, err
		}
		result, err := planRecovery(context.Background(), root, "set-1", controls)
		Expect(readFailed).To(BeTrue(), "the operational read failure must precede the root change")
		Expect(changed).To(BeTrue())
		Expect(err).To(HaveOccurred())
		Expect(result).To(Equal(RecoveryPlan{}), "the entire transient plan must be discarded")
		Expect(err.Error()).NotTo(ContainSubstring(root))
		recoveryPlanningError(result, err, 1)
		recoveryClosed(handles.files)
	}, Entry("root mode changed after an installed read failure", false), Entry("root inode replaced after an installed read failure", true))

	// per docs/adr/0092-go-recovery-restoration-planning.md:57
	It("never opens unrelated recovery sets or runtime history while planning one intact selection", func() {
		root, set, paths := recoveryPlanningFixture()
		Expect(syscall.Mkfifo(filepath.Join(root, ".factory/PRIVATE_HISTORY"), 0600)).To(Succeed())
		Expect(os.Mkdir(filepath.Join(root, ".factory/backups/PRIVATE_UNRELATED"), 0700)).To(Succeed())
		handles, controls := recoveryPlanningControls(root, set, paths)
		actualOpen := controls.open
		controls.open = func(parent *os.File, name string, flags int) (*os.File, error) {
			Expect(name).NotTo(BeElementOf("PRIVATE_UNRELATED", "PRIVATE_HISTORY"))
			return actualOpen(parent, name, flags)
		}
		result, err := planRecovery(context.Background(), root, "set-1", controls)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status()).To(Equal(2))
		Expect(result.Assets).To(HaveLen(2))
		recoveryClosed(handles.files)
	})

	// per docs/adr/0092-go-recovery-restoration-planning.md:94
	It("closes retained handles on a complete ineligible plan and keeps collaborators local to one call", func() {
		root, set, paths := recoveryPlanningFixture()
		handles, controls := recoveryPlanningControls(root, set, paths)
		reads := 0
		controls.read = func(file *os.File, buffer []byte) (int, error) { reads++; return file.Read(buffer) }
		result, err := planRecovery(context.Background(), root, "set-1", controls)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status()).To(Equal(2))
		Expect(result.Assets).To(HaveLen(2))
		Expect(reads).To(BeNumerically(">", 0))
		recoveryClosed(handles.files)
		before := reads
		result, err = PlanRecovery(context.Background(), root, "set-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Assets).To(HaveLen(2))
		Expect(reads).To(Equal(before))
	})
})

var _ = Describe("Read-only restoration planning status", func() {
	// per docs/adr/0092-go-recovery-restoration-planning.md:91
	It("never upgrades an intact all-reference plan to success status", func() {
		root, _, _ := recoveryPlanningFixture()
		result, err := PlanRecovery(context.Background(), root, "set-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status()).To(Equal(2))
		Expect(result.Counts["retain_reference"]).To(Equal(2))
	})

	// per docs/adr/0092-go-recovery-restoration-planning.md:93
	It("does not open storage when the operation is already canceled", func() {
		root, _, _ := recoveryPlanningFixture()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result, err := planRecovery(ctx, root, "set-1", ops{open: func(*os.File, string, int) (*os.File, error) {
			Fail("ended context must not open storage")
			return nil, io.EOF
		}})
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		recoveryPlanningError(result, err, 1)
	})
})
