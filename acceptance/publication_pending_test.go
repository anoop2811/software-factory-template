package acceptance_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/transition"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func publicationPendingFixture(checkout string) os.FileInfo {
	GinkgoHelper()
	state := filepath.Join(checkout, ".factory")
	Expect(os.Mkdir(state, 0755)).To(Succeed())
	pending := filepath.Join(state, "runtime-publication.pending")
	file, err := os.OpenFile(pending, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	Expect(err).NotTo(HaveOccurred())
	Expect(file.Sync()).To(Succeed())
	Expect(file.Close()).To(Succeed())
	directory, err := os.Open(state)
	Expect(err).NotTo(HaveOccurred())
	Expect(directory.Sync()).To(Succeed())
	Expect(directory.Close()).To(Succeed())
	info, err := os.Lstat(pending)
	Expect(err).NotTo(HaveOccurred())
	Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
	Expect(info.Size()).To(BeZero())
	return info
}

func publicationPendingPreserved(checkout string, before os.FileInfo) {
	GinkgoHelper()
	after, err := os.Lstat(filepath.Join(checkout, ".factory/runtime-publication.pending"))
	Expect(err).NotTo(HaveOccurred())
	Expect(os.SameFile(before, after)).To(BeTrue())
	Expect(after.Mode()).To(Equal(before.Mode()))
	Expect(after.Size()).To(Equal(before.Size()))
}

var _ = Describe("Live publication pending admission core", func() {
	// per docs/adr/0094-live-publication-restoration.md:59
	// per docs/adr/0094-live-publication-restoration.md:62
	DescribeTable("refuses actual cooperating execution before help or checks when durable pending evidence exists", func(kind string, legacy bool) {
		root, checkout, scripts := transitionFixture()
		before := publicationPendingFixture(checkout)
		out := transitionRun(root, checkout, scripts, kind, legacy, transitionRunArgs(kind))
		_, err := os.Lstat(filepath.Join(root, "child-started"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "pending evidence must block actual native help and checks: %+v", out)
		Expect(out.status).To(Equal(2), "%+v", out)
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).NotTo(BeEmpty())
		Expect(out.stderr).NotTo(ContainSubstring(checkout))
		publicationPendingPreserved(checkout, before)
		for _, name := range []string{"budget.json", "loops.json"} {
			_, err := os.Lstat(filepath.Join(checkout, ".factory", name))
			Expect(os.IsNotExist(err)).To(BeTrue(), "a restart barrier cannot admit budget or checkpoint work")
		}
	}, Entry("compiled Go budget", "budget", false), Entry("actual legacy budget", "budget", true), Entry("compiled Go manual loop", "loop", false), Entry("actual legacy manual loop", "loop", true))

	// per docs/adr/0094-live-publication-restoration.md:59
	// per docs/adr/0094-live-publication-restoration.md:57
	DescribeTable("refuses fresh exclusive acquisition even when the permanent lock is available", func(legacy bool) {
		_, checkout, scripts := transitionFixture()
		before := publicationPendingFixture(checkout)
		if legacy {
			program := `import json,sys;sys.path.insert(0,sys.argv[1]);import runtime_transition
try:
 guard=runtime_transition.exclusive(sys.argv[2]);guard.close(False);admitted=True
except runtime_transition.TransitionError:admitted=False
print(json.dumps(dict(admitted=admitted)))`
			out := loopProcess(checkout, transitionPython(checkout), []string{"-B", "-c", program, filepath.Join(scripts, "lib"), checkout}, nil)
			Expect(out.status).To(BeZero(), "%+v", out)
			Expect(out.stderr).To(BeEmpty())
			var report struct {
				Admitted bool `json:"admitted"`
			}
			Expect(json.Unmarshal([]byte(out.stdout), &report)).To(Succeed())
			Expect(report.Admitted).To(BeFalse(), "pending evidence remains authoritative after a process released its flock")
		} else {
			guard, err := transition.Exclusive(context.Background(), checkout)
			if guard != nil {
				DeferCleanup(func() { Expect(guard.Close(context.Background(), false)).To(Succeed()) })
			}
			Expect(err).To(HaveOccurred())
			Expect(guard).To(BeNil())
		}
		publicationPendingPreserved(checkout, before)
	}, Entry("actual Go exclusive component", false), Entry("actual Python exclusive component", true))
})

var _ = Describe("Live publication pending evidence boundaries", func() {
	// per docs/adr/0094-live-publication-restoration.md:60
	// per docs/adr/0094-live-publication-restoration.md:61
	DescribeTable("refuses every pending entry type without following, deleting or altering its identity", func(kind string, legacy bool) {
		root, checkout, scripts := transitionFixture()
		_ = publicationPendingFixture(checkout)
		pending := filepath.Join(checkout, ".factory/runtime-publication.pending")
		switch kind {
		case "nonempty":
			Expect(os.WriteFile(pending, []byte("PRIVATE_NOT_A_RECEIPT"), 0600)).To(Succeed())
		case "wrong mode":
			Expect(os.Chmod(pending, 0644)).To(Succeed())
		case "directory":
			Expect(os.Remove(pending)).To(Succeed())
			Expect(os.Mkdir(pending, 0700)).To(Succeed())
		case "link":
			Expect(os.Remove(pending)).To(Succeed())
			writeFixture(filepath.Join(root, "outside-evidence"), []byte("PRIVATE_OUTSIDE_EVIDENCE"), 0600)
			Expect(os.Symlink(filepath.Join(root, "outside-evidence"), pending)).To(Succeed())
		case "dangling link":
			Expect(os.Remove(pending)).To(Succeed())
			Expect(os.Symlink(filepath.Join(root, "absent"), pending)).To(Succeed())
		case "hard link":
			Expect(os.Link(pending, filepath.Join(root, "hard-evidence"))).To(Succeed())
		case "FIFO":
			Expect(os.Remove(pending)).To(Succeed())
			Expect(syscall.Mkfifo(pending, 0600)).To(Succeed())
		}
		before, err := os.Lstat(pending)
		Expect(err).NotTo(HaveOccurred())
		out := transitionRun(root, checkout, scripts, "loop", legacy, transitionRunArgs("loop"))
		_, err = os.Lstat(filepath.Join(root, "child-started"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "%+v", out)
		Expect(out.status).To(Equal(2), "%+v", out)
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).NotTo(ContainSubstring("PRIVATE_"))
		Expect(out.stderr).NotTo(ContainSubstring(checkout))
		after, err := os.Lstat(pending)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		Expect(after.Mode()).To(Equal(before.Mode()))
		Expect(after.Size()).To(Equal(before.Size()))
	}, Entry("Go nonempty", "nonempty", false), Entry("Python nonempty", "nonempty", true),
		Entry("Go wrong mode", "wrong mode", false), Entry("Python wrong mode", "wrong mode", true),
		Entry("Go directory", "directory", false), Entry("Python directory", "directory", true),
		Entry("Go link", "link", false), Entry("Python link", "link", true),
		Entry("Go dangling link", "dangling link", false), Entry("Python dangling link", "dangling link", true),
		Entry("Go hard link", "hard link", false), Entry("Python hard link", "hard link", true),
		Entry("Go FIFO", "FIFO", false), Entry("Python FIFO", "FIFO", true))

	// per docs/adr/0094-live-publication-restoration.md:64
	DescribeTable("keeps read-only and disabled entrypoints inert despite pending evidence", func(kind string, legacy, disabled bool) {
		root, checkout, scripts := transitionFixture()
		_ = publicationPendingFixture(checkout)
		args := []string{"report", "--json"}
		if kind == "loop" {
			args = transitionRunArgs(kind)
			args[0] = "plan"
		}
		if disabled {
			writeFixture(filepath.Join(checkout, "factory.yaml"), []byte("budget_enabled: false\nloop_enabled: false\ncheck_command: printf checked > \"$TRANSITION_CHILD_MARKER\"\n"), 0600)
			args = transitionRunArgs(kind)
			if kind == "loop" {
				args[len(args)-1] = "bounded"
				args = append(args, "--prompt-file", "prompt.txt")
			}
		}
		before := assessmentTree(checkout)
		out := transitionRun(root, checkout, scripts, kind, legacy, args)
		if disabled {
			Expect(out.status).To(Equal(2), "%+v", out)
		} else {
			Expect(out.status).To(BeZero(), "%+v", out)
		}
		_, err := os.Lstat(filepath.Join(root, "child-started"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(assessmentTree(checkout)).To(Equal(before))
		Expect(strings.Contains(out.stderr, "PRIVATE_")).To(BeFalse())
	}, Entry("Go budget report", "budget", false, false), Entry("Python budget report", "budget", true, false),
		Entry("Go loop plan", "loop", false, false), Entry("Python loop plan", "loop", true, false),
		Entry("Go disabled budget", "budget", false, true), Entry("Python disabled budget", "budget", true, true),
		Entry("Go disabled loop", "loop", false, true), Entry("Python disabled loop", "loop", true, true))
})

var _ = Describe("Live publication Python pending fault boundaries", func() {
	// per docs/adr/0094-live-publication-restoration.md:61
	// per docs/adr/0094-live-publication-restoration.md:62
	DescribeTable("uses real flock and filesystem inspection to refuse uncertain or late pending state", func(exclusive bool, phase string) {
		_, checkout, scripts := transitionFixture()
		mode := "shared"
		if exclusive {
			mode = "exclusive"
		}
		source := `import errno,json,os,sys
sys.path.insert(0,sys.argv[1])
import runtime_transition as rt
root,mode,phase=sys.argv[2:]
actual_stat,actual_flock,actual_fsync,actual_open,actual_close=os.stat,rt.fcntl.flock,os.fsync,os.open,os.close
project=actual_stat(root,follow_symlinks=False)
fds=set(); locked=False; reached=False; created=None
pending=os.path.join(root,'.factory','runtime-publication.pending')
def tracked_open(*args,**kwargs):
 fd=actual_open(*args,**kwargs); fds.add(fd); return fd
def tracked_close(fd):
 actual_close(fd); fds.discard(fd)
def create():
 global reached,created
 fd=tracked_open(pending,os.O_RDWR|os.O_CREAT|os.O_EXCL,0o600)
 actual_fsync(fd); created=os.fstat(fd); tracked_close(fd); reached=True
def flock(fd,operation):
 global locked
 actual_flock(fd,operation); locked=True
 if phase=='after flock' and not reached:create()
def stat(path,*args,**kwargs):
 global reached
 if os.fspath(path)=='runtime-publication.pending':
  if not locked:raise AssertionError('pending inspection before held flock')
  if phase=='inspection EIO':
   try:actual_stat(path,*args,**kwargs)
   except FileNotFoundError:pass
   else:raise AssertionError('negative control is not absent')
   reached=True; raise OSError(errno.EIO,'PRIVATE_PENDING_INSPECTION')
 return actual_stat(path,*args,**kwargs)
def fsync(fd):
 actual_fsync(fd)
 info=os.fstat(fd)
 if phase=='final sync' and not reached and (info.st_dev,info.st_ino)==(project.st_dev,project.st_ino):create()
os.open,os.close,os.stat,os.fsync,rt.fcntl.flock=tracked_open,tracked_close,stat,fsync,flock
admitted=False
try:
 guard=getattr(rt,mode)(root); admitted=True; guard.close(False)
except rt.TransitionError:pass
preserved=True
if created is not None:
 current=actual_stat(pending,follow_symlinks=False)
 preserved=(current.st_dev,current.st_ino,current.st_mode,current.st_size)==(created.st_dev,created.st_ino,created.st_mode,created.st_size)
print(json.dumps({'admitted':admitted,'locked':locked,'reached':reached,'closed':not fds,'preserved':preserved}))
`
		out := loopProcess(checkout, transitionPython(checkout), []string{"-B", "-c", source, filepath.Join(scripts, "lib"), checkout, mode, phase}, nil)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stderr).To(BeEmpty())
		var observed map[string]bool
		Expect(json.Unmarshal([]byte(out.stdout), &observed)).To(Succeed())
		Expect(observed).To(HaveKeyWithValue("locked", true))
		Expect(observed).To(HaveKeyWithValue("reached", true))
		Expect(observed).To(HaveKeyWithValue("closed", true))
		Expect(observed).To(HaveKeyWithValue("preserved", true))
		Expect(observed).To(HaveKeyWithValue("admitted", false))
	}, Entry("shared inspection EIO", false, "inspection EIO"), Entry("exclusive inspection EIO", true, "inspection EIO"),
		Entry("shared pending appears after flock", false, "after flock"), Entry("exclusive pending appears after flock", true, "after flock"),
		Entry("shared pending appears before return", false, "final sync"), Entry("exclusive pending appears before return", true, "final sync"))
})
