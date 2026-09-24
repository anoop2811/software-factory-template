// Package budgetcmd provides the private source budget command candidate.
// docs/adr/0071-go-budget-command-candidate.md:7.
package budgetcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/budget"
	"github.com/anoop2811/software-factory-template/internal/output"
	"github.com/spf13/cobra"
)

// Run owns diagnostics and status; the bridge only forwards literal operands.
// docs/adr/0071-go-budget-command-candidate.md:22.
func Run(ctx context.Context, args []string, environment map[string]string, stdout, stderr io.Writer) int {
	fail := func(message string, code int) int {
		_, _ = fmt.Fprintln(stderr, "factory budget: "+message)
		return code
	}
	root := rootCommand()
	operation, operands, rootHelp, rootUnknown, err := selectAction(ctx, root, args)
	if err != nil {
		return argumentFailure(ctx, stderr, root, err.Error())
	}
	if rootHelp {
		return showHelp(ctx, stdout, root, fail)
	}
	var request budget.Request
	var prompt, maxCost string
	var jsonOutput bool
	command := &cobra.Command{Use: operation, SilenceErrors: true, SilenceUsage: true, DisableSuggestions: true, Args: cobra.NoArgs}
	command.CompletionOptions.DisableDefaultCmd = true
	command.SetOut(stdout)
	command.SetErr(stderr)
	flags := command.Flags()
	flags.BoolP("help", "h", false, "Show command help")
	flags.BoolVar(&jsonOutput, "json", false, "Emit JSON metadata")
	if err := stringOption(command, &request.Session, "session", "", "Session identifier", operation != "report"); err != nil {
		return fail("cannot configure budget command", 2)
	}
	if operation != "report" {
		if err := stringOption(command, &request.Task, "task", "", "Task identifier", true); err != nil {
			return fail("cannot configure budget command", 2)
		}
		if err := stringOption(command, &request.Harness, "harness", "", "Native harness: "+strings.Join(budget.Harnesses(), ", "), true); err != nil {
			return fail("cannot configure budget command", 2)
		}
		flags.StringVar(&request.Role, "role", "implementer", "Factory role: "+strings.Join(budget.Roles(), ", "))
		flags.StringVar(&maxCost, "max-cost-usd", "", "Requested strict USD ceiling")
	}
	if operation == "run" {
		if err := stringOption(command, &prompt, "prompt-file", "", "Prompt file path", true); err != nil {
			return fail("cannot configure budget command", 2)
		}
	}
	status := 0
	invoked := false
	command.SetHelpFunc(func(*cobra.Command, []string) { status = 2 })
	command.RunE = func(cmd *cobra.Command, _ []string) error {
		invoked = true
		if err := cmd.Context().Err(); err != nil {
			return errors.New("budget command context ended")
		}
		if flags.Changed("max-cost-usd") {
			request.MaxCostUSD = &maxCost
		}
		request.Model = environment["FACTORY_BUDGET_MODEL"]
		root := environment["FACTORY_BUDGET_ROOT"]
		if root == "" {
			var err error
			root, err = os.Getwd()
			if err != nil {
				return errors.New("cannot locate budget checkout")
			}
		}
		ledger := budget.NewLedger(root)
		if operation == "report" {
			history, err := ledger.Read(cmd.Context())
			if err != nil {
				return err
			}
			report, err := budget.Report(cmd.Context(), history, request.Session)
			if err != nil {
				return err
			}
			return budget.RenderReport(cmd.Context(), stdout, report, jsonOutput)
		}
		configuration, err := budget.Configuration(environment)
		if err != nil {
			return err
		}
		if operation == "plan" {
			history, err := ledger.Read(cmd.Context())
			if err != nil {
				return err
			}
			plan, err := budget.MakePlan(cmd.Context(), request, configuration, history)
			if err != nil {
				return err
			}
			if err := budget.RenderPlan(cmd.Context(), stdout, plan, jsonOutput); err != nil {
				return err
			}
			if len(plan.Blockers) > 0 {
				status = 2
			}
			return nil
		}
		result, runErr := budget.NewRunner(root).Run(cmd.Context(), request, configuration, budget.RunInput{
			PromptFile: prompt, Environment: environment, WantResponse: !jsonOutput,
			OnPlan: func(plan budget.Plan) error { return budget.RenderPlan(cmd.Context(), stdout, plan, jsonOutput) },
		})
		status = result.ExitCode
		// Final durable metadata survives parent cancellation, with bounded work.
		// docs/adr/0071-go-budget-command-candidate.md:127.
		outputCtx, cancelOutput := context.WithTimeout(context.WithoutCancel(cmd.Context()), 5*time.Second)
		defer cancelOutput()
		if result.Record != nil {
			if err := budget.RenderRecord(outputCtx, stdout, *result.Record, jsonOutput); err != nil {
				status = 1
				return err
			}
		}
		if runErr != nil {
			return runErr
		}
		if status == 0 && !jsonOutput && result.Response != "" {
			if err := budget.RenderAnswer(outputCtx, stdout, result.Response); err != nil {
				status = 1
				return err
			}
		}
		return nil
	}
	// Classify legacy grammar before Cobra's leaf parse and hidden discovery.
	// docs/adr/0072-go-budget-argument-compatibility.md:54.
	normalized, help, unknown, err := normalizeArguments(ctx, command, operands)
	if err != nil {
		return argumentFailure(ctx, stderr, command, err.Error())
	}
	if help {
		return showHelp(ctx, stdout, command, fail)
	}
	if err := command.ParseFlags(normalized); err != nil || len(flags.Args()) != 0 {
		return argumentFailure(ctx, stderr, command, "invalid option syntax")
	}
	if err := command.ValidateRequiredFlags(); err != nil {
		return argumentFailure(ctx, stderr, command, err.Error())
	}
	if unknown || rootUnknown {
		return argumentFailure(ctx, stderr, command, "unrecognized option or positional operand")
	}
	if flags.Changed("session") && !budget.ValidSessionID(request.Session) {
		return argumentFailure(ctx, stderr, command, "invalid session identifier")
	}
	if operation != "report" && !budget.ValidSessionID(request.Task) {
		return argumentFailure(ctx, stderr, command, "invalid task identifier")
	}
	command.SetArgs([]string{})
	if err := command.ExecuteContext(ctx); err != nil {
		if !invoked {
			return fail("invalid command arguments", 2)
		}
		if status == 0 {
			status = 2
		}
		return fail(err.Error(), status)
	}
	if !invoked {
		return fail("invalid command arguments", 2)
	}
	return status
}

// Argument presentation contains only registered metadata and fixed categories;
// raw operands never enter diagnostics. docs/adr/0072-go-budget-argument-compatibility.md:46.
func argumentFailure(ctx context.Context, writer io.Writer, command *cobra.Command, message string) int {
	data := []byte(command.UsageString() + "\nfactory budget: error: " + message + "\n")
	_ = output.WriteEvent(ctx, writer, data)
	return 2
}
func showHelp(ctx context.Context, writer io.Writer, command *cobra.Command, fail func(string, int) int) int {
	if err := output.WriteEvent(ctx, writer, []byte(command.UsageString())); err != nil {
		return fail("cannot write command help", 2)
	}
	return 0
}
