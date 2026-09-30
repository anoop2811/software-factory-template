package filepublish

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestStagedPublication(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Staged publication ownership")
}

var _ = Describe("Prepared artifact ownership", func() {
	DescribeTable("refuses changed staging and never removes a replacement occupant", func(kind string) {
		path, err := os.MkdirTemp("", "factory-stage-ownership-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(os.RemoveAll(path)).To(Succeed()) })
		root, err := os.OpenRoot(path)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(root.Close()).To(Succeed()) })
		stage, err := Prepare(context.Background(), root, ".stage-", []byte("expected"), 0600)
		Expect(err).NotTo(HaveOccurred())
		temporary := filepath.Join(path, stage.name)
		switch kind {
		case "replacement":
			Expect(os.Rename(temporary, filepath.Join(path, "held-original"))).To(Succeed())
			Expect(os.WriteFile(temporary, []byte("adopter"), 0600)).To(Succeed())
		case "hardlink":
			Expect(os.Link(temporary, filepath.Join(path, "alias"))).To(Succeed())
		case "bytes":
			Expect(os.WriteFile(temporary, []byte("changed length"), 0600)).To(Succeed())
		}
		Expect(stage.Publish(context.Background(), "published")).NotTo(Succeed())
		_, err = os.Stat(filepath.Join(path, "published"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		if kind == "replacement" {
			Expect(stage.Cleanup()).NotTo(Succeed())
			data, err := os.ReadFile(temporary)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(Equal("adopter"))
		} else {
			Expect(stage.Cleanup()).To(Succeed())
			_, err = os.Stat(temporary)
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
		Expect(root.WriteFile("still-owned", nil, 0600)).To(Succeed())
	}, Entry("replacement", "replacement"), Entry("hardlink", "hardlink"), Entry("bytes", "bytes"))
})
