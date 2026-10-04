package transition

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

func interruptedGuardRoot() string {
	GinkgoHelper()
	root := guardRoot()
	Expect(os.Mkdir(filepath.Join(root, ".factory"), 0700)).To(Succeed())
	Expect(os.Mkdir(filepath.Join(root, ".factory/runtime-activity"), 0700)).To(Succeed())
	for _, name := range []string{lockName, pendingName} {
		Expect(os.WriteFile(filepath.Join(root, ".factory", name), nil, 0600)).To(Succeed())
	}
	return root
}

type interruptedGuardEntry struct {
	info os.FileInfo
	data string
}

func interruptedGuardTree(root string) map[string]interruptedGuardEntry {
	GinkgoHelper()
	result := map[string]interruptedGuardEntry{}
	Expect(filepath.WalkDir(root, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entry := interruptedGuardEntry{info: info}
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			entry.data = string(data)
		} else if info.Mode()&os.ModeSymlink != 0 {
			entry.data, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		result[relative] = entry
		return nil
	})).To(Succeed())
	return result
}
func interruptedGuardPreserved(root string, before map[string]interruptedGuardEntry) {
	GinkgoHelper()
	after := interruptedGuardTree(root)
	Expect(after).To(HaveLen(len(before)))
	for path, entry := range before {
		actual, ok := after[path]
		Expect(ok).To(BeTrue(), path)
		Expect(os.SameFile(entry.info, actual.info)).To(BeTrue(), path)
		Expect(actual.info.Mode()).To(Equal(entry.info.Mode()), path)
		Expect(actual.info.ModTime()).To(Equal(entry.info.ModTime()), path)
		Expect(actual.data).To(Equal(entry.data), path)
	}
}

var _ = Describe("Interrupted recovery exclusive guard", func() {
	// per docs/adr/0096-interrupted-publication-recovery.md:100
	// per docs/adr/0096-interrupted-publication-recovery.md:108
	// per docs/adr/0096-interrupted-publication-recovery.md:111
	It("owns the same permanent exclusive inode while leaving pending resolution to its caller", func() {
		root := interruptedGuardRoot()
		before := interruptedGuardTree(root)
		guard, err := ExclusiveRecovery(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		Expect(guard).NotTo(BeNil())
		DeferCleanup(func() { _ = guard.Close(context.Background(), false) })
		interruptedGuardPreserved(root, before)
		file, err := os.OpenFile(filepath.Join(root, ".factory", lockName), os.O_RDWR, 0)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = file.Close() })
		Expect(unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)).NotTo(Succeed())
		for _, acquire := range []func(context.Context, string) (*Guard, error){Shared, Exclusive, ExclusiveRecovery} {
			other, err := acquire(context.Background(), root)
			Expect(err).To(HaveOccurred())
			Expect(other).To(BeNil())
		}
		Expect(guard.Check(context.Background())).To(Succeed())
		Expect(os.Remove(filepath.Join(root, ".factory", pendingName))).To(Succeed(), "only evaluator removes its known marker to qualify legitimate consumer cleanup")
		Expect(guard.Check(context.Background())).To(Succeed(), "the guard must not retain pending as an immutable lifetime control")
		Expect(guard.Close(context.Background(), false)).To(Succeed())
		Expect(unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)).To(Succeed())
		Expect(unix.Flock(int(file.Fd()), unix.LOCK_UN)).To(Succeed())
	})

	// per docs/adr/0096-interrupted-publication-recovery.md:103
	// per docs/adr/0096-interrupted-publication-recovery.md:106
	// per docs/adr/0096-interrupted-publication-recovery.md:109
	DescribeTable("refuses missing or unsafe existing evidence without creating or repairing infrastructure", func(change string) {
		root := interruptedGuardRoot()
		outside := filepath.Join(root, "outside")
		pending := filepath.Join(root, ".factory", pendingName)
		switch change {
		case "factory missing":
			Expect(os.RemoveAll(filepath.Join(root, ".factory"))).To(Succeed())
		case "lock missing":
			Expect(os.Remove(filepath.Join(root, ".factory", lockName))).To(Succeed())
		case "activity missing":
			Expect(os.Remove(filepath.Join(root, ".factory", activityName))).To(Succeed())
		case "pending missing":
			Expect(os.Remove(pending)).To(Succeed())
		case "pending bytes":
			Expect(os.WriteFile(pending, []byte("PRIVATE_PENDING"), 0600)).To(Succeed())
		case "pending public mode":
			Expect(os.Chmod(pending, 0644)).To(Succeed())
		case "pending hardlink":
			Expect(os.Link(pending, outside)).To(Succeed())
		case "pending symlink":
			Expect(os.Rename(pending, outside)).To(Succeed())
			Expect(os.Symlink(outside, pending)).To(Succeed())
		case "pending directory":
			Expect(os.Remove(pending)).To(Succeed())
			Expect(os.Mkdir(pending, 0700)).To(Succeed())
		case "activity evidence":
			Expect(os.WriteFile(filepath.Join(root, ".factory", activityName, "unresolved"), nil, 0600)).To(Succeed())
		case "activity mode":
			Expect(os.Chmod(filepath.Join(root, ".factory", activityName), 0755)).To(Succeed())
		case "lock mode":
			Expect(os.Chmod(filepath.Join(root, ".factory", lockName), 0644)).To(Succeed())
		}
		before := interruptedGuardTree(root)
		guard, err := ExclusiveRecovery(context.Background(), root)
		guardFailure(root, guard, err)
		interruptedGuardPreserved(root, before)
	}, Entry("missing state parent", "factory missing"), Entry("missing permanent lock", "lock missing"), Entry("missing activity directory", "activity missing"), Entry("missing pending", "pending missing"), Entry("pending is not empty", "pending bytes"), Entry("pending private mode refused", "pending public mode"), Entry("pending extra hard link", "pending hardlink"), Entry("pending outside symlink", "pending symlink"), Entry("pending is directory", "pending directory"), Entry("existing activity blocker", "activity evidence"), Entry("activity not private", "activity mode"), Entry("unsafe permanent lock", "lock mode"))

	// per docs/adr/0096-interrupted-publication-recovery.md:199
	// per docs/adr/0096-interrupted-publication-recovery.md:202
	It("releases retained descriptors and flock under canceled closure without deleting pending", func() {
		root := interruptedGuardRoot()
		controls, _, _, files := guardControls()
		guard, err := acquireRecovery(context.Background(), root, controls)
		Expect(err).NotTo(HaveOccurred())
		before := interruptedGuardTree(root)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		Expect(errors.Is(guard.Close(ctx, false), context.Canceled)).To(BeTrue())
		guardClosed(*files)
		guardManualExclusive(root)
		interruptedGuardPreserved(root, before)
	})
})
