package usage

import (
	"bytes"
	"context"
	"encoding/json"
)

type jsonSyntaxError struct{}

func (jsonSyntaxError) Error() string { return "invalid JSON" }

type jsonDepthError struct{}

func (jsonDepthError) Error() string { return "JSON nesting exceeds private limit" }

type pythonJSON struct {
	data              []byte
	offset, nextCheck int
}

// A small decoder preserves Python's numeric extensions and Unicode identities.
// It does not feed permissive values back through strict JSON: docs/adr/0066-go-native-event-streams.md:43.
func decodePythonJSON(ctx context.Context, data []byte) (any, error) {
	parser := pythonJSON{data: data}
	value, err := parser.value(ctx, 0)
	if err != nil {
		return nil, err
	}
	if err := parser.space(ctx); err != nil {
		return nil, err
	}
	if parser.offset != len(data) {
		return nil, jsonSyntaxError{}
	}
	return value, nil
}

func (p *pythonJSON) check(ctx context.Context) error {
	if p.offset >= p.nextCheck {
		p.nextCheck = p.offset + 4096
		return ctx.Err()
	}
	return nil
}

func (p *pythonJSON) space(ctx context.Context) error {
	for p.offset < len(p.data) {
		if err := p.check(ctx); err != nil {
			return err
		}
		switch p.data[p.offset] {
		case ' ', '\t', '\r', '\n':
			p.offset++
		default:
			return nil
		}
	}
	return nil
}

func (p *pythonJSON) take(character byte) bool {
	if p.offset < len(p.data) && p.data[p.offset] == character {
		p.offset++
		return true
	}
	return false
}

func (p *pythonJSON) value(ctx context.Context, depth int) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := p.space(ctx); err != nil {
		return nil, err
	}
	if p.offset == len(p.data) {
		return nil, jsonSyntaxError{}
	}
	switch p.data[p.offset] {
	case '{', '[':
		if depth >= 512 {
			return nil, jsonDepthError{}
		}
		if p.take('{') {
			return p.object(ctx, depth+1)
		}
		p.offset++
		return p.array(ctx, depth+1)
	case '"':
		return p.string(ctx)
	case 't':
		return p.literal("true", true)
	case 'f':
		return p.literal("false", false)
	case 'n':
		return p.literal("null", nil)
	case 'N':
		return p.literal("NaN", json.Number("NaN"))
	case 'I':
		return p.literal("Infinity", json.Number("Infinity"))
	case '-':
		if bytes.HasPrefix(p.data[p.offset:], []byte("-Infinity")) {
			return p.literal("-Infinity", json.Number("-Infinity"))
		}
		return p.number(ctx)
	default:
		if p.data[p.offset] >= '0' && p.data[p.offset] <= '9' {
			return p.number(ctx)
		}
		return nil, jsonSyntaxError{}
	}
}

func (p *pythonJSON) literal(literal string, value any) (any, error) {
	if !bytes.HasPrefix(p.data[p.offset:], []byte(literal)) {
		return nil, jsonSyntaxError{}
	}
	p.offset += len(literal)
	return value, nil
}

func (p *pythonJSON) number(ctx context.Context) (any, error) {
	start := p.offset
scan:
	for p.offset < len(p.data) {
		if err := p.check(ctx); err != nil {
			return nil, err
		}
		switch p.data[p.offset] {
		case '-', '+', '.', 'e', 'E', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			p.offset++
		default:
			break scan
		}
	}
	raw := p.data[start:p.offset]
	if !json.Valid(raw) {
		return nil, jsonSyntaxError{}
	}
	number := json.Number(raw)
	if integerSyntax(number) && len(bytes.TrimPrefix(raw, []byte{'-'})) > 4300 {
		return nil, jsonSyntaxError{}
	}
	return number, nil
}

func (p *pythonJSON) object(ctx context.Context, depth int) (any, error) {
	result := make(map[string]any)
	if err := p.space(ctx); err != nil {
		return nil, err
	}
	if p.take('}') {
		return result, nil
	}
	for {
		if err := p.space(ctx); err != nil {
			return nil, err
		}
		if p.offset == len(p.data) || p.data[p.offset] != '"' {
			return nil, jsonSyntaxError{}
		}
		key, err := p.string(ctx)
		if err != nil {
			return nil, err
		}
		if err := p.space(ctx); err != nil {
			return nil, err
		}
		if !p.take(':') {
			return nil, jsonSyntaxError{}
		}
		value, err := p.value(ctx, depth)
		if err != nil {
			return nil, err
		}
		result[key] = value
		if err := p.space(ctx); err != nil {
			return nil, err
		}
		if p.take('}') {
			return result, nil
		}
		if !p.take(',') {
			return nil, jsonSyntaxError{}
		}
	}
}

func (p *pythonJSON) array(ctx context.Context, depth int) (any, error) {
	result := []any{}
	if err := p.space(ctx); err != nil {
		return nil, err
	}
	if p.take(']') {
		return result, nil
	}
	for {
		value, err := p.value(ctx, depth)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
		if err := p.space(ctx); err != nil {
			return nil, err
		}
		if p.take(']') {
			return result, nil
		}
		if !p.take(',') {
			return nil, jsonSyntaxError{}
		}
	}
}
