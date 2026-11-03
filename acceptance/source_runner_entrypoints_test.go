package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Exercise the shipped source bytes rather than a copied historical executor.
// per docs/adr/0133-go-source-evaluation-runners.md:49
func sourceRunnerAdapter(root, harness string) string {
	GinkgoHelper()
	path := filepath.Join("eval", "runners", harness+".sh")
	body, err := os.ReadFile(filepath.Join("..", path))
	Expect(err).NotTo(HaveOccurred())
	program := filepath.Join(root, path)
	writeFixture(program, body, 0755)
	return program
}

// Explicit shell invocation is part of the same public adapter contract.
// per docs/adr/0133-go-source-evaluation-runners.md:22
func sourceRunnerInvoke(cwd string, environment map[string]string, input, program string, shell bool, args ...string) cliResult {
	GinkgoHelper()
	if shell {
		return handoffNativeExactProcess(cwd, environment, input, "/bin/sh", append([]string{program}, args...)...)
	}
	return handoffNativeExactProcess(cwd, environment, input, program, args...)
}

// A recorded fixture group is authority; a later PID lookup cannot replace it.
// per docs/adr/0133-go-source-evaluation-runners.md:86
func sourceRunnerStopObservedGroup(pid, expectedGroup int) (bool, error) {
	if pid <= 1 || expectedGroup <= 1 || expectedGroup == syscall.Getpgrp() {
		return false, errors.New("fixture process group was not safely admitted")
	}
	actual, err := syscall.Getpgid(pid)
	if errors.Is(err, syscall.ESRCH) && errors.Is(syscall.Kill(-expectedGroup, 0), syscall.ESRCH) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("observe recorded fixture process group: %w", err)
	}
	if actual != expectedGroup {
		return false, fmt.Errorf("fixture PID %d now belongs to group %d instead of recorded group %d", pid, actual, expectedGroup)
	}
	if err := syscall.Kill(-expectedGroup, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return false, fmt.Errorf("stop recorded fixture process group: %w", err)
	}
	return false, nil
}

// A real public evaluator must settle the independently grouped native harness
// before returning; fixture cleanup runs only after that observation.
// per docs/adr/0133-go-source-evaluation-runners.md:78
// per docs/adr/0133-go-source-evaluation-runners.md:83
func sourceRunnerCancellation(mode string, explicit bool, external string) {
	GinkgoHelper()
	root, _ := fixtureDirectories()
	base := filepath.Dir(root)
	native, nestedCalls := filepath.Join(base, "native evaluator"), filepath.Join(base, "nested owner calls")
	writeFixture(native, candidateBinary, 0700)
	writeFixture(filepath.Join(root, "factory"), []byte("#!/bin/sh\nif [ \"$1\" = eval ] && [ \"$2\" = run ]; then printf '%s\\000' \"$@\" >> \"$SOURCE_RUNNER_NESTED_CALLS\"; fi\nexec \"$SOURCE_RUNNER_NATIVE\" \"$@\"\n"), 0700)
	golden, runner := sourceGoldenAdapter(root), sourceRunnerAdapter(root, "claude")
	writeFixture(filepath.Join(root, "factory.yaml"), []byte("review_lane: off\n"), 0600)
	writeFixture(filepath.Join(root, "AGENTS.md"), []byte("Private cancellation fixture.\n"), 0600)
	orchestrationTask(root, "reference-answer")
	oracleCalled := filepath.Join(base, "ORACLE_CALLED")
	writeFixture(filepath.Join(root, "eval/golden-tasks/reference-answer/verify.sh"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_RUNNER_ORACLE_CALLED\"\nexit 0\n"), 0700)
	tools, ready := filepath.Join(base, "tools"), filepath.Join(base, "harness started")
	sourceOwner, sourceBefore := "", map[string]string(nil)
	sourceCompiler := external == "source-compiler" || external == "source-compiler-slow"
	delayedPolls := filepath.Join(base, "delayed source polls")
	harnessCalled := filepath.Join(base, "HARNESS_CALLED")
	body := "#!/bin/sh\nprintf 'FIXED\\n' > answer.txt\nprintf '%s\\t%s\\n' \"$$\" \"$PWD\" >> \"$SOURCE_RUNNER_READY\"\nexec /bin/sleep 60\n"
	switch {
	case sourceCompiler:
		sourceOwner = filepath.Join(base, "cold source owner")
		launcher, err := os.ReadFile("../factory")
		Expect(err).NotTo(HaveOccurred())
		if external == "source-compiler-slow" {
			// Inject scheduling overhead only in the private cleanup function.
			// Bash versions expose different BASH_COMMAND values in signal traps.
			// per docs/adr/0133-go-source-evaluation-runners.md:166
			anchor := []byte("source_stop_build() {\n")
			Expect(bytes.Count(launcher, anchor)).To(Equal(1))
			start := bytes.Index(launcher, anchor)
			end := bytes.Index(launcher[start:], []byte("\n}\n"))
			Expect(end).To(BeNumerically(">", 0))
			end += start
			var delayed strings.Builder
			polls := 0
			for _, line := range strings.SplitAfter(string(launcher[start:end]), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "/bin/sleep ") {
					Expect(strings.Fields(line)).To(HaveLen(2), "the fixture must identify a standalone cleanup sleep")
					polls++
					delayed.WriteString("        printf 'delay\\n' >> \"$SOURCE_RUNNER_DELAYED_POLLS\"\n        /bin/sleep 0.08\n")
				}
				delayed.WriteString(line)
			}
			Expect(polls).To(Equal(2), "both cleanup polling sites must receive the scheduling delay")
			launcher = []byte(string(launcher[:start]) + delayed.String() + string(launcher[end:]))
		}
		writeFixture(filepath.Join(sourceOwner, "factory"), launcher, 0700)
		writeFixture(filepath.Join(sourceOwner, "factory-go"), []byte("untouched previous developer output\n"), 0600)
		runner = sourceRunnerAdapter(sourceOwner, "claude")
		writeFixture(filepath.Join(tools, "go"), []byte("#!/bin/sh\ntrap '' TERM\nprintf '%s\\t%s\\n' \"$$\" \"$PWD\" >> \"$SOURCE_RUNNER_READY\"\nexec /bin/sleep 60\n"), 0700)
		writeFixture(filepath.Join(tools, "claude"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_RUNNER_HARNESS_CALLED\"\nexit 95\n"), 0700)
		sourceBefore = installationSnapshot(sourceOwner)
	case external != "":
		body = "#!/bin/sh\ntrap '' TERM\nprintf 'FIXED\\n' > \"$1/answer.txt\"\nprintf '%s\\t%s\\n' \"$$\" \"$1\" >> \"$SOURCE_RUNNER_READY\"\nexec /bin/sleep 60\n"
		if external == "unacknowledged" {
			// Preserve the original non-trapping timeout fixture verbatim as a
			// distinct ownership-refusal criterion. Its child receipt is one PID.
			// per docs/adr/0133-go-source-evaluation-runners.md:88
			body = "#!/bin/sh\nprintf 'FIXED\\n' > \"$1/answer.txt\"\n/bin/sleep 60 &\nprintf '%s\\n' \"$!\" > \"$CHILD_PID_FILE\"\nwait\n"
		}
		runner = filepath.Join(root, "external runner")
		writeFixture(runner, []byte(body), 0700)
	default:
		writeFixture(filepath.Join(tools, "claude"), []byte(body), 0700)
	}
	environment := map[string]string{
		"PATH": tools + ":/usr/bin:/bin", "HOME": root, "BASH_ENV": "", "ENV": "", "GORACE": "atexit_sleep_ms=0", "TMPDIR": base,
		"FACTORY_CONFIG": filepath.Join(root, "factory.yaml"), "SOURCE_RUNNER_READY": ready,
		"CHILD_PID_FILE": ready, "SOURCE_RUNNER_NATIVE": native, "SOURCE_RUNNER_NESTED_CALLS": nestedCalls, "SOURCE_RUNNER_ORACLE_CALLED": oracleCalled,
		"SOURCE_RUNNER_HARNESS_CALLED": harnessCalled,
		"GIT_CONFIG_NOSYSTEM":          "1", "GIT_CONFIG_GLOBAL": os.DevNull,
	}
	if external == "source-compiler-slow" {
		environment["SOURCE_RUNNER_DELAYED_POLLS"] = delayedPolls
	}
	timeoutSeconds := 1
	if sourceCompiler {
		// Leave startup headroom for the cold source controller.
		// The nested supervisor's cleanup grace remains unchanged.
		// per docs/adr/0133-go-source-evaluation-runners.md:166
		timeoutSeconds = 3
	}
	args := []string{"--harness", "claude", "--runs", "1", "--timeout", strconv.Itoa(timeoutSeconds)}
	if mode == "parent-cancellation" {
		args[len(args)-1] = "60"
	}
	if explicit {
		args = append(args, "--runner", runner)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	DeferCleanup(cancel)
	command := handoffCommand(ctx, root, environment, "", golden, args...)
	command.Env = nil
	for key, value := range environment {
		command.Env = append(command.Env, key+"="+value)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	Expect(command.Start()).To(Succeed())
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	joined, childSettled, pid, group, work := false, false, 0, 0, ""
	readReady := func() (int, string) {
		data, err := os.ReadFile(ready)
		if err != nil {
			return 0, ""
		}
		fields := strings.Split(strings.TrimSuffix(string(data), "\n"), "\t")
		if external == "unacknowledged" && len(fields) == 1 {
			fields = append(fields, "")
		}
		if len(fields) != 2 {
			return 0, ""
		}
		actual, err := strconv.Atoi(fields[0])
		if err != nil || actual <= 1 {
			return 0, ""
		}
		return actual, fields[1]
	}
	DeferCleanup(func() {
		cleanObservedChild := func() {
			if childSettled {
				return
			}
			if pid == 0 {
				pid, _ = readReady()
			}
			settled, err := sourceRunnerStopObservedGroup(pid, group)
			childSettled = settled
			if err != nil {
				holdFixtureCleanup(base, "recorded evaluation fixture process identity is unconfirmed")
				AddReportEntry("source-runner.fixture-identity-uncertain", map[string]any{"pid": pid, "recorded_group": group, "error": err.Error()}, ReportEntryVisibilityNever)
			}
		}
		cleanObservedChild()
		if !joined {
			_ = command.Process.Signal(syscall.SIGTERM)
			select {
			case <-finished:
				joined = true
			case <-time.After(3 * time.Second):
				_ = command.Process.Kill()
				select {
				case <-finished:
					joined = true
				case <-time.After(2 * time.Second):
				}
			}
		}
		// Read again after joining: a failing readiness assertion can race a late
		// child handshake while the public parent is receiving its cleanup signal.
		cleanObservedChild()
		for attempt := 0; !childSettled && pid > 1 && group > 1 && attempt < 100; attempt++ {
			childSettled = errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) && errors.Is(syscall.Kill(-group, 0), syscall.ESRCH)
			if childSettled {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if !joined || !childSettled {
			holdFixtureCleanup(base, "evaluation process did not join after owned cancellation fixture cleanup")
		}
		if joined {
			AddReportEntry("source-runner.fixture-cleanup", map[string]any{"external": external, "pid": pid, "group": group, "status": command.ProcessState.ExitCode(), "stdout": stdout.String(), "stderr": stderr.String()}, ReportEntryVisibilityNever)
		}
	})
	Eventually(func() bool {
		pid, work = readReady()
		return pid > 1 && syscall.Kill(pid, 0) == nil
	}, 5*time.Second, 10*time.Millisecond).Should(BeTrue(), "observe the actual fake CLI before sending cancellation")
	var err error
	group, err = syscall.Getpgid(pid)
	Expect(err).NotTo(HaveOccurred())
	if external != "unacknowledged" {
		Expect(group).To(Equal(pid), "the actual harness must own its independently supervised process group")
	}
	Expect(group).NotTo(Equal(syscall.Getpgrp()))
	Expect(pid).NotTo(Equal(command.Process.Pid))
	if external != "unacknowledged" {
		Expect(work).NotTo(Equal(filepath.Join(root, "eval/golden-tasks/reference-answer")))
	}
	if mode == "parent-cancellation" {
		Expect(command.Process.Signal(syscall.SIGTERM)).To(Succeed())
	}
	var returned error
	Eventually(finished, 8*time.Second).Should(Receive(&returned), "cancellation remains bounded, including a TERM-ignoring runner")
	joined = true
	Expect(ctx.Err()).NotTo(HaveOccurred())
	if external == "source-compiler-slow" {
		polls, err := os.ReadFile(delayedPolls)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(polls)).To(ContainSubstring("delay\n"), "the controlled cleanup scheduling delay must actually execute")
		AddReportEntry("source-runner.delayed-source-polls", map[string]any{"delay_ms": 80, "polls": bytes.Count(polls, []byte("delay\n"))}, ReportEntryVisibilityNever)
	}
	pidAbsent := errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
	groupAbsent := errors.Is(syscall.Kill(-group, 0), syscall.ESRCH)
	childSettled = pidAbsent && groupAbsent
	AddReportEntry("source-runner.nested-shutdown", map[string]any{"mode": mode, "explicit": explicit, "external": external, "parent_pid": command.Process.Pid, "harness_pid": pid, "harness_group": group, "pid_absent_before_cleanup": pidAbsent, "group_absent_before_cleanup": groupAbsent, "status": command.ProcessState.ExitCode(), "stdout": stdout.String(), "stderr": stderr.String()}, ReportEntryVisibilityNever)
	Expect(pidAbsent).To(BeTrue(), "the evaluator returned while its harness PID %d was still present", pid)
	Expect(groupAbsent).To(BeTrue(), "the evaluator returned while its harness group %d was still present", group)
	if external != "unacknowledged" {
		Expect(os.ReadFile(ready)).To(Equal([]byte(strconv.Itoa(pid)+"\t"+work+"\n")), "cancellation must never retry the selected runner")
	}
	if external == "" {
		Expect(os.ReadFile(nestedCalls)).To(Equal([]byte("eval\x00run\x00claude\x00"+work+"\x00")), "both named and explicit selection must invoke the converted adapter exactly once")
	} else {
		_, err = os.Lstat(nestedCalls)
		Expect(os.IsNotExist(err)).To(BeTrue(), "an external runner must not become a built-in harness invocation")
	}
	_, err = os.Lstat(oracleCalled)
	Expect(os.IsNotExist(err)).To(BeTrue(), "a canceled or uncertain run must never invoke its oracle")
	if sourceOwner != "" {
		Expect(work).To(Equal(sourceOwner), "the ready process must be the compiler owned by the actual private source launcher")
		Expect(installationSnapshot(sourceOwner)).To(Equal(sourceBefore), "canceled compilation must preserve developer output and remove only its private build files")
		_, err = os.Lstat(harnessCalled)
		Expect(os.IsNotExist(err)).To(BeTrue(), "canceled compilation cannot reach the harness")
	}
	Expect(stdout.String()).NotTo(ContainSubstring("1/1 passed"))
	resultPath := filepath.Join(root, "eval/results/claude-current.json")
	if mode == "parent-cancellation" || (external != "" && !sourceCompiler) {
		Expect(returned).To(HaveOccurred())
		Expect(command.ProcessState.ExitCode()).NotTo(Equal(0))
		_, err = os.Lstat(resultPath)
		Expect(os.IsNotExist(err)).To(BeTrue(), "cancellation or uncertain ownership cannot publish a completed evaluation")
		if external != "" {
			// per docs/migration/COMPATIBILITY_INVENTORY.md:108
			Expect(command.ProcessState.ExitCode()).To(Equal(1))
			Expect(stderr.String()).To(ContainSubstring("golden-task-eval: evaluation could not complete"))
			if external == "ignores-term" {
				// per docs/adr/0133-go-source-evaluation-runners.md:98
				info, err := os.Stat(work)
				Expect(err).NotTo(HaveOccurred(), "uncertain nested ownership must retain its actual private work directory")
				Expect(info.IsDir()).To(BeTrue())
				Expect(stderr.String()).To(ContainSubstring(strconv.Quote(work)), "the refusal must safely quote retained work for recovery")
			}
		}
	} else {
		Expect(returned).NotTo(HaveOccurred(), "%s", stderr.String())
		Expect(stdout.String()).To(ContainSubstring("hit the " + strconv.Itoa(timeoutSeconds) + "s cap"))
		data, err := os.ReadFile(resultPath)
		Expect(err).NotTo(HaveOccurred())
		var result orchestrationResults
		Expect(json.Unmarshal(data, &result)).To(Succeed())
		Expect(result.Tasks).To(HaveLen(1))
		Expect(result.Tasks[0].Passes).To(BeZero())
		Expect(result.Tasks[0].Score).To(BeZero())
	}
}

const sourceRunnerDetachedHandshake = "--factory-source-runner-detached-fixture"

type sourceRunnerDetachedReceipt struct {
	PID, Group   int
	Work         string
	TokenPresent bool
}

// Re-execute the already captured Go evaluator: the child deliberately retains
// native output pipes in a separate session after its fake CLI parent exits.
// per docs/adr/0133-go-source-evaluation-runners.md:133
func init() {
	if len(os.Args) < 3 || os.Args[1] != sourceRunnerDetachedHandshake {
		return
	}
	path, base := os.Getenv("SOURCE_RUNNER_DETACHED_READY"), os.Getenv("TMPDIR")
	work, err := os.Getwd()
	relative, relativeErr := filepath.Rel(base, work)
	if err != nil || relativeErr != nil || !filepath.IsAbs(base) || !filepath.IsAbs(path) || filepath.Dir(path) != filepath.Clean(base) ||
		relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		os.Exit(95)
	}
	if os.Args[2] == "hold" {
		_, present := os.LookupEnv("FACTORY_EVAL_OWNERSHIP_TOKEN")
		receipt := sourceRunnerDetachedReceipt{os.Getpid(), syscall.Getpgrp(), work, present}
		data, encodeErr := json.Marshal(receipt)
		if encodeErr != nil || os.WriteFile(path+".pending", data, 0600) != nil || os.Rename(path+".pending", path) != nil { // #nosec G703 -- absolute readiness destination is a direct child of the explicitly supplied private TMPDIR, with cwd containment checked before any mutation; no production input enables this mode.
			os.Exit(96)
		}
		ready := os.NewFile(3, "detached child ready")
		if _, err := ready.Write([]byte{1}); err != nil {
			os.Exit(97)
		}
		_ = ready.Close()
		time.Sleep(60 * time.Second)
		os.Exit(0)
	}
	overflow := os.Args[2] == "overflow"
	if os.Args[2] != "cli" && !overflow {
		os.Exit(98)
	}
	if overflow {
		signal.Ignore(syscall.SIGTERM, syscall.SIGPIPE)
	}
	if err := os.WriteFile("answer.txt", []byte("FIXED\n"), 0600); err != nil {
		os.Exit(99)
	}
	ready, readyWriter, err := os.Pipe()
	if err != nil {
		os.Exit(100)
	}
	executable, err := os.Executable()
	if err != nil {
		os.Exit(101)
	}
	child := exec.Command(executable, sourceRunnerDetachedHandshake, "hold") // #nosec G204 -- current evaluator image and fixed private fixture mode; no shell command is constructed.
	child.Env = os.Environ()
	if !overflow {
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
	}
	child.ExtraFiles = []*os.File{readyWriter}
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		os.Exit(101)
	}
	_ = readyWriter.Close()
	observed := make(chan bool, 1)
	go func() {
		var value [1]byte
		_, err := ready.Read(value[:])
		observed <- err == nil && value[0] == 1
	}()
	select {
	case ok := <-observed:
		if ok {
			if overflow {
				payload := []byte("\nfactory-eval-result-v1 " + os.Getenv("FACTORY_EVAL_OWNERSHIP_TOKEN") + " ownership-unconfirmed\n")
				if os.Getenv("SOURCE_RUNNER_CAPTURE_OVERFLOW") == "1" {
					payload = append(bytes.Repeat([]byte{'x'}, 16<<20), payload...)
				}
				written, writeErr := os.Stderr.Write(payload)
				completed := strconv.Itoa(written) + "/" + strconv.Itoa(len(payload)) + "/" + strconv.FormatBool(writeErr == nil) + "\n"
				if os.WriteFile(path+".written", []byte(completed), 0600) != nil { // #nosec G703 -- readiness sibling is within the same explicitly validated private TMPDIR; only this test fixture produces the completion witness.
					os.Exit(103)
				}
				os.Exit(125)
			}
			os.Exit(0)
		}
	case <-time.After(5 * time.Second):
	}
	_ = syscall.Kill(-child.Process.Pid, syscall.SIGKILL)
	_, _ = child.Process.Wait()
	os.Exit(102)
}

// Own every observed detached group even when the acceptance assertion is RED.
// per docs/adr/0133-go-source-evaluation-runners.md:134
func sourceRunnerDetachedCleanup(base, ready string) {
	GinkgoHelper()
	DeferCleanup(func() {
		var receipt sourceRunnerDetachedReceipt
		data, err := os.ReadFile(ready)
		if err != nil || json.Unmarshal(data, &receipt) != nil || receipt.PID <= 1 || receipt.Group != receipt.PID || receipt.Group == syscall.Getpgrp() {
			holdFixtureCleanup(base, "detached evaluation fixture receipt is unavailable or invalid; process custody is unknown")
			return
		}
		settled, err := sourceRunnerStopObservedGroup(receipt.PID, receipt.Group)
		if err != nil {
			holdFixtureCleanup(base, "detached evaluation fixture process identity is unconfirmed")
			AddReportEntry("source-runner.detached-identity-uncertain", map[string]any{"pid": receipt.PID, "recorded_group": receipt.Group, "error": err.Error()}, ReportEntryVisibilityNever)
			return
		}
		for attempt := 0; !settled && attempt < 150; attempt++ {
			settled = errors.Is(syscall.Kill(receipt.PID, 0), syscall.ESRCH) && errors.Is(syscall.Kill(-receipt.Group, 0), syscall.ESRCH)
			if settled {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if !settled {
			holdFixtureCleanup(base, "detached evaluation fixture group remains present after owned cleanup")
		}
	})
}

// A selected external runner may report a result only with this attempt's exact
// receipt; ordinary failures and unrelated stderr retain their oracle contract.
// per docs/adr/0133-go-source-evaluation-runners.md:116
// per docs/adr/0133-go-source-evaluation-runners.md:124
func sourceRunnerReceiptCase(mode string) {
	GinkgoHelper()
	root, _ := fixtureDirectories()
	base := filepath.Dir(root)
	writeFixture(filepath.Join(root, "factory"), candidateBinary, 0700)
	golden := sourceGoldenAdapter(root)
	writeFixture(filepath.Join(root, "factory.yaml"), []byte("review_lane: off\n"), 0600)
	writeFixture(filepath.Join(root, "AGENTS.md"), []byte("Private receipt fixture.\n"), 0600)
	orchestrationTask(root, "reference-answer")
	calls, oracleCalled := filepath.Join(base, "attempt receipts"), filepath.Join(base, "ORACLE_CALLED")
	writeFixture(filepath.Join(root, "eval/golden-tasks/reference-answer/verify.sh"), []byte("#!/bin/sh\nprintf called >> \"$SOURCE_RUNNER_ORACLE_CALLED\"\nexit 0\n"), 0700)
	runner := filepath.Join(root, "literal custom runner")
	body := "#!/bin/sh\nprintf '%s\\t%s\\n' \"$FACTORY_EVAL_OWNERSHIP_TOKEN\" \"$1\" >> \"$SOURCE_RUNNER_ATTEMPTS\"\nprintf 'FIXED\\n' > \"$1/answer.txt\"\nprintf 'private child diagnostics\\n' >&2\ncase \"$SOURCE_RUNNER_RECEIPT_MODE\" in\n" +
		" failed) printf 'factory-eval-result-v1 %s failed\\n' \"$FACTORY_EVAL_OWNERSHIP_TOKEN\" >&2 ;;\n" +
		" ownership-unconfirmed) printf 'factory-eval-result-v1 %s ownership-unconfirmed\\n' \"$FACTORY_EVAL_OWNERSHIP_TOKEN\" >&2 ;;\n" +
		" both) printf 'factory-eval-result-v1 %s failed\\nfactory-eval-result-v1 %s ownership-unconfirmed\\n' \"$FACTORY_EVAL_OWNERSHIP_TOKEN\" \"$FACTORY_EVAL_OWNERSHIP_TOKEN\" >&2 ;;\n" +
		" both-reversed) printf 'factory-eval-result-v1 %s ownership-unconfirmed\\nfactory-eval-result-v1 %s failed\\n' \"$FACTORY_EVAL_OWNERSHIP_TOKEN\" \"$FACTORY_EVAL_OWNERSHIP_TOKEN\" >&2 ;;\n" +
		" foreign) printf 'factory-eval-result-v1 %s ownership-unconfirmed\\n' \"$SOURCE_RUNNER_STALE_TOKEN\" >&2 ;;\n" +
		" stale) if [ -f \"$SOURCE_RUNNER_PRIOR_TOKEN\" ]; then prior=$(/bin/cat \"$SOURCE_RUNNER_PRIOR_TOKEN\"); printf 'factory-eval-result-v1 %s ownership-unconfirmed\\n' \"$prior\" > \"$SOURCE_RUNNER_EMITTED\"; /bin/cat \"$SOURCE_RUNNER_EMITTED\" >&2; else printf '%s' \"$FACTORY_EVAL_OWNERSHIP_TOKEN\" > \"$SOURCE_RUNNER_PRIOR_TOKEN\"; fi ;;\n" +
		" prefix) printf 'prefix factory-eval-result-v1 %s ownership-unconfirmed\\n' \"$FACTORY_EVAL_OWNERSHIP_TOKEN\" >&2 ;;\n" +
		" suffix) printf 'factory-eval-result-v1 %s ownership-unconfirmed suffix\\n' \"$FACTORY_EVAL_OWNERSHIP_TOKEN\" >&2 ;;\n" +
		" partial) printf 'factory-eval-result-v1 %s ownership-unconfirmed' \"$FACTORY_EVAL_OWNERSHIP_TOKEN\" >&2 ;;\n" +
		" malformed-token) printf 'factory-eval-result-v1 %sx ownership-unconfirmed\\n' \"$FACTORY_EVAL_OWNERSHIP_TOKEN\" >&2 ;;\n" +
		" unknown-status) printf 'factory-eval-result-v1 %s success\\n' \"$FACTORY_EVAL_OWNERSHIP_TOKEN\" >&2 ;;\n" +
		" stdout) printf 'factory-eval-result-v1 %s ownership-unconfirmed\\n' \"$FACTORY_EVAL_OWNERSHIP_TOKEN\" ;;\nesac\nexit 125\n"
	writeFixture(runner, []byte(body), 0700)
	stale := strings.Repeat("b", 64)
	environment := map[string]string{
		"PATH": "/usr/bin:/bin", "HOME": root, "TMPDIR": base, "GORACE": "atexit_sleep_ms=0", "BASH_ENV": "", "ENV": "",
		"FACTORY_CONFIG": filepath.Join(root, "factory.yaml"), "FACTORY_EVAL_OWNERSHIP_TOKEN": stale,
		"SOURCE_RUNNER_ATTEMPTS": calls, "SOURCE_RUNNER_ORACLE_CALLED": oracleCalled, "SOURCE_RUNNER_STALE_TOKEN": stale, "SOURCE_RUNNER_RECEIPT_MODE": mode,
		"SOURCE_RUNNER_PRIOR_TOKEN": filepath.Join(base, "prior valid token"), "SOURCE_RUNNER_EMITTED": filepath.Join(base, "emitted stale receipt"),
		"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull,
	}
	observed := sourceRunnerInvoke(root, environment, "", golden, false, "--harness", "mock", "--runner", runner, "--runs", "2")
	raw, err := os.ReadFile(calls)
	Expect(err).NotTo(HaveOccurred())
	attempts := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	uncertain := mode == "ownership-unconfirmed" || mode == "both" || mode == "both-reversed"
	if uncertain {
		Expect(attempts).To(HaveLen(1), "fatal uncertainty must not retry or proceed to the next evaluation attempt")
	} else {
		Expect(attempts).To(HaveLen(2))
	}
	seen := map[string]bool{}
	for _, attempt := range attempts {
		fields := strings.Split(attempt, "\t")
		Expect(fields).To(HaveLen(2))
		token, work := fields[0], fields[1]
		Expect(token).To(MatchRegexp("^[0-9a-f]{64}$"))
		Expect(token).NotTo(Equal(stale), "the parent must replace caller-supplied tokens for every external attempt")
		Expect(seen[token]).To(BeFalse(), "each attempt requires a fresh private token")
		seen[token] = true
		Expect(observed.stdout).NotTo(ContainSubstring(token))
		Expect(observed.stderr).NotTo(ContainSubstring(token))
		Expect(filepath.IsAbs(work)).To(BeTrue())
		Expect(filepath.Dir(work)).To(Equal(base), "the recorded task must be an immediate child of the fixture-owned physical TMPDIR")
		Expect(filepath.Base(work)).To(HavePrefix("factory-golden-"))
		_, err = os.Stat(work) // #nosec G703 -- evaluator-created direct child of the test-owned physical TMPDIR; absolute parent and generated basename are checked immediately above, and the controlled runner only records its literal operand.
		if uncertain {
			Expect(err).NotTo(HaveOccurred())
			Expect(observed.stderr).To(ContainSubstring(strconv.Quote(work)))
		} else {
			Expect(os.IsNotExist(err)).To(BeTrue(), "confirmed work retains ordinary cleanup")
		}
	}
	Expect(observed.stdout).NotTo(ContainSubstring("factory-eval-result-v1"))
	Expect(observed.stderr).NotTo(ContainSubstring("factory-eval-result-v1"))
	Expect(observed.stderr).NotTo(ContainSubstring("private child diagnostics"))
	if mode == "stale" {
		prior := strings.Split(attempts[0], "\t")[0]
		Expect(os.ReadFile(environment["SOURCE_RUNNER_PRIOR_TOKEN"])).To(Equal([]byte(prior)))
		Expect(os.ReadFile(environment["SOURCE_RUNNER_EMITTED"])).To(Equal([]byte("factory-eval-result-v1 "+prior+" ownership-unconfirmed\n")), "the second attempt must actually replay the first attempt's valid receipt")
	}
	if uncertain {
		Expect(observed.status).To(Equal(1))
		Expect(observed.stderr).To(ContainSubstring("golden-task-eval: evaluation could not complete"))
		for _, forbidden := range []string{oracleCalled, filepath.Join(root, "eval/results/mock-current.json")} {
			_, err = os.Lstat(forbidden)
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
		return
	}
	Expect(observed.status).To(Equal(0), "%+v", observed)
	Expect(observed.stderr).To(BeEmpty())
	current := orchestrationReadResults(root, "current")
	Expect(current.Tasks).To(HaveLen(1))
	Expect(current.Tasks[0].Runs).To(Equal(2))
	if mode == "failed" {
		Expect(current.Tasks[0].Passes).To(BeZero())
		_, err = os.Lstat(oracleCalled)
		Expect(os.IsNotExist(err)).To(BeTrue(), "nested rejection must not execute the oracle")
	} else {
		Expect(current.Tasks[0].Passes).To(Equal(2), "a custom exit 125 or an unmatched receipt must retain ordinary oracle scoring")
		Expect(os.ReadFile(oracleCalled)).To(Equal([]byte("calledcalled")))
	}
}

var _ = Describe("Go source runtime entrypoints evaluation runner adapters", func() {
	// per docs/adr/0133-go-source-evaluation-runners.md:18
	// per docs/adr/0133-go-source-evaluation-runners.md:49
	DescribeTable("transports literal operands and channels to its physical owner", func(harness string) {
		operands := []string{"", "space and ' quote", "literal $(not-code)\nline", "--", "--model=not-an-option"}
		input := "original stdin\nliteral $(not-code)\x00tail\n"
		for _, shell := range []bool{false, true} {
			for _, alias := range []bool{false, true} {
				root, caller := fixtureDirectories()
				base := filepath.Dir(root)
				owner := filepath.Join(base, "physical owner ' $(not-code) \n")
				Expect(os.Rename(root, owner)).To(Succeed())
				physicalCaller := filepath.Join(base, "caller ' $(not-code) \n")
				Expect(os.Rename(caller, physicalCaller)).To(Succeed())
				program := sourceRunnerAdapter(owner, harness)
				writeFixture(filepath.Join(owner, "factory"), []byte("#!/bin/sh\nprintf '%s\\000' \"$PWD\" \"$SOURCE_ENTRY_LITERAL_ENV\" \"$FACTORY_EVAL_MODEL\" \"$@\" > \"$SOURCE_ENTRY_ARGV\"\n/bin/cat > \"$SOURCE_ENTRY_INPUT\"\nprintf 'owner stdout\\n'\nprintf 'owner stderr\\n' >&2\nexit 63\n"), 0755)
				record, capturedInput, marker := filepath.Join(base, "argv"), filepath.Join(base, "stdin"), filepath.Join(base, "DECOY_CALLED")
				foreign, tools := filepath.Join(base, "foreign owner"), filepath.Join(base, "tools")
				for _, decoy := range []string{filepath.Join(foreign, "factory"), filepath.Join(physicalCaller, "factory"), filepath.Join(tools, "factory"), filepath.Join(tools, harness)} {
					writeFixture(decoy, []byte("#!/bin/sh\nprintf called > \"$SOURCE_ENTRY_DECOY\"\nexit 64\n"), 0755)
				}
				environment := map[string]string{
					"PATH": tools + ":/usr/bin:/bin", "BASH_ENV": "", "ENV": "",
					"SOURCE_ENTRY_ARGV": record, "SOURCE_ENTRY_INPUT": capturedInput, "SOURCE_ENTRY_DECOY": marker,
					"SOURCE_ENTRY_LITERAL_ENV": "literal environment $(not-code)\nvalue\n",
					"FACTORY_EVAL_MODEL":       "literal model ' $(not-code)\nvalue\n",
					"FACTORY_RUNTIME_BINARY":   filepath.Join(foreign, "factory"), "FACTORY_HOME": foreign,
				}
				if shell {
					environment["FACTORY_EVAL_MODEL"] = ""
				}
				if alias {
					Expect(os.Symlink(filepath.Join(owner, "eval"), filepath.Join(foreign, "eval"))).To(Succeed())
					program = filepath.Join(foreign, "eval/runners", harness+".sh")
				}
				beforeOwner, beforeCaller := installationSnapshot(owner), installationSnapshot(physicalCaller)
				observed := sourceRunnerInvoke(physicalCaller, environment, input, program, shell, operands...)
				AddReportEntry("source-runner.transport", map[string]any{"harness": harness, "shell": shell, "directory_alias": alias, "status": observed.status, "stdout": observed.stdout, "stderr": observed.stderr}, ReportEntryVisibilityNever)
				Expect(observed).To(Equal(cliResult{"owner stdout\n", "owner stderr\n", 63}), "harness=%s shell=%t directory alias=%t", harness, shell, alias)
				raw, err := os.ReadFile(record)
				Expect(err).NotTo(HaveOccurred())
				Expect(strings.HasSuffix(string(raw), "\x00")).To(BeTrue())
				Expect(strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")).To(Equal(append([]string{physicalCaller, environment["SOURCE_ENTRY_LITERAL_ENV"], environment["FACTORY_EVAL_MODEL"], "eval", "run", harness}, operands...)))
				Expect(os.ReadFile(capturedInput)).To(Equal([]byte(input)))
				_, err = os.Lstat(marker)
				Expect(os.IsNotExist(err)).To(BeTrue())
				Expect(installationSnapshot(owner)).To(Equal(beforeOwner))
				Expect(installationSnapshot(physicalCaller)).To(Equal(beforeCaller))
			}
		}
		// Bare shell lookup must not turn the caller's grandparents into an owner.
		// per docs/adr/0133-go-source-evaluation-runners.md:154
		owner, caller := fixtureDirectories()
		program := sourceRunnerAdapter(owner, harness)
		base := filepath.Dir(owner)
		ownerCalled, foreignCalled := filepath.Join(base, "LOCAL_OWNER_CALLED"), filepath.Join(base, "FOREIGN_OWNER_CALLED")
		writeFixture(filepath.Join(owner, "factory"), []byte("#!/bin/sh\nprintf '%s\\000' \"$@\" > \"$SOURCE_ENTRY_OWNER\"\nexit 63\n"), 0700)
		writeFixture(filepath.Join(caller, "factory"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_ENTRY_FOREIGN\"\nexit 64\n"), 0700)
		foreignCWD := filepath.Join(caller, "x", "y")
		Expect(os.MkdirAll(foreignCWD, 0700)).To(Succeed())
		environment := map[string]string{"PATH": filepath.Dir(program) + ":/usr/bin:/bin", "BASH_ENV": "", "ENV": "", "SOURCE_ENTRY_OWNER": ownerCalled, "SOURCE_ENTRY_FOREIGN": foreignCalled}
		bare := filepath.Base(program)
		local := sourceRunnerInvoke(filepath.Dir(program), environment, "", bare, true, "literal work")
		Expect(local).To(Equal(cliResult{"", "", 63}), "bare shell invocation in the actual runner directory remains supported")
		Expect(os.ReadFile(ownerCalled)).To(Equal([]byte("eval\x00run\x00" + harness + "\x00literal work\x00")))
		Expect(os.Remove(ownerCalled)).To(Succeed())
		foreign := sourceRunnerInvoke(foreignCWD, environment, "", bare, true, "literal work")
		Expect(foreign.status).To(Equal(2), "PATH-searched bare shell invocation must refuse before choosing a caller-relative owner: %+v", foreign)
		Expect(foreign.stdout).To(BeEmpty())
		Expect(foreign.stderr).NotTo(BeEmpty())
		for _, path := range []string{ownerCalled, foreignCalled} {
			_, err := os.Lstat(path)
			Expect(os.IsNotExist(err)).To(BeTrue(), "bare PATH shell invocation must not execute any factory owner: %s", path)
		}
	}, Entry("mock", "mock"), Entry("Claude", "claude"), Entry("Codex", "codex"), Entry("OpenCode", "opencode"), Entry("unwired example", "example-harness"))

	// per docs/adr/0133-go-source-evaluation-runners.md:23
	// per docs/adr/0133-go-source-evaluation-runners.md:50
	It("refuses final-file aliases and unavailable owners without selecting any fallback", func() {
		for _, harness := range []string{"mock", "claude", "codex", "opencode", "example-harness"} {
			for _, mode := range []string{"missing-owner", "failed-owner", "file-alias"} {
				for _, shell := range []bool{false, true} {
					owner, caller := fixtureDirectories()
					base := filepath.Dir(owner)
					program := sourceRunnerAdapter(owner, harness)
					marker, ownerCalled := filepath.Join(base, "FALLBACK_CALLED"), filepath.Join(base, "OWNER_CALLED")
					tools, foreign := filepath.Join(base, "tools"), filepath.Join(base, "foreign")
					for _, path := range []string{filepath.Join(tools, "factory"), filepath.Join(tools, harness), filepath.Join(foreign, "factory"), filepath.Join(caller, "factory")} {
						writeFixture(path, []byte("#!/bin/sh\nprintf called > \"$SOURCE_ENTRY_DECOY\"\nexit 64\n"), 0700)
					}
					sourceObservabilityPoison(owner)
					if mode != "missing-owner" {
						writeFixture(filepath.Join(owner, "factory"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_ENTRY_OWNER\"\nprintf 'native output\\n'\nprintf 'native error\\n' >&2\nexit 47\n"), 0755)
					}
					if mode == "file-alias" {
						alias := filepath.Join(caller, harness+".sh")
						Expect(os.Symlink(program, alias)).To(Succeed())
						program = alias
					}
					work := filepath.Join(caller, "work")
					writeFixture(filepath.Join(work, "task.md"), []byte("literal task\n"), 0600)
					environment := map[string]string{
						"PATH": tools + ":/usr/bin:/bin", "BASH_ENV": "", "ENV": "", "HOME": caller,
						"SOURCE_ENTRY_DECOY": marker, "SOURCE_ENTRY_OWNER": ownerCalled, "SOURCE_OBSERVABILITY_FALLBACK": marker,
						"FACTORY_RUNTIME_BINARY": filepath.Join(foreign, "factory"), "FACTORY_HOME": foreign,
					}
					beforeOwner, beforeCaller := installationSnapshot(owner), installationSnapshot(caller)
					observed := sourceRunnerInvoke(caller, environment, "borrowed stdin\x00tail\n", program, shell, work)
					AddReportEntry("source-runner.owner-refusal", map[string]any{"harness": harness, "mode": mode, "shell": shell, "status": observed.status, "stdout": observed.stdout, "stderr": observed.stderr}, ReportEntryVisibilityNever)
					_, err := os.Lstat(marker)
					Expect(os.IsNotExist(err)).To(BeTrue(), "%s/%s shell=%t must not run a fallback", harness, mode, shell)
					_, ownerErr := os.Lstat(ownerCalled)
					switch mode {
					case "failed-owner":
						Expect(observed).To(Equal(cliResult{"native output\n", "native error\n", 47}))
						Expect(ownerErr).NotTo(HaveOccurred())
					case "file-alias":
						Expect(observed.status).To(Equal(2))
						Expect(observed.stdout).To(BeEmpty())
						Expect(observed.stderr).To(ContainSubstring("file symlink"))
						Expect(os.IsNotExist(ownerErr)).To(BeTrue())
					case "missing-owner":
						Expect(observed.status).NotTo(Equal(0))
						Expect(observed.stdout).To(BeEmpty())
						Expect(observed.stderr).NotTo(BeEmpty())
						Expect(os.IsNotExist(ownerErr)).To(BeTrue())
					}
					Expect(installationSnapshot(owner)).To(Equal(beforeOwner))
					Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
				}
			}
		}
	})

	// per docs/adr/0133-go-source-evaluation-runners.md:27
	// per docs/adr/0133-go-source-evaluation-runners.md:52
	// per docs/adr/0133-go-source-evaluation-runners.md:61
	It("compiles a private source owner and retains mock modes in relative work directories", func() {
		source := newSourceEntrypointFixture()
		source.environment["HOME"] = os.Getenv("HOME")
		program := sourceRunnerAdapter(source.root, "mock")
		for index, mode := range []string{"", "pass", "fail", "cheat"} {
			work := "relative work ' $(not-code) " + mode
			workPath := filepath.Join(source.caller, work)
			writeFixture(filepath.Join(workPath, "task.md"), []byte("Create answer.txt containing FIXED.\n"), 0600)
			oracle := []byte("#!/bin/sh\nexit 19\n")
			writeFixture(filepath.Join(workPath, "verify.sh"), oracle, 0700)
			source.environment["FACTORY_MOCK_MODE"] = mode
			before := sourceEntrypointSnapshot(source.root)
			observed := sourceRunnerInvoke(source.caller, source.environment, "literal input\x00tail\n", program, index%2 != 0, work)
			Expect(observed).To(Equal(cliResult{"", "", 0}), "mode=%q", mode)
			Expect(sourceEntrypointSnapshot(source.root)).To(Equal(before))
			answer, err := os.ReadFile(filepath.Join(workPath, "answer.txt"))
			if mode == "fail" {
				Expect(os.IsNotExist(err)).To(BeTrue())
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(answer).To(Equal([]byte("FIXED\n")))
			}
			if mode == "cheat" {
				oracle = []byte{}
			}
			Expect(os.ReadFile(filepath.Join(workPath, "verify.sh"))).To(Equal(oracle))
			if index == 0 {
				info, err := os.Stat(filepath.Join(source.root, "factory-go"))
				Expect(err).NotTo(HaveOccurred(), "the first invocation must actually compile its private source owner")
				Expect(info.Mode().IsRegular()).To(BeTrue())
				// Remaining modes exercise the adapter without repeatedly compiling.
				writeFixture(filepath.Join(source.root, "factory"), candidateBinary, 0700)
			}
		}
	})

	// per docs/adr/0133-go-source-evaluation-runners.md:27
	// per docs/adr/0133-go-source-evaluation-runners.md:30
	// per docs/adr/0133-go-source-evaluation-runners.md:53
	DescribeTable("retains native CLI arguments, model, prompt and combined logs", func(harness string, prefix []string) {
		owner, caller := fixtureDirectories()
		program := sourceRunnerAdapter(owner, harness)
		writeFixture(filepath.Join(owner, "factory"), candidateBinary, 0700)
		base := filepath.Dir(owner)
		tools, arguments := filepath.Join(base, "tools"), filepath.Join(base, "arguments")
		writeFixture(filepath.Join(tools, harness), []byte("#!/bin/sh\nprintf '%s\\000' \"$PWD\" \"$SOURCE_RUNNER_LITERAL_ENV\" \"$@\" > \"$SOURCE_RUNNER_ARGUMENTS\"\nprintf 'harness stdout\\n'\nprintf 'harness stderr\\n' >&2\nexit \"$SOURCE_RUNNER_STATUS\"\n"), 0700)
		environment := map[string]string{
			"PATH": tools, "HOME": caller, "BASH_ENV": "", "ENV": "", "GORACE": "atexit_sleep_ms=0",
			"SOURCE_RUNNER_ARGUMENTS": arguments, "SOURCE_RUNNER_LITERAL_ENV": "literal ' $(not-code)\nvalue\n",
		}
		for _, model := range []string{"omitted", "", "literal/model ' $(not-code)\nvalue"} {
			delete(environment, "FACTORY_EVAL_MODEL")
			if model != "omitted" {
				environment["FACTORY_EVAL_MODEL"] = model
			}
			for _, status := range []string{"0", "37"} {
				work := "relative work ' $(not-code) " + status
				workPath := filepath.Join(caller, work)
				prompt := "Implement literal $(touch injected) 'quoted' \"text\"\nsecond line\n\n"
				writeFixture(filepath.Join(workPath, "task.md"), []byte(prompt), 0600)
				environment["SOURCE_RUNNER_STATUS"] = status
				observed := sourceRunnerInvoke(caller, environment, "unused caller stdin\x00tail\n", program, status == "37", work)
				Expect(observed).To(Equal(cliResult{"", "", 0}), "%s model=%q CLI status=%s", harness, model, status)
				expected := append([]string{workPath, environment["SOURCE_RUNNER_LITERAL_ENV"]}, prefix...)
				if model != "" && model != "omitted" {
					expected = append(expected, "--model", model)
				}
				expected = append(expected, strings.TrimRight(prompt, "\n"))
				raw, err := os.ReadFile(arguments)
				Expect(err).NotTo(HaveOccurred())
				Expect(strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")).To(Equal(expected))
				Expect(os.ReadFile(filepath.Join(workPath, harness+"-runner.log"))).To(Equal([]byte("harness stdout\nharness stderr\n")))
				Expect(os.ReadFile(filepath.Join(workPath, "task.md"))).To(Equal([]byte(prompt)))
				_, err = os.Lstat(filepath.Join(workPath, "injected"))
				Expect(os.IsNotExist(err)).To(BeTrue())
			}
		}
	}, Entry("Claude", "claude", []string{"-p", "--permission-mode", "acceptEdits"}), Entry("Codex", "codex", []string{"exec", "--sandbox", "workspace-write", "--skip-git-repo-check"}), Entry("OpenCode", "opencode", []string{"run", "--agent", "implementer"}))

	// per docs/adr/0133-go-source-evaluation-runners.md:36
	// per docs/adr/0133-go-source-evaluation-runners.md:43
	It("adopts native input refusals without launching a CLI or creating a mock answer", func() {
		for _, harness := range []string{"mock", "claude", "codex", "opencode", "example-harness"} {
			owner, caller := fixtureDirectories()
			program := sourceRunnerAdapter(owner, harness)
			writeFixture(filepath.Join(owner, "factory"), candidateBinary, 0700)
			tools, marker := filepath.Join(filepath.Dir(owner), "tools"), filepath.Join(filepath.Dir(owner), "CLI_CALLED")
			for _, name := range []string{"mock", "claude", "codex", "opencode", "example-harness"} {
				writeFixture(filepath.Join(tools, name), []byte("#!/bin/sh\nprintf called > \"$SOURCE_RUNNER_CALLED\"\nexit 0\n"), 0700)
			}
			environment := map[string]string{"PATH": tools, "HOME": caller, "BASH_ENV": "", "ENV": "", "GORACE": "atexit_sleep_ms=0", "SOURCE_RUNNER_CALLED": marker, "FACTORY_EVAL_MODEL": "never select this model"}
			cases := []string{"missing-operand", "empty-work", "missing-work", "file-work"}
			if harness == "example-harness" {
				cases = append(cases, "valid-work")
			} else if harness != "mock" {
				cases = append(cases, "missing-task", "task-directory", "missing-cli")
			}
			for index, mode := range cases {
				work := filepath.Join(caller, mode)
				args, expected := []string{work}, 1
				switch mode {
				case "missing-operand":
					args, expected = nil, 2
				case "empty-work":
					args = []string{""}
				case "file-work":
					writeFixture(work, []byte("not a directory\n"), 0600)
				case "missing-task":
					Expect(os.MkdirAll(work, 0700)).To(Succeed())
				case "task-directory":
					Expect(os.MkdirAll(filepath.Join(work, "task.md"), 0700)).To(Succeed())
				case "valid-work", "missing-cli":
					writeFixture(filepath.Join(work, "task.md"), []byte("literal prompt\n"), 0600)
				}
				if harness == "example-harness" {
					expected = 2
				}
				if mode == "missing-cli" {
					Expect(os.Remove(filepath.Join(tools, harness))).To(Succeed())
				}
				observed := sourceRunnerInvoke(caller, environment, "", program, index%2 != 0, args...)
				Expect(observed.status).To(Equal(expected), "%s/%s: %+v", harness, mode, observed)
				Expect(observed.stdout).To(BeEmpty())
				Expect(observed.stderr).NotTo(BeEmpty())
				_, err := os.Lstat(marker)
				Expect(os.IsNotExist(err)).To(BeTrue(), "%s/%s must reject before invoking any CLI", harness, mode)
				Expect(filepath.WalkDir(caller, func(path string, entry os.DirEntry, walkErr error) error {
					Expect(walkErr).NotTo(HaveOccurred())
					Expect(entry.Name()).NotTo(Equal("answer.txt"), "refused runner created %s", path)
					return nil
				})).To(Succeed())
			}
		}
	})

	// per docs/adr/0133-go-source-evaluation-runners.md:53
	// per docs/adr/0133-go-source-evaluation-runners.md:54
	It("scores pass, fail and oracle tampering through the converted mock adapter", func() {
		owner, caller := fixtureDirectories()
		program := sourceRunnerAdapter(owner, "mock")
		base := filepath.Dir(owner)
		native, receipt := filepath.Join(base, "native evaluator"), filepath.Join(base, "runner calls")
		writeFixture(native, candidateBinary, 0700)
		writeFixture(filepath.Join(owner, "factory"), []byte("#!/bin/sh\nprintf '%s\\000' \"$@\" >> \"$SOURCE_RUNNER_RECEIPT\"\nexec \"$SOURCE_RUNNER_NATIVE\" \"$@\"\n"), 0700)
		writeFixture(filepath.Join(caller, "factory"), candidateBinary, 0700)
		golden := sourceGoldenAdapter(caller)
		writeFixture(filepath.Join(caller, "factory.yaml"), []byte("review_lane: off\n"), 0600)
		writeFixture(filepath.Join(caller, "AGENTS.md"), []byte("Private golden instructions.\n"), 0600)
		orchestrationTask(caller, "reference-answer")
		oraclePath := filepath.Join(caller, "eval/golden-tasks/reference-answer/verify.sh")
		oracle, err := os.ReadFile(oraclePath)
		Expect(err).NotTo(HaveOccurred())
		environment := map[string]string{
			"PATH": "/usr/bin:/bin", "HOME": caller, "BASH_ENV": "", "ENV": "", "GORACE": "atexit_sleep_ms=0",
			"FACTORY_CONFIG": filepath.Join(caller, "factory.yaml"), "SOURCE_RUNNER_RECEIPT": receipt, "SOURCE_RUNNER_NATIVE": native,
			"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull,
		}
		beforeOwner := installationSnapshot(owner)
		var baseline []byte
		for _, mode := range []string{"pass", "fail", "cheat"} {
			environment["FACTORY_MOCK_MODE"] = mode
			args := []string{"--harness", "mock", "--runner", program, "--runs", "1"}
			if mode == "pass" {
				args = append(args, "--save-baseline")
			}
			observed := sourceRunnerInvoke(caller, environment, "", golden, false, args...)
			current := orchestrationReadResults(caller, "current")
			Expect(current.Runner).To(Equal(program))
			Expect(current.Tasks).To(HaveLen(1))
			Expect(current.Tasks[0].Runs).To(Equal(1))
			if mode == "pass" {
				Expect(observed.status).To(Equal(0), "%+v", observed)
				Expect(current.Tasks[0].Passes).To(Equal(1))
				Expect(current.Tasks[0].Score).To(Equal(1.0))
				baseline, err = os.ReadFile(filepath.Join(caller, "eval/results/mock-baseline.json"))
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(observed.status).To(Equal(1), "%+v", observed)
				Expect(observed.stdout).To(ContainSubstring("REGRESSION DETECTED"))
				Expect(current.Tasks[0].Passes).To(Equal(0))
				Expect(current.Tasks[0].Score).To(Equal(0.0))
			}
			Expect(observed.stderr).To(BeEmpty())
			Expect(os.ReadFile(filepath.Join(caller, "eval/results/mock-baseline.json"))).To(Equal(baseline))
			Expect(os.ReadFile(oraclePath)).To(Equal(oracle))
			Expect(installationSnapshot(owner)).To(Equal(beforeOwner))
		}
		raw, err := os.ReadFile(receipt)
		Expect(err).NotTo(HaveOccurred(), "explicit --runner must invoke the actual converted adapter on every evaluation")
		fields := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
		Expect(fields).To(HaveLen(12))
		for offset := 0; offset < len(fields); offset += 4 {
			Expect(fields[offset : offset+3]).To(Equal([]string{"eval", "run", "mock"}))
			Expect(fields[offset+3]).NotTo(Equal(filepath.Dir(oraclePath)))
			_, err = os.Lstat(fields[offset+3])
			Expect(os.IsNotExist(err)).To(BeTrue(), "the evaluator must remove each private work directory")
		}
	})

	// per docs/adr/0133-go-source-evaluation-runners.md:78
	// per docs/adr/0133-go-source-evaluation-runners.md:83
	DescribeTable("settles nested harness process ownership before returning", func(mode string) {
		for _, explicit := range []bool{false, true} {
			sourceRunnerCancellation(mode, explicit, "")
		}
	}, Entry("task timeout", "timeout"), Entry("parent cancellation", "parent-cancellation"))

	// per docs/adr/0133-go-source-evaluation-runners.md:85
	// per docs/adr/0133-go-source-evaluation-runners.md:88
	// per docs/adr/0133-go-source-evaluation-runners.md:98
	It("forcibly bounds an external runner that ignores graceful termination", func() {
		for _, mode := range []string{"timeout", "parent-cancellation"} {
			sourceRunnerCancellation(mode, true, "ignores-term")
		}
	})

	// per docs/adr/0133-go-source-evaluation-runners.md:88
	It("refuses the original non-acknowledging shell timeout without running its oracle or publishing scores", func() {
		sourceRunnerCancellation("timeout", true, "unacknowledged")
	})

	// per docs/adr/0133-go-source-evaluation-runners.md:61
	// per docs/adr/0133-go-source-evaluation-runners.md:83
	It("settles a cold source compiler group before the outer golden timeout returns", func() {
		for _, mode := range []string{"source-compiler", "source-compiler-slow"} {
			sourceRunnerCancellation("timeout", true, mode)
		}
	})

	// per docs/adr/0133-go-source-evaluation-runners.md:111
	// per docs/adr/0133-go-source-evaluation-runners.md:133
	It("propagates a detached harness ownership failure across the actual converted adapter", func() {
		for _, explicit := range []bool{false, true} {
			root, _ := fixtureDirectories()
			base := filepath.Dir(root)
			native, calls := filepath.Join(base, "native evaluator"), filepath.Join(base, "nested owner calls")
			writeFixture(native, candidateBinary, 0700)
			writeFixture(filepath.Join(root, "factory"), []byte("#!/bin/sh\nif [ \"$1\" = eval ] && [ \"$2\" = run ]; then printf '%s\\000' \"$@\" >> \"$SOURCE_RUNNER_NESTED_CALLS\"; fi\nexec \"$SOURCE_RUNNER_NATIVE\" \"$@\"\n"), 0700)
			golden, runner := sourceGoldenAdapter(root), sourceRunnerAdapter(root, "claude")
			writeFixture(filepath.Join(root, "factory.yaml"), []byte("review_lane: off\n"), 0600)
			writeFixture(filepath.Join(root, "AGENTS.md"), []byte("Private detached-child fixture.\n"), 0600)
			orchestrationTask(root, "reference-answer")
			oracleCalled, ready := filepath.Join(base, "ORACLE_CALLED"), filepath.Join(base, "detached ready")
			writeFixture(filepath.Join(root, "eval/golden-tasks/reference-answer/verify.sh"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_RUNNER_ORACLE_CALLED\"\n[ \"$(/bin/cat answer.txt)\" = FIXED ]\n"), 0700)
			tools := filepath.Join(base, "tools")
			writeFixture(filepath.Join(tools, "claude"), []byte("#!/bin/sh\nexec \"$SOURCE_RUNNER_FIXTURE\" "+sourceRunnerDetachedHandshake+" cli \"$@\"\n"), 0700)
			environment := map[string]string{
				"PATH": tools + ":/usr/bin:/bin", "HOME": root, "TMPDIR": base, "GORACE": "atexit_sleep_ms=0", "BASH_ENV": "", "ENV": "",
				"FACTORY_CONFIG": filepath.Join(root, "factory.yaml"), "SOURCE_RUNNER_NATIVE": native, "SOURCE_RUNNER_NESTED_CALLS": calls,
				"SOURCE_RUNNER_FIXTURE": sourceCompilerHelperImage, "SOURCE_RUNNER_DETACHED_READY": ready, "SOURCE_RUNNER_ORACLE_CALLED": oracleCalled,
				"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull,
			}
			sourceRunnerDetachedCleanup(base, ready)
			args := []string{"--harness", "claude", "--runs", "1", "--timeout", "10"}
			if explicit {
				args = append(args, "--runner", runner)
			}
			observed := sourceRunnerInvoke(root, environment, "", golden, false, args...)
			data, err := os.ReadFile(ready)
			Expect(err).NotTo(HaveOccurred())
			var receipt sourceRunnerDetachedReceipt
			Expect(json.Unmarshal(data, &receipt)).To(Succeed())
			Expect(receipt.Group).To(Equal(receipt.PID))
			Expect(receipt.PID).To(BeNumerically(">", 1))
			Expect(receipt.Group).NotTo(Equal(syscall.Getpgrp()))
			Expect(syscall.Kill(receipt.PID, 0)).To(Succeed(), "the uncertainty witness must still be alive before fixture cleanup")
			Expect(syscall.Kill(-receipt.Group, 0)).To(Succeed())
			Expect(os.ReadFile(calls)).To(Equal([]byte("eval\x00run\x00claude\x00"+receipt.Work+"\x00")), "default and explicit selection must traverse the actual source adapter")
			AddReportEntry("source-runner.detached-ownership", map[string]any{"explicit": explicit, "status": observed.status, "stdout": observed.stdout, "stderr": observed.stderr, "receipt": receipt}, ReportEntryVisibilityNever)
			Expect(observed.status).To(Equal(1), "%+v", observed)
			Expect(observed.stdout).NotTo(ContainSubstring("1/1 passed"))
			Expect(observed.stderr).To(ContainSubstring(strconv.Quote(receipt.Work)))
			Expect(observed.stderr).NotTo(ContainSubstring("factory-eval-result-v1"))
			Expect(receipt.TokenPresent).To(BeFalse(), "real harness CLIs must not inherit the private ownership token")
			Expect(os.ReadFile(filepath.Join(receipt.Work, "answer.txt"))).To(Equal([]byte("FIXED\n")))
			for _, forbidden := range []string{oracleCalled, filepath.Join(root, "eval/results/claude-current.json")} {
				_, err = os.Lstat(forbidden)
				Expect(os.IsNotExist(err)).To(BeTrue(), "uncertain nested ownership must neither score nor publish: %s", forbidden)
			}
		}
	})

	// per docs/adr/0133-go-source-evaluation-runners.md:182
	It("retains live task custody when an external runner exceeds the receipt capture bound", func() {
		for _, overflow := range []bool{false, true} {
			root, _ := fixtureDirectories()
			base := filepath.Dir(root)
			writeFixture(filepath.Join(root, "factory"), candidateBinary, 0700)
			golden := sourceGoldenAdapter(root)
			writeFixture(filepath.Join(root, "factory.yaml"), []byte("review_lane: off\n"), 0600)
			writeFixture(filepath.Join(root, "AGENTS.md"), []byte("Private incomplete receipt fixture.\n"), 0600)
			orchestrationTask(root, "reference-answer")
			ready, oracleCalled := filepath.Join(base, "external child ready"), filepath.Join(base, "ORACLE_CALLED")
			writeFixture(filepath.Join(root, "eval/golden-tasks/reference-answer/verify.sh"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_RUNNER_ORACLE_CALLED\"\nexit 0\n"), 0700)
			runner := filepath.Join(root, "custom external controller")
			writeFixture(runner, []byte("#!/bin/sh\ncd \"$1\" || exit 94\nexec \"$SOURCE_RUNNER_FIXTURE\" "+sourceRunnerDetachedHandshake+" overflow\n"), 0700)
			environment := map[string]string{
				"PATH": "/usr/bin:/bin", "HOME": root, "TMPDIR": base, "GORACE": "atexit_sleep_ms=0", "BASH_ENV": "", "ENV": "",
				"FACTORY_CONFIG": filepath.Join(root, "factory.yaml"), "SOURCE_RUNNER_FIXTURE": sourceCompilerHelperImage,
				"SOURCE_RUNNER_DETACHED_READY": ready, "SOURCE_RUNNER_ORACLE_CALLED": oracleCalled,
				"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull,
			}
			if overflow {
				environment["SOURCE_RUNNER_CAPTURE_OVERFLOW"] = "1"
			}
			sourceRunnerDetachedCleanup(base, ready)
			observed := sourceRunnerInvoke(root, environment, "", golden, false, "--harness", "mock", "--runner", runner, "--runs", "1", "--timeout", "10")
			data, err := os.ReadFile(ready)
			Expect(err).NotTo(HaveOccurred())
			var receipt sourceRunnerDetachedReceipt
			Expect(json.Unmarshal(data, &receipt)).To(Succeed())
			Expect(receipt.PID).To(BeNumerically(">", 1))
			Expect(receipt.Group).To(Equal(receipt.PID))
			Expect(receipt.Group).NotTo(Equal(syscall.Getpgrp()))
			Expect(syscall.Kill(receipt.PID, 0)).To(Succeed(), "the detached devnull child remains alive independently of the exited controller")
			Expect(syscall.Kill(-receipt.Group, 0)).To(Succeed())
			Expect(receipt.TokenPresent).To(BeTrue(), "the selected custom controller receives this attempt's private receipt token")
			written, err := os.ReadFile(ready + ".written")
			Expect(err).NotTo(HaveOccurred())
			expectedBytes := len("\nfactory-eval-result-v1 " + strings.Repeat("a", 64) + " ownership-unconfirmed\n")
			if overflow {
				expectedBytes += 16 << 20
			}
			Expect(string(written)).To(Equal(strconv.Itoa(expectedBytes)+"/"+strconv.Itoa(expectedBytes)+"/true\n"), "the real controller must finish writing the complete receipt after the requested output volume")
			AddReportEntry("source-runner.incomplete-receipt", map[string]any{"overflow": overflow, "status": observed.status, "stdout": observed.stdout, "stderr": observed.stderr, "receipt": receipt, "written": string(written)}, ReportEntryVisibilityNever)
			Expect(observed.status).To(Equal(1), "%+v", observed)
			Expect(observed.stderr).To(ContainSubstring(strconv.Quote(receipt.Work)))
			Expect(observed.stdout).NotTo(ContainSubstring("hit the 10s cap"))
			Expect(observed.stdout).NotTo(ContainSubstring("1/1 passed"))
			Expect(observed.stderr).NotTo(ContainSubstring("factory-eval-result-v1"))
			Expect(os.ReadFile(filepath.Join(receipt.Work, "answer.txt"))).To(Equal([]byte("FIXED\n")))
			for _, forbidden := range []string{oracleCalled, filepath.Join(root, "eval/results/mock-current.json")} {
				_, err = os.Lstat(forbidden)
				Expect(os.IsNotExist(err)).To(BeTrue(), "incomplete external ownership reporting must neither score nor publish: %s", forbidden)
			}
		}
	})

	// per docs/adr/0133-go-source-evaluation-runners.md:117
	// per docs/adr/0133-go-source-evaluation-runners.md:128
	It("validates direct runner receipts and removes their private token before launching a real CLI", func() {
		root, _ := fixtureDirectories()
		base := filepath.Dir(root)
		writeFixture(filepath.Join(root, "factory"), candidateBinary, 0700)
		program := sourceRunnerAdapter(root, "claude")
		tools, called := filepath.Join(base, "tools"), filepath.Join(base, "CLI_ENVIRONMENT")
		writeFixture(filepath.Join(tools, "claude"), []byte("#!/bin/sh\n/usr/bin/env > \"$SOURCE_RUNNER_CLI_ENV\"\nexit 37\n"), 0700)
		work := filepath.Join(root, "valid work")
		writeFixture(filepath.Join(work, "task.md"), []byte("Literal direct task.\n"), 0600)
		token := strings.Repeat("a", 64)
		environment := map[string]string{"PATH": tools, "HOME": root, "TMPDIR": base, "GORACE": "atexit_sleep_ms=0", "BASH_ENV": "", "ENV": "", "FACTORY_EVAL_OWNERSHIP_TOKEN": token, "SOURCE_RUNNER_CLI_ENV": called}
		observed := sourceRunnerInvoke(root, environment, "", program, false, work)
		Expect(observed).To(Equal(cliResult{"", "", 0}), "ordinary real CLI nonzero status still receives oracle scoring")
		cliEnvironment, err := os.ReadFile(called)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(cliEnvironment)).NotTo(ContainSubstring("FACTORY_EVAL_OWNERSHIP_TOKEN="), "the token must be absent, rather than merely empty, in the real harness environment")
		for _, invalid := range []string{"", strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64), token + "\ninjected line"} {
			environment["FACTORY_EVAL_OWNERSHIP_TOKEN"] = invalid
			observed = sourceRunnerInvoke(root, environment, "", program, false, filepath.Join(root, "absent work"))
			Expect(observed.status).To(Equal(1))
			Expect(observed.stderr).To(Equal("claude runner: could not complete the run\n"), "an invalid token must never be reflected or emitted as a receipt")
		}
		environment["FACTORY_EVAL_OWNERSHIP_TOKEN"] = token
		for _, args := range [][]string{nil, {filepath.Join(root, "absent work")}} {
			observed = sourceRunnerInvoke(root, environment, "", program, false, args...)
			if len(args) == 0 {
				Expect(observed.status).To(Equal(2))
			} else {
				Expect(observed.status).To(Equal(1))
			}
			Expect(observed.stderr).To(ContainSubstring("factory-eval-result-v1 " + token + " failed\n"))
		}
		ready := filepath.Join(base, "direct detached ready")
		writeFixture(filepath.Join(tools, "claude"), []byte("#!/bin/sh\nexec \"$SOURCE_RUNNER_FIXTURE\" "+sourceRunnerDetachedHandshake+" cli \"$@\"\n"), 0700)
		environment["SOURCE_RUNNER_FIXTURE"] = sourceCompilerHelperImage
		environment["SOURCE_RUNNER_DETACHED_READY"] = ready
		sourceRunnerDetachedCleanup(base, ready)
		observed = sourceRunnerInvoke(root, environment, "", program, false, work)
		Expect(observed.status).To(Equal(1))
		Expect(observed.stderr).To(ContainSubstring("factory-eval-result-v1 " + token + " ownership-unconfirmed\n"))
		data, err := os.ReadFile(ready)
		Expect(err).NotTo(HaveOccurred())
		var receipt sourceRunnerDetachedReceipt
		Expect(json.Unmarshal(data, &receipt)).To(Succeed())
		Expect(receipt.Group).To(Equal(receipt.PID))
		Expect(receipt.PID).To(BeNumerically(">", 1))
		Expect(syscall.Kill(receipt.PID, 0)).To(Succeed())
		Expect(syscall.Kill(-receipt.Group, 0)).To(Succeed())
		Expect(receipt.TokenPresent).To(BeFalse())
		Expect(os.ReadFile(filepath.Join(work, "answer.txt"))).To(Equal([]byte("FIXED\n")))
	})

	// per docs/adr/0133-go-source-evaluation-runners.md:116
	// per docs/adr/0133-go-source-evaluation-runners.md:122
	DescribeTable("honors a matching private nested result receipt without running the oracle", func(status string) {
		sourceRunnerReceiptCase(status)
		if status == "ownership-unconfirmed" {
			sourceRunnerReceiptCase("both")
			sourceRunnerReceiptCase("both-reversed")
		}
	}, Entry("nested rejection", "failed"), Entry("nested ownership uncertainty", "ownership-unconfirmed"))

	// per docs/adr/0133-go-source-evaluation-runners.md:124
	// per docs/adr/0133-go-source-evaluation-runners.md:135
	It("ignores stale or malformed receipts and preserves ordinary custom exit 125 oracle scoring", func() {
		for _, mode := range []string{"none", "foreign", "stale", "prefix", "suffix", "partial", "malformed-token", "unknown-status", "stdout"} {
			sourceRunnerReceiptCase(mode)
		}
	})

	// per docs/adr/0133-go-source-evaluation-runners.md:139
	// per docs/adr/0133-go-source-evaluation-runners.md:145
	DescribeTable("reports source bootstrap failures before native runner execution", func(mode string, directStatus int) {
		root, caller := fixtureDirectories()
		base := filepath.Dir(root)
		launcher, err := os.ReadFile("../factory")
		Expect(err).NotTo(HaveOccurred())
		if mode == "uncertain-observation" {
			// Inject only the existing process-observation result in a private
			// launcher; spawn, wait, receipts and cleanup remain current bytes.
			// per docs/adr/0133-go-source-evaluation-runners.md:149
			anchor := []byte("source_group_present() {\n")
			Expect(bytes.Count(launcher, anchor)).To(Equal(1))
			launcher = bytes.Replace(launcher, anchor, append(append([]byte{}, anchor...), []byte("  return 2\n")...), 1)
		}
		writeFixture(filepath.Join(root, "factory"), launcher, 0700)
		prior := []byte("#!/bin/sh\nprintf called > \"$SOURCE_BOOTSTRAP_OLD_RUNTIME\"\nexit 99\n")
		writeFixture(filepath.Join(root, "factory-go"), prior, 0700)
		runner := sourceRunnerAdapter(root, "claude")
		writeFixture(filepath.Join(caller, "factory"), candidateBinary, 0700)
		golden := sourceGoldenAdapter(caller)
		writeFixture(filepath.Join(caller, "factory.yaml"), []byte("review_lane: off\n"), 0600)
		writeFixture(filepath.Join(caller, "AGENTS.md"), []byte("Private source bootstrap fixture.\n"), 0600)
		orchestrationTask(caller, "reference-answer")
		oracle, harness, oldRuntime := filepath.Join(base, "ORACLE_CALLED"), filepath.Join(base, "HARNESS_CALLED"), filepath.Join(base, "OLD_RUNTIME_CALLED")
		writeFixture(filepath.Join(caller, "eval/golden-tasks/reference-answer/verify.sh"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_BOOTSTRAP_ORACLE\"\nexit 0\n"), 0700)
		tools, calls, compilerPID := filepath.Join(base, "tools"), filepath.Join(base, "source calls"), filepath.Join(base, "compiler pid")
		Expect(os.MkdirAll(tools, 0700)).To(Succeed())
		for _, name := range []string{"bash", "sh", "mktemp", "rm"} {
			path, err := exec.LookPath(name)
			Expect(err).NotTo(HaveOccurred())
			if name == "bash" {
				path = "/bin/bash"
			}
			path, err = filepath.EvalSymlinks(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Symlink(path, filepath.Join(tools, name))).To(Succeed())
		}
		writeFixture(filepath.Join(tools, "git"), []byte("#!/bin/sh\nexit 128\n"), 0700)
		writeFixture(filepath.Join(tools, "claude"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_BOOTSTRAP_HARNESS\"\nexit 95\n"), 0700)
		if mode != "missing-go" {
			writeFixture(filepath.Join(tools, "go"), []byte("#!/bin/sh\nprintf '%s\\n' \"$$\" > \"$SOURCE_BOOTSTRAP_COMPILER_PID\"\nprintf 'controlled compiler failure\\n' >&2\nexit 42\n"), 0700)
		}
		startup := filepath.Join(base, "bootstrap witness.sh")
		// System Bash 3.2 reads BASH_ENV before assigning script argv. Capture
		// the first actual launcher command rather than the startup environment.
		writeFixture(startup, []byte(`trap 'if [ "$0" = "$SOURCE_BOOTSTRAP_LAUNCHER" ] && [ "${1-}" = eval ] && [ "${2-}" = run ]; then
  printf "%s\000" "$0" "$@" > "$SOURCE_BOOTSTRAP_CALLS"
  trap - DEBUG
fi' DEBUG
`), 0600)
		environment := map[string]string{
			"PATH": tools, "HOME": caller, "TMPDIR": base, "GORACE": "atexit_sleep_ms=0", "BASH_ENV": startup, "ENV": "",
			"FACTORY_CONFIG": filepath.Join(caller, "factory.yaml"), "SOURCE_BOOTSTRAP_CALLS": calls, "SOURCE_BOOTSTRAP_COMPILER_PID": compilerPID,
			"SOURCE_BOOTSTRAP_LAUNCHER": filepath.Join(root, "factory"),
			"SOURCE_BOOTSTRAP_ORACLE":   oracle, "SOURCE_BOOTSTRAP_HARNESS": harness, "SOURCE_BOOTSTRAP_OLD_RUNTIME": oldRuntime,
			"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull,
		}
		before := installationSnapshot(root)
		observed := sourceRunnerInvoke(caller, environment, "", golden, false, "--harness", "mock", "--runner", runner, "--runs", "1", "--timeout", "10")
		// A later, unrelated Bash startup must not replace the launcher witness.
		// per docs/adr/0133-go-source-evaluation-runners.md:149
		probe := sourceRunnerInvoke(caller, environment, "", filepath.Join(tools, "bash"), false, "-c", ":")
		Expect(probe).To(Equal(cliResult{"", "", 0}))
		raw, err := os.ReadFile(calls)
		Expect(err).NotTo(HaveOccurred())
		fields := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
		Expect(fields).To(HaveLen(5))
		Expect(fields[:4]).To(Equal([]string{filepath.Join(root, "factory"), "eval", "run", "claude"}), "the evaluator must actually enter the current source launcher")
		work := fields[4]
		Expect(observed.stdout).NotTo(ContainSubstring("hit the 10s cap"))
		Expect(observed.stdout).NotTo(ContainSubstring("factory-eval-result-v1"))
		Expect(observed.stderr).NotTo(ContainSubstring("factory-eval-result-v1"))
		AddReportEntry("source-runner.bootstrap-failure", map[string]any{"mode": mode, "status": observed.status, "stdout": observed.stdout, "stderr": observed.stderr, "source_argv": fields}, ReportEntryVisibilityNever)
		if mode == "uncertain-observation" {
			Expect(observed.status).To(Equal(1), "%+v", observed)
			Expect(observed.stderr).To(ContainSubstring(strconv.Quote(work)))
			_, err = os.Stat(work)
			Expect(err).NotTo(HaveOccurred())
			builds, err := filepath.Glob(filepath.Join(root, ".factory-go.*"))
			Expect(err).NotTo(HaveOccurred())
			Expect(builds).To(HaveLen(1), "source uncertainty must preserve its existing compiler/build custody")
			_, err = os.Lstat(filepath.Join(caller, "eval/results/mock-current.json"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		} else {
			Expect(observed.status).To(Equal(0), "%+v", observed)
			Expect(observed.stderr).To(BeEmpty())
			result := orchestrationReadResults(caller, "current")
			Expect(result.Tasks).To(HaveLen(1))
			Expect(result.Tasks[0].Passes).To(BeZero(), "ordinary bootstrap rejection must skip even an always-passing oracle")
			Expect(installationSnapshot(root)).To(Equal(before))
			_, err = os.Stat(work)
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
		for _, marker := range []string{oracle, harness, oldRuntime} {
			_, err = os.Lstat(marker)
			Expect(os.IsNotExist(err)).To(BeTrue(), "bootstrap failure must not execute downstream work: %s", marker)
		}
		work = filepath.Join(caller, "direct work")
		writeFixture(filepath.Join(work, "task.md"), []byte("Direct bootstrap task.\n"), 0600)
		token := strings.Repeat("c", 64)
		for _, value := range []string{"", "malformed\nreceipt", strings.Repeat("A", 64), strings.Repeat("g", 64), token} {
			if value == "" {
				delete(environment, "FACTORY_EVAL_OWNERSHIP_TOKEN")
			} else {
				environment["FACTORY_EVAL_OWNERSHIP_TOKEN"] = value
			}
			direct := sourceRunnerInvoke(caller, environment, "", runner, false, work)
			Expect(direct.status).To(Equal(directStatus), "mode=%s token=%q: %+v", mode, value, direct)
			if value == token {
				status := "failed"
				if mode == "uncertain-observation" {
					status = "ownership-unconfirmed"
				}
				Expect(direct.stderr).To(ContainSubstring("factory-eval-result-v1 " + token + " " + status + "\n"))
			} else {
				Expect(direct.stderr).NotTo(ContainSubstring("factory-eval-result-v1"))
			}
		}
		other := sourceRunnerInvoke(caller, environment, "", filepath.Join(root, "factory"), false, "help")
		Expect(other.status).To(Equal(directStatus))
		Expect(other.stderr).NotTo(ContainSubstring("factory-eval-result-v1"), "a private token must not turn other source commands into evaluation receipts")
		Expect(os.ReadFile(filepath.Join(root, "factory-go"))).To(Equal(prior))
		if mode != "missing-go" {
			data, err := os.ReadFile(compilerPID)
			Expect(err).NotTo(HaveOccurred())
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			Expect(err).NotTo(HaveOccurred())
			Expect(pid).To(BeNumerically(">", 1))
			Expect(errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)).To(BeTrue(), "the test-owned compiler must have exited before fixture cleanup")
		}
	}, Entry("missing toolchain", "missing-go", 1), Entry("ordinary compiler failure", "compiler-failed", 42), Entry("uncertain compiler observation", "uncertain-observation", 125))
})
