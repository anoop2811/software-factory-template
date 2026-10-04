package assessment

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

type durableReviewFile struct {
	info  os.FileInfo
	bytes []byte
}

// This stable test-owned tree is observed before and after the synchronous
// inspector; no untrusted tree or concurrent writer participates.
func durableReviewSnapshot(root string) map[string]durableReviewFile {
	GinkgoHelper()
	result := map[string]durableReviewFile{}
	Expect(filepath.WalkDir(root, func(path string, _ os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		state := durableReviewFile{info: info}
		if info.Mode().IsRegular() {
			state.bytes, err = os.ReadFile(path) // #nosec G122 -- stable private fixture, no concurrent mutation or external path input.
			if err != nil {
				return err
			}
		}
		result[relative] = state
		return nil
	})).To(Succeed())
	return result
}

func durableReviewPreserved(root string, before map[string]durableReviewFile, changed string, expectedMode os.FileMode, pendingAdded bool) {
	GinkgoHelper()
	after := durableReviewSnapshot(root)
	expectedCount := len(before)
	if pendingAdded {
		expectedCount++
	}
	Expect(after).To(HaveLen(expectedCount))
	for path, original := range before {
		actual, present := after[path]
		Expect(present).To(BeTrue(), "%s was removed by an inert inspector", path)
		Expect(os.SameFile(original.info, actual.info)).To(BeTrue(), "%s inode changed", path)
		Expect(actual.bytes).To(Equal(original.bytes), "%s payload changed", path)
		if !pendingAdded || path != ".factory" {
			Expect(actual.info.ModTime()).To(Equal(original.info.ModTime()), "%s modification time changed", path)
		}
		mode := original.info.Mode()
		if path == changed {
			mode = mode&^os.ModePerm | expectedMode
		}
		Expect(actual.info.Mode()).To(Equal(mode), "%s complete mode changed outside the evaluator mutation", path)
	}
	if pendingAdded {
		_ = publicationFaultPending(root)
	}
}

func durableReviewTerminalFixture() (string, string) {
	GinkgoHelper()
	root, request, environment := durablePublicationOwnedFixture()
	operation, err := BeginDurablePublication(context.Background(), root, request, environment)
	Expect(err).NotTo(HaveOccurred())
	Expect(operation.Restore(context.Background())).To(Succeed())
	Expect(operation.Close(context.Background())).To(Succeed())
	return root, filepath.Join(root, ".factory/backups/.publications", request.MigrationID+".json")
}

func durableReviewCapture(files *[]*os.File, file *os.File) {
	if file == nil {
		return
	}
	for _, prior := range *files {
		if prior == file {
			return
		}
	}
	*files = append(*files, file)
}

func durableReviewPublicationInert(inventory PublicationInventory) {
	GinkgoHelper()
	Expect(inventory.Restorable).To(BeFalse())
	Expect(inventory.RollbackReady).To(BeFalse())
	Expect(inventory.ActivationReady).To(BeFalse())
	Expect(inventory.Applicable).To(BeFalse())
	Expect(inventory.PruneAuthorized).To(BeFalse())
}

var _ = Describe("Durable publication review final metadata", func() {
	for _, inspector := range []string{"publications", "recovery"} {
		// per docs/adr/0095-durable-live-publication.md:265
		// per docs/adr/0095-durable-live-publication.md:280
		// per docs/adr/0095-durable-live-publication.md:34
		body := func(target string, reportedIO bool) {
			root, slot := durableReviewTerminalFixture()
			baseline, err := InspectPublications(context.Background(), root)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseline.Status()).To(BeZero())
			Expect(baseline.Complete).To(BeTrue())
			Expect(baseline.Records).To(HaveLen(1))
			Expect(baseline.Records[0].Classification).To(Equal("record_checked"))
			before := durableReviewSnapshot(root)
			recordBytes := len(before[filepath.Join(".factory/backups/.publications", filepath.Base(slot))].bytes)
			var files []*os.File
			readBytes := 0
			reachedEOF, faulted := false, false
			changed := ""
			expectedMode := os.FileMode(0)
			pendingAdded := false
			controls := ops{
				open: func(parent *os.File, name string, flags int) (*os.File, error) {
					durableReviewCapture(&files, parent)
					file, openErr := openAt(parent, name, flags)
					durableReviewCapture(&files, file)
					return file, openErr
				},
				read: func(file *os.File, buffer []byte) (int, error) {
					n, readErr := file.Read(buffer)
					if publicationSame(file, slot) {
						Expect(readErr == nil || errors.Is(readErr, io.EOF)).To(BeTrue(), "payload reading succeeds before any injected final error")
						readBytes += n
						if errors.Is(readErr, io.EOF) {
							reachedEOF = true
						}
					}
					return n, readErr
				},
				descriptor: func(file *os.File) (unix.Stat_t, error) {
					durableReviewCapture(&files, file)
					stat, statErr := descriptor(file)
					selected := target == "root descriptor" && publicationSame(file, root) || target == "record descriptor" && publicationSame(file, slot)
					if reachedEOF && selected && !faulted {
						Expect(statErr).NotTo(HaveOccurred(), "the real final Fstat succeeds before the evaluator reports EIO or changes mode")
						faulted = true
						if reportedIO {
							return stat, unix.EIO
						}
						path := slot
						expectedMode = 0644
						changed, _ = filepath.Rel(root, slot)
						if target == "root descriptor" {
							path, changed, expectedMode = root, ".", 0755
						}
						Expect(os.Chmod(path, expectedMode)).To(Succeed())
						return descriptor(file)
					}
					return stat, statErr
				},
				named: func(parent *os.File, name string) (unix.Stat_t, error) {
					durableReviewCapture(&files, parent)
					stat, statErr := named(parent, name)
					selected := target == "root named" && name == filepath.Base(root) && publicationSame(parent, filepath.Dir(root)) ||
						target == "record named" && name == filepath.Base(slot) && publicationSame(parent, filepath.Dir(slot)) ||
						target == "missing pending" && name == publicationPending && publicationSame(parent, filepath.Join(root, ".factory"))
					if reachedEOF && selected && !faulted {
						if target == "missing pending" {
							Expect(errors.Is(statErr, unix.ENOENT)).To(BeTrue(), "the actual final Fstatat still observes the pinned absence")
						} else {
							Expect(statErr).NotTo(HaveOccurred(), "the real final Fstatat succeeds before the injected report or metadata change")
						}
						faulted = true
						if reportedIO {
							return stat, unix.EIO
						}
						if target == "missing pending" {
							Expect(os.WriteFile(filepath.Join(root, ".factory", publicationPending), nil, 0600)).To(Succeed())
							pendingAdded = true
						} else {
							path := slot
							expectedMode = 0644
							changed, _ = filepath.Rel(root, slot)
							if target == "root named" {
								path, changed, expectedMode = root, ".", 0755
							}
							Expect(os.Chmod(path, expectedMode)).To(Succeed())
						}
						return named(parent, name)
					}
					return stat, statErr
				},
			}
			actualStatus, actualRoot := 0, ""
			if inspector == "publications" {
				inventory, inspectErr := inspectPublications(context.Background(), root, controls)
				Expect(inspectErr).NotTo(HaveOccurred())
				durableReviewPublicationInert(inventory)
				Expect(inventory.Complete).To(BeFalse())
				Expect(inventory.Records).To(BeEmpty())
				Expect(inventory.RecordCount).To(BeZero())
				actualStatus, actualRoot = inventory.Status(), inventory.RootStatus
			} else {
				inventory, inspectErr := inspectRecovery(context.Background(), root, controls)
				Expect(inspectErr).NotTo(HaveOccurred())
				Expect(inventory.Restorable).To(BeFalse())
				Expect(inventory.PruneAuthorized).To(BeFalse())
				Expect(inventory.Complete).To(BeFalse())
				Expect(inventory.Sets).To(BeEmpty())
				Expect(inventory.SetCount).To(BeZero())
				Expect(inventory.FileCount).To(BeZero())
				Expect(inventory.Bytes).To(BeZero())
				actualStatus, actualRoot = inventory.Status(), inventory.RootStatus
			}
			Expect(reachedEOF).To(BeTrue())
			Expect(readBytes).To(Equal(recordBytes), "all actual record bytes and EOF precede the final stat fault")
			Expect(faulted).To(BeTrue(), "the selected actual final stat boundary must be reached")
			durableReviewPreserved(root, before, changed, expectedMode, pendingAdded)
			recoveryClosed(files)
			durablePublicationNoExecution(root)
			expectedStatus, expectedRoot := 2, "unsafe"
			if reportedIO {
				expectedStatus, expectedRoot = 1, "assessment_error"
			}
			Expect(actualStatus).To(Equal(expectedStatus), "a real final stat followed by injected EIO must remain operational, whereas an actual mode/absence conflict is a refusal")
			Expect(actualRoot).To(Equal(expectedRoot))
		}
		// per docs/adr/0095-durable-live-publication.md:265
		// per docs/adr/0095-durable-live-publication.md:280
		DescribeTable(inspector+" classifies a first final stat error separately from an actual metadata conflict after successful record reads", body,
			Entry("root descriptor actual Fstat then injected EIO", "root descriptor", true),
			Entry("root descriptor actual mode conflict", "root descriptor", false),
			Entry("root named actual Fstatat then injected EIO", "root named", true),
			Entry("root named actual mode conflict", "root named", false),
			Entry("record descriptor actual Fstat then injected EIO", "record descriptor", true),
			Entry("record descriptor actual mode conflict", "record descriptor", false),
			Entry("record named actual Fstatat then injected EIO", "record named", true),
			Entry("record named actual mode conflict", "record named", false))
		if inspector == "publications" {
			// per docs/adr/0095-durable-live-publication.md:269
			// per docs/adr/0095-durable-live-publication.md:280
			DescribeTable("publications distinguishes a pinned absence conflict from final metadata I/O", body,
				Entry("missing pending actual ENOENT then injected EIO", "missing pending", true),
				Entry("missing pending newly occupied conflict", "missing pending", false))
		}
	}
})

var _ = Describe("Durable publication review shared recovery precedence", func() {
	// per docs/adr/0095-durable-live-publication.md:273
	// per docs/adr/0095-durable-live-publication.md:280
	// per docs/adr/0083-go-recovery-set-inspection.md:90
	DescribeTable("keeps actual reserved-record read errors operational after metadata-only invalidation", func(change, reportedIO bool) {
		root, slot := durableReviewTerminalFixture()
		baseline, err := InspectRecovery(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		Expect(baseline.Status()).To(BeZero())
		Expect(baseline.Complete).To(BeTrue())
		Expect(baseline.Sets).To(HaveLen(1))
		Expect(baseline.Sets[0].Classification).To(Equal("integrity_checked"))
		before := durableReviewSnapshot(root)
		var files []*os.File
		reached := false
		controls := ops{
			open: func(parent *os.File, name string, flags int) (*os.File, error) {
				durableReviewCapture(&files, parent)
				file, openErr := openAt(parent, name, flags)
				durableReviewCapture(&files, file)
				return file, openErr
			},
			descriptor: func(file *os.File) (unix.Stat_t, error) {
				durableReviewCapture(&files, file)
				return descriptor(file)
			},
			read: func(file *os.File, buffer []byte) (int, error) {
				n, readErr := file.Read(buffer)
				if !reached && n > 0 && publicationSame(file, slot) {
					Expect(readErr).NotTo(HaveOccurred(), "the actual reserved record supplies bytes before any reported I/O error")
					reached = true
					if change {
						Expect(file.Chmod(0644)).To(Succeed())
					}
					if reportedIO {
						return n, unix.EIO
					}
				}
				return n, readErr
			},
		}
		inventory, err := inspectRecovery(context.Background(), root, controls)
		Expect(err).NotTo(HaveOccurred())
		Expect(reached).To(BeTrue())
		changed, expectedMode := "", os.FileMode(0)
		if change {
			changed, _ = filepath.Rel(root, slot)
			expectedMode = 0644
		}
		durableReviewPreserved(root, before, changed, expectedMode, false)
		recoveryClosed(files)
		durablePublicationNoExecution(root)
		Expect(inventory.Restorable).To(BeFalse())
		Expect(inventory.PruneAuthorized).To(BeFalse())
		if change || reportedIO {
			Expect(inventory.Complete).To(BeFalse())
			Expect(inventory.Sets).To(BeEmpty())
			Expect(inventory.SetCount).To(BeZero())
			Expect(inventory.FileCount).To(BeZero())
			Expect(inventory.Bytes).To(BeZero())
		}
		expectedStatus, expectedRoot := 0, "inspected"
		if change {
			expectedStatus, expectedRoot = 2, "unsafe"
		}
		if reportedIO {
			expectedStatus, expectedRoot = 1, "assessment_error"
		}
		Expect(inventory.Status()).To(Equal(expectedStatus), "a prior real read followed by injected EIO retains status 1 when unsafe observations are discarded")
		Expect(inventory.RootStatus).To(Equal(expectedRoot))
	}, Entry("unchanged successful read control", false, false),
		Entry("real record read then injected EIO", false, true),
		Entry("real record read plus mode mutation then injected EIO", true, true),
		Entry("real record read plus mode mutation only", true, false))
})
