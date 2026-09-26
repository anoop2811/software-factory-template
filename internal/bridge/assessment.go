package bridge

import (
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
		data, err := json.Marshal(result)
		if err == nil {
			err = output.WriteEvent(ctx, cmd.OutOrStdout(), append(data, '\n'))
		}
		if err != nil {
			*status = 1
			return errors.New("cannot write assessment")
		}
		*status = result.Status()
		return nil
	}))
	return group
}
