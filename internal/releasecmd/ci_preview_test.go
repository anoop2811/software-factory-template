package releasecmd

import (
	"strings"

	"github.com/anoop2811/software-factory-template/internal/installation"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CI migration confirmation presentation", func() {
	// per docs/adr/0148-transactional-installed-ci-migration.md:39
	DescribeTable("states whether confirmation includes the listed workflow changes", func(supported bool) {
		report := installation.CIWorkflowPreview{ApplySupported: supported, Reason: "specific supplied explanation", Changes: []installation.CIWorkflowChange{{Path: ".github/workflows/factory.yml", Disposition: "create", Reason: "known target"}}}
		text := strings.ToLower(ciWorkflowReport(report))
		Expect(text).To(ContainSubstring("specific supplied explanation"))
		Expect(text).To(ContainSubstring("create .github/workflows/factory.yml"))
		if supported {
			Expect(text).NotTo(ContainSubstring("apply unsupported"))
			Expect(text).NotTo(ContainSubstring("read-only"))
			Expect(text).To(ContainSubstring("confirmation"))
		} else {
			Expect(text).To(ContainSubstring("apply unsupported"))
			Expect(text).To(ContainSubstring("read-only"))
		}
	}, Entry("authenticated integrated transaction", true), Entry("unavailable local transaction", false))
})
