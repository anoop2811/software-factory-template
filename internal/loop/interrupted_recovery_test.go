package loop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func interruptedLoopHistory(status string, uncertain bool, pid any) History {
	GinkgoHelper()
	rows := []any{}
	if status != "" {
		rows = append(rows, map[string]any{"session": "session", "task": "task", "harness": "codex", "mode": "manual", "status": status, "outcome": "manual_passed", "phase": "terminal", "owner_pid": 424242, "process_pid": pid, "uncertain": uncertain, "elapsed_seconds": 12.5, "reserved_seconds": 15, "attempts": 3, "no_progress": 0, "policy": "known", "prompt": "known", "started_at": "before", "stop_reason": "done", "next_action": "inspect", "evidence": []any{}, "budget_runs": []any{"consumed-budget"}, "snapshot": map[string]any{"head": "known", "source": "known", "safety": "known"}, "baseline": map[string]any{"head": "known", "source": "known", "safety": "known"}, "operator_extension": map[string]any{"consumed": 9}})
	}
	data, err := json.Marshal(map[string]any{"schema": 1, "runs": rows, "operator_extension": "preserve"})
	Expect(err).NotTo(HaveOccurred())
	history, err := ParseHistory(context.Background(), bytes.NewReader(data))
	Expect(err).NotTo(HaveOccurred(), "fixture passes existing checkpoint schema before querying recovery eligibility")
	return history
}

var _ = Describe("Interrupted recovery checkpoint state eligibility", func() {
	// per docs/adr/0096-interrupted-publication-recovery.md:148
	// per docs/adr/0096-interrupted-publication-recovery.md:151
	DescribeTable("preserves historical owners while refusing active, uncertain and retained process ownership", func(status string, uncertain bool, pid any, want bool) {
		history := interruptedLoopHistory(status, uncertain, pid)
		before, err := history.MarshalJSON()
		Expect(err).NotTo(HaveOccurred())
		unresolved, err := history.HasUnresolved(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(unresolved).To(Equal(want))
		after, err := history.MarshalJSON()
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before), "attempts, consumed budget links and unknown extensions cannot be rewritten")
	}, Entry("checked empty checkpoints", "", false, nil, false), Entry("completed historical owner", "completed", false, nil, false), Entry("stopped known harmless work", "stopped", false, nil, false), Entry("active checkpoint even without child PID", "active", false, nil, true), Entry("terminal but uncertain checkpoint", "completed", true, nil, true), Entry("stopped but uncertain checkpoint", "stopped", true, nil, true), Entry("terminal retained process PID without uncertain flag", "completed", false, 424243, true))

	// per docs/adr/0096-interrupted-publication-recovery.md:143
	// per docs/adr/0096-interrupted-publication-recovery.md:216
	It("rejects zero History and honors cancellation before inspecting any checkpoint", func() {
		_, err := (History{}).HasUnresolved(context.Background())
		Expect(err).To(HaveOccurred())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = interruptedLoopHistory("", false, nil).HasUnresolved(ctx)
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
	})
})
