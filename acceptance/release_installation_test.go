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
	"runtime"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type handoffReleaseImage struct {
	binary, archive             []byte
	archivePath, revision, work string
	references                  map[string]installationReference
}

// per docs/adr/0101-go-release-handoff.md:210
func handoffBuildImage() handoffReleaseImage {
	GinkgoHelper()
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	work, err := os.MkdirTemp("", "factory-release-image-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, work)
	revision, binary, assets, payload := installationTargetSnapshot(repository, filepath.Join(work, "exact-source"))
	baselines := []imageBaselineOracle{}
	references := map[string]installationReference{}
	for _, reference := range []installationReference{{profile: "v0.1.6", revision: "b71ecc32e07ecd87eb330ba8e497c86612f92acd"}, {profile: "bash-baseline", revision: "76952eaa63aebd1ecd282f5ab51dd7c3627cb497"}} {
		reference.assets, reference.payload = installationGitCatalog(repository, reference.revision)
		references[reference.profile] = reference
		baselines = append(baselines, imageBaselineOracle{reference.profile, reference.revision, reference.assets})
	}
	manifest := imageManifestOracle{1, "installation_source_image", "committed_source_and_baseline_references", handoffVersion, runtime.GOOS + "/" + runtime.GOARCH, revision, false, assets, baselines}
	archive := stageArchive(imageFixtureEntries(manifest, payload, binary))
	path := filepath.Join(work, "candidate-image.tar.gz")
	writeFixture(path, archive, 0600)
	return handoffReleaseImage{binary, archive, path, revision, work, references}
}

// The running program and the authenticated image have the same actual compiled
// bytes; the image is built from its own committed private source snapshot.
// per docs/adr/0101-go-release-handoff.md:148
func handoffNativeFixture(image handoffReleaseImage) (handoffBootstrapFixture, string) {
	GinkgoHelper()
	qualifiedGit, err := exec.LookPath("git")
	Expect(err).NotTo(HaveOccurred())
	qualifiedGit, err = filepath.EvalSymlinks(qualifiedGit)
	Expect(err).NotTo(HaveOccurred())
	Expect(filepath.IsAbs(qualifiedGit)).To(BeTrue())
	// per docs/DECISION_LOG.md:4998
	AddReportEntry("release-handoff.resolved-git", qualifiedGit, ReportEntryVisibilityNever)
	f := handoffBootstrap(map[string]string{"linux": "Linux", "darwin": "Darwin"}[runtime.GOOS], map[string]string{"amd64": "x86_64", "arm64": "arm64"}[runtime.GOARCH], runtime.GOOS+"-"+runtime.GOARCH, "")
	assets := f.environment["HANDOFF_ASSETS"]
	writeFixture(filepath.Join(assets, "factory-bootstrap-"+handoffVersion+"-"+f.target), image.binary, 0600)
	writeFixture(filepath.Join(assets, "factory-installation-"+handoffVersion+"-"+f.target+".tar.gz"), image.archive, 0600)
	f.environment["HANDOFF_REVISION"] = image.revision
	// Git is the retained initializer boundary; compilers and Python remain
	// poisoned in this child PATH, independently of the evaluator's build tools.
	// per docs/DECISION_LOG.md:4951
	Expect(os.Remove(filepath.Join(f.root, "host tools", "git"))).To(Succeed())
	Expect(os.Symlink(qualifiedGit, filepath.Join(f.root, "host tools", "git"))).To(Succeed())
	writeFixture(filepath.Join(f.root, "host tools", "npm"), []byte("#!/bin/sh\nexit 0\n"), 0700)
	f.environment["FACTORY_BRIDGE_PROTOCOL"] = ""
	f.environment["GIT_CONFIG_NOSYSTEM"] = "1"
	f.environment["GIT_CONFIG_GLOBAL"] = os.DevNull
	f.environment["GORACE"] = "atexit_sleep_ms=0"
	f.environment["HOME"] = filepath.Join(f.root, "isolated-home")
	driver := filepath.Join(f.root, "trusted caller", "factory")
	writeFixture(driver, image.binary, 0700)
	return f, driver
}

func handoffNativeProcess(cwd string, environment map[string]string, input, program string, args ...string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := handoffCommand(ctx, cwd, environment, input, program, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred(), "candidate process watchdog: %s", stderr.String())
	status := 0
	if err != nil {
		var exit *exec.ExitError
		Expect(errors.As(err, &exit)).To(BeTrue())
		status = exit.ExitCode()
	}
	return cliResult{stdout.String(), stderr.String(), status}
}

func handoffCommand(ctx context.Context, cwd string, environment map[string]string, input, program string, args ...string) *exec.Cmd {
	// Published callers authenticate release bytes before selecting this executable.
	// per acceptance/published_release_test.go:69
	cmd := exec.CommandContext(ctx, program, args...) // #nosec G204 G702 -- acceptance-owned program and literal arguments; no shell interpolation.
	cmd.Dir = cwd
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "FACTORY_") || strings.HasPrefix(key, "GIT_") {
			continue
		}
		if _, replaced := environment[key]; !replaced {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	for key, value := range environment {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.Stdin = strings.NewReader(input)
	return cmd
}

func handoffImageVersion(image handoffReleaseImage, version string) []byte {
	GinkgoHelper()
	entries, _ := imageDecodeArchive(image.archive)
	for index := range entries {
		entries[index].name = strings.Replace(entries[index].name, handoffVersion+"/", version+"/", 1)
		if strings.HasSuffix(entries[index].name, "/runtime.manifest") || strings.HasSuffix(entries[index].name, "/source.json") || strings.HasSuffix(entries[index].name, "/installation.manifest") {
			entries[index].body = bytes.ReplaceAll(entries[index].body, []byte(handoffVersion), []byte(version))
		}
	}
	return stageArchive(entries)
}

func handoffImageBinary(image handoffReleaseImage, binary []byte, version string) []byte {
	GinkgoHelper()
	entries, _ := imageDecodeArchive(handoffImageVersion(image, version))
	Expect(len(entries)).To(BeNumerically(">=", 4), "the independent image fixture requires all four controls")
	old := fmt.Sprintf("%x", sha256.Sum256(entries[0].body))
	entries[0].body = binary
	entries[1].body = bytes.Replace(entries[1].body, []byte(old), []byte(fmt.Sprintf("%x", sha256.Sum256(binary))), 1)
	return stageArchive(entries)
}

// per docs/adr/0101-go-release-handoff.md:285
func handoffRewriteAsset(archive []byte, path string, change func([]byte) []byte) []byte {
	GinkgoHelper()
	entries, _ := imageDecodeArchive(archive)
	Expect(len(entries)).To(BeNumerically(">=", 4), "the independent image fixture requires all four controls")
	var manifest imageManifestOracle
	Expect(json.Unmarshal(entries[3].body, &manifest)).To(Succeed())
	found := false
	for index, asset := range manifest.Assets {
		if asset.Path != path {
			continue
		}
		found = true
		body := change(entries[4+index].body)
		entries[4+index].body = body
		manifest.Assets[index].Bytes = int64(len(body))
		manifest.Assets[index].SHA256 = fmt.Sprintf("%x", sha256.Sum256(body))
	}
	Expect(found).To(BeTrue(), "fixture asset must exist: %s", path)
	entries[3].body = imageMarshal(manifest)
	return stageArchive(entries)
}

func handoffProposal(result cliResult) map[string]any {
	GinkgoHelper()
	Expect(result.status).To(Equal(0), "%+v", result)
	var proposal map[string]any
	Expect(json.Unmarshal([]byte(result.stdout), &proposal)).To(Succeed())
	Expect(proposal["proposal_digest"]).To(MatchRegexp("^[a-f0-9]{64}$"))
	return proposal
}

func handoffPreview(f handoffBootstrapFixture, driver, target string) map[string]any {
	return handoffProposal(handoffNativeProcess(target, f.environment, "", driver, "upgrade", "--go-candidate", handoffVersion, "--dry-run", "--json"))
}

func handoffApply(f handoffBootstrapFixture, driver, target string, proposal map[string]any) cliResult {
	GinkgoHelper()
	return handoffNativeProcess(target, f.environment, "", driver, "upgrade", "--go-candidate", handoffVersion, "--confirm", proposal["proposal_digest"].(string), "--quiescent")
}

// per docs/adr/0101-go-release-handoff.md:119
func handoffImport(image handoffReleaseImage, f handoffBootstrapFixture) string {
	GinkgoHelper()
	cache := filepath.Join(f.root, "imported release")
	Expect(os.Mkdir(cache, 0700)).To(Succeed())
	writeFixture(filepath.Join(cache, "factory-bootstrap-"+handoffVersion+"-"+f.target), image.binary, 0600)
	writeFixture(filepath.Join(cache, "factory-installation-"+handoffVersion+"-"+f.target+".tar.gz"), image.archive, 0600)
	writeFixture(filepath.Join(cache, "runtime-attestations.jsonl"), []byte("fixture attestations"), 0600)
	writeFixture(filepath.Join(cache, "revision"), []byte(image.revision+"\n"), 0600)
	f.environment["FACTORY_RELEASE_DIR"] = cache
	return cache
}

// per docs/adr/0101-go-release-handoff.md:179
func handoffRecoveryOperands(image handoffReleaseImage, f handoffBootstrapFixture, beforePath string) []string {
	roots := filepath.Join(f.root, "operator-trusted-root.jsonl")
	writeFixture(roots, []byte("independently obtained fixture roots\n"), 0600)
	args := []string{"upgrade", "--installation", "--profile", "go-hybrid-v1"}
	for _, part := range []struct{ prefix, archive, version string }{{"", image.archivePath, handoffVersion}, {"before-", beforePath, "v9.8.0"}} {
		args = append(args, "--"+part.prefix+"source", part.archive, "--"+part.prefix+"version", part.version, "--"+part.prefix+"revision", image.revision, "--"+part.prefix+"target", runtime.GOOS+"/"+runtime.GOARCH,
			"--"+part.prefix+"attestation", filepath.Join(f.environment["HANDOFF_ASSETS"], "runtime-attestations.jsonl"), "--"+part.prefix+"trusted-root", roots)
	}
	for _, value := range installationInputs {
		args = append(args, "--project-input", value)
	}
	return args
}

// Each independently scheduled ordered group owns its images and templates.
// The helper runs only in BeforeAll, so discovery performs no preparation.
// per docs/adr/0102-parallel-go-qualification.md:58
type handoffScenarioFixtures struct {
	image                       handoffReleaseImage
	beforeBinary, beforeArchive []byte
	prepared                    map[string]installationFixtureData
}

func handoffPrepareScenarioFixtures() handoffScenarioFixtures {
	GinkgoHelper()
	fixtures := handoffScenarioFixtures{image: handoffBuildImage()}
	beforePath := filepath.Join(fixtures.image.work, "compiled-predecessor")
	build := []string{"build", "-ldflags=-buildid=handoff-before", "-o", beforePath, "./cmd/factory"}
	if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
		build = append([]string{"build", "-race"}, build[1:]...)
	}
	Expect(imageProcess(filepath.Join(fixtures.image.work, "exact-source"), nil, "go", build...).status).To(Equal(0))
	var err error
	fixtures.beforeBinary, err = os.ReadFile(beforePath)
	Expect(err).NotTo(HaveOccurred())
	Expect(fixtures.beforeBinary).NotTo(Equal(fixtures.image.binary), "independently built predecessor must have distinct bytes")
	fixtures.beforeArchive = handoffImageBinary(fixtures.image, fixtures.beforeBinary, "v9.8.0")
	fixtures.prepared = map[string]installationFixtureData{}
	for _, profile := range []string{"v0.1.6", "bash-baseline"} {
		fixtures.prepared[profile] = installationLegacyFixture(fixtures.image.binary, fixtures.image.references[profile], fixtures.image.archivePath, fixtures.image.revision)
	}
	return fixtures
}

func (fixtures handoffScenarioFixtures) legacyFixture(profile string) installationFixtureData {
	GinkgoHelper()
	return installationCloneLegacyFixture(fixtures.image.binary, fixtures.prepared[profile])
}

func (fixtures handoffScenarioFixtures) goFixture() (handoffBootstrapFixture, string, installationFixtureData) {
	GinkgoHelper()
	f, driver := handoffNativeFixture(fixtures.image)
	legacy := fixtures.legacyFixture("bash-baseline")
	legacy.archive = filepath.Join(filepath.Dir(legacy.root), "before-go-image.tar.gz")
	writeFixture(legacy.archive, fixtures.beforeArchive, 0600)
	installationApply(legacy, "qualified-prior-go")
	assets := f.environment["HANDOFF_ASSETS"]
	writeFixture(filepath.Join(assets, "factory-installation-v9.8.0-"+f.target+".tar.gz"), fixtures.beforeArchive, 0600)
	writeFixture(filepath.Join(assets, "factory-bootstrap-v9.8.0-"+f.target), fixtures.beforeBinary, 0600)
	return f, driver, legacy
}

// per docs/adr/0107-published-release-qualification.md:207
var _ = Describe("Whole installation upgrade release candidate handoff", Ordered, SpecPriority(2), ContinueOnFailure, Label("handoff-legacy-and-init"), func() {
	var fixtures handoffScenarioFixtures
	BeforeAll(func() {
		fixtures = handoffPrepareScenarioFixtures()
	})

	// per docs/adr/0101-go-release-handoff.md:148
	It("initializes a real selected Go installation from the authenticated complete image", func() {
		f, driver := handoffNativeFixture(fixtures.image)
		// per docs/DECISION_LOG.md:4873
		handoffNPMLifecycleFixture(f)
		target := filepath.Join(f.cwd, "new governed project")
		result := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", target, "--go-candidate", handoffVersion, "--pack", "none")
		Expect(result.status).To(Equal(0), "%+v", result)
		handoffNPMLifecyclePreserved(f, target)
		data, err := os.ReadFile(filepath.Join(target, ".factory/installation.current"))
		Expect(err).NotTo(HaveOccurred())
		var descriptor map[string]any
		Expect(json.Unmarshal(data, &descriptor)).To(Succeed())
		Expect(descriptor["kind"]).To(Equal("go-hybrid-v1"))
		Expect(descriptor["version"]).To(Equal(handoffVersion))
		Expect(descriptor["revision"]).To(Equal(fixtures.image.revision))
		_, err = os.Lstat(filepath.Join(target, ".git"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "candidate init must preserve the existing no-git-init behavior")
		runtimeBytes, err := os.ReadFile(filepath.Join(target, ".factory/bin/factory-runtime"))
		Expect(err).NotTo(HaveOccurred())
		Expect(runtimeBytes).To(Equal(fixtures.image.binary))
		ordinary := handoffNativeProcess(target, f.environment, "", filepath.Join(target, "factory"), "selftest")
		Expect(ordinary.status).To(Equal(0), "%+v", ordinary)
		// per docs/adr/0101-go-release-handoff.md:490
		beforePreview := installationSnapshot(target)
		next := handoffPreview(f, driver, target)
		Expect(next["profile"]).To(Equal("go-hybrid-v1"))
		Expect(next["blockers"]).To(BeEmpty())
		Expect(installationSnapshot(target)).To(Equal(beforePreview), "ordinary no-Git preview remains read-only")
		// per docs/adr/0101-go-release-handoff.md:154
		direct := handoffNativeProcess(target, f.environment, "", filepath.Join(target, "scripts/factory-budget.sh"), "--help")
		Expect(direct.status).To(Equal(0), "%+v", direct)
		Expect(direct.stdout).To(ContainSubstring("Available Commands:"))
		bridge := handoffNativeProcess(target, f.environment, "", "bash", "-c", `. "$1"; factory_config_get project_name`, "candidate-config", filepath.Join(target, "scripts/lib/config.sh"))
		Expect(bridge.status).To(Equal(0), "%+v", bridge)
		Expect(bridge.stdout).To(Equal("native-project"))
		for _, retired := range []string{"budget.py", "budget_adapters.py", "loop.py", "runtime_transition.py"} {
			_, err = os.Lstat(filepath.Join(target, "scripts/lib", retired))
			Expect(os.IsNotExist(err)).To(BeTrue(), retired)
		}
		// per docs/adr/0101-go-release-handoff.md:435
		other := filepath.Join(f.cwd, "another project")
		refusal := handoffNativeProcess(target, f.environment, nativeInitAnswers("y"), filepath.Join(target, "factory"), "init", other, "--pack", "none")
		Expect(refusal.status).NotTo(Equal(0), "%+v", refusal)
		Expect(refusal.stdout).NotTo(ContainSubstring("Proceed?"))
		Expect(refusal.stderr).To(ContainSubstring("--go-candidate"))
		_, err = os.Lstat(other)
		Expect(os.IsNotExist(err)).To(BeTrue(), "installed init must not silently create a legacy Bash selection")
		help := handoffNativeProcess(target, f.environment, "", filepath.Join(target, "factory"), "init", "--help")
		Expect(help.status).To(Equal(0), "%+v", help)
		Expect(help.stdout).To(ContainSubstring("--go-candidate"))
		_, err = os.Lstat(filepath.Join(f.records, "forbidden-tool"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "no compiler, Python, archive shell fallback or paid CLI belongs in the handoff")
	})

	// per docs/adr/0101-go-release-handoff.md:80
	// per docs/adr/0101-go-release-handoff.md:154
	DescribeTable("crosses the authenticated executable handoff into a real Go installation", func(entry string) {
		f, _ := handoffNativeFixture(fixtures.image)
		target := filepath.Join(f.cwd, "handoff-project")
		var program string
		args := []string{"init", target, "--go-candidate", handoffVersion, "--pack", "none"}
		if entry == "shell" {
			program, args = "/bin/sh", append([]string{f.installer}, args...)
		} else {
			Expect(candidateBinary).NotTo(Equal(fixtures.image.binary), "the caller must differ from the authenticated candidate so a real executable handoff is necessary")
			// The existing trusted caller need not live in an acquisition-style
			// private parent. Its bytes cannot authenticate the downloaded child.
			// per docs/adr/0101-go-release-handoff.md:119
			caller, err := os.CreateTemp("/tmp", "factory-public-caller-")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(os.Remove, caller.Name())
			_, writeErr := caller.Write(candidateBinary)
			Expect(errors.Join(writeErr, caller.Chmod(0700), caller.Close())).To(Succeed())
			program = caller.Name()
		}
		result := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), program, args...)
		Expect(result.status).To(Equal(0), "%+v", result)
		runtimeBytes, err := os.ReadFile(filepath.Join(target, ".factory/bin/factory-runtime"))
		Expect(err).NotTo(HaveOccurred())
		Expect(runtimeBytes).To(Equal(fixtures.image.binary))
		ordinary := handoffNativeProcess(target, f.environment, "", filepath.Join(target, "factory"), "selftest")
		Expect(ordinary.status).To(Equal(0), "%+v", ordinary)
		trace, err := os.ReadFile(filepath.Join(f.records, "trace"))
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Count(string(trace), "verify\n")).To(BeNumerically(">=", 2), "the downloaded child must independently reauthenticate the imported image")
		_, err = os.Lstat(filepath.Join(f.records, "forbidden-tool"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("trusted shell to authenticated raw binary", "shell"), Entry("different compiled caller to authenticated raw binary", "native"))

	// per docs/adr/0101-go-release-handoff.md:262
	It("keeps a failed fresh native proof incomplete behind pending admission", func() {
		f, driver := handoffNativeFixture(fixtures.image)
		broken := handoffRewriteAsset(fixtures.image.archive, "scripts/hooks/test-edit-denial.sh", func([]byte) []byte { return []byte("#!/bin/sh\nexit 0\n") })
		writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-installation-"+handoffVersion+"-"+f.target+".tar.gz"), broken, 0600)
		target := filepath.Join(f.cwd, "failed-proof-project")
		result := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", target, "--go-candidate", handoffVersion, "--pack", "none")
		Expect(result.status).NotTo(Equal(0), "%+v", result)
		_, err := os.Lstat(filepath.Join(target, ".factory-version"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "a failed proof must never advertise successful activation")
		_, err = os.Lstat(filepath.Join(target, ".factory/runtime-publication.pending"))
		Expect(err).NotTo(HaveOccurred())
		ordinary := handoffNativeProcess(target, f.environment, "", filepath.Join(target, "factory"), "selftest")
		Expect(ordinary.status).NotTo(Equal(0), "%+v", ordinary)
		Expect(ordinary.stdout).To(BeEmpty())
		Expect(result.stderr).To(MatchRegexp(`proof.*test-edit-denial`))
		// per docs/adr/0101-go-release-handoff.md:269
		Expect(result.stderr).To(ContainSubstring("incomplete"))
		Expect(result.stderr).To(ContainSubstring(target))
		Expect(result.stderr).To(MatchRegexp(`(?i)(review|reconcil|preserv|restor|recov|inspect)`), "incomplete initialization needs an explicit operator next action")
	})

	// per docs/adr/0101-go-release-handoff.md:353
	It("refuses corrupt existing Git metadata before publishing private initialization assets", func() {
		f, driver := handoffNativeFixture(fixtures.image)
		target := filepath.Join(f.cwd, "invalid-git-project")
		writeFixture(filepath.Join(target, ".git"), []byte("not a valid Git directory indirection\n"), 0600)
		result := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", target, "--go-candidate", handoffVersion, "--pack", "none")
		Expect(result.status).NotTo(Equal(0), "%+v", result)
		for _, path := range []string{".factory-version", ".factory/bin/factory-runtime", ".factory/installation.current"} {
			_, err := os.Lstat(filepath.Join(target, path))
			Expect(os.IsNotExist(err)).To(BeTrue(), path)
		}
		metadata, err := os.ReadFile(filepath.Join(target, ".git"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(metadata)).To(Equal("not a valid Git directory indirection\n"))
	})

	// per docs/adr/0101-go-release-handoff.md:163
	DescribeTable("previews a real legacy upgrade without low-level image or profile operands or project writes", func(profile string) {
		f, driver := handoffNativeFixture(fixtures.image)
		legacy := fixtures.legacyFixture(profile)
		// per docs/adr/0101-go-release-handoff.md:467
		ref := profile
		if ref == "bash-baseline" {
			ref = "main"
		}
		fullMarker := []byte("ref=" + ref + "\ncommit=" + fixtures.image.references[profile].revision + "\n")
		writeFixture(filepath.Join(legacy.root, ".factory-version"), fullMarker, 0644)
		caller := legacy.root
		if profile == "v0.1.6" {
			caller = filepath.Join(legacy.root, "application", "nested")
			Expect(os.MkdirAll(caller, 0700)).To(Succeed())
		}
		before := installationSnapshot(legacy.root)
		result := handoffNativeProcess(caller, f.environment, "", driver, "upgrade", "--go-candidate", handoffVersion, "--dry-run", "--json")
		Expect(result.status).To(Equal(0), "%+v", result)
		var proposal map[string]any
		Expect(json.Unmarshal([]byte(result.stdout), &proposal)).To(Succeed())
		Expect(proposal["profile"]).To(Equal(profile))
		Expect(proposal["blockers"]).To(BeEmpty())
		Expect(proposal["proposal_digest"]).To(MatchRegexp("^[a-f0-9]{64}$"))
		Expect(proposal["actions"]).NotTo(BeEmpty())
		Expect(installationSnapshot(legacy.root)).To(Equal(before))
		_, err := os.Lstat(filepath.Join(f.records, "forbidden-tool"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		// per docs/adr/0101-go-release-handoff.md:471
		for _, changed := range []struct {
			body []byte
			mode os.FileMode
		}{
			{[]byte("ref=" + ref + "\ncommit=0000000\n"), 0644},
			{[]byte("ref=" + ref + "\ncommit=" + fixtures.image.references[profile].revision[:8] + "\n"), 0644},
			{[]byte("ref=another-release\ncommit=" + fixtures.image.references[profile].revision[:7] + "\n"), 0644},
			{fullMarker, 0755},
		} {
			writeFixture(filepath.Join(legacy.root, ".factory-version"), changed.body, changed.mode)
			Expect(os.Chmod(filepath.Join(legacy.root, ".factory-version"), changed.mode)).To(Succeed())
			observed := installationSnapshot(legacy.root)
			denied := handoffNativeProcess(caller, f.environment, "", driver, "upgrade", "--go-candidate", handoffVersion, "--dry-run", "--json")
			if denied.status == 0 {
				Expect(handoffProposal(denied)["blockers"]).NotTo(BeEmpty(), "changed historical marker must never gain baseline ownership")
			} else {
				Expect(denied.status).To(Equal(2), "%+v", denied)
			}
			Expect(installationSnapshot(legacy.root)).To(Equal(observed))
		}
	}, Entry("v0.1.6", "v0.1.6"), Entry("Bash baseline", "bash-baseline"))

	// per docs/adr/0101-go-release-handoff.md:246
	It("authenticates imported release bytes then declines fresh init without publishing an installed selection", func() {
		f, driver := handoffNativeFixture(fixtures.image)
		cache := handoffImport(fixtures.image, f)
		before := installationSnapshot(cache)
		target := filepath.Join(f.cwd, "declined-project")
		result := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("n"), driver, "init", target, "--go-candidate", handoffVersion, "--pack", "none")
		Expect(result.status).NotTo(Equal(0))
		Expect(result.stdout).To(ContainSubstring("Aborted"))
		Expect(installationSnapshot(cache)).To(Equal(before))
		_, err := os.Lstat(filepath.Join(f.records, "release-download"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "the imported transport hint must not trigger an independent duplicate download")
		trace, err := os.ReadFile(filepath.Join(f.records, "trace"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(trace)).To(ContainSubstring("trusted-root\n"))
		Expect(string(trace)).To(ContainSubstring("verify\n"))
		for _, name := range []string{".factory-version", ".factory/installation.current"} {
			_, err := os.Lstat(filepath.Join(target, name))
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	})

	// per docs/adr/0101-go-release-handoff.md:163
	// per docs/adr/0101-go-release-handoff.md:318
	// per docs/adr/0101-go-release-handoff.md:467
	// per docs/adr/0101-go-release-handoff.md:475
	DescribeTable("applies a confirmed historical short-marker legacy proposal and restores its original marker", func(profile string) {
		f, driver := handoffNativeFixture(fixtures.image)
		legacy := fixtures.legacyFixture(profile)
		repository, err := filepath.Abs("..")
		Expect(err).NotTo(HaveOccurred())
		reference := fixtures.image.references[profile]
		// Run the frozen upgrade writer itself against its actual pinned Git
		// object, rather than inventing the legacy marker in this evaluator.
		for _, args := range [][]string{{"init", "--quiet", legacy.source}, {"-C", legacy.source, "fetch", "--quiet", "--no-tags", repository, reference.revision}, {"-C", legacy.source, "update-ref", "HEAD", reference.revision}} {
			git := imageProcess(repository, nil, "git", args...)
			Expect(git.status).To(Equal(0), "%+v", git)
		}
		ref := "main"
		legacyEnvironment := map[string]string{"PATH": filepath.Join(filepath.Dir(legacy.source), "external tools") + string(os.PathListSeparator) + os.Getenv("PATH")}
		upgraded := imageProcess(legacy.root, legacyEnvironment, "bash", filepath.Join(legacy.root, "scripts/factory-upgrade.sh"), "--source", legacy.source, "--ref", ref)
		Expect(upgraded.status).To(Equal(0), "%+v", upgraded)
		originalMarker, err := os.ReadFile(filepath.Join(legacy.root, ".factory-version"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(originalMarker)).To(Equal("ref=" + ref + "\ncommit=" + reference.revision[:7] + "\n"))
		readme := []byte("private application documentation\n")
		writeFixture(filepath.Join(legacy.root, "README.md"), readme, 0600)
		proposal := handoffPreview(f, driver, legacy.root)
		first := handoffFlag(handoffRecorded(f, "release-download"), "--dir")
		Expect(first).To(HavePrefix(f.root + string(os.PathSeparator)))
		Expect(first).NotTo(Equal(f.root))
		Expect(os.RemoveAll(first)).To(Succeed())
		result := handoffApply(f, driver, legacy.root, proposal)
		Expect(result.status).To(Equal(0), "%+v", result)
		second := handoffFlag(handoffRecorded(f, "release-download"), "--dir")
		Expect(second).NotTo(Equal(first))
		body, err := os.ReadFile(filepath.Join(legacy.root, "README.md"))
		Expect(err).NotTo(HaveOccurred())
		Expect(body).To(Equal(readme))
		version, err := os.ReadFile(filepath.Join(legacy.root, ".factory-version"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(version)).To(ContainSubstring(handoffVersion))
		backups, err := os.ReadDir(filepath.Join(legacy.root, ".factory/backups"))
		Expect(err).NotTo(HaveOccurred())
		Expect(backups).NotTo(BeEmpty())
		Expect(imageProcess(legacy.root, nil, "git", "check-ignore", "--no-index", ".factory/backups/qualification-probe").status).To(Equal(0))
		ordinary := handoffNativeProcess(legacy.root, f.environment, "", filepath.Join(legacy.root, "factory"), "selftest")
		Expect(ordinary.status).To(Equal(0), "%+v", ordinary)
		_, err = os.Lstat(filepath.Join(f.records, "forbidden-tool"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		migrations, err := filepath.Glob(filepath.Join(legacy.root, ".factory/installation-transactions/candidate-*.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(migrations).To(HaveLen(1))
		migration := strings.TrimSuffix(filepath.Base(migrations[0]), ".json")
		roots := filepath.Join(f.root, "operator-trusted-root.jsonl")
		writeFixture(roots, []byte("independently obtained fixture roots\n"), 0600)
		rollbackArgs := []string{"upgrade", "--installation", "--source", fixtures.image.archivePath, "--profile", profile, "--version", handoffVersion, "--revision", fixtures.image.revision, "--target", runtime.GOOS + "/" + runtime.GOARCH, "--attestation", filepath.Join(f.environment["HANDOFF_ASSETS"], "runtime-attestations.jsonl"), "--trusted-root", roots, "--rollback", migration}
		for _, value := range installationInputs {
			rollbackArgs = append(rollbackArgs, "--project-input", value)
		}
		// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:297
		// Restoring the old Bash runtime deliberately requires its baseline tools;
		// the preceding Go apply and ordinary selftest retained poisoned Python.
		recoveryEnvironment := map[string]string{}
		for key, value := range f.environment {
			recoveryEnvironment[key] = value
		}
		recoveryTools := filepath.Join(f.root, "legacy recovery tools")
		verifier, err := os.ReadFile(filepath.Join(f.root, "host tools", "gh"))
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(recoveryTools, "gh"), verifier, 0700)
		recoveryEnvironment["PATH"] = recoveryTools + string(os.PathListSeparator) + legacyEnvironment["PATH"]
		reverse := handoffProposal(handoffNativeProcess(legacy.root, recoveryEnvironment, "", driver, append(rollbackArgs, "--dry-run", "--json")...))
		rollback := handoffNativeProcess(legacy.root, recoveryEnvironment, "", driver, append(rollbackArgs, "--confirm", reverse["proposal_digest"].(string), "--quiescent")...)
		Expect(rollback.status).To(Equal(0), "%+v", rollback)
		marker, err := os.ReadFile(filepath.Join(legacy.root, ".factory-version"))
		Expect(err).NotTo(HaveOccurred())
		Expect(marker).To(Equal(originalMarker), "rollback must restore exact checked short-marker bytes")
		restored := imageProcess(legacy.root, legacyEnvironment, "bash", filepath.Join(legacy.root, "scripts/selftest/run.sh"))
		Expect(restored.status).To(Equal(0), "%+v", restored)
	}, Entry("v0.1.6", "v0.1.6"), Entry("Bash baseline", "bash-baseline"))

	// per docs/adr/0101-go-release-handoff.md:163
	It("preserves a customized required launcher as an explicit conflict", func() {
		f, driver := handoffNativeFixture(fixtures.image)
		legacy := fixtures.legacyFixture("v0.1.6")
		custom := []byte("#!/bin/sh\nprintf customized-factory\n")
		writeFixture(filepath.Join(legacy.root, "factory"), custom, 0755)
		before := installationSnapshot(legacy.root)
		result := handoffNativeProcess(legacy.root, f.environment, "", driver, "upgrade", "--go-candidate", handoffVersion, "--dry-run", "--json")
		Expect(result.status).To(BeElementOf(0, 2), "%+v", result)
		var proposal map[string]any
		if result.stdout != "" {
			Expect(json.Unmarshal([]byte(result.stdout), &proposal)).To(Succeed())
			Expect(proposal["blockers"]).NotTo(BeEmpty())
		} else {
			Expect(result.status).To(Equal(2))
			Expect(result.stderr).To(MatchRegexp(`(?i)(profile|predecessor)`))
		}
		Expect(installationSnapshot(legacy.root)).To(Equal(before))
		// per docs/adr/0101-go-release-handoff.md:304
		// per docs/adr/0101-go-release-handoff.md:500
		unknown := filepath.Join(f.cwd, "marker-only-project")
		configuration, err := os.ReadFile(filepath.Join(legacy.root, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(unknown, "factory.yaml"), configuration, 0644)
		writeFixture(filepath.Join(unknown, ".factory-version"), []byte("ref=v0.1.6\ncommit="+fixtures.image.references["v0.1.6"].revision+"\n"), 0644)
		git := imageProcess(unknown, nil, "git", "init", "--quiet")
		Expect(git.status).To(Equal(0), "%+v", git)
		unknownBefore := installationSnapshot(unknown)
		denied := handoffNativeProcess(unknown, f.environment, "", driver, "upgrade", "--go-candidate", handoffVersion, "--dry-run", "--json")
		Expect(denied.status).To(Equal(2), "%+v", denied)
		Expect(denied.stdout).To(BeEmpty(), "a canonical marker without any framework rows cannot identify an installation profile")
		Expect(denied.stderr).To(MatchRegexp(`(?i)(profile|predecessor)`))
		Expect(installationSnapshot(unknown)).To(Equal(unknownBefore))
	})

	// per docs/adr/0101-go-release-handoff.md:579
	// per docs/adr/0101-go-release-handoff.md:588
	It("preserves policy-refusal status while retaining prepared recovery inputs", func() {
		f, driver := handoffNativeFixture(fixtures.image)
		legacy := fixtures.legacyFixture("v0.1.6")
		launcher, err := os.ReadFile(filepath.Join(legacy.root, "factory"))
		Expect(err).NotTo(HaveOccurred())
		ignore, err := os.ReadFile(filepath.Join(legacy.root, ".gitignore"))
		Expect(err).NotTo(HaveOccurred())
		ignore = append(ignore, []byte("\n!/.factory/\n/.factory/bin/\n/.factory/assets/\n/.factory/installation.current\n/.factory/installation-transactions/\n/.factory/backups/\n!/.factory/bin/\n")...)
		writeFixture(filepath.Join(legacy.root, ".gitignore"), ignore, 0644)
		actualIgnore := imageProcess(legacy.root, nil, "git", "check-ignore", "--no-index", "-q", "--", ".factory/bin/factory-runtime")
		Expect(actualIgnore.status).To(Equal(1), "the final negation must actually expose the private runtime path: %+v", actualIgnore)
		before := installationSnapshot(legacy.root)
		proposal := handoffPreview(f, driver, legacy.root)
		Expect(proposal["blockers"]).To(BeEmpty())
		Expect(installationSnapshot(legacy.root)).To(Equal(before))
		result := handoffApply(f, driver, legacy.root, proposal)
		Expect(result.stderr).To(ContainSubstring("not effectively ignored"))
		journals, err := filepath.Glob(filepath.Join(legacy.root, ".factory/installation-transactions/candidate-*.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(journals).To(HaveLen(1))
		body, err := os.ReadFile(journals[0])
		Expect(err).NotTo(HaveOccurred())
		var journal map[string]any
		Expect(json.Unmarshal(body, &journal)).To(Succeed())
		Expect(journal["phase"]).To(Equal("prepared"))
		migration := strings.TrimSuffix(filepath.Base(journals[0]), ".json")
		cache := handoffFlag(handoffRecorded(f, "release-download"), "--dir")
		Expect(cache).To(HavePrefix(f.root + string(os.PathSeparator)))
		root, err := os.Lstat(cache)
		Expect(err).NotTo(HaveOccurred())
		Expect(root.IsDir()).To(BeTrue())
		Expect(root.Mode().Perm()).To(Equal(os.FileMode(0700)))
		for _, name := range []string{"factory-installation-" + handoffVersion + "-" + f.target + ".tar.gz", "runtime-attestations.jsonl", "trusted-root.jsonl"} {
			path := filepath.Join(cache, "inputs", name)
			info, err := os.Lstat(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().IsRegular()).To(BeTrue())
			Expect(info.Size()).To(BeNumerically(">", 0))
			Expect(result.stderr).To(ContainSubstring(path), "retained inputs require a concrete recovery operand")
		}
		recoveryRuntime := filepath.Join(cache, "stage", handoffVersion, runtime.GOOS, runtime.GOARCH, "bin/factory-runtime")
		recoveryBinary, err := os.ReadFile(recoveryRuntime)
		Expect(err).NotTo(HaveOccurred())
		Expect(recoveryBinary).To(Equal(fixtures.image.binary))
		Expect(result.stderr).To(ContainSubstring("Recovery preview (no changes):"))
		Expect(result.stderr).To(ContainSubstring(migration))
		Expect(result.stderr).To(ContainSubstring(recoveryRuntime))
		unchanged, err := os.ReadFile(filepath.Join(legacy.root, "factory"))
		Expect(err).NotTo(HaveOccurred())
		Expect(unchanged).To(Equal(launcher), "a pre-publication refusal must preserve the legacy dispatcher")
		_, err = os.Lstat(filepath.Join(legacy.root, ".factory/installation.current"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(result.stdout).To(BeEmpty())
		Expect(result.status).To(Equal(2), "%+v", result)
	})

	// per docs/adr/0101-go-release-handoff.md:318
	It("refuses confirmation after a project observation changes", func() {
		f, driver := handoffNativeFixture(fixtures.image)
		legacy := fixtures.legacyFixture("v0.1.6")
		proposal := handoffPreview(f, driver, legacy.root)
		writeFixture(filepath.Join(legacy.root, "scripts/hooks/test-edit-denial.sh"), []byte("#!/bin/sh\n# deliberate later user edit\nexit 0\n"), 0755)
		before := installationSnapshot(legacy.root)
		result := handoffApply(f, driver, legacy.root, proposal)
		Expect(result.status).NotTo(Equal(0))
		Expect(installationSnapshot(legacy.root)).To(Equal(before))
	})

	// per docs/adr/0101-go-release-handoff.md:318
	It("refuses confirmation after the immutable release identity changes", func() {
		f, driver := handoffNativeFixture(fixtures.image)
		legacy := fixtures.legacyFixture("v0.1.6")
		proposal := handoffPreview(f, driver, legacy.root)
		f.environment["HANDOFF_REVISION"] = strings.Repeat("9", 40)
		before := installationSnapshot(legacy.root)
		result := handoffApply(f, driver, legacy.root, proposal)
		Expect(result.status).NotTo(Equal(0))
		Expect(installationSnapshot(legacy.root)).To(Equal(before))
	})

	// per docs/adr/0101-go-release-handoff.md:337
	It("reauthenticates a confirmed release before any project mutation", func() {
		f, driver := handoffNativeFixture(fixtures.image)
		legacy := fixtures.legacyFixture("v0.1.6")
		proposal := handoffPreview(f, driver, legacy.root)
		f.environment["HANDOFF_SCENARIO"] = "authentication-fail"
		before := installationSnapshot(legacy.root)
		result := handoffApply(f, driver, legacy.root, proposal)
		Expect(result.status).NotTo(Equal(0), "%+v", result)
		Expect(result.stderr).To(ContainSubstring("authentication"))
		Expect(installationSnapshot(legacy.root)).To(Equal(before))
	})

	// per docs/adr/0101-go-release-handoff.md:119
	// per docs/adr/0101-go-release-handoff.md:133
	DescribeTable("treats imported candidate files as untrusted transport rather than installation authority", func(change string) {
		f, driver := handoffNativeFixture(fixtures.image)
		cache := handoffImport(fixtures.image, f)
		rawName := "factory-bootstrap-" + handoffVersion + "-" + f.target
		archiveName := "factory-installation-" + handoffVersion + "-" + f.target + ".tar.gz"
		switch change {
		case "revision":
			writeFixture(filepath.Join(cache, "revision"), []byte(strings.Repeat("8", 40)+"\n"), 0600)
		case "symlink":
			Expect(os.Remove(filepath.Join(cache, archiveName))).To(Succeed())
			Expect(os.Symlink(fixtures.image.archivePath, filepath.Join(cache, archiveName))).To(Succeed())
		case "raw":
			writeFixture(filepath.Join(cache, rawName), []byte("#!/bin/sh\nexit 0\n"), 0600)
		case "archive":
			writeFixture(filepath.Join(cache, archiveName), []byte("not an installation image"), 0600)
		}
		target := filepath.Join(f.cwd, "cache-target")
		before := installationSnapshot(cache)
		result := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", target, "--go-candidate", handoffVersion, "--pack", "none")
		Expect(result.status).NotTo(Equal(0), "%+v", result)
		_, err := os.Lstat(target)
		Expect(os.IsNotExist(err)).To(BeTrue(), "transport refusal must precede initializer publication")
		Expect(installationSnapshot(cache)).To(Equal(before), "the imported directory is not owned cleanup material")
	}, Entry("changed resolved revision", "revision"), Entry("linked image", "symlink"), Entry("different raw executable", "raw"), Entry("invalid image bytes", "archive"))

	// per docs/adr/0101-go-release-handoff.md:246
	DescribeTable("refuses fresh initialization over existing installation metadata", func(path string) {
		f, driver := handoffNativeFixture(fixtures.image)
		target := filepath.Join(f.cwd, "existing-project")
		writeFixture(filepath.Join(target, path), []byte("existing operator-owned metadata\n"), 0600)
		before := installationSnapshot(target)
		result := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", target, "--go-candidate", handoffVersion, "--pack", "none")
		Expect(result.status).NotTo(Equal(0), "%+v", result)
		Expect(result.stderr).To(ContainSubstring("upgrade"))
		Expect(installationSnapshot(target)).To(Equal(before))
	}, Entry("legacy version marker", ".factory-version"), Entry("private Go descriptor", ".factory/installation.current"))
})

var _ = Describe("Whole installation upgrade release candidate handoff", Ordered, ContinueOnFailure, Label("handoff-go-recovery"), func() {
	var fixtures handoffScenarioFixtures
	BeforeAll(func() {
		fixtures = handoffPrepareScenarioFixtures()
	})

	// per docs/adr/0101-go-release-handoff.md:171
	// per docs/adr/0101-go-release-handoff.md:179
	It("uses authenticated predecessor evidence for an installed Go-to-Go upgrade and completed rollback", func() {
		f, driver, legacy := fixtures.goFixture()
		installedDriver := filepath.Join(legacy.root, "factory")
		proposal := handoffPreview(f, installedDriver, legacy.root)
		Expect(proposal["profile"]).To(Equal("go-hybrid-v1"))
		result := handoffApply(f, installedDriver, legacy.root, proposal)
		Expect(result.status).To(Equal(0), "%+v", result)
		ordinary := handoffNativeProcess(legacy.root, f.environment, "", filepath.Join(legacy.root, "factory"), "selftest")
		Expect(ordinary.status).To(Equal(0), "%+v", ordinary)
		actual, err := os.ReadFile(filepath.Join(legacy.root, ".factory/bin/factory-runtime"))
		Expect(err).NotTo(HaveOccurred())
		Expect(actual).To(Equal(fixtures.image.binary))
		migrations, err := filepath.Glob(filepath.Join(legacy.root, ".factory/installation-transactions/candidate-*.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(migrations).To(HaveLen(1))
		migration := strings.TrimSuffix(filepath.Base(migrations[0]), ".json")
		rollbackArgs := append(handoffRecoveryOperands(fixtures.image, f, legacy.archive), "--rollback", migration)
		reverse := handoffProposal(handoffNativeProcess(legacy.root, f.environment, "", driver, append(rollbackArgs, "--dry-run", "--json")...))
		// per docs/adr/0101-go-release-handoff.md:333
		changedBefore := bytes.Clone(fixtures.beforeArchive)
		changedBefore[4] ^= 1 // valid gzip MTIME change, same decompressed qualified image
		writeFixture(legacy.archive, changedBefore, 0600)
		changedReverse := handoffProposal(handoffNativeProcess(legacy.root, f.environment, "", driver, append(rollbackArgs, "--dry-run", "--json")...))
		Expect(changedReverse["proposal_digest"]).NotTo(Equal(reverse["proposal_digest"]), "reverse confirmation must bind freshly observed predecessor archive bytes")
		writeFixture(legacy.archive, fixtures.beforeArchive, 0600)
		rollback := handoffNativeProcess(legacy.root, f.environment, "", driver, append(rollbackArgs, "--confirm", reverse["proposal_digest"].(string), "--quiescent")...)
		Expect(rollback.status).To(Equal(0), "%+v", rollback)
		actual, err = os.ReadFile(filepath.Join(legacy.root, ".factory/bin/factory-runtime"))
		Expect(err).NotTo(HaveOccurred())
		Expect(actual).To(Equal(fixtures.beforeBinary), "reverse must restore the distinct authenticated predecessor executable")
		ordinary = handoffNativeProcess(legacy.root, f.environment, "", filepath.Join(legacy.root, "factory"), "selftest")
		Expect(ordinary.status).To(Equal(0), "%+v", ordinary)
	})

	// per docs/adr/0101-go-release-handoff.md:285
	// per docs/adr/0101-go-release-handoff.md:318
	DescribeTable("requalifies predecessor recipe and archive evidence for each candidate request", func(change string) {
		f, driver, installed := fixtures.goFixture()
		proposal := handoffPreview(f, driver, installed.root)
		changed := bytes.Clone(fixtures.beforeArchive)
		if change == "recipe" {
			// per docs/adr/0101-go-release-handoff.md:485
			descriptorPath := filepath.Join(installed.root, ".factory/installation.current")
			original, err := os.ReadFile(descriptorPath)
			Expect(err).NotTo(HaveOccurred())
			info, err := os.Stat(descriptorPath)
			Expect(err).NotTo(HaveOccurred())
			moving := bytes.Replace(original, []byte(`"v9.8.0"`), []byte(`"main"`), 1)
			Expect(moving).NotTo(Equal(original))
			writeFixture(descriptorPath, moving, info.Mode().Perm())
			invalid := handoffNativeProcess(installed.root, f.environment, "", driver, "upgrade", "--go-candidate", handoffVersion, "--dry-run", "--json")
			Expect(invalid.status).NotTo(Equal(0), "%+v", invalid)
			Expect(strings.Join(handoffRecorded(f, "api"), " ")).NotTo(ContainSubstring("refs/tags/main"), "untrusted predecessor release must be refused before gh lookup")
			Expect(handoffRecorded(f, "release-view")).NotTo(ContainElement("main"))
			writeFixture(descriptorPath, original, info.Mode().Perm())
			changed = handoffRewriteAsset(fixtures.beforeArchive, "internal/installedlayout/layout.go", func(body []byte) []byte {
				return append(body, []byte("\n// a different predecessor schema collaborator\n")...)
			})
		} else {
			// Gzip MTIME does not change any decompressed control or source asset;
			// this is a valid new archive identity for the same literal release.
			changed[4] ^= 1
		}
		writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-installation-v9.8.0-"+f.target+".tar.gz"), changed, 0600)
		before := installationSnapshot(installed.root)
		if change == "recipe" {
			preview := handoffNativeProcess(installed.root, f.environment, "", driver, "upgrade", "--go-candidate", handoffVersion, "--dry-run", "--json")
			Expect(preview.status).NotTo(Equal(0), "%+v", preview)
			Expect(preview.stderr).To(ContainSubstring("recipe"))
		} else {
			fresh := handoffPreview(f, driver, installed.root)
			Expect(fresh["proposal_digest"]).NotTo(Equal(proposal["proposal_digest"]))
			stale := handoffApply(f, driver, installed.root, proposal)
			Expect(stale.status).NotTo(Equal(0), "%+v", stale)
		}
		Expect(installationSnapshot(installed.root)).To(Equal(before))
	}, Entry("different output-affecting recipe collaborator", "recipe"), Entry("changed valid predecessor archive after confirmation", "archive"))

	// per docs/adr/0101-go-release-handoff.md:179
	DescribeTable("recovers an actually interrupted Go-to-Go transaction with distinct before and after executables", func(direction string) {
		f, driver, installed := fixtures.goFixture()
		proposal := handoffPreview(f, driver, installed.root)
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		cmd := handoffCommand(ctx, installed.root, f.environment, "", driver, "upgrade", "--go-candidate", handoffVersion, "--confirm", proposal["proposal_digest"].(string), "--quiescent")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.WaitDelay = time.Second
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		Expect(cmd.Start()).To(Succeed())
		started := time.Now()
		finished := make(chan error, 1)
		go func() { finished <- cmd.Wait() }()
		waited := false
		lastPhase := "no readable candidate journal"
		DeferCleanup(func() {
			if !waited {
				_ = syscall.Kill(cmd.Process.Pid, syscall.SIGCONT)
				_ = syscall.Kill(cmd.Process.Pid, syscall.SIGTERM)
				select {
				case <-finished:
				case <-time.After(5 * time.Second):
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
					<-finished
				}
			}
			_, _ = fmt.Fprintf(GinkgoWriter, "candidate interruption cleanup after %s; last observation=%s; stdout=%q; stderr=%q\n", time.Since(started), lastPhase, stdout.String(), stderr.String())
		})
		migration := ""
		pending := filepath.Join(installed.root, ".factory/runtime-publication.pending")
		observe := func() bool {
			select {
			case err := <-finished:
				waited = true
				StopTrying(fmt.Sprintf("candidate exited before crash witness: error=%v; phase=%s; stdout=%q; stderr=%q", err, lastPhase, stdout.String(), stderr.String())).Now()
			default:
			}
			paths, err := filepath.Glob(filepath.Join(installed.root, ".factory/installation-transactions/candidate-*.json"))
			if err != nil || len(paths) != 1 {
				return false
			}
			body, err := os.ReadFile(paths[0])
			var journal map[string]any
			if err != nil || json.Unmarshal(body, &journal) != nil {
				return false
			}
			entries, ok := journal["entries"].([]any)
			if !ok {
				return false
			}
			applied := 0
			for _, item := range entries {
				if item.(map[string]any)["phase"] == "applied" {
					applied++
				}
			}
			_, err = os.Lstat(pending)
			lastCheck := "none"
			if checks, ok := journal["checks"].([]any); ok && len(checks) != 0 {
				check := checks[len(checks)-1].(map[string]any)
				lastCheck = fmt.Sprintf("%v/%v", check["name"], check["phase"])
			}
			lastPhase = fmt.Sprintf("phase=%v applied=%d pending=%v check=%s", journal["phase"], applied, err == nil, lastCheck)
			if journal["phase"] != "applying" {
				return false
			}
			if applied < 2 || err != nil {
				return false
			}
			migration = strings.TrimSuffix(filepath.Base(paths[0]), ".json")
			return true
		}
		// Native preflight proof has its own bounded preparation phase; the
		// observed race build was still in native-gate-proof after 30 seconds.
		Eventually(func() bool {
			if observe() {
				return true
			}
			return strings.Contains(lastPhase, "phase=applying ")
		}, 80*time.Second, time.Millisecond).Should(BeTrue(), "must finish bounded preparation before the publication witness: %s", lastPhase)
		_, _ = fmt.Fprintf(GinkgoWriter, "preparation reached publication after %s; %s\n", time.Since(started), lastPhase)
		Eventually(observe, 5*time.Second, time.Millisecond).Should(BeTrue(), "must witness actual published rows under pending before interruption")
		_, _ = fmt.Fprintf(GinkgoWriter, "actual crash witness after %s; %s\n", time.Since(started), lastPhase)
		Expect(syscall.Kill(cmd.Process.Pid, syscall.SIGSTOP)).To(Succeed())
		Eventually(func() bool {
			stopped, err := installationProcessStopped(cmd.Process.Pid)
			Expect(err).NotTo(HaveOccurred())
			return stopped
		}, 5*time.Second, time.Millisecond).Should(BeTrue(), "observe actual stop before accepting the Go-to-Go crash witness")
		Expect(observe()).To(BeTrue(), "the stopped child must retain applying rows and pending")
		journalPath := filepath.Join(installed.root, ".factory/installation-transactions", migration+".json")
		stoppedJournal, err := os.ReadFile(journalPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)).To(Succeed())
		interruptionError := <-finished
		waited = true
		var exit *exec.ExitError
		Expect(errors.As(interruptionError, &exit)).To(BeTrue())
		status, ok := exit.Sys().(syscall.WaitStatus)
		Expect(ok).To(BeTrue())
		Expect(status.Signal()).To(Equal(syscall.SIGKILL))
		Expect(os.ReadFile(journalPath)).To(Equal(stoppedJournal), "the killed child cannot advance the stopped journal")
		_, err = os.Lstat(pending)
		Expect(err).NotTo(HaveOccurred())
		recoveryArgs := append(handoffRecoveryOperands(fixtures.image, f, installed.archive), "--recover", migration, "--direction", direction)
		recovery := handoffProposal(handoffNativeProcess(installed.root, f.environment, "", driver, append(recoveryArgs, "--dry-run", "--json")...))
		result := handoffNativeProcess(installed.root, f.environment, "", driver, append(recoveryArgs, "--confirm", recovery["proposal_digest"].(string), "--quiescent")...)
		Expect(result.status).To(Equal(0), "%+v", result)
		actual, err := os.ReadFile(filepath.Join(installed.root, ".factory/bin/factory-runtime"))
		Expect(err).NotTo(HaveOccurred())
		expected := fixtures.image.binary
		if direction == "reverse" {
			expected = fixtures.beforeBinary
		}
		Expect(actual).To(Equal(expected), "recovery direction must select the exact qualified executable")
		_, err = os.Lstat(pending)
		Expect(os.IsNotExist(err)).To(BeTrue())
		ordinary := handoffNativeProcess(installed.root, f.environment, "", filepath.Join(installed.root, "factory"), "selftest")
		Expect(ordinary.status).To(Equal(0), "%+v", ordinary)
		// per docs/DECISION_LOG.md:4850
		digest := installationAssertCompletedReplay(installed.root, func() cliResult {
			return handoffNativeProcess(installed.root, f.environment, "", driver, append(append([]string{}, recoveryArgs...), "--dry-run", "--json")...)
		}, func(digest string) cliResult {
			return handoffNativeProcess(installed.root, f.environment, "", driver, append(append([]string{}, recoveryArgs...), "--confirm", digest, "--quiescent")...)
		})
		if direction == "forward" {
			// Preserve a valid JSON manifest whose profile no longer agrees with
			// the completed transaction, including on a fresh public handoff.
			// per docs/DECISION_LOG.md:4855
			path := filepath.Join(installed.root, ".factory/backups", migration, "installation.manifest")
			body, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			var manifest map[string]any
			Expect(json.Unmarshal(body, &manifest)).To(Succeed())
			manifest["profile"] = "v0.1.6"
			changed, err := json.Marshal(manifest)
			Expect(err).NotTo(HaveOccurred())
			writeFixture(path, changed, 0600)
			installationAssertReplayRefused(installed.root, func() cliResult {
				return handoffNativeProcess(installed.root, f.environment, "", driver, append(append([]string{}, recoveryArgs...), "--confirm", digest, "--quiescent")...)
			})
		}
	}, Entry("resume forward", "forward"), Entry("restore predecessor", "reverse"))
})
