package acceptance_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type reviewObservation struct {
	host, path, auth string
	body             []byte
}
type reviewService struct {
	proxy, ca string
	mu        sync.Mutex
	requests  []reviewObservation
}

func reviewFixture(cwd string, respond func(http.ResponseWriter, *http.Request, int)) *reviewService {
	GinkgoHelper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-owned review service"}, DNSNames: []string{"openrouter.ai"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	Expect(err).NotTo(HaveOccurred())
	service := &reviewService{ca: filepath.Join(cwd, "review-ca.pem")}
	writeFixture(service.ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600)
	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			http.Error(w, "fixture read failure", http.StatusInternalServerError)
			return
		}
		service.mu.Lock()
		service.requests = append(service.requests, reviewObservation{r.Host, r.URL.Path, r.Header.Get("Authorization"), body})
		count := len(service.requests)
		service.mu.Unlock()
		respond(w, r, count)
	}))
	backend.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	backend.StartTLS()
	DeferCleanup(backend.Close)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "openrouter.ai:443" {
			http.Error(w, "unexpected fixture destination", http.StatusBadRequest)
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
		if _, writeErr := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); writeErr != nil {
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
func (s *reviewService) observed() []reviewObservation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]reviewObservation(nil), s.requests...)
}

const reviewPrepared = `{"model":"fixture/model","messages":[{"role":"user","content":"PRIVATE_PROMPT"}],"max_tokens":1024,"reasoning":{"effort":"low"},"provider":{"order":["deepinfra"],"allow_fallbacks":false,"require_parameters":true}}`
const reviewComplete = "data: {\"choices\":[{\"delta\":{\"content\":\"Safe findings\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"

func reviewProcess(root, cwd, input string, service *reviewService, extra ...string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(root, "factory"), "review", "openrouter") // #nosec G204 G702 -- compiled test-owned executable and fixed command; no shell or external argv.
	cmd.Dir = cwd
	cmd.Env = append([]string{"FACTORY_BRIDGE_PROTOCOL=1", "PATH=/absent-review-tools", "REVIEW_API_KEY=PRIVATE_API_KEY", "HTTPS_PROXY=" + service.proxy, "SSL_CERT_FILE=" + service.ca, "REVIEW_TIMEOUT_SECONDS=3", "REVIEW_HTTP_RETRIES=0"}, extra...)
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred(), "review command exceeded fixture watchdog")
	status := 0
	if err != nil {
		var exitErr *exec.ExitError
		Expect(errors.As(err, &exitErr)).To(BeTrue())
		status = exitErr.ExitCode()
	}
	return cliResult{stdout.String(), stderr.String(), status}
}

var _ = Describe("Bounded streaming adversarial review", func() {
	// per docs/adr/0067-streaming-adversarial-review-client.md:15
	It("sends one faithful fixed-endpoint streaming request and publishes completed findings", func() {
		root, cwd := fixture()
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, reviewComplete)
		})
		result := reviewProcess(root, cwd, reviewPrepared, service)
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(strings.TrimSpace(result.stdout)).To(Equal("Safe findings"))
		requests := service.observed()
		Expect(requests).To(HaveLen(1))
		Expect(requests[0].host).To(Equal("openrouter.ai"))
		Expect(requests[0].path).To(Equal("/api/v1/chat/completions"))
		Expect(requests[0].auth).To(Equal("Bearer PRIVATE_API_KEY"))
		var actual, expected map[string]any
		Expect(json.Unmarshal(requests[0].body, &actual)).To(Succeed())
		Expect(json.Unmarshal([]byte(reviewPrepared), &expected)).To(Succeed())
		expected["stream"] = true
		Expect(actual).To(Equal(expected))
		Expect(result.stderr).NotTo(ContainSubstring("PRIVATE_"))
		Expect(result.stderr).NotTo(ContainSubstring("Safe findings"))
	})
	// per docs/adr/0067-streaming-adversarial-review-client.md:57
	It("withholds partial findings when clean EOF arrives without DONE", func() {
		root, cwd := fixture()
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
			_, _ = io.WriteString(w, strings.Split(reviewComplete, "data: [DONE]")[0])
		})
		result := reviewProcess(root, cwd, reviewPrepared, service)
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		Expect(service.observed()).To(HaveLen(1))
		Expect(result.stderr).NotTo(ContainSubstring("Safe findings"))
	})
})

var _ = Describe("Bounded streaming adversarial review boundaries", func() {
	// per docs/adr/0067-streaming-adversarial-review-client.md:52
	DescribeTable("accepts SSE framing without exposing reasoning", func(stream string) {
		root, cwd := fixture()
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
			w.Header().Set("Content-Type", "text/event-stream")
			for i := 0; i < len(stream); i++ {
				_, err := io.WriteString(w, stream[i:i+1])
				if err != nil {
					return
				}
				w.(http.Flusher).Flush()
			}
		})
		result := reviewProcess(root, cwd, reviewPrepared, service)
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(strings.TrimSpace(result.stdout)).To(Equal("Safe findings"))
		Expect(service.observed()).To(HaveLen(1))
		Expect(result.stderr).NotTo(ContainSubstring("PRIVATE"))
	},
		Entry("CRLF", strings.ReplaceAll(reviewComplete, "\n", "\r\n")),
		Entry("CR", strings.ReplaceAll(reviewComplete, "\n", "\r")),
		Entry("comments and reasoning", ": PRIVATE_HEARTBEAT\n\ndata: {\"choices\":[{\"delta\":{\"reasoning\":\"PRIVATE_REASONING\"}}]}\n\n"+reviewComplete),
		Entry("multiline data", strings.Replace(reviewComplete, `"delta":{"content"`, `"delta":`+"\ndata: "+`{"content"`, 1)),
		Entry("matching accounting terminal", strings.Replace(reviewComplete, "data: [DONE]", "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"total_tokens\":7}}\n\ndata: [DONE]", 1)),
		Entry("content-free usage", strings.Replace(reviewComplete, "data: [DONE]", "data: {\"choices\":[],\"usage\":{\"total_tokens\":7}}\n\ndata: [DONE]", 1)),
	)
	// per docs/adr/0067-streaming-adversarial-review-client.md:57
	DescribeTable("fails closed and never retries a response stream", func(stream string) {
		root, cwd := fixture()
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) { _, _ = io.WriteString(w, stream) })
		result := reviewProcess(root, cwd, reviewPrepared, service, "REVIEW_HTTP_RETRIES=1")
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		Expect(service.observed()).To(HaveLen(1))
		Expect(result.stderr).NotTo(ContainSubstring("PRIVATE"))
		Expect(result.stderr).NotTo(ContainSubstring("Safe findings"))
	},
		Entry("malformed event", "data: PRIVATE_BROKEN_JSON\n\n"),
		Entry("embedded provider error", "data: {\"error\":{\"message\":\"PRIVATE_PROVIDER_ERROR\"}}\n\n"),
		Entry("error after content", strings.Replace(reviewComplete, "data: [DONE]", "data: {\"error\":{\"message\":\"PRIVATE_PROVIDER_ERROR\"}}\n\ndata: [DONE]", 1)),
		Entry("length", strings.ReplaceAll(reviewComplete, `"stop"`, `"length"`)),
		Entry("content filter", strings.ReplaceAll(reviewComplete, `"stop"`, `"content_filter"`)),
		Entry("tool calls", strings.ReplaceAll(reviewComplete, `"stop"`, `"tool_calls"`)),
		Entry("no terminal", strings.ReplaceAll(reviewComplete, `"stop"`, `null`)),
		Entry("empty content", strings.ReplaceAll(reviewComplete, "Safe findings", " ")),
		Entry("contradictory terminal", strings.Replace(reviewComplete, "data: [DONE]", "data: {\"choices\":[{\"finish_reason\":\"length\"}]}\n\ndata: [DONE]", 1)),
		Entry("data after DONE", reviewComplete+"data: {}\n\n"),
		Entry("line exceeds limit", ":"+strings.Repeat("x", 1024*1024)+"\n\n"+reviewComplete),
		Entry("event exceeds limit", strings.Repeat(":"+strings.Repeat("x", 4096)+"\n", 257)+"\n"+reviewComplete),
		Entry("total exceeds limit", strings.Repeat(":"+strings.Repeat("x", 65536)+"\n\n", 129)+reviewComplete),
	)
	// per docs/adr/0067-streaming-adversarial-review-client.md:21
	DescribeTable("rejects inadmissible request before HTTP", func(input string) {
		root, cwd := fixture()
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) { _, _ = io.WriteString(w, reviewComplete) })
		result := reviewProcess(root, cwd, input, service)
		Expect(result.status).NotTo(Equal(0))
		Expect(result.stdout).To(BeEmpty())
		Expect(service.observed()).To(BeEmpty())
		Expect(result.stderr).NotTo(ContainSubstring("PRIVATE"))
	},
		Entry("malformed", `{"PRIVATE_SECRET":`),
		Entry("invalid UTF8", strings.Replace(reviewPrepared, "PRIVATE_PROMPT", string([]byte{0xff}), 1)), Entry("array", "[]"), Entry("trailing object", reviewPrepared+"{}"),
		Entry("blank model", strings.Replace(reviewPrepared, "fixture/model", " ", 1)),
		Entry("empty messages", strings.Replace(reviewPrepared, `[{"role":"user","content":"PRIVATE_PROMPT"}]`, `[]`, 1)),
		Entry("too few tokens", strings.Replace(reviewPrepared, "1024", "1023", 1)),
		Entry("too many tokens", strings.Replace(reviewPrepared, "1024", "32769", 1)),
		Entry("float tokens", strings.Replace(reviewPrepared, "1024", "1024.0", 1)),
		Entry("multiple completions", strings.Replace(reviewPrepared, `"model":`, `"n":2,"model":`, 1)),
		Entry("tools", strings.Replace(reviewPrepared, `"model":`, `"tools":[],"model":`, 1)),
		Entry("provider fallback", strings.Replace(reviewPrepared, `"allow_fallbacks":false`, `"allow_fallbacks":true`, 1)),
		Entry("nested provider extras", strings.Replace(reviewPrepared, `"order":`, `"PRIVATE_EXTRA":true,"order":`, 1)),
		Entry("oversized", strings.Replace(reviewPrepared, "PRIVATE_PROMPT", strings.Repeat("x", 1024*1024), 1)),
	)
	// per docs/adr/0067-streaming-adversarial-review-client.md:70
	DescribeTable("bounds explicit HTTP retries", func(status int, retryAfter, retries string, wantCalls, wantStatus int) {
		root, cwd := fixture()
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, count int) {
			if count == 1 {
				w.Header().Set("Retry-After", retryAfter)
				w.Header().Set("Location", "https://openrouter.ai/redirected")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "PRIVATE_PROVIDER_ERROR")
				return
			}
			_, _ = io.WriteString(w, reviewComplete)
		})
		result := reviewProcess(root, cwd, reviewPrepared, service, "REVIEW_HTTP_RETRIES="+retries)
		Expect(result.status).To(Equal(wantStatus), result.stderr)
		Expect(service.observed()).To(HaveLen(wantCalls))
		Expect(result.stderr).NotTo(ContainSubstring("PRIVATE"))
		if wantStatus != 0 {
			Expect(result.stdout).To(BeEmpty())
		}
	},
		Entry("429 once", 429, "0", "1", 2, 0), Entry("503 once", 503, "0", "1", 2, 0),
		Entry("disabled", 429, "0", "0", 1, 1), Entry("negative", 429, "-1", "1", 1, 1),
		Entry("malformed", 429, "PRIVATE_BAD_DELAY", "1", 1, 1), Entry("too long", 503, "61", "1", 1, 1),
		Entry("outside deadline", 429, "60", "1", 1, 1), Entry("unauthorized", 401, "0", "1", 1, 1),
		Entry("payment", 402, "0", "1", 1, 1), Entry("server error", 500, "0", "1", 1, 1), Entry("redirect", 302, "0", "1", 1, 1),
	)
	// per docs/adr/0067-streaming-adversarial-review-client.md:47
	It("enforces one deadline and closes a stalled response", func() {
		root, cwd := fixture()
		closed := make(chan struct{})
		service := reviewFixture(cwd, func(w http.ResponseWriter, r *http.Request, _ int) {
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"PRIVATE_PARTIAL\"}}]}\n\n")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			close(closed)
		})
		result := reviewProcess(root, cwd, reviewPrepared, service, "REVIEW_TIMEOUT_SECONDS=1", "REVIEW_HTTP_RETRIES=1")
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(ContainSubstring("PRIVATE"))
		Expect(service.observed()).To(HaveLen(1))
		Eventually(closed, time.Second).Should(BeClosed())
	})
})

var _ = Describe("Bounded streaming adversarial review recovery", func() {
	// per docs/adr/0067-streaming-adversarial-review-client.md:70
	It("stops after the single permitted retry", func() {
		root, cwd := fixture()
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
		})
		result := reviewProcess(root, cwd, reviewPrepared, service, "REVIEW_HTTP_RETRIES=1")
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		Expect(service.observed()).To(HaveLen(2))
	})
	// per docs/adr/0067-streaming-adversarial-review-client.md:73
	It("accepts a valid elapsed HTTP-date without changing request parameters", func() {
		root, cwd := fixture()
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, count int) {
			if count == 1 {
				w.Header().Set("Retry-After", time.Now().Add(-time.Minute).UTC().Format(http.TimeFormat))
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = io.WriteString(w, reviewComplete)
		})
		result := reviewProcess(root, cwd, reviewPrepared, service, "REVIEW_HTTP_RETRIES=1")
		Expect(result.status).To(Equal(0), result.stderr)
		requests := service.observed()
		Expect(requests).To(HaveLen(2))
		Expect(requests[0].body).To(Equal(requests[1].body))
	})
	// per docs/adr/0067-streaming-adversarial-review-client.md:57
	It("requires clean transport completion even after DONE", func() {
		root, cwd := fixture()
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
			w.Header().Set("Content-Length", "9999")
			_, _ = io.WriteString(w, reviewComplete)
		})
		result := reviewProcess(root, cwd, reviewPrepared, service, "REVIEW_HTTP_RETRIES=1")
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		Expect(service.observed()).To(HaveLen(1))
	})
	// per docs/adr/0067-streaming-adversarial-review-client.md:47
	DescribeTable("rejects invalid bounds without a request", func(variable string) {
		root, cwd := fixture()
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) { _, _ = io.WriteString(w, reviewComplete) })
		result := reviewProcess(root, cwd, reviewPrepared, service, variable)
		Expect(result.status).NotTo(Equal(0))
		Expect(result.stdout).To(BeEmpty())
		Expect(service.observed()).To(BeEmpty())
	}, Entry("zero deadline", "REVIEW_TIMEOUT_SECONDS=0"), Entry("oversized deadline", "REVIEW_TIMEOUT_SECONDS=1201"), Entry("noninteger deadline", "REVIEW_TIMEOUT_SECONDS=1.0"), Entry("excess retries", "REVIEW_HTTP_RETRIES=2"), Entry("negative retries", "REVIEW_HTTP_RETRIES=-1"))
})

var _ = Describe("Bounded streaming adversarial review cancellation", func() {
	// per docs/adr/0067-streaming-adversarial-review-client.md:49
	It("cancels a live response on SIGTERM without publishing or retrying", func() {
		root, cwd := fixture()
		started, closed := make(chan struct{}), make(chan struct{})
		service := reviewFixture(cwd, func(w http.ResponseWriter, r *http.Request, _ int) {
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"PRIVATE_PARTIAL\"}}]}\n\n")
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
			close(closed)
		})
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, filepath.Join(root, "factory"), "review", "openrouter") // #nosec G204 G702 -- test-built executable and literal command; no shell or external argv.
		cmd.Dir = cwd
		cmd.Env = []string{"FACTORY_BRIDGE_PROTOCOL=1", "PATH=/absent-review-tools", "REVIEW_API_KEY=PRIVATE_API_KEY", "HTTPS_PROXY=" + service.proxy, "SSL_CERT_FILE=" + service.ca, "REVIEW_TIMEOUT_SECONDS=5", "REVIEW_HTTP_RETRIES=1"}
		cmd.Stdin = strings.NewReader(reviewPrepared)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		Expect(cmd.Start()).To(Succeed())
		finished := make(chan error, 1)
		go func() { finished <- cmd.Wait() }()
		Eventually(started, 3*time.Second).Should(BeClosed())
		Expect(cmd.Process.Signal(syscall.SIGTERM)).To(Succeed())
		var exitErr error
		Eventually(finished, 2*time.Second).Should(Receive(&exitErr))
		Expect(exitErr).To(HaveOccurred())
		Expect(stdout.String()).To(BeEmpty())
		Expect(stderr.String()).NotTo(ContainSubstring("PRIVATE"))
		Expect(service.observed()).To(HaveLen(1))
		Eventually(closed, time.Second).Should(BeClosed())
	})
})

var _ = Describe("Bounded streaming adversarial review hostile transport", func() {
	// per docs/adr/0067-streaming-adversarial-review-client.md:62
	It("redacts provider-controlled malformed chunked trailers", func() {
		root, cwd := fixture()
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer conn.Close()
			_, _ = fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n%x\r\n%s\r\n0\r\nPRIVATE_PROVIDER_TRAILER\r\n\r\n", len(reviewComplete), reviewComplete)
		})
		result := reviewProcess(root, cwd, reviewPrepared, service, "REVIEW_HTTP_RETRIES=1")
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		Expect(service.observed()).To(HaveLen(1))
		Expect(result.stderr).NotTo(ContainSubstring("PRIVATE_PROVIDER_TRAILER"))
	})
	// per docs/adr/0067-streaming-adversarial-review-client.md:55
	DescribeTable("rejects non-object accounting usage after valid findings", func(usage string) {
		root, cwd := fixture()
		stream := strings.Replace(reviewComplete, "data: [DONE]", `data: {"choices":[],"usage":`+usage+"}\n\ndata: [DONE]", 1)
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) { _, _ = io.WriteString(w, stream) })
		result := reviewProcess(root, cwd, reviewPrepared, service, "REVIEW_HTTP_RETRIES=1")
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		Expect(service.observed()).To(HaveLen(1))
	}, Entry("string", `"PRIVATE_INVALID_USAGE"`), Entry("boolean", "true"), Entry("array", "[]"))
})

var _ = Describe("Bounded streaming adversarial review BOM", func() {
	// per docs/adr/0067-streaming-adversarial-review-client.md:52
	It("preserves the first content chunk after an initial UTF8 BOM", func() {
		root, cwd := fixture()
		first := "data: {\"choices\":[{\"delta\":{\"content\":\"First \"}}]}\n\n"
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, _ int) {
			_, _ = io.WriteString(w, "\ufeff"+first+reviewComplete)
		})
		result := reviewProcess(root, cwd, reviewPrepared, service)
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(strings.TrimSpace(result.stdout)).To(Equal("First Safe findings"))
		Expect(service.observed()).To(HaveLen(1))
	})
})

type reviewProgressCapture struct {
	mu       sync.Mutex
	output   bytes.Buffer
	progress chan time.Time
}

func (c *reviewProgressCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n, err := c.output.Write(p)
	if strings.Contains(c.output.String(), "review: progress ") {
		select {
		case c.progress <- time.Now():
		default:
		}
	}
	return n, err
}
func (c *reviewProgressCapture) text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.output.String()
}

var _ = Describe("Bounded streaming adversarial review timing", func() {
	// per docs/adr/0067-streaming-adversarial-review-client.md:48
	It("bounds a stalled proxy CONNECT to fifteen seconds despite a sixty-second overall deadline", func() {
		root, cwd := fixture()
		release := make(chan struct{})
		var mu sync.Mutex
		connects := 0
		proxy := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			mu.Lock()
			connects++
			mu.Unlock()
			select {
			case <-r.Context().Done():
			case <-release:
			}
		}))
		DeferCleanup(proxy.Close)
		defer close(release)
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, filepath.Join(root, "factory"), "review", "openrouter") // #nosec G204 G702 -- test-built executable and fixed arguments.
		cmd.Dir = cwd
		cmd.Env = []string{"FACTORY_BRIDGE_PROTOCOL=1", "PATH=/absent-review-tools", "REVIEW_API_KEY=PRIVATE_API_KEY", "HTTPS_PROXY=" + proxy.URL, "REVIEW_TIMEOUT_SECONDS=60", "REVIEW_HTTP_RETRIES=1"}
		cmd.Stdin = strings.NewReader(reviewPrepared)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		began := time.Now()
		err := cmd.Run()
		elapsed := time.Since(began)
		Expect(ctx.Err()).NotTo(HaveOccurred(), "connection bound failed before watchdog")
		Expect(err).To(HaveOccurred())
		Expect(elapsed).To(BeNumerically(">=", 14*time.Second))
		Expect(elapsed).To(BeNumerically("<", 22*time.Second))
		Expect(stdout.String()).To(BeEmpty())
		Expect(stderr.String()).NotTo(ContainSubstring("PRIVATE"))
		mu.Lock()
		count := connects
		mu.Unlock()
		Expect(count).To(Equal(1))
	})
	// per docs/adr/0067-streaming-adversarial-review-client.md:62
	It("emits safe progress around thirty seconds before completing a stalled stream", func() {
		root, cwd := fixture()
		release := make(chan struct{})
		var once sync.Once
		defer once.Do(func() { close(release) })
		service := reviewFixture(cwd, func(w http.ResponseWriter, r *http.Request, _ int) {
			_, _ = io.WriteString(w, ": PRIVATE_HEARTBEAT\n\ndata: {\"choices\":[{\"delta\":{\"reasoning\":\"PRIVATE_REASONING\"}}]}\n\n")
			w.(http.Flusher).Flush()
			select {
			case <-release:
				_, _ = io.WriteString(w, reviewComplete)
			case <-r.Context().Done():
			}
		})
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, filepath.Join(root, "factory"), "review", "openrouter") // #nosec G204 G702 -- test-built executable and literal argv; no external command selection.
		cmd.Dir = cwd
		cmd.Env = []string{"FACTORY_BRIDGE_PROTOCOL=1", "PATH=/absent-review-tools", "REVIEW_API_KEY=PRIVATE_API_KEY", "HTTPS_PROXY=" + service.proxy, "SSL_CERT_FILE=" + service.ca, "REVIEW_TIMEOUT_SECONDS=60", "REVIEW_HTTP_RETRIES=0"}
		cmd.Stdin = strings.NewReader(reviewPrepared)
		var stdout bytes.Buffer
		capture := &reviewProgressCapture{progress: make(chan time.Time, 1)}
		cmd.Stdout, cmd.Stderr = &stdout, capture
		began := time.Now()
		Expect(cmd.Start()).To(Succeed())
		finished := make(chan error, 1)
		go func() { finished <- cmd.Wait() }()
		var observed time.Time
		Eventually(capture.progress, 38*time.Second).Should(Receive(&observed))
		Expect(observed.Sub(began)).To(BeNumerically(">=", 29*time.Second))
		Expect(observed.Sub(began)).To(BeNumerically("<", 38*time.Second))
		Expect(capture.text()).NotTo(ContainSubstring("PRIVATE"))
		Expect(strings.Count(capture.text(), "review: progress ")).To(Equal(1))
		once.Do(func() { close(release) })
		var exitErr error
		Eventually(finished, 3*time.Second).Should(Receive(&exitErr))
		Expect(exitErr).NotTo(HaveOccurred(), capture.text())
		Expect(strings.TrimSpace(stdout.String())).To(Equal("Safe findings"))
		Expect(capture.text()).NotTo(ContainSubstring("PRIVATE"))
		Expect(capture.text()).To(ContainSubstring("review: summary "))
		Expect(service.observed()).To(HaveLen(1))
	})
})
