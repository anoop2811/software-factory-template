// Package bridge provides the private, read-only sourceable-library protocol.
// It does not install or activate public adapters. docs/DECISION_LOG.md:1960.
package bridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/anoop2811/software-factory-template/internal/config"
	"github.com/anoop2811/software-factory-template/internal/roles"
	"github.com/spf13/cobra"
)

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// Run executes the explicitly selected private protocol and returns its status.
// Read failures retain get's fallback and has's error. docs/DECISION_LOG.md:2004.
func Run(ctx context.Context, args []string) int {
	if os.Getenv("FACTORY_BRIDGE_PROTOCOL") != "1" {
		fmt.Fprintln(os.Stderr, "factory bridge: unsupported FACTORY_BRIDGE_PROTOCOL")
		return 2
	}
	status := 0
	root := command("factory-bridge", cobra.NoArgs, nil)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		return cmd.Context().Err()
	}
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.SetHelpCommand(command("help", cobra.ArbitraryArgs, nil))

	configCommand := command("config", cobra.NoArgs, nil)
	configCommand.AddCommand(command("file", cobra.NoArgs, func(cmd *cobra.Command, _ []string) error {
		path, err := config.File(cmd.Context())
		if err != nil {
			return err
		}
		return writeValue(cmd, path)
	}))
	configCommand.AddCommand(command("get", configArgs(1, 2), func(cmd *cobra.Command, args []string) error {
		path, err := config.File(cmd.Context())
		if err != nil {
			return err
		}
		fallback := ""
		if len(args) == 2 {
			fallback = args[1]
		}
		value, err := config.Get(cmd.Context(), path, args[0], fallback)
		if err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "factory bridge:", err)
		}
		return writeValue(cmd, value)
	}))
	configCommand.AddCommand(command("has", configArgs(1, 1), func(cmd *cobra.Command, args []string) error {
		path, err := config.File(cmd.Context())
		if err != nil {
			return err
		}
		present, err := config.Has(cmd.Context(), path, args[0])
		if err != nil {
			return err
		}
		if !present {
			status = 1
		}
		return nil
	}))
	roleCommand := command("role", cobra.NoArgs, nil)
	roleCommand.AddCommand(command("tier", cobra.ExactArgs(1), func(cmd *cobra.Command, args []string) error {
		return writeValue(cmd, roles.Tier(args[0]))
	}))
	roleCommand.AddCommand(command("resolve", cobra.ExactArgs(2), func(cmd *cobra.Command, args []string) error {
		return writeValue(cmd, roles.Resolve(args[0], args[1]))
	}))
	root.AddCommand(configCommand, roleCommand)
	// Cobra initializes hidden completion commands even when its default
	// completion command is disabled. Admit only the literal registered request
	// pair, keeping help, completion and flag-like command tokens out of protocol 1.
	if !registeredRequest(root, args) {
		fmt.Fprintln(os.Stderr, "factory bridge: unsupported request")
		return 2
	}
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "factory bridge:", err)
		return 2
	}
	return status
}

func command(name string, args cobra.PositionalArgs, run func(*cobra.Command, []string) error) *cobra.Command {
	if run == nil {
		run = func(*cobra.Command, []string) error { return errors.New("unsupported request") }
	}
	return &cobra.Command{
		Use:                name,
		DisableFlagParsing: true,
		DisableSuggestions: true,
		Args:               args,
		RunE:               run,
	}
}

func configArgs(minimum, maximum int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.RangeArgs(minimum, maximum)(cmd, args); err != nil {
			return err
		}
		if !identifier.MatchString(args[0]) {
			return fmt.Errorf("invalid configuration key %q", args[0])
		}
		return nil
	}
}

func registeredRequest(root *cobra.Command, args []string) bool {
	if len(args) < 2 {
		return false
	}
	for _, group := range root.Commands() {
		if group.Name() != args[0] {
			continue
		}
		for _, request := range group.Commands() {
			if request.Name() == args[1] {
				return true
			}
		}
	}
	return false
}

func writeValue(cmd *cobra.Command, value string) error {
	_, err := io.WriteString(cmd.OutOrStdout(), value)
	return err
}
