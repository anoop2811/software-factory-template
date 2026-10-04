package acceptance_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/assessment"
	"github.com/anoop2811/software-factory-template/internal/transition"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func livePublicationFixture() (string, string, assessment.PublicationRequest) {
	return livePublicationSelectedFixture(assessmentPaths[:1], false)
}

func livePublicationSelectedFixture(selection []string, held bool) (string, string, assessment.PublicationRequest) {
	GinkgoHelper()
	binaryRoot, root, _ := durableRecoveryFixture()
	for _, role := range []string{"implementer", "reviewer"} {
		writeFixture(filepath.Join(root, ".opencode/agent", role+".md"), []byte("---\nname: role\n---\nCanonical role instructions\n"), 0600)
		writeFixture(filepath.Join(root, ".claude/agents", role+".md"), []byte("Native instructions\n"), 0600)
	}
	writeFixture(filepath.Join(root, "opencode.json"), []byte(`{"agent":{"reviewer":{"permission":{"edit":"deny"}},"implementer":{"permission":{"edit":"allow"}}}}`), 0600)
	writeFixture(filepath.Join(root, "prompt.txt"), []byte("PRIVATE_PUBLICATION_PROMPT\n"), 0600)
	writeFixture(filepath.Join(root, "factory.yaml"), []byte("budget_enabled: true\ncheck_command: printf checked > \"$TRANSITION_CHILD_MARKER\"\n"), 0600)
	writeFixture(filepath.Join(binaryRoot, "bin", "claude"), []byte("#!/bin/sh\nprintf probed > \"$TRANSITION_CHILD_MARKER\"\nprintf 'unsupported fixture help\\n'\n"), 0700)
	ctx := context.Background()
	proposal, err := assessment.ProposeAdoption(ctx, root, selection)
	Expect(err).NotTo(HaveOccurred())
	environment := map[string]string{}
	for _, entry := range durableGitEnvironment() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			environment[key] = value
		}
	}
	created, err := assessment.CreateRecovery(ctx, root, assessment.RecoveryRequest{
		MigrationID: durableRecoveryID, TargetRevision: strings.Repeat("a", 40),
		Paths: selection, Confirmation: proposal.ProposalDigest,
	}, environment)
	Expect(err).NotTo(HaveOccurred())
	Expect(created.Result).To(Equal("created"))
	Expect(created.Restorable).To(BeFalse())
	Expect(created.ActivationReady).To(BeFalse())
	if held {
		manifestPath := filepath.Join(root, ".factory/backups", durableRecoveryID, "manifest.json")
		data, readErr := os.ReadFile(manifestPath)
		Expect(readErr).NotTo(HaveOccurred())
		var manifest map[string]any
		Expect(json.Unmarshal(data, &manifest)).To(Succeed())
		manifest["held"] = true
		data, readErr = json.Marshal(manifest)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(os.WriteFile(manifestPath, append(data, '\n'), 0600)).To(Succeed())
	}
	proposal, err = assessment.ProposeAdoption(ctx, root, assessmentPaths[:1])
	Expect(err).NotTo(HaveOccurred())
	return binaryRoot, root, assessment.PublicationRequest{
		MigrationID: durableRecoveryID, Path: assessmentPaths[0],
		Confirmation: proposal.ProposalDigest, Replacement: []byte("#!/bin/sh\nprintf 'LIVE_PUBLICATION_REPLACEMENT\\n'\n"),
	}
}

func livePublicationBegin(root string, request assessment.PublicationRequest) *assessment.Publication {
	GinkgoHelper()
	operation, err := assessment.BeginPublication(context.Background(), root, request)
	Expect(err).NotTo(HaveOccurred())
	Expect(operation).NotTo(BeNil())
	DeferCleanup(func() { _ = operation.Close(context.Background()) })
	return operation
}

func livePublicationUntouched(root string, selected string) map[string]assessedFileState {
	GinkgoHelper()
	states := assessmentTree(root)
	result := map[string]assessedFileState{}
	for path, state := range states {
		if path == "factory.yaml" || path == "opencode.json" || path == ".factory/events.log" ||
			strings.HasPrefix(path, ".git/") || strings.HasPrefix(path, ".factory/backups/") ||
			(strings.HasPrefix(path, "scripts/") && !state.Mode.IsDir() && path != selected) {
			result[path] = state
		}
	}
	return result
}

func livePublicationNoExecutionState(root string) {
	GinkgoHelper()
	for _, name := range []string{"budget.json", "loops.json", "budget.lock", "loop.lock"} {
		_, err := os.Lstat(filepath.Join(root, ".factory", name))
		Expect(os.IsNotExist(err)).To(BeTrue(), "filesystem publication cannot create execution state: %s", name)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".factory/runtime-activity"))
	Expect(err).NotTo(HaveOccurred())
	Expect(entries).To(BeEmpty(), "exclusive publication creates no runtime participant markers")
}

var _ = Describe("Live publication reversible component core", func() {
	// per docs/adr/0094-live-publication-restoration.md:78
	// per docs/adr/0094-live-publication-restoration.md:94
	// per docs/adr/0094-live-publication-restoration.md:102
	It("publishes a new actual inode and restores only the captured original under retained exclusion", func() {
		_, root, request := livePublicationFixture()
		original := durableBytes(root, request.Path)
		before, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		untouched := livePublicationUntouched(root, request.Path)
		operation := livePublicationBegin(root, request)
		transitionLockBlocked(root)
		pending, err := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(err).NotTo(HaveOccurred())
		Expect(pending.Mode().IsRegular()).To(BeTrue())
		Expect(pending.Mode().Perm()).To(Equal(os.FileMode(0600)))
		Expect(pending.Size()).To(BeZero())
		qualified, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, qualified)).To(BeTrue())
		Expect(durableBytes(root, request.Path)).To(Equal(original))
		Expect(operation.Apply(context.Background())).To(Succeed())
		applied, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, applied)).To(BeFalse(), "publication must use the actual exclusive sibling inode")
		Expect(applied.Mode()).To(Equal(before.Mode()))
		Expect(durableBytes(root, request.Path)).To(Equal(request.Replacement))
		publicationPendingPreserved(root, pending)
		Expect(operation.Restore(context.Background())).To(Succeed())
		restored, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(restored.Mode()).To(Equal(before.Mode()))
		Expect(durableBytes(root, request.Path)).To(Equal(original))
		_, err = os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		transitionLockBlocked(root)
		Expect(livePublicationUntouched(root, request.Path)).To(Equal(untouched))
		livePublicationNoExecutionState(root)
		Expect(operation.Close(context.Background())).To(Succeed())
		guard, err := transition.Exclusive(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		Expect(guard.Close(context.Background(), true)).To(Succeed())
	})

	// per docs/adr/0094-live-publication-restoration.md:88
	// per docs/adr/0094-live-publication-restoration.md:108
	It("preserves an edited owned after-image and retains pending evidence on unsuccessful restoration and closure", func() {
		_, root, request := livePublicationFixture()
		untouched := livePublicationUntouched(root, request.Path)
		operation := livePublicationBegin(root, request)
		Expect(operation.Apply(context.Background())).To(Succeed())
		applied, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		pending, err := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(err).NotTo(HaveOccurred())
		edited := []byte("PRIVATE_OPERATOR_EDIT_AFTER_PUBLICATION\n")
		Expect(os.WriteFile(filepath.Join(root, request.Path), edited, 0600)).To(Succeed())
		Expect(operation.Restore(context.Background())).NotTo(Succeed())
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(applied, after)).To(BeTrue())
		Expect(durableBytes(root, request.Path)).To(Equal(edited))
		publicationPendingPreserved(root, pending)
		Expect(operation.Close(context.Background())).NotTo(Succeed())
		Expect(durableBytes(root, request.Path)).To(Equal(edited))
		publicationPendingPreserved(root, pending)
		Expect(livePublicationUntouched(root, request.Path)).To(Equal(untouched))
		livePublicationNoExecutionState(root)
	})

	// per docs/adr/0094-live-publication-restoration.md:108
	// per docs/adr/0094-live-publication-restoration.md:112
	It("retains unresolved publication after close and blocks actual native help and manual checks", func() {
		binaryRoot, root, request := livePublicationFixture()
		operation := livePublicationBegin(root, request)
		Expect(operation.Apply(context.Background())).To(Succeed())
		pending, err := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(err).NotTo(HaveOccurred())
		Expect(operation.Close(context.Background())).NotTo(Succeed())
		Expect(durableBytes(root, request.Path)).To(Equal(request.Replacement))
		publicationPendingPreserved(root, pending)
		for _, kind := range []string{"budget", "loop"} {
			out := transitionRun(binaryRoot, root, "", kind, false, transitionRunArgs(kind))
			_, err = os.Lstat(filepath.Join(binaryRoot, "child-started"))
			Expect(os.IsNotExist(err)).To(BeTrue(), "pending publication must block real execution: %+v", out)
			Expect(out.status).To(Equal(2), "%+v", out)
			Expect(out.stdout).To(BeEmpty())
			Expect(out.stderr).NotTo(BeEmpty())
			for _, name := range []string{"budget.json", "loops.json"} {
				_, err = os.Lstat(filepath.Join(root, ".factory", name))
				Expect(os.IsNotExist(err)).To(BeTrue())
			}
			publicationPendingPreserved(root, pending)
		}
	})
})

var _ = Describe("Live publication external qualification", func() {
	// per docs/adr/0094-live-publication-restoration.md:29
	// per docs/adr/0094-live-publication-restoration.md:30
	// per docs/adr/0094-live-publication-restoration.md:92
	DescribeTable("uses checked selected/full/held backups as unchanged inert before-images", func(full, held bool) {
		selection := assessmentPaths[:1]
		if full {
			selection = assessmentPaths
		}
		_, root, request := livePublicationSelectedFixture(selection, held)
		backup := assessmentTree(filepath.Join(root, ".factory/backups", durableRecoveryID))
		untouched := livePublicationUntouched(root, request.Path)
		original := durableBytes(root, request.Path)
		operation := livePublicationBegin(root, request)
		Expect(operation.Apply(context.Background())).To(Succeed())
		Expect(durableBytes(root, request.Path)).To(Equal(request.Replacement))
		Expect(operation.Restore(context.Background())).To(Succeed())
		Expect(operation.Close(context.Background())).To(Succeed())
		Expect(durableBytes(root, request.Path)).To(Equal(original))
		Expect(assessmentTree(filepath.Join(root, ".factory/backups", durableRecoveryID))).To(Equal(backup))
		Expect(livePublicationUntouched(root, request.Path)).To(Equal(untouched))
		livePublicationNoExecutionState(root)
	}, Entry("full unheld selection", true, false), Entry("subset held selection", false, true), Entry("full held selection", true, true))

	// per docs/adr/0094-live-publication-restoration.md:28
	It("copies replacement bytes before returning the opaque handle", func() {
		_, root, request := livePublicationFixture()
		expected := append([]byte(nil), request.Replacement...)
		operation := livePublicationBegin(root, request)
		for i := range request.Replacement {
			request.Replacement[i] = 'X'
		}
		Expect(operation.Apply(context.Background())).To(Succeed())
		Expect(durableBytes(root, request.Path)).To(Equal(expected))
		Expect(operation.Restore(context.Background())).To(Succeed())
		Expect(operation.Close(context.Background())).To(Succeed())
	})

	// per docs/adr/0094-live-publication-restoration.md:26
	// per docs/adr/0094-live-publication-restoration.md:30
	DescribeTable("refuses unqualified requests without publishing or pending ownership", func(change string) {
		_, root, request := livePublicationFixture()
		switch change {
		case "invalid identifier":
			request.MigrationID = "../escape"
		case "missing set":
			request.MigrationID = "not-created"
		case "unknown path":
			request.Path = "scripts/unknown.sh"
		case "traversal path":
			request.Path = "scripts/../scripts/factory-budget.sh"
		case "missing confirmation":
			request.Confirmation = ""
		case "wrong confirmation":
			request.Confirmation = strings.Repeat("0", 64)
		case "selection confirmation":
			proposal, err := assessment.ProposeAdoption(context.Background(), root, assessmentPaths)
			Expect(err).NotTo(HaveOccurred())
			request.Confirmation = proposal.ProposalDigest
		case "oversized replacement":
			request.Replacement = make([]byte, (1<<20)+1)
		case "customized original":
			Expect(os.WriteFile(filepath.Join(root, request.Path), []byte("PRIVATE_CUSTOMIZED_ORIGINAL"), 0600)).To(Succeed())
		case "absent original":
			Expect(os.Remove(filepath.Join(root, request.Path))).To(Succeed())
		case "saved payload changed":
			Expect(os.WriteFile(filepath.Join(root, ".factory/backups", durableRecoveryID, "files", request.Path), []byte("PRIVATE_CHANGED_BACKUP"), 0600)).To(Succeed())
		case "backup missing selected path":
			request.Path = assessmentPaths[1]
			proposal, err := assessment.ProposeAdoption(context.Background(), root, []string{request.Path})
			Expect(err).NotTo(HaveOccurred())
			request.Confirmation = proposal.ProposalDigest
		case "stale equal-content consent":
			path := filepath.Join(root, request.Path)
			data := durableBytes(root, request.Path)
			Expect(os.WriteFile(path, data, 0600)).To(Succeed())
		}
		before := livePublicationUntouched(root, "")
		operation, err := assessment.BeginPublication(context.Background(), root, request)
		if operation != nil {
			DeferCleanup(func() { _ = operation.Close(context.Background()) })
		}
		Expect(err).To(HaveOccurred())
		Expect(operation).To(BeNil())
		Expect(err.Error()).NotTo(ContainSubstring(root))
		Expect(err.Error()).NotTo(ContainSubstring("PRIVATE_"))
		Expect(livePublicationUntouched(root, "")).To(Equal(before))
		_, err = os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("invalid identifier", "invalid identifier"), Entry("missing set", "missing set"),
		Entry("unknown path", "unknown path"), Entry("traversal path", "traversal path"),
		Entry("missing confirmation", "missing confirmation"), Entry("wrong confirmation", "wrong confirmation"),
		Entry("multi-path consent is not single-path consent", "selection confirmation"),
		Entry("one byte over the asset bound", "oversized replacement"), Entry("customized original", "customized original"),
		Entry("absent original", "absent original"), Entry("changed saved payload", "saved payload changed"),
		Entry("set excludes selected path", "backup missing selected path"), Entry("consent predates equal-content write", "stale equal-content consent"))

	// per docs/adr/0094-live-publication-restoration.md:50
	// per docs/adr/0094-live-publication-restoration.md:51
	DescribeTable("refuses unsafe existing selected files without following or replacing them", func(kind string) {
		_, root, request := livePublicationFixture()
		path := filepath.Join(root, request.Path)
		original := durableBytes(root, request.Path)
		Expect(os.Remove(path)).To(Succeed())
		switch kind {
		case "symlink":
			outside := filepath.Join(root, "outside-original")
			writeFixture(outside, original, 0600)
			Expect(os.Symlink(outside, path)).To(Succeed())
		case "hard link":
			outside := filepath.Join(root, "outside-original")
			writeFixture(outside, original, 0600)
			Expect(os.Link(outside, path)).To(Succeed())
		case "FIFO":
			Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
		case "directory":
			Expect(os.Mkdir(path, 0700)).To(Succeed())
		}
		before, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		operation, err := assessment.BeginPublication(context.Background(), root, request)
		if operation != nil {
			DeferCleanup(func() { _ = operation.Close(context.Background()) })
		}
		Expect(err).To(HaveOccurred())
		Expect(operation).To(BeNil())
		after, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		Expect(after.Mode()).To(Equal(before.Mode()))
		_, err = os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("symlink", "symlink"), Entry("hard link", "hard link"), Entry("FIFO", "FIFO"), Entry("directory", "directory"))

	// per docs/adr/0094-live-publication-restoration.md:89
	// per docs/adr/0094-live-publication-restoration.md:91
	DescribeTable("preserves altered or replaced after-images rather than treating equal bytes as ownership", func(change string) {
		_, root, request := livePublicationFixture()
		operation := livePublicationBegin(root, request)
		Expect(operation.Apply(context.Background())).To(Succeed())
		path := filepath.Join(root, request.Path)
		pending, err := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(err).NotTo(HaveOccurred())
		switch change {
		case "equal bytes new inode":
			Expect(os.Rename(path, path+".owned-before-swap")).To(Succeed())
			writeFixture(path, request.Replacement, 0755)
		case "mode":
			Expect(os.Chmod(path, 0700)).To(Succeed())
		case "deleted":
			Expect(os.Remove(path)).To(Succeed())
		case "symlink":
			Expect(os.Rename(path, path+".owned-before-swap")).To(Succeed())
			Expect(os.Symlink(path+".owned-before-swap", path)).To(Succeed())
		case "backup bytes":
			Expect(os.WriteFile(filepath.Join(root, ".factory/backups", durableRecoveryID, "files", request.Path), []byte("PRIVATE_TAMPERED_ORIGINAL"), 0600)).To(Succeed())
		case "unsafe parent":
			Expect(os.Chmod(filepath.Dir(path), 0777)).To(Succeed())
		}
		before, beforeErr := os.Lstat(path)
		var bytesBefore []byte
		if beforeErr == nil && before.Mode().IsRegular() {
			bytesBefore = durableBytes(root, request.Path)
		}
		Expect(operation.Restore(context.Background())).NotTo(Succeed())
		after, afterErr := os.Lstat(path)
		if errors.Is(beforeErr, os.ErrNotExist) {
			Expect(errors.Is(afterErr, os.ErrNotExist)).To(BeTrue())
		} else {
			Expect(afterErr).NotTo(HaveOccurred())
			Expect(os.SameFile(before, after)).To(BeTrue())
			Expect(after.Mode()).To(Equal(before.Mode()))
			if before.Mode().IsRegular() {
				Expect(durableBytes(root, request.Path)).To(Equal(bytesBefore))
			}
		}
		publicationPendingPreserved(root, pending)
		Expect(operation.Close(context.Background())).NotTo(Succeed())
		publicationPendingPreserved(root, pending)
	}, Entry("equal bytes in a different inode", "equal bytes new inode"), Entry("changed complete mode", "mode"),
		Entry("deleted owned file", "deleted"), Entry("symlink replacement", "symlink"),
		Entry("changed saved original", "backup bytes"), Entry("unsafe installed ancestry", "unsafe parent"))

	// per docs/adr/0094-live-publication-restoration.md:82
	// per docs/adr/0094-live-publication-restoration.md:99
	It("supports checked prepared abort and refuses repeated or closed mutation phases", func() {
		_, root, request := livePublicationFixture()
		original := durableBytes(root, request.Path)
		operation := livePublicationBegin(root, request)
		Expect(operation.Restore(context.Background())).To(Succeed())
		Expect(durableBytes(root, request.Path)).To(Equal(original))
		Expect(operation.Apply(context.Background())).NotTo(Succeed())
		Expect(operation.Close(context.Background())).To(Succeed())
		Expect(operation.Apply(context.Background())).NotTo(Succeed())
		Expect(operation.Restore(context.Background())).NotTo(Succeed())
		Expect(durableBytes(root, request.Path)).To(Equal(original))
		next := livePublicationBegin(root, request)
		Expect(next.Apply(context.Background())).To(Succeed())
		applied, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(next.Apply(context.Background())).NotTo(Succeed())
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(applied, after)).To(BeTrue())
		Expect(durableBytes(root, request.Path)).To(Equal(request.Replacement))
		Expect(next.Restore(context.Background())).To(Succeed())
		Expect(next.Close(context.Background())).To(Succeed())
	})

	// per docs/adr/0094-live-publication-restoration.md:125
	DescribeTable("cancellation preserves current bytes and unresolved evidence until explicit checked recovery", func(phase string) {
		_, root, request := livePublicationFixture()
		canceled, cancel := context.WithCancel(context.Background())
		cancel()
		if phase == "begin" {
			before := livePublicationUntouched(root, "")
			operation, err := assessment.BeginPublication(canceled, root, request)
			Expect(err).To(HaveOccurred())
			Expect(operation).To(BeNil())
			Expect(livePublicationUntouched(root, "")).To(Equal(before))
			return
		}
		operation := livePublicationBegin(root, request)
		original := durableBytes(root, request.Path)
		expected := original
		if phase == "restore" || phase == "close" {
			Expect(operation.Apply(context.Background())).To(Succeed())
			expected = request.Replacement
		}
		pending, err := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(err).NotTo(HaveOccurred())
		switch phase {
		case "apply":
			err = operation.Apply(canceled)
		case "restore":
			err = operation.Restore(canceled)
		case "close":
			err = operation.Close(canceled)
		}
		Expect(err).To(HaveOccurred())
		Expect(durableBytes(root, request.Path)).To(Equal(expected))
		publicationPendingPreserved(root, pending)
		if phase != "close" {
			Expect(operation.Restore(context.Background())).To(Succeed())
			Expect(durableBytes(root, request.Path)).To(Equal(original))
			Expect(operation.Close(context.Background())).To(Succeed())
		}
	}, Entry("begin", "begin"), Entry("apply", "apply"), Entry("restore", "restore"), Entry("close releases but never rolls back", "close"))
})

var _ = Describe("Live publication namespace and isolated execution", func() {
	// per docs/adr/0094-live-publication-restoration.md:50
	// per docs/adr/0094-live-publication-restoration.md:90
	DescribeTable("refuses actual root or selected-parent replacement while preserving the new namespace", func(change string) {
		_, root, request := livePublicationFixture()
		operation := livePublicationBegin(root, request)
		Expect(operation.Apply(context.Background())).To(Succeed())
		pending, err := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(err).NotTo(HaveOccurred())
		pendingRoot := root
		if change == "root" {
			pendingRoot = root + ".retained-root"
			Expect(os.Rename(root, pendingRoot)).To(Succeed())
			DeferCleanup(os.RemoveAll, pendingRoot)
			Expect(os.Mkdir(root, 0700)).To(Succeed())
		} else {
			Expect(os.Rename(filepath.Join(root, "scripts"), filepath.Join(root, "retained-scripts"))).To(Succeed())
		}
		foreign := []byte("PRIVATE_FOREIGN_NAMESPACE\n")
		writeFixture(filepath.Join(root, request.Path), foreign, 0600)
		before, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(operation.Restore(context.Background())).NotTo(Succeed())
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		Expect(durableBytes(root, request.Path)).To(Equal(foreign))
		publicationPendingPreserved(pendingRoot, pending)
		Expect(operation.Close(context.Background())).NotTo(Succeed())
		publicationPendingPreserved(pendingRoot, pending)
	}, Entry("physical project root", "root"), Entry("selected ancestry", "parent"))

	// per docs/adr/0094-live-publication-restoration.md:103
	// per docs/adr/0094-live-publication-restoration.md:106
	DescribeTable("preserves foreign pending entries rather than clearing replacement evidence", func(link bool) {
		_, root, request := livePublicationFixture()
		operation := livePublicationBegin(root, request)
		Expect(operation.Apply(context.Background())).To(Succeed())
		pending := filepath.Join(root, ".factory/runtime-publication.pending")
		Expect(os.Rename(pending, pending+".owned-old")).To(Succeed())
		if link {
			writeFixture(filepath.Join(root, "foreign-evidence"), []byte("PRIVATE_FOREIGN_EVIDENCE"), 0600)
			Expect(os.Symlink(filepath.Join(root, "foreign-evidence"), pending)).To(Succeed())
		} else {
			writeFixture(pending, []byte("PRIVATE_FOREIGN_EVIDENCE"), 0600)
		}
		before, err := os.Lstat(pending)
		Expect(err).NotTo(HaveOccurred())
		selected, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		selectedBytes := durableBytes(root, request.Path)
		Expect(operation.Restore(context.Background())).NotTo(Succeed())
		selectedAfter, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(selected, selectedAfter)).To(BeTrue())
		Expect(selectedAfter.Mode()).To(Equal(selected.Mode()))
		Expect(durableBytes(root, request.Path)).To(Equal(selectedBytes))
		Expect(operation.Close(context.Background())).NotTo(Succeed())
		after, err := os.Lstat(pending)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		Expect(after.Mode()).To(Equal(before.Mode()))
		if !link {
			Expect(durableBytes(root, ".factory/runtime-publication.pending")).To(Equal([]byte("PRIVATE_FOREIGN_EVIDENCE")))
		}
	}, Entry("different regular inode", false), Entry("foreign link", true))

	// per docs/adr/0094-live-publication-restoration.md:72
	DescribeTable("accepts the actual bounded replacement byte range and restores the mode", func(size int) {
		_, root, request := livePublicationFixture()
		request.Replacement = make([]byte, size)
		operation := livePublicationBegin(root, request)
		Expect(operation.Apply(context.Background())).To(Succeed())
		Expect(durableBytes(root, request.Path)).To(Equal(request.Replacement))
		Expect(operation.Restore(context.Background())).To(Succeed())
		Expect(operation.Close(context.Background())).To(Succeed())
	}, Entry("empty replacement bytes", 0), Entry("exact one MiB asset bound", 1<<20))

	// per docs/adr/0094-live-publication-restoration.md:32
	// per docs/adr/0094-live-publication-restoration.md:165
	It("performs no Git, native, model or other subprocess after the checked fixture exists", func() {
		binaryRoot, root, request := livePublicationFixture()
		marker := filepath.Join(binaryRoot, "SUBPROCESS_CALLED")
		poison := filepath.Join(binaryRoot, "poison")
		for _, name := range []string{"git", "bash", "python3", "claude", "codex", "opencode", "sh"} {
			writeFixture(filepath.Join(poison, name), []byte("#!/bin/sh\nprintf called > '"+marker+"'\nexit 99\n"), 0700)
		}
		oldPath := os.Getenv("PATH")
		Expect(os.Setenv("PATH", poison)).To(Succeed())
		DeferCleanup(os.Setenv, "PATH", oldPath)
		untouched := livePublicationUntouched(root, request.Path)
		operation := livePublicationBegin(root, request)
		Expect(operation.Apply(context.Background())).To(Succeed())
		Expect(operation.Restore(context.Background())).To(Succeed())
		Expect(operation.Close(context.Background())).To(Succeed())
		_, err := os.Lstat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(livePublicationUntouched(root, request.Path)).To(Equal(untouched))
		livePublicationNoExecutionState(root)
	})
})

var _ = Describe("Live publication opaque handle ownership", func() {
	// per docs/adr/0094-live-publication-restoration.md:35
	// per docs/adr/0094-live-publication-restoration.md:88
	// per docs/adr/0094-live-publication-restoration.md:102
	// per docs/adr/0094-live-publication-restoration.md:197
	It("cannot clear pending from a pre-apply value copy while the actual named replacement remains", func() {
		_, root, request := livePublicationFixture()
		original := durableBytes(root, request.Path)
		before, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		operation := livePublicationBegin(root, request)
		// This copy occurs before either handle calls a lifecycle method.
		// It is an ordinary exported Go value copy, neither an imported receipt nor
		// changes private phase, identity or ownership fields.
		alias := *operation
		Expect(operation.Apply(context.Background())).To(Succeed())
		applied, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, applied)).To(BeFalse())
		Expect(durableBytes(root, request.Path)).To(Equal(request.Replacement))
		pending, err := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(err).NotTo(HaveOccurred())
		publicationPendingPreserved(root, pending)
		restoreErr := alias.Restore(context.Background())
		Expect(restoreErr).NotTo(HaveOccurred(), "a legitimate value alias shares the same live ownership and phase")
		_, pendingErr := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(pendingErr)).To(BeTrue(), "the copied handle reported checked completion and removed pending")
		active := durableBytes(root, request.Path)
		GinkgoWriter.Printf("copied restore succeeded; pending_absent=%t; named_replacement=%t; actual_original_restored=%t\n", os.IsNotExist(pendingErr), string(active) == string(request.Replacement), string(active) == string(original))
		Expect(active).To(Equal(original), "successful checked restoration cannot read only a detached original and clear the restart barrier")
		restored, statErr := os.Lstat(filepath.Join(root, request.Path))
		Expect(statErr).NotTo(HaveOccurred())
		Expect(restored.Mode()).To(Equal(before.Mode()))
		Expect(operation.Close(context.Background())).To(Succeed())
		// Closing the original closes this one shared operation; the alias
		// cannot close resources again or recover an independent lifecycle.
		Expect(alias.Close(context.Background())).To(HaveOccurred())
		Expect(alias.Apply(context.Background())).To(HaveOccurred())
		Expect(alias.Restore(context.Background())).To(HaveOccurred())
		guard, err := transition.Exclusive(context.Background(), root)
		Expect(err).NotTo(HaveOccurred(), "fresh work is admitted only after the actual before-image is restored")
		Expect(guard.Close(context.Background(), true)).To(Succeed())
	})
})

var _ = Describe("Live publication unowned handle refusal", func() {
	// per docs/adr/0094-live-publication-restoration.md:201
	DescribeTable("refuses nil and zero-state lifecycle handles rather than manufacturing a successful operation", func(zero bool, phase string) {
		var operation *assessment.Publication
		if zero {
			operation = new(assessment.Publication)
		}
		var err error
		Expect(func() {
			switch phase {
			case "apply":
				err = operation.Apply(context.Background())
			case "restore":
				err = operation.Restore(context.Background())
			case "close":
				err = operation.Close(context.Background())
			}
		}).NotTo(Panic())
		Expect(err).To(HaveOccurred(), "an unowned public zero/nil handle cannot report successful lifecycle completion")
		Expect(assessment.ErrorStatus(err)).To(Equal(2))
		Expect(err.Error()).NotTo(ContainSubstring("PRIVATE_"))
	}, Entry("nil apply", false, "apply"), Entry("nil restore", false, "restore"), Entry("nil close", false, "close"),
		Entry("zero apply", true, "apply"), Entry("zero restore", true, "restore"), Entry("zero close", true, "close"))
})
