package assessment

import "context"

// AdoptedPlan grants only the current explicit selection; all other migration
// prerequisites remain blocked and historical origin stays unproven.
// docs/adr/0081-explicit-legacy-asset-adoption.md:86.
type AdoptedPlan struct {
	PlanResult
	OwnershipBasis  string   `json:"ownership_basis"`
	ProposalDigest  string   `json:"proposal_digest"`
	AuthorizedPaths []string `json:"authorized_paths"`
}

// PlanAdopted consumes explicit positional confirmation without persisting authority.
func PlanAdopted(ctx context.Context, root, source, digest string, paths []string) (AdoptedPlan, error) {
	return planAdopted(ctx, root, source, digest, paths, ops{}, ops{})
}
func planAdopted(ctx context.Context, root, source, digest string, paths []string, installedOps, sourceOps ops) (AdoptedPlan, error) {
	if _, err := selectedReferences(paths); err != nil {
		return AdoptedPlan{}, err
	}
	if !validProposalDigest(digest) {
		return AdoptedPlan{}, adoptionConflict()
	}
	preview, err := previewAdoption(ctx, root, source, paths, digest, installedOps, sourceOps)
	if err != nil {
		return AdoptedPlan{}, err
	}
	return AdoptedPlan{PlanResult: preview.Plan, OwnershipBasis: preview.OwnershipBasis, ProposalDigest: preview.Proposal.ProposalDigest, AuthorizedPaths: preview.AuthorizedPaths}, nil
}

// AdoptionPreview keeps the proposal and action plan from one pinned observation flow.
// Empty confirmation preserves all original ownership blockers.
// docs/adr/0082-go-public-upgrade-preview.md:59.
type AdoptionPreview struct {
	Plan            PlanResult
	Proposal        Proposal
	OwnershipBasis  string
	AuthorizedPaths []string
}

// PreviewAdoption supports proposal inspection and explicit confirmation without
// combining independently opened scans into apparent authorization.
func PreviewAdoption(ctx context.Context, root, source string, paths []string, confirmation string) (AdoptionPreview, error) {
	return previewAdoption(ctx, root, source, paths, confirmation, ops{}, ops{})
}
func previewAdoption(ctx context.Context, root, source string, paths []string, digest string, installedOps, sourceOps ops) (AdoptionPreview, error) {
	selected, err := selectedReferences(paths)
	if err != nil {
		return AdoptionPreview{}, err
	}
	if digest != "" && !validProposalDigest(digest) {
		return AdoptionPreview{}, adoptionConflict()
	}
	if err := ctx.Err(); err != nil {
		return AdoptionPreview{}, err
	}
	installedRoot, err := openRoot(ctx, root)
	if err != nil {
		return AdoptionPreview{}, err
	}
	defer installedRoot.close()
	sourceRoot, err := openRoot(ctx, source)
	if err != nil {
		return AdoptionPreview{}, err
	}
	defer sourceRoot.close()
	before, err := proposalPinned(ctx, installedRoot, selected, installedOps)
	if err != nil {
		return AdoptionPreview{}, err
	}
	if digest != "" && before.ProposalDigest != digest {
		return AdoptionPreview{}, adoptionConflict()
	}
	expectedDigest := before.ProposalDigest
	result, observed, err := planPinned(ctx, installedRoot, sourceRoot, installedOps, sourceOps)
	if err != nil {
		return AdoptionPreview{}, err
	}
	used, err := proposalFromAssets(installedRoot, selected, observed)
	if err != nil {
		return AdoptionPreview{}, err
	}
	if used.ProposalDigest != expectedDigest {
		return AdoptionPreview{}, adoptionConflict()
	}
	after, err := proposalPinned(ctx, installedRoot, selected, installedOps)
	if err != nil {
		return AdoptionPreview{}, err
	}
	if after.ProposalDigest != expectedDigest {
		return AdoptionPreview{}, adoptionConflict()
	}
	if err := ctx.Err(); err != nil {
		return AdoptionPreview{}, err
	}
	if !installedRoot.valid() || !sourceRoot.valid() {
		return AdoptionPreview{}, failure(2, "unsafe assessment root")
	}
	preview := AdoptionPreview{Plan: result, Proposal: before, OwnershipBasis: "unconfirmed", AuthorizedPaths: []string{}}
	if digest != "" {
		preview.OwnershipBasis = "explicit_operator_adoption"
		preview.AuthorizedPaths = selectionPaths(selected)
		preview.Plan.OwnershipAuthorized = len(selected) == len(catalog)
		if preview.Plan.OwnershipAuthorized {
			preview.Plan.Blockers = preview.Plan.Blockers[1:]
		} else {
			preview.Plan.Blockers[0] = "ownership_scope_incomplete"
		}
	}
	return preview, nil
}
func validProposalDigest(digest string) bool {
	if len(digest) != 64 {
		return false
	}
	for _, character := range digest {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
func adoptionConflict() error {
	return failure(2, "adoption confirmation does not match current observations")
}
