package assessment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// RecoveryRequest requires current explicit consent for the exact selection.
// docs/adr/0091-durable-local-recovery-creation.md:33.
type RecoveryRequest struct {
	MigrationID    string
	TargetRevision string
	Paths          []string
	Confirmation   string
}

// RecoveryCreation describes partial inert evidence without transaction authority.
// docs/adr/0091-durable-local-recovery-creation.md:50.
type RecoveryCreation struct {
	SchemaVersion        int    `json:"schema_version"`
	Mode                 string `json:"mode"`
	Coverage             string `json:"coverage"`
	Scope                string `json:"scope"`
	MigrationID          string `json:"migration_id"`
	Path                 string `json:"path"`
	Result               string `json:"result"`
	FileCount            int    `json:"file_count"`
	Bytes                int64  `json:"bytes"`
	SourceRevision       string `json:"source_revision"`
	TargetRevision       string `json:"target_revision"`
	TargetAuthentication string `json:"target_authentication"`
	Restorable           bool   `json:"restorable"`
	ActivationReady      bool   `json:"activation_ready"`
	PruneAuthorized      bool   `json:"prune_authorized"`
}

type recoveryCreationFailure struct{ cause error }

func (e *recoveryCreationFailure) Error() string {
	return e.cause.Error() + "; local recovery state may remain and needs inspection"
}
func (e *recoveryCreationFailure) Unwrap() error { return e.cause }

// RecoveryStateMayRemain distinguishes a refusal before mutation from preserved
// evidence after writes. docs/adr/0091-durable-local-recovery-creation.md:58.
func RecoveryStateMayRemain(err error) bool {
	var failure *recoveryCreationFailure
	return errors.As(err, &failure)
}

// CreateRecovery preserves active originals and exclusively publishes a durable
// v1 recovery set. docs/adr/0091-durable-local-recovery-creation.md:114.
func CreateRecovery(ctx context.Context, root string, request RecoveryRequest, environment map[string]string) (RecoveryCreation, error) {
	return createRecovery(ctx, root, request, environment, recoveryWriteOps{})
}

func createRecovery(ctx context.Context, root string, request RecoveryRequest, environment map[string]string, operations recoveryWriteOps) (result RecoveryCreation, returned error) {
	selected, err := selectedReferences(request.Paths)
	if err != nil {
		return RecoveryCreation{}, err
	}
	if !recoveryID(request.MigrationID) || !recoveryRevision(request.TargetRevision) || request.TargetRevision == referenceRevision || !validProposalDigest(request.Confirmation) {
		return RecoveryCreation{}, failure(2, "invalid recovery creation request")
	}
	if err := ctx.Err(); err != nil {
		return RecoveryCreation{}, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return RecoveryCreation{}, failure(1, "cannot resolve recovery installation")
	}
	chain, err := openRoot(ctx, root)
	if err != nil {
		return RecoveryCreation{}, err
	}
	w := &recoveryWriter{chain: chain, ops: operations}
	w.root = &writePin{file: chain.last(), stat: chain[len(chain)-1].identity}
	w.pins = append(w.pins, w.root)
	defer func() {
		returned = errors.Join(returned, w.close())
		if returned != nil {
			result = RecoveryCreation{}
			if w.changed {
				returned = &recoveryCreationFailure{cause: returned}
			}
		}
	}()
	if !trustedDirectory(w.root.stat, false) {
		return RecoveryCreation{}, failure(2, "unsafe recovery installation directory")
	}
	proposal, err := proposalPinned(ctx, chain, selected, ops{})
	if err != nil {
		return RecoveryCreation{}, err
	}
	if proposal.ProposalDigest != request.Confirmation {
		return RecoveryCreation{}, adoptionConflict()
	}
	originals, err := w.originals(ctx, selected, proposal)
	if err != nil {
		return RecoveryCreation{}, err
	}
	if err := w.preflightStorage(ctx, request.MigrationID); err != nil {
		return RecoveryCreation{}, err
	}
	git, err := w.localGit(ctx, root, environment)
	if err != nil {
		return RecoveryCreation{}, err
	}
	if err := git.acquire(ctx); err != nil {
		return RecoveryCreation{}, err
	}
	paths := recoveryIgnorePaths(request.MigrationID, selected)
	if err := git.ensureIgnored(ctx, paths); err != nil {
		return RecoveryCreation{}, err
	}
	factory, err := w.directory(ctx, w.root, ".factory", false, true)
	if err != nil {
		return RecoveryCreation{}, err
	}
	backups, err := w.directory(ctx, factory, "backups", true, true)
	if err != nil {
		return RecoveryCreation{}, err
	}
	entries, class := recoveryEntries(ctx, backups.file, 64)
	if err := ctx.Err(); err != nil {
		return RecoveryCreation{}, err
	}
	if class != "" {
		code := 2
		if class == "assessment_error" {
			code = 1
		}
		return RecoveryCreation{}, failure(code, "cannot reserve within the bounded recovery inventory")
	}
	_, occupied := named(backups.file, request.MigrationID)
	if occupied == nil {
		if err := w.reuse(ctx, backups, factory, selected, request, git, paths); err != nil {
			return RecoveryCreation{}, err
		}
		return recoveryCreated(request, selected, "already_present"), nil
	}
	if !errors.Is(occupied, unix.ENOENT) {
		return RecoveryCreation{}, failure(1, "cannot inspect the requested recovery reservation")
	}
	if len(entries) == 64 {
		return RecoveryCreation{}, failure(2, "recovery inventory has no reservation capacity")
	}
	set, err := w.reserveDirectory(ctx, backups, request.MigrationID)
	if err != nil {
		return RecoveryCreation{}, err
	}
	dirs := []*writePin{w.root, factory, backups, set}
	files, err := w.saveOriginals(ctx, set, selected, originals, &dirs)
	if err != nil {
		return RecoveryCreation{}, err
	}
	if err := w.syncDirectories(ctx, dirs); err != nil {
		return RecoveryCreation{}, err
	}
	if err := w.completionChecks(ctx, selected, request.Confirmation, git, paths); err != nil {
		return RecoveryCreation{}, err
	}
	manifest, err := encodeRecoveryManifest(request, selected)
	if err != nil {
		return RecoveryCreation{}, err
	}
	record, err := w.createFile(ctx, set, "manifest.json")
	if err != nil {
		return RecoveryCreation{}, err
	}
	if err := w.write(ctx, record, manifest); err != nil {
		return RecoveryCreation{}, err
	}
	if err := w.sync(ctx, record); err != nil {
		return RecoveryCreation{}, err
	}
	readback, err := w.read(ctx, record, 16<<10)
	if err != nil {
		return RecoveryCreation{}, err
	}
	if !bytes.Equal(readback, manifest) {
		return RecoveryCreation{}, failure(1, "recovery completion readback did not match")
	}
	if err := w.syncDirectories(ctx, dirs); err != nil {
		return RecoveryCreation{}, err
	}
	if err := w.checkComplete(ctx, backups, request.MigrationID, len(files)); err != nil {
		return RecoveryCreation{}, err
	}
	if err := w.completionChecks(ctx, selected, request.Confirmation, git, paths); err != nil {
		return RecoveryCreation{}, err
	}
	return recoveryCreated(request, selected, "created"), nil
}

func recoveryCreated(request RecoveryRequest, selected []referenceAsset, result string) RecoveryCreation {
	report := RecoveryCreation{SchemaVersion: 1, Mode: "create_backup", Coverage: "partial", Scope: "g2-budget-loop-six", MigrationID: request.MigrationID, Path: ".factory/backups/" + request.MigrationID, Result: result, FileCount: len(selected), SourceRevision: referenceRevision, TargetRevision: request.TargetRevision, TargetAuthentication: "operator_metadata"}
	for _, reference := range selected {
		report.Bytes += reference.Reference.Bytes
	}
	return report
}

func recoveryIgnorePaths(id string, selected []referenceAsset) []string {
	set := ".factory/backups/" + id + "/"
	paths := []string{".factory/backups/", set, set + "manifest.json"}
	for _, reference := range selected {
		paths = append(paths, set+"files/"+reference.Path)
	}
	return paths
}

// Validate existing physical storage before asking Git to evaluate paths below
// it. No Git path query traverses an unqualified recovery control directory.
// docs/adr/0091-durable-local-recovery-creation.md:96.
func (w *recoveryWriter) preflightStorage(ctx context.Context, id string) error {
	parent := w.root
	for i, name := range []string{".factory", "backups"} {
		_, err := named(parent.file, name)
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		if err != nil {
			return failure(1, "cannot inspect recovery storage")
		}
		parent, err = w.directory(ctx, parent, name, i != 0, false)
		if err != nil {
			return err
		}
	}
	_, err := named(parent.file, id)
	if errors.Is(err, unix.ENOENT) {
		return w.check(ctx)
	}
	if err != nil {
		return failure(1, "cannot inspect the requested recovery reservation")
	}
	row, err := inspectRecoverySet(ctx, parent.file, id, ops{})
	if err != nil {
		return err
	}
	if row.Classification == "assessment_error" {
		return failure(1, "cannot inspect occupied recovery storage")
	}
	if row.Classification != "integrity_checked" || row.Held == nil || *row.Held {
		return failure(2, "occupied recovery ID requires inspection")
	}
	return w.check(ctx)
}

func (w *recoveryWriter) originals(ctx context.Context, selected []referenceAsset, proposal Proposal) ([][]byte, error) {
	data := make([][]byte, 0, len(selected))
	for i, reference := range selected {
		parent, name, err := w.catalogParent(ctx, w.root, reference.Path, false, false, nil)
		if err != nil {
			return nil, err
		}
		pin, err := w.existingFile(ctx, parent, name, unix.O_RDONLY, assetLimit, false)
		if err != nil {
			return nil, err
		}
		if assetIdentity(pin.stat) != proposal.Assets[i].Identity {
			return nil, adoptionConflict()
		}
		original, err := w.read(ctx, pin, assetLimit)
		if err != nil {
			return nil, err
		}
		if !referenceBytes(original, reference) {
			return nil, adoptionConflict()
		}
		data = append(data, original)
	}
	return data, w.check(ctx)
}

func (w *recoveryWriter) catalogParent(ctx context.Context, start *writePin, path string, private, create bool, dirs *[]*writePin) (*writePin, string, error) {
	components := strings.Split(path, "/")
	parent := start
	for _, name := range components[:len(components)-1] {
		child, err := w.directory(ctx, parent, name, private, create)
		if err != nil {
			return nil, "", err
		}
		if dirs != nil {
			found := false
			for _, prior := range *dirs {
				found = found || prior == child
			}
			if !found {
				*dirs = append(*dirs, child)
			}
		}
		parent = child
	}
	return parent, components[len(components)-1], nil
}

func (w *recoveryWriter) reserveDirectory(ctx context.Context, parent *writePin, name string) (*writePin, error) {
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	if err := unix.Mkdirat(int(parent.file.Fd()), name, 0700); err != nil {
		return nil, creationError(err, "requested recovery ID is unavailable")
	}
	w.changed = true
	if err := w.refresh(parent); err != nil {
		return nil, err
	}
	return w.directory(ctx, parent, name, true, false)
}

func (w *recoveryWriter) saveOriginals(ctx context.Context, set *writePin, selected []referenceAsset, originals [][]byte, dirs *[]*writePin) ([]*writePin, error) {
	files := make([]*writePin, 0, len(selected))
	for i, reference := range selected {
		parent, name, err := w.catalogParent(ctx, set, "files/"+reference.Path, true, true, dirs)
		if err != nil {
			return nil, err
		}
		pin, err := w.createFile(ctx, parent, name)
		if err != nil {
			return nil, err
		}
		if err := w.write(ctx, pin, originals[i]); err != nil {
			return nil, err
		}
		if err := w.sync(ctx, pin); err != nil {
			return nil, err
		}
		readback, err := w.read(ctx, pin, assetLimit)
		if err != nil {
			return nil, err
		}
		if !referenceBytes(readback, reference) {
			return nil, failure(1, "saved recovery readback did not match")
		}
		files = append(files, pin)
	}
	return files, nil
}

func (w *recoveryWriter) syncDirectories(ctx context.Context, dirs []*writePin) error {
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := w.sync(ctx, dirs[i]); err != nil {
			return err
		}
	}
	return nil
}

func (w *recoveryWriter) completionChecks(ctx context.Context, selected []referenceAsset, confirmation string, git *recoveryGit, paths []string) error {
	if err := w.check(ctx); err != nil {
		return err
	}
	proposal, err := proposalPinned(ctx, w.chain, selected, ops{})
	if err != nil {
		return err
	}
	if proposal.ProposalDigest != confirmation {
		return adoptionConflict()
	}
	if err := git.untracked(ctx); err != nil {
		return err
	}
	ignored, err := git.ignored(ctx, paths)
	if err != nil {
		return err
	}
	if !ignored {
		return failure(2, "project ignore rules expose recovery paths")
	}
	return w.check(ctx)
}

func (w *recoveryWriter) checkComplete(ctx context.Context, backups *writePin, id string, count int) error {
	row, err := inspectRecoverySet(ctx, backups.file, id, ops{})
	if err != nil {
		return err
	}
	if row.Classification != "integrity_checked" || row.Held == nil || *row.Held || row.FileCount != count {
		return failure(1, "recovery integrity readback did not complete")
	}
	return w.check(ctx)
}

type writtenRecoveryManifest struct {
	SchemaVersion  int                    `json:"schema_version"`
	MigrationID    string                 `json:"migration_id"`
	SourceRevision string                 `json:"source_revision"`
	TargetRevision string                 `json:"target_revision"`
	Scope          string                 `json:"scope"`
	Held           bool                   `json:"held"`
	Assets         []writtenRecoveryAsset `json:"assets"`
}
type writtenRecoveryAsset struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Mode   string `json:"mode"`
	Bytes  int64  `json:"bytes"`
}

func encodeRecoveryManifest(request RecoveryRequest, selected []referenceAsset) ([]byte, error) {
	record := writtenRecoveryManifest{SchemaVersion: 1, MigrationID: request.MigrationID, SourceRevision: referenceRevision, TargetRevision: request.TargetRevision, Scope: "g2-budget-loop-six", Assets: make([]writtenRecoveryAsset, 0, len(selected))}
	for _, reference := range selected {
		record.Assets = append(record.Assets, writtenRecoveryAsset{Path: reference.Path, SHA256: reference.Reference.SHA256, Mode: reference.Reference.Mode, Bytes: reference.Reference.Bytes})
	}
	data, err := json.Marshal(record)
	if err != nil {
		return nil, failure(1, "cannot encode recovery completion")
	}
	return append(data, '\n'), nil
}

// Exact intact sets may be re-synced; every other occupied ID is preserved.
// docs/adr/0091-durable-local-recovery-creation.md:124.
func (w *recoveryWriter) reuse(ctx context.Context, backups, factory *writePin, selected []referenceAsset, request RecoveryRequest, git *recoveryGit, paths []string) error {
	row, err := inspectRecoverySet(ctx, backups.file, request.MigrationID, ops{})
	if err != nil {
		return err
	}
	if row.Classification != "integrity_checked" || row.Held == nil || *row.Held {
		return failure(2, "occupied recovery ID requires inspection")
	}
	set, err := w.directory(ctx, backups, request.MigrationID, true, false)
	if err != nil {
		return err
	}
	record, err := w.existingFile(ctx, set, "manifest.json", unix.O_RDONLY, 16<<10, true)
	if err != nil {
		return err
	}
	data, err := w.read(ctx, record, 16<<10)
	if err != nil {
		return err
	}
	manifest, valid := parseRecoveryManifest(ctx, data, request.MigrationID)
	if !valid || manifest.held || manifest.target != request.TargetRevision || len(manifest.assets) != len(selected) {
		return failure(2, "occupied recovery ID does not match the request")
	}
	for i := range selected {
		if manifest.assets[i] != selected[i] {
			return failure(2, "occupied recovery ID does not match the request")
		}
	}
	dirs := []*writePin{w.root, factory, backups, set}
	for _, reference := range selected {
		parent, name, err := w.catalogParent(ctx, set, "files/"+reference.Path, true, false, &dirs)
		if err != nil {
			return err
		}
		pin, err := w.existingFile(ctx, parent, name, unix.O_RDONLY, assetLimit, true)
		if err != nil {
			return err
		}
		data, err := w.read(ctx, pin, assetLimit)
		if err != nil {
			return err
		}
		if !referenceBytes(data, reference) {
			return failure(2, "occupied recovery ID requires inspection")
		}
		if err := w.sync(ctx, pin); err != nil {
			return err
		}
	}
	if err := w.sync(ctx, record); err != nil {
		return err
	}
	if err := w.syncDirectories(ctx, dirs); err != nil {
		return err
	}
	if err := w.checkComplete(ctx, backups, request.MigrationID, len(selected)); err != nil {
		return err
	}
	return w.completionChecks(ctx, selected, request.Confirmation, git, paths)
}
