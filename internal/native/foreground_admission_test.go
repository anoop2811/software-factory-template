package native_test

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Native initial terminal foreground admission", func() {
	// per docs/DECISION_LOG.md:5083
	It("assigns the real child foreground before executable code can first read inherited stdin", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		terminal, err := newNativeTerminal(ctx, GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(terminal.close()).To(Succeed()) })
		_, ready, err := terminal.until(ctx, func(output string) bool { return strings.Contains(output, nativeTerminalPrompt) })
		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeTrue())
		executable, err := os.Executable()
		Expect(err).NotTo(HaveOccurred())
		_, err = terminal.master.WriteString(shellQuote(executable) + " --factory-native-foreground-admission supervisor\n")
		Expect(err).NotTo(HaveOccurred())
		pattern := regexp.MustCompile(`INITIAL_FOREGROUND=(\{[^\r\n]+\})`)
		output, ready, err := terminal.until(ctx, func(output string) bool { return pattern.MatchString(output) })
		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeTrue(), "actual first child foreground snapshot missing: %s", terminal.transcript)
		var record struct {
			PID        int `json:"pid"`
			Group      int `json:"group"`
			Foreground int `json:"foreground"`
			Owner      int `json:"owner"`
			InputFD    int `json:"input_fd"`
			Ctty       int `json:"ctty"`
		}
		Expect(json.Unmarshal([]byte(pattern.FindStringSubmatch(output)[1]), &record)).To(Succeed())
		terminal.groups[record.Group], terminal.groups[record.Owner] = true, true
		AddReportEntry("native-initial-foreground.actual-first-snapshot", record)
		Expect(record.PID).To(BeNumerically(">", 1))
		Expect(record.Group).To(Equal(record.PID))
		Expect(record.InputFD).To(BeNumerically(">", 2))
		Expect(record.Ctty).To(Equal(record.InputFD), "Ctty names the inherited stream's parent descriptor")
		Expect(record.Foreground).To(Equal(record.Group), "first executable terminal observation precedes parent Start callback return")
		_, err = terminal.master.WriteString("initial foreground input\n")
		Expect(err).NotTo(HaveOccurred())
		_, completed, err := terminal.until(ctx, func(output string) bool {
			return strings.Contains(output, "INITIAL_READ=initial foreground input\r\n") && strings.Contains(output, nativeTerminalPrompt) && terminal.foreground() == terminal.shell.Process.Pid
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(completed).To(BeTrue(), "the same initially admitted child must consume real input and restore the shell: %s", terminal.transcript)
	})

	// per docs/DECISION_LOG.md:5092
	It("restores the actual parent foreground after child-side assignment and a real failed exec", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		terminal, err := newNativeTerminal(ctx, GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(terminal.close()).To(Succeed()) })
		_, ready, err := terminal.until(ctx, func(output string) bool { return strings.Contains(output, nativeTerminalPrompt) })
		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeTrue())
		executable, err := os.Executable()
		Expect(err).NotTo(HaveOccurred())
		_, err = terminal.master.WriteString(shellQuote(executable) + " --factory-native-foreground-admission failed-start\n")
		Expect(err).NotTo(HaveOccurred())
		pattern := regexp.MustCompile(`FAILED_START=(\{[^\r\n]+\})`)
		output, ready, err := terminal.until(ctx, func(output string) bool {
			return pattern.MatchString(output) && strings.Contains(output, nativeTerminalPrompt) && terminal.foreground() == terminal.shell.Process.Pid
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeTrue(), "failed real Start must restore the parent and then the interactive shell: %s", terminal.transcript)
		var record struct {
			Before              int  `json:"before"`
			After               int  `json:"after"`
			Parent              int  `json:"parent_group"`
			Starts              int  `json:"start_calls"`
			ForegroundRequested bool `json:"foreground_requested"`
			ActualENOEXEC       bool `json:"actual_enoexec"`
			LaunchError         bool `json:"launch_error"`
		}
		Expect(json.Unmarshal([]byte(pattern.FindStringSubmatch(output)[1]), &record)).To(Succeed())
		AddReportEntry("native-initial-foreground.failed-real-exec", record)
		Expect(record.Starts).To(Equal(1))
		Expect(record.ActualENOEXEC).To(BeTrue())
		Expect(record.LaunchError).To(BeTrue())
		Expect(record.ForegroundRequested).To(BeTrue())
		Expect(record.Before).To(Equal(record.Parent))
		Expect(record.After).To(Equal(record.Parent))
	})

	// per docs/DECISION_LOG.md:5095
	It("preserves the real shell foreground when its caller runs as a background job", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		terminal, err := newNativeTerminal(ctx, GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(terminal.close()).To(Succeed()) })
		_, ready, err := terminal.until(ctx, func(output string) bool { return strings.Contains(output, nativeTerminalPrompt) })
		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeTrue())
		executable, err := os.Executable()
		Expect(err).NotTo(HaveOccurred())
		_, err = terminal.master.WriteString(shellQuote(executable) + " --factory-native-foreground-admission background &\n")
		Expect(err).NotTo(HaveOccurred())
		pattern := regexp.MustCompile(`INITIAL_FOREGROUND=(\{[^\r\n]+\})`)
		output, ready, err := terminal.until(ctx, func(output string) bool { return pattern.MatchString(output) })
		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeTrue(), "background child must report its actual terminal foreground: %s", terminal.transcript)
		var record struct {
			PID        int `json:"pid"`
			Group      int `json:"group"`
			Foreground int `json:"foreground"`
			Owner      int `json:"owner"`
		}
		Expect(json.Unmarshal([]byte(pattern.FindStringSubmatch(output)[1]), &record)).To(Succeed())
		terminal.groups[record.Group], terminal.groups[record.Owner] = true, true
		AddReportEntry("native-initial-foreground.background-caller", record)
		Expect(record.Group).To(Equal(record.PID))
		Expect(record.Foreground).To(Equal(terminal.shell.Process.Pid))
		Expect(record.Owner).NotTo(Equal(record.Foreground))
		_, completed, err := terminal.until(ctx, func(string) bool {
			return strings.Contains(string(terminal.transcript), "\r\nBACKGROUND_COMPLETE\r\n") && terminal.foreground() == terminal.shell.Process.Pid
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(completed).To(BeTrue(), "background caller must finish without taking the shell's terminal: %s", terminal.transcript)
	})

	// per docs/DECISION_LOG.md:5097
	It("restores the saved parent foreground and reaps the admitted child on context cancellation", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		terminal, err := newNativeTerminal(ctx, GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(terminal.close()).To(Succeed()) })
		_, ready, err := terminal.until(ctx, func(output string) bool { return strings.Contains(output, nativeTerminalPrompt) })
		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeTrue())
		executable, err := os.Executable()
		Expect(err).NotTo(HaveOccurred())
		_, err = terminal.master.WriteString(shellQuote(executable) + " --factory-native-foreground-admission cancel\n")
		Expect(err).NotTo(HaveOccurred())
		initial := regexp.MustCompile(`INITIAL_FOREGROUND=(\{[^\r\n]+\})`)
		completed := regexp.MustCompile(`CANCELED_ADMISSION=(\{[^\r\n]+\})`)
		output, ready, err := terminal.until(ctx, func(output string) bool {
			return initial.MatchString(output) && completed.MatchString(output) && strings.Contains(output, nativeTerminalPrompt) && terminal.foreground() == terminal.shell.Process.Pid
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeTrue(), "actual admission/cancellation did not return the real shell: %s", terminal.transcript)
		var admitted struct {
			Group      int `json:"group"`
			Foreground int `json:"foreground"`
		}
		Expect(json.Unmarshal([]byte(initial.FindStringSubmatch(output)[1]), &admitted)).To(Succeed())
		Expect(admitted.Foreground).To(Equal(admitted.Group))
		var record struct {
			After             int    `json:"after"`
			Parent            int    `json:"parent_group"`
			Outcome           string `json:"outcome"`
			NoOwnershipError  bool   `json:"no_ownership_error"`
			ExitConfirmed     bool   `json:"exit_confirmed"`
			ExitCode          *int   `json:"exit_code"`
			ActualChildAbsent bool   `json:"actual_child_absent"`
		}
		Expect(json.Unmarshal([]byte(completed.FindStringSubmatch(output)[1]), &record)).To(Succeed())
		AddReportEntry("native-initial-foreground.canceled-after-admission", record)
		Expect(record.Outcome).To(Equal("interrupted"))
		Expect(record.NoOwnershipError).To(BeTrue())
		Expect(record.ExitConfirmed).To(BeTrue())
		Expect(record.ExitCode).NotTo(BeNil())
		Expect(*record.ExitCode).To(Equal(-int(syscall.SIGKILL)))
		Expect(record.ActualChildAbsent).To(BeTrue())
		Expect(record.After).To(Equal(record.Parent))
	})
})
