package budget

import (
	"context"
	"maps"
	"math/big"
	"slices"
)

// Observe the admitted plan before any reservation write; callbacks cannot edit
// policy through shared pointers. docs/adr/0071-go-budget-command-candidate.md:65.
func observePlan(ctx context.Context, observer func(Plan) error, plan Plan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if observer == nil {
		return nil
	}
	configuration, err := validateConfig(plan.Configuration)
	if err != nil {
		return err
	}
	plan.Configuration = configuration
	plan.RemainingAttempts = new(big.Int).Set(plan.RemainingAttempts)
	plan.RemainingSessionRuns = new(big.Int).Set(plan.RemainingSessionRuns)
	plan.Services = maps.Clone(plan.Services)
	plan.Blockers = slices.Clone(plan.Blockers)
	plan.Warnings = slices.Clone(plan.Warnings)
	if err := observer(plan); err != nil {
		return err
	}
	return ctx.Err()
}
