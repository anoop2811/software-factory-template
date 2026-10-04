package transition

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

func TestTransition(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Runtime transition collaborators")
}
func guardRoot() string {
	GinkgoHelper()
	root, err := filepath.EvalSymlinks(GinkgoT().TempDir())
	Expect(err).NotTo(HaveOccurred())
	return root
}
func guardEntries(root string) []os.DirEntry {
	GinkgoHelper()
	entries, err := os.ReadDir(filepath.Join(root, ".factory/runtime-activity"))
	Expect(err).NotTo(HaveOccurred())
	return entries
}
func guardOpen(_ context.Context, parent *os.File, name string, flags int, mode uint32) (*os.File, error) {
	fd := unix.AT_FDCWD
	if parent != nil {
		fd = int(parent.Fd())
	}
	descriptor, err := unix.Openat(fd, name, flags, mode)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(descriptor), "test-owned-transition"), nil
}
func guardControls() (ops, []*os.File, map[*os.File]string, *[]*os.File) {
	files := []*os.File{}
	labels := map[*os.File]string{}
	controls := ops{open: func(ctx context.Context, parent *os.File, name string, flags int, mode uint32) (*os.File, error) {
		Expect(flags & unix.O_NOFOLLOW).NotTo(BeZero())
		Expect(flags & unix.O_CLOEXEC).NotTo(BeZero())
		Expect(flags & unix.O_NONBLOCK).NotTo(BeZero())
		Expect(flags & unix.O_TRUNC).To(BeZero())
		file, err := guardOpen(ctx, parent, name, flags, mode)
		if file != nil {
			files = append(files, file)
			label := name
			if parent == nil {
				label = "project"
			}
			if labels[parent] == "runtime-activity" {
				label = "marker"
				Expect(flags & unix.O_EXCL).NotTo(BeZero())
				Expect(flags & unix.O_CREAT).NotTo(BeZero())
				Expect(mode).To(Equal(uint32(0600)))
			}
			labels[file] = label
		}
		return file, err
	}}
	return controls, files, labels, &files
}
func guardClosed(files []*os.File) {
	GinkgoHelper()
	for _, file := range files {
		_, err := file.Stat()
		Expect(errors.Is(err, os.ErrClosed)).To(BeTrue(), "guard must close every descriptor it acquired")
	}
}
func guardFailure(root string, guard *Guard, err error) {
	GinkgoHelper()
	Expect(guard).To(BeNil())
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).NotTo(ContainSubstring(root))
	Expect(err.Error()).NotTo(ContainSubstring("PRIVATE_"))
}
func guardManualExclusive(root string) {
	GinkgoHelper()
	f, err := os.OpenFile(filepath.Join(root, ".factory/runtime-transition.lock"), os.O_RDWR, 0)
	Expect(err).NotTo(HaveOccurred())
	defer func() { Expect(f.Close()).To(Succeed()) }()
	Expect(unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)).To(Succeed())
	Expect(unix.Flock(int(f.Fd()), unix.LOCK_UN)).To(Succeed())
}

var _ = Describe("Runtime transition exclusion and evidence", func() {
	// per docs/adr/0093-runtime-transition-guard.md:27
	// per docs/adr/0093-runtime-transition-guard.md:40
	// per docs/adr/0093-runtime-transition-guard.md:54
	It("preserves an owned 0755 state parent and holds a real shared flock with private evidence", func() {
		root := guardRoot()
		Expect(os.Mkdir(filepath.Join(root, ".factory"), 0755)).To(Succeed())
		g, err := Shared(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		Expect(g).NotTo(BeNil())
		entries := guardEntries(root)
		Expect(entries).To(HaveLen(1))
		marker := filepath.Join(root, ".factory/runtime-activity", entries[0].Name())
		for path, mode := range map[string]os.FileMode{filepath.Join(root, ".factory"): 0755, filepath.Join(root, ".factory/runtime-activity"): 0700, filepath.Join(root, ".factory/runtime-transition.lock"): 0600, marker: 0600} {
			info, err := os.Lstat(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(mode))
			if info.Mode().IsRegular() {
				Expect(info.Size()).To(BeZero())
			}
		}
		exclusive, err := Exclusive(context.Background(), root)
		guardFailure(root, exclusive, err)
		Expect(g.Close(context.Background(), true)).To(Succeed())
		Expect(guardEntries(root)).To(BeEmpty())
		guardManualExclusive(root)
	})
	// per docs/adr/0093-runtime-transition-guard.md:30
	// per docs/adr/0093-runtime-transition-guard.md:79
	// per docs/adr/0093-runtime-transition-guard.md:88
	It("permits independent shared owners but removes only its own marker and never treats retained activity as a PID lease", func() {
		root := guardRoot()
		a, err := Shared(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		first := guardEntries(root)
		Expect(first).To(HaveLen(1))
		b, err := Shared(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		Expect(guardEntries(root)).To(HaveLen(2))
		Expect(a.Close(context.Background(), true)).To(Succeed())
		remaining := guardEntries(root)
		Expect(remaining).To(HaveLen(1))
		Expect(remaining[0].Name()).NotTo(Equal(first[0].Name()))
		Expect(b.Close(context.Background(), false)).To(Succeed())
		guardManualExclusive(root)
		exclusive, err := Exclusive(context.Background(), root)
		guardFailure(root, exclusive, err)
		Expect(guardEntries(root)).To(HaveLen(1))
		Expect(guardEntries(root)[0].Name()).To(Equal(remaining[0].Name()))
	})
	// per docs/adr/0093-runtime-transition-guard.md:43
	It("accepts safe parent permission tightening on the same inode during owned work", func() {
		root := guardRoot()
		Expect(os.Mkdir(filepath.Join(root, ".factory"), 0755)).To(Succeed())
		before, err := os.Stat(filepath.Join(root, ".factory"))
		Expect(err).NotTo(HaveOccurred())
		g, err := Shared(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Chmod(filepath.Join(root, ".factory"), 0700)).To(Succeed())
		after, err := os.Stat(filepath.Join(root, ".factory"))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		Expect(g.Close(context.Background(), true)).To(Succeed())
		Expect(guardEntries(root)).To(BeEmpty())
	})
	// per docs/adr/0093-runtime-transition-guard.md:88
	It("scans no more than one activity entry and owns no marker when exclusively inspecting an empty directory", func() {
		root := guardRoot()
		controls, _, labels, files := guardControls()
		calls := 0
		controls.readDir = func(_ context.Context, file *os.File, n int) ([]os.DirEntry, error) {
			calls++
			Expect(labels[file]).To(Equal("runtime-activity"))
			Expect(n).To(Equal(1))
			return file.ReadDir(n)
		}
		g, err := acquire(context.Background(), root, true, controls)
		Expect(err).NotTo(HaveOccurred())
		Expect(calls).To(Equal(1))
		Expect(guardEntries(root)).To(BeEmpty())
		Expect(g.Close(context.Background(), true)).To(Succeed())
		guardClosed(*files)
	})
	// per docs/adr/0093-runtime-transition-guard.md:89
	DescribeTable("refuses any activity evidence without reading, deleting or repairing it", func(kind string) {
		root := guardRoot()
		Expect(os.MkdirAll(filepath.Join(root, ".factory/runtime-activity"), 0700)).To(Succeed())
		path := filepath.Join(root, ".factory/runtime-activity/unknown")
		switch kind {
		case "empty":
			Expect(os.WriteFile(path, nil, 0600)).To(Succeed())
		case "malformed":
			Expect(os.WriteFile(path, []byte("PRIVATE_ACTIVITY"), 0644)).To(Succeed()) // #nosec G306 -- deliberate public-mode evidence in a test-owned directory qualifies refusal without permission repair.
		case "symlink":
			Expect(os.Symlink("PRIVATE_TARGET", path)).To(Succeed())
		case "fifo":
			Expect(unix.Mkfifo(path, 0600)).To(Succeed())
		case "directory":
			Expect(os.Mkdir(path, 0700)).To(Succeed())
		}
		before, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		controls, _, _, files := guardControls()
		actualOpen := controls.open
		controls.open = func(ctx context.Context, parent *os.File, name string, flags int, mode uint32) (*os.File, error) {
			Expect(name).NotTo(Equal("unknown"), "exclusive inspection must not open evidence")
			return actualOpen(ctx, parent, name, flags, mode)
		}
		g, err := acquire(context.Background(), root, true, controls)
		guardFailure(root, g, err)
		after, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		Expect(after.Mode()).To(Equal(before.Mode()))
		Expect(after.Size()).To(Equal(before.Size()))
		guardClosed(*files)
	}, Entry("empty retained marker", "empty"), Entry("malformed evidence", "malformed"), Entry("link", "symlink"), Entry("FIFO", "fifo"), Entry("directory", "directory"))
	// per docs/adr/0093-runtime-transition-guard.md:89
	It("refuses an unavailable activity scan and closes the exclusive descriptor", func() {
		root := guardRoot()
		controls, _, _, files := guardControls()
		called := false
		controls.readDir = func(_ context.Context, file *os.File, n int) ([]os.DirEntry, error) {
			called = true
			_, err := file.ReadDir(n)
			Expect(errors.Is(err, io.EOF)).To(BeTrue())
			return nil, unix.EIO
		}
		g, err := acquire(context.Background(), root, true, controls)
		Expect(called).To(BeTrue())
		guardFailure(root, g, err)
		guardClosed(*files)
		guardManualExclusive(root)
	})
})

var _ = Describe("Runtime transition acquisition durability", func() {
	// per docs/adr/0093-runtime-transition-guard.md:55
	DescribeTable("does not publish a guard after a sync failure and retains its visible activity", func(stage string) {
		root := guardRoot()
		controls, _, labels, files := guardControls()
		called := false
		sync := func(_ context.Context, file *os.File) error {
			Expect(file.Sync()).To(Succeed())
			if labels[file] == stage {
				called = true
				return errors.New("PRIVATE_SYNC_FAILURE")
			}
			return nil
		}
		controls.syncFile = sync
		controls.syncDirectory = sync
		g, err := acquire(context.Background(), root, false, controls)
		Expect(called).To(BeTrue())
		guardFailure(root, g, err)
		Expect(guardEntries(root)).To(HaveLen(1))
		guardClosed(*files)
		guardManualExclusive(root)
	}, Entry("marker durability", "marker"), Entry("lock durability", "runtime-transition.lock"), Entry("activity directory durability", "runtime-activity"), Entry("state directory durability", ".factory"), Entry("project directory durability", "project"))
	// per docs/adr/0093-runtime-transition-guard.md:37
	DescribeTable("reports a failed opening without leaking earlier descriptors", func(stage string) {
		root := guardRoot()
		controls, _, labels, files := guardControls()
		actualOpen := controls.open
		called := false
		controls.open = func(ctx context.Context, parent *os.File, name string, flags int, mode uint32) (*os.File, error) {
			label := name
			if parent == nil {
				label = "project"
			}
			if labels[parent] == "runtime-activity" {
				label = "marker"
			}
			if label == stage {
				called = true
				return nil, errors.New("PRIVATE_OPEN_FAILURE")
			}
			return actualOpen(ctx, parent, name, flags, mode)
		}
		g, err := acquire(context.Background(), root, false, controls)
		Expect(called).To(BeTrue())
		guardFailure(root, g, err)
		guardClosed(*files)
	}, Entry("root opening", "project"), Entry("state opening", ".factory"), Entry("lock opening", "runtime-transition.lock"), Entry("activity opening", "runtime-activity"), Entry("marker opening", "marker"))
	// per docs/adr/0093-runtime-transition-guard.md:35
	DescribeTable("rejects an unreadable control descriptor instead of assuming its empty content", func(stage string) {
		root := guardRoot()
		controls, _, labels, files := guardControls()
		called := false
		controls.read = func(_ context.Context, file *os.File, data []byte) (int, error) {
			n, err := file.Read(data)
			if labels[file] == stage {
				called = true
				Expect(n).To(BeZero())
				Expect(errors.Is(err, io.EOF)).To(BeTrue())
				return 0, unix.EIO
			}
			return n, err
		}
		g, err := acquire(context.Background(), root, false, controls)
		Expect(called).To(BeTrue())
		guardFailure(root, g, err)
		guardClosed(*files)
		if stage == "marker" {
			Expect(guardEntries(root)).To(HaveLen(1))
		}
	}, Entry("lock readback", "runtime-transition.lock"), Entry("marker readback", "marker"))
	// per docs/adr/0093-runtime-transition-guard.md:55
	It("retains a marker and closes all handles if cancellation arrives after its actual sync", func() {
		root := guardRoot()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		controls, _, labels, files := guardControls()
		called := false
		controls.syncFile = func(_ context.Context, file *os.File) error {
			err := file.Sync()
			if labels[file] == "marker" {
				called = true
				cancel()
			}
			return err
		}
		g, err := acquire(ctx, root, false, controls)
		Expect(called).To(BeTrue())
		Expect(g).To(BeNil())
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		Expect(guardEntries(root)).To(HaveLen(1))
		guardClosed(*files)
	})
	// per docs/adr/0093-runtime-transition-guard.md:134
	It("does not open or create infrastructure for an already ended operation", func() {
		root := guardRoot()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		controls := ops{open: func(context.Context, *os.File, string, int, uint32) (*os.File, error) {
			Fail("ended context must not open storage")
			return nil, unix.EIO
		}}
		g, err := acquire(ctx, root, false, controls)
		Expect(g).To(BeNil())
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		entries, err := os.ReadDir(root)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	})
	// per docs/adr/0093-runtime-transition-guard.md:36
	// per docs/adr/0093-runtime-transition-guard.md:49
	DescribeTable("rejects a real original-path replacement before publishing a shared guard", func(stage string) {
		root := guardRoot()
		controls, _, labels, files := guardControls()
		called := false
		retained := ""
		controls.syncFile = func(_ context.Context, file *os.File) error {
			err := file.Sync()
			if !called && labels[file] == "marker" {
				called = true
				entries := guardEntries(root)
				Expect(entries).To(HaveLen(1))
				marker := filepath.Join(root, ".factory/runtime-activity", entries[0].Name())
				target := root
				switch stage {
				case "state":
					target = filepath.Join(root, ".factory")
				case "activity":
					target = filepath.Join(root, ".factory/runtime-activity")
				case "lock":
					target = filepath.Join(root, ".factory/runtime-transition.lock")
				case "marker":
					target = marker
				}
				info, statErr := os.Lstat(target)
				Expect(statErr).NotTo(HaveOccurred())
				Expect(os.Rename(target, target+".held")).To(Succeed())
				if stage == "project" {
					DeferCleanup(os.RemoveAll, root+".held")
				}
				if info.IsDir() {
					Expect(os.Mkdir(target, info.Mode().Perm())).To(Succeed())
				} else {
					Expect(os.WriteFile(target, nil, 0600)).To(Succeed())
				}
				after, statErr := os.Lstat(target)
				Expect(statErr).NotTo(HaveOccurred())
				Expect(os.SameFile(info, after)).To(BeFalse())
				retained = marker
				switch stage {
				case "project":
					retained = filepath.Join(root+".held", ".factory/runtime-activity", entries[0].Name())
				case "state":
					retained = filepath.Join(root, ".factory.held/runtime-activity", entries[0].Name())
				case "activity":
					retained = filepath.Join(root, ".factory/runtime-activity.held", entries[0].Name())
				case "marker":
					retained = marker + ".held"
				}
			}
			return err
		}
		g, err := acquire(context.Background(), root, false, controls)
		Expect(called).To(BeTrue())
		guardFailure(root, g, err)
		_, err = os.Lstat(retained)
		Expect(err).NotTo(HaveOccurred(), "original evidence must survive rejected path replacement")
		guardClosed(*files)
	}, Entry("project inode", "project"), Entry("state inode", "state"), Entry("activity inode", "activity"), Entry("permanent lock inode", "lock"), Entry("marker inode", "marker"))
})

var _ = Describe("Runtime transition qualified cleanup", func() {
	// per docs/adr/0093-runtime-transition-guard.md:79
	// per docs/adr/0093-runtime-transition-guard.md:83
	DescribeTable("reports cleanup faults without discarding unqualified evidence or recreating qualified removed evidence", func(stage string) {
		root := guardRoot()
		controls, _, labels, files := guardControls()
		g, err := acquire(context.Background(), root, false, controls)
		Expect(err).NotTo(HaveOccurred())
		before := guardEntries(root)
		Expect(before).To(HaveLen(1))
		called := false
		controls = g.operations
		controls.close = func(file *os.File) error {
			err := file.Close()
			if stage == "marker close" && labels[file] == "marker" || stage == "project close" && labels[file] == "project" {
				called = true
				return errors.Join(err, errors.New("PRIVATE_CLOSE_FAILURE"))
			}
			return err
		}
		controls.unlink = func(_ context.Context, parent *os.File, name string) error {
			if stage == "unlink" {
				called = true
				return errors.New("PRIVATE_UNLINK_FAILURE")
			}
			return unix.Unlinkat(int(parent.Fd()), name, 0)
		}
		controls.syncDirectory = func(_ context.Context, file *os.File) error {
			err := file.Sync()
			if stage == "removed directory sync" && labels[file] == "runtime-activity" {
				called = true
				return errors.Join(err, errors.New("PRIVATE_REMOVAL_SYNC_FAILURE"))
			}
			return err
		}
		g.operations = controls
		err = g.Close(context.Background(), stage != "project close")
		Expect(called).To(BeTrue())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).NotTo(ContainSubstring("PRIVATE_"))
		Expect(err.Error()).NotTo(ContainSubstring(root))
		guardClosed(*files)
		entries := guardEntries(root)
		if stage == "removed directory sync" {
			Expect(entries).To(BeEmpty(), "post-unlink sync failure does not invent replacement evidence")
		} else {
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Name()).To(Equal(before[0].Name()))
		}
	}, Entry("owned marker close", "marker close"), Entry("owned unlink", "unlink"), Entry("post-qualified-unlink sync", "removed directory sync"), Entry("conservative close failure", "project close"))
	// per docs/adr/0093-runtime-transition-guard.md:79
	DescribeTable("never deletes a replacement marker or repairs its changed private mode", func(replace bool) {
		root := guardRoot()
		g, err := Shared(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		entries := guardEntries(root)
		Expect(entries).To(HaveLen(1))
		path := filepath.Join(root, ".factory/runtime-activity", entries[0].Name())
		before, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		if replace {
			Expect(os.Rename(path, path+".held")).To(Succeed())
			Expect(os.WriteFile(path, []byte("PRIVATE_REPLACEMENT"), 0600)).To(Succeed())
		} else {
			Expect(os.Chmod(path, 0644)).To(Succeed())
		}
		err = g.Close(context.Background(), true)
		Expect(err).To(HaveOccurred())
		after, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		if replace {
			Expect(os.SameFile(before, after)).To(BeFalse())
			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(Equal("PRIVATE_REPLACEMENT"))
			_, err = os.Lstat(path + ".held")
			Expect(err).NotTo(HaveOccurred())
		} else {
			Expect(after.Mode().Perm()).To(Equal(os.FileMode(0644)))
		}
	}, Entry("foreign inode at marker pathname", true), Entry("changed marker mode", false))
	// per docs/adr/0093-runtime-transition-guard.md:44
	It("retains evidence when the state parent becomes writable by another principal", func() {
		root := guardRoot()
		g, err := Shared(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Chmod(filepath.Join(root, ".factory"), 0770)).To(Succeed())
		Expect(g.Close(context.Background(), true)).NotTo(Succeed())
		Expect(guardEntries(root)).To(HaveLen(1))
		info, err := os.Stat(filepath.Join(root, ".factory"))
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0770)))
	})
})

// Keep unused error strings out of reported storage failures; real native modes
// cannot always be represented by an unprivileged host. per docs/adr/0093-runtime-transition-guard.md:35
var _ = Describe("Runtime transition native control policy", func() {
	// per docs/adr/0093-runtime-transition-guard.md:35
	DescribeTable("refuses native special bits even with an otherwise exact private permission", func(kind string) {
		stat := unix.Stat_t{Mode: unix.S_IFREG | 0600, Nlink: 1}
		stat.Uid = uint32(os.Geteuid())
		switch kind {
		case "setuid":
			stat.Mode |= unix.S_ISUID
		case "setgid":
			stat.Mode |= unix.S_ISGID
		case "sticky":
			stat.Mode |= unix.S_ISVTX
		}
		Expect(safe(stat, "control")).To(BeFalse())
	}, Entry("setuid", "setuid"), Entry("setgid", "setgid"), Entry("sticky", "sticky"))

	// per docs/adr/0093-runtime-transition-guard.md:46
	DescribeTable("refuses a final project link or textual traversal while permitting no repair", func(kind string) {
		root := guardRoot()
		var path string
		if kind == "link" {
			path = root + ".link"
			Expect(os.Symlink(root, path)).To(Succeed())
			DeferCleanup(os.Remove, path)
		} else {
			path = filepath.Dir(root) + "/unused/../" + filepath.Base(root)
		}
		g, err := Shared(context.Background(), path)
		guardFailure(path, g, err)
		entries, err := os.ReadDir(root)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	}, Entry("physical root link", "link"), Entry("unclean traversal", "traversal"))
})
