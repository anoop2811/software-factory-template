package config

import (
	"context"
	"fmt"
	"strings"
)

// Hooks groups already-expanded shell operands without evaluating their content.
// Literal token and ASCII whitespace rules: docs/adr/0062-go-local-hook-normalization.md:20.
func Hooks(ctx context.Context, mode string, tokens []string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if mode != "words" && mode != "commas" {
		return "", fmt.Errorf("unsupported hooks mode %q", mode)
	}
	var output strings.Builder
	entry := false
	for _, token := range tokens {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if mode == "commas" {
			token = strings.Join(strings.FieldsFunc(token, func(r rune) bool {
				return strings.ContainsRune(" \t\n\r\v\f", r)
			}), " ")
			if token != "" {
				output.WriteString(token)
				output.WriteByte('\n')
			}
			continue
		}
		if strings.HasPrefix(token, "-") {
			if entry {
				output.WriteByte(' ')
				output.WriteString(token)
			}
			continue
		}
		if entry {
			output.WriteByte('\n')
		}
		output.WriteString(token)
		entry = token != ""
	}
	if entry {
		output.WriteByte('\n')
	}
	return output.String(), nil
}
