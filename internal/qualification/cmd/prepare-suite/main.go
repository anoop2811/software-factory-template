// Command prepare-suite manages repository-only compiled qualification inputs.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/qualification"
	"github.com/spf13/cobra"
)

func main() { os.Exit(run()) }

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	status := 2
	command := &cobra.Command{Use: "prepare-suite", Short: "Prepare or verify repository qualification inputs", SilenceUsage: true, SilenceErrors: true, RunE: func(*cobra.Command, []string) error { return errors.New("prepare or verify operation is required") }}
	for _, name := range []string{"prepare", "verify"} {
		var options qualification.PreparedOptions
		operation := &cobra.Command{Use: name, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			status = 1
			// docs/adr/0151-reliable-ci-qualification.md:55 -- repository-only qualification.
			var receipt qualification.PreparedReceipt
			var err error
			if name == "prepare" {
				receipt, err = qualification.Prepare(cmd.Context(), options)
			} else {
				receipt, err = qualification.VerifyPrepared(cmd.Context(), options)
			}
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(receipt)
		}}
		operation.Flags().StringVar(&options.Root, "root", "", "Absolute checked-out source directory")
		operation.Flags().StringVar(&options.Revision, "revision", "", "Actual checked-out commit SHA")
		operation.Flags().StringVar(&options.RunID, "run-id", "", "Workflow run identity")
		for _, flag := range []string{"root", "revision", "run-id"} {
			if err := operation.MarkFlagRequired(flag); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return status
			}
		}
		if name == "prepare" {
			operation.Flags().StringVar(&options.ProducerAttempt, "producer-attempt", "", "Informational producer attempt")
			if err := operation.MarkFlagRequired("producer-attempt"); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return status
			}
		}
		command.AddCommand(operation)
	}
	if err := command.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return status
	}
	return 0
}
