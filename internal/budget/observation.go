package budget

import (
	"context"
	"encoding/json"
	"math/big"
)

// Invocation exposes detached ownership/accounting facts without mutable ledger maps.
// docs/adr/0076-go-bounded-loop-controller.md:107.
type Invocation struct {
	ID, Session, Task, Harness, Role, Status, Outcome string
	ElapsedSeconds                                    json.Number
	ProcessPID                                        *big.Int
}

// Observe validates a record and preserves exact elapsed/PID values for orchestration.
func (r Record) Observe(ctx context.Context) (Invocation, error) {
	if err := ctx.Err(); err != nil {
		return Invocation{}, err
	}
	if err := validateRecord(r.data); err != nil {
		return Invocation{}, err
	}
	row := r.data
	result := Invocation{ID: row["id"].(string), Session: row["session"].(string), Task: row["task"].(string), Harness: row["harness"].(string), Role: row["role"].(string), Status: row["status"].(string), Outcome: row["outcome"].(string), ElapsedSeconds: row["elapsed_seconds"].(json.Number)}
	if row["process_pid"] != nil {
		result.ProcessPID, _ = exactInteger(row["process_pid"])
	}
	return result, nil
}

// MatchingActive validates whole history before returning matching active invocations.
func (h History) MatchingActive(ctx context.Context, session, task string) ([]Invocation, error) {
	if err := validateHistory(ctx, h); err != nil {
		return nil, err
	}
	result := []Invocation{}
	for _, row := range h.rows() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if row["status"] == "active" && row["session"] == session && row["task"] == task {
			value, err := (Record{row}).Observe(ctx)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
	}
	return result, nil
}
