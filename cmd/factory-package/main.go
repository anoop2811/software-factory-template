// Command factory-package builds developer bundles from explicit committed source.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/packaging"
	"github.com/spf13/cobra"
)

func main() { os.Exit(run()) }

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var options packaging.Options
	status := 2
	cmd := &cobra.Command{Use: "factory-package", Short: "Build an inert runtime bundle from committed source", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := packaging.Build(cmd.Context(), options)
			if err != nil {
				if !errors.Is(err, packaging.ErrInvalidRequest) {
					status = 1
				}
				return err
			}
			status = 1
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&options.Source, "source", "", "Git repository containing committed runtime source")
	flags.StringVar(&options.Revision, "revision", "", "Full lowercase commit SHA")
	flags.StringVar(&options.Target, "target", "", "Target GOOS/GOARCH")
	flags.StringVar(&options.Output, "output", "", "New bundle directory (must not exist)")
	flags.StringVar(&options.Version, "version", "", "Literal version label (default local-REVISION)")
	flags.StringVar(&options.Compiler, "go", "", "Explicit pinned Go compiler")
	flags.BoolVar(&options.Installation, "installation", false, "Include the inert committed installation source image")
	if err := cmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return status
	}
	return 0
}
