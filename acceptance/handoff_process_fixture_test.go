package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type handoffProcessObservation struct {
	PID                                                       int
	Started, ContextEnded, LeaderExited, StdoutEOF, StderrEOF time.Time
	CompletionMessage, Returned                               time.Time
	CaptureForcedClosed                                       bool
	LeaderUnconfirmed                                         bool
}

type handoffProcessTrace struct {
	mu          sync.Mutex
	observation handoffProcessObservation
}

func (trace *handoffProcessTrace) record(change func(*handoffProcessObservation)) {
	trace.mu.Lock()
	defer trace.mu.Unlock()
	change(&trace.observation)
}

func (trace *handoffProcessTrace) snapshot() handoffProcessObservation {
	trace.mu.Lock()
	defer trace.mu.Unlock()
	return trace.observation
}

type handoffProcessCapture struct {
	mu             sync.Mutex
	buffer         bytes.Buffer
	trace          *handoffProcessTrace
	completionSeen bool
}

func (capture *handoffProcessCapture) Write(data []byte) (int, error) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	const marker = "factory local-source: installation completed"
	// Retain the possible cross-write prefix without rescanning old output.
	start := max(0, capture.buffer.Len()-len(marker)+1)
	n, err := capture.buffer.Write(data)
	if !capture.completionSeen && bytes.Contains(capture.buffer.Bytes()[start:], []byte(marker)) {
		capture.completionSeen = true
		capture.trace.record(func(value *handoffProcessObservation) {
			if value.CompletionMessage.IsZero() {
				value.CompletionMessage = time.Now()
			}
		})
	}
	return n, err
}

func (capture *handoffProcessCapture) text() string {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.buffer.String()
}

// Explicit file descriptors separate the leader's Wait from capture drainage.
// TERM first lets native owners reconcile separately grouped workers; unknown
// captures remain a failure and never authorize a process-tree kill.
// per docs/adr/0131-bounded-ci-feedback.md:147
func handoffObservedRun(ctx context.Context, command *exec.Cmd, trace *handoffProcessTrace) (cliResult, error) {
	defer trace.record(func(value *handoffProcessObservation) { value.Returned = time.Now() })
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		return cliResult{status: -1}, err
	}
	defer stdoutRead.Close()
	defer stdoutWrite.Close()
	stderrRead, stderrWrite, err := os.Pipe()
	if err != nil {
		return cliResult{status: -1}, err
	}
	defer stderrRead.Close()
	defer stderrWrite.Close()
	command.Stdout, command.Stderr = stdoutWrite, stderrWrite
	command.Cancel = func() error {
		trace.record(func(value *handoffProcessObservation) {
			if value.ContextEnded.IsZero() {
				value.ContextEnded = time.Now()
			}
		})
		return command.Process.Signal(syscall.SIGTERM)
	}
	// This bounds only post-deadline cleanup, never the unchanged operation budget.
	command.WaitDelay = 6 * time.Second
	if err := command.Start(); err != nil {
		return cliResult{status: -1}, err
	}
	trace.record(func(value *handoffProcessObservation) { value.PID, value.Started = command.Process.Pid, time.Now() })
	_ = stdoutWrite.Close()
	_ = stderrWrite.Close()
	stdout, stderr := &handoffProcessCapture{trace: trace}, &handoffProcessCapture{trace: trace}
	leader, output, diagnostic := make(chan error, 1), make(chan error, 1), make(chan error, 1)
	go func() {
		waitErr := command.Wait()
		trace.record(func(value *handoffProcessObservation) { value.LeaderExited = time.Now() })
		leader <- waitErr
	}()
	go func() {
		_, copyErr := io.Copy(stdout, stdoutRead)
		if copyErr == nil {
			trace.record(func(value *handoffProcessObservation) { value.StdoutEOF = time.Now() })
		}
		output <- copyErr
	}()
	go func() {
		_, copyErr := io.Copy(stderr, stderrRead)
		if copyErr == nil {
			trace.record(func(value *handoffProcessObservation) { value.StderrEOF = time.Now() })
		}
		diagnostic <- copyErr
	}()
	contextDone := ctx.Done()
	var cleanup *time.Timer
	var cleanupDone <-chan time.Time
	defer func() {
		if cleanup != nil {
			cleanup.Stop()
		}
	}()
	var waitErr, captureErr error
	forced := false
	for leader != nil || output != nil || diagnostic != nil {
		select {
		case waitErr = <-leader:
			leader = nil
		case copyErr := <-output:
			captureErr = errors.Join(captureErr, copyErr)
			output = nil
		case copyErr := <-diagnostic:
			captureErr = errors.Join(captureErr, copyErr)
			diagnostic = nil
		case <-contextDone:
			trace.record(func(value *handoffProcessObservation) {
				if value.ContextEnded.IsZero() {
					value.ContextEnded = time.Now()
				}
			})
			contextDone = nil
			cleanup = time.NewTimer(6 * time.Second)
			cleanupDone = cleanup.C
		case <-cleanupDone:
			if forced {
				trace.record(func(value *handoffProcessObservation) { value.LeaderUnconfirmed = leader != nil })
				return cliResult{stdout.text(), stderr.text(), -1}, errors.Join(ctx.Err(), errors.New("handoff fixture closure remains unconfirmed"))
			}
			forced = true
			trace.record(func(value *handoffProcessObservation) { value.CaptureForcedClosed = output != nil || diagnostic != nil })
			_ = stdoutRead.Close()
			_ = stderrRead.Close()
			if leader != nil {
				_ = command.Process.Kill()
			}
			cleanup.Reset(time.Second)
		}
	}
	status := 0
	if waitErr != nil {
		status = -1
		var exited *exec.ExitError
		if errors.As(waitErr, &exited) {
			status = exited.ExitCode()
		}
	}
	return cliResult{stdout.text(), stderr.text(), status}, errors.Join(waitErr, captureErr, ctx.Err())
}

// The separately grouped child publishes its identity only after installing its
// TERM handler and retaining both actual capture descriptors.
// per docs/adr/0131-bounded-ci-feedback.md:147
const handoffProcessWitness = `package main
import("encoding/json";"fmt";"os";"os/exec";"os/signal";"strconv";"syscall";"time")
func wait(path string)bool{until:=time.Now().Add(15*time.Second);for time.Now().Before(until){if _,err:=os.Stat(path);err==nil{return true};time.Sleep(5*time.Millisecond)};return false}
func main(){
 mode,ready:=os.Args[1],os.Args[2]
 signals:=make(chan os.Signal,1);signal.Notify(signals,syscall.SIGTERM)
 if mode=="child"{
  leader,_:=strconv.Atoi(os.Args[3]);group,_:=syscall.Getpgid(0)
  body,_:=json.Marshal(map[string]int{"PID":os.Getpid(),"Leader":leader,"Group":group})
  if os.WriteFile(ready+".tmp",body,0600)!=nil||os.Rename(ready+".tmp",ready)!=nil{os.Exit(12)}
  fmt.Fprintln(os.Stderr,"factory local-source: installation completed")
  released:=make(chan struct{});go func(){wait(ready+".release");close(released)}()
  select{case <-signals:case <-released:};return
 }
 executable,err:=os.Executable();if err!=nil{os.Exit(13)}
 child:=exec.Command(executable,"child",ready,strconv.Itoa(os.Getpid()))
 child.SysProcAttr=&syscall.SysProcAttr{Setpgid:true};child.Stdout=os.Stdout;child.Stderr=os.Stderr
 if child.Start()!=nil{os.Exit(14)}
 if !wait(ready){child.Process.Kill();child.Wait();os.Exit(15)}
 if mode=="leader-exits"{return}
 <-signals
 if child.Process.Signal(syscall.SIGTERM)!=nil{os.Exit(16)}
 if child.Wait()!=nil{os.Exit(17)}
 os.Exit(23)
}
`

type handoffProcessReceipt struct{ PID, Leader, Group int }
type handoffProcessReturn struct {
	result cliResult
	err    error
}

var _ = Describe("Handoff process fixture ownership", Ordered, ContinueOnFailure, func() {
	var binary, work string
	BeforeAll(func() {
		var err error
		work, err = os.MkdirTemp("", "factory-handoff-process-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupFixtureDirectory, work)
		binary = filepath.Join(work, "witness")
		source := filepath.Join(work, "witness.go")
		writeFixture(source, []byte(handoffProcessWitness), 0600)
		build := imageProcess(work, nil, "go", "build", "-trimpath", "-buildvcs=false", "-o", binary, source)
		Expect(build.status).To(BeZero(), "%+v", build)
	})
	// per docs/adr/0131-bounded-ci-feedback.md:147
	DescribeTable("separates leader exit from retained capture ownership", func(mode string) {
		captureTrace := &handoffProcessTrace{}
		capture := &handoffProcessCapture{trace: captureTrace}
		marker := "factory local-source: installation completed"
		prefix := string(bytes.Repeat([]byte("unrelated output\n"), 128))
		for _, part := range []string{prefix, marker[:7], marker[7:19]} {
			n, err := capture.Write([]byte(part))
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(len(part)))
			Expect(captureTrace.snapshot().CompletionMessage.IsZero()).To(BeTrue())
		}
		_, err := capture.Write([]byte(marker[19:]))
		Expect(err).NotTo(HaveOccurred())
		firstSeen := captureTrace.snapshot().CompletionMessage
		Expect(firstSeen.IsZero()).To(BeFalse(), "a completion marker may span multiple writes")
		_, err = capture.Write([]byte(" continued " + marker))
		Expect(err).NotTo(HaveOccurred())
		Expect(captureTrace.snapshot().CompletionMessage).To(Equal(firstSeen))
		Expect(capture.text()).To(Equal(prefix + marker + " continued " + marker))

		ready := filepath.Join(work, mode+".json")
		ctx, cancel := context.WithCancel(context.Background())
		trace := &handoffProcessTrace{}
		command := handoffCommand(ctx, work, nil, "", binary, mode, ready)
		finished := make(chan handoffProcessReturn, 1)
		go func() {
			result, err := handoffObservedRun(ctx, command, trace)
			finished <- handoffProcessReturn{result, err}
		}()
		var receipt handoffProcessReceipt
		joined, admitted, descendantClosed := false, false, false
		DeferCleanup(func() error {
			cancel()
			settled := joined && descendantClosed
			defer func() {
				if !settled {
					holdFixtureCleanup(work, "handoff witness readiness or process closure remains unconfirmed")
				}
				AddReportEntry("handoff-fixture.control-cleanup", map[string]any{"work": work, "ready": ready, "admitted": admitted, "joined": joined, "descendant_closed": descendantClosed, "retained": !settled, "process": trace.snapshot()}, ReportEntryVisibilityNever)
			}()
			if settled {
				return nil
			}
			// The helper may use six seconds for cooperation plus one for final
			// observation. This single cleanup allowance includes both phases.
			deadline := time.Now().Add(8 * time.Second)
			// An independently admitted numeric receipt, not process-tree guessing,
			// owns this deliberately retained witness on failure and exit-first paths.
			if admitted && receipt.PID > 1 {
				if group, err := syscall.Getpgid(receipt.PID); err == nil && group == receipt.Group {
					_ = syscall.Kill(receipt.PID, syscall.SIGKILL)
				}
			}
			releaseErr := os.WriteFile(ready+".release", nil, 0600)
			if !joined {
				select {
				case <-finished:
					joined = true
				case <-time.After(time.Until(deadline)):
					return errors.Join(releaseErr, errors.New("handoff witness caller did not settle within cleanup allowance"))
				}
			}
			for admitted && !descendantClosed && time.Now().Before(deadline) {
				if err := syscall.Kill(receipt.PID, 0); errors.Is(err, syscall.ESRCH) {
					descendantClosed, receipt.PID = true, 0
					break
				} else if err != nil {
					return errors.Join(releaseErr, err)
				}
				time.Sleep(10 * time.Millisecond)
			}
			settled = joined && descendantClosed
			if admitted && !descendantClosed {
				return errors.Join(releaseErr, errors.New("admitted handoff witness remains unconfirmed"))
			}
			return releaseErr
		})
		Eventually(func() bool {
			data, err := os.ReadFile(ready)
			return err == nil && json.Unmarshal(data, &receipt) == nil && receipt.PID > 1 && receipt.Leader > 1 && receipt.Group == receipt.PID
		}, 3*time.Second).Should(BeTrue())
		Expect(receipt.PID).NotTo(Equal(receipt.Leader))
		Expect(syscall.Kill(receipt.PID, 0)).To(Succeed())
		group, err := syscall.Getpgid(receipt.PID)
		Expect(err).NotTo(HaveOccurred())
		Expect(group).To(Equal(receipt.Group))
		admitted = true
		if mode == "leader-exits" {
			Eventually(func() bool { return !trace.snapshot().LeaderExited.IsZero() }, 3*time.Second).Should(BeTrue(), "leader exit must not wait for a separately grouped capture holder")
			Expect(trace.snapshot().StdoutEOF.IsZero()).To(BeTrue())
			Expect(trace.snapshot().StderrEOF.IsZero()).To(BeTrue())
			Expect(syscall.Kill(receipt.PID, 0)).To(Succeed())
		}
		cancel()
		var returned handoffProcessReturn
		Eventually(finished, 8*time.Second).Should(Receive(&returned))
		joined = true
		Expect(ctx.Err()).To(MatchError(context.Canceled))
		Expect(errors.Is(returned.err, context.Canceled)).To(BeTrue(), "capture drainage must never erase cancellation")
		if mode == "cooperative" {
			Expect(returned.result.status).To(Equal(23), "TERM must reach the owner, which reaps its separate child before returning")
		} else {
			Expect(returned.result.status).To(BeZero())
			Expect(syscall.Kill(receipt.PID, 0)).To(Succeed(), "unknown separate groups must not be signalled or described as cleaned")
			Expect(os.WriteFile(ready+".release", nil, 0600)).To(Succeed())
		}
		Eventually(func() error { return syscall.Kill(receipt.PID, 0) }, 3*time.Second).Should(Equal(syscall.ESRCH))
		descendantClosed = true
		receipt.PID = 0
		observation := trace.snapshot()
		Expect(observation.LeaderExited.IsZero()).To(BeFalse())
		Expect(observation.ContextEnded.IsZero()).To(BeFalse())
		Expect(observation.LeaderUnconfirmed).To(BeFalse())
		if mode == "cooperative" {
			Expect(observation.StdoutEOF.IsZero()).To(BeFalse())
			Expect(observation.StderrEOF.IsZero()).To(BeFalse())
			Expect(observation.CaptureForcedClosed).To(BeFalse())
		} else {
			Expect(observation.StdoutEOF.IsZero()).To(BeTrue())
			Expect(observation.StderrEOF.IsZero()).To(BeTrue())
			Expect(observation.CaptureForcedClosed).To(BeTrue())
			Expect(observation.LeaderExited.Before(observation.ContextEnded)).To(BeTrue())
		}
		Expect(observation.CompletionMessage.IsZero()).To(BeFalse())
		AddReportEntry("handoff-fixture.boundary-proof", observation, ReportEntryVisibilityNever)
	}, Entry("cooperative owner", "cooperative"), Entry("leader exits before capture", "leader-exits"))
})
