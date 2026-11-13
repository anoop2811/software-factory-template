package acceptance_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Add scheduling cost only to standalone sleeps in the named private cleanup function.
// per docs/adr/0133-go-source-evaluation-runners.md:256
func sourceDelayedCleanupSleeps(launcher []byte, function, receipt string, delay time.Duration) ([]byte, int) {
	GinkgoHelper()
	anchor := []byte(function + "() {\n")
	Expect(bytes.Count(launcher, anchor)).To(Equal(1))
	start := bytes.Index(launcher, anchor)
	end := bytes.Index(launcher[start:], []byte("\n}\n"))
	Expect(end).To(BeNumerically(">", 0))
	end += start
	if function == "source_stop_build" {
		// Delay the original signal-cleanup polls, preserving the new read-only window.
		// per docs/adr/0150-bounded-source-group-reobservation.md:30
		loop := []byte("  for source_signal in \"$1\" KILL; do\n")
		Expect(bytes.Count(launcher[start:end], loop)).To(Equal(1))
		start += bytes.Index(launcher[start:end], loop)
	}
	var delayed strings.Builder
	polls := 0
	for _, line := range strings.SplitAfter(string(launcher[start:end]), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "/bin/sleep ") {
			Expect(strings.Fields(line)).To(HaveLen(2), "the fixture must identify a standalone cleanup sleep")
			polls++
			fmt.Fprintf(&delayed, "        printf 'delay\\n' >> \"$%s\"\n        /bin/sleep %.3f\n", receipt, delay.Seconds())
		}
		delayed.WriteString(line)
	}
	return []byte(string(launcher[:start]) + delayed.String() + string(launcher[end:])), polls
}

// Observe only entered TERM/EXIT handlers in this evaluator's private launcher.
// Spawn, admission, job, reap and wait statements remain uninstrumented.
// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:330
func sourceAbsentObservedLauncher(launcher []byte) []byte {
	GinkgoHelper()
	marker := func(phase, indent string) string {
		return indent + "builtin printf 'phase=%s parent=%s child=%s building=%s compiler=%s source_status=%s\\n' '" + phase + "' \"$$\" \"${SOURCE_CHILD:-0}\" \"${SOURCE_BUILDING:-0}\" \"${compiler_pid:-0}\" \"${SOURCE_STATUS:-0}\" 2>/dev/null >> \"$SOURCE_LAUNCHER_PHASE_TRACE\" || :\n"
	}
	around := func(anchor, before, after string) {
		Expect(bytes.Count(launcher, []byte(anchor))).To(Equal(1), "private diagnostic anchor %q", anchor)
		launcher = bytes.Replace(launcher, []byte(anchor), []byte(before+anchor+after), 1)
	}
	around("source_cancel() {\n", "", marker("TERM-handler-entry", "  "))
	around("source_cleanup() {\n", "", marker("EXIT-cleanup-entry", "  "))
	return launcher
}

// Missing rows or probe errors are data, never process-absence evidence.
// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:331
func sourceAbsentBeforeReleaseObservation(trace string, pids ...int) map[string]any {
	observation := map[string]any{"fixture_pids": pids, "trace_best_effort": true, "trace_read_at": time.Now().UTC().Format(time.RFC3339Nano)}
	body, err := os.ReadFile(trace) // #nosec G703 -- fixed private cwd trace owned by this evaluator; only the instrumented private launcher appends phase labels and numeric state.
	if err != nil {
		observation["trace_error"] = err.Error()
	} else {
		observation["phase_trace"] = string(body)
	}
	selectors := make([]string, 0, len(pids))
	for _, pid := range pids {
		selectors = append(selectors, strconv.Itoa(pid))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/ps", "-o", "pid=,ppid=,pgid=,stat=,wchan=", "-p", strings.Join(selectors, ",")) // #nosec G204 G702 -- fixed executable/columns and the three evaluator-owned numeric process IDs; no argv or environment is requested.
	command.Env = append(os.Environ(), "LC_ALL=C")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	observation["ps_started_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	err = command.Run()
	observation["ps_finished_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	observation["ps_columns"] = "PID PPID PGID STAT WCHAN"
	observation["ps_stdout"], observation["ps_stderr"] = stdout.String(), stderr.String()
	if err != nil {
		observation["ps_error"] = fmt.Sprint(err)
	}
	if ctx.Err() != nil {
		observation["ps_context_error"] = ctx.Err().Error()
	}
	return observation
}
