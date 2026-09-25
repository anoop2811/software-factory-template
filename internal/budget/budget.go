// Package budget owns local admission, metadata and bounded native execution.
package budget

import (
	"encoding/json"
	"math/big"
)

type Config struct {
	Enabled        bool     `json:"enabled"`
	Action         string   `json:"action"`
	MaxAttempts    *big.Int `json:"max_attempts"`
	MaxSessionRuns *big.Int `json:"max_session_runs"`
	MaxConcurrent  *big.Int `json:"max_concurrent"`
	TimeoutSeconds float64  `json:"timeout_seconds"`
	SessionSeconds float64  `json:"session_seconds"`
	EstimatedUSD   *float64 `json:"estimated_usd"`
}
type Request struct {
	Session, Task, Harness, Role, Model string
	MaxCostUSD                          *string
}
type History struct{ data map[string]any }
type Record struct{ data map[string]any }
type Plan struct {
	Configuration           Config            `json:"configuration"`
	Session                 string            `json:"session"`
	Task                    string            `json:"task"`
	Harness                 string            `json:"harness"`
	Role                    string            `json:"role"`
	Model                   string            `json:"model"`
	ModelSource             string            `json:"model_source"`
	Services                map[string]string `json:"services"`
	CostReporting           string            `json:"cost_reporting"`
	RemainingAttempts       *big.Int          `json:"remaining_attempts"`
	RemainingSessionRuns    *big.Int          `json:"remaining_session_runs"`
	RemainingSessionSeconds float64           `json:"remaining_session_seconds"`
	ActiveRuns              int               `json:"active_runs"`
	Blockers                []string          `json:"blockers"`
	Warnings                []string          `json:"warnings"`
}
type ReportData struct {
	Schema int            `json:"schema"`
	Runs   []Record       `json:"runs"`
	Totals map[string]any `json:"totals"`
}
type Admission struct {
	Plan   Plan
	Record *Record
}
type Ledger struct {
	root string
	ops  storageOps
}
type Completion struct {
	Outcome                             string
	ExitCode                            *int
	ElapsedSeconds                      float64
	ProcessPID                          *int
	ExitConfirmed, OwnershipUnconfirmed bool
	Tokens                              map[string]*big.Int
	EstimatedUSD                        *json.Number
	Complete, Failed                    bool
	Source                              string
}
type PublicationError struct {
	RunID            string
	ProcessPID       *int
	MayHaveCommitted bool
}

func (*PublicationError) Error() string {
	return "budget publication failed; stop and inspect the ledger before proceeding"
}
func (h History) MarshalJSON() ([]byte, error) { return json.Marshal(h.data) }
func (r Record) MarshalJSON() ([]byte, error)  { return json.Marshal(r.data) }
func NewLedger(root string) *Ledger            { return &Ledger{root: root} }
