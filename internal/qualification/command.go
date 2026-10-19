package qualification

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// Run emits one plan for Make; worker settings never become shell source.
// docs/adr/0104-adaptive-qualification-workers.md:36.
func Run(args []string, environment map[string]string, stdout, stderr io.Writer) int {
	requested, present := environment["GO_RUNTIME_TEST_PROCS"]
	if !present {
		requested = "auto"
	}
	maximum, present := environment["GO_RUNTIME_TEST_MAX_PROCS"]
	if !present {
		maximum = "4"
	}
	status := 2
	command := &cobra.Command{Use: "worker-count", Short: "Select repository qualification workers", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			// Refuse malformed settings before reading any resource files.
			if _, err := Select(requested, maximum, Resources{}); err != nil {
				return err
			}
			plan, err := Select(requested, maximum, Detect())
			if err != nil {
				return err
			}
			status = 1
			// Reasons are fixed probe/policy messages, never raw OS paths or input text.
			if _, err = fmt.Fprintf(stderr, "go-runtime-workers: base=%d handoff=%d cpus=%d memory_bytes=%d cap=%s reason=%s\n", plan.Base, plan.Handoff, plan.CPUs, plan.MemoryBytes, maximum, plan.Note); err != nil {
				return err
			}
			_, err = fmt.Fprintf(stdout, "%d %d\n", plan.Base, plan.Handoff)
			return err
		},
	}
	command.SetArgs(append([]string{}, args...))
	command.SetOut(stdout)
	command.SetErr(stderr)
	if err := command.Execute(); err != nil {
		_, _ = fmt.Fprintln(stderr, "go-runtime-workers:", err)
		return status
	}
	return 0
}
