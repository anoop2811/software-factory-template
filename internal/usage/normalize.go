// Package usage normalizes decoded native events into accounting metadata only.
// No native process is launched: docs/adr/0064-go-native-usage-accounting.md:8.
package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"unicode/utf8"
)

// Metadata deliberately excludes transcript and arbitrary event fields.
// docs/adr/0064-go-native-usage-accounting.md:40.
type Metadata struct {
	Tokens       map[string]*big.Int `json:"tokens"`
	EstimatedUSD *json.Number        `json:"estimated_usd"`
	Complete     bool                `json:"complete"`
	Failed       bool                `json:"failed"`
	Source       string              `json:"source"`
}

func Supported(harness string) bool {
	return harness == "codex" || harness == "claude" || harness == "opencode"
}

// Normalize reads one bounded array and preserves integer lexical information.
// Private input admission: docs/adr/0064-go-native-usage-accounting.md:17.
func Normalize(ctx context.Context, harness string, input io.Reader) (Metadata, error) {
	if !Supported(harness) {
		return Metadata{}, errors.New("unsupported usage harness")
	}
	data, err := io.ReadAll(io.LimitReader(&contextReader{ctx, input}, (16<<20)+1))
	if err != nil || len(data) > 16<<20 || !utf8.Valid(data) || !json.Valid(data) {
		return Metadata{}, errors.New("invalid usage event input")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return Metadata{}, errors.New("invalid usage event input")
	}
	events, ok := decoded.([]any)
	if !ok {
		return Metadata{}, errors.New("invalid usage event input")
	}
	result := normalizeEvents(ctx, harness, events)
	if ctx.Err() != nil {
		return Metadata{}, errors.New("invalid usage event input")
	}
	return result, nil
}

func normalizeEvents(ctx context.Context, harness string, events []any) Metadata {
	switch harness {
	case "codex":
		return codex(ctx, events)
	case "claude":
		return claude(ctx, events)
	case "opencode":
		return opencode(ctx, events)
	default:
		return Metadata{}
	}
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func object(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func text(value any) string {
	result, _ := value.(string)
	return result
}

// Last-result selection and failure/accounting independence are intentional.
// docs/adr/0064-go-native-usage-accounting.md:48.
func codex(ctx context.Context, events []any) Metadata {
	result := Metadata{Source: "codex.turn.completed.usage; cost unavailable"}
	malformed := false
	var final map[string]any
	for _, event := range events {
		if ctx.Err() != nil {
			return result
		}
		row := object(event)
		if row == nil {
			malformed = true
			continue
		}
		switch text(row["type"]) {
		case "turn.failed", "error":
			result.Failed = true
		case "turn.completed":
			final = row
		}
	}
	if final != nil {
		result.Tokens = tokens(final["usage"], []string{"input_tokens", "cached_input_tokens", "output_tokens"}, "reasoning_output_tokens")
		result.Complete = result.Tokens != nil && !result.Failed && !malformed
	}
	return result
}

func claude(ctx context.Context, events []any) Metadata {
	result := Metadata{Source: "claude.result.usage (main agent); total_cost_usd (client estimate)"}
	malformed := false
	var final map[string]any
	for _, event := range events {
		if ctx.Err() != nil {
			return result
		}
		row := object(event)
		if row == nil {
			malformed = true
			continue
		}
		switch text(row["type"]) {
		case "error":
			result.Failed = true
		case "result":
			final = row
		}
	}
	if final != nil {
		subtype := text(final["subtype"])
		isError, _ := final["is_error"].(bool)
		result.Failed = result.Failed || subtype != "success" || isError
		result.Tokens = tokens(final["usage"], []string{"input_tokens", "output_tokens", "cache_creation_input_tokens", "cache_read_input_tokens"})
		cost, _, valid := finiteNumber(final["total_cost_usd"])
		if valid && subtype != "error_during_execution" && !malformed {
			result.EstimatedUSD = &cost
		}
		result.Complete = result.Tokens != nil && result.EstimatedUSD != nil
	}
	return result
}
