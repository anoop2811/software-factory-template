package upgradecmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUpgradePreview(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Public upgrade preview output")
}

type previewFaultWriter struct {
	bytes.Buffer
	kind    string
	flushes int
}

func (w *previewFaultWriter) Write(data []byte) (int, error) {
	if w.kind == "write" {
		return 0, errors.New("PRIVATE_SINK_FAILURE")
	}
	if w.kind == "short" {
		return len(data) - 1, nil
	}
	return w.Buffer.Write(data)
}
func (w *previewFaultWriter) Flush() error {
	w.flushes++
	if w.kind == "flush" {
		return errors.New("PRIVATE_FLUSH_FAILURE")
	}
	return nil
}

var _ = Describe("Upgrade preview output failures", func() {
	BeforeEach(func() {
		original, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		root, err := os.MkdirTemp("", "factory-preview-output-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, root)
		Expect(os.Chdir(root)).To(Succeed())
		DeferCleanup(os.Chdir, original)
	})
	// per docs/adr/0082-go-public-upgrade-preview.md:96
	DescribeTable("preserves output failure status and emits one sanitized diagnostic", func(kind string, jsonOutput bool) {
		writer := &previewFaultWriter{kind: kind}
		var stderr bytes.Buffer
		args := []string{"--dry-run", "--source=."}
		if jsonOutput {
			args = append(args, "--json")
		}
		status := Run(context.Background(), args, writer, &stderr)
		Expect(status).To(Equal(1))
		Expect(stderr.String()).To(Equal("factory upgrade: cannot write preview\n"))
		if kind == "write" {
			Expect(writer.Len()).To(BeZero())
		}
		if kind == "flush" {
			Expect(writer.flushes).To(Equal(1))
		}
	}, Entry("JSON write error", "write", true), Entry("JSON short write", "short", true), Entry("JSON flush error", "flush", true), Entry("text write error", "write", false), Entry("text short write", "short", false), Entry("text flush error", "flush", false))
	// per docs/adr/0082-go-public-upgrade-preview.md:96
	It("preserves cancellation as status one with no report and an actionable safe reason", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var stdout, stderr bytes.Buffer
		status := Run(ctx, []string{"--dry-run", "--source=.", "--json"}, &stdout, &stderr)
		Expect(status).To(Equal(1))
		Expect(stdout.Len()).To(BeZero())
		Expect(stderr.String()).To(Equal("factory upgrade: preview canceled\n"))
	})

	// per docs/adr/0082-go-public-upgrade-preview.md:105
	It("preserves elapsed parent deadline without a report", func() {
		ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
		defer cancel()
		var stdout, stderr bytes.Buffer
		Expect(Run(ctx, []string{"--dry-run", "--source=.", "--json"}, &stdout, &stderr)).To(Equal(1))
		Expect(stdout.Len()).To(BeZero())
		Expect(stderr.String()).To(Equal("factory upgrade: preview deadline exceeded\n"))
	})
})
