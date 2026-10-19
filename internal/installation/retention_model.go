package installation

import (
	"context"
	"errors"
	"regexp"

	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/staging"
)

// ReleaseIdentity is an untrusted acquisition hint, never deletion authority.
// docs/adr/0106-go-backup-retention.md:30.
type ReleaseIdentity struct {
	Version  string `json:"version"`
	Revision string `json:"revision"`
	Target   string `json:"target"`
}
type RetentionResolver func(context.Context, ReleaseIdentity) (staging.Options, error)

type RetentionSet struct {
	MigrationID string `json:"migration_id"`
	Path        string `json:"path"`
	Action      string `json:"action"`
	Reason      string `json:"reason"`
	Files       *int   `json:"files"`
	Bytes       *int64 `json:"bytes"`
	NextAction  string `json:"next_action"`
	Restorable  bool   `json:"restorable"`
}
type RetentionReport struct {
	SchemaVersion int            `json:"schema_version"`
	Mode          string         `json:"mode"`
	Status        string         `json:"status"`
	Advanced      bool           `json:"advanced"`
	Sets          []RetentionSet `json:"sets"`
	RemovedSets   int            `json:"removed_sets"`
	RemovedFiles  int            `json:"removed_files"`
	RemovedBytes  int64          `json:"removed_bytes"`
	RetainedSets  int            `json:"retained_sets"`
	RetainedFiles int            `json:"retained_files"`
	RetainedBytes int64          `json:"retained_bytes"`
	CountsKnown   bool           `json:"counts_known"`
}
type RetentionProposal struct {
	Proposal
	Retention RetentionReport `json:"retention"`
}

type retentionReceipt struct {
	SchemaVersion int               `json:"schema_version"`
	Kind          string            `json:"kind"`
	MigrationID   string            `json:"migration_id"`
	Profile       string            `json:"profile"`
	Before        ReleaseIdentity   `json:"before"`
	After         ReleaseIdentity   `json:"after"`
	Inputs        map[string]string `json:"inputs"`
	Manifest      *Image            `json:"manifest"`
	Saved         map[string]*Image `json:"saved"`
}
type retentionHold struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`
	MigrationID   string `json:"migration_id"`
}
type retentionSnapshot struct {
	ID           string                     `json:"migration_id"`
	Record       journal                    `json:"journal"`
	JournalImage *Image                     `json:"journal_image"`
	Manifest     backupManifest             `json:"manifest"`
	Files        map[string]*Image          `json:"files"`
	Directories  []installationfs.Directory `json:"directories"`
	Receipt      *retentionReceipt          `json:"receipt"`
	Hold         *retentionHold             `json:"hold"`
	BeforeHint   ReleaseIdentity            `json:"before_hint"`
	AfterHint    ReleaseIdentity            `json:"after_hint"`
	Unclassified *installationfs.Metadata   `json:"unclassified,omitempty"`
}
type retentionPlan struct {
	Report   RetentionReport     `json:"report"`
	Sets     []retentionSnapshot `json:"snapshots"`
	Progress *Image              `json:"progress"`
	Pending  string              `json:"pending_cleanup,omitempty"`
}
type retentionContext struct {
	resolver      RetentionResolver
	options       map[ReleaseIdentity]staging.Options
	inspected     map[ReleaseIdentity]staging.Inspection
	authenticated map[ReleaseIdentity]bool
	scratch       []retentionScratch
}
type retentionOperation struct {
	plan    retentionPlan
	context *retentionContext
	report  RetentionReport
}

const retentionProgressPath = ".factory/backups/.retention.json"

var revisionPatternRetention = regexp.MustCompile(`^[a-f0-9]{40}$`)
var releaseTagPatternRetention = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)
var backupIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func ValidBackupID(id string) bool { return backupIDPattern.MatchString(id) }
func backupBase(id string) string  { return ".factory/backups/" + id }
func releaseIdentity(o staging.Options) ReleaseIdentity {
	return ReleaseIdentity{o.Version, o.Revision, o.Target}
}
func newRetentionReport(mode string) RetentionReport {
	return RetentionReport{SchemaVersion: 1, Mode: mode, Status: "planned", Sets: []RetentionSet{}, CountsKnown: true}
}
func (r *RetentionReport) totals() {
	r.RetainedSets, r.RetainedFiles, r.RetainedBytes = 0, 0, 0
	r.CountsKnown = true
	for _, s := range r.Sets {
		if s.Action == "removed" || s.Action == "already-removed" {
			continue
		}
		r.RetainedSets++
		if s.Files == nil || s.Bytes == nil {
			r.CountsKnown = false
			continue
		}
		r.RetainedFiles += *s.Files
		r.RetainedBytes += *s.Bytes
	}
}

// CleanupIncomplete identifies a healthy committed installation with retryable
// cleanup work; it is never an interrupted activation.
func CleanupIncomplete(err error) bool {
	var failure *retentionCleanupFailure
	return errors.As(err, &failure)
}

type retentionCleanupFailure struct{ cause error }

func (e *retentionCleanupFailure) Error() string { return "backup cleanup is incomplete" }
func (e *retentionCleanupFailure) Unwrap() error { return e.cause }
