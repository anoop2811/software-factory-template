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

var _ = Describe("Native Go report safety", func() {
	// per docs/adr/0087-go-native-report.md:31
	It("unlinks only the selected symlink and preserves its target", func() {
		root, cwd, environment := reportFixture()
		target := filepath.Join(cwd, "outside.log")
		writeFixture(target, []byte("keep\n"), 0600)
		link := filepath.Join(cwd, "selected.log")
		Expect(os.Symlink(target, link)).To(Succeed())
		out := reportRun(root, cwd, append(environment, "FACTORY_EVENT_LOG=selected.log"), "--clear")
		Expect(out).To(Equal(cliResult{"factory report: event log cleared.\n", "", 0}))
		_, err := os.Lstat(link)
		Expect(os.IsNotExist(err)).To(BeTrue())
		data, err := os.ReadFile(target)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("keep\n"))
	})
	DescribeTable("preserves directories during best-effort clear", func(populated bool) {
		root, cwd, environment := reportFixture()
		path := filepath.Join(cwd, "events")
		Expect(os.Mkdir(path, 0700)).To(Succeed())
		if populated {
			writeFixture(filepath.Join(path, "KEEP"), []byte("keep"), 0600)
		}
		out := reportRun(root, cwd, append(environment, "FACTORY_EVENT_LOG="+path, "FACTORY_CONFIG=/dev/null"), "--clear")
		Expect(out.status).To(BeZero(), "%+v", out)
		info, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.IsDir()).To(BeTrue())
		if populated {
			data, err := os.ReadFile(filepath.Join(path, "KEEP"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(Equal("keep"))
		}
	}, Entry("empty", false), Entry("nonempty", true))
	// per docs/adr/0087-go-native-report.md:53
	DescribeTable("uses checked decimal estimates", func(value, total string, valid bool) {
		root, cwd, environment := reportFixture()
		writeFixture(filepath.Join(root, ".factory/events.log"), []byte("t\tg\tr\nt\tg\tr\n"), 0600)
		out := reportRun(root, cwd, append(environment, "FACTORY_REVIEW_TOKENS="+value))
		if valid {
			Expect(out.status).To(BeZero(), "%+v", out)
			Expect(out.stdout).To(ContainSubstring("Review-spend avoided:  ~" + total + " tokens"))
		} else {
			Expect(out.status).NotTo(BeZero(), "%+v", out)
			Expect(out.stderr).To(ContainSubstring("estimate"))
		}
	}, Entry("leading zero decimal", "0008", "16", true), Entry("invalid uses default", "3x", "6000", true), Entry("zero", "0", "0", true), Entry("parse overflow", "18446744073709551616", "", false), Entry("product overflow", "18446744073709551615", "", false))
	DescribeTable("rejects unsafe event input without blocking", func(kind string) {
		root, cwd, environment := reportFixture()
		path := filepath.Join(cwd, "events")
		switch kind {
		case "fifo":
			Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
		case "binary":
			writeFixture(path, []byte("t\tg\tr\x00x\n"), 0600)
		case "large":
			writeFixture(path, []byte(strings.Repeat("x", (16<<20)+1)), 0600)
		}
		out := reportRun(root, cwd, append(environment, "FACTORY_EVENT_LOG="+path))
		Expect(out.status).NotTo(BeZero(), "%+v", out)
		Expect(out.stderr).NotTo(BeEmpty())
	}, Entry("FIFO", "fifo"), Entry("NUL", "binary"), Entry("over limit", "large"))
	It("bounds configuration inputs before parsing", func() {
		root, cwd, environment := reportFixture()
		path := filepath.Join(cwd, "config.yaml")
		writeFixture(path, []byte(strings.Repeat("x", (16<<20)+1)), 0600)
		out := reportRun(root, cwd, append(environment, "FACTORY_CONFIG="+path))
		Expect(out.status).NotTo(BeZero(), "%+v", out)
		Expect(out.stderr).To(ContainSubstring("limit"))
	})
	It("bounds hook enumeration including entries that are not gates", func() {
		root, cwd, environment := reportFixture()
		for i := range 4096 {
			writeFixture(filepath.Join(root, "scripts/hooks/entry-"+strconv.Itoa(i)), nil, 0600)
		}
		out := reportRun(root, cwd, environment)
		Expect(out.status).NotTo(BeZero(), "%+v", out)
		Expect(out.stderr).To(ContainSubstring("entry limit"))
	})
	It("counts a newline-bearing hook name once", func() {
		root, cwd, environment := reportFixture()
		writeFixture(filepath.Join(root, "scripts/hooks/two\nlines.sh"), nil, 0600)
		out := reportRun(root, cwd, environment)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("Deterministic gates installed:  2 "))
	})
	DescribeTable("does not traverse a replaced hooks directory", func(link bool) {
		root, cwd, environment := reportFixture()
		path := filepath.Join(root, "scripts/hooks")
		Expect(os.Remove(filepath.Join(path, "one.sh"))).To(Succeed())
		Expect(os.Remove(path)).To(Succeed())
		if link {
			outside := filepath.Join(cwd, "outside-hooks")
			writeFixture(filepath.Join(outside, "outside.sh"), nil, 0600)
			Expect(os.Symlink(outside, path)).To(Succeed())
		} else {
			writeFixture(path, nil, 0600)
		}
		out := reportRun(root, cwd, environment)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("Deterministic gates installed:  0 "))
	}, Entry("symlink", true), Entry("regular file", false))
	It("does not treat a later clear argument as permission to delete", func() {
		root, cwd, environment := reportFixture()
		path := filepath.Join(root, ".factory/events.log")
		writeFixture(path, []byte("t\tg\tr\n"), 0600)
		out := reportRun(root, cwd, environment, "ignored", "--clear")
		Expect(out.status).To(BeZero(), "%+v", out)
		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("t\tg\tr\n"))
	})
	It("reports a closed output pipe as failure", func() {
		root, cwd, environment := reportFixture()
		read, write, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		Expect(read.Close()).To(Succeed())
		DeferCleanup(write.Close)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, filepath.Join(root, "factory"), "report") // #nosec G204 -- compiled fixture.
		command.Dir, command.Env, command.Stdout = cwd, environment, write
		var stderr bytes.Buffer
		command.Stderr = &stderr
		err = command.Run()
		Expect(ctx.Err()).NotTo(HaveOccurred())
		var exited *exec.ExitError
		Expect(errors.As(err, &exited)).To(BeTrue())
		Expect(exited.ExitCode()).To(Equal(1))
		Expect(stderr.String()).To(ContainSubstring("output"))
	})
	DescribeTable("cancels an observed Git discovery child", func(signal syscall.Signal) {
		root, cwd, environment := reportFixture()
		bin := filepath.Join(cwd, "bin")
		marker := filepath.Join(cwd, "git-pid")
		writeFixture(filepath.Join(bin, "git"), []byte("#!/bin/bash\nprintf '%s' \"$$\" > \"$REPORT_PID_MARKER\"\nexec /bin/sleep 30\n"), 0700)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		command := exec.CommandContext(ctx, filepath.Join(root, "factory"), "report") // #nosec G204 -- compiled fixture.
		command.Dir, command.Env = cwd, append(environment, "PATH="+bin, "REPORT_PID_MARKER="+marker)
		var stderr bytes.Buffer
		command.Stderr = &stderr
		Expect(command.Start()).To(Succeed())
		done := make(chan error, 1)
		go func() { defer close(done); done <- command.Wait() }()
		pid := 0
		DeferCleanup(func() {
			if pid > 0 {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			}
			cancel()
			Eventually(done, 3*time.Second).Should(BeClosed())
		})
		var raw []byte
		Eventually(func() error { var err error; raw, err = os.ReadFile(marker); return err }, 5*time.Second).Should(Succeed())
		var err error
		pid, err = strconv.Atoi(string(raw))
		Expect(err).NotTo(HaveOccurred())
		Expect(command.Process.Signal(signal)).To(Succeed())
		var waitErr error
		Eventually(done, 7*time.Second).Should(Receive(&waitErr))
		var exited *exec.ExitError
		Expect(errors.As(waitErr, &exited)).To(BeTrue())
		Expect(exited.ExitCode()).To(Equal(1))
		Expect(stderr.String()).To(ContainSubstring("cancel"))
		Expect(syscall.Kill(pid, 0)).To(Equal(syscall.ESRCH))
		pid = 0
	}, Entry("SIGINT", syscall.SIGINT), Entry("SIGTERM", syscall.SIGTERM))
})
