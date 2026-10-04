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
	"golang.org/x/sys/unix"
)

func transitionFixture() (string, string, string) {
	GinkgoHelper()
	root, _, checkout := loopFixture()
	repo, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	scripts := filepath.Join(root, "current-legacy", "scripts")
	for _, name := range []string{"factory-budget.sh", "factory-loop.sh"} {
		data, err := os.ReadFile(filepath.Join(repo, "scripts", name))
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(scripts, name), data, 0600)
	}
	entries, err := os.ReadDir(filepath.Join(repo, "scripts/lib"))
	Expect(err).NotTo(HaveOccurred())
	for _, entry := range entries {
		if entry.Type().IsRegular() && (strings.HasSuffix(entry.Name(), ".py") || strings.HasSuffix(entry.Name(), ".sh")) {
			data, err := os.ReadFile(filepath.Join(repo, "scripts/lib", entry.Name()))
			Expect(err).NotTo(HaveOccurred())
			writeFixture(filepath.Join(scripts, "lib", entry.Name()), data, 0600)
		}
	}
	for _, role := range []string{"implementer", "reviewer"} {
		writeFixture(filepath.Join(checkout, ".opencode/agent", role+".md"), []byte("---\nname: role\n---\nCanonical role instructions\n"), 0600)
		writeFixture(filepath.Join(checkout, ".claude/agents", role+".md"), []byte("Native instructions\n"), 0600)
	}
	writeFixture(filepath.Join(checkout, "opencode.json"), []byte(`{"agent":{"reviewer":{"permission":{"edit":"deny"}},"implementer":{"permission":{"edit":"allow"}}}}`), 0600)
	writeFixture(filepath.Join(checkout, "prompt.txt"), []byte("PRIVATE_TRANSITION_PROMPT\n"), 0600)
	writeFixture(filepath.Join(checkout, "factory.yaml"), []byte("budget_enabled: true\ncheck_command: printf checked > \"$TRANSITION_CHILD_MARKER\"\n"), 0600)
	writeFixture(filepath.Join(root, "bin", "claude"), []byte("#!/bin/sh\nprintf probed > \"$TRANSITION_CHILD_MARKER\"\nprintf 'unsupported fixture help\\n'\n"), 0700)
	return root, checkout, scripts
}

func transitionRun(root, checkout, scripts, kind string, legacy bool, args []string, extra ...string) cliResult {
	GinkgoHelper()
	environment := append([]string{"PATH=" + filepath.Join(root, "bin") + string(os.PathListSeparator) + os.Getenv("PATH"), "TRANSITION_CHILD_MARKER=" + filepath.Join(root, "child-started")}, extra...)
	if legacy {
		return loopProcess(checkout, "bash", append([]string{filepath.Join(scripts, "factory-"+kind+".sh")}, args...), environment)
	}
	return loopProcess(checkout, "/usr/bin/env", append([]string{"-u", "FACTORY_BRIDGE_PROTOCOL", filepath.Join(root, "factory"), kind}, args...), environment)
}

func transitionExclusive(checkout string) *os.File {
	GinkgoHelper()
	state := filepath.Join(checkout, ".factory")
	Expect(os.Mkdir(state, 0755)).To(Succeed())
	file, err := os.OpenFile(filepath.Join(state, "runtime-transition.lock"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	Expect(err).NotTo(HaveOccurred())
	Expect(file.Sync()).To(Succeed())
	Expect(unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)).To(Succeed())
	DeferCleanup(func() {
		Expect(unix.Flock(int(file.Fd()), unix.LOCK_UN)).To(Succeed())
		Expect(file.Close()).To(Succeed())
	})
	return file
}

func transitionRunArgs(kind string) []string {
	args := []string{"run", "--session", "s", "--task", "t", "--harness", "claude", "--json"}
	if kind == "budget" {
		return append(args, "--prompt-file", "prompt.txt")
	}
	return append(args, "--mode", "manual")
}

var _ = Describe("Runtime transition exclusion core", func() {
	// per docs/adr/0093-runtime-transition-guard.md:27
	// per docs/adr/0093-runtime-transition-guard.md:32
	// per docs/adr/0093-runtime-transition-guard.md:154
	DescribeTable("refuses real exclusive flock contention before native preflight or manual checks", func(kind string, legacy bool) {
		root, checkout, scripts := transitionFixture()
		lock := transitionExclusive(checkout)
		before, err := lock.Stat()
		Expect(err).NotTo(HaveOccurred())
		out := transitionRun(root, checkout, scripts, kind, legacy, transitionRunArgs(kind))
		_, err = os.Lstat(filepath.Join(root, "child-started"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "an exclusive transition owner must prevent native help and manual checks: %+v", out)
		Expect(out.status).To(Equal(2), "%+v", out)
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).NotTo(BeEmpty())
		Expect(out.stderr).NotTo(ContainSubstring("PRIVATE_"))
		Expect(out.stderr).NotTo(ContainSubstring(checkout))
		after, err := os.Lstat(filepath.Join(checkout, ".factory/runtime-transition.lock"))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		Expect(after.Mode().Perm()).To(Equal(os.FileMode(0600)))
		Expect(after.Size()).To(BeZero())
		for _, name := range []string{"budget.json", "loops.json", "runtime-activity"} {
			_, err := os.Lstat(filepath.Join(checkout, ".factory", name))
			Expect(os.IsNotExist(err)).To(BeTrue(), "refused work must not publish ledger, checkpoint or activity")
		}
	}, Entry("compiled Go budget", "budget", false), Entry("legacy Bash/Python budget", "budget", true), Entry("compiled Go manual loop", "loop", false), Entry("legacy Bash/Python manual loop", "loop", true))
})

var _ = Describe("Runtime transition legacy preflight ownership", func() {
	// per docs/adr/0093-runtime-transition-guard.md:59
	// per docs/adr/0093-runtime-transition-guard.md:66
	It("does not discard activity while a successful help probe's detached-pipe descendant remains alive", func() {
		root, checkout, scripts := transitionFixture()
		python := transitionPython(checkout)
		fake := `import sys, os, json, time, subprocess
from pathlib import Path
if '--help' in sys.argv:
 child = subprocess.Popen([sys.executable, '-c', "import os,time;from pathlib import Path;Path(os.environ['TRANSITION_HELP_READY']).write_text('ready');time.sleep(30)"], stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
 Path(os.environ['TRANSITION_HELP_DESC_PID']).write_text(str(child.pid))
 deadline=time.monotonic()+3
 while not Path(os.environ['TRANSITION_HELP_READY']).exists():
  if time.monotonic()>deadline: raise RuntimeError('descendant handshake missing')
  time.sleep(.01)
 print('--print --output-format --agent --permission-mode --model');sys.exit()
sys.stdin.read()
print(json.dumps({'type':'result','subtype':'success','result':'fixture answer','usage':{'input_tokens':3,'output_tokens':2,'cache_creation_input_tokens':0,'cache_read_input_tokens':1},'total_cost_usd':.1}))
`
		writeFixture(filepath.Join(root, "bin/claude"), []byte("#!"+python+"\n"+fake), 0700)
		pidPath, ready := filepath.Join(root, "help-descendant-pid"), filepath.Join(root, "help-descendant-ready")
		out := transitionRun(root, checkout, scripts, "budget", true, transitionRunArgs("budget"), "TRANSITION_HELP_DESC_PID="+pidPath, "TRANSITION_HELP_READY="+ready)
		data, err := os.ReadFile(pidPath)
		Expect(err).NotTo(HaveOccurred(), "actual help probe must spawn its descendant: %+v", out)
		pid, err := strconv.Atoi(string(data))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
		_, err = os.Stat(ready)
		Expect(err).NotTo(HaveOccurred(), "descendant must confirm execution independently")
		alive := syscall.Kill(pid, 0) == nil
		if alive {
			state := loopProcess(checkout, "/bin/ps", []string{"-o", "stat=", "-p", strconv.Itoa(pid)}, nil)
			alive = !strings.HasPrefix(strings.TrimSpace(state.stdout), "Z")
		}
		entries, err := os.ReadDir(filepath.Join(checkout, ".factory/runtime-activity"))
		Expect(err == nil || os.IsNotExist(err)).To(BeTrue())
		Expect(!alive || len(entries) > 0).To(BeTrue(), "successful direct help exit cannot prove descendant exit: controller status=%d, descendant pid=%d alive=%t, activity entries=%d, stdout=%s stderr=%s", out.status, pid, alive, len(entries), out.stdout, out.stderr)
	})
})

func transitionAlive(checkout string, pid int) bool {
	if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
		return false
	}
	state := loopProcess(checkout, "/bin/ps", []string{"-o", "stat=", "-p", strconv.Itoa(pid)}, nil)
	if strings.HasPrefix(strings.TrimSpace(state.stdout), "Z") {
		return false
	}
	return syscall.Kill(pid, 0) != syscall.ESRCH
}

var _ = Describe("Runtime transition snapshot ownership", func() {
	// per docs/adr/0093-runtime-transition-guard.md:56
	// per docs/adr/0093-runtime-transition-guard.md:66
	DescribeTable("does not declare quiescence after a successful snapshot leaves an executing descendant", func(legacy bool) {
		root, checkout, scripts := transitionFixture()
		python := transitionPython(checkout)
		realGit, err := exec.LookPath("git")
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(checkout, ".gitignore"), []byte(".factory/\n"), 0600)
		wrapper := `import os,sys,subprocess,time
from pathlib import Path
if 'ls-files' in sys.argv and '-u' in sys.argv:
 try:
  with open(os.environ['TRANSITION_SNAPSHOT_ONCE'],'x') as f: f.write('owned')
 except FileExistsError: pass
 else:
  child=subprocess.Popen([sys.executable,'-c',"import os,time;from pathlib import Path;Path(os.environ['TRANSITION_SNAPSHOT_READY']).write_text('ready');time.sleep(30)"],stdin=subprocess.DEVNULL,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
  Path(os.environ['TRANSITION_SNAPSHOT_PID']).write_text(str(child.pid))
  deadline=time.monotonic()+3
  while not Path(os.environ['TRANSITION_SNAPSHOT_READY']).exists():
   if time.monotonic()>deadline: raise RuntimeError('snapshot child handshake missing')
   time.sleep(.01)
os.execv(os.environ['TRANSITION_REAL_GIT'],[os.environ['TRANSITION_REAL_GIT']]+sys.argv[1:])
`
		writeFixture(filepath.Join(root, "bin/git"), []byte("#!"+python+"\n"+wrapper), 0700)
		pidPath, ready := filepath.Join(root, "snapshot-descendant-pid"), filepath.Join(root, "snapshot-descendant-ready")
		out := transitionRun(root, checkout, scripts, "loop", legacy, transitionRunArgs("loop"), "TRANSITION_REAL_GIT="+realGit, "TRANSITION_SNAPSHOT_PID="+pidPath, "TRANSITION_SNAPSHOT_READY="+ready, "TRANSITION_SNAPSHOT_ONCE="+filepath.Join(root, "snapshot-once"))
		data, err := os.ReadFile(pidPath)
		Expect(err).NotTo(HaveOccurred(), "execution snapshot must start the actual descendant: %+v", out)
		pid, err := strconv.Atoi(string(data))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
		_, err = os.Stat(ready)
		Expect(err).NotTo(HaveOccurred())
		if out.status == 0 {
			record := budgetDecode([]byte(out.stdout)).(map[string]any)
			Expect(record["outcome"]).To(Equal("manual_passed"))
			Expect(record["status"]).To(Equal("stopped"))
			Expect(record["snapshot"]).To(Equal(record["baseline"]), "wrapper must preserve real Git outputs and stable source fingerprints")
		}
		alive := transitionAlive(checkout, pid)
		entries, err := os.ReadDir(filepath.Join(checkout, ".factory/runtime-activity"))
		Expect(err).NotTo(HaveOccurred())
		Expect(!alive || len(entries) > 0 && out.status != 0).To(BeTrue(), "a successful snapshot leader does not prove descendant exit: status=%d pid=%d alive=%t markers=%d stdout=%s stderr=%s", out.status, pid, alive, len(entries), out.stdout, out.stderr)
	}, Entry("compiled Go manual loop", false), Entry("legacy Bash/Python manual loop", true))
})

func transitionPython(checkout string) string {
	GinkgoHelper()
	out := loopProcess(checkout, "python3", []string{"-B", "-c", "import sys; print(sys.executable)"}, nil)
	Expect(out.status).To(BeZero(), "%+v", out)
	return strings.TrimSpace(out.stdout)
}

const transitionNativeSource = `import os,sys,time,json
from pathlib import Path
if '--help' in sys.argv:
 if os.environ.get('TRANSITION_HELP_STARTED'): Path(os.environ['TRANSITION_HELP_STARTED']).write_text(str(os.getpid()))
 release=os.environ.get('TRANSITION_RELEASE')
 while release and not Path(release).exists(): time.sleep(.01)
 if os.environ.get('TRANSITION_BAD_HELP')=='true': print('unsupported');sys.exit()
 print('--json --sandbox --cd --model --config --print --output-format --agent --permission-mode --format');sys.exit()
sys.stdin.read()
role=os.environ.get('FACTORY_AGENT_ROLE')
if os.environ.get('TRANSITION_BOUNDED')=='true' and role=='implementer': Path('source.txt').write_text('implementation complete\n')
answer='{"verdict":"approve","findings":[]}' if role=='reviewer' else 'fixture answer'
if os.environ.get('TRANSITION_CALLS'):
 with open(os.environ['TRANSITION_CALLS'],'a') as f: f.write(role+'\n')
print(json.dumps({'type':'result','subtype':'success','result':answer,'usage':{'input_tokens':3,'output_tokens':2,'cache_creation_input_tokens':0,'cache_read_input_tokens':1},'total_cost_usd':.1}))
`

func transitionNative(root, checkout string) {
	GinkgoHelper()
	python := transitionPython(checkout)
	writeFixture(filepath.Join(root, "bin/claude"), []byte("#!"+python+"\n"+transitionNativeSource), 0700)
}
func transitionMarkers(checkout string) []os.DirEntry {
	GinkgoHelper()
	entries, err := os.ReadDir(filepath.Join(checkout, ".factory/runtime-activity"))
	Expect(err).NotTo(HaveOccurred())
	for _, entry := range entries {
		info, err := entry.Info()
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().IsRegular()).To(BeTrue())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
		Expect(info.Size()).To(BeZero())
	}
	return entries
}
func transitionLockBlocked(checkout string) {
	GinkgoHelper()
	file, err := os.OpenFile(filepath.Join(checkout, ".factory/runtime-transition.lock"), os.O_RDWR, 0)
	Expect(err).NotTo(HaveOccurred())
	defer func() { Expect(file.Close()).To(Succeed()) }()
	err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	Expect(errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)).To(BeTrue(), "controller must hold the actual permanent shared flock")
}

type transitionRunning struct {
	command        *exec.Cmd
	stdout, stderr bytes.Buffer
	done           chan error
	finished       bool
}

func transitionStart(root, checkout, scripts, kind string, legacy bool, args []string, extra ...string) *transitionRunning {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	running := &transitionRunning{done: make(chan error, 1)}
	program := "/usr/bin/env"
	operands := append([]string{"-u", "FACTORY_BRIDGE_PROTOCOL", filepath.Join(root, "factory"), kind}, args...)
	if legacy {
		program = "bash"
		operands = append([]string{filepath.Join(scripts, "factory-"+kind+".sh")}, args...)
	}
	running.command = exec.CommandContext(ctx, program, operands...) // #nosec G204 G702 -- fixed acceptance executable or test-owned compiled binary/script and literal argv.
	running.command.Dir = checkout
	running.command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "FACTORY_") && !strings.HasPrefix(entry, "OPENCODE_CONFIG_CONTENT=") {
			running.command.Env = append(running.command.Env, entry)
		}
	}
	running.command.Env = append(running.command.Env, "PYTHONDONTWRITEBYTECODE=1", "GORACE=atexit_sleep_ms=0", "PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"), "TRANSITION_CHILD_MARKER="+filepath.Join(root, "child-started"))
	running.command.Env = append(running.command.Env, extra...)
	running.command.Stdout = &running.stdout
	running.command.Stderr = &running.stderr
	Expect(running.command.Start()).To(Succeed())
	go func() { running.done <- running.command.Wait() }()
	DeferCleanup(func() {
		cancel()
		_ = syscall.Kill(-running.command.Process.Pid, syscall.SIGKILL)
		if !running.finished {
			Eventually(running.done, 3*time.Second).Should(Receive())
			running.finished = true
		}
	})
	return running
}
func (running *transitionRunning) wait() cliResult {
	GinkgoHelper()
	var err error
	Eventually(running.done, 5*time.Second).Should(Receive(&err))
	running.finished = true
	status := 0
	if err != nil {
		var exited *exec.ExitError
		Expect(errors.As(err, &exited)).To(BeTrue())
		status = exited.ExitCode()
	}
	return cliResult{running.stdout.String(), running.stderr.String(), status}
}
func transitionAwaitPID(path string) int {
	GinkgoHelper()
	var data []byte
	Eventually(func() bool { var err error; data, err = os.ReadFile(path); return err == nil }, 3*time.Second).Should(BeTrue(), "child handshake missing")
	pid, err := strconv.Atoi(string(data))
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	return pid
}

var _ = Describe("Runtime transition controller lifecycle", func() {
	// per docs/adr/0093-runtime-transition-guard.md:53
	// per docs/adr/0093-runtime-transition-guard.md:62
	DescribeTable("holds durable evidence and shared exclusion while native help is paused before admission", func(legacy bool) {
		root, checkout, scripts := transitionFixture()
		transitionNative(root, checkout)
		ready, release := filepath.Join(root, "help-ready"), filepath.Join(root, "help-release")
		running := transitionStart(root, checkout, scripts, "budget", legacy, transitionRunArgs("budget"), "TRANSITION_HELP_STARTED="+ready, "TRANSITION_RELEASE="+release)
		pid := transitionAwaitPID(ready)
		Expect(transitionAlive(checkout, pid)).To(BeTrue())
		transitionLockBlocked(checkout)
		Expect(transitionMarkers(checkout)).To(HaveLen(1))
		directory, err := os.Stat(filepath.Join(checkout, ".factory/runtime-activity"))
		Expect(err).NotTo(HaveOccurred())
		Expect(directory.Mode().Perm()).To(Equal(os.FileMode(0700)))
		_, err = os.Stat(filepath.Join(checkout, ".factory/budget.json"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "help must still be pre-admission")
		writeFixture(release, nil, 0600)
		out := running.wait()
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(transitionMarkers(checkout)).To(BeEmpty())
	}, Entry("compiled Go budget", false), Entry("legacy Bash/Python budget", true))
	// per docs/adr/0093-runtime-transition-guard.md:82
	DescribeTable("retains evidence after abrupt controller death even if its flock is released and ledger is empty", func(legacy bool) {
		root, checkout, scripts := transitionFixture()
		transitionNative(root, checkout)
		ready, release := filepath.Join(root, "help-ready"), filepath.Join(root, "help-release")
		running := transitionStart(root, checkout, scripts, "budget", legacy, transitionRunArgs("budget"), "TRANSITION_HELP_STARTED="+ready, "TRANSITION_RELEASE="+release)
		pid := transitionAwaitPID(ready)
		before := transitionMarkers(checkout)
		Expect(before).To(HaveLen(1))
		transitionLockBlocked(checkout)
		Expect(running.command.Process.Kill()).To(Succeed())
		out := running.wait()
		Expect(out.status).NotTo(BeZero())
		Expect(transitionAlive(checkout, pid)).To(BeTrue(), "child is actually still executing after controller death")
		after := transitionMarkers(checkout)
		Expect(after).To(HaveLen(1))
		Expect(after[0].Name()).To(Equal(before[0].Name()))
		_, err := os.Stat(filepath.Join(checkout, ".factory/budget.json"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		lock, err := os.OpenFile(filepath.Join(checkout, ".factory/runtime-transition.lock"), os.O_RDWR, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB)).To(Succeed(), "unlocked inode must not hide retained descendant evidence")
		Expect(lock.Close()).To(Succeed())
	}, Entry("compiled Go budget", false), Entry("legacy Bash/Python budget", true))
	// per docs/adr/0093-runtime-transition-guard.md:64
	// per docs/adr/0093-runtime-transition-guard.md:40
	DescribeTable("cleans a known harmless pre-admission refusal while preserving the existing 0755 parent", func(legacy bool) {
		root, checkout, scripts := transitionFixture()
		transitionNative(root, checkout)
		Expect(os.Mkdir(filepath.Join(checkout, ".factory"), 0755)).To(Succeed())
		out := transitionRun(root, checkout, scripts, "budget", legacy, transitionRunArgs("budget"), "TRANSITION_BAD_HELP=true")
		Expect(out.status).To(Equal(2), "%+v", out)
		Expect(transitionMarkers(checkout)).To(BeEmpty())
		info, err := os.Stat(filepath.Join(checkout, ".factory"))
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0755)))
		_, err = os.Stat(filepath.Join(checkout, ".factory/budget.json"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("compiled Go budget", false), Entry("legacy Bash/Python budget", true))
	// per docs/adr/0093-runtime-transition-guard.md:65
	// per docs/adr/0093-runtime-transition-guard.md:171
	DescribeTable("completes confirmed work and reuses the permanent inode without resetting accounting", func(kind string, legacy bool) {
		root, checkout, scripts := transitionFixture()
		transitionNative(root, checkout)
		out := transitionRun(root, checkout, scripts, kind, legacy, transitionRunArgs(kind))
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(transitionMarkers(checkout)).To(BeEmpty())
		path := filepath.Join(checkout, ".factory/runtime-transition.lock")
		before, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		args := transitionRunArgs(kind)
		args[4] = "t2"
		out = transitionRun(root, checkout, scripts, kind, legacy, args)
		Expect(out.status).To(BeZero(), "%+v", out)
		after, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		Expect(transitionMarkers(checkout)).To(BeEmpty())
		history := "budget.json"
		if kind == "loop" {
			history = "loops.json"
		}
		data, err := os.ReadFile(filepath.Join(checkout, ".factory", history))
		Expect(err).NotTo(HaveOccurred())
		rows := budgetDecode(data).(map[string]any)["runs"].([]any)
		Expect(rows).To(HaveLen(2))
	}, Entry("compiled Go budget", "budget", false), Entry("legacy budget", "budget", true), Entry("compiled Go manual loop", "loop", false), Entry("legacy manual loop", "loop", true))
	// per docs/adr/0093-runtime-transition-guard.md:30
	It("allows overlapping Go and Python preflight owners on the same permanent inode", func() {
		root, checkout, scripts := transitionFixture()
		transitionNative(root, checkout)
		aReady, bReady, release := filepath.Join(root, "a-ready"), filepath.Join(root, "b-ready"), filepath.Join(root, "release")
		a := transitionStart(root, checkout, scripts, "budget", false, transitionRunArgs("budget"), "TRANSITION_HELP_STARTED="+aReady, "TRANSITION_RELEASE="+release, "TRANSITION_BAD_HELP=true")
		transitionAwaitPID(aReady)
		before, err := os.Stat(filepath.Join(checkout, ".factory/runtime-transition.lock"))
		Expect(err).NotTo(HaveOccurred())
		args := transitionRunArgs("budget")
		args[4] = "t2"
		b := transitionStart(root, checkout, scripts, "budget", true, args, "TRANSITION_HELP_STARTED="+bReady, "TRANSITION_RELEASE="+release, "TRANSITION_BAD_HELP=true")
		transitionAwaitPID(bReady)
		after, err := os.Stat(filepath.Join(checkout, ".factory/runtime-transition.lock"))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		Expect(transitionMarkers(checkout)).To(HaveLen(2))
		transitionLockBlocked(checkout)
		writeFixture(release, nil, 0600)
		Expect(a.wait().status).To(Equal(2))
		Expect(b.wait().status).To(Equal(2))
		Expect(transitionMarkers(checkout)).To(BeEmpty())
	})
	// per docs/adr/0093-runtime-transition-guard.md:31
	DescribeTable("holds an outer loop guard while nested budget owners finish separate implementation and review", func(legacy bool) {
		root, checkout, scripts := transitionFixture()
		transitionNative(root, checkout)
		writeFixture(filepath.Join(checkout, "factory.yaml"), []byte("budget_enabled: true\nbudget_max_attempts: 4\nloop_enabled: true\ncheck_command: true\n"), 0600)
		args := transitionRunArgs("loop")
		args[len(args)-1] = "bounded"
		args = append(args, "--prompt-file", "prompt.txt")
		calls := filepath.Join(root, "calls")
		out := transitionRun(root, checkout, scripts, "loop", legacy, args, "TRANSITION_BOUNDED=true", "TRANSITION_CALLS="+calls)
		Expect(out.status).To(BeZero(), "%+v", out)
		record := budgetDecode([]byte(out.stdout)).(map[string]any)
		Expect(record["outcome"]).To(Equal("approved"))
		Expect(record["budget_runs"]).To(HaveLen(2))
		Expect(record["uncertain"]).To(BeFalse())
		data, err := os.ReadFile(calls)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Fields(string(data))).To(Equal([]string{"implementer", "reviewer"}))
		Expect(transitionMarkers(checkout)).To(BeEmpty())
	}, Entry("compiled Go loop", false), Entry("legacy Bash/Python loop", true))
})

var _ = Describe("Runtime transition read-only compatibility", func() {
	// per docs/adr/0093-runtime-transition-guard.md:134
	DescribeTable("leaves absent guard infrastructure absent for read-only and initially blocked operations", func(kind, action string, legacy bool) {
		root, checkout, scripts := transitionFixture()
		writeFixture(filepath.Join(checkout, "factory.yaml"), []byte("budget_enabled: false\nloop_enabled: false\ncheck_command: true\n"), 0600)
		args := transitionRunArgs(kind)
		args[0] = action
		if kind == "budget" && action == "plan" {
			args = args[:len(args)-2]
		}
		if action == "report" {
			args = []string{"report", "--json"}
		}
		if kind == "loop" && action == "run" {
			args[len(args)-1] = "bounded"
			args = append(args, "--prompt-file", "prompt.txt")
		}
		out := transitionRun(root, checkout, scripts, kind, legacy, args)
		status := 2
		if action == "report" || kind == "loop" && action == "plan" {
			status = 0
		}
		Expect(out.status).To(Equal(status), "%+v", out)
		Expect(out.stderr).To(BeEmpty())
		Expect(out.stdout).NotTo(BeEmpty(), "valid read-only/blocked commands must produce their existing report")
		_, err := os.Stat(filepath.Join(checkout, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "read-only/blocked calls must not create lock, activity, ledger or checkpoints")
		_, err = os.Stat(filepath.Join(root, "child-started"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("Go budget plan", "budget", "plan", false), Entry("legacy budget plan", "budget", "plan", true), Entry("Go report", "budget", "report", false), Entry("legacy report", "budget", "report", true), Entry("Go disabled run", "budget", "run", false), Entry("legacy disabled run", "budget", "run", true), Entry("Go loop plan", "loop", "plan", false), Entry("legacy loop plan", "loop", "plan", true), Entry("Go blocked bounded loop", "loop", "run", false), Entry("legacy blocked bounded loop", "loop", "run", true))
})

var _ = Describe("Runtime transition actual harmless refusal", func() {
	// per docs/adr/0093-runtime-transition-guard.md:116
	It("cleans proved missing CLI refusal through the actual legacy entrypoint without admission or history", func() {
		root, checkout, scripts := transitionFixture()
		bin := filepath.Join(root, "only-utilities")
		Expect(os.Mkdir(bin, 0700)).To(Succeed())
		python := transitionPython(checkout)
		for _, name := range []string{"bash", "python3", "git", "sed", "head", "tr", "dirname", "cat", "grep", "awk", "env"} {
			program := python
			if name != "python3" {
				var err error
				program, err = exec.LookPath(name)
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(os.Symlink(program, filepath.Join(bin, name))).To(Succeed())
		}
		out := transitionRun(root, checkout, scripts, "budget", true, transitionRunArgs("budget"), "PATH="+bin)
		Expect(out.status).To(Equal(2), "%+v", out)
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).To(ContainSubstring("CLI is missing"))
		Expect(out.stderr).NotTo(ContainSubstring(checkout))
		for _, name := range []string{"budget.json", "loops.json"} {
			_, err := os.Stat(filepath.Join(checkout, ".factory", name))
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
		entries, err := os.ReadDir(filepath.Join(checkout, ".factory/runtime-activity"))
		Expect(err == nil || os.IsNotExist(err)).To(BeTrue())
		Expect(entries).To(BeEmpty())
		_, err = os.Stat(filepath.Join(root, "child-started"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

var _ = Describe("Runtime transition unsafe storage", func() {
	// per docs/adr/0093-runtime-transition-guard.md:34
	// per docs/adr/0093-runtime-transition-guard.md:38
	DescribeTable("refuses unsafe pre-existing controls before native help and preserves their original identity", func(kind string, legacy bool) {
		root, checkout, scripts := transitionFixture()
		state := filepath.Join(checkout, ".factory")
		Expect(os.Mkdir(state, 0755)).To(Succeed())
		target := filepath.Join(state, "runtime-transition.lock")
		outside := filepath.Join(root, "outside")
		Expect(os.Mkdir(outside, 0700)).To(Succeed())
		switch kind {
		case "lock contents":
			writeFixture(target, []byte("PRIVATE_CONTROL_CONTENT"), 0600)
		case "lock public mode":
			writeFixture(target, nil, 0644)
		case "lock hardlink":
			other := filepath.Join(outside, "control")
			writeFixture(other, nil, 0600)
			Expect(os.Link(other, target)).To(Succeed())
		case "lock FIFO":
			Expect(unix.Mkfifo(target, 0600)).To(Succeed())
		case "lock link":
			other := filepath.Join(outside, "control")
			writeFixture(other, nil, 0600)
			Expect(os.Symlink(other, target)).To(Succeed())
		case "activity public":
			target = filepath.Join(state, "runtime-activity")
			Expect(os.Mkdir(target, 0755)).To(Succeed())
		case "activity link":
			target = filepath.Join(state, "runtime-activity")
			Expect(os.Symlink(outside, target)).To(Succeed())
		case "state writable":
			target = state
			Expect(os.Chmod(state, 0770)).To(Succeed())
		}
		before, err := os.Lstat(target)
		Expect(err).NotTo(HaveOccurred())
		outsideBefore := assessmentTree(outside)
		out := transitionRun(root, checkout, scripts, "budget", legacy, transitionRunArgs("budget"))
		Expect(out.status).To(Equal(2), "%+v", out)
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).NotTo(BeEmpty())
		Expect(out.stderr).NotTo(ContainSubstring("PRIVATE_"))
		Expect(out.stderr).NotTo(ContainSubstring(checkout))
		_, err = os.Lstat(filepath.Join(root, "child-started"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		after, err := os.Lstat(target)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		Expect(after.Mode()).To(Equal(before.Mode()))
		Expect(after.Size()).To(Equal(before.Size()))
		Expect(assessmentTree(outside)).To(Equal(outsideBefore))
		if kind == "lock contents" {
			data, err := os.ReadFile(target)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(Equal("PRIVATE_CONTROL_CONTENT"))
		}
	}, Entry("Go nonempty lock", "lock contents", false), Entry("legacy nonempty lock", "lock contents", true), Entry("Go public lock", "lock public mode", false), Entry("legacy public lock", "lock public mode", true), Entry("Go hardlink", "lock hardlink", false), Entry("legacy hardlink", "lock hardlink", true), Entry("Go FIFO", "lock FIFO", false), Entry("legacy FIFO", "lock FIFO", true), Entry("Go lock link", "lock link", false), Entry("legacy lock link", "lock link", true), Entry("Go public activity", "activity public", false), Entry("legacy public activity", "activity public", true), Entry("Go activity link", "activity link", false), Entry("legacy activity link", "activity link", true), Entry("Go writable parent", "state writable", false), Entry("legacy writable parent", "state writable", true))
})
