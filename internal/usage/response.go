package usage

import (
	"context"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// Response selects assistant text without authorizing its publication.
// docs/adr/0068-go-native-harness-execution.md:142.
func Response(ctx context.Context, harness string, input io.Reader) (string, error) {
	if !Supported(harness) {
		return "", errors.New("unsupported usage harness")
	}
	events, err := readStreamEvents(ctx, input)
	if err != nil {
		return "", err
	}
	answer := ""
	var parts []string
	seen := map[[3]string]bool{}
	for _, event := range events {
		if ctx.Err() != nil {
			return "", errors.New("invalid usage stream input")
		}
		row := object(event)
		if row == nil {
			continue
		}
		switch harness {
		case "codex":
			item := object(row["item"])
			if row["type"] == "item.completed" && item["type"] == "agent_message" {
				if value, ok := item["text"].(string); ok {
					answer = value
				}
			}
		case "claude":
			if row["type"] == "result" && row["subtype"] == "success" && row["is_error"] != true {
				if value, ok := row["result"].(string); ok {
					answer = value
				}
			}
		case "opencode":
			if row["type"] != "text" {
				continue
			}
			part := object(row["part"])
			value, ok := part["text"].(string)
			if !ok {
				continue
			}
			var identity [3]string
			valid := true
			for i, key := range []string{"sessionID", "messageID", "id"} {
				identity[i], ok = part[key].(string)
				valid = valid && ok
			}
			if valid && !seen[identity] {
				seen[identity] = true
				parts = append(parts, value)
			}
		}
	}
	if harness == "opencode" {
		answer = strings.Join(parts, "\n")
	}
	if !utf8.ValidString(answer) {
		return "", errors.New("invalid native response text")
	}
	return answer, nil
}
