package native_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

// Build a test-only main over the real exported inherited-stream API. An overlay
// keeps fixture source outside the repository and changes no production file.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:157
const inheritedProbeMain = `package main
import("context";"fmt";"os";"strings";"github.com/anoop2811/software-factory-template/internal/native")
func main(){
 environment:=map[string]string{}
 for _,entry:=range os.Environ(){key,value,_:=strings.Cut(entry,"=");environment[key]=value}
 root,err:=os.Getwd();if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
 result,err:=native.ExecuteInherited(context.Background(),root,os.Args[1:],environment,native.InheritedStreams{Stdin:os.Stdin,Stdout:os.Stdout,Stderr:os.Stderr},0)
 if err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
 if result.ExitCode==nil{os.Exit(1)}
 status:=*result.ExitCode;if status<0{status=128-status};os.Exit(status)
}
`

// Every transition is witnessed by terminal output and foreground ownership.
// Select deadlines bound missing events; elapsed time is never success evidence.
// Cleanup kills only this fixture's newly created shell, supervisor and job groups.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:282
const inheritedTerminalFixture = `import errno,fcntl,json,os,pty,re,select,shlex,signal,struct,sys,termios,time
prompt=b'INHERITED_FIXTURE_PROMPT> '
def terminate(number,frame):raise SystemExit(128+number)
signal.signal(signal.SIGTERM,terminate)
def remember(shell,group):
 with open(sys.argv[2],'a') as record:record.write(json.dumps([shell,group])+'\n')
def run(probe):
 shell,fd=pty.fork()
 if shell==0:
  os.environ['PS1']=prompt.decode();os.environ['BASH_SILENCE_DEPRECATION_WARNING']='1'
  os.execv('/bin/bash',['/bin/bash','--noprofile','--norc','-i'])
 remember(shell,shell)
 child=None;owner=None;transcript=bytearray()
 def until(predicate,seconds=3):
  start=len(transcript);end=time.monotonic()+seconds
  while True:
   output=bytes(transcript[start:])
   if predicate(output):return output,True
   remaining=end-time.monotonic()
   if remaining<=0:return output,False
   ready,_,_=select.select([fd],[],[],min(.05,remaining))
   if ready:
    try:part=os.read(fd,65536)
    except OSError as error:
     if error.errno!=errno.EIO:raise
     return output,False
    if not part:return output,False
    transcript.extend(part)
 record={}
 try:
  fcntl.ioctl(fd,termios.TIOCSWINSZ,struct.pack('HHHH',24,500,0,0))
  _,ready=until(lambda output:prompt in output)
  if not ready:raise RuntimeError('interactive shell did not establish its prompt')
  program="echo CHILD_PID=$$ OWNER_PID=$PPID; IFS= read -r value; printf 'READ=%s\\n' \"$value\""
  command=('/bin/bash -c '+shlex.quote(program))
  if probe:command=shlex.quote(probe)+' '+command
  os.write(fd,(command+'\n').encode())
  output,ready=until(lambda output:re.search(rb'CHILD_PID=(\d+) OWNER_PID=(\d+)',output) is not None)
  if not ready:raise RuntimeError('real child readiness not observed: '+output.decode(errors='replace'))
  child,owner=map(int,re.search(rb'CHILD_PID=(\d+) OWNER_PID=(\d+)',output).groups())
  remember(shell,child);remember(shell,owner)
  record['child_foreground']=os.tcgetpgrp(fd)==child
  os.write(fd,b'\x1a')
  output,stopped=until(lambda output:prompt in output and os.tcgetpgrp(fd)==shell)
  record['stop_returns_prompt']=stopped
  record['stop_returns_foreground']=os.tcgetpgrp(fd)==shell
  record['resume_consumes_input']=False;record['exit_returns_prompt']=False
  if stopped:
   os.write(fd,b'fg\n')
   _,resumed=until(lambda output:os.tcgetpgrp(fd)==child)
   if resumed:
    os.write(fd,b'literal resumed input\n')
    output,finished=until(lambda output:b'READ=literal resumed input\r\n' in output and prompt in output and os.tcgetpgrp(fd)==shell)
    record['resume_consumes_input']=b'READ=literal resumed input\r\n' in output
    record['exit_returns_prompt']=finished
 finally:
  try:foreground=os.tcgetpgrp(fd)
  except OSError:foreground=None
  for group in set(value for value in (child,owner,shell,foreground) if value and value!=os.getpgrp()):
   try:
    if os.getsid(group)==shell:os.killpg(group,signal.SIGKILL)
   except ProcessLookupError:pass
  os.waitpid(shell,0);os.close(fd)
 record['transcript']=transcript.decode(errors='replace')
 return record
print(json.dumps({'direct':run(None),'inherited':run(sys.argv[1])}))
`

var _ = Describe("Native inherited terminal job control", func() {
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:157
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:282
	It("returns a real shell prompt on Ctrl-Z and resumes stdin through fg beside direct Bash and non-TTY controls", func() {
		temporary := GinkgoT().TempDir()
		root, err := filepath.EvalSymlinks(temporary)
		Expect(err).NotTo(HaveOccurred())
		streams := native.InheritedStreams{}
		stdin, input, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		streams.Stdin = stdin
		streams.Stdout, err = os.CreateTemp(root, "stdout-")
		Expect(err).NotTo(HaveOccurred())
		streams.Stderr, err = os.CreateTemp(root, "stderr-")
		Expect(err).NotTo(HaveOccurred())
		for _, file := range []*os.File{streams.Stdin, streams.Stdout, streams.Stderr} {
			DeferCleanup(file.Close)
		}
		_, err = input.WriteString("literal nonterminal input\n")
		Expect(err).NotTo(HaveOccurred())
		Expect(input.Close()).To(Succeed())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result, err := native.ExecuteInherited(ctx, root, []string{"/bin/bash", "-c", `IFS= read -r value; printf '%s\n%s\n%s\n' "$value" "$1" "$PWD"; printf '%s\n' "$LITERAL_ENV" >&2; exit 7`, "fixture", "literal $(not shell syntax)"}, map[string]string{"LITERAL_ENV": "literal inherited environment"}, streams, 5*time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ExitConfirmed).To(BeTrue())
		Expect(result.ExitCode).NotTo(BeNil())
		Expect(*result.ExitCode).To(Equal(7))
		Expect(result.OwnershipUnconfirmed).To(BeFalse())
		stdout, err := os.ReadFile(streams.Stdout.Name())
		Expect(err).NotTo(HaveOccurred())
		Expect(string(stdout)).To(Equal("literal nonterminal input\nliteral $(not shell syntax)\n" + root + "\n"))
		stderr, err := os.ReadFile(streams.Stderr.Name())
		Expect(err).NotTo(HaveOccurred())
		Expect(string(stderr)).To(Equal("literal inherited environment\n"))

		repository, err := filepath.Abs("../..")
		Expect(err).NotTo(HaveOccurred())
		main := filepath.Join(root, "probe.go")
		Expect(os.WriteFile(main, []byte(inheritedProbeMain), 0600)).To(Succeed())
		overlay, err := json.Marshal(map[string]any{"Replace": map[string]string{filepath.Join(repository, "cmd/factory/main.go"): main}})
		Expect(err).NotTo(HaveOccurred())
		overlayPath := filepath.Join(root, "overlay.json")
		Expect(os.WriteFile(overlayPath, overlay, 0600)).To(Succeed())
		probe := filepath.Join(root, "inherited-probe")
		buildCtx, buildCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer buildCancel()
		buildArgs := []string{"build", "-overlay", overlayPath, "-o", probe, "./cmd/factory"}
		if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
			buildArgs = append([]string{"build", "-race"}, buildArgs[1:]...)
		}
		build := exec.CommandContext(buildCtx, "go", buildArgs...) // #nosec G204 -- fixed package and evaluator-owned overlay; no production source edit.
		build.Dir = repository
		output, err := build.CombinedOutput()
		Expect(buildCtx.Err()).NotTo(HaveOccurred(), "inherited probe build exceeded its bound: %s", output)
		Expect(err).NotTo(HaveOccurred(), "inherited probe build failed: %s", output)
		terminalCtx, terminalCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer terminalCancel()
		groupsPath := filepath.Join(root, "owned-terminal-groups.jsonl")
		command := exec.CommandContext(terminalCtx, "python3", "-B", "-c", inheritedTerminalFixture, probe, groupsPath) // #nosec G204 -- fixed test-only real PTY driver and fixture-owned probe.
		command.Dir = root
		// Give the driver's finally blocks an opportunity to reap their sessions.
		// A test-owned record independently bounds cleanup if the driver is killed.
		command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
		command.WaitDelay = 2 * time.Second
		complete := false
		DeferCleanup(func() {
			if complete {
				return
			}
			data, err := os.ReadFile(groupsPath)
			if os.IsNotExist(err) {
				return
			}
			Expect(err).NotTo(HaveOccurred())
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				var owned [2]int
				Expect(json.Unmarshal([]byte(line), &owned)).To(Succeed())
				Expect(owned[0]).To(BeNumerically(">", 1))
				Expect(owned[1]).To(BeNumerically(">", 1))
				if session, err := unix.Getsid(owned[1]); err == nil && session == owned[0] {
					_ = syscall.Kill(-owned[1], syscall.SIGKILL)
				}
			}
		})
		output, err = command.CombinedOutput()
		complete = err == nil && terminalCtx.Err() == nil
		Expect(terminalCtx.Err()).NotTo(HaveOccurred(), "PTY fixture exceeded its explicit bounds: %s", output)
		Expect(err).NotTo(HaveOccurred(), "PTY fixture did not complete its owned cleanup: %s", output)
		var observations map[string]struct {
			ChildForeground       bool   `json:"child_foreground"`
			StopReturnsPrompt     bool   `json:"stop_returns_prompt"`
			StopReturnsForeground bool   `json:"stop_returns_foreground"`
			ResumeConsumesInput   bool   `json:"resume_consumes_input"`
			ExitReturnsPrompt     bool   `json:"exit_returns_prompt"`
			Transcript            string `json:"transcript"`
		}
		Expect(json.Unmarshal(output, &observations)).To(Succeed(), "%s", output)
		_, _ = fmt.Fprintf(GinkgoWriter, "real PTY observations: %s\n", strings.TrimSpace(string(output)))
		Expect(observations).To(HaveKey("direct"))
		Expect(observations).To(HaveKey("inherited"))
		for _, label := range []string{"direct", "inherited"} {
			observed := observations[label]
			Expect(observed.ChildForeground).To(BeTrue(), "%s child never owned the real terminal: %s", label, observed.Transcript)
			Expect(observed.StopReturnsPrompt).To(BeTrue(), "%s Ctrl-Z left the shell waiting without its prompt: %s", label, observed.Transcript)
			Expect(observed.StopReturnsForeground).To(BeTrue(), "%s Ctrl-Z failed to restore terminal ownership: %s", label, observed.Transcript)
			Expect(observed.ResumeConsumesInput).To(BeTrue(), "%s fg failed to resume the same child's real stdin: %s", label, observed.Transcript)
			Expect(observed.ExitReturnsPrompt).To(BeTrue(), "%s normal resumed exit did not return terminal ownership: %s", label, observed.Transcript)
		}
	})
})
