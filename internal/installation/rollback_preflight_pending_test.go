package installation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The fixture uses real files for marker identities and the current recipe for
// the entry names needed to construct a readable installation journal.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:702
func rollbackPendingComponent() *transaction {
	GinkgoHelper()
	root, err := filepath.EvalSymlinks(GinkgoT().TempDir())
	Expect(err).NotTo(HaveOccurred())
	_, _, images := installationComponentLayout(root)
	owner := installationComponentOwner(root)
	owner.request = Request{Recover: "rollback-pending-component", Direction: "forward", Profile: "v0.1.6", Confirmation: strings.Repeat("a", 64)}
	owner.terminalCleanup = true
	owner.record = journal{SchemaVersion: 1, Kind: "installation-transaction", MigrationID: owner.request.Recover, Purpose: "upgrade", Direction: "forward", Phase: "committed", Profile: "v0.1.6", Target: runtime.GOOS + "/" + runtime.GOARCH, PlanDigest: owner.request.Confirmation, Outcome: "completed", Checks: []checkRecord{{Name: "native-gate-proof", Phase: "complete", Outcome: "completed"}}}
	for _, row := range roles {
		owner.record.Entries = append(owner.record.Entries, journalEntry{Path: row.Path, Action: "retain", Phase: "applied"})
	}
	for path, image := range images {
		owner.planned.proposal.Actions = append(owner.planned.proposal.Actions, Action{Path: path, Action: "retain", Before: image, After: image})
	}
	installationComponentWrite(root, ".factory/historical-pending-receipt", nil, 0600)
	owner.record.Pending = installationComponentImage(root, ".factory/historical-pending-receipt", nil)
	Expect(owner.save(context.Background())).To(Succeed())
	return owner
}

func rollbackPreparedMarker(owner *transaction, published bool) *Image {
	GinkgoHelper()
	ctx := context.Background()
	const prepared = ".factory/rollback-marker-prepared"
	installationComponentWrite(owner.root, prepared, nil, 0600)
	owner.record.PreparedPending = installationComponentImage(owner.root, prepared, nil)
	Expect(owner.save(ctx)).To(Succeed())
	if !published {
		return nil
	}
	const active = ".factory/runtime-publication.pending"
	Expect(os.Rename(filepath.Join(owner.root, prepared), filepath.Join(owner.root, active))).To(Succeed())
	actual := installationComponentImage(owner.root, active, nil)
	Expect(actual.Identity.Inode).To(Equal(owner.record.PreparedPending.Identity.Inode))
	Expect(actual.Identity.Device).To(Equal(owner.record.PreparedPending.Identity.Device))
	var err error
	owner.pending, err = owner.tree.OpenFile(ctx, active, 0)
	Expect(err).NotTo(HaveOccurred())
	return actual
}

func rollbackPersistedJournal(owner *transaction) (journal, []byte, os.FileInfo) {
	GinkgoHelper()
	path := filepath.Join(owner.root, journalPath(owner.record.MigrationID))
	body, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	var record journal
	Expect(json.Unmarshal(body, &record)).To(Succeed())
	info, err := os.Lstat(path)
	Expect(err).NotTo(HaveOccurred())
	return record, body, info
}

// The context remains uncancelled; its existing observation boundary triggers
// the independent filesystem actor only after actual journal publication.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:690
type rollbackJournalObservationContext struct {
	context.Context
	observe func()
}

func (ctx rollbackJournalObservationContext) Err() error {
	ctx.observe()
	return ctx.Context.Err()
}

var _ = Describe("Completed rollback pending preparation", func() {
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:668
	It("reserves a real barrier while retaining committed installation evidence", func() {
		ctx := context.Background()
		owner := rollbackPendingComponent()
		owner.request.Rollback, owner.request.Recover = owner.record.MigrationID, ""
		before, _, _ := rollbackPersistedJournal(owner)
		Expect(owner.prepareRollbackPending(ctx)).To(Succeed())
		after, _, _ := rollbackPersistedJournal(owner)
		Expect(after.Phase).To(Equal("committed"))
		Expect(after.Direction).To(Equal("forward"))
		Expect(after.Entries).To(Equal(before.Entries))
		Expect(after.Checks).To(Equal(before.Checks))
		Expect(after.PreparedPending).To(BeNil())
		Expect(after.Pending).To(Equal(installationComponentImage(owner.root, ".factory/runtime-publication.pending", nil)))
		Expect(after.Pending.Identity.Inode).NotTo(Equal(before.Pending.Identity.Inode))
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:679
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:688
	It("clears unused preparation after confirmed absence without replacing the historical receipt", func() {
		ctx := context.Background()
		owner := rollbackPendingComponent()
		before, _, _ := rollbackPersistedJournal(owner)
		rollbackPreparedMarker(owner, false)
		Expect(owner.finalizePreparedPending(ctx, nil)).To(Succeed())
		after, bytes, identity := rollbackPersistedJournal(owner)
		Expect(after.PreparedPending).To(BeNil())
		Expect(after.Pending).To(Equal(before.Pending))
		Expect(after.Entries).To(Equal(before.Entries))
		Expect(after.Checks).To(Equal(before.Checks))
		Expect(owner.finalizePreparedPending(ctx, nil)).To(Succeed())
		_, repeated, repeatedIdentity := rollbackPersistedJournal(owner)
		Expect(repeated).To(Equal(bytes))
		Expect(os.SameFile(identity, repeatedIdentity)).To(BeTrue(), "ordinary replay cannot rewrite the journal")
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:681
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:690
	It("adopts the renamed object's observed identity durably before terminal unlink", func() {
		ctx := context.Background()
		owner := rollbackPendingComponent()
		actual := rollbackPreparedMarker(owner, true)
		Expect(owner.finalizePreparedPending(ctx, actual)).To(Succeed())
		adopted, _, _ := rollbackPersistedJournal(owner)
		Expect(adopted.PreparedPending).To(BeNil())
		Expect(adopted.Pending).To(Equal(actual))
		_, statErr := os.Lstat(filepath.Join(owner.root, ".factory/runtime-publication.pending"))
		Expect(statErr).NotTo(HaveOccurred(), "the durable adoption boundary precedes unlink")
		Expect(owner.completePendingCleanup(ctx)).To(Succeed())
		_, err := os.Lstat(filepath.Join(owner.root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		cleaned, _, _ := rollbackPersistedJournal(owner)
		Expect(cleaned.Pending).To(Equal(actual))
		Expect(cleaned.PreparedPending).To(BeNil())
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:681
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:685
	DescribeTable("cleans only the owned prepared marker and preserves same-byte replacements", func(replaced bool) {
		ctx := context.Background()
		owner := rollbackPendingComponent()
		actual := rollbackPreparedMarker(owner, true)
		_, beforeBytes, beforeIdentity := rollbackPersistedJournal(owner)
		path := filepath.Join(owner.root, ".factory/runtime-publication.pending")
		if replaced {
			Expect(os.Rename(path, path+".owned-original")).To(Succeed())
			installationComponentWrite(owner.root, ".factory/runtime-publication.pending", nil, 0600)
			foreign := installationComponentImage(owner.root, ".factory/runtime-publication.pending", nil)
			Expect(foreign.SHA256).To(Equal(actual.SHA256))
			Expect(foreign.Identity.Inode).NotTo(Equal(actual.Identity.Inode))
			err := owner.completePendingCleanup(ctx)
			Expect(ErrorStatus(err)).To(Equal(2))
			Expect(installationComponentImage(owner.root, ".factory/runtime-publication.pending", nil)).To(Equal(foreign))
			_, afterBytes, afterIdentity := rollbackPersistedJournal(owner)
			Expect(afterBytes).To(Equal(beforeBytes))
			Expect(os.SameFile(beforeIdentity, afterIdentity)).To(BeTrue())
		} else {
			Expect(owner.completePendingCleanup(ctx)).To(Succeed())
			_, err := os.Lstat(path)
			Expect(os.IsNotExist(err)).To(BeTrue())
			persisted, _, _ := rollbackPersistedJournal(owner)
			Expect(persisted.Pending).To(Equal(actual))
			Expect(persisted.PreparedPending).To(BeNil())
		}
	}, Entry("native prepared rename", false), Entry("foreign equal-byte inode", true))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:685
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:690
	DescribeTable("refuses marker replacement during durable pending adoption", func(reserving bool) {
		owner := rollbackPendingComponent()
		historical := owner.record.Pending
		if reserving {
			owner.request.Rollback, owner.request.Recover = owner.record.MigrationID, ""
		} else {
			rollbackPreparedMarker(owner, true)
		}
		path := filepath.Join(owner.root, ".factory/runtime-publication.pending")
		journalFile := filepath.Join(owner.root, journalPath(owner.record.MigrationID))
		var mutex sync.Mutex
		var replaced bool
		var replacementErr error
		var foreign os.FileInfo
		ctx := rollbackJournalObservationContext{Context: context.Background(), observe: func() {
			mutex.Lock()
			defer mutex.Unlock()
			if replaced || replacementErr != nil {
				return
			}
			body, err := os.ReadFile(journalFile)
			if err != nil {
				replacementErr = err
				return
			}
			var saved journal
			if err := json.Unmarshal(body, &saved); err != nil {
				replacementErr = err
				return
			}
			if saved.PreparedPending != nil || saved.Pending == nil || saved.Pending.Identity == nil || imageEqual(saved.Pending, historical, true) {
				return
			}
			replaced = true
			if replacementErr = os.Rename(path, path+".owned-original"); replacementErr != nil {
				return
			}
			if replacementErr = os.WriteFile(path, nil, 0600); replacementErr != nil {
				return
			}
			foreign, replacementErr = os.Lstat(path)
		}}
		var err error
		if reserving {
			err = owner.prepareRollbackPending(ctx)
		} else {
			err = owner.completePendingCleanup(ctx)
		}
		Expect(replacementErr).NotTo(HaveOccurred())
		Expect(replaced).To(BeTrue(), "must witness the actual adopted journal before replacing the marker")
		Expect(ErrorStatus(err)).To(Equal(2), "the foreign same-byte leaf must be rechecked after the journal save")
		after, statErr := os.Lstat(path)
		Expect(statErr).NotTo(HaveOccurred(), "terminal cleanup deleted a foreign marker")
		Expect(os.SameFile(foreign, after)).To(BeTrue())
		owned, statErr := os.Lstat(path + ".owned-original")
		Expect(statErr).NotTo(HaveOccurred())
		Expect(os.SameFile(owned, after)).To(BeFalse(), "the independent actor must create a distinct native object")
		persisted, _, _ := rollbackPersistedJournal(owner)
		Expect(persisted.Pending.Identity.Inode).To(Equal(installationComponentImage(owner.root, ".factory/runtime-publication.pending.owned-original", nil).Identity.Inode))
		Expect(persisted.PreparedPending).To(BeNil())
	}, Entry("before launching rollback checks", true), Entry("before terminal unlink", false))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:685
	It("retains strict observed-marker identity after preparation is finalized", func() {
		ctx := context.Background()
		owner := rollbackPendingComponent()
		actual := rollbackPreparedMarker(owner, true)
		Expect(owner.finalizePreparedPending(ctx, actual)).To(Succeed())
		_, before, identity := rollbackPersistedJournal(owner)
		path := filepath.Join(owner.root, ".factory/runtime-publication.pending")
		Expect(os.Chmod(path, 0400)).To(Succeed())
		Expect(os.Chmod(path, 0600)).To(Succeed())
		changed := installationComponentImage(owner.root, ".factory/runtime-publication.pending", nil)
		Expect(changed.Identity.Inode).To(Equal(actual.Identity.Inode))
		Expect(changed.Identity).NotTo(Equal(actual.Identity), "the control must actually alter native observation")
		Expect(ErrorStatus(owner.completePendingCleanup(ctx))).To(Equal(2))
		_, statErr := os.Stat(path)
		Expect(statErr).NotTo(HaveOccurred())
		_, after, afterIdentity := rollbackPersistedJournal(owner)
		Expect(after).To(Equal(before))
		Expect(os.SameFile(identity, afterIdentity)).To(BeTrue())
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:673
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:698
	DescribeTable("refuses malformed or uncertain preparation records through the journal reader", func(change string) {
		ctx := context.Background()
		owner := rollbackPendingComponent()
		rollbackPreparedMarker(owner, false)
		malformed, err := readJournalEvidence(ctx, owner.tree, owner.record.MigrationID, false)
		Expect(err).NotTo(HaveOccurred(), "healthy component record must reach semantic preparation validation")
		switch change {
		case "phase":
			malformed.Phase = "checking"
		case "direction":
			malformed.Direction = "reverse"
		case "outcome":
			malformed.Outcome = "pending"
		case "historical receipt":
			malformed.Pending = nil
		case "mode":
			malformed.PreparedPending.Mode = 0100644
		case "size":
			malformed.PreparedPending.Bytes = 1
		case "identity":
			malformed.PreparedPending.Identity = nil
		case "unconfirmed child":
			malformed.Checks[0].OwnershipUnconfirmed = true
		case "running child":
			malformed.Checks[0].Phase = "checking"
		}
		// Persist the reader's input independently of the fixture owner's live checks.
		_, err = saveJournal(ctx, owner.tree, malformed, owner.journalImage)
		Expect(err).NotTo(HaveOccurred())
		_, before, identity := rollbackPersistedJournal(owner)
		_, err = readJournalEvidence(ctx, owner.tree, owner.record.MigrationID, false)
		Expect(err).To(HaveOccurred(), change)
		_, after, afterIdentity := rollbackPersistedJournal(owner)
		Expect(after).To(Equal(before), "read-only refusal cannot repair a record")
		Expect(os.SameFile(identity, afterIdentity)).To(BeTrue())
	}, Entry("nonterminal phase", "phase"), Entry("reverse direction", "direction"), Entry("uncompleted outcome", "outcome"), Entry("missing historical receipt", "historical receipt"), Entry("nonprivate mode", "mode"), Entry("nonempty object", "size"), Entry("missing native identity", "identity"), Entry("unknown child ownership", "unconfirmed child"), Entry("running child", "running child"))
})
