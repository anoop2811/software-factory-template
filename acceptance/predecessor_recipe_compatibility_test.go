package acceptance_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const recipeRC3Revision = "95486ecdd3868a319a0e6362ee99cafd8f154200"
const recipeRC3Version = "v0.2.0-rc.3"
const recipeAnswers = "Recipe Project\nrecipe-project\n@example\nrecipe-user\ninternal\ndocs\n\ninherit\nstandard\nn\ny\n"

var recipeHistoricalMembers = []string{
	"internal/installation/generate.go", "internal/installation/roles.go",
	"internal/installation/orchestration.go", "internal/installation/reference.go",
	"internal/installation/model.go", "internal/installation/observe.go",
	"internal/installation/selection.go", "internal/installedlayout/layout.go",
}
var recipeCurrentMembers = []string{"internal/installation/recipe.go", "internal/installation/roles_v1.go"}

type recipeFixture struct {
	before, after handoffReleaseImage
	prepared      string
	rows          []installationRoleOracle
}

// The only historical executable is rebuilt from an immutable trusted Git
// object and used to establish a genuine installation, never selected from the
// predecessor supplied to the consumer. per docs/adr/0115-frozen-rc3-recipe-compatibility.md:57
func recipePrepare() recipeFixture {
	GinkgoHelper()
	after := handoffBuildImage()
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	source := filepath.Join(after.work, "immutable-rc3")
	imageGit(repository, "clone", "--quiet", "--shared", "--no-checkout", repository, source)
	imageGit(source, "checkout", "--quiet", "--detach", recipeRC3Revision)
	Expect(strings.TrimSpace(imageGit(source, "rev-parse", "HEAD"))).To(Equal(recipeRC3Revision))
	binaryPath := filepath.Join(after.work, "locally-rebuilt-rc3-runtime")
	build := imageProcess(source, nil, "go", "build", "-o", binaryPath, "./cmd/factory")
	Expect(build.status).To(Equal(0), "%+v", build)
	binary, err := os.ReadFile(binaryPath)
	Expect(err).NotTo(HaveOccurred())
	assets, payload := installationGitCatalog(repository, recipeRC3Revision)
	var baselines []imageBaselineOracle
	for _, profile := range []string{"v0.1.6", "bash-baseline"} {
		ref := after.references[profile]
		baselines = append(baselines, imageBaselineOracle{profile, ref.revision, ref.assets})
	}
	manifest := imageManifestOracle{1, "installation_source_image", "committed_source_and_baseline_references", recipeRC3Version, runtime.GOOS + "/" + runtime.GOARCH, recipeRC3Revision, false, assets, baselines}
	archive := stageArchive(imageFixtureEntries(manifest, payload, binary))
	archivePath := filepath.Join(after.work, "locally-rebuilt-rc3-image.tar.gz")
	writeFixture(archivePath, archive, 0600)
	before := handoffReleaseImage{binary, archive, archivePath, recipeRC3Revision, after.work, after.references}
	var roles installationRoleMapOracle
	Expect(json.Unmarshal(payload["docs/migration/INSTALLATION_ROLE_MAP.json"], &roles)).To(Succeed())
	Expect(roles.CanonicalSourceAssets).To(HaveLen(85))
	rows := append(append([]installationRoleOracle{}, roles.Roles...), roles.CanonicalSourceAssets...)
	Expect(rows).To(HaveLen(141), "independent immutable RC3 role inventory")

	f, driver := handoffNativeFixture(before)
	writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+recipeRC3Version+"-"+f.target), binary, 0600)
	writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-installation-"+recipeRC3Version+"-"+f.target+".tar.gz"), archive, 0600)
	prepared := filepath.Join(f.cwd, "genuine RC3 project")
	Expect(os.Mkdir(prepared, 0700)).To(Succeed())
	imageGit(prepared, "init", "--quiet")
	initialized := handoffNativeProcess(f.cwd, f.environment, recipeAnswers, driver, "init", prepared, "--go-candidate", recipeRC3Version, "--pack", "none")
	Expect(initialized.status).To(Equal(0), "%+v", initialized)
	for _, row := range roles.CanonicalSourceAssets {
		Expect(os.ReadFile(filepath.Join(prepared, filepath.FromSlash(row.Path)))).NotTo(BeEmpty(), row.Path)
	}

	// A benign declaration delta is compatibility fixture data, not a claim
	// that a new generator has shipped. It changes no compiled output.
	// per docs/adr/0115-frozen-rc3-recipe-compatibility.md:64
	after.archive = handoffRewriteAsset(after.archive, "internal/installation/reference.go", func(body []byte) []byte {
		return append(body, []byte("\n// evaluator declares a future generation fingerprint\n")...)
	})
	for _, path := range recipeCurrentMembers {
		after.archive = recipeSupplyMember(after.archive, path)
	}
	writeFixture(after.archivePath, after.archive, 0600)
	return recipeFixture{before, after, prepared, rows}
}

// Complete manifest rewrites keep negative archives structurally valid; the
// consumer must refuse their recipe, not an unrelated checksum corruption.
func recipeAsset(archive []byte, path string, body []byte, remove bool) []byte {
	GinkgoHelper()
	entries, _ := imageDecodeArchive(archive)
	var manifest imageManifestOracle
	Expect(json.Unmarshal(entries[3].body, &manifest)).To(Succeed())
	payload := map[string][]byte{}
	assets := []imageAssetOracle{}
	found := false
	for index, asset := range manifest.Assets {
		if asset.Path == path {
			found = true
			if remove {
				continue
			}
			asset.Bytes = int64(len(body))
			asset.SHA256 = fmt.Sprintf("%x", sha256.Sum256(body))
			payload[path] = body
		} else {
			payload[asset.Path] = entries[4+index].body
		}
		assets = append(assets, asset)
	}
	if !found && !remove {
		assets = append(assets, imageAssetOracle{Path: path, Mode: "100644", SHA256: fmt.Sprintf("%x", sha256.Sum256(body)), Bytes: int64(len(body))})
		payload[path] = body
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Path < assets[j].Path })
	manifest.Assets = assets
	return stageArchive(imageFixtureEntries(manifest, payload, entries[0].body))
}

func recipeSupplyMember(archive []byte, path string) []byte {
	entries, _ := imageDecodeArchive(archive)
	var manifest imageManifestOracle
	Expect(json.Unmarshal(entries[3].body, &manifest)).To(Succeed())
	for _, asset := range manifest.Assets {
		if asset.Path == path {
			return archive
		}
	}
	return recipeAsset(archive, path, []byte("package installation\n// inert evaluator declaration\n"), false)
}

func recipeOperands(before, after handoffReleaseImage, beforeVersion string) []string {
	args := []string{"upgrade", "--installation", "--profile", "go-hybrid-v1"}
	for _, part := range []struct {
		prefix  string
		image   handoffReleaseImage
		version string
	}{{"", after, handoffVersion}, {"before-", before, beforeVersion}} {
		args = append(args, "--"+part.prefix+"source", part.image.archivePath, "--"+part.prefix+"version", part.version, "--"+part.prefix+"revision", part.image.revision, "--"+part.prefix+"target", runtime.GOOS+"/"+runtime.GOARCH, "--"+part.prefix+"local")
	}
	for _, value := range []string{"PROJECT_NAME=Recipe Project", "PROJECT_SLUG=recipe-project", "GITHUB_OWNER=@example", "OPENCODE_USERNAME=recipe-user", "PROTECTED_PATH=internal", "DOCS_ROOT=docs", "CITATION_PREFIX="} {
		args = append(args, "--project-input", value)
	}
	return args
}

func (fixture recipeFixture) predecessor() (handoffBootstrapFixture, string, string, []string) {
	GinkgoHelper()
	f, driver := handoffNativeFixture(fixture.after)
	cloned := installationCloneLegacyFixture(fixture.after.binary, installationFixtureData{root: fixture.prepared})
	return f, driver, cloned.root, recipeOperands(fixture.before, fixture.after, recipeRC3Version)
}

func recipeBeforePath(args []string, path string) []string {
	result := append([]string{}, args...)
	for index, arg := range result {
		if arg == "--before-source" {
			result[index+1] = path
		}
	}
	return result
}

var _ = Describe("Supported predecessor recipe compatibility", Ordered, ContinueOnFailure, func() {
	var fixture recipeFixture
	BeforeAll(func() { fixture = recipePrepare() })
	// per docs/adr/0115-frozen-rc3-recipe-compatibility.md:64
	It("reconstructs predecessor bytes and modes through preview apply and rollback while retaining all cache and user history", func() {
		f, driver, root, args := fixture.predecessor()
		preserved := map[string][]byte{"docs/recipe-user-note.md": []byte("user customization\n"), ".factory/loops.json": []byte("{\"schema\":1,\"runs\":[],\"note\":\"preserve history\"}\n"), ".factory/events.log": []byte("2026-10-30T00:00:00Z\toperator\tpreserve history\n")}
		for path, data := range preserved {
			writeFixture(filepath.Join(root, filepath.FromSlash(path)), data, 0600)
		}
		before := installationSnapshot(root)
		proposal := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...))
		Expect(installationSnapshot(root)).To(Equal(before))
		actions := map[string]map[string]any{}
		for _, raw := range proposal["actions"].([]any) {
			row := raw.(map[string]any)
			actions[row["path"].(string)] = row
		}
		for _, row := range fixture.rows {
			if row.Operation == "retire" {
				continue
			}
			Expect(actions).To(HaveKey(row.Path))
			original, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(row.Path)))
			Expect(err).NotTo(HaveOccurred())
			info, err := os.Stat(filepath.Join(root, filepath.FromSlash(row.Path)))
			Expect(err).NotTo(HaveOccurred())
			observed, ok := actions[row.Path]["before"].(map[string]any)
			Expect(ok).To(BeTrue(), row.Path)
			Expect(observed["sha256"]).To(Equal(fmt.Sprintf("%x", sha256.Sum256(original))), row.Path)
			Expect(observed["bytes"]).To(Equal(float64(len(original))), row.Path)
			Expect(observed["mode"]).To(Equal(float64(0100000|info.Mode().Perm())), row.Path)
		}
		applied := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--migration-id", "rc3-recipe-replay", "--confirm", proposal["proposal_digest"].(string), "--quiescent")...)
		Expect(applied.status).To(Equal(0), "%+v", applied)
		for _, row := range fixture.rows {
			if row.Operation == "inert-data" {
				Expect(os.ReadFile(filepath.Join(root, filepath.FromSlash(row.Path)))).NotTo(BeEmpty(), row.Path)
			}
		}
		for path, data := range preserved {
			Expect(os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))).To(Equal(data))
		}
		reverseArgs := append(append([]string{}, args...), "--rollback", "rc3-recipe-replay")
		reverse := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, reverseArgs...), "--dry-run", "--json")...))
		restored := handoffNativeProcess(root, f.environment, "", driver, append(reverseArgs, "--confirm", reverse["proposal_digest"].(string), "--quiescent")...)
		Expect(restored.status).To(Equal(0), "%+v", restored)
		restoredSnapshot := installationSnapshot(root)
		for _, row := range fixture.rows {
			if row.Operation != "retire" {
				Expect(restoredSnapshot[row.Path]).To(Equal(before[row.Path]), row.Path)
			}
		}
		for path, data := range preserved {
			Expect(os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))).To(Equal(data))
		}
		for path, value := range before {
			if strings.HasPrefix(path, ".git/") {
				Expect(restoredSnapshot[path]).To(Equal(value), path)
			}
		}
	})
	// per docs/adr/0115-frozen-rc3-recipe-compatibility.md:71
	It("refuses each altered or missing historical fingerprint member without executing the supplied runtime or mutating the adopter", func() {
		f, driver, root, args := fixture.predecessor()
		before := installationSnapshot(root)
		marker := filepath.Join(f.cwd, "supplied-runtime-executed")
		poison := []byte("#!/bin/sh\nprintf executed > '" + marker + "'\nexit 96\n")
		poisoned := fixture.before
		poisoned.archive = handoffImageBinary(fixture.before, poison, recipeRC3Version)
		receipts := []string{}
		for _, path := range recipeHistoricalMembers {
			for _, missing := range []bool{false, true} {
				changed := recipeAsset(poisoned.archive, path, []byte("package installation\n// unknown historical recipe\n"), missing)
				archivePath := filepath.Join(fixture.after.work, "unknown-prior-recipe.tar.gz")
				writeFixture(archivePath, changed, 0600)
				out := handoffNativeProcess(root, f.environment, "", driver, append(recipeBeforePath(args, archivePath), "--dry-run", "--json")...)
				Expect(out.status).To(Equal(2), "%s missing=%t: %+v", path, missing, out)
				Expect(out.stderr).To(ContainSubstring("recipe"), "%s missing=%t", path, missing)
				Expect(installationSnapshot(root)).To(Equal(before), path)
				_, err := os.Stat(marker)
				Expect(os.IsNotExist(err)).To(BeTrue())
				receipts = append(receipts, fmt.Sprintf("%s missing=%t status=%d", path, missing, out.status))
			}
		}
		Expect(receipts).To(HaveLen(16))
		AddReportEntry("historical-recipe.refused-members", receipts, ReportEntryVisibilityNever)
	})
	// per docs/adr/0115-frozen-rc3-recipe-compatibility.md:47
	It("preserves an edited required cache asset as a visible conflict without inferring ownership from its filename", func() {
		f, driver, root, args := fixture.predecessor()
		path := ".factory/assets/scaffold/scripts/lib/loop.py"
		custom := []byte("operator maintained reference; never execute\n")
		writeFixture(filepath.Join(root, filepath.FromSlash(path)), custom, 0600)
		before := installationSnapshot(root)
		out := handoffNativeProcess(root, f.environment, "", driver, append(args, "--dry-run", "--json")...)
		Expect(out.status).To(Equal(2), "%+v", out)
		var proposal map[string]any
		Expect(json.Unmarshal([]byte(out.stdout), &proposal)).To(Succeed())
		found := false
		for _, raw := range proposal["conflicts"].([]any) {
			row := raw.(map[string]any)
			if row["path"] == path {
				found = true
				Expect(row["reason"]).To(ContainSubstring("preserved"))
			}
		}
		Expect(found).To(BeTrue(), "edited required cache must be a visible ownership conflict, not a generic recipe refusal: %+v", out)
		Expect(installationSnapshot(root)).To(Equal(before))
		Expect(os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))).To(Equal(custom))
	})
	// per docs/adr/0115-frozen-rc3-recipe-compatibility.md:38
	It("guards both new source members against individual current-recipe changes or absence", func() {
		f, driver := handoffNativeFixture(fixture.after)
		root := filepath.Join(f.cwd, "current recipe project")
		Expect(os.Mkdir(root, 0700)).To(Succeed())
		imageGit(root, "init", "--quiet")
		out := handoffNativeProcess(f.cwd, f.environment, recipeAnswers, driver, "init", root, "--go-candidate", handoffVersion, "--pack", "none")
		Expect(out.status).To(Equal(0), "%+v", out)
		args := recipeOperands(fixture.after, fixture.after, handoffVersion)
		before := installationSnapshot(root)
		healthy := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...)
		Expect(healthy.status).To(Equal(0), "equal current closure must remain supported: %+v", healthy)
		receipts := []string{}
		for _, path := range recipeCurrentMembers {
			for _, missing := range []bool{false, true} {
				changed := recipeAsset(fixture.after.archive, path, []byte("package installation\n// incompatible current recipe\n"), missing)
				archivePath := filepath.Join(fixture.after.work, "changed-current-recipe.tar.gz")
				writeFixture(archivePath, changed, 0600)
				out = handoffNativeProcess(root, f.environment, "", driver, append(recipeBeforePath(args, archivePath), "--dry-run", "--json")...)
				Expect(out.status).To(Equal(2), "%s missing=%t: %+v", path, missing, out)
				Expect(out.stderr).To(ContainSubstring("recipe"))
				Expect(installationSnapshot(root)).To(Equal(before), path)
				receipts = append(receipts, fmt.Sprintf("%s missing=%t status=%d", path, missing, out.status))
			}
		}
		Expect(receipts).To(HaveLen(4))
		AddReportEntry("historical-recipe.current-member-refusals", receipts, ReportEntryVisibilityNever)
	})
})
