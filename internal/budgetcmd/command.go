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
	"github.com/spf13/cobra"
)

// Run owns diagnostics and status; the bridge only forwards literal operands.
// docs/adr/0071-go-budget-command-candidate.md:22.
func Run(ctx context.Context, args []string, environment map[string]string, stdout, stderr io.Writer) int {
	fail := func(message string, code int) int {
		_, _ = fmt.Fprintln(stderr, "factory budget: "+message)
		return code
	}
	if len(args) == 0 || (args[0] != "plan" && args[0] != "run" && args[0] != "report") {
		return fail("invalid command arguments", 2)
	}
	// Boolean switches take no explicit value, unlike scalar flag=value forms.
	// docs/adr/0071-go-budget-command-candidate.md:120.
	for _, argument := range args[1:] {
		if strings.HasPrefix(argument, "--json=") {
			return fail("invalid command arguments", 2)
		}
	}
	operation := args[0]
	var request budget.Request
	var prompt, maxCost string
	var jsonOutput bool
	command := &cobra.Command{Use: operation, SilenceErrors: true, SilenceUsage: true, DisableSuggestions: true, Args: cobra.NoArgs}
	command.CompletionOptions.DisableDefaultCmd = true
	command.SetOut(stdout)
	command.SetErr(stderr)
	flags := command.Flags()
	flags.BoolVar(&jsonOutput, "json", false, "")
	flags.StringVar(&request.Session, "session", "", "")
	if operation != "report" {
		flags.StringVar(&request.Task, "task", "", "")
		flags.StringVar(&request.Harness, "harness", "", "")
		flags.StringVar(&request.Role, "role", "implementer", "")
		flags.StringVar(&maxCost, "max-cost-usd", "", "")
	}
	if operation == "run" {
		flags.StringVar(&prompt, "prompt-file", "", "")
	}
	status := 0
	invoked := false
	command.SetHelpFunc(func(*cobra.Command, []string) { status = 2 })
	command.RunE = func(cmd *cobra.Command, _ []string) error {
		invoked = true
		if err := cmd.Context().Err(); err != nil {
			return errors.New("budget command context ended")
		}
		if flags.Changed("help") {
			return errors.New("invalid command arguments")
		}
		if flags.Changed("session") && request.Session == "" {
			return errors.New("invalid command arguments")
		}
		if operation != "report" && (request.Session == "" || request.Task == "" || request.Harness == "") {
			return errors.New("invalid command arguments")
		}
		if operation == "run" && !flags.Changed("prompt-file") {
			return errors.New("invalid command arguments")
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
	// Parse a single leaf before Cobra can dispatch its hidden completion commands.
	// Scalar values remain literal; only leftover positional operands are refused.
	// docs/adr/0071-go-budget-command-candidate.md:28.
	if err := command.ParseFlags(args[1:]); err != nil || len(flags.Args()) != 0 {
		return fail("invalid command arguments", 2)
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
