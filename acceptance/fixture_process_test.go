package acceptance_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Each caller supplies its existing command/environment policy and all borrowed
// fixture roots that an uncertain child could still use.
// per docs/adr/0148-transactional-installed-ci-migration.md:764
func cooperativeFixtureProcess(base, reportName, input string, retainedRoots []string, buildCommand func(context.Context) *exec.Cmd) cliResult {
	GinkgoHelper()
	return cooperativeFixtureProcessContext(context.Background(), base, reportName, input, retainedRoots, buildCommand)
}

// A parent cancellation exposes the real supervisor boundary to independent controls.
// per docs/adr/0151-reliable-ci-qualification.md:139
func cooperativeFixtureProcessContext(parent context.Context, base, reportName, input string, retainedRoots []string, buildCommand func(context.Context) *exec.Cmd) cliResult {
	GinkgoHelper()
	stream := func(name string) *os.File {
		file, err := os.OpenFile(filepath.Join(base, reportName+"-"+name), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(file.Close()).To(Succeed()) })
		return file
	}
	stdin, stdoutFile, stderrFile := stream("stdin"), stream("stdout"), stream("stderr")
	_, err := stdin.WriteString(input)
	Expect(err).NotTo(HaveOccurred())
	_, err = stdin.Seek(0, 0)
	Expect(err).NotTo(HaveOccurred())
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	command := buildCommand(ctx)
	Expect(command.Err).NotTo(HaveOccurred())
	environment := map[string]string{}
	for _, entry := range command.Env {
		key, value, _ := strings.Cut(entry, "=")
		environment[key] = value
	}
	// exec.Command already selected the interpreter against the original caller's
	// PATH. Preserve that choice when the child PATH has fixture-only overrides.
	argv := append([]string{command.Path}, command.Args[1:]...)
	started := time.Now()
	result, runErr := native.ExecuteInheritedCooperative(ctx, command.Dir, argv, environment, native.InheritedStreams{Stdin: stdin, Stdout: stdoutFile, Stderr: stderrFile}, 90*time.Second)
	elapsed, contextErr := time.Since(started), ctx.Err()
	stdout, stdoutErr := os.ReadFile(stdoutFile.Name())
	stderr, stderrErr := os.ReadFile(stderrFile.Name())
	failed := runErr != nil || result.Outcome != "completed" || result.ExitCode == nil || *result.ExitCode != 0
	if contextErr != nil || result.OwnershipUnconfirmed || !result.ExitConfirmed || failed {
		for _, root := range retainedRoots {
			holdFixtureCleanup(root, reportName+": process did not confirm successful cooperative ownership")
		}
	}
	AddReportEntry(reportName, map[string]any{
		"argv": argv, "requested_argv": command.Args, "elapsed_seconds": elapsed.Seconds(), "outcome": result.Outcome,
		"exit_code": result.ExitCode, "pid": result.ProcessPID, "exit_confirmed": result.ExitConfirmed,
		"ownership_unconfirmed": result.OwnershipUnconfirmed, "context_error": fmt.Sprint(contextErr), "run_error": fmt.Sprint(runErr),
		"stdout": string(stdout), "stderr": string(stderr), "stdout_error": fmt.Sprint(stdoutErr), "stderr_error": fmt.Sprint(stderrErr),
	}, ReportEntryVisibilityNever)
	Expect(contextErr).NotTo(HaveOccurred())
	Expect(runErr).NotTo(HaveOccurred())
	Expect(result.OwnershipUnconfirmed).To(BeFalse())
	Expect(result.ExitConfirmed).To(BeTrue())
	Expect(result.Outcome).To(Or(Equal("completed"), Equal("failed")))
	Expect(result.ExitCode).NotTo(BeNil())
	Expect(stdoutErr).NotTo(HaveOccurred())
	Expect(stderrErr).NotTo(HaveOccurred())
	return cliResult{string(stdout), string(stderr), *result.ExitCode}
}

// Successful handoff journeys retain their original command policy and deadline.
// A separate stream directory cannot alter an adopter preservation snapshot.
// per docs/adr/0151-reliable-ci-qualification.md:128
func handoffCooperativeProcess(cwd string, environment map[string]string, input string, retainedRoots []string, program string, args ...string) cliResult {
	GinkgoHelper()
	streams, err := os.MkdirTemp("", "factory-cooperative-handoff-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(cleanupFixtureDirectory, streams)
	roots := append(append([]string{}, retainedRoots...), streams)
	return cooperativeFixtureProcess(streams, "fixture.handoff-owner", input, roots, func(ctx context.Context) *exec.Cmd {
		return handoffCommand(ctx, cwd, environment, input, program, args...)
	})
}
