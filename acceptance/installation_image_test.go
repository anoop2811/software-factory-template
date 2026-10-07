package acceptance_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Independent wire types and Git oracle; no packaging/image implementation is
// imported here. per docs/adr/0097-installation-source-image-bundles.md:110
type imageAssetOracle struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type imageBaselineOracle struct {
	Label    string             `json:"label"`
	Revision string             `json:"revision"`
	Assets   []imageAssetOracle `json:"assets"`
}

type imageManifestOracle struct {
	SchemaVersion   int                   `json:"schema_version"`
	Mode            string                `json:"mode"`
	Scope           string                `json:"scope"`
	Version         string                `json:"version"`
	Target          string                `json:"target"`
	SourceRevision  string                `json:"source_revision"`
	ActivationReady bool                  `json:"activation_ready"`
	Assets          []imageAssetOracle    `json:"assets"`
	Baselines       []imageBaselineOracle `json:"baselines"`
}

func imageProcess(dir string, overrides map[string]string, program string, argv ...string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, program, argv...) // #nosec G204 -- test-owned native binaries and literal subprocess operands.
	command.Dir = dir
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_") {
			continue
		}
		if _, replaced := overrides[key]; !replaced {
			command.Env = append(command.Env, entry)
		}
	}
	for key, value := range overrides {
		command.Env = append(command.Env, key+"="+value)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred(), "subprocess timeout: %s", stderr.String())
	status := 0
	if err != nil {
		var exit *exec.ExitError
		Expect(errors.As(err, &exit)).To(BeTrue(), "subprocess could not start: %v", err)
		status = exit.ExitCode()
	}
	return cliResult{stdout.String(), stderr.String(), status}
}

func imageGit(dir string, argv ...string) string {
	GinkgoHelper()
	args := append([]string{"--no-replace-objects", "-c", "core.hooksPath=" + os.DevNull, "-c", "commit.gpgsign=false", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.com"}, argv...)
	result := imageProcess(dir, nil, "git", args...)
	Expect(result.status).To(Equal(0), result.stderr)
	return result.stdout
}

func imageGitOracle(dir, revision string) ([]imageAssetOracle, map[string][]byte) {
	GinkgoHelper()
	listing := imageGit(dir, "ls-tree", "-r", "-z", revision)
	assets := make([]imageAssetOracle, 0)
	payload := make(map[string][]byte)
	for _, record := range strings.Split(listing, "\x00") {
		if record == "" {
			continue
		}
		meta, name, ok := strings.Cut(record, "\t")
		Expect(ok).To(BeTrue())
		fields := strings.Fields(meta)
		Expect(fields).To(HaveLen(3))
		Expect(fields[1]).To(Equal("blob"), "oracle expected a committed blob: %s", name)
		body := []byte(imageGit(dir, "cat-file", "blob", fields[2]))
		assets = append(assets, imageAssetOracle{name, fields[0], fmt.Sprintf("%x", sha256.Sum256(body)), int64(len(body))})
		payload[name] = body
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Path < assets[j].Path })
	return assets, payload
}

func imageCatalogDigest(assets []imageAssetOracle) string {
	hash := sha256.New()
	_, _ = io.WriteString(hash, "FACTORY_INSTALLATION_CATALOG_V1\n")
	for _, asset := range assets {
		for _, value := range []string{asset.Path, asset.Mode, asset.SHA256, strconv.FormatInt(asset.Bytes, 10)} {
			_, _ = io.WriteString(hash, value+"\x00")
		}
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func imageMarshal(value any) []byte {
	GinkgoHelper()
	data, err := json.Marshal(value)
	Expect(err).NotTo(HaveOccurred())
	return data
}

func imageFixtureEntries(manifest imageManifestOracle, payload map[string][]byte, binary []byte) []stageEntry {
	prefix := manifest.Version + "/" + manifest.Target + "/"
	runtimeManifest := fmt.Sprintf("FACTORY_RUNTIME_ARTIFACT_V1\nversion\t%s\ntarget\t%s\nbinary\tbin/factory-runtime\nsha256\t%x\nEND\n", manifest.Version, manifest.Target, sha256.Sum256(binary))
	source := map[string]string{"revision": manifest.SourceRevision, "version": manifest.Version, "target": manifest.Target, "go_version": "go1.27.1", "kind": "installation-source-build"}
	entries := []stageEntry{
		{prefix + "bin/factory-runtime", binary, 0755, tar.TypeReg},
		{prefix + "runtime.manifest", []byte(runtimeManifest), 0644, tar.TypeReg},
		{prefix + "source.json", imageMarshal(source), 0644, tar.TypeReg},
		{prefix + "installation.manifest", imageMarshal(manifest), 0644, tar.TypeReg},
	}
	for _, asset := range manifest.Assets {
		entries = append(entries, stageEntry{prefix + "installation/assets/" + asset.Path, payload[asset.Path], 0644, tar.TypeReg})
	}
	return entries
}

func imageDecodeArchive(data []byte) ([]stageEntry, []*tar.Header) {
	GinkgoHelper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	Expect(err).NotTo(HaveOccurred())
	defer gz.Close()
	reader := tar.NewReader(gz)
	var entries []stageEntry
	var headers []*tar.Header
	for {
		header, readErr := reader.Next()
		if errors.Is(readErr, io.EOF) {
			break
		}
		Expect(readErr).NotTo(HaveOccurred())
		body, readErr := io.ReadAll(reader)
		Expect(readErr).NotTo(HaveOccurred())
		entries = append(entries, stageEntry{header.Name, body, header.Mode, header.Typeflag})
		headers = append(headers, header)
	}
	return entries, headers
}

func imageAssertTree(root string, entries []stageEntry, staged bool) {
	GinkgoHelper()
	confined, err := os.OpenRoot(root)
	Expect(err).NotTo(HaveOccurred())
	defer confined.Close()
	expected := make(map[string]stageEntry, len(entries))
	for _, entry := range entries {
		expected[filepath.FromSlash(entry.name)] = entry
	}
	Expect(fs.WalkDir(confined.FS(), ".", func(relative string, item fs.DirEntry, walkErr error) error {
		Expect(walkErr).NotTo(HaveOccurred())
		info, err := confined.Lstat(relative)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode() & os.ModeSymlink).To(BeZero())
		if item.IsDir() {
			if staged {
				Expect(info.Mode().Perm()).To(Equal(os.FileMode(0700)), relative)
			}
			return nil
		}
		entry, exists := expected[filepath.FromSlash(relative)]
		Expect(exists).To(BeTrue(), "undeclared published file: %s", relative)
		data, err := confined.ReadFile(relative)
		Expect(err).NotTo(HaveOccurred())
		Expect(data).To(Equal(entry.body), relative)
		mode := os.FileMode(0644)
		if entry.name == entries[0].name {
			mode = 0755
		}
		Expect(entry.mode).To(Equal(int64(mode)), relative)
		if staged {
			mode = 0600
			if entry.name == entries[0].name {
				mode = 0700
			}
		}
		Expect(info.Mode().Perm()).To(Equal(mode), relative)
		delete(expected, filepath.FromSlash(relative))
		return nil
	})).To(Succeed())
	Expect(expected).To(BeEmpty(), "missing published files")
}

// Native outside-in evidence only; source images grant no installation authority.
// per docs/adr/0097-installation-source-image-bundles.md:173
var _ = Describe("G1 complete installation source images", Ordered, ContinueOnFailure, func() {
	const version = "v1.2.3"
	const revision = "0123456789012345678901234567890123456789"
	var root, work, packager, stager, source, current, target, marker string
	var baselines []imageBaselineOracle
	var sequence, fixtureSequence int
	BeforeAll(func() {
		var err error
		root, err = filepath.Abs("..")
		Expect(err).NotTo(HaveOccurred())
		work, err = os.MkdirTemp("", "factory installation image ")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, work)
		work, err = filepath.EvalSymlinks(work)
		Expect(err).NotTo(HaveOccurred())
		packager, stager = filepath.Join(work, "factory-package"), filepath.Join(work, "factory-stage")
		for program, destination := range map[string]string{"factory-package": packager, "factory-stage": stager} {
			args := []string{"build", "-o", destination, "./cmd/" + program}
			if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
				args = append([]string{"build", "-race"}, args[1:]...)
			}
			result := imageProcess(root, nil, "go", args...)
			Expect(result.status).To(Equal(0), result.stderr)
		}
		target = runtime.GOOS + "/" + runtime.GOARCH
		// per docs/adr/0097-installation-source-image-bundles.md:91
		for _, reference := range []imageBaselineOracle{{Label: "v0.1.6", Revision: releaseReaderBaseline}, {Label: "bash-baseline", Revision: "76952eaa63aebd1ecd282f5ab51dd7c3627cb497"}} {
			reference.Assets, _ = imageGitOracle(root, reference.Revision)
			baselines = append(baselines, reference)
		}
		Expect(baselines[0].Assets).To(HaveLen(137))
		Expect(baselines[1].Assets).To(HaveLen(160))
		Expect(imageCatalogDigest(baselines[0].Assets)).To(Equal("595f75dddcda285ba3b43a0936970ef96af3b9eae66f4beb9fd3a37df5d7e16b"))
		Expect(imageCatalogDigest(baselines[1].Assets)).To(Equal("8514055a8ee6f79a8fd891a6107e46bd9d0f1a2096e349825db83aafc7b91bbe"))
	})
	BeforeEach(func() {
		sequence++
		marker = filepath.Join(work, fmt.Sprintf("executed-%d", sequence))
	})
	output := func(label string) string { return filepath.Join(work, fmt.Sprintf("%d-%s", sequence, label)) }
	commit := func() string {
		imageGit(source, "add", ".")
		imageGit(source, "commit", "--quiet", "-m", "fixture")
		return strings.TrimSpace(imageGit(source, "rev-parse", "HEAD"))
	}
	fixture := func(withBaselines bool) {
		GinkgoHelper()
		fixtureSequence++
		source = output(fmt.Sprintf("source-%d", fixtureSequence))
		writeFixture(filepath.Join(source, "go.mod"), []byte("module example.com/installation-fixture\n\ngo 1.27.1\n"), 0600)
		writeFixture(filepath.Join(source, "cmd/factory/main.go"), []byte("package main\nimport (\"fmt\"; \"os\")\nfunc main() { _ = os.WriteFile("+strconv.Quote(marker)+", []byte(\"native fixture\"), 0600); fmt.Println(\"committed installation fixture\") }\n"), 0600)
		writeFixture(filepath.Join(source, "docs/evidence.txt"), []byte("$Format:%H$\ncommitted evidence\x00\xff"), 0644)
		writeFixture(filepath.Join(source, "bin/factory-runtime"), []byte("source collision remains data"), 0755)
		writeFixture(filepath.Join(source, "empty"), nil, 0644)
		writeFixture(filepath.Join(source, ".factory-not-private"), []byte("permitted name"), 0644)
		writeFixture(filepath.Join(source, "unicode-é"), []byte("Unicode bytes"), 0644)
		Expect(os.Symlink(marker, filepath.Join(source, "raw-link"))).To(Succeed())
		imageGit(source, "init", "--quiet", "--template=")
		if withBaselines {
			objects := strings.TrimSpace(imageGit(root, "rev-parse", "--path-format=absolute", "--git-path", "objects"))
			writeFixture(filepath.Join(source, ".git/objects/info/alternates"), []byte(objects+"\n"), 0600)
		}
		current = commit()
	}
	packageArgs := func(destination string, installation bool) []string {
		args := []string{"--source", source, "--revision", current, "--target", target, "--output", destination, "--version", version}
		if installation {
			args = append(args, "--installation")
		}
		return args
	}
	stageArgs := func(archive, destination, rev string, installation bool) []string {
		args := []string{"--archive", archive, "--version", version, "--revision", rev, "--target", target, "--output", destination, "--local"}
		if installation {
			args = append(args, "--installation")
		}
		return args
	}
	freshManifest := func() (imageManifestOracle, map[string][]byte, []byte) {
		GinkgoHelper()
		payload := map[string][]byte{"bin/factory-runtime": []byte("source executable must stay inert"), "raw-link": []byte(marker)}
		assets := []imageAssetOracle{}
		for _, name := range []string{"bin/factory-runtime", "raw-link"} {
			mode := "100755"
			if name == "raw-link" {
				mode = "120000"
			}
			body := payload[name]
			assets = append(assets, imageAssetOracle{name, mode, fmt.Sprintf("%x", sha256.Sum256(body)), int64(len(body))})
		}
		var copied []imageBaselineOracle
		Expect(json.Unmarshal(imageMarshal(baselines), &copied)).To(Succeed())
		manifest := imageManifestOracle{1, "installation_source_image", "committed_source_and_baseline_references", version, target, revision, false, assets, copied}
		binary := []byte("#!/bin/sh\nprintf executed > " + "'" + strings.ReplaceAll(marker, "'", "'\\''") + "'\n")
		return manifest, payload, binary
	}
	writeArchive := func(label string, entries []stageEntry) string {
		GinkgoHelper()
		name := output(label + ".tar.gz")
		writeFixture(name, stageArchive(entries), 0600)
		return name
	}
	assertStageSuccess := func(entries []stageEntry, label string) string {
		GinkgoHelper()
		archive, destination := writeArchive(label, entries), output(label+"-staged")
		result := imageProcess(work, nil, stager, stageArgs(archive, destination, revision, true)...)
		Expect(result.status).To(Equal(0), result.stderr)
		var summary map[string]string
		Expect(json.Unmarshal([]byte(result.stdout), &summary)).To(Succeed())
		Expect(summary).To(Equal(map[string]string{"version": version, "revision": revision, "target": target, "root": destination, "sha256": fmt.Sprintf("%x", sha256.Sum256(stageArchive(entries))), "authentication": "local-source"}))
		imageAssertTree(destination, entries, true)
		_, err := os.Lstat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue(), "staging executed or followed source image data")
		return archive
	}
	assertStageFailure := func(entries []stageEntry, label string) cliResult {
		GinkgoHelper()
		archive, destination := writeArchive(label, entries), output(label+"-staged")
		result := imageProcess(work, nil, stager, stageArgs(archive, destination, revision, true)...)
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
		_, err := os.Lstat(destination)
		Expect(os.IsNotExist(err)).To(BeTrue(), "refusal published final output")
		_, err = os.Lstat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue(), "refusal executed payload")
		return result
	}

	// per docs/adr/0097-installation-source-image-bundles.md:33
	// per docs/adr/0097-installation-source-image-bundles.md:51
	// per docs/adr/0097-installation-source-image-bundles.md:73
	// per docs/adr/0097-installation-source-image-bundles.md:89
	// per docs/adr/0097-installation-source-image-bundles.md:141
	It("accepts the producer installation flag and emits the complete committed image", func() {
		fixture(true)
		destination := output("bundle")
		result := imageProcess(work, nil, packager, packageArgs(destination, true)...)
		Expect(result.status).To(Equal(0), result.stderr)
		var summary map[string]string
		Expect(json.Unmarshal([]byte(result.stdout), &summary)).To(Succeed())
		archivePath := filepath.Join(destination, "factory-runtime.tar.gz")
		data, err := os.ReadFile(archivePath)
		Expect(err).NotTo(HaveOccurred())
		digest := fmt.Sprintf("%x", sha256.Sum256(data))
		Expect(summary).To(Equal(map[string]string{"version": version, "revision": current, "target": target, "archive": archivePath, "sha256": digest}))
		checksums, err := os.ReadFile(filepath.Join(destination, "SHA256SUMS"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(checksums)).To(Equal(digest + "  factory-runtime.tar.gz\n"))
		entries, headers := imageDecodeArchive(data)
		assets, payload := imageGitOracle(source, current)
		Expect(entries).To(HaveLen(4 + len(assets)))
		var manifest imageManifestOracle
		Expect(json.Unmarshal(entries[3].body, &manifest)).To(Succeed())
		Expect(manifest).To(Equal(imageManifestOracle{1, "installation_source_image", "committed_source_and_baseline_references", version, target, current, false, assets, baselines}))
		expected := imageFixtureEntries(manifest, payload, entries[0].body)
		var sourceMetadata map[string]string
		Expect(json.Unmarshal(entries[2].body, &sourceMetadata)).To(Succeed())
		Expect(sourceMetadata).To(Equal(map[string]string{"revision": current, "version": version, "target": target, "go_version": "go1.27.1", "kind": "installation-source-build"}))
		var imageMetadata map[string]json.RawMessage
		Expect(json.Unmarshal(entries[3].body, &imageMetadata)).To(Succeed())
		Expect(imageMetadata).To(HaveLen(9))
		// JSON object order and final whitespace are not wire requirements.
		expected[2].body, expected[3].body = entries[2].body, entries[3].body
		Expect(entries).To(Equal(expected))
		for _, header := range headers {
			Expect(header.Uid).To(BeZero())
			Expect(header.Gid).To(BeZero())
			Expect(header.ModTime.Unix()).To(BeZero())
		}
		imageAssertTree(filepath.Join(destination, "store"), entries, false)
		staged := output("roundtrip")
		result = imageProcess(work, nil, stager, stageArgs(archivePath, staged, current, true)...)
		Expect(result.status).To(Equal(0), result.stderr)
		imageAssertTree(staged, entries, true)
		_, err = os.Lstat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue(), "build/stage must never execute the candidate")
		// This explicit independent qualification is separate from Build/Stage.
		result = imageProcess(work, map[string]string{"PATH": output("no-tools")}, filepath.Join(staged, version, filepath.FromSlash(target), "bin/factory-runtime"))
		Expect(result).To(Equal(cliResult{"committed installation fixture\n", "", 0}))
		contents, err := os.ReadFile(marker)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(contents)).To(Equal("native fixture"))
	})

	// per docs/adr/0097-installation-source-image-bundles.md:153
	It("accepts the stager installation flag with an independently authored inert image", func() {
		manifest, payload, binary := freshManifest()
		assertStageSuccess(imageFixtureEntries(manifest, payload, binary), "independent-valid")
	})

	// per docs/adr/0097-installation-source-image-bundles.md:124
	It("isolates complete images from worktree edits untracked files export attributes and replacement refs", func() {
		fixture(true)
		writeFixture(filepath.Join(source, ".gitattributes"), []byte("* export-ignore\n*.txt export-subst\n"), 0600)
		current = commit()
		first, second := output("clean"), output("tainted")
		result := imageProcess(work, nil, packager, packageArgs(first, true)...)
		Expect(result.status).To(Equal(0), result.stderr)
		writeFixture(filepath.Join(source, "docs/evidence.txt"), []byte("replacement bytes"), 0600)
		writeFixture(filepath.Join(source, "cmd/factory/main.go"), []byte("package main\nfunc main() {}\n"), 0600)
		replacement := commit()
		imageGit(source, "replace", current, replacement)
		writeFixture(filepath.Join(source, ".git/info/attributes"), []byte("** export-ignore\n"), 0600)
		writeFixture(filepath.Join(source, "cmd/factory/untracked.go"), []byte("invalid Go"), 0600)
		result = imageProcess(work, map[string]string{"GOFLAGS": "-invalid-ambient", "GOWORK": output("absent.work"), "GOTOOLCHAIN": "invalid-toolchain", "GOEXPERIMENT": "invalid-experiment", "GOOS": "windows", "GOARCH": "386", "CGO_ENABLED": "1", "GOPROXY": "http://127.0.0.1:1", "GIT_CONFIG_COUNT": "1", "GIT_CONFIG_KEY_0": "core.attributesfile", "GIT_CONFIG_VALUE_0": output("untrusted-attributes")}, packager, packageArgs(second, true)...)
		Expect(result.status).To(Equal(0), result.stderr)
		one, err := os.ReadFile(filepath.Join(first, "factory-runtime.tar.gz"))
		Expect(err).NotTo(HaveOccurred())
		two, err := os.ReadFile(filepath.Join(second, "factory-runtime.tar.gz"))
		Expect(err).NotTo(HaveOccurred())
		Expect(two).To(Equal(one))
		entries, _ := imageDecodeArchive(two)
		var manifest imageManifestOracle
		Expect(json.Unmarshal(entries[3].body, &manifest)).To(Succeed())
		assets, _ := imageGitOracle(source, current)
		Expect(manifest.Assets).To(Equal(assets))
	})

	// per docs/adr/0097-installation-source-image-bundles.md:129
	It("refuses unavailable immutable baseline objects instead of falling back or fetching", func() {
		fixture(true)
		control := imageProcess(work, nil, packager, packageArgs(output("baseline-positive"), true)...)
		Expect(control.status).To(Equal(0), control.stderr)
		fixture(false)
		destination := output("missing-baselines")
		result := imageProcess(work, map[string]string{"GIT_TERMINAL_PROMPT": "0"}, packager, packageArgs(destination, true)...)
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
		_, err := os.Lstat(destination)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0097-installation-source-image-bundles.md:82
	// per docs/adr/0097-installation-source-image-bundles.md:114
	// per docs/adr/0097-installation-source-image-bundles.md:130
	DescribeTable("refuses actual unsafe or unsupported committed source entries", func(kind string) {
		fixture(true)
		control := imageProcess(work, nil, packager, packageArgs(output("source-positive"), true)...)
		Expect(control.status).To(Equal(0), control.stderr)
		switch kind {
		case "gitlink":
			imageGit(source, "update-index", "--add", "--cacheinfo", "160000,"+current+",external-module")
			imageGit(source, "commit", "--quiet", "-m", "unsupported gitlink")
			current = strings.TrimSpace(imageGit(source, "rev-parse", "HEAD"))
		case "private":
			writeFixture(filepath.Join(source, ".factory/private"), []byte("not distributable"), 0600)
			current = commit()
		case "newline":
			writeFixture(filepath.Join(source, "unsafe\nname"), []byte("invalid path"), 0600)
			current = commit()
		case "blob bound":
			writeFixture(filepath.Join(source, "oversized"), bytes.Repeat([]byte("x"), (1<<20)+1), 0600)
			current = commit()
		case "count bound":
			for i := range 4097 {
				writeFixture(filepath.Join(source, "many", fmt.Sprintf("blob-%04d", i)), nil, 0600)
			}
			current = commit()
		case "total bound":
			body := bytes.Repeat([]byte("x"), 1<<20)
			for i := range 65 {
				writeFixture(filepath.Join(source, "total", fmt.Sprintf("blob-%02d", i)), body, 0600)
			}
			current = commit()
		case "manifest bound":
			for i := range 1500 {
				writeFixture(filepath.Join(source, "manifest", strings.Repeat("m", 230)+fmt.Sprintf("-%04d", i)), nil, 0600)
			}
			current = commit()
		}
		destination := output("invalid-source")
		result := imageProcess(work, nil, packager, packageArgs(destination, true)...)
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
		_, err := os.Lstat(destination)
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("Git submodule", "gitlink"), Entry("runtime private tree", "private"), Entry("newline path", "newline"), Entry("actual source blob overflow", "blob bound"), Entry("actual source tree overflow", "count bound"), Entry("actual total source overflow", "total bound"), Entry("actual source manifest overflow", "manifest bound"))

	// per docs/adr/0097-installation-source-image-bundles.md:38
	It("keeps V1 default behavior and refuses either direction of format downgrade", func() {
		fixture(true)
		destination := output("v1-bundle")
		result := imageProcess(work, nil, packager, append(packageArgs(destination, false), "--installation=false")...)
		Expect(result.status).To(Equal(0), result.stderr)
		data, err := os.ReadFile(filepath.Join(destination, "factory-runtime.tar.gz"))
		Expect(err).NotTo(HaveOccurred())
		entries, _ := imageDecodeArchive(data)
		Expect(entries).To(HaveLen(3))
		result = imageProcess(work, nil, stager, stageArgs(filepath.Join(destination, "factory-runtime.tar.gz"), output("v1-staged"), current, false)...)
		Expect(result.status).To(Equal(0), result.stderr)
		result = imageProcess(work, nil, stager, stageArgs(filepath.Join(destination, "factory-runtime.tar.gz"), output("v1-as-image"), current, true)...)
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		manifest, payload, binary := freshManifest()
		archive := assertStageSuccess(imageFixtureEntries(manifest, payload, binary), "image-control")
		result = imageProcess(work, nil, stager, stageArgs(archive, output("image-as-v1"), revision, false)...)
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		for _, name := range []string{"v1-as-image", "image-as-v1"} {
			_, err = os.Lstat(output(name))
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	})

	// Every mutation is preceded by a successful identical fixture, so refusals
	// cannot pass vacuously against an unsupported flag or implementation stub.
	// per docs/adr/0097-installation-source-image-bundles.md:177
	// per docs/adr/0097-installation-source-image-bundles.md:59
	// per docs/adr/0097-installation-source-image-bundles.md:65
	// per docs/adr/0097-installation-source-image-bundles.md:73
	// per docs/adr/0097-installation-source-image-bundles.md:89
	DescribeTable("rejects paired malformed installation archive or metadata", func(kind string) {
		manifest, payload, binary := freshManifest()
		assertStageSuccess(imageFixtureEntries(manifest, payload, binary), "valid-control")
		entries := imageFixtureEntries(manifest, payload, binary)
		switch kind {
		case "duplicate control":
			entries = append(entries, entries[3])
		case "duplicate payload":
			entries = append(entries, entries[4])
		case "extra":
			entries = append(entries, stageEntry{version + "/" + target + "/extra", []byte("x"), 0644, tar.TypeReg})
		case "missing control":
			entries = append(entries[:3], entries[4:]...)
		case "missing payload":
			entries = entries[:len(entries)-1]
		case "control order":
			entries[2], entries[3] = entries[3], entries[2]
		case "payload order":
			entries[4], entries[5] = entries[5], entries[4]
		case "traversal":
			entries[4].name += "/../escaped"
		case "symlink":
			entries[4].kind = tar.TypeSymlink
		case "hardlink":
			entries[4].kind = tar.TypeLink
		case "directory":
			entries[4].kind = tar.TypeDir
		case "payload mode":
			entries[4].mode = 0755
		case "digest":
			entries[4].body[0] ^= 1
		case "short body":
			entries[4].body = entries[4].body[:len(entries[4].body)-1]
		case "long body":
			entries[4].body = append(entries[4].body, 'x')
		case "unknown root field":
			entries[3].body = bytes.Replace(entries[3].body, []byte(`"schema_version":1`), []byte(`"schema_version":1,"ownership":true`), 1)
		case "duplicate root field":
			entries[3].body = bytes.Replace(entries[3].body, []byte(`"schema_version":1`), []byte(`"schema_version":1,"schema_version":1`), 1)
		case "trailing JSON":
			entries[3].body = append(entries[3].body, []byte(" {}")...)
		case "activation authority":
			manifest.ActivationReady = true
		case "revision":
			manifest.SourceRevision = strings.Repeat("a", 40)
		case "version":
			manifest.Version = "v9.9.9"
		case "target":
			manifest.Target = "windows/amd64"
		case "schema":
			manifest.SchemaVersion = 2
		case "mode":
			manifest.Mode = "installed"
		case "scope":
			manifest.Scope = "owned_installation"
		case "logical mode":
			manifest.Assets[0].Mode = "160000"
		case "uppercase digest":
			manifest.Assets[0].SHA256 = strings.ToUpper(manifest.Assets[0].SHA256)
		case "negative length":
			manifest.Assets[0].Bytes = -1
		case "empty catalog":
			manifest.Assets = []imageAssetOracle{}
		case "duplicate catalog":
			manifest.Assets[1] = manifest.Assets[0]
		case "unsorted catalog":
			manifest.Assets[0], manifest.Assets[1] = manifest.Assets[1], manifest.Assets[0]
		case "prefix collision":
			manifest.Assets[1].Path = manifest.Assets[0].Path + "/child"
		case "private catalog":
			manifest.Assets[0].Path = ".factory/private"
		case "backslash catalog":
			manifest.Assets[0].Path = "bin\\factory-runtime"
		case "unknown asset field":
			entries[3].body = bytes.Replace(entries[3].body, []byte(`"mode":"100755"`), []byte(`"mode":"100755","owned":true`), 1)
		case "duplicate asset field":
			entries[3].body = bytes.Replace(entries[3].body, []byte(`"mode":"100755"`), []byte(`"mode":"100755","mode":"100755"`), 1)
		case "fractional length":
			entries[3].body = bytes.Replace(entries[3].body, []byte(`"bytes":`+strconv.Itoa(len(payload["bin/factory-runtime"]))), []byte(`"bytes":1.5`), 1)
		case "missing authority field":
			entries[3].body = bytes.Replace(entries[3].body, []byte(`"activation_ready":false,`), nil, 1)
		case "unpaired surrogate", "invalid UTF8":
			replacement := []byte(`"path":"\ud800"`)
			if kind == "invalid UTF8" {
				replacement = append([]byte(`"path":"`), 0xff, '"')
			}
			entries[3].body = bytes.Replace(entries[3].body, []byte(`"path":"raw-link"`), replacement, 1)
			entries[5].name = version + "/" + target + "/installation/assets/�"
		case "baseline order":
			manifest.Baselines[0], manifest.Baselines[1] = manifest.Baselines[1], manifest.Baselines[0]
		case "baseline revision":
			manifest.Baselines[0].Revision = strings.Repeat("b", 40)
		case "baseline label":
			manifest.Baselines[0].Label = "latest"
		case "baseline count":
			manifest.Baselines[0].Assets = manifest.Baselines[0].Assets[1:]
		case "baseline digest":
			manifest.Baselines[0].Assets[0].SHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte("plausible different baseline bytes")))
		case "baseline logical mode":
			manifest.Baselines[0].Assets[0].Mode = "100755"
		case "unknown baseline field":
			entries[3].body = bytes.Replace(entries[3].body, []byte(`"label":"v0.1.6"`), []byte(`"label":"v0.1.6","owned":true`), 1)
		case "source kind":
			entries[2].body = bytes.Replace(entries[2].body, []byte("installation-source-build"), []byte("source-build"), 1)
		}
		// Raw JSON mutations preserve the other fixture bytes. Path/map mutations
		// rebuild matching members so unrelated archive closure cannot mask them.
		switch kind {
		case "activation authority", "revision", "version", "target", "schema", "mode", "scope", "logical mode", "uppercase digest", "negative length", "empty catalog", "duplicate catalog", "unsorted catalog", "prefix collision", "private catalog", "backslash catalog", "baseline order", "baseline revision", "baseline label", "baseline count", "baseline digest", "baseline logical mode":
			entries[3].body = imageMarshal(manifest)
		}
		switch kind {
		case "empty catalog":
			entries = imageFixtureEntries(manifest, map[string][]byte{}, binary)
		case "prefix collision", "private catalog", "backslash catalog":
			renamed := map[string][]byte{manifest.Assets[0].Path: payload["bin/factory-runtime"], manifest.Assets[1].Path: payload["raw-link"]}
			entries = imageFixtureEntries(manifest, renamed, binary)
		}
		assertStageFailure(entries, "invalid")
	},
		Entry("duplicate control", "duplicate control"), Entry("duplicate payload", "duplicate payload"), Entry("extra member", "extra"), Entry("missing control", "missing control"), Entry("missing payload", "missing payload"), Entry("control order", "control order"), Entry("payload order", "payload order"), Entry("traversal", "traversal"), Entry("tar symlink", "symlink"), Entry("tar hardlink", "hardlink"), Entry("directory", "directory"), Entry("payload executable mode", "payload mode"), Entry("payload digest", "digest"), Entry("short payload", "short body"), Entry("long payload", "long body"),
		Entry("unknown outer field", "unknown root field"), Entry("duplicate outer field", "duplicate root field"), Entry("trailing JSON", "trailing JSON"), Entry("activation authority", "activation authority"), Entry("source revision", "revision"), Entry("version", "version"), Entry("target", "target"), Entry("schema", "schema"), Entry("mode", "mode"), Entry("scope", "scope"),
		Entry("logical mode", "logical mode"), Entry("uppercase digest", "uppercase digest"), Entry("negative length", "negative length"), Entry("empty catalog", "empty catalog"), Entry("duplicate catalog", "duplicate catalog"), Entry("unsorted catalog", "unsorted catalog"), Entry("ancestor collision", "prefix collision"), Entry("private path", "private catalog"), Entry("backslash path", "backslash catalog"), Entry("unknown asset field", "unknown asset field"), Entry("duplicate asset field", "duplicate asset field"), Entry("fractional length", "fractional length"), Entry("missing activation field", "missing authority field"), Entry("unpaired surrogate", "unpaired surrogate"), Entry("invalid UTF8", "invalid UTF8"),
		Entry("baseline order", "baseline order"), Entry("baseline revision", "baseline revision"), Entry("baseline label", "baseline label"), Entry("baseline count", "baseline count"), Entry("baseline digest", "baseline digest"), Entry("baseline logical mode", "baseline logical mode"), Entry("unknown baseline field", "unknown baseline field"), Entry("V1 source kind", "source kind"),
	)

	// per docs/adr/0097-installation-source-image-bundles.md:114
	It("accepts the exact manifest byte boundary and refuses one byte more", func() {
		manifest, payload, binary := freshManifest()
		entries := imageFixtureEntries(manifest, payload, binary)
		entries[3].body = append(entries[3].body, bytes.Repeat([]byte(" "), (512<<10)-len(entries[3].body))...)
		assertStageSuccess(entries, "manifest-boundary")
		entries[3].body = append(entries[3].body, ' ')
		assertStageFailure(entries, "manifest-overflow")
	})

	// per docs/adr/0097-installation-source-image-bundles.md:115
	It("accepts a one MiB source blob and refuses its one-byte overflow", func() {
		manifest, _, binary := freshManifest()
		body := bytes.Repeat([]byte("x"), 1<<20)
		manifest.Assets = []imageAssetOracle{{"large", "100644", fmt.Sprintf("%x", sha256.Sum256(body)), int64(len(body))}}
		entries := imageFixtureEntries(manifest, map[string][]byte{"large": body}, binary)
		assertStageSuccess(entries, "blob-boundary")
		body = append(body, 'x')
		manifest.Assets[0].Bytes++
		manifest.Assets[0].SHA256 = fmt.Sprintf("%x", sha256.Sum256(body))
		assertStageFailure(imageFixtureEntries(manifest, map[string][]byte{"large": body}, binary), "blob-overflow")
	})

	// per docs/adr/0097-installation-source-image-bundles.md:114
	It("accepts 4096 short named blobs and refuses the 4097th without a larger manifest", func() {
		manifest, _, binary := freshManifest()
		manifest.Assets = []imageAssetOracle{}
		payload := map[string][]byte{}
		// Lowercase names also work on case-insensitive supported filesystems.
		alphabet := "abcdefghijklmnopqrstuvwxyz0123456789"
		names := []string{}
		for _, a := range alphabet {
			names = append(names, string(a))
		}
		for _, a := range alphabet {
			for _, b := range alphabet {
				names = append(names, string(a)+string(b))
			}
		}
		for _, a := range alphabet {
			for _, b := range alphabet {
				for _, c := range alphabet {
					if len(names) < 4097 {
						names = append(names, string(a)+string(b)+string(c))
					}
				}
			}
		}
		sort.Strings(names)
		for _, name := range names[:4096] {
			manifest.Assets = append(manifest.Assets, imageAssetOracle{name, "100644", fmt.Sprintf("%x", sha256.Sum256(nil)), 0})
			payload[name] = nil
		}
		Expect(len(imageMarshal(manifest))).To(BeNumerically("<=", 512<<10))
		assertStageSuccess(imageFixtureEntries(manifest, payload, binary), "count-boundary")
		name := names[4096]
		manifest.Assets = append(manifest.Assets, imageAssetOracle{name, "100644", fmt.Sprintf("%x", sha256.Sum256(nil)), 0})
		payload[name] = nil
		Expect(len(imageMarshal(manifest))).To(BeNumerically("<=", 512<<10))
		assertStageFailure(imageFixtureEntries(manifest, payload, binary), "count-overflow")
	})

	// per docs/adr/0097-installation-source-image-bundles.md:115
	It("accepts 64 MiB total payload and refuses another bounded blob", func() {
		manifest, _, binary := freshManifest()
		manifest.Assets = []imageAssetOracle{}
		body := bytes.Repeat([]byte("x"), 1<<20)
		payload := map[string][]byte{}
		for i := range 64 {
			name := fmt.Sprintf("blob-%02d", i)
			manifest.Assets = append(manifest.Assets, imageAssetOracle{name, "100644", fmt.Sprintf("%x", sha256.Sum256(body)), int64(len(body))})
			payload[name] = body
		}
		assertStageSuccess(imageFixtureEntries(manifest, payload, binary), "total-boundary")
		manifest.Assets = append(manifest.Assets, imageAssetOracle{"blob-64", "100644", fmt.Sprintf("%x", sha256.Sum256([]byte("x"))), 1})
		payload["blob-64"] = []byte("x")
		assertStageFailure(imageFixtureEntries(manifest, payload, binary), "total-overflow")
	})

	// per docs/adr/0097-installation-source-image-bundles.md:80
	It("accepts a 1024-byte logical path and refuses its one-byte overflow", func() {
		manifest, _, binary := freshManifest()
		name := strings.Repeat(strings.Repeat("a", 200)+"/", 4) + strings.Repeat("b", 220)
		Expect(name).To(HaveLen(1024))
		body := []byte("path boundary")
		manifest.Assets = []imageAssetOracle{{name, "100644", fmt.Sprintf("%x", sha256.Sum256(body)), int64(len(body))}}
		assertStageSuccess(imageFixtureEntries(manifest, map[string][]byte{name: body}, binary), "path-boundary")
		manifest.Assets[0].Path += "x"
		assertStageFailure(imageFixtureEntries(manifest, map[string][]byte{name + "x": body}, binary), "path-overflow")
	})

	// per docs/adr/0097-installation-source-image-bundles.md:117
	It("accepts the exact aggregate wire boundary including padding and rejects one byte more", func() {
		manifest, payload, binary := freshManifest()
		entries := imageFixtureEntries(manifest, payload, binary)
		data := stageArchive(entries)
		reader, err := gzip.NewReader(bytes.NewReader(data))
		Expect(err).NotTo(HaveOccurred())
		raw, err := io.ReadAll(reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(reader.Close()).To(Succeed())
		for _, excess := range []int64{0, 1} {
			var buffer bytes.Buffer
			gz := gzip.NewWriter(&buffer)
			_, err = gz.Write(raw)
			Expect(err).NotTo(HaveOccurred())
			remaining := int64(321<<20) + excess - int64(len(raw))
			zeros := make([]byte, 64<<10)
			for remaining > 0 {
				count := min(remaining, int64(len(zeros)))
				_, err = gz.Write(zeros[:count])
				Expect(err).NotTo(HaveOccurred())
				remaining -= count
			}
			Expect(gz.Close()).To(Succeed())
			archive, destination := output(fmt.Sprintf("aggregate-%d.tar.gz", excess)), output(fmt.Sprintf("aggregate-%d", excess))
			writeFixture(archive, buffer.Bytes(), 0600)
			result := imageProcess(work, nil, stager, stageArgs(archive, destination, revision, true)...)
			if excess == 0 {
				Expect(result.status).To(Equal(0), result.stderr)
				imageAssertTree(destination, entries, true)
			} else {
				Expect(result.status).To(Equal(1), result.stderr)
				Expect(result.stdout).To(BeEmpty())
				_, err = os.Lstat(destination)
				Expect(os.IsNotExist(err)).To(BeTrue())
			}
		}
	})

	// per docs/adr/0097-installation-source-image-bundles.md:116
	It("rejects an oversized compressed snapshot before interpreting its bytes", func() {
		manifest, payload, binary := freshManifest()
		assertStageSuccess(imageFixtureEntries(manifest, payload, binary), "compressed-control")
		archive := output("compressed-overflow.tar.gz")
		file, err := os.Create(archive)
		Expect(err).NotTo(HaveOccurred())
		Expect(file.Truncate((128 << 20) + 1)).To(Succeed())
		Expect(file.Close()).To(Succeed())
		destination := output("compressed-overflow")
		result := imageProcess(work, nil, stager, stageArgs(archive, destination, revision, true)...)
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stderr).To(ContainSubstring("archive snapshot:"))
		Expect(result.stdout).To(BeEmpty())
		_, err = os.Lstat(destination)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// Fake success proves delegated trust/snapshot transport only; it does not
	// establish actual signature acceptance. per docs/adr/0097-installation-source-image-bundles.md:184
	It("authenticates the whole immutable snapshot before staging and binds every release identity", func() {
		manifest, payload, binary := freshManifest()
		entries := imageFixtureEntries(manifest, payload, binary)
		archive := writeArchive("release", entries)
		attestation, roots, verifier, log := output("attestation"), output("roots"), output("gh"), output("verification")
		writeFixture(attestation, []byte("independent attestation fixture"), 0600)
		writeFixture(roots, []byte("independently provisioned roots"), 0600)
		quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
		writeFixture(verifier, []byte("#!/bin/sh\nset -eu\nprintf '%s\\n' \"$@\" > "+quote(log)+"\nprintf '%s' \"$GH_HOST\" > "+quote(log+".host")+"\ncp \"$3\" "+quote(log+".archive")+"\nprintf replaced > "+quote(archive)+"\n"), 0755)
		args := stageArgs(archive, output("release-staged"), revision, true)
		args = args[:len(args)-2]
		args = append(args, "--installation", "--attestation", attestation, "--trusted-root", roots, "--gh", verifier)
		result := imageProcess(work, map[string]string{"GH_HOST": "wrong.example"}, stager, args...)
		Expect(result.status).To(Equal(0), result.stderr)
		var summary map[string]string
		Expect(json.Unmarshal([]byte(result.stdout), &summary)).To(Succeed())
		Expect(summary).To(HaveLen(6))
		Expect(summary["authentication"]).To(Equal("github-attestation"))
		Expect(summary["sha256"]).To(Equal(fmt.Sprintf("%x", sha256.Sum256(stageArchive(entries)))))
		imageAssertTree(output("release-staged"), entries, true)
		argvData, err := os.ReadFile(log)
		Expect(err).NotTo(HaveOccurred())
		argv := strings.Split(strings.TrimSpace(string(argvData)), "\n")
		Expect(argv[:2]).To(Equal([]string{"attestation", "verify"}))
		Expect(argv[2]).NotTo(Equal(archive))
		for _, pair := range [][]string{{"--repo", "anoop2811/software-factory-template"}, {"--signer-workflow", "anoop2811/software-factory-template/.github/workflows/runtime-release.yml"}, {"--source-digest", revision}, {"--source-ref", "refs/tags/" + version}} {
			index := -1
			for i, operand := range argv {
				if operand == pair[0] {
					index = i
				}
			}
			Expect(index).To(BeNumerically(">=", 0))
			Expect(argv[index+1]).To(Equal(pair[1]))
		}
		Expect(argv).To(ContainElement("--deny-self-hosted-runners"))
		for _, original := range []string{attestation, roots} {
			Expect(argv).NotTo(ContainElement(original))
		}
		host, err := os.ReadFile(log + ".host")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(host)).To(Equal("github.com"))
		snapshot, err := os.ReadFile(log + ".archive")
		Expect(err).NotTo(HaveOccurred())
		Expect(snapshot).To(Equal(stageArchive(entries)))
		_, err = os.Lstat(argv[2])
		Expect(os.IsNotExist(err)).To(BeTrue(), "private snapshot leaked after staging")
		_, err = os.Lstat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0097-installation-source-image-bundles.md:148
	It("refuses failed release verification before parsing malformed installation bytes with no local fallback", func() {
		manifest, payload, binary := freshManifest()
		assertStageSuccess(imageFixtureEntries(manifest, payload, binary), "trust-positive")
		archive, attestation, roots, verifier, called := output("malformed.tar.gz"), output("bundle"), output("roots"), output("refusing-gh"), output("called")
		writeFixture(archive, []byte("not gzip and not installation metadata"), 0600)
		writeFixture(attestation, []byte("bundle"), 0600)
		writeFixture(roots, []byte("independent roots"), 0600)
		writeFixture(verifier, []byte("#!/bin/sh\nprintf called > '"+called+"'\nprintf 'independent verification refusal' >&2\nexit 9\n"), 0755)
		destination := output("trust-refused")
		args := []string{"--archive", archive, "--version", version, "--revision", revision, "--target", target, "--output", destination, "--installation", "--attestation", attestation, "--trusted-root", roots, "--gh", verifier}
		result := imageProcess(work, nil, stager, args...)
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).To(ContainSubstring("independent verification refusal"))
		body, err := os.ReadFile(called)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal("called"))
		_, err = os.Lstat(destination)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})
