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
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const batchBase = "1111111111111111111111111111111111111111"
const batchHead = "2222222222222222222222222222222222222222"

type batchFile struct {
	Filename  string `json:"filename"`
	Previous  string `json:"previous_filename,omitempty"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

type batchInput struct {
	diff  string
	files []batchFile
}

func batchAdded(paths []string, lines int) batchInput {
	var diff strings.Builder
	var files []batchFile
	for _, path := range paths {
		fmt.Fprintf(&diff, "diff --git a/%s b/%s\nnew file mode 100644\nindex 0000000..1111111\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", path, path, path, lines)
		for line := 0; line < lines; line++ {
			fmt.Fprintf(&diff, "+line-%06d-%s\n", line, strings.Repeat("x", 96))
		}
		files = append(files, batchFile{Filename: path, Status: "added", Additions: lines})
	}
	return batchInput{diff.String(), files}
}

func batchWriteInput(cwd string, input batchInput) (string, string) {
	GinkgoHelper()
	diff, manifest := filepath.Join(cwd, "changes.diff"), filepath.Join(cwd, "files.json")
	writeFixture(diff, []byte(input.diff), 0600)
	data, err := json.Marshal(input.files)
	Expect(err).NotTo(HaveOccurred())
	writeFixture(manifest, data, 0600)
	return diff, manifest
}

func batchCommand(root, cwd, output string, service *reviewService, options, environment []string) *exec.Cmd {
	GinkgoHelper()
	args := []string{"review-batch", "--diff", filepath.Join(cwd, "changes.diff"), "--files", filepath.Join(cwd, "files.json"), "--base", batchBase, "--head", batchHead, "--repository", "fixture/project", "--output", output}
	args = append(args, options...)
	cmd := exec.Command(filepath.Join(root, "factory"), args...) // #nosec G204 G702 -- test-owned compiled executable; literal argv, never a shell.
	cmd.Dir = cwd
	cmd.Env = []string{"PATH=/absent-review-tools", "FACTORY_CONFIG=" + filepath.Join(cwd, "factory.yaml"), "REVIEW_API_KEY=PRIVATE_BATCH_KEY", "REVIEW_MODEL=fixture/model", "REVIEW_OPENROUTER_PROVIDER=deepinfra", "REVIEW_MAX_TOKENS=1024", "REVIEW_REASONING_EFFORT=none", "REVIEW_BATCH_BYTES=4096", "REVIEW_MAX_BATCHES=64", "REVIEW_MAX_REQUESTS=64", "REVIEW_TOTAL_MAX_TOKENS=65536", "REVIEW_TOTAL_INPUT_BYTES=16777216", "REVIEW_TIMEOUT_SECONDS=5", "REVIEW_HTTP_RETRIES=0"}
	if service != nil {
		cmd.Env = append(cmd.Env, "HTTPS_PROXY="+service.proxy, "SSL_CERT_FILE="+service.ca)
	}
	// Explicit later values override fixture defaults just as callers override YAML.
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

func batchRun(root, cwd, output string, service *reviewService, options, environment []string) cliResult {
	GinkgoHelper()
	cmd := batchCommand(root, cwd, output, service, options, environment)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	Expect(cmd.Start()).To(Succeed())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-time.After(12 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		Fail("review-batch exceeded independent fixture watchdog")
	}
	status := 0
	if err != nil {
		var exit *exec.ExitError
		Expect(errors.As(err, &exit)).To(BeTrue())
		status = exit.ExitCode()
	}
	Expect(stderr.String()).NotTo(ContainSubstring("unknown command"), "the public review-batch route must exist")
	Expect(stdout.String() + stderr.String()).NotTo(ContainSubstring("PRIVATE_BATCH_KEY"))
	return cliResult{stdout.String(), stderr.String(), status}
}

func batchReadReport(output string) map[string]any {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join(output, "report.json"))
	Expect(err).NotTo(HaveOccurred())
	var report map[string]any
	Expect(json.Unmarshal(data, &report)).To(Succeed())
	return report
}

func batchCounts(report map[string]any) map[string]any {
	GinkgoHelper()
	result, ok := report["counts"].(map[string]any)
	Expect(ok).To(BeTrue())
	return result
}
func batchBudget(report map[string]any) map[string]any {
	GinkgoHelper()
	result, ok := report["budget"].(map[string]any)
	Expect(ok).To(BeTrue())
	return result
}

func batchServer(cwd string) *reviewService {
	return reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, reviewComplete)
	})
}

var _ = Describe("Budgeted batched adversarial review", func() {
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:23
	It("plans deterministically offline with exact revision identity and private artifacts", func() {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"internal/a.go", "internal/a_test.go"}, 12))
		service := batchServer(cwd)
		var reports []map[string]any
		for _, name := range []string{"plan-one", "plan-two"} {
			out := filepath.Join(cwd, name)
			result := batchRun(root, cwd, out, service, []string{"--plan"}, []string{"REVIEW_API_KEY="})
			Expect(result.status).To(Equal(0), result.stderr)
			report := batchReadReport(out)
			Expect(report["version"]).To(Equal(float64(1)))
			Expect(report["status"]).To(Equal("planned"))
			Expect(report["repository"]).To(Equal("fixture/project"))
			Expect(report["base"]).To(Equal(batchBase))
			Expect(report["head"]).To(Equal(batchHead))
			Expect(report["context_digest"]).To(MatchRegexp(`^[a-f0-9]{64}$`))
			Expect(report["policy_digest"]).To(MatchRegexp(`^[a-f0-9]{64}$`))
			Expect(batchCounts(report)["files_total"]).To(Equal(float64(2)))
			Expect(batchCounts(report)["files_reviewed"]).To(Equal(float64(0)))
			Expect(batchBudget(report)["requests_used"]).To(Equal(float64(0)))
			reports = append(reports, report)
			for _, path := range []string{out, filepath.Join(out, "report.json"), filepath.Join(out, "review.md")} {
				info, err := os.Lstat(path)
				Expect(err).NotTo(HaveOccurred())
				Expect(info.Mode().Perm() & 0077).To(BeZero())
			}
		}
		Expect(reports[0]).To(Equal(reports[1]))
		Expect(service.observed()).To(BeEmpty())
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:29
	DescribeTable("rejects incomplete or contradictory coverage before contacting a provider", func(kind string) {
		root, cwd := fixture()
		input := batchAdded([]string{"src/a.go"}, 3)
		options := []string{}
		switch kind {
		case "missing file":
			input.files = nil
		case "duplicate manifest":
			input.files = append(input.files, input.files[0])
		case "missing diff":
			input.diff = ""
		case "missing hunk line":
			input.diff = strings.TrimSuffix(input.diff, "\n")
			input.diff = input.diff[:strings.LastIndex(input.diff, "\n")+1]
		case "wrong additions":
			input.files[0].Additions++
		case "hunk count":
			input.diff = strings.Replace(input.diff, "+1,3", "+1,9", 1)
		case "undeclared file":
			input.files[0].Filename = "other.go"
		case "unsafe path":
			input.files[0].Filename = "../escape.go"
			input.diff = strings.ReplaceAll(input.diff, "src/a.go", "../escape.go")
		case "duplicate diff":
			input.diff += input.diff
		case "bad base":
			options = []string{"--base", "not-a-revision"}
		case "bad repository":
			options = []string{"--repository", "owner/repo/extra"}
		case "too many files":
			var paths []string
			for n := 0; n < 3001; n++ {
				paths = append(paths, fmt.Sprintf("file-%d", n))
			}
			input = batchAdded(paths, 1)
		case "input limit":
			input = batchAdded([]string{"large.go"}, 80000)
		}
		_, manifest := batchWriteInput(cwd, input)
		if kind == "malformed manifest" {
			writeFixture(manifest, []byte(`[{"filename":`), 0600)
		}
		if kind == "missing manifest" {
			Expect(os.Remove(manifest)).To(Succeed())
		}
		service := batchServer(cwd)
		result := batchRun(root, cwd, filepath.Join(cwd, "out"), service, options, nil)
		Expect(result.status).NotTo(Equal(0))
		Expect(service.observed()).To(BeEmpty())
	}, Entry("manifest omits diff file", "missing file"), Entry("duplicate manifest filename", "duplicate manifest"), Entry("empty diff for manifest", "missing diff"), Entry("truncated final hunk", "missing hunk line"), Entry("API statistics disagree", "wrong additions"), Entry("hunk counts disagree", "hunk count"), Entry("diff file absent from manifest", "undeclared file"), Entry("traversal path", "unsafe path"), Entry("repeated file section", "duplicate diff"), Entry("invalid base SHA", "bad base"), Entry("invalid repository", "bad repository"), Entry("malformed manifest", "malformed manifest"), Entry("missing manifest", "missing manifest"), Entry("over 3000 files", "too many files"), Entry("over eight MiB diff", "input limit"))
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:29
	It("uses the full diff rather than a truncated optional API patch", func() {
		root, cwd := fixture()
		input := batchInput{
			"diff --git a/a.go b/a.go\nindex 1111111..2222222 100644\n--- a/a.go\n+++ b/a.go\n@@ -1,3 +1,3 @@\n keep\n-before\n+after\n tail\n",
			[]batchFile{{Filename: "a.go", Status: "modified", Additions: 1, Deletions: 1}},
		}
		_, manifest := batchWriteInput(cwd, input)
		writeFixture(manifest, []byte(`[{"filename":"a.go","status":"modified","additions":1,"deletions":1,"patch":"@@ -0,0 +1,999 @@\n+not authoritative"}]`), 0600)
		service := batchServer(cwd)
		out := filepath.Join(cwd, "out")
		result := batchRun(root, cwd, out, service, nil, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(batchReadReport(out)["status"]).To(Equal("complete"))
		Expect(service.observed()).To(HaveLen(1))
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:29
	It("reviews Git-produced filenames with spaces and tab-terminated patch markers", func() {
		root, cwd := fixture()
		durableGit(cwd, "init", "-q")
		path := filepath.Join(cwd, "a b.txt")
		writeFixture(path, []byte("before\n"), 0600)
		durableGit(cwd, "add", "--", "a b.txt")
		writeFixture(path, []byte("after\n"), 0600)
		diff := durableGit(cwd, "diff", "--no-ext-diff", "--no-textconv", "--", "a b.txt")
		Expect(diff).To(ContainSubstring("--- a/a b.txt\t\n"))
		batchWriteInput(cwd, batchInput{diff, []batchFile{{Filename: "a b.txt", Status: "modified", Additions: 1, Deletions: 1}}})
		service := batchServer(cwd)
		out := filepath.Join(cwd, "out")
		result := batchRun(root, cwd, out, service, nil, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(batchReadReport(out)["status"]).To(Equal("complete"))
		Expect(service.observed()).To(HaveLen(1))
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:29
	It("reviews both Git type-change sections under the single changed-file manifest entry", func() {
		root, cwd := fixture()
		durableGit(cwd, "init", "-q")
		for _, path := range []string{"link", "plain.txt"} {
			writeFixture(filepath.Join(cwd, path), []byte("before\n"), 0600)
		}
		durableGit(cwd, "add", "--", "link", "plain.txt")
		Expect(os.Remove(filepath.Join(cwd, "link"))).To(Succeed())
		Expect(os.Symlink("target", filepath.Join(cwd, "link"))).To(Succeed())
		writeFixture(filepath.Join(cwd, "plain.txt"), []byte("after\n"), 0600)
		diff := durableGit(cwd, "diff", "--no-ext-diff", "--no-textconv")
		Expect(strings.Count(diff, "diff --git a/link b/link\n")).To(Equal(2))
		batchWriteInput(cwd, batchInput{diff, []batchFile{{Filename: "link", Status: "changed", Additions: 1, Deletions: 1}, {Filename: "plain.txt", Status: "modified", Additions: 1, Deletions: 1}}})
		service := batchServer(cwd)
		out := filepath.Join(cwd, "out")
		result := batchRun(root, cwd, out, service, nil, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		report := batchReadReport(out)
		Expect(report["status"]).To(Equal("complete"))
		Expect(batchCounts(report)["files_reviewed"]).To(Equal(float64(2)))
		var prompts strings.Builder
		for _, request := range service.observed() {
			prompts.Write(request.body)
		}
		Expect(prompts.String()).To(ContainSubstring("120000"))
		Expect(prompts.String()).To(ContainSubstring("+target"))
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:31
	It("accepts paired mode changes but refuses a truncated metadata-only diff before calls", func() {
		root, cwd := fixture()
		input := batchInput{"diff --git a/mode.txt b/mode.txt\nold mode 100644\nnew mode 100755\n", []batchFile{{Filename: "mode.txt", Status: "modified"}}}
		batchWriteInput(cwd, input)
		service := batchServer(cwd)
		control := batchRun(root, cwd, filepath.Join(cwd, "valid-plan"), service, []string{"--plan"}, nil)
		Expect(control.status).To(Equal(0), control.stderr)
		input.diff = strings.Replace(input.diff, "new mode 100755\n", "", 1)
		batchWriteInput(cwd, input)
		result := batchRun(root, cwd, filepath.Join(cwd, "truncated"), service, nil, nil)
		Expect(result.status).NotTo(Equal(0))
		Expect(service.observed()).To(BeEmpty())
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:32
	It("reviews a diff beyond the former 200000-byte skip with explicit complete coverage", func() {
		root, cwd := fixture()
		input := batchAdded([]string{"one.go", "two.go", "three.go", "four.go", "five.go"}, 450)
		Expect(len(input.diff)).To(BeNumerically(">", 200000))
		batchWriteInput(cwd, input)
		service := batchServer(cwd)
		out := filepath.Join(cwd, "out")
		result := batchRun(root, cwd, out, service, nil, []string{"REVIEW_BATCH_BYTES=60000"})
		Expect(result.status).To(Equal(0), result.stderr)
		report := batchReadReport(out)
		Expect(report["status"]).To(Equal("complete"))
		counts := batchCounts(report)
		Expect(counts["files_reviewed"]).To(Equal(float64(5)))
		Expect(counts["fragments_reviewed"]).To(Equal(counts["fragments_total"]))
		Expect(counts["batches_completed"]).To(Equal(counts["batches_planned"]))
		Expect(len(service.observed())).To(BeNumerically(">", 1))
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:33
	It("fragments an oversized single hunk without dropping its beginning or end", func() {
		root, cwd := fixture()
		input := batchAdded([]string{"one.go"}, 150)
		batchWriteInput(cwd, input)
		service := batchServer(cwd)
		out := filepath.Join(cwd, "out")
		result := batchRun(root, cwd, out, service, nil, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		report := batchReadReport(out)
		counts := batchCounts(report)
		Expect(counts["fragments_total"]).To(BeNumerically(">", 1))
		Expect(counts["fragments_reviewed"]).To(Equal(counts["fragments_total"]))
		var prompts strings.Builder
		for _, request := range service.observed() {
			var body map[string]any
			Expect(json.Unmarshal(request.body, &body)).To(Succeed())
			for _, message := range body["messages"].([]any) {
				prompts.WriteString(message.(map[string]any)["content"].(string))
			}
		}
		Expect(prompts.String()).To(ContainSubstring("line-000000-"))
		Expect(prompts.String()).To(ContainSubstring("line-000149-"))
		for line := 0; line < 150; line++ {
			Expect(prompts.String()).To(ContainSubstring(fmt.Sprintf("+line-%06d-", line)), "every changed line must reach a reviewed batch")
		}
		Expect(report["status"]).To(Equal("complete"))
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:34
	It("keeps binary-only changes explicitly uncovered rather than green", func() {
		root, cwd := fixture()
		input := batchInput{"diff --git a/image.bin b/image.bin\nnew file mode 100644\nindex 0000000..1111111\nBinary files /dev/null and b/image.bin differ\n", []batchFile{{Filename: "image.bin", Status: "added"}}}
		batchWriteInput(cwd, input)
		service := batchServer(cwd)
		out := filepath.Join(cwd, "out")
		result := batchRun(root, cwd, out, service, nil, nil)
		Expect(result.status).NotTo(Equal(0))
		report := batchReadReport(out)
		Expect(report["status"]).NotTo(Equal("complete"))
		Expect(batchCounts(report)["files_total"]).To(Equal(float64(1)))
		Expect(batchCounts(report)["files_reviewed"]).To(Equal(float64(0)))
		Expect(service.observed()).To(BeEmpty())
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:44
	DescribeTable("does not overwrite existing output or follow output/cache symlinks", func(kind string) {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"a.go"}, 1))
		service := batchServer(cwd)
		outside := filepath.Join(cwd, "outside")
		Expect(os.Mkdir(outside, 0700)).To(Succeed())
		sentinel := filepath.Join(outside, "sentinel")
		writeFixture(sentinel, []byte("retained"), 0600)
		out := filepath.Join(cwd, "out")
		options := []string{}
		switch kind {
		case "existing":
			Expect(os.Mkdir(out, 0700)).To(Succeed())
			writeFixture(filepath.Join(out, "report.json"), []byte("retained"), 0600)
		case "output link":
			Expect(os.Symlink(outside, out)).To(Succeed())
		case "parent link":
			link := filepath.Join(cwd, "linked-parent")
			Expect(os.Symlink(outside, link)).To(Succeed())
			out = filepath.Join(link, "out")
		case "cache link":
			link := filepath.Join(cwd, "cache-link")
			Expect(os.Symlink(outside, link)).To(Succeed())
			options = []string{"--cache", link}
		}
		result := batchRun(root, cwd, out, service, options, nil)
		Expect(result.status).NotTo(Equal(0))
		Expect(service.observed()).To(BeEmpty())
		data, err := os.ReadFile(sentinel)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("retained"))
	}, Entry("existing output directory", "existing"), Entry("linked output directory", "output link"), Entry("linked output ancestor", "parent link"), Entry("linked cache directory", "cache link"))
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:49
	DescribeTable("reserves each HTTP attempt against run-wide quotas", func(kind string, wantCalls int) {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"a.go", "b.go"}, 45))
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
			if kind == "retry" {
				w.WriteHeader(429)
				_, _ = io.WriteString(w, "PRIVATE_PROVIDER_DETAIL")
				return
			}
			_, _ = io.WriteString(w, reviewComplete)
		})
		out := filepath.Join(cwd, "out")
		environment := []string{}
		switch kind {
		case "requests":
			environment = []string{"REVIEW_MAX_REQUESTS=1"}
		case "tokens":
			environment = []string{"REVIEW_TOTAL_MAX_TOKENS=1024"}
		case "input":
			environment = []string{"REVIEW_TOTAL_INPUT_BYTES=4096"}
		case "batches":
			environment = []string{"REVIEW_MAX_BATCHES=1"}
		case "retry":
			environment = []string{"REVIEW_MAX_REQUESTS=1", "REVIEW_HTTP_RETRIES=1"}
		}
		result := batchRun(root, cwd, out, service, nil, environment)
		Expect(result.status).NotTo(Equal(0))
		report := batchReadReport(out)
		Expect(report["status"]).NotTo(Equal("complete"))
		Expect(service.observed()).To(HaveLen(wantCalls))
		budget := batchBudget(report)
		Expect(budget["requests_used"]).To(Equal(float64(wantCalls)))
		Expect(budget["response_tokens_reserved"]).To(Equal(float64(wantCalls * 1024)))
		Expect(budget["requests_used"]).To(BeNumerically("<=", budget["max_requests"]))
		Expect(budget["input_bytes_reserved"]).To(BeNumerically("<=", budget["max_input_bytes"]))
		Expect(result.stdout + result.stderr).NotTo(ContainSubstring("PRIVATE_PROVIDER_DETAIL"))
	}, Entry("request allowance", "requests", 1), Entry("response reservation allowance", "tokens", 1), Entry("input reservation allowance", "input", 0), Entry("batch allowance", "batches", 0), Entry("retry consumes the same request allowance", "retry", 1))
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:71
	It("retries an initial 429 within the same reservation budget", func() {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"a.go"}, 1))
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, n int) {
			if n == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(429)
				return
			}
			_, _ = io.WriteString(w, reviewComplete)
		})
		out := filepath.Join(cwd, "out")
		result := batchRun(root, cwd, out, service, nil, []string{"REVIEW_HTTP_RETRIES=1", "REVIEW_MAX_REQUESTS=2"})
		Expect(result.status).To(Equal(0), result.stderr)
		report := batchReadReport(out)
		Expect(report["status"]).To(Equal("complete"))
		Expect(service.observed()).To(HaveLen(2))
		Expect(batchBudget(report)["requests_used"]).To(Equal(float64(2)))
		Expect(batchBudget(report)["response_tokens_reserved"]).To(Equal(float64(2048)))
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:72
	DescribeTable("does not publish or retry ambiguous stream findings", func(stream string) {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"a.go"}, 1))
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) { _, _ = io.WriteString(w, stream) })
		out := filepath.Join(cwd, "out")
		result := batchRun(root, cwd, out, service, nil, []string{"REVIEW_HTTP_RETRIES=1"})
		Expect(result.status).NotTo(Equal(0))
		Expect(service.observed()).To(HaveLen(1))
		report := batchReadReport(out)
		Expect(report["status"]).To(Equal("failed"))
		Expect(batchCounts(report)["batches_completed"]).To(Equal(float64(0)))
		reportBytes, err := json.Marshal(report)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(reportBytes)).NotTo(ContainSubstring("Safe findings"))
		Expect(string(reportBytes)).NotTo(ContainSubstring("PRIVATE_REASONING"))
		data, err := os.ReadFile(filepath.Join(out, "review.md"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).NotTo(ContainSubstring("Safe findings"))
		Expect(string(data)).NotTo(ContainSubstring("PRIVATE_REASONING"))
	}, Entry("EOF before DONE", strings.Split(reviewComplete, "data: [DONE]")[0]), Entry("response token exhaustion", strings.ReplaceAll(reviewComplete, `"stop"`, `"length"`)), Entry("provider failure after content", strings.Replace(reviewComplete, "data: [DONE]", "data: {\"error\":{\"message\":\"PRIVATE_REASONING\"}}\n\ndata: [DONE]", 1)))
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:54
	It("keeps unreported usage null and reserves rather than invents actual token usage", func() {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"a.go"}, 1))
		service := batchServer(cwd)
		out := filepath.Join(cwd, "out")
		result := batchRun(root, cwd, out, service, nil, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		report := batchReadReport(out)
		Expect(report).To(HaveKeyWithValue("usage", BeNil()))
		Expect(batchBudget(report)["response_tokens_reserved"]).To(Equal(float64(1024)))
		batches := report["batches"].([]any)
		Expect(batches[0]).To(HaveKeyWithValue("usage", BeNil()))
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:54
	It("preserves real provider usage separately from allowance reservations", func() {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"a.go"}, 1))
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
			_, _ = io.WriteString(w, strings.Replace(reviewComplete, "data: [DONE]", "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":17,\"completion_tokens\":3,\"total_tokens\":20,\"cost\":0.0001}}\n\ndata: [DONE]", 1))
		})
		out := filepath.Join(cwd, "out")
		result := batchRun(root, cwd, out, service, nil, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		usage := batchReadReport(out)["usage"].(map[string]any)
		Expect(usage["total_tokens"]).To(Equal(float64(20)))
		Expect(usage["cost"]).To(Equal(0.0001))
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:54
	It("keeps explicitly null provider cost unknown instead of reporting zero spend", func() {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"a.go"}, 1))
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
			_, _ = io.WriteString(w, strings.Replace(reviewComplete, "data: [DONE]", "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":17,\"completion_tokens\":3,\"total_tokens\":20,\"cost\":null}}\n\ndata: [DONE]", 1))
		})
		out := filepath.Join(cwd, "out")
		result := batchRun(root, cwd, out, service, nil, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		report := batchReadReport(out)
		usage := report["usage"].(map[string]any)
		Expect(usage["total_tokens"]).To(Equal(float64(20)))
		Expect(usage["cost"]).To(BeNil(), "unknown provider cost must never become a claimed zero")
		batch := report["batches"].([]any)[0].(map[string]any)
		Expect(batch["usage"].(map[string]any)["cost"]).To(BeNil())
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:54
	It("does not report incomplete per-batch usage fields as complete aggregate spend", func() {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"a.go"}, 45))
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, n int) {
			usage := `{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8,"cost":0.0001}`
			if n > 1 {
				usage = `{"completion_tokens":7,"total_tokens":7}`
			}
			_, _ = io.WriteString(w, strings.Replace(reviewComplete, "data: [DONE]", "data: {\"choices\":[],\"usage\":"+usage+"}\n\ndata: [DONE]", 1))
		})
		out := filepath.Join(cwd, "out")
		result := batchRun(root, cwd, out, service, nil, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(service.observed()).To(HaveLen(2))
		report := batchReadReport(out)
		Expect(report["status"]).To(Equal("complete"))
		usage := report["usage"].(map[string]any)
		Expect(usage["completion_tokens"]).To(Equal(float64(10)))
		Expect(usage["total_tokens"]).To(Equal(float64(15)))
		Expect(usage["cost"]).To(BeNil(), "one batch's known cost cannot describe the whole review")
		Expect(usage["prompt_tokens"]).To(BeNil(), "missing accounting fields remain unknown")
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:38
	It("reuses authenticated successful batches across a new head while reporting the new identity", func() {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"a.go"}, 2))
		service := batchServer(cwd)
		cache := filepath.Join(cwd, "cache")
		first := filepath.Join(cwd, "first")
		result := batchRun(root, cwd, first, service, []string{"--cache", cache}, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		second := filepath.Join(cwd, "second")
		newHead := strings.Repeat("3", 40)
		result = batchRun(root, cwd, second, service, []string{"--cache", cache, "--head", newHead}, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(service.observed()).To(HaveLen(1))
		report := batchReadReport(second)
		Expect(report["head"]).To(Equal(newHead))
		Expect(report["status"]).To(Equal("complete"))
		Expect(batchCounts(report)["batches_cached"]).To(Equal(float64(1)))
		Expect(batchBudget(report)["requests_used"]).To(Equal(float64(0)))
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:38
	DescribeTable("does not reuse a result with a changed authenticated identity", func(kind string) {
		root, cwd := fixture()
		lines := 2
		if kind == "context" {
			lines = 45
		}
		input := batchAdded([]string{"a.go"}, lines)
		batchWriteInput(cwd, input)
		service := batchServer(cwd)
		cache := filepath.Join(cwd, "cache")
		result := batchRun(root, cwd, filepath.Join(cwd, "first"), service, []string{"--cache", cache}, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		options := []string{"--cache", cache}
		environment := []string{}
		switch kind {
		case "body":
			input.diff = strings.Replace(input.diff, "line-000000-", "changed-line-", 1)
			batchWriteInput(cwd, input)
		case "context":
			// Keep a.go's already-fragmented patch identical, so cache misses
			// for its batches require shared context to participate in identity.
			input = batchAdded([]string{"a.go", "new.go"}, 45)
			batchWriteInput(cwd, input)
		case "model":
			environment = []string{"REVIEW_MODEL=fixture/different-model"}
		case "base":
			options = append(options, "--base", strings.Repeat("4", 40))
		case "policy":
			environment = []string{"REVIEW_MAX_TOKENS=2048"}
		case "key":
			environment = []string{"REVIEW_API_KEY=PRIVATE_CHANGED_KEY"}
		}
		out := filepath.Join(cwd, "second")
		result = batchRun(root, cwd, out, service, options, environment)
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(len(service.observed())).To(BeNumerically(">", 1))
		Expect(batchCounts(batchReadReport(out))["batches_cached"]).To(Equal(float64(0)))
	}, Entry("changed patch bytes", "body"), Entry("changed shared context", "context"), Entry("different model", "model"), Entry("different base", "base"), Entry("different policy limits", "policy"), Entry("different HMAC key", "key"))
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:41
	It("rejects tampered cached findings and obtains a fresh real response", func() {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"a.go"}, 2))
		service := batchServer(cwd)
		cache := filepath.Join(cwd, "cache")
		result := batchRun(root, cwd, filepath.Join(cwd, "first"), service, []string{"--cache", cache}, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		changed := 0
		Expect(filepath.WalkDir(cache, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if bytes.Contains(data, []byte("Safe findings")) {
				data = bytes.ReplaceAll(data, []byte("Safe findings"), []byte("Forged review"))
				if err = os.WriteFile(path, data, 0600); err != nil {
					return err
				}
				changed++
			}
			return nil
		})).To(Succeed())
		Expect(changed).To(BeNumerically(">", 0))
		out := filepath.Join(cwd, "second")
		result = batchRun(root, cwd, out, service, []string{"--cache", cache}, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(service.observed()).To(HaveLen(2))
		report := batchReadReport(out)
		Expect(batchCounts(report)["batches_cached"]).To(Equal(float64(0)))
		data, err := os.ReadFile(filepath.Join(out, "review.md"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).NotTo(ContainSubstring("Forged review"))
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:43
	It("does not cache incomplete streams as successful review coverage", func() {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"a.go"}, 2))
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, n int) {
			if n == 1 {
				_, _ = io.WriteString(w, strings.Split(reviewComplete, "data: [DONE]")[0])
				return
			}
			_, _ = io.WriteString(w, reviewComplete)
		})
		cache := filepath.Join(cwd, "cache")
		first := batchRun(root, cwd, filepath.Join(cwd, "first"), service, []string{"--cache", cache}, nil)
		Expect(first.status).NotTo(Equal(0))
		out := filepath.Join(cwd, "second")
		second := batchRun(root, cwd, out, service, []string{"--cache", cache}, nil)
		Expect(second.status).To(Equal(0), second.stderr)
		Expect(service.observed()).To(HaveLen(2))
		Expect(batchCounts(batchReadReport(out))["batches_cached"]).To(Equal(float64(0)))
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:44
	It("reads immutable diff inputs without changing their bytes or permissions", func() {
		root, cwd := fixture()
		input := batchAdded([]string{"a.go"}, 2)
		diff, manifest := batchWriteInput(cwd, input)
		beforeManifest, err := os.ReadFile(manifest)
		Expect(err).NotTo(HaveOccurred())
		for _, path := range []string{diff, manifest} {
			Expect(os.Chmod(path, 0400)).To(Succeed())
		}
		service := batchServer(cwd)
		result := batchRun(root, cwd, filepath.Join(cwd, "out"), service, nil, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		for path, expected := range map[string][]byte{diff: []byte(input.diff), manifest: beforeManifest} {
			data, readErr := os.ReadFile(path)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(data).To(Equal(expected))
			info, statErr := os.Stat(path)
			Expect(statErr).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0400)))
		}
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:43
	It("does not trust a linked successful cache entry or change its external target", func() {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"a.go"}, 2))
		service := batchServer(cwd)
		cache := filepath.Join(cwd, "cache")
		result := batchRun(root, cwd, filepath.Join(cwd, "first"), service, []string{"--cache", cache}, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		var cacheEntry string
		Expect(filepath.WalkDir(cache, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if bytes.Contains(data, []byte("Safe findings")) {
				cacheEntry = path
			}
			return nil
		})).To(Succeed())
		Expect(cacheEntry).NotTo(BeEmpty())
		original, err := os.ReadFile(cacheEntry)
		Expect(err).NotTo(HaveOccurred())
		external := filepath.Join(cwd, "external-cache.json")
		writeFixture(external, original, 0600)
		Expect(os.Remove(cacheEntry)).To(Succeed())
		Expect(os.Symlink(external, cacheEntry)).To(Succeed())
		out := filepath.Join(cwd, "second")
		result = batchRun(root, cwd, out, service, []string{"--cache", cache}, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(service.observed()).To(HaveLen(2))
		Expect(batchCounts(batchReadReport(out))["batches_cached"]).To(Equal(float64(0)))
		retained, err := os.ReadFile(external)
		Expect(err).NotTo(HaveOccurred())
		Expect(retained).To(Equal(original))
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:76
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:54
	It("reports partial coverage when a later batch fails after one real completion", func() {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"a.go", "b.go"}, 45))
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, n int) {
			if n == 2 {
				_, _ = io.WriteString(w, strings.Split(reviewComplete, "data: [DONE]")[0])
				return
			}
			_, _ = io.WriteString(w, strings.Replace(reviewComplete, "data: [DONE]", "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":4,\"total_tokens\":14,\"cost\":0.25}}\n\ndata: [DONE]", 1))
		})
		out := filepath.Join(cwd, "out")
		result := batchRun(root, cwd, out, service, nil, nil)
		Expect(result.status).NotTo(Equal(0))
		report := batchReadReport(out)
		Expect(report["status"]).To(Equal("partial"))
		Expect(service.observed()).To(HaveLen(2))
		counts := batchCounts(report)
		Expect(counts["batches_completed"]).To(Equal(float64(1)))
		Expect(counts["fragments_reviewed"]).To(BeNumerically("<", counts["fragments_total"]))
		Expect(counts["files_reviewed"]).To(BeNumerically("<", counts["files_total"]))
		Expect(report["usage"]).To(BeNil(), "the interrupted attempted batch has unknown accounting")
		first := report["batches"].([]any)[0].(map[string]any)
		Expect(first["usage"].(map[string]any)["cost"]).To(Equal(0.25))
		// A refused reservation never starts generation and must not erase
		// accounting that is complete for every request actually attempted.
		limited := filepath.Join(cwd, "reservation-refused")
		result = batchRun(root, cwd, limited, service, nil, []string{"REVIEW_MAX_REQUESTS=1"})
		Expect(result.status).NotTo(Equal(0))
		limitedReport := batchReadReport(limited)
		Expect(limitedReport["status"]).To(Equal("partial"))
		Expect(batchBudget(limitedReport)["requests_used"]).To(Equal(float64(1)))
		Expect(limitedReport["usage"].(map[string]any)["cost"]).To(Equal(0.25))
		Expect(service.observed()).To(HaveLen(3))
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:49
	It("honors the total deadline during Retry-After instead of extending the run", func() {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"a.go"}, 1))
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(503)
		})
		out := filepath.Join(cwd, "out")
		started := time.Now()
		result := batchRun(root, cwd, out, service, nil, []string{"REVIEW_TIMEOUT_SECONDS=1", "REVIEW_HTTP_RETRIES=1"})
		Expect(result.status).NotTo(Equal(0))
		Expect(time.Since(started)).To(BeNumerically("<", 4*time.Second))
		Expect(service.observed()).To(HaveLen(1))
		Expect(batchReadReport(out)["status"]).To(Equal("failed"))
	})
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:89
	It("cancels a live request on SIGTERM and preserves an incomplete report", func() {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"a.go"}, 1))
		started := make(chan struct{}, 1)
		service := reviewFixture(cwd, func(w http.ResponseWriter, r *http.Request, _ int) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, ": waiting\n\n")
			w.(http.Flusher).Flush()
			started <- struct{}{}
			<-r.Context().Done()
		})
		out := filepath.Join(cwd, "out")
		cmd := batchCommand(root, cwd, out, service, nil, []string{"REVIEW_TIMEOUT_SECONDS=5"})
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		Expect(cmd.Start()).To(Succeed())
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			Fail("compiled review-batch never reached test-owned model service")
		}
		Expect(cmd.Process.Signal(syscall.SIGTERM)).To(Succeed())
		select {
		case err := <-done:
			Expect(err).To(HaveOccurred())
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			Fail("SIGTERM did not cancel the live review request")
		}
		Expect(service.observed()).To(HaveLen(1))
		Expect(batchReadReport(out)["status"]).To(Equal("failed"))
		Expect(stderr.String()).NotTo(ContainSubstring("PRIVATE_BATCH_KEY"))
	})
})
