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
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"
)

const installedReviewInstallerRevision = "3794e6cc8fa66eb02504e753b0ad11955f5cf85a"

type installedReviewFixture struct {
	root, cwd, temporary, client, output, receipt string
	environment                                   map[string]string
}

// The emitted scripts execute unchanged. Only acquisition and GitHub transport
// are recording fixtures; the exported executable is the real compiled CLI.
// per docs/adr/0154-installed-review-workflow-migration.md:97
func installedReviewSetup() installedReviewFixture {
	GinkgoHelper()
	root, cwd := fixtureDirectories()
	temporary := filepath.Join(root, "runner temp ' $(literal)")
	Expect(os.Mkdir(temporary, 0700)).To(Succeed())
	f := installedReviewFixture{root: root, cwd: cwd, temporary: temporary, client: filepath.Join(temporary, "factory-review-client", "factory-review-client"), output: filepath.Join(root, "step-output"), receipt: filepath.Join(root, "bootstrap-receipt.json")}
	f.environment = map[string]string{"PATH": filepath.Join(root, "tools") + ":/usr/bin:/bin", "HOME": root, "TMPDIR": temporary, "RUNNER_TEMP": temporary, "GITHUB_OUTPUT": f.output, "REVIEW_GO_CLIENT": "", "FACTORY_BRIDGE_PROTOCOL": "1", "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull, "BASH_ENV": "", "ENV": "", "IW_ROOT": root, "IW_CLIENT_BYTES": filepath.Join(root, "client-bytes"), "IW_RECEIPT": f.receipt, "IW_BOOTSTRAP_EXIT": "0", "PR": "7", "REPO": "fixture/project", "EXPECTED_BASE": ciReviewBase, "EXPECTED_HEAD": ciReviewHead, "DEFAULT_BRANCH": "main", "MAX_DIFF_BYTES": "200000", "REVIEW_TIMEOUT_SECONDS": "5", "REVIEW_HTTP_RETRIES": "0"}
	writeFixture(f.environment["IW_CLIENT_BYTES"], candidateBinary, 0600)
	writeFixture(f.output, nil, 0600)
	writeFixture(filepath.Join(cwd, ".factory-version"), []byte("ref="+handoffVersion+"\ncommit="+batchBase+"\n"), 0644)
	writeFixture(filepath.Join(cwd, "factory"), []byte("#!/bin/bash\n# Factory installation "+handoffVersion+" "+batchBase+" "+strings.Repeat("a", 64)+"\nprintf launcher > \"$IW_ROOT/forbidden\"\nexit 97\n"), 0700)
	writeFixture(filepath.Join(cwd, "scripts/adversarial-review.sh"), []byte("#!/bin/sh\nprintf legacy > \"$IW_ROOT/forbidden\"\nexit 97\n"), 0700)
	writeFixture(filepath.Join(cwd, "factory.yaml"), []byte("review_lane: on\nmodel_provider: openrouter\nreview_model: fixture/installed-model\nreview_max_tokens: 1024\nreview_reasoning_effort: none\nreview_openrouter_provider: deepinfra\n"), 0600)
	for _, name := range []string{"go", "python", "python3"} {
		writeFixture(filepath.Join(root, "tools", name), []byte("#!/bin/sh\nprintf compiler > \"$IW_ROOT/forbidden\"\nexit 97\n"), 0700)
	}
	jq, err := exec.LookPath("jq")
	Expect(err).NotTo(HaveOccurred())
	Expect(os.Symlink(jq, filepath.Join(root, "tools/jq"))).To(Succeed())
	writeFixture(filepath.Join(root, "tools/curl"), []byte(`#!/bin/sh
set -eu
printf '%s\000' "$@" >> "$IW_ROOT/curl-argv"
destination=''
while [ "$#" -gt 0 ]; do
  case "$1" in --output|-o) shift; destination=$1 ;; esac
  shift
done
[ -n "$destination" ] || exit 91
if [ "${IW_CASE-}" = download-failure ]; then printf partial > "$destination"; exit 22; fi
if [ "${IW_CASE-}" = empty-download ]; then : > "$destination"; exit 0; fi
/bin/cat "$IW_ROOT/installer-fixture" > "$destination"
`), 0700)
	writeFixture(filepath.Join(root, "installer-fixture"), []byte(`#!/bin/sh
set -eu
printf '%s\000' "$@" > "$IW_ROOT/installer-argv"
printf '%s' "${REVIEW_API_KEY-unset}" > "$IW_ROOT/acquisition-provider-key"
output=''
while [ "$#" -gt 0 ]; do
  case "$1" in --output) shift; output=$1 ;; --output=*) output=${1#*=} ;; esac
  shift
done
[ -n "$output" ] || exit 92
mkdir -m 700 "$output"
cp "$IW_CLIENT_BYTES" "$output/factory-review-client"
chmod 700 "$output/factory-review-client"
case "${IW_CASE-}" in
 linked-client) rm "$output/factory-review-client"; ln -s "$IW_CLIENT_BYTES" "$output/factory-review-client" ;;
 missing-client) rm "$output/factory-review-client" ;;
 public-client) chmod 755 "$output/factory-review-client" ;;
 wrong-bytes) printf corrupted > "$output/factory-review-client" ;;
 public-directory) chmod 755 "$output" ;;
esac
cat "$IW_RECEIPT"
exit "$IW_BOOTSTRAP_EXIT"
`), 0600)
	metadata := `{"head":{"sha":"` + ciReviewHead + `","repo":{"full_name":"fixture/project"}},"base":{"sha":"` + ciReviewBase + `","ref":"main","repo":{"full_name":"fixture/project"}}}`
	writeFixture(filepath.Join(root, "metadata.json"), []byte(metadata), 0600)
	writeFixture(filepath.Join(root, "diff"), []byte("diff --git a/example.go b/example.go\n+safe literal $(not-code)\n"), 0600)
	writeFixture(filepath.Join(root, "tools/gh"), []byte(`#!/bin/sh
set -eu
case "$*" in
 *--body-file*) while [ "$1" != --body-file ]; do shift; done; cat "$2" > "$IW_ROOT/comment" ;;
 *Accept:*) cat "$IW_ROOT/diff" ;;
 *) cat "$IW_ROOT/metadata.json" ;;
esac
`), 0700)
	f.writeReceipt(nil)
	return f
}

// per docs/adr/0154-installed-review-workflow-migration.md:42
func (f installedReviewFixture) writeReceipt(change func(map[string]any)) {
	GinkgoHelper()
	value := map[string]any{"schema_version": 1, "version": handoffVersion, "revision": batchBase, "target": runtime.GOOS + "/" + runtime.GOARCH, "sha256": fmt.Sprintf("%x", sha256.Sum256(candidateBinary)), "path": f.client}
	if change != nil {
		change(value)
	}
	data, err := json.Marshal(value)
	Expect(err).NotTo(HaveOccurred())
	writeFixture(f.receipt, append(data, '\n'), 0600)
}

// per docs/adr/0154-installed-review-workflow-migration.md:93
func (f installedReviewFixture) run(step ciReviewStep, extra map[string]string) cliResult {
	GinkgoHelper()
	environment := maps.Clone(f.environment)
	maps.Copy(environment, extra)
	streams, err := os.MkdirTemp("", "factory-installed-review-streams-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(cleanupFixtureDirectory, streams)
	path := filepath.Join(streams, "step.sh")
	writeFixture(path, []byte(step.Run), 0600)
	return cooperativeFixtureProcess(streams, "fixture.installed-review-step", "", []string{f.root, f.cwd, streams}, func(ctx context.Context) *exec.Cmd {
		command := exec.CommandContext(ctx, "/bin/bash", "-e", path) // #nosec G204 -- actual reviewed workflow script with test-owned literal argv and recording transports.
		command.Dir = f.cwd
		for key, value := range environment {
			command.Env = append(command.Env, key+"="+value)
		}
		return command
	})
}

// per docs/adr/0154-installed-review-workflow-migration.md:40
func installedReviewSteps() (ciReviewStep, ciReviewStep, ciReviewStep) {
	GinkgoHelper()
	workflow := ciReviewLoadWorkflow("packs/review-lane/review-pr.yml")
	var selectClient, bootstrap, single ciReviewStep
	for _, step := range workflow.Jobs["review"].Steps {
		switch {
		case step.ID == "client":
			selectClient = step
		case step.ID == "installed-client":
			bootstrap = step
		case strings.Contains(strings.ToLower(step.Name), "review legacy"):
			single = step
		}
	}
	Expect(selectClient.Run).NotTo(BeEmpty())
	Expect(bootstrap.Run).NotTo(BeEmpty(), "generated installed workflow must have the authenticated bootstrap boundary")
	Expect(single.Run).NotTo(BeEmpty())
	Expect(bootstrap.If).To(Equal("steps.client.outputs.installed == 'true'"))
	Expect(single.Env["REVIEW_INSTALLED_CLIENT"]).To(Equal("${{ steps.installed-client.outputs.path }}"))
	Expect(bootstrap.Env).NotTo(HaveKey("REVIEW_API_KEY"))
	return selectClient, bootstrap, single
}

// per docs/adr/0154-installed-review-workflow-migration.md:97
var _ = Describe("Installed review workflow execution", Label("installed-review-workflow"), func() {
	// per docs/adr/0154-installed-review-workflow-migration.md:33
	// per docs/adr/0154-installed-review-workflow-migration.md:40
	It("selects inert installed identity and authenticates a separate runner client before exposing its path", func() {
		var selection ciReviewStep
		for _, step := range ciReviewLoadWorkflow("packs/review-lane/review-pr.yml").Jobs["review"].Steps {
			if step.ID == "client" {
				selection = step
			}
		}
		Expect(selection.Run).NotTo(BeEmpty())
		f := installedReviewSetup()
		before := installationSnapshot(f.cwd)
		selected := f.run(selection, nil)
		Expect(selected.status).To(BeZero(), "%+v", selected)
		Expect(os.ReadFile(f.output)).To(Equal([]byte("mode=legacy\ninstalled=true\n")))
		_, bootstrap, _ := installedReviewSteps()
		writeFixture(f.output, nil, 0600)
		out := f.run(bootstrap, nil)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(os.ReadFile(f.output)).To(Equal([]byte("path=" + f.client + "\n")))
		argv, err := os.ReadFile(filepath.Join(f.root, "installer-argv"))
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Split(strings.TrimSuffix(string(argv), "\x00"), "\x00")).To(Equal([]string{"ci", "review-client", "--go-candidate", handoffVersion, "--expected-revision", batchBase, "--output", filepath.Dir(f.client)}))
		download, err := os.ReadFile(filepath.Join(f.root, "curl-argv"))
		Expect(err).NotTo(HaveOccurred())
		var urls []string
		for _, arg := range strings.Split(string(download), "\x00") {
			if strings.HasPrefix(arg, "https://") || strings.HasPrefix(arg, "http://") {
				urls = append(urls, arg)
			}
		}
		Expect(urls).To(Equal([]string{"https://raw.githubusercontent.com/anoop2811/software-factory-template/" + installedReviewInstallerRevision + "/install.sh"}))
		Expect(string(download)).To(ContainSubstring("--max-time"))
		Expect(os.ReadFile(filepath.Join(f.root, "acquisition-provider-key"))).To(Equal([]byte("unset")))
		Expect(os.ReadFile(f.client)).To(Equal(candidateBinary))
		Expect(installationSnapshot(f.cwd)).To(Equal(before))
		_, err = os.Lstat(filepath.Join(f.root, "forbidden"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0154-installed-review-workflow-migration.md:33
	It("rejects malformed linked incomplete or disagreeing installed identity before any acquisition or fallback", func() {
		selection, bootstrap, _ := installedReviewSteps()
		for _, scenario := range []string{"missing-marker", "linked-marker", "missing-launcher", "linked-launcher", "duplicate-marker", "injection-marker", "uppercase-revision", "moving-tag", "disagreement", "oversized-marker", "malformed-header"} {
			f := installedReviewSetup()
			marker, launcher := filepath.Join(f.cwd, ".factory-version"), filepath.Join(f.cwd, "factory")
			switch scenario {
			case "missing-marker":
				Expect(os.Remove(marker)).To(Succeed())
			case "linked-marker":
				Expect(os.Rename(marker, filepath.Join(f.root, "saved-marker"))).To(Succeed())
				Expect(os.Symlink(filepath.Join(f.root, "saved-marker"), marker)).To(Succeed())
			case "missing-launcher":
				Expect(os.Remove(launcher)).To(Succeed())
				writeFixture(filepath.Join(f.cwd, "scripts/adversarial-review.sh"), []byte("#!/bin/bash\n# Factory installation "+handoffVersion+" "+batchBase+" "+strings.Repeat("a", 64)+"\nexit 97\n"), 0700)
			case "linked-launcher":
				Expect(os.Rename(launcher, filepath.Join(f.root, "saved-launcher"))).To(Succeed())
				Expect(os.Symlink(filepath.Join(f.root, "saved-launcher"), launcher)).To(Succeed())
			case "duplicate-marker":
				writeFixture(marker, []byte("ref="+handoffVersion+"\ncommit="+batchBase+"\nref="+handoffVersion+"\n"), 0644)
			case "injection-marker":
				writeFixture(marker, []byte("ref=$(touch \"$IW_ROOT/forbidden\")\ncommit="+batchBase+"\n"), 0644)
			case "uppercase-revision":
				writeFixture(marker, []byte("ref="+handoffVersion+"\ncommit="+strings.Repeat("A", 40)+"\n"), 0644)
			case "moving-tag":
				writeFixture(marker, []byte("ref=main\ncommit="+batchBase+"\n"), 0644)
			case "disagreement":
				writeFixture(marker, []byte("ref="+handoffVersion+"\ncommit="+batchHead+"\n"), 0644)
			case "oversized-marker":
				writeFixture(marker, []byte(strings.Repeat("#", 1<<20+1)), 0644)
			case "malformed-header":
				writeFixture(launcher, []byte("#!/bin/bash\n# Factory installation invalid\nprintf called > \"$IW_ROOT/forbidden\"\n"), 0700)
			}
			before := installationSnapshot(f.cwd)
			out := f.run(selection, nil)
			if out.status == 0 {
				selected, err := os.ReadFile(f.output)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(selected)).To(ContainSubstring("installed=true"), "declared layout must not silently select legacy: %s", scenario)
				writeFixture(f.output, nil, 0600)
				out = f.run(bootstrap, nil)
			}
			Expect(out.status).NotTo(BeZero(), "%s: %+v", scenario, out)
			for _, name := range []string{"curl-argv", "installer-argv", "forbidden"} {
				_, err := os.Lstat(filepath.Join(f.root, name))
				Expect(os.IsNotExist(err)).To(BeTrue(), "%s: %s", scenario, name)
			}
			Expect(installationSnapshot(f.cwd)).To(Equal(before))
		}
	})

	// per docs/adr/0154-installed-review-workflow-migration.md:42
	It("refuses failed bootstrap and malformed receipt or executable custody without publishing a client output", func() {
		_, bootstrap, _ := installedReviewSteps()
		for _, scenario := range []string{"failed-status", "extra-json", "bad-json", "schema", "version", "revision", "target", "foreign-target", "missing-field", "path", "digest", "linked-client", "missing-client", "public-client", "public-directory", "wrong-bytes", "occupied", "download-failure", "empty-download"} {
			f := installedReviewSetup()
			f.environment["IW_CASE"] = scenario
			switch scenario {
			case "failed-status":
				f.environment["IW_BOOTSTRAP_EXIT"] = "37"
			case "extra-json":
				data, err := os.ReadFile(f.receipt)
				Expect(err).NotTo(HaveOccurred())
				writeFixture(f.receipt, append(data, data...), 0600)
			case "bad-json":
				writeFixture(f.receipt, []byte("not-json\n"), 0600)
			case "schema":
				f.writeReceipt(func(v map[string]any) { v["schema_version"] = 2 })
			case "version":
				f.writeReceipt(func(v map[string]any) { v["version"] = "v9.9.9" })
			case "revision":
				f.writeReceipt(func(v map[string]any) { v["revision"] = batchHead })
			case "target":
				f.writeReceipt(func(v map[string]any) { v["target"] = "windows/amd64" })
			case "foreign-target":
				f.writeReceipt(func(v map[string]any) {
					if runtime.GOOS == "linux" {
						v["target"] = "darwin/arm64"
					} else {
						v["target"] = "linux/amd64"
					}
				})
			case "missing-field":
				f.writeReceipt(func(v map[string]any) { delete(v, "sha256") })
			case "path":
				f.writeReceipt(func(v map[string]any) { v["path"] = filepath.Join(f.root, "different-client") })
			case "digest":
				f.writeReceipt(func(v map[string]any) { v["sha256"] = strings.Repeat("0", 64) })
			case "occupied":
				Expect(os.Mkdir(filepath.Dir(f.client), 0700)).To(Succeed())
				writeFixture(filepath.Join(filepath.Dir(f.client), "foreign"), []byte("keep"), 0600)
			}
			before := installationSnapshot(f.cwd)
			out := f.run(bootstrap, nil)
			Expect(out.status).NotTo(BeZero(), "%s: %+v", scenario, out)
			Expect(os.ReadFile(f.output)).To(BeEmpty(), "%s must not expose an executable path", scenario)
			Expect(installationSnapshot(f.cwd)).To(Equal(before))
			if scenario == "occupied" {
				Expect(os.ReadFile(filepath.Join(filepath.Dir(f.client), "foreign"))).To(Equal([]byte("keep")))
			}
			_, err := os.Lstat(filepath.Join(f.root, "forbidden"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	})

	// per docs/adr/0154-installed-review-workflow-migration.md:17
	// per docs/adr/0154-installed-review-workflow-migration.md:50
	It("runs the bootstrapped single-request client for all three providers without opting into batches", func() {
		_, bootstrap, single := installedReviewSteps()
		for _, provider := range []string{"openrouter", "anthropic", "openai"} {
			f := installedReviewSetup()
			writeFixture(filepath.Join(f.cwd, "factory.yaml"), []byte("review_lane: on\nmodel_provider: "+provider+"\nreview_model: fixture/installed-model\nreview_max_tokens: 1024\n"), 0600)
			service := orchestrationReviewHTTP(f.root, func(w http.ResponseWriter, _ *http.Request, _ int) {
				w.Header().Set("Content-Type", "application/json")
				if provider == "anthropic" {
					_, _ = io.WriteString(w, `{"content":[{"text":"No findings."}]}`)
				} else {
					_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"No findings."}}]}`)
				}
			})
			Expect(f.run(bootstrap, nil).status).To(BeZero())
			Expect(service.observed()).To(BeEmpty())
			before := installationSnapshot(f.cwd)
			out := f.run(single, map[string]string{"REVIEW_INSTALLED_CLIENT": f.client, "REVIEW_GO_CLIENT": "/must-not-select-a-batch-client", "REVIEW_API_KEY": "PRIVATE_INSTALLED_REVIEW_KEY", "HTTPS_PROXY": service.proxy, "SSL_CERT_FILE": service.ca})
			Expect(out.status).To(BeZero(), "%s: %+v", provider, out)
			requests := service.observed()
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].host).To(Equal(map[string]string{"openrouter": "openrouter.ai", "anthropic": "api.anthropic.com", "openai": "api.openai.com"}[provider]))
			var body map[string]any
			Expect(json.Unmarshal(requests[0].body, &body)).To(Succeed())
			Expect(body["model"]).To(Equal("fixture/installed-model"))
			Expect(body).NotTo(HaveKey("stream"))
			comment, err := os.ReadFile(filepath.Join(f.root, "comment"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(comment)).To(ContainSubstring("No findings."))
			Expect(string(comment)).NotTo(ContainSubstring("PRIVATE_"))
			var report map[string]any
			data, err := os.ReadFile(filepath.Join(f.temporary, "factory-review-output/report.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(json.Unmarshal(data, &report)).To(Succeed())
			Expect(report["status"]).To(Equal("complete"))
			Expect(report["batch_coverage_available"]).To(BeFalse())
			Expect(installationSnapshot(f.cwd)).To(Equal(before))
			_, err = os.Lstat(filepath.Join(f.root, "forbidden"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	})

	// per docs/adr/0154-installed-review-workflow-migration.md:50
	It("keeps failed installed model review nonzero and publishes only fixed diagnostics", func() {
		_, bootstrap, single := installedReviewSteps()
		f := installedReviewSetup()
		service := orchestrationReviewHTTP(f.root, func(w http.ResponseWriter, _ *http.Request, _ int) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "PRIVATE_PROVIDER_BODY PRIVATE_INSTALLED_REVIEW_KEY")
		})
		Expect(f.run(bootstrap, nil).status).To(BeZero())
		out := f.run(single, map[string]string{"REVIEW_INSTALLED_CLIENT": f.client, "REVIEW_API_KEY": "PRIVATE_INSTALLED_REVIEW_KEY", "HTTPS_PROXY": service.proxy, "SSL_CERT_FILE": service.ca})
		Expect(out.status).NotTo(BeZero(), "bootstrap success must not imply model completion")
		Expect(service.observed()).To(HaveLen(1))
		comment, err := os.ReadFile(filepath.Join(f.root, "comment"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(comment)).To(ContainSubstring("failed"))
		Expect(string(comment) + out.stdout + out.stderr).NotTo(ContainSubstring("PRIVATE_"))
	})

	// per docs/adr/0154-installed-review-workflow-migration.md:17
	It("preserves explicit external batching and the historical script selection route", func() {
		selection, _, single := installedReviewSteps()
		f := installedReviewSetup()
		external := filepath.Join(f.root, "explicit batch client")
		writeFixture(external, []byte("#!/bin/sh\nprintf called > \"$IW_ROOT/forbidden\"\n"), 0700)
		out := f.run(selection, map[string]string{"REVIEW_GO_CLIENT": external})
		Expect(out.status).To(BeZero())
		Expect(os.ReadFile(f.output)).To(Equal([]byte("mode=go\n")))
		_, err := os.Lstat(filepath.Join(f.root, "forbidden"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(os.Remove(filepath.Join(f.cwd, "factory"))).To(Succeed())
		writeFixture(filepath.Join(f.cwd, "scripts/adversarial-review.sh"), []byte("#!/bin/sh\nprintf 'No historical findings.\\n'\n"), 0700)
		writeFixture(f.output, nil, 0600)
		out = f.run(selection, nil)
		Expect(out.status).To(BeZero())
		Expect(os.ReadFile(f.output)).To(Equal([]byte("mode=legacy\n")))
		out = f.run(single, map[string]string{"REVIEW_INSTALLED_CLIENT": "", "REVIEW_API_KEY": "PRIVATE_LEGACY_KEY"})
		Expect(out.status).To(BeZero(), "%+v", out)
		comment, err := os.ReadFile(filepath.Join(f.root, "comment"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(comment)).To(ContainSubstring("No historical findings."))
		_, err = os.Lstat(filepath.Join(f.root, "curl-argv"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0154-installed-review-workflow-migration.md:104
	It("delivers the installed bootstrap template through explicit lane enable without changing provider settings", func() {
		root, caller, environment := sourceReviewNativeFixture()
		writeFixture(filepath.Join(caller, "factory.yaml"), []byte("review_lane: off\nmodel_provider: anthropic\nreview_model: fixture/kept-model\nreview_max_tokens: 4096\n"), 0600)
		streams, err := os.MkdirTemp("", "factory-review-enable-streams-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupFixtureDirectory, streams)
		out := cooperativeFixtureProcess(streams, "fixture.review-enable", "", []string{root, caller, streams}, func(ctx context.Context) *exec.Cmd {
			command := handoffCommand(ctx, caller, environment, "", filepath.Join(root, "review-runtime"), "review-lane", "enable", "FIXTURE_REVIEW_KEY")
			return command
		})
		Expect(out.status).To(BeZero(), "%+v", out)
		workflow, err := os.ReadFile(filepath.Join(caller, ".github/workflows/adversarial-review.yml"))
		Expect(err).NotTo(HaveOccurred())
		template, err := os.ReadFile("../packs/review-lane/review-pr.yml")
		Expect(err).NotTo(HaveOccurred())
		Expect(workflow).To(Equal([]byte(reviewLaneHeader + "\n" + strings.ReplaceAll(string(template), "__REVIEW_API_KEY_SECRET__", "FIXTURE_REVIEW_KEY"))))
		Expect(string(workflow)).To(ContainSubstring(installedReviewInstallerRevision))
		configuration, err := os.ReadFile(filepath.Join(caller, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		var settings map[string]any
		Expect(yaml.Unmarshal(configuration, &settings)).To(Succeed())
		Expect(settings).To(HaveKeyWithValue("model_provider", "anthropic"))
		Expect(settings).To(HaveKeyWithValue("review_model", "fixture/kept-model"))
		Expect(fmt.Sprint(settings["review_max_tokens"])).To(Equal("4096"))
	})
})
