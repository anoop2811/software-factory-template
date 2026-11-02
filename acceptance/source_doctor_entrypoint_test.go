package acceptance_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Omitted host model settings remain absent at the exact execution boundary.
// per docs/adr/0129-go-source-doctor-entrypoint.md:69
func sourceDoctorEnvironment(inherited []string) map[string]string {
	result := map[string]string{}
	for _, entry := range inherited {
		key, value, _ := strings.Cut(entry, "=")
		// These fixture-only keys must not reach the owned proof.
		// per docs/adr/0129-go-source-doctor-entrypoint.md:87
		switch key {
		case "GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN", "GH_HOST", "GH_REPO":
			continue
		}
		if strings.HasPrefix(key, "OPENCODE_") || strings.HasPrefix(key, "CLAUDE_") || strings.HasPrefix(key, "CODEX_") || key == "COST_PROFILE" || key == "MODEL_PROVIDER" {
			continue
		}
		result[key] = value
	}
	result["BASH_ENV"], result["ENV"] = "", ""
	// Health-only diagnostics must not activate a host GitHub query.
	// per docs/adr/0129-go-source-doctor-entrypoint.md:83
	result["REVIEW_LANE"] = "off"
	return result
}

// The owned proof records presence flags only, never inherited credential values.
// per docs/adr/0129-go-source-doctor-entrypoint.md:87
const sourceDoctorProof = `#!/bin/bash
if IFS= read -r line; then printf 'INHERITED_INPUT\n' >&2; exit 92; fi
printf '%s\000' "$PWD" > "$SOURCE_DOCTOR_PROOF_RECORD"
printf '%s\000' "${GH_TOKEN+x}" "${GITHUB_TOKEN+x}" "${GH_ENTERPRISE_TOKEN+x}" "${GITHUB_ENTERPRISE_TOKEN+x}" "${GH_HOST+x}" "${GH_REPO+x}" > "$SOURCE_DOCTOR_ENV_RECORD"
if [ -n "${GH_TOKEN+x}${GITHUB_TOKEN+x}${GH_ENTERPRISE_TOKEN+x}${GITHUB_ENTERPRISE_TOKEN+x}${GH_HOST+x}${GH_REPO+x}" ]; then
  printf 'GITHUB_CONTEXT_PRESENT\n'
  exit 93
fi
if [ "$SOURCE_DOCTOR_PROOF_FAIL" = 1 ]; then printf 'selftest: 0 passed, 1 failed\n'; exit 9; fi
printf 'selftest: 7 passed, 0 failed\n'
`

var _ = Describe("Go source runtime entrypoints doctor adapter", func() {
	// per docs/adr/0129-go-source-doctor-entrypoint.md:25
	// per docs/adr/0129-go-source-doctor-entrypoint.md:31
	// per docs/adr/0129-go-source-doctor-entrypoint.md:37
	// per docs/adr/0129-go-source-doctor-entrypoint.md:63
	It("runs native caller-root diagnostics and supervised proof while preserving live adapters", func() {
		caller, inherited := doctorFixture()
		// Challenge inherited activation without changing a parallel process's environment.
		// per docs/adr/0129-go-source-doctor-entrypoint.md:83
		challenged := append(append([]string{}, inherited...), "REVIEW_LANE=on",
			"GH_TOKEN=synthetic-token", "GITHUB_TOKEN=synthetic-token",
			"GH_ENTERPRISE_TOKEN=synthetic-enterprise-token", "GITHUB_ENTERPRISE_TOKEN=synthetic-enterprise-token",
			"GH_HOST=fixture.invalid", "GH_REPO=fixture/example")
		environment := sourceDoctorEnvironment(challenged)
		source := newSourceEntrypointFixture()
		owner := source.root
		for key, value := range source.environment {
			environment[key] = value
		}
		program := sourceObservabilityAdapter(owner, "doctor")
		sourceObservabilityPoison(owner)
		marker := filepath.Join(filepath.Dir(owner), "LEGACY_OWNER_CALLED")
		environment["SOURCE_OBSERVABILITY_FALLBACK"] = marker
		writeFixture(filepath.Join(owner, "scripts/selftest/run.sh"), []byte("#!/bin/sh\nprintf owner-proof > \"$SOURCE_OBSERVABILITY_FALLBACK\"\nexit 96\n"), 0700)
		original, err := os.ReadFile(filepath.Join(caller, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		selected := strings.Replace(string(original), "'*_test.go'", "'caller-spec-*.go'", 1)
		Expect(selected).NotTo(Equal(string(original)))
		writeFixture(filepath.Join(caller, "doctor-selected.yaml"), []byte(selected), 0600)
		writeFixture(filepath.Join(owner, "doctor-selected.yaml"), []byte("test_file_patterns: wrong-owner-*.go\n"), 0600)
		doctorGit(caller, inherited, "init", "-q")
		nested := filepath.Join(caller, "nested original cwd")
		Expect(os.Mkdir(nested, 0700)).To(Succeed())
		writeFixture(filepath.Join(nested, "doctor-selected.yaml"), []byte("test_file_patterns: wrong-cwd-*.go\n"), 0600)
		environment["FACTORY_CONFIG"] = "doctor-selected.yaml"
		// A diagnostic fixture must never query the operator's GitHub credentials.
		// per docs/adr/0129-go-source-doctor-entrypoint.md:83
		ghRecord := filepath.Join(filepath.Dir(caller), "unexpected gh argv")
		tools := filepath.Join(filepath.Dir(caller), "doctor controlled tools")
		writeFixture(filepath.Join(tools, "gh"), []byte("#!/bin/sh\nprintf '%s\\000' \"$@\" > \"$SOURCE_DOCTOR_GH_RECORD\"\nexit 83\n"), 0700)
		environment["PATH"] = tools + string(os.PathListSeparator) + environment["PATH"]
		environment["SOURCE_DOCTOR_GH_RECORD"] = ghRecord
		record := filepath.Join(filepath.Dir(caller), "caller proof cwd")
		environment["SOURCE_DOCTOR_PROOF_RECORD"] = record
		envRecord := filepath.Join(filepath.Dir(caller), "proof GitHub context presence")
		environment["SOURCE_DOCTOR_ENV_RECORD"] = envRecord
		writeFixture(filepath.Join(caller, "scripts/selftest/run.sh"), []byte(sourceDoctorProof), 0700)
		live := filepath.Join(caller, ".claude/settings.json")
		Expect(os.Chmod(live, 0640)).To(Succeed())
		liveBefore, err := os.Lstat(live)
		Expect(err).NotTo(HaveOccurred())
		liveBytes, err := os.ReadFile(live)
		Expect(err).NotTo(HaveOccurred())
		for _, failedProof := range []bool{false, true} {
			environment["SOURCE_DOCTOR_PROOF_FAIL"] = "0"
			if failedProof {
				environment["SOURCE_DOCTOR_PROOF_FAIL"] = "1"
				// The second invocation uses the already qualified native owner without another source build.
				writeFixture(filepath.Join(owner, "factory"), candidateBinary, 0755)
				Expect(os.Remove(record)).To(Succeed())
				Expect(os.Remove(envRecord)).To(Succeed())
			}
			beforeOwner, beforeCaller := sourceEntrypointSnapshot(owner), installationSnapshot(caller)
			observed := handoffNativeExactProcess(nested, environment, "borrowed caller input\n", program, "--help", "", "literal $(not-code)\nline", "--")
			AddReportEntry("source-doctor.native-diagnostics", map[string]any{"failed_proof": failedProof, "source_launcher": !failedProof, "status": observed.status, "stdout": observed.stdout, "stderr": observed.stderr}, ReportEntryVisibilityNever)
			_, ghErr := os.Lstat(ghRecord)
			var ghArgv []byte
			if ghErr == nil {
				ghArgv, err = os.ReadFile(ghRecord)
				Expect(err).NotTo(HaveOccurred())
			}
			AddReportEntry("source-doctor.review-boundary", map[string]any{"failed_proof": failedProof, "gh_called": ghErr == nil, "gh_argv": strings.Split(strings.TrimSuffix(string(ghArgv), "\x00"), "\x00")}, ReportEntryVisibilityNever)
			Expect(os.IsNotExist(ghErr)).To(BeTrue(), "diagnostic fixture must not query host GitHub")
			Expect(observed.stdout).To(MatchRegexp(`\[inert\]\s+review lane\s+off`))
			// Inspect child presence only; even synthetic credential values stay out of reports.
			// per docs/adr/0129-go-source-doctor-entrypoint.md:87
			presence, err := os.ReadFile(envRecord)
			Expect(err).NotTo(HaveOccurred())
			flags := strings.Split(strings.TrimSuffix(string(presence), "\x00"), "\x00")
			AddReportEntry("source-doctor.github-context-presence", map[string]any{"failed_proof": failedProof, "presence_flags": flags}, ReportEntryVisibilityNever)
			Expect(flags).To(Equal([]string{"", "", "", "", "", ""}), "owned proof must receive none of the six challenged GitHub context keys")
			Expect(sourceEntrypointSnapshot(owner)).To(Equal(beforeOwner))
			Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
			if failedProof {
				Expect(observed.status).To(Equal(1), "%+v", observed)
				Expect(observed.stdout).To(ContainSubstring("selftest: 0 passed, 1 failed"))
				Expect(observed.stdout).NotTo(ContainSubstring("factory doctor: healthy"))
			} else {
				Expect(observed.status).To(Equal(0), "%+v", observed)
				Expect(observed.stdout).To(ContainSubstring("selftest: 7 passed, 0 failed"))
				Expect(observed.stdout).To(ContainSubstring("factory doctor: healthy"))
			}
			Expect(observed.stderr).To(BeEmpty())
			Expect(observed.stdout).To(ContainSubstring("repo:   " + caller + "\n"))
			Expect(observed.stdout).To(ContainSubstring("config: doctor-selected.yaml\n"))
			Expect(observed.stdout).To(ContainSubstring("implementer cannot edit: 'caller-spec-*.go'"))
			Expect(observed.stdout).NotTo(ContainSubstring("wrong-owner-"))
			Expect(observed.stdout).NotTo(ContainSubstring("wrong-cwd-"))
			Expect(observed.stdout).NotTo(ContainSubstring("INHERITED_INPUT"))
			Expect(observed.stdout).NotTo(ContainSubstring("Usage:"))
			Expect(os.ReadFile(record)).To(Equal([]byte(caller+"\x00")), "the caller proof must run with selected-root cwd and empty input")
			liveAfter, err := os.Lstat(live)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.SameFile(liveBefore, liveAfter)).To(BeTrue())
			Expect(liveAfter.Mode()).To(Equal(liveBefore.Mode()))
			Expect(liveAfter.ModTime()).To(Equal(liveBefore.ModTime()))
			Expect(os.ReadFile(live)).To(Equal(liveBytes))
			_, err = os.Lstat(marker)
			Expect(os.IsNotExist(err)).To(BeTrue(), "neither owner shell library nor owner proof may execute")
			AddReportEntry("source-doctor.live-adapter", map[string]any{"failed_proof": failedProof, "same_inode": os.SameFile(liveBefore, liveAfter), "mode": liveAfter.Mode().String(), "proof_cwd": caller}, ReportEntryVisibilityNever)
		}
	})

	// per docs/adr/0129-go-source-doctor-entrypoint.md:19
	// per docs/adr/0129-go-source-doctor-entrypoint.md:21
	It("forwards literal doctor operands through its physical owner including newline paths and borrowed input", func() {
		operands := []string{"", "space and ' quote", "literal $(not-code)\nline", "--", "--json"}
		input := "original stdin\nliteral $(not-code)\x00tail\n"
		var violations []string
		const verb = "doctor"
		for _, alias := range []bool{false, true} {
			root, caller := fixtureDirectories()
			base := filepath.Dir(root)
			owner := filepath.Join(base, "physical owner ' $(not-code) \n")
			Expect(os.Rename(root, owner)).To(Succeed())
			physicalCaller := filepath.Join(base, "caller ' $(not-code) \n")
			Expect(os.Rename(caller, physicalCaller)).To(Succeed())
			program := sourceObservabilityAdapter(owner, verb)
			writeFixture(filepath.Join(owner, "factory"), []byte(sourceEntrypointRecorder), 0755)
			record, capturedInput := filepath.Join(base, "argv"), filepath.Join(base, "input")
			decoy := filepath.Join(base, "DECOY_EXECUTED")
			foreign := filepath.Join(base, "foreign logical owner")
			tools := filepath.Join(base, "caller tools")
			decoyBody := []byte("#!/bin/sh\nprintf called > \"$SOURCE_ENTRY_DECOY\"\nexit 64\n")
			for _, path := range []string{filepath.Join(foreign, "factory"), filepath.Join(physicalCaller, "factory"), filepath.Join(tools, "factory")} {
				writeFixture(path, decoyBody, 0755)
			}
			environment := map[string]string{
				"PATH": tools + string(os.PathListSeparator) + "/usr/bin:/bin", "BASH_ENV": "", "ENV": "",
				"SOURCE_ENTRY_ARGV": record, "SOURCE_ENTRY_INPUT": capturedInput, "SOURCE_ENTRY_DECOY": decoy,
				"SOURCE_ENTRY_LITERAL_ENV": "literal environment $(not-code)\nvalue\n",
				"FACTORY_RUNTIME_BINARY":   filepath.Join(foreign, "factory"), "FACTORY_HOME": foreign,
			}
			if alias {
				Expect(os.Symlink(filepath.Join(owner, "scripts"), filepath.Join(foreign, "scripts"))).To(Succeed())
				program = filepath.Join(foreign, "scripts", "factory-"+verb+".sh")
			}
			beforeOwner, beforeCaller, beforeForeign := installationSnapshot(owner), installationSnapshot(physicalCaller), installationSnapshot(foreign)
			observed := handoffNativeExactProcess(physicalCaller, environment, input, program, operands...)
			Expect(installationSnapshot(owner)).To(Equal(beforeOwner))
			Expect(installationSnapshot(physicalCaller)).To(Equal(beforeCaller))
			Expect(installationSnapshot(foreign)).To(Equal(beforeForeign))
			raw, recordErr := os.ReadFile(record)
			stdin, inputErr := os.ReadFile(capturedInput)
			fields := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
			_, decoyErr := os.Lstat(decoy)
			AddReportEntry("source-doctor.literal-transport", map[string]any{"verb": verb, "directory_alias": alias, "status": observed.status, "argv": fields, "stdin": string(stdin), "stderr": observed.stderr, "decoy_executed": decoyErr == nil}, ReportEntryVisibilityNever)
			violations = append(violations, InterceptGomegaFailures(func() {
				Expect(observed).To(Equal(cliResult{"", "", 63}), "verb=%s directory_alias=%t", verb, alias)
				Expect(recordErr).NotTo(HaveOccurred())
				Expect(inputErr).NotTo(HaveOccurred())
				Expect(strings.HasSuffix(string(raw), "\x00")).To(BeTrue())
				Expect(fields).To(Equal(append([]string{physicalCaller, environment["SOURCE_ENTRY_LITERAL_ENV"], verb}, operands...)))
				Expect(string(stdin)).To(Equal(input))
				Expect(os.IsNotExist(decoyErr)).To(BeTrue())
			})...)
		}
		AddReportEntry("source-doctor.transport-violations", violations, ReportEntryVisibilityNever)
		Expect(violations).To(BeEmpty())
	})

	// per docs/adr/0129-go-source-doctor-entrypoint.md:19
	// per docs/adr/0129-go-source-doctor-entrypoint.md:23
	It("refuses file aliases and missing owners and propagates failed owners without fallback", func() {
		var violations []string
		const verb = "doctor"
		for _, mode := range []string{"missing-owner", "failed-owner", "file-alias"} {
			root, caller := fixtureDirectories()
			base := filepath.Dir(root)
			program := sourceObservabilityAdapter(root, verb)
			marker, ownerCalled := filepath.Join(base, "FALLBACK_CALLED"), filepath.Join(base, "OWNER_CALLED")
			sourceObservabilityPoison(root)
			if mode != "missing-owner" {
				writeFixture(filepath.Join(root, "factory"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_OBSERVABILITY_OWNER\"\nprintf 'native output\\n'\nprintf 'native error\\n' >&2\nexit 47\n"), 0755)
			}
			if mode == "file-alias" {
				alias := filepath.Join(caller, "factory-"+verb+".sh")
				Expect(os.Symlink(program, alias)).To(Succeed())
				program = alias
			}
			environment := map[string]string{"PATH": "/usr/bin:/bin", "BASH_ENV": "", "ENV": "", "SOURCE_OBSERVABILITY_FALLBACK": marker, "SOURCE_OBSERVABILITY_OWNER": ownerCalled}
			beforeRoot, beforeCaller := installationSnapshot(root), installationSnapshot(caller)
			observed := handoffNativeExactProcess(caller, environment, "borrowed stdin\n", program)
			Expect(installationSnapshot(root)).To(Equal(beforeRoot))
			Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
			_, fallbackErr := os.Lstat(marker)
			_, ownerErr := os.Lstat(ownerCalled)
			AddReportEntry("source-doctor.refusal", map[string]any{"verb": verb, "mode": mode, "status": observed.status, "stdout": observed.stdout, "stderr": observed.stderr, "fallback_called": fallbackErr == nil, "owner_called": ownerErr == nil}, ReportEntryVisibilityNever)
			violations = append(violations, InterceptGomegaFailures(func() {
				Expect(os.IsNotExist(fallbackErr)).To(BeTrue(), "%s/%s must not run a shell fallback", verb, mode)
				switch mode {
				case "failed-owner":
					Expect(observed).To(Equal(cliResult{"native output\n", "native error\n", 47}))
					Expect(ownerErr).NotTo(HaveOccurred())
				case "file-alias":
					Expect(observed.status).To(Equal(2))
					Expect(observed.stdout).To(BeEmpty())
					Expect(observed.stderr).To(ContainSubstring("file symlink"))
					Expect(os.IsNotExist(ownerErr)).To(BeTrue(), "refuse the alias before any owner runs")
				case "missing-owner":
					Expect(observed.status).NotTo(Equal(0))
					Expect(observed.stdout).To(BeEmpty())
					Expect(observed.stderr).NotTo(BeEmpty())
					Expect(os.IsNotExist(ownerErr)).To(BeTrue())
				default:
					Fail(fmt.Sprintf("unknown private case %q", mode))
				}
			})...)
		}
		AddReportEntry("source-doctor.refusal-violations", violations, ReportEntryVisibilityNever)
		Expect(violations).To(BeEmpty())
	})
})
