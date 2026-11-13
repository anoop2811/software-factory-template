package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Only process observations are controlled; the compiler and reaper remain real.
// per docs/adr/0150-bounded-source-group-reobservation.md:49
func sourceObservationLauncher(launcher []byte) []byte {
	anchor := []byte("source_group_present() {\n")
	Expect(bytes.Count(launcher, anchor)).To(Equal(1))
	launcher = bytes.Replace(launcher, anchor, []byte("source_actual_group_present() {\n"), 1)
	wrapper := []byte(`
source_group_present() {
  SOURCE_TEST_OBSERVATIONS=${SOURCE_TEST_OBSERVATIONS:-0}
  SOURCE_TEST_OBSERVATIONS=$((SOURCE_TEST_OBSERVATIONS + 1))
  if [ "$SOURCE_TEST_OBSERVATIONS" -eq 1 ] || [ "$SOURCE_TEST_OBSERVATION" = persistent ]; then
    printf 'unknown %s\n' "$SOURCE_TEST_OBSERVATIONS" >> "$SOURCE_TEST_PROBES"
    return 2
  fi
  if [ "$SOURCE_TEST_OBSERVATION" = present ] || [ "$SOURCE_TEST_OBSERVATION" = cancel ]; then
    printf 'reported-present %s\n' "$SOURCE_TEST_OBSERVATIONS" >> "$SOURCE_TEST_PROBES"
    return 0
  fi
  local observed
  if source_actual_group_present; then observed=0; else observed=$?; fi
  printf 'actual %s %s\n' "$SOURCE_TEST_OBSERVATIONS" "$observed" >> "$SOURCE_TEST_PROBES"
  return "$observed"
}
`)
	start := bytes.Index(launcher, []byte("source_actual_group_present() {\n"))
	end := bytes.Index(launcher[start:], []byte("\n}\n"))
	Expect(end).To(BeNumerically(">", 0))
	position := start + end + len("\n}\n")
	launcher = append(append(append([]byte{}, launcher[:position]...), wrapper...), launcher[position:]...)
	// Observe every existing signal command without replacing or suppressing it.
	// per docs/adr/0150-bounded-source-group-reobservation.md:35
	var instrumented strings.Builder
	signals := 0
	for _, line := range strings.SplitAfter(string(launcher), "\n") {
		if strings.Contains(line, "builtin kill -") && !strings.Contains(line, "builtin kill -0 ") {
			instrumented.WriteString("  printf 'signal-attempt\n' >> \"$SOURCE_TEST_SIGNALS\"\n")
			signals++
		}
		instrumented.WriteString(line)
	}
	Expect(signals).To(BeNumerically(">", 0), "the signal witness must cover real launcher signal commands")
	return []byte(instrumented.String())
}

// Existing public-entrypoint criterion also owns deterministic uncertainty cases.
// per docs/adr/0150-bounded-source-group-reobservation.md:54
func sourceEntrypointGroupObservationCases() {
	GinkgoHelper()
	launcher, err := os.ReadFile("../factory")
	Expect(err).NotTo(HaveOccurred())
	launcher = sourceObservationLauncher(launcher)
	for _, test := range []struct {
		observation, actor string
		status             int
	}{
		{"transient", "healthy", 0},
		{"transient", "failed-build", 58},
		{"transient", "directory-race", 1},
		{"transient", "symlink-race", 1},
		{"persistent", "healthy", 125},
		{"present", "healthy", 125},
		{"cancel", "healthy", 125},
	} {
		root, caller := fixtureDirectories()
		base := filepath.Dir(root)
		tools := filepath.Join(base, "compiler tools")
		record, prior := filepath.Join(base, "compiler argv"), filepath.Join(base, "PRIOR_BINARY_EXECUTED")
		outside, image := filepath.Join(base, "outside output"), filepath.Join(base, "actual native image")
		probes, signals, identity := filepath.Join(base, "group observations"), filepath.Join(base, "signal attempts"), filepath.Join(base, "compiler identity")
		writeFixture(probes, nil, 0600)
		writeFixture(signals, nil, 0600)
		writeFixture(filepath.Join(outside, "sentinel"), []byte("outside user data\n"), 0600)
		writeFixture(image, candidateBinary, 0700)
		caseLauncher := launcher
		if test.observation == "cancel" {
			caseLauncher = sourceObservationCancellationLauncher(launcher)
		}
		writeFixture(filepath.Join(root, "factory"), caseLauncher, 0755)
		old := []byte("#!/bin/sh\nprintf called > \"$SOURCE_ENTRY_PRIOR\"\nexit 96\n")
		writeFixture(filepath.Join(root, "factory-go"), old, 0700)
		compiler := strings.Replace(sourceEntrypointFailingCompiler, "set -eu\n", "set -eu\nprintf '%s ' \"$$\" > \"$SOURCE_TEST_IDENTITY\"\n/bin/ps -o pgid= -p \"$$\" >> \"$SOURCE_TEST_IDENTITY\"\n", 1)
		writeFixture(filepath.Join(tools, "go"), []byte(compiler), 0700)
		environment := map[string]string{"PATH": tools + string(os.PathListSeparator) + "/usr/bin:/bin", "SOURCE_ENTRY_MODE": test.actor, "SOURCE_ENTRY_ROOT": root, "SOURCE_ENTRY_IMAGE": image, "SOURCE_ENTRY_BUILD_RECORD": record, "SOURCE_ENTRY_OUTSIDE": outside, "SOURCE_ENTRY_PRIOR": prior, "GORACE": "atexit_sleep_ms=0", "SOURCE_TEST_OBSERVATION": test.observation, "SOURCE_TEST_PROBES": probes, "SOURCE_TEST_SIGNALS": signals, "SOURCE_TEST_IDENTITY": identity}
		before, callerBefore, outsideBefore := installationSnapshot(root), installationSnapshot(caller), installationSnapshot(outside)
		started := time.Now()
		var result cliResult
		if test.observation == "cancel" {
			result = sourceObservationCancelInvocation(caller, environment, filepath.Join(root, "factory"))
		} else {
			result = handoffNativeProcess(caller, environment, "application input must remain borrowed\n", filepath.Join(root, "factory"), "help")
		}
		elapsed := time.Since(started)
		after := installationSnapshot(root)
		probeData, err := os.ReadFile(probes)
		Expect(err).NotTo(HaveOccurred())
		signalData, err := os.ReadFile(signals)
		Expect(err).NotTo(HaveOccurred())
		rawIdentity, err := os.ReadFile(identity)
		Expect(err).NotTo(HaveOccurred())
		var pid, group int
		_, err = fmt.Sscanf(string(rawIdentity), "%d %d", &pid, &group)
		Expect(err).NotTo(HaveOccurred())
		Expect(pid).To(BeNumerically(">", 1))
		Expect(group).To(Equal(pid), "the actual compiler must have been admitted to its own group")
		pidErr, groupErr := syscall.Kill(pid, 0), syscall.Kill(-group, 0)
		AddReportEntry("source-launcher.group-reobservation", map[string]any{"observation": test.observation, "actor": test.actor, "status": result.status, "stdout": result.stdout, "stderr": result.stderr, "elapsed_seconds": elapsed.Seconds(), "compiler_pid": pid, "compiler_group": group, "pid_probe_error": fmt.Sprint(pidErr), "group_probe_error": fmt.Sprint(groupErr), "observations": string(probeData), "signals": string(signalData)}, ReportEntryVisibilityNever)
		Expect(pidErr).To(Equal(syscall.ESRCH), "the real compiler must have terminated")
		Expect(groupErr).To(Equal(syscall.ESRCH), "absence must be a real kernel observation")
		Expect(signalData).To(BeEmpty(), "initial uncertainty may not authorize any signal")
		Expect(installationSnapshot(caller)).To(Equal(callerBefore))
		Expect(installationSnapshot(outside)).To(Equal(outsideBefore))
		publicNoPoison(prior)
		Expect(result.status).To(Equal(test.status), "observation=%s actor=%s stderr=%s probes=%s", test.observation, test.actor, result.stderr, probeData)
		lines := strings.Split(strings.TrimSpace(string(probeData)), "\n")
		if test.observation == "transient" {
			Expect(lines).To(Equal([]string{"unknown 1", "actual 2 1"}), "uncertainty must resolve through the unmodified real group probe")
			Expect(result.stderr).NotTo(ContainSubstring("ownership is unconfirmed"))
			switch test.actor {
			case "failed-build":
				Expect(result.stdout).To(BeEmpty())
				Expect(result.stderr).To(Equal("controlled compiler failure\n"))
				Expect(os.ReadFile(filepath.Join(root, "factory-go"))).To(Equal(old))
			case "healthy":
				Expect(result.stdout).To(Equal(nativeCLIUsage))
				Expect(result.stderr).To(BeEmpty())
				Expect(os.ReadFile(filepath.Join(root, "factory-go"))).To(Equal(candidateBinary))
			default:
				Expect(result.stdout).To(BeEmpty())
				Expect(result.stderr).To(ContainSubstring("developer build destination is unsafe"))
				if test.actor == "directory-race" {
					entries, err := os.ReadDir(filepath.Join(root, "factory-go"))
					Expect(err).NotTo(HaveOccurred())
					Expect(entries).To(HaveLen(1))
					Expect(entries[0].Name()).To(Equal("sentinel"))
					Expect(os.ReadFile(filepath.Join(root, "factory-go", "sentinel"))).To(Equal([]byte("controlled directory race\n")))
					delete(after, "factory-go/sentinel")
				} else {
					Expect(os.Readlink(filepath.Join(root, "factory-go"))).To(Equal(outside))
				}
			}
			delete(before, "factory-go")
			delete(after, "factory-go")
			Expect(after).To(Equal(before), "confirmed terminal absence must leave no private compiler artifacts")
		} else {
			Expect(result.stdout).To(BeEmpty())
			Expect(result.stderr).To(ContainSubstring("ownership is unconfirmed"))
			switch test.observation {
			case "persistent":
				Expect(lines).To(Equal([]string{"unknown 1", "unknown 2", "unknown 3", "unknown 4", "unknown 5", "unknown 6"}))
			case "cancel":
				Expect(lines).To(Equal([]string{"unknown 1"}), "signal reentry must retain the original uncertainty")
			default:
				Expect(lines).To(Equal([]string{"unknown 1", "reported-present 2"}))
			}
			retained := map[string]int{}
			for path := range after {
				for _, prefix := range []string{".factory-go.", ".factory-startup.", ".factory-admission."} {
					if strings.HasPrefix(path, prefix) {
						retained[prefix]++
						delete(after, path)
					}
				}
			}
			Expect(retained).To(Equal(map[string]int{".factory-go.": 1, ".factory-startup.": 1, ".factory-admission.": 1}))
			Expect(after).To(Equal(before), "uncertainty preserves the old executable and every unrelated source file")
		}
	}
}

// Resume while the writer still reserves the helper's lifetime, then release
// exactly once. Cleanup callers inspect both errors before discarding evidence.
// per docs/adr/0151-reliable-ci-qualification.md:261
func sourceHeldChildRelease(release *os.File, resume func() error) func() error {
	var once sync.Once
	var result error
	return func() error {
		once.Do(func() {
			if resume != nil {
				result = resume()
			}
			result = errors.Join(result, release.Close())
		})
		return result
	}
}

// Every fixture holds its actual helper executable with its private evidence.
// per docs/adr/0151-reliable-ci-qualification.md:266
func sourcePrivateCompilerHelper(base string) string {
	GinkgoHelper()
	image, err := os.ReadFile(sourceCompilerHelperImage)
	Expect(err).NotTo(HaveOccurred())
	path := filepath.Join(base, "held-child-helper")
	writeFixture(path, image, 0700)
	return path
}

// Cleanup owns one order: complete parent signaling/joining, release held
// descendants, then observe absence without sending them a late signal.
// per docs/adr/0151-reliable-ci-qualification.md:274
func sourceLauncherFixtureCleanup(owner *ciInterruptionOwner, base string, release func() error, ready bool, descendants ...int) {
	GinkgoHelper()
	result := owner.TerminateAndWait()
	if !result.Joined || result.SignalError != nil || !ready {
		holdFixtureCleanup(base, "source launcher join or descendant readiness was not confirmed")
	}
	releaseErr := release()
	if releaseErr != nil {
		holdFixtureCleanup(base, "source launcher descendant release failed")
	}
	AddReportEntry("source-launcher.fixture-cleanup", map[string]any{"launcher_joined": result.Joined, "signal_error": fmt.Sprint(result.SignalError), "wait_error": fmt.Sprint(result.WaitError), "release_error": fmt.Sprint(releaseErr), "readiness_confirmed": ready, "descendants": descendants}, ReportEntryVisibilityNever)
	for _, pid := range descendants {
		absent := false
		defer func() {
			if !absent {
				holdFixtureCleanup(base, "source launcher descendant absence was not confirmed")
			}
		}()
		Expect(pid).To(BeNumerically(">", 1), "never probe a guessed descendant identity")
		Eventually(func() bool {
			absent = errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
			return absent
		}, 3*time.Second).Should(BeTrue(), "release and observe the actual descendant before deleting its fixture")
	}
	Expect(result.Joined).To(BeTrue(), "the source launcher must join within its original cleanup bound")
	Expect(result.SignalError).NotTo(HaveOccurred())
	Expect(ready).To(BeTrue(), "unknown readiness retains the private fixture")
	Expect(releaseErr).NotTo(HaveOccurred())
}

// A real held child remains outside the expected group; observation uncertainty
// never grants authority to terminate it, even when Bash knows it is stopped.
// per docs/adr/0150-bounded-source-group-reobservation.md:58
func sourceUncertainLiveChild(stopped bool) {
	GinkgoHelper()
	root, caller := fixtureDirectories()
	base := filepath.Dir(root)
	helper := sourcePrivateCompilerHelper(base)
	launcher, err := os.ReadFile("../factory")
	Expect(err).NotTo(HaveOccurred())
	launcher = sourceObservationLauncher(launcher)
	monitor := []byte("\nset -m\n")
	Expect(bytes.Count(launcher, monitor)).To(Equal(1))
	launcher = bytes.Replace(launcher, monitor, []byte("\nset +m\n"), 1)
	admission := []byte("  compiler_group=\"$(/bin/ps -o pgid= -p \"$compiler_pid\")\" || return")
	Expect(bytes.Count(launcher, admission)).To(Equal(1))
	// Reuse the evaluator's proven capture-isolation helper: Bash may keep
	// private duplicates beyond the two visible standard descriptors.
	// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:241
	hold := []byte("  source_restore_stderr\n  exec 8>&1 9>&2\n  exec \"$SOURCE_TEST_CHILD_HELPER\" " + sourcePriorHandshake + "\n")
	launcher = bytes.Replace(launcher, admission, append(hold, admission...), 1)
	writeFixture(filepath.Join(root, "factory"), launcher, 0700)
	old := []byte("prior developer executable\n")
	writeFixture(filepath.Join(root, "factory-go"), old, 0700)
	tools, marker := filepath.Join(base, "tools"), filepath.Join(base, "UNADMITTED_COMPILER_EXECUTED")
	writeFixture(filepath.Join(tools, "go"), []byte("#!/bin/sh\nprintf called > \"$SOURCE_UNADMITTED_MARKER\"\nexit 99\n"), 0700)
	probes, signals := filepath.Join(base, "observations"), filepath.Join(base, "signal attempts")
	childReady := filepath.Join(base, "unadmitted child ready.json")
	writeFixture(probes, nil, 0600)
	writeFixture(signals, nil, 0600)
	ready, readyWriter, err := os.Pipe()
	Expect(err).NotTo(HaveOccurred())
	releaseReader, release, err := os.Pipe()
	Expect(err).NotTo(HaveOccurred())
	for _, file := range []*os.File{ready, readyWriter, releaseReader} {
		DeferCleanup(func() { err := file.Close(); Expect(err == nil || errors.Is(err, os.ErrClosed)).To(BeTrue()) })
	}
	childPID, resumeCount := 0, 0
	childStopped, readinessConfirmed := false, false
	releaseEvents := []string{}
	releaseChild := sourceHeldChildRelease(release, func() error {
		if !childStopped {
			return nil
		}
		if _, err := release.Stat(); err != nil {
			return fmt.Errorf("held helper release closed before resume: %w", err)
		}
		resumeCount++
		releaseEvents = append(releaseEvents, "resume-held-child")
		return syscall.Kill(childPID, syscall.SIGCONT)
	})
	DeferCleanup(func() { Expect(releaseChild()).To(Succeed()) })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Keep the borrowed evaluator group non-orphaned when the launcher exits;
	// an orphaned stopped group would receive kernel SIGHUP/CONT independently.
	// per docs/adr/0150-bounded-source-group-reobservation.md:58
	var stdout, stderr bytes.Buffer
	before, callerBefore := installationSnapshot(root), installationSnapshot(caller)
	owner, err := startDirectInterruptionOwner(ctx, func(childContext context.Context) *exec.Cmd {
		command := exec.CommandContext(childContext, filepath.Join(root, "factory"), "help") // #nosec G204 -- evaluator-owned launcher with a controlled admission handshake.
		command.Dir = caller
		command.Env = append(os.Environ(), "PATH="+tools+string(os.PathListSeparator)+"/usr/bin:/bin", "SOURCE_UNADMITTED_MARKER="+marker, "SOURCE_TEST_OBSERVATION=transient", "SOURCE_TEST_PROBES="+probes, "SOURCE_TEST_SIGNALS="+signals, "SOURCE_TEST_CHILD_HELPER="+helper, "SOURCE_PRIOR_READY="+childReady, "SOURCE_PRIOR_UNRELATED_TOKEN=", "GORACE=atexit_sleep_ms=0", "BASH_ENV=", "ENV=")
		command.ExtraFiles = []*os.File{readyWriter, releaseReader}
		command.Stdout, command.Stderr = &stdout, &stderr
		return command
	})
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() {
		// Finish every parent signal before the held helper can leave, and never
		// discard either process's borrowed evidence on incomplete cleanup.
		// per docs/adr/0151-reliable-ci-qualification.md:261
		result := owner.TerminateAndWait()
		if !result.Joined || result.SignalError != nil || !readinessConfirmed {
			holdFixtureCleanup(base, "uncertain-child launcher custody or helper readiness was not confirmed")
		}
		releaseErr := releaseChild()
		releaseEvents = append(releaseEvents, "release-child")
		if releaseErr != nil {
			holdFixtureCleanup(base, "uncertain-child resume or release failed")
		}
		AddReportEntry("source-launcher.uncertain-child-cleanup", map[string]any{"child_pid": childPID, "readiness_confirmed": readinessConfirmed, "stopped_by_evaluator": childStopped, "resume_count": resumeCount, "release_events": releaseEvents, "release_error": fmt.Sprint(releaseErr), "launcher_joined": result.Joined, "launcher_signal_error": fmt.Sprint(result.SignalError)}, ReportEntryVisibilityNever)
		childAbsent := false
		defer func() {
			if !childAbsent {
				holdFixtureCleanup(base, "uncertain-child helper absence was not confirmed")
			}
		}()
		if childPID > 1 {
			Eventually(func() bool {
				childAbsent = errors.Is(syscall.Kill(childPID, 0), syscall.ESRCH)
				return childAbsent
			}, 3*time.Second).Should(BeTrue(), "release and reap the actual controlled child before deleting its fixture")
		}
		Expect(result.Joined).To(BeTrue(), "the owned launcher must join during bounded fixture cleanup")
		Expect(result.SignalError).NotTo(HaveOccurred())
		Expect(readinessConfirmed).To(BeTrue())
		Expect(releaseErr).NotTo(HaveOccurred())
		if childStopped {
			Expect(resumeCount).To(Equal(1))
			Expect(releaseEvents).To(Equal([]string{"resume-held-child", "release-child"}))
		} else {
			Expect(resumeCount).To(BeZero(), "an unstopped held child receives no cleanup signal")
			Expect(releaseEvents).To(Equal([]string{"release-child"}))
		}
	})
	Expect(readyWriter.Close()).To(Succeed())
	Expect(releaseReader.Close()).To(Succeed())
	var child struct {
		PID, Group       int
		CaptureIsolation sourcePriorCaptureIsolation
	}
	Eventually(func() error {
		body, err := os.ReadFile(childReady)
		if err != nil {
			return err
		}
		return json.Unmarshal(body, &child)
	}, 3*time.Second).Should(Succeed(), "the owned child must publish readiness after isolating all capture duplicates")
	Expect(child.CaptureIsolation.ReleasePreserved).To(BeTrue())
	Expect(child.CaptureIsolation.ClosedCaptureDuplicates).To(ContainElements(8, 9))
	AddReportEntry("source-launcher.uncertain-child-capture-isolation", child.CaptureIsolation, ReportEntryVisibilityNever)
	childPID = child.PID
	Expect(childPID).To(BeNumerically(">", 1))
	childGroup, err := syscall.Getpgid(childPID)
	Expect(err).NotTo(HaveOccurred())
	launcherGroup, err := syscall.Getpgid(owner.PID())
	Expect(err).NotTo(HaveOccurred())
	Expect(childGroup).To(Equal(launcherGroup))
	Expect(childGroup).NotTo(Equal(childPID))
	Expect(syscall.Kill(-childPID, 0)).To(Equal(syscall.ESRCH), "the expected compiler group is actually absent")
	readinessConfirmed = true
	if stopped {
		Expect(syscall.Kill(childPID, syscall.SIGSTOP)).To(Succeed())
		childStopped = true
		Eventually(func() bool {
			data, err := exec.Command("/bin/ps", "-o", "stat=", "-p", strconv.Itoa(childPID)).Output() // #nosec G204 -- fixed utility and the observed owned child PID.
			return err == nil && strings.HasPrefix(strings.TrimSpace(string(data)), "T")
		}, 3*time.Second).Should(BeTrue(), "observe actual stopped state before signaling the parent")
	}
	started := time.Now()
	Expect(owner.Signal(syscall.SIGTERM)).To(Succeed())
	returned, err := owner.AwaitExit(3 * time.Second)
	Expect(err).NotTo(HaveOccurred(), "the launcher must refuse without waiting for the held child or its release")
	if !returned.Joined || returned.SignalError != nil {
		holdFixtureCleanup(base, "uncertain-child observer could not confirm terminal launcher custody")
	}
	Expect(returned.Joined).To(BeTrue())
	Expect(returned.SignalError).NotTo(HaveOccurred())
	Expect(returned.State).NotTo(BeNil())
	elapsed := time.Since(started)
	childErr := syscall.Kill(childPID, 0)
	probeData, err := os.ReadFile(probes)
	Expect(err).NotTo(HaveOccurred())
	signalData, err := os.ReadFile(signals)
	Expect(err).NotTo(HaveOccurred())
	AddReportEntry("source-launcher.uncertain-live-child", map[string]any{"stopped": stopped, "status": returned.State.ExitCode(), "elapsed_seconds": elapsed.Seconds(), "stderr": stderr.String(), "child_pid": childPID, "child_group": childGroup, "launcher_group": launcherGroup, "child_still_alive": childErr == nil, "observations": string(probeData), "signal_attempts": string(signalData)}, ReportEntryVisibilityNever)
	Expect(returned.WaitError).To(HaveOccurred())
	Expect(returned.State.ExitCode()).To(Equal(125))
	Expect(childErr).To(Succeed(), "initial uncertainty must not authorize killing the live or stopped direct child")
	Expect(signalData).To(BeEmpty(), "neither the borrowed group nor the direct child may be signalled")
	Expect(strings.Split(strings.TrimSpace(string(probeData)), "\n")).To(Equal([]string{"unknown 1", "actual 2 1"}))
	Expect(stdout.String()).To(BeEmpty())
	Expect(stderr.String()).To(ContainSubstring("ownership is unconfirmed"))
	Expect(installationSnapshot(caller)).To(Equal(callerBefore))
	publicNoPoison(marker)
	after, retained := installationSnapshot(root), map[string]int{}
	for path := range after {
		for _, prefix := range []string{".factory-go.", ".factory-startup.", ".factory-admission."} {
			if strings.HasPrefix(path, prefix) {
				retained[prefix]++
				delete(after, path)
			}
		}
	}
	Expect(retained).To(Equal(map[string]int{".factory-go.": 1, ".factory-startup.": 1, ".factory-admission.": 1}))
	Expect(after).To(Equal(before), "uncertain live-child custody preserves all existing source files")
}

// Pause the private copy at its first uncertainty sleep, after initial refusal
// has been observed but before a later group observation can authorize anything.
// per docs/adr/0150-bounded-source-group-reobservation.md:36
func sourceObservationCancellationLauncher(launcher []byte) []byte {
	anchor := []byte("source_stop_build() {\n")
	Expect(bytes.Count(launcher, anchor)).To(Equal(1))
	start := bytes.Index(launcher, anchor)
	end := bytes.Index(launcher[start:], []byte("\n}\n"))
	Expect(end).To(BeNumerically(">", 0))
	end += start + len("\n}\n")
	body := launcher[start:end]
	sleep := []byte("/bin/sleep 0.1")
	Expect(bytes.Count(body, sleep)).To(BeNumerically(">", 0))
	hold := []byte(`if [ "${SOURCE_TEST_SLEEP_HELD:-0}" -eq 0 ]; then
          SOURCE_TEST_SLEEP_HELD=1
          printf '{"PID":%s}\n' "$$" >&3
          IFS= read -r _source_test_release <&4 || :
        fi
        /bin/sleep 0.1`)
	body = bytes.Replace(body, sleep, hold, 1)
	return append(append(append([]byte{}, launcher[:start]...), body...), launcher[end:]...)
}

// The signal reaches the real parent only after its first unknown observation.
// per docs/adr/0150-bounded-source-group-reobservation.md:36
func sourceObservationCancelInvocation(caller string, environment map[string]string, program string) cliResult {
	GinkgoHelper()
	base := filepath.Dir(caller)
	ready, readyWriter, err := os.Pipe()
	Expect(err).NotTo(HaveOccurred())
	releaseReader, release, err := os.Pipe()
	Expect(err).NotTo(HaveOccurred())
	for _, file := range []*os.File{ready, readyWriter, releaseReader} {
		defer func() { _ = file.Close() }()
	}
	releaseOwner := sourceHeldChildRelease(release, nil)
	defer func() { Expect(releaseOwner()).To(Succeed()) }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	owner, err := startDirectInterruptionOwner(ctx, func(childContext context.Context) *exec.Cmd {
		command := exec.CommandContext(childContext, program, "help") // #nosec G204 -- test-owned launcher and fixed help invocation.
		command.Dir = caller
		command.Env = os.Environ()
		for name, value := range environment {
			command.Env = append(command.Env, name+"="+value)
		}
		command.ExtraFiles = []*os.File{readyWriter, releaseReader}
		command.Stdin = strings.NewReader("application input must remain borrowed\n")
		command.Stdout, command.Stderr = &stdout, &stderr
		return command
	})
	Expect(err).NotTo(HaveOccurred())
	readinessConfirmed := false
	defer func() { sourceLauncherFixtureCleanup(owner, base, releaseOwner, readinessConfirmed) }()
	Expect(readyWriter.Close()).To(Succeed())
	Expect(releaseReader.Close()).To(Succeed())
	Expect(ready.SetReadDeadline(time.Now().Add(3 * time.Second))).To(Succeed())
	var announced struct{ PID int }
	if readyErr := json.NewDecoder(ready).Decode(&announced); readyErr != nil {
		holdFixtureCleanup(base, "uncertainty handshake did not establish owner identity")
		Fail(fmt.Sprintf("uncertainty handshake failed before signaling: %v", readyErr))
	}
	Expect(announced.PID).To(Equal(owner.PID()))
	readinessConfirmed = true
	Expect(os.ReadFile(environment["SOURCE_TEST_PROBES"])).To(Equal([]byte("unknown 1\n")), "the signal must arrive after the first unknown and before the next probe")
	Expect(owner.Signal(syscall.SIGTERM)).To(Succeed())
	returned, err := owner.AwaitExit(3 * time.Second)
	Expect(err).NotTo(HaveOccurred(), "cancellation during uncertainty must not require the observation hold to be released")
	Expect(returned.Joined).To(BeTrue())
	Expect(returned.WaitError).To(HaveOccurred())
	Expect(returned.State).NotTo(BeNil())
	Expect(ctx.Err()).NotTo(HaveOccurred())
	return cliResult{stdout.String(), stderr.String(), returned.State.ExitCode()}
}
