package initcmd

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/anoop2811/software-factory-template/internal/native"
)

// Retain result evidence without exposing Git inputs or captured output.
// docs/adr/0102-parallel-go-qualification.md:155.
func ignoreProofFailure(stage, message string, result native.CommandResult, cause error) error {
	switch stage {
	case "git-init", "git-boundary", "git-ls-files", "git-check-ignore":
	default:
		stage = "unknown"
	}
	outcome := "unknown"
	switch result.Outcome {
	case "completed", "failed", "launch_error", "timeout", "interrupted", "output_limit":
		outcome = result.Outcome
	}
	exit := "unavailable"
	if result.ExitCode != nil {
		exit = strconv.Itoa(*result.ExitCode)
	}
	evidence := fmt.Sprintf("%s (stage=%s outcome=%s exit=%s pid=%d elapsed_seconds=%.3f exit_confirmed=%t ownership_unconfirmed=%t)",
		message, stage, outcome, exit, result.ProcessPID, result.ElapsedSeconds, result.ExitConfirmed, result.OwnershipUnconfirmed)
	var ownership *native.OwnershipError
	if errors.As(cause, &ownership) {
		evidence += " native cleanup evidence (" + ownership.SafeDiagnostic() + ")"
	}
	return errors.Join(cause, errors.New(evidence))
}
