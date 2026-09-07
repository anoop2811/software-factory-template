package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Independent fixture grammar per docs/adr/0059-deterministic-runtime-selection.md:14.
func selectionSlot(store, slotVersion, slotTarget, manifestVersion, manifestTarget string) (string, string) {
	GinkgoHelper()
	slot := filepath.Join(store, slotVersion, filepath.FromSlash(slotTarget))
	binary := []byte("#!/bin/sh\nprintf executed > selection-executed\nexit 93\n")
	digest := fmt.Sprintf("%x", sha256.Sum256(binary))
	writeFixture(filepath.Join(slot, "bin", "factory-runtime"), binary, 0755)
	manifest := fmt.Sprintf("FACTORY_RUNTIME_ARTIFACT_V1\nversion\t%s\ntarget\t%s\nbinary\tbin/factory-runtime\nsha256\t%s\nEND\n", manifestVersion, manifestTarget, digest)
	writeFixture(filepath.Join(slot, "runtime.manifest"), []byte(manifest), 0600)
	return slot, digest
}

func selectionSnapshot(store string) map[string]string {
	GinkgoHelper()
	storeRoot, err := os.OpenRoot(store)
	Expect(err).NotTo(HaveOccurred())
	defer func() { Expect(storeRoot.Close()).To(Succeed()) }()
	result := map[string]string{}
	Expect(filepath.WalkDir(store, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := fmt.Sprintf("%s:%d", info.Mode(), info.ModTime().UnixNano())
		if info.Mode().IsRegular() {
			relative, relErr := filepath.Rel(store, path)
			if relErr != nil {
				return relErr
			}
			contents, readErr := storeRoot.ReadFile(relative)
			if readErr != nil {
				return readErr
			}
			value += fmt.Sprintf(":%x", sha256.Sum256(contents))
		}
		result[path] = value
		return nil
	})).To(Succeed())
	return result
}

var _ = Describe("G1 deterministic runtime selection candidate", func() {
	It("rejects binary traversal after a symlink component", func() {
		root, cwd := fixture()
		store := filepath.Join(cwd, "runtime store")
		slot := filepath.Join(store, "v1.0.0", "linux", "amd64")
		Expect(os.MkdirAll(filepath.Join(slot, "sub", "deep"), 0755)).To(Succeed())
		Expect(os.Symlink("sub/deep", filepath.Join(slot, "a"))).To(Succeed())
		contents := []byte("the binary whose digest is declared")
		writeFixture(filepath.Join(slot, "sub", "b"), contents, 0600)
		writeFixture(filepath.Join(slot, "b"), []byte("a different binary after lexical cleaning"), 0600)
		digest := fmt.Sprintf("%x", sha256.Sum256(contents))
		manifest := "FACTORY_RUNTIME_ARTIFACT_V1\nversion\tv1.0.0\ntarget\tlinux/amd64\nbinary\ta/../b\nsha256\t" + digest + "\nEND\n"
		writeFixture(filepath.Join(slot, "runtime.manifest"), []byte(manifest), 0600)
		result := bridgeReader(root, cwd, nil, "runtime", "resolve", store, "v1.0.0", "linux/amd64")
		Expect(result.status).To(Equal(2))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
	})

	It("preserves a verified binary path containing a dot component", func() {
		root, cwd := fixture()
		store := filepath.Join(cwd, "runtime store")
		slot, digest := selectionSlot(store, "v1.0.0", "linux/amd64", "v1.0.0", "linux/amd64")
		manifest := "FACTORY_RUNTIME_ARTIFACT_V1\nversion\tv1.0.0\ntarget\tlinux/amd64\nbinary\tbin/./factory-runtime\nsha256\t" + digest + "\nEND\n"
		writeFixture(filepath.Join(slot, "runtime.manifest"), []byte(manifest), 0600)
		result := bridgeReader(root, cwd, nil, "runtime", "resolve", store, "v1.0.0", "linux/amd64")
		Expect(result).To(Equal(cliResult{"v1.0.0\tlinux/amd64\tv1.0.0/linux/amd64/bin/./factory-runtime\t" + digest + "\n", "", 0}))
	})

	It("resolves the physical store when an interior symlink precedes a parent component", func() {
		root, cwd := fixture()
		actual := filepath.Join(cwd, "actual")
		Expect(os.MkdirAll(filepath.Join(actual, "child"), 0755)).To(Succeed())
		link := filepath.Join(cwd, "prefix-link")
		Expect(os.Symlink(filepath.Join(actual, "child"), link)).To(Succeed())
		_, digest := selectionSlot(actual, "v1.0.0", "linux/amd64", "v1.0.0", "linux/amd64")
		// A lexically cleaned STORE reaches this incompatible decoy slot.
		selectionSlot(cwd, "v1.0.0", "linux/amd64", "v99.0.0", "linux/amd64")
		result := bridgeReader(root, cwd, nil, "runtime", "resolve", link+"/..", "v1.0.0", "linux/amd64")
		Expect(result).To(Equal(cliResult{"v1.0.0\tlinux/amd64\tv1.0.0/linux/amd64/bin/factory-runtime\t" + digest + "\n", "", 0}))
	})

	It("reports output write failures as operational failures", func() {
		if runtime.GOOS != "linux" {
			Skip("/dev/full is available on Linux")
		}
		root, cwd := fixture()
		store := filepath.Join(cwd, "runtime store")
		selectionSlot(store, "v1.0.0", "linux/amd64", "v1.0.0", "linux/amd64")
		full, err := os.OpenFile("/dev/full", os.O_WRONLY, 0)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(full.Close)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, filepath.Join(root, "factory"), "runtime", "resolve", store, "v1.0.0", "linux/amd64") // #nosec G204 -- test-owned executable and isolated fixture operands.
		cmd.Dir = cwd
		cmd.Env = append(os.Environ(), "FACTORY_BRIDGE_PROTOCOL=1")
		cmd.Stdout = full
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err = cmd.Run()
		Expect(ctx.Err()).NotTo(HaveOccurred())
		var exitError *exec.ExitError
		Expect(errors.As(err, &exitError)).To(BeTrue())
		Expect(exitError.ExitCode()).To(Equal(1))
		Expect(stderr.String()).NotTo(BeEmpty())
	})

	It("selects exactly the explicit version and foreign target without execution or mutation", func() {
		root, cwd := fixture()
		store := filepath.Join(cwd, "runtime store with spaces")
		target := "linux/amd64"
		if runtime.GOOS+"/"+runtime.GOARCH == target {
			target = "darwin/arm64"
		}
		_, digest := selectionSlot(store, "local-abc123", target, "local-abc123", target)
		selectionSlot(store, "v99.0.0", target, "v99.0.0", target)
		before := selectionSnapshot(store)
		result := bridgeReader(root, cwd, nil, "runtime", "resolve", store, "local-abc123", target)
		Expect(result).To(Equal(cliResult{fmt.Sprintf("local-abc123\t%s\tlocal-abc123/%s/bin/factory-runtime\t%s\n", target, target, digest), "", 0}))
		Expect(selectionSnapshot(store)).To(Equal(before))
		Expect(filepath.Join(cwd, "selection-executed")).NotTo(BeAnExistingFile())
	})

	DescribeTable("refuses an unavailable or invalid selected slot even when another version is valid", func(change func(string, string)) {
		root, cwd := fixture()
		store := filepath.Join(cwd, "runtime store")
		slot, _ := selectionSlot(store, "v1.0.0", "linux/amd64", "v1.0.0", "linux/amd64")
		selectionSlot(store, "v2.0.0", "linux/amd64", "v2.0.0", "linux/amd64")
		change(store, slot)
		before := selectionSnapshot(store)
		result := bridgeReader(root, cwd, nil, "runtime", "resolve", store, "v1.0.0", "linux/amd64")
		Expect(result.status).To(Equal(1))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
		Expect(selectionSnapshot(store)).To(Equal(before))
		Expect(filepath.Join(cwd, "selection-executed")).NotTo(BeAnExistingFile())
	},
		Entry("missing slot", func(_, slot string) { Expect(os.RemoveAll(slot)).To(Succeed()) }),
		Entry("missing manifest", func(_, slot string) { Expect(os.Remove(filepath.Join(slot, "runtime.manifest"))).To(Succeed()) }),
		Entry("missing binary", func(_, slot string) { Expect(os.Remove(filepath.Join(slot, "bin", "factory-runtime"))).To(Succeed()) }),
		Entry("corrupt binary", func(_, slot string) {
			writeFixture(filepath.Join(slot, "bin", "factory-runtime"), []byte("corrupt"), 0755)
		}),
		Entry("manifest version mismatch", func(store, _ string) { selectionSlot(store, "v1.0.0", "linux/amd64", "v2.0.0", "linux/amd64") }),
		Entry("manifest target mismatch", func(store, _ string) { selectionSlot(store, "v1.0.0", "linux/amd64", "v1.0.0", "darwin/arm64") }),
	)

	DescribeTable("rejects symlink directories", func(component string) {
		root, cwd := fixture()
		store := filepath.Join(cwd, "runtime store")
		selectionSlot(store, "v1.0.0", "linux/amd64", "v1.0.0", "linux/amd64")
		path := filepath.Join(store, filepath.FromSlash(component))
		relocated := filepath.Join(cwd, "relocated")
		Expect(os.Rename(path, relocated)).To(Succeed())
		Expect(os.Symlink(relocated, path)).To(Succeed())
		result := bridgeReader(root, cwd, nil, "runtime", "resolve", store, "v1.0.0", "linux/amd64")
		Expect(result.status).To(Equal(1))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
	},
		Entry("store", ""),
		Entry("version", "v1.0.0"),
		Entry("GOOS", "v1.0.0/linux"),
		Entry("GOARCH", "v1.0.0/linux/amd64"),
	)

	DescribeTable("accepts a real store with literal path suffixes", func(suffix string) {
		root, cwd := fixture()
		store := filepath.Join(cwd, "real runtime store")
		_, digest := selectionSlot(store, "v1.0.0", "linux/amd64", "v1.0.0", "linux/amd64")
		result := bridgeReader(root, cwd, nil, "runtime", "resolve", store+suffix, "v1.0.0", "linux/amd64")
		Expect(result).To(Equal(cliResult{"v1.0.0\tlinux/amd64\tv1.0.0/linux/amd64/bin/factory-runtime\t" + digest + "\n", "", 0}))
	},
		Entry("trailing slash", "/"),
		Entry("trailing dot component", "/."),
	)

	DescribeTable("rejects a symlink store with literal path suffixes", func(suffix string) {
		root, cwd := fixture()
		store := filepath.Join(cwd, "real runtime store")
		selectionSlot(store, "v1.0.0", "linux/amd64", "v1.0.0", "linux/amd64")
		link := filepath.Join(cwd, "store-link")
		Expect(os.Symlink(store, link)).To(Succeed())
		// Keep the caller's spelling: filepath.Join would erase the bypass.
		result := bridgeReader(root, cwd, nil, "runtime", "resolve", link+suffix, "v1.0.0", "linux/amd64")
		Expect(result.status).To(Equal(1))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
	},
		Entry("trailing slash", "/"),
		Entry("trailing dot component", "/."),
	)

	DescribeTable("rejects malformed versions", func(version string) {
		root, cwd := fixture()
		result := bridgeReader(root, cwd, nil, "runtime", "resolve", filepath.Join(cwd, "store"), version, "linux/amd64")
		Expect(result.status).To(Equal(2))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
	},
		Entry("empty", ""), Entry("traversal", "../v1"), Entry("absolute", "/v1"),
		Entry("dot", "."), Entry("leading dot", ".v1"), Entry("backslash", `v1\other`),
		Entry("tab", "v1\tother"), Entry("non-ASCII", "v日本"), Entry("too long", strings.Repeat("v", 129)),
		Entry("latest", "latest"), Entry("current", "CURRENT"), Entry("main", "MaIn"), Entry("HEAD", "HEAD"),
	)

	DescribeTable("rejects malformed request operands", func(args []string) {
		root, cwd := fixture()
		result := bridgeReader(root, cwd, nil, append([]string{"runtime", "resolve"}, args...)...)
		Expect(result.status).To(Equal(2))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
	},
		Entry("missing", []string{}),
		Entry("extra", []string{"store", "v1", "linux/amd64", "extra"}),
		Entry("empty store", []string{"", "v1", "linux/amd64"}),
		Entry("invalid target", []string{"store", "v1", "linux"}),
		Entry("target traversal", []string{"store", "v1", "../amd64"}),
	)
})
