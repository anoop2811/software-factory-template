package acceptance_test

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/staging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These wire fixtures are independent of the production source-identity decoder.
// per docs/adr/0149-compiler-provenance-and-security-patches.md:33
var _ = Describe("Compiler provenance across artifact readers", Ordered, ContinueOnFailure, func() {
	const version = "v9.7.4"
	const target = "linux/amd64"
	const revision = "0123456789012345678901234567890123456789"
	const prefix = version + "/" + target + "/"
	var stager string
	var baselines []imageBaselineOracle
	BeforeAll(func() {
		repository, err := filepath.Abs("..")
		Expect(err).NotTo(HaveOccurred())
		stager = filepath.Join(GinkgoT().TempDir(), "factory-stage")
		args := []string{"build", "-o", stager, "./cmd/factory-stage"}
		if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
			args = append([]string{"build", "-race"}, args[1:]...)
		}
		built := imageProcess(repository, nil, "go", args...)
		Expect(built.status).To(BeZero(), "%+v", built)
		for _, reference := range []imageBaselineOracle{{Label: "v0.1.6", Revision: releaseReaderBaseline}, {Label: "bash-baseline", Revision: "76952eaa63aebd1ecd282f5ab51dd7c3627cb497"}} {
			reference.Assets, _ = installationGitCatalog(repository, reference.Revision)
			baselines = append(baselines, reference)
		}
	})
	for _, owner := range []string{"runtime staging", "installation staging", "installation inspection"} {
		Context(owner, func() {
			var root, archive, destination, marker string
			var entries []stageEntry
			BeforeEach(func() {
				root = GinkgoT().TempDir()
				archive, destination, marker = filepath.Join(root, "source.tar.gz"), filepath.Join(root, "staged"), filepath.Join(root, "executed")
				binary := []byte("#!/bin/sh\nprintf executed > '" + strings.ReplaceAll(marker, "'", "'\\''") + "'\n")
				asset := []byte("inert independent source fixture\n")
				assets := []imageAssetOracle{{"README.md", "100644", fmt.Sprintf("%x", sha256.Sum256(asset)), int64(len(asset))}}
				manifest := imageManifestOracle{1, "installation_source_image", "committed_source_and_baseline_references", version, target, revision, false, assets, baselines}
				entries = imageFixtureEntries(manifest, map[string][]byte{"README.md": asset}, binary)
				if owner == "runtime staging" {
					entries = entries[:3]
					entries[2].body = bytes.ReplaceAll(entries[2].body, []byte("installation-source-build"), []byte("source-build"))
				}
			})
			setCompiler := func(compiler string) {
				GinkgoHelper()
				var source map[string]string
				Expect(json.Unmarshal(entries[2].body, &source)).To(Succeed())
				source["go_version"] = compiler
				entries[2].body = imageMarshal(source)
			}
			check := func(success bool, diagnostic string) {
				GinkgoHelper()
				body := stageArchive(entries)
				writeFixture(archive, body, 0600)
				before := installationSnapshot(root)
				if owner == "installation inspection" {
					inspected, err := staging.Inspect(context.Background(), staging.Options{Archive: archive, Version: version, Revision: revision, Target: target, Local: true})
					if success {
						Expect(err).NotTo(HaveOccurred())
						Expect(inspected.Source).To(Equal(entries[2].body))
						Expect(inspected.ArchiveSHA256).To(Equal(fmt.Sprintf("%x", sha256.Sum256(body))))
						Expect(inspected.Manifest.Version).To(Equal(version))
						Expect(inspected.Manifest.SourceRevision).To(Equal(revision))
						Expect(inspected.Manifest.Target).To(Equal(target))
					} else {
						Expect(err).To(HaveOccurred())
						if diagnostic != "" {
							Expect(err.Error()).To(ContainSubstring(diagnostic))
						}
					}
					Expect(installationSnapshot(root)).To(Equal(before), "read-only inspection must not publish or execute anything")
				} else {
					args := []string{"--archive", archive, "--version", version, "--revision", revision, "--target", target, "--output", destination, "--local"}
					if owner == "installation staging" {
						args = append(args, "--installation")
					}
					result := imageProcess(root, nil, stager, args...)
					if success {
						Expect(result.status).To(BeZero(), "%+v", result)
						Expect(os.ReadFile(filepath.Join(destination, prefix, "source.json"))).To(Equal(entries[2].body))
						Expect(os.ReadFile(filepath.Join(destination, prefix, "bin/factory-runtime"))).To(Equal(entries[0].body))
					} else {
						Expect(result.status).To(Equal(1), "%+v", result)
						Expect(result.stdout).To(BeEmpty())
						if diagnostic != "" {
							Expect(result.stderr).To(ContainSubstring(diagnostic))
						}
						Expect(installationSnapshot(root)).To(Equal(before), "failed staging must not publish output or change its input")
					}
				}
				Expect(os.ReadFile(archive)).To(Equal(body))
				_, err := os.Lstat(marker)
				Expect(os.IsNotExist(err)).To(BeTrue(), "compiler provenance never authorizes executing payload bytes")
			}

			// per docs/adr/0149-compiler-provenance-and-security-patches.md:28
			// per docs/adr/0149-compiler-provenance-and-security-patches.md:76
			DescribeTable("accepts canonical compiler provenance without equating it to the reader pin", func(compiler string) {
				setCompiler(compiler)
				check(true, "")
			}, Entry("historical patch", "go1.27.3"), Entry("current patch", "go1.27.4"), Entry("future grammar control", "go1.28.0"), Entry("zero components", "go1.0.0"), Entry("maximum component width", "go1.999999999.999999999"))

			// per docs/adr/0149-compiler-provenance-and-security-patches.md:277
			// per docs/adr/0149-compiler-provenance-and-security-patches.md:37
			DescribeTable("refuses malformed compiler provenance", func(compiler string) {
				setCompiler(compiler)
				check(false, "compiler provenance")
			}, Entry("empty", ""), Entry("missing prefix", "1.27.4"), Entry("wrong major", "go2.27.4"), Entry("missing patch", "go1.27"), Entry("minor leading zero", "go1.027.4"), Entry("patch leading zero", "go1.27.04"), Entry("signed component", "go1.+27.4"), Entry("leading space", " go1.27.4"), Entry("trailing newline", "go1.27.4\n"), Entry("prerelease", "go1.28rc1"), Entry("custom suffix", "go1.27.4-custom"), Entry("development", "devel go1.28"), Entry("minor overflow", "go1.1000000000.0"), Entry("patch overflow", "go1.27.1000000000"))

			// per docs/adr/0149-compiler-provenance-and-security-patches.md:33
			// per docs/adr/0149-compiler-provenance-and-security-patches.md:43
			DescribeTable("keeps exact source identity and artifact controls with future compiler provenance", func(scenario string) {
				setCompiler("go1.28.0")
				var source map[string]any
				Expect(json.Unmarshal(entries[2].body, &source)).To(Succeed())
				switch scenario {
				case "revision", "version", "target", "kind":
					source[scenario] = "wrong independent identity"
				case "missing":
					delete(source, "go_version")
				case "unknown":
					source["compiler_override"] = "go1.28.0"
				case "number":
					source["go_version"] = 1274
				case "null":
					source["go_version"] = nil
				}
				entries[2].body = imageMarshal(source)
				switch scenario {
				case "duplicate":
					entries[2].body = append(append([]byte{}, entries[2].body[:len(entries[2].body)-1]...), []byte(",\"go_version\":\"go1.28.0\"}")...)
				case "trailing":
					entries[2].body = append(entries[2].body, []byte(" {}")...)
				case "binary digest":
					entries[0].body = append(entries[0].body, []byte("# not the recorded executable\n")...)
				case "inventory":
					entries = append(entries, stageEntry{prefix + "unowned", []byte("not an allowed member"), 0644, tar.TypeReg})
				}
				check(false, "")
			}, Entry("revision", "revision"), Entry("version", "version"), Entry("target", "target"), Entry("kind", "kind"), Entry("missing compiler", "missing"), Entry("unknown field", "unknown"), Entry("non-string compiler", "number"), Entry("null compiler", "null"), Entry("duplicate compiler", "duplicate"), Entry("trailing object", "trailing"), Entry("binary digest", "binary digest"), Entry("closed inventory", "inventory"))
		})
	}
})
