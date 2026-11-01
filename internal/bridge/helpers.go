package bridge

import (
	"fmt"

	"github.com/anoop2811/software-factory-template/internal/copymanifest"
	"github.com/anoop2811/software-factory-template/internal/helpers"
	"github.com/anoop2811/software-factory-template/internal/sourcebootstrap"
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
	group.AddCommand(command("copies", cobra.ExactArgs(1), func(cmd *cobra.Command, args []string) error {
		root, _ := cmd.Context().Value(installedRootKey{}).(string)
		*status = copymanifest.Run(cmd.Context(), args[0], root, capturedEnvironment(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		return nil
	}))
	// Publish only the source launcher's private compiler output.
	// docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:48.
	group.AddCommand(command("publish-source", cobra.NoArgs, func(cmd *cobra.Command, _ []string) error {
		if err := sourcebootstrap.Publish(cmd.Context()); err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "factory source:", err)
			*status = 1
		}
		return nil
	}))
	return group
}
