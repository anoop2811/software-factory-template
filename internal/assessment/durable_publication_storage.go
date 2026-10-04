package assessment

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"

	"github.com/anoop2811/software-factory-template/internal/filepublish"
	"golang.org/x/sys/unix"
)

type durableSetup struct{ environment map[string]string }

type publicationJournal struct {
	parent          *writePin
	directory       *os.Root
	name            string
	current         *writePin
	expected        []byte
	record          publicationRecordData
	candidate       *publicationCandidate
	candidateRecord publicationRecordData
	direction       string
}

// Occupied slots are refused without launching a query or reserving pending.
// docs/adr/0095-durable-live-publication.md:58.
func (p *publicationState) preflightJournal(ctx context.Context, request PublicationRequest) error {
	backups, err := p.w.directory(ctx, p.factory, "backups", true, false)
	if err != nil {
		return err
	}
	var pins recoveryPins
	_, class := recoveryStorageEntries(ctx, backups.file, p.operations.observe, &pins)
	for _, pin := range pins {
		if pin.file != nil {
			p.w.pins = append(p.w.pins, &writePin{parent: pin.parent, file: pin.file, name: pin.name, stat: pin.stat})
		}
	}
	if class != "" {
		code := 2
		if class == "assessment_error" {
			code = 1
		}
		return failure(code, "publication record inventory unavailable")
	}
	stat, err := p.w.named(backups.file, publicationNamespace)
	if errors.Is(err, unix.ENOENT) {
		return p.check(ctx)
	}
	if err != nil {
		return observationError(err, "cannot observe publication namespace")
	}
	if !trustedDirectory(stat, true) {
		return failure(2, "unsafe publication namespace")
	}
	parent, err := p.w.directory(ctx, backups, publicationNamespace, true, false)
	if err != nil {
		return err
	}
	if _, err := p.w.named(parent.file, request.MigrationID+".json"); !errors.Is(err, unix.ENOENT) {
		if err != nil {
			return observationError(err, "cannot observe publication record slot")
		}
		return failure(2, "publication record slot is occupied")
	}
	// Count the complete checked inventory, not its exhausted directory cursor.
	// docs/adr/0095-durable-live-publication.md:219.
	recordCount := 0
	for _, pin := range pins {
		if pin.parent == parent.file && pin.file != nil {
			recordCount++
		}
	}
	if recordCount >= 64 {
		return failure(2, "publication record inventory has no capacity")
	}
	return p.check(ctx)
}

// Pending is already durable before this constructor-owned local Git boundary.
// docs/adr/0095-durable-live-publication.md:54.
func (p *publicationState) beginJournal(ctx context.Context, root string, request PublicationRequest, environment map[string]string) error {
	git, err := p.w.localGitWith(ctx, root, environment, p.operations.query)
	if err != nil {
		return err
	}
	paths := []string{".factory/backups/.publications/", ".factory/backups/.publications/" + request.MigrationID + ".json"}
	if err := git.requireIgnored(ctx, paths); err != nil {
		return err
	}
	backups, err := p.w.directory(ctx, p.factory, "backups", true, false)
	if err != nil {
		return err
	}
	parent, err := p.w.directory(ctx, backups, publicationNamespace, true, true)
	if err != nil {
		return err
	}
	if err := p.w.sync(ctx, backups); err != nil {
		return err
	}
	if err := p.w.sync(ctx, parent); err != nil {
		return err
	}
	current, err := p.w.createFile(ctx, parent, request.MigrationID+".json")
	if err != nil {
		return err
	}
	p.journal = &publicationJournal{parent: parent, current: current, name: request.MigrationID + ".json"}
	p.journal.directory, err = os.OpenRoot(filepath.Join(root, ".factory", "backups", publicationNamespace))
	if err != nil {
		return failure(1, "cannot open publication record directory")
	}
	opened, err := p.journal.directory.Stat(".")
	retained, retainedErr := parent.file.Stat()
	if err != nil || retainedErr != nil || !os.SameFile(opened, retained) {
		return failure(2, "publication record directory changed")
	}
	set, err := p.w.directory(ctx, backups, request.MigrationID, true, false)
	if err != nil {
		return err
	}
	mode := assetIdentity(p.current.stat).Mode
	setIdentity := assetIdentity(set.stat)
	record := publicationRecordData{SchemaVersion: 1, MigrationID: request.MigrationID, OperationID: rand.Text(), Path: request.Path,
		RecoveryIdentity: RootIdentity{Device: setIdentity.Device, Inode: setIdentity.Inode}, Before: publicationObservation(p.original, mode), After: publicationObservation(p.replacement, mode),
		OriginalIdentity: assetIdentity(p.current.stat), Direction: "forward", Phase: "prepared", Outcome: "pending"}
	return p.writeRecord(ctx, record)
}

func (p *publicationState) reconcileJournal(ctx context.Context) (bool, error) {
	j := p.journal
	candidate := j.candidate
	published, err := p.reconcileImage(ctx, candidate, j.parent, j.current, j.name, true)
	if err != nil {
		return false, err
	}
	if published {
		j.current, j.expected, j.record = candidate.pin, candidate.data, j.candidateRecord
	}
	j.candidate = nil
	return published, nil
}

// Journal retries consume only retained actual candidates, never parsed receipts.
// docs/adr/0095-durable-live-publication.md:120.
func (p *publicationState) writeRecord(ctx context.Context, record publicationRecordData) error {
	if err := p.check(ctx); err != nil {
		return err
	}
	j := p.journal
	data, err := encodePublicationRecord(ctx, record)
	if err != nil {
		return err
	}
	if !bytes.Equal(j.expected, data) {
		stage, pin, err := p.prepareImage(ctx, j.directory, j.parent, data, 0600, publicationRecordLimit)
		if err != nil {
			return err
		}
		if err := p.check(ctx); err != nil {
			return err
		}
		j.candidate, j.candidateRecord = &publicationCandidate{pin: pin, data: data}, record
		rename := p.operations.rename
		if rename == nil {
			rename = func(ctx context.Context, stage *filepublish.Stage, name string) error {
				return stage.Publish(ctx, name)
			}
		}
		renameErr := rename(ctx, stage, j.name)
		published, observeErr := p.reconcileJournal(ctx)
		if renameErr != nil || observeErr != nil || !published {
			return errors.Join(failure(1, "cannot confirm publication record rename"), observeErr, ctx.Err())
		}
	}
	if err := p.check(ctx); err != nil {
		return err
	}
	if err := p.w.sync(ctx, j.current); err != nil {
		return err
	}
	return p.w.sync(ctx, j.parent)
}
