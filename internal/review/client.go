// Package review implements the explicitly selected bounded review transport.
// docs/adr/0067-streaming-adversarial-review-client.md:15.
package review

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type metrics struct {
	mu                                                      sync.Mutex
	status, attempts, bytes, content, reasoning, heartbeats int
	start                                                   time.Time
}

func (m *metrics) report(out io.Writer, kind string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fmt.Fprintf(out, "review: %s http_status=%d attempts=%d elapsed_seconds=%.3f response_bytes=%d content_events=%d reasoning_events=%d heartbeats=%d\n", kind, m.status, m.attempts, time.Since(m.start).Seconds(), m.bytes, m.content, m.reasoning, m.heartbeats)
}

// Run never publishes findings until the complete stream is admitted.
func Run(ctx context.Context, input io.Reader, output, diagnostics io.Writer) error {
	m := &metrics{start: time.Now()}
	defer m.report(diagnostics, "summary")
	seconds, err := timeoutSeconds(os.Getenv("REVIEW_TIMEOUT_SECONDS"))
	if err != nil {
		return err
	}
	retries := os.Getenv("REVIEW_HTTP_RETRIES")
	if retries != "" && retries != "0" && retries != "1" {
		return errors.New("invalid review retries")
	}
	key := os.Getenv("REVIEW_API_KEY")
	if strings.TrimSpace(key) == "" {
		return errors.New("missing review API key")
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	if closer, ok := input.(io.Closer); ok {
		stopClose := context.AfterFunc(ctx, func() { _ = closer.Close() })
		defer stopClose()
	}
	body, err := prepare(ctx, input)
	if err != nil {
		return contextFailure(ctx, err)
	}
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 15 * time.Second}).DialContext, TLSHandshakeTimeout: 15 * time.Second, DisableKeepAlives: true, DisableCompression: true, MaxResponseHeaderBytes: 1 << 20}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	for attempt := 1; attempt <= 2; attempt++ {
		m.mu.Lock()
		m.attempts = attempt
		m.mu.Unlock()
		response, closeAttempt, err := send(ctx, client, body, key)
		if err != nil {
			return contextFailure(ctx, err)
		}
		m.mu.Lock()
		m.status = response.StatusCode
		m.mu.Unlock()
		if response.StatusCode != http.StatusOK {
			header := response.Header.Get("Retry-After")
			_ = response.Body.Close()
			closeAttempt()
			if attempt != 1 || retries != "1" || (response.StatusCode != 429 && response.StatusCode != 503) {
				return errors.New("review HTTP failure")
			}
			delay, err := retryDelay(header, time.Now())
			if err != nil {
				return err
			}
			deadline, _ := ctx.Deadline()
			if delay >= time.Until(deadline) {
				return errors.New("review retry refused")
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return contextFailure(ctx, errors.New("review retry refused"))
			case <-timer.C:
			}
			continue
		}
		finished := make(chan struct{})
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-finished:
					return
				case <-ticker.C:
					m.report(diagnostics, "progress")
				}
			}
		}()
		findings, err := readStream(ctx, response.Body, m)
		_ = response.Body.Close()
		closeAttempt()
		close(finished)
		<-stopped
		if err != nil {
			return contextFailure(ctx, err)
		}
		if ctx.Err() != nil {
			return contextFailure(ctx, errors.New("review canceled"))
		}
		if _, err := io.WriteString(output, strings.TrimRight(findings, "\n")+"\n"); err != nil {
			return errors.New("cannot write review")
		}
		return nil
	}
	return errors.New("review HTTP failure")
}

// GotConn ends one combined dial, proxy CONNECT and TLS deadline.
// Fresh HTTP/1 connections and absent GetBody prevent transparent replays.
// docs/adr/0067-streaming-adversarial-review-client.md:47.
func send(ctx context.Context, client *http.Client, body []byte, key string) (*http.Response, context.CancelFunc, error) {
	attempt, cancel := context.WithCancel(ctx)
	timer := time.AfterFunc(15*time.Second, cancel)
	trace := &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) { timer.Stop() }}
	request, err := http.NewRequestWithContext(httptrace.WithClientTrace(attempt, trace), http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		timer.Stop()
		cancel()
		return nil, func() {}, errors.New("invalid review request")
	}
	request.GetBody = nil
	request.Close = true
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	response, err := client.Do(request)
	timer.Stop()
	if err != nil {
		connectionExpired := attempt.Err() != nil && ctx.Err() == nil
		cancel()
		if connectionExpired {
			return nil, func() {}, errors.New("review connection timeout")
		}
		return nil, func() {}, errors.New("review transport failure")
	}
	return response, cancel, nil
}

func contextFailure(ctx context.Context, fallback error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.New("review timed out")
	}
	if ctx.Err() != nil {
		return errors.New("review canceled")
	}
	return fallback
}

func retryDelay(value string, now time.Time) (time.Duration, error) {
	if value == "" {
		n, err := rand.Int(rand.Reader, big.NewInt(4))
		if err != nil {
			return 0, errors.New("review retry refused")
		}
		return time.Duration(n.Int64()+2) * time.Second, nil
	}
	if strings.Trim(value, "0123456789") == "" {
		n, err := strconv.Atoi(value)
		if err != nil || n > 60 {
			return 0, errors.New("review retry refused")
		}
		return time.Duration(n) * time.Second, nil
	}
	date, err := http.ParseTime(value)
	if err != nil {
		return 0, errors.New("review retry refused")
	}
	delay := date.Sub(now)
	if delay > 60*time.Second {
		return 0, errors.New("review retry refused")
	}
	if delay < 0 {
		return 0, nil
	}
	return delay, nil
}
