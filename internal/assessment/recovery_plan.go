package assessment

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// RecoveryPlanAsset reports a reference action without installation ownership.
type RecoveryPlanAsset struct {
	Path      string          `json:"path"`
	Action    string          `json:"action"`
	Reason    string          `json:"reason"`
	Reference Observation     `json:"reference"`
	Installed PlanObservation `json:"installed"`
}

// RecoveryPlan grants no restoration, activation or retention authority.
// docs/adr/0092-go-recovery-restoration-planning.md:74.
type RecoveryPlan struct {
	SchemaVersion        int                 `json:"schema_version"`
	Mode                 string              `json:"mode"`
	Coverage             string              `json:"coverage"`
	Scope                string              `json:"scope"`
	MigrationID          string              `json:"migration_id"`
	SourceRevision       string              `json:"source_revision"`
	TargetRevision       string              `json:"target_revision"`
	TargetAuthentication string              `json:"target_authentication"`
	Recovery             RecoverySet         `json:"recovery"`
	Assets               []RecoveryPlanAsset `json:"assets"`
	Counts               map[string]int      `json:"counts"`
	Blockers             []string            `json:"blockers"`
	Restorable           bool                `json:"restorable"`
	RollbackReady        bool                `json:"rollback_ready"`
	ActivationReady      bool                `json:"activation_ready"`
	Applicable           bool                `json:"applicable"`
	PruneAuthorized      bool                `json:"prune_authorized"`
}

// Status keeps every complete plan ineligible and gives assessment errors priority.
func (p RecoveryPlan) Status() int {
	if p.Recovery.Classification == "assessment_error" || p.Counts["assessment_error"] != 0 {
		return 1
	}
	return 2
}

// PlanRecovery inspects exactly one existing set without writes or runtime probes.
// docs/adr/0092-go-recovery-restoration-planning.md:44.
func PlanRecovery(ctx context.Context, root, id string) (RecoveryPlan, error) {
	return planRecovery(ctx, root, id, ops{})
}

func planRecovery(ctx context.Context, root, id string, operations ops) (result RecoveryPlan, returned error) {
	if !recoveryID(id) {
		return RecoveryPlan{}, failure(2, "invalid recovery planning identifier")
	}
	if err := ctx.Err(); err != nil {
		return RecoveryPlan{}, err
	}
	chain, err := openRoot(ctx, root)
	if err != nil {
		return RecoveryPlan{}, err
	}
	var pins recoveryPins
	defer func() {
		if err := ctx.Err(); err != nil {
			result, returned = RecoveryPlan{}, err
		} else if !recoveryRootValid(chain) {
			// Preserve an earlier assessment failure across a late root change.
			// docs/adr/0092-go-recovery-restoration-planning.md:135.
			result, returned = RecoveryPlan{}, failure(result.Status(), "unsafe or changed recovery planning root")
		} else if returned == nil && !pins.valid() {
			// Changing installed observations says nothing about saved-set safety.
			// docs/adr/0092-go-recovery-restoration-planning.md:133.
			status := result.Status()
			result, returned = RecoveryPlan{}, failure(status, "recovery planning observations changed")
		}
		if returned == nil {
			result.addBlockers()
		}
		if err := closeRecoveryPlan(operations, pins, chain); err != nil {
			result = RecoveryPlan{}
			// Close failures remain operational even after a prior refusal.
			returned = errors.Join(err, returned)
		}
	}()
	if !trustedDirectory(chain[len(chain)-1].identity, false) {
		return RecoveryPlan{}, failure(2, "unsafe recovery planning root")
	}
	result = newRecoveryPlan(id, recoveryRow(id, "missing", nil))
	parent := chain.last()
	for i, name := range []string{".factory", "backups"} {
		pin, class := openRecoveryDirectory(ctx, parent, name, i == 0, operations)
		if pin.file != nil {
			pins = append(pins, pin)
		}
		if err := ctx.Err(); err != nil {
			return RecoveryPlan{}, err
		}
		if class != "" {
			if class == "missing" {
				pins = append(pins, recoveryPin{parent: parent, name: name, missing: true})
			}
			result.Recovery = recoveryRow(id, class, nil)
			return result, nil
		}
		parent = pin.file
	}
	if _, err := named(parent, id); errors.Is(err, unix.ENOENT) {
		pins = append(pins, recoveryPin{parent: parent, name: id, missing: true})
		return result, nil
	}
	var manifest recoveryManifest
	result.Recovery, manifest, err = inspectRecoverySetPinned(ctx, parent, id, operations, &pins)
	if err != nil {
		return RecoveryPlan{}, err
	}
	if result.Recovery.Classification != "integrity_checked" {
		return result, nil
	}
	result.SourceRevision = referenceRevision
	result.TargetRevision = manifest.target
	result.TargetAuthentication = "operator_metadata"
	for _, reference := range manifest.assets {
		installed, err := observePinned(ctx, chain.last(), reference, operations, &pins)
		if err != nil {
			return RecoveryPlan{}, err
		}
		action := recoveryPlanAction(installed.Classification)
		result.Assets = append(result.Assets, RecoveryPlanAsset{
			Path: reference.Path, Action: action, Reason: installed.Reason,
			Reference: reference.Reference, Installed: PlanObservation{Classification: installed.Classification, Observed: installed.Observed},
		})
		result.Counts[action]++
	}
	return result, nil
}

func newRecoveryPlan(id string, recovery RecoverySet) RecoveryPlan {
	return RecoveryPlan{
		SchemaVersion: 1, Mode: "restore_plan", Coverage: "partial", Scope: "g2-budget-loop-six",
		MigrationID: id, TargetAuthentication: "unavailable", Recovery: recovery,
		Assets: []RecoveryPlanAsset{}, Counts: map[string]int{
			"retain_reference": 0, "restore_missing_candidate": 0, "preserve_customized": 0, "conflict": 0, "assessment_error": 0,
		}, Blockers: []string{"migration_ownership_unproven", "transition_quiescence_unproven", "runtime_compatibility_unproven", "activation_checks_pending"},
	}
}

func (p *RecoveryPlan) addBlockers() {
	if p.Recovery.Classification != "integrity_checked" {
		p.Blockers = append(p.Blockers, "recovery_not_integrity_checked")
	}
	if p.Recovery.Held != nil && *p.Recovery.Held {
		p.Blockers = append(p.Blockers, "recovery_held")
	}
	if p.Counts["conflict"]+p.Counts["preserve_customized"] != 0 {
		p.Blockers = append(p.Blockers, "installed_conflicts")
	}
	if p.Status() == 1 {
		p.Blockers = append(p.Blockers, "assessment_failed")
	}
}

func recoveryPlanAction(class string) string {
	switch class {
	case "matching_reference":
		return "retain_reference"
	case "missing":
		return "restore_missing_candidate"
	case "customized":
		return "preserve_customized"
	case "assessment_error":
		return "assessment_error"
	default:
		return "conflict"
	}
}

func closeRecoveryPlan(operations ops, pins recoveryPins, chain directories) error {
	closeFile := (*os.File).Close
	if operations.close != nil {
		closeFile = operations.close
	}
	var errs []error
	for i := len(pins) - 1; i >= 0; i-- {
		if pins[i].file != nil {
			errs = append(errs, closeFile(pins[i].file))
		}
	}
	for i := len(chain) - 1; i >= 0; i-- {
		errs = append(errs, closeFile(chain[i].file))
	}
	if errors.Join(errs...) != nil {
		return failure(1, "cannot close recovery planning observations")
	}
	return nil
}
