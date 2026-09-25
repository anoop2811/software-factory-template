package loop

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/anoop2811/software-factory-template/internal/budget"
	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
)

const manualNextAction = "Inspect changes and evidence; hand off to a human. Use a new task only after resolving the stop reason."

func number(value float64) json.Number { return json.Number(jsonvalue.FloatText(value)) }
func snapshotValue(f Fingerprint) map[string]any {
	return map[string]any{"head": f.Head, "source": f.Source, "safety": f.Safety}
}
func duration(seconds float64) time.Duration {
	if seconds <= 0 {
		return 0
	}
	if seconds >= float64(math.MaxInt64)/float64(time.Second) {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(seconds * float64(time.Second))
}

// Run holds checkpoint ownership through check execution and terminal publication.
// docs/adr/0075-go-manual-loop-controller.md:47.
func (c *ManualController) Run(ctx context.Context, r ManualRequest, resume bool) (ManualResult, error) {
	if c == nil || c.store == nil || !validManual(r) {
		return ManualResult{}, manualError()
	}
	h, err := c.store.Read(ctx)
	if err != nil {
		return ManualResult{}, err
	}

	cfg, bc, err := c.configuration(ctx, r)
	if err != nil {
		return ManualResult{}, err
	}
	bh, err := c.ops.readBudget(ctx)
	if err != nil {
		return ManualResult{}, err
	}
	plan, err := makeManualPlan(ctx, r, cfg, h, bh)
	if err != nil {
		return ManualResult{}, err
	}
	if len(plan.Blockers) > 0 {
		return ManualResult{Plan: &plan, ExitCode: 2}, nil
	}
	transaction, err := c.store.Lock(ctx)
	if err != nil {
		return ManualResult{}, err
	}
	defer transaction.Close()
	h, err = transaction.Read(ctx)
	if err != nil {
		return ManualResult{}, err
	}
	bh, err = c.ops.readBudget(ctx)
	if err != nil {
		return ManualResult{}, err
	}
	plan, err = makeManualPlan(ctx, r, cfg, h, bh)
	if err != nil {
		return ManualResult{}, err
	}
	if len(plan.Blockers) > 0 {
		return ManualResult{Plan: &plan, ExitCode: 2}, nil
	}
	current, err := c.ops.snapshot(ctx, c.root, cfg, r.Environment)
	if err != nil {
		return ManualResult{}, err
	}
	policy, err := c.ops.policy(ctx, cfg, bc, r.Environment)
	if err != nil {
		return ManualResult{}, err
	}
	prompt, err := digest(ctx, "")
	if err != nil {
		return ManualResult{}, err
	}
	var row map[string]any
	for _, value := range h.data["runs"].([]any) {
		candidate := value.(map[string]any)
		if candidate["session"] == r.Session && candidate["task"] == r.Task {
			row = candidate
			break
		}
	}
	if resume {
		_, err := AssessResume(ctx, h, bh, ResumeRequest{Session: r.Session, Task: r.Task, Harness: r.Harness, Mode: "manual", Snapshot: current, Policy: policy, Prompt: prompt, TimeoutSeconds: cfg.TimeoutSeconds})
		if err != nil {
			return ManualResult{}, err
		}
		row["status"] = "active"
		row["owner_pid"] = json.Number(strconv.Itoa(os.Getpid()))
	} else {
		if row != nil {
			return ManualResult{}, manualError()
		}
		row = map[string]any{"session": r.Session, "task": r.Task, "harness": r.Harness, "mode": "manual", "status": "active", "outcome": "running", "phase": "starting", "owner_pid": json.Number(strconv.Itoa(os.Getpid())), "process_pid": nil, "started_at": time.Now().UTC().Format("2006-01-02T15:04:05Z"), "elapsed_seconds": number(0), "reserved_seconds": number(cfg.TimeoutSeconds), "attempts": json.Number("0"), "no_progress": json.Number("0"), "uncertain": false, "baseline": snapshotValue(current), "snapshot": snapshotValue(current), "policy": policy, "prompt": prompt, "evidence": []any{}, "budget_runs": []any{}, "stop_reason": "", "next_action": "Controller is active; do not overlap another run."}
		h.data["runs"] = append(h.data["runs"].([]any), row)
	}
	if err := transaction.Write(ctx, h); err != nil {
		return ManualResult{}, err
	}
	consumed, _ := row["elapsed_seconds"].(json.Number).Float64()
	invocation := manualInvocation{controller: c, request: r, config: cfg, budget: bc, history: h, row: row, transaction: transaction, started: time.Now(), consumed: consumed}
	return invocation.check(ctx)
}

type manualInvocation struct {
	controller  *ManualController
	request     ManualRequest
	config      Config
	budget      budget.Config
	history     History
	row         map[string]any
	transaction *Transaction
	started     time.Time
	consumed    float64
}

func (m *manualInvocation) elapsed() float64 { return m.consumed + time.Since(m.started).Seconds() }
func (m *manualInvocation) save(ctx context.Context) error {
	m.row["elapsed_seconds"] = number(m.elapsed())
	return m.transaction.Write(ctx, m.history)
}
func (m *manualInvocation) fresh(ctx context.Context, previous *Fingerprint) (Fingerprint, error) {
	policy, err := m.controller.ops.policy(ctx, m.config, m.budget, m.request.Environment)
	if err != nil || policy != m.row["policy"] {
		return Fingerprint{}, manualError()
	}
	current, err := m.controller.ops.snapshot(ctx, m.controller.root, m.config, m.request.Environment)
	if err != nil {
		return Fingerprint{}, err
	}
	baseline, _ := fingerprint(m.row["baseline"])
	if current.Head != baseline.Head || current.Safety != baseline.Safety || (previous != nil && current != *previous) {
		return Fingerprint{}, manualError()
	}
	m.row["snapshot"] = snapshotValue(current)
	return current, nil
}
func (m *manualInvocation) finish(parent context.Context, outcome, reason string) (ManualResult, error) {
	if m.transaction.failed {
		return ManualResult{}, manualError()
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer cancel()
	deadline, _ := ctx.Deadline()
	if parent.Err() != nil {
		outcome = "interrupted"
		reason = "Controller interrupted; inspect owned processes before recovery"
	}
	m.row["status"] = "stopped"
	m.row["outcome"] = outcome
	m.row["phase"] = "finished"
	m.row["stop_reason"] = reason
	m.row["next_action"] = manualNextAction
	if err := m.save(ctx); err != nil {
		return ManualResult{}, err
	}
	record := Record{clone(m.row).(map[string]any)}
	status := 2
	if outcome == "manual_passed" {
		status = 0
	}
	return ManualResult{Record: &record, ExitCode: status, outputDeadline: deadline}, nil
}
func (m *manualInvocation) check(parent context.Context) (ManualResult, error) {
	ctx, cancel := context.WithDeadline(parent, m.started.Add(duration(m.config.TimeoutSeconds-m.consumed)))
	defer cancel()
	if ctx.Err() != nil {
		return m.finish(parent, "handoff", "loop time limit reached")
	}
	bh, err := m.controller.ops.readBudget(ctx)
	if ctx.Err() != nil {
		return m.finish(parent, "handoff", "loop time limit reached")
	}
	if err != nil {
		m.row["uncertain"] = true
		return m.finish(parent, "handoff", "Cannot establish budget ownership before check")
	}
	active, err := bh.HasActive(ctx)
	if err != nil || active {
		m.row["uncertain"] = true
		return m.finish(parent, "handoff", "active budget invocation blocks checks; inspect owned processes and recover first")
	}
	before, err := m.fresh(ctx, nil)
	if err != nil {
		return m.finish(parent, "handoff", "Source or governing configuration changed; human handoff required")
	}
	allowance := duration(min(m.config.CheckTimeoutSeconds, m.config.TimeoutSeconds-m.elapsed()))
	if allowance <= 0 {
		return m.finish(parent, "handoff", "loop time limit reached")
	}
	m.row["phase"] = "check"
	if err := m.save(ctx); err != nil {
		if ctx.Err() != nil && !m.transaction.failed {
			return m.finish(parent, "handoff", "loop time limit reached")
		}
		return ManualResult{}, err
	}
	allowance = duration(min(m.config.CheckTimeoutSeconds, m.config.TimeoutSeconds-m.elapsed()))
	if ctx.Err() != nil || allowance <= 0 {
		return m.finish(parent, "handoff", "loop time limit reached")
	}
	execution, executionErr := m.controller.ops.execute(ctx, m.controller.root, m.config.CheckCommand, m.request.Environment, allowance, func(spawnContext context.Context, pid int) error {
		if pid <= 0 {
			return manualError()
		}
		m.row["process_pid"] = json.Number(strconv.Itoa(pid))
		return m.save(spawnContext)
	})
	if m.transaction.failed {
		return ManualResult{}, manualError()
	}
	if execution.ProcessPID > 0 && !execution.ExitConfirmed {
		m.row["process_pid"] = json.Number(strconv.Itoa(execution.ProcessPID))
	}
	certain := execution.ExitConfirmed && execution.ExitCode != nil && !execution.OwnershipUnconfirmed
	if certain {
		m.row["process_pid"] = nil
	} else if execution.ProcessPID > 0 {
		m.row["process_pid"] = json.Number(strconv.Itoa(execution.ProcessPID))
	}
	m.row["uncertain"] = execution.OwnershipUnconfirmed || (!certain && m.row["process_pid"] != nil)
	if executionErr != nil || execution.ProcessPID <= 0 {
		return m.finish(parent, "handoff", "Cannot confirm deterministic check execution; inspect owned processes")
	}
	var code any
	if execution.ExitCode != nil {
		code = json.Number(strconv.Itoa(*execution.ExitCode))
	}
	m.row["evidence"] = append(m.row["evidence"].([]any), map[string]any{"kind": "check", "command": jsonvalue.RawString(m.config.CheckCommand), "exit_code": code, "outcome": execution.Outcome, "duration_seconds": number(execution.ElapsedSeconds), "snapshot": snapshotValue(before), "harness": m.request.Harness, "role": "deterministic"})
	if ctx.Err() != nil {
		if parent.Err() != nil {
			return m.finish(parent, "interrupted", "Controller interrupted; inspect owned processes before recovery")
		}
		return m.finish(parent, "handoff", "loop time limit reached")
	}
	if err := m.save(ctx); err != nil {
		return ManualResult{}, err
	}
	if _, err := m.fresh(ctx, &before); err != nil {
		return m.finish(parent, "handoff", "Source or governing configuration changed; human handoff required")
	}
	if !certain || (execution.Outcome != "completed" && execution.Outcome != "failed") {
		return m.finish(parent, "handoff", "check "+execution.Outcome+"; handoff required")
	}
	if *execution.ExitCode == 0 {
		return m.finish(parent, "manual_passed", "Manual check passed; implementation and review were not performed.")
	}
	return m.finish(parent, "manual_failed", "Manual check failed; no model was invoked.")
}
