package assessment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/filepublish"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/transition"
	"golang.org/x/sys/unix"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func durablePublicationQuotaFixture() (string, RecoveryRequest, map[string]string) {
	GinkgoHelper()
	root, _, request, environment := creationFixture()
	created, err := CreateRecovery(context.Background(), root, request, environment)
	Expect(err).NotTo(HaveOccurred())
	Expect(created.Result).To(Equal("created"))
	seed := filepath.Join(root, ".factory/backups", request.MigrationID)
	original, err := os.ReadFile(filepath.Join(seed, "files/scripts/factory-budget.sh"))
	Expect(err).NotTo(HaveOccurred())
	manifestBytes, err := os.ReadFile(filepath.Join(seed, "manifest.json"))
	Expect(err).NotTo(HaveOccurred())
	for i := range 63 {
		id := fmt.Sprintf("qualifier-set-%02d", i)
		set := filepath.Join(root, ".factory/backups", id)
		Expect(os.MkdirAll(filepath.Join(set, "files/scripts"), 0700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(set, "files/scripts/factory-budget.sh"), original, 0600)).To(Succeed()) // #nosec G703 -- fixed catalog leaf inside a test-created private recovery set; no external input chooses the destination.
		var manifest map[string]any
		Expect(json.Unmarshal(manifestBytes, &manifest)).To(Succeed())
		manifest["migration_id"] = id
		encoded, encodeErr := json.Marshal(manifest)
		Expect(encodeErr).NotTo(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(set, "manifest.json"), encoded, 0600)).To(Succeed())
	}
	baseline, err := InspectRecovery(context.Background(), root)
	Expect(err).NotTo(HaveOccurred())
	Expect(baseline.Status()).To(BeZero())
	Expect(baseline.Complete).To(BeTrue())
	Expect(baseline.SetCount).To(Equal(64), "all 64 physical sets are independently integrity-checked before adding reserved metadata")
	Expect(baseline.Sets).To(HaveLen(64))
	for _, set := range baseline.Sets {
		Expect(set.Classification).To(Equal("integrity_checked"))
	}
	Expect(os.Mkdir(filepath.Join(root, ".factory/backups/.publications"), 0700)).To(Succeed())
	return root, request, environment
}

var _ = Describe("Durable publication independent saved-set quota", func() {
	// per docs/adr/0095-durable-live-publication.md:88
	// per docs/adr/0095-durable-live-publication.md:91
	It("keeps 64 integrity-checked saved sets separate from valid private reserved metadata", func() {
		root, _, _ := durablePublicationQuotaFixture()
		inventory, err := InspectRecovery(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		Expect(inventory.Status()).To(BeZero())
		Expect(inventory.Complete).To(BeTrue())
		Expect(inventory.SetCount).To(Equal(64))
		Expect(inventory.Sets).To(HaveLen(64))
		for _, set := range inventory.Sets {
			Expect(set.Classification).To(Equal("integrity_checked"))
			Expect(set.Restorable).To(BeFalse())
			Expect(set.PruneAuthorized).To(BeFalse())
		}
		Expect(inventory.Restorable).To(BeFalse())
		Expect(inventory.PruneAuthorized).To(BeFalse())
	})

	// per docs/adr/0095-durable-live-publication.md:69
	// per docs/adr/0095-durable-live-publication.md:91
	// per docs/adr/0091-durable-local-recovery-creation.md:124
	It("permits exact existing-set reuse at the 64-set boundary without counting reserved metadata", func() {
		root, request, environment := durablePublicationQuotaFixture()
		manifest := filepath.Join(root, ".factory/backups", request.MigrationID, "manifest.json")
		before, err := os.ReadFile(manifest)
		Expect(err).NotTo(HaveOccurred())
		metadata, err := os.Lstat(filepath.Join(root, ".factory/backups/.publications"))
		Expect(err).NotTo(HaveOccurred())
		result, err := CreateRecovery(context.Background(), root, request, environment)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Result).To(Equal("already_present"))
		after, err := os.ReadFile(manifest)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		actual, err := os.Lstat(filepath.Join(root, ".factory/backups/.publications"))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(metadata, actual)).To(BeTrue())
		Expect(actual.Mode()).To(Equal(metadata.Mode()))
		entries, err := os.ReadDir(filepath.Join(root, ".factory/backups/.publications"))
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	})
})

func durablePublicationOwnedFixture() (string, PublicationRequest, map[string]string) {
	GinkgoHelper()
	root, _, recovery, environment := creationFixture()
	created, err := CreateRecovery(context.Background(), root, recovery, environment)
	Expect(err).NotTo(HaveOccurred())
	Expect(created.Result).To(Equal("created"))
	proposal, err := ProposeAdoption(context.Background(), root, recovery.Paths)
	Expect(err).NotTo(HaveOccurred())
	return root, PublicationRequest{MigrationID: recovery.MigrationID, Path: recovery.Paths[0], Confirmation: proposal.ProposalDigest, Replacement: []byte("PRIVATE_DURABLE_REPLACEMENT\n")}, environment
}

var _ = Describe("Durable publication actual query ownership", func() {
	// per docs/adr/0095-durable-live-publication.md:54
	// per docs/adr/0095-durable-live-publication.md:61
	// per docs/adr/0095-durable-live-publication.md:63
	DescribeTable("preserves the actual query PID and typed reported uncertainty including cancellation", func(cancelAfterExit bool) {
		root, request, environment := durablePublicationOwnedFixture()
		original := publicationFaultBytes(root, request.Path)
		before, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		controls, files := publicationFaultControls()
		pendingFileSynced, pendingParentSynced := false, false
		controls.storage.syncFile = func(_ context.Context, file *os.File) error {
			syncErr := file.Sync()
			if syncErr == nil && publicationSame(file, filepath.Join(root, ".factory/runtime-publication.pending")) {
				pendingFileSynced = true
			}
			return syncErr
		}
		controls.storage.syncDirectory = func(_ context.Context, file *os.File) error {
			syncErr := file.Sync()
			if syncErr == nil && pendingFileSynced && publicationSame(file, filepath.Join(root, ".factory")) {
				pendingParentSynced = true
			}
			return syncErr
		}
		operation := publicationFaultBegin(root, request, controls)
		pending := publicationFaultPending(root)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		pid, queried := 0, false
		query := func(ctx context.Context, directory string, argv []string, values map[string]string, allowance time.Duration) (native.CommandResult, error) {
			Expect(pendingFileSynced).To(BeTrue())
			Expect(pendingParentSynced).To(BeTrue(), "the real durable marker exists before the literal query starts")
			publicationFaultPreserved(root, pending)
			result, actualErr := native.ExecuteCommand(ctx, directory, argv, values, allowance)
			Expect(actualErr).NotTo(HaveOccurred())
			Expect(result.ProcessPID).To(BeNumerically(">", 0))
			Expect(result.ExitConfirmed).To(BeTrue())
			Expect(result.OwnershipUnconfirmed).To(BeFalse())
			Expect(result.ExitCode).NotTo(BeNil())
			Expect(*result.ExitCode).To(BeZero())
			pid, queried = result.ProcessPID, true
			// This collaborator reports uncertainty after actual confirmed execution;
			// it does not claim that a kernel process group survived.
			result.OwnershipUnconfirmed = true
			reported := error(&native.OwnershipError{ProcessPID: pid})
			if cancelAfterExit {
				cancel()
				reported = errors.Join(reported, ctx.Err())
			}
			return result, reported
		}
		_, primary := operation.state.w.localGitWith(ctx, root, environment, query)
		Expect(queried).To(BeTrue(), "the actual command completes before injected uncertainty")
		Expect(primary).To(HaveOccurred())
		Expect(ErrorStatus(primary)).To(Equal(1))
		Expect(primary.Error()).NotTo(ContainSubstring(root))
		Expect(primary.Error()).NotTo(ContainSubstring("PRIVATE_"))
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		Expect(after.Mode()).To(Equal(before.Mode()))
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(original))
		publicationFaultPreserved(root, pending)
		Expect(operation.Close(context.Background())).To(HaveOccurred(), "unresolved pending is never cleared by close")
		publicationFaultPreserved(root, pending)
		publicationObservationReleased(root, *files)
		if cancelAfterExit {
			Expect(errors.Is(primary, context.Canceled)).To(BeTrue())
		}
		var ownership *native.OwnershipError
		Expect(errors.As(primary, &ownership)).To(BeTrue(), "safe diagnostics must retain the original typed query uncertainty")
		Expect(ownership.ProcessPID).To(Equal(pid), "the returned PID is from the actual native execution")
	}, Entry("reported ownership after actual execution", false), Entry("reported ownership joined with parent cancellation after actual execution", true))
})

var _ = Describe("Durable publication independent record quota", func() {
	// per docs/adr/0095-durable-live-publication.md:91
	// per docs/adr/0095-durable-live-publication.md:70
	DescribeTable("admits record 64 but preserves all evidence and refuses record 65", func(existing int) {
		root, request, environment := durablePublicationOwnedFixture()
		ctx := context.Background()
		seed, err := BeginDurablePublication(ctx, root, request, environment)
		Expect(err).NotTo(HaveOccurred())
		Expect(seed.Restore(ctx)).To(Succeed())
		Expect(seed.Close(ctx)).To(Succeed())
		directory := filepath.Join(root, ".factory/backups/.publications")
		slot := filepath.Join(directory, request.MigrationID+".json")
		data, err := os.ReadFile(slot)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Remove(slot)).To(Succeed(), "the evaluator removes only its completed seed fixture")
		preserved := map[string][]byte{}
		identities := map[string]os.FileInfo{}
		for i := range existing {
			id := fmt.Sprintf("capacity-%02d", i)
			var record map[string]any
			Expect(json.Unmarshal(data, &record)).To(Succeed())
			record["migration_id"] = id
			encoded, encodeErr := json.Marshal(record)
			Expect(encodeErr).NotTo(HaveOccurred())
			name := id + ".json"
			Expect(os.WriteFile(filepath.Join(directory, name), encoded, 0600)).To(Succeed()) // #nosec G703 -- bounded generated names in a test-owned reserved directory.
			preserved[name] = encoded
			identities[name], err = os.Lstat(filepath.Join(directory, name))
			Expect(err).NotTo(HaveOccurred())
		}
		baseline, err := InspectPublications(ctx, root)
		Expect(err).NotTo(HaveOccurred())
		Expect(baseline.Status()).To(BeZero())
		Expect(baseline.RecordCount).To(Equal(existing))
		for _, record := range baseline.Records {
			Expect(record.Classification).To(Equal("record_checked"))
			Expect(record.Phase).To(Equal("aborted"))
		}
		original := publicationFaultBytes(root, request.Path)
		before, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		controls, files := publicationFaultControls()
		operation, primary := beginDurablePublication(ctx, root, request, environment, controls)
		if operation != nil {
			Expect(operation.Restore(ctx)).To(Succeed())
			Expect(operation.Close(ctx)).To(Succeed())
		}
		for name, bytes := range preserved {
			Expect(publicationFaultBytes(directory, name)).To(Equal(bytes))
			actual, statErr := os.Lstat(filepath.Join(directory, name))
			Expect(statErr).NotTo(HaveOccurred())
			Expect(os.SameFile(identities[name], actual)).To(BeTrue())
			Expect(actual.Mode()).To(Equal(identities[name].Mode()))
		}
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		Expect(after.Mode()).To(Equal(before.Mode()))
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(original))
		_, pendingErr := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(pendingErr)).To(BeTrue())
		publicationObservationReleased(root, *files)
		if existing == 63 {
			Expect(primary).NotTo(HaveOccurred())
			inventory, inspectErr := InspectPublications(ctx, root)
			Expect(inspectErr).NotTo(HaveOccurred())
			Expect(inventory.Status()).To(BeZero())
			Expect(inventory.RecordCount).To(Equal(64))
		} else {
			Expect(primary).To(HaveOccurred(), "a fresh transaction cannot create record 65")
			Expect(ErrorStatus(primary)).To(Equal(2))
			Expect(operation).To(BeNil())
			_, slotErr := os.Lstat(slot)
			Expect(os.IsNotExist(slotErr)).To(BeTrue())
		}
	}, Entry("63 valid generated records", 63), Entry("64 valid generated records", 64))
})

var _ = Describe("Durable publication inspection I/O precedence", func() {
	// per docs/adr/0095-durable-live-publication.md:44
	// per docs/adr/0095-durable-live-publication.md:93
	DescribeTable("preserves prior actual read failure while discarding changed observations", func(change, reportedIO bool) {
		root, request, environment := durablePublicationOwnedFixture()
		ctx := context.Background()
		operation, err := BeginDurablePublication(ctx, root, request, environment)
		Expect(err).NotTo(HaveOccurred())
		Expect(operation.Restore(ctx)).To(Succeed())
		Expect(operation.Close(ctx)).To(Succeed())
		slot := filepath.Join(root, ".factory/backups/.publications", request.MigrationID+".json")
		baseline, err := InspectPublications(ctx, root)
		Expect(err).NotTo(HaveOccurred())
		Expect(baseline.Status()).To(BeZero())
		Expect(baseline.RecordCount).To(Equal(1))
		before, err := os.Lstat(slot)
		Expect(err).NotTo(HaveOccurred())
		bytes := publicationFaultBytes(filepath.Dir(slot), filepath.Base(slot))
		files := []*os.File{}
		reached := false
		operations := ops{open: func(parent *os.File, name string, flags int) (*os.File, error) {
			file, openErr := openAt(parent, name, flags)
			if file != nil {
				files = append(files, file)
			}
			return file, openErr
		}, read: func(file *os.File, buffer []byte) (int, error) {
			n, readErr := file.Read(buffer)
			if !reached && n > 0 && publicationSame(file, slot) {
				reached = true
				if change {
					Expect(file.Chmod(0644)).To(Succeed())
				}
				if reportedIO {
					return n, errors.Join(readErr, unix.EIO)
				}
			}
			return n, readErr
		}}
		inventory, err := inspectPublications(ctx, root, operations)
		Expect(err).NotTo(HaveOccurred())
		Expect(reached).To(BeTrue(), "the real record read must occur before mutation or reported I/O")
		actual, err := os.Lstat(slot)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, actual)).To(BeTrue())
		expectedMode := os.FileMode(0600)
		if change {
			expectedMode = 0644
		}
		Expect(actual.Mode().Perm()).To(Equal(expectedMode))
		Expect(publicationFaultBytes(filepath.Dir(slot), filepath.Base(slot))).To(Equal(bytes))
		recoveryClosed(files)
		Expect(inventory.Restorable).To(BeFalse())
		Expect(inventory.RollbackReady).To(BeFalse())
		Expect(inventory.ActivationReady).To(BeFalse())
		Expect(inventory.Applicable).To(BeFalse())
		Expect(inventory.PruneAuthorized).To(BeFalse())
		if change {
			Expect(inventory.Records).To(BeEmpty())
			Expect(inventory.Complete).To(BeFalse())
		}
		expected := 2
		if reportedIO {
			expected = 1
		}
		Expect(inventory.Status()).To(Equal(expected), "earlier actual I/O failure keeps operational attribution after final invalidation")
	}, Entry("real read then reported EIO", false, true), Entry("real read then mode change and reported EIO", true, true), Entry("real read then mode change without EIO", true, false))
})

var _ = Describe("Durable publication reverse direction boundary", func() {
	// per docs/adr/0095-durable-live-publication.md:138
	// per docs/adr/0095-durable-live-publication.md:145
	DescribeTable("refuses Apply after a valid reverse completion has begun", func(failSync bool) {
		root, request, environment := durablePublicationOwnedFixture()
		ctx := context.Background()
		original := publicationFaultBytes(root, request.Path)
		before, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		controls, files := publicationFaultControls()
		armed, faulted, renames := false, false, 0
		controls.storage.syncFile = func(_ context.Context, file *os.File) error {
			err := file.Sync()
			if err == nil && armed && failSync && !faulted && publicationSame(file, filepath.Join(root, request.Path)) {
				faulted = true
				return unix.EIO
			}
			return err
		}
		controls.rename = func(ctx context.Context, stage *filepublish.Stage, name string) error {
			err := stage.Publish(ctx, name)
			if err == nil && name == filepath.Base(request.Path) {
				renames++
			}
			return err
		}
		operation, err := beginDurablePublication(ctx, root, request, environment, controls)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = operation.Close(ctx) })
		armed = true
		reverseErr := operation.Restore(ctx)
		if failSync {
			Expect(faulted).To(BeTrue(), "the actual owned original sync succeeds before injected EIO")
			Expect(ErrorStatus(reverseErr)).To(Equal(1))
			_ = publicationFaultPending(root)
		} else {
			Expect(reverseErr).NotTo(HaveOccurred())
		}
		applyErr := operation.Apply(ctx)
		afterApply, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		bytesAfterApply := publicationFaultBytes(root, request.Path)
		renamedAfterApply := renames
		if failSync {
			Expect(operation.Restore(ctx)).To(Succeed())
		}
		Expect(operation.Close(ctx)).To(Succeed())
		publicationObservationReleased(root, *files)
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(original))
		Expect(os.SameFile(before, afterApply)).To(BeTrue(), "opposite-direction Apply cannot replace the original")
		Expect(afterApply.Mode()).To(Equal(before.Mode()))
		Expect(bytesAfterApply).To(Equal(original))
		Expect(renamedAfterApply).To(BeZero())
		Expect(applyErr).To(HaveOccurred())
		Expect(ErrorStatus(applyErr)).To(Equal(2))
	}, Entry("actual original sync then reported EIO", true), Entry("completed checked abort", false))
})

func durablePublicationFaultBegin(root string, request PublicationRequest, environment map[string]string, controls publicationOps) *Publication {
	GinkgoHelper()
	operation, err := beginDurablePublication(context.Background(), root, request, environment, controls)
	Expect(err).NotTo(HaveOccurred())
	Expect(operation).NotTo(BeNil())
	DeferCleanup(func() { _ = operation.Close(context.Background()) })
	return operation
}
func durablePublicationJournal(root string, request PublicationRequest) (map[string]any, os.FileInfo, []byte) {
	GinkgoHelper()
	path := ".factory/backups/.publications/" + request.MigrationID + ".json"
	data := publicationFaultBytes(root, path)
	var record map[string]any
	Expect(json.Unmarshal(data, &record)).To(Succeed())
	info, err := os.Lstat(filepath.Join(root, path))
	Expect(err).NotTo(HaveOccurred())
	Expect(info.Mode()).To(Equal(os.FileMode(0600)))
	return record, info, data
}
func durablePublicationNoExecution(root string) {
	GinkgoHelper()
	for _, name := range []string{"budget.json", "loops.json", "budget.lock", "loop.lock"} {
		_, err := os.Lstat(filepath.Join(root, ".factory", name))
		Expect(os.IsNotExist(err)).To(BeTrue(), "publication cannot create execution state: %s", name)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".factory/runtime-activity"))
	Expect(err).NotTo(HaveOccurred())
	Expect(entries).To(BeEmpty())
}

var _ = Describe("Durable publication actual journal faults", func() {
	// per docs/adr/0095-durable-live-publication.md:54
	// per docs/adr/0095-durable-live-publication.md:61
	It("durably reserves pending before an actual constructor query reports ownership and cancellation", func() {
		root, request, environment := durablePublicationOwnedFixture()
		before, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		original := publicationFaultBytes(root, request.Path)
		controls, files := publicationFaultControls()
		fileSynced, parentSynced, queried := false, false, false
		controls.storage.syncFile = func(_ context.Context, file *os.File) error {
			err := file.Sync()
			if err == nil && publicationSame(file, filepath.Join(root, ".factory/runtime-publication.pending")) {
				fileSynced = true
			}
			return err
		}
		controls.storage.syncDirectory = func(_ context.Context, file *os.File) error {
			err := file.Sync()
			if err == nil && fileSynced && publicationSame(file, filepath.Join(root, ".factory")) {
				parentSynced = true
			}
			return err
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		pid := 0
		var pending os.FileInfo
		controls.query = func(ctx context.Context, directory string, argv []string, values map[string]string, allowance time.Duration) (native.CommandResult, error) {
			Expect(fileSynced).To(BeTrue())
			Expect(parentSynced).To(BeTrue(), "actual file and directory Sync precede the first process")
			pending = publicationFaultPending(root)
			result, actualErr := native.ExecuteCommand(ctx, directory, argv, values, allowance)
			Expect(actualErr).NotTo(HaveOccurred())
			Expect(result.ExitConfirmed).To(BeTrue())
			Expect(result.ProcessPID).To(BeNumerically(">", 0))
			pid, queried = result.ProcessPID, true
			cancel()
			// Reported uncertainty after real completed execution, not a surviving-process claim.
			return result, errors.Join(&native.OwnershipError{ProcessPID: pid}, ctx.Err())
		}
		operation, primary := beginDurablePublication(ctx, root, request, environment, controls)
		Expect(queried).To(BeTrue())
		Expect(operation).To(BeNil())
		publicationFaultPreserved(root, pending)
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		Expect(after.Mode()).To(Equal(before.Mode()))
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(original))
		_, slotErr := os.Lstat(filepath.Join(root, ".factory/backups/.publications", request.MigrationID+".json"))
		Expect(os.IsNotExist(slotErr)).To(BeTrue(), "no record is written before exclusion is qualified")
		publicationObservationReleased(root, *files)
		durablePublicationNoExecution(root)
		Expect(ErrorStatus(primary)).To(Equal(1))
		Expect(errors.Is(primary, context.Canceled)).To(BeTrue())
		var ownership *native.OwnershipError
		Expect(errors.As(primary, &ownership)).To(BeTrue())
		Expect(ownership.ProcessPID).To(Equal(pid))
	})

	// per docs/adr/0095-durable-live-publication.md:113
	// per docs/adr/0095-durable-live-publication.md:148
	It("keeps the original and pending after real initial-record preparation reports EIO", func() {
		root, request, environment := durablePublicationOwnedFixture()
		before, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		original := publicationFaultBytes(root, request.Path)
		controls, files := publicationFaultControls()
		prepared := false
		controls.prepare = func(ctx context.Context, directory *os.Root, prefix string, data []byte, mode os.FileMode) (*filepublish.Stage, error) {
			stage, err := filepublish.Prepare(ctx, directory, prefix, data, mode)
			var record map[string]any
			if err == nil && json.Unmarshal(data, &record) == nil && record["phase"] == "prepared" {
				prepared = true
				Expect(directory.ReadFile(stage.Name())).To(Equal(data))
				return stage, unix.EIO
			}
			return stage, err
		}
		operation, primary := beginDurablePublication(context.Background(), root, request, environment, controls)
		Expect(prepared).To(BeTrue(), "actual prepared bytes are read back after real preparation and Sync")
		Expect(operation).To(BeNil())
		Expect(ErrorStatus(primary)).To(Equal(1))
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		Expect(after.Mode()).To(Equal(before.Mode()))
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(original))
		_ = publicationFaultPending(root)
		inventory, err := InspectPublications(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		Expect(inventory.Status()).To(Equal(2))
		Expect(inventory.Records).To(HaveLen(1))
		Expect(inventory.Records[0].Classification).NotTo(Equal("record_checked"))
		publicationObservationReleased(root, *files)
		durablePublicationNoExecution(root)
	})

	// per docs/adr/0095-durable-live-publication.md:120
	// per docs/adr/0095-durable-live-publication.md:133
	// per docs/adr/0095-durable-live-publication.md:138
	DescribeTable("reuses an actual terminal record after rename or directory-sync uncertainty without republishing the selected file", func(reverse, failSync bool) {
		root, request, environment := durablePublicationOwnedFixture()
		original := publicationFaultBytes(root, request.Path)
		manifest := publicationFaultBytes(root, ".factory/backups/"+request.MigrationID+"/manifest.json")
		controls, files := publicationFaultControls()
		prepared := map[*filepublish.Stage]*os.File{}
		controls.openPrepared = func(ctx context.Context, stage *filepublish.Stage) (*os.File, error) {
			file, err := stage.OpenPrepared(ctx)
			if file != nil {
				prepared[stage] = file
				*files = append(*files, file)
			}
			return file, err
		}
		terminal, faulted, activeRenames, recordRenames := false, false, 0, 0
		phase := "forward_completed"
		if reverse {
			phase = "restored"
		}
		controls.rename = func(ctx context.Context, stage *filepublish.Stage, name string) error {
			err := stage.Publish(ctx, name)
			if err != nil {
				return err
			}
			if name == filepath.Base(request.Path) {
				activeRenames++
				return nil
			}
			recordRenames++
			record, info, _ := durablePublicationJournal(root, request)
			retained, statErr := prepared[stage].Stat()
			Expect(statErr).NotTo(HaveOccurred())
			Expect(os.SameFile(retained, info)).To(BeTrue(), "the actual prepared descriptor becomes the named record")
			if record["phase"] == phase {
				terminal = true
				if !failSync && !faulted {
					faulted = true
					return unix.EIO
				}
			}
			return nil
		}
		controls.storage.syncDirectory = func(_ context.Context, file *os.File) error {
			err := file.Sync()
			if err == nil && failSync && terminal && !faulted && publicationSame(file, filepath.Join(root, ".factory/backups/.publications")) {
				faulted = true
				return unix.EIO
			}
			return err
		}
		operation := durablePublicationFaultBegin(root, request, environment, controls)
		pending := publicationFaultPending(root)
		Expect(operation.Apply(context.Background())).To(Succeed())
		complete, opposite := operation.Finish, operation.Restore
		if reverse {
			complete, opposite = operation.Restore, operation.Finish
		}
		primary := complete(context.Background())
		Expect(terminal).To(BeTrue())
		Expect(faulted).To(BeTrue(), "the injected failure follows an actual terminal rename or directory Sync")
		publicationFaultTyped(root, primary)
		Expect(ErrorStatus(primary)).To(Equal(1))
		publicationFaultPreserved(root, pending)
		record, terminalInfo, terminalBytes := durablePublicationJournal(root, request)
		Expect(record["phase"]).To(Equal(phase))
		selected, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		selectedBytes := publicationFaultBytes(root, request.Path)
		selectedCount, recordCount := activeRenames, recordRenames
		Expect(ErrorStatus(opposite(context.Background()))).To(Equal(2))
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(selectedBytes))
		Expect(complete(context.Background())).To(Succeed())
		_, actualRecord, actualBytes := durablePublicationJournal(root, request)
		Expect(os.SameFile(terminalInfo, actualRecord)).To(BeTrue())
		Expect(actualBytes).To(Equal(terminalBytes))
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(selected, after)).To(BeTrue())
		Expect(after.Mode()).To(Equal(selected.Mode()))
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(selectedBytes))
		Expect(activeRenames).To(Equal(selectedCount))
		Expect(recordRenames).To(Equal(recordCount), "durability-only retry must reuse the same terminal record")
		if reverse {
			Expect(selectedBytes).To(Equal(original))
		} else {
			Expect(selectedBytes).To(Equal(request.Replacement))
		}
		_, pendingErr := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(pendingErr)).To(BeTrue())
		Expect(operation.Close(context.Background())).To(Succeed())
		publicationObservationReleased(root, *files)
		Expect(publicationFaultBytes(root, ".factory/backups/"+request.MigrationID+"/manifest.json")).To(Equal(manifest))
		durablePublicationNoExecution(root)
	}, Entry("forward terminal actual rename then EIO", false, false), Entry("reverse terminal actual rename then EIO", true, false), Entry("forward terminal actual directory Sync then EIO", false, true))

	// per docs/adr/0095-durable-live-publication.md:133
	// per docs/adr/0095-durable-live-publication.md:152
	It("does not recreate pending after actual unlink followed by parent-sync failure", func() {
		root, request, environment := durablePublicationOwnedFixture()
		controls, files := publicationFaultControls()
		unlinked, faulted := false, false
		controls.unlink = func(_ context.Context, parent *os.File, name string) error {
			record, _, _ := durablePublicationJournal(root, request)
			Expect(record["phase"]).To(Equal("forward_completed"))
			err := unix.Unlinkat(int(parent.Fd()), name, 0)
			if err == nil {
				unlinked = true
			}
			return err
		}
		controls.storage.syncDirectory = func(_ context.Context, file *os.File) error {
			err := file.Sync()
			if err == nil && unlinked && !faulted && publicationSame(file, filepath.Join(root, ".factory")) {
				faulted = true
				return unix.EIO
			}
			return err
		}
		operation := durablePublicationFaultBegin(root, request, environment, controls)
		Expect(operation.Apply(context.Background())).To(Succeed())
		primary := operation.Finish(context.Background())
		Expect(unlinked).To(BeTrue())
		Expect(faulted).To(BeTrue())
		publicationFaultTyped(root, primary)
		_, pendingErr := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(pendingErr)).To(BeTrue())
		_, recordInfo, bytes := durablePublicationJournal(root, request)
		selected, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(ErrorStatus(operation.Restore(context.Background()))).To(Equal(2))
		Expect(operation.Finish(context.Background())).To(Succeed())
		_, actual, actualBytes := durablePublicationJournal(root, request)
		Expect(os.SameFile(recordInfo, actual)).To(BeTrue())
		Expect(actualBytes).To(Equal(bytes))
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(selected, after)).To(BeTrue())
		Expect(operation.Close(context.Background())).To(Succeed())
		publicationObservationReleased(root, *files)
	})

	// per docs/adr/0095-durable-live-publication.md:31
	// per docs/adr/0095-durable-live-publication.md:168
	It("copies mutable caller bytes and shares Finish ownership across ordinary aliases while preserving v1 inspection", func() {
		root, request, environment := durablePublicationOwnedFixture()
		intended := append([]byte(nil), request.Replacement...)
		operation, err := BeginDurablePublication(context.Background(), root, request, environment)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = operation.Close(context.Background()) })
		alias := *operation
		for i := range request.Replacement {
			request.Replacement[i] = 'X'
		}
		Expect(operation.Apply(context.Background())).To(Succeed())
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(intended))
		selected, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(alias.Finish(context.Background())).To(Succeed())
		Expect(ErrorStatus(operation.Finish(context.Background()))).To(Equal(2))
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(selected, after)).To(BeTrue())
		Expect(alias.Close(context.Background())).To(Succeed())
		Expect(ErrorStatus(operation.Restore(context.Background()))).To(Equal(2))
		inventory, err := InspectRecovery(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		Expect(inventory.Status()).To(BeZero())
		Expect(inventory.SetCount).To(Equal(1))
		Expect(inventory.Sets[0].Classification).To(Equal("integrity_checked"))
		durablePublicationNoExecution(root)
	})

	// per docs/adr/0095-durable-live-publication.md:123
	// per docs/adr/0095-durable-live-publication.md:138
	DescribeTable("preserves foreign terminal-record edits and equal-byte replacement inodes after real rename uncertainty", func(newInode bool) {
		root, request, environment := durablePublicationOwnedFixture()
		controls, files := publicationFaultControls()
		faulted := false
		controls.rename = func(ctx context.Context, stage *filepublish.Stage, name string) error {
			err := stage.Publish(ctx, name)
			if err == nil && name == request.MigrationID+".json" {
				record, _, _ := durablePublicationJournal(root, request)
				if record["phase"] == "forward_completed" && !faulted {
					faulted = true
					return unix.EIO
				}
			}
			return err
		}
		operation := durablePublicationFaultBegin(root, request, environment, controls)
		Expect(operation.Apply(context.Background())).To(Succeed())
		publicationFaultTyped(root, operation.Finish(context.Background()))
		Expect(faulted).To(BeTrue())
		pending := publicationFaultPending(root)
		_, old, data := durablePublicationJournal(root, request)
		slot := filepath.Join(root, ".factory/backups/.publications", request.MigrationID+".json")
		if newInode {
			Expect(os.Rename(slot, slot+".retained")).To(Succeed())
		} else {
			data = append(data, ' ')
		}
		Expect(os.WriteFile(slot, data, 0600)).To(Succeed()) // #nosec G703 -- deliberate foreign mutation of this test's actual owned record slot.
		foreign, err := os.Lstat(slot)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(old, foreign)).To(Equal(!newInode))
		selected, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(ErrorStatus(operation.Finish(context.Background()))).To(Equal(2))
		Expect(ErrorStatus(operation.Restore(context.Background()))).To(Equal(2))
		Expect(operation.Close(context.Background())).To(HaveOccurred())
		actual, err := os.Lstat(slot)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(foreign, actual)).To(BeTrue())
		Expect(publicationFaultBytes(filepath.Dir(slot), filepath.Base(slot))).To(Equal(data))
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(selected, after)).To(BeTrue())
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(request.Replacement))
		publicationFaultPreserved(root, pending)
		publicationObservationReleased(root, *files)
	}, Entry("edited terminal bytes on the same inode", false), Entry("equal terminal bytes on a foreign inode", true))

	// per docs/adr/0095-durable-live-publication.md:148
	// per docs/adr/0095-durable-live-publication.md:150
	It("retains typed active uncertainty through canceled checked-close failure while releasing every descriptor and guard", func() {
		root, request, environment := durablePublicationOwnedFixture()
		controls, files := publicationFaultControls()
		faulted, closed := false, false
		controls.rename = func(ctx context.Context, stage *filepublish.Stage, name string) error {
			err := stage.Publish(ctx, name)
			if err == nil && name == filepath.Base(request.Path) && !faulted {
				faulted = true
				return unix.EIO
			}
			return err
		}
		controls.close = func(file *os.File) error {
			err := file.Close()
			if err == nil && !closed {
				closed = true
				return unix.EIO
			}
			return err
		}
		operation := durablePublicationFaultBegin(root, request, environment, controls)
		pending := publicationFaultPending(root)
		publicationFaultTyped(root, operation.Apply(context.Background()))
		Expect(faulted).To(BeTrue())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		primary := operation.Close(ctx)
		Expect(closed).To(BeTrue(), "a real descriptor Close precedes reported cleanup EIO")
		publicationFaultTyped(root, primary)
		Expect(ErrorStatus(primary)).To(Equal(1))
		Expect(errors.Is(primary, context.Canceled)).To(BeTrue())
		publicationFaultPreserved(root, pending)
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(request.Replacement))
		publicationObservationReleased(root, *files)
		durablePublicationNoExecution(root)
	})
})

type durablePublicationCrashInput struct {
	Root        string
	Request     PublicationRequest
	Environment map[string]string
	Ready       string
	Reverse     bool
}

var _ = Describe("Durable publication terminal interruption", func() {
	// per docs/adr/0095-durable-live-publication.md:133
	// per docs/adr/0095-durable-live-publication.md:174
	// per docs/adr/0095-durable-live-publication.md:176
	DescribeTable("retains the original pending inode after ready-then-SIGKILL following actual terminal directory sync", func(reverse bool) {
		if path := os.Getenv("FACTORY_DURABLE_TERMINAL_CHILD"); path != "" {
			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			var input durablePublicationCrashInput
			Expect(json.Unmarshal(data, &input)).To(Succeed())
			if input.Reverse != reverse {
				return
			}
			phase := "forward_completed"
			if reverse {
				phase = "restored"
			}
			controls := publicationOps{storage: recoveryWriteOps{syncDirectory: func(_ context.Context, file *os.File) error {
				err := file.Sync()
				if err == nil && publicationSame(file, filepath.Join(input.Root, ".factory/backups/.publications")) {
					recordBytes, readErr := os.ReadFile(filepath.Join(input.Root, ".factory/backups/.publications", input.Request.MigrationID+".json")) // #nosec G703 -- inspect only this child fixture's selected record while waiting for the actual terminal phase.
					var record map[string]any
					if readErr == nil && json.Unmarshal(recordBytes, &record) == nil && record["phase"] == phase {
						_ = publicationFaultPending(input.Root)
						Expect(os.WriteFile(input.Ready, []byte(phase), 0600)).To(Succeed()) // #nosec G703 -- this isolated test child writes only its parent's private handshake path.
						select {}
					}
				}
				return err
			}}}
			operation, err := beginDurablePublication(context.Background(), input.Root, input.Request, input.Environment, controls)
			Expect(err).NotTo(HaveOccurred())
			Expect(operation.Apply(context.Background())).To(Succeed())
			if reverse {
				err = operation.Restore(context.Background())
			} else {
				err = operation.Finish(context.Background())
			}
			Fail(fmt.Sprintf("terminal sync handshake was not reached: %v", err))
			return
		}
		root, request, environment := durablePublicationOwnedFixture()
		original := publicationFaultBytes(root, request.Path)
		ready := filepath.Join(GinkgoT().TempDir(), "terminal-ready")
		input := durablePublicationCrashInput{Root: root, Request: request, Environment: environment, Ready: ready, Reverse: reverse}
		data, err := json.Marshal(input)
		Expect(err).NotTo(HaveOccurred())
		inputPath := filepath.Join(GinkgoT().TempDir(), "child-request.json")
		Expect(os.WriteFile(inputPath, data, 0600)).To(Succeed())
		executable, err := os.Executable()
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, executable, "-ginkgo.focus=Durable publication terminal interruption", "-ginkgo.no-color", "-ginkgo.succinct") // #nosec G204 -- execute only this compiled race test binary with literal Ginkgo selectors.
		command.Env = append(os.Environ(), "FACTORY_DURABLE_TERMINAL_CHILD="+inputPath)
		command.Stdout, command.Stderr = GinkgoWriter, GinkgoWriter
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		started, waited := false, false
		DeferCleanup(func() {
			if started && !waited {
				_ = unix.Kill(-command.Process.Pid, unix.SIGKILL)
				_ = command.Wait()
			}
		})
		Expect(command.Start()).To(Succeed())
		started = true
		Eventually(func() bool { _, err := os.Lstat(ready); return err == nil }, 10*time.Second, 10*time.Millisecond).Should(BeTrue(), "actual terminal record directory Sync must complete before the child announces readiness")
		phase := "forward_completed"
		if reverse {
			phase = "restored"
		}
		Expect(publicationFaultBytes(filepath.Dir(ready), filepath.Base(ready))).To(Equal([]byte(phase)))
		pending := publicationFaultPending(root)
		record, recordInfo, recordBytes := durablePublicationJournal(root, request)
		Expect(record["phase"]).To(Equal(phase))
		selected, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		selectedBytes := publicationFaultBytes(root, request.Path)
		if reverse {
			Expect(selectedBytes).To(Equal(original))
		} else {
			Expect(selectedBytes).To(Equal(request.Replacement))
		}
		lock, err := os.OpenFile(filepath.Join(root, ".factory/runtime-transition.lock"), os.O_RDWR, 0)
		Expect(err).NotTo(HaveOccurred())
		defer func() { Expect(lock.Close()).To(Succeed()) }()
		Expect(unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB)).To(MatchError(unix.EWOULDBLOCK))
		Expect(command.Process.Kill()).To(Succeed())
		waitErr := command.Wait()
		waited = true
		var exit *exec.ExitError
		Expect(errors.As(waitErr, &exit)).To(BeTrue())
		status, ok := exit.Sys().(syscall.WaitStatus)
		Expect(ok).To(BeTrue())
		Expect(status.Signal()).To(Equal(syscall.SIGKILL), "this observes process death, not simulated power loss")
		Expect(unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB)).To(Succeed())
		Expect(unix.Flock(int(lock.Fd()), unix.LOCK_UN)).To(Succeed())
		publicationFaultPreserved(root, pending)
		_, actualRecord, actualBytes := durablePublicationJournal(root, request)
		Expect(os.SameFile(recordInfo, actualRecord)).To(BeTrue())
		Expect(actualBytes).To(Equal(recordBytes))
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(selected, after)).To(BeTrue())
		Expect(after.Mode()).To(Equal(selected.Mode()))
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(selectedBytes))
		inventory, err := InspectPublications(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		Expect(inventory.Status()).To(Equal(2))
		Expect(inventory.Records[0].Phase).To(Equal(phase))
		Expect(inventory.Restorable).To(BeFalse())
		Expect(inventory.RollbackReady).To(BeFalse())
		Expect(inventory.ActivationReady).To(BeFalse())
		Expect(inventory.Applicable).To(BeFalse())
		Expect(inventory.PruneAuthorized).To(BeFalse())
		_, err = transition.Shared(context.Background(), root)
		Expect(err).To(HaveOccurred())
		_, err = transition.Exclusive(context.Background(), root)
		Expect(err).To(HaveOccurred())
		publicationFaultPreserved(root, pending)
		durablePublicationNoExecution(root)
	}, Entry("forward terminal record with original pending", false), Entry("reverse terminal record with original pending", true))
})
