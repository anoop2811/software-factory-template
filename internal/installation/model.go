// Package installation qualifies one complete installation proposal and capability.
package installation

import (
	"context"
	"errors"
	"fmt"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/staging"
	"github.com/anoop2811/software-factory-template/internal/transition"
	"golang.org/x/sys/unix"
	"os"
	"regexp"
)

type Request struct {
	Source, Before                                                   staging.Options
	Profile, MigrationID, Confirmation, Rollback, Recover, Direction string
	Inputs                                                           map[string]string
	Quiescent                                                        bool
}
type role struct {
	Source, Path, Operation, Generator, Mode, Reason string
	Required                                         bool
}
type Identity struct {
	Device, Inode                                                uint64
	User, Group, Mode                                            uint32
	Links                                                        uint64
	Size                                                         int64
	ModifiedSeconds, ModifiedNanos, ChangedSeconds, ChangedNanos int64
}
type Image struct {
	Type     string    `json:"type"`
	Mode     uint32    `json:"mode"`
	SHA256   string    `json:"sha256"`
	Bytes    int64     `json:"bytes"`
	Identity *Identity `json:"identity"`
}
type Action struct {
	Path       string `json:"path"`
	SourcePath string `json:"source_path"`
	Action     string `json:"action"`
	Reason     string `json:"reason"`
	Required   bool   `json:"required"`
	Before     *Image `json:"before"`
	After      *Image `json:"after"`
}
type Issue struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}
type Proposal struct {
	SchemaVersion     int      `json:"schema_version"`
	Mode              string   `json:"mode"`
	Profile           string   `json:"profile"`
	Target            string   `json:"target"`
	Authentication    string   `json:"authentication"`
	ProposalDigest    string   `json:"proposal_digest"`
	Actions           []Action `json:"actions"`
	Checks            []string `json:"checks"`
	Conflicts         []Issue  `json:"conflicts"`
	Blockers          []Issue  `json:"blockers"`
	RollbackBlockers  []Issue  `json:"rollback_blockers"`
	Applicable        bool     `json:"applicable"`
	ChecksReady       bool     `json:"checks_ready"`
	Success           bool     `json:"success"`
	RollbackReady     bool     `json:"rollback_ready"`
	RecoveryCommand   string   `json:"recovery_command,omitempty"`
	RecoveryArguments []string `json:"recovery_arguments,omitempty"`
}
type Failure struct {
	Code   int
	Reason string
	cause  error
}

func (f *Failure) Error() string    { return f.Reason }
func (f *Failure) Unwrap() error    { return f.cause }
func conflict(message string) error { return &Failure{Code: 2, Reason: message} }
func ErrorStatus(err error) int {
	if err == nil {
		return 0
	}
	status := 2
	var inspect func(error, bool)
	inspect = func(value error, expectedBusy bool) {
		if value == nil {
			return
		}
		if errors.Is(value, context.Canceled) || errors.Is(value, context.DeadlineExceeded) {
			status = 1
			return
		}
		// Inspect each raw node before classifying a policy sentinel. An aggregate
		// errors.Is(ErrUnsafe) check would hide joined native I/O causes.
		//nolint:errorlint // Traverse each raw unwrap node; aggregate errors.As could hide a joined operational cause.
		switch node := value.(type) {
		case interface{ Unwrap() []error }:
			for _, child := range node.Unwrap() {
				inspect(child, expectedBusy)
			}
		case *transition.RootError:
			if !node.Conflict() {
				status = 1
			}
			inspect(node.Unwrap(), node.Conflict())
		case *Failure:
			if node.Code == 1 {
				status = 1
			}
			inspect(node.cause, expectedBusy)
		case interface{ Unwrap() error }:
			inspect(node.Unwrap(), expectedBusy)
		default:
			// Only a raw expected flock-busy leaf under the typed root conflict
			// is policy refusal. Joined I/O siblings remain operational failures.
			if expectedBusy && (errors.Is(value, unix.EAGAIN) || errors.Is(value, unix.EWOULDBLOCK)) {
				return
			}
			if !errors.Is(value, installationfs.ErrUnsafe) {
				status = 1
			}
		}
	}
	inspect(err, false)
	return status
}

func modeFor(role role) os.FileMode {
	var mode uint32
	_, _ = fmt.Sscanf(role.Mode, "%o", &mode)
	return os.FileMode(mode)
}

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Uncertainty retains a primary operational cause after a possibly visible mutation.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:195.
type Uncertainty struct {
	Operation string
	cause     error
}

func (e *Uncertainty) Error() string {
	return "installation " + e.Operation + " requires fresh recovery inspection"
}
func (e *Uncertainty) Unwrap() error { return e.cause }
