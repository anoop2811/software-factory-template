package installation

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/anoop2811/software-factory-template/internal/installedlayout"
)

// candidateProof exists only in the retained owner's memory after actual gate execution.
// No journal, receipt, input descriptor or environment can construct this proof.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:400.
type candidateProof struct{ descriptor installedlayout.Descriptor }

func (owner *transaction) validateProvenTarget(ctx context.Context) error {
	if owner.proof == nil {
		return conflict("complete candidate proof is unavailable")
	}
	if err := owner.check(ctx); err != nil {
		return err
	}
	actual, err := installedlayout.ValidateAssets(ctx, owner.tree, nil)
	if err != nil {
		return err
	}
	provedBytes, err := json.Marshal(owner.proof.descriptor)
	if err != nil {
		return err
	}
	actualBytes, err := json.Marshal(actual)
	if err != nil {
		return err
	}
	if !bytes.Equal(provedBytes, actualBytes) {
		return conflict("installed selection differs from complete proven candidate")
	}
	control, _, err := observe(ctx, owner.tree, ".factory/installation.current", 1<<20)
	if err != nil {
		return err
	}
	retained, found := owner.published[".factory/installation.current"]
	if !found {
		for _, action := range owner.planned.proposal.Actions {
			if action.Path == ".factory/installation.current" && action.Action == "retain" {
				retained = action.Before
				found = true
			}
		}
	}
	if !found || !imageEqual(control, retained, true) {
		return conflict("installed descriptor identity differs from retained owner observation")
	}
	for _, asset := range actual.Assets {
		image, _, err := observe(ctx, owner.tree, asset.Path, leafLimit(asset.Path))
		if err != nil {
			return err
		}
		owned, found := owner.published[asset.Path]
		if !found {
			for _, action := range owner.planned.proposal.Actions {
				if action.Path == asset.Path && action.Action == "retain" {
					owned = action.Before
					found = true
				}
			}
		}
		if !found || !imageEqual(image, owned, true) {
			return conflict("installed leaf identity differs from retained owner observation")
		}
	}
	_, err = inspectState(ctx, owner.tree)
	if err != nil {
		return err
	}
	return owner.check(ctx)
}
