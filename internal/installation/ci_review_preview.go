package installation

import (
	"bytes"
	"context"

	"github.com/anoop2811/software-factory-template/internal/ciworkflow"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/reviewlanecmd"
	"github.com/anoop2811/software-factory-template/internal/staging"
)

// Only an enabled lane enters transaction custody. Missing workflows remain
// missing, while present workflows require exact predecessor ownership.
// docs/adr/0154-installed-review-workflow-migration.md:66.
func inspectCIReview(ctx context.Context, tree *installationfs.Tree, request Request, release *ciworkflow.Release, before *staging.Inspection, config []byte, ready bool, capture *ciWorkflowObservation, result *CIWorkflowPreview) error {
	review, err := ciReadReview(ctx, config)
	if err != nil {
		if ErrorStatus(err) == 1 {
			return err
		}
		result.Conflicts = append(result.Conflicts, Issue{Path: "factory.yaml", Reason: err.Error()})
		return nil
	}
	if review == nil {
		return ctx.Err()
	}
	row := CIWorkflowChange{Path: ciReviewPath, Disposition: "absent", Reason: "Enabled review workflow is absent and will not be created during upgrade"}
	actual, data, readErr := observe(ctx, tree, ciReviewPath, 1<<20)
	if readErr != nil && ErrorStatus(readErr) == 1 {
		return readErr
	}
	review.Present = actual != nil
	if actual != nil {
		row.BeforeSHA256 = actual.SHA256
	}
	if capture != nil {
		capture.before[ciReviewPath], capture.data[ciReviewPath] = actual, data
		if ready {
			capture.review = review
		}
	}
	switch {
	case readErr != nil:
		row.Disposition, row.Reason = "preserve", "Unsafe, changed or oversized enabled review workflow; preserve without following it"
	case actual == nil:
	case !ready:
		row.Disposition, row.Reason = "preserve", "Review proposal unavailable because authenticated release or project inputs could not be qualified"
	default:
		row.Disposition = "preserve"
		history, reason, historyErr := ciReviewPredecessor(ctx, request, before, review.Secret)
		row.Reason = reason
		if historyErr != nil && ErrorStatus(historyErr) == 1 {
			return historyErr
		}
		matched := false
		for _, candidate := range history {
			matched = matched || imageEqual(actual, bytesImage(candidate.data, candidate.mode), false)
		}
		if historyErr == nil && matched {
			target, targetErr := ciObservedReviewTarget(ctx, request, *release, review.Secret, capture)
			if targetErr != nil {
				if ErrorStatus(targetErr) == 1 {
					return targetErr
				}
				row.Reason = targetErr.Error()
			} else {
				row.Disposition, row.Reason = "replace", "Exact qualified enabled review workflow; proposed authenticated target refresh only"
				row.AfterSHA256 = bytesImage(target, 0644).SHA256
				if capture != nil {
					capture.targets[ciReviewPath] = target
				}
			}
		}
	}
	if row.Disposition == "preserve" {
		result.Conflicts = append(result.Conflicts, Issue{Path: ciReviewPath, Reason: row.Reason})
	}
	result.Changes = append(result.Changes, row)
	return ctx.Err()
}

func ciObservedReviewTarget(ctx context.Context, request Request, release ciworkflow.Release, secret string, capture *ciWorkflowObservation) ([]byte, error) {
	var image *staging.Inspection
	if capture != nil {
		image = capture.targetImage
	}
	if image == nil {
		inspected, err := staging.Inspect(ctx, request.Source)
		if err != nil {
			return nil, err
		}
		image = &inspected
	}
	frozen, err := ciReviewTarget(ctx, *image, release, secret)
	if err != nil {
		return nil, err
	}
	current, err := reviewlanecmd.RenderWorkflow(ctx, image.Assets[ciReviewTemplate], secret)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(current, frozen) {
		return nil, conflict("current review renderer differs from the qualified target projection")
	}
	return current, ctx.Err()
}
