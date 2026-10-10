package installation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Installation post-apply descriptor identity", func() {
	// This constructs a private local validation component, not a genuine
	// authenticated whole-installation candidate/gate proof or activation.
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:497
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:188
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:56
	DescribeTable("rejects a genuinely replaced same-byte descriptor beside unchanged native identity", func(published bool) {
		ctx := context.Background()
		root, err := filepath.EvalSymlinks(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())
		descriptor, body, images := installationComponentLayout(root)
		owner := installationComponentOwner(root)
		owner.proof = &candidateProof{descriptor: descriptor}
		owner.published = images
		path := ".factory/installation.current"
		if !published {
			owner.planned.proposal.Actions = []Action{{Path: path, Action: "retain", Before: images[path]}}
			delete(owner.published, path)
		}
		Expect(owner.validateProvenTarget(ctx)).To(Succeed(), "unchanged independently observed descriptor and actual selected assets are a healthy component")
		pendingPath := filepath.Join(root, ".factory/runtime-publication.pending")
		Expect(os.WriteFile(pendingPath, nil, 0600)).To(Succeed())
		pendingBefore, err := os.Lstat(pendingPath)
		Expect(err).NotTo(HaveOccurred())
		descriptorPath := filepath.Join(root, filepath.FromSlash(path))
		before, err := os.Lstat(descriptorPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Rename(descriptorPath, descriptorPath+".saved-original")).To(Succeed())
		Expect(os.WriteFile(descriptorPath, body, 0600)).To(Succeed())
		replacement, err := os.Lstat(descriptorPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, replacement)).To(BeFalse(), "same-byte test must create a distinct native inode while keeping the original alive")
		actualBytes, err := os.ReadFile(descriptorPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(actualBytes).To(Equal(body))
		validationErr := owner.validateProvenTarget(ctx)
		_, _ = fmt.Fprintf(GinkgoWriter, "descriptor replacement: published=%t; distinct_native_identity=%t; equal_bytes=%t; validation=%v\n", published, !os.SameFile(before, replacement), string(actualBytes) == string(body), validationErr)
		Expect(ErrorStatus(validationErr)).To(Equal(2), "same bytes do not substitute for the retained publication/preservation observation")
		Expect(owner.close(ctx)).To(Succeed())
		foreign, err := os.Lstat(descriptorPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(replacement, foreign)).To(BeTrue(), "refusal must preserve the genuine foreign descriptor")
		actualBytes, err = os.ReadFile(descriptorPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(actualBytes).To(Equal(body))
		pendingAfter, err := os.Lstat(pendingPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(pendingBefore, pendingAfter)).To(BeTrue(), "refusal/closure cannot remove or replace unresolved pending evidence")
	}, Entry("descriptor published by this owner", true), Entry("descriptor retained from the proposal", false))
})
