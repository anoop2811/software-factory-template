// Package commandenv composes the effective environment of the legacy wrappers.
package commandenv

import (
	"context"
	"errors"
	"maps"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/config"
	"github.com/anoop2811/software-factory-template/internal/roles"
)

type setting struct{ key, fallback string }

func environmentError() error { return errors.New("cannot prepare command environment") }

// Budget prepares wrapper configuration and selects a model using literal argv.
// It never mutates the supplied environment. docs/adr/0077-go-command-environment.md:62.
func Budget(ctx context.Context, args []string, environment map[string]string) (map[string]string, error) {
	effective, _, err := base(ctx, environment)
	if err != nil {
		return nil, err
	}
	harness, role := "", "implementer"
	for i, arg := range args {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		next := ""
		if i+1 < len(args) {
			next = args[i+1]
		}
		switch {
		case arg == "--harness":
			harness = next
		case strings.HasPrefix(arg, "--harness="):
			harness = strings.TrimPrefix(arg, "--harness=")
		case arg == "--role":
			role = next
		case strings.HasPrefix(arg, "--role="):
			role = strings.TrimPrefix(arg, "--role=")
		}
	}
	effective["FACTORY_BUDGET_MODEL"] = model(effective, harness, role)
	return effective, nil
}

// Loop prepares the shared budget values and all six loop role models.
// docs/adr/0077-go-command-environment.md:73.
func Loop(ctx context.Context, environment map[string]string) (map[string]string, error) {
	effective, path, err := base(ctx, environment)
	if err != nil {
		return nil, err
	}
	if err := settings(ctx, effective, path, "loop", []setting{
		{"enabled", "false"}, {"max_attempts", "2"}, {"timeout_seconds", "900"},
		{"check_timeout_seconds", "120"}, {"no_progress_limit", "1"},
	}); err != nil {
		return nil, err
	}
	fallback, err := get(ctx, path, "check_command", "")
	if err != nil {
		return nil, err
	}
	check, err := get(ctx, path, "loop_check_command", fallback)
	if err != nil {
		return nil, err
	}
	effective["FACTORY_LOOP_CHECK_COMMAND"] = check
	for _, item := range []struct{ key, variable string }{
		{"test_file_patterns", "FACTORY_LOOP_TEST_PATTERNS"},
		{"protected_paths", "FACTORY_LOOP_PROTECTED_PATHS"},
	} {
		value, err := get(ctx, path, item.key, "")
		if err != nil {
			return nil, err
		}
		effective[item.variable] = value
	}
	effective["FACTORY_LOOP_CONFIG_PATH"] = path
	for _, harness := range []string{"codex", "claude", "opencode"} {
		for _, role := range []string{"implementer", "reviewer"} {
			effective["FACTORY_LOOP_"+strings.ToUpper(harness+"_"+role)+"_MODEL"] = model(effective, harness, role)
		}
	}
	return effective, nil
}

func base(ctx context.Context, environment map[string]string) (map[string]string, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	// An absent PATH would require Bash's platform-specific compiled default.
	// The private candidate requires explicit PATH, including an explicit empty one.
	// docs/adr/0077-go-command-environment.md:95.
	if _, present := environment["PATH"]; !present {
		return nil, "", environmentError()
	}
	effective := maps.Clone(environment)
	if effective == nil {
		effective = make(map[string]string)
	}
	root, path, err := discover(ctx, environment)
	if err != nil {
		return nil, "", err
	}
	effective["FACTORY_BUDGET_ROOT"] = root
	if err := settings(ctx, effective, path, "budget", []setting{
		{"enabled", "false"}, {"max_attempts", "1"}, {"max_session_runs", "5"},
		{"timeout_seconds", "300"}, {"session_seconds", "900"}, {"max_concurrent", "1"},
		{"estimated_usd", ""}, {"action", "stop"},
	}); err != nil {
		return nil, "", err
	}
	var preserved []string
	for key := range environment {
		if config.KnownExportKey(key) {
			preserved = append(preserved, key)
		}
	}
	actions, err := config.ExportPlan(ctx, path, preserved)
	if err != nil {
		return nil, "", preparationError(ctx)
	}
	for _, action := range actions {
		if !action.ExportOnly {
			effective[action.Key] = action.Value
		}
	}
	return effective, path, nil
}
func settings(ctx context.Context, environment map[string]string, path, prefix string, values []setting) error {
	for _, item := range values {
		value, err := get(ctx, path, prefix+"_"+item.key, item.fallback)
		if err != nil {
			return err
		}
		environment["FACTORY_"+strings.ToUpper(prefix+"_"+item.key)] = value
	}
	return nil
}
func get(ctx context.Context, path, key, fallback string) (string, error) {
	value, err := config.Get(ctx, path, key, fallback)
	if err != nil {
		return "", preparationError(ctx)
	}
	return substitution(value), nil
}
func substitution(value string) string {
	return strings.TrimRight(strings.ReplaceAll(value, "\x00", ""), "\n")
}
func model(environment map[string]string, harness, role string) string {
	switch harness {
	case "codex", "claude", "opencode":
	default:
		return ""
	}
	profile := environment["COST_PROFILE"]
	if profile == "" {
		profile = "standard"
	}
	tier := roles.Resolve(profile, roles.Tier(role))
	return substitution(environment[strings.ToUpper(harness+"_"+tier)+"_MODEL"])
}
func preparationError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return environmentError()
}
