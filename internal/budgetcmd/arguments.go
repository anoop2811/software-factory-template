package budgetcmd

import (
	"context"
	"github.com/anoop2811/software-factory-template/internal/budget"
	"github.com/anoop2811/software-factory-template/internal/commandargs"
	"github.com/spf13/cobra"
)

var commandNames = [...]string{"plan", "run", "report"}

func rootCommand() *cobra.Command {
	command := &cobra.Command{Use: "factory budget", Short: "Local budget plans, explicit runs, and reports"}
	command.Flags().BoolP("help", "h", false, "Show command help")
	for _, name := range commandNames {
		command.AddCommand(&cobra.Command{Use: name, Short: "Budget " + name, Run: func(*cobra.Command, []string) {}})
	}
	return command
}
func selectAction(ctx context.Context, command *cobra.Command, args []string) (string, []string, bool, bool, error) {
	return commandargs.SelectAction(ctx, command, args)
}
func normalizeArguments(ctx context.Context, command *cobra.Command, args []string) ([]string, bool, bool, error) {
	for name, values := range map[string][]string{"harness": budget.Harnesses(), "role": budget.Roles()} {
		if command.Flags().Lookup(name) != nil {
			if err := commandargs.Choices(command, name, values); err != nil {
				return nil, false, false, err
			}
		}
	}
	return commandargs.Normalize(ctx, command, args)
}
func stringOption(command *cobra.Command, target *string, name, fallback, usage string, required bool) error {
	return commandargs.StringOption(command, target, name, fallback, usage, required)
}
