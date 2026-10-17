package reviewbatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/review"
)

// Run returns success only for an offline plan or complete exact-commit coverage.
// docs/adr/0100-budgeted-batched-adversarial-review.md:76.
func Run(parent context.Context, args []string, environment map[string]string, stdout, stderr io.Writer) int {
	o, err := parseOptions(args, stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "review-batch:", err)
		return 2
	}
	p, key, err := readPolicy(parent, environment)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "review-batch:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.TimeoutSeconds)*time.Second)
	defer cancel()
	diffs, err := readInputs(ctx, o)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "review-batch:", err)
		return 2
	}
	output, err := privateDirectory(o.output, true)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "review-batch:", err)
		return 2
	}
	defer func() { _ = output.Close() }()
	var cache *os.Root
	if o.cache != "" {
		cache, err = privateDirectory(o.cache, false)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "review-batch:", err)
			return 2
		}
		defer func() { _ = cache.Close() }()
	}
	r, shared := makePlan(o, p, diffs)
	if !o.plan {
		execute(ctx, &r, p, shared, key, cache, stderr)
	}
	r.updateCoverage()
	if err := writeReport(output, r, stdout); err != nil {
		_, _ = fmt.Fprintln(stderr, "review-batch: cannot publish complete report artifacts")
		return 2
	}
	if o.plan || r.Status == "complete" {
		return 0
	}
	return 1
}

func execute(ctx context.Context, r *report, p policy, shared, key string, cache *os.Root, diagnostics io.Writer) {
	if len(r.Batches) == 0 {
		r.Status, r.Error = "skipped", "no text-reviewable changes; uncovered items are listed"
		return
	}
	if len(r.Batches) > p.MaxBatches {
		r.Status, r.Error = "skipped", "planned batches exceed REVIEW_MAX_BATCHES; increase the explicit allowance or review a smaller change"
		return
	}
	if strings.TrimSpace(key) == "" {
		r.Status, r.Error = "failed", "missing review API key"
		return
	}
	usageUnknown := false
	for i := range r.Batches {
		b := &r.Batches[i]
		if ctx.Err() != nil {
			r.Error = "review canceled or total deadline exceeded"
			break
		}
		if saved, ok := loadCache(ctx, cache, b.ID, key); ok {
			b.Status, b.Cached, b.Findings, b.Usage = "complete", true, saved.Findings, saved.Usage
			continue
		}
		body := request(p, shared, *b)
		if len(body) > 1<<20 {
			b.Status, b.Error = "failed", "shared context and batch exceed the request limit"
			r.Error = b.Error
			break
		}
		var findings bytes.Buffer
		var usage json.RawMessage
		budgetBlocked := false
		requestsBefore := r.Budget.RequestsUsed
		err := review.RunWithOptions(ctx, bytes.NewReader(body), &findings, diagnostics, review.Options{
			APIKey: key, TimeoutSeconds: p.TimeoutSeconds, HTTPRetries: p.HTTPRetries,
			BeforeAttempt: func(requestBytes, maxTokens int) error {
				if r.Budget.RequestsUsed+1 > r.Budget.MaxRequests || maxTokens > r.Budget.MaxTokens-r.Budget.TokensReserved || requestBytes > r.Budget.MaxInput-r.Budget.InputReserved {
					budgetBlocked = true
					return errors.New("run-wide review budget exhausted")
				}
				r.Budget.RequestsUsed++
				r.Budget.TokensReserved += maxTokens
				r.Budget.InputReserved += requestBytes
				return nil
			},
			OnUsage: func(value json.RawMessage) { usage = reportedUsage(value) },
		})
		if err != nil || findings.Len() > 2<<20 {
			if r.Budget.RequestsUsed > requestsBefore {
				r.Usage = nil
			}
			b.Status, b.Error = "failed", "review request failed without complete findings"
			if budgetBlocked {
				b.Status, b.Error = "skipped", "run-wide review budget exhausted"
			} else if ctx.Err() != nil {
				b.Error = "review canceled or total deadline exceeded"
			}
			r.Error = b.Error
			break
		}
		b.Status, b.Findings, b.Usage = "complete", strings.ReplaceAll(findings.String(), key, "[REDACTED]"), usage
		if len(usage) == 0 {
			usageUnknown = true
			r.Usage = nil
		} else if !usageUnknown {
			r.Usage = addUsage(r.Usage, usage)
			usageUnknown = len(r.Usage) == 0
		}
		if err := saveCache(cache, *b, key); err != nil {
			_, _ = fmt.Fprintln(diagnostics, "review-batch: successful review retained; cache publication unavailable")
		}
	}
	r.updateCoverage()
	switch {
	case r.Counts.FilesReviewed == r.Counts.FilesTotal && r.Counts.FilesTotal > 0 && r.Counts.BatchesCompleted == r.Counts.BatchesPlanned:
		r.Status = "complete"
	case r.Counts.BatchesCompleted > 0:
		r.Status = "partial"
	default:
		r.Status = "failed"
	}
	for i := range r.Batches {
		if r.Batches[i].Status == "planned" {
			r.Batches[i].Status, r.Batches[i].Error = "skipped", "not attempted after an incomplete batch"
		}
	}
}
