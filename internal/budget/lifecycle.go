package budget

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// Admit rechecks the current ledger under the shared lock before appending one
// reservation: docs/adr/0069-go-budget-ledger-admission.md:79.
func (l *Ledger) Admit(ctx context.Context, r Request, c Config) (Admission, error) {
	return l.admit(ctx, r, c, nil)
}

// A controller may tighten execution time after waiting for the shared lock.
// The standalone ledger's finite-float contract remains unchanged.
// docs/adr/0070-go-budget-execution-controller.md:21.
type admissionPolicy func(context.Context, Config, float64) (Config, error)

func (l *Ledger) admit(ctx context.Context, r Request, c Config, policy admissionPolicy) (Admission, error) {
	if err := validateRequest(r); err != nil {
		return Admission{}, err
	}
	c, err := validateConfig(c)
	if err != nil {
		return Admission{}, err
	}
	initial, err := l.Read(ctx)
	if err != nil {
		return Admission{}, err
	}
	plan, err := MakePlan(ctx, r, c, initial)
	if err != nil {
		return Admission{}, err
	}
	if len(plan.Blockers) > 0 {
		return Admission{Plan: plan}, nil
	}
	s, err := l.locked(ctx)
	if err != nil {
		return Admission{}, err
	}
	defer s.close()
	history, err := s.read(ctx)
	if err != nil {
		return Admission{}, err
	}
	plan, err = MakePlan(ctx, r, c, history)
	if err != nil {
		return Admission{}, err
	}
	if len(plan.Blockers) > 0 {
		return Admission{Plan: plan}, nil
	}
	if err := ctx.Err(); err != nil {
		return Admission{}, err
	}
	if policy != nil {
		c, err = policy(ctx, c, plan.RemainingSessionSeconds)
		if err != nil {
			return Admission{Plan: plan}, err
		}
		plan.Configuration = c
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Admission{}, errors.New("cannot create budget identity")
	}
	random[6] = (random[6] & 15) | 64
	random[8] = (random[8] & 63) | 128
	id := hex.EncodeToString(random[:])
	attempt := new(big.Int).Add(new(big.Int).Sub(c.MaxAttempts, plan.RemainingAttempts), big.NewInt(1))
	row := map[string]any{
		"id": id, "session": r.Session, "task": r.Task, "harness": r.Harness, "role": r.Role, "model": r.Model,
		"started_at": timestamp(), "ended_at": nil, "elapsed_seconds": json.Number("0.0"), "reserved_seconds": floatNumber(math.Min(c.TimeoutSeconds, plan.RemainingSessionSeconds)),
		"attempt": integerNumber(attempt), "owner_pid": json.Number(new(big.Int).SetInt64(int64(os.Getpid())).String()), "process_pid": nil,
		"status": "active", "outcome": "running", "exit_code": nil, "tokens": nil, "estimated_usd": nil, "complete": false, "source": "not yet reported", "warnings": stringValues(plan.Warnings),
	}
	history.data["runs"] = append(history.data["runs"].([]any), row)
	if err := s.write(ctx, history, id, nil); err != nil {
		return Admission{Plan: plan}, err
	}
	record := cloneRecord(row)
	return Admission{Plan: plan, Record: &record}, nil
}
func timestamp() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000000+00:00") }
func stringValues(values []string) []any {
	result := make([]any, len(values))
	for i, s := range values {
		result[i] = s
	}
	return result
}
func ownerRecord(h History, id string) (map[string]any, error) {
	if !validID(id) {
		return nil, errors.New("invalid budget reservation identity")
	}
	for _, row := range h.rows() {
		if row["id"] != id {
			continue
		}
		owner, ok := exactInteger(row["owner_pid"])
		if !ok || !owner.IsInt64() || owner.Int64() != int64(os.Getpid()) || row["status"] != "active" {
			return nil, errors.New("budget reservation is completed or owned by another process")
		}
		return row, nil
	}
	return nil, errors.New("active budget reservation disappeared; preserve history and investigate")
}
func reconcilePID(row map[string]any, input *int) (*int, error) {
	if input != nil && (*input <= 0 || int64(*input) > math.MaxInt32) {
		return nil, errors.New("invalid native process identity")
	}
	if row["process_pid"] != nil {
		stored, ok := exactInteger(row["process_pid"])
		if !ok || !stored.IsInt64() || stored.Sign() <= 0 || stored.Int64() > math.MaxInt32 {
			return nil, errors.New("invalid recorded native process identity")
		}
		pid := int(stored.Int64())
		if input != nil && *input != pid {
			return nil, errors.New("conflicting native process identity")
		}
		return &pid, nil
	}
	return copyPID(input), nil
}

// PublishPID patches only the owning active row and never replaces its identity.
// docs/adr/0069-go-budget-ledger-admission.md:97.
func (l *Ledger) PublishPID(ctx context.Context, id string, pid int) (Record, error) {
	if !validID(id) || pid <= 0 || int64(pid) > math.MaxInt32 {
		return Record{}, errors.New("invalid budget process publication")
	}
	s, err := l.locked(ctx)
	if err != nil {
		return Record{}, err
	}
	defer s.close()
	history, err := s.read(ctx)
	if err != nil {
		return Record{}, err
	}
	row, err := ownerRecord(history, id)
	if err != nil {
		return Record{}, err
	}
	effective, err := reconcilePID(row, &pid)
	if err != nil {
		return Record{}, err
	}
	if row["process_pid"] != nil {
		return cloneRecord(row), nil
	}
	row["process_pid"] = json.Number(new(big.Int).SetInt64(int64(*effective)).String())
	if err := s.write(ctx, history, id, effective); err != nil {
		return Record{}, err
	}
	return cloneRecord(row), nil
}

// Finalize patches fresh metadata and retains any unresolved ownership, even
// after confirmed leader exit: docs/adr/0069-go-budget-ledger-admission.md:108.
func (l *Ledger) Finalize(ctx context.Context, id string, completion Completion, c Config) (Record, error) {
	if !validID(id) {
		return Record{}, errors.New("invalid budget reservation identity")
	}
	c, err := validateConfig(c)
	if err != nil {
		return Record{}, err
	}
	if err := validateCompletion(completion); err != nil {
		return Record{}, err
	}
	s, err := l.locked(ctx)
	if err != nil {
		return Record{}, err
	}
	defer s.close()
	history, err := s.read(ctx)
	if err != nil {
		return Record{}, err
	}
	row, err := ownerRecord(history, id)
	if err != nil {
		return Record{}, err
	}
	pid, err := reconcilePID(row, completion.ProcessPID)
	if err != nil {
		return Record{}, err
	}
	if pid == nil && (completion.ExitCode != nil || completion.ExitConfirmed) {
		return Record{}, errors.New("pre-spawn failure cannot claim native exit evidence")
	}
	uncertain := completion.OwnershipUnconfirmed || (pid != nil && (!completion.ExitConfirmed || completion.ExitCode == nil))
	if pid == nil && (uncertain || completion.Outcome != "launch_error") {
		return Record{}, errors.New("completion lacks native process ownership")
	}
	if pid != nil {
		row["process_pid"] = json.Number(new(big.Int).SetInt64(int64(*pid)).String())
	}
	outcome := completion.Outcome
	complete := completion.Complete
	if outcome == "completed" && (completion.Failed || !complete) {
		outcome = "failed"
	}
	row["outcome"], row["elapsed_seconds"], row["complete"], row["source"] = outcome, floatNumber(completion.ElapsedSeconds), complete, completion.Source
	row["exit_code"] = nil
	if completion.ExitCode != nil {
		row["exit_code"] = json.Number(new(big.Int).SetInt64(int64(*completion.ExitCode)).String())
	}
	row["tokens"] = nil
	if completion.Tokens != nil {
		tokens := map[string]any{}
		for key, value := range completion.Tokens {
			tokens[key] = integerNumber(value)
		}
		row["tokens"] = tokens
	}
	row["estimated_usd"] = nil
	if completion.EstimatedUSD != nil {
		row["estimated_usd"] = *completion.EstimatedUSD
	}
	warnings := row["warnings"].([]any)
	if outcome == "timeout" || outcome == "interrupted" || outcome == "output_limit" || outcome == "launch_error" || !complete || uncertain {
		row["tokens"], row["estimated_usd"], row["complete"] = nil, nil, false
	}
	if outcome == "launch_error" {
		warnings = append(warnings, "invocation or metadata processing failed; cost is unknown")
	} else if row["estimated_usd"] == nil {
		warnings = append(warnings, "cost is unknown; no complete session cost total can be claimed")
	}
	if uncertain {
		if outcome != "timeout" {
			row["outcome"] = "launch_error"
		}
		row["exit_code"], row["ended_at"], row["status"] = nil, nil, "active"
		warnings = append(warnings, "process exit is unconfirmed; reservation retained; confirm the process group has stopped before recovery in docs/BUDGETS.md")
	} else {
		row["status"], row["ended_at"] = "completed", timestamp()
	}
	row["warnings"] = warnings
	if c.EstimatedUSD != nil {
		request := Request{Session: row["session"].(string), Task: row["task"].(string), Harness: row["harness"].(string), Role: row["role"].(string), Model: row["model"].(string)}
		post, err := MakePlan(ctx, request, c, history)
		if err != nil {
			return Record{}, err
		}
		for _, warning := range append(post.Warnings, post.Blockers...) {
			if strings.Contains(warning, "cost") || strings.Contains(warning, "USD") {
				warnings = append(warnings, warning)
			}
		}
		row["warnings"] = warnings
	}
	if err := s.write(ctx, history, id, pid); err != nil {
		return Record{}, err
	}
	return cloneRecord(row), nil
}
func validateCompletion(c Completion) error {
	if !validOutcome(c.Outcome, false) || !finite(c.ElapsedSeconds) || c.ElapsedSeconds < 0 || !utf8.ValidString(c.Source) {
		return errors.New("invalid budget completion")
	}
	if c.ExitConfirmed && c.ExitCode == nil {
		return errors.New("confirmed native exit requires an exit code")
	}
	for key, value := range c.Tokens {
		if !utf8.ValidString(key) || value == nil || value.Sign() < 0 {
			return errors.New("invalid budget token metadata")
		}
		f, _ := value.Float64()
		if !finite(f) {
			return errors.New("invalid budget token metadata")
		}
	}
	if c.EstimatedUSD != nil {
		value, err := jsonNumberValue(*c.EstimatedUSD)
		if err != nil {
			return err
		}
		f, ok := numericFloat(value)
		if !ok || f < 0 {
			return errors.New("invalid budget cost metadata")
		}
	}
	return nil
}
func jsonNumberValue(value json.Number) (json.Number, error) {
	if !json.Valid([]byte(value)) {
		return "", errors.New("invalid budget cost metadata")
	}
	if _, ok := numericFloat(value); !ok {
		return "", errors.New("invalid budget cost metadata")
	}
	return value, nil
}
