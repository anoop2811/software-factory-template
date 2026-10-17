package native_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Native inherited terminal job control", func() {
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:157
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:282
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:580
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:634
	It("returns a real shell prompt on Ctrl-Z and resumes stdin through fg beside direct Bash and non-TTY controls", func() {
		temporary := GinkgoT().TempDir()
		root, err := filepath.EvalSymlinks(temporary)
		Expect(err).NotTo(HaveOccurred())
		streams := native.InheritedStreams{}
		stdin, input, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		streams.Stdin = stdin
		streams.Stdout, err = os.CreateTemp(root, "stdout-")
		Expect(err).NotTo(HaveOccurred())
		streams.Stderr, err = os.CreateTemp(root, "stderr-")
		Expect(err).NotTo(HaveOccurred())
		for _, file := range []*os.File{streams.Stdin, streams.Stdout, streams.Stderr} {
			DeferCleanup(file.Close)
		}
		_, err = input.WriteString("literal nonterminal input\n")
		Expect(err).NotTo(HaveOccurred())
		Expect(input.Close()).To(Succeed())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result, err := native.ExecuteInherited(ctx, root, []string{"/bin/bash", "-c", `IFS= read -r value; printf '%s\n%s\n%s\n' "$value" "$1" "$PWD"; printf '%s\n' "$LITERAL_ENV" >&2; exit 7`, "fixture", "literal $(not shell syntax)"}, map[string]string{"LITERAL_ENV": "literal inherited environment"}, streams, 5*time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ExitConfirmed).To(BeTrue())
		Expect(result.ExitCode).NotTo(BeNil())
		Expect(*result.ExitCode).To(Equal(7))
		Expect(result.OwnershipUnconfirmed).To(BeFalse())
		stdout, err := os.ReadFile(streams.Stdout.Name())
		Expect(err).NotTo(HaveOccurred())
		Expect(string(stdout)).To(Equal("literal nonterminal input\nliteral $(not shell syntax)\n" + root + "\n"))
		stderr, err := os.ReadFile(streams.Stderr.Name())
		Expect(err).NotTo(HaveOccurred())
		Expect(string(stderr)).To(Equal("literal inherited environment\n"))

		executable, err := os.Executable()
		Expect(err).NotTo(HaveOccurred())
		terminalCtx, terminalCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer terminalCancel()
		observations := map[string]terminalObservation{}
		for _, control := range []struct {
			label     string
			inherited bool
			restop    bool
		}{
			{"direct", false, false},
			{"inherited", true, false},
			{"direct-restop", false, true},
			{"inherited-restop", true, true},
		} {
			observed, err := observeNativeTerminal(terminalCtx, root, executable, control.inherited, control.restop)
			Expect(err).NotTo(HaveOccurred(), "%s PTY fixture did not complete its owned cleanup: %s", control.label, observed.Transcript)
			observations[control.label] = observed
		}
		output, err := json.Marshal(observations)
		Expect(err).NotTo(HaveOccurred())
		_, _ = fmt.Fprintf(GinkgoWriter, "real PTY observations: %s\n", output)
		Expect(observations).To(HaveKey("direct"))
		Expect(observations).To(HaveKey("inherited"))
		Expect(observations).To(HaveKey("direct-restop"))
		Expect(observations).To(HaveKey("inherited-restop"))
		for _, label := range []string{"direct", "inherited", "direct-restop", "inherited-restop"} {
			observed := observations[label]
			Expect(observed.ChildForeground).To(BeTrue(), "%s child never owned the real terminal: %s", label, observed.Transcript)
			Expect(observed.StopReturnsPrompt).To(BeTrue(), "%s Ctrl-Z left the shell waiting without its prompt: %s", label, observed.Transcript)
			Expect(observed.StopReturnsForeground).To(BeTrue(), "%s Ctrl-Z failed to restore terminal ownership: %s", label, observed.Transcript)
			if strings.HasSuffix(label, "-restop") {
				Expect(observed.FreshStopMarker).To(BeTrue(), "%s actual child did not announce its immediate new SIGSTOP from SIGCONT: %s", label, observed.Transcript)
				Expect(observed.SecondStopReturnsPrompt).To(BeTrue(), "%s a genuine second stop did not return the prompt and shell foreground: %s", label, observed.Transcript)
			}
			Expect(observed.ResumeConsumesInput).To(BeTrue(), "%s fg failed to resume the same child's real stdin: %s", label, observed.Transcript)
			Expect(observed.ExitReturnsPrompt).To(BeTrue(), "%s normal resumed exit did not return terminal ownership: %s", label, observed.Transcript)
		}
	})
})
