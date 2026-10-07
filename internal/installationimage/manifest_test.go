package installationimage_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/installationimage"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Pure Decode criteria distinguish schema rejection from a later native
// directory/file collision or closed-world tar rejection.
// per docs/adr/0097-installation-source-image-bundles.md:77
var _ = Describe("Installation source image catalog admission", Ordered, ContinueOnFailure, func() {
	var baselines []map[string]any
	identity := installationimage.Identity{Version: "v1.2.3", Target: "linux/amd64", Revision: "0123456789012345678901234567890123456789"}
	BeforeAll(func() {
		// Independently rebuild reference bytes; never call the production
		// collector, digest helper or validator to manufacture the fixture.
		// per docs/adr/0097-installation-source-image-bundles.md:110
		git := func(args ...string) []byte {
			GinkgoHelper()
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, "git", append([]string{"--no-replace-objects", "-C", "../.."}, args...)...) // #nosec G204 -- immutable test-owned Git reference operands; no shell.
			data, err := command.Output()
			Expect(err).NotTo(HaveOccurred())
			Expect(ctx.Err()).NotTo(HaveOccurred())
			return data
		}
		for _, reference := range []struct{ label, revision string }{{"v0.1.6", "b71ecc32e07ecd87eb330ba8e497c86612f92acd"}, {"bash-baseline", "76952eaa63aebd1ecd282f5ab51dd7c3627cb497"}} {
			assets := []map[string]any{}
			for _, record := range strings.Split(string(git("ls-tree", "-r", "-z", reference.revision)), "\x00") {
				if record == "" {
					continue
				}
				metadata, path, ok := strings.Cut(record, "\t")
				Expect(ok).To(BeTrue())
				fields := strings.Fields(metadata)
				Expect(fields).To(HaveLen(3))
				Expect(fields[1]).To(Equal("blob"))
				body := git("cat-file", "blob", fields[2])
				assets = append(assets, map[string]any{"path": path, "mode": fields[0], "sha256": fmt.Sprintf("%x", sha256.Sum256(body)), "bytes": len(body)})
			}
			sort.Slice(assets, func(i, j int) bool { return assets[i]["path"].(string) < assets[j]["path"].(string) })
			baselines = append(baselines, map[string]any{"label": reference.label, "revision": reference.revision, "assets": assets})
		}
		Expect(baselines[0]["assets"]).To(HaveLen(137))
		Expect(baselines[1]["assets"]).To(HaveLen(160))
	})
	asset := func(path string) map[string]any {
		return map[string]any{"path": path, "mode": "100644", "sha256": fmt.Sprintf("%x", sha256.Sum256([]byte("x"))), "bytes": 1}
	}
	manifest := func(assets []map[string]any) []byte {
		GinkgoHelper()
		data, err := json.Marshal(map[string]any{
			"schema_version": 1, "mode": "installation_source_image", "scope": "committed_source_and_baseline_references",
			"version": identity.Version, "target": identity.Target, "source_revision": identity.Revision,
			"activation_ready": false, "assets": assets, "baselines": baselines,
		})
		Expect(err).NotTo(HaveOccurred())
		return data
	}
	positive := func() {
		GinkgoHelper()
		decoded, err := installationimage.Decode(context.Background(), manifest([]map[string]any{asset("file"), asset("sibling")}), identity)
		Expect(err).NotTo(HaveOccurred())
		Expect(decoded.Assets).To(HaveLen(2))
		Expect(decoded.Assets[0].Path).To(Equal("file"))
		Expect(decoded.Assets[1].Path).To(Equal("sibling"))
		Expect(decoded.ActivationReady).To(BeFalse())
	}

	// per docs/adr/0097-installation-source-image-bundles.md:77
	It("refuses an empty current catalog independently of archive members", func() {
		positive()
		_, err := installationimage.Decode(context.Background(), manifest([]map[string]any{}), identity)
		Expect(err).To(HaveOccurred())
	})

	// per docs/adr/0097-installation-source-image-bundles.md:77
	It("refuses file ancestor collisions independently of filesystem publication", func() {
		positive()
		_, err := installationimage.Decode(context.Background(), manifest([]map[string]any{asset("file"), asset("file/child")}), identity)
		Expect(err).To(HaveOccurred())
	})

	// per docs/adr/0097-installation-source-image-bundles.md:79
	DescribeTable("refuses a private or noncanonical path before extraction", func(path string) {
		positive()
		_, err := installationimage.Decode(context.Background(), manifest([]map[string]any{asset(path)}), identity)
		Expect(err).To(HaveOccurred())
	}, Entry("private descendant", ".factory/private"), Entry("exact private root", ".factory"), Entry("backslash", "file\\child"))

	// per docs/adr/0097-installation-source-image-bundles.md:81
	It("permits an unrelated name sharing the private prefix", func() {
		positive()
		decoded, err := installationimage.Decode(context.Background(), manifest([]map[string]any{asset(".factory-not-private")}), identity)
		Expect(err).NotTo(HaveOccurred())
		Expect(decoded.Assets).To(HaveLen(1))
		Expect(decoded.Assets[0].Path).To(Equal(".factory-not-private"))
	})

	// Equality with the caller's request is not sufficient if that request uses
	// an unsupported identity. The CLI's separate request checks cannot protect
	// future direct consumers of this shared decoder.
	// per docs/adr/0097-installation-source-image-bundles.md:47
	// per docs/adr/0097-installation-source-image-bundles.md:69
	// per docs/adr/0060-runtime-source-bundles.md:13
	// per docs/adr/0060-runtime-source-bundles.md:19
	// per docs/adr/0060-runtime-source-bundles.md:21
	// per docs/adr/0059-deterministic-runtime-selection.md:24
	DescribeTable("refuses a matching but invalid requested image identity", func(field, value string) {
		positive()
		invalidIdentity := identity
		var data map[string]any
		Expect(json.Unmarshal(manifest([]map[string]any{asset("file"), asset("sibling")}), &data)).To(Succeed())
		switch field {
		case "version":
			invalidIdentity.Version = value
		case "target":
			invalidIdentity.Target = value
		case "source_revision":
			invalidIdentity.Revision = value
		}
		data[field] = value
		encoded, err := json.Marshal(data)
		Expect(err).NotTo(HaveOccurred())
		_, err = installationimage.Decode(context.Background(), encoded, invalidIdentity)
		Expect(err).To(HaveOccurred())
	}, Entry("empty version", "version", ""), Entry("moving version alias", "version", "latest"), Entry("case folded moving alias", "version", "HEAD"), Entry("version path", "version", "../v1"), Entry("empty commit", "source_revision", ""), Entry("uppercase commit", "source_revision", strings.Repeat("A", 40)), Entry("short commit", "source_revision", strings.Repeat("a", 39)), Entry("unsupported target", "target", "windows/amd64"))
})
