package assessment

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAssessment(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Installation assessment collaborators")
}
func assessmentReadFixture() (string, string) {
	GinkgoHelper()
	root := GinkgoT().TempDir()
	physical, err := filepath.EvalSymlinks(root)
	Expect(err).NotTo(HaveOccurred())
	root = physical
	path := filepath.Join(root, "scripts/factory-budget.sh")
	Expect(os.MkdirAll(filepath.Dir(path), 0700)).To(Succeed())
	Expect(os.WriteFile(path, []byte(strings.Repeat("PRIVATE_READ_PAYLOAD", 4096)), 0600)).To(Succeed())
	return root, path
}

var _ = Describe("Installation assessment controlled observation", func() {
	// per docs/adr/0079-go-installation-reference-assessment.md:65
	DescribeTable("classifies deterministic filesystem identity changes as unsafe", func(kind string) {
		root, path := assessmentReadFixture()
		changed := false
		hook := func(file *os.File, buffer []byte) (int, error) {
			if !changed {
				changed = true
				switch kind {
				case "content growth":
					f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
					Expect(err).NotTo(HaveOccurred())
					_, err = f.WriteString("changed")
					Expect(err).NotTo(HaveOccurred())
					Expect(f.Close()).To(Succeed())
				case "mode":
					Expect(os.Chmod(path, 0640)).To(Succeed())
				case "mtime":
					Expect(os.Chtimes(path, time.Unix(1, 0), time.Unix(1, 0))).To(Succeed())
				case "leaf replacement":
					Expect(os.Rename(path, path+".held")).To(Succeed())
					Expect(os.WriteFile(path, []byte("replacement"), 0600)).To(Succeed())
				case "ancestor replacement":
					scripts := filepath.Join(root, "scripts")
					Expect(os.Rename(scripts, scripts+".held")).To(Succeed())
					Expect(os.Mkdir(scripts, 0700)).To(Succeed())
					Expect(os.WriteFile(path, []byte("replacement"), 0600)).To(Succeed())
				case "hardlink added":
					Expect(os.Link(path, path+".link")).To(Succeed())
				}
			}
			return file.Read(buffer)
		}
		result, err := assess(context.Background(), root, ops{read: hook})
		Expect(err).NotTo(HaveOccurred())
		Expect(changed).To(BeTrue())
		Expect(result.Status()).To(Equal(2))
		Expect(result.Assets[0].Classification).To(Equal("unsafe"))
		Expect(result.Assets[0].Observed).To(BeNil())
	}, Entry("content growth", "content growth"), Entry("mode change", "mode"), Entry("mtime change", "mtime"), Entry("leaf replacement", "leaf replacement"), Entry("ancestor replacement", "ancestor replacement"), Entry("hardlink added", "hardlink added"))
	// per docs/adr/0079-go-installation-reference-assessment.md:66
	DescribeTable("sanitizes controlled I/O errors and closes each opened leaf", func(operation string) {
		root, _ := assessmentReadFixture()
		var opened []*os.File
		controls := ops{open: func(parent *os.File, name string, flags int) (*os.File, error) {
			if operation == "open" {
				return nil, syscall.EACCES
			}
			file, err := openAt(parent, name, flags)
			if file != nil {
				opened = append(opened, file)
			}
			return file, err
		}, read: func(file *os.File, buffer []byte) (int, error) {
			n, err := file.Read(buffer)
			if err != nil {
				return n, err
			}
			return n, errors.New("PRIVATE_IO_DIAGNOSTIC")
		}}
		result, err := assess(context.Background(), root, controls)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status()).To(Equal(1))
		Expect(result.Assets[0].Classification).To(Equal("assessment_error"))
		Expect(result.Assets[0].Observed).To(BeNil())
		output, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(output)).NotTo(ContainSubstring("PRIVATE_"))
		for _, file := range opened {
			_, err := file.Stat()
			Expect(errors.Is(err, os.ErrClosed)).To(BeTrue())
		}
	}, Entry("opening leaf", "open"), Entry("reading after bytes", "read"))
	// per docs/adr/0079-go-installation-reference-assessment.md:92
	It("discards the complete assessment if the named root is replaced during reading", func() {
		root, _ := assessmentReadFixture()
		changed := false
		result, err := assess(context.Background(), root, ops{read: func(file *os.File, buffer []byte) (int, error) {
			if !changed {
				changed = true
				held := root + "-held"
				Expect(os.Rename(root, held)).To(Succeed())
				DeferCleanup(os.RemoveAll, held)
				Expect(os.Mkdir(root, 0700)).To(Succeed())
			}
			return file.Read(buffer)
		}})
		Expect(changed).To(BeTrue())
		Expect(err).To(HaveOccurred())
		Expect(ErrorStatus(err)).To(Equal(2))
		Expect(result.Assets).To(BeNil())
	})
	// per docs/adr/0079-go-installation-reference-assessment.md:90
	It("bounds each read to32KiB while observing complete ordinary file bytes", func() {
		root, path := assessmentReadFixture()
		calls := 0
		result, err := assess(context.Background(), root, ops{read: func(file *os.File, buffer []byte) (int, error) {
			calls++
			Expect(len(buffer)).To(BeNumerically("<=", 32<<10))
			return file.Read(buffer)
		}})
		Expect(err).NotTo(HaveOccurred())
		Expect(calls).To(BeNumerically(">", 1))
		info, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Assets[0].Observed.Bytes).To(Equal(info.Size()))
	})
	// per docs/adr/0079-go-installation-reference-assessment.md:76
	It("preserves mid-read cancellation without returning a partial report or open leaf", func() {
		root, _ := assessmentReadFixture()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var opened *os.File
		result, err := assess(ctx, root, ops{read: func(file *os.File, buffer []byte) (int, error) {
			opened = file
			n, err := file.Read(buffer)
			cancel()
			return n, err
		}})
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		Expect(result.Assets).To(BeNil())
		Expect(opened).NotTo(BeNil())
		_, statErr := opened.Stat()
		Expect(errors.Is(statErr, os.ErrClosed)).To(BeTrue())
	})
	// per docs/adr/0079-go-installation-reference-assessment.md:81
	It("refuses embedded NUL before opening any leaf", func() {
		called := false
		result, err := assess(context.Background(), "bad\x00root", ops{open: func(*os.File, string, int) (*os.File, error) { called = true; return nil, syscall.EIO }})
		Expect(err).To(HaveOccurred())
		Expect(ErrorStatus(err)).To(Equal(2))
		Expect(called).To(BeFalse())
		Expect(result.Assets).To(BeNil())
	})
})
