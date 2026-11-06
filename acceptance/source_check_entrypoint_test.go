package acceptance_test

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Each invocation exercises the current public source entrypoint bytes.
// per docs/adr/0138-go-source-check-orchestration.md:64
func sourceCheckAdapter(path string) string {
	GinkgoHelper()
	body, err := os.ReadFile("../scripts/pre-push-check.sh")
	Expect(err).NotTo(HaveOccurred())
	writeFixture(path, body, 0755)
	return path
}

// The native fixture retains its qualified Git and isolated stage environment.
// per docs/adr/0138-go-source-check-orchestration.md:69
func sourceCheckEnvironment(f *orchestrationFixture) map[string]string {
	environment := map[string]string{}
	for _, entry := range f.env {
		key, value, _ := strings.Cut(entry, "=")
		environment[key] = value
	}
	environment["BASH_ENV"], environment["ENV"] = "", ""
	return environment
}

// Record transport without allowing a shell to interpret caller operands.
// per docs/adr/0138-go-source-check-orchestration.md:24
const sourceCheckRecorder = `#!/bin/sh
printf '%s\000' "$PWD" "$CHECK_LITERAL" "${FACTORY_AGENT_ROLE+x}" "${FACTORY_AGENT_ROLE-}" "${REPO_ROOT-}" "$@" > "$CHECK_ARGV"
/bin/cat > "$CHECK_INPUT"
printf 'selected stdout\n'
printf 'selected stderr\n' >&2
exit 63
`

var _ = Describe("Go source runtime entrypoints check orchestration adapter", func() {
	// per docs/adr/0138-go-source-check-orchestration.md:15
	// per docs/adr/0138-go-source-check-orchestration.md:22
	// per docs/adr/0138-go-source-check-orchestration.md:24
	// per docs/adr/0138-go-source-check-orchestration.md:119
	It("uses physical script identity and transports literal caller streams and environment", func() {
		for _, bash := range []bool{false, true} {
			for _, alias := range []bool{false, true} {
				root, caller := fixtureDirectories()
				owner := filepath.Join(filepath.Dir(root), "check owner ' $(literal)\n")
				Expect(os.Rename(root, owner)).To(Succeed())
				program := sourceCheckAdapter(filepath.Join(owner, "scripts/pre-push-check.sh"))
				foreign, tools := filepath.Join(filepath.Dir(owner), "foreign"), filepath.Join(filepath.Dir(owner), "tools")
				marker, record, input := filepath.Join(filepath.Dir(owner), "UNEXPECTED_CHILD"), filepath.Join(filepath.Dir(owner), "argv"), filepath.Join(filepath.Dir(owner), "stdin")
				poison := []byte("#!/bin/sh\nprintf called > \"$CHECK_DECOY\"\nexit 97\n")
				for _, path := range []string{filepath.Join(foreign, "factory"), filepath.Join(caller, "factory"), filepath.Join(tools, "factory"), filepath.Join(tools, "git"), filepath.Join(tools, "go"), filepath.Join(owner, "scripts/lib/config.sh")} {
					writeFixture(path, poison, 0700)
				}
				writeFixture(filepath.Join(owner, "factory"), []byte(sourceCheckRecorder), 0700)
				if alias {
					linked := filepath.Join(filepath.Dir(owner), "directory alias")
					Expect(os.Symlink(filepath.Join(owner, "scripts"), linked)).To(Succeed())
					program = filepath.Join(linked, "pre-push-check.sh")
				}
				environment := map[string]string{"PATH": tools + ":/usr/bin:/bin", "BASH_ENV": "", "ENV": "", "REPO_ROOT": foreign, "FACTORY_RUNTIME_BINARY": filepath.Join(foreign, "factory"), "FACTORY_HOME": foreign, "FACTORY_AGENT_ROLE": "implementer", "CHECK_LITERAL": "literal $(touch CHECK_INJECTION)\nvalue", "CHECK_DECOY": marker, "CHECK_ARGV": record, "CHECK_INPUT": input}
				args := []string{"remote ' name", "literal $(touch CHECK_INJECTION)\nlocation", "", "--help", "--config=foreign", "--"}
				stdin := "refs/heads/topic abc refs/heads/topic def\nNUL:\x00tail\n"
				beforeOwner, beforeCaller := installationSnapshot(owner), installationSnapshot(caller)
				out := sourceStructuralInvoke(caller, environment, stdin, program, bash, args...)
				Expect(out).To(Equal(cliResult{"selected stdout\n", "selected stderr\n", 63}), "bash=%t alias=%t", bash, alias)
				expected := append([]string{caller, environment["CHECK_LITERAL"], "x", "implementer", foreign, "check"}, args...)
				Expect(os.ReadFile(record)).To(Equal([]byte(strings.Join(expected, "\x00") + "\x00")))
				Expect(os.ReadFile(input)).To(Equal([]byte(stdin)))
				Expect(installationSnapshot(owner)).To(Equal(beforeOwner))
				Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
				_, err := os.Lstat(marker)
				Expect(os.IsNotExist(err)).To(BeTrue(), "direct owner discovery must not invoke Git, a compiler, a reader or another factory")
			}
		}
		shells := []string{"/bin/bash"}
		if bash, err := exec.LookPath("bash"); err == nil && bash != shells[0] {
			shells = append(shells, bash)
		}
		for _, shell := range shells {
			for _, mode := range []string{"absolute-path", "relative-path", "cwd"} {
				root, caller := fixtureDirectories()
				program := sourceCheckAdapter(filepath.Join(root, "scripts/pre-push-check.sh"))
				Expect(os.Chmod(program, 0644)).To(Succeed(), "Bash may read a nonexecutable script before a later executable PATH match")
				base := filepath.Dir(root)
				foreign, marker := filepath.Join(base, "later PATH checkout"), filepath.Join(base, "WRONG_PATH_OWNER")
				poison := []byte("#!/bin/sh\nprintf called > \"$CHECK_DECOY\"\nexit 97\n")
				writeFixture(filepath.Join(foreign, "scripts/pre-push-check.sh"), poison, 0700)
				writeFixture(filepath.Join(foreign, "factory"), poison, 0700)
				writeFixture(filepath.Join(root, "factory"), []byte(sourceCheckRecorder), 0700)
				cwd, lookup := caller, filepath.Dir(program)
				switch mode {
				case "relative-path":
					var err error
					lookup, err = filepath.Rel(caller, lookup)
					Expect(err).NotTo(HaveOccurred())
				case "cwd":
					cwd, lookup = filepath.Dir(program), filepath.Join(foreign, "scripts")
				}
				record, input := filepath.Join(base, "argv"), filepath.Join(base, "stdin")
				environment := map[string]string{"PATH": lookup + ":" + filepath.Join(foreign, "scripts") + ":/usr/bin:/bin", "BASH_ENV": "", "ENV": "", "CHECK_DECOY": marker, "CHECK_ARGV": record, "CHECK_INPUT": input, "CHECK_LITERAL": "Bash path selection"}
				out := handoffNativeExactProcess(cwd, environment, "borrowed input\n", shell, "pre-push-check.sh", "origin", "remote literal")
				Expect(out).To(Equal(cliResult{"selected stdout\n", "selected stderr\n", 63}), "%s/%s", shell, mode)
				Expect(os.ReadFile(record)).To(Equal([]byte(strings.Join([]string{cwd, "Bash path selection", "", "", "", "check", "origin", "remote literal", ""}, "\x00"))))
				Expect(os.ReadFile(input)).To(Equal([]byte("borrowed input\n")))
				_, err := os.Lstat(marker)
				Expect(os.IsNotExist(err)).To(BeTrue())
			}
			// Refuse malformed imported metadata before interpreting any source owner.
			// per docs/adr/0138-go-source-check-orchestration.md:123
			for _, scalar := range []string{"", "forged/source/path", "declare -a BASH_SOURCE=([0]=forged)"} {
				root, caller := fixtureDirectories()
				program := sourceCheckAdapter(filepath.Join(root, "scripts/pre-push-check.sh"))
				marker := filepath.Join(filepath.Dir(root), "SCALAR_OWNER_CALLED")
				writeFixture(filepath.Join(root, "factory"), []byte("#!/bin/sh\nprintf called > \"$CHECK_DECOY\"\nexit 97\n"), 0700)
				environment := map[string]string{"PATH": "/usr/bin:/bin", "BASH_ENV": "", "ENV": "", "BASH_SOURCE": scalar, "CHECK_DECOY": marker}
				out := handoffNativeExactProcess(caller, environment, "", shell, program)
				Expect(out.status).To(Equal(2), "%s scalar=%q: %+v", shell, scalar, out)
				Expect(out.stdout).To(BeEmpty())
				Expect(out.stderr).To(ContainSubstring("native Bash source metadata"))
				_, err := os.Lstat(marker)
				Expect(os.IsNotExist(err)).To(BeTrue())
			}
		}
	})

	// per docs/adr/0138-go-source-check-orchestration.md:33
	// per docs/adr/0138-go-source-check-orchestration.md:36
	// per docs/adr/0138-go-source-check-orchestration.md:101
	It("selects the copied hook owner from explicit project then caller Git root then cwd", func() {
		for _, mode := range []string{"absolute", "relative", "git", "empty-root", "cwd", "failed-git", "relative-alias-parent", "absolute-alias-parent"} {
			root, caller := fixtureDirectories()
			program := sourceCheckAdapter(filepath.Join(root, "copied hook/pre-push"))
			selected, cwd, requested, wrongOwner := caller, caller, "", ""
			if mode == "absolute" || mode == "relative" {
				selected = filepath.Join(caller, "selected ' project\n")
				Expect(os.Mkdir(selected, 0700)).To(Succeed())
				requested = selected
				if mode == "relative" {
					requested = "./" + filepath.Base(selected)
				}
			} else if strings.HasSuffix(mode, "alias-parent") {
				physical := filepath.Join(filepath.Dir(root), "physical target parent")
				Expect(os.MkdirAll(filepath.Join(physical, "child"), 0700)).To(Succeed())
				alias := filepath.Join(caller, "alias")
				Expect(os.Symlink(filepath.Join(physical, "child"), alias)).To(Succeed())
				requested, wrongOwner = "alias/..", physical
				if mode == "absolute-alias-parent" {
					requested, selected, wrongOwner = alias+"/..", physical, caller
				}
			} else if mode != "cwd" && mode != "failed-git" {
				imageGit(caller, "init", "-q")
				cwd = filepath.Join(caller, "nested caller")
				Expect(os.Mkdir(cwd, 0700)).To(Succeed())
			}
			base := filepath.Dir(root)
			record, input, marker := filepath.Join(base, "argv"), filepath.Join(base, "stdin"), filepath.Join(base, "OLD_SOURCE_OWNER")
			writeFixture(filepath.Join(root, "factory"), []byte("#!/bin/sh\nprintf called > \"$CHECK_DECOY\"\nexit 97\n"), 0700)
			if wrongOwner != "" {
				writeFixture(filepath.Join(wrongOwner, "factory"), []byte("#!/bin/sh\nprintf called > \"$CHECK_DECOY\"\nexit 97\n"), 0700)
			}
			writeFixture(filepath.Join(selected, "factory"), []byte(sourceCheckRecorder), 0700)
			git, err := exec.LookPath("git")
			Expect(err).NotTo(HaveOccurred())
			environment := map[string]string{"PATH": filepath.Dir(git) + ":/usr/bin:/bin", "BASH_ENV": "", "ENV": "", "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull, "FACTORY_RUNTIME_BINARY": filepath.Join(root, "factory"), "CHECK_LITERAL": "literal caller value", "CHECK_ARGV": record, "CHECK_INPUT": input, "CHECK_DECOY": marker}
			if mode == "failed-git" {
				tools := filepath.Join(base, "failed discovery")
				writeFixture(filepath.Join(tools, "git"), []byte("#!/bin/sh\n/bin/cat > \"$CHECK_GIT_INPUT\"\nprintf '%s\\n' \"$CHECK_GIT_DECOY\"\nexit 1\n"), 0700)
				environment["PATH"] = tools + ":" + environment["PATH"]
				environment["CHECK_GIT_INPUT"], environment["CHECK_GIT_DECOY"] = filepath.Join(base, "git stdin"), root
			}
			if requested != "" || mode == "empty-root" {
				environment["REPO_ROOT"] = requested
			}
			args := []string{"origin", "remote ' $(touch CHECK_INJECTION)\npath", "--help", ""}
			stdin := "actual copied-hook input\x00tail\n"
			out := handoffNativeExactProcess(cwd, environment, stdin, program, args...)
			Expect(out).To(Equal(cliResult{"selected stdout\n", "selected stderr\n", 63}), "%s", mode)
			expected := append([]string{cwd, environment["CHECK_LITERAL"], "", "", requested, "check"}, args...)
			Expect(os.ReadFile(record)).To(Equal([]byte(strings.Join(expected, "\x00") + "\x00")))
			Expect(os.ReadFile(input)).To(Equal([]byte(stdin)))
			_, err = os.Lstat(marker)
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	})

	// per docs/adr/0138-go-source-check-orchestration.md:18
	// per docs/adr/0138-go-source-check-orchestration.md:38
	// per docs/adr/0138-go-source-check-orchestration.md:110
	It("refuses missing nonexecutable and file-aliased owners without another controller", func() {
		for _, copied := range []bool{false, true} {
			for _, mode := range []string{"missing", "nonexecutable", "failed", "file-alias", "absent/..", "dangling/..", "file/.."} {
				invalidRelative := strings.HasSuffix(mode, "/..")
				if invalidRelative && !copied {
					continue
				}
				root, caller := fixtureDirectories()
				program := sourceCheckAdapter(filepath.Join(root, "scripts/pre-push-check.sh"))
				owner := filepath.Join(root, "factory")
				if copied {
					program = sourceCheckAdapter(filepath.Join(root, "copied/pre-push"))
					owner = filepath.Join(caller, "factory")
				}
				base := filepath.Dir(root)
				marker, tools, foreign := filepath.Join(base, "FALLBACK"), filepath.Join(base, "tools"), filepath.Join(base, "foreign")
				poison := []byte("#!/bin/sh\nprintf called > \"$CHECK_DECOY\"\nexit 97\n")
				for _, path := range []string{filepath.Join(tools, "factory"), filepath.Join(foreign, "factory"), filepath.Join(caller, "scripts/hooks/direct-main-push-block.sh")} {
					writeFixture(path, poison, 0700)
				}
				if copied {
					writeFixture(filepath.Join(root, "factory"), poison, 0700)
				}
				if mode != "missing" {
					writeFixture(owner, []byte("#!/bin/sh\nprintf 'owner stdout\\n'\nprintf 'owner failed\\n' >&2\nexit 47\n"), 0700)
				}
				if mode == "nonexecutable" {
					Expect(os.Chmod(owner, 0600)).To(Succeed())
				}
				if mode == "file-alias" {
					alias := filepath.Join(base, "alias", filepath.Base(program))
					Expect(os.MkdirAll(filepath.Dir(alias), 0700)).To(Succeed())
					Expect(os.Symlink(program, alias)).To(Succeed())
					program = alias
				}
				environment := map[string]string{"PATH": tools + ":/usr/bin:/bin", "REPO_ROOT": caller, "BASH_ENV": "", "ENV": "", "CHECK_DECOY": marker, "FACTORY_RUNTIME_BINARY": filepath.Join(foreign, "factory"), "FACTORY_HOME": foreign}
				if invalidRelative {
					environment["REPO_ROOT"] = mode
					writeFixture(owner, poison, 0700)
					switch mode {
					case "dangling/..":
						Expect(os.Symlink("absent", filepath.Join(caller, "dangling"))).To(Succeed())
					case "file/..":
						writeFixture(filepath.Join(caller, "file"), []byte("caller file"), 0600)
					}
				}
				beforeOwner, beforeCaller := installationSnapshot(root), installationSnapshot(caller)
				out := handoffNativeExactProcess(caller, environment, "borrowed input\n", program, "origin", "remote")
				if mode == "failed" {
					Expect(out).To(Equal(cliResult{"owner stdout\n", "owner failed\n", 47}))
				} else {
					Expect(out.status).NotTo(BeZero(), "copied=%t mode=%s: %+v", copied, mode, out)
					Expect(out.stdout).To(BeEmpty())
					Expect(out.stderr).NotTo(BeEmpty())
				}
				_, err := os.Lstat(marker)
				Expect(os.IsNotExist(err)).To(BeTrue())
				Expect(installationSnapshot(root)).To(Equal(beforeOwner))
				Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
			}
		}
	})

	// per docs/adr/0138-go-source-check-orchestration.md:20
	// per docs/adr/0138-go-source-check-orchestration.md:67
	It("keeps compiler refusal owned by the source launcher without using a stale runtime", func() {
		for _, missing := range []bool{false, true} {
			root, caller := fixtureDirectories()
			program := sourceCheckAdapter(filepath.Join(root, "scripts/pre-push-check.sh"))
			launcher, err := os.ReadFile("../factory")
			Expect(err).NotTo(HaveOccurred())
			writeFixture(filepath.Join(root, "factory"), launcher, 0755)
			base := filepath.Dir(root)
			tools, marker, record := filepath.Join(base, "tools"), filepath.Join(base, "STALE_EXECUTED"), filepath.Join(base, "compiler calls")
			writeFixture(filepath.Join(root, "factory-go"), []byte("#!/bin/sh\nprintf called > \"$CHECK_DECOY\"\nexit 97\n"), 0700)
			writeFixture(filepath.Join(tools, "go"), []byte("#!/bin/sh\nprintf called > \"$CHECK_COMPILER\"\nprintf 'controlled compiler failure\\n' >&2\nexit 58\n"), 0700)
			Expect(os.Symlink("/bin/bash", filepath.Join(tools, "bash"))).To(Succeed())
			environment := map[string]string{"PATH": tools + ":/usr/bin:/bin", "BASH_ENV": "", "ENV": "", "CHECK_DECOY": marker, "CHECK_COMPILER": record}
			if missing {
				Expect(os.Remove(filepath.Join(tools, "go"))).To(Succeed())
				environment["PATH"] = tools
				lookup := handoffNativeExactProcess(caller, environment, "", "/bin/bash", "-c", "command -v go")
				Expect(lookup).To(Equal(cliResult{"", "", 1}))
			}
			before := installationSnapshot(root)
			out := handoffNativeExactProcess(caller, environment, "borrowed input\n", program)
			Expect(installationSnapshot(root)).To(Equal(before))
			if missing {
				Expect(out.status).To(Equal(1))
				Expect(out.stderr).To(ContainSubstring("qualified Go toolchain"))
				_, err = os.Lstat(record)
				Expect(os.IsNotExist(err)).To(BeTrue())
			} else {
				Expect(out.status).To(Equal(58), "%+v", out)
				Expect(out.stderr).To(ContainSubstring("controlled compiler failure"))
				Expect(os.ReadFile(record)).To(Equal([]byte("called")))
			}
			_, err = os.Lstat(marker)
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	})

	// per docs/adr/0138-go-source-check-orchestration.md:48
	// per docs/adr/0138-go-source-check-orchestration.md:72
	It("compiles the source owner and retains native stage order failures and literal local hooks", func() {
		f := newOrchestrationFixture()
		source := newSourceEntrypointFixture()
		program := sourceCheckAdapter(filepath.Join(source.root, "scripts/pre-push-check.sh"))
		environment := sourceCheckEnvironment(f)
		maps.Copy(environment, source.environment)
		maps.Copy(environment, sourceReviewGoCaches())
		environment["PATH"] = f.bin + ":" + environment["PATH"]
		environment["REPO_ROOT"], environment["FACTORY_CONFIG"] = f.repo, "selected.yaml"
		writeFixture(filepath.Join(f.repo, "selected.yaml"), []byte("local_hooks: local-a.sh two words,local-b.sh --*.txt\n"), 0600)
		writeFixture(filepath.Join(f.repo, "--matched.txt"), nil, 0600)
		for _, name := range []string{"local-a.sh", "local-b.sh"} {
			writeFixture(filepath.Join(f.repo, name), []byte("#!/bin/sh\nprintf '%s' \"${0##*/}\" >> \"$CHECK_TRACE\"\nfor argument do printf '\\t%s' \"$argument\" >> \"$CHECK_TRACE\"; done\nprintf '\\n' >> \"$CHECK_TRACE\"\nexit \"${CHECK_FAIL_LOCAL:-0}\"\n"), 0700)
		}
		roleRecord := filepath.Join(filepath.Dir(f.root), "role presence")
		environment["CHECK_ROLE_RECORD"] = roleRecord
		f.gate("pending-lessons-push-block", "printf '%s\\000%s' \"${FACTORY_AGENT_ROLE+x}\" \"${FACTORY_AGENT_ROLE-}\" > \"$CHECK_ROLE_RECORD\"\n")
		cwd := filepath.Join(f.repo, "nested caller")
		Expect(os.Mkdir(cwd, 0700)).To(Succeed())
		before := sourceEntrypointSnapshot(source.root)
		stdin := "refs/heads/topic abc refs/heads/topic def\nNUL:\x00tail\n"
		for _, failing := range []bool{false, true} {
			writeFixture(f.trace, nil, 0600)
			if failing {
				compiled, err := os.ReadFile(filepath.Join(source.root, "factory-go"))
				Expect(err).NotTo(HaveOccurred(), "the source entrypoint must have compiled its actual owner")
				writeFixture(filepath.Join(source.root, "factory"), compiled, 0755)
				environment["CHECK_FAIL_MAKE_CHECK"], environment["CHECK_FAIL_LOCAL"], environment["FACTORY_AGENT_ROLE"] = "2", "7", "implementer"
			}
			out := handoffNativeExactProcess(cwd, environment, stdin, program, "--help", "--config=not-selected", "literal $(touch CHECK_INJECTION)")
			Expect(orchestrationRead(f.input)).To(Equal(stdin))
			trace := "direct-main-push-block\nmake\tcheck\nmake\tcheck-drift\ncommit-message-lint\t" + f.second + "\ncommit-message-lint\t" + f.first + "\ndiff-aware-check\t" + f.base + "\tHEAD\ndecision-log-gate\t" + f.base + "\tHEAD\npending-lessons-push-block\nlocal-a.sh\ttwo\twords\nlocal-b.sh\t--*.txt\n"
			Expect(orchestrationRead(f.trace)).To(Equal(trace))
			if failing {
				Expect(out.status).To(Equal(1), "%+v", out)
				Expect(out.stdout).To(ContainSubstring("PRE-PUSH: 3 CHECK(S) FAILED"))
				Expect(os.ReadFile(roleRecord)).To(Equal([]byte("x\x00implementer")))
			} else {
				Expect(out.status).To(BeZero(), "%+v", out)
				Expect(out.stdout).To(ContainSubstring("PUSH ALLOWED"))
				Expect(os.ReadFile(roleRecord)).To(Equal([]byte("\x00")))
				Expect(sourceEntrypointSnapshot(source.root)).To(Equal(before))
				compiled, err := os.ReadFile(filepath.Join(source.root, "factory-go"))
				Expect(err).NotTo(HaveOccurred())
				Expect(compiled).NotTo(BeEmpty())
			}
			_, err := os.Lstat(filepath.Join(f.repo, "CHECK_INJECTION"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	})

	// per docs/adr/0138-go-source-check-orchestration.md:42
	// per docs/adr/0138-go-source-check-orchestration.md:48
	It("enters the caller native owner for real copied-hook pushes and denies main before later stages", func() {
		f := newOrchestrationFixture()
		sourceHelperLibraryAssets(f.repo)
		gate, err := os.ReadFile("../scripts/hooks/direct-main-push-block.sh")
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(f.repo, "scripts/hooks/direct-main-push-block.sh"), gate, 0755)
		sourceCheckAdapter(filepath.Join(f.repo, ".git/hooks/pre-push"))
		orchestrationGit(f.repo, "config", "--unset", "core.hooksPath")
		remote := filepath.Join(filepath.Dir(f.root), "bare remote ' $(literal)")
		Expect(os.Mkdir(remote, 0700)).To(Succeed())
		orchestrationGit(remote, "init", "--bare", "-q")
		orchestrationGit(f.repo, "remote", "add", "fixture", remote)
		environment := sourceCheckEnvironment(f)
		record := filepath.Join(filepath.Dir(f.root), "native check argv")
		environment["CHECK_NATIVE_ARGV"], environment["CHECK_NATIVE"] = record, filepath.Join(f.root, "native")
		writeFixture(environment["CHECK_NATIVE"], candidateBinary, 0700)
		writeFixture(filepath.Join(f.repo, "factory"), []byte("#!/bin/sh\nif [ \"$1\" = check ]; then printf '%s\\000' \"$@\" > \"$CHECK_NATIVE_ARGV\"; fi\nexec \"$CHECK_NATIVE\" \"$@\"\n"), 0700)
		for _, branch := range []string{"main", "topic"} {
			out := handoffNativeExactProcess(f.repo, environment, "", "git", "push", "fixture", "HEAD:refs/heads/"+branch)
			Expect(os.ReadFile(record)).To(Equal([]byte("check\x00fixture\x00"+remote+"\x00")), "Git must enter the copied adapter's selected native owner")
			if branch == "main" {
				Expect(out.status).NotTo(BeZero(), "%+v", out)
				Expect(out.stdout + out.stderr).To(ContainSubstring("direct push to main"))
				Expect(orchestrationRead(f.trace)).To(BeEmpty())
				ref := handoffNativeExactProcess(remote, environment, "", "git", "rev-parse", "--verify", "refs/heads/main")
				Expect(ref.status).NotTo(BeZero())
			} else {
				Expect(out.status).To(BeZero(), "%+v", out)
				Expect(orchestrationRead(f.trace)).To(ContainSubstring("make\tcheck\nmake\tcheck-drift\n"))
				Expect(orchestrationGit(remote, "rev-parse", "refs/heads/topic")).To(Equal(f.second))
			}
		}
		// The selected runtime and the actual native checker must agree on path bytes.
		// per docs/adr/0138-go-source-check-orchestration.md:138
		newline := newOrchestrationFixture()
		project := filepath.Join(filepath.Dir(newline.repo), "newline project\n")
		Expect(os.Rename(newline.repo, project)).To(Succeed())
		newline.repo, newline.trace, newline.input = project, filepath.Join(project, "stage-trace"), filepath.Join(project, "gate-input")
		sibling := strings.TrimSuffix(project, "\n")
		base := filepath.Dir(newline.root)
		wrongRuntime, wrongGate := filepath.Join(base, "WRONG_RUNTIME"), filepath.Join(base, "WRONG_GATE")
		writeFixture(filepath.Join(sibling, "factory"), []byte("#!/bin/sh\nprintf called > \"$CHECK_WRONG_RUNTIME\"\nexit 96\n"), 0700)
		writeFixture(filepath.Join(sibling, "scripts/hooks/direct-main-push-block.sh"), []byte("#!/bin/sh\nprintf called > \"$CHECK_WRONG_GATE\"\nexit 27\n"), 0700)
		program := sourceCheckAdapter(filepath.Join(project, ".git/hooks/pre-push"))
		writeFixture(filepath.Join(project, "factory"), []byte("#!/bin/sh\nprintf '%s\\000' \"$PWD\" \"${REPO_ROOT+x}\" \"${REPO_ROOT-}\" \"$@\" > \"$CHECK_NATIVE_ARGV\"\nexec \"$CHECK_NATIVE\" \"$@\"\n"), 0700)
		environment = sourceCheckEnvironment(newline)
		maps.Copy(environment, map[string]string{"CHECK_TRACE": newline.trace, "CHECK_STDIN": newline.input, "CHECK_NATIVE": filepath.Join(newline.root, "factory"), "CHECK_NATIVE_ARGV": filepath.Join(base, "newline native argv"), "CHECK_WRONG_RUNTIME": wrongRuntime, "CHECK_WRONG_GATE": wrongGate, "REPO_ROOT": "", "CHECK_ROOT_RECORD": filepath.Join(base, "native root environment")})
		newline.gate("pending-lessons-push-block", "printf '%s\\000%s' \"${REPO_ROOT+x}\" \"${REPO_ROOT-}\" > \"$CHECK_ROOT_RECORD\"\n")
		cwd := filepath.Join(project, "nested caller")
		Expect(os.Mkdir(cwd, 0700)).To(Succeed())
		input := "refs/heads/topic abc refs/heads/topic def\nNUL:\x00tail\n"
		out := handoffNativeExactProcess(cwd, environment, input, program, "origin", "literal remote")
		Expect(os.ReadFile(environment["CHECK_NATIVE_ARGV"])).To(Equal([]byte(strings.Join([]string{cwd, "x", "", "check", "origin", "literal remote", ""}, "\x00"))))
		_, runtimeErr := os.Lstat(wrongRuntime)
		_, gateErr := os.Lstat(wrongGate)
		AddReportEntry("source-check.newline-project", map[string]any{"project": project, "sibling": sibling, "status": out.status, "stdout": out.stdout, "stderr": out.stderr, "wrong_runtime_called": runtimeErr == nil, "wrong_gate_called": gateErr == nil}, ReportEntryVisibilityNever)
		Expect(os.IsNotExist(runtimeErr)).To(BeTrue(), "copied-hook selection must preserve the newline project")
		Expect(os.IsNotExist(gateErr)).To(BeTrue(), "the actual native check must not validate the sibling without its trailing newline")
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(orchestrationRead(newline.input)).To(Equal(input))
		Expect(orchestrationRead(newline.trace)).To(HavePrefix("direct-main-push-block\nmake\tcheck\nmake\tcheck-drift\n"))
		Expect(os.ReadFile(environment["CHECK_ROOT_RECORD"])).To(Equal([]byte("x\x00")))
	})

	// per docs/adr/0138-go-source-check-orchestration.md:53
	// per docs/adr/0138-go-source-check-orchestration.md:69
	It("stops later stages and reaps the active native gate when either source invocation is cancelled", func() {
		for _, copied := range []bool{false, true} {
			f := newOrchestrationFixture()
			program := sourceCheckAdapter(filepath.Join(f.root, "scripts/pre-push-check.sh"))
			if copied {
				program = sourceCheckAdapter(filepath.Join(f.repo, ".git/hooks/pre-push"))
				writeFixture(filepath.Join(f.repo, "factory"), candidateBinary, 0700)
			}
			f.gate("direct-main-push-block", "printf '%s' \"$$\" > active-gate\nwhile :; do /bin/sleep 1; done\n")
			output, err := os.CreateTemp(filepath.Dir(f.repo), "source-check-cancel-")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(output.Close)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			DeferCleanup(cancel)
			command := handoffCommand(ctx, f.repo, sourceCheckEnvironment(f), "", program)
			command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			command.Stdout, command.Stderr = output, output
			Expect(command.Start()).To(Succeed())
			done := make(chan error, 1)
			go func() { done <- command.Wait() }()
			joined, closed := false, false
			gatePID, gateGroup := 0, 0
			DeferCleanup(func() {
				if closed {
					return
				}
				if gateGroup == gatePID && gatePID > 1 {
					_ = syscall.Kill(-gateGroup, syscall.SIGKILL)
				}
				_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
				if !joined {
					select {
					case <-done:
					case <-time.After(time.Second):
						holdFixtureCleanup(filepath.Dir(f.repo), "source check cancellation did not join its isolated command")
					}
				}
			})
			Eventually(func() string { return orchestrationRead(filepath.Join(f.repo, "active-gate")) }, 3*time.Second, 20*time.Millisecond).ShouldNot(BeEmpty())
			gatePID, err = strconv.Atoi(orchestrationRead(filepath.Join(f.repo, "active-gate")))
			Expect(err).NotTo(HaveOccurred())
			gateGroup, err = syscall.Getpgid(gatePID)
			Expect(err).NotTo(HaveOccurred())
			Expect(command.Process.Signal(syscall.SIGINT)).To(Succeed())
			Eventually(done, 4*time.Second).Should(Receive(HaveOccurred()))
			joined = true
			Expect(orchestrationRead(f.trace)).To(Equal("direct-main-push-block\n"))
			Eventually(func() error { return syscall.Kill(gatePID, 0) }, 3*time.Second).Should(MatchError(syscall.ESRCH))
			Eventually(func() error { return syscall.Kill(-gateGroup, 0) }, time.Second).Should(MatchError(syscall.ESRCH))
			Eventually(func() error { return syscall.Kill(-command.Process.Pid, 0) }, time.Second).Should(MatchError(syscall.ESRCH))
			closed = true
			AddReportEntry("source-check.cancelled", fmt.Sprintf("copied=%t caller=%d gate=%d gate_group=%d joined=true", copied, command.Process.Pid, gatePID, gateGroup), ReportEntryVisibilityNever)
		}
	})
})
