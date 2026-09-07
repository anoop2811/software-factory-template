// Command factory-stage authenticates and stages inert runtime bundles.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/anoop2811/software-factory-template/internal/staging"
	"github.com/spf13/cobra"
)

func main() {
	var options staging.Options
	status := 2
	cmd := &cobra.Command{Use: "factory-stage", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			options.ExplicitVerification = cmd.Flags().Changed("gh") || cmd.Flags().Changed("attestation") || cmd.Flags().Changed("trusted-root")
			result, err := staging.Stage(cmd.Context(), options)
			if err != nil {
				if !errors.Is(err, staging.ErrInvalidRequest) {
					status = 1
				}
				return err
			}
			status = 1
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&options.Archive, "archive", "", "Runtime archive")
	flags.StringVar(&options.Version, "version", "", "Expected literal version")
	flags.StringVar(&options.Revision, "revision", "", "Expected full lowercase commit SHA")
	flags.StringVar(&options.Target, "target", "", "Expected GOOS/GOARCH")
	flags.StringVar(&options.Output, "output", "", "New staging directory (must not exist)")
	flags.StringVar(&options.Attestation, "attestation", "", "Offline attestation bundle")
	flags.StringVar(&options.TrustedRoot, "trusted-root", "", "Independently provisioned trusted roots")
	flags.StringVar(&options.Verifier, "gh", "", "Trusted GitHub CLI executable (default gh on PATH)")
	flags.BoolVar(&options.Local, "local", false, "Explicit unauthenticated source-build mode")
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(status)
	}
}
