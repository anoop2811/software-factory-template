package acceptance_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// per docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:279
var _ = Describe("Authenticated ordinary Go installer bootstrap", func() {
	It("settles a queued watcher stop without TERM and retires control after joining", func() {
		for _, interpreter := range bootstrapWatchdogInterpreters() {
			bootstrapWatcherParentRefusal(interpreter)
			// per docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:292
			for _, stage := range []string{"verifier-publication", "queued", "joining-hup", "joined", "channel-error"} {
				bootstrapWatcherCase(interpreter, stage)
			}
		}
	})
})

// per docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:286
func bootstrapWatcherParentRefusal(interpreter string) {
	GinkgoHelper()
	f := ordinaryBootstrapSentinel(defaultInitTag, "")
	data, err := os.ReadFile(f.installer)
	Expect(err).NotTo(HaveOccurred())
	check := "    [ -p /dev/fd/9 ] || { candidate_retain=1; return 125; }"
	Expect(strings.Count(string(data), check)).To(Equal(1))
	writeFixture(f.installer, []byte(strings.Replace(string(data), check, "    false || { candidate_retain=1; return 125; }", 1)), 0600)
	sentinel := filepath.Join(f.root, "caller-fd9")
	original := []byte("caller-owned descriptor must remain unchanged\n")
	writeFixture(sentinel, original, 0600)
	caller, err := os.OpenFile(sentinel, os.O_RDWR, 0600)
	Expect(err).NotTo(HaveOccurred())
	defer caller.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := handoffCommand(ctx, f.cwd, f.environment, "", interpreter, f.installer, "init", "--ref="+defaultInitTag)
	command.ExtraFiles = make([]*os.File, 7)
	command.ExtraFiles[6] = caller // ExtraFiles[0] is FD3; the last entry is FD9.
	command.WaitDelay = time.Second
	var stderr bytes.Buffer
	command.Stderr = &stderr
	err = command.Run()
	var exited *exec.ExitError
	Expect(errors.As(err, &exited)).To(BeTrue(), "%v: %s", err, stderr.String())
	Expect(exited.ExitCode()).To(Equal(125), "%s", stderr.String())
	Expect(ctx.Err()).NotTo(HaveOccurred())
	observed, err := os.ReadFile(sentinel)
	Expect(err).NotTo(HaveOccurred())
	AddReportEntry("bootstrap.parent-channel-refusal", map[string]any{"interpreter": interpreter, "status": exited.ExitCode(), "caller_fd9": string(observed), "stderr": stderr.String()}, ReportEntryVisibilityAlways)
	Expect(observed).To(Equal(original), "failed validation must not arm writes through the restored caller FD9")
	ordinaryBootstrapNoTrace(f)
	Expect(stderr.String()).To(ContainSubstring("preserve private cache"))
	retained, err := filepath.Glob(filepath.Join(f.root, "factory-release.*"))
	Expect(err).NotTo(HaveOccurred())
	Expect(retained).To(HaveLen(1))
}

func bootstrapWatchdogInterpreters() []string {
	GinkgoHelper()
	interpreters := []string{"/bin/sh"}
	if _, err := os.Stat("/bin/dash"); err == nil {
		interpreters = append(interpreters, "/bin/dash")
	} else {
		Expect(os.IsNotExist(err)).To(BeTrue())
	}
	return interpreters
}

// Scheduling gates retain the actual public installer and its channel operations.
// per docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:239
// per docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:279
// per docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:292
func bootstrapWatcherCase(interpreter, stage string) {
	GinkgoHelper()
	readerGate := stage == "queued" || stage == "joining-hup" || stage == "channel-error"
	f := ordinaryBootstrapSentinel(defaultInitTag, "")
	args := []string{f.installer, "init", "--ref=" + defaultInitTag, "folder with spaces", "--pack", "none", "--", "--go-candidate", "quote ' and \" and $(not-code)", "line one\nline two"}
	if stage == "setup" || stage == "publication" {
		f = handoffBootstrap("Linux", "x86_64", "linux-amd64", "")
		args = []string{f.installer, "upgrade", "--go-candidate=" + handoffVersion, "--dry-run", "--json", "--project-input", "PROJECT_NAME=literal $(not-code) project"}
	} else {
		raw := strings.Replace(string(f.raw), "exit 0\n", "exit 37\n", 1)
		writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+defaultInitTag+"-"+f.target), []byte(raw), 0600)
	}
	bootstrapWatcherSchedule(f, stage)
	if stage != "joined" {
		gh := filepath.Join(f.root, "host tools", "gh")
		data, err := os.ReadFile(gh)
		Expect(err).NotTo(HaveOccurred())
		api := `  api) printf '%s\n' "$HANDOFF_REVISION" ;;`
		Expect(strings.Count(string(data), api)).To(Equal(1))
		writeFixture(gh, []byte(strings.Replace(string(data), api, `  api) printf '%s\n' "$$" > "$HANDOFF_RECORD_DIR/verifier-pid"; IFS= read -r watcher_probe_api <&4 || exit 97; printf '%s\n' "$HANDOFF_REVISION" ;;`, 1)), 0700)
	}
	gateReader, gateRelease, err := os.Pipe()
	Expect(err).NotTo(HaveOccurred())
	defer gateReader.Close()
	defer gateRelease.Close()
	apiReader, apiRelease, err := os.Pipe()
	Expect(err).NotTo(HaveOccurred())
	defer apiReader.Close()
	defer apiRelease.Close()
	outsider := exec.Command("/bin/sleep", "30")
	outsider.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	Expect(outsider.Start()).To(Succeed())
	defer func() { _ = outsider.Process.Kill(); _ = outsider.Wait() }()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := handoffCommand(ctx, f.cwd, f.environment, "", interpreter, args...)
	command.ExtraFiles = []*os.File{gateReader, apiReader}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	Expect(command.Start()).To(Succeed())
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	joined, settled := false, false
	verifierPID, verifierGroup := 0, 0
	var processError error
	group, groupErr := syscall.Getpgid(command.Process.Pid)
	readPID := func(name string) int {
		body, readErr := os.ReadFile(filepath.Join(f.records, name))
		if readErr != nil {
			return 0
		}
		pid, _ := strconv.Atoi(strings.TrimSpace(string(body)))
		return pid
	}
	observeVerifier := func() {
		if verifierPID == 0 {
			verifierPID = readPID("verifier-pid")
		}
		if verifierPID > 1 && verifierGroup == 0 {
			actual, observeErr := syscall.Getpgid(verifierPID)
			if observeErr == nil && actual == verifierPID && actual != syscall.Getpgrp() {
				verifierGroup = actual
			}
		}
	}
	defer func() {
		if settled {
			return
		}
		observeVerifier()
		_ = gateRelease.Close()
		_ = apiRelease.Close()
		if !joined {
			_ = command.Process.Signal(syscall.SIGCONT)
			select {
			case processError = <-finished:
				joined = true
			case <-time.After(3 * time.Second):
			}
		}
		if !joined {
			holdFixtureCleanup(f.root, "forced watcher probe cleanup leaves separate verifier-group custody uncertain")
			_ = command.Process.Signal(syscall.SIGTERM)
			select {
			case processError = <-finished:
				joined = true
			case <-time.After(3 * time.Second):
			}
		}
		if !joined && groupErr == nil && group == command.Process.Pid && command.Process.Signal(syscall.Signal(0)) == nil {
			_, _ = sourceRunnerStopObservedGroup(command.Process.Pid, group)
			select {
			case processError = <-finished:
				joined = true
			case <-time.After(3 * time.Second):
			}
		}
		if !joined || !errors.Is(syscall.Kill(-group, 0), syscall.ESRCH) {
			holdFixtureCleanup(f.root, "watcher probe did not establish complete owned-group absence")
		}
		observeVerifier()
		if verifierPID > 1 && verifierGroup == verifierPID && !errors.Is(syscall.Kill(-verifierGroup, 0), syscall.ESRCH) {
			holdFixtureCleanup(f.root, "watcher failure required separate verifier-group cleanup")
			_, _ = sourceRunnerStopObservedGroup(verifierPID, verifierGroup)
			Eventually(func() error { return syscall.Kill(-verifierGroup, 0) }, time.Second).Should(MatchError(syscall.ESRCH))
		}
		if verifierPID > 1 && !errors.Is(syscall.Kill(verifierPID, 0), syscall.ESRCH) {
			holdFixtureCleanup(f.root, "late verifier identity did not establish complete absence")
		}
		Expect(joined).To(BeTrue(), "owned installer must join during bounded fixture cleanup")
		Eventually(func() error { return syscall.Kill(-group, 0) }, time.Second).Should(MatchError(syscall.ESRCH))
	}()
	Expect(groupErr).NotTo(HaveOccurred())
	Expect(group).To(Equal(command.Process.Pid))
	Expect(gateReader.Close()).To(Succeed())
	Expect(apiReader.Close()).To(Succeed())
	Eventually(func() bool {
		_, readyErr := os.Stat(filepath.Join(f.records, "watcher-ready"))
		return readyErr == nil && (stage == "setup" || stage == "joined" || readPID("verifier-pid") > 1) && (stage != "verifier-publication" || readPID("watcher-pid") > 1)
	}, 3*time.Second, 10*time.Millisecond).Should(BeTrue())
	if stage != "setup" && stage != "joined" {
		verifierPID = readPID("verifier-pid")
		verifierGroup, err = syscall.Getpgid(verifierPID)
		Expect(err).NotTo(HaveOccurred())
		Expect(verifierGroup).To(Equal(verifierPID))
		Expect(verifierGroup).NotTo(Equal(syscall.Getpgrp()))
	}
	watcherPID := readPID("watcher-pid")
	if stage != "setup" {
		Expect(watcherPID).To(BeNumerically(">", 1))
		if stage == "joined" {
			Expect(syscall.Kill(watcherPID, 0)).To(MatchError(syscall.ESRCH), "the late stop must follow actual watcher reaping")
		} else {
			actual, observeErr := syscall.Getpgid(watcherPID)
			Expect(observeErr).NotTo(HaveOccurred())
			Expect(actual).To(Equal(group))
		}
	}
	switch {
	case readerGate:
		Expect(syscall.Kill(watcherPID, syscall.SIGTERM)).To(Succeed())
		Consistently(func() error { return syscall.Kill(watcherPID, 0) }, 100*time.Millisecond, 10*time.Millisecond).Should(Succeed(), "the controlled watcher must really survive TERM before reading")
		if stage != "channel-error" {
			_, err = apiRelease.Write([]byte("continue\n"))
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() bool {
				_, stopErr := os.Stat(filepath.Join(f.records, "watcher-stop-queued"))
				return stopErr == nil
			}, 3*time.Second, 10*time.Millisecond).Should(BeTrue(), "the actual stop must queue before the watcher reads")
		}
		if stage == "joining-hup" {
			Eventually(func() bool {
				_, joinErr := os.Stat(filepath.Join(f.records, "watcher-joining"))
				return joinErr == nil
			}, time.Second, 10*time.Millisecond).Should(BeTrue())
			Expect(command.Process.Signal(syscall.SIGHUP)).To(Succeed())
			Consistently(func() error { return syscall.Kill(watcherPID, 0) }, 100*time.Millisecond, 10*time.Millisecond).Should(Succeed())
		}
	case stage == "verifier-publication":
		Expect(readPID("verifier-admitted-pid")).To(Equal(verifierPID))
		published, readErr := os.ReadFile(filepath.Join(f.records, "verifier-published-child"))
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(published)).To(Equal("\n"), "the actual verifier must start before its supervisor publishes child")
		supervisorPID := readPID("verifier-supervisor-pid")
		actual, observeErr := syscall.Getpgid(supervisorPID)
		Expect(observeErr).NotTo(HaveOccurred())
		Expect(actual).To(Equal(group))
		Expect(command.Process.Signal(syscall.SIGTERM)).To(Succeed())
		Eventually(func() string {
			entered, _ := os.ReadFile(filepath.Join(f.records, "verifier-signal-entered"))
			return string(entered)
		}, 3*time.Second, 10*time.Millisecond).Should(Equal(strconv.Itoa(supervisorPID)+"\t\n"), "acknowledge cancellation while child is unpublished before releasing admission")
	default:
		Expect(command.Process.Signal(syscall.SIGSTOP)).To(Succeed())
		Eventually(func() string {
			state := imageProcess(f.cwd, nil, "/bin/ps", "-o", "stat=", "-p", strconv.Itoa(command.Process.Pid))
			return strings.TrimSpace(state.stdout)
		}, time.Second, 10*time.Millisecond).Should(HavePrefix("T"))
		Expect(command.Process.Signal(syscall.SIGTERM)).To(Succeed())
	}
	_, err = gateRelease.Write([]byte("continue\n"))
	Expect(err).NotTo(HaveOccurred())
	if !readerGate && stage != "verifier-publication" {
		Expect(command.Process.Signal(syscall.SIGCONT)).To(Succeed())
	}
	select {
	case processError = <-finished:
		joined = true
	case <-time.After(5 * time.Second):
	}
	status := 0
	var exited *exec.ExitError
	if errors.As(processError, &exited) {
		status = exited.ExitCode()
	} else {
		Expect(processError).NotTo(HaveOccurred())
	}
	calls, callsErr := os.ReadFile(filepath.Join(f.records, "watcher-numeric-signal"))
	waits, _ := os.ReadFile(filepath.Join(f.records, "watcher-waits"))
	observeVerifier()
	var verifierState string
	if verifierPID > 1 {
		probeCtx, probeCancel := context.WithTimeout(context.Background(), time.Second)
		state, _ := exec.CommandContext(probeCtx, "/bin/ps", "-o", "pid=,ppid=,pgid=,stat=", "-p", strconv.Itoa(verifierPID)).Output() // #nosec G204 -- fixed diagnostic executable/flags; the sole variable argument is a parsed positive fixture PID, never shell text.
		probeCancel()
		verifierState = strings.TrimSpace(string(state))
	}
	AddReportEntry("bootstrap.watcher-control", map[string]any{"interpreter": interpreter, "stage": stage, "watcher_pid": watcherPID, "verifier_pid": verifierPID, "verifier_group": verifierGroup, "verifier_state": verifierState, "joined": joined, "status": status, "wait_statuses": string(waits), "numeric_signal": string(calls), "stderr": stderr.String()}, ReportEntryVisibilityAlways)
	Expect(joined).To(BeTrue(), "actual watcher cleanup must settle within the existing five-second allowance")
	Eventually(func() error { return syscall.Kill(-group, 0) }, time.Second).Should(MatchError(syscall.ESRCH))
	if verifierPID == 0 {
		verifierPID = readPID("verifier-pid")
	}
	if verifierPID > 1 {
		Eventually(func() error { return syscall.Kill(verifierPID, 0) }, time.Second).Should(MatchError(syscall.ESRCH))
	}
	if verifierGroup > 1 {
		Eventually(func() error { return syscall.Kill(-verifierGroup, 0) }, time.Second).Should(MatchError(syscall.ESRCH))
	}
	settled = true
	if watcherPID == 0 {
		watcherPID = readPID("watcher-pid")
	}
	if watcherPID > 1 {
		Expect(syscall.Kill(watcherPID, 0)).To(MatchError(syscall.ESRCH))
		stops, stopErr := os.ReadFile(filepath.Join(f.records, "watcher-stop-count"))
		Expect(stopErr).NotTo(HaveOccurred())
		Expect(strings.Count("\n"+string(stops), "\n"+strconv.Itoa(watcherPID)+"\n")).To(Equal(1), "the first watcher must receive exactly one queued stop")
	}
	Expect(ctx.Err()).NotTo(HaveOccurred())
	Expect(os.IsNotExist(callsErr)).To(BeTrue(), "no numeric watcher signal is permitted: %s", calls)
	Expect(syscall.Kill(outsider.Process.Pid, 0)).To(Succeed(), "watcher cleanup must leave the unrelated process alive")
	switch stage {
	case "queued":
		Expect(status).To(Equal(37), "%s", stderr.String())
		selected, remaining := ordinaryBootstrapOperands(handoffRecorded(f, "bootstrap-argv"))
		Expect(selected).To(Equal(defaultInitTag))
		Expect(remaining).To(Equal(append([]string{"init"}, args[3:]...)))
	case "channel-error":
		Expect(status).To(Equal(125), "%s", stderr.String())
		Expect(stderr.String()).To(ContainSubstring("preserve private cache"))
		_, markerErr := os.Lstat(f.marker)
		Expect(os.IsNotExist(markerErr)).To(BeTrue())
		retained, retainErr := filepath.Glob(filepath.Join(f.root, "factory-release.*"))
		Expect(retainErr).NotTo(HaveOccurred())
		Expect(retained).To(HaveLen(1), "failure must retain its private acquisition evidence")
		return
	default:
		Expect(status).To(Equal(143), "%s", stderr.String())
		_, markerErr := os.Lstat(f.marker)
		Expect(os.IsNotExist(markerErr)).To(BeTrue(), "cancellation before acquisition completes must not execute the downloaded owner")
	}
	ordinaryBootstrapCache(f)
}

// per docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:279
// per docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:292
func bootstrapWatcherSchedule(f handoffBootstrapFixture, stage string) {
	GinkgoHelper()
	data, err := os.ReadFile(f.installer)
	Expect(err).NotTo(HaveOccurred())
	replace := func(old, replacement string) {
		Expect(strings.Count(string(data), old)).To(Equal(1), "exact scheduling anchor: %s", old)
		data = []byte(strings.Replace(string(data), old, replacement, 1))
	}
	// Observe, and refuse to deliver, any invalid numeric watcher signal. Every
	// native target signal still reaches the real shell builtin unchanged.
	start := "  candidate_signal_epoch=0 candidate_control_open=0 candidate_watch_serial=0\n"
	replace(start, start+`  kill() {
    for watcher_probe_operand do
      if [ -n "${candidate_watchdog-}" ] && [ "$watcher_probe_operand" = "$candidate_watchdog" ]; then
        printf '%s\n' "$*" >> "$HANDOFF_RECORD_DIR/watcher-numeric-signal"
        return 0
      fi
    done
    command kill "$@"
  }
`)
	publish := "    candidate_watchdog=$!\n"
	if stage == "publication" {
		replace(publish, `    if [ ! -e "$HANDOFF_RECORD_DIR/watcher-ready" ]; then
      printf '%s\n' "$!" > "$HANDOFF_RECORD_DIR/watcher-pid"
      : > "$HANDOFF_RECORD_DIR/watcher-ready"
      IFS= read -r watcher_probe_gate <&3 || :
    fi
`+publish)
	} else {
		replace(publish, publish+`    if [ ! -e "$HANDOFF_RECORD_DIR/watcher-pid" ]; then printf '%s\n' "$candidate_watchdog" > "$HANDOFF_RECORD_DIR/watcher-pid"; fi
`)
	}
	stop := "      printf '%s\\n' stop >&9 || candidate_stop_status=125\n"
	replace(stop, stop+`      if [ "$candidate_stop_status" -eq 0 ]; then
        printf '%s\n' "$candidate_watchdog" >> "$HANDOFF_RECORD_DIR/watcher-stop-count"
        : > "$HANDOFF_RECORD_DIR/watcher-stop-queued"
      fi
`)
	gate := `    if [ ! -e "$HANDOFF_RECORD_DIR/watcher-ready" ]; then
      : > "$HANDOFF_RECORD_DIR/watcher-ready"
      IFS= read -r watcher_probe_gate <&3 || :
    fi
`
	switch stage {
	case "verifier-publication":
		mode := "      set -m\n"
		replace(mode, mode+`      trap() {
        if [[ -n "$1" ]]; then
          builtin trap "printf \"%s\\t%s\\n\" \"\$\$\" \"\${child-}\" > \"\$HANDOFF_RECORD_DIR/verifier-signal-entered\"; $1" "${@:2}"
        else
          builtin trap "$@"
        fi
      }
`)
		spawn := "      \"$@\" &\n"
		replace(spawn, spawn+`      printf "%s\n" "$$" > "$HANDOFF_RECORD_DIR/verifier-supervisor-pid"
      printf "%s\n" "$!" > "$HANDOFF_RECORD_DIR/verifier-admitted-pid"
      printf "%s\n" "${child-}" > "$HANDOFF_RECORD_DIR/verifier-published-child"
      : > "$HANDOFF_RECORD_DIR/watcher-ready"
      IFS= read -r watcher_probe_gate <&3 || :
`)
	case "setup":
		setup := "    mkfifo \"$candidate_control\" || return 125\n"
		replace(setup, setup+gate)
	case "publication":
		// The shared publication patch above holds before the actual assignment.
	case "queued", "joining-hup", "channel-error":
		pause := "      pause() {\n        status=0\n"
		replace(pause, pause+`        if [[ ! -e "$HANDOFF_RECORD_DIR/watcher-ready" ]]; then
          trap "" TERM
          printf "%s\n" "$$" > "$HANDOFF_RECORD_DIR/watcher-pid"
          : > "$HANDOFF_RECORD_DIR/watcher-ready"
          IFS= read -r watcher_probe_gate <&3 || exit 0
          trap "exit 125" INT TERM HUP
        fi
`)
		if stage == "joining-hup" {
			wait := "      wait \"$1\" 2>/dev/null || candidate_join_status=$?\n"
			replace(wait, "      : > \"$HANDOFF_RECORD_DIR/watcher-joining\"\n"+wait+"      printf '%s\\n' \"$candidate_join_status\" >> \"$HANDOFF_RECORD_DIR/watcher-waits\"\n")
		}
		if stage == "channel-error" {
			restore := "          trap \"exit 125\" INT TERM HUP\n"
			replace(restore, restore+"          exec 9>&-\n")
		}
	case "joined":
		join := "      wait \"$1\" 2>/dev/null || candidate_join_status=$?\n"
		replace(join, join+gate)
	default:
		Fail("unknown watcher scheduling stage: " + stage)
	}
	writeFixture(f.installer, data, 0600)
}
