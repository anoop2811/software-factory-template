package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const transitionCompoundPython = `import argparse,json,os,sys
from pathlib import Path
sys.path.insert(0,sys.argv[1])
import budget,budget_adapters,loop,runtime_transition
root=Path(sys.argv[2]); compound=sys.argv[3]=='compound'
root.joinpath('.factory').mkdir(mode=0o700)
args=argparse.Namespace(harness='claude',session='s',task='t',mode='bounded',json=True,prompt_file=None)
record=dict(session='s',task='t',harness='claude',mode='bounded',status='active',outcome='running',
 evidence=[],budget_runs=[],attempts=0,no_progress=0,elapsed_seconds=0.,reserved_seconds=30.,
 policy='fixture',prompt='fixture',started_at='fixture',phase='starting',stop_reason='',next_action='',
 snapshot=dict(head='fixture',source='fixture',safety='fixture'),
 baseline=dict(head='fixture',source='fixture',safety='fixture'),owner_pid=os.getpid(),process_pid=None,uncertain=False)
history=dict(schema=1,runs=[record])
controller=loop.Controller(args,dict(timeout_seconds=30),budget.configuration(),root,loop.Store(root),history,record,'fixture prompt')
calls=[]; original_close=runtime_transition.Guard.close
def unconfirmed(*args):
 raise budget_adapters.UnconfirmedProcessError(424242,'fixture ownership remains unconfirmed')
def close(guard,clean=False):
 calls.append(clean)
 original_close(guard,clean)
 if compound: raise runtime_transition.TransitionError('cannot close runtime transition storage')
budget_adapters.preflight=unconfirmed
runtime_transition.Guard.close=close
try:
 controller.invoke('implementer','fixture prompt')
except BaseException as error:
 print(json.dumps(dict(typed=isinstance(error,budget_adapters.UnconfirmedProcessError),
  pid=getattr(error,'process_pid',None),message=str(error),cause=str(error.__cause__) if error.__cause__ else '',
  context=str(error.__context__) if error.__context__ else '',close_calls=calls,
  uncertain=controller.record['uncertain'],record_pid=controller.record['process_pid'],
  markers=[p.name for p in root.joinpath('.factory/runtime-activity').iterdir()],
  budget_history=root.joinpath('.factory/budget.json').exists(),checkpoint=root.joinpath('.factory/loops.json').exists())))
else: raise AssertionError('injected ownership failure did not reach actual controller')
`

const transitionGuardFaultPython = `import errno,fcntl,json,os,stat,sys
from pathlib import Path
sys.path.insert(0,sys.argv[1])
import runtime_transition as transition
root=Path(sys.argv[2]); fault=sys.argv[3]
opened=[]; labels={}; calls=[]; triggered=False; cleanup=False
original={name:getattr(os,name) for name in ('open','close','fsync','read','unlink')}
original_flock=fcntl.flock
def fail(label):
 global triggered
 triggered=True;calls.append(label);raise OSError(errno.EIO,'PRIVATE injected fault')
def opening(path,flags,mode=0o777,*,dir_fd=None):
 fd=original['open'](path,flags,mode,dir_fd=dir_fd)
 label='project' if dir_fd is None else ('marker' if labels.get(dir_fd)=='runtime-activity' else str(path))
 labels[fd]=label;opened.append(fd)
 assert flags & os.O_NOFOLLOW and flags & os.O_CLOEXEC and flags & os.O_NONBLOCK
 assert not flags & os.O_TRUNC
 if label=='marker': assert flags & os.O_CREAT and flags & os.O_EXCL and mode==0o600
 return fd
def syncing(fd):
 original['fsync'](fd)
 label=labels.get(fd)
 if not triggered and ((not cleanup and fault=='sync-'+str(label)) or (cleanup and fault=='post-unlink-sync' and label=='runtime-activity')):
  fail('fsync:'+label)
def reading(fd,size):
 data=original['read'](fd,size)
 if not triggered and fault=='marker-read' and labels.get(fd)=='marker':
  assert data==b'';fail('read:marker')
 return data
def closing(fd):
 original['close'](fd)
 if cleanup and not triggered and fault=='close-'+str(labels.get(fd)): fail('close:'+labels[fd])
def unlinking(path,*,dir_fd=None):
 if cleanup and not triggered and fault=='unlink-marker': fail('unlink:marker')
 return original['unlink'](path,dir_fd=dir_fd)
def flocking(fd,operation):
 original_flock(fd,operation)
 if not triggered and fault=='flock': fail('flock')
os.open=opening;os.fsync=syncing;os.read=reading;os.close=closing;os.unlink=unlinking;fcntl.flock=flocking
error='';returned=False
try:
 guard=transition.shared(root);returned=True;cleanup=True
 if fault=='replace-marker':
  old=root/'.factory/runtime-activity'/guard.marker[1]
  original_info=old.stat();old.rename(old.with_name('original-evidence'))
  fd=original['open'](old,os.O_CREAT|os.O_EXCL|os.O_WRONLY,0o600);original['close'](fd)
  assert old.stat().st_ino!=original_info.st_ino;triggered=True;calls.append('replace:marker')
 guard.close(fault not in ('normal-retain','close-project'))
except transition.TransitionError as exception: error=str(exception)
finally:
 for name,function in original.items():setattr(os,name,function)
 fcntl.flock=original_flock
closed=[]
for fd in opened:
 try:os.fstat(fd);closed.append(False)
 except OSError as exception:
  assert exception.errno==errno.EBADF;closed.append(True)
activity=root/'.factory/runtime-activity'
markers=[dict(name=p.name,mode=stat.S_IMODE(p.stat().st_mode),size=p.stat().st_size) for p in activity.iterdir()] if activity.exists() else []
lock=os.open(root/'.factory/runtime-transition.lock',os.O_RDWR)
try:fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
finally:os.close(lock)
exclusive=False
try:
 guard=transition.exclusive(root);exclusive=True;guard.close(False)
except transition.TransitionError: pass
print(json.dumps(dict(error=error,returned=returned,triggered=triggered,calls=calls,closed=closed,markers=markers,exclusive=exclusive)))
`

const transitionPublicationPython = `import argparse,errno,json,os,sys
from pathlib import Path
sys.path.insert(0,sys.argv[1])
import budget,budget_adapters,runtime_transition
root=Path(sys.argv[2]);fault=sys.argv[3]
args=argparse.Namespace(harness='claude',role='implementer',session='s',task='t',json=True,prompt_file=None,max_cost_usd=None)
ledger=budget.Ledger(root);calls=[];error='';error_type='';error_number=None;returned=False;status=None
original_write=ledger.write;original_close=runtime_transition.Guard.close
def write(data):
 original_write(data)
 if fault in ('terminal-write','terminal-write-close') and data['runs'] and data['runs'][0]['status']=='completed':
  calls.append('committed-terminal-write');raise OSError(errno.EIO,'terminal publication unavailable')
def close(guard,clean=False):
 calls.append('close-clean' if clean else 'close-retain')
 original_close(guard,clean)
 if fault in ('terminal-close','terminal-write-close'):raise runtime_transition.TransitionError('cannot close runtime transition storage')
def generic(*args):
 calls.append('generic-preflight');raise ValueError('unqualified preflight failure')
ledger.write=write;runtime_transition.Guard.close=close
if fault=='generic-preflight':budget_adapters.preflight=generic
try:
 status=budget.run(args,budget.configuration(),ledger,root,prompt_text='fixture prompt',quiet=True);returned=True
except (OSError,ValueError) as exception:
 error=str(exception);error_type=type(exception).__name__;error_number=getattr(exception,'errno',None)
data=ledger.read()
activity=root/'.factory/runtime-activity'
print(json.dumps(dict(error=error,error_type=error_type,errno=error_number,returned=returned,status=status,calls=calls,runs=data['runs'],
 markers=[p.name for p in activity.iterdir()])))
`

var _ = Describe("Runtime transition Python compound ownership", func() {
	// per docs/adr/0093-runtime-transition-guard.md:66
	// per docs/adr/0093-runtime-transition-guard.md:80
	// per docs/adr/0093-runtime-transition-guard.md:106
	// per docs/adr/0093-runtime-transition-guard.md:183
	DescribeTable("preserves unconfirmed preflight identity when closing the retained guard reports a second failure", func(compound bool) {
		_, checkout, scripts := transitionFixture()
		mode := "ordinary"
		if compound {
			mode = "compound"
		}
		out := loopProcess(checkout, transitionPython(checkout), []string{"-B", "-c", transitionCompoundPython, filepath.Join(scripts, "lib"), checkout, mode}, []string{"FACTORY_BUDGET_ENABLED=true"})
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stderr).To(BeEmpty())
		var report struct {
			Typed         bool     `json:"typed"`
			PID           int      `json:"pid"`
			Message       string   `json:"message"`
			Cause         string   `json:"cause"`
			Context       string   `json:"context"`
			CloseCalls    []bool   `json:"close_calls"`
			Uncertain     bool     `json:"uncertain"`
			RecordPID     int      `json:"record_pid"`
			Markers       []string `json:"markers"`
			BudgetHistory bool     `json:"budget_history"`
			Checkpoint    bool     `json:"checkpoint"`
		}
		Expect(json.Unmarshal([]byte(out.stdout), &report)).To(Succeed())
		Expect(report.CloseCalls).To(Equal([]bool{false}), "real close must retain uncertain ownership evidence: %s", out.stdout)
		Expect(report.Markers).To(HaveLen(1))
		Expect(report.BudgetHistory).To(BeFalse())
		Expect(report.Checkpoint).To(BeTrue(), "the actual loop collaborator published its preflight phase")
		Expect(report.Typed).To(BeTrue(), "cleanup must preserve the typed process cause: %s", out.stdout)
		Expect(report.Message).To(ContainSubstring("fixture ownership remains unconfirmed"))
		Expect(report.PID).To(Equal(424242))
		Expect(report.Uncertain).To(BeTrue())
		Expect(report.RecordPID).To(Equal(424242))
		if compound {
			Expect(report.Message + report.Cause + report.Context).To(ContainSubstring("cannot close runtime transition storage"))
		}
		Expect(report.Message + report.Cause + report.Context).NotTo(ContainSubstring(checkout))
	}, Entry("ordinary real retained close preserves the process error", false), Entry("real retained close followed by a cleanup failure preserves both causes", true))
})

var _ = Describe("Runtime transition Python guard durability", func() {
	// per docs/adr/0093-runtime-transition-guard.md:55
	// per docs/adr/0093-runtime-transition-guard.md:79
	// per docs/adr/0093-runtime-transition-guard.md:83
	// per docs/adr/0093-runtime-transition-guard.md:89
	// per docs/adr/0093-runtime-transition-guard.md:159
	DescribeTable("closes every real descriptor and preserves only qualified evidence at syscall failure boundaries", func(fault string, acquired bool, markers int, exclusive bool) {
		_, checkout, scripts := transitionFixture()
		out := loopProcess(checkout, transitionPython(checkout), []string{"-B", "-c", transitionGuardFaultPython, filepath.Join(scripts, "lib"), checkout, fault}, nil)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stderr).To(BeEmpty())
		var report struct {
			Error     string   `json:"error"`
			Returned  bool     `json:"returned"`
			Triggered bool     `json:"triggered"`
			Calls     []string `json:"calls"`
			Closed    []bool   `json:"closed"`
			Markers   []struct {
				Name string `json:"name"`
				Mode int    `json:"mode"`
				Size int    `json:"size"`
			} `json:"markers"`
			Exclusive bool `json:"exclusive"`
		}
		Expect(json.Unmarshal([]byte(out.stdout), &report)).To(Succeed())
		Expect(report.Returned).To(Equal(acquired), "%s", out.stdout)
		Expect(report.Closed).NotTo(BeEmpty())
		for _, closed := range report.Closed {
			Expect(closed).To(BeTrue(), "every actual open must be closed even after a fault: %s", out.stdout)
		}
		Expect(report.Markers).To(HaveLen(markers))
		for _, marker := range report.Markers {
			Expect(marker.Mode).To(Equal(0600))
			Expect(marker.Size).To(BeZero())
		}
		Expect(report.Exclusive).To(Equal(exclusive), "unlocked real inode alone cannot authorize evidence removal: %s", out.stdout)
		if fault == "normal-clean" || fault == "normal-retain" {
			Expect(report.Error).To(BeEmpty())
			Expect(report.Triggered).To(BeFalse())
		} else {
			Expect(report.Triggered).To(BeTrue(), "fault must execute the actual named syscall boundary: %s", out.stdout)
			Expect(report.Calls).To(HaveLen(1))
			Expect(report.Error).NotTo(BeEmpty())
			Expect(report.Error).NotTo(ContainSubstring("PRIVATE"))
			Expect(report.Error).NotTo(ContainSubstring(checkout))
		}
	},
		Entry("normal qualified cleanup", "normal-clean", true, 0, true),
		Entry("explicit uncertainty retains evidence", "normal-retain", true, 1, false),
		Entry("marker readback fails after a real empty read", "marker-read", false, 1, false),
		Entry("marker sync reports failure after actual fsync", "sync-marker", false, 1, false),
		Entry("permanent lock sync reports failure after actual fsync", "sync-runtime-transition.lock", false, 1, false),
		Entry("activity sync reports failure after actual fsync", "sync-runtime-activity", false, 1, false),
		Entry("state sync reports failure after actual fsync", "sync-.factory", false, 1, false),
		Entry("project sync reports failure after actual fsync", "sync-project", false, 1, false),
		Entry("flock reports failure after actual shared acquisition", "flock", false, 0, true),
		Entry("marker close reports failure after actual closure", "close-marker", true, 1, false),
		Entry("unlink fails before deletion", "unlink-marker", true, 1, false),
		Entry("post-qualified-unlink sync failure does not recreate evidence", "post-unlink-sync", true, 0, true),
		Entry("project close reports failure while evidence is retained", "close-project", true, 1, false),
		Entry("real foreign marker replacement preserves both inodes", "replace-marker", true, 2, false),
	)
})

var _ = Describe("Runtime transition Python controller publication", func() {
	// per docs/adr/0093-runtime-transition-guard.md:65
	// per docs/adr/0093-runtime-transition-guard.md:67
	// per docs/adr/0093-runtime-transition-guard.md:80
	// per docs/adr/0093-runtime-transition-guard.md:115
	// per docs/adr/0093-runtime-transition-guard.md:188
	DescribeTable("requires durable completion and reports cleanup errors after ordinary success", func(fault string, markers int) {
		root, checkout, scripts := transitionFixture()
		transitionNative(root, checkout)
		out := loopProcess(checkout, transitionPython(checkout), []string{"-B", "-c", transitionPublicationPython, filepath.Join(scripts, "lib"), checkout, fault}, []string{"FACTORY_BUDGET_ENABLED=true", "PATH=" + filepath.Join(root, "bin") + string(filepath.ListSeparator) + os.Getenv("PATH")})
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stderr).To(BeEmpty())
		var report struct {
			Error     string   `json:"error"`
			ErrorType string   `json:"error_type"`
			Errno     *int     `json:"errno"`
			Returned  bool     `json:"returned"`
			Status    *int     `json:"status"`
			Calls     []string `json:"calls"`
			Runs      []struct {
				Status     string `json:"status"`
				Outcome    string `json:"outcome"`
				ProcessPID int    `json:"process_pid"`
				Complete   bool   `json:"complete"`
			} `json:"runs"`
			Markers []string `json:"markers"`
		}
		Expect(json.Unmarshal([]byte(out.stdout), &report)).To(Succeed())
		Expect(report.Markers).To(HaveLen(markers))
		if fault == "generic-preflight" {
			Expect(report.Runs).To(BeEmpty())
			Expect(report.Calls).To(Equal([]string{"generic-preflight", "close-retain"}))
		} else {
			Expect(report.Runs).To(HaveLen(1), "real native invocation must publish an actual terminal record: %s", out.stdout)
			Expect(report.Runs[0].Status).To(Equal("completed"))
			Expect(report.Runs[0].Outcome).To(Equal("completed"))
			Expect(report.Runs[0].Complete).To(BeTrue())
			Expect(report.Runs[0].ProcessPID).To(BeNumerically(">", 0))
			Expect(transitionAlive(checkout, report.Runs[0].ProcessPID)).To(BeFalse())
		}
		switch fault {
		case "normal":
			Expect(report.Error).To(BeEmpty())
			Expect(report.Returned).To(BeTrue())
			Expect(report.Status).NotTo(BeNil())
			Expect(*report.Status).To(BeZero())
			Expect(report.Calls).To(Equal([]string{"close-clean"}))
		case "terminal-write", "terminal-write-close":
			Expect(report.Calls).To(Equal([]string{"committed-terminal-write", "close-retain"}))
			Expect(report.Error).To(ContainSubstring("terminal publication unavailable"))
			Expect(report.ErrorType).To(Equal("OSError"))
			Expect(report.Errno).NotTo(BeNil())
			Expect(*report.Errno).To(Equal(5), "primary I/O errno must remain available to its caller")
			if fault == "terminal-write-close" {
				Expect(report.Error).To(ContainSubstring("cannot close runtime transition storage"), "the controller's direct diagnostic must report both I/O and cleanup failures: %s", out.stdout)
			}
		case "terminal-close":
			Expect(report.Calls).To(Equal([]string{"close-clean"}))
			Expect(report.Error).To(ContainSubstring("cannot close runtime transition storage"))
		}
		if fault != "normal" {
			Expect(report.Returned).To(BeFalse(), "a reported terminal/cleanup failure cannot become a successful budget return: %s", out.stdout)
			Expect(report.Status).To(BeNil())
			Expect(report.Error).NotTo(BeEmpty())
		}
		Expect(report.Error).NotTo(ContainSubstring(checkout))
	}, Entry("ordinary durable terminal success", "normal", 0), Entry("terminal write committed before reporting failure", "terminal-write", 1), Entry("committed terminal I/O failure followed by real retained cleanup and a second failure", "terminal-write-close", 1), Entry("ordinary success followed by real guard cleanup and reported close failure", "terminal-close", 0), Entry("injected generic preflight error is not harmless proof", "generic-preflight", 1))
})
