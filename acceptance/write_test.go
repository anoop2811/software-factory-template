package acceptance_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// This collaborator moves a newly prepared sibling at a context-check boundary.
// It supplements subprocess acceptance without timing races or production hooks.
type replacementDuringWrite struct {
	context.Context
	directory, held, replacement string
	moved                        string
	fault                        error
}

func (c *replacementDuringWrite) Err() error {
	if c.moved != "" || c.fault != nil {
		return c.Context.Err()
	}
	entries, err := os.ReadDir(c.directory)
	if err != nil {
		c.fault = err
		return c.Context.Err()
	}
	for _, entry := range entries {
		if entry.Name() == "factory.yaml" || !entry.Type().IsRegular() {
			continue
		}
		path := filepath.Join(c.directory, entry.Name())
		if err := os.Rename(path, c.held); err != nil {
			c.fault = err
			return c.Context.Err()
		}
		c.moved = path
		c.fault = os.WriteFile(path, []byte(c.replacement), 0600)
		return c.Context.Err()
	}
	return c.Context.Err()
}

func writeShim(root, cwd string, env map[string]string, args ...string) cliResult {
	GinkgoHelper()
	selected := map[string]string{"FACTORY_CONFIG": filepath.Join(cwd, "factory.yaml"), "FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}
	for k, v := range env {
		selected[k] = v
	}
	return readerProcess(cwd, selected, "bash", append([]string{"-c", `. "$1"; shift; factory_config_set "$@"`, "write-shim", exportShimPath()}, args...)...)
}

func writeBridge(root, cwd, path string, args ...string) cliResult {
	GinkgoHelper()
	return bridgeReader(root, cwd, map[string]string{"FACTORY_CONFIG": path}, append([]string{"config", "set"}, args...)...)
}

func expectWriteFile(path, expected string, mode os.FileMode) {
	GinkgoHelper()
	b, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	Expect(string(b)).To(Equal(expected))
	st, err := os.Stat(path)
	Expect(err).NotTo(HaveOccurred())
	Expect(st.Mode().Perm()).To(Equal(mode))
}

var _ = Describe("G1 candidate configuration writes", func() {
	// per docs/adr/0063-go-configuration-writes.md:37
	DescribeTable("matches immutable bytes and ordinary modes for admitted writes", func(before, key, value, after string) {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		for _, source := range []string{hooksHistorical(root, baselineCommit), hooksHistorical(root, releaseReaderBaseline), exportShimPath(), "private"} {
			writeFixture(path, []byte(before), 0640)
			Expect(os.Chmod(path, 0640)).To(Succeed())
			var result cliResult
			if source == "private" {
				result = writeBridge(root, cwd, path, key, value)
			} else {
				result = readerProcess(cwd, map[string]string{"FACTORY_CONFIG": path, "FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}, "bash", "-c", `. "$1"; factory_config_set "$2" "$3" ignored`, "write-parity", source, key, value)
			}
			Expect(result).To(Equal(cliResult{"", "", 0}), source)
			expectWriteFile(path, after, 0640)
		}
	}, Entry("replace duplicates", "key: first\nother: keep\nkey: second\n", "key", "new", "key: \"new\"\nother: keep\nkey: \"new\"\n"),
		Entry("CRLF selective replacement", "key: old\r\nother: keep\r\n", "key", "new", "key: \"new\"\nother: keep\r\n"),
		Entry("unterminated replacement", "key: old", "key", "new", "key: \"new\""),
		Entry("unterminated append", "other: keep", "key", "new", "other: keepkey: \"new\"\n"),
		Entry("empty file", "", "key", "new", "key: \"new\"\n"),
		Entry("empty value", "key: old # comment\n", "key", "", "key: \"\"\n"),
		Entry("LF folded for replacement", "key: old\n", "key", "first\nlast\n", "key: \"first last \"\n"),
		Entry("LF retained for append", "other: keep\n", "key", "first\nlast\n", "other: keep\nkey: \"first\nlast\n\"\n"),
		Entry("quotes and replacement metacharacters", "key: old\n", "key", "a\"b\\c&d|e", "key: \"a\"b\\c&d|e\"\n"),
		Entry("CR remains literal value", "key: old\n", "key", "first\rlast", "key: \"first\rlast\"\n"),
		Entry("exact column and key boundary", " key: indented\nkey_extra: keep\nkey : keep\n", "key", "new", " key: indented\nkey_extra: keep\nkey : keep\nkey: \"new\"\n"),
		Entry("case sensitive identifier", "KEY: keep\n", "key", "new", "KEY: keep\nkey: \"new\"\n"),
		Entry("dash underscore identifier", "_Model-name: old\n", "_Model-name", "日本語 λ", "_Model-name: \"日本語 λ\"\n"),
		Entry("blank trailing line retained", "other: keep\n\n", "key", "new", "other: keep\n\nkey: \"new\"\n"),
		Entry("value is not Cobra help", "key: old\n", "key", "--help", "key: \"--help\"\n"))

	// per docs/adr/0063-go-configuration-writes.md:15
	DescribeTable("refuses invalid private operands without touching any file", func(args []string) {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		before := "key: private-value\n"
		writeFixture(path, []byte(before), 0600)
		result := writeBridge(root, cwd, path, args...)
		Expect(result.status).To(Equal(2))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).To(HavePrefix("factory bridge: "))
		Expect(result.stderr).NotTo(ContainSubstring("Usage:"))
		Expect(result.stderr).NotTo(ContainSubstring("private-value"))
		expectWriteFile(path, before, 0600)
	}, Entry("no operands", []string{}), Entry("missing value", []string{"key"}), Entry("surplus operand", []string{"key", "new", "extra"}), Entry("empty key", []string{"", "new"}), Entry("regex key", []string{"key.*", "new"}), Entry("flag key", []string{"--help", "new"}), Entry("numeric prefix", []string{"1key", "new"}), Entry("multiline key", []string{"key\nother", "new"}))

	// per docs/adr/0063-go-configuration-writes.md:31
	DescribeTable("preserves the missing-file diagnostic and never bootstraps a configuration", func(kind string) {
		root, cwd := fixture()
		path := filepath.Join(cwd, "settings file.yaml")
		if kind == "directory" {
			Expect(os.Mkdir(path, 0700)).To(Succeed())
		}
		expected := cliResult{"", "factory config: no settings file.yaml here — run factory init first.\n", 1}
		for _, source := range []string{hooksHistorical(root, baselineCommit), hooksHistorical(root, releaseReaderBaseline), exportShimPath(), "private"} {
			var result cliResult
			if source == "private" {
				result = writeBridge(root, cwd, path, "key", "value")
			} else {
				result = readerProcess(cwd, map[string]string{"FACTORY_CONFIG": path, "FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}, "bash", "-c", `. "$1"; factory_config_set key value`, "missing-write", source)
			}
			Expect(result).To(Equal(expected), source)
		}
		if kind == "missing" {
			_, err := os.Stat(path)
			Expect(os.IsNotExist(err)).To(BeTrue())
		} else {
			entries, err := os.ReadDir(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(BeEmpty())
		}
	}, Entry("missing", "missing"), Entry("directory", "directory"))

	// per docs/adr/0063-go-configuration-writes.md:31
	DescribeTable("retains lexical basenames in missing and directory diagnostics", func(suffix, basename string) {
		root, cwd := fixture()
		directory := filepath.Join(cwd, "settings")
		Expect(os.Mkdir(directory, 0700)).To(Succeed())
		path := directory + suffix
		for _, source := range []string{hooksHistorical(root, baselineCommit), hooksHistorical(root, releaseReaderBaseline), exportShimPath(), "private"} {
			var result cliResult
			if source == "private" {
				result = writeBridge(root, cwd, path, "key", "new")
			} else {
				result = readerProcess(cwd, map[string]string{"FACTORY_CONFIG": path, "FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}, "bash", "-c", `. "$1"; factory_config_set key new`, "lexical-missing", source)
			}
			Expect(result).To(Equal(cliResult{"", "factory config: no " + basename + " here — run factory init first.\n", 1}), source)
		}
		entries, err := os.ReadDir(directory)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	}, Entry("trailing slash", "/", "settings"), Entry("final dot", "/.", "."), Entry("final dotdot", "/..", ".."), Entry("missing ancestor", "/missingparent/config.yaml", "config.yaml"))

	// per docs/adr/0063-go-configuration-writes.md:51
	DescribeTable("preserves ordinary modes and ownership across publication", func(mode os.FileMode) {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		writeFixture(path, []byte("key: old\n"), 0600)
		Expect(os.Chmod(path, mode)).To(Succeed())
		before, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		beforeNative, ok := before.Sys().(*syscall.Stat_t)
		Expect(ok).To(BeTrue())
		Expect(writeBridge(root, cwd, path, "key", "new")).To(Equal(cliResult{"", "", 0}))
		expectWriteFile(path, "key: \"new\"\n", mode)
		after, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		afterNative, ok := after.Sys().(*syscall.Stat_t)
		Expect(ok).To(BeTrue())
		Expect(afterNative.Uid).To(Equal(beforeNative.Uid))
		Expect(afterNative.Gid).To(Equal(beforeNative.Gid))
	}, Entry("private", os.FileMode(0600)), Entry("world readable", os.FileMode(0644)), Entry("executable", os.FileMode(0770)))

	// per docs/adr/0063-go-configuration-writes.md:24
	DescribeTable("refuses links and special files without changing their targets", func(kind string) {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		target := filepath.Join(cwd, "target")
		original := "key: original-secret\n"
		writeFixture(target, []byte(original), 0600)
		switch kind {
		case "symlink":
			Expect(os.Symlink(target, path)).To(Succeed())
		case "dangling symlink":
			Expect(os.Symlink(filepath.Join(cwd, "absent"), path)).To(Succeed())
		case "hardlink":
			Expect(os.Link(target, path)).To(Succeed())
		case "fifo":
			Expect(readerProcess(cwd, nil, "mkfifo", path).status).To(Equal(0))
		case "setuid":
			writeFixture(path, []byte(original), 0600)
			Expect(os.Chmod(path, 0600|os.ModeSetuid)).To(Succeed())
		case "setgid":
			writeFixture(path, []byte(original), 0600)
			Expect(os.Chmod(path, 0600|os.ModeSetgid)).To(Succeed())
		}
		before, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		result := writeBridge(root, cwd, path, "key", "new-secret")
		Expect(result.status).To(Equal(1))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
		Expect(result.stderr).NotTo(ContainSubstring("new-secret"))
		Expect(result.stderr).NotTo(ContainSubstring("original-secret"))
		after, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		Expect(after.Mode()).To(Equal(before.Mode()))
		expectWriteFile(target, original, 0600)
		entries, err := os.ReadDir(cwd)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(2))
	}, Entry("symlink", "symlink"), Entry("dangling symlink", "dangling symlink"), Entry("hardlink", "hardlink"), Entry("fifo", "fifo"), Entry("setuid", "setuid"), Entry("setgid", "setgid"))

	// per docs/adr/0063-go-configuration-writes.md:63
	DescribeTable("refuses oversized inputs or results without publication or leftovers", func(overflowResult bool) {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		before := strings.Repeat("x", 16*1024*1024+1)
		if overflowResult {
			before = strings.Repeat("x", 16*1024*1024-2)
		}
		writeFixture(path, []byte(before), 0600)
		result := writeBridge(root, cwd, path, "key", "private-value")
		Expect(result.status).To(Equal(1))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).To(ContainSubstring("16 MiB"))
		Expect(result.stderr).NotTo(ContainSubstring("private-value"))
		expectWriteFile(path, before, 0600)
		entries, err := os.ReadDir(cwd)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
	}, Entry("input", false), Entry("result", true))

	// per docs/adr/0063-go-configuration-writes.md:48
	DescribeTable("publishes a replacement inode and preserves unrelated backup siblings", func(before string) {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		writeFixture(path, []byte(before), 0640)
		for _, name := range []string{"factory.yaml.factory-bak", ".factory-config-write-existing", "unrelated"} {
			writeFixture(filepath.Join(cwd, name), []byte("owned by caller"), 0600)
		}
		old, err := os.Open(path)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(old.Close)
		Expect(writeShim(root, cwd, nil, "key", "new")).To(Equal(cliResult{"", "", 0}))
		stillOld, err := io.ReadAll(old)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(stillOld)).To(Equal(before))
		oldInfo, err := old.Stat()
		Expect(err).NotTo(HaveOccurred())
		newInfo, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(oldInfo, newInfo)).To(BeFalse())
		Expect(newInfo.Mode().Perm()).To(Equal(os.FileMode(0640)))
		for _, name := range []string{"factory.yaml.factory-bak", ".factory-config-write-existing", "unrelated"} {
			expectWriteFile(filepath.Join(cwd, name), "owned by caller", 0600)
		}
		entries, err := os.ReadDir(cwd)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(4))
	}, Entry("replace", "key: old\n"), Entry("append", "other: keep\n"))

	// per docs/adr/0063-go-configuration-writes.md:30
	DescribeTable("leaves original bytes on permission failures", func(readFailure bool) {
		if os.Geteuid() == 0 {
			Skip("permission denial needs an unprivileged test user")
		}
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		before := "key: original\n"
		writeFixture(path, []byte(before), 0600)
		if readFailure {
			Expect(os.Chmod(path, 0200)).To(Succeed())
			DeferCleanup(os.Chmod, path, os.FileMode(0600))
		} else {
			Expect(os.Chmod(cwd, 0500)).To(Succeed())
			DeferCleanup(os.Chmod, cwd, os.FileMode(0700))
		}
		result := writeBridge(root, cwd, path, "key", "new")
		Expect(result.status).To(Equal(1))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
		Expect(os.Chmod(path, 0600)).To(Succeed())
		Expect(os.Chmod(cwd, 0700)).To(Succeed())
		expectWriteFile(path, before, 0600)
		entries, err := os.ReadDir(cwd)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
	}, Entry("unreadable input", true), Entry("unwritable parent", false))

	// per docs/adr/0063-go-configuration-writes.md:53
	It("cleans its preparation file after a real file-size-limit write failure", func() {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		before := "key: original\n"
		writeFixture(path, []byte(before), 0600)
		writeFixture(filepath.Join(cwd, "factory.yaml.factory-bak"), []byte("caller backup"), 0600)
		probe := `. "$1"; trap '' XFSZ; ulimit -f 1 || exit 77; factory_config_set key "$2"`
		result := readerProcess(cwd, map[string]string{"FACTORY_CONFIG": path, "FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}, "bash", "-c", probe, "limited-write", exportShimPath(), strings.Repeat("x", 8192))
		if result.status == 77 {
			Skip("file size limit unavailable on test platform")
		}
		Expect(result.status).To(Equal(1))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
		expectWriteFile(path, before, 0600)
		expectWriteFile(filepath.Join(cwd, "factory.yaml.factory-bak"), "caller backup", 0600)
		entries, err := os.ReadDir(cwd)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(2))
	})

	// per docs/adr/0063-go-configuration-writes.md:24
	DescribeTable("resolves explicit paths with existing kernel traversal semantics", func(lexical bool) {
		root, cwd := fixture()
		selected := "settings file.yaml"
		target := filepath.Join(cwd, selected)
		if lexical {
			parent := filepath.Join(root, "outside configuration")
			Expect(os.MkdirAll(filepath.Join(parent, "sub"), 0700)).To(Succeed())
			Expect(os.Symlink(filepath.Join(parent, "sub"), filepath.Join(cwd, "link"))).To(Succeed())
			selected = "link/../factory.yaml"
			target = filepath.Join(parent, "factory.yaml")
			writeFixture(filepath.Join(cwd, "factory.yaml"), []byte("must remain unchanged"), 0600)
		}
		writeFixture(target, []byte("key: old\n"), 0600)
		Expect(writeBridge(root, cwd, selected, "key", "new")).To(Equal(cliResult{"", "", 0}))
		expectWriteFile(target, "key: \"new\"\n", 0600)
		if lexical {
			expectWriteFile(filepath.Join(cwd, "factory.yaml"), "must remain unchanged", 0600)
		}
	}, Entry("relative path with spaces", false), Entry("symlink parent followed by dotdot", true))

	// per docs/adr/0063-go-configuration-writes.md:18
	It("preserves caller state and sends unexported readonly paths literally", func() {
		root, cwd := fixture()
		path := filepath.Join(cwd, "other settings")
		writeFixture(path, []byte("key: old\n"), 0600)
		probe := `. "$1"; FACTORY_CONFIG="$2"; readonly FACTORY_CONFIG; FACTORY_BRIDGE_PROTOCOL=caller; readonly FACTORY_BRIDGE_PROTOCOL; set -euf; IFS=:; before="$-"; before_pwd="$PWD"; factory_config_set key new ignored; [ "$IFS" = : ] && [ "$before" = "$-" ] && [ "$before_pwd" = "$PWD" ] && [ "$FACTORY_CONFIG" = "$2" ] && [ "$FACTORY_BRIDGE_PROTOCOL" = caller ]`
		result := readerProcess(cwd, map[string]string{"FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}, "bash", "-c", probe, "caller-state", exportShimPath(), path)
		Expect(result).To(Equal(cliResult{"", "", 0}))
		expectWriteFile(path, "key: \"new\"\n", 0600)
	})

	// per docs/adr/0063-go-configuration-writes.md:20
	DescribeTable("retains missing operands under nounset before any write", func(args []string) {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		before := "key: old\n"
		writeFixture(path, []byte(before), 0600)
		for _, source := range []string{hooksHistorical(root, baselineCommit), hooksHistorical(root, releaseReaderBaseline), exportShimPath()} {
			result := readerProcess(cwd, map[string]string{"FACTORY_CONFIG": path, "FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}, "bash", append([]string{"-u", "-c", `. "$1"; shift; factory_config_set "$@"`, "nounset-write", source}, args...)...)
			Expect(result.status).NotTo(Equal(0))
			Expect(result.stdout).To(BeEmpty())
			Expect(result.stderr).To(ContainSubstring("unbound variable"))
			expectWriteFile(path, before, 0600)
		}
	}, Entry("both missing", []string{}), Entry("value missing", []string{"key"}))

	// per docs/adr/0063-go-configuration-writes.md:20
	It("keeps the omitted value empty for an ordinary shell caller", func() {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		writeFixture(path, []byte("key: old\n"), 0600)
		Expect(writeShim(root, cwd, nil, "key")).To(Equal(cliResult{"", "", 0}))
		expectWriteFile(path, "key: \"\"\n", 0600)
	})

	// per docs/adr/0063-go-configuration-writes.md:21
	It("sources without calling the selected runtime", func() {
		root, cwd := fixture()
		fake := filepath.Join(root, "fake")
		marker := filepath.Join(cwd, "marker")
		writeFixture(fake, []byte("#!/bin/sh\ntouch \"$WRITE_MARKER\"\nexit 73\n"), 0755)
		result := readerProcess(cwd, map[string]string{"FACTORY_RUNTIME_BINARY": fake, "WRITE_MARKER": marker}, "bash", "-c", `set -euf; . "$1"; declare -F factory_config_set >/dev/null`, "source-only", exportShimPath())
		Expect(result).To(Equal(cliResult{"", "", 0}))
		_, err := os.Stat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0063-go-configuration-writes.md:19
	DescribeTable("refuses missing or unusable runtimes without falling back to shell mutation", func(kind string) {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		writeFixture(path, []byte("key: old\n"), 0600)
		runtime := filepath.Join(root, "absent")
		switch kind {
		case "empty":
			runtime = ""
		case "relative":
			runtime = "./factory"
		case "nonexecutable":
			writeFixture(runtime, nil, 0600)
		case "directory":
			Expect(os.Mkdir(runtime, 0700)).To(Succeed())
		}
		result := writeShim(root, cwd, map[string]string{"FACTORY_RUNTIME_BINARY": runtime}, "key", "new")
		Expect(result).To(Equal(cliResult{"", "factory bridge: FACTORY_RUNTIME_BINARY must name an executable absolute regular file\n", 2}))
		expectWriteFile(path, "key: old\n", 0600)
	}, Entry("missing", "missing"), Entry("empty", "empty"), Entry("relative", "relative"), Entry("nonexecutable", "nonexecutable"), Entry("directory", "directory"))

	// per docs/adr/0063-go-configuration-writes.md:93
	DescribeTable("never evaluates values under inherited helper variable attributes", func(setup string) {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		writeFixture(path, []byte("key: old\n"), 0600)
		value := "array[$(touch marker)] `touch marker` $HOME ; & | \\\""
		probe := `. "$1"; ` + setup + `; factory_config_set key "$2"`
		result := readerProcess(cwd, map[string]string{"FACTORY_CONFIG": path, "FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}, "bash", "-c", probe, "literal-write", exportShimPath(), value)
		if result.status == 77 {
			Skip("optional caller attribute is unavailable")
		}
		_, err := os.Stat(filepath.Join(cwd, "marker"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(result).To(Equal(cliResult{"", "", 0}))
		expectWriteFile(path, "key: \""+value+"\"\n", 0600)
	}, Entry("ordinary", ":"), Entry("integer helper names", "declare -i key=0 value=0 file=0 esc=0 _factory_config_value=0"), Entry("nameref helper name", "declare -i target=0; declare -n value=target 2>/dev/null || exit 77"))

	// per docs/adr/0063-go-configuration-writes.md:48
	It("writes and reads ordinary values with no external parsing tools", func() {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		writeFixture(path, []byte("key: old\n"), 0600)
		result := readerProcess(cwd, map[string]string{"FACTORY_CONFIG": path, "FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory"), "PATH": "/no-go-python-sed-or-grep"}, "/bin/bash", "-c", `. "$1"; factory_config_set key '日本語/value&literal'`, "no-tools-write", exportShimPath())
		Expect(result).To(Equal(cliResult{"", "", 0}))
		expectWriteFile(path, "key: \"日本語/value&literal\"\n", 0600)
		Expect(bridgeReader(root, cwd, map[string]string{"FACTORY_CONFIG": path}, "config", "get", "key")).To(Equal(cliResult{"日本語/value&literal", "", 0}))
	})
	// per docs/adr/0063-go-configuration-writes.md:71
	It("preserves a replacement occupant when refusing a changed preparation file", func() {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		before := "key: original\n"
		writeFixture(path, []byte(before), 0600)
		held := filepath.Join(root, "held-preparation")
		sentinel := "replacement belongs to someone else"
		ctx := &replacementDuringWrite{Context: context.Background(), directory: cwd, held: held, replacement: sentinel}
		err := config.Set(ctx, path, "key", "new")
		Expect(ctx.fault).NotTo(HaveOccurred(), "test-owned collaborator must finish its replacement")
		Expect(ctx.moved).NotTo(BeEmpty(), "the collaborator must observe an actual preparation file")
		Expect(err).To(HaveOccurred())
		expectWriteFile(path, before, 0600)
		expectWriteFile(held, "key: \"new\"\n", 0600)
		expectWriteFile(ctx.moved, sentinel, 0600)
	})

	// per docs/adr/0063-go-configuration-writes.md:24
	It("retains setter command-substitution trimming of trailing path linefeeds", func() {
		root, cwd := fixture()
		plain := filepath.Join(cwd, "factory.yaml")
		literal := plain + "\n\n"
		for _, source := range []string{hooksHistorical(root, baselineCommit), hooksHistorical(root, releaseReaderBaseline), exportShimPath(), "private"} {
			writeFixture(plain, []byte("key: plain\n"), 0600)
			writeFixture(literal, []byte("key: literal\n"), 0600)
			var result cliResult
			if source == "private" {
				result = writeBridge(root, cwd, literal, "key", "new")
			} else {
				result = readerProcess(cwd, map[string]string{"FACTORY_CONFIG": literal, "FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}, "bash", "-c", `. "$1"; factory_config_set key new`, "path-linefeeds", source)
			}
			Expect(result).To(Equal(cliResult{"", "", 0}), source)
			expectWriteFile(plain, "key: \"new\"\n", 0600)
			expectWriteFile(literal, "key: literal\n", 0600)
		}
	})

	// per docs/adr/0063-go-configuration-writes.md:31
	It("retains the empty basename diagnostic when linefeed trimming empties the selected path", func() {
		root, cwd := fixture()
		literal := filepath.Join(cwd, "\n")
		writeFixture(literal, []byte("key: literal\n"), 0600)
		for _, source := range []string{hooksHistorical(root, baselineCommit), hooksHistorical(root, releaseReaderBaseline), exportShimPath(), "private"} {
			var result cliResult
			if source == "private" {
				result = writeBridge(root, cwd, "\n", "key", "new")
			} else {
				result = readerProcess(cwd, map[string]string{"FACTORY_CONFIG": "\n", "FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}, "bash", "-c", `. "$1"; factory_config_set key new`, "empty-trimmed-path", source)
			}
			Expect(result).To(Equal(cliResult{"", "factory config: no  here — run factory init first.\n", 1}), source)
			expectWriteFile(literal, "key: literal\n", 0600)
		}
	})

})
