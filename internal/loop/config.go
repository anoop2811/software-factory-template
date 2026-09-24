// Package loop provides the read-only loop identity foundation.
package loop

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"time"
	"unicode"

	"github.com/anoop2811/software-factory-template/internal/budget"
	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
)

type Config struct {
	Enabled             bool     `json:"enabled"`
	CheckCommand        string   `json:"check_command"`
	TestPatterns        []string `json:"test_patterns"`
	ProtectedPaths      []string `json:"protected_paths"`
	MaxAttempts         *big.Int `json:"max_attempts"`
	TimeoutSeconds      float64  `json:"timeout_seconds"`
	CheckTimeoutSeconds float64  `json:"check_timeout_seconds"`
	NoProgressLimit     *big.Int `json:"no_progress_limit"`
}
type Fingerprint struct {
	Head   string `json:"head"`
	Source string `json:"source"`
	Safety string `json:"safety"`
}

func loopError() error { return errors.New("cannot establish loop fingerprint") }

// Configuration reuses numeric grammar and the actual caller's POSIX ERE engine.
// docs/adr/0073-go-loop-fingerprint-foundation.md:32.
func Configuration(ctx context.Context, environment map[string]string) (Config, error) {
	get := func(key, fallback string) string {
		if value, ok := environment["FACTORY_LOOP_"+key]; ok {
			return value
		}
		return fallback
	}
	enabled := get("ENABLED", "false")
	if enabled != "true" && enabled != "false" {
		return Config{}, loopError()
	}
	split := func(value string) []string {
		return strings.FieldsFunc(value, func(r rune) bool { return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f) })
	}
	c := Config{Enabled: enabled == "true", CheckCommand: get("CHECK_COMMAND", ""), TestPatterns: split(get("TEST_PATTERNS", "")), ProtectedPaths: split(get("PROTECTED_PATHS", ""))}
	var err error
	c.MaxAttempts, err = jsonvalue.PositiveInteger(get("MAX_ATTEMPTS", "2"))
	if err != nil {
		return Config{}, loopError()
	}
	c.NoProgressLimit, err = jsonvalue.PositiveInteger(get("NO_PROGRESS_LIMIT", "1"))
	if err != nil {
		return Config{}, loopError()
	}
	c.TimeoutSeconds, err = jsonvalue.PositiveFloat(get("TIMEOUT_SECONDS", "900"))
	if err != nil {
		return Config{}, loopError()
	}
	c.CheckTimeoutSeconds, err = jsonvalue.PositiveFloat(get("CHECK_TIMEOUT_SECONDS", "120"))
	if err != nil {
		return Config{}, loopError()
	}
	for _, pattern := range c.TestPatterns {
		_, code, err := probe(ctx, environment, 5*time.Second, []string{"grep", "-E", "-q", "--", pattern}, nil)
		if err != nil || (code != 0 && code != 1) {
			return Config{}, loopError()
		}
	}
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}
	return c, nil
}
func stringsValue(values []string) []any {
	result := make([]any, len(values))
	for i, value := range values {
		result[i] = jsonvalue.RawString(value)
	}
	return result
}
func (c Config) value() map[string]any {
	return map[string]any{"enabled": c.Enabled, "check_command": jsonvalue.RawString(c.CheckCommand), "test_patterns": stringsValue(c.TestPatterns), "protected_paths": stringsValue(c.ProtectedPaths), "max_attempts": c.MaxAttempts, "timeout_seconds": c.TimeoutSeconds, "check_timeout_seconds": c.CheckTimeoutSeconds, "no_progress_limit": c.NoProgressLimit}
}
func (c Config) MarshalJSON() ([]byte, error) {
	var buffer bytes.Buffer
	err := jsonvalue.EncodePython(context.Background(), &buffer, c.value())
	return buffer.Bytes(), err
}
func digest(ctx context.Context, value any) (string, error) {
	hash := sha256.New()
	if err := jsonvalue.EncodePython(ctx, hash, value); err != nil {
		return "", loopError()
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// Policy includes all configuration and only the selected additional environment.
// docs/adr/0073-go-loop-fingerprint-foundation.md:49.
func Policy(ctx context.Context, c Config, b budget.Config, environment map[string]string) (string, error) {
	models := map[string]any{}
	for key, value := range environment {
		if strings.HasPrefix(key, "FACTORY_LOOP_") && strings.HasSuffix(key, "_MODEL") {
			models[jsonvalue.RawString(key)] = jsonvalue.RawString(value)
		}
	}
	var cost any
	if b.EstimatedUSD != nil {
		cost = *b.EstimatedUSD
	}
	budgets := map[string]any{"enabled": b.Enabled, "action": b.Action, "max_attempts": b.MaxAttempts, "max_session_runs": b.MaxSessionRuns, "max_concurrent": b.MaxConcurrent, "timeout_seconds": b.TimeoutSeconds, "session_seconds": b.SessionSeconds, "estimated_usd": cost}
	return digest(ctx, map[string]any{"loop": c.value(), "budget": budgets, "models": models, "config_path": jsonvalue.RawString(environment["FACTORY_LOOP_CONFIG_PATH"]), "native_overlay": jsonvalue.RawString(environment["OPENCODE_CONFIG_CONTENT"])})
}
