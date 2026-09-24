package budget

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
)

const historyLimit = 32 << 20

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// ParseHistory preserves unknown scalar data and integer identity while enforcing
// the interoperable schema: docs/adr/0069-go-budget-ledger-admission.md:22.
func ParseHistory(ctx context.Context, input io.Reader) (History, error) {
	if input == nil {
		return History{}, errors.New("invalid budget history input")
	}
	data, err := io.ReadAll(io.LimitReader(&contextReader{ctx, input}, historyLimit+1))
	if err != nil || len(data) > historyLimit || !utf8.Valid(data) {
		return History{}, errors.New("invalid or oversized budget history")
	}
	value, err := jsonvalue.Decode(ctx, data)
	if err != nil {
		return History{}, errors.New("malformed budget history")
	}
	if err := strictValue(ctx, value); err != nil {
		return History{}, err
	}
	fields, ok := value.(map[string]any)
	if !ok {
		return History{}, errors.New("malformed budget history")
	}
	h := History{data: fields}
	if err := validateHistory(ctx, h); err != nil {
		return History{}, err
	}
	return h, nil
}
func strictValue(ctx context.Context, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch v := value.(type) {
	case string:
		if !utf8.ValidString(v) {
			return errors.New("non-scalar budget history")
		}
	case json.Number:
		if !integerSyntax(v) {
			if _, ok := numericFloat(v); !ok {
				return errors.New("nonfinite budget history")
			}
		}
	case []any:
		for _, child := range v {
			if err := strictValue(ctx, child); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, child := range v {
			if !utf8.ValidString(key) {
				return errors.New("non-scalar budget history")
			}
			if err := strictValue(ctx, child); err != nil {
				return err
			}
		}
	}
	return nil
}
func validateHistory(ctx context.Context, h History) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	schema, ok := exactInteger(h.data["schema"])
	if !ok || !schema.IsInt64() || schema.Int64() != 1 {
		return errors.New("unsupported or malformed budget history schema")
	}
	runs, ok := h.data["runs"].([]any)
	if !ok {
		return errors.New("malformed budget history runs")
	}
	seen := map[string]bool{}
	for _, value := range runs {
		if err := ctx.Err(); err != nil {
			return err
		}
		row, ok := value.(map[string]any)
		if !ok {
			return errors.New("malformed budget history run")
		}
		if err := validateRecord(row); err != nil {
			return err
		}
		id := row["id"].(string)
		if seen[id] {
			return errors.New("duplicate budget run identity")
		}
		seen[id] = true
	}
	return nil
}
func validateRecord(r map[string]any) error {
	invalid := errors.New("malformed budget history record")
	for _, key := range []string{"id", "session", "task"} {
		s, ok := r[key].(string)
		if !ok || !validID(s) {
			return invalid
		}
	}
	h, _ := r["harness"].(string)
	roleName, _ := r["role"].(string)
	if !harness(h) || !role(roleName) {
		return invalid
	}
	status, _ := r["status"].(string)
	if status != "active" && status != "completed" {
		return invalid
	}
	outcome, _ := r["outcome"].(string)
	if !validOutcome(outcome, true) {
		return invalid
	}
	for _, key := range []string{"elapsed_seconds", "reserved_seconds"} {
		f, ok := numericFloat(r[key])
		if !ok || f < 0 {
			return invalid
		}
	}
	for _, key := range []string{"attempt", "owner_pid"} {
		if _, ok := checkedInteger(r[key], false); !ok {
			return invalid
		}
	}
	complete, ok := r["complete"].(bool)
	if !ok {
		return invalid
	}
	if r["estimated_usd"] != nil {
		f, ok := numericFloat(r["estimated_usd"])
		if !ok || f < 0 {
			return invalid
		}
	}
	warnings, ok := r["warnings"].([]any)
	if !ok {
		return invalid
	}
	for _, w := range warnings {
		if _, ok := w.(string); !ok {
			return invalid
		}
	}
	if r["tokens"] != nil {
		tokens, ok := r["tokens"].(map[string]any)
		if !ok {
			return invalid
		}
		for _, value := range tokens {
			if _, ok := checkedInteger(value, true); !ok {
				return invalid
			}
		}
	}
	if r["process_pid"] != nil {
		if _, ok := checkedInteger(r["process_pid"], false); !ok {
			return invalid
		}
	}
	if r["exit_code"] != nil {
		if _, ok := exactInteger(r["exit_code"]); !ok {
			return invalid
		}
	}
	for _, key := range []string{"model", "started_at", "source"} {
		if _, ok := r[key].(string); !ok {
			return invalid
		}
	}
	if status == "active" {
		if outcome != "running" && outcome != "timeout" && outcome != "launch_error" || r["ended_at"] != nil {
			return invalid
		}
		_, hasCost := r["estimated_usd"]
		_, hasTokens := r["tokens"]
		if outcome != "running" && (!hasCost || !hasTokens) {
			return invalid
		}
		if outcome != "running" && (r["process_pid"] == nil || r["exit_code"] != nil || complete || r["estimated_usd"] != nil || r["tokens"] != nil) {
			return invalid
		}
	} else {
		if _, ok := r["ended_at"].(string); !ok || outcome == "running" {
			return invalid
		}
	}
	return nil
}
func validOutcome(s string, running bool) bool {
	switch s {
	case "running":
		return running
	case "completed", "failed", "timeout", "interrupted", "output_limit", "launch_error":
		return true
	default:
		return false
	}
}
func (h History) rows() []map[string]any {
	values, _ := h.data["runs"].([]any)
	rows := make([]map[string]any, 0, len(values))
	for _, value := range values {
		row, _ := value.(map[string]any)
		rows = append(rows, row)
	}
	return rows
}
func cloneValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for k, child := range v {
			result[k] = cloneValue(child)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, child := range v {
			result[i] = cloneValue(child)
		}
		return result
	default:
		return value
	}
}
func cloneRecord(row map[string]any) Record { return Record{data: cloneValue(row).(map[string]any)} }
func emptyHistory() History {
	return History{data: map[string]any{"schema": json.Number("1"), "runs": []any{}}}
}

// HasActive checks validated ledger state without exposing mutable records.
// docs/adr/0074-go-loop-checkpoint-storage.md:96.
func (h History) HasActive(ctx context.Context) (bool, error) {
	if err := validateHistory(ctx, h); err != nil {
		return false, err
	}
	for _, row := range h.rows() {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if row["status"] == "active" {
			return true, nil
		}
	}
	return false, nil
}
