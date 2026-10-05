package assessment

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/anoop2811/software-factory-template/internal/transition"
)

// InterruptedRecoveryRequest requires independently known bytes and fresh consent.
// docs/adr/0096-interrupted-publication-recovery.md:28.
type InterruptedRecoveryRequest struct {
	MigrationID        string
	OperationID        string
	Path               string
	Direction          string
	Replacement        []byte
	AfterReference     Observation
	UnbridgedQuiescent bool
}

// InterruptedRecoveryProposal describes current evidence without granting authority.
// docs/adr/0096-interrupted-publication-recovery.md:75.
type InterruptedRecoveryProposal struct {
	SchemaVersion       int           `json:"schema_version"`
	Mode                string        `json:"mode"`
	Scope               string        `json:"scope"`
	Purpose             string        `json:"purpose"`
	MigrationID         string        `json:"migration_id"`
	OperationID         string        `json:"operation_id"`
	Path                string        `json:"path"`
	Direction           string        `json:"direction"`
	CurrentImage        string        `json:"current_image"`
	ProposalDigest      string        `json:"proposal_digest"`
	RootIdentity        RootIdentity  `json:"root_identity"`
	Current             AdoptionAsset `json:"current"`
	Record              Observation   `json:"record"`
	RecordIdentity      AssetIdentity `json:"record_identity"`
	PendingIdentity     AssetIdentity `json:"pending_identity"`
	Recovery            RecoverySet   `json:"recovery"`
	StateCompatibility  string        `json:"state_compatibility"`
	UnbridgedQuiescence string        `json:"unbridged_quiescence"`
	Restorable          bool          `json:"restorable"`
	RollbackReady       bool          `json:"rollback_ready"`
	ActivationReady     bool          `json:"activation_ready"`
	Applicable          bool          `json:"applicable"`
	PruneAuthorized     bool          `json:"prune_authorized"`
}

// InterruptedRecovery is opaque; value aliases will share the private lifecycle.
// docs/adr/0096-interrupted-publication-recovery.md:63.
type InterruptedRecovery struct{ state *publicationState }

// ProposeInterruptedRecovery observes existing evidence without granting authority.
// docs/adr/0096-interrupted-publication-recovery.md:70.
func ProposeInterruptedRecovery(ctx context.Context, root string, request InterruptedRecoveryRequest) (InterruptedRecoveryProposal, error) {
	return proposeInterruptedRecovery(ctx, root, request, ops{})
}

func proposeInterruptedRecovery(ctx context.Context, root string, request InterruptedRecoveryRequest, operations ops) (result InterruptedRecoveryProposal, returned error) {
	observation, err := observeInterrupted(ctx, root, request, interruptedReadOps(operations), operations)
	if err != nil {
		return result, err
	}
	defer func() {
		if err := observation.w.close(); err != nil {
			result = InterruptedRecoveryProposal{}
			returned = errors.Join(err, returned)
		}
	}()
	if err := observation.check(ctx, observation.w); err != nil {
		return result, err
	}
	return observation.proposal, nil
}

// BeginInterruptedRecovery recomputes consent while holding existing exclusion.
// docs/adr/0096-interrupted-publication-recovery.md:127.
func BeginInterruptedRecovery(ctx context.Context, root string, request InterruptedRecoveryRequest, confirmation string, environment map[string]string) (*InterruptedRecovery, error) {
	return beginInterruptedRecovery(ctx, root, request, confirmation, environment, publicationOps{})
}

func beginInterruptedRecovery(ctx context.Context, root string, request InterruptedRecoveryRequest, confirmation string, environment map[string]string, operations publicationOps) (result *InterruptedRecovery, returned error) {
	if !validProposalDigest(confirmation) {
		return nil, failure(2, "invalid interrupted recovery consent")
	}
	request.Replacement = bytes.Clone(request.Replacement)
	p := &publicationState{operations: operations, replacement: request.Replacement, applyUsed: true, recoveryDirection: request.Direction}
	defer func() {
		if result == nil {
			// Borrowed pending is preserved; failed grant is not an incomplete handle.
			// docs/adr/0096-interrupted-publication-recovery.md:225.
			cleanup := p.closeResources(context.WithoutCancel(ctx), false)
			// Operational precedence spans both retained error trees.
			// docs/adr/0096-interrupted-publication-recovery.md:291.
			var operational error
			if interruptedGuardOperational(cleanup) || interruptedGuardOperational(returned) {
				operational = failure(1, "interrupted recovery failed")
			}
			returned = errors.Join(operational, cleanup, returned)
		}
	}()
	var err error
	p.guard, err = transition.ExclusiveRecovery(ctx, root)
	if err != nil {
		return nil, interruptedGuardError(ctx, err)
	}
	storage := operations.storage
	if operations.close != nil {
		storage.close = operations.close
	}
	observation, err := observeInterrupted(ctx, root, request, storage, operations.observe)
	if err != nil {
		return nil, err
	}
	p.w = observation.w
	if observation.proposal.ProposalDigest != confirmation {
		return nil, failure(2, "interrupted recovery consent changed")
	}
	p.factory, p.parent, p.current = observation.factory, observation.parent, observation.current
	p.saved, p.pending, p.name = observation.saved, observation.pending, observation.name
	p.original, p.expected, p.mode = observation.original, observation.currentBytes, os.FileMode(p.current.stat.Mode&07777)
	p.checkEvidence = observation.check
	p.journal = &publicationJournal{parent: observation.records, current: observation.recordPin,
		name: request.MigrationID + ".json", expected: observation.recordBytes,
		record: observation.record, direction: request.Direction}
	p.afterPublished = observation.proposal.CurrentImage == "after"
	p.undoPublished = observation.proposal.CurrentImage == "before" && (observation.record.Phase == "reverse_prepared" || observation.record.Phase == "restored")
	if p.undoPublished {
		p.afterPublished = true
	}
	// Fresh after-image metadata describes the current image, not old staging ctime.
	// docs/adr/0096-interrupted-publication-recovery.md:168.
	if observation.proposal.CurrentImage == "after" {
		identity := assetIdentity(p.current.stat)
		p.journal.record.ForwardIdentity = &identity
	}
	root, err = p.qualifiedAbsoluteRoot(ctx, root)
	if err != nil {
		return nil, err
	}
	p.directory, err = interruptedDirectory(ctx, root, filepath.Dir(request.Path), p.parent, operations)
	if err != nil {
		return nil, err
	}
	p.journal.directory, err = interruptedDirectory(ctx, root, ".factory/backups/"+publicationNamespace, p.journal.parent, operations)
	if err != nil {
		return nil, err
	}
	if err := p.check(ctx); err != nil {
		return nil, err
	}
	// Requalify durability of the existing barrier before any owned query starts.
	// docs/adr/0095-durable-live-publication.md:54.
	if err := p.w.sync(ctx, p.pending); err != nil {
		return nil, err
	}
	if err := p.w.sync(ctx, p.factory); err != nil {
		return nil, err
	}
	git, err := p.w.localGitWith(ctx, root, environment, operations.query)
	if err != nil {
		return nil, err
	}
	if err := git.requireIgnored(ctx, []string{".factory/backups/", ".factory/backups/" + publicationNamespace + "/", ".factory/backups/" + publicationNamespace + "/" + request.MigrationID + ".json"}); err != nil {
		return nil, err
	}
	if err := p.check(ctx); err != nil {
		return nil, err
	}
	return &InterruptedRecovery{state: p}, nil
}

// Complete resolves only the freshly granted direction using shared publication.
// docs/adr/0096-interrupted-publication-recovery.md:191.
func (r *InterruptedRecovery) Complete(ctx context.Context) error {
	if r == nil || r.state == nil {
		return failure(2, "interrupted recovery handle unavailable")
	}
	if r.state.recoveryDirection == "forward" {
		return r.state.finish(ctx)
	}
	return r.state.restore(ctx)
}

// Close releases the fresh capability without resolving its pending operation.
// docs/adr/0096-interrupted-publication-recovery.md:213.
func (r *InterruptedRecovery) Close(ctx context.Context) error {
	if r == nil || r.state == nil {
		return failure(2, "interrupted recovery handle unavailable")
	}
	return r.state.close(ctx)
}

func interruptedGuardError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return errors.Join(failure(1, "interrupted recovery exclusion unavailable"), ctx.Err())
	}
	if interruptedGuardOperational(err) {
		return failure(1, "interrupted recovery exclusion unavailable")
	}
	return failure(2, "interrupted recovery exclusion unavailable")
}

// Joined cleanup errors retain operational precedence over validation conflicts.
// docs/adr/0096-interrupted-publication-recovery.md:278.
//
//nolint:errorlint // Inspect this exact node; descendant matching would skip joined cleanup siblings.
func interruptedGuardOperational(err error) bool {
	if err == nil {
		return false
	}
	if conflict, ok := err.(*transition.RecoveryError); ok && conflict.Conflict() {
		return false
	}
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		for _, child := range wrapped.Unwrap() {
			if interruptedGuardOperational(child) {
				return true
			}
		}
		return false
	case interface{ Unwrap() error }:
		if child := wrapped.Unwrap(); child != nil {
			return interruptedGuardOperational(child)
		}
	}
	if known, ok := err.(*Failure); ok {
		return known.Code == 1
	}
	return classification(err) == "assessment_error"
}

// Native observation failures are classified before comparing held identities.
// docs/adr/0096-interrupted-publication-recovery.md:231.
func interruptedDirectory(ctx context.Context, root, path string, pin *writePin, operations publicationOps) (result *os.Root, returned error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	directory, err := os.OpenRoot(filepath.Join(root, path))
	if err != nil {
		return nil, observationError(err, "cannot open interrupted recovery directory")
	}
	defer func() {
		if result == nil && directory.Close() != nil {
			returned = errors.Join(failure(1, "cannot close interrupted recovery directory"), returned)
		}
	}()
	directoryStat := operations.directoryStat
	if directoryStat == nil {
		directoryStat = func(_ context.Context, directory *os.Root, name string) (os.FileInfo, error) {
			return directory.Stat(name)
		}
	}
	retainedStat := operations.retainedStat
	if retainedStat == nil {
		retainedStat = func(_ context.Context, file *os.File) (os.FileInfo, error) { return file.Stat() }
	}
	opened, err := directoryStat(ctx, directory, ".")
	if err != nil {
		return nil, observationError(err, "cannot observe interrupted recovery directory")
	}
	retained, retainedErr := retainedStat(ctx, pin.file)
	if retainedErr != nil {
		return nil, observationError(retainedErr, "cannot observe interrupted recovery directory")
	}
	if opened == nil || retained == nil || !os.SameFile(opened, retained) {
		return nil, failure(2, "interrupted recovery directory changed")
	}
	return directory, nil
}
