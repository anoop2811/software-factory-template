package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type handoffBootstrapFixture struct {
	root, cwd, installer, records, marker, target string
	environment                                   map[string]string
	raw                                           []byte
}

// per docs/adr/0101-go-release-handoff.md:80
func handoffBootstrap(osName, architecture, target, scenario string) handoffBootstrapFixture {
	GinkgoHelper()
	_, cwd := fixture()
	root := filepath.Dir(cwd)
	records := filepath.Join(root, "records")
	Expect(os.Mkdir(records, 0700)).To(Succeed())
	assets := filepath.Join(root, "download inputs")
	marker := filepath.Join(records, "candidate-executed")
	raw := []byte("#!/bin/sh\nprintf called > \"$HANDOFF_EXECUTION_MARKER\"\nprintf '%s\\000' \"$@\" > \"$HANDOFF_RECORD_DIR/bootstrap-argv\"\nprintf '%s' \"${FACTORY_RELEASE_DIR-}\" > \"$HANDOFF_RECORD_DIR/acquisition-dir\"\nprintf 'bootstrap\\n' >> \"$HANDOFF_RECORD_DIR/trace\"\n[ -f \"$HANDOFF_RECORD_DIR/authenticated\" ] || exit 88\nexit 0\n")
	writeFixture(filepath.Join(assets, "factory-bootstrap-"+handoffVersion+"-"+target), raw, 0600)
	writeFixture(filepath.Join(assets, "factory-installation-"+handoffVersion+"-"+target+".tar.gz"), []byte("inert image handled by the authenticated Go owner"), 0600)
	writeFixture(filepath.Join(assets, "runtime-attestations.jsonl"), []byte("fixture attestations"), 0600)
	tools := filepath.Join(root, "host tools")
	writeFixture(filepath.Join(tools, "uname"), []byte("#!/bin/sh\ncase \"${1-}\" in -s) printf '%s\\n' \"$HANDOFF_OS\" ;; -m) printf '%s\\n' \"$HANDOFF_ARCH\" ;; *) exit 97 ;; esac\n"), 0700)
	for _, name := range []string{"git", "curl", "wget", "tar", "go", "python", "python3"} {
		writeFixture(filepath.Join(tools, name), []byte("#!/bin/sh\nprintf '%s\\n' \"$0\" >> \"$HANDOFF_RECORD_DIR/forbidden-tool\"\nexit 97\n"), 0700)
	}
	writeFixture(filepath.Join(tools, "gh"), []byte(`#!/bin/sh
set -eu
operation="$1 ${2-}"
case "$operation" in
  'release view') record=release-view ;;
  'release download') record=release-download ;;
  'attestation trusted-root') record=trusted-root ;;
  'attestation verify') record=verify ;;
  api\ *) record=api ;;
  *) printf unexpected-gh >&2; exit 97 ;;
esac
printf '%s\000' "$@" > "$HANDOFF_RECORD_DIR/$record"
printf '%s\n' "$record" >> "$HANDOFF_RECORD_DIR/trace"
case "$record" in
  api) printf '%s\n' "$HANDOFF_REVISION" ;;
  release-view) printf '%s\tfalse\n' "$3" ;;
  trusted-root)
    [ "$HANDOFF_SCENARIO" != roots-fail ] || exit 1
    printf 'independently obtained fixture roots\n' ;;
  release-download)
    [ "$HANDOFF_SCENARIO" != download-fail ] || exit 1
    selected_version="$3"
    destination=
    while [ "$#" -gt 0 ]; do
      case "$1" in --dir) destination="$2"; shift 2 ;; *) shift ;; esac
    done
    [ -n "$destination" ] || exit 97
    for asset in "factory-bootstrap-$selected_version-$HANDOFF_TARGET" "factory-installation-$selected_version-$HANDOFF_TARGET.tar.gz" runtime-attestations.jsonl; do
      if [ "$HANDOFF_SCENARIO" = missing-image ] && [ "$asset" = "factory-installation-$selected_version-$HANDOFF_TARGET.tar.gz" ]; then continue; fi
      /bin/cp "$HANDOFF_ASSETS/$asset" "$destination/$asset"
    done ;;
  verify)
    if [ -x "$3" ]; then printf early-chmod > "$HANDOFF_RECORD_DIR/early-chmod"; fi
    case "$HANDOFF_SCENARIO" in
      authentication-fail) exit 1 ;;
      wrong-subject) printf 'false\n'; exit 0 ;;
      malformed-verification) printf 'not-a-boolean\n'; exit 0 ;;
      wrong-archive-subject)
        query= previous=
        for value do
          if [ "$previous" = --jq ]; then query="$value"; break; fi
          previous="$value"
        done
        case "$query" in *factory-installation-*) printf 'false\n'; exit 0 ;; esac ;;
    esac
    printf accepted > "$HANDOFF_RECORD_DIR/authenticated"
    printf 'true\n' ;;
esac
`), 0700)
	installer := filepath.Join(root, "install.sh")
	data, err := os.ReadFile(filepath.Join("..", "install.sh"))
	Expect(err).NotTo(HaveOccurred())
	writeFixture(installer, data, 0600)
	environment := map[string]string{
		"PATH": tools + string(os.PathListSeparator) + "/usr/bin:/bin", "FACTORY_HOME": filepath.Join(root, "template cache"),
		"HANDOFF_RECORD_DIR": records, "HANDOFF_EXECUTION_MARKER": marker, "HANDOFF_ASSETS": assets,
		"HANDOFF_VERSION": handoffVersion, "HANDOFF_TARGET": target, "HANDOFF_REVISION": batchBase,
		"HANDOFF_OS": osName, "HANDOFF_ARCH": architecture, "HANDOFF_SCENARIO": scenario,
		"TMPDIR": root,
	}
	return handoffBootstrapFixture{root, cwd, installer, records, marker, target, environment, raw}
}

func handoffRecorded(f handoffBootstrapFixture, name string) []string {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join(f.records, name))
	Expect(err).NotTo(HaveOccurred())
	return strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
}

func handoffFlag(args []string, flag string) string {
	for index, value := range args {
		if value == flag && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}

func handoffCandidateOperands(args []string) (string, []string) {
	var tag string
	var remaining []string
	for index := 0; index < len(args); index++ {
		if args[index] == "--go-candidate" && index+1 < len(args) {
			tag = args[index+1]
			index++
			continue
		}
		if strings.HasPrefix(args[index], "--go-candidate=") {
			tag = strings.TrimPrefix(args[index], "--go-candidate=")
			continue
		}
		remaining = append(remaining, args[index])
	}
	return tag, remaining
}

// per docs/adr/0101-go-release-handoff.md:119
func handoffOwnedGroup(group int) []string {
	GinkgoHelper()
	processes := imageProcess(".", nil, "ps", "-axo", "pid=,pgid=,command=")
	Expect(processes.status).To(Equal(0), "%+v", processes)
	var owned []string
	for _, line := range strings.Split(processes.stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[1] != strconv.Itoa(group) {
			continue
		}
		owned = append(owned, line)
	}
	return owned
}

var _ = Describe("Authenticated candidate shell bootstrap", func() {
	// per docs/adr/0101-go-release-handoff.md:80
	// per docs/adr/0101-go-release-handoff.md:94
	DescribeTable("authenticates the exact host subject before the first executable call", func(osName, architecture, target string) {
		f := handoffBootstrap(osName, architecture, target, "")
		result := imageProcess(f.cwd, f.environment, "/bin/sh", f.installer, "init", "--go-candidate", handoffVersion, "--pack", "none")
		Expect(result.status).To(Equal(0), "%+v", result)
		tag, operands := handoffCandidateOperands(handoffRecorded(f, "bootstrap-argv"))
		Expect(tag).To(Equal(handoffVersion))
		Expect(operands).To(Or(Equal([]string{"init", "--pack", "none"}), Equal([]string{"init", f.cwd, "--pack", "none"})))
		verification := handoffRecorded(f, "verify")
		Expect(handoffFlag(verification, "--repo")).To(Equal("anoop2811/software-factory-template"))
		Expect(handoffFlag(verification, "--signer-workflow")).To(Equal("anoop2811/software-factory-template/.github/workflows/runtime-release.yml"))
		Expect(handoffFlag(verification, "--source-digest")).To(Equal(batchBase))
		Expect(handoffFlag(verification, "--source-ref")).To(Equal("refs/tags/" + handoffVersion))
		Expect(verification).To(ContainElement("--deny-self-hosted-runners"))
		query := handoffFlag(verification, "--jq")
		Expect(query).To(ContainSubstring("factory-bootstrap-" + handoffVersion + "-" + target))
		Expect(query).To(ContainSubstring(fmt.Sprintf("%x", sha256.Sum256(f.raw))))
		Expect(handoffFlag(verification, "--custom-trusted-root")).To(HaveSuffix("/trusted-root.jsonl"))
		trace, err := os.ReadFile(filepath.Join(f.records, "trace"))
		Expect(err).NotTo(HaveOccurred())
		verifiedAt, executedAt := strings.Index(string(trace), "verify\n"), strings.Index(string(trace), "bootstrap\n")
		Expect(verifiedAt).To(BeNumerically(">=", 0))
		Expect(executedAt).To(BeNumerically(">=", 0))
		Expect(verifiedAt).To(BeNumerically("<", executedAt))
		for _, name := range []string{"early-chmod", "forbidden-tool"} {
			_, err = os.Lstat(filepath.Join(f.records, name))
			Expect(os.IsNotExist(err)).To(BeTrue(), name)
		}
		acquired, err := os.ReadFile(filepath.Join(f.records, "acquisition-dir"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(acquired)).NotTo(BeEmpty())
		_, err = os.Lstat(string(acquired))
		Expect(os.IsNotExist(err)).To(BeTrue(), "the shell must clean its private handoff after the child exits")
	}, Entry("Linux amd64", "Linux", "x86_64", "linux-amd64"), Entry("Linux arm64", "Linux", "aarch64", "linux-arm64"), Entry("Darwin amd64", "Darwin", "x86_64", "darwin-amd64"), Entry("Darwin arm64", "Darwin", "arm64", "darwin-arm64"))

	// per docs/adr/0101-go-release-handoff.md:40
	It("forwards upgrade proposal arguments as literal argv including values with spaces", func() {
		f := handoffBootstrap("Linux", "x86_64", "linux-amd64", "")
		args := []string{"upgrade", "--go-candidate=" + handoffVersion, "--dry-run", "--json", "--project-input", "PROJECT_NAME=literal $(not-code) project"}
		result := imageProcess(f.cwd, f.environment, "/bin/sh", append([]string{f.installer}, args...)...)
		Expect(result.status).To(Equal(0), "%+v", result)
		tag, operands := handoffCandidateOperands(handoffRecorded(f, "bootstrap-argv"))
		Expect(tag).To(Equal(handoffVersion))
		Expect(operands).To(Equal([]string{"upgrade", "--dry-run", "--json", "--project-input", "PROJECT_NAME=literal $(not-code) project"}))
	})

	// per docs/adr/0101-go-release-handoff.md:455
	It("keeps piped installer source out of candidate stdin while preserving file-mode typed input", func() {
		for _, piped := range []bool{false, true} {
			f := handoffBootstrap("Linux", "x86_64", "linux-amd64", "")
			capture := "#!/bin/sh\n/bin/dd bs=256 count=1 > \"$HANDOFF_RECORD_DIR/bootstrap-stdin\" 2>/dev/null\n" + strings.TrimPrefix(string(f.raw), "#!/bin/sh\n")
			writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+handoffVersion+"-"+f.target), []byte(capture), 0600)
			input := "literal typed input\n"
			args := []string{f.installer, "init", "--go-candidate", handoffVersion}
			if piped {
				source, err := os.ReadFile(f.installer)
				Expect(err).NotTo(HaveOccurred())
				input = string(source)
				args = []string{"-s", "--", "init", "--go-candidate", handoffVersion}
			}
			// The exec helper supplies pipe stdin and captured, non-TTY output.
			result := handoffNativeProcess(f.cwd, f.environment, input, "/bin/sh", args...)
			Expect(result.status).To(Equal(0), "%+v", result)
			actual, err := os.ReadFile(filepath.Join(f.records, "bootstrap-stdin"))
			Expect(err).NotTo(HaveOccurred())
			if piped {
				Expect(actual).To(BeEmpty(), "verified code must not receive unread installer source as user answers")
			} else {
				Expect(string(actual)).To(Equal(input), "executing an installer file must preserve the caller's literal stdin")
			}
		}
	})

	// per docs/adr/0101-go-release-handoff.md:108
	DescribeTable("refuses acquisition and verification failures before chmod or execution", func(scenario string) {
		f := handoffBootstrap("Linux", "x86_64", "linux-amd64", scenario)
		before := nativeInitArtifacts(f.cwd)
		result := imageProcess(f.cwd, f.environment, "/bin/sh", f.installer, "init", "--go-candidate", handoffVersion)
		Expect(result.status).NotTo(Equal(0))
		trace, err := os.ReadFile(filepath.Join(f.records, "trace"))
		Expect(err).NotTo(HaveOccurred(), "the intended failure boundary must actually be reached")
		boundary := "verify\n"
		if scenario == "roots-fail" {
			boundary = "trusted-root\n"
		}
		if scenario == "download-fail" || scenario == "missing-image" {
			boundary = "release-download\n"
		}
		Expect(string(trace)).To(ContainSubstring(boundary))
		for _, path := range []string{f.marker, filepath.Join(f.records, "early-chmod"), filepath.Join(f.records, "forbidden-tool")} {
			_, err := os.Lstat(path)
			Expect(os.IsNotExist(err)).To(BeTrue(), path)
		}
		Expect(nativeInitArtifacts(f.cwd)).To(Equal(before))
	}, Entry("download", "download-fail"), Entry("roots", "roots-fail"), Entry("verification denial", "authentication-fail"), Entry("renamed other subject", "wrong-subject"), Entry("malformed verifier output", "malformed-verification"), Entry("incomplete release assets", "missing-image"))

	// per docs/adr/0101-go-release-handoff.md:40
	It("does not treat a selector alone or a conflicting source selection as installation consent", func() {
		for _, args := range [][]string{{"--go-candidate", handoffVersion}, {"init", "--go-candidate", handoffVersion, "--ref", "main"}} {
			f := handoffBootstrap("Linux", "x86_64", "linux-amd64", "")
			result := imageProcess(f.cwd, f.environment, "/bin/sh", append([]string{f.installer}, args...)...)
			Expect(result.status).To(Equal(2), "%+v", result)
			_, err := os.Lstat(f.marker)
			Expect(os.IsNotExist(err)).To(BeTrue())
			_, err = os.Lstat(filepath.Join(f.records, "trace"))
			Expect(os.IsNotExist(err)).To(BeTrue(), "invalid consent must precede acquisition")
		}
	})

	// per docs/adr/0101-go-release-handoff.md:119
	It("cancels and reaps a verifier that ignores TERM without hanging acquisition cleanup", func() {
		f := handoffBootstrap("Linux", "x86_64", "linux-amd64", "")
		writeFixture(filepath.Join(f.root, "host tools", "gh"), []byte("#!/bin/sh\ntrap '' TERM\nprintf '%s' \"$$\" > \"$HANDOFF_RECORD_DIR/resistant-pid\"\nprintf '%s' \"$PWD\" > \"$HANDOFF_RECORD_DIR/resistant-cwd\"\nwhile :; do /bin/sleep 1; done\n"), 0700)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/bin/sh", f.installer, "init", "--go-candidate", handoffVersion) // #nosec G204 -- fixed host shell and owned inert fixture paths.
		cmd.Dir = f.cwd
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Env = os.Environ()
		for key, value := range f.environment {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		Expect(cmd.Start()).To(Succeed())
		finished := make(chan struct{})
		var processError error
		go func() { processError = cmd.Wait(); close(finished) }()
		DeferCleanup(func() {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-finished
		})
		ready := filepath.Join(f.records, "resistant-pid")
		Eventually(func() bool { _, err := os.Stat(ready); return err == nil }).WithTimeout(3 * time.Second).Should(BeTrue())
		pidBytes, err := os.ReadFile(ready)
		Expect(err).NotTo(HaveOccurred())
		childPID, err := strconv.Atoi(string(pidBytes))
		Expect(err).NotTo(HaveOccurred())
		Expect(cmd.Process.Signal(syscall.SIGTERM)).To(Succeed())
		select {
		case <-finished:
		case <-time.After(8 * time.Second):
			Fail("installer did not finish bounded TERM-resistant verifier cleanup")
		}
		Expect(ctx.Err()).NotTo(HaveOccurred())
		Expect(processError).To(HaveOccurred(), "cancellation must not report success: %s", stdout.String())
		Expect(syscall.Kill(childPID, 0)).To(MatchError(syscall.ESRCH), "the verifier child must be reaped")
		Eventually(func() []string { return handoffOwnedGroup(cmd.Process.Pid) }).WithTimeout(time.Second).Should(BeEmpty(), "watchdogs and their sleep children belong to the same cleanup")
		matches, err := filepath.Glob(filepath.Join(f.root, "factory-release.*"))
		Expect(err).NotTo(HaveOccurred())
		Expect(matches).To(BeEmpty(), "only the owned acquisition directory is cleaned")
		_, err = os.Lstat(f.marker)
		Expect(os.IsNotExist(err)).To(BeTrue(), "cancellation must never execute a candidate")
	})

	// per docs/adr/0101-go-release-handoff.md:119
	// per docs/adr/0101-go-release-handoff.md:428
	DescribeTable("reaps verifier descendants and the watchdog after an ordinary acquisition failure", func(closedPipes bool) {
		f := handoffBootstrap("Linux", "x86_64", "linux-amd64", "")
		redirect := ""
		if closedPipes {
			redirect = " >/dev/null 2>&1"
		}
		writeFixture(filepath.Join(f.root, "host tools", "gh"), []byte("#!/bin/sh\n/bin/sleep 120"+redirect+" &\nprintf '%s' \"$!\" > \"$HANDOFF_RECORD_DIR/forked-verifier-pid\"\nps -o pgid= -p \"$$\" > \"$HANDOFF_RECORD_DIR/forked-verifier-group\"\n/bin/sleep 0.2\nexit 1\n"), 0700)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		outsider := exec.CommandContext(ctx, "/bin/sleep", "30")
		outsider.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		Expect(outsider.Start()).To(Succeed())
		DeferCleanup(func() { _ = syscall.Kill(-outsider.Process.Pid, syscall.SIGKILL); _ = outsider.Wait() })
		cmd := handoffCommand(ctx, f.cwd, f.environment, "", "/bin/sh", f.installer, "init", "--go-candidate", handoffVersion)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		Expect(cmd.Start()).To(Succeed())
		DeferCleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
		Expect(cmd.Wait()).To(HaveOccurred())
		Expect(ctx.Err()).NotTo(HaveOccurred())
		pidBytes, err := os.ReadFile(filepath.Join(f.records, "forked-verifier-pid"))
		Expect(err).NotTo(HaveOccurred())
		childPID, err := strconv.Atoi(string(pidBytes))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = syscall.Kill(childPID, syscall.SIGKILL) })
		groupBytes, err := os.ReadFile(filepath.Join(f.records, "forked-verifier-group"))
		Expect(err).NotTo(HaveOccurred())
		group, err := strconv.Atoi(strings.TrimSpace(string(groupBytes)))
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() error { return syscall.Kill(childPID, 0) }).WithTimeout(time.Second).Should(MatchError(syscall.ESRCH), "the actual gh descendant must not outlive acquisition failure")
		Eventually(func() []string { return handoffOwnedGroup(group) }).WithTimeout(time.Second).Should(BeEmpty())
		Eventually(func() []string { return handoffOwnedGroup(cmd.Process.Pid) }).WithTimeout(time.Second).Should(BeEmpty(), "failed acquisition must leave no owned watchdog sleep process")
		Expect(syscall.Kill(outsider.Process.Pid, 0)).To(Succeed(), "cleanup must preserve an unrelated process group")
	}, Entry("descendant retains output streams", false), Entry("descendant closes output streams", true))
})

var _ = Describe("Authenticated release candidate acquisition cancellation", func() {
	// per docs/adr/0101-go-release-handoff.md:119
	// per docs/adr/0101-go-release-handoff.md:140
	It("closes the independent archive verifier process group before discarding its private workspace", func() {
		f := handoffBootstrap(map[string]string{"linux": "Linux", "darwin": "Darwin"}[runtime.GOOS], map[string]string{"amd64": "x86_64", "arm64": "arm64"}[runtime.GOARCH], runtime.GOOS+"-"+runtime.GOARCH, "")
		ghPath := filepath.Join(f.root, "host tools", "gh")
		script, err := os.ReadFile(ghPath)
		Expect(err).NotTo(HaveOccurred())
		blocking := `  verify)
    if [ "${3##*/}" = archive ]; then
      /bin/sleep 120 &
      printf '%s' "$!" > "$HANDOFF_RECORD_DIR/archive-verifier-descendant"
      printf '%s' "${3%/*}" > "$HANDOFF_RECORD_DIR/archive-verifier-workspace"
      wait
    fi
`
		Expect(strings.Count(string(script), "  verify)\n")).To(Equal(1))
		writeFixture(ghPath, []byte(strings.Replace(string(script), "  verify)\n", blocking, 1)), 0700)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		driver := filepath.Join(f.root, "compiled caller")
		writeFixture(driver, candidateBinary, 0700)
		cmd := handoffCommand(ctx, f.cwd, f.environment, "", driver, "init", "--go-candidate", handoffVersion)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.WaitDelay = time.Second
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		Expect(cmd.Start()).To(Succeed())
		finished := make(chan struct{})
		var processError error
		go func() { processError = cmd.Wait(); close(finished) }()
		childPID := 0
		DeferCleanup(func() {
			if childPID > 0 {
				_ = syscall.Kill(childPID, syscall.SIGKILL)
			}
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-finished
		})
		ready := filepath.Join(f.records, "archive-verifier-descendant")
		Eventually(func() bool { _, err := os.Stat(ready); return err == nil }).WithTimeout(8*time.Second).Should(BeTrue(), "must reach Stage's independent second verifier, not stop at raw authentication")
		pidBytes, err := os.ReadFile(ready)
		Expect(err).NotTo(HaveOccurred())
		childPID, err = strconv.Atoi(string(pidBytes))
		Expect(err).NotTo(HaveOccurred())
		Expect(cmd.Process.Signal(syscall.SIGTERM)).To(Succeed())
		select {
		case <-finished:
		case <-time.After(8 * time.Second):
			Fail("candidate archive verifier did not finish bounded cancellation")
		}
		Expect(ctx.Err()).NotTo(HaveOccurred())
		Expect(processError).To(HaveOccurred())
		Eventually(func() error { return syscall.Kill(childPID, 0) }).WithTimeout(time.Second).Should(MatchError(syscall.ESRCH), "archive verifier descendant survived cancellation")
		workspace, err := os.ReadFile(filepath.Join(f.records, "archive-verifier-workspace"))
		Expect(err).NotTo(HaveOccurred())
		_, err = os.Lstat(string(workspace))
		Expect(os.IsNotExist(err)).To(BeTrue(), "confirmed-closed verifier custody may be cleaned")
	})
})
