package reviewbatch

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// Run returns success only for an offline plan or complete exact-commit coverage.
// docs/adr/0100-batched-review.md:76.
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
	runBatches(ctx, r, p, shared, key, cache, diagnostics)
	r.updateCoverage()
	complete := r.Counts.FilesReviewed == r.Counts.FilesTotal && r.Counts.FilesTotal > 0 && r.Counts.BatchesCompleted == r.Counts.BatchesPlanned
	switch {
	case ctx.Err() != nil && complete:
		r.Status, r.Error = "failed", "review canceled or total deadline exceeded"
	case complete:
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
