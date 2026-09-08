// Package bridge provides the private sourceable-library protocol.
// It does not install or activate public adapters. docs/DECISION_LOG.md:1960.
package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/anoop2811/software-factory-template/internal/artifact"
	"github.com/anoop2811/software-factory-template/internal/config"
	"github.com/anoop2811/software-factory-template/internal/roles"
	"github.com/anoop2811/software-factory-template/internal/usage"
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
	// Mutation has literal arguments and preserves the legacy missing-file diagnostic.
	// docs/adr/0063-go-configuration-writes.md:15.
	configCommand.AddCommand(command("set", configArgs(2, 2), func(cmd *cobra.Command, args []string) error {
		path, err := config.File(cmd.Context())
		if err == nil {
			err = config.Set(cmd.Context(), path, args[0], args[1])
		}
		if err != nil {
			status = 1
			fmt.Fprintln(cmd.ErrOrStderr(), err)
		}
		return nil
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
	// Plans carry literal data for parent-shell effects, not generated shell code.
	// docs/adr/0056-go-configuration-export-plans.md:10.
	configCommand.AddCommand(command("export", preservedArgs(0), func(cmd *cobra.Command, args []string) error {
		path, err := config.File(cmd.Context())
		if err != nil {
			status = 1
			return err
		}
		actions, err := config.ExportPlan(cmd.Context(), path, args)
		if err != nil {
			status = 1
			return err
		}
		if err := config.WritePlan(cmd.OutOrStdout(), actions); err != nil {
			status = 1
			return err
		}
		return nil
	}))
	configCommand.AddCommand(command("legacy", preservedArgs(1), func(cmd *cobra.Command, args []string) error {
		actions, err := config.LegacyPlan(cmd.Context(), args[0], args[1:])
		if err != nil {
			status = 1
			return err
		}
		return config.WritePlan(cmd.OutOrStdout(), actions)
	}))
	// Shell-expanded operands remain literal protocol data, including flags.
	// docs/adr/0062-go-local-hook-normalization.md:15.
	configCommand.AddCommand(command("hooks", cobra.MinimumNArgs(1), func(cmd *cobra.Command, args []string) error {
		value, err := config.Hooks(cmd.Context(), args[0], args[1:])
		if err != nil {
			return err
		}
		return writeValue(cmd, value)
	}))
	roleCommand := command("role", cobra.NoArgs, nil)
	roleCommand.AddCommand(command("tier", cobra.ExactArgs(1), func(cmd *cobra.Command, args []string) error {
		return writeValue(cmd, roles.Tier(args[0]))
	}))
	roleCommand.AddCommand(command("resolve", cobra.ExactArgs(2), func(cmd *cobra.Command, args []string) error {
		return writeValue(cmd, roles.Resolve(args[0], args[1]))
	}))
	runtimeCommand := command("runtime", cobra.NoArgs, nil)
	emitMetadata := func(cmd *cobra.Command, metadata artifact.Metadata) error {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", metadata.Version, metadata.Target, metadata.Binary, metadata.SHA256)
		if err != nil {
			status = 1
		}
		return err
	}
	runtimeCommand.AddCommand(command("verify", cobra.ExactArgs(3), func(cmd *cobra.Command, args []string) error {
		metadata, err := artifact.Verify(cmd.Context(), args[0], args[1], args[2])
		if err != nil {
			if !errors.Is(err, artifact.ErrInvalidRequest) {
				status = 1
			}
			return err
		}
		return emitMetadata(cmd, metadata)
	}))
	runtimeCommand.AddCommand(command("resolve", cobra.ExactArgs(3), func(cmd *cobra.Command, args []string) error {
		metadata, err := artifact.Resolve(cmd.Context(), args[0], args[1], args[2])
		if err != nil {
			if !errors.Is(err, artifact.ErrInvalidRequest) {
				status = 1
			}
			return err
		}
		return emitMetadata(cmd, metadata)
	}))
	// Admit the harness before touching stdin; emit metadata only.
	// docs/adr/0064-go-native-usage-accounting.md:17.
	usageCommand := command("usage", cobra.NoArgs, nil)
	usageCommand.AddCommand(command("normalize", func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(1)(cmd, args); err != nil {
			return err
		}
		if !usage.Supported(args[0]) {
			return errors.New("unsupported usage harness")
		}
		return nil
	}, func(cmd *cobra.Command, args []string) error {
		metadata, err := usage.Normalize(cmd.Context(), args[0], cmd.InOrStdin())
		if err != nil {
			status = 1
			return err
		}
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(metadata); err != nil {
			status = 1
			return errors.New("cannot write usage metadata")
		}
		return nil
	}))
	root.AddCommand(configCommand, roleCommand, runtimeCommand, usageCommand)
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
		if status != 0 {
			return status
		}
		return 2
	}
	return status
}

func preservedArgs(offset int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.MinimumNArgs(offset)(cmd, args); err != nil {
			return err
		}
		for _, key := range args[offset:] {
			if !config.KnownExportKey(key) {
				return fmt.Errorf("invalid preserved configuration key %q", key)
			}
		}
		return nil
	}
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
