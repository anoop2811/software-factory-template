package installation

import (
	"context"
	"errors"

	"os"
	"path/filepath"
	"sync"

	"github.com/anoop2811/software-factory-template/internal/filepublish"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/installedlayout"
	"github.com/anoop2811/software-factory-template/internal/staging"
	"github.com/anoop2811/software-factory-template/internal/transition"
)

// Capability holds one retained exclusion lifetime; copies share private state.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:166.
type Capability struct{ state *transaction }
type transaction struct {
	mu                            sync.Mutex
	operations                    commandOps
	ownershipErr                  error
	root                          string
	request                       Request
	planned                       plan
	tree                          *installationfs.Tree
	lease                         *transition.RootLease
	guard                         *transition.Guard
	record                        journal
	journalImage                  *Image
	pending                       *installationfs.File
	stageTree                     *installationfs.Tree
	stageRoot                     string
	scratch                       string
	scratchIdentity               os.FileInfo
	candidate                     string
	proof                         *candidateProof
	published                     map[string]*Image
	newJournal                    bool
	terminalCleanup               bool
	preparationRecovery           bool
	completed, closed, idempotent bool
}

func Begin(ctx context.Context, root string, request Request) (*Capability, error) {
	return beginWith(ctx, root, request, commandOps{})
}
func beginWith(ctx context.Context, root string, request Request, operations commandOps) (result *Capability, returned error) {
	if !request.Quiescent || !digestPattern.MatchString(request.Confirmation) {
		return nil, conflict("installation requires current consent and maintained quiescence")
	}
	owner := &transaction{root: root, request: request, published: map[string]*Image{}, operations: operations}
	defer func() {
		if result == nil {
			returned = errors.Join(returned, owner.close(context.WithoutCancel(ctx)))
		}
	}()
	var err error
	owner.lease, err = transition.RootExclusive(ctx, root)
	if err != nil {
		return nil, err
	}
	if request.Recover != "" {
		owner.tree, err = installationfs.Open(ctx, root)
		if err != nil {
			return nil, err
		}
		pending, _, observationErr := observe(ctx, owner.tree, ".factory/runtime-publication.pending", 0)
		if observationErr != nil {
			return nil, observationErr
		}
		if pending == nil {
			owner.guard, err = transition.ExclusivePreparation(ctx, root)
		} else {
			owner.guard, err = transition.ExclusiveRecovery(ctx, root)
		}
	} else {
		owner.guard, err = transition.Exclusive(ctx, root)
	}
	if err != nil {
		return nil, err
	}
	if owner.tree == nil {
		owner.tree, err = installationfs.Open(ctx, root)
		if err != nil {
			return nil, err
		}
	}
	id := operationID(request)
	existing, _, err := observe(ctx, owner.tree, journalPath(id), journalLimit)
	if err != nil {
		return nil, err
	}
	owner.newJournal = existing == nil
	if existing != nil {
		owner.record, err = readJournalEvidence(ctx, owner.tree, id, request.Recover != "")
		if err != nil {
			return nil, err
		}
		owner.journalImage = existing
		owner.terminalCleanup = request.Recover != "" && owner.record.Phase == "committed"
		owner.preparationRecovery = request.Recover != "" && preparationOnly(owner.record) && owner.record.Pending == nil
		if request.Recover == "" && request.Rollback == "" && owner.record.Phase == "committed" && owner.record.Direction == "forward" && owner.record.PlanDigest == request.Confirmation {
			owner.planned, err = buildTransitionPlan(ctx, root, request, owner.record, false)
			if err != nil {
				return nil, err
			}
			if len(owner.planned.proposal.Blockers) != 0 {
				return nil, conflict("completed installation selections changed")
			}
			owner.idempotent = true
		}
		if request.Rollback == "" && request.Recover == "" && !owner.idempotent {
			return nil, conflict("installation migration identifier is already occupied")
		}
	}
	if !owner.idempotent {
		if request.Rollback != "" || request.Recover != "" {
			if existing == nil {
				return nil, conflict("installation transaction is missing")
			}
			reverse := request.Rollback != "" || request.Direction == "reverse"
			owner.planned, err = buildTransitionPlan(ctx, root, request, owner.record, reverse)
		} else {
			owner.planned, err = buildPlan(ctx, root, request)
		}
	}
	if err != nil {
		return nil, err
	}
	if !owner.idempotent && owner.planned.proposal.ProposalDigest != request.Confirmation {
		return nil, conflict("installation proposal changed; request a fresh preview")
	}
	if len(owner.planned.proposal.Blockers) != 0 || len(owner.planned.proposal.RollbackBlockers) != 0 {
		return nil, conflict("installation proposal has preserved blockers")
	}
	options := request.Source
	owner.scratch, err = os.MkdirTemp("", "factory-installation-operation-")
	if err != nil {
		return nil, err
	}
	owner.scratch, err = filepath.EvalSymlinks(owner.scratch)
	if err != nil {
		return nil, err
	}
	owner.scratchIdentity, err = os.Lstat(owner.scratch)
	if err != nil {
		return nil, err
	}
	options.Installation = true
	options.Output = filepath.Join(owner.scratch, "image")
	staged, err := staging.Stage(ctx, options)
	if err != nil {
		return nil, err
	}
	if staged.SHA256 != owner.planned.image.ArchiveSHA256 {
		return nil, conflict("source archive changed after proposal")
	}
	owner.stageRoot = filepath.Join(staged.Root, options.Version, options.Target)
	owner.stageTree, err = installationfs.Open(ctx, owner.stageRoot)
	if err != nil {
		return nil, err
	}
	// Fresh authentication and staging are required even for completed rollback.
	// docs/adr/0098-whole-installation-upgrade-and-rollback.md:122.
	owner.candidate = filepath.Join(owner.scratch, "candidate")
	if owner.idempotent {
		if _, err := installedlayout.Validate(ctx, owner.tree, []string{request.Source.Version, request.Source.Revision, owner.planned.image.Runtime.SHA256}); err != nil {
			return nil, err
		}
	} else if err := owner.materializeCandidate(ctx); err != nil {
		return nil, &Failure{Code: 1, Reason: "candidate installation preparation failed", cause: err}
	}
	if request.Recover != "" {
		pending, _, err := observe(ctx, owner.tree, ".factory/runtime-publication.pending", 0)
		if err != nil {
			return nil, err
		}
		if pending != nil {
			owner.pending, err = owner.tree.OpenFile(ctx, ".factory/runtime-publication.pending", 0)
			if err != nil {
				return nil, err
			}
		}
	}
	return &Capability{owner}, nil
}
func operationID(request Request) string {
	if request.Rollback != "" {
		return request.Rollback
	}
	if request.Recover != "" {
		return request.Recover
	}
	return request.MigrationID
}
func (capability *Capability) Complete(ctx context.Context) error {
	if capability == nil || capability.state == nil {
		return errors.New("installation capability unavailable")
	}
	owner := capability.state
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.closed || owner.completed {
		return errors.New("installation capability already completed or closed")
	}
	if owner.idempotent {
		owner.completed = true
		return owner.check(ctx)
	}
	if owner.terminalCleanup {
		return owner.completePendingCleanup(ctx)
	}
	if owner.preparationRecovery {
		return owner.abandonPreparation(ctx)
	}
	if err := owner.check(ctx); err != nil {
		return err
	}
	reverse := owner.request.Rollback != "" || owner.request.Direction == "reverse"
	if owner.request.Rollback == "" && owner.request.Recover == "" {
		entries := make([]journalEntry, 0, len(owner.planned.proposal.Actions))
		for _, action := range owner.planned.proposal.Actions {
			if selectedPath(action.Path) {
				entries = append(entries, journalEntry{Path: action.Path, Action: action.Action, Phase: "prepared", Before: action.Before, ExpectedAfter: action.After})
			}
		}
		owner.record = journal{1, "installation-transaction", operationID(owner.request), "upgrade", "forward", "prepared", owner.request.Profile, owner.request.Source.Target, owner.request.Confirmation, entries, []checkRecord{}, "pending", nil}
	}
	if err := owner.preflightCandidate(ctx, reverse); err != nil {
		cleanupErr := owner.discardPreparation(context.WithoutCancel(ctx))
		if cleanupErr != nil {
			return errors.Join(err, cleanupErr)
		}
		var failure *Failure
		if errors.As(err, &failure) {
			return err
		}
		return &Failure{Code: 1, Reason: "candidate installation health preparation failed", cause: err}
	}
	if err := owner.check(ctx); err != nil {
		return err
	}
	if err := owner.ensurePrivateIgnore(ctx); err != nil {
		return err
	}
	if owner.request.Rollback == "" && owner.request.Recover == "" {
		if err := owner.backup(ctx); err != nil {
			return err
		}
	}
	newPending := owner.pending == nil
	if newPending {
		if err := publishBytes(ctx, owner.tree, ".factory/runtime-publication.pending", nil, 0600, nil); err != nil {
			return err
		}
		var err error
		owner.pending, err = owner.tree.OpenFile(ctx, ".factory/runtime-publication.pending", 0)
		if err != nil {
			return err
		}
	}
	observedPending, _, err := observe(ctx, owner.tree, ".factory/runtime-publication.pending", 0)
	if err != nil {
		return err
	}
	if !newPending && owner.record.Pending != nil && !imageEqual(observedPending, owner.record.Pending, true) {
		return conflict("pending publication entry changed")
	}
	owner.record.Pending = observedPending
	owner.record.Purpose = "upgrade"
	if owner.request.Rollback != "" {
		owner.record.Purpose = "rollback"
	}
	if owner.request.Recover != "" {
		owner.record.Purpose = "recovery"
	}
	owner.record.Direction = "forward"
	if reverse {
		owner.record.Direction = "reverse"
	}
	owner.record.Phase = "applying"
	owner.record.Outcome = "pending"
	// Every selected mutation intent is durable before the first active change.
	// docs/adr/0098-whole-installation-upgrade-and-rollback.md:172.
	for _, action := range owner.planned.proposal.Actions {
		if action.Action == "retain" {
			continue
		}
		for index := range owner.record.Entries {
			if owner.record.Entries[index].Path == action.Path {
				owner.record.Entries[index].Phase = "prepared"
				if reverse {
					owner.record.Entries[index].Phase = "reverse_prepared"
				}
			}
		}
	}
	if err := owner.save(ctx); err != nil {
		return err
	}
	for _, action := range owner.planned.proposal.Actions {
		if action.Action == "retain" || action.Path == ".factory-version" {
			continue
		}
		if err := owner.applyRow(ctx, action, reverse); err != nil {
			return err
		}
	}
	owner.record.Phase = "checking"
	if err := owner.save(ctx); err != nil {
		return err
	}
	if reverse {
		if err := owner.validateLegacy(ctx); err != nil {
			return err
		}
	} else {
		if _, err := installedlayout.ValidateAssets(ctx, owner.tree, nil); err != nil {
			return err
		}
		if err := owner.validateProvenTarget(ctx); err != nil {
			return err
		}
	}
	owner.record.Phase = "metadata_prepared"
	if err := owner.save(ctx); err != nil {
		return err
	}
	for _, action := range owner.planned.proposal.Actions {
		if action.Path == ".factory-version" && action.Action != "retain" {
			if err := owner.applyRow(ctx, action, reverse); err != nil {
				return err
			}
		}
	}
	if !reverse {
		if _, err := installedlayout.Validate(ctx, owner.tree, nil); err != nil {
			return err
		}
	}
	owner.record.Phase = "committed"
	owner.record.Outcome = "completed"
	if err := owner.save(ctx); err != nil {
		return err
	}
	if err := owner.pending.Check(ctx); err != nil {
		return err
	}
	parent, name := owner.pending.Parent, owner.pending.Name
	if err := parent.Root.Remove(name); err != nil {
		return err
	}
	if err := parent.Sync(ctx); err != nil {
		return &Uncertainty{Operation: "pending cleanup durability", cause: err}
	}
	image, _, err := observe(ctx, owner.tree, ".factory/runtime-publication.pending", 0)
	if err != nil {
		return err
	}
	if image != nil {
		return conflict("pending cleanup ownership is uncertain")
	}
	if err := owner.pending.Close(); err != nil {
		return &Uncertainty{Operation: "pending checked closure", cause: err}
	}
	owner.pending = nil
	owner.completed = true
	return owner.check(ctx)
}
func (capability *Capability) Close(ctx context.Context) error {
	if capability == nil || capability.state == nil {
		return errors.New("installation capability unavailable")
	}
	owner := capability.state
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.closed {
		return errors.New("installation capability closed")
	}
	return owner.close(ctx)
}
func (owner *transaction) close(ctx context.Context) error {
	if owner.closed {
		return nil
	}
	var errs []error
	unresolved := owner.unresolvedWork()
	if unresolved {
		errs = append(errs, owner.persistUnresolved(ctx))
	}
	owner.closed = true
	if owner.pending != nil {
		errs = append(errs, owner.pending.Close())
	}
	if owner.stageTree != nil {
		errs = append(errs, owner.stageTree.Close())
	}
	if owner.tree != nil {
		errs = append(errs, owner.tree.Close())
	}
	if owner.scratch != "" && !unresolved {
		current, err := os.Lstat(owner.scratch)
		if err != nil || owner.scratchIdentity == nil || !os.SameFile(owner.scratchIdentity, current) {
			errs = append(errs, errors.Join(errors.New("private operation scratch changed; cleanup refused"), err))
		} else {
			errs = append(errs, os.RemoveAll(owner.scratch))
		}
	}
	if owner.guard != nil {
		errs = append(errs, owner.guard.Close(ctx, !unresolved))
	}
	if owner.lease != nil {
		errs = append(errs, owner.lease.Close(ctx))
	}
	return errors.Join(errs...)
}
func (owner *transaction) check(ctx context.Context) error {
	if err := owner.lease.Check(ctx); err != nil {
		return err
	}
	if err := owner.guard.Check(ctx); err != nil {
		return err
	}
	return owner.tree.Check(ctx)
}
func (owner *transaction) save(ctx context.Context) error {
	if err := owner.check(ctx); err != nil {
		return err
	}
	var err error
	owner.journalImage, err = saveJournal(ctx, owner.tree, owner.record, owner.journalImage)
	return err
}
func (owner *transaction) applyRow(ctx context.Context, action Action, reverse bool) error {
	if err := owner.check(ctx); err != nil {
		return err
	}
	current, _, err := observe(ctx, owner.tree, action.Path, leafLimit(action.Path))
	if err != nil {
		return err
	}
	if !imageEqual(current, action.Before, true) {
		return conflict("selected installation entry changed before mutation")
	}
	intent := false
	for _, entry := range owner.record.Entries {
		if entry.Path == action.Path && (entry.Phase == "prepared" && !reverse || entry.Phase == "reverse_prepared" && reverse) {
			intent = true
		}
	}
	if !intent {
		return conflict("durable installation row intent is unavailable")
	}
	switch {
	case action.After == nil:
		if current != nil {
			parent, name, _, err := owner.tree.Parent(ctx, action.Path, false)
			if err != nil {
				return err
			}
			if err := parent.Root.Remove(name); err != nil {
				return err
			}
			if err := parent.Sync(ctx); err != nil {
				return err
			}
		}
	case action.Path == ".factory/bin/factory-runtime" && !reverse:
		if err := owner.publishBinaryWith(ctx, owner.tree, action.Path, current, owner.preparedSaver(action.Path)); err != nil {
			return err
		}
	default:
		if err := publishBytesWith(ctx, owner.tree, action.Path, owner.planned.payload[action.Path], os.FileMode(action.After.Mode&0777), current, owner.preparedSaver(action.Path)); err != nil {
			return err
		}
	}
	actual, _, err := observe(ctx, owner.tree, action.Path, leafLimit(action.Path))
	if err != nil {
		return err
	}
	owner.published[action.Path] = actual
	if !imageEqual(actual, action.After, false) {
		return conflict("published installation entry requires recovery")
	}
	for index := range owner.record.Entries {
		if owner.record.Entries[index].Path == action.Path {
			owner.record.Entries[index].ObservedAfter = actual
			owner.record.Entries[index].Phase = "applied"
			if reverse {
				owner.record.Entries[index].Phase = "reversed"
			}
		}
	}
	return owner.save(ctx)
}
func (owner *transaction) publishBinary(ctx context.Context, tree *installationfs.Tree, path string, expected *Image) error {
	return owner.publishBinaryWith(ctx, tree, path, expected, nil)
}
func (owner *transaction) publishBinaryWith(ctx context.Context, tree *installationfs.Tree, path string, expected *Image, onPrepared func(context.Context, *Image) error) (returned error) {
	source, err := owner.stageTree.OpenFile(ctx, "bin/factory-runtime", 256<<20)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, source.Close()) }()
	parent, name, _, err := tree.Parent(ctx, path, true)
	if err != nil {
		return err
	}
	stage, err := filepublish.PrepareReader(ctx, parent.Root, ".installation-stage-", source.File, owner.planned.image.BinaryBytes, owner.planned.image.Runtime.SHA256, 0700)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, stage.Cleanup()) }()
	if onPrepared != nil {
		if err := preparedObservation(ctx, stage, &Image{Type: "regular", Mode: 0100700, SHA256: owner.planned.image.Runtime.SHA256, Bytes: owner.planned.image.BinaryBytes}, onPrepared); err != nil {
			return err
		}
	}
	if err := source.Check(ctx); err != nil {
		return err
	}
	current, _, err := observe(ctx, tree, path, 256<<20)
	if err != nil {
		return err
	}
	if !imageEqual(current, expected, true) {
		return conflict("binary destination changed")
	}
	if expected == nil {
		err = stage.PublishNoReplace(ctx, name)
	} else {
		err = stage.Publish(ctx, name)
	}
	if err != nil {
		return err
	}
	if err := parent.Sync(ctx); err != nil {
		return &Uncertainty{Operation: "binary publication durability", cause: err}
	}
	return nil
}
func Apply(ctx context.Context, root string, request Request) (returned error) {
	owner, err := Begin(ctx, root, request)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, owner.Close(context.WithoutCancel(ctx))) }()
	return owner.Complete(ctx)
}

func (owner *transaction) preparedSaver(path string) func(context.Context, *Image) error {
	return func(ctx context.Context, image *Image) error {
		for index := range owner.record.Entries {
			if owner.record.Entries[index].Path == path {
				owner.record.Entries[index].PreparedAfter = image
				return owner.save(ctx)
			}
		}
		return conflict("prepared installation row is missing")
	}
}
