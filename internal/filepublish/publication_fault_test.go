package filepublish

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func publicationStageFixture() (string, *os.Root) {
	GinkgoHelper()
	path := GinkgoT().TempDir()
	root, err := os.OpenRoot(path)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { Expect(root.Close()).To(Succeed()) })
	Expect(root.WriteFile("existing", []byte("ORIGINAL_LEAF"), 0600)).To(Succeed())
	return path, root
}

var _ = Describe("Live publication shared staging faults", func() {
	// per docs/adr/0094-live-publication-restoration.md:70
	// per docs/adr/0094-live-publication-restoration.md:72
	// per docs/adr/0094-live-publication-restoration.md:126
	DescribeTable("never returns a publishable sibling after genuine write, readback, sync, close or cancellation failure", func(phase string) {
		path, root := publicationStageFixture()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		called := false
		var files []*os.File
		capture := func(file *os.File) {
			for _, old := range files {
				if old == file {
					return
				}
			}
			files = append(files, file)
		}
		controls := ops{
			write: func(_ context.Context, file *os.File, data []byte) (int, error) {
				capture(file)
				if phase == "partial write" {
					called = true
					return file.Write(data[:len(data)/2])
				}
				n, err := file.Write(data)
				if phase == "write EIO" {
					called = true
					return n, errors.New("PRIVATE_REAL_WRITE_FAILURE")
				}
				if phase == "cancellation" {
					called = true
					cancel()
				}
				return n, err
			},
			read: func(_ context.Context, file *os.File, data []byte) (int, error) {
				capture(file)
				n, err := file.Read(data)
				if n > 0 && phase == "readback EIO" {
					called = true
					return n, errors.New("PRIVATE_READBACK_FAILURE")
				}
				if n > 0 && phase == "readback mismatch" {
					called = true
					data[0] ^= 1
				}
				return n, err
			},
			syncFile: func(_ context.Context, file *os.File) error {
				capture(file)
				err := file.Sync()
				if phase == "file sync" {
					called = true
					return errors.Join(err, errors.New("PRIVATE_SYNC_FAILURE"))
				}
				return err
			},
			close: func(file *os.File) error {
				capture(file)
				err := file.Close()
				if phase == "close" {
					called = true
					return errors.Join(err, errors.New("PRIVATE_CLOSE_FAILURE"))
				}
				return err
			},
		}
		stage, err := prepare(ctx, root, ".publication-", []byte("COMPLETE_REPLACEMENT"), 0755, controls)
		Expect(called).To(BeTrue(), "the selected real-operation fault must be reached")
		Expect(err).To(HaveOccurred())
		if phase == "partial write" {
			Expect(errors.Is(err, io.ErrShortWrite)).To(BeTrue())
		}
		Expect(stage).To(BeNil())
		Expect(files).NotTo(BeEmpty())
		for _, file := range files {
			_, statErr := file.Stat()
			Expect(errors.Is(statErr, os.ErrClosed)).To(BeTrue())
		}
		existing, readErr := root.ReadFile("existing")
		Expect(readErr).NotTo(HaveOccurred())
		Expect(existing).To(Equal([]byte("ORIGINAL_LEAF")))
		entries, readErr := os.ReadDir(path)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1), "a failed prepared sibling is never published or leaked")
		Expect(entries[0].Name()).To(Equal("existing"))
	}, Entry("real partial write", "partial write"), Entry("actual write then EIO", "write EIO"),
		Entry("actual file sync then failure", "file sync"), Entry("actual readback then EIO", "readback EIO"),
		Entry("actual readback mismatch", "readback mismatch"), Entry("actual close then failure", "close"),
		Entry("cancel after actual write", "cancellation"))

	// per docs/adr/0094-live-publication-restoration.md:75
	// per docs/adr/0094-live-publication-restoration.md:78
	It("exposes the actual prepared inode that becomes the named published leaf", func() {
		path, root := publicationStageFixture()
		stage, err := Prepare(context.Background(), root, ".publication-", []byte("REPLACEMENT"), 0755)
		Expect(err).NotTo(HaveOccurred())
		prepared, err := stage.OpenPrepared(context.Background())
		Expect(err).NotTo(HaveOccurred())
		before, err := prepared.Stat()
		Expect(err).NotTo(HaveOccurred())
		Expect(prepared.Close()).To(Succeed())
		Expect(stage.Publish(context.Background(), "existing")).To(Succeed())
		after, err := os.Lstat(filepath.Join(path, "existing"))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		Expect(after.Mode()).To(Equal(os.FileMode(0755)))
		Expect(root.ReadFile("existing")).To(Equal([]byte("REPLACEMENT")))
		Expect(stage.Cleanup()).To(Succeed())
		Expect(root.ReadFile("existing")).To(Equal([]byte("REPLACEMENT")))
	})
})
