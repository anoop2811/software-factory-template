package installation

// Frozen runtime projection from immutable 24712604cbe98dc2df087b73910141defb68f745.
// CI transaction participation does not enlarge this runtime selection.
// docs/adr/0148-transactional-installed-ci-migration.md:81.
var recipeV4 = map[string]string{
	"internal/installation/generate.go":      "6143094ba89cbe989b640aee2a354b05b605b19bf1770518f06608b7e42b7cc4",
	"internal/installation/roles.go":         "e08334bdfb77b6e058071dedcff4b8cb2fc4dc1d18e4b0793a42a2725b509937",
	"internal/installation/orchestration.go": "9510b0f141ccbba2a53862175d2bbea043c807f1ca0408ad80e416b637575e96",
	"internal/installation/reference.go":     "6195dd7d6d2b3f80ecb5e1ba570cc40cd389963159a5042a34a6d3889e206926",
	"internal/installation/model.go":         "3684d7faf3d4426533852efba153a14c0cbdd1dcdc1579fcbfa2d5cda2f44c33",
	"internal/installation/observe.go":       "80023a90d6982a1a7c022e4d5901b5d01c8a0757d0e73161d1a68616b978006f",
	"internal/installation/selection.go":     "2dd81ebb67c67bcdb0a26a9141129b4f45b4cce1c88abf5fc317b6dbd6c4ac5f",
	"internal/installedlayout/layout.go":     "c1ffbbb9a2bf4d3776367dd594df9c71135a02b90fad4d8433b7f4f501cde620",
	"internal/installation/recipe.go":        "1b2d168a9dddbc2dbba3198f65cab8b99cba2ace401ccf3aa2c0b3fb92cfddee",
	"internal/installation/roles_v1.go":      "9fcb4a7cbaf4c0eb34160f47d2e496ac9148a1e0e719299b9101b52a8ec8c853",
}

// Keep an independent slice derived from the frozen retirement transformation.
var rolesV4 = retirementRecipeRoles()
