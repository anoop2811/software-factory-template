package installationfs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/anoop2811/software-factory-template/internal/installationfs"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRetentionDirectoryCustody(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Retention directory custody")
}

type checkedDirectoryRemover interface {
	RemoveEmptyDirectory(context.Context, string, installationfs.Directory) error
}

func directoryObservation(tree *installationfs.Tree, path string) installationfs.Directory {
	GinkgoHelper()
	_, _, _, err := tree.Parent(context.Background(), path+"/observation", false)
	Expect(err).NotTo(HaveOccurred())
	parents, err := tree.Parents(context.Background())
	Expect(err).NotTo(HaveOccurred())
	for _, parent := range parents {
		if parent.Path == path {
			return parent
		}
	}
	Fail("the checked parent observation must contain the requested directory")
	return installationfs.Directory{}
}

// per docs/adr/0106-go-backup-retention.md:82
var _ = Describe("Checked postorder recovery directory removal", func() {
	var root string
	var tree *installationfs.Tree
	var remover checkedDirectoryRemover
	BeforeEach(func() {
		var err error
		root, err = filepath.EvalSymlinks(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())
		Expect(os.MkdirAll(filepath.Join(root, "backups/old/assets/nested"), 0700)).To(Succeed())
		tree, err = installationfs.Open(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(tree.Close()).To(Succeed()) })
		var ok bool
		remover, ok = any(tree).(checkedDirectoryRemover)
		Expect(ok).To(BeTrue(), "the existing confined tree must support checked empty-directory retirement")
	})

	// per docs/adr/0106-go-backup-retention.md:84
	It("removes only checked empty directories in postorder and keeps retained descriptor custody valid", func() {
		Expect(os.WriteFile(filepath.Join(root, "untouched"), []byte("operator data"), 0600)).To(Succeed())
		paths := []string{"backups/old/assets/nested", "backups/old/assets", "backups/old"}
		observations := map[string]installationfs.Directory{}
		for _, path := range paths {
			observations[path] = directoryObservation(tree, path)
		}
		for _, path := range paths {
			Expect(remover.RemoveEmptyDirectory(context.Background(), path, observations[path])).To(Succeed())
			_, err := os.Lstat(filepath.Join(root, path))
			Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
			Expect(tree.Check(context.Background())).To(Succeed(), "expected removals must retire only their own cached descriptors")
		}
		Expect(os.ReadFile(filepath.Join(root, "untouched"))).To(Equal([]byte("operator data")))
		Expect(os.Stat(filepath.Join(root, "backups"))).NotTo(BeNil())
	})

	// per docs/adr/0106-go-backup-retention.md:84
	DescribeTable("preserves unsafe or unproven directories and external referents", func(change string) {
		path := "backups/old/assets/nested"
		expected := directoryObservation(tree, path)
		outside := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(outside, "sentinel"), []byte("external content"), 0600)).To(Succeed())
		ctx := context.Background()
		switch change {
		case "nonempty":
			Expect(os.WriteFile(filepath.Join(root, path, "new-file"), []byte("later file"), 0600)).To(Succeed())
		case "retained-child":
			Expect(os.Mkdir(filepath.Join(root, path, "child"), 0700)).To(Succeed())
			directoryObservation(tree, path+"/child")
		case "identity":
			expected.Inode++
		case "replacement":
			Expect(os.Rename(filepath.Join(root, path), filepath.Join(root, "original"))).To(Succeed())
			Expect(os.Mkdir(filepath.Join(root, path), 0700)).To(Succeed())
		case "symlink":
			Expect(os.Rename(filepath.Join(root, path), filepath.Join(root, "original"))).To(Succeed())
			Expect(os.Symlink(outside, filepath.Join(root, path))).To(Succeed())
		case "unsafe-mode":
			Expect(os.Chmod(filepath.Join(root, path), 0777)).To(Succeed()) // #nosec G302 -- intentionally unsafe temporary directory must be refused.
		case "root":
			path, expected.Path = ".", "."
		case "escape":
			path, expected.Path = "../outside", "../outside"
		case "canceled":
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(ctx)
			cancel()
		}
		err := remover.RemoveEmptyDirectory(ctx, path, expected)
		Expect(err).To(HaveOccurred())
		if change == "canceled" {
			Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		}
		Expect(os.ReadFile(filepath.Join(outside, "sentinel"))).To(Equal([]byte("external content")))
		if change != "root" && change != "escape" {
			_, err = os.Lstat(filepath.Join(root, path))
			Expect(err).NotTo(HaveOccurred(), "refusal must not remove the named unqualified entry")
		}
	}, Entry("nonempty directory", "nonempty"), Entry("retained cached child", "retained-child"), Entry("forged identity", "identity"), Entry("replaced inode", "replacement"), Entry("symlink referent", "symlink"), Entry("writable foreign boundary", "unsafe-mode"), Entry("installation root", "root"), Entry("parent escape", "escape"), Entry("canceled operation", "canceled"))
})
