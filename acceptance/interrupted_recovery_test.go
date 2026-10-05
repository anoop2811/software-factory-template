package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/assessment"
	"github.com/anoop2811/software-factory-template/internal/transition"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

// Evaluator-only compiled client; it links the actual exported source API.
// per docs/adr/0096-interrupted-publication-recovery.md:240
// per docs/adr/0096-interrupted-publication-recovery.md:243
const interruptedRecoverySource = `package main
import("context";"encoding/json";"os";"strings";"github.com/anoop2811/software-factory-template/internal/assessment")
type reply struct{Method string ` + "`json:\"method\"`" + `;Status int ` + "`json:\"status\"`" + `;Error string ` + "`json:\"error\"`" + `;Proposal any ` + "`json:\"proposal,omitempty\"`" + `}
type action struct{Method string;Confirmation string}
func main(){ctx:=context.Background();root:=os.Args[1];dec:=json.NewDecoder(os.Stdin);enc:=json.NewEncoder(os.Stdout);var request assessment.InterruptedRecoveryRequest;if dec.Decode(&request)!=nil{os.Exit(2)};environment:=map[string]string{};for _,entry:=range os.Environ(){key,value,ok:=strings.Cut(entry,"=");if ok{environment[key]=value}};emit:=func(method string,err error){message:="";status:=0;if err!=nil{message=err.Error();status=assessment.ErrorStatus(err)};_ = enc.Encode(reply{Method:method,Status:status,Error:message})};var operation *assessment.InterruptedRecovery;defer func(){if operation!=nil{_ = operation.Close(ctx)}}();for{var command action;if dec.Decode(&command)!=nil{return};switch command.Method{case "propose":proposal,err:=assessment.ProposeInterruptedRecovery(ctx,root,request);if err!=nil{emit("propose",err)}else{_ = enc.Encode(reply{Method:"propose",Proposal:proposal})};case "begin":var err error;operation,err=assessment.BeginInterruptedRecovery(ctx,root,request,command.Confirmation,environment);emit("begin",err);case "complete":emit("complete",operation.Complete(ctx));case "close":emit("close",operation.Close(ctx));return;default:os.Exit(2)}}}
`

var interruptedRecoveryBinary []byte

func interruptedRecoveryBuild() []byte {
	GinkgoHelper()
	if interruptedRecoveryBinary != nil {
		return interruptedRecoveryBinary
	}
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	dir := GinkgoT().TempDir()
	module, err := os.ReadFile(filepath.Join(repository, "go.mod"))
	Expect(err).NotTo(HaveOccurred())
	parts := strings.SplitN(string(module), "\ngo ", 2)
	Expect(parts).To(HaveLen(2))
	version := strings.Fields(parts[1])[0]
	writeFixture(filepath.Join(dir, "go.mod"), []byte("module github.com/anoop2811/software-factory-template/acceptance/interruptedrecoveryfixture\n\ngo "+version+"\nrequire github.com/anoop2811/software-factory-template v0.0.0\nreplace github.com/anoop2811/software-factory-template => "+repository+"\n"), 0600)
	writeFixture(filepath.Join(dir, "main.go"), []byte(interruptedRecoverySource), 0600)
	args := []string{"build", "-mod=mod", "-o", filepath.Join(dir, "interrupted-recovery"), "."}
	if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
		args = append([]string{"build", "-race"}, args[1:]...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", args...) // #nosec G204 -- fixed evaluator-only source module and build arguments.
	command.Dir, command.Env = dir, append(os.Environ(), "GOPROXY=off")
	output, err := command.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "actual interrupted-recovery API client compilation: %s", output)
	interruptedRecoveryBinary, err = os.ReadFile(filepath.Join(dir, "interrupted-recovery"))
	Expect(err).NotTo(HaveOccurred())
	return interruptedRecoveryBinary
}

type interruptedRecoveryClient struct {
	command *exec.Cmd
	input   io.WriteCloser
	encode  *json.Encoder
	decode  *json.Decoder
	stderr  bytes.Buffer
	waited  bool
	cancel  context.CancelFunc
}

func interruptedRecoveryStart(binaryRoot, root string, request map[string]any) *interruptedRecoveryClient {
	GinkgoHelper()
	program := filepath.Join(binaryRoot, "interrupted-recovery-client")
	writeFixture(program, interruptedRecoveryBuild(), 0700)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	client := &interruptedRecoveryClient{cancel: cancel}
	client.command = exec.CommandContext(ctx, program, root) // #nosec G204 -- test-owned compiled client and physical fixture root.
	client.command.Env = append(durableGitEnvironment(), "PATH="+filepath.Join(binaryRoot, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"), "DURABLE_GIT_MARKER="+filepath.Join(binaryRoot, "durable-git-calls"))
	client.command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	client.command.Stderr = &client.stderr
	input, err := client.command.StdinPipe()
	Expect(err).NotTo(HaveOccurred())
	output, err := client.command.StdoutPipe()
	Expect(err).NotTo(HaveOccurred())
	client.input, client.encode, client.decode = input, json.NewEncoder(input), json.NewDecoder(output)
	client.decode.UseNumber()
	Expect(client.command.Start()).To(Succeed())
	DeferCleanup(func() {
		if !client.waited {
			_ = unix.Kill(-client.command.Process.Pid, unix.SIGKILL)
			_ = client.command.Wait()
		}
		cancel()
	})
	Expect(client.encode.Encode(request)).To(Succeed())
	return client
}

func (client *interruptedRecoveryClient) call(method, confirmation string) map[string]any {
	GinkgoHelper()
	Expect(client.encode.Encode(map[string]string{"Method": method, "Confirmation": confirmation})).To(Succeed())
	var object map[string]any
	Expect(client.decode.Decode(&object)).To(Succeed(), "actual client output for %s", method)
	Expect(object).To(HaveKeyWithValue("method", method))
	return object
}
func (client *interruptedRecoveryClient) wait() {
	GinkgoHelper()
	Expect(client.input.Close()).To(Succeed())
	Expect(client.command.Wait()).To(Succeed(), "%s", client.stderr.String())
	client.waited = true
	client.cancel()
	Expect(client.stderr.String()).To(BeEmpty())
}
func interruptedRecoveryKill(client *durablePublicationClient) {
	GinkgoHelper()
	Expect(client.command.Process.Kill()).To(Succeed())
	waitErr := client.command.Wait()
	client.waited = true
	client.cancel()
	var exited *exec.ExitError
	Expect(errors.As(waitErr, &exited)).To(BeTrue())
	Expect(exited.ProcessState.Sys().(syscall.WaitStatus).Signal()).To(Equal(syscall.SIGKILL))
	Expect(client.stderr.String()).To(BeEmpty())
}
func interruptedRecoveryRequest(root string, publication assessment.PublicationRequest, direction string) map[string]any {
	GinkgoHelper()
	data := durableBytes(root, ".factory/backups/.publications/"+publication.MigrationID+".json")
	var record map[string]any
	Expect(json.Unmarshal(data, &record)).To(Succeed())
	digest := sha256.Sum256(publication.Replacement)
	return map[string]any{"MigrationID": publication.MigrationID, "OperationID": record["operation_id"], "Path": publication.Path, "Direction": direction, "Replacement": bytes.Clone(publication.Replacement), "AfterReference": assessment.Observation{SHA256: hex.EncodeToString(digest[:]), Mode: "0755", Bytes: int64(len(publication.Replacement))}, "UnbridgedQuiescent": true}
}
func interruptedRecoveryProposal(object map[string]any, root, direction, image string) string {
	GinkgoHelper()
	durablePublicationSuccess(object)
	proposal, ok := object["proposal"].(map[string]any)
	Expect(ok).To(BeTrue(), "%+v", object)
	Expect(proposal).To(HaveLen(23))
	Expect(proposal).To(HaveKeyWithValue("schema_version", json.Number("1")))
	Expect(proposal).To(HaveKeyWithValue("mode", "interrupted_recovery_proposal"))
	Expect(proposal).To(HaveKeyWithValue("scope", "g2-budget-loop-single-publication"))
	Expect(proposal).To(HaveKeyWithValue("purpose", "fresh_current_image_resolution"))
	Expect(proposal).To(HaveKeyWithValue("direction", direction))
	Expect(proposal).To(HaveKeyWithValue("current_image", image))
	Expect(proposal).To(HaveKeyWithValue("state_compatibility", "g2-state-v1_checked"))
	Expect(proposal).To(HaveKeyWithValue("unbridged_quiescence", "operator_affirmed"))
	for _, field := range []string{"restorable", "rollback_ready", "activation_ready", "applicable", "prune_authorized"} {
		Expect(proposal).To(HaveKeyWithValue(field, false))
	}
	encoded, err := json.Marshal(proposal)
	Expect(err).NotTo(HaveOccurred())
	Expect(string(encoded)).NotTo(ContainSubstring(root))
	Expect(string(encoded)).NotTo(ContainSubstring("LIVE_PUBLICATION_REPLACEMENT"))
	digest, ok := proposal["proposal_digest"].(string)
	Expect(ok).To(BeTrue())
	Expect(digest).To(MatchRegexp("^[0-9a-f]{64}$"))
	return digest
}

func interruptedRecoveryPythonAdmission(root string, admitted bool) {
	GinkgoHelper()
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	source := `import json,sys
from pathlib import Path
sys.path.insert(0,sys.argv[1])
import runtime_transition as transition
results=[]
for acquire in (transition.shared,transition.exclusive):
 try:
  guard=acquire(Path(sys.argv[2]))
 except transition.TransitionError:
  results.append(False)
 else:
  guard.close(True)
  results.append(True)
print(json.dumps(results))
`
	out := loopProcess(root, transitionPython(root), []string{"-B", "-c", source, filepath.Join(repository, "scripts/lib"), root}, nil)
	Expect(out.status).To(BeZero(), "%+v", out)
	var results []bool
	Expect(json.Unmarshal([]byte(out.stdout), &results)).To(Succeed())
	Expect(results).To(Equal([]bool{admitted, admitted}), "actual legacy Python shared/exclusive API observes the same pending barrier")
}

var _ = Describe("Interrupted recovery compiled client core", func() {
	// per docs/adr/0096-interrupted-publication-recovery.md:176
	// per docs/adr/0096-interrupted-publication-recovery.md:177
	// per docs/adr/0096-interrupted-publication-recovery.md:200
	// per docs/adr/0096-interrupted-publication-recovery.md:245
	// per docs/adr/0096-interrupted-publication-recovery.md:125
	DescribeTable("freshly resolves an actual killed owner without changing known desired images unnecessarily", func(applied bool, direction, image, terminal string) {
		binaryRoot, root, publication := durablePublicationFixture()
		original := durableBytes(root, publication.Path)
		owner := durablePublicationBegin(binaryRoot, root, publication)
		if applied {
			durablePublicationSuccess(owner.call("apply"))
		}
		selected, err := os.Lstat(filepath.Join(root, publication.Path))
		Expect(err).NotTo(HaveOccurred())
		pending := publicationFaultPendingExternal(root)
		interruptedRecoveryKill(owner)
		protected := durablePublicationProtected(root, publication.Path)
		queries := durableBytes(binaryRoot, "durable-git-calls")
		before := assessmentTree(root)
		request := interruptedRecoveryRequest(root, publication, direction)
		client := interruptedRecoveryStart(binaryRoot, root, request)
		digest := interruptedRecoveryProposal(client.call("propose", ""), root, direction, image)
		Expect(assessmentTree(root)).To(Equal(before), "proposal is wholly read-only")
		Expect(durableBytes(binaryRoot, "durable-git-calls")).To(Equal(queries), "proposal cannot launch Git or child work")
		publicationPendingPreserved(root, pending)
		for _, acquire := range []func(context.Context, string) (*transition.Guard, error){transition.Shared, transition.Exclusive} {
			guard, err := acquire(context.Background(), root)
			Expect(err).To(HaveOccurred())
			Expect(guard).To(BeNil())
		}
		interruptedRecoveryPythonAdmission(root, false)
		Expect(assessmentTree(root)).To(Equal(before), "refused Python admission cannot repair controls or create activity")
		durablePublicationSuccess(client.call("begin", digest))
		transitionLockBlocked(root)
		grantedQueries := durableBytes(binaryRoot, "durable-git-calls")
		durablePublicationSuccess(client.call("complete", ""))
		actual, err := os.Lstat(filepath.Join(root, publication.Path))
		Expect(err).NotTo(HaveOccurred())
		expected := original
		if direction == "forward" {
			expected = publication.Replacement
		}
		Expect(durableBytes(root, publication.Path)).To(Equal(expected))
		Expect(actual.Mode()).To(Equal(selected.Mode()))
		if !applied || direction == "forward" {
			Expect(os.SameFile(selected, actual)).To(BeTrue(), "an already desired checked image must not be renamed again")
		} else {
			Expect(os.SameFile(selected, actual)).To(BeFalse(), "reverse publishes the known saved original")
		}
		_, err = os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		inspected := durablePublicationStart(binaryRoot, root, "inspect", publication)
		durablePublicationInventory(inspected.response("inspect"), publication, terminal, "0")
		inspected.wait()
		durablePublicationSuccess(client.call("close", ""))
		client.wait()
		Expect(durableBytes(binaryRoot, "durable-git-calls")).To(Equal(grantedQueries), "completion and closure are filesystem-only")
		Expect(durablePublicationProtected(root, publication.Path)).To(Equal(protected))
		livePublicationNoExecutionState(root)
		interruptedRecoveryPythonAdmission(root, true)
		livePublicationNoExecutionState(root)
		guard, err := transition.Shared(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		Expect(guard.Close(context.Background(), true)).To(Succeed())
	}, Entry("killed prepared owner reverse abort", false, "reverse", "before", "aborted"), Entry("killed applied owner forward completion", true, "forward", "after", "forward_completed"), Entry("killed applied owner explicit reverse", true, "reverse", "after", "restored"))

	// per docs/adr/0096-interrupted-publication-recovery.md:207
	// per docs/adr/0096-interrupted-publication-recovery.md:212
	// per docs/adr/0096-interrupted-publication-recovery.md:213
	// per docs/adr/0096-interrupted-publication-recovery.md:215
	It("close releases a fresh grant without completing it and requires another fresh consent", func() {
		binaryRoot, root, publication := durablePublicationFixture()
		owner := durablePublicationBegin(binaryRoot, root, publication)
		durablePublicationSuccess(owner.call("apply"))
		interruptedRecoveryKill(owner)
		pending := publicationFaultPendingExternal(root)
		before := assessmentTree(root)
		request := interruptedRecoveryRequest(root, publication, "forward")
		client := interruptedRecoveryStart(binaryRoot, root, request)
		digest := interruptedRecoveryProposal(client.call("propose", ""), root, "forward", "after")
		durablePublicationSuccess(client.call("begin", digest))
		closed := client.call("close", "")
		Expect(closed).To(HaveKeyWithValue("status", json.Number("1")))
		client.wait()
		Expect(assessmentTree(root)).To(Equal(before))
		publicationPendingPreserved(root, pending)
		retry := interruptedRecoveryStart(binaryRoot, root, request)
		fresh := interruptedRecoveryProposal(retry.call("propose", ""), root, "forward", "after")
		durablePublicationSuccess(retry.call("begin", fresh))
		durablePublicationSuccess(retry.call("complete", ""))
		durablePublicationSuccess(retry.call("close", ""))
		retry.wait()
	})
})

var _ = Describe("Interrupted recovery compiled client boundaries", func() {
	// per docs/adr/0096-interrupted-publication-recovery.md:48
	// per docs/adr/0096-interrupted-publication-recovery.md:55
	// per docs/adr/0096-interrupted-publication-recovery.md:70
	DescribeTable("preserves actual interrupted evidence when separately known trust or affirmation is absent", func(change string) {
		binaryRoot, root, publication := durablePublicationFixture()
		owner := durablePublicationBegin(binaryRoot, root, publication)
		durablePublicationSuccess(owner.call("apply"))
		interruptedRecoveryKill(owner)
		request := interruptedRecoveryRequest(root, publication, "forward")
		valid := interruptedRecoveryStart(binaryRoot, root, request)
		interruptedRecoveryProposal(valid.call("propose", ""), root, "forward", "after")
		valid.wait()
		if change == "known hash" {
			reference := request["AfterReference"].(assessment.Observation)
			reference.SHA256 = strings.Repeat("b", 64)
			request["AfterReference"] = reference
		} else {
			request["UnbridgedQuiescent"] = false
		}
		before := assessmentTree(root)
		pending := publicationFaultPendingExternal(root)
		queries := durableBytes(binaryRoot, "durable-git-calls")
		invalid := interruptedRecoveryStart(binaryRoot, root, request)
		response := invalid.call("propose", "")
		Expect(response).To(HaveKeyWithValue("status", json.Number("2")))
		Expect(response).NotTo(HaveKey("proposal"))
		Expect(response["error"]).NotTo(ContainSubstring(root))
		invalid.wait()
		Expect(assessmentTree(root)).To(Equal(before))
		publicationPendingPreserved(root, pending)
		Expect(durableBytes(binaryRoot, "durable-git-calls")).To(Equal(queries))
	}, Entry("journal cannot replace an independently known after hash", "known hash"), Entry("fresh operator affirmation is required", "quiescence"))

	// per docs/adr/0096-interrupted-publication-recovery.md:88
	// per docs/adr/0096-interrupted-publication-recovery.md:127
	// per docs/adr/0096-interrupted-publication-recovery.md:166
	It("refuses fresh-grant replay after an equal-byte foreign active inode replaces the proposed file", func() {
		binaryRoot, root, publication := durablePublicationFixture()
		owner := durablePublicationBegin(binaryRoot, root, publication)
		durablePublicationSuccess(owner.call("apply"))
		interruptedRecoveryKill(owner)
		request := interruptedRecoveryRequest(root, publication, "forward")
		client := interruptedRecoveryStart(binaryRoot, root, request)
		digest := interruptedRecoveryProposal(client.call("propose", ""), root, "forward", "after")
		selected := filepath.Join(root, publication.Path)
		held := filepath.Join(binaryRoot, "evaluator-held-after")
		Expect(os.Rename(selected, held)).To(Succeed())
		writeFixture(selected, publication.Replacement, 0755)
		foreign, err := os.Lstat(selected)
		Expect(err).NotTo(HaveOccurred())
		before := assessmentTree(root)
		queries := durableBytes(binaryRoot, "durable-git-calls")
		response := client.call("begin", digest)
		Expect(response).To(HaveKeyWithValue("status", json.Number("2")))
		client.wait()
		Expect(assessmentTree(root)).To(Equal(before))
		actual, err := os.Lstat(selected)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(foreign, actual)).To(BeTrue())
		Expect(durableBytes(root, publication.Path)).To(Equal(publication.Replacement))
		Expect(durableBytes(binaryRoot, "durable-git-calls")).To(Equal(queries))
	})

	// per docs/adr/0096-interrupted-publication-recovery.md:55
	// per docs/adr/0096-interrupted-publication-recovery.md:113
	// per docs/adr/0096-interrupted-publication-recovery.md:125
	It("refuses a fresh recovery grant while the actual prior owner is still alive and holds the permanent flock", func() {
		binaryRoot, root, publication := durablePublicationFixture()
		owner := durablePublicationBegin(binaryRoot, root, publication)
		durablePublicationSuccess(owner.call("apply"))
		request := interruptedRecoveryRequest(root, publication, "forward")
		client := interruptedRecoveryStart(binaryRoot, root, request)
		digest := interruptedRecoveryProposal(client.call("propose", ""), root, "forward", "after")
		before := assessmentTree(root)
		queries := durableBytes(binaryRoot, "durable-git-calls")
		Expect(owner.command.Process.Signal(syscall.Signal(0))).To(Succeed(), "real owner is alive at the refusal boundary")
		transitionLockBlocked(root)
		response := client.call("begin", digest)
		Expect(response).To(HaveKeyWithValue("status", json.Number("2")))
		client.wait()
		Expect(assessmentTree(root)).To(Equal(before))
		Expect(durableBytes(binaryRoot, "durable-git-calls")).To(Equal(queries))
		durablePublicationSuccess(owner.call("finish"))
		durablePublicationSuccess(owner.call("close"))
		owner.wait()
	})
})
