package acceptance_test

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Current policy fixtures carry the canonical source bridge closure privately.
// per docs/adr/0136-go-source-configuration-entrypoints.md:72
func sourceConfigLibraryAssets(root string) {
	GinkgoHelper()
	for _, path := range []string{"scripts/lib/config.sh", "scripts/lib/roles.sh", "scripts/lib/budget-config.sh", "runtime/shell/readers.sh", "runtime/shell/config.sh", "runtime/shell/helpers.sh", "runtime/shell/source.sh"} {
		body, err := os.ReadFile(filepath.Join("..", path))
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(root, path), body, 0644)
	}
}

// per docs/adr/0136-go-source-configuration-entrypoints.md:18
func sourceConfigMigrationAdapter(root string) string {
	GinkgoHelper()
	body, err := os.ReadFile("../scripts/factory-migrate-config.sh")
	Expect(err).NotTo(HaveOccurred())
	path := filepath.Join(root, "scripts/factory-migrate-config.sh")
	writeFixture(path, body, 0755)
	return path
}

// A recording owner delegates every semantic operation to the actual candidate.
// per docs/adr/0136-go-source-configuration-entrypoints.md:97
func sourceConfigNativeFixture() (string, string, map[string]string) {
	GinkgoHelper()
	root, caller := fixtureDirectories()
	sourceConfigLibraryAssets(root)
	sourceConfigMigrationAdapter(root)
	base := filepath.Dir(root)
	writeFixture(filepath.Join(root, "native"), candidateBinary, 0700)
	writeFixture(filepath.Join(root, "factory"), []byte("#!/bin/sh\nprintf '%s\\000' \"$@\" >> \"$CONFIG_CALLS\"\nexec \"$CONFIG_NATIVE\" \"$@\"\n"), 0700)
	return root, caller, map[string]string{"PATH": "/usr/bin:/bin", "HOME": caller, "TMPDIR": base, "BASH_ENV": "", "ENV": "", "GORACE": "atexit_sleep_ms=0", "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull, "GIT_OPTIONAL_LOCKS": "0", "CONFIG_NATIVE": filepath.Join(root, "native"), "CONFIG_CALLS": filepath.Join(base, "native calls"), "FACTORY_CONFIG": filepath.Join(caller, "factory.yaml")}
}

// /bin/sh may itself be Bash; a provisioned Dash supplies true POSIX evidence.
// per docs/adr/0136-go-source-configuration-entrypoints.md:58
func sourceConfigRoleShells() []string {
	shells := []string{"/bin/bash", "/bin/sh"}
	if dash, err := exec.LookPath("dash"); err == nil {
		shells = append(shells, dash)
	}
	return shells
}

var _ = Describe("Go source runtime entrypoints configuration adapters", func() {
	// per docs/adr/0136-go-source-configuration-entrypoints.md:18
	It("preserves migration channels and its physical owner through direct and Bash invocation", func() {
		for _, bash := range []bool{false, true} {
			for _, alias := range []bool{false, true} {
				root, caller := fixtureDirectories()
				base := filepath.Dir(root)
				owner := filepath.Join(base, "config owner ' $(literal)")
				if alias {
					owner += "\n"
				}
				Expect(os.Rename(root, owner)).To(Succeed())
				sourceConfigLibraryAssets(owner)
				program := sourceConfigMigrationAdapter(owner)
				record, input, decoy := filepath.Join(base, "argv"), filepath.Join(base, "stdin"), filepath.Join(base, "decoy")
				writeFixture(filepath.Join(owner, "factory"), []byte("#!/bin/sh\nprintf '%s\\000' \"$PWD\" \"$CONFIG_LITERAL\" \"$@\" > \"$CONFIG_ARGV\"\n/bin/cat > \"$CONFIG_STDIN\"\nprintf 'owner stdout\\n'\nprintf 'owner stderr\\n' >&2\nexit 63\n"), 0700)
				foreign, tools := filepath.Join(base, "foreign"), filepath.Join(base, "tools")
				for _, path := range []string{filepath.Join(caller, "factory"), filepath.Join(foreign, "factory"), filepath.Join(tools, "factory")} {
					writeFixture(path, []byte("#!/bin/sh\nprintf called > \"$CONFIG_DECOY\"\nexit 64\n"), 0700)
				}
				if alias {
					Expect(os.Symlink(filepath.Join(owner, "scripts"), filepath.Join(foreign, "scripts"))).To(Succeed())
					program = filepath.Join(foreign, "scripts/factory-migrate-config.sh")
				}
				environment := map[string]string{"PATH": tools + ":/usr/bin:/bin", "BASH_ENV": "", "ENV": "", "CONFIG_ARGV": record, "CONFIG_STDIN": input, "CONFIG_DECOY": decoy, "CONFIG_LITERAL": "literal $(not-code)\nvalue\n", "FACTORY_RUNTIME_BINARY": filepath.Join(foreign, "factory"), "FACTORY_HOME": foreign}
				args := []string{"--dry-run", "", "literal $(not-code)\nline", "--"}
				stdin := "borrowed stdin\x00literal $(not-code)\n"
				beforeOwner, beforeCaller := installationSnapshot(owner), installationSnapshot(caller)
				observed := sourceStructuralInvoke(caller, environment, stdin, program, bash, args...)
				Expect(observed).To(Equal(cliResult{"owner stdout\n", "owner stderr\n", 63}), "bash=%t directory_alias=%t", bash, alias)
				raw, err := os.ReadFile(record)
				Expect(err).NotTo(HaveOccurred())
				Expect(strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")).To(Equal(append([]string{caller, environment["CONFIG_LITERAL"], "migrate-config"}, args...)))
				Expect(os.ReadFile(input)).To(Equal([]byte(stdin)))
				_, err = os.Lstat(decoy)
				Expect(os.IsNotExist(err)).To(BeTrue())
				Expect(installationSnapshot(owner)).To(Equal(beforeOwner))
				Expect(installationSnapshot(caller)).To(Equal(beforeCaller))
			}
		}
	})

	// per docs/adr/0136-go-source-configuration-entrypoints.md:21
	// per docs/adr/0136-go-source-configuration-entrypoints.md:40
	It("refuses unavailable or aliased owners without invoking alternate executables", func() {
		for _, mode := range []string{"missing", "non-executable", "failed", "file-alias"} {
			for _, subject := range []string{"migration", "config", "roles", "budget-config"} {
				root, caller, environment := sourceConfigNativeFixture()
				owner := filepath.Join(root, "factory")
				marker := filepath.Join(filepath.Dir(root), "fallback called")
				tools, foreign := filepath.Join(filepath.Dir(root), "tools"), filepath.Join(filepath.Dir(root), "foreign")
				for _, path := range []string{filepath.Join(tools, "factory"), filepath.Join(caller, "factory"), filepath.Join(foreign, "factory")} {
					writeFixture(path, []byte("#!/bin/sh\nprintf called > \"$CONFIG_DECOY\"\nexit 87\n"), 0700)
				}
				maps.Copy(environment, map[string]string{"PATH": tools + ":/usr/bin:/bin", "CONFIG_DECOY": marker, "FACTORY_RUNTIME_BINARY": filepath.Join(foreign, "factory"), "FACTORY_HOME": foreign})
				writeFixture(owner, []byte("#!/bin/sh\nprintf 'owner failed\\n' >&2\nexit 47\n"), 0700)
				switch mode {
				case "missing":
					Expect(os.Remove(owner)).To(Succeed())
				case "non-executable":
					Expect(os.Chmod(owner, 0600)).To(Succeed())
				}
				program := filepath.Join(root, "scripts/lib", subject+".sh")
				if subject == "migration" {
					program = filepath.Join(root, "scripts/factory-migrate-config.sh")
				}
				if mode == "file-alias" {
					alias := filepath.Join(caller, "aliased source")
					Expect(os.Symlink(program, alias)).To(Succeed())
					program = alias
				}
				before := installationSnapshot(caller)
				var observed cliResult
				if subject == "migration" {
					observed = sourceStructuralInvoke(caller, environment, "", program, true)
				} else {
					probe := `. "$1" || exit "$?"; case "$2" in config) factory_config_get key ;; roles) role_tier reviewer ;; esac`
					if subject == "budget-config" {
						probe = `. "$3/scripts/lib/config.sh"; . "$3/scripts/lib/roles.sh"; . "$1"`
					}
					observed = handoffNativeExactProcess(caller, environment, "", "/bin/bash", "-c", probe, "refusal", program, subject, root)
				}
				Expect(observed.status).NotTo(BeZero(), "%s/%s: %+v", subject, mode, observed)
				if mode == "failed" {
					Expect(observed.status).To(Equal(47))
					Expect(observed.stderr).To(ContainSubstring("owner failed"))
				}
				_, err := os.Lstat(marker)
				Expect(os.IsNotExist(err)).To(BeTrue())
				Expect(installationSnapshot(caller)).To(Equal(before))
			}
		}
	})

	// per docs/adr/0136-go-source-configuration-entrypoints.md:26
	// per docs/adr/0136-go-source-configuration-entrypoints.md:101
	It("compiles a private source owner and migrates the caller project with its recovery file intact", func() {
		source := newSourceEntrypointFixture()
		sourceConfigLibraryAssets(source.root)
		program := sourceConfigMigrationAdapter(source.root)
		environment := maps.Clone(source.environment)
		maps.Copy(environment, sourceReviewGoCaches())
		compilerHome := filepath.Join(filepath.Dir(source.root), "compiler home")
		Expect(os.Mkdir(compilerHome, 0700)).To(Succeed())
		maps.Copy(environment, map[string]string{"HOME": compilerHome, "TMPDIR": filepath.Dir(source.root), "GOENV": "off"})
		imageGit(source.caller, "init", "-q")
		nested := filepath.Join(source.caller, "nested")
		Expect(os.Mkdir(nested, 0700)).To(Succeed())
		legacy := filepath.Join(source.caller, "factory.config")
		writeFixture(filepath.Join(source.caller, "factory.yaml"), []byte("cost_profile: mine\nreview_model:\n"), 0600)
		writeFixture(legacy, []byte("COST_PROFILE=legacy\nREVIEW_MODEL='literal $(touch CONFIG_INJECTION)'\n"), 0600)
		beforeOwner, beforeCaller := sourceEntrypointSnapshot(source.root), installationSnapshot(source.caller)
		preview := sourceStructuralInvoke(nested, environment, "", program, false, "--dry-run")
		Expect(preview.status).To(BeZero(), "%+v", preview)
		Expect(preview.stdout).To(ContainSubstring("would add: review_model: \"literal $(touch CONFIG_INJECTION)\""))
		Expect(installationSnapshot(source.caller)).To(Equal(beforeCaller))
		Expect(sourceEntrypointSnapshot(source.root)).To(Equal(beforeOwner))
		compiled, err := os.ReadFile(filepath.Join(source.root, "factory-go"))
		Expect(err).NotTo(HaveOccurred(), "the actual source launcher must compile its private owner")
		Expect(compiled).NotTo(BeEmpty())
		writeFixture(filepath.Join(source.root, "factory"), compiled, 0700)
		original, err := os.Stat(legacy)
		Expect(err).NotTo(HaveOccurred())
		applied := sourceStructuralInvoke(nested, environment, "", program, true)
		Expect(applied.status).To(BeZero(), "%+v", applied)
		Expect(os.ReadFile(filepath.Join(source.caller, "factory.yaml"))).To(Equal([]byte("cost_profile: mine\nreview_model: \"literal $(touch CONFIG_INJECTION)\"\nconfig_migrated: \"yes\"\n")))
		backup, err := os.Stat(legacy + ".migrated")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(original, backup)).To(BeTrue())
		writeFixture(legacy, []byte("COST_PROFILE=second\n"), 0600)
		before := installationSnapshot(source.caller)
		refused := sourceStructuralInvoke(nested, environment, "", program, false)
		Expect(refused.status).To(Equal(1))
		Expect(refused.stderr).To(ContainSubstring("already exists"))
		Expect(installationSnapshot(source.caller)).To(Equal(before))
		_, err = os.Lstat(filepath.Join(source.caller, "CONFIG_INJECTION"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0136-go-source-configuration-entrypoints.md:42
	It("sources configuration and roles without native or tool calls or caller-state changes", func() {
		for _, library := range []string{"config", "roles"} {
			shells := []string{"/bin/bash"}
			if library == "roles" {
				shells = sourceConfigRoleShells()
			}
			for _, shell := range shells {
				root, caller, environment := sourceConfigNativeFixture()
				capability := handoffNativeExactProcess(caller, environment, "", shell, "-c", `printf '%s' "${BASH_VERSION-}"`)
				if capability.stdout == "" {
					maps.Copy(environment, map[string]string{"BASH_VERSION": "forged", "BASH_SOURCE": "forged"})
				}
				tools := filepath.Join(filepath.Dir(root), "forbidden tools")
				marker := filepath.Join(filepath.Dir(root), "source child")
				for _, name := range []string{"go", "git", "factory"} {
					writeFixture(filepath.Join(tools, name), []byte("#!/bin/sh\nprintf called > \"$CONFIG_DECOY\"\nexit 95\n"), 0700)
				}
				writeFixture(filepath.Join(caller, "builtin"), []byte("#!/bin/sh\nprintf builtin > \"$CONFIG_DECOY\"\nexit 95\n"), 0700)
				maps.Copy(environment, map[string]string{"PATH": ":" + tools, "CONFIG_DECOY": marker})
				probe := `set -euf; IFS=:; FACTORY_CONFIG='caller path'; FACTORY_BRIDGE_PROTOCOL=caller; FACTORY_RUNTIME_BINARY='caller runtime'; readonly FACTORY_CONFIG FACTORY_BRIDGE_PROTOCOL FACTORY_RUNTIME_BINARY; cd() { command printf cd > "$CONFIG_DECOY"; return 77; }; pwd() { command printf pwd > "$CONFIG_DECOY"; return 78; }; printf() { command printf printf > "$CONFIG_DECOY"; return 79; }; before="$-"; location="$PWD"; . "$1"; [ "$before" = "$-" ] && [ "$PWD" = "$location" ] && [ "$IFS" = : ] && [ "$#" = 2 ] && [ "$2" = 'literal argument' ] || exit 96; command printf '%s\000%s\000%s' "$FACTORY_CONFIG" "$FACTORY_BRIDGE_PROTOCOL" "$FACTORY_RUNTIME_BINARY"`
				observed := handoffNativeExactProcess(caller, environment, "", shell, "-c", probe, "inert", filepath.Join(root, "scripts/lib", library+".sh"), "literal argument")
				Expect(observed).To(Equal(cliResult{"caller path\x00caller\x00caller runtime", "", 0}), "%s/%s", library, shell)
				for _, path := range []string{marker, environment["CONFIG_CALLS"]} {
					_, err := os.Lstat(path)
					Expect(os.IsNotExist(err)).To(BeTrue())
				}
			}
		}
	})

	// per docs/adr/0136-go-source-configuration-entrypoints.md:33
	// per docs/adr/0136-go-source-configuration-entrypoints.md:58
	It("retains defining Bash ownership and literal POSIX role selection without fallback", func() {
		posix := false
		for _, shell := range sourceConfigRoleShells() {
			root, caller, environment := sourceConfigNativeFixture()
			capability := handoffNativeExactProcess(caller, environment, "", shell, "-c", `printf '%s' "${BASH_VERSION-}"`)
			isBash := capability.stdout != ""
			posix = posix || !isBash
			if !isBash {
				maps.Copy(environment, map[string]string{"BASH_VERSION": "forged", "BASH_SOURCE": "forged"})
			}
			selected, foreign := filepath.Join(root, "factory"), filepath.Join(caller, "selected runtime")
			writeFixture(foreign, []byte("#!/bin/sh\nprintf 'selected stdout'\nprintf 'selected stderr' >&2\nexit 63\n"), 0700)
			environment["FACTORY_RUNTIME_BINARY"] = foreign
			probe := `. "$1" || exit "$?"; cd "$2"; role_tier "$3"`
			observed := handoffNativeExactProcess(root, environment, "", shell, "-c", probe, "role-owner", filepath.Join(root, "scripts/lib/roles.sh"), caller, "literal $(not-code)\nrole")
			if isBash {
				Expect(observed).To(Equal(cliResult{"default", "", 0}))
				calls, err := os.ReadFile(environment["CONFIG_CALLS"])
				Expect(err).NotTo(HaveOccurred())
				Expect(string(calls)).To(Equal("role\x00tier\x00literal $(not-code)\nrole\x00"))
			} else {
				Expect(observed).To(Equal(cliResult{"selected stdout", "selected stderr", 63}))
				delete(environment, "FACTORY_RUNTIME_BINARY")
				outside := handoffNativeExactProcess(caller, environment, "", shell, "-c", `. "$1"; role_tier reviewer`, "outside-repository", filepath.Join(root, "scripts/lib/roles.sh"))
				Expect(outside.status).NotTo(BeZero())
				Expect(outside.stdout).To(BeEmpty())
				imageGit(caller, "init", "-q")
				writeFixture(filepath.Join(caller, "factory"), candidateBinary, 0700)
				Expect(handoffNativeExactProcess(caller, environment, "", shell, "-c", `. "$1"; resolve_tier economy economy`, "caller-root", filepath.Join(root, "scripts/lib/roles.sh"))).To(Equal(cliResult{"economy", "", 0}))
				nonExecutable := filepath.Join(caller, "non-executable runtime")
				writeFixture(nonExecutable, []byte("#!/bin/sh\nexit 0\n"), 0600)
				for _, invalid := range []string{"", "relative-runtime", filepath.Join(caller, "absent"), nonExecutable, foreign} {
					environment["FACTORY_RUNTIME_BINARY"] = invalid
					refused := handoffNativeExactProcess(caller, environment, "", shell, "-c", `. "$1"; role_tier reviewer`, "refused-runtime", filepath.Join(root, "scripts/lib/roles.sh"))
					Expect(refused.status).NotTo(BeZero(), "%s: %+v", invalid, refused)
					Expect(refused.stdout).NotTo(Equal("frontier"))
				}
				environment["FACTORY_RUNTIME_BINARY"] = selected
			}
			missing := handoffNativeExactProcess(caller, environment, "", shell, "-u", "-c", `. "$1"; role_tier`, "nounset", filepath.Join(root, "scripts/lib/roles.sh"))
			Expect(missing.status).NotTo(BeZero())
			Expect(missing.stdout).To(BeEmpty())
		}
		if runtime.GOOS == "linux" {
			Expect(posix).To(BeTrue(), "Linux source qualification must exercise the true POSIX branch")
		}
	})

	// per docs/adr/0136-go-source-configuration-entrypoints.md:49
	It("uses canonical native configuration and role operations while keeping caller-local controls literal", func() {
		root, caller, environment := sourceConfigNativeFixture()
		writeFixture(environment["FACTORY_CONFIG"], []byte("model_provider: YAML\nreview_model:\nlocal_hooks: gate-a.sh --strict, gate-b.sh argument\n"), 0600)
		legacy := filepath.Join(caller, "factory.config")
		writeFixture(legacy, []byte("MODEL_PROVIDER=legacy\nREVIEW_MODEL='legacy # data'\n"), 0600)
		value := "literal $(touch CONFIG_INJECTION) & | \\ path"
		probe := `set -eu; . "$1/scripts/lib/config.sh"; . "$1/scripts/lib/roles.sh"; FACTORY_CONFIG="$2"; FACTORY_BRIDGE_PROTOCOL=caller; readonly FACTORY_CONFIG FACTORY_BRIDGE_PROTOCOL; MODEL_PROVIDER=caller; readonly MODEL_PROVIDER; factory_config_export; printf '%s\000%s\000' "$MODEL_PROVIDER" "$REVIEW_MODEL"; factory_config_load_legacy "$3" MODEL_PROVIDER; factory_config_set literal "$4"; factory_config_file; printf '\000'; factory_config_get literal; printf '\000'; factory_config_has review_model; if factory_config_has absent; then exit 91; fi; factory_local_hooks; role_tier reviewer; printf '\000'; resolve_tier standard economy; printf '\000%s' "$FACTORY_BRIDGE_PROTOCOL"`
		observed := handoffNativeExactProcess(caller, environment, "", "/bin/bash", "-c", probe, "configuration", root, environment["FACTORY_CONFIG"], legacy, value)
		Expect(observed).To(Equal(cliResult{"caller\x00legacy # data\x00" + environment["FACTORY_CONFIG"] + "\x00" + value + "\x00gate-a.sh --strict\ngate-b.sh argument\nfrontier\x00default\x00caller", "", 0}))
		calls, err := os.ReadFile(environment["CONFIG_CALLS"])
		Expect(err).NotTo(HaveOccurred())
		for _, command := range []string{"config\x00export\x00", "config\x00legacy\x00", "config\x00set\x00literal\x00", "config\x00file\x00", "config\x00get\x00literal\x00", "config\x00has\x00", "config\x00hooks\x00commas\x00", "role\x00tier\x00", "role\x00resolve\x00"} {
			Expect(string(calls)).To(ContainSubstring(command))
		}
		_, err = os.Lstat(filepath.Join(caller, "CONFIG_INJECTION"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0136-go-source-configuration-entrypoints.md:45
	It("applies the native budget plan at source time and resolves models for every harness", func() {
		root, caller, environment := sourceConfigNativeFixture()
		imageGit(caller, "init", "-q")
		writeFixture(environment["FACTORY_CONFIG"], []byte("budget_enabled: true\nbudget_max_attempts: 7\ncost_profile: economy\ncodex_economy_model: YAML\nclaude_frontier_model: claude/frontier\nopencode_default_model: opencode/default\n"), 0600)
		probe := `set -eu; . "$1/scripts/lib/config.sh"; . "$1/scripts/lib/roles.sh"; FACTORY_CONFIG="$2"; FACTORY_BRIDGE_PROTOCOL=caller; readonly FACTORY_CONFIG FACTORY_BRIDGE_PROTOCOL; CODEX_ECONOMY_MODEL='literal $(touch BUDGET_INJECTION)'; . "$1/scripts/lib/budget-config.sh"; for key in FACTORY_BUDGET_ROOT FACTORY_BUDGET_ENABLED FACTORY_BUDGET_MAX_ATTEMPTS FACTORY_BUDGET_MAX_SESSION_RUNS FACTORY_BUDGET_TIMEOUT_SECONDS FACTORY_BUDGET_SESSION_SECONDS FACTORY_BUDGET_MAX_CONCURRENT FACTORY_BUDGET_ESTIMATED_USD FACTORY_BUDGET_ACTION; do printf '%s\000' "${!key}"; done; factory_budget_model codex refactorer; printf '\000'; factory_budget_model claude reviewer; printf '\000'; factory_budget_model opencode implementer; printf '\000'; /bin/sh -c 'printf "%s\000%s\000%s" "$FACTORY_BUDGET_ENABLED" "$CODEX_ECONOMY_MODEL" "$FACTORY_BRIDGE_PROTOCOL"'`
		observed := handoffNativeExactProcess(caller, environment, "", "/bin/bash", "-c", probe, "budget", root, environment["FACTORY_CONFIG"])
		expected := strings.Join([]string{caller, "true", "7", "5", "300", "900", "1", "", "stop", "literal $(touch BUDGET_INJECTION)", "claude/frontier", "opencode/default", "true", "literal $(touch BUDGET_INJECTION)", ""}, "\x00")
		Expect(observed).To(Equal(cliResult{expected, "", 0}))
		calls, err := os.ReadFile(environment["CONFIG_CALLS"])
		Expect(err).NotTo(HaveOccurred())
		Expect(string(calls)).To(ContainSubstring("helper\x00budget-plan\x00"))
		Expect(strings.Count(string(calls), "helper\x00budget-model\x00")).To(Equal(3))
		_, err = os.Lstat(filepath.Join(caller, "BUDGET_INJECTION"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0136-go-source-configuration-entrypoints.md:72
	It("keeps the actual current test-edit gate reading caller policy through the native source closure", func() {
		root, caller, environment := sourceConfigNativeFixture()
		for _, path := range []string{"scripts/hooks/test-edit-denial.sh", "scripts/lib/events.sh"} {
			body, err := os.ReadFile(filepath.Join("..", path))
			Expect(err).NotTo(HaveOccurred())
			writeFixture(filepath.Join(root, path), body, 0755)
		}
		writeFixture(environment["FACTORY_CONFIG"], []byte("test_file_patterns: _test\\.go$\n"), 0600)
		writeFixture(filepath.Join(root, "factory.yaml"), []byte("test_file_patterns:\n"), 0600)
		environment["FACTORY_EVENT_LOG"] = filepath.Join(filepath.Dir(root), "gate events")
		for _, role := range []string{"implementer", "spec-writer"} {
			environment["FACTORY_AGENT_ROLE"] = role
			observed := handoffNativeExactProcess(caller, environment, "", filepath.Join(root, "scripts/hooks/test-edit-denial.sh"), "feature_test.go")
			if role == "implementer" {
				Expect(observed.status).To(Equal(2))
				Expect(observed.stderr).To(ContainSubstring("DENIED: implementer role"))
			} else {
				Expect(observed).To(Equal(cliResult{"", "", 0}))
			}
		}
		calls, err := os.ReadFile(environment["CONFIG_CALLS"])
		Expect(err).NotTo(HaveOccurred())
		Expect(string(calls)).To(ContainSubstring("config\x00get\x00test_file_patterns\x00"))
	})
})
