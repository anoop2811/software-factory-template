package config

import (
	"bytes"
	"context"
	"errors"
	"strings"
)

const selectedKeyLimit = 4097 // 4096 legacy settings and the factory-owned marker.

// EditPlan indexes selected physical-line prefixes without retaining every row.
// It supports bounded, single-line edits; Set's unrestricted grammar is unchanged.
// docs/adr/0090-go-native-config-migration.md:123.
type EditPlan struct {
	body     []byte
	entries  map[string]*editEntry
	tail     *editRow
	appended []*editRow
	byKey    map[string]*editRow
	size     int64
}

type editEntry struct {
	count       int64
	contentSize int64
	first       string
	replacement string
	replaced    bool
}

type editRow struct {
	line       string
	key        string
	value      string
	terminated bool
}

// NewEditPlan scans YAML once, indexing only caller-selected flat keys. The
// caller must keep data unchanged until Bytes returns; no input bytes are edited.
func NewEditPlan(ctx context.Context, data []byte, selectedKeys []string) (*EditPlan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(data) > configWriteLimit {
		return nil, errors.New("configuration exceeds 16 MiB")
	}
	p := &EditPlan{
		body: data, size: int64(len(data)),
		entries: make(map[string]*editEntry, min(len(selectedKeys), selectedKeyLimit)),
		byKey:   make(map[string]*editRow),
	}
	for _, key := range selectedKeys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if key == "" || strings.ContainsAny(key, ":\x00\r\n") {
			return nil, errors.New("edit plan requires nonempty flat keys without colon or line breaks")
		}
		if _, exists := p.entries[key]; exists {
			continue
		}
		if len(p.entries) == selectedKeyLimit {
			return nil, errors.New("edit plan exceeds selected-key limit")
		}
		p.entries[key] = &editEntry{}
	}
	remaining, offset := data, 0
	for len(remaining) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, rest, newline := bytes.Cut(remaining, []byte{'\n'})
		if !newline {
			p.body = data[:offset]
			p.tail = p.row(string(line), false)
			break
		}
		keyBytes, valueBytes, colon := bytes.Cut(line, []byte{':'})
		if colon {
			if entry := p.entries[string(keyBytes)]; entry != nil {
				if entry.count == 0 {
					entry.first = normalizeValue(string(valueBytes), "")
				}
				entry.count++
				entry.contentSize += int64(len(line))
			}
		}
		offset += len(line) + 1
		remaining = rest
	}
	return p, ctx.Err()
}

func (p *EditPlan) row(line string, terminated bool) *editRow {
	row := &editRow{line: line, terminated: terminated}
	key, value, colon := strings.Cut(line, ":")
	if colon && p.entries[key] != nil {
		row.key = key
		row.value = normalizeValue(value, "")
	}
	return row
}

// Get reads the first matching physical row after the planned edits.
func (p *EditPlan) Get(ctx context.Context, key, fallback string) (string, error) {
	if err := ctx.Err(); err != nil {
		return fallback, err
	}
	entry := p.entries[key]
	if entry == nil {
		return fallback, errors.New("edit plan key was not selected")
	}
	value := ""
	switch {
	case entry.count > 0:
		value = entry.first
	case p.tail != nil && p.tail.key == key:
		value = p.tail.value
	case p.byKey[key] != nil:
		value = p.byKey[key].value
	}
	if value == "" {
		return fallback, nil
	}
	return value, nil
}

// Set accumulates one physical-line rewrite, enforcing the intermediate size
// even when a later edit could shrink it. Values must round-trip on one row.
func (p *EditPlan) Set(ctx context.Context, key, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entry := p.entries[key]
	if entry == nil {
		return errors.New("edit plan key was not selected")
	}
	if strings.ContainsAny(value, "\x00\r\n\"") {
		return errors.New("edit plan value must round-trip on a single physical line")
	}
	line := configurationLine(key, value)
	count, oldSize := entry.count, entry.contentSize
	tailMatches := p.tail != nil && p.tail.key == key
	if tailMatches {
		count++
		oldSize += int64(len(p.tail.line))
	}
	appended := p.byKey[key]
	if appended != nil {
		count++
		oldSize += int64(len(appended.line))
	}
	size := p.size - oldSize + count*int64(len(line))
	if count == 0 {
		size = p.size + int64(len(line)) + 1
	}
	if size > configWriteLimit {
		return errors.New("resulting configuration exceeds 16 MiB")
	}
	if count == 0 {
		if p.tail != nil && !p.tail.terminated {
			// Appending deliberately does not repair an unterminated input row.
			// Reindex the merged row: its prefix may name another selected key.
			p.tail = p.row(p.tail.line+line, true)
		} else {
			row := p.row(line, true)
			p.appended = append(p.appended, row)
			p.byKey[key] = row
		}
	} else {
		if entry.count > 0 {
			entry.contentSize = entry.count * int64(len(line))
			entry.replacement, entry.replaced = line, true
			entry.first = normalizeValue(line[len(key)+1:], "")
		}
		if tailMatches {
			p.tail = p.row(line, p.tail.terminated)
		}
		if appended != nil {
			updated := p.row(line, true)
			*appended = *updated
		}
	}
	p.size = size
	return nil
}

// Bytes renders once, preserving original row order, duplicate replacements,
// append order and the unterminated-tail concatenation contract.
func (p *EditPlan) Bytes(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	output.Grow(int(p.size))
	remaining := p.body
	for len(remaining) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, rest, _ := bytes.Cut(remaining, []byte{'\n'})
		key, _, colon := bytes.Cut(line, []byte{':'})
		if colon {
			if entry := p.entries[string(key)]; entry != nil && entry.replaced {
				output.WriteString(entry.replacement)
			} else {
				output.Write(line)
			}
		} else {
			output.Write(line)
		}
		output.WriteByte('\n')
		remaining = rest
	}
	if p.tail != nil {
		output.WriteString(p.tail.line)
		if p.tail.terminated {
			output.WriteByte('\n')
		}
	}
	for _, row := range p.appended {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		output.WriteString(row.line)
		output.WriteByte('\n')
	}
	if int64(output.Len()) != p.size {
		return nil, errors.New("edit plan size did not match rendered configuration")
	}
	return output.Bytes(), ctx.Err()
}
