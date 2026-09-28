package acceptance_test

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func nativeInitFixture() (string, string, []string) {
	GinkgoHelper()
	base, err := os.MkdirTemp("", "factory-native-init-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, base)
	base, err = filepath.EvalSymlinks(base)
	Expect(err).NotTo(HaveOccurred())
	source := filepath.Join(base, "template")
	target := filepath.Join(base, "adopter")
	archiveCommand := exec.Command("git", "archive", "ef5a06ae080c074241a783eb151ba8bb179ff6bd") // #nosec G204 -- fixed local revision and command, no shell or external input.
	archiveCommand.Dir = ".."
	archive, err := archiveCommand.Output()
	Expect(err).NotTo(HaveOccurred())
	reader := tar.NewReader(bytes.NewReader(archive))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		Expect(err).NotTo(HaveOccurred())
		if header.Typeflag == tar.TypeDir || header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		if header.Typeflag == tar.TypeSymlink {
			path := filepath.Join(source, header.Name) // #nosec G305 -- immutable local Git archive has only repository-relative tracked names.
			Expect(os.MkdirAll(filepath.Dir(path), 0700)).To(Succeed())
			Expect(os.Symlink(header.Linkname, path)).To(Succeed())
			continue
		}
		Expect(header.Typeflag).To(Equal(byte(tar.TypeReg)), "fixture archive type: %s", header.Name)
		data, readErr := io.ReadAll(reader)
		Expect(readErr).NotTo(HaveOccurred())
		writeFixture(filepath.Join(source, header.Name), data, os.FileMode(header.Mode&0777)) // #nosec G305 -- trusted local Git archive, no external archive or traversal names.
	}
	writeFixture(filepath.Join(source, "factory-go"), candidateBinary, 0700)
	original, err := os.ReadFile(filepath.Join(source, "scripts/factory-init.sh"))
	Expect(err).NotTo(HaveOccurred())
	writeFixture(filepath.Join(source, "scripts/factory-init-legacy.sh"), original, 0700)
	writeFixture(filepath.Join(source, "scripts/factory-init.sh"), []byte("#!/bin/sh\nprintf LEGACY_INIT_CALLED >&2\nexit 93\n"), 0700)
	for _, path := range []string{"scripts/selftest/run.sh", "scripts/prereq-check.sh", "scripts/sync-opencode.sh", "scripts/sync-claude.sh", "scripts/sync-codex.sh", "scripts/factory-review-lane.sh"} {
		writeFixture(filepath.Join(source, path), []byte("#!/bin/sh\nexit 0\n"), 0700)
	}
	bin := filepath.Join(base, "bin")
	writeFixture(filepath.Join(bin, "npm"), []byte("#!/bin/sh\nexit 0\n"), 0700)
	var environment []string
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "FACTORY_") && !strings.HasPrefix(value, "GIT_") && !strings.HasPrefix(value, "PATH=") {
			environment = append(environment, value)
		}
	}
	environment = append(environment, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GORACE=atexit_sleep_ms=0")
	return source, target, environment
}
func nativeInitAnswers(confirm string) string {
	return strings.Join([]string{"Native Project", "native-project", "@example", "native-user", "", "", "", "inherit", "standard", "n", confirm, ""}, "\n")
}
func nativeInitRun(source, target, input string, environment, args []string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(source, "factory-go"), append([]string{"init", target}, args...)...) // #nosec G204 -- isolated compiled CLI fixture path and literal argv, no shell.
	command.Dir = filepath.Dir(source)
	command.Env = environment
	command.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred())
	status := 0
	if err != nil {
		var exitError *exec.ExitError
		Expect(errors.As(err, &exitError)).To(BeTrue(), "%v", err)
		status = exitError.ExitCode()
	}
	return cliResult{stdout.String(), stderr.String(), status}
}

var _ = Describe("Native Go init core", func() {
	// per docs/adr/0085-go-native-init.md:17
	It("initializes ordinary assets without invoking the poisoned legacy implementation", func() {
		source, target, environment := nativeInitFixture()
		out := nativeInitRun(source, target, nativeInitAnswers("y"), environment, []string{"--pack", "none"})
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stderr).NotTo(ContainSubstring("LEGACY_INIT_CALLED"))
		config, err := os.ReadFile(filepath.Join(target, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(config)).To(ContainSubstring("project_name: native-project"))
		Expect(string(config)).To(ContainSubstring("budget_enabled: false"))
		Expect(string(config)).To(ContainSubstring("loop_enabled: false"))
		original, err := os.ReadFile(filepath.Join(source, "factory"))
		Expect(err).NotTo(HaveOccurred())
		installed, err := os.ReadFile(filepath.Join(target, "factory"))
		Expect(err).NotTo(HaveOccurred())
		Expect(installed).To(Equal(original))
	})
	// per docs/adr/0085-go-native-init.md:99
	It("honors explicit decline without creating the target", func() {
		source, target, environment := nativeInitFixture()
		out := nativeInitRun(source, target, nativeInitAnswers("n"), environment, []string{"--pack", "none"})
		Expect(out.status).To(Equal(1), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("Aborted"))
		_, err := os.Stat(target)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0085-go-native-init.md:39
	It("treats EOF as unconfirmed without invoking scripts or creating the target", func() {
		source, target, environment := nativeInitFixture()
		out := nativeInitRun(source, target, "", environment, []string{"--pack", "none"})
		Expect(out.status).To(Equal(1), "%+v", out)
		Expect(out.stderr).NotTo(ContainSubstring("LEGACY_INIT_CALLED"))
		_, err := os.Stat(target)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

func nativeInitInput(fields, versions []string, confirm string) string {
	defaults := []string{"Native Project", "native-project", "@example", "native-user", "", "", "", "inherit", "standard", "n"}
	copy(defaults, fields)
	return strings.Join(append(append(defaults, versions...), confirm, ""), "\n")
}
func nativeInitLegacy(source, target, input string, environment, args []string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "bash", append([]string{filepath.Join(source, "scripts/factory-init-legacy.sh"), target}, args...)...) // #nosec G204 -- immutable fixture script, test-owned target, literal argv; no shell expression.
	command.Dir = filepath.Dir(source)
	command.Env = environment
	command.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred())
	status := 0
	if err != nil {
		var exitError *exec.ExitError
		Expect(errors.As(err, &exitError)).To(BeTrue())
		status = exitError.ExitCode()
	}
	return cliResult{stdout.String(), stderr.String(), status}
}
func nativeInitArtifacts(root string) map[string]string {
	GinkgoHelper()
	result := map[string]string{}
	Expect(filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			link, readErr := os.Readlink(path)
			if readErr != nil {
				return readErr
			}
			result[relative] = "symlink:" + link
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G122 -- test-owned quiescent tree inspected after child exit; links handled above.
		if err != nil {
			return err
		}
		result[relative] = fmt.Sprintf("%o:%s", info.Mode().Perm(), hex.EncodeToString(data))
		return nil
	})).To(Succeed())
	return result
}

var _ = Describe("Native Go init artifact compatibility", func() {
	// per docs/adr/0085-go-native-init.md:17
	DescribeTable("matches immutable generated artifacts for packs and model profiles", func(packs, build, hints, provider, profile string) {
		source, target, environment := nativeInitFixture()
		oracle := target + "-legacy"
		args := []string{"--pack", packs}
		if build != "" {
			args = append(args, "--java-build-tool", build)
		}
		for _, destination := range []string{target, oracle} {
			Expect(os.Mkdir(destination, 0700)).To(Succeed())
			if strings.Contains(hints, "pom") {
				writeFixture(filepath.Join(destination, "pom.xml"), []byte("<project>\r\n<!-- adopter-owned -->\r\n</project>\r\n"), 0600)
			}
			for _, wrapper := range []string{"gradlew", "mvnw"} {
				if strings.Contains(hints, wrapper) {
					writeFixture(filepath.Join(destination, wrapper), []byte("#!/bin/sh\nexit 0\n"), 0700)
				}
			}
		}
		fields := []string{"Native Project", "native-project", "@example", "native-user", "internal/core", "docs", "DISC_", provider, profile, "n"}
		var versions []string
		for _, kind := range []string{"go", "java", "typescript"} {
			if strings.Contains(packs, kind) {
				version := map[string]string{"go": "1.26", "java": "25", "typescript": "24"}[kind]
				versions = append(versions, version)
			}
		}
		input := nativeInitInput(fields, versions, "y")
		baseline := nativeInitLegacy(source, oracle, input, environment, args)
		Expect(baseline.status).To(BeZero(), "%+v", baseline)
		actual := nativeInitRun(source, target, input, environment, args)
		Expect(actual.status).To(BeZero(), "%+v", actual)
		Expect(nativeInitArtifacts(target)).To(Equal(nativeInitArtifacts(oracle)))
	}, Entry("plain inherit", "none", "", "", "inherit", "standard"), Entry("Go", "go", "", "", "inherit", "standard"), Entry("TypeScript", "typescript", "", "", "inherit", "standard"), Entry("Java default Gradle", "java", "", "", "inherit", "standard"), Entry("Maven POM detection", "java", "", "pom", "inherit", "standard"), Entry("Maven wrapper", "java", "", "pom mvnw", "inherit", "standard"), Entry("Gradle wrapper wins", "java", "", "pom mvnw gradlew", "inherit", "standard"), Entry("explicit Maven wins", "java", "maven", "pom mvnw gradlew", "inherit", "standard"), Entry("polyglot dedupe", "go,typescript,java,go", "", "pom", "inherit", "standard"), Entry("OpenRouter economy", "none", "", "", "openrouter", "Economy"), Entry("Anthropic standard", "none", "", "", "anthropic", "standard"), Entry("OpenAI economy", "none", "", "", "openai", "economy"), Entry("other provider", "none", "", "", "other", "standard"), Entry("invalid profile defaults", "none", "", "", "inherit", "UNRECOGNIZED"))
	// per docs/adr/0085-go-native-init.md:17
	It("does not require a colocated legacy init script", func() {
		source, target, environment := nativeInitFixture()
		Expect(os.Remove(filepath.Join(source, "scripts/factory-init.sh"))).To(Succeed())
		out := nativeInitRun(source, target, nativeInitAnswers("y"), environment, []string{"--pack", "none"})
		Expect(out.status).To(BeZero(), "%+v", out)
	})
	// per docs/adr/0085-go-native-init.md:35
	It("preserves repeated pack options and last unrecognized target selection", func() {
		source, target, environment := nativeInitFixture()
		actualTarget := target + "-actual"
		out := nativeInitRun(source, target, nativeInitInput(nil, []string{"1.26", "24"}, "Yes"), environment, []string{"--pack=go", "--pack", "typescript,go", actualTarget})
		Expect(out.status).To(BeZero(), "%+v", out)
		_, err := os.Stat(target)
		Expect(os.IsNotExist(err)).To(BeTrue())
		data, err := os.ReadFile(filepath.Join(actualTarget, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`language_packs: "go typescript"`))
	})
})

var _ = Describe("Native Go init admission and safety", func() {
	// per docs/adr/0085-go-native-init.md:100
	DescribeTable("refuses invalid inputs without creating an installation", func(kind string) {
		source, target, environment := nativeInitFixture()
		args := []string{"--pack", "none"}
		input := nativeInitAnswers("y")
		switch kind {
		case "pack traversal":
			args = []string{"--pack", "../go"}
		case "unknown pack":
			args = []string{"--pack", "unknown"}
		case "java invalid":
			args = []string{"--java-build-tool", "unknown"}
		case "java missing":
			args = []string{"--java-build-tool"}
		case "quote":
			input = nativeInitInput([]string{`bad"name`}, nil, "y")
		case "carriage return":
			input = nativeInitInput([]string{"bad\rname"}, nil, "y")
		case "nul":
			input = nativeInitInput([]string{"bad\x00name"}, nil, "y")
		}
		out := nativeInitRun(source, target, input, environment, args)
		if strings.Contains(kind, "pack") || strings.HasPrefix(kind, "java ") {
			Expect(out.status).To(Equal(2), "%+v", out)
		} else {
			Expect(out.status).To(Equal(1), "%+v", out)
		}
		_, err := os.Stat(target)
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("traversal pack", "pack traversal"), Entry("unknown pack", "unknown pack"), Entry("invalid Java choice", "java invalid"), Entry("missing Java value", "java missing"), Entry("unrepresentable quote", "quote"), Entry("embedded carriage return", "carriage return"), Entry("NUL", "nul"))
	// per docs/adr/0085-go-native-init.md:85
	DescribeTable("refuses missing required assets and unsafe sources before target writes", func(kind string) {
		source, target, environment := nativeInitFixture()
		path := filepath.Join(source, "opencode.json")
		switch kind {
		case "missing":
			Expect(os.Remove(path)).To(Succeed())
		case "symlink":
			Expect(os.Rename(path, path+".real")).To(Succeed())
			Expect(os.Symlink(path+".real", path)).To(Succeed())
		case "fifo":
			Expect(os.Remove(path)).To(Succeed())
			Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
		case "hardlink":
			Expect(os.Link(path, path+".linked")).To(Succeed())
		case "native dispatcher":
			writeFixture(filepath.Join(source, "factory"), candidateBinary, 0700)
		}
		out := nativeInitRun(source, target, nativeInitAnswers("y"), environment, []string{"--pack", "none"})
		Expect(out.status).NotTo(BeZero(), "%+v", out)
		_, err := os.Stat(target)
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("missing required JSON", "missing"), Entry("source symlink", "symlink"), Entry("source FIFO", "fifo"), Entry("source hardlink", "hardlink"), Entry("unqualified dispatcher activation", "native dispatcher"))
	// per docs/adr/0085-go-native-init.md:87
	DescribeTable("validates every existing planned destination before overwriting any file", func(kind string) {
		source, target, environment := nativeInitFixture()
		Expect(os.Mkdir(target, 0700)).To(Succeed())
		sentinel := []byte("ADOPTER_CONFIG_UNCHANGED\n")
		writeFixture(filepath.Join(target, "factory.yaml"), sentinel, 0600)
		outside := target + "-outside"
		writeFixture(outside, []byte("PRIVATE_OUTSIDE_UNCHANGED"), 0600)
		path := filepath.Join(target, "opencode.json")
		switch kind {
		case "symlink":
			Expect(os.Symlink(outside, path)).To(Succeed())
		case "hardlink":
			Expect(os.Link(outside, path)).To(Succeed())
		case "fifo":
			Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
		case "claude ancestor":
			Expect(os.Mkdir(target+"-claude", 0700)).To(Succeed())
			Expect(os.Symlink(target+"-claude", filepath.Join(target, ".claude"))).To(Succeed())
		case "mcp output":
			Expect(os.Symlink(outside, filepath.Join(target, ".mcp.json"))).To(Succeed())
		case "ancestor":
			Expect(os.Mkdir(target+"-scripts", 0700)).To(Succeed())
			Expect(os.Symlink(target+"-scripts", filepath.Join(target, "scripts"))).To(Succeed())
		}
		out := nativeInitRun(source, target, nativeInitAnswers("y"), environment, []string{"--pack", "none"})
		Expect(out.status).NotTo(BeZero(), "%+v", out)
		data, err := os.ReadFile(filepath.Join(target, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(data).To(Equal(sentinel))
		data, err = os.ReadFile(outside)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("PRIVATE_OUTSIDE_UNCHANGED"))
		entries, err := os.ReadDir(target)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(2))
	}, Entry("destination symlink", "symlink"), Entry("destination hardlink", "hardlink"), Entry("destination FIFO", "fifo"), Entry("destination ancestor symlink", "ancestor"), Entry("generated Claude ancestor", "claude ancestor"), Entry("generated MCP output", "mcp output"))
	// per docs/adr/0085-go-native-init.md:98
	DescribeTable("refuses source-target overlap without mutating the template", func(kind string) {
		source, _, environment := nativeInitFixture()
		var target string
		if kind == "same" {
			target = source
		} else {
			target = filepath.Join(source, "nested-adopter")
		}
		before := nativeInitArtifacts(source)
		out := nativeInitRun(source, target, nativeInitAnswers("y"), environment, []string{"--pack", "none"})
		Expect(out.status).NotTo(BeZero(), "%+v", out)
		Expect(nativeInitArtifacts(source)).To(Equal(before))
	}, Entry("same tree", "same"), Entry("nested inside source", "nested"))
	// per docs/adr/0085-go-native-init.md:19
	It("refuses a nonexecutable Maven wrapper before writing any assets", func() {
		source, target, environment := nativeInitFixture()
		writeFixture(filepath.Join(target, "mvnw"), []byte("#!/bin/sh\nexit 0\n"), 0600)
		before := nativeInitArtifacts(target)
		out := nativeInitRun(source, target, nativeInitInput(nil, []string{"25"}, "y"), environment, []string{"--pack", "java"})
		Expect(out.status).To(Equal(2))
		Expect(out.stdout + out.stderr).To(ContainSubstring("mvnw"))
		Expect(nativeInitArtifacts(target)).To(Equal(before))
	})
	// per docs/adr/0085-go-native-init.md:43
	It("substitutes metacharacters as literal data without shell or sed evaluation", func() {
		source, target, environment := nativeInitFixture()
		name := "Literal & | $(touch NEVER_EXECUTED) " + "`touch NEVER_EXECUTED`"
		original, readErr := os.ReadFile(filepath.Join(source, "AGENTS.md"))
		Expect(readErr).NotTo(HaveOccurred())
		writeFixture(filepath.Join(source, "AGENTS.md"), append(original, []byte("\nProject: __PROJECT_NAME__\n")...), 0600)
		out := nativeInitRun(source, target, nativeInitInput([]string{name}, nil, "y"), environment, []string{"--pack", "none"})
		Expect(out.status).To(BeZero(), "%+v", out)
		data, err := os.ReadFile(filepath.Join(target, "AGENTS.md"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(name))
		_, err = os.Stat(filepath.Join(filepath.Dir(source), "NEVER_EXECUTED"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		_, err = os.Stat(filepath.Join(target, "NEVER_EXECUTED"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

var _ = Describe("Native Go init preservation", func() {
	// per docs/adr/0085-go-native-init.md:101
	It("keeps adopter files and creates exclusive old-configuration backups across reruns", func() {
		source, target, environment := nativeInitFixture()
		files := map[string]string{"README.md": "adopter __PROJECT_NAME__ & retained\r\n", "Makefile": "custom-target:\n\t@echo retained\n", ".gitignore": "private-company-files/\n", "factory.yaml": "old_private_config: retained\n"}
		for path, value := range files {
			writeFixture(filepath.Join(target, path), []byte(value), 0600)
		}
		out := nativeInitRun(source, target, nativeInitAnswers("y"), environment, []string{"--pack", "none"})
		Expect(out.status).To(BeZero(), "%+v", out)
		before := nativeInitArtifacts(target)
		out = nativeInitRun(source, target, nativeInitAnswers("y"), environment, []string{"--pack", "none"})
		Expect(out.status).To(BeZero(), "%+v", out)
		after := nativeInitArtifacts(target)
		Expect(after).To(HaveLen(len(before)), "changed artifact paths: %v", initArtifactChanges(before, after))
		Expect(after).To(Equal(before), "changed artifact paths: %v", initArtifactChanges(before, after))
		for path, value := range before {
			if strings.Contains(path, ".factory-backup.") {
				Expect(after).To(HaveKeyWithValue(path, value))
			}
		}
		readme, err := os.ReadFile(filepath.Join(target, "README.md"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(readme)).To(Equal(files["README.md"]))
		for path, marker := range map[string]string{"Makefile": "# BEGIN factory targets", ".gitignore": "# BEGIN factory"} {
			data, readErr := os.ReadFile(filepath.Join(target, path))
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring(files[path]))
			Expect(strings.Count(string(data), marker)).To(Equal(1))
		}
		backups, err := filepath.Glob(filepath.Join(target, "factory.yaml.factory-backup.*"))
		Expect(err).NotTo(HaveOccurred())
		Expect(backups).To(HaveLen(1))
		found := false
		for _, path := range backups {
			info, statErr := os.Stat(path)
			Expect(statErr).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
			data, readErr := os.ReadFile(path)
			Expect(readErr).NotTo(HaveOccurred())
			if string(data) == files["factory.yaml"] {
				found = true
			}
		}
		Expect(found).To(BeTrue(), "old factory.yaml was not preserved")
	})
	// per docs/adr/0085-go-native-init.md:32
	It("tolerates missing optional evaluation and workflow assets", func() {
		source, target, environment := nativeInitFixture()
		for _, path := range []string{"wiki/README.md", "specs/TEMPLATE.md", "eval/README.md", "workflows/eval-fanout.md", "scripts/golden-task-eval.sh"} {
			Expect(os.Remove(filepath.Join(source, path))).To(Succeed())
		}
		out := nativeInitRun(source, target, nativeInitAnswers("y"), environment, []string{"--pack", "none"})
		Expect(out.status).To(BeZero(), "%+v", out)
	})
})

var _ = Describe("Native Go init external boundaries", func() {
	// per docs/adr/0085-go-native-init.md:62
	DescribeTable("distinguishes advisory failures from mandatory gate attestation", func(which string, status int) {
		source, target, environment := nativeInitFixture()
		trace := filepath.Join(filepath.Dir(source), "external-trace")
		scripts := map[string]string{"npm": filepath.Join(filepath.Dir(source), "bin/npm"), "sync": filepath.Join(source, "scripts/sync-opencode.sh"), "review": filepath.Join(source, "scripts/factory-review-lane.sh"), "selftest": filepath.Join(source, "scripts/selftest/run.sh")}
		for name, path := range scripts {
			code := "0"
			if name == which {
				code = "7"
			}
			script := "#!/bin/sh\nprintf '%s|%s|%s\\n' '" + name + "' \"$PWD\" \"$*\" >> '" + trace + "'\n"
			if name == "npm" {
				script += "printf 'NPM_PRIVATE_DIAGNOSTIC\\n' >&2\n"
			}
			script += "exit " + code + "\n"
			writeFixture(path, []byte(script), 0700)
		}
		fields := []string{"Native Project", "native-project", "@example", "native-user", "", "", "", "inherit", "standard", "y"}
		out := nativeInitRun(source, target, nativeInitInput(fields, nil, "y"), environment, []string{"--pack", "none"})
		Expect(out.status).To(Equal(status), "%+v", out)
		Expect(out.stderr).NotTo(ContainSubstring("NPM_PRIVATE_DIAGNOSTIC"))
		data, err := os.ReadFile(trace)
		Expect(err).NotTo(HaveOccurred())
		text := string(data)
		Expect(text).To(ContainSubstring("npm|" + filepath.Join(target, ".opencode") + "|install"))
		Expect(text).To(ContainSubstring("sync|" + target + "|"))
		Expect(text).To(ContainSubstring("selftest|" + target + "|"))
		Expect(text).To(ContainSubstring("review|" + target + "|enable"))
		if which == "selftest" {
			Expect(out.stdout + out.stderr).To(ContainSubstring("NOT VERIFIED"))
			Expect(out.stdout).NotTo(ContainSubstring("gates proven"))
		} else {
			Expect(strings.ToLower(out.stdout + out.stderr)).To(Or(ContainSubstring("warning"), ContainSubstring("failed")))
		}
	}, Entry("npm advisory", "npm", 0), Entry("sync advisory", "sync", 0), Entry("review advisory", "review", 0), Entry("selftest mandatory", "selftest", 1))
})

var _ = Describe("Native Go init shared synchronization", func() {
	// per docs/adr/0085-go-native-init.md:62
	DescribeTable("produces the same synchronized harness artifacts as the immutable initializer", func(provider, profile string) {
		source, target, environment := nativeInitFixture()
		for _, name := range []string{"sync-opencode.sh", "sync-claude.sh", "sync-codex.sh"} {
			command := exec.Command("git", "show", "ef5a06ae080c074241a783eb151ba8bb179ff6bd:scripts/"+name) // #nosec G204 -- fixed immutable revision and table-owned script names.
			command.Dir = ".."
			data, err := command.Output()
			Expect(err).NotTo(HaveOccurred())
			writeFixture(filepath.Join(source, "scripts", name), data, 0700)
		}
		input := nativeInitInput([]string{"Native Project", "native-project", "@example", "native-user", "", "", "", provider, profile, "n"}, nil, "y")
		oracle := target + "-oracle"
		expected := nativeInitLegacy(source, oracle, input, environment, []string{"--pack", "none"})
		Expect(expected.status).To(BeZero(), "oracle %+v", expected)
		actual := nativeInitRun(source, target, input, environment, []string{"--pack", "none"})
		Expect(actual.status).To(BeZero(), "candidate %+v", actual)
		Expect(nativeInitArtifacts(target)).To(Equal(nativeInitArtifacts(oracle)))
	}, Entry("inherit", "inherit", "standard"), Entry("economy", "openai", "economy"), Entry("standard", "anthropic", "standard"))
})

var _ = Describe("Native Go init input and child ownership", func() {
	// per docs/adr/0085-go-native-init.md:46
	It("refuses an oversized unterminated answer while the input pipe remains open", func() {
		source, target, environment := nativeInitFixture()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, filepath.Join(source, "factory-go"), "init", target, "--pack", "none") // #nosec G204 -- isolated compiled CLI, fixed literal arguments.
		command.Env = environment
		command.Dir = filepath.Dir(source)
		reader, writer, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		defer reader.Close()
		defer writer.Close()
		command.Stdin = reader
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		Expect(command.Start()).To(Succeed())
		done := make(chan error, 1)
		go func() { defer close(done); done <- command.Wait() }()
		written := make(chan struct{})
		go func() { defer close(written); _, _ = writer.Write(bytes.Repeat([]byte("x"), 128<<10)) }()
		DeferCleanup(func() {
			cancel()
			_ = writer.Close()
			Eventually(done, 3*time.Second).Should(BeClosed())
			Eventually(written, 3*time.Second).Should(BeClosed())
		})
		var waitErr error
		Eventually(done, 3*time.Second).Should(Receive(&waitErr))
		Expect(waitErr).To(HaveOccurred())
		Expect(ctx.Err()).NotTo(HaveOccurred())
		Expect(stdout.String() + stderr.String()).To(ContainSubstring("too large"))
		_, err = os.Stat(target)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0085-go-native-init.md:76
	DescribeTable("cancels its observed validation child without claiming successful installation", func(signal syscall.Signal) {
		source, target, environment := nativeInitFixture()
		marker := filepath.Join(filepath.Dir(source), "validation-pid")
		environment = append(environment, "INIT_PID_MARKER="+marker)
		writeFixture(filepath.Join(source, "scripts/selftest/run.sh"), []byte("#!/bin/sh\nprintf '%s' \"$$\" > \"$INIT_PID_MARKER\"\nexec /bin/sleep 30\n"), 0700)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		command := exec.CommandContext(ctx, filepath.Join(source, "factory-go"), "init", target, "--pack", "none") // #nosec G204 -- isolated compiled CLI, fixed literal arguments.
		command.Env = environment
		command.Dir = filepath.Dir(source)
		command.Stdin = strings.NewReader(nativeInitAnswers("y"))
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		Expect(command.Start()).To(Succeed())
		done := make(chan error, 1)
		go func() { defer close(done); done <- command.Wait() }()
		pid := 0
		DeferCleanup(func() {
			if pid > 0 {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			}
			cancel()
			Eventually(done, 3*time.Second).Should(BeClosed())
		})
		var raw []byte
		Eventually(func() error { var err error; raw, err = os.ReadFile(marker); return err }, 5*time.Second).Should(Succeed())
		var err error
		pid, err = strconv.Atoi(string(raw))
		Expect(err).NotTo(HaveOccurred())
		Expect(command.Process.Signal(signal)).To(Succeed())
		var waitErr error
		Eventually(done, 7*time.Second).Should(Receive(&waitErr))
		Expect(waitErr).To(HaveOccurred())
		var exited *exec.ExitError
		Expect(errors.As(waitErr, &exited)).To(BeTrue())
		Expect(exited.ExitCode()).To(Equal(1))
		Expect(stderr.String()).To(ContainSubstring("canceled"))
		Expect(ctx.Err()).NotTo(HaveOccurred())
		Expect(stdout.String()).NotTo(ContainSubstring("gates proven"))
		Expect(syscall.Kill(pid, 0)).To(Equal(syscall.ESRCH))
		pid = 0
	}, Entry("SIGINT", syscall.SIGINT), Entry("SIGTERM", syscall.SIGTERM))
})

var _ = Describe("Native Go init installed attestation", func() {
	// per docs/adr/0085-go-native-init.md:62
	It("runs real installed synchronization and gate selftests without contacting npm", func() {
		source, target, environment := nativeInitFixture()
		writeFixture(filepath.Join(filepath.Dir(source), "bin/gh"), []byte("#!/bin/sh\nexit 1\n"), 0700)
		for _, name := range []string{"sync-opencode.sh", "sync-claude.sh", "sync-codex.sh", "factory-review-lane.sh", "prereq-check.sh", "selftest/run.sh"} {
			command := exec.Command("git", "show", "ef5a06ae080c074241a783eb151ba8bb179ff6bd:scripts/"+name) // #nosec G204 -- fixed immutable revision and test-owned script names.
			command.Dir = ".."
			data, err := command.Output()
			Expect(err).NotTo(HaveOccurred())
			writeFixture(filepath.Join(source, "scripts", name), data, 0700)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		command := exec.CommandContext(ctx, filepath.Join(source, "factory-go"), "init", target, "--pack", "none") // #nosec G204 -- isolated compiled CLI and fixed literal arguments.
		command.Env = environment
		command.Dir = filepath.Dir(source)
		command.Stdin = strings.NewReader(nativeInitAnswers("y"))
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		err := command.Run()
		Expect(ctx.Err()).NotTo(HaveOccurred())
		Expect(err).NotTo(HaveOccurred(), "%s\n%s", stdout.String(), stderr.String())
		Expect(stdout.String()).To(ContainSubstring("gates proven"))
		Expect(stdout.String()).To(ContainSubstring("deny implementer on _test.go"))
		Expect(stdout.String()).NotTo(ContainSubstring("NOT VERIFIED"))
	})
	// per docs/adr/0085-go-native-init.md:19
	It("uses effective owner permissions when validating an existing Maven wrapper", func() {
		if os.Geteuid() == 0 {
			Skip("effective owner execution refusal requires a non-root caller")
		}
		source, target, environment := nativeInitFixture()
		wrapper := filepath.Join(target, "mvnw")
		writeFixture(wrapper, []byte("#!/bin/sh\nexit 0\n"), 0600)
		Expect(os.Chmod(wrapper, 0641)).To(Succeed())
		before := nativeInitArtifacts(target)
		actual := nativeInitRun(source, target, nativeInitInput(nil, []string{"25"}, "y"), environment, []string{"--pack", "java"})
		Expect(actual.status).To(Equal(2), "%+v", actual)
		Expect(nativeInitArtifacts(target)).To(Equal(before))
	})
})

type initBannerWriter struct {
	buffer bytes.Buffer
	once   sync.Once
	ready  chan struct{}
}

func (w *initBannerWriter) Write(p []byte) (int, error) {
	n, err := w.buffer.Write(p)
	if bytes.Contains(w.buffer.Bytes(), []byte("Software Factory Template Setup")) {
		w.once.Do(func() { close(w.ready) })
	}
	return n, err
}

var _ = Describe("Native Go init blocked prompt cancellation", func() {
	// per docs/adr/0085-go-native-init.md:76
	DescribeTable("stops waiting for an inherited answer without creating the target", func(signal syscall.Signal) {
		source, target, environment := nativeInitFixture()
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		command := exec.CommandContext(ctx, filepath.Join(source, "factory-go"), "init", target, "--pack", "none") // #nosec G204 -- isolated compiled CLI and fixed literal arguments.
		command.Env = environment
		command.Dir = filepath.Dir(source)
		reader, writer, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		defer reader.Close()
		defer writer.Close()
		command.Stdin = reader
		output := &initBannerWriter{ready: make(chan struct{})}
		var diagnostic bytes.Buffer
		command.Stdout = output
		command.Stderr = &diagnostic
		Expect(command.Start()).To(Succeed())
		done := make(chan error, 1)
		go func() { defer close(done); done <- command.Wait() }()
		DeferCleanup(func() { cancel(); _ = writer.Close(); Eventually(done, 3*time.Second).Should(BeClosed()) })
		Eventually(output.ready, 4*time.Second).Should(BeClosed())
		Expect(command.Process.Signal(signal)).To(Succeed())
		var waitErr error
		Eventually(done, 4*time.Second).Should(Receive(&waitErr))
		Expect(waitErr).To(HaveOccurred())
		var exited *exec.ExitError
		Expect(errors.As(waitErr, &exited)).To(BeTrue())
		Expect(exited.ExitCode()).To(Equal(1))
		Expect(ctx.Err()).NotTo(HaveOccurred())
		Expect(diagnostic.String()).To(ContainSubstring("canceled"))
		_, err = os.Stat(target)
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("SIGINT", syscall.SIGINT), Entry("SIGTERM", syscall.SIGTERM))
})

func initArtifactChanges(before, after map[string]string) []string {
	var changed []string
	for path, value := range after {
		if before[path] != value {
			changed = append(changed, path)
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			changed = append(changed, path)
		}
	}
	return changed
}

var _ = Describe("Native Go init child environment confinement", func() {
	// per docs/adr/0085-go-native-init.md:65
	DescribeTable("keeps review configuration and workflows inside the selected target", func(explicitGit bool) {
		source, target, environment := nativeInitFixture()
		parent := filepath.Dir(source)
		git := exec.Command("git", "init", "-q", parent)
		git.Env = environment
		Expect(git.Run()).To(Succeed())
		outside := filepath.Join(parent, "factory.yaml")
		sentinel := []byte("review_lane: off\nproject_name: outside-sentinel\n")
		writeFixture(outside, sentinel, 0600)
		environment = append(environment, "FACTORY_CONFIG="+outside)
		if explicitGit {
			environment = append(environment, "GIT_DIR="+filepath.Join(parent, ".git"), "GIT_WORK_TREE="+parent)
		}
		writeFixture(filepath.Join(parent, "bin/gh"), []byte("#!/bin/sh\nexit 1\n"), 0700)
		command := exec.Command("git", "show", "ef5a06ae080c074241a783eb151ba8bb179ff6bd:scripts/factory-review-lane.sh")
		command.Dir = ".."
		data, err := command.Output()
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(source, "scripts/factory-review-lane.sh"), data, 0700)
		fields := []string{"Native Project", "native-project", "@example", "native-user", "", "", "", "openai", "standard", "y"}
		actual := nativeInitRun(source, target, nativeInitInput(fields, nil, "y"), environment, []string{"--pack", "none"})
		Expect(actual.status).To(BeZero(), "%+v", actual)
		data, err = os.ReadFile(outside)
		Expect(err).NotTo(HaveOccurred())
		Expect(data).To(Equal(sentinel))
		_, err = os.Stat(filepath.Join(parent, ".github/workflows/adversarial-review.yml"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		data, err = os.ReadFile(filepath.Join(target, ".github/workflows/adversarial-review.yml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(HavePrefix("# Managed by: factory review-lane."))
	}, Entry("parent repository discovery", false), Entry("explicit inherited Git routing", true))
})

var _ = Describe("Native Go init terminal input", func() {
	// per docs/adr/0085-go-native-init.md:39
	DescribeTable("keeps terminal pack eligibility aligned with the selected output surface", func(redirect bool) {
		source, target, environment := nativeInitFixture()
		// A disposable PTY supplies a real controlling terminal. Its parent owns and
		// reaps the process group even when the interactive child does not finish.
		driver := nativeInitPTYDriver
		mode := "terminal"
		if redirect {
			mode = "redirect"
		}
		captured := filepath.Join(filepath.Dir(source), "redirected-output")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, "python3", "-B", "-c", driver, filepath.Join(source, "factory-go"), target, nativeInitAnswers("y"), mode, captured) // #nosec G204 -- fixed test-only PTY driver; literal fixture paths, no evaluated input.
		command.Env = environment
		command.Dir = filepath.Dir(source)
		output, err := command.CombinedOutput()
		Expect(ctx.Err()).NotTo(HaveOccurred())
		Expect(err).NotTo(HaveOccurred(), "%s", output)
		if redirect {
			data, readErr := os.ReadFile(captured)
			Expect(readErr).NotTo(HaveOccurred())
			output = append(output, data...)
		}
		Expect(string(output)).NotTo(ContainSubstring("Pack(s)?"))
		Expect(string(output)).To(ContainSubstring("gates proven"))
		_, err = os.Stat(filepath.Join(target, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
	}, Entry("explicit none with controlling terminal", false), Entry("stdin terminal with redirected stdout and implicit no packs", true))
})

var _ = Describe("Native Go init flat configuration representation", func() {
	// per docs/adr/0085-go-native-init.md:110
	DescribeTable("refuses an unquoted field that the flat parser would truncate", func(index int) {
		source, target, environment := nativeInitFixture()
		fields := []string{"Native Project", "native-project", "@example", "native-user", "", "docs", "", "inherit", "standard", "n"}
		fields[index] = "value # private-suffix"
		actual := nativeInitRun(source, target, nativeInitInput(fields, nil, "y"), environment, []string{"--pack", "none"})
		Expect(actual.status).To(Equal(1), "%+v", actual)
		_, err := os.Stat(target)
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("project slug", 1), Entry("docs root", 5))
})

var _ = Describe("Native Go init generated workflow representation", func() {
	// per docs/adr/0085-go-native-init.md:111
	DescribeTable("refuses input that would become active shell or YAML syntax", func(field, value string) {
		source, target, environment := nativeInitFixture()
		fields := []string{"Native Project", "native-project", "@example", "native-user", "internal/core", "docs", "", "inherit", "standard", "n"}
		version := "1.26"
		if field == "protected" {
			fields[4] = value
		} else {
			version = value
		}
		actual := nativeInitRun(source, target, nativeInitInput(fields, []string{version}, "y"), environment, []string{"--pack", "go"})
		Expect(actual.status).To(Equal(1), "%+v", actual)
		_, err := os.Stat(target)
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("protected shell separator", "protected", "p; touch INIT_LITERAL_MARKER; #"), Entry("protected command substitution", "protected", "$(touch INIT_LITERAL_MARKER)"), Entry("parent traversal", "protected", "../outside"), Entry("absolute protected path", "protected", "/outside"), Entry("version YAML comment", "version", "1.26 # ignored"), Entry("version expression", "version", "${{ secrets.PRIVATE }}"))
})

var _ = Describe("Native Go init Git worktree confinement", func() {
	// per docs/adr/0085-go-native-init.md:70
	It("refuses a repository configuration that redirects the target worktree before publication", func() {
		source, target, environment := nativeInitFixture()
		outside := target + "-outside"
		writeFixture(filepath.Join(outside, "factory.yaml"), []byte("review_lane: off\nproject_name: private-outside\n"), 0600)
		for _, args := range [][]string{{"init", "-q", target}, {"-C", target, "config", "core.worktree", outside}} {
			command := exec.Command("git", args...)
			command.Env = environment
			Expect(command.Run()).To(Succeed())
		}
		beforeTarget, beforeOutside := nativeInitArtifacts(target), nativeInitArtifacts(outside)
		actual := nativeInitRun(source, target, nativeInitAnswers("y"), environment, []string{"--pack", "none"})
		Expect(actual.status).To(Equal(1), "%+v", actual)
		Expect(nativeInitArtifacts(target)).To(Equal(beforeTarget))
		Expect(nativeInitArtifacts(outside)).To(Equal(beforeOutside))
	})
})

var _ = Describe("Native Go init structured artifact representation", func() {
	// per docs/adr/0085-go-native-init.md:115
	It("preserves a literal backslash in a JSON username instead of interpreting an escape", func() {
		source, target, environment := nativeInitFixture()
		username := `literal\name`
		actual := nativeInitRun(source, target, nativeInitInput([]string{"Native Project", "native-project", "@example", username}, nil, "y"), environment, []string{"--pack", "none"})
		Expect(actual.status).To(BeZero(), "%+v", actual)
		data, err := os.ReadFile(filepath.Join(target, "opencode.json"))
		Expect(err).NotTo(HaveOccurred())
		var config map[string]any
		Expect(json.Unmarshal(data, &config)).To(Succeed())
		Expect(config).To(HaveKeyWithValue("username", username))
	})
	// per docs/adr/0085-go-native-init.md:117
	It("refuses a CODEOWNERS comment that would erase the requested owner", func() {
		source, target, environment := nativeInitFixture()
		actual := nativeInitRun(source, target, nativeInitInput([]string{"Native Project", "native-project", "# @hidden-owner"}, nil, "y"), environment, []string{"--pack", "none"})
		Expect(actual.status).To(Equal(1), "%+v", actual)
		_, err := os.Stat(target)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

var _ = Describe("Native Go init root representation", func() {
	// per docs/adr/0085-go-native-init.md:108
	DescribeTable("refuses a root containing a newline before publishing template assets", func(which string) {
		source, target, environment := nativeInitFixture()
		if which == "source" {
			renamed := source + "\n"
			Expect(os.Rename(source, renamed)).To(Succeed())
			source = renamed
		} else {
			target += "\n"
		}
		actual := nativeInitRun(source, target, nativeInitAnswers("y"), environment, []string{"--pack", "none"})
		Expect(actual.status).To(Equal(1), "%+v", actual)
		_, err := os.Stat(target)
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("source root", "source"), Entry("target root", "target"))
})

var _ = Describe("Native Go init linked worktree compatibility", func() {
	// per docs/adr/0085-go-native-init.md:72
	It("supports an ordinary linked worktree whose resolved root is the selected target", func() {
		source, target, environment := nativeInitFixture()
		repository := filepath.Join(filepath.Dir(source), "repository")
		for _, args := range [][]string{{"init", "-q", repository}, {"-C", repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-q", "--allow-empty", "-m", "fixture"}, {"-C", repository, "worktree", "add", "-q", "-b", "fixture-target", target}} {
			command := exec.Command("git", args...)
			command.Env = environment
			output, err := command.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "%s", output)
		}
		before, err := os.ReadFile(filepath.Join(target, ".git"))
		Expect(err).NotTo(HaveOccurred())
		actual := nativeInitRun(source, target, nativeInitAnswers("y"), environment, []string{"--pack", "none"})
		Expect(actual.status).To(BeZero(), "%+v", actual)
		after, err := os.ReadFile(filepath.Join(target, ".git"))
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
	})
})

var _ = Describe("Native Go init invalid UTF8 admission", func() {
	// per docs/adr/0085-go-native-init.md:44
	DescribeTable("refuses consumed invalid bytes before normalization while ignoring unrelated environment", func(field string, status int) {
		source, target, environment := nativeInitFixture()
		fields := []string{"Native Project", "native-project", "@example", "native-user", "", "", "", "inherit", "standard", "n"}
		invalid := "private" + string([]byte{0xff})
		switch field {
		case "project":
			fields[0] = invalid
		case "provider":
			fields[7] = invalid
		case "model":
			environment = append(environment, "DEFAULT_MODEL="+invalid)
		case "unrelated":
			environment = append(environment, "UNUSED_INIT_TEST_VALUE="+invalid)
		}
		actual := nativeInitRun(source, target, nativeInitInput(fields, nil, "y"), environment, []string{"--pack", "none"})
		Expect(actual.status).To(Equal(status), "%+v", actual)
		if status != 0 {
			_, err := os.Stat(target)
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	}, Entry("project name", "project", 1), Entry("provider before lowercase", "provider", 1), Entry("consumed model environment", "model", 1), Entry("unrelated environment control", "unrelated", 0))
})

const nativeInitPTYDriver = `import errno,os,pty,select,signal,sys,time
pid,fd=pty.fork()
if pid==0:
 if sys.argv[4]=='redirect':
  output=os.open(sys.argv[5],os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600)
  os.dup2(output,1)
  os.close(output)
 args=[sys.argv[1],'init',sys.argv[2]]
 if sys.argv[4]!='redirect': args+=['--pack','none']
 os.execv(sys.argv[1],args)
status=None
output=bytearray()
try:
 os.write(fd,sys.argv[3].encode())
 end=time.monotonic()+25
 while time.monotonic()<end:
  ready,_,_=select.select([fd],[],[],0.1)
  if ready:
   try: part=os.read(fd,65536)
   except OSError as e:
    if e.errno!=errno.EIO: raise
    part=b''
   if part: output.extend(part)
  found,raw=os.waitpid(pid,os.WNOHANG)
  if found:
   status=os.waitstatus_to_exitcode(raw)
   break
finally:
 if status is None:
  os.killpg(pid,signal.SIGKILL)
  os.waitpid(pid,0)
  status=99
 os.close(fd)
sys.stdout.buffer.write(output)
sys.exit(status)
`

var _ = Describe("Native Go init interactive review choice", func() {
	// per docs/adr/0085-go-native-init.md:52
	It("records the explicitly selected review model after installation confirmation", func() {
		source, target, environment := nativeInitFixture()
		fields := []string{"Native Project", "native-project", "@example", "native-user", "", "", "", "openai", "standard", "y"}
		input := nativeInitInput(fields, nil, "y") + "explicit-review-model\n"
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, "python3", "-B", "-c", nativeInitPTYDriver, filepath.Join(source, "factory-go"), target, input, "terminal", filepath.Join(filepath.Dir(source), "unused-output")) // #nosec G204 -- fixed test-only PTY driver, literal fixture args.
		command.Env = environment
		command.Dir = filepath.Dir(source)
		output, err := command.CombinedOutput()
		Expect(ctx.Err()).NotTo(HaveOccurred())
		Expect(err).NotTo(HaveOccurred(), "%s", output)
		data, err := os.ReadFile(filepath.Join(target, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`review_model: "explicit-review-model"`))
	})
})

var _ = Describe("Native Go init review secret representation", func() {
	// per docs/adr/0085-go-native-init.md:120
	It("refuses a malformed secret identifier before the retained workflow renderer runs", func() {
		source, target, environment := nativeInitFixture()
		environment = append(environment, "REVIEW_API_KEY_SECRET=BAD&SECRET")
		fields := []string{"Native Project", "native-project", "@example", "native-user", "", "", "", "openai", "standard", "y"}
		actual := nativeInitRun(source, target, nativeInitInput(fields, nil, "y"), environment, []string{"--pack", "none"})
		Expect(actual.status).To(Equal(1), "%+v", actual)
		_, err := os.Stat(target)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

var _ = Describe("Native Go init exported prompt compatibility", func() {
	// per docs/adr/0085-go-native-init.md:72
	It("passes updated prompt values to real sync children when those variables were originally exported", func() {
		source, target, environment := nativeInitFixture()
		environment = append(environment, "MODEL_PROVIDER=anthropic", "COST_PROFILE=economy", "GO_VERSION=1.22", "REVIEW_LANE=on")
		for _, name := range []string{"sync-opencode.sh", "sync-claude.sh", "sync-codex.sh"} {
			command := exec.Command("git", "show", "ef5a06ae080c074241a783eb151ba8bb179ff6bd:scripts/"+name) // #nosec G204 -- fixed immutable revision, test-owned script list.
			command.Dir = ".."
			data, err := command.Output()
			Expect(err).NotTo(HaveOccurred())
			writeFixture(filepath.Join(source, "scripts", name), data, 0700)
		}
		fields := []string{"Native Project", "native-project", "@example", "native-user", "internal/core", "docs", "", "openai", "standard", "n"}
		input := nativeInitInput(fields, []string{"1.26"}, "y")
		oracle := target + "-oracle"
		args := []string{"--pack", "go"}
		expected := nativeInitLegacy(source, oracle, input, environment, args)
		Expect(expected.status).To(BeZero(), "%+v", expected)
		actual := nativeInitRun(source, target, input, environment, args)
		Expect(actual.status).To(BeZero(), "%+v", actual)
		before, after := nativeInitArtifacts(oracle), nativeInitArtifacts(target)
		Expect(after).To(Equal(before), "changed artifact paths: %v", initArtifactChanges(before, after))
	})
})
