package usage

import (
	"context"
	"strings"
)

// Surrogate pairs canonicalize; lone code units use lossless internal WTF-8.
// These strings never enter output: docs/adr/0066-go-native-event-streams.md:46.
func (p *pythonJSON) string(ctx context.Context) (string, error) {
	p.offset++ // The caller has admitted the opening quote.
	var result strings.Builder
	for p.offset < len(p.data) {
		if err := p.check(ctx); err != nil {
			return "", err
		}
		character := p.data[p.offset]
		p.offset++
		switch {
		case character == '"':
			return result.String(), nil
		case character < 0x20:
			return "", jsonSyntaxError{}
		case character != '\\':
			result.WriteByte(character)
		default:
			if p.offset == len(p.data) {
				return "", jsonSyntaxError{}
			}
			escaped := p.data[p.offset]
			p.offset++
			switch escaped {
			case '"', '\\', '/':
				result.WriteByte(escaped)
			case 'b':
				result.WriteByte('\b')
			case 'f':
				result.WriteByte('\f')
			case 'n':
				result.WriteByte('\n')
			case 'r':
				result.WriteByte('\r')
			case 't':
				result.WriteByte('\t')
			case 'u':
				code, err := p.codeUnit()
				if err != nil {
					return "", err
				}
				if code >= 0xd800 && code <= 0xdbff && p.offset+6 <= len(p.data) && p.data[p.offset] == '\\' && p.data[p.offset+1] == 'u' {
					saved := p.offset
					p.offset += 2
					low, err := p.codeUnit()
					if err != nil {
						return "", err
					}
					if low >= 0xdc00 && low <= 0xdfff {
						code = 0x10000 + (code-0xd800)*0x400 + low - 0xdc00
					} else {
						p.offset = saved
					}
				}
				if code >= 0xd800 && code <= 0xdfff {
					result.WriteByte(0xed)
					result.WriteByte(0x80 | byte((code>>6)&0x3f))
					result.WriteByte(0x80 | byte(code&0x3f))
				} else {
					result.WriteRune(code)
				}
			default:
				return "", jsonSyntaxError{}
			}
		}
	}
	return "", jsonSyntaxError{}
}

func (p *pythonJSON) codeUnit() (rune, error) {
	if len(p.data)-p.offset < 4 {
		return 0, jsonSyntaxError{}
	}
	var code rune
	for range 4 {
		character := p.data[p.offset]
		p.offset++
		code *= 16
		switch {
		case character >= '0' && character <= '9':
			code += rune(character - '0')
		case character >= 'a' && character <= 'f':
			code += rune(character-'a') + 10
		case character >= 'A' && character <= 'F':
			code += rune(character-'A') + 10
		default:
			return 0, jsonSyntaxError{}
		}
	}
	return code, nil
}
