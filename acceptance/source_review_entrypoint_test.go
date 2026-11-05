package acceptance_test

import (
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"
)

// Current source bytes are materialized without using the operator's binary.
// per docs/adr/0135-go-source-review-entrypoints.md:18
func sourceReviewAdapter(root, name string) string {
	GinkgoHelper()
	body, err := os.ReadFile(filepath.Join("../scripts", name))
	Expect(err).NotTo(HaveOccurred())
	path := filepath.Join(root, "scripts", name)
	writeFixture(path, body, 0755)
	return path
}

// The recording owner delegates to a real runtime beside its actual assets.
// per docs/adr/0135-go-source-review-entrypoints.md:31
func sourceReviewNativeFixture() (string, string, map[string]string) {
	GinkgoHelper()
	root, caller := fixtureDirectories()
	base := filepath.Dir(root)
	for _, name := range []string{"adversarial-review.sh", "factory-review-lane.sh"} {
		sourceReviewAdapter(root, name)
	}
	template, err := os.ReadFile("../packs/review-lane/review-pr.yml")
	Expect(err).NotTo(HaveOccurred())
	writeFixture(filepath.Join(root, "packs/review-lane/review-pr.yml"), template, 0600)
	writeFixture(filepath.Join(root, "review-runtime"), candidateBinary, 0700)
	writeFixture(filepath.Join(root, "factory"), []byte("#!/bin/sh\nprintf '%s\\000' \"$@\" >> \"$REVIEW_OWNER_CALLS\"\nexec \"$REVIEW_NATIVE\" \"$@\"\n"), 0700)
	writeFixture(filepath.Join(caller, "factory.yaml"), []byte("model_provider: openrouter\nreview_model: fixture/model\nreview_lane: off\n"), 0600)
	return root, caller, map[string]string{"PATH": "/usr/bin:/bin", "HOME": caller, "TMPDIR": base, "BASH_ENV": "", "ENV": "", "NO_COLOR": "1", "GORACE": "atexit_sleep_ms=0", "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull, "GIT_OPTIONAL_LOCKS": "0", "FACTORY_CONFIG": filepath.Join(caller, "factory.yaml"), "REVIEW_NATIVE": filepath.Join(root, "review-runtime"), "REVIEW_OWNER_CALLS": filepath.Join(base, "owner calls")}
}

// The fixed production endpoint is intercepted by the existing private TLS proxy.
// per docs/adr/0135-go-source-review-entrypoints.md:28
func sourceReviewHTTPEnvironment(environment map[string]string, service *orchestrationReviewService) {
	environment["HTTPS_PROXY"], environment["SSL_CERT_FILE"] = service.proxy, service.ca
	environment["CURL_CA_BUNDLE"] = service.ca
	environment["REVIEW_API_KEY"] = "PRIVATE_REVIEW_KEY"
	environment["REVIEW_TIMEOUT_SECONDS"], environment["REVIEW_HTTP_RETRIES"] = "3", "0"
}

// Private homes explicitly reuse only the evaluator's selected Go caches.
// per docs/adr/0135-go-source-review-entrypoints.md:70
func sourceReviewGoCaches() map[string]string {
	GinkgoHelper()
	compiler, err := exec.LookPath("go")
	Expect(err).NotTo(HaveOccurred())
	compiler, err = filepath.EvalSymlinks(compiler)
	Expect(err).NotTo(HaveOccurred())
	query := exec.Command(compiler, "env", "-json", "GOCACHE", "GOMODCACHE") // #nosec G204 -- evaluator-selected compiler with fixed read-only cache query.
	query.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	body, err := query.Output()
	Expect(err).NotTo(HaveOccurred())
	var caches map[string]string
	Expect(json.Unmarshal(body, &caches)).To(Succeed())
	for _, key := range []string{"GOCACHE", "GOMODCACHE"} {
		Expect(filepath.IsAbs(caches[key])).To(BeTrue())
	}
	return caches
}

// Setup provisions every locked module; behavioral downloads use only this copy.
// per docs/adr/0135-go-source-review-entrypoints.md:92
func sourceReviewModuleProxy(cache, temporary string, environment map[string]string) string {
	GinkgoHelper()
	root := filepath.Join(temporary, "proxy source module")
	locked := make(map[string][]byte)
	for _, path := range []string{"go.mod", "go.sum"} {
		body, err := os.ReadFile(filepath.Join("..", path))
		Expect(err).NotTo(HaveOccurred())
		locked[path] = body
		writeFixture(filepath.Join(root, path), body, 0600)
	}
	staging := maps.Clone(environment)
	maps.Copy(staging, map[string]string{"GOMODCACHE": filepath.Join(temporary, "proxy staging modules"), "GOPROXY": (&url.URL{Scheme: "file", Path: filepath.Join(cache, "cache/download")}).String(), "GOPRIVATE": "", "GONOPROXY": "", "GONOSUMDB": "", "GOSUMDB": "off", "GOENV": "off", "GOWORK": "off", "GOTOOLCHAIN": "local"})
	Expect(staging["GOMODCACHE"]).NotTo(Equal(environment["GOMODCACHE"]))
	compiler, err := exec.LookPath("go")
	Expect(err).NotTo(HaveOccurred())
	compiler, err = filepath.EvalSymlinks(compiler)
	Expect(err).NotTo(HaveOccurred())
	prepared := handoffNativeExactProcess(root, staging, "", compiler, "mod", "download")
	Expect(prepared.status).To(BeZero(), "fixture setup requires the complete locked module cache; run go mod download before qualification: %+v", prepared)
	for path, body := range locked {
		Expect(os.ReadFile(filepath.Join(root, path))).To(Equal(body), "proxy preparation must preserve %s", path)
	}
	return (&url.URL{Scheme: "file", Path: filepath.Join(staging["GOMODCACHE"], "cache/download")}).String()
}

// Run the extracted Actions script with its unoverridden Unix default shell.
// per docs/adr/0135-go-source-review-entrypoints.md:39
func sourceReviewWorkflowRun(root string, environment map[string]string, body string) cliResult {
	GinkgoHelper()
	script := filepath.Join(environment["TMPDIR"], "actual workflow step")
	writeFixture(script, []byte(body), 0600)
	return handoffNativeExactProcess(root, environment, "", "/bin/bash", "-e", script)
}

var _ = Describe("Go source runtime entrypoints review adapters", func() {
	// per docs/adr/0135-go-source-review-entrypoints.md:18
	DescribeTable("preserves literal channels and physical ownership through both invocation forms", func(script, command string) {
		for _, bash := range []bool{false, true} {
			for _, alias := range []bool{false, true} {
				root, caller := fixtureDirectories()
				base := filepath.Dir(root)
				owner := filepath.Join(base, "review owner ' $(literal) \n")
				Expect(os.Rename(root, owner)).To(Succeed())
				program := sourceReviewAdapter(owner, script)
				record, input, decoy := filepath.Join(base, "argv"), filepath.Join(base, "stdin"), filepath.Join(base, "decoy")
				writeFixture(filepath.Join(owner, "factory"), []byte("#!/bin/sh\nprintf '%s\\000' \"$PWD\" \"$REVIEW_LITERAL\" \"$@\" > \"$REVIEW_ARGV\"\n/bin/cat > \"$REVIEW_STDIN\"\nprintf 'owner stdout\\n'\nprintf 'owner stderr\\n' >&2\nexit 63\n"), 0700)
				foreign, tools := filepath.Join(base, "foreign"), filepath.Join(base, "tools")
				for _, path := range []string{filepath.Join(caller, "factory"), filepath.Join(foreign, "factory"), filepath.Join(tools, "factory")} {
					writeFixture(path, []byte("#!/bin/sh\nprintf called > \"$REVIEW_DECOY\"\nexit 64\n"), 0700)
				}
				if alias {
					Expect(os.Symlink(filepath.Join(owner, "scripts"), filepath.Join(foreign, "scripts"))).To(Succeed())
					program = filepath.Join(foreign, "scripts", script)
				}
				environment := map[string]string{"PATH": tools + ":/usr/bin:/bin", "BASH_ENV": "", "ENV": "", "REVIEW_ARGV": record, "REVIEW_STDIN": input, "REVIEW_DECOY": decoy, "REVIEW_LITERAL": "literal $(not-code)\nvalue\n", "FACTORY_RUNTIME_BINARY": filepath.Join(foreign, "factory"), "FACTORY_HOME": foreign}
				args := []string{"diff with ' quote", "", "literal $(not-code)\nline", "--"}
				stdin := "borrowed stdin\x00literal $(not-code)\n"
				beforeOwner, beforeCaller := installationSnapshot(owner), installationSnapshot(caller)
				observed := sourceStructuralInvoke(caller, environment, stdin, program, bash, args...)
				Expect(observed).To(Equal(cliResult{"owner stdout\n", "owner stderr\n", 63}), "%s bash=%t directory_alias=%t", script, bash, alias)
				raw, err := os.ReadFile(record)
				Expect(err).NotTo(HaveOccurred())
				Expect(strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")).To(Equal(append([]string{caller, environment["REVIEW_LITERAL"], command}, args...)))
				Expect(os.ReadFile(input)).To(Equal([]byte(stdin)))
				_, err = os.Lstat(decoy)
				Expect(os.IsNotExist(err)).To(BeTrue())
				Expect(installationSnapshot(owner)).To(Equal(beforeOwner))
				Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
			}
		}
	}, Entry("review execution", "adversarial-review.sh", "review"), Entry("lane administration", "factory-review-lane.sh", "review-lane"))

	// per docs/adr/0135-go-source-review-entrypoints.md:21
	It("refuses missing or aliased owners and preserves owner failure without fallback", func() {
		for _, name := range []string{"adversarial-review.sh", "factory-review-lane.sh"} {
			for _, mode := range []string{"failed", "missing", "non-executable", "file-alias"} {
				for _, bash := range []bool{false, true} {
					owner, caller := fixtureDirectories()
					base := filepath.Dir(owner)
					program := sourceReviewAdapter(owner, name)
					ownerCommand := filepath.Join(owner, "factory")
					if mode != "missing" {
						writeFixture(ownerCommand, []byte("#!/bin/sh\nprintf 'owner stdout\\n'\nprintf 'owner stderr\\n' >&2\nexit 47\n"), 0700)
					}
					if mode == "non-executable" {
						Expect(os.Chmod(ownerCommand, 0600)).To(Succeed())
					}
					if mode == "file-alias" {
						alias := filepath.Join(caller, "aliased review")
						Expect(os.Symlink(program, alias)).To(Succeed())
						program = alias
					}
					foreign, tools, decoy := filepath.Join(base, "foreign"), filepath.Join(base, "tools"), filepath.Join(base, "decoy called")
					for _, path := range []string{filepath.Join(caller, "factory"), filepath.Join(foreign, "factory"), filepath.Join(tools, "factory")} {
						writeFixture(path, []byte("#!/bin/sh\nprintf '%s\\n' \"$0\" >> \"$REVIEW_DECOY\"\nprintf '%s\\n' \"$REVIEW_REFUSED_OWNER\" >&2\nexit 87\n"), 0700)
					}
					environment := map[string]string{"PATH": tools + ":/usr/bin:/bin", "BASH_ENV": "", "ENV": "", "REVIEW_DECOY": decoy, "REVIEW_REFUSED_OWNER": ownerCommand, "FACTORY_RUNTIME_BINARY": filepath.Join(foreign, "factory"), "FACTORY_HOME": foreign}
					beforeOwner, beforeCaller := installationSnapshot(owner), installationSnapshot(caller)
					observed := sourceStructuralInvoke(caller, environment, "", program, bash, "status")
					_, err := os.Lstat(decoy)
					Expect(os.IsNotExist(err)).To(BeTrue(), "%s mode=%s bash=%t must not select a fallback: %+v", name, mode, bash, observed)
					switch mode {
					case "failed":
						Expect(observed).To(Equal(cliResult{"owner stdout\n", "owner stderr\n", 47}))
					case "missing", "non-executable":
						reference := handoffNativeExactProcess(caller, environment, "", "/bin/bash", "-c", "set -e; exec \"$@\"", "absolute exec reference", ownerCommand, "status")
						Expect(reference.status).NotTo(BeZero())
						Expect(observed.status).To(Equal(reference.status))
						Expect(observed.stdout).To(BeEmpty())
						Expect(observed.stderr).To(ContainSubstring(ownerCommand))
					case "file-alias":
						Expect(observed.status).To(Equal(2))
						Expect(observed.stdout).To(BeEmpty())
						Expect(observed.stderr).To(ContainSubstring("file symlink"))
					}
					Expect(installationSnapshot(owner)).To(Equal(beforeOwner))
					Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
				}
			}
		}
	})

	// per docs/adr/0135-go-source-review-entrypoints.md:25
	// per docs/adr/0135-go-source-review-entrypoints.md:65
	It("compiles its source owner and carries real private HTTP review and refusal outcomes", func() {
		source := newSourceEntrypointFixture()
		environment := maps.Clone(source.environment)
		environment["HOME"], environment["TMPDIR"] = source.caller, filepath.Dir(source.root)
		environment["FACTORY_CONFIG"] = filepath.Join(source.caller, "factory.yaml")
		writeFixture(environment["FACTORY_CONFIG"], []byte("model_provider: openrouter\nreview_model: fixture/model\n"), 0600)
		maps.Copy(environment, sourceReviewGoCaches())
		environment["GOENV"] = "off"
		service := orchestrationReviewHTTP(source.caller, func(w http.ResponseWriter, _ *http.Request, count int) {
			w.Header().Set("Content-Type", "application/json")
			if count > 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, "PRIVATE_PROVIDER_BODY")
				return
			}
			_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"No findings."}}]}`)
		})
		sourceReviewHTTPEnvironment(environment, service)
		diff := "diff --git a/example.go b/example.go\n+literal $(not-code) ' quote\n"
		diffPath := filepath.Join(source.caller, "private diff.patch")
		writeFixture(diffPath, []byte(diff), 0600)
		program := sourceReviewAdapter(source.root, "adversarial-review.sh")
		before := sourceEntrypointSnapshot(source.root)
		observed := sourceStructuralInvoke(source.caller, environment, "", program, false, diffPath, "ignored-extra")
		Expect(observed).To(Equal(cliResult{"No findings.\n", "", 0}))
		Expect(sourceEntrypointSnapshot(source.root)).To(Equal(before))
		compiled, err := os.ReadFile(filepath.Join(source.root, "factory-go"))
		Expect(err).NotTo(HaveOccurred(), "the source adapter must compile its real owner")
		Expect(compiled).NotTo(BeEmpty())
		writeFixture(filepath.Join(source.root, "factory"), compiled, 0700)
		requests := service.observed()
		Expect(requests).To(HaveLen(1))
		Expect(requests[0].host).To(Equal("openrouter.ai"))
		Expect(requests[0].path).To(Equal("/api/v1/chat/completions"))
		Expect(requests[0].method).To(Equal(http.MethodPost))
		Expect(requests[0].header.Get("Authorization")).To(Equal("Bearer PRIVATE_REVIEW_KEY"))
		var request map[string]any
		Expect(json.Unmarshal(requests[0].body, &request)).To(Succeed())
		Expect(request["model"]).To(Equal("fixture/model"))
		Expect(request["messages"]).To(Equal([]any{map[string]any{"role": "system", "content": orchestrationSystemPrompt}, map[string]any{"role": "user", "content": "Review this diff.\n\n```diff\n" + strings.TrimRight(diff, "\n") + "\n```"}}))
		failed := sourceStructuralInvoke(source.caller, environment, "", program, true, diffPath)
		Expect(failed.status).To(Equal(1))
		Expect(failed.stdout).To(BeEmpty())
		Expect(failed.stderr).To(ContainSubstring("HTTP request failed"))
		Expect(failed.stderr).NotTo(ContainSubstring("PRIVATE_"))
		for _, bash := range []bool{false, true} {
			for _, args := range [][]string{nil, {""}, {filepath.Join(source.caller, "absent.patch")}} {
				Expect(sourceStructuralInvoke(source.caller, environment, "", program, bash, args...)).To(Equal(cliResult{"", "adversarial-review: usage: adversarial-review.sh <diff-file>\n", 1}))
			}
		}
		writeFixture(diffPath, []byte(" \n\t\n"), 0600)
		withoutKey := maps.Clone(environment)
		delete(withoutKey, "REVIEW_API_KEY")
		missingKey := sourceStructuralInvoke(source.caller, withoutKey, "", program, false, diffPath)
		Expect(missingKey.status).To(Equal(1))
		Expect(missingKey.stdout).To(BeEmpty())
		Expect(missingKey.stderr).To(ContainSubstring("no REVIEW_API_KEY in the environment"))
		Expect(sourceStructuralInvoke(source.caller, environment, "", program, true, diffPath)).To(Equal(cliResult{"_No reviewable change in this diff._\n", "", 0}))
		Expect(service.observed()).To(HaveLen(2), "refusals and empty input must not make model requests")
	})

	// per docs/adr/0135-go-source-review-entrypoints.md:31
	It("runs the real lane lifecycle against caller configuration and preserves foreign workflows", func() {
		root, caller, environment := sourceReviewNativeFixture()
		program := filepath.Join(root, "scripts/factory-review-lane.sh")
		imageGit(caller, "init", "-q")
		nested := filepath.Join(caller, "nested")
		Expect(os.Mkdir(nested, 0700)).To(Succeed())
		status := sourceStructuralInvoke(nested, environment, "", program, false)
		Expect(status.status).To(BeZero(), "%+v", status)
		Expect(status.stdout).To(ContainSubstring("review lane: off\n"))
		enabled := sourceStructuralInvoke(nested, environment, "", program, true, "enable", "FIXTURE_REVIEW_KEY")
		Expect(enabled.status).To(BeZero(), "%+v", enabled)
		Expect(enabled.stdout).To(ContainSubstring("review lane: enabled."))
		template, err := os.ReadFile(filepath.Join(root, "packs/review-lane/review-pr.yml"))
		Expect(err).NotTo(HaveOccurred())
		workflow := filepath.Join(caller, ".github/workflows/adversarial-review.yml")
		Expect(os.ReadFile(workflow)).To(Equal([]byte(reviewLaneHeader + "\n" + strings.ReplaceAll(string(template), "__REVIEW_API_KEY_SECRET__", "FIXTURE_REVIEW_KEY"))))
		status = sourceStructuralInvoke(nested, environment, "", program, false, "status")
		Expect(status.status).To(BeZero(), "%+v", status)
		Expect(status.stdout).To(And(ContainSubstring("review lane: on\n"), ContainSubstring("FIXTURE_REVIEW_KEY"), ContainSubstring("workflow:  installed")))
		disabled := sourceStructuralInvoke(nested, environment, "", program, true, "disable")
		Expect(disabled.status).To(BeZero(), "%+v", disabled)
		_, err = os.Lstat(workflow)
		Expect(os.IsNotExist(err)).To(BeTrue())
		configuration, err := os.ReadFile(environment["FACTORY_CONFIG"])
		Expect(err).NotTo(HaveOccurred())
		Expect(string(configuration)).To(ContainSubstring("review_lane: \"off\""))
		for _, symlink := range []bool{false, true} {
			outside := filepath.Join(filepath.Dir(root), "foreign workflow")
			writeFixture(outside, []byte("USER_WORKFLOW"), 0600)
			if symlink {
				Expect(os.Remove(workflow)).To(Succeed())
				Expect(os.Symlink(outside, workflow)).To(Succeed())
			} else {
				writeFixture(workflow, []byte("USER_WORKFLOW"), 0600)
			}
			before := installationSnapshot(caller)
			refused := sourceStructuralInvoke(nested, environment, "", program, symlink, "enable", "FIXTURE_REVIEW_KEY")
			Expect(refused.status).To(Equal(1))
			Expect(refused.stdout).To(BeEmpty())
			Expect(installationSnapshot(caller)).To(Equal(before))
			Expect(os.ReadFile(outside)).To(Equal([]byte("USER_WORKFLOW")))
		}
		calls, err := os.ReadFile(environment["REVIEW_OWNER_CALLS"])
		Expect(err).NotTo(HaveOccurred())
		Expect(string(calls)).To(ContainSubstring("review-lane\x00"))
	})

	// per docs/adr/0135-go-source-review-entrypoints.md:31
	It("routes pending and unsupported lane commands without hidden GitHub calls", func() {
		root, caller, environment := sourceReviewNativeFixture()
		tools := filepath.Join(filepath.Dir(root), "tools")
		calls := filepath.Join(filepath.Dir(root), "gh calls")
		writeFixture(filepath.Join(tools, "gh"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$REVIEW_GH_CALLS\"\nprintf 'FIXTURE_REVIEW_KEY yesterday\\n'\n"), 0700)
		environment["PATH"], environment["REVIEW_GH_CALLS"] = tools+":"+environment["PATH"], calls
		program := filepath.Join(root, "scripts/factory-review-lane.sh")
		Expect(sourceStructuralInvoke(caller, environment, "", program, false, "pending")).To(Equal(cliResult{"", "", 0}))
		_, err := os.Lstat(calls)
		Expect(os.IsNotExist(err)).To(BeTrue())
		writeFixture(environment["FACTORY_CONFIG"], []byte("review_lane: on\nreview_api_key_secret: FIXTURE_REVIEW_KEY\n"), 0600)
		Expect(sourceStructuralInvoke(caller, environment, "", program, true, "pending")).To(Equal(cliResult{"", "", 0}))
		Expect(os.ReadFile(calls)).To(Equal([]byte("secret list\n")))
		Expect(sourceStructuralInvoke(caller, environment, "", program, false, "secret-name")).To(Equal(cliResult{"FIXTURE_REVIEW_KEY\n", "", 0}))
		before := installationSnapshot(caller)
		Expect(sourceStructuralInvoke(caller, environment, "", program, true, "--help")).To(Equal(cliResult{"", "usage: factory review-lane [status|enable|disable|pending|secret-name]\n", 2}))
		Expect(installationSnapshot(caller)).To(Equal(before))
		Expect(os.ReadFile(calls)).To(Equal([]byte("secret list\n")))
	})

	// per docs/adr/0135-go-source-review-entrypoints.md:78
	// per docs/adr/0135-go-source-review-entrypoints.md:87
	It("prepares only canonical source before credentials and stops on acquisition or lockfile failure", func() {
		workflow := ciReviewLoadWorkflow("packs/review-lane/review-pr.yml")
		steps := workflow.Jobs["review"].Steps
		selectionIndex, setupIndex, prepareIndex, credentialsIndex := -1, -1, -1, len(steps)
		for index, step := range steps {
			if step.ID == "client" {
				selectionIndex = index
			}
			if strings.HasPrefix(step.Uses, "actions/setup-go@") {
				Expect(setupIndex).To(Equal(-1))
				setupIndex = index
			}
			if strings.Contains(step.Run, "go mod download") {
				Expect(prepareIndex).To(Equal(-1))
				prepareIndex = index
			}
			credentials := []string{step.Run}
			for _, value := range step.Env {
				credentials = append(credentials, value)
			}
			for _, value := range step.With {
				if literal, ok := value.(string); ok {
					credentials = append(credentials, literal)
				}
			}
			for _, value := range credentials {
				if strings.Contains(value, "secrets.") && index < credentialsIndex {
					credentialsIndex = index
				}
			}
		}
		Expect(selectionIndex).To(BeNumerically(">=", 0))
		Expect(setupIndex).To(BeNumerically(">", selectionIndex))
		Expect(prepareIndex).To(BeNumerically(">", setupIndex))
		Expect(credentialsIndex).To(BeNumerically("<", len(steps)))
		Expect(prepareIndex).To(BeNumerically("<", credentialsIndex))
		for _, scope := range []map[string]string{workflow.Env, workflow.Jobs["review"].Env} {
			for _, value := range scope {
				Expect(value).NotTo(ContainSubstring("secrets."))
			}
		}
		setup, preparation := steps[setupIndex], steps[prepareIndex]
		Expect(setup.Uses).To(Equal("actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e"))
		Expect(setup.With).To(HaveKeyWithValue("go-version-file", "go.mod"))
		Expect(setup.With).To(HaveKeyWithValue("cache", false))
		Expect(setup.With).To(HaveKeyWithValue("token", ""))
		Expect(setup.If).To(Equal("steps.client.outputs.source == 'true'"))
		Expect(preparation.If).To(Equal(setup.If))
		Expect(preparation.Env).NotTo(HaveKey("REVIEW_API_KEY"))
		Expect(preparation.Env).NotTo(HaveKey("GITHUB_TOKEN"))
		Expect(preparation.Env).NotTo(HaveKey("GH_TOKEN"))

		source := newSourceEntrypointFixture()
		base := filepath.Dir(source.root)
		environment := maps.Clone(source.environment)
		maps.Copy(environment, map[string]string{"HOME": source.caller, "TMPDIR": base, "GITHUB_OUTPUT": filepath.Join(base, "selected mode"), "REVIEW_GO_CLIENT": ""})
		selectionTools := filepath.Join(base, "selection tools")
		selectionGoCalled := filepath.Join(base, "selection called Go")
		writeFixture(filepath.Join(selectionTools, "go"), []byte("#!/bin/sh\nprintf called > \"$REVIEW_SELECTION_GO_CALLED\"\nexit 86\n"), 0700)
		environment["PATH"], environment["REVIEW_SELECTION_GO_CALLED"] = selectionTools+":"+source.environment["PATH"], selectionGoCalled
		selectMode := func(root, client string) string {
			environment["REVIEW_GO_CLIENT"] = client
			writeFixture(environment["GITHUB_OUTPUT"], nil, 0600)
			Expect(sourceReviewWorkflowRun(root, environment, steps[selectionIndex].Run)).To(Equal(cliResult{"", "", 0}))
			body, err := os.ReadFile(environment["GITHUB_OUTPUT"])
			Expect(err).NotTo(HaveOccurred())
			return string(body)
		}
		Expect(selectMode(source.root, "")).To(Equal("mode=legacy\nsource=true\n"))
		modulePath := filepath.Join(source.root, "go.mod")
		module, err := os.ReadFile(modulePath)
		Expect(err).NotTo(HaveOccurred())
		writeFixture(modulePath, []byte("module example.com/ordinary-adopter\n\ngo 1.27.3\n"), 0600)
		Expect(selectMode(source.root, "")).To(Equal("mode=legacy\n"), "similarly named source files do not make an adopter module the factory")
		writeFixture(modulePath, module, 0600)
		bootstrap := filepath.Join(source.root, "internal/sourcebootstrap/publish.go")
		Expect(os.Rename(bootstrap, bootstrap+".held")).To(Succeed())
		Expect(selectMode(source.root, "")).To(Equal("mode=legacy\n"))
		Expect(os.Rename(bootstrap+".held", bootstrap)).To(Succeed())
		installed := filepath.Join(base, "installed project")
		writeFixture(filepath.Join(installed, "factory"), candidateBinary, 0700)
		Expect(selectMode(installed, "")).To(Equal("mode=legacy\n"), "an installed runtime does not need source preparation")
		clientCalled := filepath.Join(base, "explicit client called")
		client := filepath.Join(base, "selected client")
		writeFixture(client, []byte("#!/bin/sh\nprintf called > \"$REVIEW_CLIENT_CALLED\"\n"), 0700)
		environment["REVIEW_CLIENT_CALLED"] = clientCalled
		Expect(selectMode(source.root, client)).To(Equal("mode=go\n"), "explicit clients bypass source preparation even in a canonical checkout")
		_, err = os.Lstat(clientCalled)
		Expect(os.IsNotExist(err)).To(BeTrue())
		_, err = os.Lstat(selectionGoCalled)
		Expect(os.IsNotExist(err)).To(BeTrue(), "classification must not execute Go for any project or client selection")

		tools := filepath.Join(base, "preparation tools")
		goCalls, factoryCalled := filepath.Join(base, "go calls"), filepath.Join(base, "factory called")
		writeFixture(filepath.Join(tools, "go"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" \"${REVIEW_API_KEY-unset}\" \"${GITHUB_TOKEN-unset}\" \"${GH_TOKEN-unset}\" > \"$REVIEW_PREP_GO_CALLS\"\nif [ \"$REVIEW_PREP_FAILURE\" = download ]; then exit 79; fi\nprintf '\\n' >> go.sum\n"), 0700)
		writeFixture(filepath.Join(source.root, "factory"), []byte("#!/bin/sh\nprintf called > \"$REVIEW_PREP_FACTORY_CALLED\"\n"), 0700)
		maps.Copy(environment, map[string]string{"PATH": tools + ":" + source.environment["PATH"], "REVIEW_PREP_GO_CALLS": goCalls, "REVIEW_PREP_FACTORY_CALLED": factoryCalled, "REVIEW_GO_CLIENT": ""})
		sum, err := os.ReadFile(filepath.Join(source.root, "go.sum"))
		Expect(err).NotTo(HaveOccurred())
		for _, failure := range []string{"download", "changed-lockfile"} {
			environment["REVIEW_PREP_FAILURE"] = failure
			result := sourceReviewWorkflowRun(source.root, environment, preparation.Run)
			Expect(result.status).NotTo(BeZero(), failure)
			Expect(os.ReadFile(goCalls)).To(Equal([]byte("mod download\nunset\nunset\nunset\n")))
			_, err = os.Lstat(factoryCalled)
			Expect(os.IsNotExist(err)).To(BeTrue(), "failed preparation must stop before starting the source launcher")
			writeFixture(filepath.Join(source.root, "go.sum"), sum, 0600)
		}
	})

	// per docs/adr/0135-go-source-review-entrypoints.md:78
	It("executes the shipped direct review workflow step through the real owner and private services", func() {
		source := newSourceEntrypointFixture()
		root, temporary := source.root, source.caller
		environment := maps.Clone(source.environment)
		caches := sourceReviewGoCaches()
		maps.Copy(environment, map[string]string{"HOME": temporary, "TMPDIR": temporary, "GOENV": "off", "GOCACHE": caches["GOCACHE"], "GOMODCACHE": filepath.Join(temporary, "empty modules"), "GOFLAGS": "-modcacherw", "FACTORY_CONFIG": filepath.Join(temporary, "factory.yaml")})
		environment["GOPROXY"] = sourceReviewModuleProxy(caches["GOMODCACHE"], temporary, environment)
		writeFixture(environment["FACTORY_CONFIG"], []byte("model_provider: openrouter\nreview_model: fixture/model\n"), 0600)
		sourceReviewAdapter(root, "adversarial-review.sh")
		for _, path := range []string{"go.mod", "go.sum"} {
			body, err := os.ReadFile(filepath.Join("..", path))
			Expect(err).NotTo(HaveOccurred())
			writeFixture(filepath.Join(root, path), body, 0600)
		}
		imageGit(root, "add", "go.mod", "go.sum")
		workflow := ciReviewLoadWorkflow("packs/review-lane/review-pr.yml")
		var selected, preparation []ciReviewStep
		var selection ciReviewStep
		for _, step := range workflow.Jobs["review"].Steps {
			if strings.Contains(step.Run, "./scripts/adversarial-review.sh") {
				selected = append(selected, step)
			}
			if step.ID == "client" {
				selection = step
			}
			if strings.Contains(step.Run, "go mod download") {
				preparation = append(preparation, step)
			}
		}
		Expect(preparation).To(HaveLen(1))
		Expect(selected).To(HaveLen(1))
		Expect(selected[0].If).To(Equal("steps.client.outputs.mode == 'legacy'"))
		Expect(selected[0].Env["REVIEW_GO_CLIENT"]).To(BeEmpty())
		var shellPolicy struct {
			Defaults map[string]map[string]any `yaml:"defaults"`
			Jobs     map[string]struct {
				Defaults map[string]map[string]any `yaml:"defaults"`
				Steps    []struct {
					Name  string  `yaml:"name"`
					Run   string  `yaml:"run"`
					Shell *string `yaml:"shell"`
				} `yaml:"steps"`
			} `yaml:"jobs"`
		}
		template, err := os.ReadFile("../packs/review-lane/review-pr.yml")
		Expect(err).NotTo(HaveOccurred())
		Expect(yaml.Unmarshal(template, &shellPolicy)).To(Succeed())
		Expect(shellPolicy.Defaults["run"]).NotTo(HaveKey("shell"))
		Expect(shellPolicy.Jobs["review"].Defaults["run"]).NotTo(HaveKey("shell"))
		for _, step := range shellPolicy.Jobs["review"].Steps {
			if step.Name == selected[0].Name || step.Name == selection.Name || step.Run == preparation[0].Run {
				Expect(step.Shell).To(BeNil())
			}
		}
		service := orchestrationReviewHTTP(temporary, func(w http.ResponseWriter, _ *http.Request, count int) {
			w.Header().Set("Content-Type", "application/json")
			if count > 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, "PRIVATE_PROVIDER_BODY PRIVATE_REVIEW_KEY")
				return
			}
			_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"No findings."}}]}`)
		})
		environment["GITHUB_OUTPUT"] = filepath.Join(temporary, "selection output")
		environment["REVIEW_GO_CLIENT"] = ""
		Expect(sourceReviewWorkflowRun(root, environment, selection.Run)).To(Equal(cliResult{"", "", 0}))
		for _, step := range preparation {
			prepared := sourceReviewWorkflowRun(root, environment, step.Run)
			Expect(prepared.status).To(BeZero(), "%+v", prepared)
		}
		Expect(service.observed()).To(BeEmpty(), "preparation has no review credentials or provider work")
		sourceReviewHTTPEnvironment(environment, service)
		metadata := `{"head":{"sha":"` + ciReviewHead + `","repo":{"full_name":"fixture/project"}},"base":{"sha":"` + ciReviewBase + `","ref":"main","repo":{"full_name":"fixture/project"}}}`
		writeFixture(filepath.Join(temporary, "metadata.json"), []byte(metadata), 0600)
		tools := filepath.Join(temporary, "tools")
		writeFixture(filepath.Join(tools, "gh"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$REVIEW_GH_CALLS\"\ncase \"$*\" in\n*--body-file*) while [ \"$1\" != --body-file ]; do shift; done; /bin/cat \"$2\" > \"$CAPTURE_COMMENT\" ;;\n*Accept:*) printf 'diff --git a/x b/x\\n+private workflow diff\\n' ;;\n*) /bin/cat \"$FIXTURE_METADATA\" ;;\nesac\n"), 0700)
		ambient := filepath.Join(temporary, "ambient tools")
		ambientCalled := filepath.Join(temporary, "ambient gh called")
		writeFixture(filepath.Join(ambient, "gh"), []byte("#!/bin/sh\nprintf called > \"$REVIEW_AMBIENT_GH_CALLED\"\nprintf 'unexpected ambient GitHub client\\n' >&2\nexit 84\n"), 0700)
		environment["REVIEW_AMBIENT_GH_CALLED"] = ambientCalled
		source.environment["PATH"] = ambient + ":" + source.environment["PATH"]
		jq, err := exec.LookPath("jq")
		Expect(err).NotTo(HaveOccurred())
		jq, err = filepath.EvalSymlinks(jq)
		Expect(err).NotTo(HaveOccurred())
		// The substitute directory contains only gh; the selected compiler still precedes jq's tools.
		environment["PATH"] = tools + ":" + source.environment["PATH"] + ":" + filepath.Dir(jq)
		maps.Copy(environment, map[string]string{"PR": "7", "REPO": "fixture/project", "EXPECTED_BASE": ciReviewBase, "EXPECTED_HEAD": ciReviewHead, "DEFAULT_BRANCH": "main", "RUNNER_TEMP": temporary, "MAX_DIFF_BYTES": "200000", "REVIEW_GO_CLIENT": "", "FIXTURE_METADATA": filepath.Join(temporary, "metadata.json"), "CAPTURE_COMMENT": filepath.Join(temporary, "posted comment"), "REVIEW_GH_CALLS": filepath.Join(temporary, "gh calls")})
		observed := sourceReviewWorkflowRun(root, environment, selected[0].Run)
		diagnostic, _ := os.ReadFile(filepath.Join(temporary, "factory-review-legacy.err"))
		AddReportEntry("source-review.cold-workflow", map[string]any{"status": observed.status, "provider_requests": len(service.observed()), "review_error": string(diagnostic)}, ReportEntryVisibilityNever)
		Expect(observed).To(Equal(cliResult{"", "", 0}))
		Expect(service.observed()).To(HaveLen(1))
		Expect(os.ReadFile(environment["GITHUB_OUTPUT"])).To(ContainSubstring("source=true\n"))
		compiled, err := os.ReadFile(filepath.Join(root, "factory-go"))
		Expect(err).NotTo(HaveOccurred())
		Expect(compiled).NotTo(BeEmpty())
		var report map[string]any
		body, err := os.ReadFile(filepath.Join(temporary, "factory-review-output/report.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(json.Unmarshal(body, &report)).To(Succeed())
		Expect(report).To(Equal(map[string]any{"version": float64(1), "mode": "legacy-single-request", "status": "complete", "repository": "fixture/project", "base": ciReviewBase, "head": ciReviewHead, "batch_coverage_available": false}))
		comment, err := os.ReadFile(environment["CAPTURE_COMMENT"])
		Expect(err).NotTo(HaveOccurred())
		Expect(string(comment)).To(And(ContainSubstring("Compatibility adversarial review (advisory): complete"), ContainSubstring(ciReviewHead), ContainSubstring("No findings.")))
		Expect(string(comment)).NotTo(ContainSubstring("PRIVATE_REVIEW_KEY"))
		calls, err := os.ReadFile(environment["REVIEW_GH_CALLS"])
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Split(strings.TrimSpace(string(calls)), "\n")).To(Equal([]string{"api repos/fixture/project/pulls/7", "api repos/fixture/project/pulls/7 -H Accept: application/vnd.github.v3.diff", "api repos/fixture/project/pulls/7", "pr comment 7 --repo fixture/project --body-file " + filepath.Join(temporary, "factory-review-comment.md")}))
		failedTemp := filepath.Join(temporary, "failed review")
		Expect(os.Mkdir(failedTemp, 0700)).To(Succeed())
		environment["RUNNER_TEMP"] = failedTemp
		failed := sourceReviewWorkflowRun(root, environment, selected[0].Run)
		Expect(failed.status).To(Equal(1))
		Expect(failed.stdout).To(BeEmpty())
		Expect(service.observed()).To(HaveLen(2), "one request per review without hidden retries or fallback")
		comment, err = os.ReadFile(environment["CAPTURE_COMMENT"])
		Expect(err).NotTo(HaveOccurred())
		Expect(string(comment)).To(And(ContainSubstring("Compatibility adversarial review (advisory): failed"), ContainSubstring("adversarial-review: HTTP request failed")))
		Expect(string(comment)).NotTo(ContainSubstring("PRIVATE_"))
		body, err = os.ReadFile(filepath.Join(failedTemp, "factory-review-output/report.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(json.Unmarshal(body, &report)).To(Succeed())
		Expect(report["status"]).To(Equal("failed"))
		_, err = os.Lstat(ambientCalled)
		Expect(os.IsNotExist(err)).To(BeTrue(), "the private GitHub substitute must precede every ambient gh")
	})
})
