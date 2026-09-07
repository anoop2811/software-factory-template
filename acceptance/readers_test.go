package acceptance_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const releaseReaderBaseline = "b71ecc32e07ecd87eb330ba8e497c86612f92acd"

func readerProcess(cwd string, env map[string]string, program string, args ...string) cliResult {
	GinkgoHelper()
	// Decision 51 records C/POSIX whitespace as this candidate's explicit test
	// scope; this does not claim parity under other inherited locales.
	contextEnv := map[string]string{"LC_ALL": "C"}
	for key, value := range env {
		contextEnv[key] = value
	}
	env = contextEnv
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, args...) // #nosec G204 -- acceptance-owned executable boundary; all values are literal arguments.
	cmd.Dir = cwd
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if member(name, "FACTORY_CONFIG", "FACTORY_BRIDGE_PROTOCOL", "FACTORY_RUNTIME_BINARY", "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "BASH_ENV", "ENV") {
			continue
		}
		if _, overridden := env[name]; !overridden {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred())
	status := 0
	if err != nil {
		var exit *exec.ExitError
		Expect(errors.As(err, &exit)).To(BeTrue(), "process launch failed: %v", err)
		status = exit.ExitCode()
	}
	return cliResult{stdout.String(), stderr.String(), status}
}

func historicalReader(root, cwd, revision, function string, env map[string]string, args ...string) cliResult {
	GinkgoHelper()
	for _, library := range []string{"config", "roles"} {
		path := filepath.Join(root, revision, library+".sh")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			command := exec.Command("git", "show", revision+":scripts/lib/"+library+".sh") // #nosec G204 -- fixed library names and immutable acceptance baseline revisions.
			contents, err := command.Output()
			Expect(err).NotTo(HaveOccurred())
			writeFixture(path, contents, 0600)
		}
	}
	argv := []string{"-c", `. "$1"; . "$2"; shift 2; "$@"`, "baseline-readers", filepath.Join(root, revision, "config.sh"), filepath.Join(root, revision, "roles.sh"), function}
	if env["FACTORY_ACCEPTANCE_NOUNSET"] == "1" {
		argv = append([]string{"-u"}, argv...)
	}
	shell := "bash"
	if env["FACTORY_ACCEPTANCE_SHELL"] == "/bin/sh" {
		shell = "/bin/sh"
	}
	return readerProcess(cwd, env, shell, append(argv, args...)...)
}

func shimReader(root, cwd string, env map[string]string, function string, args ...string) cliResult {
	GinkgoHelper()
	shim, err := filepath.Abs(filepath.Join("..", "runtime", "shell", "readers.sh"))
	Expect(err).NotTo(HaveOccurred())
	contextEnv := map[string]string{"FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}
	for key, value := range env {
		contextEnv[key] = value
	}
	argv := []string{"-c", `. "$1"; shift; "$@"`, "candidate-readers", shim, function}
	if env["FACTORY_ACCEPTANCE_NOUNSET"] == "1" {
		argv = append([]string{"-u"}, argv...)
	}
	shell := "bash"
	if env["FACTORY_ACCEPTANCE_SHELL"] == "/bin/sh" {
		shell = "/bin/sh"
	}
	return readerProcess(cwd, contextEnv, shell, append(argv, args...)...)
}

func bridgeReader(root, cwd string, env map[string]string, args ...string) cliResult {
	GinkgoHelper()
	private := map[string]string{"FACTORY_BRIDGE_PROTOCOL": "1"}
	for key, value := range env {
		private[key] = value
	}
	return readerProcess(cwd, private, filepath.Join(root, "factory"), args...)
}

func expectReaderParity(root, cwd, function string, env map[string]string, expected cliResult, private, legacy []string) {
	GinkgoHelper()
	for _, revision := range []string{baselineCommit, releaseReaderBaseline} {
		Expect(historicalReader(root, cwd, revision, function, env, legacy...)).To(Equal(expected), revision)
	}
	Expect(bridgeReader(root, cwd, env, private...)).To(Equal(expected))
}

var _ = Describe("G1 private read-only reader bridge", func() {
	// per specs/001-go-runtime-conversion.md:102
	It("returns the explicit configuration path verbatim", func() {
		root, cwd := fixture()
		path := "relative config/日本語\nname.yaml"
		expectReaderParity(root, cwd, "factory_config_file", map[string]string{"FACTORY_CONFIG": path}, cliResult{path, "", 0}, []string{"config", "file"}, nil)
	})

	// per specs/001-go-runtime-conversion.md:102
	DescribeTable("reads the first matching flat value using the existing literal grammar", func(contents, key, fallback, expected string) {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		writeFixture(path, []byte(contents), 0600)
		expectReaderParity(root, cwd, "factory_config_get", map[string]string{"FACTORY_CONFIG": path}, cliResult{expected, "", 0}, []string{"config", "get", key, fallback}, []string{key, fallback})
	},
		Entry("ordinary value", "model: first\n", "model", "fallback", "first"),
		Entry("first duplicate wins", "model: first\nmodel: second\n", "model", "fallback", "first"),
		Entry("first empty duplicate chooses default", "model:\nmodel: second\n", "model", "fallback", "fallback"),
		Entry("empty quoted value chooses default", "model: \"\"\n", "model", "fallback", "fallback"),
		Entry("double-quoted hash remains literal", "model: \"value # inside\" # outside\n", "model", "fallback", "value # inside"),
		Entry("unquoted inline comment", "model: value \t # comment\n", "model", "fallback", "value"),
		Entry("hash without separating space", "model: value#literal\n", "model", "fallback", "value#literal"),
		Entry("single quotes remain raw", "model: 'value # comment'\n", "model", "fallback", "'value"),
		Entry("first closing double quote", "model: \"value\" trailing text\n", "model", "fallback", "value"),
		Entry("unclosed quote", "model: \"value", "model", "fallback", "value"),
		Entry("leading and trailing whitespace", "model:\t \v\fvalue \t\r\n", "model", "fallback", "value"),
		Entry("CRLF", "model: value\r\n", "model", "fallback", "value"),
		Entry("EOF without newline", "model: value", "model", "fallback", "value"),
		Entry("indented key does not match", " model: value\n", "model", "fallback", "fallback"),
		Entry("Unicode value", "model: 日本語 λ مرحبا\n", "model", "fallback", "日本語 λ مرحبا"),
		Entry("C locale preserves Unicode whitespace", "model: \u00a0\u2003value\u202f\u3000\n", "model", "fallback", "\u00a0\u2003value\u202f\u3000"),
		Entry("unterminated quoted CR is literal", "model: \"value\r\n", "model", "fallback", "value\r"),
		Entry("newline ends a flat value", "model: \"first\nsecond\"\n", "model", "fallback", "first"),
		Entry("newline in default is literal", "other: value\n", "model", "fallback\n\n", "fallback\n\n"),
		Entry("greater than scanner token limit", "model: "+strings.Repeat("界", 30000)+"\n", "model", "fallback", strings.Repeat("界", 30000)),
	)

	// per specs/001-go-runtime-conversion.md:102
	DescribeTable("distinguishes key presence from a nonempty value", func(contents string, expected int) {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		writeFixture(path, []byte(contents), 0600)
		expectReaderParity(root, cwd, "factory_config_has", map[string]string{"FACTORY_CONFIG": path}, cliResult{"", "", expected}, []string{"config", "has", "model"}, []string{"model"})
	}, Entry("empty is present", "model:\n", 0), Entry("quoted empty is present", "model: \"\"\n", 0),
		Entry("missing", "other: value\n", 1), Entry("case-sensitive key", "MODEL: value\n", 1))

	// per specs/001-go-runtime-conversion.md:103
	DescribeTable("maps the same roles for every native harness", func(role, expected string) {
		root, cwd := fixture()
		expectReaderParity(root, cwd, "role_tier", nil, cliResult{expected, "", 0}, []string{"role", "tier", role}, []string{role})
	}, Entry("spec-writer", "spec-writer", "frontier"), Entry("reviewer", "reviewer", "frontier"),
		Entry("refactorer", "refactorer", "economy"), Entry("wiki-maintainer", "wiki-maintainer", "economy"),
		Entry("implementer", "implementer", "default"), Entry("unknown future role", "future-role", "default"),
		Entry("empty role", "", "default"), Entry("case-sensitive role", "Reviewer", "default"))

	// per specs/001-go-runtime-conversion.md:103
	DescribeTable("resolves tiers without lowering frontier review", func(profile, tier, expected string) {
		root, cwd := fixture()
		expectReaderParity(root, cwd, "resolve_tier", nil, cliResult{expected, "", 0}, []string{"role", "resolve", profile, tier}, []string{profile, tier})
	}, Entry("standard economy collapses", "standard", "economy", "default"), Entry("economy stays economy", "economy", "economy", "economy"),
		Entry("standard frontier", "standard", "frontier", "frontier"), Entry("economy frontier", "economy", "frontier", "frontier"),
		Entry("default unchanged", "economy", "default", "default"), Entry("unknown profile collapses economy", "unknown", "economy", "default"),
		Entry("profile case-sensitive", "Economy", "economy", "default"), Entry("unknown tier remains literal", "standard", "--unknown tier\n", "--unknown tier\n"),
		Entry("empty tier", "", "", ""))
})

var _ = Describe("G1 reader protocol and filesystem boundaries", func() {
	// per specs/001-go-runtime-conversion.md:298
	DescribeTable("rejects malformed private requests without output or mutation", func(args []string) {
		root, cwd := fixture()
		result := bridgeReader(root, cwd, nil, args...)
		Expect(result.status).To(Equal(2))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).To(HavePrefix("factory bridge: "))
		Expect(result.stderr).NotTo(ContainSubstring("Usage:"))
		files, err := os.ReadDir(cwd)
		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(BeEmpty())
	}, Entry("no request", []string{}), Entry("config namespace only", []string{"config"}), Entry("role namespace only", []string{"role"}),
		Entry("unknown operation", []string{"config", "unknown-operation", "key", "value"}), Entry("public operation is not private", []string{"budget", "run"}),
		Entry("file surplus operand", []string{"config", "file", "extra"}), Entry("get missing key", []string{"config", "get"}),
		Entry("get surplus operand", []string{"config", "get", "key", "default", "extra"}), Entry("has missing key", []string{"config", "has"}),
		Entry("has surplus operand", []string{"config", "has", "key", "extra"}), Entry("tier missing role", []string{"role", "tier"}),
		Entry("tier surplus operand", []string{"role", "tier", "reviewer", "extra"}), Entry("resolve missing tier", []string{"role", "resolve", "economy"}),
		Entry("resolve surplus operand", []string{"role", "resolve", "economy", "default", "extra"}), Entry("help command", []string{"help"}),
		Entry("help flag", []string{"--help"}), Entry("nested help flag", []string{"config", "--help"}),
		Entry("Cobra completion escape", []string{"__complete", "config"}), Entry("empty key", []string{"config", "get", ""}),
		Entry("regex-like key", []string{"config", "get", "key.*"}), Entry("numeric leading key", []string{"config", "has", "0key"}),
		Entry("key argument is not a flag", []string{"config", "get", "--help"}), Entry("metacharacter key", []string{"config", "has", "$(touch marker)"}))

	// per specs/001-go-runtime-conversion.md:298
	DescribeTable("refuses unrecognized protocol versions", func(version string) {
		root, cwd := fixture()
		result := bridgeReader(root, cwd, map[string]string{"FACTORY_BRIDGE_PROTOCOL": version}, "config", "file")
		Expect(result.status).To(Equal(2))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).To(HavePrefix("factory bridge: "))
	}, Entry("zero", "0"), Entry("future version", "2"), Entry("leading zero", "01"), Entry("whitespace", " 1"))

	// per specs/001-go-runtime-conversion.md:298
	DescribeTable("keeps private namespaces absent from the ordinary public CLI", func(command string) {
		root, cwd := fixture()
		for _, env := range []map[string]string{nil, {"FACTORY_BRIDGE_PROTOCOL": ""}} {
			result := readerProcess(cwd, env, filepath.Join(root, "factory"), command, "file")
			Expect(result).To(Equal(cliResult{"", "factory: unknown command '" + command + "' (try: factory help)\n", 2}))
		}
	}, Entry("config", "config"), Entry("role", "role"))

	// per specs/001-go-runtime-conversion.md:102
	It("reads relative configuration paths against the caller directory", func() {
		root, cwd := fixture()
		writeFixture(filepath.Join(cwd, "settings file.yaml"), []byte("_Model-name: value\n"), 0600)
		env := map[string]string{"FACTORY_CONFIG": "settings file.yaml"}
		expectReaderParity(root, cwd, "factory_config_get", env, cliResult{"value", "", 0}, []string{"config", "get", "_Model-name"}, []string{"_Model-name"})
	})

	// per specs/001-go-runtime-conversion.md:102
	DescribeTable("uses Git root only when the configuration override is empty or unset", func(gitRepo, empty bool) {
		root, cwd := fixture()
		expected := "./factory.yaml"
		if gitRepo {
			project := filepath.Join(cwd, "project with trailing space ")
			Expect(os.MkdirAll(project, 0755)).To(Succeed())
			Expect(readerProcess(cwd, nil, "git", "init", "-q", project).status).To(Equal(0))
			cwd = filepath.Join(project, "deep", "nested")
			Expect(os.MkdirAll(cwd, 0755)).To(Succeed())
			expected = filepath.Join(project, "factory.yaml")
		}
		env := map[string]string{}
		if empty {
			env["FACTORY_CONFIG"] = ""
		}
		expectReaderParity(root, cwd, "factory_config_file", env, cliResult{expected, "", 0}, []string{"config", "file"}, nil)
	}, Entry("nested Git / unset", true, false), Entry("nested Git / empty", true, true), Entry("not Git / unset", false, false), Entry("not Git / empty", false, true))

	// per specs/001-go-runtime-conversion.md:102
	It("uses the existing relative fallback when Git is unavailable", func() {
		root, cwd := fixture()
		expectReaderParity(root, cwd, "factory_config_file", map[string]string{"PATH": ""}, cliResult{"./factory.yaml", "", 0}, []string{"config", "file"}, nil)
	})

	// per specs/001-go-runtime-conversion.md:102
	DescribeTable("handles missing and nonregular configurations without attempting a read", func(kind string) {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		switch kind {
		case "directory":
			Expect(os.Mkdir(path, 0755)).To(Succeed())
		case "dangling symlink":
			Expect(os.Symlink(filepath.Join(cwd, "missing-target"), path)).To(Succeed())
		}
		env := map[string]string{"FACTORY_CONFIG": path}
		expectReaderParity(root, cwd, "factory_config_get", env, cliResult{"fallback", "", 0}, []string{"config", "get", "key", "fallback"}, []string{"key", "fallback"})
		expectReaderParity(root, cwd, "factory_config_has", env, cliResult{"", "", 1}, []string{"config", "has", "key"}, []string{"key"})
	}, Entry("missing file", "missing"), Entry("directory", "directory"), Entry("dangling symlink", "dangling symlink"))

	// per specs/001-go-runtime-conversion.md:102
	It("follows a configuration symlink without rewriting it", func() {
		root, cwd := fixture()
		target := filepath.Join(cwd, "actual config")
		path := filepath.Join(cwd, "factory.yaml")
		writeFixture(target, []byte("key: from symlink\n"), 0600)
		Expect(os.Symlink(target, path)).To(Succeed())
		beforeTarget, err := os.Stat(target)
		Expect(err).NotTo(HaveOccurred())
		beforeLink, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		beforeEntries, err := os.ReadDir(cwd)
		Expect(err).NotTo(HaveOccurred())
		expectReaderParity(root, cwd, "factory_config_get", map[string]string{"FACTORY_CONFIG": path}, cliResult{"from symlink", "", 0}, []string{"config", "get", "key"}, []string{"key"})
		expectReaderParity(root, cwd, "factory_config_has", map[string]string{"FACTORY_CONFIG": path}, cliResult{"", "", 0}, []string{"config", "has", "key"}, []string{"key"})
		link, err := os.Readlink(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(link).To(Equal(target))
		contents, err := os.ReadFile(target)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(contents)).To(Equal("key: from symlink\n"))
		afterTarget, err := os.Stat(target)
		Expect(err).NotTo(HaveOccurred())
		afterLink, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(afterTarget.Mode()).To(Equal(beforeTarget.Mode()))
		Expect(afterLink.Mode()).To(Equal(beforeLink.Mode()))
		Expect(os.SameFile(beforeTarget, afterTarget)).To(BeTrue())
		Expect(os.SameFile(beforeLink, afterLink)).To(BeTrue())
		afterEntries, err := os.ReadDir(cwd)
		Expect(err).NotTo(HaveOccurred())
		Expect(afterEntries).To(HaveLen(len(beforeEntries)))
		for i, entry := range beforeEntries {
			Expect(afterEntries[i].Name()).To(Equal(entry.Name()))
		}
	})

	// per specs/001-go-runtime-conversion.md:95
	It("preserves read-failure output and status channels", func() {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		writeFixture(path, []byte("key: unreadable\n"), 0000)
		if os.Geteuid() == 0 {
			Skip("root can bypass fixture read permissions; this case requires an ordinary user")
		}
		env := map[string]string{"FACTORY_CONFIG": path}
		for _, request := range []struct {
			function string
			private  []string
			legacy   []string
			stdout   string
			status   int
		}{
			{"factory_config_get", []string{"config", "get", "key", "fallback"}, []string{"key", "fallback"}, "fallback", 0},
			{"factory_config_has", []string{"config", "has", "key"}, []string{"key"}, "", 2},
		} {
			results := []cliResult{bridgeReader(root, cwd, env, request.private...)}
			for _, revision := range []string{baselineCommit, releaseReaderBaseline} {
				results = append(results, historicalReader(root, cwd, revision, request.function, env, request.legacy...))
			}
			for _, result := range results {
				Expect(result.status).To(Equal(request.status))
				Expect(result.stdout).To(Equal(request.stdout))
				Expect(result.stderr).To(ContainSubstring(path))
				Expect(strings.ToLower(result.stderr)).To(ContainSubstring("permission denied"))
			}
		}
	})

	// per specs/001-go-runtime-conversion.md:102
	DescribeTable("preserves NUL removal after raw key selection and leading-whitespace parsing", func(contents, expected string, present int) {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		writeFixture(path, []byte(contents), 0600)
		env := map[string]string{"FACTORY_CONFIG": path}
		for _, revision := range []string{baselineCommit, releaseReaderBaseline} {
			result := historicalReader(root, cwd, revision, "factory_config_get", env, "key", "fallback")
			Expect(result.status).To(Equal(0))
			Expect(result.stdout).To(Equal(expected), revision)
			// Newer Bash emits this command-substitution warning; older Bash
			// silently removes NUL. The candidate diagnostic boundary is recorded.
			if result.stderr != "" {
				Expect(result.stderr).To(ContainSubstring("ignored null byte in input"))
			}
			Expect(historicalReader(root, cwd, revision, "factory_config_has", env, "key")).To(Equal(cliResult{"", "", present}))
		}
		Expect(bridgeReader(root, cwd, env, "config", "get", "key", "fallback")).To(Equal(cliResult{expected, "", 0}))
		Expect(bridgeReader(root, cwd, env, "config", "has", "key")).To(Equal(cliResult{"", "", present}))
	}, Entry("value NUL", "key: one\x00two\n", "onetwo", 0),
		Entry("NUL key does not become a matching identifier", "ke\x00y: wrong\n", "fallback", 1),
		Entry("selection precedes NUL removal", "ke\x00y: wrong\nkey: right\n", "right", 0),
		Entry("NUL before space and quote", "key: \x00 \"quoted\"\n", " \"quoted\"", 0),
		Entry("NUL before remaining leading space", "key:\t\x00 value # tail\n", " value", 0))
})

var _ = Describe("G1 sourceable candidate reader shim", func() {
	// per specs/001-go-runtime-conversion.md:96
	It("has no output, child launch or environment mutation when sourced by POSIX sh", func() {
		root, cwd := fixture()
		shim, err := filepath.Abs(filepath.Join("..", "runtime", "shell", "readers.sh"))
		Expect(err).NotTo(HaveOccurred())
		marker := filepath.Join(cwd, "must-not-exist")
		fake := filepath.Join(root, "must-not-run")
		writeFixture(fake, []byte("#!/bin/sh\nprintf 'unexpected child'\n: > \"$FACTORY_ACCEPTANCE_MARKER\"\n"), 0755)
		probe := `before="$(env | sort)"; before_options="$-"; before_directory="$(pwd -P)"; . "$1"; after="$(env | sort)"; [ "$before" = "$after" ] && [ "$before_options" = "$-" ] && [ "$before_directory" = "$(pwd -P)" ]`
		env := map[string]string{"FACTORY_RUNTIME_BINARY": fake, "FACTORY_ACCEPTANCE_MARKER": marker, "FACTORY_BRIDGE_PROTOCOL": "caller protocol", "FACTORY_CONFIG": "caller path"}
		Expect(readerProcess(cwd, env, "/bin/sh", "-c", probe, "source-only", shim)).To(Equal(cliResult{"", "", 0}))
		_, err = os.Stat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue())
		files, err := os.ReadDir(cwd)
		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(BeEmpty())
	})

	// per specs/001-go-runtime-conversion.md:95
	DescribeTable("forwards existing reader names and ignores surplus arguments as before", func(function, expected string, args []string) {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		writeFixture(path, []byte("key: literal # comment\n"), 0600)
		if function == "factory_config_file" {
			expected = path
		}
		for _, shell := range []string{"bash", "/bin/sh"} {
			env := map[string]string{"FACTORY_CONFIG": path, "FACTORY_ACCEPTANCE_SHELL": shell}
			for _, revision := range []string{baselineCommit, releaseReaderBaseline} {
				Expect(historicalReader(root, cwd, revision, function, env, args...)).To(Equal(cliResult{expected, "", 0}))
			}
			Expect(shimReader(root, cwd, env, function, args...)).To(Equal(cliResult{expected, "", 0}))
		}
	}, Entry("file", "factory_config_file", "", []string{"ignored"}),
		Entry("get", "factory_config_get", "literal", []string{"key", "fallback", "ignored"}),
		Entry("has", "factory_config_has", "", []string{"key", "ignored"}),
		Entry("tier", "role_tier", "frontier", []string{"reviewer", "ignored"}),
		Entry("resolve", "resolve_tier", "default", []string{"standard", "economy", "ignored"}))

	// per specs/001-go-runtime-conversion.md:95
	It("forwards an unexported caller path and leaves caller variables unmodified", func() {
		root, cwd := fixture()
		shim, err := filepath.Abs(filepath.Join("..", "runtime", "shell", "readers.sh"))
		Expect(err).NotTo(HaveOccurred())
		path := filepath.Join(cwd, "local config")
		writeFixture(path, []byte("key: local value\n"), 0600)
		probe := `. "$1"; FACTORY_CONFIG="$2"; FACTORY_BRIDGE_PROTOCOL=caller; before_options="$-"; before_directory="$(pwd -P)"; factory_config_get key || exit "$?"; [ "$before_options" = "$-" ] && [ "$before_directory" = "$(pwd -P)" ] || exit 97; printf '\n%s\n%s' "$FACTORY_CONFIG" "$FACTORY_BRIDGE_PROTOCOL"`
		Expect(readerProcess(cwd, map[string]string{"FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}, "/bin/sh", "-c", probe, "local-caller", shim, path)).To(Equal(cliResult{"local value\n" + path + "\ncaller", "", 0}))
	})

	// per specs/001-go-runtime-conversion.md:95
	DescribeTable("respects readonly parent configuration and protocol variables", func(exported bool, readonlyName string) {
		root, cwd := fixture()
		shim, err := filepath.Abs(filepath.Join("..", "runtime", "shell", "readers.sh"))
		Expect(err).NotTo(HaveOccurred())
		path := filepath.Join(cwd, "readonly config")
		writeFixture(path, []byte("key: readonly value\n"), 0600)
		probe := `. "$1"; FACTORY_CONFIG="$2"; FACTORY_BRIDGE_PROTOCOL=caller; readonly "$4"; if [ "$3" = yes ]; then export FACTORY_CONFIG FACTORY_BRIDGE_PROTOCOL; fi; factory_config_get key fallback || exit "$?"; printf '\n%s\n%s' "$FACTORY_CONFIG" "$FACTORY_BRIDGE_PROTOCOL"`
		flag := "no"
		if exported {
			flag = "yes"
		}
		for _, shell := range []string{"bash", "/bin/sh"} {
			result := readerProcess(cwd, map[string]string{"FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory")}, shell, "-c", probe, "readonly-caller", shim, path, flag, readonlyName)
			Expect(result).To(Equal(cliResult{"readonly value\n" + path + "\ncaller", "", 0}))
		}
	}, Entry("local readonly path", false, "FACTORY_CONFIG"), Entry("exported readonly path", true, "FACTORY_CONFIG"),
		Entry("local readonly protocol", false, "FACTORY_BRIDGE_PROTOCOL"), Entry("exported readonly protocol", true, "FACTORY_BRIDGE_PROTOCOL"))

	// per specs/001-go-runtime-conversion.md:103
	It("passes defaults as literal data without shell evaluation or temporary files", func() {
		root, cwd := fixture()
		literal := "$(touch must-not-exist) `touch must-not-exist` --help\n\n"
		Expect(shimReader(root, cwd, nil, "factory_config_get", "absent", literal)).To(Equal(cliResult{literal, "", 0}))
		files, err := os.ReadDir(cwd)
		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(BeEmpty())
	})

	// per specs/001-go-runtime-conversion.md:95
	It("forwards a real child's nonzero status without masking it", func() {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		writeFixture(path, []byte("other: value\n"), 0600)
		for _, shell := range []string{"bash", "/bin/sh"} {
			env := map[string]string{"FACTORY_CONFIG": path, "FACTORY_ACCEPTANCE_SHELL": shell}
			Expect(shimReader(root, cwd, env, "factory_config_has", "absent")).To(Equal(cliResult{"", "", 1}))
			invalid := shimReader(root, cwd, env, "factory_config_get", "invalid.*")
			Expect(invalid.status).To(Equal(2))
			Expect(invalid.stdout).To(BeEmpty())
			Expect(invalid.stderr).To(HavePrefix("factory bridge: "))
		}
	})

	// per specs/001-go-runtime-conversion.md:95
	DescribeTable("refuses an unusable explicitly selected runtime without fallback", func(kind string) {
		root, cwd := fixture()
		path := filepath.Join(cwd, "selected runtime")
		switch kind {
		case "empty":
			path = ""
		case "relative":
			path = "./factory"
			writeFixture(filepath.Join(cwd, "factory"), []byte("#!/bin/sh\nprintf wrong-runtime\n"), 0755)
		case "nonexecutable":
			writeFixture(path, []byte("#!/bin/sh\nprintf wrong-runtime\n"), 0644)
		case "directory":
			Expect(os.Mkdir(path, 0755)).To(Succeed())
		}
		result := shimReader(root, cwd, map[string]string{"FACTORY_RUNTIME_BINARY": path, "PATH": root + ":/bin:/usr/bin"}, "role_tier", "reviewer")
		Expect(result).To(Equal(cliResult{"", "factory bridge: FACTORY_RUNTIME_BINARY must name an executable absolute regular file\n", 2}))
	}, Entry("empty", "empty"), Entry("relative", "relative"), Entry("missing", "missing"), Entry("nonexecutable", "nonexecutable"), Entry("directory", "directory"))

	// per specs/001-go-runtime-conversion.md:95
	DescribeTable("retains role helper missing-operand behavior with ordinary and nounset callers", func(function string, args []string, nounset bool) {
		root, cwd := fixture()
		env := map[string]string{}
		if nounset {
			env["FACTORY_ACCEPTANCE_NOUNSET"] = "1"
		}
		baseline := historicalReader(root, cwd, baselineCommit, function, env, args...)
		other := historicalReader(root, cwd, releaseReaderBaseline, function, env, args...)
		Expect(other.status).To(Equal(baseline.status))
		Expect(other.stdout).To(Equal(baseline.stdout))
		if nounset {
			Expect(baseline.status).NotTo(Equal(0))
			Expect(baseline.stderr).To(ContainSubstring("unbound variable"))
		} else {
			Expect(baseline.stderr).To(BeEmpty())
		}
		candidate := shimReader(root, cwd, env, function, args...)
		Expect(candidate.status).To(Equal(baseline.status))
		Expect(candidate.stdout).To(Equal(baseline.stdout))
		if nounset {
			Expect(candidate.stderr).To(ContainSubstring("unbound variable"))
		} else {
			Expect(candidate.stderr).To(BeEmpty())
		}
	}, Entry("tier no role", "role_tier", nil, false), Entry("tier no role under nounset", "role_tier", nil, true),
		Entry("resolve no operands", "resolve_tier", nil, false), Entry("resolve no operands under nounset", "resolve_tier", nil, true),
		Entry("resolve no tier", "resolve_tier", []string{"standard"}, false), Entry("resolve no tier under nounset", "resolve_tier", []string{"standard"}, true))

	// per specs/001-go-runtime-conversion.md:95
	DescribeTable("preserves missing required configuration operands under nounset", func(function string) {
		root, cwd := fixture()
		path := filepath.Join(cwd, "factory.yaml")
		writeFixture(path, []byte("key: existing\n"), 0600)
		env := map[string]string{"FACTORY_ACCEPTANCE_NOUNSET": "1", "FACTORY_CONFIG": path}
		baseline := historicalReader(root, cwd, baselineCommit, function, env)
		Expect(baseline.status).NotTo(Equal(0))
		Expect(baseline.stderr).To(ContainSubstring("unbound variable"))
		other := historicalReader(root, cwd, releaseReaderBaseline, function, env)
		Expect(other.status).To(Equal(baseline.status))
		candidate := shimReader(root, cwd, env, function)
		Expect(candidate.status).To(Equal(baseline.status))
		Expect(candidate.stdout).To(BeEmpty())
		Expect(candidate.stderr).To(ContainSubstring("unbound variable"))
	}, Entry("get key", "factory_config_get"), Entry("has key", "factory_config_has"))

	// per specs/001-go-runtime-conversion.md:95
	It("keeps the get default optional for nounset callers", func() {
		root, cwd := fixture()
		env := map[string]string{"FACTORY_ACCEPTANCE_NOUNSET": "1"}
		Expect(historicalReader(root, cwd, baselineCommit, "factory_config_get", env, "missing")).To(Equal(cliResult{"", "", 0}))
		Expect(shimReader(root, cwd, env, "factory_config_get", "missing")).To(Equal(cliResult{"", "", 0}))
	})
})
