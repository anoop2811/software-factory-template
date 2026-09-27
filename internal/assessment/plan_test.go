package assessment

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Migration planning controlled observation", func() {
	// per docs/adr/0080-go-migration-action-planning.md:38
	DescribeTable("holds both roots until final revalidation while the opposite tree is read", func(replaced string) {
		installed, _ := assessmentReadFixture()
		source, _ := assessmentReadFixture()
		changed := false
		var opened []*os.File
		opening := func(parent *os.File, name string, flags int) (*os.File, error) {
			file, err := openAt(parent, name, flags)
			if file != nil {
				opened = append(opened, file)
			}
			return file, err
		}
		hook := func(file *os.File, buffer []byte) (int, error) {
			if !changed {
				changed = true
				root := installed
				if replaced == "source" {
					root = source
				}
				held := root + "-held"
				Expect(os.Rename(root, held)).To(Succeed())
				DeferCleanup(os.RemoveAll, held)
				Expect(os.Mkdir(root, 0700)).To(Succeed())
			}
			return file.Read(buffer)
		}
		installedOps, sourceOps := ops{open: opening}, ops{open: opening}
		if replaced == "installed" {
			sourceOps.read = hook
		} else {
			installedOps.read = hook
		}
		result, err := plan(context.Background(), installed, source, installedOps, sourceOps)
		Expect(changed).To(BeTrue())
		Expect(err).To(HaveOccurred())
		Expect(ErrorStatus(err)).To(Equal(2))
		Expect(result.Assets).To(BeNil())
		for _, file := range opened {
			_, err := file.Stat()
			Expect(errors.Is(err, os.ErrClosed)).To(BeTrue())
		}
	}, Entry("installed replaced during source read", "installed"), Entry("source replaced during installed read", "source"))
	// per docs/adr/0080-go-migration-action-planning.md:66
	DescribeTable("records source I/O failure with precedence and sanitized observations", func(operation string) {
		installed, _ := assessmentReadFixture()
		source, _ := assessmentReadFixture()
		var opened *os.File
		controls := ops{open: func(parent *os.File, name string, flags int) (*os.File, error) {
			if operation == "open" {
				return nil, syscall.EIO
			}
			file, err := openAt(parent, name, flags)
			opened = file
			return file, err
		}, read: func(file *os.File, buffer []byte) (int, error) {
			n, err := file.Read(buffer)
			if err != nil {
				return n, err
			}
			return n, errors.New("PRIVATE_TARGET_IO_ERROR")
		}}
		result, err := plan(context.Background(), installed, source, ops{}, controls)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status()).To(Equal(1))
		Expect(result.Assets).To(HaveLen(6))
		Expect(result.Assets[0].Action).To(Equal("assessment_error"))
		Expect(result.Assets[0].Reason).To(Equal("cannot_assess_path"))
		Expect(result.Assets[0].Source.Classification).To(Equal("assessment_error"))
		Expect(result.Assets[0].Source.Observed).To(BeNil())
		output, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(output)).NotTo(ContainSubstring("PRIVATE_"))
		if opened != nil {
			_, err := opened.Stat()
			Expect(errors.Is(err, os.ErrClosed)).To(BeTrue())
		}
	}, Entry("source open", "open"), Entry("source read after bytes", "read"))
	// per docs/adr/0080-go-migration-action-planning.md:119
	DescribeTable("preserves cancellation from either tree with no partial plan", func(side string) {
		installed, _ := assessmentReadFixture()
		source, _ := assessmentReadFixture()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var opened *os.File
		hook := func(file *os.File, buffer []byte) (int, error) {
			opened = file
			n, err := file.Read(buffer)
			cancel()
			return n, err
		}
		installedOps, sourceOps := ops{}, ops{}
		if side == "source" {
			sourceOps.read = hook
		} else {
			installedOps.read = hook
		}
		result, err := plan(ctx, installed, source, installedOps, sourceOps)
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		Expect(result.Assets).To(BeNil())
		Expect(opened).NotTo(BeNil())
		_, statErr := opened.Stat()
		Expect(errors.Is(statErr, os.ErrClosed)).To(BeTrue())
	}, Entry("installed", "installed"), Entry("source", "source"))
})
