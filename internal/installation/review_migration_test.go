package installation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/ciworkflow"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/installationimage"
	"github.com/anoop2811/software-factory-template/internal/staging"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const reviewMigrationPath = ".github/workflows/adversarial-review.yml"
const reviewMigrationRevision = "3794e6cc8fa66eb02504e753b0ad11955f5cf85a"

// Frozen predecessors are Git data; the target uses the actual current package
// files and template as image data, without compiling or executing either.
// per docs/adr/0154-installed-review-workflow-migration.md:59
func reviewMigrationImage(revision string, current bool) staging.Inspection {
	GinkgoHelper()
	release := ciworkflow.Release{Tag: "v0.3.0", OriginTarget: "linux/amd64", SourceRevision: revision, Repository: ciPreviewRelease.Repository}
	image := ciPreviewOldImage(release)
	paths := []string{"command.go", "files.go", "mutation.go", "settings.go"}
	if current {
		paths = nil
		entries, err := os.ReadDir("../reviewlanecmd")
		Expect(err).NotTo(HaveOccurred())
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
				paths = append(paths, entry.Name())
			}
		}
	}
	for _, name := range append(paths, "template") {
		path := "internal/reviewlanecmd/" + name
		if name == "template" {
			path = "packs/review-lane/review-pr.yml"
		}
		var data []byte
		if current {
			var err error
			data, err = os.ReadFile(filepath.Join("../..", path))
			Expect(err).NotTo(HaveOccurred())
		} else {
			data = immutableRecipeSource(revision, path)
		}
		reviewMigrationSetAsset(&image, path, data)
	}
	if current {
		image.Manifest.Version, image.Runtime.Version = ciPreviewRelease.Tag, ciPreviewRelease.Tag
		image.Manifest.SourceRevision = ciPreviewRelease.SourceRevision
	}
	return image
}

// per docs/adr/0154-installed-review-workflow-migration.md:63
func reviewMigrationSetAsset(image *staging.Inspection, path string, data []byte) {
	image.Assets[path] = data
	rows := []installationimage.Asset{}
	for _, row := range image.Manifest.Assets {
		if row.Path != path {
			rows = append(rows, row)
		}
	}
	rows = append(rows, installationimage.Asset{Path: path, Mode: "100644", Bytes: int64(len(data)), SHA256: fmt.Sprintf("%x", sha256.Sum256(data))})
	image.Manifest.Assets = rows
}

// Restricted secret names have no sed metacharacters, so both frozen renderers
// prepend this literal header and replace only the original secret marker.
// per docs/adr/0154-installed-review-workflow-migration.md:60
func reviewMigrationRender(template []byte, secret string) []byte {
	return append([]byte("# Managed by: factory review-lane. Remove with: ./factory review-lane disable\n"), bytes.ReplaceAll(template, []byte("__REVIEW_API_KEY_SECRET__"), []byte(secret))...)
}

// Exercise the existing confined-tree projection with already inspected images;
// archive authentication and public transaction replay have separate real journeys.
// per docs/adr/0154-installed-review-workflow-migration.md:59
func reviewMigrationProjection(root string, request Request, before *staging.Inspection, target staging.Inspection) map[string]any {
	GinkgoHelper()
	tree, err := installationfs.Open(context.Background(), root)
	Expect(err).NotTo(HaveOccurred())
	defer func() { Expect(tree.Close()).To(Succeed()) }()
	observation := ciWorkflowObservation{targetImage: &target, before: map[string]*Image{}, data: map[string][]byte{}, targets: map[string][]byte{}}
	return ciPreviewMap(inspectCIWorkflow(context.Background(), tree, request, &ciPreviewRelease, before, &observation))
}

// per docs/adr/0154-installed-review-workflow-migration.md:63
func reviewMigrationCorrupt(image *staging.Inspection, variant string) {
	const template = "packs/review-lane/review-pr.yml"
	const renderer = "internal/reviewlanecmd/mutation.go"
	switch variant {
	case "missing-template":
		delete(image.Assets, template)
	case "missing-renderer":
		delete(image.Assets, renderer)
	case "changed-renderer":
		reviewMigrationSetAsset(image, renderer, append(append([]byte{}, image.Assets[renderer]...), []byte("\n// unqualified recipe change\n")...))
	case "extra-renderer":
		reviewMigrationSetAsset(image, "internal/reviewlanecmd/unqualified.go", []byte("package reviewlanecmd\n"))
	default:
		for index, row := range image.Manifest.Assets {
			if row.Path != template {
				continue
			}
			switch variant {
			case "duplicate-template":
				image.Manifest.Assets = append(image.Manifest.Assets, row)
			case "missing-manifest-row":
				image.Manifest.Assets = append(image.Manifest.Assets[:index], image.Manifest.Assets[index+1:]...)
			case "wrong-digest":
				image.Manifest.Assets[index].SHA256 = strings.Repeat("0", 64)
			case "wrong-mode":
				image.Manifest.Assets[index].Mode = "100755"
			case "wrong-size":
				image.Manifest.Assets[index].Bytes++
			}
			return
		}
		Fail("template manifest row must exist before corrupting it")
	}
}

// This is the original schema-1 finite row set, independent of today's registry.
// per docs/adr/0154-installed-review-workflow-migration.md:77
func reviewMigrationV1() (*ciWorkflowContext, []journalEntry) {
	values := map[string]string{"PACKS": "", "PROTECTED_PATH": "internal"}
	ci := &ciWorkflowContext{SchemaVersion: 1, Current: values, Historical: maps.Clone(values)}
	entries := []journalEntry{}
	for _, path := range []string{"ci.yml", "go-pack.yml", "typescript-pack.yml", "java-pack.yml", "factory.yml"} {
		entries = append(entries, journalEntry{Path: ".github/workflows/" + path})
	}
	return ci, entries
}

var _ = Describe("Installed review migration replay schema", func() {
	// per docs/adr/0154-installed-review-workflow-migration.md:77
	It("preserves the original five-row context without selecting the review workflow", func() {
		ci, entries := reviewMigrationV1()
		Expect(validateCIContext(context.Background(), ci)).To(Succeed())
		remaining, complete := transactionRuntimeEntries(entries, ci)
		Expect(complete).To(BeTrue())
		Expect(remaining).To(BeEmpty())
		Expect(selectedTransactionPath(reviewMigrationPath, ci)).To(BeFalse())
		raw, err := json.Marshal(ci)
		Expect(err).NotTo(HaveOccurred())
		var record map[string]any
		Expect(json.Unmarshal(raw, &record)).To(Succeed())
		Expect(record).NotTo(HaveKey("review"))
	})

	// per docs/adr/0154-installed-review-workflow-migration.md:78
	DescribeTable("refuses corrupt finite workflow rows under the original schema", func(kind string) {
		ci, entries := reviewMigrationV1()
		switch kind {
		case "injected":
			entries = append(entries, journalEntry{Path: reviewMigrationPath})
		case "substituted":
			entries[0].Path = reviewMigrationPath
		case "duplicate":
			entries = append(entries, entries[0])
		case "missing":
			entries = entries[1:]
		}
		Expect(knownTransactionEntries(entries, ci)).To(BeFalse())
	}, Entry("injected review row", "injected"), Entry("same-count review substitution", "substituted"), Entry("duplicate ordinary row", "duplicate"), Entry("missing ordinary row", "missing"))

	// per docs/adr/0154-installed-review-workflow-migration.md:77
	It("rejects unsupported context versions and cannot promote a review context into schema one", func() {
		for _, version := range []int{0, 1, 2, 99} {
			ci, _ := reviewMigrationV1()
			raw, err := json.Marshal(ci)
			Expect(err).NotTo(HaveOccurred())
			var record map[string]any
			Expect(json.Unmarshal(raw, &record)).To(Succeed())
			record["schema_version"] = version
			if version == 1 {
				record["review"] = map[string]any{"secret": "CUSTOM_REVIEW_KEY", "present": true}
			}
			raw, err = json.Marshal(record)
			Expect(err).NotTo(HaveOccurred())
			Expect(json.Unmarshal(raw, ci)).To(Succeed())
			Expect(validateCIContext(context.Background(), ci)).NotTo(Succeed(), "version %d must not admit an absent or misplaced review context", version)
		}
	})
})

var _ = Describe("Installed review migration historical ownership", Ordered, ContinueOnFailure, func() {
	var target, old staging.Inspection
	BeforeAll(func() {
		target = reviewMigrationImage(reviewMigrationRevision, true)
		old = reviewMigrationImage(reviewMigrationRevision, false)
	})

	// per docs/adr/0154-installed-review-workflow-migration.md:59
	// per docs/adr/0154-installed-review-workflow-migration.md:107
	DescribeTable("qualifies exact immutable enabled projections with default and custom secret names", func(profile, revision string) {
		for _, selection := range []struct{ provider, configured, secret string }{
			{"openrouter", "", "OPENROUTER_API_KEY"}, {"anthropic", "", "ANTHROPIC_API_KEY"}, {"openai", "", "OPENAI_API_KEY"}, {"inherit", "TEAM_REVIEW_KEY", "TEAM_REVIEW_KEY"},
		} {
			By(selection.provider + "/" + selection.secret)
			root, request := ciPreviewFixture(profile, ciPreviewVariants[0])
			cfg := ciPreviewConfig(ciPreviewVariants[0]) + fmt.Sprintf("review_lane: on\nmodel_provider: %s\nreview_api_key_secret: %s\n", selection.provider, selection.configured)
			ciPreviewWrite(root, "factory.yaml", []byte(cfg), 0644)
			original := reviewMigrationRender(immutableRecipeSource(revision, "packs/review-lane/review-pr.yml"), selection.secret)
			ciPreviewWrite(root, reviewMigrationPath, original, 0644)
			var before *staging.Inspection
			if profile == "go-hybrid-v1" {
				image := old
				if revision != reviewMigrationRevision {
					image = reviewMigrationImage(revision, false)
				}
				before = &image
				request.Before = staging.Options{Version: image.Manifest.Version, Revision: revision, Target: image.Manifest.Target}
			}
			report := reviewMigrationProjection(root, request, before, target)
			rows := ciPreviewRows(report)
			Expect(rows).To(HaveLen(6))
			row := rows[reviewMigrationPath]
			Expect(row["disposition"]).To(Equal("replace"))
			Expect(row["before_sha256"]).To(Equal(fmt.Sprintf("%x", sha256.Sum256(original))))
			expected := reviewMigrationRender(target.Assets["packs/review-lane/review-pr.yml"], selection.secret)
			Expect(row["after_sha256"]).To(Equal(fmt.Sprintf("%x", sha256.Sum256(expected))))
			Expect(report["conflicts"]).To(BeEmpty())
			Expect(os.ReadFile(filepath.Join(root, reviewMigrationPath))).To(Equal(original))
		}
	}, Entry("Bash 0.1.6", "v0.1.6", "b71ecc32e07ecd87eb330ba8e497c86612f92acd"), Entry("last Bash baseline", "bash-baseline", "76952eaa63aebd1ecd282f5ab51dd7c3627cb497"), Entry("copied Go predecessor", "go-hybrid-v1", "95486ecdd3868a319a0e6362ee99cafd8f154200"), Entry("reviewed Go predecessor", "go-hybrid-v1", reviewMigrationRevision))

	// per docs/adr/0154-installed-review-workflow-migration.md:63
	DescribeTable("refuses incomplete or unqualified image evidence without granting review ownership", func(side string) {
		root, request := ciPreviewFixture("go-hybrid-v1", ciPreviewVariants[0])
		ciPreviewWrite(root, "factory.yaml", []byte(ciPreviewConfig(ciPreviewVariants[0])+"review_lane: on\nreview_api_key_secret: TEAM_REVIEW_KEY\n"), 0644)
		original := reviewMigrationRender(old.Assets["packs/review-lane/review-pr.yml"], "TEAM_REVIEW_KEY")
		ciPreviewWrite(root, reviewMigrationPath, original, 0644)
		request.Before = staging.Options{Version: old.Manifest.Version, Revision: old.Manifest.SourceRevision, Target: old.Manifest.Target}
		healthy := reviewMigrationProjection(root, request, &old, target)
		Expect(ciPreviewRows(healthy)[reviewMigrationPath]["disposition"]).To(Equal("replace"))
		for _, variant := range []string{"missing-template", "missing-renderer", "changed-renderer", "extra-renderer", "duplicate-template", "missing-manifest-row", "wrong-digest", "wrong-mode", "wrong-size"} {
			By(side + "/" + variant)
			before, after := old, target
			selected := &before
			if side == "target" {
				selected = &after
			}
			selected.Assets = maps.Clone(selected.Assets)
			selected.Manifest.Assets = append([]installationimage.Asset{}, selected.Manifest.Assets...)
			reviewMigrationCorrupt(selected, variant)
			report := reviewMigrationProjection(root, request, &before, after)
			Expect(ciPreviewRows(report)[reviewMigrationPath]["disposition"]).To(Equal("preserve"))
			Expect(report["conflicts"]).To(ContainElement(HaveKeyWithValue("path", reviewMigrationPath)))
			Expect(os.ReadFile(filepath.Join(root, reviewMigrationPath))).To(Equal(original))
		}
	}, Entry("predecessor image", "before"), Entry("target image", "target"))

	// per docs/adr/0154-installed-review-workflow-migration.md:233
	// per docs/adr/0154-installed-review-workflow-migration.md:239
	DescribeTable("attributes final projection failures to the responsible workflow", func(variant, expectedPath string) {
		root, request := ciPreviewFixture("v0.1.6", ciPreviewVariants[0])
		cfg := []byte(ciPreviewConfig(ciPreviewVariants[0]) + "review_lane: on\nreview_api_key_secret: TEAM_REVIEW_KEY\n")
		ciPreviewWrite(root, "factory.yaml", cfg, 0644)
		original := reviewMigrationRender(immutableRecipeSource("b71ecc32e07ecd87eb330ba8e497c86612f92acd", "packs/review-lane/review-pr.yml"), "TEAM_REVIEW_KEY")
		ciPreviewWrite(root, reviewMigrationPath, original, 0644)
		image := target
		image.Assets = maps.Clone(target.Assets)
		image.Manifest.Assets = append([]installationimage.Asset{}, target.Manifest.Assets...)
		switch variant {
		case "missing-marker":
			reviewMigrationSetAsset(&image, "packs/review-lane/review-pr.yml", []byte("name: Target review without a secret marker\n"))
		case "review-renderer":
			reviewMigrationCorrupt(&image, "changed-renderer")
		case "ordinary-renderer":
			const path = "internal/ciworkflow/workflow.go"
			reviewMigrationSetAsset(&image, path, append(append([]byte{}, image.Assets[path]...), []byte("\n// unqualified ordinary renderer\n")...))
		}
		request.CIRelease = &ciPreviewRelease
		planned := plan{request: request, image: image, payload: map[string][]byte{}, before: map[string][]byte{}}
		tree, err := installationfs.Open(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		defer func() { Expect(tree.Close()).To(Succeed()) }()
		Expect(planned.planCI(context.Background(), tree)).To(Succeed(), "qualification conflicts remain a proposal rather than an operational failure")
		Expect(planned.proposal.Conflicts).NotTo(BeEmpty())
		Expect(planned.proposal.Blockers).To(Equal(planned.proposal.Conflicts))
		Expect(planned.proposal.CIWorkflow.Conflicts).To(Equal(planned.proposal.Conflicts))
		for _, issue := range planned.proposal.Conflicts {
			Expect(issue.Path).To(Equal(expectedPath), "a review failure must not be duplicated under ordinary factory.yml")
		}
		Expect(os.ReadFile(filepath.Join(root, reviewMigrationPath))).To(Equal(original))
		Expect(os.ReadFile(filepath.Join(root, "factory.yaml"))).To(Equal(cfg))
		if variant == "ordinary-renderer" {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := planned.planCI(ctx, tree)
			Expect(errors.Is(err, context.Canceled)).To(BeTrue())
			Expect(ErrorStatus(err)).To(Equal(1), "cancellation remains an operational failure")
		}
	}, Entry("review template without marker", "missing-marker", reviewMigrationPath), Entry("unsupported review renderer", "review-renderer", reviewMigrationPath), Entry("ordinary renderer fallback", "ordinary-renderer", ".github/workflows/factory.yml"))
})
