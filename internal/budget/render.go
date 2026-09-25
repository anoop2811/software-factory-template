package budget

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
	"github.com/anoop2811/software-factory-template/internal/output"
)

// RenderPlan publishes one complete presentation event before execution.
// docs/adr/0071-go-budget-command-candidate.md:50.
func RenderPlan(ctx context.Context, writer io.Writer, plan Plan, jsonOutput bool) error {
	if jsonOutput {
		return renderJSON(ctx, writer, plan)
	}
	var text strings.Builder
	model := plan.Model
	if model == "" {
		model = "harness default"
	}
	fmt.Fprintf(&text, "Budget plan: %s, role %s, session %s, task %s\n", plan.Harness, plan.Role, plan.Session, plan.Task)
	fmt.Fprintf(&text, "Model: %s (%s)\n", model, plan.ModelSource)
	fmt.Fprintf(&text, "Remaining: %s task attempts; %s session runs; %.3fs session time; %d active runs\n", plan.RemainingAttempts, plan.RemainingSessionRuns, plan.RemainingSessionSeconds, plan.ActiveRuns)
	fmt.Fprintf(&text, "Per-run timeout: %ss; checkout concurrency: %s\n", jsonvalue.FloatText(plan.Configuration.TimeoutSeconds), plan.Configuration.MaxConcurrent)
	fmt.Fprintf(&text, "Cost reporting: %s\n", plan.CostReporting)
	text.WriteString("Services: model provider inherited; tool services unknown (harness/project configuration)\n")
	for _, blocker := range plan.Blockers {
		fmt.Fprintf(&text, "BLOCKED: %s\n", blocker)
	}
	for _, warning := range plan.Warnings {
		fmt.Fprintf(&text, "WARNING: %s\n", warning)
	}
	return writeEvent(ctx, writer, []byte(text.String()))
}

// RenderReport preserves the baseline labels and exact integer cost displays.
// docs/adr/0071-go-budget-command-candidate.md:58.
func RenderReport(ctx context.Context, writer io.Writer, report ReportData, jsonOutput bool) error {
	if jsonOutput {
		return renderJSON(ctx, writer, report)
	}
	elapsed, ok := numericFloat(report.Totals["elapsed_seconds"])
	cost, costOK := numericFloat(report.Totals["known_estimated_usd"])
	if !ok || !costOK {
		return errors.New("invalid budget report presentation")
	}
	var text strings.Builder
	fmt.Fprintf(&text, "Budget report: %v runs, %v active, %.3fs charged\n", report.Totals["runs"], report.Totals["active"], elapsed)
	partial := ""
	if report.Totals["cost_complete"] != true {
		partial = " (partial total)"
	}
	fmt.Fprintf(&text, "Known client-estimated USD: %.6f; unknown-cost runs: %v%s\n", cost, report.Totals["unknown_cost_runs"], partial)
	for _, record := range report.Runs {
		if err := ctx.Err(); err != nil {
			return err
		}
		display, err := costText(record.data["estimated_usd"])
		if err != nil {
			return err
		}
		fmt.Fprintf(&text, "%v %v/%v %v %v estimated USD %s\n", record.data["id"], record.data["session"], record.data["task"], record.data["harness"], record.data["outcome"], display)
	}
	return writeEvent(ctx, writer, []byte(text.String()))
}

// RenderRecord publishes metadata only; response text is a separate final event.
// docs/adr/0071-go-budget-command-candidate.md:52.
func RenderRecord(ctx context.Context, writer io.Writer, record Record, jsonOutput bool) error {
	if jsonOutput {
		return renderJSON(ctx, writer, record)
	}
	elapsed, ok := numericFloat(record.data["elapsed_seconds"])
	if !ok {
		return errors.New("invalid budget record presentation")
	}
	cost, err := costText(record.data["estimated_usd"])
	if err != nil {
		return err
	}
	var text strings.Builder
	fmt.Fprintf(&text, "Run %v: %v; elapsed %.3fs; estimated USD %s\n", record.data["id"], record.data["outcome"], elapsed, cost)
	warnings, ok := record.data["warnings"].([]any)
	if !ok {
		return errors.New("invalid budget record presentation")
	}
	for _, warning := range warnings {
		fmt.Fprintf(&text, "WARNING: %v\n", warning)
	}
	return writeEvent(ctx, writer, []byte(text.String()))
}

func costText(value any) (string, error) {
	if value == nil {
		return "unknown", nil
	}
	if integer, ok := exactInteger(value); ok {
		return integer.String(), nil
	}
	number, ok := numericFloat(value)
	if !ok {
		return "", errors.New("invalid budget cost presentation")
	}
	return jsonvalue.FloatText(number), nil
}
func renderJSON(ctx context.Context, writer io.Writer, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return errors.New("cannot encode budget metadata")
	}
	return writeEvent(ctx, writer, buffer.Bytes())
}

// Flush every logical event and refuse short writes before admission can proceed.
// docs/adr/0071-go-budget-command-candidate.md:73.
func writeEvent(ctx context.Context, writer io.Writer, data []byte) error {
	err := output.WriteEvent(ctx, writer, data)
	if errors.Is(err, output.ErrWrite) {
		return errors.New("cannot write budget output")
	}
	if errors.Is(err, output.ErrFlush) {
		return errors.New("cannot flush budget output")
	}
	return err
}

// RenderAnswer shares the same checked event writer as metadata. The caller
// selects it only after successful finalization. docs/adr/0071-go-budget-command-candidate.md:62.
func RenderAnswer(ctx context.Context, writer io.Writer, answer string) error {
	return writeEvent(ctx, writer, []byte(answer+"\n"))
}
