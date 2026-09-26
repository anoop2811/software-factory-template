package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/budget"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/output"
)

// Request selects explicit loop mode and captured execution inputs.
type Request struct {
	Mode, PromptFile       string
	Harness, Session, Task string
	Environment            map[string]string
}

// Plan reports shared loop and budget admission without execution.
type Plan struct {
	Session                            string       `json:"session"`
	Task                               string       `json:"task"`
	Harness                            string       `json:"harness"`
	Mode                               string       `json:"mode"`
	Configuration                      Config       `json:"configuration"`
	Budget                             *budget.Plan `json:"budget"`
	Blockers                           []string     `json:"blockers"`
	MaxImplementerAttempts             *big.Int     `json:"max_implementer_attempts"`
	ReviewerCountsTowardBudgetAttempts bool         `json:"reviewer_counts_toward_budget_attempts"`
	Warnings                           []string     `json:"warnings"`
}

// Result contains only an admitted plan or durably published terminal record.
type Result struct {
	Plan           *ManualPlan
	Record         *Record
	ExitCode       int
	outputDeadline time.Time
}
type manualOps struct {
	execute    func(context.Context, string, string, map[string]string, time.Duration, func(context.Context, int) error) (native.Execution, error)
	snapshot   func(context.Context, string, Config, map[string]string) (Fingerprint, error)
	policy     func(context.Context, Config, budget.Config, map[string]string) (string, error)
	readBudget func(context.Context) (budget.History, error)
	invoke     func(context.Context, budget.Request, budget.Config, budget.RunInput) (budget.RunResult, error)
}

// Controller composes existing storage, fingerprints and shared execution components.
// It is not safe for concurrent use; each Run holds the persistent checkout lock.
type Controller struct {
	root   string
	store  *Store
	ledger *budget.Ledger
	ops    manualOps
}

// NewController selects a checkout without reading or mutating it.
func NewController(root string) *Controller {
	c := &Controller{root: root, store: NewStore(root), ledger: budget.NewLedger(root)}
	c.ops = manualOps{execute: native.ExecuteCheck, snapshot: StableSnapshot, policy: Policy, readBudget: c.ledger.Read, invoke: budget.NewRunner(root).Run}
	return c
}
func validManual(r ManualRequest) bool {
	return budget.ValidHarness(r.Harness) && budget.ValidSessionID(r.Session) && budget.ValidSessionID(r.Task) && (requestMode(r) == "manual" || requestMode(r) == "bounded")
}
func manualError() error { return errors.New("cannot execute manual loop") }
func makeManualPlan(ctx context.Context, r ManualRequest, c Config, bc budget.Config, h History, b budget.History) (ManualPlan, error) {
	if err := validateHistory(ctx, h); err != nil {
		return ManualPlan{}, err
	}
	active, err := b.HasActive(ctx)
	if err != nil {
		return ManualPlan{}, err
	}
	p := ManualPlan{Session: r.Session, Task: r.Task, Harness: r.Harness, Mode: requestMode(r), Configuration: c, Blockers: []string{}, MaxImplementerAttempts: new(big.Int).Set(c.MaxAttempts), ReviewerCountsTowardBudgetAttempts: true, Warnings: []string{"Reviewer invocations consume the same task/session budget; limits are never raised.", "Native readiness is checked only before an explicit bounded invocation."}}
	if active {
		p.Blockers = append(p.Blockers, "active budget invocation blocks loop execution; confirm owned processes and recover budget metadata first")
	}
	if len(pythonFields(c.CheckCommand)) == 0 {
		p.Blockers = append(p.Blockers, "no loop_check_command or check_command is configured")
	}
	for _, value := range h.data["runs"].([]any) {
		row := value.(map[string]any)
		if row["status"] == "active" || row["uncertain"] == true {
			p.Blockers = append(p.Blockers, "active or uncertain checkpoint requires inspection and explicit recovery")
			break
		}
	}
	if requestMode(r) == "bounded" {
		if !c.Enabled {
			p.Blockers = append(p.Blockers, "loop feature is disabled; set loop_enabled: true")
		}
		nativePlan, err := budget.MakePlan(ctx, budgetRequest(r, "implementer"), bc, b)
		if err != nil {
			return ManualPlan{}, err
		}
		p.Budget = &nativePlan
		p.Blockers = append(p.Blockers, nativePlan.Blockers...)
	}
	return p, nil
}
func (c *Controller) configuration(ctx context.Context, r ManualRequest) (Config, budget.Config, error) {
	if c == nil || c.store == nil || !validManual(r) {
		return Config{}, budget.Config{}, manualError()
	}
	cfg, err := Configuration(ctx, r.Environment)
	if err != nil {
		return Config{}, budget.Config{}, err
	}
	bc, err := budget.Configuration(r.Environment)
	return cfg, bc, err
}

// Plan validates a stable observation but never creates checkpoint infrastructure.
// docs/adr/0075-go-manual-loop-controller.md:32.
func (c *Controller) Plan(ctx context.Context, r ManualRequest) (ManualPlan, error) {
	if c == nil || c.store == nil || !validManual(r) {
		return ManualPlan{}, manualError()
	}
	h, err := c.store.Read(ctx)
	if err != nil {
		return ManualPlan{}, err
	}

	cfg, bc, err := c.configuration(ctx, r)
	if err != nil {
		return ManualPlan{}, err
	}
	bh, err := c.ops.readBudget(ctx)
	if err != nil {
		return ManualPlan{}, err
	}
	p, err := makeManualPlan(ctx, r, cfg, bc, h, bh)
	if err != nil {
		return ManualPlan{}, err
	}
	if _, err := c.ops.snapshot(ctx, c.root, cfg, r.Environment); err != nil {
		return ManualPlan{}, err
	}
	return p, nil
}

// Write emits only the public result using the same terminal cleanup deadline.
func (r ManualResult) Write(ctx context.Context, w io.Writer) error {
	return r.WriteFormat(ctx, w, true)
}

// WriteFormat renders the existing human structure or one JSON result.
func (r ManualResult) WriteFormat(ctx context.Context, w io.Writer, jsonOutput bool) error {
	if !r.outputDeadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(context.WithoutCancel(ctx), r.outputDeadline)
		defer cancel()
	}
	if !jsonOutput {
		var text strings.Builder
		switch {
		case r.Plan != nil:
			p := r.Plan
			fmt.Fprintf(&text, "Loop plan: %s (%s) %s/%s\nImplementer attempts: %s; reviewer invocations also consume budget attempts\nCheck: %s\n", p.Harness, p.Mode, p.Session, p.Task, p.MaxImplementerAttempts.String(), p.Configuration.CheckCommand)
			for _, blocker := range p.Blockers {
				fmt.Fprintf(&text, "BLOCKED: %s\n", blocker)
			}
		case r.Record != nil:
			row := r.Record.data
			fmt.Fprintf(&text, "Loop %s/%s: %s / %s\nReason: %s\nNext: %s\n", row["session"], row["task"], row["status"], row["outcome"], row["stop_reason"], row["next_action"])
		default:
			return manualError()
		}
		return output.WriteEvent(ctx, w, []byte(text.String()))
	}
	var value any
	switch {
	case r.Record != nil:
		value = r.Record
	case r.Plan != nil:
		value = r.Plan
	default:
		return manualError()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return manualError()
	}
	if err := output.WriteEvent(ctx, w, append(data, '\n')); err != nil {
		return manualError()
	}
	return nil
}

// ManualRequest preserves the existing private manual component input name.
type ManualRequest = Request

// ManualPlan preserves the existing manual plan representation.
type ManualPlan = Plan

// ManualResult preserves existing manual result/output semantics.
type ManualResult = Result

// ManualController restricts the compatibility entry point to unpaid checks.
type ManualController struct{ *Controller }

// NewManualController creates the manual-only compatibility facade.
func NewManualController(root string) *ManualController {
	return &ManualController{NewController(root)}
}
func (c *ManualController) Plan(ctx context.Context, r ManualRequest) (ManualPlan, error) {
	if c == nil || c.Controller == nil || r.Mode != "" && r.Mode != "manual" {
		return ManualPlan{}, manualError()
	}
	return c.Controller.Plan(ctx, r)
}
func (c *ManualController) Run(ctx context.Context, r ManualRequest, resume bool) (ManualResult, error) {
	if c == nil || c.Controller == nil || r.Mode != "" && r.Mode != "manual" {
		return ManualResult{}, manualError()
	}
	return c.Controller.Run(ctx, r, resume)
}

// Modes returns the fixed domain alternatives used by command metadata.
func Modes() []string { return []string{"manual", "bounded"} }
func requestMode(r Request) string {
	if r.Mode == "" {
		return "manual"
	}
	return r.Mode
}

// Selection returns validated immutable routing metadata for resume command admission.
func (r Record) Selection(ctx context.Context) (string, string, error) {
	if err := validateHistory(ctx, History{map[string]any{"schema": json.Number("1"), "runs": []any{r.data}}}); err != nil {
		return "", "", err
	}
	return r.data["harness"].(string), r.data["mode"].(string), nil
}
