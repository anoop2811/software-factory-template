package qualification

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing/fstest"
)

// per docs/adr/0104-adaptive-qualification-workers.md:44
func linuxMemoryFixture() fstest.MapFS {
	return fstest.MapFS{
		"proc/meminfo":                            {Data: []byte("MemTotal: 33554432 kB\nMemAvailable: 16777216 kB\n")},
		"proc/self/cgroup":                        {Data: []byte("0::/runner/job\n")},
		"proc/self/mountinfo":                     {Data: []byte("29 23 0:26 / /sys/fs/cgroup rw,nosuid,nodev,noexec,relatime - cgroup2 cgroup rw\n")},
		"sys/fs/cgroup/memory.max":                {Data: []byte("max\n")},
		"sys/fs/cgroup/runner/memory.max":         {Data: []byte(fmt.Sprint(12 * gib))},
		"sys/fs/cgroup/runner/memory.current":     {Data: []byte(fmt.Sprint(4 * gib))},
		"sys/fs/cgroup/runner/job/memory.max":     {Data: []byte(fmt.Sprint(8 * gib))},
		"sys/fs/cgroup/runner/job/memory.current": {Data: []byte(fmt.Sprint(3 * gib))},
	}
}

var _ = Describe("Linux available memory accounting", func() {
	// per docs/adr/0104-adaptive-qualification-workers.md:44
	It("uses the most restrictive remaining allowance across all visible ancestors", func() {
		fsys := linuxMemoryFixture()
		available, err := linuxMemory(fsys)
		Expect(err).NotTo(HaveOccurred())
		Expect(available).To(Equal(5 * gib))
		fsys["sys/fs/cgroup/runner/memory.max"] = &fstest.MapFile{Data: []byte(fmt.Sprint(6 * gib))}
		available, err = linuxMemory(fsys)
		Expect(err).NotTo(HaveOccurred())
		Expect(available).To(Equal(2 * gib))
		fsys["sys/fs/cgroup/memory.max"] = &fstest.MapFile{Data: []byte(fmt.Sprint(5 * gib))}
		fsys["sys/fs/cgroup/memory.current"] = &fstest.MapFile{Data: []byte(fmt.Sprint(4 * gib))}
		available, err = linuxMemory(fsys)
		Expect(err).NotTo(HaveOccurred())
		Expect(available).To(Equal(gib))
	})
	// per docs/adr/0104-adaptive-qualification-workers.md:44
	It("bounds unlimited cgroups by MemAvailable and clamps exhausted limits without underflow", func() {
		fsys := linuxMemoryFixture()
		for _, prefix := range []string{"sys/fs/cgroup/runner/", "sys/fs/cgroup/runner/job/"} {
			fsys[prefix+"memory.max"] = &fstest.MapFile{Data: []byte("max\n")}
			delete(fsys, prefix+"memory.current")
		}
		available, err := linuxMemory(fsys)
		Expect(err).NotTo(HaveOccurred())
		Expect(available).To(Equal(16 * gib))
		fsys["sys/fs/cgroup/runner/job/memory.max"] = &fstest.MapFile{Data: []byte("1")}
		fsys["sys/fs/cgroup/runner/job/memory.current"] = &fstest.MapFile{Data: []byte("2")}
		available, err = linuxMemory(fsys)
		Expect(err).NotTo(HaveOccurred())
		Expect(available).To(BeZero())
	})
	// per docs/adr/0104-adaptive-qualification-workers.md:44
	It("accepts missing limits only at the genuine visible root", func() {
		fsys := linuxMemoryFixture()
		fsys["proc/self/cgroup"] = &fstest.MapFile{Data: []byte("0::/\n")}
		delete(fsys, "sys/fs/cgroup/memory.max")
		available, err := linuxMemory(fsys)
		Expect(err).NotTo(HaveOccurred())
		Expect(available).To(Equal(16 * gib))
		fsys["proc/self/cgroup"] = &fstest.MapFile{Data: []byte("0::/runner/job\n")}
		available, err = linuxMemory(fsys)
		Expect(err).NotTo(HaveOccurred(), "the actual hierarchy root lacks memory.max even when membership is nested")
		Expect(available).To(Equal(5 * gib))
		delete(fsys, "sys/fs/cgroup/runner/memory.max")
		_, err = linuxMemory(fsys)
		Expect(err).To(HaveOccurred(), "a missing non-root ancestor limit cannot assume host RAM")
	})
	// per docs/adr/0104-adaptive-qualification-workers.md:44
	DescribeTable("fails closed for unsupported or incomplete container evidence", func(boundary string) {
		fsys := linuxMemoryFixture()
		switch boundary {
		case "v1":
			fsys["proc/self/cgroup"] = &fstest.MapFile{Data: []byte("5:memory:/runner/job\n")}
		case "no-membership":
			delete(fsys, "proc/self/cgroup")
		case "no-mount":
			delete(fsys, "proc/self/mountinfo")
		case "hidden-root":
			fsys["proc/self/mountinfo"] = &fstest.MapFile{Data: []byte("29 23 0:26 /container /sys/fs/cgroup rw - cgroup2 cgroup rw\n")}
		case "missing-leaf":
			delete(fsys, "sys/fs/cgroup/runner/job/memory.max")
		case "missing-ancestor":
			delete(fsys, "sys/fs/cgroup/runner/memory.max")
		case "missing-usage":
			delete(fsys, "sys/fs/cgroup/runner/job/memory.current")
		case "invalid-limit":
			fsys["sys/fs/cgroup/runner/job/memory.max"] = &fstest.MapFile{Data: []byte("PRIVATE_BAD")}
		case "invalid-usage":
			fsys["sys/fs/cgroup/runner/job/memory.current"] = &fstest.MapFile{Data: []byte("-1")}
		case "overflow-limit":
			fsys["sys/fs/cgroup/runner/job/memory.max"] = &fstest.MapFile{Data: []byte("18446744073709551616")}
		case "missing-available":
			fsys["proc/meminfo"] = &fstest.MapFile{Data: []byte("MemTotal: 33554432 kB\n")}
		case "overflow-available":
			fsys["proc/meminfo"] = &fstest.MapFile{Data: []byte("MemAvailable: 18446744073709551615 kB\n")}
		case "escaping-membership":
			fsys["proc/self/cgroup"] = &fstest.MapFile{Data: []byte("0::/runner/../../outside\n")}
		}
		_, err := linuxMemory(fsys)
		Expect(err).To(HaveOccurred(), boundary)
	}, Entry("v1 controller", "v1"), Entry("unreadable membership", "no-membership"), Entry("unreadable mount map", "no-mount"), Entry("unrecognized mount root", "hidden-root"), Entry("missing finite leaf", "missing-leaf"), Entry("missing ancestor limit", "missing-ancestor"), Entry("missing finite usage", "missing-usage"), Entry("malformed limit", "invalid-limit"), Entry("malformed usage", "invalid-usage"), Entry("overflowing limit", "overflow-limit"), Entry("missing MemAvailable", "missing-available"), Entry("overflowing MemAvailable", "overflow-available"), Entry("path escape membership", "escaping-membership"))
})
