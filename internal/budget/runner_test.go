package budget

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/usage"
	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func runnerFixture() (*Runner, Config, Request, RunInput) {
	ginkgo.GinkgoHelper()
	ledger, cfg, request := storageFixture()
	for _, dir := range []string{".opencode/agent", ".claude/agents"} {
		Expect(os.MkdirAll(filepath.Join(ledger.root, dir), 0700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(ledger.root, dir, "reviewer.md"), []byte("Canonical instructions\n"), 0600)).To(Succeed())
	}
	Expect(os.WriteFile(filepath.Join(ledger.root, "opencode.json"), []byte(`{"agent":{"reviewer":{"permission":{"edit":"deny"}}}}`), 0600)).To(Succeed())
	runner := NewRunner(ledger.root)
	runner.ops.preflight = func(context.Context, native.Plan) error { return nil }
	prompt := "PRIVATE_PROMPT"
	return runner, cfg, request, RunInput{Prompt: &prompt, WantResponse: true}
}

var _ = ginkgo.Describe("Budget controller collaborators", func() {
	// per docs/adr/0070-go-budget-execution-controller.md:138
	ginkgo.It("retains timeout uncertainty and typed ownership evidence", func() {
		runner, cfg, request, input := runnerFixture()
		runner.ops.execute = func(ctx context.Context, _ native.Plan, _ time.Duration, spawn func(context.Context, int) error) (native.Execution, error) {
			Expect(spawn(ctx, 1234)).To(Succeed())
			return native.Execution{Outcome: "timeout", ProcessPID: 1234, OwnershipUnconfirmed: true}, &native.OwnershipError{ProcessPID: 1234}
		}
		result, err := runner.Run(context.Background(), request, cfg, input)
		var ownership *native.OwnershipError
		Expect(errors.As(err, &ownership)).To(BeTrue())
		Expect(ownership.ProcessPID).To(Equal(1234))
		Expect(result.ExitCode).NotTo(BeZero())
		Expect(result.Response).To(BeEmpty())
		Expect(result.Record).NotTo(BeNil())
		Expect(result.Record.data["status"]).To(Equal("active"))
		Expect(result.Record.data["outcome"]).To(Equal("timeout"))
		Expect(result.Record.data["tokens"]).To(BeNil())
		Expect(result.Record.data["estimated_usd"]).To(BeNil())
	})
})

func runnerSuccess(ctx context.Context, _ native.Plan, _ time.Duration, spawn func(context.Context, int) error) (native.Execution, error) {
	code := 0
	err := spawn(ctx, 1234)
	outcome := "completed"
	if err != nil {
		outcome = "launch_error"
		code = -9
	}
	return native.Execution{Outcome: outcome, ProcessPID: 1234, ExitCode: &code, ExitConfirmed: true, Stdout: []byte(`{"type":"result","subtype":"success","result":"PRIVATE_ANSWER","usage":{"input_tokens":3,"output_tokens":2,"cache_creation_input_tokens":0,"cache_read_input_tokens":0},"total_cost_usd":0.1}`)}, err
}

var _ = ginkgo.Describe("Budget controller cleanup boundaries", func() {
	// per docs/adr/0070-go-budget-execution-controller.md:69
	ginkgo.DescribeTable("preserves publication identity without retrying or releasing an answer", func(failAt int, afterRename bool) {
		runner, cfg, request, input := runnerFixture()
		executions, syncs := 0, 0
		runner.ops.execute = func(ctx context.Context, p native.Plan, d time.Duration, spawn func(context.Context, int) error) (native.Execution, error) {
			executions++
			return runnerSuccess(ctx, p, d, spawn)
		}
		fail := func(file *os.File) error {
			syncs++
			if syncs == failAt {
				return errors.New("PRIVATE_DISK_FAILURE")
			}
			return file.Sync()
		}
		if afterRename {
			runner.ledger.ops.syncDirectory = fail
		} else {
			runner.ledger.ops.syncFile = fail
		}
		result, err := runner.Run(context.Background(), request, cfg, input)
		var publication *PublicationError
		Expect(errors.As(err, &publication)).To(BeTrue())
		Expect(publication.MayHaveCommitted).To(Equal(afterRename))
		Expect(publication.RunID).NotTo(BeEmpty())
		Expect(err.Error()).NotTo(ContainSubstring("PRIVATE"))
		Expect(result.ExitCode).NotTo(BeZero())
		Expect(result.Response).To(BeEmpty())
		if failAt == 1 {
			Expect(executions).To(BeZero())
			Expect(syncs).To(Equal(1))
			Expect(result.Record).To(BeNil())
		}
		if failAt == 2 {
			Expect(executions).To(Equal(1))
			Expect(syncs).To(Equal(3))
			Expect(result.Record).NotTo(BeNil())
			Expect(result.Record.data["outcome"]).To(Equal("launch_error"))
			Expect(result.Record.data["status"]).To(Equal("completed"))
			Expect(publication.ProcessPID).NotTo(BeNil())
			Expect(*publication.ProcessPID).To(Equal(1234))
		}
		if failAt == 3 {
			Expect(executions).To(Equal(1))
			Expect(syncs).To(Equal(3))
			Expect(result.Record).To(BeNil())
		}
	}, ginkgo.Entry("admission before rename", 1, false), ginkgo.Entry("admission after rename", 1, true), ginkgo.Entry("PID before rename", 2, false), ginkgo.Entry("PID after rename", 2, true), ginkgo.Entry("finalization before rename", 3, false), ginkgo.Entry("finalization after rename", 3, true))
	// per docs/adr/0070-go-budget-execution-controller.md:96
	ginkgo.DescribeTable("clears claims after private processing failure", func(answer bool) {
		runner, cfg, request, input := runnerFixture()
		runner.ops.execute = runnerSuccess
		if answer {
			runner.ops.response = func(context.Context, string, io.Reader) (string, error) {
				return "PRIVATE_PARTIAL", errors.New("PRIVATE_ANSWER_FAILURE")
			}
		} else {
			runner.ops.parse = func(context.Context, string, io.Reader) (usage.Metadata, error) {
				return usage.Metadata{}, errors.New("PRIVATE_PARSE_FAILURE")
			}
		}
		result, err := runner.Run(context.Background(), request, cfg, input)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).NotTo(ContainSubstring("PRIVATE"))
		Expect(result.ExitCode).To(Equal(1))
		Expect(result.Response).To(BeEmpty())
		Expect(result.Record).NotTo(BeNil())
		Expect(result.Record.data["outcome"]).To(Equal("launch_error"))
		Expect(result.Record.data["tokens"]).To(BeNil())
		Expect(result.Record.data["estimated_usd"]).To(BeNil())
	}, ginkgo.Entry("parser", false), ginkgo.Entry("answer", true))
	// per docs/adr/0070-go-budget-execution-controller.md:76
	ginkgo.It("detaches bounded cleanup while retaining context values", func() {
		runner, cfg, request, input := runnerFixture()
		type contextKey struct{}
		ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "retained"))
		defer cancel()
		runner.ops.execute = func(ctx context.Context, p native.Plan, d time.Duration, spawn func(context.Context, int) error) (native.Execution, error) {
			execution, err := runnerSuccess(ctx, p, d, spawn)
			cancel()
			return execution, err
		}
		observed := false
		runner.ops.parse = func(cleanup context.Context, h string, r io.Reader) (usage.Metadata, error) {
			observed = true
			Expect(cleanup.Err()).NotTo(HaveOccurred())
			Expect(cleanup.Value(contextKey{})).To(Equal("retained"))
			deadline, ok := cleanup.Deadline()
			Expect(ok).To(BeTrue())
			Expect(time.Until(deadline)).To(BeNumerically(">", 0))
			Expect(time.Until(deadline)).To(BeNumerically("<=", 5*time.Second))
			return usage.Parse(cleanup, h, r)
		}
		result, err := runner.Run(ctx, request, cfg, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(observed).To(BeTrue())
		Expect(result.ExitCode).To(BeZero())
		Expect(result.Record.data["status"]).To(Equal("completed"))
	})
})

var _ = ginkgo.Describe("Budget controller admission policy", func() {
	// per docs/adr/0070-go-budget-execution-controller.md:66
	ginkgo.It("rechecks a slot consumed during preflight", func() {
		runner, cfg, request, input := runnerFixture()
		cfg.MaxSessionRuns.SetInt64(1)
		runner.ops.preflight = func(ctx context.Context, _ native.Plan) error {
			admission, err := runner.ledger.Admit(ctx, request, cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(admission.Record).NotTo(BeNil())
			return nil
		}
		runner.ops.execute = func(context.Context, native.Plan, time.Duration, func(context.Context, int) error) (native.Execution, error) {
			ginkgo.Fail("blocked request executed")
			return native.Execution{}, nil
		}
		result, err := runner.Run(context.Background(), request, cfg, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ExitCode).To(Equal(2))
		Expect(result.Record).To(BeNil())
	})
	// per docs/adr/0070-go-budget-execution-controller.md:61
	// per docs/adr/0070-go-budget-execution-controller.md:219
	ginkgo.It("recomputes reservation duration after waiting for the lock", func() {
		runner, cfg, request, input := runnerFixture()
		syncs := 0
		runner.ledger.ops.syncFile = func(file *os.File) error {
			syncs++
			if syncs == 1 {
				time.Sleep(200 * time.Millisecond)
			}
			return file.Sync()
		}
		file, err := os.OpenFile(filepath.Join(runner.ledger.root, ".factory/budget.lock"), os.O_CREATE|os.O_RDWR, 0600)
		Expect(err).NotTo(HaveOccurred())
		defer file.Close()
		fd := int(file.Fd())
		Expect(syscall.Flock(fd, syscall.LOCK_EX)).To(Succeed())
		type releaseEvent struct {
			at  time.Time
			err error
		}
		released := make(chan releaseEvent, 1)
		done := make(chan struct{})
		workerStarted := false
		// Join on assertion panic as well as success, before the earlier deferred Close.
		defer func() {
			if workerStarted {
				<-done
			}
		}()
		runner.ops.preflight = func(context.Context, native.Plan) error {
			workerStarted = true
			go func() {
				defer close(done)
				time.Sleep(200 * time.Millisecond)
				at := time.Now()
				err := syscall.Flock(fd, syscall.LOCK_UN)
				released <- releaseEvent{at: at, err: err}
			}()
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		parentDeadline, ok := ctx.Deadline()
		Expect(ok).To(BeTrue())
		runner.ops.execute = func(executionCtx context.Context, p native.Plan, d time.Duration, spawn func(context.Context, int) error) (native.Execution, error) {
			event := <-released
			Expect(event.err).To(Succeed())
			deadline, ok := executionCtx.Deadline()
			Expect(ok).To(BeTrue())
			Expect(deadline).To(Equal(parentDeadline))
			Expect(d).To(BeNumerically(">", 0))
			Expect(d).To(BeNumerically("<=", parentDeadline.Sub(event.at)), "reservation must deduct the time waiting for the ledger lock")
			return runnerSuccess(executionCtx, p, d, spawn)
		}
		result, err := runner.Run(ctx, request, cfg, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ExitCode).To(BeZero())
	})
	// per docs/adr/0070-go-budget-execution-controller.md:76
	ginkgo.It("bounds finalization lock waiting independently of cancellation", func() {
		runner, cfg, request, input := runnerFixture()
		file, err := os.OpenFile(filepath.Join(runner.ledger.root, ".factory/budget.lock"), os.O_CREATE|os.O_RDWR, 0600)
		Expect(err).NotTo(HaveOccurred())
		defer file.Close()
		defer func() { Expect(syscall.Flock(int(file.Fd()), syscall.LOCK_UN)).To(Succeed()) }()
		// per docs/adr/0070-go-budget-execution-controller.md:198
		var started time.Time
		runner.ops.execute = func(ctx context.Context, p native.Plan, d time.Duration, spawn func(context.Context, int) error) (native.Execution, error) {
			execution, err := runnerSuccess(ctx, p, d, spawn)
			Expect(syscall.Flock(int(file.Fd()), syscall.LOCK_EX)).To(Succeed())
			started = time.Now()
			return execution, err
		}
		result, err := runner.Run(context.Background(), request, cfg, input)
		Expect(err).To(HaveOccurred())
		Expect(time.Since(started)).To(BeNumerically(">=", 4900*time.Millisecond))
		Expect(time.Since(started)).To(BeNumerically("<", 7*time.Second))
		Expect(result.Record).To(BeNil())
		Expect(result.Response).To(BeEmpty())
		Expect(result.ExitCode).NotTo(BeZero())
	})
})

var _ = ginkgo.Describe("Budget controller compounded publication failure", func() {
	// per docs/adr/0070-go-budget-execution-controller.md:87
	ginkgo.It("retains the original PID publication error when finalization also fails", func() {
		runner, cfg, request, input := runnerFixture()
		syncs := 0
		runner.ledger.ops.syncFile = func(file *os.File) error {
			syncs++
			if syncs >= 2 {
				return errors.New("PRIVATE_FAILURE")
			}
			return file.Sync()
		}
		var original *PublicationError
		runner.ops.execute = func(ctx context.Context, p native.Plan, d time.Duration, spawn func(context.Context, int) error) (native.Execution, error) {
			execution, err := runnerSuccess(ctx, p, d, spawn)
			Expect(errors.As(err, &original)).To(BeTrue())
			return execution, err
		}
		result, err := runner.Run(context.Background(), request, cfg, input)
		Expect(errors.Is(err, original)).To(BeTrue())
		Expect(syncs).To(Equal(3))
		Expect(result.Record).To(BeNil())
		Expect(result.Response).To(BeEmpty())
		Expect(result.ExitCode).To(Equal(1))
	})
})

var _ = ginkgo.Describe("Budget controller read snapshot replacement", func() {
	// per docs/adr/0070-go-budget-execution-controller.md:146
	ginkgo.It("reacquires a validated read-only snapshot after a cooperating atomic rename", func() {
		ledger, _, _ := storageFixture()
		calls := 0
		ledger.ops.openHistory = func(root *os.Root) (*os.File, error) {
			calls++
			if calls == 1 {
				Expect(os.WriteFile(filepath.Join(ledger.root, ".factory/replacement"), []byte(`{"schema":1,"runs":[],"retained":"new"}`), 0600)).To(Succeed())
				Expect(os.Rename(filepath.Join(ledger.root, ".factory/replacement"), filepath.Join(ledger.root, ".factory/budget.json"))).To(Succeed())
			}
			return root.OpenFile("budget.json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		}
		history, err := ledger.Read(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(calls).To(Equal(2))
		Expect(history.data["retained"]).To(Equal("new"))
	})
})

type snapshotContext struct {
	context.Context
	afterOpen func()
}

func (c *snapshotContext) Err() error {
	if c.afterOpen != nil {
		f := c.afterOpen
		c.afterOpen = nil
		f()
	}
	return c.Context.Err()
}

var _ = ginkgo.Describe("Budget controller snapshot retry limits", func() {
	// per docs/adr/0070-go-budget-execution-controller.md:146
	ginkgo.DescribeTable("handles replaced descriptors without weakening stable validation", func(afterStat bool) {
		ledger, _, _ := storageFixture()
		calls := 0
		ctx := &snapshotContext{Context: context.Background()}
		replace := func() {
			Expect(os.WriteFile(filepath.Join(ledger.root, ".factory/new"), []byte(`{"schema":1,"runs":[],"new":true}`), 0600)).To(Succeed())
			Expect(os.Rename(filepath.Join(ledger.root, ".factory/new"), filepath.Join(ledger.root, ".factory/budget.json"))).To(Succeed())
		}
		ledger.ops.openHistory = func(root *os.Root) (*os.File, error) {
			calls++
			file, err := root.OpenFile("budget.json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
			if calls == 1 && err == nil {
				if afterStat {
					ctx.afterOpen = replace
				} else {
					replace()
				}
			}
			return file, err
		}
		history, err := ledger.Read(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(calls).To(Equal(2))
		Expect(history.data["new"]).To(BeTrue())
	}, ginkgo.Entry("unlinked before descriptor inspection", false), ginkgo.Entry("unlinked during parsing", true))
	// per docs/adr/0070-go-budget-execution-controller.md:149
	ginkgo.DescribeTable("does not retry nonreplacement errors and bounds repeated replacement", func(kind string, expectedCalls int) {
		ledger, _, _ := storageFixture()
		calls := 0
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ledger.ops.openHistory = func(root *os.Root) (*os.File, error) {
			calls++
			path := filepath.Join(ledger.root, ".factory/budget.json")
			replacement := filepath.Join(ledger.root, ".factory/new")
			switch kind {
			case "open":
				return nil, errors.New("PRIVATE_OPEN")
			case "parse":
				Expect(os.WriteFile(path, []byte("malformed"), 0600)).To(Succeed())
			case "symlink":
				Expect(os.Remove(path)).To(Succeed())
				Expect(os.Symlink("target", path)).To(Succeed())
			case "hardlink":
				Expect(os.Link(path, replacement)).To(Succeed())
			case "exhaustion", "cancel":
				Expect(os.WriteFile(replacement, []byte(`{"schema":1,"runs":[]}`), 0600)).To(Succeed())
				Expect(os.Rename(replacement, path)).To(Succeed())
				if kind == "cancel" {
					cancel()
				}
			}
			return root.OpenFile("budget.json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		}
		_, err := ledger.Read(ctx)
		Expect(err).To(HaveOccurred())
		Expect(calls).To(Equal(expectedCalls))
	}, ginkgo.Entry("generic open failure", "open", 1), ginkgo.Entry("malformed JSON", "parse", 1), ginkgo.Entry("symlink replacement", "symlink", 1), ginkgo.Entry("hardlink inode", "hardlink", 1), ginkgo.Entry("eight replacements", "exhaustion", 8), ginkgo.Entry("canceled replacement", "cancel", 1))
})

var _ = ginkgo.Describe("Budget controller persistent lock creation", func() {
	// per docs/adr/0070-go-budget-execution-controller.md:167
	ginkgo.DescribeTable("opens a stable persistent lock without concurrent shared creation", func(competitor bool) {
		ledger, cfg, request := storageFixture()
		calls := []int{}
		ledger.ops.openLock = func(root *os.Root, flags int) (*os.File, error) {
			calls = append(calls, flags)
			if flags&os.O_CREATE != 0 && flags&os.O_EXCL == 0 {
				return nil, syscall.ENOENT
			}
			if competitor && len(calls) == 1 {
				file, err := root.OpenFile("budget.lock", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
				Expect(err).NotTo(HaveOccurred())
				Expect(file.Close()).To(Succeed())
				return nil, os.ErrExist
			}
			return root.OpenFile("budget.lock", flags, 0600)
		}
		admission, err := ledger.Admit(context.Background(), request, cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(admission.Record).NotTo(BeNil())
		base := os.O_RDWR | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
		if competitor {
			Expect(calls).To(Equal([]int{base | os.O_CREATE | os.O_EXCL, base}))
		} else {
			Expect(calls).To(Equal([]int{base | os.O_CREATE | os.O_EXCL}))
		}
		before, err := os.Stat(filepath.Join(ledger.root, ".factory/budget.lock"))
		Expect(err).NotTo(HaveOccurred())
		ledger.ops.openLock = nil
		_, err = ledger.PublishPID(context.Background(), admission.Record.data["id"].(string), 1234)
		Expect(err).NotTo(HaveOccurred())
		after, err := os.Stat(filepath.Join(ledger.root, ".factory/budget.lock"))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
	}, ginkgo.Entry("new lock", false), ginkgo.Entry("competing creator", true))
	// per docs/adr/0070-go-budget-execution-controller.md:167
	ginkgo.DescribeTable("does not retry generic lock open failures", func(failure error) {
		ledger, cfg, request := storageFixture()
		calls := 0
		ledger.ops.openLock = func(*os.Root, int) (*os.File, error) { calls++; return nil, failure }
		admission, err := ledger.Admit(context.Background(), request, cfg)
		Expect(err).To(HaveOccurred())
		Expect(admission.Record).To(BeNil())
		Expect(calls).To(Equal(1))
	}, ginkgo.Entry("missing pathname", syscall.ENOENT), ginkgo.Entry("permission", syscall.EACCES))
})

var _ = ginkgo.Describe("Budget controller vanished lock", func() {
	// per docs/adr/0070-go-budget-execution-controller.md:170
	ginkgo.It("refuses disappearance between exclusive creation and existing open", func() {
		ledger, cfg, request := storageFixture()
		calls := 0
		ledger.ops.openLock = func(_ *os.Root, _ int) (*os.File, error) {
			calls++
			if calls == 1 {
				return nil, os.ErrExist
			}
			return nil, syscall.ENOENT
		}
		admission, err := ledger.Admit(context.Background(), request, cfg)
		Expect(err).To(HaveOccurred())
		Expect(admission.Record).To(BeNil())
		Expect(calls).To(Equal(2))
		_, err = os.Stat(filepath.Join(ledger.root, ".factory/budget.lock"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

var _ = ginkgo.Describe("Budget controller missing process identity", func() {
	// per docs/adr/0070-go-budget-execution-controller.md:93
	ginkgo.It("skips usage and response processing after completed execution is downgraded to launch error", func() {
		runner, cfg, request, input := runnerFixture()
		runner.ops.execute = func(context.Context, native.Plan, time.Duration, func(context.Context, int) error) (native.Execution, error) {
			code := 0
			return native.Execution{Outcome: "completed", ExitCode: &code, ExitConfirmed: true, Stdout: []byte("PRIVATE_UNOWNED_OUTPUT")}, nil
		}
		parsed, answered := 0, 0
		runner.ops.parse = func(context.Context, string, io.Reader) (usage.Metadata, error) {
			parsed++
			return usage.Metadata{Complete: true}, nil
		}
		runner.ops.response = func(context.Context, string, io.Reader) (string, error) {
			answered++
			return "PRIVATE_UNOWNED_ANSWER", nil
		}
		result, err := runner.Run(context.Background(), request, cfg, input)
		Expect(err).To(HaveOccurred())
		Expect(parsed).To(BeZero(), "launch_error must not parse unowned process output")
		Expect(answered).To(BeZero(), "launch_error must not select an answer")
		Expect(result.ExitCode).NotTo(BeZero())
		Expect(result.Response).To(BeEmpty())
		Expect(result.Record).To(BeNil())
	})
})
