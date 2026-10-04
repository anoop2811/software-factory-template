package assessment

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/anoop2811/software-factory-template/internal/filepublish"
	"github.com/anoop2811/software-factory-template/internal/transition"
	"golang.org/x/sys/unix"
)

const publicationPending = "runtime-publication.pending"

func beginPublication(ctx context.Context, root string, request PublicationRequest, operations publicationOps) (result *Publication, returned error) {
	selected, err := selectedReferences([]string{request.Path})
	if err != nil {
		return nil, err
	}
	if !recoveryID(request.MigrationID) || !validProposalDigest(request.Confirmation) || len(request.Replacement) > assetLimit {
		return nil, failure(2, "invalid publication request")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p := &publicationState{operations: operations, replacement: bytes.Clone(request.Replacement)}
	defer func() {
		if result == nil {
			returned = errors.Join(returned, p.close(context.WithoutCancel(ctx)))
		}
	}()
	p.guard, err = transition.Exclusive(ctx, root)
	if err != nil {
		return nil, failure(2, "publication transition unavailable")
	}
	chain, err := openRoot(ctx, root)
	if err != nil {
		return nil, err
	}
	p.w = &recoveryWriter{chain: chain, ops: operations.storage}
	p.w.root = &writePin{file: chain.last(), stat: chain[len(chain)-1].identity}
	p.w.pins = append(p.w.pins, p.w.root)
	if !trustedDirectory(p.w.root.stat, false) {
		return nil, failure(2, "unsafe publication root")
	}
	proposal, err := proposalPinned(ctx, chain, selected, operations.observe)
	if err != nil {
		return nil, err
	}
	if proposal.ProposalDigest != request.Confirmation {
		return nil, adoptionConflict()
	}
	originals, err := p.w.originals(ctx, selected, proposal)
	if err != nil {
		return nil, err
	}
	p.original, p.expected = originals[0], originals[0]
	p.parent, p.name, err = p.w.catalogParent(ctx, p.w.root, request.Path, false, false, nil)
	if err != nil {
		return nil, err
	}
	for _, pin := range p.w.pins {
		if pin.parent == p.parent.file && pin.name == p.name {
			p.current = pin
		}
	}
	if p.current == nil {
		return nil, failure(1, "publication original unavailable")
	}
	p.mode = os.FileMode(p.current.stat.Mode & 07777)
	if err := p.checkedBackup(ctx, request.MigrationID, selected[0]); err != nil {
		return nil, err
	}
	// The shared stager's root is qualified against the retained parent descriptor.
	// docs/adr/0094-live-publication-restoration.md:50.
	p.directory, err = os.OpenRoot(filepath.Join(root, filepath.Dir(request.Path)))
	if err != nil {
		return nil, failure(1, "cannot open publication directory")
	}
	opened, err := p.directory.Stat(".")
	parentInfo, parentErr := p.parent.file.Stat()
	if err != nil || parentErr != nil || !os.SameFile(opened, parentInfo) {
		return nil, failure(2, "publication directory changed")
	}
	if err := p.check(ctx); err != nil {
		return nil, err
	}
	// Reserve pending only after qualification; every failure preserves its entry.
	// docs/adr/0094-live-publication-restoration.md:54.
	p.pending, err = p.w.createFile(ctx, p.factory, publicationPending)
	if err != nil {
		return nil, err
	}
	if err := p.w.sync(ctx, p.pending); err != nil {
		return nil, err
	}
	if err := p.w.sync(ctx, p.factory); err != nil {
		return nil, err
	}
	if err := p.check(ctx); err != nil {
		return nil, err
	}
	return &Publication{state: p}, nil
}

func (p *publicationState) checkedBackup(ctx context.Context, id string, reference referenceAsset) error {
	var err error
	p.factory, err = p.w.directory(ctx, p.w.root, ".factory", false, false)
	if err != nil {
		return err
	}
	backups, err := p.w.directory(ctx, p.factory, "backups", true, false)
	if err != nil {
		return err
	}
	var pins recoveryPins
	row, manifest, err := inspectRecoverySetPinned(ctx, backups.file, id, p.operations.observe, &pins)
	// Transfer the strict reader's descriptors to the retained writer lifetime.
	for _, pin := range pins {
		if pin.file != nil {
			p.w.pins = append(p.w.pins, &writePin{parent: pin.parent, file: pin.file, name: pin.name, stat: pin.stat})
		}
	}
	if err != nil {
		return err
	}
	if row.Classification != "integrity_checked" {
		code := 2
		if row.Classification == "assessment_error" {
			code = 1
		}
		return failure(code, "publication requires an intact existing recovery set")
	}
	selected := false
	for _, saved := range manifest.assets {
		selected = selected || saved == reference
	}
	if !selected {
		return failure(2, "recovery set does not include the publication original")
	}
	set, err := p.w.directory(ctx, backups, id, true, false)
	if err != nil {
		return err
	}
	parent, name, err := p.w.catalogParent(ctx, set, "files/"+reference.Path, true, false, nil)
	if err != nil {
		return err
	}
	p.saved, err = p.w.existingFile(ctx, parent, name, unix.O_RDONLY, assetLimit, true)
	if err != nil {
		return err
	}
	data, err := p.w.read(ctx, p.saved, assetLimit)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, p.original) || !referenceBytes(data, reference) {
		return failure(2, "saved original changed")
	}
	if err := p.w.sync(ctx, p.saved); err != nil {
		return err
	}
	for _, pin := range p.w.pins {
		if pin.stat.Mode&unix.S_IFMT == unix.S_IFDIR {
			if err := p.w.sync(ctx, pin); err != nil {
				return err
			}
		}
	}
	return p.check(ctx)
}

func (p *publicationState) check(ctx context.Context) error {
	if p.candidate != nil {
		if _, err := p.reconcile(ctx); err != nil {
			return err
		}
	}
	return p.checkState(ctx, p.w, p.current, p.expected)
}

func (p *publicationState) checkState(ctx context.Context, writer *recoveryWriter, selected *writePin, expected []byte) error {
	if err := p.guard.Check(ctx); err != nil {
		return errors.Join(failure(2, "publication transition changed"), ctx.Err())
	}
	if err := writer.check(ctx); err != nil {
		return err
	}
	if p.pending != nil && p.pendingClosed && !p.pendingRemoved {
		current, err := writer.named(p.factory.file, publicationPending)
		if err != nil {
			return observationError(err, "cannot observe publication pending evidence")
		}
		if !writeUnchanged(p.pending.stat, current) {
			return failure(2, "publication pending ownership changed")
		}
	}
	for _, value := range []struct {
		pin  *writePin
		data []byte
	}{{selected, expected}, {p.saved, p.original}} {
		if value.pin == nil {
			continue
		}
		data, err := writer.read(ctx, value.pin, assetLimit)
		if err != nil {
			return err
		}
		if !bytes.Equal(data, value.data) {
			return failure(2, "publication owned bytes changed")
		}
	}
	return nil
}

func (p *publicationState) forget(pin *writePin) {
	for i, entry := range p.w.pins {
		if entry == pin {
			p.w.pins = append(p.w.pins[:i], p.w.pins[i+1:]...)
			return
		}
	}
}

func (p *publicationState) publish(ctx context.Context, data []byte, restoring bool) error {
	prepare := p.operations.prepare
	if prepare == nil {
		prepare = filepublish.Prepare
	}
	stage, err := prepare(ctx, p.directory, ".factory-publication-", data, p.mode)
	if stage != nil {
		p.stages = append(p.stages, stage)
	}
	if refreshErr := p.w.refresh(p.parent); refreshErr != nil {
		return refreshErr
	}
	if err != nil || stage == nil {
		return errors.Join(failure(1, "cannot prepare publication sibling"), ctx.Err())
	}
	openPrepared := p.operations.openPrepared
	if openPrepared == nil {
		openPrepared = func(ctx context.Context, stage *filepublish.Stage) (*os.File, error) { return stage.OpenPrepared(ctx) }
	}
	file, err := openPrepared(ctx, stage)
	if err != nil {
		return errors.Join(failure(1, "cannot retain publication sibling"), ctx.Err())
	}
	stat, err := descriptor(file)
	if err != nil || !ownedFile(stat, assetLimit) || uint32(stat.Mode)&07777 != uint32(p.mode) {
		if p.w.closeFile(file) != nil {
			return errors.Join(failure(1, "cannot close unsafe publication sibling"), failure(2, "unsafe prepared publication sibling"))
		}
		return failure(2, "unsafe prepared publication sibling")
	}
	pin := &writePin{parent: p.parent.file, file: file, name: stage.Name(), stat: stat}
	p.w.pins = append(p.w.pins, pin)
	readback, err := p.w.read(ctx, pin, assetLimit)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, readback) {
		return failure(2, "prepared publication bytes changed")
	}
	if err := p.check(ctx); err != nil {
		return err
	}
	rename := p.operations.rename
	if rename == nil {
		rename = func(ctx context.Context, stage *filepublish.Stage, name string) error {
			return stage.Publish(ctx, name)
		}
	}
	// Retained staging supplies the inode even if rename reports uncertainty.
	// Arbitrary named metadata can never supply reverse ownership.
	// docs/adr/0094-live-publication-restoration.md:78.
	p.changed = true
	p.candidate = &publicationCandidate{pin: pin, data: data, restoring: restoring}
	err = rename(ctx, stage, p.name)
	published, adoptErr := p.reconcile(ctx)
	if err != nil || adoptErr != nil || !published {
		return errors.Join(failure(1, "cannot confirm publication rename"), adoptErr, ctx.Err())
	}
	if err := p.check(ctx); err != nil {
		return err
	}
	return p.w.sync(ctx, p.parent)
}

// A retained candidate is qualified through a read-only view before promotion.
// Only its own parent metadata may be refreshed; every other pin stays intact.
// docs/adr/0094-live-publication-restoration.md:214.
func (p *publicationState) reconcile(ctx context.Context) (bool, error) {
	if err := p.guard.Check(ctx); err != nil {
		return false, errors.Join(failure(2, "publication transition changed"), ctx.Err())
	}
	candidate := p.candidate
	current, err := descriptor(candidate.pin.file)
	location, locationErr := p.w.named(p.parent.file, p.name)
	if err != nil {
		return false, errors.Join(observationError(err, "cannot observe publication candidate"), ctx.Err())
	}
	if locationErr != nil {
		return false, errors.Join(observationError(locationErr, "cannot observe publication candidate"), ctx.Err())
	}
	prepared := candidate.pin.stat
	if !sameIdentity(prepared, current) || current.Mode != prepared.Mode || current.Uid != prepared.Uid || current.Gid != prepared.Gid ||
		current.Nlink != 1 || current.Size != prepared.Size || current.Mtim != prepared.Mtim {
		return false, failure(2, "published inode ownership changed")
	}
	if !writeUnchanged(current, location) {
		// An unrenamed attempt may abort only with both old and staging pins intact.
		// docs/adr/0094-live-publication-restoration.md:224.
		if !writeUnchanged(p.current.stat, location) {
			return false, failure(2, "published inode ownership changed")
		}
		if err := p.checkState(ctx, p.w, p.current, p.expected); err != nil {
			return false, err
		}
		data, err := p.w.read(ctx, candidate.pin, assetLimit)
		if err != nil {
			return false, err
		}
		if !bytes.Equal(data, candidate.data) {
			return false, failure(2, "prepared publication bytes changed")
		}
		p.candidate = nil
		return false, nil
	}

	parent, published := *p.parent, *candidate.pin
	published.name, published.stat = p.name, current
	view := *p.w
	view.pins = make([]*writePin, 0, len(p.w.pins))
	for _, pin := range p.w.pins {
		switch pin {
		case candidate.pin:
			view.pins = append(view.pins, &published)
		case p.current:
			// The exact prior target became detached through this attempted rename.
		case p.parent:
			view.pins = append(view.pins, &parent)
		default:
			view.pins = append(view.pins, pin)
		}
	}
	if err := view.refresh(&parent); err != nil {
		return false, err
	}
	if err := p.checkState(ctx, &view, &published, candidate.data); err != nil {
		return false, err
	}
	// Promotion happens under the shared lifecycle mutex after complete validation.
	// docs/adr/0094-live-publication-restoration.md:218.
	if p.current != candidate.pin {
		p.forget(p.current)
		p.detached = append(p.detached, p.current.file)
	}
	p.parent.stat = parent.stat
	*candidate.pin = published
	p.current, p.expected = candidate.pin, candidate.data
	p.afterPublished = true
	p.undoPublished = candidate.restoring
	p.candidate = nil
	return true, nil
}

func (p *publicationState) removePending(ctx context.Context) error {
	if err := p.check(ctx); err != nil {
		return err
	}
	if !p.pendingClosed {
		p.pendingClosed = true
		p.forget(p.pending)
		if p.w.closeFile(p.pending.file) != nil {
			return failure(1, "cannot close publication pending evidence")
		}
	}
	if !p.pendingRemoved {
		if err := p.check(ctx); err != nil {
			return err
		}
		var err error
		if p.operations.unlink != nil {
			err = p.operations.unlink(ctx, p.factory.file, publicationPending)
		} else {
			err = unix.Unlinkat(int(p.factory.file.Fd()), publicationPending, 0)
		}
		if err != nil {
			return errors.Join(failure(1, "cannot remove publication pending evidence"), ctx.Err())
		}
		p.pendingRemoved = true
		if err := p.w.refresh(p.factory); err != nil {
			return err
		}
	}
	// The original was durable before unlink; never create replacement evidence.
	// docs/adr/0094-live-publication-restoration.md:104.
	return p.w.sync(ctx, p.factory)
}
