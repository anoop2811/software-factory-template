package loop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/big"
	"strings"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/budget"
	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
	"github.com/anoop2811/software-factory-template/internal/statefile"
)

// History retains validated schema-one checkpoints, including unknown fields.
type History struct{ data map[string]any }

// Record is a detached checkpoint with no exported mutable representation.
type Record struct{ data map[string]any }

func checkpointError() error { return errors.New("invalid loop checkpoint") }
func emptyHistory() History {
	return History{map[string]any{"schema": json.Number("1"), "runs": []any{}}}
}

type boundedContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r boundedContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// ParseHistory preserves unknown values and escaped surrogate strings without repair.
// docs/adr/0074-go-loop-checkpoint-storage.md:21.
func ParseHistory(ctx context.Context, input io.Reader) (History, error) {
	value, err := readValue(ctx, input)
	if err != nil {
		return History{}, err
	}
	data, ok := value.(map[string]any)
	if !ok {
		return History{}, checkpointError()
	}
	h := History{data}
	if err := validateHistory(ctx, h); err != nil {
		return History{}, err
	}
	return h, nil
}
func readValue(ctx context.Context, input io.Reader) (any, error) {
	if input == nil {
		return nil, checkpointError()
	}
	data, err := io.ReadAll(io.LimitReader(boundedContextReader{ctx, input}, statefile.Limit+1))
	if err != nil || len(data) > statefile.Limit || !utf8.Valid(data) {
		return nil, checkpointError()
	}
	value, err := jsonvalue.Decode(ctx, data)
	if err != nil {
		return nil, checkpointError()
	}
	if err := finiteValue(ctx, value); err != nil {
		return nil, err
	}
	return value, nil
}
func finiteValue(ctx context.Context, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch v := value.(type) {
	case json.Number:
		if strings.ContainsAny(string(v), ".eE") || string(v) == "NaN" || strings.Contains(string(v), "Infinity") {
			number, err := v.Float64()
			if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
				return checkpointError()
			}
		}
	case []any:
		for _, child := range v {
			if err := finiteValue(ctx, child); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, child := range v {
			if err := finiteValue(ctx, child); err != nil {
				return err
			}
		}
	}
	return nil
}
func integer(value any) (*big.Int, bool) {
	n, ok := value.(json.Number)
	if !ok || strings.ContainsAny(string(n), ".eE") {
		return nil, false
	}
	return new(big.Int).SetString(string(n), 10)
}
func nonnegative(value any, requireInteger bool) bool {
	n, ok := value.(json.Number)
	if !ok {
		return false
	}
	if requireInteger {
		if _, ok := integer(n); !ok {
			return false
		}
	}
	f, err := n.Float64()
	return err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) && f >= 0
}
func fingerprint(value any) (Fingerprint, bool) {
	object, ok := value.(map[string]any)
	if !ok || len(object) != 3 {
		return Fingerprint{}, false
	}
	h, a := object["head"].(string)
	s, b := object["source"].(string)
	p, c := object["safety"].(string)
	return Fingerprint{Head: h, Source: s, Safety: p}, a && b && c
}
func validateHistory(ctx context.Context, h History) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	schema, ok := integer(h.data["schema"])
	if !ok || schema.Cmp(big.NewInt(1)) != 0 {
		return checkpointError()
	}
	runs, ok := h.data["runs"].([]any)
	if !ok {
		return checkpointError()
	}
	seen := map[[2]string]bool{}
	for _, value := range runs {
		if err := ctx.Err(); err != nil {
			return err
		}
		row, ok := value.(map[string]any)
		if !ok {
			return checkpointError()
		}
		session, a := row["session"].(string)
		task, b := row["task"].(string)
		key := [2]string{session, task}
		if !a || !b || !budget.ValidSessionID(session) || !budget.ValidSessionID(task) || seen[key] {
			return checkpointError()
		}
		seen[key] = true
		harness, _ := row["harness"].(string)
		mode, _ := row["mode"].(string)
		status, _ := row["status"].(string)
		if !budget.ValidHarness(harness) || (mode != "manual" && mode != "bounded") || (status != "active" && status != "stopped" && status != "completed") {
			return checkpointError()
		}
		for _, key := range []string{"evidence", "budget_runs"} {
			if _, ok := row[key].([]any); !ok {
				return checkpointError()
			}
		}
		for _, key := range []string{"attempts", "no_progress"} {
			if !nonnegative(row[key], true) {
				return checkpointError()
			}
		}
		for _, key := range []string{"elapsed_seconds", "reserved_seconds"} {
			if !nonnegative(row[key], false) {
				return checkpointError()
			}
		}
		for _, key := range []string{"policy", "prompt", "started_at", "phase", "outcome", "stop_reason", "next_action"} {
			if _, ok := row[key].(string); !ok {
				return checkpointError()
			}
		}
		for _, key := range []string{"snapshot", "baseline"} {
			if _, ok := fingerprint(row[key]); !ok {
				return checkpointError()
			}
		}
		owner, ok := integer(row["owner_pid"])
		if !ok || owner.Sign() <= 0 {
			return checkpointError()
		}
		if row["process_pid"] != nil {
			pid, ok := integer(row["process_pid"])
			if !ok || pid.Sign() <= 0 {
				return checkpointError()
			}
		}
		if _, ok := row["uncertain"].(bool); !ok {
			return checkpointError()
		}
	}
	return finiteValue(ctx, h.data)
}
func clone(value any) any {
	switch v := value.(type) {
	case map[string]any:
		r := make(map[string]any, len(v))
		for k, c := range v {
			r[k] = clone(c)
		}
		return r
	case []any:
		r := make([]any, len(v))
		for i, c := range v {
			r[i] = clone(c)
		}
		return r
	default:
		return value
	}
}

// Lookup validates the whole history and returns a detached selected record.
func (h History) Lookup(ctx context.Context, session, task string) (Record, error) {
	if !budget.ValidSessionID(session) || !budget.ValidSessionID(task) {
		return Record{}, checkpointError()
	}
	if err := validateHistory(ctx, h); err != nil {
		return Record{}, err
	}
	for _, value := range h.data["runs"].([]any) {
		row := value.(map[string]any)
		if row["session"] == session && row["task"] == task {
			return Record{clone(row).(map[string]any)}, nil
		}
	}
	return Record{}, errors.New("loop checkpoint not found")
}

type limitedBuffer struct{ buffer bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > statefile.Limit-b.buffer.Len() {
		return 0, checkpointError()
	}
	return b.buffer.Write(p)
}
func encodeHistory(ctx context.Context, value any) ([]byte, error) {
	var b limitedBuffer
	if err := jsonvalue.EncodePython(ctx, &b, value); err != nil {
		return nil, checkpointError()
	}
	return b.buffer.Bytes(), nil
}

// MarshalJSON emits bounded Python-compatible JSON without losing string identity.
func (h History) MarshalJSON() ([]byte, error) { return encodeHistory(context.Background(), h.data) }

// MarshalJSON emits bounded checkpoint JSON.
func (r Record) MarshalJSON() ([]byte, error) { return encodeHistory(context.Background(), r.data) }
