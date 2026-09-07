package acceptance_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const baselineCommit = "76952eaa63aebd1ecd282f5ab51dd7c3627cb497"

type inventoryAsset struct {
	Path              string `json:"path"`
	Mode              string `json:"mode"`
	SHA256            string `json:"sha256"`
	GitBlob           string `json:"git_blob"`
	Bytes             int    `json:"bytes"`
	Classification    string `json:"classification"`
	Stage             string `json:"stage"`
	Disposition       string `json:"disposition"`
	LegacyLogicAction string `json:"legacy_logic_action"`
	Reason            string `json:"reason"`
}

type assetInventory struct {
	SchemaVersion  int              `json:"schema_version"`
	BaselineCommit string           `json:"baseline_commit"`
	AssetCount     int              `json:"asset_count"`
	Assets         []inventoryAsset `json:"assets"`
}

// Obtain raw immutable Git blobs: working-tree files may already be converted,
// and a symlink's bytes are its target text, not the dereferenced file contents.
// per specs/001-go-runtime-conversion.md:304
func baselineAssets() map[string]inventoryAsset {
	GinkgoHelper()
	root, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	tree := exec.Command("git", "ls-tree", "-r", "--full-tree", "-z", baselineCommit)
	tree.Dir = root
	output, err := tree.Output()
	Expect(err).NotTo(HaveOccurred(), "baseline commit must be available, including in CI")
	assets := make(map[string]inventoryAsset)
	var paths []string
	var blobRequests strings.Builder
	for _, record := range bytes.Split(output, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		parts := strings.SplitN(string(record), "\t", 2)
		Expect(parts).To(HaveLen(2))
		fields := strings.Fields(parts[0])
		Expect(fields).To(HaveLen(3))
		Expect(fields[1]).To(Equal("blob"), "submodules require an explicit inventory contract")
		path := parts[1]
		assets[path] = inventoryAsset{Path: path, Mode: fields[0], GitBlob: fields[2]}
		paths = append(paths, path)
		blobRequests.WriteString(fields[2] + "\n")
	}
	Expect(assets).NotTo(BeEmpty())
	blobs := exec.Command("git", "cat-file", "--batch")
	blobs.Dir = root
	blobs.Stdin = strings.NewReader(blobRequests.String())
	output, err = blobs.Output()
	Expect(err).NotTo(HaveOccurred())
	reader := bufio.NewReader(bytes.NewReader(output))
	for _, path := range paths {
		header, err := reader.ReadString('\n')
		Expect(err).NotTo(HaveOccurred())
		fields := strings.Fields(header)
		Expect(fields).To(HaveLen(3))
		Expect(fields[0]).To(Equal(assets[path].GitBlob))
		Expect(fields[1]).To(Equal("blob"))
		size, err := strconv.Atoi(fields[2])
		Expect(err).NotTo(HaveOccurred())
		content := make([]byte, size)
		_, err = io.ReadFull(reader, content)
		Expect(err).NotTo(HaveOccurred())
		separator, err := reader.ReadByte()
		Expect(err).NotTo(HaveOccurred())
		Expect(separator).To(Equal(byte('\n')))
		digest := sha256.Sum256(content)
		asset := assets[path]
		asset.Bytes, asset.SHA256 = size, hex.EncodeToString(digest[:])
		assets[path] = asset
	}
	return assets
}

func member(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

// This independent evaluator compares the recorded map against Git's immutable
// baseline, rather than regenerating expected classifications from production.
// per specs/001-go-runtime-conversion.md:586
func validateInventory(inventory assetInventory, baseline map[string]inventoryAsset) error {
	if inventory.SchemaVersion != 1 || inventory.BaselineCommit != baselineCommit {
		return fmt.Errorf("unsupported schema or baseline")
	}
	if inventory.AssetCount != len(inventory.Assets) {
		return fmt.Errorf("asset_count differs from the entry count")
	}
	seen := make(map[string]bool)
	for _, asset := range inventory.Assets {
		if seen[asset.Path] {
			return fmt.Errorf("duplicate asset: %s", asset.Path)
		}
		seen[asset.Path] = true
		expected, exists := baseline[asset.Path]
		if !exists {
			return fmt.Errorf("unexpected asset: %s", asset.Path)
		}
		if asset.Mode != expected.Mode || asset.SHA256 != expected.SHA256 || asset.GitBlob != expected.GitBlob || asset.Bytes != expected.Bytes {
			return fmt.Errorf("baseline identity differs: %s", asset.Path)
		}
		if !member(asset.Classification,
			"acceptance-test-or-fixture", "adopter-configuration", "adopter-example-hook",
			"build-and-ci-orchestration", "canonical-agent-instructions", "declarative-pack-asset",
			"documentation-and-governance", "evaluation-fixture-or-document", "generated-native-configuration",
			"internal-runtime", "knowledge-and-provenance-content", "language-pack-hook-adapter",
			"native-plugin-adapter", "presentation-asset", "project-and-release-metadata",
			"public-bootstrap-adapter", "public-command-adapter", "public-eval-adapter",
			"public-hook-adapter", "sourceable-library-adapter") {
			return fmt.Errorf("unclassified asset: %s", asset.Path)
		}
		if !member(asset.Stage, "G0", "G1", "G2", "G3", "G4", "G5") ||
			!member(asset.Disposition, "retain", "convert", "retire") ||
			!member(asset.LegacyLogicAction, "none", "convert", "audit-embedded-helpers", "replace-active-test-execution") ||
			strings.TrimSpace(asset.Reason) == "" {
			return fmt.Errorf("incomplete migration decision: %s", asset.Path)
		}
	}
	for path := range baseline {
		if !seen[path] {
			return fmt.Errorf("missing baseline asset: %s", path)
		}
	}
	return nil
}

var _ = Describe("The G0 baseline inventory gate", func() {
	var inventory assetInventory
	var baseline map[string]inventoryAsset

	BeforeEach(func() {
		contents, err := os.ReadFile(filepath.Join("..", "docs", "migration", "baseline-assets.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(json.Unmarshal(contents, &inventory)).To(Succeed())
		baseline = baselineAssets()
	})

	// per specs/001-go-runtime-conversion.md:304
	It("accounts for every immutable baseline asset with its identity and migration decision", func() {
		Expect(validateInventory(inventory, baseline)).To(Succeed())
	})

	// per specs/001-go-runtime-conversion.md:274
	DescribeTable("rejects a damaged inventory in an isolated negative fixture", func(mutate func(*assetInventory), diagnostic string) {
		Expect(validateInventory(inventory, baseline)).To(Succeed())
		mutate(&inventory)
		Expect(validateInventory(inventory, baseline)).To(MatchError(ContainSubstring(diagnostic)))
	},
		Entry("missing asset", func(m *assetInventory) { m.Assets = m.Assets[1:]; m.AssetCount-- }, "missing baseline asset"),
		Entry("duplicate asset", func(m *assetInventory) { m.Assets = append(m.Assets, m.Assets[0]); m.AssetCount++ }, "duplicate asset"),
		Entry("extra asset", func(m *assetInventory) {
			asset := m.Assets[0]
			asset.Path = "not-a-baseline-asset"
			m.Assets = append(m.Assets, asset)
			m.AssetCount++
		}, "unexpected asset"),
		Entry("unclassified asset", func(m *assetInventory) { m.Assets[0].Classification = "" }, "unclassified asset"),
		Entry("altered bytes", func(m *assetInventory) { m.Assets[0].SHA256 = strings.Repeat("0", 64) }, "baseline identity differs"),
		Entry("altered executable mode", func(m *assetInventory) {
			if m.Assets[0].Mode == "100755" {
				m.Assets[0].Mode = "100644"
			} else {
				m.Assets[0].Mode = "100755"
			}
		}, "baseline identity differs"),
		Entry("unknown stage", func(m *assetInventory) { m.Assets[0].Stage = "later" }, "incomplete migration decision"),
		Entry("unknown disposition", func(m *assetInventory) { m.Assets[0].Disposition = "delete-now" }, "incomplete migration decision"),
		Entry("missing rationale", func(m *assetInventory) { m.Assets[0].Reason = " " }, "incomplete migration decision"),
		Entry("incorrect count", func(m *assetInventory) { m.AssetCount++ }, "asset_count differs"),
		Entry("moving baseline", func(m *assetInventory) { m.BaselineCommit = "HEAD" }, "unsupported schema or baseline"),
	)
})
