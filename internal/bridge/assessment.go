package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/assessment"
	"github.com/anoop2811/software-factory-template/internal/output"
	"github.com/spf13/cobra"
)

// Reference matches are read-only observations, never ownership authority.
// docs/adr/0079-go-installation-reference-assessment.md:22.
func assessmentCommands(status *int) *cobra.Command {
	group := command("migration", cobra.NoArgs, nil)
	group.AddCommand(command("assess", cobra.ExactArgs(1), func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		result, err := assessment.Assess(ctx, args[0])
		if err != nil {
			*status = assessment.ErrorStatus(err)
			return err
		}
		return writeAssessment(ctx, cmd, result, result.Status(), status)
	}))
	// Local target comparisons remain blocked proposals, not apply authority.
	// docs/adr/0080-go-migration-action-planning.md:29.
	group.AddCommand(command("plan", cobra.ExactArgs(2), func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		result, err := assessment.Plan(ctx, args[0], args[1])
		if err != nil {
			*status = assessment.ErrorStatus(err)
			return err
		}
		return writeAssessment(ctx, cmd, result, result.Status(), status)
	}))
	return group
}

func writeAssessment(ctx context.Context, cmd *cobra.Command, result any, code int, status *int) error {
	data, err := json.Marshal(result)
	if err == nil {
		err = output.WriteEvent(ctx, cmd.OutOrStdout(), append(data, '\n'))
	}
	if err != nil {
		*status = 1
		return errors.New("cannot write assessment")
	}
	*status = code
	return nil
}

func migrationRequest(args []string) bool {
	if len(args) < 2 {
		return false
	}
	return (args[1] == "assess" && len(args) == 3) || (args[1] == "plan" && len(args) == 4)
}
