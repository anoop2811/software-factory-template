package review

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// streamFailure contains only client-defined classifications, never transport text.
type streamFailure string

func (failure streamFailure) Error() string { return string(failure) }

type boundedResponse struct {
	ctx     context.Context
	reader  io.Reader
	metrics *metrics
}

func (r *boundedResponse) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	r.metrics.mu.Lock()
	remaining := (8 << 20) - r.metrics.bytes
	r.metrics.mu.Unlock()
	if len(p) > remaining+1 {
		p = p[:remaining+1]
	}
	n, err := r.reader.Read(p)
	r.metrics.mu.Lock()
	r.metrics.bytes += n
	over := r.metrics.bytes > 8<<20
	r.metrics.mu.Unlock()
	if over {
		return n, streamFailure("review response limit exceeded")
	}
	return n, err
}

type streamState struct {
	text           strings.Builder
	terminal, done bool
	metrics        *metrics
}

// Both terminal markers and clean EOF are required before any output is released.
// docs/adr/0067-streaming-adversarial-review-client.md:57.
func readStream(ctx context.Context, body io.Reader, m *metrics) (string, error) {
	reader := bufio.NewReader(&boundedResponse{ctx: ctx, reader: body, metrics: m})
	state := streamState{metrics: m}
	var data strings.Builder
	hasData := false
	eventBytes := 0
	eventType := ""
	firstLine := true
	for {
		line, wire, err := readLine(ctx, reader)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				var classified streamFailure
				if errors.As(err, &classified) {
					return "", classified
				}
				return "", streamFailure("review transport failure")
			}
			if wire != 0 || hasData || eventType != "" {
				return "", streamFailure("incomplete review stream")
			}
			break
		}
		eventBytes += wire
		if eventBytes > 1<<20 {
			return "", streamFailure("review event limit exceeded")
		}
		if firstLine {
			line = bytes.TrimPrefix(line, []byte{0xef, 0xbb, 0xbf})
			firstLine = false
		}
		if len(line) == 0 {
			if eventType == "error" {
				return "", streamFailure("review provider error")
			}
			if hasData {
				if err := state.event([]byte(data.String())); err != nil {
					return "", err
				}
			}
			data.Reset()
			hasData = false
			eventBytes = 0
			eventType = ""
			continue
		}
		if line[0] == ':' {
			m.mu.Lock()
			m.heartbeats++
			m.mu.Unlock()
			continue
		}
		field, value, found := bytes.Cut(line, []byte{':'})
		if !found {
			value = nil
		}
		value = bytes.TrimPrefix(value, []byte{' '})
		switch string(field) {
		case "data":
			if hasData {
				data.WriteByte('\n')
			}
			data.Write(value)
			hasData = true
		case "event":
			eventType = string(value)
		}
	}
	if !state.terminal || !state.done || strings.TrimSpace(state.text.String()) == "" {
		return "", streamFailure("incomplete review stream")
	}
	return state.text.String(), nil
}

func readLine(ctx context.Context, reader *bufio.Reader) ([]byte, int, error) {
	var line []byte
	wire := 0
	for {
		if ctx.Err() != nil {
			return nil, wire, ctx.Err()
		}
		character, err := reader.ReadByte()
		if err != nil {
			return line, wire, err
		}
		wire++
		if wire > 1<<20 {
			return nil, wire, streamFailure("review line limit exceeded")
		}
		if character == '\n' {
			return line, wire, nil
		}
		if character == '\r' {
			next, err := reader.Peek(1)
			if err != nil && !errors.Is(err, io.EOF) {
				return nil, wire, err
			}
			if len(next) > 0 && next[0] == '\n' {
				_, _ = reader.ReadByte()
				wire++
				if wire > 1<<20 {
					return nil, wire, streamFailure("review line limit exceeded")
				}
			}
			return line, wire, nil
		}
		line = append(line, character)
	}
}

func (s *streamState) event(data []byte) error {
	if s.done {
		return streamFailure("review data after completion")
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		if !s.terminal {
			return streamFailure("incomplete review stream")
		}
		s.done = true
		return nil
	}
	if !utf8.Valid(data) {
		return streamFailure("invalid review stream")
	}
	var event struct {
		Error   json.RawMessage `json:"error"`
		Choices []struct {
			Index  *int                       `json:"index"`
			Delta  map[string]json.RawMessage `json:"delta"`
			Finish *string                    `json:"finish_reason"`
			Error  json.RawMessage            `json:"error"`
		} `json:"choices"`
		Usage json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(data, &event) != nil {
		return streamFailure("invalid review stream")
	}
	if nonnull(event.Error) {
		return streamFailure("review provider error")
	}
	if nonnull(event.Usage) {
		var usage map[string]json.RawMessage
		if json.Unmarshal(event.Usage, &usage) != nil || usage == nil {
			return streamFailure("invalid review stream")
		}
	}
	if len(event.Choices) == 0 {
		if !nonnull(event.Usage) {
			return streamFailure("invalid review stream")
		}
		return nil
	}
	if len(event.Choices) != 1 {
		return streamFailure("invalid review stream")
	}
	choice := event.Choices[0]
	if nonnull(choice.Error) {
		return streamFailure("review provider error")
	}
	if choice.Index != nil && *choice.Index != 0 {
		return streamFailure("invalid review stream")
	}
	if nonnull(choice.Delta["tool_calls"]) || nonnull(choice.Delta["function_call"]) {
		return streamFailure("review completion rejected")
	}
	var content string
	if nonnull(choice.Delta["content"]) && json.Unmarshal(choice.Delta["content"], &content) != nil {
		return streamFailure("invalid review stream")
	}
	reasoning := nonnull(choice.Delta["reasoning"]) || nonnull(choice.Delta["reasoning_details"])
	if s.terminal && (content != "" || reasoning) {
		return streamFailure("review data after completion")
	}
	if content != "" {
		s.text.WriteString(content)
		s.metrics.mu.Lock()
		s.metrics.content++
		s.metrics.mu.Unlock()
	}
	if reasoning {
		s.metrics.mu.Lock()
		s.metrics.reasoning++
		s.metrics.mu.Unlock()
	}
	if choice.Finish != nil {
		if *choice.Finish != "stop" {
			return streamFailure("review completion rejected")
		}
		s.terminal = true
	}
	return nil
}
func nonnull(value json.RawMessage) bool {
	return len(value) > 0 && !bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}
