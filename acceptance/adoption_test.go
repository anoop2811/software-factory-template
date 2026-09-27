package acceptance_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type adoptionRootIdentity struct {
	Device string `json:"device"`
	Inode  string `json:"inode"`
}
type adoptionAssetIdentity struct {
	Device           string `json:"device"`
	Inode            string `json:"inode"`
	Type             string `json:"type"`
	Mode             string `json:"mode"`
	Bytes            int    `json:"bytes"`
	MtimeSeconds     string `json:"mtime_seconds"`
	MtimeNanoseconds int    `json:"mtime_nanoseconds"`
	CtimeSeconds     string `json:"ctime_seconds"`
	CtimeNanoseconds int    `json:"ctime_nanoseconds"`
}
type adoptionAsset struct {
	Path      string                `json:"path"`
	Reference assessedBytes         `json:"reference"`
	Identity  adoptionAssetIdentity `json:"identity"`
	Observed  assessedBytes         `json:"observed"`
}
type adoptionProposal struct {
	SchemaVersion       int                  `json:"schema_version"`
	ReferenceRevision   string               `json:"reference_revision"`
	Scope               string               `json:"scope"`
	Purpose             string               `json:"purpose"`
	PriorOrigin         string               `json:"prior_origin"`
	OwnershipAuthorized bool                 `json:"ownership_authorized"`
	ProposalDigest      string               `json:"proposal_digest"`
	RootIdentity        adoptionRootIdentity `json:"root_identity"`
	Assets              []adoptionAsset      `json:"assets"`
}

func adoptionCommand(binary, cwd, operation string, args ...string) cliResult {
	return loopProcess(cwd, filepath.Join(binary, "factory"), append([]string{"migration", operation}, args...), nil)
}
func adoptionDecode(out cliResult, root string) adoptionProposal {
	GinkgoHelper()
	Expect(out.status).To(BeZero(), "%+v", out)
	Expect(out.stderr).To(BeEmpty())
	Expect(out.stdout).To(HaveSuffix("\n"))
	Expect(out.stdout).NotTo(ContainSubstring(root))
	decoder := json.NewDecoder(strings.NewReader(out.stdout))
	decoder.DisallowUnknownFields()
	var proposal adoptionProposal
	Expect(decoder.Decode(&proposal)).To(Succeed())
	var extra any
	Expect(decoder.Decode(&extra)).To(Equal(io.EOF))
	object := budgetDecode([]byte(out.stdout)).(map[string]any)
	Expect(object).To(HaveLen(9))
	Expect(object).To(HaveKeyWithValue("ownership_authorized", false))
	Expect(proposal.SchemaVersion).To(Equal(1))
	Expect(proposal.ReferenceRevision).To(Equal(assessmentRevision))
	Expect(proposal.Scope).To(Equal("g2-budget-loop-six"))
	Expect(proposal.Purpose).To(Equal("adopt_known_legacy_assets"))
	Expect(proposal.PriorOrigin).To(Equal("unproven"))
	Expect(proposal.ProposalDigest).To(MatchRegexp(`^[0-9a-f]{64}$`))
	Expect(proposal.RootIdentity.Device).To(MatchRegexp(`^[0-9]+$`))
	Expect(proposal.RootIdentity.Inode).To(MatchRegexp(`^[0-9]+$`))
	return proposal
}

var _ = Describe("G4 explicit legacy adoption core", func() {
	// per docs/adr/0081-explicit-legacy-asset-adoption.md:42
	It("proposes all six exact assets without granting authority", func() {
		binary, root, references := assessmentFixture()
		out := adoptionCommand(binary, root, "propose-adoption", append([]string{root}, assessmentPaths...)...)
		proposal := adoptionDecode(out, root)
		Expect(proposal.Assets).To(HaveLen(6))
		for i, asset := range proposal.Assets {
			Expect(asset.Path).To(Equal(assessmentPaths[i]))
			Expect(asset.Reference).To(Equal(references[i]))
			Expect(asset.Observed).To(Equal(references[i]))
			Expect(asset.Identity.Type).To(Equal("regular"))
			Expect(asset.Identity.Mode).To(Equal(references[i].Mode))
			Expect(asset.Identity.Bytes).To(Equal(references[i].Bytes))
		}
		Expect(adoptionCommand(binary, root, "propose-adoption", append([]string{root}, assessmentPaths...)...)).To(Equal(out))
	})
	// per docs/adr/0081-explicit-legacy-asset-adoption.md:33
	It("canonicalizes selected paths without silently selecting other files", func() {
		binary, root, _ := assessmentFixture()
		out := adoptionCommand(binary, root, "propose-adoption", root, assessmentPaths[4], assessmentPaths[0])
		proposal := adoptionDecode(out, root)
		Expect(proposal.Assets).To(HaveLen(2))
		Expect(proposal.Assets[0].Path).To(Equal(assessmentPaths[0]))
		Expect(proposal.Assets[1].Path).To(Equal(assessmentPaths[4]))
		ordered := adoptionCommand(binary, root, "propose-adoption", root, assessmentPaths[0], assessmentPaths[4])
		Expect(ordered).To(Equal(out))
	})
})

type canonicalAdoptionPayload struct {
	Purpose             string               `json:"purpose"`
	SchemaVersion       int                  `json:"schema_version"`
	Scope               string               `json:"scope"`
	ReferenceRevision   string               `json:"reference_revision"`
	PriorOrigin         string               `json:"prior_origin"`
	OwnershipAuthorized bool                 `json:"ownership_authorized"`
	RootIdentity        adoptionRootIdentity `json:"root_identity"`
	Selection           []string             `json:"selection"`
	Assets              []adoptionAsset      `json:"assets"`
}

func adoptionCheckDigest(proposal adoptionProposal) {
	GinkgoHelper()
	selection := make([]string, 0, len(proposal.Assets))
	for _, asset := range proposal.Assets {
		selection = append(selection, asset.Path)
	}
	payload := canonicalAdoptionPayload{proposal.Purpose, proposal.SchemaVersion, proposal.Scope, proposal.ReferenceRevision, proposal.PriorOrigin, proposal.OwnershipAuthorized, proposal.RootIdentity, selection, proposal.Assets}
	data, err := json.Marshal(payload)
	Expect(err).NotTo(HaveOccurred())
	digest := sha256.Sum256(append([]byte("software-factory/legacy-adoption/v1\n"), data...))
	Expect(proposal.ProposalDigest).To(Equal(hex.EncodeToString(digest[:])))
}
func adoptionPlanDecode(out cliResult, digest string, selection []string) map[string]any {
	GinkgoHelper()
	Expect(out.status).To(Equal(2), "%+v", out)
	Expect(out.stderr).To(BeEmpty())
	Expect(out.stdout).To(HaveSuffix("\n"))
	decoder := json.NewDecoder(strings.NewReader(out.stdout))
	var value map[string]any
	Expect(decoder.Decode(&value)).To(Succeed())
	var extra any
	Expect(decoder.Decode(&extra)).To(Equal(io.EOF))
	Expect(value).To(HaveLen(15))
	Expect(value).To(HaveKeyWithValue("ownership_basis", "explicit_operator_adoption"))
	Expect(value).To(HaveKeyWithValue("proposal_digest", digest))
	Expect(value).To(HaveKeyWithValue("prior_origin", "unproven"))
	Expect(value).To(HaveKeyWithValue("source_authentication", "unverified_local"))
	for _, key := range []string{"activation_ready", "rollback_ready", "applicable"} {
		Expect(value).To(HaveKeyWithValue(key, false))
	}
	Expect(value).To(HaveKeyWithValue("ownership_authorized", len(selection) == 6))
	Expect(value["authorized_paths"]).To(Equal(toAnyStrings(selection)))
	blockers := []string{"target_authentication_unproven", "runtime_qualification_pending", "transition_quiescence_unproven", "verified_recovery_pending"}
	if len(selection) != 6 {
		blockers = append([]string{"ownership_scope_incomplete"}, blockers...)
	}
	Expect(value["blockers"]).To(Equal(toAnyStrings(blockers)))
	return value
}
func toAnyStrings(values []string) []any {
	result := make([]any, len(values))
	for i, value := range values {
		result[i] = value
	}
	return result
}

var _ = Describe("G4 explicit legacy adoption authority", func() {
	// per docs/adr/0081-explicit-legacy-asset-adoption.md:56
	It("binds the canonical digest to all independently observed identity fields", func() {
		binary, root, _ := assessmentFixture()
		proposal := adoptionDecode(adoptionCommand(binary, root, "propose-adoption", append([]string{root}, assessmentPaths...)...), root)
		adoptionCheckDigest(proposal)
		info, err := os.Stat(root)
		Expect(err).NotTo(HaveOccurred())
		stat := info.Sys().(*syscall.Stat_t)
		Expect(proposal.RootIdentity.Device).To(Equal(fmt.Sprint(stat.Dev)))
		Expect(proposal.RootIdentity.Inode).To(Equal(fmt.Sprint(stat.Ino)))
		for _, asset := range proposal.Assets {
			info, err := os.Stat(filepath.Join(root, asset.Path))
			Expect(err).NotTo(HaveOccurred())
			stat := info.Sys().(*syscall.Stat_t)
			identity := asset.Identity
			Expect(identity.Device).To(Equal(fmt.Sprint(stat.Dev)))
			Expect(identity.Inode).To(Equal(fmt.Sprint(stat.Ino)))
			Expect(identity.MtimeSeconds).To(Equal(strconv.FormatInt(info.ModTime().Unix(), 10)))
			Expect(identity.MtimeNanoseconds).To(Equal(info.ModTime().Nanosecond()))
			reflected := reflect.ValueOf(stat).Elem()
			ctime := reflected.FieldByName("Ctim")
			if !ctime.IsValid() {
				ctime = reflected.FieldByName("Ctimespec")
			}
			Expect(ctime.IsValid()).To(BeTrue(), "qualified platform ctime field")
			Expect(identity.CtimeSeconds).To(Equal(strconv.FormatInt(ctime.FieldByName("Sec").Int(), 10)))
			Expect(int64(identity.CtimeNanoseconds)).To(Equal(ctime.FieldByName("Nsec").Int()))
		}
	})
	// per docs/adr/0081-explicit-legacy-asset-adoption.md:86
	DescribeTable("authorizes only the explicitly confirmed canonical selection", func(full bool) {
		binary, root, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		selection := assessmentPaths
		if !full {
			selection = []string{assessmentPaths[0], assessmentPaths[4]}
		}
		proposal := adoptionDecode(adoptionCommand(binary, root, "propose-adoption", append([]string{root}, selection...)...), root)
		beforeRoot, beforeSource := assessmentTree(root), assessmentTree(source)
		out := adoptionCommand(binary, root, "plan-adopted", append([]string{root, source, proposal.ProposalDigest}, selection...)...)
		result := adoptionPlanDecode(out, proposal.ProposalDigest, selection)
		Expect(result["counts"].(map[string]any)["retain"]).To(Equal(float64(6)))
		Expect(assessmentTree(root)).To(Equal(beforeRoot))
		Expect(assessmentTree(source)).To(Equal(beforeSource))
		reversed := append([]string(nil), selection...)
		for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
			reversed[i], reversed[j] = reversed[j], reversed[i]
		}
		Expect(adoptionCommand(binary, root, "plan-adopted", append([]string{root, source, proposal.ProposalDigest}, reversed...)...)).To(Equal(out))
	}, Entry("all six", true), Entry("partial", false))
	// per docs/adr/0081-explicit-legacy-asset-adoption.md:95
	It("preserves unselected customization while retaining incomplete ownership scope", func() {
		binary, root, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		selection := []string{assessmentPaths[0]}
		writeFixture(filepath.Join(root, assessmentPaths[1]), []byte("PRIVATE_CUSTOM"), 0755)
		proposal := adoptionDecode(adoptionCommand(binary, root, "propose-adoption", root, selection[0]), root)
		out := adoptionCommand(binary, root, "plan-adopted", root, source, proposal.ProposalDigest, selection[0])
		result := adoptionPlanDecode(out, proposal.ProposalDigest, selection)
		row := result["assets"].([]any)[1].(map[string]any)
		Expect(row["action"]).To(Equal("preserve_customized"))
		Expect(out.stdout).NotTo(ContainSubstring("PRIVATE_CUSTOM"))
	})
	// per docs/adr/0081-explicit-legacy-asset-adoption.md:66
	It("cannot gain authority from saved proposal JSON, receipts, configuration or environment", func() {
		binary, root, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		proposal := adoptionCommand(binary, root, "propose-adoption", root, assessmentPaths[0])
		adoptionDecode(proposal, root)
		for _, tree := range []string{root, source} {
			for _, path := range []string{".factory-version", ".factory/ownership.json", "factory.yaml", "proposal.json"} {
				writeFixture(filepath.Join(tree, path), []byte(proposal.stdout), 0600)
			}
		}
		env := []string{"FACTORY_ADOPTION_DIGEST=forged", "FACTORY_OWNERSHIP_AUTHORIZED=true", "FACTORY_MIGRATION_TRUSTED=true"}
		ordinary := loopProcess(root, filepath.Join(binary, "factory"), []string{"migration", "plan", root, source}, env)
		migrationPlanDecode(ordinary, root, source)
		refused := loopProcess(root, filepath.Join(binary, "factory"), []string{"migration", "plan-adopted", root, source, strings.Repeat("0", 64), assessmentPaths[0]}, env)
		Expect(refused.status).To(Equal(2))
		Expect(refused.stdout).To(BeEmpty())
		Expect(refused.stderr).NotTo(BeEmpty())
	})
})
var _ = Describe("G4 explicit legacy adoption refusals", func() {
	// per docs/adr/0081-explicit-legacy-asset-adoption.md:33
	DescribeTable("rejects invalid exact selections before root reads", func(paths []string) {
		binary, cwd := fixture()
		root := filepath.Join(cwd, "missing-root")
		proposed := adoptionCommand(binary, cwd, "propose-adoption", append([]string{root}, paths...)...)
		Expect(proposed.status).To(Equal(2))
		Expect(proposed.stdout).To(BeEmpty())
		adopted := adoptionCommand(binary, cwd, "plan-adopted", append([]string{root, root, strings.Repeat("0", 64)}, paths...)...)
		Expect(adopted.status).To(Equal(2))
		Expect(adopted.stdout).To(BeEmpty())
	}, Entry("none", []string{}), Entry("duplicate", []string{assessmentPaths[0], assessmentPaths[0]}), Entry("unknown", []string{"user-policy"}), Entry("dot alias", []string{"./" + assessmentPaths[0]}), Entry("parent alias", []string{"scripts/lib/../factory-budget.sh"}), Entry("seven", append(append([]string(nil), assessmentPaths...), "extra")))
	// per docs/adr/0081-explicit-legacy-asset-adoption.md:36
	DescribeTable("refuses selected files that cannot grant unchanged-asset authority", func(kind string, status int) {
		binary, root, _ := assessmentFixture()
		path := filepath.Join(root, assessmentPaths[0])
		switch kind {
		case "missing":
			Expect(os.Remove(path)).To(Succeed())
		case "content":
			writeFixture(path, []byte("PRIVATE_EDIT"), 0755)
		case "permissions":
			Expect(os.Chmod(path, 0600)).To(Succeed())
		case "special":
			Expect(os.Chmod(path, 0755|os.ModeSetuid)).To(Succeed())
		case "symlink":
			Expect(os.Remove(path)).To(Succeed())
			Expect(os.Symlink("/dev/null", path)).To(Succeed())
		case "hardlink":
			Expect(os.Link(path, filepath.Join(binary, "extra-link"))).To(Succeed())
		case "FIFO":
			Expect(os.Remove(path)).To(Succeed())
			Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
		case "read error":
			if os.Geteuid() == 0 {
				Skip("requires unprivileged reader")
			}
			Expect(os.Chmod(path, 0000)).To(Succeed())
			DeferCleanup(func() { Expect(os.Chmod(path, 0600)).To(Succeed()) })
		}
		out := adoptionCommand(binary, root, "propose-adoption", root, assessmentPaths[0])
		Expect(out.status).To(Equal(status))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).NotTo(ContainSubstring("PRIVATE_EDIT"))
	}, Entry("missing", "missing", 2), Entry("content edit", "content", 2), Entry("mode edit", "permissions", 2), Entry("special mode", "special", 2), Entry("symlink", "symlink", 2), Entry("hardlink", "hardlink", 2), Entry("FIFO", "FIFO", 2), Entry("read error", "read error", 1))
	// per docs/adr/0081-explicit-legacy-asset-adoption.md:72
	DescribeTable("rejects stale or mismatched confirmation without partial authority output", func(kind string) {
		binary, root, _ := assessmentFixture()
		_, source, _ := assessmentFixture()
		proposal := adoptionDecode(adoptionCommand(binary, root, "propose-adoption", root, assessmentPaths[0]), root)
		digest := proposal.ProposalDigest
		path := filepath.Join(root, assessmentPaths[0])
		selection := []string{assessmentPaths[0]}
		switch kind {
		case "short digest":
			digest = "bad"
		case "uppercase digest":
			digest = strings.ToUpper(digest)
		case "wrong digest":
			digest = strings.Repeat("0", 64)
		case "changed selection":
			selection = []string{assessmentPaths[1]}
		case "copied root":
			_, root, _ = assessmentFixture()
		case "new inode":
			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Rename(path, path+".held")).To(Succeed())
			writeFixture(path, data, 0755)
		case "content":
			writeFixture(path, []byte("changed"), 0755)
		case "mode":
			Expect(os.Chmod(path, 0600)).To(Succeed())
		}
		out := adoptionCommand(binary, root, "plan-adopted", append([]string{root, source, digest}, selection...)...)
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).NotTo(BeEmpty())
	}, Entry("malformed", "short digest"), Entry("uppercase", "uppercase digest"), Entry("mismatch", "wrong digest"), Entry("selection differs", "changed selection"), Entry("copied bytes different root", "copied root"), Entry("replacement same bytes", "new inode"), Entry("changed bytes", "content"), Entry("changed mode", "mode"))
})

func adoptionFixtureProposal(root string, references []assessedBytes) adoptionProposal {
	GinkgoHelper()
	info, err := os.Stat(root)
	Expect(err).NotTo(HaveOccurred())
	rootStat := info.Sys().(*syscall.Stat_t)
	proposal := adoptionProposal{SchemaVersion: 1, ReferenceRevision: assessmentRevision, Scope: "g2-budget-loop-six", Purpose: "adopt_known_legacy_assets", PriorOrigin: "unproven", RootIdentity: adoptionRootIdentity{fmt.Sprint(rootStat.Dev), fmt.Sprint(rootStat.Ino)}}
	for i, path := range assessmentPaths {
		info, err := os.Stat(filepath.Join(root, path))
		Expect(err).NotTo(HaveOccurred())
		stat := info.Sys().(*syscall.Stat_t)
		reflected := reflect.ValueOf(stat).Elem()
		ctime := reflected.FieldByName("Ctim")
		if !ctime.IsValid() {
			ctime = reflected.FieldByName("Ctimespec")
		}
		Expect(ctime.IsValid()).To(BeTrue())
		identity := adoptionAssetIdentity{Device: fmt.Sprint(stat.Dev), Inode: fmt.Sprint(stat.Ino), Type: "regular", Mode: references[i].Mode, Bytes: references[i].Bytes, MtimeSeconds: strconv.FormatInt(info.ModTime().Unix(), 10), MtimeNanoseconds: info.ModTime().Nanosecond(), CtimeSeconds: strconv.FormatInt(ctime.FieldByName("Sec").Int(), 10), CtimeNanoseconds: int(ctime.FieldByName("Nsec").Int())}
		proposal.Assets = append(proposal.Assets, adoptionAsset{path, references[i], identity, references[i]})
	}
	payload := canonicalAdoptionPayload{proposal.Purpose, proposal.SchemaVersion, proposal.Scope, proposal.ReferenceRevision, proposal.PriorOrigin, proposal.OwnershipAuthorized, proposal.RootIdentity, assessmentPaths, proposal.Assets}
	data, err := json.Marshal(payload)
	Expect(err).NotTo(HaveOccurred())
	digest := sha256.Sum256(append([]byte("software-factory/legacy-adoption/v1\n"), data...))
	proposal.ProposalDigest = hex.EncodeToString(digest[:])
	return proposal
}

var _ = Describe("G4 explicit legacy adoption supplied digest core", func() {
	// per docs/adr/0081-explicit-legacy-asset-adoption.md:72
	It("accepts an independently computed full digest only as a blocked adopted plan", func() {
		binary, root, references := assessmentFixture()
		_, source, _ := assessmentFixture()
		proposal := adoptionFixtureProposal(root, references)
		out := adoptionCommand(binary, root, "plan-adopted", append([]string{root, source, proposal.ProposalDigest}, assessmentPaths...)...)
		adoptionPlanDecode(out, proposal.ProposalDigest, assessmentPaths)
	})
})
