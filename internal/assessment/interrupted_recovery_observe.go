package assessment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/anoop2811/software-factory-template/internal/budget"
	"github.com/anoop2811/software-factory-template/internal/loop"
	"golang.org/x/sys/unix"
)

const interruptedHistoryLimit = 32 << 20

type interruptedBytes struct {
	pin  *writePin
	data []byte
}

type interruptedObservation struct {
	w                        *recoveryWriter
	operations               ops
	factory, parent, current *writePin
	saved, pending, records  *writePin
	recordPin                *writePin
	name                     string
	original, currentBytes   []byte
	recordBytes              []byte
	record                   publicationRecordData
	proposal                 InterruptedRecoveryProposal
	absent                   recoveryPins
	immutable                []interruptedBytes
}

func interruptedReadOps(operations ops) recoveryWriteOps {
	result := recoveryWriteOps{named: operations.named, close: operations.close}
	if operations.open != nil {
		result.open = func(_ context.Context, parent *os.File, name string, flags int, _ uint32) (*os.File, error) {
			return operations.open(parent, name, flags)
		}
	}
	if operations.read != nil {
		result.read = func(_ context.Context, file *os.File, buffer []byte) (int, error) {
			return operations.read(file, buffer)
		}
	}
	return result
}

func interruptedRequest(request InterruptedRecoveryRequest) (referenceAsset, error) {
	selected, err := selectedReferences([]string{request.Path})
	if err != nil {
		return referenceAsset{}, err
	}
	reference := selected[0]
	if !recoveryID(request.MigrationID) || !recoveryID(request.OperationID) ||
		(request.Direction != "forward" && request.Direction != "reverse") || !request.UnbridgedQuiescent ||
		len(request.Replacement) > assetLimit || request.AfterReference == reference.Reference ||
		publicationObservation(request.Replacement, reference.Reference.Mode) != request.AfterReference {
		return referenceAsset{}, failure(2, "invalid interrupted recovery request")
	}
	return reference, nil
}

// The proposal borrows only existing files and retains the complete selected set.
// docs/adr/0096-interrupted-publication-recovery.md:70.
func observeInterrupted(ctx context.Context, root string, request InterruptedRecoveryRequest, storage recoveryWriteOps, operations ops) (result *interruptedObservation, returned error) {
	reference, err := interruptedRequest(request)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	chain, err := openRoot(ctx, root)
	if err != nil {
		return nil, err
	}
	o := &interruptedObservation{w: &recoveryWriter{chain: chain, ops: storage}, operations: operations}
	defer func() {
		if result == nil {
			returned = errors.Join(o.w.close(), returned)
		}
	}()
	o.w.root = &writePin{file: chain.last(), stat: chain[len(chain)-1].identity}
	o.w.pins = append(o.w.pins, o.w.root)
	if !trustedDirectory(o.w.root.stat, false) {
		return nil, failure(2, "unsafe interrupted recovery root")
	}
	o.factory, err = o.w.directory(ctx, o.w.root, ".factory", false, false)
	if err != nil {
		return nil, err
	}
	if _, err := o.w.existingFile(ctx, o.factory, "runtime-transition.lock", unix.O_RDONLY, 0, true); err != nil {
		return nil, err
	}
	activity, err := o.w.directory(ctx, o.factory, "runtime-activity", true, false)
	if err != nil {
		return nil, err
	}
	entries, class := recoveryEntries(ctx, activity.file, 1)
	if class != "" {
		return nil, interruptedClass(ctx, class)
	}
	if len(entries) != 0 {
		return nil, failure(2, "interrupted recovery activity remains")
	}
	o.pending, err = o.w.existingFile(ctx, o.factory, publicationPending, unix.O_RDONLY, 0, true)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"budget.json", "loops.json"} {
		if err := o.history(ctx, name); err != nil {
			return nil, err
		}
	}
	backups, err := o.w.directory(ctx, o.factory, "backups", true, false)
	if err != nil {
		return nil, err
	}
	// Namespace metadata has its own bound; it never consumes saved-set capacity.
	// docs/adr/0095-durable-live-publication.md:87.
	entries, class = recoveryEntries(ctx, backups.file, 65)
	if class != "" {
		return nil, interruptedClass(ctx, class)
	}
	sets := 0
	for _, entry := range entries {
		if entry.Name() != publicationNamespace {
			sets++
		}
	}
	if sets > 64 {
		return nil, failure(2, "interrupted recovery inventory exceeds its bound")
	}
	o.records, err = o.w.directory(ctx, backups, publicationNamespace, true, false)
	if err != nil {
		return nil, err
	}
	var pins recoveryPins
	rows, class := publicationRows(ctx, o.records.file, operations, &pins)
	o.retain(pins)
	if class == "" {
		class = publicationRowsClass(rows)
	}
	if class != "" {
		return nil, interruptedClass(ctx, class)
	}
	selectedRecord := false
	for _, row := range rows {
		if row.MigrationID == request.MigrationID {
			selectedRecord = row.OperationID == request.OperationID && row.Path == request.Path
		} else if !publicationTerminal(row.Phase) {
			return nil, failure(2, "other interrupted publication evidence remains")
		}
	}
	if !selectedRecord {
		return nil, failure(2, "selected interrupted publication is unavailable")
	}
	for _, pin := range o.w.pins {
		if pin.parent == o.records.file && pin.name == request.MigrationID+".json" {
			o.recordPin = pin
		}
	}
	if o.recordPin == nil {
		return nil, failure(2, "selected interrupted publication is unavailable")
	}
	o.recordBytes, err = o.w.read(ctx, o.recordPin, publicationRecordLimit)
	if err != nil {
		return nil, err
	}
	var valid bool
	o.record, valid = parsePublicationRecord(ctx, o.recordBytes, request.MigrationID)
	if !valid || o.record.OperationID != request.OperationID || o.record.Path != request.Path || o.record.After != request.AfterReference {
		return nil, failure(2, "interrupted publication image references disagree")
	}
	pins = nil
	row, manifest, err := inspectRecoverySetPinned(ctx, backups.file, request.MigrationID, operations, &pins)
	o.retain(pins)
	if err != nil {
		return nil, err
	}
	if row.Classification != "integrity_checked" {
		return nil, interruptedClass(ctx, row.Classification)
	}
	if row.Held == nil || *row.Held {
		return nil, failure(2, "interrupted publication recovery set is held")
	}
	contains := false
	for _, asset := range manifest.assets {
		contains = contains || asset == reference
	}
	if !contains {
		return nil, failure(2, "saved set does not include the selected image")
	}
	set, err := o.w.directory(ctx, backups, request.MigrationID, true, false)
	if err != nil {
		return nil, err
	}
	identity := assetIdentity(set.stat)
	if o.record.RecoveryIdentity != (RootIdentity{Device: identity.Device, Inode: identity.Inode}) {
		return nil, failure(2, "saved set identity disagrees with the selected record")
	}
	savedParent, savedName, err := o.w.catalogParent(ctx, set, "files/"+request.Path, true, false, nil)
	if err != nil {
		return nil, err
	}
	o.saved, err = o.w.existingFile(ctx, savedParent, savedName, unix.O_RDONLY, assetLimit, true)
	if err != nil {
		return nil, err
	}
	o.original, err = o.w.read(ctx, o.saved, assetLimit)
	if err != nil {
		return nil, err
	}
	if !referenceBytes(o.original, reference) {
		return nil, failure(2, "saved original disagrees with the compiled reference")
	}
	o.parent, o.name, err = o.w.catalogParent(ctx, o.w.root, request.Path, false, false, nil)
	if err != nil {
		return nil, err
	}
	o.current, err = o.w.existingFile(ctx, o.parent, o.name, unix.O_RDONLY, assetLimit, false)
	if err != nil {
		return nil, err
	}
	o.currentBytes, err = o.w.read(ctx, o.current, assetLimit)
	if err != nil {
		return nil, err
	}
	actual := publicationObservation(o.currentBytes, assetIdentity(o.current.stat).Mode)
	image, valid := interruptedImage(o.record, assetIdentity(o.current.stat), actual, request.Direction)
	if !valid {
		return nil, failure(2, "current image is ineligible for the selected recovery direction")
	}
	known := reference.Reference
	if image == "after" {
		known = request.AfterReference
	}
	rootIdentity := assetIdentity(o.w.root.stat)
	o.proposal = InterruptedRecoveryProposal{SchemaVersion: 1, Mode: "interrupted_recovery_proposal",
		Scope: "g2-budget-loop-single-publication", Purpose: "fresh_current_image_resolution",
		MigrationID: request.MigrationID, OperationID: request.OperationID, Path: request.Path, Direction: request.Direction,
		CurrentImage: image, RootIdentity: RootIdentity{Device: rootIdentity.Device, Inode: rootIdentity.Inode},
		Current: AdoptionAsset{Path: request.Path, Reference: known, Observed: actual, Identity: assetIdentity(o.current.stat)},
		Record:  publicationObservation(o.recordBytes, "0600"), RecordIdentity: assetIdentity(o.recordPin.stat), PendingIdentity: assetIdentity(o.pending.stat), Recovery: row,
		StateCompatibility: "g2-state-v1_checked", UnbridgedQuiescence: "operator_affirmed"}
	for _, pin := range o.w.pins {
		if pin.stat.Mode&unix.S_IFMT != unix.S_IFREG || pin == o.current || pin == o.recordPin || pin == o.pending {
			continue
		}
		limit := int64(assetLimit)
		if pin.stat.Size > limit {
			limit = interruptedHistoryLimit
		}
		data, err := o.w.read(ctx, pin, limit)
		if err != nil {
			return nil, err
		}
		o.immutable = append(o.immutable, interruptedBytes{pin: pin, data: data})
	}
	if err := o.check(ctx, o.w); err != nil {
		return nil, err
	}
	o.proposal.ProposalDigest, err = o.digest(ctx, request)
	if err != nil {
		return nil, err
	}
	return o, nil
}

func interruptedClass(ctx context.Context, class string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	code := 2
	if class == "assessment_error" {
		code = 1
	}
	return failure(code, "interrupted recovery evidence is unavailable or unsafe")
}

func (o *interruptedObservation) retain(pins recoveryPins) {
	for _, pin := range pins {
		if pin.file != nil {
			o.w.pins = append(o.w.pins, &writePin{parent: pin.parent, file: pin.file, name: pin.name, stat: pin.stat})
		}
	}
}

// History is parsed without store helpers, preserving positive and absent state.
// docs/adr/0096-interrupted-publication-recovery.md:137.
func (o *interruptedObservation) history(ctx context.Context, name string) error {
	_, err := o.w.named(o.factory.file, name)
	if errors.Is(err, unix.ENOENT) {
		o.absent = append(o.absent, recoveryPin{parent: o.factory.file, name: name, missing: true})
		return nil
	}
	if err != nil {
		return observationError(err, "cannot observe interrupted recovery state")
	}
	pin, err := o.w.existingFile(ctx, o.factory, name, unix.O_RDONLY, interruptedHistoryLimit, true)
	if err != nil {
		return err
	}
	data, err := o.w.read(ctx, pin, interruptedHistoryLimit)
	if err != nil {
		return err
	}
	var unresolved bool
	if name == "budget.json" {
		history, parseErr := budget.ParseHistory(ctx, bytes.NewReader(data))
		err = parseErr
		if err == nil {
			unresolved, err = history.HasUnresolved(ctx)
		}
	} else {
		history, parseErr := loop.ParseHistory(ctx, bytes.NewReader(data))
		err = parseErr
		if err == nil {
			unresolved, err = history.HasUnresolved(ctx)
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil || unresolved {
		return failure(2, "interrupted recovery state is unreadable or unresolved")
	}
	return nil
}

// Recorded identities restrict eligibility; they never reconstruct old authority.
// docs/adr/0096-interrupted-publication-recovery.md:160.
func interruptedImage(record publicationRecordData, current AssetIdentity, observed Observation, direction string) (string, bool) {
	prepared := func(known *AssetIdentity) bool {
		if known == nil {
			return false
		}
		actual := current
		actual.CtimeSeconds, actual.CtimeNanoseconds = known.CtimeSeconds, known.CtimeNanoseconds
		return actual == *known
	}
	full := func(known *AssetIdentity) bool { return known != nil && current == *known }
	if observed == record.Before {
		if direction != "reverse" {
			return "", false
		}
		switch record.Phase {
		case "prepared", "forward_prepared", "aborted":
			return "before", current == record.OriginalIdentity
		case "reverse_prepared":
			return "before", prepared(record.CandidateIdentity)
		case "restored":
			return "before", full(record.RestoredIdentity)
		}
	}
	if observed == record.After {
		switch record.Phase {
		case "forward_prepared":
			return "after", prepared(record.ForwardIdentity) && prepared(record.CandidateIdentity)
		case "forward_published":
			return "after", full(record.ForwardIdentity)
		case "reverse_prepared":
			return "after", direction == "reverse" && full(record.ForwardIdentity)
		case "forward_completed":
			return "after", direction == "forward" && full(record.ForwardIdentity)
		}
	}
	return "", false
}

// Reconciliation passes its qualified writer view, never stale active metadata.
// docs/adr/0096-interrupted-publication-recovery.md:192.
func (o *interruptedObservation) check(ctx context.Context, writer *recoveryWriter) error {
	if err := writer.check(ctx); err != nil {
		return err
	}
	if err := checkRecoveryRoot(ctx, writer.chain, o.operations); err != nil {
		return err
	}
	for _, pin := range writer.pins {
		current, err := o.operations.stat(pin.file)
		if err != nil {
			return observationError(err, "cannot observe interrupted recovery metadata")
		}
		if !writeUnchanged(pin.stat, current) {
			return failure(2, "interrupted recovery metadata changed")
		}
		if pin.parent != nil {
			current, err = o.operations.lookup(pin.parent, pin.name)
			if err != nil {
				return observationError(err, "cannot observe interrupted recovery metadata")
			}
			if !writeUnchanged(pin.stat, current) {
				return failure(2, "interrupted recovery metadata changed")
			}
		}
	}
	if err := o.absent.check(ctx, o.operations); err != nil {
		return err
	}
	for _, value := range o.immutable {
		data, err := writer.read(ctx, value.pin, interruptedHistoryLimit)
		if err != nil {
			return err
		}
		if !bytes.Equal(data, value.data) {
			return failure(2, "interrupted recovery input changed")
		}
	}
	return ctx.Err()
}

type interruptedMetadata struct {
	Identity AssetIdentity `json:"identity"`
	Owner    uint32        `json:"owner"`
	Group    uint32        `json:"group"`
	Links    string        `json:"links"`
	Type     uint32        `json:"type"`
}

func interruptedStat(stat unix.Stat_t) interruptedMetadata {
	return interruptedMetadata{Identity: assetIdentity(stat), Owner: stat.Uid, Group: stat.Gid,
		Links: fmt.Sprint(stat.Nlink), Type: uint32(stat.Mode & unix.S_IFMT)}
}

// Consent excludes self-induced access times and unrelated system listing times.
// docs/adr/0096-interrupted-publication-recovery.md:98.
func (o *interruptedObservation) digest(ctx context.Context, request InterruptedRecoveryRequest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	type evidence struct {
		Path     string              `json:"path"`
		Metadata interruptedMetadata `json:"metadata"`
		Bytes    *Observation        `json:"bytes"`
	}
	values := make([]evidence, 0, len(o.w.pins))
	paths := map[*os.File]string{o.w.root.file: "."}
	for _, pin := range o.w.pins {
		path := "."
		if pin != o.w.root {
			parent, present := paths[pin.parent]
			if !present {
				return "", failure(1, "cannot bind interrupted recovery ancestry")
			}
			path = parent + "/" + pin.name
		}
		paths[pin.file] = path
		value := evidence{Path: path, Metadata: interruptedStat(pin.stat)}
		if pin.stat.Mode&unix.S_IFMT == unix.S_IFREG {
			data, err := o.w.read(ctx, pin, interruptedHistoryLimit)
			if err != nil {
				return "", err
			}
			observation := publicationObservation(data, value.Metadata.Identity.Mode)
			value.Bytes = &observation
		}
		values = append(values, value)
	}
	sort.SliceStable(values, func(i, j int) bool { return values[i].Path < values[j].Path })
	type ancestor struct {
		Device, Inode, Mode, Owner, Group string
	}
	ancestors := make([]ancestor, 0, len(o.w.chain)-1)
	for _, directory := range o.w.chain[:len(o.w.chain)-1] {
		stat := directory.identity
		ancestors = append(ancestors, ancestor{Device: fmt.Sprint(stat.Dev), Inode: fmt.Sprint(stat.Ino),
			Mode: fmt.Sprint(stat.Mode), Owner: fmt.Sprint(stat.Uid), Group: fmt.Sprint(stat.Gid)})
	}
	absent := make([]string, 0, len(o.absent))
	for _, pin := range o.absent {
		absent = append(absent, paths[pin.parent]+"/"+pin.name)
	}
	sort.Strings(absent)
	payload := struct {
		Proposal    InterruptedRecoveryProposal
		After       Observation
		Replacement Observation
		Quiescent   bool
		Ancestors   []ancestor
		Evidence    []evidence
		Absent      []string
	}{Proposal: o.proposal, After: request.AfterReference,
		Replacement: publicationObservation(request.Replacement, request.AfterReference.Mode), Quiescent: request.UnbridgedQuiescent,
		Ancestors: ancestors, Evidence: values, Absent: absent}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", failure(1, "cannot encode interrupted recovery proposal")
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("software-factory/interrupted-recovery/v1\n"))
	_, _ = hash.Write(encoded)
	return hex.EncodeToString(hash.Sum(nil)), nil
}
