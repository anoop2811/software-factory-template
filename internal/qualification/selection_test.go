package qualification

import (
	"bytes"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"strconv"
	"strings"
)

const gib = uint64(1 << 30)

var _ = Describe("Adaptive worker policy", func() {
	// per docs/adr/0104-adaptive-qualification-workers.md:22
	// per docs/adr/0104-adaptive-qualification-workers.md:38
	DescribeTable("uses the minimum scheduling budget while capping ordered handoff groups", func(requested, maximum string, cpus int, memory uint64, base, handoff int) {
		resources := Resources{CPUs: cpus, MemoryBytes: memory}
		plan, err := Select(requested, maximum, resources)
		Expect(err).NotTo(HaveOccurred())
		Expect(plan.Base).To(Equal(base))
		Expect(plan.Handoff).To(Equal(handoff))
		Expect(plan.CPUs).To(Equal(cpus))
		Expect(plan.MemoryBytes).To(Equal(memory))
	},
		Entry("Linux four CPU runner permits three", "auto", "4", 4, 16*gib, 3, 2),
		Entry("three CPU seven GB runner stays at two", "auto", "4", 3, uint64(7000000000), 2, 2),
		Entry("one CPU retains one", "auto", "4", 1, 16*gib, 1, 1),
		Entry("two CPUs reserve one", "auto", "4", 2, 16*gib, 1, 1),
		Entry("default cap limits a large runner", "auto", "4", 64, 128*gib, 4, 2),
		Entry("configured cap one remains serial", "auto", "1", 64, 128*gib, 1, 1),
		Entry("large cap still respects CPU reserve", "auto", "32", 8, 128*gib, 7, 2),
		Entry("four GiB leaves one worker", "auto", "32", 64, 4*gib, 1, 1),
		Entry("six GiB permits two", "auto", "32", 64, 6*gib, 2, 2),
		Entry("eight GiB permits three", "auto", "32", 64, 8*gib, 3, 2),
		Entry("just below eight GiB permits only two", "auto", "32", 64, 8*gib-1, 2, 2),
		Entry("below memory reserve keeps floor one", "auto", "4", 8, gib, 1, 1),
		Entry("unknown memory falls back one", "auto", "4", 8, uint64(0), 1, 1),
		Entry("unknown CPU falls back one", "auto", "4", 0, 16*gib, 1, 1),
		Entry("negative CPU falls back one", "auto", "4", -1, 16*gib, 1, 1),
		Entry("explicit three overrides unknown resources", "3", "4", 0, uint64(0), 3, 2),
		Entry("explicit one remains valid", "1", "4", 8, 16*gib, 1, 1),
		Entry("maximum explicit32 is bounded", "32", "32", 1, uint64(0), 32, 2),
		Entry("large memory avoids integer overflow", "auto", "32", 64, ^uint64(0), 32, 2))

	// per docs/adr/0104-adaptive-qualification-workers.md:29
	It("rejects noncanonical requests and caps rather than coercing them", func() {
		invalid := []string{"", "0", "33", "01", "+2", "-1", " 2", "2 ", "2.0", "1e1", "AUTO", "٢", "PRIVATE_BAD; exit 0", "$(touch PRIVATE_SENTINEL)"}
		for _, value := range invalid {
			_, err := Select(value, "32", Resources{CPUs: 8, MemoryBytes: 16 * gib})
			Expect(err).To(HaveOccurred(), "requested=%q", value)
			Expect(err.Error()).NotTo(ContainSubstring("PRIVATE"), "raw invalid values are never an explanation")
			_, err = Select("auto", value, Resources{CPUs: 8, MemoryBytes: 16 * gib})
			Expect(err).To(HaveOccurred(), "maximum=%q", value)
		}
		_, err := Select("5", "4", Resources{CPUs: 64, MemoryBytes: 128 * gib})
		Expect(err).To(HaveOccurred())
		_, err = Select("auto", "auto", Resources{CPUs: 8, MemoryBytes: 16 * gib})
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("Private worker helper protocol", func() {
	// per docs/adr/0104-adaptive-qualification-workers.md:36
	It("returns exactly one validated plan and bounded stderr including defaults only for absent keys", func() {
		for _, entry := range []struct {
			environment map[string]string
			want        string
		}{
			{map[string]string{"GO_RUNTIME_TEST_PROCS": "3"}, "3 2\n"},
			{map[string]string{"GO_RUNTIME_TEST_PROCS": "1"}, "1 1\n"},
			{map[string]string{"GO_RUNTIME_TEST_PROCS": "32", "GO_RUNTIME_TEST_MAX_PROCS": "32"}, "32 2\n"},
			{map[string]string{}, ""},
		} {
			var output, diagnostic bytes.Buffer
			Expect(Run(nil, entry.environment, &output, &diagnostic)).To(Equal(0))
			if entry.want != "" {
				Expect(output.String()).To(Equal(entry.want))
			} else {
				parts := strings.Fields(output.String())
				Expect(parts).To(HaveLen(2))
				base, err := strconv.Atoi(parts[0])
				Expect(err).NotTo(HaveOccurred())
				handoff, err := strconv.Atoi(parts[1])
				Expect(err).NotTo(HaveOccurred())
				Expect(base).To(BeNumerically(">=", 1))
				Expect(base).To(BeNumerically("<=", 4))
				Expect(handoff).To(Equal(min(base, 2)))
				Expect(output.String()).To(Equal(fmt.Sprintf("%d %d\n", base, handoff)))
			}
			Expect(diagnostic.Len()).To(BeNumerically(">", 0))
			Expect(diagnostic.Len()).To(BeNumerically("<=", 1024))
		}
	})
	// per docs/adr/0104-adaptive-qualification-workers.md:29
	It("refuses explicit empty and malformed settings without a plan or raw-value disclosure", func() {
		for _, environment := range []map[string]string{{"GO_RUNTIME_TEST_PROCS": ""}, {"GO_RUNTIME_TEST_MAX_PROCS": ""}, {"GO_RUNTIME_TEST_PROCS": "PRIVATE_BAD; exit 0"}, {"GO_RUNTIME_TEST_PROCS": "5", "GO_RUNTIME_TEST_MAX_PROCS": "4"}, {"GO_RUNTIME_TEST_MAX_PROCS": "33"}} {
			var output, diagnostic bytes.Buffer
			Expect(Run(nil, environment, &output, &diagnostic)).NotTo(Equal(0))
			Expect(output.String()).To(BeEmpty())
			Expect(diagnostic.Len()).To(BeNumerically(">", 0))
			Expect(diagnostic.Len()).To(BeNumerically("<=", 1024))
			Expect(diagnostic.String()).NotTo(ContainSubstring("PRIVATE_BAD"))
		}
	})
})
