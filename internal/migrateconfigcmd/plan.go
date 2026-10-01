package migrateconfigcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/config"
)

const fileLimit = 16 << 20
const whitespace = " \t\r\n\v\f"

type plan struct{ yaml, output []byte }

func setting(line string) (string, string, bool) {
	key, value, ok := strings.Cut(line, "=")
	if !ok || key == "" {
		return "", "", false
	}
	for _, c := range []byte(key) {
		valid := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
		if !valid {
			return "", "", false
		}
	}
	if strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'") {
		quote := value[0]
		value = value[1:]
		if at := strings.IndexByte(value, quote); at >= 0 {
			value = value[:at]
		}
	} else {
		for i := 1; i < len(value); i++ {
			if value[i] == '#' && strings.ContainsRune(whitespace, rune(value[i-1])) {
				value = value[:i-1]
				break
			}
		}
		value = strings.TrimRight(value, whitespace)
	}
	return key, value, true
}

// Apply observes evolving YAML; dry-run deliberately observes the original.
// docs/adr/0090-go-native-config-migration.md:42.
func buildPlan(ctx context.Context, yaml, legacy []byte, dry bool) (plan, error) {
	if bytes.IndexByte(yaml, 0) >= 0 || bytes.IndexByte(legacy, 0) >= 0 {
		return plan{}, errors.New("factory migrate-config: NUL input is not supported")
	}
	var output bytes.Buffer
	write := func(text string) error {
		if len(text) > fileLimit-output.Len() {
			return errors.New("factory migrate-config: output exceeds 16 MiB")
		}
		_, err := output.WriteString(text)
		return err
	}
	if err := write("Migrating factory.config into factory.yaml...\n"); err != nil {
		return plan{}, err
	}
	current := yaml
	moved, skipped, parsed := 0, 0, 0
	remaining := string(legacy)
	for remaining != "" {
		if err := ctx.Err(); err != nil {
			return plan{}, err
		}
		line, rest, _ := strings.Cut(remaining, "\n")
		remaining = rest
		key, value, ok := setting(line)
		if !ok {
			continue
		}
		parsed++
		if parsed > 4096 {
			return plan{}, errors.New("factory migrate-config: more than 4096 settings")
		}
		yamlKey := strings.ToLower(key)
		existing, err := config.GetBytes(ctx, current, yamlKey, "")
		if err != nil {
			return plan{}, err
		}
		if existing != "" {
			skipped++
			if err := write("  keeping yours: " + yamlKey + " (already set in factory.yaml)\n"); err != nil {
				return plan{}, err
			}
			continue
		}
		if strings.ContainsAny(value, "\r\n\"") {
			return plan{}, fmt.Errorf("factory migrate-config: value for %s cannot round-trip through the flat setter", yamlKey)
		}
		if dry {
			if err := write("  would add: " + yamlKey + ": \"" + value + "\"\n"); err != nil {
				return plan{}, err
			}
		} else {
			current, err = config.RewriteBytes(ctx, current, yamlKey, value)
			if err != nil {
				return plan{}, err
			}
			if err := write("  moved: " + key + " -> " + yamlKey + "\n"); err != nil {
				return plan{}, err
			}
		}
		moved++
	}
	var summary string
	if dry {
		summary = fmt.Sprintf("\nfactory migrate-config: dry run — %d key(s) would move, %d already set.\n  Nothing was changed. Re-run without --dry-run to apply.\n", moved, skipped)
	} else {
		var err error
		current, err = config.RewriteBytes(ctx, current, "config_migrated", "yes")
		if err != nil {
			return plan{}, err
		}
		summary = fmt.Sprintf("\nfactory migrate-config: %d key(s) moved, %d kept as you had them.\n  factory.config -> factory.config.migrated (renamed, not deleted)\n  Review the diff (git status), then commit. Nothing was committed for you.\n  If anything looks wrong: git checkout factory.yaml && git mv factory.config.migrated factory.config\n", moved, skipped)
	}
	if err := write(summary); err != nil {
		return plan{}, err
	}
	return plan{yaml: current, output: output.Bytes()}, nil
}
