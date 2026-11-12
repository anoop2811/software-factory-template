package installation

import (
	"context"
	"errors"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/ciworkflow"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/staging"
	"github.com/anoop2811/software-factory-template/internal/transition"
)

// CIWorkflowPreview describes whether the caller has integrated CI into consent.
// docs/adr/0148-transactional-installed-ci-migration.md:39.
type CIWorkflowPreview struct {
	SchemaVersion  int                `json:"schema_version"`
	Mode           string             `json:"mode"`
	ApplySupported bool               `json:"apply_supported"`
	Changes        []CIWorkflowChange `json:"changes"`
	Conflicts      []Issue            `json:"conflicts"`
	Reason         string             `json:"reason"`
}

type CIWorkflowChange struct {
	Path         string `json:"path"`
	Disposition  string `json:"disposition"`
	Reason       string `json:"reason"`
	BeforeSHA256 string `json:"before_sha256,omitempty"`
	AfterSHA256  string `json:"after_sha256,omitempty"`
}

const ciPreviewAdvice = "Confirmation applies only to the existing installation actions and does not perform these proposed CI changes."

// PreviewCIWorkflow observes only the finite workflow selection and one config snapshot.
// The caller supplies release authentication; no inspection flag establishes it.
// docs/adr/0147-installed-ci-migration-preview.md:23.
// docs/adr/0147-installed-ci-migration-preview.md:49.
func PreviewCIWorkflow(ctx context.Context, root string, request Request, release *ciworkflow.Release, before *staging.Inspection) (result CIWorkflowPreview, returned error) {
	lease, err := transition.ReadOnly(ctx, root)
	if err != nil {
		return result, err
	}
	defer func() { returned = errors.Join(returned, lease.Close(context.WithoutCancel(ctx))) }()
	tree, err := installationfs.Open(ctx, root)
	if err != nil {
		return result, err
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	result, err = inspectCIWorkflow(ctx, tree, request, release, before, nil)
	if err != nil {
		return result, err
	}
	return result, lease.Check(ctx)
}

var ciWorkflowNames = []string{"ci.yml", "go-pack.yml", "typescript-pack.yml", "java-pack.yml", "factory.yml"}

type ciWorkflowObservation struct {
	config  *Image
	inputs  ciPreviewInputs
	current map[string]string
	target  []byte
	before  map[string]*Image
	data    map[string][]byte
}

// Inspect on the caller's retained tree; transaction planning never nests a lease.
// docs/adr/0148-transactional-installed-ci-migration.md:31.
func inspectCIWorkflow(ctx context.Context, tree *installationfs.Tree, request Request, release *ciworkflow.Release, before *staging.Inspection, capture *ciWorkflowObservation) (result CIWorkflowPreview, returned error) {
	result = CIWorkflowPreview{SchemaVersion: 1, Mode: "ci-workflow-preview", Changes: []CIWorkflowChange{}, Conflicts: []Issue{}, Reason: ciPreviewAdvice}
	unavailable := ciReleaseUnavailable(request, release)
	configImage, data, err := observe(ctx, tree, "factory.yaml", 1<<20)
	if err != nil && ErrorStatus(err) == 1 {
		return result, err
	}
	inputs := ciPreviewInputs{}
	inputProblem := ""
	if err != nil || configImage == nil {
		inputProblem = "factory.yaml is missing, unsafe or oversized; CI inputs cannot be qualified"
	} else {
		inputs, err = ciReadInputs(ctx, data, request.Inputs)
		if err != nil {
			if ErrorStatus(err) == 1 {
				return result, err
			}
			inputProblem = err.Error()
		}
	}
	if inputProblem != "" {
		result.Conflicts = append(result.Conflicts, Issue{Path: "factory.yaml", Reason: inputProblem})
	}
	if unavailable != "" {
		result.Reason = unavailable + ". " + ciPreviewAdvice
	}
	ready := unavailable == "" && inputProblem == ""
	var history ciWorkflowHistory
	var target []byte
	if ready {
		target = []byte(ciworkflow.Initialized(*release, inputs.application))
		history, err = ciPredecessorBytes(ctx, request, *release, before, inputs)
		if err != nil {
			return result, err
		}
	}
	if capture != nil {
		capture.config = configImage
		capture.inputs = inputs
		capture.target = target
		if ready {
			current, currentErr := ciReadInputs(ctx, data, nil)
			if currentErr != nil {
				return result, currentErr
			}
			capture.current = current.values
		}
	}
	for _, name := range ciWorkflowNames {
		path := ".github/workflows/" + name
		row := CIWorkflowChange{Path: path, Disposition: "absent", Reason: "No workflow occupies this selected path"}
		actual, data, readErr := observe(ctx, tree, path, 1<<20)
		if capture != nil {
			capture.before[path], capture.data[path] = actual, data
		}
		if readErr != nil && ErrorStatus(readErr) == 1 {
			return result, readErr
		}
		if actual != nil {
			row.BeforeSHA256 = actual.SHA256
		}
		switch {
		case readErr != nil:
			row.Disposition, row.Reason = "preserve", "Unsafe, changed or oversized workflow occupant; preserve without following it"
		case actual == nil:
			if ready && name == "factory.yml" {
				row.Disposition, row.Reason = "create", "Proposed authenticated target rendering; applying this CI proposal is not supported"
				row.AfterSHA256 = bytesImage(target, 0644).SHA256
			}
		case !ready:
			row.Disposition, row.Reason = "preserve", "CI proposal unavailable because authenticated release or project inputs could not be qualified"
		default:
			row.Disposition, row.Reason = "preserve", history.reason
			if strings.HasSuffix(name, "-pack.yml") && !inputs.packs[strings.TrimSuffix(name, "-pack.yml")] {
				row.Reason = "Pack workflow is present but the pack is not selected; preserve it"
			}
			for _, candidate := range history.bytes[path] {
				expected := bytesImage(candidate.data, candidate.mode)
				if actual.Mode != expected.Mode {
					row.Reason = "Workflow mode differs from the qualified initializer mode"
					continue
				}
				if imageEqual(actual, expected, false) {
					row.Disposition, row.Reason = "retire", "Exact qualified copied workflow; proposed retirement only"
					if name == "factory.yml" {
						row.Disposition, row.Reason = "replace", "Exact frozen generated workflow; proposed authenticated target refresh only"
						row.AfterSHA256 = bytesImage(target, 0644).SHA256
					}
					break
				}
			}
		}
		if row.Disposition == "preserve" {
			result.Conflicts = append(result.Conflicts, Issue{Path: path, Reason: row.Reason})
		}
		result.Changes = append(result.Changes, row)
	}
	if len(result.Conflicts) > 0 {
		result.Reason += " Unresolved CI conflicts prevent a whole-workflow migration proposal."
	}
	return result, tree.Check(ctx)
}

func ciReleaseUnavailable(request Request, release *ciworkflow.Release) string {
	if request.Source.Local {
		return "Authenticated CI proposal unavailable for local-source upgrades"
	}
	if release == nil {
		return "Authenticated CI proposal unavailable without an explicit authenticated release"
	}
	if release.Repository != "anoop2811/software-factory-template" {
		return "Authenticated CI proposal unavailable for an unofficial repository"
	}
	if !ciworkflow.EligibleRelease(release.Tag) {
		return "Authenticated CI proposal unavailable for a noncanonical release tag"
	}
	if release.Tag != request.Source.Version || release.SourceRevision != request.Source.Revision || release.OriginTarget != request.Source.Target || !ciRevision(release.SourceRevision) {
		return "Authenticated CI proposal unavailable: release and source metadata mismatch"
	}
	switch release.OriginTarget {
	case "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64":
	default:
		return "Authenticated CI proposal unavailable for an unsupported platform"
	}
	return ""
}

func ciRevision(value string) bool {
	return len(value) == 40 && strings.Trim(value, "0123456789abcdef") == ""
}
