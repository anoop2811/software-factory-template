// Package loopcmd provides the private, source-qualified loop command candidate.
package loopcmd

import (
	"context"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/budget"
	"github.com/anoop2811/software-factory-template/internal/commandargs"
	"github.com/anoop2811/software-factory-template/internal/loop"
	"github.com/anoop2811/software-factory-template/internal/output"
	"github.com/spf13/cobra"
)

// Run owns loop command grammar, diagnostics and effects after complete validation.
// docs/adr/0076-go-bounded-loop-controller.md:19.
func Run(parent context.Context, args []string, environment map[string]string, stdout, stderr io.Writer) int {
	fail := func(message string) int {
		_ = output.WriteEvent(context.WithoutCancel(parent), stderr, []byte("factory loop: "+message+"\n"))
		return 2
	}
	root := &cobra.Command{Use: "factory loop", Short: "Local plans, status and explicit bounded loops"}
	root.Flags().BoolP("help", "h", false, "Show command help")
	for _, name := range []string{"plan", "run", "status", "resume"} {
		root.AddCommand(&cobra.Command{Use: name, Run: func(*cobra.Command, []string) {}})
	}
	argumentFailure := func(command *cobra.Command, message string) int {
		_ = output.WriteEvent(parent, stderr, []byte(command.UsageString()+"\nfactory loop: error: "+message+"\n"))
		return 2
	}
	help := func(command *cobra.Command) int {
		if err := output.WriteEvent(parent, stdout, []byte(command.UsageString())); err != nil {
			return fail("cannot write command help")
		}
		return 0
	}
	operation, operands, rootHelp, rootUnknown, err := commandargs.SelectAction(parent, root, args)
	if err != nil {
		return argumentFailure(root, err.Error())
	}
	if rootHelp {
		return help(root)
	}
	request := loop.Request{Environment: environment}
	jsonOutput := false
	command := &cobra.Command{Use: operation, SilenceErrors: true, SilenceUsage: true, DisableSuggestions: true, Args: cobra.NoArgs}
	command.CompletionOptions.DisableDefaultCmd = true
	command.SetOut(stdout)
	command.SetErr(stderr)
	flags := command.Flags()
	flags.BoolP("help", "h", false, "Show command help")
	flags.BoolVar(&jsonOutput, "json", false, "Emit JSON metadata")
	for _, option := range []struct {
		target                *string
		name, fallback, usage string
		required              bool
		choices               []string
	}{
		{&request.Session, "session", "", "Session identifier", true, nil}, {&request.Task, "task", "", "Task identifier", true, nil},
	} {
		if err := commandargs.StringOption(command, option.target, option.name, option.fallback, option.usage, option.required); err != nil {
			return fail("cannot configure loop command")
		}
	}
	if operation != "status" {
		if err := commandargs.StringOption(command, &request.Harness, "harness", "", "Native harness: "+strings.Join(budget.Harnesses(), ", "), operation != "resume"); err != nil {
			return fail("cannot configure loop command")
		}
		if err := commandargs.Choices(command, "harness", budget.Harnesses()); err != nil {
			return fail("cannot configure loop command")
		}
		if err := commandargs.StringOption(command, &request.Mode, "mode", "manual", "Loop mode: "+strings.Join(loop.Modes(), ", "), false); err != nil {
			return fail("cannot configure loop command")
		}
		if err := commandargs.Choices(command, "mode", loop.Modes()); err != nil {
			return fail("cannot configure loop command")
		}
	}
	if operation == "run" || operation == "resume" {
		flags.StringVar(&request.PromptFile, "prompt-file", "", "Task prompt file for bounded execution")
	}
	normalized, leafHelp, unknown, err := commandargs.Normalize(parent, command, operands)
	if err != nil {
		return argumentFailure(command, err.Error())
	}
	if leafHelp {
		return help(command)
	}
	if err := command.ParseFlags(normalized); err != nil || len(flags.Args()) != 0 {
		return argumentFailure(command, "invalid option syntax")
	}
	if err := command.ValidateRequiredFlags(); err != nil {
		return argumentFailure(command, err.Error())
	}
	if rootUnknown || unknown {
		return argumentFailure(command, "unrecognized option or positional operand")
	}
	if !budget.ValidSessionID(request.Session) || !budget.ValidSessionID(request.Task) {
		return argumentFailure(command, "invalid session or task identifier")
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	checkout := environment["FACTORY_BUDGET_ROOT"]
	if checkout == "" {
		checkout, err = os.Getwd()
		if err != nil {
			return fail("cannot locate loop checkout")
		}
	}
	store := loop.NewStore(checkout)
	if operation == "status" || operation == "resume" {
		history, err := store.Read(ctx)
		if err != nil {
			return fail("cannot read loop history")
		}
		record, err := history.Lookup(ctx, request.Session, request.Task)
		if err != nil {
			return fail("checkpoint not found")
		}
		if operation == "status" {
			if err := (loop.Result{Record: &record}).WriteFormat(ctx, stdout, jsonOutput); err != nil {
				return fail("cannot write loop status")
			}
			return 0
		}
		harness, mode, err := record.Selection(ctx)
		if err != nil {
			return fail("cannot read checkpoint selection")
		}
		if request.Harness == "" {
			request.Harness = harness
		}
		if request.Harness != harness || request.Mode != mode {
			return fail("resume must retain the original harness and mode")
		}
	}
	controller := loop.NewController(checkout)
	var result loop.Result
	if operation == "plan" {
		plan, err := controller.Plan(ctx, request)
		if err != nil {
			return fail("cannot plan loop")
		}
		result.Plan = &plan
		if len(plan.Blockers) > 0 {
			result.ExitCode = 2
		}
	} else {
		result, err = controller.Run(ctx, request, operation == "resume")
		if err != nil {
			return fail("loop stopped before terminal publication; inspect checkpoint")
		}
	}
	if err := result.WriteFormat(ctx, stdout, jsonOutput); err != nil {
		return fail("cannot write loop result")
	}
	return result.ExitCode
}
