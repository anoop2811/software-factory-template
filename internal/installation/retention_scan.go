package installation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"golang.org/x/sys/unix"
)

func retentionIDs(ctx context.Context, tree *installationfs.Tree) (ids []string, returned error) {
	parent, _, _, err := tree.Parent(ctx, retentionProgressPath, false)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	directory, err := parent.Root.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() { returned = errors.Join(returned, directory.Close()) }()
	entries, err := directory.ReadDir(18)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Name() != ".retention.json" {
			ids = append(ids, entry.Name())
		}
	}
	if len(ids) > 16 {
		return nil, conflict("backup inventory exceeds the supported operation quota")
	}
	sort.Strings(ids)
	return ids, tree.Check(ctx)
}

// Closed snapshots preserve unexpected entries instead of recursively deleting.
// docs/adr/0106-go-backup-retention.md:82.
func snapshotRetentionSet(ctx context.Context, tree *installationfs.Tree, id string) (result retentionSnapshot, returned error) {
	result = retentionSnapshot{ID: id, Files: map[string]*Image{}, Directories: []installationfs.Directory{}}
	if !ValidBackupID(id) {
		return result, conflict("unrecognized recovery identifier")
	}
	var err error
	result.JournalImage, _, err = observe(ctx, tree, journalPath(id), journalLimit)
	if err != nil {
		return result, err
	}
	result.Record, err = readJournalEvidence(ctx, tree, id, false)
	if err != nil {
		return result, err
	}
	if result.Record.Phase != "committed" || result.Record.Outcome != "completed" {
		return result, conflict("recovery transaction is incomplete")
	}
	if result.Record.Profile != "v0.1.6" && result.Record.Profile != "bash-baseline" && result.Record.Profile != "go-hybrid-v1" {
		return result, conflict("unrecognized recovery profile")
	}
	base := backupBase(id)
	manifestImage, data, err := observe(ctx, tree, base+"/installation.manifest", journalLimit)
	if err != nil {
		return result, err
	}
	if manifestImage == nil || manifestImage.Mode != unix.S_IFREG|0600 {
		return result, conflict("backup manifest is missing or unsafe")
	}
	if err = decodeRecord(ctx, data, &result.Manifest); err != nil {
		return result, conflict("backup manifest is unrecognized")
	}
	manifest := result.Manifest
	if manifest.SchemaVersion != 1 || manifest.Kind != "installation-before-images" || manifest.MigrationID != id || manifest.Profile != result.Record.Profile || !knownRoleEntries(manifest.Entries) || len(result.Record.Entries) != len(manifest.Entries) {
		return result, conflict("backup manifest is incomplete")
	}
	expected := map[string]bool{"installation.manifest": true}
	seen := map[string]bool{}
	for _, row := range manifest.Entries {
		if seen[row.Path] || !selectedPath(row.Path) {
			return result, conflict("backup has unrecognized or repeated roles")
		}
		seen[row.Path] = true
		var recorded *journalEntry
		for i := range result.Record.Entries {
			if result.Record.Entries[i].Path == row.Path {
				recorded = &result.Record.Entries[i]
				break
			}
		}
		if recorded == nil || recorded.Action != row.Action || !imageEqual(recorded.Before, row.Before, true) || !imageEqual(recorded.ExpectedAfter, row.ExpectedAfter, false) {
			return result, conflict("backup and transaction disagree")
		}
		switch row.Action {
		case "replace", "retire":
			if row.Before == nil {
				return result, conflict("backup omits a required before-image")
			}
			expected["assets/"+row.Path] = true
		case "retain", "create":
		default:
			return result, conflict("backup action is unrecognized")
		}
	}
	receiptImage, receiptData, err := observe(ctx, tree, base+"/retention.json", journalLimit)
	if err != nil {
		return result, err
	}
	if receiptImage != nil {
		var receipt retentionReceipt
		if receiptImage.Mode != unix.S_IFREG|0600 || decodeRecord(ctx, receiptData, &receipt) != nil || receipt.SchemaVersion != 1 || receipt.Kind != "installation-retention" || receipt.MigrationID != id || receipt.Profile != manifest.Profile || receipt.Saved == nil || !imageEqual(receipt.Manifest, manifestImage, true) {
			return result, conflict("retention metadata is changed or unrecognized")
		}
		if result.Record.RetentionReceipt == nil || !imageEqual(result.Record.RetentionReceipt, receiptImage, true) {
			return result, conflict("retention metadata differs from its saved observation")
		}
		result.Receipt = &receipt
		expected["retention.json"] = true
	} else if result.Record.RetentionReceipt != nil {
		return result, conflict("retention metadata was removed")
	}
	holdImage, holdData, err := observe(ctx, tree, base+"/hold.json", 4096)
	if err != nil {
		return result, err
	}
	if holdImage != nil {
		var hold retentionHold
		if holdImage.Mode != unix.S_IFREG|0600 || decodeRecord(ctx, holdData, &hold) != nil || hold.SchemaVersion != 1 || hold.Kind != "backup-hold" || hold.MigrationID != id {
			return result, conflict("backup hold is unrecognized")
		}
		result.Hold = &hold
		expected["hold.json"] = true
	}
	allowedDirectories := map[string]bool{"": true}
	for name := range expected {
		parts := strings.Split(name, "/")
		for i := 1; i < len(parts); i++ {
			allowedDirectories[strings.Join(parts[:i], "/")] = true
		}
	}
	var walk func(string) error
	walk = func(relative string) (returned error) {
		path := base
		if relative != "" {
			path += "/" + relative
		}
		node, _, _, err := tree.Parent(ctx, path+"/.retention-probe", false)
		if err != nil {
			return err
		}
		stat, err := tree.Stat(ctx, path)
		if err != nil || stat.Mode != unix.S_IFDIR|0700 {
			return errors.Join(conflict("backup directory is unsafe"), err)
		}
		directory, err := node.Root.Open(".")
		if err != nil {
			return err
		}
		defer func() { returned = errors.Join(returned, directory.Close()) }()
		entries, err := directory.ReadDir(4097)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if len(entries) > 4096 {
			return conflict("backup directory exceeds its inventory limit")
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if err = ctx.Err(); err != nil {
				return err
			}
			name := entry.Name()
			if relative != "" {
				name = relative + "/" + name
			}
			metadata, err := tree.Stat(ctx, base+"/"+name)
			if err != nil {
				return err
			}
			if metadata.Mode&unix.S_IFMT == unix.S_IFDIR {
				if !allowedDirectories[name] {
					return conflict("backup contains an unclassified directory")
				}
				if err = walk(name); err != nil {
					return err
				}
				continue
			}
			if !expected[name] {
				return conflict("backup contains an unclassified entry")
			}
			limit := int64(journalLimit)
			if strings.HasPrefix(name, "assets/") {
				limit = leafLimit(strings.TrimPrefix(name, "assets/"))
			}
			image, _, err := observe(ctx, tree, base+"/"+name, limit)
			if err != nil {
				return err
			}
			if image == nil || image.Mode != unix.S_IFREG|0600 {
				return conflict("backup contains an unsafe entry")
			}
			if strings.HasPrefix(name, "assets/") {
				original := strings.TrimPrefix(name, "assets/")
				var before *Image
				for _, row := range manifest.Entries {
					if row.Path == original {
						before = row.Before
						break
					}
				}
				if before == nil || image.SHA256 != before.SHA256 || image.Bytes != before.Bytes {
					return conflict("saved before-image was edited")
				}
				if result.Receipt != nil && !imageEqual(image, result.Receipt.Saved[original], true) {
					return conflict("saved before-image identity changed")
				}
			}
			result.Files[name] = image
		}
		return tree.Check(ctx)
	}
	if err = walk(""); err != nil {
		return result, err
	}
	if len(result.Files) != len(expected) {
		return result, conflict("backup is incomplete")
	}
	if result.Receipt != nil {
		payloadCount := 0
		for name := range expected {
			if strings.HasPrefix(name, "assets/") {
				payloadCount++
			}
		}
		if len(result.Receipt.Saved) != payloadCount {
			return result, conflict("retention metadata has an incomplete saved inventory")
		}
	}
	parents, err := tree.Parents(ctx)
	if err != nil {
		return result, err
	}
	for _, parent := range parents {
		if parent.Path == base || strings.HasPrefix(parent.Path, base+"/") {
			result.Directories = append(result.Directories, parent)
		}
	}
	return result, tree.Check(ctx)
}

func snapshotCounts(snapshot retentionSnapshot) (int, int64) {
	count := 0
	var total int64
	for _, image := range snapshot.Files {
		count++
		total += image.Bytes
	}
	return count, total
}
func reportSet(snapshot retentionSnapshot, action, reason string) RetentionSet {
	files, size := snapshotCounts(snapshot)
	return RetentionSet{MigrationID: snapshot.ID, Path: backupBase(snapshot.ID), Action: action, Reason: reason, Files: &files, Bytes: &size, NextAction: "Keep this recovery set for reference; no automatic activation."}
}
func unknownRetentionSet(id, reason string) RetentionSet {
	return RetentionSet{MigrationID: id, Path: backupBase(id), Action: "retain", Reason: reason, NextAction: "Inspect the preserved recovery set; reconcile its reported conflict before requesting cleanup."}
}
func inspectUnknownRetention(ctx context.Context, tree *installationfs.Tree, snapshot *retentionSnapshot) {
	metadata, err := tree.Stat(ctx, backupBase(snapshot.ID))
	if err == nil {
		snapshot.Unclassified = &metadata
	}
}
func retentionReason(err error) string {
	var failure *Failure
	if errors.As(err, &failure) {
		return failure.Reason
	}
	if errors.Is(err, installationfs.ErrUnsafe) {
		return "backup contains an unsafe path or entry"
	}
	if errors.Is(err, os.ErrNotExist) {
		return "backup or transaction evidence is missing"
	}
	return "backup evidence could not be inspected safely"
}
func matchingManifestEntry(manifest backupManifest, path string) (journalEntry, error) {
	for _, entry := range manifest.Entries {
		if entry.Path == path {
			return entry, nil
		}
	}
	return journalEntry{}, fmt.Errorf("%w: missing backup role", installationfs.ErrUnsafe)
}
