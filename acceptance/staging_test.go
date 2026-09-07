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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type stageEntry struct {
	name string
	body []byte
	mode int64
	kind byte
}

func stageArchive(entries []stageEntry) []byte {
	GinkgoHelper()
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		h := &tar.Header{Name: entry.name, Mode: entry.mode, Typeflag: entry.kind, Size: int64(len(entry.body))}
		if entry.kind != tar.TypeReg {
			h.Size = 0
			h.Linkname = "outside"
		}
		Expect(tw.WriteHeader(h)).To(Succeed())
		if h.Size > 0 {
			_, err := tw.Write(entry.body)
			Expect(err).NotTo(HaveOccurred())
		}
	}
	Expect(tw.Close()).To(Succeed())
	Expect(gz.Close()).To(Succeed())
	return compressed.Bytes()
}

// Independent subprocess boundary for docs/adr/0061-runtime-bundle-staging.md:79.
var _ = Describe("G1 runtime bundle staging", Ordered, ContinueOnFailure, func() {
	const version = "v1.2.3"
	const target = "linux/amd64"
	const revision = "0123456789012345678901234567890123456789"
	const prefix = version + "/" + target + "/"
	var binary, work, archive, output, marker string
	var entries []stageEntry
	BeforeAll(func() {
		build, err := os.MkdirTemp("", "factory-stage-build-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, build)
		binary = filepath.Join(build, "factory-stage")
		cmd := exec.Command("go", "build", "-o", binary, "./cmd/factory-stage") // #nosec G204 -- test-owned compiled CLI.
		cmd.Dir = ".."
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "%s", out)
	})
	BeforeEach(func() {
		var err error
		work, err = os.MkdirTemp("", "factory staging acceptance ")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, work)
		work, err = filepath.EvalSymlinks(work)
		Expect(err).NotTo(HaveOccurred())
		archive, output, marker = filepath.Join(work, "input.tar.gz"), filepath.Join(work, "output"), filepath.Join(work, "executed")
		payload := []byte("#!/bin/sh\nprintf executed > '" + marker + "'\n")
		manifest := fmt.Sprintf("FACTORY_RUNTIME_ARTIFACT_V1\nversion\t%s\ntarget\t%s\nbinary\tbin/factory-runtime\nsha256\t%x\nEND\n", version, target, sha256.Sum256(payload))
		source := fmt.Sprintf(`{"revision":%q,"version":%q,"target":%q,"go_version":"go1.27.1","kind":"source-build"}`, revision, version, target)
		entries = []stageEntry{{prefix + "bin/factory-runtime", payload, 0755, tar.TypeReg}, {prefix + "runtime.manifest", []byte(manifest), 0644, tar.TypeReg}, {prefix + "source.json", []byte(source), 0644, tar.TypeReg}}
	})
	args := func(local bool) []string {
		result := []string{"--archive", archive, "--version", version, "--revision", revision, "--target", target, "--output", output}
		if local {
			result = append(result, "--local")
		}
		return result
	}
	run := func(argv ...string) cliResult { return readerProcess(work, nil, binary, argv...) }
	write := func() { writeFixture(archive, stageArchive(entries), 0600) }
	failure := func(argv []string, status int) cliResult {
		GinkgoHelper()
		result := run(argv...)
		Expect(result.status).To(Equal(status), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
		_, err := os.Lstat(output)
		Expect(os.IsNotExist(err)).To(BeTrue(), "failure published output")
		_, err = os.Lstat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue(), "payload executed")
		return result
	}
	It("stages exact local bytes privately and reports identity without executing payload", func() {
		write()
		result := run(args(true)...)
		Expect(result.status).To(Equal(0), result.stderr)
		var summary map[string]string
		Expect(json.Unmarshal([]byte(result.stdout), &summary)).To(Succeed())
		Expect(summary).To(Equal(map[string]string{"version": version, "revision": revision, "target": target, "root": output, "sha256": fmt.Sprintf("%x", sha256.Sum256(stageArchive(entries))), "authentication": "local-source"}))
		Expect(filepath.WalkDir(output, func(path string, d os.DirEntry, err error) error {
			Expect(err).NotTo(HaveOccurred())
			info, statErr := d.Info()
			Expect(statErr).NotTo(HaveOccurred())
			mode := os.FileMode(0600)
			if d.IsDir() || d.Name() == "factory-runtime" {
				mode = 0700
			}
			Expect(info.Mode().Perm()).To(Equal(mode), path)
			return nil
		})).To(Succeed())
		for _, entry := range entries {
			body, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(entry.name)))
			Expect(err).NotTo(HaveOccurred())
			Expect(body).To(Equal(entry.body))
		}
		_, err := os.Lstat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	DescribeTable("rejects invalid requests", func(flag, value string) { write(); failure(append(args(true), flag, value), 2) },
		Entry("revision alias", "--revision", "main"), Entry("uppercase revision", "--revision", strings.Repeat("A", 40)), Entry("version traversal", "--version", "../v1"), Entry("version alias", "--version", "latest"), Entry("target", "--target", "windows/amd64"), Entry("missing archive", "--archive", ""), Entry("missing output", "--output", ""), Entry("local verifier", "--gh", "gh"), Entry("local attestation", "--attestation", "anything"), Entry("local roots", "--trusted-root", "anything"))
	It("requires explicit release attestation and roots", func() { write(); failure(args(false), 2) })
	It("accepts producer-compatible long literal version paths", func() {
		longVersion := strings.Repeat("v", 128)
		for i := range entries {
			entries[i].name = strings.Replace(entries[i].name, version, longVersion, 1)
			if i > 0 {
				entries[i].body = bytes.ReplaceAll(entries[i].body, []byte(version), []byte(longVersion))
			}
		}
		write()
		result := run(append(args(true), "--version", longVersion)...)
		Expect(result.status).To(Equal(0), result.stderr)
		body, err := os.ReadFile(filepath.Join(output, longVersion, target, "bin", "factory-runtime"))
		Expect(err).NotTo(HaveOccurred())
		Expect(body).To(Equal(entries[0].body))
	})
	DescribeTable("rejects malformed archive members", func(kind string) {
		switch kind {
		case "duplicate":
			entries = append(entries, entries[0])
		case "extra":
			entries = append(entries, stageEntry{prefix + "extra", []byte("x"), 0644, tar.TypeReg})
		case "missing":
			entries = entries[:2]
		case "traversal":
			entries[0].name = prefix + "../factory-runtime"
		case "absolute":
			entries[0].name = "/" + entries[0].name
		case "alternate":
			entries[0].name = prefix + "./bin/factory-runtime"
		case "symlink":
			entries[0].kind = tar.TypeSymlink
		case "hardlink":
			entries[0].kind = tar.TypeLink
		case "mode":
			entries[0].mode = 0777
		case "digest":
			entries[0].body = []byte("different")
		case "manifest":
			entries[1].body = []byte("invalid manifest")
		case "manifest version":
			entries[1].body = bytes.ReplaceAll(entries[1].body, []byte(version), []byte("v9.9.9"))
		case "manifest binary":
			entries[1].body = bytes.ReplaceAll(entries[1].body, []byte("bin/factory-runtime"), []byte("source.json"))
		case "source duplicate":
			entries[2].body = bytes.Replace(entries[2].body, []byte(`"kind":"source-build"`), []byte(`"kind":"source-build","kind":"source-build"`), 1)
		case "source trailing":
			entries[2].body = append(entries[2].body, []byte(" {}")...)
		case "source unknown":
			entries[2].body = bytes.Replace(entries[2].body, []byte(`"kind":"source-build"`), []byte(`"kind":"source-build","extra":"value"`), 1)
		case "source identity":
			entries[2].body = bytes.ReplaceAll(entries[2].body, []byte(revision), []byte(strings.Repeat("a", 40)))
		case "source compiler":
			entries[2].body = bytes.ReplaceAll(entries[2].body, []byte("go1.27.1"), []byte("go1.26.0"))
		case "source version":
			entries[2].body = bytes.ReplaceAll(entries[2].body, []byte(version), []byte("v9.9.9"))
		case "source target":
			entries[2].body = bytes.ReplaceAll(entries[2].body, []byte(target), []byte("darwin/arm64"))
		case "source kind":
			entries[2].body = bytes.ReplaceAll(entries[2].body, []byte("source-build"), []byte("release"))
		case "source missing":
			entries[2].body = bytes.ReplaceAll(entries[2].body, []byte(`,"kind":"source-build"`), nil)
		case "metadata bound":
			entries[2].body = bytes.Repeat([]byte(" "), 65537)
		}
		write()
		failure(args(true), 1)
	}, Entry("duplicate", "duplicate"), Entry("extra", "extra"), Entry("missing", "missing"), Entry("traversal", "traversal"), Entry("absolute", "absolute"), Entry("alternate", "alternate"), Entry("symlink", "symlink"), Entry("hardlink", "hardlink"), Entry("mode", "mode"), Entry("digest", "digest"), Entry("manifest", "manifest"), Entry("manifest version", "manifest version"), Entry("manifest binary", "manifest binary"), Entry("source duplicate", "source duplicate"), Entry("source trailing", "source trailing"), Entry("source unknown", "source unknown"), Entry("source identity", "source identity"), Entry("source compiler", "source compiler"), Entry("source version", "source version"), Entry("source target", "source target"), Entry("source kind", "source kind"), Entry("source missing", "source missing"), Entry("metadata bound", "metadata bound"))
	DescribeTable("validates the complete compressed stream", func(kind string) {
		data := stageArchive(entries)
		switch kind {
		case "truncated":
			data = data[:len(data)-4]
		case "checksum":
			data[len(data)-8] ^= 1
		case "multistream":
			data = append(data, stageArchive(entries)...)
		case "tar trailing":
			reader, err := gzip.NewReader(bytes.NewReader(data))
			Expect(err).NotTo(HaveOccurred())
			raw, err := io.ReadAll(reader)
			Expect(err).NotTo(HaveOccurred())
			Expect(reader.Close()).To(Succeed())
			raw = append(raw, []byte("nonzero")...)
			var compressed bytes.Buffer
			gz := gzip.NewWriter(&compressed)
			_, err = gz.Write(raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(gz.Close()).To(Succeed())
			data = compressed.Bytes()
		}
		writeFixture(archive, data, 0600)
		failure(args(true), 1)
	}, Entry("truncated", "truncated"), Entry("checksum", "checksum"), Entry("multistream", "multistream"), Entry("nonzero tar tail", "tar trailing"))
	It("rejects oversized binary headers before allocating or publishing", func() {
		var compressed bytes.Buffer
		gz := gzip.NewWriter(&compressed)
		tw := tar.NewWriter(gz)
		Expect(tw.WriteHeader(&tar.Header{Name: prefix + "bin/factory-runtime", Mode: 0755, Typeflag: tar.TypeReg, Size: 256*1024*1024 + 1})).To(Succeed())
		// Intentionally omit the enormous body; the header alone must be refused.
		Expect(tw.Flush()).To(MatchError(ContainSubstring("missed writing")))
		Expect(gz.Close()).To(Succeed())
		writeFixture(archive, compressed.Bytes(), 0600)
		result := failure(args(true), 1)
		Expect(result.stderr).To(ContainSubstring("size"))
	})
	DescribeTable("preserves existing output paths", func(suffix string) {
		write()
		existing := filepath.Join(work, "existing")
		writeFixture(filepath.Join(existing, "sentinel"), []byte("preserve"), 0600)
		if suffix == "directory" {
			output = existing
		} else {
			output = filepath.Join(work, "link")
			Expect(os.Symlink(existing, output)).To(Succeed())
			output += suffix
		}
		result := run(args(true)...)
		Expect(result.status).To(Equal(1), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		files, err := os.ReadDir(existing)
		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(HaveLen(1))
		body, err := os.ReadFile(filepath.Join(existing, "sentinel"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal("preserve"))
	}, Entry("directory", "directory"), Entry("symlink", ""), Entry("symlink slash", "/"), Entry("symlink dot", "/."))
	It("refuses archive symlinks", func() {
		write()
		link := filepath.Join(work, "linked.tar.gz")
		Expect(os.Symlink(archive, link)).To(Succeed())
		archive = link
		failure(args(true), 1)
	})
	It("preserves interior symlink parent semantics for archive and output", func() {
		write()
		actual := filepath.Join(work, "parent", "child")
		Expect(os.MkdirAll(actual, 0700)).To(Succeed())
		Expect(os.Symlink(actual, filepath.Join(work, "alias"))).To(Succeed())
		Expect(os.Rename(archive, filepath.Join(work, "parent", "input.tar.gz"))).To(Succeed())
		archive = work + "/alias/../input.tar.gz"
		output = work + "/alias/../staged"
		result := run(args(true)...)
		Expect(result.status).To(Equal(0), result.stderr)
		_, err := os.Stat(filepath.Join(work, "parent", "staged", filepath.FromSlash(prefix), "source.json"))
		Expect(err).NotTo(HaveOccurred())
		_, err = os.Lstat(filepath.Join(work, "staged"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	It("binds offline verification to private snapshots and stages those same bytes", func() {
		write()
		attestation, roots, verifier, log := filepath.Join(work, "attestation"), filepath.Join(work, "roots"), filepath.Join(work, "gh"), filepath.Join(work, "argv")
		writeFixture(attestation, []byte("bundle fixture"), 0600)
		writeFixture(roots, []byte("trusted fixture"), 0600)
		writeFixture(verifier, []byte("#!/bin/sh\nset -eu\nprintf '%s\\n' \"$@\" > '"+log+"'\nprintf '%s' \"$GH_HOST\" > '"+log+".host'\ncp \"$3\" '"+log+".archive'\nfind \"$3\" -type f -perm 600 > '"+log+".private'\nfind \"$(dirname \"$3\")\" -maxdepth 0 -type d -perm 700 >> '"+log+".private'\nfor arg do\n case \"$arg\" in --bundle|--custom-trusted-root) previous=\"$arg\";; *) case \"${previous-}\" in --bundle) cp \"$arg\" '"+log+".bundle'; find \"$arg\" -type f -perm 600 >> '"+log+".private';; --custom-trusted-root) cp \"$arg\" '"+log+".roots'; find \"$arg\" -type f -perm 600 >> '"+log+".private';; esac; previous=;; esac\ndone\nprintf replaced > '"+archive+"'\n"), 0755)
		result := readerProcess(work, map[string]string{"GH_HOST": "untrusted.example"}, binary, append(args(false), "--attestation", attestation, "--trusted-root", roots, "--gh", verifier)...)
		Expect(result.status).To(Equal(0), result.stderr)
		var summary map[string]string
		Expect(json.Unmarshal([]byte(result.stdout), &summary)).To(Succeed())
		Expect(summary["authentication"]).To(Equal("github-attestation"))
		data, err := os.ReadFile(log)
		Expect(err).NotTo(HaveOccurred())
		argv := strings.Split(strings.TrimSpace(string(data)), "\n")
		Expect(argv[:2]).To(Equal([]string{"attestation", "verify"}))
		Expect(argv[2]).NotTo(Equal(archive))
		pairs := map[string]string{}
		for i := 3; i < len(argv); i++ {
			if argv[i] == "--deny-self-hosted-runners" {
				pairs[argv[i]] = ""
			} else {
				Expect(i + 1).To(BeNumerically("<", len(argv)))
				pairs[argv[i]] = argv[i+1]
				i++
			}
		}
		Expect(pairs).To(HaveKeyWithValue("--repo", "anoop2811/software-factory-template"))
		Expect(pairs).To(HaveKeyWithValue("--signer-workflow", "anoop2811/software-factory-template/.github/workflows/runtime-release.yml"))
		Expect(pairs).To(HaveKeyWithValue("--source-digest", revision))
		Expect(pairs).To(HaveKeyWithValue("--source-ref", "refs/tags/"+version))
		Expect(pairs).To(HaveKey("--deny-self-hosted-runners"))
		Expect(pairs["--bundle"]).NotTo(Equal(attestation))
		Expect(pairs["--custom-trusted-root"]).NotTo(Equal(roots))
		host, err := os.ReadFile(log + ".host")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(host)).To(Equal("github.com"))
		snapshot, err := os.ReadFile(log + ".archive")
		Expect(err).NotTo(HaveOccurred())
		Expect(snapshot).To(Equal(stageArchive(entries)))
		for suffix, expected := range map[string]string{".bundle": "bundle fixture", ".roots": "trusted fixture"} {
			body, readErr := os.ReadFile(log + suffix)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal(expected))
		}
		private, err := os.ReadFile(log + ".private")
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Split(strings.TrimSpace(string(private)), "\n")).To(HaveLen(4))
		_, err = os.Lstat(argv[2])
		Expect(os.IsNotExist(err)).To(BeTrue(), "snapshot must be cleaned after staging")
	})
	DescribeTable("refuses symlink verification inputs before calling the verifier", func(flag string) {
		write()
		attestation, roots, verifier := filepath.Join(work, "attestation"), filepath.Join(work, "roots"), filepath.Join(work, "gh")
		writeFixture(attestation, []byte("bundle"), 0600)
		writeFixture(roots, []byte("roots"), 0600)
		writeFixture(verifier, []byte("#!/bin/sh\nprintf called > '"+marker+"'\n"), 0755)
		original := attestation
		if flag == "--trusted-root" {
			original = roots
		}
		link := filepath.Join(work, "linked-input")
		Expect(os.Symlink(original, link)).To(Succeed())
		failure(append(args(false), "--attestation", attestation, "--trusted-root", roots, "--gh", verifier, flag, link), 1)
	}, Entry("attestation", "--attestation"), Entry("roots", "--trusted-root"))
	It("runs verification before interpreting even malformed archive bytes", func() {
		writeFixture(archive, []byte("not gzip"), 0600)
		attestation, roots, verifier := filepath.Join(work, "attestation"), filepath.Join(work, "roots"), filepath.Join(work, "gh")
		writeFixture(attestation, []byte("bundle"), 0600)
		writeFixture(roots, []byte("roots"), 0600)
		writeFixture(verifier, []byte("#!/bin/sh\nprintf called > '"+filepath.Join(work, "called")+"'\nprintf 'verification refused' >&2\nexit 9\n"), 0755)
		result := failure(append(args(false), "--attestation", attestation, "--trusted-root", roots, "--gh", verifier), 1)
		Expect(result.stderr).To(ContainSubstring("verification refused"))
		_, err := os.Stat(filepath.Join(work, "called"))
		Expect(err).NotTo(HaveOccurred())
	})
	It("bounds verifier execution by its thirty-second deadline", func() {
		write()
		attestation, roots, verifier := filepath.Join(work, "attestation"), filepath.Join(work, "roots"), filepath.Join(work, "gh")
		called := filepath.Join(work, "called")
		writeFixture(attestation, []byte("bundle"), 0600)
		writeFixture(roots, []byte("roots"), 0600)
		writeFixture(verifier, []byte("#!/bin/sh\nprintf called > '"+called+"'\nexec sleep 60\n"), 0755)
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, append(args(false), "--attestation", attestation, "--trusted-root", roots, "--gh", verifier)...) // #nosec G204 -- isolated acceptance binary and verifier.
		cmd.Dir = work
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		start := time.Now()
		err := cmd.Run()
		Expect(ctx.Err()).NotTo(HaveOccurred())
		Expect(time.Since(start)).To(BeNumerically("<", 40*time.Second))
		var exit *exec.ExitError
		Expect(errors.As(err, &exit)).To(BeTrue())
		Expect(exit.ExitCode()).To(Equal(1), stderr.String())
		Expect(stdout.String()).To(BeEmpty())
		Expect(stderr.String()).To(ContainSubstring("deadline"))
		_, err = os.Stat(called)
		Expect(err).NotTo(HaveOccurred())
		_, err = os.Lstat(output)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})
