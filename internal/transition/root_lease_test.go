package transition

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

func rootLeaseFixture(withState bool) string {
	GinkgoHelper()
	root, err := os.MkdirTemp("", "factory-physical-root-lease-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, root)
	root, err = filepath.EvalSymlinks(root)
	Expect(err).NotTo(HaveOccurred())
	if withState {
		Expect(os.Mkdir(filepath.Join(root, ".factory"), 0700)).To(Succeed())
	}
	return root
}

func rootLeaseStat(root string) unix.Stat_t {
	GinkgoHelper()
	var result unix.Stat_t
	Expect(unix.Stat(root, &result)).To(Succeed())
	return result
}

var _ = Describe("Physical installation root read-only lease", func() {
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:150
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:275
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:349
	It("grants an owned healthy root with absent state without writing any entry", func() {
		ctx := context.Background()
		root := rootLeaseFixture(false)
		before := rootLeaseStat(root)
		lease, err := ReadOnly(ctx, root)
		Expect(err).NotTo(HaveOccurred(), "absence of optional .factory state is a healthy reader")
		DeferCleanup(lease.Close, ctx)
		Expect(lease.Check(ctx)).To(Succeed())
		entries, err := os.ReadDir(root)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
		after := rootLeaseStat(root)
		Expect(after.Dev).To(Equal(before.Dev))
		Expect(after.Ino).To(Equal(before.Ino))
		Expect(after.Mode).To(Equal(before.Mode))
		Expect(after.Mtim).To(Equal(before.Mtim))
		Expect(after.Ctim).To(Equal(before.Ctim))
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:150
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:275
	It("retains shared exclusion in existing healthy state until explicit close", func() {
		ctx := context.Background()
		root := rootLeaseFixture(true)
		before := rootLeaseStat(root)
		lease, err := ReadOnly(ctx, root)
		Expect(err).NotTo(HaveOccurred())
		closed := false
		DeferCleanup(func() {
			if !closed {
				Expect(lease.Close(ctx)).To(Succeed())
			}
		})
		competitor, err := RootExclusive(ctx, root)
		Expect(err).To(HaveOccurred())
		Expect(competitor).To(BeNil())
		Expect(lease.Check(ctx)).To(Succeed())
		Expect(lease.Close(ctx)).To(Succeed())
		closed = true
		competitor, err = RootExclusive(ctx, root)
		Expect(err).NotTo(HaveOccurred())
		Expect(competitor.Close(ctx)).To(Succeed())
		after := rootLeaseStat(root)
		Expect(after.Dev).To(Equal(before.Dev))
		Expect(after.Ino).To(Equal(before.Ino))
		Expect(after.Mode).To(Equal(before.Mode))
		Expect(after.Mtim).To(Equal(before.Mtim))
		Expect(after.Ctim).To(Equal(before.Ctim))
		entries, err := os.ReadDir(filepath.Join(root, ".factory"))
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:145
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:153
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:352
	DescribeTable("revalidates pending and state entry identity after a healthy grant", func(change string) {
		ctx := context.Background()
		root := rootLeaseFixture(true)
		lease, err := ReadOnly(ctx, root)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(lease.Close, ctx)
		Expect(lease.Check(ctx)).To(Succeed())
		if change == "pending" {
			Expect(os.WriteFile(filepath.Join(root, ".factory/runtime-publication.pending"), nil, 0600)).To(Succeed())
		} else {
			Expect(os.Rename(filepath.Join(root, ".factory"), filepath.Join(root, "former-state"))).To(Succeed())
			Expect(os.Mkdir(filepath.Join(root, ".factory"), 0700)).To(Succeed())
		}
		Expect(lease.Check(ctx)).To(HaveOccurred(), "a retained reader cannot admit changed pending/state custody")
	}, Entry("new pending entry", "pending"), Entry("replaced private state directory", "identity"))
})
