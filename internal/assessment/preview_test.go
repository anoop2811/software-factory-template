package assessment

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Upgrade preview shared pinned observations", func() {
	// per docs/adr/0082-go-public-upgrade-preview.md:59
	DescribeTable("refuses selected same-byte identity replacement without a confirmation", func(stage string) {
		root, path := adoptionMatchingFixture()
		source, _ := adoptionMatchingFixture()
		changed := false
		opens := 0
		replace := func() {
			if changed {
				return
			}
			changed = true
			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Rename(path, path+".held")).To(Succeed())
			Expect(os.WriteFile(path, data, 0600)).To(Succeed())
			Expect(os.Chmod(path, 0755)).To(Succeed())
		}
		installedOps := ops{open: func(parent *os.File, name string, flags int) (*os.File, error) {
			opens++
			if stage == "middle" && opens == 2 {
				replace()
			}
			return openAt(parent, name, flags)
		}}
		sourceOps := ops{read: func(file *os.File, buffer []byte) (int, error) {
			if stage == "final" {
				replace()
			}
			return file.Read(buffer)
		}}
		result, err := previewAdoption(context.Background(), root, source, []string{"scripts/factory-budget.sh"}, "", installedOps, sourceOps)
		Expect(changed).To(BeTrue())
		Expect(err).To(HaveOccurred())
		Expect(ErrorStatus(err)).To(Equal(2))
		Expect(result.Plan.Assets).To(BeEmpty())
		Expect(result.AuthorizedPaths).To(BeEmpty())
	}, Entry("middle planning observation", "middle"), Entry("final observation", "final"))
	// per docs/adr/0082-go-public-upgrade-preview.md:128
	DescribeTable("pins both roots during unconfirmed planning and closes observed descriptors", func(replaced string) {
		root, _ := adoptionMatchingFixture()
		source, _ := adoptionMatchingFixture()
		changed := false
		var opened []*os.File
		opening := func(parent *os.File, name string, flags int) (*os.File, error) {
			file, err := openAt(parent, name, flags)
			if file != nil {
				opened = append(opened, file)
			}
			return file, err
		}
		hook := func(file *os.File, buffer []byte) (int, error) {
			if !changed {
				changed = true
				target := root
				if replaced == "source" {
					target = source
				}
				held := target + ".held"
				Expect(os.Rename(target, held)).To(Succeed())
				DeferCleanup(os.RemoveAll, held)
				Expect(os.Mkdir(target, 0700)).To(Succeed())
			}
			return file.Read(buffer)
		}
		installedOps, sourceOps := ops{open: opening}, ops{open: opening}
		if replaced == "source" {
			installedOps.read = hook
		} else {
			sourceOps.read = hook
		}
		result, err := previewAdoption(context.Background(), root, source, []string{"scripts/factory-budget.sh"}, "", installedOps, sourceOps)
		Expect(changed).To(BeTrue())
		Expect(err).To(HaveOccurred())
		Expect(ErrorStatus(err)).To(Equal(2))
		Expect(result.Plan.Assets).To(BeEmpty())
		for _, file := range opened {
			_, err := file.Stat()
			Expect(errors.Is(err, os.ErrClosed)).To(BeTrue())
		}
	}, Entry("source replaced from installed read", "source"), Entry("installed replaced from source read", "installed"))
	// per docs/adr/0082-go-public-upgrade-preview.md:128
	DescribeTable("returns no authority after selected I/O failure or mid-read cancellation", func(cancellation bool, confirmed bool) {
		root, _ := adoptionMatchingFixture()
		source, _ := adoptionMatchingFixture()
		selection := []string{"scripts/factory-budget.sh"}
		confirmation := ""
		if confirmed {
			proposal, err := ProposeAdoption(context.Background(), root, selection)
			Expect(err).NotTo(HaveOccurred())
			confirmation = proposal.ProposalDigest
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var opened *os.File
		controls := ops{read: func(file *os.File, buffer []byte) (int, error) {
			opened = file
			n, err := file.Read(buffer)
			if err != nil {
				return n, err
			}
			if cancellation {
				cancel()
				return n, nil
			}
			return n, errors.New("PRIVATE_SELECTED_IO_FAILURE")
		}}
		result, err := previewAdoption(ctx, root, source, selection, confirmation, controls, ops{})
		Expect(err).To(HaveOccurred())
		Expect(ErrorStatus(err)).To(Equal(1))
		Expect(result.Plan.Assets).To(BeEmpty())
		Expect(result.AuthorizedPaths).To(BeEmpty())
		if cancellation {
			Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		}
		Expect(err.Error()).NotTo(ContainSubstring("PRIVATE_"))
		encoded, marshalErr := json.Marshal(result)
		Expect(marshalErr).NotTo(HaveOccurred())
		Expect(string(encoded)).NotTo(ContainSubstring("explicit_operator_adoption"))
		Expect(opened).NotTo(BeNil())
		_, statErr := opened.Stat()
		Expect(errors.Is(statErr, os.ErrClosed)).To(BeTrue())
	}, Entry("unconfirmed I/O", false, false), Entry("unconfirmed cancellation", true, false), Entry("confirmed I/O", false, true), Entry("confirmed cancellation", true, true))
})
