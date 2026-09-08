package usage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"unicode"
	"unicode/utf8"
)

// Parse interprets bounded native stdout, then uses the shared accounting logic.
// Syntax failures remain accounting evidence: docs/adr/0066-go-native-event-streams.md:24.
func Parse(ctx context.Context, harness string, input io.Reader) (Metadata, error) {
	if !Supported(harness) {
		return Metadata{}, errors.New("unsupported usage harness")
	}
	data, err := io.ReadAll(io.LimitReader(&contextReader{ctx, input}, (16<<20)+1))
	if err != nil || len(data) > 16<<20 || ctx.Err() != nil {
		return Metadata{}, errors.New("invalid usage stream input")
	}
	events, err := streamEvents(ctx, data)
	if err != nil {
		return Metadata{}, errors.New("invalid usage stream input")
	}
	result := normalizeEvents(ctx, harness, events)
	if ctx.Err() != nil {
		return Metadata{}, errors.New("invalid usage stream input")
	}
	return result, nil
}

// Whole-document decoding precedes Python's splitlines and blank-line filtering.
// docs/adr/0066-go-native-event-streams.md:32.
func streamEvents(ctx context.Context, data []byte) ([]any, error) {
	if !utf8.Valid(data) {
		return []any{nil}, nil
	}
	whole, err := decodePythonJSON(ctx, data)
	if err == nil {
		if object(whole) != nil {
			return []any{whole}, nil
		}
		return []any{nil}, nil
	}
	var syntax jsonSyntaxError
	if !errors.As(err, &syntax) {
		return nil, err
	}
	var events []any
	appendLine := func(line []byte) error {
		if len(bytes.TrimFunc(line, pythonSpace)) == 0 {
			return nil
		}
		value, err := decodePythonJSON(ctx, line)
		if err != nil && !errors.As(err, &syntax) {
			return err
		}
		if err != nil {
			value = nil
		}
		events = append(events, value)
		return nil
	}
	start, nextCheck := 0, 0
	for offset := 0; offset < len(data); {
		if offset >= nextCheck {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			nextCheck = offset + 4096
		}
		character, size := utf8.DecodeRune(data[offset:])
		if !pythonLineBreak(character) {
			offset += size
			continue
		}
		if err := appendLine(data[start:offset]); err != nil {
			return nil, err
		}
		offset += size
		if character == '\r' && offset < len(data) && data[offset] == '\n' {
			offset++
		}
		start = offset
	}
	if start < len(data) {
		if err := appendLine(data[start:]); err != nil {
			return nil, err
		}
	}
	if len(events) == 0 {
		return []any{nil}, nil
	}
	return events, nil
}

func pythonSpace(character rune) bool {
	return unicode.IsSpace(character) || (character >= 0x1c && character <= 0x1f)
}

func pythonLineBreak(character rune) bool {
	switch character {
	case '\n', '\r', '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
		return true
	default:
		return false
	}
}
