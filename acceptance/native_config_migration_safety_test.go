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

func migrationNoTemporaryFiles(cwd string) {
	GinkgoHelper()
	entries, err := os.ReadDir(cwd)
	Expect(err).NotTo(HaveOccurred())
	for _, entry := range entries {
		Expect(entry.Name()).NotTo(HavePrefix(".factory-config-"))
	}
}

var _ = Describe("Native Go config migration safety", func() {
	// per docs/adr/0090-go-native-config-migration.md:23
	It("returns a successful no-op in a read-only project when there is no legacy file", func() {
		root, cwd, environment := configMigrationFixture()
		Expect(os.Remove(filepath.Join(cwd, "factory.config"))).To(Succeed())
		Expect(os.Chmod(cwd, 0500)).To(Succeed())
		DeferCleanup(func() { Expect(os.Chmod(cwd, 0700)).To(Succeed()) })
		before := nativeInitArtifacts(cwd)
		out := configMigrationRun(root, cwd, environment)
		Expect(out).To(Equal(cliResult{"factory migrate-config: no factory.config — nothing to migrate.\n", "", 0}))
		Expect(nativeInitArtifacts(cwd)).To(Equal(before))
	})

	// per docs/adr/0090-go-native-config-migration.md:26
	It("does not validate an unused override when there is no legacy file", func() {
		root, cwd, environment := configMigrationFixture()
		Expect(os.Remove(filepath.Join(cwd, "factory.config"))).To(Succeed())
		before := nativeInitArtifacts(cwd)
		out := configMigrationRun(root, cwd, append(environment, "FACTORY_CONFIG=missing-override"))
		Expect(out).To(Equal(cliResult{"factory migrate-config: no factory.config — nothing to migrate.\n", "", 0}))
		Expect(nativeInitArtifacts(cwd)).To(Equal(before))
	})

	// per docs/adr/0090-go-native-config-migration.md:23
	It("requires YAML before accepting missing legacy input as a no-op", func() {
		root, cwd, environment := configMigrationFixture()
		Expect(os.Remove(filepath.Join(cwd, "factory.yaml"))).To(Succeed())
		Expect(os.Remove(filepath.Join(cwd, "factory.config"))).To(Succeed())
		out := configMigrationRun(root, cwd, environment)
		Expect(out.status).To(Equal(1), "%+v", out)
		Expect(out.stderr).To(ContainSubstring("no factory.yaml here"))
		Expect(out.stdout).To(BeEmpty())
		migrationNoTemporaryFiles(cwd)
	})

	// per docs/adr/0090-go-native-config-migration.md:50
	DescribeTable("validates the entire plan before applying an earlier valid setting", func(kind string, dry bool) {
		root, cwd, environment := configMigrationFixture()
		yaml := "cost_profile: mine\nreview_model:\n"
		legacy := "FIRST=valid\nLATE='bad\"quote'\n"
		switch kind {
		case "carriage return":
			legacy = "FIRST=valid\nLATE='bad\rvalue'\n"
		case "legacy NUL":
			legacy = "FIRST=valid\nLATE=bad\x00value\n"
		case "YAML NUL":
			yaml += "other: bad\x00value\n"
		}
		writeFixture(filepath.Join(cwd, "factory.yaml"), []byte(yaml), 0600)
		writeFixture(filepath.Join(cwd, "factory.config"), []byte(legacy), 0600)
		before := nativeInitArtifacts(cwd)
		var args []string
		if dry {
			args = []string{"--dry-run"}
		}
		out := configMigrationRun(root, cwd, environment, args...)
		Expect(out.status).To(Equal(1), "%+v", out)
		Expect(out.stderr).NotTo(BeEmpty())
		Expect(nativeInitArtifacts(cwd)).To(Equal(before))
	}, Entry("late quote apply", "quote", false), Entry("late quote preview", "quote", true), Entry("late carriage return", "carriage return", false), Entry("legacy NUL", "legacy NUL", false), Entry("YAML NUL", "YAML NUL", false))

	// per docs/adr/0090-go-native-config-migration.md:26
	DescribeTable("refuses an override that identifies another inode without cleaning symlink parent components", func(lexical, dry bool) {
		root, cwd, environment := configMigrationFixture()
		outside := filepath.Join(root, "outside")
		outsidePath := filepath.Join(outside, "factory.yaml")
		writeFixture(outsidePath, []byte("outside: KEEP\n"), 0600)
		override := outsidePath
		if lexical {
			Expect(os.Mkdir(filepath.Join(outside, "child"), 0700)).To(Succeed())
			Expect(os.Symlink(filepath.Join(outside, "child"), filepath.Join(cwd, "link"))).To(Succeed())
			override = "link/../factory.yaml"
		}
		before := nativeInitArtifacts(cwd)
		var args []string
		if dry {
			args = []string{"--dry-run"}
		}
		out := configMigrationRun(root, cwd, append(environment, "FACTORY_CONFIG="+override), args...)
		Expect(out.status).To(Equal(1), "%+v", out)
		Expect(nativeInitArtifacts(cwd)).To(Equal(before))
		data, err := os.ReadFile(outsidePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("outside: KEEP\n"))
	}, Entry("external apply", false, false), Entry("external preview", false, true), Entry("lexical symlink apply", true, false), Entry("lexical symlink preview", true, true))

	// per docs/adr/0090-go-native-config-migration.md:62
	DescribeTable("preserves a preexisting recovery destination before changing YAML", func(kind string) {
		root, cwd, environment := configMigrationFixture()
		path := filepath.Join(cwd, "factory.config.migrated")
		switch kind {
		case "file":
			writeFixture(path, []byte("RECOVERY KEEP"), 0640)
		case "directory":
			Expect(os.Mkdir(path, 0700)).To(Succeed())
			writeFixture(filepath.Join(path, "keep"), []byte("RECOVERY KEEP"), 0600)
		case "dangling symlink":
			Expect(os.Symlink("missing-recovery", path)).To(Succeed())
		}
		info, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		before := nativeInitArtifacts(cwd)
		out := configMigrationRun(root, cwd, environment)
		Expect(out.status).To(Equal(1), "%+v", out)
		Expect(nativeInitArtifacts(cwd)).To(Equal(before))
		after, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(info, after)).To(BeTrue())
	}, Entry("file", "file"), Entry("directory", "directory"), Entry("dangling symlink", "dangling symlink"))

	// per docs/adr/0090-go-native-config-migration.md:59
	DescribeTable("refuses unsafe inputs promptly without following them or creating a backup", func(name, kind string) {
		root, cwd, environment := configMigrationFixture()
		outside := filepath.Join(root, "outside-input")
		writeFixture(outside, []byte("OUTSIDE KEEP"), 0600)
		path := filepath.Join(cwd, name)
		Expect(os.Remove(path)).To(Succeed())
		switch kind {
		case "symlink":
			Expect(os.Symlink(outside, path)).To(Succeed())
		case "hardlink":
			Expect(os.Link(outside, path)).To(Succeed())
		case "FIFO":
			Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
		case "directory":
			Expect(os.Mkdir(path, 0700)).To(Succeed())
		}
		before, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		started := time.Now()
		out := configMigrationRun(root, cwd, environment)
		Expect(time.Since(started)).To(BeNumerically("<", 5*time.Second))
		Expect(out.status).To(Equal(1), "%+v", out)
		after, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		data, err := os.ReadFile(outside)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("OUTSIDE KEEP"))
		_, err = os.Lstat(filepath.Join(cwd, "factory.config.migrated"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		other := "factory.yaml"
		want := "cost_profile: mine\nreview_model:\n"
		if name == other {
			other, want = "factory.config", "COST_PROFILE=legacy\nREVIEW_MODEL=chosen\nEXTRA=value\n"
		}
		data, err = os.ReadFile(filepath.Join(cwd, other))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal(want))
		migrationNoTemporaryFiles(cwd)
	}, Entry("YAML symlink", "factory.yaml", "symlink"), Entry("YAML hardlink", "factory.yaml", "hardlink"), Entry("YAML FIFO", "factory.yaml", "FIFO"), Entry("YAML directory", "factory.yaml", "directory"), Entry("legacy symlink", "factory.config", "symlink"), Entry("legacy hardlink", "factory.config", "hardlink"), Entry("legacy FIFO", "factory.config", "FIFO"), Entry("legacy directory", "factory.config", "directory"))

	// per docs/adr/0090-go-native-config-migration.md:60
	DescribeTable("permits inspection of read-only YAML but refuses to apply", func(dry bool) {
		root, cwd, environment := configMigrationFixture()
		Expect(os.Chmod(filepath.Join(cwd, "factory.yaml"), 0400)).To(Succeed())
		before := nativeInitArtifacts(cwd)
		var args []string
		if dry {
			args = []string{"--dry-run"}
		}
		out := configMigrationRun(root, cwd, environment, args...)
		if dry {
			Expect(out.status).To(BeZero(), "%+v", out)
			Expect(out.stdout).To(ContainSubstring("would add"))
		} else {
			Expect(out.status).To(Equal(1), "%+v", out)
		}
		Expect(nativeInitArtifacts(cwd)).To(Equal(before))
	}, Entry("preview", true), Entry("apply", false))

	// per docs/adr/0090-go-native-config-migration.md:53
	DescribeTable("bounds inputs, settings and the completed YAML before changing files", func(kind string) {
		root, cwd, environment := configMigrationFixture()
		var args []string
		switch kind {
		case "legacy input":
			writeFixture(filepath.Join(cwd, "factory.config"), []byte(strings.Repeat("x", (16<<20)+1)), 0600)
		case "YAML input":
			writeFixture(filepath.Join(cwd, "factory.yaml"), []byte(strings.Repeat("x", (16<<20)+1)), 0600)
		case "settings":
			writeFixture(filepath.Join(cwd, "factory.config"), []byte(strings.Repeat("COST_PROFILE=legacy\n", 4097)), 0600)
		case "result":
			writeFixture(filepath.Join(cwd, "factory.yaml"), []byte("#"+strings.Repeat("x", (16<<20)-3)+"\n"), 0600)
		case "output":
			// 4096 valid settings and input below 16 MiB; only rendered output exceeds it.
			writeFixture(filepath.Join(cwd, "factory.config"), []byte(strings.Repeat("A="+strings.Repeat("v", 4092)+"\n", 4096)), 0600)
			args = []string{"--dry-run"}
		}
		yaml, err := os.ReadFile(filepath.Join(cwd, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		legacy, err := os.ReadFile(filepath.Join(cwd, "factory.config"))
		Expect(err).NotTo(HaveOccurred())
		out := configMigrationRun(root, cwd, environment, args...)
		Expect(out.status).To(Equal(1), "kind=%s stderr=%s", kind, out.stderr)
		if kind == "output" {
			Expect(out.stderr).To(ContainSubstring("output exceeds 16 MiB"))
		}
		for name, want := range map[string][]byte{"factory.yaml": yaml, "factory.config": legacy} {
			data, err := os.ReadFile(filepath.Join(cwd, name))
			Expect(err).NotTo(HaveOccurred())
			Expect(bytes.Equal(data, want)).To(BeTrue(), "%s changed", name)
		}
		_, err = os.Lstat(filepath.Join(cwd, "factory.config.migrated"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		migrationNoTemporaryFiles(cwd)
	}, Entry("legacy input", "legacy input"), Entry("YAML input", "YAML input"), Entry("parsed settings", "settings"), Entry("resulting YAML", "result"), Entry("rendered preview output", "output"))

	// per docs/adr/0090-go-native-config-migration.md:79
	DescribeTable("reports a closed stdout without undoing a completed migration", func(dry bool) {
		root, cwd, environment := configMigrationFixture()
		before := nativeInitArtifacts(cwd)
		read, write, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		Expect(read.Close()).To(Succeed())
		DeferCleanup(write.Close)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		DeferCleanup(cancel)
		args := []string{"migrate-config"}
		if dry {
			args = append(args, "--dry-run")
		}
		command := exec.CommandContext(ctx, filepath.Join(root, "factory"), args...) // #nosec G204 -- compiled fixture and fixed arguments.
		command.Dir, command.Env, command.Stdout = cwd, environment, write
		var stderr bytes.Buffer
		command.Stderr = &stderr
		err = command.Run()
		Expect(ctx.Err()).NotTo(HaveOccurred())
		var exited *exec.ExitError
		Expect(errors.As(err, &exited)).To(BeTrue())
		Expect(exited.ExitCode()).To(Equal(1))
		Expect(stderr.String()).To(ContainSubstring("output"))
		if dry {
			Expect(nativeInitArtifacts(cwd)).To(Equal(before))
		} else {
			data, err := os.ReadFile(filepath.Join(cwd, "factory.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("config_migrated: \"yes\""))
			_, err = os.Stat(filepath.Join(cwd, "factory.config.migrated"))
			Expect(err).NotTo(HaveOccurred())
			_, err = os.Stat(filepath.Join(cwd, "factory.config"))
			Expect(os.IsNotExist(err)).To(BeTrue())
			migrationNoTemporaryFiles(cwd)
		}
	}, Entry("preview", true), Entry("apply", false))

	// per docs/adr/0090-go-native-config-migration.md:20
	DescribeTable("cancels the observed Git discovery child before changing configuration", func(signal syscall.Signal) {
		root, cwd, environment := configMigrationFixture()
		marker := filepath.Join(root, "git-ready")
		bin := filepath.Join(root, "blocked-git")
		writeFixture(filepath.Join(bin, "git"), []byte("#!/bin/sh\nprintf '%s' \"$$\" > \"$MIGRATION_READY\"\nexec /bin/sleep 30\n"), 0700)
		before := nativeInitArtifacts(cwd)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		command := exec.CommandContext(ctx, filepath.Join(root, "factory"), "migrate-config") // #nosec G204 -- compiled test fixture.
		command.Dir, command.Env = cwd, append(environment, "PATH="+bin, "MIGRATION_READY="+marker)
		var stderr bytes.Buffer
		command.Stderr = &stderr
		Expect(command.Start()).To(Succeed())
		done := make(chan error, 1)
		go func() { defer close(done); done <- command.Wait() }()
		pid := 0
		DeferCleanup(func() {
			cancel()
			Eventually(done, 5*time.Second).Should(BeClosed())
			if pid > 0 {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			}
		})
		Eventually(func() bool {
			data, err := os.ReadFile(marker)
			if err != nil {
				return false
			}
			pid, err = strconv.Atoi(string(data))
			return err == nil && pid > 0
		}, 3*time.Second).Should(BeTrue())
		Expect(command.Process.Signal(signal)).To(Succeed())
		var waitErr error
		Eventually(done, 7*time.Second).Should(Receive(&waitErr))
		var exited *exec.ExitError
		Expect(errors.As(waitErr, &exited)).To(BeTrue())
		Expect(exited.ExitCode()).To(Equal(1))
		Expect(strings.ToLower(stderr.String())).To(ContainSubstring("cancel"))
		Expect(syscall.Kill(pid, 0)).To(Equal(syscall.ESRCH))
		pid = 0
		Expect(nativeInitArtifacts(cwd)).To(Equal(before))
	}, Entry("SIGINT", syscall.SIGINT), Entry("SIGTERM", syscall.SIGTERM))
})
