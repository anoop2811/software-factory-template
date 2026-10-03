package recoverycmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/assessment"
	"github.com/anoop2811/software-factory-template/internal/output"
)

func runPlanning(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	id, jsonOutput, ok := parsePlanning(args)
	if !ok {
		return diagnostic(ctx, stderr, "invalid recovery planning arguments", 2)
	}
	root, err := currentInstallation(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return diagnostic(ctx, stderr, planningMessage(ctx.Err()), 1)
		}
		return diagnostic(ctx, stderr, "cannot resolve current installation", 1)
	}
	plan, err := assessment.PlanRecovery(ctx, root, id)
	if err != nil {
		return diagnostic(ctx, stderr, planningMessage(err), assessment.ErrorStatus(err))
	}
	var data []byte
	if jsonOutput {
		data, err = json.Marshal(plan)
		data = append(data, '\n')
	} else {
		data, err = renderPlanning(plan)
	}
	if err == nil {
		err = output.WriteEvent(ctx, stdout, data)
	}
	if err != nil {
		return diagnostic(ctx, stderr, "cannot write recovery plan", 1)
	}
	return plan.Status()
}

func parsePlanning(args []string) (string, bool, bool) {
	id := ""
	seen := make(map[string]bool)
	for i := 0; i < len(args); i++ {
		name, value, attached := strings.Cut(args[i], "=")
		if seen[name] {
			return "", false, false
		}
		seen[name] = true
		switch name {
		case "--plan-restore", "--json":
			if attached {
				return "", false, false
			}
		case "--migration-id":
			if !attached {
				i++
				if i == len(args) || strings.HasPrefix(args[i], "-") {
					return "", false, false
				}
				value = args[i]
			}
			if value == "" {
				return "", false, false
			}
			id = value
		default:
			return "", false, false
		}
	}
	return id, seen["--json"], seen["--plan-restore"] && seen["--migration-id"]
}

// All 17 text fields retain the same complete data as their JSON counterparts.
// docs/adr/0092-go-recovery-restoration-planning.md:74.
func renderPlanning(plan assessment.RecoveryPlan) ([]byte, error) {
	values := []any{plan.Recovery, plan.Assets, plan.Counts, plan.Blockers}
	nested := make([][]byte, len(values))
	for i, value := range values {
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		nested[i] = data
	}
	return []byte(fmt.Sprintf("schema_version=%d\nmode=%s\ncoverage=%s\nscope=%s\nmigration_id=%s\nsource_revision=%s\ntarget_revision=%s\ntarget_authentication=%s\nrecovery=%s\nassets=%s\ncounts=%s\nblockers=%s\nrestorable=%t\nrollback_ready=%t\nactivation_ready=%t\napplicable=%t\nprune_authorized=%t\n", plan.SchemaVersion, plan.Mode, plan.Coverage, plan.Scope, plan.MigrationID, plan.SourceRevision, plan.TargetRevision, plan.TargetAuthentication, nested[0], nested[1], nested[2], nested[3], plan.Restorable, plan.RollbackReady, plan.ActivationReady, plan.Applicable, plan.PruneAuthorized)), nil
}

func planningMessage(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "recovery planning canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "recovery planning deadline exceeded"
	}
	var failure *assessment.Failure
	if errors.As(err, &failure) {
		return failure.Error()
	}
	return "cannot assess recovery plan"
}
