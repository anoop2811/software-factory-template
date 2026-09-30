package output

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

func TestOutput(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cancellable borrowed output")
}

var _ = Describe("Borrowed pipe output", func() {
	DescribeTable("restores flags and leaves the caller descriptor usable", func(nonblocking, cancelWrite bool) {
		reader, writer, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = reader.Close(); _ = writer.Close() })
		// Establish the normal post-write baseline: Darwin records FWASWRITTEN
		// on the first write, independently of descriptor borrowing.
		_, err = writer.Write([]byte("w"))
		Expect(err).NotTo(HaveOccurred())
		warmup := make([]byte, 1)
		_, err = io.ReadFull(reader, warmup)
		Expect(err).NotTo(HaveOccurred())
		descriptor := writer.Fd()
		Expect(unix.SetNonblock(int(descriptor), nonblocking)).To(Succeed())
		before, err := unix.FcntlInt(descriptor, unix.F_GETFL, 0)
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if cancelWrite {
			done := make(chan error, 1)
			go func() { defer close(done); done <- WriteEvent(ctx, writer, bytes.Repeat([]byte{'x'}, 1<<20)) }()
			DeferCleanup(func() {
				cancel()
				_ = reader.Close()
				_ = writer.Close()
				Eventually(done, time.Second).Should(BeClosed())
			})
			Expect(reader.SetReadDeadline(time.Now().Add(time.Second))).To(Succeed())
			first := make([]byte, 1)
			_, err := reader.Read(first)
			Expect(err).NotTo(HaveOccurred())
			cancel()
			var writeErr error
			Eventually(done, time.Second).Should(Receive(&writeErr))
			Expect(writeErr).To(HaveOccurred())
		} else {
			Expect(WriteEvent(ctx, writer, []byte("x"))).To(Succeed())
		}
		after, err := unix.FcntlInt(descriptor, unix.F_GETFL, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		// Drain the partial write without closing the caller's read end.
		Expect(reader.SetReadDeadline(time.Now().Add(30 * time.Millisecond))).To(Succeed())
		buffer := make([]byte, 65536)
		for {
			if _, err := reader.Read(buffer); err != nil {
				Expect(os.IsTimeout(err)).To(BeTrue(), "%v", err)
				break
			}
		}
		Expect(reader.SetReadDeadline(time.Now().Add(time.Second))).To(Succeed())
		_, err = writer.Write([]byte("y"))
		Expect(err).NotTo(HaveOccurred())
		one := make([]byte, 1)
		_, err = io.ReadFull(reader, one)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(one)).To(Equal("y"))
	}, Entry("blocking success", false, false), Entry("nonblocking success", true, false), Entry("blocking cancellation", false, true), Entry("nonblocking cancellation", true, true))
})
