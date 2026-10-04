package assessment

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/anoop2811/software-factory-template/internal/filepublish"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

func publicationFaultFixture() (string, string, PublicationRequest) {
	GinkgoHelper()
	root, set, paths := recoveryPlanningFixture()
	proposal, err := ProposeAdoption(context.Background(), root, paths[:1])
	Expect(err).NotTo(HaveOccurred())
	return root, set, PublicationRequest{MigrationID: filepath.Base(set), Path: paths[0], Confirmation: proposal.ProposalDigest, Replacement: []byte("LIVE_REPLACEMENT_BYTES\n")}
}
func publicationSame(file *os.File, path string) bool {
	info, err := file.Stat()
	other, namedErr := os.Lstat(path)
	return err == nil && namedErr == nil && os.SameFile(info, other)
}
func publicationFaultBytes(root, path string) []byte {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join(root, path))
	Expect(err).NotTo(HaveOccurred())
	return data
}
func publicationFaultPending(root string) os.FileInfo {
	GinkgoHelper()
	info, err := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
	Expect(err).NotTo(HaveOccurred())
	Expect(info.Mode()).To(Equal(os.FileMode(0600)))
	Expect(info.Size()).To(BeZero())
	return info
}
func publicationFaultPreserved(root string, before os.FileInfo) {
	GinkgoHelper()
	after := publicationFaultPending(root)
	Expect(os.SameFile(before, after)).To(BeTrue())
}
func publicationFaultTyped(root string, err error) {
	GinkgoHelper()
	Expect(err).To(HaveOccurred())
	var uncertainty *PublicationError
	Expect(errors.As(err, &uncertainty)).To(BeTrue(), "post-rename uncertainty must retain the production error type")
	Expect(uncertainty.MayHaveChanged).To(BeTrue())
	Expect(err.Error()).NotTo(ContainSubstring("PRIVATE_"))
	Expect(err.Error()).NotTo(ContainSubstring(root))
}
func publicationFaultControls() (publicationOps, *[]*os.File) {
	GinkgoHelper()
	files := []*os.File{}
	capture := func(file *os.File) {
		if file == nil {
			return
		}
		for _, seen := range files {
			if seen == file {
				return
			}
		}
		files = append(files, file)
	}
	controls := publicationOps{storage: recoveryWriteOps{open: func(_ context.Context, parent *os.File, name string, flags int, mode uint32) (*os.File, error) {
		capture(parent)
		Expect(flags & unix.O_NOFOLLOW).NotTo(BeZero())
		Expect(flags & unix.O_CLOEXEC).NotTo(BeZero())
		Expect(flags & unix.O_TRUNC).To(BeZero())
		fd, err := unix.Openat(int(parent.Fd()), name, flags, mode)
		if err != nil {
			return nil, err
		}
		file := os.NewFile(uintptr(fd), name)
		capture(file)
		return file, nil
	}}, observe: ops{open: func(parent *os.File, name string, flags int) (*os.File, error) {
		capture(parent)
		file, err := openAt(parent, name, flags)
		capture(file)
		return file, err
	}}}
	return controls, &files
}
func publicationFaultBegin(root string, request PublicationRequest, controls publicationOps) *Publication {
	GinkgoHelper()
	operation, err := beginPublication(context.Background(), root, request, controls)
	Expect(err).NotTo(HaveOccurred())
	Expect(operation).NotTo(BeNil())
	DeferCleanup(func() { _ = operation.Close(context.Background()) })
	return operation
}

var _ = Describe("Live publication pinned qualification faults", func() {
	// per docs/adr/0094-live-publication-restoration.md:31
	// per docs/adr/0094-live-publication-restoration.md:56
	// per docs/adr/0094-live-publication-restoration.md:126
	DescribeTable("qualifies actual saved input and pending durability before exposing a live handle", func(phase string) {
		root, set, request := publicationFaultFixture()
		controls, files := publicationFaultControls()
		original := publicationFaultBytes(root, request.Path)
		saved := publicationFaultBytes(set, "files/"+request.Path)
		triggered := false
		controls.observe.read = func(file *os.File, data []byte) (int, error) {
			n, err := file.Read(data)
			if !triggered && phase == "saved read" && n > 0 && publicationSame(file, filepath.Join(set, "files", request.Path)) {
				triggered = true
				return n, unix.EIO
			}
			return n, err
		}
		controls.storage.syncFile = func(_ context.Context, file *os.File) error {
			err := file.Sync()
			if !triggered && ((phase == "saved sync" && publicationSame(file, filepath.Join(set, "files", request.Path))) ||
				(phase == "pending sync" && publicationSame(file, filepath.Join(root, ".factory/runtime-publication.pending")))) {
				triggered = true
				return errors.Join(err, errors.New("PRIVATE_DURABILITY_FAILURE"))
			}
			return err
		}
		controls.storage.syncDirectory = func(_ context.Context, file *os.File) error {
			err := file.Sync()
			_, pendingErr := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
			if !triggered && ((phase == "set sync" && publicationSame(file, set)) ||
				(phase == "pending parent sync" && pendingErr == nil && publicationSame(file, filepath.Join(root, ".factory")))) {
				triggered = true
				return errors.Join(err, errors.New("PRIVATE_DIRECTORY_FAILURE"))
			}
			return err
		}
		operation, err := beginPublication(context.Background(), root, request, controls)
		if operation != nil {
			DeferCleanup(func() { _ = operation.Close(context.Background()) })
		}
		Expect(triggered).To(BeTrue(), "the real selected operation must be reached")
		Expect(operation).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).NotTo(ContainSubstring("PRIVATE_"))
		Expect(err.Error()).NotTo(ContainSubstring(root))
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(original))
		Expect(publicationFaultBytes(set, "files/"+request.Path)).To(Equal(saved))
		if phase == "pending sync" || phase == "pending parent sync" {
			_ = publicationFaultPending(root)
		}
		recoveryClosed(*files)
	}, Entry("saved read EIO", "saved read"), Entry("actual saved file sync then failure", "saved sync"),
		Entry("actual recovery directory sync then failure", "set sync"), Entry("actual pending file sync then failure", "pending sync"),
		Entry("actual pending parent sync then failure", "pending parent sync"))

	// per docs/adr/0094-live-publication-restoration.md:73
	// per docs/adr/0094-live-publication-restoration.md:91
	DescribeTable("rechecks the before-image after actual sibling preparation and before any rename", func(change string) {
		root, _, request := publicationFaultFixture()
		controls, files := publicationFaultControls()
		changed, renamed := false, false
		controls.prepare = func(ctx context.Context, directory *os.Root, prefix string, data []byte, mode os.FileMode) (*filepublish.Stage, error) {
			stage, err := filepublish.Prepare(ctx, directory, prefix, data, mode)
			if err == nil && !changed {
				path := filepath.Join(root, request.Path)
				if change == "inode" {
					before := publicationFaultBytes(root, request.Path)
					Expect(os.Rename(path, path+".retained-original")).To(Succeed())
					Expect(os.WriteFile(path, before, 0600)).To(Succeed())
					Expect(os.Chmod(path, 0755)).To(Succeed())
				} else {
					Expect(os.Chmod(path, 0700)).To(Succeed())
				}
				changed = true
			}
			return stage, err
		}
		controls.rename = func(ctx context.Context, stage *filepublish.Stage, name string) error {
			renamed = true
			return stage.Publish(ctx, name)
		}
		operation := publicationFaultBegin(root, request, controls)
		err := operation.Apply(context.Background())
		Expect(changed).To(BeTrue())
		Expect(renamed).To(BeFalse(), "immediate observation revalidation must precede the actual rename")
		Expect(err).To(HaveOccurred())
		_ = publicationFaultPending(root)
		Expect(operation.Close(context.Background())).NotTo(Succeed())
		recoveryClosed(*files)
	}, Entry("equal bytes new before-inode", "inode"), Entry("complete before-mode changed", "mode"))
})

var _ = Describe("Live publication actual rename uncertainty", func() {
	// per docs/adr/0094-live-publication-restoration.md:78
	// per docs/adr/0094-live-publication-restoration.md:81
	DescribeTable("retains actual published ownership after post-rename errors and permits explicit checked restoration", func(phase string) {
		root, _, request := publicationFaultFixture()
		controls, files := publicationFaultControls()
		original := publicationFaultBytes(root, request.Path)
		renamed, triggered := false, false
		controls.rename = func(ctx context.Context, stage *filepublish.Stage, name string) error {
			err := stage.Publish(ctx, name)
			if err == nil && !renamed {
				renamed = true
				if phase == "rename" {
					triggered = true
					return unix.EIO
				}
			}
			return err
		}
		controls.storage.syncDirectory = func(_ context.Context, file *os.File) error {
			err := file.Sync()
			if !triggered && renamed && phase == "directory sync" && publicationSame(file, filepath.Join(root, "scripts")) {
				triggered = true
				return errors.Join(err, unix.EIO)
			}
			return err
		}
		controls.storage.read = func(_ context.Context, file *os.File, data []byte) (int, error) {
			n, err := file.Read(data)
			if !triggered && renamed && phase == "readback" && n > 0 && publicationSame(file, filepath.Join(root, request.Path)) {
				triggered = true
				return n, unix.EIO
			}
			return n, err
		}
		operation := publicationFaultBegin(root, request, controls)
		pending := publicationFaultPending(root)
		err := operation.Apply(context.Background())
		Expect(renamed).To(BeTrue())
		Expect(triggered).To(BeTrue(), "the selected fault must follow a real successful rename")
		publicationFaultTyped(root, err)
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(request.Replacement))
		publicationFaultPreserved(root, pending)
		Expect(operation.Apply(context.Background())).NotTo(Succeed())
		Expect(operation.Restore(context.Background())).To(Succeed())
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(original))
		Expect(operation.Close(context.Background())).To(Succeed())
		recoveryClosed(*files)
	}, Entry("actual rename then EIO", "rename"), Entry("actual named readback then EIO", "readback"), Entry("actual directory sync then EIO", "directory sync"))

	// per docs/adr/0094-live-publication-restoration.md:96
	// per docs/adr/0094-live-publication-restoration.md:97
	DescribeTable("retries restored-inode durability without performing another publication after a real restore rename", func(phase string) {
		root, _, request := publicationFaultFixture()
		controls, files := publicationFaultControls()
		original := publicationFaultBytes(root, request.Path)
		renames, triggered := 0, false
		controls.rename = func(ctx context.Context, stage *filepublish.Stage, name string) error {
			err := stage.Publish(ctx, name)
			if err == nil {
				renames++
			}
			if err == nil && renames == 2 && phase == "rename" {
				triggered = true
				return unix.EIO
			}
			return err
		}
		controls.storage.syncDirectory = func(_ context.Context, file *os.File) error {
			err := file.Sync()
			if !triggered && renames == 2 && phase == "directory sync" && publicationSame(file, filepath.Join(root, "scripts")) {
				triggered = true
				return errors.Join(err, unix.EIO)
			}
			return err
		}
		operation := publicationFaultBegin(root, request, controls)
		Expect(operation.Apply(context.Background())).To(Succeed())
		pending := publicationFaultPending(root)
		err := operation.Restore(context.Background())
		Expect(triggered).To(BeTrue())
		Expect(renames).To(Equal(2))
		publicationFaultTyped(root, err)
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(original))
		restored, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		publicationFaultPreserved(root, pending)
		Expect(operation.Restore(context.Background())).To(Succeed())
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(restored, after)).To(BeTrue(), "retry must only finish durability of the recorded actual restored inode")
		Expect(renames).To(Equal(2))
		_, err = os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(operation.Close(context.Background())).To(Succeed())
		recoveryClosed(*files)
	}, Entry("restore rename then EIO", "rename"), Entry("restore directory sync then EIO", "directory sync"))

	// per docs/adr/0094-live-publication-restoration.md:106
	// per docs/adr/0094-live-publication-restoration.md:110
	DescribeTable("reports actual cleanup failures without losing publication uncertainty or claiming success", func(primary bool) {
		root, _, request := publicationFaultFixture()
		controls, files := publicationFaultControls()
		closed := false
		controls.close = func(file *os.File) error {
			err := file.Close()
			if !closed {
				closed = true
				return errors.Join(err, errors.New("PRIVATE_CLOSE_FAILURE"))
			}
			return err
		}
		if primary {
			controls.rename = func(ctx context.Context, stage *filepublish.Stage, name string) error {
				return errors.Join(stage.Publish(ctx, name), unix.EIO)
			}
		}
		operation := publicationFaultBegin(root, request, controls)
		pending := publicationFaultPending(root)
		if primary {
			publicationFaultTyped(root, operation.Apply(context.Background()))
		} else {
			Expect(operation.Restore(context.Background())).To(Succeed())
		}
		err := operation.Close(context.Background())
		Expect(closed).To(BeTrue(), "the selected real descriptor close must occur")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).NotTo(ContainSubstring("PRIVATE_"))
		if primary {
			publicationFaultTyped(root, err)
			Expect(publicationFaultBytes(root, request.Path)).To(Equal(request.Replacement))
			publicationFaultPreserved(root, pending)
		}
		recoveryClosed(*files)
	}, Entry("post-rename primary plus actual close failure", true), Entry("checked abort plus actual close failure still fails", false))
})

var _ = Describe("Live publication checked pending removal", func() {
	// per docs/adr/0094-live-publication-restoration.md:103
	// per docs/adr/0094-live-publication-restoration.md:106
	It("preserves pending on a real unlink refusal and retries only qualified removal after durable restoration", func() {
		root, _, request := publicationFaultFixture()
		controls, files := publicationFaultControls()
		original := publicationFaultBytes(root, request.Path)
		renames, unlinks := 0, 0
		controls.rename = func(ctx context.Context, stage *filepublish.Stage, name string) error {
			err := stage.Publish(ctx, name)
			if err == nil {
				renames++
			}
			return err
		}
		controls.unlink = func(_ context.Context, parent *os.File, name string) error {
			unlinks++
			if unlinks == 1 {
				return unix.EACCES
			}
			return unix.Unlinkat(int(parent.Fd()), name, 0)
		}
		operation := publicationFaultBegin(root, request, controls)
		Expect(operation.Apply(context.Background())).To(Succeed())
		pending := publicationFaultPending(root)
		err := operation.Restore(context.Background())
		Expect(unlinks).To(Equal(1))
		Expect(err).To(HaveOccurred())
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(original))
		restored, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		publicationFaultPreserved(root, pending)
		Expect(operation.Restore(context.Background())).To(Succeed())
		Expect(unlinks).To(Equal(2))
		Expect(renames).To(Equal(2))
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(restored, after)).To(BeTrue())
		Expect(operation.Close(context.Background())).To(Succeed())
		recoveryClosed(*files)
	})

	// per docs/adr/0094-live-publication-restoration.md:104
	// per docs/adr/0094-live-publication-restoration.md:105
	// per docs/adr/0094-live-publication-restoration.md:110
	DescribeTable("reports uncertainty after actual pending unlink and sync failure without inventing retained evidence on close", func(applied bool) {
		root, _, request := publicationFaultFixture()
		controls, files := publicationFaultControls()
		original := publicationFaultBytes(root, request.Path)
		triggered, armed := false, false
		controls.storage.syncDirectory = func(_ context.Context, file *os.File) error {
			err := file.Sync()
			_, namedErr := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
			if !triggered && publicationSame(file, filepath.Join(root, ".factory")) && errors.Is(namedErr, os.ErrNotExist) &&
				string(publicationFaultBytes(root, request.Path)) == string(original) {
				// Qualification also syncs .factory before marker creation;
				// this handshake follows actual completed Apply only.
				if armed {
					triggered = true
					return errors.Join(err, unix.EIO)
				}
			}
			return err
		}
		operation := publicationFaultBegin(root, request, controls)
		if applied {
			Expect(operation.Apply(context.Background())).To(Succeed())
		}
		armed = true
		restoreErr := operation.Restore(context.Background())
		Expect(triggered).To(BeTrue(), "actual pending removal and following real parent sync must precede this error")
		if applied {
			publicationFaultTyped(root, restoreErr)
		} else {
			Expect(restoreErr).To(HaveOccurred())
			Expect(ErrorStatus(restoreErr)).To(Equal(1))
		}
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(original))
		_, err := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "durable restored original precedes pending unlink; failure must not recreate evidence")
		closeErr := operation.Close(context.Background())
		if applied {
			publicationFaultTyped(root, closeErr)
		} else {
			Expect(closeErr).To(HaveOccurred())
			Expect(ErrorStatus(closeErr)).To(Equal(1))
		}
		Expect(closeErr.Error()).NotTo(ContainSubstring("pending evidence retained"), "the actual pending path is absent")
		recoveryClosed(*files)
	}, Entry("after actual apply and restore", true), Entry("prepared abort without active rename", false))
})

var _ = Describe("Live publication invalid prepared descriptor cleanup", func() {
	// per docs/adr/0094-live-publication-restoration.md:75
	// per docs/adr/0094-live-publication-restoration.md:106
	// per docs/adr/0094-live-publication-restoration.md:110
	DescribeTable("retains an actual close failure when the actual prepared inode fails mode qualification", func(failedClose bool) {
		root, _, request := publicationFaultFixture()
		controls, files := publicationFaultControls()
		original := publicationFaultBytes(root, request.Path)
		var prepared *os.File
		qualified, closed := false, false
		controls.openPrepared = func(ctx context.Context, stage *filepublish.Stage) (*os.File, error) {
			file, err := stage.OpenPrepared(ctx)
			if err != nil {
				return file, err
			}
			prepared = file
			Expect(file.Chmod(0700)).To(Succeed())
			info, statErr := file.Stat()
			Expect(statErr).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0700)))
			qualified = true
			return file, nil
		}
		controls.storage.close = func(file *os.File) error {
			err := file.Close()
			if file == prepared {
				closed = true
				if failedClose {
					return errors.Join(err, unix.EIO)
				}
			}
			return err
		}
		operation := publicationFaultBegin(root, request, controls)
		err := operation.Apply(context.Background())
		Expect(qualified).To(BeTrue(), "the test changes a real descriptor from actual Stage.OpenPrepared, not a forged stat")
		Expect(closed).To(BeTrue(), "failed qualification must actually close the owned prepared descriptor")
		Expect(err).To(HaveOccurred())
		expected := 2
		if failedClose {
			expected = 1
		}
		Expect(ErrorStatus(err)).To(Equal(expected), "an operational descriptor-close failure cannot be silently replaced by a safety refusal")
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(original))
		_ = publicationFaultPending(root)
		Expect(err.Error()).NotTo(ContainSubstring(root))
		Expect(err.Error()).NotTo(ContainSubstring("PRIVATE_"))
		_, statErr := prepared.Stat()
		Expect(errors.Is(statErr, os.ErrClosed)).To(BeTrue())
		Expect(operation.Close(context.Background())).NotTo(Succeed())
		recoveryClosed(*files)
	}, Entry("safe metadata refusal with confirmed close", false), Entry("metadata refusal plus actual close then EIO", true))
})

// Per-operation observation controls retain descriptors obtained from the real
// shared preparer. They report one EIO only after successful actual Fstatat.
// per docs/adr/0094-live-publication-restoration.md:231
// per docs/adr/0094-live-publication-restoration.md:234
type publicationObservationWitness struct {
	renames   int
	faulted   bool
	failures  int
	allowance int
	qualified bool
	prepared  *os.File
}

func publicationObservationControls(root string, request PublicationRequest, original []byte, reverse bool, boundary string, inject bool) (publicationOps, *publicationObservationWitness, *[]*os.File) {
	GinkgoHelper()
	controls, files := publicationFaultControls()
	witness := &publicationObservationWitness{}
	if inject {
		witness.allowance = 1
	}
	wanted := 1
	if reverse {
		wanted = 2
	}
	controls.openPrepared = func(ctx context.Context, stage *filepublish.Stage) (*os.File, error) {
		file, err := stage.OpenPrepared(ctx)
		if err == nil {
			witness.prepared = file
			*files = append(*files, file)
		}
		return file, err
	}
	controls.rename = func(ctx context.Context, stage *filepublish.Stage, name string) error {
		err := stage.Publish(ctx, name)
		if err == nil {
			witness.renames++
		}
		return err
	}
	controls.storage.named = func(parent *os.File, name string) (unix.Stat_t, error) {
		actual, err := named(parent, name)
		if (!inject && witness.qualified) || (witness.faulted && witness.failures >= witness.allowance) || witness.renames != wanted {
			return actual, err
		}
		selected := name == filepath.Base(request.Path) && publicationSame(parent, filepath.Join(root, filepath.Dir(request.Path)))
		if boundary == "parent" {
			selected = name == filepath.Dir(request.Path) && publicationSame(parent, root)
		}
		if !selected {
			return actual, err
		}
		Expect(err).NotTo(HaveOccurred(), "the actual named Fstatat must succeed before reporting this observation failure")
		Expect(witness.prepared).NotTo(BeNil())
		prepared, preparedErr := witness.prepared.Stat()
		location, namedErr := os.Lstat(filepath.Join(root, request.Path))
		Expect(preparedErr).NotTo(HaveOccurred())
		Expect(namedErr).NotTo(HaveOccurred())
		Expect(os.SameFile(prepared, location)).To(BeTrue(), "the named target is still the actual descriptor obtained from Stage.OpenPrepared")
		expected := request.Replacement
		if reverse {
			expected = original
		}
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(expected))
		Expect(location.Mode()).To(Equal(prepared.Mode()))
		witness.qualified = true
		if inject && witness.failures < witness.allowance {
			witness.faulted = true
			witness.failures++
			return actual, unix.EIO
		}
		return actual, err
	}
	return controls, witness, files
}

var _ = Describe("Live publication retained candidate observation retry", func() {
	// per docs/adr/0094-live-publication-restoration.md:214
	// per docs/adr/0094-live-publication-restoration.md:217
	// per docs/adr/0094-live-publication-restoration.md:223
	// per docs/adr/0094-live-publication-restoration.md:231
	DescribeTable("reconciles the actual candidate after a real forward or reverse rename and one observation EIO", func(reverse bool, boundary string) {
		root, _, request := publicationFaultFixture()
		original := publicationFaultBytes(root, request.Path)
		controls, witness, files := publicationObservationControls(root, request, original, reverse, boundary, true)
		operation := publicationFaultBegin(root, request, controls)
		pending := publicationFaultPending(root)
		if reverse {
			Expect(operation.Apply(context.Background())).To(Succeed())
		}
		var primary error
		if reverse {
			primary = operation.Restore(context.Background())
		} else {
			primary = operation.Apply(context.Background())
		}
		Expect(witness.faulted).To(BeTrue(), "the fault follows an actual successful rename and actual named stat")
		Expect(witness.qualified).To(BeTrue())
		wanted := 1
		if reverse {
			wanted = 2
		}
		Expect(witness.renames).To(Equal(wanted))
		publicationFaultTyped(root, primary)
		Expect(ErrorStatus(primary)).To(Equal(1))
		publicationFaultPreserved(root, pending)
		before, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		expected := request.Replacement
		if reverse {
			expected = original
		}
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(expected))
		_, err = witness.prepared.Stat()
		Expect(err).NotTo(HaveOccurred(), "uncertain publication retains its actual candidate descriptor")
		retry := operation.Restore(context.Background())
		Expect(retry).To(Succeed(), "the one-shot observation failure has cleared; explicit restoration must qualify the retained actual candidate")
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		if reverse {
			Expect(os.SameFile(before, after)).To(BeTrue(), "reverse retry completes durability of the actual restored inode")
		}
		Expect(witness.renames).To(Equal(2), "reverse retry never performs a third rename")
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(original))
		_, err = os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(operation.Close(context.Background())).To(Succeed())
		recoveryClosed(*files)
	}, Entry("forward actual target stat then EIO", false, "target"), Entry("forward actual parent refresh stat then EIO", false, "parent"),
		Entry("reverse actual target stat then EIO", true, "target"), Entry("reverse actual parent refresh stat then EIO", true, "parent"))
})

func publicationObservationReleased(root string, files []*os.File) {
	GinkgoHelper()
	recoveryClosed(files)
	file, err := os.OpenFile(filepath.Join(root, ".factory/runtime-transition.lock"), os.O_RDWR, 0)
	Expect(err).NotTo(HaveOccurred())
	defer func() { Expect(file.Close()).To(Succeed()) }()
	Expect(unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)).To(Succeed(), "closure must release the actual OS guard even when pending evidence remains")
	Expect(unix.Flock(int(file.Fd()), unix.LOCK_UN)).To(Succeed())
}

var _ = Describe("Live publication retained candidate qualification controls", func() {
	// per docs/adr/0094-live-publication-restoration.md:231
	// per docs/adr/0094-live-publication-restoration.md:234
	DescribeTable("performs both observed publication directions without an injected observation failure", func(reverse bool) {
		root, _, request := publicationFaultFixture()
		original := publicationFaultBytes(root, request.Path)
		controls, witness, files := publicationObservationControls(root, request, original, reverse, "target", false)
		operation := publicationFaultBegin(root, request, controls)
		Expect(operation.Apply(context.Background())).To(Succeed())
		Expect(operation.Restore(context.Background())).To(Succeed())
		Expect(witness.qualified).To(BeTrue(), "the exact same actual descriptor/named observation boundary must be reachable without a fault")
		Expect(witness.faulted).To(BeFalse())
		Expect(witness.renames).To(Equal(2))
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(original))
		Expect(operation.Close(context.Background())).To(Succeed())
		publicationObservationReleased(root, *files)
	}, Entry("forward boundary", false), Entry("reverse boundary", true))

	// per docs/adr/0094-live-publication-restoration.md:214
	// per docs/adr/0094-live-publication-restoration.md:223
	// per docs/adr/0094-live-publication-restoration.md:228
	// per docs/adr/0094-live-publication-restoration.md:244
	DescribeTable("fails closed through another real observation EIO and completes only on a later explicit retry", func(reverse bool, boundary string) {
		root, _, request := publicationFaultFixture()
		original := publicationFaultBytes(root, request.Path)
		controls, witness, files := publicationObservationControls(root, request, original, reverse, boundary, true)
		witness.allowance = 2
		operation := publicationFaultBegin(root, request, controls)
		pending := publicationFaultPending(root)
		if reverse {
			Expect(operation.Apply(context.Background())).To(Succeed())
		}
		var primary error
		if reverse {
			primary = operation.Restore(context.Background())
		} else {
			primary = operation.Apply(context.Background())
		}
		publicationFaultTyped(root, primary)
		Expect(witness.failures).To(Equal(1), "one caller operation reports one real observation failure; it must not silently retry")
		before, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		expected := request.Replacement
		if reverse {
			expected = original
		}
		retry := operation.Restore(context.Background())
		publicationFaultTyped(root, retry)
		Expect(witness.failures).To(Equal(2))
		wanted := 1
		if reverse {
			wanted = 2
		}
		Expect(witness.renames).To(Equal(wanted), "another observation failure must not perform a new rename")
		afterError, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, afterError)).To(BeTrue())
		Expect(afterError.Mode()).To(Equal(before.Mode()))
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(expected))
		publicationFaultPreserved(root, pending)
		_, descriptorErr := witness.prepared.Stat()
		Expect(descriptorErr).NotTo(HaveOccurred(), "another failed observation retains the actual candidate descriptor")
		Expect(operation.Restore(context.Background())).To(Succeed())
		final, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		if reverse {
			Expect(os.SameFile(before, final)).To(BeTrue())
		}
		Expect(witness.renames).To(Equal(2), "reverse retry must complete the same restored inode without a third rename")
		Expect(publicationFaultBytes(root, request.Path)).To(Equal(original))
		Expect(operation.Close(context.Background())).To(Succeed())
		publicationObservationReleased(root, *files)
		Expect(ErrorStatus(retry)).To(Equal(1))
	}, Entry("forward target repeats EIO", false, "target"), Entry("forward parent repeats EIO", false, "parent"),
		Entry("reverse target repeats EIO", true, "target"), Entry("reverse parent repeats EIO", true, "parent"))

	// per docs/adr/0094-live-publication-restoration.md:217
	// per docs/adr/0094-live-publication-restoration.md:221
	// per docs/adr/0094-live-publication-restoration.md:226
	// per docs/adr/0094-live-publication-restoration.md:228
	// per docs/adr/0094-live-publication-restoration.md:245
	DescribeTable("preserves genuine conflicts introduced after a real rename and observation loss in either direction", func(reverse bool, change string) {
		root, set, request := publicationFaultFixture()
		original := publicationFaultBytes(root, request.Path)
		controls, witness, files := publicationObservationControls(root, request, original, reverse, "target", true)
		operation := publicationFaultBegin(root, request, controls)
		pending := publicationFaultPending(root)
		if reverse {
			Expect(operation.Apply(context.Background())).To(Succeed())
		}
		var primary error
		if reverse {
			primary = operation.Restore(context.Background())
		} else {
			primary = operation.Apply(context.Background())
		}
		publicationFaultTyped(root, primary)
		Expect(witness.faulted).To(BeTrue())
		Expect(witness.qualified).To(BeTrue())
		path := filepath.Join(root, request.Path)
		owned, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		captured := publicationFaultBytes(root, request.Path)
		savedPath := filepath.Join(set, "files", request.Path)
		switch change {
		case "edit":
			Expect(os.WriteFile(path, []byte("PRIVATE_EDIT_AFTER_OBSERVATION_LOSS"), 0600)).To(Succeed())
		case "equal inode replacement":
			Expect(os.Rename(path, path+".retained-owned")).To(Succeed())
			Expect(os.WriteFile(path, captured, 0600)).To(Succeed())
			Expect(os.Chmod(path, owned.Mode().Perm())).To(Succeed())
			foreign, statErr := os.Lstat(path)
			Expect(statErr).NotTo(HaveOccurred())
			Expect(os.SameFile(owned, foreign)).To(BeFalse(), "equal bytes are in an actual different inode")
		case "mode":
			Expect(os.Chmod(path, 0700)).To(Succeed())
		case "deleted":
			Expect(os.Remove(path)).To(Succeed())
		case "link":
			Expect(os.Rename(path, path+".retained-owned")).To(Succeed())
			Expect(os.Symlink(path+".retained-owned", path)).To(Succeed())
		case "ancestry":
			Expect(os.Rename(filepath.Join(root, "scripts"), filepath.Join(root, "retained-owned-scripts"))).To(Succeed())
			Expect(os.Mkdir(filepath.Join(root, "scripts"), 0700)).To(Succeed())
			Expect(os.WriteFile(path, []byte("PRIVATE_FOREIGN_ANCESTRY"), 0600)).To(Succeed())
		case "saved":
			Expect(os.WriteFile(savedPath, []byte("PRIVATE_CHANGED_SAVED_INPUT"), 0600)).To(Succeed())
		case "pending":
			marker := filepath.Join(root, ".factory/runtime-publication.pending")
			Expect(os.Rename(marker, marker+".retained-owned")).To(Succeed())
			file, createErr := os.OpenFile(marker, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
			Expect(createErr).NotTo(HaveOccurred())
			Expect(file.Sync()).To(Succeed())
			Expect(file.Close()).To(Succeed())
			pending = publicationFaultPending(root)
		}
		before, beforeErr := os.Lstat(path)
		var beforeBytes []byte
		var beforeLink string
		if beforeErr == nil {
			if before.Mode().IsRegular() {
				beforeBytes = publicationFaultBytes(root, request.Path)
			}
			if before.Mode()&os.ModeSymlink != 0 {
				beforeLink, err = os.Readlink(path)
				Expect(err).NotTo(HaveOccurred())
			}
		} else {
			Expect(errors.Is(beforeErr, os.ErrNotExist)).To(BeTrue())
		}
		savedBefore, err := os.ReadFile(savedPath)
		Expect(err).NotTo(HaveOccurred())
		renameCount := witness.renames
		retry := operation.Restore(context.Background())
		publicationFaultTyped(root, retry)
		Expect(witness.renames).To(Equal(renameCount), "conflict cannot authorize any compensation or extra reverse rename")
		assertPreserved := func() {
			after, afterErr := os.Lstat(path)
			if errors.Is(beforeErr, os.ErrNotExist) {
				Expect(errors.Is(afterErr, os.ErrNotExist)).To(BeTrue())
			} else {
				Expect(afterErr).NotTo(HaveOccurred())
				Expect(os.SameFile(before, after)).To(BeTrue())
				Expect(after.Mode()).To(Equal(before.Mode()))
				if before.Mode().IsRegular() {
					Expect(publicationFaultBytes(root, request.Path)).To(Equal(beforeBytes))
				}
				if before.Mode()&os.ModeSymlink != 0 {
					target, linkErr := os.Readlink(path)
					Expect(linkErr).NotTo(HaveOccurred())
					Expect(target).To(Equal(beforeLink))
				}
			}
			savedAfter, readErr := os.ReadFile(savedPath)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(savedAfter).To(Equal(savedBefore))
			publicationFaultPreserved(root, pending)
		}
		assertPreserved()
		publicationFaultTyped(root, operation.Close(context.Background()))
		assertPreserved()
		publicationObservationReleased(root, *files)
		Expect(ErrorStatus(retry)).To(Equal(2))
	}, Entry("forward later edit", false, "edit"), Entry("reverse later edit", true, "edit"),
		Entry("forward equal bytes in foreign inode", false, "equal inode replacement"), Entry("reverse equal bytes in foreign inode", true, "equal inode replacement"),
		Entry("forward complete mode changed", false, "mode"), Entry("reverse complete mode changed", true, "mode"),
		Entry("forward deletion", false, "deleted"), Entry("reverse deletion", true, "deleted"),
		Entry("forward symlink", false, "link"), Entry("reverse symlink", true, "link"),
		Entry("forward foreign installed parent", false, "ancestry"), Entry("reverse foreign installed parent", true, "ancestry"),
		Entry("forward saved input changed", false, "saved"), Entry("reverse saved input changed", true, "saved"),
		Entry("forward pending inode changed", false, "pending"), Entry("reverse pending inode changed", true, "pending"))
})
