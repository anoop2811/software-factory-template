package assessment

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const adoptionGoldenJSON = `{"purpose":"adopt_known_legacy_assets","schema_version":1,"scope":"g2-budget-loop-six","reference_revision":"c8f8d34edbc14df5655fcbf0aab7eea46ced0295","prior_origin":"unproven","ownership_authorized":false,"root_identity":{"device":"7","inode":"9007199254740993"},"selection":["scripts/factory-budget.sh"],"assets":[{"path":"scripts/factory-budget.sh","reference":{"sha256":"abababababababababababababababababababababababababababababababab","mode":"0755","bytes":3},"identity":{"device":"7","inode":"9007199254740995","type":"regular","mode":"0755","bytes":3,"mtime_seconds":"-2","mtime_nanoseconds":4,"ctime_seconds":"3","ctime_nanoseconds":5},"observed":{"sha256":"abababababababababababababababababababababababababababababababab","mode":"0755","bytes":3}}]}`
const adoptionGoldenSHA = "81eee3ecbe29de7511a12e0b5e1cdf47052ba885ee94419936bc16c625c7ac2f"

func goldenProposal() Proposal {
	GinkgoHelper()
	var proposal Proposal
	Expect(json.Unmarshal([]byte(adoptionGoldenJSON), &proposal)).To(Succeed())
	return proposal
}
func proposalLeafPaths(value reflect.Value, prefix string) []string {
	var paths []string
	switch value.Kind() {
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			name := value.Type().Field(i).Name
			if prefix == "" && name == "ProposalDigest" {
				continue
			}
			next := name
			if prefix != "" {
				next = prefix + "." + name
			}
			paths = append(paths, proposalLeafPaths(value.Field(i), next)...)
		}
	case reflect.Slice:
		if value.Len() > 0 {
			paths = append(paths, proposalLeafPaths(value.Index(0), prefix+"[]")...)
		} else {
			paths = append(paths, prefix+"[]")
		}
	default:
		paths = append(paths, prefix)
	}
	return paths
}
func changeProposalLeaf(value reflect.Value, target string) {
	if strings.HasPrefix(target, "Assets[].") {
		changeProposalLeaf(value.FieldByName("Assets").Index(0), strings.TrimPrefix(target, "Assets[]."))
		return
	}
	name, rest, more := strings.Cut(target, ".")
	field := value.FieldByName(name)
	Expect(field.IsValid()).To(BeTrue())
	if more {
		changeProposalLeaf(field, rest)
		return
	}
	switch field.Kind() {
	case reflect.String:
		field.SetString(field.String() + "changed")
	case reflect.Bool:
		field.SetBool(!field.Bool())
	case reflect.Int, reflect.Int64:
		field.SetInt(field.Int() + 1)
	default:
		Fail("unqualified proposal leaf type")
	}
}

var _ = Describe("Legacy adoption canonical digest", func() {
	// per docs/adr/0081-explicit-legacy-asset-adoption.md:55
	It("matches an independently serialized golden vector including numeric strings", func() {
		digest, err := proposalDigest(goldenProposal(), []string{"scripts/factory-budget.sh"})
		Expect(err).NotTo(HaveOccurred())
		Expect(digest).To(Equal(adoptionGoldenSHA))
	})
	// per docs/adr/0081-explicit-legacy-asset-adoption.md:55
	It("binds every declared field and fails coverage when new fields are omitted", func() {
		proposal := goldenProposal()
		expected := []string{"SchemaVersion", "ReferenceRevision", "Scope", "Purpose", "PriorOrigin", "OwnershipAuthorized", "RootIdentity.Device", "RootIdentity.Inode", "Assets[].Path", "Assets[].Reference.SHA256", "Assets[].Reference.Mode", "Assets[].Reference.Bytes", "Assets[].Identity.Device", "Assets[].Identity.Inode", "Assets[].Identity.Type", "Assets[].Identity.Mode", "Assets[].Identity.Bytes", "Assets[].Identity.MtimeSeconds", "Assets[].Identity.MtimeNanoseconds", "Assets[].Identity.CtimeSeconds", "Assets[].Identity.CtimeNanoseconds", "Assets[].Observed.SHA256", "Assets[].Observed.Mode", "Assets[].Observed.Bytes"}
		actual := proposalLeafPaths(reflect.ValueOf(proposal), "")
		sort.Strings(actual)
		sort.Strings(expected)
		Expect(actual).To(Equal(expected))
		for _, path := range expected {
			changed := goldenProposal()
			changeProposalLeaf(reflect.ValueOf(&changed).Elem(), path)
			digest, err := proposalDigest(changed, []string{"scripts/factory-budget.sh"})
			Expect(err).NotTo(HaveOccurred())
			Expect(digest).NotTo(Equal(adoptionGoldenSHA), "unbound field %s", path)
		}
		selectionChanged, err := proposalDigest(proposal, []string{"scripts/factory-loop.sh"})
		Expect(err).NotTo(HaveOccurred())
		Expect(selectionChanged).NotTo(Equal(adoptionGoldenSHA))
		proposal.ProposalDigest = "not part of its own digest"
		same, err := proposalDigest(proposal, []string{"scripts/factory-budget.sh"})
		Expect(err).NotTo(HaveOccurred())
		Expect(same).To(Equal(adoptionGoldenSHA))
	})
})

func adoptionMatchingFixture() (string, string) {
	GinkgoHelper()
	root, path := assessmentReadFixture()
	data, err := exec.Command("git", "show", "c8f8d34edbc14df5655fcbf0aab7eea46ced0295:scripts/factory-budget.sh").Output() // #nosec G204 -- immutable fixed Git blob supplies independent known reference.
	Expect(err).NotTo(HaveOccurred())
	Expect(os.WriteFile(path, data, 0600)).To(Succeed())
	Expect(os.Chmod(path, 0755)).To(Succeed())
	return root, path
}

var _ = Describe("Legacy adoption controlled observations", func() {
	// per docs/adr/0081-explicit-legacy-asset-adoption.md:78
	DescribeTable("rejects same-byte selected inode changes at each planning observation", func(stage string) {
		root, path := adoptionMatchingFixture()
		source, _ := adoptionMatchingFixture()
		selection := []string{"scripts/factory-budget.sh"}
		proposal, err := ProposeAdoption(context.Background(), root, selection)
		Expect(err).NotTo(HaveOccurred())
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
			if stage == "middle before open" && opens == 2 {
				replace()
			}
			return openAt(parent, name, flags)
		}}
		sourceOps := ops{read: func(file *os.File, buffer []byte) (int, error) {
			if stage == "during source read" {
				replace()
			}
			return file.Read(buffer)
		}}
		result, err := planAdopted(context.Background(), root, source, proposal.ProposalDigest, selection, installedOps, sourceOps)
		Expect(changed).To(BeTrue())
		Expect(err).To(HaveOccurred())
		Expect(ErrorStatus(err)).To(Equal(2))
		encoded, marshalErr := json.Marshal(result)
		Expect(marshalErr).NotTo(HaveOccurred())
		Expect(string(encoded)).NotTo(ContainSubstring("explicit_operator_adoption"))
	}, Entry("middle installed observation", "middle before open"), Entry("final reobservation", "during source read"))
	// per docs/adr/0081-explicit-legacy-asset-adoption.md:78
	DescribeTable("rejects either root changing while the opposite tree is read", func(replaced string) {
		root, _ := adoptionMatchingFixture()
		source, _ := adoptionMatchingFixture()
		selection := []string{"scripts/factory-budget.sh"}
		proposal, err := ProposeAdoption(context.Background(), root, selection)
		Expect(err).NotTo(HaveOccurred())
		changed := false
		hook := func(file *os.File, buffer []byte) (int, error) {
			if !changed {
				changed = true
				target := root
				if replaced == "source" {
					target = source
				}
				held := target + "-held"
				Expect(os.Rename(target, held)).To(Succeed())
				DeferCleanup(os.RemoveAll, held)
				Expect(os.Mkdir(target, 0700)).To(Succeed())
			}
			return file.Read(buffer)
		}
		installedOps, sourceOps := ops{}, ops{}
		if replaced == "source" {
			installedOps.read = hook
		} else {
			sourceOps.read = hook
		}
		_, err = planAdopted(context.Background(), root, source, proposal.ProposalDigest, selection, installedOps, sourceOps)
		Expect(changed).To(BeTrue())
		Expect(err).To(HaveOccurred())
		Expect(ErrorStatus(err)).To(Equal(2))
	}, Entry("installed", "installed"), Entry("source", "source"))
	// per docs/adr/0081-explicit-legacy-asset-adoption.md:37
	DescribeTable("closes selected handles on proposal I/O and cancellation failures", func(cancellation bool) {
		root, _ := adoptionMatchingFixture()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var opened *os.File
		_, err := proposeAdoption(ctx, root, []string{"scripts/factory-budget.sh"}, ops{read: func(file *os.File, buffer []byte) (int, error) {
			opened = file
			n, err := file.Read(buffer)
			if err != nil {
				return n, err
			}
			if cancellation {
				cancel()
				return n, nil
			}
			return n, syscall.EIO
		}})
		Expect(err).To(HaveOccurred())
		Expect(ErrorStatus(err)).To(Equal(1))
		if cancellation {
			Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		}
		Expect(opened).NotTo(BeNil())
		_, statErr := opened.Stat()
		Expect(errors.Is(statErr, os.ErrClosed)).To(BeTrue())
	}, Entry("read failure", false), Entry("cancellation", true))
})
