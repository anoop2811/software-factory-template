package upgradecmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/assessment"
)

type previewReport struct {
	SchemaVersion      int                           `json:"schema_version"`
	Mode               string                        `json:"mode"`
	Coverage           string                        `json:"coverage"`
	Plan               assessment.PlanResult         `json:"plan"`
	AdoptionProposal   *assessment.Proposal          `json:"adoption_proposal"`
	OwnershipBasis     string                        `json:"ownership_basis"`
	AuthorizedPaths    []string                      `json:"authorized_paths"`
	RecoveryAssessment string                        `json:"recovery_assessment"`
	RecoveryInventory  *assessment.RecoveryInventory `json:"recovery_inventory,omitempty"`
}

// Text names the incomplete scope and every remaining prerequisite without an
// executable confirmation command or unassessed recovery claims.
// docs/adr/0082-go-public-upgrade-preview.md:76.
func renderText(report previewReport, confirmed bool) string {
	var text strings.Builder
	text.WriteString("Read-only upgrade preview\nCoverage: partial; six legacy budget/loop paths in the current directory.\n")
	fmt.Fprintf(&text, "Scope: %s\n", report.Plan.Scope)
	for _, asset := range report.Plan.Assets {
		fmt.Fprintf(&text, "%s: %s (%s)\n", asset.Path, asset.Action, asset.Reason)
	}
	text.WriteString("Action counts:\n")
	for _, action := range []string{"retain", "replace_candidate", "add_candidate", "retire_candidate", "preserve_customized", "absent", "conflict", "assessment_error"} {
		fmt.Fprintf(&text, "  %s: %d\n", action, report.Plan.Counts[action])
	}
	fmt.Fprintf(&text, "Ownership basis: %s\nAuthorized paths: %d of 6\nRollback ready: %t\nApplicable: %t\n", report.OwnershipBasis, len(report.AuthorizedPaths), report.Plan.RollbackReady, report.Plan.Applicable)
	text.WriteString("Remaining blockers:\n")
	for _, blocker := range report.Plan.Blockers {
		fmt.Fprintf(&text, "  %s\n", blocker)
	}
	if report.AdoptionProposal != nil {
		fmt.Fprintf(&text, "Proposal digest: %s\nConfirmation supplied: %t\nSelected paths:\n", report.AdoptionProposal.ProposalDigest, confirmed)
		for _, asset := range report.AdoptionProposal.Assets {
			fmt.Fprintf(&text, "  %s\n", asset.Path)
		}
		if !confirmed {
			text.WriteString("To confirm, rerun the same selection with --confirm-adoption DIGEST using the proposal digest above.\n")
		}
	}
	if report.RecoveryInventory == nil {
		text.WriteString("Backup and retention: not assessed. No backup changes made.\n")
	} else {
		inventory := report.RecoveryInventory
		fmt.Fprintf(&text, "Recovery inventory: %s; enumeration complete: %t\n", inventory.RootStatus, inventory.Complete)
		for _, set := range inventory.Sets {
			held := "unknown"
			if set.Held != nil {
				held = strconv.FormatBool(*set.Held)
			}
			fmt.Fprintf(&text, "  %q: %s (%s); next action: %s; files: %d; bytes: %d; held: %s\n", set.Path, set.Classification, set.Reason, set.NextAction, set.FileCount, set.Bytes, held)
		}
		fmt.Fprintf(&text, "Recovery counts: sets=%d files=%d bytes=%d\n", inventory.SetCount, inventory.FileCount, inventory.Bytes)
		text.WriteString("Restorable: false. Prune authorized: false. Integrity observations do not establish transaction authority. No backup changes made.\n")
	}
	text.WriteString("Legacy upgrade is separate: removing --dry-run selects the existing upgrade script; it does not apply this Go plan.\n")
	return text.String()
}
