package acceptance_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type parallelReviewProcess struct {
	command        *exec.Cmd
	finished       chan error
	stdout, stderr parallelReviewCapture
	reaped         bool
}

// This evaluator-only entry point calls the public package API with a real
// parent context. Its diagnostic collaborator cancels at the acknowledged final
// completion, avoiding an external signal racing an already-exiting process.
// per docs/adr/0112-bounded-parallel-review.md:71
const parallelReviewCancelDriver = `package main
import("context";"io";"os";"strings";"sync";"github.com/anoop2811/software-factory-template/internal/reviewbatch")
type cancelWriter struct{cancel context.CancelFunc;once sync.Once}
func(w *cancelWriter)Write(data []byte)(int,error){
 if strings.Contains(string(data),"batch 1/1 status=complete"){w.once.Do(func(){w.cancel();_,_=io.WriteString(os.Stderr,"owned parent cancellation observed\n")})}
 return os.Stderr.Write(data)
}
func main(){
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 environment:=map[string]string{}
 for _,entry:=range os.Environ(){key,value,_:=strings.Cut(entry,"=");environment[key]=value}
 args:=os.Args[1:];if len(args)>0&&args[0]=="review-batch"{args=args[1:]}
 os.Exit(reviewbatch.Run(ctx,args,environment,os.Stdout,&cancelWriter{cancel:cancel}))
}
`

type parallelReviewCapture struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (c *parallelReviewCapture) Write(data []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buffer.Write(data)
}
func (c *parallelReviewCapture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buffer.String()
}

func parallelReviewStart(command *exec.Cmd) *parallelReviewProcess {
	GinkgoHelper()
	p := &parallelReviewProcess{command: command, finished: make(chan error, 1)}
	command.Stdout, command.Stderr = &p.stdout, &p.stderr
	Expect(command.Start()).To(Succeed())
	go func() { p.finished <- command.Wait() }()
	DeferCleanup(func() {
		if p.reaped {
			return
		}
		_ = command.Process.Kill()
		select {
		case <-p.finished:
			p.reaped = true
		case <-time.After(3 * time.Second):
			Fail("the test-owned review CLI was not reaped within bounded cleanup")
		}
	})
	return p
}

func (p *parallelReviewProcess) wait() cliResult {
	GinkgoHelper()
	var returned error
	select {
	case returned = <-p.finished:
		p.reaped = true
	case <-time.After(10 * time.Second):
		Fail("parallel review exceeded its independent fixture watchdog")
	}
	status := 0
	if returned != nil {
		var exit *exec.ExitError
		Expect(errors.As(returned, &exit)).To(BeTrue())
		status = exit.ExitCode()
	}
	Expect(p.stdout.String() + p.stderr.String()).NotTo(ContainSubstring("PRIVATE_BATCH_KEY"))
	return cliResult{p.stdout.String(), p.stderr.String(), status}
}

func parallelReviewInput(cwd string, count int) {
	GinkgoHelper()
	paths := make([]string, count)
	for i := range paths {
		paths[i] = fmt.Sprintf("src/file-%03d.go", i)
	}
	batchWriteInput(cwd, batchAdded(paths, 30))
}

func parallelReviewArrival(arrivals <-chan int) int {
	GinkgoHelper()
	select {
	case request := <-arrivals:
		return request
	case <-time.After(2 * time.Second):
		Fail("a second real HTTPS request did not overlap the held first request")
		return 0
	}
}

func parallelReviewResponse(w http.ResponseWriter, findings string) {
	w.Header().Set("Content-Type", "text/event-stream")
	stream := strings.Replace(reviewComplete, "Safe findings", findings, 1)
	stream = strings.Replace(stream, "data: [DONE]", "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":4,\"total_tokens\":14,\"cost\":0.25}}\n\ndata: [DONE]", 1)
	_, _ = io.WriteString(w, stream) // #nosec G705 -- inert TLS fixture emits text/event-stream to the compiled CLI; injected authorization data tests redaction and is never HTML.
}

func parallelReviewIndex(service *reviewService, attempt int) int {
	data := string(service.observed()[attempt-1].body)
	for i := range 8 {
		if strings.Contains(data, fmt.Sprintf("diff --git a/src/file-%03d.go ", i)) {
			return i
		}
	}
	return -1
}

func parallelReviewCached(cache, text string) bool {
	entries, err := os.ReadDir(cache)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(cache, entry.Name()))
		if err == nil && bytes.Contains(data, []byte(text)) {
			return true
		}
	}
	return false
}

var _ = Describe("Bounded parallel adversarial review", func() {
	// per docs/adr/0112-bounded-parallel-review.md:42
	// per docs/adr/0112-bounded-parallel-review.md:108
	DescribeTable("overlaps real paid requests without exceeding the explicit in-flight bound", func(workers int) {
		root, cwd := fixture()
		parallelReviewInput(cwd, 6)
		arrivals := make(chan int, 16)
		release := make(chan struct{})
		var once sync.Once
		var active, peak atomic.Int32
		service := reviewFixture(cwd, func(w http.ResponseWriter, r *http.Request, count int) {
			current := active.Add(1)
			defer active.Add(-1)
			for previous := peak.Load(); current > previous; previous = peak.Load() {
				if peak.CompareAndSwap(previous, current) {
					break
				}
			}
			arrivals <- count
			select {
			case <-release:
				parallelReviewResponse(w, "complete")
			case <-r.Context().Done():
			}
		})
		DeferCleanup(func() { once.Do(func() { close(release) }) })
		out := filepath.Join(cwd, "parallel")
		process := parallelReviewStart(batchCommand(root, cwd, out, service, nil, []string{"REVIEW_PARALLELISM=" + strconv.Itoa(workers), "REVIEW_TIMEOUT_SECONDS=8"}))
		for range workers {
			parallelReviewArrival(arrivals)
		}
		Consistently(arrivals, 150*time.Millisecond).ShouldNot(Receive(), "held requests occupy every permitted slot")
		once.Do(func() { close(release) })
		result := process.wait()
		Expect(result.status).To(BeZero(), "%+v", result)
		Expect(peak.Load()).To(Equal(int32(workers)))
		Expect(service.observed()).To(HaveLen(6))
		report := batchReadReport(out)
		Expect(report["status"]).To(Equal("complete"))
		Expect(batchCounts(report)["batches_completed"]).To(Equal(float64(6)))
	}, Entry("two concurrent requests", 2), Entry("four concurrent requests", 4))
	// per docs/adr/0112-bounded-parallel-review.md:42
	// per docs/adr/0112-bounded-parallel-review.md:48
	DescribeTable("rejects invalid concurrency and batch deadlines before paying for a request", func(setting string) {
		root, cwd := fixture()
		parallelReviewInput(cwd, 1)
		service := batchServer(cwd)
		result := batchRun(root, cwd, filepath.Join(cwd, "invalid"), service, nil, []string{setting})
		Expect(result.status).To(Equal(2), "%+v", result)
		Expect(service.observed()).To(BeEmpty())
	}, Entry("zero workers", "REVIEW_PARALLELISM=0"), Entry("above four workers", "REVIEW_PARALLELISM=5"), Entry("fractional workers", "REVIEW_PARALLELISM=2.5"), Entry("zero batch deadline", "REVIEW_BATCH_TIMEOUT_SECONDS=0"), Entry("batch deadline above 1200 seconds", "REVIEW_BATCH_TIMEOUT_SECONDS=1201"), Entry("nonnumeric batch deadline", "REVIEW_BATCH_TIMEOUT_SECONDS=word"))
	// per docs/adr/0112-bounded-parallel-review.md:42
	It("preserves serial admission by default and with an explicit single worker", func() {
		for _, setting := range []string{"", "REVIEW_PARALLELISM=1"} {
			root, cwd := fixture()
			parallelReviewInput(cwd, 3)
			arrivals, release := make(chan int, 8), make(chan struct{})
			var once sync.Once
			service := reviewFixture(cwd, func(w http.ResponseWriter, r *http.Request, n int) {
				arrivals <- n
				select {
				case <-release:
					parallelReviewResponse(w, "serial")
				case <-r.Context().Done():
				}
			})
			DeferCleanup(func() { once.Do(func() { close(release) }) })
			environment := []string{}
			if setting != "" {
				environment = append(environment, setting)
			}
			process := parallelReviewStart(batchCommand(root, cwd, filepath.Join(cwd, "serial"), service, nil, environment))
			parallelReviewArrival(arrivals)
			Consistently(arrivals, 100*time.Millisecond).ShouldNot(Receive())
			once.Do(func() { close(release) })
			Expect(process.wait().status).To(BeZero())
			Expect(service.observed()).To(HaveLen(3))
		}
	})
	// per docs/adr/0112-bounded-parallel-review.md:54
	DescribeTable("atomically shares run-wide reservations among competing workers", func(kind string) {
		root, cwd := fixture()
		parallelReviewInput(cwd, 6)
		environment := []string{"REVIEW_PARALLELISM=4", "REVIEW_TIMEOUT_SECONDS=8"}
		switch kind {
		case "requests":
			environment = append(environment, "REVIEW_MAX_REQUESTS=2")
		case "tokens":
			environment = append(environment, "REVIEW_TOTAL_MAX_TOKENS=2048")
		case "input":
			calibration := batchServer(cwd)
			Expect(batchRun(root, cwd, filepath.Join(cwd, "calibration"), calibration, nil, nil).status).To(BeZero())
			bodies := calibration.observed()
			Expect(bodies).To(HaveLen(6))
			for _, body := range bodies {
				Expect(body.body).To(HaveLen(len(bodies[0].body)))
			}
			environment = append(environment, "REVIEW_TOTAL_INPUT_BYTES="+strconv.Itoa(2*len(bodies[0].body)))
		}
		arrivals, release := make(chan int, 16), make(chan struct{})
		var once sync.Once
		service := reviewFixture(cwd, func(w http.ResponseWriter, r *http.Request, n int) {
			arrivals <- n
			select {
			case <-release:
				parallelReviewResponse(w, "budgeted")
			case <-r.Context().Done():
			}
		})
		DeferCleanup(func() { once.Do(func() { close(release) }) })
		out := filepath.Join(cwd, "budgeted")
		process := parallelReviewStart(batchCommand(root, cwd, out, service, nil, environment))
		parallelReviewArrival(arrivals)
		parallelReviewArrival(arrivals)
		Consistently(arrivals, 100*time.Millisecond).ShouldNot(Receive())
		once.Do(func() { close(release) })
		Expect(process.wait().status).To(Equal(1))
		Expect(service.observed()).To(HaveLen(2))
		report := batchReadReport(out)
		Expect(report["status"]).To(Equal("partial"))
		Expect(batchCounts(report)["batches_completed"]).To(Equal(float64(2)))
		budget := batchBudget(report)
		Expect(budget["requests_used"]).To(Equal(float64(2)))
		Expect(budget["response_tokens_reserved"]).To(Equal(float64(2048)))
		var sent int
		for _, call := range service.observed() {
			sent += len(call.body)
		}
		Expect(budget["input_bytes_reserved"]).To(Equal(float64(sent)))
		Expect(budget["requests_used"]).To(BeNumerically("<=", budget["max_requests"]))
		Expect(budget["response_tokens_reserved"]).To(BeNumerically("<=", budget["max_response_tokens"]))
		Expect(budget["input_bytes_reserved"]).To(BeNumerically("<=", budget["max_input_bytes"]))
	}, Entry("request count", "requests"), Entry("aggregate response tokens", "tokens"), Entry("aggregate input bytes", "input"))
	// per docs/adr/0112-bounded-parallel-review.md:54
	It("reserves concurrent 429 retries against the same shared allowance", func() {
		root, cwd := fixture()
		parallelReviewInput(cwd, 2)
		arrivals, release := make(chan int, 8), make(chan struct{})
		var once sync.Once
		service := reviewFixture(cwd, func(w http.ResponseWriter, r *http.Request, n int) {
			if n <= 2 {
				arrivals <- n
				select {
				case <-release:
					w.Header().Set("Retry-After", "0")
					w.WriteHeader(http.StatusTooManyRequests)
				case <-r.Context().Done():
				}
				return
			}
			parallelReviewResponse(w, "retried")
		})
		DeferCleanup(func() { once.Do(func() { close(release) }) })
		out := filepath.Join(cwd, "retry")
		process := parallelReviewStart(batchCommand(root, cwd, out, service, nil, []string{"REVIEW_PARALLELISM=2", "REVIEW_HTTP_RETRIES=1", "REVIEW_MAX_REQUESTS=3", "REVIEW_TOTAL_MAX_TOKENS=3072"}))
		parallelReviewArrival(arrivals)
		parallelReviewArrival(arrivals)
		once.Do(func() { close(release) })
		Expect(process.wait().status).To(Equal(1))
		calls := service.observed()
		Expect(calls).To(HaveLen(3))
		Expect(calls[2].body).To(Or(Equal(calls[0].body), Equal(calls[1].body)))
		report := batchReadReport(out)
		Expect(batchCounts(report)["batches_completed"]).To(Equal(float64(1)))
		Expect(batchBudget(report)["requests_used"]).To(Equal(float64(3)))
		Expect(batchBudget(report)["response_tokens_reserved"]).To(Equal(float64(3072)))
	})
	// per docs/adr/0112-bounded-parallel-review.md:60
	It("publishes plan order when real responses complete in reverse order", func() {
		root, cwd := fixture()
		parallelReviewInput(cwd, 3)
		cache := filepath.Join(cwd, "cache")
		arrivals := make(chan int, 8)
		releases := []chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{})}
		var once [3]sync.Once
		var service *reviewService
		service = reviewFixture(cwd, func(w http.ResponseWriter, r *http.Request, n int) {
			index := parallelReviewIndex(service, n)
			arrivals <- index
			if index < 0 || index >= len(releases) {
				return
			}
			select {
			case <-releases[index]:
				parallelReviewResponse(w, "finding-"+strconv.Itoa(index))
			case <-r.Context().Done():
			}
		})
		DeferCleanup(func() {
			for i := range releases {
				once[i].Do(func() { close(releases[i]) })
			}
		})
		out := filepath.Join(cwd, "ordered")
		process := parallelReviewStart(batchCommand(root, cwd, out, service, []string{"--cache", cache}, []string{"REVIEW_PARALLELISM=3"}))
		var started []int
		for range 3 {
			started = append(started, parallelReviewArrival(arrivals))
		}
		Expect(started).To(ConsistOf(0, 1, 2))
		for i := 2; i >= 0; i-- {
			once[i].Do(func() { close(releases[i]) })
			Eventually(func() bool { return parallelReviewCached(cache, "finding-"+strconv.Itoa(i)) }, 2*time.Second).Should(BeTrue(), "observe client-completed authenticated cache publication before releasing the next response")
		}
		Expect(process.wait().status).To(BeZero())
		report := batchReadReport(out)
		for i, value := range report["batches"].([]any) {
			batch := value.(map[string]any)
			Expect(batch["files"]).To(Equal([]any{fmt.Sprintf("src/file-%03d.go", i)}))
			Expect(batch["findings"]).To(Equal("finding-" + strconv.Itoa(i) + "\n"))
		}
		markdown, err := os.ReadFile(filepath.Join(out, "review.md"))
		Expect(err).NotTo(HaveOccurred())
		for i := range 3 {
			Expect(string(markdown)).To(ContainSubstring("finding-" + strconv.Itoa(i)))
		}
		Expect(strings.Index(string(markdown), "finding-0")).To(BeNumerically("<", strings.Index(string(markdown), "finding-1")))
		Expect(strings.Index(string(markdown), "finding-1")).To(BeNumerically("<", strings.Index(string(markdown), "finding-2")))
		Expect(report["usage"].(map[string]any)["cost"]).To(Equal(0.75))
	})
	// per docs/adr/0112-bounded-parallel-review.md:48
	DescribeTable("cancels an actual stalled HTTP request at the smaller batch or global deadline", func(batchSeconds, totalSeconds string) {
		root, cwd := fixture()
		parallelReviewInput(cwd, 1)
		started, canceled := make(chan int, 1), make(chan int, 1)
		service := reviewFixture(cwd, func(w http.ResponseWriter, r *http.Request, n int) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, ": PRIVATE_PARALLEL_REASONING\n\n")
			w.(http.Flusher).Flush()
			started <- n
			<-r.Context().Done()
			canceled <- n
		})
		out := filepath.Join(cwd, "timeout")
		process := parallelReviewStart(batchCommand(root, cwd, out, service, nil, []string{"REVIEW_PARALLELISM=2", "REVIEW_BATCH_TIMEOUT_SECONDS=" + batchSeconds, "REVIEW_TIMEOUT_SECONDS=" + totalSeconds, "REVIEW_HTTP_RETRIES=1"}))
		parallelReviewArrival(started)
		Eventually(canceled, 3*time.Second).Should(Receive(), "deadline must cancel the actual request, not just abandon a worker")
		result := process.wait()
		Expect(result.status).To(Equal(1))
		Expect(result.stdout + result.stderr).NotTo(ContainSubstring("PRIVATE_PARALLEL_REASONING"))
		Expect(service.observed()).To(HaveLen(1), "an ambiguous stalled generation must not retry")
		Expect(batchReadReport(out)["status"]).To(Equal("failed"))
	}, Entry("per-batch deadline", "1", "6"), Entry("remaining global deadline", "6", "1"))
	// per docs/adr/0112-bounded-parallel-review.md:48
	It("includes Retry-After backoff in the batch deadline", func() {
		root, cwd := fixture()
		parallelReviewInput(cwd, 1)
		started := make(chan int, 1)
		service := reviewFixture(cwd, func(w http.ResponseWriter, _ *http.Request, n int) {
			started <- n
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
		})
		out := filepath.Join(cwd, "backoff")
		process := parallelReviewStart(batchCommand(root, cwd, out, service, nil, []string{"REVIEW_PARALLELISM=2", "REVIEW_BATCH_TIMEOUT_SECONDS=1", "REVIEW_TIMEOUT_SECONDS=6", "REVIEW_HTTP_RETRIES=1"}))
		parallelReviewArrival(started)
		Eventually(process.finished, 3*time.Second).Should(Receive())
		process.reaped = true
		Expect(process.command.ProcessState.ExitCode()).To(Equal(1))
		Expect(service.observed()).To(HaveLen(1))
		Expect(batchReadReport(out)["status"]).To(Equal("failed"))
	})
	// per docs/adr/0112-bounded-parallel-review.md:60
	// per docs/adr/0112-bounded-parallel-review.md:66
	// per docs/adr/0112-bounded-parallel-review.md:71
	It("stops new paid admission after failure and retains an already admitted healthy sibling", func() {
		root, cwd := fixture()
		parallelReviewInput(cwd, 4)
		arrivals, release := make(chan int, 8), make(chan struct{})
		failure := make(chan struct{})
		var once sync.Once
		var failureOnce sync.Once
		var replay atomic.Bool
		var service *reviewService
		service = reviewFixture(cwd, func(w http.ResponseWriter, r *http.Request, n int) {
			index := parallelReviewIndex(service, n)
			if replay.Load() {
				parallelReviewResponse(w, "resumed-"+strconv.Itoa(index))
				return
			}
			arrivals <- index
			if index == 0 {
				select {
				case <-failure:
					_, _ = io.WriteString(w, "data: {\"error\":{\"message\":\"PRIVATE_PARALLEL_PROVIDER\"}}\n\n")
				case <-r.Context().Done():
				}
				return
			}
			select {
			case <-release:
				parallelReviewResponse(w, "retained-sibling PRIVATE_BATCH_KEY")
			case <-r.Context().Done():
			}
		})
		DeferCleanup(func() { once.Do(func() { close(release) }) })
		DeferCleanup(func() { failureOnce.Do(func() { close(failure) }) })
		cache, out := filepath.Join(cwd, "cache"), filepath.Join(cwd, "failed")
		environment := []string{"REVIEW_PARALLELISM=2"}
		process := parallelReviewStart(batchCommand(root, cwd, out, service, []string{"--cache", cache}, environment))
		Expect([]int{parallelReviewArrival(arrivals), parallelReviewArrival(arrivals)}).To(ConsistOf(0, 1))
		failureOnce.Do(func() { close(failure) })
		Eventually(func() string { return process.stderr.String() }, 2*time.Second).Should(ContainSubstring("status=failed"), "observe client-processed failure before releasing its admitted sibling")
		once.Do(func() { close(release) })
		result := process.wait()
		Expect(result.status).To(Equal(1))
		Expect(result.stdout + result.stderr).NotTo(ContainSubstring("PRIVATE_PARALLEL_PROVIDER"))
		Expect(service.observed()).To(HaveLen(2))
		report := batchReadReport(out)
		Expect(report["status"]).To(Equal("partial"))
		Expect(report["usage"]).To(BeNil())
		Expect(batchCounts(report)["batches_completed"]).To(Equal(float64(1)))
		Expect(parallelReviewCached(cache, "retained-sibling [REDACTED]")).To(BeTrue())
		Expect(parallelReviewCached(cache, "PRIVATE_BATCH_KEY")).To(BeFalse())
		replay.Store(true)
		resumed := filepath.Join(cwd, "resumed")
		Expect(batchRun(root, cwd, resumed, service, []string{"--cache", cache}, environment).status).To(BeZero())
		Expect(service.observed()).To(HaveLen(5))
		Expect(batchCounts(batchReadReport(resumed))["batches_cached"]).To(Equal(float64(1)))
	})
	// per docs/adr/0112-bounded-parallel-review.md:48
	// per docs/adr/0112-bounded-parallel-review.md:60
	It("cancels concurrent requests and resumes only authenticated completed coverage", func() {
		root, cwd := fixture()
		parallelReviewInput(cwd, 4)
		cache, out := filepath.Join(cwd, "cache"), filepath.Join(cwd, "canceled")
		held, canceled := make(chan int, 8), make(chan int, 8)
		var replay atomic.Bool
		var service *reviewService
		service = reviewFixture(cwd, func(w http.ResponseWriter, r *http.Request, n int) {
			index := parallelReviewIndex(service, n)
			if replay.Load() || index == 0 {
				parallelReviewResponse(w, "completed-"+strconv.Itoa(index)+" "+strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, ": PRIVATE_PARALLEL_REASONING\n\n")
			w.(http.Flusher).Flush()
			held <- index
			<-r.Context().Done()
			canceled <- index
		})
		environment := []string{"REVIEW_PARALLELISM=2", "REVIEW_TIMEOUT_SECONDS=8"}
		process := parallelReviewStart(batchCommand(root, cwd, out, service, []string{"--cache", cache}, environment))
		Eventually(func() bool { return parallelReviewCached(cache, "completed-0 [REDACTED]") }, 2*time.Second).Should(BeTrue())
		Expect([]int{parallelReviewArrival(held), parallelReviewArrival(held)}).To(ConsistOf(1, 2))
		Expect(process.command.Process.Signal(syscall.SIGTERM)).To(Succeed())
		Expect(process.wait().status).To(Equal(1))
		Eventually(canceled, 2*time.Second).Should(Receive())
		Eventually(canceled, 2*time.Second).Should(Receive())
		canceledReport := batchReadReport(out)
		Expect(canceledReport["status"]).To(Equal("partial"))
		Expect(canceledReport["usage"]).To(BeNil())
		Expect(batchCounts(canceledReport)["batches_completed"]).To(Equal(float64(1)), "cancellation retains completed partial coverage")
		Expect(service.observed()).To(HaveLen(3))
		replay.Store(true)
		resumed := filepath.Join(cwd, "resumed")
		Expect(batchRun(root, cwd, resumed, service, []string{"--cache", cache}, environment).status).To(BeZero())
		Expect(service.observed()).To(HaveLen(6))
		Expect(batchCounts(batchReadReport(resumed))["batches_cached"]).To(Equal(float64(1)))
		foreign := filepath.Join(cwd, "different-key")
		Expect(batchRun(root, cwd, foreign, service, []string{"--cache", cache}, append(environment, "REVIEW_API_KEY=OTHER_PRIVATE_KEY")).status).To(BeZero())
		Expect(service.observed()).To(HaveLen(10), "a different authentication key cannot trust the previous completed entries")
		Expect(batchCounts(batchReadReport(foreign))["batches_cached"]).To(Equal(float64(0)))
		Expect(parallelReviewCached(cache, "PRIVATE_BATCH_KEY")).To(BeFalse())
		Expect(parallelReviewCached(cache, "OTHER_PRIVATE_KEY")).To(BeFalse())
	})
	// per docs/adr/0112-bounded-parallel-review.md:48
	// per docs/adr/0112-bounded-parallel-review.md:60
	// per docs/adr/0112-bounded-parallel-review.md:71
	It("refuses overall success when its real parent is canceled at the final completion acknowledgement", func() {
		root, cwd := fixture()
		parallelReviewInput(cwd, 1)
		service := batchServer(cwd)
		repository, err := filepath.Abs("..")
		Expect(err).NotTo(HaveOccurred())
		source, overlay, driver := filepath.Join(cwd, "entry.go"), filepath.Join(cwd, "entry-overlay.json"), filepath.Join(cwd, "parent-cancel-driver")
		writeFixture(source, []byte(parallelReviewCancelDriver), 0600)
		data, err := json.Marshal(map[string]map[string]string{"Replace": {filepath.Join(repository, "cmd/factory/main.go"): source}})
		Expect(err).NotTo(HaveOccurred())
		writeFixture(overlay, data, 0600)
		build := imageProcess(repository, nil, "go", "build", "-race", "-overlay", overlay, "-o", driver, "./cmd/factory")
		Expect(build.status).To(BeZero(), "%+v", build)
		cache, out := filepath.Join(cwd, "cache"), filepath.Join(cwd, "final-canceled")
		environment := []string{"REVIEW_PARALLELISM=2"}
		command := batchCommand(root, cwd, out, service, []string{"--cache", cache}, environment)
		command.Path, command.Args[0] = driver, driver
		result := parallelReviewStart(command).wait()
		Expect(result.stderr).To(ContainSubstring("owned parent cancellation observed"))
		Expect(result.status).To(Equal(1))
		Expect(service.observed()).To(HaveLen(1))
		report := batchReadReport(out)
		Expect(report["status"]).To(Equal("failed"))
		Expect(report["error"]).To(ContainSubstring("canceled"))
		Expect(batchCounts(report)["batches_completed"]).To(Equal(float64(1)), "cancellation changes overall disposition without discarding completed coverage")
		Expect(parallelReviewCached(cache, "Safe findings")).To(BeTrue())
		resumed := filepath.Join(cwd, "resumed")
		Expect(batchRun(root, cwd, resumed, service, []string{"--cache", cache}, environment).status).To(BeZero())
		Expect(service.observed()).To(HaveLen(1), "completed authenticated coverage remains reusable after cancellation")
		Expect(batchCounts(batchReadReport(resumed))["batches_cached"]).To(Equal(float64(1)))
	})
})
