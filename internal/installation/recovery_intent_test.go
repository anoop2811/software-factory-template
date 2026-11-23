package installation

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Use real rename and replacement identities at both crash boundaries.
// per docs/DECISION_LOG.md:9151
var _ = Describe("Recovery after an interrupted direction change", func() {
	It("keeps the qualified active inode while the next replacement is staged", func() {
		root := GinkgoT().TempDir()
		active := filepath.Join(root, "active")
		old, candidate := []byte("before"), []byte("candidate")
		installationComponentWrite(root, "active", old, 0700)
		original := installationComponentImage(root, "active", old)
		installationComponentWrite(root, "stage", candidate, 0700)
		entry := journalEntry{Phase: "prepared", Before: original, PreparedAfter: installationComponentImage(root, "stage", candidate)}
		Expect(os.Rename(filepath.Join(root, "stage"), active)).To(Succeed())
		published := installationComponentImage(root, "active", candidate)
		Expect(entry.recognizesPrepared(published, false, true, &journal{Direction: "forward", Purpose: "upgrade"})).To(BeTrue())

		// An older writer carried the prepared forward receipt into reverse intent.
		legacy := entry
		legacy.Phase = "reverse_prepared"
		reversing := &journal{Direction: "reverse", Purpose: "recovery"}
		Expect(legacy.recognizesPrepared(published, false, true, reversing)).To(BeTrue())
		Expect(legacy.recognizesPrepared(published, false, false, reversing)).To(BeFalse())
		Expect(legacy.recognizesPrepared(published, false, true, &journal{Purpose: "rollback", Direction: "reverse"})).To(BeFalse())
		observed := legacy
		observed.ObservedAfter = published
		changed := *published
		changedID := *published.Identity
		changedID.ChangedSeconds++
		changed.Identity = &changedID
		Expect(observed.recognizesPrepared(&changed, false, true, reversing)).To(BeFalse())

		entry.prepareIntent(Action{Action: "replace", Before: published, After: original}, true)
		Expect(entry.Phase).To(Equal("reverse_prepared"))
		Expect(entry.ObservedAfter).To(Equal(published))
		Expect(entry.PreparedAfter).To(BeNil())
		installationComponentWrite(root, "reverse-stage", old, 0700)
		entry.PreparedAfter = installationComponentImage(root, "reverse-stage", old)
		Expect(imageEqual(installationComponentImage(root, "active", candidate), entry.ObservedAfter, true)).To(BeTrue())

		installationComponentWrite(root, "foreign", candidate, 0700)
		Expect(os.Rename(filepath.Join(root, "foreign"), active)).To(Succeed())
		foreign := installationComponentImage(root, "active", candidate)
		Expect(imageEqual(foreign, entry.ObservedAfter, true)).To(BeFalse())
		Expect(legacy.recognizesPrepared(foreign, false, true, reversing)).To(BeFalse())
		Expect(entry.recognizesPrepared(foreign, false, true, reversing)).To(BeFalse())

		Expect(os.Rename(filepath.Join(root, "reverse-stage"), active)).To(Succeed())
		restored := installationComponentImage(root, "active", old)
		Expect(entry.recognizesPrepared(restored, true, false, reversing)).To(BeTrue())
	})
})
