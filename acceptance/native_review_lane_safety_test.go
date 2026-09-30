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

func laneWorkflow(cwd string) string {
	return filepath.Join(cwd, ".github/workflows/adversarial-review.yml")
}

func laneNoStaging(cwd string) {
	GinkgoHelper()
	entries, err := os.ReadDir(filepath.Join(cwd, ".github/workflows"))
	if os.IsNotExist(err) {
		return
	}
	Expect(err).NotTo(HaveOccurred())
	for _, entry := range entries {
		Expect(entry.Name()).To(Equal("adversarial-review.yml"), "unexpected staging artifact")
	}
}

const reviewLaneTTYDriver = `import errno,os,pty,select,signal,sys,time,tty
binary,mode,answer,output=sys.argv[1:]
pid,master=pty.fork()
if pid==0:
 if mode=='redirect':
  fd=os.open(output,os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600);os.dup2(fd,1);os.close(fd)
 if mode=='oversize': tty.setraw(0)
 os.execv(binary,[binary,'review-lane','enable'])
os.set_blocking(master,False)
data=b'';pending=b'';sent=False;status=None
deadline=time.monotonic()+10
try:
 while time.monotonic()<deadline:
  readable,writable,_=select.select([master],[master] if pending else [],[],0.02)
  if writable:
   try: pending=pending[os.write(master,pending):]
   except BlockingIOError: pass
  if readable:
   try: chunk=os.read(master,65536)
   except OSError as e:
    if e.errno==errno.EIO: break
    raise
   if not chunk: break
   data+=chunk
   if not sent and b'Model to use' in data:
    sent=True
    if mode=='cancel': os.kill(pid,signal.SIGTERM)
    elif mode=='eof': pending=b'\x04'
    else: pending=answer.encode()+b'\n'
  child,code=os.waitpid(pid,os.WNOHANG)
  if child: status=code;break
 reap_deadline=min(deadline,time.monotonic()+1)
 while status is None and time.monotonic()<reap_deadline:
  child,code=os.waitpid(pid,os.WNOHANG)
  if child: status=code;break
  time.sleep(0.01)
 if status is None:
  os.killpg(pid,signal.SIGKILL);_,status=os.waitpid(pid,0)
  raise RuntimeError('review lane did not exit promptly')
 os.write(1,data)
 if mode=='redirect': os.write(1,open(output,'rb').read())
 sys.exit(os.waitstatus_to_exitcode(status))
finally:
 if status is None:
  try: os.killpg(pid,signal.SIGKILL);os.waitpid(pid,0)
  except ProcessLookupError: pass
 os.close(master)
`

var _ = Describe("Native Go review-lane terminal prompt", func() {
	// per docs/adr/0089-go-native-review-lane.md:43
	DescribeTable("uses only the controlling terminal and handles cancellation", func(mode, answer, model string, status int, prompted bool) {
		root, cwd, environment := reviewLaneFixture()
		if mode == "preset" {
			environment = append(environment, "REVIEW_MODEL="+model)
		}
		if mode == "oversize" {
			answer = strings.Repeat("x", 65537)
		}
		out := reportProcess(cwd, environment, "python3", []string{"-B", "-c", reviewLaneTTYDriver, filepath.Join(root, "factory"), mode, answer, filepath.Join(cwd, "captured-output")})
		Expect(out.status).To(Equal(status), "%+v", out)
		if prompted {
			Expect(out.stdout).To(ContainSubstring("Model to use"))
		} else {
			Expect(out.stdout).NotTo(ContainSubstring("Model to use"))
		}
		if status == 0 {
			data, err := os.ReadFile(filepath.Join(cwd, "factory.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("review_model: \"" + model + "\""))
			_, err = os.Stat(laneWorkflow(cwd))
			Expect(err).NotTo(HaveOccurred())
		} else {
			_, err := os.Stat(laneWorkflow(cwd))
			Expect(os.IsNotExist(err)).To(BeTrue())
			laneNoStaging(cwd)
		}
	}, Entry("model answer", "answer", "provider/model", "provider/model", 0, true), Entry("empty answer", "answer", "", "", 0, true), Entry("EOF", "eof", "", "", 0, true), Entry("preset", "preset", "", "chosen/model", 0, false), Entry("redirected stdout", "redirect", "", "", 0, false), Entry("SIGTERM", "cancel", "", "", 1, true), Entry("oversized answer", "oversize", "", "", 1, true))
})

var _ = Describe("Native Go review-lane safety", func() {
	DescribeTable("preserves the managed workflow when disable cannot update configuration", func(missing bool) {
		root, cwd, environment := reviewLaneFixture()
		path := laneWorkflow(cwd)
		body := []byte(reviewLaneHeader + "\nname: previous\n")
		writeFixture(path, body, 0600)
		configPath := filepath.Join(cwd, "factory.yaml")
		if missing {
			Expect(os.Remove(configPath)).To(Succeed())
		} else {
			Expect(os.Chmod(configPath, 0400)).To(Succeed())
		}
		out := reviewLaneRun(root, cwd, environment, "disable")
		Expect(out.status).To(Equal(1), "%+v", out)
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(body))
	}, Entry("missing", true), Entry("read-only", false))
	DescribeTable("reports output failure without undoing a completed publication", func(command string) {
		root, cwd, environment := reviewLaneFixture()
		read, write, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		Expect(read.Close()).To(Succeed())
		DeferCleanup(write.Close)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		process := exec.CommandContext(ctx, filepath.Join(root, "factory"), "review-lane", command) // #nosec G204 -- compiled fixture, fixed command selection.
		process.Dir, process.Env, process.Stdout = cwd, environment, write
		var stderr bytes.Buffer
		process.Stderr = &stderr
		err = process.Run()
		Expect(ctx.Err()).NotTo(HaveOccurred())
		var exited *exec.ExitError
		Expect(errors.As(err, &exited)).To(BeTrue())
		Expect(exited.ExitCode()).To(Equal(1))
		Expect(stderr.String()).To(ContainSubstring("output"))
		if command == "enable" {
			data, err := os.ReadFile(laneWorkflow(cwd))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(HavePrefix(reviewLaneHeader + "\n"))
			laneNoStaging(cwd)
		}
	}, Entry("status", "status"), Entry("enable", "enable"))
	// per docs/adr/0089-go-native-review-lane.md:52
	DescribeTable("rejects invalid secret identifiers before changing configuration", func(secret string) {
		root, cwd, environment := reviewLaneFixture()
		before, err := os.ReadFile(filepath.Join(cwd, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		out := reviewLaneRun(root, cwd, environment, "enable", secret)
		Expect(out.status).To(Equal(1), "%+v", out)
		after, err := os.ReadFile(filepath.Join(cwd, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		_, err = os.Lstat(laneWorkflow(cwd))
		Expect(os.IsNotExist(err)).To(BeTrue())
		laneNoStaging(cwd)
	}, Entry("numeric prefix", "9KEY"), Entry("reserved prefix", "github_key"), Entry("expression", "KEY }}${{ github.token"), Entry("sed metacharacters", "KEY&|"), Entry("newline", "KEY\nOTHER"), Entry("space", "KEY NAME"))
	DescribeTable("rejects model values that cannot round-trip through the flat setter", func(model string) {
		root, cwd, environment := reviewLaneFixture()
		environment = append(environment, "REVIEW_MODEL="+model)
		before, err := os.ReadFile(filepath.Join(cwd, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		out := reviewLaneRun(root, cwd, environment, "enable")
		Expect(out.status).To(Equal(1), "%+v", out)
		after, err := os.ReadFile(filepath.Join(cwd, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		_, err = os.Lstat(laneWorkflow(cwd))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("quote", "model\"tail"), Entry("newline", "model\nreview_lane: on"), Entry("carriage return", "model\rtail"))
	// per docs/adr/0089-go-native-review-lane.md:64
	DescribeTable("preserves workflows without the exact first-line marker", func(command, body string) {
		root, cwd, environment := reviewLaneFixture()
		path := laneWorkflow(cwd)
		writeFixture(path, []byte(body), 0644)
		out := reviewLaneRun(root, cwd, environment, command)
		Expect(out.status).To(Equal(1), "%+v", out)
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(after)).To(Equal(body))
		if command == "disable" {
			data, err := os.ReadFile(filepath.Join(cwd, "factory.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("review_lane: \"off\""))
		}
		laneNoStaging(cwd)
	}, Entry("enable substring", "enable", "# adopter\n"+reviewLaneHeader+"\n"), Entry("disable substring", "disable", "# adopter\n"+reviewLaneHeader+"\n"), Entry("enable extended marker", "enable", reviewLaneHeader+" extra\n"), Entry("disable extended marker", "disable", reviewLaneHeader+" extra\n"))
	DescribeTable("does not follow unsafe workflow paths", func(command, kind string) {
		root, cwd, environment := reviewLaneFixture()
		outside := filepath.Join(filepath.Dir(root), "outside-lane")
		sentinel := []byte(reviewLaneHeader + "\nname: KEEP\n")
		writeFixture(filepath.Join(outside, "adversarial-review.yml"), sentinel, 0600)
		path := laneWorkflow(cwd)
		Expect(os.MkdirAll(filepath.Dir(path), 0700)).To(Succeed())
		switch kind {
		case "parent":
			Expect(os.Remove(filepath.Dir(path))).To(Succeed())
			Expect(os.Symlink(outside, filepath.Dir(path))).To(Succeed())
		case "link":
			Expect(os.Symlink(filepath.Join(outside, "adversarial-review.yml"), path)).To(Succeed())
		case "hardlink":
			Expect(os.Link(filepath.Join(outside, "adversarial-review.yml"), path)).To(Succeed())
		case "fifo":
			Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
		case "directory":
			Expect(os.Mkdir(path, 0700)).To(Succeed())
		}
		out := reviewLaneRun(root, cwd, environment, command)
		Expect(out.status).To(Equal(1), "%+v", out)
		data, err := os.ReadFile(filepath.Join(outside, "adversarial-review.yml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(data).To(Equal(sentinel))
		_, err = os.Lstat(path)
		Expect(err).NotTo(HaveOccurred(), "unsafe path was removed")
	}, Entry("enable parent link", "enable", "parent"), Entry("disable parent link", "disable", "parent"), Entry("enable leaf link", "enable", "link"), Entry("disable leaf link", "disable", "link"), Entry("enable hardlink", "enable", "hardlink"), Entry("disable hardlink", "disable", "hardlink"), Entry("enable fifo", "enable", "fifo"), Entry("disable fifo", "disable", "fifo"), Entry("enable directory", "enable", "directory"), Entry("disable directory", "disable", "directory"))
	// per docs/adr/0089-go-native-review-lane.md:72
	DescribeTable("never publishes a new workflow after a failed prerequisite", func(kind string) {
		root, cwd, environment := reviewLaneFixture()
		configPath := filepath.Join(cwd, "factory.yaml")
		template := filepath.Join(root, "packs/review-lane/review-pr.yml")
		switch kind {
		case "missing config":
			Expect(os.Remove(configPath)).To(Succeed())
		case "linked config":
			outside := filepath.Join(filepath.Dir(root), "outside.yaml")
			Expect(os.Rename(configPath, outside)).To(Succeed())
			Expect(os.Symlink(outside, configPath)).To(Succeed())
		case "missing template":
			Expect(os.Remove(template)).To(Succeed())
		case "missing placeholder":
			writeFixture(template, []byte("name: invalid\n"), 0600)
		case "large template":
			writeFixture(template, bytes.Repeat([]byte{'x'}, (16<<20)+1), 0600)
		}
		out := reviewLaneRun(root, cwd, environment, "enable")
		Expect(out.status).To(Equal(1), "%+v", out)
		_, err := os.Lstat(laneWorkflow(cwd))
		Expect(os.IsNotExist(err)).To(BeTrue())
		laneNoStaging(cwd)
	}, Entry("missing config", "missing config"), Entry("linked config", "linked config"), Entry("missing template", "missing template"), Entry("missing placeholder", "missing placeholder"), Entry("large template", "large template"))
	It("publishes a complete replacement without accumulating staging or backup files", func() {
		root, cwd, environment := reviewLaneFixture()
		path := laneWorkflow(cwd)
		writeFixture(path, []byte(reviewLaneHeader+"\nold body\n"), 0600)
		for range 2 {
			out := reviewLaneRun(root, cwd, environment, "enable", "NEW_KEY")
			Expect(out.status).To(BeZero(), "%+v", out)
			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(Equal(reviewLaneHeader + "\nname: advisory review\nsecret: NEW_KEY\n"))
			info, err := os.Stat(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0644)))
			laneNoStaging(cwd)
		}
	})
	DescribeTable("cancels a started secret lookup and reaps its child", func(signal syscall.Signal) {
		root, cwd, environment := reviewLaneFixture()
		marker := filepath.Join(cwd, "gh-ready")
		bin := filepath.Join(filepath.Dir(root), "lane-bin")
		writeFixture(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nprintf '%s' \"$$\" > \"$LANE_READY\"\nexec /bin/sleep 30\n"), 0700)
		environment = append(environment, "REVIEW_LANE=on", "LANE_READY="+marker)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		DeferCleanup(cancel)
		command := exec.CommandContext(ctx, filepath.Join(root, "factory"), "review-lane", "pending") // #nosec G204 -- compiled fixture command, literal arguments.
		command.Dir, command.Env = cwd, environment
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
				_ = syscall.Kill(pid, syscall.SIGKILL)
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
	}, Entry("SIGINT", syscall.SIGINT), Entry("SIGTERM", syscall.SIGTERM))
})
