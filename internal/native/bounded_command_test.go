package native_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Post-implementation contract qualification; these cases do not claim a new
// pre-implementation RED. The process regressions independently supplied RED.
// per docs/adr/0097-installation-source-image-bundles.md:196
var _ = Describe("Bounded native command contract", func() {
	var root, marker string
	var argv []string
	BeforeEach(func() {
		root = GinkgoT().TempDir()
		marker = filepath.Join(root, "spawned")
		program := filepath.Join(root, "split-streams")
		Expect(os.WriteFile(program, []byte("#!/bin/sh\nprintf spawned > \"$1\"\nprintf '%s' \"$2\"\nprintf '%s' \"$3\" >&2\n"), 0600)).To(Succeed())
		Expect(os.Chmod(program, 0700)).To(Succeed())
		argv = []string{program, marker, "O", "ERR!"}
	})
	assertGone := func(result native.CommandResult) {
		GinkgoHelper()
		Expect(result.ProcessPID).To(BeNumerically(">", 0))
		Expect(syscall.Kill(result.ProcessPID, 0)).To(Equal(syscall.ESRCH))
		Expect(syscall.Kill(-result.ProcessPID, 0)).To(Equal(syscall.ESRCH), "the actual owned process group must be gone")
	}
	positive := func(stdoutLimit, stderrLimit int) {
		GinkgoHelper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result, err := native.ExecuteCommandBounded(ctx, root, argv, map[string]string{}, stdoutLimit, stderrLimit)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Outcome).To(Equal("completed"))
		Expect(string(result.Stdout)).To(Equal("O"))
		Expect(string(result.Stderr)).To(Equal("ERR!"))
		Expect(result.ExitConfirmed).To(BeTrue())
		Expect(result.ExitCode).NotTo(BeNil())
		Expect(*result.ExitCode).To(BeZero())
		data, err := os.ReadFile(marker)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("spawned"))
		assertGone(result)
		Expect(result.OwnershipUnconfirmed).To(BeFalse())
	}

	// per docs/adr/0097-installation-source-image-bundles.md:205
	It("captures both channels exactly at their different declared boundaries", func() {
		positive(1, 4)
	})

	// per docs/adr/0097-installation-source-image-bundles.md:198
	It("accepts both ceiling values without allocating ceiling-sized fixture output", func() {
		positive(16<<20, 16<<20)
	})

	// per docs/adr/0097-installation-source-image-bundles.md:198
	DescribeTable("refuses invalid independent channel limits before spawning", func(channel string, invalid int) {
		positive(1, 4)
		Expect(os.Remove(marker)).To(Succeed())
		stdoutLimit, stderrLimit := 1, 4
		if channel == "stdout" {
			stdoutLimit = invalid
		} else {
			stderrLimit = invalid
		}
		result, err := native.ExecuteCommandBounded(context.Background(), root, argv, map[string]string{}, stdoutLimit, stderrLimit)
		Expect(err).To(HaveOccurred())
		Expect(result.ProcessPID).To(BeZero())
		Expect(result.Stdout).To(BeEmpty())
		Expect(result.Stderr).To(BeEmpty())
		_, err = os.Lstat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue(), "invalid limits must not execute the fixture")
	}, Entry("zero stdout", "stdout", 0), Entry("negative stdout", "stdout", -1), Entry("stdout above native ceiling", "stdout", (16<<20)+1), Entry("zero stderr", "stderr", 0), Entry("negative stderr", "stderr", -1), Entry("stderr above native ceiling", "stderr", (16<<20)+1))

	// per docs/adr/0097-installation-source-image-bundles.md:208
	// per docs/adr/0097-installation-source-image-bundles.md:209
	// per docs/adr/0097-installation-source-image-bundles.md:223
	// per docs/adr/0097-installation-source-image-bundles.md:227
	DescribeTable("reports one-channel overflow explicitly and cleans actual process ownership", func(channel string) {
		positive(1, 4)
		Expect(os.Remove(marker)).To(Succeed())
		if channel == "stdout" {
			argv[2] += "O"
		} else {
			argv[3] += "!"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result, err := native.ExecuteCommandBounded(ctx, root, argv, map[string]string{}, 1, 4)
		processErr := err
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("limit"))
		Expect(result.Outcome).To(Equal("output_limit"))
		Expect(len(result.Stdout)).To(BeNumerically("<=", 1))
		Expect(len(result.Stderr)).To(BeNumerically("<=", 4))
		data, err := os.ReadFile(marker)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("spawned"), "overflow must be observed after actual fixture execution")
		assertGone(result)
		if result.OwnershipUnconfirmed {
			var ownership *native.OwnershipError
			Expect(errors.As(processErr, &ownership)).To(BeTrue(), "preserve conservative uncertainty even if later OS observation establishes group disappearance")
			Expect(ownership.ProcessPID).To(Equal(result.ProcessPID))
		}
	}, Entry("stdout one byte over", "stdout"), Entry("stderr one byte over", "stderr"))

	// per docs/adr/0097-installation-source-image-bundles.md:199
	// per docs/adr/0097-installation-source-image-bundles.md:209
	It("preserves an already-cancelled parent without launching the same valid fixture", func() {
		positive(1, 4)
		Expect(os.Remove(marker)).To(Succeed())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result, err := native.ExecuteCommandBounded(ctx, root, argv, map[string]string{}, 1, 4)
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		Expect(result.ProcessPID).To(BeZero())
		_, err = os.Lstat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})
