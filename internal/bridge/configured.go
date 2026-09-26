package bridge

import (
	"github.com/anoop2811/software-factory-template/internal/configuredcmd"
	"github.com/spf13/cobra"
)

// Both entrypoints share preparation and controller behavior without fallback.
// docs/adr/0078-go-public-budget-loop.md:56.
func configuredCommand(kind string, status *int) *cobra.Command {
	return command("configured", cobra.ArbitraryArgs, func(cmd *cobra.Command, args []string) error {
		code, err := configuredcmd.Run(cmd.Context(), kind, args, capturedEnvironment(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		*status = code
		return err
	})
}
