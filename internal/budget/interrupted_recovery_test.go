package budget

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func interruptedBudgetHistory(status string, pid, exit any) History {
	ginkgo.GinkgoHelper()
	rows := []any{}
	if status != "" {
		rows = append(rows, map[string]any{"id": "run", "session": "session", "task": "task", "harness": "codex", "role": "implementer", "status": status, "outcome": "completed", "elapsed_seconds": 12.5, "reserved_seconds": 15, "attempt": 1, "owner_pid": 424242, "process_pid": pid, "exit_code": exit, "complete": true, "estimated_usd": 0.2, "tokens": nil, "warnings": []any{}, "model": "caller_model", "started_at": "before", "ended_at": "after", "source": "native", "operator_extension": map[string]any{"consumed": 9}})
		if status == "active" {
			row := rows[0].(map[string]any)
			row["outcome"] = "running"
			row["ended_at"] = nil
			row["complete"] = false
		}
	}
	data, err := json.Marshal(map[string]any{"schema": 1, "runs": rows, "operator_extension": "preserve"})
	Expect(err).NotTo(HaveOccurred())
	history, err := ParseHistory(context.Background(), bytes.NewReader(data))
	Expect(err).NotTo(HaveOccurred(), "fixture must pass the actual existing schema before querying recovery eligibility")
	return history
}

var _ = ginkgo.Describe("Interrupted recovery budget state eligibility", func() {
	// per docs/adr/0096-interrupted-publication-recovery.md:145
	// per docs/adr/0096-interrupted-publication-recovery.md:146
	// per docs/adr/0096-interrupted-publication-recovery.md:147
	// per docs/adr/0096-interrupted-publication-recovery.md:151
	ginkgo.DescribeTable("distinguishes unresolved ownership from recorded historical PID without changing history", func(status string, pid, exit any, want, active bool) {
		history := interruptedBudgetHistory(status, pid, exit)
		before, err := history.MarshalJSON()
		Expect(err).NotTo(HaveOccurred())
		unresolved, err := history.HasUnresolved(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(unresolved).To(Equal(want))
		remainsActive, err := history.HasActive(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(remainsActive).To(Equal(active), "the additive recovery query must not change existing HasActive semantics")
		after, err := history.MarshalJSON()
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before), "consumption and operator extensions remain unchanged")
	}, ginkgo.Entry("checked empty state", "", nil, nil, false, false), ginkgo.Entry("active reservation without process PID", "active", nil, nil, true, true), ginkgo.Entry("terminal historical PID with recorded zero exit", "completed", 424243, 0, false, false), ginkgo.Entry("terminal historical PID with recorded failure exit", "completed", 424243, 7, false, false), ginkgo.Entry("terminal PID with no exit proof", "completed", 424243, nil, true, false), ginkgo.Entry("terminal pre-child refusal without PID", "completed", nil, nil, false, false))

	// per docs/adr/0096-interrupted-publication-recovery.md:143
	// per docs/adr/0096-interrupted-publication-recovery.md:216
	ginkgo.It("rejects zero History and cancellation rather than granting a vacuous empty-state result", func() {
		_, err := (History{}).HasUnresolved(context.Background())
		Expect(err).To(HaveOccurred())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = interruptedBudgetHistory("", nil, nil).HasUnresolved(ctx)
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
	})
})
