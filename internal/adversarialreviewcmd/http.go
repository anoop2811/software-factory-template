package adversarialreviewcmd

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/fileinput"
	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
	"golang.org/x/net/http/httpproxy"
)

// Provider bodies remain bounded and unpublished until successful admission.
// docs/adr/0099-native-factory-orchestration.md:126.
func requestReview(ctx context.Context, endpoint, provider, key string, body []byte, environment map[string]string) (string, error) {
	proxy := &httpproxy.Config{HTTPProxy: defaultValue(environment["HTTP_PROXY"], environment["http_proxy"]), HTTPSProxy: defaultValue(environment["HTTPS_PROXY"], environment["https_proxy"]), NoProxy: defaultValue(environment["NO_PROXY"], environment["no_proxy"]), CGI: environment["REQUEST_METHOD"] != ""}
	proxyFunc := proxy.ProxyFunc()
	transport := &http.Transport{Proxy: func(request *http.Request) (*url.URL, error) { return proxyFunc(request.URL) }, DialContext: (&net.Dialer{Timeout: 15 * time.Second}).DialContext, TLSHandshakeTimeout: 15 * time.Second, DisableKeepAlives: true, DisableCompression: true, MaxResponseHeaderBytes: 1 << 20}
	defer transport.CloseIdleConnections()
	if path := environment["SSL_CERT_FILE"]; path != "" {
		certificates, err := fileinput.Read(ctx, path, fileLimit)
		if err != nil {
			return "", errors.New("cannot prepare review TLS")
		}
		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(certificates) {
			return "", errors.New("cannot prepare review TLS")
		}
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	attempt, cancel := context.WithCancel(ctx)
	defer cancel()
	timer := time.AfterFunc(15*time.Second, cancel)
	defer timer.Stop()
	trace := &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) { timer.Stop() }}
	request, err := http.NewRequestWithContext(httptrace.WithClientTrace(attempt, trace), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", errors.New("invalid review request")
	}
	request.GetBody = nil
	request.Close = true
	request.Header.Set("Content-Type", "application/json")
	if provider == "anthropic" {
		request.Header.Set("x-api-key", key)
		request.Header.Set("anthropic-version", "2023-06-01")
	} else {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", errors.New("request timed out")
		}
		if ctx.Err() != nil {
			return "", errors.New("request canceled")
		}
		return "", errors.New("transport failure")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", errors.New("HTTP request failed")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, fileLimit+1))
	if err != nil || len(raw) > fileLimit {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", errors.New("request timed out")
		}
		return "", errors.New("invalid review response")
	}
	decoded, err := jsonvalue.DecodeUnique(ctx, raw)
	if err != nil {
		return "", errors.New("invalid review response")
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return "", errors.New("the model returned no review")
	}
	var text string
	if provider == "anthropic" {
		if contents, ok := object["content"].([]any); ok && len(contents) > 0 {
			if first, ok := contents[0].(map[string]any); ok {
				text, _ = first["text"].(string)
			}
		}
	} else if choices, ok := object["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if provider != "openai" && choice["finish_reason"] == "length" {
				return "", errors.New("incomplete review: OpenRouter reached the response token limit (finish_reason=length)")
			}
			if message, ok := choice["message"].(map[string]any); ok {
				text, _ = message["content"].(string)
			}
		}
	}
	if strings.TrimRight(text, "\n") == "" {
		return "", errors.New("the model returned no review")
	}
	if ctx.Err() != nil {
		return "", errors.New("request canceled")
	}
	return text, nil
}
