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
	selected, err := selectedReferences(paths)
	if err != nil {
		return AdoptedPlan{}, err
	}
	if !validProposalDigest(digest) {
		return AdoptedPlan{}, adoptionConflict()
	}
	if err := ctx.Err(); err != nil {
		return AdoptedPlan{}, err
	}
	installedRoot, err := openRoot(ctx, root)
	if err != nil {
		return AdoptedPlan{}, err
	}
	defer installedRoot.close()
	sourceRoot, err := openRoot(ctx, source)
	if err != nil {
		return AdoptedPlan{}, err
	}
	defer sourceRoot.close()
	before, err := proposalPinned(ctx, installedRoot, selected, installedOps)
	if err != nil {
		return AdoptedPlan{}, err
	}
	if before.ProposalDigest != digest {
		return AdoptedPlan{}, adoptionConflict()
	}
	result, observed, err := planPinned(ctx, installedRoot, sourceRoot, installedOps, sourceOps)
	if err != nil {
		return AdoptedPlan{}, err
	}
	used, err := proposalFromAssets(installedRoot, selected, observed)
	if err != nil {
		return AdoptedPlan{}, err
	}
	if used.ProposalDigest != digest {
		return AdoptedPlan{}, adoptionConflict()
	}
	after, err := proposalPinned(ctx, installedRoot, selected, installedOps)
	if err != nil {
		return AdoptedPlan{}, err
	}
	if after.ProposalDigest != digest {
		return AdoptedPlan{}, adoptionConflict()
	}
	if err := ctx.Err(); err != nil {
		return AdoptedPlan{}, err
	}
	if !installedRoot.valid() || !sourceRoot.valid() {
		return AdoptedPlan{}, failure(2, "unsafe assessment root")
	}
	// Only the ownership prerequisite changes. Local source, runtime qualification,
	// quiescence and verified recovery remain independently blocked.
	result.OwnershipAuthorized = len(selected) == len(catalog)
	if result.OwnershipAuthorized {
		result.Blockers = result.Blockers[1:]
	} else {
		result.Blockers[0] = "ownership_scope_incomplete"
	}
	return AdoptedPlan{PlanResult: result, OwnershipBasis: "explicit_operator_adoption", ProposalDigest: digest, AuthorizedPaths: selectionPaths(selected)}, nil
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
