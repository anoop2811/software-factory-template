package loop

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"time"

	"github.com/anoop2811/software-factory-template/internal/budget"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/output"
)

// ManualRequest selects a deterministic check, never a model invocation.
type ManualRequest struct {
	Harness, Session, Task string
	Environment            map[string]string
}

// ManualPlan reports existing Python manual admission policy.
type ManualPlan struct {
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

// ManualResult contains only an admitted plan or durably published terminal record.
type ManualResult struct {
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
}

// ManualController composes existing storage, fingerprints and process supervision.
// It is not safe for concurrent use; each Run holds the persistent checkout lock.
type ManualController struct {
	root   string
	store  *Store
	ledger *budget.Ledger
	ops    manualOps
}

// NewManualController selects a checkout without reading or mutating it.
func NewManualController(root string) *ManualController {
	c := &ManualController{root: root, store: NewStore(root), ledger: budget.NewLedger(root)}
	c.ops = manualOps{execute: native.ExecuteCheck, snapshot: StableSnapshot, policy: Policy, readBudget: c.ledger.Read}
	return c
}
func validManual(r ManualRequest) bool {
	return budget.ValidHarness(r.Harness) && budget.ValidSessionID(r.Session) && budget.ValidSessionID(r.Task)
}
func manualError() error { return errors.New("cannot execute manual loop") }
func makeManualPlan(ctx context.Context, r ManualRequest, c Config, h History, b budget.History) (ManualPlan, error) {
	if err := validateHistory(ctx, h); err != nil {
		return ManualPlan{}, err
	}
	active, err := b.HasActive(ctx)
	if err != nil {
		return ManualPlan{}, err
	}
	p := ManualPlan{Session: r.Session, Task: r.Task, Harness: r.Harness, Mode: "manual", Configuration: c, Blockers: []string{}, MaxImplementerAttempts: new(big.Int).Set(c.MaxAttempts), ReviewerCountsTowardBudgetAttempts: true, Warnings: []string{"Reviewer invocations consume the same task/session budget; limits are never raised.", "Native readiness is checked only before an explicit bounded invocation."}}
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
	return p, nil
}
func (c *ManualController) configuration(ctx context.Context, r ManualRequest) (Config, budget.Config, error) {
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
// docs/adr/0075-go-manual-loop-controller.md:31.
func (c *ManualController) Plan(ctx context.Context, r ManualRequest) (ManualPlan, error) {
	if c == nil || c.store == nil || !validManual(r) {
		return ManualPlan{}, manualError()
	}
	h, err := c.store.Read(ctx)
	if err != nil {
		return ManualPlan{}, err
	}

	cfg, _, err := c.configuration(ctx, r)
	if err != nil {
		return ManualPlan{}, err
	}
	bh, err := c.ops.readBudget(ctx)
	if err != nil {
		return ManualPlan{}, err
	}
	p, err := makeManualPlan(ctx, r, cfg, h, bh)
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
	if !r.outputDeadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(context.WithoutCancel(ctx), r.outputDeadline)
		defer cancel()
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
