package acceptance_test

import (
	"bytes"
	"context"
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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

// This evaluator-only client links the actual exported API. Its stdin protocol
// lets the evaluator observe filesystem identities between real lifecycle calls.
// It is never installed or dispatched as a factory command.
// per docs/adr/0095-durable-live-publication.md:162
// per docs/adr/0095-durable-live-publication.md:177
const durablePublicationSource = `package main
import("context";"encoding/json";"os";"strings";"github.com/anoop2811/software-factory-template/internal/assessment")
type reply struct{Method string ` + "`json:\"method\"`" + `;Status int ` + "`json:\"status\"`" + `;Error string ` + "`json:\"error\"`" + `;Inventory any ` + "`json:\"inventory,omitempty\"`" + `}
func main(){ctx:=context.Background();enc:=json.NewEncoder(os.Stdout);dec:=json.NewDecoder(os.Stdin);root:=os.Args[1];emit:=func(method string,err error){message:="";status:=0;if err!=nil{message=err.Error();status=assessment.ErrorStatus(err)};_ = enc.Encode(reply{Method:method,Status:status,Error:message})};inspect:=func(){report,err:=assessment.InspectPublications(ctx,root);if err!=nil{emit("inspect",err)}else{_ = enc.Encode(reply{Method:"inspect",Status:report.Status(),Inventory:report})}};if os.Args[2]=="inspect"{inspect();return};var request assessment.PublicationRequest;if dec.Decode(&request)!=nil{os.Exit(2)};environment:=map[string]string{};for _,entry:=range os.Environ(){key,value,ok:=strings.Cut(entry,"=");if ok{environment[key]=value}};operation,err:=assessment.BeginDurablePublication(ctx,root,request,environment);emit("begin",err);if err!=nil{return};defer func(){_ = operation.Close(ctx)}();for{var method string;if dec.Decode(&method)!=nil{return};switch method{case "apply":err=operation.Apply(ctx);case "restore":err=operation.Restore(ctx);case "finish":err=operation.Finish(ctx);case "close":err=operation.Close(ctx);case "inspect":inspect();continue;default:os.Exit(2)};emit(method,err);if method=="close"{return}}}
`

var durablePublicationBinary []byte

func durablePublicationBuild() []byte {
	GinkgoHelper()
	if durablePublicationBinary != nil {
		return durablePublicationBinary
	}
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	dir := GinkgoT().TempDir()
	module, err := os.ReadFile(filepath.Join(repository, "go.mod"))
	Expect(err).NotTo(HaveOccurred())
	parts := strings.SplitN(string(module), "\ngo ", 2)
	Expect(parts).To(HaveLen(2))
	version := strings.Fields(parts[1])[0]
	writeFixture(filepath.Join(dir, "go.mod"), []byte("module github.com/anoop2811/software-factory-template/acceptance/durablepublicationfixture\n\ngo "+version+"\nrequire github.com/anoop2811/software-factory-template v0.0.0\nreplace github.com/anoop2811/software-factory-template => "+repository+"\n"), 0600)
	writeFixture(filepath.Join(dir, "main.go"), []byte(durablePublicationSource), 0600)
	args := []string{"build", "-mod=mod", "-o", filepath.Join(dir, "durable-publication"), "."}
	if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
		args = append([]string{"build", "-race"}, args[1:]...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", args...) // #nosec G204 -- fixed evaluator module/build arguments.
	command.Dir, command.Env = dir, append(os.Environ(), "GOPROXY=off")
	output, err := command.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "actual production client compilation: %s", output)
	durablePublicationBinary, err = os.ReadFile(filepath.Join(dir, "durable-publication"))
	Expect(err).NotTo(HaveOccurred())
	return durablePublicationBinary
}

type durablePublicationClient struct {
	command *exec.Cmd
	input   io.WriteCloser
	encode  *json.Encoder
	decode  *json.Decoder
	stderr  bytes.Buffer
	waited  bool
	cancel  context.CancelFunc
}

func durablePublicationStart(binaryRoot, root, mode string, request assessment.PublicationRequest) *durablePublicationClient {
	GinkgoHelper()
	program := filepath.Join(binaryRoot, "durable-publication-client")
	writeFixture(program, durablePublicationBuild(), 0700)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	client := &durablePublicationClient{cancel: cancel}
	client.command = exec.CommandContext(ctx, program, root, mode) // #nosec G204 -- owned evaluator binary and fixture paths.
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
	if mode != "inspect" {
		Expect(client.encode.Encode(request)).To(Succeed())
	}
	return client
}

func (client *durablePublicationClient) response(method string) map[string]any {
	GinkgoHelper()
	var object map[string]any
	Expect(client.decode.Decode(&object)).To(Succeed(), "actual client output for %s", method)
	Expect(object).To(HaveKeyWithValue("method", method))
	return object
}

func (client *durablePublicationClient) call(method string) map[string]any {
	GinkgoHelper()
	Expect(client.encode.Encode(method)).To(Succeed())
	return client.response(method)
}

func durablePublicationSuccess(object map[string]any) {
	GinkgoHelper()
	Expect(object).To(HaveKeyWithValue("status", json.Number("0")), "%+v", object)
	Expect(object).To(HaveKeyWithValue("error", ""))
}

func (client *durablePublicationClient) wait() {
	GinkgoHelper()
	Expect(client.input.Close()).To(Succeed())
	Expect(client.command.Wait()).To(Succeed(), "%s", client.stderr.String())
	client.waited = true
	client.cancel()
	Expect(client.stderr.String()).To(BeEmpty())
}

func durablePublicationFixture() (string, string, assessment.PublicationRequest) {
	GinkgoHelper()
	binaryRoot, root, request := livePublicationFixture()
	git, err := exec.LookPath("git")
	Expect(err).NotTo(HaveOccurred())
	quoted := "'" + strings.ReplaceAll(git, "'", "'\\''") + "'"
	writeFixture(filepath.Join(binaryRoot, "bin/git"), []byte("#!/bin/sh\nprintf 'query\\n' >> \"$DURABLE_GIT_MARKER\"\nexec "+quoted+" \"$@\"\n"), 0700)
	return binaryRoot, root, request
}

func durablePublicationProtected(root, selected string) map[string]assessedFileState {
	GinkgoHelper()
	states := livePublicationUntouched(root, selected)
	for path := range states {
		if path == ".factory/backups/.publications" || strings.HasPrefix(path, ".factory/backups/.publications/") {
			delete(states, path)
		}
	}
	return states
}

func durablePublicationSlot(root, id string) string {
	return filepath.Join(root, ".factory/backups/.publications", id+".json")
}

func durablePublicationRecord(root, id string) os.FileInfo {
	GinkgoHelper()
	directory, err := os.Lstat(filepath.Join(root, ".factory/backups/.publications"))
	Expect(err).NotTo(HaveOccurred())
	Expect(directory.IsDir()).To(BeTrue())
	Expect(directory.Mode().Perm()).To(Equal(os.FileMode(0700)))
	info, err := os.Lstat(durablePublicationSlot(root, id))
	Expect(err).NotTo(HaveOccurred())
	Expect(info.Mode().IsRegular()).To(BeTrue())
	Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
	Expect(info.Size()).To(BeNumerically(">", 0))
	Expect(info.Size()).To(BeNumerically("<=", 64*1024))
	data := durableBytes(root, ".factory/backups/.publications/"+id+".json")
	Expect(json.Valid(data)).To(BeTrue())
	Expect(string(data)).NotTo(ContainSubstring(root))
	Expect(string(data)).NotTo(ContainSubstring("PRIVATE_"))
	Expect(string(data)).NotTo(ContainSubstring("LIVE_PUBLICATION_REPLACEMENT"))
	return info
}

func durablePublicationInventory(response map[string]any, request assessment.PublicationRequest, phase string, status string) map[string]any {
	GinkgoHelper()
	Expect(response).To(HaveKeyWithValue("status", json.Number(status)))
	Expect(response).To(HaveKeyWithValue("error", ""))
	inventory, ok := response["inventory"].(map[string]any)
	Expect(ok).To(BeTrue(), "%+v", response)
	Expect(inventory).To(HaveLen(12))
	Expect(inventory).To(HaveKeyWithValue("schema_version", json.Number("1")))
	Expect(inventory).To(HaveKeyWithValue("mode", "inspect_publications"))
	Expect(inventory).To(HaveKeyWithValue("complete", true))
	Expect(inventory).To(HaveKeyWithValue("record_count", json.Number("1")))
	for _, field := range []string{"restorable", "rollback_ready", "activation_ready", "applicable", "prune_authorized"} {
		Expect(inventory).To(HaveKeyWithValue(field, false))
	}
	for _, field := range []string{"root_status", "pending_status"} {
		Expect(inventory[field]).To(BeAssignableToTypeOf(""))
		Expect(inventory[field]).NotTo(BeEmpty())
	}
	records, ok := inventory["records"].([]any)
	Expect(ok).To(BeTrue())
	Expect(records).To(HaveLen(1))
	record, ok := records[0].(map[string]any)
	Expect(ok).To(BeTrue())
	Expect(record).To(HaveLen(7))
	Expect(record).To(HaveKeyWithValue("migration_id", request.MigrationID))
	Expect(record).To(HaveKeyWithValue("path", request.Path))
	Expect(record).To(HaveKeyWithValue("phase", phase))
	outcome := phase
	if phase != "forward_completed" && phase != "restored" && phase != "aborted" {
		outcome = "pending"
	}
	Expect(record).To(HaveKeyWithValue("outcome", outcome))
	for _, field := range []string{"operation_id", "classification", "next_action"} {
		Expect(record[field]).To(BeAssignableToTypeOf(""))
		Expect(record[field]).NotTo(BeEmpty())
	}
	return inventory
}

func durablePublicationBegin(binaryRoot, root string, request assessment.PublicationRequest) *durablePublicationClient {
	GinkgoHelper()
	client := durablePublicationStart(binaryRoot, root, "begin", request)
	durablePublicationSuccess(client.response("begin"))
	queries := durableBytes(binaryRoot, "durable-git-calls")
	Expect(queries).NotTo(BeEmpty(), "effective ignored/untracked storage is proved through real Git queries")
	durablePublicationRecord(root, request.MigrationID)
	return client
}

var _ = Describe("Durable live publication core", func() {
	// per docs/adr/0095-durable-live-publication.md:129
	// per docs/adr/0095-durable-live-publication.md:131
	// per docs/adr/0095-durable-live-publication.md:135
	It("finishes the actual after-inode and retains an inert terminal record after removing pending", func() {
		binaryRoot, root, request := durablePublicationFixture()
		protected := durablePublicationProtected(root, request.Path)
		before, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		client := durablePublicationBegin(binaryRoot, root, request)
		queries := durableBytes(binaryRoot, "durable-git-calls")
		durablePublicationSuccess(client.call("apply"))
		applied, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, applied)).To(BeFalse())
		Expect(applied.Mode()).To(Equal(before.Mode()))
		Expect(durableBytes(root, request.Path)).To(Equal(request.Replacement))
		pending := publicationFaultPendingExternal(root)
		durablePublicationSuccess(client.call("finish"))
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(applied, after)).To(BeTrue(), "Finish never republishes the active file")
		Expect(after.Mode()).To(Equal(applied.Mode()))
		Expect(durableBytes(root, request.Path)).To(Equal(request.Replacement))
		_, err = os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(pending.Mode().Perm()).To(Equal(os.FileMode(0600)))
		durablePublicationRecord(root, request.MigrationID)
		durablePublicationInventory(client.call("inspect"), request, "forward_completed", "0")
		durablePublicationSuccess(client.call("close"))
		client.wait()
		Expect(durableBytes(binaryRoot, "durable-git-calls")).To(Equal(queries), "only constructor qualification may query Git")
		Expect(durablePublicationProtected(root, request.Path)).To(Equal(protected))
		livePublicationNoExecutionState(root)
	})

	// per docs/adr/0095-durable-live-publication.md:133
	// per docs/adr/0095-durable-live-publication.md:135
	// per docs/adr/0095-durable-live-publication.md:151
	It("durably restores the original and retains its terminal record without changing the saved set", func() {
		binaryRoot, root, request := durablePublicationFixture()
		original := durableBytes(root, request.Path)
		before, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		protected := durablePublicationProtected(root, request.Path)
		client := durablePublicationBegin(binaryRoot, root, request)
		queries := durableBytes(binaryRoot, "durable-git-calls")
		durablePublicationSuccess(client.call("apply"))
		durablePublicationSuccess(client.call("restore"))
		restored, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(restored.Mode()).To(Equal(before.Mode()))
		Expect(durableBytes(root, request.Path)).To(Equal(original))
		_, err = os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		durablePublicationRecord(root, request.MigrationID)
		durablePublicationInventory(client.call("inspect"), request, "restored", "0")
		durablePublicationSuccess(client.call("close"))
		client.wait()
		Expect(durableBytes(binaryRoot, "durable-git-calls")).To(Equal(queries))
		Expect(durablePublicationProtected(root, request.Path)).To(Equal(protected))
		livePublicationNoExecutionState(root)
	})

	// per docs/adr/0095-durable-live-publication.md:70
	// per docs/adr/0095-durable-live-publication.md:72
	It("preserves every occupied foreign slot and refuses it before a valid empty-slot control", func() {
		binaryRoot, root, request := durablePublicationFixture()
		directory := filepath.Join(root, ".factory/backups/.publications")
		Expect(os.Mkdir(directory, 0700)).To(Succeed())
		slot := durablePublicationSlot(root, request.MigrationID)
		foreign := []byte("PRIVATE_FOREIGN_RECEIPT\n")
		writeFixture(slot, foreign, 0600)
		occupied, err := os.Lstat(slot)
		Expect(err).NotTo(HaveOccurred())
		active, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		original := durableBytes(root, request.Path)
		protected := durablePublicationProtected(root, request.Path)
		client := durablePublicationStart(binaryRoot, root, "begin", request)
		refused := client.response("begin")
		Expect(refused).To(HaveKeyWithValue("status", json.Number("2")))
		Expect(refused["error"]).NotTo(ContainSubstring(root))
		Expect(refused["error"]).NotTo(ContainSubstring("PRIVATE_"))
		client.wait()
		after, err := os.Lstat(slot)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(occupied, after)).To(BeTrue())
		Expect(after.Mode()).To(Equal(occupied.Mode()))
		Expect(durableBytes(root, ".factory/backups/.publications/"+request.MigrationID+".json")).To(Equal(foreign))
		afterActive, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(active, afterActive)).To(BeTrue())
		Expect(durableBytes(root, request.Path)).To(Equal(original))
		Expect(durablePublicationProtected(root, request.Path)).To(Equal(protected))
		Expect(os.Remove(slot)).To(Succeed(), "only the evaluator removes its own foreign fixture for the paired admission control")
		valid := durablePublicationBegin(binaryRoot, root, request)
		durablePublicationSuccess(valid.call("apply"))
		durablePublicationSuccess(valid.call("finish"))
		durablePublicationSuccess(valid.call("close"))
		valid.wait()
	})

	// per docs/adr/0095-durable-live-publication.md:34
	// per docs/adr/0095-durable-live-publication.md:37
	// per docs/adr/0095-durable-live-publication.md:136
	It("inspects a terminal record beside pending after restart without granting or consuming authority", func() {
		binaryRoot, root, request := durablePublicationFixture()
		client := durablePublicationBegin(binaryRoot, root, request)
		durablePublicationSuccess(client.call("apply"))
		durablePublicationSuccess(client.call("finish"))
		terminal := durablePublicationInventory(client.call("inspect"), request, "forward_completed", "0")
		durablePublicationSuccess(client.call("close"))
		client.wait()
		writeFixture(filepath.Join(root, ".factory/runtime-publication.pending"), nil, 0600)
		pending := publicationFaultPendingExternal(root)
		before := assessmentTree(root)
		queries := durableBytes(binaryRoot, "durable-git-calls")
		restarted := durablePublicationStart(binaryRoot, root, "inspect", request)
		inventory := durablePublicationInventory(restarted.response("inspect"), request, "forward_completed", "2")
		restarted.wait()
		Expect(inventory["pending_status"]).NotTo(Equal(terminal["pending_status"]))
		Expect(assessmentTree(root)).To(Equal(before), "restart inspection creates, repairs and removes no storage")
		publicationPendingPreserved(root, pending)
		Expect(durableBytes(binaryRoot, "durable-git-calls")).To(Equal(queries), "restart inspection launches no Git query")
		livePublicationNoExecutionState(root)
	})
})

func publicationFaultPendingExternal(root string) os.FileInfo {
	GinkgoHelper()
	info, err := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
	Expect(err).NotTo(HaveOccurred())
	Expect(info.Mode().IsRegular()).To(BeTrue())
	Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
	Expect(info.Size()).To(BeZero())
	return info
}

var _ = Describe("Durable live publication qualification", func() {
	// per docs/adr/0095-durable-live-publication.md:29
	// per docs/adr/0095-durable-live-publication.md:145
	It("refuses unapplied Finish without preventing checked abort and its terminal record", func() {
		binaryRoot, root, request := durablePublicationFixture()
		original := durableBytes(root, request.Path)
		before, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		client := durablePublicationBegin(binaryRoot, root, request)
		refused := client.call("finish")
		Expect(refused).To(HaveKeyWithValue("status", json.Number("2")))
		durablePublicationSuccess(client.call("restore"))
		durablePublicationInventory(client.call("inspect"), request, "aborted", "0")
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue(), "invalid Finish cannot turn a prepared abort into active publication")
		Expect(after.Mode()).To(Equal(before.Mode()))
		Expect(durableBytes(root, request.Path)).To(Equal(original))
		durablePublicationSuccess(client.call("close"))
		client.wait()
	})

	// per docs/adr/0095-durable-live-publication.md:26
	// per docs/adr/0095-durable-live-publication.md:29
	It("preserves legacy live publication after Finish refuses without creating records or querying Git", func() {
		binaryRoot, root, request := durablePublicationFixture()
		before := durablePublicationProtected(root, request.Path)
		original := durableBytes(root, request.Path)
		operation, err := assessment.BeginPublication(context.Background(), root, request)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = operation.Close(context.Background()) })
		Expect(assessment.ErrorStatus(operation.Finish(context.Background()))).To(Equal(2))
		Expect(operation.Apply(context.Background())).To(Succeed())
		Expect(durableBytes(root, request.Path)).To(Equal(request.Replacement))
		Expect(assessment.ErrorStatus(operation.Finish(context.Background()))).To(Equal(2))
		Expect(operation.Restore(context.Background())).To(Succeed())
		Expect(operation.Close(context.Background())).To(Succeed())
		Expect(durableBytes(root, request.Path)).To(Equal(original))
		_, err = os.Lstat(filepath.Join(root, ".factory/backups/.publications"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		_, err = os.Lstat(filepath.Join(binaryRoot, "durable-git-calls"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(durablePublicationProtected(root, request.Path)).To(Equal(before))
		livePublicationNoExecutionState(root)
	})

	// per docs/adr/0095-durable-live-publication.md:74
	// per docs/adr/0095-durable-live-publication.md:79
	DescribeTable("preserves Git controls and refuses storage that lacks exact durable effective untracked exclusion", func(change string) {
		binaryRoot, root, request := durablePublicationFixture()
		exclude := filepath.Join(root, ".git/info/exclude")
		switch change {
		case "exact rule missing under broad project ignore":
			Expect(os.WriteFile(exclude, []byte("# operator exclusions retained\n"), 0600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(root, ".gitignore"), []byte("/.factory/\n"), 0600)).To(Succeed())
			durableGit(root, "check-ignore", "--no-index", "-q", "--", ".factory/backups/.publications/")
		case "effective negation":
			Expect(os.WriteFile(exclude, []byte("/.factory/backups/\n!/.factory/backups/\n/.factory/backups/*\n!/.factory/backups/.publications/\n"), 0600)).To(Succeed())
			probe := exec.Command("git", "check-ignore", "--no-index", "-q", "--", ".factory/backups/.publications/") // #nosec G204 -- fixed fixture query verifies this exact negative control.
			probe.Dir, probe.Env = root, durableGitEnvironment()
			err := probe.Run()
			var exited *exec.ExitError
			Expect(errors.As(err, &exited)).To(BeTrue())
			Expect(exited.ExitCode()).To(Equal(1))
		case "tracked absent record slot":
			object := strings.TrimSpace(durableGit(root, "hash-object", "-w", "--", "scripts/factory-budget.sh"))
			durableGit(root, "update-index", "--add", "--cacheinfo", "100600", object, ".factory/backups/.publications/"+request.MigrationID+".json")
			Expect(durableGit(root, "ls-files", "--", ".factory/backups/.publications")).NotTo(BeEmpty())
		}
		before := durablePublicationProtected(root, request.Path)
		active, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		original := durableBytes(root, request.Path)
		client := durablePublicationStart(binaryRoot, root, "begin", request)
		refused := client.response("begin")
		Expect(refused).To(HaveKeyWithValue("status", json.Number("2")))
		client.wait()
		if change != "exact rule missing under broad project ignore" {
			queries := durableBytes(binaryRoot, "durable-git-calls")
			Expect(queries).NotTo(BeEmpty(), "actual Git visibility must be assessed, not inferred from caller metadata")
		}
		pending := publicationFaultPendingExternal(root)
		publicationPendingPreserved(root, pending)
		_, err = os.Lstat(durablePublicationSlot(root, request.MigrationID))
		Expect(os.IsNotExist(err)).To(BeTrue(), "failed exclusion qualification cannot publish a record")
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(active, after)).To(BeTrue())
		Expect(after.Mode()).To(Equal(active.Mode()))
		Expect(durableBytes(root, request.Path)).To(Equal(original))
		Expect(durablePublicationProtected(root, request.Path)).To(Equal(before))
	}, Entry("exact rule missing under broad project ignore", "exact rule missing under broad project ignore"),
		Entry("effective negation", "effective negation"), Entry("tracked absent record slot", "tracked absent record slot"))

	// per docs/adr/0095-durable-live-publication.md:93
	// per docs/adr/0095-durable-live-publication.md:101
	// per docs/adr/0095-durable-live-publication.md:108
	DescribeTable("reports malformed or unsafe existing records without modifying or granting authority", func(change string) {
		binaryRoot, root, request := durablePublicationFixture()
		client := durablePublicationBegin(binaryRoot, root, request)
		durablePublicationSuccess(client.call("apply"))
		durablePublicationSuccess(client.call("finish"))
		durablePublicationSuccess(client.call("close"))
		client.wait()
		slot := durablePublicationSlot(root, request.MigrationID)
		data, err := os.ReadFile(slot)
		Expect(err).NotTo(HaveOccurred())
		var record map[string]any
		Expect(json.Unmarshal(data, &record)).To(Succeed())
		referent := filepath.Join(binaryRoot, "owned-journal-referent")
		switch change {
		case "unknown key":
			record["unexpected_authority"] = true
			data, err = json.Marshal(record)
			Expect(err).NotTo(HaveOccurred())
		case "unsupported schema":
			record["schema_version"] = 999
			data, err = json.Marshal(record)
			Expect(err).NotTo(HaveOccurred())
		case "duplicate key":
			data = append([]byte(`{"schema_version":1,`), bytes.TrimSpace(data)[1:]...)
			Expect(json.Valid(data)).To(BeTrue(), "duplicate-key control remains syntactically valid JSON")
		case "truncated JSON":
			data = data[:len(data)/2]
		case "oversized":
			data = bytes.Repeat([]byte("x"), 64*1024+1)
		case "foreign symlink":
			Expect(os.Rename(slot, referent)).To(Succeed())
			Expect(os.Symlink(referent, slot)).To(Succeed())
		case "foreign hard link":
			Expect(os.Link(slot, referent)).To(Succeed())
		}
		if change != "foreign symlink" && change != "foreign hard link" {
			Expect(os.WriteFile(slot, data, 0600)).To(Succeed())
		}
		before := assessmentTree(root)
		queries := durableBytes(binaryRoot, "durable-git-calls")
		inspect := durablePublicationStart(binaryRoot, root, "inspect", request)
		observed := inspect.response("inspect")
		Expect(observed).To(HaveKeyWithValue("status", json.Number("2")))
		inventory, ok := observed["inventory"].(map[string]any)
		Expect(ok).To(BeTrue(), "%+v", observed)
		for _, field := range []string{"restorable", "rollback_ready", "activation_ready", "applicable", "prune_authorized"} {
			Expect(inventory).To(HaveKeyWithValue(field, false))
		}
		inspect.wait()
		Expect(assessmentTree(root)).To(Equal(before))
		if change == "foreign symlink" || change == "foreign hard link" {
			Expect(durableBytes(binaryRoot, "owned-journal-referent")).To(Equal(data), "unsafe namespace inspection never modifies the outside referent")
		}
		Expect(durableBytes(binaryRoot, "durable-git-calls")).To(Equal(queries))
	}, Entry("unknown key", "unknown key"), Entry("unsupported schema", "unsupported schema"),
		Entry("duplicate key", "duplicate key"), Entry("truncated JSON", "truncated JSON"),
		Entry("oversized", "oversized"), Entry("foreign symlink", "foreign symlink"), Entry("foreign hard link", "foreign hard link"))
})

var _ = Describe("Durable live publication required record fields", func() {
	// per docs/adr/0095-durable-live-publication.md:101
	// per docs/adr/0095-durable-live-publication.md:104
	DescribeTable("rejects a missing or null required byte count beside an actual valid zero-byte publication", func(nullValue bool) {
		binaryRoot, root, request := durablePublicationFixture()
		request.Replacement = []byte{}
		client := durablePublicationBegin(binaryRoot, root, request)
		durablePublicationSuccess(client.call("apply"))
		durablePublicationSuccess(client.call("finish"))
		durablePublicationInventory(client.call("inspect"), request, "forward_completed", "0")
		durablePublicationSuccess(client.call("close"))
		client.wait()
		Expect(durableBytes(root, request.Path)).To(BeEmpty(), "the actual generated valid record describes a legitimate zero-byte after-image")
		slot := durablePublicationSlot(root, request.MigrationID)
		data, err := os.ReadFile(slot)
		Expect(err).NotTo(HaveOccurred())
		var record map[string]any
		Expect(json.Unmarshal(data, &record)).To(Succeed())
		after, ok := record["after"].(map[string]any)
		Expect(ok).To(BeTrue(), "the actual generated schema contains its recorded after-image")
		Expect(after).To(HaveKeyWithValue("bytes", float64(0)))
		if nullValue {
			after["bytes"] = nil
		} else {
			delete(after, "bytes")
		}
		data, err = json.Marshal(record)
		Expect(err).NotTo(HaveOccurred())
		Expect(json.Valid(data)).To(BeTrue(), "the negative control changes field presence/type, not JSON syntax")
		Expect(os.WriteFile(slot, data, 0600)).To(Succeed())
		before := assessmentTree(root)
		queries := durableBytes(binaryRoot, "durable-git-calls")
		inspect := durablePublicationStart(binaryRoot, root, "inspect", request)
		observed := inspect.response("inspect")
		inventory, ok := observed["inventory"].(map[string]any)
		Expect(ok).To(BeTrue())
		for _, field := range []string{"restorable", "rollback_ready", "activation_ready", "applicable", "prune_authorized"} {
			Expect(inventory).To(HaveKeyWithValue(field, false))
		}
		inspect.wait()
		Expect(assessmentTree(root)).To(Equal(before))
		Expect(durableBytes(binaryRoot, "durable-git-calls")).To(Equal(queries))
		Expect(observed).To(HaveKeyWithValue("status", json.Number("2")), "required numeric presence/type cannot be inferred from the decoder's zero value")
	}, Entry("required after byte count omitted", false), Entry("required after byte count replaced by JSON null", true))
})
