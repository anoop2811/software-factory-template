package acceptance_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func hooksHistorical(root, revision string) string {
	GinkgoHelper()
	contents, err := os.ReadFile(historicalConfiguration(root, revision))
	Expect(err).NotTo(HaveOccurred())
	path := filepath.Join(root, revision, "hooks-config.sh")
	writeFixture(path, contents, 0600)
	return path
}

func hooksShimPath() string {
	GinkgoHelper()
	p, err := filepath.Abs(filepath.Join("..", "runtime", "shell", "readers.sh"))
	Expect(err).NotTo(HaveOccurred())
	return p
}

func hooksProcess(root, cwd, source, setup string) cliResult {
	GinkgoHelper()
	// The setup is a test-owned shell fragment, never configuration data.
	probe := `. "$1"
` + setup + `
 before_options="$-"; before_ifs="${IFS-unset}"; before_pwd="$PWD"
 before_args="$*"
 factory_local_hooks ignored surplus || exit "$?"
 [ "$before_options" = "$-" ] && [ "$before_ifs" = "${IFS-unset}" ] && [ "$before_pwd" = "$PWD" ] && [ "$before_args" = "$*" ] || exit 97
`
	return readerProcess(cwd, map[string]string{"FACTORY_CONFIG": filepath.Join(cwd, "factory.yaml"), "FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}, "bash", "-c", probe, "hooks-probe", source)
}

var _ = Describe("G1 local hook registrations", func() {
	// per specs/001-go-runtime-conversion.md:102
	DescribeTable("preserves immutable Bash registration output and caller state", func(raw, setup, expected string) {
		root, cwd := fixture()
		writeFixture(filepath.Join(cwd, "factory.yaml"), []byte("local_hooks: \""+raw+"\"\n"), 0600)
		for _, name := range []string{"a.sh", "b.sh", "z.txt", "space hook.txt"} {
			writeFixture(filepath.Join(cwd, name), nil, 0600)
		}
		for _, source := range []string{hooksHistorical(root, baselineCommit), hooksHistorical(root, releaseReaderBaseline), hooksShimPath()} {
			Expect(hooksProcess(root, cwd, source, setup)).To(Equal(cliResult{expected, "", 0}), source)
		}
	},
		Entry("ordinary hooks", "a.sh b.sh", "", "a.sh\nb.sh\n"),
		Entry("flag grouping and orphan flags", "--orphan -x a.sh --strict -n b.sh --help", "", "a.sh --strict -n\nb.sh --help\n"),
		Entry("nonflag arguments start entries", "a.sh argument b.sh", "", "a.sh\nargument\nb.sh\n"),
		Entry("comma positional argument", "a.sh argument, b.sh --strict", "", "a.sh argument\nb.sh --strict\n"),
		Entry("empty comma fields", ",, a.sh ,, , b.sh,", "", "a.sh\nb.sh\n"),
		Entry("comma orphan flag remains entry", "--orphan,a.sh", "", "--orphan\na.sh\n"),
		Entry("ASCII comma whitespace", " \ta.sh\t\v\f argument\r , \tb.sh\t ", "", "a.sh argument\nb.sh\n"),
		Entry("Unicode whitespace remains literal", "a.sh\u00a0arg,b.sh\u2003arg", "", "a.sh\u00a0arg\nb.sh\u2003arg\n"),
		Entry("empty registration", "", "", ""),
		Entry("only flags", "--one -x", "", ""),
		Entry("caller custom IFS", "a.sh:--strict:b.sh", "IFS=:", "a.sh --strict\nb.sh\n"),
		Entry("custom IFS empty word resets grouping", "a.sh::--strict:b.sh", "IFS=:", "a.sh\nb.sh\n"),
		Entry("caller empty IFS", "a.sh b.sh", "IFS=", "a.sh b.sh\n"),
		Entry("unset IFS words", "a.sh b.sh", "unset IFS", "a.sh\nb.sh\n"),
		Entry("comma ignores custom IFS", "a.sh arg, b.sh arg", "IFS=:", "a.sh arg\nb.sh arg\n"),
		Entry("matching pathname expansion", "*.sh --strict", "", "a.sh\nb.sh --strict\n"),
		Entry("comma pathname with embedded space", "space*.txt,z.txt", "", "space hook.txt\nz.txt\n"),
		Entry("comma pathname expansion", "*.sh,z.txt", "", "a.sh\nb.sh\nz.txt\n"),
		Entry("noglob preserved", "*.sh --strict", "set -f", "*.sh --strict\n"),
		Entry("nullglob preserved", "missing*.sh a.sh", "shopt -s nullglob", "a.sh\n"),
		Entry("comma noglob", "*.sh,z.txt", "set -f", "*.sh\nz.txt\n"),
		Entry("nounset normal IFS", "a.sh,b.sh", "set -u", "a.sh\nb.sh\n"),
		Entry("strict caller", "a.sh --strict b.sh", "set -euo pipefail", "a.sh --strict\nb.sh\n"),
	)

	// per specs/001-go-runtime-conversion.md:103
	It("keeps command substitutions and shell metacharacters inert", func() {
		root, cwd := fixture()
		raw := "$(touch marker),`touch marker`,a.sh;touch marker"
		writeFixture(filepath.Join(cwd, "factory.yaml"), []byte("local_hooks: \""+raw+"\"\n"), 0600)
		for _, source := range []string{hooksHistorical(root, baselineCommit), hooksHistorical(root, releaseReaderBaseline), hooksShimPath()} {
			Expect(hooksProcess(root, cwd, source, "")).To(Equal(cliResult{"$(touch marker)\n`touch marker`\na.sh;touch marker\n", "", 0}))
			_, err := os.Stat(filepath.Join(cwd, "marker"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	})

	// per specs/001-go-runtime-conversion.md:95
	It("sources without executing the selected runtime", func() {
		root, cwd := fixture()
		writeFixture(filepath.Join(root, "fake"), []byte("#!/bin/sh\ntouch \"$HOOK_MARKER\"\nexit 79\n"), 0755)
		result := readerProcess(cwd, map[string]string{"FACTORY_RUNTIME_BINARY": filepath.Join(root, "fake"), "HOOK_MARKER": filepath.Join(cwd, "marker")}, "bash", "-c", `set -euf; IFS=:; before="$-"; . "$1"; [ "$before" = "$-" ] && [ "$IFS" = : ]; declare -F factory_local_hooks >/dev/null`, "source-probe", hooksShimPath())
		Expect(result).To(Equal(cliResult{"", "", 0}))
		_, err := os.Stat(filepath.Join(cwd, "marker"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per specs/001-go-runtime-conversion.md:95
	DescribeTable("refuses an unusable selected runtime without fallback", func(kind string) {
		root, cwd := fixture()
		selected := filepath.Join(root, "absent")
		switch kind {
		case "empty":
			selected = ""
		case "relative":
			selected = "./factory"
		case "nonexecutable":
			writeFixture(selected, []byte("ignored"), 0600)
		case "directory":
			Expect(os.Mkdir(selected, 0700)).To(Succeed())
		}
		result := readerProcess(cwd, map[string]string{"FACTORY_RUNTIME_BINARY": selected}, "bash", "-c", `. "$1"; factory_local_hooks`, "missing-runtime", hooksShimPath())
		Expect(result).To(Equal(cliResult{"", "factory bridge: FACTORY_RUNTIME_BINARY must name an executable absolute regular file\n", 2}))
	}, Entry("missing", "missing"), Entry("empty", "empty"), Entry("relative", "relative"), Entry("nonexecutable", "nonexecutable"), Entry("directory", "directory"))

	// per specs/001-go-runtime-conversion.md:95
	It("propagates a failed reader before parsing any registration", func() {
		root, cwd := fixture()
		fake := filepath.Join(root, "failed-reader")
		writeFixture(fake, []byte("#!/bin/sh\nprintf 'must-not-be-a-hook'\nprintf 'reader failed\\n' >&2\nexit 73\n"), 0755)
		result := readerProcess(cwd, map[string]string{"FACTORY_RUNTIME_BINARY": fake}, "bash", "-c", `. "$1"; factory_local_hooks`, "failed-reader", hooksShimPath())
		Expect(result).To(Equal(cliResult{"", "reader failed\n", 73}))
	})

	// per specs/001-go-runtime-conversion.md:102
	DescribeTable("returns no hooks for absent configuration", func(contents string) {
		root, cwd := fixture()
		if contents != "missing" {
			writeFixture(filepath.Join(cwd, "factory.yaml"), []byte(contents), 0600)
		}
		for _, source := range []string{hooksHistorical(root, baselineCommit), hooksHistorical(root, releaseReaderBaseline), hooksShimPath()} {
			Expect(hooksProcess(root, cwd, source, "")).To(Equal(cliResult{"", "", 0}))
		}
	}, Entry("missing file", "missing"), Entry("missing key", "other: value\n"), Entry("empty key", "local_hooks:\n"))

	// per specs/001-go-runtime-conversion.md:102
	DescribeTable("groups literal private operands without Cobra flag interpretation", func(args []string, expected string) {
		root, cwd := fixture()
		Expect(bridgeReader(root, cwd, nil, append([]string{"config", "hooks"}, args...)...)).To(Equal(cliResult{expected, "", 0}))
	}, Entry("word grouping", []string{"words", "--orphan", "a.sh", "--help", "b.sh", "-x"}, "a.sh --help\nb.sh -x\n"),
		Entry("comma normalization", []string{"commas", " \ta.sh\n\rarg\v\f ", "", " ", "--help"}, "a.sh arg\n--help\n"),
		Entry("empty words flush grouping", []string{"words", "", "--orphan", "a.sh", "", "--strict"}, "a.sh\n"),
		Entry("no words", []string{"words"}, ""), Entry("no fields", []string{"commas"}, ""))

	// per specs/001-go-runtime-conversion.md:102
	It("preserves failglob refusal instead of accepting an unmatched registration", func() {
		root, cwd := fixture()
		writeFixture(filepath.Join(cwd, "factory.yaml"), []byte("local_hooks: unmatched*.sh\n"), 0600)
		for _, source := range []string{hooksHistorical(root, baselineCommit), hooksHistorical(root, releaseReaderBaseline), hooksShimPath()} {
			result := readerProcess(cwd, map[string]string{"FACTORY_CONFIG": filepath.Join(cwd, "factory.yaml"), "FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}, "bash", "-c", `. "$1"; shopt -s failglob; factory_local_hooks`, "failglob", source)
			Expect(result.status).NotTo(Equal(0), source+" stdout="+result.stdout+" stderr="+result.stderr)
			Expect(result.stdout).To(BeEmpty())
			Expect(result.stderr).To(ContainSubstring("no match: unmatched*.sh"))
		}
	})

	// per specs/001-go-runtime-conversion.md:103
	It("reports executable hook paths without running them or external parsing tools", func() {
		root, cwd := fixture()
		hook := filepath.Join(cwd, "hook")
		writeFixture(hook, []byte("#!/bin/sh\ntouch \"$HOOK_MARKER\"\n"), 0755)
		writeFixture(filepath.Join(cwd, "factory.yaml"), []byte("local_hooks: \""+hook+" argument, second --strict\"\n"), 0600)
		result := readerProcess(cwd, map[string]string{"PATH": "/no-external-tools", "FACTORY_CONFIG": filepath.Join(cwd, "factory.yaml"), "FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory"), "HOOK_MARKER": filepath.Join(cwd, "marker")}, "/bin/bash", "-c", `. "$1"; factory_local_hooks`, "no-tools", hooksShimPath())
		Expect(result).To(Equal(cliResult{hook + " argument\nsecond --strict\n", "", 0}))
		_, err := os.Stat(filepath.Join(cwd, "marker"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per specs/001-go-runtime-conversion.md:103
	DescribeTable("never evaluates configuration through inherited helper attributes", func(setup string) {
		root, cwd := fixture()
		raw := "array[$(touch marker)]"
		writeFixture(filepath.Join(cwd, "factory.yaml"), []byte("local_hooks: \""+raw+"\"\n"), 0600)
		for _, source := range []string{hooksHistorical(root, baselineCommit), hooksHistorical(root, releaseReaderBaseline), hooksShimPath()} {
			probe := `. "$1"; ` + setup + `; factory_local_hooks`
			result := readerProcess(cwd, map[string]string{"FACTORY_CONFIG": filepath.Join(cwd, "factory.yaml"), "FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}, "bash", "-c", probe, "attributed-caller", source)
			if result.status == 77 {
				Skip("caller Bash lacks the optional attribute capability")
			}
			_, err := os.Stat(filepath.Join(cwd, "marker"))
			Expect(os.IsNotExist(err)).To(BeTrue(), "configuration executed through inherited variable attributes")
			Expect(result).To(Equal(cliResult{"array[$(touch\nmarker)]\n", "", 0}))
		}
	}, Entry("integer", "declare -i _factory_hooks_raw=0"),
		Entry("indexed array", "declare -a _factory_hooks_raw=(original)"),
		Entry("nameref to integer", "declare -i reference_target=0; declare -n _factory_hooks_raw=reference_target 2>/dev/null || exit 77"),
		Entry("localvar inheritance", "shopt -s localvar_inherit 2>/dev/null || exit 77; declare -i _factory_hooks_raw=0"))

	// per specs/001-go-runtime-conversion.md:95
	DescribeTable("keeps the sourceable interface usable through bin sh", func(raw, expected string) {
		root, cwd := fixture()
		writeFixture(filepath.Join(cwd, "factory.yaml"), []byte("local_hooks: \""+raw+"\"\n"), 0600)
		for _, source := range []string{hooksHistorical(root, baselineCommit), hooksHistorical(root, releaseReaderBaseline), hooksShimPath()} {
			result := readerProcess(cwd, map[string]string{"FACTORY_CONFIG": filepath.Join(cwd, "factory.yaml"), "FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}, "/bin/sh", "-c", `. "$1"; factory_local_hooks`, "sh-hooks", source)
			Expect(result).To(Equal(cliResult{expected, "", 0}))
		}
	}, Entry("words", "a.sh --strict b.sh", "a.sh --strict\nb.sh\n"), Entry("commas", "a.sh argument, b.sh", "a.sh argument\nb.sh\n"))

	// per specs/001-go-runtime-conversion.md:102
	It("uses the first configured registration including an empty duplicate", func() {
		root, cwd := fixture()
		for _, contents := range []string{"local_hooks: a.sh --strict\nlocal_hooks: b.sh\n", "local_hooks: \"\"\nlocal_hooks: b.sh\n"} {
			writeFixture(filepath.Join(cwd, "factory.yaml"), []byte(contents), 0600)
			expected := "a.sh --strict\n"
			if contents == "local_hooks: \"\"\nlocal_hooks: b.sh\n" {
				expected = ""
			}
			for _, source := range []string{hooksHistorical(root, baselineCommit), hooksHistorical(root, releaseReaderBaseline), hooksShimPath()} {
				Expect(hooksProcess(root, cwd, source, "")).To(Equal(cliResult{expected, "", 0}))
			}
		}
	})

	// per specs/001-go-runtime-conversion.md:298
	DescribeTable("refuses malformed grouping requests", func(args []string) {
		root, cwd := fixture()
		result := bridgeReader(root, cwd, nil, append([]string{"config", "hooks"}, args...)...)
		Expect(result.status).To(Equal(2))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).To(HavePrefix("factory bridge: "))
		Expect(result.stderr).NotTo(ContainSubstring("Usage:"))
	}, Entry("missing mode", []string{}), Entry("unknown mode", []string{"unknown"}), Entry("flag mode", []string{"--help"}))
})
