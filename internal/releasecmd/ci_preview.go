package releasecmd

import (
	"fmt"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/installation"
)

// Keep informational CI conflicts separate from installation confirmation advice.
// docs/adr/0147-installed-ci-migration-preview.md:35.
func ciWorkflowReport(report installation.CIWorkflowPreview) string {
	var text strings.Builder
	text.WriteString("\nCI workflow preview (read-only; apply unsupported)\n")
	text.WriteString(report.Reason + "\n")
	for _, change := range report.Changes {
		fmt.Fprintf(&text, "  %s %s: %s\n", change.Disposition, change.Path, change.Reason)
	}
	for _, issue := range report.Conflicts {
		fmt.Fprintf(&text, "  CI conflict %s: %s\n", issue.Path, issue.Reason)
	}
	return text.String()
}
