package acceptance_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func installationRollbackPreflightSpecs(fixture func(string) installationFixtureData) {
	Describe("completed rollback preflight", func() {
		// per docs/DECISION_LOG.md:9000
		// per docs/DECISION_LOG.md:9006
		DescribeTable("retains a supported retry after a settled legacy prerequisite failure", func(profile string) {
			installed := fixture(profile)
			const migration = "rollback-prerequisite-retry"
			writeFixture(filepath.Join(installed.root, "application.txt"), []byte("application remains operator owned\n"), 0644)
			installationApply(installed, migration)
			before := installationSnapshot(installed.root)
			proposal := installationProposal(installed, "--rollback", migration)
			tools := filepath.Join(filepath.Dir(installed.root), "unavailable legacy tool")
			marker := filepath.Join(tools, "called")
			writeFixture(filepath.Join(tools, "python3"), []byte("#!/bin/sh\nprintf unavailable > \"$ROLLBACK_TOOL_PROBE\"\nexit 127\n"), 0700)
			originalPATH := os.Getenv("PATH")
			GinkgoT().Setenv("ROLLBACK_TOOL_PROBE", marker)
			GinkgoT().Setenv("PATH", tools+string(os.PathListSeparator)+originalPATH)
			failed := installationMutationCommand(installed.driver, installed.root, installed.sentinel, append(installed.arguments(), "--rollback", migration, "--confirm", proposal["proposal_digest"].(string), "--quiescent")...)
			GinkgoT().Setenv("PATH", originalPATH)
			Expect(failed.status).To(Equal(2), "unavailable legacy prerequisite must refuse reverse activation: %+v", failed)
			Expect(failed.stderr).To(ContainSubstring("legacy runtime health"))
			Expect(os.ReadFile(marker)).To(Equal([]byte("unavailable")), "failure must reach the actual historical tool boundary")
			installationRollbackAssetsUnchanged(installed.root, before)
			installationRetryCompletedRollback(installed, migration)
			Expect(os.ReadFile(filepath.Join(installed.root, "application.txt"))).To(Equal([]byte("application remains operator owned\n")))
		}, Entry("v0.1.6", "v0.1.6"), Entry("immutable Bash baseline", "bash-baseline"))

		// per docs/DECISION_LOG.md:9002
		// per docs/DECISION_LOG.md:9007
		It("retains supported recovery after an observed settled reverse-preflight interruption", func() {
			installed := fixture("v0.1.6")
			const migration = "rollback-preflight-interrupted"
			writeFixture(filepath.Join(installed.root, "application.txt"), []byte("application survives interrupted preparation\n"), 0644)
			installationApply(installed, migration)
			before := installationSnapshot(installed.root)
			proposal := installationProposal(installed, "--rollback", migration)
			installationInterruptRollbackPreflight(installed, migration, proposal["proposal_digest"].(string))
			installationRollbackAssetsUnchanged(installed.root, before)
			installationRetryCompletedRollback(installed, migration)
			Expect(os.ReadFile(filepath.Join(installed.root, "application.txt"))).To(Equal([]byte("application survives interrupted preparation\n")))
		})
	})
}

// Failed preparation may retain its own custody evidence, but not alter the
// installed selection, application data, original backups or Git history.
// per docs/DECISION_LOG.md:9008
func installationRollbackAssetsUnchanged(root string, before map[string]string) {
	GinkgoHelper()
	after := installationSnapshot(root)
	for path, value := range before {
		if strings.HasPrefix(path, ".factory/installation-transactions/") {
			continue
		}
		Expect(after[path]).To(Equal(value), "rollback preflight changed existing installation/application data: %s", path)
	}
}

// Accept either retained committed evidence or a supported explicit forward
// recovery. The test never edits the journal, pending marker or backup to retry.
// per docs/DECISION_LOG.md:8998
func installationRetryCompletedRollback(installed installationFixtureData, migration string) {
	GinkgoHelper()
	rollback := append(installed.arguments(), "--rollback", migration)
	fresh := installationCommand(installed.driver, installed.root, installed.sentinel, append(append([]string{}, rollback...), "--dry-run", "--json")...)
	if fresh.status != 0 {
		recoveryArgs := append(installed.arguments(), "--recover", migration, "--direction", "forward")
		preview := installationCommand(installed.driver, installed.root, installed.sentinel, append(append([]string{}, recoveryArgs...), "--dry-run", "--json")...)
		Expect(preview.status).To(Equal(0), "failed reverse preparation must retain a supported recovery route; rollback=%+v recovery=%+v", fresh, preview)
		proposal := installationDecodeProposal(preview)
		out := installationMutationCommand(installed.driver, installed.root, installed.sentinel, append(recoveryArgs, "--confirm", proposal["proposal_digest"].(string), "--quiescent")...)
		Expect(out.status).To(Equal(0), "fresh public recovery failed: %+v", out)
		fresh = installationCommand(installed.driver, installed.root, installed.sentinel, append(append([]string{}, rollback...), "--dry-run", "--json")...)
	}
	Expect(fresh.status).To(Equal(0), "healthy prerequisites must permit fresh rollback consent: %+v", fresh)
	proposal := installationDecodeProposal(fresh)
	out := installationMutationCommand(installed.driver, installed.root, installed.sentinel, append(rollback, "--confirm", proposal["proposal_digest"].(string), "--quiescent")...)
	Expect(out.status).To(Equal(0), "fresh rollback failed after preflight recovery: %+v", out)
	after := installationSnapshot(installed.root)
	for path, value := range installed.before {
		if !strings.HasPrefix(path, ".git/") && path != ".gitignore" {
			Expect(after[path]).To(Equal(value), "legacy entry not restored: %s", path)
		}
	}
	for _, path := range []string{".factory/installation.current", ".factory/bin/factory-runtime", ".factory/runtime-publication.pending"} {
		_, err := os.Lstat(filepath.Join(installed.root, path))
		Expect(os.IsNotExist(err)).To(BeTrue(), "rollback left active Go custody: %s", path)
	}
}

// Observe settled check evidence, then stop and re-read it before the SIGKILL.
// Killing a still-owned check would be a different, correctly uncertain case.
// per docs/DECISION_LOG.md:9003
// per docs/adr/0151-reliable-ci-qualification.md:193
func installationInterruptRollbackPreflight(installed installationFixtureData, migration, digest string) {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	capture := filepath.Join(filepath.Dir(installed.root), "rollback-preflight-output")
	output, err := os.OpenFile(capture, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(output.Close)
	args := append(installed.arguments(), "--rollback", migration, "--confirm", digest, "--quiescent")
	owner, err := startCIInterruptionOwner(ctx, func(childContext context.Context) *exec.Cmd {
		command := exec.CommandContext(childContext, filepath.Join(installed.driver, "factory"), append([]string{"upgrade"}, args...)...) // #nosec G204 -- independently compiled evaluator-owned public CLI and literal fixture operands.
		command.Dir = installed.root
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "FACTORY_BRIDGE_PROTOCOL=") {
				command.Env = append(command.Env, entry)
			}
		}
		command.Stdout, command.Stderr = output, output
		return command
	})
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() {
		result := owner.TerminateAndWait()
		if !result.Joined || result.SignalError != nil {
			holdFixtureCleanup(filepath.Dir(installed.root), "reverse-preflight interruption custody was not confirmed")
		}
		Expect(result.Joined).To(BeTrue())
		Expect(result.SignalError).NotTo(HaveOccurred())
	})
	path := filepath.Join(installed.root, ".factory/installation-transactions", migration+".json")
	settled := func() ([]byte, bool) {
		body, err := os.ReadFile(path)
		var journal struct {
			Phase     string `json:"phase"`
			Direction string `json:"direction"`
			Checks    []struct {
				Phase                string `json:"phase"`
				OwnershipUnconfirmed bool   `json:"ownership_unconfirmed"`
			} `json:"checks"`
		}
		if err != nil || json.Unmarshal(body, &journal) != nil || journal.Direction != "forward" || (journal.Phase != "checking" && journal.Phase != "prepared") || len(journal.Checks) == 0 {
			return nil, false
		}
		for _, check := range journal.Checks {
			if check.Phase != "complete" || check.OwnershipUnconfirmed {
				return nil, false
			}
		}
		return body, true
	}
	var stopped []byte
	Eventually(func() bool {
		exited, exitErr := owner.Exited()
		Expect(exitErr).NotTo(HaveOccurred())
		if exited {
			result, joinErr := owner.JoinExited()
			Expect(joinErr).NotTo(HaveOccurred())
			body, readErr := os.ReadFile(capture)
			Expect(readErr).NotTo(HaveOccurred())
			StopTrying(fmt.Sprintf("rollback completed before a settled interruption witness: %+v, %s", result, body)).Now()
		}
		if _, ready := settled(); !ready {
			return false
		}
		Expect(owner.Signal(syscall.SIGSTOP)).To(Succeed())
		Eventually(func() (bool, error) { return installationProcessStopped(owner.PID()) }, 5*time.Second, time.Millisecond).Should(BeTrue())
		var ready bool
		stopped, ready = settled()
		if !ready {
			Expect(owner.Signal(syscall.SIGCONT)).To(Succeed())
		}
		return ready
	}, 50*time.Second, time.Millisecond).Should(BeTrue(), "must observe completed child checks during reverse preparation")
	result := owner.TerminateAndWait()
	Expect(result.Joined).To(BeTrue())
	Expect(result.SignalError).NotTo(HaveOccurred())
	var exit *exec.ExitError
	Expect(errors.As(result.WaitError, &exit)).To(BeTrue())
	status, ok := exit.Sys().(syscall.WaitStatus)
	Expect(ok).To(BeTrue())
	Expect(status.Signaled()).To(BeTrue())
	Expect(status.Signal()).To(Equal(syscall.SIGKILL))
	Expect(os.ReadFile(path)).To(Equal(stopped), "the observed stopped journal must remain unchanged after SIGKILL")
}
