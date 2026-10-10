package installation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/anoop2811/software-factory-template/internal/installationfs"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:238
func TestInstallationObservation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Whole installation observation causes")
}

func observationErrorFixture() (string, *installationfs.Tree) {
	GinkgoHelper()
	root, err := os.MkdirTemp("", "factory-observation-native-cause-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, root)
	root, err = filepath.EvalSymlinks(root)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.Mkdir(filepath.Join(root, "scripts"), 0700)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(root, "scripts/asset"), []byte("actual native before-image\n"), 0600)).To(Succeed())
	tree, err := installationfs.Open(context.Background(), root)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(tree.Close)
	return root, tree
}

var _ = Describe("Native installation observation operational causes", func() {
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:56
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:58
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:387
	DescribeTable("preserves a reported cause after actual native metadata observation", func(canceled bool) {
		_, tree := observationErrorFixture()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		healthy, contents, err := observe(ctx, tree, "scripts/asset", 1<<20)
		Expect(err).NotTo(HaveOccurred())
		Expect(healthy).NotTo(BeNil())
		Expect(string(contents)).To(Equal("actual native before-image\n"))
		Expect(ErrorStatus(err)).To(Equal(0))
		calls := 0
		cause := error(unix.EIO)
		if canceled {
			cause = context.Canceled
		}
		operations := observeOps{statFile: func(file *os.File) (unix.Stat_t, error) {
			var stat unix.Stat_t
			nativeError := unix.Fstat(int(file.Fd()), &stat)
			Expect(nativeError).NotTo(HaveOccurred(), "the collaborator must really execute native metadata observation")
			calls++
			if calls == 2 {
				if canceled {
					cancel()
				}
				return stat, errors.Join(nativeError, cause)
			}
			return stat, nativeError
		}}
		image, saved, err := observeWith(ctx, tree, "scripts/asset", 1<<20, operations)
		Expect(calls).To(Equal(2))
		Expect(image).To(BeNil())
		Expect(saved).To(BeNil())
		Expect(errors.Is(err, cause)).To(BeTrue(), "observed native cause was discarded: %v", err)
		Expect(ErrorStatus(err)).To(Equal(1), "operational cause cannot be reported as preserved conflict")
	}, Entry("native EIO after final fstat", false), Entry("real cancellation after final fstat", true))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:184
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:382
	It("retains policy refusal for a changed parent after real observation and preserves both trees", func() {
		root, tree := observationErrorFixture()
		outside, err := os.MkdirTemp("", "factory-observation-foreign-parent-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, outside)
		foreign := []byte("foreign tree must stay untouched\n")
		Expect(os.WriteFile(filepath.Join(outside, "asset"), foreign, 0600)).To(Succeed())
		healthy, _, err := observe(context.Background(), tree, "scripts/asset", 1<<20)
		Expect(err).NotTo(HaveOccurred())
		Expect(healthy).NotTo(BeNil())
		calls := 0
		operations := observeOps{statFile: func(file *os.File) (unix.Stat_t, error) {
			var stat unix.Stat_t
			nativeError := unix.Fstat(int(file.Fd()), &stat)
			Expect(nativeError).NotTo(HaveOccurred())
			calls++
			if calls == 2 {
				Expect(os.Rename(filepath.Join(root, "scripts"), filepath.Join(root, "original-scripts"))).To(Succeed())
				Expect(os.Symlink(outside, filepath.Join(root, "scripts"))).To(Succeed())
			}
			return stat, nativeError
		}}
		image, saved, err := observeWith(context.Background(), tree, "scripts/asset", 1<<20, operations)
		Expect(calls).To(Equal(2))
		Expect(image).To(BeNil())
		Expect(saved).To(BeNil())
		Expect(ErrorStatus(err)).To(Equal(2))
		original, err := os.ReadFile(filepath.Join(root, "original-scripts/asset"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(original)).To(Equal("actual native before-image\n"))
		actualForeign, err := os.ReadFile(filepath.Join(outside, "asset"))
		Expect(err).NotTo(HaveOccurred())
		Expect(actualForeign).To(Equal(foreign))
	})
})
