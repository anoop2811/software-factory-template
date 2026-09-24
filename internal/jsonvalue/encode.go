package jsonvalue

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// RawString applies Python filesystem/environment surrogateescape explicitly.
// Decoded JSON strings already have their own identity and must not pass here.
// docs/adr/0073-go-loop-fingerprint-foundation.md:55.
func RawString(raw string) string {
	var result strings.Builder
	for len(raw) > 0 {
		value, size := utf8.DecodeRuneInString(raw)
		if value == utf8.RuneError && size == 1 {
			result.WriteByte(0xed)
			result.WriteByte(0xb0 | (raw[0] >> 6))
			result.WriteByte(0x80 | (raw[0] & 0x3f))
		} else {
			result.WriteString(raw[:size])
		}
		raw = raw[size:]
	}
	return result.String()
}

// EncodePython streams the exact sorted, ASCII Python json.dumps representation.
// docs/adr/0073-go-loop-fingerprint-foundation.md:41.
func EncodePython(ctx context.Context, writer io.Writer, value any) error {
	buffered := bufio.NewWriter(writer)
	if err := encodeValue(ctx, buffered, value, 0); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return buffered.Flush()
}
func encodeValue(ctx context.Context, w *bufio.Writer, value any, depth int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if depth > 512 {
		return DepthError{}
	}
	var text string
	switch v := value.(type) {
	case nil:
		text = "null"
	case bool:
		if v {
			text = "true"
		} else {
			text = "false"
		}
	case string:
		return encodeString(ctx, w, v)
	case int:
		text = strconv.Itoa(v)
	case int64:
		text = strconv.FormatInt(v, 10)
	case uint32:
		text = strconv.FormatUint(uint64(v), 10)
	case *big.Int:
		if v == nil {
			text = "null"
		} else {
			text = v.String()
		}
	case float64:
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return errors.New("nonfinite canonical number")
		}
		text = FloatText(v)
	case json.Number:
		if !strings.ContainsAny(string(v), ".eE") {
			integer, ok := new(big.Int).SetString(string(v), 10)
			if !ok {
				return SyntaxError{}
			}
			text = integer.String()
		} else {
			number, err := v.Float64()
			if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
				return SyntaxError{}
			}
			text = FloatText(number)
		}
	case []any:
		if err := w.WriteByte('['); err != nil {
			return err
		}
		for i, child := range v {
			if i > 0 {
				if _, err := w.WriteString(", "); err != nil {
					return err
				}
			}
			if err := encodeValue(ctx, w, child, depth+1); err != nil {
				return err
			}
		}
		return w.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		if err := w.WriteByte('{'); err != nil {
			return err
		}
		for i, key := range keys {
			if i > 0 {
				if _, err := w.WriteString(", "); err != nil {
					return err
				}
			}
			if err := encodeString(ctx, w, key); err != nil {
				return err
			}
			if _, err := w.WriteString(": "); err != nil {
				return err
			}
			if err := encodeValue(ctx, w, v[key], depth+1); err != nil {
				return err
			}
		}
		return w.WriteByte('}')
	default:
		return errors.New("unsupported canonical value")
	}
	_, err := w.WriteString(text)
	return err
}
func encodeString(ctx context.Context, w *bufio.Writer, value string) error {
	if err := w.WriteByte('"'); err != nil {
		return err
	}
	count := 0
	for len(value) > 0 {
		if count%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		count++
		r, size := utf8.DecodeRuneInString(value)
		if r == utf8.RuneError && size == 1 {
			if len(value) < 3 || value[0] != 0xed || value[1] < 0xa0 || value[1] > 0xbf || value[2] < 0x80 || value[2] > 0xbf {
				return errors.New("invalid canonical string")
			}
			r = rune(value[0]&15)<<12 | rune(value[1]&63)<<6 | rune(value[2]&63)
			size = 3
		}
		value = value[size:]
		var escaped string
		switch r {
		case '"':
			escaped = `\"`
		case '\\':
			escaped = `\\`
		case '\b':
			escaped = `\b`
		case '\f':
			escaped = `\f`
		case '\n':
			escaped = `\n`
		case '\r':
			escaped = `\r`
		case '\t':
			escaped = `\t`
		default:
			if r < 32 || r >= 127 {
				if r > 0xffff {
					r -= 0x10000
					escaped = fmt.Sprintf(`\u%04x\u%04x`, 0xd800+(r>>10), 0xdc00+(r&0x3ff))
				} else {
					escaped = fmt.Sprintf(`\u%04x`, r)
				}
			} else {
				escaped = string(r)
			}
		}
		if _, err := w.WriteString(escaped); err != nil {
			return err
		}
	}
	return w.WriteByte('"')
}
