package loop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Loop transition compound publication evidence", func() {
	// per docs/adr/0093-runtime-transition-guard.md:67
	// per docs/adr/0093-runtime-transition-guard.md:205
	// per docs/adr/0093-runtime-transition-guard.md:209
	DescribeTable("retains typed potentially committed checkpoint publication when guard cleanup also refuses changed storage", func(changeGuard bool) {
		controller, request := manualFixture()
		publications, executions := 0, 0
		activity := filepath.Join(controller.root, ".factory/runtime-activity")
		controller.store.ops.syncDirectory = func(file *os.File) error {
			publications++
			if err := file.Sync(); err != nil {
				return err
			}
			if changeGuard {
				if err := os.Chmod(activity, 0755); err != nil {
					return err
				}
			}
			return syscall.EIO
		}
		controller.ops.execute = func(context.Context, string, string, map[string]string, time.Duration, func(context.Context, int) error) (native.Execution, error) {
			executions++
			return native.Execution{}, errors.New("test-owned unexpected native execution")
		}
		result, err := controller.Run(context.Background(), request, false)
		Expect(publications).To(Equal(1), "initial active checkpoint really reached its post-rename directory sync")
		Expect(executions).To(BeZero())
		Expect(result.Record).To(BeNil())
		row := manualDurable(controller)
		Expect(row["status"]).To(Equal("active"))
		Expect(row["phase"]).To(Equal("starting"))
		Expect(row["process_pid"]).To(BeNil())
		entries, readErr := os.ReadDir(activity)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1), "neither failure may remove the actual durable activity evidence")
		info, statErr := os.Lstat(activity)
		Expect(statErr).NotTo(HaveOccurred())
		if changeGuard {
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0755)), "unsafe storage is preserved, not repaired")
		} else {
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0700)))
		}
		var publication *PublicationError
		Expect(errors.As(err, &publication)).To(BeTrue(), "the original potentially committed publication type must remain available: %v", err)
		Expect(publication.MayHaveCommitted).To(BeTrue())
		Expect(err.Error()).NotTo(ContainSubstring(controller.root))
		if changeGuard {
			Expect(err.Error()).To(ContainSubstring("runtime transition"), "the compound error must also report the guard release failure safely")
		}
	}, Entry("actual publication EIO without a guard-release failure", false), Entry("actual publication EIO plus a real private-mode change before guard release", true))
})
