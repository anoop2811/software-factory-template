//go:build publishedrelease

package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const publishedRepository = "anoop2811/software-factory-template"

type publishedAsset struct {
	Name          string `json:"name"`
	SHA256        string `json:"sha256"`
	Authenticated bool   `json:"authenticated"`
}

type publishedEvidence struct {
	SchemaVersion      int               `json:"schema_version"`
	Repository         string            `json:"repository"`
	Tag                string            `json:"tag"`
	Revision           string            `json:"revision"`
	HarnessCommit      string            `json:"harness_commit"`
	ReleaseID          int64             `json:"release_id"`
	Target             string            `json:"target"`
	OSVersion          string            `json:"os_version"`
	Architecture       string            `json:"architecture"`
	RunID              string            `json:"run_id"`
	RunAttempt         string            `json:"run_attempt"`
	MinimumOSQualified bool              `json:"minimum_os_qualified"`
	MinimumOSGap       string            `json:"minimum_os_gap,omitempty"`
	Assets             []publishedAsset  `json:"assets"`
	Scenarios          map[string]string `json:"scenarios"`
}

type publishedFixture struct {
	root, installer, bootstrap, archive, bundle, roots, gh, evidenceDirectory string
	environment                                                               map[string]string
	runtime                                                                   []byte
	evidence                                                                  publishedEvidence
	prepared                                                                  map[string]installationFixtureData
}

func (fixture *publishedFixture) saveEvidence() {
	GinkgoHelper()
	data, err := json.MarshalIndent(fixture.evidence, "", "  ")
	Expect(err).NotTo(HaveOccurred())
	Expect(os.WriteFile(filepath.Join(fixture.evidenceDirectory, "release.json"), append(data, '\n'), 0600)).To(Succeed())
}

// The evaluator invokes an independently installed verifier. No stub, local
// package builder or candidate executable supplies this release identity.
// per docs/adr/0107-published-release-qualification.md:59
func publishedAcquire() *publishedFixture {
	GinkgoHelper()
	tag, revision, destination := os.Getenv("FACTORY_PUBLISHED_TAG"), os.Getenv("FACTORY_PUBLISHED_REVISION"), os.Getenv("FACTORY_PUBLISHED_EVIDENCE_DIR")
	Expect(regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`).MatchString(tag)).To(BeTrue(), "explicit live qualification requires a literal FACTORY_PUBLISHED_TAG")
	Expect(strings.ToLower(tag)).NotTo(BeElementOf("main", "head", "latest", "current"))
	Expect(regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(revision)).To(BeTrue(), "explicit live qualification requires the full FACTORY_PUBLISHED_REVISION")
	Expect(filepath.IsAbs(destination)).To(BeTrue(), "explicit live qualification requires an absolute FACTORY_PUBLISHED_EVIDENCE_DIR")
	Expect(os.MkdirAll(destination, 0700)).To(Succeed()) // #nosec G703 -- explicit absolute evidence destination from the trusted qualification runner, not release content.
	root, err := filepath.EvalSymlinks(GinkgoT().TempDir())
	Expect(err).NotTo(HaveOccurred())
	gh, err := exec.LookPath("gh")
	Expect(err).NotTo(HaveOccurred(), "the live lane requires the real trusted GitHub CLI")
	gh, err = filepath.EvalSymlinks(gh)
	Expect(err).NotTo(HaveOccurred())
	fixture := &publishedFixture{root: root, gh: gh, evidenceDirectory: destination, environment: map[string]string{"GH_HOST": "github.com", "GH_PROMPT_DISABLED": "1", "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull, "PUBLISHED_EVIDENCE_DIR": destination}, prepared: map[string]installationFixtureData{}}
	target := runtime.GOOS + "/" + runtime.GOARCH
	if requested := os.Getenv("FACTORY_PUBLISHED_TARGET"); requested != "" {
		Expect(target).To(Equal(requested), "qualification must execute on the requested native architecture")
	}
	fixture.evidence = publishedEvidence{SchemaVersion: 1, Repository: publishedRepository, Tag: tag, Revision: revision, Target: target, Architecture: runtime.GOARCH, RunID: os.Getenv("GITHUB_RUN_ID"), RunAttempt: os.Getenv("GITHUB_RUN_ATTEMPT"), Scenarios: map[string]string{}}
	harness := publishedExecute("..", fixture.environment, "", "git", "rev-parse", "HEAD")
	Expect(harness.status).To(Equal(0), "%+v", harness)
	fixture.evidence.HarnessCommit = strings.TrimSpace(harness.stdout)
	osCommand, osArgs := "uname", []string{"-srv"}
	if runtime.GOOS == "darwin" {
		osCommand, osArgs = "sw_vers", []string{"-productVersion"}
	}
	observedOS := publishedExecute(".", fixture.environment, "", osCommand, osArgs...)
	Expect(observedOS.status).To(Equal(0), "%+v", observedOS)
	fixture.evidence.OSVersion = strings.TrimSpace(observedOS.stdout)
	if runtime.GOOS == "linux" {
		body, readErr := os.ReadFile("/etc/os-release")
		Expect(readErr).NotTo(HaveOccurred())
		fixture.evidence.OSVersion = string(body) + "\n" + fixture.evidence.OSVersion
		fixture.evidence.MinimumOSQualified = bytes.Contains(body, []byte(`ID=ubuntu`)) && bytes.Contains(body, []byte(`VERSION_ID="24.04"`))
	} else {
		fixture.evidence.MinimumOSQualified = strings.HasPrefix(fixture.evidence.OSVersion, "14.")
	}
	if !fixture.evidence.MinimumOSQualified {
		fixture.evidence.MinimumOSGap = "Observed newer/different OS does not establish the approved Ubuntu 24.04 or macOS 14 native minimum; that release gate remains open."
	}
	fixture.saveEvidence()
	view := publishedExecute(root, fixture.environment, "", gh, "release", "view", tag, "--repo", publishedRepository, "--json", "databaseId,tagName,isDraft")
	Expect(view.status).To(Equal(0), "real published release lookup failed: %+v", view)
	var release struct {
		ID    int64  `json:"databaseId"`
		Tag   string `json:"tagName"`
		Draft bool   `json:"isDraft"`
	}
	Expect(json.Unmarshal([]byte(view.stdout), &release)).To(Succeed())
	Expect(release.Tag).To(Equal(tag))
	Expect(release.Draft).To(BeFalse())
	Expect(release.ID).To(BeNumerically(">", 0))
	fixture.evidence.ReleaseID = release.ID
	resolved := publishedExecute(root, fixture.environment, "", gh, "api", "repos/"+publishedRepository+"/commits/refs/tags/"+tag, "--jq", ".sha")
	Expect(resolved.status).To(Equal(0), "%+v", resolved)
	Expect(strings.TrimSpace(resolved.stdout)).To(Equal(revision))
	artifactTarget := strings.ReplaceAll(target, "/", "-")
	names := []string{"factory-bootstrap-" + tag + "-" + artifactTarget, "factory-runtime-" + tag + "-" + artifactTarget + ".tar.gz", "factory-installation-" + tag + "-" + artifactTarget + ".tar.gz"}
	args := []string{"release", "download", tag, "--repo", publishedRepository, "--dir", root, "--pattern", "runtime-attestations.jsonl"}
	for _, name := range names {
		args = append(args, "--pattern", name)
	}
	download := publishedExecute(root, fixture.environment, "", gh, args...)
	Expect(download.status).To(Equal(0), "real release assets unavailable: %+v", download)
	fixture.bundle, fixture.roots = filepath.Join(root, "runtime-attestations.jsonl"), filepath.Join(root, "trusted-root.jsonl")
	roots := publishedExecute(root, fixture.environment, "", gh, "attestation", "trusted-root")
	Expect(roots.status).To(Equal(0), "%+v", roots)
	Expect(roots.stdout).NotTo(BeEmpty())
	Expect(os.WriteFile(fixture.roots, []byte(roots.stdout), 0600)).To(Succeed())
	for _, name := range names {
		path := filepath.Join(root, name)
		body, readErr := os.ReadFile(path) // #nosec G703 -- validated literal asset basename inside the evaluator-owned private download directory.
		Expect(readErr).NotTo(HaveOccurred())
		digest := fmt.Sprintf("%x", sha256.Sum256(body))
		query := fmt.Sprintf(`any(.[]; any(.verificationResult.statement.subject[]; .name == %q and .digest.sha256 == %q))`, name, digest)
		verified := publishedExecute(root, fixture.environment, "", gh, "attestation", "verify", path, "--repo", publishedRepository, "--signer-workflow", publishedRepository+"/.github/workflows/runtime-release.yml", "--source-digest", revision, "--source-ref", "refs/tags/"+tag, "--bundle", fixture.bundle, "--custom-trusted-root", fixture.roots, "--deny-self-hosted-runners", "--format", "json", "--jq", query)
		Expect(verified.status).To(Equal(0), "real attestation refused %s: %+v", name, verified)
		Expect(strings.TrimSpace(verified.stdout)).To(Equal("true"), "verified exact subject name and digest required")
		fixture.evidence.Assets = append(fixture.evidence.Assets, publishedAsset{name, digest, true})
		fixture.saveEvidence()
	}
	fixture.bootstrap, fixture.archive = filepath.Join(root, names[0]), filepath.Join(root, names[2])
	fixture.runtime, err = os.ReadFile(fixture.bootstrap) // #nosec G703 -- exact previously authenticated raw asset in the evaluator-owned private directory.
	Expect(err).NotTo(HaveOccurred())
	// Only authenticated bytes become executable. Git supplies the installer
	// from that same signed source revision, not the harness's current file.
	// per docs/adr/0107-published-release-qualification.md:68
	Expect(os.Chmod(fixture.bootstrap, 0700)).To(Succeed()) // #nosec G302 G703 -- exact previously authenticated subject in the evaluator-owned private directory, owner-only executable mode.
	installer := publishedExecute("..", fixture.environment, "", "git", "show", revision+":install.sh")
	Expect(installer.status).To(Equal(0), "candidate installer must exist at its verified source commit: %+v", installer)
	fixture.installer = filepath.Join(root, "published-install.sh")
	Expect(os.WriteFile(fixture.installer, []byte(installer.stdout), 0600)).To(Succeed())
	poison := filepath.Join(root, "unavailable-adopter-tools")
	for _, tool := range []string{"go", "python", "python3", "claude", "codex", "opencode", "ollama"} {
		writeFixture(filepath.Join(poison, tool), []byte("#!/bin/sh\n{ printf 'tool=%s parent_pid=%s argv=' \"$0\" \"${PPID-unknown}\"; printf ' %s' \"$@\"; printf '\\n'; } >> \"$PUBLISHED_FORBIDDEN\"\nexit 97\n"), 0700)
	}
	fixture.environment["PATH"] = poison + string(os.PathListSeparator) + os.Getenv("PATH")
	fixture.environment["PUBLISHED_FORBIDDEN"] = filepath.Join(root, "forbidden-tool")
	fixture.saveEvidence()
	return fixture
}

// Keep the original poison flag and retain its contents with the enclosing
// command boundary. A probe is a hard failure, including when the child exits 0.
// per docs/DECISION_LOG.md:4860
func publishedAssertNoForbiddenTool(cwd string, environment map[string]string, program string, args []string) {
	GinkgoHelper()
	path := environment["PUBLISHED_FORBIDDEN"]
	if path == "" {
		return
	}
	observed, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return
	}
	Expect(err).NotTo(HaveOccurred(), "cannot inspect forbidden-tool evidence after %q %q", program, args)
	body, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred(), "cannot read forbidden-tool evidence after %q %q", program, args)
	evidence := fmt.Sprintf("program=%q\narguments=%q\ncwd=%q\nforbidden_tool_probe:\n%s", program, args, cwd, body)
	_, _ = fmt.Fprintln(GinkgoWriter, evidence)
	if destination := environment["PUBLISHED_EVIDENCE_DIR"]; destination != "" {
		Expect(os.WriteFile(filepath.Join(destination, "forbidden-tool-evidence.txt"), []byte(evidence), 0600)).To(Succeed()) // #nosec G703 -- fixed basename under the trusted output validated by publishedAcquire or an evaluator-owned TempDir; candidate content cannot select the destination.
	}
	Expect(observed).To(BeNil(), "published Go dependency control observed a forbidden tool at this command boundary:\n%s", evidence)
}

func publishedExecute(cwd string, environment map[string]string, input, program string, args ...string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	command := handoffCommand(ctx, cwd, environment, input, program, args...)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.WaitDelay = time.Second
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	publishedAssertNoForbiddenTool(cwd, environment, program, args)
	Expect(ctx.Err()).NotTo(HaveOccurred(), "published command exceeded its deadline: %s", stderr.String())
	status := 0
	if err != nil {
		var exit *exec.ExitError
		Expect(errors.As(err, &exit)).To(BeTrue(), "%v", err)
		status = exit.ExitCode()
	}
	return cliResult{stdout.String(), stderr.String(), status}
}

func (fixture *publishedFixture) preview(root string) map[string]any {
	GinkgoHelper()
	before := installationSnapshot(root)
	result := publishedExecute(root, fixture.environment, "", "/bin/sh", fixture.installer, "upgrade", "--go-candidate", fixture.evidence.Tag, "--dry-run", "--json")
	Expect(installationSnapshot(root)).To(Equal(before), "published preview cannot write the project")
	proposal := handoffProposal(result)
	Expect(proposal["blockers"]).To(BeEmpty())
	return proposal
}

func (fixture *publishedFixture) installed(root string) {
	GinkgoHelper()
	Expect(os.ReadFile(filepath.Join(root, ".factory/bin/factory-runtime"))).To(Equal(fixture.runtime))
	data, err := os.ReadFile(filepath.Join(root, ".factory/installation.current"))
	Expect(err).NotTo(HaveOccurred())
	var descriptor map[string]any
	Expect(json.Unmarshal(data, &descriptor)).To(Succeed())
	Expect(descriptor["version"]).To(Equal(fixture.evidence.Tag))
	Expect(descriptor["revision"]).To(Equal(fixture.evidence.Revision))
	for _, args := range [][]string{{"--help"}, {"selftest"}, {"doctor"}} {
		result := publishedExecute(root, fixture.environment, "", filepath.Join(root, "factory"), args...)
		Expect(result.status).To(Equal(0), "installed public %q failed: %+v", args, result)
	}
	for _, path := range []string{".codex/config.toml", ".claude/settings.json", "opencode.json"} {
		data, err := os.ReadFile(filepath.Join(root, path))
		Expect(err).NotTo(HaveOccurred())
		Expect(data).NotTo(BeEmpty())
	}
	_, err = os.Lstat(fixture.environment["PUBLISHED_FORBIDDEN"])
	Expect(os.IsNotExist(err)).To(BeTrue(), "Go/Python/model clients cannot be a shipped Go dependency")
}

func (fixture *publishedFixture) recoveryArgs(profile, id, mode string) []string {
	args := []string{"upgrade", "--installation", "--source", fixture.archive, "--version", fixture.evidence.Tag, "--revision", fixture.evidence.Revision, "--target", fixture.evidence.Target, "--attestation", fixture.bundle, "--trusted-root", fixture.roots, "--profile", profile, mode, id}
	for _, input := range installationInputs {
		args = append(args, "--project-input", input)
	}
	if mode == "--recover" {
		args = append(args, "--direction", "forward")
	}
	return args
}

func publishedUserData(root, profile string) map[string][]byte {
	GinkgoHelper()
	values := map[string][]byte{"application.txt": []byte("application remains operator owned\n"), "scripts/operator-custom.sh": []byte("#!/bin/sh\nprintf custom\n"), ".factory/events.log": []byte("2026-10-19T00:00:00Z\toperator\tqualification history retained\n")}
	// v0.1.6 has no controller schema: inventing modern files would correctly
	// make its rollback unsafe. The later Bash profile already consumes these
	// independently qualified completed accounting/checkpoint records.
	// per docs/adr/0107-published-release-qualification.md:100
	if profile == "bash-baseline" {
		values[".factory/budget.json"] = budgetHistory(budgetRow("published-consumed-accounting"))
		values[".factory/loops.json"] = []byte(checkpointHistory(checkpointRecord))
	}
	for path, data := range values {
		writeFixture(filepath.Join(root, path), data, 0600)
	}
	return values
}

func publishedPreserved(root string, values map[string][]byte) {
	GinkgoHelper()
	for path, want := range values {
		actual, err := os.ReadFile(filepath.Join(root, path))
		Expect(err).NotTo(HaveOccurred())
		if path == ".factory/events.log" {
			Expect(bytes.HasPrefix(actual, want)).To(BeTrue())
		} else {
			Expect(actual).To(Equal(want), path)
		}
	}
}

// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:270
// per docs/DECISION_LOG.md:4842
func publishedRequiredRoleConflict(conflict cliResult, path string) {
	GinkgoHelper()
	Expect(conflict.status).To(Equal(2), "a preserved required conflict must return its documented status: %+v", conflict)
	proposal := installationDecodeProposal(conflict)
	for _, name := range []string{"blockers", "conflicts"} {
		Expect(proposal[name]).To(ContainElement(HaveKeyWithValue("path", path)), "the proposal must identify the customized required entry in %s", name)
	}
	row := installationRows(proposal)[path]
	Expect(row).NotTo(BeNil(), "the complete proposal must retain the required entry action")
	Expect(row["required"]).To(BeTrue())
	Expect(row["action"]).To(Equal("retain"))
	for _, name := range []string{"applicable", "checks_ready", "success", "rollback_ready"} {
		Expect(proposal[name]).To(BeFalse(), "blocked preview cannot claim %s", name)
	}
}

// The live lane and the offline regression execute the same deterministic
// evaluator controls before inspecting any authenticated candidate.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:264
// per docs/DECISION_LOG.md:4842
func publishedEvaluatorProof() {
	GinkgoHelper()
	path := "scripts/factory-report.sh"
	issue := map[string]any{"path": path, "reason": "operator customization is preserved"}
	proposal := map[string]any{
		"proposal_digest": strings.Repeat("a", 64),
		"actions":         []any{map[string]any{"path": path, "action": "retain", "required": true}},
		"blockers":        []any{issue}, "conflicts": []any{issue},
		"applicable": false, "checks_ready": false, "success": false, "rollback_ready": false,
	}
	encode := func(value map[string]any) cliResult {
		body, err := json.Marshal(value)
		Expect(err).NotTo(HaveOccurred())
		return cliResult{stdout: string(body), status: 2}
	}
	valid := encode(proposal)
	Expect(InterceptGomegaFailure(func() { publishedRequiredRoleConflict(valid, path) })).To(Succeed(), "status 2 carries the complete blocked proposal on stdout with empty stderr")
	changed := func(field string, value any) cliResult {
		var altered map[string]any
		Expect(json.Unmarshal([]byte(valid.stdout), &altered)).To(Succeed())
		altered[field] = value
		return encode(altered)
	}
	controls := []struct {
		name   string
		result cliResult
	}{
		{"operational inspection failure", cliResult{stderr: "cannot inspect installation", status: 1}},
		{"parser refusal without a proposal", cliResult{stderr: "invalid operand", status: 2}},
		{"malformed JSON", cliResult{stdout: "{", status: 2}},
		{"trailing JSON", cliResult{stdout: valid.stdout + "\n{}", status: 2}},
		{"a status zero masquerading as a preserved mandatory conflict", cliResult{stdout: valid.stdout, status: 0}},
		{"error-stream contamination", cliResult{stdout: valid.stdout, stderr: "partial inspection", status: 2}},
		{"missing blocker", changed("blockers", []any{})},
		{"blocker for another path", changed("blockers", []any{map[string]any{"path": "scripts/other.sh", "reason": "unrelated"}})},
		{"conflict for another path", changed("conflicts", []any{map[string]any{"path": "scripts/other.sh", "reason": "unrelated"}})},
		{"an optional retained action", changed("actions", []any{map[string]any{"path": path, "action": "retain", "required": false}})},
		{"a replacing action", changed("actions", []any{map[string]any{"path": path, "action": "replace", "required": true}})},
		{"an authority claim despite the conflict", changed("success", true)},
	}
	for _, control := range controls {
		By(control.name)
		Expect(InterceptGomegaFailure(func() { publishedRequiredRoleConflict(control.result, path) })).To(HaveOccurred(), control.name)
	}

	// A controlled probe must fail with retained bytes and command context;
	// qualification must never clear the flag to manufacture later success.
	// per docs/DECISION_LOG.md:4860
	root := GinkgoT().TempDir()
	marker := filepath.Join(root, "forbidden-tool")
	environment := map[string]string{"PUBLISHED_FORBIDDEN": marker, "PUBLISHED_EVIDENCE_DIR": root}
	arguments := []string{"published-install.sh", "init", "fixture-adopter"}
	Expect(InterceptGomegaFailure(func() { publishedAssertNoForbiddenTool(root, environment, "/bin/sh", arguments) })).To(Succeed())
	probe := []byte("tool=python3 parent_pid=123 argv= --version\n")
	Expect(os.WriteFile(marker, probe, 0600)).To(Succeed())
	failure := InterceptGomegaFailure(func() { publishedAssertNoForbiddenTool(root, environment, "/bin/sh", arguments) })
	Expect(failure).To(HaveOccurred())
	Expect(failure.Error()).To(ContainSubstring("python3"))
	Expect(failure.Error()).To(ContainSubstring("fixture-adopter"))
	Expect(os.ReadFile(marker)).To(Equal(probe))
	retained, err := os.ReadFile(filepath.Join(root, "forbidden-tool-evidence.txt"))
	Expect(err).NotTo(HaveOccurred())
	Expect(string(retained)).To(ContainSubstring("program=\"/bin/sh\""))
	Expect(string(retained)).To(ContainSubstring("fixture-adopter"))
	Expect(string(retained)).To(ContainSubstring(string(probe)))
}

var _ = Describe("Published release qualification evaluator", func() {
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:264
	// per docs/DECISION_LOG.md:4842
	It("distinguishes a machine-readable required conflict from malformed and operational failures", publishedEvaluatorProof)
})

// These criteria are never registered by ordinary source compilation. Missing
// explicit live inputs fail; no environment-dependent skip can masquerade as a pass.
// per docs/adr/0107-published-release-qualification.md:79
var _ = Describe("Published release qualification live", Ordered, ContinueOnFailure, func() {
	var release *publishedFixture
	BeforeAll(func() {
		publishedEvaluatorProof()
		release = publishedAcquire()
		for _, reference := range []installationReference{{profile: "v0.1.6", revision: "b71ecc32e07ecd87eb330ba8e497c86612f92acd"}, {profile: "bash-baseline", revision: "76952eaa63aebd1ecd282f5ab51dd7c3627cb497"}} {
			reference.assets, reference.payload = installationGitCatalog("..", reference.revision)
			release.prepared[reference.profile] = installationLegacyFixture(candidateBinary, reference, release.archive, release.evidence.Revision)
		}
	})
	ReportAfterEach(func(report SpecReport) {
		if release != nil {
			release.evidence.Scenarios[report.FullText()] = report.State.String()
			release.saveEvidence()
		}
	})

	// per docs/adr/0107-published-release-qualification.md:89
	It("initializes and runs the authenticated published Go selection without compiler interpreter or model clients", func() {
		root := filepath.Join(release.root, "fresh-adopter")
		result := publishedExecute(release.root, release.environment, nativeInitAnswers("y"), "/bin/sh", release.installer, "init", root, "--go-candidate", release.evidence.Tag, "--pack", "none")
		Expect(result.status).To(Equal(0), "real public initializer failed: %+v", result)
		release.installed(root)
	})

	// per docs/adr/0107-published-release-qualification.md:92
	// per docs/adr/0107-published-release-qualification.md:104
	DescribeTable("upgrades a genuinely initialized predecessor and rolls back from its ignored inert recovery", func(profile string) {
		legacy := installationCloneLegacyFixture(candidateBinary, release.prepared[profile])
		values := publishedUserData(legacy.root, profile)
		imageGit(legacy.root, "add", "--", "application.txt", "scripts/operator-custom.sh")
		imageGit(legacy.root, "commit", "--quiet", "-m", "qualification adopter history")
		headBefore, indexBefore := imageGit(legacy.root, "rev-parse", "HEAD"), imageGit(legacy.root, "ls-files", "--stage", "-z")
		oldFactory, err := os.ReadFile(filepath.Join(legacy.root, "factory"))
		Expect(err).NotTo(HaveOccurred())
		// A customized required role remains a preserved blocker, never adoption permission.
		role := filepath.Join(legacy.root, "scripts/factory-report.sh")
		original, err := os.ReadFile(role)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(role, append(append([]byte{}, original...), []byte("\n# operator required-role edit\n")...), 0700)).To(Succeed()) // #nosec G306 -- test-owned existing executable; preserve its existing fixture mode.
		before := installationSnapshot(legacy.root)
		conflict := publishedExecute(legacy.root, release.environment, "", "/bin/sh", release.installer, "upgrade", "--go-candidate", release.evidence.Tag, "--dry-run", "--json")
		Expect(installationSnapshot(legacy.root)).To(Equal(before))
		publishedRequiredRoleConflict(conflict, "scripts/factory-report.sh")
		Expect(os.WriteFile(role, original, 0700)).To(Succeed()) // #nosec G306 -- same owned fixture executable restored before clean preview.
		proposal := release.preview(legacy.root)
		result := publishedExecute(legacy.root, release.environment, "", "/bin/sh", release.installer, "upgrade", "--go-candidate", release.evidence.Tag, "--confirm", proposal["proposal_digest"].(string), "--quiescent")
		Expect(result.status).To(Equal(0), "real published migration failed: %+v", result)
		release.installed(legacy.root)
		publishedPreserved(legacy.root, values)
		Expect(imageGit(legacy.root, "rev-parse", "HEAD")).To(Equal(headBefore))
		Expect(imageGit(legacy.root, "ls-files", "--stage", "-z")).To(Equal(indexBefore))
		ids := retentionBackupIDs(legacy.root)
		Expect(ids).To(HaveLen(1))
		id := ids[0]
		ignored := publishedExecute(legacy.root, release.environment, "", "git", "check-ignore", "--no-index", "--", ".factory/backups/"+id+"/installation.manifest")
		Expect(ignored.status).To(Equal(0), "recovery must be ignored: %+v", ignored)
		Expect(filepath.WalkDir(filepath.Join(legacy.root, ".factory/backups", id), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			Expect(info.Mode().Perm()&0111).To(BeZero(), "saved recovery is inert: %s", path)
			return nil
		})).To(Succeed())
		for _, retired := range []string{"budget.py", "budget_adapters.py", "loop.py", "runtime_transition.py"} {
			_, err := os.Lstat(filepath.Join(legacy.root, "scripts/lib", retired))
			Expect(os.IsNotExist(err)).To(BeTrue(), retired)
		}
		values["application.txt"] = []byte("application changed after upgrade\n")
		writeFixture(filepath.Join(legacy.root, "application.txt"), values["application.txt"], 0600)
		args := release.recoveryArgs(profile, id, "--rollback")
		reverse := handoffProposal(publishedExecute(legacy.root, release.environment, "", release.bootstrap, append(append([]string{}, args...), "--dry-run", "--json")...))
		// Restored Bash health uses its established tools, separately from the Go-only PATH proof.
		legacyEnvironment := map[string]string{"GH_HOST": "github.com", "GH_PROMPT_DISABLED": "1", "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull}
		rolled := publishedExecute(legacy.root, legacyEnvironment, "", release.bootstrap, append(args, "--confirm", reverse["proposal_digest"].(string), "--quiescent")...)
		Expect(rolled.status).To(Equal(0), "real retained rollback failed: %+v", rolled)
		Expect(os.ReadFile(filepath.Join(legacy.root, "factory"))).To(Equal(oldFactory))
		publishedPreserved(legacy.root, values)
		Expect(imageGit(legacy.root, "rev-parse", "HEAD")).To(Equal(headBefore))
		Expect(imageGit(legacy.root, "ls-files", "--stage", "-z")).To(Equal(indexBefore))
		health := publishedExecute(legacy.root, legacyEnvironment, "", "/bin/bash", filepath.Join(legacy.root, "scripts/selftest/run.sh"))
		Expect(health.status).To(Equal(0), "restored baseline proof failed: %+v", health)
	}, Entry("v0.1.6", "v0.1.6"), Entry("immutable Bash baseline", "bash-baseline"))

	// per docs/adr/0107-published-release-qualification.md:109
	DescribeTable("recovers a witnessed interrupted published upgrade without changing signed inputs", func(profile string) {
		legacy := installationCloneLegacyFixture(candidateBinary, release.prepared[profile])
		values := publishedUserData(legacy.root, profile)
		imageGit(legacy.root, "add", "--", "application.txt", "scripts/operator-custom.sh")
		imageGit(legacy.root, "commit", "--quiet", "-m", "qualification adopter history")
		headBefore, indexBefore := imageGit(legacy.root, "rev-parse", "HEAD"), imageGit(legacy.root, "ls-files", "--stage", "-z")
		proposal := release.preview(legacy.root)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		command := handoffCommand(ctx, legacy.root, release.environment, "", release.bootstrap, []string{"upgrade", "--go-candidate", release.evidence.Tag, "--confirm", proposal["proposal_digest"].(string), "--quiescent"}...)
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		command.WaitDelay = time.Second
		command.Cancel = func() error {
			if command.Process == nil {
				return nil
			}
			return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		}
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		Expect(command.Start()).To(Succeed())
		finished := make(chan error, 1)
		go func() { finished <- command.Wait() }()
		waited := false
		DeferCleanup(func() {
			if !waited {
				_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
				<-finished
			}
		})
		pending := filepath.Join(legacy.root, ".factory/runtime-publication.pending")
		var migration string
		observe := func() bool {
			select {
			case err := <-finished:
				waited = true
				StopTrying(fmt.Sprintf("published candidate exited before durable crash witness: %v", err)).Now()
			default:
			}
			paths, err := filepath.Glob(filepath.Join(legacy.root, ".factory/installation-transactions/candidate-*.json"))
			if err != nil || len(paths) != 1 {
				return false
			}
			body, err := os.ReadFile(paths[0])
			if err != nil {
				return false
			}
			var journal struct {
				Phase   string `json:"phase"`
				Entries []struct {
					Path  string `json:"path"`
					Phase string `json:"phase"`
				} `json:"entries"`
			}
			if json.Unmarshal(body, &journal) != nil || journal.Phase != "applying" {
				return false
			}
			applied, runtimeApplied := 0, false
			for _, entry := range journal.Entries {
				if entry.Phase == "applied" {
					applied++
					if entry.Path == ".factory/bin/factory-runtime" {
						runtimeApplied = true
					}
				}
			}
			_, err = os.Lstat(pending)
			if applied < 2 || !runtimeApplied || err != nil {
				return false
			}
			migration = strings.TrimSuffix(filepath.Base(paths[0]), ".json")
			return true
		}
		Eventually(observe, 3*time.Minute, time.Millisecond).Should(BeTrue(), "must observe durable applied rows under pending, not perform a timed kill")
		Expect(syscall.Kill(command.Process.Pid, syscall.SIGSTOP)).To(Succeed())
		// A signal acknowledgement is not the stopped-process witness.
		// per docs/DECISION_LOG.md:4695.
		Eventually(func() bool {
			select {
			case err := <-finished:
				waited = true
				StopTrying(fmt.Sprintf("published candidate exited before observed stop: %v", err)).Now()
			default:
			}
			stopped, stopErr := installationProcessStopped(command.Process.Pid)
			Expect(stopErr).NotTo(HaveOccurred(), "observe actual published candidate stop before reading crash evidence")
			return stopped
		}, 5*time.Second, time.Millisecond).Should(BeTrue(), "must observe the actual process stopped before accepting its durable witness")
		// Re-read the stopped process's actual durable boundary. A completed
		// transaction between the poll and SIGSTOP must fail this proof closed.
		stoppedBytes, err := os.ReadFile(filepath.Join(legacy.root, ".factory/installation-transactions", migration+".json"))
		Expect(err).NotTo(HaveOccurred())
		var stopped struct {
			Phase   string `json:"phase"`
			Entries []struct {
				Path  string `json:"path"`
				Phase string `json:"phase"`
			} `json:"entries"`
		}
		Expect(json.Unmarshal(stoppedBytes, &stopped)).To(Succeed())
		Expect(stopped.Phase).To(Equal("applying"), "the process must still be partially applying when actually stopped")
		stoppedApplied, stoppedRuntime := 0, false
		for _, entry := range stopped.Entries {
			if entry.Phase == "applied" {
				stoppedApplied++
				if entry.Path == ".factory/bin/factory-runtime" {
					stoppedRuntime = true
				}
			}
		}
		Expect(stoppedApplied).To(BeNumerically(">=", 2))
		Expect(stoppedRuntime).To(BeTrue())
		_, err = os.Lstat(pending)
		Expect(err).NotTo(HaveOccurred())
		Expect(syscall.Kill(-command.Process.Pid, syscall.SIGKILL)).To(Succeed())
		Expect(<-finished).To(HaveOccurred())
		waited = true
		publishedAssertNoForbiddenTool(legacy.root, release.environment, command.Args[0], command.Args[1:])
		journal, err := os.ReadFile(filepath.Join(legacy.root, ".factory/installation-transactions", migration+".json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(journal).To(Equal(stoppedBytes), "the killed transaction cannot advance its recorded witness")
		Expect(os.WriteFile(filepath.Join(release.evidenceDirectory, "interrupted-"+profile+".json"), stoppedBytes, 0600)).To(Succeed())
		_, err = os.Lstat(pending)
		Expect(err).NotTo(HaveOccurred())
		blocked := publishedExecute(legacy.root, release.environment, "", filepath.Join(legacy.root, ".factory/bin/factory-runtime"), "selftest")
		Expect(blocked.status).To(Equal(2))
		Expect(blocked.stderr).To(MatchRegexp(`(?i)(pending|maintenance|incomplete|admission unavailable)`))
		args := release.recoveryArgs(profile, migration, "--recover")
		recovery := handoffProposal(publishedExecute(legacy.root, release.environment, "", release.bootstrap, append(append([]string{}, args...), "--dry-run", "--json")...))
		result := publishedExecute(legacy.root, release.environment, "", release.bootstrap, append(args, "--confirm", recovery["proposal_digest"].(string), "--quiescent")...)
		Expect(result.status).To(Equal(0), "published recovery failed: %+v", result)
		release.installed(legacy.root)
		publishedPreserved(legacy.root, values)
		Expect(imageGit(legacy.root, "rev-parse", "HEAD")).To(Equal(headBefore))
		Expect(imageGit(legacy.root, "ls-files", "--stage", "-z")).To(Equal(indexBefore))
		_, err = os.Lstat(pending)
		Expect(os.IsNotExist(err)).To(BeTrue())
		before := installationSnapshot(legacy.root)
		repeated := handoffProposal(publishedExecute(legacy.root, release.environment, "", release.bootstrap, append(append([]string{}, args...), "--dry-run", "--json")...))
		Expect(installationSnapshot(legacy.root)).To(Equal(before), "repeated recovery preview is read-only")
		reapplied := publishedExecute(legacy.root, release.environment, "", release.bootstrap, append(append([]string{}, args...), "--confirm", repeated["proposal_digest"].(string), "--quiescent")...)
		Expect(reapplied.status).To(Equal(0), "completed public recovery must safely accept its freshly confirmed terminal replay: %+v", reapplied)
		Expect(installationSnapshot(legacy.root)).To(Equal(before), "terminal replay cannot republish or rewind state")
	}, Entry("v0.1.6", "v0.1.6"), Entry("immutable Bash baseline", "bash-baseline"))
})
