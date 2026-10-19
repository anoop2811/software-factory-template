package installation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"golang.org/x/sys/unix"
)

type retentionPruneOps struct {
	beforeStep  func(context.Context, string) error
	afterUnlink func(context.Context, string) error
	afterStep   func(context.Context, string) error
}
type retentionRemoval struct {
	Path      string                    `json:"path"`
	Image     *Image                    `json:"image,omitempty"`
	Directory *installationfs.Directory `json:"directory,omitempty"`
	Journal   bool                      `json:"journal"`
}
type retentionProgress struct {
	SchemaVersion int                `json:"schema_version"`
	Kind          string             `json:"kind"`
	Phase         string             `json:"phase"`
	Snapshot      retentionSnapshot  `json:"snapshot"`
	Protect       string             `json:"protected_recovery"`
	Rows          []retentionRemoval `json:"rows"`
	Next          int                `json:"next"`
	Intent        bool               `json:"intent"`
}

func retentionRows(snapshot retentionSnapshot) ([]retentionRemoval, error) {
	if !ValidBackupID(snapshot.ID) || snapshot.Manifest.MigrationID != snapshot.ID || snapshot.Record.MigrationID != snapshot.ID || len(snapshot.Manifest.Entries) != len(roles) || snapshot.JournalImage == nil {
		return nil, conflict("invalid retention progress selection")
	}
	allowed := map[string]bool{"installation.manifest": true}
	for _, entry := range snapshot.Manifest.Entries {
		if !selectedPath(entry.Path) {
			return nil, conflict("unrecognized retention progress role")
		}
		if entry.Action == "replace" || entry.Action == "retire" {
			allowed["assets/"+entry.Path] = true
		}
	}
	metadata := map[string]any{"installation.manifest": snapshot.Manifest}
	if snapshot.Receipt != nil {
		allowed["retention.json"] = true
		metadata["retention.json"] = *snapshot.Receipt
	}
	if snapshot.Hold != nil {
		allowed["hold.json"] = true
		metadata["hold.json"] = *snapshot.Hold
	}
	if len(snapshot.Files) != len(allowed) {
		return nil, conflict("incomplete retention progress file selection")
	}
	for name, record := range metadata {
		data, err := encodeRecord(record)
		if err != nil {
			return nil, err
		}
		if !imageEqual(bytesImage(data, 0600), snapshot.Files[name], false) {
			return nil, conflict("retention progress metadata is not the recorded canonical value")
		}
	}
	journalData, err := encodeRecord(snapshot.Record)
	if err != nil {
		return nil, err
	}
	if !imageEqual(bytesImage(journalData, 0600), snapshot.JournalImage, false) {
		return nil, conflict("retention progress journal disagrees with its canonical value")
	}
	names := make([]string, 0, len(snapshot.Files))
	dirNames := map[string]bool{backupBase(snapshot.ID): true}
	for name, image := range snapshot.Files {
		if !allowed[name] || image == nil || image.Type != "regular" || image.Mode != unix.S_IFREG|0600 || image.Identity == nil || !digestPattern.MatchString(image.SHA256) {
			return nil, conflict("unsafe retention progress file")
		}
		names = append(names, name)
		parts := strings.Split(name, "/")
		for i := 1; i < len(parts); i++ {
			dirNames[backupBase(snapshot.ID)+"/"+strings.Join(parts[:i], "/")] = true
		}
	}
	priority := func(name string) int {
		if strings.HasPrefix(name, "assets/") {
			return 0
		}
		switch name {
		case "hold.json":
			return 1
		case "retention.json":
			return 2
		default:
			return 3
		}
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := priority(names[i]), priority(names[j])
		if a != b {
			return a < b
		}
		return names[i] < names[j]
	})
	rows := make([]retentionRemoval, 0, len(names)+len(snapshot.Directories)+1)
	for _, name := range names {
		rows = append(rows, retentionRemoval{Path: backupBase(snapshot.ID) + "/" + name, Image: snapshot.Files[name]})
	}
	directories := append([]installationfs.Directory{}, snapshot.Directories...)
	if len(directories) != len(dirNames) {
		return nil, conflict("incomplete retention progress directory selection")
	}
	seen := map[string]bool{}
	for _, directory := range directories {
		if !dirNames[directory.Path] || seen[directory.Path] || directory.Mode != unix.S_IFDIR|0700 {
			return nil, conflict("unsafe retention progress directory")
		}
		seen[directory.Path] = true
	}
	sort.Slice(directories, func(i, j int) bool {
		if len(directories[i].Path) != len(directories[j].Path) {
			return len(directories[i].Path) > len(directories[j].Path)
		}
		return directories[i].Path > directories[j].Path
	})
	for _, directory := range directories {
		pinned := directory
		rows = append(rows, retentionRemoval{Path: pinned.Path, Directory: &pinned})
	}
	rows = append(rows, retentionRemoval{Path: journalPath(snapshot.ID), Image: snapshot.JournalImage, Journal: true})
	return rows, nil
}
func readRetentionProgress(ctx context.Context, tree *installationfs.Tree) (retentionProgress, error) {
	image, data, err := observe(ctx, tree, retentionProgressPath, journalLimit)
	if err != nil {
		return retentionProgress{}, err
	}
	if image == nil || image.Mode != unix.S_IFREG|0600 {
		return retentionProgress{}, conflict("retention progress is missing or unsafe")
	}
	var progress retentionProgress
	if decodeRecord(ctx, data, &progress) != nil || progress.SchemaVersion != 1 || progress.Kind != "backup-retention-prune" || (progress.Phase != "pruning" && progress.Phase != "completed") || !ValidBackupID(progress.Protect) || progress.Protect == progress.Snapshot.ID {
		return progress, conflict("retention progress is unrecognized")
	}
	rows, err := retentionRows(progress.Snapshot)
	if err != nil {
		return progress, err
	}
	expected, _ := json.Marshal(rows)
	actual, _ := json.Marshal(progress.Rows)
	if string(expected) != string(actual) || progress.Next < 0 || progress.Next > len(rows) || progress.Intent && progress.Next == len(rows) || progress.Phase == "completed" && (progress.Next != len(rows) || progress.Intent) {
		return progress, conflict("retention progress selection is inconsistent")
	}
	return progress, nil
}

// Only a caller with fresh source/current/alternate proofs may invoke this private
// mutation engine. Progress alone never supplies those proofs.
// docs/adr/0106-go-backup-retention.md:89.
func executeRetentionPrune(ctx context.Context, tree *installationfs.Tree, snapshot retentionSnapshot, protect string, ops retentionPruneOps) (files int, size int64, returned error) {
	rows, err := retentionRows(snapshot)
	if err != nil {
		return 0, 0, err
	}
	if !ValidBackupID(protect) || protect == snapshot.ID {
		return 0, 0, conflict("the sole usable recovery set cannot be pruned")
	}
	progress := retentionProgress{SchemaVersion: 1, Kind: "backup-retention-prune", Phase: "pruning", Snapshot: snapshot, Protect: protect, Rows: rows}
	observed, _, err := observe(ctx, tree, retentionProgressPath, journalLimit)
	if err != nil {
		return 0, 0, err
	}
	if observed != nil {
		existing, err := readRetentionProgress(ctx, tree)
		if err != nil {
			return 0, 0, err
		}
		if existing.Phase == "pruning" {
			if existing.Snapshot.ID != snapshot.ID {
				return 0, 0, conflict("another backup cleanup must be reconciled first")
			}
			progress = existing
			progress.Protect = protect
		}
	}
	save := func() error {
		if err := writeRecord(ctx, tree, retentionProgressPath, progress, observed); err != nil {
			return err
		}
		var err error
		observed, _, err = observe(ctx, tree, retentionProgressPath, journalLimit)
		return err
	}
	if err = save(); err != nil {
		return 0, 0, err
	}
	for progress.Next < len(progress.Rows) {
		row := progress.Rows[progress.Next]
		wasIntended := progress.Intent
		progress.Intent = true
		if err = save(); err != nil {
			return files, size, err
		}
		if ops.beforeStep != nil {
			if err = ops.beforeStep(ctx, row.Path); err != nil {
				return files, size, err
			}
		}
		missing := false
		if row.Directory != nil {
			_, err = tree.Stat(ctx, row.Path)
			if errors.Is(err, os.ErrNotExist) {
				missing = true
				err = nil
			}
			if err != nil {
				return files, size, err
			}
			if !missing {
				err = tree.RemoveEmptyDirectory(ctx, row.Path, *row.Directory)
			}
		} else {
			current, _, inspectErr := observe(ctx, tree, row.Path, retentionLeafLimit(snapshot.ID, row.Path))
			if inspectErr != nil {
				return files, size, inspectErr
			}
			missing = current == nil
			if !missing {
				if !imageEqual(current, row.Image, true) {
					return files, size, conflict("retention entry changed before unlink")
				}
				parent, name, _, err := tree.Parent(ctx, row.Path, false)
				if err != nil {
					return files, size, err
				}
				if err = tree.Check(ctx); err != nil {
					return files, size, err
				}
				if err = parent.Root.Remove(name); err != nil {
					return files, size, err
				}
				if !row.Journal {
					files++
					size += current.Bytes
				}
				if ops.afterUnlink != nil {
					if err = ops.afterUnlink(ctx, row.Path); err != nil {
						return files, size, err
					}
				}
				if err = parent.Sync(ctx); err != nil {
					return files, size, err
				}
			}
		}
		if missing && !wasIntended {
			return files, size, conflict("retention entry disappeared without a durable prior intent")
		}
		if err != nil {
			return files, size, err
		}
		if missing {
			parent, _, _, err := tree.Parent(ctx, row.Path, false)
			if err != nil {
				return files, size, err
			}
			if err = parent.Sync(ctx); err != nil {
				return files, size, err
			}
		}
		if ops.afterStep != nil {
			if err = ops.afterStep(ctx, row.Path); err != nil {
				return files, size, err
			}
		}
		progress.Next++
		progress.Intent = false
		if err = save(); err != nil {
			return files, size, err
		}
	}
	progress.Phase = "completed"
	if err = save(); err != nil {
		return files, size, err
	}
	return files, size, tree.Check(ctx)
}

func retentionLeafLimit(id, path string) int64 {
	prefix := backupBase(id) + "/assets/"
	if strings.HasPrefix(path, prefix) {
		return leafLimit(strings.TrimPrefix(path, prefix))
	}
	return leafLimit(path)
}
