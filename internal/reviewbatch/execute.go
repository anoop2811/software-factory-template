package reviewbatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/anoop2811/software-factory-template/internal/review"
)

var (
	errReviewBudget  = errors.New("run-wide review budget exhausted")
	errReviewStopped = errors.New("review admission stopped after an incomplete batch")
)

// Every worker and retry shares this ledger; no request gets a private copy of
// the run's allowances. docs/adr/0112-bounded-parallel-review.md:54.
type reviewAdmission struct {
	mu      sync.Mutex
	budget  *budget
	stopped bool
}

func (a *reviewAdmission) reserve(ctx context.Context, inputBytes, maxTokens int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.stopped {
		return errReviewStopped
	}
	b := a.budget
	if b.RequestsUsed >= b.MaxRequests || maxTokens > b.MaxTokens-b.TokensReserved || inputBytes > b.MaxInput-b.InputReserved {
		a.stopped = true
		return errReviewBudget
	}
	b.RequestsUsed++
	b.TokensReserved += maxTokens
	b.InputReserved += inputBytes
	return nil
}

func (a *reviewAdmission) stop() {
	a.mu.Lock()
	a.stopped = true
	a.mu.Unlock()
}

func (a *reviewAdmission) open() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.stopped
}

type reviewDiagnostics struct {
	mu     sync.Mutex
	writer io.Writer
}

func (d *reviewDiagnostics) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.writer.Write(p)
}

type batchOutcome struct {
	index    int
	attempts int
	status   string
	failure  string
	findings string
	usage    json.RawMessage
	elapsed  time.Duration
}

func runBatch(ctx context.Context, index int, body []byte, p policy, key string, admission *reviewAdmission, diagnostics io.Writer) (result batchOutcome) {
	started := time.Now()
	defer func() { result.elapsed = time.Since(started) }()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.BatchTimeout)*time.Second)
	defer cancel()
	result = batchOutcome{index: index, status: "complete"}
	var findings bytes.Buffer
	var denied error
	err := review.RunWithOptions(ctx, bytes.NewReader(body), &findings, diagnostics, review.Options{
		APIKey: key, TimeoutSeconds: p.BatchTimeout, HTTPRetries: p.HTTPRetries,
		BeforeAttempt: func(inputBytes, maxTokens int) error {
			denied = admission.reserve(ctx, inputBytes, maxTokens)
			if denied == nil {
				result.attempts++
			}
			return denied
		},
		OnUsage: func(value json.RawMessage) { result.usage = reportedUsage(value) },
	})
	if err != nil || findings.Len() > 2<<20 {
		admission.stop()
		result.status, result.failure = "failed", "review request failed without complete findings"
		switch {
		case errors.Is(denied, errReviewBudget):
			result.status, result.failure = "skipped", errReviewBudget.Error()
		case errors.Is(denied, errReviewStopped):
			result.status, result.failure = "skipped", errReviewStopped.Error()
		case ctx.Err() != nil:
			result.failure = "review canceled or batch deadline exceeded"
		}
		return result
	}
	result.findings = strings.ReplaceAll(findings.String(), key, "[REDACTED]")
	return result
}

// Only this coordinator mutates results and publishes cache files. Completed
// siblings survive a failure, and report order stays the original plan order.
// docs/adr/0112-bounded-parallel-review.md:66.
func runBatches(ctx context.Context, r *report, p policy, shared, key string, cache *os.Root, diagnostics io.Writer) {
	admission := &reviewAdmission{budget: &r.Budget}
	lockedDiagnostics := &reviewDiagnostics{writer: diagnostics}
	var pending []int
	for i := range r.Batches {
		b := &r.Batches[i]
		if saved, ok := loadCache(ctx, cache, b.ID, key); ok {
			b.Status, b.Cached, b.Findings, b.Usage = "complete", true, saved.Findings, saved.Usage
		} else {
			pending = append(pending, i)
		}
	}
	completed := make(chan batchOutcome, p.Parallelism)
	outcomes := make([]batchOutcome, len(r.Batches))
	next, running := 0, 0
	for {
		for running < p.Parallelism && next < len(pending) && ctx.Err() == nil && admission.open() {
			index := pending[next]
			next++
			body := request(p, shared, r.Batches[index])
			if len(body) > 1<<20 {
				r.Batches[index].Status, r.Batches[index].Error = "failed", "shared context and batch exceed the request limit"
				admission.stop()
				break
			}
			running++
			_, _ = fmt.Fprintf(lockedDiagnostics, "review-batch: batch %d/%d started\n", index+1, len(r.Batches))
			go func() {
				completed <- runBatch(ctx, index, body, p, key, admission, lockedDiagnostics)
			}()
		}
		if running == 0 {
			break
		}
		result := <-completed
		running--
		_, _ = fmt.Fprintf(lockedDiagnostics, "review-batch: batch %d/%d status=%s elapsed_seconds=%.3f\n", result.index+1, len(r.Batches), result.status, result.elapsed.Seconds())
		outcomes[result.index] = result
		b := &r.Batches[result.index]
		b.Status, b.Error, b.Findings = result.status, result.failure, result.findings
		if result.status == "complete" {
			b.Usage = result.usage
			if err := saveCache(cache, *b, key); err != nil {
				_, _ = fmt.Fprintln(lockedDiagnostics, "review-batch: successful review retained; cache publication unavailable")
			}
		}
	}
	usageUnknown := false
	for i, result := range outcomes {
		if result.attempts == 0 {
			continue
		}
		if result.status != "complete" || len(result.usage) == 0 {
			usageUnknown = true
			r.Usage = nil
		} else if !usageUnknown {
			r.Usage = addUsage(r.Usage, result.usage)
			usageUnknown = len(r.Usage) == 0
		}
		if r.Batches[i].Error != "" && r.Error == "" {
			r.Error = r.Batches[i].Error
		}
	}
	for _, b := range r.Batches {
		if b.Error != "" && r.Error == "" {
			r.Error = b.Error
		}
	}
	if ctx.Err() != nil {
		r.Error = "review canceled or total deadline exceeded"
	}
}
