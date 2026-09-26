// Package assessment observes reviewed reference files without claiming ownership.
package assessment

import (
	"context"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// Observation describes complete bytes and displayed ordinary permission bits.
type Observation struct {
	SHA256 string `json:"sha256"`
	Mode   string `json:"mode"`
	Bytes  int64  `json:"bytes"`
}
type referenceAsset struct {
	Path      string
	Reference Observation
}

// Asset reports one scoped reference comparison, never mutation authority.
type Asset struct {
	Path           string       `json:"path"`
	Classification string       `json:"classification"`
	Reference      Observation  `json:"reference"`
	Observed       *Observation `json:"observed"`
	Reason         string       `json:"reason"`
}

// Result is a read-only reference report. Its false authorization fields are deliberate.
// docs/adr/0079-go-installation-reference-assessment.md:48.
type Result struct {
	SchemaVersion       int            `json:"schema_version"`
	ReferenceRevision   string         `json:"reference_revision"`
	Scope               string         `json:"scope"`
	OwnershipAuthorized bool           `json:"ownership_authorized"`
	ActivationReady     bool           `json:"activation_ready"`
	Assets              []Asset        `json:"assets"`
	Counts              map[string]int `json:"counts"`
}

// Status gives assessment errors precedence over any reference mismatch.
func (r Result) Status() int {
	if r.Counts["assessment_error"] != 0 {
		return 1
	}
	if r.Counts["customized"]+r.Counts["missing"]+r.Counts["unsafe"] != 0 {
		return 2
	}
	return 0
}

// Failure contains a fixed diagnostic and operation status, without filesystem data.
type Failure struct {
	Code    int
	message string
}

func (e *Failure) Error() string { return e.message }

// ErrorStatus returns the operation status for a failed assessment.
func ErrorStatus(err error) int {
	var failure *Failure
	if errors.As(err, &failure) {
		return failure.Code
	}
	return 1
}
func failure(code int, message string) error { return &Failure{Code: code, message: message} }

// Assess reads exactly the compiled reference catalog through confined descriptors.
// docs/adr/0079-go-installation-reference-assessment.md:86.
func Assess(ctx context.Context, root string) (Result, error) { return assess(ctx, root, ops{}) }

type ops struct {
	open func(parent *os.File, name string, flags int) (*os.File, error)
	read func(file *os.File, buffer []byte) (int, error)
}

func assess(ctx context.Context, root string, operations ops) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	chain, err := openRoot(ctx, root)
	if err != nil {
		return Result{}, err
	}
	defer chain.close()
	result := Result{SchemaVersion: 1, ReferenceRevision: referenceRevision, Scope: "g2-budget-loop-six", Assets: make([]Asset, 0, len(catalog)), Counts: map[string]int{
		"matching_reference": 0, "customized": 0, "missing": 0, "unsafe": 0, "assessment_error": 0,
	}}
	for _, reference := range catalog {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		asset, err := observe(ctx, chain.last(), reference, operations)
		if err != nil {
			return Result{}, err
		}
		result.Assets = append(result.Assets, asset)
		result.Counts[asset.Classification]++
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if !chain.valid() {
		return Result{}, failure(2, "unsafe assessment root")
	}
	return result, nil
}
func row(reference referenceAsset, classification string, observed *Observation) Asset {
	reason := ""
	switch classification {
	case "matching_reference":
		reason = "content_type_mode_match"
	case "customized":
		reason = "content_or_mode_differs"
	case "missing":
		reason = "path_absent"
	case "unsafe":
		reason = "unsafe_path_or_file"
	case "assessment_error":
		reason = "cannot_read_asset"
	}
	return Asset{Path: reference.Path, Classification: classification, Reference: reference.Reference, Observed: observed, Reason: reason}
}
func classification(err error) string {
	switch {
	case errors.Is(err, unix.ENOENT):
		return "missing"
	case errors.Is(err, unix.ELOOP), errors.Is(err, unix.ENOTDIR):
		return "unsafe"
	default:
		return "assessment_error"
	}
}
func permissions(stat unix.Stat_t) string { return fmt.Sprintf("%04o", stat.Mode&0777) }
