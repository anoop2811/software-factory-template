package installation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/staging"
)

// Discard only this live owner's resolved failed preparation, never imported evidence.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:411.
func (owner *transaction) discardPreparation(ctx context.Context) error {
	if !owner.newJournal || owner.journalImage == nil || owner.pending != nil {
		return nil
	}
	if len(owner.record.Checks) == 0 {
		return nil
	}
	for _, check := range owner.record.Checks {
		if check.Phase != "complete" || check.OwnershipUnconfirmed || (check.Name == "candidate-help" || check.Name == "native-gate-proof") && check.PID == nil {
			return nil
		}
	}
	for _, entry := range owner.record.Entries {
		if entry.Phase != "prepared" || entry.ObservedAfter != nil {
			return nil
		}
	}
	if err := owner.check(ctx); err != nil {
		return &Uncertainty{Operation: "preparation cleanup", cause: err}
	}
	path := journalPath(operationID(owner.request))
	current, _, err := observe(ctx, owner.tree, path, journalLimit)
	if err != nil || !imageEqual(current, owner.journalImage, true) {
		return &Uncertainty{Operation: "preparation cleanup identity", cause: errors.Join(err, conflict("owned preparation journal changed"))}
	}
	parent, name, _, err := owner.tree.Parent(ctx, path, false)
	if err != nil {
		return &Uncertainty{Operation: "preparation cleanup parent", cause: err}
	}
	if err := parent.Root.Remove(name); err != nil {
		return &Uncertainty{Operation: "preparation cleanup unlink", cause: err}
	}
	if err := parent.Sync(ctx); err != nil {
		return &Uncertainty{Operation: "preparation cleanup durability", cause: err}
	}
	absent, _, err := observe(ctx, owner.tree, path, journalLimit)
	if err != nil || absent != nil {
		return &Uncertainty{Operation: "preparation cleanup absence", cause: errors.Join(err, conflict("preparation journal cleanup is uncertain"))}
	}
	owner.journalImage = nil
	return nil
}

func preparationOnly(record journal) bool {
	if record.Pending != nil || (record.Phase != "prepared" && record.Phase != "checking") {
		return false
	}
	for _, entry := range record.Entries {
		if entry.Phase != "prepared" || entry.ObservedAfter != nil {
			return false
		}
	}
	return true
}
func buildPreparationPlan(ctx context.Context, root string, request Request, record journal, tree *installationfs.Tree, image staging.Inspection, reference baseline) (result plan, returned error) {
	if request.Recover == "" || !preparationOnly(record) {
		return result, conflict("installation is not unchanged preparation")
	}
	result = plan{image: image, request: request, payload: map[string][]byte{}, before: map[string][]byte{}, proposal: Proposal{SchemaVersion: 1, Mode: "installation-proposal", Profile: request.Profile, Target: request.Source.Target, Authentication: "not-verified", Actions: []Action{}, Checks: []string{"candidate-help", "installed-assets", "native-gate-proof", "state-compatibility"}, Conflicts: []Issue{}, Blockers: []Issue{}, RollbackBlockers: []Issue{}}}
	if request.Source.Local {
		result.proposal.Authentication = "local-source"
	}
	if len(record.Entries) != len(roles) {
		return result, conflict("preparation journal omits complete selection")
	}
	for _, row := range roles {
		var saved *journalEntry
		for index := range record.Entries {
			if record.Entries[index].Path == row.Path {
				saved = &record.Entries[index]
			}
		}
		if saved == nil {
			return result, conflict("preparation journal omits selected path")
		}
		current, _, err := observe(ctx, tree, row.Path, leafLimit(row.Path))
		if err != nil {
			return result, err
		}
		reason := "freshly qualified unchanged preparation; no active mutation"
		if !imageEqual(current, saved.Before, true) {
			issue := Issue{row.Path, "preparation before selection changed; preserved"}
			result.proposal.Conflicts = append(result.proposal.Conflicts, issue)
			result.proposal.Blockers = append(result.proposal.Blockers, issue)
			reason = issue.Reason
		}
		if saved.Before != nil && saved.Action != "retain" {
			raw, mode, known, err := reference.original(ctx, row.Path, request.Inputs)
			if err != nil {
				return result, err
			}
			if !known || !imageEqual(saved.Before, bytesImage(raw, mode), false) {
				return result, conflict("preparation before selection is not qualified baseline")
			}
		}
		result.proposal.Actions = append(result.proposal.Actions, Action{Path: row.Path, SourcePath: row.Source, Action: "retain", Reason: reason, Required: row.Required, Before: current, After: current})
	}
	for _, check := range record.Checks {
		if check.OwnershipUnconfirmed || check.Phase == "checking" {
			result.proposal.Blockers = append(result.proposal.Blockers, Issue{journalPath(record.MigrationID), "unknown candidate group ownership remains; operator reconciliation required"})
		}
	}
	if request.Direction != "reverse" {
		result.proposal.Blockers = append(result.proposal.Blockers, Issue{journalPath(record.MigrationID), "unchanged preparation requires manual reverse abandonment; no automatic candidate restart"})
	}
	state, err := inspectState(ctx, tree)
	if err != nil {
		return result, err
	}
	encoded, err := json.Marshal(struct {
		Root                   string
		Source                 staging.Options
		Archive                string
		Profile, ID, Direction string
		Inputs                 map[string]string
		Entries                []Action
		State                  []stateObservation
	}{root, request.Source, image.ArchiveSHA256, request.Profile, record.MigrationID, request.Direction, request.Inputs, result.proposal.Actions, state})
	if err != nil {
		return result, err
	}
	sum := sha256.Sum256(encoded)
	result.proposal.ProposalDigest = hex.EncodeToString(sum[:])
	result.proposal.RecoveryCommand = "factory upgrade --installation --recover " + record.MigrationID + " --direction reverse --dry-run"
	result.proposal.RecoveryArguments = []string{"factory", "upgrade", "--installation", "--recover", record.MigrationID, "--direction", "reverse", "--dry-run", "--source", request.Source.Archive, "--profile", request.Profile, "--version", request.Source.Version, "--revision", request.Source.Revision, "--target", request.Source.Target}
	return result, tree.Check(ctx)
}
func (owner *transaction) abandonPreparation(ctx context.Context) error {
	if owner.request.Direction != "reverse" || !preparationOnly(owner.record) {
		return conflict("preparation requires manual reverse abandonment")
	}
	for _, check := range owner.record.Checks {
		if check.OwnershipUnconfirmed || check.Phase == "checking" {
			return conflict("unknown candidate group ownership requires reconciliation")
		}
	}
	owner.newJournal = true
	if err := owner.discardPreparation(ctx); err != nil {
		return err
	}
	if owner.journalImage != nil {
		return conflict("preparation journal abandonment was not established")
	}
	owner.completed = true
	return owner.check(ctx)
}
func (owner *transaction) completePendingCleanup(ctx context.Context) error {
	if !owner.terminalCleanup || owner.pending == nil || owner.record.Pending == nil || owner.request.Direction != owner.record.Direction {
		return conflict("terminal pending cleanup grant unavailable")
	}
	if err := owner.check(ctx); err != nil {
		return err
	}
	for _, action := range owner.planned.proposal.Actions {
		current, _, err := observe(ctx, owner.tree, action.Path, leafLimit(action.Path))
		if err != nil {
			return err
		}
		if !imageEqual(current, action.Before, true) {
			return conflict("terminal installation entry changed after consent")
		}
	}
	if _, err := inspectState(ctx, owner.tree); err != nil {
		return err
	}
	pending, _, err := observe(ctx, owner.tree, ".factory/runtime-publication.pending", 0)
	if err != nil {
		return err
	}
	if !imageEqual(pending, owner.record.Pending, true) {
		return conflict("terminal pending entry changed after consent")
	}
	if err := owner.pending.Check(ctx); err != nil {
		return err
	}
	parent, name := owner.pending.Parent, owner.pending.Name
	if err := parent.Root.Remove(name); err != nil {
		return &Uncertainty{Operation: "terminal pending cleanup", cause: err}
	}
	if err := parent.Sync(ctx); err != nil {
		return &Uncertainty{Operation: "terminal pending cleanup durability", cause: err}
	}
	absence, _, err := observe(ctx, owner.tree, ".factory/runtime-publication.pending", 0)
	if err != nil || absence != nil {
		return &Uncertainty{Operation: "terminal pending cleanup absence", cause: errors.Join(err, conflict("terminal pending absence could not be established"))}
	}
	if err := owner.pending.Close(); err != nil {
		return &Uncertainty{Operation: "terminal pending checked closure", cause: err}
	}
	owner.pending = nil
	owner.completed = true
	return owner.check(ctx)
}
