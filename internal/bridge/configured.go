package bridge

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/budgetcmd"
	"github.com/anoop2811/software-factory-template/internal/commandenv"
	"github.com/anoop2811/software-factory-template/internal/loopcmd"
	"github.com/spf13/cobra"
)

// Configuration preparation precedes the existing command's unchanged parsing.
// docs/adr/0077-go-command-environment.md:32.
func configuredCommand(kind string, status *int) *cobra.Command {
	return command("configured", cobra.ArbitraryArgs, func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		environment := capturedEnvironment()
		var err error
		if kind == "budget" {
			environment, err = commandenv.Budget(ctx, args, environment)
		} else {
			environment, err = commandenv.Loop(ctx, environment)
		}
		if err != nil {
			*status = 2
			return err
		}
		if kind == "budget" {
			*status = budgetcmd.Run(ctx, args, environment, cmd.OutOrStdout(), cmd.ErrOrStderr())
		} else {
			*status = loopcmd.Run(ctx, args, environment, cmd.OutOrStdout(), cmd.ErrOrStderr())
		}
		return nil
	})
}
