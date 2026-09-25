package jsonvalue

import (
	"errors"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// PositiveInteger retains exact decimal counters while requiring finite conversion.
// docs/adr/0073-go-loop-fingerprint-foundation.md:32.
func PositiveInteger(raw string) (*big.Int, error) {
	if raw == "" || strings.Trim(raw, "0123456789") != "" || len(strings.TrimLeft(raw, "0")) > 309 {
		return nil, errors.New("invalid positive integer")
	}
	value, ok := new(big.Int).SetString(raw, 10)
	if !ok || value.Sign() <= 0 {
		return nil, errors.New("invalid positive integer")
	}
	f, _ := value.Float64()
	if math.IsInf(f, 0) {
		return nil, errors.New("invalid positive integer")
	}
	return value, nil
}

// PositiveFloat shares the established Python-compatible input grammar.
// docs/adr/0073-go-loop-fingerprint-foundation.md:32.
func PositiveFloat(raw string) (float64, error) {
	raw = strings.TrimSpace(raw)
	var text strings.Builder
	for _, r := range raw {
		if r < 128 {
			text.WriteRune(r)
			continue
		}
		digit := -1
		for _, span := range unicode.Digit.R16 {
			if r >= rune(span.Lo) && r <= rune(span.Hi) && (r-rune(span.Lo))%rune(span.Stride) == 0 {
				digit = int((r-rune(span.Lo))/rune(span.Stride)) % 10
				break
			}
		}
		if digit < 0 {
			for _, span := range unicode.Digit.R32 {
				if uint32(r) >= span.Lo && uint32(r) <= span.Hi && (uint32(r)-span.Lo)%span.Stride == 0 {
					digit = int((uint32(r)-span.Lo)/span.Stride) % 10
					break
				}
			}
		}
		if digit < 0 {
			return 0, errors.New("invalid positive number")
		}
		text.WriteByte(byte('0' + digit))
	}
	number := text.String()
	matched, _ := regexp.MatchString(`^[+-]?(?:[0-9](?:_?[0-9])*(?:\.(?:[0-9](?:_?[0-9])*)?)?|\.[0-9](?:_?[0-9])*)(?:[eE][+-]?[0-9](?:_?[0-9])*)?$`, number)
	if !matched {
		return 0, errors.New("invalid positive number")
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(number, "_", ""), 64)
	if err != nil || (math.IsInf(f, 0) || math.IsNaN(f)) || f <= 0 {
		return 0, errors.New("invalid positive number")
	}
	return f, nil
}

// FloatText preserves Python finite float spelling for text and identity.
func FloatText(value float64) string {
	scientific := strconv.FormatFloat(value, 'e', -1, 64)
	_, exponent, _ := strings.Cut(scientific, "e")
	power, _ := strconv.Atoi(exponent)
	if power >= -4 && power < 16 {
		text := strconv.FormatFloat(value, 'f', -1, 64)
		if !strings.ContainsRune(text, '.') {
			text += ".0"
		}
		return text
	}
	return scientific
}
