package review

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

func timeoutSeconds(value string) (int, error) {
	if value == "" {
		return 1200, nil
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0, errors.New("invalid review timeout")
		}
	}
	value = strings.TrimLeft(value, "0")
	if len(value) == 0 || len(value) > 4 {
		return 0, errors.New("invalid review timeout")
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || n > 1200 {
		return 0, errors.New("invalid review timeout")
	}
	return n, nil
}

// Admit only the single text-review request built by the shared shell adapter.
// docs/adr/0067-streaming-adversarial-review-client.md:21.
func prepare(ctx context.Context, input io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(&checkedReader{ctx: ctx, reader: input}, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 || !utf8.Valid(raw) {
		return nil, errors.New("invalid review request")
	}
	var request map[string]json.RawMessage
	if json.Unmarshal(raw, &request) != nil || request == nil {
		return nil, errors.New("invalid review request")
	}
	for key := range request {
		switch key {
		case "model", "messages", "max_tokens", "reasoning", "provider":
		default:
			return nil, errors.New("invalid review request")
		}
	}
	var model string
	var messages []map[string]json.RawMessage
	var maxTokens int
	if json.Unmarshal(request["model"], &model) != nil || strings.TrimSpace(model) == "" ||
		json.Unmarshal(request["messages"], &messages) != nil || len(messages) == 0 ||
		json.Unmarshal(request["max_tokens"], &maxTokens) != nil || maxTokens < 1024 || maxTokens > 32768 {
		return nil, errors.New("invalid review request")
	}
	for _, message := range messages {
		var role, content string
		if len(message) != 2 || json.Unmarshal(message["role"], &role) != nil || json.Unmarshal(message["content"], &content) != nil || strings.TrimSpace(content) == "" {
			return nil, errors.New("invalid review request")
		}
		switch role {
		case "system", "user", "assistant", "developer":
		default:
			return nil, errors.New("invalid review request")
		}
	}
	if raw, exists := request["reasoning"]; exists {
		var reasoning map[string]string
		if json.Unmarshal(raw, &reasoning) != nil || len(reasoning) != 1 {
			return nil, errors.New("invalid review request")
		}
		switch reasoning["effort"] {
		case "none", "minimal", "low", "medium", "high", "xhigh", "max":
		default:
			return nil, errors.New("invalid review request")
		}
	}
	if raw, exists := request["provider"]; exists {
		var fields map[string]json.RawMessage
		var order []string
		if json.Unmarshal(raw, &fields) != nil || len(fields) != 3 || json.Unmarshal(fields["order"], &order) != nil || len(order) != 1 ||
			!regexp.MustCompile(`^[a-z0-9]+([-/][a-z0-9]+)*$`).MatchString(order[0]) ||
			!bytes.Equal(bytes.TrimSpace(fields["allow_fallbacks"]), []byte("false")) || !bytes.Equal(bytes.TrimSpace(fields["require_parameters"]), []byte("true")) {
			return nil, errors.New("invalid review request")
		}
	}
	request["stream"] = json.RawMessage("true")
	body, err := json.Marshal(request)
	if err != nil || len(body) > 1<<20 || ctx.Err() != nil {
		return nil, errors.New("invalid review request")
	}
	return body, nil
}

type checkedReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *checkedReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
