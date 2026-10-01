package migrateconfigcmd

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

// per docs/adr/0090-go-native-config-migration.md:88
func TestMigrationPublication(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Native config migration publication")
}

type migrationBoundaryContext struct {
	context.Context
	boundary func()
}

func (c migrationBoundaryContext) Err() error {
	c.boundary()
	return c.Context.Err()
}

func publicationFixture() (string, []byte, []byte) {
	GinkgoHelper()
	root, err := os.MkdirTemp("", "factory-migration-publication-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { Expect(os.RemoveAll(root)).To(Succeed()) })
	yaml := []byte("existing: mine\n")
	legacy := []byte("ADDED=literal\n")
	Expect(os.WriteFile(filepath.Join(root, "factory.yaml"), yaml, 0640)).To(Succeed()) // #nosec G306 -- intentional fixture permissions qualify preservation: docs/adr/0090-go-native-config-migration.md:68.
	Expect(os.WriteFile(filepath.Join(root, "factory.config"), legacy, 0600)).To(Succeed())
	return root, yaml, legacy
}

func publicationBytes(root, name string) []byte {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join(root, name))
	Expect(err).NotTo(HaveOccurred())
	return data
}

func noPublicationStages(root string) {
	GinkgoHelper()
	stages, err := filepath.Glob(filepath.Join(root, ".factory-config-*"))
	Expect(err).NotTo(HaveOccurred())
	Expect(stages).To(BeEmpty())
}

var _ = Describe("Native config migration publication boundaries", func() {
	// per docs/adr/0090-go-native-config-migration.md:71
	DescribeTable("rechecks the original legacy snapshot at rename-helper admission", func(replace bool) {
		root, yaml, legacy := publicationFixture()
		project, err := openProject(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(project.directory.Close)
		original, err := project.read(context.Background(), "factory.config")
		Expect(err).NotTo(HaveOccurred())
		Expect(original.data).To(Equal(legacy))
		changed := false
		unknown := []byte("ADDED=changed\n")
		ctx := migrationBoundaryContext{Context: context.Background(), boundary: func() {
			if changed {
				return
			}
			changed = true
			if replace {
				Expect(os.Rename(filepath.Join(root, "factory.config"), filepath.Join(root, "legacy.held"))).To(Succeed())
			}
			Expect(os.WriteFile(filepath.Join(root, "factory.config"), unknown, 0600)).To(Succeed())
			if !replace {
				// Keep inode, size and mtime equal so the original bytes must be checked.
				Expect(len(unknown)).To(Equal(len(original.data)))
				Expect(os.Chtimes(filepath.Join(root, "factory.config"), original.info.ModTime(), original.info.ModTime())).To(Succeed())
			}
		}}
		err = project.renameLegacy(ctx, original)
		Expect(changed).To(BeTrue(), "replacement must occur on the helper's own first admission check")
		Expect(err).To(HaveOccurred(), "the rename helper must not move bytes absent from its original snapshot")
		Expect(publicationBytes(root, "factory.config")).To(Equal(unknown))
		Expect(publicationBytes(root, "factory.yaml")).To(Equal(yaml))
		if replace {
			Expect(publicationBytes(root, "legacy.held")).To(Equal(legacy))
		}
		_, err = os.Lstat(filepath.Join(root, "factory.config.migrated"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("replacement inode", true), Entry("same inode changed bytes", false))

	// per docs/adr/0090-go-native-config-migration.md:67
	DescribeTable("refuses observed legacy replacement after YAML staging and cleans its stage", func(replace bool) {
		root, yaml, legacy := publicationFixture()
		changed := false
		ctx := migrationBoundaryContext{Context: context.Background(), boundary: func() {
			if changed {
				return
			}
			stages, err := filepath.Glob(filepath.Join(root, ".factory-config-*"))
			Expect(err).NotTo(HaveOccurred())
			if len(stages) == 0 {
				return
			}
			Expect(stages).To(HaveLen(1))
			Expect(publicationBytes(root, "factory.yaml")).To(Equal(yaml), "injection must precede YAML publication")
			changed = true
			if replace {
				Expect(os.Rename(filepath.Join(root, "factory.config"), filepath.Join(root, "legacy.held"))).To(Succeed())
			}
			Expect(os.WriteFile(filepath.Join(root, "factory.config"), []byte("ADOPTER=KEEP\n"), 0600)).To(Succeed())
		}}
		var stdout bytes.Buffer
		err := Execute(ctx, nil, root, map[string]string{}, &stdout)
		Expect(changed).To(BeTrue(), "test must observe the prepared stage before injecting replacement")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).NotTo(ContainSubstring("YAML was updated"))
		Expect(publicationBytes(root, "factory.yaml")).To(Equal(yaml))
		Expect(publicationBytes(root, "factory.config")).To(Equal([]byte("ADOPTER=KEEP\n")))
		if replace {
			Expect(publicationBytes(root, "legacy.held")).To(Equal(legacy))
		}
		_, err = os.Lstat(filepath.Join(root, "factory.config.migrated"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(stdout.String()).To(BeEmpty())
		noPublicationStages(root)
	}, Entry("changed bytes in original inode", false), Entry("replacement inode", true))

	// per docs/adr/0090-go-native-config-migration.md:74
	DescribeTable("retains complete YAML and reports partial state after publication", func(mode string) {
		root, _, legacy := publicationFixture()
		base, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)
		changed := false
		ctx := migrationBoundaryContext{Context: base, boundary: func() {
			if changed {
				return
			}
			data := publicationBytes(root, "factory.yaml")
			if !strings.Contains(string(data), "config_migrated: \"yes\"") {
				return
			}
			Expect(string(data)).To(ContainSubstring("added: \"literal\""))
			changed = true
			switch mode {
			case "new backup":
				Expect(os.WriteFile(filepath.Join(root, "factory.config.migrated"), []byte("UNKNOWN RECOVERY KEEP"), 0640)).To(Succeed()) // #nosec G306 -- intentional recovery metadata must survive: docs/adr/0090-go-native-config-migration.md:63.
			case "replacement legacy":
				Expect(os.Rename(filepath.Join(root, "factory.config"), filepath.Join(root, "legacy.held"))).To(Succeed())
				Expect(os.WriteFile(filepath.Join(root, "factory.config"), []byte("ADOPTER=KEEP\n"), 0600)).To(Succeed())
			case "moved legacy":
				Expect(os.Rename(filepath.Join(root, "factory.config"), filepath.Join(root, "legacy.held"))).To(Succeed())
			case "removed legacy":
				Expect(os.Remove(filepath.Join(root, "factory.config"))).To(Succeed())
			default:
				cancel()
			}
		}}
		var stdout bytes.Buffer
		err := Execute(ctx, nil, root, map[string]string{}, &stdout)
		Expect(changed).To(BeTrue(), "test must observe complete YAML before injecting failure")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("YAML was updated"))
		Expect(err.Error()).To(ContainSubstring("legacy rename did not complete"))
		if mode == "replacement legacy" || mode == "moved legacy" || mode == "removed legacy" {
			Expect(err.Error()).NotTo(ContainSubstring("factory.config remains for recovery"), "do not assert original recovery input survived when it did not")
		}
		Expect(stdout.String()).To(BeEmpty())
		data := publicationBytes(root, "factory.yaml")
		Expect(string(data)).To(Equal("existing: mine\nadded: \"literal\"\nconfig_migrated: \"yes\"\n"))
		info, err := os.Stat(filepath.Join(root, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0640)))
		switch mode {
		case "replacement legacy":
			Expect(publicationBytes(root, "factory.config")).To(Equal([]byte("ADOPTER=KEEP\n")))
			Expect(publicationBytes(root, "legacy.held")).To(Equal(legacy))
		case "moved legacy", "removed legacy":
			_, err = os.Lstat(filepath.Join(root, "factory.config"))
			Expect(os.IsNotExist(err)).To(BeTrue())
			if mode == "moved legacy" {
				Expect(publicationBytes(root, "legacy.held")).To(Equal(legacy))
			}
		default:
			Expect(publicationBytes(root, "factory.config")).To(Equal(legacy))
		}
		if mode == "new backup" {
			Expect(publicationBytes(root, "factory.config.migrated")).To(Equal([]byte("UNKNOWN RECOVERY KEEP")))
		} else {
			_, err = os.Lstat(filepath.Join(root, "factory.config.migrated"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
		noPublicationStages(root)
	}, Entry("cancellation", "cancel"), Entry("destination appeared", "new backup"), Entry("legacy replaced", "replacement legacy"), Entry("legacy moved", "moved legacy"), Entry("legacy removed", "removed legacy"))

	// per docs/adr/0090-go-native-config-migration.md:72
	It("uses a kernel no-replace rename that preserves both occupied names", func() {
		root, _, legacy := publicationFixture()
		backup := filepath.Join(root, "factory.config.migrated")
		Expect(os.WriteFile(backup, []byte("UNKNOWN RECOVERY KEEP"), 0640)).To(Succeed()) // #nosec G306 -- intentional recovery metadata must survive: docs/adr/0090-go-native-config-migration.md:63.
		before, err := os.Stat(backup)
		Expect(err).NotTo(HaveOccurred())
		directory, err := os.Open(root)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(directory.Close)
		err = renameNoReplace(int(directory.Fd()), "factory.config", "factory.config.migrated") // #nosec G115 -- test-owned ordinary directory descriptor fits int on the supported Linux/macOS platforms.
		Expect(err).To(HaveOccurred())
		Expect(publicationBytes(root, "factory.config")).To(Equal(legacy))
		Expect(publicationBytes(root, "factory.config.migrated")).To(Equal([]byte("UNKNOWN RECOVERY KEEP")))
		after, err := os.Stat(backup)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
	})
})
