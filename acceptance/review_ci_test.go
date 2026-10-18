package acceptance_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const ciReviewBase = "1111111111111111111111111111111111111111"
const ciReviewHead = "2222222222222222222222222222222222222222"
const ciReviewOther = "3333333333333333333333333333333333333333"

type ciReviewRequest struct {
	host, method, path, query, authorization, accept string
	body                                             []byte
}

type ciReviewFile struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

type ciReviewScenario struct {
	headChangeAt, baseChangeAt                int
	head, base, branch, headRepository, state string
	changedFiles                              int
	files                                     []ciReviewFile
	diff                                      string
	failurePath                               string
	failureStatus                             int
	failureLocation                           string
	providerStream                            string
	providerStatus                            int
	providerStarted                           chan struct{}
	providerRelease                           <-chan struct{}
}

type ciReviewService struct {
	proxy, ca string
	mu        sync.Mutex
	requests  []ciReviewRequest
	metadata  int
}

func ciReviewInput(count int) ([]ciReviewFile, string) {
	files := make([]ciReviewFile, 0, count)
	var diff strings.Builder
	for i := 0; i < count; i++ {
		path := fmt.Sprintf("internal/sample/file%03d.go", i)
		files = append(files, ciReviewFile{Filename: path, Status: "added", Additions: 1})
		fmt.Fprintf(&diff, "diff --git a/%s b/%s\nnew file mode 100644\nindex 0000000..1111111\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1 @@\n+package sample\n", path, path, path)
	}
	return files, diff.String()
}

// The proxy routes only the two production HTTPS destinations to a test-owned
// certificate. It cannot silently permit a configurable service endpoint.
// per docs/adr/0100-batched-review.md:106
func ciReviewFixture(cwd string, scenario ciReviewScenario) *ciReviewService {
	GinkgoHelper()
	if scenario.files == nil {
		scenario.files, scenario.diff = ciReviewInput(1)
	}
	if scenario.changedFiles == 0 {
		scenario.changedFiles = len(scenario.files)
	}
	if scenario.head == "" {
		scenario.head = ciReviewHead
	}
	if scenario.base == "" {
		scenario.base = ciReviewBase
	}
	if scenario.branch == "" {
		scenario.branch = "main"
	}
	if scenario.headRepository == "" {
		scenario.headRepository = "fixture/project"
	}
	if scenario.state == "" {
		scenario.state = "open"
	}
	if scenario.providerStream == "" {
		scenario.providerStream = reviewComplete
	}
	service := &ciReviewService{}
	service.proxy, service.ca = reviewTLSProxy(cwd, []string{"api.github.com", "openrouter.ai"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			http.Error(w, "fixture read failure", 500)
			return
		}
		service.mu.Lock()
		service.requests = append(service.requests, ciReviewRequest{r.Host, r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), r.Header.Get("Accept"), body})
		metadata := service.metadata
		if r.Host == "api.github.com" && r.Method == http.MethodGet && r.URL.Path == "/repos/fixture/project/pulls/7" && !strings.Contains(r.Header.Get("Accept"), "diff") {
			service.metadata++
			metadata = service.metadata
		}
		service.mu.Unlock()
		if r.Host == "openrouter.ai" {
			if scenario.providerStarted != nil {
				select {
				case scenario.providerStarted <- struct{}{}:
				default:
				}
			}
			if scenario.providerRelease != nil {
				select {
				case <-scenario.providerRelease:
				case <-r.Context().Done():
					return
				}
			}
			if scenario.providerStatus != 0 {
				http.Error(w, "PRIVATE_PROVIDER_ERROR PRIVATE_CI_REVIEW_KEY", scenario.providerStatus)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, scenario.providerStream)
			return
		}
		if r.URL.Path == scenario.failurePath {
			if scenario.failureLocation != "" {
				w.Header().Set("Location", scenario.failureLocation)
			}
			http.Error(w, "PRIVATE_GITHUB_ERROR PRIVATE_CI_GITHUB_KEY", scenario.failureStatus)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/fixture/project":
			_ = json.NewEncoder(w).Encode(map[string]any{"full_name": "fixture/project", "default_branch": "main"})
		case r.Method == http.MethodGet && r.URL.Path == "/repos/fixture/project/pulls/7" && strings.Contains(r.Header.Get("Accept"), "diff"):
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, scenario.diff)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/fixture/project/pulls/7":
			head, base := scenario.head, scenario.base
			if scenario.headChangeAt > 0 && metadata >= scenario.headChangeAt {
				head = ciReviewOther
			}
			if scenario.baseChangeAt > 0 && metadata >= scenario.baseChangeAt {
				base = ciReviewOther
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"number": 7, "state": scenario.state, "changed_files": scenario.changedFiles, "base": map[string]any{"sha": base, "ref": scenario.branch, "repo": map[string]any{"full_name": "fixture/project"}}, "head": map[string]any{"sha": head, "repo": map[string]any{"full_name": scenario.headRepository}}})
		case r.Method == http.MethodGet && r.URL.Path == "/repos/fixture/project/pulls/7/files":
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page == 0 {
				page = 1
			}
			start, end := (page-1)*100, page*100
			if start > len(scenario.files) {
				start = len(scenario.files)
			}
			if end > len(scenario.files) {
				end = len(scenario.files)
			}
			_ = json.NewEncoder(w).Encode(scenario.files[start:end])
		case r.Method == http.MethodPost && r.URL.Path == "/repos/fixture/project/check-runs":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 741})
		case r.Method == http.MethodPatch && r.URL.Path == "/repos/fixture/project/check-runs/741":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 741})
		case r.Method == http.MethodPost && r.URL.Path == "/repos/fixture/project/issues/7/comments":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 742})
		default:
			http.Error(w, "unexpected fixture endpoint", 404)
		}
	}))
	return service
}

func (s *ciReviewService) observed() []ciReviewRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ciReviewRequest(nil), s.requests...)
}

func ciReviewCommand(root, cwd, output string, service *ciReviewService, options, environment []string) *exec.Cmd {
	args := []string{"review-batch", "github", "--pr", "7", "--repository", "fixture/project", "--expected-base", ciReviewBase, "--output", output}
	args = append(args, options...)
	cmd := exec.Command(filepath.Join(root, "factory"), args...) // #nosec G204 G702 -- compiled fixture executable and literal public argv; no shell.
	cmd.Dir = cwd
	cmd.Env = []string{"PATH=/absent-ci-review-tools", "FACTORY_CONFIG=" + filepath.Join(cwd, "factory.yaml"), "GITHUB_TOKEN=PRIVATE_CI_GITHUB_KEY", "REVIEW_API_KEY=PRIVATE_CI_REVIEW_KEY", "REVIEW_MODEL=fixture/model", "REVIEW_OPENROUTER_PROVIDER=deepinfra", "REVIEW_MAX_TOKENS=1024", "REVIEW_REASONING_EFFORT=none", "REVIEW_BATCH_BYTES=60000", "REVIEW_MAX_BATCHES=16", "REVIEW_MAX_REQUESTS=16", "REVIEW_TOTAL_MAX_TOKENS=16384", "REVIEW_TOTAL_INPUT_BYTES=2097152", "REVIEW_TIMEOUT_SECONDS=5", "REVIEW_HTTP_RETRIES=0", "GITHUB_RUN_ID=123456", "GITHUB_SERVER_URL=https://github.com"}
	if service != nil {
		cmd.Env = append(cmd.Env, "HTTPS_PROXY="+service.proxy, "SSL_CERT_FILE="+service.ca)
	}
	for _, value := range environment {
		key, _, _ := strings.Cut(value, "=")
		filtered := cmd.Env[:0]
		for _, old := range cmd.Env {
			if !strings.HasPrefix(old, key+"=") {
				filtered = append(filtered, old)
			}
		}
		filtered = append(filtered, value)
		cmd.Env = filtered
	}
	return cmd
}

func ciReviewRun(root, cwd, output string, service *ciReviewService, options, environment []string) cliResult {
	GinkgoHelper()
	cmd := ciReviewCommand(root, cwd, output, service, options, environment)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	Expect(cmd.Start()).To(Succeed())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-time.After(12 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		Fail("hosted review exceeded independent fixture watchdog")
	}
	status := 0
	if err != nil {
		var exit *exec.ExitError
		Expect(errors.As(err, &exit)).To(BeTrue())
		status = exit.ExitCode()
	}
	Expect(stderr.String()).NotTo(ContainSubstring("unknown command"), "the public hosted adapter route must exist")
	Expect(stdout.String() + stderr.String()).NotTo(ContainSubstring("PRIVATE_CI_"))
	return cliResult{stdout.String(), stderr.String(), status}
}

func ciReviewRequests(service *ciReviewService, host, method, path string) []ciReviewRequest {
	var selected []ciReviewRequest
	for _, request := range service.observed() {
		if request.host == host && request.method == method && request.path == path {
			selected = append(selected, request)
		}
	}
	return selected
}

func ciReviewJSON(request ciReviewRequest) map[string]any {
	GinkgoHelper()
	var value map[string]any
	Expect(json.Unmarshal(request.body, &value)).To(Succeed())
	return value
}

func ciReviewNoSuccess(service *ciReviewService) {
	GinkgoHelper()
	for _, request := range ciReviewRequests(service, "api.github.com", http.MethodPatch, "/repos/fixture/project/check-runs/741") {
		Expect(ciReviewJSON(request)["conclusion"]).NotTo(Equal("success"))
	}
}

var _ = Describe("Hosted budgeted adversarial review", func() {
	// per docs/adr/0100-batched-review.md:103
	// per docs/adr/0100-batched-review.md:113
	It("reviews the exact current head and publishes a separate successful completion check", func() {
		root, cwd := fixture()
		service := ciReviewFixture(cwd, ciReviewScenario{})
		out := filepath.Join(cwd, "hosted-report")
		result := ciReviewRun(root, cwd, out, service, []string{"--expected-head", ciReviewHead}, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		data, err := os.ReadFile(filepath.Join(out, "report.json"))
		Expect(err).NotTo(HaveOccurred())
		var report map[string]any
		Expect(json.Unmarshal(data, &report)).To(Succeed())
		Expect(report["status"]).To(Equal("complete"))
		Expect(report["repository"]).To(Equal("fixture/project"))
		Expect(report["base"]).To(Equal(ciReviewBase))
		Expect(report["head"]).To(Equal(ciReviewHead))
		Expect(ciReviewRequests(service, "openrouter.ai", http.MethodPost, "/api/v1/chat/completions")).To(HaveLen(1))
		checks := ciReviewRequests(service, "api.github.com", http.MethodPost, "/repos/fixture/project/check-runs")
		Expect(checks).To(HaveLen(1))
		Expect(ciReviewJSON(checks[0])["head_sha"]).To(Equal(ciReviewHead))
		Expect(ciReviewJSON(checks[0])["status"]).To(Equal("in_progress"))
		completed := ciReviewRequests(service, "api.github.com", http.MethodPatch, "/repos/fixture/project/check-runs/741")
		Expect(completed).To(HaveLen(1))
		Expect(ciReviewJSON(completed[0])["status"]).To(Equal("completed"))
		Expect(ciReviewJSON(completed[0])["conclusion"]).To(Equal("success"))
		comments := ciReviewRequests(service, "api.github.com", http.MethodPost, "/repos/fixture/project/issues/7/comments")
		Expect(comments).To(HaveLen(1))
		Expect(ciReviewJSON(comments[0])["body"]).To(And(ContainSubstring(ciReviewHead), ContainSubstring("Safe findings")))
		metadata := 0
		for _, request := range service.observed() {
			if request.host == "api.github.com" {
				Expect(request.authorization).To(Equal("Bearer PRIVATE_CI_GITHUB_KEY"))
				Expect(string(request.body)).NotTo(ContainSubstring("PRIVATE_CI_REVIEW_KEY"))
			} else {
				Expect(request.authorization).To(Equal("Bearer PRIVATE_CI_REVIEW_KEY"))
				Expect(string(request.body)).NotTo(ContainSubstring("PRIVATE_CI_GITHUB_KEY"))
			}
			if request.method == http.MethodGet && request.path == "/repos/fixture/project/pulls/7" && !strings.Contains(request.accept, "diff") {
				metadata++
			}
		}
		Expect(metadata).To(BeNumerically(">=", 3))
	})
	// per docs/adr/0100-batched-review.md:108
	It("binds manual reruns to valid initial revisions and refuses subsequent base changes", func() {
		for _, scenario := range []ciReviewScenario{{base: ciReviewOther}, {base: "invalid"}, {baseChangeAt: 2}} {
			root, cwd := fixture()
			service := ciReviewFixture(cwd, scenario)
			out := filepath.Join(cwd, "manual-report")
			result := ciReviewRun(root, cwd, out, service, []string{"--expected-base", ""}, nil)
			if scenario.base == ciReviewOther {
				Expect(result.status).To(Equal(0), result.stderr)
				report := batchReadReport(out)
				Expect(report["base"]).To(Equal(ciReviewOther))
				Expect(report["head"]).To(Equal(ciReviewHead))
				Expect(report["status"]).To(Equal("complete"))
			} else {
				Expect(result.status).NotTo(Equal(0))
				Expect(ciReviewRequests(service, "openrouter.ai", http.MethodPost, "/api/v1/chat/completions")).To(BeEmpty())
				ciReviewNoSuccess(service)
			}
		}
	})
	// per docs/adr/0100-batched-review.md:107
	DescribeTable("refuses unsupported or mismatched pull requests without model calls", func(kind string) {
		root, cwd := fixture()
		scenario := ciReviewScenario{}
		switch kind {
		case "fork":
			scenario.headRepository = "outsider/project"
		case "nondefault target":
			scenario.branch = "release"
		case "wrong trusted base":
			scenario.base = ciReviewOther
		case "closed":
			scenario.state = "closed"
		case "wrong expected head":
			scenario.head = ciReviewOther
		}
		service := ciReviewFixture(cwd, scenario)
		result := ciReviewRun(root, cwd, filepath.Join(cwd, "refused-report"), service, []string{"--expected-head", ciReviewHead}, nil)
		Expect(result.status).NotTo(Equal(0))
		Expect(ciReviewRequests(service, "openrouter.ai", http.MethodPost, "/api/v1/chat/completions")).To(BeEmpty())
		Expect(ciReviewRequests(service, "api.github.com", http.MethodPost, "/repos/fixture/project/issues/7/comments")).To(BeEmpty())
		ciReviewNoSuccess(service)
	}, Entry("fork", "fork"), Entry("nondefault target", "nondefault target"), Entry("wrong trusted base", "wrong trusted base"), Entry("closed", "closed"), Entry("wrong expected head", "wrong expected head"))
	// per docs/adr/0100-batched-review.md:109
	It("rechecks downloaded data before spending model requests", func() {
		root, cwd := fixture()
		service := ciReviewFixture(cwd, ciReviewScenario{headChangeAt: 2})
		result := ciReviewRun(root, cwd, filepath.Join(cwd, "stale-input-report"), service, []string{"--expected-head", ciReviewHead}, nil)
		Expect(result.status).NotTo(Equal(0))
		Expect(ciReviewRequests(service, "openrouter.ai", http.MethodPost, "/api/v1/chat/completions")).To(BeEmpty())
		Expect(ciReviewRequests(service, "api.github.com", http.MethodPost, "/repos/fixture/project/issues/7/comments")).To(BeEmpty())
		ciReviewNoSuccess(service)
	})
	// per docs/adr/0100-batched-review.md:110
	It("withholds stale findings and success if the head changes during generation", func() {
		root, cwd := fixture()
		service := ciReviewFixture(cwd, ciReviewScenario{headChangeAt: 3})
		result := ciReviewRun(root, cwd, filepath.Join(cwd, "stale-publication-report"), service, []string{"--expected-head", ciReviewHead}, nil)
		Expect(result.status).NotTo(Equal(0))
		Expect(ciReviewRequests(service, "openrouter.ai", http.MethodPost, "/api/v1/chat/completions")).To(HaveLen(1))
		Expect(ciReviewRequests(service, "api.github.com", http.MethodPost, "/repos/fixture/project/issues/7/comments")).To(BeEmpty())
		ciReviewNoSuccess(service)
	})
	// per docs/adr/0100-batched-review.md:111
	It("flattens all manifest pages and covers the final file", func() {
		root, cwd := fixture()
		files, diff := ciReviewInput(101)
		service := ciReviewFixture(cwd, ciReviewScenario{files: files, diff: diff})
		out := filepath.Join(cwd, "paginated-report")
		result := ciReviewRun(root, cwd, out, service, nil, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		pages := ciReviewRequests(service, "api.github.com", http.MethodGet, "/repos/fixture/project/pulls/7/files")
		Expect(pages).To(HaveLen(2))
		Expect(pages[0].query).To(And(ContainSubstring("per_page=100"), ContainSubstring("page=1")))
		Expect(pages[1].query).To(And(ContainSubstring("per_page=100"), ContainSubstring("page=2")))
		data, err := os.ReadFile(filepath.Join(out, "report.json"))
		Expect(err).NotTo(HaveOccurred())
		var report map[string]any
		Expect(json.Unmarshal(data, &report)).To(Succeed())
		Expect(report["status"]).To(Equal("complete"))
		counts, ok := report["counts"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(counts["files_total"]).To(Equal(float64(101)))
		Expect(counts["files_reviewed"]).To(Equal(float64(101)))
		Expect(string(data)).To(ContainSubstring("internal/sample/file100.go"))
	})
	// per docs/adr/0100-batched-review.md:111
	// per docs/adr/0100-batched-review.md:31
	DescribeTable("refuses incomplete GitHub coverage before model calls", func(kind string) {
		root, cwd := fixture()
		files, diff := ciReviewInput(1)
		scenario := ciReviewScenario{files: files, diff: diff}
		if kind == "manifest count" {
			scenario.changedFiles = 2
		} else {
			scenario.diff = strings.TrimSuffix(diff, "+package sample\n")
		}
		service := ciReviewFixture(cwd, scenario)
		result := ciReviewRun(root, cwd, filepath.Join(cwd, "incomplete-input-report"), service, nil, nil)
		Expect(result.status).NotTo(Equal(0))
		Expect(ciReviewRequests(service, "openrouter.ai", http.MethodPost, "/api/v1/chat/completions")).To(BeEmpty())
		ciReviewNoSuccess(service)
	}, Entry("manifest count", "manifest count"), Entry("truncated diff", "truncated diff"))
	// per docs/adr/0100-batched-review.md:116
	DescribeTable("missing credentials never imply a completed model review", func(key string) {
		root, cwd := fixture()
		service := ciReviewFixture(cwd, ciReviewScenario{})
		result := ciReviewRun(root, cwd, filepath.Join(cwd, "missing-key-report"), service, nil, []string{key + "="})
		Expect(result.status).NotTo(Equal(0))
		Expect(ciReviewRequests(service, "openrouter.ai", http.MethodPost, "/api/v1/chat/completions")).To(BeEmpty())
		ciReviewNoSuccess(service)
	}, Entry("GitHub", "GITHUB_TOKEN"), Entry("OpenRouter", "REVIEW_API_KEY"))
	// per docs/adr/0100-batched-review.md:112
	DescribeTable("sanitizes GitHub preparation failures without creating success", func(path string, code int) {
		root, cwd := fixture()
		service := ciReviewFixture(cwd, ciReviewScenario{failurePath: path, failureStatus: code})
		result := ciReviewRun(root, cwd, filepath.Join(cwd, "api-error-report"), service, nil, nil)
		Expect(result.status).NotTo(Equal(0))
		Expect(result.stdout + result.stderr).NotTo(ContainSubstring("PRIVATE_GITHUB_ERROR"))
		Expect(ciReviewRequests(service, "openrouter.ai", http.MethodPost, "/api/v1/chat/completions")).To(BeEmpty())
		ciReviewNoSuccess(service)
	}, Entry("metadata forbidden", "/repos/fixture/project/pulls/7", http.StatusForbidden), Entry("files unavailable", "/repos/fixture/project/pulls/7/files", http.StatusBadGateway))
	// per docs/adr/0100-batched-review.md:112
	It("refuses a GitHub redirect instead of following it with credentials", func() {
		root, cwd := fixture()
		service := ciReviewFixture(cwd, ciReviewScenario{failurePath: "/repos/fixture/project/pulls/7", failureStatus: http.StatusFound, failureLocation: "https://api.github.com/repos/fixture/project/redirect-target"})
		result := ciReviewRun(root, cwd, filepath.Join(cwd, "redirect-report"), service, nil, nil)
		Expect(result.status).NotTo(Equal(0))
		Expect(ciReviewRequests(service, "api.github.com", http.MethodGet, "/repos/fixture/project/redirect-target")).To(BeEmpty())
		Expect(ciReviewRequests(service, "openrouter.ai", http.MethodPost, "/api/v1/chat/completions")).To(BeEmpty())
		ciReviewNoSuccess(service)
	})
	// per docs/adr/0100-batched-review.md:113
	// per docs/adr/0100-batched-review.md:72
	DescribeTable("fails the completion check when generation does not complete", func(kind string) {
		root, cwd := fixture()
		scenario := ciReviewScenario{}
		if kind == "provider error" {
			scenario.providerStatus = http.StatusBadGateway
		} else {
			scenario.providerStream = strings.Split(reviewComplete, "data: [DONE]")[0]
		}
		service := ciReviewFixture(cwd, scenario)
		result := ciReviewRun(root, cwd, filepath.Join(cwd, "generation-error-report"), service, nil, nil)
		Expect(result.status).NotTo(Equal(0))
		Expect(result.stdout + result.stderr).NotTo(ContainSubstring("PRIVATE_PROVIDER_ERROR"))
		completed := ciReviewRequests(service, "api.github.com", http.MethodPatch, "/repos/fixture/project/check-runs/741")
		Expect(completed).To(HaveLen(1))
		Expect(ciReviewJSON(completed[0])["status"]).To(Equal("completed"))
		Expect(ciReviewJSON(completed[0])["conclusion"]).To(Equal("failure"))
		for _, request := range ciReviewRequests(service, "api.github.com", http.MethodPost, "/repos/fixture/project/issues/7/comments") {
			Expect(ciReviewJSON(request)["body"]).NotTo(ContainSubstring("Safe findings"))
		}
	}, Entry("provider error", "provider error"), Entry("interrupted stream", "interrupted stream"))
	// per docs/adr/0100-batched-review.md:115
	It("labels long findings as an excerpt with the complete run artifact link", func() {
		root, cwd := fixture()
		finding := strings.Repeat("Detailed finding with evidence.\n", 2400)
		delta, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": finding}}}})
		Expect(err).NotTo(HaveOccurred())
		stream := "data: " + string(delta) + "\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
		service := ciReviewFixture(cwd, ciReviewScenario{providerStream: stream})
		out := filepath.Join(cwd, "long-report")
		result := ciReviewRun(root, cwd, out, service, nil, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		comments := ciReviewRequests(service, "api.github.com", http.MethodPost, "/repos/fixture/project/issues/7/comments")
		Expect(comments).To(HaveLen(1))
		body, ok := ciReviewJSON(comments[0])["body"].(string)
		Expect(ok).To(BeTrue())
		Expect(strings.ToLower(body)).To(ContainSubstring("excerpt"))
		Expect(body).To(ContainSubstring("https://github.com/fixture/project/actions/runs/123456"))
		Expect(body).To(ContainSubstring(ciReviewHead))
		Expect(len(body)).To(BeNumerically("<", 65536))
		full, err := os.ReadFile(filepath.Join(out, "review.md"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(full)).To(ContainSubstring(finding))
	})
	// per docs/adr/0100-batched-review.md:89
	// per docs/adr/0100-batched-review.md:113
	It("cancels an active model request without publishing completed coverage", func() {
		root, cwd := fixture()
		started, release := make(chan struct{}, 1), make(chan struct{})
		defer close(release)
		service := ciReviewFixture(cwd, ciReviewScenario{providerStarted: started, providerRelease: release})
		cmd := ciReviewCommand(root, cwd, filepath.Join(cwd, "cancelled-report"), service, nil, []string{"REVIEW_TIMEOUT_SECONDS=60"})
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		Expect(cmd.Start()).To(Succeed())
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-started:
		case err := <-done:
			Fail(fmt.Sprintf("hosted review exited before model request: %v %s", err, stderr.String()))
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			Fail("hosted review did not reach model fixture")
		}
		Expect(cmd.Process.Signal(syscall.SIGTERM)).To(Succeed())
		select {
		case err := <-done:
			Expect(err).To(HaveOccurred())
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			Fail("hosted review ignored cancellation")
		}
		Expect(ciReviewRequests(service, "api.github.com", http.MethodPost, "/repos/fixture/project/issues/7/comments")).To(BeEmpty())
		ciReviewNoSuccess(service)
		Expect(stdout.String() + stderr.String()).NotTo(ContainSubstring("PRIVATE_CI_"))
	})
})
