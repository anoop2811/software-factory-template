package evalcmd

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

const runnerOwnershipToken = "FACTORY_EVAL_OWNERSHIP_TOKEN"

// Private receipts carry nested results without reserving an exit code.
// docs/adr/0133-go-source-evaluation-runners.md:116.
func writeRunnerReceipt(stderr io.Writer, token, status string) {
	if !validRunnerToken(token) {
		return
	}
	fmt.Fprintf(stderr, "factory-eval-result-v1 %s %s\n", token, status)
}

func validRunnerToken(token string) bool {
	return len(token) == 64 && strings.Trim(token, "0123456789abcdef") == ""
}

// Only complete, exact lines from this attempt can affect evaluation.
// docs/adr/0133-go-source-evaluation-runners.md:120.
func readRunnerReceipt(output []byte, token string) string {
	if !validRunnerToken(token) {
		return ""
	}
	prefix := "factory-eval-result-v1 " + token + " "
	status := ""
	pending := false
	for len(output) > 0 {
		line, remaining, complete := bytes.Cut(output, []byte{'\n'})
		if !complete {
			break
		}
		output = remaining
		switch string(line) {
		// Completion closes only an earlier handoff and cannot erase rejection.
		// docs/adr/0133-go-source-evaluation-runners.md:204.
		case prefix + "handoff-pending":
			pending = true
		case prefix + "completed":
			pending = false
		case prefix + "ownership-unconfirmed":
			return "ownership-unconfirmed"
		case prefix + "failed":
			status = "failed"
		}
	}
	if pending {
		return "failed"
	}
	return status
}
