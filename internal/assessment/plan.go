package assessment

import (
	"context"
	"strconv"
)

// PlanObservation reports reference classification without implying provenance.
type PlanObservation struct {
	Classification string       `json:"classification"`
	Observed       *Observation `json:"observed"`
}

// PlanAsset records an unauthorised candidate action and both observations.
type PlanAsset struct {
	Path      string          `json:"path"`
	Role      string          `json:"role"`
	Action    string          `json:"action"`
	Reason    string          `json:"reason"`
	Reference Observation     `json:"reference"`
	Installed PlanObservation `json:"installed"`
	Source    PlanObservation `json:"source"`
}

// PlanResult is always blocked; local equality supplies neither trust nor ownership.
// docs/adr/0080-go-migration-action-planning.md:91.
type PlanResult struct {
	SchemaVersion        int            `json:"schema_version"`
	ReferenceRevision    string         `json:"reference_revision"`
	Scope                string         `json:"scope"`
	PriorOrigin          string         `json:"prior_origin"`
	SourceAuthentication string         `json:"source_authentication"`
	OwnershipAuthorized  bool           `json:"ownership_authorized"`
	ActivationReady      bool           `json:"activation_ready"`
	RollbackReady        bool           `json:"rollback_ready"`
	Applicable           bool           `json:"applicable"`
	Blockers             []string       `json:"blockers"`
	Assets               []PlanAsset    `json:"assets"`
	Counts               map[string]int `json:"counts"`
}

// Status never authorizes applying a plan, even when all files are unchanged.
func (r PlanResult) Status() int {
	if r.Counts["assessment_error"] != 0 {
		return 1
	}
	return 2
}

// Plan compares only reviewed paths and keeps both roots pinned through validation.
// docs/adr/0080-go-migration-action-planning.md:36.
func Plan(ctx context.Context, root, source string) (PlanResult, error) {
	return plan(ctx, root, source, ops{}, ops{})
}

func plan(ctx context.Context, root, source string, installedOps, sourceOps ops) (PlanResult, error) {
	if err := ctx.Err(); err != nil {
		return PlanResult{}, err
	}
	installedRoot, err := openRoot(ctx, root)
	if err != nil {
		return PlanResult{}, err
	}
	defer installedRoot.close()
	sourceRoot, err := openRoot(ctx, source)
	if err != nil {
		return PlanResult{}, err
	}
	defer sourceRoot.close()
	result := PlanResult{
		SchemaVersion: 1, ReferenceRevision: referenceRevision, Scope: "g2-budget-loop-six",
		PriorOrigin: "unproven", SourceAuthentication: "unverified_local",
		Blockers: []string{"prior_origin_unproven", "target_authentication_unproven", "runtime_qualification_pending", "transition_quiescence_unproven", "verified_recovery_pending"},
		Assets:   make([]PlanAsset, 0, len(catalog)), Counts: map[string]int{
			"retain": 0, "replace_candidate": 0, "add_candidate": 0, "retire_candidate": 0, "preserve_customized": 0, "absent": 0, "conflict": 0, "assessment_error": 0,
		}}
	for _, reference := range catalog {
		if err := ctx.Err(); err != nil {
			return PlanResult{}, err
		}
		installed, err := observe(ctx, installedRoot.last(), reference, installedOps)
		if err != nil {
			return PlanResult{}, err
		}
		proposed, err := observe(ctx, sourceRoot.last(), reference, sourceOps)
		if err != nil {
			return PlanResult{}, err
		}
		asset, err := plannedAsset(reference, installed, proposed)
		if err != nil {
			return PlanResult{}, err
		}
		result.Assets = append(result.Assets, asset)
		result.Counts[asset.Action]++
	}
	if err := ctx.Err(); err != nil {
		return PlanResult{}, err
	}
	if !installedRoot.valid() || !sourceRoot.valid() {
		return PlanResult{}, failure(2, "unsafe assessment root")
	}
	return result, nil
}

// Reviewed intent is independent of arbitrary target omissions.
// docs/adr/0080-go-migration-action-planning.md:53.
var transitionRoles = map[string]string{
	"scripts/factory-budget.sh":      "compatibility_adapter",
	"scripts/factory-loop.sh":        "compatibility_adapter",
	"scripts/lib/budget-config.sh":   "compatibility_adapter",
	"scripts/lib/budget.py":          "legacy_implementation",
	"scripts/lib/budget_adapters.py": "legacy_implementation",
	"scripts/lib/loop.py":            "legacy_implementation",
}

// Action precedence preserves installed customization before considering target equality.
// docs/adr/0080-go-migration-action-planning.md:64.
func plannedAsset(reference referenceAsset, installed, source Asset) (PlanAsset, error) {
	role, known := transitionRoles[reference.Path]
	if !known {
		return PlanAsset{}, failure(1, "cannot plan reference action")
	}
	expectedMode, err := strconv.ParseUint(reference.Reference.Mode, 8, 32)
	if err != nil {
		return PlanAsset{}, failure(1, "cannot plan reference action")
	}
	result := PlanAsset{Path: reference.Path, Role: role, Reference: reference.Reference,
		Installed: PlanObservation{Classification: installed.Classification, Observed: installed.Observed},
		Source:    PlanObservation{Classification: source.Classification, Observed: source.Observed}}
	switch {
	case installed.Classification == "assessment_error" || source.Classification == "assessment_error":
		result.Action, result.Reason = "assessment_error", "cannot_assess_path"
	case installed.Classification == "unsafe" || source.Classification == "unsafe" || source.Observed != nil && source.fullMode != uint32(expectedMode):
		result.Action, result.Reason = "conflict", "unsafe_path_or_source_mode"
	case installed.Classification == "customized":
		result.Action, result.Reason = "preserve_customized", "installed_customization"
	case source.Classification == "missing" && role == "compatibility_adapter":
		result.Action, result.Reason = "conflict", "required_compatibility_path_missing"
	case source.Classification == "missing" && installed.Classification == "missing":
		result.Action, result.Reason = "absent", "absent_in_both"
	case source.Classification == "missing":
		result.Action, result.Reason = "retire_candidate", "explicit_legacy_retirement"
	case installed.Classification == "missing":
		result.Action, result.Reason = "add_candidate", "missing_installed_path"
	case installed.Observed != nil && source.Observed != nil && *installed.Observed == *source.Observed && installed.fullMode == source.fullMode:
		result.Action, result.Reason = "retain", "unchanged_target"
	default:
		result.Action, result.Reason = "replace_candidate", "target_differs"
	}
	return result, nil
}
