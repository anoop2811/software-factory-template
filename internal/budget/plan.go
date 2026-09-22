package budget

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"syscall"
)

// MakePlan is read-only; hard limits stay blockers even when cost action is warn.
// docs/adr/0069-go-budget-ledger-admission.md:51.
func MakePlan(ctx context.Context, r Request, c Config, h History) (Plan, error) {
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	if err := validateRequest(r); err != nil {
		return Plan{}, err
	}
	c, err := validateConfig(c)
	if err != nil {
		return Plan{}, err
	}
	if err := validateHistory(ctx, h); err != nil {
		return Plan{}, err
	}
	var session, task, active []map[string]any
	charge, cost := newSum(), newSum()
	for _, row := range h.rows() {
		if err := ctx.Err(); err != nil {
			return Plan{}, err
		}
		if row["status"] == "active" {
			active = append(active, row)
		}
		if row["session"] != r.Session {
			continue
		}
		session = append(session, row)
		if row["task"] == r.Task {
			task = append(task, row)
		}
		amount := row["elapsed_seconds"]
		if row["status"] == "active" {
			amount = row["reserved_seconds"]
		}
		if err := charge.add(amount); err != nil {
			return Plan{}, err
		}
		if c.EstimatedUSD != nil && row["estimated_usd"] != nil {
			if err := cost.add(row["estimated_usd"]); err != nil {
				return Plan{}, err
			}
		}
	}
	charged, err := charge.float()
	if err != nil {
		return Plan{}, err
	}
	p := Plan{Configuration: c, Session: r.Session, Task: r.Task, Harness: r.Harness, Role: r.Role, Model: r.Model, ModelSource: "inherited", Services: map[string]string{"model_provider": "inherited; usage may incur provider charges", "tools": "unknown; inherited from harness/project configuration"}, CostReporting: "client-reported estimate, not billing", RemainingAttempts: remaining(c.MaxAttempts, len(task)), RemainingSessionRuns: remaining(c.MaxSessionRuns, len(session)), RemainingSessionSeconds: math.Max(0, c.SessionSeconds-charged), ActiveRuns: len(active), Blockers: []string{}, Warnings: []string{}}
	if r.Model != "" {
		p.ModelSource = "selected"
	}
	if r.Harness == "codex" {
		p.CostReporting = "unknown"
	}
	if !c.Enabled {
		p.Blockers = append(p.Blockers, "budget feature is disabled; set budget_enabled: true to permit explicit runs")
	}
	if r.MaxCostUSD != nil {
		p.Blockers = append(p.Blockers, "strict USD ceilings are unsupported for all harnesses; estimates are not billing limits")
	}
	for _, row := range active {
		id := row["id"].(string)
		if row["outcome"] != "running" {
			p.Blockers = append(p.Blockers, fmt.Sprintf("unconfirmed process exit for run %s: confirm its process group is stopped, then recover its metadata using docs/BUDGETS.md", id))
			continue
		}
		alive, err := ownerAlive(row["owner_pid"])
		if err != nil {
			return Plan{}, err
		}
		if !alive {
			p.Blockers = append(p.Blockers, fmt.Sprintf("stale active run %s: confirm its process group is stopped, then recover its metadata using docs/BUDGETS.md; do not delete the attempt", id))
		}
	}
	if p.RemainingAttempts.Sign() == 0 {
		p.Blockers = append(p.Blockers, "task attempt limit reached")
	}
	if p.RemainingSessionRuns.Sign() == 0 {
		p.Blockers = append(p.Blockers, "session run limit reached")
	}
	if p.RemainingSessionSeconds <= 0 {
		p.Blockers = append(p.Blockers, "session time limit reached")
	}
	if new(big.Int).SetInt64(int64(len(active))).Cmp(c.MaxConcurrent) >= 0 {
		p.Blockers = append(p.Blockers, "checkout concurrency limit reached")
	}
	if c.EstimatedUSD != nil {
		concerns := []string{}
		if r.Harness == "codex" {
			concerns = append(concerns, "Codex does not report a usable USD estimate")
		}
		for _, row := range session {
			if row["status"] == "active" {
				concerns = append(concerns, "another session invocation has not reported its cost")
				break
			}
		}
		for _, row := range session {
			if row["status"] != "active" && row["estimated_usd"] == nil {
				concerns = append(concerns, "previous session cost is unknown")
				break
			}
		}
		reached, err := cost.atLeast(*c.EstimatedUSD)
		if err != nil {
			return Plan{}, err
		}
		if reached {
			known, err := cost.float()
			if err != nil {
				return Plan{}, err
			}
			concerns = append(concerns, fmt.Sprintf("session estimated USD threshold reached (%.6f >= %.6f)", known, *c.EstimatedUSD))
		}
		if c.Action == "stop" {
			p.Blockers = append(p.Blockers, concerns...)
		} else {
			p.Warnings = append(p.Warnings, concerns...)
		}
	}
	return p, ctx.Err()
}
func remaining(limit *big.Int, count int) *big.Int {
	r := new(big.Int).Sub(limit, new(big.Int).SetInt64(int64(count)))
	if r.Sign() < 0 {
		r.SetInt64(0)
	}
	return r
}
func ownerAlive(value any) (bool, error) {
	pid, ok := exactInteger(value)
	if !ok || !pid.IsInt64() || pid.Sign() <= 0 || pid.Int64() > math.MaxInt32 {
		return false, errors.New("active budget owner PID is outside the operating-system range")
	}
	err := syscall.Kill(int(pid.Int64()), 0)
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	if err == nil || errors.Is(err, syscall.EPERM) {
		return true, nil
	}
	return false, errors.New("cannot establish active budget owner status")
}

// Report preserves exact integer-only totals and CPython's float accumulation.
// docs/adr/0069-go-budget-ledger-admission.md:27.
func Report(ctx context.Context, h History, session string) (ReportData, error) {
	if session != "" && !validID(session) {
		return ReportData{}, errors.New("invalid budget report session")
	}
	if err := validateHistory(ctx, h); err != nil {
		return ReportData{}, err
	}
	report := ReportData{Schema: 1, Runs: []Record{}}
	elapsed, reserved, cost := newSum(), newSum(), newSum()
	active, unknown := 0, 0
	for _, row := range h.rows() {
		if err := ctx.Err(); err != nil {
			return ReportData{}, err
		}
		if session != "" && row["session"] != session {
			continue
		}
		report.Runs = append(report.Runs, cloneRecord(row))
		if err := elapsed.add(row["elapsed_seconds"]); err != nil {
			return ReportData{}, err
		}
		if row["status"] == "active" {
			active++
			if err := reserved.add(row["reserved_seconds"]); err != nil {
				return ReportData{}, err
			}
		}
		if row["estimated_usd"] == nil {
			unknown++
		} else {
			if err := cost.add(row["estimated_usd"]); err != nil {
				return ReportData{}, err
			}
		}
	}
	e, err := elapsed.number()
	if err != nil {
		return ReportData{}, err
	}
	r, err := reserved.number()
	if err != nil {
		return ReportData{}, err
	}
	c, err := cost.number()
	if err != nil {
		return ReportData{}, err
	}
	report.Totals = map[string]any{"runs": len(report.Runs), "active": active, "elapsed_seconds": e, "reserved_seconds": r, "known_estimated_usd": c, "unknown_cost_runs": unknown, "cost_complete": unknown == 0}
	return report, ctx.Err()
}
