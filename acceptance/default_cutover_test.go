package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type defaultCutoverFixture struct {
	root, cwd, request, legacy, forbidden string
	environment                           map[string]string
}

// per docs/adr/0108-default-go-cutover.md:26
func defaultCutoverSurface() defaultCutoverFixture {
	GinkgoHelper()
	root, cwd := fixture()
	tools := filepath.Join(root, "selection tools")
	request, legacy, forbidden := filepath.Join(root, "release-request"), filepath.Join(root, "legacy-executed"), filepath.Join(root, "forbidden-tool")
	writeFixture(filepath.Join(tools, "gh"), []byte("#!/bin/sh\nprintf '%s\\000' \"$@\" > \"$DEFAULT_RELEASE_REQUEST\"\nprintf 'controlled release refusal\\n' >&2\nexit 1\n"), 0700)
	for _, name := range []string{"factory-init.sh", "factory-upgrade.sh"} {
		writeFixture(filepath.Join(root, "scripts", name), []byte("#!/bin/sh\nprintf called > \"$DEFAULT_LEGACY_EXECUTED\"\nexit 91\n"), 0700)
	}
	for _, name := range []string{"go", "python", "python3"} {
		writeFixture(filepath.Join(tools, name), []byte("#!/bin/sh\nprintf '%s\\n' \"$0\" > \"$DEFAULT_FORBIDDEN_TOOL\"\nexit 92\n"), 0700)
	}
	return defaultCutoverFixture{root, cwd, request, legacy, forbidden, map[string]string{
		"PATH": tools + string(os.PathListSeparator) + "/usr/bin:/bin", "FACTORY_BRIDGE_PROTOCOL": "",
		"DEFAULT_RELEASE_REQUEST": request, "DEFAULT_LEGACY_EXECUTED": legacy, "DEFAULT_FORBIDDEN_TOOL": forbidden,
	}}
}

// per docs/adr/0108-default-go-cutover.md:31
func defaultCutoverRefusal(f defaultCutoverFixture, version string, args []string) {
	GinkgoHelper()
	before := installationSnapshot(f.cwd)
	result := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("n"), filepath.Join(f.root, "factory"), args...)
	Expect(result.status).To(Equal(1), "selected release acquisition must refuse without script fallback: %+v", result)
	request, err := os.ReadFile(f.request)
	Expect(err).NotTo(HaveOccurred(), "the public route must attempt its exact immutable release before invoking an initializer")
	Expect(strings.Split(strings.TrimSuffix(string(request), "\x00"), "\x00")).To(Equal([]string{"api", "repos/anoop2811/software-factory-template/commits/refs/tags/" + version, "--jq", ".sha"}))
	for _, path := range []string{f.legacy, f.forbidden} {
		_, err := os.Lstat(path)
		Expect(os.IsNotExist(err)).To(BeTrue(), "refusal cannot invoke legacy scripts, Go or Python: %s", path)
	}
	Expect(installationSnapshot(f.cwd)).To(Equal(before), "release refusal must preserve all project bytes and modes")
}

var _ = Describe("Go default release cutover", func() {
	// per docs/adr/0108-default-go-cutover.md:26
	// per docs/adr/0108-default-go-cutover.md:43
	DescribeTable("selects the prepared immutable Go release before mutation and refuses unavailable acquisition", func(verb string) {
		modes := [][]string{{}}
		if verb == "upgrade" {
			modes = append(modes, []string{"--dry-run", "--json"})
		}
		for _, mode := range modes {
			f := defaultCutoverSurface()
			args := append([]string{verb}, mode...)
			if verb == "init" {
				args = append(args, filepath.Join(f.cwd, "literal $(not-code) project"), "--pack", "none")
			}
			defaultCutoverRefusal(f, "v0.2.0", args)
		}
	}, Entry("source init", "init"), Entry("ordinary upgrade", "upgrade"))

	// per docs/adr/0108-default-go-cutover.md:29
	DescribeTable("authenticates an explicit exact ref without falling back to source scripts", func(verb string) {
		for _, selector := range [][]string{{"--ref", "v0.2.1"}, {"--ref=v0.2.1"}} {
			modes := [][]string{{}}
			if verb == "upgrade" {
				modes = append(modes, []string{"--dry-run", "--json"})
			}
			for _, mode := range modes {
				f := defaultCutoverSurface()
				args := append([]string{verb}, mode...)
				if verb == "init" {
					args = append(args, filepath.Join(f.cwd, "literal ref target"), "--pack", "none")
				}
				defaultCutoverRefusal(f, "v0.2.1", append(args, selector...))
			}
		}
	}, Entry("source init", "init"), Entry("ordinary upgrade", "upgrade"))

	// per docs/adr/0108-default-go-cutover.md:170
	DescribeTable("preserves explicit ref then environment then declared release precedence", func(verb string) {
		for _, control := range []struct {
			selector []string
			version  string
		}{
			{nil, "v0.2.3"},
			{[]string{"--ref", "v0.2.1"}, "v0.2.1"},
		} {
			f := defaultCutoverSurface()
			f.environment["FACTORY_REF"] = "v0.2.3"
			args := []string{verb}
			if verb == "init" {
				args = append(args, filepath.Join(f.cwd, "environment selected project"), "--pack", "none")
			} else {
				args = append(args, "--dry-run", "--json")
			}
			defaultCutoverRefusal(f, control.version, append(args, control.selector...))
		}
	}, Entry("source init", "init"), Entry("ordinary upgrade", "upgrade"))

	// per docs/adr/0108-default-go-cutover.md:174
	It("keeps an explicit candidate selection ahead of an ambient release reference", func() {
		for _, verb := range []string{"init", "upgrade"} {
			f := defaultCutoverSurface()
			f.environment["FACTORY_REF"] = "main"
			args := []string{verb, "--go-candidate", handoffVersion}
			if verb == "init" {
				args = append(args, filepath.Join(f.cwd, "explicit candidate project"), "--pack", "none")
			} else {
				args = append(args, "--dry-run", "--json")
			}
			defaultCutoverRefusal(f, handoffVersion, args)
		}
	})
})
