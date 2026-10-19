package installation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/staging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type pruneComponentFixture struct {
	root     string
	tree     *installationfs.Tree
	snapshot retentionSnapshot
}

// This is an authorized-caller deletion component, not an authenticated release
// or independent recovery qualification. It uses real saved files and custody.
// per docs/adr/0106-go-backup-retention.md:102
func newPruneComponentFixture() *pruneComponentFixture {
	GinkgoHelper()
	ctx := context.Background()
	root, err := filepath.EvalSymlinks(GinkgoT().TempDir())
	Expect(err).NotTo(HaveOccurred())
	fixture := &pruneComponentFixture{root: root}
	id := "component-older"
	entries := make([]journalEntry, 0, len(roles))
	before := map[string]*Image{}
	for _, path := range []string{"factory", "scripts/factory-report.sh"} {
		body := []byte("saved predecessor " + path + "\n")
		active := filepath.Join(root, filepath.FromSlash(path))
		Expect(os.MkdirAll(filepath.Dir(active), 0700)).To(Succeed())
		Expect(os.WriteFile(active, body, 0600)).To(Succeed())
		Expect(os.Chmod(active, 0700)).To(Succeed()) // #nosec G302 -- test-owned active executable with owner-only permissions.
		saved := filepath.Join(root, filepath.FromSlash(backupBase(id)+"/assets/"+path))
		Expect(os.MkdirAll(filepath.Dir(saved), 0700)).To(Succeed())
		Expect(os.WriteFile(saved, body, 0600)).To(Succeed())
	}
	fixture.tree, err = installationfs.Open(ctx, root)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { Expect(fixture.tree.Close()).To(Succeed()) })
	for _, path := range []string{"factory", "scripts/factory-report.sh"} {
		before[path], _, err = observe(ctx, fixture.tree, path, 1<<20)
		Expect(err).NotTo(HaveOccurred())
	}
	for _, row := range roles {
		entry := journalEntry{Path: row.Path, Action: "retain", Phase: "applied"}
		if original := before[row.Path]; original != nil {
			entry.Action, entry.Before = "replace", original
			entry.ExpectedAfter = bytesImage([]byte("qualified successor "+row.Path+"\n"), 0700)
		}
		entries = append(entries, entry)
	}
	record := journal{SchemaVersion: 1, Kind: "installation-transaction", MigrationID: id, Purpose: "upgrade", Direction: "forward", Phase: "committed", Profile: "go-hybrid-v1", Target: runtime.GOOS + "/" + runtime.GOARCH, PlanDigest: strings.Repeat("a", 64), Entries: entries, Checks: []checkRecord{}, Outcome: "completed"}
	for path, value := range map[string]any{
		backupBase(id) + "/installation.manifest": backupManifest{1, "installation-before-images", id, record.Profile, entries},
		journalPath(id): record,
	} {
		Expect(writeRecord(ctx, fixture.tree, path, value, nil)).To(Succeed())
	}
	fixture.snapshot, err = snapshotRetentionSet(ctx, fixture.tree, id)
	Expect(err).NotTo(HaveOccurred())
	Expect(fixture.snapshot.Files).To(HaveLen(3))
	return fixture
}

func (fixture *pruneComponentFixture) reopen() {
	GinkgoHelper()
	Expect(fixture.tree.Close()).To(Succeed())
	var err error
	fixture.tree, err = installationfs.Open(context.Background(), fixture.root)
	Expect(err).NotTo(HaveOccurred())
}

func (fixture *pruneComponentFixture) absolute(path string) string {
	return filepath.Join(fixture.root, filepath.FromSlash(path))
}

func (fixture *pruneComponentFixture) progress() retentionProgress {
	GinkgoHelper()
	progress, err := readRetentionProgress(context.Background(), fixture.tree)
	Expect(err).NotTo(HaveOccurred())
	return progress
}

func (fixture *pruneComponentFixture) interruptAfterUnlink() retentionProgress {
	GinkgoHelper()
	interrupted := errors.New("test interruption after unlink before parent sync")
	calls := 0
	_, _, err := executeRetentionPrune(context.Background(), fixture.tree, fixture.snapshot, "component-newer", retentionPruneOps{afterUnlink: func(_ context.Context, path string) error {
		calls++
		_, inspectErr := os.Lstat(fixture.absolute(path))
		Expect(os.IsNotExist(inspectErr)).To(BeTrue(), "the interruption must follow a real unlink")
		return interrupted
	}})
	Expect(errors.Is(err, interrupted)).To(BeTrue(), "%v", err)
	Expect(calls).To(Equal(1))
	fixture.reopen()
	progress := fixture.progress()
	Expect(progress.Next).To(Equal(0))
	Expect(progress.Intent).To(BeTrue())
	Expect(progress.Phase).To(Equal("pruning"))
	return progress
}

var _ = Describe("Installation backup prune interruption component", func() {
	// per docs/adr/0106-go-backup-retention.md:90
	It("resumes an actual unlink before parent sync without double counting repeated intended absence", func() {
		fixture := newPruneComponentFixture()
		progress := fixture.interruptAfterUnlink()
		first := progress.Rows[0]
		Expect(first.Path).To(ContainSubstring("/assets/"))
		Expect(validateRemainingRetention(context.Background(), fixture.tree, progress)).To(Succeed())

		interrupted := errors.New("second interruption after reconciling intended absence")
		files, size, err := executeRetentionPrune(context.Background(), fixture.tree, fixture.snapshot, "component-newer", retentionPruneOps{afterStep: func(_ context.Context, path string) error {
			Expect(path).To(Equal(first.Path))
			return interrupted
		}})
		Expect(errors.Is(err, interrupted)).To(BeTrue())
		Expect(files).To(BeZero())
		Expect(size).To(BeZero())
		fixture.reopen()
		progress = fixture.progress()
		Expect(progress.Next).To(Equal(0))
		Expect(progress.Intent).To(BeTrue())
		Expect(validateRemainingRetention(context.Background(), fixture.tree, progress)).To(Succeed())

		totalFiles, totalBytes := snapshotCounts(fixture.snapshot)
		files, size, err = executeRetentionPrune(context.Background(), fixture.tree, fixture.snapshot, "component-newer", retentionPruneOps{})
		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(Equal(totalFiles - 1))
		Expect(size).To(Equal(totalBytes - first.Image.Bytes))
		for _, path := range []string{backupBase(fixture.snapshot.ID), journalPath(fixture.snapshot.ID)} {
			_, inspectErr := os.Lstat(fixture.absolute(path))
			Expect(os.IsNotExist(inspectErr)).To(BeTrue(), path)
		}
		fixture.reopen()
		completed := fixture.progress()
		Expect(completed.Phase).To(Equal("completed"))
		Expect(completed.Next).To(Equal(len(completed.Rows)))
		Expect(completed.Intent).To(BeFalse())
		Expect(validateRemainingRetention(context.Background(), fixture.tree, completed)).To(Succeed())
		for _, path := range []string{"factory", "scripts/factory-report.sh"} {
			body, readErr := os.ReadFile(fixture.absolute(path))
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal("saved predecessor " + path + "\n"))
		}
	})

	// per docs/adr/0106-go-backup-retention.md:93
	DescribeTable("refuses changed recovery contents after interruption without removing the replacement", func(change string) {
		fixture := newPruneComponentFixture()
		progress := fixture.interruptAfterUnlink()
		path := progress.Rows[1].Path
		want := []byte("replacement must survive")
		switch change {
		case "edited remaining file":
			Expect(os.WriteFile(fixture.absolute(path), want, 0600)).To(Succeed())
		case "reoccupied intended absence":
			path = progress.Rows[0].Path
			Expect(os.WriteFile(fixture.absolute(path), want, 0600)).To(Succeed())
		case "unknown regular entry":
			path = backupBase(fixture.snapshot.ID) + "/unclassified"
			Expect(os.WriteFile(fixture.absolute(path), want, 0600)).To(Succeed())
		case "unknown empty directory":
			path = backupBase(fixture.snapshot.ID) + "/unclassified"
			Expect(os.Mkdir(fixture.absolute(path), 0700)).To(Succeed())
		}
		fixture.reopen()
		before, err := os.ReadFile(fixture.absolute(retentionProgressPath))
		Expect(err).NotTo(HaveOccurred())
		Expect(validateRemainingRetention(context.Background(), fixture.tree, fixture.progress())).NotTo(Succeed())
		after, err := os.ReadFile(fixture.absolute(retentionProgressPath))
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before), "refusal must not advance deletion authority")
		if change == "unknown empty directory" {
			info, statErr := os.Lstat(fixture.absolute(path))
			Expect(statErr).NotTo(HaveOccurred())
			Expect(info.IsDir()).To(BeTrue())
		} else {
			body, readErr := os.ReadFile(fixture.absolute(path))
			Expect(readErr).NotTo(HaveOccurred())
			Expect(body).To(Equal(want))
		}
	}, Entry("edited remaining file", "edited remaining file"), Entry("reoccupied intended absence", "reoccupied intended absence"), Entry("unknown regular entry", "unknown regular entry"), Entry("unknown empty directory", "unknown empty directory"))

	// per docs/adr/0106-go-backup-retention.md:94
	It("refuses a future missing leaf which has no matching durable intent", func() {
		fixture := newPruneComponentFixture()
		progress := fixture.interruptAfterUnlink()
		Expect(os.Remove(fixture.absolute(progress.Rows[1].Path))).To(Succeed())
		fixture.reopen()
		err := validateRemainingRetention(context.Background(), fixture.tree, fixture.progress())
		Expect(err).To(HaveOccurred())
		Expect(ErrorStatus(err)).To(Equal(2))
		_, statErr := os.Lstat(fixture.absolute(backupBase(fixture.snapshot.ID) + "/installation.manifest"))
		Expect(statErr).NotTo(HaveOccurred(), "remaining recovery metadata must be preserved")
	})

	// per docs/adr/0106-go-backup-retention.md:94
	It("classifies a future missing directory without durable intent as a preserved conflict", func() {
		fixture := newPruneComponentFixture()
		rows, err := retentionRows(fixture.snapshot)
		Expect(err).NotTo(HaveOccurred())
		firstDirectory := -1
		for index, row := range rows {
			if row.Directory != nil {
				firstDirectory = index
				break
			}
		}
		Expect(firstDirectory).To(BeNumerically(">", 0))
		preceding := rows[firstDirectory-1]
		Expect(preceding.Image).NotTo(BeNil())
		interrupted := errors.New("test interruption before the first directory intent")
		journalBefore, err := os.ReadFile(fixture.absolute(journalPath(fixture.snapshot.ID)))
		Expect(err).NotTo(HaveOccurred())
		_, _, err = executeRetentionPrune(context.Background(), fixture.tree, fixture.snapshot, "component-newer", retentionPruneOps{afterStep: func(_ context.Context, path string) error {
			if path == preceding.Path {
				return interrupted
			}
			return nil
		}})
		Expect(errors.Is(err, interrupted)).To(BeTrue(), "%v", err)
		fixture.reopen()
		progress := fixture.progress()
		Expect(progress.Next).To(Equal(firstDirectory - 1))
		Expect(progress.Intent).To(BeTrue(), "only the preceding file has deletion intent")
		_, err = os.Lstat(fixture.absolute(preceding.Path))
		Expect(os.IsNotExist(err)).To(BeTrue(), "the interruption follows the actual last-file unlink")
		Expect(os.Remove(fixture.absolute(rows[firstDirectory].Path))).To(Succeed(), "the future directory is really empty")
		fixture.reopen()
		progressBefore, err := os.ReadFile(fixture.absolute(retentionProgressPath))
		Expect(err).NotTo(HaveOccurred())
		refusal := validateRemainingRetention(context.Background(), fixture.tree, fixture.progress())
		Expect(refusal).To(HaveOccurred())
		Expect(os.ReadFile(fixture.absolute(retentionProgressPath))).To(Equal(progressBefore), "refusal cannot advance deletion intent")
		Expect(os.ReadFile(fixture.absolute(journalPath(fixture.snapshot.ID)))).To(Equal(journalBefore))
		for _, path := range []string{"factory", "scripts/factory-report.sh"} {
			Expect(os.ReadFile(fixture.absolute(path))).To(Equal([]byte("saved predecessor " + path + "\n")))
		}
		Expect(ErrorStatus(refusal)).To(Equal(2), "missing future directories are policy conflicts, not operational failures")
	})

	// per docs/adr/0106-go-backup-retention.md:95
	It("refuses reoccupation of a durably completed deletion even with identical bytes", func() {
		fixture := newPruneComponentFixture()
		interrupted := errors.New("interrupt before second owned leaf")
		var first string
		_, _, err := executeRetentionPrune(context.Background(), fixture.tree, fixture.snapshot, "component-newer", retentionPruneOps{beforeStep: func(_ context.Context, path string) error {
			if first == "" {
				first = path
				return nil
			}
			return interrupted
		}})
		Expect(errors.Is(err, interrupted)).To(BeTrue())
		fixture.reopen()
		progress := fixture.progress()
		Expect(progress.Next).To(Equal(1))
		original, err := os.ReadFile(fixture.absolute(strings.TrimPrefix(first, backupBase(fixture.snapshot.ID)+"/assets/")))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(fixture.absolute(first), original, 0600)).To(Succeed())
		fixture.reopen()
		err = validateRemainingRetention(context.Background(), fixture.tree, fixture.progress())
		Expect(err).To(MatchError(ContainSubstring("occupied again")))
		got, err := os.ReadFile(fixture.absolute(first))
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(original))
	})

	// per docs/adr/0106-go-backup-retention.md:95
	DescribeTable("rejects changed prune progress before it can name another deletion", func(change string) {
		fixture := newPruneComponentFixture()
		progress := fixture.interruptAfterUnlink()
		switch change {
		case "foreign row":
			progress.Rows[0].Path = "factory"
		case "out of range":
			progress.Next = len(progress.Rows) + 1
		case "sole recovery":
			progress.Protect = progress.Snapshot.ID
		}
		current, _, err := observe(context.Background(), fixture.tree, retentionProgressPath, journalLimit)
		Expect(err).NotTo(HaveOccurred())
		Expect(writeRecord(context.Background(), fixture.tree, retentionProgressPath, progress, current)).To(Succeed())
		fixture.reopen()
		_, err = readRetentionProgress(context.Background(), fixture.tree)
		Expect(err).To(HaveOccurred())
		Expect(ErrorStatus(err)).To(Equal(2))
		active, err := os.ReadFile(fixture.absolute("factory"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(active)).To(Equal("saved predecessor factory\n"))
	}, Entry("foreign row", "foreign row"), Entry("out of range", "out of range"), Entry("sole recovery", "sole recovery"))
})

// Policy only: activation/check success remains the live capability's obligation.
// These cases do not pretend that Request contains a successful-check marker.
// per docs/adr/0106-go-backup-retention.md:18
var _ = Describe("Installation retention advancement policy", func() {
	DescribeTable("advances only a distinct forward official semantic release", func(scenario string, advanced bool) {
		request := Request{Profile: "go-hybrid-v1", Before: staging.Options{Version: "v1.0.0"}, Source: staging.Options{Version: "v2.0.0"}}
		switch scenario {
		case "same":
			request.Source.Version = request.Before.Version
		case "build metadata":
			request.Before.Version, request.Source.Version = "v1.0.0+one", "v1.0.0+two"
		case "earlier":
			request.Source.Version = "v0.9.0"
		case "incomparable predecessor":
			request.Before.Version = "release-one"
		case "incomparable candidate":
			request.Source.Version = "release-two"
		case "local predecessor":
			request.Before.Local = true
		case "local candidate":
			request.Source.Local = true
		case "initial legacy":
			request.Profile = "v0.1.6"
		case "rollback":
			request.Rollback = "component-rollback"
		case "recovery":
			request.Recover = "component-recovery"
		case "prerelease to final":
			request.Before.Version, request.Source.Version = "v2.0.0-rc.1", "v2.0.0"
		}
		actual, reason := retentionAdvances(request)
		Expect(actual).To(Equal(advanced))
		Expect(reason).NotTo(BeEmpty())
	}, Entry("skipped forward release", "forward", true), Entry("prerelease to final", "prerelease to final", true), Entry("same release", "same", false), Entry("build metadata only", "build metadata", false), Entry("earlier release", "earlier", false), Entry("incomparable predecessor", "incomparable predecessor", false), Entry("incomparable candidate", "incomparable candidate", false), Entry("local predecessor", "local predecessor", false), Entry("local candidate", "local candidate", false), Entry("initial legacy migration", "initial legacy", false), Entry("explicit rollback", "rollback", false), Entry("explicit recovery", "recovery", false))
})
