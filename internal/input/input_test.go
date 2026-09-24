package input

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

func TestInput(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Command-owned input acceptance")
}

var _ = Describe("Cancellable owned input", func() {
	// per docs/adr/0074-go-loop-checkpoint-storage.md:129
	DescribeTable("restores shared descriptor flags and leaves the original pipe usable", func(nonblocking bool, operation string) {
		original, writer, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(original.Close()).To(Succeed()); Expect(writer.Close()).To(Succeed()) })
		descriptor := original.Fd()
		Expect(unix.SetNonblock(int(descriptor), nonblocking)).To(Succeed())
		before, err := unix.FcntlInt(descriptor, unix.F_GETFL, 0)
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var reader io.Reader
		var cleanup func() error
		if operation == "setup failure" {
			reader, cleanup, err = prepare(ctx, original, setupOps{newFile: func(uintptr, string) *os.File { return nil }})
			Expect(err).To(HaveOccurred())
			Expect(reader).To(BeNil())
		} else {
			reader, cleanup, err = Cancellable(ctx, original)
			Expect(err).NotTo(HaveOccurred())
			Expect(cleanup).NotTo(BeNil())
			DeferCleanup(func() { cancel(); Expect(cleanup()).To(Succeed()) })
			if operation == "success" {
				_, err = writer.Write([]byte("x"))
				Expect(err).NotTo(HaveOccurred())
				data := make([]byte, 1)
				_, err = io.ReadFull(reader, data)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(data)).To(Equal("x"))
			} else {
				first := make(chan error, 1)
				done := make(chan error, 1)
				go func() {
					defer close(done)
					data := make([]byte, 1)
					_, readErr := io.ReadFull(reader, data)
					first <- readErr
					close(first)
					if readErr == nil {
						_, readErr = io.ReadFull(reader, data)
					}
					done <- readErr
				}()
				DeferCleanup(func() { cancel(); Eventually(done, time.Second).Should(BeClosed()) })
				_, err = writer.Write([]byte("x"))
				Expect(err).NotTo(HaveOccurred())
				var readErr error
				Eventually(first, time.Second).Should(Receive(&readErr))
				Expect(readErr).NotTo(HaveOccurred())
				cancel()
				Eventually(done, time.Second).Should(Receive(&readErr))
				Expect(readErr).To(HaveOccurred())
			}
			Expect(cleanup()).To(Succeed())
		}
		after, err := unix.FcntlInt(descriptor, unix.F_GETFL, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		_, err = writer.Write([]byte("y"))
		Expect(err).NotTo(HaveOccurred())
		data := make([]byte, 1)
		_, err = io.ReadFull(original, data)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("y"))
		if operation == "setup failure" && cleanup != nil {
			Expect(cleanup()).To(Succeed())
		}
	}, Entry("blocking success", false, "success"), Entry("nonblocking success", true, "success"), Entry("blocking cancellation", false, "cancel"), Entry("nonblocking cancellation", true, "cancel"), Entry("blocking setup failure", false, "setup failure"), Entry("nonblocking setup failure", true, "setup failure"))
})
