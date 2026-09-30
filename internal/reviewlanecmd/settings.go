package reviewlanecmd

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/config"
	"github.com/anoop2811/software-factory-template/internal/fileinput"
	"github.com/anoop2811/software-factory-template/internal/native"
)

const fileLimit = 16 << 20

type Settings struct{ Values map[string]string }

// SettingsFromBytes retains the shared export grammar and caller precedence.
// docs/adr/0089-go-native-review-lane.md:26.
func SettingsFromBytes(ctx context.Context, yaml, legacy []byte, environment map[string]string) (Settings, error) {
	var preserved []string
	for key := range environment {
		if config.KnownExportKey(key) {
			preserved = append(preserved, key)
		}
	}
	actions, err := config.ExportPlanBytes(ctx, yaml, legacy, preserved)
	if err != nil {
		return Settings{}, err
	}
	effective := maps.Clone(environment)
	if effective == nil {
		effective = map[string]string{}
	}
	for _, action := range actions {
		if !action.ExportOnly {
			effective[action.Key] = action.Value
		}
	}
	return Settings{Values: effective}, nil
}
func optionalInput(ctx context.Context, path string) ([]byte, error) {
	data, err := fileinput.Read(ctx, path, fileLimit)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}
func pathAt(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return root + "/" + path
}
func load(ctx context.Context, root string, environment map[string]string) (Settings, string, error) {
	selected := environment["FACTORY_CONFIG"]
	if selected == "" {
		selected = "factory.yaml"
	}
	path := pathAt(root, selected)
	yaml, err := optionalInput(ctx, path)
	if err != nil {
		return Settings{}, path, err
	}
	legacy, err := optionalInput(ctx, config.LegacyPath(path))
	if err != nil {
		return Settings{}, path, err
	}
	settings, err := SettingsFromBytes(ctx, yaml, legacy, environment)
	return settings, path, err
}
func (s Settings) value(key, fallback string) string {
	if v := s.Values[key]; v != "" {
		return v
	}
	return fallback
}
func (s Settings) Enabled() bool { return s.value("REVIEW_LANE", "off") == "on" }
func (s Settings) Secret() string {
	fallback := "OPENROUTER_API_KEY"
	switch s.value("MODEL_PROVIDER", "openrouter") {
	case "anthropic":
		fallback = "ANTHROPIC_API_KEY"
	case "openai":
		fallback = "OPENAI_API_KEY"
	}
	return s.value("REVIEW_API_KEY_SECRET", fallback)
}
func (s Settings) model() string {
	switch s.value("MODEL_PROVIDER", "openrouter") {
	case "anthropic":
		return s.value("CLAUDE_FRONTIER_MODEL", "claude-opus-4-8")
	case "openai":
		return s.value("CODEX_FRONTIER_MODEL", "gpt-5.6-sol")
	default:
		return s.value("OPENCODE_FRONTIER_MODEL", "openrouter/z-ai/glm-5.2")
	}
}
func unsafe(result native.CommandResult) bool {
	return result.OwnershipUnconfirmed || result.Outcome == "timeout" || result.Outcome == "interrupted" || result.Outcome == "output_limit"
}

// SecretStatus performs one bounded read-only query, preserving ordinary unknowns.
// docs/adr/0089-go-native-review-lane.md:35.
func SecretStatus(ctx context.Context, root string, s Settings) (string, error) {
	if !s.Enabled() {
		return "n/a", nil
	}
	result, err := native.ExecuteCommand(ctx, root, []string{"gh", "secret", "list"}, s.Values, 10*time.Second)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if unsafe(result) {
		return "", errors.New("review lane secret query did not complete safely")
	}
	if err != nil || result.ExitCode == nil || *result.ExitCode != 0 {
		//nolint:nilerr // Ordinary query failure is unknown: docs/adr/0089-go-native-review-lane.md:37.
		return "unknown", nil
	}
	for _, line := range strings.Split(string(result.Stdout), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.EqualFold(fields[0], s.Secret()) {
			return "set", nil
		}
	}
	return "missing", nil
}
