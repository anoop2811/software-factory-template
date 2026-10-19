package installation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/transition"
	"golang.org/x/mod/semver"
)

func retentionAdvances(request Request) (bool, string) {
	switch {
	case request.Rollback != "" || request.Recover != "":
		return false, "rollback or recovery does not advance release retention"
	case request.Source.Local || request.Before.Local:
		return false, "local builds do not advance release retention"
	case request.Profile != "go-hybrid-v1":
		return false, "initial migration keeps its predecessor recovery"
	case !semver.IsValid(request.Before.Version) || !semver.IsValid(request.Source.Version):
		return false, "incomparable release tags do not advance retention"
	case semver.Compare(request.Source.Version, request.Before.Version) <= 0:
		return false, "same or earlier release does not advance retention"
	}
	return true, "successful later forward release"
}

func planRetention(ctx context.Context, root string, request Request, images *retentionContext) (result retentionPlan, returned error) {
	result = retentionPlan{Report: newRetentionReport("retention-preview"), Sets: []retentionSnapshot{}}
	advance, reason := retentionAdvances(request)
	result.Report.Advanced = advance
	tree, err := installationfs.Open(ctx, root)
	if err != nil {
		return result, err
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	ids, err := retentionIDs(ctx, tree)
	if err != nil {
		return result, err
	}
	result.Progress, _, err = observe(ctx, tree, retentionProgressPath, journalLimit)
	if err != nil {
		return result, err
	}
	if result.Progress != nil {
		progress, err := readRetentionProgress(ctx, tree)
		if err != nil {
			result.Pending = "unrecognized"
		} else if progress.Phase != "completed" {
			result.Pending = progress.Snapshot.ID
		}
	}
	hints := []ReleaseIdentity{}
	for hint := range images.options {
		hints = append(hints, hint)
	}
	good := map[string]bool{}
	for _, id := range ids {
		snapshot, err := snapshotRetentionSet(ctx, tree, id)
		if err != nil {
			inspectUnknownRetention(ctx, tree, &snapshot)
			result.Sets = append(result.Sets, snapshot)
			result.Report.Sets = append(result.Report.Sets, unknownRetentionSet(id, retentionReason(err)))
			continue
		}
		good[id] = true
		if hint, ok := markerHint(ctx, tree, snapshot); ok {
			snapshot.BeforeHint = hint
			hints = append(hints, hint)
		}
		result.Sets = append(result.Sets, snapshot)
		if snapshot.Receipt != nil {
			hints = append(hints, snapshot.Receipt.After)
		}
		result.Report.Sets = append(result.Report.Sets, reportSet(snapshot, "retain", reason))
	}
	for i, snapshot := range result.Sets {
		if !good[snapshot.ID] {
			continue
		}
		row := &result.Report.Sets[i]
		switch {
		case snapshot.Hold != nil:
			row.Reason = "held for reference"
			row.NextAction = "To request qualified older-set cleanup, run factory upgrade --prune-backup " + snapshot.ID
			continue
		case result.Pending != "":
			row.Reason = "another retention cleanup is incomplete"
			continue
		case !advance:
			continue
		case snapshot.Record.Direction != "forward" || snapshot.Record.Purpose != "upgrade":
			row.Reason = "rollback or recovered history is preserved"
			continue
		}
		target, ok := afterHint(snapshot, hints)
		if !ok {
			row.Reason = "backup target release cannot be independently identified"
			continue
		}
		if !semver.IsValid(target.Version) || semver.Compare(target.Version, request.Before.Version) > 0 {
			row.Reason = "backup is not older than the selected predecessor"
			continue
		}
		if err = qualifyRetentionSet(ctx, tree, snapshot, hints, request.Inputs, images, false, false); err != nil {
			row.Reason = retentionReason(err)
			continue
		}
		if err = retentionIgnored(ctx, root, snapshot); err != nil {
			row.Reason = retentionReason(err)
			continue
		}
		result.Sets[i].AfterHint = target
		row.Action = "prune"
		row.Reason = "eligible older recovery after the new predecessor backup is qualified"
		row.NextAction = "Removed only after successful activation and an independent predecessor recovery proof."
	}
	if result.Pending != "" {
		result.Report.Status = "incomplete"
	}
	result.Report.totals()
	return result, tree.Check(ctx)
}
func bindRetention(planned *plan, retention retentionPlan) error {
	data, err := json.Marshal(struct {
		Installation string
		Retention    retentionPlan
	}{planned.proposal.ProposalDigest, retention})
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	planned.proposal.ProposalDigest = hex.EncodeToString(sum[:])
	return nil
}

// ProposeRetaining binds exact existing recovery observations to consent.
// docs/adr/0106-go-backup-retention.md:58.
func ProposeRetaining(ctx context.Context, root string, request Request, resolver RetentionResolver) (result RetentionProposal, returned error) {
	if request.Rollback != "" || request.Recover != "" {
		proposal, err := Propose(ctx, root, request)
		report := newRetentionReport("retention-preview")
		report.Status = "planned"
		return RetentionProposal{proposal, report}, err
	}
	lease, err := transition.ReadOnly(ctx, root)
	if err != nil {
		return result, err
	}
	defer func() { returned = errors.Join(returned, lease.Close(context.WithoutCancel(ctx))) }()
	images := newRetentionContext(request, resolver)
	defer func() { returned = errors.Join(returned, images.close()) }()
	planned, err := buildPlan(ctx, root, request)
	if err != nil {
		return result, err
	}
	retention, err := planRetention(ctx, root, request, images)
	if err != nil {
		return result, err
	}
	if err = bindRetention(&planned, retention); err != nil {
		return result, err
	}
	return RetentionProposal{planned.proposal, retention.Report}, lease.Check(ctx)
}

func writeRetentionReceipt(ctx context.Context, owner *transaction) error {
	saved := map[string]*Image{}
	for _, entry := range owner.record.Entries {
		if entry.Action != "replace" && entry.Action != "retire" {
			continue
		}
		image, _, err := observe(ctx, owner.tree, backupBase(owner.record.MigrationID)+"/assets/"+entry.Path, leafLimit(entry.Path))
		if err != nil {
			return err
		}
		if image == nil {
			return conflict("retention receipt is missing a checked saved copy")
		}
		saved[entry.Path] = image
	}
	manifest, _, err := observe(ctx, owner.tree, backupBase(owner.record.MigrationID)+"/installation.manifest", journalLimit)
	if err != nil {
		return err
	}
	before := releaseIdentity(owner.request.Before)
	if owner.request.Profile != "go-hybrid-v1" {
		before = ReleaseIdentity{owner.request.Profile, owner.planned.reference.Revision, owner.request.Source.Target}
	}
	inputs := map[string]string{}
	for key, value := range owner.request.Inputs {
		inputs[key] = value
	}
	receipt := retentionReceipt{1, "installation-retention", owner.record.MigrationID, owner.request.Profile, before, releaseIdentity(owner.request.Source), inputs, manifest, saved}
	path := backupBase(owner.record.MigrationID) + "/retention.json"
	if err = writeRecord(ctx, owner.tree, path, receipt, nil); err != nil {
		return err
	}
	owner.record.RetentionReceipt, _, err = observe(ctx, owner.tree, path, journalLimit)
	return err
}

func ApplyRetaining(ctx context.Context, root string, request Request, resolver RetentionResolver) (report RetentionReport, returned error) {
	capability, err := beginWithRetention(ctx, root, request, commandOps{}, resolver, true)
	if err != nil {
		report = newRetentionReport("retention-apply")
		report.Status = "incomplete"
		return report, err
	}
	defer func() { returned = errors.Join(returned, capability.Close(context.WithoutCancel(ctx))) }()
	returned = capability.Complete(ctx)
	owner := capability.state
	if owner.idempotent && returned == nil {
		return reportExistingRetention(ctx, owner.tree)
	}
	if owner.retention != nil {
		report = owner.retention.report
	} else {
		report = newRetentionReport("retention-apply")
	}
	report.Mode = "retention-apply"
	if returned != nil {
		report.Status = "incomplete"
		if !owner.completed {
			report.Advanced = false
			for i := range report.Sets {
				if report.Sets[i].Action == "prune" {
					report.Sets[i].Action = "retain"
					report.Sets[i].Reason = "installation did not complete; release retention did not advance"
					report.Sets[i].NextAction = "Resolve the installation failure before requesting another upgrade."
				}
			}
		}
	} else {
		report.Status = "completed"
	}
	report.totals()
	return report, returned
}

// A completed apply replay reports current observations without advancing expiry.
func reportExistingRetention(ctx context.Context, tree *installationfs.Tree) (RetentionReport, error) {
	report := newRetentionReport("retention-apply")
	report.Status = "completed"
	ids, err := retentionIDs(ctx, tree)
	if err != nil {
		return report, err
	}
	for _, id := range ids {
		snapshot, err := snapshotRetentionSet(ctx, tree, id)
		if err != nil {
			report.Sets = append(report.Sets, unknownRetentionSet(id, retentionReason(err)))
			continue
		}
		action, reason := "retain", "completed installation replay does not advance retention"
		if snapshot.Hold != nil {
			action, reason = "held", "held for reference"
		}
		report.Sets = append(report.Sets, reportSet(snapshot, action, reason))
	}
	report.totals()
	progressImage, _, err := observe(ctx, tree, retentionProgressPath, journalLimit)
	if err != nil {
		return report, err
	}
	if progressImage != nil {
		progress, err := readRetentionProgress(ctx, tree)
		if err != nil || progress.Phase != "completed" {
			report.Status = "incomplete"
			return report, &Failure{Code: 1, Reason: "installation completed; previous backup cleanup is incomplete", cause: &retentionCleanupFailure{err}}
		}
	}
	return report, tree.Check(ctx)
}

func (owner *transaction) finishRetention(ctx context.Context) (returned error) {
	operation := owner.retention
	if operation == nil {
		return nil
	}
	defer func() {
		if returned == nil {
			return
		}
		operation.report.Status = "incomplete"
		for i := range operation.report.Sets {
			row := &operation.report.Sets[i]
			if row.Action == "prune" {
				row.Action = "retain"
				row.Reason = retentionReason(returned)
				row.NextAction = "Reconcile the reported proof conflict before retrying factory upgrade --prune-backup " + row.MigrationID
			}
		}
	}()
	operation.report = operation.plan.Report
	operation.report.Mode = "retention-apply"
	operation.report.Status = "completed"
	current, err := snapshotRetentionSet(ctx, owner.tree, owner.record.MigrationID)
	if err != nil {
		return err
	}
	operation.report.Sets = append(operation.report.Sets, reportSet(current, "retain", "immediately preceding installation recovery"))
	operation.report.totals()
	if operation.plan.Pending != "" {
		return &Failure{Code: 1, Reason: "installation completed; previous backup cleanup is incomplete"}
	}
	pruning := false
	for _, row := range operation.report.Sets {
		if row.Action == "prune" {
			pruning = true
			break
		}
	}
	if !operation.report.Advanced || !pruning {
		return nil
	}
	if err = owner.validateProvenTarget(ctx); err != nil {
		return err
	}
	operation.context.authenticated[releaseIdentity(owner.request.Source)] = !owner.request.Source.Local
	if owner.request.Before.Archive != "" {
		operation.context.authenticated[releaseIdentity(owner.request.Before)] = !owner.request.Before.Local
	}
	hints := []ReleaseIdentity{releaseIdentity(owner.request.Source), releaseIdentity(owner.request.Before)}
	for _, snapshot := range operation.plan.Sets {
		if snapshot.Receipt != nil {
			hints = append(hints, snapshot.Receipt.Before, snapshot.Receipt.After)
		}
	}
	if err = qualifyRetentionSet(ctx, owner.tree, current, hints, owner.request.Inputs, operation.context, true, true); err != nil {
		return err
	}
	for i := range operation.plan.Sets {
		row := &operation.report.Sets[i]
		if row.Action != "prune" {
			continue
		}
		snapshot, err := snapshotRetentionSet(ctx, owner.tree, row.MigrationID)
		if err != nil {
			return err
		}
		snapshot.BeforeHint = operation.plan.Sets[i].BeforeHint
		snapshot.AfterHint = operation.plan.Sets[i].AfterHint
		before, _ := json.Marshal(operation.plan.Sets[i])
		after, _ := json.Marshal(snapshot)
		if string(before) != string(after) {
			return conflict("backup changed after confirmed retention preview")
		}
		if err = qualifyRetentionSet(ctx, owner.tree, snapshot, hints, owner.request.Inputs, operation.context, true, false); err != nil {
			return err
		}
		if err = owner.validateProvenTarget(ctx); err != nil {
			return err
		}
		if err = retentionIgnored(ctx, owner.root, snapshot); err != nil {
			return err
		}
		files, size, err := executeRetentionPrune(ctx, owner.tree, snapshot, owner.record.MigrationID, retentionPruneOps{})
		operation.report.RemovedFiles += files
		operation.report.RemovedBytes += size
		if err != nil {
			row.Action = "incomplete"
			row.Files, row.Bytes = nil, nil
			row.Reason = "installation is healthy; older backup cleanup is incomplete"
			row.NextAction = "Retry factory upgrade --prune-backup " + row.MigrationID
			return &Failure{Code: 1, Reason: row.Reason, cause: err}
		}
		row.Action = "removed"
		row.Reason = "qualified older recovery removed after successful release"
		row.NextAction = "The immediately preceding usable installation recovery was preserved."
		operation.report.RemovedSets++
	}
	operation.report.totals()
	return owner.check(ctx)
}

func RetentionDiagnosticReport(report RetentionReport) string {
	data, err := json.Marshal(report)
	if err != nil {
		return "factory retention: report unavailable\n"
	}
	return fmt.Sprintf("factory retention: %s\n", data)
}
