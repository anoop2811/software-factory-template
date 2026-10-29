package acceptance_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Adversarial review large payload boundary", func() {
	// per docs/DECISION_LOG.md:2478
	// per docs/adr/0099-native-factory-orchestration.md:120
	DescribeTable("carries the complete diff without payload-sized argv or private output remnants", func(provider string, selected, failed bool) {
		diff := strings.Repeat("+ payload \"quoted\" \\slash\tline\n", 6200) + "+ FINAL_FULL_DIFF_SENTINEL"
		Expect(len(diff)).To(BeNumerically(">", 128<<10))
		Expect(len(diff)).To(BeNumerically("<", 200000))
		root, cwd, path, environment := orchestrationReviewFixture(diff)
		private := filepath.Join(cwd, "private-temp")
		writeFixture(filepath.Join(private, "unrelated"), []byte("preserve sibling"), 0600)
		service := orchestrationReviewHTTP(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
			if failed {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"PRIVATE_PARTIAL"}}]}`)
			} else if provider == "anthropic" {
				_, _ = io.WriteString(w, `{"content":[{"text":"No findings."}]}`)
			} else {
				_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"No findings."}}]}`)
			}
		})
		environment = orchestrationReviewEnvironment(environment, service, "MODEL_PROVIDER="+provider, "REVIEW_MODEL=fixture-model", "REVIEW_REASONING_EFFORT=low", "REVIEW_OPENROUTER_PROVIDER=deepinfra", "TMPDIR="+private)
		capture := filepath.Join(cwd, "selected-request")
		if selected {
			client := filepath.Join(cwd, "selected client")
			writeFixture(client, []byte("#!/bin/sh\numask 077\nfor arg do [ \"${#arg}\" -le 131071 ] || exit 90; done\n/bin/cat > \"$PAYLOAD_CAPTURE\"\n[ \"${PAYLOAD_FAIL-}\" != 1 ] || exit 73\nprintf 'No findings.\\n'\n"), 0700)
			environment = append(environment, "REVIEW_GO_CLIENT="+client, "PAYLOAD_CAPTURE="+capture)
			if failed {
				environment = append(environment, "PAYLOAD_FAIL=1")
			}
		}
		result := orchestrationReview(root, cwd, environment, path)
		if failed {
			Expect(result.status).NotTo(BeZero())
			Expect(result.stdout).To(BeEmpty())
			Expect(result.stderr).NotTo(ContainSubstring("PRIVATE_PARTIAL"))
		} else {
			Expect(result).To(Equal(cliResult{"No findings.\n", "", 0}))
		}
		var raw []byte
		if selected {
			Expect(service.observed()).To(BeEmpty(), "selected client must not trigger secondary HTTP")
			var err error
			raw, err = os.ReadFile(capture)
			Expect(err).NotTo(HaveOccurred())
			info, err := os.Stat(capture)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
		} else {
			requests := service.observed()
			Expect(requests).To(HaveLen(1))
			raw = requests[0].body
		}
		var body map[string]any
		Expect(json.Unmarshal(raw, &body)).To(Succeed())
		Expect(body["model"]).To(Equal("fixture-model"))
		messages := body["messages"].([]any)
		userIndex := 1
		switch provider {
		case "anthropic":
			userIndex = 0
			Expect(body["max_tokens"]).To(Equal(float64(4096)))
		case "openrouter":
			Expect(body["max_tokens"]).To(Equal(float64(8192)))
			Expect(body["reasoning"]).To(Equal(map[string]any{"effort": "low"}))
			Expect(body["provider"]).To(Equal(map[string]any{"order": []any{"deepinfra"}, "allow_fallbacks": false, "require_parameters": true}))
		default:
			Expect(body).NotTo(HaveKey("max_tokens"))
		}
		Expect(messages[userIndex].(map[string]any)["content"]).To(Equal("Review this diff.\n\n```diff\n" + diff + "\n```"))
		entries, err := os.ReadDir(private)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1), "native review must not retain provider payloads in private temporary storage")
		Expect(entries[0].Name()).To(Equal("unrelated"))
		preserved, err := os.ReadFile(filepath.Join(private, "unrelated"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(preserved)).To(Equal("preserve sibling"))
	}, Entry("Anthropic native HTTP", "anthropic", false, false), Entry("OpenAI native HTTP", "openai", false, false), Entry("OpenRouter native HTTP", "openrouter", false, false), Entry("selected Go client", "openrouter", true, false), Entry("native HTTP failure", "openrouter", false, true), Entry("selected client failure", "openrouter", true, true))
})
