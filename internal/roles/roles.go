// Package roles preserves the shared role routing policy.
// specs/001-go-runtime-conversion.md:307; docs/DECISION_LOG.md:1977.
package roles

// Tier classifies roles independently of model names.
func Tier(role string) string {
	switch role {
	case "spec-writer", "reviewer":
		return "frontier"
	case "refactorer", "wiki-maintainer":
		return "economy"
	default:
		return "default"
	}
}

// Resolve keeps economy routing opt-in without lowering frontier roles.
func Resolve(profile, tier string) string {
	if tier == "economy" && profile != "economy" {
		return "default"
	}
	return tier
}
