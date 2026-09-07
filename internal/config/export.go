package config

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// exportKeys is the fixed configuration contract. The Bash adapter retains the
// same names as data to inspect caller-local variables. docs/adr/0056-go-configuration-export-plans.md:25.
var exportKeys = []string{
	"COST_PROFILE", "MODEL_PROVIDER",
	"OPENCODE_FRONTIER_MODEL", "OPENCODE_DEFAULT_MODEL", "OPENCODE_ECONOMY_MODEL",
	"CLAUDE_FRONTIER_MODEL", "CLAUDE_DEFAULT_MODEL", "CLAUDE_ECONOMY_MODEL",
	"CODEX_FRONTIER_MODEL", "CODEX_DEFAULT_MODEL", "CODEX_ECONOMY_MODEL",
	"REVIEW_LANE", "REVIEW_MODEL", "REVIEW_API_KEY_SECRET",
	"REVIEW_REASONING_EFFORT", "REVIEW_OPENROUTER_PROVIDER",
}

// Action contains data for a parent-shell assignment or export, never shell code.
// Ordered effects and caller preservation: docs/adr/0056-go-configuration-export-plans.md:17.
type Action struct {
	Key        string
	Value      string
	ExportOnly bool
}

func KnownExportKey(key string) bool {
	for _, known := range exportKeys {
		if key == known {
			return true
		}
	}
	return false
}

func action(key, value string, preserved []string) Action {
	for _, caller := range preserved {
		if key == caller {
			return Action{Key: key, ExportOnly: true}
		}
	}
	return Action{Key: key, Value: value}
}

// ExportPlan reads legacy entries first and overlays nonempty YAML values.
// Caller values are not serialized in the plan. docs/adr/0056-go-configuration-export-plans.md:33.
func ExportPlan(ctx context.Context, path string, preserved []string) ([]Action, error) {
	var actions []Action
	legacy := legacyPath(path)
	if info, err := os.Stat(legacy); err == nil && info.Mode().IsRegular() {
		var err error
		actions, err = LegacyPlan(ctx, legacy, preserved)
		if err != nil {
			return nil, err
		}
	}
	for _, key := range exportKeys {
		value, err := Get(ctx, path, strings.ToLower(key), "")
		if err != nil {
			return nil, err
		}
		if value != "" {
			actions = append(actions, action(key, value, preserved))
		}
	}
	return actions, nil
}

// Preserve dirname's lexical path: cleaning an interior '..' can cross a
// different directory when an earlier component is a symlink.
func legacyPath(path string) string {
	trimmed := strings.TrimRight(path, "/")
	separator := strings.LastIndexByte(trimmed, '/')
	directory := "."
	if trimmed == "" || separator == 0 {
		directory = "/"
	} else if separator > 0 {
		directory = strings.TrimRight(trimmed[:separator], "/")
		if directory == "" {
			directory = "/"
		}
	}
	// The existing shell captures dirname using command substitution.
	return strings.TrimRight(directory, "\n") + "/factory.config"
}

func LegacyPlan(ctx context.Context, path string, preserved []string) ([]Action, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// #nosec G304 -- Explicit read-only configuration input, with the legacy regular-symlink contract.
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read legacy configuration: %w", err)
	}
	defer file.Close()
	var actions []Action
	reader := bufio.NewReader(file)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, readErr := reader.ReadString('\n')
		// Follow modern Bash NUL normalization; Bash 3.2 parity remains open.
		// docs/adr/0056-go-configuration-export-plans.md:97. EOF still ends a record.
		line = strings.ReplaceAll(strings.TrimSuffix(line, "\n"), "\x00", "")
		if key, value, ok := legacyEntry(line); ok {
			actions = append(actions, action(key, value, preserved))
		}
		if readErr == io.EOF {
			return actions, nil
		}
		if readErr != nil {
			return nil, fmt.Errorf("read legacy configuration %q: %w", path, readErr)
		}
	}
}

func legacyEntry(line string) (string, string, bool) {
	if strings.HasPrefix(line, "#") {
		return "", "", false
	}
	key, value, present := strings.Cut(line, "=")
	if !present {
		return "", "", false
	}
	key = strings.TrimPrefix(key, "export ")
	key = strings.Map(func(r rune) rune {
		if strings.ContainsRune(" \t\r\v\f", r) {
			return -1
		}
		return r
	}, key)
	for _, character := range key {
		if character > 127 {
			return "", "", false
		}
	}
	key = strings.ToUpper(key)
	if !KnownExportKey(key) {
		return "", "", false
	}
	if strings.HasPrefix(value, `"`) || strings.HasPrefix(value, `'`) {
		quote := value[0]
		value = value[1:]
		if end := strings.IndexByte(value, quote); end >= 0 {
			value = value[:end]
		}
	} else {
		for i := 1; i < len(value); i++ {
			if value[i] == '#' && asciiSpace(value[i-1]) {
				value = value[:i-1]
				break
			}
		}
		value = strings.Trim(value, " \t\r\v\f")
	}
	return key, value, true
}

// WritePlan frames the complete action list; readers validate it before mutation.
// Values cannot contain LF in the flat format. docs/adr/0056-go-configuration-export-plans.md:42.
func WritePlan(out io.Writer, actions []Action) error {
	var plan strings.Builder
	plan.WriteString("FACTORY_CONFIG_PLAN_V1\n")
	for _, item := range actions {
		if !KnownExportKey(item.Key) || strings.ContainsAny(item.Value, "\n\x00") {
			return fmt.Errorf("invalid configuration plan action")
		}
		if item.ExportOnly {
			fmt.Fprintf(&plan, "export\t%s\n", item.Key)
		} else {
			fmt.Fprintf(&plan, "set\t%s\t%s\n", item.Key, item.Value)
		}
	}
	plan.WriteString("END\n")
	_, err := io.WriteString(out, plan.String())
	return err
}
