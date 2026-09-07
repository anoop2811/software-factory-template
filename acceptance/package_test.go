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
	"runtime"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// External CLI oracle for the packaging contract in docs/adr/0060-runtime-source-bundles.md:68.
var _ = Describe("G1 committed-source runtime packaging", Ordered, ContinueOnFailure, func() {
	var packager, source, revision, target, work string
	var sequence int
	run := func(program, dir string, env []string, args ...string) cliResult {
		GinkgoHelper()
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, program, args...) // #nosec G204 -- test-owned executable and literal acceptance operands.
		cmd.Dir, cmd.Env = dir, env
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		Expect(ctx.Err()).NotTo(HaveOccurred(), "subprocess timed out: %s", stderr.String())
		status := 0
		if err != nil {
			var exit *exec.ExitError
			Expect(errors.As(err, &exit)).To(BeTrue(), "could not execute subprocess: %v", err)
			status = exit.ExitCode()
		}
		return cliResult{stdout.String(), stderr.String(), status}
	}
	BeforeAll(func() {
		var err error
		work, err = os.MkdirTemp("", "factory package acceptance ")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, work)
		work, err = filepath.EvalSymlinks(work)
		Expect(err).NotTo(HaveOccurred())
		root, err := filepath.Abs("..")
		Expect(err).NotTo(HaveOccurred())
		packager = filepath.Join(work, "factory-package")
		result := run("go", root, nil, "build", "-o", packager, "./cmd/factory-package")
		Expect(result.status).To(Equal(0), result.stderr)
		target = runtime.GOOS + "/" + runtime.GOARCH
	})
	BeforeEach(func() {
		sequence++
		source = filepath.Join(work, fmt.Sprintf("source-%d", sequence))
		writeFixture(filepath.Join(source, "go.mod"), []byte("module example.com/package-fixture\n\ngo 1.27.1\n"), 0600)
		writeFixture(filepath.Join(source, "cmd", "factory", "main.go"), []byte("package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"committed fixture\") }\n"), 0600)
		for _, args := range [][]string{{"init", "--quiet"}, {"add", "."}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.com", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "fixture"}} {
			result := run("git", source, nil, args...)
			Expect(result.status).To(Equal(0), result.stderr)
		}
		result := run("git", source, nil, "rev-parse", "HEAD")
		Expect(result.status).To(Equal(0), result.stderr)
		revision = strings.TrimSpace(result.stdout)
	})
	argsFor := func(output string) []string {
		return []string{"--source", source, "--revision", revision, "--target", target, "--output", output}
	}
	outputFor := func(name string) string { return filepath.Join(source, name) }
	assertFailure := func(args []string, status int) {
		GinkgoHelper()
		result := run(packager, source, nil, args...)
		Expect(result.status).To(Equal(status), result.stderr)
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
	}
	It("publishes a native bundle with exact metadata, archive files, digest and executable modes", func() {
		out := outputFor("bundle")
		result := run(packager, source, nil, argsFor(out)...)
		Expect(result.status).To(Equal(0), result.stderr)
		var summary map[string]string
		Expect(json.Unmarshal([]byte(result.stdout), &summary)).To(Succeed())
		version := "local-" + revision
		Expect(summary).To(HaveKeyWithValue("version", version))
		Expect(summary).To(HaveKeyWithValue("revision", revision))
		Expect(summary).To(HaveKeyWithValue("target", target))
		archivePath := filepath.Join(out, "factory-runtime.tar.gz")
		Expect(summary).To(HaveKeyWithValue("archive", archivePath))
		archiveBytes, err := os.ReadFile(archivePath)
		Expect(err).NotTo(HaveOccurred())
		digest := fmt.Sprintf("%x", sha256.Sum256(archiveBytes))
		Expect(summary).To(HaveKeyWithValue("sha256", digest))
		checksums, err := os.ReadFile(filepath.Join(out, "SHA256SUMS"))
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Fields(string(checksums))).To(Equal([]string{digest, "factory-runtime.tar.gz"}))
		gz, err := gzip.NewReader(bytes.NewReader(archiveBytes))
		Expect(err).NotTo(HaveOccurred())
		defer gz.Close()
		reader := tar.NewReader(gz)
		prefix := version + "/" + target + "/"
		files := map[string][]byte{}
		for {
			header, readErr := reader.Next()
			if errors.Is(readErr, io.EOF) {
				break
			}
			Expect(readErr).NotTo(HaveOccurred())
			Expect(header.Typeflag).To(Equal(byte(tar.TypeReg)))
			Expect(files).NotTo(HaveKey(header.Name))
			Expect(header.Uid).To(BeZero())
			Expect(header.Gid).To(BeZero())
			mode := int64(0644)
			if strings.HasSuffix(header.Name, "/bin/factory-runtime") {
				mode = 0755
			}
			Expect(header.Mode).To(Equal(mode))
			contents, readErr := io.ReadAll(reader)
			Expect(readErr).NotTo(HaveOccurred())
			files[header.Name] = contents
		}
		Expect(files).To(HaveLen(3))
		Expect(files).To(HaveKey(prefix + "bin/factory-runtime"))
		Expect(files).To(HaveKey(prefix + "runtime.manifest"))
		Expect(files).To(HaveKey(prefix + "source.json"))
		binaryDigest := fmt.Sprintf("%x", sha256.Sum256(files[prefix+"bin/factory-runtime"]))
		Expect(string(files[prefix+"runtime.manifest"])).To(Equal(fmt.Sprintf("FACTORY_RUNTIME_ARTIFACT_V1\nversion\t%s\ntarget\t%s\nbinary\tbin/factory-runtime\nsha256\t%s\nEND\n", version, target, binaryDigest)))
		var metadata map[string]string
		Expect(json.Unmarshal(files[prefix+"source.json"], &metadata)).To(Succeed())
		Expect(metadata).To(Equal(map[string]string{"revision": revision, "version": version, "target": target, "go_version": "go1.27.1", "kind": "source-build"}))
		for name, contents := range files {
			onDisk, readErr := os.ReadFile(filepath.Join(out, "store", filepath.FromSlash(name)))
			Expect(readErr).NotTo(HaveOccurred())
			Expect(onDisk).To(Equal(contents))
		}
		binary := filepath.Join(out, "store", filepath.FromSlash(prefix), "bin", "factory-runtime")
		info, err := os.Stat(binary)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0755)))
		result = run(binary, out, []string{"PATH=" + filepath.Join(work, "empty-path")})
		Expect(result).To(Equal(cliResult{"committed fixture\n", "", 0}))
	})
	It("produces identical archives despite working-tree edits and ambient Go build settings", func() {
		first, second := outputFor("first"), outputFor("second")
		args := append(argsFor(first), "--version", "v1.2.3")
		result := run(packager, source, nil, args...)
		Expect(result.status).To(Equal(0), result.stderr)
		writeFixture(filepath.Join(source, "cmd", "factory", "main.go"), []byte("not valid Go"), 0600)
		writeFixture(filepath.Join(source, "cmd", "factory", "untracked.go"), []byte("not valid Go either"), 0600)
		overrides := map[string]string{
			"GOFLAGS": "-not-a-go-build-flag", "GOWORK": filepath.Join(source, "absent.work"),
			"GOEXPERIMENT": "not-a-go-experiment", "GOTOOLCHAIN": "unavailable-toolchain",
			"CGO_ENABLED": "1", "GOOS": "windows", "GOARCH": "386",
			"GOAMD64": "v4", "GOARM64": "v9.5", "GOPROXY": "http://127.0.0.1:1", "GOSUMDB": "invalid",
		}
		var env []string
		for _, entry := range os.Environ() {
			if _, replaced := overrides[strings.SplitN(entry, "=", 2)[0]]; !replaced {
				env = append(env, entry)
			}
		}
		for name, value := range overrides {
			env = append(env, name+"="+value)
		}
		result = run(packager, source, env, append(argsFor(second), "--version", "v1.2.3")...)
		Expect(result.status).To(Equal(0), result.stderr)
		one, err := os.ReadFile(filepath.Join(first, "factory-runtime.tar.gz"))
		Expect(err).NotTo(HaveOccurred())
		two, err := os.ReadFile(filepath.Join(second, "factory-runtime.tar.gz"))
		Expect(err).NotTo(HaveOccurred())
		Expect(two).To(Equal(one))
	})
	It("ignores replacement refs and export attributes when building committed bytes", func() {
		writeFixture(filepath.Join(source, "cmd", "factory", "main.go"), []byte("package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"$Format:%H$\") }\n"), 0600)
		writeFixture(filepath.Join(source, ".gitattributes"), []byte("cmd export-ignore\ncmd/factory/*.go export-subst\n"), 0600)
		for _, args := range [][]string{{"add", "."}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.com", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "literal source"}} {
			result := run("git", source, nil, args...)
			Expect(result.status).To(Equal(0), result.stderr)
		}
		result := run("git", source, nil, "rev-parse", "HEAD")
		Expect(result.status).To(Equal(0), result.stderr)
		revision = strings.TrimSpace(result.stdout)
		writeFixture(filepath.Join(source, "cmd", "factory", "main.go"), []byte("package main\nfunc main() {}\n"), 0600)
		result = run("git", source, nil, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.com", "-c", "commit.gpgsign=false", "commit", "--quiet", "-am", "replacement source")
		Expect(result.status).To(Equal(0), result.stderr)
		result = run("git", source, nil, "replace", revision, "HEAD")
		Expect(result.status).To(Equal(0), result.stderr)
		writeFixture(filepath.Join(source, ".git", "info", "attributes"), []byte("* export-ignore\n"), 0600)
		out := outputFor("isolated-git")
		result = run(packager, source, nil, argsFor(out)...)
		Expect(result.status).To(Equal(0), result.stderr)
		binary := filepath.Join(out, "store", "local-"+revision, filepath.FromSlash(target), "bin", "factory-runtime")
		result = run(binary, out, []string{"PATH=" + filepath.Join(work, "absent-tools")})
		Expect(result).To(Equal(cliResult{"$Format:%H$\n", "", 0}))
	})
	DescribeTable("rejects invalid operands", func(flag, value string) {
		args := argsFor(outputFor("invalid"))
		if flag == "positional" {
			args = append(args, value)
		} else {
			args = append(args, flag, value)
		}
		assertFailure(args, 2)
	},
		Entry("branch revision", "--revision", "main"),
		Entry("uppercase revision", "--revision", strings.Repeat("A", 40)),
		Entry("unsupported target", "--target", "windows/amd64"),
		Entry("traversal version", "--version", "../bad"),
		Entry("missing source", "--source", ""),
		Entry("unexpected operand", "positional", "extra"),
	)
	It("refuses an absent commit without publishing output", func() {
		out := outputFor("absent")
		assertFailure(append(argsFor(out), "--revision", strings.Repeat("0", 40)), 1)
		_, err := os.Lstat(out)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	It("refuses a full tree object identity because a committed revision is required", func() {
		result := run("git", source, nil, "rev-parse", "HEAD^{tree}")
		Expect(result.status).To(Equal(0), result.stderr)
		out := outputFor("tree-object")
		assertFailure(append(argsFor(out), "--revision", strings.TrimSpace(result.stdout)), 1)
		_, err := os.Lstat(out)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	It("preserves an existing output directory and its contents", func() {
		out := outputFor("existing")
		writeFixture(filepath.Join(out, "sentinel"), []byte("preserve"), 0600)
		assertFailure(argsFor(out), 1)
		entries, err := os.ReadDir(out)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		contents, err := os.ReadFile(filepath.Join(out, "sentinel"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(contents)).To(Equal("preserve"))
	})
	It("preserves filesystem parent semantics through an interior symlink", func() {
		actual := filepath.Join(source, "parent", "actual")
		Expect(os.MkdirAll(actual, 0700)).To(Succeed())
		Expect(os.Symlink(actual, filepath.Join(source, "alias"))).To(Succeed())
		out := source + "/alias/../bundle"
		result := run(packager, source, nil, argsFor(out)...)
		Expect(result.status).To(Equal(0), result.stderr)
		_, err := os.Stat(filepath.Join(source, "parent", "bundle", "factory-runtime.tar.gz"))
		Expect(err).NotTo(HaveOccurred())
		_, err = os.Lstat(filepath.Join(source, "bundle"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	It("refuses an existing output symlink without touching its target", func() {
		out := outputFor("link")
		Expect(os.Symlink("missing-target", out)).To(Succeed())
		assertFailure(argsFor(out), 1)
		value, err := os.Readlink(out)
		Expect(err).NotTo(HaveOccurred())
		Expect(value).To(Equal("missing-target"))
	})
	It("reports a missing explicit compiler without output", func() {
		out := outputFor("compiler-failure")
		assertFailure(append(argsFor(out), "--go", filepath.Join(work, "missing-go")), 1)
		_, err := os.Lstat(out)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	It("preserves filesystem parent semantics for an explicit compiler", func() {
		compiler, err := exec.LookPath("go")
		Expect(err).NotTo(HaveOccurred())
		compiler, err = filepath.Abs(compiler)
		Expect(err).NotTo(HaveOccurred())
		actual := filepath.Join(source, "toolchain", "child")
		Expect(os.MkdirAll(actual, 0700)).To(Succeed())
		Expect(os.Symlink(actual, filepath.Join(source, "tool-alias"))).To(Succeed())
		Expect(os.Symlink(compiler, filepath.Join(source, "toolchain", "go"))).To(Succeed())
		out := outputFor("explicit-compiler")
		result := run(packager, source, nil, append(argsFor(out), "--go", source+"/tool-alias/../go")...)
		Expect(result.status).To(Equal(0), result.stderr)
	})
	It("refuses a compiler whose version differs from the pin", func() {
		compiler := filepath.Join(source, "wrong-go")
		writeFixture(compiler, []byte("#!/bin/sh\nprintf 'go version go1.26.0 linux/amd64\\n'\n"), 0755)
		out := outputFor("wrong-compiler")
		assertFailure(append(argsFor(out), "--go", compiler), 1)
		_, err := os.Lstat(out)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	It("rejects symlinks from the committed source archive", func() {
		Expect(os.Symlink("main.go", filepath.Join(source, "cmd", "factory", "link.go"))).To(Succeed())
		result := run("git", source, nil, "add", ".")
		Expect(result.status).To(Equal(0), result.stderr)
		result = run("git", source, nil, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.com", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "symlink fixture")
		Expect(result.status).To(Equal(0), result.stderr)
		result = run("git", source, nil, "rev-parse", "HEAD")
		Expect(result.status).To(Equal(0), result.stderr)
		revision = strings.TrimSpace(result.stdout)
		out := outputFor("source-link")
		assertFailure(argsFor(out), 1)
		_, err := os.Lstat(out)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	It("rejects filesystem module replacements instead of importing ambient source", func() {
		dependency := filepath.Join(work, "ambient-dependency")
		writeFixture(filepath.Join(dependency, "go.mod"), []byte("module example.com/ambient\n\ngo 1.27.1\n"), 0600)
		writeFixture(filepath.Join(dependency, "value.go"), []byte("package ambient\nconst Value = \"outside commit\"\n"), 0600)
		writeFixture(filepath.Join(source, "go.mod"), []byte(fmt.Sprintf("module example.com/package-fixture\n\ngo 1.27.1\n\nrequire example.com/ambient v0.0.0\nreplace example.com/ambient => %q\n", dependency)), 0600)
		writeFixture(filepath.Join(source, "cmd", "factory", "main.go"), []byte("package main\nimport (\"fmt\"; \"example.com/ambient\")\nfunc main() { fmt.Println(ambient.Value) }\n"), 0600)
		result := run("git", source, nil, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.com", "-c", "commit.gpgsign=false", "commit", "--quiet", "-am", "ambient dependency")
		Expect(result.status).To(Equal(0), result.stderr)
		result = run("git", source, nil, "rev-parse", "HEAD")
		Expect(result.status).To(Equal(0), result.stderr)
		revision = strings.TrimSpace(result.stdout)
		out := outputFor("ambient-build")
		assertFailure(argsFor(out), 1)
		_, err := os.Lstat(out)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	It("reports committed compilation failures and leaves no output", func() {
		writeFixture(filepath.Join(source, "cmd", "factory", "main.go"), []byte("package main\nfunc main() { undefinedFixtureSymbol() }\n"), 0600)
		result := run("git", source, nil, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.com", "-c", "commit.gpgsign=false", "commit", "--quiet", "-am", "broken fixture")
		Expect(result.status).To(Equal(0), result.stderr)
		result = run("git", source, nil, "rev-parse", "HEAD")
		Expect(result.status).To(Equal(0), result.stderr)
		revision = strings.TrimSpace(result.stdout)
		out := outputFor("build-failure")
		result = run(packager, source, nil, argsFor(out)...)
		Expect(result.status).To(Equal(1))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).To(ContainSubstring("undefinedFixtureSymbol"))
		_, err := os.Lstat(out)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

// The native CI matrix provides an actual bundle; no replacement binary is used.
var _ = Describe("Packaged runtime conformance", func() {
	var binary, cwd string
	var env map[string]string
	BeforeEach(func() {
		binary = os.Getenv("FACTORY_BUNDLE_BINARY")
		if binary == "" {
			Skip("native bundle is supplied only by the packaging qualification job")
		}
		Expect(filepath.IsAbs(binary)).To(BeTrue())
		var err error
		cwd, err = os.MkdirTemp("", "factory packaged conformance ")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, cwd)
		env = map[string]string{"FACTORY_BRIDGE_PROTOCOL": "1", "PATH": filepath.Join(cwd, "absent-tools")}
	})
	It("reads literal configuration without Go or Python on PATH", func() {
		config := filepath.Join(cwd, "factory.yaml")
		writeFixture(config, []byte("model: literal packaged value\n"), 0600)
		env["FACTORY_CONFIG"] = config
		Expect(readerProcess(cwd, env, binary, "config", "get", "model")).To(Equal(cliResult{"literal packaged value", "", 0}))
	})
	It("resolves implementer and default role tiers without external tools", func() {
		Expect(readerProcess(cwd, env, binary, "role", "tier", "implementer")).To(Equal(cliResult{"default", "", 0}))
		Expect(readerProcess(cwd, env, binary, "role", "tier", "unknown-role")).To(Equal(cliResult{"default", "", 0}))
	})
	It("rejects corrupted artifacts without executing their payload", func() {
		manifest, root, _ := artifactFixture(cwd)
		marker := filepath.Join(cwd, "payload-executed")
		writeFixture(filepath.Join(root, "bin", "factory-runtime"), []byte("#!/bin/sh\nprintf executed > '"+marker+"'\n"), 0755)
		result := readerProcess(cwd, env, binary, "runtime", "verify", manifest, root, "linux/amd64")
		Expect(result.status).To(Equal(1))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
		_, err := os.Stat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0063-go-configuration-writes.md:96
	// per docs/adr/0063-go-configuration-writes.md:37
	// per docs/adr/0063-go-configuration-writes.md:51
	It("writes literal bytes and retains modes and siblings without Go or Python on PATH", func() {
		selected := filepath.Join(cwd, "factory.yaml")
		writeFixture(selected, []byte("model: old\r\nother: keep\r\nmodel: duplicate\n"), 0640)
		Expect(os.Chmod(selected, 0640)).To(Succeed())
		sibling := filepath.Join(cwd, "factory.yaml.factory-bak")
		writeFixture(sibling, []byte("caller backup"), 0600)
		env["FACTORY_CONFIG"] = selected
		value := "packaged/value&literal|slash\\tail"
		Expect(readerProcess(cwd, env, binary, "config", "set", "model", value)).To(Equal(cliResult{"", "", 0}))
		expectWriteFile(selected, "model: \""+value+"\"\nother: keep\r\nmodel: \""+value+"\"\n", 0640)
		expectWriteFile(sibling, "caller backup", 0600)
		Expect(readerProcess(cwd, env, binary, "config", "get", "model")).To(Equal(cliResult{value, "", 0}))
		entries, err := os.ReadDir(cwd)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(2))
	})

	// per docs/adr/0063-go-configuration-writes.md:30
	It("refuses writes through a symlink and preserves the referent without external tools", func() {
		selected := filepath.Join(cwd, "factory.yaml")
		referent := filepath.Join(cwd, "actual.yaml")
		writeFixture(referent, []byte("model: original\n"), 0600)
		Expect(os.Symlink(referent, selected)).To(Succeed())
		env["FACTORY_CONFIG"] = selected
		result := readerProcess(cwd, env, binary, "config", "set", "model", "new")
		Expect(result.status).To(Equal(1))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
		expectWriteFile(referent, "model: original\n", 0600)
		target, err := os.Readlink(selected)
		Expect(err).NotTo(HaveOccurred())
		Expect(target).To(Equal(referent))
		entries, err := os.ReadDir(cwd)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(2))
	})

	// per docs/adr/0064-go-native-usage-accounting.md:86
	DescribeTable("normalizes native usage without Python or native CLIs on PATH", func(harness string) {
		cost := "0.25"
		if harness == "codex" {
			cost = "null"
		}
		result := usageProcess(cwd, "["+usageEvents[harness]+"]", binary, "usage", "normalize", harness)
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(result.stderr).To(BeEmpty())
		Expect(result.stdout).To(HaveSuffix("\n"))
		Expect(usageMetadata(result.stdout)).To(Equal(usageMetadata(usageExpected(harness, usageTokens[harness], cost, true, false))))
		entries, err := os.ReadDir(cwd)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	}, Entry("Codex accounting", "codex"), Entry("Claude accounting", "claude"), Entry("OpenCode accounting", "opencode"))

})
