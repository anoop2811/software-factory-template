package installation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/filepublish"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
)

const journalLimit = 2 << 20

type journalEntry struct {
	Path          string `json:"path"`
	Action        string `json:"action"`
	Phase         string `json:"phase"`
	Before        *Image `json:"before"`
	ExpectedAfter *Image `json:"expected_after"`
	ObservedAfter *Image `json:"observed_after"`
	PreparedAfter *Image `json:"prepared_after"`
}
type checkRecord struct {
	Name                 string `json:"name"`
	Phase                string `json:"phase"`
	PID                  *int   `json:"pid"`
	Outcome              string `json:"outcome"`
	OwnershipUnconfirmed bool   `json:"ownership_unconfirmed"`
}
type journal struct {
	SchemaVersion int            `json:"schema_version"`
	Kind          string         `json:"kind"`
	MigrationID   string         `json:"migration_id"`
	Purpose       string         `json:"purpose"`
	Direction     string         `json:"direction"`
	Phase         string         `json:"phase"`
	Profile       string         `json:"profile"`
	Target        string         `json:"target"`
	PlanDigest    string         `json:"plan_digest"`
	Entries       []journalEntry `json:"entries"`
	Checks        []checkRecord  `json:"checks"`
	Outcome       string         `json:"outcome"`
	Pending       *Image         `json:"pending"`
}
type backupManifest struct {
	SchemaVersion int            `json:"schema_version"`
	Kind          string         `json:"kind"`
	MigrationID   string         `json:"migration_id"`
	Profile       string         `json:"profile"`
	Entries       []journalEntry `json:"entries"`
}

// Journal and before-images are descriptive evidence; fresh images confer authority.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:326.
func decodeRecord(ctx context.Context, data []byte, result any) error {
	if len(data) > journalLimit {
		return conflict("installation record exceeds limit")
	}
	if _, err := jsonvalue.DecodeUnique(ctx, data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(result); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return conflict("trailing installation record")
	}
	return ctx.Err()
}
func journalPath(id string) string { return ".factory/installation-transactions/" + id + ".json" }
func readJournalEvidence(ctx context.Context, tree *installationfs.Tree, id string, permitUnknown bool) (journal, error) {
	image, data, err := observe(ctx, tree, journalPath(id), journalLimit)
	if err != nil {
		return journal{}, err
	}
	if image == nil || image.Mode != 0100600 {
		return journal{}, conflict("installation journal unavailable")
	}
	value, uniqueErr := jsonvalue.DecodeUnique(ctx, data)
	if uniqueErr != nil {
		return journal{}, uniqueErr
	}
	object, objectOK := value.(map[string]any)
	if !objectOK {
		return journal{}, conflict("invalid installation journal object")
	}
	if _, present := object["pending"]; !present {
		return journal{}, conflict("installation pending observation is missing")
	}
	var record journal
	if err := decodeRecord(ctx, data, &record); err != nil {
		return journal{}, err
	}
	if record.SchemaVersion != 1 || record.Kind != "installation-transaction" || record.MigrationID != id || len(record.Entries) == 0 || len(record.Entries) > 512 || len(record.Checks) > 64 || !digestPattern.MatchString(record.PlanDigest) {
		return journal{}, conflict("invalid installation journal")
	}
	if record.Purpose != "upgrade" && record.Purpose != "rollback" && record.Purpose != "recovery" {
		return journal{}, conflict("invalid installation purpose")
	}
	if record.Direction != "forward" && record.Direction != "reverse" {
		return journal{}, conflict("invalid installation direction")
	}
	switch record.Phase {
	case "prepared", "applying", "checking", "metadata_prepared", "committed":
	default:
		return journal{}, conflict("unsupported installation phase")
	}
	if record.Pending != nil && (record.Pending.Type != "regular" || record.Pending.Mode != 0100600 || record.Pending.Bytes != 0 || record.Pending.Identity == nil) {
		return journal{}, conflict("invalid installation pending observation")
	}
	seen := map[string]bool{}
	for _, entry := range record.Entries {
		if seen[entry.Path] || !selectedPath(entry.Path) {
			return journal{}, conflict("invalid installation journal selection")
		}
		seen[entry.Path] = true
		switch entry.Action {
		case "create", "replace", "retire", "retain":
		default:
			return journal{}, conflict("invalid installation journal action")
		}
		switch entry.Phase {
		case "prepared", "applied", "reverse_prepared", "reversed":
		default:
			return journal{}, conflict("invalid installation row phase")
		}
	}
	for _, check := range record.Checks {
		if check.Name != "candidate-help" && check.Name != "installed-assets" && check.Name != "native-gate-proof" && check.Name != "state-compatibility" {
			return journal{}, conflict("unknown installation check")
		}
		if check.PID != nil && *check.PID <= 0 {
			return journal{}, conflict("invalid installation check process")
		}
		if check.Phase != "prepared" && check.Phase != "checking" && check.Phase != "complete" {
			return journal{}, conflict("invalid installation check phase")
		}
		if !permitUnknown && (check.OwnershipUnconfirmed || check.Phase == "checking") {
			return journal{}, conflict("installation check ownership requires operator reconciliation")
		}
	}
	return record, nil
}
func selectedPath(path string) bool {
	for _, row := range roles {
		if row.Path == path {
			return true
		}
	}
	return false
}
func imageEqual(a, b *Image, identities bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Type != b.Type || a.Mode != b.Mode || a.SHA256 != b.SHA256 || a.Bytes != b.Bytes {
		return false
	}
	return !identities || a.Identity != nil && b.Identity != nil && *a.Identity == *b.Identity
}
func leafLimit(path string) int64 {
	if path == ".factory/bin/factory-runtime" {
		return 256 << 20
	}
	if strings.HasSuffix(path, "installation.manifest") || strings.Contains(path, "installation-transactions/") {
		return journalLimit
	}
	return 1 << 20
}
func encodeRecord(record any) ([]byte, error) {
	data, err := json.Marshal(record)
	if len(data) >= journalLimit {
		return nil, conflict("installation record exceeds limit")
	}
	return append(data, '\n'), err
}
func writeRecord(ctx context.Context, tree *installationfs.Tree, path string, record any, expected *Image) error {
	data, err := encodeRecord(record)
	if err != nil {
		return err
	}
	return publishBytes(ctx, tree, path, data, 0600, expected)
}
func publishBytes(ctx context.Context, tree *installationfs.Tree, path string, data []byte, mode os.FileMode, expected *Image) error {
	return publishBytesWith(ctx, tree, path, data, mode, expected, nil)
}
func publishBytesWith(ctx context.Context, tree *installationfs.Tree, path string, data []byte, mode os.FileMode, expected *Image, onPrepared func(context.Context, *Image) error) (returned error) {
	parent, name, _, err := tree.Parent(ctx, path, true)
	if err != nil {
		return err
	}
	stage, err := filepublish.Prepare(ctx, parent.Root, ".installation-stage-", data, mode)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, stage.Cleanup()) }()
	if onPrepared != nil {
		if err := preparedObservation(ctx, stage, bytesImage(data, uint32(mode)), onPrepared); err != nil {
			return err
		}
	}
	current, _, err := observe(ctx, tree, path, leafLimit(path))
	if err != nil {
		return err
	}
	if !imageEqual(current, expected, true) {
		return conflict("installation destination changed before publication")
	}
	if err := tree.Check(ctx); err != nil {
		return err
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
		return &Uncertainty{Operation: "publication durability", cause: err}
	}
	return nil
}
func saveJournal(ctx context.Context, tree *installationfs.Tree, record journal, expected *Image) (*Image, error) {
	if err := writeRecord(ctx, tree, journalPath(record.MigrationID), record, expected); err != nil {
		return nil, err
	}
	image, _, err := observe(ctx, tree, journalPath(record.MigrationID), journalLimit)
	return image, err
}

// Prepare observation records the actual checked inode before its active rename.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:505.
func preparedObservation(ctx context.Context, stage *filepublish.Stage, expected *Image, save func(context.Context, *Image) error) (returned error) {
	file, err := stage.OpenPrepared(ctx)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, file.Close()) }()
	stat, err := statFile(file)
	if err != nil {
		return err
	}
	image := *expected
	id := identity(stat)
	image.Identity = &id
	if image.Mode != uint32(stat.Mode) || image.Bytes != stat.Size {
		return conflict("checked prepared image differs from expected selection")
	}
	return save(ctx, &image)
}
