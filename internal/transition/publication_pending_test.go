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

func pendingGuardNamed(_ context.Context, parent *os.File, name string) (unix.Stat_t, error) {
	fd := unix.AT_FDCWD
	if parent != nil {
		fd = int(parent.Fd())
	}
	var stat unix.Stat_t
	err := unix.Fstatat(fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	return stat, err
}
func pendingGuardWrite(root string) os.FileInfo {
	GinkgoHelper()
	path := filepath.Join(root, ".factory/runtime-publication.pending")
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	Expect(err).NotTo(HaveOccurred())
	Expect(file.Sync()).To(Succeed())
	Expect(file.Close()).To(Succeed())
	info, err := os.Lstat(path)
	Expect(err).NotTo(HaveOccurred())
	return info
}

var _ = Describe("Live publication guard inspection boundaries", func() {
	// per docs/adr/0094-live-publication-restoration.md:61
	// per docs/adr/0094-live-publication-restoration.md:62
	DescribeTable("fails bounded pending inspection under the actual held lock and closes descriptors", func(exclusive bool) {
		root := guardRoot()
		controls, _, labels, files := guardControls()
		locked, inspected := false, false
		controls.flock = func(_ context.Context, file *os.File, mode int) error {
			err := unix.Flock(int(file.Fd()), mode)
			if err == nil {
				locked = true
			}
			return err
		}
		controls.named = func(ctx context.Context, parent *os.File, name string) (unix.Stat_t, error) {
			if name == "runtime-publication.pending" {
				Expect(locked).To(BeTrue(), "pending must be inspected under the permanent held lock")
				Expect(labels[parent]).To(Equal(".factory"))
				_, actualErr := pendingGuardNamed(ctx, parent, name)
				Expect(errors.Is(actualErr, unix.ENOENT)).To(BeTrue())
				inspected = true
				return unix.Stat_t{}, unix.EIO
			}
			return pendingGuardNamed(ctx, parent, name)
		}
		guard, err := acquire(context.Background(), root, exclusive, controls)
		if guard != nil {
			DeferCleanup(func() { _ = guard.Close(context.Background(), false) })
		}
		Expect(inspected).To(BeTrue(), "a real absent-entry query must reach the injected inspection failure")
		guardFailure(root, guard, err)
		guardClosed(*files)
		guardManualExclusive(root)
	}, Entry("shared admission", false), Entry("fresh exclusive admission", true))

	// per docs/adr/0094-live-publication-restoration.md:62
	DescribeTable("refuses pending created after flock or during the last real durability sync", func(exclusive bool, phase string) {
		root := guardRoot()
		controls, _, labels, files := guardControls()
		var created os.FileInfo
		controls.flock = func(_ context.Context, file *os.File, mode int) error {
			err := unix.Flock(int(file.Fd()), mode)
			if err == nil && phase == "flock" {
				created = pendingGuardWrite(root)
			}
			return err
		}
		controls.syncDirectory = func(_ context.Context, file *os.File) error {
			err := file.Sync()
			if err == nil && labels[file] == "project" && phase == "final sync" {
				created = pendingGuardWrite(root)
			}
			return err
		}
		guard, err := acquire(context.Background(), root, exclusive, controls)
		if guard != nil {
			DeferCleanup(func() { _ = guard.Close(context.Background(), false) })
		}
		Expect(created).NotTo(BeNil(), "the genuine lifetime mutation must occur")
		guardFailure(root, guard, err)
		current, statErr := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(statErr).NotTo(HaveOccurred())
		Expect(os.SameFile(created, current)).To(BeTrue())
		Expect(current.Mode().Perm()).To(Equal(os.FileMode(0600)))
		Expect(current.Size()).To(BeZero())
		guardClosed(*files)
		guardManualExclusive(root)
	}, Entry("shared after flock", false, "flock"), Entry("exclusive after flock", true, "flock"),
		Entry("shared before return", false, "final sync"), Entry("exclusive before return", true, "final sync"))
})
