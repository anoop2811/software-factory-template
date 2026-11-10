package installation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/artifact"
	"github.com/anoop2811/software-factory-template/internal/ciworkflow"
	"github.com/anoop2811/software-factory-template/internal/installationimage"
	"github.com/anoop2811/software-factory-template/internal/staging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

const ciPreviewRevision = "8f2c6ae27f47a1ddbffc76cc10bdceffd424f2c4"

var ciPreviewRelease = ciworkflow.Release{Tag: "v0.3.1", OriginTarget: "linux/amd64", SourceRevision: strings.Repeat("a", 40), Repository: "anoop2811/software-factory-template"}

type ciPreviewVariant struct{ name, packs, build, maven string }

var ciPreviewVariants = []ciPreviewVariant{
	{"none", "", "", "mvn"}, {"go", "go", "", "mvn"}, {"typescript", "typescript", "", "mvn"},
	{"gradle", "java", "gradle", "mvn"}, {"maven", "java", "maven", "mvn"},
	{"go-typescript", "go typescript", "", "mvn"}, {"go-gradle", "go java", "gradle", "mvn"}, {"go-maven", "go java", "maven", "mvn"},
	{"typescript-gradle", "typescript java", "gradle", "mvn"}, {"typescript-maven", "typescript java", "maven", "mvn"},
	{"all-gradle", "go typescript java", "gradle", "mvn"}, {"all-maven", "go typescript java", "maven", "mvn"}, {"maven-wrapper", "java", "maven", "./mvnw"},
}

func ciPreviewWrite(root, path string, body []byte, mode os.FileMode) {
	GinkgoHelper()
	full := filepath.Join(root, path)
	Expect(os.MkdirAll(filepath.Dir(full), 0755)).To(Succeed())
	Expect(os.WriteFile(full, body, mode)).To(Succeed())
}
func ciPreviewConfig(v ciPreviewVariant) string {
	return fmt.Sprintf("project_name: preview\nlanguage_packs: %q\nprotected_paths: internal\ngo_version: \"1.26\"\nnode_version: \"24\"\njava_version: \"25\"\njava_build_tool: %q\n", v.packs, v.build)
}
func ciPreviewFixture(profile string, v ciPreviewVariant) (string, Request) {
	GinkgoHelper()
	root, err := os.MkdirTemp("", "factory-ci-preview-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, root)
	root, err = filepath.EvalSymlinks(root)
	Expect(err).NotTo(HaveOccurred())
	ciPreviewWrite(root, "factory.yaml", []byte(ciPreviewConfig(v)), 0644)
	return root, Request{Profile: profile, Source: staging.Options{Version: ciPreviewRelease.Tag, Revision: ciPreviewRelease.SourceRevision, Target: ciPreviewRelease.OriginTarget}, Inputs: map[string]string{"PROJECT_NAME": "Preview", "PROJECT_SLUG": "preview", "GITHUB_OWNER": "@preview", "OPENCODE_USERNAME": "preview", "PROTECTED_PATH": "internal", "DOCS_ROOT": "docs"}}
}
func ciPreviewMap(result CIWorkflowPreview, err error) map[string]any {
	GinkgoHelper()
	Expect(err).NotTo(HaveOccurred())
	raw, err := json.Marshal(result)
	Expect(err).NotTo(HaveOccurred())
	var object map[string]any
	Expect(json.Unmarshal(raw, &object)).To(Succeed())
	Expect(object["schema_version"]).To(Equal(float64(1)))
	Expect(object["mode"]).To(Equal("ci-workflow-preview"))
	Expect(object["apply_supported"]).To(BeFalse())
	Expect(object["reason"]).NotTo(BeEmpty())
	Expect(object).NotTo(HaveKey("proposal_digest"))
	Expect(object).NotTo(HaveKey("activation_ready"))
	return object
}
func ciPreviewRows(object map[string]any) map[string]map[string]any {
	GinkgoHelper()
	rows := map[string]map[string]any{}
	for _, value := range object["changes"].([]any) {
		row := value.(map[string]any)
		path := row["path"].(string)
		Expect(rows).NotTo(HaveKey(path))
		Expect(row["reason"]).NotTo(BeEmpty())
		Expect(row).NotTo(HaveKey("identity"))
		Expect(row).NotTo(HaveKey("custody"))
		rows[path] = row
	}
	return rows
}

// The old initializer's closed literal replacements are independently applied to
// immutable Git bytes; neither current planner nor reference() supplies this oracle.
// per docs/adr/0147-installed-ci-migration-preview.md:56
func ciPreviewLegacy(revision, path string, v ciPreviewVariant) []byte {
	raw := string(immutableRecipeSource(revision, path))
	if path == ".github/workflows/ci.yml" {
		return []byte(strings.NewReplacer("__PROJECT_NAME__", "Preview", "__PROJECT_SLUG__", "preview", "__GITHUB_OWNER__", "@preview", "__OPENCODE_USERNAME__", "preview", "__DOCS_ROOT__", "docs", "__MUTATION_TARGET__", "./internal/...", "__PROTECTED_PATH__", "internal").Replace(raw))
	}
	return []byte(strings.NewReplacer("__GO_VERSION__", "1.26", "__NODE_VERSION__", "24", "__JAVA_VERSION__", "25", "__MUTATION_TARGET__", "./internal/...", "__PROTECTED_PATH__", "internal", "__MAVEN_COMMAND__", v.maven).Replace(raw))
}

// Parse inert original Go string constants at 8f2c6ae27f47a1ddbffc76cc10bdceffd424f2c4
// without executing either renderer.
// per docs/adr/0147-installed-ci-migration-preview.md:67
func ciPreviewFrozenWorkflow(release ciworkflow.Release, variants ...ciPreviewVariant) []byte {
	GinkgoHelper()
	constants := map[string]string{}
	for _, path := range []string{"internal/ciworkflow/workflow.go", "internal/ciworkflow/application.go"} {
		file, err := parser.ParseFile(token.NewFileSet(), path, immutableRecipeSource(ciPreviewRevision, path), 0)
		Expect(err).NotTo(HaveOccurred())
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			for _, spec := range general.Specs {
				value := spec.(*ast.ValueSpec)
				if len(value.Names) != 1 || len(value.Values) != 1 {
					continue
				}
				literal, ok := value.Values[0].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					continue
				}
				text, err := strconv.Unquote(literal.Value)
				Expect(err).NotTo(HaveOccurred())
				constants[value.Names[0].Name] = text
			}
		}
	}
	for _, key := range []string{"preparationTemplate", "actionlintInstall", "installedChecks", "actionlintCheck", "mockGolden"} {
		Expect(constants[key]).NotTo(BeEmpty(), key)
	}
	variant := ciPreviewVariants[0]
	if len(variants) > 0 {
		variant = variants[0]
	}
	parts := []string{fmt.Sprintf(constants["preparationTemplate"], release.Tag, release.OriginTarget, release.SourceRevision, release.Repository)}
	goPack, nodePack, javaPack := strings.Contains(variant.packs, "go"), strings.Contains(variant.packs, "typescript"), strings.Contains(variant.packs, "java")
	if goPack {
		parts = append(parts, fmt.Sprintf(constants["goSetup"], strconv.Quote("1.26")))
	}
	if nodePack {
		parts = append(parts, fmt.Sprintf(constants["nodeSetup"], strconv.Quote("24")))
	}
	if javaPack {
		parts = append(parts, fmt.Sprintf(constants["javaSetup"], strconv.Quote("25")))
	}
	parts = append(parts, constants["actionlintInstall"])
	if goPack {
		parts = append(parts, constants["goToolsInstall"])
	}
	parts = append(parts, constants["installedChecks"], constants["actionlintCheck"])
	if goPack {
		parts = append(parts, constants["goSecurity"])
	}
	if nodePack || javaPack {
		parts = append(parts, constants["osvScan"])
	}
	if javaPack && variant.build == "gradle" {
		parts = append(parts, constants["gradleChecks"])
	}
	if goPack {
		parts = append(parts, fmt.Sprintf(constants["goMutation"], strconv.Quote("./internal")))
	}
	if nodePack {
		parts = append(parts, constants["nodeMutation"])
	}
	if javaPack && variant.build == "gradle" {
		parts = append(parts, constants["gradleMutation"])
	}
	parts = append(parts, constants["mockGolden"])
	return []byte(strings.Join(parts, ""))
}

// The full inert image comes from the independently pinned merged predecessor.
// per docs/adr/0147-installed-ci-migration-preview.md:63
func ciPreviewOldImage(release ciworkflow.Release) staging.Inspection {
	GinkgoHelper()
	command := exec.Command("git", "--no-replace-objects", "ls-tree", "-r", "--name-only", release.SourceRevision) // #nosec G204 -- closed immutable Git revision, read-only oracle.
	command.Dir = "../.."
	listing, err := command.Output()
	Expect(err).NotTo(HaveOccurred())
	image := staging.Inspection{Manifest: installationimage.Manifest{Version: release.Tag, Target: release.OriginTarget, SourceRevision: release.SourceRevision}, Runtime: artifact.Metadata{Version: release.Tag, Target: release.OriginTarget}, Assets: map[string][]byte{}}
	for _, path := range strings.Split(strings.TrimSpace(string(listing)), "\n") {
		if (strings.HasPrefix(path, "internal/initcmd/") || strings.HasPrefix(path, "internal/initinventory/") || strings.HasPrefix(path, "internal/ciworkflow/")) && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "workflows/ci.yml") {
			image.Assets[path] = immutableRecipeSource(release.SourceRevision, path)
			image.Manifest.Assets = append(image.Manifest.Assets, installationimage.Asset{Path: path, Mode: "100644", SHA256: fmt.Sprintf("%x", sha256.Sum256(image.Assets[path])), Bytes: int64(len(image.Assets[path]))})
		}
	}
	return image
}

var _ = Describe("Installed CI migration preview", func() {
	// per docs/adr/0147-installed-ci-migration-preview.md:56
	// per docs/adr/0147-installed-ci-migration-preview.md:89
	DescribeTable("recognizes the independently pinned copied workflow projection for every pack union", func(profile, revision string) {
		for _, variant := range ciPreviewVariants {
			By(variant.name)
			root, request := ciPreviewFixture(profile, variant)
			expected := map[string][]byte{".github/workflows/ci.yml": ciPreviewLegacy(revision, ".github/workflows/ci.yml", variant)}
			for _, pack := range strings.Fields(variant.packs) {
				source := "packs/" + pack + "/workflows/ci.yml"
				if pack == "java" && variant.build == "maven" {
					source = "packs/java/maven/workflows/ci.yml"
				}
				expected[".github/workflows/"+pack+"-pack.yml"] = ciPreviewLegacy(revision, source, variant)
			}
			for path, body := range expected {
				ciPreviewWrite(root, path, body, 0644)
			}
			// A wrapper is never invoked to infer historical Maven command selection.
			ciPreviewWrite(root, "mvnw", []byte("#!/bin/sh\nexit 99\n"), 0755)
			report := ciPreviewMap(PreviewCIWorkflow(context.Background(), root, request, &ciPreviewRelease, nil))
			rows := ciPreviewRows(report)
			Expect(rows).To(HaveLen(5))
			Expect(report["conflicts"]).To(BeEmpty(), variant.name)
			for _, name := range []string{"ci.yml", "go-pack.yml", "typescript-pack.yml", "java-pack.yml"} {
				path := ".github/workflows/" + name
				if body, present := expected[path]; present {
					Expect(rows[path]["disposition"]).To(Equal("retire"), path)
					Expect(rows[path]["before_sha256"]).To(Equal(fmt.Sprintf("%x", sha256.Sum256(body))))
					Expect(os.ReadFile(filepath.Join(root, path))).To(Equal(body))
				} else {
					Expect(rows[path]["disposition"]).To(Equal("absent"), path)
				}
			}
			Expect(rows[".github/workflows/factory.yml"]["disposition"]).To(Equal("create"))
			Expect(rows[".github/workflows/factory.yml"]["after_sha256"]).To(MatchRegexp("^[a-f0-9]{64}$"))
			_, err := os.Lstat(filepath.Join(root, ".github/workflows/factory.yml"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	}, Entry("v0.1.6", "v0.1.6", "b71ecc32e07ecd87eb330ba8e497c86612f92acd"), Entry("Bash baseline", "bash-baseline", "76952eaa63aebd1ecd282f5ab51dd7c3627cb497"))

	// per docs/adr/0147-installed-ci-migration-preview.md:53
	DescribeTable("preserves every unsafe or customized known occupant and reports all conflicts", func(kind string) {
		root, request := ciPreviewFixture("v0.1.6", ciPreviewVariants[0])
		path := filepath.Join(root, ".github/workflows/ci.yml")
		ciPreviewWrite(root, ".github/workflows/ci.yml", ciPreviewLegacy("b71ecc32e07ecd87eb330ba8e497c86612f92acd", ".github/workflows/ci.yml", ciPreviewVariants[0]), 0644)
		switch kind {
		case "custom":
			ciPreviewWrite(root, ".github/workflows/ci.yml", []byte("adopter-owned\n"), 0644)
		case "mode":
			Expect(os.Chmod(path, 0755)).To(Succeed())
		case "directory":
			Expect(os.Remove(path)).To(Succeed())
			Expect(os.Mkdir(path, 0755)).To(Succeed())
		case "link":
			Expect(os.Remove(path)).To(Succeed())
			Expect(os.Symlink("/must/not/be/read", path)).To(Succeed())
		case "fifo":
			Expect(os.Remove(path)).To(Succeed())
			Expect(unix.Mkfifo(path, 0600)).To(Succeed())
		case "oversized":
			ciPreviewWrite(root, ".github/workflows/ci.yml", make([]byte, (1<<20)+1), 0644)
		}
		ciPreviewWrite(root, ".github/workflows/factory.yml", []byte("second customization\n"), 0644)
		Expect(os.Symlink("/outside/application.yml", filepath.Join(root, ".github/workflows/application.yml"))).To(Succeed())
		report := ciPreviewMap(PreviewCIWorkflow(context.Background(), root, request, &ciPreviewRelease, nil))
		rows := ciPreviewRows(report)
		Expect(rows).To(HaveLen(5))
		Expect(rows[".github/workflows/ci.yml"]["disposition"]).To(Equal("preserve"))
		Expect(rows[".github/workflows/factory.yml"]["disposition"]).To(Equal("preserve"))
		Expect(report["conflicts"]).To(HaveLen(2))
		info, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		if kind == "mode" {
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0755)))
		}
		if kind == "link" {
			Expect(os.Readlink(path)).To(Equal("/must/not/be/read"))
		}
	}, Entry("custom bytes", "custom"), Entry("changed mode", "mode"), Entry("directory", "directory"), Entry("dangling symlink", "link"), Entry("FIFO", "fifo"), Entry("oversized file", "oversized"))

	// per docs/adr/0147-installed-ci-migration-preview.md:61
	// per docs/adr/0147-installed-ci-migration-preview.md:86
	DescribeTable("preserves unselected or changed-settings pack bytes", func(change string) {
		root, request := ciPreviewFixture("v0.1.6", ciPreviewVariants[1])
		body := ciPreviewLegacy("b71ecc32e07ecd87eb330ba8e497c86612f92acd", "packs/go/workflows/ci.yml", ciPreviewVariants[1])
		ciPreviewWrite(root, ".github/workflows/go-pack.yml", body, 0644)
		cfg := ciPreviewConfig(ciPreviewVariants[1])
		if change == "unselected" {
			cfg = ciPreviewConfig(ciPreviewVariants[0])
		} else {
			cfg = strings.Replace(cfg, "1.26", "1.27.3", 1)
		}
		ciPreviewWrite(root, "factory.yaml", []byte(cfg), 0644)
		report := ciPreviewMap(PreviewCIWorkflow(context.Background(), root, request, &ciPreviewRelease, nil))
		Expect(ciPreviewRows(report)[".github/workflows/go-pack.yml"]["disposition"]).To(Equal("preserve"))
		Expect(report["conflicts"]).NotTo(BeEmpty())
		Expect(os.ReadFile(filepath.Join(root, ".github/workflows/go-pack.yml"))).To(Equal(body))
	}, Entry("unselected pack", "unselected"), Entry("changed language version", "changed"))

	// per docs/adr/0147-installed-ci-migration-preview.md:81
	DescribeTable("refuses to propose generated bytes from ambiguous or unsupported inputs", func(change string) {
		root, request := ciPreviewFixture("v0.1.6", ciPreviewVariants[1])
		cfg := ciPreviewConfig(ciPreviewVariants[1])
		switch change {
		case "missing-packs":
			cfg = strings.Replace(cfg, `language_packs: "go"`+"\n", "", 1)
		case "missing-protected":
			cfg = strings.Replace(cfg, "protected_paths: internal\n", "", 1)
			delete(request.Inputs, "PROTECTED_PATH")
		case "suffix":
			cfg = strings.Replace(cfg, `go_version: "1.26"`, `go_version: "1.26" trailing`, 1)
		case "duplicate":
			cfg += "go_version: \"1.27.3\"\n"
		case "packs":
			cfg = strings.Replace(cfg, "language_packs: \"go\"", "language_packs: ruby", 1)
		case "missing":
			cfg = strings.Replace(cfg, "go_version: \"1.26\"\n", "", 1)
		case "java":
			cfg = ciPreviewConfig(ciPreviewVariant{packs: "java", build: "ant"})
		case "multipath":
			cfg = strings.Replace(cfg, "protected_paths: internal", "protected_paths: internal other", 1)
		case "nul":
			cfg += "note: \x00\n"
		case "unicode":
			cfg += "note: \xff\n"
		case "sed":
			cfg = strings.Replace(cfg, "protected_paths: internal", "protected_paths: internal&other", 1)
		}
		ciPreviewWrite(root, "factory.yaml", []byte(cfg), 0644)
		report, err := PreviewCIWorkflow(context.Background(), root, request, &ciPreviewRelease, nil)
		object := ciPreviewMap(report, err)
		Expect(object["conflicts"]).NotTo(BeEmpty())
		for _, row := range ciPreviewRows(object) {
			Expect(row["disposition"]).NotTo(BeElementOf("create", "replace", "retire"))
		}
	}, Entry("missing pack selection", "missing-packs"), Entry("missing protected-path selection", "missing-protected"), Entry("quoted scalar with trailing junk", "suffix"), Entry("duplicate selected key", "duplicate"), Entry("unsupported pack", "packs"), Entry("missing selected version", "missing"), Entry("invalid Java build", "java"), Entry("multiple mutation paths", "multipath"), Entry("NUL", "nul"), Entry("invalid Unicode", "unicode"), Entry("unsupported sed replacement", "sed"))

	// per docs/adr/0147-installed-ci-migration-preview.md:27
	// per docs/adr/0147-installed-ci-migration-preview.md:44
	DescribeTable("does not manufacture release authority from inspection candidate local or noncanonical metadata", func(kind string) {
		root, request := ciPreviewFixture("v0.1.6", ciPreviewVariants[0])
		release := ciPreviewRelease
		supplied := &release
		switch kind {
		case "nil":
			supplied = nil
			request.Candidate = true
		case "local":
			request.Source.Local = true
			request.Candidate = true
		case "noncanonical":
			release.Tag = "v0.3"
			request.Source.Version = release.Tag
		case "repository":
			release.Repository = "adopter/fork"
		case "mismatch":
			release.SourceRevision = strings.Repeat("b", 40)
		}
		report := ciPreviewMap(PreviewCIWorkflow(context.Background(), root, request, supplied, nil))
		for _, row := range ciPreviewRows(report) {
			Expect(row["disposition"]).NotTo(BeElementOf("create", "replace", "retire"))
		}
		Expect(strings.ToLower(report["reason"].(string))).To(MatchRegexp("authenticat|canonical|unavailable|official|mismatch|local"))
		_, err := os.Lstat(filepath.Join(root, ".github"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("candidate metadata only", "nil"), Entry("local source even with release-shaped data", "local"), Entry("noncanonical tag", "noncanonical"), Entry("unofficial repository", "repository"), Entry("mismatched revision", "mismatch"))

	// per docs/adr/0147-installed-ci-migration-preview.md:49
	// per docs/adr/0147-installed-ci-migration-preview.md:53
	It("preserves unsafe workflow ancestors without traversing the linked directory", func() {
		root, request := ciPreviewFixture("v0.1.6", ciPreviewVariants[0])
		foreign, err := os.MkdirTemp("", "factory-ci-foreign-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, foreign)
		ciPreviewWrite(foreign, "workflows/ci.yml", []byte("foreign content\n"), 0644)
		Expect(os.Symlink(foreign, filepath.Join(root, ".github"))).To(Succeed())
		report := ciPreviewMap(PreviewCIWorkflow(context.Background(), root, request, &ciPreviewRelease, nil))
		rows := ciPreviewRows(report)
		Expect(rows).To(HaveLen(5))
		for _, row := range rows {
			Expect(row["disposition"]).To(Equal("preserve"))
		}
		Expect(report["conflicts"]).To(HaveLen(5))
		Expect(os.ReadFile(filepath.Join(foreign, "workflows/ci.yml"))).To(Equal([]byte("foreign content\n")))
		_, err = os.Lstat(filepath.Join(foreign, "workflows/factory.yml"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0147-installed-ci-migration-preview.md:43
	It("propagates a missing project root as an operational failure", func() {
		root, request := ciPreviewFixture("v0.1.6", ciPreviewVariants[0])
		_, err := PreviewCIWorkflow(context.Background(), filepath.Join(root, "missing"), request, &ciPreviewRelease, nil)
		Expect(err).To(HaveOccurred())
		Expect(ErrorStatus(err)).To(Equal(1))
	})

	// per docs/adr/0147-installed-ci-migration-preview.md:43
	It("returns cancellation as an operational error without successful unknown output", func() {
		root, request := ciPreviewFixture("v0.1.6", ciPreviewVariants[0])
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := PreviewCIWorkflow(ctx, root, request, &ciPreviewRelease, nil)
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		Expect(ErrorStatus(err)).To(Equal(1))
	})

	// per docs/adr/0147-installed-ci-migration-preview.md:63
	// per docs/adr/0147-installed-ci-migration-preview.md:75
	It("refreshes exact frozen generated bytes using old selectors while proposing the exact target selectors", func() {
		old := ciworkflow.Release{Tag: "v0.3.0", OriginTarget: "linux/amd64", SourceRevision: ciPreviewRevision, Repository: ciPreviewRelease.Repository}
		before := ciPreviewOldImage(old)
		for _, variant := range ciPreviewVariants {
			By(variant.name)
			root, request := ciPreviewFixture("go-hybrid-v1", variant)
			body := ciPreviewFrozenWorkflow(old, variant)
			// per docs/adr/0147-installed-ci-migration-preview.md:209
			if variant.name == "all-gradle" {
				Expect(body).To(Equal([]byte(ciPreviewAllGradleHistoricalGolden)), "the composed oracle must equal bytes captured from the actual immutable historical renderer")
				body = []byte(ciPreviewAllGradleHistoricalGolden)
			}
			ciPreviewWrite(root, ".github/workflows/factory.yml", body, 0644)
			request.Before = staging.Options{Version: old.Tag, Target: old.OriginTarget, Revision: old.SourceRevision}
			report := ciPreviewMap(PreviewCIWorkflow(context.Background(), root, request, &ciPreviewRelease, &before))
			row := ciPreviewRows(report)[".github/workflows/factory.yml"]
			Expect(row["disposition"]).To(Equal("replace"))
			Expect(row["before_sha256"]).To(Equal(fmt.Sprintf("%x", sha256.Sum256(body))))
			target := ciPreviewFrozenWorkflow(ciPreviewRelease, variant)
			Expect(row["after_sha256"]).To(Equal(fmt.Sprintf("%x", sha256.Sum256(target))))
			Expect(report["conflicts"]).To(BeEmpty())
			Expect(os.ReadFile(filepath.Join(root, ".github/workflows/factory.yml"))).To(Equal(body))
			// A customization after an authentic header still cannot become ownership.
			changed := append(append([]byte{}, body...), []byte("# adopter change\n")...)
			ciPreviewWrite(root, ".github/workflows/factory.yml", changed, 0644)
			report = ciPreviewMap(PreviewCIWorkflow(context.Background(), root, request, &ciPreviewRelease, &before))
			Expect(ciPreviewRows(report)[".github/workflows/factory.yml"]["disposition"]).To(Equal("preserve"))
			Expect(report["conflicts"]).NotTo(BeEmpty())
		}
	})

	// per docs/adr/0147-installed-ci-migration-preview.md:63
	DescribeTable("preserves generated-looking bytes from an unqualified Go initializer recipe", func(extra bool) {
		root, request := ciPreviewFixture("go-hybrid-v1", ciPreviewVariants[0])
		old := ciworkflow.Release{Tag: "v0.3.0", OriginTarget: "linux/amd64", SourceRevision: ciPreviewRevision, Repository: ciPreviewRelease.Repository}
		body := ciPreviewFrozenWorkflow(old)
		ciPreviewWrite(root, ".github/workflows/factory.yml", body, 0644)
		before := ciPreviewOldImage(old)
		changedPath := "internal/initcmd/workflow.go"
		if extra {
			changedPath = "internal/initcmd/unqualified.go"
			before.Assets[changedPath] = []byte("package initcmd\n")
		} else {
			before.Assets[changedPath] = append(append([]byte{}, before.Assets[changedPath]...), []byte("// unqualified recipe\n")...)
		}
		changed := installationimage.Asset{Path: changedPath, Mode: "100644", SHA256: fmt.Sprintf("%x", sha256.Sum256(before.Assets[changedPath])), Bytes: int64(len(before.Assets[changedPath]))}
		if extra {
			before.Manifest.Assets = append(before.Manifest.Assets, changed)
		} else {
			for index, asset := range before.Manifest.Assets {
				if asset.Path == changedPath {
					before.Manifest.Assets[index] = changed
				}
			}
		}
		request.Before = staging.Options{Version: old.Tag, Target: old.OriginTarget, Revision: old.SourceRevision}
		report := ciPreviewMap(PreviewCIWorkflow(context.Background(), root, request, &ciPreviewRelease, &before))
		Expect(ciPreviewRows(report)[".github/workflows/factory.yml"]["disposition"]).To(Equal("preserve"))
		Expect(report["conflicts"]).NotTo(BeEmpty())
		Expect(os.ReadFile(filepath.Join(root, ".github/workflows/factory.yml"))).To(Equal(body))
	}, Entry("changed qualified source", false), Entry("unexpected production source", true))
	// per docs/adr/0147-installed-ci-migration-preview.md:63
	It("recognizes copied workflows from the frozen pre-generated Go initializer recipe", func() {
		const revision = "6b8f08c820c347a9c9d12c4afd00b3f7e6aca547"
		old := ciworkflow.Release{Tag: "v0.2.9", OriginTarget: "linux/amd64", SourceRevision: revision, Repository: ciPreviewRelease.Repository}
		before := ciPreviewOldImage(old)
		variant := ciPreviewVariants[1]
		root, request := ciPreviewFixture("go-hybrid-v1", variant)
		request.Before = staging.Options{Version: old.Tag, Target: old.OriginTarget, Revision: old.SourceRevision}
		ciPreviewWrite(root, ".github/workflows/ci.yml", ciPreviewLegacy(revision, ".github/workflows/ci.yml", variant), 0644)
		ciPreviewWrite(root, ".github/workflows/go-pack.yml", ciPreviewLegacy(revision, "packs/go/workflows/ci.yml", variant), 0644)
		report := ciPreviewMap(PreviewCIWorkflow(context.Background(), root, request, &ciPreviewRelease, &before))
		rows := ciPreviewRows(report)
		Expect(rows[".github/workflows/ci.yml"]["disposition"]).To(Equal("retire"))
		Expect(rows[".github/workflows/go-pack.yml"]["disposition"]).To(Equal("retire"))
		Expect(rows[".github/workflows/factory.yml"]["disposition"]).To(Equal("create"))
		Expect(report["conflicts"]).To(BeEmpty())
	})

	// per docs/adr/0147-installed-ci-migration-preview.md:43
	It("returns an actual configuration read failure as an operational error", func() {
		if os.Geteuid() == 0 {
			Skip("root can read mode-zero files; this native EACCES criterion requires an ordinary user")
		}
		root, request := ciPreviewFixture("v0.1.6", ciPreviewVariants[0])
		path := filepath.Join(root, "factory.yaml")
		Expect(os.Chmod(path, 0000)).To(Succeed())
		DeferCleanup(os.Chmod, path, os.FileMode(0644))
		_, nativeError := os.ReadFile(path)
		Expect(errors.Is(nativeError, os.ErrPermission)).To(BeTrue())
		_, err := PreviewCIWorkflow(context.Background(), root, request, &ciPreviewRelease, nil)
		Expect(err).To(HaveOccurred())
		Expect(ErrorStatus(err)).To(Equal(1))
	})

	// per docs/adr/0147-installed-ci-migration-preview.md:81
	DescribeTable("preserves unsafe configuration snapshots without following or blocking on them", func(kind string) {
		root, request := ciPreviewFixture("v0.1.6", ciPreviewVariants[0])
		path := filepath.Join(root, "factory.yaml")
		Expect(os.Remove(path)).To(Succeed())
		switch kind {
		case "link":
			Expect(os.Symlink("/outside/factory.yaml", path)).To(Succeed())
		case "directory":
			Expect(os.Mkdir(path, 0755)).To(Succeed())
		case "fifo":
			Expect(unix.Mkfifo(path, 0600)).To(Succeed())
		case "oversized":
			ciPreviewWrite(root, "factory.yaml", make([]byte, (1<<20)+1), 0644)
		}
		report := ciPreviewMap(PreviewCIWorkflow(context.Background(), root, request, &ciPreviewRelease, nil))
		Expect(report["conflicts"]).NotTo(BeEmpty())
		for _, row := range ciPreviewRows(report) {
			Expect(row["disposition"]).NotTo(BeElementOf("create", "replace", "retire"))
		}
		_, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		_, err = os.Lstat(filepath.Join(root, ".github"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("symlink", "link"), Entry("directory", "directory"), Entry("FIFO", "fifo"), Entry("oversized", "oversized"))

	// per docs/adr/0147-installed-ci-migration-preview.md:56
	// per docs/adr/0147-installed-ci-migration-preview.md:90
	DescribeTable("retains the Bash initializer's literal dot-path mutation projection", func(profile, revision string) {
		variant := ciPreviewVariants[1]
		root, request := ciPreviewFixture(profile, variant)
		request.Inputs["PROTECTED_PATH"] = "."
		cfg := strings.Replace(ciPreviewConfig(variant), "protected_paths: internal", "protected_paths: .", 1)
		ciPreviewWrite(root, "factory.yaml", []byte(cfg), 0644)
		source := string(immutableRecipeSource(revision, "packs/go/workflows/ci.yml"))
		Expect(source).To(ContainSubstring("__MUTATION_TARGET__"))
		original := []byte(strings.NewReplacer("__GO_VERSION__", "1.26", "__MUTATION_TARGET__", "././...", "__PROTECTED_PATH__", ".").Replace(source))
		path := ".github/workflows/go-pack.yml"
		ciPreviewWrite(root, path, original, 0644)
		report := ciPreviewMap(PreviewCIWorkflow(context.Background(), root, request, &ciPreviewRelease, nil))
		Expect(ciPreviewRows(report)[path]["disposition"]).To(Equal("retire"))
		Expect(report["conflicts"]).To(BeEmpty())
		normalized := []byte(strings.ReplaceAll(string(original), "././...", "./..."))
		Expect(normalized).NotTo(Equal(original))
		ciPreviewWrite(root, path, normalized, 0644)
		report = ciPreviewMap(PreviewCIWorkflow(context.Background(), root, request, &ciPreviewRelease, nil))
		Expect(ciPreviewRows(report)[path]["disposition"]).To(Equal("preserve"))
		Expect(report["conflicts"]).NotTo(BeEmpty())
		Expect(os.ReadFile(filepath.Join(root, path))).To(Equal(normalized))
	}, Entry("v0.1.6", "v0.1.6", "b71ecc32e07ecd87eb330ba8e497c86612f92acd"), Entry("Bash baseline", "bash-baseline", "76952eaa63aebd1ecd282f5ab51dd7c3627cb497"))

	// per docs/adr/0147-installed-ci-migration-preview.md:152
	It("requires historical Go path overrides to be producible by the frozen initializer", func() {
		old := ciworkflow.Release{Tag: "v0.3.0", OriginTarget: "linux/amd64", SourceRevision: ciPreviewRevision, Repository: ciPreviewRelease.Repository}
		before := ciPreviewOldImage(old)
		variant := ciPreviewVariants[1]
		for _, historicalPath := range []string{"old/path", "../outside", "/outside", `bad"path`} {
			By(historicalPath)
			root, request := ciPreviewFixture("go-hybrid-v1", variant)
			request.Before = staging.Options{Version: old.Tag, Target: old.OriginTarget, Revision: old.SourceRevision}
			request.Inputs["PROTECTED_PATH"] = historicalPath
			target := ciPreviewFrozenWorkflow(ciPreviewRelease, variant)
			original := ciPreviewFrozenWorkflow(old, variant)
			body := []byte(strings.Replace(string(original), strconv.Quote("./internal"), strconv.Quote("./"+historicalPath), 1))
			Expect(body).NotTo(Equal(original))
			ciPreviewWrite(root, ".github/workflows/factory.yml", body, 0644)
			report := ciPreviewMap(PreviewCIWorkflow(context.Background(), root, request, &ciPreviewRelease, &before))
			row := ciPreviewRows(report)[".github/workflows/factory.yml"]
			if historicalPath == "old/path" {
				Expect(row["disposition"]).To(Equal("replace"))
				Expect(row["after_sha256"]).To(Equal(fmt.Sprintf("%x", sha256.Sum256(target))))
				Expect(report["conflicts"]).To(BeEmpty())
			} else {
				Expect(row["disposition"]).To(Equal("preserve"))
				Expect(report["conflicts"]).NotTo(BeEmpty())
			}
			Expect(os.ReadFile(filepath.Join(root, ".github/workflows/factory.yml"))).To(Equal(body))
		}
	})

	// per docs/adr/0147-installed-ci-migration-preview.md:152
	It("requires historical Go generic scalars to round-trip through the frozen flat configuration reader", func() {
		const revision = "6b8f08c820c347a9c9d12c4afd00b3f7e6aca547"
		old := ciworkflow.Release{Tag: "v0.2.9", OriginTarget: "linux/amd64", SourceRevision: revision, Repository: ciPreviewRelease.Repository}
		before := ciPreviewOldImage(old)
		source := string(immutableRecipeSource(revision, "packs/go/workflows/ci.yml"))
		for _, key := range []string{"DOCS_ROOT", "PROJECT_SLUG"} {
			for _, historicalValue := range []string{"old-value", "docs #tail"} {
				By(key + "=" + historicalValue)
				root, request := ciPreviewFixture("go-hybrid-v1", ciPreviewVariants[1])
				marker := "__" + key + "__"
				cfg := strings.Replace(ciPreviewConfig(ciPreviewVariants[1]), `go_version: "1.26"`, `go_version: "`+marker+`"`, 1)
				cfg += "docs_root: docs\n"
				ciPreviewWrite(root, "factory.yaml", []byte(cfg), 0644)
				request.Before = staging.Options{Version: old.Tag, Target: old.OriginTarget, Revision: old.SourceRevision}
				request.Inputs[key] = historicalValue
				// The frozen copied Go initializer substitutes pack versions first,
				// then performs its simultaneous generic replacement. The marker
				// exposes a scalar that the old collector could never persist.
				pack := strings.ReplaceAll(source, "__GO_VERSION__", marker)
				body := []byte(strings.NewReplacer(marker, historicalValue, "__MUTATION_TARGET__", "./internal/...", "__PROTECTED_PATH__", "internal").Replace(pack))
				Expect(string(body)).To(ContainSubstring(historicalValue))
				path := ".github/workflows/go-pack.yml"
				ciPreviewWrite(root, path, body, 0644)
				report := ciPreviewMap(PreviewCIWorkflow(context.Background(), root, request, &ciPreviewRelease, &before))
				row := ciPreviewRows(report)[path]
				if historicalValue == "old-value" {
					Expect(row["disposition"]).To(Equal("retire"))
					Expect(report["conflicts"]).To(BeEmpty())
				} else {
					Expect(row["disposition"]).To(Equal("preserve"))
					Expect(report["conflicts"]).NotTo(BeEmpty())
				}
				Expect(os.ReadFile(filepath.Join(root, path))).To(Equal(body))
			}
		}
	})

})
