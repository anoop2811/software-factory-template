package usage

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strconv"
)

// Duplicate signatures use Python equality, including bool/numeric equivalence.
// This comparison precedes duplicate admission: docs/adr/0064-go-native-usage-accounting.md:60.
func equalValue(ctx context.Context, a, b any) bool {
	if ctx.Err() != nil {
		return false
	}
	if left, ok := equalityNumber(a); ok {
		right, ok := equalityNumber(b)
		return ok && equalNumber(left, right)
	}
	switch left := a.(type) {
	case nil:
		return b == nil
	case string:
		right, ok := b.(string)
		return ok && left == right
	case []any:
		right, ok := b.([]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for i, value := range left {
			if !equalValue(ctx, value, right[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		right, ok := b.(map[string]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for key, value := range left {
			other, exists := right[key]
			if !exists || !equalValue(ctx, value, other) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func equalityNumber(value any) (json.Number, bool) {
	switch value := value.(type) {
	case json.Number:
		return value, true
	case bool:
		if value {
			return "1", true
		}
		return "0", true
	default:
		return "", false
	}
}

func equalNumber(a, b json.Number) bool {
	aInteger, bInteger := integerSyntax(a), integerSyntax(b)
	if aInteger && bInteger {
		if a == "-0" {
			a = "0"
		}
		if b == "-0" {
			b = "0"
		}
		return a == b
	}
	if !aInteger && !bInteger {
		left, leftErr := a.Float64()
		right, rightErr := b.Float64()
		return (leftErr == nil || errors.Is(leftErr, strconv.ErrRange)) &&
			(rightErr == nil || errors.Is(rightErr, strconv.ErrRange)) && left == right
	}
	if !aInteger {
		a, b = b, a
	}
	whole := integer(a)
	fraction, err := b.Float64()
	if whole == nil || err != nil || !finite(fraction) {
		return false
	}
	return new(big.Rat).SetInt(whole).Cmp(new(big.Rat).SetFloat64(fraction)) == 0
}

func equalTokens(a, b map[string]*big.Int) bool {
	if (a == nil) != (b == nil) || len(a) != len(b) {
		return false
	}
	for key, left := range a {
		right, exists := b[key]
		if !exists || left.Cmp(right) != 0 {
			return false
		}
	}
	return true
}
