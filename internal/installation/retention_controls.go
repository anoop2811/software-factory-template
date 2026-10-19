package installation

import (
	"context"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/installedlayout"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/transition"
	"golang.org/x/mod/semver"
)

func retentionIgnored(ctx context.Context, root string, snapshot retentionSnapshot) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	env := operationEnvironment()
	env["GIT_CONFIG_NOSYSTEM"], env["GIT_CONFIG_GLOBAL"] = "1", os.DevNull
	tracked, err := native.ExecuteCommandBounded(ctx, root, []string{"git", "ls-files", "-z", "--", backupBase(snapshot.ID), journalPath(snapshot.ID), retentionProgressPath}, env, 64<<10, 4096)
	if err != nil || tracked.ExitCode == nil || *tracked.ExitCode != 0 || len(tracked.Stdout) != 0 {
		return errors.Join(conflict("backup or retention evidence is tracked or cannot be inspected"), err)
	}
	paths := []string{backupBase(snapshot.ID) + "/hold.json", retentionProgressPath, journalPath(snapshot.ID)}
	for name := range snapshot.Files {
		paths = append(paths, backupBase(snapshot.ID)+"/"+name)
	}
	sort.Strings(paths)
	// Git requires --stdin with -z; retain literal NUL-separated paths.
	// https://git-scm.com/docs/git-check-ignore (fetched 2026-10-19).
	args := []string{"git", "check-ignore", "--no-index", "-z", "--stdin"}
	ignored, err := native.ExecuteCommandWithOptions(ctx, root, args, env, native.CommandOptions{Input: strings.Join(paths, "\x00") + "\x00", StdoutLimit: 512 << 10, StderrLimit: 4096})
	if err != nil || ignored.ExitCode == nil || *ignored.ExitCode != 0 {
		return errors.Join(conflict("backup paths are not effectively ignored"), err)
	}
	got := strings.Split(strings.TrimSuffix(string(ignored.Stdout), "\x00"), "\x00")
	sort.Strings(got)
	if len(got) != len(paths) {
		return conflict("backup paths are not effectively ignored")
	}
	for i, path := range paths {
		if got[i] != path {
			return conflict("backup paths are not effectively ignored")
		}
	}
	return ctx.Err()
}
func noActiveRetentionTransactions(ctx context.Context, tree *installationfs.Tree) (returned error) {
	node, _, _, err := tree.Parent(ctx, ".factory/installation-transactions/.probe", false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	directory, err := node.Root.Open(".")
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, directory.Close()) }()
	entries, err := directory.ReadDir(17)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(entries) > 16 {
		return conflict("installation transaction inventory exceeds its limit")
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			return conflict("unrecognized installation transaction must be reconciled")
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !ValidBackupID(id) {
			return conflict("unrecognized installation transaction must be reconciled")
		}
		record, err := readJournalEvidence(ctx, tree, id, false)
		if err != nil {
			return conflict("unresolved installation transaction prevents backup cleanup")
		}
		if record.Phase != "committed" || record.Outcome != "completed" {
			return conflict("active installation transaction prevents backup cleanup")
		}
	}
	return tree.Check(ctx)
}

// KeepBackup adds only an inert protective hold; it grants no restoration right.
// docs/adr/0106-go-backup-retention.md:63.
func KeepBackup(ctx context.Context, root, id string) (report RetentionReport, returned error) {
	report = newRetentionReport("keep-backup")
	if !ValidBackupID(id) {
		return report, conflict("invalid backup identifier")
	}
	lease, err := transition.RootExclusive(ctx, root)
	if err != nil {
		return report, err
	}
	defer func() { returned = errors.Join(returned, lease.Close(context.WithoutCancel(ctx))) }()
	guard, err := transition.Exclusive(ctx, root)
	if err != nil {
		return report, err
	}
	defer func() { returned = errors.Join(returned, guard.Close(context.WithoutCancel(ctx), true)) }()
	tree, err := installationfs.Open(ctx, root)
	if err != nil {
		return report, err
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	if err = noActiveRetentionTransactions(ctx, tree); err != nil {
		return report, err
	}
	existing, _, err := observe(ctx, tree, retentionProgressPath, journalLimit)
	if err != nil {
		return report, err
	}
	if existing != nil {
		progress, err := readRetentionProgress(ctx, tree)
		if err != nil {
			return report, err
		}
		if progress.Phase != "completed" {
			return report, conflict("incomplete pruning must be reconciled before adding a hold")
		}
	}
	snapshot, err := snapshotRetentionSet(ctx, tree, id)
	if err != nil {
		return report, err
	}
	if err = retentionIgnored(ctx, root, snapshot); err != nil {
		return report, err
	}
	if snapshot.Hold == nil {
		if err = guard.Check(ctx); err != nil {
			return report, err
		}
		if err = lease.Check(ctx); err != nil {
			return report, err
		}
		hold := retentionHold{1, "backup-hold", id}
		if err = writeRecord(ctx, tree, backupBase(id)+"/hold.json", hold, nil); err != nil {
			return report, err
		}
		snapshot, err = snapshotRetentionSet(ctx, tree, id)
		if err != nil {
			return report, err
		}
	}
	report.Sets = append(report.Sets, reportSet(snapshot, "held", "held for reference; automatic expiry is disabled"))
	report.Status = "completed"
	report.totals()
	return report, errors.Join(tree.Check(ctx), guard.Check(ctx), lease.Check(ctx))
}

// CurrentReleaseHint is descriptive only; consumers must authenticate and compare
// the complete current projection before granting deletion authority.
func CurrentReleaseHint(ctx context.Context, root string) (hint ReleaseIdentity, returned error) {
	tree, err := installationfs.Open(ctx, root)
	if err != nil {
		return hint, err
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	_, data, err := observe(ctx, tree, ".factory/installation.current", journalLimit)
	if err != nil {
		return hint, err
	}
	descriptor, err := installedlayout.Decode(ctx, data)
	if err != nil {
		return hint, conflict("current release identity is unavailable")
	}
	hint = ReleaseIdentity{descriptor.Version, descriptor.Revision, descriptor.Target}
	if !validRetentionIdentity(hint) {
		return ReleaseIdentity{}, conflict("current release identity is unsupported")
	}
	return hint, tree.Check(ctx)
}

// PruneBackup independently proves a current installation and a complete alternate;
// neither a local completed flag nor a serialized retain row grants authority.
// docs/adr/0106-go-backup-retention.md:48.
func PruneBackup(ctx context.Context, root, id string, resolver RetentionResolver) (report RetentionReport, returned error) {
	report = newRetentionReport("prune-backup")
	if !ValidBackupID(id) {
		return report, conflict("invalid backup identifier")
	}
	lease, err := transition.RootExclusive(ctx, root)
	if err != nil {
		return report, err
	}
	defer func() { returned = errors.Join(returned, lease.Close(context.WithoutCancel(ctx))) }()
	guard, err := transition.Exclusive(ctx, root)
	if err != nil {
		return report, err
	}
	defer func() { returned = errors.Join(returned, guard.Close(context.WithoutCancel(ctx), true)) }()
	tree, err := installationfs.Open(ctx, root)
	if err != nil {
		return report, err
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	if err = noActiveRetentionTransactions(ctx, tree); err != nil {
		return report, err
	}
	progressImage, _, err := observe(ctx, tree, retentionProgressPath, journalLimit)
	if err != nil {
		return report, err
	}
	var progress *retentionProgress
	if progressImage != nil {
		value, err := readRetentionProgress(ctx, tree)
		if err != nil {
			return report, err
		}
		progress = &value
		if value.Snapshot.ID == id && value.Phase == "completed" {
			_, err := tree.Stat(ctx, backupBase(id))
			if err == nil {
				return report, conflict("completed backup identifier is occupied again")
			}
			if !errors.Is(err, os.ErrNotExist) {
				return report, err
			}
			journalImage, _, err := observe(ctx, tree, journalPath(id), journalLimit)
			if err != nil {
				return report, err
			}
			if journalImage != nil {
				return report, conflict("completed backup journal is occupied again")
			}
			report.Status = "completed"
			zero := 0
			bytes := int64(0)
			report.Sets = append(report.Sets, RetentionSet{MigrationID: id, Path: backupBase(id), Action: "already-removed", Reason: "backup and associated journal are already absent", Files: &zero, Bytes: &bytes})
			report.totals()
			return report, nil
		}
		if value.Phase == "pruning" && value.Snapshot.ID != id {
			return report, conflict("another backup cleanup must be reconciled first")
		}
	}
	hint, err := CurrentReleaseHint(ctx, root)
	if err != nil {
		return report, err
	}
	images := newRetentionContext(Request{}, resolver)
	defer func() { returned = errors.Join(returned, images.close()) }()
	current, err := images.image(ctx, hint, true)
	if err != nil {
		return report, err
	}
	currentRequest, err := InferCandidate(ctx, root, images.options[hint], nil)
	if err != nil {
		return report, err
	}
	if err = qualifyCurrentRetention(ctx, tree, current, currentRequest.Inputs); err != nil {
		return report, err
	}
	ids, err := retentionIDs(ctx, tree)
	if err != nil {
		return report, err
	}
	snapshots := []retentionSnapshot{}
	preserved := []RetentionSet{}
	hints := []ReleaseIdentity{hint}
	var selected *retentionSnapshot
	for _, candidate := range ids {
		snapshot, err := snapshotRetentionSet(ctx, tree, candidate)
		if err != nil {
			if candidate != id {
				preserved = append(preserved, unknownRetentionSet(candidate, retentionReason(err)))
			}
			continue
		}
		if before, ok := markerHint(ctx, tree, snapshot); ok {
			snapshot.BeforeHint = before
			hints = append(hints, before)
		}
		if snapshot.Receipt != nil {
			hints = append(hints, snapshot.Receipt.After)
		}
		snapshots = append(snapshots, snapshot)
	}
	for i := range snapshots {
		if snapshots[i].ID == id {
			selected = &snapshots[i]
		}
	}
	if progress != nil && progress.Phase == "pruning" && progress.Snapshot.ID == id {
		resuming := progress.Snapshot
		selected = &resuming
	}
	if selected == nil {
		return report, conflict("selected backup is unavailable or incomplete")
	}
	target, ok := afterHint(*selected, hints)
	if !ok {
		return report, conflict("selected backup release cannot be independently identified")
	}
	selected.AfterHint = target
	if target == hint || semver.IsValid(target.Version) && semver.IsValid(hint.Version) && semver.Compare(target.Version, hint.Version) > 0 {
		return report, conflict("the current or sole immediately preceding recovery is not an eligible older set")
	}
	if err = qualifyRetentionSet(ctx, tree, *selected, hints, currentRequest.Inputs, images, true, false); err != nil {
		return report, err
	}
	if progress != nil && progress.Phase == "pruning" {
		if err = validateRemainingRetention(ctx, tree, *progress); err != nil {
			return report, err
		}
	}
	if err = retentionIgnored(ctx, root, *selected); err != nil {
		return report, err
	}
	protect := ""
	for _, alternate := range snapshots {
		if alternate.ID == id {
			continue
		}
		target, ok := afterHint(alternate, hints)
		if !ok || target != hint {
			continue
		}
		if err = qualifyRetentionSet(ctx, tree, alternate, hints, currentRequest.Inputs, images, true, true); err != nil {
			continue
		}
		if err = retentionIgnored(ctx, root, alternate); err != nil {
			continue
		}
		protect = alternate.ID
		break
	}
	if protect == "" {
		return report, conflict("no independently usable alternate recovery; preserve the sole recovery set")
	}
	if err = qualifyCurrentRetention(ctx, tree, current, currentRequest.Inputs); err != nil {
		return report, err
	}
	if err = guard.Check(ctx); err != nil {
		return report, err
	}
	if err = lease.Check(ctx); err != nil {
		return report, err
	}
	row := reportSet(*selected, "removed", "qualified older recovery removed; another usable recovery is preserved")
	report.Sets = append(report.Sets, row)
	for _, snapshot := range snapshots {
		if snapshot.ID == id {
			continue
		}
		reason := "unselected recovery preserved for reference"
		action := "retain"
		if snapshot.Hold != nil {
			action, reason = "held", "held for reference"
		}
		if snapshot.ID == protect {
			reason = "independently qualified alternate recovery preserved"
		}
		report.Sets = append(report.Sets, reportSet(snapshot, action, reason))
	}
	report.Sets = append(report.Sets, preserved...)
	report.RemovedFiles, report.RemovedBytes, err = executeRetentionPrune(ctx, tree, *selected, protect, retentionPruneOps{})
	if err != nil {
		report.Status = "incomplete"
		report.Sets[0].Action = "incomplete"
		report.Sets[0].Files, report.Sets[0].Bytes = nil, nil
		report.Sets[0].Reason = "backup cleanup is incomplete"
		report.Sets[0].NextAction = "Reconcile the preserved conflict, then retry factory upgrade --prune-backup " + id
		report.totals()
		return report, &Failure{Code: 1, Reason: "backup cleanup is incomplete", cause: err}
	}
	report.RemovedSets = 1
	report.Status = "completed"
	report.totals()
	return report, errors.Join(tree.Check(ctx), guard.Check(ctx), lease.Check(ctx))
}
