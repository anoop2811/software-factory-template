package recoverycmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anoop2811/software-factory-template/internal/assessment"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

// per docs/adr/0091-durable-local-recovery-creation.md:141
func TestRecoveryCreation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Durable recovery command boundaries")
}

type recoverySink struct {
	bytes.Buffer
	kind    string
	flushes int
	cancel  context.CancelFunc
}

func (w *recoverySink) Write(data []byte) (int, error) {
	if w.kind == "write" {
		return 0, errors.New("PRIVATE_RECOVERY_SINK_FAILURE")
	}
	if w.kind == "short" {
		return w.Buffer.Write(data[:len(data)-1])
	}
	n, err := w.Buffer.Write(data)
	if w.cancel != nil {
		w.cancel()
	}
	return n, err
}

func (w *recoverySink) Flush() error {
	w.flushes++
	if w.kind == "flush" {
		return errors.New("PRIVATE_RECOVERY_FLUSH_FAILURE")
	}
	return nil
}

func recoveryCommandFixture() (string, []string, map[string]string) {
	GinkgoHelper()
	data, err := exec.Command("git", "show", "c8f8d34edbc14df5655fcbf0aab7eea46ced0295:scripts/factory-budget.sh").Output() // #nosec G204 -- fixed immutable source reference, no shell.
	Expect(err).NotTo(HaveOccurred())
	previous, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	root := GinkgoT().TempDir()
	root, err = filepath.EvalSymlinks(root)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.Chmod(root, 0700)).To(Succeed())
	Expect(os.Mkdir(filepath.Join(root, "scripts"), 0700)).To(Succeed())
	path := filepath.Join(root, "scripts/factory-budget.sh")
	Expect(os.WriteFile(path, data, 0600)).To(Succeed())
	Expect(os.Chmod(path, 0755)).To(Succeed())
	environment := map[string]string{"PATH": os.Getenv("PATH"), "HOME": root}
	for _, args := range [][]string{{"init", "-q"}, {"add", "--", "scripts"}} {
		command := exec.Command("git", args...) // #nosec G204 -- fixture-local literal Git operations, no shell.
		command.Dir = root
		command.Env = []string{"PATH=" + environment["PATH"], "HOME=" + root, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull}
		output, commandErr := command.CombinedOutput()
		Expect(commandErr).NotTo(HaveOccurred(), "%s", output)
	}
	// per docs/adr/0091-durable-local-recovery-creation.md:76
	// per docs/adr/0091-durable-local-recovery-creation.md:200
	for _, relative := range []string{".git", ".git/info", ".git/info/exclude", ".git/config", ".git/HEAD", ".git/index"} {
		mode := os.FileMode(0600)
		if relative == ".git" || relative == ".git/info" {
			mode = 0700
		}
		path := filepath.Join(root, relative)
		Expect(os.Chmod(path, mode)).To(Succeed())
		info, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(mode), "positive fixture must retain trusted Git metadata")
	}
	proposal, err := assessment.ProposeAdoption(context.Background(), root, []string{"scripts/factory-budget.sh"})
	Expect(err).NotTo(HaveOccurred())
	Expect(os.Chdir(root)).To(Succeed())
	DeferCleanup(func() { Expect(os.Chdir(previous)).To(Succeed()) })
	args := []string{"--create-backup", "--migration-id", "command-set", "--target-revision", strings.Repeat("a", 40), "--adopt-path", "scripts/factory-budget.sh", "--confirm-adoption", proposal.ProposalDigest}
	return root, args, environment
}

var _ = Describe("Recovery creation public output failures", func() {
	// per docs/adr/0091-durable-local-recovery-creation.md:57
	DescribeTable("uses operational status for a failed diagnostic stream even on invalid requests", func(kind string) {
		var stdout bytes.Buffer
		stderr := &recoverySink{kind: kind}
		Expect(Run(context.Background(), []string{"--create-backup"}, nil, &stdout, stderr)).To(Equal(1))
		Expect(stdout.Len()).To(BeZero())
		Expect(stderr.String()).NotTo(ContainSubstring("PRIVATE_"))
	}, Entry("diagnostic write error", "write"), Entry("diagnostic short write", "short"), Entry("diagnostic flush error", "flush"))

	// per docs/adr/0091-durable-local-recovery-creation.md:57
	It("cancels a blocked diagnostic pipe instead of hanging on invalid arguments", func() {
		readEnd, writeEnd, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(readEnd.Close)
		DeferCleanup(writeEnd.Close)
		writeDescriptor := int(writeEnd.Fd())
		Expect(unix.SetNonblock(writeDescriptor, true)).To(Succeed())
		padding := bytes.Repeat([]byte{'x'}, 4096)
		for {
			_, fillErr := unix.Write(writeDescriptor, padding)
			if errors.Is(fillErr, unix.EAGAIN) || errors.Is(fillErr, unix.EWOULDBLOCK) {
				break
			}
			Expect(fillErr).NotTo(HaveOccurred())
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		status := make(chan int, 1)
		go func() { status <- Run(ctx, []string{"--create-backup"}, nil, &bytes.Buffer{}, writeEnd) }()
		Eventually(status, 2*time.Second).Should(Receive(Equal(1)))
	})

	// per docs/adr/0091-durable-local-recovery-creation.md:57
	// per docs/adr/0091-durable-local-recovery-creation.md:58
	DescribeTable("refuses sink errors with one private diagnostic while preserving completed recovery", func(kind string, json bool) {
		root, args, environment := recoveryCommandFixture()
		before, err := os.ReadFile(filepath.Join(root, "scripts/factory-budget.sh"))
		Expect(err).NotTo(HaveOccurred())
		if json {
			args = append(args, "--json")
		}
		writer := &recoverySink{kind: kind}
		var stderr bytes.Buffer
		Expect(Run(context.Background(), args, environment, writer, &stderr)).To(Equal(1))
		Expect(stderr.String()).To(Equal("factory upgrade: cannot write recovery report; local recovery state may remain and needs inspection\n"))
		Expect(writer.String()).NotTo(ContainSubstring(root))
		Expect(writer.String()).NotTo(ContainSubstring("PRIVATE_"))
		if kind == "write" {
			Expect(writer.Len()).To(BeZero())
		}
		if kind == "flush" {
			Expect(writer.flushes).To(Equal(1))
		}
		inventory, err := assessment.InspectRecovery(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		Expect(inventory.Sets).To(HaveLen(1))
		Expect(inventory.Sets[0].Classification).To(Equal("integrity_checked"))
		Expect(inventory.Restorable).To(BeFalse())
		Expect(inventory.PruneAuthorized).To(BeFalse())
		after, err := os.ReadFile(filepath.Join(root, "scripts/factory-budget.sh"))
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
	}, Entry("JSON write error", "write", true), Entry("JSON short write", "short", true), Entry("JSON flush error", "flush", true), Entry("text write error", "write", false), Entry("text short write", "short", false), Entry("text flush error", "flush", false))

	// per docs/adr/0091-durable-local-recovery-creation.md:57
	It("reports cancellation during output as failure with a recovery inspection warning", func() {
		root, args, environment := recoveryCommandFixture()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		writer := &recoverySink{cancel: cancel}
		var stderr bytes.Buffer
		Expect(Run(ctx, append(args, "--json"), environment, writer, &stderr)).To(Equal(1))
		Expect(stderr.String()).To(Equal("factory upgrade: cannot write recovery report; local recovery state may remain and needs inspection\n"))
		inventory, err := assessment.InspectRecovery(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		Expect(inventory.Sets).To(HaveLen(1))
		Expect(inventory.Sets[0].Classification).To(Equal("integrity_checked"))
	})
})

var _ = Describe("Recovery creation public cancellation", func() {
	// per docs/adr/0091-durable-local-recovery-creation.md:57
	DescribeTable("refuses an already-ended context before persistent changes", func(deadline bool) {
		root, args, environment := recoveryCommandFixture()
		ctx, cancel := context.WithCancel(context.Background())
		message := "factory upgrade: recovery creation canceled\n"
		if deadline {
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), time.Unix(0, 0))
			message = "factory upgrade: recovery creation deadline exceeded\n"
		} else {
			cancel()
		}
		defer cancel()
		var stdout, stderr bytes.Buffer
		Expect(Run(ctx, args, environment, &stdout, &stderr)).To(Equal(1))
		Expect(stdout.Len()).To(BeZero())
		Expect(stderr.String()).To(Equal(message))
		_, err := os.Lstat(filepath.Join(root, ".factory"))
		Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
		_, err = os.Lstat(filepath.Join(root, ".git/info/factory-recovery.lock"))
		Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
	}, Entry("parent cancellation", false), Entry("parent deadline", true))
})
