package acceptance_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/releasecmd"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"
)

const ciWorkflowInstallerRevision = "9467b34d611bb62de7f8dfe8000404eb026f1dbe"

// per docs/adr/0145-installed-ci-workflow-rendering.md:29
func ciWorkflowArguments() []string {
	return []string{"ci", "workflow", "--go-candidate", handoffVersion, "--origin-target", "darwin/arm64", "--installer-revision", ciWorkflowInstallerRevision}
}

// per docs/adr/0145-installed-ci-workflow-rendering.md:93
var _ = Describe("Whole installation upgrade release candidate handoff CI checkout preparation", SpecPriority(0), Label("handoff-ci-workflow-surface"), func() {
	// per docs/adr/0145-installed-ci-workflow-rendering.md:29
	// per docs/adr/0145-installed-ci-workflow-rendering.md:36
	It("renders explicit workflow selectors and help without project or acquisition effects", func() {
		root, cwd, marker := handoffSurfaceFixture()
		driver := filepath.Join(root, "factory")
		env := map[string]string{"PATH": filepath.Join(root, "tools"), "HANDOFF_EXECUTION_MARKER": marker, "FACTORY_BRIDGE_PROTOCOL": "", "FACTORY_CONFIG": filepath.Join(cwd, "factory.yaml"), "FACTORY_REF": "main", "FACTORY_REPO": "file:///not-the-authority", "FACTORY_HOME": filepath.Join(cwd, "not-a-source"), "FACTORY_RELEASE_DIR": filepath.Join(cwd, "not-a-release")}
		writeFixture(filepath.Join(cwd, "factory.yaml"), []byte("invalid: [\n$(must-not-execute)\n"), 0600)
		writeFixture(filepath.Join(cwd, ".github/workflows/existing.yml"), []byte("keep existing workflow\n"), 0600)
		before := nativeInitArtifacts(cwd)
		for _, args := range [][]string{{"ci", "workflow", "--help"}, {"ci", "workflow", "-h"}, append(ciWorkflowArguments(), "--help")} {
			out := imageProcess(cwd, env, driver, args...)
			Expect(out.status).To(BeZero(), "%q: %+v", args, out)
			Expect(out.stdout).To(ContainSubstring("ci workflow"))
			Expect(out.stdout).To(ContainSubstring("--installer-revision"))
			Expect(out.stderr).To(BeEmpty())
		}
		valid := ciWorkflowArguments()
		requests := [][]string{{"ci", "workflow"}, {"ci", "workflow", "--go-candidate"}, {"ci", "workflow", "--origin-target"}, {"ci", "workflow", "--installer-revision"}}
		for _, missing := range []int{2, 4, 6} {
			requests = append(requests, append(append([]string{}, valid[:missing]...), valid[missing+2:]...))
		}
		for _, extra := range [][]string{{"--help", "-h"}, {"--help=true"}, {"--ephemeral"}, {"--unknown"}, {"--"}, {"operand"}, {"--go-candidate", handoffVersion}, {"--origin-target", "darwin/arm64"}, {"--installer-revision", ciWorkflowInstallerRevision}} {
			requests = append(requests, append(append([]string{}, valid...), extra...))
		}
		for name, values := range map[string][]string{"--go-candidate": {"", "main", "latest", "v01.2.3", "v1.2.3-01", "v1.2.3+é", "v1.2.3+" + strings.Repeat("a", 122)}, "--origin-target": {"", "windows/amd64", "linux/386"}, "--installer-revision": {"", "main", strings.Repeat("a", 39), strings.Repeat("A", 40), strings.Repeat("a", 41), strings.Repeat("g", 40), "$(touch marker)"}} {
			for _, value := range values {
				requests = append(requests, []string{"ci", "workflow", name + "=" + value, "--help"})
			}
		}
		for _, args := range requests {
			out := imageProcess(cwd, env, driver, args...)
			Expect(out.status).To(Equal(2), "%q: %+v", args, out)
			Expect(out.stdout).To(BeEmpty(), "refusal must not emit a partial workflow")
			Expect(out.stderr).NotTo(BeEmpty())
		}
		separate := imageProcess(cwd, env, driver, valid...)
		Expect(separate.status).To(BeZero(), "%+v", separate)
		equals := imageProcess(cwd, env, driver, "ci", "workflow", "--installer-revision="+ciWorkflowInstallerRevision, "--origin-target=darwin/arm64", "--go-candidate="+handoffVersion)
		Expect(equals.status).To(BeZero(), "%+v", equals)
		Expect(equals.stdout).To(Equal(separate.stdout))
		Expect(equals.stderr + separate.stderr).To(BeEmpty())
		for _, tag := range []string{"v0.0.0", "v1.2.3+001", "v1.2.3-rc.1+build.001", "v" + strings.Repeat("9", 80) + ".0.0", "v1.2.3+" + strings.Repeat("a", 121)} {
			out := imageProcess(cwd, env, driver, "ci", "workflow", "--go-candidate="+tag, "--help")
			Expect(out.status).To(BeZero(), "%s: %+v", tag, out)
		}
		for _, origin := range []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"} {
			out := imageProcess(cwd, env, driver, "ci", "workflow", "--origin-target="+origin, "--help")
			Expect(out.status).To(BeZero(), "%s: %+v", origin, out)
		}
		Expect(nativeInitArtifacts(cwd)).To(Equal(before))
		_, err := os.Lstat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue(), "rendering must not invoke acquisition or legacy tools")
	})

	// per docs/adr/0145-installed-ci-workflow-rendering.md:50
	// per docs/adr/0145-installed-ci-workflow-rendering.md:58
	// per docs/adr/0145-installed-ci-workflow-rendering.md:79
	It("renders one parsed least-privilege workflow with explicit setup and installed checks", func() {
		root, cwd, _ := handoffSurfaceFixture()
		out := imageProcess(cwd, map[string]string{"PATH": "", "FACTORY_BRIDGE_PROTOCOL": ""}, filepath.Join(root, "factory"), ciWorkflowArguments()...)
		Expect(out.status).To(BeZero(), "%+v", out)
		document := ciWorkflowDecode(out.stdout)
		Expect(document.Name).NotTo(BeEmpty())
		Expect(document.On).To(HaveLen(3))
		Expect(document.On).To(HaveKey("pull_request"))
		Expect(document.On).To(HaveKey("push"))
		Expect(document.On).To(HaveKey("workflow_dispatch"))
		Expect(document.Permissions).To(Equal(map[string]string{"contents": "read"}))
		Expect(document.Jobs).To(HaveLen(1))
		Expect(document.Concurrency.Group).To(ContainSubstring("github.workflow"))
		Expect(document.Concurrency.Group).To(ContainSubstring("github.event_name"))
		Expect(document.Concurrency.Group).To(ContainSubstring("github.event.pull_request.number"))
		Expect(document.Concurrency.Group).To(ContainSubstring("github.run_id"))
		Expect(document.Concurrency.Group).To(ContainSubstring("github.event_name == 'pull_request' && github.event.pull_request.number || github.run_id"), "PR updates share one key; independent push/manual runs have unique run IDs")
		Expect(ciWorkflowExpression(document.Concurrency.Cancel)).To(Equal("github.event_name == 'pull_request'"))
		for _, job := range document.Jobs {
			Expect(job.Runner).To(Equal("ubuntu-24.04"))
			Expect(job.Timeout).To(Equal(20))
			Expect(ciWorkflowExpression(job.If)).To(Equal("github.event_name != 'push' || github.ref == format('refs/heads/{0}', github.event.repository.default_branch)"))
			Expect(job.Steps).To(HaveLen(3), "checkout, preparation and checks; setup is a deliberate comment, not a successful no-op")
			checkout, preparation, checks := job.Steps[0], job.Steps[1], job.Steps[2]
			Expect(checkout.Uses).To(Equal("actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1"))
			Expect(checkout.With).To(Equal(map[string]any{"fetch-depth": 0, "persist-credentials": false}))
			Expect(checkout.Run).To(BeEmpty())
			Expect(checkout.Env).To(BeEmpty())
			// #nosec G101 -- This is a fixed GitHub expression, not a credential value.
			Expect(preparation.Env).To(Equal(map[string]string{"GH_TOKEN": "${{ github.token }}", "FACTORY_RELEASE_TAG": handoffVersion, "FACTORY_ORIGIN_TARGET": "darwin/arm64", "FACTORY_INSTALLER_REVISION": ciWorkflowInstallerRevision}))
			Expect(checks.Env).To(BeEmpty())
			for _, step := range []ciWorkflowStep{preparation, checks} {
				Expect(step.Uses).To(BeEmpty())
				Expect(step.Shell).To(Equal("bash"))
				Expect(step.Run).NotTo(BeEmpty())
				Expect(step.Run).NotTo(ContainSubstring("${{"), "GitHub expressions are data, never interpolated shell source")
				Expect(step.Run).NotTo(ContainSubstring("|| true"))
			}
			Expect(checks.Run).To(ContainSubstring("./factory selftest"))
			Expect(checks.Run).To(ContainSubstring("./factory check"))
			for _, harness := range []string{"opencode", "claude", "codex"} {
				Expect(checks.Run).To(ContainSubstring("./factory eval structural --harness=" + harness))
			}
			Expect(checks.Run).NotTo(ContainSubstring("sync"), "the job must expose structural drift, not repair it before evaluation")
			setup := strings.Index(strings.ToLower(out.stdout), "application-specific")
			Expect(setup).To(BeNumerically(">", strings.Index(out.stdout, strings.SplitN(preparation.Run, "\n", 2)[0])))
			Expect(setup).To(BeNumerically("<", strings.Index(out.stdout, "./factory selftest")))
		}
	})

	// per docs/adr/0145-installed-ci-workflow-rendering.md:36
	It("reports workflow output and cancellation failures without reading stdin", func() {
		args := ciWorkflowArguments()[1:]
		input := &ciWorkflowUnreadInput{}
		var stdout, stderr bytes.Buffer
		Expect(releasecmd.RunCI(context.Background(), args, map[string]string{"PATH": "", "FACTORY_CONFIG": "/not-a-project"}, input, &stdout, &stderr)).To(BeZero())
		Expect(stdout.Len()).To(BeNumerically(">", 0))
		Expect(input.read).To(BeFalse())
		failed := &ciWorkflowFailedOutput{}
		Expect(releasecmd.RunCI(context.Background(), args, nil, input, failed, &stderr)).NotTo(BeZero())
		Expect(failed.calls).To(BeNumerically(">", 0), "the actual renderer must encounter the output failure")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		stdout.Reset()
		Expect(releasecmd.RunCI(ctx, args, nil, input, &stdout, &stderr)).NotTo(BeZero())
		Expect(stdout.Len()).To(BeZero())
		Expect(input.read).To(BeFalse())
	})
})

// per docs/adr/0145-installed-ci-workflow-rendering.md:99
var _ = Describe("Whole installation upgrade release candidate handoff CI checkout preparation", Ordered, ContinueOnFailure, SpecPriority(2), Label("handoff-ci-workflow-project"), func() {
	var project ciPrepareProject
	BeforeAll(func() {
		project = ciPrepareProjectCustomized(func(root string, makefile []byte, environment map[string]string) {
			Expect(string(makefile)).To(ContainSubstring("check-drift: sync-harnesses"))
			Expect(string(makefile)).To(ContainSubstring("check: selftest"))
			writeFixture(filepath.Join(root, "Makefile"), makefile, 0644)
			writeFixture(filepath.Join(root, "check-application.sh"), []byte(`#!/bin/sh
set -eu
test -f "$CI_WORKFLOW_SETUP_READY"
test -z "${GH_TOKEN-}"
printf 'application check\n' >> "$CI_PREPARE_CHECK_LOG"
exit "${CI_PREPARE_CHECK_STATUS:-0}"
`), 0755)
			// Establish the initializer's documented prepared-project baseline before
			// the first Git commit; no synchronization is added to preparation or CI.
			// per docs/adr/0145-installed-ci-workflow-rendering.md:139
			first := handoffNativeProcess(root, environment, "", filepath.Join(root, "factory"), "sync", "all")
			Expect(first.status).To(BeZero(), "%+v", first)
			managed := map[string]map[string]string{}
			for _, directory := range []string{".claude", ".codex"} {
				managed[directory] = nativeInitArtifacts(filepath.Join(root, directory))
			}
			files := map[string][]byte{}
			for _, path := range []string{".mcp.json", "opencode.json"} {
				data, err := os.ReadFile(filepath.Join(root, path))
				Expect(err).NotTo(HaveOccurred())
				files[path] = data
			}
			second := handoffNativeProcess(root, environment, "", filepath.Join(root, "factory"), "sync", "all")
			Expect(second.status).To(BeZero(), "%+v", second)
			for directory, before := range managed {
				Expect(nativeInitArtifacts(filepath.Join(root, directory))).To(Equal(before), "second precommit synchronization must preserve every managed byte and mode")
			}
			for path, before := range files {
				Expect(os.ReadFile(filepath.Join(root, path))).To(Equal(before), path)
			}
		})
	})

	// per docs/adr/0145-installed-ci-workflow-rendering.md:64
	// per docs/adr/0145-installed-ci-workflow-rendering.md:79
	// per docs/adr/0145-installed-ci-workflow-rendering.md:99
	It("executes emitted preparation and real installed Makefile and harness checks without compiler or Python", func() {
		f, driver, root := project.checkout()
		preparation, checks, rendered := ciWorkflowEmittedSteps(f, driver, root)
		ciWorkflowTransport(f, root, "success")
		privatePublished := ciPreparePrivateWitness(root, project.private)
		preserved := map[string][]byte{}
		for _, path := range []string{"factory.yaml", "Makefile", ".github/workflows/application.yml", "application.txt", "notes/operator-note", ".factory/operator-note", "install.sh", ".claude/settings.json", ".codex/config.toml", "opencode.json"} {
			data, err := os.ReadFile(filepath.Join(root, path))
			Expect(err).NotTo(HaveOccurred())
			preserved[path] = data
		}
		head, index, paths := imageGit(root, "rev-parse", "HEAD"), imageGit(root, "ls-files", "--stage", "-z"), ciPreparePublicPaths(root)
		prepared := ciWorkflowRunStep(f, root, preparation)
		Expect(prepared.status).To(BeZero(), "%+v", prepared)
		privatePublished()
		defaultInitDescriptor(root, handoffVersion, project.image.revision, project.image.binary)
		ciWorkflowDownloadSettled(f)
		// This deliberate adopter setup runs at the emitted comment's position,
		// after clean-checkout preparation and before the unchanged check block.
		setup := handoffNativeProcess(root, f.environment, "", "/bin/bash", "-eu", "-c", `test -f .factory/installation.current; test -z "${GH_TOKEN-}"; printf ready > "$CI_WORKFLOW_SETUP_READY"`)
		Expect(setup.status).To(BeZero(), "%+v", setup)
		for _, status := range []string{"0", "23"} {
			f.environment["CI_PREPARE_CHECK_STATUS"] = status
			checked := ciWorkflowRunStep(f, root, checks)
			AddReportEntry("ci-workflow.emitted-checks", map[string]any{"application_status": status, "status": checked.status, "stdout": checked.stdout, "stderr": checked.stderr}, ReportEntryVisibilityAlways)
			AddReportEntry("ci-workflow.managed-config-diff", imageGit(root, "diff", "--", ".claude", ".codex", ".mcp.json", "opencode.json"), ReportEntryVisibilityNever)
			if status == "0" {
				Expect(checked.status).To(BeZero(), "%+v", checked)
				Expect(checked.stdout).To(ContainSubstring("make check-drift"))
				for _, harness := range []string{"opencode", "claude", "codex"} {
					Expect(checked.stdout).To(ContainSubstring("harness-structural-eval: PASS harness=" + harness))
				}
			} else {
				Expect(checked.status).NotTo(BeZero(), "emitted fail-fast block must retain real application failure: %+v", checked)
				Expect(checked.stdout).NotTo(ContainSubstring("harness-structural-eval: PASS"), "later checks must not disguise the failed application stage")
			}
		}
		Expect(os.ReadFile(f.environment["CI_PREPARE_CHECK_LOG"])).To(Equal([]byte("application check\napplication check\n")))
		for path, data := range preserved {
			Expect(os.ReadFile(filepath.Join(root, path))).To(Equal(data), path)
		}
		Expect(imageGit(root, "rev-parse", "HEAD")).To(Equal(head))
		Expect(imageGit(root, "ls-files", "--stage", "-z")).To(Equal(index))
		Expect(ciPreparePublicPaths(root)).To(Equal(paths))
		for _, name := range []string{"forbidden-tool", "checkout-installer-executed", "partial-installer-executed"} {
			_, err := os.Lstat(filepath.Join(f.records, name))
			Expect(os.IsNotExist(err)).To(BeTrue(), name)
		}
		ordinaryBootstrapCache(f)
		AddReportEntry("ci-workflow.rendered", rendered, ReportEntryVisibilityNever)
	})

	// per docs/adr/0145-installed-ci-workflow-rendering.md:64
	// per docs/adr/0145-installed-ci-workflow-rendering.md:105
	It("refuses failed or empty emitted installer downloads without executing checkout or partial bytes", func() {
		for _, mode := range []string{"failed-partial", "empty"} {
			f, driver, root := project.checkout()
			preparation, _, _ := ciWorkflowEmittedSteps(f, driver, root)
			ciWorkflowTransport(f, root, mode)
			unchanged := installationPreservationWitness(root)
			out := ciWorkflowRunStep(f, root, preparation)
			Expect(out.status).NotTo(BeZero(), "%s: %+v", mode, out)
			ciWorkflowDownloadSettled(f)
			unchanged()
			for _, name := range []string{"trace", "checkout-installer-executed", "partial-installer-executed", "forbidden-tool"} {
				_, err := os.Lstat(filepath.Join(f.records, name))
				Expect(os.IsNotExist(err)).To(BeTrue(), "%s must not reach %s", mode, name)
			}
		}
	})
})

// Render through the actual public executable, decode it, and return its exact
// run blocks. No independently reconstructed bootstrap/check command is used.
// per docs/adr/0145-installed-ci-workflow-rendering.md:99
func ciWorkflowEmittedSteps(f handoffBootstrapFixture, driver, root string) (ciWorkflowStep, ciWorkflowStep, string) {
	GinkgoHelper()
	args := ciWorkflowArguments()
	args[5] = runtime.GOOS + "/" + runtime.GOARCH
	out := handoffNativeProcess(root, f.environment, "", driver, args...)
	Expect(out.status).To(BeZero(), "%+v", out)
	document := ciWorkflowDecode(out.stdout)
	Expect(document.Jobs).To(HaveLen(1))
	for _, job := range document.Jobs {
		Expect(job.Steps).To(HaveLen(3))
		return job.Steps[1], job.Steps[2], out.stdout
	}
	panic("one parsed workflow job is required")
}

// per docs/adr/0145-installed-ci-workflow-rendering.md:64
func ciWorkflowTransport(f handoffBootstrapFixture, root, mode string) {
	GinkgoHelper()
	f.environment["RUNNER_TEMP"] = filepath.Join(f.root, "runner temporary")
	Expect(os.Mkdir(f.environment["RUNNER_TEMP"], 0700)).To(Succeed())
	f.environment["CI_WORKFLOW_INSTALLER"] = f.installer
	f.environment["CI_WORKFLOW_URL"] = "https://raw.githubusercontent.com/anoop2811/software-factory-template/" + ciWorkflowInstallerRevision + "/install.sh"
	f.environment["CI_WORKFLOW_DOWNLOAD_MODE"] = mode
	f.environment["CI_WORKFLOW_SETUP_READY"] = filepath.Join(f.records, "application-setup")
	f.environment["GH_TOKEN"] = ""
	writeFixture(filepath.Join(root, "install.sh"), []byte("#!/bin/sh\nprintf called > \"$HANDOFF_RECORD_DIR/checkout-installer-executed\"\nexit 97\n"), 0755)
	for _, client := range []string{"opencode", "claude", "codex"} {
		writeFixture(filepath.Join(f.root, "host tools", client), []byte("#!/bin/sh\nprintf called >> \"$HANDOFF_RECORD_DIR/forbidden-tool\"\nexit 97\n"), 0700)
	}
	writeFixture(filepath.Join(f.root, "host tools/curl"), []byte(`#!/bin/sh
set -eu
destination= url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o|--output) shift; destination=$1 ;;
    --proto|--proto-redir) shift; [ "$1" = '=https' ] || exit 91 ;;
    --tlsv1.2) ;;
    -f|-s|-S|-L|-fsSL|--fail|--silent|--show-error|--location) ;;
    https://*) [ -z "$url" ] || exit 92; url=$1 ;;
    *) exit 93 ;;
  esac
  shift
done
[ "$url" = "$CI_WORKFLOW_URL" ] || exit 94
case "$destination" in "$RUNNER_TEMP"/*) ;; *) exit 95 ;; esac
[ -f "$destination" ] && [ ! -L "$destination" ] || exit 96
printf '%s\n%s\n' "$url" "$destination" > "$HANDOFF_RECORD_DIR/installer-download"
/bin/ls -ld "$destination" > "$HANDOFF_RECORD_DIR/installer-download-mode"
case "$CI_WORKFLOW_DOWNLOAD_MODE" in
  failed-partial) printf '#!/bin/sh\nprintf called > "$HANDOFF_RECORD_DIR/partial-installer-executed"\n' > "$destination"; exit 22 ;;
  empty) : > "$destination" ;;
  success) /bin/cat "$CI_WORKFLOW_INSTALLER" > "$destination" ;;
  *) exit 98 ;;
esac
`), 0700)
}

// per docs/adr/0145-installed-ci-workflow-rendering.md:72
func ciWorkflowRunStep(f handoffBootstrapFixture, root string, step ciWorkflowStep) cliResult {
	GinkgoHelper()
	Expect(step.Shell).To(Equal("bash"))
	environment := map[string]string{}
	for key, value := range f.environment {
		environment[key] = value
	}
	for key, value := range step.Env {
		if key == "GH_TOKEN" {
			Expect(value).To(Equal("${{ github.token }}"))
			value = "independent-readonly-job-token"
		}
		environment[key] = value
	}
	return handoffNativeProcess(root, environment, "", "/bin/bash", "--noprofile", "--norc", "-eo", "pipefail", "-c", step.Run)
}

// per docs/adr/0145-installed-ci-workflow-rendering.md:66
func ciWorkflowDownloadSettled(f handoffBootstrapFixture) {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join(f.records, "installer-download"))
	Expect(err).NotTo(HaveOccurred(), "the emitted script must use the exact-bound transport")
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	Expect(lines).To(HaveLen(2))
	Expect(lines[0]).To(Equal(f.environment["CI_WORKFLOW_URL"]))
	Expect(lines[1]).To(HavePrefix(f.environment["RUNNER_TEMP"] + string(os.PathSeparator)))
	mode, err := os.ReadFile(filepath.Join(f.records, "installer-download-mode"))
	Expect(err).NotTo(HaveOccurred())
	Expect(string(mode)).To(HavePrefix("-rw-------"))
	entries, err := os.ReadDir(f.environment["RUNNER_TEMP"])
	Expect(err).NotTo(HaveOccurred())
	Expect(entries).To(BeEmpty(), fmt.Sprintf("no adjacent installer residue under %s", f.environment["RUNNER_TEMP"]))
}

type ciWorkflowDocument struct {
	Name        string            `yaml:"name"`
	On          map[string]any    `yaml:"on"`
	Permissions map[string]string `yaml:"permissions"`
	Concurrency struct {
		Group  string `yaml:"group"`
		Cancel string `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
	Jobs map[string]struct {
		Runner  string           `yaml:"runs-on"`
		Timeout int              `yaml:"timeout-minutes"`
		If      string           `yaml:"if"`
		Steps   []ciWorkflowStep `yaml:"steps"`
	} `yaml:"jobs"`
}

type ciWorkflowStep struct {
	Name  string            `yaml:"name"`
	Uses  string            `yaml:"uses"`
	With  map[string]any    `yaml:"with"`
	Env   map[string]string `yaml:"env"`
	Shell string            `yaml:"shell"`
	Run   string            `yaml:"run"`
}

// Reject unknown workflow fields so hidden credentials, matrices, caches or
// permissive failure settings cannot escape the parsed contract.
// per docs/adr/0145-installed-ci-workflow-rendering.md:50
func ciWorkflowDecode(source string) ciWorkflowDocument {
	GinkgoHelper()
	var document ciWorkflowDocument
	decoder := yaml.NewDecoder(strings.NewReader(source))
	decoder.KnownFields(true)
	Expect(decoder.Decode(&document)).To(Succeed())
	return document
}

func ciWorkflowExpression(value string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(value), "${{"), "}}"))
}

type ciWorkflowUnreadInput struct{ read bool }

func (r *ciWorkflowUnreadInput) Read([]byte) (int, error) {
	r.read = true
	return 0, errors.New("workflow rendering must not read stdin")
}

type ciWorkflowFailedOutput struct{ calls int }

func (w *ciWorkflowFailedOutput) Write([]byte) (int, error) {
	w.calls++
	return 0, errors.New("independent workflow output refusal")
}
