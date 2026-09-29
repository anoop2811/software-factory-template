package acceptance_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const doctorBaseline = "22a60d5e76ba30430b4c5f5d44d27746425533bc"

const doctorProgressDriver = `import errno,os,pty,select,signal,subprocess,sys,time
binary,mode,release=sys.argv[1:]
if mode=='tty':
 pid,fd=pty.fork()
 if pid==0: os.execv(binary,[binary,'doctor'])
else:
 child=subprocess.Popen([binary,'doctor'],stdout=subprocess.PIPE,stderr=subprocess.STDOUT,start_new_session=True)
 pid,fd=child.pid,child.stdout.fileno()
output=bytearray(); status=None; observed=False
try:
 end=time.monotonic()+15
 while time.monotonic()<end:
  ready,_,_=select.select([fd],[],[],0.1)
  if ready:
   try: part=os.read(fd,65536)
   except OSError as e:
    if e.errno!=errno.EIO: raise
    part=b''
   output.extend(part)
  token=b'still going' if mode=='tty' else b'breaking each gate on purpose'
  if token in output and not observed:
   observed=True
   open(release,'w').close()
  if mode=='tty':
   found,raw=os.waitpid(pid,os.WNOHANG)
   if found: status=os.waitstatus_to_exitcode(raw)
  else: status=child.poll()
  if status is not None: break
finally:
 if status is None:
  os.killpg(pid,signal.SIGKILL)
  if mode=='tty': os.waitpid(pid,0)
  else: child.wait()
  status=98
sys.stdout.buffer.write(output)
if not observed: sys.stderr.write('progress was not observed before proof completion\n'); sys.exit(97)
sys.exit(status)
`

var _ = Describe("Native Go doctor retained validation", func() {
	// per docs/adr/0086-go-native-doctor.md:69
	DescribeTable("emits progress before a blocked proof can finish", func(mode string) {
		root, environment := doctorFixture()
		release := filepath.Join(filepath.Dir(root), "release-proof")
		writeFixture(filepath.Join(root, "scripts/selftest/run.sh"), []byte("#!/bin/bash\nwhile [ ! -f \"$DOCTOR_RELEASE\" ]; do /bin/sleep 0.1; done\nprintf '  ok: released\\nselftest: 1 passed, 0 failed\\n'\n"), 0700)
		resolve := exec.Command("python3", "-c", "import sys;print(sys.executable)")
		raw, err := resolve.Output()
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, strings.TrimSpace(string(raw)), "-B", "-c", doctorProgressDriver, filepath.Join(root, "factory-go"), mode, release) // #nosec G204 -- fixed PTY driver and literal fixture paths.
		command.Dir, command.Env = root, append(environment, "DOCTOR_RELEASE="+release)
		output, err := command.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "%s", output)
		Expect(string(output)).To(ContainSubstring("selftest: 1 passed, 0 failed"))
		Expect(string(output)).To(MatchRegexp(` in [0-9]`))
	}, Entry("non-TTY", "pipe"), Entry("TTY", "tty"))

	// per docs/adr/0086-go-native-doctor.md:91
	It("runs real retained sync and break-fix proof without models or network", func() {
		root, environment := doctorFixture()
		for _, name := range []string{"sync-opencode.sh", "sync-claude.sh", "sync-codex.sh", "factory-review-lane.sh", "selftest/run.sh"} {
			command := exec.Command("git", "show", doctorBaseline+":scripts/"+name) // #nosec G204 -- fixed frozen local sources.
			command.Dir = ".."
			raw, err := command.Output()
			Expect(err).NotTo(HaveOccurred())
			writeFixture(filepath.Join(root, "scripts", name), raw, 0700)
		}
		writeFixture(filepath.Join(filepath.Dir(root), "bin/gh"), []byte("#!/bin/bash\nexit 1\n"), 0700)
		for _, name := range []string{"curl", "wget", "npm", "opencode", "claude", "codex"} {
			writeFixture(filepath.Join(filepath.Dir(root), "bin", name), []byte("#!/bin/bash\nprintf forbidden >> \"$DOCTOR_FORBIDDEN\"\nexit 99\n"), 0700)
		}
		marker := filepath.Join(filepath.Dir(root), "forbidden")
		environment = append(environment, "DOCTOR_FORBIDDEN="+marker)
		out := doctorRunWithLimit(root, environment, false, 4*time.Minute)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(MatchRegexp(`selftest: [1-9][0-9]* passed, 0 failed`))
		Expect(out.stdout).NotTo(ContainSubstring("sync-claude.sh failed"))
		_, err := os.Stat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

func doctorFixture() (string, []string) {
	GinkgoHelper()
	root, _, environment := nativeInitFixture()
	for _, path := range []string{"scripts/factory-doctor.sh", "scripts/lib/config.sh", "scripts/lib/timing.sh", "scripts/lib/hookspath.sh"} {
		command := exec.Command("git", "show", doctorBaseline+":"+path) // #nosec G204 -- fixed local revision and fixture paths.
		command.Dir = ".."
		data, err := command.Output()
		Expect(err).NotTo(HaveOccurred())
		if path == "scripts/factory-doctor.sh" {
			path = "scripts/doctor-frozen.sh"
		}
		writeFixture(filepath.Join(root, path), data, 0700)
	}
	Expect(os.Remove(filepath.Join(root, "scripts/factory-doctor.sh"))).To(Succeed())
	writeFixture(filepath.Join(root, "factory.yaml"), []byte("test_file_patterns: '*_test.go'\ncitation_prefix: docs/\ndocs_root: docs\ncheck_command: make check\nprotected_paths: internal/core\ndecision_log: docs/DECISIONS.md\nreview_lane: off\n"), 0600)
	writeFixture(filepath.Join(root, ".github/CODEOWNERS"), []byte("/internal/core @fixture\n"), 0600)
	writeFixture(filepath.Join(root, "scripts/selftest/run.sh"), []byte("#!/bin/bash\nprintf '  ok: fixture proof\\nselftest: 1 passed, 0 failed\\n'\n"), 0700)
	return root, environment
}

func doctorRun(root string, environment []string, legacy bool, args ...string) cliResult {
	return doctorRunWithLimit(root, environment, legacy, 45*time.Second, args...)
}

func doctorRunWithLimit(root string, environment []string, legacy bool, timeout time.Duration, args ...string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	executable := filepath.Join(root, "factory-go")
	argv := append([]string{"doctor"}, args...)
	if legacy {
		executable = "/bin/bash"
		argv = append([]string{filepath.Join(root, "scripts/doctor-frozen.sh")}, args...)
	}
	command := exec.CommandContext(ctx, executable, argv...) // #nosec G204 -- compiled fixture CLI or frozen local oracle, literal argv.
	command.Dir, command.Env = root, environment
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred())
	status := 0
	if err != nil {
		var exitError *exec.ExitError
		Expect(errors.As(err, &exitError)).To(BeTrue(), "%v", err)
		status = exitError.ExitCode()
	}
	return cliResult{stdout.String(), stderr.String(), status}
}

var _ = Describe("Native Go doctor core", func() {
	// per docs/adr/0086-go-native-doctor.md:18
	It("reports healthy gates and proof without a legacy doctor script", func() {
		root, environment := doctorFixture()
		out := doctorRun(root, environment, false)
		Expect(out.status).To(BeZero(), "%+v", out)
		for _, value := range []string{"factory doctor", "Gates", "Integrity", "Proof (break/fix self-test)", "[ARMED]", "selftest: 1 passed, 0 failed", "factory doctor: healthy"} {
			Expect(out.stdout).To(ContainSubstring(value))
		}
	})
	// per docs/adr/0086-go-native-doctor.md:20
	It("diagnoses missing configuration with status one without a legacy script", func() {
		root, environment := doctorFixture()
		Expect(os.Remove(filepath.Join(root, "factory.yaml"))).To(Succeed())
		out := doctorRun(root, environment, false)
		Expect(out.status).To(Equal(1), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("factory.yaml not found"))
		Expect(out.stdout).To(ContainSubstring("factory doctor: 1 problem"))
	})
	// per docs/adr/0086-go-native-doctor.md:21
	It("reports inert choices without treating them as failure and ignores argv", func() {
		root, environment := doctorFixture()
		writeFixture(filepath.Join(root, "factory.yaml"), []byte("test_file_patterns:\ncitation_prefix:\ncheck_command:\ndecision_log:\n"), 0600)
		out := doctorRun(root, environment, false, "--help", "--bogus", "$(touch DOCTOR_INJECTION)")
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("[inert]"))
		for _, gate := range []string{"test-edit-denial", "citation-lint", "diff-aware-check"} {
			Expect(out.stdout).To(MatchRegexp(`\[inert\]\s+` + gate))
		}
		Expect(out.stdout).To(ContainSubstring("factory doctor: healthy"))
		Expect(out.stdout).NotTo(ContainSubstring("Usage:"))
		_, err := os.Stat(filepath.Join(root, "DOCTOR_INJECTION"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

func doctorConfig(root, text string) {
	GinkgoHelper()
	writeFixture(filepath.Join(root, "factory.yaml"), []byte(text), 0600)
}

var _ = Describe("Native Go doctor scratch and process safety", func() {
	// per docs/adr/0086-go-native-doctor.md:58
	DescribeTable("refuses cleanup after its guarded scratch path is replaced", func(kind string) {
		root, environment := doctorFixture()
		base := filepath.Dir(root)
		tmp := filepath.Join(base, "doctor-owned-tmp")
		outside := filepath.Join(base, "outside-scratch")
		marker := filepath.Join(base, "checked-scratch-pwd")
		Expect(os.Mkdir(tmp, 0700)).To(Succeed())
		Expect(os.Mkdir(outside, 0750)).To(Succeed())
		writeFixture(filepath.Join(outside, "KEEP"), []byte("OUTSIDE_KEEP"), 0600)
		outsideBefore, err := os.Stat(outside)
		Expect(err).NotTo(HaveOccurred())
		// This helper may mutate only a single, direct child of a dedicated
		// temporary directory. All guards precede the marker and rename.
		const replaceOwnScratch = `#!/bin/bash
set -euo pipefail
: "${DOCTOR_TEST_TMP:?}" "${DOCTOR_LIVE_ROOT:?}" "${DOCTOR_CHECKED_PWD:?}" "${DOCTOR_OUTSIDE:?}" "${DOCTOR_REPLACE_KIND:?}"
[ "$PWD" != "$DOCTOR_LIVE_ROOT" ] || exit 81
[ "${PWD%/*}" = "$DOCTOR_TEST_TMP" ] || exit 82
case "${PWD##*/}" in factory-doctor-config-?*) ;; *) exit 83 ;; esac
[ -d "$PWD" ] && [ ! -L "$PWD" ] || exit 84
[ ! -e "$PWD.retired" ] && [ ! -L "$PWD.retired" ] || exit 85
case "$DOCTOR_REPLACE_KIND" in directory|symlink) ;; *) exit 86 ;; esac
printf '%s' "$PWD" > "$DOCTOR_CHECKED_PWD"
/bin/mv "$PWD" "$PWD.retired"
if [ "$DOCTOR_REPLACE_KIND" = directory ]; then
  /bin/mkdir "$PWD"
  printf 'REPLACEMENT_KEEP' > "$PWD/KEEP"
else
  /bin/ln -s "$DOCTOR_OUTSIDE" "$PWD"
fi
exit 0
`
		writeFixture(filepath.Join(root, "scripts/sync-claude.sh"), []byte(replaceOwnScratch), 0700)
		before := nativeInitArtifacts(root)
		environment = append(environment, "TMPDIR="+tmp, "DOCTOR_TEST_TMP="+tmp, "DOCTOR_LIVE_ROOT="+root, "DOCTOR_CHECKED_PWD="+marker, "DOCTOR_OUTSIDE="+outside, "DOCTOR_REPLACE_KIND="+kind)
		out := doctorRun(root, environment, false)
		Expect(out.status).To(Equal(1), "%+v", out)
		Expect(out.stdout + out.stderr).To(ContainSubstring("scratch identity changed; cleanup refused"))
		checked, err := os.ReadFile(marker)
		Expect(err).NotTo(HaveOccurred(), "guarded helper must have reached the isolated mutation")
		replacement := string(checked)
		Expect(filepath.Dir(replacement)).To(Equal(tmp))
		Expect(filepath.Base(replacement)).To(HavePrefix("factory-doctor-config-"))
		info, err := os.Lstat(replacement)
		Expect(err).NotTo(HaveOccurred())
		if kind == "directory" {
			Expect(info.IsDir()).To(BeTrue())
			data, err := os.ReadFile(filepath.Join(replacement, "KEEP"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(Equal("REPLACEMENT_KEEP"))
		} else {
			Expect(info.Mode() & os.ModeSymlink).NotTo(BeZero())
			target, err := os.Readlink(replacement)
			Expect(err).NotTo(HaveOccurred())
			Expect(target).To(Equal(outside))
		}
		data, err := os.ReadFile(filepath.Join(outside, "KEEP"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("OUTSIDE_KEEP"))
		outsideAfter, err := os.Stat(outside)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(outsideBefore, outsideAfter)).To(BeTrue())
		Expect(outsideAfter.Mode()).To(Equal(outsideBefore.Mode()))
		Expect(nativeInitArtifacts(root)).To(Equal(before))
	}, Entry("unrelated directory replacement", "directory"), Entry("outside symlink replacement", "symlink"))

	// per docs/adr/0086-go-native-doctor.md:41
	It("gives helpers empty input even when the caller provides bytes", func() {
		root, environment := doctorFixture()
		writeFixture(filepath.Join(root, "scripts/selftest/run.sh"), []byte("#!/bin/bash\nif read -r line; then echo INPUT_LEAKED; exit 8; fi\necho 'selftest: 1 passed, 0 failed'\n"), 0700)
		command := exec.Command(filepath.Join(root, "factory-go"), "doctor") // #nosec G204 -- compiled fixture and literal argv.
		command.Dir, command.Env, command.Stdin = root, environment, strings.NewReader("CALLER_PRIVATE_INPUT\n")
		output, err := command.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "%s", output)
		Expect(string(output)).NotTo(ContainSubstring("INPUT_LEAKED"))
	})
	// per docs/adr/0086-go-native-doctor.md:44
	It("fails visibly when proof output exceeds the bounded capture", func() {
		root, environment := doctorFixture()
		writeFixture(filepath.Join(root, "scripts/selftest/run.sh"), []byte("#!/bin/bash\n/usr/bin/head -c 17825792 /dev/zero\n"), 0700)
		out := doctorRun(root, environment, false)
		Expect(out.status).To(Equal(1), "%+v", out)
		Expect(out.stdout).NotTo(ContainSubstring("factory doctor: healthy"))
		Expect(strings.ToLower(out.stdout + out.stderr)).To(Or(ContainSubstring("limit"), ContainSubstring("large"), ContainSubstring("overflow")))
	})
	// per docs/adr/0086-go-native-doctor.md:73
	It("returns nonzero when its report output pipe is already closed", func() {
		root, environment := doctorFixture()
		reader, writer, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		Expect(reader.Close()).To(Succeed())
		defer writer.Close()
		command := exec.Command(filepath.Join(root, "factory-go"), "doctor") // #nosec G204 -- compiled fixture and literal argv.
		command.Dir, command.Env, command.Stdout = root, environment, writer
		Expect(command.Run()).NotTo(Succeed())
	})
	// per docs/adr/0086-go-native-doctor.md:58
	It("removes owned scratch even when adapter directories are read-only", func() {
		root, environment := doctorFixture()
		tmp := filepath.Join(filepath.Dir(root), "doctor-tmp")
		Expect(os.Mkdir(tmp, 0700)).To(Succeed())
		path := filepath.Join(root, ".claude/readonly")
		writeFixture(filepath.Join(path, "user.json"), []byte("USER"), 0400)
		Expect(os.Chmod(path, 0500)).To(Succeed())
		DeferCleanup(os.Chmod, path, os.FileMode(0700))
		before := nativeInitArtifacts(root)
		out := doctorRun(root, append(environment, "TMPDIR="+tmp), false)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(nativeInitArtifacts(root)).To(Equal(before))
		entries, err := os.ReadDir(tmp)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty(), "doctor must remove only its owned scratch")
	})
	// per docs/adr/0086-go-native-doctor.md:50
	DescribeTable("detects adapter changes without touching the live inode or backups", func(kind string) {
		root, environment := doctorFixture()
		doctorGit(root, environment, "init", "-q")
		writeFixture(filepath.Join(root, "node_modules/PRIVATE"), []byte("EXCLUDED"), 0600)
		path := filepath.Join(root, ".claude/settings.json")
		writeFixture(path, []byte("USER_EDITS\n"), 0600)
		beforeInfo, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(root, "CLAUDE.md.replaced-by-symlink"), []byte("OLD_BACKUP"), 0600)
		body := ""
		switch kind {
		case "content":
			body = "printf generated > .claude/settings.json"
		case "mode":
			body = "chmod 755 .claude/settings.json"
		case "new":
			body = "printf new > .claude/new-adapter.json"
		case "link":
			body = "rm -f CLAUDE.md; ln -s AGENTS-other.md CLAUDE.md"
		}
		// Require a scratch cwd and excluded assets; inode/mtime assertions below
		// additionally reject restore-after-write mutation of live adapters.
		writeFixture(filepath.Join(root, "scripts/sync-claude.sh"), []byte("#!/bin/bash\n[ \"$PWD\" != \"$DOCTOR_LIVE_ROOT\" ] || exit 81\n[ ! -e node_modules/PRIVATE ] || exit 82\n[ ! -e .git ] || exit 83\n"+body+"\n"), 0700)
		before := nativeInitArtifacts(root)
		out := doctorRun(root, append(environment, "DOCTOR_LIVE_ROOT="+root), false)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("harness adapters drifted"))
		afterInfo, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(beforeInfo, afterInfo)).To(BeTrue())
		Expect(afterInfo.ModTime()).To(Equal(beforeInfo.ModTime()))
		Expect(nativeInitArtifacts(root)).To(Equal(before))
	}, Entry("content", "content"), Entry("mode", "mode"), Entry("new output", "new"), Entry("link target", "link"))

	// per docs/adr/0086-go-native-doctor.md:46
	It("reports unsuccessful sync as a warning rather than agreement", func() {
		root, environment := doctorFixture()
		writeFixture(filepath.Join(root, "scripts/sync-claude.sh"), []byte("#!/bin/bash\nexit 9\n"), 0700)
		out := doctorRun(root, environment, false)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("[warn]"))
		Expect(out.stdout).NotTo(ContainSubstring("adapters match"))
	})

	// per docs/adr/0086-go-native-doctor.md:59
	DescribeTable("refuses unsafe adapter input with visible incomplete-inspection diagnostics", func(kind string) {
		root, environment := doctorFixture()
		path := filepath.Join(root, ".claude/unsafe")
		outside := filepath.Join(filepath.Dir(root), "private")
		writeFixture(outside, []byte("PRIVATE_NEVER_REPORTED"), 0600)
		switch kind {
		case "link":
			Expect(os.Symlink(outside, path)).To(Succeed())
		case "fifo":
			Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
		case "large":
			writeFixture(path, bytes.Repeat([]byte("x"), (16<<20)+1), 0600)
		case "entries":
			for i := 0; i < 4097; i++ {
				writeFixture(filepath.Join(root, ".claude/many", strconv.Itoa(i)), nil, 0600)
			}
		case "total":
			for i := 0; i < 5; i++ {
				writeFixture(filepath.Join(root, ".claude", "large-"+strconv.Itoa(i)), bytes.Repeat([]byte("x"), 14<<20), 0600)
			}
		}
		out := doctorRun(root, environment, false)
		Expect(out.stdout + out.stderr).To(Or(ContainSubstring("[warn]"), ContainSubstring("[FAIL]")))
		Expect(out.stdout).NotTo(ContainSubstring("adapters match"))
		Expect(out.stdout + out.stderr).NotTo(ContainSubstring("PRIVATE_NEVER_REPORTED"))
	}, Entry("outside link", "link"), Entry("FIFO", "fifo"), Entry("file size limit", "large"), Entry("tree entry limit", "entries"), Entry("total snapshot limit", "total"))

	// per docs/adr/0086-go-native-doctor.md:45
	DescribeTable("cancels an observed proof child and returns a diagnostic status one", func(signal syscall.Signal) {
		root, environment := doctorFixture()
		marker := filepath.Join(filepath.Dir(root), "proof-pid")
		writeFixture(filepath.Join(root, "scripts/selftest/run.sh"), []byte("#!/bin/bash\nprintf '%s' \"$$\" > \"$DOCTOR_PID_MARKER\"\nexec /bin/sleep 30\n"), 0700)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		command := exec.CommandContext(ctx, filepath.Join(root, "factory-go"), "doctor") // #nosec G204 -- compiled test fixture.
		command.Dir, command.Env = root, append(environment, "DOCTOR_PID_MARKER="+marker)
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
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
		Expect(strings.ToLower(stdout.String() + stderr.String())).To(ContainSubstring("cancel"))
		Expect(stdout.String()).NotTo(ContainSubstring("factory doctor: healthy"))
		Expect(syscall.Kill(pid, 0)).To(Equal(syscall.ESRCH))
		pid = 0
	}, Entry("SIGINT", syscall.SIGINT), Entry("SIGTERM", syscall.SIGTERM))
})

func doctorGit(root string, environment []string, args ...string) {
	GinkgoHelper()
	command := exec.Command("git", args...) // #nosec G204 -- literal fixture Git operations.
	command.Dir, command.Env = root, environment
	output, err := command.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "%s", output)
}

func doctorNormalize(text string) string {
	return regexp.MustCompile(`(?m)^(  \[ ok \]\s+selftest:[^\n]+) in [0-9hms .]+$`).ReplaceAllString(text, "$1 in <duration>")
}

var _ = Describe("Native Go doctor classifications", func() {
	// per docs/adr/0086-go-native-doctor.md:23
	It("discovers the adopter Git root from a nested cwd", func() {
		root, environment := doctorFixture()
		doctorGit(root, environment, "init", "-q")
		nested := filepath.Join(root, "nested/child")
		Expect(os.MkdirAll(nested, 0700)).To(Succeed())
		command := exec.Command(filepath.Join(root, "factory-go"), "doctor") // #nosec G204 -- compiled fixture and literal argv.
		command.Dir, command.Env = nested, environment
		output, err := command.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "%s", output)
		Expect(string(output)).To(ContainSubstring("repo:   " + root + "\n"))
		Expect(string(output)).To(ContainSubstring("implementer cannot edit:"))
	})
	// per docs/adr/0086-go-native-doctor.md:30
	DescribeTable("reports wiki content and staleness mode", func(fresh bool) {
		root, environment := doctorFixture()
		text := "wiki_root: knowledge\nwiki_staleness: false\n"
		if fresh {
			text = "wiki_root: knowledge\nwiki_staleness: true\n"
		}
		doctorConfig(root, text)
		writeFixture(filepath.Join(root, "knowledge/page.md"), []byte("# Page\n"), 0600)
		out := doctorRun(root, environment, false)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(MatchRegexp(`\[ARMED\]\s+wiki-lint`))
		if fresh {
			Expect(out.stdout).To(ContainSubstring("cited, reachable, fresh"))
		} else {
			Expect(out.stdout).To(ContainSubstring("staleness opt-in"))
		}
	}, Entry("default staleness", false), Entry("enabled staleness", true))
	// per docs/adr/0086-go-native-doctor.md:24
	It("retains shell IFS word boundaries for pack and protected-path values", func() {
		root, environment := doctorFixture()
		doctorConfig(root, "language_packs: go\u00a0java\nprotected_paths: left\u00a0right\n")
		writeFixture(filepath.Join(root, ".github/CODEOWNERS"), []byte("left @fixture\nright @fixture\n"), 0600)
		out := doctorRun(root, environment, false)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).NotTo(ContainSubstring("pack:go"))
		Expect(out.stdout).NotTo(ContainSubstring("pack:java"))
		Expect(out.stdout).To(ContainSubstring("protected path(s) not in CODEOWNERS: left\u00a0right"))
	})
	// per docs/adr/0086-go-native-doctor.md:23
	It("preserves lexical legacy siblings for explicit config paths through symlink dot-dot", func() {
		root, environment := doctorFixture()
		Expect(os.MkdirAll(filepath.Join(root, "real/child"), 0700)).To(Succeed())
		Expect(os.Symlink(filepath.Join(root, "real/child"), filepath.Join(root, "link"))).To(Succeed())
		writeFixture(filepath.Join(root, "real/factory.yaml"), []byte("project_name: fixture\n"), 0600)
		writeFixture(filepath.Join(root, "real/factory.config"), []byte("REVIEW_LANE=on\n"), 0600)
		writeFixture(filepath.Join(root, "factory.config"), []byte("REVIEW_LANE=off\n"), 0600)
		environment = append(environment, "FACTORY_CONFIG="+root+"/link/../factory.yaml")
		out := doctorRun(root, environment, false)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("review lane            advisory PR review, secret present"))
	})
	// per docs/adr/0086-go-native-doctor.md:90
	DescribeTable("matches the frozen report and status", func(kind string) {
		root, environment := doctorFixture()
		switch kind {
		case "inert":
			doctorConfig(root, "project_name: fixture\n")
		case "stale":
			writeFixture(filepath.Join(root, "memory/.parity-stale"), nil, 0600)
			writeFixture(filepath.Join(root, "memory/PENDING-LESSONS.md"), []byte("pending"), 0600)
		case "missing":
			Expect(os.Remove(filepath.Join(root, "scripts/hooks/test-edit-denial.sh"))).To(Succeed())
		case "nonexecutable":
			Expect(os.Chmod(filepath.Join(root, "scripts/hooks/test-edit-denial.sh"), 0600)).To(Succeed())
		case "proof":
			writeFixture(filepath.Join(root, "scripts/selftest/run.sh"), []byte("#!/bin/bash\necho BROKEN_PROOF >&2\necho 'selftest: 0 passed, 1 failed'\nexit 7\n"), 0700)
		case "skip":
			Expect(os.RemoveAll(filepath.Join(root, ".claude"))).To(Succeed())
		}
		baseline := doctorRun(root, environment, true)
		actual := doctorRun(root, environment, false)
		Expect(actual.status).To(Equal(baseline.status), "%+v", actual)
		Expect(actual.stderr).To(Equal(baseline.stderr))
		Expect(doctorNormalize(actual.stdout)).To(Equal(doctorNormalize(baseline.stdout)))
	}, Entry("healthy", "healthy"), Entry("inert", "inert"), Entry("stale", "stale"), Entry("missing core hook", "missing"), Entry("nonexecutable hook", "nonexecutable"), Entry("failed proof", "proof"), Entry("optional adapter absent", "skip"))

	// per docs/adr/0086-go-native-doctor.md:30
	DescribeTable("classifies pack wiring independently of presence", func(lang, hook string, present, wired bool) {
		root, environment := doctorFixture()
		check := "make check"
		if wired {
			check += " scripts/hooks/" + hook
		}
		doctorConfig(root, "language_packs: "+lang+"\ncheck_command: "+check+"\n")
		path := filepath.Join(root, "scripts/hooks", hook)
		_ = os.Remove(path)
		if present {
			writeFixture(path, []byte("#!/bin/bash\nexit 0\n"), 0700)
		}
		out := doctorRun(root, environment, false)
		if wired && !present {
			Expect(out.status).To(Equal(1))
			Expect(out.stdout).To(ContainSubstring(hook + " is in check_command but missing"))
		} else {
			Expect(out.status).To(BeZero(), "%+v", out)
			marker := "[inert]"
			if wired {
				marker = "[ARMED]"
			}
			Expect(out.stdout).To(MatchRegexp(regexp.QuoteMeta(marker) + `\s+pack:` + lang + ` dialect gate`))
		}
	}, Entry("Go armed", "go", "ginkgo-only-check.sh", true, true), Entry("Go inert present", "go", "ginkgo-only-check.sh", true, false), Entry("Go wired missing", "go", "ginkgo-only-check.sh", false, true), Entry("Java deliberately absent", "java", "junit5-only-check.sh", false, false), Entry("Java armed", "java", "junit5-only-check.sh", true, true), Entry("TypeScript armed", "typescript", "vitest-only-check.sh", true, true))

	// per docs/adr/0086-go-native-doctor.md:23
	DescribeTable("uses explicit inert configuration with first-key and review environment precedence", func(relative bool) {
		root, environment := doctorFixture()
		path := filepath.Join(root, "custom.yaml")
		writeFixture(path, []byte("test_file_patterns: FIRST\ntest_file_patterns: SECOND\ncheck_command: $(touch DOCTOR_INJECTION)\nreview_lane: on\n"), 0600)
		selected := path
		if relative {
			selected = "custom.yaml"
		}
		environment = append(environment, "FACTORY_CONFIG="+selected, "REVIEW_LANE=off")
		out := doctorRun(root, environment, false)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("implementer cannot edit: FIRST"))
		Expect(out.stdout).NotTo(ContainSubstring("SECOND"))
		Expect(out.stdout).To(ContainSubstring("$(touch DOCTOR_INJECTION)"))
		Expect(out.stdout).To(ContainSubstring("review lane            off"))
		_, err := os.Stat(filepath.Join(root, "DOCTOR_INJECTION"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("absolute", false), Entry("relative", true))

	// per docs/adr/0086-go-native-doctor.md:35
	It("checks CODEOWNERS as literal references rather than regular expressions", func() {
		root, environment := doctorFixture()
		doctorConfig(root, "protected_paths: internal/[abc]\n")
		writeFixture(filepath.Join(root, ".github/CODEOWNERS"), []byte("/internal/a @fixture\n"), 0600)
		out := doctorRun(root, environment, false)
		Expect(out.status).To(BeZero())
		Expect(out.stdout).To(ContainSubstring("protected path(s) not in CODEOWNERS: internal/[abc]"))
		Expect(out.stdout).NotTo(ContainSubstring("CODEOWNERS references every protected path"))
	})

	// per docs/adr/0086-go-native-doctor.md:32
	DescribeTable("reports effective Git hook state even without the old optional library", func(state, diagnostic string) {
		root, environment := doctorFixture()
		doctorGit(root, environment, "init", "-q")
		Expect(os.Remove(filepath.Join(root, "scripts/lib/hookspath.sh"))).To(Succeed())
		switch state {
		case "armed", "inert":
			doctorGit(root, environment, "config", "core.hooksPath", ".githooks")
		case "hijacked":
			doctorGit(root, environment, "config", "core.hooksPath", "elsewhere")
			writeFixture(filepath.Join(root, "elsewhere/pre-push"), []byte("#!/bin/bash\nexit 0\n"), 0700)
		}
		if state == "inert" {
			Expect(os.Chmod(filepath.Join(root, ".githooks/pre-push"), 0600)).To(Succeed())
		}
		out := doctorRun(root, environment, false)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(ContainSubstring(diagnostic))
	}, Entry("armed", "armed", "git resolves the pre-push hook"), Entry("inert", "inert", "pre-push hook is not executable"), Entry("hijacked", "hijacked", "core.hooksPath redirects git"), Entry("absent", "absent", "push gate not installed"))

	// per docs/adr/0086-go-native-doctor.md:47
	DescribeTable("never claims review credentials are present after a failed status lookup", func(body string, healthy bool) {
		root, environment := doctorFixture()
		environment = append(environment, "REVIEW_LANE=on")
		writeFixture(filepath.Join(root, "scripts/factory-review-lane.sh"), []byte("#!/bin/bash\nif [ \"$1\" = secret-name ]; then echo FIXTURE_SECRET; exit 0; fi\n"+body), 0700)
		out := doctorRun(root, environment, false)
		Expect(out.status).To(BeZero(), "%+v", out)
		if healthy {
			Expect(out.stdout).To(ContainSubstring("secret present"))
		} else {
			Expect(out.stdout).NotTo(ContainSubstring("secret present"))
			Expect(out.stdout).To(ContainSubstring("unverified"))
		}
	}, Entry("empty success", "exit 0\n", true), Entry("pending", "echo missing\nexit 0\n", false), Entry("failed empty", "exit 7\n", false))
})
