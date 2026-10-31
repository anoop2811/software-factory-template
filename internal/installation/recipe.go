package installation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/staging"
)

// The frozen RC3 vector was read from immutable source 95486ecdd3868a319a0e6362ee99cafd8f154200.
// Closed historical recipes can cross a generation change without executing old code.
// docs/adr/0115-frozen-rc3-recipe-compatibility.md:23.
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

// The post-RC3 vector and identical frozen role data come from immutable 9be9d033.
// docs/adr/0119-minimal-qualified-initializer-inventory.md:35.
var recipeV2 = map[string]string{
	"internal/installation/generate.go":      "2efc19f6ca0ccb63c5f032b6191d8ccf957672499d7ca3b03576ce2426d8bf5e",
	"internal/installation/roles.go":         "d24777171bc40a71b85c964aafbd26f04587e7b92769b20ca45b8d35ae100c8d",
	"internal/installation/orchestration.go": "9510b0f141ccbba2a53862175d2bbea043c807f1ca0408ad80e416b637575e96",
	"internal/installation/reference.go":     "6195dd7d6d2b3f80ecb5e1ba570cc40cd389963159a5042a34a6d3889e206926",
	"internal/installation/model.go":         "3684d7faf3d4426533852efba153a14c0cbdd1dcdc1579fcbfa2d5cda2f44c33",
	"internal/installation/observe.go":       "80023a90d6982a1a7c022e4d5901b5d01c8a0757d0e73161d1a68616b978006f",
	"internal/installation/selection.go":     "3c7595593d84f689ae148cb8b8aca1c57d9007f46a507cf7e1b146196b2fe7ba",
	"internal/installedlayout/layout.go":     "c1ffbbb9a2bf4d3776367dd594df9c71135a02b90fad4d8433b7f4f501cde620",
	"internal/installation/recipe.go":        "e30282ddd68625ae0fa62c9a1817279bb3ba057b27c455dd5c67d7e77d98f070",
	"internal/installation/roles_v1.go":      "9fcb4a7cbaf4c0eb34160f47d2e496ac9148a1e0e719299b9101b52a8ec8c853",
}

// Original source-image generation controls selection and transaction shape.
// docs/adr/0119-minimal-qualified-initializer-inventory.md:61.
func rolesForImage(image staging.Inspection) []role {
	if isRecipeV1(image) || isRecipeV2(image) {
		return rolesV1
	}
	return roles
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
	if !isRecipeV1(before) && !isRecipeV2(before) {
		return nil, conflict("unsupported Go predecessor generation recipe; preserve the existing installation")
	}
	return selectionForRoles(ctx, before, inputs, rolesV1)
}

func isRecipeV1(image staging.Inspection) bool {
	return matchesRecipe(image, recipeV1)
}

func isRecipeV2(image staging.Inspection) bool {
	return matchesRecipe(image, recipeV2)
}

func matchesRecipe(image staging.Inspection, recipe map[string]string) bool {
	for path, expected := range recipe {
		data, exists := image.Assets[path]
		digest := sha256.Sum256(data)
		if !exists || hex.EncodeToString(digest[:]) != expected {
			return false
		}
	}
	return true
}

// Record parsing recognizes complete closed sets, never authority from a count.
// Fresh image qualification still selects the original generation before mutation.
// docs/adr/0119-minimal-qualified-initializer-inventory.md:69.
func matchesRoleEntries(entries []journalEntry, recipe []role) bool {
	if len(entries) != len(recipe) {
		return false
	}
	paths := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if paths[entry.Path] {
			return false
		}
		paths[entry.Path] = true
	}
	for _, row := range recipe {
		if !paths[row.Path] {
			return false
		}
	}
	return true
}

func knownRoleEntries(entries []journalEntry) bool {
	return matchesRoleEntries(entries, roles) || matchesRoleEntries(entries, rolesV1)
}

func rolePath(recipe []role, path string) bool {
	for _, row := range recipe {
		if row.Path == path {
			return true
		}
	}
	return false
}

// An old descriptor may still require preserved cache outside the new selection.
// Observe it before reverse publication or proof of an alternate usable recovery.
// docs/adr/0119-minimal-qualified-initializer-inventory.md:84.
func preservedPredecessorIssues(ctx context.Context, tree *installationfs.Tree, reference baseline, target []role, inputs map[string]string) ([]Issue, error) {
	issues := []Issue{}
	if reference.Runtime == nil {
		return issues, nil
	}
	for _, row := range rolesForImage(*reference.Runtime) {
		if !row.Required || row.Operation == "retire" || rolePath(target, row.Path) {
			continue
		}
		expected, _, known, err := reference.expected(ctx, row.Path, inputs)
		if err != nil {
			return nil, err
		}
		actual, _, err := observe(ctx, tree, row.Path, leafLimit(row.Path))
		if err != nil && ErrorStatus(err) == 1 {
			return nil, err
		}
		if err != nil || !known || expected == nil || !imageEqual(actual, expected, false) {
			issues = append(issues, Issue{row.Path, "historical required cache is changed, missing or unsafe; preserved"})
		}
	}
	return issues, nil
}
