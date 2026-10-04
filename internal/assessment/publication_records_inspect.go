package assessment

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

func publicationRows(ctx context.Context, parent *os.File, operations ops, pins *recoveryPins) ([]PublicationRecord, string) {
	entries, class := recoveryEntries(ctx, parent, 64)
	if class != "" {
		return nil, class
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	rows := make([]PublicationRecord, 0, len(entries))
	for _, entry := range entries {
		if ctx.Err() != nil {
			return nil, "assessment_error"
		}
		name := entry.Name()
		id := strings.TrimSuffix(name, ".json")
		row := PublicationRecord{MigrationID: recoveryDisplayName(id), Classification: "unrecognized", NextAction: "inspect_preserved_publication"}
		data, pin, fileClass := readRecoveryFile(ctx, parent, name, publicationRecordLimit, operations)
		if pin.file != nil {
			*pins = append(*pins, pin)
		}
		if fileClass != "" {
			if fileClass == "missing" {
				fileClass = "unsafe"
			}
			row.Classification = fileClass
		} else if strings.HasSuffix(name, ".json") && recoveryID(id) {
			record, valid := parsePublicationRecord(ctx, data, id)
			if valid {
				row = PublicationRecord{MigrationID: record.MigrationID, OperationID: record.OperationID, Path: record.Path,
					Classification: "record_checked", Phase: record.Phase, Outcome: record.Outcome, NextAction: "review_inert_publication_evidence"}
			}
		}
		rows = append(rows, row)
	}
	return rows, ""
}

func publicationRowsClass(rows []PublicationRecord) string {
	class := ""
	for _, row := range rows {
		next := row.Classification
		if next == "record_checked" {
			continue
		}
		if next == "unrecognized" {
			next = "incomplete"
		}
		class = mergeRecoveryClass(class, next)
	}
	return class
}

// Only the exact private metadata namespace is excluded from the saved-set quota.
// Its bounded contents remain validated rather than silently skipped.
// docs/adr/0095-durable-live-publication.md:87.
func recoveryStorageEntries(ctx context.Context, parent *os.File, operations ops, pins *recoveryPins) ([]os.DirEntry, string) {
	entries, class := recoveryEntries(ctx, parent, 65)
	if class != "" {
		return nil, class
	}
	saved := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() != publicationNamespace {
			saved = append(saved, entry)
			continue
		}
		pin, directoryClass := openRecoveryDirectory(ctx, parent, publicationNamespace, false, operations)
		if pin.file != nil {
			*pins = append(*pins, pin)
		}
		if directoryClass != "" {
			if directoryClass == "missing" {
				directoryClass = "unsafe"
			}
			class = mergeRecoveryClass(class, directoryClass)
			continue
		}
		rows, recordClass := publicationRows(ctx, pin.file, operations, pins)
		class = mergeRecoveryClass(class, recordClass)
		class = mergeRecoveryClass(class, publicationRowsClass(rows))
	}
	if len(saved) > 64 {
		return nil, "limit_exceeded"
	}
	return saved, class
}

func inspectPublications(ctx context.Context, root string, operations ops) (result PublicationInventory, returned error) {
	result = PublicationInventory{SchemaVersion: 1, Mode: "inspect_publications", RootStatus: "absent", Complete: true,
		PendingStatus: "absent", Records: []PublicationRecord{}}
	chain, err := openRoot(ctx, root)
	if err != nil {
		return PublicationInventory{}, err
	}
	var pins recoveryPins
	defer func() {
		if !recoveryRootValid(chain) || !pins.valid() {
			// Discard changed observations while retaining prior operational failure.
			// docs/adr/0095-durable-live-publication.md:235.
			status := result.Status()
			result.RootStatus, result.Complete = "unsafe", false
			if status == 1 || returned != nil && ErrorStatus(returned) == 1 {
				result.RootStatus = "assessment_error"
			}
			result.Records, result.RecordCount = []PublicationRecord{}, 0
		}
		if err := closeRecoveryPlan(operations, pins, chain); err != nil {
			returned = errors.Join(failure(1, "cannot close publication observations"), returned)
		}
	}()
	factory, class := openRecoveryDirectory(ctx, chain.last(), ".factory", true, operations)
	if factory.file != nil {
		pins = append(pins, factory)
	}
	if class != "" {
		if class != "missing" {
			result.RootStatus, result.Complete, result.PendingStatus = class, false, "unknown"
		}
		return result, ctx.Err()
	}
	stat, pendingErr := named(factory.file, publicationPending)
	switch {
	case errors.Is(pendingErr, unix.ENOENT):
		pins = append(pins, recoveryPin{parent: factory.file, name: publicationPending, missing: true})
	case pendingErr != nil:
		result.PendingStatus = classification(pendingErr)
	case !recoveryFile(stat, 0):
		result.PendingStatus = "unsafe"
	default:
		result.PendingStatus = "present"
		pins = append(pins, recoveryPin{parent: factory.file, name: publicationPending, stat: stat})
	}
	parent := factory.file
	for _, name := range []string{"backups", publicationNamespace} {
		pin, directoryClass := openRecoveryDirectory(ctx, parent, name, false, operations)
		if pin.file != nil {
			pins = append(pins, pin)
		}
		if directoryClass != "" {
			if directoryClass != "missing" {
				result.RootStatus, result.Complete = directoryClass, false
			}
			return result, ctx.Err()
		}
		parent = pin.file
	}
	result.RootStatus = "inspected"
	result.Records, class = publicationRows(ctx, parent, operations, &pins)
	if class != "" {
		result.RootStatus, result.Complete = class, false
		result.Records = []PublicationRecord{}
	}
	result.RecordCount = len(result.Records)
	return result, ctx.Err()
}
