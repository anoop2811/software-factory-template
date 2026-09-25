package loop

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/budget"
	"github.com/anoop2811/software-factory-template/internal/native"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func manualFixture() (*ManualController, ManualRequest) {
	GinkgoHelper()
	root, _, environment := snapshotFixture()
	environment["FACTORY_LOOP_CHECK_COMMAND"] = "true"
	return NewManualController(root), ManualRequest{Harness: "codex", Session: "s", Task: "t", Environment: environment}
}

var _ = Describe("Manual loop total deadline", func() {
	// per docs/adr/0075-go-manual-loop-controller.md:73
	It("does not launch after phase publication consumes the remaining total allowance", func() {
		controller, request := manualFixture()
		request.Environment["FACTORY_LOOP_TIMEOUT_SECONDS"] = "1"
		saves := 0
		executions := 0
		controller.store.ops.syncFile = func(file *os.File) error {
			saves++
			if saves == 2 {
				time.Sleep(1100 * time.Millisecond)
			}
			return file.Sync()
		}
		controller.ops.execute = func(context.Context, string, string, map[string]string, time.Duration, func(context.Context, int) error) (native.Execution, error) {
			executions++
			return native.Execution{}, errors.New("test-owned unexpected execution")
		}
		result, err := controller.Run(context.Background(), request, false)
		Expect(saves).To(BeNumerically(">=", 2))
		Expect(executions).To(BeZero())
		var publication *PublicationError
		Expect(errors.As(err, &publication)).To(BeTrue())
		Expect(publication.MayHaveCommitted).To(BeFalse())
		Expect(result.Record).To(BeNil())
		Expect(saves).To(Equal(2))
		row := manualDurable(controller)
		Expect(row["status"]).To(Equal("active"))
		Expect(row["phase"]).To(Equal("starting"))
	})
})

func manualDurable(controller *ManualController) map[string]any {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join(controller.root, ".factory", "loops.json"))
	Expect(err).NotTo(HaveOccurred())
	var history map[string]any
	Expect(json.Unmarshal(data, &history)).To(Succeed())
	return history["runs"].([]any)[0].(map[string]any)
}

var _ = Describe("Manual loop publication barriers", func() {
	// per docs/adr/0075-go-manual-loop-controller.md:101
	DescribeTable("never retries a failed checkpoint publication", func(failAt int, afterRename bool) {
		controller, request := manualFixture()
		saves, executions := 0, 0
		failure := errors.New("test-owned publication failure")
		fault := func(file *os.File) error {
			saves++
			if saves == failAt {
				return failure
			}
			return file.Sync()
		}
		if afterRename {
			controller.store.ops.syncDirectory = fault
		} else {
			controller.store.ops.syncFile = fault
		}
		controller.ops.execute = func(ctx context.Context, _ string, _ string, _ map[string]string, _ time.Duration, spawn func(context.Context, int) error) (native.Execution, error) {
			executions++
			if err := spawn(ctx, 424242); err != nil {
				return native.Execution{ProcessPID: 424242, OwnershipUnconfirmed: true}, err
			}
			code := 0
			return native.Execution{ProcessPID: 424242, ExitConfirmed: true, ExitCode: &code, Outcome: "completed"}, nil
		}
		result, err := controller.Run(context.Background(), request, false)
		Expect(err).To(HaveOccurred())
		Expect(result.Record).To(BeNil())
		Expect(saves).To(Equal(failAt))
		if failAt <= 2 {
			Expect(executions).To(BeZero())
		} else {
			Expect(executions).To(Equal(1))
		}
		if failAt == 1 && !afterRename {
			_, err := os.Stat(filepath.Join(controller.root, ".factory", "loops.json"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		} else {
			row := manualDurable(controller)
			// A post-rename terminal error may already have committed the terminal row.
			if failAt == 5 && afterRename {
				Expect(row["status"]).To(Equal("stopped"))
			} else {
				Expect(row["status"]).To(Equal("active"))
			}
		}
	}, Entry("initial before rename", 1, false), Entry("initial after rename", 1, true), Entry("PID before rename", 3, false), Entry("PID after rename", 3, true), Entry("evidence before rename", 4, false), Entry("terminal before rename", 5, false), Entry("terminal after rename", 5, true))
	// per docs/adr/0075-go-manual-loop-controller.md:96
	It("retains a locally owned PID when cleanup cannot be confirmed", func() {
		controller, request := manualFixture()
		controller.ops.execute = func(ctx context.Context, _ string, _ string, _ map[string]string, _ time.Duration, spawn func(context.Context, int) error) (native.Execution, error) {
			Expect(spawn(ctx, 424242)).To(Succeed())
			return native.Execution{ProcessPID: 424242, OwnershipUnconfirmed: true, Outcome: "timeout"}, &native.OwnershipError{ProcessPID: 424242}
		}
		result, err := controller.Run(context.Background(), request, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ExitCode).To(Equal(2))
		row := manualDurable(controller)
		Expect(row["process_pid"]).To(Equal(float64(424242)))
		Expect(row["uncertain"]).To(BeTrue())
		Expect(row["outcome"]).To(Equal("handoff"))
	})
})

var _ = Describe("Manual loop ownership", func() {
	// per docs/adr/0075-go-manual-loop-controller.md:47
	It("keeps the persistent loop lock throughout execution", func() {
		controller, request := manualFixture()
		controller.ops.execute = func(ctx context.Context, _ string, _ string, _ map[string]string, _ time.Duration, spawn func(context.Context, int) error) (native.Execution, error) {
			file, err := os.OpenFile(filepath.Join(controller.root, ".factory", "loops.lock"), os.O_RDWR, 0600)
			Expect(err).NotTo(HaveOccurred())
			defer file.Close()
			Expect(syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)).To(MatchError(syscall.EWOULDBLOCK))
			Expect(spawn(ctx, 424242)).To(Succeed())
			code := 0
			return native.Execution{ProcessPID: 424242, ExitConfirmed: true, ExitCode: &code, Outcome: "completed"}, nil
		}
		result, err := controller.Run(context.Background(), request, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ExitCode).To(BeZero())
		transaction, err := controller.store.Lock(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(transaction.Close()).To(Succeed())
	})
	// per docs/adr/0075-go-manual-loop-controller.md:92
	It("persists a confirmed interruption without fresh probes after parent cancellation", func() {
		controller, request := manualFixture()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		originalSnapshot := controller.ops.snapshot
		originalPolicy := controller.ops.policy
		controller.ops.snapshot = func(c context.Context, r string, cfg Config, env map[string]string) (Fingerprint, error) {
			Expect(ctx.Err()).NotTo(HaveOccurred())
			return originalSnapshot(c, r, cfg, env)
		}
		controller.ops.policy = func(c context.Context, cfg Config, bc budget.Config, env map[string]string) (string, error) {
			Expect(ctx.Err()).NotTo(HaveOccurred())
			return originalPolicy(c, cfg, bc, env)
		}
		controller.ops.execute = func(c context.Context, _ string, _ string, _ map[string]string, _ time.Duration, spawn func(context.Context, int) error) (native.Execution, error) {
			Expect(spawn(c, 424242)).To(Succeed())
			cancel()
			code := -1
			return native.Execution{ProcessPID: 424242, ExitConfirmed: true, ExitCode: &code, Outcome: "interrupted"}, nil
		}
		result, err := controller.Run(ctx, request, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ExitCode).To(Equal(2))
		row := manualDurable(controller)
		Expect(row["outcome"]).To(Equal("interrupted"))
		Expect(row["uncertain"]).To(BeFalse())
		Expect(row["process_pid"]).To(BeNil())
		Expect(time.Until(result.outputDeadline)).To(BeNumerically(">", 0))
		Expect(time.Until(result.outputDeadline)).To(BeNumerically("<=", 5*time.Second))
	})
})
