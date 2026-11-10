package ciworkflow

import "golang.org/x/mod/semver"

// EligibleRelease retains the preparation parser's full canonical tag policy.
// Canonical semver is a subset of the literal release-tag character grammar;
// keep its original 128-byte bound, including build metadata.
// docs/adr/0146-initialized-installed-ci.md:184.
func EligibleRelease(tag string) bool {
	return len(tag) <= 128 && semver.IsValid(tag) && semver.Canonical(tag)+semver.Build(tag) == tag
}
