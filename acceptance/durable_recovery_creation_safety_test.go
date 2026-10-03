package acceptance_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func durableReplaceOption(args []string, flag, value string, omit bool) []string {
	result := []string{}
	for i := 0; i < len(args); i++ {
		if args[i] == flag {
			i++
			if !omit {
				result = append(result, flag, value)
			}
		} else {
			result = append(result, args[i])
		}
	}
	return result
}

func durableRefusal(out cliResult, root string) {
	GinkgoHelper()
	Expect(out.status).To(Equal(2), "%+v", out)
	Expect(out.stdout).To(BeEmpty())
	Expect(out.stderr).To(HavePrefix("factory upgrade:"))
	Expect(strings.Count(out.stderr, "\n")).To(Equal(1))
	Expect(out.stderr).NotTo(ContainSubstring(root))
	Expect(out.stderr).NotTo(ContainSubstring("PRIVATE_"))
}

func durableNoManifest(root, id string) {
	GinkgoHelper()
	_, err := os.Lstat(filepath.Join(root, ".factory/backups", id, "manifest.json"))
	Expect(err).To(HaveOccurred(), "a rejected operation must not publish completion")
}

var _ = Describe("Durable local recovery creation admission", func() {
	// per docs/adr/0091-durable-local-recovery-creation.md:43
	// per docs/adr/0091-durable-local-recovery-creation.md:197
	It("refuses an explicit digest conflict even when the environment supplies the current confirmation", func() {
		binary, root, _ := durableRecoveryFixture()
		args := durableRecoveryArgs(binary, root, durableRecoveryID, strings.Repeat("a", 40), assessmentPaths[:1])
		var digest string
		for i, arg := range args {
			if arg == "--confirm-adoption" {
				digest = args[i+1]
			}
		}
		Expect(digest).NotTo(BeEmpty())
		wrong := strings.Repeat("0", 64)
		if wrong == digest {
			wrong = strings.Repeat("1", 64)
		}
		args = durableReplaceOption(args, "--confirm-adoption", wrong, false)
		marker := previewPoison(binary)
		before := assessmentTree(root)
		out := durableRecoveryRun(binary, root, args, "FACTORY_ADOPTION_DIGEST="+digest)
		durableRefusal(out, root)
		Expect(out.stderr).To(Equal("factory upgrade: adoption confirmation does not match current observations\n"))
		Expect(assessmentTree(root)).To(Equal(before))
		publicNoPoison(marker)
	})

	// per docs/adr/0091-durable-local-recovery-creation.md:57
	It("uses operational status when the compiled diagnostic pipe is already closed", func() {
		binary, root, _ := durableRecoveryFixture()
		readEnd, writeEnd, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		Expect(readEnd.Close()).To(Succeed())
		DeferCleanup(writeEnd.Close)
		var stdout bytes.Buffer
		command := exec.Command(filepath.Join(binary, "factory"), "upgrade", "--create-backup") // #nosec G204 -- source-built fixture binary with fixed invalid request.
		command.Dir, command.Env = root, durableGitEnvironment()
		command.Stdout, command.Stderr = &stdout, writeEnd
		runErr := command.Run()
		var exitErr *exec.ExitError
		Expect(errors.As(runErr, &exitErr)).To(BeTrue())
		Expect(exitErr.ExitCode()).To(Equal(1))
		Expect(stdout.Len()).To(BeZero())
		durableNoManifest(root, durableRecoveryID)
	})

	// per docs/adr/0091-durable-local-recovery-creation.md:33
	DescribeTable("rejects malformed reserved requests before dispatch or persistent changes", func(kind string) {
		binary, root, _ := durableRecoveryFixture()
		args := durableRecoveryArgs(binary, root, durableRecoveryID, strings.Repeat("a", 40), assessmentPaths[:1])
		var environment []string
		switch kind {
		case "attached create":
			args[0] = "--create-backup=PRIVATE_VALUE"
		case "empty attached create":
			args[0] = "--create-backup="
		case "duplicate create":
			args = append(args, "--create-backup")
		case "missing id":
			args = durableReplaceOption(args, "--migration-id", "", true)
		case "unsafe id":
			args = durableReplaceOption(args, "--migration-id", "../PRIVATE_PATH\nSECOND", false)
		case "long id":
			args = durableReplaceOption(args, "--migration-id", strings.Repeat("a", 65), false)
		case "missing target":
			args = durableReplaceOption(args, "--target-revision", "", true)
		case "uppercase target":
			args = durableReplaceOption(args, "--target-revision", strings.Repeat("A", 40), false)
		case "reference target":
			args = durableReplaceOption(args, "--target-revision", assessmentRevision, false)
		case "missing selection":
			args = durableReplaceOption(args, "--adopt-path", "", true)
		case "unknown selection":
			args = durableReplaceOption(args, "--adopt-path", "PRIVATE_PATH", false)
		case "selection alias":
			args = durableReplaceOption(args, "--adopt-path", "./"+assessmentPaths[0], false)
		case "duplicate selection":
			args = append(args, "--adopt-path", assessmentPaths[0])
		case "missing confirmation":
			args = durableReplaceOption(args, "--confirm-adoption", "", true)
		case "environment confirmation":
			for i, arg := range args {
				if arg == "--confirm-adoption" {
					environment = []string{"FACTORY_ADOPTION_DIGEST=" + args[i+1]}
				}
			}
			args = durableReplaceOption(args, "--confirm-adoption", "", true)
		case "mismatched confirmation":
			args = durableReplaceOption(args, "--confirm-adoption", strings.Repeat("0", 64), false)
		case "duplicate id":
			args = append(args, "--migration-id", "PRIVATE_SECOND")
		case "duplicate target":
			args = append(args, "--target-revision", strings.Repeat("b", 40))
		case "duplicate confirmation":
			args = append(args, "--confirm-adoption", strings.Repeat("b", 64))
		case "duplicate json":
			args = append(args, "--json")
		case "valued json":
			args[len(args)-1] = "--json=true"
		default:
			args = append(args, kind)
		}
		marker := previewPoison(binary)
		before := assessmentTree(root)
		durableRefusal(durableRecoveryRun(binary, root, args, environment...), root)
		Expect(assessmentTree(root)).To(Equal(before))
		publicNoPoison(marker)
	}, Entry("attached create", "attached create"), Entry("empty attached create", "empty attached create"), Entry("duplicate create", "duplicate create"), Entry("missing id", "missing id"), Entry("unsafe id", "unsafe id"), Entry("long id", "long id"), Entry("missing target", "missing target"), Entry("uppercase target", "uppercase target"), Entry("reference target", "reference target"), Entry("missing selection", "missing selection"), Entry("unknown selection", "unknown selection"), Entry("selection alias", "selection alias"), Entry("duplicate selection", "duplicate selection"), Entry("missing confirmation", "missing confirmation"), Entry("environment confirmation", "environment confirmation"), Entry("mismatched confirmation", "mismatched confirmation"), Entry("duplicate id", "duplicate id"), Entry("duplicate target", "duplicate target"), Entry("duplicate confirmation", "duplicate confirmation"), Entry("duplicate json", "duplicate json"), Entry("valued json", "valued json"), Entry("source", "--source=PRIVATE_SOURCE"), Entry("preview", "--dry-run"), Entry("inspection", "--inspect-backups"), Entry("ref", "--ref=PRIVATE_REF"), Entry("help", "--help"), Entry("unknown", "--PRIVATE_UNKNOWN"))

	// per docs/adr/0091-durable-local-recovery-creation.md:40
	It("preserves the private protocol's precedence", func() {
		binary, root, _ := durableRecoveryFixture()
		args := durableRecoveryArgs(binary, root, durableRecoveryID, strings.Repeat("a", 40), assessmentPaths[:1])
		before := assessmentTree(root)
		out := loopProcess(root, filepath.Join(binary, "factory"), append([]string{"upgrade"}, args...), nil)
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).To(HavePrefix("factory bridge:"))
		Expect(assessmentTree(root)).To(Equal(before))
	})

	// per docs/adr/0091-durable-local-recovery-creation.md:40
	DescribeTable("keeps ordinary literal legacy dispatch without the reserved marker", func(arg string) {
		binary, root, _ := durableRecoveryFixture()
		writeFixture(filepath.Join(binary, "scripts/factory-upgrade.sh"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\nexit 37\n"), 0700)
		out := durableRecoveryRun(binary, root, []string{arg})
		Expect(out).To(Equal(cliResult{arg + "\n", "", 37}))
	}, Entry("similar switch", "--create-backups"), Entry("literal operand", "--source=--create-backup"))

	// per docs/adr/0091-durable-local-recovery-creation.md:192
	DescribeTable("preserves detached legacy option values that spell the recovery marker", func(ref bool) {
		binary, root, _ := durableRecoveryFixture()
		writeFixture(filepath.Join(binary, "scripts/factory-upgrade.sh"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\nexit 37\n"), 0700)
		args := []string{"--source", "--create-backup"}
		if ref {
			args = []string{"--ref", "--create-backup", "--source", filepath.Join(root, "source")}
		}
		before := assessmentTree(root)
		out := durableRecoveryRun(binary, root, args)
		Expect(out).To(Equal(cliResult{strings.Join(args, "\n") + "\n", "", 37}))
		Expect(assessmentTree(root)).To(Equal(before))
	}, Entry("detached source value", false), Entry("detached ref value", true))
})

var _ = Describe("Durable local recovery creation confinement", func() {
	// per docs/adr/0091-durable-local-recovery-creation.md:43
	It("binds explicit confirmation to the physical root even when selected bytes are identical", func() {
		binary, root, _ := durableRecoveryFixture()
		args := durableRecoveryArgs(binary, root, durableRecoveryID, strings.Repeat("a", 40), assessmentPaths[:1])
		data := durableBytes(root, assessmentPaths[0])
		held := root + ".held"
		Expect(os.Rename(root, held)).To(Succeed())
		DeferCleanup(os.RemoveAll, held)
		Expect(os.Mkdir(root, 0700)).To(Succeed())
		writeFixture(filepath.Join(root, assessmentPaths[0]), data, 0755)
		durableGit(root, "init", "-q")
		durableGit(root, "add", "--", "scripts")
		before := assessmentTree(root)
		durableRefusal(durableRecoveryRun(binary, root, args), root)
		Expect(assessmentTree(root)).To(Equal(before))
		durableNoManifest(root, durableRecoveryID)
	})

	// per docs/adr/0091-durable-local-recovery-creation.md:43
	DescribeTable("refuses changed selected identities before creating recovery payloads", func(kind string) {
		binary, root, _ := durableRecoveryFixture()
		args := durableRecoveryArgs(binary, root, durableRecoveryID, strings.Repeat("a", 40), assessmentPaths[:1])
		path := filepath.Join(root, assessmentPaths[0])
		switch kind {
		case "bytes":
			writeFixture(path, []byte("PRIVATE_CUSTOM_CONTENT"), 0755)
		case "inode":
			data := durableBytes(root, assessmentPaths[0])
			Expect(os.Rename(path, path+".held")).To(Succeed())
			writeFixture(path, data, 0755)
		case "mode":
			Expect(os.Chmod(path, 0700)).To(Succeed())
		case "missing":
			Expect(os.Remove(path)).To(Succeed())
		case "symlink":
			Expect(os.Rename(path, path+".held")).To(Succeed())
			Expect(os.Symlink(path+".held", path)).To(Succeed())
		case "hardlink":
			Expect(os.Link(path, path+".linked")).To(Succeed())
		case "FIFO":
			Expect(os.Remove(path)).To(Succeed())
			Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
		}
		before := assessmentTree(root)
		durableRefusal(durableRecoveryRun(binary, root, args), root)
		Expect(assessmentTree(root)).To(Equal(before))
		durableNoManifest(root, durableRecoveryID)
	}, Entry("content", "bytes"), Entry("identity", "inode"), Entry("mode", "mode"), Entry("missing", "missing"), Entry("symlink", "symlink"), Entry("hardlink", "hardlink"), Entry("FIFO", "FIFO"))

	// per docs/adr/0091-durable-local-recovery-creation.md:46
	It("backs up one approved reference while leaving unselected customization and runtime bytes intact", func() {
		binary, root, references := durableRecoveryFixture()
		writeFixture(filepath.Join(root, assessmentPaths[1]), []byte("PRIVATE_USER_CUSTOMIZATION"), 0600)
		args := durableRecoveryArgs(binary, root, durableRecoveryID, strings.Repeat("a", 40), assessmentPaths[:1])
		durableRecoveryObject(durableRecoveryRun(binary, root, args), root, durableRecoveryID, strings.Repeat("a", 40), "created", 1, references[0].Bytes)
		Expect(string(durableBytes(root, assessmentPaths[1]))).To(Equal("PRIVATE_USER_CUSTOMIZATION"))
		_, err := os.Lstat(filepath.Join(root, ".factory/backups", durableRecoveryID, "files", assessmentPaths[1]))
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(string(durableBytes(root, ".factory/events.log"))).To(Equal("PRIVATE_RUNTIME_HISTORY\n"))
	})

	// per docs/adr/0091-durable-local-recovery-creation.md:63
	// per docs/adr/0091-durable-local-recovery-creation.md:96
	DescribeTable("preserves unsafe Git or recovery controls and refuses completion", func(kind string) {
		binary, root, _ := durableRecoveryFixture()
		args := durableRecoveryArgs(binary, root, durableRecoveryID, strings.Repeat("a", 40), assessmentPaths[:1])
		path := filepath.Join(root, ".git/info/exclude")
		switch kind {
		case "missing Git":
			Expect(os.RemoveAll(filepath.Join(root, ".git"))).To(Succeed())
		case "Git file":
			Expect(os.RemoveAll(filepath.Join(root, ".git"))).To(Succeed())
			writeFixture(filepath.Join(root, ".git"), []byte("gitdir: PRIVATE_EXTERNAL\n"), 0600)
		case "bare Git":
			durableGit(root, "config", "core.bare", "true")
		case "missing info":
			Expect(os.RemoveAll(filepath.Join(root, ".git/info"))).To(Succeed())
		case "Git symlink":
			path = filepath.Join(root, ".git")
			Expect(os.Rename(path, path+".held")).To(Succeed())
			Expect(os.Symlink(path+".held", path)).To(Succeed())
		case "info symlink":
			path = filepath.Join(root, ".git/info")
			Expect(os.Rename(path, path+".held")).To(Succeed())
			Expect(os.Symlink(path+".held", path)).To(Succeed())
		case "info public writing":
			path = filepath.Join(root, ".git/info")
			Expect(os.Chmod(path, 0770)).To(Succeed())
		case "exclude symlink":
			Expect(os.Rename(path, path+".held")).To(Succeed())
			Expect(os.Symlink(path+".held", path)).To(Succeed())
		case "exclude hardlink":
			Expect(os.Link(path, path+".linked")).To(Succeed())
		case "exclude FIFO":
			Expect(os.Remove(path)).To(Succeed())
			Expect(syscall.Mkfifo(path, 0600)).To(Succeed())
		case "exclude public writing":
			Expect(os.Chmod(path, 0666)).To(Succeed())
		case "exclude group writing survives overwrite":
			Expect(os.Chmod(path, 0664)).To(Succeed())
			writeFixture(path, []byte("# PRIVATE_TEMPLATE_EXCLUDE\n"), 0600)
			info, err := os.Lstat(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0664)), "WriteFile creation mode must not conceal the unsafe template fixture")
		case "exclude special mode":
			Expect(os.Chmod(path, 0600|os.ModeSticky)).To(Succeed())
			info, err := os.Lstat(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode()&os.ModeSticky).NotTo(BeZero(), "fixture must retain the unsafe special mode")
		case "exclude oversized":
			writeFixture(path, []byte(strings.Repeat("#", (64<<10)+1)), 0600)
		case "recovery symlink":
			path = filepath.Join(root, ".factory/backups")
			Expect(os.Symlink(root, path)).To(Succeed())
		case "recovery public directory":
			path = filepath.Join(root, ".factory/backups")
			Expect(os.Mkdir(path, 0755)).To(Succeed())
			writeFixture(filepath.Join(path, "PRIVATE_KEEP"), []byte("PRIVATE_RECOVERY_KEEP"), 0600)
		case "factory symlink":
			path = filepath.Join(root, ".factory")
			Expect(os.Rename(path, path+".held")).To(Succeed())
			Expect(os.Symlink(path+".held", path)).To(Succeed())
		case "factory public writing":
			path = filepath.Join(root, ".factory")
			Expect(os.Chmod(path, 0770)).To(Succeed())
		}
		var protected []byte
		var identity os.FileInfo
		if info, err := os.Lstat(path); err == nil {
			identity = info
			if info.Mode().IsRegular() {
				protected = durableBytes(root, strings.TrimPrefix(path, root+string(os.PathSeparator)))
			}
		}
		durableRefusal(durableRecoveryRun(binary, root, args), root)
		durableNoManifest(root, durableRecoveryID)
		if identity != nil {
			after, err := os.Lstat(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.SameFile(identity, after)).To(BeTrue())
			Expect(after.Mode()).To(Equal(identity.Mode()))
			if protected != nil {
				Expect(durableBytes(root, strings.TrimPrefix(path, root+string(os.PathSeparator)))).To(Equal(protected))
			}
		}
	}, Entry("missing Git", "missing Git"), Entry("Git file", "Git file"), Entry("bare Git", "bare Git"), Entry("missing info", "missing info"), Entry("Git symlink", "Git symlink"), Entry("info symlink", "info symlink"), Entry("info public writing", "info public writing"), Entry("exclude symlink", "exclude symlink"), Entry("exclude hardlink", "exclude hardlink"), Entry("exclude FIFO", "exclude FIFO"), Entry("exclude public writing", "exclude public writing"), Entry("exclude group writing survives overwrite", "exclude group writing survives overwrite"), Entry("exclude special mode", "exclude special mode"), Entry("exclude oversized", "exclude oversized"), Entry("recovery symlink", "recovery symlink"), Entry("recovery public directory", "recovery public directory"), Entry("factory symlink", "factory symlink"), Entry("factory public writing", "factory public writing"))
})

var _ = Describe("Durable local recovery creation Git exclusion", func() {
	// per docs/adr/0091-durable-local-recovery-creation.md:68
	DescribeTable("bounds Git query duration and captured private output", func(timeout bool) {
		binary, root, _ := durableRecoveryFixture()
		args := durableRecoveryArgs(binary, root, durableRecoveryID, strings.Repeat("a", 40), assessmentPaths[:1])
		helper := "#!/bin/sh\nwhile :; do printf PRIVATE_GIT_QUERY_OUTPUT >&2; done\n"
		if timeout {
			helper = "#!/bin/sh\nexec /bin/sleep 60\n"
		}
		writeFixture(filepath.Join(binary, "git"), []byte(helper), 0700)
		before := assessmentTree(root)
		started := time.Now()
		out := durableRecoveryRun(binary, root, args, "PATH="+binary)
		Expect(time.Since(started)).To(BeNumerically("<", 20*time.Second))
		Expect(out.status).To(Equal(1), "%+v", out)
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).To(HavePrefix("factory upgrade:"))
		Expect(strings.Count(out.stderr, "\n")).To(Equal(1))
		Expect(out.stderr).NotTo(ContainSubstring(root))
		Expect(out.stderr).NotTo(ContainSubstring("PRIVATE_"))
		Expect(assessmentTree(root)).To(Equal(before))
		durableNoManifest(root, durableRecoveryID)
	}, Entry("finite query deadline", true), Entry("bounded query stderr", false))

	// per docs/adr/0091-durable-local-recovery-creation.md:65
	It("safely refuses an unavailable Git executable without dispatching a legacy command", func() {
		binary, root, _ := durableRecoveryFixture()
		args := durableRecoveryArgs(binary, root, durableRecoveryID, strings.Repeat("a", 40), assessmentPaths[:1])
		marker := previewPoison(binary)
		before := assessmentTree(root)
		durableRefusal(durableRecoveryRun(binary, root, args, "PATH="+binary), root)
		Expect(assessmentTree(root)).To(Equal(before))
		publicNoPoison(marker)
	})

	// per docs/adr/0091-durable-local-recovery-creation.md:73
	It("preserves an unterminated exclude file and appends exactly one delimited local rule", func() {
		binary, root, references := durableRecoveryFixture()
		writeFixture(filepath.Join(root, ".git/info/exclude"), []byte("PRIVATE_EXISTING_EXCLUDE"), 0600)
		args := durableRecoveryArgs(binary, root, durableRecoveryID, strings.Repeat("a", 40), assessmentPaths[:1])
		durableRecoveryObject(durableRecoveryRun(binary, root, args), root, durableRecoveryID, strings.Repeat("a", 40), "created", 1, references[0].Bytes)
		Expect(string(durableBytes(root, ".git/info/exclude"))).To(Equal("PRIVATE_EXISTING_EXCLUDE\n/.factory/backups/\n"))
	})

	// per docs/adr/0091-durable-local-recovery-creation.md:71
	DescribeTable("refuses tracked recovery data without altering the index or saved bytes", func(rootFile bool) {
		binary, root, _ := durableRecoveryFixture()
		args := durableRecoveryArgs(binary, root, durableRecoveryID, strings.Repeat("a", 40), assessmentPaths[:1])
		path := ".factory/backups/PRIVATE_TRACKED"
		if rootFile {
			path = ".factory/backups"
		}
		writeFixture(filepath.Join(root, path), []byte("PRIVATE_TRACKED_BYTES"), 0600)
		durableGit(root, "add", "-f", "--", path)
		index := durableBytes(root, ".git/index")
		durableRefusal(durableRecoveryRun(binary, root, args), root)
		Expect(durableBytes(root, ".git/index")).To(Equal(index))
		Expect(string(durableBytes(root, path))).To(Equal("PRIVATE_TRACKED_BYTES"))
		durableNoManifest(root, durableRecoveryID)
	}, Entry("tracked leaf", false), Entry("tracked recovery root file", true))

	// per docs/adr/0091-durable-local-recovery-creation.md:80
	It("refuses higher-precedence negation instead of rewriting project ignore rules", func() {
		binary, root, _ := durableRecoveryFixture()
		args := durableRecoveryArgs(binary, root, durableRecoveryID, strings.Repeat("a", 40), assessmentPaths[:1])
		ignore := "!/.factory/backups/\n!/.factory/backups/**\n"
		writeFixture(filepath.Join(root, ".gitignore"), []byte(ignore), 0600)
		durableRefusal(durableRecoveryRun(binary, root, args), root)
		Expect(string(durableBytes(root, ".gitignore"))).To(Equal(ignore))
		durableNoManifest(root, durableRecoveryID)
		_, err := os.Lstat(filepath.Join(root, ".factory/backups", durableRecoveryID, "files"))
		Expect(err).To(HaveOccurred(), "effective exclusion must precede payload copying")
	})

	// per docs/adr/0091-durable-local-recovery-creation.md:73
	// per docs/adr/0091-durable-local-recovery-creation.md:181
	DescribeTable("establishes the canonical local exclusion even when other rules already ignore backups", func(kind string) {
		binary, root, references := durableRecoveryFixture()
		prefix := "# existing local exclusions\nkeep-local\n"
		switch kind {
		case "project with local file":
			writeFixture(filepath.Join(root, ".gitignore"), []byte("/.factory/backups/\n"), 0600)
		case "project without local file":
			writeFixture(filepath.Join(root, ".gitignore"), []byte("/.factory/backups/\n"), 0600)
			Expect(os.Remove(filepath.Join(root, ".git/info/exclude"))).To(Succeed())
			prefix = ""
		case "broader local rule":
			prefix += "/.factory/\n"
			writeFixture(filepath.Join(root, ".git/info/exclude"), []byte(prefix), 0600)
		}
		args := durableRecoveryArgs(binary, root, durableRecoveryID, strings.Repeat("a", 40), assessmentPaths[:1])
		project := durableBytes(root, ".gitignore")
		durableRecoveryObject(durableRecoveryRun(binary, root, args), root, durableRecoveryID, strings.Repeat("a", 40), "created", 1, references[0].Bytes)
		Expect(string(durableBytes(root, ".git/info/exclude"))).To(Equal(prefix + "/.factory/backups/\n"))
		Expect(durableBytes(root, ".gitignore")).To(Equal(project))
		durableRecoveryObject(durableRecoveryRun(binary, root, args), root, durableRecoveryID, strings.Repeat("a", 40), "already_present", 1, references[0].Bytes)
		Expect(string(durableBytes(root, ".git/info/exclude"))).To(Equal(prefix + "/.factory/backups/\n"))
		Expect(durableBytes(root, ".gitignore")).To(Equal(project))
	}, Entry("project rules with existing local prefix", "project with local file"), Entry("project rules with absent local exclude", "project without local file"), Entry("broader existing local rule", "broader local rule"))

	// per docs/adr/0091-durable-local-recovery-creation.md:67
	It("ignores inherited Git routing and configuration injection", func() {
		binary, root, references := durableRecoveryFixture()
		args := durableRecoveryArgs(binary, root, durableRecoveryID, strings.Repeat("a", 40), assessmentPaths[:1])
		external := filepath.Join(binary, "PRIVATE_INDEX")
		writeFixture(external, []byte("PRIVATE_INDEX_KEEP"), 0600)
		environment := []string{"GIT_DIR=/PRIVATE_EXTERNAL", "GIT_WORK_TREE=/PRIVATE_EXTERNAL", "GIT_INDEX_FILE=" + external, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.excludesFile", "GIT_CONFIG_VALUE_0=/PRIVATE_EXTERNAL"}
		index := durableBytes(root, ".git/index")
		durableRecoveryObject(durableRecoveryRun(binary, root, args, environment...), root, durableRecoveryID, strings.Repeat("a", 40), "created", 1, references[0].Bytes)
		Expect(durableBytes(root, ".git/index")).To(Equal(index))
		data, err := os.ReadFile(external)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("PRIVATE_INDEX_KEEP"))
	})

	// per docs/adr/0091-durable-local-recovery-creation.md:154
	It("does not execute a configured Git filesystem-monitor hook during queries", func() {
		binary, root, references := durableRecoveryFixture()
		args := durableRecoveryArgs(binary, root, durableRecoveryID, strings.Repeat("a", 40), assessmentPaths[:1])
		marker := filepath.Join(binary, "PRIVATE_FSMONITOR_EXECUTED")
		hook := filepath.Join(binary, "PRIVATE_FSMONITOR")
		writeFixture(hook, []byte("#!/bin/sh\nprintf called > \"$RECOVERY_FSMONITOR_MARKER\"\nprintf PRIVATE_FSMONITOR_OUTPUT >&2\nexit 1\n"), 0700)
		durableGit(root, "config", "core.fsmonitor", hook)
		control := exec.Command("git", "ls-files", "--cached", "-z", "--", ".factory/backups") // #nosec G204 -- same fixed local metadata query used by the creator.
		control.Dir = root
		control.Env = append(durableGitEnvironment(), "RECOVERY_FSMONITOR_MARKER="+marker)
		_, controlErr := control.CombinedOutput()
		Expect(controlErr).NotTo(HaveOccurred())
		_, markerErr := os.Lstat(marker)
		if os.IsNotExist(markerErr) {
			Skip("this Git does not invoke configured fsmonitor for the creator's cached-path query")
		}
		Expect(markerErr).NotTo(HaveOccurred())
		Expect(os.Remove(marker)).To(Succeed())
		durableRecoveryObject(durableRecoveryRun(binary, root, args, "RECOVERY_FSMONITOR_MARKER="+marker), root, durableRecoveryID, strings.Repeat("a", 40), "created", 1, references[0].Bytes)
		_, err := os.Lstat(marker)
		Expect(os.IsNotExist(err)).To(BeTrue(), "Git metadata queries must not execute the configured helper")
	})
})

var _ = Describe("Durable local recovery creation occupied sets", func() {
	// per docs/adr/0091-durable-local-recovery-creation.md:124
	DescribeTable("preserves held changed incomplete or differently requested sets", func(kind string) {
		binary, root, references := durableRecoveryFixture()
		args := durableRecoveryArgs(binary, root, durableRecoveryID, strings.Repeat("a", 40), assessmentPaths[:1])
		selected := references[:1]
		if kind == "different selection" {
			selected = references
		}
		set := recoverySet(root, durableRecoveryID, selected, kind == "held")
		switch kind {
		case "different target":
			manifest := recoveryManifest(set)
			manifest["target_revision"] = strings.Repeat("b", 40)
			recoveryWriteManifest(set, manifest)
		case "changed bytes":
			writeFixture(filepath.Join(set, "files", assessmentPaths[0]), []byte("PRIVATE_CHANGED_BACKUP"), 0600)
		case "incomplete":
			Expect(os.Remove(filepath.Join(set, "manifest.json"))).To(Succeed())
		case "extra leaf":
			writeFixture(filepath.Join(set, "PRIVATE_EXTRA"), []byte("PRIVATE_KEEP"), 0600)
		case "public set":
			Expect(os.Chmod(set, 0755)).To(Succeed())
		}
		before := assessmentTree(set)
		durableRefusal(durableRecoveryRun(binary, root, args), root)
		Expect(assessmentTree(set)).To(Equal(before))
	}, Entry("held", "held"), Entry("different target", "different target"), Entry("different selection", "different selection"), Entry("changed bytes", "changed bytes"), Entry("incomplete", "incomplete"), Entry("unknown leaf", "extra leaf"), Entry("public set", "public set"))

	// per docs/adr/0091-durable-local-recovery-creation.md:107
	It("refuses a sixty-fifth reservation without adding retry cruft", func() {
		binary, root, _ := durableRecoveryFixture()
		args := durableRecoveryArgs(binary, root, durableRecoveryID, strings.Repeat("a", 40), assessmentPaths[:1])
		backups := filepath.Join(root, ".factory/backups")
		Expect(os.Mkdir(backups, 0700)).To(Succeed())
		for index := range 64 {
			Expect(os.Mkdir(filepath.Join(backups, "occupied-"+strings.Repeat("a", index+1)), 0700)).To(Succeed())
		}
		before := assessmentTree(backups)
		durableRefusal(durableRecoveryRun(binary, root, args), root)
		Expect(assessmentTree(backups)).To(Equal(before))
		_, err := os.Lstat(filepath.Join(backups, durableRecoveryID))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})
