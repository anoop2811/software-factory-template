package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"
)

type publishedWorkflow struct {
	On          map[string]any          `yaml:"on"`
	Permissions map[string]string       `yaml:"permissions"`
	Jobs        map[string]publishedJob `yaml:"jobs"`
}

type publishedJob struct {
	Uses        string            `yaml:"uses"`
	Needs       any               `yaml:"needs"`
	If          string            `yaml:"if"`
	RunsOn      any               `yaml:"runs-on"`
	Timeout     int               `yaml:"timeout-minutes"`
	Permissions map[string]string `yaml:"permissions"`
	With        map[string]string `yaml:"with"`
	Env         map[string]string `yaml:"env"`
	Strategy    struct {
		FailFast *bool `yaml:"fail-fast"`
		Matrix   struct {
			Include []map[string]string `yaml:"include"`
		} `yaml:"matrix"`
	} `yaml:"strategy"`
	Steps []ciReviewStep `yaml:"steps"`
}

func publishedLoadWorkflow(path string) publishedWorkflow {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join("..", path))
	Expect(err).NotTo(HaveOccurred(), "the dedicated opt-in release qualification workflow must exist")
	var workflow publishedWorkflow
	Expect(yaml.Unmarshal(data, &workflow)).To(Succeed())
	return workflow
}

func publishedRunStep(script, root string, environment map[string]string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "bash", "-e", "-u", "-o", "pipefail", "-c", script) // #nosec G204 G702 -- reviewed workflow under evaluator-owned inert tools, never a candidate or live verifier.
	command.Dir = root
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		isolated := strings.HasPrefix(key, "GH_") || strings.HasPrefix(key, "GITHUB_") || strings.HasPrefix(key, "ACTIONS_") || strings.HasPrefix(key, "FACTORY_PUBLISHED_")
		if _, replaced := environment[key]; !replaced && !isolated {
			command.Env = append(command.Env, entry)
		}
	}
	for key, value := range environment {
		command.Env = append(command.Env, key+"="+value)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred(), "%s", stderr.String())
	status := 0
	if err != nil {
		var exit *exec.ExitError
		Expect(errors.As(err, &exit)).To(BeTrue())
		status = exit.ExitCode()
	}
	return cliResult{stdout.String(), stderr.String(), status}
}

func publishedRunScript(job publishedJob) string {
	GinkgoHelper()
	for _, step := range job.Steps {
		if step.Run != "" {
			return step.Run
		}
	}
	Fail("missing executable qualification step")
	return ""
}

var _ = Describe("Published release qualification workflow contract", func() {
	// per docs/adr/0107-published-release-qualification.md:28
	// per docs/adr/0107-published-release-qualification.md:44
	// per docs/adr/0107-published-release-qualification.md:123
	It("qualifies only after publication upload using a read-only four-native-target trusted harness", func() {
		workflow := publishedLoadWorkflow(".github/workflows/published-release-qualification.yml")
		Expect(workflow.On).To(HaveKey("workflow_call"))
		Expect(workflow.On).To(HaveKey("workflow_dispatch"))
		Expect(workflow.On).To(HaveLen(2), "PRs and ordinary source CI must never trigger published acquisition")
		for _, event := range []string{"workflow_call", "workflow_dispatch"} {
			definition, ok := workflow.On[event].(map[string]any)
			Expect(ok).To(BeTrue())
			inputs, ok := definition["inputs"].(map[string]any)
			Expect(ok).To(BeTrue())
			for _, name := range []string{"tag", "revision"} {
				input, ok := inputs[name].(map[string]any)
				Expect(ok).To(BeTrue())
				Expect(input).To(HaveKeyWithValue("type", "string"))
				Expect(input).To(HaveKeyWithValue("required", true))
			}
		}
		Expect(workflow.Jobs["qualify"].Needs).To(Equal("guard"), "native candidate work cannot bypass the trusted identity guard")
		Expect(workflow.Jobs["qualify"].Env).To(HaveKeyWithValue("FACTORY_PUBLISHED_TAG", "${{ needs.guard.outputs.tag }}"))
		Expect(workflow.Jobs["qualify"].Env).To(HaveKeyWithValue("FACTORY_PUBLISHED_REVISION", "${{ needs.guard.outputs.revision }}"))
		Expect(workflow.Permissions).To(Equal(map[string]string{"contents": "read"}))
		matrixJobs := 0
		for _, job := range workflow.Jobs {
			for name, permission := range job.Permissions {
				Expect(name).To(Equal("contents"))
				Expect(permission).To(Equal("read"))
			}
			if len(job.Strategy.Matrix.Include) == 0 {
				continue
			}
			matrixJobs++
			Expect(job.RunsOn).To(Equal("${{ matrix.os }}"))
			Expect(job.Timeout).To(BeNumerically(">", 0))
			Expect(job.Strategy.FailFast).NotTo(BeNil())
			Expect(*job.Strategy.FailFast).To(BeFalse())
			actual := map[string]string{}
			for _, row := range job.Strategy.Matrix.Include {
				Expect(actual).NotTo(HaveKey(row["target"]))
				actual[row["target"]] = row["os"]
			}
			Expect(actual).To(Equal(map[string]string{"linux/amd64": "ubuntu-24.04", "linux/arm64": "ubuntu-24.04-arm", "darwin/amd64": "macos-15-intel", "darwin/arm64": "macos-15"}))
			checkouts := 0
			for _, step := range job.Steps {
				if !strings.HasPrefix(step.Uses, "actions/checkout@") {
					continue
				}
				checkouts++
				Expect(step.With["ref"]).To(BeElementOf("main", "${{ github.event.repository.default_branch }}"), "manual qualification must not execute the candidate/dispatch-ref test harness")
				Expect(step.With["persist-credentials"]).To(BeFalse())
			}
			Expect(checkouts).To(Equal(1))
		}
		Expect(matrixJobs).To(Equal(1))
		release := publishedLoadWorkflow(".github/workflows/runtime-release.yml")
		callers := 0
		for _, job := range release.Jobs {
			if job.Uses != "./.github/workflows/published-release-qualification.yml" {
				continue
			}
			callers++
			switch needs := job.Needs.(type) {
			case string:
				Expect(needs).To(Equal("attest"))
			case []any:
				Expect(needs).To(ContainElement("attest"))
			default:
				Fail("published qualification must depend on successful attestation and asset upload")
			}
			Expect(job.With).To(HaveKeyWithValue("tag", "${{ github.event.release.tag_name }}"))
			Expect(job.With).To(HaveKeyWithValue("revision", "${{ github.sha }}"))
		}
		Expect(callers).To(Equal(1))
	})
	// per docs/adr/0107-published-release-qualification.md:36
	// per docs/adr/0107-published-release-qualification.md:142
	It("executes strict published identity and trusted-event refusal before exposing a candidate", func() {
		workflow := publishedLoadWorkflow(".github/workflows/published-release-qualification.yml")
		script := publishedRunScript(workflow.Jobs["guard"])
		// Presence is a preflight condition, never authentication authority.
		// Require each cross-platform subject and the shared attestation bundle.
		// per docs/adr/0107-published-release-qualification.md:39
		// per docs/adr/0107-published-release-qualification.md:177
		tag, revision := "v9.8.1", strings.Repeat("a", 40)
		assets := []string{"runtime-attestations.jsonl"}
		for _, target := range []string{"linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64"} {
			assets = append(assets, "factory-bootstrap-"+tag+"-"+target, "factory-runtime-"+tag+"-"+target+".tar.gz", "factory-installation-"+tag+"-"+target+".tar.gz")
		}
		scenarios := []string{}
		for _, asset := range assets {
			scenarios = append(scenarios, "missing "+asset)
		}
		scenarios = append(scenarios, "release", "manual", "foreign repository", "manual branch", "PR", "long tag", "moving tag", "malformed revision", "draft", "wrong release tag", "moved tag")
		for _, scenario := range scenarios {
			root := GinkgoT().TempDir()
			trace, output := filepath.Join(root, "gh-calls"), filepath.Join(root, "outputs")
			writeFixture(filepath.Join(root, "tools/gh"), []byte(`#!/bin/sh
set -eu
case " $* " in
  *' --json assets '*)
    printf 'release assets\n' >> "$FIXTURE_TRACE"
    cat "$FIXTURE_ASSETS"
    exit 0 ;;
esac
printf '%s\n' "$1 ${2-}" >> "$FIXTURE_TRACE"
case "$1 ${2-}" in
  'release view') printf '%s\t%s\n' "$FIXTURE_TAG" "$FIXTURE_DRAFT" ;;
  api*) printf '%s\n' "$FIXTURE_REVISION" ;;
  *) exit 93 ;;
esac
`), 0700)
			available := []string{}
			for _, asset := range assets {
				if scenario != "missing "+asset {
					available = append(available, asset)
				}
			}
			assetFile := filepath.Join(root, "published-assets")
			writeFixture(assetFile, []byte(strings.Join(available, "\n")+"\n"), 0600)
			env := map[string]string{"PATH": filepath.Join(root, "tools") + string(os.PathListSeparator) + os.Getenv("PATH"), "GITHUB_REPOSITORY": "anoop2811/software-factory-template", "GITHUB_EVENT_NAME": "release", "GITHUB_REF": "refs/tags/" + tag, "FACTORY_PUBLISHED_TAG": tag, "FACTORY_PUBLISHED_REVISION": revision, "GITHUB_OUTPUT": output, "FIXTURE_TRACE": trace, "FIXTURE_TAG": tag, "FIXTURE_DRAFT": "false", "FIXTURE_REVISION": revision, "FIXTURE_ASSETS": assetFile}
			healthy, beforeNetwork := scenario == "release" || scenario == "manual", false
			switch scenario {
			case "manual":
				env["GITHUB_EVENT_NAME"], env["GITHUB_REF"] = "workflow_dispatch", "refs/heads/main"
			case "foreign repository":
				env["GITHUB_REPOSITORY"] = "foreign/template"
				beforeNetwork = true
			case "manual branch":
				env["GITHUB_EVENT_NAME"], env["GITHUB_REF"] = "workflow_dispatch", "refs/heads/other"
				beforeNetwork = true
			case "PR":
				env["GITHUB_EVENT_NAME"] = "pull_request"
				beforeNetwork = true
			case "long tag":
				env["FACTORY_PUBLISHED_TAG"] = strings.Repeat("x", 129)
				beforeNetwork = true
			case "moving tag":
				env["FACTORY_PUBLISHED_TAG"] = "LATEST"
				beforeNetwork = true
			case "malformed revision":
				env["FACTORY_PUBLISHED_REVISION"] = "$(touch PRIVATE_WITNESS)"
				beforeNetwork = true
			case "draft":
				env["FIXTURE_DRAFT"] = "true"
			case "wrong release tag":
				env["FIXTURE_TAG"] = "v9.8.2"
			case "moved tag":
				env["FIXTURE_REVISION"] = strings.Repeat("b", 40)
			}
			result := publishedRunStep(script, root, env)
			if healthy {
				Expect(result.status).To(Equal(0), "%s: %+v", scenario, result)
				data, err := os.ReadFile(output)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(data)).To(ContainSubstring("tag=" + tag + "\n"))
				Expect(string(data)).To(ContainSubstring("revision=" + revision + "\n"))
				Expect(os.ReadFile(trace)).To(Equal([]byte("release view\napi repos/anoop2811/software-factory-template/commits/refs/tags/" + tag + "\nrelease assets\n")))
			} else {
				Expect(result.status).NotTo(Equal(0), "%s must refuse", scenario)
				_, err := os.Lstat(output)
				Expect(os.IsNotExist(err)).To(BeTrue(), "refusal cannot publish candidate authority")
				if beforeNetwork {
					_, err = os.Lstat(trace)
					Expect(os.IsNotExist(err)).To(BeTrue(), "%s must refuse before gh", scenario)
				}
			}
			_, err := os.Lstat(filepath.Join(root, "PRIVATE_WITNESS"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	})

	// per docs/adr/0107-published-release-qualification.md:51
	// per docs/adr/0107-published-release-qualification.md:79
	// per docs/adr/0107-published-release-qualification.md:123
	It("executes the tagged live suite with literal evidence paths and propagates its failure", func() {
		job := publishedLoadWorkflow(".github/workflows/published-release-qualification.yml").Jobs["qualify"]
		var script string
		for _, step := range job.Steps {
			if strings.Contains(step.Run, "go test") {
				Expect(script).To(BeEmpty())
				script = step.Run
			}
		}
		Expect(script).NotTo(BeEmpty())
		for _, status := range []string{"0", "37"} {
			root := GinkgoT().TempDir()
			reports := filepath.Join(root, `reports "literal" with spaces`)
			writeFixture(filepath.Join(root, "tools/go"), []byte(`#!/bin/sh
printf '%s\000' "$@" > "$FIXTURE_ARGS"
printf '%s\n%s\n%s\n' "$FACTORY_PUBLISHED_TAG" "$FACTORY_PUBLISHED_REVISION" "$FACTORY_PUBLISHED_EVIDENCE_DIR" > "$FIXTURE_ENV"
exit "$FIXTURE_STATUS"
`), 0700)
			env := map[string]string{"PATH": filepath.Join(root, "tools") + string(os.PathListSeparator) + os.Getenv("PATH"), "FACTORY_PUBLISHED_TAG": "v9.8.1", "FACTORY_PUBLISHED_REVISION": strings.Repeat("a", 40), "FACTORY_PUBLISHED_EVIDENCE_DIR": reports, "FACTORY_PUBLISHED_TARGET": "darwin/arm64", "RUNNER_TEMP": root, "FIXTURE_ARGS": filepath.Join(root, "args"), "FIXTURE_ENV": filepath.Join(root, "env"), "FIXTURE_STATUS": status}
			result := publishedRunStep(script, root, env)
			if status == "0" {
				Expect(result.status).To(Equal(0), "%+v", result)
			} else {
				Expect(result.status).NotTo(Equal(0), "failed live suite cannot become qualification success")
			}
			data, err := os.ReadFile(env["FIXTURE_ARGS"])
			Expect(err).NotTo(HaveOccurred())
			args := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
			Expect(args).To(ContainElements("test", "-tags=publishedrelease", "-race", "-count=1", "-timeout=25m", "./acceptance", "-ginkgo.focus=^Go factory command acceptance Published release qualification live ", "-ginkgo.fail-on-empty", "-ginkgo.no-color", "-ginkgo.json-report="+filepath.Join(reports, "ginkgo.json"), "-ginkgo.junit-report="+filepath.Join(reports, "junit.xml")))
			Expect(os.ReadFile(env["FIXTURE_ENV"])).To(Equal([]byte("v9.8.1\n" + strings.Repeat("a", 40) + "\n" + reports + "\n")))
		}
		uploads := 0
		for _, step := range job.Steps {
			if strings.HasPrefix(step.Uses, "actions/upload-artifact@") {
				uploads++
				Expect(step.If).To(Equal("always()"))
				Expect(step.With["if-no-files-found"]).To(Equal("warn"), "a failed compilation can leave no report; the primary runner failure must still propagate")
				Expect(step.With["path"]).NotTo(BeEmpty())
			}
		}
		Expect(uploads).To(Equal(1))
		// Exercise the actual shared command builder: a sparse rollback map
		// inherits the caller's qualified host tools, home/temp and GitHub auth;
		// explicit candidate overrides still replace those values literally.
		// per docs/adr/0107-published-release-qualification.md:84
		// per docs/adr/0107-published-release-qualification.md:104
		root := GinkgoT().TempDir()
		parentTools, poisonTools := filepath.Join(root, "baseline-tools"), filepath.Join(root, "poison-tools")
		for _, toolDirectory := range []string{parentTools, poisonTools} {
			writeFixture(filepath.Join(toolDirectory, "go"), []byte("#!/bin/sh\nexit 97\n"), 0700)
		}
		parentPATH := parentTools + string(os.PathListSeparator) + os.Getenv("PATH")
		GinkgoT().Setenv("PATH", parentPATH)
		GinkgoT().Setenv("GH_TOKEN", "synthetic-parent-read-token")
		GinkgoT().Setenv("GIT_CONFIG_GLOBAL", "parent-config-must-be-removed")
		GinkgoT().Setenv("FACTORY_PUBLISHED_TAG", "parent-tag-must-be-removed")
		child := filepath.Join(root, "child-receipt.sh")
		writeFixture(child, []byte(`#!/bin/sh
set -eu
printf '%s\n' "$PATH" "${HOME-unset}" "${TMPDIR-unset}" "${GH_TOKEN-unset}" "${GIT_CONFIG_GLOBAL-unset}" "${FACTORY_PUBLISHED_TAG-unset}" "$(command -v go)" > "$1"
`), 0700)
		for _, overridden := range []bool{false, true, false} {
			environment := map[string]string{}
			expectedPATH, expectedHOME, expectedTMP, expectedToken := parentPATH, os.Getenv("HOME"), os.Getenv("TMPDIR"), "synthetic-parent-read-token"
			expectedGit, expectedTag, expectedGo := "unset", "unset", filepath.Join(parentTools, "go")
			if _, present := os.LookupEnv("HOME"); !present {
				expectedHOME = "unset"
			}
			if _, present := os.LookupEnv("TMPDIR"); !present {
				expectedTMP = "unset"
			}
			if overridden {
				expectedPATH = poisonTools + string(os.PathListSeparator) + parentPATH
				expectedHOME, expectedTMP, expectedToken = filepath.Join(root, "child-home"), filepath.Join(root, "child-temp"), "synthetic-explicit-read-token" // #nosec G101 -- inert test-only receipt value, never an authentication credential.
				expectedGit, expectedTag, expectedGo = os.DevNull, "explicit-test-tag", filepath.Join(poisonTools, "go")
				environment = map[string]string{"PATH": expectedPATH, "HOME": expectedHOME, "TMPDIR": expectedTMP, "GH_TOKEN": expectedToken, "GIT_CONFIG_GLOBAL": expectedGit, "FACTORY_PUBLISHED_TAG": expectedTag}
			}
			receipt := filepath.Join(root, "actual-child-environment")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			command := handoffCommand(ctx, root, environment, "", "/bin/sh", child, receipt)
			output, err := command.CombinedOutput()
			cancel()
			Expect(err).NotTo(HaveOccurred(), "actual handoff child failed: %s", output)
			Expect(os.ReadFile(receipt)).To(Equal([]byte(strings.Join([]string{expectedPATH, expectedHOME, expectedTMP, expectedToken, expectedGit, expectedTag, expectedGo}, "\n") + "\n")))
		}
		// Offline workflow fixtures must not borrow any ambient CI credentials
		// or context. Only explicitly supplied synthetic values may cross.
		// per docs/adr/0107-published-release-qualification.md:146
		for _, key := range []string{"GH_PARENT_SENTINEL", "GITHUB_PARENT_SENTINEL", "ACTIONS_PARENT_SENTINEL", "FACTORY_PUBLISHED_PARENT_SENTINEL"} {
			GinkgoT().Setenv(key, "synthetic-parent-poison")
		}
		receipt := filepath.Join(root, "isolated-workflow-environment")
		result := publishedRunStep(`printf '%s\n' "${GH_PARENT_SENTINEL-unset}" "${GITHUB_PARENT_SENTINEL-unset}" "${ACTIONS_PARENT_SENTINEL-unset}" "${FACTORY_PUBLISHED_PARENT_SENTINEL-unset}" "$GH_TOKEN" "$GITHUB_REPOSITORY" > "$RECEIPT"`, root, map[string]string{"RECEIPT": receipt, "GH_TOKEN": "synthetic-explicit-workflow-token", "GITHUB_REPOSITORY": "explicit/test-fixture"}) // #nosec G101 -- inert synthetic credential tests environment isolation; no real secret or network request.
		Expect(result.status).To(Equal(0), "%+v", result)
		Expect(os.ReadFile(receipt)).To(Equal([]byte("unset\nunset\nunset\nunset\nsynthetic-explicit-workflow-token\nexplicit/test-fixture\n")))
	})

	// per docs/adr/0107-published-release-qualification.md:79
	It("excludes live release criteria from ordinary offline source compilation", func() {
		for _, tagged := range []bool{false, true} {
			args := []string{"list", "-json"}
			if tagged {
				args = append(args, "-tags=publishedrelease")
			}
			args = append(args, "./acceptance")
			result := imageProcess("..", nil, "go", args...)
			Expect(result.status).To(Equal(0), "%+v", result)
			var data struct{ XTestGoFiles []string }
			Expect(json.Unmarshal([]byte(result.stdout), &data)).To(Succeed())
			if tagged {
				Expect(data.XTestGoFiles).To(ContainElement("published_release_test.go"))
			} else {
				Expect(data.XTestGoFiles).NotTo(ContainElement("published_release_test.go"))
			}
		}
		// Actual Ginkgo discovery must select the five live boundaries, not the
		// similarly named offline workflow contract. Removing all live nodes is
		// an independent negative control for fail-on-empty.
		root := GinkgoT().TempDir()
		args := []string{"test", "-tags=publishedrelease", "-race", "-count=1", "-timeout=90s", "-v", "./acceptance", "-ginkgo.dry-run", "-ginkgo.focus=^Go factory command acceptance Published release qualification live ", "-ginkgo.fail-on-empty", "-ginkgo.no-color"}
		healthy := imageProcess("..", map[string]string{"GOFLAGS": ""}, "go", args...)
		Expect(healthy.status).To(Equal(0), "%+v", healthy)
		Expect(healthy.stdout).To(ContainSubstring("Ran 5 of "))
		Expect(healthy.stdout).To(ContainSubstring("5 Passed | 0 Failed"))
		original, err := filepath.Abs("published_release_test.go")
		Expect(err).NotTo(HaveOccurred())
		empty := filepath.Join(root, "empty_test.go")
		writeFixture(empty, []byte("package acceptance_test\n"), 0600)
		overlay, err := json.Marshal(map[string]any{"Replace": map[string]string{original: empty}})
		Expect(err).NotTo(HaveOccurred())
		overlayPath := filepath.Join(root, "overlay.json")
		writeFixture(overlayPath, overlay, 0600)
		removed := imageProcess("..", map[string]string{"GOFLAGS": "-overlay=" + overlayPath}, "go", args...)
		Expect(removed.status).NotTo(Equal(0), "missing live scenarios must not be replaced by green workflow metaspecs")
		Expect(removed.stdout).To(ContainSubstring("Ran 0 of "))
		// Deliberate unavailable-input refusal, not a live authenticity pass: no
		// network may be needed to establish that explicit selection cannot skip.
		withoutDryRun := []string{}
		for _, arg := range args {
			if arg != "-ginkgo.dry-run" {
				withoutDryRun = append(withoutDryRun, arg)
			}
		}
		missing := imageProcess("..", map[string]string{"GOFLAGS": "", "FACTORY_PUBLISHED_TAG": "", "FACTORY_PUBLISHED_REVISION": "", "FACTORY_PUBLISHED_EVIDENCE_DIR": ""}, "go", withoutDryRun...)
		Expect(missing.status).NotTo(Equal(0))
		Expect(missing.stdout).To(ContainSubstring("explicit live qualification requires a literal FACTORY_PUBLISHED_TAG"))
	})

})
