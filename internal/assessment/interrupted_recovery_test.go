package assessment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/budget"
	"github.com/anoop2811/software-factory-template/internal/filepublish"
	"github.com/anoop2811/software-factory-template/internal/loop"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/transition"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

func interruptedFaultFixture(applied bool, direction string) (string, PublicationRequest, InterruptedRecoveryRequest, map[string]string) {
	GinkgoHelper()
	root, request, environment := durablePublicationOwnedFixture()
	owner, err := BeginDurablePublication(context.Background(), root, request, environment)
	Expect(err).NotTo(HaveOccurred())
	if applied {
		Expect(owner.Apply(context.Background())).To(Succeed())
	}
	Expect(owner.Close(context.Background())).To(HaveOccurred(), "evaluator explicitly releases old capability while preserving unresolved pending")
	return root, request, interruptedFaultRequest(root, request, direction), environment
}
func interruptedFaultRequest(root string, request PublicationRequest, direction string) InterruptedRecoveryRequest {
	GinkgoHelper()
	data := publicationFaultBytes(root, ".factory/backups/.publications/"+request.MigrationID+".json")
	var record map[string]any
	Expect(json.Unmarshal(data, &record)).To(Succeed())
	operation, ok := record["operation_id"].(string)
	Expect(ok).To(BeTrue())
	digest := sha256.Sum256(request.Replacement)
	return InterruptedRecoveryRequest{MigrationID: request.MigrationID, OperationID: operation, Path: request.Path, Direction: direction, Replacement: bytes.Clone(request.Replacement), AfterReference: Observation{SHA256: hex.EncodeToString(digest[:]), Bytes: int64(len(request.Replacement)), Mode: "0755"}, UnbridgedQuiescent: true}
}
func interruptedCapture(controls publicationOps) (publicationOps, *[]*os.File) {
	GinkgoHelper()
	files := []*os.File{}
	capture := func(file *os.File) {
		if file == nil {
			return
		}
		for _, known := range files {
			if known == file {
				return
			}
		}
		files = append(files, file)
	}
	storageOpen := controls.storage.open
	controls.storage.open = func(ctx context.Context, parent *os.File, name string, flags int, mode uint32) (*os.File, error) {
		capture(parent)
		var file *os.File
		var err error
		if storageOpen != nil {
			file, err = storageOpen(ctx, parent, name, flags, mode)
		} else {
			fd, openErr := unix.Openat(int(parent.Fd()), name, flags, mode)
			err = openErr
			if err == nil {
				file = os.NewFile(uintptr(fd), name)
			}
		}
		capture(file)
		return file, err
	}
	observeOpen := controls.observe.open
	controls.observe.open = func(parent *os.File, name string, flags int) (*os.File, error) {
		capture(parent)
		var file *os.File
		var err error
		if observeOpen != nil {
			file, err = observeOpen(parent, name, flags)
		} else {
			file, err = openAt(parent, name, flags)
		}
		capture(file)
		return file, err
	}
	openPrepared := controls.openPrepared
	controls.openPrepared = func(ctx context.Context, stage *filepublish.Stage) (*os.File, error) {
		var file *os.File
		var err error
		if openPrepared != nil {
			file, err = openPrepared(ctx, stage)
		} else {
			file, err = stage.OpenPrepared(ctx)
		}
		capture(file)
		return file, err
	}
	return controls, &files
}

func interruptedFaultProposal(root string, request InterruptedRecoveryRequest) InterruptedRecoveryProposal {
	GinkgoHelper()
	controls, files := interruptedCapture(publicationOps{})
	proposal, err := proposeInterruptedRecovery(context.Background(), root, request, controls.observe)
	Expect(err).NotTo(HaveOccurred())
	recoveryClosed(*files)
	Expect(proposal.ProposalDigest).To(MatchRegexp("^[0-9a-f]{64}$"))
	Expect(proposal.Restorable).To(BeFalse())
	Expect(proposal.RollbackReady).To(BeFalse())
	Expect(proposal.ActivationReady).To(BeFalse())
	Expect(proposal.Applicable).To(BeFalse())
	Expect(proposal.PruneAuthorized).To(BeFalse())
	return proposal
}
func interruptedFaultBegin(root string, request InterruptedRecoveryRequest, environment map[string]string, controls publicationOps) *InterruptedRecovery {
	GinkgoHelper()
	proposal := interruptedFaultProposal(root, request)
	controls, files := interruptedCapture(controls)
	operation, err := beginInterruptedRecovery(context.Background(), root, request, proposal.ProposalDigest, environment, controls)
	Expect(err).NotTo(HaveOccurred())
	Expect(operation).NotTo(BeNil())
	DeferCleanup(func() {
		_ = operation.Close(context.Background())
		recoveryClosed(*files)
		interruptedGuardReleased(root)
	})
	return operation
}

type interruptedFile struct {
	info os.FileInfo
	data []byte
}

func interruptedFaultTree(root string) map[string]interruptedFile {
	GinkgoHelper()
	result := map[string]interruptedFile{}
	Expect(filepath.WalkDir(root, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entry := interruptedFile{info: info}
		if info.Mode().IsRegular() {
			entry.data, err = os.ReadFile(path) // #nosec G122 -- stable evaluator-owned fixture snapshot, with no concurrent path mutation during this walk.
			if err != nil {
				return err
			}
		} else if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entry.data = []byte(target)
		}
		result[relative] = entry
		return nil
	})).To(Succeed())
	return result
}
func interruptedFaultPreserved(root string, before map[string]interruptedFile) {
	GinkgoHelper()
	after := interruptedFaultTree(root)
	Expect(after).To(HaveLen(len(before)))
	for path, prior := range before {
		current, ok := after[path]
		Expect(ok).To(BeTrue(), path)
		Expect(os.SameFile(prior.info, current.info)).To(BeTrue(), path)
		Expect(current.info.Mode()).To(Equal(prior.info.Mode()), path)
		Expect(current.info.ModTime()).To(Equal(prior.info.ModTime()), path)
		Expect(current.data).To(Equal(prior.data), path)
	}
}
func interruptedSame(file *os.File, path string) bool {
	info, err := file.Stat()
	other, namedErr := os.Lstat(path) // #nosec G703 -- identity oracle for fixed catalog/control paths in evaluator-owned private fixtures, including evaluator-only child environment.
	return err == nil && namedErr == nil && os.SameFile(info, other)
}

func interruptedGuardReleased(root string) {
	GinkgoHelper()
	file, err := os.OpenFile(filepath.Join(root, ".factory/runtime-transition.lock"), os.O_RDWR, 0)
	Expect(err).NotTo(HaveOccurred())
	defer func() { Expect(file.Close()).To(Succeed()) }()
	Expect(unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)).To(Succeed(), "actual OS flock is released even when pending remains")
	Expect(unix.Flock(int(file.Fd()), unix.LOCK_UN)).To(Succeed())
}

func interruptedFaultProtected(root, id string) map[string]interruptedFile {
	GinkgoHelper()
	all := interruptedFaultTree(root)
	result := map[string]interruptedFile{}
	for path, entry := range all {
		if strings.HasPrefix(path, ".factory/backups/"+id+"/") || path == ".git/index" || path == ".git/info/exclude" || path == ".factory/budget.json" || path == ".factory/loops.json" {
			result[path] = entry
		}
	}
	return result
}
func interruptedFaultProtectedPreserved(root string, before map[string]interruptedFile) {
	GinkgoHelper()
	after := interruptedFaultTree(root)
	for path, prior := range before {
		current, ok := after[path]
		Expect(ok).To(BeTrue(), path)
		Expect(os.SameFile(prior.info, current.info)).To(BeTrue(), path)
		Expect(current.info.Mode()).To(Equal(prior.info.Mode()), path)
		Expect(current.info.ModTime()).To(Equal(prior.info.ModTime()), path)
		Expect(current.data).To(Equal(prior.data), path)
	}
}

var _ = Describe("Interrupted recovery fresh authority controls", func() {
	// per docs/adr/0096-interrupted-publication-recovery.md:44
	// per docs/adr/0096-interrupted-publication-recovery.md:48
	// per docs/adr/0096-interrupted-publication-recovery.md:55
	// per docs/adr/0096-interrupted-publication-recovery.md:70
	DescribeTable("preserves checked evidence when fresh caller selection, trust or direction is invalid", func(change string) {
		root, _, request, _ := interruptedFaultFixture(true, "forward")
		interruptedFaultProposal(root, request)
		switch change {
		case "operation":
			request.OperationID = "another-operation"
		case "path":
			request.Path = "scripts/factory-loop.sh"
		case "migration":
			request.MigrationID = "another-set"
		case "direction":
			request.Direction = "automatic"
		case "quiescence":
			request.UnbridgedQuiescent = false
		case "known hash":
			request.AfterReference.SHA256 = strings.Repeat("b", 64)
		case "known size":
			request.AfterReference.Bytes++
		case "known mode":
			request.AfterReference.Mode = "0644"
		case "replacement bytes":
			request.Replacement = []byte("PRIVATE_OTHER_AFTER")
		case "oversized":
			request.Replacement = bytes.Repeat([]byte("x"), (1<<20)+1)
		}
		before := interruptedFaultTree(root)
		proposal, err := ProposeInterruptedRecovery(context.Background(), root, request)
		Expect(err).To(HaveOccurred())
		Expect(ErrorStatus(err)).To(Equal(2))
		Expect(proposal.ProposalDigest).To(BeEmpty())
		Expect(err.Error()).NotTo(ContainSubstring(root))
		Expect(err.Error()).NotTo(ContainSubstring("PRIVATE_"))
		interruptedFaultPreserved(root, before)
	}, Entry("wrong selected operation", "operation"), Entry("wrong catalog path", "path"), Entry("wrong saved set", "migration"), Entry("no direction", "direction"), Entry("no unbridged affirmation", "quiescence"), Entry("wrong independently known hash", "known hash"), Entry("wrong independently known count", "known size"), Entry("wrong independently known mode", "known mode"), Entry("wrong separately supplied bytes", "replacement bytes"), Entry("oversized independent image", "oversized"))

	// per docs/adr/0096-interrupted-publication-recovery.md:88
	// per docs/adr/0096-interrupted-publication-recovery.md:127
	// per docs/adr/0096-interrupted-publication-recovery.md:166
	DescribeTable("refuses stale consent after actual checked identity, bytes or absent state changes", func(change string) {
		root, publication, request, environment := interruptedFaultFixture(true, "forward")
		proposal := interruptedFaultProposal(root, request)
		relative := publication.Path
		switch change {
		case "record":
			relative = ".factory/backups/.publications/" + request.MigrationID + ".json"
		case "pending":
			relative = ".factory/runtime-publication.pending"
		case "saved":
			relative = ".factory/backups/" + request.MigrationID + "/files/" + request.Path
		}
		path := filepath.Join(root, relative)
		switch change {
		case "active", "record", "pending":
			data := publicationFaultBytes(root, relative)
			info, err := os.Lstat(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Rename(path, filepath.Join(GinkgoT().TempDir(), "evaluator-held-inode"))).To(Succeed())
			Expect(os.WriteFile(path, data, info.Mode().Perm())).To(Succeed())
		case "active edit", "saved":
			Expect(os.WriteFile(path, []byte("PRIVATE_POST_PROPOSAL_EDIT"), 0600)).To(Succeed())
		case "budget appears":
			Expect(os.WriteFile(filepath.Join(root, ".factory/budget.json"), []byte(`{"schema":1,"runs":[]}`), 0600)).To(Succeed())
		case "checkpoint appears":
			Expect(os.WriteFile(filepath.Join(root, ".factory/loops.json"), []byte(`{"schema":1,"runs":[]}`), 0600)).To(Succeed())
		}
		if change == "record" || change == "pending" || change == "budget appears" || change == "checkpoint appears" {
			fresh := interruptedFaultProposal(root, request)
			Expect(fresh.ProposalDigest).NotTo(Equal(proposal.ProposalDigest), "a valid current observation remains eligible; only the stale digest grants no authority")
		}
		before := interruptedFaultTree(root)
		controls, files := interruptedCapture(publicationOps{})
		operation, err := beginInterruptedRecovery(context.Background(), root, request, proposal.ProposalDigest, environment, controls)
		Expect(operation).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(ErrorStatus(err)).To(Equal(2))
		interruptedFaultPreserved(root, before)
		recoveryClosed(*files)
		interruptedGuardReleased(root)
	}, Entry("equal-byte foreign active inode", "active"), Entry("equal-byte foreign record inode", "record"), Entry("equal-byte foreign pending inode", "pending"), Entry("later user content edit", "active edit"), Entry("saved input edit", "saved"), Entry("formerly absent budget becomes present", "budget appears"), Entry("formerly absent checkpoint becomes present", "checkpoint appears"))

	// per docs/adr/0096-interrupted-publication-recovery.md:47
	// per docs/adr/0096-interrupted-publication-recovery.md:63
	// per docs/adr/0096-interrupted-publication-recovery.md:212
	It("clones caller bytes, serializes copied aliases and refuses completed, nil and zero handles", func() {
		root, publication, request, environment := interruptedFaultFixture(true, "reverse")
		original := publicationFaultBytes(root, ".factory/backups/"+request.MigrationID+"/files/"+request.Path)
		controls, files := publicationFaultControls()
		operation := interruptedFaultBegin(root, request, environment, controls)
		request.Replacement[0] ^= 0x20
		alias := *operation
		results := make(chan error, 2)
		var wait sync.WaitGroup
		for _, handle := range []*InterruptedRecovery{operation, &alias} {
			wait.Add(1)
			go func(handle *InterruptedRecovery) { defer wait.Done(); results <- handle.Complete(context.Background()) }(handle)
		}
		wait.Wait()
		close(results)
		successes, refusals := 0, 0
		for err := range results {
			if err == nil {
				successes++
			} else {
				Expect(ErrorStatus(err)).To(Equal(2))
				refusals++
			}
		}
		Expect(successes).To(Equal(1))
		Expect(refusals).To(Equal(1))
		Expect(publicationFaultBytes(root, publication.Path)).To(Equal(original))
		before := interruptedFaultTree(root)
		Expect(ErrorStatus(operation.Complete(context.Background()))).To(Equal(2))
		Expect(operation.Close(context.Background())).To(Succeed())
		Expect(ErrorStatus(alias.Complete(context.Background()))).To(Equal(2))
		for _, handle := range []*InterruptedRecovery{nil, {}} {
			Expect(ErrorStatus(handle.Complete(context.Background()))).To(Equal(2))
			Expect(ErrorStatus(handle.Close(context.Background()))).To(Equal(2))
		}
		interruptedFaultPreserved(root, before)
		recoveryClosed(*files)
		interruptedGuardReleased(root)
	})
})

var _ = Describe("Interrupted recovery actual completion I/O", func() {
	// per docs/adr/0096-interrupted-publication-recovery.md:191
	// per docs/adr/0096-interrupted-publication-recovery.md:200
	// per docs/adr/0096-interrupted-publication-recovery.md:203
	// per docs/adr/0096-interrupted-publication-recovery.md:205
	// per docs/adr/0096-interrupted-publication-recovery.md:210
	// per docs/adr/0096-interrupted-publication-recovery.md:216
	DescribeTable("retains checked direction and retries actual native operations after one injected EIO", func(boundary string) {
		direction := "forward"
		if boundary == "reverse rename" {
			direction = "reverse"
		}
		root, publication, request, environment := interruptedFaultFixture(true, direction)
		protected := interruptedFaultProtected(root, request.MigrationID)
		pending := publicationFaultPending(root)
		controls, files := publicationFaultControls()
		armed, triggered := false, false
		activeRenames, recordRenames := 0, 0
		controls.rename = func(ctx context.Context, stage *filepublish.Stage, name string) error {
			active := name == filepath.Base(request.Path)
			if active {
				activeRenames++
			} else {
				recordRenames++
			}
			err := stage.Publish(ctx, name)
			if err == nil && armed && !triggered && (boundary == "reverse rename" && active || boundary == "terminal record rename" && !active) {
				triggered = true
				return unix.EIO
			}
			return err
		}
		controls.storage.syncFile = func(_ context.Context, file *os.File) error {
			err := file.Sync()
			if err == nil && armed && !triggered && boundary == "known image sync" && interruptedSame(file, filepath.Join(root, request.Path)) {
				triggered = true
				return unix.EIO
			}
			return err
		}
		controls.storage.syncDirectory = func(_ context.Context, file *os.File) error {
			err := file.Sync()
			if err != nil || !armed || triggered {
				return err
			}
			if boundary == "terminal record parent sync" && recordRenames > 0 && interruptedSame(file, filepath.Join(root, ".factory/backups/.publications")) {
				triggered = true
				return unix.EIO
			}
			_, pendingErr := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
			if boundary == "post-unlink parent sync" && os.IsNotExist(pendingErr) && interruptedSame(file, filepath.Join(root, ".factory")) {
				triggered = true
				return unix.EIO
			}
			return nil
		}
		controls.close = func(file *os.File) error {
			isPending := interruptedSame(file, filepath.Join(root, ".factory/runtime-publication.pending"))
			err := file.Close()
			if err == nil && armed && !triggered && boundary == "pending close" && isPending {
				triggered = true
				return unix.EIO
			}
			return err
		}
		operation := interruptedFaultBegin(root, request, environment, controls)
		armed = true
		failed := operation.Complete(context.Background())
		Expect(triggered).To(BeTrue(), "actual specified native operation must precede injected EIO")
		Expect(failed).To(HaveOccurred())
		Expect(ErrorStatus(failed)).To(Equal(1))
		Expect(failed.Error()).NotTo(ContainSubstring(root))
		Expect(failed.Error()).NotTo(ContainSubstring("PRIVATE_"))
		if boundary == "reverse rename" {
			publicationFaultTyped(root, failed)
		}
		if boundary == "post-unlink parent sync" {
			_, err := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
			Expect(os.IsNotExist(err)).To(BeTrue(), "already unlinked evidence cannot be recreated")
		} else {
			publicationFaultPreserved(root, pending)
		}
		interruptedFaultProtectedPreserved(root, protected)
		priorActiveRenames := activeRenames
		Expect(operation.Complete(context.Background())).To(Succeed(), "same newly granted handle retries only its retained candidate and selected direction")
		Expect(activeRenames).To(Equal(priorActiveRenames), "retry must not manufacture another target inode")
		desired := publication.Replacement
		if direction == "reverse" {
			desired = publicationFaultBytes(root, ".factory/backups/"+request.MigrationID+"/files/"+request.Path)
		}
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(desired))
		Expect(operation.Close(context.Background())).To(Succeed())
		recoveryClosed(*files)
		interruptedGuardReleased(root)
		interruptedFaultProtectedPreserved(root, protected)
	}, Entry("actual known-image Sync then EIO", "known image sync"), Entry("actual terminal record rename then EIO", "terminal record rename"), Entry("actual terminal record directory Sync then EIO", "terminal record parent sync"), Entry("actual reverse target rename then EIO", "reverse rename"), Entry("actual pending descriptor close then EIO", "pending close"), Entry("actual pending unlink and parent Sync then EIO", "post-unlink parent sync"))
})

var _ = Describe("Interrupted recovery read-only operational faults", func() {
	// per docs/adr/0096-interrupted-publication-recovery.md:70
	// per docs/adr/0096-interrupted-publication-recovery.md:94
	// per docs/adr/0096-interrupted-publication-recovery.md:95
	// per docs/adr/0096-interrupted-publication-recovery.md:216
	DescribeTable("performs an actual pinned read then preserves operational EIO and closes every observation", func(target string) {
		root, _, request, _ := interruptedFaultFixture(true, "forward")
		Expect(os.WriteFile(filepath.Join(root, ".factory/budget.json"), []byte(`{"schema":1,"runs":[],"note":"PRIVATE_PRESERVED_HISTORY"}`), 0600)).To(Succeed())
		interruptedFaultProposal(root, request)
		relative := request.Path
		switch target {
		case "record":
			relative = ".factory/backups/.publications/" + request.MigrationID + ".json"
		case "saved":
			relative = ".factory/backups/" + request.MigrationID + "/files/" + request.Path
		case "budget":
			relative = ".factory/budget.json"
		}
		controls, files := publicationFaultControls()
		triggered := false
		controls.observe.read = func(file *os.File, data []byte) (int, error) {
			n, err := file.Read(data)
			if err == nil && n > 0 && !triggered && interruptedSame(file, filepath.Join(root, relative)) {
				triggered = true
				return n, unix.EIO
			}
			return n, err
		}
		before := interruptedFaultTree(root)
		proposal, err := proposeInterruptedRecovery(context.Background(), root, request, controls.observe)
		Expect(triggered).To(BeTrue(), "actual requested existing-file read precedes the injected EIO")
		Expect(err).To(HaveOccurred())
		Expect(ErrorStatus(err)).To(Equal(1))
		Expect(proposal.ProposalDigest).To(BeEmpty())
		Expect(err.Error()).NotTo(ContainSubstring(root))
		Expect(err.Error()).NotTo(ContainSubstring("PRIVATE_"))
		recoveryClosed(*files)
		interruptedFaultPreserved(root, before)
	}, Entry("actual current read then error", "current"), Entry("actual selected journal read then error", "record"), Entry("actual saved original read then error", "saved"), Entry("actual compatible budget history read then error", "budget"))
})

// Child witness executes production private collaborators in the ordinary test
// binary. It pauses only after the specified actual operation; parent kills and
// reaps it. No exported/test execution helper is shipped.
// per docs/adr/0096-interrupted-publication-recovery.md:245
func interruptedCrashWitness() {
	GinkgoHelper()
	root := os.Getenv("FACTORY_R22_CRASH_ROOT")
	var publication PublicationRequest
	Expect(json.Unmarshal([]byte(os.Getenv("FACTORY_R22_CRASH_REQUEST")), &publication)).To(Succeed())
	environment := map[string]string{"PATH": os.Getenv("PATH"), "HOME": root}
	point := os.Getenv("FACTORY_R22_CRASH_POINT")
	ready := func() {
		Expect(os.WriteFile(os.Getenv("FACTORY_R22_CRASH_READY"), []byte("ready"), 0600)).To(Succeed()) // #nosec G703 -- ready path is created only by the evaluator parent in its private temporary directory.
		select {}
	}
	controls := publicationOps{}
	renames := 0
	controls.rename = func(ctx context.Context, stage *filepublish.Stage, name string) error {
		active := name == filepath.Base(publication.Path)
		if active {
			renames++
		}
		target := point == "forward_before" || point == "forward_after"
		if point == "reverse_before" || point == "reverse_after" {
			target = renames == 2 && active
		}
		if point == "recovery_before" || point == "recovery_after" {
			target = active
		}
		target = target && active
		if target && strings.HasSuffix(point, "_before") {
			ready()
		}
		err := stage.Publish(ctx, name)
		if err == nil && target && strings.HasSuffix(point, "_after") {
			ready()
		}
		return err
	}
	controls.storage.syncDirectory = func(_ context.Context, file *os.File) error {
		err := file.Sync()
		if err != nil || point != "forward_terminal" && point != "reverse_terminal" || !interruptedSame(file, filepath.Join(root, ".factory/backups/.publications")) {
			return err
		}
		data, err := os.ReadFile(filepath.Join(root, ".factory/backups/.publications", publication.MigrationID+".json")) // #nosec G703 -- root/request supplied only by evaluator parent; exact fixed publication namespace inside the private fixture.
		if os.IsNotExist(err) {
			return nil //nolint:nilerr // Initial namespace sync legitimately precedes journal-slot creation.
		}
		if err != nil {
			return err
		}
		var record map[string]any
		if err := json.Unmarshal(data, &record); err != nil {
			return err
		}
		phase := "forward_completed"
		if point == "reverse_terminal" {
			phase = "restored"
		}
		if record["phase"] == phase {
			ready()
		}
		return nil
	}
	if strings.HasPrefix(point, "recovery_") {
		request := interruptedFaultRequest(root, publication, "reverse")
		proposal := interruptedFaultProposal(root, request)
		operation, err := beginInterruptedRecovery(context.Background(), root, request, proposal.ProposalDigest, environment, controls)
		Expect(err).NotTo(HaveOccurred())
		Expect(operation.Complete(context.Background())).To(Succeed())
	} else {
		operation, err := beginDurablePublication(context.Background(), root, publication, environment, controls)
		Expect(err).NotTo(HaveOccurred())
		Expect(operation.Apply(context.Background())).To(Succeed())
		if point == "forward_terminal" {
			Expect(operation.Finish(context.Background())).To(Succeed())
		}
		if strings.HasPrefix(point, "reverse_") {
			Expect(operation.Restore(context.Background())).To(Succeed())
		}
	}
	Fail("specified crash boundary was not reached")
}
func interruptedCrashStart(root string, publication PublicationRequest, point string) (*exec.Cmd, *bytes.Buffer) {
	GinkgoHelper()
	ready := filepath.Join(GinkgoT().TempDir(), "actual-operation-ready")
	encoded, err := json.Marshal(publication)
	Expect(err).NotTo(HaveOccurred())
	binary, err := os.Executable()
	Expect(err).NotTo(HaveOccurred())
	command := exec.Command(binary, "-test.run=^TestAssessment$", "-ginkgo.focus=Interrupted recovery crash child witness", "-ginkgo.no-color") // #nosec G204 -- owned current test binary, fixed test arguments and private fixture environment.
	command.Env = append(os.Environ(), "FACTORY_R22_CRASH_ROOT="+root, "FACTORY_R22_CRASH_REQUEST="+string(encoded), "FACTORY_R22_CRASH_POINT="+point, "FACTORY_R22_CRASH_READY="+ready)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	output := &bytes.Buffer{}
	command.Stdout, command.Stderr = output, output
	Expect(command.Start()).To(Succeed())
	waited := false
	DeferCleanup(func() {
		if !waited {
			_ = unix.Kill(-command.Process.Pid, unix.SIGKILL)
			_ = command.Wait()
			_, _ = GinkgoWriter.Write(output.Bytes())
		}
	})
	// The ready file is written only inside the exact native-operation witness.
	Eventually(func() bool { _, err := os.Lstat(ready); return err == nil }, 10*time.Second, 10*time.Millisecond).Should(BeTrue(), "actual child must acknowledge the requested %s boundary", point)
	// Parent owns final wait through the returned closure, tracked on the command.
	DeferCleanup(func() {
		if command.ProcessState != nil {
			waited = true
		}
	})
	return command, output
}
func interruptedCrashKill(command *exec.Cmd) {
	GinkgoHelper()
	Expect(command.Process.Kill()).To(Succeed())
	err := command.Wait()
	var exited *exec.ExitError
	Expect(errors.As(err, &exited)).To(BeTrue())
	Expect(exited.ProcessState.Sys().(syscall.WaitStatus).Signal()).To(Equal(syscall.SIGKILL))
}

var _ = Describe("Interrupted recovery crash child witness", func() {
	// per docs/adr/0096-interrupted-publication-recovery.md:245
	It("pauses at an actual requested publication operation", func() {
		if os.Getenv("FACTORY_R22_CRASH_ROOT") == "" {
			Skip("evaluator subprocess witness only")
		}
		interruptedCrashWitness()
	})
})
var _ = Describe("Interrupted recovery real process interruption", func() {
	// per docs/adr/0096-interrupted-publication-recovery.md:168
	// per docs/adr/0096-interrupted-publication-recovery.md:176
	// per docs/adr/0096-interrupted-publication-recovery.md:179
	// per docs/adr/0096-interrupted-publication-recovery.md:186
	// per docs/adr/0096-interrupted-publication-recovery.md:245
	// per docs/adr/0096-interrupted-publication-recovery.md:250
	DescribeTable("qualifies fresh resolution after acknowledged SIGKILL across actual rename and terminal boundaries", func(point, phase, direction, image, terminal string) {
		root, publication, environment := durablePublicationOwnedFixture()
		child, _ := interruptedCrashStart(root, publication, point)
		interruptedCrashKill(child)
		const orphan = "scripts/.factory-publication-EVALUATORORPHAN"
		Expect(os.WriteFile(filepath.Join(root, orphan), bytes.Clone(publication.Replacement), 0600)).To(Succeed())
		orphanState := interruptedFaultTree(root)[orphan]
		orphanProtected := map[string]interruptedFile{orphan: orphanState}
		pending := publicationFaultPending(root)
		before := interruptedFaultTree(root)
		data := publicationFaultBytes(root, ".factory/backups/.publications/"+publication.MigrationID+".json")
		var record map[string]any
		Expect(json.Unmarshal(data, &record)).To(Succeed())
		Expect(record["phase"]).To(Equal(phase))
		request := interruptedFaultRequest(root, publication, direction)
		proposal := interruptedFaultProposal(root, request)
		Expect(proposal.CurrentImage).To(Equal(image))
		interruptedFaultPreserved(root, before)
		publicationFaultPreserved(root, pending)
		protected := interruptedFaultProtected(root, request.MigrationID)
		current, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		operation := interruptedFaultBegin(root, request, environment, publicationOps{})
		Expect(operation.Complete(context.Background())).To(Succeed())
		Expect(operation.Close(context.Background())).To(Succeed())
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		if image == "before" || direction == "forward" {
			Expect(os.SameFile(current, after)).To(BeTrue(), "already desired image needs no redundant rename")
		}
		inventory, err := InspectPublications(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		Expect(inventory.Status()).To(BeZero())
		Expect(inventory.Records).To(HaveLen(1))
		Expect(inventory.Records[0].Phase).To(Equal(terminal))
		Expect(inventory.Applicable).To(BeFalse())
		interruptedFaultProtectedPreserved(root, protected)
		interruptedFaultProtectedPreserved(root, orphanProtected)
		interruptedGuardReleased(root)
	}, Entry("before actual forward rename", "forward_before", "forward_prepared", "reverse", "before", "aborted"), Entry("after actual forward rename before journal phase update", "forward_after", "forward_prepared", "reverse", "after", "restored"), Entry("before actual reverse rename", "reverse_before", "reverse_prepared", "reverse", "after", "restored"), Entry("after actual reverse rename before terminal record", "reverse_after", "reverse_prepared", "reverse", "before", "restored"), Entry("forward terminal record durable before pending removal", "forward_terminal", "forward_completed", "forward", "after", "forward_completed"), Entry("reverse terminal record durable before pending removal", "reverse_terminal", "restored", "reverse", "before", "restored"))

	// per docs/adr/0096-interrupted-publication-recovery.md:168
	// per docs/adr/0096-interrupted-publication-recovery.md:198
	// per docs/adr/0096-interrupted-publication-recovery.md:207
	// per docs/adr/0096-interrupted-publication-recovery.md:245
	It("requalifies another real interruption after refreshing renamed forward-candidate metadata for reverse preparation", func() {
		root, publication, environment := durablePublicationOwnedFixture()
		first, _ := interruptedCrashStart(root, publication, "forward_after")
		interruptedCrashKill(first)
		second, _ := interruptedCrashStart(root, publication, "recovery_before")
		interruptedCrashKill(second)
		recordBytes := publicationFaultBytes(root, ".factory/backups/.publications/"+publication.MigrationID+".json")
		var record map[string]any
		Expect(json.Unmarshal(recordBytes, &record)).To(Succeed())
		Expect(record["phase"]).To(Equal("reverse_prepared"))
		request := interruptedFaultRequest(root, publication, "reverse")
		proposal := interruptedFaultProposal(root, request)
		Expect(proposal.CurrentImage).To(Equal("after"))
		protected := interruptedFaultProtected(root, request.MigrationID)
		operation := interruptedFaultBegin(root, request, environment, publicationOps{})
		Expect(operation.Complete(context.Background())).To(Succeed())
		Expect(operation.Close(context.Background())).To(Succeed())
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(publicationFaultBytes(root, ".factory/backups/"+request.MigrationID+"/files/"+request.Path)))
		interruptedFaultProtectedPreserved(root, protected)
		interruptedGuardReleased(root)
	})
})

var _ = Describe("Interrupted recovery qualified Git refusal status", func() {
	// per docs/adr/0096-interrupted-publication-recovery.md:129
	// per docs/adr/0096-interrupted-publication-recovery.md:224
	// per docs/adr/0096-interrupted-publication-recovery.md:227
	DescribeTable("preserves the existing pending barrier and safe refusal status after actual Git qualification", func(change string) {
		root, _, request, environment := interruptedFaultFixture(true, "forward")
		valid := interruptedFaultBegin(root, request, environment, publicationOps{})
		Expect(ErrorStatus(valid.Close(context.Background()))).To(Equal(1), "paired valid existing-Git grant is explicitly closed without completing its work")
		if change == "exact exclusion" {
			Expect(os.WriteFile(filepath.Join(root, ".git/info/exclude"), []byte("# evaluator retains operator comments\n"), 0600)).To(Succeed())
		} else {
			creationGit(root, "add", "-f", "--", ".factory/backups/.publications/"+request.MigrationID+".json")
		}
		proposal := interruptedFaultProposal(root, request)
		before := interruptedFaultTree(root)
		pending := publicationFaultPending(root)
		controls, files := interruptedCapture(publicationOps{})
		queries := 0
		controls.query = func(ctx context.Context, directory string, argv []string, values map[string]string, allowance time.Duration) (native.CommandResult, error) {
			result, err := native.ExecuteCommand(ctx, directory, argv, values, allowance)
			queries++
			if err == nil {
				Expect(result.ProcessPID).To(BeNumerically(">", 0))
				Expect(result.ExitConfirmed).To(BeTrue())
				Expect(result.OwnershipUnconfirmed).To(BeFalse())
			}
			return result, err
		}
		operation, err := beginInterruptedRecovery(context.Background(), root, request, proposal.ProposalDigest, environment, controls)
		Expect(operation).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(queries).To(BeNumerically(">", 0), "actual supervised Git query must be reached after the eligible proposal and guard")
		interruptedFaultPreserved(root, before)
		publicationFaultPreserved(root, pending)
		recoveryClosed(*files)
		interruptedGuardReleased(root)
		Expect(err.Error()).NotTo(ContainSubstring(root))
		Expect(err.Error()).NotTo(ContainSubstring("PRIVATE_"))
		Expect(ErrorStatus(err)).To(Equal(2), "a newly refused grant cannot turn already-existing pending into operational cleanup uncertainty")
	}, Entry("missing exact durable backup exclusion", "exact exclusion"), Entry("actual tracked selected record", "tracked record"))
})

var _ = Describe("Interrupted recovery qualified metadata error status", func() {
	// per docs/adr/0096-interrupted-publication-recovery.md:231
	// per docs/adr/0096-interrupted-publication-recovery.md:232
	// per docs/adr/0096-interrupted-publication-recovery.md:234
	DescribeTable("distinguishes actual directory observation failures from metadata conflicts", func(target, operation string) {
		root, _, request, environment := interruptedFaultFixture(true, "reverse")
		eligible := interruptedFaultBegin(root, request, environment, publicationOps{})
		Expect(eligible.Close(context.Background())).To(HaveOccurred())
		proposal := interruptedFaultProposal(root, request)
		before := interruptedFaultTree(root)
		pending := publicationFaultPending(root)
		controls, files := interruptedCapture(publicationOps{})
		fired := false
		opened := []*os.Root{}
		controls.directoryStat = func(_ context.Context, directory *os.Root, name string) (os.FileInfo, error) {
			opened = append(opened, directory)
			info, err := directory.Stat(name)
			if err == nil && operation == "directory" && !fired {
				wanted, statErr := os.Stat(filepath.Join(root, target))
				Expect(statErr).NotTo(HaveOccurred())
				if os.SameFile(info, wanted) {
					fired = true
					return info, unix.EIO
				}
			}
			return info, err
		}
		controls.retainedStat = func(_ context.Context, file *os.File) (os.FileInfo, error) {
			info, err := file.Stat()
			if err == nil && operation == "retained" && !fired && interruptedSame(file, filepath.Join(root, target)) {
				fired = true
				return info, unix.EIO
			}
			return info, err
		}
		grant, err := beginInterruptedRecovery(context.Background(), root, request, proposal.ProposalDigest, environment, controls)
		Expect(grant).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(fired).To(BeTrue(), "the existing native Stat succeeds before its collaborator reports EIO")
		interruptedFaultPreserved(root, before)
		publicationFaultPreserved(root, pending)
		recoveryClosed(*files)
		Expect(opened).NotTo(BeEmpty())
		for _, directory := range opened {
			_, statErr := directory.Stat(".")
			Expect(errors.Is(statErr, os.ErrClosed)).To(BeTrue(), "every real confined root opened before refusal is closed")
		}
		interruptedGuardReleased(root)
		Expect(err.Error()).NotTo(ContainSubstring(root))
		Expect(ErrorStatus(err)).To(Equal(1), "a reported native observation failure is operational, not an identity conflict")
	}, Entry("active directory root Stat", "scripts", "directory"), Entry("active retained directory Stat", "scripts", "retained"), Entry("record directory root Stat", ".factory/backups/.publications", "directory"), Entry("record retained directory Stat", ".factory/backups/.publications", "retained"))
})

var _ = Describe("Interrupted recovery qualified named observation error status", func() {
	// per docs/adr/0096-interrupted-publication-recovery.md:73
	// per docs/adr/0096-interrupted-publication-recovery.md:215
	// per docs/adr/0096-interrupted-publication-recovery.md:216
	DescribeTable("preserves native observation error classification during proposal and grant", func(method, target string) {
		root, _, request, environment := interruptedFaultFixture(true, "reverse")
		eligible := interruptedFaultBegin(root, request, environment, publicationOps{})
		Expect(eligible.Close(context.Background())).To(HaveOccurred())
		proposal := interruptedFaultProposal(root, request)
		before := interruptedFaultTree(root)
		pending := publicationFaultPending(root)
		controls, files := interruptedCapture(publicationOps{})
		fired := false
		observe := func(parent *os.File, name string) (unix.Stat_t, error) {
			seen := false
			for _, file := range *files {
				seen = seen || file == parent
			}
			if !seen {
				*files = append(*files, parent)
			}
			info, err := named(parent, name)
			if err == nil && !fired && name == filepath.Base(target) && interruptedSame(parent, filepath.Join(root, filepath.Dir(target))) {
				fired = true
				return info, unix.EIO
			}
			return info, err
		}
		controls.observe.named = observe
		controls.storage.named = observe
		var err error
		if method == "propose" {
			var observed InterruptedRecoveryProposal
			observed, err = proposeInterruptedRecovery(context.Background(), root, request, controls.observe)
			Expect(observed.ProposalDigest).To(BeEmpty())
		} else {
			var grant *InterruptedRecovery
			grant, err = beginInterruptedRecovery(context.Background(), root, request, proposal.ProposalDigest, environment, controls)
			Expect(grant).To(BeNil())
		}
		Expect(err).To(HaveOccurred())
		Expect(fired).To(BeTrue(), "native confined named observation must succeed before the single injected EIO")
		interruptedFaultPreserved(root, before)
		publicationFaultPreserved(root, pending)
		recoveryClosed(*files)
		interruptedGuardReleased(root)
		Expect(err.Error()).NotTo(ContainSubstring(root))
		Expect(ErrorStatus(err)).To(Equal(1), "real observation errors cannot become missing/unsafe-evidence refusals")
	}, Entry("proposal factory directory", "propose", ".factory"), Entry("proposal pending marker", "propose", ".factory/runtime-publication.pending"), Entry("proposal selected current", "propose", "scripts/factory-budget.sh"), Entry("grant factory directory", "begin", ".factory"), Entry("grant pending marker", "begin", ".factory/runtime-publication.pending"), Entry("grant selected current", "begin", "scripts/factory-budget.sh"))
})

func interruptedState(root, kind string, status string, pid, exit any, uncertain bool) {
	GinkgoHelper()
	var row map[string]any
	if kind == "budget" {
		row = map[string]any{"id": "run", "session": "session", "task": "task", "harness": "codex", "role": "implementer", "status": status, "outcome": "completed", "elapsed_seconds": 12.5, "reserved_seconds": 15, "attempt": 1, "owner_pid": 424242, "process_pid": pid, "exit_code": exit, "complete": true, "model": "caller_model", "estimated_usd": 0.2, "tokens": nil, "warnings": []any{}, "started_at": "before", "ended_at": "after", "source": "native", "operator_extension": map[string]any{"consumed": 9}}
		if status == "active" {
			row["outcome"], row["ended_at"], row["complete"] = "running", nil, false
		}
	} else {
		row = map[string]any{"session": "session", "task": "task", "harness": "codex", "mode": "manual", "status": status, "outcome": "manual_passed", "phase": "terminal", "owner_pid": 424242, "process_pid": pid, "uncertain": uncertain, "elapsed_seconds": 12.5, "reserved_seconds": 15, "attempts": 3, "no_progress": 0, "policy": "known", "prompt": "known", "started_at": "before", "stop_reason": "done", "next_action": "inspect", "evidence": []any{}, "budget_runs": []any{"consumed-budget"}, "snapshot": map[string]any{"head": "known", "source": "known", "safety": "known"}, "baseline": map[string]any{"head": "known", "source": "known", "safety": "known"}, "operator_extension": map[string]any{"consumed": 9}}
	}
	data, err := json.Marshal(map[string]any{"schema": 1, "runs": []any{row}, "operator_extension": "preserve"})
	Expect(err).NotTo(HaveOccurred())
	if kind == "budget" {
		_, err = budget.ParseHistory(context.Background(), bytes.NewReader(data))
	} else {
		_, err = loop.ParseHistory(context.Background(), bytes.NewReader(data))
	}
	Expect(err).NotTo(HaveOccurred(), "independently validate that state is schema-readable before testing recovery composition")
	name := "budget.json"
	if kind == "loop" {
		name = "loops.json"
	}
	Expect(os.WriteFile(filepath.Join(root, ".factory", name), data, 0600)).To(Succeed())
}

func interruptedCompetingRecord(root string, request InterruptedRecoveryRequest, terminal bool) {
	GinkgoHelper()
	const other = "other-saved-set"
	selected := filepath.Join(root, ".factory/backups", request.MigrationID)
	set := filepath.Join(root, ".factory/backups", other)
	Expect(os.MkdirAll(filepath.Join(set, "files/scripts"), 0700)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(set, "files/scripts/factory-budget.sh"), publicationFaultBytes(selected, "files/scripts/factory-budget.sh"), 0600)).To(Succeed())
	var manifest map[string]any
	Expect(json.Unmarshal(publicationFaultBytes(selected, "manifest.json"), &manifest)).To(Succeed())
	manifest["migration_id"] = other
	data, err := json.Marshal(manifest)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.WriteFile(filepath.Join(set, "manifest.json"), data, 0600)).To(Succeed())
	inventory, err := InspectRecovery(context.Background(), root)
	Expect(err).NotTo(HaveOccurred())
	Expect(inventory.Status()).To(BeZero(), "both selected and competing saved sets are independently integrity-checked")
	var record map[string]any
	Expect(json.Unmarshal(publicationFaultBytes(root, ".factory/backups/.publications/"+request.MigrationID+".json"), &record)).To(Succeed())
	file, err := os.Open(set)
	Expect(err).NotTo(HaveOccurred())
	stat, err := descriptor(file)
	Expect(err).NotTo(HaveOccurred())
	Expect(file.Close()).To(Succeed())
	identity := assetIdentity(stat)
	record["migration_id"], record["operation_id"] = other, "another-operation"
	record["recovery_identity"] = RootIdentity{Device: identity.Device, Inode: identity.Inode}
	if terminal {
		record["phase"], record["direction"], record["outcome"] = "forward_completed", "forward", "forward_completed"
	}
	data, err = json.Marshal(record)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.WriteFile(filepath.Join(root, ".factory/backups/.publications", other+".json"), data, 0600)).To(Succeed())
	publications, err := InspectPublications(context.Background(), root)
	Expect(err).NotTo(HaveOccurred())
	Expect(publications.Records).To(HaveLen(2))
	for _, row := range publications.Records {
		Expect(row.Classification).To(Equal("record_checked"), "a second malformed receipt would not isolate competing nonterminal policy")
	}
}

var _ = Describe("Interrupted recovery actual G2 policy composition", func() {
	// per docs/adr/0096-interrupted-publication-recovery.md:145
	// per docs/adr/0096-interrupted-publication-recovery.md:146
	// per docs/adr/0096-interrupted-publication-recovery.md:148
	// per docs/adr/0096-interrupted-publication-recovery.md:156
	// per docs/adr/0096-interrupted-publication-recovery.md:176
	// per docs/adr/0096-interrupted-publication-recovery.md:183
	DescribeTable("refuses independently readable unresolved or held evidence without disturbing it", func(change string) {
		root, _, request, _ := interruptedFaultFixture(change != "before forward", "reverse")
		interruptedFaultProposal(root, request)
		switch change {
		case "budget active":
			interruptedState(root, "budget", "active", nil, nil, false)
		case "budget exit uncertain":
			interruptedState(root, "budget", "completed", 424243, nil, false)
		case "checkpoint uncertain":
			interruptedState(root, "loop", "completed", nil, nil, true)
		case "checkpoint retained PID":
			interruptedState(root, "loop", "completed", 424243, nil, false)
		case "held saved set":
			path := filepath.Join(root, ".factory/backups", request.MigrationID, "manifest.json")
			var manifest map[string]any
			Expect(json.Unmarshal(publicationFaultBytes(root, ".factory/backups/"+request.MigrationID+"/manifest.json"), &manifest)).To(Succeed())
			manifest["held"] = true
			data, err := json.Marshal(manifest)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(path, data, 0600)).To(Succeed())
			inventory, err := InspectRecovery(context.Background(), root)
			Expect(err).NotTo(HaveOccurred())
			Expect(inventory.Sets).To(HaveLen(1))
			Expect(inventory.Sets[0].Classification).To(Equal("integrity_checked"))
			Expect(inventory.Sets[0].Held).NotTo(BeNil())
			Expect(*inventory.Sets[0].Held).To(BeTrue())
		case "other nonterminal":
			interruptedCompetingRecord(root, request, false)
		case "before forward":
			request.Direction = "forward"
		}
		before := interruptedFaultTree(root)
		controls, files := interruptedCapture(publicationOps{})
		proposal, err := proposeInterruptedRecovery(context.Background(), root, request, controls.observe)
		Expect(err).To(HaveOccurred())
		Expect(proposal.ProposalDigest).To(BeEmpty())
		interruptedFaultPreserved(root, before)
		recoveryClosed(*files)
		interruptedGuardReleased(root)
		Expect(ErrorStatus(err)).To(Equal(2))
	}, Entry("active budget with no PID", "budget active"), Entry("completed budget PID without recorded exit", "budget exit uncertain"), Entry("uncertain terminal checkpoint", "checkpoint uncertain"), Entry("terminal checkpoint retaining process PID", "checkpoint retained PID"), Entry("integrity checked held saved set", "held saved set"), Entry("independently checked competing nonterminal record", "other nonterminal"), Entry("prepared original before-image forbids forward apply", "before forward"))

	// per docs/adr/0096-interrupted-publication-recovery.md:147
	// per docs/adr/0096-interrupted-publication-recovery.md:149
	// per docs/adr/0096-interrupted-publication-recovery.md:151
	// per docs/adr/0096-interrupted-publication-recovery.md:158
	DescribeTable("resolves the selected publication while preserving harmless historical evidence", func(kind string) {
		root, _, request, environment := interruptedFaultFixture(true, "forward")
		interruptedFaultProposal(root, request)
		if kind == "history" {
			interruptedState(root, "budget", "completed", 424243, 7, false)
			interruptedState(root, "loop", "completed", nil, nil, false)
		} else {
			interruptedCompetingRecord(root, request, true)
		}
		protected := interruptedFaultProtected(root, request.MigrationID)
		if kind == "terminal record" {
			for path, state := range interruptedFaultTree(root) {
				if path == ".factory/backups/other-saved-set" || strings.HasPrefix(path, ".factory/backups/other-saved-set/") {
					protected[path] = state
				}
			}
		}
		var record []byte
		var recordInfo os.FileInfo
		if kind == "terminal record" {
			record = publicationFaultBytes(root, ".factory/backups/.publications/other-saved-set.json")
			var err error
			recordInfo, err = os.Lstat(filepath.Join(root, ".factory/backups/.publications/other-saved-set.json"))
			Expect(err).NotTo(HaveOccurred())
		}
		current, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		grant := interruptedFaultBegin(root, request, environment, publicationOps{})
		Expect(grant.Complete(context.Background())).To(Succeed())
		Expect(grant.Close(context.Background())).To(Succeed())
		interruptedFaultProtectedPreserved(root, protected)
		actual, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(current, actual)).To(BeTrue())
		if recordInfo != nil {
			actual, err := os.Lstat(filepath.Join(root, ".factory/backups/.publications/other-saved-set.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(os.SameFile(recordInfo, actual)).To(BeTrue())
			Expect(publicationFaultBytes(root, ".factory/backups/.publications/other-saved-set.json")).To(Equal(record))
		}
	}, Entry("completed budget PID with recorded exit and historical loop owner", "history"), Entry("independently checked inert other terminal record", "terminal record"))
})

var _ = Describe("Interrupted recovery checked cancellation closure", func() {
	// per docs/adr/0096-interrupted-publication-recovery.md:213
	// per docs/adr/0096-interrupted-publication-recovery.md:215
	// per docs/adr/0096-interrupted-publication-recovery.md:216
	It("closes actual granted resources under cancellation while preserving unresolved evidence for new consent", func() {
		root, _, request, environment := interruptedFaultFixture(true, "reverse")
		proposal := interruptedFaultProposal(root, request)
		controls, files := interruptedCapture(publicationOps{})
		grant, err := beginInterruptedRecovery(context.Background(), root, request, proposal.ProposalDigest, environment, controls)
		Expect(err).NotTo(HaveOccurred())
		Expect(grant).NotTo(BeNil())
		before := interruptedFaultTree(root)
		pending := publicationFaultPending(root)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err = grant.Close(ctx)
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		interruptedFaultPreserved(root, before)
		publicationFaultPreserved(root, pending)
		recoveryClosed(*files)
		interruptedGuardReleased(root)
		Expect(ErrorStatus(err)).To(Equal(1))
		Expect(ErrorStatus(grant.Complete(context.Background()))).To(Equal(2))
		fresh := interruptedFaultProposal(root, request)
		Expect(fresh.ProposalDigest).To(Equal(proposal.ProposalDigest), "closure alone cannot change durable evidence or confer resolution")
	})
})

var _ = Describe("Interrupted recovery qualified constructor late activity status", func() {
	// per docs/adr/0096-interrupted-publication-recovery.md:276
	// per docs/adr/0096-interrupted-publication-recovery.md:278
	// per docs/adr/0096-interrupted-publication-recovery.md:280
	DescribeTable("preserves guard conflicts while retaining operational precedence for actual reported closure failures", func(add, reported bool) {
		root, _, request, environment := interruptedFaultFixture(true, "forward")
		proposal := interruptedFaultProposal(root, request)
		controls, files := interruptedCapture(publicationOps{})
		underlying := controls.storage.open
		reached, held, closed := false, false, false
		var before map[string]interruptedFile
		pending := publicationFaultPending(root)
		controls.storage.open = func(ctx context.Context, parent *os.File, name string, flags int, mode uint32) (*os.File, error) {
			file, err := underlying(ctx, parent, name, flags, mode)
			if err == nil && name == "runtime-transition.lock" && !reached {
				reached = true
				lock, err := os.OpenFile(filepath.Join(root, ".factory/runtime-transition.lock"), os.O_RDWR, 0)
				Expect(err).NotTo(HaveOccurred())
				err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB)
				held = errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)
				Expect(held).To(BeTrue(), "actual observer open must occur while the recovery-only permanent EX lock is held")
				Expect(lock.Close()).To(Succeed())
				if add {
					Expect(os.WriteFile(filepath.Join(root, ".factory/runtime-activity/observed-late-work"), nil, 0600)).To(Succeed())
				}
				before = interruptedFaultTree(root)
			}
			return file, err
		}
		controls.guardClose = func(ctx context.Context, guard *transition.Guard, clean bool) error {
			err := guard.Close(ctx, clean)
			Expect(closed).To(BeFalse(), "the real guard is closed exactly once")
			closed = true
			if add {
				Expect(err).To(HaveOccurred(), "actual preserved activity invalidates the retained guard")
				var conflict *transition.RecoveryError
				Expect(errors.As(err, &conflict)).To(BeTrue())
				Expect(conflict.Conflict()).To(BeTrue(), "native guard closure reports the observed metadata conflict, not a failed descriptor syscall")
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
			interruptedGuardReleased(root)
			if reported {
				return errors.Join(err, unix.EIO)
			}
			return err
		}
		grant, err := beginInterruptedRecovery(context.Background(), root, request, proposal.ProposalDigest, environment, controls)
		var closeErr error
		if grant != nil {
			closeErr = grant.Close(context.Background())
		}
		Expect(reached).To(BeTrue())
		Expect(held).To(BeTrue())
		Expect(closed).To(BeTrue(), "native Guard.Close must run before one collaborator-reported error")
		recoveryClosed(*files)
		interruptedGuardReleased(root)
		interruptedFaultPreserved(root, before)
		publicationFaultPreserved(root, pending)
		entries, readErr := os.ReadDir(filepath.Join(root, ".factory/runtime-activity"))
		Expect(readErr).NotTo(HaveOccurred())
		if add {
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Name()).To(Equal("observed-late-work"))
			Expect(grant).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring(root))
			want := 2
			if reported {
				want = 1
			}
			Expect(ErrorStatus(err)).To(Equal(want), "successful native closure preserves conflict; real post-close reported EIO has operational precedence")
		} else {
			Expect(entries).To(BeEmpty())
			Expect(grant).NotTo(BeNil())
			Expect(err).NotTo(HaveOccurred())
			Expect(ErrorStatus(closeErr)).To(Equal(1), "returned unresolved capability keeps its normal incomplete Close diagnostic")
		}
	}, Entry("no late activity paired eligible grant", false, false), Entry("actual late activity and successful native resource closure", true, false), Entry("actual late activity then native closure and reported EIO", true, true))
})

var _ = Describe("Interrupted recovery qualified confined open conflict status", func() {
	// per docs/adr/0096-interrupted-publication-recovery.md:284
	// per docs/adr/0096-interrupted-publication-recovery.md:285
	// per docs/adr/0096-interrupted-publication-recovery.md:286
	DescribeTable("preserves evidence that disappears or becomes linked between native qualification and native confined open", func(kind, change string) {
		root, _, request, environment := interruptedFaultFixture(true, "forward")
		proposal := interruptedFaultProposal(root, request)
		controls, files := interruptedCapture(publicationOps{})
		underlying := controls.storage.open
		reached, held := false, false
		var before map[string]interruptedFile
		var actualErr error
		selected := filepath.Join(root, request.Path)
		if kind == "directory" {
			selected = filepath.Dir(selected)
		}
		detached := filepath.Join(GinkgoT().TempDir(), "evaluator-owned-prior")
		var detachedInfo os.FileInfo
		var detachedData []byte
		var detachedTree map[string]interruptedFile
		pending := publicationFaultPending(root)
		controls.storage.open = func(ctx context.Context, parent *os.File, name string, flags int, mode uint32) (*os.File, error) {
			target := name == filepath.Base(selected) && interruptedSame(parent, filepath.Dir(selected)) && !reached
			if target {
				reached = true
				Expect(flags & unix.O_NOFOLLOW).NotTo(BeZero())
				lock, err := os.OpenFile(filepath.Join(root, ".factory/runtime-transition.lock"), os.O_RDWR, 0)
				Expect(err).NotTo(HaveOccurred())
				err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB)
				held = errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)
				Expect(held).To(BeTrue())
				Expect(lock.Close()).To(Succeed())
				if change != "none" {
					Expect(os.Rename(selected, detached)).To(Succeed())
					detachedInfo, err = os.Lstat(detached)
					Expect(err).NotTo(HaveOccurred())
					if kind == "directory" {
						detachedTree = interruptedFaultTree(detached)
					} else {
						detachedData, err = os.ReadFile(detached)
						Expect(err).NotTo(HaveOccurred())
					}
					if change == "symlink" {
						Expect(os.Symlink(detached, selected)).To(Succeed())
					}
				}
				before = interruptedFaultTree(root)
			}
			file, err := underlying(ctx, parent, name, flags, mode)
			if target {
				actualErr = err
			}
			return file, err
		}
		grant, err := beginInterruptedRecovery(context.Background(), root, request, proposal.ProposalDigest, environment, controls)
		if grant != nil {
			Expect(grant.Close(context.Background())).To(HaveOccurred())
		}
		Expect(reached).To(BeTrue(), "native named qualification reaches the existing-file/directory confined-open collaborator")
		Expect(held).To(BeTrue())
		recoveryClosed(*files)
		interruptedGuardReleased(root)
		interruptedFaultPreserved(root, before)
		publicationFaultPreserved(root, pending)
		if detachedInfo != nil {
			actual, statErr := os.Lstat(detached)
			Expect(statErr).NotTo(HaveOccurred())
			Expect(os.SameFile(detachedInfo, actual)).To(BeTrue())
			Expect(actual.Mode()).To(Equal(detachedInfo.Mode()))
			Expect(actual.ModTime()).To(Equal(detachedInfo.ModTime()))
			if kind == "directory" {
				interruptedFaultPreserved(detached, detachedTree)
			} else {
				data, readErr := os.ReadFile(detached)
				Expect(readErr).NotTo(HaveOccurred())
				Expect(data).To(Equal(detachedData))
			}
		}
		if change == "none" {
			Expect(actualErr).NotTo(HaveOccurred())
			Expect(grant).NotTo(BeNil())
			Expect(err).NotTo(HaveOccurred())
		} else {
			if change == "missing" {
				Expect(errors.Is(actualErr, unix.ENOENT)).To(BeTrue(), "actual Openat observes missing qualified evidence")
			} else {
				Expect(errors.Is(actualErr, unix.ELOOP) || kind == "directory" && errors.Is(actualErr, unix.ENOTDIR)).To(BeTrue(), "actual no-follow directory opens may report ENOTDIR rather than ELOOP")
				target, readErr := os.Readlink(selected)
				Expect(readErr).NotTo(HaveOccurred())
				Expect(target).To(Equal(detached))
			}
			Expect(grant).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring(root))
			Expect(ErrorStatus(err)).To(Equal(2), "real missing/unsafe evidence at Openat remains a preserved conflict")
		}
	}, Entry("unchanged actual open eligible grant", "file", "none"), Entry("actual selected file disappearance", "file", "missing"), Entry("actual selected file symlink before no-follow open", "file", "symlink"), Entry("actual catalog directory disappearance", "directory", "missing"), Entry("actual catalog directory symlink before no-follow open", "directory", "symlink"))
})
