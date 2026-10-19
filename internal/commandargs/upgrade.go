package commandargs

import "strings"

// LegacyUpgradeMarker reserves a marker without reinterpreting source/ref values.
// Explicit whole-installation/candidate selectors retain their own parsers.
// docs/adr/0106-go-backup-retention.md:133.
func LegacyUpgradeMarker(args []string, markers ...string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--source" || arg == "--ref" {
			i++
			continue
		}
		for _, marker := range markers {
			if arg == marker || strings.HasPrefix(arg, marker+"=") {
				return true
			}
		}
	}
	return false
}
