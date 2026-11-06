package acceptance_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Independently enumerate the public source boundary and its canonical closure.
// per docs/adr/0137-go-source-shared-helpers.md:77
func sourceHelperLibraryAssets(root string) {
	GinkgoHelper()
	sourceConfigLibraryAssets(root)
	for _, name := range []string{"events", "timing", "color", "hookspath"} {
		path := filepath.Join("scripts/lib", name+".sh")
		body, err := os.ReadFile(filepath.Join("..", path))
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(root, path), body, 0644)
	}
}

// The recorder observes literal requests while the actual candidate owns effects.
// per docs/adr/0137-go-source-shared-helpers.md:99
func sourceHelperNativeFixture() (string, string, map[string]string) {
	GinkgoHelper()
	root, caller, environment := sourceConfigNativeFixture()
	sourceHelperLibraryAssets(root)
	git, err := exec.LookPath("git")
	Expect(err).NotTo(HaveOccurred())
	git, err = filepath.EvalSymlinks(git)
	Expect(err).NotTo(HaveOccurred())
	environment["PATH"] = filepath.Dir(git) + ":/usr/bin:/bin"
	return root, caller, environment
}

// A real PTY supplies only stdout's terminal property; no Python or shell driver.
// per docs/adr/0137-go-source-shared-helpers.md:60
func sourceHelperTerminal(cwd string, environment map[string]string, shell, probe string, args ...string) cliResult {
	GinkgoHelper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	Expect(err).NotTo(HaveOccurred())
	defer master.Close()
	name, err := sourceHelperPTYSlave(master)
	Expect(err).NotTo(HaveOccurred())
	slave, err := os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0)
	Expect(err).NotTo(HaveOccurred())
	defer slave.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, shell, append([]string{"-c", probe, "helper-tty"}, args...)...) // #nosec G204 -- fixed test-owned shell and probe with literal fixture operands.
	command.Dir, command.Stdout = cwd, slave
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }
	for key, value := range environment {
		command.Env = append(command.Env, key+"="+value)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	Expect(command.Start()).To(Succeed())
	Expect(slave.Close()).To(Succeed())
	output := make(chan []byte, 1)
	go func() { body, _ := io.ReadAll(master); output <- body }()
	err = command.Wait()
	Expect(ctx.Err()).NotTo(HaveOccurred())
	status := 0
	if err != nil {
		var exited *exec.ExitError
		Expect(errors.As(err, &exited)).To(BeTrue())
		status = exited.ExitCode()
	}
	var stdout []byte
	Eventually(output).WithTimeout(time.Second).Should(Receive(&stdout))
	return cliResult{strings.ReplaceAll(string(stdout), "\r\n", "\n"), stderr.String(), status}
}

var _ = Describe("Go source runtime entrypoints shared helper adapters", func() {
	// per docs/adr/0137-go-source-shared-helpers.md:19
	// per docs/adr/0137-go-source-shared-helpers.md:29
	It("retains the physical Bash owner and literal child controls after caller directory changes", func() {
		for _, alias := range []bool{false, true} {
			root, caller, environment := sourceHelperNativeFixture()
			base := filepath.Dir(root)
			owner := filepath.Join(base, "helper owner ' $(literal)\n")
			Expect(os.Rename(root, owner)).To(Succeed())
			environment["CONFIG_NATIVE"] = filepath.Join(owner, "native")
			imageGit(caller, "init", "-q")
			foreign, marker := filepath.Join(base, "foreign"), filepath.Join(base, "FOREIGN_OWNER")
			writeFixture(filepath.Join(foreign, "factory"), []byte("#!/bin/sh\nprintf called > \"$HELPER_DECOY\"\nexit 97\n"), 0700)
			environment["HELPER_DECOY"] = marker
			environment["FACTORY_RUNTIME_BINARY"], environment["FACTORY_SOURCE_ROOT"] = filepath.Join(foreign, "factory"), foreign
			library := filepath.Join(owner, "scripts/lib")
			if alias {
				linked := filepath.Join(base, "directory alias")
				Expect(os.Symlink(library, linked)).To(Succeed())
				library = linked
			}
			log := filepath.Join(base, "literal event log")
			title, line := "title ' $(touch HELPER_INJECTION)", "literal line\nsecond \\ line"
			probe := `set -eu; FACTORY_BRIDGE_PROTOCOL=caller; FACTORY_EVENT_LOG="$5"; FACTORY_EVENT_MAX_LINES=1; NO_COLOR=caller; readonly FACTORY_CONFIG FACTORY_BRIDGE_PROTOCOL FACTORY_RUNTIME_BINARY FACTORY_SOURCE_ROOT FACTORY_EVENT_LOG FACTORY_EVENT_MAX_LINES NO_COLOR; . "$1/events.sh"; . "$1/timing.sh"; . "$1/hookspath.sh"; . "$1/color.sh"; cd "$2"; factory_duration 10 75; printf '\000'; factory_log_event 'literal gate' 'literal reason'; hookspath_status "$2"; printf '\000'; C_RESET='<R>'; C_BOLD='<B>'; C_YELLOW='<Y>'; action_box "$3" "$4"; printf '%s' "$FACTORY_BRIDGE_PROTOCOL"`
			out := handoffNativeExactProcess(owner, environment, "", "/bin/bash", "-c", probe, "source-helper-owner", library, caller, title, line, log)
			expected := "1m 5s\x00absent\t" + filepath.Join(caller, ".git/hooks/pre-push") + "\x00<Y><B>┌─ " + title + " <R>\n<Y>│<R> " + line + "\n<Y>└─<R>\ncaller"
			Expect(out).To(Equal(cliResult{expected, "", 0}), "directory alias=%t", alias)
			calls, err := os.ReadFile(environment["CONFIG_CALLS"])
			Expect(err).NotTo(HaveOccurred(), "each public helper must enter its physical native owner")
			Expect(string(calls)).To(Equal(strings.Join([]string{"helper", "palette", "0", "helper", "duration", "10", "75", "helper", "event", "literal gate", "literal reason", "helper", "hookspath", caller, "helper", "action-box", "<R>", "<B>", "<Y>", title, line, ""}, "\x00")))
			event, err := os.ReadFile(log)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(event)).To(HaveSuffix("\tliteral gate\tliteral reason\n"))
			for _, path := range []string{marker, filepath.Join(caller, "HELPER_INJECTION")} {
				_, err := os.Lstat(path)
				Expect(os.IsNotExist(err)).To(BeTrue())
			}
		}
		// Temporary loaders disappear after successful imports of all seven libraries.
		// per docs/adr/0137-go-source-shared-helpers.md:131
		for _, library := range []string{"config", "roles", "budget-config", "events", "timing", "color", "hookspath"} {
			root, caller, environment := sourceHelperNativeFixture()
			imageGit(caller, "init", "-q")
			loader := "_factory_source_" + strings.TrimSuffix(library, "-config") + "_load"
			probe := `. "$1" || exit "$?"; if builtin declare -F "$2" >/dev/null; then printf loader-remains; fi`
			out := handoffNativeExactProcess(caller, environment, "", "/bin/bash", "-c", probe, "helper-loader-success", filepath.Join(root, "scripts/lib", library+".sh"), loader)
			Expect(out).To(Equal(cliResult{"", "", 0}), "%s", library)
		}
		// A file alias cannot establish a different source owner.
		// per docs/adr/0137-go-source-shared-helpers.md:23
		// per docs/adr/0137-go-source-shared-helpers.md:132
		for _, library := range []string{"config", "roles", "budget-config", "events", "timing", "color", "hookspath"} {
			root, caller, environment := sourceHelperNativeFixture()
			alias := filepath.Join(caller, library+" alias")
			Expect(os.Symlink(filepath.Join(root, "scripts/lib", library+".sh"), alias)).To(Succeed())
			beforeOwner, beforeCaller := installationSnapshot(root), installationSnapshot(caller)
			loader := "_factory_source_" + strings.TrimSuffix(library, "-config") + "_load"
			probe := `. "$1"; status=$?; if builtin declare -F "$2" >/dev/null; then printf loader-remains; fi; exit "$status"`
			out := handoffNativeExactProcess(caller, environment, "", "/bin/bash", "-c", probe, "helper-file-alias", alias, loader)
			Expect(out.status).To(Equal(2), "%s: %+v", library, out)
			Expect(out.stdout).To(BeEmpty())
			Expect(out.stderr).To(ContainSubstring("file symlink"))
			_, err := os.Lstat(environment["CONFIG_CALLS"])
			Expect(os.IsNotExist(err)).To(BeTrue())
			Expect(installationSnapshot(root)).To(Equal(beforeOwner))
			Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
		}
	})

	// per docs/adr/0137-go-source-shared-helpers.md:27
	// per docs/adr/0137-go-source-shared-helpers.md:36
	It("sources non-color definitions without commands or caller-state changes in Bash and POSIX shells", func() {
		for _, shell := range sourceConfigRoleShells() {
			for _, library := range []string{"events", "timing", "hookspath"} {
				root, caller, environment := sourceHelperNativeFixture()
				if capability := handoffNativeExactProcess(caller, environment, "", shell, "-c", `printf '%s' "${BASH_VERSION-}"`); capability.stdout == "" {
					environment["BASH_VERSION"], environment["BASH_SOURCE"] = "forged", "forged"
				}
				marker, tools := filepath.Join(filepath.Dir(root), "SOURCE_EFFECT"), filepath.Join(filepath.Dir(root), "forbidden tools")
				poison := []byte("#!/bin/sh\nprintf called > \"$HELPER_DECOY\"\nexit 97\n")
				for _, path := range []string{filepath.Join(root, "factory"), filepath.Join(caller, "builtin"), filepath.Join(tools, "go"), filepath.Join(tools, "git"), filepath.Join(tools, "date")} {
					writeFixture(path, poison, 0700)
				}
				environment["PATH"], environment["HELPER_DECOY"] = ":"+tools, marker
				probe := `set -euf; IFS=:; FACTORY_RUNTIME_BINARY=caller; FACTORY_SOURCE_ROOT=caller; FACTORY_CONFIG=caller; FACTORY_BRIDGE_PROTOCOL=caller; readonly FACTORY_RUNTIME_BINARY FACTORY_SOURCE_ROOT FACTORY_CONFIG FACTORY_BRIDGE_PROTOCOL; cd() { command printf cd > "$HELPER_DECOY"; return 77; }; pwd() { command printf pwd > "$HELPER_DECOY"; return 78; }; printf() { command printf printf > "$HELPER_DECOY"; return 79; }; options="$-"; directory="$PWD"; . "$1"; [ "$options" = "$-" ] && [ "$directory" = "$PWD" ] && [ "$IFS" = : ] && [ "$#" = 2 ] && [ "$2" = 'literal argument' ] || exit 96; command printf '%s\000%s\000%s\000%s' "$FACTORY_RUNTIME_BINARY" "$FACTORY_SOURCE_ROOT" "$FACTORY_CONFIG" "$FACTORY_BRIDGE_PROTOCOL"`
				out := handoffNativeExactProcess(caller, environment, "", shell, "-c", probe, "helper-inert", filepath.Join(root, "scripts/lib", library+".sh"), "literal argument")
				Expect(out).To(Equal(cliResult{"caller\x00caller\x00caller\x00caller", "", 0}), "%s/%s", shell, library)
				_, err := os.Lstat(marker)
				Expect(os.IsNotExist(err)).To(BeTrue())
			}
		}
	})

	// per docs/adr/0137-go-source-shared-helpers.md:38
	// per docs/adr/0137-go-source-shared-helpers.md:56
	It("selects POSIX owners lazily and retains helper-specific refusal behavior without fallback", func() {
		// Actual Bash can inherit an exported scalar in place of source-array metadata.
		// per docs/adr/0137-go-source-shared-helpers.md:123
		// per docs/adr/0137-go-source-shared-helpers.md:132
		shells := []string{"/bin/bash"}
		if bash, err := exec.LookPath("bash"); err == nil && bash != shells[0] {
			shells = append(shells, bash)
		}
		for _, shell := range shells {
			for _, library := range []string{"config", "roles", "budget-config", "events", "timing", "color", "hookspath"} {
				root, caller, environment := sourceHelperNativeFixture()
				path := filepath.Join(root, "scripts/lib", library+".sh")
				environment["FACTORY_RUNTIME_BINARY"], environment["FACTORY_SOURCE_ROOT"] = filepath.Join(root, "factory"), root
				loader := "_factory_source_" + strings.TrimSuffix(library, "-config") + "_load"
				for _, scalar := range []string{"", path, "declare -a BASH_SOURCE=([0]=forged)"} {
					environment["BASH_SOURCE"] = scalar
					probe := `before=$(builtin declare -p BASH_SOURCE); . "$1"; status=$?; builtin printf '%s\000' "$status"; [ "$before" = "$(builtin declare -p BASH_SOURCE)" ] || builtin printf metadata-changed; if builtin declare -F "$2" >/dev/null; then builtin printf loader-remains; fi; if [ "$3" = events ]; then factory_log_event gate reason; builtin printf '%s' "$?"; fi`
					out := handoffNativeExactProcess(caller, environment, "", shell, "-c", probe, "helper-scalar-metadata", path, loader, library)
					if library == "events" {
						Expect(out).To(Equal(cliResult{"0\x000", "", 0}), "%s/%s scalar=%q", shell, library, scalar)
					} else {
						Expect(out.status).To(BeZero(), "%s/%s scalar=%q: %+v", shell, library, scalar, out)
						Expect(out.stdout).To(Equal("2\x00"), "%s/%s scalar=%q: %+v", shell, library, scalar, out)
						Expect(out.stderr).To(ContainSubstring("native Bash source metadata"))
					}
					_, err := os.Lstat(environment["CONFIG_CALLS"])
					Expect(os.IsNotExist(err)).To(BeTrue(), "%s/%s scalar=%q must not execute an owner", shell, library, scalar)
				}
			}
		}
		posix := false
		for _, shell := range sourceConfigRoleShells() {
			root, caller, environment := sourceHelperNativeFixture()
			if handoffNativeExactProcess(caller, environment, "", shell, "-c", `printf '%s' "${BASH_VERSION-}"`).stdout != "" {
				continue
			}
			posix = true
			environment["FACTORY_RUNTIME_BINARY"] = filepath.Join(root, "factory")
			probe := `. "$1/timing.sh"; factory_duration 10 15`
			Expect(handoffNativeExactProcess(caller, environment, "", shell, "-c", probe, "explicit-helper", filepath.Join(root, "scripts/lib"))).To(Equal(cliResult{"5s", "", 0}))
			Expect(os.ReadFile(environment["CONFIG_CALLS"])).To(Equal([]byte("helper\x00duration\x0010\x0015\x00")))
			imageGit(caller, "init", "-q")
			marker := filepath.Join(filepath.Dir(root), "FALLBACK_OWNER")
			writeFixture(filepath.Join(caller, "factory"), []byte("#!/bin/sh\nprintf called > \"$HELPER_DECOY\"\nexec \"$CONFIG_NATIVE\" \"$@\"\n"), 0700)
			environment["HELPER_DECOY"] = marker
			failed, nonExecutable := filepath.Join(root, "failed runtime"), filepath.Join(root, "non-executable runtime")
			writeFixture(failed, []byte("#!/bin/sh\nexit 47\n"), 0700)
			writeFixture(nonExecutable, []byte("#!/bin/sh\nexit 0\n"), 0600)
			for _, invalid := range []string{"", "relative", filepath.Join(root, "absent"), nonExecutable, failed} {
				environment["FACTORY_RUNTIME_BINARY"] = invalid
				refusal := `. "$1/events.sh"; . "$1/timing.sh"; . "$1/hookspath.sh"; factory_log_event gate reason; printf '%s\000' "$?"; factory_now; printf '\000'; factory_duration 10 15; printf '\000'; hookspath_status "$2" >/dev/null 2>&1; printf '%s' "$?"`
				out := handoffNativeExactProcess(caller, environment, "", shell, "-c", refusal, "invalid-helper", filepath.Join(root, "scripts/lib"), caller)
				Expect(out.status).To(BeZero(), "%s: %+v", invalid, out)
				Expect(out.stderr).To(BeEmpty())
				fields := strings.Split(out.stdout, "\x00")
				Expect(fields).To(HaveLen(4))
				Expect(fields[:3]).To(Equal([]string{"0", "0", "unknown"}))
				Expect(fields[3]).NotTo(Equal("0"))
				_, err := os.Lstat(marker)
				Expect(os.IsNotExist(err)).To(BeTrue())
			}
			delete(environment, "FACTORY_RUNTIME_BINARY")
			Expect(handoffNativeExactProcess(caller, environment, "", shell, "-c", probe, "caller-helper", filepath.Join(root, "scripts/lib"))).To(Equal(cliResult{"5s", "", 0}))
			Expect(os.ReadFile(marker)).To(Equal([]byte("called")))
			// Loading color cannot change any non-color helper's caller-Git owner.
			// per docs/adr/0137-go-source-shared-helpers.md:38
			callerCalls := filepath.Join(filepath.Dir(root), "caller helper calls")
			environment["HELPER_CALLER_CALLS"], environment["FACTORY_SOURCE_ROOT"] = callerCalls, root
			environment["FACTORY_EVENT_LOG"] = filepath.Join(filepath.Dir(root), "source order events")
			writeFixture(filepath.Join(caller, "factory"), []byte("#!/bin/sh\nprintf '%s\\000' \"$@\" >> \"$HELPER_CALLER_CALLS\"\nexec \"$CONFIG_NATIVE\" \"$@\"\n"), 0700)
			loadHelpers := `. "$1/events.sh"; . "$1/timing.sh"; . "$1/hookspath.sh"; `
			callHelpers := `factory_duration 10 15; printf '\000'; factory_log_event order reason; hookspath_status "$2"; printf '\000'; `
			for _, colorFirst := range []bool{false, true} {
				writeFixture(environment["CONFIG_CALLS"], nil, 0600)
				writeFixture(callerCalls, nil, 0600)
				ordered, count := loadHelpers+callHelpers+`. "$1/color.sh"; `+callHelpers, 2
				if colorFirst {
					ordered, count = `. "$1/color.sh"; `+loadHelpers+callHelpers, 1
				}
				out := handoffNativeExactProcess(caller, environment, "", shell, "-c", ordered, "helper-source-order", filepath.Join(root, "scripts/lib"), caller)
				Expect(out).To(Equal(cliResult{strings.Repeat("5s\x00absent\t"+filepath.Join(caller, ".git/hooks/pre-push")+"\x00", count), "", 0}))
				Expect(os.ReadFile(environment["CONFIG_CALLS"])).To(Equal([]byte("helper\x00palette\x000\x00")), "color source root owns only palette/action-box, color-first=%t", colorFirst)
				Expect(os.ReadFile(callerCalls)).To(Equal([]byte(strings.Repeat("helper\x00duration\x0010\x0015\x00helper\x00event\x00order\x00reason\x00helper\x00hookspath\x00"+caller+"\x00", count))))
			}
		}
		if runtime.GOOS == "linux" {
			Expect(posix).To(BeTrue(), "Linux qualification must exercise the true POSIX branch")
		}
		// Events must remain safe even before native invocation is possible.
		for _, missing := range []string{"runtime/shell/helpers.sh", "runtime/shell/source.sh"} {
			root, caller, environment := sourceHelperNativeFixture()
			Expect(os.Remove(filepath.Join(root, missing))).To(Succeed())
			out := handoffNativeExactProcess(caller, environment, "", "/bin/bash", "-c", `set -eu; . "$1"; factory_log_event gate reason; printf survived`, "incomplete-events", filepath.Join(root, "scripts/lib/events.sh"))
			Expect(out).To(Equal(cliResult{"survived", "", 0}))
			_, err := os.Lstat(environment["CONFIG_CALLS"])
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	})

	// per docs/adr/0137-go-source-shared-helpers.md:77
	// per docs/adr/0137-go-source-shared-helpers.md:82
	It("compiles a private source owner and keeps real gate decisions independent of event failures", func() {
		source := newSourceEntrypointFixture()
		sourceHelperLibraryAssets(source.root)
		environment := maps.Clone(source.environment)
		maps.Copy(environment, sourceReviewGoCaches())
		home := filepath.Join(filepath.Dir(source.root), "compiler home")
		Expect(os.Mkdir(home, 0700)).To(Succeed())
		maps.Copy(environment, map[string]string{"HOME": home, "GOENV": "off", "TMPDIR": filepath.Dir(source.root)})
		log := filepath.Join(filepath.Dir(source.root), "events")
		environment["FACTORY_EVENT_LOG"] = log
		probe := `. "$1/timing.sh"; . "$1/events.sh"; factory_duration 10 15; factory_log_event source-probe 'literal $(touch HELPER_INJECTION)'`
		before := sourceEntrypointSnapshot(source.root)
		out := handoffNativeExactProcess(source.caller, environment, "", "/bin/bash", "-c", probe, "compiled-helper", filepath.Join(source.root, "scripts/lib"))
		Expect(out).To(Equal(cliResult{"5s", "", 0}))
		Expect(sourceEntrypointSnapshot(source.root)).To(Equal(before))
		compiled, err := os.ReadFile(filepath.Join(source.root, "factory-go"))
		Expect(err).NotTo(HaveOccurred(), "a real source helper must compile its private owner")
		Expect(compiled).NotTo(BeEmpty())
		writeFixture(filepath.Join(source.root, "factory"), compiled, 0700)
		gate, err := os.ReadFile("../scripts/hooks/direct-main-push-block.sh")
		Expect(err).NotTo(HaveOccurred())
		path := filepath.Join(source.root, "scripts/hooks/direct-main-push-block.sh")
		writeFixture(path, gate, 0700)
		for _, blocked := range []bool{false, true} {
			if blocked {
				environment["FACTORY_EVENT_LOG"] = filepath.Dir(log)
			}
			denied := handoffNativeExactProcess(source.caller, environment, "refs/heads/topic a refs/heads/main b\n", path)
			Expect(denied.status).To(Equal(1), "%+v", denied)
			allowed := handoffNativeExactProcess(source.caller, environment, "refs/heads/topic a refs/heads/topic b\n", path)
			Expect(allowed.status).To(BeZero(), "%+v", allowed)
		}
		events, err := os.ReadFile(log)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(events)).To(ContainSubstring("\tsource-probe\tliteral $(touch HELPER_INJECTION)\n"))
		Expect(string(events)).To(ContainSubstring("\tdirect-main-push-block\tdirect push to main\n"))
		_, err = os.Lstat(filepath.Join(source.caller, "HELPER_INJECTION"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0137-go-source-shared-helpers.md:29
	// per docs/adr/0137-go-source-shared-helpers.md:45
	// per docs/adr/0137-go-source-shared-helpers.md:60
	It("applies native palette plans for real output surfaces and requires a valid POSIX source root", func() {
		for _, shell := range sourceConfigRoleShells() {
			root, caller, environment := sourceHelperNativeFixture()
			isBash := handoffNativeExactProcess(caller, environment, "", shell, "-c", `printf '%s' "${BASH_VERSION-}"`).stdout != ""
			environment["FACTORY_SOURCE_ROOT"] = root
			probe := `. "$1" || exit "$?"; printf '%s\000%s\000%s\000%s\000%s\000%s' "$C_RESET" "$C_BOLD" "$C_DIM" "$C_YELLOW" "$C_GREEN" "$C_RED"`
			library := filepath.Join(root, "scripts/lib/color.sh")
			for _, terminal := range []bool{false, true} {
				for _, noColor := range []string{"", "literal override"} {
					environment["NO_COLOR"] = noColor
					var out cliResult
					if terminal {
						out = sourceHelperTerminal(caller, environment, shell, probe, library)
					} else {
						out = handoffNativeExactProcess(caller, environment, "", shell, "-c", probe, "palette-pipe", library)
					}
					values := []string{"", "", "", "", "", ""}
					if terminal && noColor == "" {
						values = []string{"\x1b[0m", "\x1b[1m", "\x1b[2m", "\x1b[33m", "\x1b[32m", "\x1b[31m"}
					}
					Expect(out).To(Equal(cliResult{strings.Join(values, "\x00"), "", 0}), "%s tty=%t NO_COLOR=%q", shell, terminal, noColor)
				}
			}
			calls, err := os.ReadFile(environment["CONFIG_CALLS"])
			Expect(err).NotTo(HaveOccurred())
			Expect(string(calls)).To(Equal(strings.Join([]string{"helper", "palette", "0", "helper", "palette", "0", "helper", "palette", "1", "helper", "palette", "1", ""}, "\x00")))
			if !isBash {
				// A refused load also releases its temporary definition.
				// per docs/adr/0137-go-source-shared-helpers.md:132
				for _, invalid := range []string{"", "relative", filepath.Join(root, "absent")} {
					environment["FACTORY_SOURCE_ROOT"] = invalid
					environment["FACTORY_RUNTIME_BINARY"] = filepath.Join(root, "factory")
					refusal := `. "$1"; status=$?; if command -v _factory_posix_color_load >/dev/null; then printf loader-remains; fi; exit "$status"`
					out := handoffNativeExactProcess(caller, environment, "", shell, "-c", refusal, "invalid-color-root", library)
					Expect(out.status).NotTo(BeZero(), "invalid source root %q: %+v", invalid, out)
					Expect(out.stdout).To(BeEmpty())
				}
				// The root resolver remains available for a later action-box call.
				// per docs/adr/0137-go-source-shared-helpers.md:134
				environment["FACTORY_SOURCE_ROOT"] = root
				delete(environment, "FACTORY_RUNTIME_BINARY")
				later := `. "$1" || exit "$?"; if command -v _factory_posix_color_load >/dev/null; then exit 96; fi; cd "$2"; action_box title line`
				Expect(handoffNativeExactProcess(root, environment, "", shell, "-c", later, "later-color-action", library, caller)).To(Equal(cliResult{"┌─ title \n│ line\n└─\n", "", 0}))
				delete(environment, "FACTORY_SOURCE_ROOT")
				imageGit(caller, "init", "-q")
				sourceHelperLibraryAssets(caller)
				writeFixture(filepath.Join(caller, "factory"), candidateBinary, 0700)
				Expect(handoffNativeExactProcess(caller, environment, "", shell, "-c", probe, "caller-color-root", library)).To(Equal(cliResult{strings.Repeat("\x00", 5), "", 0}))
			}
		}
	})

	// per docs/adr/0137-go-source-shared-helpers.md:102
	It("classifies the hooks path Git actually resolves through the source helper", func() {
		root, caller, environment := sourceHelperNativeFixture()
		imageGit(caller, "init", "-q")
		hook := filepath.Join(caller, ".githooks/pre-push")
		writeFixture(hook, []byte("#!/bin/sh\nexit 0\n"), 0600)
		for _, state := range []string{"absent", "inert", "armed", "hijacked"} {
			resolved := filepath.Join(caller, ".git/hooks/pre-push")
			if state != "absent" {
				imageGit(caller, "config", "core.hooksPath", ".githooks")
				resolved = hook
			}
			if state == "armed" {
				Expect(os.Chmod(hook, 0700)).To(Succeed())
			}
			if state == "hijacked" {
				foreign := filepath.Join(filepath.Dir(root), "foreign hooks")
				imageGit(caller, "config", "core.hooksPath", foreign)
				resolved = filepath.Join(foreign, "pre-push")
			}
			out := handoffNativeExactProcess(caller, environment, "", "/bin/bash", "-c", `. "$1"; hookspath_status "$2"`, "real-hookspath", filepath.Join(root, "scripts/lib/hookspath.sh"), caller)
			Expect(out).To(Equal(cliResult{state + "\t" + resolved, "", 0}))
		}
		calls, err := os.ReadFile(environment["CONFIG_CALLS"])
		Expect(err).NotTo(HaveOccurred())
		Expect(string(calls)).To(Equal(strings.Repeat("helper\x00hookspath\x00"+caller+"\x00", 4)))
	})

	// per docs/adr/0137-go-source-shared-helpers.md:67
	It("fetches and refreshes source caches without executing their timing library or runtime", func() {
		root, caller, environment := sourceHelperNativeFixture()
		base := filepath.Dir(root)
		imageGit(root, "init", "-q")
		marker := filepath.Join(base, "SELECTED_SOURCE_EXECUTED")
		environment["HELPER_DECOY"] = marker
		poison := []byte("#!/bin/sh\nprintf called > \"$HELPER_DECOY\"\nexit 97\n")
		writeFixture(filepath.Join(root, "factory"), poison, 0700)
		writeFixture(filepath.Join(root, "scripts/lib/timing.sh"), []byte("printf sourced > \"$HELPER_DECOY\"\nfactory_duration() { printf poisoned; }\n"), 0644)
		initAdapter, err := os.ReadFile("../scripts/factory-init.sh")
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(root, "scripts/factory-init.sh"), initAdapter, 0700)
		imageGit(root, "add", "--all")
		imageGit(root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "fixture source")
		imageGit(root, "branch", "helper-bootstrap")
		installer, err := os.ReadFile("../install.sh")
		Expect(err).NotTo(HaveOccurred())
		path := filepath.Join(base, "installer.sh")
		writeFixture(path, installer, 0700)
		cache := filepath.Join(base, "cache")
		environment["FACTORY_HOME"], environment["FACTORY_REPO"] = cache, (&url.URL{Scheme: "file", Path: root}).String()
		tools := filepath.Join(base, "forbidden compiler")
		writeFixture(filepath.Join(tools, "go"), poison, 0700)
		environment["PATH"] = tools + ":" + environment["PATH"]
		for _, args := range [][]string{{"--ref", "helper-bootstrap"}, {"upgrade", "--ref", "helper-bootstrap"}} {
			out := handoffNativeExactProcess(caller, environment, "", "/bin/sh", append([]string{path}, args...)...)
			Expect(out.status).To(BeZero(), "%v: %+v", args, out)
			Expect(out.stdout).NotTo(ContainSubstring("poisoned"))
			_, err := os.Lstat(marker)
			Expect(os.IsNotExist(err)).To(BeTrue(), "Git-only acquisition must not source fetched timing code")
		}
		Expect(strings.TrimSpace(imageGit(cache, "rev-parse", "HEAD"))).To(Equal(strings.TrimSpace(imageGit(root, "rev-parse", "HEAD"))))
		_, err = os.Lstat(filepath.Join(cache, "factory-go"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})
