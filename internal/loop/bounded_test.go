package loop

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/anoop2811/software-factory-template/internal/budget"
	"github.com/anoop2811/software-factory-template/internal/native"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Bounded loop strict review", func() {
	// per docs/adr/0076-go-bounded-loop-controller.md:97
	DescribeTable("admits only exact independent review verdicts", func(raw string, valid bool, verdict string, count int) {
		result, err := parseVerdict(context.Background(), raw)
		if !valid {
			Expect(err).To(HaveOccurred())
			return
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Verdict).To(Equal(verdict))
		Expect(result.Findings).To(HaveLen(count))
	}, Entry("approve", `{"verdict":"approve","findings":[]}`, true, "approve", 0),
		Entry("repair", `{"verdict":"repair","findings":["fix"]}`, true, "repair", 1),
		Entry("surrogate counts one", `{"verdict":"repair","findings":["\ud800"]}`, true, "repair", 1),
		Entry("zero width nonblank", `{"verdict":"repair","findings":["\u200b"]}`, true, "repair", 1),
		Entry("4000 codepoints", `{"verdict":"repair","findings":["`+strings.Repeat("😀", 4000)+`"]}`, true, "repair", 1),
		Entry("4001 codepoints", `{"verdict":"repair","findings":["`+strings.Repeat("😀", 4001)+`"]}`, false, "", 0),
		Entry("50 findings", `{"verdict":"repair","findings":[`+strings.Repeat(`"x",`, 49)+`"x"]}`, true, "repair", 50),
		Entry("51 findings", `{"verdict":"repair","findings":[`+strings.Repeat(`"x",`, 50)+`"x"]}`, false, "", 0),
		Entry("null findings", `{"verdict":"approve","findings":null}`, false, "", 0),
		Entry("wrong type", `{"verdict":"repair","findings":[1]}`, false, "", 0),
		Entry("extra field", `{"verdict":"approve","findings":[],"extra":null}`, false, "", 0),
		Entry("escaped duplicate key", `{"verdict":"repair","\u0076erdict":"approve","findings":[]}`, false, "", 0),
		Entry("duplicate findings", `{"verdict":"approve","findings":[],"findings":[]}`, false, "", 0),
		Entry("trailing object", `{"verdict":"approve","findings":[]}{}`, false, "", 0),
		Entry("fence", "```json\n{}\n```", false, "", 0),
		Entry("array", `[]`, false, "", 0),
		Entry("case sensitive", `{"verdict":"Approve","findings":[]}`, false, "", 0),
		Entry("approve nonempty", `{"verdict":"approve","findings":["x"]}`, false, "", 0),
		Entry("repair empty", `{"verdict":"repair","findings":[]}`, false, "", 0),
		Entry("Python control whitespace", `{"verdict":"repair","findings":["\u001c\u0085\u2028\u00a0"]}`, false, "", 0))
	// per docs/adr/0076-go-bounded-loop-controller.md:71
	It("refuses a canceled parser context", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := parseVerdict(ctx, `{"verdict":"approve","findings":[]}`)
		Expect(err).To(HaveOccurred())
	})
})

func boundedControllerFixture() (*Controller, Request) {
	GinkgoHelper()
	root, _, environment := snapshotFixture()
	environment["FACTORY_LOOP_ENABLED"] = "true"
	environment["FACTORY_BUDGET_ENABLED"] = "true"
	environment["FACTORY_LOOP_CHECK_COMMAND"] = "true"
	environment["FACTORY_BUDGET_MAX_ATTEMPTS"] = "10"
	prompt := filepath.Join(GinkgoT().TempDir(), "prompt")
	Expect(os.WriteFile(prompt, []byte("PRIVATE_TASK"), 0600)).To(Succeed())
	return NewController(root), Request{Mode: "bounded", PromptFile: prompt, Harness: "claude", Session: "s", Task: "t", Environment: environment}
}
func boundedPaid(ctx context.Context, controller *Controller, request budget.Request, cfg budget.Config) budget.RunResult {
	GinkgoHelper()
	ledger := budget.NewLedger(controller.root)
	admission, err := ledger.Admit(ctx, request, cfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(admission.Record).NotTo(BeNil())
	view, err := admission.Record.Observe(ctx)
	Expect(err).NotTo(HaveOccurred())
	_, err = ledger.PublishPID(ctx, view.ID, 424242)
	Expect(err).NotTo(HaveOccurred())
	code := 0
	record, err := ledger.Finalize(ctx, view.ID, budget.Completion{Outcome: "completed", ExitCode: &code, ExitConfirmed: true, ElapsedSeconds: 0.01, Complete: false, Source: "test-owned metadata"}, cfg)
	Expect(err).NotTo(HaveOccurred())
	return budget.RunResult{Record: &record, ExitCode: 0, Response: `{"verdict":"approve","findings":[]}`}
}

var _ = Describe("Bounded loop lifecycle boundaries", func() {
	// per docs/adr/0076-go-bounded-loop-controller.md:71
	// per docs/DECISION_LOG.md:5317
	It("does not invoke budget execution after phase persistence exhausts the total deadline", func() {
		controller, request := boundedControllerFixture()
		request.Environment["FACTORY_LOOP_TIMEOUT_SECONDS"] = "5"
		actualSnapshot := controller.ops.snapshot
		var baseline Fingerprint
		var boundedContext context.Context
		// Qualify the real baseline before the loop clock starts. This criterion
		// targets publication, so its freshness observer then reuses that stable
		// image and captures the actual execution context without Git scheduling.
		controller.ops.snapshot = func(ctx context.Context, root string, cfg Config, environment map[string]string) (Fingerprint, error) {
			if _, bounded := ctx.Deadline(); bounded {
				boundedContext = ctx
				return baseline, nil
			}
			var err error
			baseline, err = actualSnapshot(ctx, root, cfg, environment)
			return baseline, err
		}
		phases, calls := []string{}, 0
		var phaseDeadline time.Time
		controller.store.ops.syncFile = func(file *os.File) error {
			data, err := os.ReadFile(file.Name())
			Expect(err).NotTo(HaveOccurred())
			var history map[string]any
			Expect(json.Unmarshal(data, &history)).To(Succeed())
			phase := history["runs"].([]any)[0].(map[string]any)["phase"].(string)
			phases = append(phases, phase)
			if phase == "implementer" {
				Expect(boundedContext).NotTo(BeNil(), "the actual execution context must reach the intended publication")
				Expect(boundedContext.Err()).NotTo(HaveOccurred(), "setup must not exhaust the deadline before the sync barrier")
				var bounded bool
				phaseDeadline, bounded = boundedContext.Deadline()
				Expect(bounded).To(BeTrue())
				<-boundedContext.Done()
				Expect(boundedContext.Err()).To(MatchError(context.DeadlineExceeded))
			}
			return file.Sync()
		}
		controller.ops.invoke = func(context.Context, budget.Request, budget.Config, budget.RunInput) (budget.RunResult, error) {
			calls++
			return budget.RunResult{}, errors.New("unexpected paid call")
		}
		result, err := controller.Run(context.Background(), request, false)
		history, readErr := controller.store.Read(context.Background())
		Expect(readErr).NotTo(HaveOccurred())
		rows := history.data["runs"].([]any)
		Expect(rows).To(HaveLen(1))
		durable := rows[0].(map[string]any)
		var publication *PublicationError
		isPublication := errors.As(err, &publication)
		committed := publication != nil && publication.MayHaveCommitted
		AddReportEntry("phase-publication-deadline", map[string]any{"phases": phases, "paid_calls": calls, "phase_deadline": phaseDeadline, "publication_error": isPublication, "may_have_committed": committed, "durable_phase": durable["phase"]}, ReportEntryVisibilityNever)
		Expect(calls).To(BeZero())
		Expect(err).To(HaveOccurred())
		Expect(isPublication).To(BeTrue())
		Expect(committed).To(BeFalse(), "expired phase publication must stop before replacing the durable checkpoint")
		Expect(result.Record).To(BeNil())
		Expect(phases).To(Equal([]string{"starting", "implementer"}))
		Expect(durable["phase"]).To(Equal("starting"))
		Expect(durable["status"]).To(Equal("active"))
		Expect(durable["attempts"]).To(Equal(json.Number("0")))
		Expect(durable["budget_runs"]).To(BeEmpty())
		_, budgetErr := os.Lstat(filepath.Join(controller.root, ".factory/budget.json"))
		Expect(os.IsNotExist(budgetErr)).To(BeTrue(), "phase publication failure must not create paid accounting")
	})
	// per docs/adr/0076-go-bounded-loop-controller.md:108
	It("retains paid accounting when the following loop save fails and never relaunches", func() {
		controller, request := boundedControllerFixture()
		calls, failedSaves := 0, 0
		controller.ops.invoke = func(ctx context.Context, r budget.Request, cfg budget.Config, _ budget.RunInput) (budget.RunResult, error) {
			calls++
			return boundedPaid(ctx, controller, r, cfg), nil
		}
		controller.store.ops.syncFile = func(file *os.File) error {
			data, err := os.ReadFile(file.Name())
			if err != nil {
				return err
			}
			var history map[string]any
			if err = json.Unmarshal(data, &history); err != nil {
				return err
			}
			row := history["runs"].([]any)[0].(map[string]any)
			if len(row["budget_runs"].([]any)) > 0 {
				failedSaves++
				return errors.New("PRIVATE_LOOP_SAVE_FAILURE")
			}
			return file.Sync()
		}
		result, err := controller.Run(context.Background(), request, false)
		Expect(err).To(HaveOccurred())
		Expect(result.Record).To(BeNil())
		Expect(calls).To(Equal(1))
		Expect(failedSaves).To(Equal(1))
		data, err := os.ReadFile(filepath.Join(controller.root, ".factory/budget.json"))
		Expect(err).NotTo(HaveOccurred())
		var history map[string]any
		Expect(json.Unmarshal(data, &history)).To(Succeed())
		rows := history["runs"].([]any)
		Expect(rows).To(HaveLen(1))
		Expect(rows[0].(map[string]any)["status"]).To(Equal("completed"))
	})
	// per docs/adr/0076-go-bounded-loop-controller.md:114
	DescribeTable("reconciles invocation failures without inventing or losing ownership", func(kind string, uncertain bool) {
		controller, request := boundedControllerFixture()
		calls := 0
		controller.ops.invoke = func(ctx context.Context, r budget.Request, cfg budget.Config, _ budget.RunInput) (budget.RunResult, error) {
			calls++
			switch kind {
			case "active":
				ledger := budget.NewLedger(controller.root)
				admission, err := ledger.Admit(ctx, r, cfg)
				Expect(err).NotTo(HaveOccurred())
				view, err := admission.Record.Observe(ctx)
				Expect(err).NotTo(HaveOccurred())
				_, err = ledger.PublishPID(ctx, view.ID, 424242)
				Expect(err).NotTo(HaveOccurred())
			case "unreadable":
				Expect(os.WriteFile(filepath.Join(controller.root, ".factory/budget.json"), []byte("PRIVATE_CORRUPT"), 0600)).To(Succeed())
			case "typed":
				return budget.RunResult{}, &native.OwnershipError{ProcessPID: 424242}
			}
			return budget.RunResult{}, errors.New("PRIVATE_NATIVE_ERROR")
		}
		result, err := controller.Run(context.Background(), request, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ExitCode).To(Equal(2))
		Expect(calls).To(Equal(1))
		raw, err := result.Record.MarshalJSON()
		Expect(err).NotTo(HaveOccurred())
		var row map[string]any
		Expect(json.Unmarshal(raw, &row)).To(Succeed())
		Expect(row["uncertain"]).To(Equal(uncertain))
		Expect(row["outcome"]).To(Equal("handoff"))
		Expect(string(raw)).NotTo(ContainSubstring("PRIVATE"))
		if kind == "active" {
			Expect(row["budget_runs"]).To(HaveLen(1))
		}
		if kind == "active" || kind == "typed" {
			Expect(row["process_pid"]).To(Equal(float64(424242)))
		} else {
			Expect(row["process_pid"]).To(BeNil())
		}
	}, Entry("certain preflight refusal", "preflight", false), Entry("matching active ledger row", "active", true), Entry("unreadable accounting", "unreadable", true), Entry("typed unknown ownership", "typed", true))
})

var _ = Describe("Bounded loop diagnostic decoding", func() {
	// per docs/adr/0076-go-bounded-loop-controller.md:90
	DescribeTable("uses Python replacement boundaries for partial diagnostic bytes", func(raw []byte, expected string) { Expect(diagnosticText(raw)).To(Equal(expected)) },
		Entry("valid UTF8", []byte("a😀b"), "a😀b"), Entry("truncated nonBMP", []byte{0xf0, 0x9f, 0x98}, "�"), Entry("invalid continuation", []byte{0xe2, 0x82, 'x'}, "�x"), Entry("surrogate encoding", []byte{0xed, 0xa0, 0x80}, "���"), Entry("overlong", []byte{0xc0, 0x80}, "��"))
})
