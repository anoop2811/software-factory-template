package reviewlanecmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLanePublication(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Native review lane publication")
}

type laneBoundaryContext struct {
	context.Context
	boundary func()
}

func (c laneBoundaryContext) Err() error { c.boundary(); return c.Context.Err() }

var _ = Describe("Native review lane rollback", func() {
	// per docs/adr/0089-go-native-review-lane.md:115
	DescribeTable("qualifies the boundary after lane-on and before workflow publication", func(mode string) {
		root, err := os.MkdirTemp("", "factory-lane-publication-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(os.RemoveAll(root)).To(Succeed()) })
		configPath := filepath.Join(root, "factory.yaml")
		Expect(os.WriteFile(configPath, []byte("review_lane: off\n"), 0600)).To(Succeed())
		template := filepath.Join(root, "packs/review-lane")
		Expect(os.MkdirAll(template, 0700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(template, "review-pr.yml"), []byte("secret: __REVIEW_API_KEY_SECRET__\n"), 0600)).To(Succeed())
		path := filepath.Join(root, workflow)
		Expect(os.MkdirAll(filepath.Dir(path), 0700)).To(Succeed())
		previous := []byte(managedHeader + "\nname: prior workflow\n")
		if mode == "cancel existing" {
			Expect(os.WriteFile(path, previous, 0600)).To(Succeed())
		}
		base, cancel := context.WithCancel(context.Background())
		defer cancel()
		changed := false
		ctx := laneBoundaryContext{Context: base, boundary: func() {
			if changed {
				return
			}
			data, err := os.ReadFile(configPath)
			Expect(err).NotTo(HaveOccurred())
			if !strings.Contains(string(data), "review_lane: \"on\"") {
				return
			}
			changed = true
			stages, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".adversarial-review.*"))
			Expect(err).NotTo(HaveOccurred())
			Expect(stages).To(HaveLen(1), "complete workflow must already be staged")
			switch mode {
			case "replace destination":
				Expect(os.WriteFile(path, []byte("adopter replacement\n"), 0600)).To(Succeed())
			case "rollback unavailable":
				Expect(os.Rename(configPath, configPath+".held")).To(Succeed())
				Expect(os.Mkdir(configPath, 0700)).To(Succeed())
				cancel()
			default:
				cancel()
			}
		}}
		var stdout, stderr bytes.Buffer
		err = Execute(ctx, []string{"enable", "FIXTURE_KEY"}, root, root, map[string]string{}, &stdout, &stderr)
		Expect(changed).To(BeTrue(), "test must observe the on setting before injecting failure")
		Expect(err).To(HaveOccurred())
		Expect(stdout.String()).NotTo(ContainSubstring("review lane: enabled"))
		Expect(stderr.String()).NotTo(ContainSubstring("nothing is installed"))
		if mode == "rollback unavailable" {
			Expect(stderr.String()).To(ContainSubstring("could not be rolled back"))
			info, err := os.Stat(configPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.IsDir()).To(BeTrue())
		} else {
			data, err := os.ReadFile(configPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("review_lane: \"off\""))
			Expect(stderr.String()).To(ContainSubstring("back to off"))
		}
		switch mode {
		case "cancel existing":
			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(data).To(Equal(previous))
		case "replace destination":
			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(Equal("adopter replacement\n"))
		default:
			_, err := os.Stat(path)
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
		stages, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".adversarial-review.*"))
		Expect(err).NotTo(HaveOccurred())
		Expect(stages).To(BeEmpty())
	}, Entry("cancel existing", "cancel existing"), Entry("replace destination", "replace destination"), Entry("rollback unavailable", "rollback unavailable"))
})
