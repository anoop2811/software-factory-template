package initcmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInitPublication(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Native init publication")
}

type publicationBoundaryContext struct {
	context.Context
	boundary func()
}

func (c publicationBoundaryContext) Err() error { c.boundary(); return c.Context.Err() }

var _ = Describe("Native init prepared-file ownership", func() {
	// per docs/adr/0085-go-native-init.md:93
	DescribeTable("refuses substituted staged bytes and preserves the unknown pathname occupant", func(cancelAtBoundary bool) {
		root, err := os.MkdirTemp("", "factory-init-publication-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(os.RemoveAll(root)).To(Succeed()) })
		opened, err := openTree(root)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(opened.root.Close()).To(Succeed()) })
		base, cancel := context.WithCancel(context.Background())
		defer cancel()
		var temporary string
		changed := false
		ctx := publicationBoundaryContext{Context: base, boundary: func() {
			if changed {
				return
			}
			entries, readErr := os.ReadDir(root)
			Expect(readErr).NotTo(HaveOccurred())
			for _, entry := range entries {
				if !strings.HasPrefix(entry.Name(), ".factory-init-") {
					continue
				}
				temporary = filepath.Join(root, entry.Name())
				Expect(os.Rename(temporary, filepath.Join(root, "held-original"))).To(Succeed())
				Expect(os.WriteFile(temporary, []byte("unknown occupant"), 0600)).To(Succeed())
				changed = true
				if cancelAtBoundary {
					cancel()
				}
				return
			}
		}}
		err = opened.publish(ctx, asset{path: "published", data: []byte("expected bytes"), mode: 0644})
		Expect(changed).To(BeTrue(), "test must reach the real post-sync publication boundary")
		Expect(err).To(HaveOccurred())
		if cancelAtBoundary {
			Expect(err).To(MatchError(context.Canceled))
		}
		_, statErr := os.Stat(filepath.Join(root, "published"))
		Expect(os.IsNotExist(statErr)).To(BeTrue())
		contents, readErr := os.ReadFile(temporary)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(contents)).To(Equal("unknown occupant"))
		contents, readErr = os.ReadFile(filepath.Join(root, "held-original"))
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(contents)).To(Equal("expected bytes"))
	}, Entry("before publication", false), Entry("during canceled cleanup", true))
})
