package bridge

import (
	"github.com/anoop2811/software-factory-template/internal/helpers"
	"github.com/spf13/cobra"
)

// The private helper group delegates all literal protocol admission and effects.
// docs/adr/0099-native-factory-orchestration.md:134.
func helperCommand(status *int) *cobra.Command {
	group := command("helper", cobra.ArbitraryArgs, func(cmd *cobra.Command, args []string) error {
		*status = helpers.Run(cmd.Context(), args, capturedEnvironment(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		return nil
	})
	for _, operation := range []string{"event", "now", "duration", "palette", "action-box", "hookspath", "budget-plan", "budget-model"} {
		group.AddCommand(command(operation, cobra.ArbitraryArgs, func(cmd *cobra.Command, args []string) error {
			*status = helpers.Run(cmd.Context(), append([]string{cmd.Name()}, args...), capturedEnvironment(), cmd.OutOrStdout(), cmd.ErrOrStderr())
			return nil
		}))
	}
	return group
}
