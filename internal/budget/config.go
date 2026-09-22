package budget

import (
	"errors"
	"math/big"
	"strings"
	"unicode/utf8"
)

// Configuration preserves the environment-only budget settings and defaults.
// docs/adr/0069-go-budget-ledger-admission.md:36.
func Configuration(environment map[string]string) (Config, error) {
	get := func(key, fallback string) string {
		if value, ok := environment["FACTORY_BUDGET_"+key]; ok {
			return value
		}
		return fallback
	}
	enabled, action := get("ENABLED", "false"), get("ACTION", "stop")
	if enabled != "true" && enabled != "false" {
		return Config{}, errors.New("budget_enabled must be true or false")
	}
	if action != "stop" && action != "warn" {
		return Config{}, errors.New("budget_action must be stop or warn")
	}
	c := Config{Enabled: enabled == "true", Action: action}
	for _, field := range []struct {
		key, defaultValue string
		target            **big.Int
	}{{"MAX_ATTEMPTS", "1", &c.MaxAttempts}, {"MAX_SESSION_RUNS", "5", &c.MaxSessionRuns}, {"MAX_CONCURRENT", "1", &c.MaxConcurrent}} {
		raw := get(field.key, field.defaultValue)
		if raw == "" || strings.Trim(raw, "0123456789") != "" || len(strings.TrimLeft(raw, "0")) > 309 {
			return Config{}, errors.New("invalid budget integer configuration")
		}
		value, ok := new(big.Int).SetString(raw, 10)
		if !ok || value.Sign() <= 0 {
			return Config{}, errors.New("invalid budget integer configuration")
		}
		f, _ := value.Float64()
		if !finite(f) {
			return Config{}, errors.New("invalid budget integer configuration")
		}
		*field.target = value
	}
	var err error
	c.TimeoutSeconds, err = pythonFloat(get("TIMEOUT_SECONDS", "300"))
	if err != nil {
		return Config{}, err
	}
	c.SessionSeconds, err = pythonFloat(get("SESSION_SECONDS", "900"))
	if err != nil {
		return Config{}, err
	}
	if raw := get("ESTIMATED_USD", ""); raw != "" {
		value, err := pythonFloat(raw)
		if err != nil {
			return Config{}, err
		}
		c.EstimatedUSD = &value
	}
	return c, nil
}
func validateConfig(c Config) (Config, error) {
	if c.Action != "stop" && c.Action != "warn" {
		return Config{}, errors.New("invalid budget action")
	}
	for _, i := range []*big.Int{c.MaxAttempts, c.MaxSessionRuns, c.MaxConcurrent} {
		if i == nil || i.Sign() <= 0 {
			return Config{}, errors.New("invalid budget limits")
		}
		f, _ := i.Float64()
		if !finite(f) {
			return Config{}, errors.New("invalid budget limits")
		}
	}
	for _, f := range []float64{c.TimeoutSeconds, c.SessionSeconds} {
		if !finite(f) || f <= 0 {
			return Config{}, errors.New("invalid budget seconds")
		}
	}
	if c.EstimatedUSD != nil && (!finite(*c.EstimatedUSD) || *c.EstimatedUSD <= 0) {
		return Config{}, errors.New("invalid budget cost threshold")
	}
	c.MaxAttempts = new(big.Int).Set(c.MaxAttempts)
	c.MaxSessionRuns = new(big.Int).Set(c.MaxSessionRuns)
	c.MaxConcurrent = new(big.Int).Set(c.MaxConcurrent)
	if c.EstimatedUSD != nil {
		f := *c.EstimatedUSD
		c.EstimatedUSD = &f
	}
	return c, nil
}
func validID(s string) bool {
	if len(s) < 1 || len(s) > 64 {
		return false
	}
	for i, c := range []byte(s) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			continue
		}
		if i > 0 && (c == '_' || c == '-') {
			continue
		}
		return false
	}
	return true
}
func harness(s string) bool { return s == "codex" || s == "claude" || s == "opencode" }
func role(s string) bool {
	switch s {
	case "spec-writer", "implementer", "refactorer", "reviewer", "wiki-maintainer":
		return true
	default:
		return false
	}
}
func validateRequest(r Request) error {
	if !validID(r.Session) || !validID(r.Task) || !harness(r.Harness) || !role(r.Role) || !utf8.ValidString(r.Model) {
		return errors.New("invalid budget request")
	}
	if r.MaxCostUSD != nil {
		_, err := pythonFloat(*r.MaxCostUSD)
		return err
	}
	return nil
}
