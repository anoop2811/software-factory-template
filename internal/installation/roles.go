package installation

// The closed minimal generation remains independent of current retirement policy.
// docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:41.
var rolesV3 = minimalRecipeRoles()

// Reviewed current selection mirrors docs/migration/INSTALLATION_ROLE_MAP.json.
// docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:51.
var roles = retirementRecipeRoles()

const obsoleteCacheReason = "obsolete initializer cache; retirement requires independently qualified predecessor ownership"

// Reconstruct the original minimal generation from frozen, inert V1 values.
// No current role or local installation metadata can enlarge this selection.
// docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:41.
func minimalRecipeRoles() []role {
	result := make([]role, 0, len(rolesV1))
	for _, row := range rolesV1 {
		if row.Operation != "inert-data" || consumedInitializerData(row.Source) {
			result = append(result, row)
		}
	}
	return result
}

// Current rows keep the original noncache selection, declare unused cache retired,
// and retain the two consumed data rows. Range values are copies of frozen data.
// docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:51.
func retirementRecipeRoles() []role {
	result := make([]role, 0, len(rolesV1))
	consumed := make([]role, 0, 2)
	for _, row := range rolesV1 {
		if row.Operation == "inert-data" {
			if consumedInitializerData(row.Source) {
				consumed = append(consumed, row)
				continue
			}
			row = role{Path: row.Path, Operation: "retire", Reason: obsoleteCacheReason}
		}
		result = append(result, row)
	}
	return append(result, consumed...)
}

func consumedInitializerData(source string) bool {
	return source == "templates/metrics.html" || source == "packs/review-lane/review-pr.yml"
}

var preservePaths = []string{"factory.yaml", "factory.config", "AGENTS.md", "CLAUDE.md", "README.md", "Makefile", "opencode.json", ".mcp.json", ".opencode/package.json", ".opencode/.gitignore", ".github/CODEOWNERS", ".github/workflows/ci.yml", "docs/DECISION_LOG.md"}
