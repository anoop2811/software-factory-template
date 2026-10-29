package reviewbatch

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

// ApplyFilePatch proves complete text hunks against one exact old Git blob.
// It shares the review parser and preserves LF, CRLF and nonterminated lines.
// docs/adr/0109-complete-large-review-inputs.md:56.
func ApplyFilePatch(ctx context.Context, before, patch []byte, additions, deletions int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(before) > inputLimit || len(patch) > inputLimit || !utf8.Valid(before) || !utf8.Valid(patch) ||
		bytes.IndexByte(before, 0) >= 0 || bytes.IndexByte(patch, 0) >= 0 || !bytes.HasPrefix(patch, []byte("@@ ")) || additions < 0 || deletions < 0 {
		return nil, errors.New("invalid bounded text patch")
	}
	file := changedFile{Filename: "review-input", Status: "modified", Additions: &additions, Deletions: &deletions}
	lines := []string{"diff --git a/review-input b/review-input", "--- a/review-input", "+++ b/review-input"}
	lines = append(lines, strings.Split(strings.TrimSuffix(string(patch), "\n"), "\n")...)
	parsed, err := parseSection(lines, file, true)
	if err != nil {
		return nil, err
	}
	application := patchApplication{before: before}
	for _, h := range parsed.hunks {
		oldStart, newStart := h.oldStart-1, h.newStart-1
		if h.old == 0 {
			oldStart = h.oldStart
		}
		if h.new == 0 {
			newStart = h.newStart
		}
		if oldStart < application.oldLine {
			return nil, errors.New("patch old coordinates overlap")
		}
		for application.oldLine < oldStart {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			line, err := application.consume()
			if err != nil {
				return nil, err
			}
			if err := application.emit(line); err != nil {
				return nil, err
			}
		}
		if application.newLine != newStart {
			return nil, errors.New("patch new coordinates disagree with source")
		}
		for index := 0; index < len(h.lines); index++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			line := h.lines[index]
			content := []byte(line[1:])
			if index+1 < len(h.lines) && h.lines[index+1] == `\ No newline at end of file` {
				if len(content) == 0 {
					return nil, errors.New("empty nonterminated patch line")
				}
				index++
			} else {
				content = append(content, '\n')
			}
			if line[0] != '+' {
				actual, err := application.consume()
				if err != nil || !bytes.Equal(actual, content) {
					return nil, errors.New("patch context or removal does not match source")
				}
			}
			if line[0] != '-' {
				if err := application.emit(content); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tail := before[application.offset:]
	if len(tail) > 0 && (application.unterminated || len(application.output)+len(tail) > inputLimit) {
		return nil, errors.New("patch result is oversized or has a nonterminal newline marker")
	}
	return append(application.output, tail...), nil
}

type patchApplication struct {
	before, output           []byte
	offset, oldLine, newLine int
	unterminated             bool
}

func (a *patchApplication) consume() ([]byte, error) {
	if a.offset >= len(a.before) {
		return nil, errors.New("patch old range exceeds source")
	}
	end := len(a.before)
	if next := bytes.IndexByte(a.before[a.offset:], '\n'); next >= 0 {
		end = a.offset + next + 1
	}
	line := a.before[a.offset:end]
	a.offset, a.oldLine = end, a.oldLine+1
	return line, nil
}

func (a *patchApplication) emit(line []byte) error {
	if a.unterminated || len(a.output)+len(line) > inputLimit {
		return errors.New("patch result is oversized or has a nonterminal newline marker")
	}
	a.output = append(a.output, line...)
	a.newLine++
	a.unterminated = len(line) > 0 && line[len(line)-1] != '\n'
	return nil
}
