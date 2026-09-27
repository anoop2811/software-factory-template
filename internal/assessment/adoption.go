package assessment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"golang.org/x/sys/unix"
)

// RootIdentity binds a proposal to one physical installation directory.
type RootIdentity struct {
	Device string `json:"device"`
	Inode  string `json:"inode"`
}

// AssetIdentity preserves every identity and modification field bound by consent.
type AssetIdentity struct {
	Device           string `json:"device"`
	Inode            string `json:"inode"`
	Type             string `json:"type"`
	Mode             string `json:"mode"`
	Bytes            int64  `json:"bytes"`
	MtimeSeconds     string `json:"mtime_seconds"`
	MtimeNanoseconds int64  `json:"mtime_nanoseconds"`
	CtimeSeconds     string `json:"ctime_seconds"`
	CtimeNanoseconds int64  `json:"ctime_nanoseconds"`
}

// AdoptionAsset is an unchanged reference and its complete observed identity.
type AdoptionAsset struct {
	Path      string        `json:"path"`
	Reference Observation   `json:"reference"`
	Identity  AssetIdentity `json:"identity"`
	Observed  Observation   `json:"observed"`
}

// Proposal supplies reviewable observations, never authority by itself.
// docs/adr/0081-explicit-legacy-asset-adoption.md:42.
type Proposal struct {
	SchemaVersion       int             `json:"schema_version"`
	ReferenceRevision   string          `json:"reference_revision"`
	Scope               string          `json:"scope"`
	Purpose             string          `json:"purpose"`
	PriorOrigin         string          `json:"prior_origin"`
	OwnershipAuthorized bool            `json:"ownership_authorized"`
	ProposalDigest      string          `json:"proposal_digest"`
	RootIdentity        RootIdentity    `json:"root_identity"`
	Assets              []AdoptionAsset `json:"assets"`
}

// The canonical type binds each declared proposal field except its own digest.
// Selection is explicit even though asset paths also carry it.
// docs/adr/0081-explicit-legacy-asset-adoption.md:55.
type adoptionPayload struct {
	Purpose             string          `json:"purpose"`
	SchemaVersion       int             `json:"schema_version"`
	Scope               string          `json:"scope"`
	ReferenceRevision   string          `json:"reference_revision"`
	PriorOrigin         string          `json:"prior_origin"`
	OwnershipAuthorized bool            `json:"ownership_authorized"`
	RootIdentity        RootIdentity    `json:"root_identity"`
	Selection           []string        `json:"selection"`
	Assets              []AdoptionAsset `json:"assets"`
}

func proposalDigest(proposal Proposal, selection []string) (string, error) {
	payload := adoptionPayload{
		Purpose: proposal.Purpose, SchemaVersion: proposal.SchemaVersion, Scope: proposal.Scope,
		ReferenceRevision: proposal.ReferenceRevision, PriorOrigin: proposal.PriorOrigin,
		OwnershipAuthorized: proposal.OwnershipAuthorized, RootIdentity: proposal.RootIdentity,
		Selection: selection, Assets: proposal.Assets,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", failure(1, "cannot encode adoption proposal")
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("software-factory/legacy-adoption/v1\n"))
	_, _ = hash.Write(encoded)
	return hex.EncodeToString(hash.Sum(nil)), nil
}
func assetIdentity(stat unix.Stat_t) AssetIdentity {
	return AssetIdentity{
		Device: fmt.Sprint(stat.Dev), Inode: fmt.Sprint(stat.Ino), Type: "regular", Mode: fmt.Sprintf("%04o", stat.Mode&07777), Bytes: stat.Size,
		MtimeSeconds: fmt.Sprint(stat.Mtim.Sec), MtimeNanoseconds: int64(stat.Mtim.Nsec),
		CtimeSeconds: fmt.Sprint(stat.Ctim.Sec), CtimeNanoseconds: int64(stat.Ctim.Nsec),
	}
}
func selectedReferences(paths []string) ([]referenceAsset, error) {
	if len(paths) < 1 || len(paths) > len(catalog) {
		return nil, failure(2, "invalid adoption selection")
	}
	chosen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if chosen[path] {
			return nil, failure(2, "invalid adoption selection")
		}
		known := false
		for _, reference := range catalog {
			if path == reference.Path {
				known = true
				break
			}
		}
		if !known {
			return nil, failure(2, "invalid adoption selection")
		}
		chosen[path] = true
	}
	selected := make([]referenceAsset, 0, len(paths))
	for _, reference := range catalog {
		if chosen[reference.Path] {
			selected = append(selected, reference)
		}
	}
	return selected, nil
}
func selectionPaths(selected []referenceAsset) []string {
	paths := make([]string, 0, len(selected))
	for _, reference := range selected {
		paths = append(paths, reference.Path)
	}
	return paths
}

// ProposeAdoption returns a digest-bound proposal for unchanged selected assets.
// docs/adr/0081-explicit-legacy-asset-adoption.md:28.
func ProposeAdoption(ctx context.Context, root string, paths []string) (Proposal, error) {
	return proposeAdoption(ctx, root, paths, ops{})
}
func proposeAdoption(ctx context.Context, root string, paths []string, operations ops) (Proposal, error) {
	selected, err := selectedReferences(paths)
	if err != nil {
		return Proposal{}, err
	}
	if err := ctx.Err(); err != nil {
		return Proposal{}, err
	}
	chain, err := openRoot(ctx, root)
	if err != nil {
		return Proposal{}, err
	}
	defer chain.close()
	return proposalPinned(ctx, chain, selected, operations)
}
func proposalPinned(ctx context.Context, chain directories, selected []referenceAsset, operations ops) (Proposal, error) {
	assets := make([]Asset, 0, len(selected))
	for _, reference := range selected {
		if err := ctx.Err(); err != nil {
			return Proposal{}, err
		}
		asset, err := observe(ctx, chain.last(), reference, operations)
		if err != nil {
			return Proposal{}, err
		}
		assets = append(assets, asset)
	}
	if err := ctx.Err(); err != nil {
		return Proposal{}, err
	}
	if !chain.valid() {
		return Proposal{}, failure(2, "unsafe assessment root")
	}
	return proposalFromAssets(chain, selected, assets)
}
func proposalFromAssets(chain directories, selected []referenceAsset, assets []Asset) (Proposal, error) {
	root := chain[len(chain)-1].identity
	result := Proposal{SchemaVersion: 1, ReferenceRevision: referenceRevision, Scope: "g2-budget-loop-six", Purpose: "adopt_known_legacy_assets", PriorOrigin: "unproven", RootIdentity: RootIdentity{Device: fmt.Sprint(root.Dev), Inode: fmt.Sprint(root.Ino)}, Assets: make([]AdoptionAsset, 0, len(selected))}
	for _, asset := range assets {
		for _, reference := range selected {
			if asset.Path == reference.Path && asset.Classification == "assessment_error" {
				return Proposal{}, failure(1, "cannot read adoption asset")
			}
		}
	}
	for _, reference := range selected {
		var observed *Asset
		for i := range assets {
			if assets[i].Path == reference.Path {
				observed = &assets[i]
				break
			}
		}
		if observed == nil {
			return Proposal{}, failure(1, "cannot establish adoption observations")
		}
		if observed.Classification == "assessment_error" {
			return Proposal{}, failure(1, "cannot read adoption asset")
		}
		if observed.Classification != "matching_reference" || observed.Observed == nil {
			return Proposal{}, failure(2, "adoption requires unchanged reference assets")
		}
		result.Assets = append(result.Assets, AdoptionAsset{Path: reference.Path, Reference: reference.Reference, Identity: observed.identity, Observed: *observed.Observed})
	}
	digest, err := proposalDigest(result, selectionPaths(selected))
	if err != nil {
		return Proposal{}, err
	}
	result.ProposalDigest = digest
	return result, nil
}
