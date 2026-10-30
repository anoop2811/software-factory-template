package installation

import (
	"strings"

	"github.com/anoop2811/software-factory-template/internal/staging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Installation recovery argument custody", func() {
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:340
	DescribeTable("retains fresh image trust and literal project operands without carrying consent or credentials", func(local, goBefore bool, expected []string) {
		GinkgoT().Setenv("GITHUB_TOKEN", "PRIVATE_RECOVERY_CREDENTIAL")
		GinkgoT().Setenv("REVIEW_API_KEY", "PRIVATE_RECOVERY_REVIEW_KEY")
		source := staging.Options{Archive: "/source dir/$(touch SOURCE).tar.gz", Version: "v9.8.0", Revision: strings.Repeat("a", 40), Target: "linux/amd64", Local: local, Output: "PRIVATE_STAGE_OUTPUT"}
		before := staging.Options{Archive: "/before dir/with \"quotes\".tar.gz", Version: "v9.7.0", Revision: strings.Repeat("b", 40), Target: "linux/amd64", Local: !local}
		if !local {
			source.Attestation, source.TrustedRoot = "/source trust/attestation.json", "/source trust/root.json"
			if goBefore {
				source.Verifier = "/source tools/gh;literal"
			}
		} else {
			before.Attestation, before.TrustedRoot, before.Verifier = "/before trust/attestation.json", "/before trust/root.json", "/before tools/gh $(literal)"
		}
		request := Request{Source: source, Before: before, Profile: "bash-baseline", Confirmation: "PRIVATE_ALREADY_GRANTED_CONSENT", Quiescent: true, Inputs: map[string]string{"PROJECT_SLUG": "slug=literal", "PROJECT_NAME": "Project $(touch NEVER_EXECUTED)\nline=two", "GITHUB_OWNER": "@owner", "OPENCODE_USERNAME": "user;literal", "PROTECTED_PATH": "", "DOCS_ROOT": "specs", "CITATION_PREFIX": "SPEC_"}}
		if goBefore {
			request.Profile = "go-hybrid-v1"
		}
		actual := recoveryArguments(request, "known-id", "reverse")
		Expect(actual).To(Equal(expected))
		Expect(actual).NotTo(ContainElement("--confirm"))
		Expect(actual).NotTo(ContainElement("--quiescent"))
		Expect(strings.Join(actual, "\x00")).NotTo(ContainSubstring("PRIVATE_"))
	}, Entry("local target with legacy predecessor", true, false, []string{
		"factory", "upgrade", "--installation", "--recover", "known-id", "--direction", "reverse", "--dry-run", "--profile", "bash-baseline",
		"--source", "/source dir/$(touch SOURCE).tar.gz", "--version", "v9.8.0", "--revision", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "--target", "linux/amd64", "--local",
		"--project-input", "CITATION_PREFIX=SPEC_", "--project-input", "DOCS_ROOT=specs", "--project-input", "GITHUB_OWNER=@owner", "--project-input", "OPENCODE_USERNAME=user;literal", "--project-input", "PROJECT_NAME=Project $(touch NEVER_EXECUTED)\nline=two", "--project-input", "PROJECT_SLUG=slug=literal", "--project-input", "PROTECTED_PATH=",
	}), Entry("authenticated target with legacy predecessor and default verifier", false, false, []string{
		"factory", "upgrade", "--installation", "--recover", "known-id", "--direction", "reverse", "--dry-run", "--profile", "bash-baseline",
		"--source", "/source dir/$(touch SOURCE).tar.gz", "--version", "v9.8.0", "--revision", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "--target", "linux/amd64", "--attestation", "/source trust/attestation.json", "--trusted-root", "/source trust/root.json",
		"--project-input", "CITATION_PREFIX=SPEC_", "--project-input", "DOCS_ROOT=specs", "--project-input", "GITHUB_OWNER=@owner", "--project-input", "OPENCODE_USERNAME=user;literal", "--project-input", "PROJECT_NAME=Project $(touch NEVER_EXECUTED)\nline=two", "--project-input", "PROJECT_SLUG=slug=literal", "--project-input", "PROTECTED_PATH=",
	}), Entry("local target with authenticated Go predecessor", true, true, []string{
		"factory", "upgrade", "--installation", "--recover", "known-id", "--direction", "reverse", "--dry-run", "--profile", "go-hybrid-v1",
		"--source", "/source dir/$(touch SOURCE).tar.gz", "--version", "v9.8.0", "--revision", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "--target", "linux/amd64", "--local",
		"--before-source", "/before dir/with \"quotes\".tar.gz", "--before-version", "v9.7.0", "--before-revision", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "--before-target", "linux/amd64", "--before-attestation", "/before trust/attestation.json", "--before-trusted-root", "/before trust/root.json", "--before-gh", "/before tools/gh $(literal)",
		"--project-input", "CITATION_PREFIX=SPEC_", "--project-input", "DOCS_ROOT=specs", "--project-input", "GITHUB_OWNER=@owner", "--project-input", "OPENCODE_USERNAME=user;literal", "--project-input", "PROJECT_NAME=Project $(touch NEVER_EXECUTED)\nline=two", "--project-input", "PROJECT_SLUG=slug=literal", "--project-input", "PROTECTED_PATH=",
	}), Entry("authenticated target with local Go predecessor", false, true, []string{
		"factory", "upgrade", "--installation", "--recover", "known-id", "--direction", "reverse", "--dry-run", "--profile", "go-hybrid-v1",
		"--source", "/source dir/$(touch SOURCE).tar.gz", "--version", "v9.8.0", "--revision", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "--target", "linux/amd64", "--attestation", "/source trust/attestation.json", "--trusted-root", "/source trust/root.json", "--gh", "/source tools/gh;literal",
		"--before-source", "/before dir/with \"quotes\".tar.gz", "--before-version", "v9.7.0", "--before-revision", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "--before-target", "linux/amd64", "--before-local",
		"--project-input", "CITATION_PREFIX=SPEC_", "--project-input", "DOCS_ROOT=specs", "--project-input", "GITHUB_OWNER=@owner", "--project-input", "OPENCODE_USERNAME=user;literal", "--project-input", "PROJECT_NAME=Project $(touch NEVER_EXECUTED)\nline=two", "--project-input", "PROJECT_SLUG=slug=literal", "--project-input", "PROTECTED_PATH=",
	}))
})
