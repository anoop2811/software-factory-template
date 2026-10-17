package acceptance_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const orchestrationSystemPrompt = `You are an adversarial code reviewer. Your job is to find what is wrong with a
change before a human spends attention on it. You are not the model that wrote
this code, and you are not here to be agreeable.

Review the diff for, in priority order:
1. Correctness — logic errors, off-by-one, wrong operator, unhandled nil/empty,
   a case the change silently breaks.
2. Security — injection, unsafe input handling, secrets in code, permissions
   widened, a path that trusts what it should not.
3. Invariants — behaviour the change breaks that callers depend on:
   idempotency, immutability, ordering, error contracts.
4. Confabulation — a comment, citation, or doc reference asserting something the
   diff does not support, or pointing at a file or line that may not exist.
5. Over-engineering — abstraction with one caller, generalisation for a future
   that has not arrived, code that a stdlib call would replace.

Rules you must follow:
- Report ONLY what you can point at in this diff. Cite ` + "`file:line`" + `.
- Do not summarise what the change does. The author knows. Findings only.
- Do not praise. "Looks good" is not a finding and wastes the reader.
- If a concern is speculative, label it **speculative** and say what would
  confirm it. Do not present a guess as a defect.
- If you find nothing worth a human's time, say exactly: "No findings." That is
  a valid, useful answer. Inventing a finding to appear thorough is the failure
  mode this review exists to avoid.
- You see only a diff, not the whole repository. Say so when it limits you
  rather than assuming the worst.

Format each finding as:
### <severity: critical | major | minor> — <file>:<line>
<what is wrong, and what would go wrong because of it>
**Fix:** <the smallest change that resolves it>`

type orchestrationReviewRequest struct {
	host, path, method string
	header             http.Header
	body               []byte
}
type orchestrationReviewService struct {
	proxy, ca string
	mu        sync.Mutex
	requests  []orchestrationReviewRequest
}

func orchestrationReviewHTTP(cwd string, respond func(http.ResponseWriter, *http.Request, int)) *orchestrationReviewService {
	GinkgoHelper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	certificate := &x509.Certificate{SerialNumber: big.NewInt(99), Subject: pkix.Name{CommonName: "test-owned orchestration providers"}, DNSNames: []string{"openrouter.ai", "api.openai.com", "api.anthropic.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	Expect(err).NotTo(HaveOccurred())
	service := &orchestrationReviewService{ca: filepath.Join(cwd, "orchestration-ca.pem")}
	writeFixture(service.ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600)
	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			http.Error(w, "fixture read failed", 500)
			return
		}
		service.mu.Lock()
		service.requests = append(service.requests, orchestrationReviewRequest{r.Host, r.URL.Path, r.Method, r.Header.Clone(), body})
		count := len(service.requests)
		service.mu.Unlock()
		respond(w, r, count)
	}))
	backend.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	backend.StartTLS()
	DeferCleanup(backend.Close)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || (r.Host != "openrouter.ai:443" && r.Host != "api.openai.com:443" && r.Host != "api.anthropic.com:443") {
			http.Error(w, "unexpected destination", 400)
			return
		}
		upstream, dialErr := net.DialTimeout("tcp", backend.Listener.Addr().String(), time.Second)
		if dialErr != nil {
			http.Error(w, "fixture unavailable", http.StatusBadGateway)
			return
		}
		client, _, hijackErr := w.(http.Hijacker).Hijack()
		if hijackErr != nil {
			_ = upstream.Close()
			return
		}
		defer client.Close()
		defer upstream.Close()
		if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		done := make(chan struct{})
		go func() { _, _ = io.Copy(upstream, client); _ = upstream.Close(); close(done) }()
		_, _ = io.Copy(client, upstream)
		_ = client.Close()
		<-done
	}))
	DeferCleanup(proxy.Close)
	service.proxy = proxy.URL
	return service
}

func (s *orchestrationReviewService) observed() []orchestrationReviewRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]orchestrationReviewRequest(nil), s.requests...)
}

func orchestrationReviewFixture(diff string) (string, string, string, []string) {
	GinkgoHelper()
	root, cwd := fixture()
	path := filepath.Join(cwd, "private $(touch injected) diff.patch")
	writeFixture(path, []byte(diff), 0600)
	writeFixture(filepath.Join(root, "factory.yaml"), []byte("review_lane: off\nmodel_provider: openrouter\nopencode_frontier_model: openrouter/fixture/frontier\n"), 0600)
	writeFixture(filepath.Join(root, "scripts/adversarial-review.sh"), []byte("#!/bin/sh\nprintf legacy-fallback >&2\nexit 98\n"), 0700)
	return root, cwd, path, []string{"PATH=/no-curl-or-jq", "HOME=" + cwd, "FACTORY_CONFIG=" + filepath.Join(root, "factory.yaml"), "GORACE=atexit_sleep_ms=0", "REVIEW_TIMEOUT_SECONDS=3", "REVIEW_HTTP_RETRIES=0"}
}

func orchestrationReview(root, cwd string, environment []string, args ...string) cliResult {
	return orchestrationCommand(root, cwd, environment, filepath.Join(root, "factory"), append([]string{"review"}, args...)...)
}

func orchestrationReviewEnvironment(environment []string, service *orchestrationReviewService, extra ...string) []string {
	environment = append(environment, "HTTPS_PROXY="+service.proxy, "SSL_CERT_FILE="+service.ca, "REVIEW_API_KEY=PRIVATE_API_KEY")
	return append(environment, extra...)
}

func orchestrationLegacyReview(root, cwd, diffPath string, environment []string, provider string) (cliResult, []byte) {
	GinkgoHelper()
	for _, path := range []string{"scripts/adversarial-review.sh", "scripts/lib/config.sh"} {
		writeFixture(filepath.Join(root, "frozen-review", path), orchestrationFrozen(path), 0700)
	}
	capture := filepath.Join(cwd, "legacy-request.json")
	// The immutable shell builds its own request. This transport only copies the
	// resulting private file and supplies a deterministic provider response.
	writeFixture(filepath.Join(root, "review-fixture-bin/curl"), []byte(`#!/bin/sh
while [ "$#" -gt 0 ]; do
 case "$1" in
 --output) shift; response="$1" ;;
 --data-binary) shift; /bin/cat "${1#@}" > "$LEGACY_REQUEST_CAPTURE" ;;
 esac
 shift
done
if [ "$LEGACY_PROVIDER" = anthropic ]; then
 printf '%s' '{"content":[{"text":"No findings."}]}' > "$response"
else
 printf '%s' '{"choices":[{"finish_reason":"stop","message":{"content":"No findings."}}]}' > "$response"
fi
printf '200\t0.1\t0.05\t100'
`), 0700)
	environment = append(environment, "PATH="+filepath.Join(root, "review-fixture-bin")+":/bin:/usr/bin:/opt/homebrew/bin", "LEGACY_REQUEST_CAPTURE="+capture, "LEGACY_PROVIDER="+provider)
	result := orchestrationCommand(root, cwd, environment, "/bin/bash", filepath.Join(root, "frozen-review/scripts/adversarial-review.sh"), diffPath, "ignored-extra-operand")
	data, err := os.ReadFile(capture)
	Expect(err).NotTo(HaveOccurred(), "immutable review transport: %+v", result)
	return result, data
}

var _ = Describe("Native orchestration review", func() {
	// per docs/adr/0099-native-factory-orchestration.md:120
	DescribeTable("preserves argument, key and empty-diff ordering without legacy tools", func(kind string, status int, stdout, diagnostic string) {
		root, cwd, path, environment := orchestrationReviewFixture(" \n\t\n")
		args := []string{path}
		switch kind {
		case "missing operand":
			args = nil
		case "missing file":
			args = []string{filepath.Join(cwd, "absent.patch")}
		case "empty with key":
			environment = append(environment, "REVIEW_API_KEY=PRIVATE_API_KEY")
		}
		result := orchestrationReview(root, cwd, environment, args...)
		Expect(result.status).To(Equal(status), result.stderr)
		Expect(result.stdout).To(Equal(stdout))
		if diagnostic != "" {
			Expect(result.stderr).To(ContainSubstring(diagnostic))
		} else {
			Expect(result.stderr).To(BeEmpty())
		}
	}, Entry("missing operand", "missing operand", 1, "", "usage:"), Entry("missing file", "missing file", 1, "", "usage:"), Entry("key is checked before empty diff", "missing key", 1, "", "no REVIEW_API_KEY in the environment"), Entry("empty diff with key", "empty with key", 0, "_No reviewable change in this diff._\n", ""))

	// per docs/adr/0099-native-factory-orchestration.md:120
	// per docs/adr/0099-native-factory-orchestration.md:125
	DescribeTable("sends faithful fixed-endpoint JSON shapes without curl or jq", func(provider, model, host, path string) {
		diff := "diff --git a/example.go b/example.go\n+PRIVATE_DIFF $(touch injected) \\\"quoted\\\"\n\n"
		root, cwd, diffPath, environment := orchestrationReviewFixture(diff)
		service := orchestrationReviewHTTP(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
			w.Header().Set("Content-Type", "application/json")
			if provider == "anthropic" {
				_, _ = io.WriteString(w, `{"content":[{"text":"No findings."}]}`)
			} else {
				_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"No findings."}}]}`)
			}
		})
		environment = orchestrationReviewEnvironment(environment, service, "MODEL_PROVIDER="+provider, "REVIEW_MODEL="+model, "REVIEW_REASONING_EFFORT=low", "REVIEW_OPENROUTER_PROVIDER=deepinfra/endpoint", "REVIEW_MAX_TOKENS=00032768")
		legacy, legacyBody := orchestrationLegacyReview(root, cwd, diffPath, environment, provider)
		Expect(legacy).To(Equal(cliResult{"No findings.\n", "", 0}))
		user := "Review this diff.\n\n```diff\n" + strings.TrimRight(diff, "\n") + "\n```"
		var expected map[string]any
		if provider == "anthropic" {
			expected = map[string]any{"model": model, "max_tokens": float64(4096), "system": orchestrationSystemPrompt, "messages": []any{map[string]any{"role": "user", "content": user}}}
		} else {
			expected = map[string]any{"model": model, "messages": []any{map[string]any{"role": "system", "content": orchestrationSystemPrompt}, map[string]any{"role": "user", "content": user}}}
			if provider == "openrouter" {
				expected["max_tokens"] = float64(32768)
				expected["reasoning"] = map[string]any{"effort": "low"}
				expected["provider"] = map[string]any{"order": []any{"deepinfra/endpoint"}, "allow_fallbacks": false, "require_parameters": true}
			}
		}
		var frozenBody map[string]any
		Expect(json.Unmarshal(legacyBody, &frozenBody)).To(Succeed())
		Expect(frozenBody).To(Equal(expected))
		result := orchestrationReview(root, cwd, environment, diffPath, "ignored-extra-operand")
		Expect(result).To(Equal(cliResult{"No findings.\n", "", 0}))
		requests := service.observed()
		Expect(requests).To(HaveLen(1))
		request := requests[0]
		Expect(request.host).To(Equal(host))
		Expect(request.path).To(Equal(path))
		Expect(request.method).To(Equal(http.MethodPost))
		Expect(request.header.Get("Content-Type")).To(ContainSubstring("application/json"))
		if provider == "anthropic" {
			Expect(request.header.Get("X-Api-Key")).To(Equal("PRIVATE_API_KEY"))
			Expect(request.header.Get("Anthropic-Version")).To(Equal("2023-06-01"))
		} else {
			Expect(request.header.Get("Authorization")).To(Equal("Bearer PRIVATE_API_KEY"))
		}
		var body map[string]any
		Expect(json.Unmarshal(request.body, &body)).To(Succeed())
		Expect(body).To(Equal(expected))
		_, err := os.Stat(filepath.Join(cwd, "injected"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		configuration, err := os.ReadFile(filepath.Join(root, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(configuration)).To(ContainSubstring("review_lane: off"))
	}, Entry("OpenRouter", "openrouter", "fixture/router", "openrouter.ai", "/api/v1/chat/completions"), Entry("OpenAI", "openai", "fixture/openai", "api.openai.com", "/v1/chat/completions"), Entry("Anthropic", "anthropic", "fixture/anthropic", "api.anthropic.com", "/v1/messages"))

	// per docs/adr/0099-native-factory-orchestration.md:120
	It("retains environment presence over YAML and YAML over parsed legacy settings", func() {
		root, cwd, path, environment := orchestrationReviewFixture("+PRIVATE_DIFF\n")
		writeFixture(filepath.Join(root, "factory.yaml"), []byte("model_provider: openrouter\nreview_model: yaml-review\nreview_reasoning_effort: high\nreview_openrouter_provider: yaml-host\nreview_max_tokens: 16384\n"), 0600)
		writeFixture(filepath.Join(root, "factory.config"), []byte("REVIEW_MODEL=legacy-review\nREVIEW_MAX_TOKENS=4096\n"), 0600)
		service := orchestrationReviewHTTP(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"No findings."}}]}`)
		})
		result := orchestrationReview(root, cwd, orchestrationReviewEnvironment(environment, service, "REVIEW_REASONING_EFFORT=", "REVIEW_OPENROUTER_PROVIDER="), path)
		Expect(result.status).To(Equal(0), result.stderr)
		requests := service.observed()
		Expect(requests).To(HaveLen(1))
		var body map[string]any
		Expect(json.Unmarshal(requests[0].body, &body)).To(Succeed())
		Expect(body["model"]).To(Equal("yaml-review"))
		Expect(body["max_tokens"]).To(Equal(float64(16384)))
		Expect(body).NotTo(HaveKey("reasoning"))
		Expect(body).NotTo(HaveKey("provider"))
	})

	// per docs/adr/0099-native-factory-orchestration.md:121
	DescribeTable("rejects inadmissible review controls before HTTP without leaking private input", func(setting, diagnostic string) {
		root, cwd, path, environment := orchestrationReviewFixture("+PRIVATE_DIFF\n")
		service := orchestrationReviewHTTP(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) { _, _ = io.WriteString(w, `{}`) })
		result := orchestrationReview(root, cwd, orchestrationReviewEnvironment(environment, service, setting), path)
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).To(ContainSubstring(diagnostic))
		Expect(result.stderr).NotTo(ContainSubstring("PRIVATE"))
		Expect(service.observed()).To(BeEmpty())
	}, Entry("timeout zero", "REVIEW_TIMEOUT_SECONDS=0", "invalid REVIEW_TIMEOUT_SECONDS"), Entry("timeout excessive", "REVIEW_TIMEOUT_SECONDS=1201", "invalid REVIEW_TIMEOUT_SECONDS"), Entry("tokens below floor", "REVIEW_MAX_TOKENS=1023", "invalid REVIEW_MAX_TOKENS"), Entry("tokens above cap", "REVIEW_MAX_TOKENS=32769", "invalid REVIEW_MAX_TOKENS"), Entry("reasoning", "REVIEW_REASONING_EFFORT=PRIVATE_INVALID", "invalid REVIEW_REASONING_EFFORT"), Entry("provider pin", "REVIEW_OPENROUTER_PROVIDER=PRIVATE/Upper", "invalid REVIEW_OPENROUTER_PROVIDER"))

	// per docs/adr/0099-native-factory-orchestration.md:123
	It("uses an explicitly selected Go stream client once and never falls back on its failure", func() {
		root, cwd, path, environment := orchestrationReviewFixture("+PRIVATE_DIFF\n")
		service := orchestrationReviewHTTP(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) { _, _ = io.WriteString(w, `{}`) })
		client := filepath.Join(cwd, "selected-client")
		capture := filepath.Join(cwd, "selected-request")
		writeFixture(client, []byte("#!/bin/sh\n[ \"$FACTORY_BRIDGE_PROTOCOL\" = 1 ] || exit 91\n[ \"$1:$2\" = review:openrouter ] || exit 92\n[ \"$REVIEW_TIMEOUT_SECONDS:$REVIEW_HTTP_RETRIES\" = 3:0 ] || exit 93\n/bin/cat > \"$SELECTED_CAPTURE\"\nexit 37\n"), 0700)
		result := orchestrationReview(root, cwd, orchestrationReviewEnvironment(environment, service, "REVIEW_GO_CLIENT="+client, "SELECTED_CAPTURE="+capture), path)
		Expect(result.status).To(Equal(37), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		Expect(service.observed()).To(BeEmpty())
		raw, err := os.ReadFile(capture)
		Expect(err).NotTo(HaveOccurred())
		var request map[string]any
		Expect(json.Unmarshal(raw, &request)).To(Succeed())
		Expect(request["model"]).To(Equal("fixture/frontier"))
		Expect(result.stderr).NotTo(ContainSubstring("PRIVATE"))
	})

	// per docs/adr/0099-native-factory-orchestration.md:126
	It("withholds provider bodies and partial output on HTTP failure", func() {
		root, cwd, path, environment := orchestrationReviewFixture("+PRIVATE_DIFF\n")
		service := orchestrationReviewHTTP(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":{"message":"PRIVATE_PROVIDER_BODY PRIVATE_API_KEY"}}`)
		})
		result := orchestrationReview(root, cwd, orchestrationReviewEnvironment(environment, service), path)
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(ContainSubstring("PRIVATE"))
		Expect(service.observed()).To(HaveLen(1))
	})

	// per docs/adr/0099-native-factory-orchestration.md:121
	It("rejects an OpenRouter length-limited answer as incomplete", func() {
		root, cwd, path, environment := orchestrationReviewFixture("+PRIVATE_DIFF\n")
		service := orchestrationReviewHTTP(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
			_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"length","message":{"content":"PRIVATE_PARTIAL"}}]}`)
		})
		result := orchestrationReview(root, cwd, orchestrationReviewEnvironment(environment, service), path)
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).To(ContainSubstring("incomplete review"))
		Expect(result.stderr).NotTo(ContainSubstring("PRIVATE_PARTIAL"))
		Expect(service.observed()).To(HaveLen(1))
	})
})
