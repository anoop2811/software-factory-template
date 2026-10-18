// Package reviewbatch provides bounded, exact-revision adversarial review.
// docs/adr/0100-batched-review.md:17.
package reviewbatch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/config"
	"github.com/spf13/pflag"
)

type options struct {
	diff, files, base, head, repository, output, cache string
	plan                                               bool
}

type policy struct {
	Model          string         `json:"model"`
	Reasoning      string         `json:"reasoning"`
	Provider       map[string]any `json:"provider,omitempty"`
	MaxTokens      int            `json:"max_tokens"`
	BatchBytes     int            `json:"batch_bytes"`
	MaxBatches     int            `json:"max_batches"`
	MaxRequests    int            `json:"max_requests"`
	TotalTokens    int            `json:"total_max_tokens"`
	TotalInput     int            `json:"total_input_bytes"`
	TimeoutSeconds int            `json:"timeout_seconds"`
	HTTPRetries    int            `json:"http_retries"`
	Prompt         string         `json:"prompt"`
}

var revisionPattern = regexp.MustCompile(`^([a-f0-9]{40}|[a-f0-9]{64})$`)
var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var providerPattern = regexp.MustCompile(`^[a-z0-9]+([-/][a-z0-9]+)*$`)

func parseOptions(args []string, stderr io.Writer) (options, error) {
	var o options
	f := pflag.NewFlagSet("review-batch", pflag.ContinueOnError)
	f.SetOutput(stderr)
	f.StringVar(&o.diff, "diff", "", "Complete Git unified diff")
	f.StringVar(&o.files, "files", "", "Complete GitHub files manifest")
	f.StringVar(&o.base, "base", "", "Exact base commit")
	f.StringVar(&o.head, "head", "", "Exact head commit")
	f.StringVar(&o.repository, "repository", "", "Repository owner/name")
	f.StringVar(&o.output, "output", "", "New private report directory")
	f.StringVar(&o.cache, "cache", "", "Private authenticated cache directory")
	f.BoolVar(&o.plan, "plan", false, "Plan offline without contacting a provider")
	if err := f.Parse(args); err != nil {
		return o, errors.New("invalid review-batch arguments")
	}
	if f.NArg() != 0 || o.diff == "" || o.files == "" || o.output == "" || !revisionPattern.MatchString(o.base) || !revisionPattern.MatchString(o.head) || !repositoryPattern.MatchString(o.repository) {
		return o, errors.New("review-batch requires diff, files, repository, exact base/head and a new output directory")
	}
	return o, nil
}

func readPolicy(ctx context.Context, environment map[string]string) (policy, string, error) {
	e := maps.Clone(environment)
	if e == nil {
		e = make(map[string]string)
	}
	path := e["FACTORY_CONFIG"]
	if path == "" {
		path = filepath.Join(".", "factory.yaml")
	}
	var preserved []string
	for key := range e {
		if config.KnownExportKey(key) {
			preserved = append(preserved, key)
		}
	}
	actions, err := config.ExportPlan(ctx, path, preserved)
	if err != nil {
		return policy{}, "", errors.New("cannot load review configuration")
	}
	for _, action := range actions {
		if !action.ExportOnly {
			e[action.Key] = action.Value
		}
	}
	if e["MODEL_PROVIDER"] != "" && e["MODEL_PROVIDER"] != "openrouter" {
		return policy{}, "", errors.New("batched review requires the OpenRouter provider")
	}
	p := policy{Model: e["REVIEW_MODEL"], Reasoning: e["REVIEW_REASONING_EFFORT"], Prompt: systemPrompt}
	if p.Model == "" {
		p.Model = strings.TrimPrefix(e["OPENCODE_FRONTIER_MODEL"], "openrouter/")
	}
	if strings.TrimSpace(p.Model) == "" || len(p.Model) > 256 || strings.ContainsAny(p.Model, "\r\n\x00") {
		return p, "", errors.New("missing or invalid review model")
	}
	for _, item := range []struct {
		key                string
		value              *int
		fallback, min, max int
	}{
		{"REVIEW_MAX_TOKENS", &p.MaxTokens, 8192, 1024, 32768},
		{"REVIEW_BATCH_BYTES", &p.BatchBytes, 60000, 4096, 100000},
		{"REVIEW_MAX_BATCHES", &p.MaxBatches, 16, 1, 64},
		{"REVIEW_MAX_REQUESTS", &p.MaxRequests, 16, 1, 64},
		{"REVIEW_TOTAL_MAX_TOKENS", &p.TotalTokens, 524288, 1024, 2097152},
		{"REVIEW_TOTAL_INPUT_BYTES", &p.TotalInput, 2097152, 4096, 16777216},
		{"REVIEW_TIMEOUT_SECONDS", &p.TimeoutSeconds, 1200, 1, 1200},
		{"REVIEW_HTTP_RETRIES", &p.HTTPRetries, 0, 0, 1},
	} {
		*item.value, err = integer(e[item.key], item.fallback, item.min, item.max)
		if err != nil {
			return p, "", errors.New("invalid " + item.key)
		}
	}
	switch p.Reasoning {
	case "", "none", "minimal", "low", "medium", "high", "xhigh", "max":
	default:
		return p, "", errors.New("invalid review reasoning effort")
	}
	p.Provider, err = routing(e)
	return p, e["REVIEW_API_KEY"], err
}

func integer(text string, fallback, lower, upper int) (int, error) {
	if text == "" {
		return fallback, nil
	}
	if strings.Trim(text, "0123456789") != "" {
		return 0, errors.New("invalid integer")
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < lower || n > upper {
		return 0, errors.New("integer out of range")
	}
	return n, nil
}

func routing(e map[string]string) (map[string]any, error) {
	primary, fallback := e["REVIEW_OPENROUTER_PROVIDER"], e["REVIEW_FALLBACK_PROVIDERS"]
	if primary == "" && fallback == "" && e["REVIEW_MAX_PROMPT_PRICE"] == "" && e["REVIEW_MAX_COMPLETION_PRICE"] == "" {
		return nil, nil
	}
	order := []string{primary}
	if fallback != "" {
		order = append(order, strings.Split(fallback, ",")...)
	}
	seen := map[string]bool{}
	for _, slug := range order {
		if !providerPattern.MatchString(slug) || seen[slug] {
			return nil, errors.New("invalid review provider allowlist")
		}
		seen[slug] = true
	}
	if len(order) > 4 {
		return nil, errors.New("review provider allowlist exceeds four providers")
	}
	route := map[string]any{"order": order, "allow_fallbacks": len(order) > 1, "require_parameters": true}
	if len(order) > 1 || e["REVIEW_MAX_PROMPT_PRICE"] != "" || e["REVIEW_MAX_COMPLETION_PRICE"] != "" {
		prices := map[string]json.Number{}
		for _, item := range []struct{ env, field string }{{"REVIEW_MAX_PROMPT_PRICE", "prompt"}, {"REVIEW_MAX_COMPLETION_PRICE", "completion"}} {
			text := e[item.env]
			value, err := strconv.ParseFloat(text, 64)
			if err != nil || !regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`).MatchString(text) || value <= 0 || math.IsInf(value, 0) || math.IsNaN(value) {
				return nil, errors.New("provider fallback requires positive prompt and completion price ceilings")
			}
			prices[item.field] = json.Number(text)
		}
		route["only"] = order
		route["max_price"] = prices
	}
	return route, nil
}
