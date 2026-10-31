package cli

import (
	"strings"

	"github.com/anoop2811/software-factory-template/internal/installationcmd"
	"github.com/anoop2811/software-factory-template/internal/recoverycmd"
	"github.com/anoop2811/software-factory-template/internal/releasecmd"
	"github.com/anoop2811/software-factory-template/internal/upgradecmd"
)

// UpgradeOwnsAdmission selects public owners before retaining an installed SH
// lease. Each owner acquires its own required admission mode.
// docs/adr/0122-default-packaged-go-upgrade.md:71.
func UpgradeOwnsAdmission(args []string) bool {
	if installationcmd.Claimed(args) || releasecmd.Claimed(args) || releasecmd.BackupClaimed(args) || releasecmd.LocalUpgradeClaimed(args) {
		return true
	}
	return !legacyUpgradeClaimed(args)
}

// Bare release previews use packaged delivery; explicit source assessments and
// historical recovery markers retain their existing source-only owner.
// docs/adr/0122-default-packaged-go-upgrade.md:31.
func legacyUpgradeClaimed(args []string) bool {
	if recoverycmd.Claimed(args) {
		return true
	}
	if upgradecmd.Claimed(args) {
		for _, arg := range args {
			if arg == "--source" || strings.HasPrefix(arg, "--source=") {
				return true
			}
		}
	}
	return false
}
