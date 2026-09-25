package loop

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"math/big"

	"github.com/anoop2811/software-factory-template/internal/budget"
)

// ResumeRequest supplies explicit observations; it does not establish their freshness.
type ResumeRequest struct {
	Session, Task, Harness, Mode string
	Snapshot                     Fingerprint
	Policy, Prompt               string
	TimeoutSeconds               float64
}

// ResumeAssessment reports pure eligibility without granting execution permission.
type ResumeAssessment struct {
	Eligible         bool    `json:"eligible"`
	RemainingSeconds float64 `json:"remaining_seconds"`
	Record           Record  `json:"record"`
}

// ParseResumeRequest validates the bounded private request without touching storage.
func ParseResumeRequest(ctx context.Context, input io.Reader, session, task string) (ResumeRequest, error) {
	value, err := readValue(ctx, input)
	if err != nil {
		return ResumeRequest{}, err
	}
	fields, ok := value.(map[string]any)
	if !ok || len(fields) != 6 {
		return ResumeRequest{}, checkpointError()
	}
	r := ResumeRequest{Session: session, Task: task}
	var valid bool
	if r.Harness, valid = fields["harness"].(string); !valid {
		return r, checkpointError()
	}
	if r.Mode, valid = fields["mode"].(string); !valid {
		return r, checkpointError()
	}
	if r.Policy, valid = fields["policy"].(string); !valid {
		return r, checkpointError()
	}
	if r.Prompt, valid = fields["prompt"].(string); !valid {
		return r, checkpointError()
	}
	if r.Snapshot, valid = fingerprint(fields["snapshot"]); !valid {
		return r, checkpointError()
	}
	timeout, ok := fields["timeout_seconds"].(json.Number)
	if !ok {
		return r, checkpointError()
	}
	r.TimeoutSeconds, err = timeout.Float64()
	if err != nil || !validResumeRequest(r) {
		return r, checkpointError()
	}
	return r, nil
}
func validResumeRequest(r ResumeRequest) bool {
	return budget.ValidSessionID(r.Session) && budget.ValidSessionID(r.Task) && budget.ValidHarness(r.Harness) && (r.Mode == "manual" || r.Mode == "bounded") && r.TimeoutSeconds > 0 && !math.IsInf(r.TimeoutSeconds, 0) && !math.IsNaN(r.TimeoutSeconds)
}

// AssessResume never mutates evidence or turns dead owners into recovery permission.
// docs/adr/0074-go-loop-checkpoint-storage.md:89.
func AssessResume(ctx context.Context, h History, b budget.History, r ResumeRequest) (ResumeAssessment, error) {
	if !validResumeRequest(r) {
		return ResumeAssessment{}, checkpointError()
	}
	if err := validateHistory(ctx, h); err != nil {
		return ResumeAssessment{}, err
	}
	active, err := b.HasActive(ctx)
	if err != nil || active {
		return ResumeAssessment{}, checkpointError()
	}
	for _, value := range h.data["runs"].([]any) {
		if err := ctx.Err(); err != nil {
			return ResumeAssessment{}, err
		}
		row := value.(map[string]any)
		if row["status"] == "active" || row["uncertain"] == true {
			return ResumeAssessment{}, checkpointError()
		}
	}
	record, err := h.Lookup(ctx, r.Session, r.Task)
	if err != nil {
		return ResumeAssessment{}, err
	}
	row := record.data
	previous, _ := fingerprint(row["snapshot"])
	if row["harness"] != r.Harness || row["mode"] != r.Mode || r.Mode != "manual" || previous != r.Snapshot || row["policy"] != r.Policy || row["prompt"] != r.Prompt || (row["outcome"] != "manual_passed" && row["outcome"] != "manual_failed") {
		return ResumeAssessment{}, checkpointError()
	}
	elapsed := row["elapsed_seconds"].(json.Number)
	f, _ := elapsed.Float64()
	if exact, ok := integer(elapsed); ok {
		if new(big.Rat).SetInt(exact).Cmp(new(big.Rat).SetFloat64(r.TimeoutSeconds)) >= 0 {
			return ResumeAssessment{}, checkpointError()
		}
	} else if f >= r.TimeoutSeconds {
		return ResumeAssessment{}, checkpointError()
	}
	return ResumeAssessment{Eligible: true, RemainingSeconds: r.TimeoutSeconds - f, Record: record}, nil
}
