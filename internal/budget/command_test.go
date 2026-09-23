package budget

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("Budget command plan observer", func() {
	// per docs/adr/0071-go-budget-command-candidate.md:65
	ginkgo.It("observes a disabled plan once without execution or mutation", func() {
		runner, cfg, request, input := runnerFixture()
		cfg.Enabled = false
		calls := 0
		before, err := os.ReadFile(filepath.Join(runner.ledger.root, ".factory/budget.json"))
		Expect(err).NotTo(HaveOccurred())
		input.OnPlan = func(plan Plan) error { calls++; Expect(plan.Blockers).NotTo(BeEmpty()); return nil }
		runner.ops.preflight = func(context.Context, native.Plan) error { ginkgo.Fail("disabled plan probed"); return nil }
		result, err := runner.Run(context.Background(), request, cfg, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ExitCode).To(Equal(2))
		Expect(calls).To(Equal(1))
		after, err := os.ReadFile(filepath.Join(runner.ledger.root, ".factory/budget.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
	})
	// per docs/adr/0071-go-budget-command-candidate.md:67
	ginkgo.It("observes a newly blocked locked plan once after preflight consumes the final slot", func() {
		runner, cfg, request, input := runnerFixture()
		cfg.MaxSessionRuns.SetInt64(1)
		calls := 0
		runner.ops.preflight = func(ctx context.Context, _ native.Plan) error {
			admission, err := runner.ledger.Admit(ctx, request, cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(admission.Record).NotTo(BeNil())
			return nil
		}
		input.OnPlan = func(plan Plan) error {
			calls++
			Expect(plan.Blockers).NotTo(BeEmpty())
			Expect(plan.ActiveRuns).To(Equal(1))
			return nil
		}
		runner.ops.execute = func(context.Context, native.Plan, time.Duration, func(context.Context, int) error) (native.Execution, error) {
			ginkgo.Fail("blocked plan executed")
			return native.Execution{}, nil
		}
		result, err := runner.Run(context.Background(), request, cfg, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ExitCode).To(Equal(2))
		Expect(calls).To(Equal(1))
		history, err := runner.ledger.Read(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(history.rows()).To(HaveLen(1))
	})
	// per docs/adr/0071-go-budget-command-candidate.md:69
	ginkgo.It("refuses observer failure before reservation and never retries the observer", func() {
		runner, cfg, request, input := runnerFixture()
		calls := 0
		input.OnPlan = func(Plan) error {
			calls++
			history, err := runner.ledger.Read(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(history.rows()).To(BeEmpty())
			return errors.New("PRIVATE_WRITER_FAILURE")
		}
		runner.ops.execute = func(context.Context, native.Plan, time.Duration, func(context.Context, int) error) (native.Execution, error) {
			ginkgo.Fail("failed observer authorized execution")
			return native.Execution{}, nil
		}
		result, err := runner.Run(context.Background(), request, cfg, input)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).NotTo(ContainSubstring("PRIVATE"))
		Expect(calls).To(Equal(1))
		Expect(result.Record).To(BeNil())
		history, err := runner.ledger.Read(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(history.rows()).To(BeEmpty())
	})
	// per docs/adr/0071-go-budget-command-candidate.md:68
	ginkgo.It("emits nothing when preflight fails", func() {
		runner, cfg, request, input := runnerFixture()
		calls := 0
		input.OnPlan = func(Plan) error { calls++; return nil }
		runner.ops.preflight = func(context.Context, native.Plan) error { return errors.New("PRIVATE_PROBE") }
		_, err := runner.Run(context.Background(), request, cfg, input)
		Expect(err).To(HaveOccurred())
		Expect(calls).To(BeZero())
	})
	// per docs/adr/0071-go-budget-command-candidate.md:67
	ginkgo.It("observes the successful plan before reservation and execution", func() {
		runner, cfg, request, input := runnerFixture()
		calls := 0
		input.OnPlan = func(plan Plan) error {
			calls++
			Expect(plan.Blockers).To(BeEmpty())
			file, err := os.OpenFile(filepath.Join(runner.ledger.root, ".factory/budget.lock"), os.O_RDWR, 0)
			Expect(err).NotTo(HaveOccurred())
			defer file.Close()
			err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
			Expect(errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)).To(BeTrue(), "observer must run while the ledger lock is held")
			history, err := runner.ledger.Read(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(history.rows()).To(BeEmpty())
			return nil
		}
		runner.ops.execute = func(ctx context.Context, p native.Plan, d time.Duration, spawn func(context.Context, int) error) (native.Execution, error) {
			Expect(calls).To(Equal(1))
			return runnerSuccess(ctx, p, d, spawn)
		}
		result, err := runner.Run(context.Background(), request, cfg, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(calls).To(Equal(1))
		Expect(result.ExitCode).To(BeZero())
	})
})

type commandEventWriter struct {
	bytes.Buffer
	mode            string
	writes, flushes int
}

func (w *commandEventWriter) Write(data []byte) (int, error) {
	w.writes++
	if w.mode == "short" {
		return len(data) - 1, nil
	}
	if w.mode == "write" {
		return 0, errors.New("PRIVATE_WRITE")
	}
	return w.Buffer.Write(data)
}
func (w *commandEventWriter) Flush() error {
	w.flushes++
	if w.mode == "flush" {
		return errors.New("PRIVATE_FLUSH")
	}
	return nil
}

var _ = ginkgo.Describe("Budget command renderer writer boundary", func() {
	// per docs/adr/0071-go-budget-command-candidate.md:73
	ginkgo.DescribeTable("checks complete writes and event flushes", func(kind, mode string) {
		ledger, cfg, request := storageFixture()
		admission, err := ledger.Admit(context.Background(), request, cfg)
		Expect(err).NotTo(HaveOccurred())
		history, err := ledger.Read(context.Background())
		Expect(err).NotTo(HaveOccurred())
		report, err := Report(context.Background(), history, "")
		Expect(err).NotTo(HaveOccurred())
		writer := &commandEventWriter{mode: mode}
		var render func(context.Context, io.Writer) error
		switch kind {
		case "plan":
			render = func(ctx context.Context, w io.Writer) error { return RenderPlan(ctx, w, admission.Plan, false) }
		case "record":
			render = func(ctx context.Context, w io.Writer) error { return RenderRecord(ctx, w, *admission.Record, true) }
		case "report":
			render = func(ctx context.Context, w io.Writer) error { return RenderReport(ctx, w, report, false) }
		}
		err = render(context.Background(), writer)
		if mode == "ok" {
			Expect(err).NotTo(HaveOccurred())
			Expect(writer.flushes).To(Equal(1))
			Expect(writer.String()).To(HaveSuffix("\n"))
		} else {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring("PRIVATE"))
			if mode == "flush" {
				Expect(writer.flushes).To(Equal(1))
			} else {
				Expect(writer.flushes).To(BeZero())
			}
		}
		Expect(writer.writes).To(Equal(1))
	}, ginkgo.Entry("plan success", "plan", "ok"), ginkgo.Entry("record success", "record", "ok"), ginkgo.Entry("report success", "report", "ok"), ginkgo.Entry("plan short", "plan", "short"), ginkgo.Entry("record short", "record", "short"), ginkgo.Entry("report short", "report", "short"), ginkgo.Entry("plan write failure", "plan", "write"), ginkgo.Entry("record write failure", "record", "write"), ginkgo.Entry("report write failure", "report", "write"), ginkgo.Entry("plan flush failure", "plan", "flush"), ginkgo.Entry("record flush failure", "record", "flush"), ginkgo.Entry("report flush failure", "report", "flush"))
})
