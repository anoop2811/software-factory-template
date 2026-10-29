package acceptance_test

import (
	"crypto/sha1" // #nosec G505 -- Git blob identity, not authentication; per docs/adr/0109-complete-large-review-inputs.md:60.
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const largeReviewOldTree = "4444444444444444444444444444444444444444"
const largeReviewNewTree = "5555555555555555555555555555555555555555"

type largeReviewBlob struct {
	data       []byte
	mode, kind string
}
type largeReviewFile struct {
	Filename string  `json:"filename"`
	Previous string  `json:"previous_filename,omitempty"`
	Status   string  `json:"status"`
	SHA      string  `json:"sha"`
	Patch    *string `json:"patch,omitempty"`
	Added    int     `json:"additions"`
	Removed  int     `json:"deletions"`
	RawURL   string  `json:"raw_url"`
}
type largeReviewScenario struct {
	files                      []largeReviewFile
	old, current               map[string]largeReviewBlob
	diffStatus                 int
	diffBody                   string
	headChangeAt, baseChangeAt int
	transform                  func(string, map[string]any)
}

func largeReviewBlobID(data []byte) string {
	framed := append([]byte(fmt.Sprintf("blob %d\x00", len(data))), data...)
	digest := sha1.Sum(framed) // #nosec G401 -- required Git object identity; not a cryptographic trust assertion.
	return fmt.Sprintf("%x", digest)
}

func largeReviewTree(files map[string]largeReviewBlob, identity string) map[string]any {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	entries := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		file := files[path]
		kind := file.kind
		if kind == "" {
			kind = "blob"
		}
		entries = append(entries, map[string]any{"path": path, "mode": file.mode, "type": kind, "sha": largeReviewBlobID(file.data), "size": len(file.data), "url": "https://untrusted.invalid/never-follow"})
	}
	return map[string]any{"sha": identity, "truncated": false, "tree": entries}
}

// Only authenticated production destinations reach this inert TLS responder.
// per docs/adr/0109-complete-large-review-inputs.md:29
func largeReviewFixture(cwd string, scenario largeReviewScenario) *ciReviewService {
	GinkgoHelper()
	if scenario.diffStatus == 0 {
		scenario.diffStatus = http.StatusNotAcceptable
	}
	service := &ciReviewService{}
	service.proxy, service.ca = reviewTLSProxy(cwd, []string{"api.github.com", "openrouter.ai"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "fixture read failure", http.StatusBadRequest)
			return
		}
		service.mu.Lock()
		service.requests = append(service.requests, ciReviewRequest{r.Host, r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), r.Header.Get("Accept"), body})
		if r.Host == "api.github.com" && r.Method == http.MethodGet && r.URL.Path == "/repos/fixture/project/pulls/7" && !strings.Contains(r.Header.Get("Accept"), "diff") {
			service.metadata++
		}
		metadata := service.metadata
		service.mu.Unlock()
		if r.Host == "openrouter.ai" {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, reviewComplete)
			return
		}
		const prefix = "/repos/fixture/project"
		path := strings.TrimPrefix(r.URL.Path, prefix)
		if path == "/pulls/7" && strings.Contains(r.Header.Get("Accept"), "diff") {
			body := scenario.diffBody
			if body == "" {
				body = "PRIVATE_GITHUB_BODY the diff exceeded the maximum number of lines (20000)"
			}
			http.Error(w, body, scenario.diffStatus)
			return
		}
		var response map[string]any
		switch {
		case path == "":
			response = map[string]any{"full_name": "fixture/project", "default_branch": "main"}
		case path == "/pulls/7":
			head, base := ciReviewHead, ciReviewBase
			if scenario.headChangeAt > 0 && metadata >= scenario.headChangeAt {
				head = ciReviewOther
			}
			if scenario.baseChangeAt > 0 && metadata >= scenario.baseChangeAt {
				base = ciReviewOther
			}
			response = map[string]any{"number": 7, "state": "open", "changed_files": len(scenario.files), "base": map[string]any{"sha": base, "ref": "main", "repo": map[string]any{"full_name": "fixture/project"}}, "head": map[string]any{"sha": head, "repo": map[string]any{"full_name": "fixture/project"}}}
		case path == "/pulls/7/files":
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page < 1 {
				http.Error(w, "missing fixture page", http.StatusBadRequest)
				return
			}
			start, end := min((page-1)*100, len(scenario.files)), min(page*100, len(scenario.files))
			_ = json.NewEncoder(w).Encode(scenario.files[start:end])
			return
		case path == "/compare/"+ciReviewBase+"..."+ciReviewHead:
			response = map[string]any{"base_commit": map[string]string{"sha": ciReviewBase}, "merge_base_commit": map[string]string{"sha": ciReviewOther}}
		case path == "/git/commits/"+ciReviewOther:
			response = map[string]any{"sha": ciReviewOther, "tree": map[string]string{"sha": largeReviewOldTree}}
		case path == "/git/commits/"+ciReviewHead:
			response = map[string]any{"sha": ciReviewHead, "tree": map[string]string{"sha": largeReviewNewTree}}
		case path == "/git/trees/"+largeReviewOldTree:
			response = largeReviewTree(scenario.old, largeReviewOldTree)
		case path == "/git/trees/"+largeReviewNewTree:
			response = largeReviewTree(scenario.current, largeReviewNewTree)
		case strings.HasPrefix(path, "/git/blobs/"):
			for _, file := range scenario.old {
				if path == "/git/blobs/"+largeReviewBlobID(file.data) {
					response = map[string]any{"sha": largeReviewBlobID(file.data), "size": len(file.data), "encoding": "base64", "content": base64.StdEncoding.EncodeToString(file.data)}
					break
				}
			}
		case path == "/check-runs" && r.Method == http.MethodPost, path == "/check-runs/741" && r.Method == http.MethodPatch:
			response = map[string]any{"id": 741}
		case path == "/issues/7/comments" && r.Method == http.MethodPost:
			response = map[string]any{"id": 742}
		}
		if response == nil {
			http.Error(w, "unexpected fixture endpoint", http.StatusNotFound)
			return
		}
		if scenario.transform != nil {
			scenario.transform(path, response)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	return service
}

func largeReviewAdded(lines int) largeReviewScenario {
	var patch, content strings.Builder
	fmt.Fprintf(&patch, "@@ -0,0 +1,%d @@\n", lines)
	for i := 0; i < lines; i++ {
		line := fmt.Sprintf("line%05d", i)
		fmt.Fprintf(&patch, "+%s\n", line)
		fmt.Fprintln(&content, line)
	}
	text := patch.String()
	path := "scripts/space \"quoted\" $(touch NEVER_EXECUTED).sh"
	data := []byte(content.String())
	return largeReviewScenario{files: []largeReviewFile{{Filename: path, Status: "added", SHA: largeReviewBlobID(data), Patch: &text, Added: lines, RawURL: "https://untrusted.invalid/never-follow"}}, old: map[string]largeReviewBlob{}, current: map[string]largeReviewBlob{path: {data: data, mode: "100755"}}}
}

func largeReviewModified() largeReviewScenario {
	patch := "@@ -1,2 +1,2 @@\n-one\n+two\n keep"
	old, current := []byte("one\nkeep\n"), []byte("two\nkeep\n")
	return largeReviewScenario{files: []largeReviewFile{{Filename: "src/item.txt", Status: "modified", SHA: largeReviewBlobID(current), Patch: &patch, Added: 1, Removed: 1}}, old: map[string]largeReviewBlob{"src/item.txt": {data: old, mode: "100644"}}, current: map[string]largeReviewBlob{"src/item.txt": {data: current, mode: "100644"}}}
}

func largeReviewMessages(service *ciReviewService) string {
	GinkgoHelper()
	var sent strings.Builder
	for _, request := range ciReviewRequests(service, "openrouter.ai", http.MethodPost, "/api/v1/chat/completions") {
		for _, value := range ciReviewJSON(request)["messages"].([]any) {
			sent.WriteString(value.(map[string]any)["content"].(string))
		}
	}
	return sent.String()
}

func largeReviewRefused(scenario largeReviewScenario, diagnostics ...string) *ciReviewService {
	GinkgoHelper()
	root, cwd := fixture()
	service := largeReviewFixture(cwd, scenario)
	result := ciReviewRun(root, cwd, filepath.Join(cwd, "refused-large-review"), service, nil, nil)
	Expect(result.status).NotTo(BeZero(), "%+v", result)
	Expect(result.stderr).NotTo(ContainSubstring("PRIVATE"))
	for _, diagnostic := range diagnostics {
		Expect(result.stderr).To(ContainSubstring(diagnostic))
	}
	Expect(ciReviewRequests(service, "openrouter.ai", http.MethodPost, "/api/v1/chat/completions")).To(BeEmpty(), "all acquisition evidence must qualify before spending")
	Expect(ciReviewRequests(service, "api.github.com", http.MethodPost, "/repos/fixture/project/issues/7/comments")).To(BeEmpty())
	Expect(ciReviewRequests(service, "api.github.com", http.MethodPatch, "/repos/fixture/project/check-runs/741")).To(HaveLen(1))
	ciReviewNoSuccess(service)
	return service
}

func largeReviewMixed() largeReviewScenario {
	scenario := largeReviewAdded(1)
	add := func(path, previous, status string, old, current []byte, oldMode, newMode string, patch *string, additions, deletions int) {
		oldPath := path
		if previous != "" {
			oldPath = previous
		}
		if oldMode != "" {
			scenario.old[oldPath] = largeReviewBlob{data: old, mode: oldMode}
		}
		if newMode != "" {
			scenario.current[path] = largeReviewBlob{data: current, mode: newMode}
		}
		sha := largeReviewBlobID(current)
		if status == "removed" {
			sha = largeReviewBlobID(old)
		}
		scenario.files = append(scenario.files, largeReviewFile{Filename: path, Previous: previous, Status: status, SHA: sha, Patch: patch, Added: additions, Removed: deletions})
	}
	crlf := "@@ -1,2 +1,2 @@\n-before\r\n+after\r\n keep\r"
	add("src/naïve \"quoted\".txt", "", "modified", []byte("before\r\nkeep\r\n"), []byte("after\r\nkeep\r\n"), "100644", "100644", &crlf, 1, 1)
	noNewline := "@@ -1 +1 @@\n-old\n\\ No newline at end of file\n+new\n\\ No newline at end of file"
	add("no-final-newline.txt", "", "modified", []byte("old"), []byte("new"), "100644", "100644", &noNewline, 1, 1)
	add("bin/tool", "", "modified", []byte("tool\n"), []byte("tool\n"), "100644", "100755", nil, 0, 0)
	add("new name\\path.txt", "old name\\path.txt", "renamed", []byte("rename\n"), []byte("rename\n"), "100644", "100644", nil, 0, 0)
	add("copy.txt", "source.txt", "copied", []byte("copy\n"), []byte("copy\n"), "100644", "100644", nil, 0, 0)
	scenario.current["source.txt"] = scenario.old["source.txt"]
	add("empty-new.txt", "", "added", nil, nil, "", "100644", nil, 0, 0)
	add("empty-old.txt", "", "removed", nil, nil, "100644", "", nil, 0, 0)
	removed := "@@ -1 +0,0 @@\n-gone"
	add("gone.txt", "", "removed", []byte("gone\n"), nil, "100644", "", &removed, 0, 1)
	return scenario
}

var _ = Describe("Hosted budgeted adversarial review complete large diff acquisition", func() {
	// per docs/adr/0109-complete-large-review-inputs.md:72
	It("reviews every added line beyond GitHub's 20000-line raw-diff refusal using exact tree identity", func() {
		root, cwd := fixture()
		scenario := largeReviewAdded(20001)
		service := largeReviewFixture(cwd, scenario)
		out := filepath.Join(cwd, "large-review")
		result := ciReviewRun(root, cwd, out, service, nil, nil)
		Expect(result.status).To(BeZero(), "%+v", result)
		requests := ciReviewRequests(service, "openrouter.ai", http.MethodPost, "/api/v1/chat/completions")
		Expect(len(requests)).To(BeNumerically(">", 1), "large content must be bounded into real batches")
		var sent strings.Builder
		for _, request := range requests {
			body := ciReviewJSON(request)
			for _, value := range body["messages"].([]any) {
				sent.WriteString(value.(map[string]any)["content"].(string))
			}
		}
		var actual []string
		expected := make([]string, 20001)
		for i := range expected {
			expected[i] = fmt.Sprintf("+line%05d", i)
		}
		for line := range strings.SplitSeq(sent.String(), "\n") {
			if strings.HasPrefix(line, "+line") {
				actual = append(actual, line)
			}
		}
		Expect(actual).To(Equal(expected), "every unique added line must reach the model exactly once and in order")
		Expect(sent.String()).To(ContainSubstring("new file mode 100755"))
		completed := ciReviewRequests(service, "api.github.com", http.MethodPatch, "/repos/fixture/project/check-runs/741")
		Expect(completed).To(HaveLen(1))
		Expect(ciReviewJSON(completed[0])["conclusion"]).To(Equal("success"))
		for _, request := range service.observed() {
			if request.host == "api.github.com" {
				Expect(request.authorization).To(Equal("Bearer PRIVATE_CI_GITHUB_KEY"))
			} else {
				Expect(request.authorization).To(Equal("Bearer PRIVATE_CI_REVIEW_KEY"))
			}
		}
		_, err := os.Lstat(filepath.Join(cwd, "NEVER_EXECUTED"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0109-complete-large-review-inputs.md:40
	// per docs/adr/0109-complete-large-review-inputs.md:47
	// per docs/adr/0109-complete-large-review-inputs.md:59
	It("preserves proven lifecycle modes quoted names CRLF and absent final newlines", func() {
		root, cwd := fixture()
		scenario := largeReviewMixed()
		service := largeReviewFixture(cwd, scenario)
		out := filepath.Join(cwd, "mixed-review")
		result := ciReviewRun(root, cwd, out, service, nil, nil)
		Expect(result.status).To(BeZero(), "%+v", result)
		sent := largeReviewMessages(service)
		for _, fragment := range []string{"old mode 100644\nnew mode 100755", "rename from", "rename to", "copy from", "copy to", "deleted file mode 100644", "+after\r\n", "\\ No newline at end of file", "empty-new.txt", "empty-old.txt", "old name\\\\path.txt", "naïve"} {
			Expect(sent).To(ContainSubstring(fragment))
		}
		data, err := os.ReadFile(filepath.Join(out, "report.json"))
		Expect(err).NotTo(HaveOccurred())
		var report map[string]any
		Expect(json.Unmarshal(data, &report)).To(Succeed())
		Expect(report["status"]).To(Equal("complete"))
		Expect(report["base"]).To(Equal(ciReviewBase), "comparison merge base must not replace current PR identity")
		Expect(report["head"]).To(Equal(ciReviewHead))
		Expect(report["counts"].(map[string]any)["files_reviewed"]).To(Equal(float64(len(scenario.files))))
		Expect(ciReviewRequests(service, "api.github.com", http.MethodGet, "/repos/fixture/project/git/commits/"+ciReviewBase)).To(BeEmpty(), "old context must use the selected merge base, not current main")
	})
	// per docs/adr/0109-complete-large-review-inputs.md:91
	DescribeTable("refuses lifecycle claims that omit or contradict complete tree changes", func(kind string) {
		scenario := largeReviewMixed()
		switch kind {
		case "retained rename source":
			scenario.current["old name\\path.txt"] = scenario.old["old name\\path.txt"]
		case "deleted copy source":
			delete(scenario.current, "source.txt")
		case "overwritten copy destination":
			scenario.old["copy.txt"] = largeReviewBlob{data: []byte("old target bytes\n"), mode: "100644"}
		case "unexplained added path":
			scenario.current["unlisted.txt"] = largeReviewBlob{data: []byte("added\n"), mode: "100644"}
		case "unexplained removed path":
			scenario.old["unlisted.txt"] = largeReviewBlob{data: []byte("removed\n"), mode: "100644"}
		case "unexplained modified path":
			scenario.old["unlisted.txt"] = largeReviewBlob{data: []byte("before\n"), mode: "100644"}
			scenario.current["unlisted.txt"] = largeReviewBlob{data: []byte("after\n"), mode: "100644"}
		}
		largeReviewRefused(scenario)
	}, Entry("rename source remains unchanged without its own row", "retained rename source"), Entry("copy source disappears without its own row", "deleted copy source"), Entry("copy destination overwrites an unreviewed old blob", "overwritten copy destination"), Entry("regular addition outside the manifest", "unexplained added path"), Entry("regular deletion outside the manifest", "unexplained removed path"), Entry("regular modification outside the manifest", "unexplained modified path"))
	// per docs/adr/0109-complete-large-review-inputs.md:93
	It("accepts a copy source deletion explained by an explicit removed row", func() {
		scenario := largeReviewMixed()
		delete(scenario.current, "source.txt")
		patch := "@@ -1 +0,0 @@\n-copy"
		scenario.files = append(scenario.files, largeReviewFile{Filename: "source.txt", Status: "removed", SHA: largeReviewBlobID([]byte("copy\n")), Patch: &patch, Removed: 1})
		root, cwd := fixture()
		service := largeReviewFixture(cwd, scenario)
		result := ciReviewRun(root, cwd, filepath.Join(cwd, "explained-source-deletion"), service, nil, nil)
		Expect(result.status).To(BeZero(), "%+v", result)
		Expect(largeReviewMessages(service)).To(ContainSubstring("-copy\n"))
		completed := ciReviewRequests(service, "api.github.com", http.MethodPatch, "/repos/fixture/project/check-runs/741")
		Expect(completed).To(HaveLen(1))
		Expect(ciReviewJSON(completed[0])["conclusion"]).To(Equal("success"))
	})
	// per docs/adr/0109-complete-large-review-inputs.md:94
	It("accepts rename and copy source modifications explained by explicit modified rows", func() {
		for _, source := range []string{"old name\\path.txt", "source.txt"} {
			scenario := largeReviewMixed()
			old := scenario.old[source]
			current := []byte("updated source\n")
			scenario.current[source] = largeReviewBlob{data: current, mode: old.mode}
			patch := "@@ -1 +1 @@\n-" + strings.TrimSuffix(string(old.data), "\n") + "\n+updated source"
			scenario.files = append(scenario.files, largeReviewFile{Filename: source, Status: "modified", SHA: largeReviewBlobID(current), Patch: &patch, Added: 1, Removed: 1})
			root, cwd := fixture()
			service := largeReviewFixture(cwd, scenario)
			result := ciReviewRun(root, cwd, filepath.Join(cwd, "explained-source-modification"), service, nil, nil)
			Expect(result.status).To(BeZero(), "source %q: %+v", source, result)
			Expect(largeReviewMessages(service)).To(ContainSubstring("+updated source\n"))
			completed := ciReviewRequests(service, "api.github.com", http.MethodPatch, "/repos/fixture/project/check-runs/741")
			Expect(completed).To(HaveLen(1))
			Expect(ciReviewJSON(completed[0])["conclusion"]).To(Equal("success"))
		}
	})
	// per docs/adr/0109-complete-large-review-inputs.md:29
	It("reconstructs the final changed file from a complete paginated manifest", func() {
		scenario := largeReviewScenario{old: map[string]largeReviewBlob{}, current: map[string]largeReviewBlob{}}
		for i := 0; i < 101; i++ {
			path, text := fmt.Sprintf("src/file%03d.txt", i), fmt.Sprintf("file%03d\n", i)
			patch := "@@ -0,0 +1 @@\n+" + strings.TrimSuffix(text, "\n")
			scenario.files = append(scenario.files, largeReviewFile{Filename: path, Status: "added", SHA: largeReviewBlobID([]byte(text)), Patch: &patch, Added: 1})
			scenario.current[path] = largeReviewBlob{data: []byte(text), mode: "100644"}
		}
		root, cwd := fixture()
		service := largeReviewFixture(cwd, scenario)
		result := ciReviewRun(root, cwd, filepath.Join(cwd, "paginated-review"), service, nil, nil)
		Expect(result.status).To(BeZero(), "%+v", result)
		pages := ciReviewRequests(service, "api.github.com", http.MethodGet, "/repos/fixture/project/pulls/7/files")
		Expect(pages).To(HaveLen(2))
		Expect(pages[1].query).To(Equal("per_page=100&page=2"))
		Expect(largeReviewMessages(service)).To(ContainSubstring("+file100\n"))
	})
	// per docs/adr/0109-complete-large-review-inputs.md:40
	// per docs/adr/0109-complete-large-review-inputs.md:50
	// per docs/adr/0109-complete-large-review-inputs.md:55
	DescribeTable("refuses incomplete or incompatible fallback evidence before the first model request", func(kind string) {
		scenario := largeReviewModified()
		file := &scenario.files[0]
		if kind == "wrong old blob bytes" {
			scenario.old[file.Filename] = largeReviewBlob{data: []byte("one\nkeep\ntailA\n"), mode: "100644"}
			scenario.current[file.Filename] = largeReviewBlob{data: []byte("two\nkeep\ntailB\n"), mode: "100644"}
			file.SHA = largeReviewBlobID(scenario.current[file.Filename].data)
		}
		switch kind {
		case "missing patch":
			file.Patch = nil
		case "truncated hunk":
			patch := "@@ -1,2 +1,2 @@\n-one\n+two"
			file.Patch = &patch
		case "statistics":
			file.Added++
		case "context":
			patch := strings.Replace(*file.Patch, " keep", " WRONG", 1)
			file.Patch = &patch
			scenario.current[file.Filename] = largeReviewBlob{data: []byte("two\nWRONG\n"), mode: "100644"}
			file.SHA = largeReviewBlobID(scenario.current[file.Filename].data)
		case "removed bytes":
			patch := strings.Replace(*file.Patch, "-one", "-bad", 1)
			file.Patch = &patch
		case "result identity":
			scenario.current[file.Filename] = largeReviewBlob{data: []byte("bad\nkeep\n"), mode: "100644"}
			file.SHA = largeReviewBlobID(scenario.current[file.Filename].data)
		case "new coordinates":
			patch := strings.Replace(*file.Patch, "+1,2", "+2,2", 1)
			file.Patch = &patch
		case "overlap":
			patch := *file.Patch + "\n" + *file.Patch
			file.Patch, file.Added, file.Removed = &patch, 2, 2
		case "missing newline marker":
			scenario.old[file.Filename] = largeReviewBlob{data: []byte("one\nkeep"), mode: "100644"}
		case "unexplained zero-content change":
			file.Patch, file.Added, file.Removed = nil, 0, 0
		case "rename missing previous":
			file.Status = "renamed"
		case "rename missing old path":
			file.Status, file.Previous = "renamed", "absent-old.txt"
		case "unsafe path":
			old, current := scenario.old[file.Filename], scenario.current[file.Filename]
			delete(scenario.old, file.Filename)
			delete(scenario.current, file.Filename)
			file.Filename = "../escape.txt"
			scenario.old[file.Filename], scenario.current[file.Filename] = old, current
		case "symlink mode":
			current := scenario.current[file.Filename]
			current.mode = "120000"
			scenario.current[file.Filename] = current
		case "gitlink type":
			current := scenario.current[file.Filename]
			current.mode, current.kind = "160000", "commit"
			scenario.current[file.Filename] = current
		case "binary blob":
			scenario.old[file.Filename] = largeReviewBlob{data: []byte{'x', 0, '\n'}, mode: "100644"}
			scenario.current[file.Filename] = largeReviewBlob{data: []byte("two\n"), mode: "100644"}
			patch := "@@ -1 +1 @@\n-x\x00\n+two"
			file.Patch, file.SHA = &patch, largeReviewBlobID([]byte("two\n"))
		case "manifest blob identity":
			file.SHA = ciReviewOther
		default:
			scenario.transform = func(path string, response map[string]any) {
				switch {
				case path == "/git/trees/"+largeReviewNewTree:
					switch kind {
					case "truncated tree":
						response["truncated"] = true
					case "missing truncation flag":
						delete(response, "truncated")
					case "wrong tree identity":
						response["sha"] = ciReviewOther
					case "null tree":
						response["tree"] = nil
					case "missing head path":
						response["tree"] = []map[string]any{}
					case "duplicate tree path":
						entries := response["tree"].([]map[string]any)
						response["tree"] = append(entries, entries[0])
					}
				case strings.HasPrefix(path, "/compare/") && kind == "wrong compared base":
					response["base_commit"] = map[string]string{"sha": ciReviewOther}
				case path == "/git/commits/"+ciReviewOther && kind == "wrong commit identity":
					response["sha"] = ciReviewBase
				case strings.HasPrefix(path, "/git/blobs/"):
					switch kind {
					case "wrong old blob bytes":
						response["content"] = base64.StdEncoding.EncodeToString([]byte("one\nkeep\ntailB\n"))
					case "wrong decoded size":
						response["size"] = response["size"].(int) + 1
					case "invalid base64":
						response["content"] = "PRIVATE_INVALID_BASE64"
					}
				}
			}
		}
		largeReviewRefused(scenario)
	}, Entry("missing text patch", "missing patch"), Entry("truncated hunk despite complete tree", "truncated hunk"), Entry("independent additions disagree", "statistics"), Entry("cached patch context differs with matching counts", "context"), Entry("cached patch removal differs with matching counts", "removed bytes"), Entry("applied result differs from actual head object", "result identity"), Entry("new hunk coordinates contradict the produced prefix", "new coordinates"), Entry("overlapping hunks", "overlap"), Entry("unmarked absent old final newline", "missing newline marker"), Entry("missing patch cannot explain changed blob IDs", "unexplained zero-content change"), Entry("rename requires previous filename", "rename missing previous"), Entry("rename requires proven old path", "rename missing old path"), Entry("traversal path", "unsafe path"), Entry("symlink type change", "symlink mode"), Entry("submodule type change", "gitlink type"), Entry("binary old blob", "binary blob"), Entry("manifest SHA contradicts head tree", "manifest blob identity"), Entry("explicitly truncated tree", "truncated tree"), Entry("absent truncation evidence", "missing truncation flag"), Entry("wrong tree identity", "wrong tree identity"), Entry("null tree", "null tree"), Entry("tree omits changed path", "missing head path"), Entry("duplicate tree path", "duplicate tree path"), Entry("comparison is not for bound base", "wrong compared base"), Entry("wrong exact commit identity", "wrong commit identity"), Entry("decoded old bytes fail Git object identity", "wrong old blob bytes"), Entry("decoded size disagrees", "wrong decoded size"), Entry("invalid base64 is private", "invalid base64"))
	// per docs/adr/0109-complete-large-review-inputs.md:24
	DescribeTable("does not acquire weaker inputs after a non406 raw-diff failure", func(status int, oversized bool) {
		scenario := largeReviewAdded(1)
		scenario.diffStatus = status
		if oversized {
			scenario.diffBody = strings.Repeat("x", (8<<20)+1)
		}
		service := largeReviewRefused(scenario)
		for _, request := range service.observed() {
			Expect(request.path).NotTo(ContainSubstring("/compare/"))
			Expect(request.path).NotTo(ContainSubstring("/git/"))
		}
	}, Entry("authentication", http.StatusUnauthorized, false), Entry("permission", http.StatusForbidden, false), Entry("rate limit", http.StatusTooManyRequests, false), Entry("server error", http.StatusInternalServerError, false), Entry("oversized successful response", http.StatusOK, true))
	// per docs/adr/0109-complete-large-review-inputs.md:63
	DescribeTable("bounds decoded old source and combined reconstructed inputs before spending", func(sourceBytes bool) {
		scenario := largeReviewScenario{old: map[string]largeReviewBlob{}, current: map[string]largeReviewBlob{}}
		if sourceBytes {
			for _, name := range []string{"a", "b"} {
				old := []byte("old" + name + "\n" + strings.Repeat("x", 4500000) + "\n")
				current := []byte("new" + name + "\n" + strings.Repeat("x", 4500000) + "\n")
				patch := "@@ -1 +1 @@\n-old" + name + "\n+new" + name
				scenario.files = append(scenario.files, largeReviewFile{Filename: name + ".txt", Status: "modified", SHA: largeReviewBlobID(current), Patch: &patch, Added: 1, Removed: 1})
				scenario.old[name+".txt"], scenario.current[name+".txt"] = largeReviewBlob{data: old, mode: "100644"}, largeReviewBlob{data: current, mode: "100644"}
			}
		} else {
			content := strings.Repeat("x", 4500000) + "\n"
			patch := "@@ -0,0 +1 @@\n+" + strings.TrimSuffix(content, "\n")
			scenario.files = []largeReviewFile{{Filename: "large.txt", Status: "added", SHA: largeReviewBlobID([]byte(content)), Patch: &patch, Added: 1}}
			scenario.current["large.txt"] = largeReviewBlob{data: []byte(content), mode: "100644"}
		}
		largeReviewRefused(scenario, "oversized")
	}, Entry("aggregate decoded old blobs", true), Entry("manifest plus assembled diff", false))
	// per docs/adr/0109-complete-large-review-inputs.md:66
	DescribeTable("rechecks current revisions after fallback preparation before model spending", func(head bool) {
		scenario := largeReviewModified()
		if head {
			scenario.headChangeAt = 2
		} else {
			scenario.baseChangeAt = 2
		}
		largeReviewRefused(scenario)
	}, Entry("head changed", true), Entry("base changed", false))
	// per docs/adr/0109-complete-large-review-inputs.md:66
	It("withholds fallback findings and completion when the head changes during generation", func() {
		root, cwd := fixture()
		scenario := largeReviewModified()
		scenario.headChangeAt = 3
		service := largeReviewFixture(cwd, scenario)
		result := ciReviewRun(root, cwd, filepath.Join(cwd, "stale-generated-review"), service, nil, nil)
		Expect(result.status).NotTo(BeZero())
		Expect(ciReviewRequests(service, "openrouter.ai", http.MethodPost, "/api/v1/chat/completions")).To(HaveLen(1))
		Expect(ciReviewRequests(service, "api.github.com", http.MethodPost, "/repos/fixture/project/issues/7/comments")).To(BeEmpty())
		ciReviewNoSuccess(service)
	})
})
