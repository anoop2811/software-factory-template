package installation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/anoop2811/software-factory-template/internal/staging"
)

// The frozen RC3 vector was read from immutable source 95486ecdd3868a319a0e6362ee99cafd8f154200.
// Only this closed historical recipe can cross a generation change.
// docs/adr/0108-default-go-cutover.md:119.
var recipeV1 = map[string]string{
	"internal/installation/generate.go":      "8bac18ca0cda2b1412164f70c33f14b442895d25229563337ac7a9955899e541",
	"internal/installation/roles.go":         "d24777171bc40a71b85c964aafbd26f04587e7b92769b20ca45b8d35ae100c8d",
	"internal/installation/orchestration.go": "9510b0f141ccbba2a53862175d2bbea043c807f1ca0408ad80e416b637575e96",
	"internal/installation/reference.go":     "188a7217cf7ab8276aed6cf901220a7a62bbd799a8080c6bed660fadaabc9f93",
	"internal/installation/model.go":         "3684d7faf3d4426533852efba153a14c0cbdd1dcdc1579fcbfa2d5cda2f44c33",
	"internal/installation/observe.go":       "80023a90d6982a1a7c022e4d5901b5d01c8a0757d0e73161d1a68616b978006f",
	"internal/installation/selection.go":     "dad913f4c1e837b6fb5d3660bfdb63e6129ce8ad186931a7cffb86a207e2721b",
	"internal/installedlayout/layout.go":     "c1ffbbb9a2bf4d3776367dd594df9c71135a02b90fad4d8433b7f4f501cde620",
}

func predecessorSelection(ctx context.Context, before, after staging.Inspection, inputs map[string]string) ([]SelectedAsset, error) {
	current := true
	paths := make([]string, 0, len(recipeV1)+2)
	for path := range recipeV1 {
		paths = append(paths, path)
	}
	paths = append(paths, "internal/installation/recipe.go", "internal/installation/roles_v1.go")
	for _, path := range paths {
		old, oldOK := before.Assets[path]
		present, presentOK := after.Assets[path]
		if !oldOK || !presentOK || !bytes.Equal(old, present) {
			current = false
			break
		}
	}
	if current {
		return Selection(ctx, before, inputs)
	}
	if !isRecipeV1(before) {
		return nil, conflict("unsupported Go predecessor generation recipe; preserve the existing installation")
	}
	return selectionForRoles(ctx, before, inputs, rolesV1)
}

func isRecipeV1(image staging.Inspection) bool {
	for path, expected := range recipeV1 {
		data, exists := image.Assets[path]
		digest := sha256.Sum256(data)
		if !exists || hex.EncodeToString(digest[:]) != expected {
			return false
		}
	}
	return true
}
