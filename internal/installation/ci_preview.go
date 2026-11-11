package installation

import (
	"context"
	"errors"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/ciworkflow"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/staging"
	"github.com/anoop2811/software-factory-template/internal/transition"
	"golang.org/x/sys/unix"
)

// CIWorkflowPreview is informational and never participates in installation consent.
// docs/adr/0147-installed-ci-migration-preview.md:30.
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
	result = CIWorkflowPreview{SchemaVersion: 1, Mode: "ci-workflow-preview", Changes: []CIWorkflowChange{}, Conflicts: []Issue{}, Reason: ciPreviewAdvice}
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
	for _, name := range []string{"ci.yml", "go-pack.yml", "typescript-pack.yml", "java-pack.yml", "factory.yml"} {
		path := ".github/workflows/" + name
		row := CIWorkflowChange{Path: path, Disposition: "absent", Reason: "No workflow occupies this selected path"}
		actual, _, readErr := observe(ctx, tree, path, 1<<20)
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
		case actual.Mode != unix.S_IFREG|0644:
			row.Disposition, row.Reason = "preserve", "Workflow mode differs from the qualified initializer mode"
		default:
			row.Disposition, row.Reason = "preserve", history.reason
			if strings.HasSuffix(name, "-pack.yml") && !inputs.packs[strings.TrimSuffix(name, "-pack.yml")] {
				row.Reason = "Pack workflow is present but the pack is not selected; preserve it"
			}
			for _, body := range history.bytes[path] {
				if imageEqual(actual, bytesImage(body, 0644), false) {
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
	if err := tree.Check(ctx); err != nil {
		return result, err
	}
	return result, lease.Check(ctx)
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
