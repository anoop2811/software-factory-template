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

const metricsTTYDriver = `import errno,os,pty,select,signal,subprocess,sys,time
master,slave=pty.openpty()
p=subprocess.Popen(sys.argv[1:],stdin=subprocess.DEVNULL,stdout=slave,stderr=slave,start_new_session=True)
os.close(slave)
deadline=time.monotonic()+9
while time.monotonic()<deadline:
 ready,_,_=select.select([master],[],[],0.05)
 if ready:
  try: data=os.read(master,65536)
  except OSError as e:
   if e.errno==errno.EIO: break
   raise
  if not data: break
  os.write(1,data)
 elif p.poll() is not None: break
else:
 os.killpg(p.pid,signal.SIGKILL);p.wait();raise RuntimeError('metrics timed out')
code=p.wait(timeout=1)
os.close(master)
sys.exit(code)
`

func metricsTTY(root, cwd string, environment []string, args ...string) cliResult {
	GinkgoHelper()
	resolve := exec.Command("python3", "-c", "import sys;print(sys.executable)")
	raw, err := resolve.Output()
	Expect(err).NotTo(HaveOccurred())
	argv := []string{"-B", "-c", metricsTTYDriver, filepath.Join(root, "factory"), "metrics"}
	return reportProcess(cwd, environment, strings.TrimSpace(string(raw)), append(argv, args...))
}

var _ = Describe("Native Go metrics browser boundary", func() {
	DescribeTable("launches only on the permitted output surface with literal arguments", func(mode string, permitted bool) {
		root, cwd, environment := metricsFixture()
		marker := filepath.Join(cwd, "browser-marker")
		bin := filepath.Join(cwd, "browser-bin")
		browser := filepath.Join(bin, "browser with spaces")
		body := []byte("#!/bin/bash\nprintf '%s\\n' \"$$\" \"$@\" > \"$METRICS_BROWSER_MARKER\"\nexec /bin/sleep 30\n")
		writeFixture(browser, body, 0700)
		writeFixture(filepath.Join(bin, "fixture-browser"), body, 0700)
		writeFixture(filepath.Join(bin, "open"), body, 0700)
		var inheritedPath string
		for _, value := range environment {
			if strings.HasPrefix(value, "PATH=") {
				inheritedPath = strings.TrimPrefix(value, "PATH=")
			}
		}
		environment = append(environment, "PATH="+bin+string(os.PathListSeparator)+inheritedPath, "CI=", "BROWSER="+browser, "METRICS_BROWSER_MARKER="+marker)
		args := []string{"--html"}
		switch mode {
		case "arguments":
			environment = append(environment, "BROWSER=fixture-browser --flag $(touch DANGER)")
		case "fallback":
			environment = append(environment, "BROWSER=missing-fixture-browser")
		case "CI":
			environment = append(environment, "CI=true")
		case "no-open":
			args = append(args, "--no-open")
		case "json":
			args = []string{"--json"}
		}
		pid := 0
		DeferCleanup(func() {
			if pid == 0 {
				if data, err := os.ReadFile(marker); err == nil {
					first, _, _ := strings.Cut(string(data), "\n")
					pid, _ = strconv.Atoi(first)
				}
			}
			if pid > 0 {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		})
		var out cliResult
		if mode == "pipe" {
			out = metricsRun(root, cwd, environment, args...)
		} else {
			out = metricsTTY(root, cwd, environment, args...)
		}
		Expect(out.status).To(BeZero(), "%+v", out)
		if permitted {
			var data []byte
			Eventually(func() error { var err error; data, err = os.ReadFile(marker); return err }, 3*time.Second).Should(Succeed())
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			var err error
			pid, err = strconv.Atoi(lines[0])
			Expect(err).NotTo(HaveOccurred())
			Expect(lines[len(lines)-1]).To(Equal(filepath.Join(cwd, ".factory/metrics.html")))
			Expect(out.stdout).To(ContainSubstring("opening it"))
			if mode == "arguments" {
				Expect(lines[1 : len(lines)-1]).To(Equal([]string{"--flag", "$(touch", "DANGER)"}))
			}
		} else {
			_, err := os.Stat(marker)
			Expect(os.IsNotExist(err)).To(BeTrue())
			Expect(out.stdout).NotTo(ContainSubstring("opening it"))
		}
		_, err := os.Stat(filepath.Join(cwd, "DANGER"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("whole executable path", "path", true), Entry("literal arguments", "arguments", true), Entry("fallback opener", "fallback", true), Entry("CI terminal", "CI", false), Entry("explicit no-open", "no-open", false), Entry("pipe", "pipe", false), Entry("JSON terminal", "json", false))
})

var _ = Describe("Native Go metrics publication safety", func() {
	It("cancels after output starts while the reader keeps the pipe blocked", func() {
		root, cwd, environment := metricsFixture()
		var events strings.Builder
		for i := range 1000 {
			events.WriteString("9999-01-01T01:00:00Z\t" + strings.Repeat("x", 256) + strconv.Itoa(i) + "\tr\n")
		}
		writeFixture(filepath.Join(cwd, ".factory/events.log"), []byte(events.String()), 0600)
		read, write, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(read.Close)
		DeferCleanup(write.Close)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		command := exec.CommandContext(ctx, filepath.Join(root, "factory"), "metrics", "--json") // #nosec G204 -- compiled fixture.
		command.Dir, command.Env, command.Stdout = cwd, environment, write
		var stderr bytes.Buffer
		command.Stderr = &stderr
		Expect(command.Start()).To(Succeed())
		done := make(chan error, 1)
		go func() { defer close(done); done <- command.Wait() }()
		DeferCleanup(func() { cancel(); Eventually(done, 3*time.Second).Should(BeClosed()) })
		Expect(read.SetReadDeadline(time.Now().Add(5 * time.Second))).To(Succeed())
		first := make([]byte, 1)
		_, err = read.Read(first)
		Expect(err).NotTo(HaveOccurred(), "child must begin its JSON write before cancellation")
		Expect(command.Process.Signal(syscall.SIGTERM)).To(Succeed())
		var waitErr error
		Eventually(done, 3*time.Second).Should(Receive(&waitErr))
		var exited *exec.ExitError
		Expect(errors.As(waitErr, &exited)).To(BeTrue())
		Expect(exited.ExitCode()).To(Equal(1))
		Expect(stderr.String()).To(ContainSubstring("cancel"))
	})
	DescribeTable("bounds derived output before publication", func(kind string) {
		root, cwd, environment := metricsFixture()
		args := []string{"--html", "--no-open"}
		switch kind {
		case "markers":
			writeFixture(filepath.Join(root, "templates/metrics.html"), []byte("/*__FACTORY_METRICS_JSON__*/null /*__FACTORY_METRICS_JSON__*/null"), 0600)
		case "page":
			template := "/*__FACTORY_METRICS_JSON__*/null" + strings.Repeat(" ", (16<<20)-100)
			writeFixture(filepath.Join(root, "templates/metrics.html"), []byte(template), 0600)
		case "metadata":
			rows := strings.TrimSuffix(strings.Repeat(`{"task":"same","score":1},`, 2100), ",")
			data := `{"harness":"` + strings.Repeat("x", 8192) + `","tasks":[` + rows + `]}`
			writeFixture(filepath.Join(cwd, "eval/results/repeated-baseline.json"), []byte(data), 0600)
			args = []string{"--json"}
		case "rows":
			rows := strings.TrimSuffix(strings.Repeat(`{"task":"same","score":1},`, 4097), ",")
			writeFixture(filepath.Join(cwd, "eval/results/many-baseline.json"), []byte(`{"tasks":[`+rows+`]}`), 0600)
		}
		path := filepath.Join(cwd, ".factory/metrics.html")
		writeFixture(path, []byte("KEEP"), 0600)
		out := metricsRun(root, cwd, environment, args...)
		Expect(out.status).NotTo(BeZero(), "stderr=%s", out.stderr)
		page, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(page)).To(Equal("KEEP"))
	}, Entry("duplicate placeholders", "markers"), Entry("generated HTML", "page"), Entry("repeated metadata", "metadata"), Entry("task rows", "rows"))
	It("preserves frozen text alignment for Unicode gate names", func() {
		root, cwd, environment := metricsFixture()
		writeFixture(filepath.Join(cwd, ".factory/events.log"), []byte("9999-01-01T01:00:00Z\tgâté-漢\treason\n"), 0600)
		want := metricsLegacy(root, cwd, environment)
		Expect(want.status).To(BeZero(), "%+v", want)
		got := metricsRun(root, cwd, environment)
		Expect(got).To(Equal(want))
	})
	It("counts directory hook entries without trying to read their contents", func() {
		root, cwd, environment := metricsFixture()
		directory := filepath.Join(cwd, "scripts/hooks/directory.sh")
		Expect(os.Mkdir(directory, 0700)).To(Succeed())
		Expect(os.Symlink(directory, filepath.Join(cwd, "scripts/hooks/link.sh"))).To(Succeed())
		out := metricsRun(root, cwd, environment, "--json")
		Expect(out.status).To(BeZero(), "%+v", out)
		enforcement := metricsJSON(out.stdout)["enforcement"].(map[string]any)
		Expect(enforcement["gates_installed"]).To(Equal(float64(3)))
		Expect(enforcement["gates_reporting"]).To(Equal(float64(0)))
		Expect(enforcement["gates_mute"]).To(Equal(float64(0)))
	})
	It("does not invent a nonzero gate exit across separate lines", func() {
		root, cwd, environment := metricsFixture()
		writeFixture(filepath.Join(cwd, "scripts/hooks/not-blocking.sh"), []byte("#!/bin/sh\nexit\n1\n"), 0600)
		writeFixture(filepath.Join(cwd, "scripts/hooks/blocking.sh"), []byte("#!/bin/sh\nexit\t2\n"), 0600)
		out := metricsRun(root, cwd, environment, "--json")
		Expect(out.status).To(BeZero(), "%+v", out)
		enforcement := metricsJSON(out.stdout)["enforcement"].(map[string]any)
		Expect(enforcement["gates_mute"]).To(Equal(float64(1)))
	})
	It("rejects an out-of-range numeric window", func() {
		root, cwd, environment := metricsFixture()
		out := metricsRun(root, cwd, environment, "--days=2147483648", "--json")
		Expect(out.status).To(Equal(2), "%+v", out)
		Expect(out.stderr).To(ContainSubstring("days"))
	})
	It("keeps valid eval results when another baseline is malformed", func() {
		root, cwd, environment := metricsFixture()
		writeFixture(filepath.Join(cwd, "eval/results/bad-baseline.json"), []byte(`{"tasks":`), 0600)
		writeFixture(filepath.Join(cwd, "eval/results/good-baseline.json"), []byte(`{"harness":"codex","tasks":[{"task":"kept","score":0.75}]}`), 0600)
		out := metricsRun(root, cwd, environment, "--json")
		Expect(out.status).To(BeZero(), "%+v", out)
		agents := metricsJSON(out.stdout)["agents"].(map[string]any)
		Expect(agents["harnesses"]).To(Equal(float64(2)))
		tasks := agents["tasks"].([]any)
		Expect(tasks).To(HaveLen(1))
		Expect(tasks[0].(map[string]any)["task"]).To(Equal("kept"))
	})
	DescribeTable("bounds metrics inputs", func(kind string) {
		root, cwd, environment := metricsFixture()
		switch kind {
		case "config":
			writeFixture(filepath.Join(cwd, "factory.yaml"), []byte(strings.Repeat("x", (16<<20)+1)), 0600)
		case "eval":
			writeFixture(filepath.Join(cwd, "eval/results/large-baseline.json"), []byte(strings.Repeat("x", (16<<20)+1)), 0600)
		case "aggregate":
			data := []byte(`{"tasks":[]}` + strings.Repeat(" ", 12<<20))
			for i := range 6 {
				writeFixture(filepath.Join(cwd, "eval/results/"+strconv.Itoa(i)+"-baseline.json"), data, 0600)
			}
		case "hooks":
			for i := range 4096 {
				writeFixture(filepath.Join(cwd, "scripts/hooks/entry-"+strconv.Itoa(i)), nil, 0600)
			}
		}
		out := metricsRun(root, cwd, environment, "--json")
		Expect(out.status).NotTo(BeZero(), "%+v", out)
		Expect(out.stderr).NotTo(BeEmpty())
	}, Entry("config file", "config"), Entry("eval file", "eval"), Entry("aggregate eval", "aggregate"), Entry("hook enumeration", "hooks"))
	It("reports a closed output pipe as failure", func() {
		root, cwd, environment := metricsFixture()
		read, write, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		Expect(read.Close()).To(Succeed())
		DeferCleanup(write.Close)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, filepath.Join(root, "factory"), "metrics", "--json") // #nosec G204 -- compiled fixture.
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
	DescribeTable("cancels an observed Git child before publication", func(signal syscall.Signal) {
		root, cwd, environment := metricsFixture()
		bin := filepath.Join(cwd, "blocked-git")
		marker := filepath.Join(cwd, "git-pid")
		writeFixture(filepath.Join(bin, "git"), []byte("#!/bin/bash\nprintf '%s' \"$$\" > \"$METRICS_PID_MARKER\"\nexec /bin/sleep 30\n"), 0700)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		command := exec.CommandContext(ctx, filepath.Join(root, "factory"), "metrics", "--html", "--no-open") // #nosec G204 -- compiled fixture.
		command.Dir, command.Env = cwd, append(environment, "PATH="+bin, "METRICS_PID_MARKER="+marker)
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
		_, err = os.Stat(filepath.Join(cwd, ".factory/metrics.html"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("SIGINT", syscall.SIGINT), Entry("SIGTERM", syscall.SIGTERM))
	It("keeps event data inert in the generated script element", func() {
		root, cwd, environment := metricsFixture()
		payload := "<!--<script></script><script>alert(1)</script>$(touch SHOULD_NOT_EXIST)"
		writeFixture(filepath.Join(cwd, ".factory/events.log"), []byte("9999-01-01T01:00:00Z\t"+payload+"\treason\n"), 0600)
		out := metricsRun(root, cwd, environment, "--html", "--no-open")
		Expect(out.status).To(BeZero(), "%+v", out)
		page, err := os.ReadFile(filepath.Join(cwd, ".factory/metrics.html"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(page)).NotTo(ContainSubstring("</script><script>"))
		Expect(string(page)).NotTo(ContainSubstring("<!--<script>"))
		Expect(string(page)).To(ContainSubstring("SHOULD_NOT_EXIST"))
		_, err = os.Stat(filepath.Join(cwd, "SHOULD_NOT_EXIST"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	DescribeTable("refuses an unsafe destination and preserves external content", func(kind string) {
		root, cwd, environment := metricsFixture()
		outside := filepath.Join(root, "outside")
		writeFixture(filepath.Join(outside, "metrics.html"), []byte("KEEP"), 0600)
		parent := filepath.Join(cwd, ".factory")
		path := filepath.Join(parent, "metrics.html")
		if kind == "parent link" {
			Expect(os.Symlink(outside, parent)).To(Succeed())
		} else {
			Expect(os.Mkdir(parent, 0700)).To(Succeed())
			switch kind {
			case "leaf link":
				Expect(os.Symlink(filepath.Join(outside, "metrics.html"), path)).To(Succeed())
			case "hardlink":
				Expect(os.Link(filepath.Join(outside, "metrics.html"), path)).To(Succeed())
			case "fifo":
				Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
			case "directory":
				Expect(os.Mkdir(path, 0700)).To(Succeed())
			}
		}
		beforeInfo, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		var before map[string]string
		if kind != "fifo" {
			before = nativeInitArtifacts(cwd)
		}
		out := metricsRun(root, cwd, environment, "--html", "--no-open")
		Expect(out.status).NotTo(BeZero(), "%+v", out)
		data, err := os.ReadFile(filepath.Join(outside, "metrics.html"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("KEEP"))
		afterInfo, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(beforeInfo, afterInfo)).To(BeTrue())
		if kind != "fifo" {
			Expect(nativeInitArtifacts(cwd)).To(Equal(before))
		}
	}, Entry("parent symlink", "parent link"), Entry("output symlink", "leaf link"), Entry("hardlink", "hardlink"), Entry("FIFO", "fifo"), Entry("directory", "directory"))
	It("preserves an existing page when the template placeholder is missing", func() {
		root, cwd, environment := metricsFixture()
		writeFixture(filepath.Join(root, "templates/metrics.html"), []byte("<html>no marker</html>"), 0600)
		path := filepath.Join(cwd, ".factory/metrics.html")
		writeFixture(path, []byte("KEEP"), 0640)
		before := nativeInitArtifacts(cwd)
		out := metricsRun(root, cwd, environment, "--html", "--no-open")
		Expect(out.status).NotTo(BeZero(), "%+v", out)
		Expect(nativeInitArtifacts(cwd)).To(Equal(before))
	})
	DescribeTable("publishes a complete page with the intended permissions and no cruft", func(existing bool) {
		root, cwd, environment := metricsFixture()
		path := filepath.Join(cwd, ".factory/metrics.html")
		mode := os.FileMode(0600)
		if existing {
			mode = 0640
			writeFixture(path, []byte("old page"), mode)
		}
		out := metricsRun(root, cwd, environment, "--html", "--no-open")
		Expect(out.status).To(BeZero(), "%+v", out)
		info, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(mode))
		entries, err := os.ReadDir(filepath.Dir(path))
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].Name()).To(Equal("metrics.html"))
	}, Entry("new", false), Entry("existing", true))
	DescribeTable("rejects unsafe event inputs without publishing", func(kind string) {
		root, cwd, environment := metricsFixture()
		path := filepath.Join(cwd, "unsafe-events")
		switch kind {
		case "fifo":
			Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
		case "large":
			writeFixture(path, []byte(strings.Repeat("x", (16<<20)+1)), 0600)
		case "binary":
			writeFixture(path, []byte("t\tg\t\x00\n"), 0600)
		}
		out := metricsRun(root, cwd, append(environment, "FACTORY_EVENT_LOG="+path), "--html", "--no-open")
		Expect(out.status).NotTo(BeZero(), "%+v", out)
		_, err := os.Stat(filepath.Join(cwd, ".factory/metrics.html"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("FIFO", "fifo"), Entry("large", "large"), Entry("binary", "binary"))
})
