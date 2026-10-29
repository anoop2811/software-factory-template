package acceptance_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The fixture returns the selected official metadata fields, including the
// independent GitHub prerelease flag even when the tag is valid stable semver.
// per docs/adr/0108-default-go-cutover.md:48
func defaultReleasePolicyMetadata(f handoffBootstrapFixture) {
	GinkgoHelper()
	ghPath := filepath.Join(f.root, "host tools", "gh")
	gh, err := os.ReadFile(ghPath)
	Expect(err).NotTo(HaveOccurred())
	original := `release-view) printf '%s\tfalse\n' "$3" ;;`
	replacement := `release-view)
    query= previous=
    for value do
      if [ "$previous" = --jq ]; then query="$value"; break; fi
      previous="$value"
    done
    prerelease="${HANDOFF_METADATA_PRERELEASE-true}"
    case "$query" in
      *isPrerelease*) printf '%s\tfalse\t%s\n' "$3" "$prerelease" ;;
      *'@tsv'*) printf '%s\tfalse\n' "$3" ;;
      '') printf '{"tagName":"%s","isDraft":false,"isPrerelease":%s}\n' "$3" "$prerelease" ;;
      *) printf 'unsupported fixture metadata query\n' >&2; exit 97 ;;
    esac ;;`
	Expect(strings.Count(string(gh), original)).To(Equal(1))
	writeFixture(ghPath, []byte(strings.Replace(string(gh), original, replacement, 1)), 0700)
}

// per docs/adr/0108-default-go-cutover.md:48
func defaultReleasePolicyFixture() (handoffBootstrapFixture, string) {
	GinkgoHelper()
	f := handoffBootstrap(map[string]string{"linux": "Linux", "darwin": "Darwin"}[runtime.GOOS], map[string]string{"amd64": "x86_64", "arm64": "arm64"}[runtime.GOARCH], runtime.GOOS+"-"+runtime.GOARCH, "download-fail")
	defaultReleasePolicyMetadata(f)
	driver := filepath.Join(f.root, "policy caller", "factory")
	writeFixture(driver, candidateBinary, 0700)
	f.environment["FACTORY_BRIDGE_PROTOCOL"] = ""
	f.environment["GORACE"] = "atexit_sleep_ms=0"
	return f, driver
}

// per docs/adr/0108-default-go-cutover.md:43
func defaultReleasePolicyUnchanged(f handoffBootstrapFixture, before map[string]string) {
	GinkgoHelper()
	Expect(installationSnapshot(f.cwd)).To(Equal(before), "release metadata refusal must preserve every project byte and mode")
	for _, name := range []string{"candidate-executed", "forbidden-tool", "verify", "bootstrap-argv"} {
		_, err := os.Lstat(filepath.Join(f.records, name))
		Expect(os.IsNotExist(err)).To(BeTrue(), "metadata refusal must precede execution and authentication: %s", name)
	}
}

var _ = Describe("Stable release metadata policy", Label("default-release-policy"), func() {
	// per docs/adr/0108-default-go-cutover.md:29
	// per docs/adr/0108-default-go-cutover.md:48
	It("keeps stable or candidate selection explicit in replayed text preview advice", func() {
		image := handoffBuildImage()
		prepared := installationLegacyFixture(image.binary, image.references["v0.1.6"], image.archivePath, image.revision)
		for _, selector := range []string{"--go-candidate", "--ref"} {
			f, driver := handoffNativeFixture(image)
			defaultReleasePolicyMetadata(f)
			f.environment["HANDOFF_METADATA_PRERELEASE"] = "false"
			cutoverReleaseAssets(f, image, "v0.2.0")
			legacy := installationCloneLegacyFixture(image.binary, prepared)
			input := "PROJECT_NAME=literal $(not-code) preview value"
			args := []string{"upgrade", selector, "v0.2.0", "--project-input", input}
			before := installationSnapshot(legacy.root)
			proposal := handoffProposal(handoffNativeProcess(legacy.root, f.environment, "", driver, append(args, "--dry-run", "--json")...))
			text := handoffNativeProcess(legacy.root, f.environment, "", driver, append(args, "--dry-run")...)
			Expect(text.status).To(Equal(0), "%s: %+v", selector, text)
			command := localSourceAdvice(text.stdout, "Apply with:")
			f.environment["STABLE_ADVICE_ARGV"] = filepath.Join(f.records, "stable-advice-argv")
			f.environment["STABLE_ADVICE_SCRIPT"] = filepath.Join(f.root, "advice receipt")
			writeFixture(f.environment["STABLE_ADVICE_SCRIPT"], []byte("#!/bin/sh\nprintf '%s\\000' \"$@\" > \"$STABLE_ADVICE_ARGV\"\n"), 0600)
			writeFixture(filepath.Join(f.root, "host tools", "curl"), []byte("#!/bin/sh\n/bin/cat \"$STABLE_ADVICE_SCRIPT\"\n"), 0700)
			replay := handoffNativeProcess(legacy.root, f.environment, "", "/bin/sh", "-c", command)
			Expect(replay.status).To(Equal(0), "%s advice must replay to an inert consumer: %+v", selector, replay)
			actual := handoffRecorded(f, "stable-advice-argv")
			Expect(actual).To(Equal([]string{"upgrade", selector, "v0.2.0", "--confirm", proposal["proposal_digest"].(string), "--quiescent", "--project-input", input}), "an apply instruction must preserve the exact stable/candidate policy, complete digest and literal project input")
			Expect(installationSnapshot(legacy.root)).To(Equal(before), "preview and inert advice replay must preserve project state")
			_, err := os.Lstat(filepath.Join(f.records, "forbidden-tool"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	})

	// per docs/adr/0108-default-go-cutover.md:35
	// per docs/adr/0108-default-go-cutover.md:48
	DescribeTable("refuses installer prerelease metadata before download", func(preparedDefault bool) {
		f := installerCutoverSurface("download-fail")
		f.environment["HANDOFF_METADATA_PRERELEASE"] = "true"
		args := installerCutoverOperands(f, "init")
		if preparedDefault {
			installerPreparedGoPin(f)
		} else {
			args = append(args, "--ref", "v0.2.0")
		}
		before := installationSnapshot(f.cwd)
		result := imageProcess(f.cwd, f.environment, "/bin/sh", append([]string{f.installer}, args...)...)
		Expect(result.status).NotTo(Equal(0), "%+v", result)
		defaultReleasePolicyUnchanged(f, before)
		Expect(handoffRecorded(f, "release-view")[2]).To(Equal("v0.2.0"))
		_, err := os.Lstat(filepath.Join(f.records, "release-download"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "the shell must refuse prerelease metadata before asset download: %+v", result)
	}, Entry("prepared Go default", true), Entry("explicit Go ref", false))

	// per docs/adr/0108-default-go-cutover.md:29
	// per docs/adr/0108-default-go-cutover.md:48
	It("preserves stable ref policy across a freshly authenticated raw executable handoff", func() {
		image := handoffBuildImage()
		f, driver := defaultReleasePolicyFixture()
		f.environment["HANDOFF_SCENARIO"] = ""
		f.environment["HANDOFF_METADATA_PRERELEASE"] = "false"
		f.environment["HANDOFF_REVISION"] = image.revision
		assets := f.environment["HANDOFF_ASSETS"]
		writeFixture(filepath.Join(assets, "factory-bootstrap-v0.2.0-"+f.target), f.raw, 0600)
		writeFixture(filepath.Join(assets, "factory-installation-v0.2.0-"+f.target+".tar.gz"), handoffImageBinary(image, f.raw, "v0.2.0"), 0600)
		before := installationSnapshot(f.cwd)
		target := filepath.Join(f.cwd, "literal $(not-code) stable handoff")
		result := handoffNativeProcess(f.cwd, f.environment, "", driver, "init", target, "--ref", "v0.2.0", "--pack", "none")
		Expect(result.status).To(Equal(0), "the authenticated receipt executable must actually run: %+v", result)
		Expect(handoffRecorded(f, "bootstrap-argv")).To(Equal([]string{"init", "--ref", "v0.2.0", target, "--pack", "none"}), "stable policy must remain explicit when the authenticated child reselects the release")
		trace, err := os.ReadFile(filepath.Join(f.records, "trace"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(trace)).To(ContainSubstring("verify\nbootstrap\n"), "raw receipt must follow authentication of both complete-image subjects")
		Expect(installationSnapshot(f.cwd)).To(Equal(before), "the receipt child has no initialization authority")
	})

	// per docs/adr/0108-default-go-cutover.md:26
	// per docs/adr/0108-default-go-cutover.md:48
	DescribeTable("rejects a stable-looking release marked prerelease before download", func(verb string, selector []string) {
		f, driver := defaultReleasePolicyFixture()
		before := installationSnapshot(f.cwd)
		args := []string{verb}
		if verb == "init" {
			args = append(args, filepath.Join(f.cwd, "unpublished health project"), "--pack", "none")
		} else {
			args = append(args, "--dry-run", "--json")
		}
		result := handoffNativeProcess(f.cwd, f.environment, "", driver, append(args, selector...)...)
		Expect(result.status).NotTo(Equal(0), "%+v", result)
		defaultReleasePolicyUnchanged(f, before)
		view := handoffRecorded(f, "release-view")
		Expect(view[2]).To(Equal("v0.2.0"))
		Expect(handoffFlag(view, "--repo")).To(Equal("anoop2811/software-factory-template"))
		_, err := os.Lstat(filepath.Join(f.records, "release-download"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "the GitHub prerelease flag must refuse before any asset download: %+v", result)
	},
		Entry("ordinary init", "init", []string{}),
		Entry("explicit init ref", "init", []string{"--ref", "v0.2.0"}),
		Entry("ordinary upgrade", "upgrade", []string{}),
		Entry("explicit upgrade ref", "upgrade", []string{"--ref=v0.2.0"}),
	)

	// per docs/adr/0108-default-go-cutover.md:30
	// per docs/adr/0108-default-go-cutover.md:48
	DescribeTable("allows explicit candidate selection to reach a controlled download refusal", func(verb string) {
		f, driver := defaultReleasePolicyFixture()
		before := installationSnapshot(f.cwd)
		args := []string{verb, "--go-candidate", "v0.2.0"}
		if verb == "init" {
			args = append(args, filepath.Join(f.cwd, "explicit candidate project"), "--pack", "none")
		} else {
			args = append(args, "--dry-run", "--json")
		}
		result := handoffNativeProcess(f.cwd, f.environment, "", driver, args...)
		Expect(result.status).NotTo(Equal(0), "controlled download must refuse: %+v", result)
		defaultReleasePolicyUnchanged(f, before)
		download := handoffRecorded(f, "release-download")
		Expect(download[:3]).To(Equal([]string{"release", "download", "v0.2.0"}))
		Expect(handoffFlag(download, "--repo")).To(Equal("anoop2811/software-factory-template"))
	}, Entry("init", "init"), Entry("upgrade", "upgrade"))
})
