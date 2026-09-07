package acceptance_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func artifactFixture(root, cwd string) (string, string, string) {
	GinkgoHelper()
	artifactRoot := filepath.Join(cwd, "artifact root")
	Expect(os.MkdirAll(artifactRoot, 0755)).To(Succeed())
	binary := []byte("candidate runtime sentinel")
	binaryPath := filepath.Join(artifactRoot, "bin", "factory-runtime")
	writeFixture(binaryPath, binary, 0755)
	digest := sha256.Sum256(binary)
	manifest := filepath.Join(cwd, "runtime.manifest")
	contents := fmt.Sprintf("FACTORY_RUNTIME_ARTIFACT_V1\nversion\tv0.2.0\ntarget\tlinux/amd64\nbinary\tbin/factory-runtime\nsha256\t%s\nEND\n", hex.EncodeToString(digest[:]))
	writeFixture(manifest, []byte(contents), 0600)
	return manifest, artifactRoot, hex.EncodeToString(digest[:])
}

var _ = Describe("G1 runtime artifact verification candidate", func() {
	It("verifies a release-matched artifact without executing it", func() {
		root, cwd := fixture()
		manifest, artifactRoot, digest := artifactFixture(root, cwd)
		result := bridgeReader(root, cwd, nil, "runtime", "verify", manifest, artifactRoot, "linux/amd64")
		Expect(result).To(Equal(cliResult{fmt.Sprintf("v0.2.0\tlinux/amd64\tbin/factory-runtime\t%s\n", digest), "", 0}))
	})

	DescribeTable("rejects invalid artifact inputs before success", func(mutator func(string, string, string)) {
		root, cwd := fixture()
		manifest, artifactRoot, _ := artifactFixture(root, cwd)
		mutator(manifest, artifactRoot, cwd)
		result := bridgeReader(root, cwd, nil, "runtime", "verify", manifest, artifactRoot, "linux/amd64")
		Expect(result.status).To(Equal(1))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
	},
		Entry("digest mismatch", func(manifest, _, _ string) {
			writeFixture(manifest, []byte("FACTORY_RUNTIME_ARTIFACT_V1\nversion\tv0.2.0\ntarget\tlinux/amd64\nbinary\tbin/factory-runtime\nsha256\t"+strings.Repeat("0", 64)+"\nEND\n"), 0600)
		}),
		Entry("path traversal", func(manifest, artifactRoot, cwd string) {
			outside := filepath.Join(cwd, "outside")
			writeFixture(outside, []byte("outside"), 0600)
			writeFixture(manifest, []byte("FACTORY_RUNTIME_ARTIFACT_V1\nversion\tv0.2.0\ntarget\tlinux/amd64\nbinary\t../outside\nsha256\t"+strings.Repeat("0", 64)+"\nEND\n"), 0600)
		}),
		Entry("target mismatch", func(manifest, _, _ string) {
			data, err := os.ReadFile(manifest)
			Expect(err).NotTo(HaveOccurred())
			writeFixture(manifest, []byte(strings.Replace(string(data), "linux/amd64", "darwin/arm64", 1)), 0600)
		}),
		Entry("binary symlink", func(manifest, artifactRoot, _ string) {
			binaryPath := filepath.Join(artifactRoot, "bin", "factory-runtime")
			Expect(os.Remove(binaryPath)).To(Succeed())
			Expect(os.Symlink("/etc/hosts", binaryPath)).To(Succeed())
		}),
	)

	It("refuses malformed private operands with status 2", func() {
		root, cwd := fixture()
		result := bridgeReader(root, cwd, nil, "runtime", "verify")
		Expect(result.status).To(Equal(2))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
	})
})
