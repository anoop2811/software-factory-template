package acceptance_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// per docs/adr/0153-authenticated-review-client-bootstrap.md:24
var _ = Describe("Whole installation upgrade release candidate handoff CI checkout preparation", SpecPriority(0), Label("handoff-ci-review-client-surface"), func() {
	// per docs/adr/0153-authenticated-review-client-bootstrap.md:31
	// per docs/adr/0153-authenticated-review-client-bootstrap.md:38
	It("owns review-client native and installer help and strict selectors before acquisition", func() {
		f := ordinaryBootstrapSentinel(handoffVersion, "")
		ciBootstrapPoison(f)
		f.environment["PATH"] = filepath.Join(f.root, "no-tools")
		Expect(os.Mkdir(f.environment["PATH"], 0700)).To(Succeed())
		driver := filepath.Join(f.root, "native-parser")
		writeFixture(driver, candidateBinary, 0700)
		nativeEnv := map[string]string{}
		for key, value := range f.environment {
			nativeEnv[key] = value
		}
		nativeEnv["FACTORY_BRIDGE_PROTOCOL"] = ""
		output := filepath.Join(f.root, "private review $(literal); output")
		valid := reviewClientArguments(handoffVersion, batchBase, output)
		type request struct {
			args   []string
			status int
		}
		requests := []request{{[]string{"ci", "review-client", "--help"}, 0}, {[]string{"ci", "review-client", "-h"}, 0}, {append(append([]string{}, valid...), "--help"), 0}, {[]string{"ci", "review-client"}, 2}}
		requests = append(requests, request{[]string{"ci", "review-client", "--output=" + f.cwd, "--help"}, 0})
		for _, tag := range []string{"v0.0.0", "v1.2.3-rc.1+build.001", "v" + strings.Repeat("9", 80) + ".0.0"} {
			requests = append(requests, request{[]string{"ci", "review-client", "--go-candidate=" + tag, "--expected-revision=" + batchBase, "--output=" + output, "--help"}, 0})
		}
		for _, tag := range []string{"", "main", "latest", "V1.2.3", "v1.2", "v01.2.3", "v1.2.3-01", "v1.2.3+", "v1.2.3+é", "v1.2.3+" + strings.Repeat("a", 122)} {
			requests = append(requests, request{[]string{"ci", "review-client", "--go-candidate=" + tag, "--help"}, 2})
		}
		for _, revision := range []string{"", "main", strings.Repeat("a", 39), strings.Repeat("b", 41), strings.Repeat("A", 40), strings.Repeat("g", 40), " " + batchBase} {
			requests = append(requests, request{[]string{"ci", "review-client", "--expected-revision=" + revision, "--help"}, 2})
		}
		for _, path := range []string{"", "relative", "/", f.root + "/../escaped", f.root + "/./unclean", f.root + "//duplicate", output + "/"} {
			requests = append(requests, request{[]string{"ci", "review-client", "--output=" + path, "--help"}, 2})
		}
		for _, extra := range [][]string{{"--help", "-h"}, {"--help=x"}, {"--go-candidate", handoffVersion}, {"--expected-revision=" + batchBase}, {"--output", output}, {"--origin-target=linux/amd64"}, {"--ephemeral"}, {"--"}, {"operand"}, {"--source", "/unused"}, {"--ref", "v0.1.6"}, {"--local"}, {"--unknown"}} {
			requests = append(requests, request{append(append([]string{}, valid...), extra...), 2})
		}
		for _, args := range [][]string{{"ci", "review-client", "--go-candidate"}, {"ci", "review-client", "--expected-revision"}, {"ci", "review-client", "--output"}, valid[:4], valid[:6], {"ci", "review-client", "--expected-revision", batchBase, "--output", output}} {
			requests = append(requests, request{args, 2})
		}
		before := installationSnapshot(f.cwd)
		for _, testcase := range requests {
			native := imageProcess(f.cwd, nativeEnv, driver, testcase.args...)
			Expect(native.status).To(Equal(testcase.status), "native argv %q: %+v", testcase.args, native)
			if testcase.status == 0 {
				Expect(native.stdout).To(ContainSubstring("review-client"))
			}
			for _, shell := range bootstrapWatchdogInterpreters() {
				out := imageProcess(f.cwd, f.environment, shell, append([]string{f.installer}, testcase.args...)...)
				Expect(out.status).To(Equal(testcase.status), "%s argv %q: %+v", shell, testcase.args, out)
				if testcase.status == 0 {
					Expect(out.stdout).To(ContainSubstring("review-client"))
					Expect(out.stdout).To(ContainSubstring("--expected-revision"))
					Expect(out.stderr).To(BeEmpty())
				}
			}
			ordinaryBootstrapNoTrace(f)
			Expect(installationSnapshot(f.cwd)).To(Equal(before))
			ciBootstrapCacheUnchanged(f)
			_, err := os.Lstat(output)
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	})

	// per docs/adr/0153-authenticated-review-client-bootstrap.md:49
	// per docs/adr/0153-authenticated-review-client-bootstrap.md:59
	It("hands authenticated review-client literal argv streams and selected-owner status through the installer", func() {
		for _, shell := range bootstrapWatchdogInterpreters() {
			for _, equals := range []bool{false, true} {
				f := ordinaryBootstrapSentinel(handoffVersion, "")
				ciBootstrapPoison(f)
				f.environment["REVIEW_BOOTSTRAP_LITERAL"] = "literal ' \" $(not-code)\nsecond line"
				raw := strings.Replace(string(f.raw), "exit 0\n", `pwd -P > "$HANDOFF_RECORD_DIR/native-cwd"
/bin/cat > "$HANDOFF_RECORD_DIR/native-stdin"
printf '%s' "$REVIEW_BOOTSTRAP_LITERAL" > "$HANDOFF_RECORD_DIR/native-environment"
printf '%s' "${FACTORY_BRIDGE_PROTOCOL-}" > "$HANDOFF_RECORD_DIR/native-bridge"
printf 'selected review owner output\n'
printf 'selected review owner failure\n' >&2
exit 37
`, 1)
				writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+handoffVersion+"-"+f.target), []byte(raw), 0600)
				output := filepath.Join(f.root, "client ' $(literal); output")
				args := reviewClientArguments(handoffVersion, batchBase, output)
				if equals {
					args = []string{"ci", "review-client", "--output=" + output, "--expected-revision=" + batchBase, "--go-candidate=" + handoffVersion}
				}
				input := "literal stdin\nwith \"quotes\"\n"
				out := handoffNativeProcess(f.cwd, f.environment, input, shell, append([]string{f.installer}, args...)...)
				Expect(out.status).To(Equal(37), "%+v", out)
				Expect(out.stdout).To(Equal("selected review owner output\n"))
				Expect(out.stderr).To(Equal("selected review owner failure\n"))
				Expect(handoffRecorded(f, "bootstrap-argv")).To(Equal(args))
				for name, expected := range map[string]string{"native-cwd": f.cwd + "\n", "native-stdin": input, "native-environment": f.environment["REVIEW_BOOTSTRAP_LITERAL"], "native-bridge": ""} {
					Expect(os.ReadFile(filepath.Join(f.records, name))).To(Equal([]byte(expected)), name)
				}
				verify := handoffRecorded(f, "verify")
				Expect(handoffFlag(verify, "--repo")).To(Equal("anoop2811/software-factory-template"))
				Expect(handoffFlag(verify, "--source-ref")).To(Equal("refs/tags/" + handoffVersion))
				Expect(verify[2]).To(HaveSuffix("factory-bootstrap-" + handoffVersion + "-" + f.target))
				ciBootstrapCacheUnchanged(f)
				ordinaryBootstrapCache(f)
			}
		}
	})

	// per docs/adr/0153-authenticated-review-client-bootstrap.md:49
	It("rejects a moved review-client tag before download chmod or first execution", func() {
		f := ordinaryBootstrapSentinel(handoffVersion, "")
		ciBootstrapPoison(f)
		output := filepath.Join(f.root, "never-published")
		out := imageProcess(f.cwd, f.environment, "/bin/sh", append([]string{f.installer}, reviewClientArguments(handoffVersion, batchHead, output)...)...)
		Expect(out.status).NotTo(BeZero(), "%+v", out)
		Expect(out.status).NotTo(Equal(2), "valid selectors must reach revision verification: %+v", out)
		Expect(strings.ToLower(out.stderr)).To(ContainSubstring("revision"))
		trace, err := os.ReadFile(filepath.Join(f.records, "trace"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(trace)).To(ContainSubstring("api\n"))
		for _, operation := range []string{"release-download\n", "verify\n", "bootstrap\n"} {
			Expect(string(trace)).NotTo(ContainSubstring(operation))
		}
		for _, path := range []string{output, filepath.Join(f.records, "candidate-executed"), filepath.Join(f.records, "early-chmod"), filepath.Join(f.records, "forbidden-tool")} {
			_, err := os.Lstat(path)
			Expect(os.IsNotExist(err)).To(BeTrue(), path)
		}
		Expect(out.stdout).To(BeEmpty())
		ordinaryBootstrapCache(f)
		ciBootstrapCacheUnchanged(f)
	})

	// per docs/adr/0153-authenticated-review-client-bootstrap.md:49
	// per docs/adr/0153-authenticated-review-client-bootstrap.md:92
	It("refuses unauthenticated review-client installer selections without execution or publication", func() {
		for _, scenario := range []string{"download-fail", "roots-fail", "authentication-fail", "wrong-subject", "malformed-verification"} {
			f := ordinaryBootstrapSentinel(handoffVersion, scenario)
			ciBootstrapPoison(f)
			output := filepath.Join(f.root, "never-published")
			before := installationSnapshot(f.cwd)
			out := imageProcess(f.cwd, f.environment, "/bin/sh", append([]string{f.installer}, reviewClientArguments(handoffVersion, batchBase, output)...)...)
			Expect(out.status).NotTo(BeZero(), "%s: %+v", scenario, out)
			Expect(out.status).NotTo(Equal(2), "valid selectors must reach acquisition: %+v", out)
			Expect(out.stdout).To(BeEmpty())
			trace, err := os.ReadFile(filepath.Join(f.records, "trace"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(trace)).To(ContainSubstring("release-download\n"))
			Expect(string(trace)).NotTo(ContainSubstring("bootstrap\n"))
			for _, path := range []string{output, filepath.Join(f.records, "candidate-executed"), filepath.Join(f.records, "early-chmod"), filepath.Join(f.records, "forbidden-tool")} {
				_, err := os.Lstat(path)
				Expect(os.IsNotExist(err)).To(BeTrue(), path)
			}
			Expect(installationSnapshot(f.cwd)).To(Equal(before))
			ciBootstrapCacheUnchanged(f)
			ordinaryBootstrapCache(f)
		}
	})
})

// per docs/adr/0153-authenticated-review-client-bootstrap.md:105
var _ = Describe("Whole installation upgrade release candidate handoff CI checkout preparation", Ordered, ContinueOnFailure, SpecPriority(2), Label("handoff-ci-review-client-project"), func() {
	var image handoffReleaseImage
	BeforeAll(func() { image = handoffBuildImage() })

	// per docs/adr/0153-authenticated-review-client-bootstrap.md:79
	// per docs/adr/0153-authenticated-review-client-bootstrap.md:113
	It("exports authenticated native review clients and runs both standalone review routes without project tools", func() {
		for _, installer := range []bool{false, true} {
			f, driver := handoffNativeFixture(image)
			imageGit(f.cwd, "init", "-q")
			writeFixture(filepath.Join(f.cwd, "application.txt"), []byte("application must remain unchanged\n"), 0600)
			writeFixture(filepath.Join(f.cwd, "factory.yaml"), []byte("review_lane: off\nmodel_provider: openrouter\nreview_model: fixture/configured-model\nreview_max_tokens: 1024\nreview_reasoning_effort: none\nreview_openrouter_provider: deepinfra\n"), 0600)
			writeFixture(filepath.Join(f.cwd, "factory"), []byte("#!/bin/sh\nprintf forbidden > project-script-executed\nexit 97\n"), 0700)
			imageGit(f.cwd, "add", "application.txt", "factory.yaml", "factory")
			Expect(os.Remove(filepath.Join(f.root, "host tools", "git"))).To(Succeed())
			writeFixture(filepath.Join(f.root, "host tools", "git"), []byte("#!/bin/sh\nprintf git > \"$HANDOFF_RECORD_DIR/forbidden-tool\"\nexit 97\n"), 0700)
			input := batchAdded([]string{"application.go"}, 1)
			diff, manifest := batchWriteInput(f.cwd, input)
			service := reviewFixture(f.root, func(w http.ResponseWriter, _ *http.Request, count int) {
				if count == 1 {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"Safe findings"}}]}`)
				} else {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, reviewComplete)
				}
			})
			before := installationSnapshot(f.cwd)
			output := filepath.Join(f.root, "review client ' $(literal); output")
			args := reviewClientArguments(handoffVersion, image.revision, output)
			if installer {
				driver, args = "/bin/sh", append([]string{f.installer}, args...)
				f.environment["FACTORY_BRIDGE_PROTOCOL"] = "inherited-private-bridge-must-clear"
			} else {
				Expect(candidateBinary).NotTo(Equal(image.binary), "direct caller must exercise handoff to the authenticated selected owner")
				writeFixture(driver, candidateBinary, 0700)
			}
			f.environment["REVIEW_API_KEY"] = ""
			f.environment["HTTPS_PROXY"], f.environment["SSL_CERT_FILE"] = service.proxy, service.ca
			out := reviewClientCooperativeProcess(f, f.environment, nil, driver, args...)
			Expect(out.status).To(BeZero(), "installer=%t: %+v", installer, out)
			client := reviewClientReceipt(out.stdout, output, image)
			AddReportEntry("review-client.export", map[string]any{"installer": installer, "receipt": out.stdout, "status": out.status}, ReportEntryVisibilityNever)
			Expect(service.observed()).To(BeEmpty(), "bootstrap must not contact a review provider")
			Expect(installationSnapshot(f.cwd)).To(Equal(before))
			ordinaryBootstrapCache(f)
			reviewTools := filepath.Join(f.root, "standalone review tools")
			for _, name := range []string{"go", "python", "python3"} {
				poison, err := os.ReadFile(filepath.Join(f.root, "host tools", name))
				Expect(err).NotTo(HaveOccurred())
				writeFixture(filepath.Join(reviewTools, name), poison, 0700)
			}
			reviewEnv := map[string]string{"PATH": reviewTools, "HOME": f.environment["HOME"], "HANDOFF_RECORD_DIR": f.records, "HTTPS_PROXY": service.proxy, "SSL_CERT_FILE": service.ca, "REVIEW_API_KEY": "PRIVATE_EXPORTED_REVIEW_KEY", "REVIEW_TIMEOUT_SECONDS": "5", "REVIEW_HTTP_RETRIES": "0"}
			single := reviewClientCooperativeProcess(f, reviewEnv, nil, client, "review", diff)
			Expect(single.status).To(BeZero(), "%+v", single)
			Expect(single.stdout).To(Equal("Safe findings\n"))
			batchOutput := filepath.Join(f.root, "batch report")
			batch := reviewClientCooperativeProcess(f, reviewEnv, nil, client, "review-batch", "--diff", diff, "--files", manifest, "--base", batchBase, "--head", batchHead, "--repository", "fixture/project", "--output", batchOutput)
			Expect(batch.status).To(BeZero(), "%+v", batch)
			Expect(batchCounts(batchReadReport(batchOutput))["batches_completed"]).To(Equal(float64(1)))
			requests := service.observed()
			Expect(requests).To(HaveLen(2))
			for _, request := range requests {
				Expect(request.auth).To(Equal("Bearer PRIVATE_EXPORTED_REVIEW_KEY"))
				var body map[string]any
				Expect(json.Unmarshal(request.body, &body)).To(Succeed())
				Expect(body["model"]).To(Equal("fixture/configured-model"))
				Expect(body["max_tokens"]).To(Equal(float64(1024)))
				Expect(body["provider"]).To(Equal(map[string]any{"order": []any{"deepinfra"}, "allow_fallbacks": false, "require_parameters": true}))
			}
			Expect(installationSnapshot(f.cwd)).To(Equal(before))
			_, err := os.Lstat(filepath.Join(f.records, "forbidden-tool"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	})

	// per docs/adr/0153-authenticated-review-client-bootstrap.md:56
	// per docs/adr/0153-authenticated-review-client-bootstrap.md:117
	It("refuses review-client native revision authentication and raw image disagreement without publication", func() {
		for _, scenario := range []string{"revision", "authentication-fail", "wrong-archive-subject", "raw-disagreement"} {
			f, driver := handoffNativeFixture(image)
			revision := image.revision
			switch scenario {
			case "revision":
				revision = batchBase
			case "raw-disagreement":
				writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+handoffVersion+"-"+f.target), append(append([]byte{}, image.binary...), []byte("unapproved trailing bytes")...), 0600)
			default:
				f.environment["HANDOFF_SCENARIO"] = scenario
			}
			output := filepath.Join(f.root, "never-published")
			before := installationSnapshot(f.cwd)
			out := reviewClientCooperativeProcess(f, f.environment, nil, driver, reviewClientArguments(handoffVersion, revision, output)...)
			Expect(out.status).NotTo(BeZero(), "%s: %+v", scenario, out)
			Expect(out.status).NotTo(Equal(2), "valid selectors must reach native acquisition")
			Expect(out.stdout).To(BeEmpty())
			_, err := os.Lstat(output)
			Expect(os.IsNotExist(err)).To(BeTrue(), scenario)
			Expect(installationSnapshot(f.cwd)).To(Equal(before))
			ordinaryBootstrapCache(f)
		}
	})

	// per docs/adr/0153-authenticated-review-client-bootstrap.md:31
	// per docs/adr/0153-authenticated-review-client-bootstrap.md:92
	It("preserves foreign review-client destinations and rejects linked missing unsafe or project-local output", func() {
		f, driver := handoffNativeFixture(image)
		parent := filepath.Join(f.root, "physical-parent")
		Expect(os.Mkdir(parent, 0700)).To(Succeed())
		writeFixture(filepath.Join(parent, "canary"), []byte("foreign canary\n"), 0600)
		occupied := filepath.Join(parent, "occupied")
		Expect(os.Mkdir(occupied, 0700)).To(Succeed())
		writeFixture(filepath.Join(occupied, "keep"), []byte("foreign occupant\n"), 0600)
		linkedParent := filepath.Join(f.root, "linked-parent")
		Expect(os.Symlink(parent, linkedParent)).To(Succeed())
		linkedLeaf := filepath.Join(parent, "linked-leaf")
		Expect(os.Symlink(filepath.Join(parent, "missing-target"), linkedLeaf)).To(Succeed())
		unsafe := filepath.Join(f.root, "unsafe-parent")
		Expect(os.Mkdir(unsafe, 0700)).To(Succeed())
		Expect(os.Chmod(unsafe, 0777)).To(Succeed())
		outputs := []string{occupied, filepath.Join(parent, "canary"), filepath.Join(linkedParent, "new"), linkedLeaf, filepath.Join(parent, "missing", "new"), filepath.Join(unsafe, "new"), filepath.Join(f.cwd, "private-output")}
		before, projectBefore := installationSnapshot(parent), installationSnapshot(f.cwd)
		for _, output := range outputs {
			out := reviewClientCooperativeProcess(f, f.environment, nil, driver, reviewClientArguments(handoffVersion, image.revision, output)...)
			Expect(out.status).NotTo(BeZero(), "%q: %+v", output, out)
			Expect(out.stdout).To(BeEmpty())
			Expect(installationSnapshot(parent)).To(Equal(before), output)
			Expect(installationSnapshot(f.cwd)).To(Equal(projectBefore), output)
		}
		_, err := os.Lstat(filepath.Join(parent, "missing"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "missing parents must not be created")
		ordinaryBootstrapCache(f)
	})

	// per docs/adr/0153-authenticated-review-client-bootstrap.md:59
	// per docs/adr/0153-authenticated-review-client-bootstrap.md:122
	It("joins acknowledged review-client acquisition cancellation without disturbing unrelated processes", func() {
		f, _ := handoffNativeFixture(image)
		ready := filepath.Join(f.records, "review-verifier-ready")
		gh := filepath.Join(f.root, "host tools", "gh")
		data, err := os.ReadFile(gh)
		Expect(err).NotTo(HaveOccurred())
		old := "  verify)\n"
		Expect(strings.Count(string(data), old)).To(Equal(1))
		body := `  verify)
    if [ "${3##*/}" = archive ]; then
      /bin/sleep 120 &
      printf '%s %s\n' "$$" "$!" > "$CI_PREPARE_CANCEL_READY"
      wait
    fi
`
		writeFixture(gh, []byte(strings.Replace(string(data), old, body, 1)), 0700)
		f.environment["CI_PREPARE_CANCEL_READY"] = ready
		output := filepath.Join(f.root, "canceled-client")
		before := installationSnapshot(f.cwd)
		outsider := exec.Command("/bin/sleep", "30")
		outsider.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		Expect(outsider.Start()).To(Succeed())
		defer func() { _ = outsider.Process.Kill(); _ = outsider.Wait() }()
		args := append([]string{f.installer}, reviewClientArguments(handoffVersion, image.revision, output)...)
		ciCanceledArgumentsProcess(f, "/bin/sh", f.cwd, ready, "acquisition", args)
		Expect(outsider.Process.Signal(syscall.Signal(0))).To(Succeed())
		exited, observationErr := installationProcessExited(outsider.Process.Pid)
		Expect(observationErr).NotTo(HaveOccurred())
		Expect(exited).To(BeFalse(), "unrelated process must remain live, not merely present as an unreaped zombie")
		Expect(installationSnapshot(f.cwd)).To(Equal(before))
		_, err = os.Lstat(output)
		Expect(os.IsNotExist(err)).To(BeTrue())
		ordinaryBootstrapCache(f)
	})

	// per docs/adr/0153-authenticated-review-client-bootstrap.md:92
	It("reports failed review-client receipt output while retaining only a complete private executable", func() {
		f, driver := handoffNativeFixture(image)
		output := filepath.Join(f.root, "client-with-failed-receipt")
		readOnly, err := os.Open(f.installer)
		Expect(err).NotTo(HaveOccurred())
		defer readOnly.Close()
		out := reviewClientCooperativeProcess(f, f.environment, readOnly, driver, reviewClientArguments(handoffVersion, image.revision, output)...)
		Expect(out.status).To(BeNumerically(">", 0), "%+v", out)
		Expect(strings.ToLower(out.stderr)).To(ContainSubstring("receipt"))
		client := filepath.Join(output, "factory-review-client")
		Expect(os.ReadFile(client)).To(Equal(image.binary), "output failure follows complete authenticated publication")
		info, err := os.Stat(client)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0700)))
		second := reviewClientCooperativeProcess(f, f.environment, nil, driver, reviewClientArguments(handoffVersion, image.revision, output)...)
		Expect(second.status).NotTo(BeZero(), "a failed-receipt output directory is never silently reused")
		Expect(second.stdout).To(BeEmpty())
		ordinaryBootstrapCache(f)
	})
})

// Actual image journeys preserve exact caller environments and bounded descendant
// ownership even when the independent watchdog or receipt descriptor fails.
// per docs/adr/0153-authenticated-review-client-bootstrap.md:105
func reviewClientCooperativeProcess(f handoffBootstrapFixture, environment map[string]string, stdout *os.File, program string, args ...string) cliResult {
	GinkgoHelper()
	streams, err := os.MkdirTemp("", "factory-cooperative-review-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(cleanupFixtureDirectory, streams)
	return cooperativeFixtureProcessContextWithOutput(context.Background(), streams, "fixture.review-client-owner", "", []string{f.root, streams}, stdout, func(ctx context.Context) *exec.Cmd {
		command := handoffCommand(ctx, f.cwd, environment, "", program, args...)
		command.Env = make([]string, 0, len(environment))
		for key, value := range environment {
			command.Env = append(command.Env, key+"="+value)
		}
		return command
	})
}

// per docs/adr/0153-authenticated-review-client-bootstrap.md:79
func reviewClientReceipt(stdout, output string, image handoffReleaseImage) string {
	GinkgoHelper()
	var receipt map[string]any
	decoder := json.NewDecoder(strings.NewReader(stdout))
	Expect(decoder.Decode(&receipt)).To(Succeed())
	var extra any
	Expect(decoder.Decode(&extra)).To(MatchError(io.EOF), "stdout contains exactly one JSON receipt")
	path := filepath.Join(output, "factory-review-client")
	Expect(receipt).To(Equal(map[string]any{"schema_version": float64(1), "version": handoffVersion, "revision": image.revision, "target": runtime.GOOS + "/" + runtime.GOARCH, "sha256": fmt.Sprintf("%x", sha256.Sum256(image.binary)), "path": path}))
	Expect(os.ReadFile(path)).To(Equal(image.binary))
	for _, name := range []string{output, path} {
		info, err := os.Stat(name)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0700)), name)
	}
	entries, err := os.ReadDir(output)
	Expect(err).NotTo(HaveOccurred())
	Expect(entries).To(HaveLen(1))
	Expect(entries[0].Name()).To(Equal("factory-review-client"))
	return path
}

// per docs/adr/0153-authenticated-review-client-bootstrap.md:24
func reviewClientArguments(version, revision, output string) []string {
	return []string{"ci", "review-client", "--go-candidate", version, "--expected-revision", revision, "--output", output}
}
