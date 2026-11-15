package synccmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/fileinput"
	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
)

// Equivalent generated JSON retains its original bytes through the same checked
// publication path. Malformed destinations remain eligible for ordinary repair.
// docs/adr/0156-integrated-adopter-rehearsal.md:91.
func (target *destination) preserveJSON(ctx context.Context) error {
	switch target.path {
	case "opencode.json", ".claude/settings.json", ".mcp.json":
	default:
		return nil
	}
	if err := target.previous.Rewind(ctx); err != nil {
		return unsafe(err)
	}
	original, readErr := fileinput.ReadFile(ctx, target.previous.File, inputLimit)
	if err := errors.Join(readErr, target.previous.Check(ctx)); err != nil {
		return unsafe(err)
	}
	equivalent, err := equivalentJSON(ctx, target.path, original, target.data)
	if err != nil {
		return unsafe(err)
	}
	if equivalent {
		target.data = original
	}
	return ctx.Err()
}

func equivalentJSON(ctx context.Context, path string, before, after []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !utf8.Valid(before) || !utf8.Valid(after) || !json.Valid(before) || !json.Valid(after) {
		return false, nil
	}
	left, err := jsonvalue.DecodeUnique(ctx, before)
	if err != nil {
		return false, ctx.Err()
	}
	right, err := jsonvalue.DecodeUnique(ctx, after)
	if err != nil {
		return false, ctx.Err()
	}
	// DecodeUnique retains json.Number lexemes and array order, without float64
	// coercion or silently choosing one of duplicate object members.
	if !reflect.DeepEqual(left, right) {
		return false, ctx.Err()
	}
	if path == "opencode.json" {
		return equivalentOpenCode(ctx, before, after, 0)
	}
	return true, ctx.Err()
}

// Only root, root.agent and its direct role containers have incidental order.
// Every other subtree retains its complete ordered token stream, including
// nested fields named agent and both levels of permission rules.
// docs/adr/0156-integrated-adopter-rehearsal.md:94.
func equivalentOpenCode(ctx context.Context, before, after []byte, depth int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	left, leftErr := jsonObject(before)
	right, rightErr := jsonObject(after)
	if leftErr != nil || rightErr != nil {
		// The accepted null agent value is not an unordered object container.
		return orderedJSON(ctx, before, after)
	}
	if len(left) != len(right) {
		return false, nil
	}
	values := make(map[string]json.RawMessage, len(right))
	for _, entry := range right {
		values[entry.key] = entry.value
	}
	for _, entry := range left {
		value, found := values[entry.key]
		if !found {
			return false, nil
		}
		var equivalent bool
		var err error
		if depth == 0 && entry.key == "agent" || depth == 1 {
			equivalent, err = equivalentOpenCode(ctx, entry.value, value, depth+1)
		} else {
			equivalent, err = orderedJSON(ctx, entry.value, value)
		}
		if err != nil || !equivalent {
			return false, err
		}
	}
	return true, ctx.Err()
}

func orderedJSON(ctx context.Context, before, after []byte) (bool, error) {
	left, right := json.NewDecoder(bytes.NewReader(before)), json.NewDecoder(bytes.NewReader(after))
	left.UseNumber()
	right.UseNumber()
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		a, leftErr := left.Token()
		b, rightErr := right.Token()
		if leftErr != nil || rightErr != nil {
			return errors.Is(leftErr, io.EOF) && errors.Is(rightErr, io.EOF), ctx.Err()
		}
		if a != b {
			return false, nil
		}
	}
}
