package acceptance_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"
)

// Package the actual committed selected source, rather than copying selected
// private state into the future checkout or constructing a generated oracle.
// per docs/adr/0146-initialized-installed-ci.md:112
func initCIPackagedImage() handoffReleaseImage {
	GinkgoHelper()
	image := handoffBuildImage()
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	source := filepath.Join(image.work, "exact-source")
	imageGit(source, "fetch", "--quiet", "--no-tags", repository, "b71ecc32e07ecd87eb330ba8e497c86612f92acd", "76952eaa63aebd1ecd282f5ab51dd7c3627cb497")
	bundle := localUpgradeBundle(source, image.revision, handoffVersion, filepath.Join(image.work, "initialized-ci-image"))
	archive, err := os.ReadFile(bundle.Archive)
	Expect(err).NotTo(HaveOccurred())
	entries, _ := imageDecodeArchive(archive)
	image.binary, image.archive, image.archivePath = entries[0].body, archive, bundle.Archive
	return image
}

type initCIVariant struct{ name, packs, build string }

var initCIVariants = []initCIVariant{
	{"none", "none", ""}, {"go", "go", ""}, {"typescript", "typescript", ""},
	{"gradle", "java", "gradle"}, {"maven", "java", "maven"},
	{"go-typescript", "go,typescript", ""}, {"go-gradle", "go,java", "gradle"}, {"go-maven", "go,java", "maven"},
	{"typescript-gradle", "typescript,java", "gradle"}, {"typescript-maven", "typescript,java", "maven"},
	{"all-gradle", "go,typescript,java,go", "gradle"}, {"all-maven", "go,typescript,java", "maven"},
	{"maven-wrapper", "java", "maven"},
}

// per docs/adr/0146-initialized-installed-ci.md:112
func initCIInitialize(f handoffBootstrapFixture, driver, root string, variant initCIVariant, localSource string) cliResult {
	GinkgoHelper()
	args := []string{"init", root, "--pack", variant.packs}
	if localSource == "" {
		args = append(args, "--go-candidate", handoffVersion)
	} else {
		args = append(args, "--local", "--source", localSource)
	}
	if variant.build != "" {
		args = append(args, "--java-build-tool", variant.build)
	}
	versions := []string{}
	for _, language := range []string{"go", "java", "typescript"} {
		if strings.Contains(variant.packs, language) {
			versions = append(versions, map[string]string{"go": "1.27.3", "java": "25", "typescript": "24"}[language])
		}
	}
	return handoffNativeProcess(f.cwd, f.environment, nativeInitInput(nil, versions, "y"), driver, args...)
}

type initCIWorkflowStep struct {
	ciWorkflowStep `yaml:",inline"`
	If             string `yaml:"if"`
	Advisory       bool   `yaml:"continue-on-error"`
}

type initCIWorkflow struct {
	Name        string            `yaml:"name"`
	On          map[string]any    `yaml:"on"`
	Permissions map[string]string `yaml:"permissions"`
	Concurrency struct {
		Group  string `yaml:"group"`
		Cancel string `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
	Jobs map[string]struct {
		Runner  string               `yaml:"runs-on"`
		Timeout int                  `yaml:"timeout-minutes"`
		If      string               `yaml:"if"`
		Steps   []initCIWorkflowStep `yaml:"steps"`
	} `yaml:"jobs"`
}

// This schema adds the ADR's explicit mutation/mock conditions without widening
// the older explicit renderer's strict unknown-field oracle.
// per docs/adr/0146-initialized-installed-ci.md:51
func initCIWorkflowRead(root string) (initCIWorkflow, string) {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join(root, ".github/workflows/factory.yml"))
	Expect(err).NotTo(HaveOccurred(), "authenticated init must publish ordinary installed CI without a separate rendering command")
	var document initCIWorkflow
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	Expect(decoder.Decode(&document)).To(Succeed())
	return document, string(data)
}

// per docs/adr/0146-initialized-installed-ci.md:51
// per docs/adr/0146-initialized-installed-ci.md:143
func initCIWorkflowContract(document initCIWorkflow, image handoffReleaseImage, variant initCIVariant) {
	GinkgoHelper()
	Expect(document.On).To(HaveLen(3))
	Expect(document.On).To(HaveKey("pull_request"))
	Expect(document.On).To(HaveKey("push"))
	Expect(document.On).To(HaveKey("workflow_dispatch"))
	Expect(document.Permissions).To(Equal(map[string]string{"contents": "read"}))
	Expect(document.Concurrency.Group).To(ContainSubstring("github.event.pull_request.number"))
	Expect(document.Concurrency.Group).To(ContainSubstring("github.run_id"))
	Expect(ciWorkflowExpression(document.Concurrency.Cancel)).To(Equal("github.event_name == 'pull_request'"))
	Expect(document.Jobs).To(HaveLen(1))
	for _, job := range document.Jobs {
		Expect(job.Runner).To(Equal("ubuntu-24.04"))
		Expect(job.Timeout).To(Equal(20))
		Expect(job.If).To(ContainSubstring("github.event.repository.default_branch"))
		Expect(job.Steps[0].Uses).To(Equal("actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1"))
		Expect(job.Steps[0].With).To(HaveKeyWithValue("fetch-depth", 0))
		Expect(job.Steps[0].With).To(HaveKeyWithValue("persist-credentials", false))
		setup := map[string]int{}
		allRuns := ""
		preparation, checks, golden, actionlintInstall, actionlintCheck := 0, 0, 0, 0, 0
		checksAt, setupAt := -1, 0
		for index, step := range job.Steps {
			allRuns += "\n" + step.Run
			if step.Uses != "" {
				Expect(step.Uses).To(MatchRegexp(`^[A-Za-z0-9_./-]+@[0-9a-f]{40}$`))
			}
			if strings.Contains(step.Run, "ci prepare") {
				preparation++
				Expect(index).To(Equal(1), "installation must precede application setup")
				Expect(step.Env).To(HaveKeyWithValue("FACTORY_RELEASE_TAG", handoffVersion))
				Expect(step.Env).To(HaveKeyWithValue("FACTORY_ORIGIN_TARGET", runtime.GOOS+"/"+runtime.GOARCH))
				Expect(step.Env).To(HaveKeyWithValue("FACTORY_INSTALLER_REVISION", image.revision))
				Expect(step.Env["GH_TOKEN"]).To(Equal("${{ github.token }}")) // #nosec G101 -- fixed public expression, not a credential.
			} else {
				Expect(step.Env).NotTo(HaveKey("GH_TOKEN"))
				Expect(step.Env).NotTo(HaveKey("GITHUB_TOKEN"))
			}
			for _, language := range []string{"go", "node", "java"} {
				if !strings.HasPrefix(step.Uses, "actions/setup-"+language+"@") {
					continue
				}
				setup[language]++
				setupAt = index
				Expect(index).To(BeNumerically(">", 1))
				Expect(step.With).To(HaveKeyWithValue("token", ""))
				switch language {
				case "go":
					Expect(step.With["go-version"]).To(Equal("1.27.3"))
					Expect(step.With["cache"]).To(Equal(false))
				case "node":
					Expect(fmt.Sprint(step.With["node-version"])).To(Equal("24"))
					Expect(step.With["package-manager-cache"]).To(Equal(false))
					Expect(step.With).NotTo(HaveKey("cache"))
				case "java":
					Expect(fmt.Sprint(step.With["java-version"])).To(Equal("25"))
					Expect(step.With["distribution"]).To(Equal("temurin"))
					Expect(step.With["cache-jdk"]).To(Equal(false))
					Expect(step.With).NotTo(HaveKey("cache"))
				}
			}
			if strings.Contains(step.Run, "./factory selftest") {
				checks++
				checksAt = index
				for _, command := range []string{"./factory check", "--harness=opencode", "--harness=claude", "--harness=codex"} {
					Expect(step.Run).To(ContainSubstring(command))
				}
				Expect(step.Run).NotTo(ContainSubstring("sync"))
				Expect(step.Advisory).To(BeFalse())
				Expect(step.If).To(BeEmpty())
			}
			if strings.Contains(step.Run, "golden") {
				golden++
				Expect(ciWorkflowExpression(step.If)).To(Equal("github.event_name == 'pull_request'"))
				Expect(step.Run).To(ContainSubstring("mock"))
				Expect(step.Run).NotTo(ContainSubstring("live"))
				Expect(step.Advisory).To(BeFalse())
			}
			if step.Advisory {
				Expect(step.Run).To(MatchRegexp(`(gremlins|stryker|pitest)`), "only mutation analysis is advisory")
			}
			if strings.Contains(step.Run, "gremlins unleash") || strings.Contains(step.Run, "stryker run") || strings.Contains(step.Run, "pitest") {
				Expect(step.Advisory).To(BeTrue(), "each mutation analysis remains explicitly advisory")
			}
			if strings.Contains(step.Run, "actionlint_1.7.12_linux_amd64.tar.gz") {
				actionlintInstall++
				Expect(step.Advisory).To(BeFalse())
				Expect(step.If).To(BeEmpty())
			}
			if strings.HasPrefix(strings.TrimSpace(step.Run), "actionlint") {
				actionlintCheck++
				Expect(step.Advisory).To(BeFalse())
				Expect(step.If).To(BeEmpty())
				// per docs/adr/0146-initialized-installed-ci.md:194
				Expect(strings.TrimSpace(step.Run)).To(Equal(`actionlint -ignore 'unexpected key "cache-mode" for "job" section'`))
			}
		}
		Expect(preparation).To(Equal(1))
		Expect(checks).To(Equal(1))
		Expect(golden).To(Equal(1))
		Expect(actionlintInstall).To(Equal(1))
		Expect(actionlintCheck).To(Equal(1))
		Expect(checksAt).To(BeNumerically(">", setupAt), "all selected application setup precedes installed checks")
		for _, language := range []string{"go", "typescript", "java"} {
			expected := 0
			if strings.Contains(variant.packs, language) {
				expected = 1
			}
			key := language
			if key == "typescript" {
				key = "node"
			}
			Expect(setup[key]).To(Equal(expected), "one setup for selected %s", language)
		}
		Expect(allRuns).To(ContainSubstring("8aca8db96f1b94770f1b0d72b6dddcb1ebb8123cb3712530b08cc387b349a3d8"))
		Expect(allRuns).To(ContainSubstring("sha256sum"))
		Expect(allRuns).To(ContainSubstring("actionlint"))
		Expect(allRuns).NotTo(ContainSubstring("/main/"))
		if strings.Contains(variant.packs, "go") {
			for _, tool := range []string{"golangci-lint", "gosec", "govulncheck", "gremlins"} {
				Expect(allRuns).To(ContainSubstring(tool))
			}
			Expect(allRuns).NotTo(MatchRegexp(`gremlins[^\n]*\./\.\.\.`))
			Expect(allRuns).To(ContainSubstring("gosec ./..."))
			Expect(allRuns).To(ContainSubstring("govulncheck ./..."))
		}
		if strings.Contains(variant.packs, "typescript") {
			Expect(allRuns).To(ContainSubstring("npm ci"))
			Expect(allRuns).To(ContainSubstring("stryker"))
		}
		if variant.build == "gradle" {
			for _, task := range []string{"compileJava", "spotbugsMain", "pitest"} {
				Expect(allRuns).To(ContainSubstring(task))
			}
		}
		if strings.Contains(variant.packs, "typescript") || strings.Contains(variant.packs, "java") {
			found := false
			for _, step := range job.Steps {
				if step.Uses == "google/osv-scanner-action/osv-scanner-action@b1d267aea5211fa648d9d442fa64b90dbdcbdd36" {
					found = true
					Expect(step.With).To(HaveKey("scan-args"))
					Expect(step.Advisory).To(BeFalse())
					Expect(step.If).To(BeEmpty())
				}
			}
			Expect(found).To(BeTrue(), "the selected pack's explicit security scan must survive Makefile composition")
		}
	}
}

// per docs/adr/0146-initialized-installed-ci.md:30
var _ = Describe("Whole installation upgrade release candidate handoff initialized CI", Ordered, ContinueOnFailure, SpecPriority(2), Label("handoff-init-ci-project"), func() {
	var image handoffReleaseImage
	BeforeAll(func() { image = initCIPackagedImage() })

	// per docs/adr/0146-initialized-installed-ci.md:24
	// per docs/adr/0146-initialized-installed-ci.md:30
	// per docs/adr/0146-initialized-installed-ci.md:107
	It("creates an authenticated ordinary workflow with stable initial harnesses for every pack union", func() {
		for _, variant := range initCIVariants {
			f, driver := handoffNativeFixture(image)
			root := filepath.Join(f.cwd, "untouched initialized "+variant.name)
			if variant.name == "maven-wrapper" {
				writeFixture(filepath.Join(root, "mvnw"), []byte("#!/bin/sh\nexit 0\n"), 0755)
			}
			out := initCIInitialize(f, driver, root, variant, "")
			Expect(out.status).To(BeZero(), "%+v", out)
			document, workflow := initCIWorkflowRead(root)
			initCIWorkflowContract(document, image, variant)
			if variant.build == "maven" {
				command := "mvn -B verify"
				if variant.name == "maven-wrapper" {
					command = "./mvnw -B verify"
				}
				Expect(syncRead(root, "factory.yaml")).To(ContainSubstring(command))
			}
			AddReportEntry("init-ci.workflow-"+variant.name, workflow, ReportEntryVisibilityNever)
			for _, path := range []string{"ci.yml", "go-pack.yml", "typescript-pack.yml", "java-pack.yml"} {
				_, err := os.Lstat(filepath.Join(root, ".github/workflows", path))
				Expect(os.IsNotExist(err)).To(BeTrue(), "source-only workflow must not be newly installed: %s", path)
			}
			before := map[string]map[string]string{}
			for _, directory := range []string{".claude", ".codex", ".opencode"} {
				before[directory] = nativeInitArtifacts(filepath.Join(root, directory))
			}
			canonical := syncRead(root, "opencode.json")
			synced := handoffNativeProcess(root, f.environment, "", filepath.Join(root, "factory"), "sync", "all")
			Expect(synced.status).To(BeZero(), "%+v", synced)
			Expect(syncRead(root, "opencode.json")).To(Equal(canonical))
			for directory, artifacts := range before {
				Expect(nativeInitArtifacts(filepath.Join(root, directory))).To(Equal(artifacts), "initial %s must already be stable without a precommit repair", directory)
			}
			if variant.name == "all-gradle" {
				initCIExtraChecks(f, root, document)
			}
		}
	})

	// per docs/adr/0146-initialized-installed-ci.md:24
	// per docs/adr/0146-initialized-installed-ci.md:37
	It("preserves application workflows while refusing every occupied generated destination before publication", func() {
		for _, kind := range []string{"application workflows", "omitted workflow link", "file", "directory", "symlink", "workflow parent link", "github parent file"} {
			f, driver := handoffNativeFixture(image)
			root := filepath.Join(f.cwd, "project")
			Expect(os.Mkdir(root, 0700)).To(Succeed())
			imageGit(root, "init", "--quiet")
			imageGit(root, "config", "factory.operator-note", "preserve exactly")
			foreign := filepath.Join(f.cwd, "foreign workflow")
			writeFixture(foreign, []byte("foreign content\n"), 0600)
			destination := filepath.Join(root, ".github/workflows/factory.yml")
			switch kind {
			case "application workflows":
				for _, name := range []string{"ci.yml", "go-pack.yml", "typescript-pack.yml", "java-pack.yml", "application.yml"} {
					writeFixture(filepath.Join(root, ".github/workflows", name), []byte("name: exact adopter workflow "+name+"\n"), 0600)
				}
			case "omitted workflow link":
				Expect(os.MkdirAll(filepath.Dir(destination), 0700)).To(Succeed())
				Expect(os.Symlink(foreign, filepath.Join(root, ".github/workflows/ci.yml"))).To(Succeed())
			case "file":
				writeFixture(destination, []byte("existing factory-named application workflow\n"), 0600)
			case "directory":
				Expect(os.MkdirAll(destination, 0700)).To(Succeed())
			case "symlink":
				Expect(os.MkdirAll(filepath.Dir(destination), 0700)).To(Succeed())
				Expect(os.Symlink(foreign, destination)).To(Succeed())
			case "workflow parent link":
				Expect(os.Mkdir(filepath.Join(root, ".github"), 0700)).To(Succeed())
				Expect(os.Symlink(f.cwd, filepath.Dir(destination))).To(Succeed())
			case "github parent file":
				writeFixture(filepath.Join(root, ".github"), []byte("occupied parent\n"), 0600)
			}
			unchanged := installationPreservationWitness(root)
			before := nativeInitArtifacts(root)
			out := initCIInitialize(f, driver, root, initCIVariants[10], "")
			if kind == "application workflows" || kind == "omitted workflow link" {
				Expect(out.status).To(BeZero(), "%s: %+v", kind, out)
				after := nativeInitArtifacts(root)
				for path, original := range before {
					Expect(after).To(HaveKeyWithValue(path, original), "preserved application/Git input %s", path)
				}
				Expect(destination).To(BeAnExistingFile())
			} else {
				Expect(out.status).NotTo(BeZero(), "%s must refuse before initialization: %+v", kind, out)
				unchanged()
				Expect(out.stdout).NotTo(ContainSubstring("Proceed?"), "collision must be planned before confirmation")
			}
			Expect(os.ReadFile(foreign)).To(Equal([]byte("foreign content\n")))
		}
	})

	// per docs/adr/0146-initialized-installed-ci.md:43
	// per docs/adr/0146-initialized-installed-ci.md:184
	It("omits active CI for ineligible authenticated tags and offline local initialization", func() {
		const literalTag = "candidate-r4"
		bundle := localUpgradeBundle(filepath.Join(image.work, "exact-source"), image.revision, literalTag, filepath.Join(image.work, "literal-candidate-image"))
		archive, err := os.ReadFile(bundle.Archive)
		Expect(err).NotTo(HaveOccurred())
		entries, _ := imageDecodeArchive(archive)
		candidate := image
		candidate.binary, candidate.archive, candidate.archivePath = entries[0].body, archive, bundle.Archive
		for _, occupied := range []bool{false, true} {
			f, driver := handoffNativeFixture(candidate)
			writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+literalTag+"-"+f.target), candidate.binary, 0600)
			writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-installation-"+literalTag+"-"+f.target+".tar.gz"), candidate.archive, 0600)
			root := filepath.Join(f.cwd, "literal release project")
			workflow := filepath.Join(root, ".github/workflows/factory.yml")
			if occupied {
				writeFixture(workflow, []byte("adopter-owned factory workflow\n"), 0600)
			}
			out := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", root, "--go-candidate", literalTag, "--pack", "none")
			Expect(out.status).To(BeZero(), "authenticated literal candidate remains a valid initializer selection: %+v", out)
			if occupied {
				Expect(os.ReadFile(workflow)).To(Equal([]byte("adopter-owned factory workflow\n")))
			} else {
				_, err := os.Lstat(workflow)
				Expect(os.IsNotExist(err)).To(BeTrue(), "ineligible authenticated tags cannot produce an unusable ordinary workflow")
			}
			Expect(strings.ToLower(out.stdout + out.stderr)).To(ContainSubstring("canonical release"))
			defaultInitDescriptor(root, literalTag, image.revision, candidate.binary)
		}
		f, driver := localInitSurface(image)
		root := filepath.Join(f.cwd, "local project")
		writeFixture(filepath.Join(root, ".github/workflows/application.yml"), []byte("keep local application CI\n"), 0600)
		f.environment["FACTORY_INSTALLER_REVISION"] = strings.Repeat("a", 40)
		f.environment["FACTORY_ORIGIN_TARGET"] = "linux/amd64"
		out := initCIInitialize(f, driver, root, initCIVariants[10], filepath.Join(image.work, "exact-source"))
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(strings.ToLower(out.stdout + out.stderr)).To(ContainSubstring("authenticated release"))
		for _, name := range []string{"factory.yml", "ci.yml", "go-pack.yml", "typescript-pack.yml", "java-pack.yml"} {
			_, err := os.Lstat(filepath.Join(root, ".github/workflows", name))
			Expect(os.IsNotExist(err)).To(BeTrue(), "local init must not invent or copy %s", name)
		}
		Expect(os.ReadFile(filepath.Join(root, ".github/workflows/application.yml"))).To(Equal([]byte("keep local application CI\n")))
		localInitDescriptor(root, image.revision)
		localInitNoAcquisition(f)
	})

	// per docs/adr/0146-initialized-installed-ci.md:118
	It("runs the committed initialized workflow through clone preparation and real Makefile gates without repairing harnesses", func() {
		producer, driver := handoffNativeFixture(image)
		repository := filepath.Join(producer.cwd, "application")
		writeFixture(filepath.Join(repository, "package.json"), []byte("{\"name\":\"prepared-app\",\"private\":true}\n"), 0644)
		writeFixture(filepath.Join(repository, "application.txt"), []byte("application-owned\n"), 0644)
		writeFixture(filepath.Join(repository, ".github/workflows/application.yml"), []byte("name: Application CI\non: workflow_dispatch\njobs:\n  app:\n    runs-on: ubuntu-24.04\n    steps:\n      - run: echo application\n"), 0644)
		out := initCIInitialize(producer, driver, repository, initCIVariants[2], "")
		Expect(out.status).To(BeZero(), "%+v", out)
		// Commit initialized bytes directly: no synchronization, config edits or
		// replacement Makefile is allowed between init and this Git publication.
		project := initCICommit(image, repository)
		f, _, root := project.checkout()
		document, rendered := initCIWorkflowRead(root)
		var preparation, checks, golden ciWorkflowStep
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
				}
			}
		}
		Expect(preparation.Run).NotTo(BeEmpty())
		Expect(checks.Run).NotTo(BeEmpty())
		Expect(golden.Run).NotTo(BeEmpty())
		ciWorkflowTransport(f, root, "success")
		f.environment["CI_WORKFLOW_URL"] = "https://raw.githubusercontent.com/anoop2811/software-factory-template/" + image.revision + "/install.sh"
		Expect(os.ReadFile(f.installer)).To(Equal([]byte(imageGit(filepath.Join(image.work, "exact-source"), "show", image.revision+":install.sh"))), "the exact emitted revision must own the transported installer")
		preserved := map[string][]byte{}
		for _, path := range []string{"factory.yaml", "Makefile", "package.json", "opencode.json", ".claude/settings.json", ".codex/config.toml", ".github/workflows/factory.yml", ".github/workflows/application.yml"} {
			data, err := os.ReadFile(filepath.Join(root, path))
			Expect(err).NotTo(HaveOccurred())
			preserved[path] = data
		}
		Expect(string(preserved["Makefile"])).To(ContainSubstring("check-drift: sync-harnesses"))
		head, index := imageGit(root, "rev-parse", "HEAD"), imageGit(root, "ls-files", "--stage", "-z")
		privatePublished := ciPreparePrivateWitness(root, project.private)
		prepared := ciWorkflowRunStep(f, root, preparation)
		Expect(prepared.status).To(BeZero(), "%+v", prepared)
		privatePublished()
		// Only external application tools are fixture-owned; the generated Makefile,
		// installed factory, policy gates and three structural evaluators stay real.
		writeFixture(filepath.Join(f.root, "host tools/npx"), []byte("#!/bin/sh\ntest -z \"${GH_TOKEN-}\" || exit 92\nprintf 'npx %s\\n' \"$*\" >> \"$HANDOFF_RECORD_DIR/application-tools\"\nexit 0\n"), 0700)
		writeFixture(filepath.Join(root, "node_modules/.bin/tsc"), []byte("#!/bin/sh\nprintf 'tsc %s\\n' \"$*\" >> \"$HANDOFF_RECORD_DIR/application-tools\"\nexit 0\n"), 0700)
		writeFixture(filepath.Join(root, "node_modules/.bin/vitest"), []byte("#!/bin/sh\nprintf 'vitest %s\\n' \"$*\" >> \"$HANDOFF_RECORD_DIR/application-tools\"\nexit \"${INIT_CI_APPLICATION_STATUS:-0}\"\n"), 0700)
		previousCalls := 0
		for _, status := range []string{"0", "23"} {
			f.environment["INIT_CI_APPLICATION_STATUS"] = status
			checked := ciWorkflowRunStep(f, root, checks)
			AddReportEntry("init-ci.emitted-checks", map[string]any{"application_status": status, "status": checked.status, "stdout": checked.stdout, "stderr": checked.stderr}, ReportEntryVisibilityAlways)
			if status == "0" {
				Expect(checked.status).To(BeZero(), "%+v", checked)
				Expect(checked.stdout).To(ContainSubstring("make check-drift"))
				for _, harness := range []string{"opencode", "claude", "codex"} {
					Expect(checked.stdout).To(ContainSubstring("harness-structural-eval: PASS harness=" + harness))
				}
				f.environment["ANTHROPIC_API_KEY"] = "independent-unused-test-value"
				f.environment["OPENAI_API_KEY"] = "independent-unused-test-value"
				mocked := ciWorkflowRunStep(f, root, golden)
				AddReportEntry("init-ci.mock-golden", map[string]any{"status": mocked.status, "stdout": mocked.stdout, "stderr": mocked.stderr}, ReportEntryVisibilityAlways)
				Expect(mocked.status).To(BeZero(), "emitted PR-only mock evaluation must remain unpaid even with ambient credentials: %+v", mocked)
				Expect(mocked.stdout).To(ContainSubstring("golden-task-eval: done"))
				result, err := os.ReadFile(filepath.Join(root, "eval/results/mock-current.json"))
				Expect(err).NotTo(HaveOccurred())
				Expect(syncJSON(string(result))).To(HaveKeyWithValue("harness", "mock"))
			} else {
				Expect(checked.status).NotTo(BeZero(), "%+v", checked)
				Expect(checked.stdout).NotTo(ContainSubstring("harness-structural-eval: PASS"))
			}
			calls, err := os.ReadFile(filepath.Join(f.records, "application-tools"))
			Expect(err).NotTo(HaveOccurred())
			count := strings.Count(string(calls), "vitest run\n")
			// The actual pack Makefile runs its test prerequisite, then the
			// configured check command. Failure of the prerequisite stops both.
			// per docs/adr/0146-initialized-installed-ci.md:118
			expectedCalls := 2
			if status != "0" {
				expectedCalls = 1
			}
			Expect(count-previousCalls).To(Equal(expectedCalls), "application status=%s; recorded tools:\n%s", status, calls)
			previousCalls = count
			AddReportEntry("init-ci.application-tools", string(calls), ReportEntryVisibilityNever)
		}
		for path, expected := range preserved {
			Expect(os.ReadFile(filepath.Join(root, path))).To(Equal(expected), path)
		}
		Expect(imageGit(root, "rev-parse", "HEAD")).To(Equal(head))
		Expect(imageGit(root, "ls-files", "--stage", "-z")).To(Equal(index))
		Expect(imageGit(root, "diff", "--exit-code")).To(BeEmpty(), "fresh initializer output must need no post-clone repair")
		for _, name := range []string{"forbidden-tool", "checkout-installer-executed"} {
			_, err := os.Lstat(filepath.Join(f.records, name))
			Expect(os.IsNotExist(err)).To(BeTrue(), name)
		}
		entries, err := os.ReadDir(f.environment["RUNNER_TEMP"])
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
		AddReportEntry("init-ci.committed-workflow", rendered, ReportEntryVisibilityNever)
	})
})

// per docs/adr/0146-initialized-installed-ci.md:118
func initCICommit(image handoffReleaseImage, root string) ciPrepareProject {
	GinkgoHelper()
	descriptor, err := os.ReadFile(filepath.Join(root, ".factory/installation.current"))
	Expect(err).NotTo(HaveOccurred())
	var selection struct {
		Assets []struct {
			Path string `json:"path"`
		} `json:"assets"`
	}
	Expect(json.Unmarshal(descriptor, &selection)).To(Succeed())
	project := ciPrepareProject{image: image, repository: root}
	for _, asset := range selection.Assets {
		if strings.HasPrefix(asset.Path, ".factory/") {
			project.private = append(project.private, asset.Path)
		} else {
			project.selected = append(project.selected, asset.Path)
		}
	}
	imageGit(root, "init", "--quiet")
	imageGit(root, "add", "--all")
	imageGit(root, "commit", "--quiet", "-m", "test: commit untouched initialized application")
	for _, path := range strings.Split(strings.TrimSuffix(imageGit(root, "ls-files"), "\n"), "\n") {
		Expect(path).NotTo(HavePrefix(".factory/"))
	}
	return project
}

// Execute emitted supplemental commands with bounded external-tool witnesses.
// This proves argv, unconditional failure propagation and advisory classification,
// not hosted tool installation or the tools' own analysis correctness.
// per docs/adr/0146-initialized-installed-ci.md:65
// per docs/adr/0146-initialized-installed-ci.md:123
func initCIExtraChecks(f handoffBootstrapFixture, root string, document initCIWorkflow) {
	GinkgoHelper()
	tool := []byte("#!/bin/sh\nname=${0##*/}\nprintf '%s %s\\n' \"$name\" \"$*\" >> \"$HANDOFF_RECORD_DIR/supplemental-tools\"\n[ \"${INIT_CI_FAIL_TOOL-}\" != \"$name\" ] || exit 37\n")
	for _, name := range []string{"actionlint", "gosec", "govulncheck", "gremlins", "npx"} {
		writeFixture(filepath.Join(f.root, "host tools", name), tool, 0700)
	}
	writeFixture(filepath.Join(root, "gradlew"), tool, 0755)
	for _, job := range document.Jobs {
		for _, step := range job.Steps {
			first := ""
			switch {
			case strings.HasPrefix(strings.TrimSpace(step.Run), "actionlint"):
				first = "actionlint"
			case strings.Contains(step.Run, "gosec ./..."):
				first = "gosec"
			case strings.Contains(step.Run, "spotbugsMain"):
				first = "gradlew"
			case strings.Contains(step.Run, "gremlins unleash"):
				first = "gremlins"
			case strings.Contains(step.Run, "stryker run"):
				first = "npx"
			case strings.Contains(step.Run, "pitest"):
				first = "gradlew"
			}
			if first == "" {
				continue
			}
			Expect(step.If).To(BeEmpty(), "selected security/mutation commands must not hide behind an event condition")
			f.environment["INIT_CI_FAIL_TOOL"] = ""
			out := ciWorkflowRunStep(f, root, step.ciWorkflowStep)
			Expect(out.status).To(BeZero(), "%s: %+v", first, out)
			calls, err := os.ReadFile(filepath.Join(f.records, "supplemental-tools"))
			Expect(err).NotTo(HaveOccurred())
			if strings.Contains(step.Run, "spotbugsMain") {
				Expect(string(calls)).To(ContainSubstring("gradlew --no-daemon compileJava compileTestJava\n"))
				Expect(string(calls)).To(ContainSubstring("gradlew --no-daemon spotbugsMain\n"))
			}
			if first == "gremlins" {
				Expect(string(calls)).To(ContainSubstring("gremlins unleash . --dry-run\n"))
			}
			f.environment["INIT_CI_FAIL_TOOL"] = first
			out = ciWorkflowRunStep(f, root, step.ciWorkflowStep)
			Expect(out.status).To(Equal(37), "the emitted script must preserve the external failure; only explicit mutation step metadata may make it advisory: %+v", out)
			if first == "gosec" {
				calls, err := os.ReadFile(filepath.Join(f.records, "supplemental-tools"))
				Expect(err).NotTo(HaveOccurred())
				Expect(strings.Count(string(calls), "govulncheck ./...\n")).To(Equal(1), "failed gosec must stop before the second scanner")
				f.environment["INIT_CI_FAIL_TOOL"] = "govulncheck"
				out = ciWorkflowRunStep(f, root, step.ciWorkflowStep)
				Expect(out.status).To(Equal(37), "the second scanner's error also remains mandatory: %+v", out)
			}
		}
	}
	delete(f.environment, "INIT_CI_FAIL_TOOL")
}
