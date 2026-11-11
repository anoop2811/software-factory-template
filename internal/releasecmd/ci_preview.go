package releasecmd

import (
	"fmt"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/installation"
)

// Show whether the release owner included CI in installation confirmation.
// docs/adr/0148-transactional-installed-ci-migration.md:39.
func ciWorkflowReport(report installation.CIWorkflowPreview) string {
	var text strings.Builder
	if report.ApplySupported {
		text.WriteString("\nCI workflow migration (included in installation confirmation)\n")
	} else {
		text.WriteString("\nCI workflow preview (read-only; apply unsupported)\n")
	}
	text.WriteString(report.Reason + "\n")
	for _, change := range report.Changes {
		fmt.Fprintf(&text, "  %s %s: %s\n", change.Disposition, change.Path, change.Reason)
	}
	for _, issue := range report.Conflicts {
		fmt.Fprintf(&text, "  CI conflict %s: %s\n", issue.Path, issue.Reason)
	}
	return text.String()
}
