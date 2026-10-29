package budget

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("Budget admission policy inherited from script controllers", func() {
	// per docs/BUDGETS.md:65
	ginkgo.DescribeTable("refuses a strict USD ceiling before native preparation or accounting changes", func(harness string) {
		runner, cfg, request, input := runnerFixture()
		request.Harness = harness
		ceiling := "1"
		request.MaxCostUSD = &ceiling
		cfg.Action = "warn"
		input.PromptFile, input.Prompt = "missing-private-prompt", nil
		input.Environment = map[string]string{"PATH": "/no-native-clients"}
		runner.ops.preflight = func(context.Context, native.Plan) error {
			ginkgo.Fail("strict USD reached native preflight")
			return nil
		}
		runner.ops.execute = func(context.Context, native.Plan, time.Duration, func(context.Context, int) error) (native.Execution, error) {
			ginkgo.Fail("strict USD launched a native client")
			return native.Execution{}, nil
		}
		path := filepath.Join(runner.ledger.root, ".factory/budget.json")
		before, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		result, err := runner.Run(context.Background(), request, cfg, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ExitCode).To(Equal(2))
		Expect(result.Plan.Blockers).To(ConsistOf(ContainSubstring("strict USD ceilings are unsupported")))
		Expect(result.Record).To(BeNil())
		Expect(result.Response).To(BeEmpty())
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		_, err = os.Stat(filepath.Join(runner.ledger.root, ".factory/budget.lock"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, ginkgo.Entry("Codex", "codex"), ginkgo.Entry("Claude", "claude"), ginkgo.Entry("OpenCode", "opencode"))

	// per docs/BUDGETS.md:87
	// per docs/BUDGETS.md:95
	// per docs/adr/0069-go-budget-ledger-admission.md:51
	ginkgo.DescribeTable("isolates cost concerns from hard limits during read-only and locked admission", func(scenario, action, concern string, blocked bool) {
		ledger, cfg, request := storageFixture()
		cfg.Action = action
		threshold := 1.0
		cfg.EstimatedUSD = &threshold
		row := map[string]any{"id": "prior", "session": request.Session, "task": "prior", "harness": "claude", "role": "reviewer", "status": "completed", "outcome": "completed", "elapsed_seconds": 0, "reserved_seconds": 0, "attempt": 1, "owner_pid": os.Getpid(), "process_pid": nil, "exit_code": 0, "complete": true, "tokens": nil, "estimated_usd": 1, "warnings": []any{}, "model": "", "started_at": "before", "ended_at": "after", "source": "client estimate"}
		switch scenario {
		case "below threshold":
			row["estimated_usd"] = 0.5
		case "other session cost":
			row["session"] = "other"
		case "previous unknown":
			row["estimated_usd"], row["complete"] = nil, false
		case "active unknown", "reserved time", "checkout concurrency":
			row["status"], row["outcome"] = "active", "running"
			row["estimated_usd"], row["ended_at"], row["exit_code"], row["complete"] = nil, nil, nil, false
			cfg.MaxConcurrent.SetInt64(2)
			if scenario == "reserved time" {
				cfg.EstimatedUSD, cfg.SessionSeconds = nil, 5
				row["reserved_seconds"] = 5
			}
			if scenario == "checkout concurrency" {
				row["session"], cfg.EstimatedUSD = "other", nil
				cfg.MaxConcurrent.SetInt64(1)
			}
		case "task attempts":
			cfg.EstimatedUSD, row["task"] = nil, request.Task
		case "session runs":
			cfg.EstimatedUSD = nil
			cfg.MaxSessionRuns.SetInt64(1)
		case "Codex cost capability":
			request.Harness, row["estimated_usd"] = "codex", 0
		}
		path := filepath.Join(ledger.root, ".factory/budget.json")
		before, err := json.Marshal(map[string]any{"schema": 1, "runs": []any{row}})
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(path, before, 0600)).To(Succeed())
		history, err := ledger.Read(context.Background())
		Expect(err).NotTo(HaveOccurred())
		plan, err := MakePlan(context.Background(), request, cfg, history)
		Expect(err).NotTo(HaveOccurred())
		if blocked {
			Expect(plan.Blockers).To(ConsistOf(ContainSubstring(concern)))
			Expect(plan.Warnings).To(BeEmpty())
		} else {
			Expect(plan.Blockers).To(BeEmpty())
			if concern == "" {
				Expect(plan.Warnings).To(BeEmpty())
			} else {
				Expect(plan.Warnings).To(ConsistOf(ContainSubstring(concern)))
			}
		}
		if scenario == "reserved time" {
			Expect(plan.RemainingSessionSeconds).To(BeZero(), "the live reservation, not zero elapsed time, consumes the allowance")
		}
		admission, err := ledger.Admit(context.Background(), request, cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(admission.Plan.Blockers).To(Equal(plan.Blockers))
		Expect(admission.Plan.Warnings).To(Equal(plan.Warnings))
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		if blocked {
			Expect(admission.Record).To(BeNil())
			Expect(after).To(Equal(before), "a refused admission must not reset the prior charge")
		} else {
			Expect(admission.Record).NotTo(BeNil())
			history, err = ledger.Read(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(history.rows()).To(HaveLen(2))
		}
	}, ginkgo.Entry("known cost below threshold", "below threshold", "stop", "", false),
		ginkgo.Entry("other session cost is not charged", "other session cost", "stop", "", false),
		ginkgo.Entry("known cost stops at threshold", "known threshold", "stop", "threshold reached", true),
		ginkgo.Entry("known cost warns at threshold", "known threshold", "warn", "threshold reached", false),
		ginkgo.Entry("previous unknown cost stops", "previous unknown", "stop", "previous session cost is unknown", true),
		ginkgo.Entry("previous unknown cost warns", "previous unknown", "warn", "previous session cost is unknown", false),
		ginkgo.Entry("active same-session cost stops", "active unknown", "stop", "has not reported its cost", true),
		ginkgo.Entry("active same-session cost warns", "active unknown", "warn", "has not reported its cost", false),
		ginkgo.Entry("Codex unknown estimate stops", "Codex cost capability", "stop", "Codex does not report", true),
		ginkgo.Entry("Codex unknown estimate warns", "Codex cost capability", "warn", "Codex does not report", false),
		ginkgo.Entry("reserved time blocks despite warn", "reserved time", "warn", "session time limit", true),
		ginkgo.Entry("other session concurrency blocks despite warn", "checkout concurrency", "warn", "checkout concurrency limit", true),
		ginkgo.Entry("task attempts block despite warn", "task attempts", "warn", "task attempt limit", true),
		ginkgo.Entry("session runs block despite warn", "session runs", "warn", "session run limit", true))
})
