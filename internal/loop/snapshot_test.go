package loop

import (
	"bytes"
	"context"
	"fmt"
	"github.com/anoop2811/software-factory-template/internal/budget"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLoopSnapshot(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Loop snapshot collaborator acceptance")
}
func loopTestEnvironment() map[string]string {
	environment := map[string]string{}
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok && !strings.HasPrefix(key, "FACTORY_") {
			environment[key] = value
		}
	}
	return environment
}
func snapshotFixture() (string, Config, map[string]string) {
	GinkgoHelper()
	root := GinkgoT().TempDir()
	environment := loopTestEnvironment()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "Fixture"}, {"config", "user.email", "fixture@example.invalid"}, {"config", "commit.gpgsign", "false"}} {
		command := exec.Command("git", args...) // #nosec G204 G702 -- fixed test-owned Git argument tables, no shell.
		command.Dir = root
		out, err := command.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "%s", out)
	}
	Expect(os.WriteFile(filepath.Join(root, "source.txt"), []byte("initial"), 0600)).To(Succeed())
	for _, args := range [][]string{{"add", "source.txt"}, {"commit", "-qm", "fixture"}} {
		command := exec.Command("git", args...) // #nosec G204 G702 -- fixed test-owned Git argument tables, no shell.
		command.Dir = root
		out, err := command.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "%s", out)
	}
	config, err := Configuration(context.Background(), environment)
	Expect(err).NotTo(HaveOccurred())
	return root, config, environment
}

var _ = Describe("Loop snapshot collaborator boundaries", func() {
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:81
	It("refuses an edit between full observations", func() {
		root, config, environment := snapshotFixture()
		calls := 0
		scanner := inspector{betweenSnapshots: func(context.Context) error {
			calls++
			return os.WriteFile(filepath.Join(root, "source.txt"), []byte("changed"), 0600)
		}}
		_, err := scanner.stableSnapshot(context.Background(), root, config, environment)
		Expect(err).To(HaveOccurred())
		Expect(calls).To(Equal(1))
	})
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:104
	DescribeTable("refuses a nonregular replacement between stat and open", func(kind string) {
		root, config, environment := snapshotFixture()
		changed := false
		scanner := inspector{openFile: func(path string, flags int) (*os.File, error) {
			if path == filepath.Join(root, "source.txt") && !changed {
				changed = true
				Expect(os.Remove(path)).To(Succeed())
				if kind == "fifo" {
					Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
				} else {
					Expect(os.Symlink("PRIVATE_TARGET", path)).To(Succeed())
				}
			}
			return os.OpenFile(path, flags, 0)
		}}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, err := scanner.snapshot(ctx, root, config, environment)
		Expect(changed).To(BeTrue())
		Expect(err).To(HaveOccurred())
	}, Entry("FIFO", "fifo"), Entry("symlink", "link"))
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:136
	It("counts actual native policy bytes after stat across multiple growing files", func() {
		root, config, environment := snapshotFixture()
		tree := filepath.Join(root, ".codex/hooks")
		Expect(os.MkdirAll(tree, 0700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".codex/\n"), 0600)).To(Succeed())
		for i := range 9 {
			Expect(os.WriteFile(filepath.Join(tree, fmt.Sprint(i)), nil, 0600)).To(Succeed())
		}
		opened := 0
		scanner := inspector{openFile: func(path string, flags int) (*os.File, error) {
			if strings.HasPrefix(path, tree+string(os.PathSeparator)) {
				Expect(os.Truncate(path, 1024*1024)).To(Succeed())
				opened++
			}
			return os.OpenFile(path, flags, 0)
		}}
		_, err := scanner.snapshot(context.Background(), root, config, environment)
		Expect(err).To(HaveOccurred())
		Expect(opened).To(Equal(9))
	})
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:107
	It("refuses a native file grown beyond its per-file bound after stat", func() {
		root, config, environment := snapshotFixture()
		path := filepath.Join(root, "opencode.json")
		Expect(os.WriteFile(path, nil, 0600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, ".gitignore"), []byte("opencode.json\n"), 0600)).To(Succeed())
		grew := false
		scanner := inspector{openFile: func(name string, flags int) (*os.File, error) {
			if name == path {
				Expect(os.Truncate(path, 1024*1024+1)).To(Succeed())
				grew = true
			}
			return os.OpenFile(name, flags, 0)
		}}
		_, err := scanner.snapshot(context.Background(), root, config, environment)
		Expect(err).To(HaveOccurred())
		Expect(grew).To(BeTrue())
	})
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:91
	It("refuses a source file grown past64MiB after stat", func() {
		root, config, environment := snapshotFixture()
		grew := false
		scanner := inspector{openFile: func(path string, flags int) (*os.File, error) {
			if filepath.Base(path) == "source.txt" {
				Expect(os.Truncate(path, 64*1024*1024+1)).To(Succeed())
				grew = true
			}
			return os.OpenFile(path, flags, 0)
		}}
		_, err := scanner.snapshot(context.Background(), root, config, environment)
		Expect(err).To(HaveOccurred())
		Expect(grew).To(BeTrue())
	})
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:96
	It("honors cancellation before probing or opening source", func() {
		root, config, environment := snapshotFixture()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		opened := 0
		scanner := inspector{openFile: func(path string, flags int) (*os.File, error) { opened++; return os.OpenFile(path, flags, 0) }}
		_, err := scanner.snapshot(ctx, root, config, environment)
		Expect(err).To(HaveOccurred())
		Expect(opened).To(BeZero())
	})
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:96
	It("cancels and reaps an observed hung git probe within a bounded wait", func() {
		root, config, environment := snapshotFixture()
		bin := GinkgoT().TempDir()
		marker := filepath.Join(bin, "pid")
		Expect(os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\necho $$ > \"$LOOP_PROBE_PID\"\nexec /bin/sleep 30\n"), 0700)).To(Succeed()) // #nosec G306 -- fixed test-owned probe fixture must be owner-executable; no external code is installed.
		environment["PATH"] = bin
		environment["LOOP_PROBE_PID"] = marker
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		done := make(chan error, 1)
		go func() { defer close(done); _, err := Snapshot(ctx, root, config, environment); done <- err }()
		DeferCleanup(func() { cancel(); Eventually(done, 6*time.Second).Should(BeClosed()) })
		var raw []byte
		Eventually(func() error { var err error; raw, err = os.ReadFile(marker); return err }, 2*time.Second).Should(Succeed())
		started := time.Now()
		cancel()
		var result error
		Eventually(done, 2*time.Second).Should(Receive(&result))
		Expect(result).To(HaveOccurred())
		Expect(time.Since(started)).To(BeNumerically("<", 2*time.Second))
		pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		Expect(err).NotTo(HaveOccurred())
		Expect(syscall.Kill(pid, 0)).To(Equal(syscall.ESRCH))
	})
})

var _ = Describe("Loop policy field coverage", func() {
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:49
	It("accounts for every exported loop and budget configuration field", func() {
		environment := loopTestEnvironment()
		ctx := context.Background()
		config, err := Configuration(ctx, environment)
		Expect(err).NotTo(HaveOccurred())
		budgetConfig, err := budget.Configuration(environment)
		Expect(err).NotTo(HaveOccurred())
		baseline, err := Policy(ctx, config, budgetConfig, environment)
		Expect(err).NotTo(HaveOccurred())
		groups := []struct {
			kind     reflect.Type
			prefix   string
			settings map[string]string
		}{
			{reflect.TypeOf(config), "FACTORY_LOOP_", map[string]string{"Enabled": "ENABLED=true", "CheckCommand": "CHECK_COMMAND=true", "TestPatterns": "TEST_PATTERNS=x", "ProtectedPaths": "PROTECTED_PATHS=y", "MaxAttempts": "MAX_ATTEMPTS=3", "TimeoutSeconds": "TIMEOUT_SECONDS=901", "CheckTimeoutSeconds": "CHECK_TIMEOUT_SECONDS=121", "NoProgressLimit": "NO_PROGRESS_LIMIT=2"}},
			{reflect.TypeOf(budgetConfig), "FACTORY_BUDGET_", map[string]string{"Enabled": "ENABLED=true", "Action": "ACTION=warn", "MaxAttempts": "MAX_ATTEMPTS=2", "MaxSessionRuns": "MAX_SESSION_RUNS=6", "MaxConcurrent": "MAX_CONCURRENT=2", "TimeoutSeconds": "TIMEOUT_SECONDS=301", "SessionSeconds": "SESSION_SECONDS=901", "EstimatedUSD": "ESTIMATED_USD=0.1"}},
		}
		for _, group := range groups {
			Expect(group.settings).To(HaveLen(group.kind.NumField()), "new Config fields require independent digest perturbation coverage")
			for index := range group.kind.NumField() {
				name := group.kind.Field(index).Name
				setting, exists := group.settings[name]
				Expect(exists).To(BeTrue(), "uncovered %s.%s", group.kind.Name(), name)
				key, value, _ := strings.Cut(setting, "=")
				next := loopTestEnvironment()
				next[group.prefix+key] = value
				loopNext, err := Configuration(ctx, next)
				Expect(err).NotTo(HaveOccurred())
				budgetNext, err := budget.Configuration(next)
				Expect(err).NotTo(HaveOccurred())
				fingerprint, err := Policy(ctx, loopNext, budgetNext, next)
				Expect(err).NotTo(HaveOccurred())
				Expect(fingerprint).NotTo(Equal(baseline), "policy omitted %s%s", group.prefix, name)
			}
		}
	})
})

var _ = Describe("Loop probe resource boundaries", func() {
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:91
	DescribeTable("bounds actual probe stdout at32MiB", func(excess int) {
		data := bytes.Repeat([]byte("x"), 32*1024*1024+excess)
		environment := loopTestEnvironment()
		environment["PATH"] = "/bin:/usr/bin"
		output, status, err := probe(context.Background(), environment, 5*time.Second, []string{"cat"}, data)
		if excess == 0 {
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(BeZero())
			Expect(output).To(Equal(data))
		} else {
			Expect(err).To(HaveOccurred())
		}
	}, Entry("exact", 0), Entry("one extra byte", 1))
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:90
	DescribeTable("bounds enumerated source paths at100000", func(count int, valid bool) {
		root, config, environment := snapshotFixture()
		bin := GinkgoT().TempDir()
		var names strings.Builder
		for index := range count {
			fmt.Fprintf(&names, "absent%06d%c", index, 0)
		}
		dataPath := filepath.Join(bin, "names")
		Expect(os.WriteFile(dataPath, []byte(names.String()), 0600)).To(Succeed())
		script := "#!/bin/sh\ncase \"$3\" in\nls-files) case \"$4\" in -u|--stage) exit 0;; *) exec /bin/cat \"$LOOP_NAMES\";; esac;;\nrev-parse) printf '%s\\n' 0123456789012345678901234567890123456789;;\n*) exit 1;;\nesac\n"
		Expect(os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0700)).To(Succeed()) // #nosec G306 -- fixed test-owned probe fixture must be owner-executable; no external code is installed.
		environment["PATH"] = bin
		environment["LOOP_NAMES"] = dataPath
		_, err := Snapshot(context.Background(), root, config, environment)
		if valid {
			Expect(err).NotTo(HaveOccurred())
		} else {
			Expect(err).To(HaveOccurred())
		}
	}, Entry("exact", 100000, true), Entry("one extra", 100001, false))
})

var _ = Describe("Loop captured probe environment", func() {
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:99
	It("resolves relative captured PATH from caller cwd and preserves that cwd", func() {
		caller, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		bin := GinkgoT().TempDir()
		relative, err := filepath.Rel(caller, bin)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\nexec /bin/pwd\n"), 0700)).To(Succeed()) // #nosec G306 -- fixed test-owned probe fixture must be owner-executable; no external code is installed.
		environment := loopTestEnvironment()
		environment["PATH"] = relative
		output, status, err := probe(context.Background(), environment, time.Second, []string{"git"}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(BeZero())
		Expect(strings.TrimSpace(string(output))).To(Equal(caller))
	})
})

var _ = Describe("Loop signal-refused grep", func() {
	// per docs/adr/0073-go-loop-fingerprint-foundation.md:35
	DescribeTable("refuses a signaled POSIX ERE probe", func(configuration bool) {
		root, config, environment := snapshotFixture()
		bin := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(bin, "grep"), []byte("#!/bin/sh\nkill -TERM $$\n"), 0700)).To(Succeed()) // #nosec G306 -- fixed test-owned probe fixture must be owner-executable; no external code is installed.
		environment["PATH"] = bin + string(os.PathListSeparator) + environment["PATH"]
		if configuration {
			environment["FACTORY_LOOP_TEST_PATTERNS"] = "x"
			_, err := Configuration(context.Background(), environment)
			Expect(err).To(HaveOccurred())
		} else {
			config.TestPatterns = []string{"x"}
			_, err := Snapshot(context.Background(), root, config, environment)
			Expect(err).To(HaveOccurred())
		}
	}, Entry("configuration", true), Entry("source classification", false))
})
