package budget

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/usage"
)

// Runner composes execution after durable admission.
// docs/adr/0070-go-budget-execution-controller.md:17.
type Runner struct {
	ledger *Ledger
	ops    runnerOps
}

type RunInput struct {
	PromptFile   string
	Prompt       *string
	Environment  map[string]string
	WantResponse bool
}
type RunResult struct {
	Plan     Plan
	Record   *Record
	Response string `json:"-"`
	ExitCode int
}
type runnerOps struct {
	preflight func(context.Context, native.Plan) error
	execute   func(context.Context, native.Plan, time.Duration, func(context.Context, int) error) (native.Execution, error)
	parse     func(context.Context, string, io.Reader) (usage.Metadata, error)
	response  func(context.Context, string, io.Reader) (string, error)
}

func NewRunner(root string) *Runner { return &Runner{ledger: NewLedger(root)} }

// Run preserves read-only admission before probes and locked admission before
// execution: docs/adr/0070-go-budget-execution-controller.md:41.
func (runner *Runner) Run(ctx context.Context, request Request, config Config, input RunInput) (RunResult, error) {
	result := RunResult{ExitCode: 2}
	config, err := validateConfig(config)
	if err != nil {
		return result, controllerError("invalid budget execution configuration", err)
	}
	if err := validateRequest(request); err != nil {
		return result, controllerError("invalid budget execution request", err)
	}
	history, err := runner.ledger.Read(ctx)
	if err != nil {
		return result, controllerError("cannot read budget execution history", err)
	}
	result.Plan, err = MakePlan(ctx, request, config, history)
	if err != nil {
		return result, controllerError("cannot plan budget execution", err)
	}
	if len(result.Plan.Blockers) > 0 {
		return result, nil
	}
	if _, err := executionDuration(config.TimeoutSeconds); err != nil {
		return result, err
	}
	if _, err := executionDuration(math.Min(config.TimeoutSeconds, result.Plan.RemainingSessionSeconds)); err != nil {
		return result, err
	}
	prompt, err := loadPrompt(ctx, input)
	if err != nil {
		return result, err
	}
	prepared, err := native.Prepare(native.Request{Harness: request.Harness, Role: request.Role, Model: request.Model, Root: runner.ledger.root, Prompt: prompt}, input.Environment)
	if err != nil {
		return result, controllerError("cannot prepare native budget execution", err)
	}
	if err := runner.preflight(ctx, prepared); err != nil {
		return result, controllerError("native budget preflight failed", err)
	}
	config, err = executionConfig(ctx, config)
	if err != nil {
		return result, controllerError("budget execution deadline expired", err)
	}
	admission, err := runner.ledger.admit(ctx, request, config, executionAdmission)
	if err != nil {
		return result, controllerError("budget execution admission failed", err)
	}
	result.Plan = admission.Plan
	if admission.Record == nil {
		return result, nil
	}
	return runner.executeAdmitted(ctx, request, prepared, *admission.Record, result, input.WantResponse)
}
func (runner *Runner) preflight(ctx context.Context, plan native.Plan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if runner.ops.preflight != nil {
		return runner.ops.preflight(ctx, plan)
	}
	return native.Preflight(ctx, plan)
}
func (runner *Runner) execute(ctx context.Context, plan native.Plan, allowance time.Duration, callback func(context.Context, int) error) (native.Execution, error) {
	if runner.ops.execute != nil {
		return runner.ops.execute(ctx, plan, allowance, callback)
	}
	return native.Execute(ctx, plan, allowance, callback)
}
func (runner *Runner) parse(ctx context.Context, harness string, input io.Reader) (usage.Metadata, error) {
	if runner.ops.parse != nil {
		return runner.ops.parse(ctx, harness, input)
	}
	return usage.Parse(ctx, harness, input)
}
func (runner *Runner) response(ctx context.Context, harness string, input io.Reader) (string, error) {
	if runner.ops.response != nil {
		return runner.ops.response(ctx, harness, input)
	}
	return usage.Response(ctx, harness, input)
}

// Finalization has one detached, bounded cleanup context, including parsing and
// lock waits: docs/adr/0070-go-budget-execution-controller.md:76.
func (runner *Runner) executeAdmitted(ctx context.Context, request Request, prepared native.Plan, record Record, result RunResult, wantResponse bool) (RunResult, error) {
	result.ExitCode = 1
	id := record.data["id"].(string)
	seconds, _ := numericFloat(record.data["reserved_seconds"])
	allowance, durationErr := executionDuration(seconds)
	var localPID *int
	var publicationErr error
	var execution native.Execution
	executionErr := durationErr
	if executionErr == nil {
		if err := ctx.Err(); err != nil {
			executionErr = err
		} else {
			execution, executionErr = runner.execute(ctx, prepared, allowance, func(callbackCtx context.Context, pid int) error {
				localPID = copyPID(&pid)
				_, publicationErr = runner.ledger.PublishPID(callbackCtx, id, pid)
				return publicationErr
			})
		}
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	completion := Completion{Outcome: execution.Outcome, ExitCode: execution.ExitCode, ElapsedSeconds: execution.ElapsedSeconds, ExitConfirmed: execution.ExitConfirmed, OwnershipUnconfirmed: execution.OwnershipUnconfirmed, ProcessPID: localPID, Source: "not yet reported"}
	if completion.ProcessPID == nil && execution.ProcessPID > 0 {
		completion.ProcessPID = copyPID(&execution.ProcessPID)
	}
	// Preserve native timeout evidence when unresolved ownership is the error.
	// docs/adr/0070-go-budget-execution-controller.md:138.
	retainedTimeout := execution.Outcome == "timeout" && execution.OwnershipUnconfirmed && completion.ProcessPID != nil
	if completion.ProcessPID == nil || ((executionErr != nil || publicationErr != nil) && !retainedTimeout) {
		completion.Outcome = "launch_error"
	}
	var processingErr error
	var answer string
	if executionErr == nil && publicationErr == nil && !completion.OwnershipUnconfirmed && completion.ProcessPID != nil && (completion.Outcome == "completed" || completion.Outcome == "failed") {
		metadata, err := runner.parse(cleanupCtx, request.Harness, bytes.NewReader(execution.Stdout))
		processingErr = err
		if err == nil {
			completion.Tokens = metadata.Tokens
			completion.EstimatedUSD = metadata.EstimatedUSD
			completion.Complete = metadata.Complete
			completion.Failed = metadata.Failed
			completion.Source = metadata.Source
			if wantResponse && execution.Outcome == "completed" && metadata.Complete && !metadata.Failed && execution.ExitConfirmed && execution.ExitCode != nil && *execution.ExitCode == 0 {
				answer, processingErr = runner.response(cleanupCtx, request.Harness, bytes.NewReader(execution.Stdout))
			}
		}
		if processingErr != nil {
			completion.Outcome = "launch_error"
			answer = ""
		}
	}
	execution.Stdout = nil
	final, finalErr := runner.ledger.Finalize(cleanupCtx, id, completion, result.Plan.Configuration)
	if finalErr != nil {
		return result, controllerError("budget execution finalization failed", publicationErr, executionErr, processingErr, finalErr)
	}
	result.Record = &final
	if publicationErr != nil || executionErr != nil || processingErr != nil {
		return result, controllerError("budget execution failed", publicationErr, executionErr, processingErr)
	}
	switch final.data["outcome"] {
	case "completed":
		result.ExitCode = 0
	case "timeout":
		result.ExitCode = 124
	case "interrupted":
		result.ExitCode = 130
	}
	if result.ExitCode == 0 && final.data["status"] == "completed" && final.data["complete"] == true {
		result.Response = answer
	}
	return result, nil
}

type runError struct {
	message string
	cause   error
}

func (e *runError) Error() string { return e.message }
func (e *runError) Unwrap() error { return e.cause }

// Retain typed ownership/publication evidence without exposing arbitrary injected
// or native diagnostics: docs/adr/0070-go-budget-execution-controller.md:104.
func controllerError(message string, causes ...error) error {
	var safe []error
	for _, cause := range causes {
		if cause == nil {
			continue
		}
		var publication *PublicationError
		if errors.As(cause, &publication) {
			safe = append(safe, publication)
		}
		var ownership *native.OwnershipError
		if errors.As(cause, &ownership) {
			safe = append(safe, ownership)
		}
		if errors.Is(cause, context.Canceled) {
			safe = append(safe, context.Canceled)
		}
		if errors.Is(cause, context.DeadlineExceeded) {
			safe = append(safe, context.DeadlineExceeded)
		}
	}
	return &runError{message: message, cause: errors.Join(safe...)}
}
