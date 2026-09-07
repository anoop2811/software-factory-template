// Package config implements the candidate flat configuration readers.
// Configuration remains inert data. specs/001-go-runtime-conversion.md:103.
package config

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// File preserves explicit configuration paths and the legacy Git-root fallback.
// The private candidate boundary is defined in docs/DECISION_LOG.md:1952.
func File(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if path := os.Getenv("FACTORY_CONFIG"); path != "" {
		return path, nil
	}
	output, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		// The shell appends echo's dot after any stdout produced by failed Git.
		output = append(output, '.', '\n')
	}
	return strings.TrimRight(string(output), "\n") + "/factory.yaml", nil
}

// Get returns the first matching value, or fallback when absent or empty.
// A read error accompanies fallback so the caller can preserve its diagnostic
// independently of the legacy successful get exit status.
func Get(ctx context.Context, path, key, fallback string) (string, error) {
	value, found, err := firstValue(ctx, path, key)
	if err != nil || !found {
		return fallback, err
	}
	value = strings.TrimLeft(value, " \t\r\v\f")
	// Bash command substitution drops NUL bytes after sed removes leading space.
	value = strings.ReplaceAll(value, "\x00", "")
	if strings.HasPrefix(value, `"`) {
		value = value[1:]
		if end := strings.IndexByte(value, '"'); end >= 0 {
			value = value[:end]
		}
	} else {
		for i := 1; i < len(value); i++ {
			if value[i] == '#' && asciiSpace(value[i-1]) {
				value = value[:i-1]
				break
			}
		}
		value = strings.TrimRight(value, " \t\r\v\f")
	}
	if value == "" {
		return fallback, nil
	}
	return value, nil
}

// Has distinguishes an explicitly empty setting from an absent setting.
func Has(ctx context.Context, path, key string) (bool, error) {
	_, found, err := firstValue(ctx, path, key)
	return found, err
}

func firstValue(ctx context.Context, path, key string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false, nil
	}
	// #nosec G304 -- The caller explicitly selects a read-only configuration file, as with the sourceable legacy reader.
	file, err := os.Open(path)
	if err != nil {
		return "", false, fmt.Errorf("read configuration: %w", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	prefix := key + ":"
	for {
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		line, err := reader.ReadString('\n')
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSuffix(line[len(prefix):], "\n"), true, nil
		}
		if errors.Is(err, io.EOF) {
			return "", false, nil
		}
		if err != nil {
			return "", false, fmt.Errorf("read configuration: %w", err)
		}
	}
}

func asciiSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\v' || value == '\f'
}
