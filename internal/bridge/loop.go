package bridge

import (
	"encoding/json"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/budget"
	"github.com/anoop2811/software-factory-template/internal/input"
	"github.com/anoop2811/software-factory-template/internal/loop"
	"github.com/anoop2811/software-factory-template/internal/output"
	"github.com/spf13/cobra"
)

func loopCommands() *cobra.Command {
	loopCommand := command("loop", cobra.NoArgs, nil)
	loopCommand.AddCommand(command("fingerprint", cobra.NoArgs, func(cmd *cobra.Command, _ []string) error {
		// Interrupts cancel and reap isolated probes before this private command returns.
		// docs/adr/0073-go-loop-fingerprint-foundation.md:141.
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		environment := capturedEnvironment()
		checkout := environment["FACTORY_LOOP_ROOT"]
		if checkout == "" {
			var err error
			checkout, err = os.Getwd()
			if err != nil {
				return errors.New("cannot establish loop fingerprint")
			}
		}
		configuration, err := loop.Configuration(ctx, environment)
		if err != nil {
			return errors.New("invalid loop configuration")
		}
		budgetConfiguration, err := budget.Configuration(environment)
		if err != nil {
			return errors.New("invalid loop budget configuration")
		}
		snapshot, err := loop.StableSnapshot(ctx, checkout, configuration, environment)
		if err != nil {
			return errors.New("cannot establish loop source snapshot")
		}
		policy, err := loop.Policy(ctx, configuration, budgetConfiguration, environment)
		if err != nil {
			return errors.New("cannot fingerprint loop policy")
		}
		data, err := json.Marshal(struct {
			Configuration loop.Config      `json:"configuration"`
			Snapshot      loop.Fingerprint `json:"snapshot"`
			Policy        string           `json:"policy"`
		}{configuration, snapshot, policy})
		if err != nil {
			return errors.New("cannot encode loop fingerprint")
		}
		data = append(data, '\n')
		if err := output.WriteEvent(ctx, cmd.OutOrStdout(), data); err != nil {
			return errors.New("cannot write loop fingerprint")
		}
		return nil
	}))
	loopCommand.AddCommand(command("checkpoint", cobra.ArbitraryArgs, runCheckpoint))
	return loopCommand
}

// Validate the complete literal request before Cobra or storage can reflect input.
// docs/adr/0074-go-loop-checkpoint-storage.md:123.
func loopRequest(args []string) bool {
	if len(args) == 2 && args[1] == "fingerprint" {
		return true
	}
	if len(args) < 3 || args[1] != "checkpoint" {
		return false
	}
	switch args[2] {
	case "read", "roundtrip":
		return len(args) == 3
	case "status", "resume-check":
		return len(args) == 5 && budget.ValidSessionID(args[3]) && budget.ValidSessionID(args[4])
	default:
		return false
	}
}
func runCheckpoint(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	environment := capturedEnvironment()
	root := environment["FACTORY_LOOP_ROOT"]
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return errors.New("cannot locate loop checkpoint")
		}
	}
	var request loop.ResumeRequest
	if args[0] == "resume-check" {
		source, cleanup, err := input.Cancellable(ctx, cmd.InOrStdin())
		if err != nil {
			return errors.New("cannot read loop resume request")
		}
		request, err = loop.ParseResumeRequest(ctx, source, args[1], args[2])
		cleanupErr := cleanup()
		if cleanupErr != nil {
			return errors.New("cannot release loop resume input")
		}

		if err != nil {
			return errors.New("invalid loop resume request")
		}
	}
	store := loop.NewStore(root)
	var result any
	if args[0] == "roundtrip" {
		transaction, err := store.Lock(ctx)
		if err != nil {
			return errors.New("cannot lock loop checkpoint")
		}
		defer transaction.Close()
		history, err := transaction.Read(ctx)
		if err != nil {
			return errors.New("cannot read loop checkpoint")
		}
		if err := transaction.Write(ctx, history); err != nil {
			return errors.New("cannot publish loop checkpoint; inspect storage before proceeding")
		}
		result = history
	} else {
		history, err := store.Read(ctx)
		if err != nil {
			return errors.New("cannot read loop checkpoint")
		}
		switch args[0] {
		case "read":
			result = history
		case "status":
			record, err := history.Lookup(ctx, args[1], args[2])
			if err != nil {
				return errors.New("cannot select loop checkpoint")
			}
			result = record
		case "resume-check":
			ledger, err := budget.NewLedger(root).Read(ctx)
			if err != nil {
				return errors.New("cannot assess loop checkpoint")
			}
			assessment, err := loop.AssessResume(ctx, history, ledger, request)
			if err != nil {
				return errors.New("loop checkpoint is not eligible for resume")
			}
			result = assessment
		}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return errors.New("cannot encode loop checkpoint")
	}
	if err := output.WriteEvent(ctx, cmd.OutOrStdout(), append(data, '\n')); err != nil {
		return errors.New("cannot write loop checkpoint")
	}
	return nil
}
