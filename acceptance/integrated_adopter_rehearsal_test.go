package acceptance_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"
)

const integratedAdopterRepository = "fixture/integrated-adopter"

// These are ordinary public processes. The environment is exact so ambient
// provider credentials cannot enter bootstrap, checks or the mock evaluation.
// per docs/adr/0156-integrated-adopter-rehearsal.md:58
// per docs/adr/0156-integrated-adopter-rehearsal.md:68
func integratedAdopterRun(phase, root string, environment map[string]string, retainedRoots []string, input, program string, args ...string) cliResult {
	GinkgoHelper()
	started := time.Now()
	streams, err := os.MkdirTemp("", "factory-integrated-adopter-streams-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(cleanupFixtureDirectory, streams)
	retained := append([]string{streams}, retainedRoots...)
	out := cooperativeFixtureProcess(streams, "integrated-"+phase, input, retained, func(ctx context.Context) *exec.Cmd {
		command := handoffCommand(ctx, root, environment, input, program, args...)
		command.Env = make([]string, 0, len(environment))
		for key, value := range environment {
			command.Env = append(command.Env, key+"="+value)
		}
		return command
	})
	AddReportEntry("integrated-adopter."+phase, map[string]any{"command": append([]string{program}, args...), "status": out.status, "stdout": out.stdout, "stderr": out.stderr, "elapsed_seconds": time.Since(started).Seconds()}, ReportEntryVisibilityAlways)
	return out
}

// Reuse only the surface/image cache, without the planner or initializer witness
// patches used by default-dispatch ownership tests.
// per docs/adr/0156-integrated-adopter-rehearsal.md:15
func integratedAdopterFixtures(image handoffReleaseImage) *defaultUpgradeJourneyFixtures {
	GinkgoHelper()
	selected := defaultInitImage{binary: image.binary, archive: image.archive}
	selected.result.Archive = image.archivePath
	defaults := &defaultInitFixtures{invoker: image, revision: image.revision, images: map[string]defaultInitImage{defaultInitTag: selected}}
	f := &defaultUpgradeJourneyFixtures{defaultInitFixtures: defaults}
	f.legacy = installationLegacyFixture(image.binary, image.references["v0.1.6"], image.archivePath, image.revision)
	return f
}

// The adopter deliberately configures its own application check before Git
// publication. No generated Makefile, harness, gate or workflow is repaired.
// per docs/adr/0156-integrated-adopter-rehearsal.md:37
func integratedAdopterApplication(root string) {
	GinkgoHelper()
	writeFixture(filepath.Join(root, "application.txt"), []byte("application canary\n"), 0644)
	writeFixture(filepath.Join(root, "application-check.sh"), []byte("#!/bin/sh\nset -eu\ntest -z \"${GH_TOKEN-}\"\ntest \"$(cat application.txt)\" = 'application canary'\nprintf 'application check\\n' >> \"$REHEARSAL_CHECK_LOG\"\nexit \"${REHEARSAL_APPLICATION_STATUS:-0}\"\n"), 0755)
	ciTransactionSetting(root, "check_command", "./application-check.sh")
}

// Validate each immutable download and deliver its real Git bytes. All release
// authentication still runs through the original recording gh transport.
// per docs/adr/0156-integrated-adopter-rehearsal.md:61
func integratedAdopterTransport(f handoffBootstrapFixture, root string, image handoffReleaseImage) {
	GinkgoHelper()
	f.environment["RUNNER_TEMP"] = filepath.Join(f.root, "integrated runner temporary")
	Expect(os.Mkdir(f.environment["RUNNER_TEMP"], 0700)).To(Succeed())
	f.environment["REHEARSAL_CHECK_LOG"] = filepath.Join(f.records, "application-checks")
	f.environment["REHEARSAL_APPLICATION_STATUS"] = "0"
	f.environment["GH_TOKEN"] = ""
	f.environment["GITHUB_OUTPUT"] = filepath.Join(f.records, "step-output")
	f.environment["REVIEW_GO_CLIENT"] = ""
	f.environment["PR"], f.environment["REPO"] = "7", integratedAdopterRepository
	f.environment["DEFAULT_BRANCH"] = "main"
	f.environment["EXPECTED_BASE"] = strings.TrimSpace(imageGit(root, "rev-parse", "HEAD"))
	f.environment["EXPECTED_HEAD"] = ciReviewHead
	f.environment["MAX_DIFF_BYTES"] = "200000"
	delete(f.environment, "REVIEW_API_KEY")
	ciInstaller := []byte(imageGit(filepath.Join(image.work, "exact-source"), "show", image.revision+":install.sh"))
	reviewInstaller := []byte(imageGit("..", "show", installedReviewInstallerRevision+":install.sh"))
	writeFixture(filepath.Join(f.root, "ordinary-installer"), ciInstaller, 0600)
	writeFixture(filepath.Join(f.root, "review-installer"), reviewInstaller, 0600)
	f.environment["REHEARSAL_CI_URL"] = "https://raw.githubusercontent.com/anoop2811/software-factory-template/" + image.revision + "/install.sh"
	f.environment["REHEARSAL_REVIEW_URL"] = "https://raw.githubusercontent.com/anoop2811/software-factory-template/" + installedReviewInstallerRevision + "/install.sh"
	f.environment["REHEARSAL_ROOT"] = f.root
	jq, err := exec.LookPath("jq")
	Expect(err).NotTo(HaveOccurred())
	jq, err = filepath.EvalSymlinks(jq)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.Symlink(jq, filepath.Join(f.root, "host tools/jq"))).To(Succeed())
	writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+defaultInitTag+"-"+f.target), image.binary, 0600)
	writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-installation-"+defaultInitTag+"-"+f.target+".tar.gz"), image.archive, 0600)
	writeFixture(filepath.Join(f.root, "host tools/curl"), []byte(`#!/bin/sh
set -eu
destination= url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o|--output) shift; destination=$1 ;;
    --proto|--proto-redir) shift; [ "$1" = '=https' ] || exit 91 ;;
    --connect-timeout|--max-time|--max-filesize) shift ;;
    --tlsv1.2|--fail|--silent|--show-error|--location) ;;
    https://*) [ -z "$url" ] || exit 92; url=$1 ;;
    *) exit 93 ;;
  esac
  shift
done
case "$destination" in "$RUNNER_TEMP"/*) ;; *) exit 94 ;; esac
[ ! -L "$destination" ] || exit 95
if [ -e "$destination" ]; then [ -f "$destination" ] || exit 95; fi
parent=${destination%/*}
[ -d "$parent" ] && [ ! -L "$parent" ] || exit 95
[ -z "${REVIEW_API_KEY+x}" ] || { printf present > "$HANDOFF_RECORD_DIR/acquisition-provider-key"; exit 96; }
case "$url" in
  "$REHEARSAL_CI_URL") source="$REHEARSAL_ROOT/ordinary-installer" ;;
  "$REHEARSAL_REVIEW_URL") source="$REHEARSAL_ROOT/review-installer" ;;
  *) exit 97 ;;
esac
printf '%s\n' "$url" >> "$HANDOFF_RECORD_DIR/installer-urls"
cat "$source" > "$destination"
`), 0700)
	gh := filepath.Join(f.root, "host tools/gh")
	Expect(os.Rename(gh, filepath.Join(f.root, "release-gh"))).To(Succeed())
	writeFixture(gh, []byte(`#!/bin/sh
set -eu
if [ "$1" = api ] && [ "$2" = "repos/$REPO/pulls/$PR" ]; then
  case "$*" in *application/vnd.github.v3.diff*) cat "$REHEARSAL_ROOT/review.diff" ;; *) cat "$REHEARSAL_ROOT/pr.json" ;; esac
  exit 0
fi
if [ "$1 ${2-}" = 'pr comment' ]; then
  while [ "$1" != --body-file ]; do shift; done
  cat "$2" > "$HANDOFF_RECORD_DIR/review-comment"
  exit 0
fi
[ -z "${REVIEW_API_KEY+x}" ] || { printf present > "$HANDOFF_RECORD_DIR/acquisition-provider-key"; exit 96; }
exec /bin/sh "$REHEARSAL_ROOT/release-gh" "$@"
`), 0700)
	metadata := map[string]any{"head": map[string]any{"sha": ciReviewHead, "repo": map[string]string{"full_name": integratedAdopterRepository}}, "base": map[string]any{"sha": f.environment["EXPECTED_BASE"], "ref": "main", "repo": map[string]string{"full_name": integratedAdopterRepository}}}
	writeFixture(filepath.Join(f.root, "pr.json"), imageMarshal(metadata), 0600)
	writeFixture(filepath.Join(f.root, "review.diff"), []byte("diff --git a/application.txt b/application.txt\n--- a/application.txt\n+++ b/application.txt\n@@ -1 +1 @@\n-before\n+application canary\n"), 0600)
}

// Execute a committed adopter's emitted scripts, translating only their Actions
// environment expressions; acquisition never receives a provider credential.
// per docs/adr/0156-integrated-adopter-rehearsal.md:29
func integratedAdopterReview(project ciPrepareProject, label string) {
	GinkgoHelper()
	f, _, root := project.checkout()
	integratedAdopterTransport(f, root, project.image)
	retained := []string{f.root, root, project.repository, project.image.work, project.image.archivePath, filepath.Join(project.image.work, "exact-source")}
	data, err := os.ReadFile(filepath.Join(root, installedReviewPath))
	Expect(err).NotTo(HaveOccurred())
	var workflow ciReviewWorkflow
	Expect(yaml.Unmarshal(data, &workflow)).To(Succeed())
	steps := map[string]ciReviewStep{}
	for _, step := range workflow.Jobs["review"].Steps {
		switch {
		case step.ID == "client":
			steps["select"] = step
		case step.ID == "installed-client":
			steps["bootstrap"] = step
		case strings.Contains(strings.ToLower(step.Name), "review legacy"):
			steps["review"] = step
		}
	}
	Expect(steps).To(HaveLen(3))
	before, head, index := installationSnapshot(root), imageGit(root, "rev-parse", "HEAD"), imageGit(root, "ls-files", "--stage", "-z")
	service := orchestrationReviewHTTP(f.root, func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"No findings."}}]}`)
	})
	f.environment["HTTPS_PROXY"], f.environment["SSL_CERT_FILE"] = service.proxy, service.ca
	client := ""
	for _, phase := range []string{"select", "bootstrap", "review"} {
		step := steps[phase]
		Expect(step.Run).NotTo(BeEmpty())
		environment := maps.Clone(f.environment)
		for key, value := range step.Env {
			switch key {
			case "GH_TOKEN":
				value = "fixture-job-token"
			case "REVIEW_API_KEY":
				value = "PRIVATE_INTEGRATED_REVIEW_KEY"
			case "REVIEW_GO_CLIENT":
				value = ""
			case "REVIEW_INSTALLED_CLIENT":
				value = client
			case "REVIEW_TIMEOUT_SECONDS":
				value = "1080"
			case "REVIEW_HTTP_RETRIES":
				value = "1"
			case "DEFAULT_BRANCH":
				value = "main"
			default:
				Expect(value).NotTo(ContainSubstring("${{"), "untranslated generated environment: %s", key)
			}
			environment[key] = value
		}
		writeFixture(f.environment["GITHUB_OUTPUT"], nil, 0600)
		out := integratedAdopterRun(label+".review."+phase, root, environment, retained, "", "/bin/bash", "--noprofile", "--norc", "-e", "-c", step.Run)
		Expect(out.status).To(BeZero(), "%s: %+v", phase, out)
		if phase == "select" {
			Expect(os.ReadFile(f.environment["GITHUB_OUTPUT"])).To(Equal([]byte("mode=legacy\ninstalled=true\n")))
		}
		if phase == "bootstrap" {
			output, err := os.ReadFile(f.environment["GITHUB_OUTPUT"])
			Expect(err).NotTo(HaveOccurred())
			client = filepath.Join(f.environment["RUNNER_TEMP"], "factory-review-client/factory-review-client")
			Expect(string(output)).To(Equal("path=" + client + "\n"))
			Expect(os.ReadFile(client)).To(Equal(project.image.binary))
			Expect(service.observed()).To(BeEmpty())
			ordinaryBootstrapCache(f)
		}
	}
	Expect(service.observed()).To(HaveLen(1))
	comment, err := os.ReadFile(filepath.Join(f.records, "review-comment"))
	Expect(err).NotTo(HaveOccurred())
	Expect(string(comment)).To(ContainSubstring("No findings."))
	Expect(string(comment)).NotTo(ContainSubstring("PRIVATE_"))
	var report map[string]any
	status, err := os.ReadFile(filepath.Join(f.environment["RUNNER_TEMP"], "factory-review-output/report.json"))
	Expect(err).NotTo(HaveOccurred())
	Expect(json.Unmarshal(status, &report)).To(Succeed())
	Expect(report["status"]).To(Equal("complete"))
	Expect(report["batch_coverage_available"]).To(BeFalse())
	urls, err := os.ReadFile(filepath.Join(f.records, "installer-urls"))
	Expect(err).NotTo(HaveOccurred())
	Expect(string(urls)).To(Equal(f.environment["REHEARSAL_REVIEW_URL"] + "\n"))
	for _, path := range []string{".factory/bin", ".factory/assets", ".factory/installation.current"} {
		_, err := os.Lstat(filepath.Join(root, path))
		Expect(os.IsNotExist(err)).To(BeTrue(), "standalone review does not prepare checkout runtime custody")
	}
	Expect(installationSnapshot(root)).To(Equal(before))
	Expect(imageGit(root, "rev-parse", "HEAD")).To(Equal(head))
	Expect(imageGit(root, "ls-files", "--stage", "-z")).To(Equal(index))
	integratedAdopterNoForbidden(f)
	remaining, err := filepath.Glob(filepath.Join(f.environment["RUNNER_TEMP"], "factory-review-bootstrap.*"))
	Expect(err).NotTo(HaveOccurred())
	Expect(remaining).To(BeEmpty())
}

// per docs/adr/0156-integrated-adopter-rehearsal.md:58
func integratedAdopterNoForbidden(f handoffBootstrapFixture) {
	GinkgoHelper()
	for _, name := range []string{"forbidden-tool", "acquisition-provider-key"} {
		_, err := os.Lstat(filepath.Join(f.records, name))
		Expect(os.IsNotExist(err)).To(BeTrue(), name)
	}
}

// Run the actual emitted preparation, checks and PR-only mock evaluation,
// including the application's deliberate nonzero result. Retain generated
// bytes and the Git baseline.
// per docs/adr/0156-integrated-adopter-rehearsal.md:25
// per docs/adr/0156-integrated-adopter-rehearsal.md:37
func integratedAdopterOrdinary(project ciPrepareProject, label string, push bool) {
	GinkgoHelper()
	f, _, root := project.checkout()
	integratedAdopterTransport(f, root, project.image)
	retained := []string{f.root, root, project.repository, project.image.work, project.image.archivePath, filepath.Join(project.image.work, "exact-source")}
	document, _ := initCIWorkflowRead(root)
	var preparation, checks, golden ciWorkflowStep
	goldenCount := 0
	for _, job := range document.Jobs {
		for _, step := range job.Steps {
			if strings.Contains(step.Run, "ci prepare") {
				preparation = step.ciWorkflowStep
			}
			if strings.Contains(step.Run, "./factory selftest") {
				checks = step.ciWorkflowStep
			}
			if strings.Contains(step.Run, "golden") {
				golden = step.ciWorkflowStep
				goldenCount++
			}
		}
	}
	Expect(preparation.Run).NotTo(BeEmpty())
	Expect(checks.Run).NotTo(BeEmpty())
	Expect(goldenCount).To(Equal(1))
	before := map[string][]byte{}
	for _, path := range strings.Split(strings.TrimSuffix(imageGit(root, "ls-files"), "\n"), "\n") {
		data, err := os.ReadFile(filepath.Join(root, path))
		Expect(err).NotTo(HaveOccurred(), path)
		before[path] = data
	}
	head, index := imageGit(root, "rev-parse", "HEAD"), imageGit(root, "ls-files", "--stage", "-z")
	privatePublished := ciPreparePrivateWitness(root, project.private)
	prepared := integratedAdopterRun(label+".ordinary.prepare", root, ciWorkflowStepEnvironment(f, preparation), retained, "", "/bin/bash", "--noprofile", "--norc", "-eo", "pipefail", "-c", preparation.Run)
	Expect(prepared.status).To(BeZero(), "%+v", prepared)
	privatePublished()
	defaultInitDescriptor(root, defaultInitTag, project.image.revision, project.image.binary)
	for _, applicationStatus := range []string{"0", "23"} {
		f.environment["REHEARSAL_APPLICATION_STATUS"] = applicationStatus
		prior := 0
		if data, err := os.ReadFile(f.environment["REHEARSAL_CHECK_LOG"]); err == nil {
			prior = strings.Count(string(data), "application check\n")
		}
		checked := integratedAdopterRun(label+".ordinary.check."+applicationStatus, root, ciWorkflowStepEnvironment(f, checks), retained, "", "/bin/bash", "--noprofile", "--norc", "-eo", "pipefail", "-c", checks.Run)
		AddReportEntry("integrated-adopter."+label+".harness-diff", imageGit(root, "diff", "--", ".claude", ".codex", ".mcp.json", "opencode.json"), ReportEntryVisibilityAlways)
		if applicationStatus == "0" {
			Expect(checked.status).To(BeZero(), "%+v", checked)
			for _, harness := range []string{"opencode", "claude", "codex"} {
				Expect(checked.stdout).To(ContainSubstring("harness-structural-eval: PASS harness=" + harness))
			}
			// Execute the emitted PR path against the preserved adopter's actual
			// golden tasks, rather than only inspecting its mock selector.
			// per docs/adr/0156-integrated-adopter-rehearsal.md:36
			mocked := integratedAdopterRun(label+".ordinary.mock-golden", root, ciWorkflowStepEnvironment(f, golden), retained, "", "/bin/bash", "--noprofile", "--norc", "-eo", "pipefail", "-c", golden.Run)
			Expect(mocked.status).To(BeZero(), "%+v", mocked)
			Expect(mocked.stdout).To(ContainSubstring("golden-task-eval: done"))
			result, err := os.ReadFile(filepath.Join(root, "eval/results/mock-current.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(syncJSON(string(result))).To(HaveKeyWithValue("harness", "mock"))
		} else {
			Expect(checked.status).NotTo(BeZero(), "a real application failure cannot pass emitted CI: %+v", checked)
			Expect(checked.stdout).NotTo(ContainSubstring("harness-structural-eval: PASS"))
		}
		data, err := os.ReadFile(f.environment["REHEARSAL_CHECK_LOG"])
		Expect(err).NotTo(HaveOccurred(), "the application control must reach its intended phase")
		Expect(strings.Count(string(data), "application check\n")).To(BeNumerically(">", prior))
	}
	f.environment["REHEARSAL_APPLICATION_STATUS"] = "0"
	for path, data := range before {
		Expect(os.ReadFile(filepath.Join(root, path))).To(Equal(data), path)
	}
	Expect(imageGit(root, "rev-parse", "HEAD")).To(Equal(head))
	Expect(imageGit(root, "ls-files", "--stage", "-z")).To(Equal(index))
	Expect(imageGit(root, "diff", "--exit-code")).To(BeEmpty(), "generated workflows must need no post-clone repair")
	urls, err := os.ReadFile(filepath.Join(f.records, "installer-urls"))
	Expect(err).NotTo(HaveOccurred())
	Expect(string(urls)).To(Equal(f.environment["REHEARSAL_CI_URL"] + "\n"))
	ordinaryBootstrapCache(f)
	entries, err := os.ReadDir(f.environment["RUNNER_TEMP"])
	Expect(err).NotTo(HaveOccurred())
	Expect(entries).To(BeEmpty())
	integratedAdopterNoForbidden(f)
	if push {
		integratedAdopterPush(f, root, label, retained)
	}
}

// Install only the documented opt-in hook at Git's absent destination. Make,
// the application check and installed policy gates remain real for both pushes.
// per docs/adr/0156-integrated-adopter-rehearsal.md:39
func integratedAdopterPush(f handoffBootstrapFixture, root, label string, retainedRoots []string) {
	GinkgoHelper()
	remote := filepath.Join(f.root, "disposable push remote")
	retained := append(append([]string{}, retainedRoots...), remote)
	imageGit(f.cwd, "init", "--bare", "--quiet", remote)
	imageGit(root, "remote", "add", "rehearsal", remote)
	imageGit(root, "switch", "--quiet", "-c", "integrated-topic")
	hook := installedCheckHookPath(root)
	body := installedCheckDocumentedHook()
	writeFixture(hook, body, 0750)
	original, err := os.Lstat(hook)
	Expect(err).NotTo(HaveOccurred())
	healthy := integratedAdopterRun(label+".push.healthy", root, f.environment, retained, "", "git", "push", "rehearsal", "HEAD:refs/heads/integrated-topic")
	Expect(healthy.status).To(BeZero(), "%+v", healthy)
	remoteBefore := imageGit(remote, "rev-parse", "refs/heads/integrated-topic")
	writeFixture(filepath.Join(root, "push-change.txt"), []byte("a later application commit\n"), 0644)
	imageGit(root, "add", "push-change.txt")
	imageGit(root, "-c", "core.hooksPath="+os.DevNull, "commit", "--quiet", "-m", "test: exercise a rejected application update")
	f.environment["REHEARSAL_APPLICATION_STATUS"] = "23"
	failed := integratedAdopterRun(label+".push.application-failure", root, f.environment, retained, "", "git", "push", "rehearsal", "HEAD:refs/heads/integrated-topic")
	Expect(failed.status).NotTo(BeZero(), "%+v", failed)
	Expect(imageGit(remote, "rev-parse", "refs/heads/integrated-topic")).To(Equal(remoteBefore))
	current, err := os.Lstat(hook)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.SameFile(original, current)).To(BeTrue())
	Expect(current.Mode()).To(Equal(original.Mode()))
	Expect(os.ReadFile(hook)).To(Equal(body))
	integratedAdopterNoForbidden(f)
}

// The existing initialized-CI owner supplies its unpatched source. Package one
// real stable fixture image and initialize the immutable legacy tree only once.
// per docs/adr/0156-integrated-adopter-rehearsal.md:50
func integratedAdopterRehearsalSpecs(imageAccessor func() handoffReleaseImage) {
	Describe("integrated adopter rehearsal", func() {
		var image handoffReleaseImage
		var fixtures *defaultUpgradeJourneyFixtures
		BeforeAll(func() {
			image = initCIPackagedVersion(imageAccessor(), defaultInitTag, "integrated-adopter-image")
			fixtures = integratedAdopterFixtures(image)
			AddReportEntry("integrated-adopter.fixture-selection", map[string]string{"version": defaultInitTag, "revision": image.revision, "sha256": fmt.Sprintf("%x", sha256.Sum256(image.binary)), "authentication": "recording release/attestation fixtures; not a published release"}, ReportEntryVisibilityAlways)
		})

		// per docs/adr/0156-integrated-adopter-rehearsal.md:25
		// per docs/adr/0156-integrated-adopter-rehearsal.md:39
		It("rehearses ordinary stable initialization and both clean-checkout jobs with real factory owners", func() {
			f, _ := fixtures.surface(defaultInitTag)
			repository := filepath.Join(f.cwd, "fresh integrated adopter")
			retained := []string{f.root, repository, image.work, image.archivePath, filepath.Join(image.work, "exact-source")}
			hook := filepath.Join(repository, ".githooks/operator-owned")
			writeFixture(hook, []byte("operator-owned hook content\n"), 0640)
			originalHook, err := os.Lstat(hook)
			Expect(err).NotTo(HaveOccurred())
			f.environment["REVIEW_MODEL"] = "fixture/integrated-review"
			answers := nativeInitInput([]string{"Integrated Project", "integrated-project", "@fixture", "fixture-user", "", "docs", "", "openrouter", "economy", "y"}, nil, "y")
			initialized := integratedAdopterRun("fresh.init", f.cwd, f.environment, retained, answers, "/bin/sh", f.installer, "init", repository, "--ref", defaultInitTag, "--pack", "none")
			Expect(initialized.status).To(BeZero(), "%+v", initialized)
			defaultInitDescriptor(repository, defaultInitTag, image.revision, image.binary)
			currentHook, err := os.Lstat(hook)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.SameFile(originalHook, currentHook)).To(BeTrue())
			Expect(currentHook.Mode()).To(Equal(originalHook.Mode()))
			integratedAdopterApplication(repository)
			project := initCICommit(image, repository)
			integratedAdopterReview(project, "fresh")
			integratedAdopterOrdinary(project, "fresh", true)
			defaultInitNoForbidden(f)
			ordinaryBootstrapCache(f)
		})

		// per docs/adr/0156-integrated-adopter-rehearsal.md:32
		// per docs/adr/0156-integrated-adopter-rehearsal.md:42
		It("rehearses genuine v0.1.6 adoption and clean-checkout jobs while preserving originals and rollback", func() {
			p := fixtures.legacyProject()
			retained := []string{p.f.root, p.root, image.work, image.archivePath, filepath.Join(image.work, "exact-source"), filepath.Dir(p.legacy.source)}
			p.f.environment["REHEARSAL_CHECK_LOG"] = filepath.Join(p.f.records, "original-application-checks")
			p.f.environment["REHEARSAL_APPLICATION_STATUS"] = "0"
			integratedAdopterApplication(p.root)
			ciTransactionSetting(p.root, "review_model", "fixture/integrated-review")
			enabled := integratedAdopterRun("migrated.legacy-review-enable", p.root, p.f.environment, retained, "", "/bin/bash", filepath.Join(p.root, "factory"), "review-lane", "enable", "INTEGRATED_REVIEW_KEY")
			Expect(enabled.status).To(BeZero(), "%+v", enabled)
			// A real v0.1.6 success/rollback chain has no Go-only consumed ledger;
			// modern-history refusal remains covered by the state compatibility family.
			canaries := map[string][]byte{"application.txt": []byte("application canary\n"), ".githooks/operator-owned": []byte("operator-owned hook content\n"), ".factory/operator-history.json": []byte("{\"note\":\"retain operator history\"}\n")}
			for path, data := range canaries {
				writeFixture(filepath.Join(p.root, path), data, 0640)
			}
			hookInfo, err := os.Lstat(filepath.Join(p.root, ".githooks/operator-owned"))
			Expect(err).NotTo(HaveOccurred())
			for _, path := range []string{"factory.yaml", "opencode.json", ".claude/settings.json", ".codex/config.toml", installedReviewPath} {
				data, err := os.ReadFile(filepath.Join(p.root, path))
				Expect(err).NotTo(HaveOccurred())
				canaries[path] = data
			}
			before := installationSnapshot(p.root)
			args := defaultUpgradeInputArgs(p.inputs, "--ref", defaultInitTag)
			run := func(phase string, extra ...string) cliResult {
				return integratedAdopterRun(phase, p.root, p.f.environment, retained, "", "/bin/sh", append([]string{p.f.installer}, append(append([]string{}, args...), extra...)...)...)
			}
			proposal := handoffProposal(run("migrated.preview", "--dry-run", "--json"))
			Expect(installationSnapshot(p.root)).To(Equal(before))
			Expect(installedReviewRows(proposal)[installedReviewPath]["disposition"]).To(Equal("replace"))
			applied := run("migrated.apply", "--confirm", proposal["proposal_digest"].(string), "--quiescent")
			Expect(applied.status).To(BeZero(), "%+v", applied)
			defaultInitDescriptor(p.root, defaultInitTag, image.revision, image.binary)
			for path, data := range canaries {
				if path != installedReviewPath {
					Expect(os.ReadFile(filepath.Join(p.root, path))).To(Equal(data), path)
				}
			}
			hookAfter, err := os.Lstat(filepath.Join(p.root, ".githooks/operator-owned"))
			Expect(err).NotTo(HaveOccurred())
			Expect(os.SameFile(hookInfo, hookAfter)).To(BeTrue())
			Expect(hookAfter.Mode()).To(Equal(hookInfo.Mode()))
			ids := retentionBackupIDs(p.root)
			Expect(ids).To(HaveLen(1))
			Expect(os.ReadFile(filepath.Join(p.root, ".factory/backups", ids[0], "assets", installedReviewPath))).To(Equal(canaries[installedReviewPath]))
			retired := []string{}
			for _, value := range proposal["actions"].([]any) {
				action := value.(map[string]any)
				if action["action"] == "retire" && action["before"] != nil {
					path := action["path"].(string)
					retired = append(retired, path)
					_, err := os.Lstat(filepath.Join(p.root, path))
					Expect(os.IsNotExist(err)).To(BeTrue(), path)
					saved, err := os.Lstat(filepath.Join(p.root, ".factory/backups", ids[0], "assets", path))
					Expect(err).NotTo(HaveOccurred())
					Expect(saved.Mode().Perm() & 0111).To(BeZero())
				}
			}
			Expect(retired).To(ContainElement(".github/workflows/ci.yml"))
			Expect(imageProcess(p.root, nil, "git", "check-ignore", "--no-index", "-q", "--", ".factory/backups/"+ids[0]).status).To(BeZero())
			project := initCICommit(image, p.root)
			integratedAdopterReview(project, "migrated")
			integratedAdopterOrdinary(project, "migrated", false)
			defaultInitNoForbidden(p.f)
			rollback := fixtures.rollbackArgs(p, defaultInitTag, ids[0])
			// Only restored historical health receives its scoped old-runtime tools.
			environment := defaultUpgradeLegacyRecoveryEnvironment(p)
			retained = append(retained, filepath.Join(filepath.Dir(p.legacy.source), "external tools"))
			reverse := handoffProposal(integratedAdopterRun("migrated.rollback-preview", p.root, environment, retained, "", p.driver, append(append([]string{}, rollback...), "--dry-run", "--json")...))
			restored := integratedAdopterRun("migrated.rollback", p.root, environment, retained, "", p.driver, append(rollback, "--confirm", reverse["proposal_digest"].(string), "--quiescent")...)
			Expect(restored.status).To(BeZero(), "%+v", restored)
			for path, data := range canaries {
				Expect(os.ReadFile(filepath.Join(p.root, path))).To(Equal(data), path)
			}
			for _, path := range retired {
				saved, err := os.ReadFile(filepath.Join(p.root, ".factory/backups", ids[0], "assets", path))
				Expect(err).NotTo(HaveOccurred())
				Expect(os.ReadFile(filepath.Join(p.root, path))).To(Equal(saved))
			}
			ordinaryBootstrapCache(p.f)
		})
	})
}
